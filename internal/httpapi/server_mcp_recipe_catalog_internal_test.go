package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type recipeVersionStore struct {
	*store.Memory
	revisions []model.IntegrationRevision
	reads     int
}

func (s *recipeVersionStore) IntegrationRevisions(context.Context, string) ([]model.IntegrationRevision, error) {
	s.reads++
	return s.revisions, nil
}

func TestRecipeCatalogVersionsUseExactHistoricalSnapshots(t *testing.T) {
	now := time.Now().UTC()
	old := model.IntegrationRevision{ID: "old-revision", IntegrationID: "api", Revision: 1, State: "published", ManifestHash: "old-hash", PublishedAt: &now, Snapshot: json.RawMessage(`{"family_key":"orders","version_key":"v1","display_name":"Original Orders","visibility":"public"}`)}
	backend := &recipeVersionStore{Memory: store.NewMemory(), revisions: []model.IntegrationRevision{{ID: "new-revision", IntegrationID: "api", Revision: 2, State: "published", ManifestHash: "new-hash", PublishedAt: &now, Snapshot: json.RawMessage(`{"family_key":"orders","version_key":"v2","display_name":"New Orders","visibility":"public"}`)}, old}}
	s := Server{service: platform.New(backend)}
	manifest := model.ProductManifest{Integrations: []model.IntegrationManifest{{ID: "api", FamilyKey: "orders", VersionKey: "v2", DisplayName: "New Orders", ManifestHash: "new-hash", Visibility: model.VisibilityPublic}}}
	recipe := model.Recipe{StableURI: "dokosoko://products/acme/recipes/order", Slug: "order", Title: "Read order", Outcome: "Order is read.", ContractVersion: model.RecipeContractDeploymentV3, APIAttachments: []model.RecipeAPIAttachment{{IntegrationID: "api"}}, CurrentRevisionID: "recipe-revision", PublishedAt: &now, CurrentRevision: &model.RecipeRevision{APIBindings: []model.RecipeAPIBinding{{IntegrationID: "api", IntegrationRevisionID: "old-revision", IntegrationManifestHash: "old-hash"}}}}
	resolve := s.recipeCatalogVersionResolver(context.Background(), manifest, true)
	for i := 0; i < 35; i++ {
		summary, err := compactRecipeSummary(recipe, resolve)
		if err != nil {
			t.Fatal(err)
		}
		version := summary["api_versions"].([]map[string]any)[0]
		if version["version_key"] != "v1" || version["display_name"] != "Original Orders" || version["integration_revision_id"] != "old-revision" || version["manifest_hash"] != "old-hash" {
			t.Fatal("current API metadata replaced the historical selection")
		}
		// The custom-tool validator intentionally omits JSON Schema format.
		// Check the advertised date-time constraint separately, then the shape.
		outputSchema := compactRecipeSummarySchema()
		publishedSchema := outputSchema["properties"].(map[string]any)["published_at"].(map[string]any)
		if publishedSchema["format"] != "date-time" {
			t.Fatal("publication timestamp lost its contract")
		}
		if _, err := time.Parse(time.RFC3339Nano, summary["published_at"].(string)); err != nil {
			t.Fatal(err)
		}
		delete(publishedSchema, "format")
		schema, _ := json.Marshal(outputSchema)
		wire, _ := json.Marshal(summary)
		_ = json.Unmarshal(wire, &summary)
		if err := toolruntime.ValidateArguments(schema, summary); err != nil {
			t.Fatalf("summary violates advertised schema: %v", err)
		}
	}
	legacy := recipe
	legacy.ContractVersion, legacy.IntegrationID, legacy.APIAttachments = model.RecipeContractProductIntegrationV2, "api", nil
	legacy.CurrentRevision = &model.RecipeRevision{IntegrationRevisionID: "old-revision", IntegrationManifestHash: "old-hash"}
	legacySummary, err := compactRecipeSummary(legacy, resolve)
	if err != nil || legacySummary["api_versions"].([]map[string]any)[0]["version_key"] != "v1" {
		t.Fatal("legacy recipe revision lost its exact API version")
	}
	if backend.reads != 1 {
		t.Fatalf("history was reloaded for each task: %d", backend.reads)
	}
	for _, failure := range []string{"revision", "hash", "state", "timestamp", "scope", "snapshot", "audience", "duplicate"} {
		t.Run(failure, func(t *testing.T) {
			raw, _ := json.Marshal(recipe)
			var changed model.Recipe
			_ = json.Unmarshal(raw, &changed)
			backend.revisions = []model.IntegrationRevision{old}
			switch failure {
			case "revision":
				changed.CurrentRevision.APIBindings[0].IntegrationRevisionID = "missing"
			case "hash":
				changed.CurrentRevision.APIBindings[0].IntegrationManifestHash = "wrong"
			case "state":
				backend.revisions[0].State = "draft"
			case "timestamp":
				backend.revisions[0].PublishedAt = nil
			case "scope":
				backend.revisions[0].IntegrationID = "another-api"
			case "snapshot":
				backend.revisions[0].Snapshot = json.RawMessage(`{"family_key":"orders"}`)
			case "audience":
				backend.revisions[0].Snapshot = json.RawMessage(`{"family_key":"orders","version_key":"v1","display_name":"PRIVATE-TITLE","visibility":"private"}`)
			case "duplicate":
				changed.CurrentRevision.APIBindings = append(changed.CurrentRevision.APIBindings, changed.CurrentRevision.APIBindings[0])
			}
			if value, err := compactRecipeSummary(changed, s.recipeCatalogVersionResolver(context.Background(), manifest, true)); err == nil || value != nil {
				t.Fatal("invalid historical selection produced metadata")
			}
		})
	}
}

func TestRecipeCatalogPagesBoundEncodedBytesAndSortTitles(t *testing.T) {
	entries := []map[string]any{{"title": "Zulu", "uri": "z", "outcome": strings.Repeat("<&>", 6000)}, {"title": "alpha", "uri": "a", "outcome": "small"}, {"title": "Beta", "uri": "b", "outcome": strings.Repeat("x", 40000)}}
	page, next, err := recipeCatalogPage(json.RawMessage(`{}`), entries, "scope", 32)
	if err == nil || page != nil || next != "" {
		t.Fatal("oversized escaped entry was omitted instead of failing")
	}
	for _, entry := range entries {
		if entry["title"] == "Zulu" {
			entry["outcome"] = strings.Repeat("x", 40000)
		}
	}
	page, next, err = recipeCatalogPage(json.RawMessage(`{}`), entries, "scope", 32)
	if err != nil || len(page) != 2 || page[0]["title"] != "alpha" || page[1]["title"] != "Beta" || next == "" {
		t.Fatal("recipe page did not enforce its byte budget and title order")
	}
	raw, _ := json.Marshal(map[string]any{"cursor": next})
	second, last, err := recipeCatalogPage(raw, entries, "scope", 32)
	if err != nil || len(second) != 1 || second[0]["title"] != "Zulu" || last != "" {
		t.Fatal("byte-limited recipe continuation was incomplete")
	}
}
