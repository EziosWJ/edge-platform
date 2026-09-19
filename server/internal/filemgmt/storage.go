package filemgmt

import (
	"context"
	"crypto/md5" // #nosec G501 -- MD5 is retained as metadata compatibility, not security.
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalStorage struct{ root string }

func NewLocalStorage(root string) (*LocalStorage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &LocalStorage{root: filepath.Clean(abs)}, nil
}

func (s *LocalStorage) Save(ctx context.Context, originalName string, src io.Reader) (StoredFile, error) {
	if src == nil || strings.TrimSpace(originalName) == "" {
		return StoredFile{}, ErrFileEmpty
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	relDir := time.Now().Format("2006/01/02")
	dir, err := s.resolve(relDir)
	if err != nil {
		return StoredFile{}, err
	}
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return StoredFile{}, err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return StoredFile{}, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	hash := md5.New() // #nosec G401 -- compatibility checksum only.
	n, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(src, MaxFileSize+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return StoredFile{}, copyErr
	}
	if closeErr != nil {
		return StoredFile{}, closeErr
	}
	if n == 0 {
		return StoredFile{}, ErrFileEmpty
	}
	if n > MaxFileSize {
		return StoredFile{}, ErrFileTooLarge
	}

	ext := extension(originalName)
	name, err := randomName(ext)
	if err != nil {
		return StoredFile{}, err
	}
	rel := filepath.Join(relDir, name)
	target, err := s.resolve(rel)
	if err != nil {
		return StoredFile{}, err
	}
	if err = os.Rename(tmpPath, target); err != nil {
		return StoredFile{}, err
	}
	return StoredFile{Name: name, Path: filepath.ToSlash(rel), Extension: ext, Size: n, MD5: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *LocalStorage) Open(ctx context.Context, relativePath string) (io.ReadSeekCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.resolve(relativePath)
	if err != nil {
		return nil, ErrNotFound
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrNotFound
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (s *LocalStorage) Remove(ctx context.Context, relativePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := s.resolve(relativePath)
	if err != nil {
		return ErrNotFound
	}
	if err = os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *LocalStorage) resolve(relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) || strings.TrimSpace(relativePath) == "" {
		return "", fmt.Errorf("%w: unsafe storage path", ErrInvalid)
	}
	p := filepath.Clean(filepath.Join(s.root, relativePath))
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: unsafe storage path", ErrInvalid)
	}
	return p, nil
}

func extension(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	if len(ext) > 50 {
		return ext[:50]
	}
	return ext
}

func randomName(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	name := hex.EncodeToString(b)
	if ext != "" {
		name += "." + ext
	}
	return name, nil
}
