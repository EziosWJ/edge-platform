package command

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type handlerPermissionChecker struct{ allowed bool }

func (c handlerPermissionChecker) HasPermission(context.Context, int64, string) (bool, error) {
	return c.allowed, nil
}

type permissionSetChecker struct{ permissions map[string]bool }

func (c permissionSetChecker) HasPermission(_ context.Context, _ int64, permission string) (bool, error) {
	return c.permissions[permission], nil
}

func newCommandHandlerRouter(t *testing.T, allowed bool) *gin.Engine {
	t.Helper()
	store := &serviceStore{}
	service, err := NewService(store, serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service, handlerPermissionChecker{allowed: allowed})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithPrincipal(c.Request.Context(), auth.Principal{UserID: 7})
		ctx = platformhttp.ContextWithUserID(ctx, 7)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	group := router.Group("/api/command")
	RegisterRoutes(group, handler)
	return router
}

func TestCreateRequiresServerSideExecutePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCommandHandlerRouter(t, false)
	request := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(`{"commandId":"11111111-1111-4111-8111-111111111111","deviceId":"device","name":"close","args":{}}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCreateReturnsAcceptedAndCanonicalCommand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCommandHandlerRouter(t, true)
	request := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(`{"commandId":"11111111-1111-4111-8111-111111111111","deviceId":"device","name":"close","args":{"b":2,"a":1}}`))
	request.Header.Set("X-Request-ID", "request-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"PENDING"`) || !strings.Contains(response.Body.String(), `"args":{"a":1,"b":2}`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestCommandQueryPermissionsAreCheckedIndependently(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &serviceStore{}
	service, err := NewService(store, serviceDeviceReader{value: device.Device{EdgeID: "edge", SourceDeviceID: "source"}}, "edge")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(service, permissionSetChecker{permissions: map[string]bool{DetailPermission: true}})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithPrincipal(c.Request.Context(), auth.Principal{UserID: 7}))
		c.Next()
	})
	RegisterRoutes(router.Group("/api/command"), handler)

	pageResponse := httptest.NewRecorder()
	router.ServeHTTP(pageResponse, httptest.NewRequest(http.MethodGet, "/api/command/page", nil))
	if pageResponse.Code != http.StatusForbidden {
		t.Fatalf("list status = %d", pageResponse.Code)
	}
	detailResponse := httptest.NewRecorder()
	router.ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/api/command/missing", nil))
	if detailResponse.Code != http.StatusNotFound {
		t.Fatalf("detail status = %d, body=%s", detailResponse.Code, detailResponse.Body.String())
	}
}

func TestCommandPageRejectsNonIntegerRequestedBy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCommandHandlerRouter(t, true)
	request := httptest.NewRequest(http.MethodGet, "/api/command/page?requestedBy=7.5", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestCommandPageAcceptsPendingStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCommandHandlerRouter(t, true)
	request := httptest.NewRequest(http.MethodGet, "/api/command/page?status=PENDING", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}
