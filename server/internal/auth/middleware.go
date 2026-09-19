package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

// BearerMiddleware accepts exactly a Bearer token, normalizes its scheme for
// the Service, and propagates the authenticated identity through context.Context.
func BearerMiddleware(authenticator Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := normalizedBearerToken(c.GetHeader("Authorization"))
		if !ok {
			writeUnauthorized(c)
			return
		}

		principal, err := authenticator.Authenticate(c.Request.Context(), token)
		if err != nil {
			if platformhttp.IsTemporaryUnavailable(err) {
				c.Abort()
				platformhttp.TemporaryUnavailable(c)
				return
			}
			writeUnauthorized(c)
			return
		}

		ctx := ContextWithPrincipal(c.Request.Context(), principal)
		ctx = platformhttp.ContextWithUserID(ctx, principal.UserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func normalizedBearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return "Bearer " + parts[1], true
}

func writeUnauthorized(c *gin.Context) {
	platformhttp.AbortError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, ErrUnauthenticated.Error(), nil)
}
