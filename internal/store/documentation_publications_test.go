package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemorySourcePublicationPinsExactCrawlDocuments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	source, err := memory.CreateSource(ctx, model.Source{ID: "source_review_test", OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Reviewed docs", Kind: "upload", Location: "review.md"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := model.CrawlJob{ID: "crawl_review_test", OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID, State: "review", DiscoveredCount: 2, FetchedCount: 2, ChangedCount: 2, QueuedAt: now, FinishedAt: &now}
	runningJob := model.CrawlJob{ID: "crawl_running_test", OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID, State: "running", QueuedAt: now.Add(time.Second)}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job, runningJob}
	memory.crawlReviewDocuments[job.ID] = []model.CrawlReviewDocument{
		{ID: "document_selected", CrawlJobID: job.ID, SnapshotID: "snapshot_selected", Title: "Selected", CanonicalURL: "https://docs.example.test/selected", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("a", 64), Changed: true},
		{ID: "document_excluded", CrawlJobID: job.ID, SnapshotID: "snapshot_excluded", Title: "Excluded", CanonicalURL: "https://docs.example.test/excluded", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("b", 64), Changed: true},
	}
	memory.knowledge[source.ProductID] = append(memory.knowledge[source.ProductID],
		model.KnowledgeRecord{ID: "document_selected", ProductID: source.ProductID, SourceID: source.ID, Title: "Selected", Text: "selected text", Published: false, Visibility: model.VisibilityPrivate},
		model.KnowledgeRecord{ID: "document_excluded", ProductID: source.ProductID, SourceID: source.ID, Title: "Excluded", Text: "excluded text", Published: false, Visibility: model.VisibilityPrivate},
	)
	memory.mu.Unlock()

	review, err := memory.SourceReview(ctx, source.ProductID, source.ID, job.ID)
	if err != nil || len(review.Documents) != 2 || review.Publication != nil {
		t.Fatalf("review = %#v, err = %v", review, err)
	}
	publicationHash, err := docreview.PublicationContentHash([]model.CrawlReviewDocument{review.Documents[0]})
	if err != nil {
		t.Fatal(err)
	}
	publication := model.SourcePublication{ID: "publication_review_test", CrawlJobID: job.ID, ContentHash: publicationHash, ReviewedBy: "reviewer", ReviewedAt: now, PublishedAt: now}
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("older finished crawl published while a newer crawl was active with err %v", err)
	}
	memory.mu.Lock()
	memory.crawls[source.ID] = []model.CrawlJob{job}
	memory.mu.Unlock()
	staleHash := publication
	staleHash.ContentHash = "sha256:" + strings.Repeat("f", 64)
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, staleHash, []string{"document_selected"}); err != ErrConflict {
		t.Fatalf("stale service-computed publication hash was accepted with err %v", err)
	}
	updated, published, err := memory.PublishSource(ctx, source.ProductID, source.ID, source.Revision, publication, []string{"document_selected"})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Published || published.DocumentCount != 1 || published.CrawlJobID != job.ID {
		t.Fatalf("source = %#v, publication = %#v", updated, published)
	}
	items, err := memory.PrivateKnowledge(ctx, source.ProductID, []string{published.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "document_selected" {
		t.Fatalf("exact publication returned %#v", items)
	}
	updated.Visibility = model.VisibilityPublic
	updated, err = memory.UpdateSource(ctx, updated, updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	publicItems, err := memory.PublicKnowledge(ctx, source.ProductID, []string{published.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(publicItems) != 0 {
		t.Fatalf("a private publication became public after only changing current source visibility: %#v", publicItems)
	}
	if _, _, err := memory.PublishSource(ctx, source.ProductID, source.ID, updated.Revision, publication, []string{"document_excluded"}); err != ErrConflict {
		t.Fatalf("same crawl generation republished with err %v", err)
	}
}
