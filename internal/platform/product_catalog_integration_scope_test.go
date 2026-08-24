package platform

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestSelectedIntegrationsUsesEffectiveCustomerProfile(t *testing.T) {
	version := model.ProductVersion{ProfileID: "profile_customer_a", Manifest: model.ProductDefinition{Profiles: []model.ProductProfile{{ID: "profile_customer_a", Selections: []model.ProductProfileSelection{{ComponentID: "component_records", ReleaseID: "release_v1"}}}}, Components: []model.ProductComponent{{ID: "component_records", Slug: "records-api", Releases: []model.ProductRelease{{ID: "release_v1", Version: "v1"}, {ID: "release_v2", Version: "v2"}}}, {ID: "component_billing", Slug: "billing-api", Releases: []model.ProductRelease{{ID: "release_billing", Version: "v1"}}}}}}
	integrations := []model.IntegrationManifest{{ID: "integration_records_v1", FamilyKey: "records-api", VersionKey: "v1"}, {ID: "integration_records_v2", FamilyKey: "records-api", VersionKey: "v2"}, {ID: "integration_billing", FamilyKey: "billing-api", VersionKey: "v1"}}

	selected := selectedIntegrations(version, integrations)
	if len(selected) != 1 || selected[0].ID != "integration_records_v1" {
		t.Fatalf("customer profile leaked unrelated Integrations: %#v", selected)
	}
}

func TestProductManifestRetainsManagedToolFactWhenProfileFiltersEveryIntegration(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	integration, err := service.CreateIntegration(ctx, IntegrationInput{FamilyKey: "records-api", VersionKey: "v1", DisplayName: "Records API", Description: "Managed records contract.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, Actor{ID: "root_scope"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(map[string]any{"family_key": integration.FamilyKey, "version_key": integration.VersionKey, "display_name": integration.DisplayName, "description": integration.Description, "visibility": integration.Visibility, "lifecycle": integration.Lifecycle, "resource_sets": []any{}, "authorization_points": []any{}, "tools": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{ID: "revision_managed_scope", IntegrationID: integration.ID, Revision: 1, State: "published", Snapshot: snapshot, ManifestHash: "sha256:managed", PublishedAt: &now}); err != nil {
		t.Fatal(err)
	}
	definition := model.ProductDefinition{Profiles: []model.ProductProfile{{ID: "profile_other", State: "published", Selections: []model.ProductProfileSelection{{ComponentID: "component_other", ReleaseID: "release_other"}}}}, Components: []model.ProductComponent{{ID: "component_other", Slug: "other-api", Releases: []model.ProductRelease{{ID: "release_other", Version: "v1"}}}}}
	if _, err := memory.CreateProductVersion(ctx, model.ProductVersion{ID: "version_other", OrganisationID: "org_acme", ProductID: "prod_acme", Version: "other", ProfileID: "profile_other", ProfileName: "Other", ReleaseStage: "active", PromotionState: "not_required", DriftStatus: "healthy", RolloutPercentage: 100, IsLatest: true, Manifest: definition}); err != nil {
		t.Fatal(err)
	}
	manifest, err := service.ProductManifestFor(ctx, "prod_acme", model.ProductSelectionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.ManagedIntegrationTools || len(manifest.Integrations) != 0 {
		t.Fatalf("managed fact or filtered scope was lost: %#v", manifest)
	}
}
