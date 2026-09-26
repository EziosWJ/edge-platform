// Package hmi owns canonical HMI page documents and their published snapshots.
package hmi

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	SchemaV1          = "hmi-page/v1"
	MaxDocumentBytes  = 2 << 20
	MaxNodes          = 500
	MaxBindings       = 1000
	ListPermission    = "hmi:list"
	DetailPermission  = "hmi:detail"
	EditPermission    = "hmi:edit"
	PublishPermission = "hmi:publish"
	RunPermission     = "hmi:run"
)

var (
	ErrInvalid        = errors.New("HMI 参数错误")
	ErrNotFound       = errors.New("HMI 页面不存在")
	ErrConflict       = errors.New("HMI 草稿已被其他请求修改")
	ErrNotPublished   = errors.New("HMI_PAGE_NOT_PUBLISHED")
	ErrForbidden      = errors.New("无权访问 HMI 页面")
	ErrInvalidBinding = errors.New("HMI binding 无效")
)

type JSONDocument string

func (d JSONDocument) Value() (driver.Value, error) {
	if d == "" {
		return nil, nil
	}
	return string(d), nil
}
func (d JSONDocument) MarshalJSON() ([]byte, error) {
	if d == "" || !json.Valid([]byte(d)) {
		return []byte("null"), nil
	}
	return bytes.Clone([]byte(d)), nil
}
func (d *JSONDocument) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*d = ""
	case string:
		*d = JSONDocument(v)
	case []byte:
		*d = JSONDocument(string(v))
	default:
		return fmt.Errorf("scan HMI document from %T", value)
	}
	return nil
}

type Page struct {
	PageID             string       `gorm:"column:page_id;primaryKey" json:"pageId"`
	Name               string       `gorm:"column:name" json:"name"`
	Description        string       `gorm:"column:description" json:"description"`
	DraftDocument      JSONDocument `gorm:"column:draft_document;type:jsonb" json:"draftDocument"`
	DraftRevision      int64        `gorm:"column:draft_revision" json:"draftRevision"`
	PublishedVersionID *string      `gorm:"column:published_version_id" json:"publishedVersionId"`
	CreatedAt          time.Time    `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt          time.Time    `gorm:"column:updated_at" json:"updatedAt"`
}

func (Page) TableName() string { return "hmi_page" }

type Version struct {
	VersionID           string       `gorm:"column:version_id;primaryKey" json:"versionId"`
	PageID              string       `gorm:"column:page_id" json:"pageId"`
	VersionNo           int          `gorm:"column:version_no" json:"versionNo"`
	SourceDraftRevision int64        `gorm:"column:source_draft_revision" json:"sourceDraftRevision"`
	Document            JSONDocument `gorm:"column:document;type:jsonb" json:"document"`
	PublishedBy         int64        `gorm:"column:published_by" json:"publishedBy"`
	PublishedAt         time.Time    `gorm:"column:published_at" json:"publishedAt"`
}

func (Version) TableName() string { return "hmi_page_version" }

type PageQuery struct {
	Page, PageSize int
	Name           string
}
type PageList struct {
	Records  []Page `json:"records"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type CreateInput struct {
	Name, Description string
	Document          JSONDocument
}
type MetadataInput struct{ Name, Description string }
type DraftInput struct {
	ExpectedDraftRevision int64
	Document              JSONDocument
}
type PublishInput struct {
	ExpectedDraftRevision int64
	ActorID               int64
}
type RuntimeBootstrap struct {
	Page               PageMetadata        `json:"page"`
	Version            Version             `json:"version"`
	DataPoints         []DataPointMetadata `json:"dataPoints"`
	CanExecuteCommands bool                `json:"canExecuteCommands"`
}
type PageMetadata struct {
	PageID      string `json:"pageId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type DataPointMetadata struct {
	DataPointID string  `json:"dataPointId"`
	DeviceID    string  `json:"deviceId"`
	PointKey    string  `json:"pointKey"`
	Name        string  `json:"name"`
	ValueType   string  `json:"valueType"`
	Unit        *string `json:"unit"`
	Precision   *int    `json:"precision"`
	Enabled     bool    `json:"enabled"`
}
type Document struct {
	Schema string `json:"schema"`
	Canvas Canvas `json:"canvas"`
	Nodes  []Node `json:"nodes"`
}
type Canvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Node struct {
	NodeID   string                     `json:"nodeId"`
	X        float64                    `json:"x"`
	Y        float64                    `json:"y"`
	Width    float64                    `json:"width"`
	Height   float64                    `json:"height"`
	Rotation int                        `json:"rotation"`
	ZIndex   int                        `json:"zIndex"`
	Type     string                     `json:"type"`
	Props    map[string]json.RawMessage `json:"props"`
	Bindings map[string]json.RawMessage `json:"bindings"`
}
