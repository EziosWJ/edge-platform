package hmi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/command"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type HandlerService interface {
	Page(context.Context, PageQuery) (PageList, error)
	Detail(context.Context, string) (Page, error)
	Create(context.Context, audit.Metadata, CreateInput) (Page, error)
	UpdateMetadata(context.Context, audit.Metadata, string, MetadataInput) (Page, error)
	SaveDraft(context.Context, audit.Metadata, string, DraftInput) (Page, error)
	Publish(context.Context, audit.Metadata, string, PublishInput) (Version, error)
	Runtime(context.Context, string) (RuntimeBootstrap, error)
}
type PermissionChecker interface {
	HasPermission(context.Context, int64, string) (bool, error)
}
type Handler struct {
	service    HandlerService
	authorizer PermissionChecker
}

func NewHandler(service HandlerService, authorizer PermissionChecker) (*Handler, error) {
	if service == nil || authorizer == nil {
		return nil, ErrInvalid
	}
	return &Handler{service: service, authorizer: authorizer}, nil
}
func RegisterRoutes(router gin.IRouter, h *Handler) {
	router.GET("/page", h.page)
	router.GET("/:pageId", h.detail)
	router.POST("", h.create)
	router.PUT("/:pageId", h.update)
	router.PUT("/:pageId/draft", h.saveDraft)
	router.POST("/:pageId/publish", h.publish)
	router.GET("/:pageId/runtime", h.runtime)
}

type apiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}
type createRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	DraftDocument json.RawMessage `json:"draftDocument"`
}
type createRequestDoc struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	DraftDocument map[string]any `json:"draftDocument,omitempty"`
}
type metadataRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type draftRequest struct {
	ExpectedDraftRevision int64           `json:"expectedDraftRevision"`
	Document              json.RawMessage `json:"document"`
}
type draftRequestDoc struct {
	ExpectedDraftRevision int64          `json:"expectedDraftRevision"`
	Document              map[string]any `json:"document"`
}
type publishRequest struct {
	ExpectedDraftRevision int64 `json:"expectedDraftRevision"`
}
type publishResponse struct {
	PageID              string       `json:"pageId"`
	PublishedVersionID  string       `json:"publishedVersionId"`
	VersionID           string       `json:"versionId"`
	VersionNo           int          `json:"versionNo"`
	SourceDraftRevision int64        `json:"sourceDraftRevision"`
	Document            JSONDocument `json:"document"`
	PublishedBy         int64        `json:"publishedBy"`
	PublishedAt         time.Time    `json:"publishedAt"`
}

// @Summary HMI 页面分页列表
// @Tags HMI
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数，1-500"
// @Param name query string false "页面名称包含搜索"
// @Success 200 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Router /api/hmi/page/page [get]
func (h *Handler) page(c *gin.Context) {
	p, ok := h.authorize(c, ListPermission)
	if !ok {
		return
	}
	_ = p
	page, valid := queryInt(c, "page", 1)
	size, validSize := queryInt(c, "pageSize", 20)
	if !valid || !validSize {
		write(c, http.StatusBadRequest, ErrInvalid)
		return
	}
	name := c.Query("name")
	out, err := h.service.Page(c.Request.Context(), PageQuery{Page: page, PageSize: size, Name: name})
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, out)
}

// @Summary HMI 页面详情（含 Draft）
// @Tags HMI
// @Security BearerAuth
// @Param pageId path string true "页面 UUID"
// @Success 200 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Failure 404 {object} apiEnvelope
// @Router /api/hmi/page/{pageId} [get]
func (h *Handler) detail(c *gin.Context) {
	if _, ok := h.authorize(c, DetailPermission); !ok {
		return
	}
	out, err := h.service.Detail(c.Request.Context(), c.Param("pageId"))
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, out)
}

// @Summary 创建 HMI 页面
// @Tags HMI
// @Security BearerAuth
// @Param body body createRequestDoc true "页面 metadata 与初始 Draft"
// @Success 200 {object} apiEnvelope
// @Failure 400 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Router /api/hmi/page [post]
func (h *Handler) create(c *gin.Context) {
	principal, ok := h.authorize(c, EditPermission)
	if !ok {
		return
	}
	var req createRequest
	if decodeRequest(c, &req) != nil {
		write(c, http.StatusBadRequest, ErrInvalid)
		return
	}
	doc := JSONDocument(req.DraftDocument)
	out, err := h.service.Create(c.Request.Context(), auditMetadata(c, principal), CreateInput{Name: req.Name, Description: req.Description, Document: doc})
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, out)
}

// @Summary 更新 HMI 页面 metadata
// @Tags HMI
// @Security BearerAuth
// @Param pageId path string true "页面 UUID"
// @Param body body metadataRequest true "页面 metadata"
// @Success 200 {object} apiEnvelope
// @Failure 400 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Failure 404 {object} apiEnvelope
// @Router /api/hmi/page/{pageId} [put]
func (h *Handler) update(c *gin.Context) {
	principal, ok := h.authorize(c, EditPermission)
	if !ok {
		return
	}
	var req metadataRequest
	if decodeRequest(c, &req) != nil {
		write(c, http.StatusBadRequest, ErrInvalid)
		return
	}
	out, err := h.service.UpdateMetadata(c.Request.Context(), auditMetadata(c, principal), c.Param("pageId"), MetadataInput{Name: req.Name, Description: req.Description})
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, out)
}

// @Summary 保存 HMI Draft（乐观锁）
// @Tags HMI
// @Security BearerAuth
// @Param pageId path string true "页面 UUID"
// @Param body body draftRequestDoc true "Draft 与 expectedDraftRevision"
// @Success 200 {object} apiEnvelope
// @Failure 400 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Failure 404 {object} apiEnvelope
// @Failure 409 {object} apiEnvelope
// @Router /api/hmi/page/{pageId}/draft [put]
func (h *Handler) saveDraft(c *gin.Context) {
	principal, ok := h.authorize(c, EditPermission)
	if !ok {
		return
	}
	var req draftRequest
	if decodeRequest(c, &req) != nil {
		write(c, http.StatusBadRequest, ErrInvalid)
		return
	}
	out, err := h.service.SaveDraft(c.Request.Context(), auditMetadata(c, principal), c.Param("pageId"), DraftInput{ExpectedDraftRevision: req.ExpectedDraftRevision, Document: JSONDocument(req.Document)})
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, out)
}

// @Summary 发布已保存的 HMI Draft
// @Tags HMI
// @Security BearerAuth
// @Param pageId path string true "页面 UUID"
// @Param body body publishRequest true "expectedDraftRevision"
// @Success 200 {object} apiEnvelope
// @Failure 400 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Failure 404 {object} apiEnvelope
// @Failure 409 {object} apiEnvelope
// @Router /api/hmi/page/{pageId}/publish [post]
func (h *Handler) publish(c *gin.Context) {
	principal, ok := h.authorize(c, PublishPermission)
	if !ok {
		return
	}
	var req publishRequest
	if decodeRequest(c, &req) != nil {
		write(c, http.StatusBadRequest, ErrInvalid)
		return
	}
	out, err := h.service.Publish(c.Request.Context(), auditMetadata(c, principal), c.Param("pageId"), PublishInput{ExpectedDraftRevision: req.ExpectedDraftRevision, ActorID: principal.UserID})
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	platformhttp.OK(c, publishResponse{PageID: out.PageID, PublishedVersionID: out.VersionID, VersionID: out.VersionID, VersionNo: out.VersionNo, SourceDraftRevision: out.SourceDraftRevision, Document: out.Document, PublishedBy: out.PublishedBy, PublishedAt: out.PublishedAt})
}

// @Summary 获取 HMI Published Runtime bootstrap
// @Tags HMI
// @Security BearerAuth
// @Param pageId path string true "页面 UUID"
// @Success 200 {object} apiEnvelope
// @Failure 401 {object} apiEnvelope
// @Failure 403 {object} apiEnvelope
// @Failure 404 {object} apiEnvelope
// @Router /api/hmi/page/{pageId}/runtime [get]
func (h *Handler) runtime(c *gin.Context) {
	principal, ok := h.authorize(c, RunPermission)
	if !ok {
		return
	}
	out, err := h.service.Runtime(c.Request.Context(), c.Param("pageId"))
	if err != nil {
		write(c, statusFor(err), err)
		return
	}
	allowed, err := h.authorizer.HasPermission(c.Request.Context(), principal.UserID, command.ExecutePermission)
	if err != nil {
		if platformhttp.IsTemporaryUnavailable(err) {
			platformhttp.TemporaryUnavailable(c)
		} else {
			platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
		}
		return
	}
	out.CanExecuteCommands = allowed
	platformhttp.OK(c, out)
}

func (h *Handler) authorize(c *gin.Context, permission string) (auth.Principal, bool) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok || p.UserID <= 0 {
		platformhttp.AbortError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, auth.ErrUnauthenticated.Error(), nil)
		return auth.Principal{}, false
	}
	allowed, err := h.authorizer.HasPermission(c.Request.Context(), p.UserID, permission)
	if err != nil {
		if platformhttp.IsTemporaryUnavailable(err) {
			platformhttp.TemporaryUnavailable(c)
		} else {
			platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
		}
		return p, false
	}
	if !allowed {
		platformhttp.WriteError(c, http.StatusForbidden, platformhttp.CodeForbidden, ErrForbidden.Error(), nil)
		return p, false
	}
	return p, true
}
func decodeRequest(c *gin.Context, out any) error {
	d := json.NewDecoder(io.LimitReader(c.Request.Body, MaxDocumentBytes+8192))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalid
	}
	return nil
}
func queryInt(c *gin.Context, key string, def int) (int, bool) {
	v, ok := c.GetQuery(key)
	if !ok {
		return def, true
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
func auditMetadata(c *gin.Context, p auth.Principal) audit.Metadata {
	return audit.Metadata{ActorID: p.UserID, RequestID: platformhttp.RequestIDFromContext(c.Request.Context()), ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestMethod: c.Request.Method, RequestURL: c.Request.URL.String()}
}
func statusFor(err error) int {
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNotPublished):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrInvalidBinding):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
func write(c *gin.Context, status int, err error) {
	if platformhttp.IsTemporaryUnavailable(err) {
		platformhttp.TemporaryUnavailable(c)
		return
	}
	code := status
	if status == http.StatusInternalServerError {
		code = platformhttp.CodeInternalError
	}
	msg := err.Error()
	if status == http.StatusInternalServerError {
		msg = "系统错误"
	}
	platformhttp.WriteError(c, status, code, msg, nil)
}
