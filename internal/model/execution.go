package model

import (
	"encoding/json"
	"time"
)

// ReportSubmission is a consent-based, plaintext support outbox record.
type ReportSubmission struct {
	ID                string          `json:"id"`
	OrganisationID    string          `json:"organisation_id"`
	ProductID         string          `json:"product_id"`
	IntegrationID     string          `json:"integration_id,omitempty"`
	Kind              string          `json:"kind"`
	State             string          `json:"state"`
	DeliveryURL       string          `json:"-"`
	Attempts          int             `json:"attempts"`
	AvailableAt       time.Time       `json:"-"`
	LeaseOwner        string          `json:"-"`
	LeasedUntil       *time.Time      `json:"-"`
	LastError         string          `json:"last_error,omitempty"`
	DeliveredAt       *time.Time      `json:"delivered_at,omitempty"`
	ActorPseudonym    string          `json:"actor_pseudonym"`
	IdempotencyDigest []byte          `json:"-"`
	Payload           json.RawMessage `json:"payload"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ExpiresAt         time.Time       `json:"expires_at"`
}

// AuthorizationUsageEvent is a durable, value-free delivery record. It pins
// the Authorization version used for the execution without retaining tool
// arguments, tool results, tokens, or plaintext credentials.
type AuthorizationUsageEvent struct {
	ID                    string          `json:"id"`
	OrganisationID        string          `json:"-"`
	ProductID             string          `json:"-"`
	IntegrationID         string          `json:"-"`
	AuthorizationID       string          `json:"authorization_id"`
	URL                   string          `json:"-"`
	AuthenticationType    string          `json:"-"`
	HeaderName            string          `json:"-"`
	AuthConfig            json.RawMessage `json:"-"`
	CredentialVersionID   string          `json:"-"`
	CredentialSecretID    string          `json:"-"`
	CredentialFingerprint string          `json:"-"`
	Payload               json.RawMessage `json:"-"`
	State                 string          `json:"state"`
	Attempts              int             `json:"attempts"`
	AvailableAt           time.Time       `json:"available_at"`
	LeaseOwner            string          `json:"-"`
	LeasedUntil           *time.Time      `json:"-"`
	LastError             string          `json:"-"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}
