package edge

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Observe is one database statement. Current status/lastSeenAt only move
// forward by Cloud ReceivedAt. registeredAt independently keeps the earliest
// Cloud observation so concurrent first-discovery statements cannot make
// registration time depend on database execution order. Equal ReceivedAt
// values follow database statement order for the current status projection.
func (r *Repository) Observe(ctx context.Context, observation Observation) (Edge, error) {
	var result Edge
	err := r.db.WithContext(ctx).Raw(`
INSERT INTO edge (edge_id, status, registered_at, last_seen_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (edge_id) DO UPDATE SET
    status = CASE WHEN edge.last_seen_at > excluded.last_seen_at THEN edge.status ELSE excluded.status END,
    registered_at = CASE WHEN edge.registered_at <= excluded.registered_at THEN edge.registered_at ELSE excluded.registered_at END,
    last_seen_at = CASE WHEN edge.last_seen_at > excluded.last_seen_at THEN edge.last_seen_at ELSE excluded.last_seen_at END
RETURNING edge_id, status, registered_at, last_seen_at`,
		observation.EdgeID, observation.Status, observation.ReceivedAt, observation.ReceivedAt,
	).Scan(&result).Error
	if err != nil {
		return Edge{}, err
	}
	return normalizeEdge(result), nil
}

func (r *Repository) Page(ctx context.Context, query PageQuery) (Page, error) {
	var result Page
	db := r.db.WithContext(ctx).Model(&Edge{})
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.EdgeID != "" {
		db = db.Where("edge_id = ?", query.EdgeID)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	if err := db.Order("registered_at DESC, edge_id ASC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&result.Records).Error; err != nil {
		return result, err
	}
	result.Page, result.PageSize = query.Page, query.PageSize
	for i := range result.Records {
		result.Records[i] = normalizeEdge(result.Records[i])
	}
	return result, nil
}

func (r *Repository) Detail(ctx context.Context, edgeID string) (Edge, error) {
	var result Edge
	err := r.db.WithContext(ctx).Where("edge_id = ?", edgeID).Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Edge{}, ErrNotFound
	}
	if err != nil {
		return Edge{}, err
	}
	return normalizeEdge(result), nil
}

func normalizeEdge(value Edge) Edge {
	value.RegisteredAt = value.RegisteredAt.UTC()
	value.LastSeenAt = value.LastSeenAt.UTC()
	return value
}
