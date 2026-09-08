package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

const mcpCatalogVersionKey = "com.dokosoko/catalogVersion"
const mcpToolPageBytes = 960 << 10    // Fits both maximum-size escaped schemas, leaving room for the RPC envelope.
const mcpAPIManifestBytes = 256 << 10 // The MCP result also contains a JSON text copy.

func mcpToolCatalogPage(params json.RawMessage, tools []map[string]any, scope any) ([]map[string]any, string, error) {
	// Keep the small task/API routing surface on the first page, even when a
	// deployment publishes many tools whose names would sort ahead of it.
	priority := map[string]int{"integration.plan": 0, "deployment.apis.list": 1, "deployment.apis.get": 2, "developer_assets.search": 3, "integration.check": 4, "integration.recipes.list": 5, "search_knowledge": 6}
	return mcpOrderedCatalogPage(params, tools, scope, 32, mcpToolPageBytes, func(i, j int) bool {
		left, right := tools[i]["name"].(string), tools[j]["name"].(string)
		lp, lok := priority[left]
		rp, rok := priority[right]
		if lok != rok {
			return lok
		}
		if lok {
			return lp < rp
		}
		return left < right
	})
}

// Version 1 retains the full deployment/product catalog extensions. Version 2
// is the compact default; the MCP wire protocol and publication contents do not
// change. Reject an unknown explicit version instead of guessing its contract.
func mcpRequestedCatalogVersion(params json.RawMessage) (int, error) {
	var request struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(params, &request) != nil {
		return 0, errors.New("Invalid catalog metadata")
	}
	raw, exists := request.Meta[mcpCatalogVersionKey]
	if !exists {
		return 2, nil
	}
	var version int
	if json.Unmarshal(raw, &version) != nil || (version != 1 && version != 2) {
		return 0, errors.New("Catalog version must be 1 (legacy full catalog) or 2 (compact catalog)")
	}
	return version, nil
}

func catalogExcerpt(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func compactMCPCatalog(manifest model.ProductManifest) map[string]any {
	return map[string]any{
		"deployment_id": manifest.DeploymentID, "deployment_slug": manifest.DeploymentSlug,
		"name": catalogExcerpt(manifest.DeploymentName, 200), "description_excerpt": catalogExcerpt(manifest.Description, 500),
		"catalog_revision": manifest.CatalogRevision, "api_count": len(manifest.Integrations),
		"apis_tool": "deployment.apis.list", "api_manifest_tool": "deployment.apis.get",
	}
}

func mcpAPIListInputSchema() map[string]any {
	return apiToolSchema(map[string]any{
		"query":  map[string]any{"type": "string", "maxLength": 500, "description": "Optional words to match in the published API name, family, exact version, or description. All words must match."},
		"cursor": map[string]any{"type": "string", "minLength": 1, "maxLength": mcpCursorMaxBytes},
	})
}

func mcpAPIGetInputSchema() map[string]any {
	return apiToolSchema(map[string]any{
		"api_id":        map[string]any{"type": "string", "minLength": 1, "maxLength": 100},
		"manifest_hash": map[string]any{"type": "string", "minLength": 1, "maxLength": 100, "description": "Exact manifest_hash returned by deployment.apis.list. A changed publication requires a new selection."},
	}, "api_id", "manifest_hash")
}

func validCatalogArguments(schema map[string]any, arguments map[string]any) bool {
	if arguments == nil {
		arguments = map[string]any{}
	}
	raw, err := json.Marshal(schema)
	return err == nil && toolruntime.ValidateArguments(raw, arguments) == nil
}

func compactAPIToolDefinitions() []map[string]any {
	readOnly := map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true}
	return []map[string]any{
		{"name": "deployment.apis.list", "description": "Find published APIs by name and version. Returns one compact page; continue with next_cursor using the same query. Select an exact API and manifest hash before reading its manifest.", "inputSchema": mcpAPIListInputSchema(), "annotations": readOnly},
		{"name": "deployment.apis.get", "description": "Read the manifest of one selected published API. Requires the exact API ID and manifest hash from deployment.apis.list; publication changes never silently replace the selection.", "inputSchema": mcpAPIGetInputSchema(), "annotations": readOnly},
	}
}

func writeMCPCatalogError(w http.ResponseWriter, id any, err error) {
	if errors.Is(err, errMCPCursor) {
		writeRPCError(w, id, -32602, err.Error())
		return
	}
	writeRPCError(w, id, -32603, "A catalog entry exceeds the discovery budget or could not be encoded")
}

func (s *Server) listMCPAPIs(ctx context.Context, w http.ResponseWriter, request rpcRequest, arguments map[string]any, productID string, public bool, manifest model.ProductManifest) {
	var input struct {
		Query  string  `json:"query"`
		Cursor *string `json:"cursor"`
	}
	if !validCatalogArguments(mcpAPIListInputSchema(), arguments) || decodeArguments(arguments, &input) != nil {
		writeRPCError(w, request.ID, -32602, "API catalog accepts a query of at most 500 characters and an optional opaque cursor")
		return
	}
	query := strings.ToLower(strings.Join(strings.Fields(input.Query), " "))
	words := strings.Fields(query)
	values := make([]map[string]any, 0, len(manifest.Integrations))
	for _, api := range manifest.Integrations {
		searchable := strings.ToLower(strings.Join([]string{api.DisplayName, api.FamilyKey, api.VersionKey, api.Description}, " "))
		matches := true
		for _, word := range words {
			matches = matches && strings.Contains(searchable, word)
		}
		if !matches {
			continue
		}
		values = append(values, map[string]any{
			"id": api.ID, "family_key": api.FamilyKey, "version_key": api.VersionKey,
			"display_name": catalogExcerpt(api.DisplayName, 200), "description_excerpt": catalogExcerpt(api.Description, 500),
			"visibility": api.Visibility, "lifecycle": api.Lifecycle, "revision": api.Revision, "manifest_hash": api.ManifestHash,
		})
	}
	params := map[string]any{}
	if input.Cursor != nil {
		params["cursor"] = *input.Cursor
	}
	raw, _ := json.Marshal(params)
	scope := map[string]any{"request": mcpCatalogScope(ctx, productID, public, manifest.CatalogRevision, "deployment.apis.list"), "query": query}
	page, next, err := mcpCatalogPage(raw, values, scope, "id", 32, 64<<10)
	if err != nil {
		writeMCPCatalogError(w, request.ID, err)
		return
	}
	result := map[string]any{"catalog_version": 2, "catalog_revision": manifest.CatalogRevision, "apis": page}
	if next != "" {
		result["next_cursor"] = next
	}
	writeToolResult(w, request.ID, result)
}

func (s *Server) getMCPAPI(ctx context.Context, w http.ResponseWriter, request rpcRequest, arguments map[string]any, manifest model.ProductManifest) {
	var input struct {
		APIID        string `json:"api_id"`
		ManifestHash string `json:"manifest_hash"`
	}
	if !validCatalogArguments(mcpAPIGetInputSchema(), arguments) || decodeArguments(arguments, &input) != nil {
		writeRPCError(w, request.ID, -32602, "Select one exact api_id and manifest_hash from deployment.apis.list")
		return
	}
	for _, api := range manifest.Integrations {
		if api.ID != input.APIID {
			continue
		}
		if api.ManifestHash != input.ManifestHash {
			writeRPCError(w, request.ID, -32009, "The selected API publication changed; refresh deployment.apis.list and select its exact manifest hash")
			return
		}
		result := map[string]any{"catalog_version": 2, "catalog_revision": manifest.CatalogRevision, "api": api}
		// The legacy manifest projection omits modern developer-asset bindings.
		// Route the selected API directly to its exact ready publication map.
		revisions, err := s.service.Store().IntegrationRevisions(ctx, api.ID)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "The selected API publication could not be resolved")
			return
		}
		matched := false
		for _, revision := range revisions {
			if revision.IntegrationID != api.ID || revision.State != "published" || revision.PublishedAt == nil || revision.Revision != api.Revision || revision.ManifestHash != api.ManifestHash {
				continue
			}
			var snapshot struct {
				DeveloperAssets struct {
					SchemaVersion string `json:"schema_version"`
				} `json:"developer_assets"`
			}
			if json.Unmarshal(revision.Snapshot, &snapshot) != nil {
				writeRPCError(w, request.ID, -32603, "The selected API publication is invalid")
				return
			}
			if snapshot.DeveloperAssets.SchemaVersion != "" {
				publication, lookupErr := s.service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
				if lookupErr != nil {
					writeRPCError(w, request.ID, -32603, "The selected API's developer-asset publication is not available; refresh the catalog")
					return
				}
				if publication.APIRevisionID != revision.ID || publication.APIID != api.ID || publication.DeploymentID != manifest.DeploymentID {
					writeRPCError(w, request.ID, -32009, "The selected API publication changed; refresh deployment.apis.list")
					return
				}
				result["publication"] = map[string]any{"id": publication.ID, "api_revision_id": revision.ID, "snapshot_hash": publication.SnapshotHash, "uri": developerAssetResourceURI("apis", api.ID, "publications", publication.ID) + mcpPublicationMapSuffix}
			}
			matched = true
			break
		}
		if !matched {
			writeRPCError(w, request.ID, -32009, "The selected API publication changed; refresh deployment.apis.list")
			return
		}
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > mcpAPIManifestBytes {
			writeRPCError(w, request.ID, -32010, "The selected API manifest exceeds its read budget; use its publication map and scoped developer_assets.search")
			return
		}
		writeToolResult(w, request.ID, result)
		return
	}
	writeRPCError(w, request.ID, -32004, "API not found in this deployment publication scope")
}
