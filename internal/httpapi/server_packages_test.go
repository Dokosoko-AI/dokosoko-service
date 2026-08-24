package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPackageCatalogueHTTPFlowUsesExactRegistryRelease(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"billing-api","version_key":"v1","display_name":"Billing API","description":"Billing","lifecycle":"active"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", w.Code, w.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(w.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/package-artifacts", "doko_admin_demo", `{"name":"Billing Python SDK","description":"Registry SDK","ecosystem":"pypi","coordinate":"acme-billing","purl":"pkg:pypi/acme-billing","registry_url":"https://pypi.org/project/acme-billing","source_url":"https://github.com/acme/billing-python","language":"Python","platform":"server","visibility":"private"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create package = %d: %s", w.Code, w.Body.String())
	}
	var artifact model.PackageArtifact
	if err := json.Unmarshal(w.Body.Bytes(), &artifact); err != nil {
		t.Fatal(err)
	}
	publishBody := `{"version":"2.4.0","purl":"pkg:pypi/acme-billing@2.4.0","install_command":"pip install acme-billing==2.4.0","digest":"sha256:` + strings.Repeat("d", 64) + `","provenance_url":"https://github.com/acme/billing-python/attestations/2.4.0","sbom_url":"https://github.com/acme/billing-python/sbom/2.4.0.json","artifact_revision":1}`
	w = request(t, handler, http.MethodPost, "/api/v1/package-artifacts/"+artifact.ID+"/publish", "doko_admin_demo", publishBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish package = %d: %s", w.Code, w.Body.String())
	}
	var published struct {
		Artifact model.PackageArtifact `json:"artifact"`
		Release  model.PackageRelease  `json:"release"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.Release.Version != "2.4.0" || published.Release.ContentHash == "" || published.Artifact.Revision != 2 {
		t.Fatalf("unexpected publish response: %#v", published)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/packages", "doko_admin_demo", `{"package_release_id":"`+published.Release.ID+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("bind package = %d: %s", w.Code, w.Body.String())
	}
	var binding model.IntegrationPackageBinding
	if err := json.Unmarshal(w.Body.Bytes(), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.Release == nil || binding.Release.ID != published.Release.ID || binding.PackageArtifactID != artifact.ID {
		t.Fatalf("binding is not exact: %#v", binding)
	}
	w = request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID+"/packages", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"package_release_id":"`+published.Release.ID+`"`) {
		t.Fatalf("list binding = %d: %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/package-artifacts/"+artifact.ID+"/retire", "doko_admin_demo", `{"message":"Retired SDK","revision":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale retire = %d: %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/package-artifacts/"+artifact.ID+"/retire", "doko_admin_demo", `{"message":"Retired SDK","revision":2}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"lifecycle":"retired"`) {
		t.Fatalf("retire = %d: %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID+"/packages", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"package_release_id":"`+published.Release.ID+`"`) {
		t.Fatalf("retired binding was not readable = %d: %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/packages", "doko_admin_demo", `{"package_release_id":"`+published.Release.ID+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("retired release was newly bound = %d: %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodDelete, "/api/v1/integrations/"+integration.ID+"/packages/"+artifact.ID, "doko_admin_demo", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("unbind package = %d: %s", w.Code, w.Body.String())
	}
}
