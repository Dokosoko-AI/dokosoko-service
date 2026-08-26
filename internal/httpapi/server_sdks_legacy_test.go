package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestLegacySDKHTTPProjectionIsReadOnlyAndExplicitlyDeprecated(t *testing.T) {
	t.Parallel()
	memory, service, handler := newDeveloperAssetServer()
	api := createDeveloperAssetAPI(t, service, "legacy-sdk-http", "v1")

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name: "create", method: http.MethodPost, path: "/api/v1/integrations/" + api.ID + "/sdks",
			body: `{"ecosystem":"npm","coordinate":"@acme/legacy","exact_version":"1.0.0","install_command":"npm install @acme/legacy@1.0.0","visibility":"private"}`,
		},
		{
			name: "replace", method: http.MethodPut, path: "/api/v1/integrations/" + api.ID + "/sdks/legacy-reference",
			body: `{"ecosystem":"npm","coordinate":"@acme/legacy","exact_version":"2.0.0","install_command":"npm install @acme/legacy@2.0.0","visibility":"private","revision":1}`,
		},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/integrations/" + api.ID + "/sdks/legacy-reference"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, test.method, test.path, "doko_admin_demo", test.body)
			if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), `"code":"legacy_sdk_mutation_removed"`) {
				t.Fatalf("legacy %s = %d: %s", test.name, response.Code, response.Body.String())
			}
			if response.Header().Get("Deprecation") != "true" || !strings.Contains(response.Header().Get("Link"), `rel="successor-version"`) {
				t.Fatalf("legacy %s deprecation headers = %#v", test.name, response.Header())
			}
		})
	}

	packages, err := memory.SDKPackages(t.Context(), "prod_acme")
	if err != nil || len(packages) != 0 {
		t.Fatalf("removed mutations changed SDK catalog: %#v, err=%v", packages, err)
	}
	bindings, err := memory.APISDKBindings(t.Context(), "prod_acme", api.ID)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("removed mutations changed API resources: %#v, err=%v", bindings, err)
	}

	projection := request(t, handler, http.MethodGet, "/api/v1/integrations/"+api.ID+"/sdks", "doko_admin_demo", "")
	if projection.Code != http.StatusOK || !strings.Contains(projection.Body.String(), `"items":[]`) {
		t.Fatalf("legacy read projection = %d: %s", projection.Code, projection.Body.String())
	}
}
