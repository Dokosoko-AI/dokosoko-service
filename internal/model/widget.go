package model

import (
	"encoding/json"
	"time"
)

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
	KnowledgeBindings   []WidgetKnowledgeBinding   `json:"knowledge_bindings"`
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

// WidgetKnowledgeBinding pins one exact, reviewed recipe revision to the
// widget activation that selected it. Recipes remain deployment-wide authoring
// resources; their integration dependencies decide which widgets may receive
// them. Runtime chat never follows a mutable recipe row implicitly.
type WidgetKnowledgeBinding struct {
	RecipeID         string            `json:"recipe_id"`
	RecipeRevisionID string            `json:"recipe_revision_id"`
	RecipeRevision   int               `json:"recipe_revision"`
	IntegrationIDs   []string          `json:"integration_ids"`
	Title            string            `json:"title"`
	Outcome          string            `json:"outcome"`
	Audience         string            `json:"audience"`
	StableURI        string            `json:"stable_uri"`
	Markdown         string            `json:"markdown"`
	References       []RecipeReference `json:"references"`
	ContentHash      string            `json:"content_hash"`
	BoundAt          time.Time         `json:"bound_at"`
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

const (
	WidgetSessionKindCustomer     = "customer"
	WidgetSessionKindAdminPreview = "admin_preview"
)

// WidgetContextFact is a deliberately small, display-ready fact selected by
// the customer's trusted backend. It lets the embedded assistant answer about
// the page the customer is already viewing without giving the model a customer
// identifier or arbitrary API access.
type WidgetContextFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type WidgetSessionContext struct {
	View  string              `json:"view,omitempty"`
	Title string              `json:"title,omitempty"`
	Facts []WidgetContextFact `json:"facts,omitempty"`
}

// WidgetBootstrap is a one-time, origin-bound token created by a customer's
// trusted backend. It never carries customer credentials or requested scopes.
type WidgetBootstrap struct {
	Digest                 []byte
	WidgetID               string
	Kind                   string
	UserID                 string
	CustomerOrganisationID string
	Context                WidgetSessionContext
	Origin                 string
	ExpiresAt              time.Time
	UsedAt                 *time.Time
	CreatedAt              time.Time
}

// WidgetSession is the short-lived bearer accepted by the hosted widget
// runtime. Authorization remains the current Widget configuration.
type WidgetSession struct {
	ID                     string               `json:"id"`
	WidgetID               string               `json:"widget_id"`
	Kind                   string               `json:"kind"`
	Digest                 []byte               `json:"-"`
	UserID                 string               `json:"user_id"`
	CustomerOrganisationID string               `json:"customer_organisation_id,omitempty"`
	Context                WidgetSessionContext `json:"-"`
	Origin                 string               `json:"origin"`
	ExpiresAt              time.Time            `json:"expires_at"`
	RevokedAt              *time.Time           `json:"revoked_at,omitempty"`
	CreatedAt              time.Time            `json:"created_at"`
	LastSeenAt             *time.Time           `json:"last_seen_at,omitempty"`
}

// WidgetAgentMessage is bounded, session-scoped conversation context. It is
// deliberately separate from analytics so customer questions and answers do
// not become product-wide knowledge or model-training material by accident.
type WidgetAgentMessage struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
