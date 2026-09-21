package edge

import (
	"context"
	"errors"
	"strings"
)

type Store interface {
	Observe(context.Context, Observation) (Edge, error)
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (Edge, error)
}

type Service struct{ store Store }

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("edge store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) Observe(ctx context.Context, observation Observation) (Edge, error) {
	if strings.TrimSpace(observation.EdgeID) == "" || !validStatus(observation.Status) || observation.ReceivedAt.IsZero() {
		return Edge{}, ErrInvalid
	}
	observation.ReceivedAt = observation.ReceivedAt.UTC()
	return s.store.Observe(ctx, observation)
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
