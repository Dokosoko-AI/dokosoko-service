package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type apiToolResolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f apiToolResolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type apiToolDoerFunc func(*http.Request) (*http.Response, error)

func (f apiToolDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func apiToolResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type apiToolFixture struct {
	server      *Server
	service     *platform.Service
	memory      *store.Memory
	integration model.Integration
	definition  model.AccessDefinition
	connection  model.AccessConnection
	manifest    model.ProductManifest
	principal   identity.Principal
}

func newAPIToolFixture(t *testing.T, doer apiToolDoerFunc) apiToolFixture {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	actor := platform.Actor{ID: "root"}
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice-api", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice contract.", Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	operations := `{"required_grants":["developer.pro"],"max_ttl_seconds":3600,"credential_storage_mode":"one_time","authorize":{"method":"POST","path":"/v1/authorize"},"instances.create":{"method":"POST","path":"/v1/instances"},"credentials.create":{"method":"POST","path":"/v1/credentials"},"credentials.revoke":{"method":"POST","path":"/v1/credentials/{credential_id}/revoke"}}`
	definition, err := service.CreateAccessDefinition(ctx, platform.AccessDefinitionInput{ServiceKey: "acme-voice", Name: "Acme Voice", InstanceCardinality: "many", InstanceLabelSingular: "Project", InstanceLabelPlural: "Projects", CredentialScope: "instance", ManagementAuthType: "bearer", Operations: []byte(operations)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := service.CreateAccessConnection(ctx, platform.AccessConnectionInput{AccessDefinitionID: definition.ID, EnvironmentID: "env_prod", Name: "Voice production", BaseURL: "https://provider.example", ManagementSecret: "management-secret", IntegrationIDs: []string{integration.ID}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if doer == nil {
		doer = func(request *http.Request) (*http.Response, error) {
			switch request.URL.Path {
			case "/v1/authorize":
				return apiToolResponse(`{"allowed":true}`), nil
			case "/v1/instances":
				return apiToolResponse(`{"instance_id":"provider-project-1","display_name":"SDK project","state":"active"}`), nil
			default:
				return apiToolResponse(`{}`), nil
			}
		}
	}
	resolver := apiToolResolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	runtime := accessruntime.New(memory, vault, resolver, doer)
	manifest := model.ProductManifest{DeploymentID: "prod_acme", ProductID: "prod_acme", Integrations: []model.IntegrationManifest{{ID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, Resources: []model.IntegrationManifestResource{{ResourceSetID: "docs", Kind: "documentation", SourcePublications: []model.IntegrationManifestSourcePublication{{ID: "pub_docs_seed", SourceID: "src_docs", Revision: 1}}}}, AccessConnections: []model.IntegrationManifestAccessConnection{{ConnectionID: connection.ID, ConnectionRevision: connection.Revision, AccessDefinitionID: definition.ID, AccessDefinitionRevision: definition.Revision, EnvironmentID: connection.EnvironmentID, State: "active"}}}}}
	principal := identity.Principal{ProductID: "prod_acme", Issuer: "https://id.vendor.example", Subject: "user-a", InstallationID: "installation-a", CustomerAccountID: "account-a", Grants: map[string]bool{"developer.pro": true}, AccessEvaluationID: "evaluation-a", AccessEvaluatedAt: time.Now().UTC()}
	return apiToolFixture{server: &Server{service: service, accessRuntime: runtime}, service: service, memory: memory, integration: integration, definition: definition, connection: connection, manifest: manifest, principal: principal}
}

func toolDefinitionByName(values []map[string]any, name string) map[string]any {
	for _, value := range values {
		if value["name"] == name {
			return value
		}
	}
	return nil
}

func TestAPIDefaultToolsBindKnowledgeAndDedicatedAdminWithoutClientSelectors(t *testing.T) {
	fixture := newAPIToolFixture(t, nil)
	definitions, bindings := fixture.server.apiDefaultToolDefinitions(context.Background(), "prod_acme", fixture.manifest, fixture.principal, false)
	for _, name := range []string{"voice-api.knowledge.search", "voice-api.admin.instances.list", "voice-api.admin.instances.create", "voice-api.admin.credentials.list", "voice-api.admin.credentials.rotate", "voice-api.admin.credentials.revoke"} {
		if toolDefinitionByName(definitions, name) == nil || bindings[name].IntegrationID != fixture.integration.ID {
			t.Fatalf("generated tool %q missing or unbound: %#v", name, definitions)
		}
	}
	rotation := toolDefinitionByName(definitions, "voice-api.admin.credentials.rotate")
	schema := rotation["inputSchema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	if _, ok := properties["connection_id"]; ok {
		t.Fatal("generated API admin tool exposes connection_id")
	}
	if _, ok := properties["integration_id"]; ok {
		t.Fatal("generated API admin tool exposes integration_id")
	}
	required := schema["required"].([]string)
	if slices.Contains(required, "rotated_from_credential_id") || !slices.Contains(required, "environment_id") || !slices.Contains(required, "scopes") {
		t.Fatalf("rotation bootstrap requirements = %#v", required)
	}
	if bindings["voice-api.admin.credentials.rotate"].EnvironmentVariable != "VOICE_API_KEY" {
		t.Fatalf("dedicated environment label = %q", bindings["voice-api.admin.credentials.rotate"].EnvironmentVariable)
	}
	metadata := rotation["_meta"].(map[string]any)
	if metadata["com.dokosoko/confirmationRequired"] != true || metadata["com.dokosoko/idempotencyKeyRequired"] != true {
		t.Fatalf("rotation safety metadata = %#v", metadata)
	}
	operationKey := bindings["voice-api.admin.credentials.rotate"].confirmationOperationKey()
	if strings.ContainsRune(operationKey, '\x00') || !strings.HasPrefix(operationKey, "api-admin-v1:") || len(operationKey) != len("api-admin-v1:")+64 {
		t.Fatalf("database-safe confirmation operation key = %q", operationKey)
	}
}

func TestAPIDefaultAdminUsesServiceSegmentsAndSharedEnvironmentLabel(t *testing.T) {
	fixture := newAPIToolFixture(t, nil)
	ctx := context.Background()
	actor := platform.Actor{ID: "root"}
	sharedIntegration, err := fixture.service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "face-api", VersionKey: "v1", DisplayName: "Face API", Description: "Face contract.", Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.SetIntegrationAccessConnections(ctx, sharedIntegration.ID, []string{fixture.connection.ID}, actor); err != nil {
		t.Fatal(err)
	}
	secondDefinition, err := fixture.service.CreateAccessDefinition(ctx, platform.AccessDefinitionInput{ServiceKey: "secondary-admin", Name: "Secondary Admin", InstanceCardinality: "one", InstanceLabelSingular: "Account", InstanceLabelPlural: "Accounts", CredentialScope: "connection", ManagementAuthType: "bearer", Operations: []byte(`{"required_grants":["developer.pro"],"authorize":{"method":"POST","path":"/v1/authorize"},"credentials.create":{"method":"POST","path":"/v1/credentials"},"credentials.revoke":{"method":"POST","path":"/v1/credentials/{credential_id}/revoke"}}`)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	secondConnection, err := fixture.service.CreateAccessConnection(ctx, platform.AccessConnectionInput{AccessDefinitionID: secondDefinition.ID, EnvironmentID: "env_prod", Name: "Secondary", BaseURL: "https://secondary.example", ManagementSecret: "secondary-secret", IntegrationIDs: []string{fixture.integration.ID}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	fixture.manifest.Integrations[0].AccessConnections = append(fixture.manifest.Integrations[0].AccessConnections, model.IntegrationManifestAccessConnection{ConnectionID: secondConnection.ID, ConnectionRevision: secondConnection.Revision, AccessDefinitionID: secondDefinition.ID, AccessDefinitionRevision: secondDefinition.Revision, EnvironmentID: secondConnection.EnvironmentID, State: "active"})
	definitions, bindings := fixture.server.apiDefaultToolDefinitions(ctx, "prod_acme", fixture.manifest, fixture.principal, false)
	if toolDefinitionByName(definitions, "voice-api.admin.acme-voice.credentials.rotate") == nil || toolDefinitionByName(definitions, "voice-api.admin.secondary-admin.credentials.rotate") == nil {
		t.Fatalf("multiple management connections were not deterministically segmented: %#v", definitions)
	}
	if bindings["voice-api.admin.acme-voice.credentials.rotate"].EnvironmentVariable != "SERVICE_API_KEY" {
		t.Fatalf("shared management connection label = %q", bindings["voice-api.admin.acme-voice.credentials.rotate"].EnvironmentVariable)
	}
}

func rpcResponseMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("invalid RPC response %q: %v", recorder.Body.String(), err)
	}
	return value
}

func confirmationFromResponse(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	response := rpcResponseMap(t, recorder)
	errorValue, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("confirmation response has no error: %#v", response)
	}
	data, ok := errorValue["data"].(map[string]any)
	if !ok {
		t.Fatalf("confirmation response has no data: %#v", response)
	}
	challenge, _ := data["confirmation_challenge"].(string)
	if challenge == "" {
		t.Fatalf("confirmation response has no challenge: %#v", response)
	}
	return challenge
}

func TestAPIMutationConfirmationRejectsMismatchAndReplayThenExecutesExactRetry(t *testing.T) {
	providerCalls := 0
	fixture := newAPIToolFixture(t, func(request *http.Request) (*http.Response, error) {
		providerCalls++
		switch request.URL.Path {
		case "/v1/authorize":
			return apiToolResponse(`{"allowed":true}`), nil
		case "/v1/instances":
			return apiToolResponse(`{"instance_id":"provider-project-1","display_name":"SDK project","state":"active"}`), nil
		default:
			return apiToolResponse(`{}`), nil
		}
	})
	_, bindings := fixture.server.apiDefaultToolDefinitions(context.Background(), "prod_acme", fixture.manifest, fixture.principal, false)
	binding := bindings["voice-api.admin.instances.create"]
	arguments := map[string]any{"environment_id": "env_prod", "display_name": "SDK project"}
	params := toolCallParams{Name: binding.Name, Arguments: arguments}
	params.Meta.IdempotencyKey = "stable-instance-key-0001"
	first := httptest.NewRecorder()
	fixture.server.executeAPIDefaultTool(context.Background(), first, rpcRequest{ID: 1}, params, "prod_acme", binding, false, fixture.manifest, model.ProductSelectionContext{}, fixture.principal)
	challenge := confirmationFromResponse(t, first)
	if providerCalls != 0 {
		t.Fatalf("provider called before confirmation: %d", providerCalls)
	}

	mismatched := params
	mismatched.Arguments = map[string]any{"environment_id": "env_prod", "display_name": "Different project"}
	mismatched.Meta.ConfirmationChallenge, mismatched.Meta.Confirmed = challenge, true
	mismatchResponse := httptest.NewRecorder()
	fixture.server.executeAPIDefaultTool(context.Background(), mismatchResponse, rpcRequest{ID: 2}, mismatched, "prod_acme", binding, false, fixture.manifest, model.ProductSelectionContext{}, fixture.principal)
	if _, ok := rpcResponseMap(t, mismatchResponse)["error"]; !ok || providerCalls != 0 {
		t.Fatalf("mismatched confirmation executed: response=%s calls=%d", mismatchResponse.Body.String(), providerCalls)
	}

	confirmed := params
	confirmed.Meta.ConfirmationChallenge, confirmed.Meta.Confirmed = challenge, true
	success := httptest.NewRecorder()
	fixture.server.executeAPIDefaultTool(context.Background(), success, rpcRequest{ID: 3}, confirmed, "prod_acme", binding, false, fixture.manifest, model.ProductSelectionContext{}, fixture.principal)
	if _, ok := rpcResponseMap(t, success)["result"]; !ok || providerCalls != 2 {
		t.Fatalf("exact confirmed retry failed: response=%s calls=%d", success.Body.String(), providerCalls)
	}

	replay := httptest.NewRecorder()
	fixture.server.executeAPIDefaultTool(context.Background(), replay, rpcRequest{ID: 4}, confirmed, "prod_acme", binding, false, fixture.manifest, model.ProductSelectionContext{}, fixture.principal)
	if _, ok := rpcResponseMap(t, replay)["error"]; !ok || providerCalls != 2 {
		t.Fatalf("confirmation replay executed: response=%s calls=%d", replay.Body.String(), providerCalls)
	}
}

func TestAPIDefaultKnowledgeCallIsBoundWithoutIntegrationArgument(t *testing.T) {
	fixture := newAPIToolFixture(t, nil)
	_, bindings := fixture.server.apiDefaultToolDefinitions(context.Background(), "prod_acme", fixture.manifest, fixture.principal, false)
	binding := bindings["voice-api.knowledge.search"]
	params := toolCallParams{Name: binding.Name, Arguments: map[string]any{"query": "API key"}}
	recorder := httptest.NewRecorder()
	fixture.server.executeAPIDefaultTool(context.Background(), recorder, rpcRequest{ID: 1}, params, "prod_acme", binding, false, fixture.manifest, model.ProductSelectionContext{}, fixture.principal)
	body := recorder.Body.String()
	if !strings.Contains(body, "doc_api_keys") || strings.Contains(body, "doc_internal") {
		t.Fatalf("knowledge call escaped its Integration publication binding: %s", body)
	}
}

func TestCanonicalCustomToolNamesAndLegacyAliasResolution(t *testing.T) {
	manifest := model.ProductManifest{Integrations: []model.IntegrationManifest{{ID: "integration_voice", FamilyKey: "voice-api", VersionKey: "v1"}}}
	apiTool := model.Tool{Scope: model.ToolScopeAPI, OwnerIntegrationID: "integration_voice", Name: "transcribe"}
	if name, ok := canonicalCustomToolName(manifest, apiTool); !ok || name != "voice-api.custom.transcribe" {
		t.Fatalf("API canonical name = %q, %v", name, ok)
	}
	commonTool := model.Tool{Scope: model.ToolScopeCommon, Name: "health"}
	if name, ok := canonicalCustomToolName(manifest, commonTool); !ok || name != "common.health" {
		t.Fatalf("common canonical name = %q, %v", name, ok)
	}

	ctx := context.Background()
	memory := store.NewMemory()
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_alias", OrganisationID: "org_acme", ProductID: "prod_acme", Scope: model.ToolScopeCommon, Namespace: "legacy", Name: "health", Description: "Health check", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object"}`), BaseURL: "https://api.vendor.example/health", HTTPMethod: "GET", AuthorizationPolicy: json.RawMessage(`{"required_grants":[]}`), BackendKind: "http"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := memory.PublishTool(ctx, draft.ProductID, draft.ID, draft.Revision, "root")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{service: platform.New(memory)}
	for _, name := range []string{"common.health", "legacy.health"} {
		resolved, err := server.executableTool(ctx, "prod_acme", name, model.ProductSelectionContext{})
		if err != nil || resolved.ID != published.ID {
			t.Fatalf("alias %q resolved %#v, err=%v", name, resolved, err)
		}
	}
}
