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
	ActorPseudonym    string          `json:"actor_pseudonym"`
	IdempotencyDigest []byte          `json:"-"`
	Payload           json.RawMessage `json:"payload"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ExpiresAt         time.Time       `json:"expires_at"`
}
