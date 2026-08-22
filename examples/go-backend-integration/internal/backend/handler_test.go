package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const testToken = "test-backend-token-at-least-32-characters"

type memoryResult struct {
	digest []byte
	result AcceptResult
}

type memoryAcceptor struct {
	mu        sync.Mutex
	results   map[string]memoryResult
	readyErr  error
	acceptErr error
}

func (m *memoryAcceptor) Accept(_ context.Context, input AcceptInput) (AcceptResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.acceptErr != nil {
		return AcceptResult{}, m.acceptErr
	}
	if current, ok := m.results[input.IdempotencyKey]; ok {
		if !bytes.Equal(current.digest, input.RequestSHA256) {
			return AcceptResult{}, ErrIdempotencyConflict
		}
		current.result.Replayed = true
		return current.result, nil
	}
	result := AcceptResult{StatusCode: http.StatusAccepted, Body: append([]byte(nil), input.ReceiptBody...)}
	m.results[input.IdempotencyKey] = memoryResult{digest: append([]byte(nil), input.RequestSHA256...), result: result}
	return result, nil
}

func (m *memoryAcceptor) Ready(context.Context) error { return m.readyErr }

func newTestHandler(t *testing.T, acceptor *memoryAcceptor) http.Handler {
	t.Helper()
	if acceptor.results == nil {
		acceptor.results = make(map[string]memoryResult)
	}
	handler, err := NewHandler(acceptor, testToken, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return handler.Routes()
}

func validBody(t *testing.T) []byte {
	t.Helper()
	allowContact := false
	severity := "medium"
	value := SupportSubmissionRequest{
		SubmissionID: "submission_01JY4R8T7N6M5K4J3H2G1F0E9D",
		CreatedAt:    "2026-08-22T12:00:00Z",
		Submission: SupportSubmission{
			SchemaVersion: "2026-08-20",
			Kind:          "bug",
			Bug:           &BugReport{Summary: "Request fails", Description: "The request returns an unexpected response.", Severity: &severity},
			Reporter: ReporterContext{
				Principal:          ReporterPrincipal{Issuer: "https://identity.vendor.example", Subject: "user_123"},
				ExternalCustomerID: "customer_456",
				AllowContact:       &allowContact,
			},
			Product:     ProductContext{ProductID: "product_123", ProductName: "Example API"},
			Integration: &IntegrationContext{IntegrationID: "integration_123", FamilyKey: "payments", VersionKey: "v1", DisplayName: "Payments API", Lifecycle: "active", Revision: 3},
			Source:      "private_mcp",
			ConfirmedAt: "2026-08-22T12:00:00Z",
			RequestID:   "req_22222222222222222222222222222222",
		},
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func request(handler http.Handler, body []byte, token, key, requestID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/support-submissions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if requestID != "" {
		req.Header.Set("X-DokoSoko-Request-ID", requestID)
	}
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestAcceptsAndReplaysSameSubmission(t *testing.T) {
	acceptor := &memoryAcceptor{}
	handler := newTestHandler(t, acceptor)
	body := validBody(t)
	key := "submission-idempotency-key-123"
	first := request(handler, body, testToken, key, "req_11111111111111111111111111111111")
	if first.Code != http.StatusAccepted {
		t.Fatalf("first response = %d %s", first.Code, first.Body.String())
	}
	indented := new(bytes.Buffer)
	if err := json.Indent(indented, body, "", "  "); err != nil {
		t.Fatal(err)
	}
	second := request(handler, indented.Bytes(), testToken, key, "req_33333333333333333333333333333333")
	if second.Code != http.StatusAccepted || second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay response = %d headers=%v body=%s", second.Code, second.Header(), second.Body.String())
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay changed receipt: first=%s second=%s", first.Body.String(), second.Body.String())
	}
}

func TestRejectsIdempotencyKeyReuseWithDifferentContent(t *testing.T) {
	acceptor := &memoryAcceptor{}
	handler := newTestHandler(t, acceptor)
	body := validBody(t)
	key := "submission-idempotency-key-123"
	if response := request(handler, body, testToken, key, "req_11111111111111111111111111111111"); response.Code != http.StatusAccepted {
		t.Fatalf("first response = %d %s", response.Code, response.Body.String())
	}
	var changed map[string]any
	if err := json.Unmarshal(body, &changed); err != nil {
		t.Fatal(err)
	}
	changed["submission_id"] = "submission_changed"
	changedBody, _ := json.Marshal(changed)
	response := request(handler, changedBody, testToken, key, "req_33333333333333333333333333333333")
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"idempotency_key_conflict"`)) {
		t.Fatalf("conflict response = %d %s", response.Code, response.Body.String())
	}
}

func TestRejectsMissingAuthenticationBeforeReadingContent(t *testing.T) {
	handler := newTestHandler(t, &memoryAcceptor{})
	response := request(handler, []byte(`not-json`), "", "submission-idempotency-key-123", "req_11111111111111111111111111111111")
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthorized response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestRejectsUnknownAndMutuallyExclusiveFields(t *testing.T) {
	handler := newTestHandler(t, &memoryAcceptor{})
	body := validBody(t)
	body = bytes.Replace(body, []byte(`"created_at"`), []byte(`"unexpected":true,"created_at"`), 1)
	response := request(handler, body, testToken, "submission-idempotency-key-123", "req_11111111111111111111111111111111")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field response = %d %s", response.Code, response.Body.String())
	}

	body = validBody(t)
	var input SupportSubmissionRequest
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	input.Submission.Feedback = &FeedbackReport{Message: "Also feedback"}
	body, _ = json.Marshal(input)
	response = request(handler, body, testToken, "another-idempotency-key-123", "req_11111111111111111111111111111111")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mutually exclusive response = %d %s", response.Code, response.Body.String())
	}
}

func TestRejectsDuplicateObjectFields(t *testing.T) {
	handler := newTestHandler(t, &memoryAcceptor{})
	body := validBody(t)
	body = bytes.Replace(body, []byte(`"submission_id"`), []byte(`"submission_id":"different","submission_id"`), 1)
	response := request(handler, body, testToken, "submission-idempotency-key-123", "req_11111111111111111111111111111111")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("duplicate field response = %d %s", response.Code, response.Body.String())
	}
}

func TestPersistenceFailureIsExplicitlyRetryable(t *testing.T) {
	handler := newTestHandler(t, &memoryAcceptor{acceptErr: errors.New("database unavailable")})
	response := request(handler, validBody(t), testToken, "submission-idempotency-key-123", "req_11111111111111111111111111111111")
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "5" {
		t.Fatalf("unavailable response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestRequestFixturesMatchContract(t *testing.T) {
	handler := newTestHandler(t, &memoryAcceptor{})
	for index, name := range []string{"bug.json", "feedback.json"} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "..", "requests", name))
			if err != nil {
				t.Fatal(err)
			}
			key := "fixture-idempotency-key-000" + string(rune('1'+index))
			response := request(handler, body, testToken, key, "req_11111111111111111111111111111111")
			if response.Code != http.StatusAccepted {
				t.Fatalf("fixture response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReadinessReflectsPersistence(t *testing.T) {
	acceptor := &memoryAcceptor{readyErr: errors.New("database unavailable")}
	handler := newTestHandler(t, acceptor)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness response = %d %s", response.Code, response.Body.String())
	}
	acceptor.readyErr = nil
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready response = %d %s", response.Code, response.Body.String())
	}
}

func TestCanonicalJSONPreservesMeaningfulPresence(t *testing.T) {
	first, err := canonicalJSON([]byte(`{"b":2,"a":""}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalJSON([]byte("{\n  \"a\": \"\",\n  \"b\": 2\n}"))
	if err != nil {
		t.Fatal(err)
	}
	absent, err := canonicalJSON([]byte(`{"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("insignificant order or whitespace changed canonical JSON: %s != %s", first, second)
	}
	if bytes.Equal(first, absent) {
		t.Fatal("an explicitly present empty field was conflated with an absent field")
	}
}
