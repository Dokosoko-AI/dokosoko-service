package model

import (
	"time"
)

type AuditEvent struct {
	ID             string         `json:"id"`
	OrganisationID string         `json:"organisation_id"`
	ProductID      string         `json:"product_id,omitempty"`
	ActorID        string         `json:"actor_id"`
	Action         string         `json:"action"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id"`
	Prior          map[string]any `json:"prior,omitempty"`
	Current        map[string]any `json:"current,omitempty"`
	RequestID      string         `json:"request_id"`
	Outcome        string         `json:"outcome,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type KnowledgeRecord struct {
	ID         string     `json:"id"`
	ProductID  string     `json:"product_id"`
	SourceID   string     `json:"source_id"`
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	URL        string     `json:"url"`
	Visibility Visibility `json:"visibility"`
	Published  bool       `json:"published"`
}

type AnalyticsEvent struct {
	OrganisationID string         `json:"organisation_id"`
	ProductID      string         `json:"product_id"`
	EventName      string         `json:"event_name"`
	ActorKind      string         `json:"actor_kind"`
	ActorPseudonym string         `json:"-"`
	Dimensions     map[string]any `json:"dimensions,omitempty"`
	Value          float64        `json:"value,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}
