package dept

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Service struct {
	store Store
}

func NewService(s Store) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("dept store is required")
	}
	return &Service{s}, nil
}
func (s *Service) Tree(c context.Context) ([]Dept, error) { v, e := s.store.List(c); return tree(v), e }
func (s *Service) Options(c context.Context) ([]Dept, error) {
	v, e := s.store.List(c)
	if e != nil {
		return nil, e
	}
	out := v[:0]
	for _, x := range v {
		if x.Status == 1 {
			out = append(out, x)
		}
	}
	return tree(out), nil
}
func (s *Service) Page(c context.Context, q Query) (Page, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	return s.store.Page(c, q)
}
func (s *Service) Detail(c context.Context, id int64) (*Dept, error) { return s.store.Find(c, id) }
func (s *Service) Create(c context.Context, m AuditMetadata, in Input) (Dept, error) {
	if e := valid(in); e != nil {
		return Dept{}, e
	}
	if e := s.parent(c, 0, in.ParentID); e != nil {
		return Dept{}, e
	}
	x, e := s.store.CodeExists(c, in.DeptCode, 0)
	if e != nil {
		return Dept{}, e
	}
	if x {
		return Dept{}, ErrConflict
	}
	v, e := s.store.Create(c, from(in, 0), event(m, "dept.create", "dept", 0, "创建部门"))
	if e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) Update(c context.Context, m AuditMetadata, id int64, in Input) (Dept, error) {
	old, e := s.store.Find(c, id)
	if e != nil {
		return Dept{}, e
	}
	if old.IsBuiltin == 1 && old.DeptCode != in.DeptCode {
		return Dept{}, ErrBuiltin
	}
	if e = valid(in); e != nil {
		return Dept{}, e
	}
	if e = s.parent(c, id, in.ParentID); e != nil {
		return Dept{}, e
	}
	x, e := s.store.CodeExists(c, in.DeptCode, id)
	if e != nil {
		return Dept{}, e
	}
	if x {
		return Dept{}, ErrConflict
	}
	v, e := s.store.Update(c, from(in, id), event(m, "dept.update", "dept", id, "更新部门"))
	if e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) Delete(c context.Context, m AuditMetadata, id int64) error {
	if e := s.deleteCheck(c, id); e != nil {
		return e
	}
	if e := s.store.Delete(c, id, event(m, "dept.delete", "dept", id, "删除部门")); e != nil {
		return e
	}
	return nil
}
func (s *Service) DeleteBatch(c context.Context, m AuditMetadata, ids []int64) error {
	clean := unique(ids)
	for _, id := range clean {
		if e := s.deleteCheck(c, id); e != nil {
			return e
		}
	}
	return s.store.DeleteBatch(c, clean, event(m, "dept.delete", "dept", 0, "删除部门"))
}
func (s *Service) deleteCheck(c context.Context, id int64) error {
	v, e := s.store.Find(c, id)
	if e != nil {
		return e
	}
	if v.IsBuiltin == BuiltinYes {
		return ErrDeleteBuiltin
	}
	n, e := s.store.CountChildren(c, id)
	if e != nil {
		return e
	}
	if n > 0 {
		return ErrHasChildren
	}
	n, e = s.store.CountUsers(c, id)
	if e != nil {
		return e
	}
	if n > 0 {
		return ErrHasUsers
	}
	return nil
}
func (s *Service) SetStatus(c context.Context, m AuditMetadata, id int64, status int) error {
	if status != 0 && status != 1 {
		return ErrInvalid
	}
	if _, e := s.store.Find(c, id); e != nil {
		return e
	}
	if e := s.store.SetStatus(c, id, status, event(m, "dept.status", "dept", id, "更新部门状态")); e != nil {
		return e
	}
	return nil
}
func event(m AuditMetadata, a, r string, id int64, sum string) AuditEvent {
	return AuditEvent{Action: a, Resource: r, ResourceID: id, Summary: sum, Metadata: m}
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
func valid(v Input) error {
	if v.ParentID < 0 || strings.TrimSpace(v.DeptName) == "" || strings.TrimSpace(v.DeptCode) == "" || v.Status < 0 || v.Status > 1 {
		return ErrInvalid
	}
	return nil
}
func from(v Input, id int64) Dept {
	return Dept{ID: id, ParentID: v.ParentID, DeptName: v.DeptName, DeptCode: v.DeptCode, Leader: v.Leader, Phone: v.Phone, Email: v.Email, SortOrder: v.SortOrder, Status: v.Status, Remark: v.Remark}
}
func (s *Service) parent(c context.Context, id, parent int64) error {
	if parent == 0 {
		return nil
	}
	v, e := s.store.List(c)
	if e != nil {
		return e
	}
	p := map[int64]int64{}
	found := false
	for _, x := range v {
		p[x.ID] = x.ParentID
		if x.ID == parent {
			found = true
		}
	}
	if !found || id == parent {
		return ErrInvalid
	}
	for parent != 0 {
		if parent == id {
			return ErrInvalid
		}
		parent = p[parent]
	}
	return nil
}
func tree(v []Dept) []Dept {
	m := map[int64]*Dept{}
	for i := range v {
		v[i].Children = []Dept{}
		m[v[i].ID] = &v[i]
	}
	roots := []*Dept{}
	for _, x := range v {
		n := m[x.ID]
		p, ok := m[n.ParentID]
		if n.ParentID == 0 || !ok {
			roots = append(roots, n)
		} else {
			p.Children = append(p.Children, *n)
		}
	}
	var f func([]*Dept)
	f = func(x []*Dept) {
		sort.Slice(x, func(i, j int) bool {
			if x[i].SortOrder == x[j].SortOrder {
				return x[i].ID < x[j].ID
			}
			return x[i].SortOrder < x[j].SortOrder
		})
		for _, n := range x {
			c := make([]*Dept, len(n.Children))
			for i := range n.Children {
				c[i] = &n.Children[i]
			}
			f(c)
		}
	}
	f(roots)
	out := make([]Dept, len(roots))
	for i := range roots {
		out[i] = *roots[i]
	}
	return out
}
