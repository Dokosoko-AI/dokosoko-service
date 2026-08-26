package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type sdkAncestryCandidateStore struct {
	*store.Memory
	release     model.SDKRelease
	publication store.SDKContentPublicationRecord
	candidate   store.SDKContentCandidateRecord
}

func (s *sdkAncestryCandidateStore) SDKRelease(_ context.Context, deploymentID, id string) (model.SDKRelease, error) {
	if s.release.DeploymentID != deploymentID || s.release.ID != id {
		return model.SDKRelease{}, store.ErrNotFound
	}
	return s.release, nil
}

func (s *sdkAncestryCandidateStore) SDKContentPublication(_ context.Context, deploymentID, id string) (store.SDKContentPublicationRecord, error) {
	if s.publication.Publication.DeploymentID != deploymentID || s.publication.Publication.ID != id {
		return store.SDKContentPublicationRecord{}, store.ErrNotFound
	}
	return s.publication, nil
}

func (s *sdkAncestryCandidateStore) SDKContentCandidate(_ context.Context, deploymentID, id string) (store.SDKContentCandidateRecord, error) {
	if s.candidate.Candidate.DeploymentID != deploymentID || s.candidate.Candidate.ID != id {
		return store.SDKContentCandidateRecord{}, store.ErrNotFound
	}
	return s.candidate, nil
}

func TestBuildAPISDKAssetIndexRejectsCrossFileSectionAncestry(t *testing.T) {
	const (
		deploymentID         = "prod_acme"
		packageID            = "index-ancestry-package"
		releaseID            = "index-ancestry-release"
		candidateID          = "index-ancestry-candidate"
		publicationID        = "index-ancestry-publication"
		includedFileID       = "index-ancestry-included-file"
		quarantinedFileID    = "index-ancestry-quarantined-file"
		includedSectionID    = "index-ancestry-included-section"
		quarantinedSectionID = "index-ancestry-quarantined-section"
	)
	hash := func(value string) string { return contentHash([]byte(value)) }
	ordinal := 0
	packageValue := model.SDKPackage{
		ID: packageID, DeploymentID: deploymentID, Ecosystem: "npm", CanonicalCoordinate: "@acme/index-ancestry",
		DisplayCoordinate: "@acme/index-ancestry", Name: "Index Ancestry SDK", Language: "typescript",
		Visibility: model.VisibilityPrivate,
	}
	release := model.SDKRelease{
		ID: releaseID, DeploymentID: deploymentID, SDKPackageID: packageID, ExactVersion: "1.0.0",
		InstallCommand: "npm install @acme/index-ancestry@1.0.0", Visibility: model.VisibilityPrivate,
		ReleaseHash: hash("release"),
	}
	candidate := store.SDKContentCandidateRecord{
		Candidate: model.SDKContentCandidate{
			ID: candidateID, DeploymentID: deploymentID, SDKReleaseID: releaseID,
			ContentHash: hash("candidate"), Visibility: model.VisibilityPrivate,
		},
		Files: []model.SDKPublicationFile{
			{
				ID: includedFileID, SDKContentCandidateID: candidateID, SourcePath: "src/client.ts", Role: "source",
				Language: "typescript", ContentHash: hash("included-file"), Metadata: json.RawMessage(`{}`),
			},
			{
				ID: quarantinedFileID, SDKContentCandidateID: candidateID, SourcePath: "fixtures/unsafe.ts", Role: "fixture",
				Language: "typescript", ContentHash: hash("quarantined-file"), Metadata: json.RawMessage(`{}`),
			},
		},
		Sections: []model.SDKSection{
			{
				ID: includedSectionID, SDKContentCandidateID: candidateID, SDKPublicationFileID: includedFileID,
				Heading: "Safe client", ContentKind: "prose", NormalizedText: "Use the reviewed client.",
				ContentHash: hash("included-section"), Metadata: json.RawMessage(`{}`),
			},
			{
				ID: quarantinedSectionID, SDKContentCandidateID: candidateID, SDKPublicationFileID: quarantinedFileID,
				Heading: "FORBIDDEN QUARANTINED SECTION", ContentKind: "prose", NormalizedText: "FORBIDDEN_ANCESTRY_SECTION",
				ContentHash: hash("quarantined-section"), Metadata: json.RawMessage(`{}`),
			},
		},
		Symbols: []model.SDKSymbol{{
			ID: "index-ancestry-symbol", SDKContentCandidateID: candidateID, SDKPublicationFileID: includedFileID,
			SDKSectionID: includedSectionID, Language: "typescript", Kind: "class", QualifiedName: "sdk.Client",
			DisplayName: "Client", Documentation: "Reviewed symbol", ContentHash: hash("symbol"), Metadata: json.RawMessage(`{}`),
		}},
		Samples: []model.SDKCodeSample{{
			ID: "index-ancestry-sample", DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
			SDKPublicationFileID: includedFileID, SDKSectionID: includedSectionID, Language: "typescript",
			Title: "Reviewed sample", Intent: "Create a client", Code: "new Client()", Origin: model.SDKSampleCurated,
			ValidationStatus: model.SDKSampleSyntaxChecked, ValidationEvidence: json.RawMessage(`{"passed":true,"validator":"test/parser"}`),
			Visibility: model.VisibilityPrivate, ContentHash: hash("sample"),
		}},
	}
	publication := store.SDKContentPublicationRecord{
		Publication: model.SDKContentPublication{
			ID: publicationID, DeploymentID: deploymentID, SDKReleaseID: releaseID, SDKContentCandidateID: candidateID,
			Revision: 1, ContentHash: candidate.Candidate.ContentHash, Visibility: model.VisibilityPrivate,
		},
		FileSelections: []model.SDKContentPublicationFileSelection{
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: includedFileID, Decision: "included", Ordinal: &ordinal, ContentHash: candidate.Files[0].ContentHash,
			},
			{
				SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
				SDKPublicationFileID: quarantinedFileID, Decision: "quarantined", Reason: "untrusted fixture", ContentHash: candidate.Files[1].ContentHash,
			},
		},
		SampleSelections: []model.SDKContentPublicationSampleSelection{{
			SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
			SDKCodeSampleID: candidate.Samples[0].ID, Decision: "approved", Ordinal: &ordinal, ContentHash: candidate.Samples[0].ContentHash,
		}},
	}
	publishedMap, err := store.BuildReviewedSDKPublicationMap(packageValue, release, candidate, publication)
	if err != nil {
		t.Fatal(err)
	}
	publication.PublishedMap = publishedMap
	publication.Map = &model.SDKContentPublicationMap{
		SDKContentPublicationID: publicationID, DeploymentID: deploymentID, SDKContentCandidateID: candidateID,
		SDKMapID: publishedMap.ID, ContentHash: publishedMap.ContentHash,
	}
	asset := model.APIPublicationSDKAsset{
		BindingID: "index-ancestry-binding", SDKPackageID: packageID, SDKReleaseID: releaseID,
		SDKPackageEcosystem: packageValue.Ecosystem, SDKPackageCoordinate: packageValue.CanonicalCoordinate,
		SDKPackageDisplayCoordinate: packageValue.DisplayCoordinate, SDKPackageDisplayName: packageValue.Name,
		SDKPackageLanguage: packageValue.Language, SDKPackagePlatform: packageValue.Platform,
		SDKContentPublicationID: publicationID, Selector: json.RawMessage(`{}`),
		ContentHash: candidate.Candidate.ContentHash, Visibility: model.VisibilityPrivate,
	}
	_, canonicalSelector, err := parseDeveloperAssetSelector(asset.Selector, developerAssetSDKSelector)
	if err != nil {
		t.Fatal(err)
	}
	asset.SelectorHash = contentHash(canonicalSelector)
	apiPublication := model.APIDeveloperAssetPublication{ID: "index-ancestry-api-publication", APIID: "index-ancestry-api"}

	validStore := &sdkAncestryCandidateStore{Memory: store.NewMemory(), release: release, publication: publication, candidate: candidate}
	validDrafts, err := New(validStore).buildAPISDKAssetIndex(context.Background(), deploymentID, apiPublication, model.VisibilityPrivate, asset, 0)
	if err != nil || len(validDrafts) != 4 {
		t.Fatalf("valid fixture drafts = %d, err=%v", len(validDrafts), err)
	}
	for _, draft := range validDrafts {
		if strings.Contains(draft.unit.Content, "FORBIDDEN_ANCESTRY_SECTION") {
			t.Fatalf("quarantined section entered valid index: %#v", draft.unit)
		}
	}

	corrupt := candidate
	corrupt.Symbols = append([]model.SDKSymbol(nil), candidate.Symbols...)
	corrupt.Samples = append([]model.SDKCodeSample(nil), candidate.Samples...)
	corrupt.Symbols[0].SDKPublicationFileID = quarantinedFileID
	corrupt.Samples[0].SDKSectionID = quarantinedSectionID
	corruptStore := &sdkAncestryCandidateStore{Memory: store.NewMemory(), release: release, publication: publication, candidate: corrupt}
	drafts, err := New(corruptStore).buildAPISDKAssetIndex(context.Background(), deploymentID, apiPublication, model.VisibilityPrivate, asset, 0)
	if err == nil || len(drafts) != 0 || !strings.Contains(err.Error(), "outside its section ancestry") {
		t.Fatalf("cross-file candidate index result: drafts=%#v err=%v", drafts, err)
	}
}
