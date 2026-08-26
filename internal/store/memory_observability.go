package store

import (
	"context"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) PublicKnowledge(_ context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	allowed := make(map[string]bool)
	for _, publicationID := range publicationIDs {
		publication, ok := m.sourcePublications[productID][publicationID]
		source, sourceOK := m.sources[productID][publication.SourceID]
		if !ok || !sourceOK || publication.Visibility != model.VisibilityPublic || source.Visibility != model.VisibilityPublic || !source.Published || source.Quarantined {
			continue
		}
		for documentID := range m.publicationDocuments[publicationID] {
			allowed[documentID] = true
		}
	}
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published || record.Visibility != model.VisibilityPublic || !allowed[record.ID] {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(record.Title+" "+record.Text), query) {
			result = append(result, record)
		}
	}
	return result, nil
}

func (m *Memory) PrivateKnowledge(_ context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	allowed := make(map[string]bool)
	for _, publicationID := range publicationIDs {
		if _, ok := m.sourcePublications[productID][publicationID]; !ok {
			continue
		}
		for documentID := range m.publicationDocuments[publicationID] {
			allowed[documentID] = true
		}
	}
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published || !allowed[record.ID] {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(record.Title+" "+record.Text), query) {
			result = append(result, record)
		}
	}
	return result, nil
}

func (m *Memory) AppendAudit(_ context.Context, event model.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return errors.New("audit event ID is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.audit {
		if current.ID == event.ID {
			return nil
		}
	}
	m.audit = append(m.audit, event)
	return nil
}

func (m *Memory) AuditEvents(_ context.Context, organisationID string) ([]model.AuditEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AuditEvent, 0)
	for _, event := range m.audit {
		if event.OrganisationID == organisationID {
			result = append(result, event)
		}
	}
	return result, nil
}
