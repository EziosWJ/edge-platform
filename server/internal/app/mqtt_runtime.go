package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/EziosWJ/edge-platform/server/internal/edge"
	"github.com/EziosWJ/edge-platform/server/internal/mqtt"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/prometheus/client_golang/prometheus"
)

var errMQTTNotReady = errors.New("mqtt runtime is not ready")

// newMQTTRuntime selects the injected runtime used by tests or constructs the
// production MQTT runtime. MQTT remains an in-process component; this adapter
// is the only place where the internal ingest types meet the HTTP seam.
func newMQTTRuntime(cfg config.MQTTConfig, runtime platformhttp.MQTTRuntime, edgeServices ...*edge.Service) (platformhttp.MQTTRuntime, error) {
	if runtime != nil {
		return runtime, nil
	}
	var edgeService *edge.Service
	if len(edgeServices) > 0 {
		edgeService = edgeServices[0]
	}

	consumer := newEdgeStatusConsumer(edgeService)
	internalConfig := mqtt.Config{
		Enabled:           cfg.Enabled,
		BrokerURL:         cfg.URL,
		Protocol:          mqtt.Protocol(cfg.Protocol),
		ClientID:          cfg.ClientID,
		TopicPrefix:       cfg.Prefix,
		Username:          cfg.Username,
		Password:          cfg.Password,
		TLS:               mqtt.TLSConfig{CACertFile: cfg.CAFile, ClientCertFile: cfg.ClientCertFile, ClientKeyFile: cfg.ClientKeyFile},
		KeepAlive:         cfg.KeepAlive,
		ConnectTimeout:    cfg.ConnectTimeout,
		ReconnectMin:      cfg.ReconnectMin,
		ReconnectMax:      cfg.ReconnectMax,
		SessionExpiry:     cfg.SessionExpiry,
		MaxPayloadBytes:   int(cfg.MaxPayloadBytes),
		ReliableQueueSize: cfg.ReliableQueueSize,
		RawQueueSize:      cfg.RawQueueSize,
		ConsumerTimeout:   cfg.ConsumerTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
	}
	internalRuntime, err := mqtt.NewRuntime(internalConfig, consumer, nil, mqtt.NewMetrics(prometheus.DefaultRegisterer), nil)
	if err != nil {
		return nil, fmt.Errorf("build MQTT runtime: %w", err)
	}
	return &mqttRuntimeAdapter{runtime: internalRuntime}, nil
}

type mqttRuntimeAdapter struct {
	runtime *mqtt.Runtime
}

func (a *mqttRuntimeAdapter) Start(ctx context.Context) error { return a.runtime.Start(ctx) }

func (a *mqttRuntimeAdapter) Stop(ctx context.Context) error { return a.runtime.Stop(ctx) }

func (a *mqttRuntimeAdapter) Ready(context.Context) error {
	if a.runtime.Ready() {
		return nil
	}
	return errMQTTNotReady
}

func (a *mqttRuntimeAdapter) Status() platformhttp.MQTTStatus {
	status := a.runtime.Status()
	result := platformhttp.MQTTStatus{
		Enabled:         status.Enabled,
		State:           string(status.State),
		ProtocolVersion: string(status.Protocol),
		ConnectedAt:     optionalTime(status.ConnectedAt),
		DisconnectedAt:  optionalTime(status.DisconnectedAt),
		LastMessageAt:   optionalTime(status.LastMessageAt),
		LastErrorAt:     optionalTime(status.RecentErrorAt),
		LastErrorCode:   status.RecentErrorCode,
		RetryCount:      uint64(max(status.RetryCount, 0)),
		NextRetryAt:     optionalTime(status.NextRetryAt),
	}
	for filter, ready := range status.Subscriptions {
		switch {
		case strings.HasSuffix(filter, "/device/+/status"):
			result.Subscriptions.DeviceStatus = ready
		case strings.HasSuffix(filter, "/device/+/raw"):
			result.Subscriptions.Raw = ready
		case strings.HasSuffix(filter, "/device/+/event"):
			result.Subscriptions.Event = ready
		case strings.HasSuffix(filter, "/+/status"):
			result.Subscriptions.EdgeStatus = ready
		}
	}
	return result
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
