package store

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type Memory struct {
	mu                               sync.RWMutex
	developerAssets                  *memoryDeveloperAssets
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
	sdkReferences                    map[string]map[string]model.SDKReference
	grantDefinitions                 map[string]model.GrantDefinition
	authorizationPoints              map[string]map[string]model.AuthorizationPoint
	integrationToolLinks             map[string]map[string]model.IntegrationToolBinding
	products                         map[string]model.Product
	envs                             map[string]map[string]model.Environment
	sources                          map[string]map[string]model.Source
	sourcePublications               map[string]map[string]model.SourcePublication
	publicationDocuments             map[string]map[string]bool
	crawlReviewDocuments             map[string][]model.CrawlReviewDocument
	secrets                          map[string]model.Secret
	tools                            map[string]map[string]model.Tool
	nativePluginState                map[nativePluginStateScope]map[string]nativeplugin.StateValue
	toolRuntimeTargets               map[string]map[int64][]model.ToolRuntimeTarget
	toolTestConfirmations            map[string]model.ToolTestConfirmation
	toolTestConfirmationUses         map[string]time.Time
	managedOperationConfirmations    map[string]model.ManagedOperationConfirmation
	managedOperationConfirmationUses map[string]time.Time
	toolTestRuns                     []model.ToolTestRun
	mcpConnections                   map[string]map[string]model.MCPConnection
	reportSubmissions                map[string]map[string]model.ReportSubmission
	aiProviderConnections            map[string]map[string]model.AIProviderConnection
	aiWorkloadProfiles               map[string]map[string]model.AIWorkloadProfile
	aiPromptStates                   map[string]map[string]model.AIPromptState
	aiBudgetReservations             map[string]model.AIBudgetReservation
	aiBudgetUsed                     map[string]int64
	aiUsage                          []model.AIUsageEvent
	integrationAnalyses              map[string]map[string]model.IntegrationAnalysis
	recipes                          map[string]map[string]model.Recipe
	recipeRevisions                  map[string]map[string]model.RecipeRevision
	knowledge                        map[string][]model.KnowledgeRecord
	crawls                           map[string][]model.CrawlJob
	audit                            []model.AuditEvent
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
	product := model.Product{ID: "prod_acme", OrganisationID: "org_acme", Name: "Acme Platform", Slug: "acme", Description: "Build voice and messaging integrations with Acme APIs, SDKs, documentation, and managed tools.", CatalogRevision: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
	deployment := model.Deployment{ID: product.ID, OrganisationID: product.OrganisationID, Name: product.Name, Slug: product.Slug, Description: product.Description, CatalogRevision: product.CatalogRevision, Revision: product.Revision, CreatedAt: now, UpdatedAt: now}
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
		developerAssets:          newMemoryDeveloperAssets(),
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
		sdkReferences:            make(map[string]map[string]model.SDKReference),
		grantDefinitions:         make(map[string]model.GrantDefinition),
		authorizationPoints:      make(map[string]map[string]model.AuthorizationPoint),
		integrationToolLinks:     make(map[string]map[string]model.IntegrationToolBinding),
		products:                 map[string]model.Product{product.ID: product},
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
		nativePluginState:                make(map[nativePluginStateScope]map[string]nativeplugin.StateValue),
		toolRuntimeTargets:               make(map[string]map[int64][]model.ToolRuntimeTarget),
		toolTestConfirmations:            make(map[string]model.ToolTestConfirmation),
		toolTestConfirmationUses:         make(map[string]time.Time),
		managedOperationConfirmations:    make(map[string]model.ManagedOperationConfirmation),
		managedOperationConfirmationUses: make(map[string]time.Time),
		mcpConnections:                   map[string]map[string]model.MCPConnection{product.ID: {}},
		reportSubmissions:                map[string]map[string]model.ReportSubmission{product.ID: {}},
		aiProviderConnections:            map[string]map[string]model.AIProviderConnection{product.ID: {}},
		aiWorkloadProfiles:               map[string]map[string]model.AIWorkloadProfile{product.ID: {}},
		aiPromptStates:                   map[string]map[string]model.AIPromptState{product.ID: {}},
		aiBudgetReservations:             make(map[string]model.AIBudgetReservation),
		aiBudgetUsed:                     make(map[string]int64),
		integrationAnalyses:              map[string]map[string]model.IntegrationAnalysis{product.ID: {}},
		recipes:                          map[string]map[string]model.Recipe{product.ID: {}},
		recipeRevisions:                  make(map[string]map[string]model.RecipeRevision),
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
