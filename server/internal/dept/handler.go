package dept

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platform "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

type HandlerService interface {
	Tree(context.Context) ([]Dept, error)
	Options(context.Context) ([]Dept, error)
	Page(context.Context, Query) (Page, error)
	Detail(context.Context, int64) (*Dept, error)
	Create(context.Context, AuditMetadata, Input) (Dept, error)
	Update(context.Context, AuditMetadata, int64, Input) (Dept, error)
	Delete(context.Context, AuditMetadata, int64) error
	DeleteBatch(context.Context, AuditMetadata, []int64) error
	SetStatus(context.Context, AuditMetadata, int64, int) error
}
type Handler struct{ s HandlerService }

// ApiEnvelope is used by Swaggo to describe the established response shape.
//
//nolint:unused // referenced by Swaggo annotations below
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s HandlerService) (*Handler, error) {
	if s == nil {
		return nil, errors.New("dept handler service is required")
	}
	return &Handler{s}, nil
}
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/dept")
	g.GET("/tree", h.tree)
	g.GET("/options", h.options)
	g.GET("/page", h.page)
	g.GET("/:id", h.detail)
	g.POST("", h.create)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/batch-delete", h.batch)
	g.PATCH("/:id/status", h.status)
}

type request struct {
	ParentID  *int64  `json:"parentId"`
	DeptName  string  `json:"deptName"`
	DeptCode  string  `json:"deptCode"`
	Leader    *string `json:"leader"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	Remark    *string `json:"remark"`
	SortOrder *int    `json:"sortOrder"`
	Status    *int    `json:"status"`
}
type ids struct {
	IDs []int64 `json:"ids"`
}
type statusReq struct {
	Status int `json:"status"`
}

// tree godoc
// @Summary 部门树
// @Tags 部门管理
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Router /api/system/dept/tree [get]
func (h *Handler) tree(c *gin.Context) { v, e := h.s.Tree(c); out(c, v, e) }

// options godoc
// @Summary 启用部门树
// @Tags 部门管理
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Router /api/system/dept/options [get]
func (h *Handler) options(c *gin.Context) { v, e := h.s.Options(c); out(c, v, e) }

// page godoc
// @Summary 部门分页
// @Tags 部门管理
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param deptName query string false "部门名称"
// @Param deptCode query string false "部门编码"
// @Param status query int false "状态：0 或 1"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept/page [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: qi(c, "page", 1), PageSize: qi(c, "pageSize", 10), DeptName: c.Query("deptName"), DeptCode: c.Query("deptCode")}
	if x, ok := qs(c, "status"); ok {
		q.Status = x
	} else {
		bad(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 500 {
		bad(c, map[string]string{"page": "页码或每页条数不合法"})
		return
	}
	v, e := h.s.Page(c, q)
	out(c, v, e)
}

// detail godoc
// @Summary 部门详情
// @Tags 部门管理
// @Security BearerAuth
// @Param id path int true "部门 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, ok := id(c)
	if !ok {
		return
	}
	v, e := h.s.Detail(c, id)
	out(c, v, e)
}

// create godoc
// @Summary 新增部门
// @Tags 部门管理
// @Security BearerAuth
// @Param request body request true "部门"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept [post]
func (h *Handler) create(c *gin.Context) {
	var r request
	if c.ShouldBindJSON(&r) != nil {
		bad(c, map[string]string{"body": "参数错误"})
		return
	}
	in, fields := r.input()
	if len(fields) > 0 {
		bad(c, fields)
		return
	}
	v, e := h.s.Create(c, meta(c), in)
	if e == nil {
		platform.OK(c, nil)
	} else {
		_ = v
		err(c, e)
	}
}

// update godoc
// @Summary 修改部门
// @Tags 部门管理
// @Security BearerAuth
// @Param id path int true "部门 ID"
// @Param request body request true "部门"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := id(c)
	if !ok {
		return
	}
	var r request
	if c.ShouldBindJSON(&r) != nil {
		bad(c, map[string]string{"body": "参数错误"})
		return
	}
	in, fields := r.input()
	if len(fields) > 0 {
		bad(c, fields)
		return
	}
	_, e := h.s.Update(c, meta(c), id, in)
	if e == nil {
		platform.OK(c, nil)
	} else {
		err(c, e)
	}
}

// delete godoc
// @Summary 删除部门
// @Tags 部门管理
// @Security BearerAuth
// @Param id path int true "部门 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/dept/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := id(c)
	if ok {
		err(c, h.s.Delete(c, meta(c), id))
	}
}

// batch godoc
// @Summary 批量删除部门
// @Tags 部门管理
// @Security BearerAuth
// @Param request body ids true "部门 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept/batch-delete [post]
func (h *Handler) batch(c *gin.Context) {
	var r ids
	if c.ShouldBindJSON(&r) != nil || len(r.IDs) == 0 {
		bad(c, map[string]string{"ids": "ID 列表不能为空"})
		return
	}
	err(c, h.s.DeleteBatch(c, meta(c), r.IDs))
}

// status godoc
// @Summary 修改部门状态
// @Tags 部门管理
// @Security BearerAuth
// @Param id path int true "部门 ID"
// @Param request body statusReq true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Router /api/system/dept/{id}/status [patch]
func (h *Handler) status(c *gin.Context) {
	id, ok := id(c)
	if !ok {
		return
	}
	var r statusReq
	if c.ShouldBindJSON(&r) != nil || r.Status < 0 || r.Status > 1 {
		bad(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	err(c, h.s.SetStatus(c, meta(c), id, r.Status))
}
func (r request) input() (Input, map[string]string) {
	fields := map[string]string{}
	if r.ParentID == nil || *r.ParentID < 0 {
		fields["parentId"] = "父级部门不能为空"
	}
	if strings.TrimSpace(r.DeptName) == "" || utf8.RuneCountInString(r.DeptName) > 100 {
		fields["deptName"] = "部门名称不能为空且长度不能超过 100"
	}
	if strings.TrimSpace(r.DeptCode) == "" || utf8.RuneCountInString(r.DeptCode) > 50 {
		fields["deptCode"] = "部门编码不能为空且长度不能超过 50"
	}
	if r.Email != nil && strings.TrimSpace(*r.Email) != "" {
		address, err := mail.ParseAddress(strings.TrimSpace(*r.Email))
		if err != nil || address.Address != strings.TrimSpace(*r.Email) {
			fields["email"] = "邮箱格式不正确"
		}
	}
	return Input{ParentID: iv64(r.ParentID), DeptName: r.DeptName, DeptCode: r.DeptCode, Leader: r.Leader, Phone: r.Phone, Email: r.Email, Remark: r.Remark, SortOrder: iv(r.SortOrder, 0), Status: iv(r.Status, 1)}, fields
}
func iv64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
func iv(v *int, d int) int {
	if v == nil {
		return d
	}
	return *v
}
func qi(c *gin.Context, k string, d int) int {
	if x := c.Query(k); x != "" {
		n, e := strconv.Atoi(x)
		if e == nil {
			return n
		}
		return 0
	}
	return d
}
func qs(c *gin.Context, k string) (*int, bool) {
	x := c.Query(k)
	if x == "" {
		return nil, true
	}
	n, e := strconv.Atoi(x)
	if e != nil || n < 0 || n > 1 {
		return nil, false
	}
	return &n, true
}
func id(c *gin.Context) (int64, bool) {
	n, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || n < 1 {
		bad(c, map[string]string{"id": "ID 不合法"})
		return 0, false
	}
	return n, true
}
func meta(c *gin.Context) AuditMetadata {
	p, _ := auth.PrincipalFromContext(c.Request.Context())
	m, _ := platform.RequestMetaFromContext(c.Request.Context())
	return AuditMetadata{ActorID: p.UserID, RequestID: m.RequestID, ClientIP: m.ClientIP, UserAgent: m.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()}
}
func out(c *gin.Context, v any, e error) {
	if e != nil {
		err(c, e)
	} else {
		platform.OK(c, v)
	}
}
func bad(c *gin.Context, fields map[string]string) {
	platform.WriteError(c, http.StatusBadRequest, 400, "参数错误", fields)
}
func err(c *gin.Context, e error) {
	if e == nil {
		platform.OK(c, nil)
		return
	}
	if platform.IsTemporaryUnavailable(e) {
		platform.TemporaryUnavailable(c)
	} else if errors.Is(e, ErrNotFound) {
		platform.WriteError(c, 200, 404, e.Error(), nil)
	} else if errors.Is(e, ErrInvalid) || errors.Is(e, ErrConflict) || errors.Is(e, ErrBuiltin) || errors.Is(e, ErrDeleteBuiltin) || errors.Is(e, ErrHasChildren) || errors.Is(e, ErrHasUsers) {
		platform.WriteError(c, 200, 400, e.Error(), nil)
	} else {
		platform.WriteError(c, 500, 500, "系统错误", nil)
	}
}
