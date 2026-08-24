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

func TestPackageCataloguePinsExactReleaseInPublishedIntegration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_packages", RequestID: "req_packages"}

	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "payments-api", VersionKey: "v1", DisplayName: "Payments API", Description: "Payments integration", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	configurePrivateIntegrationFoundations(t, service, memory, integration, actor)
	configurePrivateIntegrationPolicyTool(t, service, memory, integration, actor)
	artifact, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: "Payments Go SDK", Description: "Registry-delivered client SDK.", Ecosystem: "go", Coordinate: "example.com/acme/payments-go", PURL: "pkg:golang/example.com/acme/payments-go", RegistryURL: "https://proxy.golang.org/example.com/acme/payments-go", SourceURL: "https://github.com/acme/payments-go", Language: "Go", Platform: "server", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "v1.2.3", PURL: "pkg:golang/example.com/acme/payments-go@v1.2.3", InstallCommand: "go get example.com/acme/payments-go@v1.2.3", Digest: "sha256:abcd", ArtifactRevision: artifact.Revision}, actor); err == nil {
		t.Fatal("short digest was accepted")
	}
	artifact, first, err := service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "v1.2.3", PURL: "pkg:golang/example.com/acme/payments-go@v1.2.3", InstallCommand: "go get example.com/acme/payments-go@v1.2.3", Digest: "sha256:" + strings.Repeat("a", 64), ProvenanceURL: "https://github.com/acme/payments-go/attestations/v1.2.3", SBOMURL: "https://github.com/acme/payments-go/sbom/v1.2.3.json", ArtifactRevision: artifact.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.BindPackageRelease(ctx, integration.ID, first.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Release == nil || binding.Release.Version != "v1.2.3" {
		t.Fatalf("binding did not resolve exact release: %#v", binding)
	}
	published, err := service.PublishIntegration(ctx, integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Packages []struct {
			PackageReleaseID string `json:"package_release_id"`
			Version          string `json:"version"`
			Digest           string `json:"digest"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(published.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Packages) != 1 || snapshot.Packages[0].PackageReleaseID != first.ID || snapshot.Packages[0].Version != "v1.2.3" {
		t.Fatalf("published snapshot did not pin first release: %s", published.Snapshot)
	}

	artifact, second, err := service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "v1.3.0", PURL: "pkg:golang/example.com/acme/payments-go@v1.3.0", InstallCommand: "go get example.com/acme/payments-go@v1.3.0", Digest: "sha256:" + strings.Repeat("b", 64), ArtifactRevision: artifact.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.LatestRelease == nil || artifact.LatestRelease.ID != second.ID {
		t.Fatalf("latest release not advanced: %#v", artifact.LatestRelease)
	}
	status, err := service.IntegrationPublishStatus(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.HasChanges {
		t.Fatal("publishing a newer package release implicitly changed an exact Integration binding")
	}
	if _, err := service.BindPackageRelease(ctx, integration.ID, second.ID, actor); err != nil {
		t.Fatal(err)
	}
	status, err = service.IntegrationPublishStatus(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.HasChanges {
		t.Fatal("explicitly replacing the exact package binding did not change publish status")
	}
}

func TestPackageCataloguePublicBoundaryAndDeprecation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := platform.New(store.NewMemory())
	actor := platform.Actor{ID: "root_packages"}

	_, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: "Public JS SDK", Ecosystem: "npm", Coordinate: "@acme/public-sdk", PURL: "pkg:npm/%40acme/public-sdk", RegistryURL: "https://registry.npmjs.org/@acme/public-sdk", Visibility: model.VisibilityPublic}, actor)
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("public package without acknowledgement = %v", err)
	}
	artifact, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: "Public JS SDK", Ecosystem: "npm", Coordinate: "@acme/public-sdk", PURL: "pkg:npm/%40acme/public-sdk", RegistryURL: "https://registry.npmjs.org/@acme/public-sdk", Visibility: model.VisibilityPublic, AcknowledgePublic: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "1.0.0", PURL: "pkg:npm/%40acme/public-sdk@1.0.0", InstallCommand: "npm install @acme/public-sdk@1.0.0", Digest: "sha512:" + strings.Repeat("c", 128), ArtifactRevision: artifact.Revision}, actor)
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("publishing public package without acknowledgement = %v", err)
	}
	updated, _, err := service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "1.0.0", PURL: "pkg:npm/%40acme/public-sdk@1.0.0", InstallCommand: "npm install @acme/public-sdk@1.0.0", Digest: "sha512:" + strings.Repeat("c", 128), ArtifactRevision: artifact.Revision, AcknowledgePublic: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdatePackageArtifact(ctx, artifact.ID, platform.PackageArtifactInput{Name: "Changed after publish", Ecosystem: artifact.Ecosystem, Coordinate: artifact.Coordinate, PURL: artifact.PURL, RegistryURL: artifact.RegistryURL, Visibility: artifact.Visibility, AcknowledgePublic: true, Revision: updated.Revision}, actor); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("published package identity metadata was mutable: %v", err)
	}
	deprecated, err := service.DeprecatePackageArtifact(ctx, artifact.ID, platform.PackageDeprecationInput{Message: "Use the unified SDK.", Revision: updated.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if deprecated.Lifecycle != "deprecated" || deprecated.DeprecationMessage == "" {
		t.Fatalf("deprecation lifecycle not recorded: %#v", deprecated)
	}
}

func TestPackageCatalogueRejectsCredentialBearingMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := platform.New(store.NewMemory())
	actor := platform.Actor{ID: "root_packages"}

	invalidArtifacts := []platform.PackageArtifactInput{
		{Name: "Registry query", Ecosystem: "npm", Coordinate: "registry-query", PURL: "pkg:npm/registry-query", RegistryURL: "https://registry.npmjs.org/registry-query?token=secret", Visibility: model.VisibilityPrivate},
		{Name: "Registry userinfo", Ecosystem: "npm", Coordinate: "registry-userinfo", PURL: "pkg:npm/registry-userinfo", RegistryURL: "https://user:secret@registry.npmjs.org/registry-userinfo", Visibility: model.VisibilityPrivate},
		{Name: "Source fragment", Ecosystem: "npm", Coordinate: "source-fragment", PURL: "pkg:npm/source-fragment", RegistryURL: "https://registry.npmjs.org/source-fragment", SourceURL: "https://example.test/source#token", Visibility: model.VisibilityPrivate},
		{Name: "Versioned identity", Ecosystem: "npm", Coordinate: "versioned-identity", PURL: "pkg:npm/versioned-identity@1.0.0", RegistryURL: "https://registry.npmjs.org/versioned-identity", Visibility: model.VisibilityPrivate},
		{Name: "PURL query", Ecosystem: "npm", Coordinate: "purl-query", PURL: "pkg:npm/purl-query?repository_url=https://user:secret@example.test", RegistryURL: "https://registry.npmjs.org/purl-query", Visibility: model.VisibilityPrivate},
		{Name: "Mismatched PURL", Ecosystem: "npm", Coordinate: "mismatched-purl", PURL: "pkg:pypi/mismatched-purl", RegistryURL: "https://registry.npmjs.org/mismatched-purl", Visibility: model.VisibilityPrivate},
	}
	for _, input := range invalidArtifacts {
		input := input
		t.Run(input.Name, func(t *testing.T) {
			if _, err := service.CreatePackageArtifact(ctx, input, actor); err == nil {
				t.Fatalf("unsafe artifact metadata was accepted: %#v", input)
			}
		})
	}

	artifact, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: "Strict SDK", Ecosystem: "npm", Coordinate: "strict-sdk", PURL: "pkg:npm/strict-sdk", RegistryURL: "https://registry.npmjs.org/strict-sdk", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	base := platform.PackageReleaseInput{Version: "1.0.0", PURL: "pkg:npm/strict-sdk@1.0.0", InstallCommand: "npm install strict-sdk@1.0.0", Digest: "sha384:" + strings.Repeat("a", 96), ArtifactRevision: artifact.Revision}
	invalidReleases := []platform.PackageReleaseInput{
		func() platform.PackageReleaseInput { value := base; value.PURL += "?token=secret"; return value }(),
		func() platform.PackageReleaseInput {
			value := base
			value.PURL = "pkg:npm/other-sdk@1.0.0"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.PURL = "pkg:pypi/strict-sdk@1.0.0"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.InstallCommand = "NPM_TOKEN=secret npm install strict-sdk@1.0.0"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.InstallCommand = "npm config set //registry.npmjs.org/:_authToken secret"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.InstallCommand = "npm install https://user:secret@example.test/strict-sdk.tgz"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.ProvenanceURL = "https://example.test/provenance?token=secret"
			return value
		}(),
		func() platform.PackageReleaseInput {
			value := base
			value.SBOMURL = "https://example.test/sbom#secret"
			return value
		}(),
	}
	for index, input := range invalidReleases {
		if _, _, err := service.PublishPackageArtifact(ctx, artifact.ID, input, actor); err == nil {
			t.Fatalf("unsafe release metadata %d was accepted: %#v", index, input)
		}
	}
	if _, release, err := service.PublishPackageArtifact(ctx, artifact.ID, base, actor); err != nil {
		t.Fatalf("valid SHA-384 release was rejected: %v", err)
	} else if !strings.HasPrefix(release.Digest, "sha384:") {
		t.Fatalf("SHA-384 digest was not retained: %#v", release)
	}
}

func TestPackageLifecycleBlocksNewUseButPreservesPublishedBindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_packages", RequestID: "req_package_lifecycle"}
	integration := configureReadyPrivateIntegration(t, service, memory, actor)

	createPublished := func(name, coordinate string) (model.PackageArtifact, model.PackageRelease) {
		t.Helper()
		artifact, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: name, Ecosystem: "npm", Coordinate: coordinate, PURL: "pkg:npm/" + coordinate, RegistryURL: "https://registry.npmjs.org/" + coordinate, Visibility: model.VisibilityPrivate}, actor)
		if err != nil {
			t.Fatal(err)
		}
		artifact, release, err := service.PublishPackageArtifact(ctx, artifact.ID, platform.PackageReleaseInput{Version: "1.0.0", PURL: "pkg:npm/" + coordinate + "@1.0.0", InstallCommand: "npm install " + coordinate + "@1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), ArtifactRevision: artifact.Revision}, actor)
		if err != nil {
			t.Fatal(err)
		}
		return artifact, release
	}
	original, originalRelease := createPublished("Original SDK", "original-sdk")
	if _, err := service.BindPackageRelease(ctx, integration.ID, originalRelease.ID, actor); err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishIntegration(ctx, integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}

	replacementDraft, err := service.CreatePackageArtifact(ctx, platform.PackageArtifactInput{Name: "Replacement SDK", Ecosystem: "npm", Coordinate: "replacement-sdk", PURL: "pkg:npm/replacement-sdk", RegistryURL: "https://registry.npmjs.org/replacement-sdk", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeprecatePackageArtifact(ctx, original.ID, platform.PackageDeprecationInput{ReplacementPackageArtifactID: replacementDraft.ID, Message: "Move to the replacement.", Revision: original.Revision}, actor); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("draft replacement was accepted: %v", err)
	}
	replacement, _, err := service.PublishPackageArtifact(ctx, replacementDraft.ID, platform.PackageReleaseInput{Version: "1.0.0", PURL: "pkg:npm/replacement-sdk@1.0.0", InstallCommand: "npm install replacement-sdk@1.0.0", Digest: "sha256:" + strings.Repeat("b", 64), ArtifactRevision: replacementDraft.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	sunset := time.Now().Add(24 * time.Hour).UTC()
	deprecated, err := service.DeprecatePackageArtifact(ctx, original.ID, platform.PackageDeprecationInput{ReplacementPackageArtifactID: replacement.ID, Message: "Move to the replacement.", SunsetAt: &sunset, Revision: original.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PublishPackageArtifact(ctx, original.ID, platform.PackageReleaseInput{Version: "1.1.0", PURL: "pkg:npm/original-sdk@1.1.0", InstallCommand: "npm install original-sdk@1.1.0", Digest: "sha256:" + strings.Repeat("c", 64), ArtifactRevision: deprecated.Revision}, actor); err == nil {
		t.Fatal("deprecated artifact published a new release")
	}
	secondIntegration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "second-api", VersionKey: "v1", DisplayName: "Second API", Description: "Second", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindPackageRelease(ctx, secondIntegration.ID, originalRelease.ID, actor); err == nil {
		t.Fatal("deprecated artifact was newly bound")
	}
	bindings, err := memory.IntegrationPackageBindings(ctx, integration.ID)
	if err != nil || len(bindings) != 1 || bindings[0].Release == nil || bindings[0].Release.ID != originalRelease.ID {
		t.Fatalf("existing exact binding stopped resolving: %#v, %v", bindings, err)
	}

	status, err := service.IntegrationPublishStatus(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready {
		t.Fatal("candidate with deprecated package was publishable")
	}
	if !strings.Contains(string(status.CurrentSnapshot), `"lifecycle":"deprecated"`) || !strings.Contains(string(status.CurrentSnapshot), replacement.ID) || !strings.Contains(string(status.CurrentSnapshot), "Move to the replacement") {
		t.Fatalf("candidate snapshot omitted lifecycle guidance: %s", status.CurrentSnapshot)
	}
	preflight, err := service.IntegrationPreflight(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundPackageFailure := false
	for _, check := range preflight.Checks {
		if check.Code == "package_releases" && check.Required && check.Status == "fail" && strings.Contains(check.Message, replacement.ID) && strings.Contains(check.Message, "sunset_at=") {
			foundPackageFailure = true
		}
	}
	if preflight.Ready || !foundPackageFailure {
		t.Fatalf("preflight did not surface unavailable package: %#v", preflight)
	}
	if _, err := service.PublishIntegration(ctx, integration.ID, actor); err == nil {
		t.Fatal("candidate with deprecated package was published")
	}
	if !strings.Contains(string(published.Snapshot), `"lifecycle":"active"`) {
		t.Fatalf("existing published snapshot was mutated: %s", published.Snapshot)
	}
	if _, err := service.RetirePackageArtifact(ctx, original.ID, platform.PackageRetirementInput{ReplacementPackageArtifactID: replacement.ID, Message: "Retired; use replacement.", Revision: original.Revision}, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale retirement revision = %v", err)
	}
	retired, err := service.RetirePackageArtifact(ctx, original.ID, platform.PackageRetirementInput{ReplacementPackageArtifactID: replacement.ID, Message: "Retired; use replacement.", Revision: deprecated.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if retired.Lifecycle != "retired" || retired.ReplacementPackageArtifactID != replacement.ID || retired.Revision != deprecated.Revision+1 {
		t.Fatalf("retirement metadata was not recorded: %#v", retired)
	}
	bindings, err = memory.IntegrationPackageBindings(ctx, integration.ID)
	if err != nil || len(bindings) != 1 || bindings[0].Release == nil {
		t.Fatalf("retirement removed an existing immutable binding: %#v, %v", bindings, err)
	}
}
