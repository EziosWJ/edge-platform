package device

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type handlerService struct{}

func (handlerService) Page(context.Context, PageQuery) (Page, error) {
	errorText := "partial"
	return Page{Records: []Device{{
		DeviceID: "cloud-device-01", EdgeID: "edge-01", SourceDeviceID: "source-01",
		CommunicationStatus: StatusDegraded,
		RegisteredAt:        time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC),
		LastSeenAt:          time.Date(2026, 9, 21, 1, 1, 0, 0, time.UTC),
		CommunicationError:  &errorText,
	}}, Total: 1}, nil
}

func (handlerService) Detail(_ context.Context, deviceID string) (Device, error) {
	if deviceID == "missing" {
		return Device{}, ErrNotFound
	}
	return Device{DeviceID: deviceID, EdgeID: "edge-01", SourceDeviceID: "source-01", CommunicationStatus: StatusOnline}, nil
}

type handlerAuthenticator struct{}

func (handlerAuthenticator) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token != "Bearer good" {
		return auth.Principal{}, errors.New("bad token")
	}
	return auth.Principal{UserID: 1}, nil
}

func TestHandlerRequiresAuthenticationAndReturnsOnlyDeviceProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/device")
	group.Use(auth.BearerMiddleware(handlerAuthenticator{}))
	handler, err := NewHandler(handlerService{})
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(group, handler)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/device/page", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/device/page?status=DEGRADED&sourceDeviceId=source-01", nil)
	request.Header.Set("Authorization", "Bearer good")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, value := range []string{"cloud-device-01", "edge-01", "source-01", "DEGRADED", "communicationError"} {
		if !strings.Contains(body, value) {
			t.Fatalf("response body = %s, missing %q", body, value)
		}
	}
	for _, value := range []string{"topic", "payload", "sourceTimestamp", "messageId", "modbus", "channel", "unit", "registerAddress"} {
		if strings.Contains(body, value) {
			t.Fatalf("response leaked non-domain field %q: %s", value, body)
		}
	}
}

func TestHandlerRejectsInvalidQueriesAndSupportsDetail404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/device")
	group.Use(auth.BearerMiddleware(handlerAuthenticator{}))
	handler, err := NewHandler(handlerService{})
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(group, handler)

	for _, path := range []string{
		"/api/device/page?page=0",
		"/api/device/page?page=abc",
		"/api/device/page?pageSize=501",
		"/api/device/page?status=online",
		"/api/device/page?status=UNKNOWN",
		"/api/device/page?deviceId=",
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer good")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400; body=%s", path, response.Code, response.Body.String())
		}
	}

	for _, test := range []struct {
		path       string
		statusCode int
		contains   string
	}{
		{path: "/api/device/cloud-device-01", statusCode: http.StatusOK, contains: `"deviceId":"cloud-device-01"`},
		{path: "/api/device/missing", statusCode: http.StatusNotFound, contains: `"code":404`},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer good")
		router.ServeHTTP(response, request)
		if response.Code != test.statusCode || !strings.Contains(response.Body.String(), test.contains) {
			t.Errorf("GET %s status/body = %d/%s, want %d and %s", test.path, response.Code, response.Body.String(), test.statusCode, test.contains)
		}
	}
}
