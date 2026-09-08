package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestSourceUploadRetryKeepsOneSourceFileAndAudit(t *testing.T) {
	directory := t.TempDir()
	handler, backend := sourceUploadServer(directory, 5000000)
	fields := map[string]string{"organisation_id": "org_acme"}
	content := []byte("# API guide\nUse the exact SDK version. 東京\n")
	var first model.Source
	for range 3 {
		response := sourceUploadRequest(t, handler, "prod_acme", "guide.md", content, fields, "upload-retry-request-0001")
		if response.Code != http.StatusCreated {
			t.Fatalf("upload=%d %s", response.Code, response.Body.String())
		}
		var value model.Source
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if first.ID == "" {
			first = value
		}
		if value.ID != first.ID || value.Location != first.Location {
			t.Fatalf("duplicate upload source: %#v", value)
		}
	}
	response := sourceUploadRequest(t, handler, "prod_acme", "guide.md", []byte("Different content"), fields, "upload-retry-request-0001")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source_creation_conflict") {
		t.Fatalf("changed upload=%d %s", response.Code, response.Body.String())
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 1 || files[0].Name() != first.Location {
		t.Fatalf("retained upload files=%v err=%v", files, err)
	}
	stored, err := os.ReadFile(filepath.Join(directory, first.Location))
	if err != nil || string(stored) != string(content) {
		t.Fatalf("original upload changed: %q err=%v", stored, err)
	}
	events, err := backend.AuditEvents(t.Context(), first.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Action == "source.created" && event.TargetID == first.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}

func TestSourceCreationHTTPKeyValidationAndScope(t *testing.T) {
	handler, _ := sourceUploadServer(t.TempDir(), 5000000)
	for _, item := range []struct {
		key, token, body string
		status           int
	}{
		{"short", "doko_admin_demo", `{"organisation_id":"org_acme","kind":"website","location":"https://docs.example.test"}`, 400},
		{"source-key-request-0001", "", `{"organisation_id":"org_acme","kind":"website","location":"https://docs.example.test"}`, 401},
		{"source-key-request-0001", "doko_admin_demo", `{"organisation_id":"org_acme","kind":"website","location":"https://docs.example.test"}`, 201},
		{"source-key-request-0001", "doko_admin_demo", `{"organisation_id":"org_acme","kind":"website","location":"https://docs.example.test/other"}`, 409},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/products/prod_acme/sources", strings.NewReader(item.body))
		if item.token != "" {
			r.Header.Set("Authorization", "Bearer "+item.token)
		}
		r.Header.Set("Idempotency-Key", item.key)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != item.status {
			t.Fatalf("source creation=%d expected=%d body=%s", w.Code, item.status, w.Body.String())
		}
	}
}
