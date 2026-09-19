package usermgmt

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

const (
	StatusEnabled  = 1
	StatusDisabled = 0
	BuiltinYes     = 1
)

var (
	ErrNotFound    = errors.New("数据不存在")
	ErrBuiltin     = errors.New("内置用户禁止删除")
	ErrConflict    = errors.New("数据已存在")
	ErrInvalid     = errors.New("参数错误")
	ErrOldPassword = errors.New("旧密码错误")
)

type User struct {
	ID            int64      `gorm:"column:id;primaryKey" json:"id"`
	Username      string     `gorm:"column:username" json:"username"`
	Nickname      string     `gorm:"column:nickname" json:"nickname"`
	Password      string     `gorm:"column:password" json:"-"`
	Phone         *string    `gorm:"column:phone" json:"phone"`
	Email         *string    `gorm:"column:email" json:"email"`
	Avatar        *string    `gorm:"column:avatar" json:"avatar"`
	Gender        string     `gorm:"column:gender" json:"gender"`
	DeptID        *int64     `gorm:"column:dept_id" json:"deptId"`
	Status        int        `gorm:"column:status" json:"status"`
	IsBuiltin     int        `gorm:"column:is_builtin" json:"isBuiltin"`
	LastLoginTime *time.Time `gorm:"column:last_login_time" json:"lastLoginTime"`
	LastLoginIP   *string    `gorm:"column:last_login_ip" json:"lastLoginIp"`
	Remark        *string    `gorm:"column:remark" json:"remark"`
	CreateTime    time.Time  `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime    time.Time  `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted       int        `gorm:"column:deleted" json:"-"`
	DeptName      *string    `gorm:"-" json:"deptName"`
	Roles         []Role     `gorm:"-" json:"roles"`
}

func (User) TableName() string { return "sys_user" }

type Role struct {
	ID       int64  `json:"id"`
	RoleName string `gorm:"column:role_name" json:"roleName"`
	RoleCode string `gorm:"column:role_code" json:"roleCode"`
	Status   int    `json:"status"`
}
type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
type PageQuery struct {
	Page, PageSize                   int
	Username, Nickname, Phone, Email string
	Status                           *int
	DeptID                           *int64
}
type Input struct {
	Username, Nickname   string
	Phone, Email, Avatar *string
	Gender               string
	DeptID               *int64
	Status               int
	Remark               *string
}
type PasswordChange struct{ OldPassword, NewPassword string }
type AuditMetadata = audit.Metadata
type AuditEvent = audit.Event
type AuditRecorder = audit.Recorder

type UserPageQuery = PageQuery
type UserCreateInput = Input
type UserUpdateInput = Input
type ChangePasswordInput = PasswordChange
type UserDetail struct{ User }
type ResetPasswordResult struct {
	Password string `json:"password"`
}
type Store interface {
	Page(context.Context, PageQuery) (Page[User], error)
	Find(context.Context, int64) (*User, error)
	UsernameExists(context.Context, string, int64) (bool, error)
	DeptExists(context.Context, int64) (bool, error)
	RolesExist(context.Context, []int64) (bool, error)
	Create(context.Context, User, AuditEvent) (User, error)
	Update(context.Context, User, bool, AuditEvent) error
	Delete(context.Context, int64, AuditEvent) error
	DeleteUsers(context.Context, []int64, AuditEvent) error
	AssignRoles(context.Context, int64, []int64, AuditEvent) error
	ResetPassword(context.Context, int64, string, AuditEvent) error
	ChangePassword(context.Context, int64, string, AuditEvent) error
	UpdateAvatar(context.Context, int64, *string, AuditEvent) error
}
