package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// RelevantPrivateKnowledge searches only documents pinned by the supplied
// reviewed publications. It deliberately excludes draft or quarantined
// sources even if a stale in-memory publication link remains.
func (m *Memory) RelevantPrivateKnowledge(ctx context.Context, productID string, publicationIDs []string, outcome string, limit int) ([]model.KnowledgeRecord, error) {
	limit = boundedRelevantKnowledgeLimit(limit)
	if limit == 0 || len(relevantKnowledgeTerms(outcome)) == 0 {
		return []model.KnowledgeRecord{}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	allowed := make(map[string]map[string]bool)
	for _, publicationID := range publicationIDs {
		publication, exists := m.sourcePublications[productID][publicationID]
		if !exists || publication.ProductID != productID {
			continue
		}
		source, exists := m.sources[productID][publication.SourceID]
		if !exists || source.ProductID != productID || !source.Published || source.Quarantined {
			continue
		}
		for documentID := range m.publicationDocuments[publicationID] {
			if allowed[documentID] == nil {
				allowed[documentID] = make(map[string]bool)
			}
			allowed[documentID][publication.SourceID] = true
		}
	}

	eligible := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if record.ProductID != productID || !record.Published || !allowed[record.ID][record.SourceID] {
			continue
		}
		eligible = append(eligible, record)
	}
	return rankRelevantKnowledge(eligible, boundedRelevantKnowledgeQuery(outcome), limit), nil
}
