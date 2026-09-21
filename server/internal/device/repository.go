package device

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Observe is one database statement. The INSERT ... SELECT only has a source
// row when the parent Edge already exists, so DeviceStatus cannot implicitly
// register an Edge. The conflict target is the source identity and the
// generated UUID is used only on a real first insert.
func (r *Repository) Observe(ctx context.Context, observation Observation) (Device, error) {
	deviceID := uuid.NewString()
	snapshot := normalizeSnapshot(observation.Snapshot)

	const query = `
INSERT INTO device (
    device_id, edge_id, source_device_id, communication_status,
    registered_at, last_seen_at, last_attempt_at, last_success_at,
    communication_error
)
SELECT ?, e.edge_id, ?, ?, ?, ?, ?, ?, ?
FROM edge e
WHERE e.edge_id = ?
ON CONFLICT (edge_id, source_device_id) DO UPDATE SET
    registered_at = CASE
        WHEN device.registered_at <= excluded.registered_at THEN device.registered_at
        ELSE excluded.registered_at
    END,
    last_seen_at = CASE
        WHEN device.last_seen_at > excluded.last_seen_at THEN device.last_seen_at
        ELSE excluded.last_seen_at
    END,
    communication_status = CASE
        WHEN device.last_seen_at > excluded.last_seen_at THEN device.communication_status
        ELSE excluded.communication_status
    END,
    last_attempt_at = CASE
        WHEN device.last_seen_at > excluded.last_seen_at THEN device.last_attempt_at
        ELSE excluded.last_attempt_at
    END,
    last_success_at = CASE
        WHEN device.last_seen_at > excluded.last_seen_at THEN device.last_success_at
        ELSE excluded.last_success_at
    END,
    communication_error = CASE
        WHEN device.last_seen_at > excluded.last_seen_at THEN device.communication_error
        ELSE excluded.communication_error
    END
RETURNING device_id, edge_id, source_device_id, communication_status,
          registered_at, last_seen_at, last_attempt_at, last_success_at,
          communication_error`

	rows, err := r.db.WithContext(ctx).Raw(query,
		deviceID,
		observation.SourceDeviceID,
		snapshot.CommunicationStatus,
		observation.ReceivedAt,
		observation.ReceivedAt,
		snapshot.LastAttemptAt,
		snapshot.LastSuccessAt,
		snapshot.CommunicationError,
		observation.EdgeID,
	).Rows()
	if err != nil {
		return Device{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Device{}, err
		}
		return Device{}, ErrParentNotFound
	}

	result, err := scanDevice(rows)
	if err != nil {
		return Device{}, err
	}
	return normalizeDevice(result), nil
}

func (r *Repository) Page(ctx context.Context, query PageQuery) (Page, error) {
	var result Page
	db := r.db.WithContext(ctx).Model(&Device{})
	if query.EdgeID != "" {
		db = db.Where("edge_id = ?", query.EdgeID)
	}
	if query.DeviceID != "" {
		db = db.Where("device_id = ?", query.DeviceID)
	}
	if query.SourceDeviceID != "" {
		db = db.Where("source_device_id = ?", query.SourceDeviceID)
	}
	if query.Status != nil {
		db = db.Where("communication_status = ?", *query.Status)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	if err := db.Order("registered_at DESC, device_id ASC").
		Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).
		Find(&result.Records).Error; err != nil {
		return result, err
	}
	result.Page, result.PageSize = query.Page, query.PageSize
	for i := range result.Records {
		result.Records[i] = normalizeDevice(result.Records[i])
	}
	return result, nil
}

func (r *Repository) Detail(ctx context.Context, deviceID string) (Device, error) {
	var result Device
	err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	return normalizeDevice(result), nil
}

func scanDevice(rows *sql.Rows) (Device, error) {
	var (
		result        Device
		lastAttempt   sql.NullTime
		lastSuccess   sql.NullTime
		communication sql.NullString
	)
	if err := rows.Scan(
		&result.DeviceID,
		&result.EdgeID,
		&result.SourceDeviceID,
		&result.CommunicationStatus,
		&result.RegisteredAt,
		&result.LastSeenAt,
		&lastAttempt,
		&lastSuccess,
		&communication,
	); err != nil {
		return Device{}, err
	}
	if lastAttempt.Valid {
		value := lastAttempt.Time
		result.LastAttemptAt = &value
	}
	if lastSuccess.Valid {
		value := lastSuccess.Time
		result.LastSuccessAt = &value
	}
	if communication.Valid {
		value := communication.String
		result.CommunicationError = &value
	}
	return result, nil
}
