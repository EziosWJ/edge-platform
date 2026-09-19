//go:build integration

package integration

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/usermgmt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type queryTraceCounter struct {
	logger.Interface

	mu      sync.Mutex
	queries []string
}

func (c *queryTraceCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.mu.Lock()
	c.queries = append(c.queries, sql)
	c.mu.Unlock()
}

func (c *queryTraceCounter) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queries)
}

func TestUserPageLoadsDepartmentAndRolesInBatches(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	if err := database.GORM.Exec(`
		INSERT INTO sys_dept (id, parent_id, dept_name, dept_code, status, is_builtin)
		VALUES (2, 1, '研发部', 'BATCH-RND', 1, 0);
		INSERT INTO sys_role (id, role_name, role_code, status, sort_order, is_builtin)
		VALUES (2, '运营', 'BATCH-OPS', 1, 2, 0);
		INSERT INTO sys_user (id, username, nickname, password, gender, dept_id, status, is_builtin)
		VALUES
			(2, 'batch-user-1', '批量用户一', 'password', 'UNSPECIFIED', 2, 1, 0),
			(3, 'batch-user-2', '批量用户二', 'password', 'UNSPECIFIED', 2, 1, 0);
		INSERT INTO sys_user_role (id, user_id, role_id)
		VALUES (2, 2, 2), (3, 3, 1);
	`).Error; err != nil {
		t.Fatalf("seed batch user page fixture: %v", err)
	}

	counter := &queryTraceCounter{Interface: logger.Discard}
	repository := usermgmt.NewRepository(database.GORM.Session(&gorm.Session{Logger: counter}))
	page, err := repository.Page(context.Background(), usermgmt.PageQuery{
		Page: 1, PageSize: 500, Username: "batch-user-",
	})
	if err != nil {
		t.Fatalf("load user page: %v", err)
	}
	if counter.Count() != 4 {
		t.Fatalf("user page executed %d SQL queries, want 4 (count, users, departments, roles)", counter.Count())
	}
	if page.Total != 2 || len(page.Records) != 2 {
		t.Fatalf("page = total %d, records %d; want 2, 2", page.Total, len(page.Records))
	}
	wantRoleCodes := map[string]string{
		"batch-user-1": "BATCH-OPS",
		"batch-user-2": "ADMIN",
	}
	for _, user := range page.Records {
		if user.DeptName == nil || *user.DeptName != "研发部" {
			t.Fatalf("user %d department = %v, want 研发部", user.ID, user.DeptName)
		}
		if len(user.Roles) != 1 || user.Roles[0].RoleCode != wantRoleCodes[user.Username] {
			t.Fatalf("user %d roles = %+v, want one role", user.ID, user.Roles)
		}
	}
}
