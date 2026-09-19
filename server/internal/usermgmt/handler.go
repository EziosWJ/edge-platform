package usermgmt

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
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

// HandlerService is the user-management use-case boundary consumed by HTTP.
// Mutations receive request and actor metadata so Service can persist audit
// records without depending on Gin.
type HandlerService interface {
	UserPage(context.Context, UserPageQuery) (Page[User], error)
	UserDetail(context.Context, int64) (*UserDetail, error)
	CreateUser(context.Context, AuditMetadata, UserCreateInput) error
	UpdateUser(context.Context, AuditMetadata, int64, UserUpdateInput) error
	DeleteUser(context.Context, AuditMetadata, int64) error
	DeleteUsers(context.Context, AuditMetadata, []int64) error
	SetUserStatus(context.Context, AuditMetadata, int64, int) error
	AssignUserRoles(context.Context, AuditMetadata, int64, []int64) error
	ResetUserPassword(context.Context, AuditMetadata, int64) (ResetPasswordResult, error)
	ChangeCurrentPassword(context.Context, AuditMetadata, int64, ChangePasswordInput) error
	UpdateCurrentAvatar(context.Context, AuditMetadata, int64, *string) error
}

var _ HandlerService = (*Service)(nil)

type Handler struct {
	service HandlerService
}

// ApiEnvelope is the Swagger representation of the established response.
//
//nolint:unused // referenced by Swaggo annotations below
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(service HandlerService) (*Handler, error) {
	if service == nil {
		return nil, errors.New("user handler service is required")
	}
	return &Handler{service: service}, nil
}

// RegisterRoutes registers the Java-compatible user routes on an already
// authenticated /api/system router group.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	users := router.Group("/user")
	users.GET("/page", handler.userPage)
	users.PUT("/me/password", handler.changeCurrentPassword)
	users.PUT("/me/avatar", handler.updateCurrentAvatar)
	users.GET("/:id", handler.userDetail)
	users.POST("", handler.createUser)
	users.PUT("/:id", handler.updateUser)
	users.DELETE("/:id", handler.deleteUser)
	users.POST("/batch-delete", handler.deleteUsers)
	users.PATCH("/:id/status", handler.userStatus)
	users.PUT("/:id/roles", handler.userRoles)
	users.PUT("/:id/reset-password", handler.resetUserPassword)
}

type userCreateRequest struct {
	Username string  `json:"username"`
	Nickname string  `json:"nickname"`
	Phone    *string `json:"phone"`
	Email    *string `json:"email"`
	Avatar   *string `json:"avatar"`
	Gender   string  `json:"gender"`
	DeptID   *int64  `json:"deptId"`
	Status   *int    `json:"status"`
	Remark   *string `json:"remark"`
}

type userUpdateRequest struct {
	Nickname string  `json:"nickname"`
	Phone    *string `json:"phone"`
	Email    *string `json:"email"`
	Avatar   *string `json:"avatar"`
	Gender   string  `json:"gender"`
	DeptID   *int64  `json:"deptId"`
	Status   *int    `json:"status"`
	Remark   *string `json:"remark"`
}

type batchRequest struct {
	IDs []int64 `json:"ids"`
}

type statusRequest struct {
	Status *int `json:"status"`
}

type rolesRequest struct {
	RoleIDs []int64 `json:"roleIds"`
}

type passwordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type avatarRequest struct {
	Avatar string `json:"avatar"`
}

// userPage godoc
// @Summary 用户分页
// @Tags 用户管理
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param username query string false "用户名"
// @Param nickname query string false "昵称"
// @Param phone query string false "手机号"
// @Param email query string false "邮箱"
// @Param status query int false "状态：0 或 1"
// @Param deptId query int false "部门 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/page [get]
func (h *Handler) userPage(c *gin.Context) {
	page, pageOK := queryInt(c, "page", 1)
	pageSize, pageSizeOK := queryInt(c, "pageSize", 10)
	status, statusOK := optionalStatus(c, "status")
	deptID, deptOK := optionalPositiveInt64(c, "deptId")
	if !pageOK || !pageSizeOK || page < 1 || pageSize < 1 || pageSize > 500 || !statusOK || !deptOK {
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
		if !deptOK {
			fields["deptId"] = "部门 ID 不合法"
		}
		writeFields(c, fields)
		return
	}

	result, err := h.service.UserPage(c.Request.Context(), UserPageQuery{
		Page: page, PageSize: pageSize,
		Username: c.Query("username"), Nickname: c.Query("nickname"),
		Phone: c.Query("phone"), Email: c.Query("email"),
		Status: status, DeptID: deptID,
	})
	writeResult(c, result, err)
}

// userDetail godoc
// @Summary 用户详情
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id} [get]
func (h *Handler) userDetail(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.UserDetail(c.Request.Context(), id)
	writeResult(c, result, err)
}

// createUser godoc
// @Summary 新增用户
// @Tags 用户管理
// @Security BearerAuth
// @Param request body userCreateRequest true "用户"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user [post]
func (h *Handler) createUser(c *gin.Context) {
	var request userCreateRequest
	if !bindJSON(c, &request) {
		return
	}
	input, fields := request.input()
	if len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	writeMutation(c, h.service.CreateUser(c.Request.Context(), auditMetadata(c), input))
}

// updateUser godoc
// @Summary 修改用户
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Param request body userUpdateRequest true "用户"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id} [put]
func (h *Handler) updateUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request userUpdateRequest
	if !bindJSON(c, &request) {
		return
	}
	input, fields := request.input()
	if len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	writeMutation(c, h.service.UpdateUser(c.Request.Context(), auditMetadata(c), id, input))
}

// deleteUser godoc
// @Summary 删除用户
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id} [delete]
func (h *Handler) deleteUser(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	writeMutation(c, h.service.DeleteUser(c.Request.Context(), auditMetadata(c), id))
}

// deleteUsers godoc
// @Summary 批量删除用户
// @Tags 用户管理
// @Security BearerAuth
// @Param request body batchRequest true "用户 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/batch-delete [post]
func (h *Handler) deleteUsers(c *gin.Context) {
	var request batchRequest
	if !bindJSON(c, &request) {
		return
	}
	if !validIDs(request.IDs, false) {
		writeFields(c, map[string]string{"ids": "ID 列表不能为空且必须为正整数"})
		return
	}
	writeMutation(c, h.service.DeleteUsers(c.Request.Context(), auditMetadata(c), request.IDs))
}

// userStatus godoc
// @Summary 修改用户状态
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id}/status [patch]
func (h *Handler) userStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request statusRequest
	if !bindJSON(c, &request) {
		return
	}
	if request.Status == nil || !validStatus(*request.Status) {
		writeFields(c, map[string]string{"status": "状态只能为 0 或 1"})
		return
	}
	writeMutation(c, h.service.SetUserStatus(c.Request.Context(), auditMetadata(c), id, *request.Status))
}

// userRoles godoc
// @Summary 分配用户角色
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Param request body rolesRequest true "角色 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id}/roles [put]
func (h *Handler) userRoles(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var request rolesRequest
	if !bindJSON(c, &request) {
		return
	}
	if !validIDs(request.RoleIDs, true) {
		writeFields(c, map[string]string{"roleIds": "角色 ID 必须为正整数"})
		return
	}
	writeMutation(c, h.service.AssignUserRoles(c.Request.Context(), auditMetadata(c), id, request.RoleIDs))
}

// resetUserPassword godoc
// @Summary 重置用户密码
// @Tags 用户管理
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/{id}/reset-password [put]
func (h *Handler) resetUserPassword(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.ResetUserPassword(c.Request.Context(), auditMetadata(c), id)
	writeResult(c, result, err)
}

// changeCurrentPassword godoc
// @Summary 当前用户修改密码
// @Tags 用户管理
// @Security BearerAuth
// @Param request body passwordRequest true "新旧密码"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/me/password [put]
func (h *Handler) changeCurrentPassword(c *gin.Context) {
	actorID, ok := actorID(c)
	if !ok {
		return
	}
	var request passwordRequest
	if !bindJSON(c, &request) {
		return
	}
	fields := map[string]string{}
	if strings.TrimSpace(request.OldPassword) == "" {
		fields["oldPassword"] = "旧密码不能为空"
	}
	if size := utf8.RuneCountInString(request.NewPassword); size < 6 || size > 50 {
		fields["newPassword"] = "新密码长度必须在 6 到 50 之间"
	}
	if len(fields) != 0 {
		writeFields(c, fields)
		return
	}
	input := ChangePasswordInput(request)
	writeMutation(c, h.service.ChangeCurrentPassword(c.Request.Context(), auditMetadata(c), actorID, input))
}

// updateCurrentAvatar godoc
// @Summary 当前用户修改头像
// @Tags 用户管理
// @Security BearerAuth
// @Param request body avatarRequest true "头像地址"
// @Success 200 {object} ApiEnvelope
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/system/user/me/avatar [put]
func (h *Handler) updateCurrentAvatar(c *gin.Context) {
	actorID, ok := actorID(c)
	if !ok {
		return
	}
	var request avatarRequest
	if !bindJSON(c, &request) {
		return
	}
	if strings.TrimSpace(request.Avatar) == "" || utf8.RuneCountInString(request.Avatar) > 255 {
		writeFields(c, map[string]string{"avatar": "头像不能为空且长度不能超过 255"})
		return
	}
	writeMutation(c, h.service.UpdateCurrentAvatar(c.Request.Context(), auditMetadata(c), actorID, &request.Avatar))
}

func (r userCreateRequest) input() (UserCreateInput, map[string]string) {
	input := UserCreateInput{
		Username: r.Username, Nickname: r.Nickname,
		Phone: r.Phone, Email: r.Email, Avatar: r.Avatar,
		Gender: defaultGender(r.Gender), DeptID: r.DeptID,
		Status: intOr(r.Status, StatusEnabled), Remark: r.Remark,
	}
	return input, validateUserInput(input, true)
}

func (r userUpdateRequest) input() (UserUpdateInput, map[string]string) {
	input := UserUpdateInput{
		Nickname: r.Nickname, Phone: r.Phone, Email: r.Email, Avatar: r.Avatar,
		Gender: defaultGender(r.Gender), DeptID: r.DeptID,
		Status: intOr(r.Status, StatusEnabled), Remark: r.Remark,
	}
	return input, validateUserInput(input, false)
}

func validateUserInput(input Input, create bool) map[string]string {
	fields := map[string]string{}
	if create && (strings.TrimSpace(input.Username) == "" || utf8.RuneCountInString(input.Username) > 50) {
		fields["username"] = "用户名不能为空且长度不能超过 50"
	}
	if strings.TrimSpace(input.Nickname) == "" || utf8.RuneCountInString(input.Nickname) > 50 {
		fields["nickname"] = "昵称不能为空且长度不能超过 50"
	}
	validateOptionalString(fields, "phone", input.Phone, 20)
	validateOptionalString(fields, "email", input.Email, 100)
	validateOptionalString(fields, "avatar", input.Avatar, 255)
	validateOptionalString(fields, "remark", input.Remark, 500)
	if input.Email != nil && strings.TrimSpace(*input.Email) != "" && !validEmail(*input.Email) {
		fields["email"] = "邮箱格式不正确"
	}
	if input.DeptID != nil && *input.DeptID <= 0 {
		fields["deptId"] = "部门 ID 不合法"
	}
	if utf8.RuneCountInString(input.Gender) > 20 {
		fields["gender"] = "性别长度不能超过 20"
	}
	if !validStatus(input.Status) {
		fields["status"] = "状态只能为 0 或 1"
	}
	return fields
}

func validateOptionalString(fields map[string]string, name string, value *string, max int) {
	if value != nil && utf8.RuneCountInString(*value) > max {
		fields[name] = "长度不能超过 " + strconv.Itoa(max)
	}
}

func validEmail(value string) bool {
	trimmed := strings.TrimSpace(value)
	address, err := mail.ParseAddress(trimmed)
	return err == nil && address.Address == trimmed
}

func defaultGender(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UNSPECIFIED"
	}
	return value
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
	if err != nil || !validStatus(parsed) {
		return nil, false
	}
	return &parsed, true
}

func optionalPositiveInt64(c *gin.Context, key string) (*int64, bool) {
	value := c.Query(key)
	if value == "" {
		return nil, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, false
	}
	return &parsed, true
}

func validStatus(value int) bool {
	return value == StatusDisabled || value == StatusEnabled
}

func validIDs(ids []int64, allowEmpty bool) bool {
	if !allowEmpty && len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func bindJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		writeFields(c, map[string]string{"body": "参数错误"})
		return false
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

func actorID(c *gin.Context) (int64, bool) {
	principal, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok || principal.UserID <= 0 {
		platformhttp.WriteError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, "未登录", nil)
		return 0, false
	}
	return principal.UserID, true
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
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict), errors.Is(err, ErrBuiltin), errors.Is(err, ErrOldPassword):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
