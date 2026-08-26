package platform_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestDeploymentOwnedSDKReleasesCanServeDifferentAPIs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "catalog-admin", RequestID: "request-sdk-catalog"}

	packageValue, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{
		Ecosystem: "npm", Coordinate: "@acme/platform-sdk", Name: "Acme JavaScript SDK",
		Description: "Exact SDK releases shared by multiple APIs.", Visibility: model.VisibilityPublic, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveSDKPackage(ctx, packageValue.ID, platform.SDKPackageInput{
		Ecosystem: "npm", Coordinate: "@acme/renamed-sdk", Name: packageValue.Name,
		Description: packageValue.Description, Visibility: packageValue.Visibility,
		Lifecycle: packageValue.Lifecycle, Revision: packageValue.Revision,
	}, actor); err == nil {
		t.Fatal("stable SDK package coordinate was mutated")
	}
	v1, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{ExactVersion: "1.4.0", Visibility: model.VisibilityPublic}, actor)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{ExactVersion: "2.1.0", Visibility: model.VisibilityPublic}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if v1.SDKPackageID != packageValue.ID || v2.SDKPackageID != packageValue.ID || v1.ID == v2.ID || v1.ReleaseHash == v2.ReleaseHash {
		t.Fatalf("releases = %#v, %#v", v1, v2)
	}
	if _, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{ExactVersion: "^2.1.0", Visibility: model.VisibilityPublic}, actor); err == nil {
		t.Fatal("floating SDK release version was accepted")
	}

	apiV1, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "payments", VersionKey: "v1", DisplayName: "Payments v1", Visibility: model.VisibilityPublic, AcknowledgePublic: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	apiV2, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "payments", VersionKey: "v2", DisplayName: "Payments v2", Visibility: model.VisibilityPublic, AcknowledgePublic: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	bindingV1, err := service.SaveAPISDKBinding(ctx, apiV1.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: v1.ID, Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPublic,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	bindingV2, err := service.SaveAPISDKBinding(ctx, apiV2.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: v2.ID, Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPublic,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if bindingV1.SDKPackageID != bindingV2.SDKPackageID || bindingV1.SDKReleaseID == bindingV2.SDKReleaseID {
		t.Fatalf("bindings = %#v, %#v", bindingV1, bindingV2)
	}
	legacy, err := memory.SDKReferences(ctx, apiV1.ID)
	if err != nil || len(legacy) != 1 || legacy[0].ID != bindingV1.ID || legacy[0].ExactVersion != "1.4.0" {
		t.Fatalf("legacy projection = %#v, err = %v", legacy, err)
	}
	catalog, err := service.DeveloperAssetCatalog(ctx)
	if err != nil || len(catalog.SDKPackages) != 1 || catalog.SDKPackages[0].ID != packageValue.ID {
		t.Fatalf("catalog = %#v, err = %v", catalog, err)
	}
}

func TestSDKBindingCannotWidenReleaseVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "catalog-admin"}

	packageValue, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{Ecosystem: "pypi", Coordinate: "acme-sdk", Name: "Acme Python SDK", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, packageValue.ID, platform.SDKReleaseInput{ExactVersion: "1.2.3", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	api, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "private-api", VersionKey: "v1", DisplayName: "Private API"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAPISDKBinding(ctx, api.ID, "", platform.APISDKBindingInput{
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID, Selector: json.RawMessage(`{}`), Visibility: model.VisibilityPublic,
	}, actor); err == nil {
		t.Fatal("public binding widened a private SDK package and release")
	}
}
