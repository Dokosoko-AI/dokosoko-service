package store

import (
	"context"
	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"time"
)

func sortedSources(values map[string]model.Source) []model.Source {
	result := make([]model.Source, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (m *Memory) Sources(_ context.Context, productID string) ([]model.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.sources[productID]
	if !ok {
		return nil, ErrNotFound
	}
	return sortedSources(values), nil
}

func (m *Memory) Source(_ context.Context, productID, id string) (model.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sources[productID][id]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateSource(_ context.Context, value model.Source) (model.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Source{}, ErrNotFound
	}
	value.Visibility = model.VisibilityPrivate
	value.Published = false
	value.Quarantined = false
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.sources[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) UpdateSource(_ context.Context, value model.Source, expected int64) (model.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.sources[value.ProductID][value.ID]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Source{}, ErrConflict
	}
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.sources[value.ProductID][value.ID] = value
	for i := range m.knowledge[value.ProductID] {
		if m.knowledge[value.ProductID][i].SourceID == value.ID {
			m.knowledge[value.ProductID][i].Visibility = value.Visibility
		}
	}
	return value, nil
}

func (m *Memory) SourceReview(_ context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	source, ok := m.sources[productID][sourceID]
	if !ok {
		return model.SourceReview{}, ErrNotFound
	}
	jobs := append([]model.CrawlJob(nil), m.crawls[sourceID]...)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].QueuedAt.After(jobs[j].QueuedAt) || (jobs[i].QueuedAt.Equal(jobs[j].QueuedAt) && jobs[i].ID > jobs[j].ID)
	})
	var job model.CrawlJob
	for _, candidate := range jobs {
		if crawlJobID == "" || candidate.ID == crawlJobID {
			job = candidate
			break
		}
	}
	if job.ID == "" {
		return model.SourceReview{}, ErrNotFound
	}
	review := model.SourceReview{Source: source, CrawlJob: job, Documents: append([]model.CrawlReviewDocument(nil), m.crawlReviewDocuments[job.ID]...)}
	for _, publication := range m.sourcePublications[productID] {
		if publication.SourceID == sourceID && publication.CrawlJobID == job.ID {
			copy := publication
			review.Publication = &copy
			break
		}
	}
	return review, nil
}

func (m *Memory) SourcePublications(_ context.Context, productID, sourceID string) ([]model.SourcePublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.sources[productID][sourceID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.SourcePublication, 0)
	for _, value := range m.sourcePublications[productID] {
		if value.SourceID == sourceID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Revision > values[j].Revision })
	return values, nil
}

func (m *Memory) SourcePublication(_ context.Context, productID, publicationID string) (model.SourcePublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sourcePublications[productID][publicationID]
	if !ok {
		return model.SourcePublication{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) PublishSource(_ context.Context, productID, sourceID string, expected int64, publication model.SourcePublication, documentIDs []string) (model.Source, model.SourcePublication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.sources[productID][sourceID]
	if !ok {
		return model.Source{}, model.SourcePublication{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if value.Quarantined {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	jobs := append([]model.CrawlJob(nil), m.crawls[sourceID]...)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].QueuedAt.After(jobs[j].QueuedAt) || (jobs[i].QueuedAt.Equal(jobs[j].QueuedAt) && jobs[i].ID > jobs[j].ID)
	})
	if len(jobs) == 0 || jobs[0].ID != publication.CrawlJobID || jobs[0].FinishedAt == nil || (jobs[0].State != "review" && jobs[0].State != "succeeded") || jobs[0].FetchedCount == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	reviewDocs := make(map[string]model.CrawlReviewDocument, len(m.crawlReviewDocuments[publication.CrawlJobID]))
	for _, document := range m.crawlReviewDocuments[publication.CrawlJobID] {
		reviewDocs[document.ID] = document
	}
	selected := make([]model.CrawlReviewDocument, 0, len(documentIDs))
	seen := make(map[string]bool, len(documentIDs))
	for _, documentID := range documentIDs {
		document, exists := reviewDocs[documentID]
		if seen[documentID] || !exists || !docreview.SafeAssessment(document.State, document.InjectionIndicators) {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
		seen[documentID] = true
		selected = append(selected, document)
	}
	if len(documentIDs) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	lockedHash, err := docreview.PublicationContentHash(selected)
	if err != nil || publication.ContentHash != lockedHash {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	for _, existing := range m.sourcePublications[productID] {
		if existing.SourceID == sourceID && existing.CrawlJobID == publication.CrawlJobID {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
	}
	value.Published = true
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.sources[productID][sourceID] = value
	publication.OrganisationID = value.OrganisationID
	publication.ProductID = productID
	publication.SourceID = sourceID
	publication.Visibility = value.Visibility
	publication.DocumentCount = len(documentIDs)
	publication.Revision = 1
	for _, existing := range m.sourcePublications[productID] {
		if existing.SourceID == sourceID && existing.Revision >= publication.Revision {
			publication.Revision = existing.Revision + 1
		}
	}
	if m.sourcePublications[productID] == nil {
		m.sourcePublications[productID] = make(map[string]model.SourcePublication)
	}
	m.sourcePublications[productID][publication.ID] = publication
	m.publicationDocuments[publication.ID] = make(map[string]bool, len(documentIDs))
	for _, documentID := range documentIDs {
		m.publicationDocuments[publication.ID][documentID] = true
	}
	for index := range m.knowledge[productID] {
		if m.knowledge[productID][index].SourceID == sourceID && m.publicationDocuments[publication.ID][m.knowledge[productID][index].ID] {
			m.knowledge[productID][index].Published = true
			m.knowledge[productID][index].Visibility = value.Visibility
		}
	}
	return value, publication, nil
}

func (m *Memory) CrawlJobs(_ context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.sources[productID][sourceID]; !ok {
		return nil, ErrNotFound
	}
	values := m.crawls[sourceID]
	result := make([]model.CrawlJob, len(values))
	copy(result, values)
	sort.Slice(result, func(i, j int) bool {
		return result[i].QueuedAt.After(result[j].QueuedAt) || (result[i].QueuedAt.Equal(result[j].QueuedAt) && result[i].ID > result[j].ID)
	})
	return result, nil
}

func (m *Memory) CreateCrawlJob(_ context.Context, value model.CrawlJob) (model.CrawlJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[value.ProductID][value.SourceID]; !ok {
		return model.CrawlJob{}, ErrNotFound
	}
	for _, current := range m.crawls[value.SourceID] {
		if current.State == "queued" || current.State == "running" {
			return model.CrawlJob{}, ErrConflict
		}
	}
	value.State = "queued"
	value.Attempt = 1
	value.QueuedAt = time.Now().UTC()
	m.crawls[value.SourceID] = append(m.crawls[value.SourceID], value)
	return value, nil
}

func (m *Memory) CreateSecret(_ context.Context, value model.Secret) (model.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.secrets {
		if current.OrganisationID == value.OrganisationID && current.Name == value.Name {
			return model.Secret{}, ErrConflict
		}
	}
	value.CreatedAt = time.Now().UTC()
	m.secrets[value.ID] = cloneSecret(value)
	return cloneSecret(value), nil
}

func (m *Memory) Secret(_ context.Context, organisationID, id string) (model.Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.secrets[id]
	if !ok || value.OrganisationID != organisationID {
		return model.Secret{}, ErrNotFound
	}
	return cloneSecret(value), nil
}

func (m *Memory) DeleteSecret(_ context.Context, organisationID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.secrets[id]
	if !ok || value.OrganisationID != organisationID {
		return ErrNotFound
	}
	delete(m.secrets, id)
	return nil
}
