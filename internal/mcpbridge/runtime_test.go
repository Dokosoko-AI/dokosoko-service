package mcpbridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type fixedResolver struct{ address net.IP }

func (r fixedResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

type recordingDoer func(*http.Request) (*http.Response, error)

func (function recordingDoer) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func response(contentType, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}

func managerForTest(t *testing.T, doer recordingDoer) (*mcpbridge.Manager, *store.Memory, *secrets.Vault) {
	t.Helper()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x6a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return mcpbridge.New(memory, vault, fixedResolver{net.ParseIP("8.8.8.8")}, doer), memory, vault
}

func decodeRequest(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.NewDecoder(request.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func rpcResult(t *testing.T, request map[string]any, rawResult string) string {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(rawResult), &result); err != nil {
		t.Fatal(err)
	}
	result["_meta"] = map[string]any{"io.modelcontextprotocol/serverInfo": map[string]any{"name": "Test upstream", "version": "2.0.0"}}
	encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestManagedImportPinsSchemasAndRuntimeAuthorizesBeforeServiceCall(t *testing.T) {
	t.Parallel()
	version := 1
	listCalls := 0
	toolCalls := 0
	missingStructuredOutput := false
	manager, memory, vault := managerForTest(t, recordingDoer(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("MCP-Protocol-Version") != model.StatelessMCPv2Protocol {
			t.Fatalf("protocol header = %q", request.Header.Get("MCP-Protocol-Version"))
		}
		if request.Header.Get("Authorization") != "Bearer upstream-service-token" {
			t.Fatalf("upstream authorization = %q", request.Header.Get("Authorization"))
		}
		body := decodeRequest(t, request)
		method, _ := body["method"].(string)
		if request.Header.Get("Mcp-Method") != method {
			t.Fatalf("Mcp-Method = %q, body method = %q", request.Header.Get("Mcp-Method"), method)
		}
		params := body["params"].(map[string]any)
		meta := params["_meta"].(map[string]any)
		if meta["io.modelcontextprotocol/protocolVersion"] != model.StatelessMCPv2Protocol {
			t.Fatalf("request metadata = %#v", meta)
		}
		switch method {
		case "tools/list":
			listCalls++
			schema := `{"type":"object","additionalProperties":false,"properties":{"title":{"type":"string"}},"required":["title"]}`
			if version == 2 {
				schema = `{"type":"object","additionalProperties":false,"properties":{"title":{"type":"string"},"priority":{"type":"string"}},"required":["title"]}`
			}
			return response("application/json", rpcResult(t, body, `{"resultType":"complete","ttlMs":30000,"tools":[{"name":"incidents.create","description":"Create an incident","inputSchema":`+schema+`,"outputSchema":{"type":"object","additionalProperties":false,"properties":{"incident_id":{"type":"string"}},"required":["incident_id"]},"annotations":{"destructiveHint":false}}]}`)), nil
		case "tools/call":
			toolCalls++
			if request.Header.Get("Mcp-Name") != "incidents.create" {
				t.Fatalf("Mcp-Name = %q", request.Header.Get("Mcp-Name"))
			}
			if missingStructuredOutput {
				return response("application/json", rpcResult(t, body, `{"resultType":"complete","content":[{"type":"text","text":"created"}],"isError":false}`)), nil
			}
			return response("application/json", rpcResult(t, body, `{"resultType":"complete","content":[{"type":"text","text":"created"}],"structuredContent":{"incident_id":"inc_42"},"isError":false}`)), nil
		default:
			t.Fatalf("unexpected method %q", method)
			return nil, nil
		}
	}))

	connection, err := manager.CreateConnection(context.Background(), mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Support", Namespace: "support", Endpoint: "https://mcp.vendor.example/v2", AccessToken: "upstream-service-token"}, mcpbridge.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := manager.Import(context.Background(), "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.create"}, RequiredGrants: []string{"support.write"}, ConfirmationRequired: false, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root", RequestID: "import"})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Created) != 1 || imported.Created[0].State != "draft" || imported.Created[0].UpstreamSchemaHash == "" {
		t.Fatalf("import result = %#v", imported)
	}
	service := platform.NewWithVault(memory, vault)
	dryRun, err := service.DryRunTool(context.Background(), "prod_acme", imported.Created[0].ID, map[string]any{"title": "Help"})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Risk != "medium" {
		t.Fatalf("imported MCP risk = %q", dryRun.Risk)
	}
	if _, err := service.SaveGrantDefinition(context.Background(), "", platform.GrantDefinitionInput{Key: "support.write", DisplayName: "Write support data", Risk: "high", State: "active"}, platform.Actor{ID: "root"}); err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishTool(context.Background(), "prod_acme", imported.Created[0].ID, imported.Created[0].Revision, platform.Actor{ID: "root", RequestID: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloneTool(context.Background(), "prod_acme", published.ID, platform.ToolCloneInput{Namespace: "support_copy", Name: "incidents_create", Revision: published.Revision}, platform.Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "MCP tools cannot be cloned") {
		t.Fatalf("imported MCP clone error = %v", err)
	}
	runtime := tools.NewRuntime(memory, nil, nil)
	runtime.SetMCPExecutor(manager)
	if _, err := runtime.Execute(context.Background(), "prod_acme", "support.incidents.create", map[string]any{"title": "Help"}, tools.Principal{Subject: "user_1", Grants: map[string]bool{}}); !errors.Is(err, tools.ErrDenied) {
		t.Fatalf("missing grant error = %v", err)
	}
	if toolCalls != 0 {
		t.Fatalf("upstream was called before authorization: %d", toolCalls)
	}
	value, err := runtime.Execute(context.Background(), "prod_acme", "support.incidents.create", map[string]any{"title": "Help"}, tools.Principal{Subject: "user_1", Grants: map[string]bool{"support.write": true}, RequestID: "execute"})
	if err != nil {
		t.Fatal(err)
	}
	if value.(tools.MCPCallResult).Result["resultType"] != "complete" || toolCalls != 1 {
		t.Fatalf("value=%#v toolCalls=%d", value, toolCalls)
	}
	missingStructuredOutput = true
	if _, err := runtime.Execute(context.Background(), "prod_acme", "support.incidents.create", map[string]any{"title": "Help"}, tools.Principal{Subject: "user_1", Grants: map[string]bool{"support.write": true}}); err == nil || !strings.Contains(err.Error(), "structuredContent is required") {
		t.Fatalf("missing structured output error = %v", err)
	}
	missingStructuredOutput = false

	version = 2
	drift, err := manager.Import(context.Background(), "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.create"}, RequiredGrants: []string{"support.write"}, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root", RequestID: "inspect-again"})
	if err != nil {
		t.Fatal(err)
	}
	if len(drift.Drifted) != 1 || !drift.Drifted[0].UpstreamDrifted || drift.Drifted[0].ID != published.ID {
		t.Fatalf("drift result = %#v", drift)
	}
	if drift.Drifted[0].Revision != published.Revision {
		t.Fatalf("operational drift changed contract revision: published=%d drifted=%d", published.Revision, drift.Drifted[0].Revision)
	}
	if _, err := runtime.Execute(context.Background(), "prod_acme", "support.incidents.create", map[string]any{"title": "Help"}, tools.Principal{Subject: "user_1", Grants: map[string]bool{"support.write": true}}); !errors.Is(err, tools.ErrDenied) {
		t.Fatalf("drifted execution error = %v", err)
	}
	version = 1
	recovered, err := manager.Import(context.Background(), "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.create"}, RequiredGrants: []string{"support.write"}, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root", RequestID: "inspect-recovered"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.Unchanged) != 1 || recovered.Unchanged[0].UpstreamDrifted || recovered.Unchanged[0].Revision != published.Revision {
		t.Fatalf("recovered result = %#v", recovered)
	}
	if _, err := runtime.Execute(context.Background(), "prod_acme", "support.incidents.create", map[string]any{"title": "Help"}, tools.Principal{Subject: "user_1", Grants: map[string]bool{"support.write": true}}); err != nil {
		t.Fatalf("recovered execution error = %v", err)
	}
	if listCalls < 2 {
		t.Fatalf("listCalls = %d", listCalls)
	}
}

func TestImportFailsClosedForDuplicateUpstreamToolIdentities(t *testing.T) {
	t.Parallel()
	manager, memory, _ := managerForTest(t, recordingDoer(func(request *http.Request) (*http.Response, error) {
		body := decodeRequest(t, request)
		return response("application/json", rpcResult(t, body, `{"resultType":"complete","tools":[{"name":"incidents.create","description":"Create an incident","inputSchema":{"type":"object","additionalProperties":false,"properties":{}},"outputSchema":{"type":"object","additionalProperties":false,"properties":{}}}]}`)), nil
	}))
	ctx := context.Background()
	connection, err := manager.CreateConnection(ctx, mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Duplicate guard", Namespace: "support_duplicate", Endpoint: "https://mcp.vendor.example/v2", AccessToken: "upstream-token"}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := manager.Import(ctx, "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.create"}, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root"})
	if err != nil || len(imported.Created) != 1 {
		t.Fatalf("initial import = %#v, %v", imported, err)
	}
	duplicate := imported.Created[0]
	duplicate.ID, duplicate.Name, duplicate.State, duplicate.Revision = "duplicate_upstream_tool", "incidents_create_copy", "draft", 0
	duplicate, err = memory.CreateTool(ctx, duplicate)
	if err != nil {
		t.Fatal(err)
	}
	service := platform.New(memory)
	first, err := service.PublishTool(ctx, "prod_acme", imported.Created[0].ID, imported.Created[0].Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.PublishTool(ctx, "prod_acme", duplicate.ID, duplicate.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Import(ctx, "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.create"}, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Rejected["incidents.create"], "multiple local tools") || len(result.Drifted) != 2 {
		t.Fatalf("duplicate import result = %#v", result)
	}
	for _, published := range []model.Tool{first, second} {
		current, lookupErr := memory.Tool(ctx, published.ProductID, published.ID)
		if lookupErr != nil {
			t.Fatal(lookupErr)
		}
		if !current.UpstreamDrifted || current.Revision != published.Revision {
			t.Fatalf("duplicate tool did not fail closed without changing revision: before=%#v after=%#v", published, current)
		}
	}
}

func TestAccessTokenProxyCanForwardSignedUserIdentity(t *testing.T) {
	t.Parallel()
	var callAuthorization, identityHeader, signatureHeader string
	var identityMeta map[string]any
	manager, memory, vault := managerForTest(t, recordingDoer(func(request *http.Request) (*http.Response, error) {
		body := decodeRequest(t, request)
		method, _ := body["method"].(string)
		if method == "tools/list" {
			if request.Header.Get("X-DokoSoko-User") != "" {
				t.Fatal("user identity was forwarded during catalog inspection")
			}
			return response("application/json", rpcResult(t, body, `{"resultType":"complete","tools":[{"name":"incidents.comment","description":"Comment","inputSchema":{"type":"object","additionalProperties":false,"properties":{"body":{"type":"string"}},"required":["body"]}}]}`)), nil
		}
		callAuthorization = request.Header.Get("Authorization")
		identityHeader, signatureHeader = request.Header.Get("X-DokoSoko-User"), request.Header.Get("X-DokoSoko-User-Signature")
		params := body["params"].(map[string]any)
		meta := params["_meta"].(map[string]any)
		identityMeta, _ = meta["com.dokosoko/userIdentity"].(map[string]any)
		return response("application/json", rpcResult(t, body, `{"resultType":"complete","content":[{"type":"text","text":"ok"}],"isError":false}`)), nil
	}))
	connection, err := manager.CreateConnection(context.Background(), mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Support users", Namespace: "support_user", Endpoint: "https://mcp.vendor.example/v2", AccessToken: "upstream-service-token", ForwardUserIdentity: true}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	principal := tools.Principal{Issuer: "https://identity.vendor.example", Subject: "doko-user-7", ExternalCustomerID: "org_customer", Grants: map[string]bool{"support.write": true}}
	imported, err := manager.Import(context.Background(), "prod_acme", connection.ID, mcpbridge.ImportInput{ToolNames: []string{"incidents.comment"}, RequiredGrants: []string{"support.write"}, TimeoutMS: 5000}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	if _, err := service.SaveGrantDefinition(context.Background(), "", platform.GrantDefinitionInput{Key: "support.write", DisplayName: "Write support data", Risk: "high", State: "active"}, platform.Actor{ID: "root"}); err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishTool(context.Background(), "prod_acme", imported.Created[0].ID, imported.Created[0].Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ExecuteMCP(context.Background(), published, map[string]any{"body": "Update"}, principal); err != nil {
		t.Fatal(err)
	}
	if callAuthorization != "Bearer upstream-service-token" {
		t.Fatalf("tool authorization = %q", callAuthorization)
	}
	if identityHeader == "" || !strings.HasPrefix(signatureHeader, "sha256=") || identityMeta["subject"] != "doko-user-7" || identityMeta["external_customer_id"] != "org_customer" {
		t.Fatalf("identity header=%q signature=%q meta=%#v", identityHeader, signatureHeader, identityMeta)
	}
}

func TestBridgeRejectsPrivateNetworkResolution(t *testing.T) {
	t.Parallel()
	called := false
	memory := store.NewMemory()
	vault, _ := secrets.New(bytes.Repeat([]byte{0x2c}, 32))
	manager := mcpbridge.New(memory, vault, fixedResolver{net.ParseIP("127.0.0.1")}, recordingDoer(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not be called")
	}))
	connection, err := manager.CreateConnection(context.Background(), mcpbridge.ConnectionInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Unsafe", Namespace: "unsafe", Endpoint: "https://internal.example/v2", AccessToken: "upstream-token"}, mcpbridge.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Inspect(context.Background(), "prod_acme", connection.ID); !errors.Is(err, mcpbridge.ErrUnsafeDestination) {
		t.Fatalf("inspection error = %v", err)
	}
	if called {
		t.Fatal("network client was called for a private destination")
	}
}
