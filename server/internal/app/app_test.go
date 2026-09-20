package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/EziosWJ/edge-platform/server/internal/dept"
	"github.com/EziosWJ/edge-platform/server/internal/dictionary"
	"github.com/EziosWJ/edge-platform/server/internal/filemgmt"
	"github.com/EziosWJ/edge-platform/server/internal/logmgmt"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/EziosWJ/edge-platform/server/internal/rbac"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
	"github.com/EziosWJ/edge-platform/server/internal/usermgmt"
)

type readyProbe struct {
	err error
}

func (p readyProbe) Ready(context.Context) error {
	return p.err
}

type mqttRuntimeProbe struct {
	readyErr error
	startErr error
	stopErr  error
	starts   int
	stops    int
	status   platformhttp.MQTTStatus
}

func (p *mqttRuntimeProbe) Start(context.Context) error {
	p.starts++
	return p.startErr
}

func (p *mqttRuntimeProbe) Stop(context.Context) error {
	p.stops++
	return p.stopErr
}

func (p *mqttRuntimeProbe) Ready(context.Context) error { return p.readyErr }

func (p *mqttRuntimeProbe) Status() platformhttp.MQTTStatus { return p.status }

func testConfig(environment string, swaggerEnabled bool) config.Config {
	return config.Config{
		Environment: environment,
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"*"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Swagger: config.SwaggerConfig{Enabled: swaggerEnabled},
		HTTP: config.HTTPConfig{
			ShutdownTimeout: time.Second,
		},
	}
}

// fakeStores provide enough in-memory state to construct every real business
// service without a database.
type fakeStores struct {
	auth auth.Store
	file filemgmt.Storage
}

func newFakeStores() *fakeStores {
	return &fakeStores{
		auth: &authMemoryStore{users: map[string]auth.User{}, sessions: map[string]auth.AuthSession{}},
		file: memoryStorage{},
	}
}

func (f *fakeStores) deps() Dependencies {
	authService, err := auth.NewService(f.auth, &auth.TokenManager{})
	if err != nil {
		panic(err)
	}
	rbacService, err := rbac.NewService(emptyRBACStore{})
	if err != nil {
		panic(err)
	}
	deptService, err := dept.NewService(emptyDeptStore{})
	if err != nil {
		panic(err)
	}
	userService, err := usermgmt.NewService(emptyUserStore{}, "admin123")
	if err != nil {
		panic(err)
	}
	dictionaryService, err := dictionary.NewService(emptyDictionaryStore{})
	if err != nil {
		panic(err)
	}
	configService := sysconfig.NewService(emptyConfigStore{})
	fileService, err := filemgmt.NewService(emptyFileStore{}, f.file)
	if err != nil {
		panic(err)
	}
	logService, err := logmgmt.NewService(emptyLogStore{}, configService)
	if err != nil {
		panic(err)
	}
	return Dependencies{
		Auth:       authService,
		RBAC:       rbacService,
		Department: deptService,
		User:       userService,
		Dictionary: dictionaryService,
		SysConfig:  configService,
		File:       fileService,
		Log:        logService,
	}
}

func TestBuildRegistersSystemRoutes(t *testing.T) {
	router, err := Build(testConfig("test", false), readyProbe{}, newFakeStores().deps())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}
}

func TestMQTTReadinessIsOptionalWhenDisabled(t *testing.T) {
	runtime := &mqttRuntimeProbe{readyErr: context.Canceled}
	cfg := testConfig("test", false)
	cfg.MQTT.Protocol = config.MQTTProtocol5
	runtime.status = platformhttp.MQTTStatus{Enabled: true, State: platformhttp.MQTTStateStopped}
	deps := newFakeStores().deps()
	deps.MQTT = runtime

	router, err := Build(cfg, readyProbe{}, deps)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("disabled MQTT readiness status = %d, want %d", ready.Code, http.StatusOK)
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/system/mqtt/status", nil))
	if status.Code != http.StatusUnauthorized {
		t.Fatalf("MQTT status without login = %d, want %d", status.Code, http.StatusUnauthorized)
	}
}

func TestMQTTReadinessFailsWhenEnabledRuntimeIsNotReady(t *testing.T) {
	runtime := &mqttRuntimeProbe{readyErr: context.Canceled}
	cfg := testConfig("test", false)
	cfg.MQTT.Enabled = true
	cfg.MQTT.Protocol = config.MQTTProtocol5
	deps := newFakeStores().deps()
	deps.MQTT = runtime

	application, err := New(cfg, readyProbe{}, deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := application.StartRuntime(context.Background()); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if err := application.StopRuntime(context.Background()); err != nil {
		t.Fatalf("StopRuntime() error = %v", err)
	}
	if runtime.starts != 1 || runtime.stops != 1 {
		t.Fatalf("runtime lifecycle calls = (%d, %d), want (1, 1)", runtime.starts, runtime.stops)
	}

	ready := httptest.NewRecorder()
	application.Router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("enabled MQTT unready status = %d, want %d", ready.Code, http.StatusServiceUnavailable)
	}
	health := httptest.NewRecorder()
	application.Router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}
}

func TestBuildRegistersAllSystemManagementRoutes(t *testing.T) {
	router, err := Build(testConfig("test", false), readyProbe{}, newFakeStores().deps())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Every module route must exist behind bearer auth; an unauthenticated
	// request proves the route is registered without exercising handlers.
	for _, path := range []string{
		"/api/system/role/page",
		"/api/system/dept/tree",
		"/api/system/user/page",
		"/api/system/dict-type/page",
		"/api/system/config/page",
		"/api/system/file/page",
		"/api/system/login-log/page",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want %d (route must be registered)", path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestBuildAuthenticatesMultipartRequestsBeforeBodyPolicy(t *testing.T) {
	router, err := Build(testConfig("test", false), readyProbe{}, newFakeStores().deps())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/system/file/upload", strings.NewReader("oversized or malformed body"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("multipart unauthenticated status = %d body=%s, want %d", response.Code, response.Body.String(), http.StatusUnauthorized)
	}
}

func TestNewFailsWhenRequiredServiceMissing(t *testing.T) {
	full := newFakeStores().deps()
	for _, test := range []struct {
		name   string
		mutate func(*Dependencies)
		want   string
	}{
		{name: "auth", mutate: func(d *Dependencies) { d.Auth = nil }, want: "auth service is required"},
		{name: "rbac", mutate: func(d *Dependencies) { d.RBAC = nil }, want: "rbac service is required"},
		{name: "department", mutate: func(d *Dependencies) { d.Department = nil }, want: "department service is required"},
		{name: "user", mutate: func(d *Dependencies) { d.User = nil }, want: "user service is required"},
		{name: "dictionary", mutate: func(d *Dependencies) { d.Dictionary = nil }, want: "dictionary service is required"},
		{name: "sysconfig", mutate: func(d *Dependencies) { d.SysConfig = nil }, want: "sysconfig service is required"},
		{name: "file", mutate: func(d *Dependencies) { d.File = nil }, want: "file service is required"},
		{name: "log", mutate: func(d *Dependencies) { d.Log = nil }, want: "log service is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps := full
			test.mutate(&deps)
			_, err := New(testConfig("test", false), readyProbe{}, deps)
			if err == nil {
				t.Fatalf("New() error = nil, want %q", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("New() error = %q, want it to contain %q", err.Error(), test.want)
			}
		})
	}
}

func TestNewRejectsInvalidTrustedProxy(t *testing.T) {
	cfg := testConfig("test", false)
	cfg.HTTP.TrustedProxies = []string{"not-a-proxy"}

	_, err := New(cfg, readyProbe{}, newFakeStores().deps())
	if err == nil || !strings.Contains(err.Error(), "configure trusted proxies") {
		t.Fatalf("New() error = %v, want trusted proxy configuration error", err)
	}
}

func TestBuildRegistersSwaggerOnlyInDev(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment string
		enabled     bool
		wantStatus  int
	}{
		{name: "development enabled", environment: "dev", enabled: true, wantStatus: http.StatusOK},
		{name: "test enabled", environment: "test", enabled: true, wantStatus: http.StatusNotFound},
		{name: "development disabled", environment: "dev", enabled: false, wantStatus: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := Build(testConfig(test.environment, test.enabled), readyProbe{}, newFakeStores().deps())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
			if response.Code != test.wantStatus {
				t.Errorf("GET /swagger/index.html status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

// --- in-memory stores that satisfy the module Store interfaces ---

type authMemoryStore struct {
	users    map[string]auth.User
	sessions map[string]auth.AuthSession
}

func (s *authMemoryStore) FindUserByUsername(_ context.Context, username string) (*auth.User, error) {
	u, ok := s.users[username]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return &u, nil
}

func (s *authMemoryStore) RecordLoginFailure(context.Context, auth.LoginLog) error { return nil }
func (s *authMemoryStore) CompleteLogin(_ context.Context, userID int64, loginTime time.Time, loginIP string, session auth.AuthSession, log auth.LoginLog) error {
	s.sessions[session.JTI] = session
	return nil
}
func (s *authMemoryStore) IsSessionActive(context.Context, int64, string, time.Time) (bool, error) {
	return true, nil
}
func (s *authMemoryStore) RevokeSession(context.Context, int64, string, time.Time) error { return nil }
func (s *authMemoryStore) RevokeSessionsByUserID(context.Context, int64, time.Time) error {
	return nil
}
func (s *authMemoryStore) FindCurrentUser(context.Context, int64) (*auth.CurrentUser, error) {
	return nil, auth.ErrUserNotFound
}
func (s *authMemoryStore) FindVisibleMenusByUserID(context.Context, int64) ([]auth.CurrentUserMenu, error) {
	return nil, nil
}

type emptyRBACStore struct{}

func (emptyRBACStore) PageRoles(context.Context, rbac.RolePageQuery) (rbac.Page[rbac.Role], error) {
	return rbac.Page[rbac.Role]{}, nil
}
func (emptyRBACStore) FindRole(context.Context, int64) (*rbac.Role, error) {
	return nil, nil
}
func (emptyRBACStore) RoleCodeExists(context.Context, string, int64) (bool, error) { return false, nil }
func (emptyRBACStore) CreateRole(context.Context, rbac.Role, rbac.AuditEvent) (rbac.Role, error) {
	return rbac.Role{}, nil
}
func (emptyRBACStore) UpdateRole(context.Context, rbac.Role, rbac.AuditEvent) (rbac.Role, error) {
	return rbac.Role{}, nil
}
func (emptyRBACStore) CountUsersByRole(context.Context, int64) (int64, error)           { return 0, nil }
func (emptyRBACStore) DeleteRole(context.Context, int64, rbac.AuditEvent) error         { return nil }
func (emptyRBACStore) DeleteRoles(context.Context, []int64, rbac.AuditEvent) error      { return nil }
func (emptyRBACStore) SetRoleStatus(context.Context, int64, int, rbac.AuditEvent) error { return nil }
func (emptyRBACStore) RoleMenuIDs(context.Context, int64) ([]int64, error)              { return nil, nil }
func (emptyRBACStore) ReplaceRoleMenus(context.Context, int64, []int64, rbac.AuditEvent) error {
	return nil
}
func (emptyRBACStore) EnabledRoles(context.Context) ([]rbac.Role, error) { return nil, nil }
func (emptyRBACStore) ListMenus(context.Context) ([]rbac.Menu, error)    { return nil, nil }
func (emptyRBACStore) PageMenus(context.Context, rbac.MenuPageQuery) (rbac.Page[rbac.Menu], error) {
	return rbac.Page[rbac.Menu]{}, nil
}
func (emptyRBACStore) FindMenu(context.Context, int64) (*rbac.Menu, error) { return nil, nil }
func (emptyRBACStore) PermissionCodeExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (emptyRBACStore) CreateMenu(context.Context, rbac.Menu, rbac.AuditEvent) (rbac.Menu, error) {
	return rbac.Menu{}, nil
}
func (emptyRBACStore) UpdateMenu(context.Context, rbac.Menu, rbac.AuditEvent) (rbac.Menu, error) {
	return rbac.Menu{}, nil
}
func (emptyRBACStore) CountChildren(context.Context, int64) (int64, error)              { return 0, nil }
func (emptyRBACStore) CountRolesByMenu(context.Context, int64) (int64, error)           { return 0, nil }
func (emptyRBACStore) DeleteMenu(context.Context, int64, rbac.AuditEvent) error         { return nil }
func (emptyRBACStore) DeleteMenus(context.Context, []int64, rbac.AuditEvent) error      { return nil }
func (emptyRBACStore) SetMenuStatus(context.Context, int64, int, rbac.AuditEvent) error { return nil }

type emptyDeptStore struct{}

func (emptyDeptStore) List(context.Context) ([]dept.Dept, error)               { return nil, nil }
func (emptyDeptStore) Page(context.Context, dept.Query) (dept.Page, error)     { return dept.Page{}, nil }
func (emptyDeptStore) Find(context.Context, int64) (*dept.Dept, error)         { return nil, nil }
func (emptyDeptStore) CodeExists(context.Context, string, int64) (bool, error) { return false, nil }
func (emptyDeptStore) Create(context.Context, dept.Dept, dept.AuditEvent) (dept.Dept, error) {
	return dept.Dept{}, nil
}
func (emptyDeptStore) Update(context.Context, dept.Dept, dept.AuditEvent) (dept.Dept, error) {
	return dept.Dept{}, nil
}
func (emptyDeptStore) Delete(context.Context, int64, dept.AuditEvent) error         { return nil }
func (emptyDeptStore) DeleteBatch(context.Context, []int64, dept.AuditEvent) error  { return nil }
func (emptyDeptStore) SetStatus(context.Context, int64, int, dept.AuditEvent) error { return nil }
func (emptyDeptStore) CountChildren(context.Context, int64) (int64, error)          { return 0, nil }
func (emptyDeptStore) CountUsers(context.Context, int64) (int64, error)             { return 0, nil }

type emptyUserStore struct{}

func (emptyUserStore) Page(context.Context, usermgmt.PageQuery) (usermgmt.Page[usermgmt.User], error) {
	return usermgmt.Page[usermgmt.User]{}, nil
}
func (emptyUserStore) Find(context.Context, int64) (*usermgmt.User, error) { return nil, nil }
func (emptyUserStore) UsernameExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (emptyUserStore) DeptExists(context.Context, int64) (bool, error)   { return false, nil }
func (emptyUserStore) RolesExist(context.Context, []int64) (bool, error) { return false, nil }
func (emptyUserStore) Create(context.Context, usermgmt.User, usermgmt.AuditEvent) (usermgmt.User, error) {
	return usermgmt.User{}, nil
}
func (emptyUserStore) Update(context.Context, usermgmt.User, bool, usermgmt.AuditEvent) error {
	return nil
}
func (emptyUserStore) Delete(context.Context, int64, usermgmt.AuditEvent) error { return nil }
func (emptyUserStore) DeleteUsers(context.Context, []int64, usermgmt.AuditEvent) error {
	return nil
}
func (emptyUserStore) AssignRoles(context.Context, int64, []int64, usermgmt.AuditEvent) error {
	return nil
}
func (emptyUserStore) ResetPassword(context.Context, int64, string, usermgmt.AuditEvent) error {
	return nil
}
func (emptyUserStore) ChangePassword(context.Context, int64, string, usermgmt.AuditEvent) error {
	return nil
}
func (emptyUserStore) UpdateAvatar(context.Context, int64, *string, usermgmt.AuditEvent) error {
	return nil
}

type emptyDictionaryStore struct{}

func (emptyDictionaryStore) PageTypes(context.Context, dictionary.TypePageQuery) (dictionary.Page[dictionary.DictType], error) {
	return dictionary.Page[dictionary.DictType]{}, nil
}
func (emptyDictionaryStore) FindType(context.Context, int64) (*dictionary.DictType, error) {
	return nil, nil
}
func (emptyDictionaryStore) DictCodeExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (emptyDictionaryStore) CreateType(context.Context, dictionary.DictType, dictionary.AuditEvent) (dictionary.DictType, error) {
	return dictionary.DictType{}, nil
}
func (emptyDictionaryStore) UpdateType(context.Context, dictionary.DictType, dictionary.AuditEvent) (dictionary.DictType, error) {
	return dictionary.DictType{}, nil
}
func (emptyDictionaryStore) CountDataByType(context.Context, int64) (int64, error) { return 0, nil }
func (emptyDictionaryStore) DeleteTypes(context.Context, []int64, dictionary.AuditEvent) error {
	return nil
}
func (emptyDictionaryStore) SetTypeStatus(context.Context, int64, int, dictionary.AuditEvent) error {
	return nil
}
func (emptyDictionaryStore) PageData(context.Context, dictionary.DataPageQuery) (dictionary.Page[dictionary.DictData], error) {
	return dictionary.Page[dictionary.DictData]{}, nil
}
func (emptyDictionaryStore) FindData(context.Context, int64) (*dictionary.DictData, error) {
	return nil, nil
}
func (emptyDictionaryStore) DictValueExists(context.Context, int64, string, int64) (bool, error) {
	return false, nil
}
func (emptyDictionaryStore) CreateData(context.Context, dictionary.DictData, dictionary.AuditEvent) (dictionary.DictData, error) {
	return dictionary.DictData{}, nil
}
func (emptyDictionaryStore) UpdateData(context.Context, dictionary.DictData, dictionary.AuditEvent) (dictionary.DictData, error) {
	return dictionary.DictData{}, nil
}
func (emptyDictionaryStore) DeleteData(context.Context, []int64, dictionary.AuditEvent) error {
	return nil
}
func (emptyDictionaryStore) Items(context.Context, string) ([]dictionary.DictItem, error) {
	return nil, nil
}

type emptyConfigStore struct{}

func (emptyConfigStore) Page(context.Context, sysconfig.Query) (sysconfig.Page, error) {
	return sysconfig.Page{}, nil
}
func (emptyConfigStore) Find(context.Context, int64) (*sysconfig.Config, error) { return nil, nil }
func (emptyConfigStore) ByKey(context.Context, string) (*sysconfig.ByKey, error) {
	return nil, nil
}
func (emptyConfigStore) KeyExists(context.Context, string, int64) (bool, error) { return false, nil }
func (emptyConfigStore) Create(context.Context, sysconfig.Config, audit.Event) (sysconfig.Config, error) {
	return sysconfig.Config{}, nil
}
func (emptyConfigStore) Update(context.Context, sysconfig.Config, audit.Event) error { return nil }
func (emptyConfigStore) Delete(context.Context, int64, audit.Event) error            { return nil }
func (emptyConfigStore) DeleteBatch(context.Context, []int64, audit.Event) error     { return nil }
func (emptyConfigStore) SetStatus(context.Context, int64, int, audit.Event) error    { return nil }

type emptyFileStore struct{}

func (emptyFileStore) Page(context.Context, filemgmt.FilePageQuery) (filemgmt.Page[filemgmt.File], error) {
	return filemgmt.Page[filemgmt.File]{}, nil
}
func (emptyFileStore) Find(context.Context, int64) (*filemgmt.File, error) { return nil, nil }
func (emptyFileStore) Create(context.Context, filemgmt.File, filemgmt.AuditEvent) (filemgmt.File, error) {
	return filemgmt.File{}, nil
}
func (emptyFileStore) Update(context.Context, int64, filemgmt.UpdateInput, filemgmt.AuditEvent) error {
	return nil
}
func (emptyFileStore) Delete(context.Context, int64, filemgmt.AuditEvent) error { return nil }
func (emptyFileStore) DeleteBatch(context.Context, []int64, filemgmt.AuditEvent) error {
	return nil
}
func (emptyFileStore) SetStatus(context.Context, int64, int, filemgmt.AuditEvent) error { return nil }

type emptyLogStore struct{}

func (emptyLogStore) LoginLogPage(context.Context, logmgmt.LoginLogPageQuery) (logmgmt.Page[logmgmt.LoginLog], error) {
	return logmgmt.Page[logmgmt.LoginLog]{}, nil
}
func (emptyLogStore) FindLoginLog(context.Context, int64) (*logmgmt.LoginLog, error) {
	return nil, nil
}
func (emptyLogStore) ClearLoginLogs(context.Context, audit.Event) error { return nil }
func (emptyLogStore) OperLogPage(context.Context, logmgmt.OperLogPageQuery) (logmgmt.Page[logmgmt.OperLogRecord], error) {
	return logmgmt.Page[logmgmt.OperLogRecord]{}, nil
}
func (emptyLogStore) FindOperLog(context.Context, int64) (*logmgmt.OperLogDetail, error) {
	return nil, nil
}
func (emptyLogStore) ClearOperLogs(context.Context, audit.Event) error { return nil }

type memoryStorage struct{}

func (memoryStorage) Save(context.Context, string, io.Reader) (filemgmt.StoredFile, error) {
	return filemgmt.StoredFile{}, nil
}
func (memoryStorage) Open(context.Context, string) (io.ReadSeekCloser, error) {
	return nil, filemgmt.ErrNotFound
}
func (memoryStorage) Remove(context.Context, string) error { return nil }
