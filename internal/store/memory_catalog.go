package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func memoryClone[T any](value T) T {
	encoded, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func (m *Memory) Deployment(context.Context) (model.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment {
		return model.Deployment{}, ErrNotFound
	}
	return m.deployment, nil
}

func (m *Memory) CreateDeployment(_ context.Context, value model.Deployment) (model.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasDeployment {
		return model.Deployment{}, ErrConflict
	}
	if _, ok := m.orgs[value.OrganisationID]; !ok {
		return model.Deployment{}, ErrNotFound
	}
	now := time.Now().UTC()
	value.Revision, value.CatalogRevision = 1, 1
	value.CreatedAt, value.UpdatedAt = now, now
	m.deployment, m.hasDeployment = value, true
	product := model.Product{ID: value.ID, OrganisationID: value.OrganisationID, Name: value.Name, Slug: value.Slug, Description: value.Description, CatalogRevision: value.CatalogRevision, PublicMCPEnabled: value.PublicMCPEnabled, Revision: value.Revision, CreatedAt: now, UpdatedAt: now}
	m.products[value.ID] = product
	m.sources[value.ID] = make(map[string]model.Source)
	m.knowledge[value.ID] = nil
	m.envs[value.ID] = make(map[string]model.Environment)
	m.tools[value.ID] = make(map[string]model.Tool)
	m.mcpConnections[value.ID] = make(map[string]model.MCPConnection)
	m.reportSubmissions[value.ID] = make(map[string]model.ReportSubmission)
	m.aiProviderConnections[value.ID] = make(map[string]model.AIProviderConnection)
	m.aiWorkloadProfiles[value.ID] = make(map[string]model.AIWorkloadProfile)
	m.aiPromptStates[value.ID] = make(map[string]model.AIPromptState)
	return value, nil
}

func (m *Memory) UpdateDeployment(_ context.Context, value model.Deployment, expected int64) (model.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.ID {
		return model.Deployment{}, ErrNotFound
	}
	if m.deployment.Revision != expected {
		return model.Deployment{}, ErrConflict
	}
	value.CreatedAt = m.deployment.CreatedAt
	value.Revision = expected + 1
	value.UpdatedAt = time.Now().UTC()
	m.deployment = value
	if product, ok := m.products[value.ID]; ok {
		product.Name, product.Slug, product.Description = value.Name, value.Slug, value.Description
		product.CatalogRevision, product.PublicMCPEnabled = value.CatalogRevision, value.PublicMCPEnabled
		product.Revision, product.UpdatedAt = value.Revision, value.UpdatedAt
		m.products[value.ID] = product
	}
	return value, nil
}

func (m *Memory) integrationLocked(value model.Integration) model.Integration {
	value.Resources = nil
	for _, link := range m.integrationResourceLinks[value.ID] {
		value.Resources = append(value.Resources, m.integrationResourceLinkLocked(link))
	}
	sort.Slice(value.Resources, func(i, j int) bool {
		if value.Resources[i].Kind == value.Resources[j].Kind {
			return value.Resources[i].Name < value.Resources[j].Name
		}
		return value.Resources[i].Kind < value.Resources[j].Kind
	})
	value.SDKs = nil
	for _, reference := range m.sdkReferences[value.ID] {
		value.SDKs = append(value.SDKs, memoryClone(reference))
	}
	sort.Slice(value.SDKs, func(i, j int) bool {
		if value.SDKs[i].Ecosystem == value.SDKs[j].Ecosystem {
			return value.SDKs[i].Coordinate < value.SDKs[j].Coordinate
		}
		return value.SDKs[i].Ecosystem < value.SDKs[j].Ecosystem
	})
	return memoryClone(value)
}

func (m *Memory) Integrations(_ context.Context, deploymentID string) ([]model.Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.Integration, 0)
	for _, value := range m.integrations {
		if value.DeploymentID == deploymentID {
			result = append(result, m.integrationLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DisplayName == result[j].DisplayName {
			return result[i].VersionKey < result[j].VersionKey
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, nil
}

func (m *Memory) Integration(_ context.Context, deploymentID, id string) (model.Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.integrations[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.Integration{}, ErrNotFound
	}
	return m.integrationLocked(value), nil
}

func (m *Memory) CreateIntegration(_ context.Context, value model.Integration) (model.Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return model.Integration{}, ErrNotFound
	}
	for _, current := range m.integrations {
		if current.DeploymentID == value.DeploymentID && current.FamilyKey == value.FamilyKey && current.VersionKey == value.VersionKey {
			return model.Integration{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.Lifecycle == "" {
		value.Lifecycle = "draft"
	}
	if value.Visibility == "" {
		value.Visibility = model.VisibilityPrivate
	}
	m.integrations[value.ID] = value
	m.integrationResourceLinks[value.ID] = make(map[string]model.IntegrationResourceLink)
	m.sdkReferences[value.ID] = make(map[string]model.SDKReference)
	m.integrationRevisions[value.ID] = make(map[string]model.IntegrationRevision)
	m.deployment.CatalogRevision++
	return m.integrationLocked(value), nil
}

func (m *Memory) UpdateIntegration(_ context.Context, value model.Integration, expected int64) (model.Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.integrations[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.Integration{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Integration{}, ErrConflict
	}
	for id, candidate := range m.integrations {
		if id != value.ID && candidate.DeploymentID == value.DeploymentID && candidate.FamilyKey == value.FamilyKey && candidate.VersionKey == value.VersionKey {
			return model.Integration{}, ErrConflict
		}
	}
	value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, time.Now().UTC()
	m.integrations[value.ID] = value
	m.deployment.CatalogRevision++
	return m.integrationLocked(value), nil
}

func (m *Memory) IntegrationRevisions(_ context.Context, integrationID string) ([]model.IntegrationRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationRevision, 0, len(m.integrationRevisions[integrationID]))
	for _, value := range m.integrationRevisions[integrationID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) CreateIntegrationRevision(_ context.Context, value model.IntegrationRevision) (model.IntegrationRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.IntegrationID]
	if !ok {
		return model.IntegrationRevision{}, ErrNotFound
	}
	for _, current := range m.integrationRevisions[value.IntegrationID] {
		if current.Revision == value.Revision || current.ManifestHash == value.ManifestHash {
			return model.IntegrationRevision{}, ErrConflict
		}
	}
	value.CreatedAt = time.Now().UTC()
	m.integrationRevisions[value.IntegrationID][value.ID] = memoryClone(value)
	if m.hasDeployment && m.deployment.ID == integration.DeploymentID {
		m.deployment.CatalogRevision++
		if product, exists := m.products[integration.DeploymentID]; exists {
			product.CatalogRevision, product.UpdatedAt = m.deployment.CatalogRevision, value.CreatedAt
			m.products[integration.DeploymentID] = product
		}
	}
	return memoryClone(value), nil
}
