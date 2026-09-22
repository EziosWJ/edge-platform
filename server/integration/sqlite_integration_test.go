//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
	"github.com/EziosWJ/edge-platform/server/internal/realtime"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
	"github.com/EziosWJ/edge-platform/server/internal/usermgmt"
)

func TestSQLiteMigrationLifecycleAndBackup(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "app.db")
	runSQLiteMigrations(t, path)
	runSQLiteMigrations(t, path)

	database := openSQLiteDatabase(t, path)
	assertSQLitePragmas(t, database)
	var users, menus, configs int64
	if err := database.GORM.Table("sys_user").Count(&users).Error; err != nil {
		t.Fatalf("count SQLite users: %v", err)
	}
	if err := database.GORM.Table("sys_menu").Count(&menus).Error; err != nil {
		t.Fatalf("count SQLite menus: %v", err)
	}
	if err := database.GORM.Table("sys_config").Count(&configs).Error; err != nil {
		t.Fatalf("count SQLite configs: %v", err)
	}
	if users != 1 || menus != 17 || configs != 1 {
		t.Fatalf("seed counts = users %d, menus %d, configs %d; want 1, 17, 1", users, menus, configs)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close SQLite database: %v", err)
	}

	database = openSQLiteDatabase(t, path)
	defer func() { _ = database.Close() }()
	if err := database.Ready(context.Background()); err != nil {
		t.Fatalf("reopened SQLite database is not ready: %v", err)
	}
	var username string
	if err := database.GORM.Table("sys_user").Where("id = 1").Pluck("username", &username).Error; err != nil {
		t.Fatalf("read persisted SQLite user: %v", err)
	}
	if username != "admin" {
		t.Fatalf("persisted username = %q, want admin", username)
	}

	backupPath := filepath.Join(directory, "backup.db")
	if err := platformdatabase.BackupSQLite(context.Background(), path, backupPath); err != nil {
		t.Fatalf("backup SQLite database: %v", err)
	}
	if err := platformdatabase.VerifySQLiteFile(context.Background(), backupPath); err != nil {
		t.Fatalf("verify SQLite backup: %v", err)
	}
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat SQLite backup: %v", err)
	}
	if backupInfo.Mode().Perm() != 0o600 {
		t.Fatalf("SQLite backup permissions = %o, want 600", backupInfo.Mode().Perm())
	}
	runSQLiteMigrations(t, backupPath)
	restored := openSQLiteDatabase(t, backupPath)
	defer func() { _ = restored.Close() }()
	if err := restored.GORM.Table("sys_config").Where("config_key = ?", sysconfig.LogClearEnabledKey).Count(&configs).Error; err != nil {
		t.Fatalf("read restored SQLite config: %v", err)
	}
	if configs != 1 {
		t.Fatalf("restored config count = %d, want 1", configs)
	}
}

func TestSQLiteLogClearSeedDefaultsByEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment string
		want        string
	}{
		{environment: config.EnvironmentDev, want: "true"},
		{environment: config.EnvironmentTest, want: "false"},
		{environment: config.EnvironmentProd, want: "false"},
	} {
		t.Run(test.environment, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "seed.db")
			runSQLiteMigrationsWithEnvironment(t, path, test.environment)
			database := openSQLiteDatabase(t, path)
			defer func() { _ = database.Close() }()

			var value string
			if err := database.SQL.QueryRow("SELECT config_value FROM sys_config WHERE config_key = ?", sysconfig.LogClearEnabledKey).Scan(&value); err != nil {
				t.Fatalf("read SQLite log-clear seed: %v", err)
			}
			if value != test.want {
				t.Fatalf("SQLite log-clear seed for %s = %q, want %q", test.environment, value, test.want)
			}
		})
	}
}

func TestSQLiteSharedHTTPBusinessContract(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "contract.db")
	runSQLiteMigrations(t, databasePath)
	database := openSQLiteDatabase(t, databasePath)
	defer func() { _ = database.Close() }()

	dependencies := sqliteDependencies(t, database, filepath.Join(directory, "uploads"))
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build SQLite API: %v", err)
	}

	failedLogin := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	assertEnvelopeCode(t, failedLogin, http.StatusOK, 400, "用户名或密码错误")
	adminToken := loginAdmin(t, router)
	me := serveJSON(router, http.MethodGet, "/api/auth/me", "", adminToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"roleCode":"ADMIN"`) {
		t.Fatalf("SQLite current user = %d %s", me.Code, me.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", adminToken)
	if menus.Code != http.StatusOK || !strings.Contains(menus.Body.String(), `"menuName":"系统管理"`) {
		t.Fatalf("SQLite current menus = %d %s", menus.Code, menus.Body.String())
	}
	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"SQLite运营","roleCode":"SQLITE-OPS","status":1,"sortOrder":2}`, adminToken)
	assertEnvelopeCode(t, createdRole, http.StatusOK, 200, "success")
	roleMenus := serveJSON(router, http.MethodPut, "/api/system/role/2/menus", `{"menuIds":[1,2]}`, adminToken)
	assertEnvelopeCode(t, roleMenus, http.StatusOK, 200, "success")
	roleDetail := serveJSON(router, http.MethodGet, "/api/system/role/2", "", adminToken)
	if roleDetail.Code != http.StatusOK || !strings.Contains(roleDetail.Body.String(), `"menuIds":[1,2]`) {
		t.Fatalf("SQLite role detail = %d %s", roleDetail.Code, roleDetail.Body.String())
	}

	createdDept := serveJSON(router, http.MethodPost, "/api/system/dept", `{"parentId":1,"deptName":"SQLite研发部","deptCode":"SQLITE-RND","status":1}`, adminToken)
	assertEnvelopeCode(t, createdDept, http.StatusOK, 200, "success")
	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"sqlite-user","nickname":"SQLite用户","deptId":2,"status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assignedRoles := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[2]}`, adminToken)
	assertEnvelopeCode(t, assignedRoles, http.StatusOK, 200, "success")
	userToken := loginUser(t, router, "sqlite-user", "admin123")

	items := serveJSON(router, http.MethodGet, "/api/system/dict/USER_STATUS/items", "", adminToken)
	if items.Code != http.StatusOK || !strings.Contains(items.Body.String(), `"value":"1"`) {
		t.Fatalf("SQLite seed dict items = %d %s", items.Code, items.Body.String())
	}
	createdType := serveJSON(router, http.MethodPost, "/api/system/dict-type", `{"dictName":"SQLite环境","dictCode":"SQLITE_ENV"}`, adminToken)
	assertEnvelopeCode(t, createdType, http.StatusOK, 200, "success")
	createdConfig := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"SQLite演示","configKey":"sqlite.demo","configValue":"true"}`, adminToken)
	assertEnvelopeCode(t, createdConfig, http.StatusOK, 200, "success")
	configPage := serveJSON(router, http.MethodGet, "/api/system/config/page?page=1&pageSize=20&configKey=sqlite.demo", "", adminToken)
	if configPage.Code != http.StatusOK || !strings.Contains(configPage.Body.String(), `"configKey":"sqlite.demo"`) {
		t.Fatalf("SQLite config page = %d %s", configPage.Code, configPage.Body.String())
	}
	key := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, key, http.StatusOK, 200, "success")
	disabledConfig := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, adminToken)
	assertEnvelopeCode(t, disabledConfig, http.StatusOK, 200, "success")
	missingKey := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, missingKey, http.StatusOK, 404, "数据不存在")

	loginPage := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=20", "", adminToken)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `"loginStatus":"FAIL"`) {
		t.Fatalf("SQLite login logs = %d %s", loginPage.Code, loginPage.Body.String())
	}
	operPage := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=20&operatorName=admin", "", adminToken)
	if operPage.Code != http.StatusOK || !strings.Contains(operPage.Body.String(), `"operatorName":"admin"`) {
		t.Fatalf("SQLite operation logs = %d %s", operPage.Code, operPage.Body.String())
	}

	upload := serveMultipartFile(router, "/api/system/file/upload", adminToken, "sqlite.txt", "SQLite file content")
	assertEnvelopeCode(t, upload, http.StatusOK, 200, "success")
	var uploadPayload struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadPayload); err != nil || uploadPayload.Data.ID <= 0 {
		t.Fatalf("decode SQLite upload = %s, error=%v", upload.Body.String(), err)
	}
	download := servePlain(t, router, http.MethodGet, "/api/system/file/"+itoa(uploadPayload.Data.ID)+"/download", adminToken)
	if download.Code != http.StatusOK || download.Body.String() != "SQLite file content" {
		t.Fatalf("SQLite file download = %d %q", download.Code, download.Body.String())
	}

	publish := serveJSON(router, http.MethodPost, "/api/system/notification", `{"title":"SQLite通知","content":"通知内容","userIds":[2]}`, adminToken)
	assertEnvelopeCode(t, publish, http.StatusOK, 200, "success")
	page := serveJSON(router, http.MethodGet, "/api/system/notification/page?page=1&pageSize=20", "", userToken)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"title":"SQLite通知"`) || !strings.Contains(page.Body.String(), `"isRead":0`) {
		t.Fatalf("SQLite notification page = %d %s", page.Code, page.Body.String())
	}
	unread := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unread.Code != http.StatusOK || !strings.Contains(unread.Body.String(), `"data":2`) {
		t.Fatalf("SQLite unread count = %d %s", unread.Code, unread.Body.String())
	}
	detail := serveJSON(router, http.MethodGet, "/api/system/notification/2", "", userToken)
	assertEnvelopeCode(t, detail, http.StatusOK, 200, "success")
	unreadAfterDetail := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unreadAfterDetail.Code != http.StatusOK || !strings.Contains(unreadAfterDetail.Body.String(), `"data":1`) {
		t.Fatalf("SQLite unread count after detail = %d %s", unreadAfterDetail.Code, unreadAfterDetail.Body.String())
	}

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("operator_id = 1 AND operation_status = 'SUCCESS'").Count(&auditCount).Error; err != nil {
		t.Fatalf("count SQLite operation audits: %v", err)
	}
	if auditCount < 5 {
		t.Fatalf("SQLite successful operation audit count = %d, want at least 5", auditCount)
	}
}

func TestSQLiteBusinessAndAuditWriteRollbackTogether(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "rollback.db")
	runSQLiteMigrations(t, databasePath)
	database := openSQLiteDatabase(t, databasePath)
	defer func() { _ = database.Close() }()
	router, err := app.Build(testAPIConfig(), database, sqliteDependencies(t, database, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite rollback-test API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	if err := database.GORM.Exec(`
CREATE TRIGGER fail_success_audit
BEFORE INSERT ON sys_oper_log
WHEN NEW.operation_status = 'SUCCESS'
BEGIN
    SELECT RAISE(ABORT, 'audit failure');
END`).Error; err != nil {
		t.Fatalf("create SQLite audit failure trigger: %v", err)
	}
	defer func() { _ = database.GORM.Exec("DROP TRIGGER IF EXISTS fail_success_audit").Error }()

	attempt := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"回滚角色","roleCode":"ROLLBACK","status":1,"sortOrder":2}`, adminToken)
	if attempt.Code != http.StatusInternalServerError || strings.Contains(attempt.Body.String(), "audit failure") {
		t.Fatalf("SQLite audit failure response = %d %s, want generic 500", attempt.Code, attempt.Body.String())
	}
	var roleCount int64
	if err := database.GORM.Table("sys_role").Where("role_code = ?", "ROLLBACK").Count(&roleCount).Error; err != nil {
		t.Fatalf("count SQLite rolled-back role: %v", err)
	}
	if roleCount != 0 {
		t.Fatalf("SQLite role count after audit failure = %d, want 0", roleCount)
	}
}

func TestSQLiteLockTimeoutUsesTemporaryUnavailableContract(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "lock.db")
	runSQLiteMigrations(t, databasePath)
	lockerDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = lockerDatabase.Close() }()
	requestDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = requestDatabase.Close() }()
	router, err := app.Build(testAPIConfig(), requestDatabase, sqliteDependencies(t, requestDatabase, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite lock-test API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	transaction, err := lockerDatabase.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin SQLite lock transaction: %v", err)
	}
	if _, err := transaction.ExecContext(context.Background(), "UPDATE sys_config SET update_time = update_time WHERE id = 1"); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("hold SQLite write lock: %v", err)
	}

	started := time.Now()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":1}`, adminToken)
	}()
	var result *httptest.ResponseRecorder
	select {
	case result = <-response:
	case <-time.After(8 * time.Second):
		_ = transaction.Rollback()
		t.Fatal("SQLite lock request exceeded its bounded timeout")
	}
	_ = transaction.Rollback()
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("SQLite lock request took %s, want bounded response", elapsed)
	}
	if result.Code != http.StatusServiceUnavailable || !strings.Contains(result.Body.String(), `"code":503`) {
		t.Fatalf("SQLite lock response = %d %s, want 503 contract", result.Code, result.Body.String())
	}
	if strings.Contains(strings.ToLower(result.Body.String()), "database is locked") {
		t.Fatalf("SQLite lock response leaked driver detail: %s", result.Body.String())
	}
}

func TestSQLiteTransientLockWaitsAndCommitsOnce(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "transient-lock.db")
	runSQLiteMigrations(t, databasePath)
	lockerDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = lockerDatabase.Close() }()
	requestDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = requestDatabase.Close() }()
	router, err := app.Build(testAPIConfig(), requestDatabase, sqliteDependencies(t, requestDatabase, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite transient-lock API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	transaction, err := lockerDatabase.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin SQLite transient lock transaction: %v", err)
	}
	if _, err := transaction.ExecContext(context.Background(), "UPDATE sys_config SET update_time = update_time WHERE id = 1"); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("hold SQLite transient write lock: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = transaction.Rollback()
		close(released)
	}()

	response := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, adminToken)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("SQLite transient lock was not released")
	}
	assertEnvelopeCode(t, response, http.StatusOK, 200, "success")
	var auditCount int64
	if err := requestDatabase.GORM.Table("sys_oper_log").Where("module_name = 'config' AND operation_type = 'config.status' AND operator_id = 1").Count(&auditCount).Error; err != nil {
		t.Fatalf("count SQLite transient-lock audits: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("SQLite transient-lock audit count = %d, want exactly 1", auditCount)
	}
}

func runSQLiteMigrations(t *testing.T, path string) {
	runSQLiteMigrationsWithEnvironment(t, path, config.EnvironmentTest)
}

func runSQLiteMigrationsWithEnvironment(t *testing.T, path, environmentName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up", "--kind", "all")
	command.Dir = projectRoot(t)
	command.Env = sqliteEnvironment(path, environmentName)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run SQLite migrations: %v\n%s", err, output)
	}
}

func sqliteEnvironment(path, environmentName string) []string {
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "APP_DATABASE__") || strings.HasPrefix(entry, "APP_JWT__SECRET=") || strings.HasPrefix(entry, "APP_ENV=") || strings.HasPrefix(entry, "APP_CONFIG_PROFILE=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"APP_ENV="+environmentName,
		"APP_DATABASE__DRIVER=sqlite",
		"APP_DATABASE__URL="+path,
		"APP_DATABASE__USERNAME=",
		"APP_DATABASE__PASSWORD=",
		"APP_JWT__SECRET=integration-test-secret-that-is-never-deployed",
	)
}

func openSQLiteDatabase(t *testing.T, path string) *platformdatabase.Database {
	t.Helper()
	database, err := platformdatabase.Open(context.Background(), config.DatabaseConfig{
		Driver:       platformdatabase.DriverSQLite,
		URL:          path,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	return database
}

func assertSQLitePragmas(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	checks := []struct {
		name string
		want string
	}{
		{name: "foreign_keys", want: "1"},
		{name: "journal_mode", want: "wal"},
		{name: "busy_timeout", want: "5000"},
		{name: "synchronous", want: "1"},
	}
	for _, check := range checks {
		var got string
		if err := database.SQL.QueryRow("PRAGMA " + check.name).Scan(&got); err != nil {
			t.Fatalf("query SQLite pragma %s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("SQLite pragma %s = %q, want %q", check.name, got, check.want)
		}
	}
}

func sqliteDependencies(t *testing.T, database *platformdatabase.Database, storageRoot string) app.Dependencies {
	t.Helper()
	authService, err := auth.NewService(auth.NewRepository(database.GORM), mustTokenManager(t))
	if err != nil {
		t.Fatalf("create SQLite auth service: %v", err)
	}
	rbacService, err := rbac.NewService(rbac.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite RBAC service: %v", err)
	}
	deptService, err := dept.NewService(dept.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite department service: %v", err)
	}
	notificationRepository := notification.NewRepository(database.GORM)
	userService, err := usermgmt.NewService(usermgmt.NewRepository(database.GORM, notificationRepository), "admin123")
	if err != nil {
		t.Fatalf("create SQLite user service: %v", err)
	}
	dictionaryService, err := dictionary.NewService(dictionary.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite dictionary service: %v", err)
	}
	configService := sysconfig.NewService(sysconfig.NewRepository(database.GORM))
	storage, err := filemgmt.NewLocalStorage(storageRoot)
	if err != nil {
		t.Fatalf("create SQLite file storage: %v", err)
	}
	fileService, err := filemgmt.NewService(filemgmt.NewRepository(database.GORM), storage)
	if err != nil {
		t.Fatalf("create SQLite file service: %v", err)
	}
	logService, err := logmgmt.NewService(logmgmt.NewRepository(database.GORM), configService)
	if err != nil {
		t.Fatalf("create SQLite log service: %v", err)
	}
	realtimeHub := realtime.NewHub(0)
	datapointService, err := datapoint.NewService(datapoint.NewRepository(database.GORM, realtimeHub))
	if err != nil {
		t.Fatalf("create SQLite DataPoint service: %v", err)
	}
	notificationService, err := notification.NewService(notificationRepository)
	if err != nil {
		t.Fatalf("create SQLite notification service: %v", err)
	}
	return app.Dependencies{
		Auth: authService, RBAC: rbacService, Department: deptService, User: userService,
		Dictionary: dictionaryService, SysConfig: configService, File: fileService,
		Log: logService, Notification: notificationService, DataPoint: datapointService,
		RealtimeHub: realtimeHub,
	}
}

func serveMultipartFile(router http.Handler, path, token, filename, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		panic(err)
	}
	_, _ = io.WriteString(part, content)
	_ = writer.WriteField("businessModule", "sqlite")
	if err := writer.Close(); err != nil {
		panic(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func itoa(value int64) string {
	return fmt.Sprintf("%d", value)
}
