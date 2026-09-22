package realtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
)

type Service struct {
	sessions auth.SessionValidator
	points   PointReader
	tickets  *ticketStore
	config   Config
	hub      *Hub

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func NewService(sessions auth.SessionValidator, points PointReader, config Config, hubs ...*Hub) (*Service, error) {
	if sessions == nil {
		return nil, errors.New("realtime session validator is required")
	}
	if points == nil {
		return nil, errors.New("realtime point reader is required")
	}
	if config.TicketTTL <= 0 {
		config.TicketTTL = defaultTicketTTL
	}
	if config.SessionCheckPeriod <= 0 {
		config.SessionCheckPeriod = defaultSessionCheckPeriod
	}
	if config.PingPeriod <= 0 {
		config.PingPeriod = defaultPingPeriod
	}
	if config.OutboundQueueSize <= 0 {
		config.OutboundQueueSize = defaultOutboundQueueSize
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	hub := (*Hub)(nil)
	if len(hubs) > 0 {
		hub = hubs[0]
	}
	if hub == nil {
		hub = NewHub(config.OutboundQueueSize)
	}
	return &Service{
		sessions: sessions,
		points:   points,
		tickets:  newTicketStore(config.TicketTTL, config.Now),
		config:   config,
		hub:      hub,
	}, nil
}

func (s *Service) Hub() *Hub { return s.hub }

func (s *Service) IssueTicket(ctx context.Context, principal auth.Principal) (TicketResponse, error) {
	if err := s.sessions.ValidateSession(ctx, principal); err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) || errors.Is(err, ErrSessionInvalid) {
			return TicketResponse{}, ErrSessionInvalid
		}
		return TicketResponse{}, err
	}
	return s.tickets.issue(principal)
}

func (s *Service) peekTicket(value string) (ticketRecord, error) {
	return s.tickets.peek(value)
}

func (s *Service) reserveTicket(value string, principal auth.Principal) (ticketRecord, error) {
	return s.tickets.reserve(value, principal)
}

func (s *Service) commitTicket(value string, principal auth.Principal) error {
	return s.tickets.commit(value, principal)
}

func (s *Service) releaseTicket(value string, principal auth.Principal) {
	s.tickets.release(value, principal)
}

func (s *Service) addConnection(c *connection) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	if !s.hub.register(c) {
		s.mu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.mu.Unlock()
	return true
}

func (s *Service) removeConnection(c *connection) {
	s.hub.unregister(c)
	s.wg.Done()
}

// Close terminates all upgraded connections with the normal server-shutdown
// code. It is called before http.Server.Shutdown because hijacked WebSocket
// connections are not managed by net/http's graceful shutdown.
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.hub.closeAll(1001, "server shutting down")

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
