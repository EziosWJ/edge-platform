package dept

import (
	"context"
	"errors"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) List(c context.Context) (v []Dept, e error) {
	e = r.db.WithContext(c).Where("deleted=0").Order("sort_order,id").Find(&v).Error
	return
}
func (r *Repository) Page(c context.Context, q Query) (p Page, e error) {
	d := r.db.WithContext(c).Model(&Dept{}).Where("deleted=0")
	if q.DeptName != "" {
		d = d.Where("dept_name LIKE ?", "%"+q.DeptName+"%")
	}
	if q.DeptCode != "" {
		d = d.Where("dept_code LIKE ?", "%"+q.DeptCode+"%")
	}
	if q.Status != nil {
		d = d.Where("status=?", *q.Status)
	}
	e = d.Count(&p.Total).Error
	if e == nil {
		e = d.Order("sort_order,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	return
}
func (r *Repository) Find(c context.Context, id int64) (*Dept, error) {
	var v Dept
	e := r.db.WithContext(c).Where("id=? AND deleted=0", id).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (r *Repository) CodeExists(c context.Context, x string, id int64) (b bool, e error) {
	var n int64
	e = r.db.WithContext(c).Model(&Dept{}).Where("dept_code=? AND deleted=0 AND id<>?", x, id).Count(&n).Error
	return n > 0, e
}
func (r *Repository) Create(c context.Context, v Dept, e AuditEvent) (Dept, error) {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return audit.RecordOn(c, tx, e)
	})
	return v, err
}
func (r *Repository) Update(c context.Context, v Dept, e AuditEvent) (Dept, error) {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Dept{}).Where("id=? AND deleted=0", v.ID).Updates(map[string]any{"parent_id": v.ParentID, "dept_name": v.DeptName, "dept_code": v.DeptCode, "leader": v.Leader, "phone": v.Phone, "email": v.Email, "sort_order": v.SortOrder, "status": v.Status, "remark": v.Remark}).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
	return v, err
}
func (r *Repository) Delete(c context.Context, id int64, e AuditEvent) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Dept{}).Where("id=?", id).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) DeleteBatch(c context.Context, ids []int64, e AuditEvent) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Dept{}).Where("id IN ?", ids).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) SetStatus(c context.Context, id int64, s int, e AuditEvent) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Dept{}).Where("id=?", id).Update("status", s).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) CountChildren(c context.Context, id int64) (n int64, e error) {
	e = r.db.WithContext(c).Model(&Dept{}).Where("parent_id=? AND deleted=0", id).Count(&n).Error
	return
}
func (r *Repository) CountUsers(c context.Context, id int64) (n int64, e error) {
	e = r.db.WithContext(c).Table("sys_user").Where("dept_id=? AND deleted=0", id).Count(&n).Error
	return
}
