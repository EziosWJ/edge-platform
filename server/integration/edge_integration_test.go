//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/app"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
)

func TestEdgePageContractUsesPostgresProjection(t *testing.T) {
	t.Helper()
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	edgeService, err := edge.NewService(edge.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create Edge service: %v", err)
	}
	dependencies := testDependencies(t, database, t.TempDir())
	dependencies.Edge = edgeService
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build Edge API: %v", err)
	}

	registeredAt := time.Date(2026, 9, 21, 3, 4, 5, 123456000, time.UTC)
	for _, observation := range []edge.Observation{
		{EdgeID: "edge-postgres-z", Status: edge.StatusOnline, ReceivedAt: registeredAt},
		{EdgeID: "edge-postgres-a", Status: edge.StatusOffline, ReceivedAt: registeredAt},
		{EdgeID: "edge-postgres-old", Status: edge.StatusOnline, ReceivedAt: registeredAt.Add(-time.Minute)},
	} {
		if _, err := edgeService.Observe(t.Context(), observation); err != nil {
			t.Fatalf("persist EdgeStatus %s: %v", observation.EdgeID, err)
		}
	}

	if response := serveJSON(router, http.MethodGet, "/api/edge/page", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Edge page status = %d, want 401", response.Code)
	}
	token := loginAdmin(t, router)
	page := serveJSON(router, http.MethodGet, "/api/edge/page", "", token)
	if page.Code != http.StatusOK {
		t.Fatalf("Edge page status = %d, body=%s", page.Code, page.Body.String())
	}
	var defaultPage struct {
		Data edge.Page `json:"data"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &defaultPage); err != nil {
		t.Fatalf("decode default Edge page: %v", err)
	}
	if defaultPage.Data.Page != 1 || defaultPage.Data.PageSize != 10 || defaultPage.Data.Total != 3 || len(defaultPage.Data.Records) != 3 {
		t.Fatalf("default Edge page = %+v", defaultPage.Data)
	}
	if got := []string{defaultPage.Data.Records[0].EdgeID, defaultPage.Data.Records[1].EdgeID, defaultPage.Data.Records[2].EdgeID}; !equalStrings(got, []string{"edge-postgres-a", "edge-postgres-z", "edge-postgres-old"}) {
		t.Fatalf("stable Edge order = %v", got)
	}
	if got := defaultPage.Data.Records[0].RegisteredAt; !got.Equal(registeredAt) || got.Location() != time.UTC {
		t.Fatalf("registeredAt = %v, want UTC %v", got, registeredAt)
	}

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/edge/page?page=2&pageSize=1", want: "edge-postgres-z"},
		{path: "/api/edge/page?pageSize=500", want: "edge-postgres-a"},
		{path: "/api/edge/page?status=OFFLINE", want: "edge-postgres-a"},
		{path: "/api/edge/page?status=ONLINE&edgeId=edge-postgres-z", want: "edge-postgres-z"},
	} {
		response := serveJSON(router, http.MethodGet, test.path, "", token)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"edgeId":"`+test.want+`"`) {
			t.Fatalf("GET %s status/body = %d/%s, want Edge %s", test.path, response.Code, response.Body.String(), test.want)
		}
	}

	for _, path := range []string{
		"/api/edge/page?page=0",
		"/api/edge/page?pageSize=501",
		"/api/edge/page?pageSize=abc",
		"/api/edge/page?status=online",
		"/api/edge/page?status=UNKNOWN",
	} {
		assertEnvelopeCode(t, serveJSON(router, http.MethodGet, path, "", token), http.StatusBadRequest, 400, "参数错误")
	}

	detail := serveJSON(router, http.MethodGet, "/api/edge/edge-postgres-a", "", token)
	if detail.Code != http.StatusOK {
		t.Fatalf("Edge detail status = %d, body=%s", detail.Code, detail.Body.String())
	}
	var detailPayload struct {
		Data edge.Edge `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("decode Edge detail: %v", err)
	}
	if detailPayload.Data.EdgeID != "edge-postgres-a" || detailPayload.Data.Status != edge.StatusOffline {
		t.Fatalf("Edge detail = %+v", detailPayload.Data)
	}
	if body := detail.Body.String(); strings.Contains(body, "topic") || strings.Contains(body, "payload") {
		t.Fatalf("Edge detail leaked non-domain fields: %s", body)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, "/api/edge/edge-missing", "", token), http.StatusNotFound, 404, "数据不存在")

	for _, path := range []string{"/api/edge/page", "/api/edge/edge-postgres-a"} {
		assertEnvelopeCode(t, serveJSON(router, http.MethodGet, path, "", ""), http.StatusUnauthorized, 401, "未登录或 token 已失效")
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
