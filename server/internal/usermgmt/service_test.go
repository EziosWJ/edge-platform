package usermgmt

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

type fake struct {
	u         User
	revoked   bool
	roles     []int64
	avatar    *string
	auditErr  error
	lastEvent AuditEvent
	auditN    int
}

func (f *fake) Page(context.Context, PageQuery) (Page[User], error) { return Page[User]{}, nil }
func (f *fake) Find(_ context.Context, id int64) (*User, error) {
	if f.u.ID == 0 {
		return nil, ErrNotFound
	}
	v := f.u
	return &v, nil
}
func (f *fake) UsernameExists(context.Context, string, int64) (bool, error) { return false, nil }
func (f *fake) DeptExists(context.Context, int64) (bool, error)             { return true, nil }
func (f *fake) RolesExist(context.Context, []int64) (bool, error)           { return true, nil }
func (f *fake) Create(_ context.Context, u User, e AuditEvent) (User, error) {
	if f.auditErr != nil {
		return u, f.auditErr
	}
	u.ID = 1
	f.u = u
	f.lastEvent = e
	f.lastEvent.ResourceID = u.ID
	f.auditN++
	return u, nil
}
func (f *fake) Update(_ context.Context, u User, revoke bool, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.u.Status = u.Status
	f.revoked = revoke
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) Delete(_ context.Context, _ int64, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.revoked = true
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) DeleteUsers(_ context.Context, _ []int64, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) AssignRoles(_ context.Context, _ int64, ids []int64, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.roles = ids
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) ResetPassword(_ context.Context, _ int64, p string, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.u.Password = p
	f.revoked = true
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) ChangePassword(_ context.Context, _ int64, p string, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.u.Password = p
	f.revoked = true
	f.lastEvent = e
	f.auditN++
	return nil
}
func (f *fake) UpdateAvatar(_ context.Context, _ int64, a *string, e AuditEvent) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.avatar = a
	f.lastEvent = e
	f.auditN++
	return nil
}

func TestDisableAndPasswordChangeRevokeAndAudit(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.MinCost)
	f := &fake{u: User{ID: 1, Username: "u", Nickname: "u", Password: string(h), Status: 1}}
	s, e := NewService(f, "default123")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetUserStatus(context.Background(), AuditMetadata{}, 1, 0); e != nil || !f.revoked {
		t.Fatalf("disable e=%v revoked=%v", e, f.revoked)
	}
	f.revoked = false
	if e = s.ChangeCurrentPassword(context.Background(), AuditMetadata{}, 1, ChangePasswordInput{"oldpass", "newpass"}); e != nil || !f.revoked {
		t.Fatalf("password e=%v revoked=%v", e, f.revoked)
	}
	if f.auditN != 2 {
		t.Fatalf("audit=%d", f.auditN)
	}
	if f.lastEvent.Action != "user.change_password" || f.lastEvent.Resource != "user" || f.lastEvent.ResourceID != 1 {
		t.Fatalf("event=%+v", f.lastEvent)
	}
}

func TestResetPasswordEventCarriesNoPassword(t *testing.T) {
	f := &fake{u: User{ID: 1, Username: "u", Nickname: "u", Status: 1}}
	s, _ := NewService(f, "default123")
	result, e := s.ResetUserPassword(context.Background(), AuditMetadata{ActorID: 1}, 1)
	if e != nil || result.Password != "default123" || !f.revoked {
		t.Fatalf("reset e=%v result=%+v revoked=%v", e, result, f.revoked)
	}
	if f.lastEvent.Summary != "重置用户密码" {
		t.Fatalf("summary=%q", f.lastEvent.Summary)
	}
}

func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.MinCost)
	base := User{ID: 1, Username: "u", Nickname: "u", Password: string(h), Status: 1}
	meta := AuditMetadata{ActorID: 1, RequestID: "r"}
	ctx := context.Background()

	t.Run("create", func(t *testing.T) {
		f := &fake{auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if e := s.CreateUser(ctx, meta, UserCreateInput{Username: "u", Nickname: "u", Status: 1}); e == nil {
			t.Fatal("create must fail when audit fails")
		}
		if f.u.ID != 0 {
			t.Fatal("user must roll back when audit fails")
		}
	})
	t.Run("update", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if e := s.SetUserStatus(ctx, meta, 1, 0); e == nil {
			t.Fatal("update must fail when audit fails")
		}
		if f.u.Status != 1 || f.revoked {
			t.Fatalf("status must roll back: status=%d revoked=%v", f.u.Status, f.revoked)
		}
	})
	t.Run("delete", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if e := s.DeleteUser(ctx, meta, 1); e == nil {
			t.Fatal("delete must fail when audit fails")
		}
		if f.revoked {
			t.Fatal("session revoke must roll back when audit fails")
		}
	})
	t.Run("roles", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if e := s.AssignUserRoles(ctx, meta, 1, []int64{3}); e == nil {
			t.Fatal("assign must fail when audit fails")
		}
		if f.roles != nil {
			t.Fatal("roles must roll back when audit fails")
		}
	})
	t.Run("reset password", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if _, e := s.ResetUserPassword(ctx, meta, 1); e == nil {
			t.Fatal("reset must fail when audit fails")
		}
		if f.u.Password != string(h) || f.revoked {
			t.Fatal("password and session must roll back when audit fails")
		}
	})
	t.Run("change password", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		if e := s.ChangeCurrentPassword(ctx, meta, 1, ChangePasswordInput{"oldpass", "newpass"}); e == nil {
			t.Fatal("change must fail when audit fails")
		}
		if f.u.Password != string(h) || f.revoked {
			t.Fatal("password and session must roll back when audit fails")
		}
	})
	t.Run("avatar", func(t *testing.T) {
		f := &fake{u: base, auditErr: errors.New("audit write failed")}
		s, _ := NewService(f, "default123")
		a := "x.png"
		if e := s.UpdateCurrentAvatar(ctx, meta, 1, &a); e == nil {
			t.Fatal("avatar must fail when audit fails")
		}
		if f.avatar != nil {
			t.Fatal("avatar must roll back when audit fails")
		}
	})
}

func TestBatchDeleteWritesOneAuditEvent(t *testing.T) {
	f := &fake{u: User{ID: 1, Username: "u", Nickname: "u", Status: 1}}
	s, _ := NewService(f, "default123")
	if e := s.DeleteUsers(context.Background(), AuditMetadata{}, []int64{1, 2}); e != nil {
		t.Fatalf("batch delete=%v", e)
	}
	if f.auditN != 1 {
		t.Fatalf("audit=%d", f.auditN)
	}
	if f.lastEvent.Action != "user.delete" || f.lastEvent.ResourceID != 0 {
		t.Fatalf("event=%+v", f.lastEvent)
	}
}
