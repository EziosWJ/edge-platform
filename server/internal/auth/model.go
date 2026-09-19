// Package auth contains authentication rules and persistence for the Go API.
package auth

import (
	"context"
	"errors"
	"time"
)

const (
	UserStatusEnabled  = 1
	LoginStatusSuccess = "SUCCESS"
	LoginStatusFailure = "FAIL"
)

var (
	ErrUserNotFound       = errors.New("auth user not found")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("用户已禁用")
	ErrUnauthenticated    = errors.New("未登录或 token 已失效")
	ErrInvalidToken       = errors.New("invalid JWT")
)

// User is the authentication-focused projection of sys_user.
type User struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	Username      string     `gorm:"column:username"`
	Nickname      string     `gorm:"column:nickname"`
	PasswordHash  string     `gorm:"column:password"`
	Phone         *string    `gorm:"column:phone"`
	Email         *string    `gorm:"column:email"`
	Avatar        *string    `gorm:"column:avatar"`
	DeptID        *int64     `gorm:"column:dept_id"`
	Status        int        `gorm:"column:status"`
	LastLoginTime *time.Time `gorm:"column:last_login_time"`
	LastLoginIP   *string    `gorm:"column:last_login_ip"`
	Deleted       int        `gorm:"column:deleted"`
}

func (User) TableName() string { return "sys_user" }

// AuthSession persists the server-side validity state for a JWT jti.
type AuthSession struct {
	ID         int64      `gorm:"column:id;primaryKey"`
	UserID     int64      `gorm:"column:user_id"`
	JTI        string     `gorm:"column:jti"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	CreateTime time.Time  `gorm:"column:create_time;autoCreateTime"`
}

func (AuthSession) TableName() string { return "auth_session" }

// LoginLog is the sys_login_log record written for every credential outcome.
type LoginLog struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	Username    string    `gorm:"column:username"`
	LoginStatus string    `gorm:"column:login_status"`
	LoginIP     string    `gorm:"column:login_ip"`
	UserAgent   string    `gorm:"column:user_agent"`
	Message     string    `gorm:"column:message"`
	LoginTime   time.Time `gorm:"column:login_time"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime"`
}

func (LoginLog) TableName() string { return "sys_login_log" }

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginMetadata is HTTP-derived metadata supplied by the Handler. It keeps the
// Service independent of Gin and its context type.
type LoginMetadata struct {
	ClientIP  string
	UserAgent string
}

type LoginResult struct {
	TokenName  string `json:"tokenName"`
	TokenValue string `json:"tokenValue"`
	ExpiresIn  int64  `json:"expiresIn"`
}

type CurrentUser struct {
	ID            int64             `json:"id"`
	Username      string            `json:"username"`
	Nickname      string            `json:"nickname"`
	Avatar        *string           `json:"avatar"`
	Phone         *string           `json:"phone"`
	Email         *string           `json:"email"`
	Dept          *CurrentUserDept  `json:"dept"`
	Roles         []CurrentUserRole `json:"roles"`
	LastLoginTime *time.Time        `json:"lastLoginTime"`
	LastLoginIP   *string           `json:"lastLoginIp"`
}

type CurrentUserDept struct {
	ID       int64  `json:"id"`
	DeptName string `json:"deptName"`
	DeptCode string `json:"deptCode"`
}

type CurrentUserRole struct {
	ID       int64  `json:"id"`
	RoleName string `json:"roleName"`
	RoleCode string `json:"roleCode"`
}

type CurrentUserMenu struct {
	ID             int64             `json:"id"`
	ParentID       int64             `json:"parentId"`
	MenuName       string            `json:"menuName"`
	MenuType       string            `json:"menuType"`
	Path           string            `json:"path"`
	Component      *string           `json:"component"`
	ExternalURL    *string           `json:"externalUrl"`
	Icon           *string           `json:"icon"`
	PermissionCode *string           `json:"permissionCode"`
	SortOrder      int               `json:"sortOrder"`
	Visible        int               `json:"visible"`
	Children       []CurrentUserMenu `json:"children" gorm:"-"`
}

// Principal is the authenticated identity exposed to HTTP middleware.
type Principal struct {
	UserID    int64
	JTI       string
	ExpiresAt time.Time
}

// Authenticator is the small interface a future HTTP middleware needs to
// validate a Bearer token without importing the Service concrete type.
type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

// Store is the authentication persistence boundary. Its small interface is a
// real cross-layer boundary: Service has no GORM dependency and can be tested
// without a database.
type Store interface {
	FindUserByUsername(context.Context, string) (*User, error)
	RecordLoginFailure(context.Context, LoginLog) error
	CompleteLogin(context.Context, int64, time.Time, string, AuthSession, LoginLog) error
	IsSessionActive(context.Context, int64, string, time.Time) (bool, error)
	RevokeSession(context.Context, int64, string, time.Time) error
	RevokeSessionsByUserID(context.Context, int64, time.Time) error
	FindCurrentUser(context.Context, int64) (*CurrentUser, error)
	FindVisibleMenusByUserID(context.Context, int64) ([]CurrentUserMenu, error)
}
