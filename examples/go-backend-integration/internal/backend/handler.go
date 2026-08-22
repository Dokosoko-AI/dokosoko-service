package backend

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRequestBody = 256 << 10

var requestIDPattern = regexp.MustCompile(`^req_[a-f0-9]{32}$`)

type Handler struct {
	acceptor        Acceptor
	bearerTokenHash [sha256.Size]byte
	logger          *slog.Logger
	now             func() time.Time
}

func NewHandler(acceptor Acceptor, bearerToken string, logger *slog.Logger) (*Handler, error) {
	if acceptor == nil {
		return nil, errors.New("acceptor is required")
	}
	if len(bearerToken) < 32 {
		return nil, errors.New("backend bearer token must contain at least 32 characters")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{acceptor: acceptor, bearerTokenHash: sha256.Sum256([]byte(bearerToken)), logger: logger, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/support-submissions", h.supportSubmissions)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	return h.recover(mux)
}

func (h *Handler) supportSubmissions(w http.ResponseWriter, r *http.Request) {
	started := h.now()
	requestID := strings.TrimSpace(r.Header.Get("X-DokoSoko-Request-ID"))
	if !requestIDPattern.MatchString(requestID) {
		requestID = ""
	}
	status := http.StatusInternalServerError
	defer func() {
		h.logger.InfoContext(r.Context(), "support submission request", "method", r.Method, "path", r.URL.Path, "request_id", requestID, "status", status, "duration_ms", h.now().Sub(started).Milliseconds())
	}()
	fail := func(code int, errorCode, message string) {
		status = code
		h.writeError(w, code, errorCode, message, requestID)
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		fail(http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this resource.")
		return
	}
	if !h.authenticated(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="dokosoko-backend"`)
		fail(http.StatusUnauthorized, "authentication_required", "A valid backend bearer credential is required.")
		return
	}
	if requestID == "" {
		fail(http.StatusBadRequest, "invalid_request_id", "X-DokoSoko-Request-ID must match req_ followed by 32 lowercase hexadecimal characters.")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	idempotencyKeyLength := utf8.RuneCountInString(idempotencyKey)
	if idempotencyKeyLength < 16 || idempotencyKeyLength > 200 {
		fail(http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must contain between 16 and 200 characters.")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		fail(http.StatusBadRequest, "invalid_content_type", "Content-Type must be application/json.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		fail(http.StatusBadRequest, "invalid_request", "The request body must be valid JSON and must not exceed 256 KiB.")
		return
	}
	if err := rejectDuplicateObjectKeys(rawBody); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "The request body must not contain duplicate object fields.")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	var input SupportSubmissionRequest
	if err := decoder.Decode(&input); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "The request body must be one valid support-submission JSON object.")
		return
	}
	if err := ensureEOF(decoder); err != nil {
		fail(http.StatusBadRequest, "invalid_request", "The request body must contain exactly one JSON object.")
		return
	}
	if err := validateSubmission(input); err != nil {
		fail(http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	canonical, err := canonicalJSON(rawBody)
	if err != nil {
		fail(http.StatusInternalServerError, "internal_error", "The request could not be processed.")
		return
	}
	digest := sha256.Sum256(canonical)
	receiptID, err := randomID("receipt")
	if err != nil {
		fail(http.StatusInternalServerError, "internal_error", "The request could not be processed.")
		return
	}
	receiptBody, err := json.Marshal(SupportSubmissionReceipt{ID: receiptID, Status: "accepted", ExternalID: input.SubmissionID})
	if err != nil {
		fail(http.StatusInternalServerError, "internal_error", "The request could not be processed.")
		return
	}

	result, err := h.acceptor.Accept(r.Context(), AcceptInput{
		IdempotencyKey: idempotencyKey,
		RequestID:      requestID,
		RequestSHA256:  digest[:],
		CanonicalBody:  canonical,
		ReceiptBody:    receiptBody,
		ReceiptID:      receiptID,
		SubmissionID:   input.SubmissionID,
		Kind:           input.Submission.Kind,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrIdempotencyConflict):
			fail(http.StatusConflict, "idempotency_key_conflict", "Idempotency-Key was already used with different request content.")
		case errors.Is(err, ErrSubmissionConflict):
			fail(http.StatusConflict, "submission_conflict", "The submission was already accepted under another idempotency key.")
		default:
			h.logger.ErrorContext(r.Context(), "support submission persistence failed", "request_id", requestID, "error", err)
			w.Header().Set("Retry-After", "5")
			fail(http.StatusServiceUnavailable, "temporarily_unavailable", "The submission could not be persisted. Retry this request.")
		}
		return
	}
	status = result.StatusCode
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

func (h *Handler) authenticated(r *http.Request) bool {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 8 || !strings.EqualFold(value[:7], "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(value[7:])
	providedHash := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(providedHash[:], h.bearerTokenHash[:]) == 1
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.acceptor.Ready(ctx); err != nil {
		h.writeError(w, http.StatusServiceUnavailable, "not_ready", "Persistence is unavailable.", "")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.ErrorContext(r.Context(), "request panic recovered", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(recovered))
				h.writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed.", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	h.writeJSON(w, status, ErrorEnvelope{Error: APIError{Code: code, Message: message, RequestID: requestID}})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func rejectDuplicateObjectKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := inspectJSONValue(decoder); err != nil {
		return err
	}
	return ensureEOF(decoder)
}

func inspectJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := inspectJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := inspectJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func randomID(prefix string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(random), nil
}
