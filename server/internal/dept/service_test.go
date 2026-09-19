package dept

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	depts       map[int64]Dept
	usersByDept map[int64]int64
	auditError  error
	auditCount  int
	lastEvent   AuditEvent
}

func newMemoryStore() *memoryStore {
	return &memoryStore{depts: map[int64]Dept{}, usersByDept: map[int64]int64{}}
}
func (s *memoryStore) List(context.Context) ([]Dept, error) {
	v := []Dept{}
	for _, d := range s.depts {
		v = append(v, d)
	}
	return v, nil
}
func (s *memoryStore) Page(context.Context, Query) (Page, error) { return Page{}, nil }
func (s *memoryStore) Find(_ context.Context, id int64) (*Dept, error) {
	v, ok := s.depts[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (s *memoryStore) CodeExists(_ context.Context, c string, x int64) (bool, error) {
	for id, v := range s.depts {
		if id != x && v.DeptCode == c {
			return true, nil
		}
	}
	return false, nil
}
func (s *memoryStore) Create(_ context.Context, v Dept, e AuditEvent) (Dept, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	v.ID = int64(len(s.depts) + 1)
	s.depts[v.ID] = v
	s.lastEvent = e
	s.lastEvent.ResourceID = v.ID
	s.auditCount++
	return v, nil
}
func (s *memoryStore) Update(_ context.Context, v Dept, e AuditEvent) (Dept, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	s.depts[v.ID] = v
	s.lastEvent = e
	s.auditCount++
	return v, nil
}
func (s *memoryStore) Delete(_ context.Context, id int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	delete(s.depts, id)
	s.lastEvent = e
	s.auditCount++
	return nil
}
func (s *memoryStore) DeleteBatch(_ context.Context, ids []int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	for _, id := range ids {
		delete(s.depts, id)
	}
	s.lastEvent = e
	s.auditCount++
	return nil
}
func (s *memoryStore) SetStatus(_ context.Context, id int64, status int, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	v := s.depts[id]
	v.Status = status
	s.depts[id] = v
	s.lastEvent = e
	s.auditCount++
	return nil
}
func (s *memoryStore) CountChildren(_ context.Context, id int64) (int64, error) {
	var n int64
	for _, d := range s.depts {
		if d.ParentID == id {
			n++
		}
	}
	return n, nil
}
func (s *memoryStore) CountUsers(_ context.Context, id int64) (int64, error) {
	return s.usersByDept[id], nil
}

func TestValidInputRejectsMissingRequiredFields(t *testing.T) {
	if err := valid(Input{ParentID: 0, Status: StatusEnabled}); err == nil {
		t.Fatal("missing department fields were accepted")
	}
	if err := valid(Input{ParentID: 0, DeptName: "研发", DeptCode: "RND", Status: StatusEnabled}); err != nil {
		t.Fatalf("valid department input rejected: %v", err)
	}
}

func TestTreeSortsRootsAndChildren(t *testing.T) {
	result := tree([]Dept{{ID: 3, ParentID: 1, SortOrder: 1}, {ID: 1, ParentID: 0, SortOrder: 2}, {ID: 2, ParentID: 0, SortOrder: 1}})
	if len(result) != 2 || result[0].ID != 2 || result[1].ID != 1 || len(result[1].Children) != 1 || result[1].Children[0].ID != 3 {
		t.Fatalf("tree = %+v", result)
	}
}

func TestDeleteProtections(t *testing.T) {
	store := newMemoryStore()
	store.depts[1] = Dept{ID: 1, DeptName: "总公司", DeptCode: "HQ", IsBuiltin: BuiltinYes}
	store.depts[2] = Dept{ID: 2, DeptName: "研发部", DeptCode: "RND"}
	store.depts[4] = Dept{ID: 4, DeptName: "平台组", DeptCode: "PLAT", ParentID: 2}
	store.depts[3] = Dept{ID: 3, DeptName: "测试部", DeptCode: "QA"}
	store.usersByDept[3] = 1
	s, _ := NewService(store)
	if err := s.Delete(context.Background(), AuditMetadata{}, 1); !errors.Is(err, ErrDeleteBuiltin) {
		t.Fatalf("builtin delete=%v", err)
	}
	if err := s.Delete(context.Background(), AuditMetadata{}, 2); !errors.Is(err, ErrHasChildren) {
		t.Fatalf("parent delete=%v", err)
	}
	if err := s.Delete(context.Background(), AuditMetadata{}, 3); !errors.Is(err, ErrHasUsers) {
		t.Fatalf("bound delete=%v", err)
	}
}

func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	store := newMemoryStore()
	store.auditError = errors.New("audit write failed")
	s, _ := NewService(store)
	_, err := s.Create(context.Background(), AuditMetadata{ActorID: 1}, Input{ParentID: 0, DeptName: "研发部", DeptCode: "RND", Status: StatusEnabled})
	if err == nil {
		t.Fatal("create must fail when audit fails")
	}
	if len(store.depts) != 0 {
		t.Fatal("dept must roll back when audit fails")
	}
}

func TestBatchDeleteWritesOneAuditEvent(t *testing.T) {
	store := newMemoryStore()
	store.depts[2] = Dept{ID: 2, DeptName: "研发部", DeptCode: "RND"}
	store.depts[3] = Dept{ID: 3, DeptName: "测试部", DeptCode: "QA"}
	s, _ := NewService(store)
	if err := s.DeleteBatch(context.Background(), AuditMetadata{}, []int64{2, 3}); err != nil {
		t.Fatalf("batch delete=%v", err)
	}
	if store.auditCount != 1 {
		t.Fatalf("audit events = %d, want 1", store.auditCount)
	}
	if _, ok := store.depts[2]; ok {
		t.Fatal("dept 2 must be deleted")
	}
	if _, ok := store.depts[3]; ok {
		t.Fatal("dept 3 must be deleted")
	}
}
