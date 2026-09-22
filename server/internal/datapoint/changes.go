package datapoint

import "time"

// CurrentValueChange is the stable semantic notification emitted after a
// CurrentValue transaction commits. It intentionally contains neither source
// mapping nor collector transport details.
type CurrentValueChange struct {
	DataPointID     string
	DeviceID        string
	PointKey        string
	ValueType       ValueType
	Value           any
	Quality         Quality
	SourceTimestamp *time.Time
	ObservedAt      *time.Time
	Revision        int64
}

// CurrentValueNotifier is a best-effort, non-blocking post-commit seam. A
// false result must never turn a committed database change into a business
// error.
type CurrentValueNotifier interface {
	TryPublish(CurrentValueChange) bool
}

type discardCurrentValueNotifier struct{}

func (discardCurrentValueNotifier) TryPublish(CurrentValueChange) bool { return false }

func currentValueChange(point DataPoint, current CurrentValue) CurrentValueChange {
	return CurrentValueChange{
		DataPointID:     point.DataPointID,
		DeviceID:        point.DeviceID,
		PointKey:        point.PointKey,
		ValueType:       point.ValueType,
		Value:           current.Value(),
		Quality:         current.Quality,
		SourceTimestamp: cloneTime(current.SourceTimestamp),
		ObservedAt:      cloneTime(current.ObservedAt),
		Revision:        current.Revision,
	}
}

func noDataChange(point DataPoint, revision int64) CurrentValueChange {
	return CurrentValueChange{
		DataPointID: point.DataPointID,
		DeviceID:    point.DeviceID,
		PointKey:    point.PointKey,
		ValueType:   point.ValueType,
		Quality:     QualityNoData,
		Revision:    revision,
	}
}
