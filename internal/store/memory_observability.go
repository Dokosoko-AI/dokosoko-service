package store

import (
	"context"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"strings"
	"time"
)

func (m *Memory) LLMProfiles(_ context.Context, productID string) ([]model.LLMProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.LLMProfile, 0, len(m.llmProfiles[productID]))
	for _, value := range m.llmProfiles[productID] {
		value.Hardening = append([]byte(nil), value.Hardening...)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Role < result[j].Role })
	return result, nil
}

func (m *Memory) SaveLLMProfile(_ context.Context, value model.LLMProfile) (model.LLMProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.LLMProfile{}, ErrNotFound
	}
	if current, ok := m.llmProfiles[value.ProductID][value.Role]; ok {
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		value.Revision, value.CreatedAt = 1, time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	value.Hardening = append([]byte(nil), value.Hardening...)
	m.llmProfiles[value.ProductID][value.Role] = value
	return value, nil
}

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

func (m *Memory) AppendAnalytics(_ context.Context, event model.AnalyticsEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	event.Dimensions = cloneMap(event.Dimensions)
	m.analytics = append(m.analytics, event)
	return nil
}

func (m *Memory) LLMTokensUsed(_ context.Context, productID, role string, since time.Time) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total int64
	for _, event := range m.analytics {
		if event.ProductID == productID && event.EventName == "llm.tokens" && !event.CreatedAt.Before(since) && event.Dimensions["role"] == role {
			total += int64(event.Value)
		}
	}
	return total, nil
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
