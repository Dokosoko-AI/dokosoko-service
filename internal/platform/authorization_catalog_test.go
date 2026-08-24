package platform_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

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
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_customers_read", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "customers", Name: "read", Description: "Read one customer.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"customer_id":{"type":"string"}},"required":["customer_id"]}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"customer_id":{"type":"string"}},"required":["customer_id"]}`), BaseURL: "https://api.vendor.example/v1/customers", HTTPMethod: "GET", AuthorizationPolicy: json.RawMessage(`{"required_grants":["customers.read"],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http", UpstreamAnnotations: json.RawMessage(`{}`)})
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
	invalidSelections := []struct {
		name       string
		selections []platform.ToolRevisionSelection
		message    string
	}{
		{name: "blank tool id", selections: []platform.ToolRevisionSelection{{ToolID: "  ", Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, message: "tool_id is required"},
		{name: "missing authorization point", selections: []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision}}, message: "authorization_point_id"},
		{name: "duplicate tool id", selections: []platform.ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}, {ToolID: publishedTool.ID, Revision: publishedTool.Revision + 1, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, message: "selected more than once"},
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
