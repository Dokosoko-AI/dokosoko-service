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
	OrganisationID   string         `json:"organisation_id"`
	ProductID        string         `json:"product_id"`
	EventName        string         `json:"event_name"`
	ActorKind        string         `json:"actor_kind"`
	ActorPseudonym   string         `json:"-"`
	IntegrationRunID string         `json:"integration_run_id,omitempty"`
	Dimensions       map[string]any `json:"dimensions,omitempty"`
	Value            float64        `json:"value,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type AnalyticsPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// RecipePopularity measures which published guidance developers actually use.
// Views and plan selections are kept separate so opening a recipe is not
// mistaken for choosing it as the implementation path.
type RecipePopularity struct {
	RecipeID       string `json:"recipe_id"`
	RecipeSlug     string `json:"recipe_slug"`
	Views          int64  `json:"views"`
	PlanSelections int64  `json:"plan_selections"`
}

type AnalyticsSummary struct {
	ActiveDevelopers int64            `json:"active_developers"`
	AuthorizedUsers  int64            `json:"authorized_users"`
	MCPRequests      int64            `json:"mcp_requests"`
	ToolCalls        int64            `json:"tool_calls"`
	IntegrationRuns  int64            `json:"integration_runs"`
	ValidatedRuns    int64            `json:"validated_runs"`
	ValidatedSuccess int64            `json:"validated_success"`
	FirstPassRate    float64          `json:"first_pass_rate"`
	Channels         map[string]int64 `json:"channels"`
	Versions         map[string]int64 `json:"versions"`
	Funnel           map[string]int64 `json:"funnel"`
	DailyRequests    []AnalyticsPoint `json:"daily_requests"`
	Since            time.Time        `json:"since"`
	GeneratedAt      time.Time        `json:"generated_at"`
}
