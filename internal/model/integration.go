package model

import (
	"encoding/json"
	"time"
)

type Integration struct {
	ID                       string                    `json:"id"`
	DeploymentID             string                    `json:"deployment_id"`
	OrganisationID           string                    `json:"organisation_id"`
	FamilyKey                string                    `json:"family_key"`
	VersionKey               string                    `json:"version_key"`
	DisplayName              string                    `json:"display_name"`
	Description              string                    `json:"description"`
	Visibility               Visibility                `json:"visibility"`
	Lifecycle                string                    `json:"lifecycle"`
	ReplacementIntegrationID string                    `json:"replacement_integration_id,omitempty"`
	SunsetAt                 *time.Time                `json:"sunset_at,omitempty"`
	Revision                 int64                     `json:"revision"`
	Resources                []IntegrationResourceLink `json:"resources,omitempty"`
	SDKs                     []SDKReference            `json:"sdks,omitempty"`
	CreatedAt                time.Time                 `json:"created_at"`
	UpdatedAt                time.Time                 `json:"updated_at"`
}

// SDKReference is the read-only compatibility projection of one exact
// deployment-owned SDK release attached through an API Resources binding.
type SDKReference struct {
	ID               string     `json:"id"`
	DeploymentID     string     `json:"deployment_id"`
	OrganisationID   string     `json:"organisation_id"`
	IntegrationID    string     `json:"integration_id"`
	Ecosystem        string     `json:"ecosystem"`
	Coordinate       string     `json:"coordinate"`
	ExactVersion     string     `json:"exact_version"`
	InstallCommand   string     `json:"install_command"`
	DocumentationURL string     `json:"documentation_url,omitempty"`
	SourceURL        string     `json:"source_url,omitempty"`
	Checksum         string     `json:"checksum,omitempty"`
	Visibility       Visibility `json:"visibility"`
	Revision         int64      `json:"revision"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
