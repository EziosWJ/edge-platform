package edge

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type Store interface {
	Observe(context.Context, Observation) (Edge, error)
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (Edge, error)
}

type ObservationResult struct {
	Edge    Edge
	Created bool
}

// RegistrationStore is an optional repository seam used by M3 to tell a
// composition-layer coordinator whether this observation really inserted a
// new Edge. Existing Edge stores keep the original Observe contract.
type RegistrationStore interface {
	ObserveRegistration(context.Context, Observation) (ObservationResult, error)
}

type Service struct {
	store Store

	listenerMu   sync.RWMutex
	onRegistered func()
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("edge store is required")
	}
	return &Service{store: store}, nil
}

// SetRegistrationListener installs the composition-layer callback invoked
// after a real first Edge insertion. The callback must be non-blocking.
func (s *Service) SetRegistrationListener(listener func()) {
	s.listenerMu.Lock()
	s.onRegistered = listener
	s.listenerMu.Unlock()
}

func (s *Service) Observe(ctx context.Context, observation Observation) (Edge, error) {
	if strings.TrimSpace(observation.EdgeID) == "" || !validStatus(observation.Status) || observation.ReceivedAt.IsZero() {
		return Edge{}, ErrInvalid
	}
	observation.ReceivedAt = observation.ReceivedAt.UTC()
	if registrationStore, ok := s.store.(RegistrationStore); ok {
		result, err := registrationStore.ObserveRegistration(ctx, observation)
		if err != nil {
			return Edge{}, err
		}
		if result.Created {
			s.notifyRegistration()
		}
		return normalizeEdge(result.Edge), nil
	}
	return s.store.Observe(ctx, observation)
}

func (s *Service) notifyRegistration() {
	s.listenerMu.RLock()
	listener := s.onRegistered
	s.listenerMu.RUnlock()
	if listener != nil {
		listener()
	}
}

func (s *Service) Page(ctx context.Context, query PageQuery) (Page, error) {
	if err := validatePageQuery(query); err != nil {
		return Page{}, err
	}
	page, err := s.store.Page(ctx, query)
	if page.Records == nil {
		page.Records = []Edge{}
	}
	for i := range page.Records {
		page.Records[i] = normalizeEdge(page.Records[i])
	}
	page.Page, page.PageSize = query.Page, query.PageSize
	return page, err
}

func (s *Service) Detail(ctx context.Context, edgeID string) (Edge, error) {
	if strings.TrimSpace(edgeID) == "" {
		return Edge{}, ErrInvalid
	}
	detail, err := s.store.Detail(ctx, edgeID)
	if err != nil {
		return Edge{}, err
	}
	return normalizeEdge(detail), nil
}

func validStatus(status Status) bool {
	return status == StatusOnline || status == StatusOffline
}

func validatePageQuery(query PageQuery) error {
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 500 {
		return ErrInvalid
	}
	if query.Status != nil && !validStatus(*query.Status) {
		return ErrInvalid
	}
	return nil
}
