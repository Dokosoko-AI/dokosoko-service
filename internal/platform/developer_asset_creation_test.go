package platform

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestContractCreationRequestNormalizationAndCurrentRoot(t *testing.T) {
	backend := store.NewMemory()
	service := New(backend)
	actor := Actor{ID: "creator"}
	input := APIContractInput{Name: "Orders", Slug: "orders", RequestKey: "contract-creation-request-0001"}
	first, err := service.SaveAPIContract(t.Context(), "", input, actor)
	if err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.Name, changed.Slug, changed.RequestKey, changed.Revision, changed.Lifecycle = "Renamed", "renamed", "", first.Revision, "archived"
	updated, err := service.SaveAPIContract(t.Context(), first.ID, changed, actor)
	if err != nil {
		t.Fatal(err)
	}
	normalized := input
	normalized.Name, normalized.Slug = " Orders ", " ORDERS "
	recovered, err := service.SaveAPIContract(t.Context(), "", normalized, actor)
	if err != nil || recovered.ID != first.ID || recovered.Name != "Renamed" || recovered.Revision != updated.Revision || recovered.Lifecycle != "archived" {
		t.Fatalf("recovery reset current root: %+v %v", recovered, err)
	}
	changed = input
	changed.Description = "Different intent"
	if _, err := service.SaveAPIContract(t.Context(), "", changed, actor); !errors.Is(err, store.ErrDeveloperAssetCreationConflict) {
		t.Fatalf("changed input: %v", err)
	}
	changed = input
	changed.RequestKey = "short"
	if _, err := service.SaveAPIContract(t.Context(), "", changed, actor); err == nil {
		t.Fatal("accepted short request key")
	}
	if _, err := service.SaveAPIContract(t.Context(), first.ID, input, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("update accepted creation key: %v", err)
	}
	other, err := service.SaveAPIContract(t.Context(), "", input, Actor{ID: "different-reviewer"})
	if err != nil || other.ID == first.ID {
		t.Fatalf("other actor adopted a previous request: %+v %v", other, err)
	}
}

// This fixture isolates transaction behavior. Its synthetic collection revision
// is not an ingestion/publication acceptance claim; that path is tested above.
func testDeveloperAssetCreationTransactions(t *testing.T, backend store.Store) {
	t.Helper()
	deployment, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"api_contract", "documentation_collection"} {
		t.Run(kind, func(t *testing.T) {
			create := func(slug, requestDigest, inputDigest, auditID string) (string, error) {
				id, err := randomUUID()
				if err != nil {
					return "", err
				}
				action := "api_contract.saved"
				if kind == "documentation_collection" {
					action = "documentation_collection.revision_saved"
				}
				request := store.DeveloperAssetCreation{RequestDigest: requestDigest, InputDigest: inputDigest, Audit: model.AuditEvent{ID: auditID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: "creator", Action: action, TargetType: kind, TargetID: id, Current: map[string]any{"name": slug}, CreatedAt: New(backend).now()}}
				if kind == "api_contract" {
					value, err := backend.SaveAPIContract(t.Context(), model.APIContract{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: slug, Slug: slug, Kind: "openapi", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, 0, request)
					return value.ID, err
				}
				revisionID, err := randomUUID()
				if err != nil {
					return "", err
				}
				value, err := backend.CreateDocumentationCollection(t.Context(), model.DocumentationCollection{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: slug, Slug: slug, Visibility: model.VisibilityPrivate, Lifecycle: "active"}, store.DocumentationCollectionRevisionRecord{Revision: model.DocumentationCollectionRevision{ID: revisionID, DeploymentID: deployment.ID, DocumentationCollectionID: id, Revision: 1, Visibility: model.VisibilityPrivate, ContentHash: "sha256:" + strings.Repeat("a", 64), SelectionManifest: json.RawMessage(`[]`), ReviewedBy: "creator", ReviewedAt: New(backend).now()}}, request)
				return value.ID, err
			}
			const count = 12
			type result struct {
				id  string
				err error
			}
			results := make(chan result, count)
			var group sync.WaitGroup
			slug := strings.ReplaceAll(kind, "_", "-")
			for range count {
				group.Add(1)
				go func() {
					defer group.Done()
					id, err := create(slug, strings.Repeat("a", 64), strings.Repeat("b", 64), randomID("audit"))
					results <- result{id, err}
				}()
			}
			group.Wait()
			close(results)
			first := ""
			for value := range results {
				if value.err != nil {
					t.Fatal(value.err)
				}
				if first == "" {
					first = value.id
				}
				if value.id != first {
					t.Fatalf("duplicate creation %s != %s", value.id, first)
				}
			}
			if kind == "documentation_collection" {
				revisions, err := backend.DocumentationCollectionRevisions(t.Context(), deployment.ID, first)
				if err != nil || len(revisions) != 1 {
					t.Fatalf("duplicate initial revision: %v %v", revisions, err)
				}
			}
			events, err := backend.AuditEvents(t.Context(), deployment.OrganisationID)
			if err != nil {
				t.Fatal(err)
			}
			audits, auditID := 0, ""
			for _, event := range events {
				if event.TargetID == first {
					audits++
					auditID = event.ID
				}
			}
			if audits != 1 {
				t.Fatalf("creation audits=%d", audits)
			}
			if _, err := create(slug, strings.Repeat("a", 64), strings.Repeat("c", 64), randomID("audit")); !errors.Is(err, store.ErrDeveloperAssetCreationConflict) {
				t.Fatalf("changed input reused request: %v", err)
			}
			if _, err := create(slug, strings.Repeat("d", 64), strings.Repeat("b", 64), randomID("audit")); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("new request adopted existing slug: %v", err)
			}
			// Audit conflict must roll back both resource creation and the request key.
			if _, err := create(slug+"-atomic", strings.Repeat("e", 64), strings.Repeat("f", 64), auditID); err == nil {
				t.Fatal("creation succeeded despite duplicate audit key")
			}
			if _, err := create(slug+"-atomic", strings.Repeat("e", 64), strings.Repeat("f", 64), randomID("audit")); err != nil {
				t.Fatalf("failed transaction retained creation or request: %v", err)
			}
		})
	}
}
func TestDeveloperAssetCreationTransactions(t *testing.T) {
	testDeveloperAssetCreationTransactions(t, store.NewMemory())
}
func TestDeveloperAssetCreationTransactionsPostgres(t *testing.T) {
	testDeveloperAssetCreationTransactions(t, knowledgePostgresFixture(t))
}
