package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresSourcePublicationAtomicallyPersistsTypedReview(t *testing.T) {
	pool, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployment, err := postgres.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		organisationID := storeTestUUID(t)
		if _, err = postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Typed source publication", Slug: "typed-source-" + organisationID[:8]}); err != nil {
			t.Fatal(err)
		}
		deployment, err = postgres.CreateDeployment(ctx, model.Deployment{
			ID: storeTestUUID(t), OrganisationID: organisationID, Name: "Typed source publication", Slug: "typed-source-publication",
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceID, crawlID := storeTestUUID(t), storeTestUUID(t)
	source, err := postgres.CreateSource(ctx, model.Source{
		ID: sourceID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		Name: "Typed documentation", Kind: "upload", Location: "typed.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `INSERT INTO crawl_jobs(
		id,organisation_id,product_id,source_id,state,discovered_count,fetched_count,changed_count,
		failed_count,skipped_count,pipeline_version,diagnostics,queued_at,started_at,finished_at
	) VALUES($1,$2,$3,$4,'review',2,2,2,0,0,'crawler-documentation-candidate/1','{}'::jsonb,$5,$5,$5)`,
		crawlID, deployment.OrganisationID, deployment.ID, sourceID, now); err != nil {
		t.Fatal(err)
	}
	legacyIDs := []string{storeTestUUID(t), storeTestUUID(t)}
	for index, legacyID := range legacyIDs {
		snapshotID := storeTestUUID(t)
		canonicalURL := "https://docs.example.test/" + string(rune('a'+index))
		if _, err := pool.Exec(ctx, `INSERT INTO source_snapshots(
			id,organisation_id,product_id,source_id,crawl_job_id,canonical_url,object_key,content_sha256,content_type,response_status,trust_indicators
		) VALUES($1,$2,$3,$4,$5,$6,$7,decode($8,'hex'),'text/markdown',200,'{}'::jsonb)`,
			snapshotID, deployment.OrganisationID, deployment.ID, sourceID, crawlID, canonicalURL, "snapshot-"+legacyID, strings.Repeat(string(rune('1'+index)), 64)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO knowledge_documents(
			id,organisation_id,product_id,source_id,snapshot_id,title,canonical_url,body,visibility,state,trust_level,injection_indicators
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'private','validated',70,'[]'::jsonb)`,
			legacyID, deployment.OrganisationID, deployment.ID, sourceID, snapshotID, "Document", canonicalURL, "Body"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO crawl_job_documents(
			crawl_job_id,knowledge_document_id,changed,assessment_state,assessment_trust_level,assessment_injection_indicators
		) VALUES($1,$2,true,'validated',70,'[]'::jsonb)`, crawlID, legacyID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := postgres.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{
		ID: crawlID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		AssetKind: model.DeveloperAssetDocumentation, TargetID: sourceID, TargetKey: "source:" + sourceID, SourceID: sourceID,
		State: model.DeveloperAssetIngestionReviewReady, Attempt: 1,
		Versions:    model.ProcessorVersions{Pipeline: "pipeline-v1", Parser: "parser-v1", Normalizer: "normalizer-v1", Mapper: "mapper-v1"},
		RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`), DiscoveredCount: 2, AcquiredCount: 2,
		QueuedAt: now, StartedAt: &now, FinishedAt: &now,
	}); err != nil {
		t.Fatal(err)
	}
	typedIDs := []string{storeTestUUID(t), storeTestUUID(t)}
	mapID, mapHash := storeTestUUID(t), developerAssetTestHash("9")
	if err := postgres.SaveDocumentationIngestionOutput(ctx, deployment.ID, DocumentationIngestionOutput{
		Documents: []model.DocumentationDocument{
			{ID: typedIDs[0], DeploymentID: deployment.ID, IngestionRunID: crawlID, LegacyKnowledgeDocumentID: legacyIDs[0], SourcePath: "a.md", Title: "A", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "A", ContentHash: developerAssetTestHash("7"), Visibility: model.VisibilityPrivate, Ordinal: 0, Metadata: json.RawMessage(`{}`)},
			{ID: typedIDs[1], DeploymentID: deployment.ID, IngestionRunID: crawlID, LegacyKnowledgeDocumentID: legacyIDs[1], SourcePath: "b.md", Title: "B", Kind: "guide", MediaType: "text/markdown", NormalizedMarkdown: "B", ContentHash: developerAssetTestHash("8"), Visibility: model.VisibilityPrivate, Ordinal: 1, Metadata: json.RawMessage(`{}`)},
		},
		Map: &model.DocumentationMap{
			ID: mapID, DeploymentID: deployment.ID, IngestionRunID: crawlID, MapVersion: "map-v1",
			Map: model.DocumentationMapBody{Overview: "Exact typed map."}, AgentMarkdown: "# Exact typed map\n",
			ContentHash: mapHash, Visibility: model.VisibilityPrivate,
		},
	}); err != nil {
		t.Fatal(err)
	}

	sourceReview, err := postgres.SourceReview(ctx, deployment.ID, sourceID, crawlID)
	if err != nil || len(sourceReview.Documents) != 2 {
		t.Fatalf("source review = %#v, err=%v", sourceReview, err)
	}
	selected := sourceReview.Documents[:1]
	publicationHash, err := docreview.PublicationContentHash(selected)
	if err != nil {
		t.Fatal(err)
	}
	publication := model.SourcePublication{
		ID: storeTestUUID(t), CrawlJobID: crawlID, ContentHash: publicationHash,
		ReviewedBy: "postgres-reviewer", ReviewedAt: now, PublishedAt: now,
	}
	updated, published, err := postgres.PublishSource(ctx, deployment.ID, sourceID, source.Revision, publication, []string{selected[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Published || published.DocumentCount != 1 {
		t.Fatalf("updated source=%#v publication=%#v", updated, published)
	}
	typedReview, err := postgres.SourcePublicationDocumentationReview(ctx, deployment.ID, published.ID)
	if err != nil || len(typedReview.Selections) != 2 || typedReview.MapLink == nil || typedReview.MapLink.DocumentationMapID != mapID || typedReview.MapLink.ContentHash != mapHash {
		t.Fatalf("typed review=%#v err=%v", typedReview, err)
	}
	includedCount, excludedCount := 0, 0
	for _, decision := range typedReview.Selections {
		switch decision.Decision {
		case "included":
			includedCount++
			if decision.Ordinal == nil || *decision.Ordinal != 0 || decision.ReviewedBy != publication.ReviewedBy || !decision.ReviewedAt.Equal(publication.ReviewedAt) {
				t.Fatalf("included decision=%#v", decision)
			}
		case "excluded":
			excludedCount++
			if decision.Reason != sourcePublicationDocumentExcludedReason {
				t.Fatalf("excluded decision=%#v", decision)
			}
		}
	}
	if includedCount != 1 || excludedCount != 1 {
		t.Fatalf("typed decisions=%#v", typedReview.Selections)
	}
	typedRun, err := postgres.DeveloperAssetIngestionRun(ctx, deployment.ID, crawlID)
	if err != nil || typedRun.State != model.DeveloperAssetIngestionPublished {
		t.Fatalf("typed run=%#v err=%v", typedRun, err)
	}
	records, err := postgres.DocumentationCandidateDocuments(ctx, DocumentationCandidateQuery{DeploymentID: deployment.ID, SourcePublicationID: published.ID, Limit: 20})
	if err != nil || records.Total != 2 || len(records.Items) != 2 || records.Items[0].DocumentationMap == nil || records.Items[0].DocumentationMap.ID != mapID {
		t.Fatalf("typed retrieval=%#v err=%v", records, err)
	}
}
