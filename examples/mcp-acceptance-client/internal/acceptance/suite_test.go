package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExercisesProtocolAuthorizationAndConfirmation(t *testing.T) {
	t.Parallel()
	var confirmationUsed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("MCP-Protocol-Version"); got != ProtocolVersion {
			t.Errorf("protocol header = %q", got)
		}
		var request struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      string         `json:"id"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid request JSON")
			return
		}
		if r.Header.Get("Mcp-Method") != request.Method || r.Header.Get("X-Request-ID") != request.ID {
			t.Error("method or request id header mismatch")
		}
		meta, _ := request.Params["_meta"].(map[string]any)
		if meta["io.modelcontextprotocol/protocolVersion"] != ProtocolVersion {
			t.Error("missing protocol metadata")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", request.ID)
		restricted := r.Header.Get("Authorization") == "Bearer restricted-token"
		switch request.Method {
		case "server/discover":
			writeTestRPC(w, request.ID, map[string]any{"supportedVersions": []string{ProtocolVersion}}, nil)
		case "resources/list":
			writeTestRPC(w, request.ID, map[string]any{"resources": []map[string]any{{"uri": "mcp://example/guide"}}}, nil)
		case "resources/read":
			writeTestRPC(w, request.ID, map[string]any{"contents": []map[string]any{{"uri": "mcp://example/guide", "text": "safe"}}}, nil)
		case "tools/list":
			tools := []map[string]any{{"name": "safe.read"}, {"name": "confirmed.write"}}
			if !restricted {
				tools = append(tools, map[string]any{"name": "granted.write"})
			}
			writeTestRPC(w, request.ID, map[string]any{"tools": tools}, nil)
		case "tools/call":
			name, _ := request.Params["name"].(string)
			if r.Header.Get("Mcp-Name") != name {
				t.Error("tool name header mismatch")
			}
			if name == "granted.write" && restricted {
				writeTestRPC(w, request.ID, nil, &rpcError{Code: -32003, Message: "denied"})
				return
			}
			if name == "confirmed.write" {
				challenge, _ := meta["confirmation_challenge"].(string)
				if meta["confirmed"] != true || challenge == "" {
					data, _ := json.Marshal(map[string]any{"confirmation_required": true, "confirmation_challenge": "test_one_time_challenge", "retry_metadata_field": "params._meta.confirmation_challenge"})
					writeTestRPC(w, request.ID, nil, &rpcError{Code: -32003, Message: "confirmation required", Data: data})
					return
				}
				if challenge != "test_one_time_challenge" || !confirmationUsed.CompareAndSwap(false, true) {
					writeTestRPC(w, request.ID, nil, &rpcError{Code: -32003, Message: "confirmation challenge invalid or replayed"})
					return
				}
			}
			writeTestRPC(w, request.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}, nil)
		default:
			writeTestRPC(w, request.ID, nil, &rpcError{Code: -32601, Message: "not found"})
		}
	}))
	defer server.Close()

	report, err := Run(context.Background(), Config{
		Endpoint: server.URL, AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
		Token: "full-token", RestrictedToken: "restricted-token",
		ExpectedTools: []string{"safe.read", "granted.write"}, ExpectedResources: []string{"mcp://example/guide"},
		CallTool: "safe.read", GrantTool: "granted.write", VerifyRestrictedCallDenied: true,
		ConfirmationTool: "confirmed.write", VerifyConfirmedCall: true, CheckUnauthenticated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Accepted() || report.Summary.Skipped != 0 {
		body, _ := json.MarshalIndent(report, "", "  ")
		t.Fatalf("acceptance failed:\n%s", body)
	}
	for _, required := range []string{"server/discover", "resources/list", "resources/read: mcp://example/guide", "tools/list", "tools/call: safe.read", "authorization.grant.negative", "authorization.confirmation.negative", "authorization.confirmation.positive", "authorization.confirmation.replay", "authorization.unauthenticated"} {
		if !hasCheck(report, required, Pass) {
			t.Errorf("missing passing check %q", required)
		}
	}
}

func TestSecureEndpointRequiresExactLoopbackHTTPAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		endpoint string
		allowed  []string
		wantErr  bool
	}{
		{name: "HTTPS", endpoint: "https://mcp.example.test/mcp"},
		{name: "loopback missing opt in", endpoint: "http://localhost:8080/mcp", wantErr: true},
		{name: "loopback exact", endpoint: "http://localhost:8080/mcp", allowed: []string{"localhost:8080"}},
		{name: "localhost subdomain exact", endpoint: "http://api.complicatedauth.localhost:38080/mcp", allowed: []string{"api.complicatedauth.localhost:38080"}},
		{name: "port mismatch", endpoint: "http://localhost:8080/mcp", allowed: []string{"localhost:8081"}, wantErr: true},
		{name: "non loopback cannot opt in", endpoint: "http://mcp.example.test:8080/mcp", allowed: []string{"mcp.example.test:8080"}, wantErr: true},
		{name: "HTTP requires explicit port", endpoint: "http://localhost/mcp", allowed: []string{"localhost:80"}, wantErr: true},
		{name: "IPv6 loopback exact", endpoint: "http://[::1]:8080/mcp", allowed: []string{"[::1]:8080"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSecureEndpoint(test.endpoint, test.allowed)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSecureEndpoint() error = %v, want error %v", err, test.wantErr)
			}
		})
	}
}

func TestRunRejectsUnsafeHTTPBeforeSendingBearerCredential(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("request must not be sent")
	})}
	_, err := Run(context.Background(), Config{
		Endpoint:   "http://mcp.example.test:8080/mcp",
		Token:      "sensitive-bearer",
		HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("sent %d request(s) before rejecting unsafe HTTP", requests.Load())
	}
}

func TestOAuthStartRejectsPlainHTTPBeforeNetworkRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("request must not be sent")
	})}
	_, err := StartOAuth(context.Background(), OAuthStartConfig{
		Endpoint:    "http://localhost:8080/mcp",
		RedirectURI: "http://127.0.0.1:7777/callback",
		StateFile:   filepath.Join(t.TempDir(), "state.json"),
		HTTPClient:  client,
	})
	if err == nil || !strings.Contains(err.Error(), "explicitly allow") {
		t.Fatalf("error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("sent %d OAuth request(s) before rejecting unsafe HTTP", requests.Load())
	}
}

func TestMCPClientDoesNotFollowBearerRedirect(t *testing.T) {
	t.Parallel()
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect-target" {
			redirected.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/redirect-target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client := mcpClient{
		endpoint:   server.URL + "/mcp",
		token:      "sensitive-bearer",
		httpClient: clientWithoutRedirects(nil, time.Second),
	}
	outcome := client.call(context.Background(), "server/discover", "", nil, false)
	if outcome.HTTPStatus != http.StatusTemporaryRedirect || outcome.TransportError == nil {
		t.Fatalf("redirect outcome = %#v", outcome)
	}
	if redirected.Load() != 0 {
		t.Fatal("bearer request followed an HTTP redirect")
	}
}

func TestRequestedGrantCheckSkippedMakesReportUnaccepted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("X-Request-ID", request.ID)
		switch request.Method {
		case "server/discover":
			writeTestRPC(w, request.ID, map[string]any{"supportedVersions": []string{ProtocolVersion}}, nil)
		case "resources/list":
			writeTestRPC(w, request.ID, map[string]any{"resources": []any{}}, nil)
		case "tools/list":
			writeTestRPC(w, request.ID, map[string]any{"tools": []map[string]any{{"name": "granted.write"}}}, nil)
		default:
			writeTestRPC(w, request.ID, nil, &rpcError{Code: -32601, Message: "not found"})
		}
	}))
	defer server.Close()

	report, err := Run(context.Background(), Config{
		Endpoint: server.URL, AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
		Token: "primary-token", GrantTool: "granted.write",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Accepted() || report.Summary.RequiredSkipped != 1 {
		body, _ := json.MarshalIndent(report, "", "  ")
		t.Fatalf("required skip did not fail acceptance:\n%s", body)
	}
	check, ok := findCheck(report, "authorization.grant.negative")
	if !ok || check.Status != Skip || !check.Required {
		t.Fatalf("grant negative check = %#v, found %v", check, ok)
	}
}

func TestReportsNeverContainTokensBodiesOrArguments(t *testing.T) {
	t.Parallel()
	const secret = "never-print-this-bearer-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"`+secret+`"}`)
	}))
	defer server.Close()
	report, err := Run(context.Background(), Config{
		Endpoint: server.URL, AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
		Token: secret, CallTool: "danger.write",
		CallArguments: map[string]any{"password": "argument-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var human strings.Builder
	if err := WriteHuman(&human, report); err != nil {
		t.Fatal(err)
	}
	var machine strings.Builder
	if err := WriteJSON(&machine, report); err != nil {
		t.Fatal(err)
	}
	combined := human.String() + machine.String()
	for _, forbidden := range []string{secret, "argument-secret"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("report contained secret %q", forbidden)
		}
	}
}

func TestOAuthStartAndFinishWithDCRAndPKCE(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	var mu sync.Mutex
	var challenge string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			writeJSONTest(w, map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
				"code_challenge_methods_supported":      []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/register":
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["token_endpoint_auth_method"] != "none" {
				t.Error("DCR did not register a public client")
			}
			w.WriteHeader(http.StatusCreated)
			writeJSONTest(w, map[string]any{"client_id": "registered-client"})
		case "/token":
			_ = r.ParseForm()
			verifier := r.Form.Get("code_verifier")
			digest := sha256.Sum256([]byte(verifier))
			mu.Lock()
			expectedChallenge := challenge
			mu.Unlock()
			if len(verifier) < 43 || base64.RawURLEncoding.EncodeToString(digest[:]) != expectedChallenge {
				t.Error("invalid PKCE verifier")
			}
			if r.Form.Get("code") != "one-time-code" || r.Form.Get("resource") != server.URL+"/mcp" {
				t.Error("invalid token exchange")
			}
			writeJSONTest(w, map[string]any{"access_token": "secret-access-token", "token_type": "Bearer", "expires_in": 300, "scope": "mcp:private"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	stateFile := filepath.Join(directory, "oauth-state.json")
	tokenFile := filepath.Join(directory, "token.json")
	started, err := StartOAuth(context.Background(), OAuthStartConfig{
		Endpoint: server.URL + "/mcp", RedirectURI: "http://127.0.0.1:7777/callback",
		AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL),
		StateFile:           stateFile, TokenFile: tokenFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("client_id") != "registered-client" || parsed.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL = %s", started.AuthorizationURL)
	}
	mu.Lock()
	challenge = parsed.Query().Get("code_challenge")
	mu.Unlock()
	assertPrivateMode(t, stateFile)
	callback := "http://127.0.0.1:7777/callback?code=one-time-code&state=" + url.QueryEscape(parsed.Query().Get("state"))
	finished, err := FinishOAuth(context.Background(), OAuthFinishConfig{StateFile: stateFile, TokenFile: tokenFile, CallbackURL: callback})
	if err != nil {
		t.Fatal(err)
	}
	if finished.TokenFile != tokenFile || finished.ExpiresIn != 300 {
		t.Fatalf("finish result = %#v", finished)
	}
	assertPrivateMode(t, tokenFile)
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("consumed state file still exists: %v", err)
	}
	token, err := LoadToken(tokenFile, "")
	if err != nil || token != "secret-access-token" {
		t.Fatalf("loaded token mismatch: value length=%d err=%v", len(token), err)
	}
}

func TestOAuthFinishRejectsMismatchedStateBeforeExchange(t *testing.T) {
	t.Parallel()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	stateFile := filepath.Join(t.TempDir(), "state.json")
	err := writePrivateNew(stateFile, oauthState{
		Version: oauthStateVersion, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Minute).UTC(),
		TokenEndpoint: server.URL, ClientID: "client", RedirectURI: "http://127.0.0.1/callback",
		Resource: server.URL + "/mcp", State: "expected", CodeVerifier: strings.Repeat("v", 48),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = FinishOAuth(context.Background(), OAuthFinishConfig{
		StateFile: stateFile, TokenFile: filepath.Join(t.TempDir(), "token.json"),
		CallbackURL: "http://127.0.0.1/callback?code=code&state=wrong",
	})
	if err == nil || !strings.Contains(err.Error(), "state did not match") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("token endpoint was called before state validation")
	}
}

func TestAuthorizationServerMetadataURLPreservesIssuerPath(t *testing.T) {
	t.Parallel()
	value, err := authorizationServerMetadataURL("https://identity.example/tenant%20one")
	if err != nil {
		t.Fatal(err)
	}
	if value != "https://identity.example/.well-known/oauth-authorization-server/tenant%20one" {
		t.Fatalf("metadata URL = %q", value)
	}
}

func writeTestRPC(w http.ResponseWriter, id string, result any, rpcErr *rpcError) {
	payload := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		payload["error"] = rpcErr
	} else {
		payload["result"] = result
	}
	writeJSONTest(w, payload)
}

func writeJSONTest(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func hasCheck(report Report, name string, status Status) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}

func findCheck(report Report, name string) (Check, bool) {
	for _, check := range report.Checks {
		if check.Name == name {
			return check, true
		}
	}
	return Check{}, false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func assertPrivateMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func testLoopbackHTTP(t *testing.T, raw string) []string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		t.Fatalf("test URL %q was invalid: %v", raw, err)
	}
	return []string{parsed.Host}
}

func ExampleWriteHuman() {
	report := Report{Endpoint: "http://api.example.localhost:8080/mcp", ProtocolVersion: ProtocolVersion}
	report.Add(Check{Name: "server/discover", Status: Pass, RequestID: "mcpacc_example"})
	_ = WriteHuman(os.Stdout, report)
	fmt.Println("done")
	// Output:
	// MCP acceptance report
	// Endpoint: http://api.example.localhost:8080/mcp
	// Protocol: 2026-07-28
	//
	// [PASS] server/discover — request mcpacc_example
	//
	// Summary: 1 passed, 0 failed, 0 skipped (0 required; 0 ms)
	// done
}
