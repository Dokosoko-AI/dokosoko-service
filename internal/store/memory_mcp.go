package store

import (
	"context"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"time"
)

func (m *Memory) MCPConnections(_ context.Context, productID string) ([]model.MCPConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.mcpConnections[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.MCPConnection, 0, len(values))
	for _, value := range values {
		result = append(result, cloneMCPConnection(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) MCPConnection(_ context.Context, productID, id string) (model.MCPConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.mcpConnections[productID][id]
	if !ok {
		return model.MCPConnection{}, ErrNotFound
	}
	return cloneMCPConnection(value), nil
}

func (m *Memory) CreateMCPConnection(_ context.Context, value model.MCPConnection) (model.MCPConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.MCPConnection{}, ErrNotFound
	}
	if m.mcpConnections[value.ProductID] == nil {
		m.mcpConnections[value.ProductID] = make(map[string]model.MCPConnection)
	}
	for _, current := range m.mcpConnections[value.ProductID] {
		if current.Namespace == value.Namespace || current.Name == value.Name || current.Endpoint == value.Endpoint {
			return model.MCPConnection{}, ErrConflict
		}
	}
	value.ProtocolVersion = model.StatelessMCPv2Protocol
	value.State = "active"
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.mcpConnections[value.ProductID][value.ID] = cloneMCPConnection(value)
	return cloneMCPConnection(value), nil
}

func (m *Memory) UpdateMCPConnectionSync(_ context.Context, productID, id, catalogHash string, syncedAt time.Time) (model.MCPConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.mcpConnections[productID][id]
	if !ok {
		return model.MCPConnection{}, ErrNotFound
	}
	value.LastCatalogHash = catalogHash
	value.LastSyncedAt = &syncedAt
	value.Revision++
	value.UpdatedAt = syncedAt
	m.mcpConnections[productID][id] = cloneMCPConnection(value)
	return cloneMCPConnection(value), nil
}
