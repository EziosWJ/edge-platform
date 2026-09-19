package sysconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

type memoryStore struct {
	configs     map[int64]Config
	auditError  error
	auditEvents []audit.Event
	lastID      int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{configs: map[int64]Config{}}
}
func (s *memoryStore) Page(context.Context, Query) (Page, error) {
	return Page{}, nil
}
func (s *memoryStore) Find(_ context.Context, id int64) (*Config, error) {
	v, ok := s.configs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (s *memoryStore) ByKey(context.Context, string) (*ByKey, error) { return nil, nil }
func (s *memoryStore) KeyExists(_ context.Context, k string, x int64) (bool, error) {
	for id, v := range s.configs {
		if id != x && v.ConfigKey == k {
			return true, nil
		}
	}
	return false, nil
}
func (s *memoryStore) Create(_ context.Context, v Config, e audit.Event) (Config, error) {
	if s.auditError != nil {
		return v, s.auditError
	}
	s.lastID++
	v.ID = s.lastID
	s.configs[v.ID] = v
	e.ResourceID = v.ID
	s.auditEvents = append(s.auditEvents, e)
	return v, nil
}
func (s *memoryStore) Update(_ context.Context, v Config, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	s.configs[v.ID] = v
	s.auditEvents = append(s.auditEvents, e)
	return nil
}
func (s *memoryStore) Delete(_ context.Context, id int64, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	delete(s.configs, id)
	s.auditEvents = append(s.auditEvents, e)
	return nil
}
func (s *memoryStore) DeleteBatch(_ context.Context, ids []int64, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	for _, id := range ids {
		delete(s.configs, id)
	}
	s.auditEvents = append(s.auditEvents, e)
	return nil
}
func (s *memoryStore) SetStatus(_ context.Context, id int64, status int, e audit.Event) error {
	if s.auditError != nil {
		return s.auditError
	}
	v := s.configs[id]
	v.Status = status
	s.configs[id] = v
	s.auditEvents = append(s.auditEvents, e)
	return nil
}

func TestCreateAndUpdateWithAudit(t *testing.T) {
	store := newMemoryStore()
	s := NewService(store)
	if err := s.Create(context.Background(), audit.Metadata{}, Input{ConfigName: "演示", ConfigKey: "demo.enabled", ConfigValue: "true"}); err != nil {
		t.Fatalf("create=%v", err)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].Action != "config.create" || store.auditEvents[0].ResourceID == 0 {
		t.Fatalf("create audit events=%+v", store.auditEvents)
	}
	if err := s.Update(context.Background(), audit.Metadata{}, 1, Input{ConfigName: "演示", ConfigKey: "demo.enabled", ConfigValue: "false"}); err != nil {
		t.Fatalf("update=%v", err)
	}
	if len(store.auditEvents) != 2 || store.auditEvents[1].Action != "config.update" || store.auditEvents[1].ResourceID != 1 {
		t.Fatalf("update audit events=%+v", store.auditEvents)
	}
	if store.configs[1].ConfigValue != "false" {
		t.Fatalf("config value missing: %+v", store.configs[1])
	}
}
func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	store := newMemoryStore()
	store.auditError = errors.New("audit write failed")
	s := NewService(store)
	if err := s.Create(context.Background(), audit.Metadata{}, Input{ConfigName: "演示", ConfigKey: "demo.enabled", ConfigValue: "true"}); err == nil {
		t.Fatal("create must fail when audit fails")
	}
	if len(store.configs) != 0 {
		t.Fatal("config must roll back when audit fails")
	}
}
func TestBuiltinConfigProtected(t *testing.T) {
	store := newMemoryStore()
	store.configs[1] = Config{ID: 1, ConfigName: "内置", ConfigKey: "builtin.key", IsBuiltin: builtin}
	s := NewService(store)
	if err := s.Delete(context.Background(), audit.Metadata{}, 1); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("builtin delete error=%v", err)
	}
	if err := s.Update(context.Background(), audit.Metadata{}, 1, Input{ConfigName: "内置", ConfigKey: "builtin.key", ConfigValue: "x"}); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("builtin update error=%v", err)
	}
	if _, ok := store.configs[1]; !ok {
		t.Fatal("builtin config must remain")
	}
}
func TestBuiltinLogClearValueCanBeUpdatedWithoutChangingMetadata(t *testing.T) {
	store := newMemoryStore()
	remark := "控制日志清空接口是否可用"
	store.configs[1] = Config{
		ID: 1, ConfigName: "日志清空开关", ConfigKey: LogClearEnabledKey,
		ConfigValue: "false", ConfigType: "SYSTEM", ValueType: "BOOLEAN",
		Status: 1, IsBuiltin: builtin, Remark: &remark,
	}
	s := NewService(store)

	if err := s.Update(context.Background(), audit.Metadata{}, 1, Input{ConfigValue: " TRUE "}); err != nil {
		t.Fatalf("update builtin log-clear value=%v", err)
	}

	updated := store.configs[1]
	if updated.ConfigValue != "true" || updated.ConfigName != "日志清空开关" || updated.ConfigKey != LogClearEnabledKey || updated.ValueType != "BOOLEAN" || updated.Remark == nil || *updated.Remark != remark {
		t.Fatalf("builtin config metadata changed: %+v", updated)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].Action != "config.update" || store.auditEvents[0].ResourceID != 1 {
		t.Fatalf("update audit events=%+v", store.auditEvents)
	}
}

func TestBuiltinLogClearValueRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct {
		name      string
		valueType string
		value     string
		want      error
	}{
		{name: "invalid boolean", valueType: "BOOLEAN", value: "yes", want: ErrInvalid},
		{name: "invalid type", valueType: "TEXT", value: "true", want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore()
			store.configs[1] = Config{ID: 1, ConfigKey: LogClearEnabledKey, ValueType: test.valueType, IsBuiltin: builtin}
			s := NewService(store)
			if err := s.Update(context.Background(), audit.Metadata{}, 1, Input{ConfigValue: test.value}); !errors.Is(err, test.want) {
				t.Fatalf("update error=%v, want %v", err, test.want)
			}
			if len(store.auditEvents) != 0 {
				t.Fatalf("invalid update wrote audit events=%+v", store.auditEvents)
			}
		})
	}
}

func TestBatchDeleteWritesOneAuditEvent(t *testing.T) {
	store := newMemoryStore()
	store.configs[2] = Config{ID: 2, ConfigName: "运营", ConfigKey: "ops.enabled", ConfigValue: "true"}
	store.configs[3] = Config{ID: 3, ConfigName: "测试", ConfigKey: "qa.enabled", ConfigValue: "true"}
	s := NewService(store)
	if err := s.DeleteBatch(context.Background(), audit.Metadata{}, []int64{2, 3}); err != nil {
		t.Fatalf("batch delete=%v", err)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].Action != "config.batch-delete" {
		t.Fatalf("batch audit events=%+v", store.auditEvents)
	}
	if _, ok := store.configs[2]; ok {
		t.Fatal("config 2 must be deleted")
	}
	if _, ok := store.configs[3]; ok {
		t.Fatal("config 3 must be deleted")
	}
}
