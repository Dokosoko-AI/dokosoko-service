package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) SDKReferences(_ context.Context, integrationID string) ([]model.SDKReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.SDKReference, 0, len(m.sdkReferences[integrationID]))
	for _, value := range m.sdkReferences[integrationID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ecosystem == result[j].Ecosystem {
			return result[i].Coordinate < result[j].Coordinate
		}
		return result[i].Ecosystem < result[j].Ecosystem
	})
	return result, nil
}

func (m *Memory) SDKReference(_ context.Context, integrationID, id string) (model.SDKReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sdkReferences[integrationID][id]
	if !ok {
		return model.SDKReference{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveSDKReference(_ context.Context, value model.SDKReference, expected int64) (model.SDKReference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.IntegrationID]
	if !ok || integration.DeploymentID != value.DeploymentID || integration.OrganisationID != value.OrganisationID {
		return model.SDKReference{}, ErrNotFound
	}
	values := m.sdkReferences[value.IntegrationID]
	if values == nil {
		values = make(map[string]model.SDKReference)
		m.sdkReferences[value.IntegrationID] = values
	}
	for id, current := range values {
		if id != value.ID && current.Ecosystem == value.Ecosystem && current.Coordinate == value.Coordinate {
			return model.SDKReference{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	if current, exists := values[value.ID]; exists {
		if current.Revision != expected {
			return model.SDKReference{}, ErrConflict
		}
		value.CreatedAt = current.CreatedAt
		value.Revision = expected + 1
	} else {
		if expected != 0 {
			return model.SDKReference{}, ErrConflict
		}
		value.CreatedAt = now
		value.Revision = 1
	}
	value.UpdatedAt = now
	values[value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

func (m *Memory) DeleteSDKReference(_ context.Context, integrationID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sdkReferences[integrationID][id]; !ok {
		return ErrNotFound
	}
	delete(m.sdkReferences[integrationID], id)
	return nil
}
