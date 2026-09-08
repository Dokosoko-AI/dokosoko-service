package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/testutil"
)

type recipeCatalogStore struct {
	*store.Memory
	values       []model.Recipe
	historyReads int
}

func (s *recipeCatalogStore) Recipes(ctx context.Context, productID string) ([]model.Recipe, error) {
	if s.values == nil || productID != "prod_acme" {
		return s.Memory.Recipes(ctx, productID)
	}
	raw, _ := json.Marshal(s.values)
	var values []model.Recipe
	_ = json.Unmarshal(raw, &values)
	return values, nil
}
func (s *recipeCatalogStore) IntegrationRevisions(ctx context.Context, id string) ([]model.IntegrationRevision, error) {
	s.historyReads++
	return s.Memory.IntegrationRevisions(ctx, id)
}

type recipeCatalogReply struct {
	Result struct {
		Structured struct {
			Version int    `json:"catalog_version"`
			Count   int    `json:"recipe_count"`
			Next    string `json:"next_cursor"`
			Recipes []struct {
				URI        string `json:"uri"`
				Title      string `json:"title"`
				RevisionID string `json:"revision_id"`
				APIs       []struct {
					ID         string `json:"api_id"`
					Family     string `json:"family_key"`
					Version    string `json:"version_key"`
					Name       string `json:"display_name"`
					RevisionID string `json:"integration_revision_id"`
					Hash       string `json:"manifest_hash"`
				} `json:"api_versions"`
			} `json:"recipes"`
		} `json:"structuredContent"`
	} `json:"result"`
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func TestMCPRecipeCatalogPagesExactVersionsAndAudience(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	overlay := &recipeCatalogStore{Memory: memory}
	service := testutil.NewRecipeService(t, overlay, testutil.RecipeAI{})
	actor := platform.Actor{ID: "recipe-catalog-reviewer"}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	prepare := func(namespace string, visibility model.Visibility) (model.Integration, model.Recipe) {
		t.Helper()
		api, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: namespace, VersionKey: "v1.2", DisplayName: "Catalog " + namespace, Description: "Integration task catalog.", Visibility: visibility, AcknowledgePublic: visibility == model.VisibilityPublic, Lifecycle: "active"}, actor)
		if err != nil {
			t.Fatal(err)
		}
		if visibility == model.VisibilityPublic {
			attachMCPTestContract(t, service, api.ID)
		} else {
			prepareHTTPPrivateIntegrationFoundations(t, handler, api.ID)
		}
		preparePublishedRecipeHTTPIntegration(t, ctx, memory, service, api, namespace, actor)
		analysis, err := service.AnalyseIntegrationFor(ctx, "prod_acme", api.ID, actor)
		if err != nil {
			t.Fatal(err)
		}
		recipes, err := service.GenerateRecipesForIntegration(ctx, "prod_acme", analysis.ID, api.ID, actor)
		if err != nil || len(recipes) != 1 {
			t.Fatalf("required AI recipe generation: %v", err)
		}
		recipe := recipes[0]
		if visibility == model.VisibilityPublic {
			recipe, err = service.UpdateRecipeReferences(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, nil, visibility, actor)
			if err != nil {
				t.Fatal(err)
			}
		}
		recipe, err = service.ApproveRecipe(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, actor)
		if err != nil {
			t.Fatal(err)
		}
		recipe, err = service.PublishRecipe(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, actor)
		if err != nil {
			t.Fatal(err)
		}
		return api, recipe
	}
	api, base := prepare("catalogpublic", model.VisibilityPublic)
	privateAPI, private := prepare("catalogprivate", model.VisibilityPrivate)
	enablePublicMCPForDeveloperAssetTest(t, ctx, memory, "prod_acme")
	// Duplicate the reviewed task only in the read overlay to isolate list growth.
	// Its content/spec/dependencies stay intact; this is not 35 independently authored tasks.
	overlay.values = []model.Recipe{}
	for i := 0; i < 35; i++ {
		raw, _ := json.Marshal(base)
		var value model.Recipe
		_ = json.Unmarshal(raw, &value)
		value.ID = fmt.Sprintf("recipe-catalog-%02d", i)
		value.StableURI = fmt.Sprintf("dokosoko://products/acme/recipes/catalog-task-%02d", i)
		value.CurrentRevisionID = "revision-" + value.ID
		value.CurrentRevision.ID = value.CurrentRevisionID
		value.CurrentRevision.RecipeID = value.ID
		overlay.values = append(overlay.values, value)
	}
	overlay.values = append(overlay.values, private)
	call := func(public bool, version int, arguments map[string]any) (recipeCatalogReply, string) {
		t.Helper()
		path, token := "/mcp/public", ""
		if !public {
			path, token = "/mcp", "doko_private_demo"
		}
		params := map[string]any{"name": "integration.recipes.list", "arguments": arguments}
		if version != 0 {
			params["_meta"] = map[string]any{"com.dokosoko/catalogVersion": version}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params})
		response := request(t, handler, http.MethodPost, path, token, string(raw))
		var value recipeCatalogReply
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &value) != nil {
			t.Fatalf("recipe list failed: %.200s", response.Body.String())
		}
		return value, response.Body.String()
	}
	legacy, legacyBody := call(true, 1, map[string]any{})
	first, firstBody := call(true, 0, map[string]any{})
	if first.Error != nil || first.Result.Structured.Count != 35 || len(first.Result.Structured.Recipes) != 32 || first.Result.Structured.Next == "" {
		t.Fatalf("first recipe page: %.1000s", firstBody)
	}
	if legacy.Error != nil || len(legacy.Result.Structured.Recipes) != 35 || legacy.Result.Structured.Version != 0 || legacy.Result.Structured.Next != "" || strings.Contains(legacyBody, "api_versions") {
		t.Fatal("legacy full recipe list changed")
	}
	t.Logf("recipes=35 legacy_response_bytes=%d first_page_bytes=%d", len(legacyBody), len(firstBody))
	second, _ := call(true, 2, map[string]any{"cursor": first.Result.Structured.Next})
	if second.Error != nil || len(second.Result.Structured.Recipes) != 3 || second.Result.Structured.Next != "" {
		t.Fatal("later recipes unreachable")
	}
	seen := map[string]bool{}
	binding := base.CurrentRevision.APIBindings[0]
	for _, value := range append(first.Result.Structured.Recipes, second.Result.Structured.Recipes...) {
		if seen[value.URI] || len(value.APIs) != 1 || value.RevisionID == "" {
			t.Fatal("duplicate or incomplete recipe summary")
		}
		seen[value.URI] = true
		version := value.APIs[0]
		if version.ID != api.ID || version.Version != api.VersionKey || version.Name != api.DisplayName || version.RevisionID != binding.IntegrationRevisionID || version.Hash != binding.IntegrationManifestHash {
			t.Fatal("recipe did not retain its exact API version")
		}
	}
	for _, field := range []string{private.StableURI, privateAPI.DisplayName, `"dependencies"`, `"markdown"`, `"spec"`, `"organisation_id"`} {
		if strings.Contains(firstBody, field) {
			t.Fatalf("private/internal metadata exposed: %s", field)
		}
	}
	filtered, _ := call(true, 2, map[string]any{"api_id": api.ID, "query": "  CATALOGPUBLIC   V1.2 "})
	if filtered.Error != nil || filtered.Result.Structured.Count != 35 {
		t.Fatal("API version query failed")
	}
	narrowed, narrowBody := call(true, 2, map[string]any{"api_id": privateAPI.ID})
	if narrowed.Error != nil || narrowed.Result.Structured.Count != 0 || len(narrowed.Result.Structured.Recipes) != 0 || strings.Contains(narrowBody, privateAPI.DisplayName) {
		t.Fatal("private API filter leaked a task")
	}
	continued, _ := call(true, 2, map[string]any{"api_id": api.ID, "query": "catalogpublic v1_2", "cursor": filtered.Result.Structured.Next})
	if continued.Error != nil || len(continued.Result.Structured.Recipes) != 3 {
		t.Fatal("equivalent normalized query could not continue")
	}
	changedFilter, _ := call(true, 2, map[string]any{"api_id": api.ID, "cursor": first.Result.Structured.Next})
	if changedFilter.Error == nil || changedFilter.Error.Code != -32602 {
		t.Fatal("changed API filter retained an old continuation")
	}
	privatePage, _ := call(false, 2, map[string]any{"api_id": privateAPI.ID})
	if privatePage.Error != nil || privatePage.Result.Structured.Count != 1 || privatePage.Result.Structured.Recipes[0].URI != private.StableURI {
		t.Fatal("private task filtering failed")
	}
	for _, arguments := range []map[string]any{{"query": nil}, {"query": 1}, {"query": strings.Repeat("a", 501)}, {"api_id": ""}, {"api_id": nil}, {"cursor": ""}, {"cursor": nil}, {"extra": true}} {
		value, _ := call(true, 2, arguments)
		if value.Error == nil || value.Error.Code != -32602 {
			t.Fatalf("invalid recipe arguments accepted: %#v", arguments)
		}
	}
	for _, arguments := range []map[string]any{{"query": "x"}, {"cursor": first.Result.Structured.Next}} {
		value, _ := call(true, 1, arguments)
		if value.Error == nil || value.Error.Code != -32602 {
			t.Fatal("legacy list accepted new arguments")
		}
	}
	changed, _ := call(true, 2, map[string]any{"cursor": first.Result.Structured.Next, "query": "catalogpublic"})
	if changed.Error == nil || changed.Error.Code != -32602 {
		t.Fatal("changed query retained continuation")
	}
	foreign, _ := call(false, 2, map[string]any{"cursor": first.Result.Structured.Next})
	if foreign.Error == nil || foreign.Error.Code != -32602 {
		t.Fatal("changed audience retained continuation")
	}
	// Growth is measured separately from AI quality using copies of that same
	// canonical reviewed task. Page size must stop growing with total count.
	originals := overlay.values
	for _, count := range []int{100, 1000} {
		values := make([]model.Recipe, 0, count+1)
		for i := 0; i < count; i++ {
			raw, _ := json.Marshal(originals[i%35])
			var value model.Recipe
			_ = json.Unmarshal(raw, &value)
			value.StableURI = fmt.Sprintf("dokosoko://products/acme/recipes/growth-task-%04d", i)
			values = append(values, value)
		}
		overlay.values = append(values, private)
		old, oldBody := call(true, 1, map[string]any{})
		page, pageBody := call(true, 2, map[string]any{})
		if old.Error != nil || len(old.Result.Structured.Recipes) != count || page.Error != nil || page.Result.Structured.Count != count || len(page.Result.Structured.Recipes) != 32 || len(pageBody) > 60000 {
			t.Fatal("recipe response growth is not bounded")
		}
		t.Logf("recipes=%d legacy_response_bytes=%d first_page_bytes=%d", count, len(oldBody), len(pageBody))
	}
	overlay.values = originals
	first, _ = call(true, 2, map[string]any{})
	// Remove one currently available task; a cursor may not traverse that changed list.
	overlay.values[0].State = "draft"
	stale, _ := call(true, 2, map[string]any{"cursor": first.Result.Structured.Next})
	if stale.Error == nil || stale.Error.Code != -32602 {
		t.Fatal("withdrawn task retained continuation")
	}
	fresh, _ := call(true, 2, map[string]any{})
	if fresh.Error != nil || fresh.Result.Structured.Count != 34 {
		t.Fatal("draft task remained discoverable")
	}
	t.Run("standalone_client", func(t *testing.T) {
		binary := os.Getenv("DOKOSOKO_MCP_ACCEPTANCE_CLIENT")
		if binary == "" {
			t.Skip("set the standalone acceptance-client binary to run actual client checks")
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		for _, scenario := range []struct {
			name      string
			public    bool
			arguments map[string]any
		}{
			{"public_first_page", true, map[string]any{}},
			{"public_next_page", true, map[string]any{"cursor": fresh.Result.Structured.Next}},
			{"private_api_query", false, map[string]any{"api_id": privateAPI.ID, "query": "catalogprivate v1.2"}},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "arguments.json")
				raw, _ := json.Marshal(scenario.arguments)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				endpoint := server.URL + "/mcp/public"
				token := ""
				if !scenario.public {
					endpoint = server.URL + "/mcp"
					token = "doko_private_demo"
				}
				args := []string{"run", "--endpoint", endpoint, "--allow-loopback-http", parsed.Host, "--expect-tool", "integration.recipes.list", "--call-tool", "integration.recipes.list", "--call-args-file", path, "--format", "json", "--token-env", "DOKOSOKO_CATALOG_TEST_TOKEN", "--restricted-token-env", "DOKOSOKO_CATALOG_RESTRICTED_TOKEN"}
				if !scenario.public {
					args = append(args, "--check-unauthenticated")
				}
				command := exec.CommandContext(t.Context(), binary, args...)
				command.Env = append(os.Environ(), "DOKOSOKO_CATALOG_TEST_TOKEN="+token, "DOKOSOKO_CATALOG_RESTRICTED_TOKEN=")
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("client failed: %v: %.1200s", err, output)
				}
				var report struct {
					Version string `json:"client_version"`
					Checks  []struct {
						Name      string `json:"name"`
						Status    string `json:"status"`
						Required  bool   `json:"required"`
						RequestID string `json:"request_id"`
					} `json:"checks"`
				}
				if json.Unmarshal(output, &report) != nil || report.Version != "0.3.0" {
					t.Fatal("client report did not identify its actual version")
				}
				found := false
				for _, check := range report.Checks {
					if check.Name == "tools/call: integration.recipes.list" {
						found = check.Status == "pass" && check.Required && check.RequestID != ""
					}
				}
				if !found {
					t.Fatal("client did not perform the recipe catalog call")
				}
				if directory := os.Getenv("DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR"); directory != "" {
					if err = os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(directory, "recipe_catalog_"+scenario.name+".json"), output, 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	})

}
