package notification

import (
	"context"
	"sort"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Page(ctx context.Context, userID int64, q PageQuery) (Page, error) {
	var out Page
	d := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, r.is_read").Joins("JOIN sys_notification_recipient r ON r.notification_id=n.id").Where("r.user_id=?", userID)
	if e := d.Count(&out.Total).Error; e != nil {
		return out, e
	}
	e := d.Order("n.publish_time DESC,n.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&out.Records).Error
	out.Page, out.PageSize = q.Page, q.PageSize
	return out, e
}
func (r *Repository) Find(ctx context.Context, userID, id int64) (*Notification, error) {
	var n Notification
	e := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, r.is_read").Joins("JOIN sys_notification_recipient r ON r.notification_id=n.id").Where("n.id=? AND r.user_id=?", id, userID).Scan(&n).Error
	if e != nil {
		return nil, e
	}
	if n.ID == 0 {
		return nil, ErrNotFound
	}
	return &n, nil
}
func (r *Repository) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	var c int64
	e := r.db.WithContext(ctx).Table("sys_notification_recipient").Where("user_id=? AND is_read=0", userID).Count(&c).Error
	return c, e
}
func (r *Repository) MarkRead(ctx context.Context, userID, id int64) error {
	return r.db.WithContext(ctx).Model(&Recipient{}).Where("notification_id=? AND user_id=?", id, userID).Updates(map[string]any{"is_read": 1, "read_time": time.Now().UTC()}).Error
}
func (r *Repository) MarkAllRead(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Model(&Recipient{}).Where("user_id=? AND is_read=0", userID).Updates(map[string]any{"is_read": 1, "read_time": time.Now().UTC()}).Error
}
func (r *Repository) Publish(ctx context.Context, actor int64, in PublishInput, source string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.AllUsers {
			if err := tx.Table("sys_user").Where("status=1 AND deleted=0").Pluck("id", &in.UserIDs).Error; err != nil {
				return err
			}
		}
		var active int64
		if err := tx.Table("sys_user").Where("id IN ? AND status=1 AND deleted=0", in.UserIDs).Count(&active).Error; err != nil {
			return err
		}
		if active != int64(len(in.UserIDs)) {
			return ErrInvalid
		}
		n := Notification{Title: trim(in.Title), Content: trim(in.Content), SourceType: source, PublisherID: &actor, PublishTime: time.Now().UTC()}
		if e := tx.Create(&n).Error; e != nil {
			return e
		}
		for _, id := range in.UserIDs {
			if e := tx.Create(&Recipient{NotificationID: n.ID, UserID: id}).Error; e != nil {
				return e
			}
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "notification.publish", Resource: "notification", ResourceID: n.ID, Summary: "发布站内通知", Metadata: audit.Metadata{ActorID: actor}})
	})
}
func (r *Repository) AdminPage(ctx context.Context, q PageQuery) (Page, error) {
	var out Page
	if e := r.db.WithContext(ctx).Model(&Notification{}).Count(&out.Total).Error; e != nil {
		return out, e
	}
	d := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, COUNT(r.id) AS recipient_count, COALESCE(SUM(CASE WHEN r.is_read=1 THEN 1 ELSE 0 END), 0) AS read_count").Joins("LEFT JOIN sys_notification_recipient r ON r.notification_id=n.id").Group("n.id")
	e := d.Order("publish_time DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out.Records).Error
	out.Page, out.PageSize = q.Page, q.PageSize
	return out, e
}
func (r *Repository) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var c int64
	e := r.db.WithContext(ctx).Table("sys_user_role ur").Joins("JOIN sys_role r ON r.id=ur.role_id").Where("ur.user_id=? AND r.role_code='ADMIN' AND r.status=1 AND r.deleted=0", userID).Count(&c).Error
	return c > 0, e
}
func (r *Repository) RecordRoleChange(ctx context.Context, tx *gorm.DB, userID int64, before, after []string) error {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	b, a := joinRoles(before), joinRoles(after)
	in := PublishInput{Title: "用户角色已更新", Content: "你的角色已发生变更：" + b + " → " + a, UserIDs: []int64{userID}}
	n := Notification{Title: in.Title, Content: in.Content, SourceType: SourceRoleChange, PublishTime: time.Now().UTC()}
	if e := tx.WithContext(ctx).Create(&n).Error; e != nil {
		return e
	}
	return tx.WithContext(ctx).Create(&Recipient{NotificationID: n.ID, UserID: userID}).Error
}
func joinRoles(v []string) string {
	if len(v) == 0 {
		return "无"
	}
	sort.Strings(v)
	out := ""
	for i, s := range v {
		if i > 0 {
			out += "、"
		}
		out += s
	}
	return out
}
