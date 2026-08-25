package platform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestIntegrationToolBindingsRejectWeakerAuthorizationActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		method  string
		action  string
		allowed bool
	}{
		{name: "GET accepts read", method: http.MethodGet, action: "read", allowed: true},
		{name: "GET accepts write", method: http.MethodGet, action: "write", allowed: true},
		{name: "GET accepts destructive", method: http.MethodGet, action: "destructive", allowed: true},
		{name: "POST rejects read", method: http.MethodPost, action: "read"},
		{name: "POST accepts write", method: http.MethodPost, action: "write", allowed: true},
		{name: "POST accepts destructive", method: http.MethodPost, action: "destructive", allowed: true},
		{name: "PUT rejects read", method: http.MethodPut, action: "read"},
		{name: "PUT accepts write", method: http.MethodPut, action: "write", allowed: true},
		{name: "PATCH rejects read", method: http.MethodPatch, action: "read"},
		{name: "PATCH accepts write", method: http.MethodPatch, action: "write", allowed: true},
		{name: "DELETE rejects read", method: http.MethodDelete, action: "read"},
		{name: "DELETE rejects write", method: http.MethodDelete, action: "write"},
		{name: "DELETE accepts destructive", method: http.MethodDelete, action: "destructive", allowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			memory := store.NewMemory()
			service := platform.New(memory)
			actor := platform.Actor{ID: "root_binding", RequestID: "req_binding"}

			integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "actions-api", VersionKey: "v1", DisplayName: "Actions API", Description: "Authorization action compatibility tests.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
			if err != nil {
				t.Fatal(err)
			}
			point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "actions.operation." + test.action, Name: test.action + " operation", Description: "Authorize the operation.", ActionType: test.action, ConfirmationRequired: test.action == "destructive", DecisionTTLSeconds: 120, State: "active"}, actor)
			if err != nil {
				t.Fatal(err)
			}

			policy := json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`)
			if test.method != http.MethodGet {
				policy = json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"medium","idempotency_required":true}`)
			}
			if test.method == http.MethodDelete {
				policy = json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"critical","idempotency_required":true}`)
			}
			draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_" + strings.ToLower(test.method) + "_" + test.action, OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "actions", Name: strings.ToLower(test.method) + "_" + test.action, Description: "Exercise authorization action compatibility.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), BaseURL: "https://api.vendor.example/v1/actions", HTTPMethod: test.method, UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: policy, TimeoutMS: 5000, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
			if err != nil {
				t.Fatal(err)
			}
			published, err := service.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, actor)
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: published.ID, Revision: published.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor)
			if test.allowed {
				if err != nil {
					t.Fatalf("strong-enough %s action rejected for %s: %v", test.action, test.method, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "weaker than the minimum") {
				t.Fatalf("weak %s action error for %s = %v", test.action, test.method, err)
			}
		})
	}
}

func TestPublishedIntegrationManifestCarriesExactAuthorizationAndToolContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_manifest", RequestID: "req_manifest"}

	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "customers-api", VersionKey: "v1", DisplayName: "Customers API", Description: "Customer contract.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	configurePrivateIntegrationFoundations(t, service, memory, integration, actor)
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: "customers.read", DisplayName: "Read customers", Description: "Read customer records.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "customers.record.read", Name: "Read customer", Description: "Read one customer.", ActionType: "read", RequiredGrants: []string{grant.Key}, DecisionTTLSeconds: 120, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_customers_read", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "customers", Name: "read", Description: "Read one customer.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"customer_id":{"type":"string"}},"required":["customer_id"]}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"customer_id":{"type":"string"}},"required":["customer_id"]}`), BaseURL: "https://api.vendor.example/v1/customers", HTTPMethod: "GET", UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":["customers.read"],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	publishedTool, err := service.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}
	bindingsBeforeInvalidSelections, err := service.IntegrationToolBindings(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	mcpDraft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_drifted_support", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "support", Name: "create_incident", Description: "Create an incident.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), HTTPMethod: "MCP", AuthorizationPolicy: json.RawMessage(`{"required_grants":[]}`), TimeoutMS: 5000, BackendKind: "mcp", UpstreamToolName: "incidents.create", UpstreamSchemaHash: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	mcpPublished, err := service.PublishTool(ctx, "prod_acme", mcpDraft.ID, mcpDraft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.MarkImportedToolDrift(ctx, "prod_acme", mcpPublished.ID, true); err != nil {
		t.Fatal(err)
	}
	invalidSelections := []struct {
		name       string
		selections []platform.ToolRevisionSelection
		message    string
	}{
		{name: "blank tool id", selections: []platform.ToolRevisionSelection{{ToolID: "  ", Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, message: "tool_id is required"},
		{name: "missing authorization point", selections: []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision}}, message: "authorization_point_id"},
		{name: "duplicate tool id", selections: []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}, {ToolID: publishedTool.ID, Revision: publishedTool.Revision + 1, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, message: "selected more than once"},
		{name: "drifted MCP tool", selections: []platform.ToolRevisionSelection{{ToolID: mcpPublished.ID, Revision: mcpPublished.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, message: "exact non-drifted published revision"},
	}
	for _, test := range invalidSelections {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.SetIntegrationToolBindings(ctx, integration.ID, test.selections, actor); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("invalid selections error = %v, want %q", err, test.message)
			}
			bindingsAfterInvalidSelection, err := service.IntegrationToolBindings(ctx, integration.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(bindingsBeforeInvalidSelections) != 1 || len(bindingsAfterInvalidSelection) != 1 || bindingsAfterInvalidSelection[0].ToolID != bindingsBeforeInvalidSelections[0].ToolID || bindingsAfterInvalidSelection[0].ToolRevision != bindingsBeforeInvalidSelections[0].ToolRevision || bindingsAfterInvalidSelection[0].CreatedBy != bindingsBeforeInvalidSelections[0].CreatedBy || !bindingsAfterInvalidSelection[0].CreatedAt.Equal(bindingsBeforeInvalidSelections[0].CreatedAt) {
				t.Fatalf("invalid selections changed bindings: before=%#v after=%#v", bindingsBeforeInvalidSelections, bindingsAfterInvalidSelection)
			}
		})
	}

	before, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := service.PublishIntegration(ctx, integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	after, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.CatalogRevision != before.CatalogRevision+1 {
		t.Fatalf("publication did not invalidate the deployment catalog: before=%d after=%d", before.CatalogRevision, after.CatalogRevision)
	}
	manifest, err := service.ProductManifest(ctx, "prod_acme", "")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CatalogRevision != after.CatalogRevision || len(manifest.Integrations) != 1 {
		t.Fatalf("unexpected deployment manifest after publication: %#v", manifest)
	}
	published := manifest.Integrations[0]
	if published.ManifestHash != revision.ManifestHash || len(published.AuthorizationPoints) != 1 || published.AuthorizationPoints[0].ID != point.ID || published.AuthorizationPoints[0].RequiredGrants[0] != grant.Key {
		t.Fatalf("authorization contract missing from manifest: %#v", published.AuthorizationPoints)
	}
	if len(published.Tools) != 1 || published.Tools[0].ToolID != publishedTool.ID || published.Tools[0].ToolRevision != publishedTool.Revision || published.Tools[0].AuthorizationPointID != point.ID || published.Tools[0].AuthorizationPointRevision != point.Revision || published.Tools[0].ContentHash == "" {
		t.Fatalf("exact tool contract missing from manifest: %#v", published.Tools)
	}
}
