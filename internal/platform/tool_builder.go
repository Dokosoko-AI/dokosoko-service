package platform

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const (
	maxToolBuilderInstructionBytes = 8 << 10
	maxToolBuilderImportBytes      = 512 << 10
	maxToolBuilderCandidates       = 50
	maxToolBuilderChatMessages     = 12
	maxToolBuilderChatMessageBytes = 2 << 10
	maxToolBuilderChatHistoryBytes = 12 << 10
)

var (
	ErrToolBuilderInvalidInput = errors.New("tool builder input is invalid")
	ErrToolBuilderUnsafeInput  = errors.New("tool builder input contains credential material")

	toolBuilderPlaceholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_.-]{0,63})\}`)
	toolBuilderSecretAssignment   = regexp.MustCompile(`(?i)(authorization|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|client[-_ ]?secret|password|secret)\s*[:=]\s*["']?[^\s,"'}]{8,}`)
	toolBuilderBearerValue        = regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._~+/=-]{8,})`)
	toolBuilderBasicValue         = regexp.MustCompile(`(?i)\bbasic\s+([A-Za-z0-9+/=]{8,})`)
	toolBuilderJWTValue           = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\b`)
	toolBuilderKnownSecretValue   = regexp.MustCompile(`\b(?:(?:sk|pk|rk|xox[baprs])[-_][A-Za-z0-9_-]{8,}|gh[opusr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AIza[A-Za-z0-9_-]{20,}|npm_[A-Za-z0-9]{20,}|glpat-[A-Za-z0-9_-]{20,})\b`)
	toolBuilderURLUserInfo        = regexp.MustCompile(`(?i)\bhttps?://[^\s/?#@]+@[^\s/?#]+`)
)

var emptyToolBuilderSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)

// ToolDraft is the complete, non-secret candidate contract shared by manual,
// imported and AI-assisted builder modes. Credential material deliberately has
// no field in this type; only the presence bit crosses the public boundary.
type ToolDraft struct {
	Namespace           string              `json:"namespace"`
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	HTTPMethod          string              `json:"http_method"`
	Endpoint            string              `json:"endpoint"`
	TimeoutMS           int                 `json:"timeout_ms"`
	InputSchema         json.RawMessage     `json:"input_schema"`
	OutputSchema        json.RawMessage     `json:"output_schema"`
	UpstreamAuth        ToolUpstreamAuth    `json:"upstream_auth"`
	RequestMapping      ToolRequestMapping  `json:"request_mapping"`
	ResponseMapping     ToolResponseMapping `json:"response_mapping"`
	AuthorizationPolicy ToolPolicy          `json:"authorization_policy"`
	RequestExample      map[string]any      `json:"request_example,omitempty"`
	ResponseExample     any                 `json:"response_example,omitempty"`
	CredentialPresent   bool                `json:"credential_present"`
}

type ToolDraftFinding struct {
	Level      string `json:"level"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Field      string `json:"field,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type ToolDraftChange struct {
	Field             string `json:"field"`
	Rationale         string `json:"rationale,omitempty"`
	SecuritySensitive bool   `json:"security_sensitive,omitempty"`
}

type ToolDraftContext struct {
	Draft                    ToolDraft `json:"draft"`
	BaseToolID               string    `json:"base_tool_id,omitempty"`
	BaseRevision             int64     `json:"base_revision,omitempty"`
	CredentialWillBeSupplied bool      `json:"credential_will_be_supplied,omitempty"`
}

type ToolDraftProposalInput struct {
	ToolDraftContext
	Instruction string                   `json:"instruction"`
	History     []ToolBuilderChatMessage `json:"history,omitempty"`
}

// ToolBuilderChatMessage is a bounded, non-secret conversational hint. Chat
// history is supplied by the administrator on each request and is never a
// trusted instruction or a persistence mechanism.
type ToolBuilderChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolDraftImportSource struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type ToolDraftImportInput struct {
	ToolDraftContext
	Source ToolDraftImportSource `json:"source"`
}

type ToolDraftAnalysisInput struct {
	ToolDraftContext
}

type ToolDraftValidation struct {
	Valid                bool               `json:"valid"`
	NetworkCallPerformed bool               `json:"network_call_performed"`
	Findings             []ToolDraftFinding `json:"findings"`
	NormalizedDraft      ToolDraft          `json:"normalized_draft"`
	CheckedAt            time.Time          `json:"checked_at,omitempty"`
}

type ToolDraftProposal struct {
	ProposalID      string             `json:"proposal_id,omitempty"`
	BaseFingerprint string             `json:"base_fingerprint,omitempty"`
	Summary         string             `json:"summary,omitempty"`
	Reply           string             `json:"reply,omitempty"`
	Draft           ToolDraft          `json:"draft"`
	Changes         []ToolDraftChange  `json:"changes"`
	Findings        []ToolDraftFinding `json:"findings"`
	Valid           bool               `json:"valid"`
	GeneratedAt     time.Time          `json:"generated_at,omitempty"`
}

type ToolDraftImportCandidate struct {
	Summary  string             `json:"summary,omitempty"`
	Draft    ToolDraft          `json:"draft"`
	Changes  []ToolDraftChange  `json:"changes"`
	Findings []ToolDraftFinding `json:"findings"`
	Valid    bool               `json:"valid"`
}

type ToolDraftImportResult struct {
	Candidates  []ToolDraftImportCandidate `json:"candidates"`
	Findings    []ToolDraftFinding         `json:"findings"`
	GeneratedAt time.Time                  `json:"generated_at,omitempty"`
}

type ToolDraftAnalysis struct {
	Summary              string             `json:"summary"`
	Reply                string             `json:"reply,omitempty"`
	Draft                ToolDraft          `json:"draft"`
	Valid                bool               `json:"valid"`
	NetworkCallPerformed bool               `json:"network_call_performed"`
	Findings             []ToolDraftFinding `json:"findings"`
	GeneratedAt          time.Time          `json:"generated_at,omitempty"`
}
