package model

import (
	"encoding/json"
	"time"
)

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
