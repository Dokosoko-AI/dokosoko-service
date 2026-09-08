package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	s.receiveSourceUpload(w, r, productID, "")
}

func (s *Server) replaceSourceUpload(w http.ResponseWriter, r *http.Request, productID, sourceID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	if r.Method == http.MethodGet {
		value, err := s.service.SourceInputReplacement(r.Context(), productID, sourceID, r.Header.Get("Idempotency-Key"), actor(r))
		if err != nil {
			s.sourceInputReplacementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
		return
	}
	s.receiveSourceUpload(w, r, productID, sourceID)
}

func (s *Server) sourceInputReplacementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSourceInputChanged):
		writeError(w, http.StatusConflict, "source_input_changed", "This source or replacement request changed. Open the current source before replacing it again.", nil)
	case errors.Is(err, store.ErrSourceImportActive):
		writeError(w, http.StatusConflict, "source_import_active", "Wait for the queued or running import to finish before replacing this file.", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "source_input_changed", "The source changed or the replacement could not commit. Reload the current source before retrying.", nil)
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrInvalidSourceReplacementInput):
		writeError(w, http.StatusBadRequest, "invalid_source_upload", err.Error(), nil)
	default:
		writeError(w, http.StatusServiceUnavailable, "source_replacement_unavailable", "The replacement could not be confirmed. Reload to recover a saved import, or retry with the same file.", nil)
	}
}

func (s *Server) receiveSourceUpload(w http.ResponseWriter, r *http.Request, productID, replacementSourceID string) {
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

	var organisationID, name, location, uploadFilename, contentDigest, revisionText string
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
		case "organisation_id", "name", "revision":
			if field == "revision" && replacementSourceID == "" || field == "name" && replacementSourceID != "" {
				_ = part.Close()
				writeError(w, http.StatusBadRequest, "invalid_source_upload", "Creation accepts an optional name; replacement requires a revision and retains the source name.", nil)
				return
			}
			if part.FileName() != "" {
				_ = part.Close()
				writeError(w, http.StatusBadRequest, "invalid_source_upload", "Upload metadata must be text fields.", nil)
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
			} else if field == "revision" {
				revisionText = strings.TrimSpace(value)
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
			location, contentDigest, err = s.storeSourceUpload(part)
			_ = part.Close()
			if err != nil {
				s.writeSourceUploadError(w, err)
				return
			}
		default:
			_ = part.Close()
			writeError(w, http.StatusBadRequest, "invalid_source_upload", "Use organisation_id and file, plus optional name for creation or revision for replacement.", nil)
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

	if replacementSourceID != "" {
		revision, parseErr := strconv.ParseInt(revisionText, 10, 64)
		if parseErr != nil || revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_source_upload", "Replacement requires the positive revision of the source being replaced.", nil)
			return
		}
		value, err := s.service.ReplaceSourceInput(r.Context(), platform.SourceInputReplacementInput{ProductID: productID, SourceID: replacementSourceID, Revision: revision, Location: location, Filename: uploadFilename, ContentDigest: contentDigest, RequestKey: r.Header.Get("Idempotency-Key")}, actor(r))
		keepFile = value.Source.ID != "" && value.Source.Location == location
		if err != nil {
			s.sourceInputReplacementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
		return
	}
	value, err := s.service.CreateSourceWithRequest(r.Context(), platform.SourceCreationInput{OrganisationID: organisationID, ProductID: productID, Name: name, Kind: "upload", Location: location, RequestKey: r.Header.Get("Idempotency-Key"), ContentDigest: contentDigest}, actor(r))
	if err != nil {
		// CreateSource returns the created value if only its audit append failed.
		// Preserve the file whenever a durable source now references it.
		keepFile = value.ID != "" && value.Location == location
		s.sourceCreationError(w, err)
		return
	}
	keepFile = value.Location == location
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

func (s *Server) storeSourceUpload(part *multipart.Part) (string, string, error) {
	extension := strings.ToLower(filepath.Ext(part.FileName()))
	if !sourceUploadExtensions[extension] {
		return "", "", uploadError(http.StatusUnsupportedMediaType, "source_upload_type_unsupported", "Upload a UTF-8 Markdown, text, HTML, JSON, or YAML source file.")
	}
	info, err := os.Lstat(s.uploadDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", uploadError(http.StatusServiceUnavailable, "source_upload_storage_unavailable", "The deployment upload volume is unavailable.")
	}

	var location string
	var file *os.File
	for attempt := 0; attempt < 8; attempt++ {
		name, randomErr := opaqueSourceUploadName(extension)
		if randomErr != nil {
			return "", "", randomErr
		}
		candidate := filepath.Join(s.uploadDirectory, name)
		file, err = os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		location = name
		break
	}
	if file == nil {
		return "", "", errors.New("could not allocate an opaque upload filename")
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
		return "", "", err
	}

	digest := sha256.New()
	validator := &utf8StreamWriter{destination: io.MultiWriter(file, digest)}
	limited := &io.LimitedReader{R: part, N: s.uploadMaxBytes + 1}
	written, err := io.CopyBuffer(validator, limited, make([]byte, 32<<10))
	if errors.Is(err, errSourceUploadInvalidUTF8) {
		return "", "", uploadError(http.StatusBadRequest, "source_upload_invalid_utf8", "Source uploads must use valid UTF-8.")
	}
	if err != nil {
		var storageError *sourceUploadStorageError
		if errors.As(err, &storageError) {
			return "", "", storageError
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return "", "", uploadError(http.StatusRequestEntityTooLarge, "source_upload_too_large", "The upload exceeds the configured size limit.")
		}
		return "", "", classifySourceUploadReadError(err)
	}
	if written > s.uploadMaxBytes {
		return "", "", uploadError(http.StatusRequestEntityTooLarge, "source_upload_too_large", "The upload exceeds the configured size limit.")
	}
	if written == 0 {
		return "", "", uploadError(http.StatusBadRequest, "source_upload_empty", "The source upload must not be empty.")
	}
	if err := validator.Finish(); err != nil {
		return "", "", uploadError(http.StatusBadRequest, "source_upload_invalid_utf8", "Source uploads must use valid UTF-8.")
	}
	if err := file.Sync(); err != nil {
		return "", "", err
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	succeeded = true
	return location, hex.EncodeToString(digest.Sum(nil)), nil
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
