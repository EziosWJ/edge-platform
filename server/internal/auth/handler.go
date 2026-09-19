package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
)

// HandlerService is the HTTP-facing subset of authentication use cases.
// Keeping it small makes Handler independent of the concrete Service.
type HandlerService interface {
	Login(context.Context, LoginInput, LoginMetadata) (LoginResult, error)
	Logout(context.Context, Principal) error
	CurrentUser(context.Context, Principal) (*CurrentUser, error)
	CurrentUserMenus(context.Context, Principal) ([]CurrentUserMenu, error)
}

// Handler adapts the authentication use cases to the existing /api/auth HTTP
// contract. It contains no token, database, or business-rule implementation.
type Handler struct {
	service       HandlerService
	authenticator Authenticator
}

func NewHandler(service HandlerService, authenticator Authenticator) (*Handler, error) {
	if service == nil {
		return nil, errors.New("auth handler service is required")
	}
	if authenticator == nil {
		return nil, errors.New("auth handler authenticator is required")
	}
	return &Handler{service: service, authenticator: authenticator}, nil
}

// RegisterRoutes keeps the existing React contract unchanged. Only login is
// public; the remaining endpoints require a valid Bearer session.
func RegisterRoutes(router gin.IRouter, handler *Handler) {
	group := router.Group("/api/auth")
	group.POST("/login", handler.Login)

	protected := group.Group("")
	protected.Use(BearerMiddleware(handler.authenticator))
	protected.POST("/logout", handler.Logout)
	protected.GET("/me", handler.CurrentUser)
	protected.GET("/menus", handler.CurrentUserMenus)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

//nolint:unused // Swaggo reads this type from handler annotations.
type loginResponseEnvelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    LoginResult `json:"data"`
}

//nolint:unused // Swaggo reads this type from handler annotations.
type emptyResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

//nolint:unused // Swaggo reads this type from handler annotations.
type errorResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Login handles password authentication.
// @Summary 登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body loginRequest true "登录请求"
// @Success 200 {object} loginResponseEnvelope
// @Failure 400 {object} errorResponseEnvelope
// @Failure 429 {object} errorResponseEnvelope
// @Header 429 {string} Retry-After "等待重试的秒数"
// @Router /api/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}

	if fields := loginFieldErrors(request); len(fields) > 0 {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", fields)
		return
	}

	result, err := h.service.Login(c.Request.Context(), LoginInput(request), LoginMetadata{
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		handleLoginError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

// Logout revokes the current JWT session.
// @Summary 退出登录
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} emptyResponseEnvelope
// @Failure 401 {object} errorResponseEnvelope
// @Router /api/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	principal, ok := PrincipalFromContext(c.Request.Context())
	if !ok {
		writeUnauthenticated(c)
		return
	}
	if err := h.service.Logout(c.Request.Context(), principal); err != nil {
		handleProtectedError(c, err)
		return
	}
	platformhttp.OK(c, nil)
}

// CurrentUser returns the authenticated user's profile.
// @Summary 当前用户信息
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} currentUserResponseEnvelope
// @Failure 401 {object} errorResponseEnvelope
// @Router /api/auth/me [get]
func (h *Handler) CurrentUser(c *gin.Context) {
	principal, ok := PrincipalFromContext(c.Request.Context())
	if !ok {
		writeUnauthenticated(c)
		return
	}
	user, err := h.service.CurrentUser(c.Request.Context(), principal)
	if err != nil {
		handleProtectedError(c, err)
		return
	}
	platformhttp.OK(c, toCurrentUserResponse(user))
}

// CurrentUserMenus returns the authenticated user's visible menu tree.
// @Summary 当前用户可见菜单
// @Tags 认证
// @Security BearerAuth
// @Success 200 {object} currentUserMenusResponseEnvelope
// @Failure 401 {object} errorResponseEnvelope
// @Router /api/auth/menus [get]
func (h *Handler) CurrentUserMenus(c *gin.Context) {
	principal, ok := PrincipalFromContext(c.Request.Context())
	if !ok {
		writeUnauthenticated(c)
		return
	}
	menus, err := h.service.CurrentUserMenus(c.Request.Context(), principal)
	if err != nil {
		handleProtectedError(c, err)
		return
	}
	platformhttp.OK(c, toCurrentUserMenuResponses(menus))
}

func loginFieldErrors(request loginRequest) map[string]string {
	fields := make(map[string]string, 2)
	if strings.TrimSpace(request.Username) == "" {
		fields["username"] = "用户名不能为空"
	}
	if request.Password == "" {
		fields["password"] = "密码不能为空"
	}
	return fields
}

func handleLoginError(c *gin.Context, err error) {
	var rateLimitError *LoginRateLimitError
	if errors.As(err, &rateLimitError) {
		retryAfter := int64((rateLimitError.RetryAfter + time.Second - 1) / time.Second)
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
		platformhttp.WriteError(c, http.StatusTooManyRequests, platformhttp.CodeTooManyRequests, "请求过于频繁，请稍后再试", nil)
		return
	}
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrInvalidCredentials):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, ErrInvalidCredentials.Error(), nil)
	case errors.Is(err, ErrUserDisabled):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeForbidden, ErrUserDisabled.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}

func handleProtectedError(c *gin.Context, err error) {
	if platformhttp.IsTemporaryUnavailable(err) {
		platformhttp.TemporaryUnavailable(c)
		return
	}
	if errors.Is(err, ErrUnauthenticated) {
		writeUnauthenticated(c)
		return
	}
	platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
}

func writeUnauthenticated(c *gin.Context) {
	platformhttp.WriteError(c, http.StatusUnauthorized, platformhttp.CodeUnauthorized, ErrUnauthenticated.Error(), nil)
}

type currentUserResponse struct {
	ID            int64                     `json:"id"`
	Username      string                    `json:"username"`
	Nickname      string                    `json:"nickname"`
	Avatar        *string                   `json:"avatar"`
	Phone         *string                   `json:"phone"`
	Email         *string                   `json:"email"`
	Dept          *currentUserDeptResponse  `json:"dept"`
	Roles         []currentUserRoleResponse `json:"roles"`
	LastLoginTime *time.Time                `json:"lastLoginTime"`
	LastLoginIP   *string                   `json:"lastLoginIp"`
}

type currentUserDeptResponse struct {
	ID       int64  `json:"id"`
	DeptName string `json:"deptName"`
	DeptCode string `json:"deptCode"`
}

type currentUserRoleResponse struct {
	ID       int64  `json:"id"`
	RoleName string `json:"roleName"`
	RoleCode string `json:"roleCode"`
}

type currentUserMenuResponse struct {
	ID             int64                     `json:"id"`
	ParentID       int64                     `json:"parentId"`
	MenuName       string                    `json:"menuName"`
	MenuType       string                    `json:"menuType"`
	Path           string                    `json:"path"`
	Component      *string                   `json:"component"`
	Icon           *string                   `json:"icon"`
	PermissionCode *string                   `json:"permissionCode"`
	SortOrder      int                       `json:"sortOrder"`
	Visible        int                       `json:"visible"`
	Children       []currentUserMenuResponse `json:"children"`
}

//nolint:unused // Swaggo reads this type from handler annotations.
type currentUserResponseEnvelope struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    currentUserResponse `json:"data"`
}

//nolint:unused // Swaggo reads this type from handler annotations.
type currentUserMenusResponseEnvelope struct {
	Code    int                       `json:"code"`
	Message string                    `json:"message"`
	Data    []currentUserMenuResponse `json:"data"`
}

func toCurrentUserResponse(user *CurrentUser) currentUserResponse {
	roles := make([]currentUserRoleResponse, len(user.Roles))
	for index, role := range user.Roles {
		roles[index] = currentUserRoleResponse(role)
	}

	response := currentUserResponse{
		ID:            user.ID,
		Username:      user.Username,
		Nickname:      user.Nickname,
		Avatar:        user.Avatar,
		Phone:         user.Phone,
		Email:         user.Email,
		Roles:         roles,
		LastLoginTime: user.LastLoginTime,
		LastLoginIP:   user.LastLoginIP,
	}
	if user.Dept != nil {
		response.Dept = &currentUserDeptResponse{ID: user.Dept.ID, DeptName: user.Dept.DeptName, DeptCode: user.Dept.DeptCode}
	}
	return response
}

func toCurrentUserMenuResponses(menus []CurrentUserMenu) []currentUserMenuResponse {
	responses := make([]currentUserMenuResponse, len(menus))
	for index, menu := range menus {
		responses[index] = currentUserMenuResponse{
			ID:             menu.ID,
			ParentID:       menu.ParentID,
			MenuName:       menu.MenuName,
			MenuType:       menu.MenuType,
			Path:           menu.Path,
			Component:      menu.Component,
			Icon:           menu.Icon,
			PermissionCode: menu.PermissionCode,
			SortOrder:      menu.SortOrder,
			Visible:        menu.Visible,
			Children:       toCurrentUserMenuResponses(menu.Children),
		}
	}
	return responses
}
