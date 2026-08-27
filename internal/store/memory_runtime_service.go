package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) currentToolRuntimeTargetsLocked(connectionID string) []model.ToolRuntimeTarget {
	result := make([]model.ToolRuntimeTarget, 0)
	for _, revision := range m.runtimeConnectionHistory[connectionID] {
		if !revision.Current {
			continue
		}
		result = append(result, model.ToolRuntimeTarget{
			EnvironmentID:              revision.EnvironmentID,
			RuntimeServiceConnectionID: revision.ConnectionID,
			ConnectionRevisionID:       revision.ID,
			BaseURL:                    revision.BaseURL,
			AuthenticationType:         revision.AuthenticationType,
			CredentialSetID:            revision.CredentialSetID,
			AuthConfig:                 append(json.RawMessage(nil), revision.AuthConfig...),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EnvironmentID < result[j].EnvironmentID })
	return result
}

func (m *Memory) enrichToolRuntimeTargetsLocked(value model.Tool) model.Tool {
	value = cloneTool(value)
	if value.RuntimeServiceConnectionID == "" {
		return value
	}
	if value.State == "published" {
		value.RuntimeTargets = cloneToolRuntimeTargets(m.toolRuntimeTargets[value.ID][value.Revision])
	} else {
		value.RuntimeTargets = m.currentToolRuntimeTargetsLocked(value.RuntimeServiceConnectionID)
	}
	value.CredentialPresent = true
	now := time.Now().UTC()
	for index := range value.RuntimeTargets {
		target := &value.RuntimeTargets[index]
		if target.CredentialSetID == "" {
			continue
		}
		credentialSet, ok := m.runtimeCredentialSets[target.CredentialSetID]
		if !ok || credentialSet.State != "active" || credentialSet.EnvironmentID != target.EnvironmentID {
			value.CredentialPresent = false
			continue
		}
		target.HeaderName = credentialSet.HeaderName
		target.AuthConfig = append(json.RawMessage(nil), credentialSet.AuthConfig...)
		target.AccessEvaluationURL = credentialSet.AccessEvaluationURL
		target.UsageURL = credentialSet.UsageURL
		found := false
		for _, version := range m.runtimeCredentialHistory[credentialSet.ID] {
			if version.State != "active" || version.ExpiresAt != nil && !version.ExpiresAt.After(now) {
				continue
			}
			target.CredentialVersionID = version.ID
			target.CredentialSecretID = version.SecretID
			target.CredentialFingerprint = version.Fingerprint
			found = true
			break
		}
		if !found {
			value.CredentialPresent = false
		}
	}
	return value
}

func cloneRuntimeConnectionRevision(value model.RuntimeServiceConnectionRevision) model.RuntimeServiceConnectionRevision {
	value.AuthConfig = append(json.RawMessage(nil), value.AuthConfig...)
	return value
}

func cloneRuntimeCredentialVersion(value model.RuntimeCredentialVersion) model.RuntimeCredentialVersion {
	return value
}

func (m *Memory) enrichRuntimeConnectionLocked(value model.RuntimeServiceConnection) model.RuntimeServiceConnection {
	value.CurrentRevisions = nil
	for _, revision := range m.runtimeConnectionHistory[value.ID] {
		if revision.Current {
			value.CurrentRevisions = append(value.CurrentRevisions, cloneRuntimeConnectionRevision(revision))
		}
	}
	sort.Slice(value.CurrentRevisions, func(i, j int) bool {
		return value.CurrentRevisions[i].EnvironmentID < value.CurrentRevisions[j].EnvironmentID
	})
	return value
}

func (m *Memory) RuntimeServiceConnections(_ context.Context, deploymentID, integrationID string) ([]model.RuntimeServiceConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.RuntimeServiceConnection, 0)
	for _, value := range m.runtimeConnections {
		if value.DeploymentID == deploymentID && (integrationID == "" || value.IntegrationID == integrationID) {
			result = append(result, m.enrichRuntimeConnectionLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) RuntimeServiceConnection(_ context.Context, deploymentID, id string) (model.RuntimeServiceConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.runtimeConnections[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.RuntimeServiceConnection{}, ErrNotFound
	}
	return m.enrichRuntimeConnectionLocked(value), nil
}

func (m *Memory) CreateRuntimeServiceConnection(_ context.Context, value model.RuntimeServiceConnection) (model.RuntimeServiceConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.IntegrationID]
	if !ok || integration.DeploymentID != value.DeploymentID || integration.OrganisationID != value.OrganisationID {
		return model.RuntimeServiceConnection{}, ErrNotFound
	}
	for _, current := range m.runtimeConnections {
		if current.IntegrationID == value.IntegrationID && current.Name == value.Name {
			return model.RuntimeServiceConnection{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = now, now
	value.CurrentRevisions = nil
	m.runtimeConnections[value.ID] = value
	return value, nil
}

func (m *Memory) UpdateRuntimeServiceConnection(_ context.Context, value model.RuntimeServiceConnection, expected int64) (model.RuntimeServiceConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runtimeConnections[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.RuntimeServiceConnection{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.RuntimeServiceConnection{}, ErrConflict
	}
	if current.OrganisationID != value.OrganisationID || current.IntegrationID != value.IntegrationID {
		return model.RuntimeServiceConnection{}, ErrConflict
	}
	for _, candidate := range m.runtimeConnections {
		if candidate.ID != value.ID && candidate.IntegrationID == value.IntegrationID && candidate.Name == value.Name {
			return model.RuntimeServiceConnection{}, ErrConflict
		}
	}
	value.Revision = expected + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	value.CurrentRevisions = nil
	m.runtimeConnections[value.ID] = value
	return m.enrichRuntimeConnectionLocked(value), nil
}

func (m *Memory) RuntimeServiceConnectionRevisions(_ context.Context, connectionID, environmentID string) ([]model.RuntimeServiceConnectionRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.runtimeConnections[connectionID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.RuntimeServiceConnectionRevision, 0)
	for _, value := range m.runtimeConnectionHistory[connectionID] {
		if environmentID == "" || value.EnvironmentID == environmentID {
			result = append(result, cloneRuntimeConnectionRevision(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].EnvironmentID == result[j].EnvironmentID {
			return result[i].Revision > result[j].Revision
		}
		return result[i].EnvironmentID < result[j].EnvironmentID
	})
	return result, nil
}

func (m *Memory) runtimeEnvironmentLocked(deploymentID, environmentID string) (model.Environment, bool) {
	values := m.envs[deploymentID]
	value, ok := values[environmentID]
	return value, ok
}

func (m *Memory) CreateRuntimeServiceConnectionRevision(_ context.Context, value model.RuntimeServiceConnectionRevision) (model.RuntimeServiceConnectionRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	connection, ok := m.runtimeConnections[value.ConnectionID]
	if !ok {
		return model.RuntimeServiceConnectionRevision{}, ErrNotFound
	}
	if _, ok := m.runtimeEnvironmentLocked(connection.DeploymentID, value.EnvironmentID); !ok {
		return model.RuntimeServiceConnectionRevision{}, ErrNotFound
	}
	if value.CredentialSetID != "" {
		credentialSet, ok := m.runtimeCredentialSets[value.CredentialSetID]
		if !ok || credentialSet.DeploymentID != connection.DeploymentID || credentialSet.EnvironmentID != value.EnvironmentID || (credentialSet.Scope == "dedicated" && credentialSet.OwnerIntegrationID != connection.IntegrationID) {
			return model.RuntimeServiceConnectionRevision{}, ErrConflict
		}
	}
	if m.runtimeConnectionHistory[value.ConnectionID] == nil {
		m.runtimeConnectionHistory[value.ConnectionID] = make(map[string]model.RuntimeServiceConnectionRevision)
	}
	next := int64(1)
	for id, current := range m.runtimeConnectionHistory[value.ConnectionID] {
		if current.EnvironmentID != value.EnvironmentID {
			continue
		}
		if current.ContentHash == value.ContentHash {
			return model.RuntimeServiceConnectionRevision{}, ErrConflict
		}
		if current.Revision >= next {
			next = current.Revision + 1
		}
		if current.Current {
			current.Current = false
			m.runtimeConnectionHistory[value.ConnectionID][id] = current
		}
	}
	value.Revision = next
	value.Current = true
	value.CreatedAt = time.Now().UTC()
	value.AuthConfig = append(json.RawMessage(nil), value.AuthConfig...)
	m.runtimeConnectionHistory[value.ConnectionID][value.ID] = value
	connection.Revision++
	connection.UpdatedAt = value.CreatedAt
	m.runtimeConnections[connection.ID] = connection
	return cloneRuntimeConnectionRevision(value), nil
}

func (m *Memory) enrichRuntimeCredentialSetLocked(value model.RuntimeCredentialSet) model.RuntimeCredentialSet {
	value.AuthConfig = append(json.RawMessage(nil), value.AuthConfig...)
	value.Versions = nil
	value.CredentialPresent = false
	value.ActiveFingerprint = ""
	for _, version := range m.runtimeCredentialHistory[value.ID] {
		cloned := cloneRuntimeCredentialVersion(version)
		value.Versions = append(value.Versions, cloned)
		if version.State == "active" {
			value.CredentialPresent = true
			value.ActiveFingerprint = version.Fingerprint
		}
	}
	sort.Slice(value.Versions, func(i, j int) bool { return value.Versions[i].CreatedAt.After(value.Versions[j].CreatedAt) })
	return value
}

func (m *Memory) RuntimeCredentialSets(_ context.Context, deploymentID, environmentID string) ([]model.RuntimeCredentialSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.RuntimeCredentialSet, 0)
	for _, value := range m.runtimeCredentialSets {
		if value.DeploymentID == deploymentID && (environmentID == "" || value.EnvironmentID == environmentID) {
			result = append(result, m.enrichRuntimeCredentialSetLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) RuntimeCredentialSet(_ context.Context, deploymentID, id string) (model.RuntimeCredentialSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.runtimeCredentialSets[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.RuntimeCredentialSet{}, ErrNotFound
	}
	return m.enrichRuntimeCredentialSetLocked(value), nil
}

func (m *Memory) CreateRuntimeCredentialSet(_ context.Context, value model.RuntimeCredentialSet) (model.RuntimeCredentialSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runtimeEnvironmentLocked(value.DeploymentID, value.EnvironmentID); !ok {
		return model.RuntimeCredentialSet{}, ErrNotFound
	}
	if value.Scope == "dedicated" {
		integration, ok := m.integrations[value.OwnerIntegrationID]
		if !ok || integration.DeploymentID != value.DeploymentID || integration.OrganisationID != value.OrganisationID {
			return model.RuntimeCredentialSet{}, ErrNotFound
		}
	} else if value.Scope != "shared" || value.OwnerIntegrationID != "" {
		return model.RuntimeCredentialSet{}, ErrConflict
	}
	for _, current := range m.runtimeCredentialSets {
		if current.DeploymentID == value.DeploymentID && current.EnvironmentID == value.EnvironmentID && (current.Name == value.Name || current.EnvironmentVariable == value.EnvironmentVariable) {
			return model.RuntimeCredentialSet{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = now, now
	value.Versions = nil
	value.AuthConfig = append(json.RawMessage(nil), value.AuthConfig...)
	m.runtimeCredentialSets[value.ID] = value
	return value, nil
}

func (m *Memory) UpdateRuntimeCredentialSet(_ context.Context, value model.RuntimeCredentialSet, expected int64) (model.RuntimeCredentialSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.runtimeCredentialSets[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.RuntimeCredentialSet{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.RuntimeCredentialSet{}, ErrConflict
	}
	if current.OrganisationID != value.OrganisationID || current.EnvironmentID != value.EnvironmentID || current.Scope != value.Scope || current.OwnerIntegrationID != value.OwnerIntegrationID || current.AuthenticationType != value.AuthenticationType {
		return model.RuntimeCredentialSet{}, ErrConflict
	}
	for _, candidate := range m.runtimeCredentialSets {
		if candidate.ID != value.ID && candidate.DeploymentID == value.DeploymentID && candidate.EnvironmentID == value.EnvironmentID && (candidate.Name == value.Name || candidate.EnvironmentVariable == value.EnvironmentVariable) {
			return model.RuntimeCredentialSet{}, ErrConflict
		}
	}
	value.Revision = expected + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	value.Versions = nil
	value.AuthConfig = append(json.RawMessage(nil), value.AuthConfig...)
	m.runtimeCredentialSets[value.ID] = value
	return m.enrichRuntimeCredentialSetLocked(value), nil
}

func (m *Memory) RuntimeCredentialVersions(_ context.Context, credentialSetID string) ([]model.RuntimeCredentialVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.runtimeCredentialSets[credentialSetID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.RuntimeCredentialVersion, 0, len(m.runtimeCredentialHistory[credentialSetID]))
	for _, value := range m.runtimeCredentialHistory[credentialSetID] {
		result = append(result, cloneRuntimeCredentialVersion(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) CreateRuntimeCredentialVersion(_ context.Context, value model.RuntimeCredentialVersion) (model.RuntimeCredentialVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	credentialSet, ok := m.runtimeCredentialSets[value.CredentialSetID]
	if !ok {
		return model.RuntimeCredentialVersion{}, ErrNotFound
	}
	secret, ok := m.secrets[value.SecretID]
	if !ok || secret.OrganisationID != credentialSet.OrganisationID {
		return model.RuntimeCredentialVersion{}, ErrNotFound
	}
	if m.runtimeCredentialHistory[value.CredentialSetID] == nil {
		m.runtimeCredentialHistory[value.CredentialSetID] = make(map[string]model.RuntimeCredentialVersion)
	}
	if _, exists := m.runtimeCredentialHistory[value.CredentialSetID][value.ID]; exists {
		return model.RuntimeCredentialVersion{}, ErrConflict
	}
	value.State = "pending"
	value.ActivatedAt, value.RetiresAt, value.RevokedAt = nil, nil, nil
	value.CreatedAt = time.Now().UTC()
	m.runtimeCredentialHistory[value.CredentialSetID][value.ID] = value
	return cloneRuntimeCredentialVersion(value), nil
}

func (m *Memory) ActivateRuntimeCredentialVersion(_ context.Context, deploymentID, credentialSetID, versionID string, now time.Time) (model.RuntimeCredentialVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	credentialSet, ok := m.runtimeCredentialSets[credentialSetID]
	if !ok || credentialSet.DeploymentID != deploymentID {
		return model.RuntimeCredentialVersion{}, ErrNotFound
	}
	versions := m.runtimeCredentialHistory[credentialSetID]
	target, ok := versions[versionID]
	if !ok || target.State != "pending" {
		return model.RuntimeCredentialVersion{}, ErrConflict
	}
	activatedAt := now.UTC()
	for id, current := range versions {
		if current.State == "active" {
			current.State = "retiring"
			current.RetiresAt = &activatedAt
			versions[id] = current
		}
	}
	target.State = "active"
	target.ActivatedAt = &activatedAt
	versions[versionID] = target
	credentialSet.Revision++
	credentialSet.UpdatedAt = activatedAt
	m.runtimeCredentialSets[credentialSetID] = credentialSet
	return cloneRuntimeCredentialVersion(target), nil
}

func (m *Memory) RevokeRuntimeCredentialVersion(_ context.Context, deploymentID, credentialSetID, versionID string, now time.Time) (model.RuntimeCredentialVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	credentialSet, ok := m.runtimeCredentialSets[credentialSetID]
	if !ok || credentialSet.DeploymentID != deploymentID {
		return model.RuntimeCredentialVersion{}, ErrNotFound
	}
	versions := m.runtimeCredentialHistory[credentialSetID]
	target, ok := versions[versionID]
	if !ok {
		return model.RuntimeCredentialVersion{}, ErrNotFound
	}
	if target.State == "revoked" {
		return cloneRuntimeCredentialVersion(target), nil
	}
	revokedAt := now.UTC()
	target.State = "revoked"
	target.RevokedAt = &revokedAt
	versions[versionID] = target
	credentialSet.Revision++
	credentialSet.UpdatedAt = revokedAt
	m.runtimeCredentialSets[credentialSetID] = credentialSet
	return cloneRuntimeCredentialVersion(target), nil
}
