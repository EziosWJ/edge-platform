package logmgmt

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/EziosWJ/edge-platform/server/internal/rbac"
)

// HandlerService is the log-management use-case boundary consumed by HTTP.
type HandlerService interface {
	LoginLogPage(context.Context, LoginLogPageQuery) (Page[LoginLog], error)
	LoginLogDetail(context.Context, int64) (*LoginLog, error)
	ClearLoginLogs(context.Context, rbac.AuditMetadata) error
	OperLogPage(context.Context, OperLogPageQuery) (Page[OperLogRecord], error)
	OperLogDetail(context.Context, int64) (*OperLogDetail, error)
	ClearOperLogs(context.Context, rbac.AuditMetadata) error
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
		return nil, errors.New("log handler service is required")
	}
	return &Handler{service: service}, nil
}

// RegisterRoutes registers Java-compatible log routes on an already
// authenticated /api/system router group.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	loginLogs := router.Group("/login-log")
	loginLogs.GET("/page", handler.loginLogPage)
	loginLogs.DELETE("/clear", handler.clearLoginLogs)
	loginLogs.GET("/:id", handler.loginLogDetail)
	operLogs := router.Group("/oper-log")
	operLogs.GET("/page", handler.operLogPage)
	operLogs.DELETE("/clear", handler.clearOperLogs)
	operLogs.GET("/:id", handler.operLogDetail)
}

// loginLogPage godoc
// @Summary 登录日志分页
// @Tags 登录日志
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param username query string false "用户名"
// @Param loginStatus query string false "登录状态：SUCCESS 或 FAIL"
// @Param loginIp query string false "登录 IP"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/login-log/page [get]
func (h *Handler) loginLogPage(c *gin.Context) {
	query := LoginLogPageQuery{
		Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10),
		Username: c.Query("username"), LoginStatus: c.Query("loginStatus"), LoginIP: c.Query("loginIp"),
	}
	if !validPage(query.Page, query.PageSize) {
		writeFields(c, pageFields(query.Page, query.PageSize))
		return
	}
	result, err := h.service.LoginLogPage(c.Request.Context(), query)
	writeResult(c, result, err)
}

// loginLogDetail godoc
// @Summary 登录日志详情
// @Tags 登录日志
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/login-log/{id} [get]
func (h *Handler) loginLogDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.LoginLogDetail(c.Request.Context(), id)
	writeResult(c, result, err)
}

// clearLoginLogs godoc
// @Summary 清空登录日志
// @Tags 登录日志
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/login-log/clear [delete]
func (h *Handler) clearLoginLogs(c *gin.Context) {
	writeMutation(c, h.service.ClearLoginLogs(c.Request.Context(), auditMetadata(c)))
}

// operLogPage godoc
// @Summary 操作日志分页
// @Tags 操作日志
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param moduleName query string false "模块名称"
// @Param operationType query string false "操作类型"
// @Param operatorName query string false "操作人"
// @Param operationStatus query string false "操作状态：SUCCESS 或 FAIL"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/oper-log/page [get]
func (h *Handler) operLogPage(c *gin.Context) {
	query := OperLogPageQuery{
		Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10),
		ModuleName: c.Query("moduleName"), OperationType: c.Query("operationType"),
		OperatorName: c.Query("operatorName"), OperationStatus: c.Query("operationStatus"),
	}
	if !validPage(query.Page, query.PageSize) {
		writeFields(c, pageFields(query.Page, query.PageSize))
		return
	}
	result, err := h.service.OperLogPage(c.Request.Context(), query)
	writeResult(c, result, err)
}

// operLogDetail godoc
// @Summary 操作日志详情
// @Tags 操作日志
// @Security BearerAuth
// @Param id path int true "日志 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/oper-log/{id} [get]
func (h *Handler) operLogDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.OperLogDetail(c.Request.Context(), id)
	writeResult(c, result, err)
}

// clearOperLogs godoc
// @Summary 清空操作日志
// @Tags 操作日志
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/oper-log/clear [delete]
func (h *Handler) clearOperLogs(c *gin.Context) {
	writeMutation(c, h.service.ClearOperLogs(c.Request.Context(), auditMetadata(c)))
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeFields(c, map[string]string{"id": "ID 不合法"})
		return 0, false
	}
	return id, true
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func validPage(page, pageSize int) bool { return page >= 1 && pageSize >= 1 && pageSize <= 500 }

func pageFields(page, pageSize int) map[string]string {
	fields := map[string]string{}
	if page < 1 {
		fields["page"] = "页码必须大于 0"
	}
	if pageSize < 1 || pageSize > 500 {
		fields["pageSize"] = "每页条数必须在 1 到 500 之间"
	}
	return fields
}

func auditMetadata(c *gin.Context) rbac.AuditMetadata {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	requestMeta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return rbac.AuditMetadata{
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
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrInvalid):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, ErrInvalid.Error(), nil)
	case errors.Is(err, ErrForbidden):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeForbidden, ErrForbidden.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
