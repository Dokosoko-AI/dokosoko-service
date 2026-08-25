package store

import (
	"context"
	"encoding/json"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"time"
)

func (m *Memory) Organisations(_ context.Context) ([]model.Organisation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Organisation, 0, len(m.orgs))
	for _, value := range m.orgs {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateOrganisation(_ context.Context, value model.Organisation) (model.Organisation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.orgs {
		if current.Slug == value.Slug {
			return model.Organisation{}, ErrConflict
		}
	}
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.orgs[value.ID] = value
	return value, nil
}

func (m *Memory) Products(_ context.Context, organisationID string) ([]model.Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Product, 0)
	for _, value := range m.products {
		if value.OrganisationID == organisationID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateProduct(_ context.Context, value model.Product) (model.Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orgs[value.OrganisationID]; !ok {
		return model.Product{}, ErrNotFound
	}
	for _, current := range m.products {
		if current.OrganisationID == value.OrganisationID && current.Slug == value.Slug {
			return model.Product{}, ErrConflict
		}
	}
	value.Revision = 1
	if value.CatalogRevision == 0 {
		value.CatalogRevision = 1
	}
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.products[value.ID] = value
	m.productVersions[value.ID] = make(map[string]model.ProductVersion)
	m.productVersionPins[value.ID] = make(map[string]model.ProductVersionPin)
	m.productVersionPinHistory[value.ID] = nil
	m.productInstallations[value.ID] = make(map[string]model.ProductInstallation)
	m.productBuilds[value.ID] = make(map[string]model.ProductBuild)
	m.sources[value.ID] = make(map[string]model.Source)
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
	m.aiProviderConnections[value.ID] = make(map[string]model.AIProviderConnection)
	m.aiWorkloadProfiles[value.ID] = make(map[string]model.AIWorkloadProfile)
	m.integrationAnalyses[value.ID] = make(map[string]model.IntegrationAnalysis)
	m.recipes[value.ID] = make(map[string]model.Recipe)
	m.aiJobs[value.ID] = make(map[string]model.AIJob)
	return value, nil
}

func (m *Memory) Environments(_ context.Context, productID string) ([]model.Environment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.envs[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.Environment, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) CreateEnvironment(_ context.Context, value model.Environment) (model.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Environment{}, ErrNotFound
	}
	for _, current := range m.envs[value.ProductID] {
		if current.Slug == value.Slug || (value.IsProduction && current.IsProduction) {
			return model.Environment{}, ErrConflict
		}
	}
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.envs[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) Product(_ context.Context, id string) (model.Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.products[id]
	if !ok {
		return model.Product{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) UpdateProduct(_ context.Context, value model.Product, expected int64) (model.Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.products[value.ID]
	if !ok {
		return model.Product{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Product{}, ErrConflict
	}
	value.Revision = current.Revision + 1
	value.CatalogRevision = current.CatalogRevision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.products[value.ID] = value
	return value, nil
}

func (m *Memory) BumpProductCatalogRevision(_ context.Context, productID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bumpProductCatalogRevisionLocked(productID, time.Now().UTC())
}

func (m *Memory) bumpProductCatalogRevisionLocked(productID string, now time.Time) (int64, error) {
	value, ok := m.products[productID]
	if !ok {
		return 0, ErrNotFound
	}
	value.CatalogRevision++
	value.UpdatedAt = now
	m.products[productID] = value
	return value.CatalogRevision, nil
}

func (m *Memory) ProductVersions(_ context.Context, productID string) ([]model.ProductVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.productVersions[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ProductVersion, 0, len(values))
	for _, value := range values {
		value.Manifest = cloneProductDefinition(value.Manifest)
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].PublishedAt.After(result[j].PublishedAt) })
	return result, nil
}

func (m *Memory) ProductVersion(_ context.Context, productID, id string) (model.ProductVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.productVersions[productID][id]
	if !ok {
		return model.ProductVersion{}, ErrNotFound
	}
	value.Manifest = cloneProductDefinition(value.Manifest)
	return value, nil
}

func (m *Memory) CreateProductVersion(_ context.Context, value model.ProductVersion) (model.ProductVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productVersions[value.ProductID]
	if !ok {
		return model.ProductVersion{}, ErrNotFound
	}
	for _, current := range values {
		if current.Version == value.Version {
			return model.ProductVersion{}, ErrConflict
		}
	}
	if value.IsLatest {
		for id, current := range values {
			if !current.IsLatest {
				continue
			}
			current.IsLatest = false
			current.Revision++
			current.UpdatedAt = time.Now().UTC()
			values[id] = current
		}
	}
	now := time.Now().UTC()
	value.Revision, value.PublishedAt, value.CreatedAt, value.UpdatedAt = 1, now, now, now
	value.Manifest = cloneProductDefinition(value.Manifest)
	values[value.ID] = value
	if _, err := m.bumpProductCatalogRevisionLocked(value.ProductID, now); err != nil {
		delete(values, value.ID)
		return model.ProductVersion{}, err
	}
	return value, nil
}

func (m *Memory) UpdateProductVersion(_ context.Context, value model.ProductVersion, expected int64) (model.ProductVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productVersions[value.ProductID]
	if !ok {
		return model.ProductVersion{}, ErrNotFound
	}
	current, ok := values[value.ID]
	if !ok {
		return model.ProductVersion{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.ProductVersion{}, ErrConflict
	}
	if value.IsLatest {
		for id, candidate := range values {
			if id == value.ID || !candidate.IsLatest {
				continue
			}
			candidate.IsLatest = false
			candidate.Revision++
			candidate.UpdatedAt = time.Now().UTC()
			values[id] = candidate
		}
	}
	value.Version, value.ProfileID, value.ProfileName = current.Version, current.ProfileID, current.ProfileName
	value.DefinitionRevision, value.Manifest = current.DefinitionRevision, cloneProductDefinition(current.Manifest)
	value.OrganisationID, value.PublishedAt, value.CreatedAt = current.OrganisationID, current.PublishedAt, current.CreatedAt
	value.Revision, value.UpdatedAt = current.Revision+1, time.Now().UTC()
	values[value.ID] = value
	if _, err := m.bumpProductCatalogRevisionLocked(value.ProductID, value.UpdatedAt); err != nil {
		values[value.ID] = current
		return model.ProductVersion{}, err
	}
	return value, nil
}

func (m *Memory) ProductVersionPins(_ context.Context, productID string) ([]model.ProductVersionPin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.productVersionPins[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ProductVersionPin, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Scope == result[j].Scope {
			return result[i].ScopeID < result[j].ScopeID
		}
		return result[i].Scope < result[j].Scope
	})
	return result, nil
}

func pinMapKey(scope, scopeID string) string { return scope + "\x00" + scopeID }

func (m *Memory) ProductVersionPin(_ context.Context, productID, scope, scopeID string) (model.ProductVersionPin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.productVersionPins[productID][pinMapKey(scope, scopeID)]
	if !ok {
		return model.ProductVersionPin{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) SaveProductVersionPin(_ context.Context, value model.ProductVersionPin, expected int64, history model.ProductVersionPinHistory) (model.ProductVersionPin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productVersionPins[value.ProductID]
	if !ok {
		return model.ProductVersionPin{}, ErrNotFound
	}
	if _, ok := m.productVersions[value.ProductID][value.ProductVersionID]; !ok {
		return model.ProductVersionPin{}, ErrNotFound
	}
	now := time.Now().UTC()
	key := pinMapKey(value.Scope, value.ScopeID)
	previous, existed := values[key]
	if existed {
		if previous.Revision != expected {
			return model.ProductVersionPin{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = previous.ID, previous.CreatedAt, previous.Revision+1
	} else {
		if expected != 0 {
			return model.ProductVersionPin{}, ErrConflict
		}
		value.Revision, value.CreatedAt = 1, now
	}
	value.UpdatedAt = now
	values[key] = value
	history.PinID = value.ID
	m.productVersionPinHistory[value.ProductID] = append(m.productVersionPinHistory[value.ProductID], history)
	if _, err := m.bumpProductCatalogRevisionLocked(value.ProductID, now); err != nil {
		if existed {
			values[key] = previous
		} else {
			delete(values, key)
		}
		m.productVersionPinHistory[value.ProductID] = m.productVersionPinHistory[value.ProductID][:len(m.productVersionPinHistory[value.ProductID])-1]
		return model.ProductVersionPin{}, err
	}
	return value, nil
}

func (m *Memory) DeleteProductVersionPin(_ context.Context, productID, id string, history model.ProductVersionPinHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productVersionPins[productID]
	if !ok {
		return ErrNotFound
	}
	for key, value := range values {
		if value.ID == id {
			delete(values, key)
			m.productVersionPinHistory[productID] = append(m.productVersionPinHistory[productID], history)
			if _, err := m.bumpProductCatalogRevisionLocked(productID, time.Now().UTC()); err != nil {
				values[key] = value
				m.productVersionPinHistory[productID] = m.productVersionPinHistory[productID][:len(m.productVersionPinHistory[productID])-1]
				return err
			}
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) ProductVersionPinHistory(_ context.Context, productID string) ([]model.ProductVersionPinHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.productVersionPinHistory[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := append([]model.ProductVersionPinHistory(nil), values...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) AppendProductVersionPinHistory(_ context.Context, value model.ProductVersionPinHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return ErrNotFound
	}
	m.productVersionPinHistory[value.ProductID] = append(m.productVersionPinHistory[value.ProductID], value)
	return nil
}

func (m *Memory) ProductInstallations(_ context.Context, productID string) ([]model.ProductInstallation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.productInstallations[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ProductInstallation, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) ProductInstallation(_ context.Context, productID, id string) (model.ProductInstallation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.productInstallations[productID][id]
	if !ok {
		return model.ProductInstallation{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) ProductInstallationByExternalID(_ context.Context, productID, externalID string) (model.ProductInstallation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.productInstallations[productID] {
		if value.ExternalID == externalID {
			return value, nil
		}
	}
	return model.ProductInstallation{}, ErrNotFound
}

func (m *Memory) SaveProductInstallation(_ context.Context, value model.ProductInstallation, expected int64) (model.ProductInstallation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productInstallations[value.ProductID]
	if !ok {
		return model.ProductInstallation{}, ErrNotFound
	}
	for id, current := range values {
		if current.ExternalID == value.ExternalID && id != value.ID {
			return model.ProductInstallation{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := values[value.ID]
	if exists {
		if current.Revision != expected {
			return model.ProductInstallation{}, ErrConflict
		}
		value.Revision, value.CreatedAt = current.Revision+1, current.CreatedAt
	} else {
		if expected != 0 {
			return model.ProductInstallation{}, ErrConflict
		}
		value.Revision, value.CreatedAt = 1, now
	}
	value.UpdatedAt = now
	values[value.ID] = value
	if _, err := m.bumpProductCatalogRevisionLocked(value.ProductID, now); err != nil {
		if exists {
			values[value.ID] = current
		} else {
			delete(values, value.ID)
		}
		return model.ProductInstallation{}, err
	}
	return value, nil
}

func cloneProductDefinition(value model.ProductDefinition) model.ProductDefinition {
	encoded, _ := json.Marshal(value)
	var cloned model.ProductDefinition
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func cloneProductBuild(value model.ProductBuild) model.ProductBuild {
	encoded, _ := json.Marshal(value)
	var cloned model.ProductBuild
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func (m *Memory) ProductDefinition(_ context.Context, productID string) (model.ProductDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.productDefinitions[productID]
	if !ok {
		return model.ProductDefinition{}, ErrNotFound
	}
	return cloneProductDefinition(value), nil
}

func (m *Memory) SaveProductDefinition(_ context.Context, value model.ProductDefinition, expected int64) (model.ProductDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.ProductDefinition{}, ErrNotFound
	}
	now := time.Now().UTC()
	current, exists := m.productDefinitions[value.ProductID]
	if exists {
		if current.Revision != expected {
			return model.ProductDefinition{}, ErrConflict
		}
		value.Revision = current.Revision + 1
		value.CreatedAt = current.CreatedAt
	} else {
		if expected != 0 {
			return model.ProductDefinition{}, ErrConflict
		}
		value.Revision = 1
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	m.productDefinitions[value.ProductID] = cloneProductDefinition(value)
	return cloneProductDefinition(value), nil
}

func (m *Memory) ProductBuilds(_ context.Context, productID string) ([]model.ProductBuild, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.productBuilds[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ProductBuild, 0, len(values))
	for _, value := range values {
		result = append(result, cloneProductBuild(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) ProductBuild(_ context.Context, productID, id string) (model.ProductBuild, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.productBuilds[productID][id]
	if !ok {
		return model.ProductBuild{}, ErrNotFound
	}
	return cloneProductBuild(value), nil
}

func (m *Memory) CreateProductBuild(_ context.Context, value model.ProductBuild) (model.ProductBuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.ProductBuild{}, ErrNotFound
	}
	if m.productBuilds[value.ProductID] == nil {
		m.productBuilds[value.ProductID] = make(map[string]model.ProductBuild)
	}
	if _, ok := m.productBuilds[value.ProductID][value.ID]; ok {
		return model.ProductBuild{}, ErrConflict
	}
	m.productBuilds[value.ProductID][value.ID] = cloneProductBuild(value)
	return cloneProductBuild(value), nil
}

func (m *Memory) MarkProductBuildPublished(_ context.Context, productID, id string) (model.ProductBuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.productBuilds[productID][id]
	if !ok {
		return model.ProductBuild{}, ErrNotFound
	}
	value.State = "published"
	m.productBuilds[productID][id] = cloneProductBuild(value)
	return cloneProductBuild(value), nil
}
