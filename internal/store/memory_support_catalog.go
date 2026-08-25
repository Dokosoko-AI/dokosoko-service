package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) BackendConnections(_ context.Context, deploymentID string) ([]model.BackendConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.BackendConnection, 0)
	for _, value := range m.backendConnections {
		if value.DeploymentID == deploymentID {
			result = append(result, cloneBackendConnection(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) BackendConnection(_ context.Context, deploymentID, id string) (model.BackendConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.backendConnections[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.BackendConnection{}, ErrNotFound
	}
	return cloneBackendConnection(value), nil
}

func (m *Memory) CreateBackendConnection(_ context.Context, value model.BackendConnection) (model.BackendConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return model.BackendConnection{}, ErrNotFound
	}
	for _, current := range m.backendConnections {
		if current.DeploymentID == value.DeploymentID && current.Name == value.Name {
			return model.BackendConnection{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	m.backendConnections[value.ID] = cloneBackendConnection(value)
	return cloneBackendConnection(value), nil
}

func (m *Memory) UpdateBackendConnection(_ context.Context, value model.BackendConnection, expected int64) (model.BackendConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.backendConnections[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.BackendConnection{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.BackendConnection{}, ErrConflict
	}
	for id, candidate := range m.backendConnections {
		if id != value.ID && candidate.DeploymentID == value.DeploymentID && candidate.Name == value.Name {
			return model.BackendConnection{}, ErrConflict
		}
	}
	value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, time.Now().UTC()
	m.backendConnections[value.ID] = cloneBackendConnection(value)
	return cloneBackendConnection(value), nil
}

func (m *Memory) supportRouteLocked(value model.SupportRoute) model.SupportRoute {
	value.IntegrationIDs = nil
	for integrationID, routeID := range m.integrationSupportRoutes {
		if routeID == value.ID {
			value.IntegrationIDs = append(value.IntegrationIDs, integrationID)
		}
	}
	sort.Strings(value.IntegrationIDs)
	return value
}

func (m *Memory) SupportRoutes(_ context.Context, deploymentID string) ([]model.SupportRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.SupportRoute, 0)
	for _, value := range m.supportRoutes {
		if value.DeploymentID == deploymentID {
			result = append(result, m.supportRouteLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) SupportRoute(_ context.Context, deploymentID, id string) (model.SupportRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.supportRoutes[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.SupportRoute{}, ErrNotFound
	}
	return m.supportRouteLocked(value), nil
}

func (m *Memory) SupportRouteForIntegration(_ context.Context, deploymentID, integrationID string) (model.SupportRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if routeID := m.integrationSupportRoutes[integrationID]; routeID != "" {
		if value, ok := m.supportRoutes[routeID]; ok && value.DeploymentID == deploymentID && value.State == "active" {
			return m.supportRouteLocked(value), nil
		}
		return model.SupportRoute{}, ErrNotFound
	}
	for _, value := range m.supportRoutes {
		if value.DeploymentID == deploymentID && value.IsDefault && value.State == "active" {
			return m.supportRouteLocked(value), nil
		}
	}
	return model.SupportRoute{}, ErrNotFound
}

func (m *Memory) SaveSupportRoute(_ context.Context, value model.SupportRoute, expected int64) (model.SupportRoute, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	current, exists := m.supportRoutes[value.ID]
	if exists {
		if current.DeploymentID != value.DeploymentID || current.Revision != expected {
			return model.SupportRoute{}, ErrConflict
		}
		value.CreatedAt, value.Revision = current.CreatedAt, expected+1
	} else {
		if expected != 0 {
			return model.SupportRoute{}, ErrConflict
		}
		value.CreatedAt, value.Revision = now, 1
	}
	if value.State == "" {
		value.State = "active"
	}
	if value.IsDefault {
		for id, route := range m.supportRoutes {
			if id != value.ID && route.DeploymentID == value.DeploymentID && route.IsDefault {
				route.IsDefault = false
				m.supportRoutes[id] = route
			}
		}
	}
	integrationIDs := append([]string(nil), value.IntegrationIDs...)
	value.IntegrationIDs, value.UpdatedAt = nil, now
	m.supportRoutes[value.ID] = value
	for integrationID, routeID := range m.integrationSupportRoutes {
		if routeID == value.ID {
			delete(m.integrationSupportRoutes, integrationID)
		}
	}
	for _, integrationID := range integrationIDs {
		if integration, ok := m.integrations[integrationID]; ok && integration.DeploymentID == value.DeploymentID {
			m.integrationSupportRoutes[integrationID] = value.ID
		}
	}
	return m.supportRouteLocked(value), nil
}

func (m *Memory) SetIntegrationSupportRoute(_ context.Context, deploymentID, integrationID, routeID, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[integrationID]
	if !ok || integration.DeploymentID != deploymentID {
		return ErrNotFound
	}
	if routeID == "" {
		delete(m.integrationSupportRoutes, integrationID)
		m.deployment.CatalogRevision++
		return nil
	}
	route, ok := m.supportRoutes[routeID]
	if !ok || route.DeploymentID != deploymentID || route.State != "active" {
		return ErrNotFound
	}
	m.integrationSupportRoutes[integrationID] = routeID
	m.deployment.CatalogRevision++
	return nil
}
