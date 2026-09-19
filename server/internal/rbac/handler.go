package rbac

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

type HandlerService interface {
	RolePage(context.Context, RolePageQuery) (Page[Role], error)
	RoleDetail(context.Context, int64) (*RoleDetail, error)
	RoleOptions(context.Context) ([]Role, error)
	CreateRole(context.Context, AuditMetadata, RoleInput) (Role, error)
	UpdateRole(context.Context, AuditMetadata, int64, RoleInput) (Role, error)
	DeleteRole(context.Context, AuditMetadata, int64) error
	DeleteRoles(context.Context, AuditMetadata, []int64) error
	SetRoleStatus(context.Context, AuditMetadata, int64, int) error
	AssignRoleMenus(context.Context, AuditMetadata, int64, []int64) error
	MenuTree(context.Context) ([]Menu, error)
	MenuPage(context.Context, MenuPageQuery) (Page[Menu], error)
	MenuDetail(context.Context, int64) (*Menu, error)
	CreateMenu(context.Context, AuditMetadata, MenuInput) (Menu, error)
	UpdateMenu(context.Context, AuditMetadata, int64, MenuInput) (Menu, error)
	DeleteMenu(context.Context, AuditMetadata, int64) error
	DeleteMenus(context.Context, AuditMetadata, []int64) error
	SetMenuStatus(context.Context, AuditMetadata, int64, int) error
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
		return nil, errors.New("rbac handler service is required")
	}
	return &Handler{service}, nil
}

// RegisterRoutes registers routes on an already authenticated router group.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	roles := router.Group("/role")
	roles.GET("/page", handler.rolePage)
	roles.GET("/options", handler.roleOptions)
	roles.GET("/:id", handler.roleDetail)
	roles.POST("", handler.createRole)
	roles.PUT("/:id", handler.updateRole)
	roles.DELETE("/:id", handler.deleteRole)
	roles.POST("/batch-delete", handler.deleteRoles)
	roles.PATCH("/:id/status", handler.roleStatus)
	roles.PUT("/:id/menus", handler.roleMenus)
	menus := router.Group("/menu")
	menus.GET("/tree", handler.menuTree)
	menus.GET("/page", handler.menuPage)
	menus.GET("/:id", handler.menuDetail)
	menus.POST("", handler.createMenu)
	menus.PUT("/:id", handler.updateMenu)
	menus.DELETE("/:id", handler.deleteMenu)
	menus.POST("/batch-delete", handler.deleteMenus)
	menus.PATCH("/:id/status", handler.menuStatus)
}

type roleRequest struct {
	RoleName  string  `json:"roleName"`
	RoleCode  string  `json:"roleCode"`
	Status    *int    `json:"status"`
	SortOrder *int    `json:"sortOrder"`
	Remark    *string `json:"remark"`
}
type menuRequest struct {
	ParentID       int64   `json:"parentId"`
	MenuName       string  `json:"menuName"`
	MenuType       string  `json:"menuType"`
	Path           *string `json:"path"`
	Component      *string `json:"component"`
	ExternalURL    *string `json:"externalUrl"`
	Icon           *string `json:"icon"`
	PermissionCode *string `json:"permissionCode"`
	SortOrder      *int    `json:"sortOrder"`
	Visible        *int    `json:"visible"`
	Status         *int    `json:"status"`
	Remark         *string `json:"remark"`
}
type batchRequest struct {
	IDs []int64 `json:"ids"`
}
type statusRequest struct {
	Status int `json:"status"`
}
type menusRequest struct {
	MenuIDs []int64 `json:"menuIds"`
}

// rolePage godoc
// @Summary 角色分页
// @Tags 角色管理
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param roleName query string false "角色名称"
// @Param roleCode query string false "角色编码"
// @Param status query int false "状态：0 或 1"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/page [get]
func (h *Handler) rolePage(c *gin.Context) {
	status, validStatus := queryOptionalStatus(c, "status")
	q := RolePageQuery{Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10), RoleName: c.Query("roleName"), RoleCode: c.Query("roleCode"), Status: status}
	if !validPage(q.Page, q.PageSize) || !validStatus {
		writeFields(c, map[string]string{"page": "页码或每页条数不合法"})
		return
	}
	v, e := h.service.RolePage(c, q)
	writeResult(c, v, e)
}

// roleOptions godoc
// @Summary 启用角色选项
// @Tags 角色管理
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/options [get]
func (h *Handler) roleOptions(c *gin.Context) { v, e := h.service.RoleOptions(c); writeResult(c, v, e) }

// roleDetail godoc
// @Summary 角色详情
// @Tags 角色管理
// @Security BearerAuth
// @Param id path int true "角色 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/{id} [get]
func (h *Handler) roleDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	v, e := h.service.RoleDetail(c, id)
	writeResult(c, v, e)
}

// createRole godoc
// @Summary 新增角色
// @Tags 角色管理
// @Security BearerAuth
// @Param request body roleRequest true "角色"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role [post]
func (h *Handler) createRole(c *gin.Context) {
	var r roleRequest
	if !bindJSON(c, &r) {
		return
	}
	in, fields := r.roleInput()
	if len(fields) > 0 {
		writeFields(c, fields)
		return
	}
	_, e := h.service.CreateRole(c, auditMetadata(c), in)
	writeMutation(c, e)
}

// updateRole godoc
// @Summary 修改角色
// @Tags 角色管理
// @Security BearerAuth
// @Param id path int true "角色 ID"
// @Param request body roleRequest true "角色"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/{id} [put]
func (h *Handler) updateRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var r roleRequest
	if !bindJSON(c, &r) {
		return
	}
	in, fields := r.roleInput()
	if len(fields) > 0 {
		writeFields(c, fields)
		return
	}
	_, e := h.service.UpdateRole(c, auditMetadata(c), id, in)
	writeMutation(c, e)
}

// deleteRole godoc
// @Summary 删除角色
// @Tags 角色管理
// @Security BearerAuth
// @Param id path int true "角色 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/{id} [delete]
func (h *Handler) deleteRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.DeleteRole(c, auditMetadata(c), id))
}

// deleteRoles godoc
// @Summary 批量删除角色
// @Tags 角色管理
// @Security BearerAuth
// @Param request body batchRequest true "角色 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/batch-delete [post]
func (h *Handler) deleteRoles(c *gin.Context) {
	var r batchRequest
	if !bindJSON(c, &r) {
		return
	}
	if len(r.IDs) == 0 {
		writeFields(c, map[string]string{"ids": "ID 列表不能为空"})
		return
	}
	writeMutation(c, h.service.DeleteRoles(c, auditMetadata(c), r.IDs))
}

// roleStatus godoc
// @Summary 修改角色状态
// @Tags 角色管理
// @Security BearerAuth
// @Param id path int true "角色 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/{id}/status [patch]
func (h *Handler) roleStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var r statusRequest
	if !bindJSON(c, &r) {
		return
	}
	if !handlerValidStatus(r.Status) {
		writeFields(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	writeMutation(c, h.service.SetRoleStatus(c, auditMetadata(c), id, r.Status))
}

// roleMenus godoc
// @Summary 分配角色菜单
// @Tags 角色管理
// @Security BearerAuth
// @Param id path int true "角色 ID"
// @Param request body menusRequest true "菜单 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/role/{id}/menus [put]
func (h *Handler) roleMenus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var r menusRequest
	if !bindJSON(c, &r) {
		return
	}
	writeMutation(c, h.service.AssignRoleMenus(c, auditMetadata(c), id, r.MenuIDs))
}

// menuTree godoc
// @Summary 菜单树
// @Tags 菜单管理
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/tree [get]
func (h *Handler) menuTree(c *gin.Context) { v, e := h.service.MenuTree(c); writeResult(c, v, e) }

// menuPage godoc
// @Summary 菜单分页
// @Tags 菜单管理
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param menuName query string false "菜单名称"
// @Param menuType query string false "菜单类型"
// @Param status query int false "状态：0 或 1"
// @Param visible query int false "可见：0 或 1"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/page [get]
func (h *Handler) menuPage(c *gin.Context) {
	status, validStatus := queryOptionalStatus(c, "status")
	visible, validVisible := queryOptionalStatus(c, "visible")
	q := MenuPageQuery{Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10), MenuName: c.Query("menuName"), MenuType: c.Query("menuType"), Status: status, Visible: visible}
	if !validPage(q.Page, q.PageSize) || !validStatus || !validVisible {
		writeFields(c, map[string]string{"page": "页码或每页条数不合法"})
		return
	}
	v, e := h.service.MenuPage(c, q)
	writeResult(c, v, e)
}

// menuDetail godoc
// @Summary 菜单详情
// @Tags 菜单管理
// @Security BearerAuth
// @Param id path int true "菜单 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/{id} [get]
func (h *Handler) menuDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	v, e := h.service.MenuDetail(c, id)
	writeResult(c, v, e)
}

// createMenu godoc
// @Summary 新增菜单
// @Tags 菜单管理
// @Security BearerAuth
// @Param request body menuRequest true "菜单"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu [post]
func (h *Handler) createMenu(c *gin.Context) {
	var r menuRequest
	if !bindJSON(c, &r) {
		return
	}
	in := r.menuInput()
	if fields := validateMenuInput(in); len(fields) > 0 {
		writeFields(c, fields)
		return
	}
	_, e := h.service.CreateMenu(c, auditMetadata(c), in)
	writeMutation(c, e)
}

// updateMenu godoc
// @Summary 修改菜单
// @Tags 菜单管理
// @Security BearerAuth
// @Param id path int true "菜单 ID"
// @Param request body menuRequest true "菜单"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/{id} [put]
func (h *Handler) updateMenu(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var r menuRequest
	if !bindJSON(c, &r) {
		return
	}
	in := r.menuInput()
	if fields := validateMenuInput(in); len(fields) > 0 {
		writeFields(c, fields)
		return
	}
	_, e := h.service.UpdateMenu(c, auditMetadata(c), id, in)
	writeMutation(c, e)
}

// deleteMenu godoc
// @Summary 删除菜单
// @Tags 菜单管理
// @Security BearerAuth
// @Param id path int true "菜单 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/{id} [delete]
func (h *Handler) deleteMenu(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.DeleteMenu(c, auditMetadata(c), id))
}

// deleteMenus godoc
// @Summary 批量删除菜单
// @Tags 菜单管理
// @Security BearerAuth
// @Param request body batchRequest true "菜单 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/batch-delete [post]
func (h *Handler) deleteMenus(c *gin.Context) {
	var r batchRequest
	if !bindJSON(c, &r) {
		return
	}
	if len(r.IDs) == 0 {
		writeFields(c, map[string]string{"ids": "ID 列表不能为空"})
		return
	}
	writeMutation(c, h.service.DeleteMenus(c, auditMetadata(c), r.IDs))
}

// menuStatus godoc
// @Summary 修改菜单状态
// @Tags 菜单管理
// @Security BearerAuth
// @Param id path int true "菜单 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/menu/{id}/status [patch]
func (h *Handler) menuStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var r statusRequest
	if !bindJSON(c, &r) {
		return
	}
	if !handlerValidStatus(r.Status) {
		writeFields(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	writeMutation(c, h.service.SetMenuStatus(c, auditMetadata(c), id, r.Status))
}

func (r roleRequest) roleInput() (RoleInput, map[string]string) {
	fields := map[string]string{}
	if strings.TrimSpace(r.RoleName) == "" {
		fields["roleName"] = "角色名称不能为空"
	}
	if strings.TrimSpace(r.RoleCode) == "" {
		fields["roleCode"] = "角色编码不能为空"
	}
	status := intOr(r.Status, StatusEnabled)
	if !handlerValidStatus(status) {
		fields["status"] = "状态只能为 0 或 1"
	}
	return RoleInput{RoleName: r.RoleName, RoleCode: r.RoleCode, Status: status, SortOrder: intOr(r.SortOrder, 0), Remark: r.Remark}, fields
}
func (r menuRequest) menuInput() MenuInput {
	return MenuInput{ParentID: r.ParentID, MenuName: r.MenuName, MenuType: r.MenuType, Path: r.Path, Component: r.Component, ExternalURL: r.ExternalURL, Icon: r.Icon, PermissionCode: r.PermissionCode, Remark: r.Remark, SortOrder: intOr(r.SortOrder, 0), Visible: intOr(r.Visible, 1), Status: intOr(r.Status, 1)}
}
func validateMenuInput(v MenuInput) map[string]string {
	e := map[string]string{}
	if v.ParentID < 0 {
		e["parentId"] = "父级菜单不能为空"
	}
	if strings.TrimSpace(v.MenuName) == "" {
		e["menuName"] = "菜单名称不能为空"
	}
	if strings.TrimSpace(v.MenuType) == "" {
		e["menuType"] = "菜单类型不能为空"
	}
	if !handlerValidStatus(v.Visible) || !handlerValidStatus(v.Status) {
		e["status"] = "状态只能为 0 或 1"
	}
	return e
}
func intOr(v *int, d int) int {
	if v == nil {
		return d
	}
	return *v
}
func handlerValidStatus(v int) bool { return v == 0 || v == 1 }
func validPage(p, s int) bool       { return p >= 1 && s >= 1 && s <= 500 }
func queryInt(c *gin.Context, key string, d int) int {
	v := c.Query(key)
	if v == "" {
		return d
	}
	n, e := strconv.Atoi(v)
	if e != nil {
		return 0
	}
	return n
}
func queryOptionalStatus(c *gin.Context, key string) (*int, bool) {
	v := c.Query(key)
	if v == "" {
		return nil, true
	}
	n, e := strconv.Atoi(v)
	if e != nil || !handlerValidStatus(n) {
		return nil, false
	}
	return &n, true
}
func pathID(c *gin.Context) (int64, bool) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		writeFields(c, map[string]string{"id": "ID 不合法"})
		return 0, false
	}
	return id, true
}
func bindJSON(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		writeFields(c, map[string]string{"body": "参数错误"})
		return false
	}
	return true
}
func auditMetadata(c *gin.Context) AuditMetadata {
	p, _ := auth.PrincipalFromContext(c.Request.Context())
	meta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return AuditMetadata{ActorID: p.UserID, RequestID: meta.RequestID, ClientIP: meta.ClientIP, UserAgent: meta.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()}
}
func writeResult(c *gin.Context, v any, e error) {
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
func writeFields(c *gin.Context, fields map[string]string) {
	platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", fields)
}
func writeError(c *gin.Context, e error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(e):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(e, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(e, ErrInvalidInput), errors.Is(e, ErrConflict), errors.Is(e, ErrBuiltinProtected):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, e.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
