package rbac

import (
	"context"
	"errors"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
)

// Repository is the sole rbac type coupled to GORM.
type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) PageRoles(ctx context.Context, q RolePageQuery) (Page[Role], error) {
	var p Page[Role]
	db := r.db.WithContext(ctx).Model(&Role{}).Where("deleted = 0")
	if q.RoleName != "" {
		db = db.Where("role_name LIKE ?", "%"+q.RoleName+"%")
	}
	if q.RoleCode != "" {
		db = db.Where("role_code LIKE ?", "%"+q.RoleCode+"%")
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	if err := db.Order("sort_order,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error; err != nil {
		return p, err
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	return p, nil
}
func (r *Repository) FindRole(ctx context.Context, id int64) (*Role, error) {
	var v Role
	err := r.db.WithContext(ctx).Where("id=? AND deleted=0", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, err
}
func (r *Repository) RoleCodeExists(ctx context.Context, code string, exclude int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Role{}).Where("role_code=? AND deleted=0 AND id<>?", code, exclude).Count(&n).Error
	return n > 0, err
}
func (r *Repository) CreateRole(ctx context.Context, v Role, e AuditEvent) (Role, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return audit.RecordOn(ctx, tx, e)
	})
	return v, err
}
func (r *Repository) UpdateRole(ctx context.Context, v Role, e AuditEvent) (Role, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Role{}).Where("id=? AND deleted=0", v.ID).Updates(map[string]any{"role_name": v.RoleName, "role_code": v.RoleCode, "status": v.Status, "sort_order": v.SortOrder, "remark": v.Remark}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
	return v, err
}
func (r *Repository) CountUsersByRole(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_user_role").Where("role_id=?", id).Count(&n).Error
	return n, err
}
func (r *Repository) DeleteRole(ctx context.Context, id int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("sys_role_menu").Where("role_id=?", id).Delete(&struct{}{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Role{}).Where("id=?", id).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) DeleteRoles(ctx context.Context, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("sys_role_menu").Where("role_id IN ?", ids).Delete(&struct{}{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Role{}).Where("id IN ?", ids).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) SetRoleStatus(ctx context.Context, id int64, status int, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Role{}).Where("id=? AND deleted=0", id).Update("status", status).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) RoleMenuIDs(ctx context.Context, id int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Table("sys_role_menu").Where("role_id=?", id).Pluck("menu_id", &ids).Error
	return ids, err
}
func (r *Repository) ReplaceRoleMenus(ctx context.Context, id int64, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(ids) > 0 {
			var n int64
			if err := tx.Model(&Menu{}).Where("id IN ? AND deleted=0", ids).Count(&n).Error; err != nil {
				return err
			}
			if n != int64(len(ids)) {
				return ErrNotFound
			}
		}
		if err := tx.Table("sys_role_menu").Where("role_id=?", id).Delete(map[string]any{}).Error; err != nil {
			return err
		}
		for _, mid := range ids {
			if err := tx.Table("sys_role_menu").Create(map[string]any{"role_id": id, "menu_id": mid}).Error; err != nil {
				return err
			}
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) EnabledRoles(ctx context.Context) ([]Role, error) {
	var v []Role
	err := r.db.WithContext(ctx).Where("status=1 AND deleted=0").Order("sort_order,id").Find(&v).Error
	return v, err
}

// HasPermission is the narrow server-side authorization seam used by domain
// modules. It deliberately checks the current user/role/menu graph instead
// of reusing the visible-menu projection, because visibility is a UI concern.
func (r *Repository) HasPermission(ctx context.Context, userID int64, permissionCode string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("sys_user AS u").
		Joins("JOIN sys_user_role AS ur ON ur.user_id = u.id").
		Joins("JOIN sys_role AS r ON r.id = ur.role_id").
		Joins("JOIN sys_role_menu AS rm ON rm.role_id = r.id").
		Joins("JOIN sys_menu AS m ON m.id = rm.menu_id").
		Where("u.id = ? AND u.status = 1 AND u.deleted = 0", userID).
		Where("r.status = 1 AND r.deleted = 0").
		Where("m.permission_code = ? AND m.status = 1 AND m.deleted = 0", permissionCode).
		Count(&count).Error
	return count > 0, err
}

func (r *Repository) ListMenus(ctx context.Context) ([]Menu, error) {
	var v []Menu
	err := r.db.WithContext(ctx).Where("deleted=0").Order("sort_order,id").Find(&v).Error
	return v, err
}
func (r *Repository) PageMenus(ctx context.Context, q MenuPageQuery) (Page[Menu], error) {
	var p Page[Menu]
	db := r.db.WithContext(ctx).Model(&Menu{}).Where("deleted=0")
	if q.MenuName != "" {
		db = db.Where("menu_name LIKE ?", "%"+q.MenuName+"%")
	}
	if q.MenuType != "" {
		db = db.Where("menu_type=?", q.MenuType)
	}
	if q.Status != nil {
		db = db.Where("status=?", *q.Status)
	}
	if q.Visible != nil {
		db = db.Where("visible=?", *q.Visible)
	}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	err := db.Order("sort_order,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	p.Page, p.PageSize = q.Page, q.PageSize
	return p, err
}
func (r *Repository) FindMenu(ctx context.Context, id int64) (*Menu, error) {
	var v Menu
	err := r.db.WithContext(ctx).Where("id=? AND deleted=0", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, err
}
func (r *Repository) PermissionCodeExists(ctx context.Context, code string, exclude int64) (bool, error) {
	if strings.TrimSpace(code) == "" {
		return false, nil
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&Menu{}).Where("permission_code=? AND deleted=0 AND id<>?", code, exclude).Count(&n).Error
	return n > 0, err
}
func (r *Repository) CreateMenu(ctx context.Context, v Menu, e AuditEvent) (Menu, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return audit.RecordOn(ctx, tx, e)
	})
	return v, err
}
func (r *Repository) UpdateMenu(ctx context.Context, v Menu, e AuditEvent) (Menu, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Menu{}).Where("id=? AND deleted=0", v.ID).Updates(map[string]any{"parent_id": v.ParentID, "menu_name": v.MenuName, "menu_type": v.MenuType, "path": v.Path, "component": v.Component, "external_url": v.ExternalURL, "icon": v.Icon, "permission_code": v.PermissionCode, "sort_order": v.SortOrder, "visible": v.Visible, "status": v.Status, "remark": v.Remark}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
	return v, err
}
func (r *Repository) CountChildren(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Menu{}).Where("parent_id=? AND deleted=0", id).Count(&n).Error
	return n, err
}
func (r *Repository) CountRolesByMenu(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("sys_role_menu").Where("menu_id=?", id).Count(&n).Error
	return n, err
}
func (r *Repository) DeleteMenu(ctx context.Context, id int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Menu{}).Where("id=?", id).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) DeleteMenus(ctx context.Context, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Menu{}).Where("id IN ?", ids).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) SetMenuStatus(ctx context.Context, id int64, status int, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Menu{}).Where("id=? AND deleted=0", id).Update("status", status).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
