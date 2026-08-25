package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) AccessDefinitions(_ context.Context, deploymentID string) ([]model.AccessDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AccessDefinition, 0)
	for _, value := range m.accessDefinitions {
		if value.DeploymentID == deploymentID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) AccessDefinition(_ context.Context, deploymentID, id string) (model.AccessDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessDefinitions[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.AccessDefinition{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateAccessDefinition(_ context.Context, value model.AccessDefinition) (model.AccessDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.accessDefinitions {
		if current.DeploymentID == value.DeploymentID && current.ServiceKey == value.ServiceKey {
			return model.AccessDefinition{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.State == "" {
		value.State = "active"
	}
	m.accessDefinitions[value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

func (m *Memory) UpdateAccessDefinition(_ context.Context, value model.AccessDefinition, expected int64) (model.AccessDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.accessDefinitions[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.AccessDefinition{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.AccessDefinition{}, ErrConflict
	}
	value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, time.Now().UTC()
	m.accessDefinitions[value.ID] = memoryClone(value)
	m.deployment.CatalogRevision++
	return memoryClone(value), nil
}

func (m *Memory) accessConnectionLocked(value model.AccessConnection) model.AccessConnection {
	if definition, ok := m.accessDefinitions[value.AccessDefinitionID]; ok {
		copy := memoryClone(definition)
		value.Definition = &copy
	}
	value.IntegrationIDs = nil
	for integrationID := range m.integrationAccessLinks[value.ID] {
		value.IntegrationIDs = append(value.IntegrationIDs, integrationID)
	}
	sort.Strings(value.IntegrationIDs)
	return cloneAccessConnection(value)
}

func (m *Memory) AccessConnections(_ context.Context, deploymentID string) ([]model.AccessConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AccessConnection, 0)
	for _, value := range m.accessConnections {
		if value.DeploymentID == deploymentID {
			result = append(result, m.accessConnectionLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) AccessConnection(_ context.Context, deploymentID, id string) (model.AccessConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessConnections[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.AccessConnection{}, ErrNotFound
	}
	return m.accessConnectionLocked(value), nil
}

func (m *Memory) CreateAccessConnection(_ context.Context, value model.AccessConnection) (model.AccessConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	definition, ok := m.accessDefinitions[value.AccessDefinitionID]
	if !ok || definition.DeploymentID != value.DeploymentID {
		return model.AccessConnection{}, ErrNotFound
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.State == "" {
		value.State = "active"
	}
	integrationIDs := append([]string(nil), value.IntegrationIDs...)
	value.IntegrationIDs, value.Definition = nil, nil
	m.accessConnections[value.ID] = cloneAccessConnection(value)
	m.integrationAccessLinks[value.ID] = make(map[string]bool)
	for _, integrationID := range integrationIDs {
		if integration, ok := m.integrations[integrationID]; ok && integration.DeploymentID == value.DeploymentID {
			m.integrationAccessLinks[value.ID][integrationID] = true
		}
	}
	return m.accessConnectionLocked(value), nil
}

func (m *Memory) SetIntegrationAccessConnections(_ context.Context, deploymentID, integrationID string, connectionIDs []string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[integrationID]
	if !ok || integration.DeploymentID != deploymentID {
		return ErrNotFound
	}
	selected := make(map[string]bool, len(connectionIDs))
	for _, connectionID := range connectionIDs {
		connection, ok := m.accessConnections[connectionID]
		if !ok || connection.DeploymentID != deploymentID {
			return ErrNotFound
		}
		selected[connectionID] = true
	}
	for connectionID, links := range m.integrationAccessLinks {
		if selected[connectionID] {
			links[integrationID] = true
		} else {
			delete(links, integrationID)
		}
	}
	m.deployment.CatalogRevision++
	return nil
}

func (m *Memory) accessInstanceLocked(value model.AccessInstance) model.AccessInstance {
	value.IntegrationIDs = nil
	for integrationID := range m.instanceIntegrationLinks[value.ID] {
		value.IntegrationIDs = append(value.IntegrationIDs, integrationID)
	}
	sort.Strings(value.IntegrationIDs)
	return memoryClone(value)
}

func (m *Memory) AccessInstances(_ context.Context, deploymentID, connectionID string) ([]model.AccessInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AccessInstance, 0)
	for _, value := range m.accessInstances {
		if value.DeploymentID == deploymentID && (connectionID == "" || value.AccessConnectionID == connectionID) {
			result = append(result, m.accessInstanceLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) AccessInstance(_ context.Context, deploymentID, id string) (model.AccessInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessInstances[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.AccessInstance{}, ErrNotFound
	}
	return m.accessInstanceLocked(value), nil
}

func (m *Memory) CreateAccessInstance(_ context.Context, value model.AccessInstance) (model.AccessInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	connection, ok := m.accessConnections[value.AccessConnectionID]
	if !ok || connection.DeploymentID != value.DeploymentID {
		return model.AccessInstance{}, ErrNotFound
	}
	for _, current := range m.accessInstances {
		if current.AccessConnectionID == value.AccessConnectionID && (current.IdempotencyKey == value.IdempotencyKey || current.ExternalID == value.ExternalID) {
			return m.accessInstanceLocked(current), nil
		}
	}
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	if value.State == "" {
		value.State = "active"
	}
	integrationIDs := append([]string(nil), value.IntegrationIDs...)
	value.IntegrationIDs = nil
	m.accessInstances[value.ID] = memoryClone(value)
	m.instanceIntegrationLinks[value.ID] = make(map[string]bool)
	for _, integrationID := range integrationIDs {
		if m.integrationAccessLinks[value.AccessConnectionID][integrationID] {
			m.instanceIntegrationLinks[value.ID][integrationID] = true
		}
	}
	return m.accessInstanceLocked(value), nil
}

func (m *Memory) AccessCredentials(_ context.Context, deploymentID, connectionID, instanceID string) ([]model.AccessCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AccessCredential, 0)
	for _, value := range m.accessCredentials {
		if value.DeploymentID == deploymentID && (connectionID == "" || value.AccessConnectionID == connectionID) && (instanceID == "" || value.AccessInstanceID == instanceID) {
			result = append(result, cloneAccessCredential(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) AccessCredential(_ context.Context, deploymentID, id string) (model.AccessCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessCredentials[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.AccessCredential{}, ErrNotFound
	}
	return cloneAccessCredential(value), nil
}

func (m *Memory) CreateAccessCredential(_ context.Context, value model.AccessCredential) (model.AccessCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	connection, ok := m.accessConnections[value.AccessConnectionID]
	if !ok || connection.DeploymentID != value.DeploymentID {
		return model.AccessCredential{}, ErrNotFound
	}
	for _, current := range m.accessCredentials {
		if current.AccessConnectionID == value.AccessConnectionID && value.IdempotencyKey != "" && current.IdempotencyKey == value.IdempotencyKey {
			return cloneAccessCredential(current), nil
		}
	}
	if value.State == "" {
		value.State = "active"
	}
	value.CreatedAt = time.Now().UTC()
	m.accessCredentials[value.ID] = cloneAccessCredential(value)
	return cloneAccessCredential(value), nil
}

func (m *Memory) RevokeAccessCredential(_ context.Context, deploymentID, id string, revokedAt time.Time) (model.AccessCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.accessCredentials[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.AccessCredential{}, ErrNotFound
	}
	if value.RevokedAt != nil {
		return cloneAccessCredential(value), nil
	}
	value.State, value.RevokedAt = "revoked", &revokedAt
	m.accessCredentials[id] = value
	return cloneAccessCredential(value), nil
}
