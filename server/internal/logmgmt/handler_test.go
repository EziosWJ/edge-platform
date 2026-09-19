package logmgmt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
)

func newHandlerRouter(t *testing.T, service HandlerService) *gin.Engine {
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

func newServiceRouter(t *testing.T, store Store, config *configStub) *gin.Engine {
	t.Helper()
	service, err := NewService(store, config)
	if err != nil {
		t.Fatal(err)
	}
	return newHandlerRouter(t, service)
}

func authenticated(request *http.Request) *http.Request {
	ctx := auth.ContextWithPrincipal(request.Context(), auth.Principal{UserID: 7, JTI: "jti"})
	return request.WithContext(ctx)
}

func getBody(t *testing.T, router http.Handler, method, path string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, authenticated(request))
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s %s response: %v body=%s", method, path, err, response.Body.String())
	}
	return response.Code, payload
}

func TestLoginLogPageValidatesPagination(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		path      string
		wantCode  float64
		wantField string
	}{
		{name: "page zero", path: "/api/system/login-log/page?page=0", wantCode: 400, wantField: "page"},
		{name: "page not a number", path: "/api/system/login-log/page?page=abc", wantCode: 400, wantField: "page"},
		{name: "pageSize zero", path: "/api/system/login-log/page?pageSize=0", wantCode: 400, wantField: "pageSize"},
		{name: "pageSize too large", path: "/api/system/login-log/page?pageSize=501", wantCode: 400, wantField: "pageSize"},
		{name: "pageSize not a number", path: "/api/system/login-log/page?pageSize=abc", wantCode: 400, wantField: "pageSize"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := newServiceRouter(t, newMemoryStore(), nil)
			status, payload := getBody(t, router, http.MethodGet, test.path)
			if status != http.StatusBadRequest || payload["code"].(float64) != 400 {
				t.Fatalf("status=%d payload=%v", status, payload)
			}
			fields := payload["data"].(map[string]any)
			if _, ok := fields[test.wantField]; !ok {
				t.Fatalf("fields=%v want %q", fields, test.wantField)
			}
		})
	}
}

func TestOperLogPageValidatesPagination(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		path      string
		wantField string
	}{
		{name: "page zero", path: "/api/system/oper-log/page?page=0", wantField: "page"},
		{name: "pageSize too large", path: "/api/system/oper-log/page?pageSize=501", wantField: "pageSize"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := newServiceRouter(t, newMemoryStore(), nil)
			status, payload := getBody(t, router, http.MethodGet, test.path)
			if status != http.StatusBadRequest || payload["code"].(float64) != 400 {
				t.Fatalf("status=%d payload=%v", status, payload)
			}
			fields := payload["data"].(map[string]any)
			if _, ok := fields[test.wantField]; !ok {
				t.Fatalf("fields=%v want %q", fields, test.wantField)
			}
		})
	}
}

func TestLoginLogPageServesRecords(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	store.loginLogs = append(store.loginLogs, LoginLog{ID: 2, Username: "admin", LoginStatus: "SUCCESS"})
	store.loginLogs = append(store.loginLogs, LoginLog{ID: 1, Username: "nobody", LoginStatus: "FAIL"})
	router := newServiceRouter(t, store, nil)
	status, payload := getBody(t, router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=10")
	if status != http.StatusOK || payload["code"].(float64) != 200 {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
	data := payload["data"].(map[string]any)
	if data["total"].(float64) != 2 || len(data["records"].([]any)) != 2 {
		t.Fatalf("data=%v", data)
	}
}

func TestLoginLogDetailNotFoundMapsToBusinessEnvelope(t *testing.T) {
	t.Parallel()
	router := newServiceRouter(t, newMemoryStore(), nil)
	status, payload := getBody(t, router, http.MethodGet, "/api/system/login-log/99")
	if status != http.StatusOK || payload["code"].(float64) != platformhttp.CodeNotFound || payload["message"] != ErrNotFound.Error() {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
}

func TestLoginLogDetailInvalidID(t *testing.T) {
	t.Parallel()
	router := newServiceRouter(t, newMemoryStore(), nil)
	status, payload := getBody(t, router, http.MethodGet, "/api/system/login-log/abc")
	if status != http.StatusBadRequest || payload["code"].(float64) != platformhttp.CodeBadRequest {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
}

func TestOperLogDetailMapsNotFoundAndServesPayloadColumns(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	store.operLogDetail = &OperLogDetail{OperLogRecord: OperLogRecord{ID: 3, ModuleName: "role"}, RequestParams: `{"a":1}`, ResponseResult: "ok"}
	router := newServiceRouter(t, store, nil)
	status, payload := getBody(t, router, http.MethodGet, "/api/system/oper-log/3")
	if status != http.StatusOK || payload["code"].(float64) != 200 {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
	data := payload["data"].(map[string]any)
	if data["moduleName"] != "role" || data["requestParams"] != `{"a":1}` {
		t.Fatalf("data=%v", data)
	}
	status, payload = getBody(t, router, http.MethodGet, "/api/system/oper-log/99")
	if status != http.StatusOK || payload["code"].(float64) != platformhttp.CodeNotFound {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
}

func TestClearLoginLogsGatedByConfigSwitch(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	router := newServiceRouter(t, store, &configStub{value: "false"})
	status, payload := getBody(t, router, http.MethodDelete, "/api/system/login-log/clear")
	if status != http.StatusOK || payload["code"].(float64) != platformhttp.CodeForbidden || payload["message"] != ErrForbidden.Error() {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
	if store.cleared {
		t.Fatal("clear must not run when the switch is off")
	}
}

func TestClearOperLogsGatedByConfigSwitch(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	router := newServiceRouter(t, store, &configStub{value: "false"})
	status, payload := getBody(t, router, http.MethodDelete, "/api/system/oper-log/clear")
	if status != http.StatusOK || payload["code"].(float64) != platformhttp.CodeForbidden || payload["message"] != ErrForbidden.Error() {
		t.Fatalf("status=%d payload=%v", status, payload)
	}
	if store.cleared {
		t.Fatal("clear must not run when the switch is off")
	}
}

func TestClearLoginLogsSucceedsAndAudits(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	service, err := NewService(store, &configStub{value: "true"})
	if err != nil {
		t.Fatal(err)
	}
	router := newHandlerRouter(t, service)
	request := httptest.NewRequest(http.MethodDelete, "/api/system/login-log/clear", nil)
	request.Header.Set(platformhttp.RequestIDHeader, "request-42")
	request.Header.Set("User-Agent", "handler-test")
	request.RemoteAddr = "203.0.113.9:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, authenticated(request))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !store.cleared || len(store.events) != 1 {
		t.Fatalf("cleared=%v audit=%d", store.cleared, len(store.events))
	}
	event := store.events[0]
	if event.Action != "login-log.clear" || event.Metadata.ActorID != 7 || event.Metadata.RequestID != "request-42" || event.Metadata.ClientIP != "203.0.113.9" || event.Metadata.RequestMethod != http.MethodDelete || event.Metadata.RequestURL != "/api/system/login-log/clear" {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestClearOperLogsSucceedsAndAudits(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	service, err := NewService(store, &configStub{value: "true"})
	if err != nil {
		t.Fatal(err)
	}
	router := newHandlerRouter(t, service)
	request := httptest.NewRequest(http.MethodDelete, "/api/system/oper-log/clear", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, authenticated(request))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !store.cleared || len(store.events) != 1 || store.events[0].Action != "oper-log.clear" {
		t.Fatalf("cleared=%v audit=%d events=%+v", store.cleared, len(store.events), store.events)
	}
}

func TestClearWithoutConfigIsForbidden(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	router := newServiceRouter(t, store, nil)
	status, payload := getBody(t, router, http.MethodDelete, "/api/system/oper-log/clear")
	if status != http.StatusOK || payload["code"].(float64) != platformhttp.CodeForbidden || store.cleared {
		t.Fatalf("status=%d payload=%v cleared=%v", status, payload, store.cleared)
	}
}

func TestClearRejectsInvalidConfigAndReadErrors(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		config     *configStub
		wantStatus int
		wantCode   float64
	}{
		{name: "missing config", config: &configStub{err: sysconfig.ErrNotFound}, wantStatus: http.StatusOK, wantCode: platformhttp.CodeForbidden},
		{name: "wrong type", config: &configStub{value: "true", valueType: "TEXT"}, wantStatus: http.StatusOK, wantCode: platformhttp.CodeForbidden},
		{name: "invalid value", config: &configStub{value: "yes"}, wantStatus: http.StatusOK, wantCode: platformhttp.CodeForbidden},
		{name: "read error", config: &configStub{err: errors.New("config read failed")}, wantStatus: http.StatusInternalServerError, wantCode: platformhttp.CodeInternalError},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore()
			router := newServiceRouter(t, store, test.config)
			status, payload := getBody(t, router, http.MethodDelete, "/api/system/oper-log/clear")
			if status != test.wantStatus || payload["code"].(float64) != test.wantCode || store.cleared {
				t.Fatalf("status=%d payload=%v cleared=%v", status, payload, store.cleared)
			}
		})
	}
}

func TestClearRollsBackWhenAuditFails(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/api/system/login-log/clear", "/api/system/oper-log/clear"} {
		store := newMemoryStore()
		store.auditError = errors.New("audit write failed")
		router := newServiceRouter(t, store, &configStub{value: "true"})
		status, payload := getBody(t, router, http.MethodDelete, path)
		if status != http.StatusInternalServerError {
			t.Fatalf("%s status=%d payload=%v", path, status, payload)
		}
		if store.cleared {
			t.Fatalf("%s logs must roll back when audit fails", path)
		}
	}
}

func TestRoutesRegistered(t *testing.T) {
	t.Parallel()
	router := newServiceRouter(t, newMemoryStore(), nil)
	want := map[string]string{
		"GET /api/system/login-log/page":     "",
		"GET /api/system/login-log/:id":      "",
		"DELETE /api/system/login-log/clear": "",
		"GET /api/system/oper-log/page":      "",
		"GET /api/system/oper-log/:id":       "",
		"DELETE /api/system/oper-log/clear":  "",
	}
	for _, route := range router.Routes() {
		delete(want, route.Method+" "+route.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing routes: %v", want)
	}
}

type memoryStore struct {
	loginLogs     []LoginLog
	operLogs      []OperLogRecord
	operLogDetail *OperLogDetail
	cleared       bool
	queries       []string
	events        []audit.Event
	auditError    error
}

func newMemoryStore() *memoryStore { return &memoryStore{} }

func (s *memoryStore) LoginLogPage(_ context.Context, q LoginLogPageQuery) (Page[LoginLog], error) {
	s.queries = append(s.queries, "login:"+q.Username+":"+q.LoginStatus+":"+q.LoginIP)
	records := make([]LoginLog, 0, len(s.loginLogs))
	for _, log := range s.loginLogs {
		if q.Username != "" && !strings.Contains(log.Username, q.Username) {
			continue
		}
		if q.LoginStatus != "" && log.LoginStatus != q.LoginStatus {
			continue
		}
		if q.LoginIP != "" && !strings.Contains(log.LoginIP, q.LoginIP) {
			continue
		}
		records = append(records, log)
	}
	return Page[LoginLog]{Records: records, Total: int64(len(records)), Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *memoryStore) FindLoginLog(_ context.Context, id int64) (*LoginLog, error) {
	for i := range s.loginLogs {
		if s.loginLogs[i].ID == id {
			copy := s.loginLogs[i]
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

func (s *memoryStore) ClearLoginLogs(_ context.Context, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	s.cleared = true
	s.loginLogs = nil
	s.events = append(s.events, e)
	return nil
}

func (s *memoryStore) OperLogPage(_ context.Context, q OperLogPageQuery) (Page[OperLogRecord], error) {
	s.queries = append(s.queries, "oper:"+q.ModuleName+":"+q.OperationType+":"+q.OperatorName+":"+q.OperationStatus)
	records := make([]OperLogRecord, 0, len(s.operLogs))
	for _, log := range s.operLogs {
		if q.ModuleName != "" && !strings.Contains(log.ModuleName, q.ModuleName) {
			continue
		}
		if q.OperationType != "" && log.OperationType != q.OperationType {
			continue
		}
		if q.OperatorName != "" && !strings.Contains(log.OperatorName, q.OperatorName) {
			continue
		}
		if q.OperationStatus != "" && log.OperationStatus != q.OperationStatus {
			continue
		}
		records = append(records, log)
	}
	return Page[OperLogRecord]{Records: records, Total: int64(len(records)), Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *memoryStore) FindOperLog(_ context.Context, id int64) (*OperLogDetail, error) {
	if s.operLogDetail != nil && s.operLogDetail.ID == id {
		copy := *s.operLogDetail
		return &copy, nil
	}
	return nil, ErrNotFound
}

func (s *memoryStore) ClearOperLogs(_ context.Context, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	s.cleared = true
	s.operLogs = nil
	s.events = append(s.events, e)
	return nil
}

type configStub struct {
	value     string
	valueType string
	err       error
}

func (c *configStub) GetByKey(context.Context, string) (*sysconfig.ByKey, error) {
	if c == nil {
		return nil, sysconfig.ErrNotFound
	}
	if c.err != nil {
		return nil, c.err
	}
	valueType := c.valueType
	if valueType == "" {
		valueType = "BOOLEAN"
	}
	return &sysconfig.ByKey{ConfigValue: c.value, ValueType: valueType}, nil
}
