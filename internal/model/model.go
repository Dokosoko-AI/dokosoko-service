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
	ID                       string    `json:"id"`
	OrganisationID           string    `json:"organisation_id"`
	Name                     string    `json:"name"`
	Slug                     string    `json:"slug"`
	Description              string    `json:"description"`
	DefaultVersionPolicy     string    `json:"default_version_policy"`
	CatalogRevision          int64     `json:"catalog_revision"`
	RequirePromotionApproval bool      `json:"require_promotion_approval"`
	PublicMCPEnabled         bool      `json:"public_mcp_enabled"`
	Revision                 int64     `json:"revision"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ProductVersion struct {
	ID                   string                 `json:"id"`
	OrganisationID       string                 `json:"organisation_id"`
	ProductID            string                 `json:"product_id"`
	Version              string                 `json:"version"`
	ProfileID            string                 `json:"profile_id"`
	ProfileName          string                 `json:"profile_name"`
	DefinitionRevision   int64                  `json:"definition_revision"`
	ManifestHash         string                 `json:"manifest_hash"`
	Diff                 ProductVersionDiff     `json:"diff"`
	ReleaseStage         string                 `json:"release_stage"`
	RolloutPercentage    int                    `json:"rollout_percentage"`
	PromotionState       string                 `json:"promotion_state"`
	PromotionNote        string                 `json:"promotion_note,omitempty"`
	RequestedLatest      bool                   `json:"requested_latest"`
	RequestedLTS         bool                   `json:"requested_lts"`
	PublisherActorID     string                 `json:"publisher_actor_id,omitempty"`
	PromotionRequestedBy string                 `json:"promotion_requested_by,omitempty"`
	ApprovedBy           string                 `json:"approved_by,omitempty"`
	ApprovedAt           *time.Time             `json:"approved_at,omitempty"`
	DriftStatus          string                 `json:"drift_status"`
	DriftDetails         []ProductArtifactDrift `json:"drift_details"`
	DriftCheckedAt       *time.Time             `json:"drift_checked_at,omitempty"`
	IsLatest             bool                   `json:"is_latest"`
	IsLTS                bool                   `json:"is_lts"`
	DeprecatedAt         *time.Time             `json:"deprecated_at,omitempty"`
	DeprecationMessage   string                 `json:"deprecation_message,omitempty"`
	ReplacementVersion   string                 `json:"replacement_version,omitempty"`
	SunsetAt             *time.Time             `json:"sunset_at,omitempty"`
	Revision             int64                  `json:"revision"`
	PublishedAt          time.Time              `json:"published_at"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	Manifest             ProductDefinition      `json:"-"`
}

type ProductVersionPin struct {
	ID               string    `json:"id"`
	OrganisationID   string    `json:"organisation_id"`
	ProductID        string    `json:"product_id"`
	Scope            string    `json:"scope"`
	ScopeID          string    `json:"scope_id"`
	CustomerID       string    `json:"customer_id"`
	EnvironmentID    string    `json:"environment_id,omitempty"`
	InstallationID   string    `json:"installation_id,omitempty"`
	ProductVersionID string    `json:"product_version_id"`
	ProductVersion   string    `json:"product_version"`
	Reason           string    `json:"reason,omitempty"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ProductInstallation struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	CustomerID     string    `json:"customer_id"`
	EnvironmentID  string    `json:"environment_id"`
	ExternalID     string    `json:"external_id"`
	Name           string    `json:"name"`
	State          string    `json:"state"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ProductVersionPinHistory struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	PinID          string    `json:"pin_id"`
	Scope          string    `json:"scope"`
	ScopeID        string    `json:"scope_id"`
	PriorVersion   string    `json:"prior_version,omitempty"`
	ProductVersion string    `json:"product_version,omitempty"`
	Action         string    `json:"action"`
	Reason         string    `json:"reason,omitempty"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type ProductVersionChange struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

type ProductVersionDiff struct {
	FromVersionID string                 `json:"from_version_id,omitempty"`
	FromVersion   string                 `json:"from_version,omitempty"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Summary       string                 `json:"summary"`
	Added         []ProductVersionChange `json:"added"`
	Removed       []ProductVersionChange `json:"removed"`
	Changed       []ProductVersionChange `json:"changed"`
}

type ProductArtifactDrift struct {
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id,omitempty"`
	Name        string `json:"name"`
	Expected    string `json:"expected,omitempty"`
	Observed    string `json:"observed,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type ProductManifestArtifact struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ProductManifestCapability struct {
	ID        string                    `json:"id"`
	Name      string                    `json:"name"`
	Release   string                    `json:"release"`
	Artifacts []ProductManifestArtifact `json:"artifacts"`
}

type ProductVersionSummary struct {
	ID                 string     `json:"id"`
	Version            string     `json:"version"`
	ProfileName        string     `json:"profile_name"`
	ManifestHash       string     `json:"manifest_hash"`
	ReleaseStage       string     `json:"release_stage"`
	RolloutPercentage  int        `json:"rollout_percentage"`
	PromotionState     string     `json:"promotion_state"`
	DriftStatus        string     `json:"drift_status"`
	IsLatest           bool       `json:"is_latest"`
	IsLTS              bool       `json:"is_lts"`
	Deprecated         bool       `json:"deprecated"`
	DeprecationMessage string     `json:"deprecation_message,omitempty"`
	ReplacementVersion string     `json:"replacement_version,omitempty"`
	SunsetAt           *time.Time `json:"sunset_at,omitempty"`
}

type ProductManifest struct {
	ProductID            string                      `json:"product_id"`
	ProductSlug          string                      `json:"product_slug"`
	ProductName          string                      `json:"product_name"`
	Description          string                      `json:"description"`
	DefaultVersionPolicy string                      `json:"default_version_policy"`
	CatalogRevision      int64                       `json:"catalog_revision"`
	ManifestHash         string                      `json:"manifest_hash,omitempty"`
	DefinitionRevision   int64                       `json:"definition_revision,omitempty"`
	EffectiveVersion     *ProductVersionSummary      `json:"effective_version,omitempty"`
	SelectionSource      string                      `json:"selection_source"`
	CustomerID           string                      `json:"customer_id,omitempty"`
	EnvironmentID        string                      `json:"environment_id,omitempty"`
	InstallationID       string                      `json:"installation_id,omitempty"`
	OperationalWarnings  []string                    `json:"operational_warnings"`
	Artifacts            []ProductManifestArtifact   `json:"artifacts"`
	Capabilities         []ProductManifestCapability `json:"capabilities"`
	AvailableVersions    []ProductVersionSummary     `json:"available_versions"`
}

type ProductSelectionContext struct {
	CustomerID     string
	EnvironmentID  string
	InstallationID string
}

type ProductVersionImpact struct {
	ProductVersionID      string   `json:"product_version_id"`
	ProductVersion        string   `json:"product_version"`
	CustomerPins          int      `json:"customer_pins"`
	EnvironmentPins       int      `json:"environment_pins"`
	InstallationPins      int      `json:"installation_pins"`
	AffectedCustomers     []string `json:"affected_customers"`
	AffectedEnvironments  []string `json:"affected_environments"`
	AffectedInstallations []string `json:"affected_installations"`
	Requests30Days        int64    `json:"requests_30_days"`
	ToolCalls30Days       int64    `json:"tool_calls_30_days"`
}

type ProductVersionActivity struct {
	Requests  int64
	ToolCalls int64
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

// ReportingConfig controls the two product-owned Private MCP reporting tools.
// Hook credentials are referenced by ID and are never serialized to an API or
// MCP client.
type ReportingConfig struct {
	ID                       string    `json:"id"`
	OrganisationID           string    `json:"organisation_id"`
	ProductID                string    `json:"product_id"`
	BugReportsEnabled        bool      `json:"bug_reports_enabled"`
	FeedbackEnabled          bool      `json:"feedback_enabled"`
	BugHookURL               string    `json:"bug_hook_url"`
	BugHookCredentialID      string    `json:"-"`
	FeedbackHookURL          string    `json:"feedback_hook_url"`
	FeedbackHookCredentialID string    `json:"-"`
	RetentionDays            int       `json:"retention_days"`
	Revision                 int64     `json:"revision"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// ReportSubmission is the durable outbox record. PayloadCiphertext contains
// both user-authored content and trusted reporter/product context. Only routing
// and delivery state are stored in plaintext.
type ReportSubmission struct {
	ID                 string     `json:"id"`
	OrganisationID     string     `json:"organisation_id"`
	ProductID          string     `json:"product_id"`
	Kind               string     `json:"kind"`
	State              string     `json:"state"`
	ActorPseudonym     string     `json:"actor_pseudonym"`
	IdempotencyDigest  []byte     `json:"-"`
	PayloadCiphertext  []byte     `json:"-"`
	PayloadNonce       []byte     `json:"-"`
	PayloadKeyVersion  int        `json:"-"`
	PayloadFingerprint string     `json:"-"`
	Attempts           int        `json:"attempts"`
	NextAttemptAt      *time.Time `json:"next_attempt_at,omitempty"`
	DeliveryStartedAt  *time.Time `json:"delivery_started_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	ExternalID         string     `json:"external_id,omitempty"`
	ExternalURL        string     `json:"external_url,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeliveredAt        *time.Time `json:"delivered_at,omitempty"`
	ExpiresAt          time.Time  `json:"expires_at"`
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
	Versions         map[string]int64 `json:"versions"`
	Funnel           map[string]int64 `json:"funnel"`
	DailyRequests    []AnalyticsPoint `json:"daily_requests"`
	Since            time.Time        `json:"since"`
	GeneratedAt      time.Time        `json:"generated_at"`
}
