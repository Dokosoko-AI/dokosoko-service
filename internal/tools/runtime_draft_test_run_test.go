package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

func draftTestTool(method string) model.Tool {
	return model.Tool{
		ID: "tool_test", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "test", Name: "shape", BackendKind: "http", State: "draft", Revision: 3,
		BaseURL: "https://api.example.test/items", HTTPMethod: method, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		InputSchema:    json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema:   json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"detail":{"type":"string"}},"required":["ok","detail"]}`),
		RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`),
		AuthorizationPolicy: json.RawMessage(fmt.Sprintf(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":%t}`, method != http.MethodGet)), TimeoutMS: 1000,
	}
}

type liveCredentialResolver struct{ value string }

func (r liveCredentialResolver) ResolveToolCredential(context.Context, model.Tool) ([]byte, error) {
	return []byte(r.value), nil
}

func TestDraftTestReusesAllNonDelegatedUpstreamAuthenticationModes(t *testing.T) {
	const credential = "private-upstream-credential"
	tests := []struct {
		name  string
		auth  json.RawMessage
		check func(*testing.T, *http.Request)
	}{
		{name: "none", auth: json.RawMessage(`{"type":"none"}`), check: func(t *testing.T, request *http.Request) {}},
		{name: "bearer", auth: json.RawMessage(`{"type":"bearer"}`), check: func(t *testing.T, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer "+credential {
				t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
			}
		}},
		{name: "authorization scheme", auth: json.RawMessage(`{"type":"authorization_scheme","scheme":"SSWS"}`), check: func(t *testing.T, request *http.Request) {
			if request.Header.Get("Authorization") != "SSWS "+credential {
				t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
			}
		}},
		{name: "API key header", auth: json.RawMessage(`{"type":"api_key_header","header_name":"X-API-Key"}`), check: func(t *testing.T, request *http.Request) {
			if request.Header.Get("X-API-Key") != credential {
				t.Fatalf("api key=%q", request.Header.Get("X-API-Key"))
			}
		}},
		{name: "API key query", auth: json.RawMessage(`{"type":"api_key_query","query_name":"api_key"}`), check: func(t *testing.T, request *http.Request) {
			if request.URL.Query().Get("api_key") != credential {
				t.Fatalf("api key query missing")
			}
		}},
		{name: "basic", auth: json.RawMessage(`{"type":"basic","username":"vendor-user"}`), check: func(t *testing.T, request *http.Request) {
			username, password, ok := request.BasicAuth()
			if !ok || username != "vendor-user" || password != credential {
				t.Fatalf("basic auth=%q/%q/%v", username, password, ok)
			}
		}},
		{name: "custom header", auth: json.RawMessage(`{"type":"custom_header","header_name":"X-Vendor-Auth","prefix":"Token"}`), check: func(t *testing.T, request *http.Request) {
			if request.Header.Get("X-Vendor-Auth") != "Token "+credential {
				t.Fatalf("custom header=%q", request.Header.Get("X-Vendor-Auth"))
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := draftTestTool(http.MethodGet)
			tool.UpstreamAuth, tool.APIConnectionID = test.auth, "connection-test"
			if test.name != "none" {
				tool.CredentialID = "credential-test"
			}
			calls := 0
			runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
				calls++
				test.check(t, request)
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
			}))
			runtime.SetCredentialResolver(liveCredentialResolver{value: credential})
			report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x"}, tools.Principal{Subject: "root"})
			encoded, _ := json.Marshal(report)
			if calls != 1 || report.Outcome != "success" || strings.Contains(string(encoded), credential) {
				t.Fatalf("calls=%d report=%#v encoded=%s", calls, report, encoded)
			}
		})
	}
	t.Run("OAuth client credentials", func(t *testing.T) {
		tool := draftTestTool(http.MethodGet)
		tool.APIConnectionID, tool.CredentialID = "oauth-connection", "oauth-credential"
		tool.UpstreamAuth = json.RawMessage(`{"type":"oauth_client_credentials","client_id":"client-id","token_url":"https://auth.example.test/oauth/token","token_endpoint_auth_method":"client_secret_basic"}`)
		calls := 0
		runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Hostname() == "auth.example.test" {
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"private-oauth-access-token","token_type":"Bearer","expires_in":300}`))}, nil
			}
			if request.Header.Get("Authorization") != "Bearer private-oauth-access-token" {
				t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
			}
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x"}`))}, nil
		}))
		runtime.SetCredentialResolver(liveCredentialResolver{value: credential})
		report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x"}, tools.Principal{Subject: "root"})
		encoded, _ := json.Marshal(report)
		if calls != 2 || report.Outcome != "success" || strings.Contains(string(encoded), credential) || strings.Contains(string(encoded), "private-oauth-access-token") {
			t.Fatalf("calls=%d report=%#v encoded=%s", calls, report, encoded)
		}
	})
}

func TestDraftTestPreflightAndDelegatedOAuthPerformNoCall(t *testing.T) {
	calls := 0
	runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("must not call")
	}))
	tool := draftTestTool(http.MethodGet)
	report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": 7}, tools.Principal{Subject: "root"})
	if calls != 0 || report.AuthenticationType != "none" || report.Phase != "preflight" || len(report.Findings) != 1 || report.Findings[0].Code != "input_schema_mismatch" || report.Findings[0].InstancePath != "/label" {
		t.Fatalf("calls=%d report=%#v", calls, report)
	}
	tool.UpstreamAuth = json.RawMessage(`{"type":"delegated_oauth"}`)
	report = runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "private-request-value"}, tools.Principal{Subject: "root", DelegatedAccessToken: "must-not-be-accepted"})
	if calls != 0 || report.Phase != "auth" || report.Findings[0].Code != "test_authorization_unavailable" {
		t.Fatalf("calls=%d report=%#v", calls, report)
	}
}

func TestDraftTestReportKeepsOnlyBoundedValueFreeShapes(t *testing.T) {
	calls := 0
	runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"private-response-value"}`))}, nil
	}))
	tool := draftTestTool(http.MethodPost)
	report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "private-request-value"}, tools.Principal{Subject: "root", IdempotencyKey: "draft-test-idempotency-0001"})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || report.Outcome != "success" || report.Phase != "success" || !report.NetworkCallPerformed || report.ResponseShape == nil {
		t.Fatalf("calls=%d report=%#v", calls, report)
	}
	if strings.Contains(string(encoded), "private-request-value") || strings.Contains(string(encoded), "private-response-value") {
		t.Fatalf("sanitized report leaked scalar values: %s", encoded)
	}
	if report.RequestShape.Properties["label"].Type != "string" || report.ResponseShape.Properties["detail"].Type != "string" {
		t.Fatalf("shape evidence = %#v / %#v", report.RequestShape, report.ResponseShape)
	}
}

func TestDraftTestProjectsUnexpectedResponseKeysAndDiagnosticPaths(t *testing.T) {
	const maliciousKey = "Authorization: Bearer live-secret"
	calls := 0
	runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"detail":"x","Authorization: Bearer live-secret":true}`))}, nil
	}))
	tool := draftTestTool(http.MethodGet)
	requestReport := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x", maliciousKey: true}, tools.Principal{Subject: "root"})
	requestEncoded, _ := json.Marshal(requestReport)
	if calls != 0 || requestReport.AuthenticationType != "none" || requestReport.Findings[0].InstancePath != "/[unexpected-property]" {
		t.Fatalf("unexpected request-key report=%#v calls=%d", requestReport, calls)
	}
	if _, ok := requestReport.RequestShape.Properties["[unexpected-property-1]"]; !ok || strings.Contains(string(requestEncoded), "live-secret") || strings.Contains(string(requestEncoded), "Authorization") {
		t.Fatalf("sanitized request evidence leaked an unexpected key: %s", requestEncoded)
	}
	report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x"}, tools.Principal{Subject: "root"})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || report.Outcome != "failure" || report.Phase != "output_schema" || report.ResponseShape == nil || report.Findings[0].InstancePath != "/[unexpected-property]" || report.Findings[0].SchemaPath != "/additionalProperties" {
		t.Fatalf("calls=%d report=%#v", calls, report)
	}
	if _, ok := report.ResponseShape.Properties["[unexpected-property-1]"]; !ok {
		t.Fatalf("unexpected key was not projected: %#v", report.ResponseShape)
	}
	if strings.Contains(string(encoded), maliciousKey) || strings.Contains(string(encoded), "live-secret") || strings.Contains(string(encoded), "Authorization") {
		t.Fatalf("sanitized report leaked an unexpected response key: %s", encoded)
	}
}

func TestDraftTestProjectsUnsafeDeclaredKeysAndDiagnosticPaths(t *testing.T) {
	const maliciousKey = "AuthorizationBearerLiveSecret"
	calls := 0
	runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"AuthorizationBearerLiveSecret":true}`))}, nil
	}))
	tool := draftTestTool(http.MethodGet)
	tool.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"AuthorizationBearerLiveSecret":{"type":"string"}},"required":["AuthorizationBearerLiveSecret"]}`)
	requestReport := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{maliciousKey: "value"}, tools.Principal{Subject: "root"})
	requestEncoded, _ := json.Marshal(requestReport)
	if calls != 0 || requestReport.Phase != "preflight" || requestReport.Findings[0].Code != "preflight_failed" {
		t.Fatalf("unsafe declared request-key report=%#v calls=%d", requestReport, calls)
	}
	if _, ok := requestReport.RequestShape.Properties["[schema-property-1]"]; !ok || strings.Contains(string(requestEncoded), maliciousKey) || strings.Contains(string(requestEncoded), "Authorization") || strings.Contains(string(requestEncoded), "LiveSecret") {
		t.Fatalf("sanitized request evidence leaked an unsafe declared key: %s", requestEncoded)
	}

	tool.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`)
	tool.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"AuthorizationBearerLiveSecret":{"type":"string"}},"required":["AuthorizationBearerLiveSecret"]}`)
	report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x"}, tools.Principal{Subject: "root"})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || report.Outcome != "failure" || report.Phase != "preflight" || report.Findings[0].Code != "preflight_failed" {
		t.Fatalf("calls=%d report=%#v", calls, report)
	}
	if strings.Contains(string(encoded), maliciousKey) || strings.Contains(string(encoded), "Authorization") || strings.Contains(string(encoded), "LiveSecret") {
		t.Fatalf("sanitized report leaked an unsafe declared response key: %s", encoded)
	}
}

func TestDraftTestNeverRetriesAndCategorizesResponseFailures(t *testing.T) {
	t.Run("transport", func(t *testing.T) {
		calls := 0
		runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("private transport detail")
		}))
		report := runtime.ExecuteHTTPDraftTest(context.Background(), "prod_acme", draftTestTool(http.MethodDelete), map[string]any{"label": "x"}, tools.Principal{Subject: "root", IdempotencyKey: "draft-test-idempotency-0002"})
		if calls != 1 || report.Phase != "transport" || report.Findings[0].Code != "transport_failed" {
			t.Fatalf("calls=%d report=%#v", calls, report)
		}
	})

	tests := []struct {
		name          string
		body          string
		mapping       json.RawMessage
		expectedPhase string
		expectedCode  string
	}{
		{name: "json", body: `{`, expectedPhase: "json", expectedCode: "invalid_json_response"},
		{name: "mapping", body: `{"ok":true,"detail":"x"}`, mapping: json.RawMessage(`{"result_path":"missing"}`), expectedPhase: "response_mapping", expectedCode: "response_mapping_failed"},
		{name: "schema", body: `{"ok":"wrong","detail":"x"}`, expectedPhase: "output_schema", expectedCode: "output_schema_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := draftTestTool(http.MethodGet)
			if test.mapping != nil {
				tool.ResponseMapping = test.mapping
			}
			calls := 0
			runtime := tools.NewRuntime(store.NewMemory(), runtimeResolver{address: net.ParseIP("8.8.8.8")}, runtimeDoer(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}))
			report := runtime.ExecuteHTTPDraftTest(context.Background(), tool.ProductID, tool, map[string]any{"label": "x"}, tools.Principal{Subject: "root"})
			if calls != 1 || report.Phase != test.expectedPhase || len(report.Findings) != 1 || report.Findings[0].Code != test.expectedCode {
				t.Fatalf("calls=%d report=%#v", calls, report)
			}
			if test.name == "schema" && (report.Findings[0].InstancePath != "/ok" || !strings.HasSuffix(report.Findings[0].SchemaPath, "/type")) {
				t.Fatalf("schema paths = %#v", report.Findings[0])
			}
		})
	}
}
