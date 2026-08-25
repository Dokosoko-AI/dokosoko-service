package model

import (
	"encoding/json"
	"time"
)

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
