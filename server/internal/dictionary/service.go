package dictionary

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Service struct {
	store Store
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("dictionary store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) TypePage(ctx context.Context, query TypePageQuery) (Page[DictType], error) {
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)
	return s.store.PageTypes(ctx, query)
}

func (s *Service) TypeDetail(ctx context.Context, id int64) (*DictType, error) {
	return s.store.FindType(ctx, id)
}

func (s *Service) CreateType(ctx context.Context, meta AuditMetadata, input TypeInput) (DictType, error) {
	if err := validateType(input); err != nil {
		return DictType{}, err
	}
	conflict, err := s.store.DictCodeExists(ctx, input.DictCode, 0)
	if err != nil {
		return DictType{}, err
	}
	if conflict {
		return DictType{}, ErrDictCodeConflict
	}
	value := DictType{
		DictName: input.DictName, DictCode: input.DictCode,
		Status: intOr(input.Status, StatusEnabled), SortOrder: intOr(input.SortOrder, 0),
		IsBuiltin: BuiltinNo, Remark: input.Remark,
	}
	value, err = s.store.CreateType(ctx, value, event(meta, "dict-type.create", "dict-type", 0, "创建字典类型"))
	if err != nil {
		return DictType{}, err
	}
	return value, nil
}

func (s *Service) UpdateType(ctx context.Context, meta AuditMetadata, id int64, input TypeInput) (DictType, error) {
	if err := validateType(input); err != nil {
		return DictType{}, err
	}
	existing, err := s.store.FindType(ctx, id)
	if err != nil {
		return DictType{}, err
	}
	if existing.IsBuiltin == BuiltinYes && existing.DictCode != input.DictCode {
		return DictType{}, fmt.Errorf("内置字典类型禁止修改编码: %w", ErrBuiltinProtected)
	}
	conflict, err := s.store.DictCodeExists(ctx, input.DictCode, id)
	if err != nil {
		return DictType{}, err
	}
	if conflict {
		return DictType{}, ErrDictCodeConflict
	}
	remark := input.Remark
	if remark == nil {
		remark = existing.Remark
	}
	value := DictType{
		ID: id, DictName: input.DictName, DictCode: input.DictCode,
		Status: intOr(input.Status, existing.Status), SortOrder: intOr(input.SortOrder, existing.SortOrder),
		IsBuiltin: existing.IsBuiltin, Remark: remark,
	}
	value, err = s.store.UpdateType(ctx, value, event(meta, "dict-type.update", "dict-type", id, "更新字典类型"))
	if err != nil {
		return DictType{}, err
	}
	return value, nil
}

func (s *Service) DeleteType(ctx context.Context, meta AuditMetadata, id int64) error {
	if err := s.validateTypeDelete(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteTypes(ctx, []int64{id}, event(meta, "dict-type.delete", "dict-type", id, "删除字典类型"))
}

func (s *Service) DeleteTypes(ctx context.Context, meta AuditMetadata, ids []int64) error {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return ErrInvalidInput
	}
	for _, id := range ids {
		if err := s.validateTypeDelete(ctx, id); err != nil {
			return err
		}
	}
	return s.store.DeleteTypes(ctx, ids, event(meta, "dict-type.batch-delete", "dict-type", 0, "批量删除字典类型"))
}

func (s *Service) SetTypeStatus(ctx context.Context, meta AuditMetadata, id int64, status int) error {
	if !validStatus(status) {
		return ErrInvalidInput
	}
	if _, err := s.store.FindType(ctx, id); err != nil {
		return err
	}
	return s.store.SetTypeStatus(ctx, id, status, event(meta, "dict-type.status", "dict-type", id, "更新字典类型状态"))
}

func (s *Service) DataPage(ctx context.Context, query DataPageQuery) (Page[DictData], error) {
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)
	return s.store.PageData(ctx, query)
}

func (s *Service) DataDetail(ctx context.Context, id int64) (*DictData, error) {
	return s.store.FindData(ctx, id)
}

func (s *Service) CreateData(ctx context.Context, meta AuditMetadata, input DataInput) (DictData, error) {
	if err := validateData(input); err != nil {
		return DictData{}, err
	}
	if _, err := s.store.FindType(ctx, input.DictTypeID); err != nil {
		return DictData{}, err
	}
	conflict, err := s.store.DictValueExists(ctx, input.DictTypeID, input.DictValue, 0)
	if err != nil {
		return DictData{}, err
	}
	if conflict {
		return DictData{}, ErrDictValueConflict
	}
	value := DictData{
		DictTypeID: input.DictTypeID, DictLabel: input.DictLabel, DictValue: input.DictValue,
		SortOrder: intOr(input.SortOrder, 0), Remark: input.Remark,
	}
	value, err = s.store.CreateData(ctx, value, event(meta, "dict-data.create", "dict-data", 0, "创建字典数据"))
	if err != nil {
		return DictData{}, err
	}
	return value, nil
}

func (s *Service) UpdateData(ctx context.Context, meta AuditMetadata, id int64, input DataInput) (DictData, error) {
	if err := validateData(input); err != nil {
		return DictData{}, err
	}
	existing, err := s.store.FindData(ctx, id)
	if err != nil {
		return DictData{}, err
	}
	if _, err = s.store.FindType(ctx, input.DictTypeID); err != nil {
		return DictData{}, err
	}
	conflict, err := s.store.DictValueExists(ctx, input.DictTypeID, input.DictValue, id)
	if err != nil {
		return DictData{}, err
	}
	if conflict {
		return DictData{}, ErrDictValueConflict
	}
	remark := input.Remark
	if remark == nil {
		remark = existing.Remark
	}
	value := DictData{
		ID: id, DictTypeID: input.DictTypeID, DictLabel: input.DictLabel, DictValue: input.DictValue,
		SortOrder: intOr(input.SortOrder, existing.SortOrder), Remark: remark,
	}
	value, err = s.store.UpdateData(ctx, value, event(meta, "dict-data.update", "dict-data", id, "更新字典数据"))
	if err != nil {
		return DictData{}, err
	}
	return value, nil
}

func (s *Service) DeleteData(ctx context.Context, meta AuditMetadata, id int64) error {
	if _, err := s.store.FindData(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteData(ctx, []int64{id}, event(meta, "dict-data.delete", "dict-data", id, "删除字典数据"))
}

func (s *Service) DeleteDataBatch(ctx context.Context, meta AuditMetadata, ids []int64) error {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return ErrInvalidInput
	}
	for _, id := range ids {
		if _, err := s.store.FindData(ctx, id); err != nil {
			return err
		}
	}
	return s.store.DeleteData(ctx, ids, event(meta, "dict-data.batch-delete", "dict-data", 0, "批量删除字典数据"))
}

func (s *Service) Items(ctx context.Context, dictCode string) ([]DictItem, error) {
	if strings.TrimSpace(dictCode) == "" {
		return []DictItem{}, nil
	}
	items, err := s.store.Items(ctx, dictCode)
	if items == nil {
		items = []DictItem{}
	}
	return items, err
}

func (s *Service) validateTypeDelete(ctx context.Context, id int64) error {
	value, err := s.store.FindType(ctx, id)
	if err != nil {
		return err
	}
	if value.IsBuiltin == BuiltinYes {
		return fmt.Errorf("内置字典类型禁止删除: %w", ErrBuiltinProtected)
	}
	count, err := s.store.CountDataByType(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrTypeHasData
	}
	return nil
}

func event(meta AuditMetadata, action, resource string, id int64, summary string) AuditEvent {
	return AuditEvent{Action: action, Resource: resource, ResourceID: id, Summary: summary, Metadata: meta}
}

func validateType(value TypeInput) error {
	if blankOrLong(value.DictName, 100) || blankOrLong(value.DictCode, 100) {
		return ErrInvalidInput
	}
	if value.Status != nil && !validStatus(*value.Status) {
		return ErrInvalidInput
	}
	return nil
}

func validateData(value DataInput) error {
	if value.DictTypeID <= 0 || blankOrLong(value.DictLabel, 100) || blankOrLong(value.DictValue, 100) {
		return ErrInvalidInput
	}
	return nil
}

func blankOrLong(value string, max int) bool {
	return strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > max
}

func validStatus(value int) bool { return value == StatusDisabled || value == StatusEnabled }

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 500 {
		size = 500
	}
	return page, size
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
