package platform

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestIntegrationSnapshotDoesNotDuplicateTypedSDKAsLegacyEvidence(t *testing.T) {
	t.Parallel()
	const bindingID = "00000000-0000-0000-0000-000000000010"
	integration := model.Integration{
		ID: "api-a", DeploymentID: "deployment-a", OrganisationID: "organisation-a",
		FamilyKey: "payments", VersionKey: "v1", DisplayName: "Payments", Visibility: model.VisibilityPrivate,
		SDKs: []model.SDKReference{{
			ID: bindingID, DeploymentID: "deployment-a", OrganisationID: "organisation-a", IntegrationID: "api-a",
			Ecosystem: "npm", Coordinate: "@acme/payments", ExactVersion: "1.2.3",
			InstallCommand: "npm install @acme/payments@1.2.3", Visibility: model.VisibilityPrivate, Revision: 1,
		}},
	}
	inputs := integrationPublicationInputSet{DeveloperAssets: integrationDeveloperAssetSnapshot{
		SchemaVersion: developerAssetSnapshotSchemaVersion,
		Documentation: []model.APIPublicationDocumentationAsset{}, Contracts: []model.APIPublicationContractAsset{},
		SDKs: []model.APIPublicationSDKAsset{{BindingID: bindingID, SDKPackageID: "package-a", SDKReleaseID: "release-a"}},
	}}

	snapshot, _, err := buildIntegrationSnapshot(integration, inputs)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		LegacySDKs      []integrationSDKSnapshot          `json:"sdks"`
		DeveloperAssets integrationDeveloperAssetSnapshot `json:"developer_assets"`
	}
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.LegacySDKs) != 0 || len(decoded.DeveloperAssets.SDKs) != 1 || decoded.DeveloperAssets.SDKs[0].BindingID != bindingID {
		t.Fatalf("SDK evidence was duplicated or lost: legacy=%#v typed=%#v", decoded.LegacySDKs, decoded.DeveloperAssets.SDKs)
	}
}
