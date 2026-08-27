package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

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
	broker := identity.NewBroker(memory, vault, "https://dokosoko.example", nil, nil, nil)
	return httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", IdentityBroker: broker, Reporting: reporting.New(memory), AllowDemoTokens: true})
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

func preflightAndPublishIntegration(t *testing.T, handler http.Handler, integrationID string) *httptest.ResponseRecorder {
	t.Helper()
	preflight := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integrationID+"/preflight", "doko_admin_demo", `{}`)
	if preflight.Code != http.StatusOK {
		t.Fatalf("integration preflight = %d: %s", preflight.Code, preflight.Body.String())
	}
	var candidate struct {
		CandidateRevision     int64  `json:"candidate_revision"`
		CandidateManifestHash string `json:"candidate_manifest_hash"`
		Ready                 bool   `json:"ready"`
	}
	if err := json.Unmarshal(preflight.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if !candidate.Ready || candidate.CandidateRevision < 1 || candidate.CandidateManifestHash == "" {
		t.Fatalf("integration preflight did not return a publishable candidate: %s", preflight.Body.String())
	}
	body, err := json.Marshal(map[string]any{"candidate_revision": candidate.CandidateRevision, "candidate_manifest_hash": candidate.CandidateManifestHash})
	if err != nil {
		t.Fatal(err)
	}
	return request(t, handler, http.MethodPost, "/api/v1/integrations/"+integrationID+"/publish", "doko_admin_demo", string(body))
}

func prepareHTTPPrivateIntegrationFoundations(t *testing.T, handler http.Handler, integrationID string) {
	t.Helper()
	documentationManifest, err := json.Marshal([]map[string]any{{
		"source_publication_id": "pub_docs_seed",
		"source_id":             "src_docs",
		"revision":              1,
		"content_hash":          "sha256:" + strings.Repeat("1", 64),
		"name":                  "Reviewed developer documentation",
	}})
	if err != nil {
		t.Fatal(err)
	}
	resources := []struct {
		kind     string
		name     string
		manifest string
	}{
		{kind: "documentation", name: "Reviewed documentation", manifest: string(documentationManifest)},
		{kind: "api", name: "Reviewed API contract", manifest: `[{"name":"health.read","path":"/health"}]`},
	}
	for _, resource := range resources {
		body, marshalErr := json.Marshal(map[string]any{"kind": resource.kind, "name": resource.name, "description": resource.name, "manifest": json.RawMessage(resource.manifest)})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		created := request(t, handler, http.MethodPost, "/api/v1/resource-sets", "doko_admin_demo", string(body))
		if created.Code != http.StatusCreated {
			t.Fatalf("create %s resource = %d: %s", resource.kind, created.Code, created.Body.String())
		}
		var resourceSet model.ResourceSet
		if unmarshalErr := json.Unmarshal(created.Body.Bytes(), &resourceSet); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		attached := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integrationID+"/resource-sets", "doko_admin_demo", `{"resource_set_id":"`+resourceSet.ID+`","pinned_revision_id":"`+resourceSet.Latest.ID+`"}`)
		if attached.Code != http.StatusOK {
			t.Fatalf("attach %s resource = %d: %s", resource.kind, attached.Code, attached.Body.String())
		}
	}

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

func TestPublicMCPRequiresAnExplicitlyPublicPublicationAndReadOnlyTools(t *testing.T) {
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
	if strings.Contains(w.Body.String(), "Create an API key") || strings.Contains(w.Body.String(), "operator-only") {
		t.Fatalf("changing current source visibility exposed an immutable private publication: %s", w.Body.String())
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

func TestSupportReportingToolsRequireConsentQueuePlaintextReportsAndStayPrivate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	reporter := reporting.New(memory)
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
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"queued"`) || !strings.Contains(w.Body.String(), `"submission_id"`) || strings.Contains(w.Body.String(), "unexpected result") {
		t.Fatalf("confirmed bug was not queued safely: status=%d body=%s", w.Code, w.Body.String())
	}
	values, _, err = memory.ReportSubmissions(ctx, "prod_acme", "", 10)
	if err != nil || len(values) != 1 || values[0].State != "queued" || !bytes.Contains(values[0].Payload, []byte("Connector failure")) {
		t.Fatalf("plaintext queued bug missing: values=%#v err=%v", values, err)
	}
	w = request(t, handler, http.MethodGet, "/api/v1/support-submissions", "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Connector failure") || strings.Contains(w.Body.String(), "unexpected result") || strings.Contains(w.Body.String(), `"content"`) {
		t.Fatalf("inbox list did not limit decrypted content: status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodGet, "/api/v1/support-submissions/"+values[0].ID, "doko_admin_demo", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "unexpected result") || !strings.Contains(w.Body.String(), `"content"`) || strings.Contains(w.Body.String(), `"support_route_id"`) {
		t.Fatalf("on-demand report detail missing: status=%d body=%s", w.Code, w.Body.String())
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
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"queued"`) {
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

func TestPublishedRecipesAreStableMCPResourcesWithUsageAnalytics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root-test", RequestID: "req-recipe-resource"}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "orders-api", VersionKey: "v1", DisplayName: "Orders API", Description: "Read order status.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	prepareHTTPPrivateIntegrationFoundations(t, handler, integration.ID)
	preparePublishedRecipeHTTPIntegration(t, ctx, memory, service, integration, "orders", actor)
	analysis, err := service.AnalyseIntegrationFor(ctx, "prod_acme", integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := service.GenerateRecipesForIntegration(ctx, "prod_acme", analysis.ID, integration.ID, actor)
	if err != nil || len(recipes) != 1 {
		t.Fatalf("generate recipes: values=%#v err=%v", recipes, err)
	}
	recipe, err := service.ApproveRecipe(ctx, "prod_acme", recipes[0].ID, recipes[0].Revision, recipes[0].CurrentRevisionID, actor)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err = service.PublishRecipe(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, actor)
	if err != nil {
		t.Fatal(err)
	}
	discovery := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":0,"method":"server/discover","params":{}}`)
	if discovery.Code != http.StatusOK || !strings.Contains(discovery.Body.String(), "already connected to this MCP server") || !strings.Contains(discovery.Body.String(), "not MCP setup guides") {
		t.Fatalf("recipe discovery instructions status=%d body=%s", discovery.Code, discovery.Body.String())
	}
	tools := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":10,"method":"tools/list","params":{}}`)
	if tools.Code != http.StatusOK || !strings.Contains(tools.Body.String(), `"name":"integration.recipes.list"`) || !strings.Contains(tools.Body.String(), `"outputSchema"`) || !strings.Contains(tools.Body.String(), `"enum":["product-integration-v2","deployment-recipe-v3"]`) || !strings.Contains(tools.Body.String(), "This tool never guesses") {
		t.Fatalf("recipe tool contracts status=%d body=%s", tools.Code, tools.Body.String())
	}
	if recipe.ContractVersion != model.RecipeContractDeploymentV3 || len(recipe.APIAttachments) != 1 {
		t.Fatalf("generated MCP recipe did not use deployment contract: %#v", recipe)
	}
	integrationID := recipe.APIAttachments[0].IntegrationID

	w := request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), recipe.StableURI) || !strings.Contains(w.Body.String(), `"description":"Product integration implementation:`) || !strings.Contains(w.Body.String(), `"mimeType":"text/markdown"`) || !strings.Contains(w.Body.String(), `"contract_version":"deployment-recipe-v3"`) || !strings.Contains(w.Body.String(), `"integration_ids":["`+integrationID+`"]`) || !strings.Contains(w.Body.String(), `"published_at"`) {
		t.Fatalf("recipe resource list status=%d body=%s", w.Code, w.Body.String())
	}
	readBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "resources/read", "params": map[string]any{"uri": recipe.StableURI}})
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", string(readBody))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), recipe.StableURI) || !strings.Contains(w.Body.String(), `"contract_version":"deployment-recipe-v3"`) || !strings.Contains(w.Body.String(), `"integration_ids":["`+integrationID+`"]`) {
		t.Fatalf("recipe resource read status=%d body=%s", w.Code, w.Body.String())
	}
	checkBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 21, "method": "tools/call", "params": map[string]any{"name": "integration.check", "arguments": map[string]any{"recipe_uri": recipe.StableURI}}})
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", string(checkBody))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"contract_version":"deployment-recipe-v3"`) || !strings.Contains(w.Body.String(), `"integration_ids":["`+integrationID+`"]`) || !strings.Contains(w.Body.String(), `"current":true`) {
		t.Fatalf("recipe check status=%d body=%s", w.Code, w.Body.String())
	}
	listBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 20, "method": "tools/call", "params": map[string]any{"name": "integration.recipes.list", "arguments": map[string]any{}}})
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", string(listBody))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), recipe.StableURI) || !strings.Contains(w.Body.String(), `"contract_version":"deployment-recipe-v3"`) || !strings.Contains(w.Body.String(), `"integration_ids":["`+integrationID+`"]`) {
		t.Fatalf("compact recipe list status=%d body=%s", w.Code, w.Body.String())
	}
	for _, internalField := range []string{`"organisation_id"`, `"product_id"`, `"dependencies"`, `"current_revision"`, `"analysis_id"`} {
		if strings.Contains(w.Body.String(), internalField) {
			t.Fatalf("compact recipe list leaked %s: %s", internalField, w.Body.String())
		}
	}
	planBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "integration.plan", "arguments": map[string]any{"outcome": recipe.Outcome}}})
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", string(planBody))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), recipe.StableURI) || !strings.Contains(w.Body.String(), `"contract_version":"deployment-recipe-v3"`) || !strings.Contains(w.Body.String(), `"integration_ids":["`+integrationID+`"]`) {
		t.Fatalf("recipe plan status=%d body=%s", w.Code, w.Body.String())
	}
	noMatchBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 30, "method": "tools/call", "params": map[string]any{"name": "integration.plan", "arguments": map[string]any{"outcome": "unrelated outcome that is not published"}}})
	w = request(t, handler, http.MethodPost, "/mcp", "doko_private_demo", string(noMatchBody))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":-32004`) || !strings.Contains(w.Body.String(), `"reason":"no_exact_match"`) || !strings.Contains(w.Body.String(), recipe.StableURI) {
		t.Fatalf("recipe plan no-match status=%d body=%s", w.Code, w.Body.String())
	}
	product, _ := memory.Product(ctx, "prod_acme")
	product.PublicMCPEnabled = true
	if _, err := memory.UpdateProduct(ctx, product, product.Revision); err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPost, "/mcp/public", "", `{"jsonrpc":"2.0","id":4,"method":"resources/list","params":{}}`)
	if strings.Contains(w.Body.String(), recipe.StableURI) {
		t.Fatalf("private recipe leaked through Public MCP: %s", w.Body.String())
	}
}

func TestAgentSetupDistributionAndPromptsFollowReadiness(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	distribution := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", "")
	if distribution.Code != http.StatusOK {
		t.Fatalf("distribution status = %d, body = %s", distribution.Code, distribution.Body.String())
	}
	var payload struct {
		AgentSetup map[string]struct {
			Available         bool   `json:"available"`
			UnavailableReason string `json:"unavailable_reason"`
			URL               string `json:"url"`
			EmbedScriptURL    string `json:"embed_script_url"`
			EmbedCode         string `json:"embed_code"`
			EmbedHTML         string `json:"embed_html"`
			ContainsSecret    bool   `json:"contains_secret"`
		} `json:"agent_setup"`
	}
	if err := json.Unmarshal(distribution.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AgentSetup["public"].Available || payload.AgentSetup["public"].UnavailableReason != "public_mcp_disabled" {
		t.Fatalf("unexpected public setup state: %#v", payload.AgentSetup["public"])
	}
	private := payload.AgentSetup["private"]
	if !private.Available || private.ContainsSecret || private.URL != "https://dokosoko.example/agent-setup/private/prompt.md" {
		t.Fatalf("unexpected private setup state: %#v", private)
	}
	if private.EmbedScriptURL != "https://dokosoko.example/agent-setup/button.js" || !strings.Contains(private.EmbedCode, `<script async src="https://dokosoko.example/agent-setup/button.js"></script>`) || !strings.Contains(private.EmbedCode, `<dokosoko-mcp-button kind="private" lang="auto"></dokosoko-mcp-button>`) {
		t.Fatalf("private embed code is not the localized Web Component: %#v", private)
	}
	if strings.Contains(private.EmbedCode, "<a ") || strings.Contains(private.EmbedCode, "style=") {
		t.Fatalf("private embed code copied rendered button HTML: %s", private.EmbedCode)
	}
	for _, marker := range []string{"data-dokosoko-agent-setup=", "data-agent-client=\"codex\"", "data-agent-client=\"claude-code\"", "data-agent-client=\"cursor\"", "data-agent-client=\"opencode\"", "https://dokosoko.example/agent-client-icons/codex.svg", "https://dokosoko.example/agent-client-icons/claude-code.svg", "https://dokosoko.example/agent-client-icons/cursor.svg", "https://dokosoko.example/agent-client-icons/opencode.svg"} {
		if !strings.Contains(private.EmbedHTML, marker) {
			t.Fatalf("private embed omitted %q: %s", marker, private.EmbedHTML)
		}
	}
	if strings.Contains(private.EmbedHTML, ">Private<") {
		t.Fatalf("legacy private embed retained the removed access chip: %s", private.EmbedHTML)
	}

	buttonScript := request(t, handler, http.MethodGet, "/agent-setup/button.js", "", "")
	if buttonScript.Code != http.StatusOK || !strings.HasPrefix(buttonScript.Header().Get("Content-Type"), "text/javascript") || buttonScript.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatalf("button script status = %d, headers = %#v, body = %s", buttonScript.Code, buttonScript.Header(), buttonScript.Body.String())
	}
	for _, marker := range []string{`customElements.define(elementName, DokoSokoMCPButton)`, `t(locale, "agentAccess.connectYourAgentToName"`, `"deploymentName":"Acme Platform"`, `"public":"https://dokosoko.example/agent-setup/public/prompt.md"`, `"private":"https://dokosoko.example/agent-setup/private/prompt.md"`, `Conecta tu agente a {{name}}`, `Connectez votre agent à {{name}}`, `Agent mit {{name}} verbinden`, `エージェントを{{name}}に接続`, `Підключити агента до {{name}}`, `Conecte seu agente a {{name}}`} {
		if !strings.Contains(buttonScript.Body.String(), marker) {
			t.Fatalf("button script omitted %q: %s", marker, buttonScript.Body.String())
		}
	}
	if strings.Contains(buttonScript.Body.String(), "agent-access-chip") || strings.Contains(buttonScript.Body.String(), "__DOKOSOKO_AGENT_SETUP_CONFIG__") {
		t.Fatalf("button script retained a chip or unresolved config: %s", buttonScript.Body.String())
	}
	for _, placeholder := range []string{"◉", "✳", "◆", "▣"} {
		if strings.Contains(private.EmbedHTML, placeholder) {
			t.Fatalf("private embed contains placeholder glyph %q: %s", placeholder, private.EmbedHTML)
		}
	}

	publicPrompt := request(t, handler, http.MethodGet, "/agent-setup/public/prompt.md", "", "")
	if publicPrompt.Code != http.StatusNotFound {
		t.Fatalf("disabled public prompt status = %d, body = %s", publicPrompt.Code, publicPrompt.Body.String())
	}
	privatePrompt := request(t, handler, http.MethodGet, "/agent-setup/private/prompt.md", "", "")
	if privatePrompt.Code != http.StatusOK || !strings.HasPrefix(privatePrompt.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("private prompt status = %d, content-type = %q, body = %s", privatePrompt.Code, privatePrompt.Header().Get("Content-Type"), privatePrompt.Body.String())
	}
	for _, expected := range []string{"https://dokosoko.example/mcp", "## Codex", "## Claude Code", "## Cursor", "## OpenCode", "codex mcp login acme-private", "opencode mcp auth acme-private"} {
		if !strings.Contains(privatePrompt.Body.String(), expected) {
			t.Fatalf("private prompt omitted %q: %s", expected, privatePrompt.Body.String())
		}
	}
	if strings.Contains(privatePrompt.Body.String(), "doko_at_") || strings.Contains(privatePrompt.Body.String(), "doko_private_demo") {
		t.Fatalf("private setup leaked a credential: %s", privatePrompt.Body.String())
	}
	privatePromptWithLanguage := request(t, handler, http.MethodGet, "/agent-setup/private/prompt.md?lang=ja", "", "")
	if privatePromptWithLanguage.Code != http.StatusOK || privatePromptWithLanguage.Body.String() != privatePrompt.Body.String() {
		t.Fatalf("prompt.md changed with the human UI locale: status=%d body=%s", privatePromptWithLanguage.Code, privatePromptWithLanguage.Body.String())
	}

	request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/distribution", "doko_admin_demo", `{"public_mcp_enabled":true,"acknowledge_public":true,"revision":1}`)
	publicPrompt = request(t, handler, http.MethodGet, "/agent-setup/public/prompt.md", "", "")
	if publicPrompt.Code != http.StatusOK || !strings.Contains(publicPrompt.Body.String(), "https://dokosoko.example/mcp/public") || strings.Contains(publicPrompt.Body.String(), "mcp login") || strings.Contains(publicPrompt.Body.String(), "mcp auth") {
		t.Fatalf("enabled public prompt status = %d, body = %s", publicPrompt.Code, publicPrompt.Body.String())
	}

	withoutIdentity := request(t, newServer(), http.MethodGet, "/agent-setup/private/prompt.md", "", "")
	if withoutIdentity.Code != http.StatusNotFound {
		t.Fatalf("identity-free private prompt status = %d, body = %s", withoutIdentity.Code, withoutIdentity.Body.String())
	}
}

func TestDynamicOAuthClientRegistrationIsPublicIdempotentAndPKCEOnly(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)
	body := `{"client_name":"Cursor","redirect_uris":["http://localhost:8787/callback","https://www.cursor.com/agents/mcp/oauth/callback"],"token_endpoint_auth_method":"none","grant_types":["authorization_code"],"response_types":["code"],"scope":"mcp:private"}`
	first := request(t, handler, http.MethodPost, "/oauth/register", "", body)
	if first.Code != http.StatusCreated || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("registration status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstRegistration struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstRegistration); err != nil || !strings.HasPrefix(firstRegistration.ClientID, "mcp_client_") {
		t.Fatalf("registration = %#v, error = %v", firstRegistration, err)
	}
	retry := request(t, handler, http.MethodPost, "/oauth/register", "", body)
	if retry.Code != http.StatusCreated || !strings.Contains(retry.Body.String(), `"client_id":"`+firstRegistration.ClientID+`"`) {
		t.Fatalf("idempotent retry status = %d, body = %s", retry.Code, retry.Body.String())
	}
	invalid := request(t, handler, http.MethodPost, "/oauth/register", "", `{"redirect_uris":["https://client.example/callback#fragment"]}`)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_redirect_uri") {
		t.Fatalf("invalid redirect status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
	metadata := request(t, handler, http.MethodGet, "/.well-known/oauth-authorization-server", "", "")
	if !strings.Contains(metadata.Body.String(), `"registration_endpoint":"https://dokosoko.example/oauth/register"`) {
		t.Fatalf("authorization metadata omitted registration endpoint: %s", metadata.Body.String())
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

func TestIdentityAndVisibilityContractsAreIndependent(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)

	identityResponse := request(t, handler, http.MethodGet, "/api/v1/identity-provider", "doko_admin_demo", "")
	if identityResponse.Code != http.StatusOK || !strings.Contains(identityResponse.Body.String(), `"authorization_api_origin":"https://api.vendor.example"`) || strings.Contains(identityResponse.Body.String(), `"delegated_api_origin"`) {
		t.Fatalf("identity provider status=%d body=%s", identityResponse.Code, identityResponse.Body.String())
	}
	legacyIdentity := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/identity", "doko_admin_demo", "")
	if legacyIdentity.Code != http.StatusNotFound {
		t.Fatalf("legacy identity route status=%d body=%s", legacyIdentity.Code, legacyIdentity.Body.String())
	}

	removedBackend := request(t, handler, http.MethodGet, "/api/v1/backend-connections", "doko_admin_demo", "")
	if removedBackend.Code != http.StatusNotFound {
		t.Fatalf("removed backend route status=%d body=%s", removedBackend.Code, removedBackend.Body.String())
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
