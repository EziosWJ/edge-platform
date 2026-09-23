package hmi

import (
	"context"
	"errors"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	Page(context.Context, PageQuery) (PageList, error)
	Detail(context.Context, string) (Page, error)
	Create(context.Context, CreateInput, audit.Event) (Page, error)
	UpdateMetadata(context.Context, string, MetadataInput, audit.Event) (Page, error)
	SaveDraft(context.Context, string, DraftInput, audit.Event) (Page, error)
	PublishedForRevision(context.Context, string, int64) (Version, error)
	Publish(context.Context, string, PublishInput, audit.Event) (Version, error)
	Runtime(context.Context, string) (Page, Version, error)
}

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ Store = (*Repository)(nil)

func (r *Repository) Page(ctx context.Context, q PageQuery) (PageList, error) {
	var out PageList
	db := r.db.WithContext(ctx).Model(&Page{})
	if q.Name != "" {
		db = db.Where("LOWER(name) LIKE LOWER(?)", "%"+q.Name+"%")
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := db.Order("updated_at DESC, page_id ASC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out.Records).Error; err != nil {
		return out, err
	}
	if out.Records == nil {
		out.Records = []Page{}
	}
	out.Page, out.PageSize = q.Page, q.PageSize
	return out, nil
}
func (r *Repository) Detail(ctx context.Context, id string) (Page, error) {
	var p Page
	if err := r.db.WithContext(ctx).Where("page_id = ?", id).Take(&p).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Page{}, ErrNotFound
	} else if err != nil {
		return Page{}, err
	}
	return p, nil
}
func (r *Repository) Create(ctx context.Context, in CreateInput, event audit.Event) (Page, error) {
	now := time.Now().UTC()
	p := Page{PageID: uuid.NewString(), Name: in.Name, Description: in.Description, DraftDocument: in.Document, DraftRevision: 1, CreatedAt: now, UpdatedAt: now}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	}); err != nil {
		return Page{}, err
	}
	return p, nil
}
func (r *Repository) UpdateMetadata(ctx context.Context, id string, in MetadataInput, event audit.Event) (Page, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Page
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("page_id = ?", id).Take(&p).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := tx.Model(&p).Updates(map[string]any{"name": in.Name, "description": in.Description, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return Page{}, err
	}
	return r.Detail(ctx, id)
}
func (r *Repository) SaveDraft(ctx context.Context, id string, in DraftInput, event audit.Event) (Page, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Page
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("page_id = ?", id).Take(&p).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if in.ExpectedDraftRevision != p.DraftRevision {
			return ErrConflict
		}
		if err := tx.Model(&p).Updates(map[string]any{"draft_document": in.Document, "draft_revision": gorm.Expr("draft_revision + 1"), "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return Page{}, err
	}
	return r.Detail(ctx, id)
}
func (r *Repository) PublishedForRevision(ctx context.Context, id string, revision int64) (Version, error) {
	var v Version
	if err := r.db.WithContext(ctx).Where("page_id = ? AND source_draft_revision = ?", id, revision).Take(&v).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Version{}, ErrNotFound
	} else if err != nil {
		return Version{}, err
	}
	return v, nil
}
func (r *Repository) Publish(ctx context.Context, id string, in PublishInput, event audit.Event) (Version, error) {
	var result Version
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check successful retries before comparing the current revision.
		var existing Version
		if err := tx.Where("page_id = ? AND source_draft_revision = ?", id, in.ExpectedDraftRevision).Take(&existing).Error; err == nil {
			result = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var p Page
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("page_id = ?", id).Take(&p).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		// A concurrent retry may have committed while this transaction waited for
		// the page lock. Re-read under READ COMMITTED before creating a new version.
		if err := tx.Where("page_id = ? AND source_draft_revision = ?", id, in.ExpectedDraftRevision).Take(&existing).Error; err == nil {
			result = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if in.ExpectedDraftRevision != p.DraftRevision {
			return ErrConflict
		}
		var versionNo int
		if err := tx.Model(&Version{}).Where("page_id = ?", id).Select("COALESCE(MAX(version_no),0)").Scan(&versionNo).Error; err != nil {
			return err
		}
		result = Version{VersionID: uuid.NewString(), PageID: id, VersionNo: versionNo + 1, SourceDraftRevision: p.DraftRevision, Document: p.DraftDocument, PublishedBy: in.ActorID, PublishedAt: time.Now().UTC()}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		if err := tx.Model(&p).Updates(map[string]any{"published_version_id": result.VersionID, "updated_at": result.PublishedAt}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, event)
	})
	return result, err
}
func (r *Repository) Runtime(ctx context.Context, id string) (Page, Version, error) {
	p, err := r.Detail(ctx, id)
	if err != nil {
		return Page{}, Version{}, err
	}
	if p.PublishedVersionID == nil {
		return Page{}, Version{}, ErrNotPublished
	}
	var v Version
	if err := r.db.WithContext(ctx).Where("version_id = ? AND page_id = ?", *p.PublishedVersionID, id).Take(&v).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return Page{}, Version{}, ErrNotPublished
	} else if err != nil {
		return Page{}, Version{}, err
	}
	return p, v, nil
}
