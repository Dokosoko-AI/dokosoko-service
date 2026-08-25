package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

func publishStaticTool(t *testing.T, service *platform.Service, input platform.ToolInput) string {
	t.Helper()
	tool, err := service.CreateTool(context.Background(), input, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err = service.PublishTool(context.Background(), tool.ProductID, tool.ID, tool.Revision, platform.Actor{ID: "root", RequestID: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	return tool.Namespace + "." + tool.Name
}

func upstreamToolInput(name, method, endpoint string, auth any, credential string) platform.ToolInput {
	authJSON, _ := json.Marshal(auth)
	return platform.ToolInput{
		ProductID:           "prod_acme",
		Namespace:           "upstream",
		Name:                name,
		Description:         "Call one configured upstream operation.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		Endpoint:            endpoint,
		HTTPMethod:          method,
		UpstreamAuth:        authJSON,
		Credential:          credential,
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:           5000,
	}
}

func TestRuntimeAppliesEncryptedBearerCredential(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x64}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	fullName := publishStaticTool(t, service, upstreamToolInput("bearer", http.MethodGet, "https://api.example.test/status", platform.ToolUpstreamAuth{Type: "bearer"}, "server-side-token"))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer server-side-token" {
			t.Fatalf("authorization = %q", got)
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	runtime.SetCredentialResolver(service)
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAppliesFixedVendorAuthorizationScheme(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x65}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	fullName := publishStaticTool(t, service, upstreamToolInput("ssws", http.MethodGet, "https://api.example.test/status", platform.ToolUpstreamAuth{Type: "authorization_scheme", Scheme: "SSWS"}, "vendor-token"))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "SSWS vendor-token" {
			t.Fatalf("authorization = %q", got)
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	runtime.SetCredentialResolver(service)
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAppliesNormalizedFixedHeaderPrefixes(t *testing.T) {
	for _, test := range []struct {
		name       string
		authType   string
		headerName string
	}{
		{name: "API key header", authType: "api_key_header", headerName: "X-API-Key"},
		{name: "custom header", authType: "custom_header", headerName: "X-Vendor-Token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			memory := store.NewMemory()
			vault, err := secrets.New(bytes.Repeat([]byte{0x66}, 32))
			if err != nil {
				t.Fatal(err)
			}
			service := platform.NewWithVault(memory, vault)
			auth := platform.ToolUpstreamAuth{Type: test.authType, HeaderName: test.headerName, Prefix: "  Token  "}
			fullName := publishStaticTool(t, service, upstreamToolInput(strings.ReplaceAll(test.authType, "_", ""), http.MethodGet, "https://api.example.test/status", auth, "vendor-secret"))
			runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
				if got := request.Header.Get(test.headerName); got != "Token vendor-secret" {
					t.Fatalf("%s = %q", test.headerName, got)
				}
				return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
			}))
			runtime.SetCredentialResolver(service)
			if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeMapsRequestAndExtractsResponse(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	input := upstreamToolInput("mapped", http.MethodPost, "https://api.example.test/items/{item_id}", platform.ToolUpstreamAuth{Type: "none"}, "")
	input.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"item_id":{"type":"string"},"filter":{"type":"string"},"trace_id":{"type":"string"},"payload":{"type":"string"}},"required":["item_id","filter","trace_id","payload"]}`)
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"item_id":"path","filter":"query","trace_id":"header","payload":"body"}}`)
	input.ResponseMapping = json.RawMessage(`{"result_path":"data"}`)
	input.AuthorizationPolicy = json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":true}`)
	fullName := publishStaticTool(t, service, input)
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/items/item-42" || request.URL.Query().Get("filter") != "active" {
			t.Fatalf("mapped URL = %s", request.URL)
		}
		if key := request.Header.Get("Idempotency-Key"); request.Header.Get("trace-id") != "trace-7" || !strings.HasPrefix(key, "doko_") || key == "mapped-invocation-0042" {
			t.Fatalf("mapped headers = %#v", request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != `{"payload":"hello"}` {
			t.Fatalf("mapped body = %s", body)
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"ok":true},"ignored":"value"}`))}, nil
	}))
	value, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{"item_id": "item-42", "filter": "active", "trace_id": "trace-7", "payload": "hello"}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "request-42", IdempotencyKey: "mapped-invocation-0042"})
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["ok"] != true {
		t.Fatalf("mapped output = %#v", value)
	}
}

func TestRuntimePreservesInvocationIdempotencyKeyAcrossRetryRequestIDs(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	input := upstreamToolInput("stable_retry", http.MethodPost, "https://api.example.test/items", platform.ToolUpstreamAuth{Type: "none"}, "")
	input.AuthorizationPolicy = json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":true}`)
	fullName := publishStaticTool(t, service, input)
	var upstreamKeys []string
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		upstreamKeys = append(upstreamKeys, request.Header.Get("Idempotency-Key"))
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))

	const invocationKey = "stable-retry-001"
	for _, requestID := range []string{"request-attempt-one", "request-attempt-two"} {
		if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: requestID, IdempotencyKey: invocationKey}); err != nil {
			t.Fatal(err)
		}
	}
	if len(upstreamKeys) != 2 || upstreamKeys[0] != upstreamKeys[1] || upstreamKeys[0] == invocationKey || !strings.HasPrefix(upstreamKeys[0], "doko_") {
		t.Fatalf("upstream idempotency keys = %#v", upstreamKeys)
	}
}

func TestRuntimeRejectsMissingOrInvalidRequiredIdempotencyKey(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	input := upstreamToolInput("invalid_idempotency", http.MethodPost, "https://api.example.test/items", platform.ToolUpstreamAuth{Type: "none"}, "")
	input.AuthorizationPolicy = json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":true}`)
	fullName := publishStaticTool(t, service, input)
	calls := 0
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))

	for name, key := range map[string]string{
		"missing":           "",
		"below lower bound": strings.Repeat("a", 15),
		"above upper bound": strings.Repeat("a", 201),
		"not visible ASCII": strings.Repeat("a", 15) + " ",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "request-id-must-not-be-used", IdempotencyKey: key})
			if !errors.Is(err, tools.ErrInvalidIdempotencyKey) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid idempotency metadata performed %d upstream calls", calls)
	}
}

func TestRuntimeExchangesAndCachesOAuthClientCredential(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x65}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	auth := platform.ToolUpstreamAuth{Type: "oauth_client_credentials", ClientID: "tool-client", TokenURL: "https://identity.example.test/oauth/token", Scopes: []string{"status.read"}, Audience: "status-api"}
	fullName := publishStaticTool(t, service, upstreamToolInput("oauth", http.MethodGet, "https://api.example.test/status", auth, "client-secret"))
	tokenCalls, apiCalls := 0, 0
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "identity.example.test" {
			tokenCalls++
			username, password, ok := request.BasicAuth()
			if !ok || username != "tool-client" || password != "client-secret" {
				t.Fatalf("token basic auth = %q %q %v", username, password, ok)
			}
			body, _ := io.ReadAll(request.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("grant_type") != "client_credentials" || values.Get("scope") != "status.read" || values.Get("audience") != "status-api" {
				t.Fatalf("token form = %#v", values)
			}
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"short-lived-token","token_type":"Bearer","expires_in":300}`))}, nil
		}
		apiCalls++
		if request.Header.Get("Authorization") != "Bearer short-lived-token" {
			t.Fatalf("API authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	runtime.SetCredentialResolver(service)
	for range 2 {
		if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls != 1 || apiCalls != 2 {
		t.Fatalf("token calls = %d, API calls = %d", tokenCalls, apiCalls)
	}
}

func TestRuntimeFormEncodesOAuthClientSecretBasicCredentials(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x66}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	auth := platform.ToolUpstreamAuth{Type: "oauth_client_credentials", ClientID: "client:id with space/+", TokenURL: "https://identity.example.test/oauth/token"}
	fullName := publishStaticTool(t, service, upstreamToolInput("oauth_basic_encoding", http.MethodGet, "https://api.example.test/status", auth, "s:e cr/et+?&=%"))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "identity.example.test" {
			username, password, ok := request.BasicAuth()
			if !ok || username != "client%3Aid+with+space%2F%2B" || password != "s%3Ae+cr%2Fet%2B%3F%26%3D%25" {
				t.Fatalf("token basic auth = %q %q %v", username, password, ok)
			}
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"encoded-basic-token","token_type":"Bearer","expires_in":300}`))}, nil
		}
		if request.Header.Get("Authorization") != "Bearer encoded-basic-token" {
			t.Fatalf("API authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	runtime.SetCredentialResolver(service)
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeRejectsNonObjectToolOutput(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	fullName := publishStaticTool(t, service, upstreamToolInput("scalar", http.MethodGet, "https://api.example.test/status", platform.ToolUpstreamAuth{Type: "none"}, ""))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	}))
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err == nil || !strings.Contains(err.Error(), "must resolve to an object") {
		t.Fatalf("non-object output error = %v", err)
	}
}

func TestRuntimeSupportsClientSecretPostAndLocalhostOAuth(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x66}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	auth := platform.ToolUpstreamAuth{Type: "oauth_client_credentials", ClientID: "local-client", TokenURL: "http://identity.vendor.localhost:18081/oauth/token", TokenEndpointAuthMethod: "client_secret_post", Scopes: []string{"status.read"}, Resource: "urn:vendor:status"}
	fullName := publishStaticTool(t, service, upstreamToolInput("oauth_post", http.MethodGet, "http://api.vendor.localhost:18080/status", auth, "local-client-secret"))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("127.0.0.1")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() == "identity.vendor.localhost" {
			if _, _, ok := request.BasicAuth(); ok {
				t.Fatal("client_secret_post unexpectedly used HTTP Basic authentication")
			}
			body, _ := io.ReadAll(request.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("client_id") != "local-client" || values.Get("client_secret") != "local-client-secret" || values.Get("resource") != "urn:vendor:status" {
				t.Fatalf("token form = %#v", values)
			}
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"local-token","token_type":"Bearer","expires_in":300}`))}, nil
		}
		if request.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("API authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	runtime.SetCredentialResolver(service)
	runtime.SetPrivateLocalhostHosts([]string{"identity.vendor.localhost:18081", "api.vendor.localhost:18080"})
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDoesNotLeakAPIKeyQueryInTransportErrors(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x67}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	secret := "weak-query-secret"
	fullName := publishStaticTool(t, service, upstreamToolInput("query_error", http.MethodGet, "https://api.example.test/status", platform.ToolUpstreamAuth{Type: "api_key_query", QueryName: "api_key"}, secret))
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed for " + request.URL.String())
	}))
	runtime.SetCredentialResolver(service)
	_, err = runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"})
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "api_key=") {
		t.Fatalf("transport error leaked query credential: %v", err)
	}
}

func TestRuntimeRejectsNonScalarValueForCorruptedURLMapping(t *testing.T) {
	memory := store.NewMemory()
	created, err := memory.CreateTool(context.Background(), model.Tool{ID: "tool_corrupted_mapping", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "upstream", Name: "corrupted_mapping", Description: "Corrupted mapping fixture.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"filters":{"type":"object","additionalProperties":false,"properties":{}}},"required":["filters"]}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`), BaseURL: "https://api.example.test/status", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{"parameter_locations":{"filters":"query"}}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`), TimeoutMS: 5000, BackendKind: "http"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = memory.PublishTool(context.Background(), created.ProductID, created.ID, created.Revision, "root"); err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("runtime called upstream with a non-scalar query value")
		return nil, nil
	}))
	_, err = runtime.Execute(context.Background(), "prod_acme", "upstream.corrupted_mapping", map[string]any{"filters": map[string]any{}}, tools.Principal{Subject: "root-test", Grants: map[string]bool{}, RequestID: "execute"})
	if err == nil || !strings.Contains(err.Error(), "scalar") {
		t.Fatalf("non-scalar runtime mapping error = %v", err)
	}
}

func TestRuntimeRejectsUnresolvedPathPlaceholderBeforeNetworkCall(t *testing.T) {
	memory := store.NewMemory()
	created, err := memory.CreateTool(context.Background(), model.Tool{
		ID:                  "tool_corrupted_optional_path",
		OrganisationID:      "org_acme",
		ProductID:           "prod_acme",
		Namespace:           "upstream",
		Name:                "optional_path",
		Description:         "Corrupted optional path fixture.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"item_id":{"type":"string"}}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		BaseURL:             "https://api.example.test/items/{item_id}",
		HTTPMethod:          http.MethodDelete,
		UpstreamAuth:        json.RawMessage(`{"type":"none"}`),
		RequestMapping:      json.RawMessage(`{"parameter_locations":{"item_id":"path"}}`),
		ResponseMapping:     json.RawMessage(`{}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":false}`),
		TimeoutMS:           5000,
		BackendKind:         "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = memory.PublishTool(context.Background(), created.ProductID, created.ID, created.Revision, "root"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	if _, err := runtime.Execute(context.Background(), created.ProductID, created.Namespace+"."+created.Name, map[string]any{}, tools.Principal{Subject: "root", Grants: map[string]bool{}}); err == nil || !strings.Contains(err.Error(), "path argument") {
		t.Fatalf("unresolved path error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("unresolved path made %d upstream calls", calls)
	}
}

func TestRuntimeRejectsLegacyEndpointQueryBeforeNetworkCall(t *testing.T) {
	memory := store.NewMemory()
	created, err := memory.CreateTool(context.Background(), model.Tool{
		ID:                  "tool_legacy_endpoint_query",
		OrganisationID:      "org_acme",
		ProductID:           "prod_acme",
		Namespace:           "upstream",
		Name:                "legacy_query",
		Description:         "Legacy endpoint query fixture.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		BaseURL:             "https://api.example.test/items?api_key=legacy-secret",
		HTTPMethod:          http.MethodGet,
		UpstreamAuth:        json.RawMessage(`{"type":"none"}`),
		RequestMapping:      json.RawMessage(`{}`),
		ResponseMapping:     json.RawMessage(`{}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:           5000,
		BackendKind:         "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = memory.PublishTool(context.Background(), created.ProductID, created.ID, created.Revision, "root"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("must not be called")
	}))
	if _, err := runtime.Execute(context.Background(), created.ProductID, created.Namespace+"."+created.Name, map[string]any{}, tools.Principal{Subject: "root", Grants: map[string]bool{}}); !errors.Is(err, tools.ErrUnsafeDestination) {
		t.Fatalf("legacy query error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("legacy query made %d upstream calls", calls)
	}
}

func TestRuntimePreservesLargeIntegerOutputExactly(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	input := upstreamToolInput("large_integer", http.MethodGet, "https://api.example.test/items", platform.ToolUpstreamAuth{Type: "none"}, "")
	input.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"record_id":{"type":"integer"}},"required":["record_id"]}`)
	fullName := publishStaticTool(t, service, input)
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"record_id":9007199254740993}`))}, nil
	}))
	value, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root", Grants: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	identifier, ok := value.(map[string]any)["record_id"].(json.Number)
	if !ok || identifier.String() != "9007199254740993" {
		t.Fatalf("large integer output = %#v", value)
	}
}

func TestRuntimeAuditsEverySanitizedHTTPExecutionOutcome(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	fullName := publishStaticTool(t, service, upstreamToolInput("audit_outcomes", http.MethodGet, "https://api.example.test/items", platform.ToolUpstreamAuth{Type: "none"}, ""))
	tests := []struct {
		name     string
		category string
		do       runtimeDoer
	}{
		{name: "upstream status", category: "upstream_status", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Status: "500 Internal Server Error", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`secret-response-marker`))}, nil
		}},
		{name: "timeout after send", category: "transport_failed", do: func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }},
		{name: "invalid JSON", category: "response_invalid", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`secret-invalid-json-marker`))}, nil
		}},
		{name: "schema mismatch", category: "response_schema_mismatch", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":"secret-schema-marker"}`))}, nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before, err := memory.AuditEvents(context.Background(), "org_acme")
			if err != nil {
				t.Fatal(err)
			}
			runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, test.do)
			if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "root", Grants: map[string]bool{}, RequestID: "request-audit"}); err == nil {
				t.Fatal("failing upstream response unexpectedly succeeded")
			}
			after, err := memory.AuditEvents(context.Background(), "org_acme")
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(before)+1 {
				t.Fatalf("audit count before=%d after=%d", len(before), len(after))
			}
			event := after[len(after)-1]
			if event.Action != "tool.executed" || event.Outcome != "failure" || event.Current["category"] != test.category {
				t.Fatalf("audit event = %#v", event)
			}
			encoded, _ := json.Marshal(event)
			for _, secret := range []string{"secret-response-marker", "secret-invalid-json-marker", "secret-schema-marker", "api.example.test"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("audit leaked %q: %s", secret, encoded)
				}
			}
		})
	}
}

func TestRuntimeEnforcesPerProcessSharedConnectionRateLimit(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	fullName := publishStaticTool(t, service, upstreamToolInput("connection_limit", http.MethodGet, "https://api.example.test/items", platform.ToolUpstreamAuth{Type: "none"}, ""))
	calls := 0
	runtime := tools.NewRuntime(memory, runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	}))
	for attempt := 0; attempt < 60; attempt++ {
		if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "user", Grants: map[string]bool{}}); err != nil {
			t.Fatalf("attempt %d failed: %v", attempt+1, err)
		}
	}
	if _, err := runtime.Execute(context.Background(), "prod_acme", fullName, map[string]any{}, tools.Principal{Subject: "different-user", Grants: map[string]bool{}}); !errors.Is(err, tools.ErrRateLimited) {
		t.Fatalf("61st per-process shared-connection call error = %v", err)
	}
	if calls != 60 {
		t.Fatalf("rate-limited call reached upstream: calls=%d", calls)
	}
}
