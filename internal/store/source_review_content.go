package store

import (
	"context"
	"errors"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) SourceReviewContent(_ context.Context, productID, sourceID, crawlID, documentID string) (model.SourceReviewContent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	source, ok := m.sources[productID][sourceID]
	if !ok {
		return model.SourceReviewContent{}, ErrNotFound
	}
	var currentJob model.CrawlJob
	for _, job := range m.crawls[sourceID] {
		if job.ID == crawlID && job.ProductID == productID && job.SourceID == sourceID {
			currentJob = job
			break
		}
	}
	if currentJob.ID == "" {
		return model.SourceReviewContent{}, ErrNotFound
	}
	value := model.SourceReviewContent{}
	for _, document := range m.crawlReviewDocuments[crawlID] {
		if document.ID == documentID && document.CrawlJobID == crawlID {
			value.Document = memoryClone(document)
			break
		}
	}
	if value.Document.ID == "" {
		return model.SourceReviewContent{}, ErrNotFound
	}
	found := false
	for _, record := range m.knowledge[productID] {
		if record.ID == documentID && record.SourceID == sourceID {
			value.Body = record.Text
			found = true
			break
		}
	}
	if !found {
		return model.SourceReviewContent{}, ErrNotFound
	}
	// An upload replaces the one file belonging to this source. Its opaque
	// storage URL is provenance, not a stable identity across replacements.
	// Count the whole import, including excluded documents, before falling back
	// to this identity; ambiguous imports still require the exact path.
	singleUpload := source.Kind == "upload" && len(m.crawlReviewDocuments[crawlID]) == 1
	for _, publication := range m.sourcePublications[productID] {
		if publication.SourceID != sourceID || publication.CrawlJobID == crawlID || (value.Previous != nil && publication.Revision <= value.Previous.PublicationRevision) {
			continue
		}
		older := false
		for _, job := range m.crawls[sourceID] {
			if job.ID == publication.CrawlJobID && job.ProductID == productID {
				older = job.QueuedAt.Before(currentJob.QueuedAt) || (job.QueuedAt.Equal(currentJob.QueuedAt) && job.ID < currentJob.ID)
				break
			}
		}
		if !older {
			continue
		}
		for _, document := range m.crawlReviewDocuments[publication.CrawlJobID] {
			matches := document.CanonicalURL == value.Document.CanonicalURL || singleUpload && len(m.crawlReviewDocuments[publication.CrawlJobID]) == 1
			if !matches || document.CrawlJobID != publication.CrawlJobID || !m.publicationDocuments[publication.ID][document.ID] {
				continue
			}
			for _, record := range m.knowledge[productID] {
				if record.ID == document.ID && record.SourceID == sourceID {
					value.Previous = &model.SourceReviewPreviousContent{PublicationID: publication.ID, PublicationRevision: publication.Revision, DocumentID: document.ID, ContentHash: document.ContentHash, Body: record.Text}
					break
				}
			}
		}
	}
	return value, nil
}

func (p *Postgres) SourceReviewContent(ctx context.Context, productID, sourceID, crawlID, documentID string) (model.SourceReviewContent, error) {
	value := model.SourceReviewContent{}
	var queuedAt time.Time
	var singleUpload bool
	err := p.pool.QueryRow(ctx, `SELECT kd.id::text,cjd.crawl_job_id::text,kd.snapshot_id::text,kd.title,kd.canonical_url,
 cjd.assessment_state::text,cjd.assessment_trust_level,cjd.assessment_injection_indicators,
 'sha256:' || encode(ss.content_sha256,'hex'),cjd.changed,kd.body,cj.queued_at,
 source.kind='upload' AND (SELECT count(*) FROM crawl_job_documents member WHERE member.crawl_job_id=cj.id)=1
 FROM crawl_job_documents cjd
 JOIN crawl_jobs cj ON cj.id=cjd.crawl_job_id
 JOIN sources source ON source.id=cj.source_id AND source.product_id=cj.product_id
 JOIN knowledge_documents kd ON kd.id=cjd.knowledge_document_id
 JOIN source_snapshots ss ON ss.id=kd.snapshot_id
 WHERE cj.product_id=$1 AND cj.source_id=$2 AND cj.id=$3 AND kd.id=$4
 AND kd.product_id=$1 AND kd.source_id=$2 AND ss.product_id=$1 AND ss.source_id=$2`, productID, sourceID, crawlID, documentID).Scan(
		&value.Document.ID, &value.Document.CrawlJobID, &value.Document.SnapshotID, &value.Document.Title, &value.Document.CanonicalURL,
		&value.Document.State, &value.Document.TrustLevel, &value.Document.InjectionIndicators, &value.Document.ContentHash, &value.Document.Changed, &value.Body, &queuedAt, &singleUpload)
	if err != nil {
		return model.SourceReviewContent{}, databaseError(err)
	}
	previous := model.SourceReviewPreviousContent{}
	err = p.pool.QueryRow(ctx, `SELECT pub.id::text,pub.revision,kd.id::text,'sha256:' || encode(ss.content_sha256,'hex'),kd.body
 FROM source_publications pub
 JOIN crawl_jobs cj ON cj.id=pub.crawl_job_id AND cj.product_id=pub.product_id AND cj.source_id=pub.source_id
 JOIN source_publication_documents member ON member.source_publication_id=pub.id
 JOIN knowledge_documents kd ON kd.id=member.knowledge_document_id AND kd.product_id=pub.product_id AND kd.source_id=pub.source_id
 JOIN crawl_job_documents cjd ON cjd.crawl_job_id=cj.id AND cjd.knowledge_document_id=kd.id
 JOIN source_snapshots ss ON ss.id=kd.snapshot_id AND ss.product_id=pub.product_id AND ss.source_id=pub.source_id
 WHERE pub.product_id=$1 AND pub.source_id=$2 AND (cj.queued_at,cj.id)<($3,$4::uuid)
 AND (kd.canonical_url=$5 OR ($6 AND (SELECT count(*) FROM crawl_job_documents imported WHERE imported.crawl_job_id=cj.id)=1))
 ORDER BY pub.revision DESC,kd.id LIMIT 1`, productID, sourceID, queuedAt, crawlID, value.Document.CanonicalURL, singleUpload).Scan(
		&previous.PublicationID, &previous.PublicationRevision, &previous.DocumentID, &previous.ContentHash, &previous.Body)
	if err == nil {
		value.Previous = &previous
	} else if !errors.Is(databaseError(err), ErrNotFound) {
		return model.SourceReviewContent{}, databaseError(err)
	}
	return value, nil
}
