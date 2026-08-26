package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) Organisations(_ context.Context) ([]model.Organisation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Organisation, 0, len(m.orgs))
	for _, value := range m.orgs {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateOrganisation(_ context.Context, value model.Organisation) (model.Organisation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.orgs {
		if current.Slug == value.Slug {
			return model.Organisation{}, ErrConflict
		}
	}
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.orgs[value.ID] = value
	return value, nil
}

func (m *Memory) Products(_ context.Context, organisationID string) ([]model.Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Product, 0)
	for _, value := range m.products {
		if value.OrganisationID == organisationID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateProduct(_ context.Context, value model.Product) (model.Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orgs[value.OrganisationID]; !ok {
		return model.Product{}, ErrNotFound
	}
	for _, current := range m.products {
		if current.OrganisationID == value.OrganisationID && current.Slug == value.Slug {
			return model.Product{}, ErrConflict
		}
	}
	value.Revision = 1
	if value.CatalogRevision == 0 {
		value.CatalogRevision = 1
	}
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.products[value.ID] = value
	m.sources[value.ID] = make(map[string]model.Source)
	m.knowledge[value.ID] = nil
	m.envs[value.ID] = make(map[string]model.Environment)
	m.tools[value.ID] = make(map[string]model.Tool)
	m.mcpConnections[value.ID] = make(map[string]model.MCPConnection)
	m.reportSubmissions[value.ID] = make(map[string]model.ReportSubmission)
	m.aiProviderConnections[value.ID] = make(map[string]model.AIProviderConnection)
	m.aiWorkloadProfiles[value.ID] = make(map[string]model.AIWorkloadProfile)
	m.aiPromptStates[value.ID] = make(map[string]model.AIPromptState)
	m.integrationAnalyses[value.ID] = make(map[string]model.IntegrationAnalysis)
	m.recipes[value.ID] = make(map[string]model.Recipe)
	return value, nil
}

func (m *Memory) Environments(_ context.Context, productID string) ([]model.Environment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.envs[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.Environment, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateEnvironment(_ context.Context, value model.Environment) (model.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Environment{}, ErrNotFound
	}
	for _, current := range m.envs[value.ProductID] {
		if current.Slug == value.Slug || (value.IsProduction && current.IsProduction) {
			return model.Environment{}, ErrConflict
		}
	}
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.envs[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) Product(_ context.Context, id string) (model.Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.products[id]
	if !ok {
		return model.Product{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) UpdateProduct(_ context.Context, value model.Product, expected int64) (model.Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.products[value.ID]
	if !ok {
		return model.Product{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Product{}, ErrConflict
	}
	value.Revision = current.Revision + 1
	value.CatalogRevision = current.CatalogRevision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.products[value.ID] = value
	return value, nil
}

func (m *Memory) BumpProductCatalogRevision(_ context.Context, productID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bumpProductCatalogRevisionLocked(productID, time.Now().UTC())
}

func (m *Memory) bumpProductCatalogRevisionLocked(productID string, now time.Time) (int64, error) {
	value, ok := m.products[productID]
	if !ok {
		return 0, ErrNotFound
	}
	value.CatalogRevision++
	value.UpdatedAt = now
	m.products[productID] = value
	return value.CatalogRevision, nil
}
