package filemgmt

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

type parsedUpload struct {
	files          []parsedUploadFile
	businessModule string
	remark         string
	seenModule     bool
	seenRemark     bool
}

type parsedUploadFile struct {
	input    UploadInput
	parseErr error
	cleanup  func()
}

func (p *parsedUpload) cleanupFiles() {
	for _, file := range p.files {
		if file.cleanup != nil {
			file.cleanup()
		}
	}
}

// parseMultipartUpload reads an upload without calling ParseMultipartForm.
// Files are staged in bounded temporary files so the Service can validate
// images with a seekable reader while the parser retains batch preflight
// semantics.
func parseMultipartUpload(request *http.Request, fileField string, maxFiles int, maxTotal int64) (parsedUpload, error) {
	if request.ContentLength == 0 {
		return parsedUpload{files: []parsedUploadFile{}}, nil
	}
	reader, err := request.MultipartReader()
	if err != nil {
		return parsedUpload{}, fmt.Errorf("%w: %w", ErrMultipartMalformed, err)
	}

	parsed := parsedUpload{files: []parsedUploadFile{}}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			parsed.cleanupFiles()
			return parsedUpload{}, fmt.Errorf("%w: %w", ErrMultipartMalformed, nextErr)
		}

		fieldName := part.FormName()
		filename := part.FileName()
		if fieldName == fileField {
			if len(parsed.files) >= maxFiles {
				parsed.cleanupFiles()
				if maxFiles == 1 {
					return parsedUpload{}, fmt.Errorf("%w: duplicate file part", ErrMultipartMalformed)
				}
				return parsedUpload{}, ErrBatchTooMany
			}
			if len([]byte(filename)) > MaxFilenameBytes {
				parsed.cleanupFiles()
				return parsedUpload{}, ErrFilenameTooLong
			}

			file, stageErr := stageMultipartFile(part, filename)
			if stageErr != nil && !isPerFileParseError(stageErr) {
				parsed.cleanupFiles()
				return parsedUpload{}, stageErr
			}
			parsed.files = append(parsed.files, file)
			if maxTotal > 0 {
				var total int64
				for _, parsedFile := range parsed.files {
					total += parsedFile.input.Size
				}
				if total > maxTotal {
					parsed.cleanupFiles()
					return parsedUpload{}, ErrBatchTooLarge
				}
			}
			continue
		}

		if filename != "" {
			parsed.cleanupFiles()
			return parsedUpload{}, fmt.Errorf("%w: unsupported file field %q", ErrMultipartMalformed, fieldName)
		}
		switch fieldName {
		case "businessModule":
			value, readErr := readMultipartField(part, MaxBusinessModuleBytes)
			if readErr != nil {
				parsed.cleanupFiles()
				return parsedUpload{}, readErr
			}
			if !parsed.seenModule {
				parsed.businessModule = value
				parsed.seenModule = true
			}
		case "remark":
			value, readErr := readMultipartField(part, MaxRemarkBytes)
			if readErr != nil {
				parsed.cleanupFiles()
				return parsedUpload{}, readErr
			}
			if !parsed.seenRemark {
				parsed.remark = value
				parsed.seenRemark = true
			}
		default:
			parsed.cleanupFiles()
			return parsedUpload{}, fmt.Errorf("%w: unsupported field %q", ErrMultipartMalformed, fieldName)
		}
	}
	return parsed, nil
}

func isPerFileParseError(err error) bool {
	return err == ErrFileEmpty || err == ErrFileTooLarge
}

func readMultipartField(part *multipart.Part, maxBytes int64) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMultipartMalformed, err)
	}
	if int64(len(value)) > maxBytes {
		return "", ErrMultipartFieldLarge
	}
	return string(value), nil
}

func stageMultipartFile(part *multipart.Part, filename string) (parsedUploadFile, error) {
	temporary, err := os.CreateTemp("", ".server-upload-*")
	if err != nil {
		return parsedUploadFile{}, err
	}
	path := temporary.Name()
	removeTemporary := func() {
		_ = temporary.Close()
		_ = os.Remove(path)
	}

	writer := &cappedFileWriter{destination: temporary, limit: MaxFileSize}
	_, copyErr := io.Copy(writer, part)
	if copyErr != nil {
		removeTemporary()
		return parsedUploadFile{}, copyErr
	}
	if writer.total == 0 {
		removeTemporary()
		return parsedUploadFile{input: UploadInput{Filename: filename}, parseErr: ErrFileEmpty}, ErrFileEmpty
	}
	if writer.total > MaxFileSize {
		removeTemporary()
		return parsedUploadFile{
			input:    UploadInput{Filename: filename, Size: writer.total},
			parseErr: ErrFileTooLarge,
		}, ErrFileTooLarge
	}
	if _, err = temporary.Seek(0, io.SeekStart); err != nil {
		removeTemporary()
		return parsedUploadFile{}, err
	}

	return parsedUploadFile{
		input: UploadInput{
			Filename: filename, ContentType: part.Header.Get("Content-Type"), Size: writer.total, Reader: temporary,
		},
		cleanup: removeTemporary,
	}, nil
}

type cappedFileWriter struct {
	destination io.Writer
	limit       int64
	total       int64
}

func (w *cappedFileWriter) Write(p []byte) (int, error) {
	start := w.total
	w.total += int64(len(p))
	if start >= w.limit {
		return len(p), nil
	}

	writeLength := int64(len(p))
	if remaining := w.limit - start; writeLength > remaining {
		writeLength = remaining
	}
	if writeLength > 0 {
		written, err := w.destination.Write(p[:writeLength])
		if err != nil {
			return written, err
		}
		if int64(written) != writeLength {
			return written, io.ErrShortWrite
		}
	}
	return len(p), nil
}
