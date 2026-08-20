package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func memoryClone[T any](value T) T {
	encoded, _ := json.Marshal(value)
	var cloned T
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

// The public JSON representation intentionally omits provider endpoints and
// secret references. Preserve those internal fields when cloning values for
// the in-memory store; using memoryClone directly would silently erase them.
func cloneAccessConnection(value model.AccessConnection) model.AccessConnection {
	baseURL := value.BaseURL
	managementSecretID := value.ManagementSecretID
	legacyProviderID := value.LegacyProviderID
	cloned := memoryClone(value)
	cloned.BaseURL = baseURL
	cloned.ManagementSecretID = managementSecretID
	cloned.LegacyProviderID = legacyProviderID
	return cloned
}

func cloneAccessCredential(value model.AccessCredential) model.AccessCredential {
	encryptedSecretID := value.EncryptedSecretID
	cloned := memoryClone(value)
	cloned.EncryptedSecretID = encryptedSecretID
	return cloned
}

func (m *Memory) Deployment(context.Context) (model.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment {
		return model.Deployment{}, ErrNotFound
	}
	return m.deployment, nil
}

func (m *Memory) CreateDeployment(_ context.Context, value model.Deployment) (model.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasDeployment {
		return model.Deployment{}, ErrConflict
	}
	if _, ok := m.orgs[value.OrganisationID]; !ok {
		return model.Deployment{}, ErrNotFound
	}
	now := time.Now().UTC()
	value.Revision, value.CatalogRevision = 1, 1
	if value.DefaultReleasePolicy == "" {
		value.DefaultReleasePolicy = "latest"
	}
	value.CreatedAt, value.UpdatedAt = now, now
	m.deployment, m.hasDeployment = value, true
	product := model.Product{ID: value.ID, OrganisationID: value.OrganisationID, Name: value.Name, Slug: value.Slug, Description: value.Description, DefaultVersionPolicy: value.DefaultReleasePolicy, CatalogRevision: value.CatalogRevision, RequirePromotionApproval: value.RequirePromotionApproval, PublicMCPEnabled: value.PublicMCPEnabled, Revision: value.Revision, CreatedAt: now, UpdatedAt: now}
	m.products[value.ID] = product
	m.productVersions[value.ID] = make(map[string]model.ProductVersion)
	m.productVersionPins[value.ID] = make(map[string]model.ProductVersionPin)
	m.productVersionPinHistory[value.ID] = nil
	m.productInstallations[value.ID] = make(map[string]model.ProductInstallation)
	m.productBuilds[value.ID] = make(map[string]model.ProductBuild)
	m.sources[value.ID] = make(map[string]model.Source)
	m.packages[value.ID] = make(map[string]model.Package)
	m.knowledge[value.ID] = nil
	m.envs[value.ID] = make(map[string]model.Environment)
	m.tools[value.ID] = make(map[string]model.Tool)
	m.mcpConnections[value.ID] = make(map[string]model.MCPConnection)
	m.providers[value.ID] = make(map[string]model.Provider)
	m.projects[value.ID] = make(map[string]model.Project)
	m.leases[value.ID] = make(map[string]model.CredentialLease)
	m.integrationRuns[value.ID] = make(map[string]model.IntegrationRun)
	m.reportSubmissions[value.ID] = make(map[string]model.ReportSubmission)
	m.llmProfiles[value.ID] = make(map[string]model.LLMProfile)
	return value, nil
}

func (m *Memory) UpdateDeployment(_ context.Context, value model.Deployment, expected int64) (model.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.ID {
		return model.Deployment{}, ErrNotFound
	}
	if m.deployment.Revision != expected {
		return model.Deployment{}, ErrConflict
	}
	value.CreatedAt = m.deployment.CreatedAt
	value.Revision = expected + 1
	value.UpdatedAt = time.Now().UTC()
	m.deployment = value
	if product, ok := m.products[value.ID]; ok {
		product.Name, product.Slug, product.Description = value.Name, value.Slug, value.Description
		product.DefaultVersionPolicy, product.CatalogRevision = value.DefaultReleasePolicy, value.CatalogRevision
		product.RequirePromotionApproval, product.PublicMCPEnabled = value.RequirePromotionApproval, value.PublicMCPEnabled
		product.Revision, product.UpdatedAt = value.Revision, value.UpdatedAt
		m.products[value.ID] = product
	}
	return value, nil
}

func (m *Memory) integrationLocked(value model.Integration) model.Integration {
	value.Resources = nil
	for _, link := range m.integrationResourceLinks[value.ID] {
		value.Resources = append(value.Resources, m.integrationResourceLinkLocked(link))
	}
	sort.Slice(value.Resources, func(i, j int) bool {
		if value.Resources[i].Kind == value.Resources[j].Kind {
			return value.Resources[i].Name < value.Resources[j].Name
		}
		return value.Resources[i].Kind < value.Resources[j].Kind
	})
	value.AccessConnections = nil
	for connectionID, links := range m.integrationAccessLinks {
		if links[value.ID] {
			value.AccessConnections = append(value.AccessConnections, connectionID)
		}
	}
	sort.Strings(value.AccessConnections)
	value.SupportRouteID = m.integrationSupportRoutes[value.ID]
	return memoryClone(value)
}

func (m *Memory) Integrations(_ context.Context, deploymentID string) ([]model.Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.Integration, 0)
	for _, value := range m.integrations {
		if value.DeploymentID == deploymentID {
			result = append(result, m.integrationLocked(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DisplayName == result[j].DisplayName {
			return result[i].VersionKey < result[j].VersionKey
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, nil
}

func (m *Memory) Integration(_ context.Context, deploymentID, id string) (model.Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.integrations[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.Integration{}, ErrNotFound
	}
	return m.integrationLocked(value), nil
}

func (m *Memory) CreateIntegration(_ context.Context, value model.Integration) (model.Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return model.Integration{}, ErrNotFound
	}
	for _, current := range m.integrations {
		if current.DeploymentID == value.DeploymentID && current.FamilyKey == value.FamilyKey && current.VersionKey == value.VersionKey {
			return model.Integration{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	if value.Lifecycle == "" {
		value.Lifecycle = "draft"
	}
	m.integrations[value.ID] = value
	m.integrationResourceLinks[value.ID] = make(map[string]model.IntegrationResourceLink)
	m.integrationRevisions[value.ID] = make(map[string]model.IntegrationRevision)
	m.deployment.CatalogRevision++
	return m.integrationLocked(value), nil
}

func (m *Memory) UpdateIntegration(_ context.Context, value model.Integration, expected int64) (model.Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.integrations[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.Integration{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Integration{}, ErrConflict
	}
	for id, candidate := range m.integrations {
		if id != value.ID && candidate.DeploymentID == value.DeploymentID && candidate.FamilyKey == value.FamilyKey && candidate.VersionKey == value.VersionKey {
			return model.Integration{}, ErrConflict
		}
	}
	value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, time.Now().UTC()
	m.integrations[value.ID] = value
	m.deployment.CatalogRevision++
	return m.integrationLocked(value), nil
}

func (m *Memory) IntegrationRevisions(_ context.Context, integrationID string) ([]model.IntegrationRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationRevision, 0, len(m.integrationRevisions[integrationID]))
	for _, value := range m.integrationRevisions[integrationID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) CreateIntegrationRevision(_ context.Context, value model.IntegrationRevision) (model.IntegrationRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.integrations[value.IntegrationID]; !ok {
		return model.IntegrationRevision{}, ErrNotFound
	}
	for _, current := range m.integrationRevisions[value.IntegrationID] {
		if current.Revision == value.Revision || current.ManifestHash == value.ManifestHash {
			return model.IntegrationRevision{}, ErrConflict
		}
	}
	value.CreatedAt = time.Now().UTC()
	m.integrationRevisions[value.IntegrationID][value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

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
