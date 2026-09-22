package datapoint

import (
	"context"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:datapoint-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE edge (edge_id TEXT PRIMARY KEY)`,
		`CREATE TABLE device (device_id TEXT PRIMARY KEY, edge_id TEXT, source_device_id TEXT)`,
		`CREATE TABLE data_point (data_point_id TEXT PRIMARY KEY, device_id TEXT, point_key TEXT, name TEXT, value_type TEXT, unit TEXT, precision INTEGER, enabled BOOLEAN, config_generation INTEGER, effective_at DATETIME)`,
		`CREATE TABLE source_mapping (data_point_id TEXT PRIMARY KEY, source_type TEXT, function_code INTEGER, address INTEGER, encoding TEXT, word_order TEXT, byte_order TEXT, bit_index INTEGER, scale REAL, offset REAL)`,
		`CREATE TABLE current_value (data_point_id TEXT PRIMARY KEY, value_type TEXT, number_value REAL, boolean_value BOOLEAN, quality TEXT, source_timestamp DATETIME, observed_at DATETIME, revision INTEGER)`,
		`CREATE TABLE sys_oper_log (id INTEGER PRIMARY KEY AUTOINCREMENT, module_name TEXT, operation_type TEXT, request_id TEXT, request_method TEXT, request_url TEXT, operator_id INTEGER, operator_ip TEXT, user_agent TEXT, operation_status TEXT, operation_time DATETIME, create_time DATETIME)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO edge(edge_id) VALUES ('edge-1'); INSERT INTO device(device_id,edge_id,source_device_id) VALUES ('cloud-1','edge-1','source-1')`).Error; err != nil {
		t.Fatal(err)
	}
	return NewRepository(db)
}

func testInput() CreateInput {
	return CreateInput{DeviceID: "cloud-1", PointKey: "temperature", Name: "Temperature", ValueType: ValueTypeNumber, Enabled: true, Mapping: SourceMapping{SourceType: "MODBUS_REGISTER", FunctionCode: 3, Address: 100, Encoding: EncodingUINT16, ByteOrder: ByteOrderBigEndian, Scale: 1, Offset: 0}}
}
func rawAt(at time.Time, value uint16, valid bool) RawObservation {
	return RawObservation{EdgeID: "edge-1", SourceDeviceID: "source-1", ReceivedAt: at, Snapshot: RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: valid, LastSuccessAt: &at, Registers: []RawRegister{{Address: intPtr(100), Value: &value}}}}}}
}

func TestRepositoryCurrentValueOrderingBadRetentionAndConfigGuard(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC().Add(time.Second)
	if err := r.Project(ctx, rawAt(first, 123, true)); err != nil {
		t.Fatal(err)
	}
	got, err := r.Detail(ctx, point.DataPointID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current.Quality != QualityGood || got.Current.Value().(float64) != 123 || got.Current.Revision != 1 {
		t.Fatalf("good current = %+v", got.Current)
	}
	if err := r.Project(ctx, rawAt(first.Add(time.Second), 0, false)); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Quality != QualityBad || got.Current.Value().(float64) != 123 || got.Current.Revision != 2 {
		t.Fatalf("bad current = %+v", got.Current)
	}
	if err := r.Project(ctx, rawAt(first, 999, true)); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Value().(float64) != 123 || got.Current.Revision != 2 {
		t.Fatalf("older observation applied = %+v", got.Current)
	}
	changed := testInput().Mapping
	changed.Address = 101
	reset, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Temperature", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	if reset.Current.Quality != QualityNoData || reset.Current.Value() != nil || reset.Current.Revision != 3 {
		t.Fatalf("reset current = %+v", reset.Current)
	}
	preChange := rawAt(reset.EffectiveAt.Add(-time.Nanosecond), 5, true)
	if err := r.Project(ctx, preChange); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Quality != QualityNoData {
		t.Fatalf("pre-change raw repopulated = %+v", got.Current)
	}
	postChange := rawAt(reset.EffectiveAt.Add(time.Second), 8, true)
	postChange.Snapshot.Blocks[0].Registers[0].Address = intPtr(101)
	if err := r.Project(ctx, postChange); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Quality != QualityGood || got.Current.Value().(float64) != 8 {
		t.Fatalf("post-change current = %+v", got.Current)
	}
}
