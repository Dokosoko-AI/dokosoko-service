package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresDocumentationExplorerReturnsExactMapAndPagination(t *testing.T) {
	pool, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployment, err := postgres.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		organisationID := storeTestUUID(t)
		if _, err = postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Documentation explorer", Slug: "documentation-explorer-" + organisationID[:8]}); err != nil {
			t.Fatal(err)
		}
		deployment, err = postgres.CreateDeployment(ctx, model.Deployment{
			ID: storeTestUUID(t), OrganisationID: organisationID, Name: "Documentation explorer", Slug: "documentation-explorer",
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceID := storeTestUUID(t)
	if _, err := postgres.CreateSource(ctx, model.Source{
		ID: sourceID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		Name: "Explorer source", Kind: "upload", Location: "explorer.md",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	runID := storeTestUUID(t)
	if _, err := postgres.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		AssetKind: model.DeveloperAssetDocumentation, TargetID: sourceID, TargetKey: "source:" + sourceID, SourceID: sourceID,
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), QueuedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	documentAID, documentBID, mapID := storeTestUUID(t), storeTestUUID(t), storeTestUUID(t)
	mapHash := developerAssetTestHash("c")
	if err := postgres.SaveDocumentationIngestionOutput(ctx, deployment.ID, DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{
			{ID: documentAID, DeploymentID: deployment.ID, IngestionRunID: runID, SourcePath: "a.md", Title: "A", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "A", ContentHash: developerAssetTestHash("d"), Visibility: model.VisibilityPrivate, Ordinal: 0, Metadata: json.RawMessage(`{}`)},
			{ID: documentBID, DeploymentID: deployment.ID, IngestionRunID: runID, SourcePath: "b.md", Title: "B", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "B", ContentHash: developerAssetTestHash("e"), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
		},
		Map: &model.DocumentationMap{
			ID: mapID, DeploymentID: deployment.ID, IngestionRunID: runID, MapVersion: "documentation-map-v1",
			Map:           model.DocumentationMapBody{Overview: "Exact PostgreSQL map.", Documents: []model.KnowledgeMapEntry{}, Topics: []model.KnowledgeMapEntry{}, Workflows: []model.KnowledgeMapEntry{}},
			AgentMarkdown: "# Exact PostgreSQL map\n", ContentHash: mapHash, Visibility: model.VisibilityPrivate,
		},
	}); err != nil {
		t.Fatal(err)
	}

	type reviewFixture struct {
		decision string
		reason   string
		reviewer string
		reviewed time.Time
	}
	reviews := []reviewFixture{
		{decision: "excluded", reason: "Superseded PostgreSQL guidance.", reviewer: "old-reviewer", reviewed: now.Add(time.Hour)},
		{decision: "quarantined", reason: "PostgreSQL security review required.", reviewer: "security-reviewer", reviewed: now.Add(2 * time.Hour)},
		{decision: "included", reviewer: "current-reviewer", reviewed: now.Add(3 * time.Hour)},
	}
	for index, review := range reviews {
		crawlID, publicationID := storeTestUUID(t), storeTestUUID(t)
		if _, err := pool.Exec(ctx, `INSERT INTO crawl_jobs(id,organisation_id,product_id,source_id,state,discovered_count,fetched_count,changed_count,queued_at,started_at,finished_at)
			VALUES($1,$2,$3,$4,'succeeded',1,1,1,$5,$5,$5)`, crawlID, deployment.OrganisationID, deployment.ID, sourceID, review.reviewed); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO source_publications(id,organisation_id,product_id,source_id,crawl_job_id,revision,visibility,content_hash,document_count,reviewed_by,reviewed_at,published_at)
			VALUES($1,$2,$3,$4,$5,$6,'private',$7,1,$8,$9,$9)`, publicationID, deployment.OrganisationID, deployment.ID, sourceID, crawlID, index+1, developerAssetTestHash([]string{"f", "e", "c"}[index]), review.reviewer, review.reviewed); err != nil {
			t.Fatal(err)
		}
		var ordinal *int
		if review.decision == "included" {
			value := 0
			ordinal = &value
		}
		if _, err := pool.Exec(ctx, `INSERT INTO source_publication_document_selections(source_publication_id,deployment_id,documentation_document_id,decision,reason,ordinal,content_hash,reviewed_by,reviewed_at,created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`, publicationID, deployment.ID, documentAID, review.decision, review.reason, ordinal, developerAssetTestHash("d"), review.reviewer, review.reviewed); err != nil {
			t.Fatal(err)
		}
	}

	first, err := postgres.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: deployment.ID, IngestionRunID: runID, Limit: 1})
	if err != nil || len(first.Items) != 1 || first.Total != 2 || !first.HasMore || first.Items[0].Document.ID != documentAID || first.Items[0].DocumentationMap == nil || first.Items[0].DocumentationMap.ID != mapID || first.Items[0].DocumentationMap.Map.Overview != "Exact PostgreSQL map." || len(first.Items[0].SourcePublicationSelections) != 3 || first.Items[0].SourcePublicationSelections[0].Decision != "included" || first.Items[0].SourcePublicationSelections[1].Decision != "quarantined" || first.Items[0].SourcePublicationSelections[1].Reason != "PostgreSQL security review required." || first.Items[0].SourcePublicationSelections[2].Decision != "excluded" {
		t.Fatalf("first PostgreSQL documentation page = %#v, err=%v", first, err)
	}
	second, err := postgres.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: deployment.ID, IngestionRunID: runID, Limit: 1, Offset: 1})
	if err != nil || len(second.Items) != 1 || second.Total != 2 || second.HasMore || second.Items[0].Document.ID != documentBID || second.Items[0].DocumentationMap == nil || second.Items[0].DocumentationMap.ContentHash != mapHash || second.Items[0].SourcePublicationSelections == nil || len(second.Items[0].SourcePublicationSelections) != 0 {
		t.Fatalf("second PostgreSQL documentation page = %#v, err=%v", second, err)
	}
}
