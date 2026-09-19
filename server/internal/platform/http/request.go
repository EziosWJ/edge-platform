package http

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestIDHeader = "X-Request-ID"

type requestMetaKey struct{}
type userIDKey struct{}

// RequestMeta is request-scoped information safe for Service to consume via
// context.Context without depending on Gin.
type RequestMeta struct {
	RequestID string
	ClientIP  string
	UserAgent string
}

var fallbackRequestSequence atomic.Uint64

// RequestMetadata creates or forwards a request ID and stores request metadata
// in the standard request context.
func RequestMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}

		meta := RequestMeta{
			RequestID: requestID,
			ClientIP:  c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		}
		ctx := context.WithValue(c.Request.Context(), requestMetaKey{}, meta)
		c.Request = c.Request.WithContext(ctx)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func RequestMetaFromContext(ctx context.Context) (RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta, ok
}

func RequestIDFromContext(ctx context.Context) string {
	meta, ok := RequestMetaFromContext(ctx)
	if !ok {
		return ""
	}
	return meta.RequestID
}

// ContextWithUserID lets authentication middleware expose its authenticated
// user to request logging without coupling this package to the auth module.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey{}).(int64)
	return userID, ok
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := cryptorand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("%x-%x", time.Now().UnixNano(), fallbackRequestSequence.Add(1))
}
