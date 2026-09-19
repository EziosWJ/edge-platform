package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTypeRulesAndAudit(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.types[1] = DictType{ID: 1, DictName: "内置", DictCode: "BUILTIN", Status: 1, IsBuiltin: 1}
	store.types[2] = DictType{ID: 2, DictName: "普通", DictCode: "NORMAL", Status: 1}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateType(context.Background(), AuditMetadata{ActorID: 7}, TypeInput{DictName: "测试", DictCode: "TEST"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != StatusEnabled || created.SortOrder != 0 || created.IsBuiltin != BuiltinNo {
		t.Fatalf("create defaults = %+v", created)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].Action != "dict-type.create" || store.auditEvents[0].Metadata.ActorID != 7 {
		t.Fatalf("audit events = %+v", store.auditEvents)
	}

	_, err = service.CreateType(context.Background(), AuditMetadata{}, TypeInput{DictName: "重复", DictCode: "TEST"})
	if !errors.Is(err, ErrDictCodeConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	if len(store.auditEvents) != 1 {
		t.Fatalf("failed create wrote audit: %+v", store.auditEvents)
	}

	_, err = service.UpdateType(context.Background(), AuditMetadata{}, 1, TypeInput{DictName: "内置", DictCode: "CHANGED"})
	if !errors.Is(err, ErrBuiltinProtected) {
		t.Fatalf("builtin code error = %v", err)
	}
	if err := service.DeleteType(context.Background(), AuditMetadata{}, 1); !errors.Is(err, ErrBuiltinProtected) {
		t.Fatalf("builtin delete error = %v", err)
	}

	store.data[1] = DictData{ID: 1, DictTypeID: 2, DictLabel: "有数据", DictValue: "1"}
	if err := service.DeleteType(context.Background(), AuditMetadata{}, 2); !errors.Is(err, ErrTypeHasData) {
		t.Fatalf("associated delete error = %v", err)
	}
	if len(store.deletedTypes) != 0 {
		t.Fatalf("protected types deleted: %v", store.deletedTypes)
	}
}

func TestBatchValidationHappensBeforeDelete(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.types[2] = DictType{ID: 2, DictName: "普通", DictCode: "NORMAL"}
	store.types[3] = DictType{ID: 3, DictName: "内置", DictCode: "BUILTIN", IsBuiltin: BuiltinYes}
	service, _ := NewService(store)

	err := service.DeleteTypes(context.Background(), AuditMetadata{}, []int64{2, 3})
	if !errors.Is(err, ErrBuiltinProtected) {
		t.Fatalf("batch error = %v", err)
	}
	if len(store.deletedTypes) != 0 || len(store.auditEvents) != 0 {
		t.Fatalf("failed batch mutated state: deleted=%v audit=%v", store.deletedTypes, store.auditEvents)
	}
}

func TestDataRulesItemsAndEmptySlices(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.types[1] = DictType{ID: 1, DictName: "状态", DictCode: "STATUS", Status: StatusEnabled}
	service, _ := NewService(store)

	created, err := service.CreateData(context.Background(), AuditMetadata{}, DataInput{DictTypeID: 1, DictLabel: "启用", DictValue: "1"})
	if err != nil {
		t.Fatalf("create data: %v", err)
	}
	if created.SortOrder != 0 || len(store.auditEvents) != 1 || store.auditEvents[0].Action != "dict-data.create" {
		t.Fatalf("created=%+v audit=%+v", created, store.auditEvents)
	}
	_, err = service.CreateData(context.Background(), AuditMetadata{}, DataInput{DictTypeID: 1, DictLabel: "重复", DictValue: "1"})
	if !errors.Is(err, ErrDictValueConflict) || len(store.auditEvents) != 1 {
		t.Fatalf("duplicate result err=%v audit=%v", err, store.auditEvents)
	}
	_, err = service.CreateData(context.Background(), AuditMetadata{}, DataInput{DictTypeID: 999, DictLabel: "未知", DictValue: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing type error = %v", err)
	}

	store.items = nil
	items, err := service.Items(context.Background(), "UNKNOWN")
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty items = %#v, %v", items, err)
	}
	page, err := service.TypePage(context.Background(), TypePageQuery{})
	if err != nil || page.Records == nil || page.Page != 1 || page.PageSize != 10 {
		t.Fatalf("empty page = %+v, %v", page, err)
	}
}

func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	store := newFakeStore()
	store.auditError = errors.New("audit write failed")
	service, _ := NewService(store)

	_, err := service.CreateType(context.Background(), AuditMetadata{}, TypeInput{DictName: "测试", DictCode: "TEST"})
	if err == nil {
		t.Fatal("create must fail when audit fails")
	}
	if len(store.types) != 0 {
		t.Fatal("type must roll back when audit fails")
	}
}

func TestBatchDeleteWritesOneAuditEvent(t *testing.T) {
	store := newFakeStore()
	store.types[2] = DictType{ID: 2, DictName: "普通", DictCode: "NORMAL"}
	store.types[3] = DictType{ID: 3, DictName: "测试", DictCode: "QA"}
	service, _ := NewService(store)

	if err := service.DeleteTypes(context.Background(), AuditMetadata{}, []int64{2, 3}); err != nil {
		t.Fatalf("batch delete=%v", err)
	}
	if len(store.auditEvents) != 1 || store.auditEvents[0].Action != "dict-type.batch-delete" {
		t.Fatalf("audit events=%+v", store.auditEvents)
	}
	if store.types[2].Deleted != 1 || store.types[3].Deleted != 1 {
		t.Fatalf("batch delete left live rows: %+v", store.types)
	}
}

func TestHandlerContractUsesEmptyArrayNullMutationAndTransportValidation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	store := newFakeStore()
	service, _ := NewService(store)
	handler, _ := NewHandler(service)
	router := gin.New()
	group := router.Group("/api/system")
	RegisterRoutes(group, handler)

	response := perform(router, http.MethodGet, "/api/system/dict/UNKNOWN/items", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) {
		t.Fatalf("items response = %d %s", response.Code, response.Body.String())
	}

	response = perform(router, http.MethodPost, "/api/system/dict-type", `{"dictName":"测试","dictCode":"TEST"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":200`) || !strings.Contains(response.Body.String(), `"data":null`) {
		t.Fatalf("mutation response = %d %s", response.Code, response.Body.String())
	}

	response = perform(router, http.MethodGet, "/api/system/dict-type/page?pageSize=501", "")
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadRequest || envelope.Code != 400 {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

func perform(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakeStore struct {
	types        map[int64]DictType
	data         map[int64]DictData
	nextTypeID   int64
	nextDataID   int64
	deletedTypes []int64
	deletedData  []int64
	items        []DictItem
	auditEvents  []AuditEvent
	auditError   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{types: map[int64]DictType{}, data: map[int64]DictData{}, nextTypeID: 10, nextDataID: 10}
}

func (f *fakeStore) PageTypes(_ context.Context, query TypePageQuery) (Page[DictType], error) {
	records := []DictType{}
	for _, value := range f.types {
		if value.Deleted == 0 {
			records = append(records, value)
		}
	}
	return Page[DictType]{Records: records, Total: int64(len(records)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (f *fakeStore) FindType(_ context.Context, id int64) (*DictType, error) {
	value, ok := f.types[id]
	if !ok || value.Deleted != 0 {
		return nil, ErrNotFound
	}
	return &value, nil
}

func (f *fakeStore) DictCodeExists(_ context.Context, code string, excludeID int64) (bool, error) {
	for id, value := range f.types {
		if id != excludeID && value.Deleted == 0 && value.DictCode == code {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) CreateType(_ context.Context, value DictType, e AuditEvent) (DictType, error) {
	if f.auditError != nil {
		return value, f.auditError
	}
	f.nextTypeID++
	value.ID = f.nextTypeID
	f.types[value.ID] = value
	e.ResourceID = value.ID
	f.auditEvents = append(f.auditEvents, e)
	return value, nil
}

func (f *fakeStore) UpdateType(_ context.Context, value DictType, e AuditEvent) (DictType, error) {
	if f.auditError != nil {
		return value, f.auditError
	}
	f.types[value.ID] = value
	f.auditEvents = append(f.auditEvents, e)
	return value, nil
}

func (f *fakeStore) CountDataByType(_ context.Context, typeID int64) (int64, error) {
	var count int64
	for _, value := range f.data {
		if value.DictTypeID == typeID && value.Deleted == 0 {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) DeleteTypes(_ context.Context, ids []int64, e AuditEvent) error {
	if f.auditError != nil {
		return f.auditError
	}
	f.deletedTypes = append(f.deletedTypes, ids...)
	for _, id := range ids {
		value := f.types[id]
		value.Deleted = 1
		f.types[id] = value
	}
	f.auditEvents = append(f.auditEvents, e)
	return nil
}

func (f *fakeStore) SetTypeStatus(_ context.Context, id int64, status int, e AuditEvent) error {
	if f.auditError != nil {
		return f.auditError
	}
	value := f.types[id]
	value.Status = status
	f.types[id] = value
	f.auditEvents = append(f.auditEvents, e)
	return nil
}

func (f *fakeStore) PageData(_ context.Context, query DataPageQuery) (Page[DictData], error) {
	records := []DictData{}
	for _, value := range f.data {
		if value.Deleted == 0 {
			records = append(records, value)
		}
	}
	return Page[DictData]{Records: records, Total: int64(len(records)), Page: query.Page, PageSize: query.PageSize}, nil
}

func (f *fakeStore) FindData(_ context.Context, id int64) (*DictData, error) {
	value, ok := f.data[id]
	if !ok || value.Deleted != 0 {
		return nil, ErrNotFound
	}
	return &value, nil
}

func (f *fakeStore) DictValueExists(_ context.Context, typeID int64, value string, excludeID int64) (bool, error) {
	for id, data := range f.data {
		if id != excludeID && data.Deleted == 0 && data.DictTypeID == typeID && data.DictValue == value {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) CreateData(_ context.Context, value DictData, e AuditEvent) (DictData, error) {
	if f.auditError != nil {
		return value, f.auditError
	}
	f.nextDataID++
	value.ID = f.nextDataID
	f.data[value.ID] = value
	e.ResourceID = value.ID
	f.auditEvents = append(f.auditEvents, e)
	return value, nil
}

func (f *fakeStore) UpdateData(_ context.Context, value DictData, e AuditEvent) (DictData, error) {
	if f.auditError != nil {
		return value, f.auditError
	}
	f.data[value.ID] = value
	f.auditEvents = append(f.auditEvents, e)
	return value, nil
}

func (f *fakeStore) DeleteData(_ context.Context, ids []int64, e AuditEvent) error {
	if f.auditError != nil {
		return f.auditError
	}
	f.deletedData = append(f.deletedData, ids...)
	for _, id := range ids {
		value := f.data[id]
		value.Deleted = 1
		f.data[id] = value
	}
	f.auditEvents = append(f.auditEvents, e)
	return nil
}

func (f *fakeStore) Items(context.Context, string) ([]DictItem, error) { return f.items, nil }
