// Package dictionary owns dictionary type, dictionary data, and item lookup rules.
package dictionary

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

const (
	StatusDisabled = 0
	StatusEnabled  = 1
	BuiltinNo      = 0
	BuiltinYes     = 1
)

var (
	ErrNotFound          = errors.New("数据不存在")
	ErrInvalidInput      = errors.New("参数错误")
	ErrDictCodeConflict  = errors.New("字典编码已存在")
	ErrDictValueConflict = errors.New("字典值已存在")
	ErrBuiltinProtected  = errors.New("内置字典类型受保护")
	ErrTypeHasData       = errors.New("字典类型下存在字典数据，禁止删除")
)

type DictType struct {
	ID         int64     `gorm:"column:id;primaryKey" json:"id"`
	DictName   string    `gorm:"column:dict_name" json:"dictName"`
	DictCode   string    `gorm:"column:dict_code" json:"dictCode"`
	Status     int       `gorm:"column:status" json:"status"`
	SortOrder  int       `gorm:"column:sort_order" json:"sortOrder"`
	IsBuiltin  int       `gorm:"column:is_builtin" json:"isBuiltin"`
	Remark     *string   `gorm:"column:remark" json:"remark"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    int       `gorm:"column:deleted" json:"-"`
}

func (DictType) TableName() string { return "sys_dict_type" }

type DictData struct {
	ID         int64     `gorm:"column:id;primaryKey" json:"id"`
	DictTypeID int64     `gorm:"column:dict_type_id" json:"dictTypeId"`
	DictCode   string    `gorm:"column:dict_code;->" json:"dictCode"`
	DictLabel  string    `gorm:"column:dict_label" json:"dictLabel"`
	DictValue  string    `gorm:"column:dict_value" json:"dictValue"`
	SortOrder  int       `gorm:"column:sort_order" json:"sortOrder"`
	Remark     *string   `gorm:"column:remark" json:"remark"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    int       `gorm:"column:deleted" json:"-"`
}

func (DictData) TableName() string { return "sys_dict_data" }

type DictItem struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	SortOrder int    `json:"sortOrder"`
}

type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type TypePageQuery struct {
	Page, PageSize     int
	DictName, DictCode string
	Status             *int
}

type DataPageQuery struct {
	Page, PageSize       int
	DictTypeID           *int64
	DictCode             string
	DictLabel, DictValue string
}

type TypeInput struct {
	DictName, DictCode string
	Status             *int
	SortOrder          *int
	Remark             *string
}

type DataInput struct {
	DictTypeID           int64
	DictLabel, DictValue string
	SortOrder            *int
	Remark               *string
}

type AuditMetadata = audit.Metadata
type AuditEvent = audit.Event
type AuditRecorder = audit.Recorder

type Store interface {
	PageTypes(context.Context, TypePageQuery) (Page[DictType], error)
	FindType(context.Context, int64) (*DictType, error)
	DictCodeExists(context.Context, string, int64) (bool, error)
	CreateType(context.Context, DictType, AuditEvent) (DictType, error)
	UpdateType(context.Context, DictType, AuditEvent) (DictType, error)
	CountDataByType(context.Context, int64) (int64, error)
	DeleteTypes(context.Context, []int64, AuditEvent) error
	SetTypeStatus(context.Context, int64, int, AuditEvent) error

	PageData(context.Context, DataPageQuery) (Page[DictData], error)
	FindData(context.Context, int64) (*DictData, error)
	DictValueExists(context.Context, int64, string, int64) (bool, error)
	CreateData(context.Context, DictData, AuditEvent) (DictData, error)
	UpdateData(context.Context, DictData, AuditEvent) (DictData, error)
	DeleteData(context.Context, []int64, AuditEvent) error
	Items(context.Context, string) ([]DictItem, error)
}
