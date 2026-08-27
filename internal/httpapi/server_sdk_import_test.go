package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type sdkImportHTTPDoer func(*http.Request) (*http.Response, error)

func (doer sdkImportHTTPDoer) Do(request *http.Request) (*http.Response, error) { return doer(request) }

func TestSDKPackageImportEndpointKeepsCredentialWriteOnly(t *testing.T) {
	t.Parallel()
	const token = "npm_PRIVATE_ENDPOINT_TOKEN_DO_NOT_ECHO"
	_, service, handler := newDeveloperAssetServer()
	service.SetSDKImportDoerForTesting(sdkImportHTTPDoer(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("authorization header = %q", request.Header.Get("Authorization"))
		}
		body := `{"name":"@acme/private","versions":{"1.0.0":{"name":"@acme/private","version":"1.0.0","dist":{}}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}))
	created := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-package-imports", "doko_admin_demo", `{
		"ecosystem":"npm","source_kind":"registry","source_url":"https://registry.example.com/@acme/private",
		"coordinate":"@acme/private","exact_version":"1.0.0","visibility":"private",
		"authentication":{"type":"bearer","credential":"`+token+`"}
	}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"exact_version":"1.0.0"`) {
		t.Fatalf("import = %d: %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), token) || strings.Contains(created.Body.String(), "credential") || strings.Contains(created.Body.String(), "authorization") {
		t.Fatalf("credential-bearing input was echoed: %s", created.Body.String())
	}
	retried := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-package-imports", "doko_admin_demo", `{
		"ecosystem":"npm","source_kind":"registry","source_url":"https://registry.example.com/@acme/private",
		"coordinate":"@acme/private","exact_version":"1.0.0","visibility":"private",
		"authentication":{"type":"bearer","credential":"`+token+`"}
	}`)
	if retried.Code != http.StatusOK || !strings.Contains(retried.Body.String(), `"already_imported":true`) {
		t.Fatalf("idempotent import = %d: %s", retried.Code, retried.Body.String())
	}

	unknown := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-package-imports", "doko_admin_demo", `{
		"ecosystem":"npm","source_kind":"registry","source_url":"https://registry.example.com/@acme/private",
		"coordinate":"@acme/private","exact_version":"1.0.0","visibility":"private",
		"authentication":{"type":"none"},"follow_latest":true
	}`)
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "unknown field") {
		t.Fatalf("unknown import field = %d: %s", unknown.Code, unknown.Body.String())
	}
}

var _ platform.SDKImportDoer = sdkImportHTTPDoer(nil)
