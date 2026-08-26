package store

import (
	"context"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
)

// NativePluginStateStore is the single host-owned state abstraction exposed
// to trusted native plugins. Plugins never receive a database handle.
type NativePluginStateStore interface {
	nativepluginstate.Backend
}

// DeploymentCatalogStore owns persistence operations for one cohesive application domain.
type DeploymentCatalogStore interface {
	Organisations(context.Context) ([]model.Organisation, error)
	CreateOrganisation(context.Context, model.Organisation) (model.Organisation, error)
	Deployment(context.Context) (model.Deployment, error)
	CreateDeployment(context.Context, model.Deployment) (model.Deployment, error)
	UpdateDeployment(context.Context, model.Deployment, int64) (model.Deployment, error)
	Integrations(context.Context, string) ([]model.Integration, error)
	Integration(context.Context, string, string) (model.Integration, error)
	CreateIntegration(context.Context, model.Integration) (model.Integration, error)
	UpdateIntegration(context.Context, model.Integration, int64) (model.Integration, error)
	IntegrationRevisions(context.Context, string) ([]model.IntegrationRevision, error)
	CreateIntegrationRevision(context.Context, model.IntegrationRevision) (model.IntegrationRevision, error)
}

// RuntimeServiceStore owns persistence operations for one cohesive application domain.
type RuntimeServiceStore interface {
	RuntimeServiceConnections(context.Context, string, string) ([]model.RuntimeServiceConnection, error)
	RuntimeServiceConnection(context.Context, string, string) (model.RuntimeServiceConnection, error)
	CreateRuntimeServiceConnection(context.Context, model.RuntimeServiceConnection) (model.RuntimeServiceConnection, error)
	UpdateRuntimeServiceConnection(context.Context, model.RuntimeServiceConnection, int64) (model.RuntimeServiceConnection, error)
	RuntimeServiceConnectionRevisions(context.Context, string, string) ([]model.RuntimeServiceConnectionRevision, error)
	CreateRuntimeServiceConnectionRevision(context.Context, model.RuntimeServiceConnectionRevision) (model.RuntimeServiceConnectionRevision, error)
	RuntimeCredentialSets(context.Context, string, string) ([]model.RuntimeCredentialSet, error)
	RuntimeCredentialSet(context.Context, string, string) (model.RuntimeCredentialSet, error)
	CreateRuntimeCredentialSet(context.Context, model.RuntimeCredentialSet) (model.RuntimeCredentialSet, error)
	UpdateRuntimeCredentialSet(context.Context, model.RuntimeCredentialSet, int64) (model.RuntimeCredentialSet, error)
	RuntimeCredentialVersions(context.Context, string) ([]model.RuntimeCredentialVersion, error)
	CreateRuntimeCredentialVersion(context.Context, model.RuntimeCredentialVersion) (model.RuntimeCredentialVersion, error)
	ActivateRuntimeCredentialVersion(context.Context, string, string, string, time.Time) (model.RuntimeCredentialVersion, error)
	RevokeRuntimeCredentialVersion(context.Context, string, string, string, time.Time) (model.RuntimeCredentialVersion, error)
	ResourceSets(context.Context, string, string) ([]model.ResourceSet, error)
	ResourceSet(context.Context, string, string) (model.ResourceSet, error)
	CreateResourceSet(context.Context, model.ResourceSet, model.ResourceSetRevision) (model.ResourceSet, error)
	UpdateResourceSet(context.Context, model.ResourceSet, model.ResourceSetRevision, int64) (model.ResourceSet, error)
	ResourceSetRevisions(context.Context, string) ([]model.ResourceSetRevision, error)
	IntegrationResourceLinks(context.Context, string) ([]model.IntegrationResourceLink, error)
	SaveIntegrationResourceLink(context.Context, model.IntegrationResourceLink) (model.IntegrationResourceLink, error)
	DeleteIntegrationResourceLink(context.Context, string, string) error
}

// IntegrationPolicyStore owns persistence operations for one cohesive application domain.
type IntegrationPolicyStore interface {
	SDKReferences(context.Context, string) ([]model.SDKReference, error)
	SDKReference(context.Context, string, string) (model.SDKReference, error)
	SaveSDKReference(context.Context, model.SDKReference, int64) (model.SDKReference, error)
	DeleteSDKReference(context.Context, string, string) error
	GrantDefinitions(context.Context, string) ([]model.GrantDefinition, error)
	GrantDefinition(context.Context, string, string) (model.GrantDefinition, error)
	SaveGrantDefinition(context.Context, model.GrantDefinition, int64) (model.GrantDefinition, error)
	AuthorizationPoints(context.Context, string) ([]model.AuthorizationPoint, error)
	AuthorizationPoint(context.Context, string, string) (model.AuthorizationPoint, error)
	SaveAuthorizationPoint(context.Context, model.AuthorizationPoint, int64) (model.AuthorizationPoint, error)
	IntegrationToolBindings(context.Context, string) ([]model.IntegrationToolBinding, error)
	SaveIntegrationToolBindings(context.Context, string, []model.IntegrationToolBinding) ([]model.IntegrationToolBinding, error)
}

// ProductCatalogStore owns persistence operations for one cohesive application domain.
type ProductCatalogStore interface {
	Products(context.Context, string) ([]model.Product, error)
	CreateProduct(context.Context, model.Product) (model.Product, error)
	Environments(context.Context, string) ([]model.Environment, error)
	CreateEnvironment(context.Context, model.Environment) (model.Environment, error)
	Product(context.Context, string) (model.Product, error)
	UpdateProduct(context.Context, model.Product, int64) (model.Product, error)
	BumpProductCatalogRevision(context.Context, string) (int64, error)
}

// KnowledgeStore owns persistence operations for one cohesive application domain.
type KnowledgeStore interface {
	Sources(context.Context, string) ([]model.Source, error)
	CreateSource(context.Context, model.Source) (model.Source, error)
	Source(context.Context, string, string) (model.Source, error)
	UpdateSource(context.Context, model.Source, int64) (model.Source, error)
	SourceReview(context.Context, string, string, string) (model.SourceReview, error)
	SourcePublications(context.Context, string, string) ([]model.SourcePublication, error)
	SourcePublication(context.Context, string, string) (model.SourcePublication, error)
	PublishSource(context.Context, string, string, int64, model.SourcePublication, []string) (model.Source, model.SourcePublication, error)
	CrawlJobs(context.Context, string, string) ([]model.CrawlJob, error)
	CreateCrawlJob(context.Context, model.CrawlJob) (model.CrawlJob, error)
	CreateSecret(context.Context, model.Secret) (model.Secret, error)
	Secret(context.Context, string, string) (model.Secret, error)
	DeleteSecret(context.Context, string, string) error
}

// ToolStore owns persistence operations for one cohesive application domain.
type ToolStore interface {
	Tools(context.Context, string, bool) ([]model.Tool, error)
	Tool(context.Context, string, string) (model.Tool, error)
	CreateTool(context.Context, model.Tool) (model.Tool, error)
	UpdateTool(context.Context, model.Tool, int64) (model.Tool, error)
	RetireTool(context.Context, string, string, int64) (model.Tool, error)
	UpdateImportedTool(context.Context, model.Tool, int64) (model.Tool, error)
	MarkImportedToolDrift(context.Context, string, string, bool) (model.Tool, error)
	StageNativeTool(context.Context, model.Tool, int64) (model.Tool, error)
	PublishTool(context.Context, string, string, int64, string) (model.Tool, error)
	CreateToolTestConfirmation(context.Context, model.ToolTestConfirmation) error
	ConsumeToolTestConfirmation(context.Context, []byte, string, string, int64, []byte, string, string, time.Time) (model.ToolTestConfirmation, error)
	CreateManagedOperationConfirmation(context.Context, model.ManagedOperationConfirmation) error
	ConsumeManagedOperationConfirmation(context.Context, []byte, string, string, []byte, string, string, time.Time) (model.ManagedOperationConfirmation, error)
	DeleteExpiredToolTestData(context.Context, time.Time, int) (int64, error)
	AppendToolTestRun(context.Context, model.ToolTestRun) error
	ToolTestRuns(context.Context, string, string, time.Time) ([]model.ToolTestRun, error)
	ToolTestRun(context.Context, string, string, string, time.Time) (model.ToolTestRun, error)
}

// MCPStore owns persistence operations for one cohesive application domain.
type MCPStore interface {
	MCPConnections(context.Context, string) ([]model.MCPConnection, error)
	MCPConnection(context.Context, string, string) (model.MCPConnection, error)
	CreateMCPConnection(context.Context, model.MCPConnection) (model.MCPConnection, error)
	UpdateMCPConnectionSync(context.Context, string, string, string, time.Time) (model.MCPConnection, error)
}

// ReportingStore owns persistence operations for one cohesive application domain.
type ReportingStore interface {
	ReportSubmissions(context.Context, string, string, int) ([]model.ReportSubmission, bool, error)
	ReportSubmission(context.Context, string, string) (model.ReportSubmission, error)
	CreateReportSubmission(context.Context, model.ReportSubmission) (model.ReportSubmission, error)
}

// AIRecipeStore owns persistence operations for one cohesive application domain.
type AIRecipeStore interface {
	AIProviderConnections(context.Context, string) ([]model.AIProviderConnection, error)
	AIProviderConnection(context.Context, string, string) (model.AIProviderConnection, error)
	SaveAIProviderConnection(context.Context, model.AIProviderConnection, int64) (model.AIProviderConnection, error)
	AIWorkloadProfiles(context.Context, string) ([]model.AIWorkloadProfile, error)
	AIWorkloadProfile(context.Context, string, string) (model.AIWorkloadProfile, error)
	SaveAIWorkloadProfile(context.Context, model.AIWorkloadProfile, int64) (model.AIWorkloadProfile, error)
	AIPromptStates(context.Context, string) ([]model.AIPromptState, error)
	AIPromptState(context.Context, string, string) (model.AIPromptState, error)
	SaveAIPromptStateAndAudit(context.Context, model.AIPromptState, int64, model.AuditEvent) (model.AIPromptState, error)
	ReserveAIBudget(context.Context, model.AIBudgetReservation, int64) (bool, error)
	FinishAIUsage(context.Context, string, model.AIUsageEvent) error
	AIUsageEvents(context.Context, string, time.Time) ([]model.AIUsageEvent, error)
	IntegrationAnalyses(context.Context, string) ([]model.IntegrationAnalysis, error)
	IntegrationAnalysis(context.Context, string, string) (model.IntegrationAnalysis, error)
	SaveIntegrationAnalysis(context.Context, model.IntegrationAnalysis, int64) (model.IntegrationAnalysis, error)
	Recipes(context.Context, string) ([]model.Recipe, error)
	Recipe(context.Context, string, string) (model.Recipe, error)
	RecipeBySlug(context.Context, string, string) (model.Recipe, error)
	CreateRecipeWithRevision(context.Context, model.Recipe, model.RecipeRevision) (model.Recipe, error)
	SaveRecipeTransition(context.Context, model.Recipe, int64, bool, *model.AuditEvent) (model.Recipe, error)
	SaveRecipe(context.Context, model.Recipe, int64) (model.Recipe, error)
	RecipeRevisions(context.Context, string) ([]model.RecipeRevision, error)
	SaveRecipeRevision(context.Context, model.Recipe, model.RecipeRevision, int64, bool) (model.Recipe, error)
}

// IdentityStore owns persistence operations for one cohesive application domain.
type IdentityStore interface {
	IdentityProvider(context.Context, string) (identity.ProviderConfig, error)
	SaveIdentityProvider(context.Context, identity.ProviderConfig) (identity.ProviderConfig, error)
	DeleteIdentityProvider(context.Context, string, int64) (identity.ProviderConfig, error)
	CreateIdentityProviderTest(context.Context, identity.ProviderTest) error
	IdentityProviderTest(context.Context, string, string) (identity.ProviderTest, error)
	ClaimIdentityProviderTestByStateDigest(context.Context, []byte, time.Time) (identity.ProviderTest, error)
	LatestIdentityProviderTest(context.Context, string) (identity.ProviderTest, error)
	CompleteIdentityProviderTest(context.Context, identity.ProviderTest) (identity.ProviderTest, error)
	ExpireIdentityProviderTests(context.Context, string, time.Time) error
	OAuthClient(context.Context, string, string) (identity.OAuthClient, error)
	CreateOAuthClient(context.Context, identity.OAuthClient) (identity.OAuthClient, error)
	CustomerAccounts(context.Context, string, string, int) ([]identity.CustomerAccount, bool, error)
	CustomerAccount(context.Context, string, string) (identity.CustomerAccount, error)
	UpdateCustomerAccount(context.Context, identity.CustomerAccount, int64) (identity.CustomerAccount, error)
	ResolveCustomerAccount(context.Context, identity.CustomerAccount) (identity.CustomerAccount, error)
	CreateOAuthState(context.Context, identity.OAuthState) error
	ConsumeOAuthState(context.Context, []byte) (identity.OAuthState, error)
	CreateOAuthCode(context.Context, identity.OAuthCode) error
	ConsumeOAuthCode(context.Context, []byte) (identity.OAuthCode, error)
	CreateAccessToken(context.Context, identity.AccessToken) error
	AccessTokenByDigest(context.Context, []byte) (identity.AccessToken, error)
	DeleteStaleOAuthArtifacts(context.Context, string, time.Time, int) (int64, error)
}

// ObservabilityStore owns persistence operations for one cohesive application domain.
type ObservabilityStore interface {
	PublicKnowledge(context.Context, string, []string, string) ([]model.KnowledgeRecord, error)
	PrivateKnowledge(context.Context, string, []string, string) ([]model.KnowledgeRecord, error)
	RelevantPrivateKnowledge(context.Context, string, []string, string, int) ([]model.KnowledgeRecord, error)
	AppendAudit(context.Context, model.AuditEvent) error
	AuditEvents(context.Context, string) ([]model.AuditEvent, error)
}
