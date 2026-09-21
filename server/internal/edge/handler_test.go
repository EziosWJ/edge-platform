package edge

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
	return Page{Records: []Edge{{EdgeID: "edge-01", Status: StatusOnline, RegisteredAt: time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC), LastSeenAt: time.Date(2026, 9, 21, 1, 1, 0, 0, time.UTC)}}, Total: 1}, nil
}

func (handlerService) Detail(_ context.Context, edgeID string) (Edge, error) {
	if edgeID == "missing" {
		return Edge{}, ErrNotFound
	}
	return Edge{EdgeID: edgeID, Status: StatusOnline, RegisteredAt: time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC), LastSeenAt: time.Date(2026, 9, 21, 1, 1, 0, 0, time.UTC)}, nil
}

type handlerAuthenticator struct{}

func (handlerAuthenticator) Authenticate(_ context.Context, token string) (auth.Principal, error) {
	if token != "Bearer good" {
		return auth.Principal{}, errors.New("bad token")
	}
	return auth.Principal{UserID: 1}, nil
}

func TestHandlerRequiresAuthenticationAndListsOnlyEdgeProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/edge")
	group.Use(auth.BearerMiddleware(handlerAuthenticator{}))
	handler, err := NewHandler(handlerService{})
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(group, handler)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/edge/page", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/edge/page?page=1&pageSize=10", nil)
	request.Header.Set("Authorization", "Bearer good")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	if !containsAll(body, `"edgeId":"edge-01"`, `"status":"ONLINE"`, `"registeredAt":"2026-09-21T01:00:00Z"`, `"lastSeenAt":"2026-09-21T01:01:00Z"`) {
		t.Fatalf("response body = %s", body)
	}
	if containsAny(body, "topic", "payload", "sourceTimestamp", "messageId") {
		t.Fatalf("response leaked non-domain fields: %s", body)
	}
}

func TestHandlerRejectsInvalidPageAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/edge")
	group.Use(auth.BearerMiddleware(handlerAuthenticator{}))
	handler, err := NewHandler(handlerService{})
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(group, handler)

	for _, path := range []string{
		"/api/edge/page?page=0",
		"/api/edge/page?page=abc",
		"/api/edge/page?pageSize=0",
		"/api/edge/page?pageSize=501",
		"/api/edge/page?status=online",
		"/api/edge/page?status=UNKNOWN",
		"/api/edge/page?edgeId=",
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer good")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400; body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestHandlerSupportsDetailAndReturnsNotFoundEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/edge")
	group.Use(auth.BearerMiddleware(handlerAuthenticator{}))
	handler, err := NewHandler(handlerService{})
	if err != nil {
		t.Fatal(err)
	}
	RegisterRoutes(group, handler)

	for _, test := range []struct {
		path       string
		statusCode int
		contains   string
	}{
		{path: "/api/edge/edge-detail", statusCode: http.StatusOK, contains: `"edgeId":"edge-detail"`},
		{path: "/api/edge/missing", statusCode: http.StatusNotFound, contains: `"code":404`},
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

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func containsAny(value string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
