package platform

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAPIDeveloperAssetPublicationIsRevisionOwnedAndUsesImmutableVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	actor := Actor{ID: "revision-owned-assets"}
	api, err := service.CreateIntegration(ctx, IntegrationInput{
		FamilyKey: "asset-visibility", VersionKey: "v1", DisplayName: "Asset visibility",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	privateSnapshot := json.RawMessage(`{"visibility":"private"}`)
	privateRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "00000000-0000-4000-8000-000000000901", IntegrationID: api.ID,
		Revision: 1, State: "published", Snapshot: privateSnapshot,
		ManifestHash: contentHash(privateSnapshot), PublishedBy: actor.ID, PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateAssets := integrationDeveloperAssetSnapshot{
		SchemaVersion: developerAssetSnapshotSchemaVersion, APIVisibility: model.VisibilityPrivate,
		Documentation: []model.APIPublicationDocumentationAsset{},
		Contracts:     []model.APIPublicationContractAsset{}, SDKs: []model.APIPublicationSDKAsset{},
	}
	privatePublication, err := service.ensureAPIDeveloperAssetPublication(ctx, api, privateRevision, privateAssets, actor)
	if err != nil {
		t.Fatal(err)
	}

	api.Visibility = model.VisibilityPublic
	api, err = memory.UpdateIntegration(ctx, api, api.Revision)
	if err != nil {
		t.Fatal(err)
	}
	publicSnapshot := json.RawMessage(`{"visibility":"public"}`)
	publicRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "00000000-0000-4000-8000-000000000902", IntegrationID: api.ID,
		Revision: 2, State: "published", Snapshot: publicSnapshot,
		ManifestHash: contentHash(publicSnapshot), PublishedBy: actor.ID, PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicAssets := privateAssets
	publicAssets.APIVisibility = model.VisibilityPublic
	publicPublication, err := service.ensureAPIDeveloperAssetPublication(ctx, api, publicRevision, publicAssets, actor)
	if err != nil {
		t.Fatal(err)
	}
	if publicPublication.ID == privatePublication.ID ||
		publicPublication.APIRevisionID != publicRevision.ID ||
		privatePublication.APIRevisionID != privateRevision.ID {
		t.Fatalf("developer-asset publications were reused across API revisions: private=%#v public=%#v", privatePublication, publicPublication)
	}

	// Mutating the draft root again must not reinterpret either historical
	// publication during a retry or rebuild.
	api.Visibility = model.VisibilityPrivate
	if _, err := memory.UpdateIntegration(ctx, api, api.Revision); err != nil {
		t.Fatal(err)
	}
	privateVisibility, err := service.apiDeveloperAssetPublicationVisibility(ctx, privatePublication)
	if err != nil {
		t.Fatal(err)
	}
	publicVisibility, err := service.apiDeveloperAssetPublicationVisibility(ctx, publicPublication)
	if err != nil {
		t.Fatal(err)
	}
	if privateVisibility != model.VisibilityPrivate || publicVisibility != model.VisibilityPublic {
		t.Fatalf("historical visibility drifted with the mutable API root: private=%q public=%q", privateVisibility, publicVisibility)
	}
}

func TestFailedNewestDeveloperAssetProjectionDoesNotMoveDiscovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	actor := Actor{ID: "readiness-gate"}
	api, err := service.CreateIntegration(ctx, IntegrationInput{
		FamilyKey: "readiness", VersionKey: "v1", DisplayName: "Ready API",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	assets := integrationDeveloperAssetSnapshot{
		SchemaVersion: developerAssetSnapshotSchemaVersion, APIVisibility: model.VisibilityPrivate,
		Documentation: []model.APIPublicationDocumentationAsset{}, Contracts: []model.APIPublicationContractAsset{},
		SDKs: []model.APIPublicationSDKAsset{},
	}
	snapshotFor := func(displayName string) json.RawMessage {
		encoded, marshalErr := json.Marshal(integrationSnapshot{
			FamilyKey: api.FamilyKey, VersionKey: api.VersionKey, DisplayName: displayName,
			Visibility: api.Visibility, Lifecycle: api.Lifecycle, DeveloperAssets: assets,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return encoded
	}
	now := time.Now().UTC()
	readySnapshot := snapshotFor("Ready API")
	readyRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "00000000-0000-4000-8000-000000000911", IntegrationID: api.ID, Revision: 1,
		State: "published", Snapshot: readySnapshot, ManifestHash: contentHash(readySnapshot), PublishedBy: actor.ID, PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	readyPublication, err := service.ensureAPIDeveloperAssetPublication(ctx, api, readyRevision, assets, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ActivateDeveloperAssetPublication(ctx, "api", readyPublication.ID, actor); err != nil {
		t.Fatal(err)
	}

	failedSnapshot := snapshotFor("Broken newer API")
	failedRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "00000000-0000-4000-8000-000000000912", IntegrationID: api.ID, Revision: 2,
		State: "published", Snapshot: failedSnapshot, ManifestHash: contentHash(failedSnapshot), PublishedBy: actor.ID, PublishedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedPublication, err := memory.CreateAPIDeveloperAssetPublication(ctx, model.APIDeveloperAssetPublication{
		ID: "00000000-0000-4000-8000-000000000913", DeploymentID: api.DeploymentID, APIID: api.ID,
		APIRevisionID: failedRevision.ID, SnapshotSchemaVersion: developerAssetSnapshotSchemaVersion,
		SnapshotHash: contentHash([]byte("failed-publication")), Documentation: assets.Documentation,
		Contracts: assets.Contracts, SDKs: assets.SDKs, PublishedBy: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSearchIndexGeneration(ctx, model.SearchIndexGeneration{
		ID: "00000000-0000-4000-8000-000000000914", DeploymentID: api.DeploymentID,
		PublicationKind: "api", PublicationID: failedPublication.ID, AssetKind: "mixed",
		BuilderVersion: DeveloperAssetIndexBuilderVersion, RetrievalProfileVersion: DeveloperAssetRetrievalProfileVersion,
		State: "failed", Diagnostics: json.RawMessage(`{"error":"forced regression failure"}`),
	}); err != nil {
		t.Fatal(err)
	}

	current, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != readyPublication.ID {
		t.Fatalf("failed publication moved the ready head: got %s want %s", current.ID, readyPublication.ID)
	}
	manifest, err := service.ProductManifestFor(ctx, api.DeploymentID, model.CatalogScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Integrations) != 1 || manifest.Integrations[0].Revision != readyRevision.Revision || manifest.Integrations[0].DisplayName != "Ready API" {
		t.Fatalf("discovery moved to a failed projection: %#v", manifest.Integrations)
	}
}
