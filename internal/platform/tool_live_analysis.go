package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const (
	maxToolTestAnalysisQuestionBytes = 2 << 10
	maxToolTestAnalysisMessages      = 12
	maxToolTestAnalysisHistoryBytes  = 12 << 10
	maxToolTestAnalysisEvidenceBytes = 96 << 10
)

var (
	ErrToolTestAnalysisConsentRequired  = errors.New("tool test analysis provider consent is required")
	ErrToolTestAnalysisInvalidInput     = errors.New("tool test analysis input is invalid")
	ErrToolTestAnalysisEvidenceMismatch = errors.New("tool test analysis evidence hash does not match")

	toolTestAnalysisHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	toolTestAnalysisHashValue   = regexp.MustCompile(`(?i)\bsha256:[0-9a-f]{64}\b`)
	toolTestAnalysisURL         = regexp.MustCompile(`(?i)https?://\S+`)
	toolTestAnalysisUUID        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	toolTestAnalysisNonce       = regexp.MustCompile(`\bttc_[A-Za-z0-9_-]{20,}\b`)
	toolTestAnalysisSafeName    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)
)

// ToolTestAnalysisMessage is caller-supplied, bounded conversational context.
// It is never persisted and is always treated as untrusted data.
type ToolTestAnalysisMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolTestAnalysisInput struct {
	Revision      int64                     `json:"revision"`
	EvidenceHash  string                    `json:"evidence_hash"`
	ConsentToSend bool                      `json:"consent_to_analysis_provider"`
	Question      string                    `json:"question"`
	History       []ToolTestAnalysisMessage `json:"history,omitempty"`
}

// ToolTestAnalysisProposal is a complete locally validated draft. It remains
// advisory: draft tools must be reviewed in the builder, and published tools
// must first be cloned to a new draft.
type ToolTestAnalysisProposal struct {
	ProposalID      string             `json:"proposal_id"`
	BaseToolID      string             `json:"base_tool_id"`
	BaseRevision    int64              `json:"base_revision"`
	BaseFingerprint string             `json:"base_fingerprint"`
	RequiresClone   bool               `json:"requires_clone"`
	Draft           ToolDraft          `json:"draft"`
	Changes         []ToolDraftChange  `json:"changes"`
	Findings        []ToolDraftFinding `json:"findings"`
	Valid           bool               `json:"valid"`
}

type ToolTestAnalysisResult struct {
	ToolRevision    int64                     `json:"tool_revision"`
	EvidenceHash    string                    `json:"evidence_hash"`
	Reply           string                    `json:"reply"`
	Findings        []ToolDraftFinding        `json:"findings"`
	Proposal        *ToolTestAnalysisProposal `json:"proposal,omitempty"`
	ProviderOutcome string                    `json:"provider_outcome"`
	Advisory        bool                      `json:"advisory"`
	GeneratedAt     time.Time                 `json:"generated_at"`
}

// toolTestAnalysisEvidence is the canonical browser/server hash projection of
// the short-lived value-free ToolTestRun. A separate narrower projection is
// used for the provider payload, which never includes run, tool, product,
// organisation, actor, request or nonce identifiers.
type toolTestAnalysisEvidence struct {
	Method               string                  `json:"method"`
	AuthenticationType   string                  `json:"authentication_type"`
	Outcome              string                  `json:"outcome"`
	Phase                string                  `json:"phase"`
	NetworkCallPerformed bool                    `json:"network_call_performed"`
	UpstreamStatusCode   int                     `json:"upstream_status_code,omitempty"`
	ResponseBytes        int64                   `json:"response_bytes,omitempty"`
	DurationMS           int64                   `json:"duration_ms"`
	RequestShape         model.JSONShape         `json:"request_shape"`
	ResponseShape        *model.JSONShape        `json:"response_shape,omitempty"`
	Findings             []model.ToolTestFinding `json:"findings"`
}

type toolTestAnalysisHashMaterial struct {
	SchemaVersion int                      `json:"schema_version"`
	ToolRevision  int64                    `json:"tool_revision"`
	CreatedAt     string                   `json:"created_at"`
	ExpiresAt     string                   `json:"expires_at"`
	Evidence      toolTestAnalysisEvidence `json:"evidence"`
}

// toolTestAIAnalysisEvidence is the still-narrower provider projection. The
// browser/server evidence hash binds the complete sanitized stored run, while
// the configured provider receives only schema-declared shape names and no
// diagnostic paths that could have originated in an upstream object key.
type toolTestAIAnalysisEvidence struct {
	Method               string              `json:"method"`
	AuthenticationType   string              `json:"authentication_type"`
	Outcome              string              `json:"outcome"`
	Phase                string              `json:"phase"`
	NetworkCallPerformed bool                `json:"network_call_performed"`
	UpstreamStatusCode   int                 `json:"upstream_status_code,omitempty"`
	ResponseBytes        int64               `json:"response_bytes,omitempty"`
	DurationMS           int64               `json:"duration_ms"`
	RequestShape         model.JSONShape     `json:"request_shape"`
	ResponseShape        *model.JSONShape    `json:"response_shape,omitempty"`
	Findings             []toolTestAIFinding `json:"findings"`
}

type toolTestAIFinding struct {
	Phase   string `json:"phase"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toolTestEvidence(run model.ToolTestRun) toolTestAnalysisEvidence {
	findings := append([]model.ToolTestFinding(nil), run.Findings...)
	if findings == nil {
		findings = []model.ToolTestFinding{}
	}
	return toolTestAnalysisEvidence{
		Method: strings.ToUpper(run.Method), AuthenticationType: run.AuthenticationType, Outcome: run.Outcome, Phase: run.Phase,
		NetworkCallPerformed: run.NetworkCallPerformed, UpstreamStatusCode: run.UpstreamStatusCode, ResponseBytes: run.ResponseBytes,
		DurationMS: run.DurationMS, RequestShape: run.RequestShape, ResponseShape: run.ResponseShape, Findings: findings,
	}
}

// ToolTestAnalysisEvidenceHash canonically binds an analysis request to one
// exact short-lived run without exposing an opaque internal identifier to the
// configured provider.
func ToolTestAnalysisEvidenceHash(run model.ToolTestRun) string {
	material := toolTestAnalysisHashMaterial{
		SchemaVersion: 1,
		ToolRevision:  run.ToolRevision,
		// PostgreSQL timestamps round-trip at microsecond precision. Normalize the
		// freshly executed in-memory run to that same durable representation so
		// the hash returned by POST /test-runs still matches the run loaded by the
		// later consented analysis request.
		CreatedAt: run.CreatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		ExpiresAt: run.ExpiresAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		Evidence:  toolTestEvidence(run),
	}
	encoded, _ := json.Marshal(material)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type toolTestAIContract struct {
	HTTPMethod          string              `json:"http_method"`
	TimeoutMS           int                 `json:"timeout_ms"`
	PathParameterNames  []string            `json:"path_parameter_names"`
	InputSchema         json.RawMessage     `json:"input_schema"`
	OutputSchema        json.RawMessage     `json:"output_schema"`
	AuthenticationType  string              `json:"authentication_type"`
	RequestMapping      ToolRequestMapping  `json:"request_mapping"`
	ResponseMapping     ToolResponseMapping `json:"response_mapping"`
	AuthorizationPolicy ToolPolicy          `json:"authorization_policy"`
}

type toolTestAIResponse struct {
	Reply        string             `json:"reply"`
	Findings     []ToolDraftFinding `json:"findings"`
	ProposalJSON string             `json:"proposal_json"`
}

// This is the complete subset the provider may edit. Endpoint, identity,
// upstream authentication configuration, credentials and examples never cross
// the provider boundary and are restored from the exact base revision.
type toolTestAIEditableDraft struct {
	Description         *string              `json:"description"`
	HTTPMethod          *string              `json:"http_method"`
	TimeoutMS           *int                 `json:"timeout_ms"`
	InputSchema         json.RawMessage      `json:"input_schema"`
	OutputSchema        json.RawMessage      `json:"output_schema"`
	RequestMapping      *ToolRequestMapping  `json:"request_mapping"`
	ResponseMapping     *ToolResponseMapping `json:"response_mapping"`
	AuthorizationPolicy *ToolPolicy          `json:"authorization_policy"`
}

var toolTestAnalysisOutputSchema = json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "reply":{"type":"string"},
    "findings":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "level":{"type":"string","enum":["warning","info"]},
          "code":{"type":"string"},
          "field":{"type":"string"},
          "message":{"type":"string"},
          "suggestion":{"type":"string"}
        },
        "required":["level","code","field","message","suggestion"]
      }
    },
    "proposal_json":{"type":"string"}
  },
  "required":["reply","findings","proposal_json"]
}`)
