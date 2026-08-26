package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type mcpPreviewEnvelope struct {
	Audience        string          `json:"audience"`
	Method          string          `json:"method"`
	ProtocolVersion string          `json:"protocol_version"`
	Endpoint        string          `json:"endpoint"`
	GeneratedAt     string          `json:"generated_at"`
	Authorization   map[string]any  `json:"authorization"`
	Request         json.RawMessage `json:"request"`
	Response        json.RawMessage `json:"response"`
}

func decodedJSON(t *testing.T, value []byte) any {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, value)
	}
	return decoded
}

func TestMCPPreviewUsesExactPrivateDiscoveryResponses(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)
	methods := []string{"server/discover", "tools/list", "resources/list", "resources/templates/list"}
	for _, method := range methods {
		method := method
		t.Run(method, func(t *testing.T) {
			liveBody := `{"jsonrpc":"2.0","id":"preview","method":"` + method + `","params":{}}`
			live := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", liveBody)
			if live.Code != http.StatusOK {
				t.Fatalf("live %s = %d: %s", method, live.Code, live.Body.String())
			}

			preview := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/mcp-preview?audience=private&method="+url.QueryEscape(method), "doko_admin_demo", "")
			if preview.Code != http.StatusOK {
				t.Fatalf("preview %s = %d: %s", method, preview.Code, preview.Body.String())
			}
			var envelope mcpPreviewEnvelope
			if err := json.Unmarshal(preview.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Audience != "private" || envelope.Method != method || envelope.ProtocolVersion != "2026-07-28" || envelope.Endpoint != "/mcp" || envelope.GeneratedAt == "" {
				t.Fatalf("unexpected preview context: %#v", envelope)
			}
			if envelope.Authorization["mode"] != "simulated" {
				t.Fatalf("authorization context = %#v", envelope.Authorization)
			}
			if !reflect.DeepEqual(decodedJSON(t, envelope.Response), decodedJSON(t, live.Body.Bytes())) {
				t.Fatalf("preview drifted from live %s\npreview: %s\nlive: %s", method, envelope.Response, live.Body.String())
			}
		})
	}
}

func TestMCPPreviewIsAdminOnlyReadOnlyAndContextExplicit(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)
	path := "/api/v1/products/prod_acme/mcp-preview?audience=private&method=tools%2Flist&grant=write.records&grant=read.records"

	unauthorized := request(t, handler, http.MethodGet, path, "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized preview = %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	wrongMethod := request(t, handler, http.MethodPost, path, "doko_admin_demo", `{}`)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST preview = %d: %s", wrongMethod.Code, wrongMethod.Body.String())
	}
	missingProduct := request(t, handler, http.MethodGet, "/api/v1/products/prod_missing/mcp-preview", "doko_admin_demo", "")
	if missingProduct.Code != http.StatusNotFound {
		t.Fatalf("missing-product preview = %d: %s", missingProduct.Code, missingProduct.Body.String())
	}

	preview := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if preview.Code != http.StatusOK {
		t.Fatalf("grant preview = %d: %s", preview.Code, preview.Body.String())
	}
	var envelope mcpPreviewEnvelope
	if err := json.Unmarshal(preview.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	grants, ok := envelope.Authorization["grants"].([]any)
	if !ok || len(grants) != 2 || grants[0] != "read.records" || grants[1] != "write.records" {
		t.Fatalf("sorted explicit grants = %#v", envelope.Authorization["grants"])
	}
	if strings.Contains(string(envelope.Response), "tools/call") {
		t.Fatalf("preview unexpectedly included an executable call: %s", envelope.Response)
	}

	publicPreview := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/mcp-preview?audience=public&method=tools%2Flist", "doko_admin_demo", "")
	if publicPreview.Code != http.StatusOK {
		t.Fatalf("public preview = %d: %s", publicPreview.Code, publicPreview.Body.String())
	}
	if err := json.Unmarshal(publicPreview.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Audience != "public" || envelope.Endpoint != "/mcp/public" || envelope.Authorization["mode"] != "anonymous" {
		t.Fatalf("unexpected public preview context: %#v", envelope)
	}

	publicWithGrant := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/mcp-preview?audience=public&method=tools%2Flist&grant=read.records", "doko_admin_demo", "")
	if publicWithGrant.Code != http.StatusBadRequest {
		t.Fatalf("public grant preview = %d: %s", publicWithGrant.Code, publicWithGrant.Body.String())
	}
	toolCall := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/mcp-preview?audience=private&method=tools%2Fcall", "doko_admin_demo", "")
	if toolCall.Code != http.StatusBadRequest {
		t.Fatalf("tools/call preview = %d: %s", toolCall.Code, toolCall.Body.String())
	}
}
