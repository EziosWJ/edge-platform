//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/app"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/EziosWJ/edge-platform/server/internal/dept"
	"github.com/EziosWJ/edge-platform/server/internal/dictionary"
	"github.com/EziosWJ/edge-platform/server/internal/filemgmt"
	"github.com/EziosWJ/edge-platform/server/internal/logmgmt"
	"github.com/EziosWJ/edge-platform/server/internal/notification"
	platformdatabase "github.com/EziosWJ/edge-platform/server/internal/platform/database"
	"github.com/EziosWJ/edge-platform/server/internal/rbac"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
	"github.com/EziosWJ/edge-platform/server/internal/usermgmt"
)

const postgresImage = "postgres:17-alpine"

func TestPostgresMigrationsUseEphemeralDatabase(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	root := projectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "cmd", "migrate")); errors.Is(err, os.ErrNotExist) {
		t.Skip("cmd/migrate is not available yet")
	} else if err != nil {
		t.Fatalf("inspect cmd/migrate: %v", err)
	}

	database := startPostgres(t)
	runMigrations(t, root, database.dsn)
	database.assertGooseSchemaVersionTable(t)
	verifyDatabaseReadiness(t, database.dsn)
}

func TestPostgresLogClearSeedDefaultsByEnvironment(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	for _, test := range []struct {
		environment string
		want        string
	}{
		{environment: config.EnvironmentDev, want: "true"},
		{environment: config.EnvironmentTest, want: "false"},
		{environment: config.EnvironmentProd, want: "false"},
	} {
		t.Run(test.environment, func(t *testing.T) {
			database := startPostgres(t)
			runMigrationsWithEnvironment(t, projectRoot(t), database.dsn, test.environment)
			connection := openTemporaryDatabase(t, database.dsn)
			defer func() { _ = connection.Close() }()

			var value struct {
				ConfigValue string `gorm:"column:config_value"`
			}
			if err := connection.GORM.Table("sys_config").Select("config_value").Where("config_key=?", sysconfig.LogClearEnabledKey).Take(&value).Error; err != nil {
				t.Fatalf("load log-clear seed: %v", err)
			}
			if value.ConfigValue != test.want {
				t.Fatalf("log-clear seed for %s = %q, want %q", test.environment, value.ConfigValue, test.want)
			}
		})
	}
}

func TestAuthContractUsesPostgresSessions(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build authentication API: %v", err)
	}

	failedLogin := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	assertEnvelopeCode(t, failedLogin, http.StatusOK, 400, "用户名或密码错误")

	login := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"admin123"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", login.Code, login.Body.String())
	}
	var loginResponse struct {
		Code int `json:"code"`
		Data struct {
			TokenValue string `json:"tokenValue"`
			ExpiresIn  int64  `json:"expiresIn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginResponse); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResponse.Code != 200 || !strings.HasPrefix(loginResponse.Data.TokenValue, "Bearer ") || loginResponse.Data.ExpiresIn != 7200 {
		t.Fatalf("login response = %+v", loginResponse)
	}

	me := serveJSON(router, http.MethodGet, "/api/auth/me", "", loginResponse.Data.TokenValue)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"username":"admin"`) || !strings.Contains(me.Body.String(), `"roleCode":"ADMIN"`) {
		t.Fatalf("current user response = status %d body=%s", me.Code, me.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", loginResponse.Data.TokenValue)
	if menus.Code != http.StatusOK || !strings.Contains(menus.Body.String(), `"menuName":"系统管理"`) || !strings.Contains(menus.Body.String(), `"children"`) {
		t.Fatalf("current menu response = status %d body=%s", menus.Code, menus.Body.String())
	}

	logout := serveJSON(router, http.MethodPost, "/api/auth/logout", "", loginResponse.Data.TokenValue)
	assertEnvelopeCode(t, logout, http.StatusOK, 200, "success")
	revoked := serveJSON(router, http.MethodGet, "/api/auth/me", "", loginResponse.Data.TokenValue)
	assertEnvelopeCode(t, revoked, http.StatusUnauthorized, 401, auth.ErrUnauthenticated.Error())

	var loginLogCount int64
	if err := database.GORM.Table("sys_login_log").Count(&loginLogCount).Error; err != nil {
		t.Fatalf("count login logs: %v", err)
	}
	if loginLogCount != 2 {
		t.Fatalf("login log count = %d, want 2 (one failed and one successful login)", loginLogCount)
	}
}

func TestRBACContractWritesAuditLog(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build RBAC API: %v", err)
	}
	token := loginAdmin(t, router)

	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"运营","roleCode":"OPS","status":1,"sortOrder":2}`, token)
	assertEnvelopeCode(t, createdRole, http.StatusOK, 200, "success")
	rolePage := serveJSON(router, http.MethodGet, "/api/system/role/page?page=1&pageSize=500", "", token)
	if rolePage.Code != http.StatusOK || !strings.Contains(rolePage.Body.String(), `"roleCode":"OPS"`) {
		t.Fatalf("role page = status %d body=%s", rolePage.Code, rolePage.Body.String())
	}
	var createdRoleTime time.Time
	if err := database.GORM.Table("sys_role").Select("create_time").Where("role_code = ?", "OPS").Scan(&createdRoleTime).Error; err != nil {
		t.Fatalf("load created role time: %v", err)
	}
	if createdRoleTime.IsZero() {
		t.Fatal("created role create_time must not be zero")
	}

	assigned := serveJSON(router, http.MethodPut, "/api/system/role/2/menus", `{"menuIds":[1,2]}`, token)
	assertEnvelopeCode(t, assigned, http.StatusOK, 200, "success")
	roleDetail := serveJSON(router, http.MethodGet, "/api/system/role/2", "", token)
	if roleDetail.Code != http.StatusOK || !strings.Contains(roleDetail.Body.String(), `"menuIds":[1,2]`) {
		t.Fatalf("role detail = status %d body=%s", roleDetail.Code, roleDetail.Body.String())
	}

	createdMenu := serveJSON(router, http.MethodPost, "/api/system/menu", `{"parentId":0,"menuName":"custom-menu","menuType":"MENU","path":"/custom","visible":1,"status":1}`, token)
	assertEnvelopeCode(t, createdMenu, http.StatusOK, 200, "success")
	menuTree := serveJSON(router, http.MethodGet, "/api/system/menu/tree", "", token)
	if menuTree.Code != http.StatusOK || !strings.Contains(menuTree.Body.String(), `"menuName":"custom-menu"`) {
		t.Fatalf("menu tree = status %d body=%s", menuTree.Code, menuTree.Body.String())
	}

	invalidPage := serveJSON(router, http.MethodGet, "/api/system/menu/page?pageSize=501", "", token)
	assertEnvelopeCode(t, invalidPage, http.StatusBadRequest, 400, "参数错误")
	missingMenu := serveJSON(router, http.MethodPut, "/api/system/role/2/menus", `{"menuIds":[999]}`, token)
	assertEnvelopeCode(t, missingMenu, http.StatusOK, 404, "数据不存在")

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("request_id <> '' AND operator_id = ?", 1).Count(&auditCount).Error; err != nil {
		t.Fatalf("count operation audit logs: %v", err)
	}
	if auditCount != 3 {
		t.Fatalf("operation audit count = %d, want 3 successful mutations", auditCount)
	}
}

func TestRBACWritesAndAuditsInOneTransaction(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build RBAC API: %v", err)
	}
	token := loginAdmin(t, router)

	// 成功写操作后业务数据与审计日志同时存在。
	created := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"运营","roleCode":"OPS","status":1,"sortOrder":2}`, token)
	assertEnvelopeCode(t, created, http.StatusOK, 200, "success")
	var roleCount, auditCount int64
	if err := database.GORM.Table("sys_role").Where("role_code='OPS' AND deleted=0").Count(&roleCount).Error; err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roleCount != 1 {
		t.Fatalf("role count = %d, want 1", roleCount)
	}
	if err := database.GORM.Table("sys_oper_log").Where("module_name='role' AND operator_id=1 AND request_id <> ''").Count(&auditCount).Error; err != nil {
		t.Fatalf("count operation logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("operation audit count = %d, want 1", auditCount)
	}

	// 让后续审计写入必然失败（NOT VALID 约束只校验新插入行）。
	if err := database.GORM.Exec("ALTER TABLE sys_oper_log ADD CONSTRAINT chk_oper_status CHECK (operation_status <> 'SUCCESS') NOT VALID").Error; err != nil {
		t.Fatalf("add audit constraint: %v", err)
	}

	// 审计写入失败时业务写入必须整体回滚。
	attempt := serveJSON(router, http.MethodPost, "/api/system/menu", `{"parentId":0,"menuName":"custom-menu","menuType":"MENU","path":"/custom","visible":1,"status":1}`, token)
	if attempt.Code == http.StatusOK {
		t.Fatalf("menu create must fail when audit write fails, body=%s", attempt.Body.String())
	}
	var menuCount int64
	if err := database.GORM.Table("sys_menu").Where("menu_name='custom-menu'").Count(&menuCount).Error; err != nil {
		t.Fatalf("count menus after failed create: %v", err)
	}
	if menuCount != 0 {
		t.Fatalf("menu count = %d, want 0 after rollback", menuCount)
	}
}

func TestSysConfigCreateRollsBackWhenAuditFails(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build sysconfig API: %v", err)
	}
	token := loginAdmin(t, router)

	created := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"原子化","configKey":"atomic.config","configValue":"true"}`, token)
	assertEnvelopeCode(t, created, http.StatusOK, 200, "success")
	var configCount, auditCount int64
	if err := database.GORM.Table("sys_config").Where("config_key='atomic.config' AND deleted=0").Count(&configCount).Error; err != nil {
		t.Fatalf("count configs: %v", err)
	}
	if configCount != 1 {
		t.Fatalf("config count = %d, want 1", configCount)
	}
	if err := database.GORM.Table("sys_oper_log").Where("module_name='config' AND operator_id=1 AND request_id <> ''").Count(&auditCount).Error; err != nil {
		t.Fatalf("count operation logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("operation audit count = %d, want 1", auditCount)
	}

	// 让后续审计写入必然失败（NOT VALID 约束只校验新插入行）。
	if err := database.GORM.Exec("ALTER TABLE sys_oper_log ADD CONSTRAINT chk_oper_status CHECK (operation_status <> 'SUCCESS') NOT VALID").Error; err != nil {
		t.Fatalf("add audit constraint: %v", err)
	}

	// 审计写入失败时业务写入必须整体回滚。
	attempt := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"回滚","configKey":"atomic.rollback","configValue":"true"}`, token)
	if attempt.Code == http.StatusOK {
		t.Fatalf("config create must fail when audit write fails, body=%s", attempt.Body.String())
	}
	if err := database.GORM.Table("sys_config").Where("config_key='atomic.rollback'").Count(&configCount).Error; err != nil {
		t.Fatalf("count configs after failed create: %v", err)
	}
	if configCount != 0 {
		t.Fatalf("config count = %d, want 0 after rollback", configCount)
	}
}

func TestDepartmentAndUserContractRevokesSessions(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build department and user API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	createdDept := serveJSON(router, http.MethodPost, "/api/system/dept", `{"parentId":1,"deptName":"研发部","deptCode":"RND","status":1}`, adminToken)
	assertEnvelopeCode(t, createdDept, http.StatusOK, 200, "success")
	deptTree := serveJSON(router, http.MethodGet, "/api/system/dept/tree", "", adminToken)
	if deptTree.Code != http.StatusOK || !strings.Contains(deptTree.Body.String(), `"deptCode":"RND"`) {
		t.Fatalf("department tree = status %d body=%s", deptTree.Code, deptTree.Body.String())
	}

	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"developer","nickname":"开发者","deptId":2,"status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assignedRoles := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[1]}`, adminToken)
	assertEnvelopeCode(t, assignedRoles, http.StatusOK, 200, "success")
	userPage := serveJSON(router, http.MethodGet, "/api/system/user/page?page=1&pageSize=10", "", adminToken)
	if userPage.Code != http.StatusOK || !strings.Contains(userPage.Body.String(), `"deptName":"研发部"`) || !strings.Contains(userPage.Body.String(), `"roleCode":"ADMIN"`) {
		t.Fatalf("user page = status %d body=%s", userPage.Code, userPage.Body.String())
	}
	invalidPage := serveJSON(router, http.MethodGet, "/api/system/user/page?pageSize=501", "", adminToken)
	assertEnvelopeCode(t, invalidPage, http.StatusBadRequest, 400, "参数错误")

	userToken := loginUser(t, router, "developer", "admin123")
	disabled := serveJSON(router, http.MethodPatch, "/api/system/user/2/status", `{"status":0}`, adminToken)
	assertEnvelopeCode(t, disabled, http.StatusOK, 200, "success")
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, "/api/auth/me", "", userToken))

	enabled := serveJSON(router, http.MethodPatch, "/api/system/user/2/status", `{"status":1}`, adminToken)
	assertEnvelopeCode(t, enabled, http.StatusOK, 200, "success")
	userToken = loginUser(t, router, "developer", "admin123")
	reset := serveJSON(router, http.MethodPut, "/api/system/user/2/reset-password", "", adminToken)
	assertEnvelopeCode(t, reset, http.StatusOK, 200, "success")
	if !strings.Contains(reset.Body.String(), `"password":"admin123"`) {
		t.Fatalf("reset password response = %s", reset.Body.String())
	}
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, "/api/auth/me", "", userToken))

	userToken = loginUser(t, router, "developer", "admin123")
	changed := serveJSON(router, http.MethodPut, "/api/system/user/me/password", `{"oldPassword":"admin123","newPassword":"new-password"}`, userToken)
	assertEnvelopeCode(t, changed, http.StatusOK, 200, "success")
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, "/api/auth/me", "", userToken))
	_ = loginUser(t, router, "developer", "new-password")

	invalidRole := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[999]}`, adminToken)
	assertEnvelopeCode(t, invalidRole, http.StatusOK, 404, "数据不存在")

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("request_id <> ''").Count(&auditCount).Error; err != nil {
		t.Fatalf("count operation audit logs: %v", err)
	}
	if auditCount != 7 {
		t.Fatalf("operation audit count = %d, want 7 successful mutations", auditCount)
	}
}

func TestDictionaryAndConfigContractUsesSeedAndAudit(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	token := loginAdmin(t, router)
	items := serveJSON(router, http.MethodGet, "/api/system/dict/USER_STATUS/items", "", token)
	if items.Code != http.StatusOK || !strings.Contains(items.Body.String(), `"value":"1"`) {
		t.Fatalf("seed dict items=%d %s", items.Code, items.Body.String())
	}
	createdType := serveJSON(router, http.MethodPost, "/api/system/dict-type", `{"dictName":"环境","dictCode":"ENV"}`, token)
	assertEnvelopeCode(t, createdType, http.StatusOK, 200, "success")
	createdConfig := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"演示","configKey":"demo.enabled","configValue":"true"}`, token)
	assertEnvelopeCode(t, createdConfig, http.StatusOK, 200, "success")
	configPage := serveJSON(router, http.MethodGet, "/api/system/config/page?page=1&pageSize=10", "", token)
	if configPage.Code != http.StatusOK || !strings.Contains(configPage.Body.String(), `"configKey":"demo.enabled"`) || !strings.Contains(configPage.Body.String(), `"status":1`) {
		t.Fatalf("config page=%d %s", configPage.Code, configPage.Body.String())
	}
	key := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", token)
	assertEnvelopeCode(t, key, http.StatusOK, 200, "success")
	disabled := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, token)
	assertEnvelopeCode(t, disabled, http.StatusOK, 200, "success")
	missingKey := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", token)
	assertEnvelopeCode(t, missingKey, http.StatusOK, 404, "数据不存在")
}

func TestLoginAndOperLogContractServesQueriesAndDetails(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build log management API: %v", err)
	}

	failedLogin := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	assertEnvelopeCode(t, failedLogin, http.StatusOK, 400, "用户名或密码错误")
	token := loginAdmin(t, router)

	createdConfig := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"日志测试","configKey":"log.demo","configValue":"on"}`, token)
	assertEnvelopeCode(t, createdConfig, http.StatusOK, 200, "success")

	loginPage := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=10&username=admin", "", token)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `"total":2`) || !strings.Contains(loginPage.Body.String(), `"loginStatus":"FAIL"`) {
		t.Fatalf("login-log page = status %d body=%s", loginPage.Code, loginPage.Body.String())
	}
	operPage := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=10&operatorName=admin", "", token)
	if operPage.Code != http.StatusOK || !strings.Contains(operPage.Body.String(), `"moduleName":"config"`) || !strings.Contains(operPage.Body.String(), `"operatorName":"admin"`) || !strings.Contains(operPage.Body.String(), `"total":1`) {
		t.Fatalf("oper-log page = status %d body=%s", operPage.Code, operPage.Body.String())
	}

	var detailID int64
	var operLogCount int64
	if err := database.GORM.Table("sys_oper_log").Where("module_name='config' AND operator_id=1").Count(&operLogCount).Error; err != nil {
		t.Fatalf("count operation logs: %v", err)
	}
	if operLogCount != 1 {
		t.Fatalf("operation log count = %d, want 1", operLogCount)
	}
	if err := database.GORM.Table("sys_oper_log").Where("module_name='config' AND operator_id=1").Pluck("id", &detailID).Error; err != nil {
		t.Fatalf("load operation log id: %v", err)
	}

	detail := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/system/oper-log/%d", detailID), "", token)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"operatorName":"admin"`) || !strings.Contains(detail.Body.String(), `"moduleName":"config"`) {
		t.Fatalf("oper-log detail = status %d body=%s", detail.Code, detail.Body.String())
	}
	missing := serveJSON(router, http.MethodGet, "/api/system/oper-log/99999", "", token)
	assertEnvelopeCode(t, missing, http.StatusOK, 404, "数据不存在")
	missingLogin := serveJSON(router, http.MethodGet, "/api/system/login-log/99999", "", token)
	assertEnvelopeCode(t, missingLogin, http.StatusOK, 404, "数据不存在")

	setSwitch := func(value string) {
		t.Helper()
		if err := database.GORM.Table("sys_config").Where("id=1").Update("config_value", value).Error; err != nil {
			t.Fatalf("set log-clear switch: %v", err)
		}
	}
	setSwitch("false")
	forbidden := serveJSON(router, http.MethodDelete, "/api/system/login-log/clear", "", token)
	assertEnvelopeCode(t, forbidden, http.StatusOK, 403, "无权限")
	stillPresent := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=10", "", token)
	if stillPresent.Code != http.StatusOK || !strings.Contains(stillPresent.Body.String(), `"total":2`) {
		t.Fatalf("login logs must survive a forbidden clear: status %d body=%s", stillPresent.Code, stillPresent.Body.String())
	}

	forbidden = serveJSON(router, http.MethodDelete, "/api/system/oper-log/clear", "", token)
	assertEnvelopeCode(t, forbidden, http.StatusOK, 403, "无权限")
	operLogsSurvive := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=10", "", token)
	if operLogsSurvive.Code != http.StatusOK || !strings.Contains(operLogsSurvive.Body.String(), `"total":1`) {
		t.Fatalf("oper logs must survive a forbidden clear: status %d body=%s", operLogsSurvive.Code, operLogsSurvive.Body.String())
	}

	setSwitch("true")
	cleared := serveJSON(router, http.MethodDelete, "/api/system/login-log/clear", "", token)
	assertEnvelopeCode(t, cleared, http.StatusOK, 200, "success")
	emptyPage := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=10", "", token)
	if emptyPage.Code != http.StatusOK || !strings.Contains(emptyPage.Body.String(), `"total":0`) || !strings.Contains(emptyPage.Body.String(), `"records":[]`) {
		t.Fatalf("login logs must be cleared: status %d body=%s", emptyPage.Code, emptyPage.Body.String())
	}
	var clearAuditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("module_name='login-log'").Count(&clearAuditCount).Error; err != nil {
		t.Fatalf("count clear audit logs: %v", err)
	}
	if clearAuditCount != 1 {
		t.Fatalf("clear audit count = %d, want 1", clearAuditCount)
	}
	clearedOper := serveJSON(router, http.MethodDelete, "/api/system/oper-log/clear", "", token)
	assertEnvelopeCode(t, clearedOper, http.StatusOK, 200, "success")
	emptyOperPage := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=10&moduleName=zzz_nonexistent", "", token)
	if emptyOperPage.Code != http.StatusOK || !strings.Contains(emptyOperPage.Body.String(), `"total":0`) || !strings.Contains(emptyOperPage.Body.String(), `"records":[]`) {
		t.Fatalf("oper logs empty page must serialize records as array: status %d body=%s", emptyOperPage.Code, emptyOperPage.Body.String())
	}
}

func TestFileContractUploadsAndStreamsWithPostgres(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	storageRoot := t.TempDir()
	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, storageRoot))
	if err != nil {
		t.Fatalf("build file management API: %v", err)
	}
	token := loginAdmin(t, router)

	const content = "hello file content"
	upload := serveMultipart(t, router, http.MethodPost, "/api/system/file/upload", token, []filePart{
		{field: "file", filename: "contract.txt", contentType: "text/plain", content: content},
	}, "system", "")
	if upload.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body=%s", upload.Code, upload.Body.String())
	}
	var uploaded struct {
		Code int `json:"code"`
		Data struct {
			ID             int64  `json:"id"`
			OriginalName   string `json:"originalName"`
			FileSize       int64  `json:"fileSize"`
			MimeType       string `json:"mimeType"`
			AccessURL      string `json:"accessUrl"`
			BusinessModule string `json:"businessModule"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode upload response: %v body=%s", err, upload.Body.String())
	}
	if uploaded.Code != 200 || uploaded.Data.OriginalName != "contract.txt" || uploaded.Data.FileSize != int64(len(content)) || uploaded.Data.MimeType != "text/plain" || uploaded.Data.BusinessModule != "system" {
		t.Fatalf("upload response = %+v", uploaded)
	}
	if want := fmt.Sprintf("/api/system/file/%d/view", uploaded.Data.ID); uploaded.Data.AccessURL != want {
		t.Fatalf("accessUrl = %q, want %q", uploaded.Data.AccessURL, want)
	}

	page := serveJSON(router, http.MethodGet, "/api/system/file/page?page=1&pageSize=10&businessModule=system", "", token)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"originalName":"contract.txt"`) || !strings.Contains(page.Body.String(), `"total":1`) {
		t.Fatalf("file page = status %d body=%s", page.Code, page.Body.String())
	}

	for _, test := range []struct {
		name     string
		path     string
		disposit string
	}{
		{name: "download", path: fmt.Sprintf("/api/system/file/%d/download", uploaded.Data.ID), disposit: "attachment"},
		{name: "view", path: fmt.Sprintf("/api/system/file/%d/view", uploaded.Data.ID), disposit: "inline"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := servePlain(t, router, http.MethodGet, test.path, token)
			if response.Code != http.StatusOK || response.Body.String() != content {
				t.Fatalf("%s = status %d body=%q", test.name, response.Code, response.Body.String())
			}
			if disposition := response.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, test.disposit+"; filename*=UTF-8''contract.txt") {
				t.Fatalf("%s Content-Disposition = %q", test.name, disposition)
			}
		})
	}

	update := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/system/file/%d", uploaded.Data.ID), `{"businessModule":"avatar","remark":"头像文件"}`, token)
	assertEnvelopeCode(t, update, http.StatusOK, 200, "success")
	detail := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/system/file/%d", uploaded.Data.ID), "", token)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"businessModule":"avatar"`) || !strings.Contains(detail.Body.String(), `"remark":"头像文件"`) {
		t.Fatalf("file detail = status %d body=%s", detail.Code, detail.Body.String())
	}

	status := serveJSON(router, http.MethodPatch, fmt.Sprintf("/api/system/file/%d/status", uploaded.Data.ID), `{"status":0}`, token)
	assertEnvelopeCode(t, status, http.StatusOK, 200, "success")
	disabledPage := serveJSON(router, http.MethodGet, "/api/system/file/page?page=1&pageSize=10&status=0", "", token)
	if disabledPage.Code != http.StatusOK || !strings.Contains(disabledPage.Body.String(), `"status":0`) {
		t.Fatalf("disabled page = status %d body=%s", disabledPage.Code, disabledPage.Body.String())
	}

	var storagePath string
	if err := database.GORM.Table("sys_file").Where("id=?", uploaded.Data.ID).Pluck("storage_path", &storagePath).Error; err != nil {
		t.Fatalf("load storage path: %v", err)
	}
	physicalPath := filepath.Join(storageRoot, filepath.FromSlash(storagePath))
	if _, err := os.Stat(physicalPath); err != nil {
		t.Fatalf("physical file missing before delete: %v", err)
	}
	failedBatch := serveJSON(router, http.MethodPost, "/api/system/file/batch-delete", fmt.Sprintf(`{"ids":[%d,99999]}`, uploaded.Data.ID), token)
	assertEnvelopeCode(t, failedBatch, http.StatusOK, 404, "数据不存在")
	if _, err := os.Stat(physicalPath); err != nil {
		t.Fatalf("physical file missing after rejected batch delete: %v", err)
	}
	stillPresent := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/system/file/%d", uploaded.Data.ID), "", token)
	assertEnvelopeCode(t, stillPresent, http.StatusOK, 200, "success")

	deleted := serveJSON(router, http.MethodDelete, fmt.Sprintf("/api/system/file/%d", uploaded.Data.ID), "", token)
	assertEnvelopeCode(t, deleted, http.StatusOK, 200, "success")
	afterDelete := serveJSON(router, http.MethodGet, "/api/system/file/page?page=1&pageSize=10", "", token)
	if afterDelete.Code != http.StatusOK || !strings.Contains(afterDelete.Body.String(), `"total":0`) {
		t.Fatalf("page after soft delete = status %d body=%s", afterDelete.Code, afterDelete.Body.String())
	}
	if _, err := os.Stat(physicalPath); err != nil {
		t.Fatalf("physical file must remain after metadata delete: %v", err)
	}
	missingDetail := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/system/file/%d", uploaded.Data.ID), "", token)
	assertEnvelopeCode(t, missingDetail, http.StatusOK, 404, "数据不存在")

	emptyUpload := serveMultipart(t, router, http.MethodPost, "/api/system/file/upload", token, []filePart{
		{field: "file", filename: "empty.txt", contentType: "text/plain", content: ""},
	}, "", "")
	if emptyUpload.Code != http.StatusBadRequest || !strings.Contains(emptyUpload.Body.String(), `"code":400`) {
		t.Fatalf("empty upload = status %d body=%s", emptyUpload.Code, emptyUpload.Body.String())
	}
	missingStream := servePlain(t, router, http.MethodGet, "/api/system/file/99999/download", token)
	assertEnvelopeCode(t, missingStream, http.StatusOK, 404, "数据不存在")

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("module_name='file' AND operator_id=1 AND request_id <> ''").Count(&auditCount).Error; err != nil {
		t.Fatalf("count file operation logs: %v", err)
	}
	if auditCount != 4 {
		t.Fatalf("file operation audit count = %d, want 4 (upload, update, status, delete)", auditCount)
	}
}

func TestNotificationContractUsesPostgres(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	router, err := app.Build(testAPIConfig(), database, testDependencies(t, database, t.TempDir()))
	if err != nil {
		t.Fatalf("build PostgreSQL notification API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"postgres-notify-user","nickname":"PostgreSQL通知用户","status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assignedRoles := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[1]}`, adminToken)
	assertEnvelopeCode(t, assignedRoles, http.StatusOK, 200, "success")
	userToken := loginUser(t, router, "postgres-notify-user", "admin123")

	published := serveJSON(router, http.MethodPost, "/api/system/notification", `{"title":"PostgreSQL通知","content":"通知内容","userIds":[2]}`, adminToken)
	assertEnvelopeCode(t, published, http.StatusOK, 200, "success")

	page := serveJSON(router, http.MethodGet, "/api/system/notification/page?page=1&pageSize=20", "", userToken)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"title":"PostgreSQL通知"`) || !strings.Contains(page.Body.String(), `"isRead":0`) {
		t.Fatalf("PostgreSQL notification page = %d %s", page.Code, page.Body.String())
	}
	unread := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unread.Code != http.StatusOK || !strings.Contains(unread.Body.String(), `"data":2`) {
		t.Fatalf("PostgreSQL unread count = %d %s", unread.Code, unread.Body.String())
	}

	var notificationID int64
	if err := database.GORM.Table("sys_notification").Where("title = ?", "PostgreSQL通知").Pluck("id", &notificationID).Error; err != nil {
		t.Fatalf("load PostgreSQL notification ID: %v", err)
	}
	detail := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/system/notification/%d", notificationID), "", userToken)
	assertEnvelopeCode(t, detail, http.StatusOK, 200, "success")
	unreadAfterDetail := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unreadAfterDetail.Code != http.StatusOK || !strings.Contains(unreadAfterDetail.Body.String(), `"data":1`) {
		t.Fatalf("PostgreSQL unread count after detail = %d %s", unreadAfterDetail.Code, unreadAfterDetail.Body.String())
	}
}

// filePart is one multipart file part for serveMultipart.
type filePart struct {
	field       string
	filename    string
	contentType string
	content     string
}

func serveMultipart(t *testing.T, router http.Handler, method, path, token string, parts []filePart, businessModule, remark string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+part.field+`"; filename="`+part.filename+`"`)
		if part.contentType != "" {
			header.Set("Content-Type", part.contentType)
		}
		contentPart, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = contentPart.Write([]byte(part.content)); err != nil {
			t.Fatal(err)
		}
	}
	if businessModule != "" {
		if err := writer.WriteField("businessModule", businessModule); err != nil {
			t.Fatal(err)
		}
	}
	if remark != "" {
		if err := writer.WriteField("remark", remark); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// servePlain sends a request without a body and returns the raw response.
func servePlain(t *testing.T, router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type temporaryPostgres struct {
	container string
	dsn       string
}

func startPostgres(t *testing.T) temporaryPostgres {
	t.Helper()
	container := fmt.Sprintf("server-integration-%d", time.Now().UnixNano())
	password := "integration-password"
	command := exec.Command(
		"docker", "run", "--detach", "--rm", "--name", container,
		"--env", "POSTGRES_DB=integration",
		"--env", "POSTGRES_USER=integration",
		"--env", "POSTGRES_PASSWORD="+password,
		"--publish", "127.0.0.1::5432",
		postgresImage,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("start temporary PostgreSQL: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "--force", container).Run()
	})

	portOutput, err := exec.Command("docker", "port", container, "5432/tcp").Output()
	if err != nil {
		t.Fatalf("resolve temporary PostgreSQL port: %v", err)
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(portOutput)))
	if err != nil {
		t.Fatalf("parse temporary PostgreSQL port: %v", err)
	}

	database := temporaryPostgres{
		container: container,
		dsn:       "postgres://integration:" + password + "@127.0.0.1:" + port + "/integration?sslmode=disable",
	}
	database.waitUntilReady(t)
	return database
}

func (database temporaryPostgres) waitUntilReady(t *testing.T) {
	t.Helper()
	endpoint, err := url.Parse(database.dsn)
	if err != nil {
		t.Fatalf("parse temporary PostgreSQL endpoint: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	readyStreak := 0
	for time.Now().Before(deadline) {
		if err := exec.Command("docker", "exec", database.container, "pg_isready", "-U", "integration", "-d", "integration").Run(); err == nil {
			connection, dialErr := net.DialTimeout("tcp", endpoint.Host, time.Second)
			if dialErr == nil {
				_ = connection.Close()
				readyStreak++
				if readyStreak >= 2 {
					return
				}
			} else {
				readyStreak = 0
			}
		} else {
			readyStreak = 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("temporary PostgreSQL did not become ready within 45 seconds")
}

func runMigrations(t *testing.T, root, dsn string) {
	runMigrationsWithEnvironment(t, root, dsn, config.EnvironmentTest)
}

func runMigrationsWithEnvironment(t *testing.T, root, dsn, environment string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up", "--kind", "all")
	command.Dir = root
	command.Env = integrationEnvironment(t, dsn, environment)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run migrations against temporary PostgreSQL: %v\n%s", err, output)
	}
}

func integrationEnvironment(t *testing.T, dsn, environmentName string) []string {
	t.Helper()
	databaseConfig := databaseConfigFromDSN(t, dsn)
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "APP_DATABASE__") || strings.HasPrefix(entry, "APP_JWT__SECRET=") || strings.HasPrefix(entry, "APP_ENV=") || strings.HasPrefix(entry, "APP_CONFIG_PROFILE=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"APP_ENV="+environmentName,
		"APP_DATABASE__URL="+databaseConfig.URL,
		"APP_DATABASE__USERNAME="+databaseConfig.Username,
		"APP_DATABASE__PASSWORD="+databaseConfig.Password,
		"APP_JWT__SECRET=integration-test-secret-that-is-never-deployed",
	)
}

func (database temporaryPostgres) assertGooseSchemaVersionTable(t *testing.T) {
	t.Helper()
	command := exec.Command("docker", "exec", database.container, "psql", "-U", "integration", "-d", "integration", "-tAc", "SELECT 1 FROM goose_schema_db_version LIMIT 1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("verify Goose schema migration table: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "1" {
		t.Fatalf("schema baseline version = %q, want 1", output)
	}
}

func verifyDatabaseReadiness(t *testing.T, dsn string) {
	t.Helper()
	database := openTemporaryDatabase(t, dsn)
	if err := database.Ready(context.Background()); err != nil {
		t.Fatalf("ready database returned error: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database after readiness check: %v", err)
	}
	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("closed database was unexpectedly ready")
	}
}

func openTemporaryDatabase(t *testing.T, dsn string) *platformdatabase.Database {
	t.Helper()
	databaseConfig := databaseConfigFromDSN(t, dsn)
	databaseConfig.Driver = platformdatabase.DriverPostgres
	databaseConfig.MaxOpenConns = 2
	databaseConfig.MaxIdleConns = 1
	database, err := platformdatabase.Open(context.Background(), databaseConfig)
	if err != nil {
		t.Fatalf("open database for readiness check: %v", err)
	}
	return database
}

func databaseConfigFromDSN(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	endpoint, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse temporary PostgreSQL DSN: %v", err)
	}
	if endpoint.User == nil {
		t.Fatal("temporary PostgreSQL DSN does not contain username and password")
	}
	password, hasPassword := endpoint.User.Password()
	if !hasPassword {
		t.Fatal("temporary PostgreSQL DSN does not contain username and password")
	}
	username := endpoint.User.Username()
	endpoint.User = nil
	return config.DatabaseConfig{
		URL:      endpoint.String(),
		Username: username,
		Password: password,
	}
}

func mustTokenManager(t *testing.T) *auth.TokenManager {
	t.Helper()
	manager, err := auth.NewTokenManager(auth.TokenConfig{
		SigningKey: "integration-test-signing-key",
		Issuer:     "server",
		Audience:   "web",
		TTL:        2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	return manager
}

func testAPIConfig() config.Config {
	return config.Config{
		Environment: config.EnvironmentTest,
		CORS:        config.CORSConfig{AllowedOrigins: []string{"*"}},
		Log:         config.LogConfig{Level: "error", Format: "text"},
	}
}

// testDependencies constructs every business service from real repositories so
// that app.Build receives a complete named dependency set, as required by the
// HTTP application contract. Each test overrides only the modules it exercises.
func testDependencies(t *testing.T, database *platformdatabase.Database, storageRoot string) app.Dependencies {
	t.Helper()
	authService, err := auth.NewService(auth.NewRepository(database.GORM), mustTokenManager(t))
	if err != nil {
		t.Fatalf("create authentication service: %v", err)
	}
	rbacService, err := rbac.NewService(rbac.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create RBAC service: %v", err)
	}
	deptService, err := dept.NewService(dept.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create department service: %v", err)
	}
	notificationRepository := notification.NewRepository(database.GORM)
	userService, err := usermgmt.NewService(usermgmt.NewRepository(database.GORM, notificationRepository), "admin123")
	if err != nil {
		t.Fatalf("create user service: %v", err)
	}
	dictRepository := dictionary.NewRepository(database.GORM)
	dictionaryService, err := dictionary.NewService(dictRepository)
	if err != nil {
		t.Fatalf("create dictionary service: %v", err)
	}
	configService := sysconfig.NewService(sysconfig.NewRepository(database.GORM))
	fileStorage, err := filemgmt.NewLocalStorage(storageRoot)
	if err != nil {
		t.Fatalf("create file storage: %v", err)
	}
	fileService, err := filemgmt.NewService(filemgmt.NewRepository(database.GORM), fileStorage)
	if err != nil {
		t.Fatalf("create file service: %v", err)
	}
	logService, err := logmgmt.NewService(logmgmt.NewRepository(database.GORM), configService)
	if err != nil {
		t.Fatalf("create log service: %v", err)
	}
	datapointService, err := datapoint.NewService(datapoint.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create DataPoint service: %v", err)
	}
	return app.Dependencies{
		Auth:         authService,
		RBAC:         rbacService,
		Department:   deptService,
		User:         userService,
		Dictionary:   dictionaryService,
		SysConfig:    configService,
		File:         fileService,
		Log:          logService,
		DataPoint:    datapointService,
		Notification: mustNotificationService(t, notificationRepository),
	}
}

func mustNotificationService(t *testing.T, repository *notification.Repository) *notification.Service {
	t.Helper()
	service, err := notification.NewService(repository)
	if err != nil {
		t.Fatalf("create notification service: %v", err)
	}
	return service
}

func loginAdmin(t *testing.T, router http.Handler) string {
	t.Helper()
	return loginUser(t, router, "admin", "admin123")
}

func loginUser(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	response := serveJSON(router, http.MethodPost, "/api/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), "")
	if response.Code != http.StatusOK {
		t.Fatalf("%s login status = %d, body=%s", username, response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			TokenValue string `json:"tokenValue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s login: %v", username, err)
	}
	if !strings.HasPrefix(payload.Data.TokenValue, "Bearer ") {
		t.Fatalf("%s token = %q", username, payload.Data.TokenValue)
	}
	return payload.Data.TokenValue
}

func assertUnauthenticated(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	assertEnvelopeCode(t, response, http.StatusUnauthorized, 401, "未登录或 token 已失效")
}

func serveJSON(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertEnvelopeCode(t *testing.T, response *httptest.ResponseRecorder, status, code int, message string) {
	t.Helper()
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if response.Code != status || payload.Code != code || payload.Message != message {
		t.Fatalf("response = status %d payload=%+v body=%s, want status=%d code=%d message=%q", response.Code, payload, response.Body.String(), status, code, message)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	return root
}
