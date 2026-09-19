package dept

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

const (
	StatusDisabled = 0
	StatusEnabled  = 1
	BuiltinYes     = 1
)

var (
	ErrNotFound      = errors.New("数据不存在")
	ErrInvalid       = errors.New("参数错误")
	ErrConflict      = errors.New("部门编码已存在")
	ErrBuiltin       = errors.New("内置部门禁止修改编码")
	ErrDeleteBuiltin = errors.New("内置部门禁止删除")
	ErrHasChildren   = errors.New("存在子部门，禁止删除")
	ErrHasUsers      = errors.New("部门已关联用户，禁止删除")
)

type Dept struct {
	ID         int64     `gorm:"column:id;primaryKey" json:"id"`
	ParentID   int64     `gorm:"column:parent_id" json:"parentId"`
	DeptName   string    `gorm:"column:dept_name" json:"deptName"`
	DeptCode   string    `gorm:"column:dept_code" json:"deptCode"`
	Leader     *string   `gorm:"column:leader" json:"leader"`
	Phone      *string   `gorm:"column:phone" json:"phone"`
	Email      *string   `gorm:"column:email" json:"email"`
	SortOrder  int       `gorm:"column:sort_order" json:"sortOrder"`
	Status     int       `gorm:"column:status" json:"status"`
	IsBuiltin  int       `gorm:"column:is_builtin" json:"isBuiltin"`
	Remark     *string   `gorm:"column:remark" json:"remark"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    int       `gorm:"column:deleted" json:"-"`
	Children   []Dept    `gorm:"-" json:"children"`
}

func (Dept) TableName() string { return "sys_dept" }

type Page struct {
	Records  []Dept `json:"records"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type Query struct {
	Page, PageSize     int
	DeptName, DeptCode string
	Status             *int
}
type Input struct {
	ParentID                     int64
	DeptName, DeptCode           string
	Leader, Phone, Email, Remark *string
	SortOrder, Status            int
}
type Store interface {
	List(context.Context) ([]Dept, error)
	Page(context.Context, Query) (Page, error)
	Find(context.Context, int64) (*Dept, error)
	CodeExists(context.Context, string, int64) (bool, error)
	Create(context.Context, Dept, AuditEvent) (Dept, error)
	Update(context.Context, Dept, AuditEvent) (Dept, error)
	Delete(context.Context, int64, AuditEvent) error
	DeleteBatch(context.Context, []int64, AuditEvent) error
	SetStatus(context.Context, int64, int, AuditEvent) error
	CountChildren(context.Context, int64) (int64, error)
	CountUsers(context.Context, int64) (int64, error)
}
type AuditEvent = audit.Event
type AuditMetadata = audit.Metadata
type AuditRecorder = audit.Recorder
