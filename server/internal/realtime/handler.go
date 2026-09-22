package realtime

import (
	"errors"
	"net/http"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) (*Handler, error) {
	if service == nil {
		return nil, errors.New("realtime service is required")
	}
	return &Handler{service: service}, nil
}

func RegisterRoutes(router gin.IRouter, handler *Handler, authenticator auth.Authenticator) {
	group := router.Group("/api/realtime")
	protected := group.Group("")
	protected.Use(auth.BearerMiddleware(authenticator))
	protected.POST("/ticket", handler.issueTicket)
	group.GET("/ws", handler.websocket)
}

type ticketResponseEnvelope struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    TicketResponse `json:"data"`
}

// issueTicket godoc
// @Summary 创建 WebSocket 一次性 ticket
// @Tags Realtime
// @Security BearerAuth
// @Success 200 {object} ticketResponseEnvelope
// @Failure 401 {object} ticketResponseEnvelope
// @Router /api/realtime/ticket [post]
func (h *Handler) issueTicket(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		platformhttp.AbortError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, auth.ErrUnauthenticated.Error(), nil)
		return
	}
	result, err := h.service.IssueTicket(c.Request.Context(), principal)
	if err != nil {
		if errors.Is(err, ErrSessionInvalid) {
			platformhttp.WriteError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, auth.ErrUnauthenticated.Error(), nil)
			return
		}
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
		return
	}
	platformhttp.OK(c, result)
}

func (h *Handler) websocket(c *gin.Context) {
	if strings.TrimSpace(c.Query("ticket")) == "" {
		http.Error(c.Writer, "invalid realtime ticket", http.StatusUnauthorized)
		return
	}
	h.service.serveWebSocket(c)
}
