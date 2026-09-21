package app

import (
	"context"
	"encoding/json"
	"errors"

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
