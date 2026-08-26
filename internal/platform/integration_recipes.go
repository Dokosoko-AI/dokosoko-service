package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const integrationAnalysisSchemaVersion = 1
const integrationScopeEvidenceKind = "integration_scope"
const recipeAuthoringInputDependencyKind = "recipe_authoring_input"
const recipeAuthoringContractVersion = recipeAuthoringPromptVersionV8
const recipeMissingEndpointMarker = "<!-- recipe-missing-endpoint-selection -->"

const (
	maxAnalysisKnowledgeRunes      = 16_000
	maxAnalysisIntegrationRunes    = 8_000
	maxAnalysisToolRunes           = 8_000
	maxAnalysisScopedDocumentRunes = 16_000
	maxAnalysisSourceExcerptRunes  = 6_000
	maxAnalysisDocumentRunes       = 2_000
	maxAnalysisDocumentsPerSource  = 3
	maxAnalysisIntegrationItem     = 4_000
	maxAnalysisToolItem            = 2_000
)

var integrationAnalysisSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string","minLength":1,"maxLength":1000},"summary_evidence_ids":{"type":"array","maxItems":24,"uniqueItems":true,"items":{"type":"string"}},"recipes":{"type":"array","maxItems":12,"items":{"type":"object","additionalProperties":false,"properties":{"slug":{"type":"string","minLength":1,"maxLength":160},"title":{"type":"string","minLength":1,"maxLength":160},"outcome":{"type":"string","minLength":1,"maxLength":1000},"audience":{"type":"string","minLength":1,"maxLength":80},"endpoint_ids":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string"}},"evidence_ids":{"type":"array","minItems":1,"maxItems":24,"uniqueItems":true,"items":{"type":"string"}},"rationale":{"type":"string","minLength":1,"maxLength":1000}},"required":["slug","title","outcome","audience","endpoint_ids","evidence_ids","rationale"]}}},"required":["summary","summary_evidence_ids","recipes"]}`)

var recipeBriefSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string","enum":["ready","needs_input"]},"slug":{"type":"string","maxLength":160},"title":{"type":"string","maxLength":160},"outcome":{"type":"string","maxLength":1000},"audience":{"type":"string","maxLength":80},"endpoint_ids":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string"}},"evidence_ids":{"type":"array","maxItems":24,"uniqueItems":true,"items":{"type":"string"}},"gaps":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":500}}},"required":["status","slug","title","outcome","audience","endpoint_ids","evidence_ids","gaps"]}`)
var recipeAuthoringSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"markdown":{"type":"string","minLength":1,"maxLength":100000},"reference_ids":{"type":"array","maxItems":32,"uniqueItems":true,"items":{"type":"string"}},"evidence_ids":{"type":"array","minItems":1,"maxItems":64,"uniqueItems":true,"items":{"type":"string"}}},"required":["markdown","reference_ids","evidence_ids"]}`)
var recipeReviewSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string","minLength":1,"maxLength":2000},"recommendation":{"type":"string","enum":["pass","revise"]},"findings":{"type":"array","maxItems":12,"items":{"type":"object","additionalProperties":false,"properties":{"level":{"type":"string","enum":["info","warning","error"]},"code":{"type":"string","minLength":1,"maxLength":80},"message":{"type":"string","minLength":1,"maxLength":500}},"required":["level","code","message"]}}},"required":["summary","recommendation","findings"]}`)
var recipeURLPattern = regexp.MustCompile(`https://[^\s)<>{}"']+`)
var recipeMarkdownLinkPattern = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\s]+)\)`)

type recipeAuthoringResponse struct {
	Markdown     string   `json:"markdown"`
	ReferenceIDs []string `json:"reference_ids"`
	EvidenceIDs  []string `json:"evidence_ids"`
}

type recipeBriefAIResponse struct {
	Status      string   `json:"status"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Outcome     string   `json:"outcome"`
	Audience    string   `json:"audience"`
	EndpointIDs []string `json:"endpoint_ids"`
	EvidenceIDs []string `json:"evidence_ids"`
	Gaps        []string `json:"gaps"`
}

type recipeReviewResponse struct {
	Summary        string                          `json:"summary"`
	Recommendation string                          `json:"recommendation"`
	Findings       []model.RecipeValidationFinding `json:"findings"`
}

type integrationAnalysisAIRecipe struct {
	model.RecipeSeed
	EvidenceIDs []string `json:"evidence_ids"`
	Rationale   string   `json:"rationale"`
}

type integrationAnalysisAIResponse struct {
	Summary            string                        `json:"summary"`
	SummaryEvidenceIDs []string                      `json:"summary_evidence_ids"`
	Recipes            []integrationAnalysisAIRecipe `json:"recipes"`
}

func evidenceFingerprint(values ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(hash[:])
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
