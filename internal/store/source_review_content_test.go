package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func seedSourceReviewGeneration(t *testing.T, backend Store, source model.Source, body string, queued time.Time) (string, model.CrawlReviewDocument) {
	t.Helper()
	ctx := t.Context()
	runID, documentID, snapshotID := storeTestUUID(t), storeTestUUID(t), storeTestUUID(t)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	document := model.CrawlReviewDocument{ID: documentID, CrawlJobID: runID, SnapshotID: snapshotID, Title: "Guide", CanonicalURL: "https://docs.example.test/guide", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + hash, Changed: true}
	if memory, ok := backend.(*Memory); ok {
		memory.mu.Lock()
		defer memory.mu.Unlock()
		memory.crawls[source.ID] = append(memory.crawls[source.ID], model.CrawlJob{ID: runID, ProductID: source.ProductID, OrganisationID: source.OrganisationID, SourceID: source.ID, State: "review", FetchedCount: 1, DiscoveredCount: 1, QueuedAt: queued, StartedAt: &queued, FinishedAt: &queued})
		memory.crawlReviewDocuments[runID] = []model.CrawlReviewDocument{document}
		memory.knowledge[source.ProductID] = append(memory.knowledge[source.ProductID], model.KnowledgeRecord{ID: documentID, ProductID: source.ProductID, SourceID: source.ID, Title: document.Title, Text: body, URL: document.CanonicalURL, Visibility: model.VisibilityPrivate})
	} else {
		postgres := backend.(*Postgres)
		statements := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO crawl_jobs(id,organisation_id,product_id,source_id,state,fetched_count,discovered_count,pipeline_version,diagnostics,queued_at,started_at) VALUES($1,$2,$3,$4,'running',1,1,'review-content-test','{}',$5,$5)`, []any{runID, source.OrganisationID, source.ProductID, source.ID, queued}},
			{`INSERT INTO source_snapshots(id,organisation_id,product_id,source_id,crawl_job_id,canonical_url,object_key,content_sha256,content_type,response_status,trust_indicators) VALUES($1,$2,$3,$4,$5,$6,$7,decode($8,'hex'),'text/markdown',200,'{}')`, []any{snapshotID, source.OrganisationID, source.ProductID, source.ID, runID, document.CanonicalURL, "review-test-" + snapshotID, hash}},
			{`INSERT INTO knowledge_documents(id,organisation_id,product_id,source_id,snapshot_id,title,canonical_url,body,visibility,state,trust_level,injection_indicators) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'private','validated',70,'[]')`, []any{documentID, source.OrganisationID, source.ProductID, source.ID, snapshotID, document.Title, document.CanonicalURL, body}},
			{`INSERT INTO crawl_job_documents(crawl_job_id,knowledge_document_id,changed,assessment_state,assessment_trust_level,assessment_injection_indicators) VALUES($1,$2,true,'validated',70,'[]')`, []any{runID, documentID}},
			{`UPDATE crawl_jobs SET state='review',finished_at=$2 WHERE id=$1`, []any{runID, queued}},
		}
		for _, statement := range statements {
			if _, err := postgres.pool.Exec(ctx, statement.sql, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	return runID, document
}

func testSourceReviewContent(t *testing.T, backend Store) {
	t.Helper()
	ctx := t.Context()
	deployment, err := backend.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		org, createErr := backend.CreateOrganisation(ctx, model.Organisation{ID: storeTestUUID(t), Name: "Review", Slug: "review-content"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		deployment, err = backend.CreateDeployment(ctx, model.Deployment{ID: storeTestUUID(t), OrganisationID: org.ID, Name: "Review", Slug: "review-content"})
	}
	if err != nil {
		t.Fatal(err)
	}
	source, err := backend.CreateSource(ctx, model.Source{ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Name: "Exact review", Kind: "openapi", Location: "https://docs.example.test/openapi.json", Visibility: model.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	publish := func(runID string, document model.CrawlReviewDocument) model.SourcePublication {
		current, err := backend.Source(ctx, deployment.ID, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := docreview.PublicationContentHash([]model.CrawlReviewDocument{document})
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		_, publication, err := backend.PublishSource(ctx, deployment.ID, source.ID, current.Revision, model.SourcePublication{ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, SourceID: source.ID, CrawlJobID: runID, Visibility: model.VisibilityPrivate, ContentHash: hash, DocumentCount: 1, ReviewedBy: "reviewer", ReviewedAt: now, PublishedAt: now}, []string{document.ID})
		if err != nil {
			t.Fatal(err)
		}
		return publication
	}
	firstTime := time.Now().UTC().Add(-2 * time.Hour)
	firstRun, firstDocument := seedSourceReviewGeneration(t, backend, source, "# Install\nUse version 1.0.", firstTime)
	firstPublication := publish(firstRun, firstDocument)
	currentBody := "# Install\nUse version 2.0.\n<iframe src='https://untrusted.example'></iframe>"
	currentRun, currentDocument := seedSourceReviewGeneration(t, backend, source, currentBody, firstTime.Add(time.Minute))
	futureRun, futureDocument := seedSourceReviewGeneration(t, backend, source, "Later publication must not become an earlier review's comparison.", firstTime.Add(2*time.Minute))
	_ = publish(futureRun, futureDocument)
	content, err := backend.SourceReviewContent(ctx, deployment.ID, source.ID, currentRun, currentDocument.ID)
	if err != nil || content.Body != currentBody || content.Document.ContentHash != currentDocument.ContentHash || content.Previous == nil || content.Previous.PublicationID != firstPublication.ID || content.Previous.Body != "# Install\nUse version 1.0." {
		t.Fatalf("exact current content=%#v %v", content, err)
	}
	oldest, err := backend.SourceReviewContent(ctx, deployment.ID, source.ID, firstRun, firstDocument.ID)
	if err != nil || oldest.Previous != nil || oldest.Body != "# Install\nUse version 1.0." {
		t.Fatalf("historical read=%#v %v", oldest, err)
	}
	review, err := backend.SourceReview(ctx, deployment.ID, source.ID, firstRun)
	if err != nil || len(review.PublishedDocumentIDs) != 1 || review.PublishedDocumentIDs[0] != firstDocument.ID {
		t.Fatalf("published decisions=%#v %v", review, err)
	}
	for _, scope := range [][4]string{{storeTestUUID(t), source.ID, currentRun, currentDocument.ID}, {deployment.ID, storeTestUUID(t), currentRun, currentDocument.ID}, {deployment.ID, source.ID, firstRun, currentDocument.ID}, {deployment.ID, source.ID, currentRun, futureDocument.ID}} {
		if value, err := backend.SourceReviewContent(ctx, scope[0], scope[1], scope[2], scope[3]); !errors.Is(err, ErrNotFound) || value.Body != "" {
			t.Fatalf("invalid scope returned content=%#v %v", value, err)
		}
	}
}

func TestMemorySourceReviewContent(t *testing.T) { testSourceReviewContent(t, NewMemory()) }
func TestPostgresSourceReviewContent(t *testing.T) {
	_, backend := migratedPostgresForStoreTest(t)
	testSourceReviewContent(t, backend)
}
