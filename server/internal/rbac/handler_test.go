package rbac

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

func newHandlerRouter(t *testing.T, service *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(platformhttp.RequestMetadata())
	RegisterRoutes(router.Group("/api/system"), handler)
	return router
}

func TestCreateRoleUsesPrincipalAndRequestMetadataForAudit(t *testing.T) {
	store := newMemoryStore()
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	router := newHandlerRouter(t, service)

	request := httptest.NewRequest(http.MethodPost, "/api/system/role", strings.NewReader(`{"roleName":"运营","roleCode":"OPS","status":1,"sortOrder":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(platformhttp.RequestIDHeader, "request-42")
	request.Header.Set("User-Agent", "handler-test")
	request.RemoteAddr = "203.0.113.9:1234"
	request = request.WithContext(auth.ContextWithPrincipal(context.Background(), auth.Principal{UserID: 7, JTI: "jti"}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	event := store.lastEvent
	if event.Action != "role.create" || event.ResourceID != 1 || event.Metadata.ActorID != 7 || event.Metadata.RequestID != "request-42" || event.Metadata.ClientIP != "203.0.113.9" || event.Metadata.RequestMethod != http.MethodPost || event.Metadata.RequestURL != "/api/system/role" {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestRoleDetailMapsNotFoundToLegacyBusinessEnvelope(t *testing.T) {
	service, err := NewService(newMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	router := newHandlerRouter(t, service)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/system/role/99", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != platformhttp.CodeNotFound || body.Message != ErrNotFound.Error() {
		t.Fatalf("body = %+v", body)
	}
}
