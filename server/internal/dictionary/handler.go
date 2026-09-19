package dictionary

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

type HandlerService interface {
	TypePage(context.Context, TypePageQuery) (Page[DictType], error)
	TypeDetail(context.Context, int64) (*DictType, error)
	CreateType(context.Context, AuditMetadata, TypeInput) (DictType, error)
	UpdateType(context.Context, AuditMetadata, int64, TypeInput) (DictType, error)
	DeleteType(context.Context, AuditMetadata, int64) error
	DeleteTypes(context.Context, AuditMetadata, []int64) error
	SetTypeStatus(context.Context, AuditMetadata, int64, int) error
	DataPage(context.Context, DataPageQuery) (Page[DictData], error)
	DataDetail(context.Context, int64) (*DictData, error)
	CreateData(context.Context, AuditMetadata, DataInput) (DictData, error)
	UpdateData(context.Context, AuditMetadata, int64, DataInput) (DictData, error)
	DeleteData(context.Context, AuditMetadata, int64) error
	DeleteDataBatch(context.Context, AuditMetadata, []int64) error
	Items(context.Context, string) ([]DictItem, error)
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
		return nil, errors.New("dictionary handler service is required")
	}
	return &Handler{service: service}, nil
}

// RegisterRoutes registers dictionary routes on an already authenticated
// /api/system router group.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	types := router.Group("/dict-type")
	types.GET("/page", handler.typePage)
	types.GET("/:id", handler.typeDetail)
	types.POST("", handler.createType)
	types.PUT("/:id", handler.updateType)
	types.DELETE("/:id", handler.deleteType)
	types.POST("/batch-delete", handler.deleteTypes)
	types.PATCH("/:id/status", handler.typeStatus)

	data := router.Group("/dict-data")
	data.GET("/page", handler.dataPage)
	data.GET("/:id", handler.dataDetail)
	data.POST("", handler.createData)
	data.PUT("/:id", handler.updateData)
	data.DELETE("/:id", handler.deleteData)
	data.POST("/batch-delete", handler.deleteDataBatch)

	dict := router.Group("/dict")
	dict.GET("/:dictCode/items", handler.items)
}

type typeRequest struct {
	DictName  string  `json:"dictName"`
	DictCode  string  `json:"dictCode"`
	Status    *int    `json:"status"`
	SortOrder *int    `json:"sortOrder"`
	Remark    *string `json:"remark"`
}

type dataRequest struct {
	DictTypeID int64   `json:"dictTypeId"`
	DictLabel  string  `json:"dictLabel"`
	DictValue  string  `json:"dictValue"`
	SortOrder  *int    `json:"sortOrder"`
	Remark     *string `json:"remark"`
}

type batchRequest struct {
	IDs []int64 `json:"ids"`
}

type statusRequest struct {
	Status int `json:"status"`
}

// typePage godoc
// @Summary 字典类型分页
// @Tags 字典类型
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param dictName query string false "字典名称"
// @Param dictCode query string false "字典编码"
// @Param status query int false "状态：0 或 1"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/page [get]
func (h *Handler) typePage(c *gin.Context) {
	status, statusOK := optionalStatus(c, "status")
	query := TypePageQuery{
		Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10),
		DictName: c.Query("dictName"), DictCode: c.Query("dictCode"), Status: status,
	}
	if !validPage(query.Page, query.PageSize) || !statusOK {
		badRequest(c, map[string]string{"page": "页码或每页条数不合法"})
		return
	}
	value, err := h.service.TypePage(c, query)
	writeResult(c, value, err)
}

// typeDetail godoc
// @Summary 字典类型详情
// @Tags 字典类型
// @Security BearerAuth
// @Param id path int true "字典类型 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/{id} [get]
func (h *Handler) typeDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	value, err := h.service.TypeDetail(c, id)
	writeResult(c, value, err)
}

// createType godoc
// @Summary 新增字典类型
// @Tags 字典类型
// @Security BearerAuth
// @Param request body typeRequest true "字典类型"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type [post]
func (h *Handler) createType(c *gin.Context) {
	request, ok := bindTypeRequest(c)
	if !ok {
		return
	}
	_, err := h.service.CreateType(c, auditMetadata(c), request.input())
	writeMutation(c, err)
}

// updateType godoc
// @Summary 修改字典类型
// @Tags 字典类型
// @Security BearerAuth
// @Param id path int true "字典类型 ID"
// @Param request body typeRequest true "字典类型"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/{id} [put]
func (h *Handler) updateType(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	request, ok := bindTypeRequest(c)
	if !ok {
		return
	}
	_, err := h.service.UpdateType(c, auditMetadata(c), id, request.input())
	writeMutation(c, err)
}

// deleteType godoc
// @Summary 删除字典类型
// @Tags 字典类型
// @Security BearerAuth
// @Param id path int true "字典类型 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/{id} [delete]
func (h *Handler) deleteType(c *gin.Context) {
	id, ok := pathID(c)
	if ok {
		writeMutation(c, h.service.DeleteType(c, auditMetadata(c), id))
	}
}

// deleteTypes godoc
// @Summary 批量删除字典类型
// @Tags 字典类型
// @Security BearerAuth
// @Param request body batchRequest true "字典类型 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/batch-delete [post]
func (h *Handler) deleteTypes(c *gin.Context) {
	var request batchRequest
	if !bindJSON(c, &request) {
		return
	}
	if len(request.IDs) == 0 {
		badRequest(c, map[string]string{"ids": "ID 列表不能为空"})
		return
	}
	writeMutation(c, h.service.DeleteTypes(c, auditMetadata(c), request.IDs))
}

// typeStatus godoc
// @Summary 修改字典类型状态
// @Tags 字典类型
// @Security BearerAuth
// @Param id path int true "字典类型 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-type/{id}/status [patch]
func (h *Handler) typeStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request statusRequest
	if !bindJSON(c, &request) {
		return
	}
	if !validStatus(request.Status) {
		badRequest(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	writeMutation(c, h.service.SetTypeStatus(c, auditMetadata(c), id, request.Status))
}

// dataPage godoc
// @Summary 字典数据分页
// @Tags 字典数据
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param dictTypeId query int false "字典类型 ID"
// @Param dictCode query string false "字典编码"
// @Param dictLabel query string false "字典标签"
// @Param dictValue query string false "字典值"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data/page [get]
func (h *Handler) dataPage(c *gin.Context) {
	typeID, typeIDOK := optionalPositiveInt64(c, "dictTypeId")
	query := DataPageQuery{
		Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10), DictTypeID: typeID,
		DictCode: c.Query("dictCode"), DictLabel: c.Query("dictLabel"), DictValue: c.Query("dictValue"),
	}
	if !validPage(query.Page, query.PageSize) || !typeIDOK {
		badRequest(c, map[string]string{"page": "页码或每页条数不合法"})
		return
	}
	value, err := h.service.DataPage(c, query)
	writeResult(c, value, err)
}

// dataDetail godoc
// @Summary 字典数据详情
// @Tags 字典数据
// @Security BearerAuth
// @Param id path int true "字典数据 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data/{id} [get]
func (h *Handler) dataDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	value, err := h.service.DataDetail(c, id)
	writeResult(c, value, err)
}

// createData godoc
// @Summary 新增字典数据
// @Tags 字典数据
// @Security BearerAuth
// @Param request body dataRequest true "字典数据"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data [post]
func (h *Handler) createData(c *gin.Context) {
	request, ok := bindDataRequest(c)
	if !ok {
		return
	}
	_, err := h.service.CreateData(c, auditMetadata(c), request.input())
	writeMutation(c, err)
}

// updateData godoc
// @Summary 修改字典数据
// @Tags 字典数据
// @Security BearerAuth
// @Param id path int true "字典数据 ID"
// @Param request body dataRequest true "字典数据"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data/{id} [put]
func (h *Handler) updateData(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	request, ok := bindDataRequest(c)
	if !ok {
		return
	}
	_, err := h.service.UpdateData(c, auditMetadata(c), id, request.input())
	writeMutation(c, err)
}

// deleteData godoc
// @Summary 删除字典数据
// @Tags 字典数据
// @Security BearerAuth
// @Param id path int true "字典数据 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data/{id} [delete]
func (h *Handler) deleteData(c *gin.Context) {
	id, ok := pathID(c)
	if ok {
		writeMutation(c, h.service.DeleteData(c, auditMetadata(c), id))
	}
}

// deleteDataBatch godoc
// @Summary 批量删除字典数据
// @Tags 字典数据
// @Security BearerAuth
// @Param request body batchRequest true "字典数据 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict-data/batch-delete [post]
func (h *Handler) deleteDataBatch(c *gin.Context) {
	var request batchRequest
	if !bindJSON(c, &request) {
		return
	}
	if len(request.IDs) == 0 {
		badRequest(c, map[string]string{"ids": "ID 列表不能为空"})
		return
	}
	writeMutation(c, h.service.DeleteDataBatch(c, auditMetadata(c), request.IDs))
}

// items godoc
// @Summary 按编码查询启用字典项
// @Tags 字典项
// @Security BearerAuth
// @Param dictCode path string true "字典编码"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/dict/{dictCode}/items [get]
func (h *Handler) items(c *gin.Context) {
	value, err := h.service.Items(c, c.Param("dictCode"))
	writeResult(c, value, err)
}

func bindTypeRequest(c *gin.Context) (typeRequest, bool) {
	var request typeRequest
	if !bindJSON(c, &request) {
		return request, false
	}
	fields := map[string]string{}
	if blankOrTooLong(request.DictName, 100) {
		fields["dictName"] = "字典名称不能为空且长度不能超过 100"
	}
	if blankOrTooLong(request.DictCode, 100) {
		fields["dictCode"] = "字典编码不能为空且长度不能超过 100"
	}
	if request.Status != nil && !validStatus(*request.Status) {
		fields["status"] = "状态只能为 0 或 1"
	}
	if len(fields) > 0 {
		badRequest(c, fields)
		return request, false
	}
	return request, true
}

func bindDataRequest(c *gin.Context) (dataRequest, bool) {
	var request dataRequest
	if !bindJSON(c, &request) {
		return request, false
	}
	fields := map[string]string{}
	if request.DictTypeID <= 0 {
		fields["dictTypeId"] = "字典类型不能为空"
	}
	if blankOrTooLong(request.DictLabel, 100) {
		fields["dictLabel"] = "字典标签不能为空且长度不能超过 100"
	}
	if blankOrTooLong(request.DictValue, 100) {
		fields["dictValue"] = "字典值不能为空且长度不能超过 100"
	}
	if len(fields) > 0 {
		badRequest(c, fields)
		return request, false
	}
	return request, true
}

func (r typeRequest) input() TypeInput {
	return TypeInput(r)
}

func (r dataRequest) input() DataInput {
	return DataInput(r)
}

func blankOrTooLong(value string, max int) bool {
	return strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > max
}

func validPage(page, size int) bool { return page >= 1 && size >= 1 && size <= 500 }

func queryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return result
}

func optionalStatus(c *gin.Context, key string) (*int, bool) {
	value := c.Query(key)
	if value == "" {
		return nil, true
	}
	result, err := strconv.Atoi(value)
	if err != nil || !validStatus(result) {
		return nil, false
	}
	return &result, true
}

func optionalPositiveInt64(c *gin.Context, key string) (*int64, bool) {
	value := c.Query(key)
	if value == "" {
		return nil, true
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil || result <= 0 {
		return nil, false
	}
	return &result, true
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, map[string]string{"id": "ID 不合法"})
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		badRequest(c, map[string]string{"body": "参数错误"})
		return false
	}
	return true
}

func auditMetadata(c *gin.Context) AuditMetadata {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	requestMeta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return AuditMetadata{
		ActorID: principal.UserID, RequestID: requestMeta.RequestID, ClientIP: requestMeta.ClientIP,
		UserAgent: requestMeta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI(),
	}
}

func writeResult(c *gin.Context, value any, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, value)
}

func writeMutation(c *gin.Context, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	platformhttp.OK(c, nil)
}

func badRequest(c *gin.Context, fields map[string]string) {
	platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", fields)
}

func writeError(c *gin.Context, err error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrBuiltinProtected):
		message := "内置字典类型受保护"
		if strings.Contains(err.Error(), "修改编码") {
			message = "内置字典类型禁止修改编码"
		} else if strings.Contains(err.Error(), "禁止删除") {
			message = "内置字典类型禁止删除"
		}
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, message, nil)
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrDictCodeConflict),
		errors.Is(err, ErrDictValueConflict), errors.Is(err, ErrTypeHasData):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
