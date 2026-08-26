package platform_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func createLegacySDKTestAPI(t *testing.T, service *platform.Service, family string) model.Integration {
	t.Helper()
	value, err := service.CreateIntegration(t.Context(), platform.IntegrationInput{
		FamilyKey: family, VersionKey: "v1", DisplayName: family,
	}, platform.Actor{ID: "legacy-sdk-test"})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func legacySDKTestInput() platform.SDKReferenceInput {
	return platform.SDKReferenceInput{
		Ecosystem: "npm", Coordinate: "@acme/legacy-compatible", ExactVersion: "1.2.3",
		InstallCommand:   "npm install @acme/legacy-compatible@1.2.3",
		DocumentationURL: "https://docs.example.test/sdk/1.2.3",
		SourceURL:        "https://github.com/acme/legacy-compatible/tree/v1.2.3",
		Checksum:         "sha256:" + strings.Repeat("a", 64), Visibility: model.VisibilityPublic,
	}
}

func TestLegacySDKWritesMaterializeOneDeploymentOwnedExactRelease(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "legacy-sdk-test", RequestID: "legacy-sdk-compat"}
	apiA := createLegacySDKTestAPI(t, service, "legacy-sdk-a")
	apiB := createLegacySDKTestAPI(t, service, "legacy-sdk-b")

	input := legacySDKTestInput()
	referenceA, err := service.SaveSDKReference(ctx, apiA.ID, "", input, actor)
	if err != nil {
		t.Fatalf("create first legacy reference: %v", err)
	}
	referenceB, err := service.SaveSDKReference(ctx, apiB.ID, "", input, actor)
	if err != nil {
		t.Fatalf("create identical second legacy reference: %v", err)
	}
	if referenceA.ID == referenceB.ID {
		t.Fatal("API binding IDs were unexpectedly shared")
	}

	packages, err := memory.SDKPackages(ctx, "prod_acme")
	if err != nil || len(packages) != 1 {
		t.Fatalf("deployment SDK packages = %#v, err=%v", packages, err)
	}
	releases, err := memory.SDKReleases(ctx, "prod_acme", packages[0].ID)
	if err != nil || len(releases) != 1 {
		t.Fatalf("exact releases = %#v, err=%v", releases, err)
	}
	bindingA, err := memory.APISDKBinding(ctx, "prod_acme", apiA.ID, referenceA.ID)
	if err != nil || bindingA.ID != referenceA.ID || bindingA.SDKReleaseID != releases[0].ID || bindingA.State != "legacy_metadata" {
		t.Fatalf("typed binding A = %#v, err=%v", bindingA, err)
	}
	bindingB, err := memory.APISDKBinding(ctx, "prod_acme", apiB.ID, referenceB.ID)
	if err != nil || bindingB.ID != referenceB.ID || bindingB.SDKReleaseID != releases[0].ID {
		t.Fatalf("typed binding B = %#v, err=%v", bindingB, err)
	}

	conflicts := []struct {
		name   string
		change func(*platform.SDKReferenceInput)
	}{
		{name: "documentation URL", change: func(value *platform.SDKReferenceInput) {
			value.DocumentationURL = "https://docs.example.test/sdk/different"
		}},
		{name: "source URL", change: func(value *platform.SDKReferenceInput) {
			value.SourceURL = "https://github.com/acme/legacy-compatible/tree/different"
		}},
		{name: "checksum", change: func(value *platform.SDKReferenceInput) { value.Checksum = "sha256:" + strings.Repeat("b", 64) }},
		{name: "visibility", change: func(value *platform.SDKReferenceInput) { value.Visibility = model.VisibilityPrivate }},
	}
	for index, test := range conflicts {
		t.Run(test.name, func(t *testing.T) {
			api := createLegacySDKTestAPI(t, service, "legacy-sdk-conflict-"+string(rune('a'+index)))
			conflicting := input
			test.change(&conflicting)
			if _, err := service.SaveSDKReference(ctx, api.ID, "", conflicting, actor); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("conflicting exact release error = %v, want ErrConflict", err)
			}
			bindings, lookupErr := memory.APISDKBindings(ctx, "prod_acme", api.ID)
			if lookupErr != nil || len(bindings) != 0 {
				t.Fatalf("conflict created binding %#v, err=%v", bindings, lookupErr)
			}
		})
	}

	updatedInput := input
	updatedInput.ExactVersion = "2.0.0"
	updatedInput.InstallCommand = "npm install @acme/legacy-compatible@2.0.0"
	updatedInput.DocumentationURL = "https://docs.example.test/sdk/2.0.0"
	updatedInput.SourceURL = "https://github.com/acme/legacy-compatible/tree/v2.0.0"
	updatedInput.Revision = referenceA.Revision
	updated, err := service.SaveSDKReference(ctx, apiA.ID, referenceA.ID, updatedInput, actor)
	if err != nil {
		t.Fatalf("retarget legacy binding to another exact release: %v", err)
	}
	if updated.ID != referenceA.ID || updated.Revision != referenceA.Revision+1 {
		t.Fatalf("updated legacy projection = %#v", updated)
	}
	releases, err = memory.SDKReleases(ctx, "prod_acme", packages[0].ID)
	if err != nil || len(releases) != 2 {
		t.Fatalf("retargeting removed or failed to create immutable release: %#v, err=%v", releases, err)
	}

	if err := service.DeleteSDKReference(ctx, apiA.ID, referenceA.ID, actor); err != nil {
		t.Fatalf("delete legacy reference: %v", err)
	}
	detached, err := memory.APISDKBinding(ctx, "prod_acme", apiA.ID, referenceA.ID)
	if err != nil || detached.State != "detached" {
		t.Fatalf("legacy delete did not retain detached binding: %#v, err=%v", detached, err)
	}
	packagesAfter, _ := memory.SDKPackages(ctx, "prod_acme")
	releasesAfter, _ := memory.SDKReleases(ctx, "prod_acme", packages[0].ID)
	if len(packagesAfter) != 1 || len(releasesAfter) != 2 {
		t.Fatalf("legacy delete removed deployment-owned truth: packages=%d releases=%d", len(packagesAfter), len(releasesAfter))
	}
	legacyA, err := memory.SDKReferences(ctx, apiA.ID)
	if err != nil || len(legacyA) != 0 {
		t.Fatalf("detached legacy projection = %#v, err=%v", legacyA, err)
	}
}
