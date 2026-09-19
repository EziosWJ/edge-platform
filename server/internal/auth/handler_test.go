package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformerrors "github.com/EziosWJ/edge-platform/server/internal/platform/errors"
	"github.com/gin-gonic/gin"
)

type handlerServiceStub struct {
	loginInput     LoginInput
	loginMetadata  LoginMetadata
	loginResult    LoginResult
	loginErr       error
	logoutErr      error
	currentUser    *CurrentUser
	currentUserErr error
	menus          []CurrentUserMenu
	menusErr       error
	principal      Principal
}

func (s *handlerServiceStub) Login(_ context.Context, input LoginInput, metadata LoginMetadata) (LoginResult, error) {
	s.loginInput = input
	s.loginMetadata = metadata
	return s.loginResult, s.loginErr
}

func (s *handlerServiceStub) Logout(_ context.Context, principal Principal) error {
	s.principal = principal
	return s.logoutErr
}

func (s *handlerServiceStub) CurrentUser(_ context.Context, principal Principal) (*CurrentUser, error) {
	s.principal = principal
	return s.currentUser, s.currentUserErr
}

func (s *handlerServiceStub) CurrentUserMenus(_ context.Context, principal Principal) ([]CurrentUserMenu, error) {
	s.principal = principal
	return s.menus, s.menusErr
}

type authenticatorStub struct {
	principal Principal
	err       error
	token     string
}

func (a *authenticatorStub) Authenticate(_ context.Context, token string) (Principal, error) {
	a.token = token
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	return a.principal, a.err
}

func newAuthRouter(t *testing.T, service *handlerServiceStub, authenticator *authenticatorStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler, err := NewHandler(service, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router, handler)
	return router
}

func TestLoginMatchesFrontendContract(t *testing.T) {
	service := &handlerServiceStub{loginResult: LoginResult{
		TokenName: "Authorization", TokenValue: "Bearer signed-jwt", ExpiresIn: 7200,
	}}
	router := newAuthRouter(t, service, &authenticatorStub{})

	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "frontend-test")
	request.RemoteAddr = "203.0.113.8:4123"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if service.loginInput != (LoginInput{Username: "admin", Password: "admin123"}) {
		t.Fatalf("login input = %+v", service.loginInput)
	}
	if service.loginMetadata.ClientIP != "203.0.113.8" || service.loginMetadata.UserAgent != "frontend-test" {
		t.Fatalf("login metadata = %+v", service.loginMetadata)
	}
	assertJSON(t, response.Body.Bytes(), map[string]any{
		"code": float64(200), "message": "success", "data": map[string]any{
			"tokenName": "Authorization", "tokenValue": "Bearer signed-jwt", "expiresIn": float64(7200),
		},
	})
}

func TestLoginValidationAndBusinessErrorsKeepLegacyContract(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       string
		serviceErr error
		status     int
		code       int
		message    string
		data       any
	}{
		{name: "fields", body: `{"username":" ","password":""}`, status: 400, code: 400, message: "参数错误", data: map[string]any{"username": "用户名不能为空", "password": "密码不能为空"}},
		{name: "bad credentials", body: `{"username":"admin","password":"wrong"}`, serviceErr: ErrInvalidCredentials, status: 200, code: 400, message: "用户名或密码错误", data: nil},
		{name: "disabled", body: `{"username":"admin","password":"wrong"}`, serviceErr: ErrUserDisabled, status: 200, code: 403, message: "用户已禁用", data: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &handlerServiceStub{loginErr: test.serviceErr}
			router := newAuthRouter(t, service, &authenticatorStub{})
			request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			var payload struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    any    `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != test.code || payload.Message != test.message {
				t.Fatalf("payload = %+v", payload)
			}
			if test.data == nil && payload.Data != nil {
				t.Fatalf("data = %#v, want nil", payload.Data)
			}
			if fields, ok := test.data.(map[string]any); ok {
				for key, expected := range fields {
					if payload.Data.(map[string]any)[key] != expected {
						t.Fatalf("field %q = %#v, want %#v", key, payload.Data.(map[string]any)[key], expected)
					}
				}
			}
		})
	}
}

func TestLoginRateLimitReturnsRetryAfterAnd429Envelope(t *testing.T) {
	service := &handlerServiceStub{loginErr: &LoginRateLimitError{RetryAfter: 2500 * time.Millisecond}}
	router := newAuthRouter(t, service, &authenticatorStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if response.Header().Get("Retry-After") != "3" {
		t.Fatalf("Retry-After = %q, want 3", response.Header().Get("Retry-After"))
	}
	assertJSON(t, response.Body.Bytes(), map[string]any{
		"code": float64(http.StatusTooManyRequests), "message": "请求过于频繁，请稍后再试", "data": nil,
	})
}

func TestProtectedRoutesRequireBearerAndExposePrincipal(t *testing.T) {
	principal := Principal{UserID: 42, JTI: "session", ExpiresAt: time.Now().Add(time.Hour)}
	service := &handlerServiceStub{
		currentUser: &CurrentUser{ID: 42, Username: "admin", Nickname: "管理员", Roles: []CurrentUserRole{}},
		menus:       []CurrentUserMenu{{ID: 1, ParentID: 0, MenuName: "系统管理", MenuType: "DIR", Path: "/system", Children: []CurrentUserMenu{}}},
	}
	authenticator := &authenticatorStub{principal: principal}
	router := newAuthRouter(t, service, authenticator)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status = %d", unauthorized.Code)
	}
	assertJSON(t, unauthorized.Body.Bytes(), map[string]any{"code": float64(401), "message": ErrUnauthenticated.Error(), "data": nil})
	if authenticator.token != "" {
		t.Fatalf("authenticator received malformed authorization %q", authenticator.token)
	}

	malformed := httptest.NewRecorder()
	malformedRequest := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	malformedRequest.Header.Set("Authorization", "signed-jwt")
	router.ServeHTTP(malformed, malformedRequest)
	if malformed.Code != http.StatusUnauthorized {
		t.Fatalf("non-Bearer status = %d", malformed.Code)
	}

	for _, endpoint := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/auth/me"},
		{method: http.MethodGet, path: "/api/auth/menus"},
		{method: http.MethodPost, path: "/api/auth/logout"},
	} {
		request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
		request.Header.Set("Authorization", "bearer signed-jwt")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d", endpoint.method, endpoint.path, response.Code)
		}
		if service.principal.UserID != principal.UserID || authenticator.token != "Bearer signed-jwt" {
			t.Fatalf("principal = %+v, authorization = %q", service.principal, authenticator.token)
		}
	}
}

func TestCurrentUserAndMenuResponsesUseFrontendJSONFields(t *testing.T) {
	externalURL := "https://not-returned.example"
	service := &handlerServiceStub{
		currentUser: &CurrentUser{
			ID:       3,
			Username: "admin",
			Nickname: "管理员",
			Dept:     &CurrentUserDept{ID: 4, DeptName: "研发", DeptCode: "RND"},
			Roles:    nil,
		},
		menus: []CurrentUserMenu{{
			ID: 5, ParentID: 0, MenuName: "系统", MenuType: "DIR", Path: "/system",
			ExternalURL: &externalURL, Children: nil,
		}},
	}
	router := newAuthRouter(t, service, &authenticatorStub{principal: Principal{UserID: 3, JTI: "jti"}})

	for _, path := range []string{"/api/auth/me", "/api/auth/menus"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "BEARER token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, response.Code)
		}

		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		var payload any
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			t.Fatal(err)
		}
		if path == "/api/auth/me" {
			user := payload.(map[string]any)
			if _, exists := user["last_login_time"]; exists || user["username"] != "admin" {
				t.Fatalf("user JSON = %#v", user)
			}
			if roles, ok := user["roles"].([]any); !ok || len(roles) != 0 {
				t.Fatalf("roles JSON = %#v", user["roles"])
			}
			continue
		}

		menu := payload.([]any)[0].(map[string]any)
		if _, exists := menu["externalUrl"]; exists {
			t.Fatalf("menu must not expose externalUrl: %#v", menu)
		}
		if children, ok := menu["children"].([]any); !ok || len(children) != 0 {
			t.Fatalf("children JSON = %#v", menu["children"])
		}
	}
}

func TestProtectedRouteMapsAuthenticationAndUnexpectedErrors(t *testing.T) {
	service := &handlerServiceStub{currentUserErr: ErrUnauthenticated}
	authenticator := &authenticatorStub{principal: Principal{UserID: 1, JTI: "jti"}}
	router := newAuthRouter(t, service, authenticator)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("authentication error status = %d", response.Code)
	}

	service.currentUserErr = errors.New("database unavailable")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected error status = %d", response.Code)
	}

	authenticator.err = platformerrors.ErrTemporarilyUnavailable
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("temporary authentication error status = %d", response.Code)
	}
	assertJSON(t, response.Body.Bytes(), map[string]any{
		"code": float64(http.StatusServiceUnavailable), "message": "service temporarily unavailable", "data": nil,
	})
}

func assertJSON(t *testing.T, bytes []byte, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(got, want) {
		t.Fatalf("JSON = %#v, want %#v", got, want)
	}
}

func jsonEqual(got, want any) bool {
	gotBytes, _ := json.Marshal(got)
	wantBytes, _ := json.Marshal(want)
	return string(gotBytes) == string(wantBytes)
}
