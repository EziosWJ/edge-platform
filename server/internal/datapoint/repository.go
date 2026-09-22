package datapoint

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	Create(context.Context, CreateInput, audit.Event) (DataPoint, error)
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (DataPoint, error)
	Update(context.Context, string, UpdateInput, audit.Event) (DataPoint, error)
	SetEnabled(context.Context, string, bool, audit.Event) (DataPoint, error)
	Project(context.Context, RawObservation) error
}

type Repository struct {
	db       *gorm.DB
	notifier CurrentValueNotifier
}

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB, notifiers ...CurrentValueNotifier) *Repository {
	notifier := CurrentValueNotifier(discardCurrentValueNotifier{})
	if len(notifiers) > 0 && notifiers[0] != nil {
		notifier = notifiers[0]
	}
	return &Repository{db: db, notifier: notifier}
}

func (r *Repository) Create(ctx context.Context, input CreateInput, event audit.Event) (DataPoint, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	point := DataPoint{DataPointID: id, DeviceID: input.DeviceID, PointKey: input.PointKey, Name: input.Name, ValueType: input.ValueType, Unit: input.Unit, Precision: input.Precision, Enabled: input.Enabled, ConfigGeneration: 1, EffectiveAt: now}
	mapping := input.Mapping
	mapping.DataPointID = id
	current := CurrentValue{DataPointID: id, ValueType: input.ValueType, Quality: QualityNoData, Revision: 0}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&struct {
			DeviceID string `gorm:"column:device_id"`
		}{}).Table("device").Where("device_id = ?", input.DeviceID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		if err := tx.Create(&point).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrConflict
			}
			return err
		}
		if err := tx.Create(&mapping).Error; err != nil {
			return err
		}
		if err := tx.Create(&current).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return DataPoint{}, err
	}
	return r.Detail(ctx, id)
}

func (r *Repository) Page(ctx context.Context, query PageQuery) (Page, error) {
	var result Page
	db := r.db.WithContext(ctx).Model(&DataPoint{}).Joins("JOIN current_value ON current_value.data_point_id = data_point.data_point_id").Joins("JOIN source_mapping ON source_mapping.data_point_id = data_point.data_point_id")
	if query.DeviceID != "" {
		db = db.Where("data_point.device_id = ?", query.DeviceID)
	}
	if query.PointKey != "" {
		db = db.Where("data_point.point_key = ?", query.PointKey)
	}
	if query.ValueType != nil {
		db = db.Where("data_point.value_type = ?", *query.ValueType)
	}
	if query.Enabled != nil {
		db = db.Where("data_point.enabled = ?", *query.Enabled)
	}
	if query.Quality != nil {
		db = db.Where("current_value.quality = ?", *query.Quality)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var points []DataPoint
	if err := db.Select("data_point.*").Order("data_point.device_id ASC, data_point.point_key ASC, data_point.data_point_id ASC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&points).Error; err != nil {
		return result, err
	}
	for i := range points {
		if err := r.loadAssociations(ctx, r.db, &points[i]); err != nil {
			return result, err
		}
	}
	result.Records, result.Page, result.PageSize = points, query.Page, query.PageSize
	if result.Records == nil {
		result.Records = []DataPoint{}
	}
	return result, nil
}

func (r *Repository) Detail(ctx context.Context, id string) (DataPoint, error) {
	var point DataPoint
	if err := r.db.WithContext(ctx).Where("data_point_id = ?", id).Take(&point).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return DataPoint{}, ErrNotFound
	} else if err != nil {
		return DataPoint{}, err
	}
	if err := r.loadAssociations(ctx, r.db, &point); err != nil {
		return DataPoint{}, err
	}
	return point, nil
}

func (r *Repository) CurrentByIdentity(ctx context.Context, deviceID, pointKey string) (CurrentPoint, error) {
	var point DataPoint
	if err := r.db.WithContext(ctx).
		Where("device_id = ? AND point_key = ?", deviceID, pointKey).
		Take(&point).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return CurrentPoint{}, ErrNotFound
	} else if err != nil {
		return CurrentPoint{}, err
	}

	var mapping SourceMapping
	if err := r.db.WithContext(ctx).Where("data_point_id = ?", point.DataPointID).Take(&mapping).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return CurrentPoint{}, ErrInvalidBinding
	} else if err != nil {
		return CurrentPoint{}, err
	}
	if err := ValidateMapping(point.ValueType, mapping); err != nil {
		return CurrentPoint{}, ErrInvalidBinding
	}

	var current CurrentValue
	if err := r.db.WithContext(ctx).Where("data_point_id = ?", point.DataPointID).Take(&current).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return CurrentPoint{}, ErrInvalidBinding
	} else if err != nil {
		return CurrentPoint{}, err
	}
	if current.ValueType != point.ValueType {
		return CurrentPoint{}, ErrInvalidBinding
	}
	return CurrentPoint{
		DataPointID:     point.DataPointID,
		DeviceID:        point.DeviceID,
		PointKey:        point.PointKey,
		ValueType:       point.ValueType,
		Value:           current.Value(),
		Quality:         current.Quality,
		SourceTimestamp: current.SourceTimestamp,
		ObservedAt:      current.ObservedAt,
		Revision:        current.Revision,
	}, nil
}

func (r *Repository) loadAssociations(ctx context.Context, db *gorm.DB, point *DataPoint) error {
	if err := db.WithContext(ctx).Where("data_point_id = ?", point.DataPointID).Take(&point.Mapping).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Where("data_point_id = ?", point.DataPointID).Take(&point.Current).Error; err != nil {
		return err
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput, event audit.Event) (DataPoint, error) {
	var change *CurrentValueChange
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var point DataPoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("data_point_id = ?", id).Take(&point).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		var old SourceMapping
		if err := tx.Where("data_point_id = ?", id).Take(&old).Error; err != nil {
			return err
		}
		input.Mapping.DataPointID = id
		if err := ValidateMapping(point.ValueType, input.Mapping); err != nil {
			return err
		}
		if err := tx.Model(&point).Updates(map[string]any{"name": input.Name, "unit": input.Unit, "precision": input.Precision}).Error; err != nil {
			return err
		}
		semantic := mappingChanged(old, input.Mapping)
		if semantic {
			now := time.Now().UTC()
			if err := tx.Model(&SourceMapping{}).Where("data_point_id = ?", id).Updates(map[string]any{"source_type": input.Mapping.SourceType, "function_code": input.Mapping.FunctionCode, "address": input.Mapping.Address, "encoding": input.Mapping.Encoding, "word_order": input.Mapping.WordOrder, "byte_order": input.Mapping.ByteOrder, "bit_index": input.Mapping.BitIndex, "scale": input.Mapping.Scale, "offset": input.Mapping.Offset}).Error; err != nil {
				return err
			}
			var current CurrentValue
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("data_point_id = ?", id).Take(&current).Error; err != nil {
				return err
			}
			if err := resetCurrent(tx, id, point.ValueType, point.ConfigGeneration+1, now); err != nil {
				return err
			}
			if err := tx.Model(&DataPoint{}).Where("data_point_id = ?", id).Updates(map[string]any{"config_generation": point.ConfigGeneration + 1, "effective_at": now}).Error; err != nil {
				return err
			}
			changeValue := noDataChange(point, current.Revision+1)
			change = &changeValue
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return DataPoint{}, err
	}
	if change != nil {
		r.notifier.TryPublish(*change)
	}
	return r.Detail(ctx, id)
}

func (r *Repository) SetEnabled(ctx context.Context, id string, enabled bool, event audit.Event) (DataPoint, error) {
	var change *CurrentValueChange
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var point DataPoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("data_point_id = ?", id).Take(&point).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if point.Enabled == enabled {
			return audit.RecordOn(ctx, tx, event)
		}
		now := time.Now().UTC()
		var current CurrentValue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("data_point_id = ?", id).Take(&current).Error; err != nil {
			return err
		}
		if err := tx.Model(&DataPoint{}).Where("data_point_id = ?", id).Updates(map[string]any{"enabled": enabled, "config_generation": point.ConfigGeneration + 1, "effective_at": now}).Error; err != nil {
			return err
		}
		if err := resetCurrent(tx, id, point.ValueType, point.ConfigGeneration+1, now); err != nil {
			return err
		}
		changeValue := noDataChange(point, current.Revision+1)
		change = &changeValue
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return DataPoint{}, err
	}
	if change != nil {
		r.notifier.TryPublish(*change)
	}
	return r.Detail(ctx, id)
}

func resetCurrent(tx *gorm.DB, id string, valueType ValueType, _ int64, _ time.Time) error {
	return tx.Model(&CurrentValue{}).Where("data_point_id = ?", id).Updates(map[string]any{"value_type": valueType, "number_value": nil, "boolean_value": nil, "quality": QualityNoData, "source_timestamp": nil, "observed_at": nil, "revision": gorm.Expr("revision + 1")}).Error
}

func (r *Repository) Project(ctx context.Context, observation RawObservation) error {
	if observation.ReceivedAt.IsZero() {
		return ErrInvalid
	}
	var changes []CurrentValueChange
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var deviceID string
		if err := tx.Table("device").Select("device_id").Where("edge_id = ? AND source_device_id = ?", observation.EdgeID, observation.SourceDeviceID).Take(&deviceID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnknownDevice
		} else if err != nil {
			return err
		}
		var points []DataPoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("device_id = ? AND enabled = ?", deviceID, true).Find(&points).Error; err != nil {
			return err
		}
		for i := range points {
			if err := r.loadAssociations(ctx, tx, &points[i]); err != nil {
				return err
			}
			if !observation.ReceivedAt.UTC().After(points[i].EffectiveAt.UTC()) {
				continue
			}
			projection := project(points[i].Mapping, observation.Snapshot)
			var current CurrentValue
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("data_point_id = ?", points[i].DataPointID).Take(&current).Error; err != nil {
				return err
			}
			if current.ObservedAt != nil && observation.ReceivedAt.UTC().Before(current.ObservedAt.UTC()) {
				continue
			}
			current.Quality = projection.Quality
			observed := observation.ReceivedAt.UTC()
			current.ObservedAt = &observed
			current.Revision++
			if projection.Quality == QualityGood {
				current.SourceTimestamp = projection.SourceTimestamp
				current.NumberValue, current.BooleanValue = nil, nil
				if points[i].ValueType == ValueTypeNumber {
					value := projection.Value.(float64)
					current.NumberValue = &value
				} else {
					value := projection.Value.(bool)
					current.BooleanValue = &value
				}
			}
			if err := tx.Model(&CurrentValue{}).Where("data_point_id = ?", current.DataPointID).Updates(map[string]any{"number_value": current.NumberValue, "boolean_value": current.BooleanValue, "quality": current.Quality, "source_timestamp": current.SourceTimestamp, "observed_at": current.ObservedAt, "revision": current.Revision}).Error; err != nil {
				return err
			}
			changes = append(changes, currentValueChange(points[i], current))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, change := range changes {
		r.notifier.TryPublish(change)
	}
	return nil
}
