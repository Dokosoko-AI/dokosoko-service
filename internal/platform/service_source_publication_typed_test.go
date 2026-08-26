package platform

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type sourcePublicationCaptureStore struct {
	*store.Memory
	source              model.Source
	review              model.SourceReview
	publishErr          error
	capturedPublication model.SourcePublication
	capturedDocumentIDs []string
}

func (s *sourcePublicationCaptureStore) Source(_ context.Context, productID, sourceID string) (model.Source, error) {
	if s.source.ProductID != productID || s.source.ID != sourceID {
		return model.Source{}, store.ErrNotFound
	}
	return s.source, nil
}

func (s *sourcePublicationCaptureStore) CrawlJobs(_ context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	if s.review.CrawlJob.ProductID != productID || s.review.CrawlJob.SourceID != sourceID {
		return nil, store.ErrNotFound
	}
	return []model.CrawlJob{s.review.CrawlJob}, nil
}

func (s *sourcePublicationCaptureStore) SourceReview(_ context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	if s.review.Source.ProductID != productID || s.review.Source.ID != sourceID || s.review.CrawlJob.ID != crawlJobID {
		return model.SourceReview{}, store.ErrNotFound
	}
	return s.review, nil
}

func (s *sourcePublicationCaptureStore) PublishSource(_ context.Context, productID, sourceID string, expected int64, publication model.SourcePublication, documentIDs []string) (model.Source, model.SourcePublication, error) {
	s.capturedPublication = publication
	s.capturedDocumentIDs = append([]string(nil), documentIDs...)
	if s.publishErr != nil {
		return model.Source{}, model.SourcePublication{}, s.publishErr
	}
	if productID != s.source.ProductID || sourceID != s.source.ID || expected != s.source.Revision {
		return model.Source{}, model.SourcePublication{}, store.ErrConflict
	}
	updated := s.source
	updated.Published = true
	updated.Revision++
	publication.Revision = 1
	return updated, publication, nil
}

func newSourcePublicationCaptureStore(t *testing.T) (*sourcePublicationCaptureStore, time.Time) {
	t.Helper()
	now := time.Date(2026, time.August, 26, 3, 4, 5, 0, time.UTC)
	source := model.Source{
		ID: "source-service-bridge", OrganisationID: "org_acme", ProductID: "prod_acme",
		Name: "Service bridge", Kind: "upload", Location: "bridge.md", Visibility: model.VisibilityPrivate, Revision: 7,
	}
	job := model.CrawlJob{
		ID: "crawl-service-bridge", OrganisationID: source.OrganisationID, ProductID: source.ProductID, SourceID: source.ID,
		State: "review", FetchedCount: 2, DiscoveredCount: 2, QueuedAt: now, FinishedAt: &now,
	}
	documents := []model.CrawlReviewDocument{
		{ID: "legacy-selected", CrawlJobID: job.ID, SnapshotID: "snapshot-selected", Title: "Selected", CanonicalURL: "https://docs.example.test/selected", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("1", 64)},
		{ID: "legacy-excluded", CrawlJobID: job.ID, SnapshotID: "snapshot-excluded", Title: "Excluded", CanonicalURL: "https://docs.example.test/excluded", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("2", 64)},
	}
	return &sourcePublicationCaptureStore{
		Memory: store.NewMemory(), source: source,
		review: model.SourceReview{Source: source, CrawlJob: job, Documents: documents},
	}, now
}

func TestServicePublishSourceCarriesExactReviewerEvidenceIntoAtomicStoreCall(t *testing.T) {
	t.Parallel()
	capture, now := newSourcePublicationCaptureStore(t)
	service := New(capture)
	service.now = func() time.Time { return now }
	updated, publication, err := service.PublishSource(context.Background(), capture.source.ProductID, capture.source.ID, SourcePublicationInput{
		Revision: capture.source.Revision, CrawlJobID: capture.review.CrawlJob.ID,
		DocumentIDs: []string{"legacy-selected"}, AcknowledgeReviewed: true,
	}, Actor{ID: "reviewer-123", RequestID: "request-123"})
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := docreview.PublicationContentHash(capture.review.Documents[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Published || publication.ReviewedBy != "reviewer-123" || !publication.ReviewedAt.Equal(now) || !publication.PublishedAt.Equal(now) ||
		capture.capturedPublication.ContentHash != wantHash || capture.capturedPublication.CrawlJobID != capture.review.CrawlJob.ID ||
		!slices.Equal(capture.capturedDocumentIDs, []string{"legacy-selected"}) {
		t.Fatalf("captured publication=%#v document_ids=%#v updated=%#v", capture.capturedPublication, capture.capturedDocumentIDs, updated)
	}
}

func TestServicePublishSourceDoesNotAuditFailedAtomicStoreBridge(t *testing.T) {
	t.Parallel()
	capture, now := newSourcePublicationCaptureStore(t)
	capture.publishErr = store.ErrConflict
	service := New(capture)
	service.now = func() time.Time { return now }
	_, _, err := service.PublishSource(context.Background(), capture.source.ProductID, capture.source.ID, SourcePublicationInput{
		Revision: capture.source.Revision, CrawlJobID: capture.review.CrawlJob.ID,
		DocumentIDs: []string{"legacy-selected"}, AcknowledgeReviewed: true,
	}, Actor{ID: "reviewer-123", RequestID: "request-123"})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("PublishSource error = %v, want conflict", err)
	}
	events, auditErr := capture.AuditEvents(context.Background(), capture.source.OrganisationID)
	if auditErr != nil {
		t.Fatal(auditErr)
	}
	for _, event := range events {
		if event.TargetID == capture.capturedPublication.ID && event.Action == "source.publication.created" {
			t.Fatalf("failed atomic publication emitted audit event %#v", event)
		}
	}
}
