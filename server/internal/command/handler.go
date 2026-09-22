package command

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service    *Service
	authorizer PermissionChecker
}

func NewHandler(service *Service, authorizer PermissionChecker) (*Handler, error) {
	if service == nil {
		return nil, errors.New("command handler service is required")
	}
	if authorizer == nil {
		return nil, errors.New("command permission checker is required")
	}
	return &Handler{service: service, authorizer: authorizer}, nil
}

func RegisterRoutes(router gin.IRouter, handler *Handler) {
	router.GET("/page", handler.page)
	router.GET("/:commandId", handler.detail)
	router.POST("", handler.create)
}

type createRequest struct {
	CommandID  string          `json:"commandId"`
	DeviceID   string          `json:"deviceId"`
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args"`
	TTLSeconds *int            `json:"ttlSeconds"`
}

// ApiEnvelope is the Swagger representation of the established API response.
//
//nolint:unused // referenced by Swaggo annotations below
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// createRequestDoc keeps the OpenAPI input schema independent from
// json.RawMessage, which Swaggo cannot expand as a request model.
type createRequestDoc struct {
	CommandID  string         `json:"commandId"`
	DeviceID   string         `json:"deviceId"`
	Name       string         `json:"name"`
	Args       map[string]any `json:"args"`
	TTLSeconds *int           `json:"ttlSeconds,omitempty"`
}

// commandPageResponse is intentionally an envelope-only Swagger model; the
// public CommandView is the data payload and never includes topic or payload.
type commandPageResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    CommandPage `json:"data"`
}

// create godoc
// @Summary 创建并幂等受理 Command
// @Tags Command
// @Security BearerAuth
// @Param request body createRequestDoc true "Command 创建请求"
// @Success 202 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 413 {object} ApiEnvelope
// @Router /api/command [post]
func (h *Handler) create(c *gin.Context) {
	principal, ok := h.authorize(c, ExecutePermission)
	if !ok {
		return
	}

	var request createRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, MaxPayloadBytes+1024))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, ErrPayloadSize.Error(), nil)
			return
		}
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
		return
	}
	metadata := audit.Metadata{ActorID: principal.UserID, RequestID: platformhttp.RequestIDFromContext(c.Request.Context()), ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestMethod: c.Request.Method, RequestURL: c.Request.URL.String()}
	result, err := h.service.Create(c.Request.Context(), principal.UserID, CreateInput{CommandID: request.CommandID, DeviceID: request.DeviceID, Name: request.Name, Args: request.Args, TTLSeconds: request.TTLSeconds}, audit.Event{Action: "command.create", Resource: "command", Summary: "创建 Command", Metadata: metadata})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, platformhttp.Success(result))
}

// page godoc
// @Summary 查询 Command 分页
// @Tags Command
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500，默认 10"
// @Param commandId query string false "Command ID，精确匹配"
// @Param deviceId query string false "Cloud Device ID，精确匹配"
// @Param name query string false "Command 名称，精确匹配"
// @Param status query string false "PENDING、ACCEPTED、REJECTED、EXPIRED、SUCCEEDED 或 FAILED"
// @Param requestedBy query int64 false "创建者用户 ID，严格整数"
// @Success 200 {object} commandPageResponse
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/command/page [get]
func (h *Handler) page(c *gin.Context) {
	if _, ok := h.authorize(c, ListPermission); !ok {
		return
	}
	query, ok := commandPageQuery(c)
	if !ok {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
		return
	}
	result, err := h.service.Page(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

// detail godoc
// @Summary 查询 Command 详情
// @Tags Command
// @Security BearerAuth
// @Param commandId path string true "Command ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/command/{commandId} [get]
func (h *Handler) detail(c *gin.Context) {
	if _, ok := h.authorize(c, DetailPermission); !ok {
		return
	}
	commandID := strings.TrimSpace(c.Param("commandId"))
	if commandID == "" {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
		return
	}
	result, err := h.service.Detail(c.Request.Context(), commandID)
	if err != nil {
		handleError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

func (h *Handler) authorize(c *gin.Context, permission string) (auth.Principal, bool) {
	principal, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok || principal.UserID <= 0 {
		platformhttp.AbortError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, auth.ErrUnauthenticated.Error(), nil)
		return auth.Principal{}, false
	}
	allowed, err := h.authorizer.HasPermission(c.Request.Context(), principal.UserID, permission)
	if err != nil {
		if platformhttp.IsTemporaryUnavailable(err) {
			platformhttp.TemporaryUnavailable(c)
		} else {
			platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
		}
		return auth.Principal{}, false
	}
	if !allowed {
		platformhttp.WriteError(c, http.StatusForbidden, platformhttp.CodeForbidden, ErrForbidden.Error(), nil)
		return auth.Principal{}, false
	}
	return principal, true
}

func commandPageQuery(c *gin.Context) (CommandPageQuery, bool) {
	page, ok := queryInt(c, "page", 1)
	if !ok || page < 1 {
		return CommandPageQuery{}, false
	}
	pageSize, ok := queryInt(c, "pageSize", 10)
	if !ok || pageSize < 1 || pageSize > 500 {
		return CommandPageQuery{}, false
	}
	query := CommandPageQuery{Page: page, PageSize: pageSize}
	for _, filter := range []struct {
		key    string
		target *string
	}{
		{key: "commandId", target: &query.CommandID},
		{key: "deviceId", target: &query.DeviceID},
		{key: "name", target: &query.Name},
	} {
		if value, present := c.GetQuery(filter.key); present {
			if strings.TrimSpace(value) == "" {
				return CommandPageQuery{}, false
			}
			*filter.target = value
		}
	}
	if value, present := c.GetQuery("status"); present {
		status := strings.TrimSpace(value)
		if !validStatusForQuery(status) {
			return CommandPageQuery{}, false
		}
		query.Status = &status
	}
	if value, present := c.GetQuery("requestedBy"); present {
		requestedBy, err := strconv.ParseInt(value, 10, 64)
		if err != nil || requestedBy <= 0 {
			return CommandPageQuery{}, false
		}
		query.RequestedBy = &requestedBy
	}
	return query, true
}

func queryInt(c *gin.Context, key string, defaultValue int) (int, bool) {
	value, present := c.GetQuery(key)
	if !present {
		return defaultValue, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func handleError(c *gin.Context, err error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrConflict):
		platformhttp.WriteError(c, http.StatusConflict, http.StatusConflict, ErrConflict.Error(), nil)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusNotFound, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrPayloadSize):
		platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, ErrPayloadSize.Error(), nil)
	case errors.Is(err, ErrInvalid):
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
