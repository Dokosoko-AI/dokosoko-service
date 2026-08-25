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
const recipeAuthoringContractVersion = "recipe-authoring-v7"
const recipeMissingEndpointMarker = "<!-- recipe-missing-endpoint-selection -->"

const (
	maxAnalysisKnowledgeRunes     = 16_000
	maxAnalysisIntegrationRunes   = 8_000
	maxAnalysisToolRunes          = 8_000
	maxAnalysisSourceExcerptRunes = 6_000
	maxAnalysisDocumentRunes      = 2_000
	maxAnalysisDocumentsPerSource = 3
	maxAnalysisIntegrationItem    = 4_000
	maxAnalysisToolItem           = 2_000
)

var integrationAnalysisSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string"},"identity":{"type":"object","additionalProperties":false,"properties":{"mode":{"type":"string","enum":["none","oauth2","api_key","service_account"]},"issuer":{"type":"string"},"audience":{"type":"string"},"grants":{"type":"array","items":{"type":"string"}},"explanation":{"type":"string"}},"required":["mode","explanation"]},"endpoints":{"type":"array","maxItems":24,"items":{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"},"method":{"type":"string"},"path":{"type":"string"},"purpose":{"type":"string"},"identity":{"type":"string"},"evidence":{"type":"array","items":{"type":"string"}}},"required":["name","method","path","purpose","identity","evidence"]}},"recipes":{"type":"array","maxItems":12,"items":{"type":"object","additionalProperties":false,"properties":{"slug":{"type":"string"},"title":{"type":"string"},"outcome":{"type":"string"},"audience":{"type":"string"},"endpoint_ids":{"type":"array","items":{"type":"string"}}},"required":["slug","title","outcome","audience"]}}},"required":["summary","identity","endpoints","recipes"]}`)

var recipeBriefSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"slug":{"type":"string"},"title":{"type":"string"},"outcome":{"type":"string"},"audience":{"type":"string"},"endpoint_ids":{"type":"array","items":{"type":"string"}}},"required":["slug","title","outcome","audience","endpoint_ids"]}`)
var recipeAuthoringSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"markdown":{"type":"string"},"reference_ids":{"type":"array","uniqueItems":true,"items":{"type":"string"}}},"required":["markdown","reference_ids"]}`)
var recipeReviewSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string"},"approved":{"type":"boolean"},"findings":{"type":"array","maxItems":12,"items":{"type":"object","additionalProperties":false,"properties":{"level":{"type":"string","enum":["info","warning","error"]},"code":{"type":"string"},"message":{"type":"string"}},"required":["level","code","message"]}}},"required":["summary","approved","findings"]}`)
var recipeURLPattern = regexp.MustCompile(`https://[^\s)<>{}"']+`)

type recipeAuthoringResponse struct {
	Markdown     string   `json:"markdown"`
	ReferenceIDs []string `json:"reference_ids"`
}

type recipeReviewResponse struct {
	Summary  string                          `json:"summary"`
	Approved bool                            `json:"approved"`
	Findings []model.RecipeValidationFinding `json:"findings"`
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
