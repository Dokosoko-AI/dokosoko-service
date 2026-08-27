package model

import (
	"encoding/json"
	"time"
)

// RuntimeServiceConnection is the stable, API-owned identity for an upstream
// service. Mutable endpoint and authentication configuration is published as
// immutable, environment-specific revisions.
type RuntimeServiceConnection struct {
	ID               string                             `json:"id"`
	DeploymentID     string                             `json:"deployment_id"`
	OrganisationID   string                             `json:"organisation_id"`
	IntegrationID    string                             `json:"integration_id"`
	Name             string                             `json:"name"`
	Description      string                             `json:"description,omitempty"`
	State            string                             `json:"state"`
	Revision         int64                              `json:"revision"`
	CurrentRevisions []RuntimeServiceConnectionRevision `json:"current_revisions,omitempty"`
	CreatedAt        time.Time                          `json:"created_at"`
	UpdatedAt        time.Time                          `json:"updated_at"`
}

// RuntimeServiceConnectionRevision is immutable after creation. A connection
// has at most one current revision in each environment.
type RuntimeServiceConnectionRevision struct {
	ID                 string          `json:"id"`
	ConnectionID       string          `json:"connection_id"`
	EnvironmentID      string          `json:"environment_id"`
	BaseURL            string          `json:"base_url"`
	AuthenticationType string          `json:"authentication_type"`
	CredentialSetID    string          `json:"authorization_id,omitempty"`
	AuthConfig         json.RawMessage `json:"auth_config,omitempty"`
	ContentHash        string          `json:"content_hash"`
	Revision           int64           `json:"revision"`
	Current            bool            `json:"current"`
	CreatedBy          string          `json:"created_by,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

// RuntimeCredentialSet gives a secret a stable operator-facing identity.
// Dedicated sets belong to exactly one API. Shared sets are reusable by any
// API in the same deployment and environment.
type RuntimeCredentialSet struct {
	ID                  string                     `json:"id"`
	DeploymentID        string                     `json:"deployment_id"`
	OrganisationID      string                     `json:"organisation_id"`
	EnvironmentID       string                     `json:"environment_id"`
	Scope               string                     `json:"-"`
	OwnerIntegrationID  string                     `json:"-"`
	Name                string                     `json:"name"`
	EnvironmentVariable string                     `json:"environment_variable"`
	AuthenticationType  string                     `json:"authentication_type"`
	HeaderName          string                     `json:"header_name,omitempty"`
	AuthConfig          json.RawMessage            `json:"auth_config"`
	KeyManagementURL    string                     `json:"key_management_url,omitempty"`
	AccessEvaluationURL string                     `json:"access_evaluation_url"`
	UsageURL            string                     `json:"usage_url"`
	State               string                     `json:"state"`
	CredentialPresent   bool                       `json:"credential_present"`
	ActiveFingerprint   string                     `json:"active_fingerprint,omitempty"`
	Revision            int64                      `json:"revision"`
	Versions            []RuntimeCredentialVersion `json:"versions,omitempty"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

// RuntimeCredentialVersion references encrypted secret material internally.
// SecretID is never serialized or exposed by the administration API.
type RuntimeCredentialVersion struct {
	ID              string     `json:"id"`
	CredentialSetID string     `json:"authorization_id"`
	SecretID        string     `json:"-"`
	Fingerprint     string     `json:"fingerprint"`
	State           string     `json:"state"`
	CreatedBy       string     `json:"created_by,omitempty"`
	ActivatedAt     *time.Time `json:"activated_at,omitempty"`
	RetiresAt       *time.Time `json:"retires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// RuntimeSetup is the API-local Authorization read model. Runtime service
// connections remain an internal execution detail and are exposed only as
// endpoint-binding metadata.
type RuntimeSetup struct {
	Integration    Integration                `json:"integration"`
	Environments   []Environment              `json:"environments"`
	Connections    []RuntimeServiceConnection `json:"endpoint_bindings"`
	CredentialSets []RuntimeCredentialSet     `json:"authorizations"`
}

type RuntimeServiceConnectionCheck struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Ready         bool   `json:"ready"`
	Message       string `json:"message"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

type RuntimeServiceConnectionReadiness struct {
	ConnectionID string                          `json:"connection_id"`
	Ready        bool                            `json:"ready"`
	Checks       []RuntimeServiceConnectionCheck `json:"checks"`
}

// ToolRuntimeTarget is the resolved execution configuration for one
// environment. ConnectionRevisionID is immutable once a tool is published;
// credential version/secret fields are enriched from the active credential at
// read time so rotation does not require a new tool release.
type ToolRuntimeTarget struct {
	EnvironmentID              string          `json:"environment_id"`
	RuntimeServiceConnectionID string          `json:"runtime_service_connection_id"`
	ConnectionRevisionID       string          `json:"connection_revision_id"`
	BaseURL                    string          `json:"-"`
	AuthenticationType         string          `json:"-"`
	CredentialSetID            string          `json:"-"`
	AuthConfig                 json.RawMessage `json:"-"`
	CredentialVersionID        string          `json:"-"`
	CredentialSecretID         string          `json:"-"`
	CredentialFingerprint      string          `json:"-"`
	HeaderName                 string          `json:"-"`
	AccessEvaluationURL        string          `json:"-"`
	UsageURL                   string          `json:"-"`
}
