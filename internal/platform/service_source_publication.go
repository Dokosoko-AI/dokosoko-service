package platform

import (
	"context"
	"fmt"
	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"strings"
)

type SourcePublicationInput struct {
	Revision            int64
	CrawlJobID          string
	DocumentIDs         []string
	AcknowledgeReviewed bool
}

func (s *Service) SourceReview(ctx context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	review, err := s.store.SourceReview(ctx, productID, sourceID, strings.TrimSpace(crawlJobID))
	if err != nil {
		return model.SourceReview{}, err
	}
	if review.CrawlJob.ProductID != productID || review.CrawlJob.SourceID != sourceID {
		return model.SourceReview{}, store.ErrNotFound
	}
	return review, nil
}

func (s *Service) PublishSource(ctx context.Context, productID, sourceID string, input SourcePublicationInput, actor Actor) (model.Source, model.SourcePublication, error) {
	current, err := s.store.Source(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if current.Quarantined {
		return model.Source{}, model.SourcePublication{}, fmt.Errorf("%w: quarantined sources require remediation and a clean crawl", ErrUnsafeForPublic)
	}
	if !input.AcknowledgeReviewed {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	crawls, err := s.store.CrawlJobs(ctx, productID, sourceID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	input.CrawlJobID = strings.TrimSpace(input.CrawlJobID)
	latestReviewable := len(crawls) > 0 && crawls[0].ID == input.CrawlJobID && crawls[0].FinishedAt != nil && (crawls[0].State == "review" || crawls[0].State == "succeeded")
	completeCoverage := len(crawls) > 0 && crawls[0].FetchedCount > 0 && crawls[0].FailedCount == 0 && crawls[0].SkippedCount == 0
	if !latestReviewable || !completeCoverage || input.Revision != current.Revision {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	review, err := s.store.SourceReview(ctx, productID, sourceID, input.CrawlJobID)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if review.Publication != nil || len(review.Documents) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	documents := make(map[string]model.CrawlReviewDocument, len(review.Documents))
	for _, document := range review.Documents {
		documents[document.ID] = document
	}
	selected := make([]model.CrawlReviewDocument, 0, len(input.DocumentIDs))
	selectedIDs := make([]string, 0, len(input.DocumentIDs))
	seen := make(map[string]bool, len(input.DocumentIDs))
	for _, documentID := range input.DocumentIDs {
		documentID = strings.TrimSpace(documentID)
		document, ok := documents[documentID]
		if documentID == "" || seen[documentID] || !ok || !docreview.SafeAssessment(document.State, document.InjectionIndicators) {
			return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
		}
		seen[documentID] = true
		selectedIDs = append(selectedIDs, documentID)
		selected = append(selected, document)
	}
	if len(selected) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrSourceReviewRequired
	}
	publicationHash, err := docreview.PublicationContentHash(selected)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	now := s.now()
	publication := model.SourcePublication{ID: id, OrganisationID: current.OrganisationID, ProductID: productID, SourceID: sourceID, CrawlJobID: input.CrawlJobID, Visibility: current.Visibility, ContentHash: publicationHash, DocumentCount: len(selectedIDs), ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now}
	updated, publication, err := s.store.PublishSource(ctx, productID, sourceID, input.Revision, publication, selectedIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "source.publication.created", TargetType: "source_publication", TargetID: publication.ID, Prior: map[string]any{"source_revision": current.Revision}, Current: map[string]any{"source_id": sourceID, "source_revision": updated.Revision, "crawl_job_id": publication.CrawlJobID, "publication_revision": publication.Revision, "content_hash": publication.ContentHash, "document_count": publication.DocumentCount, "visibility": updated.Visibility}, RequestID: actor.RequestID, CreatedAt: now})
	return updated, publication, err
}
