package store

import (
	"context"
	"errors"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("revision conflict")
)

type Store interface {
	Organisations(context.Context) ([]model.Organisation, error)
	CreateOrganisation(context.Context, model.Organisation) (model.Organisation, error)
	Products(context.Context, string) ([]model.Product, error)
	CreateProduct(context.Context, model.Product) (model.Product, error)
	Environments(context.Context, string) ([]model.Environment, error)
	CreateEnvironment(context.Context, model.Environment) (model.Environment, error)
	Product(context.Context, string) (model.Product, error)
	UpdateProduct(context.Context, model.Product, int64) (model.Product, error)
	ProductVersions(context.Context, string) ([]model.ProductVersion, error)
	ProductVersion(context.Context, string, string) (model.ProductVersion, error)
	CreateProductVersion(context.Context, model.ProductVersion) (model.ProductVersion, error)
	UpdateProductVersion(context.Context, model.ProductVersion, int64) (model.ProductVersion, error)
	ProductVersionPins(context.Context, string) ([]model.ProductVersionPin, error)
	ProductVersionPin(context.Context, string, string) (model.ProductVersionPin, error)
	SaveProductVersionPin(context.Context, model.ProductVersionPin) (model.ProductVersionPin, error)
	DeleteProductVersionPin(context.Context, string, string) error
	ProductDefinition(context.Context, string) (model.ProductDefinition, error)
	SaveProductDefinition(context.Context, model.ProductDefinition, int64) (model.ProductDefinition, error)
	ProductBuilds(context.Context, string) ([]model.ProductBuild, error)
	ProductBuild(context.Context, string, string) (model.ProductBuild, error)
	CreateProductBuild(context.Context, model.ProductBuild) (model.ProductBuild, error)
	MarkProductBuildPublished(context.Context, string, string) (model.ProductBuild, error)
	Sources(context.Context, string) ([]model.Source, error)
	CreateSource(context.Context, model.Source) (model.Source, error)
	Source(context.Context, string, string) (model.Source, error)
	UpdateSource(context.Context, model.Source, int64) (model.Source, error)
	PublishSource(context.Context, string, string, int64) (model.Source, error)
	CrawlJobs(context.Context, string, string) ([]model.CrawlJob, error)
	CreateCrawlJob(context.Context, model.CrawlJob) (model.CrawlJob, error)
	Packages(context.Context, string) ([]model.Package, error)
	CreatePackage(context.Context, model.Package) (model.Package, error)
	Package(context.Context, string, string) (model.Package, error)
	UpdatePackage(context.Context, model.Package, int64) (model.Package, error)
	PublishPackage(context.Context, string, string, int64) (model.Package, error)
	CreateSecret(context.Context, model.Secret) (model.Secret, error)
	Secret(context.Context, string, string) (model.Secret, error)
	Tools(context.Context, string, bool) ([]model.Tool, error)
	Tool(context.Context, string, string) (model.Tool, error)
	CreateTool(context.Context, model.Tool) (model.Tool, error)
	UpdateImportedTool(context.Context, model.Tool, int64) (model.Tool, error)
	MarkImportedToolDrift(context.Context, string, string, bool) (model.Tool, error)
	PublishTool(context.Context, string, string, int64, string) (model.Tool, error)
	MCPConnections(context.Context, string) ([]model.MCPConnection, error)
	MCPConnection(context.Context, string, string) (model.MCPConnection, error)
	CreateMCPConnection(context.Context, model.MCPConnection) (model.MCPConnection, error)
	UpdateMCPConnectionSync(context.Context, string, string, string, time.Time) (model.MCPConnection, error)
	MCPUserGrant(context.Context, string, string) (model.MCPUserGrant, error)
	SaveMCPUserGrant(context.Context, model.MCPUserGrant) (model.MCPUserGrant, error)
	CreateMCPAuthorizationState(context.Context, model.MCPAuthorizationState) error
	ConsumeMCPAuthorizationState(context.Context, []byte) (model.MCPAuthorizationState, error)
	Providers(context.Context, string) ([]model.Provider, error)
	Provider(context.Context, string, string) (model.Provider, error)
	CreateProvider(context.Context, model.Provider) (model.Provider, error)
	Projects(context.Context, string) ([]model.Project, error)
	Project(context.Context, string, string) (model.Project, error)
	CreateProject(context.Context, model.Project) (model.Project, error)
	CredentialLeases(context.Context, string) ([]model.CredentialLease, error)
	CredentialLease(context.Context, string, string) (model.CredentialLease, error)
	CreateCredentialLease(context.Context, model.CredentialLease) (model.CredentialLease, error)
	RevokeCredentialLease(context.Context, string, string, time.Time) (model.CredentialLease, error)
	IntegrationRuns(context.Context, string) ([]model.IntegrationRun, error)
	IntegrationRun(context.Context, string, string) (model.IntegrationRun, error)
	CreateIntegrationRun(context.Context, model.IntegrationRun) (model.IntegrationRun, error)
	CompleteIntegrationRun(context.Context, string, string, *bool, *bool, string, time.Time) (model.IntegrationRun, error)
	LLMProfiles(context.Context, string) ([]model.LLMProfile, error)
	SaveLLMProfile(context.Context, model.LLMProfile) (model.LLMProfile, error)
	VendorIdentity(context.Context, string) (identity.VendorConfig, error)
	SaveVendorIdentity(context.Context, identity.VendorConfig) (identity.VendorConfig, error)
	CreateOAuthState(context.Context, identity.OAuthState) error
	ConsumeOAuthState(context.Context, []byte) (identity.OAuthState, error)
	CreateOAuthCode(context.Context, identity.OAuthCode) error
	ConsumeOAuthCode(context.Context, []byte) (identity.OAuthCode, error)
	CreateAccessToken(context.Context, identity.AccessToken) error
	AccessTokenByDigest(context.Context, []byte) (identity.AccessToken, error)
	PublicKnowledge(context.Context, string, string) ([]model.KnowledgeRecord, error)
	PrivateKnowledge(context.Context, string, string) ([]model.KnowledgeRecord, error)
	AppendAnalytics(context.Context, model.AnalyticsEvent) error
	AnalyticsSummary(context.Context, string, time.Time) (model.AnalyticsSummary, error)
	AppendAudit(context.Context, model.AuditEvent) error
	AuditEvents(context.Context, string) ([]model.AuditEvent, error)
}
