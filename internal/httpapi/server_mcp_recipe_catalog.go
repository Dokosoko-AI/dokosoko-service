package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func mcpRecipeListInputSchema() map[string]any {
	return apiToolSchema(map[string]any{
		"query":  map[string]any{"type": "string", "maxLength": 500, "description": "Words matching the published task title, slug, outcome, or its exact API name and version. All words must match."},
		"api_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 100, "description": "Optional exact API ID from deployment.apis.list. Only recipes using that API are returned."},
		"cursor": map[string]any{"type": "string", "minLength": 1, "maxLength": mcpCursorMaxBytes},
	})
}

func mcpRecipeAPIVersionSchema() map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"api_id", "family_key", "version_key", "display_name", "integration_revision_id", "manifest_hash"} {
		properties[key] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": []string{"api_id", "family_key", "version_key", "display_name", "integration_revision_id", "manifest_hash"}}
}

func compactRecipeSummarySchema() map[string]any {
	schema := recipeSummaryOutputSchema()
	schema["properties"].(map[string]any)["api_versions"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": mcpRecipeAPIVersionSchema()}
	schema["required"] = append(schema["required"].([]string), "api_versions")
	return schema
}

func mcpRecipeListOutputSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"catalog_version": map[string]any{"const": 2}, "catalog_revision": map[string]any{"type": "integer"},
		"recipe_count": map[string]any{"type": "integer", "minimum": 0},
		"recipes":      map[string]any{"type": "array", "maxItems": 32, "items": compactRecipeSummarySchema()},
		"next_cursor":  map[string]any{"type": "string"},
	}, "required": []string{"catalog_version", "catalog_revision", "recipe_count", "recipes"}}
}

// Labels come from the recipe's immutable API revision, even when unrelated
// changes have produced a newer API publication without invalidating the task.
func (s *Server) recipeCatalogVersionResolver(ctx context.Context, manifest model.ProductManifest, public bool) func(model.RecipeAPIBinding) (map[string]any, error) {
	histories := make(map[string][]model.IntegrationRevision)
	allowed := make(map[string]bool)
	for _, api := range manifest.Integrations {
		allowed[api.ID] = !public || api.Visibility == model.VisibilityPublic
	}
	return func(binding model.RecipeAPIBinding) (map[string]any, error) {
		if !allowed[binding.IntegrationID] {
			return nil, errors.New("recipe API is outside the current audience scope")
		}
		revisions, loaded := histories[binding.IntegrationID]
		if !loaded {
			var err error
			revisions, err = s.service.Store().IntegrationRevisions(ctx, binding.IntegrationID)
			if err != nil {
				return nil, err
			}
			histories[binding.IntegrationID] = revisions
		}
		for _, revision := range revisions {
			if revision.ID != binding.IntegrationRevisionID {
				continue
			}
			if revision.IntegrationID != binding.IntegrationID || revision.State != "published" || revision.PublishedAt == nil || revision.ManifestHash != binding.IntegrationManifestHash {
				return nil, errors.New("recipe API revision is invalid")
			}
			var snapshot struct {
				FamilyKey   string           `json:"family_key"`
				VersionKey  string           `json:"version_key"`
				DisplayName string           `json:"display_name"`
				Visibility  model.Visibility `json:"visibility"`
			}
			if json.Unmarshal(revision.Snapshot, &snapshot) != nil || snapshot.FamilyKey == "" || snapshot.VersionKey == "" || snapshot.DisplayName == "" || !publicDeveloperAssetVisibilityAllowed(public, snapshot.Visibility) {
				return nil, errors.New("recipe API snapshot is unavailable in this audience")
			}
			return map[string]any{"api_id": binding.IntegrationID, "family_key": snapshot.FamilyKey, "version_key": snapshot.VersionKey, "display_name": catalogExcerpt(snapshot.DisplayName, 200), "integration_revision_id": revision.ID, "manifest_hash": revision.ManifestHash}, nil
		}
		return nil, errors.New("recipe API revision is unavailable")
	}
}

func compactRecipeSummary(value model.Recipe, resolve func(model.RecipeAPIBinding) (map[string]any, error)) (map[string]any, error) {
	if value.CurrentRevision == nil {
		return nil, errors.New("recipe has no exact revision")
	}
	bindings := value.CurrentRevision.APIBindings
	if value.ContractVersion == model.RecipeContractProductIntegrationV2 {
		bindings = []model.RecipeAPIBinding{{IntegrationID: value.IntegrationID, IntegrationRevisionID: value.CurrentRevision.IntegrationRevisionID, IntegrationManifestHash: value.CurrentRevision.IntegrationManifestHash}}
	}
	ids := recipeAPIIDs(value)
	if len(ids) == 0 || len(ids) > 8 || len(ids) != len(bindings) {
		return nil, errors.New("recipe API bindings are incomplete")
	}
	versions := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		matches := 0
		for _, binding := range bindings {
			if binding.IntegrationID != id {
				continue
			}
			if binding.IntegrationRevisionID == "" || binding.IntegrationManifestHash == "" {
				return nil, errors.New("recipe API publication changed")
			}
			version, err := resolve(binding)
			if err != nil {
				return nil, err
			}
			versions = append(versions, version)
			matches++
		}
		if matches != 1 {
			return nil, errors.New("recipe API binding is ambiguous")
		}
	}
	raw, err := json.Marshal(recipeSummary(value))
	if err != nil {
		return nil, err
	}
	var summary map[string]any
	if err = json.Unmarshal(raw, &summary); err != nil {
		return nil, err
	}
	summary["api_versions"] = versions
	return summary, nil
}

func recipeCatalogPage(params json.RawMessage, entries []map[string]any, scope any, limit int) ([]map[string]any, string, error) {
	return mcpOrderedCatalogPage(params, entries, scope, limit, 64<<10, func(i, j int) bool {
		left, right := strings.ToLower(entries[i]["title"].(string)), strings.ToLower(entries[j]["title"].(string))
		if left != right {
			return left < right
		}
		return entries[i]["uri"].(string) < entries[j]["uri"].(string)
	})
}

func (s *Server) listMCPRecipes(ctx context.Context, w http.ResponseWriter, request rpcRequest, arguments map[string]any, productID string, public bool, manifest model.ProductManifest) {
	var input struct {
		Query  string  `json:"query"`
		APIID  string  `json:"api_id"`
		Cursor *string `json:"cursor"`
	}
	if !validCatalogArguments(mcpRecipeListInputSchema(), arguments) || decodeArguments(arguments, &input) != nil {
		writeRPCError(w, request.ID, -32602, "Recipe discovery accepts a query of at most 500 characters, an optional api_id and an opaque cursor")
		return
	}
	values, err := s.publishedRecipes(ctx, productID, public)
	if err != nil {
		writeRPCError(w, request.ID, -32603, "Published tasks could not be listed")
		return
	}
	query := normalizeRecipeLookup(input.Query)
	words := strings.Fields(query)
	entries := make([]map[string]any, 0)
	resolve := s.recipeCatalogVersionResolver(ctx, manifest, public)
	for _, value := range values {
		ids := recipeAPIIDs(value)
		selectedAPI := input.APIID == ""
		for _, id := range ids {
			selectedAPI = selectedAPI || id == input.APIID
		}
		if !selectedAPI {
			continue
		}
		summary, err := compactRecipeSummary(value, resolve)
		if err != nil {
			writeRPCError(w, request.ID, -32009, "A task's exact API publication is unavailable or changed; refresh recipe discovery")
			return
		}
		searchable := value.Title + " " + value.Slug + " " + value.Outcome
		for _, version := range summary["api_versions"].([]map[string]any) {
			searchable += " " + version["display_name"].(string) + " " + version["family_key"].(string) + " " + version["version_key"].(string)
		}
		searchable = normalizeRecipeLookup(searchable)
		matches := true
		for _, word := range words {
			matches = matches && strings.Contains(searchable, word)
		}
		if matches {
			entries = append(entries, summary)
		}
	}
	params := map[string]any{}
	if input.Cursor != nil {
		params["cursor"] = *input.Cursor
	}
	raw, _ := json.Marshal(params)
	scope := map[string]any{"request": mcpCatalogScope(ctx, productID, public, manifest.CatalogRevision, "integration.recipes.list"), "query": query, "api_id": input.APIID}
	page, next, err := recipeCatalogPage(raw, entries, scope, 32)
	if err != nil {
		writeMCPCatalogError(w, request.ID, err)
		return
	}
	result := map[string]any{"catalog_version": 2, "catalog_revision": manifest.CatalogRevision, "recipe_count": len(entries), "recipes": page}
	if next != "" {
		result["next_cursor"] = next
	}
	writeToolResult(w, request.ID, result)
}
