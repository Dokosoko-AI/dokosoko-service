package store

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func testDocumentationLibrary(t *testing.T, backend Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := storeTestUUID(t)
	_, err = backend.CreateSource(ctx, model.Source{ID: sourceID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Name: "Library source " + sourceID, Kind: "upload", Location: "library.md"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for index, text := range []string{"oldphrase only", "currentphrase only"} {
		runID, documentID := storeTestUUID(t), storeTestUUID(t)
		ids = append(ids, documentID)
		_, err = backend.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{ID: runID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, AssetKind: model.DeveloperAssetDocumentation, TargetID: sourceID, TargetKey: "source:" + sourceID, SourceID: sourceID, State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, Versions: model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"}, RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), QueuedAt: time.Now().UTC().Add(time.Duration(index) * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		err = backend.SaveDocumentationIngestionOutput(ctx, deployment.ID, DocumentationIngestionOutput{Documents: []model.DocumentationDocument{{ID: documentID, DeploymentID: deployment.ID, IngestionRunID: runID, SourcePath: "guide.md", Title: "Guide", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: text, ContentHash: developerAssetTestHash(string(rune('a' + index))), Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`)}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	query := DocumentationLibraryQuery{DeploymentID: deployment.ID, SourceID: sourceID, Limit: 1}
	current, err := backend.DocumentationLibrary(ctx, query)
	if err != nil || current.Total != 1 || len(current.Items) != 1 || current.HasMore || current.Items[0].ID != ids[1] || current.Items[0].PreviousDocumentID != ids[0] || current.Items[0].Decision != "unreviewed" {
		t.Fatalf("current=%#v, err=%v", current, err)
	}
	encoded, _ := json.Marshal(current)
	for _, forbidden := range []string{"normalized_markdown", "currentphrase", "sections", "diagnostics"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("list hydrated content: %s", encoded)
		}
	}
	query.Query = "oldphrase"
	page, err := backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 0 {
		t.Fatalf("old match leaked into current files: %#v %v", page, err)
	}
	query.Query = "currentphrase"
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 1 {
		t.Fatalf("current body search failed: %#v %v", page, err)
	}
	query.Query = ""
	query.History = true
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 2 || !page.HasMore || page.Items[0].ID != ids[1] {
		t.Fatalf("history=%#v %v", page, err)
	}
	query.Offset = 1
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != ids[0] || page.HasMore {
		t.Fatalf("history next page=%#v %v", page, err)
	}
	query.DeploymentID = storeTestUUID(t)
	page, err = backend.DocumentationLibrary(ctx, query)
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("cross-deployment files=%#v %v", page, err)
	}
	// Historical identifiers continue to resolve their original evidence.
	old, err := backend.DocumentationCandidateDocument(ctx, deployment.ID, ids[0])
	if err != nil || old.Document.NormalizedMarkdown != "oldphrase only" {
		t.Fatalf("historical content changed: %#v %v", old, err)
	}
}

func TestMemoryDocumentationLibrary(t *testing.T) { testDocumentationLibrary(t, NewMemory()) }

func TestPostgresDocumentationLibrary(t *testing.T) {
	_, backend := migratedPostgresForStoreTest(t)
	if _, err := backend.Deployment(t.Context()); err == ErrNotFound {
		org, err := backend.CreateOrganisation(t.Context(), model.Organisation{ID: storeTestUUID(t), Name: "Library", Slug: "library"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = backend.CreateDeployment(t.Context(), model.Deployment{ID: storeTestUUID(t), OrganisationID: org.ID, Name: "Library", Slug: "library"}); err != nil {
			t.Fatal(err)
		}
	}
	testDocumentationLibrary(t, backend)
}
