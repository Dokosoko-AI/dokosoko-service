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

// Deployment is the singleton identity of a DokoSoko installation. Product is
// retained above only while legacy product-scoped clients are migrated.
type Deployment struct {
	ID                       string    `json:"id"`
	OrganisationID           string    `json:"organisation_id"`
	Name                     string    `json:"name"`
	Slug                     string    `json:"slug"`
	Description              string    `json:"description"`
	PublicMCPEnabled         bool      `json:"public_mcp_enabled"`
	DefaultReleasePolicy     string    `json:"default_release_policy"`
	RequirePromotionApproval bool      `json:"require_promotion_approval"`
	CatalogRevision          int64     `json:"catalog_revision"`
	Revision                 int64     `json:"revision"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type Integration struct {
	ID                       string                      `json:"id"`
	DeploymentID             string                      `json:"deployment_id"`
	OrganisationID           string                      `json:"organisation_id"`
	FamilyKey                string                      `json:"family_key"`
	VersionKey               string                      `json:"version_key"`
	DisplayName              string                      `json:"display_name"`
	Description              string                      `json:"description"`
	Visibility               Visibility                  `json:"visibility"`
	Lifecycle                string                      `json:"lifecycle"`
	ReplacementIntegrationID string                      `json:"replacement_integration_id,omitempty"`
	SunsetAt                 *time.Time                  `json:"sunset_at,omitempty"`
	Revision                 int64                       `json:"revision"`
	Resources                []IntegrationResourceLink   `json:"resources,omitempty"`
	Packages                 []IntegrationPackageBinding `json:"packages,omitempty"`
	AccessConnections        []string                    `json:"access_connection_ids,omitempty"`
	SupportRouteID           string                      `json:"support_route_id,omitempty"`
	CreatedAt                time.Time                   `json:"created_at"`
	UpdatedAt                time.Time                   `json:"updated_at"`
}

// PackageArtifact is the registry-neutral identity of an SDK or package.
// DokoSoko records catalogue metadata only: package bytes remain hosted and
// delivered by the package's registry.
type PackageArtifact struct {
	ID                           string          `json:"id"`
	DeploymentID                 string          `json:"deployment_id"`
	OrganisationID               string          `json:"organisation_id"`
	Name                         string          `json:"name"`
	Description                  string          `json:"description"`
	Ecosystem                    string          `json:"ecosystem"`
	Coordinate                   string          `json:"coordinate"`
	PURL                         string          `json:"purl"`
	RegistryURL                  string          `json:"registry_url"`
	SourceURL                    string          `json:"source_url,omitempty"`
	Language                     string          `json:"language,omitempty"`
	Platform                     string          `json:"platform,omitempty"`
	Visibility                   Visibility      `json:"visibility"`
	Lifecycle                    string          `json:"lifecycle"`
	ReplacementPackageArtifactID string          `json:"replacement_package_artifact_id,omitempty"`
	DeprecationMessage           string          `json:"deprecation_message,omitempty"`
	SunsetAt                     *time.Time      `json:"sunset_at,omitempty"`
	Revision                     int64           `json:"revision"`
	LatestRelease                *PackageRelease `json:"latest_release,omitempty"`
	UsedBy                       []string        `json:"integration_ids,omitempty"`
	CreatedAt                    time.Time       `json:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at"`
}

// PackageRelease is an immutable, exact registry release. Its content hash
// covers all package-consumer metadata, making it safe to pin in an
// Integration revision without a follow-latest interpretation.
type PackageRelease struct {
	ID                string     `json:"id"`
	PackageArtifactID string     `json:"package_artifact_id"`
	ArtifactName      string     `json:"artifact_name"`
	Ecosystem         string     `json:"ecosystem"`
	Coordinate        string     `json:"coordinate"`
	Version           string     `json:"version"`
	PURL              string     `json:"purl"`
	RegistryURL       string     `json:"registry_url"`
	SourceURL         string     `json:"source_url,omitempty"`
	Language          string     `json:"language,omitempty"`
	Platform          string     `json:"platform,omitempty"`
	InstallCommand    string     `json:"install_command"`
	Digest            string     `json:"digest"`
	ProvenanceURL     string     `json:"provenance_url,omitempty"`
	SBOMURL           string     `json:"sbom_url,omitempty"`
	Visibility        Visibility `json:"visibility"`
	ContentHash       string     `json:"content_hash"`
	PublishedBy       string     `json:"published_by,omitempty"`
	PublishedAt       time.Time  `json:"published_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// IntegrationPackageBinding is compatibility by construction: it always
// names one exact immutable PackageRelease. There is deliberately no
// follow-latest flag.
type IntegrationPackageBinding struct {
	ID                string           `json:"id"`
	DeploymentID      string           `json:"deployment_id"`
	IntegrationID     string           `json:"integration_id"`
	PackageArtifactID string           `json:"package_artifact_id"`
	PackageReleaseID  string           `json:"package_release_id"`
	Artifact          *PackageArtifact `json:"artifact,omitempty"`
	Release           *PackageRelease  `json:"release,omitempty"`
	CreatedBy         string           `json:"created_by,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// BackendConnection is an operator-managed service-to-service connection.
// CredentialSecretID is deliberately excluded from the public representation;
// callers only see the non-secret fingerprint of the active credential.
type BackendConnection struct {
	ID                    string    `json:"id"`
	DeploymentID          string    `json:"deployment_id"`
	OrganisationID        string    `json:"organisation_id"`
	Name                  string    `json:"name"`
	BaseURL               string    `json:"base_url"`
	AuthenticationType    string    `json:"authentication_type"`
	CredentialSecretID    string    `json:"-"`
	CredentialFingerprint string    `json:"credential_fingerprint,omitempty"`
	State                 string    `json:"state"`
	Revision              int64     `json:"revision"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type BackendConnectionCredential struct {
	ID                  string    `json:"id"`
	BackendConnectionID string    `json:"backend_connection_id"`
	Fingerprint         string    `json:"fingerprint"`
	ConnectionRevision  int64     `json:"connection_revision"`
	CreatedAt           time.Time `json:"created_at"`
}

type IntegrationRevision struct {
	ID            string          `json:"id"`
	IntegrationID string          `json:"integration_id"`
	Revision      int64           `json:"revision"`
	State         string          `json:"state"`
	Snapshot      json.RawMessage `json:"snapshot"`
	ManifestHash  string          `json:"manifest_hash"`
	PublishedBy   string          `json:"published_by,omitempty"`
	PublishedAt   *time.Time      `json:"published_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type ResourceSet struct {
	ID             string               `json:"id"`
	DeploymentID   string               `json:"deployment_id"`
	OrganisationID string               `json:"organisation_id"`
	Kind           string               `json:"kind"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	State          string               `json:"state"`
	Revision       int64                `json:"revision"`
	Latest         *ResourceSetRevision `json:"latest_revision,omitempty"`
	UsedBy         []string             `json:"integration_ids,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type ResourceSetRevision struct {
	ID            string          `json:"id"`
	ResourceSetID string          `json:"resource_set_id"`
	Revision      int64           `json:"revision"`
	Manifest      json.RawMessage `json:"manifest"`
	ContentHash   string          `json:"content_hash"`
	CreatedBy     string          `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type IntegrationResourceLink struct {
	ID               string               `json:"id"`
	IntegrationID    string               `json:"integration_id"`
	ResourceSetID    string               `json:"resource_set_id"`
	Kind             string               `json:"kind"`
	Name             string               `json:"name"`
	FollowLatest     bool                 `json:"follow_latest"`
	PinnedRevisionID string               `json:"pinned_revision_id,omitempty"`
	ResolvedRevision *ResourceSetRevision `json:"resolved_revision,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type AccessDefinition struct {
	ID                    string          `json:"id"`
	DeploymentID          string          `json:"deployment_id"`
	OrganisationID        string          `json:"organisation_id"`
	ServiceKey            string          `json:"service_key"`
	Name                  string          `json:"name"`
	InstanceCardinality   string          `json:"instance_cardinality"`
	InstanceLabelSingular string          `json:"instance_label_singular"`
	InstanceLabelPlural   string          `json:"instance_label_plural"`
	CredentialScope       string          `json:"credential_scope"`
	ManagementAuthType    string          `json:"management_auth_type"`
	APIResourceSetID      string          `json:"api_resource_set_id,omitempty"`
	Operations            json.RawMessage `json:"operations"`
	State                 string          `json:"state"`
	Revision              int64           `json:"revision"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type AccessConnection struct {
	ID                 string            `json:"id"`
	DeploymentID       string            `json:"deployment_id"`
	OrganisationID     string            `json:"organisation_id"`
	AccessDefinitionID string            `json:"access_definition_id"`
	EnvironmentID      string            `json:"environment_id,omitempty"`
	Name               string            `json:"name"`
	Region             string            `json:"region,omitempty"`
	BaseURL            string            `json:"-"`
	ManagementSecretID string            `json:"-"`
	LegacyProviderID   string            `json:"-"`
	Config             json.RawMessage   `json:"config"`
	State              string            `json:"state"`
	Revision           int64             `json:"revision"`
	Definition         *AccessDefinition `json:"definition,omitempty"`
	IntegrationIDs     []string          `json:"integration_ids,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type AccessInstance struct {
	ID                 string          `json:"id"`
	DeploymentID       string          `json:"deployment_id"`
	OrganisationID     string          `json:"organisation_id"`
	AccessConnectionID string          `json:"access_connection_id"`
	EnvironmentID      string          `json:"environment_id"`
	OwnerType          string          `json:"owner_type"`
	OwnerID            string          `json:"owner_id"`
	ExternalID         string          `json:"external_id"`
	DisplayName        string          `json:"display_name"`
	IdempotencyKey     string          `json:"idempotency_key"`
	State              string          `json:"state"`
	ProviderMetadata   json.RawMessage `json:"provider_metadata"`
	ExpiresAt          *time.Time      `json:"expires_at,omitempty"`
	IntegrationIDs     []string        `json:"integration_ids,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type AccessCredential struct {
	ID                 string     `json:"id"`
	DeploymentID       string     `json:"deployment_id"`
	OrganisationID     string     `json:"organisation_id"`
	AccessConnectionID string     `json:"access_connection_id"`
	AccessInstanceID   string     `json:"access_instance_id,omitempty"`
	EnvironmentID      string     `json:"environment_id"`
	SubjectID          string     `json:"subject_id"`
	ExternalID         string     `json:"external_id,omitempty"`
	IdempotencyKey     string     `json:"idempotency_key,omitempty"`
	Scopes             []string   `json:"scopes"`
	SecretFingerprint  string     `json:"secret_fingerprint"`
	StorageMode        string     `json:"storage_mode"`
	EncryptedSecretID  string     `json:"-"`
	State              string     `json:"state"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	RotatedFromID      string     `json:"rotated_from_id,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type SupportRoute struct {
	ID                  string    `json:"id"`
	DeploymentID        string    `json:"deployment_id"`
	OrganisationID      string    `json:"organisation_id"`
	Name                string    `json:"name"`
	IsDefault           bool      `json:"is_default"`
	BugReportsEnabled   bool      `json:"bug_reports_enabled"`
	FeedbackEnabled     bool      `json:"feedback_enabled"`
	BackendConnectionID string    `json:"backend_connection_id,omitempty"`
	RetentionDays       int       `json:"retention_days"`
	State               string    `json:"state"`
	Revision            int64     `json:"revision"`
	IntegrationIDs      []string  `json:"integration_ids,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
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

type IntegrationEvidence struct {
	Kind        string            `json:"kind"`
	ResourceID  string            `json:"resource_id"`
	Label       string            `json:"label"`
	Location    string            `json:"location,omitempty"`
	Excerpt     string            `json:"excerpt,omitempty"`
	References  []RecipeReference `json:"references,omitempty"`
	Version     string            `json:"version,omitempty"`
	Visibility  Visibility        `json:"visibility"`
	Fingerprint string            `json:"fingerprint"`
}

type IntegrationUnknown struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Why      string `json:"why"`
	Blocking bool   `json:"blocking"`
	Answer   string `json:"answer,omitempty"`
}

type IntegrationEndpointPlan struct {
	Name     string   `json:"name"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Purpose  string   `json:"purpose"`
	Identity string   `json:"identity"`
	Evidence []string `json:"evidence"`
}

type IntegrationIdentityPlan struct {
	Mode        string   `json:"mode"`
	Issuer      string   `json:"issuer,omitempty"`
	Audience    string   `json:"audience,omitempty"`
	Grants      []string `json:"grants,omitempty"`
	Explanation string   `json:"explanation"`
}

type RecipeSeed struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Outcome     string   `json:"outcome"`
	Audience    string   `json:"audience"`
	EndpointIDs []string `json:"endpoint_ids,omitempty"`
}

type IntegrationPlan struct {
	Summary   string                    `json:"summary"`
	Identity  IntegrationIdentityPlan   `json:"identity"`
	Endpoints []IntegrationEndpointPlan `json:"endpoints"`
	Recipes   []RecipeSeed              `json:"recipes"`
}

type IntegrationAnalysis struct {
	ID             string                `json:"id"`
	OrganisationID string                `json:"organisation_id"`
	ProductID      string                `json:"product_id"`
	SchemaVersion  int                   `json:"schema_version"`
	State          string                `json:"state"`
	GeneratedBy    string                `json:"generated_by"`
	Evidence       []IntegrationEvidence `json:"evidence"`
	Plan           IntegrationPlan       `json:"plan"`
	Unknowns       []IntegrationUnknown  `json:"unknowns"`
	ErrorCode      string                `json:"error_code,omitempty"`
	Revision       int64                 `json:"revision"`
	CreatedAt      time.Time             `json:"created_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
}

type RecipeReference struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Anchor     string `json:"anchor,omitempty"`
}

type RecipeDependency struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Version    string `json:"version"`
}

type RecipeValidationFinding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RecipeRevision struct {
	ID          string                    `json:"id"`
	RecipeID    string                    `json:"recipe_id"`
	Revision    int                       `json:"revision"`
	Markdown    string                    `json:"markdown"`
	References  []RecipeReference         `json:"references"`
	Validation  []RecipeValidationFinding `json:"validation"`
	Review      string                    `json:"review,omitempty"`
	GeneratedBy string                    `json:"generated_by"`
	Model       string                    `json:"model,omitempty"`
	CreatedBy   string                    `json:"created_by"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type Recipe struct {
	ID                string             `json:"id"`
	OrganisationID    string             `json:"organisation_id"`
	ProductID         string             `json:"product_id"`
	AnalysisID        string             `json:"analysis_id,omitempty"`
	Slug              string             `json:"slug"`
	Title             string             `json:"title"`
	Outcome           string             `json:"outcome"`
	Audience          string             `json:"audience"`
	State             string             `json:"state"`
	Generated         bool               `json:"generated"`
	NeedsAttention    bool               `json:"needs_attention"`
	Visibility        Visibility         `json:"visibility"`
	Dependencies      []RecipeDependency `json:"dependencies"`
	CurrentRevisionID string             `json:"current_revision_id"`
	CurrentRevision   *RecipeRevision    `json:"current_revision,omitempty"`
	StableURI         string             `json:"stable_uri"`
	ApprovedBy        string             `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time         `json:"approved_at,omitempty"`
	PublishedAt       *time.Time         `json:"published_at,omitempty"`
	Revision          int64              `json:"revision"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type AIJob struct {
	ID             string          `json:"id"`
	OrganisationID string          `json:"organisation_id"`
	ProductID      string          `json:"product_id"`
	Kind           string          `json:"kind"`
	TargetID       string          `json:"target_id,omitempty"`
	State          string          `json:"state"`
	Attempt        int             `json:"attempt"`
	Input          json.RawMessage `json:"input"`
	Output         json.RawMessage `json:"output,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
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

// CrawlReviewDocument is an immutable document candidate produced by one
// specific crawl generation. Reused snapshots are linked to every generation
// that observed them so a reviewer always approves a complete, exact set.
type CrawlReviewDocument struct {
	ID                  string          `json:"id"`
	CrawlJobID          string          `json:"crawl_job_id"`
	SnapshotID          string          `json:"snapshot_id"`
	Title               string          `json:"title"`
	CanonicalURL        string          `json:"canonical_url"`
	State               string          `json:"state"`
	TrustLevel          int             `json:"trust_level"`
	InjectionIndicators json.RawMessage `json:"injection_indicators"`
	ContentHash         string          `json:"content_hash"`
	Changed             bool            `json:"changed"`
}

// SourcePublication pins the reviewed crawl generation and the selected
// immutable documents which may be referenced by Integration resource sets.
type SourcePublication struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProductID      string     `json:"product_id"`
	SourceID       string     `json:"source_id"`
	CrawlJobID     string     `json:"crawl_job_id"`
	Revision       int64      `json:"revision"`
	Visibility     Visibility `json:"visibility"`
	ContentHash    string     `json:"content_hash"`
	DocumentCount  int        `json:"document_count"`
	ReviewedBy     string     `json:"reviewed_by"`
	ReviewedAt     time.Time  `json:"reviewed_at"`
	PublishedAt    time.Time  `json:"published_at"`
}

type SourceReview struct {
	Source      Source                `json:"source"`
	CrawlJob    CrawlJob              `json:"crawl_job"`
	Documents   []CrawlReviewDocument `json:"documents"`
	Publication *SourcePublication    `json:"publication,omitempty"`
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

// Widget is an authenticated delivery channel. It references integrations
// that already belong to the deployment; session callers cannot expand this
// allow-list while minting a token.
type Widget struct {
	ID                  string                     `json:"id"`
	DeploymentID        string                     `json:"deployment_id"`
	OrganisationID      string                     `json:"organisation_id"`
	Name                string                     `json:"name"`
	State               string                     `json:"state"`
	AllowedOrigins      []string                   `json:"allowed_origins"`
	IntegrationIDs      []string                   `json:"integration_ids"`
	IntegrationBindings []WidgetIntegrationBinding `json:"integration_bindings"`
	Appearance          json.RawMessage            `json:"appearance"`
	Revision            int64                      `json:"revision"`
	ActivatedAt         *time.Time                 `json:"activated_at,omitempty"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

// WidgetIntegrationBinding is the widget's activation-time pin to one exact
// immutable Integration publication. Runtime widget requests consume Snapshot
// directly and never rebuild their allowed catalog from mutable Integration
// rows or follow a later publication implicitly.
type WidgetIntegrationBinding struct {
	IntegrationID         string          `json:"integration_id"`
	IntegrationRevisionID string          `json:"integration_revision_id"`
	IntegrationRevision   int64           `json:"integration_revision"`
	ManifestHash          string          `json:"manifest_hash"`
	Snapshot              json.RawMessage `json:"snapshot"`
	BoundAt               time.Time       `json:"bound_at"`
}

// WidgetSecret stores only a SHA-256 digest. The raw credential is returned
// exactly once by the control-plane operation that creates it.
type WidgetSecret struct {
	ID          string     `json:"id"`
	WidgetID    string     `json:"widget_id"`
	Digest      []byte     `json:"-"`
	Fingerprint string     `json:"fingerprint"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// WidgetBootstrap is a one-time, origin-bound token created by a customer's
// trusted backend. It never carries customer credentials or requested scopes.
type WidgetBootstrap struct {
	Digest                 []byte
	WidgetID               string
	UserID                 string
	CustomerOrganisationID string
	Origin                 string
	ExpiresAt              time.Time
	UsedAt                 *time.Time
	CreatedAt              time.Time
}

// WidgetSession is the short-lived bearer accepted by the hosted widget
// runtime. Authorization remains the current Widget configuration.
type WidgetSession struct {
	ID                     string     `json:"id"`
	WidgetID               string     `json:"widget_id"`
	Digest                 []byte     `json:"-"`
	UserID                 string     `json:"user_id"`
	CustomerOrganisationID string     `json:"customer_organisation_id,omitempty"`
	Origin                 string     `json:"origin"`
	ExpiresAt              time.Time  `json:"expires_at"`
	RevokedAt              *time.Time `json:"revoked_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	LastSeenAt             *time.Time `json:"last_seen_at,omitempty"`
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

// ConnectorRun is the deployment terminology for the legacy IntegrationRun.
// The alias keeps persisted data and compatibility handlers stable while new
// APIs avoid confusing a run with a first-class Integration.
type ConnectorRun = IntegrationRun

// ReportSubmission is the durable outbox record. PayloadCiphertext contains
// both user-authored content and trusted reporter/product context. Only routing
// and delivery state are stored in plaintext.
type ReportSubmission struct {
	ID                  string          `json:"id"`
	OrganisationID      string          `json:"organisation_id"`
	ProductID           string          `json:"product_id"`
	IntegrationID       string          `json:"integration_id,omitempty"`
	IntegrationSnapshot json.RawMessage `json:"integration_snapshot,omitempty"`
	SupportRouteID      string          `json:"support_route_id,omitempty"`
	Kind                string          `json:"kind"`
	State               string          `json:"state"`
	ActorPseudonym      string          `json:"actor_pseudonym"`
	IdempotencyDigest   []byte          `json:"-"`
	PayloadCiphertext   []byte          `json:"-"`
	PayloadNonce        []byte          `json:"-"`
	PayloadKeyVersion   int             `json:"-"`
	PayloadFingerprint  string          `json:"-"`
	Attempts            int             `json:"attempts"`
	NextAttemptAt       *time.Time      `json:"next_attempt_at,omitempty"`
	DeliveryStartedAt   *time.Time      `json:"delivery_started_at,omitempty"`
	LastError           string          `json:"last_error,omitempty"`
	ExternalID          string          `json:"external_id,omitempty"`
	ExternalURL         string          `json:"external_url,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	DeliveredAt         *time.Time      `json:"delivered_at,omitempty"`
	ExpiresAt           time.Time       `json:"expires_at"`
}

type LLMProfile struct {
	ID                  string          `json:"id"`
	OrganisationID      string          `json:"organisation_id"`
	ProductID           string          `json:"product_id"`
	Role                string          `json:"role"`
	Provider            string          `json:"provider"`
	Endpoint            string          `json:"endpoint"`
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

// AIProviderConnection owns one provider credential and its transport
// boundary. Workload profiles reference this record so the same credential is
// not copied between Analysis and Assistant.
type AIProviderConnection struct {
	ID             string          `json:"id"`
	OrganisationID string          `json:"organisation_id"`
	DeploymentID   string          `json:"deployment_id"`
	Provider       string          `json:"provider"`
	Endpoint       string          `json:"endpoint"`
	CredentialID   string          `json:"-"`
	ManagedBy      string          `json:"managed_by"`
	Enabled        bool            `json:"enabled"`
	IsBackup       bool            `json:"is_backup"`
	BackupModels   json.RawMessage `json:"backup_models"`
	LastTestedAt   *time.Time      `json:"last_tested_at,omitempty"`
	LastErrorCode  string          `json:"last_error_code,omitempty"`
	Revision       int64           `json:"revision"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type AIWorkloadProfile struct {
	ID                   string          `json:"id"`
	OrganisationID       string          `json:"organisation_id"`
	ProductID            string          `json:"product_id"`
	Workload             string          `json:"workload"`
	ProviderConnectionID string          `json:"provider_connection_id"`
	Model                string          `json:"model"`
	MaxInputTokens       int             `json:"max_input_tokens"`
	MaxOutputTokens      int             `json:"max_output_tokens"`
	DailyTokenBudget     int64           `json:"daily_token_budget"`
	Hardening            json.RawMessage `json:"hardening"`
	Enabled              bool            `json:"enabled"`
	Revision             int64           `json:"revision"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// AIBudgetReservation makes daily limits concurrency-safe. The caller must
// finish the reservation after every provider attempt; abandoned reservations
// expire automatically and stop counting against the budget.
type AIBudgetReservation struct {
	ID             string    `json:"id"`
	ProductID      string    `json:"product_id"`
	Workload       string    `json:"workload"`
	Day            time.Time `json:"day"`
	ReservedTokens int64     `json:"reserved_tokens"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type AIUsageEvent struct {
	ID                string        `json:"id"`
	OrganisationID    string        `json:"organisation_id"`
	ProductID         string        `json:"product_id"`
	Workload          string        `json:"workload"`
	Action            string        `json:"action"`
	Provider          string        `json:"provider"`
	ProviderRole      string        `json:"provider_role"`
	FallbackReason    string        `json:"fallback_reason,omitempty"`
	RequestedModel    string        `json:"requested_model"`
	ResolvedModel     string        `json:"resolved_model"`
	ProviderRequestID string        `json:"provider_request_id,omitempty"`
	InputTokens       int64         `json:"input_tokens"`
	OutputTokens      int64         `json:"output_tokens"`
	Duration          time.Duration `json:"-"`
	DurationMS        int64         `json:"duration_ms"`
	Outcome           string        `json:"outcome"`
	ErrorCode         string        `json:"error_code,omitempty"`
	PromptVersion     string        `json:"prompt_version"`
	CreatedAt         time.Time     `json:"created_at"`
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

// RecipePopularity measures which published guidance developers actually use.
// Views and plan selections are kept separate so opening a recipe is not
// mistaken for choosing it as the implementation path.
type RecipePopularity struct {
	RecipeID       string `json:"recipe_id"`
	RecipeSlug     string `json:"recipe_slug"`
	Views          int64  `json:"views"`
	PlanSelections int64  `json:"plan_selections"`
}

type AnalyticsSummary struct {
	ActiveDevelopers int64            `json:"active_developers"`
	AuthorizedUsers  int64            `json:"authorized_users"`
	MCPRequests      int64            `json:"mcp_requests"`
	ToolCalls        int64            `json:"tool_calls"`
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
