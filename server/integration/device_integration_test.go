//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
)

func TestDeviceDiscoveryUsesPostgresIdentityAndAtomicProjection(t *testing.T) {
	t.Helper()
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
	registeredAt := time.Date(2026, 9, 21, 2, 0, 0, 0, time.UTC)
	if _, err := edgeService.Observe(context.Background(), edge.Observation{EdgeID: "edge-device-pg", Status: edge.StatusOffline, ReceivedAt: registeredAt}); err != nil {
		t.Fatal(err)
	}

	firstAttempt := registeredAt.Add(-time.Minute)
	first, err := deviceService.Observe(context.Background(), device.Observation{
		EdgeID: "edge-device-pg", SourceDeviceID: "source-01",
		Snapshot: device.Snapshot{
			CommunicationStatus: device.StatusInitial,
			LastAttemptAt:       &firstAttempt,
		},
		ReceivedAt: registeredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.CommunicationStatus != device.StatusInitial || first.EdgeID != "edge-device-pg" {
		t.Fatalf("first device projection = %+v", first)
	}

	later := registeredAt.Add(time.Minute)
	lastAttempt := later.Add(-time.Second)
	lastSuccess := later.Add(-2 * time.Second)
	errorText := "offline"
	updated, err := deviceService.Observe(context.Background(), device.Observation{
		EdgeID: "edge-device-pg", SourceDeviceID: "source-01",
		Snapshot: device.Snapshot{
			CommunicationStatus: device.StatusOffline,
			LastAttemptAt:       &lastAttempt,
			LastSuccessAt:       &lastSuccess,
			CommunicationError:  &errorText,
		},
		ReceivedAt: later,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DeviceID != first.DeviceID || !updated.RegisteredAt.Equal(registeredAt) || !updated.LastSeenAt.Equal(later) || updated.CommunicationStatus != device.StatusOffline {
		t.Fatalf("updated device projection = %+v", updated)
	}

	if _, err := deviceService.Observe(context.Background(), device.Observation{
		EdgeID: "missing-edge", SourceDeviceID: "source-01",
		Snapshot: device.Snapshot{CommunicationStatus: device.StatusOnline}, ReceivedAt: later,
	}); !errors.Is(err, device.ErrParentNotFound) {
		t.Fatalf("unknown parent error = %v, want ErrParentNotFound", err)
	}

	const workers = 8
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	ids := make(chan string, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			result, err := deviceService.Observe(context.Background(), device.Observation{
				EdgeID: "edge-device-pg", SourceDeviceID: "source-concurrent",
				Snapshot:   device.Snapshot{CommunicationStatus: device.StatusOnline},
				ReceivedAt: later.Add(time.Duration(index) * time.Second),
			})
			if err == nil {
				ids <- result.DeviceID
			}
			errorsCh <- err
		}(index)
	}
	wait.Wait()
	close(errorsCh)
	close(ids)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var firstID string
	for id := range ids {
		if firstID == "" {
			firstID = id
		} else if id != firstID {
			t.Fatalf("concurrent PostgreSQL discovery returned UUIDs %q and %q", firstID, id)
		}
	}
	page, err := deviceService.Page(context.Background(), device.PageQuery{Page: 1, PageSize: 10, EdgeID: "edge-device-pg"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("device page total = %d, want 2", page.Total)
	}
}
