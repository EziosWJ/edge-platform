package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/EziosWJ/edge-platform/server/internal/command"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
	"github.com/prometheus/client_golang/prometheus"
)

type commandResultObserver interface {
	Observe(string)
}

type commandResultConsumer struct {
	service  *command.Service
	observer commandResultObserver
}

func newCommandResultConsumer(service *command.Service) mqtt.Consumer {
	return newCommandResultConsumerWithObserver(service, nil)
}

func newCommandResultConsumerWithObserver(service *command.Service, observer commandResultObserver) mqtt.Consumer {
	if service == nil {
		return mqtt.ConsumerFunc(func(context.Context, mqtt.IngressMessage) mqtt.DeliveryOutcome {
			return mqtt.OutcomeAccepted
		})
	}
	return commandResultConsumer{service: service, observer: observer}
}

func (c commandResultConsumer) Consume(ctx context.Context, message mqtt.IngressMessage) mqtt.DeliveryOutcome {
	var result mqtt.CommandResultMessage
	switch value := message.(type) {
	case mqtt.CommandResultMessage:
		result = value
	case *mqtt.CommandResultMessage:
		if value == nil {
			return mqtt.OutcomeAccepted
		}
		result = *value
	default:
		return mqtt.OutcomeAccepted
	}
	metadata := result.Metadata()
	if metadata.DeviceID == nil || *metadata.DeviceID == "" {
		return c.commandResultPermanent("identity")
	}
	observation, err := command.DecodeResult(result.Data)
	if err != nil {
		return c.commandResultPermanent("contract")
	}
	observation.EdgeID = metadata.EdgeID
	observation.SourceDeviceID = *metadata.DeviceID
	observation.CloudReceivedAt = metadata.ReceivedAt
	projection, err := c.service.ProjectResultDecision(ctx, observation)
	switch {
	case err == nil:
		if projection.Disposition == "" {
			projection.Disposition = command.ResultApplied
		}
		if c.observer != nil {
			c.observer.Observe(string(projection.Disposition))
		}
		if projection.Disposition == command.ResultConflict {
			slog.Default().Warn("command result terminal conflict", "reason", "first_terminal_wins")
		}
		return mqtt.OutcomeAccepted
	case errors.Is(err, command.ErrInvalidResult), errors.Is(err, command.ErrUnknownCommand),
		errors.Is(err, command.ErrResultRouteMismatch), errors.Is(err, command.ErrResultNameMismatch):
		return c.commandResultPermanent(resultReason(err))
	default:
		// Do not include payload, result, error message, or IDs in logs. A
		// database outage remains retryable and the MQTT runtime withholds ACK.
		slog.Default().Warn("command result projection failed", "reason", "persistence")
		return mqtt.OutcomeRetry
	}
}

func (c commandResultConsumer) commandResultPermanent(reason string) mqtt.DeliveryOutcome {
	if c.observer != nil {
		c.observer.Observe(reason)
	}
	slog.Default().Warn("command result acknowledged without projection", "reason", reason)
	return mqtt.OutcomeRejected
}

type commandResultMetricObserver struct {
	events *prometheus.CounterVec
}

func newCommandResultMetricObserver(reg prometheus.Registerer) commandResultObserver {
	events := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "command_result_projection_events_total",
		Help: "Classified command result projection outcomes.",
	}, []string{"classification"})
	if reg != nil {
		if err := reg.Register(events); err != nil {
			var alreadyRegistered prometheus.AlreadyRegisteredError
			if errors.As(err, &alreadyRegistered) {
				if existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec); ok {
					events = existing
				}
			}
		}
	}
	return commandResultMetricObserver{events: events}
}

func (o commandResultMetricObserver) Observe(classification string) {
	if o.events != nil {
		o.events.WithLabelValues(classification).Inc()
	}
}

func resultReason(err error) string {
	switch {
	case errors.Is(err, command.ErrInvalidResult):
		return "contract"
	case errors.Is(err, command.ErrUnknownCommand):
		return "unknown_command"
	case errors.Is(err, command.ErrResultRouteMismatch):
		return "route"
	case errors.Is(err, command.ErrResultNameMismatch):
		return "name"
	default:
		return "rejected"
	}
}
