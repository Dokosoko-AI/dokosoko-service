package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
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
	definition, err := service.CreateAccessDefinition(ctx, platform.AccessDefinitionInput{ServiceKey: "ready-access", Name: "Ready access", InstanceCardinality: "one", InstanceLabelSingular: "account", InstanceLabelPlural: "accounts", CredentialScope: "connection", ManagementAuthType: "none", Operations: json.RawMessage(`{}`)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := service.CreateAccessConnection(ctx, platform.AccessConnectionInput{AccessDefinitionID: definition.ID, EnvironmentID: "env_prod", Name: "Ready production", BaseURL: "https://api.example.test", Config: json.RawMessage(`{}`), IntegrationIDs: []string{integration.ID}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetIntegrationAccessConnections(ctx, integration.ID, []string{connection.ID}, actor); err != nil {
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
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_ready_status", OrganisationID: integration.OrganisationID, ProductID: integration.DeploymentID, Namespace: "ready", Name: "status", Description: "Read readiness status.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`), BaseURL: "https://api.example.test/health/ready", HTTPMethod: "GET", AuthorizationPolicy: json.RawMessage(`{"required_grants":["ready.read"],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
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

func configureReadyPrivateIntegration(t *testing.T, service *platform.Service, memory *store.Memory, actor platform.Actor) model.Integration {
	t.Helper()
	ctx := t.Context()
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "ready-api", VersionKey: "v1", DisplayName: "Ready API", Description: "Immutable published description.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
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

func TestWidgetPinsPublishedIntegrationAndChatIgnoresMutableRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x31}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &productBuilderDoer{response: `{"choices":[{"message":{"content":"The pinned Ready API exposes ready.status."}}],"usage":{"total_tokens":20}}`}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root_widget_pin", RequestID: "req_widget_pin"}
	integration := configureReadyPrivateIntegration(t, service, memory, actor)
	preflight, err := service.IntegrationPreflight(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishIntegrationCandidate(ctx, integration.ID, preflight.CandidateRevision, preflight.CandidateManifestHash, actor)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := service.ProductManifest(ctx, integration.DeploymentID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Integrations) != 1 || len(manifest.Integrations[0].AccessConnections) != 1 || manifest.Integrations[0].AccessConnections[0].ConnectionRevision < 1 || manifest.Integrations[0].AccessConnections[0].AccessDefinitionRevision < 1 || manifest.Integrations[0].AccessConnections[0].ContentHash == "" {
		t.Fatalf("product manifest omitted the exact safe access binding: %#v", manifest.Integrations)
	}
	if _, err = service.SaveLLMProfile(ctx, platform.LLMProfileInput{OrganisationID: integration.OrganisationID, ProductID: integration.DeploymentID, Role: "assistant", Provider: "openai-compatible", Endpoint: "https://llm.example.com", Model: "widget-assistant-1", Credential: "provider-secret", MaxInputTokens: 4096, MaxOutputTokens: 512, DailyTokenBudget: 10000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	provisioning, err := service.CreateWidget(ctx, platform.WidgetInput{Name: "Pinned assistant", AllowedOrigins: []string{"https://app.customer.example"}, IntegrationIDs: []string{integration.ID}, Appearance: platform.WidgetAppearance{Theme: "auto", LauncherPosition: "right"}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.SetWidgetState(ctx, provisioning.Widget.ID, "active", provisioning.Widget.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(active.IntegrationBindings) != 1 || active.IntegrationBindings[0].IntegrationRevisionID != published.ID || active.IntegrationBindings[0].ManifestHash != published.ManifestHash || !bytes.Contains(active.IntegrationBindings[0].Snapshot, []byte(`"authorization_points"`)) || !bytes.Contains(active.IntegrationBindings[0].Snapshot, []byte(`"tools"`)) || !bytes.Contains(active.IntegrationBindings[0].Snapshot, []byte(`"resource_sets"`)) || !bytes.Contains(active.IntegrationBindings[0].Snapshot, []byte(`"packages"`)) || !bytes.Contains(active.IntegrationBindings[0].Snapshot, []byte(`"access_connections"`)) {
		t.Fatalf("widget did not pin the complete Integration publication: %#v", active.IntegrationBindings)
	}
	mutable, err := memory.Integration(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateIntegration(ctx, mutable.ID, platform.IntegrationInput{FamilyKey: mutable.FamilyKey, VersionKey: mutable.VersionKey, DisplayName: mutable.DisplayName, Description: "MUTABLE DESCRIPTION MUST NOT REACH CHAT", Visibility: mutable.Visibility, Lifecycle: mutable.Lifecycle, Revision: mutable.Revision}, actor); err != nil {
		t.Fatal(err)
	}
	updatedWidget, err := service.UpdateWidget(ctx, active.ID, platform.WidgetInput{Name: active.Name, AllowedOrigins: active.AllowedOrigins, IntegrationIDs: active.IntegrationIDs, Appearance: platform.WidgetAppearance{Theme: "auto", LauncherPosition: "right", Greeting: "Updated without following a later Integration"}, Revision: active.Revision}, actor)
	if err != nil {
		t.Fatalf("safe widget appearance update failed: %v", err)
	}
	if len(updatedWidget.IntegrationBindings) != 1 || updatedWidget.IntegrationBindings[0].IntegrationRevisionID != published.ID || updatedWidget.IntegrationBindings[0].ManifestHash != published.ManifestHash {
		t.Fatalf("widget update implicitly followed a later Integration candidate: %#v", updatedWidget.IntegrationBindings)
	}
	chatWidget := updatedWidget
	var normalizedSnapshot any
	if err = json.Unmarshal(chatWidget.IntegrationBindings[0].Snapshot, &normalizedSnapshot); err != nil {
		t.Fatal(err)
	}
	chatWidget.IntegrationBindings[0].Snapshot, err = json.Marshal(normalizedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL jsonb may normalize object-key order. The immutable revision ID
	// and stored manifest hash remain authoritative across that representation.
	reply, err := service.AnswerWidgetMessage(ctx, platform.WidgetPrincipal{Widget: chatWidget, Session: model.WidgetSession{ID: "session-pinned"}}, "Which tool is available?")
	if err != nil || reply != "The pinned Ready API exposes ready.status." {
		t.Fatalf("reply=%q err=%v", reply, err)
	}
	if !bytes.Contains(doer.requestBody, []byte("Immutable published description.")) || !bytes.Contains(doer.requestBody, []byte("ready.status")) || bytes.Contains(doer.requestBody, []byte("MUTABLE DESCRIPTION MUST NOT REACH CHAT")) {
		t.Fatalf("widget assistant did not consume only the pinned manifest: %s", doer.requestBody)
	}
	if _, err = service.SetWidgetState(ctx, updatedWidget.ID, "active", updatedWidget.Revision, actor); err == nil || !strings.Contains(err.Error(), "publish the exact preflight candidate") {
		t.Fatalf("reactivation followed an unpublished mutable row: %v", err)
	}
}

func TestWidgetExplicitToolExecutionRequestStatesCatalogOnlyBoundary(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &productBuilderDoer{response: `{"choices":[{"message":{"content":"No executable tool connection is available."}}],"usage":{"total_tokens":20}}`}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root_widget_catalog_only", RequestID: "req_widget_catalog_only"}
	integration := configureReadyPrivateIntegration(t, service, memory, actor)
	preflight, err := service.IntegrationPreflight(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegrationCandidate(ctx, integration.ID, preflight.CandidateRevision, preflight.CandidateManifestHash, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveLLMProfile(ctx, platform.LLMProfileInput{OrganisationID: integration.OrganisationID, ProductID: integration.DeploymentID, Role: "assistant", Provider: "openai-compatible", Endpoint: "https://llm.example.com", Model: "widget-assistant-1", Credential: "provider-secret", MaxInputTokens: 4096, MaxOutputTokens: 512, DailyTokenBudget: 10000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	provisioning, err := service.CreateWidget(ctx, platform.WidgetInput{Name: "Catalog-only assistant", AllowedOrigins: []string{"https://app.customer.example"}, IntegrationIDs: []string{integration.ID}, Appearance: platform.WidgetAppearance{Theme: "auto", LauncherPosition: "right"}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.SetWidgetState(ctx, provisioning.Widget.ID, "active", provisioning.Widget.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}

	reply, err := service.AnswerWidgetMessage(ctx, platform.WidgetPrincipal{Widget: active, Session: model.WidgetSession{ID: "session-catalog-only"}}, "Run ready.status now")
	if err != nil {
		t.Fatal(err)
	}
	expected := "This widget can explain the published `ready.status` tool contract, but it cannot execute tools. Use an authorized private MCP client to call `ready.status` so DokoSoko can enforce the configured identity, grants, and authorization policy."
	if reply != expected {
		t.Fatalf("catalog-only reply = %q", reply)
	}
	if len(doer.requestBody) != 0 {
		t.Fatalf("explicit execution request reached the assistant model: %s", doer.requestBody)
	}
}
