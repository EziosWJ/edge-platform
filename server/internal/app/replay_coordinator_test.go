package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type replayRequester struct {
	mu            sync.Mutex
	calls         int
	active        int
	maximum       int
	started       chan struct{}
	release       chan struct{}
	waitForCancel atomic.Bool
	err           error
}

func (r *replayRequester) ReplayDeviceStatus(ctx context.Context) error {
	r.mu.Lock()
	r.calls++
	r.active++
	if r.active > r.maximum {
		r.maximum = r.active
	}
	started := r.started
	release := r.release
	waitForCancel := r.waitForCancel.Load()
	r.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if waitForCancel {
		<-ctx.Done()
	} else if release != nil {
		<-release
	}
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return r.err
}

func (r *replayRequester) stats() (calls, active, maximum int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.active, r.maximum
}

func TestReplayCoordinatorCoalescesBeforeAndDuringReplay(t *testing.T) {
	requester := &replayRequester{started: make(chan struct{}, 4), release: make(chan struct{})}
	coordinator := newDeviceStatusReplayCoordinator(requester, nil)
	defer func() { _ = coordinator.Stop(context.Background()) }()
	for i := 0; i < 10; i++ {
		coordinator.Notify()
	}
	select {
	case <-requester.started:
	case <-time.After(time.Second):
		t.Fatal("first replay did not start")
	}
	for i := 0; i < 10; i++ {
		coordinator.Notify()
	}
	close(requester.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		calls, active, maximum := requester.stats()
		if calls == 2 && active == 0 {
			if maximum != 1 {
				t.Fatalf("maximum replay concurrency = %d, want 1", maximum)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	calls, active, maximum := requester.stats()
	t.Fatalf("replay stats = calls=%d active=%d maximum=%d, want 2/0/1", calls, active, maximum)
}

func TestReplayCoordinatorFailureDoesNotBusyLoop(t *testing.T) {
	requester := &replayRequester{release: make(chan struct{}), err: errors.New("subscribe failed")}
	coordinator := newDeviceStatusReplayCoordinator(requester, nil)
	defer func() { _ = coordinator.Stop(context.Background()) }()
	coordinator.Notify()
	close(requester.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		calls, _, _ := requester.stats()
		if calls == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	calls, _, _ := requester.stats()
	if calls != 1 {
		t.Fatalf("failed replay calls = %d, want one bounded attempt", calls)
	}
}

func TestReplayCoordinatorShutdownCancelsInFlightRequest(t *testing.T) {
	requester := &replayRequester{started: make(chan struct{}, 1)}
	requester.waitForCancel.Store(true)
	coordinator := newDeviceStatusReplayCoordinator(requester, nil)
	coordinator.Notify()
	select {
	case <-requester.started:
	case <-time.After(time.Second):
		t.Fatal("replay did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinator.Stop(ctx); err != nil {
		t.Fatalf("coordinator Stop() error = %v", err)
	}
	calls, active, maximum := requester.stats()
	if calls != 1 || active != 0 || maximum != 1 {
		t.Fatalf("shutdown replay stats = calls=%d active=%d maximum=%d", calls, active, maximum)
	}
}
