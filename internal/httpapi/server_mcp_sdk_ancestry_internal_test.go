package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestMCPRejectsFailedCrossFileSDKAncestryGeneration(t *testing.T) {
	now := time.Now().UTC()
	record := store.SearchIndexGenerationRecord{
		Generation: model.SearchIndexGeneration{
			ID: "mcp-sdk-ancestry-generation", DeploymentID: "prod_acme", PublicationKind: "api",
			PublicationID: "mcp-sdk-ancestry-publication", AssetKind: "mixed", BuilderVersion: "builder-test",
			RetrievalProfileVersion: "retrieval-test", State: "failed", UnitCount: 1,
			Diagnostics: json.RawMessage(`{"error":"SDK symbol names a file outside its section ancestry"}`), CreatedAt: now,
		},
		Units: []model.KnowledgeUnit{{
			ID: "FORBIDDEN_ANCESTRY_UNIT", SearchIndexGenerationID: "mcp-sdk-ancestry-generation",
			DeploymentID: "prod_acme", Kind: "sdk_symbol", SourcePublicationKind: "sdk",
			SourcePublicationID: "sdk-publication", SourceEntityID: "forbidden-symbol",
			Content: "FORBIDDEN_ANCESTRY_CONTENT", Visibility: model.VisibilityPublic,
		}},
	}
	resources, toc, err := publicationScopedDeveloperAssetEvidence(record, []string{"developer-assets", "apis", "api"}, "api", true)
	if err == nil || len(resources) != 0 || toc != "" || !strings.Contains(err.Error(), "generation is inconsistent") {
		t.Fatalf("failed ancestry generation reached MCP: resources=%#v toc=%q err=%v", resources, toc, err)
	}
}
