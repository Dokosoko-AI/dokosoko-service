package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type accessResolverStub struct{}

func (accessResolverStub) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("93.184.216.34")}, nil
}

type accessDoerStub struct{}

func (accessDoerStub) Do(request *http.Request) (*http.Response, error) {
	body := `{"external_id":"provider-instance-1","display_name":"Voice sandbox","state":"active"}`
	if request.URL.Path == "/v1/authorize" {
		body = `{"allowed":true}`
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func newServer() http.Handler {
	return httpapi.New(platform.New(store.NewMemory()), "https://dokosoko.example")
}

func newCatalogServer(t *testing.T) http.Handler {
	t.Helper()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x53}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "idp_catalog", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://identity.vendor.example", ClientID: "vendor-client", DelegatedAPIOrigin: "https://api.vendor.example", State: "active"}); err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	return httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", Reporting: reporting.New(memory, vault), AllowDemoTokens: true})
}

func newProductionAuthServer(t *testing.T) http.Handler {
	t.Helper()
	memory := store.NewMemory()
	manager, err := auth.New(memory, auth.Config{
		SetupToken: "setup-token-for-tests",
		MasterKey:  bytes.Repeat([]byte{0x42}, 32),
		PublicURL:  "https://dokosoko.example",
		SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.NewWithOptions(platform.New(memory), httpapi.Options{
		BaseURL:         "https://dokosoko.example",
		Auth:            manager,
		AllowDemoTokens: false,
	})
}

func requestWithCookies(t *testing.T, handler http.Handler, method, path, body string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Origin", "https://dokosoko.example")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func request(t *testing.T, handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	mcpMethod := ""
	mcpName := ""
	if method == http.MethodPost && (path == "/mcp" || path == "/mcp/public") && body != "" {
		var envelope map[string]any
		if err := json.Unmarshal([]byte(body), &envelope); err != nil {
			t.Fatal(err)
		}
		mcpMethod, _ = envelope["method"].(string)
		params, _ := envelope["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
			envelope["params"] = params
		}
		meta, _ := params["_meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		meta["io.modelcontextprotocol/protocolVersion"] = "2026-07-28"
		params["_meta"] = meta
		mcpName, _ = params["name"].(string)
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		body = string(encoded)
	}
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if mcpMethod != "" {
		r.Header.Set("MCP-Protocol-Version", "2026-07-28")
		r.Header.Set("Mcp-Method", mcpMethod)
		if mcpName != "" {
			r.Header.Set("Mcp-Name", mcpName)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestMCPRejectsPreV2Requests(t *testing.T) {
	t.Parallel()
	handler := newServer()
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	r.Header.Set("Authorization", "Bearer doko_private_demo")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Stateless MCPv2 Only") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestMCPPublishesOneCanonicalOAuthResource(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	metadata := request(t, handler, http.MethodGet, "/.well-known/oauth-protected-resource/mcp", "", "")
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"resource":"https://dokosoko.example/mcp"`) {
		t.Fatalf("protected resource metadata status = %d, body = %s", metadata.Code, metadata.Body.String())
	}
	private := request(t, handler, http.MethodPost, "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if private.Code != http.StatusUnauthorized || private.Header().Get("WWW-Authenticate") != `Bearer resource_metadata="https://dokosoko.example/.well-known/oauth-protected-resource/mcp", scope="mcp:private"` {
		t.Fatalf("private challenge status = %d, header = %q", private.Code, private.Header().Get("WWW-Authenticate"))
	}
	legacy := request(t, handler, http.MethodPost, "/mcp/prod_acme", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy endpoint status = %d, body = %s", legacy.Code, legacy.Body.String())
	}
}

func TestIntegrationCatalogAccessAndSupportAdminFlow(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	w := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"voice-api","version_key":"v2","display_name":"Voice API v2","description":"Voice calls","lifecycle":"active"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create integration status = %d, body = %s", w.Code, w.Body.String())
	}
	var integration model.Integration
	if err := json.Unmarshal(w.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}

	w = request(t, handler, http.MethodPost, "/api/v1/resource-sets", "doko_admin_demo", `{"kind":"api","name":"Voice API","description":"Shared provider operations","manifest":[{"name":"calls.create","path":"/v2/calls"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create resource set status = %d, body = %s", w.Code, w.Body.String())
	}
	var resourceSet model.ResourceSet
	if err := json.Unmarshal(w.Body.Bytes(), &resourceSet); err != nil {
		t.Fatal(err)
	}

	w = request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/resource-sets", "doko_admin_demo", `{"resource_set_id":"`+resourceSet.ID+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("attach resource set status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/publish", "doko_admin_demo", `{}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"manifest_hash":"sha256:`) {
		t.Fatalf("publish integration status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/api/v1/access-definitions", "doko_admin_demo", `{"service_key":"voice-access","name":"Voice access","instance_cardinality":"one","instance_label_singular":"account","instance_label_plural":"accounts","credential_scope":"connection","management_auth_type":"none","operations":{}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create access definition status = %d, body = %s", w.Code, w.Body.String())
	}
	var definition model.AccessDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &definition); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/access-connections", "doko_admin_demo", `{"access_definition_id":"`+definition.ID+`","environment_id":"env_prod","name":"Voice production","base_url":"https://provider.example","config":{},"integration_ids":["`+integration.ID+`"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create access connection status = %d, body = %s", w.Code, w.Body.String())
	}
	var connection model.AccessConnection
	if err := json.Unmarshal(w.Body.Bytes(), &connection); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/access-connections", "doko_admin_demo", `{"access_connection_ids":[]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("clear Integration access status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/access-connections", "doko_admin_demo", `{"access_connection_ids":["`+connection.ID+`"]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), connection.ID) {
		t.Fatalf("assign Integration access status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/api/v1/backend-connections", "doko_admin_demo", `{"name":"Default support backend","base_url":"https://api.vendor.example","authentication_type":"bearer","credential":"default-secret","state":"active"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create default backend connection status = %d, body = %s", w.Code, w.Body.String())
	}
	var defaultBackend model.BackendConnection
	if err := json.Unmarshal(w.Body.Bytes(), &defaultBackend); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/backend-connections", "doko_admin_demo", `{"name":"Voice support backend","base_url":"https://api.vendor.example","authentication_type":"bearer","credential":"voice-secret","state":"active"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create voice backend connection status = %d, body = %s", w.Code, w.Body.String())
	}
	var voiceBackend model.BackendConnection
	if err := json.Unmarshal(w.Body.Bytes(), &voiceBackend); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/support-routes", "doko_admin_demo", `{"name":"Default support","is_default":true,"bug_reports_enabled":true,"feedback_enabled":true,"backend_connection_id":"`+defaultBackend.ID+`","retention_days":30,"state":"active","integration_ids":[]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create support route status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/support-routes", "doko_admin_demo", `{"name":"Voice support","is_default":false,"bug_reports_enabled":true,"feedback_enabled":true,"backend_connection_id":"`+voiceBackend.ID+`","retention_days":30,"state":"active","integration_ids":["`+integration.ID+`"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create Integration support route status = %d, body = %s", w.Code, w.Body.String())
	}
	var supportRoute model.SupportRoute
	if err := json.Unmarshal(w.Body.Bytes(), &supportRoute); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/support-route", "doko_admin_demo", `{"support_route_id":"`+supportRoute.ID+`"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), supportRoute.ID) {
		t.Fatalf("assign Integration support route status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"publish_status"`) || !strings.Contains(w.Body.String(), `"has_changes":true`) || !strings.Contains(w.Body.String(), `"access_connection_ids"`) {
		t.Fatalf("Integration publish status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/publish", "doko_admin_demo", `{}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish updated Integration status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"has_changes":false`) || !strings.Contains(w.Body.String(), `"latest_revision"`) {
		t.Fatalf("clean Integration publish status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/api/v1/integrations", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), resourceSet.ID) || !strings.Contains(w.Body.String(), `"access_connection_ids":[`) {
		t.Fatalf("hydrated integration list status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPrivateMCPDiscoversAndExecutesProviderAccessWithoutLegacyProjects(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	integration, err := service.CreateIntegration(t.Context(), platform.IntegrationInput{FamilyKey: "voice-api", VersionKey: "v2", DisplayName: "Voice API v2", Lifecycle: "active"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	operations := json.RawMessage(`{"max_ttl_seconds":3600,"credential_storage_mode":"one_time","authorize":{"method":"POST","path":"/v1/authorize"},"instances.create":{"method":"POST","path":"/v1/instances"},"credentials.create":{"method":"POST","path":"/v1/credentials"},"credentials.revoke":{"method":"POST","path":"/v1/credentials/{credential_id}/revoke"}}`)
	definition, err := service.CreateAccessDefinition(t.Context(), platform.AccessDefinitionInput{ServiceKey: "voice-access", Name: "Voice access", InstanceCardinality: "many", InstanceLabelSingular: "workspace", InstanceLabelPlural: "workspaces", CredentialScope: "instance", ManagementAuthType: "none", Operations: operations}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := service.CreateAccessConnection(t.Context(), platform.AccessConnectionInput{AccessDefinitionID: definition.ID, EnvironmentID: "env_prod", Name: "Voice production", BaseURL: "https://provider.example", IntegrationIDs: []string{integration.ID}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AccessRuntime: accessruntime.New(memory, vault, accessResolverStub{}, accessDoerStub{}), AllowDemoTokens: true})

	w := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"access.instances.create"`) || !strings.Contains(w.Body.String(), `"instance_label":"workspace"`) {
		t.Fatalf("access discovery status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"name":"projects.create"`) || strings.Contains(w.Body.String(), `"name":"credentials.issue"`) {
		t.Fatalf("legacy DokoSoko project tools must not be discoverable: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"access.instances.create","arguments":{"connection_id":"`+connection.ID+`","integration_id":"`+integration.ID+`","environment_id":"env_prod","display_name":"Voice sandbox","idempotency_key":"mcp-access-instance-0001"}}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"external_id":"provider-instance-1"`) {
		t.Fatalf("access instance call status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPublicMCPIsAnonymousButOffByDefault(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "public_mcp_unavailable") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAIProductBuilderAPIProducesReviewAndPublishesDefinition(t *testing.T) {
	t.Parallel()
	handler := newServer()
	body := `{"inputs":[{"kind":"openapi","name":"Voice API","location":"https://api.example.com/voice/v3/openapi.yaml","version":"v3"},{"kind":"docs","name":"Voice documentation","location":"https://docs.example.com/voice/v3","version":"v3"}]}`
	w := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/product-builds", "doko_admin_demo", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("build status = %d, body = %s", w.Code, w.Body.String())
	}
	var build struct {
		ID           string `json:"id"`
		State        string `json:"state"`
		AnalysisMode string `json:"analysis_mode"`
		Proposal     struct {
			Components []any `json:"components"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &build); err != nil {
		t.Fatal(err)
	}
	if build.ID == "" || build.State != "review" || build.AnalysisMode != "automatic" || len(build.Proposal.Components) == 0 {
		t.Fatalf("build = %#v", build)
	}

	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/product-builds/"+build.ID+"/publish", "doko_admin_demo", `{}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"published"`) {
		t.Fatalf("publish status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/definition", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"source_build_id":"`+build.ID+`"`) {
		t.Fatalf("definition status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestMCPDiscoversTheEffectiveDokosokoProductVersion(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_catalog", RequestID: "req_catalog"}
	product, err := memory.Product(t.Context(), "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	product, err = service.UpdateProductSettings(t.Context(), product.ID, "Build voice and messaging integrations with version-matched APIs, SDKs, documentation, and authorized tools.", "latest", product.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	build, err := service.BuildProductDefinition(t.Context(), product.ID, []model.ProductBuildInput{
		{Kind: "openapi", Name: "Voice API", Location: "https://api.example.com/voice/v3/openapi.yaml", Version: "v3"},
		{Kind: "openapi", Name: "Messages API", Location: "https://api.example.com/messages/v2/openapi.yaml", Version: "v2"},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := service.PublishProductDefinition(t.Context(), product.ID, build.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProductVersion(t.Context(), product.ID, platform.ProductVersionInput{Version: "2026.8", ProfileID: definition.Profiles[0].ID, IsLatest: true, IsLTS: true}, actor); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "https://dokosoko.example")

	w := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"product_name":"Acme Platform"`) || !strings.Contains(w.Body.String(), `"version":"2026.8"`) || !strings.Contains(w.Body.String(), `"release":"v3"`) || !strings.Contains(w.Body.String(), `"release":"v2"`) || !strings.Contains(w.Body.String(), `"catalogRevision":`) || !strings.Contains(w.Body.String(), `"manifestHash":"sha256:`) {
		t.Fatalf("discover status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"deployment.get_manifest"`) || !strings.Contains(w.Body.String(), `"name":"deployment.releases.list"`) || !strings.Contains(w.Body.String(), `"com.dokosoko/deploymentRelease"`) || !strings.Contains(w.Body.String(), `"is_lts":true`) {
		t.Fatalf("tools/list status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"name":"projects.create"`) || strings.Contains(w.Body.String(), `"name":"credentials.issue"`) {
		t.Fatalf("legacy DokoSoko project tools must not be discoverable: %s", w.Body.String())
	}
}

func TestProductVersionAdminAPIManagesChannelsAndPins(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	account, err := memory.ResolveCustomerAccount(t.Context(), identity.CustomerAccount{ID: "account_contoso", OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: "https://identity.vendor.example", ExternalID: "contoso", State: "active", LastAuthenticatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(platform.New(memory), "https://dokosoko.example")
	w := request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme", "doko_admin_demo", `{"description":"Build supported Acme API integrations with version-matched documentation and tools.","default_version_policy":"latest","revision":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/product-builds", "doko_admin_demo", `{"inputs":[{"kind":"openapi","name":"Voice API","location":"https://api.example.com/voice/v3/openapi.yaml","version":"v3"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("build status = %d, body = %s", w.Code, w.Body.String())
	}
	var build struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &build); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/product-builds/"+build.ID+"/publish", "doko_admin_demo", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body = %s", w.Code, w.Body.String())
	}
	var definition struct {
		Profiles []struct {
			ID string `json:"id"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &definition); err != nil || len(definition.Profiles) == 0 {
		t.Fatalf("definition = %#v, err = %v", definition, err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/versions", "doko_admin_demo", fmt.Sprintf(`{"version":"2026.8","profile_id":%q,"is_latest":true,"is_lts":true}`, definition.Profiles[0].ID))
	if w.Code != http.StatusCreated {
		t.Fatalf("version status = %d, body = %s", w.Code, w.Body.String())
	}
	var version struct {
		ID           string `json:"id"`
		Version      string `json:"version"`
		ManifestHash string `json:"manifest_hash"`
		Revision     int64  `json:"revision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version.ManifestHash, "sha256:") {
		t.Fatalf("manifest hash = %q", version.ManifestHash)
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/versions/"+version.ID+"/diff", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"summary":"Initial product release"`) {
		t.Fatalf("diff status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/version-pins", "doko_admin_demo", fmt.Sprintf(`{"customer_account_id":%q,"product_version_id":%q,"reason":"Production stability"}`, account.ID, version.ID))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"customer_account_id":"`+account.ID+`"`) || !strings.Contains(w.Body.String(), `"product_version":"2026.8"`) {
		t.Fatalf("pin status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/versions", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"is_latest":true`) || !strings.Contains(w.Body.String(), `"is_lts":true`) {
		t.Fatalf("versions status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/version-pins", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"customer_account_id":"`+account.ID+`"`) {
		t.Fatalf("pins status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/version-pins/history", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"action":"created"`) || !strings.Contains(w.Body.String(), `"scope":"customer"`) {
		t.Fatalf("pin history status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/installations", "doko_admin_demo", fmt.Sprintf(`{"customer_account_id":%q,"environment_id":"env_prod","external_id":"contoso-prod","name":"Contoso production","state":"active","revision":0}`, account.ID))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"external_id":"contoso-prod"`) {
		t.Fatalf("installation status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/versions/"+version.ID+"/impact", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"customer_pins":1`) {
		t.Fatalf("impact status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/versions/"+version.ID+"/reconcile", "doko_admin_demo", fmt.Sprintf(`{"revision":%d}`, version.Revision))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"drift_status":"healthy"`) {
		t.Fatalf("reconcile status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPublicTransitionWarningsAreEnforcedByAPI(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/sources/src_docs/visibility", "doko_admin_demo", `{"visibility":"public","revision":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "public_confirmation_required") || !strings.Contains(w.Body.String(), "without authentication") {
		t.Fatalf("body = %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"revision":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

}

func TestPublicMCPOnlyReturnsExplicitlyPublicRecordsAndReadOnlyTools(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/sources/src_docs/visibility", "doko_admin_demo", `{"visibility":"public","acknowledge_public":true,"revision":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("source status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("distribution status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "provision_resource") {
		t.Fatalf("privileged tool leaked: %s", w.Body.String())
	}
	for _, expected := range []string{"search_knowledge"} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("missing tool %q: %s", expected, w.Body.String())
		}
	}

	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"API key"}}}`)
	if !strings.Contains(w.Body.String(), "Create an API key") {
		t.Fatalf("public record missing: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "operator-only") {
		t.Fatalf("private record leaked: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"provision_resource","arguments":{}}}`)
	if !strings.Contains(w.Body.String(), "not available on Public MCP") {
		t.Fatalf("privileged tool was not rejected: %s", w.Body.String())
	}

}

func TestMakingSourcePrivateRemovesItImmediately(t *testing.T) {
	t.Parallel()
	handler := newServer()
	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/sources/src_docs/visibility", "doko_admin_demo", `{"visibility":"public","acknowledge_public":true,"revision":1}`)
	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`)

	w := request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/sources/src_docs/visibility", "doko_admin_demo", `{"visibility":"private","revision":2}`)
	if w.Code != http.StatusOK {
		t.Fatalf("private transition status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"API key"}}}`)
	if strings.Contains(w.Body.String(), "Create an API key") {
		t.Fatalf("newly private source still visible: %s", w.Body.String())
	}
}

func TestPrivateMCPRequiresAuthentication(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "search_knowledge") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCustomerAccountsUseStableCursorPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	base := time.Date(2026, time.August, 22, 0, 0, 0, 0, time.UTC)
	for index, id := range []string{"account-a", "account-b", "account-c"} {
		if _, err := memory.ResolveCustomerAccount(ctx, identity.CustomerAccount{
			ID:                  id,
			OrganisationID:      "org_acme",
			ProductID:           "prod_acme",
			Issuer:              "https://identity.vendor.example",
			ExternalID:          fmt.Sprintf("customer-%d", index),
			State:               "active",
			LastAuthenticatedAt: base.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	handler := httpapi.NewWithOptions(platform.New(memory), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	type page struct {
		Items   []identity.CustomerAccount `json:"items"`
		HasMore bool                       `json:"has_more"`
	}
	first := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/customer-accounts?limit=2", "doko_admin_demo", "")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstPage page
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if !firstPage.HasMore || len(firstPage.Items) != 2 || firstPage.Items[0].ID != "account-c" || firstPage.Items[1].ID != "account-b" {
		t.Fatalf("unexpected first page: %#v", firstPage)
	}

	second := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/customer-accounts?limit=2&starting_after=account-b", "doko_admin_demo", "")
	var secondPage page
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d, body = %s", second.Code, second.Body.String())
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatal(err)
	}
	if secondPage.HasMore || len(secondPage.Items) != 1 || secondPage.Items[0].ID != "account-a" {
		t.Fatalf("unexpected second page: %#v", secondPage)
	}

	invalid := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/customer-accounts?starting_after=missing", "doko_admin_demo", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
}

func TestSupportReportingToolsRequireConsentQueueEncryptedReportsAndStayPrivate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x39}, 32))
	if err != nil {
		t.Fatal(err)
	}
	reporter := reporting.New(memory, vault)
	backend, err := platform.NewWithVault(memory, vault).CreateBackendConnection(ctx, platform.BackendConnectionInput{Name: "Support backend", BaseURL: "https://api.vendor.example", AuthenticationType: "bearer", Credential: "support-delivery-secret", State: "active"}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reporter.SaveRoute(ctx, "prod_acme", "", reporting.RouteInput{Name: "Default support", IsDefault: true, BugReportsEnabled: true, FeedbackEnabled: true, BackendConnectionID: backend.ID, RetentionDays: 30, State: "active"}, "root-test", "req-config"); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithOptions(platform.New(memory), httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, Reporting: reporter})

	w := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "do not submit automatically") || !strings.Contains(w.Body.String(), "obtain explicit approval") {
		t.Fatalf("reporting agent policy missing from discovery: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"support.report_bug"`) || !strings.Contains(w.Body.String(), `"name":"support.submit_feedback"`) || !strings.Contains(w.Body.String(), `"com.dokosoko/confirmationRequired":true`) || !strings.Contains(w.Body.String(), "never invent ratings") {
		t.Fatalf("support reporting definitions missing: status=%d body=%s", w.Code, w.Body.String())
	}

	bugArgs := `"summary":"Connector failure","description":"The connector returned an unexpected result.","related_tool":"access.credentials.create","idempotency_key":"bug-report-idempotency-http-1"`
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"support.report_bug","arguments":{`+bugArgs+`}}}`)
	if !strings.Contains(w.Body.String(), "Explicit user confirmation is required") {
		t.Fatalf("unconfirmed report was not denied: %s", w.Body.String())
	}
	values, _, err := memory.ReportSubmissions(ctx, "prod_acme", "", 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("unconfirmed report was persisted: values=%#v err=%v", values, err)
	}

	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"support.report_bug","arguments":{`+bugArgs+`},"_meta":{"confirmed":true}}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"pending"`) || !strings.Contains(w.Body.String(), `"submission_id"`) || strings.Contains(w.Body.String(), "unexpected result") {
		t.Fatalf("confirmed bug was not queued safely: status=%d body=%s", w.Code, w.Body.String())
	}
	values, _, err = memory.ReportSubmissions(ctx, "prod_acme", "", 10)
	if err != nil || len(values) != 1 || values[0].State != "pending" || bytes.Contains(values[0].PayloadCiphertext, []byte("Connector failure")) {
		t.Fatalf("encrypted queued bug missing: values=%#v err=%v", values, err)
	}
	w = request(t, handler, http.MethodGet, "/api/v1/support-submissions", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Connector failure") || strings.Contains(w.Body.String(), "unexpected result") || strings.Contains(w.Body.String(), `"content"`) {
		t.Fatalf("inbox list did not limit decrypted content: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/support-submissions/"+values[0].ID, "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "unexpected result") || !strings.Contains(w.Body.String(), `"content"`) || !strings.Contains(w.Body.String(), `"support_route_id"`) {
		t.Fatalf("on-demand report detail missing: status=%d body=%s", w.Code, w.Body.String())
	}
	failed := values[0]
	failed.State, failed.NextAttemptAt, failed.DeliveryStartedAt = "failed", nil, nil
	if _, err := memory.UpdateReportSubmissionDelivery(ctx, failed); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/support-submissions/"+values[0].ID+"/delivery-attempts", "doko_admin_demo", "")
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"state":"pending"`) {
		t.Fatalf("delivery attempt was not accepted: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/support-submissions/"+values[0].ID+"/delivery-attempts", "doko_admin_demo", "")
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"state":"pending"`) {
		t.Fatalf("delivery-attempt retry was not idempotent: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/report-submissions", "doko_admin_demo", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy product-scoped support submissions must not remain addressable: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/reporting", "doko_admin_demo", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy reporting configuration must not remain addressable: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/report-submissions/"+values[0].ID+"/retry", "doko_admin_demo", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy retry action must not remain addressable: status=%d body=%s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"support.submit_feedback","arguments":{"message":"The connector workflow was excellent.","category":"usability","idempotency_key":"feedback-idempotency-http-1"},"_meta":{"confirmed":true}}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("confirmed feedback was not queued: status=%d body=%s", w.Code, w.Body.String())
	}

	product, _ := memory.Product(ctx, "prod_acme")
	product.PublicMCPEnabled = true
	if _, err := memory.UpdateProduct(ctx, product, product.Revision); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":6,"method":"tools/list","params":{}}`)
	if strings.Contains(w.Body.String(), "support.report_bug") || strings.Contains(w.Body.String(), "support.submit_feedback") {
		t.Fatalf("support reporting tools leaked to Public MCP: %s", w.Body.String())
	}
}

func TestWidgetSnippetsContainNoSecretAndPublicLoaderFollowsMCPState(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/widgets", "doko_admin_demo", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "token") || strings.Contains(w.Body.String(), "doko_private_demo") {
		t.Fatalf("snippet response contains credential material: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) || !strings.Contains(w.Body.String(), "/widgets/prod_acme/public.js") || !strings.Contains(w.Body.String(), "/widgets/prod_acme/private.js") {
		t.Fatalf("widget response is incomplete: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/widgets/prod_acme/public.js", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled public widget status = %d", w.Code)
	}
	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`)
	w = request(t, handler, http.MethodGet, "/widgets/prod_acme/public.js", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "dokosoko:open") {
		t.Fatalf("enabled public widget status = %d, body = %s", w.Code, w.Body.String())
	}
	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":false,"revision":2}`)
	w = request(t, handler, http.MethodGet, "/widgets/prod_acme/public.js", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("re-disabled public widget status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestGoServiceServesStaticConsoleWithoutShadowingAPI(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<!doctype html><title>DokoSoko console</title><script>window.__dokosoko_bootstrap=true</script>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "index.rsc"), []byte("0:{\"__route\":\"route:/\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithUI(platform.New(store.NewMemory()), "https://dokosoko.example", directory)

	w := request(t, handler, http.MethodGet, "/", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "DokoSoko console") {
		t.Fatalf("console status = %d, body = %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/overview", "/integrations/documentation", "/operations/reporting", "/integration/int_voice"} {
		w = request(t, handler, http.MethodGet, path, "", "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "DokoSoko console") {
			t.Fatalf("console route %s status = %d, body = %s", path, w.Code, w.Body.String())
		}
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("missing console CSP: %q", w.Header().Get("Content-Security-Policy"))
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "'sha256-") || strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("console CSP must hash its static bootstrap scripts: %q", w.Header().Get("Content-Security-Policy"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "RSC") || !strings.Contains(w.Header().Get("Vary"), "Accept") {
		t.Fatalf("console HTML does not vary from RSC: %q", w.Header().Get("Vary"))
	}

	rscRequest := httptest.NewRequest(http.MethodGet, "/?__rsc=test", nil)
	rscRequest.Header.Set("RSC", "1")
	rscRequest.Header.Set("Accept", "text/x-component")
	rscRequest.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, rscRequest)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/x-component" || !strings.Contains(w.Body.String(), `"route:/"`) {
		t.Fatalf("RSC bootstrap status = %d, content-type = %q, body = %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("RSC bootstrap cache-control = %q", w.Header().Get("Cache-Control"))
	}

	missingRSCRequest := httptest.NewRequest(http.MethodGet, "/missing?__rsc=test", nil)
	missingRSCRequest.Header.Set("RSC", "1")
	missingRSCRequest.Header.Set("Accept", "text/x-component")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, missingRSCRequest)
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "DokoSoko console") {
		t.Fatalf("missing RSC route fell back to HTML: status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/healthz", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Fatalf("health status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPublicMCPHasAnAnonymousRequestBudget(t *testing.T) {
	t.Parallel()
	handler := newServer()
	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`)

	for index := 0; index < 120; index++ {
		w := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, body = %s", index+1, w.Code, w.Body.String())
		}
	}
	w := request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "rate_limited") {
		t.Fatalf("over-budget status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestFirstRunSetupCreatesMFARootAndCookieSession(t *testing.T) {
	t.Parallel()
	handler := newProductionAuthServer(t)

	w := request(t, handler, http.MethodGet, "/api/v1/setup/status", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"setup_complete":false`) {
		t.Fatalf("initial status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/setup/begin", "wrong-token", `{"email":"root@example.com","display_name":"Root Operator","password":"Correct-Horse-47!Battery"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong setup token status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/setup/begin", "setup-token-for-tests", `{"email":"root@example.com","password":"Correct-Horse-47!Battery"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("begin setup status = %d, body = %s", w.Code, w.Body.String())
	}
	var enrollment struct {
		ID     string `json:"enrollment_id"`
		Secret string `json:"totp_secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	code := auth.TOTP(secret, time.Now().UTC())
	w = request(t, handler, http.MethodPost, "/api/v1/setup/complete", "", fmt.Sprintf(`{"enrollment_id":%q,"code":%q}`, enrollment.ID, code))
	if w.Code != http.StatusCreated {
		t.Fatalf("complete setup status = %d, body = %s", w.Code, w.Body.String())
	}
	var completed struct {
		CSRFToken     string   `json:"csrf_token"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.CSRFToken == "" || len(completed.RecoveryCodes) != 10 {
		t.Fatalf("setup did not return CSRF and recovery codes: %s", w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("session cookie count = %d", len(cookies))
	}
	if cookies[0].Name != "dokosoko_session" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe session cookie: %#v", cookies[0])
	}

	w = requestWithCookies(t, handler, http.MethodGet, "/api/v1/products/prod_acme/distribution", "", cookies, "")
	if w.Code != http.StatusOK {
		t.Fatalf("session GET status = %d, body = %s", w.Code, w.Body.String())
	}
	w = requestWithCookies(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`, cookies, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d, body = %s", w.Code, w.Body.String())
	}
	w = requestWithCookies(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`, cookies, completed.CSRFToken)
	if w.Code != http.StatusOK {
		t.Fatalf("valid CSRF status = %d, body = %s", w.Code, w.Body.String())
	}
	w = requestWithCookies(t, handler, http.MethodGet, "/api/v1/root/users", "", cookies, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "root@example.com") || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("root users status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("production demo token status = %d", w.Code)
	}
	w = request(t, handler, http.MethodGet, "/api/v1/setup/status", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"setup_complete":true`) {
		t.Fatalf("completed status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestWorkspaceCanBeConfiguredEntirelyThroughAdminAPI(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/api/v1/organisations", "doko_admin_demo", `{"name":"Example Company","slug":"example-company"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create organisation status = %d, body = %s", w.Code, w.Body.String())
	}
	var organisation struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &organisation); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/organisations/"+organisation.ID+"/products", "doko_admin_demo", `{"name":"Example Platform","slug":"example-platform"}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"public_mcp_enabled":false`) {
		t.Fatalf("create product status = %d, body = %s", w.Code, w.Body.String())
	}
	var product struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &product); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/"+product.ID+"/environments", "doko_admin_demo", fmt.Sprintf(`{"organisation_id":%q,"name":"Production","slug":"production","is_production":true}`, organisation.ID))
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"is_production":true`) {
		t.Fatalf("create environment status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/organisations/"+organisation.ID+"/products", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Example Platform") {
		t.Fatalf("list products status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/"+product.ID+"/sources", "doko_admin_demo", fmt.Sprintf(`{"organisation_id":%q,"name":"Developer docs","kind":"website","location":"https://docs.example.com"}`, organisation.ID))
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"visibility":"private"`) || !strings.Contains(w.Body.String(), `"published":false`) {
		t.Fatalf("create source status = %d, body = %s", w.Code, w.Body.String())
	}
	var source struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/"+product.ID+"/sources/"+source.ID+"/crawl", "doko_admin_demo", "")
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"state":"queued"`) {
		t.Fatalf("queue crawl status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/"+product.ID+"/sources/"+source.ID+"/crawl", "doko_admin_demo", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "crawl_already_active") {
		t.Fatalf("duplicate crawl status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestIntegrationRunsFeedValidatedAnalyticsAndScopedAudit(t *testing.T) {
	t.Parallel()
	handler := newServer()

	w := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/integration-runs", "doko_admin_demo", `{"environment_id":"env_prod","requested_outcome":"Install the SDK and validate authentication"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("start run status = %d, body = %s", w.Code, w.Body.String())
	}
	var run struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.State != "running" {
		t.Fatalf("unexpected run: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/integration-runs/"+run.ID+"/complete", "doko_admin_demo", `{"reported_success":true,"validated_success":true}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"succeeded"`) {
		t.Fatalf("complete run status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/integration-runs/"+run.ID+"/complete", "doko_admin_demo", `{"validated_success":true}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate completion status = %d, body = %s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/analytics", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"integration_runs":1`) || !strings.Contains(w.Body.String(), `"validated_success":1`) || !strings.Contains(w.Body.String(), `"first_pass_rate":100`) {
		t.Fatalf("analytics status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/organisations/org_acme/audit", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "integration_run.started") || !strings.Contains(w.Body.String(), "integration_run.completed") {
		t.Fatalf("audit status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestIdentityBackendAndVisibilityContractsAreIndependent(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	identityResponse := request(t, handler, http.MethodGet, "/api/v1/identity-provider", "doko_admin_demo", "")
	if identityResponse.Code != http.StatusOK || !strings.Contains(identityResponse.Body.String(), `"delegated_api_origin":"https://api.vendor.example"`) {
		t.Fatalf("identity provider status=%d body=%s", identityResponse.Code, identityResponse.Body.String())
	}
	legacyIdentity := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/identity", "doko_admin_demo", "")
	if legacyIdentity.Code != http.StatusNotFound {
		t.Fatalf("legacy identity route status=%d body=%s", legacyIdentity.Code, legacyIdentity.Body.String())
	}

	createdBackend := request(t, handler, http.MethodPost, "/api/v1/backend-connections", "doko_admin_demo", `{"name":"Support backend","base_url":"https://backend.vendor.example","authentication_type":"bearer","credential":"first-secret","state":"active"}`)
	if createdBackend.Code != http.StatusCreated {
		t.Fatalf("backend create status=%d body=%s", createdBackend.Code, createdBackend.Body.String())
	}
	var backend model.BackendConnection
	if err := json.Unmarshal(createdBackend.Body.Bytes(), &backend); err != nil {
		t.Fatal(err)
	}
	if backend.CredentialFingerprint == "" || strings.Contains(createdBackend.Body.String(), "first-secret") {
		t.Fatalf("backend credential was not safely redacted: %s", createdBackend.Body.String())
	}
	rotated := request(t, handler, http.MethodPost, "/api/v1/backend-connections/"+backend.ID+"/credentials", "doko_admin_demo", fmt.Sprintf(`{"credential":"second-secret","revision":%d}`, backend.Revision))
	if rotated.Code != http.StatusCreated || strings.Contains(rotated.Body.String(), "second-secret") {
		t.Fatalf("backend rotation status=%d body=%s", rotated.Code, rotated.Body.String())
	}
	stale := request(t, handler, http.MethodPost, "/api/v1/backend-connections/"+backend.ID+"/credentials", "doko_admin_demo", fmt.Sprintf(`{"credential":"stale-secret","revision":%d}`, backend.Revision))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale backend rotation status=%d body=%s", stale.Code, stale.Body.String())
	}

	unconfirmed := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"public-api","version_key":"v1","display_name":"Public API","description":"Public API metadata.","visibility":"public","lifecycle":"active"}`)
	if unconfirmed.Code != http.StatusConflict || !strings.Contains(unconfirmed.Body.String(), `"code":"public_confirmation_required"`) {
		t.Fatalf("unconfirmed public Integration status=%d body=%s", unconfirmed.Code, unconfirmed.Body.String())
	}
	confirmed := request(t, handler, http.MethodPost, "/api/v1/integrations", "doko_admin_demo", `{"family_key":"public-api","version_key":"v1","display_name":"Public API","description":"Public API metadata.","visibility":"public","acknowledge_public":true,"lifecycle":"active"}`)
	if confirmed.Code != http.StatusCreated || !strings.Contains(confirmed.Body.String(), `"visibility":"public"`) {
		t.Fatalf("confirmed public Integration status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
}
