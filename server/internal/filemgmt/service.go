package filemgmt

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"
)

const maxImagePixels int64 = 25000000

type Service struct {
	store   Store
	storage Storage
}

func NewService(store Store, storage Storage) (*Service, error) {
	if store == nil || storage == nil {
		return nil, ErrInvalid
	}
	return &Service{store: store, storage: storage}, nil
}

func (s *Service) Upload(ctx context.Context, m AuditMetadata, header *multipart.FileHeader, businessModule, remark string) (File, error) {
	if header == nil || strings.TrimSpace(header.Filename) == "" || header.Size == 0 {
		return File{}, ErrFileEmpty
	}
	if header.Size > MaxFileSize {
		return File{}, ErrFileTooLarge
	}
	src, err := header.Open()
	if err != nil {
		return File{}, err
	}
	defer func() { _ = src.Close() }()
	return s.uploadReader(ctx, m, UploadInput{
		Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Size: header.Size, Reader: src,
	}, businessModule, remark)
}

// UploadContent persists a bounded upload stream prepared by the HTTP layer.
func (s *Service) UploadContent(ctx context.Context, m AuditMetadata, input UploadInput, businessModule, remark string) (File, error) {
	return s.uploadReader(ctx, m, input, businessModule, remark)
}

func (s *Service) uploadReader(ctx context.Context, m AuditMetadata, input UploadInput, businessModule, remark string) (File, error) {
	if input.Reader == nil || strings.TrimSpace(input.Filename) == "" || input.Size == 0 {
		return File{}, ErrFileEmpty
	}
	if input.Size > MaxFileSize {
		return File{}, ErrFileTooLarge
	}
	mimeType, err := validateContentType(input.ContentType, input.Filename, input.Reader)
	if err != nil {
		return File{}, err
	}
	stored, err := s.storage.Save(ctx, input.Filename, input.Reader)
	if err != nil {
		return File{}, err
	}
	if !validMD5(stored.MD5) {
		s.compensate(ctx, stored.Path)
		return File{}, ErrInvalid
	}
	f := File{OriginalName: input.Filename, StorageName: stored.Name, Extension: stored.Extension, MimeType: mimeType, FileSize: stored.Size, FileMD5: stored.MD5, StoragePath: stored.Path, BusinessModule: businessModule, Status: StatusEnabled, Remark: stringPtr(remark)}
	f, err = s.store.Create(ctx, f, AuditEvent{Action: "file.upload", Resource: "file", ResourceID: 0, Summary: "上传文件", Metadata: m})
	if err != nil {
		s.compensate(ctx, stored.Path)
		return File{}, err
	}
	return f, nil
}

// compensate deletes the physical file written before a failed database
// transaction. Compensation failure is logged, leaving a best-effort orphan.
func (s *Service) compensate(ctx context.Context, storagePath string) {
	if err := s.storage.Remove(ctx, storagePath); err != nil {
		slog.Error("补偿删除物理文件失败", "storagePath", storagePath, "error", err)
	}
}

func (s *Service) UploadBatch(ctx context.Context, m AuditMetadata, headers []*multipart.FileHeader, businessModule, remark string) (BatchUploadResult, error) {
	if len(headers) == 0 {
		return BatchUploadResult{}, ErrFileEmpty
	}
	result := BatchUploadResult{Succeeded: []File{}, Failed: []BatchUploadFailure{}}
	for index, header := range headers {
		f, err := s.Upload(ctx, m, header, businessModule, remark)
		if err != nil {
			name := "unknown"
			if header != nil && header.Filename != "" {
				name = header.Filename
			}
			result.Failed = append(result.Failed, BatchUploadFailure{FileName: name, Message: err.Error(), Index: index})
			continue
		}
		result.Succeeded = append(result.Succeeded, f)
	}
	return result, nil
}

// UploadBatchContent persists the valid members of a parsed batch. The HTTP
// layer performs batch-level preflight before this method is called, so this
// method retains the existing per-file success/failure behavior.
func (s *Service) UploadBatchContent(ctx context.Context, m AuditMetadata, inputs []UploadInput, businessModule, remark string) (BatchUploadResult, error) {
	if len(inputs) == 0 {
		return BatchUploadResult{}, ErrFileEmpty
	}
	result := BatchUploadResult{Succeeded: []File{}, Failed: []BatchUploadFailure{}}
	for index, input := range inputs {
		f, err := s.UploadContent(ctx, m, input, businessModule, remark)
		if err != nil {
			name := "unknown"
			if input.Filename != "" {
				name = input.Filename
			}
			result.Failed = append(result.Failed, BatchUploadFailure{FileName: name, Message: err.Error(), Index: index})
			continue
		}
		result.Succeeded = append(result.Succeeded, f)
	}
	return result, nil
}

func validateContentType(contentType, filename string, src io.ReadSeeker) (string, error) {
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	declaredImage := parseErr == nil && strings.HasPrefix(strings.ToLower(mediaType), "image/")
	extensionImage := imageMimeFromExtension(filename)
	if parseErr != nil && (strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") || extensionImage != "") {
		return "", ErrInvalidImage
	}
	if !declaredImage && extensionImage == "" {
		return contentType, nil
	}

	expectedMime := ""
	if declaredImage {
		expectedMime = canonicalImageMime(mediaType)
		if expectedMime == "" {
			return "", ErrInvalidImage
		}
	}
	if extensionImage != "" {
		if expectedMime != "" && expectedMime != extensionImage {
			return "", ErrInvalidImage
		}
		expectedMime = extensionImage
	}

	config, _, err := image.DecodeConfig(src)
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxImagePixels/int64(config.Height) {
		return "", ErrInvalidImage
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return "", ErrInvalidImage
	}
	_, format, err := image.Decode(src)
	if err != nil || !matchesImageMime(expectedMime, format) {
		return "", ErrInvalidImage
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return "", ErrInvalidImage
	}
	return imageMimeFromFormat(format), nil
}

func matchesImageMime(mediaType, format string) bool {
	return mediaType == imageMimeFromFormat(format)
}

func canonicalImageMime(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/png":
		return "image/png"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/gif":
		return "image/gif"
	default:
		return ""
	}
}

func imageMimeFromFormat(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	default:
		return ""
	}
}

func imageMimeFromExtension(filename string) string {
	switch strings.ToLower(filepath.Ext(strings.ReplaceAll(filename, "\\", "/"))) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func (s *Service) Page(ctx context.Context, q FilePageQuery) (Page[File], error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	return s.store.Page(ctx, q)
}

func (s *Service) Detail(ctx context.Context, id int64) (*File, error) { return s.store.Find(ctx, id) }

func (s *Service) Update(ctx context.Context, m AuditMetadata, id int64, in UpdateInput) error {
	if len(in.BusinessModule) > 50 || (in.Remark != nil && len(*in.Remark) > 500) {
		return ErrInvalid
	}
	if _, err := s.store.Find(ctx, id); err != nil {
		return err
	}
	return s.store.Update(ctx, id, in, AuditEvent{Action: "file.update", Resource: "file", ResourceID: id, Summary: "更新文件", Metadata: m})
}

func (s *Service) Delete(ctx context.Context, m AuditMetadata, id int64) error {
	if _, err := s.store.Find(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id, AuditEvent{Action: "file.delete", Resource: "file", ResourceID: id, Summary: "删除文件", Metadata: m})
}

func (s *Service) DeleteBatch(ctx context.Context, m AuditMetadata, ids []int64) error {
	if len(ids) == 0 {
		return ErrInvalid
	}
	return s.store.DeleteBatch(ctx, ids, AuditEvent{Action: "file.delete", Resource: "file", ResourceID: 0, Summary: "删除文件", Metadata: m})
}

func (s *Service) SetStatus(ctx context.Context, m AuditMetadata, id int64, status int) error {
	if status != StatusDisabled && status != StatusEnabled {
		return ErrInvalid
	}
	if _, err := s.store.Find(ctx, id); err != nil {
		return err
	}
	return s.store.SetStatus(ctx, id, status, AuditEvent{Action: "file.status", Resource: "file", ResourceID: id, Summary: "更新文件状态", Metadata: m})
}

func (s *Service) Open(ctx context.Context, id int64, _ bool) (FileResource, error) {
	f, err := s.store.Find(ctx, id)
	if err != nil {
		return FileResource{}, err
	}
	r, err := s.storage.Open(ctx, f.StoragePath)
	if err != nil {
		return FileResource{}, err
	}
	return FileResource{Reader: r, File: *f}, nil
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
