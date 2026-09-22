package realtime

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
)

type ticketRecord struct {
	principal auth.Principal
	expiresAt time.Time
	reserved  bool
}

type ticketStore struct {
	mu      sync.Mutex
	records map[string]ticketRecord
	ttl     time.Duration
	now     func() time.Time
}

func newTicketStore(ttl time.Duration, now func() time.Time) *ticketStore {
	if ttl <= 0 {
		ttl = defaultTicketTTL
	}
	if now == nil {
		now = time.Now
	}
	return &ticketStore{records: make(map[string]ticketRecord), ttl: ttl, now: now}
}

func (s *ticketStore) issue(principal auth.Principal) (TicketResponse, error) {
	if principal.UserID <= 0 || principal.JTI == "" || principal.ExpiresAt.IsZero() {
		return TicketResponse{}, ErrSessionInvalid
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.ttl)
	if principal.ExpiresAt.Before(expiresAt) {
		expiresAt = principal.ExpiresAt
	}
	if !expiresAt.After(now) {
		return TicketResponse{}, ErrSessionInvalid
	}
	value, err := randomTicket()
	if err != nil {
		return TicketResponse{}, err
	}
	s.mu.Lock()
	s.records[value] = ticketRecord{principal: principal, expiresAt: expiresAt}
	s.mu.Unlock()
	return TicketResponse{Ticket: value, ExpiresAt: expiresAt, ExpiresIn: int64(expiresAt.Sub(now).Seconds())}, nil
}

func (s *ticketStore) peek(value string) (ticketRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[value]
	if !ok {
		return ticketRecord{}, ErrInvalidTicket
	}
	if !record.expiresAt.After(s.now().UTC()) {
		delete(s.records, value)
		return ticketRecord{}, ErrInvalidTicket
	}
	return record, nil
}

// reserve prevents concurrent upgrades from using the same ticket while the
// HTTP upgrade is in progress. It is not consumption: a failed upgrade can
// call release and let the client retry with the same ticket.
func (s *ticketStore) reserve(value string, expected auth.Principal) (ticketRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[value]
	if !ok || !record.expiresAt.After(s.now().UTC()) {
		delete(s.records, value)
		return ticketRecord{}, ErrInvalidTicket
	}
	if record.reserved {
		return ticketRecord{}, ErrInvalidTicket
	}
	if record.principal.UserID != expected.UserID || record.principal.JTI != expected.JTI || !record.principal.ExpiresAt.Equal(expected.ExpiresAt) {
		return ticketRecord{}, ErrInvalidTicket
	}
	record.reserved = true
	s.records[value] = record
	return record, nil
}

func (s *ticketStore) commit(value string, expected auth.Principal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[value]
	if !ok || !record.reserved || record.principal.UserID != expected.UserID || record.principal.JTI != expected.JTI {
		return ErrInvalidTicket
	}
	delete(s.records, value)
	return nil
}

func (s *ticketStore) release(value string, expected auth.Principal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[value]
	if !ok || !record.reserved || record.principal.UserID != expected.UserID || record.principal.JTI != expected.JTI {
		return
	}
	record.reserved = false
	s.records[value] = record
}

func randomTicket() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("create realtime ticket: random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
