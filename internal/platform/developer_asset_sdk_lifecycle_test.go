package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestSDKReleaseLifecycleEventsResolveAvailabilityAndPreserveHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "release-operator", RequestID: "request-release-lifecycle"}
	packageValue, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{
		Ecosystem: "npm", Coordinate: "@acme/lifecycle-sdk", Name: "Lifecycle SDK",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{
		ExactVersion: "1.2.3", Visibility: model.VisibilityPrivate,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "lifecycle", VersionKey: "v1", DisplayName: "Lifecycle API",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.SaveAPISDKBinding(ctx, api.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID,
		State: "draft", Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPrivate,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}

	initial, err := service.SDKReleaseLifecycle(ctx, release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.InitialLifecycle != "active" || initial.EffectiveLifecycle != "active" || !initial.Selectable || len(initial.Events) != 0 {
		t.Fatalf("initial lifecycle = %#v", initial)
	}

	now := time.Now().UTC()
	publishedAt := now.Add(-10 * time.Minute)
	revision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID: "revision-sdk-lifecycle-history", IntegrationID: api.ID, Revision: 1, State: "published",
		Snapshot: json.RawMessage(`{}`), ManifestHash: "sha256:" + strings.Repeat("a", 64),
		PublishedBy: actor.ID, PublishedAt: &publishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	historical, err := memory.CreateAPIDeveloperAssetPublication(ctx, model.APIDeveloperAssetPublication{
		ID: "publication-sdk-lifecycle-history", DeploymentID: api.DeploymentID, APIID: api.ID, APIRevisionID: revision.ID,
		SnapshotSchemaVersion: "developer-assets-v1", SnapshotHash: "sha256:" + strings.Repeat("b", 64),
		Documentation: []model.APIPublicationDocumentationAsset{}, Contracts: []model.APIPublicationContractAsset{},
		SDKs: []model.APIPublicationSDKAsset{{
			BindingID: binding.ID, SDKPackageID: packageValue.ID, SDKPackageEcosystem: packageValue.Ecosystem,
			SDKPackageCoordinate: packageValue.CanonicalCoordinate, SDKPackageDisplayCoordinate: packageValue.DisplayCoordinate,
			SDKPackageDisplayName: packageValue.Name, SDKPackageLanguage: packageValue.Language, SDKPackagePlatform: packageValue.Platform,
			SDKReleaseID: release.ID, Selector: binding.Selector, SelectorHash: binding.SelectorHash,
			ContentHash: release.ReleaseHash, Visibility: model.VisibilityPrivate,
		}}, PublishedBy: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	yankedAt := now.Add(-2 * time.Minute)
	state, err := service.AppendSDKReleaseLifecycleEvent(ctx, release.ID, platform.SDKReleaseLifecycleEventInput{
		Lifecycle: "yanked", Reason: "The registry withdrew this exact version.",
		ObservedSourceURI: "https://registry.example/releases/1.2.3", ObservedAt: yankedAt,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if state.EffectiveLifecycle != "yanked" || state.Selectable || state.EffectiveEvent == nil || state.EffectiveEvent.RecordedBy != actor.ID {
		t.Fatalf("yanked lifecycle = %#v", state)
	}
	state, err = service.AppendSDKReleaseLifecycleEvent(ctx, release.ID, platform.SDKReleaseLifecycleEventInput{
		Lifecycle: "deprecated", Reason: "Backfilled an older registry observation.", ObservedAt: now.Add(-3 * time.Minute),
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if state.EffectiveLifecycle != "yanked" || len(state.Events) != 2 || state.Events[0].Lifecycle != "yanked" {
		t.Fatalf("backfilled lifecycle ordering = %#v", state)
	}
	if _, err := service.AppendSDKReleaseLifecycleEvent(ctx, release.ID, platform.SDKReleaseLifecycleEventInput{
		Lifecycle: "active", Reason: "Impossible future observation.", ObservedAt: now.Add(time.Hour),
	}, actor); err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("future lifecycle event error = %v", err)
	}

	secondAPI, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: "lifecycle", VersionKey: "v2", DisplayName: "Lifecycle API v2",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAPISDKBinding(ctx, secondAPI.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID,
		State: "draft", Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPrivate,
	}, actor); !errors.Is(err, platform.ErrSDKReleaseUnavailable) {
		t.Fatalf("binding yanked release error = %v", err)
	}
	status, err := service.IntegrationPublishStatus(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundUnavailable := false
	for _, validation := range status.Validations {
		foundUnavailable = foundUnavailable || validation.Code == "sdk_release_unavailable"
	}
	if status.Ready || !foundUnavailable {
		t.Fatalf("API publication status = %#v", status)
	}
	storedHistorical, err := memory.APIDeveloperAssetPublication(ctx, api.DeploymentID, historical.ID)
	if err != nil || storedHistorical.ID != historical.ID || len(storedHistorical.SDKs) != 1 || storedHistorical.SDKs[0].SDKReleaseID != release.ID {
		t.Fatalf("historical publication = %#v, err = %v", storedHistorical, err)
	}

	audits, err := memory.AuditEvents(ctx, "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	lifecycleAudits := 0
	for _, audit := range audits {
		if audit.Action == "sdk_release.lifecycle_event.appended" && audit.TargetID == release.ID {
			lifecycleAudits++
			if audit.Current["effective_lifecycle"] == nil || audit.Current["event_lifecycle"] == nil {
				t.Fatalf("lifecycle audit omitted resolved state: %#v", audit)
			}
		}
	}
	if lifecycleAudits != 2 {
		t.Fatalf("lifecycle audit count = %d, want 2", lifecycleAudits)
	}
}

func TestInitiallyArchivedSDKReleaseCannotBeBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "release-operator"}
	packageValue, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{
		Ecosystem: "pypi", Coordinate: "archived-sdk", Name: "Archived SDK", Visibility: model.VisibilityPrivate,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{
		ExactVersion: "2.0.0", Visibility: model.VisibilityPrivate, Lifecycle: "archived",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "archived", VersionKey: "v1", DisplayName: "Archived API"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAPISDKBinding(ctx, api.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID, Selector: json.RawMessage(`{}`),
	}, actor); !errors.Is(err, platform.ErrSDKReleaseUnavailable) {
		t.Fatalf("binding archived release error = %v", err)
	}
}
