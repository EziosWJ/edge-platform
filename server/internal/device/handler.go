package device

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type HandlerService interface {
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (Device, error)
}

type Handler struct{ service HandlerService }

// ApiEnvelope is the Swagger representation of the established API response.
//
//nolint:unused // referenced by Swaggo annotations below
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(service HandlerService) (*Handler, error) {
	if service == nil {
		return nil, errors.New("device handler service is required")
	}
	return &Handler{service: service}, nil
}

func RegisterRoutes(router gin.IRouter, handler *Handler) {
	router.GET("/page", handler.page)
	router.GET("/:deviceId", handler.detail)
}

// page godoc
// @Summary Device 分页查询
// @Tags Device
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500，默认 10"
// @Param edgeId query string false "所属 Edge ID，精确匹配"
// @Param deviceId query string false "Cloud Device ID，精确匹配"
// @Param sourceDeviceId query string false "来源 Device ID，精确匹配"
// @Param status query string false "Device 状态：INITIAL、ONLINE、DEGRADED 或 OFFLINE"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/device/page [get]
func (h *Handler) page(c *gin.Context) {
	query, ok := pageQuery(c)
	if !ok {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
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
// @Summary Device 详情
// @Tags Device
// @Security BearerAuth
// @Param deviceId path string true "Cloud Device ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/device/{deviceId} [get]
func (h *Handler) detail(c *gin.Context) {
	deviceID := c.Param("deviceId")
	if deviceID == "" {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	result, err := h.service.Detail(c.Request.Context(), deviceID)
	if err != nil {
		handleError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

func pageQuery(c *gin.Context) (PageQuery, bool) {
	page, ok := queryInt(c, "page", 1)
	if !ok || page < 1 {
		return PageQuery{}, false
	}
	pageSize, ok := queryInt(c, "pageSize", 10)
	if !ok || pageSize < 1 || pageSize > 500 {
		return PageQuery{}, false
	}
	query := PageQuery{Page: page, PageSize: pageSize}
	for _, filter := range []struct {
		key    string
		target *string
	}{
		{key: "edgeId", target: &query.EdgeID},
		{key: "deviceId", target: &query.DeviceID},
		{key: "sourceDeviceId", target: &query.SourceDeviceID},
	} {
		if value, present := c.GetQuery(filter.key); present {
			if value == "" {
				return PageQuery{}, false
			}
			*filter.target = value
		}
	}
	if value, present := c.GetQuery("status"); present {
		status := CommunicationStatus(value)
		if !validStatus(status) {
			return PageQuery{}, false
		}
		query.Status = &status
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
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusNotFound, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrInvalid):
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
