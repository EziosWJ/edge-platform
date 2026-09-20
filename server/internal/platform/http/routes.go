package http

import (
	"context"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadinessChecker is intentionally independent of GORM. The application
// supplies a database adapter when it assembles the router.
type ReadinessChecker interface {
	Ready(context.Context) error
}

const (
	MQTTStateDisabled     = "DISABLED"
	MQTTStateConnecting   = "CONNECTING"
	MQTTStateSubscribing  = "SUBSCRIBING"
	MQTTStateReady        = "READY"
	MQTTStateReconnecting = "RECONNECTING"
	MQTTStateStopping     = "STOPPING"
	MQTTStateStopped      = "STOPPED"
)

// MQTTSubscriptionStatus is a stable, domain-neutral projection of the four
// M1 subscriptions. It deliberately contains no MQTT envelope or raw payload.
type MQTTSubscriptionStatus struct {
	EdgeStatus   bool `json:"edgeStatus"`
	DeviceStatus bool `json:"deviceStatus"`
	Raw          bool `json:"raw"`
	Event        bool `json:"event"`
}

// MQTTStatus is the redacted diagnostic projection exposed by the HTTP layer.
// Implementations must never put broker URLs, credentials, certificate data,
// topic instances, message IDs, or raw client errors in this value.
type MQTTStatus struct {
	Enabled         bool                   `json:"enabled"`
	State           string                 `json:"state"`
	ProtocolVersion string                 `json:"protocolVersion"`
	ConnectedAt     *time.Time             `json:"connectedAt"`
	DisconnectedAt  *time.Time             `json:"disconnectedAt"`
	LastMessageAt   *time.Time             `json:"lastMessageAt"`
	LastErrorAt     *time.Time             `json:"lastErrorAt"`
	LastErrorCode   string                 `json:"lastErrorCode"`
	RetryCount      uint64                 `json:"retryCount"`
	NextRetryAt     *time.Time             `json:"nextRetryAt"`
	Subscriptions   MQTTSubscriptionStatus `json:"subscriptions"`
}

// MQTTRuntime is the seam between app composition and the in-process MQTT
// implementation. Runtime implementations own broker connections, message
// delivery, and their internal DTOs; callers only need lifecycle, readiness,
// and this redacted status projection.
type MQTTRuntime interface {
	Start(context.Context) error
	Stop(context.Context) error
	Ready(context.Context) error
	Status() MQTTStatus
}

type SystemRoutes struct {
	Readiness ReadinessChecker
	// Metrics writes Prometheus-compatible plaintext. It does not receive the
	// default CORS headers even if CORS is installed globally.
	Metrics gin.HandlerFunc
}

// RegisterSystemRoutes adds unauthenticated operational endpoints.
func RegisterSystemRoutes(router gin.IRouter, routes SystemRoutes) {
	router.GET("/health", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok"})
	})
	router.GET("/ready", readinessHandler(routes.Readiness))
	if routes.Metrics != nil {
		router.GET("/metrics", routes.Metrics)
		return
	}
	router.GET("/metrics", defaultMetricsHandler)
}

// RegisterMQTTStatusRoute registers the authenticated MQTT diagnostic route.
// The caller is responsible for placing it on an authenticated route group.
func RegisterMQTTStatusRoute(router gin.IRouter, runtime MQTTRuntime) {
	router.GET("/mqtt/status", func(c *gin.Context) {
		if runtime == nil {
			WriteError(c, stdhttp.StatusServiceUnavailable, stdhttp.StatusServiceUnavailable, "service unavailable", nil)
			return
		}
		OK(c, runtime.Status())
	})
}

func readinessHandler(checker ReadinessChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker != nil && checker.Ready(c.Request.Context()) == nil {
			OK(c, gin.H{"status": "ok"})
			return
		}
		WriteError(c, stdhttp.StatusServiceUnavailable, stdhttp.StatusServiceUnavailable, "service unavailable", nil)
	}
}

func defaultMetricsHandler(c *gin.Context) {
	promhttp.Handler().ServeHTTP(c.Writer, c.Request)
}
