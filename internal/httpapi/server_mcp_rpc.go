package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/ratelimit"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"net"
	"net/http"
	"strings"
	"time"
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

func (s *Server) validateStatelessMCPv2(r *http.Request, request rpcRequest) error {
	if request.JSONRPC != "2.0" || request.Method == "" {
		return errors.New("a JSON-RPC 2.0 method is required")
	}
	if r.Header.Get("MCP-Protocol-Version") != model.StatelessMCPv2Protocol {
		return errors.New("this endpoint is Stateless MCPv2 Only and requires MCP-Protocol-Version 2026-07-28")
	}
	if r.Header.Get("Mcp-Method") != request.Method {
		return errors.New("Mcp-Method must exactly match the JSON-RPC method")
	}
	var params map[string]json.RawMessage
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return errors.New("request params must contain Stateless MCPv2 metadata")
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(params["_meta"], &meta) != nil {
		return errors.New("request params._meta is required")
	}
	var protocolVersion string
	if json.Unmarshal(meta["io.modelcontextprotocol/protocolVersion"], &protocolVersion) != nil || protocolVersion != model.StatelessMCPv2Protocol {
		return errors.New("params._meta must declare protocol version 2026-07-28")
	}
	if request.Method == "tools/call" {
		var name string
		if json.Unmarshal(params["name"], &name) != nil || name == "" || r.Header.Get("Mcp-Name") != name {
			return errors.New("Mcp-Name must exactly match the requested tool name")
		}
	}
	if origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"); origin != "" && origin != s.baseURL {
		return errors.New("the request Origin is not allowed")
	}
	return nil
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      struct {
		Confirmed             bool   `json:"confirmed"`
		ConfirmationChallenge string `json:"confirmation_challenge"`
		IdempotencyKey        string `json:"idempotency_key"`
	} `json:"_meta"`
}

func decodeToolCallParams(raw json.RawMessage) (toolCallParams, error) {
	var params toolCallParams
	paramsDecoder := json.NewDecoder(bytes.NewReader(raw))
	paramsDecoder.UseNumber()
	err := paramsDecoder.Decode(&params)
	return params, err
}

type managedToolConfirmationChallenge struct {
	Nonce     string
	ExpiresAt time.Time
}

type managedToolConfirmationHashInput struct {
	ProductID                  string         `json:"product_id"`
	ToolID                     string         `json:"tool_id"`
	ToolRevision               int64          `json:"tool_revision"`
	IntegrationID              string         `json:"integration_id"`
	AuthorizationPointID       string         `json:"authorization_point_id"`
	AuthorizationPointRevision int64          `json:"authorization_point_revision"`
	Issuer                     string         `json:"issuer"`
	Subject                    string         `json:"subject"`
	CustomerAccountID          string         `json:"customer_account_id"`
	InstallationID             string         `json:"installation_id"`
	AccessEvaluationID         string         `json:"access_evaluation_id"`
	AccessEvaluatedAt          string         `json:"access_evaluated_at"`
	IdempotencyKey             string         `json:"idempotency_key"`
	Arguments                  map[string]any `json:"arguments"`
}

func managedToolConfirmationArgumentHash(productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string) ([]byte, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	payload, err := json.Marshal(managedToolConfirmationHashInput{
		ProductID:                  productID,
		ToolID:                     tool.ID,
		ToolRevision:               tool.Revision,
		IntegrationID:              binding.IntegrationID,
		AuthorizationPointID:       binding.AuthorizationPoint.ID,
		AuthorizationPointRevision: binding.AuthorizationPointRevision,
		Issuer:                     principal.Issuer,
		Subject:                    principal.Subject,
		CustomerAccountID:          principal.CustomerAccountID,
		InstallationID:             principal.InstallationID,
		AccessEvaluationID:         principal.AccessEvaluationID,
		AccessEvaluatedAt:          principal.AccessEvaluatedAt.UTC().Format(time.RFC3339Nano),
		IdempotencyKey:             idempotencyKey,
		Arguments:                  arguments,
	})
	if err != nil {
		return nil, errors.New("managed tool arguments are not canonical JSON")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(managedToolConfirmationDomain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return digest.Sum(nil), nil
}

func randomManagedToolConfirmationUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func randomManagedToolConfirmationNonce() (string, []byte, error) {
	raw := make([]byte, managedToolConfirmationNonceBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	nonce := "mtc_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(nonce))
	return nonce, digest[:], nil
}

func managedToolConfirmationActor(principal identity.Principal) (string, error) {
	actorID := vendorActorID(principal)
	if actorID == "" || strings.TrimSpace(principal.AccessEvaluationID) == "" || principal.AccessEvaluatedAt.IsZero() {
		return "", errors.New("managed tool confirmation requires an exact authenticated access evaluation")
	}
	return actorID, nil
}

func (s *Server) issueManagedToolConfirmation(ctx context.Context, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) (managedToolConfirmationChallenge, error) {
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	nonce, nonceDigest, err := randomManagedToolConfirmationNonce()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	id, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	expiresAt := now.Add(managedToolConfirmationTTL)
	decisionExpiresAt := principal.AccessEvaluatedAt.Add(time.Duration(binding.AuthorizationPoint.DecisionTTLSeconds) * time.Second)
	if decisionExpiresAt.Before(expiresAt) {
		expiresAt = decisionExpiresAt
	}
	if !now.Before(expiresAt) {
		return managedToolConfirmationChallenge{}, errors.New("the access evaluation expires before confirmation can be issued")
	}
	confirmation := model.ToolTestConfirmation{
		ID:             id,
		OrganisationID: tool.OrganisationID,
		ProductID:      productID,
		ToolID:         tool.ID,
		ToolRevision:   tool.Revision,
		ArgumentHash:   argumentHash,
		NonceDigest:    nonceDigest,
		ActorID:        actorID,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
	}
	if err := s.service.Store().CreateToolTestConfirmation(ctx, confirmation); err != nil {
		return managedToolConfirmationChallenge{}, err
	}
	return managedToolConfirmationChallenge{Nonce: nonce, ExpiresAt: expiresAt}, nil
}

func (s *Server) consumeManagedToolConfirmation(ctx context.Context, challenge, productID string, tool model.Tool, binding toolruntime.BoundAuthorization, principal identity.Principal, arguments map[string]any, idempotencyKey string, now time.Time) error {
	if len(challenge) != len("mtc_")+base64.RawURLEncoding.EncodedLen(managedToolConfirmationNonceBytes) || !strings.HasPrefix(challenge, "mtc_") {
		return errors.New("managed tool confirmation challenge is malformed")
	}
	actorID, err := managedToolConfirmationActor(principal)
	if err != nil {
		return err
	}
	argumentHash, err := managedToolConfirmationArgumentHash(productID, tool, binding, principal, arguments, idempotencyKey)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(challenge))
	consumptionID, err := randomManagedToolConfirmationUUID()
	if err != nil {
		return err
	}
	_, err = s.service.Store().ConsumeToolTestConfirmation(ctx, digest[:], productID, tool.ID, tool.Revision, argumentHash, actorID, consumptionID, now)
	return err
}

func managedToolPolicy(tool model.Tool, binding toolruntime.BoundAuthorization) (confirmationRequired, idempotencyRequired bool, err error) {
	var policy struct {
		ConfirmationRequired bool `json:"confirmation_required"`
		IdempotencyRequired  bool `json:"idempotency_required"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return false, false, err
	}
	return policy.ConfirmationRequired || binding.AuthorizationPoint.ConfirmationRequired, strings.ToUpper(strings.TrimSpace(tool.HTTPMethod)) != http.MethodGet && policy.IdempotencyRequired, nil
}

func writeManagedToolConfirmationRequired(w http.ResponseWriter, id any, challenge managedToolConfirmationChallenge, tool model.Tool, binding toolruntime.BoundAuthorization) {
	writeRPCErrorData(w, id, -32003, "Client confirmation attestation is required for this exact managed tool invocation", map[string]any{
		"confirmation_required":          true,
		"confirmation_challenge":         challenge.Nonce,
		"expires_at":                     challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"retry_metadata_field":           "params._meta." + managedToolConfirmationMetaField,
		"confirmation_attestation_field": "params._meta.confirmed",
		"confirmation_attestation_value": true,
		"tool_id":                        tool.ID,
		"tool_revision":                  tool.Revision,
		"authorization_point_id":         binding.AuthorizationPoint.ID,
		"authorization_point_revision":   binding.AuthorizationPointRevision,
		"notice":                         "Retrying with the challenge and confirmed=true is the client's attestation that it obtained confirmation; the server does not independently prove that a human approved.",
	})
}

func (s *Server) callTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, productID string, public bool, productManifest model.ProductManifest, manifestErr error) {
	params, err := decodeToolCallParams(request.Params)
	if err != nil {
		writeRPCError(w, request.ID, -32602, "Invalid params")
		return
	}
	principal, _ := ctx.Value(principalKey).(identity.Principal)
	actorKind, actorID, channel := "anonymous", "", "public_mcp"
	if !public {
		actorKind, actorID, channel = "vendor_user", pseudonym(productID, principal), "private_mcp"
	}
	selection := model.ProductSelectionContext{Public: public}
	if !public {
		selection.CustomerAccountID, selection.InstallationID = principal.CustomerAccountID, principal.InstallationID
	}
	dimensions := map[string]any{"channel": channel, "tool": params.Name}
	if manifestErr == nil {
		dimensions["catalog_revision"], dimensions["selection_source"] = productManifest.CatalogRevision, productManifest.SelectionSource
		dimensions["environment_id"], dimensions["installation_id"] = productManifest.EnvironmentID, productManifest.InstallationID
		if productManifest.EffectiveVersion != nil {
			dimensions["product_version_id"], dimensions["product_version"], dimensions["manifest_hash"] = productManifest.EffectiveVersion.ID, productManifest.EffectiveVersion.Version, productManifest.ManifestHash
		}
	}
	s.recordAnalytics(ctx, productID, "tool.called", actorKind, actorID, dimensions)
	if manifestErr == nil && len(productManifest.Integrations) > 0 {
		_, generatedBindings := s.apiDefaultToolDefinitions(ctx, productID, productManifest, principal, public)
		if binding, ok := generatedBindings[params.Name]; ok {
			s.executeAPIDefaultTool(ctx, w, request, params, productID, binding, public, productManifest, selection, principal)
			return
		}
	}
	switch params.Name {
	case "integration.recipes.list":
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipes could not be listed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipes": values})
	case "integration.plan":
		outcome, _ := params.Arguments["outcome"].(string)
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil || strings.TrimSpace(outcome) == "" {
			writeRPCError(w, request.ID, -32602, "A valid integration outcome is required")
			return
		}
		var selected *model.Recipe
		needle := strings.ToLower(strings.TrimSpace(outcome))
		for index := range values {
			candidate := strings.ToLower(values[index].Title + " " + values[index].Outcome)
			if selected == nil || strings.Contains(candidate, needle) || strings.Contains(needle, strings.ToLower(values[index].Slug)) {
				copy := values[index]
				selected = &copy
				if strings.Contains(candidate, needle) {
					break
				}
			}
		}
		if selected == nil {
			writeRPCError(w, request.ID, -32004, "No published recipe matches this outcome")
			return
		}
		s.recordAnalytics(ctx, productID, "recipe.plan_selected", actorKind, actorID, map[string]any{"recipe_id": selected.ID, "recipe_slug": selected.Slug, "channel": channel})
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": selected.StableURI, "title": selected.Title, "outcome": selected.Outcome, "revision_id": selected.CurrentRevisionID, "next_step": "Read the recipe resource, verify its prerequisites, then implement and validate each step."})
	case "integration.check":
		recipeURI, _ := params.Arguments["recipe_uri"].(string)
		recipe, err := s.publishedRecipeByURI(ctx, productID, recipeURI, public)
		if err != nil {
			writeRPCError(w, request.ID, -32004, "Recipe resource not found")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": recipe.StableURI, "state": recipe.State, "current": recipe.State == "published" && !recipe.NeedsAttention, "needs_attention": recipe.NeedsAttention, "revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt})
	case "deployment.get_manifest", "product.get_manifest":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		writeToolResult(w, request.ID, productManifest)
	case "deployment.releases.list", "product.versions.list":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Connector release discovery failed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"product_id": productManifest.ProductID, "catalog_revision": productManifest.CatalogRevision, "manifest_hash": productManifest.ManifestHash, "effective_version": productManifest.EffectiveVersion, "selection_source": productManifest.SelectionSource, "available_versions": productManifest.AvailableVersions, "operational_warnings": productManifest.OperationalWarnings})
	case "support.report_bug":
		if public || s.reporting == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		if !params.Meta.Confirmed {
			writeRPCError(w, request.ID, -32003, "Explicit user confirmation is required after previewing the exact bug report")
			return
		}
		if !s.allowFixedWindow("support-reporting|"+productID+"|"+vendorActorID(principal), 30, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeRPCError(w, request.ID, -32029, "Support reporting request limit exceeded")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Trusted product context is unavailable")
			return
		}
		var input reporting.BugInput
		if err := decodeArguments(params.Arguments, &input); err != nil {
			writeRPCError(w, request.ID, -32602, "Bug report arguments are invalid")
			return
		}
		integration, err := s.reportIntegrationContext(ctx, productID, input.IntegrationID)
		if err != nil {
			writeRPCError(w, request.ID, -32602, "The selected Integration is not available in this deployment")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.reporting.SubmitBug(ctx, input, reporting.SubmitContext{Principal: principal, ActorPseudonym: actorID, Product: reportProductContext(productManifest), Integration: integration, RequestID: requestID})
		if err != nil {
			reportingRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "support.bug_reported", "vendor_user", actorID, map[string]any{"channel": "private_mcp", "state": value.State})
		writeToolResult(w, request.ID, reportToolResult(value))
	case "support.submit_feedback":
		if public || s.reporting == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		if !params.Meta.Confirmed {
			writeRPCError(w, request.ID, -32003, "Explicit user confirmation is required after previewing the exact feedback")
			return
		}
		if !s.allowFixedWindow("support-reporting|"+productID+"|"+vendorActorID(principal), 30, time.Now().UTC()) {
			w.Header().Set("Retry-After", "60")
			writeRPCError(w, request.ID, -32029, "Support reporting request limit exceeded")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Trusted product context is unavailable")
			return
		}
		var input reporting.FeedbackInput
		if err := decodeArguments(params.Arguments, &input); err != nil {
			writeRPCError(w, request.ID, -32602, "Feedback arguments are invalid")
			return
		}
		integration, err := s.reportIntegrationContext(ctx, productID, input.IntegrationID)
		if err != nil {
			writeRPCError(w, request.ID, -32602, "The selected Integration is not available in this deployment")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.reporting.SubmitFeedback(ctx, input, reporting.SubmitContext{Principal: principal, ActorPseudonym: actorID, Product: reportProductContext(productManifest), Integration: integration, RequestID: requestID})
		if err != nil {
			reportingRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "support.feedback_submitted", "vendor_user", actorID, map[string]any{"channel": "private_mcp", "state": value.State})
		writeToolResult(w, request.ID, reportToolResult(value))
	case "mcp_connections.authorize":
		if public || s.mcpBridge == nil {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		requestID, _ := ctx.Value(requestIDKey).(string)
		authorizationURL, err := s.mcpBridge.BeginAuthorization(ctx, productID, connectionID, toolPrincipal(principal, false, requestID, ""))
		if err != nil {
			writeRPCError(w, request.ID, -32003, "Upstream user authorization could not be started")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"authorization_url": authorizationURL, "expires_in_seconds": 600, "connection_id": connectionID})
	case "integration_runs.start":
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			EnvironmentID    string `json:"environment_id"`
			RequestedOutcome string `json:"requested_outcome"`
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.service.StartIntegrationRun(ctx, productID, input.EnvironmentID, input.RequestedOutcome, platform.Actor{ID: vendorActorID(principal), RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32602, "Integration run could not be started")
			return
		}
		writeToolResult(w, request.ID, value)
	case "integration_runs.complete":
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			RunID            string `json:"run_id"`
			ReportedSuccess  *bool  `json:"reported_success"`
			ValidatedSuccess *bool  `json:"validated_success"`
			FailureCode      string `json:"failure_code"`
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		requestID, _ := ctx.Value(requestIDKey).(string)
		value, err := s.service.CompleteIntegrationRun(ctx, productID, input.RunID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, platform.Actor{ID: vendorActorID(principal), RequestID: requestID})
		if err != nil {
			writeRPCError(w, request.ID, -32602, "Integration run could not be completed")
			return
		}
		writeToolResult(w, request.ID, value)
	case "access.instances.list":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		values, err := s.accessRuntime.ListInstances(ctx, productID, connectionID, integrationID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"instances": values})
	case "access.instances.create":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ConnectionID string `json:"connection_id"`
			accessruntime.InstanceRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.accessRuntime.CreateInstance(ctx, productID, input.ConnectionID, input.InstanceRequest, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "access_instance.created", "vendor_user", pseudonym(productID, principal), map[string]any{"connection_id": input.ConnectionID, "integration_id": input.IntegrationID})
		writeToolResult(w, request.ID, value)
	case "access.credentials.list":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		connectionID, _ := params.Arguments["connection_id"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		instanceID, _ := params.Arguments["access_instance_id"].(string)
		values, err := s.accessRuntime.ListCredentials(ctx, productID, connectionID, integrationID, instanceID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, map[string]any{"credentials": values})
	case "access.credentials.create":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		var input struct {
			ConnectionID string `json:"connection_id"`
			accessruntime.CredentialRequest
		}
		if decodeArguments(params.Arguments, &input) != nil {
			writeRPCError(w, request.ID, -32602, "Invalid params")
			return
		}
		value, err := s.accessRuntime.IssueCredential(ctx, productID, input.ConnectionID, input.CredentialRequest, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		s.recordAnalytics(ctx, productID, "access_credential.created", "vendor_user", pseudonym(productID, principal), map[string]any{"connection_id": input.ConnectionID, "integration_id": input.IntegrationID, "existing": value.Existing})
		writeToolResult(w, request.ID, value)
	case "access.credentials.revoke":
		if public || s.accessRuntime == nil || len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available")
			return
		}
		credentialID, _ := params.Arguments["credential_id"].(string)
		value, err := s.accessRuntime.RevokeCredential(ctx, productID, credentialID, accessPrincipal(principal, ctx))
		if err != nil {
			accessRPCError(w, request.ID, err)
			return
		}
		writeToolResult(w, request.ID, value)
	case "search_knowledge":
		if len(productManifest.Integrations) > 0 {
			writeRPCError(w, request.ID, -32601, "Tool is not available for Integration-catalog deployments")
			return
		}
		query, _ := params.Arguments["query"].(string)
		integrationID, _ := params.Arguments["integration_id"].(string)
		publicationIDs, scopeErr := s.knowledgePublicationIDs(ctx, productID, integrationID, productManifest)
		if scopeErr != nil {
			writeRPCError(w, request.ID, -32003, "Select exactly one published Integration with reviewed documentation")
			return
		}
		if public {
			items, err := s.service.Store().PublicKnowledge(ctx, productID, publicationIDs, query)
			if err != nil {
				writeRPCError(w, request.ID, -32603, "Search failed")
				return
			}
			filtered := make([]model.KnowledgeRecord, 0, len(items))
			for _, item := range items {
				allowed := false
				for _, kind := range []string{"docs", "openapi", "git"} {
					managed, candidateAllowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, kind, item.SourceID, "", "")
					if allowErr == nil && (!managed || candidateAllowed) {
						allowed = true
						break
					}
				}
				if allowed {
					filtered = append(filtered, item)
				}
			}
			writeToolResult(w, request.ID, filtered)
			return
		}
		if principal.ProductID != productID {
			writeRPCError(w, request.ID, -32003, "Knowledge access was denied by product policy")
			return
		}
		items, err := s.service.Store().PrivateKnowledge(ctx, productID, publicationIDs, query)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Search failed")
			return
		}
		filtered := make([]model.KnowledgeRecord, 0, len(items))
		for _, item := range items {
			allowed := false
			for _, kind := range []string{"docs", "openapi", "git"} {
				managed, candidateAllowed, allowErr := s.service.ProductVersionAllowsArtifactFor(ctx, productID, selection, kind, item.SourceID, "", "")
				if allowErr == nil && (!managed || candidateAllowed) {
					allowed = true
					break
				}
			}
			if allowed {
				filtered = append(filtered, item)
			}
		}
		writeToolResult(w, request.ID, filtered)
	default:
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available on Public MCP")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32003, "Tool execution requires a resolved deployment and customer selection context")
			return
		}
		if s.toolRuntime != nil {
			requestID, _ := ctx.Value(requestIDKey).(string)
			principal, _ := ctx.Value(principalKey).(identity.Principal)
			selectedTool, lookupErr := s.executableTool(ctx, productID, params.Name, selection)
			if errors.Is(lookupErr, errToolVersionExcluded) {
				writeRPCError(w, request.ID, -32003, "Tool is not included in the effective product version")
				return
			}
			if lookupErr != nil || selectedTool.ID == "" {
				writeRPCError(w, request.ID, -32601, "Tool not found")
				return
			}
			var value any
			var err error
			bound, managedByIntegration, bindingErr := s.integrationToolAuthorization(ctx, productManifest, selectedTool)
			if managedByIntegration {
				if bindingErr != nil {
					writeRPCError(w, request.ID, -32003, "Tool has no unique exact authorization action in an applicable published Integration revision")
					return
				}
				confirmationRequired, idempotencyRequired, policyErr := managedToolPolicy(selectedTool, bound)
				if policyErr != nil {
					writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
					return
				}
				if (params.Meta.IdempotencyKey != "" && !toolruntime.ValidIdempotencyKey(params.Meta.IdempotencyKey)) || (idempotencyRequired && !toolruntime.ValidIdempotencyKey(params.Meta.IdempotencyKey)) {
					writeRPCError(w, request.ID, -32602, "params._meta.idempotency_key must contain 16 to 200 visible ASCII characters")
					return
				}
				managedPrincipal := toolPrincipal(principal, false, requestID, params.Meta.IdempotencyKey)
				managedPrincipal.EnvironmentID = productManifest.EnvironmentID
				if confirmationRequired {
					if params.Arguments == nil {
						params.Arguments = map[string]any{}
					}
					if validationErr := toolruntime.ValidateArguments(selectedTool.InputSchema, params.Arguments); validationErr != nil {
						writeRPCError(w, request.ID, -32602, "Tool arguments do not match the declared input schema")
						return
					}
					available, availabilityErr := s.toolRuntime.AvailableBound(ctx, productID, []toolruntime.BoundAuthorization{bound}, managedPrincipal)
					if availabilityErr != nil || len(available) != 1 || available[0].ID != selectedTool.ID || available[0].Revision != selectedTool.Revision {
						writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
						return
					}
					now := time.Now().UTC()
					if strings.TrimSpace(params.Meta.ConfirmationChallenge) == "" {
						challenge, challengeErr := s.issueManagedToolConfirmation(ctx, productID, selectedTool, bound, principal, params.Arguments, params.Meta.IdempotencyKey, now)
						if challengeErr != nil {
							writeRPCError(w, request.ID, -32603, "A confirmation challenge could not be issued safely")
							return
						}
						writeManagedToolConfirmationRequired(w, request.ID, challenge, selectedTool, bound)
						return
					}
					if !params.Meta.Confirmed {
						writeRPCErrorData(w, request.ID, -32003, "Client confirmation attestation is required with the server-issued challenge", map[string]any{
							"confirmation_required":          true,
							"confirmation_attestation_field": "params._meta.confirmed",
							"confirmation_attestation_value": true,
							"notice":                         "confirmed=true is a client attestation; the server does not independently prove that a human approved.",
						})
						return
					}
					if confirmationErr := s.consumeManagedToolConfirmation(ctx, params.Meta.ConfirmationChallenge, productID, selectedTool, bound, principal, params.Arguments, params.Meta.IdempotencyKey, now); confirmationErr != nil {
						writeRPCErrorData(w, request.ID, -32003, "The confirmation challenge is invalid, expired, already used, or does not match this exact invocation", map[string]any{
							"confirmation_required":                        true,
							"retry_without_challenge_to_request_a_new_one": true,
						})
						return
					}
					managedPrincipal.Confirmed = true
				}
				runtimeName := selectedTool.Namespace + "." + selectedTool.Name
				value, err = s.toolRuntime.ExecuteBound(ctx, productID, runtimeName, params.Arguments, managedPrincipal, bound)
			} else {
				runtimePrincipal := toolPrincipal(principal, params.Meta.Confirmed, requestID, params.Meta.IdempotencyKey)
				runtimePrincipal.EnvironmentID = productManifest.EnvironmentID
				runtimeName := selectedTool.Namespace + "." + selectedTool.Name
				value, err = s.toolRuntime.Execute(ctx, productID, runtimeName, params.Arguments, runtimePrincipal)
			}
			if err == nil {
				if upstream, ok := value.(toolruntime.MCPCallResult); ok {
					writeRPC(w, request.ID, upstream.Result)
					return
				}
				writeToolResult(w, request.ID, value)
				return
			}
			if errors.Is(err, toolruntime.ErrDenied) || errors.Is(err, toolruntime.ErrConfirmation) {
				writeRPCError(w, request.ID, -32003, "Tool execution was denied by policy")
				return
			}
			if errors.Is(err, toolruntime.ErrInvalidIdempotencyKey) {
				writeRPCError(w, request.ID, -32602, "params._meta.idempotency_key must contain 16 to 200 visible ASCII characters")
				return
			}
			if errors.Is(err, toolruntime.ErrRateLimited) {
				writeRPCError(w, request.ID, -32029, "The upstream tool connection request limit was exceeded")
				return
			}
			if errors.Is(err, mcpbridge.ErrGrantRequired) {
				writeRPCError(w, request.ID, -32001, "Authorize this Stateless MCPv2 connection with mcp_connections.authorize before calling its tools")
				return
			}
			writeRPCError(w, request.ID, -32603, "Tool execution failed safely; review the tool activity for the sanitized failure category")
			return
		}
		writeRPCError(w, request.ID, -32601, "Tool not found")
	}
}
