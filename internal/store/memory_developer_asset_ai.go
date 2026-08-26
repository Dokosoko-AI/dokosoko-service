package store

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func developerAssetAIInputKey(deploymentID, promptKey, inputHash string) string {
	return deploymentID + "\x00" + promptKey + "\x00" + inputHash
}

func (m *Memory) DeveloperAssetAIAdvisoryRuns(_ context.Context, query DeveloperAssetAIAdvisoryQuery) ([]model.DeveloperAssetAIAdvisoryRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != query.DeploymentID {
		return nil, ErrNotFound
	}
	limit := query.Limit
	if limit < 1 || limit > 200 {
		limit = 100
	}
	result := make([]model.DeveloperAssetAIAdvisoryRun, 0)
	for _, value := range m.developerAssets.aiAdvisoryRuns {
		if value.DeploymentID != query.DeploymentID || query.PromptKey != "" && value.PromptKey != query.PromptKey ||
			query.ScopeID != "" && value.ScopeID != query.ScopeID {
			continue
		}
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *Memory) DeveloperAssetAIAdvisoryRun(_ context.Context, deploymentID, id string) (model.DeveloperAssetAIAdvisoryRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.aiAdvisoryRuns[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) DeveloperAssetAIAdvisoryRunByInputHash(_ context.Context, deploymentID, promptKey, inputHash string) (model.DeveloperAssetAIAdvisoryRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id := m.developerAssets.aiAdvisoryInputIDs[developerAssetAIInputKey(deploymentID, promptKey, inputHash)]
	value, ok := m.developerAssets.aiAdvisoryRuns[id]
	if !ok {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateDeveloperAssetAIAdvisoryRun(_ context.Context, value model.DeveloperAssetAIAdvisoryRun) (model.DeveloperAssetAIAdvisoryRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID || !value.Valid() {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrConflict
	}
	if _, exists := m.developerAssets.aiAdvisoryRuns[value.ID]; exists {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrConflict
	}
	key := developerAssetAIInputKey(value.DeploymentID, value.PromptKey, value.InputHash)
	if existingID := m.developerAssets.aiAdvisoryInputIDs[key]; existingID != "" {
		return memoryClone(m.developerAssets.aiAdvisoryRuns[existingID]), nil
	}
	if strings.TrimSpace(value.CreatedBy) == "" {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrConflict
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	m.developerAssets.aiAdvisoryRuns[value.ID] = memoryClone(value)
	m.developerAssets.aiAdvisoryInputIDs[key] = value.ID
	return memoryClone(value), nil
}
