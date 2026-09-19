package auth

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLoginGuardRejectsIPAfterConcurrentAdmissionLimit(t *testing.T) {
	guard := newTestLoginGuard(t, LoginGuardConfig{
		IPWindow:            10 * time.Minute,
		IPMaxAttempts:       1,
		UsernameMaxAttempts: 0,
		BackoffInitial:      time.Second,
		BackoffMax:          30 * time.Second,
		LockDuration:        10 * time.Minute,
		MaxEntries:          10,
	})

	const requestCount = 32
	var wg sync.WaitGroup
	results := make(chan error, requestCount)
	for range requestCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- guard.Admit("203.0.113.8", "admin")
		}()
	}
	wg.Wait()
	close(results)

	admitted := 0
	rejected := 0
	for err := range results {
		if err == nil {
			admitted++
			continue
		}
		var rateLimitError *LoginRateLimitError
		if !errors.As(err, &rateLimitError) {
			t.Fatalf("Admit() error = %v, want LoginRateLimitError", err)
		}
		rejected++
	}
	if admitted != 1 || rejected != requestCount-1 {
		t.Fatalf("admitted=%d rejected=%d, want 1 and %d", admitted, rejected, requestCount-1)
	}
}

func TestLoginGuardAppliesExponentialBackoffAndSuccessReset(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	guard := newTestLoginGuard(t, LoginGuardConfig{
		UsernameWindow:      10 * time.Minute,
		UsernameMaxAttempts: 5,
		BackoffInitial:      time.Second,
		BackoffMax:          4 * time.Second,
		LockDuration:        10 * time.Minute,
		MaxEntries:          10,
	})
	guard.now = func() time.Time { return now }

	if err := guard.Admit("203.0.113.8", " admin "); err != nil {
		t.Fatal(err)
	}
	guard.RecordFailure("203.0.113.8", "admin")

	err := guard.Admit("198.51.100.9", "admin")
	assertRetryAfter(t, err, time.Second)

	now = now.Add(time.Second)
	if err := guard.Admit("198.51.100.9", "admin"); err != nil {
		t.Fatal(err)
	}
	guard.RecordFailure("198.51.100.9", "admin")
	assertRetryAfter(t, guard.Admit("192.0.2.4", "admin"), 2*time.Second)

	guard.RecordSuccess(" admin ")
	if err := guard.Admit("192.0.2.4", "admin"); err != nil {
		t.Fatalf("successful login should clear username state: %v", err)
	}
}

func TestLoginGuardTemporarilyLocksUsernameAtAttemptLimit(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	guard := newTestLoginGuard(t, LoginGuardConfig{
		UsernameWindow:      10 * time.Minute,
		UsernameMaxAttempts: 2,
		BackoffInitial:      time.Second,
		BackoffMax:          time.Second,
		LockDuration:        10 * time.Minute,
		MaxEntries:          10,
	})
	guard.now = func() time.Time { return now }

	for attempt := 0; attempt < 2; attempt++ {
		if err := guard.Admit("203.0.113.8", "admin"); err != nil {
			t.Fatalf("attempt %d Admit() error = %v", attempt+1, err)
		}
		guard.RecordFailure("203.0.113.8", "admin")
		if attempt == 0 {
			now = now.Add(time.Second)
		}
	}

	assertRetryAfter(t, guard.Admit("203.0.113.8", "admin"), 10*time.Minute)
}

func TestLoginGuardEnforcesCapacityPerDimension(t *testing.T) {
	guard := newTestLoginGuard(t, LoginGuardConfig{
		IPWindow:            time.Minute,
		IPMaxAttempts:       10,
		UsernameWindow:      time.Minute,
		UsernameMaxAttempts: 10,
		BackoffInitial:      time.Second,
		BackoffMax:          time.Second,
		LockDuration:        time.Minute,
		MaxEntries:          1,
	})

	if err := guard.Admit("203.0.113.1", "one"); err != nil {
		t.Fatal(err)
	}
	if err := guard.Admit("203.0.113.2", "two"); err != nil {
		t.Fatal(err)
	}
	if len(guard.ipStates) != 1 || len(guard.usernameStates) != 1 {
		t.Fatalf("state sizes = (%d, %d), want (1, 1)", len(guard.ipStates), len(guard.usernameStates))
	}
}

func TestLoginGuardAllowsBothDimensionsToBeDisabled(t *testing.T) {
	guard := newTestLoginGuard(t, LoginGuardConfig{
		BackoffInitial: time.Second,
		BackoffMax:     time.Second,
		LockDuration:   time.Minute,
		MaxEntries:     1,
	})

	for range 3 {
		if err := guard.Admit("203.0.113.8", "admin"); err != nil {
			t.Fatalf("disabled guard should admit: %v", err)
		}
	}
	if len(guard.ipStates) != 0 || len(guard.usernameStates) != 0 {
		t.Fatalf("disabled guard state = (%d, %d), want empty", len(guard.ipStates), len(guard.usernameStates))
	}
}

func TestExponentialBackoffCapsAtMaximum(t *testing.T) {
	for _, test := range []struct {
		failures int
		want     time.Duration
	}{
		{failures: 1, want: time.Second},
		{failures: 2, want: 2 * time.Second},
		{failures: 3, want: 4 * time.Second},
		{failures: 4, want: 4 * time.Second},
	} {
		if got := exponentialBackoff(time.Second, 4*time.Second, test.failures); got != test.want {
			t.Errorf("exponentialBackoff(%d) = %s, want %s", test.failures, got, test.want)
		}
	}
}

func newTestLoginGuard(t *testing.T, config LoginGuardConfig) *LoginGuard {
	t.Helper()
	guard, err := NewLoginGuard(config)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func assertRetryAfter(t *testing.T, err error, want time.Duration) {
	t.Helper()
	var rateLimitError *LoginRateLimitError
	if !errors.As(err, &rateLimitError) {
		t.Fatalf("error = %v, want LoginRateLimitError", err)
	}
	if rateLimitError.RetryAfter != want {
		t.Fatalf("RetryAfter = %s, want %s", rateLimitError.RetryAfter, want)
	}
}
