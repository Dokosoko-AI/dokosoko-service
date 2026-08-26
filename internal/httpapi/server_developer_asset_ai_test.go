package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func httpDeveloperAssetAIHash(value json.RawMessage) string {
	digest := sha256.Sum256(value)
	return "sha256:" + fmt.Sprintf("%x", digest)
}

func TestDeveloperAssetAIAdvisoryHTTPReadsPersistedRunsWithoutProvider(t *testing.T) {
	t.Parallel()
	memory, _, handler := newDeveloperAssetServer()
	result := json.RawMessage(`{"status":"uncertain","entries":[],"gaps":[{"code":"missing_evidence","evidence_ids":[]}]}`)
	artifact, err := memory.CreateDeveloperAssetAIAdvisoryRun(context.Background(), model.DeveloperAssetAIAdvisoryRun{
		ID: "40000000-0000-4000-8000-000000000001", DeploymentID: "prod_acme",
		PromptKey: "documentation.map_enrichment", PromptVersion: "documentation-map-enrichment-v1",
		ScopeKind: "documentation_publication", ScopeID: "40000000-0000-4000-8000-000000000002",
		ScopeVisibility: model.VisibilityPrivate, IngestionRunID: "40000000-0000-4000-8000-000000000003",
		SourcePublicationID: "40000000-0000-4000-8000-000000000002",
		AllowedEvidenceIDs:  []string{"40000000-0000-4000-8000-000000000004"},
		EvidenceHash:        "sha256:" + strings.Repeat("1", 64), InputHash: "sha256:" + strings.Repeat("2", 64),
		Result: result, ResultHash: httpDeveloperAssetAIHash(result), CreatedBy: "asset-admin", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	listed := request(t, handler, http.MethodGet,
		"/api/v1/developer-assets/ai-advisories?prompt_key=documentation.map_enrichment&scope_id="+artifact.ScopeID,
		"doko_admin_demo", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"items":[`) || !strings.Contains(listed.Body.String(), artifact.ID) {
		t.Fatalf("list persisted advisories = %d: %s", listed.Code, listed.Body.String())
	}
	got := request(t, handler, http.MethodGet, "/api/v1/developer-assets/ai-advisories/"+artifact.ID, "doko_admin_demo", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"result_hash":"`+artifact.ResultHash+`"`) {
		t.Fatalf("get persisted advisory = %d: %s", got.Code, got.Body.String())
	}
	missing := request(t, handler, http.MethodGet, "/api/v1/developer-assets/ai-advisories/missing", "doko_admin_demo", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing advisory = %d: %s", missing.Code, missing.Body.String())
	}
}

func TestDeveloperAssetAIAdvisoryHTTPRejectsUnknownAndInvalidInputs(t *testing.T) {
	t.Parallel()
	_, _, handler := newDeveloperAssetServer()
	unknown := request(t, handler, http.MethodPost, "/api/v1/developer-assets/ai-advisories", "doko_admin_demo",
		`{"prompt_key":"documentation.map_enrichment","source_publication_id":"pub_docs_seed","model":"caller-choice"}`)
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("unknown advisory input = %d: %s", unknown.Code, unknown.Body.String())
	}
	invalid := request(t, handler, http.MethodPost, "/api/v1/developer-assets/ai-advisories", "doko_admin_demo",
		`{"prompt_key":"unregistered.workflow"}`)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"invalid_ai_advisory"`) {
		t.Fatalf("invalid advisory input = %d: %s", invalid.Code, invalid.Body.String())
	}
}
