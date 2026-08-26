package model

import (
	"encoding/json"
	"time"
)

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

type IntegrationManifestSDK struct {
	ID               string     `json:"id"`
	Ecosystem        string     `json:"ecosystem"`
	Coordinate       string     `json:"coordinate"`
	ExactVersion     string     `json:"exact_version"`
	InstallCommand   string     `json:"install_command"`
	DocumentationURL string     `json:"documentation_url,omitempty"`
	SourceURL        string     `json:"source_url,omitempty"`
	Checksum         string     `json:"checksum,omitempty"`
	Visibility       Visibility `json:"visibility"`
	Revision         int64      `json:"revision"`
	ContentHash      string     `json:"content_hash"`
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
	Effect                     string `json:"effect"`
	IdempotencyMode            string `json:"idempotency_mode"`
	IdentityRequirement        string `json:"identity_requirement"`
	StateScope                 string `json:"state_scope"`
	MaxConcurrency             int    `json:"max_concurrency,omitempty"`
	MaxResultBytes             int64  `json:"max_result_bytes,omitempty"`
	ContentHash                string `json:"content_hash"`
	UpstreamSchemaHash         string `json:"upstream_schema_hash,omitempty"`
	NativePluginID             string `json:"native_plugin_id,omitempty"`
	NativeToolID               string `json:"native_tool_id,omitempty"`
	NativePluginVersion        string `json:"native_plugin_version,omitempty"`
	NativeSDKVersion           int    `json:"native_sdk_version,omitempty"`
	NativeManifestHash         string `json:"native_manifest_hash,omitempty"`
	NativeContractHash         string `json:"native_contract_hash,omitempty"`
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
	SDKs                     []IntegrationManifestSDK                `json:"sdks"`
	AuthorizationPoints      []IntegrationManifestAuthorizationPoint `json:"authorization_points"`
	Tools                    []IntegrationManifestTool               `json:"tools"`
	ServiceConnections       []IntegrationManifestServiceConnection  `json:"service_connections"`
}

// ProductManifest is the current deployment catalog. API publications carry
// their own immutable revisions and hashes; the deployment has no release
// channels, pins, staged rollout, or derived product version.
type ProductManifest struct {
	DeploymentID            string                `json:"deployment_id"`
	DeploymentSlug          string                `json:"deployment_slug"`
	DeploymentName          string                `json:"deployment_name"`
	ProductID               string                `json:"product_id"`
	ProductSlug             string                `json:"product_slug"`
	ProductName             string                `json:"product_name"`
	Description             string                `json:"description"`
	CatalogRevision         int64                 `json:"catalog_revision"`
	ManagedIntegrationTools bool                  `json:"managed_integration_tools"`
	Integrations            []IntegrationManifest `json:"integrations"`
}

type CatalogScope struct {
	Public bool
}
