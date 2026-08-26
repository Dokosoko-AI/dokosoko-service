package store

import (
	"context"
	"sort"

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
