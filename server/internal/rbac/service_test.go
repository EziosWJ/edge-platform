package rbac

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	roles       map[int64]Role
	menus       map[int64]Menu
	roleMenus   map[int64][]int64
	usersByRole map[int64]int64
	auditError  error
	lastEvent   AuditEvent
}

func newMemoryStore() *memoryStore {
	return &memoryStore{roles: map[int64]Role{}, menus: map[int64]Menu{}, roleMenus: map[int64][]int64{}, usersByRole: map[int64]int64{}}
}
func (s *memoryStore) PageRoles(context.Context, RolePageQuery) (Page[Role], error) {
	return Page[Role]{}, nil
}
func (s *memoryStore) FindRole(_ context.Context, id int64) (*Role, error) {
	v, ok := s.roles[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (s *memoryStore) RoleCodeExists(_ context.Context, c string, x int64) (bool, error) {
	for id, v := range s.roles {
		if id != x && v.RoleCode == c {
			return true, nil
		}
	}
	return false, nil
}
func (s *memoryStore) CreateRole(_ context.Context, v Role, e AuditEvent) (Role, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	v.ID = int64(len(s.roles) + 1)
	s.roles[v.ID] = v
	s.lastEvent = e
	s.lastEvent.ResourceID = v.ID
	return v, nil
}
func (s *memoryStore) UpdateRole(_ context.Context, v Role, e AuditEvent) (Role, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	old := s.roles[v.ID]
	v.IsBuiltin = old.IsBuiltin
	s.roles[v.ID] = v
	return v, nil
}
func (s *memoryStore) CountUsersByRole(_ context.Context, id int64) (int64, error) {
	return s.usersByRole[id], nil
}
func (s *memoryStore) DeleteRole(_ context.Context, id int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	delete(s.roles, id)
	delete(s.roleMenus, id)
	return nil
}
func (s *memoryStore) DeleteRoles(_ context.Context, ids []int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	for _, id := range ids {
		delete(s.roles, id)
		delete(s.roleMenus, id)
	}
	return nil
}
func (s *memoryStore) SetRoleStatus(_ context.Context, id int64, status int, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	v := s.roles[id]
	v.Status = status
	s.roles[id] = v
	return nil
}
func (s *memoryStore) RoleMenuIDs(_ context.Context, id int64) ([]int64, error) {
	return s.roleMenus[id], nil
}
func (s *memoryStore) ReplaceRoleMenus(_ context.Context, id int64, ids []int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	s.roleMenus[id] = ids
	return nil
}
func (s *memoryStore) EnabledRoles(context.Context) ([]Role, error) { return nil, nil }
func (s *memoryStore) ListMenus(context.Context) ([]Menu, error) {
	v := []Menu{}
	for _, m := range s.menus {
		v = append(v, m)
	}
	return v, nil
}
func (s *memoryStore) PageMenus(context.Context, MenuPageQuery) (Page[Menu], error) {
	return Page[Menu]{}, nil
}
func (s *memoryStore) FindMenu(_ context.Context, id int64) (*Menu, error) {
	v, ok := s.menus[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (s *memoryStore) PermissionCodeExists(_ context.Context, c string, x int64) (bool, error) {
	for id, v := range s.menus {
		if id != x && deref(v.PermissionCode) == c && c != "" {
			return true, nil
		}
	}
	return false, nil
}
func (s *memoryStore) CreateMenu(_ context.Context, v Menu, e AuditEvent) (Menu, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	v.ID = int64(len(s.menus) + 1)
	s.menus[v.ID] = v
	return v, nil
}
func (s *memoryStore) UpdateMenu(_ context.Context, v Menu, e AuditEvent) (Menu, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	old := s.menus[v.ID]
	v.IsBuiltin = old.IsBuiltin
	s.menus[v.ID] = v
	return v, nil
}
func (s *memoryStore) CountChildren(_ context.Context, id int64) (int64, error) {
	var n int64
	for _, m := range s.menus {
		if m.ParentID == id {
			n++
		}
	}
	return n, nil
}
func (s *memoryStore) CountRolesByMenu(context.Context, int64) (int64, error) { return 0, nil }
func (s *memoryStore) DeleteMenu(_ context.Context, id int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	delete(s.menus, id)
	return nil
}
func (s *memoryStore) DeleteMenus(_ context.Context, ids []int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	for _, id := range ids {
		delete(s.menus, id)
	}
	return nil
}
func (s *memoryStore) SetMenuStatus(_ context.Context, id int64, status int, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	v := s.menus[id]
	v.Status = status
	s.menus[id] = v
	return nil
}

func TestBuiltinCodeProtectedButStatusMayChange(t *testing.T) {
	store := newMemoryStore()
	store.roles[1] = Role{ID: 1, RoleName: "管理员", RoleCode: "ADMIN", Status: 1, IsBuiltin: BuiltinYes}
	s, _ := NewService(store)
	if err := s.SetRoleStatus(context.Background(), AuditMetadata{ActorID: 1, RequestID: "r"}, 1, StatusDisabled); err != nil {
		t.Fatalf("builtin role status should follow Java contract: %v", err)
	}
	if store.roles[1].Status != 0 {
		t.Fatal("status missing")
	}
	_, err := s.UpdateRole(context.Background(), AuditMetadata{}, 1, RoleInput{RoleName: "管理员", RoleCode: "ROOT", Status: 1})
	if !errors.Is(err, ErrBuiltinProtected) {
		t.Fatalf("builtin code update error=%v", err)
	}
}
func TestRoleDetailAndDeleteProtection(t *testing.T) {
	store := newMemoryStore()
	store.roles[2] = Role{ID: 2, RoleName: "运营", RoleCode: "OPS", Status: 1}
	store.roleMenus[2] = []int64{3, 4}
	store.usersByRole[2] = 1
	s, _ := NewService(store)
	detail, err := s.RoleDetail(context.Background(), 2)
	if err != nil || len(detail.MenuIDs) != 2 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if err = s.DeleteRole(context.Background(), AuditMetadata{}, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("bound role delete=%v", err)
	}
}
func TestMenuTreeSortAndParentValidation(t *testing.T) {
	store := newMemoryStore()
	store.menus[1] = Menu{ID: 1, ParentID: 0, MenuName: "root", MenuType: "DIR", SortOrder: 2}
	store.menus[2] = Menu{ID: 2, ParentID: 1, MenuName: "child", MenuType: "MENU", SortOrder: 2}
	store.menus[3] = Menu{ID: 3, ParentID: 1, MenuName: "first", MenuType: "MENU", SortOrder: 1}
	s, _ := NewService(store)
	tree, err := s.MenuTree(context.Background())
	if err != nil || len(tree) != 1 || tree[0].Children[0].ID != 3 {
		t.Fatalf("tree=%+v err=%v", tree, err)
	}
	_, err = s.UpdateMenu(context.Background(), AuditMetadata{}, 1, MenuInput{ParentID: 2, MenuName: "root", MenuType: "DIR", Status: 1, Visible: 1})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cycle error=%v", err)
	}
}
func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	store := newMemoryStore()
	store.auditError = errors.New("audit write failed")
	s, _ := NewService(store)
	_, err := s.CreateRole(context.Background(), AuditMetadata{}, RoleInput{RoleName: "运营", RoleCode: "OPS", Status: 1})
	if err == nil {
		t.Fatal("create must fail when audit fails")
	}
	if _, ok := store.roles[1]; ok {
		t.Fatal("role must roll back when audit fails")
	}
}
func TestBatchDeleteWritesOneAuditEvent(t *testing.T) {
	store := newMemoryStore()
	store.roles[2] = Role{ID: 2, RoleName: "运营", RoleCode: "OPS", Status: 1}
	store.roles[3] = Role{ID: 3, RoleName: "测试", RoleCode: "QA", Status: 1}
	s, _ := NewService(store)
	if err := s.DeleteRoles(context.Background(), AuditMetadata{}, []int64{2, 3}); err != nil {
		t.Fatalf("batch delete=%v", err)
	}
	if _, ok := store.roles[2]; ok {
		t.Fatal("role 2 must be deleted")
	}
	if _, ok := store.roles[3]; ok {
		t.Fatal("role 3 must be deleted")
	}
}
