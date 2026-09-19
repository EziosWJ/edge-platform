// Package rbac owns role and menu management rules.
package rbac

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

type AuditMetadata = audit.Metadata
type AuditEvent = audit.Event
type AuditRecorder = audit.Recorder

const (
	StatusDisabled = 0
	StatusEnabled  = 1
	BuiltinNo      = 0
	BuiltinYes     = 1
)

var (
	ErrNotFound         = errors.New("数据不存在")
	ErrBuiltinProtected = errors.New("内置数据受保护")
	ErrConflict         = errors.New("数据已存在")
	ErrInvalidInput     = errors.New("参数错误")
)

type Role struct {
	ID         int64     `gorm:"column:id;primaryKey" json:"id"`
	RoleName   string    `gorm:"column:role_name" json:"roleName"`
	RoleCode   string    `gorm:"column:role_code" json:"roleCode"`
	Status     int       `gorm:"column:status" json:"status"`
	SortOrder  int       `gorm:"column:sort_order" json:"sortOrder"`
	IsBuiltin  int       `gorm:"column:is_builtin" json:"isBuiltin"`
	Remark     *string   `gorm:"column:remark" json:"remark"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    int       `gorm:"column:deleted" json:"-"`
}

type RoleDetail struct {
	Role
	MenuIDs []int64 `json:"menuIds"`
}

func (Role) TableName() string { return "sys_role" }

type Menu struct {
	ID             int64     `gorm:"column:id;primaryKey" json:"id"`
	ParentID       int64     `gorm:"column:parent_id" json:"parentId"`
	MenuName       string    `gorm:"column:menu_name" json:"menuName"`
	MenuType       string    `gorm:"column:menu_type" json:"menuType"`
	Path           *string   `gorm:"column:path" json:"path"`
	Component      *string   `gorm:"column:component" json:"component"`
	ExternalURL    *string   `gorm:"column:external_url" json:"externalUrl"`
	Icon           *string   `gorm:"column:icon" json:"icon"`
	PermissionCode *string   `gorm:"column:permission_code" json:"permissionCode"`
	SortOrder      int       `gorm:"column:sort_order" json:"sortOrder"`
	Visible        int       `gorm:"column:visible" json:"visible"`
	Status         int       `gorm:"column:status" json:"status"`
	IsBuiltin      int       `gorm:"column:is_builtin" json:"isBuiltin"`
	Remark         *string   `gorm:"column:remark" json:"remark"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted        int       `gorm:"column:deleted" json:"-"`
	Children       []Menu    `gorm:"-" json:"children"`
}

func (Menu) TableName() string { return "sys_menu" }

type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
type RolePageQuery struct {
	Page     int
	PageSize int
	RoleName string
	RoleCode string
	Status   *int
}
type MenuPageQuery struct {
	Page     int
	PageSize int
	MenuName string
	MenuType string
	Status   *int
	Visible  *int
}
type RoleInput struct {
	RoleName  string
	RoleCode  string
	Status    int
	SortOrder int
	Remark    *string
}
type MenuInput struct {
	ParentID                                                   int64
	MenuName                                                   string
	MenuType                                                   string
	Path, Component, ExternalURL, Icon, PermissionCode, Remark *string
	SortOrder, Visible, Status                                 int
}

type Store interface {
	PageRoles(context.Context, RolePageQuery) (Page[Role], error)
	FindRole(context.Context, int64) (*Role, error)
	RoleCodeExists(context.Context, string, int64) (bool, error)
	CreateRole(context.Context, Role, AuditEvent) (Role, error)
	UpdateRole(context.Context, Role, AuditEvent) (Role, error)
	CountUsersByRole(context.Context, int64) (int64, error)
	DeleteRole(context.Context, int64, AuditEvent) error
	DeleteRoles(context.Context, []int64, AuditEvent) error
	SetRoleStatus(context.Context, int64, int, AuditEvent) error
	RoleMenuIDs(context.Context, int64) ([]int64, error)
	ReplaceRoleMenus(context.Context, int64, []int64, AuditEvent) error
	EnabledRoles(context.Context) ([]Role, error)
	ListMenus(context.Context) ([]Menu, error)
	PageMenus(context.Context, MenuPageQuery) (Page[Menu], error)
	FindMenu(context.Context, int64) (*Menu, error)
	PermissionCodeExists(context.Context, string, int64) (bool, error)
	CreateMenu(context.Context, Menu, AuditEvent) (Menu, error)
	UpdateMenu(context.Context, Menu, AuditEvent) (Menu, error)
	CountChildren(context.Context, int64) (int64, error)
	CountRolesByMenu(context.Context, int64) (int64, error)
	DeleteMenu(context.Context, int64, AuditEvent) error
	DeleteMenus(context.Context, []int64, AuditEvent) error
	SetMenuStatus(context.Context, int64, int, AuditEvent) error
}
