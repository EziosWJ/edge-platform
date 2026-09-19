package http

import (
	"context"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadinessChecker is intentionally independent of GORM. The application
// supplies a database adapter when it assembles the router.
type ReadinessChecker interface {
	Ready(context.Context) error
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
