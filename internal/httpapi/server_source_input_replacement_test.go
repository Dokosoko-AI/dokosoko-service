package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func replacementUpload(t *testing.T, handler http.Handler, sourceID, filename string, content []byte, revision int64, key, token string) *httptest.ResponseRecorder {
	t.Helper()
	route := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1/products/prod_acme/sources/" + sourceID + "/upload"
		r.Header.Del("Authorization")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		handler.ServeHTTP(w, r)
	})
	return sourceUploadRequest(t, route, "prod_acme", filename, content, map[string]string{"organisation_id": "org_acme", "revision": strconv.FormatInt(revision, 10)}, key)
}

func TestSourceInputReplacementHTTPRetainsFilesAndRecoversExactImport(t *testing.T) {
	directory := t.TempDir()
	handler, backend := sourceUploadServer(directory, 5000000)
	original := []byte("# Original retained content\n")
	created := sourceUploadRequest(t, handler, "prod_acme", "original.md", original, map[string]string{"organisation_id": "org_acme"})
	var source model.Source
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &source) != nil {
		t.Fatalf("create: %s", created.Body)
	}
	source.Quarantined = true
	source.Visibility = model.VisibilityPublic
	source, err := backend.UpdateSource(t.Context(), source, source.Revision)
	if err != nil {
		t.Fatal(err)
	}
	const key = "source-replacement-request-0001"
	content := []byte("# Corrected guide\nUse the documented API version.\n")
	var first store.SourceInputReplacementResult
	for range 3 {
		response := replacementUpload(t, handler, source.ID, "corrected.md", content, source.Revision, key, "doko_admin_demo")
		var value store.SourceInputReplacementResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &value) != nil {
			t.Fatalf("replace: %d %s", response.Code, response.Body)
		}
		if first.Source.ID == "" {
			first = value
		}
		if value.CrawlJob.ID != first.CrawlJob.ID || value.Source.Location != first.Source.Location || !value.Source.Quarantined || value.Source.Visibility != source.Visibility {
			t.Fatalf("incorrect recovery: %#v", value)
		}
	}
	for _, token := range []string{"", "doko_customer_demo"} {
		response := replacementUpload(t, handler, source.ID, "corrected.md", content, source.Revision, key, token)
		if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
			t.Fatalf("unauthorized replacement: %d %s", response.Code, response.Body)
		}
	}
	for _, item := range []struct {
		filename string
		content  []byte
		revision int64
		key      string
		status   int
	}{
		{"corrected.md", []byte("Changed bytes"), source.Revision, key, 409},
		{"different.md", content, source.Revision, key, 409},
		{"corrected.md", content, source.Revision + 1, key, 409},
		{"corrected.md", content, first.Source.Revision, "source-replacement-new-0002", 409},
		{"corrected.md", content, 0, "source-replacement-new-0002", 400},
		{"corrected.md", content, source.Revision, "", 400},
		{"corrected.exe", content, source.Revision, key, 415},
		{"corrected.md", []byte{0xff}, source.Revision, key, 400},
	} {
		response := replacementUpload(t, handler, source.ID, item.filename, item.content, item.revision, item.key, "doko_admin_demo")
		if response.Code != item.status {
			t.Fatalf("invalid replacement: %d want %d: %s", response.Code, item.status, response.Body)
		}
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 2 {
		t.Fatalf("files: %v %v", files, err)
	}
	for path, expected := range map[string][]byte{source.Location: original, first.Source.Location: content} {
		stored, err := os.ReadFile(filepath.Join(directory, path))
		if err != nil || string(stored) != string(expected) {
			t.Fatalf("file changed: %q %v", path, err)
		}
	}
	// Recover through the read path even when new uploads are disabled.
	readHandler := httpapi.NewWithOptions(platform.New(backend), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	for _, item := range []struct {
		sourceID, key, token string
		status               int
	}{
		{source.ID, key, "doko_admin_demo", 200}, {source.ID, "uncommitted-replacement-key", "doko_admin_demo", 404},
		{"foreign-source", key, "doko_admin_demo", 404}, {source.ID, key, "", 401}, {source.ID, key, "doko_customer_demo", 401},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/prod_acme/sources/"+item.sourceID+"/upload", nil)
		r.Header.Set("Idempotency-Key", item.key)
		if item.token != "" {
			r.Header.Set("Authorization", "Bearer "+item.token)
		}
		response := httptest.NewRecorder()
		readHandler.ServeHTTP(response, r)
		if response.Code != item.status {
			t.Fatalf("recovery: %d want %d %s", response.Code, item.status, response.Body)
		}
		if item.status == 200 {
			var recovered store.SourceInputReplacementResult
			if json.Unmarshal(response.Body.Bytes(), &recovered) != nil || recovered.CrawlJob.ID != first.CrawlJob.ID {
				t.Fatalf("wrong import: %s", response.Body)
			}
		}
	}
	jobs, err := backend.CrawlJobs(t.Context(), source.ProductID, source.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("recovery queued another job: %#v %v", jobs, err)
	}
}

type uncertainReplacementStore struct{ store.Store }

func (s uncertainReplacementStore) ReplaceSourceInput(ctx context.Context, input store.SourceInputReplacement) (store.SourceInputReplacementResult, error) {
	value, err := s.Store.ReplaceSourceInput(ctx, input)
	if err != nil {
		return value, err
	}
	return value, errors.New("fixture response lost after commit")
}
func TestSourceInputReplacementRetainsUploadOnUncertainCommit(t *testing.T) {
	directory := t.TempDir()
	creation, backend := sourceUploadServer(directory, 5000000)
	response := sourceUploadRequest(t, creation, "prod_acme", "original.md", []byte("Original"), map[string]string{"organisation_id": "org_acme"})
	var source model.Source
	if json.Unmarshal(response.Body.Bytes(), &source) != nil {
		t.Fatal("create")
	}
	handler := httpapi.NewWithOptions(platform.New(uncertainReplacementStore{backend}), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, UploadDirectory: directory})
	response = replacementUpload(t, handler, source.ID, "new.md", []byte("Replacement"), source.Revision, "uncertain-replacement-key-0001", "doko_admin_demo")
	if response.Code == 200 {
		t.Fatal("fixture did not fail")
	}
	current, err := backend.Source(t.Context(), source.ProductID, source.ID)
	if err != nil || current.Location == source.Location {
		t.Fatalf("fixture did not commit: %#v %v", current, err)
	}
	stored, err := os.ReadFile(filepath.Join(directory, current.Location))
	if err != nil || string(stored) != "Replacement" {
		t.Fatalf("committed file discarded: %q %v", stored, err)
	}
}
