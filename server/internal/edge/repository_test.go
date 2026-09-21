package edge

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`
CREATE TABLE edge (
    edge_id TEXT NOT NULL PRIMARY KEY,
    status TEXT NOT NULL,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRepositoryObserveUsesAtomicUpsertAndKeepsEarliestRegistration(t *testing.T) {
	repository := NewRepository(openRepositoryTestDB(t))
	ctx := context.Background()
	first := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	later := first.Add(2 * time.Minute)
	earlier := first.Add(-2 * time.Minute)

	created, err := repository.Observe(ctx, Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: first})
	if err != nil {
		t.Fatal(err)
	}
	if created.RegisteredAt != first || created.LastSeenAt != first || created.Status != StatusOnline {
		t.Fatalf("first projection = %+v", created)
	}
	updated, err := repository.Observe(ctx, Observation{EdgeID: "edge-01", Status: StatusOffline, ReceivedAt: later})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RegisteredAt != first || updated.LastSeenAt != later || updated.Status != StatusOffline {
		t.Fatalf("repeat projection = %+v", updated)
	}
	updated, err = repository.Observe(ctx, Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: earlier})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RegisteredAt != earlier || updated.LastSeenAt != later || updated.Status != StatusOffline {
		t.Fatalf("older observation projection = %+v, want registeredAt=%v lastSeenAt=%v status=%s", updated, earlier, later, StatusOffline)
	}

	// Equal Cloud receive times are ordered by the atomic statement execution,
	// so the later statement wins even though its timestamp is unchanged.
	updated, err = repository.Observe(ctx, Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: later})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RegisteredAt != earlier || updated.LastSeenAt != later || updated.Status != StatusOnline {
		t.Fatalf("equal-time observation was not applied = %+v", updated)
	}

	repeatedAt := later.Add(time.Minute)
	updated, err = repository.Observe(ctx, Observation{EdgeID: "edge-01", Status: StatusOnline, ReceivedAt: repeatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RegisteredAt != earlier || updated.LastSeenAt != repeatedAt || updated.Status != StatusOnline {
		t.Fatalf("repeated status did not refresh lastSeenAt = %+v", updated)
	}
}

func TestRepositoryObserveCorrectsRegistrationWhenEarlierDiscoveryCommitsLater(t *testing.T) {
	repository := NewRepository(openRepositoryTestDB(t))
	ctx := context.Background()
	earlier := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Second)

	if _, err := repository.Observe(ctx, Observation{
		EdgeID: "edge-reversed-discovery", Status: StatusOffline, ReceivedAt: later,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := repository.Observe(ctx, Observation{
		EdgeID: "edge-reversed-discovery", Status: StatusOnline, ReceivedAt: earlier,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.RegisteredAt != earlier {
		t.Fatalf("registeredAt = %v, want earliest Cloud ReceivedAt %v", result.RegisteredAt, earlier)
	}
	if result.LastSeenAt != later || result.Status != StatusOffline {
		t.Fatalf("current projection regressed = %+v, want lastSeenAt=%v status=%s", result, later, StatusOffline)
	}
}

func TestRepositoryConcurrentObserveLeavesOneRowAndProjectsLatestObservation(t *testing.T) {
	repository := NewRepository(openRepositoryTestDB(t))
	base := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	const workers = 12
	if _, err := repository.Observe(context.Background(), Observation{
		EdgeID: "edge-concurrent", Status: StatusOffline, ReceivedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := repository.Observe(context.Background(), Observation{
				EdgeID:     "edge-concurrent",
				Status:     StatusOnline,
				ReceivedAt: base.Add(time.Duration(index+1) * time.Second),
			})
			errors <- err
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	var count int64
	if err := repository.db.Model(&Edge{}).Where("edge_id = ?", "edge-concurrent").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent discovery row count = %d, want 1", count)
	}
	var result Edge
	if err := repository.db.Where("edge_id = ?", "edge-concurrent").Take(&result).Error; err != nil {
		t.Fatal(err)
	}
	wantLastSeenAt := base.Add(time.Duration(workers) * time.Second)
	if !result.RegisteredAt.Equal(base) || !result.LastSeenAt.Equal(wantLastSeenAt) || result.Status != StatusOnline {
		t.Fatalf("concurrent projection = %+v, want registeredAt=%v lastSeenAt=%v status=%s", result, base, wantLastSeenAt, StatusOnline)
	}
}

func TestRepositoryPageFiltersExactlyAndSortsStably(t *testing.T) {
	repository := NewRepository(openRepositoryTestDB(t))
	base := time.Date(2026, 9, 21, 3, 0, 0, 0, time.UTC)
	observations := []Observation{
		{EdgeID: "edge-z", Status: StatusOnline, ReceivedAt: base},
		{EdgeID: "edge-a", Status: StatusOffline, ReceivedAt: base},
		{EdgeID: "edge-b", Status: StatusOnline, ReceivedAt: base.Add(-time.Minute)},
	}
	for _, observation := range observations {
		if _, err := repository.Observe(context.Background(), observation); err != nil {
			t.Fatal(err)
		}
	}

	page, err := repository.Page(context.Background(), PageQuery{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Records) != 2 {
		t.Fatalf("page total/records = %d/%d, want 3/2", page.Total, len(page.Records))
	}
	if page.Records[0].EdgeID != "edge-a" || page.Records[1].EdgeID != "edge-z" {
		t.Fatalf("stable order = %v, want edge-a then edge-z", []string{page.Records[0].EdgeID, page.Records[1].EdgeID})
	}

	status := StatusOnline
	filtered, err := repository.Page(context.Background(), PageQuery{Page: 1, PageSize: 10, Status: &status, EdgeID: "edge-z"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Records) != 1 || filtered.Records[0].EdgeID != "edge-z" {
		t.Fatalf("filtered page = %+v, want only edge-z", filtered)
	}
}

func TestRepositoryDetailReturnsNotFound(t *testing.T) {
	repository := NewRepository(openRepositoryTestDB(t))
	if _, err := repository.Detail(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("Detail missing error = %v, want ErrNotFound", err)
	}
}
