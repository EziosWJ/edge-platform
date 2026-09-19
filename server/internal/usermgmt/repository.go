package usermgmt

import (
	"context"
	"errors"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
)

type roleChangeNotifier interface {
	RecordRoleChange(context.Context, *gorm.DB, int64, []string, []string) error
}

type Repository struct {
	db       *gorm.DB
	notifier roleChangeNotifier
}

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB, notifiers ...roleChangeNotifier) *Repository {
	var notifier roleChangeNotifier
	if len(notifiers) > 0 {
		notifier = notifiers[0]
	}
	return &Repository{db: db, notifier: notifier}
}
func (r *Repository) Page(ctx context.Context, q PageQuery) (Page[User], error) {
	var p Page[User]
	d := r.db.WithContext(ctx).Model(&User{}).Where("deleted=0")
	if q.Username != "" {
		d = d.Where("username LIKE ?", "%"+q.Username+"%")
	}
	if q.Nickname != "" {
		d = d.Where("nickname LIKE ?", "%"+q.Nickname+"%")
	}
	if q.Phone != "" {
		d = d.Where("phone LIKE ?", "%"+q.Phone+"%")
	}
	if q.Email != "" {
		d = d.Where("email LIKE ?", "%"+q.Email+"%")
	}
	if q.Status != nil {
		d = d.Where("status=?", *q.Status)
	}
	if q.DeptID != nil {
		d = d.Where("dept_id=?", *q.DeptID)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	e := d.Order("create_time DESC,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	p.Page, p.PageSize = q.Page, q.PageSize
	if e != nil {
		return p, e
	}
	if e = r.enrichPage(ctx, p.Records); e != nil {
		return p, e
	}
	return p, nil
}
func (r *Repository) Find(ctx context.Context, id int64) (*User, error) {
	var u User
	e := r.db.WithContext(ctx).Where("id=? AND deleted=0", id).Take(&u).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return &u, r.enrich(ctx, &u)
}
func (r *Repository) enrich(ctx context.Context, u *User) error {
	if u.DeptID != nil {
		var name string
		e := r.db.WithContext(ctx).Table("sys_dept").Select("dept_name").Where("id=? AND deleted=0", *u.DeptID).Scan(&name).Error
		if e != nil {
			return e
		}
		u.DeptName = &name
	}
	return r.db.WithContext(ctx).Table("sys_role r").Select("r.id,r.role_name,r.role_code,r.status").Joins("JOIN sys_user_role ur ON ur.role_id=r.id").Where("ur.user_id=? AND r.deleted=0", u.ID).Scan(&u.Roles).Error
}
func (r *Repository) enrichPage(ctx context.Context, users []User) error {
	if len(users) == 0 {
		return nil
	}

	deptIDs := make([]int64, 0, len(users))
	seenDeptIDs := make(map[int64]struct{}, len(users))
	userIDs := make([]int64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
		if user.DeptID != nil {
			if _, ok := seenDeptIDs[*user.DeptID]; !ok {
				seenDeptIDs[*user.DeptID] = struct{}{}
				deptIDs = append(deptIDs, *user.DeptID)
			}
		}
	}

	deptNames := make(map[int64]string, len(deptIDs))
	if len(deptIDs) > 0 {
		var departments []struct {
			ID       int64  `gorm:"column:id"`
			DeptName string `gorm:"column:dept_name"`
		}
		if err := r.db.WithContext(ctx).Table("sys_dept").Select("id,dept_name").Where("id IN ? AND deleted=0", deptIDs).Scan(&departments).Error; err != nil {
			return err
		}
		for _, department := range departments {
			deptNames[department.ID] = department.DeptName
		}
	}

	var roleRows []struct {
		UserID   int64  `gorm:"column:user_id"`
		ID       int64  `gorm:"column:id"`
		RoleName string `gorm:"column:role_name"`
		RoleCode string `gorm:"column:role_code"`
		Status   int    `gorm:"column:status"`
	}
	if err := r.db.WithContext(ctx).Table("sys_role r").Select("ur.user_id,r.id,r.role_name,r.role_code,r.status").Joins("JOIN sys_user_role ur ON ur.role_id=r.id").Where("ur.user_id IN ? AND r.deleted=0", userIDs).Scan(&roleRows).Error; err != nil {
		return err
	}
	rolesByUser := make(map[int64][]Role, len(userIDs))
	for _, role := range roleRows {
		rolesByUser[role.UserID] = append(rolesByUser[role.UserID], Role{
			ID: role.ID, RoleName: role.RoleName, RoleCode: role.RoleCode, Status: role.Status,
		})
	}

	for i := range users {
		if users[i].DeptID != nil {
			name := deptNames[*users[i].DeptID]
			users[i].DeptName = &name
		}
		users[i].Roles = rolesByUser[users[i].ID]
	}
	return nil
}
func (r *Repository) UsernameExists(ctx context.Context, n string, x int64) (bool, error) {
	var c int64
	e := r.db.WithContext(ctx).Model(&User{}).Where("username=? AND deleted=0 AND id<>?", n, x).Count(&c).Error
	return c > 0, e
}
func (r *Repository) DeptExists(ctx context.Context, id int64) (bool, error) {
	var c int64
	e := r.db.WithContext(ctx).Table("sys_dept").Where("id=? AND deleted=0", id).Count(&c).Error
	return c > 0, e
}
func (r *Repository) RolesExist(ctx context.Context, ids []int64) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	var c int64
	e := r.db.WithContext(ctx).Table("sys_role").Where("id IN ? AND deleted=0", ids).Count(&c).Error
	return c == int64(len(ids)), e
}
func (r *Repository) Create(ctx context.Context, u User, e AuditEvent) (User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		e.ResourceID = u.ID
		return audit.RecordOn(ctx, tx, e)
	})
	return u, err
}
func (r *Repository) Update(ctx context.Context, u User, revokeSessions bool, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id=? AND deleted=0", u.ID).Updates(map[string]any{"nickname": u.Nickname, "phone": u.Phone, "email": u.Email, "avatar": u.Avatar, "gender": u.Gender, "dept_id": u.DeptID, "status": u.Status, "remark": u.Remark}).Error; e != nil {
			return e
		}
		if revokeSessions {
			if e := revoke(tx, u.ID); e != nil {
				return e
			}
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) Delete(ctx context.Context, id int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id=?", id).Update("deleted", 1).Error; e != nil {
			return e
		}
		if e := tx.Table("sys_user_role").Where("user_id=?", id).Delete(&struct{}{}).Error; e != nil {
			return e
		}
		if e := revoke(tx, id); e != nil {
			return e
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) DeleteUsers(ctx context.Context, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id IN ?", ids).Update("deleted", 1).Error; e != nil {
			return e
		}
		if e := tx.Table("sys_user_role").Where("user_id IN ?", ids).Delete(&struct{}{}).Error; e != nil {
			return e
		}
		if e := tx.Table("auth_session").Where("user_id IN ? AND revoked_at IS NULL", ids).Update("revoked_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; e != nil {
			return e
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) AssignRoles(ctx context.Context, id int64, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before []string
		if r.notifier != nil {
			if err := tx.Table("sys_role r").Select("r.role_code").Joins("JOIN sys_user_role ur ON ur.role_id=r.id").Where("ur.user_id=? AND r.deleted=0", id).Pluck("r.role_code", &before).Error; err != nil {
				return err
			}
		}
		if e := tx.Table("sys_user_role").Where("user_id=?", id).Delete(&struct{}{}).Error; e != nil {
			return e
		}
		for _, v := range ids {
			if e := tx.Table("sys_user_role").Create(map[string]any{"user_id": id, "role_id": v}).Error; e != nil {
				return e
			}
		}
		if r.notifier != nil {
			var after []string
			if err := tx.Table("sys_role").Where("id IN ? AND deleted=0", ids).Pluck("role_code", &after).Error; err != nil {
				return err
			}
			if !sameStrings(before, after) {
				if err := r.notifier.RecordRoleChange(ctx, tx, id, before, after); err != nil {
					return err
				}
			}
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, v := range counts {
		if v != 0 {
			return false
		}
	}
	return true
}
func (r *Repository) ResetPassword(ctx context.Context, id int64, p string, e AuditEvent) error {
	return r.password(ctx, id, p, e)
}
func (r *Repository) ChangePassword(ctx context.Context, id int64, p string, e AuditEvent) error {
	return r.password(ctx, id, p, e)
}
func (r *Repository) password(ctx context.Context, id int64, p string, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id=? AND deleted=0", id).Update("password", p).Error; e != nil {
			return e
		}
		if e := revoke(tx, id); e != nil {
			return e
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func (r *Repository) UpdateAvatar(ctx context.Context, id int64, a *string, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id=? AND deleted=0", id).Update("avatar", a).Error; e != nil {
			return e
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
func revoke(tx *gorm.DB, id int64) error {
	return tx.Table("auth_session").Where("user_id=? AND revoked_at IS NULL", id).Update("revoked_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}
