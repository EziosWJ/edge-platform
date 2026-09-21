package device

import (
	"context"
	"errors"
	"strings"
)

type Store interface {
	Observe(context.Context, Observation) (Device, error)
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (Device, error)
}

type Service struct{ store Store }

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("device store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) Observe(ctx context.Context, observation Observation) (Device, error) {
	if strings.TrimSpace(observation.EdgeID) == "" ||
		strings.TrimSpace(observation.SourceDeviceID) == "" ||
		!validStatus(observation.Snapshot.CommunicationStatus) ||
		observation.ReceivedAt.IsZero() {
		return Device{}, ErrInvalid
	}
	observation.ReceivedAt = observation.ReceivedAt.UTC()
	observation.Snapshot = normalizeSnapshot(observation.Snapshot)
	return s.store.Observe(ctx, observation)
}

func (s *Service) Page(ctx context.Context, query PageQuery) (Page, error) {
	if err := validatePageQuery(query); err != nil {
		return Page{}, err
	}
	page, err := s.store.Page(ctx, query)
	if page.Records == nil {
		page.Records = []Device{}
	}
	for i := range page.Records {
		page.Records[i] = normalizeDevice(page.Records[i])
	}
	page.Page, page.PageSize = query.Page, query.PageSize
	return page, err
}

func (s *Service) Detail(ctx context.Context, deviceID string) (Device, error) {
	if strings.TrimSpace(deviceID) == "" {
		return Device{}, ErrInvalid
	}
	detail, err := s.store.Detail(ctx, deviceID)
	if err != nil {
		return Device{}, err
	}
	return normalizeDevice(detail), nil
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
