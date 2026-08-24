package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func sourceUploadRequest(t *testing.T, handler http.Handler, productID, filename string, content []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range []string{"organisation_id", "name"} {
		if value, ok := fields[name]; ok {
			if err := writer.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID+"/sources/upload", &body)
	request.Header.Set("Authorization", "Bearer doko_admin_demo")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sourceUploadServer(directory string, maxBytes int64) (http.Handler, *store.Memory) {
	memory := store.NewMemory()
	return httpapi.NewWithOptions(platform.New(memory), httpapi.Options{
		BaseURL:         "https://dokosoko.example",
		AllowDemoTokens: true,
		UploadDirectory: directory,
		UploadMaxBytes:  maxBytes,
	}), memory
}

func uploadDirectoryEntries(t *testing.T, directory string) []os.DirEntry {
	t.Helper()
	values, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestReviewedSourceUploadCreatesPrivateDraftWithoutQueueingCrawl(t *testing.T) {
	directory := t.TempDir()
	handler, memory := sourceUploadServer(directory, 1024)
	content := []byte("# Developer guide\nUse café credentials only from the server.\n")
	response := sourceUploadRequest(t, handler, "prod_acme", "customer-facing-name.MD", content, map[string]string{
		"organisation_id": "org_acme",
		"name":            "Reviewed developer guide",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", response.Code, response.Body.String())
	}
	var source model.Source
	if err := json.Unmarshal(response.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	if source.Kind != "upload" || source.Visibility != model.VisibilityPrivate || source.Published || source.Quarantined {
		t.Fatalf("unexpected source state: %+v", source)
	}
	if source.Name != "Reviewed developer guide" || source.OrganisationID != "org_acme" || source.ProductID != "prod_acme" {
		t.Fatalf("unexpected source identity: %+v", source)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}\.md$`).MatchString(source.Location) || strings.Contains(source.Location, "customer-facing-name") || filepath.IsAbs(source.Location) {
		t.Fatalf("source location is not opaque and relative: %q", source.Location)
	}
	stored, err := os.ReadFile(filepath.Join(directory, source.Location))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, content) {
		t.Fatalf("stored content = %q", stored)
	}
	info, err := os.Stat(filepath.Join(directory, source.Location))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("stored mode = %o, want 600", info.Mode().Perm())
	}
	jobs, err := memory.CrawlJobs(context.Background(), source.ProductID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("upload unexpectedly queued %d crawl jobs", len(jobs))
	}
	publish := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/sources/"+source.ID+"/publish", "doko_admin_demo", `{"revision":1}`)
	if publish.Code != http.StatusBadRequest || !strings.Contains(publish.Body.String(), "completed, reviewable crawl") {
		t.Fatalf("unreviewed source publish status = %d, body = %s", publish.Code, publish.Body.String())
	}
}

func TestReviewedSourceUploadRejectsOversizedFileAndCleansStorage(t *testing.T) {
	directory := t.TempDir()
	handler, _ := sourceUploadServer(directory, 4)
	response := sourceUploadRequest(t, handler, "prod_acme", "guide.txt", []byte("12345"), map[string]string{
		"organisation_id": "org_acme",
		"name":            "Guide",
	})
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "source_upload_too_large") {
		t.Fatalf("oversized upload status = %d, body = %s", response.Code, response.Body.String())
	}
	if entries := uploadDirectoryEntries(t, directory); len(entries) != 0 {
		t.Fatalf("oversized upload left %d files", len(entries))
	}
}

func TestReviewedSourceUploadRejectsUnsupportedTypeBeforeStorage(t *testing.T) {
	directory := t.TempDir()
	handler, _ := sourceUploadServer(directory, 1024)
	response := sourceUploadRequest(t, handler, "prod_acme", "manual.pdf", []byte("not a supported document"), map[string]string{
		"organisation_id": "org_acme",
		"name":            "Manual",
	})
	if response.Code != http.StatusUnsupportedMediaType || !strings.Contains(response.Body.String(), "source_upload_type_unsupported") {
		t.Fatalf("unsupported upload status = %d, body = %s", response.Code, response.Body.String())
	}
	if entries := uploadDirectoryEntries(t, directory); len(entries) != 0 {
		t.Fatalf("unsupported upload left %d files", len(entries))
	}
}

func TestReviewedSourceUploadDisabledWithoutConfiguredDirectory(t *testing.T) {
	handler, _ := sourceUploadServer("", 1024)
	response := sourceUploadRequest(t, handler, "prod_acme", "guide.md", []byte("# Guide"), map[string]string{
		"organisation_id": "org_acme",
		"name":            "Guide",
	})
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "source_upload_disabled") {
		t.Fatalf("disabled upload status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGenericSourceCreationCannotBypassReviewedUploadStorage(t *testing.T) {
	directory := t.TempDir()
	handler, _ := sourceUploadServer(directory, 1024)
	response := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/sources", "doko_admin_demo", `{"organisation_id":"org_acme","name":"Bypass","kind":"upload","location":"../../caller-selected.md"}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "source_upload_requires_multipart") {
		t.Fatalf("generic upload source status = %d, body = %s", response.Code, response.Body.String())
	}
	if entries := uploadDirectoryEntries(t, directory); len(entries) != 0 {
		t.Fatalf("generic source bypass left %d files", len(entries))
	}
}

func TestReviewedSourceUploadReportsUnavailableStorage(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing-volume")
	handler, _ := sourceUploadServer(directory, 1024)
	response := sourceUploadRequest(t, handler, "prod_acme", "guide.md", []byte("# Guide"), map[string]string{
		"organisation_id": "org_acme",
		"name":            "Guide",
	})
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "source_upload_storage_unavailable") {
		t.Fatalf("unavailable storage status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReviewedSourceUploadRejectsInvalidUTF8AndCleansIncompleteForm(t *testing.T) {
	t.Run("invalid UTF-8", func(t *testing.T) {
		directory := t.TempDir()
		handler, _ := sourceUploadServer(directory, 1024)
		response := sourceUploadRequest(t, handler, "prod_acme", "guide.txt", []byte{0xff, 0xfe}, map[string]string{
			"organisation_id": "org_acme",
			"name":            "Guide",
		})
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "source_upload_invalid_utf8") {
			t.Fatalf("invalid UTF-8 status = %d, body = %s", response.Code, response.Body.String())
		}
		if entries := uploadDirectoryEntries(t, directory); len(entries) != 0 {
			t.Fatalf("invalid UTF-8 upload left %d files", len(entries))
		}
	})

	t.Run("missing metadata", func(t *testing.T) {
		directory := t.TempDir()
		handler, _ := sourceUploadServer(directory, 1024)
		response := sourceUploadRequest(t, handler, "prod_acme", "guide.md", []byte("# Guide"), map[string]string{
			"organisation_id": "org_acme",
		})
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_source_upload") {
			t.Fatalf("incomplete upload status = %d, body = %s", response.Code, response.Body.String())
		}
		if entries := uploadDirectoryEntries(t, directory); len(entries) != 0 {
			t.Fatalf("incomplete upload left %d files", len(entries))
		}
	})
}
