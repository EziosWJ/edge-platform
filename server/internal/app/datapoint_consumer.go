package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

// raw-register-snapshot/v1 is QoS0. Projection failures are best effort and
// must never be turned into reconnect/redelivery requests.
type rawSnapshotConsumer struct{ service *datapoint.Service }

func newRawSnapshotConsumer(service *datapoint.Service) mqtt.Consumer {
	if service == nil {
		return mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome { return mqtt.OutcomeAccepted })
	}
	return rawSnapshotConsumer{service: service}
}

func (c rawSnapshotConsumer) Consume(ctx context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
	var raw mqtt.RawRegisterSnapshotMessage
	switch value := message.(type) {
	case mqtt.RawRegisterSnapshotMessage:
		raw = value
	case *mqtt.RawRegisterSnapshotMessage:
		if value == nil {
			return mqtt.OutcomeAccepted
		}
		raw = *value
	default:
		return mqtt.OutcomeAccepted
	}
	metadata := raw.Metadata()
	if metadata.DeviceID == nil || *metadata.DeviceID == "" {
		return mqtt.OutcomeRejected
	}
	snapshot, err := datapoint.ParseRawSnapshot(raw.Data)
	if err != nil {
		return mqtt.OutcomeRejected
	}
	err = c.service.Project(ctx, datapoint.RawObservation{EdgeID: metadata.EdgeID, SourceDeviceID: *metadata.DeviceID, ReceivedAt: metadata.ReceivedAt, Snapshot: snapshot})
	if err == nil || errors.Is(err, datapoint.ErrUnknownDevice) || errors.Is(err, datapoint.ErrInvalid) {
		return mqtt.OutcomeAccepted
	}
	// Keep the log bounded and deliberately omit the raw payload/message ID.
	slog.Default().Warn("raw snapshot projection failed", "reason", "persistence")
	return mqtt.OutcomeAccepted
}
