package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestSDKCandidateGraphRejectsCrossFileSectionAncestry(t *testing.T) {
	const (
		deploymentID         = "prod_acme"
		packageID            = "ancestry-sdk-package"
		releaseID            = "ancestry-sdk-release"
		runID                = "ancestry-sdk-run"
		candidateID          = "ancestry-sdk-candidate"
		publicationID        = "ancestry-sdk-publication"
		includedFileID       = "ancestry-included-file"
		quarantinedFileID    = "ancestry-quarantined-file"
		includedSectionID    = "ancestry-included-section"
		quarantinedSectionID = "ancestry-quarantined-section"
	)
	includedOrdinal := 0
	includedHash := developerAssetTestHash("a")
	quarantinedHash := developerAssetTestHash("b")
	symbolHash := developerAssetTestHash("c")
	sampleHash := developerAssetTestHash("d")
	candidateHash := developerAssetTestHash("e")
	packageValue := model.SDKPackage{
		ID: packageID, DeploymentID: deploymentID, OrganisationID: "org_acme", Ecosystem: "npm",
		CanonicalCoordinate: "@acme/ancestry", DisplayCoordinate: "@acme/ancestry", Name: "Ancestry SDK",
		Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}
	release := model.SDKRelease{
		ID: releaseID, DeploymentID: deploymentID, SDKPackageID: packageID, ExactVersion: "1.0.0",
		InstallCommand: "npm install @acme/ancestry@1.0.0", Visibility: model.VisibilityPrivate,
		Lifecycle: "active", ReleaseHash: developerAssetTestHash("f"),
	}
	base := SDKContentCandidateRecord{
		Candidate: model.SDKContentCandidate{
			ID: candidateID, DeploymentID: deploymentID, SDKReleaseID: releaseID, IngestionRunID: runID,
			ContentHash: candidateHash, Visibility: model.VisibilityPrivate,
		},
		Files: []model.SDKPublicationFile{
			{ID: includedFileID, SDKContentCandidateID: candidateID, SourcePath: "src/client.ts", ContentHash: includedHash},
			{ID: quarantinedFileID, SDKContentCandidateID: candidateID, SourcePath: "fixtures/unsafe.ts", ContentHash: quarantinedHash},
		},
		Sections: []model.SDKSection{
			{ID: includedSectionID, SDKContentCandidateID: candidateID, SDKPublicationFileID: includedFileID, ContentHash: developerAssetTestHash("1")},
			{ID: quarantinedSectionID, SDKContentCandidateID: candidateID, SDKPublicationFileID: quarantinedFileID, ContentHash: developerAssetTestHash("2")},
		},
	}
	basePublication := SDKContentPublicationRecord{
		Publication: model.SDKContentPublication{
			ID: publicationID, DeploymentID: deploymentID, SDKReleaseID: releaseID,
			SDKContentCandidateID: candidateID, ContentHash: candidateHash, Visibility: model.VisibilityPrivate,
		},
		FileSelections: []model.SDKContentPublicationFileSelection{
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: includedFileID, Decision: "included", Ordinal: &includedOrdinal, ContentHash: includedHash,
			},
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: quarantinedFileID, Decision: "quarantined", Reason: "untrusted fixture", ContentHash: quarantinedHash,
			},
		},
	}

	tests := []struct {
		name      string
		candidate SDKContentCandidateRecord
		publish   SDKContentPublicationRecord
		entityID  string
	}{
		{
			name: "symbol cannot borrow an included section ordinal for a quarantined file",
			candidate: func() SDKContentCandidateRecord {
				value := memoryClone(base)
				value.Symbols = []model.SDKSymbol{{
					ID: "ancestry-symbol", SDKContentCandidateID: candidateID,
					SDKPublicationFileID: quarantinedFileID, SDKSectionID: includedSectionID,
					QualifiedName: "unsafe.Marker", DisplayName: "FORBIDDEN_ANCESTRY_SYMBOL", ContentHash: symbolHash,
				}}
				return value
			}(),
			publish:  basePublication,
			entityID: "ancestry-symbol",
		},
		{
			name: "approved sample cannot claim an included file while citing a quarantined section",
			candidate: func() SDKContentCandidateRecord {
				value := memoryClone(base)
				value.Samples = []model.SDKCodeSample{{
					ID: "ancestry-sample", DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
					SDKPublicationFileID: includedFileID, SDKSectionID: quarantinedSectionID,
					Language: "typescript", Title: "FORBIDDEN_ANCESTRY_SAMPLE", Intent: "Must stay quarantined",
					Code: "forbidden()", Origin: model.SDKSampleCurated, ValidationStatus: model.SDKSampleSyntaxChecked,
					ValidationEvidence: json.RawMessage(`{"passed":true,"validator":"test/parser"}`),
					Visibility:         model.VisibilityPrivate, ContentHash: sampleHash,
				}}
				return value
			}(),
			publish: func() SDKContentPublicationRecord {
				value := memoryClone(basePublication)
				value.SampleSelections = []model.SDKContentPublicationSampleSelection{{
					SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
					SDKCodeSampleID: "ancestry-sample", Decision: "approved", Ordinal: &includedOrdinal, ContentHash: sampleHash,
				}}
				return value
			}(),
			entityID: "ancestry-sample",
		},
	}

	memory := NewMemory()
	ctx := context.Background()
	if _, err := memory.SaveSDKPackage(ctx, packageValue, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSDKRelease(ctx, release); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deploymentID, OrganisationID: "org_acme", AssetKind: model.DeveloperAssetSDK,
		TargetID: releaseID, TargetKey: releaseID,
		Versions: model.ProcessorVersions{Pipeline: "test-v1", Parser: "test-v1", Normalizer: "test-v1", Mapper: "test-v1"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateSDKContentCandidateGraph(test.candidate); err == nil || !strings.Contains(err.Error(), "outside its section ancestry") {
				t.Fatalf("candidate graph validation error = %v", err)
			}
			if _, err := memory.CreateSDKContentCandidate(ctx, test.candidate); !errors.Is(err, ErrConflict) {
				t.Fatalf("memory candidate persistence error = %v, want ErrConflict", err)
			}
			if reviewedMap, err := BuildReviewedSDKPublicationMap(packageValue, release, test.candidate, test.publish); err == nil || reviewedMap != nil {
				t.Fatalf("canonical map accepted mismatched entity %s: map=%#v err=%v", test.entityID, reviewedMap, err)
			}
		})
	}
}
