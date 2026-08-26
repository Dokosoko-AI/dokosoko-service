package model

import (
	"encoding/json"
	"time"
)

const (
	RecipeContractLegacyMCPV1          = "legacy-mcp-v1"
	RecipeContractProductIntegrationV2 = "product-integration-v2"
	RecipeSpecVersion2                 = 2
)

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
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Outcome       string   `json:"outcome"`
	Audience      string   `json:"audience"`
	CapabilityIDs []string `json:"capability_ids,omitempty"`
	SDKID         string   `json:"sdk_id,omitempty"`
	EvidenceIDs   []string `json:"evidence_ids,omitempty"`
	// EndpointIDs is retained only to decode historical analysis snapshots.
	// Product-integration recipe v2 never populates or consumes it.
	EndpointIDs []string `json:"endpoint_ids,omitempty"`
}

// RecipeEvidenceRef records the exact reviewed fact that supports one recipe
// instruction. Fingerprints make the stored spec independently auditable and
// prevent a reused resource ID from silently changing meaning.
type RecipeEvidenceRef struct {
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	Fingerprint string `json:"fingerprint"`
}

// RecipeInstruction is intentionally small. A recipe is an ordered product
// implementation plan, not a workflow language or a free-form article.
type RecipeInstruction struct {
	Action         string              `json:"action"`
	ExpectedResult string              `json:"expected_result,omitempty"`
	Evidence       []RecipeEvidenceRef `json:"evidence"`
}

// RecipeSpec is the canonical recipe representation. Markdown is a rendered
// publication format; approval and drift decisions are made against this
// structured, evidence-bound spec.
type RecipeSpec struct {
	SchemaVersion int                 `json:"schema_version"`
	IntegrationID string              `json:"integration_id"`
	Title         string              `json:"title"`
	Outcome       string              `json:"outcome"`
	SDKID         string              `json:"sdk_id,omitempty"`
	CapabilityIDs []string            `json:"capability_ids"`
	Prerequisites []RecipeInstruction `json:"prerequisites"`
	Steps         []RecipeInstruction `json:"steps"`
	Checks        []RecipeInstruction `json:"checks"`
	ReferenceIDs  []string            `json:"reference_ids,omitempty"`
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
	ID                      string                    `json:"id"`
	RecipeID                string                    `json:"recipe_id"`
	Revision                int                       `json:"revision"`
	SpecVersion             int                       `json:"spec_version"`
	Spec                    json.RawMessage           `json:"spec"`
	Markdown                string                    `json:"markdown"`
	References              []RecipeReference         `json:"references"`
	Validation              []RecipeValidationFinding `json:"validation"`
	Review                  string                    `json:"review,omitempty"`
	GeneratedBy             string                    `json:"generated_by"`
	Model                   string                    `json:"model,omitempty"`
	IntegrationRevisionID   string                    `json:"integration_revision_id,omitempty"`
	IntegrationManifestHash string                    `json:"integration_manifest_hash,omitempty"`
	PromptVersion           string                    `json:"prompt_version,omitempty"`
	PromptHash              string                    `json:"prompt_hash,omitempty"`
	CreatedBy               string                    `json:"created_by"`
	CreatedAt               time.Time                 `json:"created_at"`
}

type Recipe struct {
	ID                string             `json:"id"`
	OrganisationID    string             `json:"organisation_id"`
	ProductID         string             `json:"product_id"`
	IntegrationID     string             `json:"integration_id,omitempty"`
	AnalysisID        string             `json:"analysis_id,omitempty"`
	ContractVersion   string             `json:"contract_version"`
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
