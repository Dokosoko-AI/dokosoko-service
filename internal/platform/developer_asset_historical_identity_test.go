package platform

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type historicalIdentityIndexStore struct {
	store.Store
	contractRevision  model.APIContractRevision
	contractCandidate store.APIContractCandidateRecord
	collectionReads   int
	contractReads     int
}

func (s *historicalIdentityIndexStore) DocumentationCollection(context.Context, string, string) (model.DocumentationCollection, error) {
	s.collectionReads++
	return model.DocumentationCollection{Name: "Renamed Guide", Slug: "renamed-guide", Description: "New description"}, nil
}

func (s *historicalIdentityIndexStore) APIContract(context.Context, string, string) (model.APIContract, error) {
	s.contractReads++
	return model.APIContract{Name: "Renamed Contract", Slug: "renamed-contract", Description: "New description", Kind: "openapi"}, nil
}

func (s *historicalIdentityIndexStore) APIContractRevision(_ context.Context, deploymentID, id string) (model.APIContractRevision, error) {
	if s.contractRevision.DeploymentID != deploymentID || s.contractRevision.ID != id {
		return model.APIContractRevision{}, store.ErrNotFound
	}
	return s.contractRevision, nil
}

func (s *historicalIdentityIndexStore) APIContractCandidate(_ context.Context, deploymentID, id string) (store.APIContractCandidateRecord, error) {
	if s.contractCandidate.Candidate.DeploymentID != deploymentID || s.contractCandidate.Candidate.ID != id {
		return store.APIContractCandidateRecord{}, store.ErrNotFound
	}
	return s.contractCandidate, nil
}

func TestHistoricalIndexRenderingUsesRevisionAndPublicationRootSnapshotsAfterRename(t *testing.T) {
	ctx := context.Background()
	const deploymentID = "prod_acme"
	documentationRevision := model.DocumentationCollectionRevision{
		ID: "historical-documentation-revision", DeploymentID: deploymentID,
		DocumentationCollectionID:   "documentation-collection",
		DocumentationCollectionName: "Original Guide", DocumentationCollectionSlug: "original-guide",
		DocumentationCollectionDescription: "Original guide description.",
		Revision:                           1, Visibility: model.VisibilityPrivate, ContentHash: contentHash([]byte("documentation-revision")),
	}
	documentationRecord := store.DocumentationCollectionRevisionRecord{
		Revision: documentationRevision,
		Map: &model.DocumentationMap{
			ID: "historical-documentation-map", DeploymentID: deploymentID,
			DocumentationCollectionRevisionID: documentationRevision.ID, MapVersion: "documentation-map-v1",
			Map: model.DocumentationMapBody{Overview: "Original guide map."}, AgentMarkdown: "# Original Guide",
			ContentHash: contentHash([]byte("documentation-map")), Visibility: model.VisibilityPrivate,
		},
	}
	contractRevision := model.APIContractRevision{
		ID: "historical-contract-revision", DeploymentID: deploymentID, APIContractID: "api-contract",
		APIContractName: "Original Contract", APIContractSlug: "original-contract",
		APIContractDescription: "Original contract description.", APIContractKind: "openapi",
		APIContractCandidateID: "historical-contract-candidate", Revision: 1,
		ContentHash: contentHash([]byte("contract-revision")), Visibility: model.VisibilityPrivate,
	}
	contractMap := &model.APIContractMap{
		ID: "historical-contract-map", DeploymentID: deploymentID,
		APIContractCandidateID: contractRevision.APIContractCandidateID, MapVersion: "contract-map-v1",
		Map: model.ContractMapBody{Overview: "Original contract map."}, AgentMarkdown: "# Original Contract",
		ContentHash: contentHash([]byte("contract-map")),
	}
	backend := &historicalIdentityIndexStore{
		contractRevision: contractRevision,
		contractCandidate: store.APIContractCandidateRecord{
			Candidate: model.APIContractCandidate{
				ID: contractRevision.APIContractCandidateID, DeploymentID: deploymentID,
				APIContractID: contractRevision.APIContractID, ContentHash: contractRevision.ContentHash,
				Visibility: model.VisibilityPrivate, NormalizedContract: json.RawMessage(`{"openapi":"3.1.0"}`),
			},
			Map: contractMap,
		},
	}
	service := New(backend)

	documentationDrafts, err := service.buildDocumentationRevisionIndex(ctx, deploymentID, documentationRecord, developerAssetDocumentationBuildOptions{
		visibility:             model.VisibilityPrivate,
		outerSelector:          developerAssetSelector{values: map[string]map[string]bool{}, present: map[string]bool{}},
		wrapperPublicationKind: "global_documentation", wrapperPublicationID: "historical-global-publication",
	})
	if err != nil {
		t.Fatal(err)
	}
	contractAsset := model.APIPublicationContractAsset{
		APIContractID: contractRevision.APIContractID, APIContractName: contractRevision.APIContractName,
		APIContractSlug: contractRevision.APIContractSlug, APIContractDescription: contractRevision.APIContractDescription,
		APIContractKind: contractRevision.APIContractKind, APIContractRevisionID: contractRevision.ID,
		ContentHash: contractRevision.ContentHash, Visibility: model.VisibilityPrivate,
	}
	contractDrafts, err := service.buildAPIContractAssetIndex(ctx, deploymentID, model.APIDeveloperAssetPublication{
		ID: "historical-api-publication", APIID: "api-id",
	}, model.VisibilityPrivate, contractAsset, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(documentationDrafts) != 1 || len(contractDrafts) != 1 {
		t.Fatalf("unexpected historical draft counts: documentation=%d contract=%d", len(documentationDrafts), len(contractDrafts))
	}
	documentationUnit, contractUnit := documentationDrafts[0].unit, contractDrafts[0].unit
	if documentationUnit.Title != "Original Guide documentation map" || !slices.Contains(documentationUnit.Identifiers, "original-guide") ||
		strings.Contains(documentationUnit.Title, "Renamed") || strings.Contains(string(documentationUnit.Metadata), "New description") {
		t.Fatalf("documentation index used mutable root metadata: %#v", documentationUnit)
	}
	if contractUnit.Title != "Original Contract contract map" || !slices.Contains(contractUnit.Identifiers, "original-contract") ||
		strings.Contains(contractUnit.Title, "Renamed") || strings.Contains(string(contractUnit.Metadata), "New description") {
		t.Fatalf("contract index used mutable root metadata: %#v", contractUnit)
	}
	if !strings.Contains(string(documentationUnit.Metadata), "Original guide description") ||
		!strings.Contains(string(contractUnit.Metadata), "Original contract description") {
		t.Fatalf("historical descriptions were not snapshotted into index metadata: documentation=%s contract=%s", documentationUnit.Metadata, contractUnit.Metadata)
	}
	if backend.collectionReads != 0 || backend.contractReads != 0 {
		t.Fatalf("historical rendering dereferenced mutable roots: collection=%d contract=%d", backend.collectionReads, backend.contractReads)
	}
}
