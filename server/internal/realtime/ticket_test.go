package realtime

import (
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
)

func TestTicketStoreIsShortLivedAndOneTime(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	store := newTicketStore(30*time.Second, func() time.Time { return now })
	principal := auth.Principal{UserID: 7, JTI: "session-jti", ExpiresAt: now.Add(time.Minute)}

	result, err := store.issue(principal)
	if err != nil {
		t.Fatalf("issue() error = %v", err)
	}
	if result.ExpiresIn != 30 || result.ExpiresAt.Equal(principal.ExpiresAt) {
		t.Fatalf("ticket expiry = %+v, want 30 seconds capped by TTL", result)
	}
	if len(result.Ticket) < 40 {
		t.Fatalf("ticket is too short: %q", result.Ticket)
	}
	if _, err := store.peek(result.Ticket); err != nil {
		t.Fatalf("peek() error = %v", err)
	}
	if _, err := store.reserve(result.Ticket, principal); err != nil {
		t.Fatalf("first reserve() error = %v", err)
	}
	if _, err := store.reserve(result.Ticket, principal); err != ErrInvalidTicket {
		t.Fatalf("concurrent reserve() error = %v, want ErrInvalidTicket", err)
	}
	if err := store.commit(result.Ticket, principal); err != nil {
		t.Fatalf("commit() error = %v", err)
	}
	if _, err := store.reserve(result.Ticket, principal); err != ErrInvalidTicket {
		t.Fatalf("second reserve() error = %v, want ErrInvalidTicket", err)
	}
}

func TestTicketStoreExpiresAndNeverOutlivesSession(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	store := newTicketStore(time.Minute, func() time.Time { return now })
	principal := auth.Principal{UserID: 7, JTI: "session-jti", ExpiresAt: now.Add(10 * time.Second)}

	result, err := store.issue(principal)
	if err != nil {
		t.Fatalf("issue() error = %v", err)
	}
	if result.ExpiresAt != principal.ExpiresAt || result.ExpiresIn != 10 {
		t.Fatalf("ticket expiry = %+v, want session expiry", result)
	}

	now = now.Add(10 * time.Second)
	if _, err := store.peek(result.Ticket); err != ErrInvalidTicket {
		t.Fatalf("expired peek() error = %v, want ErrInvalidTicket", err)
	}
}

func TestTicketReservationCanBeReleasedAfterFailedUpgrade(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	store := newTicketStore(time.Minute, func() time.Time { return now })
	principal := auth.Principal{UserID: 7, JTI: "session-jti", ExpiresAt: now.Add(time.Minute)}
	result, err := store.issue(principal)
	if err != nil {
		t.Fatalf("issue() error = %v", err)
	}
	if _, err := store.reserve(result.Ticket, principal); err != nil {
		t.Fatalf("reserve() error = %v", err)
	}
	store.release(result.Ticket, principal)
	if _, err := store.reserve(result.Ticket, principal); err != nil {
		t.Fatalf("reserve() after release error = %v", err)
	}
}
