package model

import (
	"encoding/json"
	"time"
)

// AIProviderConnection owns one provider credential and its transport boundary.
// The Analysis workload references this record instead of copying credentials.
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
	ID                   string    `json:"id"`
	OrganisationID       string    `json:"organisation_id"`
	ProductID            string    `json:"product_id"`
	Workload             string    `json:"workload"`
	ProviderConnectionID string    `json:"provider_connection_id"`
	Model                string    `json:"model"`
	MaxInputTokens       int       `json:"max_input_tokens"`
	MaxOutputTokens      int       `json:"max_output_tokens"`
	DailyTokenBudget     int64     `json:"daily_token_budget"`
	Enabled              bool      `json:"enabled"`
	Revision             int64     `json:"revision"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AIPromptState is the mutable state for one server-owned prompt definition.
// Empty Instructions selects the server default while retaining the revision,
// so resetting an override cannot reintroduce an earlier revision (the ABA
// problem). Labels, descriptions, defaults, versions, and immutable safety
// policy remain owned by the server binary.
type AIPromptState struct {
	ProductID    string    `json:"product_id"`
	Key          string    `json:"key"`
	Instructions string    `json:"instructions,omitempty"`
	Revision     int64     `json:"revision"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AIPromptConfiguration is the effective administrative view assembled from a
// server-owned definition and an optional persisted override.
type AIPromptConfiguration struct {
	Key              string     `json:"key"`
	Label            string     `json:"label"`
	Description      string     `json:"description"`
	Instructions     string     `json:"instructions"`
	DefaultVersion   string     `json:"default_version"`
	EffectiveVersion string     `json:"effective_version"`
	Source           string     `json:"source"`
	Revision         int64      `json:"revision"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
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
