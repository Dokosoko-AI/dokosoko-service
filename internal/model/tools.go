package model

import (
	"encoding/json"
	"time"
)

const (
	ToolScopeCommon = "common"
	ToolScopeAPI    = "api"
)

type Tool struct {
	ID                          string              `json:"id"`
	OrganisationID              string              `json:"organisation_id"`
	ProductID                   string              `json:"product_id"`
	Scope                       string              `json:"scope"`
	OwnerIntegrationID          string              `json:"owner_integration_id,omitempty"`
	RuntimeServiceConnectionID  string              `json:"runtime_service_connection_id,omitempty"`
	HTTPPath                    string              `json:"http_path,omitempty"`
	RuntimeTargets              []ToolRuntimeTarget `json:"-"`
	RuntimeConnectionRevisionID string              `json:"-"`
	RuntimeCredentialSetID      string              `json:"-"`
	RuntimeCredentialVersionID  string              `json:"-"`
	Namespace                   string              `json:"namespace"`
	Name                        string              `json:"name"`
	Description                 string              `json:"description"`
	InputSchema                 json.RawMessage     `json:"input_schema"`
	OutputSchema                json.RawMessage     `json:"output_schema"`
	State                       string              `json:"state"`
	Revision                    int64               `json:"revision"`
	APIConnectionID             string              `json:"-"`
	BaseURL                     string              `json:"-"`
	HTTPMethod                  string              `json:"http_method"`
	UpstreamAuth                json.RawMessage     `json:"upstream_auth,omitempty"`
	CredentialID                string              `json:"-"`
	CredentialFingerprint       string              `json:"-"`
	CredentialPresent           bool                `json:"credential_present,omitempty"`
	RequestMapping              json.RawMessage     `json:"request_mapping,omitempty"`
	ResponseMapping             json.RawMessage     `json:"response_mapping,omitempty"`
	RequestExample              json.RawMessage     `json:"request_example,omitempty"`
	ResponseExample             json.RawMessage     `json:"response_example,omitempty"`
	AuthorizationPolicy         json.RawMessage     `json:"authorization_policy"`
	TimeoutMS                   int                 `json:"timeout_ms"`
	BackendKind                 string              `json:"backend_kind"`
	MCPConnectionID             string              `json:"mcp_connection_id,omitempty"`
	UpstreamToolName            string              `json:"upstream_tool_name,omitempty"`
	UpstreamSchemaHash          string              `json:"upstream_schema_hash,omitempty"`
	UpstreamAnnotations         json.RawMessage     `json:"upstream_annotations,omitempty"`
	UpstreamDrifted             bool                `json:"upstream_drifted"`
	CreatedAt                   time.Time           `json:"created_at"`
	UpdatedAt                   time.Time           `json:"updated_at"`
}

// JSONShape is a bounded, value-free description of a JSON value. Tool test
// evidence may retain object keys, JSON types, and array lengths, but never
// request or response scalar values.
type JSONShape struct {
	Type       string               `json:"type"`
	Properties map[string]JSONShape `json:"properties,omitempty"`
	Items      []JSONShape          `json:"items,omitempty"`
	Length     int                  `json:"length,omitempty"`
	Truncated  bool                 `json:"truncated,omitempty"`
}

type ToolTestFinding struct {
	Phase        string `json:"phase"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	InstancePath string `json:"instance_path,omitempty"`
	SchemaPath   string `json:"schema_path,omitempty"`
}

// ToolTestConfirmation is the durable one-time confirmation primitive used by
// both administrator live tests and managed MCP tool invocations. It contains
// only a digest of the nonce; the raw nonce is returned to the caller once and
// is never persisted. Each protocol uses a separate domain-bound argument hash.
type ToolTestConfirmation struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	ToolID         string    `json:"tool_id"`
	ToolRevision   int64     `json:"tool_revision"`
	ArgumentHash   []byte    `json:"-"`
	NonceDigest    []byte    `json:"-"`
	ActorID        string    `json:"-"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// ManagedOperationConfirmation is a one-time, server-side confirmation for a
// generated MCP operation that has no durable Tool row of its own. The
// argument hash binds the exact operation, immutable catalog binding,
// authenticated access evaluation, idempotency key, and arguments. Raw nonces
// and argument values are never persisted.
type ManagedOperationConfirmation struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	ProductID      string    `json:"product_id"`
	OperationKey   string    `json:"operation_key"`
	ArgumentHash   []byte    `json:"-"`
	NonceDigest    []byte    `json:"-"`
	ActorID        string    `json:"-"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// ToolTestRun is deliberately sanitized, short-lived evidence. It must never
// contain a destination, path, query, header, credential, argument value, or
// response scalar value.
type ToolTestRun struct {
	ID                   string            `json:"id"`
	OrganisationID       string            `json:"organisation_id"`
	ProductID            string            `json:"product_id"`
	ToolID               string            `json:"tool_id"`
	ToolRevision         int64             `json:"tool_revision"`
	ToolName             string            `json:"tool_name"`
	ActorID              string            `json:"-"`
	RequestID            string            `json:"-"`
	ArgumentHash         []byte            `json:"-"`
	Method               string            `json:"method"`
	AuthenticationType   string            `json:"authentication_type"`
	Outcome              string            `json:"outcome"`
	Phase                string            `json:"phase"`
	NetworkCallPerformed bool              `json:"network_call_performed"`
	UpstreamStatusCode   int               `json:"upstream_status_code,omitempty"`
	ResponseBytes        int64             `json:"response_bytes,omitempty"`
	RequestShape         JSONShape         `json:"request_shape"`
	ResponseShape        *JSONShape        `json:"response_shape,omitempty"`
	Findings             []ToolTestFinding `json:"findings"`
	DurationMS           int64             `json:"duration_ms"`
	ExpiresAt            time.Time         `json:"expires_at"`
	CreatedAt            time.Time         `json:"created_at"`
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
