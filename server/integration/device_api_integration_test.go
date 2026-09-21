//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/app"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
)

func TestDeviceAPIUsesAuthenticatedStableProjectionQueries(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()

	edgeService, err := edge.NewService(edge.NewRepository(database.GORM))
	if err != nil {
		t.Fatal(err)
	}
	deviceService, err := device.NewService(device.NewRepository(database.GORM))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 21, 5, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		id     string
		status edge.Status
	}{
		{id: "edge-api-a", status: edge.StatusOffline},
		{id: "edge-api-b", status: edge.StatusOnline},
	} {
		if _, err := edgeService.Observe(t.Context(), edge.Observation{EdgeID: item.id, Status: item.status, ReceivedAt: base}); err != nil {
			t.Fatal(err)
		}
	}

	devices := make([]device.Device, 0, 3)
	for index, item := range []struct {
		edgeID string
		source string
		status device.CommunicationStatus
	}{
		{edgeID: "edge-api-a", source: "source-z", status: device.StatusOffline},
		{edgeID: "edge-api-a", source: "source-a", status: device.StatusOnline},
		{edgeID: "edge-api-b", source: "source-shared", status: device.StatusDegraded},
	} {
		receivedAt := base.Add(time.Duration(index) * time.Minute)
		attempt := receivedAt.Add(-time.Second)
		result, err := deviceService.Observe(t.Context(), device.Observation{
			EdgeID: item.edgeID, SourceDeviceID: item.source,
			Snapshot:   device.Snapshot{CommunicationStatus: item.status, LastAttemptAt: &attempt},
			ReceivedAt: receivedAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		devices = append(devices, result)
	}

	dependencies := testDependencies(t, database, t.TempDir())
	dependencies.Device = deviceService
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build Device API: %v", err)
	}
	if response := serveJSON(router, http.MethodGet, "/api/device/page", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Device page status = %d", response.Code)
	}
	token := loginAdmin(t, router)

	pageResponse := serveJSON(router, http.MethodGet, "/api/device/page?pageSize=10", "", token)
	if pageResponse.Code != http.StatusOK {
		t.Fatalf("Device page status = %d, body=%s", pageResponse.Code, pageResponse.Body.String())
	}
	var pagePayload struct {
		Data device.Page `json:"data"`
	}
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &pagePayload); err != nil {
		t.Fatal(err)
	}
	if pagePayload.Data.Total != 3 || len(pagePayload.Data.Records) != 3 || pagePayload.Data.Page != 1 || pagePayload.Data.PageSize != 10 {
		t.Fatalf("Device page = %+v", pagePayload.Data)
	}
	for index := 1; index < len(pagePayload.Data.Records); index++ {
		previous := pagePayload.Data.Records[index-1]
		current := pagePayload.Data.Records[index]
		if previous.RegisteredAt.Before(current.RegisteredAt) || (previous.RegisteredAt.Equal(current.RegisteredAt) && previous.DeviceID > current.DeviceID) {
			t.Fatalf("Device order is not registeredAt DESC, deviceId ASC: %+v", pagePayload.Data.Records)
		}
	}

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/device/page?edgeId=edge-api-a", want: "edge-api-a"},
		{path: "/api/device/page?sourceDeviceId=source-shared", want: "source-shared"},
		{path: "/api/device/page?status=OFFLINE", want: "OFFLINE"},
		{path: "/api/device/page?deviceId=" + devices[0].DeviceID, want: devices[0].DeviceID},
		{path: "/api/device/page?page=2&pageSize=1", want: ""},
	} {
		response := serveJSON(router, http.MethodGet, test.path, "", token)
		if response.Code != http.StatusOK || (test.want != "" && !strings.Contains(response.Body.String(), test.want)) {
			t.Fatalf("GET %s status/body = %d/%s, want %q", test.path, response.Code, response.Body.String(), test.want)
		}
	}
	for _, path := range []string{
		"/api/device/page?page=0",
		"/api/device/page?pageSize=501",
		"/api/device/page?status=online",
		"/api/device/page?sourceDeviceId=",
	} {
		assertEnvelopeCode(t, serveJSON(router, http.MethodGet, path, "", token), http.StatusBadRequest, 400, "参数错误")
	}

	detail := serveJSON(router, http.MethodGet, "/api/device/"+devices[0].DeviceID, "", token)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"sourceDeviceId":"source-z"`) {
		t.Fatalf("Device detail status/body = %d/%s", detail.Code, detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), "topic") || strings.Contains(detail.Body.String(), "payload") {
		t.Fatalf("Device detail leaked non-domain fields: %s", detail.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, "/api/device/missing-device", "", token), http.StatusNotFound, 404, "数据不存在")
	for _, path := range []string{"/api/device/page", "/api/device/" + devices[0].DeviceID} {
		assertEnvelopeCode(t, serveJSON(router, http.MethodGet, path, "", ""), http.StatusUnauthorized, 401, "未登录或 token 已失效")
	}
}
