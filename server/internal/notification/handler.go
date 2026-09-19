package notification

import (
	"errors"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type Handler struct{ service *Service }

// ApiEnvelope is the common response shape used by the existing API.
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(service *Service) (*Handler, error) {
	if service == nil {
		return nil, errors.New("notification service is required")
	}
	return &Handler{service}, nil
}
func RegisterRoutes(router gin.IRouter, h *Handler) {
	n := router.Group("/notification")
	n.GET("/page", h.page)
	n.GET("/unread-count", h.unread)
	n.GET("/:id", h.detail)
	n.PUT("/:id/read", h.read)
	n.PUT("/read-all", h.readAll)
	n.POST("", h.publish)
	router.GET("/notification-admin/page", h.adminPage)
}

type publishRequest struct {
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	UserIDs  []int64 `json:"userIds"`
	AllUsers bool    `json:"allUsers"`
}

func principalID(c *gin.Context) (int64, bool) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok || p.UserID <= 0 {
		platformhttp.WriteError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, "未登录", nil)
		return 0, false
	}
	return p.UserID, true
}

// page godoc
// @Summary 我的通知分页
// @Tags 站内通知
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification/page [get]
func (h *Handler) page(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	p, z := pageParams(c)
	v, e := h.service.Page(c.Request.Context(), id, PageQuery{Page: p, PageSize: z})
	write(c, v, e)
}

// adminPage godoc
// @Summary 通知管理分页
// @Tags 站内通知
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification-admin/page [get]
func (h *Handler) adminPage(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	p, z := pageParams(c)
	v, e := h.service.AdminPage(c.Request.Context(), id, PageQuery{Page: p, PageSize: z})
	write(c, v, e)
}

// unread godoc
// @Summary 未读通知数
// @Tags 站内通知
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification/unread-count [get]
func (h *Handler) unread(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	v, e := h.service.UnreadCount(c.Request.Context(), id)
	write(c, v, e)
}

// detail godoc
// @Summary 通知详情
// @Tags 站内通知
// @Security BearerAuth
// @Param id path int true "通知 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	nid, ok := pathID(c)
	if !ok {
		return
	}
	v, e := h.service.Find(c.Request.Context(), id, nid)
	write(c, v, e)
}

// read godoc
// @Summary 标记通知已读
// @Tags 站内通知
// @Security BearerAuth
// @Param id path int true "通知 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification/{id}/read [put]
func (h *Handler) read(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	nid, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.MarkRead(c.Request.Context(), id, nid))
}

// readAll godoc
// @Summary 全部标记已读
// @Tags 站内通知
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification/read-all [put]
func (h *Handler) readAll(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.MarkAllRead(c.Request.Context(), id))
}

// publish godoc
// @Summary 发布站内通知
// @Tags 站内通知
// @Security BearerAuth
// @Param body body publishRequest true "通知内容"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/notification [post]
func (h *Handler) publish(c *gin.Context) {
	id, ok := principalID(c)
	if !ok {
		return
	}
	var in publishRequest
	if c.ShouldBindJSON(&in) != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	writeMutation(c, h.service.Publish(c.Request.Context(), id, PublishInput{Title: in.Title, Content: in.Content, UserIDs: in.UserIDs, AllUsers: in.AllUsers}))
}
func pageParams(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	z, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	return normalizePage(p, z)
}
func pathID(c *gin.Context) (int64, bool) {
	v, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || v <= 0 {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return 0, false
	}
	return v, true
}
func write(c *gin.Context, v any, e error) {
	if e != nil {
		writeError(c, e)
		return
	}
	platformhttp.OK(c, v)
}
func writeMutation(c *gin.Context, e error) {
	if e != nil {
		writeError(c, e)
		return
	}
	platformhttp.OK(c, nil)
}
func writeError(c *gin.Context, e error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(e):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(e, ErrForbidden):
		platformhttp.WriteError(c, http.StatusForbidden, platformhttp.CodeForbidden, e.Error(), nil)
	case errors.Is(e, ErrInvalid):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, e.Error(), nil)
	case errors.Is(e, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, e.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
