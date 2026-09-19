package filemgmt

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

// HandlerService is the file-management use-case boundary consumed by HTTP.
type HandlerService interface {
	UploadContent(context.Context, AuditMetadata, UploadInput, string, string) (File, error)
	UploadBatchContent(context.Context, AuditMetadata, []UploadInput, string, string) (BatchUploadResult, error)
	Page(context.Context, FilePageQuery) (Page[File], error)
	Detail(context.Context, int64) (*File, error)
	Update(context.Context, AuditMetadata, int64, UpdateInput) error
	Delete(context.Context, AuditMetadata, int64) error
	DeleteBatch(context.Context, AuditMetadata, []int64) error
	SetStatus(context.Context, AuditMetadata, int64, int) error
	Open(context.Context, int64, bool) (FileResource, error)
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
		return nil, errors.New("file handler service is required")
	}
	return &Handler{service: service}, nil
}

// RegisterRoutes registers Java-compatible file routes on an already
// authenticated /api/system router group.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	files := router.Group("/file")
	files.POST("/upload", handler.upload)
	files.POST("/upload-batch", handler.uploadBatch)
	files.GET("/page", handler.page)
	files.POST("/batch-delete", handler.deleteBatch)
	files.GET("/:id", handler.detail)
	files.PUT("/:id", handler.update)
	files.DELETE("/:id", handler.delete)
	files.PATCH("/:id/status", handler.status)
	files.GET("/:id/download", handler.download)
	files.GET("/:id/view", handler.view)
}

type updateRequest struct {
	BusinessModule string  `json:"businessModule"`
	Remark         *string `json:"remark"`
}

type batchRequest struct {
	IDs []int64 `json:"ids"`
}

type statusRequest struct {
	Status *int `json:"status"`
}

// upload godoc
// @Summary 上传文件
// @Description 原始请求体最多 55 MiB，单文件内容最多 50 MiB；businessModule 最多 50 个 Unicode 字符且最多 200 字节，remark 最多 500 个 Unicode 字符且最多 2000 字节，文件名最多 255 字节。声明为图片或使用 PNG、JPEG、GIF 扩展名的文件会校验真实内容，图片最多 25000000 像素。
// @Tags 文件管理
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "文件"
// @Param businessModule formData string false "业务模块"
// @Param remark formData string false "备注"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 413 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope
// @Router /api/system/file/upload [post]
func (h *Handler) upload(c *gin.Context) {
	parsed, err := parseMultipartUpload(c.Request, "file", 1, 0)
	if err != nil {
		writeUploadParseError(c, "file", err)
		return
	}
	defer parsed.cleanupFiles()
	if len(parsed.files) == 0 {
		writeFields(c, map[string]string{"file": "文件不能为空"})
		return
	}
	if parsed.files[0].parseErr != nil {
		if errors.Is(parsed.files[0].parseErr, ErrFileEmpty) {
			writeFields(c, map[string]string{"file": "文件不能为空"})
			return
		}
		writeError(c, parsed.files[0].parseErr)
		return
	}
	businessModule, remark := parsed.businessModule, parsed.remark
	if fields := validateMetadata(businessModule, &remark); len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	result, err := h.service.UploadContent(c.Request.Context(), auditMetadata(c), parsed.files[0].input, businessModule, remark)
	writeResult(c, result, err)
}

// uploadBatch godoc
// @Summary 批量上传文件
// @Description 原始请求体最多 210 MiB；最多 20 个文件，所有文件内容总量最多 200 MiB。每个文件独立返回成功或失败结果；失败项带原始文件索引，便于同名文件精确重试。
// @Tags 文件管理
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param files formData file true "文件列表"
// @Param businessModule formData string false "业务模块"
// @Param remark formData string false "备注"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 413 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope
// @Router /api/system/file/upload-batch [post]
func (h *Handler) uploadBatch(c *gin.Context) {
	parsed, err := parseMultipartUpload(c.Request, "files", MaxBatchFiles, MaxBatchContentSize)
	if err != nil {
		writeUploadParseError(c, "files", err)
		return
	}
	defer parsed.cleanupFiles()
	if len(parsed.files) == 0 {
		writeFields(c, map[string]string{"files": "文件不能为空"})
		return
	}
	businessModule, remark := parsed.businessModule, parsed.remark
	if fields := validateMetadata(businessModule, &remark); len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	inputs := make([]UploadInput, 0, len(parsed.files))
	originalIndexes := make([]int, 0, len(parsed.files))
	result := BatchUploadResult{Succeeded: []File{}, Failed: []BatchUploadFailure{}}
	for index, file := range parsed.files {
		if file.parseErr != nil {
			result.Failed = append(result.Failed, BatchUploadFailure{
				FileName: batchFailureName(file.input.Filename), Message: file.parseErr.Error(), Index: index,
			})
			continue
		}
		inputs = append(inputs, file.input)
		originalIndexes = append(originalIndexes, index)
	}
	if len(inputs) > 0 {
		serviceResult, serviceErr := h.service.UploadBatchContent(
			c.Request.Context(), auditMetadata(c), inputs, businessModule, remark,
		)
		if serviceErr != nil {
			writeError(c, serviceErr)
			return
		}
		result.Succeeded = append(result.Succeeded, serviceResult.Succeeded...)
		for _, failure := range serviceResult.Failed {
			if failure.Index >= 0 && failure.Index < len(originalIndexes) {
				failure.Index = originalIndexes[failure.Index]
			}
			result.Failed = append(result.Failed, failure)
		}
	}
	sort.SliceStable(result.Failed, func(i, j int) bool { return result.Failed[i].Index < result.Failed[j].Index })
	writeResult(c, result, err)
}

// page godoc
// @Summary 文件分页
// @Tags 文件管理
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param originalName query string false "原始文件名"
// @Param businessModule query string false "业务模块"
// @Param mimeType query string false "MIME 类型"
// @Param status query int false "状态：0 或 1"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/page [get]
func (h *Handler) page(c *gin.Context) {
	page, pageOK := queryInt(c, "page", 1)
	pageSize, pageSizeOK := queryInt(c, "pageSize", 10)
	status, statusOK := optionalStatus(c, "status")
	if !pageOK || !pageSizeOK || page < 1 || pageSize < 1 || pageSize > 500 || !statusOK {
		fields := map[string]string{}
		if !pageOK || page < 1 {
			fields["page"] = "页码必须大于 0"
		}
		if !pageSizeOK || pageSize < 1 || pageSize > 500 {
			fields["pageSize"] = "每页条数必须在 1 到 500 之间"
		}
		if !statusOK {
			fields["status"] = "状态只能为 0 或 1"
		}
		writeFields(c, fields)
		return
	}
	result, err := h.service.Page(c.Request.Context(), FilePageQuery{
		Page: page, PageSize: pageSize, OriginalName: c.Query("originalName"),
		BusinessModule: c.Query("businessModule"), MimeType: c.Query("mimeType"), Status: status,
	})
	writeResult(c, result, err)
}

// detail godoc
// @Summary 文件详情
// @Tags 文件管理
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.Detail(c.Request.Context(), id)
	writeResult(c, result, err)
}

// update godoc
// @Summary 修改文件元信息
// @Tags 文件管理
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Param request body updateRequest true "文件元信息"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request updateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeFields(c, map[string]string{"body": "参数错误"})
		return
	}
	if fields := validateMetadata(request.BusinessModule, request.Remark); len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	writeMutation(c, h.service.Update(c.Request.Context(), auditMetadata(c), id, UpdateInput(request)))
}

// delete godoc
// @Summary 删除文件
// @Tags 文件管理
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.Delete(c.Request.Context(), auditMetadata(c), id))
}

// deleteBatch godoc
// @Summary 批量删除文件
// @Tags 文件管理
// @Security BearerAuth
// @Param request body batchRequest true "文件 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/batch-delete [post]
func (h *Handler) deleteBatch(c *gin.Context) {
	var request batchRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validIDs(request.IDs) {
		writeFields(c, map[string]string{"ids": "ID 列表不能为空且必须为正整数"})
		return
	}
	writeMutation(c, h.service.DeleteBatch(c.Request.Context(), auditMetadata(c), request.IDs))
}

// status godoc
// @Summary 修改文件状态
// @Tags 文件管理
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id}/status [patch]
func (h *Handler) status(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request statusRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Status == nil || (*request.Status != 0 && *request.Status != 1) {
		writeFields(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	writeMutation(c, h.service.SetStatus(c.Request.Context(), auditMetadata(c), id, *request.Status))
}

// download godoc
// @Summary 下载文件
// @Tags 文件管理
// @Security BearerAuth
// @Produce application/octet-stream
// @Param id path int true "文件 ID"
// @Success 200 {file} binary
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id}/download [get]
func (h *Handler) download(c *gin.Context) { h.stream(c, false) }

// view godoc
// @Summary 预览文件
// @Tags 文件管理
// @Security BearerAuth
// @Produce application/octet-stream
// @Param id path int true "文件 ID"
// @Success 200 {file} binary
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/file/{id}/view [get]
func (h *Handler) view(c *gin.Context) { h.stream(c, true) }

func (h *Handler) stream(c *gin.Context, view bool) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	resource, err := h.service.Open(c.Request.Context(), id, view)
	if err != nil {
		writeError(c, err)
		return
	}
	defer func() { _ = resource.Reader.Close() }()

	contentType := resource.File.MimeType
	if parsed, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = parsed
	} else {
		contentType = "application/octet-stream"
	}
	disposition := "attachment"
	if view {
		disposition = "inline"
	}
	filename := strings.ReplaceAll(url.QueryEscape(resource.File.OriginalName), "+", "%20")
	c.DataFromReader(http.StatusOK, resource.File.FileSize, contentType, resource.Reader, map[string]string{
		"Content-Disposition": disposition + "; filename*=UTF-8''" + filename,
	})
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeFields(c, map[string]string{"id": "ID 不合法"})
		return 0, false
	}
	return id, true
}

func queryInt(c *gin.Context, key string, fallback int) (int, bool) {
	value := c.Query(key)
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func optionalStatus(c *gin.Context, key string) (*int, bool) {
	value := c.Query(key)
	if value == "" {
		return nil, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || (parsed != 0 && parsed != 1) {
		return nil, false
	}
	return &parsed, true
}

func validateMetadata(businessModule string, remark *string) map[string]string {
	fields := map[string]string{}
	if utf8.RuneCountInString(businessModule) > 50 {
		fields["businessModule"] = "业务模块长度不能超过 50"
	}
	if remark != nil && utf8.RuneCountInString(*remark) > 500 {
		fields["remark"] = "备注长度不能超过 500"
	}
	return fields
}

func writeUploadParseError(c *gin.Context, field string, err error) {
	if errors.Is(err, ErrFileEmpty) {
		writeFields(c, map[string]string{field: "文件不能为空"})
		return
	}
	writeError(c, err)
}

func batchFailureName(filename string) string {
	if filename == "" {
		return "unknown"
	}
	return filename
}

func validIDs(ids []int64) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func auditMetadata(c *gin.Context) AuditMetadata {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	requestMeta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return AuditMetadata{
		ActorID: principal.UserID, RequestID: requestMeta.RequestID,
		ClientIP: requestMeta.ClientIP, UserAgent: requestMeta.UserAgent,
		RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI(),
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

func writeFields(c *gin.Context, fields map[string]string) {
	platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", fields)
}

func writeError(c *gin.Context, err error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case platformhttp.IsRequestBodyTooLarge(err):
		platformhttp.RecordMultipartRejection(c, nil, "body_too_large")
		platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, "请求体超过大小限制", nil)
	case errors.Is(err, ErrBatchTooMany):
		platformhttp.RecordMultipartRejection(c, nil, "file_count_limit")
		platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, ErrBatchTooMany.Error(), nil)
	case errors.Is(err, ErrBatchTooLarge):
		platformhttp.RecordMultipartRejection(c, nil, "batch_size_limit")
		platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, ErrBatchTooLarge.Error(), nil)
	case errors.Is(err, ErrMultipartFieldLarge), errors.Is(err, ErrFilenameTooLong):
		platformhttp.RecordMultipartRejection(c, nil, "part_size_limit")
		platformhttp.WriteError(c, http.StatusRequestEntityTooLarge, platformhttp.CodeRequestEntityTooLarge, err.Error(), nil)
	case errors.Is(err, ErrMultipartMalformed):
		platformhttp.RecordMultipartRejection(c, nil, "malformed")
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, ErrMultipartMalformed.Error(), nil)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrFileEmpty), errors.Is(err, ErrFileTooLarge), errors.Is(err, ErrInvalidImage):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
