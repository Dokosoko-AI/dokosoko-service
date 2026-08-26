package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type apiDefaultToolBinding struct {
	Name          string
	IntegrationID string
	InputSchema   json.RawMessage
}

func validGeneratedToolSegment(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || (index > 0 && (char == '-' || char == '_'))
		if !valid {
			return false
		}
	}
	return true
}

func manifestFamilyCounts(manifest model.ProductManifest) map[string]int {
	result := make(map[string]int, len(manifest.Integrations))
	for _, integration := range manifest.Integrations {
		result[integration.FamilyKey]++
	}
	return result
}

func integrationHasKnowledge(integration model.IntegrationManifest) bool {
	for _, resource := range integration.Resources {
		if resource.Kind == "documentation" && len(resource.SourcePublications) > 0 {
			return true
		}
	}
	return false
}

func apiToolSchema(properties map[string]any, required ...string) map[string]any {
	value := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}

// apiDefaultToolDefinitions publishes one fixed knowledge-search tool per
// unambiguous API family. Provider provisioning is deliberately not part of
// the connector surface; API credentials are configured by administrators and
// injected server-side when reviewed HTTP or native tools execute.
func (s *Server) apiDefaultToolDefinitions(_ context.Context, _ string, manifest model.ProductManifest, _ identity.Principal, _ bool) ([]map[string]any, map[string]apiDefaultToolBinding) {
	definitions := make([]map[string]any, 0)
	bindings := make(map[string]apiDefaultToolBinding)
	familyCounts := manifestFamilyCounts(manifest)
	for _, integration := range manifest.Integrations {
		if familyCounts[integration.FamilyKey] != 1 || !validGeneratedToolSegment(integration.FamilyKey) || !integrationHasKnowledge(integration) {
			continue
		}
		name := integration.FamilyKey + ".knowledge.search"
		schema := apiToolSchema(map[string]any{"query": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000}}, "query")
		encoded, _ := json.Marshal(schema)
		binding := apiDefaultToolBinding{Name: name, IntegrationID: integration.ID, InputSchema: encoded}
		bindings[name] = binding
		definitions = append(definitions, map[string]any{
			"name":        name,
			"description": "Search only the reviewed documentation pinned by this published API revision.",
			"inputSchema": schema,
			"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true},
			"_meta":       map[string]any{"com.dokosoko/integrationId": integration.ID},
		})
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i]["name"].(string) < definitions[j]["name"].(string) })
	return definitions, bindings
}

func (s *Server) executeKnowledgeSearch(ctx context.Context, w http.ResponseWriter, requestID any, productID, integrationID string, query string, public bool, manifest model.ProductManifest, scope model.CatalogScope, principal identity.Principal) {
	publicationIDs, scopeErr := s.knowledgePublicationIDs(ctx, productID, integrationID, manifest)
	if scopeErr != nil {
		writeRPCError(w, requestID, -32003, "Select exactly one published API with reviewed documentation")
		return
	}
	if strings.TrimSpace(query) == "" {
		writeRPCError(w, requestID, -32602, "A knowledge query is required")
		return
	}
	var items []model.KnowledgeRecord
	var err error
	if public {
		items, err = s.service.Store().PublicKnowledge(ctx, productID, publicationIDs, query)
	} else {
		if principal.ProductID != productID {
			writeRPCError(w, requestID, -32003, "Knowledge access was denied by product policy")
			return
		}
		items, err = s.service.Store().PrivateKnowledge(ctx, productID, publicationIDs, query)
	}
	if err != nil {
		writeRPCError(w, requestID, -32603, "Search failed")
		return
	}
	filtered := make([]model.KnowledgeRecord, 0, len(items))
	for _, item := range items {
		allowed := false
		for _, kind := range []string{"docs", "openapi", "git"} {
			managed, candidateAllowed, allowErr := s.service.CatalogAllowsArtifact(ctx, productID, scope, kind, item.SourceID)
			if allowErr == nil && (!managed || candidateAllowed) {
				allowed = true
				break
			}
		}
		if allowed {
			filtered = append(filtered, item)
		}
	}
	writeToolResult(w, requestID, filtered)
}

func (s *Server) executeAPIDefaultTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, params toolCallParams, productID string, binding apiDefaultToolBinding, public bool, manifest model.ProductManifest, scope model.CatalogScope, principal identity.Principal) {
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	if err := toolruntime.ValidateArguments(binding.InputSchema, params.Arguments); err != nil {
		writeRPCError(w, request.ID, -32602, "Tool arguments do not match the declared input schema")
		return
	}
	query, _ := params.Arguments["query"].(string)
	s.executeKnowledgeSearch(ctx, w, request.ID, productID, binding.IntegrationID, query, public, manifest, scope, principal)
}

func canonicalCustomToolName(manifest model.ProductManifest, tool model.Tool) (string, bool) {
	switch tool.Scope {
	case "":
		if !validGeneratedToolSegment(tool.Namespace) || !validGeneratedToolSegment(tool.Name) {
			return "", false
		}
		return tool.Namespace + "." + tool.Name, true
	case model.ToolScopeCommon:
		if !validGeneratedToolSegment(tool.Name) {
			return "", false
		}
		return "common." + tool.Name, true
	case model.ToolScopeAPI:
		counts := manifestFamilyCounts(manifest)
		for _, integration := range manifest.Integrations {
			if integration.ID == tool.OwnerIntegrationID && counts[integration.FamilyKey] == 1 && validGeneratedToolSegment(integration.FamilyKey) && validGeneratedToolSegment(tool.Name) {
				return integration.FamilyKey + ".custom." + tool.Name, true
			}
		}
	}
	return "", false
}
