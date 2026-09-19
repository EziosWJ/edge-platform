// Package logmgmt owns login-log and operation-log management APIs.
package logmgmt

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

var (
	ErrNotFound  = errors.New("数据不存在")
	ErrInvalid   = errors.New("参数错误")
	ErrForbidden = errors.New("无权限")
)

// LoginLog is the sys_login_log projection returned by the API. loginLocation,
// userAgent and createTime are included for Java VO compatibility although the
// frontend does not read them.
type LoginLog struct {
	ID            int64     `gorm:"column:id;primaryKey" json:"id"`
	Username      string    `gorm:"column:username" json:"username"`
	LoginStatus   string    `gorm:"column:login_status" json:"loginStatus"`
	LoginIP       string    `gorm:"column:login_ip" json:"loginIp"`
	LoginLocation string    `gorm:"column:login_location" json:"loginLocation"`
	Browser       string    `gorm:"column:browser" json:"browser"`
	OS            string    `gorm:"column:os" json:"os"`
	UserAgent     string    `gorm:"column:user_agent" json:"userAgent"`
	Message       string    `gorm:"column:message" json:"message"`
	LoginTime     time.Time `gorm:"column:login_time" json:"loginTime"`
	CreateTime    time.Time `gorm:"column:create_time" json:"createTime"`
}

func (LoginLog) TableName() string { return "sys_login_log" }

// OperLogRecord is the sys_oper_log projection returned by the page API.
// requestParams/responseResult/errorMessage are exposed only through detail.
type OperLogRecord struct {
	ID              int64     `gorm:"column:id;primaryKey" json:"id"`
	ModuleName      string    `gorm:"column:module_name" json:"moduleName"`
	OperationType   string    `gorm:"column:operation_type" json:"operationType"`
	RequestMethod   string    `gorm:"column:request_method" json:"requestMethod"`
	RequestURL      string    `gorm:"column:request_url" json:"requestUrl"`
	OperatorName    string    `gorm:"column:operator_name" json:"operatorName"`
	OperatorIP      string    `gorm:"column:operator_ip" json:"operatorIp"`
	OperationStatus string    `gorm:"column:operation_status" json:"operationStatus"`
	CostTime        int64     `gorm:"column:cost_time" json:"costTime"`
	OperationTime   time.Time `gorm:"column:operation_time" json:"operationTime"`
}

// OperLogDetail extends the page record with the audit payload columns.
type OperLogDetail struct {
	OperLogRecord
	RequestParams  string `gorm:"column:request_params" json:"requestParams"`
	ResponseResult string `gorm:"column:response_result" json:"responseResult"`
	ErrorMessage   string `gorm:"column:error_message" json:"errorMessage"`
}

func (OperLogDetail) TableName() string { return "sys_oper_log" }

// operatorName selects the log's operator_name, falling back to the sys_user
// username so rows recorded before operator_name was populated still resolve.
const operatorName = "COALESCE(NULLIF(l.operator_name, ''), u.username)"

type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type LoginLogPageQuery struct {
	Page, PageSize    int
	Username, LoginIP string
	LoginStatus       string
}

type OperLogPageQuery struct {
	Page, PageSize           int
	ModuleName, OperatorName string
	OperationType            string
	OperationStatus          string
}

// Store is the persistence boundary for both log tables.
type Store interface {
	LoginLogPage(context.Context, LoginLogPageQuery) (Page[LoginLog], error)
	FindLoginLog(context.Context, int64) (*LoginLog, error)
	ClearLoginLogs(context.Context, audit.Event) error
	OperLogPage(context.Context, OperLogPageQuery) (Page[OperLogRecord], error)
	FindOperLog(context.Context, int64) (*OperLogDetail, error)
	ClearOperLogs(context.Context, audit.Event) error
}
