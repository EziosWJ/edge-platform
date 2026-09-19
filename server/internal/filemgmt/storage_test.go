package filemgmt

import (
	"context"
	"crypto/md5" // #nosec G501 -- test verifies compatibility checksum.
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageSaveAndOpen(t *testing.T) {
	t.Parallel()
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := "file content"
	stored, err := storage.Save(context.Background(), "report.txt", strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Extension != "txt" || stored.Size != int64(len(content)) {
		t.Fatalf("stored = %#v", stored)
	}
	wantMD5 := md5.Sum([]byte(content)) // #nosec G401 -- test verifies compatibility checksum.
	if stored.MD5 != hex.EncodeToString(wantMD5[:]) {
		t.Fatalf("md5 = %q", stored.MD5)
	}
	parts := strings.Split(filepath.ToSlash(stored.Path), "/")
	if len(parts) != 4 || len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 || !strings.HasSuffix(parts[3], ".txt") {
		t.Fatalf("storage path = %q, want yyyy/mm/dd/random.txt", stored.Path)
	}
	reader, err := storage.Open(context.Background(), stored.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestLocalStorageRejectsOversizeAndTraversal(t *testing.T) {
	t.Parallel()
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = storage.Save(context.Background(), "large.bin", &zeroReader{remaining: MaxFileSize + 1}); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("oversize error = %v, want ErrFileTooLarge", err)
	}
	if _, err = storage.Open(context.Background(), "../outside.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal error = %v, want ErrNotFound", err)
	}
}

type zeroReader struct{ remaining int64 }

func (r *zeroReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	r.remaining -= n
	return int(n), nil
}
