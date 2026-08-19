package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

type Memory struct {
	mu                       sync.RWMutex
	orgs                     map[string]model.Organisation
	products                 map[string]model.Product
	productVersions          map[string]map[string]model.ProductVersion
	productVersionPins       map[string]map[string]model.ProductVersionPin
	productVersionPinHistory map[string][]model.ProductVersionPinHistory
	productInstallations     map[string]map[string]model.ProductInstallation
	productDefinitions       map[string]model.ProductDefinition
	productBuilds            map[string]map[string]model.ProductBuild
	envs                     map[string]map[string]model.Environment
	sources                  map[string]map[string]model.Source
	packages                 map[string]map[string]model.Package
	secrets                  map[string]model.Secret
	tools                    map[string]map[string]model.Tool
	mcpConnections           map[string]map[string]model.MCPConnection
	mcpGrants                map[string]map[string]model.MCPUserGrant
	mcpAuthStates            map[string]model.MCPAuthorizationState
	providers                map[string]map[string]model.Provider
	projects                 map[string]map[string]model.Project
	leases                   map[string]map[string]model.CredentialLease
	integrationRuns          map[string]map[string]model.IntegrationRun
	llmProfiles              map[string]map[string]model.LLMProfile
	knowledge                map[string][]model.KnowledgeRecord
	crawls                   map[string][]model.CrawlJob
	audit                    []model.AuditEvent
	analytics                []model.AnalyticsEvent
	setupDone                bool
	roots                    map[string]auth.RootAccount
	rootEmail                map[string]string
	sessions                 map[string]auth.SessionRecord
	idps                     map[string]identity.VendorConfig
	oauthState               map[string]identity.OAuthState
	oauthCodes               map[string]identity.OAuthCode
	accessTokens             map[string]identity.AccessToken
}

func NewMemory() *Memory {
	now := time.Now().UTC()
	organisation := model.Organisation{ID: "org_acme", Name: "Acme", Slug: "acme", Revision: 1, CreatedAt: now, UpdatedAt: now}
	product := model.Product{ID: "prod_acme", OrganisationID: "org_acme", Name: "Acme Platform", Slug: "acme", Description: "Build voice and messaging integrations with Acme APIs, SDKs, documentation, and managed tools.", DefaultVersionPolicy: "latest", CatalogRevision: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
	environment := model.Environment{ID: "env_prod", OrganisationID: organisation.ID, ProductID: product.ID, Name: "Production", Slug: "production", IsProduction: true, Revision: 1, CreatedAt: now, UpdatedAt: now}
	sources := map[string]model.Source{
		"src_docs": {ID: "src_docs", OrganisationID: "org_acme", ProductID: product.ID, Name: "Developer documentation", Kind: "website", Location: "https://docs.acme.dev", Visibility: model.VisibilityPrivate, Published: true, Revision: 1, CreatedAt: now, UpdatedAt: now},
		"src_api":  {ID: "src_api", OrganisationID: "org_acme", ProductID: product.ID, Name: "Platform API", Kind: "openapi", Location: "git://api/openapi.yaml", Visibility: model.VisibilityPrivate, Published: true, Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	packages := map[string]model.Package{
		"pkg_node": {ID: "pkg_node", OrganisationID: "org_acme", ProductID: product.ID, Name: "@acme/node", Ecosystem: "npm", Version: "2.4.1", Mode: "proxy", Visibility: model.VisibilityPrivate, Published: true, Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	return &Memory{
		orgs:                     map[string]model.Organisation{organisation.ID: organisation},
		products:                 map[string]model.Product{product.ID: product},
		productVersions:          map[string]map[string]model.ProductVersion{product.ID: {}},
		productVersionPins:       map[string]map[string]model.ProductVersionPin{product.ID: {}},
		productVersionPinHistory: map[string][]model.ProductVersionPinHistory{product.ID: {}},
		productInstallations:     map[string]map[string]model.ProductInstallation{product.ID: {}},
		productDefinitions:       make(map[string]model.ProductDefinition),
		productBuilds:            map[string]map[string]model.ProductBuild{product.ID: {}},
		envs:                     map[string]map[string]model.Environment{product.ID: {environment.ID: environment}},
		sources:                  map[string]map[string]model.Source{product.ID: sources},
		packages:                 map[string]map[string]model.Package{product.ID: packages},
		secrets:                  make(map[string]model.Secret),
		tools:                    map[string]map[string]model.Tool{product.ID: {}},
		mcpConnections:           map[string]map[string]model.MCPConnection{product.ID: {}},
		mcpGrants:                make(map[string]map[string]model.MCPUserGrant),
		mcpAuthStates:            make(map[string]model.MCPAuthorizationState),
		providers:                map[string]map[string]model.Provider{product.ID: {}},
		projects:                 map[string]map[string]model.Project{product.ID: {}},
		leases:                   map[string]map[string]model.CredentialLease{product.ID: {}},
		integrationRuns:          map[string]map[string]model.IntegrationRun{product.ID: {}},
		llmProfiles:              map[string]map[string]model.LLMProfile{product.ID: {}},
		knowledge: map[string][]model.KnowledgeRecord{product.ID: {
			{ID: "doc_api_keys", ProductID: product.ID, SourceID: "src_docs", Title: "Create an API key", Text: "Create an API key in the Acme dashboard under Developer settings. Store it server-side and rotate it regularly.", URL: "https://docs.acme.dev/api-keys", Visibility: model.VisibilityPrivate, Published: true},
			{ID: "doc_internal", ProductID: product.ID, SourceID: "src_api", Title: "Internal administration", Text: "Private operator-only administration reference.", URL: "https://docs.acme.dev/internal", Visibility: model.VisibilityPrivate, Published: true},
		}},
		crawls:       make(map[string][]model.CrawlJob),
		roots:        make(map[string]auth.RootAccount),
		rootEmail:    make(map[string]string),
		sessions:     make(map[string]auth.SessionRecord),
		idps:         make(map[string]identity.VendorConfig),
		oauthState:   make(map[string]identity.OAuthState),
		oauthCodes:   make(map[string]identity.OAuthCode),
		accessTokens: make(map[string]identity.AccessToken),
	}
}

func (m *Memory) Ping(context.Context) error { return nil }

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
	m.packages[value.ID] = make(map[string]model.Package)
	m.knowledge[value.ID] = nil
	m.envs[value.ID] = make(map[string]model.Environment)
	m.tools[value.ID] = make(map[string]model.Tool)
	m.mcpConnections[value.ID] = make(map[string]model.MCPConnection)
	m.providers[value.ID] = make(map[string]model.Provider)
	m.projects[value.ID] = make(map[string]model.Project)
	m.leases[value.ID] = make(map[string]model.CredentialLease)
	m.integrationRuns[value.ID] = make(map[string]model.IntegrationRun)
	m.llmProfiles[value.ID] = make(map[string]model.LLMProfile)
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
	value, ok := m.products[productID]
	if !ok {
		return 0, ErrNotFound
	}
	value.CatalogRevision++
	value.UpdatedAt = time.Now().UTC()
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

func (m *Memory) SaveProductVersionPin(_ context.Context, value model.ProductVersionPin, expected int64) (model.ProductVersionPin, error) {
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
	if current, ok := values[key]; ok {
		if current.Revision != expected {
			return model.ProductVersionPin{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		if expected != 0 {
			return model.ProductVersionPin{}, ErrConflict
		}
		value.Revision, value.CreatedAt = 1, now
	}
	value.UpdatedAt = now
	values[key] = value
	return value, nil
}

func (m *Memory) DeleteProductVersionPin(_ context.Context, productID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.productVersionPins[productID]
	if !ok {
		return ErrNotFound
	}
	for key, value := range values {
		if value.ID == id {
			delete(values, key)
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

func sortedSources(values map[string]model.Source) []model.Source {
	result := make([]model.Source, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (m *Memory) Sources(_ context.Context, productID string) ([]model.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.sources[productID]
	if !ok {
		return nil, ErrNotFound
	}
	return sortedSources(values), nil
}

func (m *Memory) Source(_ context.Context, productID, id string) (model.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sources[productID][id]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateSource(_ context.Context, value model.Source) (model.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Source{}, ErrNotFound
	}
	value.Visibility = model.VisibilityPrivate
	value.Published = false
	value.Quarantined = false
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.sources[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) UpdateSource(_ context.Context, value model.Source, expected int64) (model.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.sources[value.ProductID][value.ID]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Source{}, ErrConflict
	}
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.sources[value.ProductID][value.ID] = value
	for i := range m.knowledge[value.ProductID] {
		if m.knowledge[value.ProductID][i].SourceID == value.ID {
			m.knowledge[value.ProductID][i].Visibility = value.Visibility
		}
	}
	return value, nil
}

func (m *Memory) PublishSource(_ context.Context, productID, sourceID string, expected int64) (model.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.sources[productID][sourceID]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.Source{}, ErrConflict
	}
	if value.Quarantined {
		return model.Source{}, ErrConflict
	}
	value.Published = true
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.sources[productID][sourceID] = value
	for index := range m.knowledge[productID] {
		if m.knowledge[productID][index].SourceID == sourceID {
			m.knowledge[productID][index].Published = true
			m.knowledge[productID][index].Visibility = value.Visibility
		}
	}
	return value, nil
}

func (m *Memory) CrawlJobs(_ context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.sources[productID][sourceID]; !ok {
		return nil, ErrNotFound
	}
	values := m.crawls[sourceID]
	result := make([]model.CrawlJob, len(values))
	copy(result, values)
	sort.Slice(result, func(i, j int) bool { return result[i].QueuedAt.After(result[j].QueuedAt) })
	return result, nil
}

func (m *Memory) CreateCrawlJob(_ context.Context, value model.CrawlJob) (model.CrawlJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[value.ProductID][value.SourceID]; !ok {
		return model.CrawlJob{}, ErrNotFound
	}
	for _, current := range m.crawls[value.SourceID] {
		if current.State == "queued" || current.State == "running" {
			return model.CrawlJob{}, ErrConflict
		}
	}
	value.State = "queued"
	value.Attempt = 1
	value.QueuedAt = time.Now().UTC()
	m.crawls[value.SourceID] = append(m.crawls[value.SourceID], value)
	return value, nil
}

func sortedPackages(values map[string]model.Package) []model.Package {
	result := make([]model.Package, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (m *Memory) Packages(_ context.Context, productID string) ([]model.Package, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.packages[productID]
	if !ok {
		return nil, ErrNotFound
	}
	return sortedPackages(values), nil
}

func (m *Memory) Package(_ context.Context, productID, id string) (model.Package, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.packages[productID][id]
	if !ok {
		return model.Package{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreatePackage(_ context.Context, value model.Package) (model.Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Package{}, ErrNotFound
	}
	for _, current := range m.packages[value.ProductID] {
		if current.Ecosystem == value.Ecosystem && current.Name == value.Name && current.Version == value.Version {
			return model.Package{}, ErrConflict
		}
	}
	value.Visibility = model.VisibilityPrivate
	value.Published = false
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.packages[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) UpdatePackage(_ context.Context, value model.Package, expected int64) (model.Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.packages[value.ProductID][value.ID]
	if !ok {
		return model.Package{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Package{}, ErrConflict
	}
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.packages[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) PublishPackage(_ context.Context, productID, packageID string, expected int64) (model.Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.packages[productID][packageID]
	if !ok {
		return model.Package{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.Package{}, ErrConflict
	}
	value.Published = true
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.packages[productID][packageID] = value
	return value, nil
}

func (m *Memory) CreateSecret(_ context.Context, value model.Secret) (model.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.secrets {
		if current.OrganisationID == value.OrganisationID && current.Name == value.Name {
			return model.Secret{}, ErrConflict
		}
	}
	value.CreatedAt = time.Now().UTC()
	m.secrets[value.ID] = cloneSecret(value)
	return cloneSecret(value), nil
}

func (m *Memory) Secret(_ context.Context, organisationID, id string) (model.Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.secrets[id]
	if !ok || value.OrganisationID != organisationID {
		return model.Secret{}, ErrNotFound
	}
	return cloneSecret(value), nil
}

func (m *Memory) Tools(_ context.Context, productID string, publishedOnly bool) ([]model.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.tools[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		if !publishedOnly || value.State == "published" {
			result = append(result, cloneTool(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Namespace+result[i].Name < result[j].Namespace+result[j].Name })
	return result, nil
}

func (m *Memory) Tool(_ context.Context, productID, id string) (model.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.tools[productID][id]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	return cloneTool(value), nil
}

func (m *Memory) CreateTool(_ context.Context, value model.Tool) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Tool{}, ErrNotFound
	}
	for _, current := range m.tools[value.ProductID] {
		if current.Namespace == value.Namespace && current.Name == value.Name {
			return model.Tool{}, ErrConflict
		}
	}
	value.State = "draft"
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) UpdateImportedTool(_ context.Context, value model.Tool, expected int64) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tools[value.ProductID][value.ID]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if current.Revision != expected || current.BackendKind != "mcp" || current.State == "published" {
		return model.Tool{}, ErrConflict
	}
	value.State = current.State
	value.Revision = current.Revision + 1
	value.CreatedAt = current.CreatedAt
	value.UpdatedAt = time.Now().UTC()
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) MarkImportedToolDrift(_ context.Context, productID, id string, drifted bool) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.tools[productID][id]
	if !ok || value.BackendKind != "mcp" {
		return model.Tool{}, ErrNotFound
	}
	value.UpstreamDrifted = drifted
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.tools[productID][id] = cloneTool(value)
	return cloneTool(value), nil
}

func (m *Memory) PublishTool(_ context.Context, productID, id string, expected int64, _ string) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.tools[productID][id]
	if !ok {
		return model.Tool{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.Tool{}, ErrConflict
	}
	value.State = "published"
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.tools[productID][id] = cloneTool(value)
	return cloneTool(value), nil
}

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

func (m *Memory) MCPUserGrant(_ context.Context, connectionID, subjectID string) (model.MCPUserGrant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.mcpGrants[connectionID][subjectID]
	if !ok || value.RevokedAt != nil {
		return model.MCPUserGrant{}, ErrNotFound
	}
	return cloneMCPGrant(value), nil
}

func (m *Memory) SaveMCPUserGrant(_ context.Context, value model.MCPUserGrant) (model.MCPUserGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mcpGrants[value.ConnectionID] == nil {
		m.mcpGrants[value.ConnectionID] = make(map[string]model.MCPUserGrant)
	}
	if current, ok := m.mcpGrants[value.ConnectionID][value.SubjectID]; ok {
		value.ID = current.ID
		value.CreatedAt = current.CreatedAt
	} else {
		value.CreatedAt = time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	m.mcpGrants[value.ConnectionID][value.SubjectID] = cloneMCPGrant(value)
	return cloneMCPGrant(value), nil
}

func (m *Memory) CreateMCPAuthorizationState(_ context.Context, value model.MCPAuthorizationState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(value.Digest)
	if _, exists := m.mcpAuthStates[key]; exists {
		return ErrConflict
	}
	m.mcpAuthStates[key] = cloneMCPAuthorizationState(value)
	return nil
}

func (m *Memory) ConsumeMCPAuthorizationState(_ context.Context, digest []byte) (model.MCPAuthorizationState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.mcpAuthStates[key]
	if !ok || time.Now().UTC().After(value.ExpiresAt) {
		delete(m.mcpAuthStates, key)
		return model.MCPAuthorizationState{}, ErrNotFound
	}
	delete(m.mcpAuthStates, key)
	return cloneMCPAuthorizationState(value), nil
}

func (m *Memory) Providers(_ context.Context, productID string) ([]model.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Provider, 0, len(m.providers[productID]))
	for _, value := range m.providers[productID] {
		result = append(result, cloneProvider(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) Provider(_ context.Context, productID, id string) (model.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.providers[productID][id]
	if !ok {
		return model.Provider{}, ErrNotFound
	}
	return cloneProvider(value), nil
}

func (m *Memory) CreateProvider(_ context.Context, value model.Provider) (model.Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Provider{}, ErrNotFound
	}
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	m.providers[value.ProductID][value.ID] = cloneProvider(value)
	return cloneProvider(value), nil
}

func (m *Memory) Projects(_ context.Context, productID string) ([]model.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Project, 0, len(m.projects[productID]))
	for _, value := range m.projects[productID] {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) Project(_ context.Context, productID, id string) (model.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.projects[productID][id]
	if !ok {
		return model.Project{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateProject(_ context.Context, value model.Project) (model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.projects[value.ProductID] {
		if existing.ProviderID == value.ProviderID && existing.IdempotencyKey == value.IdempotencyKey {
			return existing, nil
		}
	}
	value.CreatedAt, value.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	m.projects[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) CredentialLeases(_ context.Context, productID string) ([]model.CredentialLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.CredentialLease, 0, len(m.leases[productID]))
	for _, value := range m.leases[productID] {
		value.Scopes = append([]string(nil), value.Scopes...)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) CredentialLease(_ context.Context, productID, id string) (model.CredentialLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.leases[productID][id]
	if !ok {
		return model.CredentialLease{}, ErrNotFound
	}
	value.Scopes = append([]string(nil), value.Scopes...)
	return value, nil
}

func (m *Memory) CreateCredentialLease(_ context.Context, value model.CredentialLease) (model.CredentialLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.leases[value.ProductID] {
		if value.IdempotencyKey != "" && existing.ProviderID == value.ProviderID && existing.IdempotencyKey == value.IdempotencyKey {
			return existing, nil
		}
	}
	value.CreatedAt = time.Now().UTC()
	value.Scopes = append([]string(nil), value.Scopes...)
	m.leases[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) RevokeCredentialLease(_ context.Context, productID, id string, revokedAt time.Time) (model.CredentialLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.leases[productID][id]
	if !ok {
		return model.CredentialLease{}, ErrNotFound
	}
	value.RevokedAt = &revokedAt
	m.leases[productID][id] = value
	return value, nil
}

func (m *Memory) IntegrationRuns(_ context.Context, productID string) ([]model.IntegrationRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.integrationRuns[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationRun, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.After(result[j].StartedAt) })
	return result, nil
}

func (m *Memory) IntegrationRun(_ context.Context, productID, id string) (model.IntegrationRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.integrationRuns[productID][id]
	if !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateIntegrationRun(_ context.Context, value model.IntegrationRun) (model.IntegrationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	if _, exists := m.integrationRuns[value.ProductID][value.ID]; exists {
		return model.IntegrationRun{}, ErrConflict
	}
	value.State = "running"
	value.StartedAt = time.Now().UTC()
	m.integrationRuns[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) CompleteIntegrationRun(_ context.Context, productID, id string, reported, validated *bool, failureCode string, finishedAt time.Time) (model.IntegrationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.integrationRuns[productID][id]
	if !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	if value.FinishedAt != nil {
		return model.IntegrationRun{}, ErrConflict
	}
	value.ReportedSuccess, value.ValidatedSuccess = reported, validated
	value.FailureCode, value.FinishedAt = failureCode, &finishedAt
	value.State = "failed"
	if validated != nil && *validated {
		value.State = "succeeded"
	}
	m.integrationRuns[productID][id] = value
	return value, nil
}

func (m *Memory) LLMProfiles(_ context.Context, productID string) ([]model.LLMProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.LLMProfile, 0, len(m.llmProfiles[productID]))
	for _, value := range m.llmProfiles[productID] {
		value.Hardening = append([]byte(nil), value.Hardening...)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Role < result[j].Role })
	return result, nil
}

func (m *Memory) SaveLLMProfile(_ context.Context, value model.LLMProfile) (model.LLMProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.LLMProfile{}, ErrNotFound
	}
	if current, ok := m.llmProfiles[value.ProductID][value.Role]; ok {
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		value.Revision, value.CreatedAt = 1, time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	value.Hardening = append([]byte(nil), value.Hardening...)
	m.llmProfiles[value.ProductID][value.Role] = value
	return value, nil
}

func (m *Memory) PublicKnowledge(_ context.Context, productID, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published || record.Visibility != model.VisibilityPublic {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(record.Title+" "+record.Text), query) {
			result = append(result, record)
		}
	}
	return result, nil
}

func (m *Memory) PrivateKnowledge(_ context.Context, productID, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(record.Title+" "+record.Text), query) {
			result = append(result, record)
		}
	}
	return result, nil
}

func (m *Memory) AppendAnalytics(_ context.Context, event model.AnalyticsEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	event.Dimensions = cloneMap(event.Dimensions)
	m.analytics = append(m.analytics, event)
	return nil
}

func (m *Memory) ProductVersionActivity(_ context.Context, productID, versionID string, since time.Time) (model.ProductVersionActivity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var value model.ProductVersionActivity
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) || event.Dimensions["product_version_id"] != versionID {
			continue
		}
		switch event.EventName {
		case "mcp.request":
			value.Requests++
		case "tool.called":
			value.ToolCalls++
		}
	}
	return value, nil
}

func (m *Memory) LLMTokensUsed(_ context.Context, productID, role string, since time.Time) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total int64
	for _, event := range m.analytics {
		if event.ProductID == productID && event.EventName == "llm.tokens" && !event.CreatedAt.Before(since) && event.Dimensions["role"] == role {
			total += int64(event.Value)
		}
	}
	return total, nil
}

func (m *Memory) AnalyticsSummary(_ context.Context, productID string, since time.Time) (model.AnalyticsSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := model.AnalyticsSummary{Since: since, GeneratedAt: time.Now().UTC(), Channels: map[string]int64{"private_mcp": 0, "public_mcp": 0, "private_widget": 0, "public_widget": 0}, Versions: map[string]int64{}, Funnel: map[string]int64{"connector_authorized": 0, "run_started": 0, "capability_resolved": 0, "package_acquired": 0, "credentials_issued": 0, "implementation_validated": 0, "success_reported": 0}}
	actors := map[string]bool{}
	daily := map[string]int64{}
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) {
			continue
		}
		if event.ActorPseudonym != "" {
			actors[event.ActorPseudonym] = true
		}
		switch event.EventName {
		case "mcp.request":
			value.MCPRequests++
			channel, _ := event.Dimensions["channel"].(string)
			value.Channels[channel]++
			daily[event.CreatedAt.UTC().Format("2006-01-02")]++
			if version, _ := event.Dimensions["product_version"].(string); version != "" {
				value.Versions[version]++
			}
		case "tool.called":
			value.ToolCalls++
		case "package.downloaded":
			value.PackageDownloads++
		}
		if _, ok := value.Funnel[event.EventName]; ok {
			value.Funnel[event.EventName]++
		}
	}
	value.ActiveDevelopers = int64(len(actors))
	authorized := map[string]bool{}
	for _, token := range m.accessTokens {
		if token.ProductID == productID && !token.CreatedAt.Before(since) {
			authorized[token.Issuer+"\x00"+token.Subject] = true
		}
	}
	value.AuthorizedUsers = int64(len(authorized))
	for _, run := range m.integrationRuns[productID] {
		if run.StartedAt.Before(since) {
			continue
		}
		value.IntegrationRuns++
		if run.ValidatedSuccess != nil {
			value.ValidatedRuns++
			if *run.ValidatedSuccess {
				value.ValidatedSuccess++
			}
		}
	}
	if value.ValidatedRuns > 0 {
		value.FirstPassRate = float64(value.ValidatedSuccess) * 100 / float64(value.ValidatedRuns)
	}
	for date, count := range daily {
		value.DailyRequests = append(value.DailyRequests, model.AnalyticsPoint{Date: date, Count: count})
	}
	sort.Slice(value.DailyRequests, func(i, j int) bool { return value.DailyRequests[i].Date < value.DailyRequests[j].Date })
	return value, nil
}

func (m *Memory) AppendAudit(_ context.Context, event model.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, event)
	return nil
}

func (m *Memory) AuditEvents(_ context.Context, organisationID string) ([]model.AuditEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AuditEvent, 0)
	for _, event := range m.audit {
		if event.OrganisationID == organisationID {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *Memory) SetupCompleted(_ context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setupDone, nil
}

func (m *Memory) CreateInitialRoot(_ context.Context, account auth.RootAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setupDone || len(m.roots) != 0 {
		return ErrConflict
	}
	account.Email = strings.ToLower(account.Email)
	m.roots[account.UserID] = cloneRoot(account)
	m.rootEmail[account.Email] = account.UserID
	m.setupDone = true
	return nil
}

func (m *Memory) CreateRoot(_ context.Context, account auth.RootAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	account.Email = strings.ToLower(account.Email)
	if _, exists := m.rootEmail[account.Email]; exists {
		return ErrConflict
	}
	m.roots[account.UserID] = cloneRoot(account)
	m.rootEmail[account.Email] = account.UserID
	return nil
}

func (m *Memory) RevokeRoot(_ context.Context, userID string, revokedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.roots[userID]
	if !ok {
		return ErrNotFound
	}
	active := 0
	for _, value := range m.roots {
		if value.RevokedAt == nil {
			active++
		}
	}
	if account.RevokedAt == nil && active <= 1 {
		return auth.ErrLastRoot
	}
	account.RevokedAt = &revokedAt
	m.roots[userID] = account
	for key, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, key)
		}
	}
	return nil
}

func (m *Memory) RootByEmail(_ context.Context, email string) (auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.rootEmail[strings.ToLower(email)]
	if !ok {
		return auth.RootAccount{}, ErrNotFound
	}
	return cloneRoot(m.roots[id]), nil
}

func (m *Memory) RootByID(_ context.Context, id string) (auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	account, ok := m.roots[id]
	if !ok {
		return auth.RootAccount{}, ErrNotFound
	}
	return cloneRoot(account), nil
}

func (m *Memory) RootAccounts(_ context.Context) ([]auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]auth.RootAccount, 0, len(m.roots))
	for _, account := range m.roots {
		result = append(result, cloneRoot(account))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) CreateSession(_ context.Context, session auth.SessionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roots[session.UserID]; !ok {
		return ErrNotFound
	}
	m.sessions[hex.EncodeToString(session.TokenDigest)] = cloneSession(session)
	return nil
}

func (m *Memory) SessionByDigest(_ context.Context, digest []byte) (auth.SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[hex.EncodeToString(digest)]
	if !ok {
		return auth.SessionRecord{}, ErrNotFound
	}
	return cloneSession(session), nil
}

func (m *Memory) DeleteSession(_ context.Context, digest []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hex.EncodeToString(digest))
	return nil
}

func (m *Memory) VendorIdentity(_ context.Context, productID string) (identity.VendorConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.idps[productID]
	if !ok {
		return identity.VendorConfig{}, ErrNotFound
	}
	value.Scopes = append([]string(nil), value.Scopes...)
	value.AllowedRedirectURIs = append([]string(nil), value.AllowedRedirectURIs...)
	return value, nil
}

func (m *Memory) SaveVendorIdentity(_ context.Context, value identity.VendorConfig) (identity.VendorConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return identity.VendorConfig{}, ErrNotFound
	}
	if current, ok := m.idps[value.ProductID]; ok {
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		value.Revision = 1
		value.CreatedAt = time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	value.Scopes = append([]string(nil), value.Scopes...)
	value.AllowedRedirectURIs = append([]string(nil), value.AllowedRedirectURIs...)
	m.idps[value.ProductID] = value
	return value, nil
}

func (m *Memory) CreateOAuthState(_ context.Context, value identity.OAuthState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauthState[hex.EncodeToString(value.Digest)] = cloneOAuthState(value)
	return nil
}

func (m *Memory) ConsumeOAuthState(_ context.Context, digest []byte) (identity.OAuthState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.oauthState[key]
	if !ok {
		return identity.OAuthState{}, ErrNotFound
	}
	delete(m.oauthState, key)
	return cloneOAuthState(value), nil
}

func (m *Memory) CreateOAuthCode(_ context.Context, value identity.OAuthCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauthCodes[hex.EncodeToString(value.Digest)] = cloneOAuthCode(value)
	return nil
}

func (m *Memory) ConsumeOAuthCode(_ context.Context, digest []byte) (identity.OAuthCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.oauthCodes[key]
	if !ok {
		return identity.OAuthCode{}, ErrNotFound
	}
	delete(m.oauthCodes, key)
	return cloneOAuthCode(value), nil
}

func (m *Memory) CreateAccessToken(_ context.Context, value identity.AccessToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accessTokens[hex.EncodeToString(value.Digest)] = cloneAccessToken(value)
	return nil
}

func (m *Memory) AccessTokenByDigest(_ context.Context, digest []byte) (identity.AccessToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.accessTokens[hex.EncodeToString(digest)]
	if !ok {
		return identity.AccessToken{}, ErrNotFound
	}
	return cloneAccessToken(value), nil
}

func cloneRoot(account auth.RootAccount) auth.RootAccount {
	account.TOTPSecretCiphertext = append([]byte(nil), account.TOTPSecretCiphertext...)
	account.RecoveryCodeDigests = make([][]byte, len(account.RecoveryCodeDigests))
	for index, digest := range account.RecoveryCodeDigests {
		account.RecoveryCodeDigests[index] = append([]byte(nil), digest...)
	}
	return account
}

func cloneSession(session auth.SessionRecord) auth.SessionRecord {
	session.TokenDigest = append([]byte(nil), session.TokenDigest...)
	session.CSRFDigest = append([]byte(nil), session.CSRFDigest...)
	return session
}

func cloneSecret(value model.Secret) model.Secret {
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	value.Nonce = append([]byte(nil), value.Nonce...)
	return value
}

func cloneTool(value model.Tool) model.Tool {
	value.InputSchema = append([]byte(nil), value.InputSchema...)
	value.OutputSchema = append([]byte(nil), value.OutputSchema...)
	value.AuthorizationPolicy = append([]byte(nil), value.AuthorizationPolicy...)
	value.UpstreamAnnotations = append([]byte(nil), value.UpstreamAnnotations...)
	return value
}

func cloneMCPConnection(value model.MCPConnection) model.MCPConnection {
	value.Scopes = append([]string(nil), value.Scopes...)
	value.Config = append([]byte(nil), value.Config...)
	if value.LastSyncedAt != nil {
		lastSynced := *value.LastSyncedAt
		value.LastSyncedAt = &lastSynced
	}
	return value
}

func cloneMCPGrant(value model.MCPUserGrant) model.MCPUserGrant {
	value.Scopes = append([]string(nil), value.Scopes...)
	if value.RevokedAt != nil {
		revoked := *value.RevokedAt
		value.RevokedAt = &revoked
	}
	return value
}

func cloneMCPAuthorizationState(value model.MCPAuthorizationState) model.MCPAuthorizationState {
	value.Digest = append([]byte(nil), value.Digest...)
	return value
}

func cloneProvider(value model.Provider) model.Provider {
	value.Config = append([]byte(nil), value.Config...)
	return value
}

func cloneOAuthState(value identity.OAuthState) identity.OAuthState {
	value.Digest = append([]byte(nil), value.Digest...)
	return value
}

func cloneOAuthCode(value identity.OAuthCode) identity.OAuthCode {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Entitlements = cloneEntitlements(value.Entitlements)
	return value
}

func cloneAccessToken(value identity.AccessToken) identity.AccessToken {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Entitlements = cloneEntitlements(value.Entitlements)
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

func cloneEntitlements(value map[string]bool) map[string]bool {
	result := make(map[string]bool, len(value))
	for key, enabled := range value {
		result[key] = enabled
	}
	return result
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
