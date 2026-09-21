package device

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "device.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	if err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE edge (
    edge_id TEXT NOT NULL PRIMARY KEY,
    status TEXT NOT NULL,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL
);
CREATE TABLE device (
    device_id TEXT NOT NULL PRIMARY KEY,
    edge_id TEXT NOT NULL,
    source_device_id TEXT NOT NULL,
    communication_status TEXT NOT NULL,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    last_attempt_at DATETIME,
    last_success_at DATETIME,
    communication_error TEXT,
    FOREIGN KEY (edge_id) REFERENCES edge(edge_id),
    UNIQUE (edge_id, source_device_id)
)`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func insertTestEdge(t *testing.T, db *gorm.DB, edgeID string) {
	t.Helper()
	at := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	if err := db.Exec(`INSERT INTO edge (edge_id, status, registered_at, last_seen_at) VALUES (?, ?, ?, ?)`, edgeID, "ONLINE", at, at).Error; err != nil {
		t.Fatal(err)
	}
}

func snapshot(status CommunicationStatus, attempt, success time.Time, diagnostic *string) Snapshot {
	return Snapshot{
		CommunicationStatus: status,
		LastAttemptAt:       &attempt,
		LastSuccessAt:       &success,
		CommunicationError:  diagnostic,
	}
}

func TestRepositoryObserveDiscoversAllFourStatesAndKeepsSourceIdentity(t *testing.T) {
	db := openRepositoryTestDB(t)
	repository := NewRepository(db)
	insertTestEdge(t, db, "edge-01")
	base := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)

	for _, status := range []CommunicationStatus{StatusInitial, StatusOnline, StatusDegraded, StatusOffline} {
		got, err := repository.Observe(context.Background(), Observation{
			EdgeID:         "edge-01",
			SourceDeviceID: "source-" + string(status),
			Snapshot:       snapshot(status, base, base, nil),
			ReceivedAt:     base,
		})
		if err != nil {
			t.Fatalf("discover %s: %v", status, err)
		}
		if _, err := uuid.Parse(got.DeviceID); err != nil {
			t.Fatalf("deviceId %q is not a UUID: %v", got.DeviceID, err)
		}
		if got.EdgeID != "edge-01" || got.CommunicationStatus != status || got.SourceDeviceID != "source-"+string(status) {
			t.Fatalf("projection for %s = %+v", status, got)
		}
	}
}

func TestRepositoryUnknownParentDoesNotCreateEdgeOrDevice(t *testing.T) {
	db := openRepositoryTestDB(t)
	repository := NewRepository(db)
	at := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	_, err := repository.Observe(context.Background(), Observation{
		EdgeID:         "unknown-edge",
		SourceDeviceID: "source-01",
		Snapshot:       snapshot(StatusOnline, at, at, nil),
		ReceivedAt:     at,
	})
	if !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("unknown parent error = %v, want ErrParentNotFound", err)
	}
	var edges, devices int64
	if err := db.Table("edge").Count(&edges).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("device").Count(&devices).Error; err != nil {
		t.Fatal(err)
	}
	if edges != 0 || devices != 0 {
		t.Fatalf("unknown parent created rows: edges=%d devices=%d", edges, devices)
	}
}

func TestRepositorySameSourceDeviceUnderDifferentEdgesGetsIndependentUUIDs(t *testing.T) {
	db := openRepositoryTestDB(t)
	repository := NewRepository(db)
	insertTestEdge(t, db, "edge-a")
	insertTestEdge(t, db, "edge-b")
	at := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	left, err := repository.Observe(context.Background(), Observation{EdgeID: "edge-a", SourceDeviceID: "same-source", Snapshot: snapshot(StatusOnline, at, at, nil), ReceivedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	right, err := repository.Observe(context.Background(), Observation{EdgeID: "edge-b", SourceDeviceID: "same-source", Snapshot: snapshot(StatusOnline, at, at, nil), ReceivedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if left.DeviceID == right.DeviceID || left.EdgeID == right.EdgeID {
		t.Fatalf("independent source identities collapsed: left=%+v right=%+v", left, right)
	}
}

func TestRepositoryObserveKeepsEarliestRegistrationAndNewestAtomicSnapshot(t *testing.T) {
	db := openRepositoryTestDB(t)
	repository := NewRepository(db)
	insertTestEdge(t, db, "edge-01")
	earlier := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	later := earlier.Add(2 * time.Minute)
	oldAttempt := earlier.Add(-time.Hour)
	newAttempt := later.Add(-time.Hour)

	created, err := repository.Observe(context.Background(), Observation{
		EdgeID: "edge-01", SourceDeviceID: "source-01",
		Snapshot: snapshot(StatusOffline, newAttempt, newAttempt, stringPointer("offline")), ReceivedAt: later,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repository.Observe(context.Background(), Observation{
		EdgeID: "edge-01", SourceDeviceID: "source-01",
		Snapshot: snapshot(StatusOnline, oldAttempt, oldAttempt, stringPointer("older")), ReceivedAt: earlier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DeviceID != created.DeviceID || !updated.RegisteredAt.Equal(earlier) || !updated.LastSeenAt.Equal(later) || updated.CommunicationStatus != StatusOffline || *updated.CommunicationError != "offline" {
		t.Fatalf("older observation regressed projection: %+v", updated)
	}

	equalAttempt := later.Add(time.Minute)
	updated, err = repository.Observe(context.Background(), Observation{
		EdgeID: "edge-01", SourceDeviceID: "source-01",
		Snapshot: snapshot(StatusDegraded, equalAttempt, equalAttempt, stringPointer("equal")), ReceivedAt: later,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CommunicationStatus != StatusDegraded || !updated.LastSeenAt.Equal(later) || *updated.CommunicationError != "equal" {
		t.Fatalf("equal observation did not win by statement order: %+v", updated)
	}
}

func TestRepositoryConcurrentFirstDiscoveryLeavesOneCloudDevice(t *testing.T) {
	db := openRepositoryTestDB(t)
	repository := NewRepository(db)
	insertTestEdge(t, db, "edge-concurrent")
	base := time.Date(2026, 9, 21, 3, 0, 0, 0, time.UTC)
	const workers = 12
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	ids := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			result, err := repository.Observe(context.Background(), Observation{
				EdgeID: "edge-concurrent", SourceDeviceID: "source-concurrent",
				Snapshot: snapshot(StatusOnline, base, base, nil), ReceivedAt: base.Add(time.Duration(index) * time.Second),
			})
			if err == nil {
				ids <- result.DeviceID
			}
			errorsCh <- err
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	close(ids)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&Device{}).Where("edge_id = ? AND source_device_id = ?", "edge-concurrent", "source-concurrent").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent discovery count = %d, want 1", count)
	}
	var firstID string
	for id := range ids {
		if firstID == "" {
			firstID = id
		} else if id != firstID {
			t.Fatalf("concurrent discovery returned multiple Cloud UUIDs: %q and %q", firstID, id)
		}
	}
}

func stringPointer(value string) *string { return &value }
