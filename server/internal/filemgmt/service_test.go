package filemgmt

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
)

func TestServiceUploadPersistsMetadataAndContent(t *testing.T) {
	t.Parallel()
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "greeting.txt", "hello", "text/plain")
	file, err := service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", "example")
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != 1 || file.AccessURL != "/api/system/file/1/view" || file.BusinessModule != "system" {
		t.Fatalf("file = %#v", file)
	}
	resource, err := service.Open(context.Background(), file.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resource.Reader.Close() }()
	content, err := io.ReadAll(resource.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" || resource.File.FileMD5 == "" || len(store.events) != 1 {
		t.Fatalf("content=%q resource=%#v audits=%d", content, resource.File, len(store.events))
	}
}

func TestUploadRollsBackMetadataAndRemovesPhysicalFileWhenAuditFails(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{auditError: errors.New("audit write failed")}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "greeting.txt", "hello", "text/plain")
	if _, err = service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", "example"); err == nil {
		t.Fatal("upload must fail when audit write fails")
	}
	if len(store.records) != 0 {
		t.Fatalf("metadata must roll back when audit fails: %v", store.records)
	}
	if len(storage.saved) != 1 || len(storage.removed) != 1 {
		t.Fatalf("physical file must be compensated: saved=%v removed=%v", storage.saved, storage.removed)
	}
	if storage.saved[0] != storage.removed[0] {
		t.Fatalf("compensation path = %q, want %q", storage.removed[0], storage.saved[0])
	}
}

func TestUploadCompensationFailureKeepsRequestFailure(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	storage := &memoryStorage{removeError: errors.New("remove failed")}
	store := &memoryStore{auditError: errors.New("audit write failed")}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "greeting.txt", "hello", "text/plain")
	if _, err = service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", "example"); err == nil {
		t.Fatal("request must stay failed even when compensation fails")
	}
	if len(store.records) != 0 {
		t.Fatalf("metadata must roll back when audit fails: %v", store.records)
	}
	if len(storage.removed) != 1 {
		t.Fatalf("compensation must be attempted: removed=%v", storage.removed)
	}
	if !strings.Contains(logs.String(), "补偿删除物理文件失败") {
		t.Fatalf("compensation failure must be logged: %q", logs.String())
	}
}

func TestUploadBatchKeepsPerFileResults(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	one := multipartHeader(t, "one.txt", "1", "text/plain")
	two := multipartHeader(t, "two.txt", "22", "text/plain")
	result, err := service.UploadBatch(context.Background(), AuditMetadata{ActorID: 1}, []*multipart.FileHeader{one, two}, "system", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 2 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.records) != 2 || len(store.events) != 2 {
		t.Fatalf("records=%d events=%d", len(store.records), len(store.events))
	}
	for _, file := range store.records {
		if file.AccessURL == "" {
			t.Fatalf("record %d must have access_url: %+v", file.ID, file)
		}
	}
}

func TestUploadBatchReportsFailedFileIndexForDuplicateNames(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	fakeImage := multipartHeader(t, "same.png", "not an image", "image/png")
	validImage := multipartHeader(t, "same.png", onePixelPNG(t), "image/png")

	result, err := service.UploadBatch(context.Background(), AuditMetadata{ActorID: 1}, []*multipart.FileHeader{fakeImage, validImage}, "system", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failed) != 1 || result.Failed[0].FileName != "same.png" || result.Failed[0].Index != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0].OriginalName != "same.png" {
		t.Fatalf("succeeded = %+v", result.Succeeded)
	}
}

func TestUploadRejectsInvalidImageContent(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "photo.png", "not an image", "image/png")

	if _, err = service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", ""); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("upload error = %v, want ErrInvalidImage", err)
	}
	if len(storage.saved) != 0 || len(store.records) != 0 {
		t.Fatalf("invalid image must not be stored: saved=%v records=%v", storage.saved, store.records)
	}
}

func TestUploadAcceptsValidImageContent(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "photo.png", onePixelPNG(t), "image/png")

	file, err := service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", "")
	if err != nil {
		t.Fatal(err)
	}
	if file.MimeType != "image/png" || file.FileSize == 0 {
		t.Fatalf("file = %+v", file)
	}
}

func TestUploadRejectsTruncatedImageContent(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	service, err := NewService(&memoryStore{}, storage)
	if err != nil {
		t.Fatal(err)
	}
	content := onePixelPNG(t)
	truncated := content[:len(content)-5]
	header := multipartHeader(t, "photo.png", truncated, "image/png")

	if _, err = service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", ""); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("upload error = %v, want ErrInvalidImage", err)
	}
	if len(storage.saved) != 0 {
		t.Fatalf("truncated image must not be stored: %v", storage.saved)
	}
}

func TestUploadValidatesImageExtensionWhenContentTypeIsGeneric(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	service, err := NewService(&memoryStore{}, storage)
	if err != nil {
		t.Fatal(err)
	}
	header := multipartHeader(t, "photo.png", "not an image", "application/octet-stream")

	if _, err = service.Upload(context.Background(), AuditMetadata{ActorID: 1}, header, "system", ""); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("upload error = %v, want ErrInvalidImage", err)
	}
}

func onePixelPNG(t *testing.T) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUploadBatchFailureDoesNotAffectIndependentFiles(t *testing.T) {
	t.Parallel()
	storage := &memoryStorage{}
	store := &memoryStore{auditError: errors.New("audit write failed")}
	service, err := NewService(store, storage)
	if err != nil {
		t.Fatal(err)
	}
	one := multipartHeader(t, "one.txt", "1", "text/plain")
	two := multipartHeader(t, "two.txt", "22", "text/plain")
	result, err := service.UploadBatch(context.Background(), AuditMetadata{ActorID: 1}, []*multipart.FileHeader{one, two}, "system", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failed) != 2 || len(result.Succeeded) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.records) != 0 {
		t.Fatalf("no file may be committed: %v", store.records)
	}
	if len(storage.saved) != 2 || len(storage.removed) != 2 {
		t.Fatalf("every saved file must be compensated: saved=%v removed=%v", storage.saved, storage.removed)
	}
}

func multipartHeader(t *testing.T, name, content, contentType string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, "/", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err = request.ParseMultipartForm(int64(body.Len())); err != nil {
		t.Fatal(err)
	}
	return request.MultipartForm.File["file"][0]
}

type memoryStore struct {
	file       File
	records    []File
	events     []AuditEvent
	auditError error
}

func (s *memoryStore) recordFor(id int64) *File {
	for i := range s.records {
		if s.records[i].ID == id {
			return &s.records[i]
		}
	}
	return nil
}

// seed pre-populates a record without emitting an audit event.
func (s *memoryStore) seed(file File) {
	file.ID = int64(len(s.records) + 1)
	if s.file.ID == 0 {
		s.file = file
	}
	s.records = append(s.records, file)
}

func (s *memoryStore) Page(_ context.Context, q FilePageQuery) (Page[File], error) {
	records := make([]File, 0, len(s.records))
	for _, file := range s.records {
		if file.Deleted != 0 {
			continue
		}
		if q.OriginalName != "" && !strings.Contains(file.OriginalName, q.OriginalName) {
			continue
		}
		if q.BusinessModule != "" && file.BusinessModule != q.BusinessModule {
			continue
		}
		if q.MimeType != "" && !strings.Contains(file.MimeType, q.MimeType) {
			continue
		}
		if q.Status != nil && file.Status != *q.Status {
			continue
		}
		records = append(records, file)
	}
	return Page[File]{Records: records, Total: int64(len(records)), Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *memoryStore) Find(_ context.Context, id int64) (*File, error) {
	record := s.recordFor(id)
	if record == nil || record.Deleted != 0 {
		return nil, ErrNotFound
	}
	copy := *record
	return &copy, nil
}

func (s *memoryStore) Create(_ context.Context, file File, e AuditEvent) (File, error) {
	if s.auditError != nil {
		return file, s.auditError
	}
	file.ID = int64(len(s.records) + 1)
	file.AccessURL = "/api/system/file/" + strconv.FormatInt(file.ID, 10) + "/view"
	e.ResourceID = file.ID
	s.events = append(s.events, e)
	if s.file.ID == 0 {
		s.file = file
	}
	s.records = append(s.records, file)
	return file, nil
}

func (s *memoryStore) Update(_ context.Context, id int64, in UpdateInput, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	record := s.recordFor(id)
	if record == nil {
		return ErrNotFound
	}
	record.BusinessModule = in.BusinessModule
	record.Remark = in.Remark
	if s.file.ID == id {
		s.file = *record
	}
	s.events = append(s.events, e)
	return nil
}

func (s *memoryStore) Delete(_ context.Context, id int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	record := s.recordFor(id)
	if record == nil {
		return ErrNotFound
	}
	record.Deleted = 1
	if s.file.ID == id {
		s.file = *record
	}
	s.events = append(s.events, e)
	return nil
}

func (s *memoryStore) DeleteBatch(_ context.Context, ids []int64, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	for _, id := range ids {
		record := s.recordFor(id)
		if record == nil || record.Deleted != 0 {
			return ErrNotFound
		}
	}
	for _, id := range ids {
		record := s.recordFor(id)
		record.Deleted = 1
		if s.file.ID == id {
			s.file = *record
		}
		e.ResourceID = id
		s.events = append(s.events, e)
	}
	return nil
}

func (s *memoryStore) SetStatus(_ context.Context, id int64, status int, e AuditEvent) error {
	if s.auditError != nil {
		return s.auditError
	}
	record := s.recordFor(id)
	if record == nil {
		return ErrNotFound
	}
	record.Status = status
	if s.file.ID == id {
		s.file = *record
	}
	s.events = append(s.events, e)
	return nil
}

// memoryStorage records Save and Remove calls to verify physical compensation.
type memoryStorage struct {
	saved       []string
	removed     []string
	removeError error
}

func (s *memoryStorage) Save(_ context.Context, name string, src io.Reader) (StoredFile, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return StoredFile{}, err
	}
	path := "2026/01/01/" + name
	s.saved = append(s.saved, path)
	return StoredFile{Name: name, Path: path, Size: int64(len(data))}, nil
}

func (s *memoryStorage) Open(_ context.Context, path string) (io.ReadSeekCloser, error) {
	return nil, ErrNotFound
}

func (s *memoryStorage) Remove(_ context.Context, path string) error {
	s.removed = append(s.removed, path)
	return s.removeError
}
