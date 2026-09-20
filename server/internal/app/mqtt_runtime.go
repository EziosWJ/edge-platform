package app

import (
	"context"
	"errors"
	"sync"

	"github.com/EziosWJ/edge-platform/server/internal/config"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

var errMQTTRuntimeUnavailable = errors.New("mqtt runtime implementation is not wired")

// newMQTTRuntime selects the real adapter when one is supplied. Until
// internal/mqtt is connected, the explicit placeholder keeps the HTTP process
// alive while making enabled MQTT readiness fail and reporting no broker facts.
func newMQTTRuntime(cfg config.MQTTConfig, runtime platformhttp.MQTTRuntime) platformhttp.MQTTRuntime {
	if !cfg.Enabled {
		return newPlaceholderMQTTRuntime(false, cfg.Protocol)
	}
	if runtime != nil {
		return runtime
	}
	return newPlaceholderMQTTRuntime(true, cfg.Protocol)
}

type placeholderMQTTRuntime struct {
	mu     sync.RWMutex
	status platformhttp.MQTTStatus
}

func newPlaceholderMQTTRuntime(enabled bool, protocol string) *placeholderMQTTRuntime {
	state := platformhttp.MQTTStateDisabled
	lastErrorCode := ""
	if enabled {
		state = platformhttp.MQTTStateStopped
		lastErrorCode = "RUNTIME_UNAVAILABLE"
	}
	return &placeholderMQTTRuntime{status: platformhttp.MQTTStatus{
		Enabled:         enabled,
		State:           state,
		ProtocolVersion: protocol,
		LastErrorCode:   lastErrorCode,
	}}
}

func (r *placeholderMQTTRuntime) Start(context.Context) error {
	return nil
}

func (r *placeholderMQTTRuntime) Stop(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status.Enabled {
		r.status.State = platformhttp.MQTTStateStopped
	}
	return nil
}

func (r *placeholderMQTTRuntime) Ready(context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.status.Enabled {
		return nil
	}
	return errMQTTRuntimeUnavailable
}

func (r *placeholderMQTTRuntime) Status() platformhttp.MQTTStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}
