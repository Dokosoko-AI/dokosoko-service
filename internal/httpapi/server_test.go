package httpapi_test

import (
	"bytes"
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

	"github.com/dokosoko/dokosoko/v2/internal/auth"
	"github.com/dokosoko/dokosoko/v2/internal/httpapi"
	"github.com/dokosoko/dokosoko/v2/internal/platform"
	"github.com/dokosoko/dokosoko/v2/internal/store"
)

func newServer() http.Handler {
	return httpapi.New(platform.New(store.NewMemory()), "https://dokosoko.example")
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
	if method == http.MethodPost && strings.HasPrefix(path, "/mcp/") && body != "" {
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
		params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28"}
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
	r := httptest.NewRequest(http.MethodPost, "/mcp/prod_acme", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	r.Header.Set("Authorization", "Bearer doko_private_demo")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Stateless MCPv2 Only") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPublicMCPIsAnonymousButOffByDefault(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
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

	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/packages/pkg_node/visibility", "doko_admin_demo", `{"visibility":"public","revision":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("package status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "public_confirmation_required") || !strings.Contains(w.Body.String(), "without authentication") {
		t.Fatalf("package body = %s", w.Body.String())
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

	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "provision_resource") {
		t.Fatalf("privileged tool leaked: %s", w.Body.String())
	}
	for _, expected := range []string{"search_knowledge", "find_package", "get_package"} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("missing tool %q: %s", expected, w.Body.String())
		}
	}

	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"API key"}}}`)
	if !strings.Contains(w.Body.String(), "Create an API key") {
		t.Fatalf("public record missing: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "operator-only") {
		t.Fatalf("private record leaked: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"provision_resource","arguments":{}}}`)
	if !strings.Contains(w.Body.String(), "not available on Public MCP") {
		t.Fatalf("privileged tool was not rejected: %s", w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"find_package","arguments":{}}}`)
	if strings.Contains(w.Body.String(), "@acme/node") {
		t.Fatalf("private package leaked: %s", w.Body.String())
	}
	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/packages/pkg_node/visibility", "doko_admin_demo", `{"visibility":"public","acknowledge_public":true,"revision":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("package publication status = %d, body = %s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"find_package","arguments":{}}}`)
	if !strings.Contains(w.Body.String(), "@acme/node") {
		t.Fatalf("public package missing: %s", w.Body.String())
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
	w = request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_knowledge","arguments":{"query":"API key"}}}`)
	if strings.Contains(w.Body.String(), "Create an API key") {
		t.Fatalf("newly private source still visible: %s", w.Body.String())
	}
}

func TestPrivateMCPRequiresAuthentication(t *testing.T) {
	t.Parallel()
	handler := newServer()
	w := request(t, handler, http.MethodPost, "/mcp/prod_acme", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	w = request(t, handler, http.MethodPost, "/mcp/prod_acme", "doko_private_demo", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "search_knowledge") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
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
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<!doctype html><title>DokoSoko console</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithUI(platform.New(store.NewMemory()), "https://dokosoko.example", directory)

	w := request(t, handler, http.MethodGet, "/", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "DokoSoko console") {
		t.Fatalf("console status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("missing console CSP: %q", w.Header().Get("Content-Security-Policy"))
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
		w := request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, body = %s", index+1, w.Code, w.Body.String())
		}
	}
	w := request(t, handler, http.MethodPost, "/mcp/public/prod_acme", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
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
	w = request(t, handler, http.MethodPost, "/api/v1/setup/begin", "setup-token-for-tests", `{"email":"root@example.com","display_name":"Root Operator","password":"Correct-Horse-47!Battery"}`)
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
