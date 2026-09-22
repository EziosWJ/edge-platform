//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/app"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
	"gorm.io/gorm"
)

func TestDataPointManagementAndCurrentValueProjection(t *testing.T) {
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	database := openTemporaryDatabase(t, temporary.dsn)
	defer func() { _ = database.Close() }()
	edgeService, _ := edge.NewService(edge.NewRepository(database.GORM))
	deviceService, _ := device.NewService(device.NewRepository(database.GORM))
	if _, err := edgeService.Observe(t.Context(), edge.Observation{EdgeID: "m4-edge", Status: edge.StatusOnline, ReceivedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := deviceService.Observe(t.Context(), device.Observation{EdgeID: "m4-edge", SourceDeviceID: "m4-source", Snapshot: device.Snapshot{CommunicationStatus: device.StatusOnline}, ReceivedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	deps := testDependencies(t, database, t.TempDir())
	deps.Edge, deps.Device = edgeService, deviceService
	router, err := app.Build(testAPIConfig(), database, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, "/api/datapoint/page", "", ""))
	token := loginAdmin(t, router)
	body := `{"deviceId":"m4-source-does-not-exist","pointKey":"temperature","name":"Temperature","valueType":"NUMBER","mapping":{"sourceType":"MODBUS_REGISTER","functionCode":3,"address":100,"encoding":"UINT16","byteOrder":"BIG_ENDIAN","wordOrder":null,"bitIndex":null,"scale":1,"offset":0}}`
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/datapoint", body, token), http.StatusNotFound, 404, "数据不存在")
	body = `{"deviceId":"` + deviceIDFor(t, database.GORM) + `","pointKey":"temperature","name":"Temperature","valueType":"NUMBER","unit":"A","precision":1,"mapping":{"sourceType":"MODBUS_REGISTER","functionCode":3,"address":100,"encoding":"UINT16","byteOrder":"BIG_ENDIAN","wordOrder":null,"bitIndex":null,"scale":0.1,"offset":0}}`
	create := serveJSON(router, http.MethodPost, "/api/datapoint", body, token)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			DataPointID string `json:"dataPointId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.DataPointID == "" {
		t.Fatal("missing datapoint id")
	}
	if err := deps.DataPoint.Project(t.Context(), datapoint.RawObservation{EdgeID: "m4-edge", SourceDeviceID: "m4-source", ReceivedAt: time.Now().UTC().Add(time.Second), Snapshot: datapoint.RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []datapoint.RawBlock{{FunctionCode: 3, Valid: true, LastSuccessAt: timePtr(time.Now().UTC()), Registers: []datapoint.RawRegister{{Address: intPtr(100), Value: uint16Ptr(123)}}}}}}); err != nil {
		t.Fatal(err)
	}
	detail := serveJSON(router, http.MethodGet, "/api/datapoint/"+created.Data.DataPointID, "", token)
	if detail.Code != http.StatusOK || !contains(detail.Body.String(), `"value":12.3`) || !contains(detail.Body.String(), `"quality":"GOOD"`) {
		t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
	}
}

// deviceIDFor reads the stable Cloud device identity created above.
func deviceIDFor(t *testing.T, db *gorm.DB) string {
	var row struct {
		DeviceID string `gorm:"column:device_id"`
	}
	if err := db.Table("device").Where("source_device_id = ?", "m4-source").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.DeviceID
}
func timePtr(v time.Time) *time.Time     { return &v }
func intPtr(v int) *int                  { return &v }
func uint16Ptr(v uint16) *uint16         { return &v }
func contains(value, needle string) bool { return strings.Contains(value, needle) }
