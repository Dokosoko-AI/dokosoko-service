package model

import "time"

// GrantDefinition is deployment-owned authorization vocabulary. Evaluators
// may return these keys, and policies may require them; the definition itself
// never grants access.
type GrantDefinition struct {
	ID             string    `json:"id"`
	DeploymentID   string    `json:"deployment_id"`
	OrganisationID string    `json:"organisation_id"`
	Key            string    `json:"key"`
	DisplayName    string    `json:"display_name"`
	Description    string    `json:"description"`
	Risk           string    `json:"risk"`
	State          string    `json:"state"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AuthorizationPoint binds a named Integration action to declarative policy.
// It intentionally has no URL or credential field: live customer decisions
// use the deployment's fixed, versioned access-evaluation contract.
type AuthorizationPoint struct {
	ID                   string    `json:"id"`
	DeploymentID         string    `json:"deployment_id"`
	OrganisationID       string    `json:"organisation_id"`
	IntegrationID        string    `json:"integration_id"`
	Key                  string    `json:"key"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	ActionType           string    `json:"action_type"`
	RequiredGrants       []string  `json:"required_grants"`
	ConfirmationRequired bool      `json:"confirmation_required"`
	DecisionTTLSeconds   int       `json:"decision_ttl_seconds"`
	State                string    `json:"state"`
	Revision             int64     `json:"revision"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// IntegrationToolBinding records an exact published tool revision and the
// exact active authorization-point revision governing that Integration action.
// Tool definitions remain reusable deployment assets and are never owned by
// one API.
type IntegrationToolBinding struct {
	IntegrationID              string              `json:"integration_id"`
	ToolID                     string              `json:"tool_id"`
	ToolRevision               int64               `json:"tool_revision"`
	AuthorizationPointID       string              `json:"authorization_point_id"`
	AuthorizationPointRevision int64               `json:"authorization_point_revision"`
	Tool                       *Tool               `json:"tool,omitempty"`
	AuthorizationPoint         *AuthorizationPoint `json:"authorization_point,omitempty"`
	CreatedBy                  string              `json:"created_by,omitempty"`
	CreatedAt                  time.Time           `json:"created_at"`
}
