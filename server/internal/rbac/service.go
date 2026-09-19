package rbac

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Service struct {
	store Store
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("rbac store is required")
	}
	return &Service{store}, nil
}
func (s *Service) RolePage(ctx context.Context, q RolePageQuery) (Page[Role], error) {
	q.Page, q.PageSize = page(q.Page, q.PageSize)
	return s.store.PageRoles(ctx, q)
}
func (s *Service) RoleDetail(ctx context.Context, id int64) (*RoleDetail, error) {
	v, e := s.store.FindRole(ctx, id)
	if e != nil {
		return nil, e
	}
	ids, e := s.store.RoleMenuIDs(ctx, id)
	if e != nil {
		return nil, e
	}
	return &RoleDetail{Role: *v, MenuIDs: ids}, nil
}
func (s *Service) RoleOptions(ctx context.Context) ([]Role, error) { return s.store.EnabledRoles(ctx) }
func (s *Service) CreateRole(ctx context.Context, m AuditMetadata, in RoleInput) (Role, error) {
	if e := validRole(in); e != nil {
		return Role{}, e
	}
	exists, e := s.store.RoleCodeExists(ctx, in.RoleCode, 0)
	if e != nil {
		return Role{}, e
	}
	if exists {
		return Role{}, ErrConflict
	}
	v, e := s.store.CreateRole(ctx, Role{RoleName: in.RoleName, RoleCode: in.RoleCode, Status: in.Status, SortOrder: in.SortOrder, Remark: in.Remark, IsBuiltin: BuiltinNo}, event(m, "role.create", "role", 0, "创建角色"))
	if e != nil {
		return Role{}, e
	}
	return v, nil
}
func (s *Service) UpdateRole(ctx context.Context, m AuditMetadata, id int64, in RoleInput) (Role, error) {
	old, e := s.store.FindRole(ctx, id)
	if e != nil {
		return Role{}, e
	}
	if old.IsBuiltin == BuiltinYes && old.RoleCode != in.RoleCode {
		return Role{}, ErrBuiltinProtected
	}
	if e = validRole(in); e != nil {
		return Role{}, e
	}
	exists, e := s.store.RoleCodeExists(ctx, in.RoleCode, id)
	if e != nil {
		return Role{}, e
	}
	if exists {
		return Role{}, ErrConflict
	}
	v, e := s.store.UpdateRole(ctx, Role{ID: id, RoleName: in.RoleName, RoleCode: in.RoleCode, Status: in.Status, SortOrder: in.SortOrder, Remark: in.Remark, IsBuiltin: old.IsBuiltin}, event(m, "role.update", "role", id, "更新角色"))
	if e != nil {
		return Role{}, e
	}
	return v, nil
}
func (s *Service) DeleteRole(ctx context.Context, m AuditMetadata, id int64) error {
	v, e := s.store.FindRole(ctx, id)
	if e != nil {
		return e
	}
	if v.IsBuiltin == BuiltinYes {
		return ErrBuiltinProtected
	}
	n, e := s.store.CountUsersByRole(ctx, id)
	if e != nil {
		return e
	}
	if n > 0 {
		return fmt.Errorf("角色已绑定用户: %w", ErrConflict)
	}
	if e = s.store.DeleteRole(ctx, id, event(m, "role.delete", "role", id, "删除角色")); e != nil {
		return e
	}
	return nil
}
func (s *Service) DeleteRoles(ctx context.Context, m AuditMetadata, ids []int64) error {
	clean := unique(ids)
	for _, id := range clean {
		v, e := s.store.FindRole(ctx, id)
		if e != nil {
			return e
		}
		if v.IsBuiltin == BuiltinYes {
			return ErrBuiltinProtected
		}
		n, e := s.store.CountUsersByRole(ctx, id)
		if e != nil {
			return e
		}
		if n > 0 {
			return fmt.Errorf("角色已绑定用户: %w", ErrConflict)
		}
	}
	return s.store.DeleteRoles(ctx, clean, event(m, "role.delete", "role", 0, "删除角色"))
}
func (s *Service) SetRoleStatus(ctx context.Context, m AuditMetadata, id int64, status int) error {
	if !validStatus(status) {
		return ErrInvalidInput
	}
	if _, e := s.store.FindRole(ctx, id); e != nil {
		return e
	}
	if e := s.store.SetRoleStatus(ctx, id, status, event(m, "role.status", "role", id, "更新角色状态")); e != nil {
		return e
	}
	return nil
}
func (s *Service) AssignRoleMenus(ctx context.Context, m AuditMetadata, id int64, ids []int64) error {
	if _, e := s.store.FindRole(ctx, id); e != nil {
		return e
	}
	if e := s.store.ReplaceRoleMenus(ctx, id, unique(ids), event(m, "role.menus", "role", id, "分配角色菜单")); e != nil {
		return e
	}
	return nil
}
func (s *Service) MenuTree(ctx context.Context) ([]Menu, error) {
	v, e := s.store.ListMenus(ctx)
	if e != nil {
		return nil, e
	}
	return tree(v), nil
}
func (s *Service) MenuPage(ctx context.Context, q MenuPageQuery) (Page[Menu], error) {
	q.Page, q.PageSize = page(q.Page, q.PageSize)
	return s.store.PageMenus(ctx, q)
}
func (s *Service) MenuDetail(ctx context.Context, id int64) (*Menu, error) {
	return s.store.FindMenu(ctx, id)
}
func (s *Service) CreateMenu(ctx context.Context, m AuditMetadata, in MenuInput) (Menu, error) {
	if e := validMenu(in); e != nil {
		return Menu{}, e
	}
	if e := s.parent(ctx, 0, in.ParentID); e != nil {
		return Menu{}, e
	}
	code := ""
	if in.PermissionCode != nil {
		code = *in.PermissionCode
	}
	x, e := s.store.PermissionCodeExists(ctx, code, 0)
	if e != nil {
		return Menu{}, e
	}
	if x {
		return Menu{}, ErrConflict
	}
	v, e := s.store.CreateMenu(ctx, menuFrom(in, 0), event(m, "menu.create", "menu", 0, "创建菜单"))
	if e != nil {
		return Menu{}, e
	}
	return v, nil
}
func (s *Service) UpdateMenu(ctx context.Context, m AuditMetadata, id int64, in MenuInput) (Menu, error) {
	old, e := s.store.FindMenu(ctx, id)
	if e != nil {
		return Menu{}, e
	}
	if e = validMenu(in); e != nil {
		return Menu{}, e
	}
	if e = s.parent(ctx, id, in.ParentID); e != nil {
		return Menu{}, e
	}
	code := ""
	if in.PermissionCode != nil {
		code = *in.PermissionCode
	}
	if old.IsBuiltin == BuiltinYes && deref(old.PermissionCode) != code {
		return Menu{}, ErrBuiltinProtected
	}
	x, e := s.store.PermissionCodeExists(ctx, code, id)
	if e != nil {
		return Menu{}, e
	}
	if x {
		return Menu{}, ErrConflict
	}
	v, e := s.store.UpdateMenu(ctx, menuFrom(in, id), event(m, "menu.update", "menu", id, "更新菜单"))
	if e != nil {
		return Menu{}, e
	}
	return v, nil
}
func (s *Service) DeleteMenu(ctx context.Context, m AuditMetadata, id int64) error {
	v, e := s.store.FindMenu(ctx, id)
	if e != nil {
		return e
	}
	if v.IsBuiltin == BuiltinYes {
		return ErrBuiltinProtected
	}
	n, e := s.store.CountChildren(ctx, id)
	if e != nil {
		return e
	}
	if n > 0 {
		return fmt.Errorf("存在子菜单: %w", ErrConflict)
	}
	n, e = s.store.CountRolesByMenu(ctx, id)
	if e != nil {
		return e
	}
	if n > 0 {
		return fmt.Errorf("菜单已绑定角色: %w", ErrConflict)
	}
	if e = s.store.DeleteMenu(ctx, id, event(m, "menu.delete", "menu", id, "删除菜单")); e != nil {
		return e
	}
	return nil
}
func (s *Service) DeleteMenus(ctx context.Context, m AuditMetadata, ids []int64) error {
	clean := unique(ids)
	for _, id := range clean {
		v, e := s.store.FindMenu(ctx, id)
		if e != nil {
			return e
		}
		if v.IsBuiltin == BuiltinYes {
			return ErrBuiltinProtected
		}
		n, e := s.store.CountChildren(ctx, id)
		if e != nil {
			return e
		}
		if n > 0 {
			return fmt.Errorf("存在子菜单: %w", ErrConflict)
		}
		n, e = s.store.CountRolesByMenu(ctx, id)
		if e != nil {
			return e
		}
		if n > 0 {
			return fmt.Errorf("菜单已绑定角色: %w", ErrConflict)
		}
	}
	return s.store.DeleteMenus(ctx, clean, event(m, "menu.delete", "menu", 0, "删除菜单"))
}
func (s *Service) SetMenuStatus(ctx context.Context, m AuditMetadata, id int64, status int) error {
	if !validStatus(status) {
		return ErrInvalidInput
	}
	if _, e := s.store.FindMenu(ctx, id); e != nil {
		return e
	}
	if e := s.store.SetMenuStatus(ctx, id, status, event(m, "menu.status", "menu", id, "更新菜单状态")); e != nil {
		return e
	}
	return nil
}
func event(m AuditMetadata, a, r string, id int64, sum string) AuditEvent {
	return AuditEvent{Action: a, Resource: r, ResourceID: id, Summary: sum, Metadata: m}
}
func validRole(v RoleInput) error {
	if strings.TrimSpace(v.RoleName) == "" || strings.TrimSpace(v.RoleCode) == "" || !validStatus(v.Status) {
		return ErrInvalidInput
	}
	return nil
}
func validMenu(v MenuInput) error {
	if v.ParentID < 0 || strings.TrimSpace(v.MenuName) == "" || !oneOf(v.MenuType, "DIR", "MENU", "LINK") || !validStatus(v.Status) || !validStatus(v.Visible) {
		return ErrInvalidInput
	}
	return nil
}
func validStatus(v int) bool { return v == 0 || v == 1 }
func oneOf(v string, vs ...string) bool {
	for _, x := range vs {
		if v == x {
			return true
		}
	}
	return false
}
func page(p, s int) (int, int) {
	if p < 1 {
		p = 1
	}
	if s < 1 {
		s = 10
	}
	if s > 500 {
		s = 500
	}
	return p, s
}
func unique(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func menuFrom(in MenuInput, id int64) Menu {
	return Menu{ID: id, ParentID: in.ParentID, MenuName: in.MenuName, MenuType: in.MenuType, Path: in.Path, Component: in.Component, ExternalURL: in.ExternalURL, Icon: in.Icon, PermissionCode: in.PermissionCode, SortOrder: in.SortOrder, Visible: in.Visible, Status: in.Status, Remark: in.Remark}
}
func (s *Service) parent(ctx context.Context, id, parent int64) error {
	if parent == 0 {
		return nil
	}
	all, e := s.store.ListMenus(ctx)
	if e != nil {
		return e
	}
	parents := map[int64]int64{}
	found := false
	for _, v := range all {
		parents[v.ID] = v.ParentID
		if v.ID == parent {
			found = true
		}
	}
	if !found || parent == id {
		return ErrInvalidInput
	}
	for parent != 0 {
		if parent == id {
			return ErrInvalidInput
		}
		parent = parents[parent]
	}
	return nil
}
func tree(in []Menu) []Menu {
	type n struct {
		m Menu
		c []*n
	}
	by := map[int64]*n{}
	for _, m := range in {
		m.Children = []Menu{}
		by[m.ID] = &n{m: m}
	}
	roots := []*n{}
	for _, m := range in {
		x := by[m.ID]
		p, ok := by[x.m.ParentID]
		if x.m.ParentID == 0 || !ok {
			roots = append(roots, x)
		} else {
			p.c = append(p.c, x)
		}
	}
	var sortN func([]*n)
	sortN = func(v []*n) {
		sort.Slice(v, func(i, j int) bool {
			if v[i].m.SortOrder == v[j].m.SortOrder {
				return v[i].m.ID < v[j].m.ID
			}
			return v[i].m.SortOrder < v[j].m.SortOrder
		})
		for _, x := range v {
			sortN(x.c)
		}
	}
	var out func(*n) Menu
	out = func(x *n) Menu {
		m := x.m
		m.Children = make([]Menu, len(x.c))
		for i, c := range x.c {
			m.Children[i] = out(c)
		}
		return m
	}
	sortN(roots)
	o := make([]Menu, len(roots))
	for i, x := range roots {
		o[i] = out(x)
	}
	return o
}
