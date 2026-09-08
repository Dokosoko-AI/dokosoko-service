package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

// Test the actual wire names. Decoding these into Go structs would silently
// accept uppercase keys and miss the browser's undefined-property failure.
type assetDetailWireStore struct{ *store.Memory }

func (s *assetDetailWireStore) DocumentationCollectionRevision(_ context.Context, deploymentID, revisionID string) (store.DocumentationCollectionRevisionRecord, error) {
	if deploymentID != "prod_acme" || revisionID != "reviewed-revision" {
		return store.DocumentationCollectionRevisionRecord{}, store.ErrNotFound
	}
	return store.DocumentationCollectionRevisionRecord{
		Revision: model.DocumentationCollectionRevision{ID: revisionID, DeploymentID: deploymentID, DocumentationCollectionID: "reviewed-set", Revision: 1},
		Members:  []model.DocumentationCollectionMember{{ID: "member", DocumentationCollectionRevisionID: revisionID, Kind: "section", DocumentationSectionID: "exact-section"}},
		Map:      &model.DocumentationMap{ID: "exact-map", AgentMarkdown: "# Reviewed guide"},
	}, nil
}
func (s *assetDetailWireStore) APIContractCandidate(_ context.Context, deploymentID, candidateID string) (store.APIContractCandidateRecord, error) {
	if deploymentID != "prod_acme" || candidateID != "exact-candidate" {
		return store.APIContractCandidateRecord{}, store.ErrNotFound
	}
	return store.APIContractCandidateRecord{
		Candidate: model.APIContractCandidate{ID: candidateID, DeploymentID: deploymentID, APIContractID: "reviewed-contract"},
		Map:       &model.APIContractMap{ID: "contract-map"},
	}, nil
}

func (s *assetDetailWireStore) SDKContentCandidate(_ context.Context, deploymentID, candidateID string) (store.SDKContentCandidateRecord, error) {
	if deploymentID != "prod_acme" || candidateID != "sdk-candidate" {
		return store.SDKContentCandidateRecord{}, store.ErrNotFound
	}
	return store.SDKContentCandidateRecord{Candidate: model.SDKContentCandidate{ID: candidateID, DeploymentID: deploymentID, SDKReleaseID: "sdk-release"}}, nil
}
func (s *assetDetailWireStore) SDKContentPublication(_ context.Context, deploymentID, publicationID string) (store.SDKContentPublicationRecord, error) {
	if deploymentID != "prod_acme" || publicationID != "sdk-publication" {
		return store.SDKContentPublicationRecord{}, store.ErrNotFound
	}
	return store.SDKContentPublicationRecord{Publication: model.SDKContentPublication{ID: publicationID, DeploymentID: deploymentID, SDKReleaseID: "sdk-release"}}, nil
}

func TestDeveloperAssetReviewDetailsMatchBrowserContract(t *testing.T) {
	handler := httpapi.New(platform.New(&assetDetailWireStore{store.NewMemory()}), "https://dokosoko.example")
	for _, tc := range []struct {
		path                string
		keys                []string
		parentKey, parentID string
	}{
		{"/api/v1/developer-assets/documentation-collections/reviewed-set/revisions/reviewed-revision", []string{"revision", "members", "map"}, "revision", "reviewed-revision"},
		{"/api/v1/developer-assets/api-contracts/reviewed-contract/candidates/exact-candidate", []string{"candidate", "operations", "schemas", "examples", "map"}, "candidate", "exact-candidate"},
		{"/api/v1/developer-assets/sdk-releases/sdk-release/content-candidates/sdk-candidate", []string{"candidate", "files", "sections", "symbols", "samples", "sample_refs"}, "candidate", "sdk-candidate"},
		{"/api/v1/developer-assets/sdk-releases/sdk-release/content-publications/sdk-publication", []string{"publication", "file_selections", "sample_selections"}, "publication", "sdk-publication"},
	} {
		t.Run(tc.parentKey, func(t *testing.T) {
			res := request(t, handler, http.MethodGet, tc.path, "doko_admin_demo", "")
			var body map[string]json.RawMessage
			if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &body) != nil {
				t.Fatalf("detail=%d %s", res.Code, res.Body.String())
			}
			if len(body) != len(tc.keys) {
				t.Fatalf("unexpected wire keys: %s", res.Body.String())
			}
			for _, key := range tc.keys {
				if len(body[key]) == 0 {
					t.Fatalf("missing browser field %q: %s", key, res.Body.String())
				}
				if key != tc.parentKey && key != "map" && body[key][0] != '[' {
					t.Fatalf("browser list %q must be an array: %s", key, res.Body.String())
				}
			}
			var parent map[string]any
			if json.Unmarshal(body[tc.parentKey], &parent) != nil || parent["id"] != tc.parentID {
				t.Fatalf("wrong exact evidence: %s", res.Body.String())
			}
			denied := request(t, handler, http.MethodGet, tc.path, "", "")
			if denied.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous detail=%d", denied.Code)
			}
		})
	}
	for _, path := range []string{
		"/api/v1/developer-assets/documentation-collections/other-set/revisions/reviewed-revision",
		"/api/v1/developer-assets/api-contracts/other-contract/candidates/exact-candidate",
		"/api/v1/developer-assets/sdk-releases/other-release/content-candidates/sdk-candidate",
		"/api/v1/developer-assets/sdk-releases/other-release/content-publications/sdk-publication",
	} {
		res := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
		if res.Code != http.StatusNotFound {
			t.Fatalf("cross-root detail=%d %s", res.Code, res.Body.String())
		}
	}
}

// PostgreSQL can return nil for empty asset categories. Exercise those exact
// wire values so an SDK-only API cannot crash publication history.
type nullableAPIPublicationStore struct {
	*store.Memory
	apiID string
}

func (s *nullableAPIPublicationStore) APIDeveloperAssetPublications(_ context.Context, deploymentID, apiID string) ([]model.APIDeveloperAssetPublication, error) {
	if deploymentID != "prod_acme" || apiID != s.apiID {
		return nil, store.ErrNotFound
	}
	return []model.APIDeveloperAssetPublication{{ID: "exact-publication", DeploymentID: deploymentID, APIID: apiID}}, nil
}
func (s *nullableAPIPublicationStore) APIDeveloperAssetPublication(_ context.Context, deploymentID, publicationID string) (model.APIDeveloperAssetPublication, error) {
	if deploymentID != "prod_acme" || publicationID != "exact-publication" {
		return model.APIDeveloperAssetPublication{}, store.ErrNotFound
	}
	return model.APIDeveloperAssetPublication{ID: publicationID, DeploymentID: deploymentID, APIID: s.apiID}, nil
}
func TestAPIPublicationEmptyCategoriesAreArraysOnWire(t *testing.T) {
	backend := &nullableAPIPublicationStore{Memory: store.NewMemory()}
	service := platform.New(backend)
	integration, err := service.CreateIntegration(t.Context(), platform.IntegrationInput{FamilyKey: "publication-wire", VersionKey: "v1", DisplayName: "Wire", Visibility: model.VisibilityPrivate, Lifecycle: "draft"}, platform.Actor{ID: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	backend.apiID = integration.ID
	handler := httpapi.New(service, "https://dokosoko.example")
	path := "/api/v1/integrations/" + integration.ID + "/resources/publications"
	for _, suffix := range []string{"", "/exact-publication"} {
		response := request(t, handler, http.MethodGet, path+suffix, "doko_admin_demo", "")
		var value map[string]json.RawMessage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &value) != nil {
			t.Fatalf("detail=%d %s", response.Code, response.Body.String())
		}
		if suffix == "" {
			var rows []map[string]json.RawMessage
			if json.Unmarshal(value["items"], &rows) != nil || len(rows) != 1 {
				t.Fatalf("items=%s", response.Body.String())
			}
			value = rows[0]
		}
		for _, key := range []string{"documentation", "contracts", "sdks"} {
			if string(value[key]) != "[]" {
				t.Fatalf("%s must be an empty array: %s", key, response.Body.String())
			}
		}
	}
	denied := request(t, handler, http.MethodGet, "/api/v1/integrations/other-api/resources/publications/exact-publication", "doko_admin_demo", "")
	if denied.Code != http.StatusNotFound {
		t.Fatalf("cross-API detail=%d", denied.Code)
	}
}
