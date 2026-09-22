package datapoint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type HandlerService interface {
	Create(context.Context, audit.Metadata, CreateInput) (DataPoint, error)
	Page(context.Context, PageQuery) (Page, error)
	Detail(context.Context, string) (DataPoint, error)
	Update(context.Context, audit.Metadata, string, UpdateInput) (DataPoint, error)
	SetEnabled(context.Context, audit.Metadata, string, bool) (DataPoint, error)
}
type Handler struct{ service HandlerService }

// ApiEnvelope is the common Swagger response envelope.
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type CurrentValueView struct {
	Value           any        `json:"value"`
	Quality         Quality    `json:"quality"`
	SourceTimestamp *time.Time `json:"sourceTimestamp"`
	ObservedAt      *time.Time `json:"observedAt"`
	Revision        int64      `json:"revision"`
}
type DataPointView struct {
	DataPointID  string           `json:"dataPointId"`
	DeviceID     string           `json:"deviceId"`
	PointKey     string           `json:"pointKey"`
	Name         string           `json:"name"`
	ValueType    ValueType        `json:"valueType"`
	Unit         *string          `json:"unit"`
	Precision    *int             `json:"precision"`
	Enabled      bool             `json:"enabled"`
	CurrentValue CurrentValueView `json:"currentValue"`
	Mapping      *SourceMapping   `json:"mapping,omitempty"`
}
type PageView struct {
	Records  []DataPointView `json:"records"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

type createRequest struct {
	DeviceID  string        `json:"deviceId"`
	PointKey  string        `json:"pointKey"`
	Name      string        `json:"name"`
	ValueType ValueType     `json:"valueType"`
	Unit      *string       `json:"unit"`
	Precision *int          `json:"precision"`
	Enabled   *bool         `json:"enabled"`
	Mapping   SourceMapping `json:"mapping"`
}
type updateRequest struct {
	PointKey  *string       `json:"pointKey"`
	ValueType *ValueType    `json:"valueType"`
	Name      string        `json:"name"`
	Unit      *string       `json:"unit"`
	Precision *int          `json:"precision"`
	Mapping   SourceMapping `json:"mapping"`
}
type enabledRequest struct {
	Enabled *bool `json:"enabled"`
}

func NewHandler(service HandlerService) (*Handler, error) {
	if service == nil {
		return nil, ErrInvalid
	}
	return &Handler{service: service}, nil
}
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	router.GET("/page", handler.page)
	router.GET("/:dataPointId", handler.detail)
	router.POST("", handler.create)
	router.PUT("/:dataPointId", handler.update)
	router.PUT("/:dataPointId/enabled", handler.enabled)
}

// page godoc
// @Summary DataPoint 分页查询
// @Tags DataPoint
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500，默认 10"
// @Param deviceId query string false "Cloud Device ID，精确匹配"
// @Param pointKey query string false "语义 pointKey，精确匹配"
// @Param valueType query string false "NUMBER 或 BOOLEAN"
// @Param enabled query bool false "是否启用"
// @Param quality query string false "NO_DATA、GOOD 或 BAD"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/datapoint/page [get]
func (h *Handler) page(c *gin.Context) {
	query, ok := parsePageQuery(c)
	if !ok {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	result, err := h.service.Page(c.Request.Context(), query)
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]DataPointView, len(result.Records))
	for i := range result.Records {
		views[i] = view(result.Records[i], false)
	}
	platformhttp.OK(c, PageView{Records: views, Total: result.Total, Page: result.Page, PageSize: result.PageSize})
}

// detail godoc
// @Summary DataPoint 详情
// @Tags DataPoint
// @Security BearerAuth
// @Param dataPointId path string true "DataPoint ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/datapoint/{dataPointId} [get]
func (h *Handler) detail(c *gin.Context) {
	result, err := h.service.Detail(c.Request.Context(), c.Param("dataPointId"))
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, view(result, true))
}

// create godoc
// @Summary 创建 DataPoint
// @Tags DataPoint
// @Security BearerAuth
// @Param body body createRequest true "DataPoint 配置"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Router /api/datapoint [post]
func (h *Handler) create(c *gin.Context) {
	var request createRequest
	if c.ShouldBindJSON(&request) != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	result, err := h.service.Create(c.Request.Context(), auditMetadata(c), CreateInput{DeviceID: request.DeviceID, PointKey: request.PointKey, Name: request.Name, ValueType: request.ValueType, Unit: request.Unit, Precision: request.Precision, Enabled: enabled, Mapping: request.Mapping})
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, view(result, true))
}

// update godoc
// @Summary 修改 DataPoint metadata 或 SourceMapping
// @Tags DataPoint
// @Security BearerAuth
// @Param dataPointId path string true "DataPoint ID"
// @Param body body updateRequest true "DataPoint 配置"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/datapoint/{dataPointId} [put]
func (h *Handler) update(c *gin.Context) {
	var request updateRequest
	if c.ShouldBindJSON(&request) != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	if request.PointKey != nil || request.ValueType != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrImmutable.Error(), nil)
		return
	}
	result, err := h.service.Update(c.Request.Context(), auditMetadata(c), c.Param("dataPointId"), UpdateInput{Name: request.Name, Unit: request.Unit, Precision: request.Precision, Mapping: request.Mapping})
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, view(result, true))
}

// enabled godoc
// @Summary 启用或禁用 DataPoint
// @Tags DataPoint
// @Security BearerAuth
// @Param dataPointId path string true "DataPoint ID"
// @Param body body enabledRequest true "enabled 状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/datapoint/{dataPointId}/enabled [put]
func (h *Handler) enabled(c *gin.Context) {
	var request enabledRequest
	if c.ShouldBindJSON(&request) != nil || request.Enabled == nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	result, err := h.service.SetEnabled(c.Request.Context(), auditMetadata(c), c.Param("dataPointId"), *request.Enabled)
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, view(result, true))
}

func view(point DataPoint, includeMapping bool) DataPointView {
	result := DataPointView{DataPointID: point.DataPointID, DeviceID: point.DeviceID, PointKey: point.PointKey, Name: point.Name, ValueType: point.ValueType, Unit: point.Unit, Precision: point.Precision, Enabled: point.Enabled, CurrentValue: CurrentValueView{Value: point.Current.Value(), Quality: point.Current.Quality, SourceTimestamp: point.Current.SourceTimestamp, ObservedAt: point.Current.ObservedAt, Revision: point.Current.Revision}}
	if includeMapping {
		mapping := point.Mapping
		result.Mapping = &mapping
	}
	return result
}
func parsePageQuery(c *gin.Context) (PageQuery, bool) {
	page := 1
	size := 10
	if value, ok := c.GetQuery("page"); ok {
		if _, err := fmt.Sscan(value, &page); err != nil {
			return PageQuery{}, false
		}
	}
	if value, ok := c.GetQuery("pageSize"); ok {
		if _, err := fmt.Sscan(value, &size); err != nil {
			return PageQuery{}, false
		}
	}
	if page < 1 || size < 1 || size > 500 {
		return PageQuery{}, false
	}
	q := PageQuery{Page: page, PageSize: size}
	if value, ok := c.GetQuery("deviceId"); ok {
		if value == "" {
			return PageQuery{}, false
		}
		q.DeviceID = value
	}
	if value, ok := c.GetQuery("pointKey"); ok {
		if value == "" || !ValidatePointKey(value) {
			return PageQuery{}, false
		}
		q.PointKey = value
	}
	if value, ok := c.GetQuery("valueType"); ok {
		v := ValueType(value)
		if !validValueType(v) {
			return PageQuery{}, false
		}
		q.ValueType = &v
	}
	if value, ok := c.GetQuery("enabled"); ok {
		var v bool
		if _, err := fmt.Sscan(value, &v); err != nil {
			return PageQuery{}, false
		}
		q.Enabled = &v
	}
	if value, ok := c.GetQuery("quality"); ok {
		v := Quality(value)
		if !validQuality(v) {
			return PageQuery{}, false
		}
		q.Quality = &v
	}
	return q, true
}
func auditMetadata(c *gin.Context) audit.Metadata {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	meta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return audit.Metadata{ActorID: principal.UserID, RequestID: meta.RequestID, ClientIP: meta.ClientIP, UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()}
}
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUnknownDevice):
		platformhttp.WriteError(c, http.StatusNotFound, platformhttp.CodeNotFound, err.Error(), nil)
	case errors.Is(err, ErrConflict):
		platformhttp.WriteError(c, http.StatusConflict, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrImmutable):
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, err.Error(), nil)
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
