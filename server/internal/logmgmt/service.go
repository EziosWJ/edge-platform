package logmgmt

import (
	"context"
	"errors"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
)

const clearEnabledKey = sysconfig.LogClearEnabledKey

// ByKeyReader reads a sys_config value by key. *sysconfig.Service satisfies it.
type ByKeyReader interface {
	GetByKey(context.Context, string) (*sysconfig.ByKey, error)
}

// Service is the log-management use-case boundary consumed by HTTP.
type Service struct {
	store  Store
	config ByKeyReader
}

func NewService(store Store, config ByKeyReader) (*Service, error) {
	if store == nil {
		return nil, ErrInvalid
	}
	return &Service{store: store, config: config}, nil
}

func (s *Service) LoginLogPage(ctx context.Context, q LoginLogPageQuery) (Page[LoginLog], error) {
	if err := validatePage(q.Page, q.PageSize); err != nil {
		return Page[LoginLog]{}, err
	}
	return s.store.LoginLogPage(ctx, q)
}

func (s *Service) LoginLogDetail(ctx context.Context, id int64) (*LoginLog, error) {
	return s.store.FindLoginLog(ctx, id)
}

func (s *Service) ClearLoginLogs(ctx context.Context, m audit.Metadata) error {
	if err := s.assertClearEnabled(ctx); err != nil {
		return err
	}
	return s.store.ClearLoginLogs(ctx, audit.Event{Action: "login-log.clear", Resource: "login-log", Summary: "清空登录日志", Metadata: m})
}

func (s *Service) OperLogPage(ctx context.Context, q OperLogPageQuery) (Page[OperLogRecord], error) {
	if err := validatePage(q.Page, q.PageSize); err != nil {
		return Page[OperLogRecord]{}, err
	}
	return s.store.OperLogPage(ctx, q)
}

func (s *Service) OperLogDetail(ctx context.Context, id int64) (*OperLogDetail, error) {
	return s.store.FindOperLog(ctx, id)
}

func (s *Service) ClearOperLogs(ctx context.Context, m audit.Metadata) error {
	if err := s.assertClearEnabled(ctx); err != nil {
		return err
	}
	return s.store.ClearOperLogs(ctx, audit.Event{Action: "oper-log.clear", Resource: "oper-log", Summary: "清空操作日志", Metadata: m})
}

func validatePage(page, pageSize int) error {
	if page < 1 {
		return ErrInvalid
	}
	if pageSize < 1 || pageSize > 500 {
		return ErrInvalid
	}
	return nil
}

// assertClearEnabled reads the sys_config gate and fails closed whenever the
// switch is missing, disabled, malformed, or otherwise unavailable.
func (s *Service) assertClearEnabled(ctx context.Context) error {
	if s.config == nil {
		return ErrForbidden
	}
	key, err := s.config.GetByKey(ctx, clearEnabledKey)
	if err != nil {
		if errors.Is(err, sysconfig.ErrNotFound) {
			return ErrForbidden
		}
		return err
	}
	if key == nil || key.ValueType != "BOOLEAN" || !strings.EqualFold(strings.TrimSpace(key.ConfigValue), "true") {
		return ErrForbidden
	}
	return nil
}
