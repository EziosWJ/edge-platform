package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
)

// edgeStatusConsumer is the adapter between MQTT ingest DTOs and the Edge
// domain. The domain only receives edge identity, projected status, and the
// Cloud receive time.
type edgeStatusConsumer struct{ service *edge.Service }

func newEdgeStatusConsumer(service *edge.Service) mqtt.Consumer {
	if service == nil {
		return mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		})
	}
	return edgeStatusConsumer{service: service}
}

func (c edgeStatusConsumer) Consume(ctx context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
	var status mqtt.EdgeStatusMessage
	switch value := message.(type) {
	case mqtt.EdgeStatusMessage:
		status = value
	case *mqtt.EdgeStatusMessage:
		if value != nil {
			status = *value
		} else {
			return mqtt.OutcomeAccepted
		}
	default:
		return mqtt.OutcomeAccepted
	}

	var data struct {
		Online *bool `json:"online"`
	}
	if err := json.Unmarshal(status.Data, &data); err != nil || data.Online == nil {
		return mqtt.OutcomeRejected
	}

	projectedStatus := edge.StatusOffline
	if *data.Online {
		projectedStatus = edge.StatusOnline
	}
	_, err := c.service.Observe(ctx, edge.Observation{
		EdgeID:     status.Metadata().EdgeID,
		Status:     projectedStatus,
		ReceivedAt: status.Metadata().ReceivedAt,
	})
	if err == nil {
		return mqtt.OutcomeAccepted
	}
	if errors.Is(err, edge.ErrInvalid) {
		return mqtt.OutcomeRejected
	}
	return mqtt.OutcomeRetry
}

// deviceStatusConsumer is the adapter between the MQTT ingest DTO and the
// Device domain. It passes only the source identity, parsed current snapshot,
// and Cloud ReceivedAt across the boundary.
type deviceStatusConsumer struct{ service *device.Service }

func newDeviceStatusConsumer(service *device.Service) mqtt.Consumer {
	if service == nil {
		return mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		})
	}
	return deviceStatusConsumer{service: service}
}

func (c deviceStatusConsumer) Consume(ctx context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
	var status mqtt.DeviceStatusMessage
	switch value := message.(type) {
	case mqtt.DeviceStatusMessage:
		status = value
	case *mqtt.DeviceStatusMessage:
		if value != nil {
			status = *value
		} else {
			return mqtt.OutcomeRejected
		}
	default:
		return mqtt.OutcomeAccepted
	}
	if status.Metadata().DeviceID == nil || *status.Metadata().DeviceID == "" {
		return mqtt.OutcomeRejected
	}
	snapshot, err := device.ParseSnapshot(status.Data)
	if err != nil {
		return mqtt.OutcomeRejected
	}
	_, err = c.service.Observe(ctx, device.Observation{
		EdgeID:         status.Metadata().EdgeID,
		SourceDeviceID: *status.Metadata().DeviceID,
		Snapshot:       snapshot,
		ReceivedAt:     status.Metadata().ReceivedAt,
	})
	switch {
	case err == nil:
		return mqtt.OutcomeAccepted
	case errors.Is(err, device.ErrInvalid), errors.Is(err, device.ErrParentNotFound):
		// An unknown parent is a validly parsed delivery that cannot be
		// projected yet. #22 coordinates a later retained replay after Edge
		// registration; this delivery must still be ACKed here.
		return mqtt.OutcomeAccepted
	default:
		return mqtt.OutcomeRetry
	}
}

type ingressConsumer struct {
	edge          mqtt.Consumer
	device        mqtt.Consumer
	raw           mqtt.Consumer
	commandResult mqtt.Consumer
}

func (c ingressConsumer) Consume(ctx context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
	switch message.Kind() {
	case mqtt.KindEdgeStatus:
		return c.edge.Consume(ctx, message)
	case mqtt.KindDeviceStatus:
		return c.device.Consume(ctx, message)
	case mqtt.KindRawRegisterSnapshot:
		if c.raw == nil {
			return mqtt.OutcomeAccepted
		}
		return c.raw.Consume(ctx, message)
	case mqtt.KindCommandResult:
		if c.commandResult == nil {
			return mqtt.OutcomeAccepted
		}
		return c.commandResult.Consume(ctx, message)
	default:
		return mqtt.OutcomeAccepted
	}
}
