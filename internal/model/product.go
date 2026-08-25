package model

import (
	"encoding/json"
	"time"
)

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
	ID                string    `json:"id"`
	OrganisationID    string    `json:"organisation_id"`
	ProductID         string    `json:"product_id"`
	Scope             string    `json:"scope"`
	ScopeID           string    `json:"scope_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	EnvironmentID     string    `json:"environment_id,omitempty"`
	InstallationID    string    `json:"installation_id,omitempty"`
	ProductVersionID  string    `json:"product_version_id"`
	ProductVersion    string    `json:"product_version"`
	Reason            string    `json:"reason,omitempty"`
	Revision          int64     `json:"revision"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ProductInstallation struct {
	ID                string    `json:"id"`
	OrganisationID    string    `json:"organisation_id"`
	ProductID         string    `json:"product_id"`
	CustomerAccountID string    `json:"customer_account_id"`
	EnvironmentID     string    `json:"environment_id"`
	ExternalID        string    `json:"external_id"`
	Name              string    `json:"name"`
	State             string    `json:"state"`
	Revision          int64     `json:"revision"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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

type IntegrationManifestResource struct {
	ResourceSetID      string                                 `json:"resource_set_id"`
	Kind               string                                 `json:"kind"`
	Name               string                                 `json:"name"`
	Revision           int64                                  `json:"revision"`
	ContentHash        string                                 `json:"content_hash"`
	SourcePublications []IntegrationManifestSourcePublication `json:"source_publications,omitempty"`
}

type IntegrationManifestSourcePublication struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type IntegrationManifestPackage struct {
	PackageArtifactID            string     `json:"package_artifact_id"`
	PackageReleaseID             string     `json:"package_release_id"`
	Name                         string     `json:"name"`
	Ecosystem                    string     `json:"ecosystem"`
	Coordinate                   string     `json:"coordinate"`
	Version                      string     `json:"version"`
	PURL                         string     `json:"purl"`
	RegistryURL                  string     `json:"registry_url"`
	SourceURL                    string     `json:"source_url,omitempty"`
	Language                     string     `json:"language,omitempty"`
	Platform                     string     `json:"platform,omitempty"`
	InstallCommand               string     `json:"install_command"`
	Digest                       string     `json:"digest"`
	ProvenanceURL                string     `json:"provenance_url,omitempty"`
	SBOMURL                      string     `json:"sbom_url,omitempty"`
	Visibility                   Visibility `json:"visibility"`
	Lifecycle                    string     `json:"lifecycle"`
	ReplacementPackageArtifactID string     `json:"replacement_package_artifact_id,omitempty"`
	DeprecationMessage           string     `json:"deprecation_message,omitempty"`
	SunsetAt                     *time.Time `json:"sunset_at,omitempty"`
	ContentHash                  string     `json:"content_hash"`
}

type IntegrationManifestAuthorizationPoint struct {
	ID                   string   `json:"id"`
	Key                  string   `json:"key"`
	Name                 string   `json:"name"`
	ActionType           string   `json:"action_type"`
	RequiredGrants       []string `json:"required_grants"`
	ConfirmationRequired bool     `json:"confirmation_required"`
	DecisionTTLSeconds   int      `json:"decision_ttl_seconds"`
	Revision             int64    `json:"revision"`
}

type IntegrationManifestTool struct {
	ToolID                     string `json:"tool_id"`
	ToolRevision               int64  `json:"tool_revision"`
	AuthorizationPointID       string `json:"authorization_point_id"`
	AuthorizationPointRevision int64  `json:"authorization_point_revision"`
	RuntimeServiceConnectionID string `json:"runtime_service_connection_id,omitempty"`
	Namespace                  string `json:"namespace"`
	Name                       string `json:"name"`
	BackendKind                string `json:"backend_kind"`
	ContentHash                string `json:"content_hash"`
	UpstreamSchemaHash         string `json:"upstream_schema_hash,omitempty"`
}

type IntegrationManifestAccessConnection struct {
	ConnectionID             string `json:"connection_id"`
	ConnectionRevision       int64  `json:"connection_revision"`
	AccessDefinitionID       string `json:"access_definition_id"`
	AccessDefinitionRevision int64  `json:"access_definition_revision"`
	EnvironmentID            string `json:"environment_id,omitempty"`
	State                    string `json:"state"`
	ContentHash              string `json:"content_hash"`
}

type IntegrationManifestServiceConnectionRevision struct {
	RevisionID         string          `json:"revision_id"`
	Revision           int64           `json:"revision"`
	EnvironmentID      string          `json:"environment_id"`
	BaseURL            string          `json:"base_url"`
	AuthenticationType string          `json:"authentication_type"`
	CredentialSetID    string          `json:"credential_set_id,omitempty"`
	AuthConfig         json.RawMessage `json:"auth_config"`
	ContentHash        string          `json:"content_hash"`
	Current            bool            `json:"current"`
	CredentialReady    bool            `json:"credential_ready"`
}

type IntegrationManifestServiceConnection struct {
	ConnectionID       string                                         `json:"connection_id"`
	ConnectionRevision int64                                          `json:"connection_revision"`
	Name               string                                         `json:"name"`
	Description        string                                         `json:"description,omitempty"`
	State              string                                         `json:"state"`
	CurrentRevisions   []IntegrationManifestServiceConnectionRevision `json:"current_revisions"`
}

type IntegrationManifest struct {
	ID                       string                                  `json:"id"`
	FamilyKey                string                                  `json:"family_key"`
	VersionKey               string                                  `json:"version_key"`
	DisplayName              string                                  `json:"display_name"`
	Description              string                                  `json:"description"`
	Visibility               Visibility                              `json:"visibility"`
	Lifecycle                string                                  `json:"lifecycle"`
	ReplacementIntegrationID string                                  `json:"replacement_integration_id,omitempty"`
	SunsetAt                 *time.Time                              `json:"sunset_at,omitempty"`
	Revision                 int64                                   `json:"revision"`
	ManifestHash             string                                  `json:"manifest_hash"`
	Resources                []IntegrationManifestResource           `json:"resources"`
	Packages                 []IntegrationManifestPackage            `json:"packages"`
	AuthorizationPoints      []IntegrationManifestAuthorizationPoint `json:"authorization_points"`
	Tools                    []IntegrationManifestTool               `json:"tools"`
	ServiceConnections       []IntegrationManifestServiceConnection  `json:"service_connections"`
	AccessConnections        []IntegrationManifestAccessConnection   `json:"access_connections"`
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
	DeploymentID            string                      `json:"deployment_id"`
	DeploymentSlug          string                      `json:"deployment_slug"`
	DeploymentName          string                      `json:"deployment_name"`
	ProductID               string                      `json:"product_id"`
	ProductSlug             string                      `json:"product_slug"`
	ProductName             string                      `json:"product_name"`
	Description             string                      `json:"description"`
	DefaultVersionPolicy    string                      `json:"default_version_policy"`
	CatalogRevision         int64                       `json:"catalog_revision"`
	ManifestHash            string                      `json:"manifest_hash,omitempty"`
	DefinitionRevision      int64                       `json:"definition_revision,omitempty"`
	EffectiveVersion        *ProductVersionSummary      `json:"effective_version,omitempty"`
	SelectionSource         string                      `json:"selection_source"`
	CustomerAccountID       string                      `json:"customer_account_id,omitempty"`
	EnvironmentID           string                      `json:"environment_id,omitempty"`
	InstallationID          string                      `json:"installation_id,omitempty"`
	OperationalWarnings     []string                    `json:"operational_warnings"`
	Artifacts               []ProductManifestArtifact   `json:"artifacts"`
	Capabilities            []ProductManifestCapability `json:"capabilities"`
	ManagedIntegrationTools bool                        `json:"managed_integration_tools"`
	Integrations            []IntegrationManifest       `json:"integrations"`
	AvailableVersions       []ProductVersionSummary     `json:"available_versions"`
}

type ProductSelectionContext struct {
	CustomerAccountID string
	EnvironmentID     string
	InstallationID    string
	Public            bool
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
	Kind     string            `json:"kind"`
	Name     string            `json:"name,omitempty"`
	Location string            `json:"location"`
	Version  string            `json:"version,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
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
