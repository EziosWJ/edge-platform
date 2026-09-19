package logmgmt

import (
	"context"
	"errors"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) LoginLogPage(ctx context.Context, q LoginLogPageQuery) (Page[LoginLog], error) {
	var p Page[LoginLog]
	db := r.db.WithContext(ctx).Model(&LoginLog{})
	if q.Username != "" {
		db = db.Where("username LIKE ?", "%"+q.Username+"%")
	}
	if q.LoginStatus != "" {
		db = db.Where("login_status=?", q.LoginStatus)
	}
	if q.LoginIP != "" {
		db = db.Where("login_ip LIKE ?", "%"+q.LoginIP+"%")
	}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	err := db.Order("login_time DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, err
}

func (r *Repository) FindLoginLog(ctx context.Context, id int64) (*LoginLog, error) {
	var log LoginLog
	err := r.db.WithContext(ctx).Where("id=?", id).Take(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *Repository) ClearLoginLogs(ctx context.Context, e audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1=1").Delete(&LoginLog{}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

func (r *Repository) OperLogPage(ctx context.Context, q OperLogPageQuery) (Page[OperLogRecord], error) {
	p := Page[OperLogRecord]{Records: []OperLogRecord{}}
	db := r.db.WithContext(ctx).Table("sys_oper_log l").
		Select("l.id,l.module_name,l.operation_type,l.request_method,l.request_url," +
			operatorName + " AS operator_name,l.operator_ip,l.operation_status,l.cost_time,l.operation_time").
		Joins("LEFT JOIN sys_user u ON u.id=l.operator_id AND u.deleted=0")
	if q.ModuleName != "" {
		db = db.Where("l.module_name LIKE ?", "%"+q.ModuleName+"%")
	}
	if q.OperationType != "" {
		db = db.Where("l.operation_type=?", q.OperationType)
	}
	if q.OperatorName != "" {
		db = db.Where("("+operatorName+") LIKE ?", "%"+q.OperatorName+"%")
	}
	if q.OperationStatus != "" {
		db = db.Where("l.operation_status=?", q.OperationStatus)
	}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	err := db.Order("l.operation_time DESC,l.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&p.Records).Error
	return p, err
}

func (r *Repository) FindOperLog(ctx context.Context, id int64) (*OperLogDetail, error) {
	var log OperLogDetail
	err := r.db.WithContext(ctx).Table("sys_oper_log l").
		Select("l.id,l.module_name,l.operation_type,l.request_method,l.request_url,"+
			operatorName+" AS operator_name,l.operator_ip,l.operation_status,l.cost_time,l.operation_time,"+
			"l.request_params,l.response_result,l.error_message").
		Joins("LEFT JOIN sys_user u ON u.id=l.operator_id AND u.deleted=0").
		Where("l.id=?", id).Take(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *Repository) ClearOperLogs(ctx context.Context, e audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1=1").Delete(&OperLogDetail{}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
