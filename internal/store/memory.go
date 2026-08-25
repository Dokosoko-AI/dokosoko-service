package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

type Memory struct {
	mu                               sync.RWMutex
	orgs                             map[string]model.Organisation
	deployment                       model.Deployment
	hasDeployment                    bool
	integrations                     map[string]model.Integration
	integrationRevisions             map[string]map[string]model.IntegrationRevision
	runtimeConnections               map[string]model.RuntimeServiceConnection
	runtimeConnectionHistory         map[string]map[string]model.RuntimeServiceConnectionRevision
	runtimeCredentialSets            map[string]model.RuntimeCredentialSet
	runtimeCredentialHistory         map[string]map[string]model.RuntimeCredentialVersion
	resourceSets                     map[string]model.ResourceSet
	resourceSetRevisions             map[string]map[string]model.ResourceSetRevision
	integrationResourceLinks         map[string]map[string]model.IntegrationResourceLink
	packageArtifacts                 map[string]model.PackageArtifact
	packageReleases                  map[string]map[string]model.PackageRelease
	integrationPackageLinks          map[string]map[string]model.IntegrationPackageBinding
	grantDefinitions                 map[string]model.GrantDefinition
	authorizationPoints              map[string]map[string]model.AuthorizationPoint
	integrationToolLinks             map[string]map[string]model.IntegrationToolBinding
	accessDefinitions                map[string]model.AccessDefinition
	accessConnections                map[string]model.AccessConnection
	integrationAccessLinks           map[string]map[string]bool
	accessInstances                  map[string]model.AccessInstance
	instanceIntegrationLinks         map[string]map[string]bool
	accessCredentials                map[string]model.AccessCredential
	backendConnections               map[string]model.BackendConnection
	supportRoutes                    map[string]model.SupportRoute
	integrationSupportRoutes         map[string]string
	products                         map[string]model.Product
	productVersions                  map[string]map[string]model.ProductVersion
	productVersionPins               map[string]map[string]model.ProductVersionPin
	productVersionPinHistory         map[string][]model.ProductVersionPinHistory
	productInstallations             map[string]map[string]model.ProductInstallation
	productDefinitions               map[string]model.ProductDefinition
	productBuilds                    map[string]map[string]model.ProductBuild
	envs                             map[string]map[string]model.Environment
	sources                          map[string]map[string]model.Source
	sourcePublications               map[string]map[string]model.SourcePublication
	publicationDocuments             map[string]map[string]bool
	crawlReviewDocuments             map[string][]model.CrawlReviewDocument
	secrets                          map[string]model.Secret
	tools                            map[string]map[string]model.Tool
	toolRuntimeTargets               map[string]map[int64][]model.ToolRuntimeTarget
	toolTestConfirmations            map[string]model.ToolTestConfirmation
	toolTestConfirmationUses         map[string]time.Time
	managedOperationConfirmations    map[string]model.ManagedOperationConfirmation
	managedOperationConfirmationUses map[string]time.Time
	toolTestRuns                     []model.ToolTestRun
	mcpConnections                   map[string]map[string]model.MCPConnection
	mcpGrants                        map[string]map[string]model.MCPUserGrant
	mcpAuthStates                    map[string]model.MCPAuthorizationState
	providers                        map[string]map[string]model.Provider
	projects                         map[string]map[string]model.Project
	leases                           map[string]map[string]model.CredentialLease
	integrationRuns                  map[string]map[string]model.IntegrationRun
	reportSubmissions                map[string]map[string]model.ReportSubmission
	llmProfiles                      map[string]map[string]model.LLMProfile
	aiProviderConnections            map[string]map[string]model.AIProviderConnection
	aiWorkloadProfiles               map[string]map[string]model.AIWorkloadProfile
	aiBudgetReservations             map[string]model.AIBudgetReservation
	aiBudgetUsed                     map[string]int64
	aiUsage                          []model.AIUsageEvent
	integrationAnalyses              map[string]map[string]model.IntegrationAnalysis
	recipes                          map[string]map[string]model.Recipe
	recipeRevisions                  map[string]map[string]model.RecipeRevision
	aiJobs                           map[string]map[string]model.AIJob
	widgets                          map[string]model.Widget
	widgetSecrets                    map[string]model.WidgetSecret
	widgetSecretDigests              map[string]string
	widgetBootstraps                 map[string]model.WidgetBootstrap
	widgetSessions                   map[string]model.WidgetSession
	widgetAgentMessages              map[string][]model.WidgetAgentMessage
	knowledge                        map[string][]model.KnowledgeRecord
	crawls                           map[string][]model.CrawlJob
	audit                            []model.AuditEvent
	analytics                        []model.AnalyticsEvent
	setupDone                        bool
	roots                            map[string]auth.RootAccount
	rootEmail                        map[string]string
	sessions                         map[string]auth.SessionRecord
	idps                             map[string]identity.ProviderConfig
	identityTests                    map[string]identity.ProviderTest
	oauthClients                     map[string]identity.OAuthClient
	customerAccounts                 map[string]identity.CustomerAccount
	oauthState                       map[string]identity.OAuthState
	oauthCodes                       map[string]identity.OAuthCode
	accessTokens                     map[string]identity.AccessToken
}

func NewMemory() *Memory {
	now := time.Now().UTC()
	organisation := model.Organisation{ID: "org_acme", Name: "Acme", Slug: "acme", Revision: 1, CreatedAt: now, UpdatedAt: now}
	product := model.Product{ID: "prod_acme", OrganisationID: "org_acme", Name: "Acme Platform", Slug: "acme", Description: "Build voice and messaging integrations with Acme APIs, SDKs, documentation, and managed tools.", DefaultVersionPolicy: "latest", CatalogRevision: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
	deployment := model.Deployment{ID: product.ID, OrganisationID: product.OrganisationID, Name: product.Name, Slug: product.Slug, Description: product.Description, DefaultReleasePolicy: product.DefaultVersionPolicy, CatalogRevision: product.CatalogRevision, Revision: product.Revision, CreatedAt: now, UpdatedAt: now}
	environment := model.Environment{ID: "env_prod", OrganisationID: organisation.ID, ProductID: product.ID, Name: "Production", Slug: "production", IsProduction: true, Revision: 1, CreatedAt: now, UpdatedAt: now}
	sources := map[string]model.Source{
		"src_docs": {ID: "src_docs", OrganisationID: "org_acme", ProductID: product.ID, Name: "Developer documentation", Kind: "website", Location: "https://docs.acme.dev", Visibility: model.VisibilityPrivate, Published: true, Revision: 1, CreatedAt: now, UpdatedAt: now},
		"src_api":  {ID: "src_api", OrganisationID: "org_acme", ProductID: product.ID, Name: "Platform API", Kind: "openapi", Location: "git://api/openapi.yaml", Visibility: model.VisibilityPrivate, Published: true, Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	docsCrawl := model.CrawlJob{ID: "crawl_docs_seed", OrganisationID: organisation.ID, ProductID: product.ID, SourceID: "src_docs", State: "succeeded", DiscoveredCount: 1, FetchedCount: 1, ChangedCount: 1, QueuedAt: now, FinishedAt: &now}
	apiCrawl := model.CrawlJob{ID: "crawl_api_seed", OrganisationID: organisation.ID, ProductID: product.ID, SourceID: "src_api", State: "succeeded", DiscoveredCount: 1, FetchedCount: 1, ChangedCount: 1, QueuedAt: now.Add(-time.Second), FinishedAt: &now}
	docsPublication := model.SourcePublication{ID: "pub_docs_seed", OrganisationID: organisation.ID, ProductID: product.ID, SourceID: "src_docs", CrawlJobID: docsCrawl.ID, Revision: 1, Visibility: model.VisibilityPrivate, ContentHash: "sha256:" + strings.Repeat("1", 64), DocumentCount: 1, ReviewedBy: "seed", ReviewedAt: now, PublishedAt: now}
	apiPublication := model.SourcePublication{ID: "pub_api_seed", OrganisationID: organisation.ID, ProductID: product.ID, SourceID: "src_api", CrawlJobID: apiCrawl.ID, Revision: 1, Visibility: model.VisibilityPrivate, ContentHash: "sha256:" + strings.Repeat("2", 64), DocumentCount: 1, ReviewedBy: "seed", ReviewedAt: now, PublishedAt: now}
	return &Memory{
		orgs:                     map[string]model.Organisation{organisation.ID: organisation},
		deployment:               deployment,
		hasDeployment:            true,
		integrations:             make(map[string]model.Integration),
		integrationRevisions:     make(map[string]map[string]model.IntegrationRevision),
		runtimeConnections:       make(map[string]model.RuntimeServiceConnection),
		runtimeConnectionHistory: make(map[string]map[string]model.RuntimeServiceConnectionRevision),
		runtimeCredentialSets:    make(map[string]model.RuntimeCredentialSet),
		runtimeCredentialHistory: make(map[string]map[string]model.RuntimeCredentialVersion),
		resourceSets:             make(map[string]model.ResourceSet),
		resourceSetRevisions:     make(map[string]map[string]model.ResourceSetRevision),
		integrationResourceLinks: make(map[string]map[string]model.IntegrationResourceLink),
		packageArtifacts:         make(map[string]model.PackageArtifact),
		packageReleases:          make(map[string]map[string]model.PackageRelease),
		integrationPackageLinks:  make(map[string]map[string]model.IntegrationPackageBinding),
		grantDefinitions:         make(map[string]model.GrantDefinition),
		authorizationPoints:      make(map[string]map[string]model.AuthorizationPoint),
		integrationToolLinks:     make(map[string]map[string]model.IntegrationToolBinding),
		accessDefinitions:        make(map[string]model.AccessDefinition),
		accessConnections:        make(map[string]model.AccessConnection),
		integrationAccessLinks:   make(map[string]map[string]bool),
		accessInstances:          make(map[string]model.AccessInstance),
		instanceIntegrationLinks: make(map[string]map[string]bool),
		accessCredentials:        make(map[string]model.AccessCredential),
		backendConnections:       make(map[string]model.BackendConnection),
		supportRoutes:            make(map[string]model.SupportRoute),
		integrationSupportRoutes: make(map[string]string),
		products:                 map[string]model.Product{product.ID: product},
		productVersions:          map[string]map[string]model.ProductVersion{product.ID: {}},
		productVersionPins:       map[string]map[string]model.ProductVersionPin{product.ID: {}},
		productVersionPinHistory: map[string][]model.ProductVersionPinHistory{product.ID: {}},
		productInstallations:     map[string]map[string]model.ProductInstallation{product.ID: {}},
		productDefinitions:       make(map[string]model.ProductDefinition),
		productBuilds:            map[string]map[string]model.ProductBuild{product.ID: {}},
		envs:                     map[string]map[string]model.Environment{product.ID: {environment.ID: environment}},
		sources:                  map[string]map[string]model.Source{product.ID: sources},
		sourcePublications: map[string]map[string]model.SourcePublication{product.ID: {
			docsPublication.ID: docsPublication,
			apiPublication.ID:  apiPublication,
		}},
		publicationDocuments: map[string]map[string]bool{
			docsPublication.ID: {"doc_api_keys": true},
			apiPublication.ID:  {"doc_internal": true},
		},
		crawlReviewDocuments: map[string][]model.CrawlReviewDocument{
			docsCrawl.ID: {{ID: "doc_api_keys", CrawlJobID: docsCrawl.ID, SnapshotID: "snapshot_docs_seed", Title: "Create an API key", CanonicalURL: "https://docs.acme.dev/api-keys", State: "published", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("1", 64), Changed: true}},
			apiCrawl.ID:  {{ID: "doc_internal", CrawlJobID: apiCrawl.ID, SnapshotID: "snapshot_api_seed", Title: "Internal administration", CanonicalURL: "https://docs.acme.dev/internal", State: "published", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("2", 64), Changed: true}},
		},
		secrets:                          make(map[string]model.Secret),
		tools:                            map[string]map[string]model.Tool{product.ID: {}},
		toolRuntimeTargets:               make(map[string]map[int64][]model.ToolRuntimeTarget),
		toolTestConfirmations:            make(map[string]model.ToolTestConfirmation),
		toolTestConfirmationUses:         make(map[string]time.Time),
		managedOperationConfirmations:    make(map[string]model.ManagedOperationConfirmation),
		managedOperationConfirmationUses: make(map[string]time.Time),
		mcpConnections:                   map[string]map[string]model.MCPConnection{product.ID: {}},
		mcpGrants:                        make(map[string]map[string]model.MCPUserGrant),
		mcpAuthStates:                    make(map[string]model.MCPAuthorizationState),
		providers:                        map[string]map[string]model.Provider{product.ID: {}},
		projects:                         map[string]map[string]model.Project{product.ID: {}},
		leases:                           map[string]map[string]model.CredentialLease{product.ID: {}},
		integrationRuns:                  map[string]map[string]model.IntegrationRun{product.ID: {}},
		reportSubmissions:                map[string]map[string]model.ReportSubmission{product.ID: {}},
		llmProfiles:                      map[string]map[string]model.LLMProfile{product.ID: {}},
		aiProviderConnections:            map[string]map[string]model.AIProviderConnection{product.ID: {}},
		aiWorkloadProfiles:               map[string]map[string]model.AIWorkloadProfile{product.ID: {}},
		aiBudgetReservations:             make(map[string]model.AIBudgetReservation),
		aiBudgetUsed:                     make(map[string]int64),
		integrationAnalyses:              map[string]map[string]model.IntegrationAnalysis{product.ID: {}},
		recipes:                          map[string]map[string]model.Recipe{product.ID: {}},
		recipeRevisions:                  make(map[string]map[string]model.RecipeRevision),
		aiJobs:                           map[string]map[string]model.AIJob{product.ID: {}},
		widgets:                          make(map[string]model.Widget),
		widgetSecrets:                    make(map[string]model.WidgetSecret),
		widgetSecretDigests:              make(map[string]string),
		widgetBootstraps:                 make(map[string]model.WidgetBootstrap),
		widgetSessions:                   make(map[string]model.WidgetSession),
		widgetAgentMessages:              make(map[string][]model.WidgetAgentMessage),
		knowledge: map[string][]model.KnowledgeRecord{product.ID: {
			{ID: "doc_api_keys", ProductID: product.ID, SourceID: "src_docs", Title: "Create an API key", Text: "Create an API key in the Acme dashboard under Developer settings. Store it server-side and rotate it regularly.", URL: "https://docs.acme.dev/api-keys", Visibility: model.VisibilityPrivate, Published: true},
			{ID: "doc_internal", ProductID: product.ID, SourceID: "src_api", Title: "Internal administration", Text: "Private operator-only administration reference.", URL: "https://docs.acme.dev/internal", Visibility: model.VisibilityPrivate, Published: true},
		}},
		crawls: map[string][]model.CrawlJob{
			docsCrawl.SourceID: {docsCrawl},
			apiCrawl.SourceID:  {apiCrawl},
		},
		roots:            make(map[string]auth.RootAccount),
		rootEmail:        make(map[string]string),
		sessions:         make(map[string]auth.SessionRecord),
		idps:             make(map[string]identity.ProviderConfig),
		identityTests:    make(map[string]identity.ProviderTest),
		oauthClients:     make(map[string]identity.OAuthClient),
		customerAccounts: make(map[string]identity.CustomerAccount),
		oauthState:       make(map[string]identity.OAuthState),
		oauthCodes:       make(map[string]identity.OAuthCode),
		accessTokens:     make(map[string]identity.AccessToken),
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

func (m *Memory) SourceReview(_ context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	source, ok := m.sources[productID][sourceID]
	if !ok {
		return model.SourceReview{}, ErrNotFound
	}
	jobs := append([]model.CrawlJob(nil), m.crawls[sourceID]...)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].QueuedAt.After(jobs[j].QueuedAt) || (jobs[i].QueuedAt.Equal(jobs[j].QueuedAt) && jobs[i].ID > jobs[j].ID)
	})
	var job model.CrawlJob
	for _, candidate := range jobs {
		if crawlJobID == "" || candidate.ID == crawlJobID {
			job = candidate
			break
		}
	}
	if job.ID == "" {
		return model.SourceReview{}, ErrNotFound
	}
	review := model.SourceReview{Source: source, CrawlJob: job, Documents: append([]model.CrawlReviewDocument(nil), m.crawlReviewDocuments[job.ID]...)}
	for _, publication := range m.sourcePublications[productID] {
		if publication.SourceID == sourceID && publication.CrawlJobID == job.ID {
			copy := publication
			review.Publication = &copy
			break
		}
	}
	return review, nil
}

func (m *Memory) SourcePublications(_ context.Context, productID, sourceID string) ([]model.SourcePublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.sources[productID][sourceID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.SourcePublication, 0)
	for _, value := range m.sourcePublications[productID] {
		if value.SourceID == sourceID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Revision > values[j].Revision })
	return values, nil
}

func (m *Memory) SourcePublication(_ context.Context, productID, publicationID string) (model.SourcePublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sourcePublications[productID][publicationID]
	if !ok {
		return model.SourcePublication{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) PublishSource(_ context.Context, productID, sourceID string, expected int64, publication model.SourcePublication, documentIDs []string) (model.Source, model.SourcePublication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.sources[productID][sourceID]
	if !ok {
		return model.Source{}, model.SourcePublication{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if value.Quarantined {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	jobs := append([]model.CrawlJob(nil), m.crawls[sourceID]...)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].QueuedAt.After(jobs[j].QueuedAt) || (jobs[i].QueuedAt.Equal(jobs[j].QueuedAt) && jobs[i].ID > jobs[j].ID)
	})
	if len(jobs) == 0 || jobs[0].ID != publication.CrawlJobID || jobs[0].FinishedAt == nil || (jobs[0].State != "review" && jobs[0].State != "succeeded") || jobs[0].FetchedCount == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	reviewDocs := make(map[string]model.CrawlReviewDocument, len(m.crawlReviewDocuments[publication.CrawlJobID]))
	for _, document := range m.crawlReviewDocuments[publication.CrawlJobID] {
		reviewDocs[document.ID] = document
	}
	selected := make([]model.CrawlReviewDocument, 0, len(documentIDs))
	seen := make(map[string]bool, len(documentIDs))
	for _, documentID := range documentIDs {
		document, exists := reviewDocs[documentID]
		if seen[documentID] || !exists || !docreview.SafeAssessment(document.State, document.InjectionIndicators) {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
		seen[documentID] = true
		selected = append(selected, document)
	}
	if len(documentIDs) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	lockedHash, err := docreview.PublicationContentHash(selected)
	if err != nil || publication.ContentHash != lockedHash {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	for _, existing := range m.sourcePublications[productID] {
		if existing.SourceID == sourceID && existing.CrawlJobID == publication.CrawlJobID {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
	}
	value.Published = true
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.sources[productID][sourceID] = value
	publication.OrganisationID = value.OrganisationID
	publication.ProductID = productID
	publication.SourceID = sourceID
	publication.Visibility = value.Visibility
	publication.DocumentCount = len(documentIDs)
	publication.Revision = 1
	for _, existing := range m.sourcePublications[productID] {
		if existing.SourceID == sourceID && existing.Revision >= publication.Revision {
			publication.Revision = existing.Revision + 1
		}
	}
	if m.sourcePublications[productID] == nil {
		m.sourcePublications[productID] = make(map[string]model.SourcePublication)
	}
	m.sourcePublications[productID][publication.ID] = publication
	m.publicationDocuments[publication.ID] = make(map[string]bool, len(documentIDs))
	for _, documentID := range documentIDs {
		m.publicationDocuments[publication.ID][documentID] = true
	}
	for index := range m.knowledge[productID] {
		if m.knowledge[productID][index].SourceID == sourceID && m.publicationDocuments[publication.ID][m.knowledge[productID][index].ID] {
			m.knowledge[productID][index].Published = true
			m.knowledge[productID][index].Visibility = value.Visibility
		}
	}
	return value, publication, nil
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
	sort.Slice(result, func(i, j int) bool {
		return result[i].QueuedAt.After(result[j].QueuedAt) || (result[i].QueuedAt.Equal(result[j].QueuedAt) && result[i].ID > result[j].ID)
	})
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

func (m *Memory) DeleteSecret(_ context.Context, organisationID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.secrets[id]
	if !ok || value.OrganisationID != organisationID {
		return ErrNotFound
	}
	delete(m.secrets, id)
	return nil
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
			result = append(result, m.enrichToolRuntimeTargetsLocked(value))
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
	return m.enrichToolRuntimeTargetsLocked(value), nil
}

func (m *Memory) CreateTool(_ context.Context, value model.Tool) (model.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Tool{}, ErrNotFound
	}
	var err error
	value.Scope, value.OwnerIntegrationID, err = normalizeToolOwnership(value.Scope, value.OwnerIntegrationID)
	if err != nil {
		return model.Tool{}, err
	}
	if value.Scope == model.ToolScopeAPI {
		owner, ok := m.integrations[value.OwnerIntegrationID]
		if !ok || owner.DeploymentID != value.ProductID || owner.OrganisationID != value.OrganisationID {
			return model.Tool{}, ErrConflict
		}
	}
	if value.RuntimeServiceConnectionID != "" {
		connection, ok := m.runtimeConnections[value.RuntimeServiceConnectionID]
		if !ok || value.Scope != model.ToolScopeAPI || value.BackendKind != "http" || value.APIConnectionID != "" || connection.IntegrationID != value.OwnerIntegrationID || connection.DeploymentID != value.ProductID || connection.State != "active" || value.HTTPPath == "" {
			return model.Tool{}, ErrConflict
		}
		current := false
		for _, revision := range m.runtimeConnectionHistory[connection.ID] {
			current = current || revision.Current
		}
		if !current {
			return model.Tool{}, ErrConflict
		}
	}
	for _, current := range m.tools[value.ProductID] {
		if current.Namespace == value.Namespace && current.Name == value.Name {
			return model.Tool{}, ErrConflict
		}
	}
	value.State = "draft"
	if value.BackendKind == "mcp" && len(value.UpstreamAuth) == 0 {
		value.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	}
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.tools[value.ProductID][value.ID] = cloneTool(value)
	return m.enrichToolRuntimeTargetsLocked(value), nil
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
	value.Scope, value.OwnerIntegrationID = current.Scope, current.OwnerIntegrationID
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
	if value.Revision != expected || value.State != "draft" {
		return model.Tool{}, ErrConflict
	}
	if value.RuntimeServiceConnectionID != "" {
		connection, ok := m.runtimeConnections[value.RuntimeServiceConnectionID]
		if !ok || connection.State != "active" || connection.IntegrationID != value.OwnerIntegrationID {
			return model.Tool{}, ErrConflict
		}
		targets := m.currentToolRuntimeTargetsLocked(connection.ID)
		if len(targets) == 0 {
			return model.Tool{}, ErrConflict
		}
		if m.toolRuntimeTargets[value.ID] == nil {
			m.toolRuntimeTargets[value.ID] = make(map[int64][]model.ToolRuntimeTarget)
		}
		m.toolRuntimeTargets[value.ID][expected+1] = cloneToolRuntimeTargets(targets)
	}
	value.State = "published"
	value.Revision++
	value.UpdatedAt = time.Now().UTC()
	m.tools[productID][id] = cloneTool(value)
	return m.enrichToolRuntimeTargetsLocked(value), nil
}

func (m *Memory) CreateToolTestConfirmation(_ context.Context, value model.ToolTestConfirmation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[value.ProductID][value.ToolID]
	if !ok || tool.Revision != value.ToolRevision || value.ID == "" || len(value.NonceDigest) != 32 || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	key := hex.EncodeToString(value.NonceDigest)
	if _, exists := m.toolTestConfirmations[key]; exists {
		return ErrConflict
	}
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	m.toolTestConfirmations[key] = value
	return nil
}

func (m *Memory) ConsumeToolTestConfirmation(_ context.Context, digest []byte, productID, toolID string, revision int64, argumentHash []byte, actorID, _ string, now time.Time) (model.ToolTestConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.toolTestConfirmations[hex.EncodeToString(digest)]
	if !ok || value.ProductID != productID || value.ToolID != toolID || value.ToolRevision != revision || value.ActorID != actorID || !bytes.Equal(value.ArgumentHash, argumentHash) || !now.Before(value.ExpiresAt) {
		return model.ToolTestConfirmation{}, ErrNotFound
	}
	tool, ok := m.tools[productID][toolID]
	if !ok || tool.Revision != revision {
		return model.ToolTestConfirmation{}, ErrConflict
	}
	if _, used := m.toolTestConfirmationUses[value.ID]; used {
		return model.ToolTestConfirmation{}, ErrConflict
	}
	m.toolTestConfirmationUses[value.ID] = now
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	return value, nil
}

func (m *Memory) CreateManagedOperationConfirmation(_ context.Context, value model.ManagedOperationConfirmation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok || value.ID == "" || value.OperationKey == "" || len(value.NonceDigest) != 32 || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	key := hex.EncodeToString(value.NonceDigest)
	if _, exists := m.managedOperationConfirmations[key]; exists {
		return ErrConflict
	}
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	m.managedOperationConfirmations[key] = value
	return nil
}

func (m *Memory) ConsumeManagedOperationConfirmation(_ context.Context, digest []byte, productID, operationKey string, argumentHash []byte, actorID, _ string, now time.Time) (model.ManagedOperationConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.managedOperationConfirmations[hex.EncodeToString(digest)]
	if !ok || value.ProductID != productID || value.OperationKey != operationKey || value.ActorID != actorID || !bytes.Equal(value.ArgumentHash, argumentHash) || !now.Before(value.ExpiresAt) {
		return model.ManagedOperationConfirmation{}, ErrNotFound
	}
	if _, used := m.managedOperationConfirmationUses[value.ID]; used {
		return model.ManagedOperationConfirmation{}, ErrConflict
	}
	m.managedOperationConfirmationUses[value.ID] = now
	value.NonceDigest = append([]byte(nil), value.NonceDigest...)
	value.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	return value, nil
}

func (m *Memory) DeleteExpiredToolTestData(_ context.Context, now time.Time, limit int) (int64, error) {
	limit = boundedToolTestCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	confirmationDeletes := 0
	for digest, value := range m.toolTestConfirmations {
		if confirmationDeletes >= limit {
			break
		}
		if now.Before(value.ExpiresAt) {
			continue
		}
		delete(m.toolTestConfirmations, digest)
		delete(m.toolTestConfirmationUses, value.ID)
		confirmationDeletes++
		deleted++
	}
	managedDeletes := 0
	for digest, value := range m.managedOperationConfirmations {
		if managedDeletes >= limit {
			break
		}
		if now.Before(value.ExpiresAt) {
			continue
		}
		delete(m.managedOperationConfirmations, digest)
		delete(m.managedOperationConfirmationUses, value.ID)
		managedDeletes++
		deleted++
	}
	runDeletes := 0
	kept := m.toolTestRuns[:0]
	for _, value := range m.toolTestRuns {
		if runDeletes < limit && !now.Before(value.ExpiresAt) {
			runDeletes++
			deleted++
			continue
		}
		kept = append(kept, value)
	}
	for index := len(kept); index < len(m.toolTestRuns); index++ {
		m.toolTestRuns[index] = model.ToolTestRun{}
	}
	m.toolTestRuns = kept
	return deleted, nil
}

func cloneJSONShape(value model.JSONShape) model.JSONShape {
	result := value
	if value.Properties != nil {
		result.Properties = make(map[string]model.JSONShape, len(value.Properties))
		for key, child := range value.Properties {
			result.Properties[key] = cloneJSONShape(child)
		}
	}
	if value.Items != nil {
		result.Items = make([]model.JSONShape, len(value.Items))
		for index, child := range value.Items {
			result.Items[index] = cloneJSONShape(child)
		}
	}
	return result
}

func cloneToolTestRun(value model.ToolTestRun) model.ToolTestRun {
	result := value
	result.ArgumentHash = append([]byte(nil), value.ArgumentHash...)
	result.RequestShape = cloneJSONShape(value.RequestShape)
	if value.ResponseShape != nil {
		shape := cloneJSONShape(*value.ResponseShape)
		result.ResponseShape = &shape
	}
	result.Findings = append([]model.ToolTestFinding(nil), value.Findings...)
	return result
}

func (m *Memory) AppendToolTestRun(_ context.Context, value model.ToolTestRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.tools[value.ProductID][value.ToolID]
	if !ok || value.ID == "" || len(value.ArgumentHash) != 32 {
		return ErrConflict
	}
	for _, current := range m.toolTestRuns {
		if current.ID == value.ID {
			return ErrConflict
		}
	}
	m.toolTestRuns = append(m.toolTestRuns, cloneToolTestRun(value))
	return nil
}

func (m *Memory) ToolTestRuns(_ context.Context, productID, toolID string, now time.Time) ([]model.ToolTestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.tools[productID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.ToolTestRun, 0)
	for _, value := range m.toolTestRuns {
		if value.ProductID == productID && (toolID == "" || value.ToolID == toolID) && now.Before(value.ExpiresAt) {
			result = append(result, cloneToolTestRun(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > 100 {
		result = result[:100]
	}
	return result, nil
}

func (m *Memory) ToolTestRun(_ context.Context, productID, toolID, runID string, now time.Time) (model.ToolTestRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.toolTestRuns {
		if value.ID == runID && value.ProductID == productID && value.ToolID == toolID && now.Before(value.ExpiresAt) {
			return cloneToolTestRun(value), nil
		}
	}
	return model.ToolTestRun{}, ErrNotFound
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

func cloneReportSubmission(value model.ReportSubmission) model.ReportSubmission {
	value.IdempotencyDigest = append([]byte(nil), value.IdempotencyDigest...)
	value.PayloadCiphertext = append([]byte(nil), value.PayloadCiphertext...)
	value.PayloadNonce = append([]byte(nil), value.PayloadNonce...)
	value.IntegrationSnapshot = append([]byte(nil), value.IntegrationSnapshot...)
	return value
}

func (m *Memory) ReportSubmissions(_ context.Context, productID, startingAfter string, limit int) ([]model.ReportSubmission, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.reportSubmissions[productID]
	if !ok {
		return nil, false, ErrNotFound
	}
	result := make([]model.ReportSubmission, 0, len(values))
	for _, value := range values {
		result = append(result, cloneReportSubmission(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	start := 0
	if startingAfter != "" {
		start = -1
		for index := range result {
			if result[index].ID == startingAfter {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, false, ErrNotFound
		}
	}
	result = result[start:]
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (m *Memory) ReportSubmission(_ context.Context, productID, id string) (model.ReportSubmission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.reportSubmissions[productID][id]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	return cloneReportSubmission(value), nil
}

func (m *Memory) CreateReportSubmission(_ context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.reportSubmissions[value.ProductID]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	for _, current := range values {
		if current.ActorPseudonym == value.ActorPseudonym && current.Kind == value.Kind && hex.EncodeToString(current.IdempotencyDigest) == hex.EncodeToString(value.IdempotencyDigest) {
			return cloneReportSubmission(current), nil
		}
	}
	if _, exists := values[value.ID]; exists {
		return model.ReportSubmission{}, ErrConflict
	}
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	values[value.ID] = cloneReportSubmission(value)
	return cloneReportSubmission(value), nil
}

func (m *Memory) ActivateHeldReportSubmissions(_ context.Context, productID, routeID, kind string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.reportSubmissions[productID]
	if !ok {
		return ErrNotFound
	}
	for id, value := range values {
		if value.SupportRouteID == routeID && value.Kind == kind && value.State == "held" && value.ExpiresAt.After(now) {
			value.State, value.NextAttemptAt, value.UpdatedAt = "pending", &now, now
			values[id] = value
		}
	}
	return nil
}

func (m *Memory) ClaimReportSubmissions(_ context.Context, now time.Time, limit int) ([]model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 25
	}
	stale := now.Add(-5 * time.Minute)
	result := make([]model.ReportSubmission, 0, limit)
	for productID, values := range m.reportSubmissions {
		for id, value := range values {
			ready := value.State == "pending" && (value.NextAttemptAt == nil || !value.NextAttemptAt.After(now))
			recoverable := value.State == "delivering" && value.DeliveryStartedAt != nil && value.DeliveryStartedAt.Before(stale)
			if (!ready && !recoverable) || !value.ExpiresAt.After(now) || len(result) >= limit {
				continue
			}
			value.State, value.DeliveryStartedAt, value.UpdatedAt = "delivering", &now, now
			value.Attempts++
			m.reportSubmissions[productID][id] = value
			result = append(result, cloneReportSubmission(value))
		}
	}
	return result, nil
}

func (m *Memory) UpdateReportSubmissionDelivery(_ context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.reportSubmissions[value.ProductID][value.ID]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	current.State = value.State
	current.Attempts = value.Attempts
	current.NextAttemptAt = value.NextAttemptAt
	current.DeliveryStartedAt = value.DeliveryStartedAt
	current.LastError = value.LastError
	current.ExternalID = value.ExternalID
	current.ExternalURL = value.ExternalURL
	current.DeliveredAt = value.DeliveredAt
	current.UpdatedAt = time.Now().UTC()
	m.reportSubmissions[value.ProductID][value.ID] = current
	return cloneReportSubmission(current), nil
}

func (m *Memory) RetryReportSubmission(_ context.Context, productID, id string, now time.Time) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.reportSubmissions[productID][id]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	if (value.State != "held" && value.State != "failed") || !value.ExpiresAt.After(now) {
		return model.ReportSubmission{}, ErrConflict
	}
	value.State, value.NextAttemptAt, value.DeliveryStartedAt = "pending", &now, nil
	value.LastError, value.UpdatedAt = "", now
	m.reportSubmissions[productID][id] = value
	return cloneReportSubmission(value), nil
}

func (m *Memory) DeleteExpiredReportSubmissions(_ context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for _, values := range m.reportSubmissions {
		for id, value := range values {
			if !value.ExpiresAt.After(now) {
				delete(values, id)
				deleted++
			}
		}
	}
	return deleted, nil
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

func (m *Memory) PublicKnowledge(_ context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	allowed := make(map[string]bool)
	for _, publicationID := range publicationIDs {
		publication, ok := m.sourcePublications[productID][publicationID]
		source, sourceOK := m.sources[productID][publication.SourceID]
		if !ok || !sourceOK || publication.Visibility != model.VisibilityPublic || source.Visibility != model.VisibilityPublic || !source.Published || source.Quarantined {
			continue
		}
		for documentID := range m.publicationDocuments[publicationID] {
			allowed[documentID] = true
		}
	}
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published || record.Visibility != model.VisibilityPublic || !allowed[record.ID] {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(record.Title+" "+record.Text), query) {
			result = append(result, record)
		}
	}
	return result, nil
}

func (m *Memory) PrivateKnowledge(_ context.Context, productID string, publicationIDs []string, query string) ([]model.KnowledgeRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	allowed := make(map[string]bool)
	for _, publicationID := range publicationIDs {
		if _, ok := m.sourcePublications[productID][publicationID]; !ok {
			continue
		}
		for documentID := range m.publicationDocuments[publicationID] {
			allowed[documentID] = true
		}
	}
	result := make([]model.KnowledgeRecord, 0)
	for _, record := range m.knowledge[productID] {
		if !record.Published || !allowed[record.ID] {
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
	value := model.AnalyticsSummary{Since: since, GeneratedAt: time.Now().UTC(), Channels: map[string]int64{"private_mcp": 0, "public_mcp": 0, "widget": 0}, Versions: map[string]int64{}, Funnel: map[string]int64{"connector_authorized": 0, "run_started": 0, "capability_resolved": 0, "credentials_issued": 0, "implementation_validated": 0, "success_reported": 0}}
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

func (m *Memory) RecipePopularity(_ context.Context, productID string, since time.Time) ([]model.RecipePopularity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	byID := map[string]model.RecipePopularity{}
	for _, event := range m.analytics {
		if event.ProductID != productID || event.CreatedAt.Before(since) || (event.EventName != "recipe.view" && event.EventName != "recipe.plan_selected") {
			continue
		}
		recipeID, _ := event.Dimensions["recipe_id"].(string)
		if recipeID == "" {
			continue
		}
		value := byID[recipeID]
		value.RecipeID = recipeID
		value.RecipeSlug, _ = event.Dimensions["recipe_slug"].(string)
		if event.EventName == "recipe.view" {
			value.Views++
		} else {
			value.PlanSelections++
		}
		byID[recipeID] = value
	}
	result := make([]model.RecipePopularity, 0, len(byID))
	for _, value := range byID {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PlanSelections == result[j].PlanSelections {
			if result[i].Views == result[j].Views {
				return result[i].RecipeSlug < result[j].RecipeSlug
			}
			return result[i].Views > result[j].Views
		}
		return result[i].PlanSelections > result[j].PlanSelections
	})
	return result, nil
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

func (m *Memory) IdentityProvider(_ context.Context, productID string) (identity.ProviderConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.idps[productID]
	if !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	value.Scopes = append([]string(nil), value.Scopes...)
	return value, nil
}

func (m *Memory) SaveIdentityProvider(_ context.Context, value identity.ProviderConfig) (identity.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.DeploymentID]; !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	if current, ok := m.idps[value.DeploymentID]; ok {
		if value.Revision != current.Revision {
			return identity.ProviderConfig{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		if value.Revision != 0 {
			return identity.ProviderConfig{}, ErrConflict
		}
		value.Revision = 1
		value.CreatedAt = time.Now().UTC()
	}
	value.UpdatedAt = time.Now().UTC()
	value.Scopes = append([]string(nil), value.Scopes...)
	m.idps[value.DeploymentID] = value
	return value, nil
}

func (m *Memory) DeleteIdentityProvider(_ context.Context, deploymentID string, expectedRevision int64) (identity.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.idps[deploymentID]
	if !ok {
		return identity.ProviderConfig{}, ErrNotFound
	}
	if current.Revision != expectedRevision || current.State != "disabled" {
		return identity.ProviderConfig{}, ErrConflict
	}
	secretIDs := map[string]bool{current.ClientSecretID: true}
	for key, value := range m.oauthState {
		if value.ProductID == deploymentID {
			delete(m.oauthState, key)
		}
	}
	for key, value := range m.oauthCodes {
		if value.ProductID == deploymentID {
			secretIDs[value.UpstreamAccessSecretID] = true
			delete(m.oauthCodes, key)
		}
	}
	for key, value := range m.accessTokens {
		if value.ProductID == deploymentID {
			secretIDs[value.UpstreamAccessSecretID] = true
			delete(m.accessTokens, key)
		}
	}
	for id, value := range m.identityTests {
		if value.DeploymentID == deploymentID {
			delete(m.identityTests, id)
		}
	}
	delete(m.idps, deploymentID)
	for secretID := range secretIDs {
		if secretID == "" {
			continue
		}
		referenced := false
		for _, provider := range m.idps {
			if provider.ClientSecretID == secretID {
				referenced = true
				break
			}
		}
		for _, code := range m.oauthCodes {
			if code.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		for _, token := range m.accessTokens {
			if token.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		if !referenced {
			delete(m.secrets, secretID)
		}
	}
	return current, nil
}

func (m *Memory) CreateIdentityProviderTest(_ context.Context, value identity.ProviderTest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return ErrNotFound
	}
	provider, ok := m.idps[value.DeploymentID]
	if !ok || provider.Revision != value.ConfigurationRevision || (provider.State != "disabled" && provider.State != "active") {
		return ErrConflict
	}
	if _, exists := m.identityTests[value.ID]; exists {
		return ErrConflict
	}
	for _, current := range m.identityTests {
		if bytes.Equal(current.StateDigest, value.StateDigest) {
			return ErrConflict
		}
	}
	m.identityTests[value.ID] = cloneIdentityProviderTest(value)
	return nil
}

func (m *Memory) IdentityProviderTest(_ context.Context, deploymentID, id string) (identity.ProviderTest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.identityTests[id]
	if !ok || value.DeploymentID != deploymentID {
		return identity.ProviderTest{}, ErrNotFound
	}
	return cloneIdentityProviderTest(value), nil
}

func (m *Memory) ClaimIdentityProviderTestByStateDigest(_ context.Context, digest []byte, now time.Time) (identity.ProviderTest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, value := range m.identityTests {
		if !bytes.Equal(value.StateDigest, digest) {
			continue
		}
		if value.Status != "pending" || value.CallbackClaimedAt != nil {
			return identity.ProviderTest{}, ErrConflict
		}
		if !value.ExpiresAt.After(now) {
			completedAt := now
			value.Status = "expired"
			value.FailureCode = "test_expired"
			value.UpstreamVerifier = ""
			value.Nonce = ""
			value.Subject = ""
			value.CustomerID = ""
			value.CompletedAt = &completedAt
			m.identityTests[id] = cloneIdentityProviderTest(value)
			return identity.ProviderTest{}, ErrConflict
		}
		claimedAt := now
		value.CallbackClaimedAt = &claimedAt
		m.identityTests[id] = cloneIdentityProviderTest(value)
		return cloneIdentityProviderTest(value), nil
	}
	return identity.ProviderTest{}, ErrNotFound
}

func (m *Memory) LatestIdentityProviderTest(_ context.Context, deploymentID string) (identity.ProviderTest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest identity.ProviderTest
	found := false
	for _, value := range m.identityTests {
		if value.DeploymentID != deploymentID {
			continue
		}
		if !found || value.CreatedAt.After(latest.CreatedAt) || value.CreatedAt.Equal(latest.CreatedAt) && value.ID > latest.ID {
			latest, found = value, true
		}
	}
	if !found {
		return identity.ProviderTest{}, ErrNotFound
	}
	return cloneIdentityProviderTest(latest), nil
}

func (m *Memory) CompleteIdentityProviderTest(_ context.Context, value identity.ProviderTest) (identity.ProviderTest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.identityTests[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return identity.ProviderTest{}, ErrNotFound
	}
	if current.Status != "pending" || current.ConfigurationRevision != value.ConfigurationRevision {
		return identity.ProviderTest{}, ErrConflict
	}
	current.Status = value.Status
	current.FailureCode = value.FailureCode
	current.Issuer = value.Issuer
	current.Subject = value.Subject
	current.CustomerID = value.CustomerID
	current.CompletedAt = value.CompletedAt
	current.UpstreamVerifier = ""
	current.Nonce = ""
	m.identityTests[current.ID] = cloneIdentityProviderTest(current)
	return cloneIdentityProviderTest(current), nil
}

func (m *Memory) ExpireIdentityProviderTests(_ context.Context, deploymentID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, current := range m.identityTests {
		if current.DeploymentID != deploymentID || current.ExpiresAt.After(now) {
			continue
		}
		claimActive := current.Status == "pending" && current.CallbackClaimedAt != nil && current.CallbackClaimedAt.Add(2*time.Minute).After(now)
		if claimActive {
			continue
		}
		if current.Status == "pending" {
			completedAt := now
			current.Status = "expired"
			current.FailureCode = "test_expired"
			current.UpstreamVerifier = ""
			current.Nonce = ""
			current.CompletedAt = &completedAt
		}
		current.Subject = ""
		current.CustomerID = ""
		m.identityTests[id] = cloneIdentityProviderTest(current)
	}
	return nil
}

func (m *Memory) OAuthClient(_ context.Context, deploymentID, clientID string) (identity.OAuthClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.oauthClients[clientID]
	if !ok || value.DeploymentID != deploymentID {
		return identity.OAuthClient{}, ErrNotFound
	}
	value.RedirectURIs = append([]string(nil), value.RedirectURIs...)
	return value, nil
}

func (m *Memory) CreateOAuthClient(_ context.Context, value identity.OAuthClient) (identity.OAuthClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID {
		return identity.OAuthClient{}, ErrNotFound
	}
	if current, ok := m.oauthClients[value.ClientID]; ok {
		if current.DeploymentID != value.DeploymentID || current.ClientName != value.ClientName || !slices.Equal(current.RedirectURIs, value.RedirectURIs) {
			return identity.OAuthClient{}, ErrConflict
		}
		current.RedirectURIs = append([]string(nil), current.RedirectURIs...)
		return current, nil
	}
	value.CreatedAt = time.Now().UTC()
	value.RedirectURIs = append([]string(nil), value.RedirectURIs...)
	m.oauthClients[value.ClientID] = value
	return value, nil
}

func (m *Memory) ResolveCustomerAccount(_ context.Context, value identity.CustomerAccount) (identity.CustomerAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, current := range m.customerAccounts {
		if current.ProductID == value.ProductID && current.Issuer == value.Issuer && current.ExternalID == value.ExternalID {
			current.LastAuthenticatedAt = value.LastAuthenticatedAt
			current.UpdatedAt = value.LastAuthenticatedAt
			m.customerAccounts[id] = current
			return current, nil
		}
	}
	if _, ok := m.products[value.ProductID]; !ok {
		return identity.CustomerAccount{}, ErrNotFound
	}
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = value.LastAuthenticatedAt, value.LastAuthenticatedAt
	m.customerAccounts[value.ID] = value
	return value, nil
}

func (m *Memory) CustomerAccounts(_ context.Context, productID, startingAfter string, limit int) ([]identity.CustomerAccount, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.products[productID]; !ok {
		return nil, false, ErrNotFound
	}
	result := make([]identity.CustomerAccount, 0)
	for _, value := range m.customerAccounts {
		if value.ProductID == productID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	start := 0
	if startingAfter != "" {
		start = -1
		for index := range result {
			if result[index].ID == startingAfter {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, false, ErrNotFound
		}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	result = result[start:]
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (m *Memory) CustomerAccount(_ context.Context, productID, id string) (identity.CustomerAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.customerAccounts[id]
	if !ok || value.ProductID != productID {
		return identity.CustomerAccount{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) UpdateCustomerAccount(_ context.Context, value identity.CustomerAccount, expected int64) (identity.CustomerAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.customerAccounts[value.ID]
	if !ok || current.ProductID != value.ProductID {
		return identity.CustomerAccount{}, ErrNotFound
	}
	if current.Revision != expected {
		return identity.CustomerAccount{}, ErrConflict
	}
	current.State, current.Revision, current.UpdatedAt = value.State, current.Revision+1, time.Now().UTC()
	m.customerAccounts[value.ID] = current
	return current, nil
}

func (m *Memory) CreateOAuthState(_ context.Context, value identity.OAuthState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
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
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
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
	provider, ok := m.idps[value.ProductID]
	if !ok || provider.State != "active" || provider.Revision != value.ProviderRevision {
		return ErrConflict
	}
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

func (m *Memory) DeleteStaleOAuthArtifacts(_ context.Context, productID string, now time.Time, limit int) (int64, error) {
	limit = boundedOAuthArtifactCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	activeProviderRevision := int64(0)
	if provider, ok := m.idps[productID]; ok && provider.State == "active" {
		activeProviderRevision = provider.Revision
	}

	var deleted int64
	statesDeleted := 0
	for key, value := range m.oauthState {
		if statesDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.oauthState, key)
			statesDeleted++
			deleted++
		}
	}

	secretIDs := make(map[string]bool)
	codesDeleted := 0
	for key, value := range m.oauthCodes {
		if codesDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || !value.AccessExpiresAt.After(now) || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.oauthCodes, key)
			secretIDs[value.UpstreamAccessSecretID] = true
			codesDeleted++
			deleted++
		}
	}

	tokensDeleted := 0
	for key, value := range m.accessTokens {
		if tokensDeleted >= limit {
			break
		}
		if value.ProductID == productID && (!value.ExpiresAt.After(now) || value.RevokedAt != nil || activeProviderRevision <= 0 || value.ProviderRevision != activeProviderRevision) {
			delete(m.accessTokens, key)
			secretIDs[value.UpstreamAccessSecretID] = true
			tokensDeleted++
			deleted++
		}
	}

	for secretID := range secretIDs {
		if secretID == "" {
			continue
		}
		referenced := false
		for _, value := range m.oauthCodes {
			if value.UpstreamAccessSecretID == secretID {
				referenced = true
				break
			}
		}
		if !referenced {
			for _, value := range m.accessTokens {
				if value.UpstreamAccessSecretID == secretID {
					referenced = true
					break
				}
			}
		}
		if secret, ok := m.secrets[secretID]; ok && !referenced && secret.Purpose == "vendor_delegated_access" {
			delete(m.secrets, secretID)
		}
	}

	product, ok := m.products[productID]
	if !ok {
		return deleted, ErrNotFound
	}
	orphansDeleted := 0
	orphanCutoff := now.Add(-identitySecretOrphanGrace)
	for secretID, secret := range m.secrets {
		if orphansDeleted >= limit || secret.OrganisationID != product.OrganisationID || secret.CreatedAt.After(orphanCutoff) {
			continue
		}
		referencedByOAuth := false
		for _, value := range m.oauthCodes {
			if value.UpstreamAccessSecretID == secretID {
				referencedByOAuth = true
				break
			}
		}
		if !referencedByOAuth {
			for _, value := range m.accessTokens {
				if value.UpstreamAccessSecretID == secretID {
					referencedByOAuth = true
					break
				}
			}
		}
		referencedByProvider := false
		for _, value := range m.idps {
			if value.ClientSecretID == secretID {
				referencedByProvider = true
				break
			}
		}
		vendorOrphan := secret.Purpose == "vendor_delegated_access" && !referencedByOAuth
		providerOrphan := secret.Purpose == "identity_provider_oidc_client" && !referencedByProvider
		if vendorOrphan || providerOrphan {
			delete(m.secrets, secretID)
			orphansDeleted++
			deleted++
		}
	}
	return deleted, nil
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
	value.CredentialPresent = value.CredentialID != ""
	value.InputSchema = append([]byte(nil), value.InputSchema...)
	value.OutputSchema = append([]byte(nil), value.OutputSchema...)
	value.UpstreamAuth = append([]byte(nil), value.UpstreamAuth...)
	value.RequestMapping = append([]byte(nil), value.RequestMapping...)
	value.ResponseMapping = append([]byte(nil), value.ResponseMapping...)
	value.RequestExample = append([]byte(nil), value.RequestExample...)
	value.ResponseExample = append([]byte(nil), value.ResponseExample...)
	value.AuthorizationPolicy = append([]byte(nil), value.AuthorizationPolicy...)
	value.UpstreamAnnotations = append([]byte(nil), value.UpstreamAnnotations...)
	value.RuntimeTargets = cloneToolRuntimeTargets(value.RuntimeTargets)
	return value
}

func cloneToolRuntimeTargets(values []model.ToolRuntimeTarget) []model.ToolRuntimeTarget {
	result := make([]model.ToolRuntimeTarget, len(values))
	copy(result, values)
	for index := range result {
		result[index].AuthConfig = append(json.RawMessage(nil), result[index].AuthConfig...)
	}
	return result
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

func cloneIdentityProviderTest(value identity.ProviderTest) identity.ProviderTest {
	value.StateDigest = append([]byte(nil), value.StateDigest...)
	if value.CompletedAt != nil {
		completed := *value.CompletedAt
		value.CompletedAt = &completed
	}
	if value.CallbackClaimedAt != nil {
		claimed := *value.CallbackClaimedAt
		value.CallbackClaimedAt = &claimed
	}
	return value
}

func cloneOAuthState(value identity.OAuthState) identity.OAuthState {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

func cloneOAuthCode(value identity.OAuthCode) identity.OAuthCode {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Scopes = append([]string(nil), value.Scopes...)
	value.Grants = cloneGrants(value.Grants)
	return value
}

func cloneAccessToken(value identity.AccessToken) identity.AccessToken {
	value.Digest = append([]byte(nil), value.Digest...)
	value.Grants = cloneGrants(value.Grants)
	value.Scopes = append([]string(nil), value.Scopes...)
	return value
}

func cloneGrants(value map[string]bool) map[string]bool {
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
