package usermgmt

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"strings"
)

type Service struct {
	store           Store
	defaultPassword string
}

func NewService(store Store, defaultPassword string) (*Service, error) {
	if store == nil || defaultPassword == "" {
		return nil, ErrInvalid
	}
	return &Service{store, defaultPassword}, nil
}
func (s *Service) UserPage(ctx context.Context, q UserPageQuery) (Page[User], error) {
	q.Page, q.PageSize = page(q.Page, q.PageSize)
	return s.store.Page(ctx, q)
}
func (s *Service) UserDetail(ctx context.Context, id int64) (*UserDetail, error) {
	u, e := s.store.Find(ctx, id)
	if e != nil {
		return nil, e
	}
	return &UserDetail{*u}, nil
}
func (s *Service) CreateUser(ctx context.Context, m AuditMetadata, in UserCreateInput) error {
	if e := s.valid(ctx, in, 0, true); e != nil {
		return e
	}
	h, e := bcrypt.GenerateFromPassword([]byte(s.defaultPassword), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	if _, e := s.store.Create(ctx, User{Username: in.Username, Nickname: in.Nickname, Password: string(h), Phone: in.Phone, Email: in.Email, Avatar: in.Avatar, Gender: gender(in.Gender), DeptID: in.DeptID, Status: in.Status, Remark: in.Remark}, event(m, "user.create", "user", 0, "创建用户")); e != nil {
		return e
	}
	return nil
}
func (s *Service) UpdateUser(ctx context.Context, m AuditMetadata, id int64, in UserUpdateInput) error {
	old, e := s.store.Find(ctx, id)
	if e != nil {
		return e
	}
	if e = s.valid(ctx, in, id, false); e != nil {
		return e
	}
	u := User{ID: id, Nickname: in.Nickname, Phone: in.Phone, Email: in.Email, Avatar: in.Avatar, Gender: gender(in.Gender), DeptID: in.DeptID, Status: in.Status, Remark: in.Remark}
	if e = s.store.Update(ctx, u, old.Status == 1 && in.Status == 0, event(m, "user.update", "user", id, "修改用户")); e != nil {
		return e
	}
	return nil
}
func (s *Service) DeleteUser(ctx context.Context, m AuditMetadata, id int64) error {
	u, e := s.store.Find(ctx, id)
	if e != nil {
		return e
	}
	if u.IsBuiltin == 1 {
		return ErrBuiltin
	}
	if e = s.store.Delete(ctx, id, event(m, "user.delete", "user", id, "删除用户")); e != nil {
		return e
	}
	return nil
}
func (s *Service) DeleteUsers(ctx context.Context, m AuditMetadata, ids []int64) error {
	clean := unique(ids)
	for _, id := range clean {
		u, e := s.store.Find(ctx, id)
		if e != nil {
			return e
		}
		if u.IsBuiltin == 1 {
			return ErrBuiltin
		}
	}
	return s.store.DeleteUsers(ctx, clean, event(m, "user.delete", "user", 0, "删除用户"))
}
func (s *Service) SetUserStatus(ctx context.Context, m AuditMetadata, id int64, status int) error {
	u, e := s.store.Find(ctx, id)
	if e != nil {
		return e
	}
	if status < 0 || status > 1 {
		return ErrInvalid
	}
	u.Status = status
	if e = s.store.Update(ctx, *u, status == 0, event(m, "user.status", "user", id, "修改用户状态")); e != nil {
		return e
	}
	return nil
}
func (s *Service) AssignUserRoles(ctx context.Context, m AuditMetadata, id int64, ids []int64) error {
	if _, e := s.store.Find(ctx, id); e != nil {
		return e
	}
	ids = unique(ids)
	ok, e := s.store.RolesExist(ctx, ids)
	if e != nil {
		return e
	}
	if !ok {
		return ErrNotFound
	}
	if e = s.store.AssignRoles(ctx, id, ids, event(m, "user.roles", "user", id, "分配用户角色")); e != nil {
		return e
	}
	return nil
}
func (s *Service) ResetUserPassword(ctx context.Context, m AuditMetadata, id int64) (ResetPasswordResult, error) {
	if _, e := s.store.Find(ctx, id); e != nil {
		return ResetPasswordResult{}, e
	}
	h, e := bcrypt.GenerateFromPassword([]byte(s.defaultPassword), bcrypt.DefaultCost)
	if e != nil {
		return ResetPasswordResult{}, e
	}
	if e = s.store.ResetPassword(ctx, id, string(h), event(m, "user.reset_password", "user", id, "重置用户密码")); e != nil {
		return ResetPasswordResult{}, e
	}
	return ResetPasswordResult{s.defaultPassword}, nil
}
func (s *Service) ChangeCurrentPassword(ctx context.Context, m AuditMetadata, id int64, in ChangePasswordInput) error {
	if in.OldPassword == "" || len(in.NewPassword) < 6 || len(in.NewPassword) > 50 {
		return ErrInvalid
	}
	u, e := s.store.Find(ctx, id)
	if e != nil {
		return e
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.OldPassword)) != nil {
		return ErrOldPassword
	}
	h, e := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	if e = s.store.ChangePassword(ctx, id, string(h), event(m, "user.change_password", "user", id, "修改密码")); e != nil {
		return e
	}
	return nil
}
func (s *Service) UpdateCurrentAvatar(ctx context.Context, m AuditMetadata, id int64, a *string) error {
	if _, e := s.store.Find(ctx, id); e != nil {
		return e
	}
	if e := s.store.UpdateAvatar(ctx, id, a, event(m, "user.avatar", "user", id, "修改用户头像")); e != nil {
		return e
	}
	return nil
}
func (s *Service) valid(ctx context.Context, in Input, id int64, create bool) error {
	if strings.TrimSpace(in.Nickname) == "" || in.Status < 0 || in.Status > 1 || (create && strings.TrimSpace(in.Username) == "") {
		return ErrInvalid
	}
	if create {
		v, e := s.store.UsernameExists(ctx, in.Username, id)
		if e != nil {
			return e
		}
		if v {
			return ErrConflict
		}
	}
	if in.DeptID != nil {
		v, e := s.store.DeptExists(ctx, *in.DeptID)
		if e != nil {
			return e
		}
		if !v {
			return ErrNotFound
		}
	}
	return nil
}
func event(m AuditMetadata, a, r string, id int64, sum string) AuditEvent {
	return AuditEvent{Action: a, Resource: r, ResourceID: id, Summary: sum, Metadata: m}
}
func gender(v string) string {
	if v == "" {
		return "UNSPECIFIED"
	}
	return v
}
func page(p, z int) (int, int) {
	if p < 1 {
		p = 1
	}
	if z < 1 {
		z = 10
	}
	return p, z
}
func unique(a []int64) []int64 {
	m := map[int64]bool{}
	o := []int64{}
	for _, v := range a {
		if v > 0 && !m[v] {
			m[v] = true
			o = append(o, v)
		}
	}
	return o
}
