package datapoint

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingNotifier struct {
	mu      sync.Mutex
	changes []CurrentValueChange
}

func (n *recordingNotifier) TryPublish(change CurrentValueChange) bool {
	n.mu.Lock()
	n.changes = append(n.changes, change)
	n.mu.Unlock()
	return true
}

func (n *recordingNotifier) snapshot() []CurrentValueChange {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]CurrentValueChange(nil), n.changes...)
}

type committedBatchNotifier struct {
	db             *gorm.DB
	allRowsVisible bool
	count          int
}

func (n *committedBatchNotifier) TryPublish(change CurrentValueChange) bool {
	var revisions []int64
	n.db.Table("current_value").Order("data_point_id").Pluck("revision", &revisions)
	n.allRowsVisible = len(revisions) == 2 && revisions[0] == 1 && revisions[1] == 1
	n.count++
	return true
}

type committedCurrentNotifier struct {
	db     *gorm.DB
	states []struct {
		quality  Quality
		revision int64
		enabled  bool
	}
	firstErr error
}

func (n *committedCurrentNotifier) TryPublish(change CurrentValueChange) bool {
	var current struct {
		Quality  Quality `gorm:"column:quality"`
		Revision int64   `gorm:"column:revision"`
	}
	if err := n.db.Table("current_value").Where("data_point_id = ?", change.DataPointID).Take(&current).Error; err != nil {
		n.firstErr = err
		return false
	}
	var point struct {
		Enabled bool `gorm:"column:enabled"`
	}
	if err := n.db.Table("data_point").Where("data_point_id = ?", change.DataPointID).Take(&point).Error; err != nil {
		n.firstErr = err
		return false
	}
	if current.Quality != change.Quality || current.Revision != change.Revision {
		n.firstErr = fmt.Errorf("notification observed quality=%s revision=%d, payload quality=%s revision=%d", current.Quality, current.Revision, change.Quality, change.Revision)
		return false
	}
	n.states = append(n.states, struct {
		quality  Quality
		revision int64
		enabled  bool
	}{quality: current.Quality, revision: current.Revision, enabled: point.Enabled})
	return true
}

func testRepository(t *testing.T) *Repository {
	return testRepositoryWithNotifier(t, nil)
}

func testRepositoryWithNotifier(t *testing.T, notifier CurrentValueNotifier) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:datapoint-test-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
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
	if notifier == nil {
		return NewRepository(db)
	}
	return NewRepository(db, notifier)
}

func testInput() CreateInput {
	return CreateInput{DeviceID: "cloud-1", PointKey: "temperature", Name: "Temperature", ValueType: ValueTypeNumber, Enabled: true, Mapping: SourceMapping{SourceType: "MODBUS_REGISTER", FunctionCode: 3, Address: 100, Encoding: EncodingUINT16, ByteOrder: ByteOrderBigEndian, Scale: 1, Offset: 0}}
}
func rawAt(at time.Time, value uint16, valid bool) RawObservation {
	return RawObservation{EdgeID: "edge-1", SourceDeviceID: "source-1", ReceivedAt: at, Snapshot: RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: valid, LastSuccessAt: &at, Registers: []RawRegister{{Address: intPtr(100), Value: &value}}}}}}
}

func TestRepositoryCurrentValueOrderingBadRetentionAndConfigGuard(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
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
	changes := notifier.snapshot()
	if len(changes) != 3 {
		t.Fatalf("notifications after reset = %d, want 3", len(changes))
	}
	if resetChange := changes[2]; resetChange.Quality != QualityNoData || resetChange.Value != nil || resetChange.SourceTimestamp != nil || resetChange.ObservedAt != nil || resetChange.Revision != 3 {
		t.Fatalf("reset notification = %+v", resetChange)
	}

	// This raw was received before the configuration transaction became
	// effective, but its handler is deliberately delayed until after the
	// reset commits. It must be rejected by EffectiveAt and must not emit a
	// live update.
	preChange := rawAt(reset.EffectiveAt.Add(-time.Nanosecond), 5, true)
	if err := r.Project(ctx, preChange); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Quality != QualityNoData {
		t.Fatalf("pre-change raw repopulated = %+v", got.Current)
	}
	if got := notifier.snapshot(); len(got) != 3 {
		t.Fatalf("stale raw notifications = %d, want 3", len(got))
	}

	// Only an observation received after EffectiveAt may form a new value
	// using the new mapping.
	postChange := rawAt(reset.EffectiveAt.Add(time.Second), 8, true)
	postChange.Snapshot.Blocks[0].Registers[0].Address = intPtr(101)
	if err := r.Project(ctx, postChange); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Detail(ctx, point.DataPointID)
	if got.Current.Quality != QualityGood || got.Current.Value().(float64) != 8 {
		t.Fatalf("post-change current = %+v", got.Current)
	}
	changes = notifier.snapshot()
	if len(changes) != 4 || changes[3].Quality != QualityGood || changes[3].Value != float64(8) || changes[3].Revision != 4 {
		t.Fatalf("post-change notification = %+v", changes)
	}
}

func TestRepositoryDoesNotUseStaleMappingAfterConfigCommit(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}

	// Model a raw handler that had already loaded the old mapping before the
	// configuration transaction committed. The repository must resolve the
	// mapping in its projection transaction rather than accepting that stale
	// snapshot as authority.
	var stale SourceMapping
	if err := r.db.Where("data_point_id = ?", point.DataPointID).Take(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if stale.Address != 100 {
		t.Fatalf("stale mapping address = %d, want 100", stale.Address)
	}

	changed := testInput().Mapping
	changed.Address = 101
	reset, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Temperature", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}

	// Both addresses are present so a projection using the stale mapping
	// would produce 5. A projection after the config commit must use address
	// 101 and produce 8 instead.
	receivedAt := reset.EffectiveAt.Add(time.Second)
	oldValue, newValue := uint16(5), uint16(8)
	snapshot := RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{
		FunctionCode: 3, Valid: true, LastSuccessAt: &receivedAt,
		Registers: []RawRegister{{Address: intPtr(100), Value: &oldValue}, {Address: intPtr(101), Value: &newValue}},
	}}}
	if staleProjection := project(stale, snapshot); staleProjection.Value != float64(5) {
		t.Fatalf("stale mapping projection = %+v, want value 5", staleProjection)
	}
	if err := r.Project(ctx, RawObservation{
		EdgeID: "edge-1", SourceDeviceID: "source-1", ReceivedAt: receivedAt,
		Snapshot: snapshot,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := r.Detail(ctx, point.DataPointID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current.Quality != QualityGood || got.Current.Value() != float64(8) || got.Current.Revision != reset.Current.Revision+1 {
		t.Fatalf("projection after config commit = %+v, want value 8 revision %d", got.Current, reset.Current.Revision+1)
	}
	changes := notifier.snapshot()
	if len(changes) != 2 || changes[0].Quality != QualityNoData || changes[1].Value != float64(8) || changes[1].Revision != reset.Current.Revision+1 {
		t.Fatalf("stale mapping notifications = %+v", changes)
	}
}

func TestRepositoryResetNotificationsObserveCommittedState(t *testing.T) {
	notifier := &committedCurrentNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	notifier.db = r.db
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	changed := testInput().Mapping
	changed.Address = 101
	if _, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Temperature", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, false, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, true, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if notifier.firstErr != nil {
		t.Fatal(notifier.firstErr)
	}
	if len(notifier.states) != 3 {
		t.Fatalf("committed reset notifications = %+v, want 3", notifier.states)
	}
	want := []struct {
		quality  Quality
		revision int64
		enabled  bool
	}{{QualityNoData, 1, true}, {QualityNoData, 2, false}, {QualityNoData, 3, true}}
	for index, state := range notifier.states {
		if state != want[index] {
			t.Fatalf("committed notification %d = %+v, want %+v", index, state, want[index])
		}
	}
}

func TestCurrentValueChangePayloadHasOnlySemanticFields(t *testing.T) {
	typeOfChange := reflect.TypeOf(CurrentValueChange{})
	want := []string{"DataPointID", "DeviceID", "PointKey", "ValueType", "Value", "Quality", "SourceTimestamp", "ObservedAt", "Revision"}
	if typeOfChange.NumField() != len(want) {
		t.Fatalf("CurrentValueChange fields = %d, want %d", typeOfChange.NumField(), len(want))
	}
	for index, name := range want {
		if typeOfChange.Field(index).Name != name {
			t.Fatalf("CurrentValueChange field %d = %q, want %q", index, typeOfChange.Field(index).Name, name)
		}
	}
}

func TestRepositoryPublishesRawGoodAndBadAfterCommit(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC().Add(time.Second)
	if err := r.Project(ctx, rawAt(first, 123, true)); err != nil {
		t.Fatal(err)
	}
	if err := r.Project(ctx, rawAt(first.Add(time.Second), 0, false)); err != nil {
		t.Fatal(err)
	}

	changes := notifier.snapshot()
	if len(changes) != 2 {
		t.Fatalf("notifications = %d, want 2", len(changes))
	}
	good, bad := changes[0], changes[1]
	if good.Quality != QualityGood || good.Value != float64(123) || good.Revision != 1 || good.ObservedAt == nil {
		t.Fatalf("good change = %+v", good)
	}
	if bad.Quality != QualityBad || bad.Value != float64(123) || bad.Revision != 2 || bad.SourceTimestamp == nil || bad.ObservedAt == nil {
		t.Fatalf("bad change = %+v", bad)
	}
	if bad.DataPointID != point.DataPointID || bad.DeviceID != "cloud-1" || bad.PointKey != "temperature" || bad.ValueType != ValueTypeNumber {
		t.Fatalf("bad identity = %+v", bad)
	}
}

func TestRepositoryPublishesBooleanGoodAndBadAfterCommit(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	ctx := context.Background()
	bitIndex := 0
	input := testInput()
	input.PointKey = "breaker_closed"
	input.ValueType = ValueTypeBoolean
	input.Mapping = SourceMapping{SourceType: "MODBUS_REGISTER", FunctionCode: 3, Address: 100, Encoding: EncodingBooleanBit, ByteOrder: ByteOrderBigEndian, BitIndex: &bitIndex, Scale: 1, Offset: 0}
	if _, err := r.Create(ctx, input, audit.Event{Action: "datapoint.create", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC().Add(time.Second)
	if err := r.Project(ctx, rawAt(first, 1, true)); err != nil {
		t.Fatal(err)
	}
	if err := r.Project(ctx, rawAt(first.Add(time.Second), 0, false)); err != nil {
		t.Fatal(err)
	}
	changes := notifier.snapshot()
	if len(changes) != 2 || changes[0].Value != true || changes[0].ValueType != ValueTypeBoolean || changes[0].Quality != QualityGood {
		t.Fatalf("boolean good notifications = %+v", changes)
	}
	if changes[1].Value != true || changes[1].Quality != QualityBad || changes[1].Revision != 2 {
		t.Fatalf("boolean bad notification = %+v", changes[1])
	}
}

func TestRepositoryPublishesSemanticResetButNotMetadataOrIdempotentChanges(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Renamed", Mapping: testInput().Mapping}, audit.Event{Action: "datapoint.update", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if len(notifier.snapshot()) != 0 {
		t.Fatal("metadata-only update emitted a notification")
	}
	changed := testInput().Mapping
	changed.Address = 101
	reset, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Renamed", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	changes := notifier.snapshot()
	if len(changes) != 1 || changes[0].Quality != QualityNoData || changes[0].Value != nil || changes[0].Revision != reset.Current.Revision {
		t.Fatalf("mapping reset notifications = %+v, reset = %+v", changes, reset.Current)
	}
	if _, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Renamed", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if len(notifier.snapshot()) != 1 {
		t.Fatal("idempotent mapping update emitted a notification")
	}
	metadataRaw := rawAt(reset.EffectiveAt.Add(time.Second), 42, true)
	metadataRaw.Snapshot.Blocks[0].Registers[0].Address = intPtr(101)
	if err := r.Project(ctx, metadataRaw); err != nil {
		t.Fatal(err)
	}
	beforeMetadata := notifier.snapshot()
	if _, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Metadata only", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	afterMetadata := notifier.snapshot()
	if len(afterMetadata) != len(beforeMetadata) {
		t.Fatalf("metadata-only update notifications = %d, want %d", len(afterMetadata), len(beforeMetadata))
	}
	current, err := r.CurrentByIdentity(ctx, "cloud-1", "temperature")
	if err != nil {
		t.Fatal(err)
	}
	if current.Quality != QualityGood || current.Value != float64(42) {
		t.Fatalf("metadata-only update changed current = %+v", current)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, false, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, false, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, true, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	changes = notifier.snapshot()
	if len(changes) != 4 || changes[2].Quality != QualityNoData || changes[3].Quality != QualityNoData {
		t.Fatalf("enable transitions = %+v", changes)
	}
	for _, index := range []int{2, 3} {
		if changes[index].Value != nil || changes[index].SourceTimestamp != nil || changes[index].ObservedAt != nil {
			t.Fatalf("NO_DATA transition payload %d = %+v", index, changes[index])
		}
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, true, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err != nil {
		t.Fatal(err)
	}
	if len(notifier.snapshot()) != 4 {
		t.Fatalf("idempotent enabled=true notifications = %d, want 4", len(notifier.snapshot()))
	}
}

func TestRepositoryDoesNotPublishWhenTransactionRollsBack(t *testing.T) {
	notifier := &recordingNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	ctx := context.Background()
	point, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	changed := testInput().Mapping
	changed.Address = 101
	if err := r.db.Exec("DROP TABLE sys_oper_log").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := r.Update(ctx, point.DataPointID, UpdateInput{Name: "Temperature", Mapping: changed}, audit.Event{Action: "datapoint.update", Resource: "datapoint"}); err == nil {
		t.Fatal("Update() succeeded without audit table")
	}
	if got := notifier.snapshot(); len(got) != 0 {
		t.Fatalf("rollback notifications = %+v, want none", got)
	}
	current, err := r.CurrentByIdentity(ctx, "cloud-1", "temperature")
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 0 || current.Quality != QualityNoData {
		t.Fatalf("rolled back current = %+v", current)
	}
	if _, err := r.SetEnabled(ctx, point.DataPointID, false, audit.Event{Action: "datapoint.enabled", Resource: "datapoint"}); err == nil {
		t.Fatal("SetEnabled() succeeded without audit table")
	}
	if got := notifier.snapshot(); len(got) != 0 {
		t.Fatalf("SetEnabled rollback notifications = %+v, want none", got)
	}
	var enabled bool
	if err := r.db.Table("data_point").Where("data_point_id = ?", point.DataPointID).Pluck("enabled", &enabled).Error; err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("rolled back SetEnabled changed enabled state")
	}
}

func TestRepositoryPublishesBatchOnlyAfterAllRowsCommit(t *testing.T) {
	notifier := &committedBatchNotifier{}
	r := testRepositoryWithNotifier(t, notifier)
	notifier.db = r.db
	ctx := context.Background()
	first, err := r.Create(ctx, testInput(), audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := testInput()
	secondInput.PointKey = "humidity"
	second, err := r.Create(ctx, secondInput, audit.Event{Action: "datapoint.create", Resource: "datapoint"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(time.Second)
	if err := r.Project(ctx, RawObservation{
		EdgeID: "edge-1", SourceDeviceID: "source-1", ReceivedAt: at,
		Snapshot: RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{
			FunctionCode: 3, Valid: true, LastSuccessAt: &at,
			Registers: []RawRegister{{Address: intPtr(100), Value: uint16Ptr(123)}, {Address: intPtr(101), Value: uint16Ptr(45)}},
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	if notifier.count != 2 || !notifier.allRowsVisible {
		t.Fatalf("batch notifications count=%d allRowsVisible=%v", notifier.count, notifier.allRowsVisible)
	}
	for _, id := range []string{first.DataPointID, second.DataPointID} {
		current, err := r.Detail(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if current.Current.Revision != 1 {
			t.Fatalf("point %s revision = %d, want 1", id, current.Current.Revision)
		}
	}
}
