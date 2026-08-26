package model

import "time"

type IntegrationEvidence struct {
	Kind        string            `json:"kind"`
	ResourceID  string            `json:"resource_id"`
	Label       string            `json:"label"`
	Location    string            `json:"location,omitempty"`
	Excerpt     string            `json:"excerpt,omitempty"`
	References  []RecipeReference `json:"references,omitempty"`
	Version     string            `json:"version,omitempty"`
	Visibility  Visibility        `json:"visibility"`
	Fingerprint string            `json:"fingerprint"`
}

type IntegrationUnknown struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Why      string `json:"why"`
	Blocking bool   `json:"blocking"`
}

type IntegrationEndpointPlan struct {
	Name     string   `json:"name"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Purpose  string   `json:"purpose"`
	Identity string   `json:"identity"`
	Evidence []string `json:"evidence"`
}

type IntegrationIdentityPlan struct {
	Mode        string   `json:"mode"`
	Issuer      string   `json:"issuer,omitempty"`
	Audience    string   `json:"audience,omitempty"`
	Grants      []string `json:"grants,omitempty"`
	Explanation string   `json:"explanation"`
}

type RecipeSeed struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Outcome     string   `json:"outcome"`
	Audience    string   `json:"audience"`
	EndpointIDs []string `json:"endpoint_ids,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type IntegrationPlan struct {
	Summary   string                    `json:"summary"`
	Identity  IntegrationIdentityPlan   `json:"identity"`
	Endpoints []IntegrationEndpointPlan `json:"endpoints"`
	Recipes   []RecipeSeed              `json:"recipes"`
}

type IntegrationAnalysis struct {
	ID             string                `json:"id"`
	OrganisationID string                `json:"organisation_id"`
	ProductID      string                `json:"product_id"`
	SchemaVersion  int                   `json:"schema_version"`
	State          string                `json:"state"`
	GeneratedBy    string                `json:"generated_by"`
	Evidence       []IntegrationEvidence `json:"evidence"`
	Plan           IntegrationPlan       `json:"plan"`
	Unknowns       []IntegrationUnknown  `json:"unknowns"`
	ErrorCode      string                `json:"error_code,omitempty"`
	Revision       int64                 `json:"revision"`
	CreatedAt      time.Time             `json:"created_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
}

type RecipeReference struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Anchor     string `json:"anchor,omitempty"`
}

type RecipeDependency struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Version    string `json:"version"`
}

type RecipeValidationFinding struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RecipeRevision struct {
	ID          string                    `json:"id"`
	RecipeID    string                    `json:"recipe_id"`
	Revision    int                       `json:"revision"`
	Markdown    string                    `json:"markdown"`
	References  []RecipeReference         `json:"references"`
	Validation  []RecipeValidationFinding `json:"validation"`
	Review      string                    `json:"review,omitempty"`
	GeneratedBy string                    `json:"generated_by"`
	Model       string                    `json:"model,omitempty"`
	CreatedBy   string                    `json:"created_by"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type Recipe struct {
	ID                string             `json:"id"`
	OrganisationID    string             `json:"organisation_id"`
	ProductID         string             `json:"product_id"`
	AnalysisID        string             `json:"analysis_id,omitempty"`
	Slug              string             `json:"slug"`
	Title             string             `json:"title"`
	Outcome           string             `json:"outcome"`
	Audience          string             `json:"audience"`
	State             string             `json:"state"`
	Generated         bool               `json:"generated"`
	NeedsAttention    bool               `json:"needs_attention"`
	Visibility        Visibility         `json:"visibility"`
	Dependencies      []RecipeDependency `json:"dependencies"`
	CurrentRevisionID string             `json:"current_revision_id"`
	CurrentRevision   *RecipeRevision    `json:"current_revision,omitempty"`
	StableURI         string             `json:"stable_uri"`
	ApprovedBy        string             `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time         `json:"approved_at,omitempty"`
	PublishedAt       *time.Time         `json:"published_at,omitempty"`
	Revision          int64              `json:"revision"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}
