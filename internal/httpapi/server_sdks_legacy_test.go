package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestLegacySDKHTTPCompatibilityIsExplicitlyDeprecatedAndDetachesBinding(t *testing.T) {
	t.Parallel()
	memory, service, handler := newDeveloperAssetServer()
	api := createDeveloperAssetAPI(t, service, "legacy-sdk-http", "v1")
	created := request(t, handler, http.MethodPost, "/api/v1/integrations/"+api.ID+"/sdks", "doko_admin_demo", `{
		"ecosystem":"npm","coordinate":"@acme/http-legacy","exact_version":"1.0.0",
		"install_command":"npm install @acme/http-legacy@1.0.0",
		"documentation_url":"https://docs.example.test/http-legacy/1.0.0",
		"source_url":"https://github.com/acme/http-legacy/tree/v1.0.0","visibility":"private"
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create legacy SDK = %d: %s", created.Code, created.Body.String())
	}
	if created.Header().Get("Deprecation") != "true" || !strings.Contains(created.Header().Get("Link"), `rel="successor-version"`) {
		t.Fatalf("legacy create deprecation headers = %#v", created.Header())
	}
	var reference model.SDKReference
	if err := json.Unmarshal(created.Body.Bytes(), &reference); err != nil {
		t.Fatal(err)
	}
	binding, err := memory.APISDKBinding(t.Context(), "prod_acme", api.ID, reference.ID)
	if err != nil || binding.ID != reference.ID {
		t.Fatalf("legacy ID was not preserved as binding ID: %#v, err=%v", binding, err)
	}

	deleted := request(t, handler, http.MethodDelete, "/api/v1/integrations/"+api.ID+"/sdks/"+reference.ID, "doko_admin_demo", "")
	if deleted.Code != http.StatusNoContent || deleted.Header().Get("Deprecation") != "true" {
		t.Fatalf("delete legacy SDK = %d, headers=%#v body=%s", deleted.Code, deleted.Header(), deleted.Body.String())
	}
	binding, err = memory.APISDKBinding(t.Context(), "prod_acme", api.ID, reference.ID)
	if err != nil || binding.State != "detached" {
		t.Fatalf("legacy delete did not detach typed binding: %#v, err=%v", binding, err)
	}
	packages, _ := memory.SDKPackages(t.Context(), "prod_acme")
	if len(packages) != 1 {
		t.Fatalf("legacy delete removed SDK package: %#v", packages)
	}
}
