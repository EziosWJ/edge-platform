package notification

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const (
	SourceManual     = "MANUAL"
	SourceRoleChange = "ROLE_CHANGE"
)

var (
	ErrNotFound  = err("通知不存在")
	ErrInvalid   = err("参数错误")
	ErrForbidden = err("无权限")
)

type errorString string

func (e errorString) Error() string { return string(e) }
func err(value string) errorString  { return errorString(value) }

type Notification struct {
	ID             int64     `gorm:"column:id" json:"id"`
	Title          string    `gorm:"column:title" json:"title"`
	Content        string    `gorm:"column:content" json:"content"`
	SourceType     string    `gorm:"column:source_type" json:"sourceType"`
	PublisherID    *int64    `gorm:"column:publisher_id" json:"publisherId"`
	PublishTime    time.Time `gorm:"column:publish_time" json:"publishTime"`
	CreateTime     time.Time `gorm:"column:create_time" json:"createTime"`
	IsRead         int       `gorm:"column:is_read;->" json:"isRead"`
	RecipientCount int64     `gorm:"column:recipient_count;->" json:"recipientCount,omitempty"`
	ReadCount      int64     `gorm:"column:read_count;->" json:"readCount,omitempty"`
}

func (Notification) TableName() string { return "sys_notification" }

type Recipient struct {
	ID             int64      `gorm:"column:id"`
	NotificationID int64      `gorm:"column:notification_id"`
	UserID         int64      `gorm:"column:user_id"`
	IsRead         int        `gorm:"column:is_read"`
	ReadTime       *time.Time `gorm:"column:read_time"`
}

func (Recipient) TableName() string { return "sys_notification_recipient" }

type PageQuery struct{ Page, PageSize int }
type Page struct {
	Records  []Notification `json:"records"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}
type PublishInput struct {
	Title, Content string
	UserIDs        []int64
	AllUsers       bool
}

type Store interface {
	Page(context.Context, int64, PageQuery) (Page, error)
	Find(context.Context, int64, int64) (*Notification, error)
	UnreadCount(context.Context, int64) (int64, error)
	MarkRead(context.Context, int64, int64) error
	MarkAllRead(context.Context, int64) error
	Publish(context.Context, int64, PublishInput, string) error
	AdminPage(context.Context, PageQuery) (Page, error)
	IsAdmin(context.Context, int64) (bool, error)
	RecordRoleChange(context.Context, *gorm.DB, int64, []string, []string) error
}

type Service struct{ store Store }

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalid
	}
	return &Service{store: store}, nil
}
func (s *Service) Page(ctx context.Context, userID int64, q PageQuery) (Page, error) {
	q.Page, q.PageSize = normalizePage(q.Page, q.PageSize)
	page, err := s.store.Page(ctx, userID, q)
	if page.Records == nil {
		page.Records = []Notification{}
	}
	return page, err
}
func (s *Service) Find(ctx context.Context, userID, id int64) (*Notification, error) {
	notification, err := s.store.Find(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.store.MarkRead(ctx, userID, id); err != nil {
		return nil, err
	}
	notification.IsRead = 1
	return notification, nil
}
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.store.UnreadCount(ctx, userID)
}
func (s *Service) MarkRead(ctx context.Context, userID, id int64) error {
	return s.store.MarkRead(ctx, userID, id)
}
func (s *Service) MarkAllRead(ctx context.Context, userID int64) error {
	return s.store.MarkAllRead(ctx, userID)
}
func (s *Service) Publish(ctx context.Context, actor int64, in PublishInput) error {
	if ok, e := s.store.IsAdmin(ctx, actor); e != nil {
		return e
	} else if !ok {
		return ErrForbidden
	}
	in.UserIDs = uniqueIDs(in.UserIDs)
	if len(trim(in.Title)) == 0 || len(trim(in.Content)) == 0 || (!in.AllUsers && len(in.UserIDs) == 0) {
		return ErrInvalid
	}
	return s.store.Publish(ctx, actor, in, SourceManual)
}
func (s *Service) AdminPage(ctx context.Context, actor int64, q PageQuery) (Page, error) {
	if ok, e := s.store.IsAdmin(ctx, actor); e != nil {
		return Page{}, e
	} else if !ok {
		return Page{}, ErrForbidden
	}
	q.Page, q.PageSize = normalizePage(q.Page, q.PageSize)
	page, err := s.store.AdminPage(ctx, q)
	if page.Records == nil {
		page.Records = []Notification{}
	}
	return page, err
}
func trim(v string) string {
	for len(v) > 0 && (v[0] == ' ' || v[0] == '\n' || v[0] == '\r' || v[0] == '\t') {
		v = v[1:]
	}
	for len(v) > 0 && (v[len(v)-1] == ' ' || v[len(v)-1] == '\n' || v[len(v)-1] == '\r' || v[len(v)-1] == '\t') {
		v = v[:len(v)-1]
	}
	return v
}
func normalizePage(p, z int) (int, int) {
	if p < 1 {
		p = 1
	}
	if z < 1 {
		z = 10
	}
	if z > 500 {
		z = 500
	}
	return p, z
}

func uniqueIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
