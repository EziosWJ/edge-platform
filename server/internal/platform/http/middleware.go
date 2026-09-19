package http

import (
	"log/slog"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSConfig permits all browser origins when AllowedOrigins is empty. When it
// is configured, only an exact origin match is permitted. Credentials are never
// enabled because API authentication uses Bearer tokens.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
	MaxAge         time.Duration
}

func CORS(config CORSConfig) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowedOrigins[origin] = struct{}{}
		}
	}
	allowAnyOrigin := len(allowedOrigins) == 0
	if _, ok := allowedOrigins["*"]; ok {
		allowAnyOrigin = true
	}
	allowedMethods := strings.Join(orDefault(config.AllowedMethods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}), ", ")
	allowedHeaders := strings.Join(orDefault(config.AllowedHeaders, []string{"Authorization", "Content-Type", RequestIDHeader}), ", ")
	exposedHeaders := strings.Join(orDefault(config.ExposedHeaders, []string{RequestIDHeader}), ", ")

	return func(c *gin.Context) {
		// Metrics are intended for Prometheus, not browser clients.
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if allowAnyOrigin {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			if _, allowed := allowedOrigins[origin]; !allowed {
				AbortError(c, stdhttp.StatusForbidden, CodeForbidden, "无权限", nil)
				return
			}
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Expose-Headers", exposedHeaders)
		if config.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", strconv.FormatInt(int64(config.MaxAge.Seconds()), 10))
		}

		if c.Request.Method == stdhttp.MethodOptions {
			c.AbortWithStatus(stdhttp.StatusNoContent)
			return
		}
		c.Next()
	}
}

func orDefault(values, defaults []string) []string {
	if len(values) == 0 {
		return defaults
	}
	return values
}

// RequestLogger emits one structured log record after every request.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		attributes := []slog.Attr{
			slog.String("request_id", RequestIDFromContext(c.Request.Context())),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(startedAt)),
		}
		if meta, ok := RequestMetaFromContext(c.Request.Context()); ok {
			attributes = append(attributes, slog.String("client_ip", meta.ClientIP))
		}
		if userID, ok := UserIDFromContext(c.Request.Context()); ok {
			attributes = append(attributes, slog.Int64("user_id", userID))
		}
		if len(c.Errors) > 0 {
			attributes = append(attributes, slog.String("error", c.Errors.String()))
		}
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "http request completed", attributes...)
	}
}

// Recovery converts unexpected panics into the established API error envelope.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.ErrorContext(c.Request.Context(), "http request panicked",
			slog.String("request_id", RequestIDFromContext(c.Request.Context())),
			slog.Any("panic", recovered),
		)
		AbortError(c, stdhttp.StatusInternalServerError, CodeInternalError, "系统错误", nil)
	})
}
