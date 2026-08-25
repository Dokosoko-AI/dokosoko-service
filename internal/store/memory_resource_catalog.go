package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) resourceSetLocked(value model.ResourceSet) model.ResourceSet {
	value.UsedBy, value.Latest = nil, nil
	var latest model.ResourceSetRevision
	for _, revision := range m.resourceSetRevisions[value.ID] {
		if latest.ID == "" || revision.Revision > latest.Revision {
			latest = revision
		}
	}
	if latest.ID != "" {
		cloned := memoryClone(latest)
		value.Latest = &cloned
	}
	for integrationID, links := range m.integrationResourceLinks {
		if _, ok := links[value.ID]; ok {
			value.UsedBy = append(value.UsedBy, integrationID)
		}
	}
	sort.Strings(value.UsedBy)
	return memoryClone(value)
}

func (m *Memory) ResourceSets(_ context.Context, deploymentID, kind string) ([]model.ResourceSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.ResourceSet, 0)
	for _, value := range m.resourceSets {
		if value.DeploymentID == deploymentID && (kind == "" || value.Kind == kind) {
			result = append(result, m.resourceSetLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) ResourceSet(_ context.Context, deploymentID, id string) (model.ResourceSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.resourceSets[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.ResourceSet{}, ErrNotFound
	}
	return m.resourceSetLocked(value), nil
}

func (m *Memory) CreateResourceSet(_ context.Context, value model.ResourceSet, revision model.ResourceSetRevision) (model.ResourceSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.resourceSets {
		if current.DeploymentID == value.DeploymentID && current.Kind == value.Kind && current.Name == value.Name {
			return model.ResourceSet{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.State == "" {
		value.State = "active"
	}
	revision.ResourceSetID, revision.Revision, revision.CreatedAt = value.ID, 1, now
	m.resourceSets[value.ID] = value
	m.resourceSetRevisions[value.ID] = map[string]model.ResourceSetRevision{revision.ID: memoryClone(revision)}
	m.deployment.CatalogRevision++
	return m.resourceSetLocked(value), nil
}

func (m *Memory) UpdateResourceSet(_ context.Context, value model.ResourceSet, revision model.ResourceSetRevision, expected int64) (model.ResourceSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.resourceSets[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.ResourceSet{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.ResourceSet{}, ErrConflict
	}
	value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, time.Now().UTC()
	revision.ResourceSetID, revision.Revision, revision.CreatedAt = value.ID, value.Revision, value.UpdatedAt
	m.resourceSets[value.ID] = value
	if m.resourceSetRevisions[value.ID] == nil {
		m.resourceSetRevisions[value.ID] = make(map[string]model.ResourceSetRevision)
	}
	m.resourceSetRevisions[value.ID][revision.ID] = memoryClone(revision)
	m.deployment.CatalogRevision++
	return m.resourceSetLocked(value), nil
}

func (m *Memory) ResourceSetRevisions(_ context.Context, resourceSetID string) ([]model.ResourceSetRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.resourceSets[resourceSetID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ResourceSetRevision, 0, len(m.resourceSetRevisions[resourceSetID]))
	for _, value := range m.resourceSetRevisions[resourceSetID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) integrationResourceLinkLocked(value model.IntegrationResourceLink) model.IntegrationResourceLink {
	if set, ok := m.resourceSets[value.ResourceSetID]; ok {
		value.Kind, value.Name = set.Kind, set.Name
	}
	var resolved model.ResourceSetRevision
	for _, revision := range m.resourceSetRevisions[value.ResourceSetID] {
		if value.FollowLatest {
			if resolved.ID == "" || revision.Revision > resolved.Revision {
				resolved = revision
			}
		} else if revision.ID == value.PinnedRevisionID {
			resolved = revision
		}
	}
	if resolved.ID != "" {
		copy := memoryClone(resolved)
		value.ResolvedRevision = &copy
	}
	return memoryClone(value)
}

func (m *Memory) IntegrationResourceLinks(_ context.Context, integrationID string) ([]model.IntegrationResourceLink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationResourceLink, 0, len(m.integrationResourceLinks[integrationID]))
	for _, value := range m.integrationResourceLinks[integrationID] {
		result = append(result, m.integrationResourceLinkLocked(value))
	}
	return result, nil
}

func (m *Memory) SaveIntegrationResourceLink(_ context.Context, value model.IntegrationResourceLink) (model.IntegrationResourceLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.integrations[value.IntegrationID]; !ok {
		return model.IntegrationResourceLink{}, ErrNotFound
	}
	if _, ok := m.resourceSets[value.ResourceSetID]; !ok {
		return model.IntegrationResourceLink{}, ErrNotFound
	}
	if !value.FollowLatest {
		if _, ok := m.resourceSetRevisions[value.ResourceSetID][value.PinnedRevisionID]; !ok {
			return model.IntegrationResourceLink{}, ErrNotFound
		}
	}
	now := time.Now().UTC()
	if current, ok := m.integrationResourceLinks[value.IntegrationID][value.ResourceSetID]; ok {
		value.ID, value.CreatedAt = current.ID, current.CreatedAt
	} else {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	m.integrationResourceLinks[value.IntegrationID][value.ResourceSetID] = value
	m.deployment.CatalogRevision++
	return m.integrationResourceLinkLocked(value), nil
}

func (m *Memory) DeleteIntegrationResourceLink(_ context.Context, integrationID, resourceSetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.integrationResourceLinks[integrationID][resourceSetID]; !ok {
		return ErrNotFound
	}
	delete(m.integrationResourceLinks[integrationID], resourceSetID)
	m.deployment.CatalogRevision++
	return nil
}
