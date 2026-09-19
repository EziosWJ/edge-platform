package http

import (
	"errors"
	"log/slog"
	"mime"
	stdhttp "net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

const defaultMultipartConcurrency = 8

// MultipartPolicy describes the raw HTTP body budget for one registered
// multipart route. The budget includes multipart boundaries and form fields.
type MultipartPolicy struct {
	MaxBodyBytes int64
}

// MultipartProtectionConfig configures the global multipart boundary. Every
// multipart route must be present in Policies before its body can be read.
type MultipartProtectionConfig struct {
	Policies      map[string]MultipartPolicy
	MaxConcurrent int
	Logger        *slog.Logger
}

var (
	multipartMetricsOnce sync.Once
	multipartRejections  = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "edge_platform_multipart_rejections_total",
			Help: "Total multipart requests rejected before or during bounded parsing.",
		},
		[]string{"route", "reason"},
	)
	multipartInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "edge_platform_multipart_in_flight",
		Help: "Current number of active multipart requests.",
	})
)

func registerMultipartMetrics() {
	multipartMetricsOnce.Do(func() {
		prometheus.MustRegister(multipartRejections, multipartInFlight)
	})
}

// MultipartProtection enforces route-specific body limits before downstream
// handlers can invoke multipart parsing. It also bounds concurrent multipart
// requests and records rejection reasons for operational visibility.
func MultipartProtection(config MultipartProtectionConfig) gin.HandlerFunc {
	registerMultipartMetrics()

	policies := make(map[string]MultipartPolicy, len(config.Policies))
	for route, policy := range config.Policies {
		policies[route] = policy
	}
	maxConcurrent := config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMultipartConcurrency
	}
	semaphore := make(chan struct{}, maxConcurrent)
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		if !isMultipartRequest(c.Request) {
			c.Next()
			return
		}

		route := c.FullPath()
		policy, ok := policies[route]
		if !ok || policy.MaxBodyBytes <= 0 {
			recordMultipartRejection(c, logger, "policy_missing")
			AbortError(c, stdhttp.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "未登记的 multipart 接口", nil)
			return
		}

		if c.Request.ContentLength > policy.MaxBodyBytes {
			recordMultipartRejection(c, logger, "body_too_large")
			AbortError(c, stdhttp.StatusRequestEntityTooLarge, CodeRequestEntityTooLarge, "请求体超过大小限制", nil)
			return
		}

		select {
		case semaphore <- struct{}{}:
			multipartInFlight.Inc()
			defer func() {
				<-semaphore
				multipartInFlight.Dec()
			}()
		default:
			recordMultipartRejection(c, logger, "concurrency_limit")
			AbortError(c, stdhttp.StatusServiceUnavailable, CodeServiceUnavailable, "上传服务繁忙，请稍后重试", nil)
			return
		}

		c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, policy.MaxBodyBytes)
		c.Next()
	}
}

// RecordMultipartRejection records a rejection detected by a downstream
// bounded multipart parser, such as an exceeded MaxBytesReader budget.
func RecordMultipartRejection(c *gin.Context, logger *slog.Logger, reason string) {
	registerMultipartMetrics()
	recordMultipartRejection(c, logger, reason)
}

func recordMultipartRejection(c *gin.Context, logger *slog.Logger, reason string) {
	if logger == nil {
		logger = slog.Default()
	}
	route := c.FullPath()
	if route == "" {
		route = "unknown"
	}
	multipartRejections.WithLabelValues(route, reason).Inc()
	logger.WarnContext(c.Request.Context(), "multipart request rejected",
		slog.String("route", route),
		slog.String("path", c.Request.URL.Path),
		slog.String("reason", reason),
	)
}

func isMultipartRequest(request *stdhttp.Request) bool {
	contentType := request.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		return strings.EqualFold(mediaType, "multipart/form-data")
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "multipart/form-data")
}

// IsRequestBodyTooLarge reports whether an error came from MaxBytesReader.
func IsRequestBodyTooLarge(err error) bool {
	var maxBytesError *stdhttp.MaxBytesError
	return errors.As(err, &maxBytesError)
}
