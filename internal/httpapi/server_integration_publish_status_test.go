package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestIntegrationStatusProvidesExactReviewCandidateOnWire(t *testing.T) {
	handler := httpapi.New(platform.New(store.NewMemory()), "https://dokosoko.example")
	created := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"review-wire","version_key":"v1","display_name":"Review wire","visibility":"private","lifecycle":"draft"}`)
	var integration struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &integration) != nil {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	response := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	var detail struct {
		Status map[string]json.RawMessage `json:"publish_status"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &detail) != nil {
		t.Fatalf("status=%d %s", response.Code, response.Body.String())
	}
	var id, hash string
	var revision int64
	if json.Unmarshal(detail.Status["integration_id"], &id) != nil || json.Unmarshal(detail.Status["candidate_revision"], &revision) != nil || json.Unmarshal(detail.Status["current_manifest_hash"], &hash) != nil || id != integration.ID || revision != integration.Revision+1 || hash == "" {
		t.Fatalf("incomplete exact candidate: %s", response.Body.String())
	}
	preflight := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/preflight", "doko_admin_demo", "")
	var check platform.IntegrationPreflightResult
	if preflight.Code != http.StatusOK || json.Unmarshal(preflight.Body.Bytes(), &check) != nil || check.CandidateManifestHash != hash || check.CandidateRevision != revision {
		t.Fatalf("preflight=%d %s", preflight.Code, preflight.Body.String())
	}
}
