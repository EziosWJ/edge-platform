package audit

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type operationLog struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	ModuleName      string    `gorm:"column:module_name"`
	OperationType   string    `gorm:"column:operation_type"`
	RequestID       string    `gorm:"column:request_id"`
	RequestMethod   string    `gorm:"column:request_method"`
	RequestURL      string    `gorm:"column:request_url"`
	OperatorID      int64     `gorm:"column:operator_id"`
	OperatorIP      string    `gorm:"column:operator_ip"`
	UserAgent       string    `gorm:"column:user_agent"`
	OperationStatus string    `gorm:"column:operation_status"`
	OperationTime   time.Time `gorm:"column:operation_time"`
	CreateTime      time.Time `gorm:"column:create_time"`
}

func (operationLog) TableName() string { return "sys_oper_log" }

type recorder struct{ db *gorm.DB }

func NewRecorder(db *gorm.DB) Recorder { return &recorder{db: db} }

func (r *recorder) Record(ctx context.Context, e Event) error {
	return r.db.WithContext(ctx).Create(&operationLog{
		ModuleName: e.Resource, OperationType: e.Action,
		RequestID: e.Metadata.RequestID, OperatorID: e.Metadata.ActorID,
		RequestMethod: e.Metadata.RequestMethod, RequestURL: e.Metadata.RequestURL,
		OperatorIP: e.Metadata.ClientIP, UserAgent: e.Metadata.UserAgent,
		OperationStatus: "SUCCESS", OperationTime: time.Now().UTC(), CreateTime: time.Now().UTC(),
	}).Error
}

func RecordOn(ctx context.Context, tx *gorm.DB, e Event) error {
	return tx.WithContext(ctx).Create(&operationLog{
		ModuleName: e.Resource, OperationType: e.Action,
		RequestID: e.Metadata.RequestID, OperatorID: e.Metadata.ActorID,
		RequestMethod: e.Metadata.RequestMethod, RequestURL: e.Metadata.RequestURL,
		OperatorIP: e.Metadata.ClientIP, UserAgent: e.Metadata.UserAgent,
		OperationStatus: "SUCCESS", OperationTime: time.Now().UTC(), CreateTime: time.Now().UTC(),
	}).Error
}
