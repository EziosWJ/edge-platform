package realtime

import (
	"context"
	"errors"

	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
)

type currentPointReader interface {
	CurrentByIdentity(context.Context, string, string) (datapoint.CurrentPoint, error)
}

// DataPointReader adapts the DataPoint semantic read model to the realtime
// seam. It intentionally asks DataPoint for CurrentValue only, never mapping.
type DataPointReader struct {
	service currentPointReader
}

func NewDataPointReader(service currentPointReader) *DataPointReader {
	return &DataPointReader{service: service}
}

func (r *DataPointReader) ReadCurrent(ctx context.Context, deviceID, pointKey string) (Point, error) {
	current, err := r.service.CurrentByIdentity(ctx, deviceID, pointKey)
	if err != nil {
		switch {
		case errors.Is(err, datapoint.ErrNotFound):
			return Point{}, ErrPointNotFound
		case errors.Is(err, datapoint.ErrInvalid):
			return Point{}, ErrInvalidPoint
		case errors.Is(err, datapoint.ErrInvalidBinding):
			return Point{}, ErrInvalidBinding
		default:
			return Point{}, err
		}
	}
	return Point{
		DataPointID:     current.DataPointID,
		DeviceID:        current.DeviceID,
		PointKey:        current.PointKey,
		ValueType:       current.ValueType,
		Value:           current.Value,
		Quality:         current.Quality,
		SourceTimestamp: current.SourceTimestamp,
		ObservedAt:      current.ObservedAt,
		Revision:        current.Revision,
	}, nil
}
