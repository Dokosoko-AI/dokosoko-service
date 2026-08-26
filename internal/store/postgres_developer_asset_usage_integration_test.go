package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresDeveloperAssetUsageIncludesSDKBindingAndLegacyProjection(t *testing.T) {
	_, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployment, err := postgres.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		organisationID := storeTestUUID(t)
		suffix := organisationID[:8]
		if _, err = postgres.CreateOrganisation(ctx, model.Organisation{
			ID: organisationID, Name: "Developer asset usage", Slug: "developer-asset-usage-" + suffix,
		}); err != nil {
			t.Fatal(err)
		}
		deployment, err = postgres.CreateDeployment(ctx, model.Deployment{
			ID: storeTestUUID(t), OrganisationID: organisationID,
			Name: "Developer asset usage", Slug: "developer-asset-usage-" + suffix,
		})
	}
	if err != nil {
		t.Fatal(err)
	}

	fixtureID := storeTestUUID(t)
	api, err := postgres.CreateIntegration(ctx, model.Integration{
		ID: fixtureID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		FamilyKey: "usage-" + fixtureID, VersionKey: "v1", DisplayName: "Usage API",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	})
	if err != nil {
		t.Fatal(err)
	}

	packageID := storeTestUUID(t)
	coordinate := "@dokosoko/usage-" + packageID
	sdkPackage, err := postgres.SaveSDKPackage(ctx, model.SDKPackage{
		ID: packageID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		Ecosystem: "npm", CanonicalCoordinate: coordinate, DisplayCoordinate: coordinate,
		Name: "Usage SDK", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	releaseID := storeTestUUID(t)
	release, err := postgres.CreateSDKRelease(ctx, model.SDKRelease{
		ID: releaseID, DeploymentID: deployment.ID, SDKPackageID: sdkPackage.ID,
		ExactVersion: "1.0.0", InstallCommand: "npm install " + coordinate + "@1.0.0",
		IdentityAssurance: "metadata_only", Visibility: model.VisibilityPrivate,
		Lifecycle: "active", ReleaseHash: developerAssetTestHash("a"),
	})
	if err != nil {
		t.Fatal(err)
	}

	selector := json.RawMessage(`{}`)
	selectorHash, err := documentationSelectorHash(selector)
	if err != nil {
		t.Fatal(err)
	}
	bindingID := storeTestUUID(t)
	binding, err := postgres.SaveAPISDKBinding(ctx, model.APISDKBinding{
		ID: bindingID, DeploymentID: deployment.ID, APIID: api.ID,
		SDKPackageID: sdkPackage.ID, SDKReleaseID: release.ID, State: "draft",
		Coverage: model.SDKCoverageUnknown, Assurance: model.SDKAssuranceRelated,
		ApplicableModules: []string{}, ApplicableCapabilities: []string{}, ApplicableOperationKeys: []string{},
		Selector: selector, SelectorHash: selectorHash, Visibility: model.VisibilityPrivate,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	usage, err := postgres.DeveloperAssetUsage(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Documentation == nil || usage.Contracts == nil || usage.SDKs == nil || usage.Publications == nil {
		t.Fatalf("usage collections must be non-nil: %#v", usage)
	}
	found := false
	for _, item := range usage.SDKs {
		if item.ID == binding.ID {
			found = item.APIID == api.ID && item.SDKPackageID == sdkPackage.ID && item.SDKReleaseID == release.ID
			break
		}
	}
	if !found {
		t.Fatalf("usage does not contain exact SDK binding %#v: %#v", binding, usage.SDKs)
	}

	legacy, err := postgres.SDKReferences(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 1 || legacy[0].ID != binding.ID || legacy[0].Coordinate != sdkPackage.DisplayCoordinate || legacy[0].ExactVersion != release.ExactVersion {
		t.Fatalf("legacy SDK projection = %#v", legacy)
	}
}
