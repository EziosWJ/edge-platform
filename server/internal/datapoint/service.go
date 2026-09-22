package datapoint

import (
	"context"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

type Service struct{ store Store }

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalid
	}
	return &Service{store: store}, nil
}

func (s *Service) Create(ctx context.Context, metadata audit.Metadata, input CreateInput) (DataPoint, error) {
	if err := ValidateCreate(input); err != nil {
		return DataPoint{}, err
	}
	return s.store.Create(ctx, input, audit.Event{Action: "datapoint.create", Resource: "datapoint", Summary: "创建 DataPoint", Metadata: metadata})
}

func (s *Service) Page(ctx context.Context, query PageQuery) (Page, error) {
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 500 || strings.TrimSpace(query.DeviceID) != query.DeviceID || strings.TrimSpace(query.PointKey) != query.PointKey {
		return Page{}, ErrInvalid
	}
	if query.PointKey != "" && !ValidatePointKey(query.PointKey) {
		return Page{}, ErrInvalid
	}
	if query.ValueType != nil && !validValueType(*query.ValueType) || query.Quality != nil && !validQuality(*query.Quality) {
		return Page{}, ErrInvalid
	}
	return s.store.Page(ctx, query)
}

func (s *Service) Detail(ctx context.Context, id string) (DataPoint, error) {
	if strings.TrimSpace(id) == "" {
		return DataPoint{}, ErrInvalid
	}
	return s.store.Detail(ctx, id)
}

func (s *Service) Update(ctx context.Context, metadata audit.Metadata, id string, input UpdateInput) (DataPoint, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(input.Name) == "" || input.Precision != nil && (*input.Precision < 0 || *input.Precision > 12) {
		return DataPoint{}, ErrInvalid
	}
	if err := ValidateMapping(ValueTypeNumber, input.Mapping); err != nil {
		// BOOLEAN mappings are valid only for BOOLEAN points; the repository
		// checks the immutable point type and performs the final compatibility validation.
		if err := ValidateMapping(ValueTypeBoolean, input.Mapping); err != nil {
			return DataPoint{}, ErrInvalid
		}
	}
	return s.store.Update(ctx, id, input, audit.Event{Action: "datapoint.update", Resource: "datapoint", Summary: "修改 DataPoint", Metadata: metadata})
}

func (s *Service) SetEnabled(ctx context.Context, metadata audit.Metadata, id string, enabled bool) (DataPoint, error) {
	if strings.TrimSpace(id) == "" {
		return DataPoint{}, ErrInvalid
	}
	return s.store.SetEnabled(ctx, id, enabled, audit.Event{Action: "datapoint.enabled", Resource: "datapoint", Summary: "修改 DataPoint 启用状态", Metadata: metadata})
}

func (s *Service) Project(ctx context.Context, observation RawObservation) error {
	return s.store.Project(ctx, observation)
}
