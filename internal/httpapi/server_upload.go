package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	defaultSourceUploadMaxBytes = int64(5_000_000)
	sourceUploadFormOverhead    = int64(256 << 10)
	sourceUploadFieldMaxBytes   = int64(4 << 10)
)

var (
	errSourceUploadInvalidUTF8 = errors.New("source upload is not valid UTF-8")
	sourceUploadExtensions     = map[string]bool{
		".md": true, ".mdx": true, ".txt": true, ".html": true, ".htm": true,
		".json": true, ".yaml": true, ".yml": true,
	}
)

type sourceUploadError struct {
	status  int
	code    string
	message string
}

type sourceUploadStorageError struct{ err error }

func (e *sourceUploadStorageError) Error() string { return e.err.Error() }
func (e *sourceUploadStorageError) Unwrap() error { return e.err }

func (e *sourceUploadError) Error() string { return e.message }

func uploadError(status int, code, message string) *sourceUploadError {
	return &sourceUploadError{status: status, code: code, message: message}
}

func (s *Server) uploadSource(w http.ResponseWriter, r *http.Request, productID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	if s.uploadDirectory == "" {
		writeError(w, http.StatusServiceUnavailable, "source_upload_disabled", "Reviewed file uploads are not enabled for this deployment.", nil)
		return
	}
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil {
		s.storeError(w, err)
		return
	}

	requestLimit := s.uploadMaxBytes + sourceUploadFormOverhead
	if s.uploadMaxBytes > math.MaxInt64-sourceUploadFormOverhead {
		requestLimit = math.MaxInt64
	}
	r.Body = http.MaxBytesReader(w, r.Body, requestLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_source_upload", "Use multipart/form-data with organisation_id and file fields; name is optional for older clients.", nil)
		return
	}

	var organisationID, name, location, uploadFilename string
	seen := make(map[string]bool)
	keepFile := false
	defer func() {
		if location != "" && !keepFile {
			_ = os.Remove(filepath.Join(s.uploadDirectory, location))
		}
	}()

	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			s.writeSourceUploadError(w, classifySourceUploadReadError(nextErr))
			return
		}
		field := part.FormName()
		if seen[field] || field == "" {
			_ = part.Close()
			writeError(w, http.StatusBadRequest, "invalid_source_upload", "Upload fields must be present exactly once.", nil)
			return
		}
		seen[field] = true

		switch field {
		case "organisation_id", "name":
			if part.FileName() != "" {
				_ = part.Close()
				writeError(w, http.StatusBadRequest, "invalid_source_upload", "organisation_id and name must be text fields.", nil)
				return
			}
			value, readErr := readSourceUploadField(part)
			_ = part.Close()
			if readErr != nil {
				s.writeSourceUploadError(w, readErr)
				return
			}
			if field == "organisation_id" {
				organisationID = strings.TrimSpace(value)
			} else {
				name = strings.TrimSpace(value)
			}
		case "file":
			if part.FileName() == "" {
				_ = part.Close()
				writeError(w, http.StatusBadRequest, "invalid_source_upload", "The file field must include a filename.", nil)
				return
			}
			uploadFilename = sourceUploadDisplayName(part.FileName())
			location, err = s.storeSourceUpload(part)
			_ = part.Close()
			if err != nil {
				s.writeSourceUploadError(w, err)
				return
			}
		default:
			_ = part.Close()
			writeError(w, http.StatusBadRequest, "invalid_source_upload", "Only organisation_id, optional name, and file fields are accepted.", nil)
			return
		}
	}

	if organisationID == "" || location == "" {
		writeError(w, http.StatusBadRequest, "invalid_source_upload", "organisation_id and file are required.", nil)
		return
	}
	if name == "" {
		name = uploadFilename
	}
	if organisationID != product.OrganisationID {
		writeError(w, http.StatusBadRequest, "source_upload_organisation_mismatch", "The organisation does not own the selected product.", nil)
		return
	}

	value, err := s.service.CreateSource(r.Context(), organisationID, productID, name, "upload", location, actor(r))
	if err != nil {
		// CreateSource returns the created value if only its audit append failed.
		// Preserve the file whenever a durable source now references it.
		keepFile = value.ID != ""
		s.creationError(w, err)
		return
	}
	keepFile = true
	writeJSON(w, http.StatusCreated, value)
}

func sourceUploadDisplayName(filename string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	name = strings.ToValidUTF8(name, "�")
	if name == "" || name == "." {
		return "Uploaded document"
	}
	runes := []rune(name)
	if len(runes) > 120 {
		runes = runes[:120]
	}
	return string(runes)
}

func (s *Server) writeSourceUploadError(w http.ResponseWriter, err error) {
	var value *sourceUploadError
	if errors.As(err, &value) {
		writeError(w, value.status, value.code, value.message, nil)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "source_upload_storage_unavailable", "The upload could not be stored. Check the deployment upload volume.", nil)
}

func classifySourceUploadReadError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return uploadError(http.StatusRequestEntityTooLarge, "source_upload_too_large", "The upload exceeds the configured size limit.")
	}
	return uploadError(http.StatusBadRequest, "invalid_source_upload", "The multipart upload is malformed.")
}

func readSourceUploadField(part *multipart.Part) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, sourceUploadFieldMaxBytes+1))
	if err != nil {
		return "", classifySourceUploadReadError(err)
	}
	if int64(len(value)) > sourceUploadFieldMaxBytes {
		return "", uploadError(http.StatusBadRequest, "invalid_source_upload", "Upload text fields are too large.")
	}
	if !utf8.Valid(value) {
		return "", uploadError(http.StatusBadRequest, "invalid_source_upload", "Upload text fields must use valid UTF-8.")
	}
	return string(value), nil
}

func (s *Server) storeSourceUpload(part *multipart.Part) (string, error) {
	extension := strings.ToLower(filepath.Ext(part.FileName()))
	if !sourceUploadExtensions[extension] {
		return "", uploadError(http.StatusUnsupportedMediaType, "source_upload_type_unsupported", "Upload a UTF-8 Markdown, text, HTML, JSON, or YAML source file.")
	}
	info, err := os.Lstat(s.uploadDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", uploadError(http.StatusServiceUnavailable, "source_upload_storage_unavailable", "The deployment upload volume is unavailable.")
	}

	var location string
	var file *os.File
	for attempt := 0; attempt < 8; attempt++ {
		name, randomErr := opaqueSourceUploadName(extension)
		if randomErr != nil {
			return "", randomErr
		}
		candidate := filepath.Join(s.uploadDirectory, name)
		file, err = os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		location = name
		break
	}
	if file == nil {
		return "", errors.New("could not allocate an opaque upload filename")
	}
	storedPath := filepath.Join(s.uploadDirectory, location)
	succeeded := false
	defer func() {
		_ = file.Close()
		if !succeeded {
			_ = os.Remove(storedPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}

	validator := &utf8StreamWriter{destination: file}
	limited := &io.LimitedReader{R: part, N: s.uploadMaxBytes + 1}
	written, err := io.CopyBuffer(validator, limited, make([]byte, 32<<10))
	if errors.Is(err, errSourceUploadInvalidUTF8) {
		return "", uploadError(http.StatusBadRequest, "source_upload_invalid_utf8", "Source uploads must use valid UTF-8.")
	}
	if err != nil {
		var storageError *sourceUploadStorageError
		if errors.As(err, &storageError) {
			return "", storageError
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return "", uploadError(http.StatusRequestEntityTooLarge, "source_upload_too_large", "The upload exceeds the configured size limit.")
		}
		return "", classifySourceUploadReadError(err)
	}
	if written > s.uploadMaxBytes {
		return "", uploadError(http.StatusRequestEntityTooLarge, "source_upload_too_large", "The upload exceeds the configured size limit.")
	}
	if written == 0 {
		return "", uploadError(http.StatusBadRequest, "source_upload_empty", "The source upload must not be empty.")
	}
	if err := validator.Finish(); err != nil {
		return "", uploadError(http.StatusBadRequest, "source_upload_invalid_utf8", "Source uploads must use valid UTF-8.")
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	succeeded = true
	return location, nil
}

func opaqueSourceUploadName(extension string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value) + extension, nil
}

type utf8StreamWriter struct {
	destination io.Writer
	pending     []byte
}

func (w *utf8StreamWriter) Write(value []byte) (int, error) {
	combined := make([]byte, 0, len(w.pending)+len(value))
	combined = append(combined, w.pending...)
	combined = append(combined, value...)
	w.pending = w.pending[:0]

	validEnd := 0
	for validEnd < len(combined) {
		remaining := combined[validEnd:]
		if !utf8.FullRune(remaining) {
			w.pending = append(w.pending, remaining...)
			break
		}
		r, size := utf8.DecodeRune(remaining)
		if r == utf8.RuneError && size == 1 {
			return 0, errSourceUploadInvalidUTF8
		}
		validEnd += size
	}
	if validEnd > 0 {
		if err := writeAll(w.destination, combined[:validEnd]); err != nil {
			return 0, &sourceUploadStorageError{err: err}
		}
	}
	return len(value), nil
}

func (w *utf8StreamWriter) Finish() error {
	if len(w.pending) != 0 {
		return errSourceUploadInvalidUTF8
	}
	return nil
}

func writeAll(destination io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := destination.Write(value)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}
