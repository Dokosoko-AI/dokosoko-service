package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/ratelimit"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func (s *Server) publicMCP(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	if productID == "" {
		deployment, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			writeError(w, http.StatusNotFound, "public_mcp_unavailable", "Public MCP is not configured for this deployment.", nil)
			return
		}
		productID = deployment.ID
	}
	product, err := s.service.Store().Product(r.Context(), productID)
	if err != nil || !product.PublicMCPEnabled {
		writeError(w, http.StatusNotFound, "public_mcp_unavailable", "Public MCP is not enabled for this deployment.", nil)
		return
	}
	if !s.allowAnonymous(productID, r.RemoteAddr, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Public MCP request limit exceeded.", nil)
		return
	}
	w.Header().Set("X-RateLimit-Limit", "120")
	s.handleMCP(w, r, productID, true)
}

func (s *Server) allowAnonymous(productID, remoteAddress string, now time.Time) bool {
	return s.allowFixedWindow("public|"+productID+"|"+remoteHost(remoteAddress), 120, now)
}

func remoteHost(remoteAddress string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddress))
	if err == nil {
		return host
	}
	return strings.Trim(strings.TrimSpace(remoteAddress), "[]")
}

func (s *Server) allowFixedWindow(key string, limit int, now time.Time) bool {
	s.rateOnce.Do(func() {
		if s.rateLimiter == nil {
			s.rateLimiter = ratelimit.NewFixedWindow(time.Minute, maxHTTPRateWindows)
		}
	})
	return s.rateLimiter.Allow(key, limit, now)
}

func (s *Server) privateMCP(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("productID")
	if productID == "" {
		deployment, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			writeError(w, http.StatusNotFound, "mcp_unavailable", "Private MCP is not configured for this deployment.", nil)
			return
		}
		productID = deployment.ID
	}
	var principal identity.Principal
	if s.identityBroker != nil {
		value, err := s.identityBroker.Authenticate(r.Context(), bearerToken(r))
		if err == nil && value.ProductID == productID {
			principal = value
		}
	}
	if principal.Subject == "" && s.allowDemoTokens && isBearer(r, demoPrivateToken) {
		principal = identity.Principal{ProductID: productID, ClientID: productID, Issuer: "development", Subject: "private_mcp_demo", Grants: map[string]bool{}, AccessEvaluatedAt: time.Now().UTC()}
	}
	if principal.Subject == "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, s.baseURL+"/.well-known/oauth-protected-resource/mcp", "mcp:private"))
		writeError(w, http.StatusUnauthorized, "authentication_required", "Private MCP requires a DokoSoko access token.", nil)
		return
	}
	if !s.allowFixedWindow("private-mcp|"+productID+"|"+vendorActorID(principal), 600, time.Now().UTC()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Private MCP request limit exceeded.", nil)
		return
	}
	s.handleMCP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)), productID, false)
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request, productID string, public bool) {
	var request rpcRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := s.validateStatelessMCPv2(r, request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32022, "message": err.Error(), "data": map[string]any{"supported": []string{model.StatelessMCPv2Protocol}, "policy": "Stateless MCPv2 Only", "specification": "https://blog.modelcontextprotocol.io/posts/2026-07-28/"}}})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	scope := model.CatalogScope{Public: public}
	if !public {
		_ = s.syncNativePlugins(r.Context(), productID)
	}
	productManifest, manifestErr := s.service.ProductManifestFor(r.Context(), productID, scope)
	if manifestErr != nil {
		writeRPCError(w, request.ID, -32603, "Deployment context could not be resolved")
		return
	}
	switch request.Method {
	case "server/discover":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		cacheScope := "private"
		if public {
			cacheScope = "public"
		}
		instructions := "Use the current deployment catalog and the exact immutable API publication revisions returned in discovery."
		if !public && s.reporting != nil {
			capabilities, _ := s.reporting.Capabilities(r.Context(), productID)
			reportingEnabled := false
			for _, capability := range capabilities {
				reportingEnabled = reportingEnabled || capability.BugReportsEnabled || capability.FeedbackEnabled
			}
			if reportingEnabled {
				instructions += reportingAgentInstructions
			}
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "supportedVersions": []string{model.StatelessMCPv2Protocol}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}, "resources": map[string]any{"listChanged": true}}, "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "instructions": instructions, "ttlMs": 30000, "cacheScope": cacheScope})
	case "resources/list":
		values, err := s.publishedRecipes(r.Context(), productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipe resources could not be listed")
			return
		}
		resources := make([]map[string]any, 0, len(values))
		for _, recipe := range values {
			resources = append(resources, map[string]any{"uri": recipe.StableURI, "name": recipe.Slug, "title": recipe.Title, "description": recipe.Outcome, "mimeType": "text/markdown", "_meta": map[string]any{"generated": recipe.Generated, "state": recipe.State, "revision_id": recipe.CurrentRevisionID}})
		}
		writeRPC(w, request.ID, map[string]any{"resources": resources})
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(request.Params, &params) != nil || params.URI == "" {
			writeRPCError(w, request.ID, -32602, "A recipe URI is required")
			return
		}
		recipe, err := s.publishedRecipeByURI(r.Context(), productID, params.URI, public)
		if err != nil || recipe.CurrentRevision == nil {
			writeRPCError(w, request.ID, -32004, "Recipe resource not found")
			return
		}
		writeRPC(w, request.ID, map[string]any{"contents": []map[string]any{{"uri": recipe.StableURI, "mimeType": "text/markdown", "text": recipe.CurrentRevision.Markdown, "_meta": map[string]any{"revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt}}}})
	case "tools/list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		tools := []map[string]any{
			{"name": "deployment.get_manifest", "description": "Return this DokoSoko deployment and its exact immutable API publication revisions.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "integration.recipes.list", "description": "List published implementation recipes and their stable MCP resource URIs.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "integration.plan", "description": "Choose the closest published recipe for a requested integration outcome. This returns a plan reference, not a claim that work was completed.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"outcome": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"outcome"}}},
			{"name": "integration.check", "description": "Check whether a published recipe URI is current or needs attention before implementation.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"recipe_uri": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"recipe_uri"}}},
		}
		principal, _ := r.Context().Value(principalKey).(identity.Principal)
		if len(productManifest.Integrations) == 0 {
			tools = append(tools, map[string]any{"name": "search_knowledge", "description": "Search the latest reviewed documentation for a legacy deployment without an Integration catalog.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}})
		} else {
			generated, _ := s.apiDefaultToolDefinitions(r.Context(), productID, productManifest, principal, public)
			tools = append(tools, generated...)
		}
		if !public {
			if s.reporting != nil {
				capabilities, _ := s.reporting.Capabilities(r.Context(), productID)
				bugEnabled, feedbackEnabled := false, false
				for _, capability := range capabilities {
					bugEnabled = bugEnabled || capability.BugReportsEnabled
					feedbackEnabled = feedbackEnabled || capability.FeedbackEnabled
				}
				metadata := map[string]any{"com.dokosoko/supportCapabilities": capabilities}
				if bugEnabled {
					definition := bugReportToolDefinition()
					definition["_meta"] = mergeMetadata(definition["_meta"], metadata)
					tools = append(tools, definition)
				}
				if feedbackEnabled {
					definition := feedbackToolDefinition()
					definition["_meta"] = mergeMetadata(definition["_meta"], metadata)
					tools = append(tools, definition)
				}
			}
			if s.toolRuntime != nil {
				custom, err := s.toolRuntime.Published(r.Context(), productID)
				if err == nil {
					type namedCustomDefinition struct {
						name       string
						definition map[string]any
					}
					candidates := make([]namedCustomDefinition, 0, len(custom))
					nameCounts := make(map[string]int)
					for _, item := range custom {
						_, allowed, allowErr := s.service.CatalogAllowsTool(r.Context(), productID, scope, item)
						if allowErr != nil || !allowed {
							continue
						}
						binding, managedByIntegration, bindingErr := s.integrationToolAuthorization(r.Context(), productManifest, item)
						if managedByIntegration {
							if bindingErr != nil {
								continue
							}
							available, availableErr := s.toolRuntime.AvailableBound(r.Context(), productID, []toolruntime.BoundAuthorization{binding}, toolPrincipal(principal, false, "", ""))
							if availableErr != nil || len(available) != 1 || available[0].ID != item.ID {
								continue
							}
						} else {
							available, availableErr := s.toolRuntime.Available(r.Context(), productID, principal.Grants)
							legacyAllowed := false
							for _, candidate := range available {
								legacyAllowed = legacyAllowed || candidate.ID == item.ID
							}
							if availableErr != nil || !legacyAllowed {
								continue
							}
						}
						canonicalName, canonical := canonicalCustomToolName(productManifest, item)
						if !canonical {
							continue
						}
						definition := customToolDefinitionForAuthorization(productManifest, item, binding, managedByIntegration)
						definition["name"] = canonicalName
						if len(item.OutputSchema) > 0 {
							definition["outputSchema"] = item.OutputSchema
						}
						nameCounts[canonicalName]++
						candidates = append(candidates, namedCustomDefinition{name: canonicalName, definition: definition})
					}
					for _, candidate := range candidates {
						if nameCounts[candidate.name] == 1 {
							tools = append(tools, candidate.definition)
						}
					}
				}
			}
		}
		cacheScope := "private"
		if public {
			cacheScope = "public"
		}
		catalogMeta := map[string]any{"deployment_id": productManifest.DeploymentID, "catalog_revision": productManifest.CatalogRevision}
		for _, definition := range tools {
			metadata, _ := definition["_meta"].(map[string]any)
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata["com.dokosoko/catalog"] = catalogMeta
			definition["_meta"] = metadata
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "tools": tools, "ttlMs": 30000, "cacheScope": cacheScope})
	case "tools/call":
		s.callTool(r.Context(), w, request, productID, public, productManifest, manifestErr)
	default:
		writeRPCError(w, request.ID, -32601, "Method not found")
	}
}
