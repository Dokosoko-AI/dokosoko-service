package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) GrantDefinitions(_ context.Context, deploymentID string) ([]model.GrantDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != deploymentID {
		return nil, ErrNotFound
	}
	values := make([]model.GrantDefinition, 0)
	for _, value := range m.grantDefinitions {
		if value.DeploymentID == deploymentID {
			values = append(values, memoryClone(value))
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values, nil
}

func (m *Memory) GrantDefinition(_ context.Context, deploymentID, id string) (model.GrantDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.grantDefinitions[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.GrantDefinition{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveGrantDefinition(_ context.Context, value model.GrantDefinition, expected int64) (model.GrantDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.DeploymentID {
		return model.GrantDefinition{}, ErrNotFound
	}
	for id, current := range m.grantDefinitions {
		if id != value.ID && current.DeploymentID == value.DeploymentID && current.Key == value.Key {
			return model.GrantDefinition{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := m.grantDefinitions[value.ID]
	if !exists {
		if expected != 0 {
			return model.GrantDefinition{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	} else {
		if expected == 0 || current.Revision != expected {
			return model.GrantDefinition{}, ErrConflict
		}
		value.Key, value.Revision = current.Key, current.Revision+1
		value.CreatedAt, value.UpdatedAt = current.CreatedAt, now
	}
	m.grantDefinitions[value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

func (m *Memory) AuthorizationPoints(_ context.Context, integrationID string) ([]model.AuthorizationPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.AuthorizationPoint, 0, len(m.authorizationPoints[integrationID]))
	for _, value := range m.authorizationPoints[integrationID] {
		values = append(values, memoryClone(value))
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values, nil
}

func (m *Memory) AuthorizationPoint(_ context.Context, integrationID, id string) (model.AuthorizationPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.authorizationPoints[integrationID][id]
	if !ok {
		return model.AuthorizationPoint{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveAuthorizationPoint(_ context.Context, value model.AuthorizationPoint, expected int64) (model.AuthorizationPoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.integrations[value.IntegrationID]; !ok {
		return model.AuthorizationPoint{}, ErrNotFound
	}
	if m.authorizationPoints[value.IntegrationID] == nil {
		m.authorizationPoints[value.IntegrationID] = make(map[string]model.AuthorizationPoint)
	}
	for id, current := range m.authorizationPoints[value.IntegrationID] {
		if id != value.ID && current.Key == value.Key {
			return model.AuthorizationPoint{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := m.authorizationPoints[value.IntegrationID][value.ID]
	if !exists {
		if expected != 0 {
			return model.AuthorizationPoint{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	} else {
		if expected == 0 || current.Revision != expected {
			return model.AuthorizationPoint{}, ErrConflict
		}
		value.Key, value.Revision = current.Key, current.Revision+1
		value.CreatedAt, value.UpdatedAt = current.CreatedAt, now
	}
	m.authorizationPoints[value.IntegrationID][value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

func (m *Memory) IntegrationToolBindings(_ context.Context, integrationID string) ([]model.IntegrationToolBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.IntegrationToolBinding, 0, len(m.integrationToolLinks[integrationID]))
	for _, value := range m.integrationToolLinks[integrationID] {
		value.Tool, value.AuthorizationPoint = nil, nil
		for _, productTools := range m.tools {
			if tool, ok := productTools[value.ToolID]; ok {
				copy := cloneTool(tool)
				value.Tool = &copy
				break
			}
		}
		if point, ok := m.authorizationPoints[integrationID][value.AuthorizationPointID]; ok {
			copy := memoryClone(point)
			value.AuthorizationPoint = &copy
		}
		cloned := memoryClone(value)
		if value.Tool != nil {
			tool := cloneTool(*value.Tool)
			cloned.Tool = &tool
		}
		if value.AuthorizationPoint != nil {
			point := memoryClone(*value.AuthorizationPoint)
			cloned.AuthorizationPoint = &point
		}
		values = append(values, cloned)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ToolID < values[j].ToolID })
	return values, nil
}

func (m *Memory) SaveIntegrationToolBindings(ctx context.Context, integrationID string, values []model.IntegrationToolBinding) ([]model.IntegrationToolBinding, error) {
	m.mu.Lock()
	if _, ok := m.integrations[integrationID]; !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	links := make(map[string]model.IntegrationToolBinding, len(values))
	for _, value := range values {
		value.IntegrationID, value.Tool, value.AuthorizationPoint = integrationID, nil, nil
		value.CreatedAt = now
		links[value.ToolID] = memoryClone(value)
	}
	m.integrationToolLinks[integrationID] = links
	m.mu.Unlock()
	return m.IntegrationToolBindings(ctx, integrationID)
}

func (m *Memory) UpdateTool(_ context.Context, value model.Tool, expected int64) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tools[value.ProductID][value.ID]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if current.Revision != expected || current.State != "draft" {
		return model.Tool{}, ErrConflict
	}
	value.State, value.Revision = current.State, current.Revision+1
	value.CreatedAt, value.UpdatedAt = current.CreatedAt, time.Now().UTC()
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) RetireTool(_ context.Context, productID, id string, expected int64) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tools[productID][id]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if current.State == "retired" {
		return cloneTool(current), nil
	}
	if current.Revision != expected {
		return model.Tool{}, ErrConflict
	}
	current.State, current.Revision, current.UpdatedAt = "retired", current.Revision+1, time.Now().UTC()
	m.tools[productID][id] = cloneTool(current)
	return cloneTool(current), nil
}
