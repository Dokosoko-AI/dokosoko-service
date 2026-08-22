package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestIntegrationEvidenceExcerptsStayWithinProviderBoundary(t *testing.T) {
	t.Parallel()
	records := make([]model.KnowledgeRecord, 0, 30)
	for source := 0; source < 5; source++ {
		for document := 0; document < 6; document++ {
			records = append(records, model.KnowledgeRecord{
				ID:        fmt.Sprintf("doc-%d-%d", source, document),
				SourceID:  fmt.Sprintf("source-%d", source),
				Title:     "Sample code",
				Text:      strings.Repeat("é", 5_000),
				URL:       fmt.Sprintf("https://docs.example.com/%d/%d", source, document),
				Published: true,
			})
		}
	}
	excerpts := integrationSourceExcerpts(records)
	total := 0
	for sourceID, excerpt := range excerpts {
		length := len([]rune(excerpt.Text))
		if length > maxAnalysisSourceExcerptRunes {
			t.Fatalf("%s excerpt has %d runes, max %d", sourceID, length, maxAnalysisSourceExcerptRunes)
		}
		if len(excerpt.References) > maxAnalysisDocumentsPerSource {
			t.Fatalf("%s has %d references, max %d", sourceID, len(excerpt.References), maxAnalysisDocumentsPerSource)
		}
		total += length
	}
	if total > maxAnalysisKnowledgeRunes {
		t.Fatalf("knowledge evidence has %d runes, max %d", total, maxAnalysisKnowledgeRunes)
	}

	manifest := json.RawMessage(`{"paths":{"/calls":` + `"` + strings.Repeat("x", 10_000) + `"}}`)
	integration := model.Integration{FamilyKey: "voice", VersionKey: "v1", Lifecycle: "active", Description: strings.Repeat("d", 10_000), Resources: []model.IntegrationResourceLink{{Name: "Voice API", Kind: "openapi", ResolvedRevision: &model.ResourceSetRevision{Manifest: manifest}}}}
	if length := len([]rune(integrationCatalogExcerpt(integration, maxAnalysisIntegrationItem))); length > maxAnalysisIntegrationItem {
		t.Fatalf("integration evidence has %d runes, max %d", length, maxAnalysisIntegrationItem)
	}
	tool := model.Tool{Description: strings.Repeat("d", 5_000), InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)}
	if length := len([]rune(toolCatalogExcerpt(tool, maxAnalysisToolItem))); length > maxAnalysisToolItem {
		t.Fatalf("tool evidence has %d runes, max %d", length, maxAnalysisToolItem)
	}
}
