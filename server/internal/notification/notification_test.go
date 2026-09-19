package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type testStore struct {
	admin     bool
	published PublishInput
}

func (s *testStore) Page(context.Context, int64, PageQuery) (Page, error) { return Page{}, nil }
func (s *testStore) Find(context.Context, int64, int64) (*Notification, error) {
	return nil, ErrNotFound
}
func (s *testStore) UnreadCount(context.Context, int64) (int64, error) { return 0, nil }
func (s *testStore) MarkRead(context.Context, int64, int64) error      { return nil }
func (s *testStore) MarkAllRead(context.Context, int64) error          { return nil }
func (s *testStore) Publish(_ context.Context, _ int64, in PublishInput, _ string) error {
	s.published = in
	return nil
}
func (s *testStore) AdminPage(context.Context, PageQuery) (Page, error) { return Page{}, nil }
func (s *testStore) IsAdmin(context.Context, int64) (bool, error)       { return s.admin, nil }
func (s *testStore) RecordRoleChange(context.Context, *gorm.DB, int64, []string, []string) error {
	return nil
}

func TestPublishRequiresAdminAndDeduplicatesRecipients(t *testing.T) {
	store := &testStore{admin: true}
	service, _ := NewService(store)
	if err := service.Publish(context.Background(), 1, PublishInput{Title: "公告", Content: "内容", UserIDs: []int64{2, 2, 3}}); err != nil {
		t.Fatalf("Publish() error=%v", err)
	}
	if len(store.published.UserIDs) != 2 {
		t.Fatalf("recipient count=%d,want 2", len(store.published.UserIDs))
	}
	store.admin = false
	if err := service.Publish(context.Background(), 1, PublishInput{Title: "公告", Content: "内容", UserIDs: []int64{2}}); err != ErrForbidden {
		t.Fatalf("non-admin error=%v,want %v", err, ErrForbidden)
	}
}

func TestRegisterRoutesDoesNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service, _ := NewService(&testStore{admin: true})
	handler, _ := NewHandler(service)
	RegisterRoutes(router, handler)
}

func TestPublishRouteRejectsNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service, _ := NewService(&testStore{admin: false})
	handler, _ := NewHandler(service)
	RegisterRoutes(router, handler)
	request := httptest.NewRequest(http.MethodPost, "/notification", strings.NewReader(`{"title":"公告","content":"内容","userIds":[1]}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.ContextWithPrincipal(request.Context(), auth.Principal{UserID: 9}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d,want %d", response.Code, http.StatusForbidden)
	}
}

func TestPageRoutesReturnEmptyRecordsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/notification/page", "/notification-admin/page"} {
		t.Run(path, func(t *testing.T) {
			router := gin.New()
			service, _ := NewService(&testStore{admin: true})
			handler, _ := NewHandler(service)
			RegisterRoutes(router, handler)
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request = request.WithContext(auth.ContextWithPrincipal(request.Context(), auth.Principal{UserID: 9}))
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status=%d,want %d", response.Code, http.StatusOK)
			}
			var body struct {
				Data struct {
					Records json.RawMessage `json:"records"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if string(body.Data.Records) != "[]" {
				t.Fatalf("records=%s,want []", body.Data.Records)
			}
		})
	}
}

func TestNotificationCreateDoesNotWriteQueryOnlyColumns(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	statement := db.Create(&Notification{Title: "公告", Content: "内容", SourceType: SourceManual}).Statement
	if statement.Error != nil {
		t.Fatalf("build insert: %v", statement.Error)
	}
	for _, column := range []string{"is_read", "recipient_count", "read_count"} {
		if strings.Contains(statement.SQL.String(), `"`+column+`"`) {
			t.Fatalf("insert writes query-only column %s: %s", column, statement.SQL.String())
		}
	}
}
