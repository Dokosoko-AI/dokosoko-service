package model

import (
	"encoding/json"
	"time"
)

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
