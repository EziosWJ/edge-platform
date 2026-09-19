package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repository is the GORM implementation of Store. It is the only auth type
// that depends on GORM or database table details.
type Repository struct {
	db *gorm.DB
}

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("username = ? AND deleted = ?", username, 0).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return &user, nil
}

func (r *Repository) RecordLoginFailure(ctx context.Context, log LoginLog) error {
	if err := r.db.WithContext(ctx).Create(&log).Error; err != nil {
		return fmt.Errorf("create failed login log: %w", err)
	}
	return nil
}

func (r *Repository) CompleteLogin(ctx context.Context, userID int64, loginTime time.Time, loginIP string, session AuthSession, log LoginLog) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).
			Where("id = ? AND deleted = ?", userID, 0).
			Updates(map[string]any{
				"last_login_time": loginTime,
				"last_login_ip":   loginIP,
				"update_time":     loginTime,
			})
		if result.Error != nil {
			return fmt.Errorf("update login metadata: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrUserNotFound
		}
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create auth session: %w", err)
		}
		if err := tx.Create(&log).Error; err != nil {
			return fmt.Errorf("create successful login log: %w", err)
		}
		return nil
	})
}

func (r *Repository) IsSessionActive(ctx context.Context, userID int64, jti string, now time.Time) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&AuthSession{}).
		Where("user_id = ? AND jti = ? AND revoked_at IS NULL AND expires_at > ?", userID, jti, now).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check auth session: %w", err)
	}
	return count == 1, nil
}

func (r *Repository) RevokeSession(ctx context.Context, userID int64, jti string, revokedAt time.Time) error {
	err := r.db.WithContext(ctx).Model(&AuthSession{}).
		Where("user_id = ? AND jti = ? AND revoked_at IS NULL", userID, jti).
		Update("revoked_at", revokedAt).Error
	if err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	return nil
}

// RevokeSessionsByUserID is the future user-module boundary for disable,
// delete, and password-reset flows.
func (r *Repository) RevokeSessionsByUserID(ctx context.Context, userID int64, revokedAt time.Time) error {
	err := r.db.WithContext(ctx).Model(&AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", revokedAt).Error
	if err != nil {
		return fmt.Errorf("revoke user auth sessions: %w", err)
	}
	return nil
}

func (r *Repository) FindCurrentUser(ctx context.Context, userID int64) (*CurrentUser, error) {
	var user User
	err := r.db.WithContext(ctx).Where("id = ? AND deleted = ?", userID, 0).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find current user: %w", err)
	}

	current := &CurrentUser{
		ID:            user.ID,
		Username:      user.Username,
		Nickname:      user.Nickname,
		Avatar:        user.Avatar,
		Phone:         user.Phone,
		Email:         user.Email,
		LastLoginTime: user.LastLoginTime,
		LastLoginIP:   user.LastLoginIP,
		Roles:         []CurrentUserRole{},
	}
	if user.DeptID != nil {
		var dept CurrentUserDept
		err := r.db.WithContext(ctx).
			Table("sys_dept").
			Select("id, dept_name, dept_code").
			Where("id = ? AND deleted = ?", *user.DeptID, 0).
			Take(&dept).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find current user department: %w", err)
		}
		if err == nil {
			current.Dept = &dept
		}
	}

	err = r.db.WithContext(ctx).
		Table("sys_role AS r").
		Select("r.id, r.role_name, r.role_code").
		Joins("JOIN sys_user_role AS ur ON ur.role_id = r.id").
		Where("ur.user_id = ? AND r.status = ? AND r.deleted = ?", userID, UserStatusEnabled, 0).
		Order("r.id ASC").
		Scan(&current.Roles).Error
	if err != nil {
		return nil, fmt.Errorf("find current user roles: %w", err)
	}
	return current, nil
}

func (r *Repository) FindVisibleMenusByUserID(ctx context.Context, userID int64) ([]CurrentUserMenu, error) {
	menus := []CurrentUserMenu{}
	err := r.db.WithContext(ctx).
		Table("sys_menu AS m").
		Select(`DISTINCT m.id, m.parent_id, m.menu_name, m.menu_type, m.path,
            m.component, m.external_url, m.icon, m.permission_code, m.sort_order, m.visible`).
		Joins("JOIN sys_role_menu AS rm ON rm.menu_id = m.id").
		Joins("JOIN sys_user_role AS ur ON ur.role_id = rm.role_id").
		Joins("JOIN sys_role AS r ON r.id = ur.role_id").
		Where("ur.user_id = ? AND r.status = ? AND r.deleted = ? AND m.status = ? AND m.visible = ? AND m.deleted = ?", userID, UserStatusEnabled, 0, UserStatusEnabled, 1, 0).
		Order("m.sort_order ASC, m.id ASC").
		Scan(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("find visible menus: %w", err)
	}
	return menus, nil
}
