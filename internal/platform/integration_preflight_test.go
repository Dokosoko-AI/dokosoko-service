package platform_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func configurePrivateIntegrationFoundations(t *testing.T, service *platform.Service, memory *store.Memory, integration model.Integration, actor platform.Actor) {
	t.Helper()
	ctx := t.Context()
	publication, err := memory.SourcePublication(ctx, integration.DeploymentID, "pub_docs_seed")
	if err != nil {
		t.Fatal(err)
	}
	documentationManifest, err := json.Marshal([]map[string]any{{"source_publication_id": publication.ID, "source_id": publication.SourceID, "revision": publication.Revision, "content_hash": publication.ContentHash, "name": "Reviewed documentation"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []struct {
		kind     string
		name     string
		manifest json.RawMessage
	}{
		{kind: "documentation", name: "Ready API documentation", manifest: documentationManifest},
		{kind: "api", name: "Ready API contract", manifest: json.RawMessage(`[{"name":"readiness","path":"/health/ready"}]`)},
	} {
		set, createErr := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: resource.kind, Name: resource.name, Description: resource.name, State: "active", Manifest: resource.manifest}, actor)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, attachErr := service.AttachResourceSet(ctx, integration.ID, set.ID, set.Latest.ID, actor); attachErr != nil {
			t.Fatal(attachErr)
		}
	}
	if _, err = memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "idp_ready", OrganisationID: integration.OrganisationID, DeploymentID: integration.DeploymentID, Issuer: "https://identity.example.test", ClientID: "client-ready", Scopes: []string{"openid", "ready.read"}, Audience: "https://api.example.test", OAuthResource: "https://api.example.test", OrganisationClaim: "tenant_id", DelegatedAPIOrigin: "https://api.example.test", State: "active"}); err != nil {
		t.Fatal(err)
	}
}

func configurePrivateIntegrationPolicyTool(t *testing.T, service *platform.Service, memory *store.Memory, integration model.Integration, actor platform.Actor) {
	t.Helper()
	ctx := t.Context()
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: "ready.read", DisplayName: "Read readiness", Description: "Read readiness status.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "ready.status.read", Name: "Read readiness", Description: "Read readiness status.", ActionType: "read", RequiredGrants: []string{grant.Key}, DecisionTTLSeconds: 300, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_ready_status", OrganisationID: integration.OrganisationID, ProductID: integration.DeploymentID, Namespace: "ready", Name: "status", Description: "Read readiness status.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), BaseURL: "https://api.example.test/health/ready", HTTPMethod: "GET", UpstreamAuth: json.RawMessage(`{"type":"delegated_oauth"}`), RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":["ready.read"],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	publishedTool, err := service.PublishTool(ctx, integration.DeploymentID, draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}
}

func configurePrivateIntegrationRuntimePolicyTool(t *testing.T, service *platform.Service, integration model.Integration, actor platform.Actor) model.RuntimeServiceConnection {
	t.Helper()
	ctx := t.Context()
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: "ready.runtime.read", DisplayName: "Read runtime readiness", Description: "Read runtime readiness status.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "ready.runtime.status.read", Name: "Read runtime readiness", Description: "Read runtime readiness status.", ActionType: "read", RequiredGrants: []string{grant.Key}, DecisionTTLSeconds: 300, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := service.ConfigureRuntimeSetup(ctx, integration.ID, platform.RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://api.example.test", AuthenticationType: "none"}, actor)
	if err != nil || len(setup.Connections) != 1 {
		t.Fatalf("runtime setup=%#v err=%v", setup, err)
	}
	connection := setup.Connections[0]
	draft, err := service.CreateTool(ctx, platform.ToolInput{ProductID: integration.DeploymentID, Scope: model.ToolScopeAPI, OwnerIntegrationID: integration.ID, RuntimeServiceConnectionID: connection.ID, HTTPPath: "/health/ready", Namespace: "ready", Name: "runtime_status", Description: "Read runtime readiness status.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}}}`), HTTPMethod: "GET", AuthorizationPolicy: json.RawMessage(`{"required_grants":["ready.runtime.read"],"confirmation_required":false,"risk":"low"}`), TimeoutMS: 5000}, actor)
	if err != nil {
		t.Fatal(err)
	}
	publishedTool, err := service.PublishTool(ctx, integration.DeploymentID, draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}
	return connection
}

func configureReadyPrivateIntegration(t *testing.T, service *platform.Service, memory *store.Memory, actor platform.Actor) model.Integration {
	t.Helper()
	integration, err := service.CreateIntegration(t.Context(), platform.IntegrationInput{FamilyKey: "ready-api", VersionKey: "v1", DisplayName: "Ready API", Description: "Immutable published description.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	configurePrivateIntegrationFoundations(t, service, memory, integration, actor)
	configurePrivateIntegrationPolicyTool(t, service, memory, integration, actor)
	return integration
}

func TestPrivateIntegrationPublicationRequiresServerPreflightAndExactCandidate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_preflight", RequestID: "req_preflight"}

	incomplete, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "incomplete", VersionKey: "v1", DisplayName: "Incomplete API", Description: "Missing required private bindings.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := service.IntegrationPreflight(ctx, incomplete.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Ready {
		t.Fatalf("incomplete preflight unexpectedly ready: %#v", failed.Checks)
	}
	if _, err = service.PublishIntegration(ctx, incomplete.ID, actor); err == nil || !strings.Contains(err.Error(), "Published documentation") {
		t.Fatalf("incomplete publication error = %v", err)
	}

	ready := configureReadyPrivateIntegration(t, service, memory, actor)
	preflight, err := service.IntegrationPreflight(ctx, ready.ID)
	if err != nil || !preflight.Ready || preflight.CandidateManifestHash == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	published, err := service.PublishIntegrationCandidate(ctx, ready.ID, preflight.CandidateRevision, preflight.CandidateManifestHash, actor)
	if err != nil || published.ManifestHash != preflight.CandidateManifestHash {
		t.Fatalf("publication=%#v err=%v", published, err)
	}
	current, err := memory.Integration(ctx, ready.DeploymentID, ready.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.UpdateIntegration(ctx, current.ID, platform.IntegrationInput{FamilyKey: current.FamilyKey, VersionKey: current.VersionKey, DisplayName: current.DisplayName, Description: "Changed after preflight.", Visibility: current.Visibility, Lifecycle: current.Lifecycle, Revision: current.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegrationCandidate(ctx, current.ID, preflight.CandidateRevision, preflight.CandidateManifestHash, actor); err == nil || !strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("stale candidate publication error = %v", err)
	}
}
