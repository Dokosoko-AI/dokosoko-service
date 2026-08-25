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

func (m *Memory) ProductVersionActivity(_ context.Context, productID, versionID string, since time.Time) (model.ProductVersionActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var value model.ProductVersionActivity
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) || event.Dimensions["product_version_id"] != versionID {
			continue
		}
		switch event.EventName {
		case "mcp.request":
			value.Requests++
		case "tool.called":
			value.ToolCalls++
		}
	}
	return value, nil
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

func (m *Memory) AnalyticsSummary(_ context.Context, productID string, since time.Time) (model.AnalyticsSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := model.AnalyticsSummary{Since: since, GeneratedAt: time.Now().UTC(), Channels: map[string]int64{"private_mcp": 0, "public_mcp": 0, "widget": 0}, Versions: map[string]int64{}, Funnel: map[string]int64{"connector_authorized": 0, "run_started": 0, "capability_resolved": 0, "credentials_issued": 0, "implementation_validated": 0, "success_reported": 0}}
	actors := map[string]bool{}
	daily := map[string]int64{}
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) {
			continue
		}
		if event.ActorPseudonym != "" {
			actors[event.ActorPseudonym] = true
		}
		switch event.EventName {
		case "mcp.request":
			value.MCPRequests++
			channel, _ := event.Dimensions["channel"].(string)
			value.Channels[channel]++
			daily[event.CreatedAt.UTC().Format("2006-01-02")]++
			if version, _ := event.Dimensions["product_version"].(string); version != "" {
				value.Versions[version]++
			}
		case "tool.called":
			value.ToolCalls++
		}
		if _, ok := value.Funnel[event.EventName]; ok {
			value.Funnel[event.EventName]++
		}
	}
	value.ActiveDevelopers = int64(len(actors))
	authorized := map[string]bool{}
	for _, token := range m.accessTokens {
		if token.ProductID == productID && !token.CreatedAt.Before(since) {
			authorized[token.Issuer+"\x00"+token.Subject] = true
		}
	}
	value.AuthorizedUsers = int64(len(authorized))
	for _, run := range m.integrationRuns[productID] {
		if run.StartedAt.Before(since) {
			continue
		}
		value.IntegrationRuns++
		if run.ValidatedSuccess != nil {
			value.ValidatedRuns++
			if *run.ValidatedSuccess {
				value.ValidatedSuccess++
			}
		}
	}
	if value.ValidatedRuns > 0 {
		value.FirstPassRate = float64(value.ValidatedSuccess) * 100 / float64(value.ValidatedRuns)
	}
	for date, count := range daily {
		value.DailyRequests = append(value.DailyRequests, model.AnalyticsPoint{Date: date, Count: count})
	}
	sort.Slice(value.DailyRequests, func(i, j int) bool { return value.DailyRequests[i].Date < value.DailyRequests[j].Date })
	return value, nil
}

func (m *Memory) RecipePopularity(_ context.Context, productID string, since time.Time) ([]model.RecipePopularity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	byID := map[string]model.RecipePopularity{}
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) || (event.EventName != "recipe.view" && event.EventName != "recipe.plan_selected") {
			continue
		}
		recipeID, _ := event.Dimensions["recipe_id"].(string)
		if recipeID == "" {
			continue
		}
		value := byID[recipeID]
		value.RecipeID = recipeID
		value.RecipeSlug, _ = event.Dimensions["recipe_slug"].(string)
		if event.EventName == "recipe.view" {
			value.Views++
		} else {
			value.PlanSelections++
		}
		byID[recipeID] = value
	}
	result := make([]model.RecipePopularity, 0, len(byID))
	for _, value := range byID {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PlanSelections == result[j].PlanSelections {
			if result[i].Views == result[j].Views {
				return result[i].RecipeSlug < result[j].RecipeSlug
			}
			return result[i].Views > result[j].Views
		}
		return result[i].PlanSelections > result[j].PlanSelections
	})
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
