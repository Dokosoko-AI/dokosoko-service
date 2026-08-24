package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) packageArtifactLocked(value model.PackageArtifact) model.PackageArtifact {
	value.LatestRelease, value.UsedBy = nil, nil
	var latest model.PackageRelease
	for _, release := range m.packageReleases[value.ID] {
		if latest.ID == "" || release.PublishedAt.After(latest.PublishedAt) {
			latest = release
		}
	}
	if latest.ID != "" {
		copy := memoryClone(latest)
		value.LatestRelease = &copy
	}
	for integrationID, bindings := range m.integrationPackageLinks {
		if _, ok := bindings[value.ID]; ok {
			value.UsedBy = append(value.UsedBy, integrationID)
		}
	}
	sort.Strings(value.UsedBy)
	return memoryClone(value)
}

func (m *Memory) packageReleaseLocked(value model.PackageRelease) model.PackageRelease {
	return memoryClone(value)
}

func (m *Memory) integrationPackageBindingLocked(value model.IntegrationPackageBinding) model.IntegrationPackageBinding {
	value.Artifact, value.Release = nil, nil
	if artifact, ok := m.packageArtifacts[value.PackageArtifactID]; ok {
		copy := m.packageArtifactLocked(artifact)
		value.Artifact = &copy
	}
	if release, ok := m.packageReleases[value.PackageArtifactID][value.PackageReleaseID]; ok {
		copy := m.packageReleaseLocked(release)
		value.Release = &copy
	}
	return memoryClone(value)
}

func (m *Memory) PackageArtifacts(_ context.Context, deploymentID string) ([]model.PackageArtifact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.PackageArtifact, 0)
	for _, value := range m.packageArtifacts {
		if value.DeploymentID == deploymentID {
			result = append(result, m.packageArtifactLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ecosystem == result[j].Ecosystem {
			return result[i].Coordinate < result[j].Coordinate
		}
		return result[i].Ecosystem < result[j].Ecosystem
	})
	return result, nil
}

func (m *Memory) PackageArtifact(_ context.Context, deploymentID, id string) (model.PackageArtifact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.packageArtifacts[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.PackageArtifact{}, ErrNotFound
	}
	return m.packageArtifactLocked(value), nil
}

func (m *Memory) CreatePackageArtifact(_ context.Context, value model.PackageArtifact) (model.PackageArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return model.PackageArtifact{}, ErrNotFound
	}
	for _, current := range m.packageArtifacts {
		if current.DeploymentID == value.DeploymentID && current.Ecosystem == value.Ecosystem && current.Coordinate == value.Coordinate {
			return model.PackageArtifact{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.Lifecycle == "" {
		value.Lifecycle = "draft"
	}
	m.packageArtifacts[value.ID] = memoryClone(value)
	m.packageReleases[value.ID] = make(map[string]model.PackageRelease)
	m.deployment.CatalogRevision++
	return m.packageArtifactLocked(value), nil
}

func (m *Memory) UpdatePackageArtifact(_ context.Context, value model.PackageArtifact, expected int64) (model.PackageArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.packageArtifacts[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.PackageArtifact{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.PackageArtifact{}, ErrConflict
	}
	for id, candidate := range m.packageArtifacts {
		if id != value.ID && candidate.DeploymentID == value.DeploymentID && candidate.Ecosystem == value.Ecosystem && candidate.Coordinate == value.Coordinate {
			return model.PackageArtifact{}, ErrConflict
		}
	}
	value.Revision, value.CreatedAt, value.UpdatedAt = expected+1, current.CreatedAt, time.Now().UTC()
	value.LatestRelease, value.UsedBy = nil, nil
	m.packageArtifacts[value.ID] = memoryClone(value)
	m.deployment.CatalogRevision++
	return m.packageArtifactLocked(value), nil
}

func (m *Memory) PackageReleases(_ context.Context, artifactID string) ([]model.PackageRelease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.packageArtifacts[artifactID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.PackageRelease, 0, len(m.packageReleases[artifactID]))
	for _, value := range m.packageReleases[artifactID] {
		result = append(result, m.packageReleaseLocked(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PublishedAt.After(result[j].PublishedAt) })
	return result, nil
}

func (m *Memory) PackageRelease(_ context.Context, deploymentID, id string) (model.PackageRelease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for artifactID, releases := range m.packageReleases {
		artifact, ok := m.packageArtifacts[artifactID]
		if !ok || artifact.DeploymentID != deploymentID {
			continue
		}
		if value, ok := releases[id]; ok {
			return m.packageReleaseLocked(value), nil
		}
	}
	return model.PackageRelease{}, ErrNotFound
}

func (m *Memory) CreatePackageRelease(_ context.Context, deploymentID string, value model.PackageRelease, expectedArtifactRevision int64) (model.PackageArtifact, model.PackageRelease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	artifact, ok := m.packageArtifacts[value.PackageArtifactID]
	if !ok || artifact.DeploymentID != deploymentID {
		return model.PackageArtifact{}, model.PackageRelease{}, ErrNotFound
	}
	if artifact.Revision != expectedArtifactRevision {
		return model.PackageArtifact{}, model.PackageRelease{}, ErrConflict
	}
	if artifact.Lifecycle != "draft" && artifact.Lifecycle != "active" || artifact.SunsetAt != nil && !artifact.SunsetAt.After(time.Now().UTC()) {
		return model.PackageArtifact{}, model.PackageRelease{}, ErrConflict
	}
	for _, current := range m.packageReleases[artifact.ID] {
		if current.Version == value.Version || current.ContentHash == value.ContentHash {
			return model.PackageArtifact{}, model.PackageRelease{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	if value.PublishedAt.IsZero() {
		value.PublishedAt = now
	}
	value.CreatedAt = now
	m.packageReleases[artifact.ID][value.ID] = memoryClone(value)
	if artifact.Lifecycle == "draft" {
		artifact.Lifecycle = "active"
	}
	artifact.Revision++
	artifact.UpdatedAt = now
	m.packageArtifacts[artifact.ID] = artifact
	m.deployment.CatalogRevision++
	return m.packageArtifactLocked(artifact), m.packageReleaseLocked(value), nil
}

func (m *Memory) IntegrationPackageBindings(_ context.Context, integrationID string) ([]model.IntegrationPackageBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationPackageBinding, 0, len(m.integrationPackageLinks[integrationID]))
	for _, value := range m.integrationPackageLinks[integrationID] {
		result = append(result, m.integrationPackageBindingLocked(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackageArtifactID < result[j].PackageArtifactID })
	return result, nil
}

func (m *Memory) SaveIntegrationPackageBinding(_ context.Context, value model.IntegrationPackageBinding) (model.IntegrationPackageBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.IntegrationID]
	if !ok {
		return model.IntegrationPackageBinding{}, ErrNotFound
	}
	artifact, ok := m.packageArtifacts[value.PackageArtifactID]
	if !ok || artifact.DeploymentID != integration.DeploymentID {
		return model.IntegrationPackageBinding{}, ErrNotFound
	}
	if artifact.Lifecycle != "active" || artifact.SunsetAt != nil && !artifact.SunsetAt.After(time.Now().UTC()) {
		return model.IntegrationPackageBinding{}, ErrConflict
	}
	if release, ok := m.packageReleases[value.PackageArtifactID][value.PackageReleaseID]; !ok || release.PackageArtifactID != artifact.ID {
		return model.IntegrationPackageBinding{}, ErrNotFound
	}
	now := time.Now().UTC()
	if current, ok := m.integrationPackageLinks[value.IntegrationID][value.PackageArtifactID]; ok {
		value.ID, value.CreatedAt = current.ID, current.CreatedAt
	} else {
		value.CreatedAt = now
	}
	value.DeploymentID = integration.DeploymentID
	value.UpdatedAt = now
	m.integrationPackageLinks[value.IntegrationID][value.PackageArtifactID] = memoryClone(value)
	m.deployment.CatalogRevision++
	return m.integrationPackageBindingLocked(value), nil
}

func (m *Memory) DeleteIntegrationPackageBinding(_ context.Context, integrationID, artifactID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	links, ok := m.integrationPackageLinks[integrationID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := links[artifactID]; !ok {
		return ErrNotFound
	}
	delete(links, artifactID)
	m.deployment.CatalogRevision++
	return nil
}
