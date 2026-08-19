package model

import (
	"encoding/json"
	"time"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

func (v Visibility) Valid() bool {
	return v == VisibilityPrivate || v == VisibilityPublic
}

type Organisation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Environment struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	IsProduction   bool      `json:"is_production"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Product struct {
	ID               string    `json:"id"`
	OrganisationID   string    `json:"organisation_id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	PublicMCPEnabled bool      `json:"public_mcp_enabled"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ProductBinding struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	ReferenceID string   `json:"reference_id,omitempty"`
	Location    string   `json:"location,omitempty"`
	Version     string   `json:"version,omitempty"`
	Scope       string   `json:"scope"`
	Confidence  float64  `json:"confidence"`
	Evidence    []string `json:"evidence"`
	Verified    bool     `json:"verified"`
}

type ProductRelease struct {
	ID       string           `json:"id"`
	Version  string           `json:"version"`
	State    string           `json:"state"`
	Bindings []ProductBinding `json:"bindings"`
}

type ProductComponent struct {
	ID              string           `json:"id"`
	Kind            string           `json:"kind"`
	Name            string           `json:"name"`
	Slug            string           `json:"slug"`
	Description     string           `json:"description,omitempty"`
	VersionStrategy string           `json:"version_strategy"`
	Releases        []ProductRelease `json:"releases"`
}

type ProductProfileSelection struct {
	ComponentID string `json:"component_id"`
	ReleaseID   string `json:"release_id"`
}

type ProductProfile struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	State      string                    `json:"state"`
	Selections []ProductProfileSelection `json:"selections"`
}

type ProductValidationFinding struct {
	Level       string `json:"level"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	ComponentID string `json:"component_id,omitempty"`
	BindingID   string `json:"binding_id,omitempty"`
}

type ProductDefinition struct {
	ID              string                     `json:"id"`
	OrganisationID  string                     `json:"organisation_id"`
	ProductID       string                     `json:"product_id"`
	Name            string                     `json:"name"`
	Slug            string                     `json:"slug"`
	State           string                     `json:"state"`
	VersionStrategy string                     `json:"version_strategy"`
	MCPPolicy       string                     `json:"mcp_policy"`
	Components      []ProductComponent         `json:"components"`
	ProductBindings []ProductBinding           `json:"product_bindings"`
	Profiles        []ProductProfile           `json:"profiles"`
	Validation      []ProductValidationFinding `json:"validation"`
	GeneratedBy     string                     `json:"generated_by"`
	SourceBuildID   string                     `json:"source_build_id,omitempty"`
	Revision        int64                      `json:"revision"`
	PublishedAt     *time.Time                 `json:"published_at,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
}

type ProductBuildInput struct {
	Kind      string            `json:"kind"`
	Name      string            `json:"name,omitempty"`
	Location  string            `json:"location"`
	Version   string            `json:"version,omitempty"`
	Ecosystem string            `json:"ecosystem,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ProductBuild struct {
	ID             string                     `json:"id"`
	OrganisationID string                     `json:"organisation_id"`
	ProductID      string                     `json:"product_id"`
	State          string                     `json:"state"`
	AnalysisMode   string                     `json:"analysis_mode"`
	Inputs         []ProductBuildInput        `json:"inputs"`
	Proposal       ProductDefinition          `json:"proposal"`
	Unresolved     []ProductValidationFinding `json:"unresolved"`
	CreatedAt      time.Time                  `json:"created_at"`
	CompletedAt    *time.Time                 `json:"completed_at,omitempty"`
}

type Source struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Location       string     `json:"location"`
	Visibility     Visibility `json:"visibility"`
	Published      bool       `json:"published"`
	Quarantined    bool       `json:"quarantined"`
	Revision       int64      `json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CrawlJob struct {
	ID              string     `json:"id"`
	OrganisationID  string     `json:"organisation_id"`
	ProductID       string     `json:"product_id"`
	SourceID        string     `json:"source_id"`
	State           string     `json:"state"`
	Attempt         int        `json:"attempt"`
	DiscoveredCount int        `json:"discovered_count"`
	FetchedCount    int        `json:"fetched_count"`
	ChangedCount    int        `json:"changed_count"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	QueuedAt        time.Time  `json:"queued_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type Package struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	Name           string     `json:"name"`
	Ecosystem      string     `json:"ecosystem"`
	Version        string     `json:"version"`
	Mode           string     `json:"mode"`
	Location       string     `json:"-"`
	FetchHookURL   string     `json:"-"`
	CredentialID   string     `json:"-"`
	ChecksumSHA256 []byte     `json:"-"`
	ExpectedSize   int64      `json:"-"`
	Visibility     Visibility `json:"visibility"`
	Published      bool       `json:"published"`
	Revision       int64      `json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Secret struct {
	ID             string
	OrganisationID string
	Name           string
	Purpose        string
	Ciphertext     []byte
	Nonce          []byte
	KeyVersion     int
	Fingerprint    string
	CreatedAt      time.Time
}

type Tool struct {
	ID                  string          `json:"id"`
	OrganisationID      string          `json:"organisation_id"`
	ProductID           string          `json:"product_id"`
	Namespace           string          `json:"namespace"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	InputSchema         json.RawMessage `json:"input_schema"`
	OutputSchema        json.RawMessage `json:"output_schema"`
	State               string          `json:"state"`
	Revision            int64           `json:"revision"`
	APIConnectionID     string          `json:"-"`
	BaseURL             string          `json:"-"`
	HTTPMethod          string          `json:"http_method"`
	CredentialID        string          `json:"-"`
	AuthorizationPolicy json.RawMessage `json:"authorization_policy"`
	TimeoutMS           int             `json:"timeout_ms"`
	BackendKind         string          `json:"backend_kind"`
	MCPConnectionID     string          `json:"mcp_connection_id,omitempty"`
	UpstreamToolName    string          `json:"upstream_tool_name,omitempty"`
	UpstreamSchemaHash  string          `json:"upstream_schema_hash,omitempty"`
	UpstreamAnnotations json.RawMessage `json:"upstream_annotations,omitempty"`
	UpstreamDrifted     bool            `json:"upstream_drifted"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

const StatelessMCPv2Protocol = "2026-07-28"

type MCPConnection struct {
	ID                  string          `json:"id"`
	OrganisationID      string          `json:"organisation_id"`
	ProductID           string          `json:"product_id"`
	Name                string          `json:"name"`
	Namespace           string          `json:"namespace"`
	Endpoint            string          `json:"endpoint"`
	ProtocolVersion     string          `json:"protocol_version"`
	AuthMode            string          `json:"auth_mode"`
	CredentialID        string          `json:"-"`
	OAuthClientID       string          `json:"oauth_client_id,omitempty"`
	OAuthClientSecretID string          `json:"-"`
	OAuthIssuer         string          `json:"oauth_issuer,omitempty"`
	AuthorizationURL    string          `json:"authorization_url,omitempty"`
	TokenURL            string          `json:"token_url,omitempty"`
	Scopes              []string        `json:"scopes"`
	State               string          `json:"state"`
	LastSyncedAt        *time.Time      `json:"last_synced_at,omitempty"`
	LastCatalogHash     string          `json:"last_catalog_hash,omitempty"`
	Config              json.RawMessage `json:"config"`
	Revision            int64           `json:"revision"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type MCPUserGrant struct {
	ID              string     `json:"id"`
	OrganisationID  string     `json:"organisation_id"`
	ProductID       string     `json:"product_id"`
	ConnectionID    string     `json:"connection_id"`
	SubjectID       string     `json:"-"`
	UpstreamSubject string     `json:"upstream_subject,omitempty"`
	AccessSecretID  string     `json:"-"`
	RefreshSecretID string     `json:"-"`
	Scopes          []string   `json:"scopes"`
	ExpiresAt       time.Time  `json:"expires_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type MCPAuthorizationState struct {
	Digest       []byte
	ConnectionID string
	ProductID    string
	SubjectID    string
	CodeVerifier string
	ExpiresAt    time.Time
}

type Provider struct {
	ID             string          `json:"id"`
	OrganisationID string          `json:"organisation_id"`
	ProductID      string          `json:"product_id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	BaseURL        string          `json:"-"`
	CredentialID   string          `json:"-"`
	Config         json.RawMessage `json:"config"`
	Revision       int64           `json:"revision"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type Project struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	EnvironmentID  string     `json:"environment_id"`
	ProviderID     string     `json:"provider_id"`
	OwnerType      string     `json:"owner_type"`
	OwnerID        string     `json:"owner_id"`
	ExternalID     string     `json:"external_id"`
	IdempotencyKey string     `json:"idempotency_key"`
	State          string     `json:"state"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CredentialLease struct {
	ID                string     `json:"id"`
	OrganisationID    string     `json:"organisation_id"`
	ProductID         string     `json:"product_id"`
	EnvironmentID     string     `json:"environment_id"`
	ProjectID         string     `json:"project_id,omitempty"`
	ProviderID        string     `json:"provider_id"`
	SubjectID         string     `json:"subject_id"`
	ExternalID        string     `json:"external_id"`
	IdempotencyKey    string     `json:"idempotency_key"`
	Scopes            []string   `json:"scopes"`
	SecretFingerprint string     `json:"secret_fingerprint"`
	ExpiresAt         time.Time  `json:"expires_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type IntegrationRun struct {
	ID               string     `json:"id"`
	OrganisationID   string     `json:"organisation_id"`
	ProductID        string     `json:"product_id"`
	EnvironmentID    string     `json:"environment_id"`
	UserID           string     `json:"user_id,omitempty"`
	ActorPseudonym   string     `json:"-"`
	RequestedOutcome string     `json:"requested_outcome"`
	State            string     `json:"state"`
	ReportedSuccess  *bool      `json:"reported_success,omitempty"`
	ValidatedSuccess *bool      `json:"validated_success,omitempty"`
	FailureCode      string     `json:"failure_code,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

type LLMProfile struct {
	ID                  string          `json:"id"`
	OrganisationID      string          `json:"organisation_id"`
	ProductID           string          `json:"product_id"`
	Role                string          `json:"role"`
	Provider            string          `json:"provider"`
	Endpoint            string          `json:"-"`
	Model               string          `json:"model"`
	CredentialID        string          `json:"-"`
	EmbeddingDimensions int             `json:"embedding_dimensions,omitempty"`
	MaxInputTokens      int             `json:"max_input_tokens"`
	MaxOutputTokens     int             `json:"max_output_tokens"`
	DailyTokenBudget    int64           `json:"daily_token_budget"`
	Hardening           json.RawMessage `json:"hardening"`
	Enabled             bool            `json:"enabled"`
	Revision            int64           `json:"revision"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type AuditEvent struct {
	ID             string         `json:"id"`
	OrganisationID string         `json:"organisation_id"`
	ProductID      string         `json:"product_id,omitempty"`
	ActorID        string         `json:"actor_id"`
	Action         string         `json:"action"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id"`
	Prior          map[string]any `json:"prior,omitempty"`
	Current        map[string]any `json:"current,omitempty"`
	RequestID      string         `json:"request_id"`
	CreatedAt      time.Time      `json:"created_at"`
}

type KnowledgeRecord struct {
	ID         string     `json:"id"`
	ProductID  string     `json:"product_id"`
	SourceID   string     `json:"source_id"`
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	URL        string     `json:"url"`
	Visibility Visibility `json:"visibility"`
	Published  bool       `json:"published"`
}

type AnalyticsEvent struct {
	OrganisationID   string         `json:"organisation_id"`
	ProductID        string         `json:"product_id"`
	EventName        string         `json:"event_name"`
	ActorKind        string         `json:"actor_kind"`
	ActorPseudonym   string         `json:"-"`
	IntegrationRunID string         `json:"integration_run_id,omitempty"`
	Dimensions       map[string]any `json:"dimensions,omitempty"`
	Value            float64        `json:"value,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type AnalyticsPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type AnalyticsSummary struct {
	ActiveDevelopers int64            `json:"active_developers"`
	AuthorizedUsers  int64            `json:"authorized_users"`
	MCPRequests      int64            `json:"mcp_requests"`
	ToolCalls        int64            `json:"tool_calls"`
	PackageDownloads int64            `json:"package_downloads"`
	IntegrationRuns  int64            `json:"integration_runs"`
	ValidatedRuns    int64            `json:"validated_runs"`
	ValidatedSuccess int64            `json:"validated_success"`
	FirstPassRate    float64          `json:"first_pass_rate"`
	Channels         map[string]int64 `json:"channels"`
	Funnel           map[string]int64 `json:"funnel"`
	DailyRequests    []AnalyticsPoint `json:"daily_requests"`
	Since            time.Time        `json:"since"`
	GeneratedAt      time.Time        `json:"generated_at"`
}
