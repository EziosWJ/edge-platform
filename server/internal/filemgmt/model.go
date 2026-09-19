// Package filemgmt owns local file metadata and storage operations.
package filemgmt

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

const (
	StatusDisabled         = 0
	StatusEnabled          = 1
	MaxFileSize            = 50 * 1024 * 1024
	MaxBatchFiles          = 20
	MaxBatchContentSize    = 200 * 1024 * 1024
	MaxSingleBodySize      = 55 * 1024 * 1024
	MaxBatchBodySize       = 210 * 1024 * 1024
	MaxBusinessModuleBytes = 200
	MaxRemarkBytes         = 2000
	MaxFilenameBytes       = 255
)

var (
	ErrNotFound            = errors.New("数据不存在")
	ErrInvalid             = errors.New("参数错误")
	ErrFileEmpty           = errors.New("文件不能为空")
	ErrFileTooLarge        = errors.New("单文件不能超过 50 MiB")
	ErrInvalidImage        = errors.New("图片内容无效")
	ErrMultipartMalformed  = errors.New("multipart 请求格式错误")
	ErrMultipartFieldLarge = errors.New("multipart 字段超过大小限制")
	ErrFilenameTooLong     = errors.New("文件名不能超过 255 字节")
	ErrBatchTooMany        = errors.New("批量上传文件数不能超过 20 个")
	ErrBatchTooLarge       = errors.New("批量上传文件总大小不能超过 200 MiB")
)

type File struct {
	ID             int64     `gorm:"column:id;primaryKey" json:"id"`
	OriginalName   string    `gorm:"column:original_name" json:"originalName"`
	StorageName    string    `gorm:"column:storage_name" json:"storageName"`
	Extension      string    `gorm:"column:extension" json:"extension"`
	MimeType       string    `gorm:"column:mime_type" json:"mimeType"`
	FileSize       int64     `gorm:"column:file_size" json:"fileSize"`
	FileMD5        string    `gorm:"column:file_md5" json:"fileMd5"`
	StoragePath    string    `gorm:"column:storage_path" json:"storagePath"`
	AccessURL      string    `gorm:"column:access_url" json:"accessUrl"`
	BusinessModule string    `gorm:"column:business_module" json:"businessModule"`
	Status         int       `gorm:"column:status" json:"status"`
	Remark         *string   `gorm:"column:remark" json:"remark"`
	CreateTime     time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime     time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted        int       `gorm:"column:deleted" json:"-"`
}

func (File) TableName() string { return "sys_file" }

type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

type FilePageQuery struct {
	Page, PageSize                         int
	OriginalName, BusinessModule, MimeType string
	Status                                 *int
}

type UpdateInput struct {
	BusinessModule string
	Remark         *string
}

type StoredFile struct {
	Name, Path, Extension, MD5 string
	Size                       int64
}

func validMD5(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// UploadInput is a bounded, seekable file stream prepared by the HTTP layer.
// The Service consumes it without depending on Gin or multipart internals.
type UploadInput struct {
	Filename    string
	ContentType string
	Size        int64
	Reader      io.ReadSeeker
}

// FileResource deliberately exposes a stream instead of a byte slice.
type FileResource struct {
	Reader io.ReadSeekCloser
	File   File
}

type BatchUploadResult struct {
	Succeeded []File               `json:"succeeded"`
	Failed    []BatchUploadFailure `json:"failed"`
}

type BatchUploadFailure struct {
	FileName string `json:"fileName"`
	Message  string `json:"message"`
	Index    int    `json:"index"`
}

type AuditMetadata = audit.Metadata
type AuditEvent = audit.Event

type Store interface {
	Page(context.Context, FilePageQuery) (Page[File], error)
	Find(context.Context, int64) (*File, error)
	Create(context.Context, File, AuditEvent) (File, error)
	Update(context.Context, int64, UpdateInput, AuditEvent) error
	Delete(context.Context, int64, AuditEvent) error
	DeleteBatch(context.Context, []int64, AuditEvent) error
	SetStatus(context.Context, int64, int, AuditEvent) error
}

type Storage interface {
	Save(context.Context, string, io.Reader) (StoredFile, error)
	Open(context.Context, string) (io.ReadSeekCloser, error)
	Remove(context.Context, string) error
}
