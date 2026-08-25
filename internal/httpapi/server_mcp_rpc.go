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
	channel, actorKind, actorID := "public_mcp", "anonymous", ""
	selection := model.ProductSelectionContext{Public: public}
	if !public {
		principal, _ := r.Context().Value(principalKey).(identity.Principal)
		channel, actorKind, actorID = "private_mcp", "vendor_user", pseudonym(productID, principal)
		selection.CustomerAccountID, selection.InstallationID = principal.CustomerAccountID, principal.InstallationID
	}
	productManifest, manifestErr := s.service.ProductManifestFor(r.Context(), productID, selection)
	if manifestErr != nil {
		writeRPCError(w, request.ID, -32603, "Deployment context could not be resolved")
		return
	}
	analyticsDimensions := map[string]any{"channel": channel, "method": request.Method}
	if manifestErr == nil {
		analyticsDimensions["catalog_revision"] = productManifest.CatalogRevision
		analyticsDimensions["selection_source"] = productManifest.SelectionSource
		analyticsDimensions["environment_id"] = productManifest.EnvironmentID
		analyticsDimensions["installation_id"] = productManifest.InstallationID
		if productManifest.EffectiveVersion != nil {
			analyticsDimensions["product_version_id"] = productManifest.EffectiveVersion.ID
			analyticsDimensions["product_version"] = productManifest.EffectiveVersion.Version
			analyticsDimensions["manifest_hash"] = productManifest.ManifestHash
		}
	}
	s.recordAnalytics(r.Context(), productID, "mcp.request", actorKind, actorID, analyticsDimensions)
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
		instructions := "Use the effective DokoSoko connector release and Integration revisions returned in discovery. Authenticated installation, environment, and customer pins override default deployment channels in that order."
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
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "supportedVersions": []string{model.StatelessMCPv2Protocol}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}, "resources": map[string]any{"listChanged": true}}, "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "instructions": instructions, "ttlMs": 30000, "cacheScope": cacheScope})
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
		s.recordAnalytics(r.Context(), productID, "recipe.view", actorKind, actorID, map[string]any{"recipe_id": recipe.ID, "recipe_slug": recipe.Slug, "channel": channel})
		writeRPC(w, request.ID, map[string]any{"contents": []map[string]any{{"uri": recipe.StableURI, "mimeType": "text/markdown", "text": recipe.CurrentRevision.Markdown, "_meta": map[string]any{"revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt}}}})
	case "tools/list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		tools := []map[string]any{
			{"name": "deployment.get_manifest", "description": "Return this DokoSoko deployment, its applicable Integration revisions, effective pinned or default connector release, and available releases.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
			{"name": "deployment.releases.list", "description": "List published connector releases and their latest, LTS, deprecated, replacement, and sunset metadata.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}},
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
			tools = append(tools,
				map[string]any{"name": "integration_runs.start", "description": "Start an environment-scoped integration outcome run.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"environment_id": map[string]any{"type": "string"}, "requested_outcome": map[string]any{"type": "string", "maxLength": 500}}, "required": []string{"environment_id", "requested_outcome"}}},
				map[string]any{"name": "integration_runs.complete", "description": "Complete a run with a deterministic validation result.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"run_id": map[string]any{"type": "string"}, "reported_success": map[string]any{"type": "boolean"}, "validated_success": map[string]any{"type": "boolean"}, "failure_code": map[string]any{"type": "string", "maxLength": 120}}, "required": []string{"run_id", "validated_success"}}},
			)
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
			if s.mcpBridge != nil {
				connections, _ := s.service.Store().MCPConnections(r.Context(), productID)
				for _, connection := range connections {
					if connection.AuthMode == "delegated_oauth" && connection.State == "active" {
						tools = append(tools, map[string]any{"name": "mcp_connections.authorize", "description": "Create a short-lived authorization URL that connects your identity to a delegated Stateless MCPv2 upstream.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}}, "required": []string{"connection_id"}}})
						break
					}
				}
			}
			if s.accessRuntime != nil && len(productManifest.Integrations) == 0 {
				capabilities := s.accessRuntime.Capabilities(r.Context(), productID, principal.Grants)
				if len(capabilities) > 0 {
					metadata := map[string]any{"com.dokosoko/accessConnections": capabilities}
					canCreateInstance, canCreateCredential, canRevokeCredential := false, false, false
					for _, capability := range capabilities {
						canCreateInstance = canCreateInstance || capability.CanCreateInstance
						canCreateCredential = canCreateCredential || capability.CanCreateCredential
						canRevokeCredential = canRevokeCredential || capability.CanRevokeCredential
					}
					tools = append(tools,
						map[string]any{"name": "access.instances.list", "description": "List provider-owned resources available to the authenticated subject. The provider-specific resource label and allowed Integrations are supplied in tool metadata.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}}, "required": []string{"connection_id", "integration_id"}}, "_meta": metadata},
						map[string]any{"name": "access.credentials.list", "description": "List credential metadata and fingerprints for an allowed provider connection or resource. Credential material is never returned by list operations.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "access_instance_id": map[string]any{"type": "string"}}, "required": []string{"connection_id", "integration_id"}}, "_meta": metadata},
					)
					if canCreateInstance {
						tools = append(tools, map[string]any{"name": "access.instances.create", "description": "Create an idempotent provider resource using the provider-specific label shown in tool metadata. This tool is omitted for single-instance services.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "display_name": map[string]any{"type": "string", "maxLength": 160}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"connection_id", "integration_id", "environment_id", "display_name", "idempotency_key"}}, "_meta": metadata})
					}
					if canCreateCredential {
						tools = append(tools, map[string]any{"name": "access.credentials.create", "description": "Create scoped credential material once for an allowed provider connection or resource. DokoSoko retains only a fingerprint unless the provider definition explicitly requires encrypted managed storage.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "integration_id": map[string]any{"type": "string"}, "environment_id": map[string]any{"type": "string"}, "access_instance_id": map[string]any{"type": "string"}, "scopes": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string"}}, "idempotency_key": map[string]any{"type": "string", "minLength": 16}, "ttl_seconds": map[string]any{"type": "integer", "minimum": 300}}, "required": []string{"connection_id", "integration_id", "environment_id", "scopes", "idempotency_key"}}, "_meta": metadata})
					}
					if canRevokeCredential {
						tools = append(tools, map[string]any{"name": "access.credentials.revoke", "description": "Revoke provider credential material owned by the authenticated subject.", "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"credential_id": map[string]any{"type": "string"}}, "required": []string{"credential_id"}}, "_meta": metadata})
					}
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
						_, allowed, allowErr := s.service.ProductVersionAllowsToolFor(r.Context(), productID, selection, item)
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
		versionMeta := map[string]any{"product_id": productManifest.ProductID, "catalog_revision": productManifest.CatalogRevision, "manifest_hash": productManifest.ManifestHash, "definition_revision": productManifest.DefinitionRevision, "selection_source": productManifest.SelectionSource, "environment_id": productManifest.EnvironmentID, "installation_id": productManifest.InstallationID}
		if productManifest.EffectiveVersion != nil {
			versionMeta["version"] = productManifest.EffectiveVersion.Version
			versionMeta["is_latest"] = productManifest.EffectiveVersion.IsLatest
			versionMeta["is_lts"] = productManifest.EffectiveVersion.IsLTS
			versionMeta["deprecated"] = productManifest.EffectiveVersion.Deprecated
		}
		for _, definition := range tools {
			metadata, _ := definition["_meta"].(map[string]any)
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata["com.dokosoko/productVersion"] = versionMeta
			metadata["com.dokosoko/deploymentRelease"] = versionMeta
			definition["_meta"] = metadata
		}
		writeRPC(w, request.ID, map[string]any{"resultType": "complete", "deployment": productManifest, "product": productManifest, "catalogRevision": productManifest.CatalogRevision, "manifestHash": productManifest.ManifestHash, "tools": tools, "ttlMs": 30000, "cacheScope": cacheScope})
	case "tools/call":
		s.callTool(r.Context(), w, request, productID, public, productManifest, manifestErr)
	default:
		writeRPCError(w, request.ID, -32601, "Method not found")
	}
}
