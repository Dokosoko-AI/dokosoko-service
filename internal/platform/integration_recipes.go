package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const integrationAnalysisSchemaVersion = 2
const integrationScopeEvidenceKind = "integration_scope"
const recipeAuthoringInputDependencyKind = "recipe_authoring_input"
const recipeAuthoringContractVersion = model.RecipeContractProductIntegrationV2

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

var integrationAnalysisSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"recipes":{"type":"array","maxItems":12,"items":{"type":"object","additionalProperties":false,"properties":{"capability_ids":{"type":"array","minItems":1,"maxItems":1,"uniqueItems":true,"items":{"type":"string"}},"evidence_ids":{"type":"array","minItems":1,"maxItems":24,"uniqueItems":true,"items":{"type":"string"}}},"required":["capability_ids","evidence_ids"]}}},"required":["recipes"]}`)

var recipeBriefSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string","enum":["ready","needs_input"]},"capability_ids":{"type":"array","maxItems":1,"uniqueItems":true,"items":{"type":"string"}},"evidence_ids":{"type":"array","maxItems":24,"uniqueItems":true,"items":{"type":"string"}},"gaps":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":500}}},"required":["status","capability_ids","evidence_ids","gaps"]}`)
var recipeAuthoringSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string","enum":["ready","needs_input"]},"reference_ids":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string"}},"gaps":{"type":"array","maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":500}}},"required":["status","reference_ids","gaps"]}`)
var recipeReviewSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"recommendation":{"type":"string","enum":["pass","revise"]},"findings":{"type":"array","maxItems":9,"items":{"type":"object","additionalProperties":false,"properties":{"code":{"type":"string","enum":["delivery_scope","multiple_capabilities","sdk_scope","non_actionable_step","unobservable_check","unsupported_claim","unsafe_content","not_minimal","evidence_gap"]},"evidence_ids":{"type":"array","minItems":1,"maxItems":8,"uniqueItems":true,"items":{"type":"string","minLength":1,"maxLength":512}}},"required":["code","evidence_ids"]}}},"required":["recommendation","findings"]}`)
var recipeURLPattern = regexp.MustCompile(`(?i)https://[^\s)<>{}"']+`)
var recipeMarkdownLinkPattern = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\s]+)\)`)
var recipeURISchemePattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]{0,31}):(?://|[^\s])`)

func recipeContainsURI(value string) bool {
	return recipeURISchemePattern.MatchString(value)
}

func recipeContainsUnsupportedURI(value string) bool {
	for _, match := range recipeURISchemePattern.FindAllStringSubmatch(value, -1) {
		if len(match) > 1 && !strings.EqualFold(match[1], "https") {
			return true
		}
	}
	return false
}

type recipeAuthoringResponse struct {
	Status       string   `json:"status"`
	ReferenceIDs []string `json:"reference_ids"`
	Gaps         []string `json:"gaps"`
}

type recipeBriefAIResponse struct {
	Status        string   `json:"status"`
	CapabilityIDs []string `json:"capability_ids"`
	EvidenceIDs   []string `json:"evidence_ids"`
	Gaps          []string `json:"gaps"`
}

type recipeReviewResponse struct {
	Recommendation string                         `json:"recommendation"`
	Findings       []recipeReviewFindingSelection `json:"findings"`
}

type recipeReviewFindingSelection struct {
	Code        string   `json:"code"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type integrationAnalysisAIRecipe struct {
	CapabilityIDs []string `json:"capability_ids"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

type integrationAnalysisAIResponse struct {
	Recipes []integrationAnalysisAIRecipe `json:"recipes"`
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
