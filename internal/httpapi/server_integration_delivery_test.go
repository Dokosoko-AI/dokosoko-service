package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type deliveryFailureStore struct {
	*store.Memory
	phase string
}

func (s *deliveryFailureStore) CreateAPIDeveloperAssetPublication(ctx context.Context, value model.APIDeveloperAssetPublication) (model.APIDeveloperAssetPublication, error) {
	if s.phase == "asset" {
		return model.APIDeveloperAssetPublication{}, errors.New("fixture asset persistence outage")
	}
	return s.Memory.CreateAPIDeveloperAssetPublication(ctx, value)
}
func (s *deliveryFailureStore) CompleteSearchIndexGeneration(ctx context.Context, value store.SearchIndexGenerationRecord, expected string) (model.SearchIndexGeneration, error) {
	if s.phase == "index" && value.Generation.PublicationKind == "api" {
		return model.SearchIndexGeneration{}, errors.New("fixture index persistence outage")
	}
	return s.Memory.CompleteSearchIndexGeneration(ctx, value, expected)
}
func TestAPIDeliveryRetryUsesSavedRevisionAfterDraftChanges(t *testing.T) {
	for _, phase := range []string{"asset", "index"} {
		t.Run(phase, func(t *testing.T) {
			backend := &deliveryFailureStore{Memory: store.NewMemory(), phase: phase}
			service := platform.New(backend)
			ctx := t.Context()
			actor := platform.Actor{ID: "reviewer"}
			integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "delivery-" + phase, VersionKey: "v1", DisplayName: "Reviewed name", Visibility: model.VisibilityPrivate, Lifecycle: "draft"}, actor)
			if err != nil {
				t.Fatal(err)
			}
			attachMCPTestContract(t, service, integration.ID)
			handler := httpapi.New(service, "https://dokosoko.example")
			status, err := service.IntegrationPublishStatus(ctx, integration.ID)
			if err != nil {
				t.Fatal(err)
			}
			response := request(t, handler, http.MethodPost, "/api/v1/integrations/"+integration.ID+"/publish", "doko_admin_demo", fmt.Sprintf(`{"candidate_revision":%d,"candidate_manifest_hash":%q}`, status.CandidateRevision, status.CurrentManifestHash))
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("publish=%d %s", response.Code, response.Body.String())
			}
			saved, err := service.IntegrationPublishStatus(ctx, integration.ID)
			if err != nil || saved.LatestRevision == nil || saved.LatestDeliveryReady || saved.ServingRevisionID != "" || saved.HasChanges {
				t.Fatalf("saved=%#v error=%v", saved, err)
			}
			if _, err = service.ReadyAPIDeveloperAssetPublication(ctx, integration.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("failed projection became serving: %v", err)
			}
			current, err := backend.Integration(ctx, integration.DeploymentID, integration.ID)
			if err != nil {
				t.Fatal(err)
			}
			current.DisplayName = "Unreviewed draft change"
			if _, err = backend.UpdateIntegration(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			path := "/api/v1/integrations/" + integration.ID + "/revisions/" + saved.LatestRevision.ID + "/activate"
			denied := request(t, handler, http.MethodPost, path, "", "")
			if denied.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous retry=%d", denied.Code)
			}
			stillFailed := request(t, handler, http.MethodPost, path, "doko_admin_demo", "")
			if stillFailed.Code == http.StatusNoContent {
				t.Fatal("outage retry claimed delivery succeeded")
			}
			backend.phase = ""
			if err := service.ActivateIntegrationRevision(ctx, integration.ID, saved.LatestRevision.ID, platform.Actor{ID: "recovery-operator"}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				retried := request(t, handler, http.MethodPost, path, "doko_admin_demo", "")
				if retried.Code != http.StatusNoContent {
					t.Fatalf("retry=%d %s", retried.Code, retried.Body.String())
				}
				wire := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID, "doko_admin_demo", "")
				var result struct {
					Status platform.IntegrationPublishStatus `json:"publish_status"`
				}
				if wire.Code != http.StatusOK || json.Unmarshal(wire.Body.Bytes(), &result) != nil || !result.Status.LatestDeliveryReady || !result.Status.HasChanges || result.Status.ServingRevisionID != saved.LatestRevision.ID || result.Status.ServingPublicationID == "" {
					t.Fatalf("status=%d %s", wire.Code, wire.Body.String())
				}
			}
			revisions, err := backend.IntegrationRevisions(ctx, integration.ID)
			if err != nil || len(revisions) != 1 || revisions[0].ManifestHash != saved.LatestRevision.ManifestHash {
				t.Fatalf("duplicate or changed revision: %#v %v", revisions, err)
			}
			var snapshot map[string]any
			if json.Unmarshal(revisions[0].Snapshot, &snapshot) != nil || snapshot["display_name"] != "Reviewed name" {
				t.Fatalf("draft change leaked into retry: %s", revisions[0].Snapshot)
			}
			publications, err := backend.APIDeveloperAssetPublications(ctx, integration.DeploymentID, integration.ID)
			if err != nil || len(publications) != 1 || publications[0].PublishedBy != saved.LatestRevision.PublishedBy {
				t.Fatalf("asset publications=%#v %v", publications, err)
			}
			other, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "other-" + phase, VersionKey: "v1", DisplayName: "Other API", Visibility: model.VisibilityPrivate, Lifecycle: "draft"}, actor)
			if err != nil {
				t.Fatal(err)
			}
			cross := request(t, handler, http.MethodPost, "/api/v1/integrations/"+other.ID+"/revisions/"+saved.LatestRevision.ID+"/activate", "doko_admin_demo", "")
			if cross.Code != http.StatusNotFound {
				t.Fatalf("cross-API retry=%d %s", cross.Code, cross.Body.String())
			}
		})
	}
}
