package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"net/http"
)

func (s *Server) mcpConnections(w http.ResponseWriter, r *http.Request, productID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().MCPConnections(r.Context(), productID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values, "protocol_policy": "Stateless MCPv2 Only", "protocol_version": model.StatelessMCPv2Protocol, "specification_url": "https://blog.modelcontextprotocol.io/posts/2026-07-28/"})
	case http.MethodPost:
		var input struct {
			OrganisationID      string `json:"organisation_id"`
			Name                string `json:"name"`
			Namespace           string `json:"namespace"`
			Endpoint            string `json:"endpoint"`
			AccessToken         string `json:"access_token"`
			ForwardUserIdentity bool   `json:"forward_user_identity"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.mcpBridge.CreateConnection(r.Context(), mcpbridge.ConnectionInput{OrganisationID: input.OrganisationID, ProductID: productID, Name: input.Name, Namespace: input.Namespace, Endpoint: input.Endpoint, AccessToken: input.AccessToken, ForwardUserIdentity: input.ForwardUserIdentity}, mcpbridge.Actor{ID: actor(r).ID, RequestID: actor(r).RequestID})
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) inspectMCPConnection(w http.ResponseWriter, r *http.Request, productID, connectionID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	value, err := s.mcpBridge.Inspect(r.Context(), productID, connectionID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_upstream_inspection_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) importMCPConnection(w http.ResponseWriter, r *http.Request, productID, connectionID string) {
	if s.mcpBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp_bridge_unavailable", "Stateless MCPv2 bridge is unavailable.", nil)
		return
	}
	var input struct {
		ToolNames            []string `json:"tool_names"`
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
		TimeoutMS            int      `json:"timeout_ms"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	adminActor := actor(r)
	value, err := s.mcpBridge.Import(r.Context(), productID, connectionID, mcpbridge.ImportInput{ToolNames: input.ToolNames, RequiredGrants: input.RequiredGrants, ConfirmationRequired: input.ConfirmationRequired, TimeoutMS: input.TimeoutMS}, mcpbridge.Actor{ID: adminActor.ID, RequestID: adminActor.RequestID})
	if err != nil {
		writeError(w, http.StatusBadGateway, "mcp_upstream_import_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) llmProfiles(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().LLMProfiles(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPut:
		var input struct {
			OrganisationID      string `json:"organisation_id"`
			Role                string `json:"role"`
			Provider            string `json:"provider"`
			Endpoint            string `json:"endpoint"`
			Model               string `json:"model"`
			Credential          string `json:"credential"`
			EmbeddingDimensions int    `json:"embedding_dimensions"`
			MaxInputTokens      int    `json:"max_input_tokens"`
			MaxOutputTokens     int    `json:"max_output_tokens"`
			DailyTokenBudget    int64  `json:"daily_token_budget"`
			Enabled             bool   `json:"enabled"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveLLMProfile(r.Context(), platform.LLMProfileInput{OrganisationID: input.OrganisationID, ProductID: productID, Role: input.Role, Provider: input.Provider, Endpoint: input.Endpoint, Model: input.Model, Credential: input.Credential, EmbeddingDimensions: input.EmbeddingDimensions, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Enabled: input.Enabled}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func pseudonym(productID string, principal identity.Principal) string {
	if principal.Subject == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(productID + "\x00" + vendorActorID(principal)))
	return hex.EncodeToString(digest[:16])
}

func vendorActorID(principal identity.Principal) string {
	if principal.Subject == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(principal.Issuer)) + "." + base64.RawURLEncoding.EncodeToString([]byte(principal.Subject))
}

func (s *Server) adminAPI(w http.ResponseWriter, r *http.Request) {
	actorID, cookieSession, ok := s.authenticateAdmin(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required", "Use an administrator access token.", nil)
		return
	}
	if cookieSession && mutating(r.Method) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || s.auth == nil || s.auth.VerifyCSRF(r.Context(), cookie.Value, r.Header.Get("X-CSRF-Token")) != nil {
			writeError(w, http.StatusForbidden, "csrf_validation_failed", "A valid CSRF token is required.", nil)
			return
		}
		if !s.validOrigin(r) {
			writeError(w, http.StatusForbidden, "origin_validation_failed", "The request origin is not allowed.", nil)
			return
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), actorIDKey, actorID))
	parts := splitPath(r.URL.Path)
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "v1" {
		writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
		return
	}
	switch {
	case len(parts) == 4 && parts[2] == "root" && parts[3] == "users":
		s.rootUsers(w, r)
	case len(parts) == 4 && parts[2] == "system" && parts[3] == "doctor" && r.Method == http.MethodGet:
		s.systemDoctor(w, r)
	case len(parts) == 4 && parts[2] == "ai" && parts[3] == "connections":
		s.aiProviderConnections(w, r)
	case len(parts) == 6 && parts[2] == "ai" && parts[3] == "connections" && parts[5] == "test" && r.Method == http.MethodPost:
		s.testAIProviderConnection(w, r, parts[4])
	case len(parts) == 5 && parts[2] == "root" && parts[3] == "users" && r.Method == http.MethodDelete:
		s.revokeRootUser(w, r, parts[4])
	case len(parts) == 3 && parts[2] == "organisations":
		s.organisations(w, r)
	case len(parts) == 3 && parts[2] == "deployment":
		s.deployment(w, r)
	case len(parts) == 3 && parts[2] == "native-plugins":
		s.nativePluginStatuses(w, r)
	case len(parts) == 5 && parts[2] == "native-plugins" && parts[4] == "state" && r.Method == http.MethodPatch:
		s.nativePluginState(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "environments":
		s.deploymentEnvironments(w, r)
	case len(parts) == 3 && parts[2] == "integrations":
		s.integrations(w, r)
	case len(parts) == 4 && parts[2] == "integrations":
		s.integration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "publish" && r.Method == http.MethodPost:
		s.publishIntegration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "preflight" && r.Method == http.MethodPost:
		s.preflightIntegration(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-setup":
		s.integrationRuntimeSetup(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-connections":
		s.integrationRuntimeConnections(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "runtime-credential-sets" && r.Method == http.MethodPost:
		s.createIntegrationRuntimeCredentialSet(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "runtime-credential-sets" && r.Method == http.MethodGet:
		s.runtimeCredentialSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-credential-sets" && parts[4] == "usage" && r.Method == http.MethodGet:
		s.runtimeCredentialUsage(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-credential-sets" && parts[4] == "rotate" && r.Method == http.MethodPost:
		s.rotateRuntimeCredential(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "runtime-service-connections" && parts[4] == "check" && r.Method == http.MethodPost:
		s.checkRuntimeServiceConnection(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "runtime-credential-sets" && parts[4] == "versions" && parts[6] == "revoke" && r.Method == http.MethodPost:
		s.revokeRuntimeCredential(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "authorization-points":
		s.authorizationPoints(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "authorization-points":
		s.authorizationPoint(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "tools":
		s.integrationTools(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "resource-sets" && r.Method == http.MethodPost:
		s.attachResourceSet(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "resource-sets" && r.Method == http.MethodDelete:
		s.detachResourceSet(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "integrations" && parts[4] == "sdks":
		s.integrationSDKs(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "integrations" && parts[4] == "sdks":
		s.integrationSDK(w, r, parts[3], parts[5])
	case len(parts) == 3 && parts[2] == "resource-sets":
		s.resourceSets(w, r)
	case len(parts) == 3 && parts[2] == "grant-definitions":
		s.grantDefinitions(w, r)
	case len(parts) == 4 && parts[2] == "grant-definitions":
		s.grantDefinition(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "resource-sets":
		s.resourceSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "resource-sets" && parts[4] == "duplicate" && r.Method == http.MethodPost:
		s.duplicateResourceSet(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "resource-sets" && parts[4] == "revisions" && r.Method == http.MethodGet:
		s.resourceSetRevisions(w, r, parts[3])
	case len(parts) == 3 && parts[2] == "support-submissions" && r.Method == http.MethodGet:
		s.supportSubmissions(w, r)
	case len(parts) == 4 && parts[2] == "support-submissions" && r.Method == http.MethodGet:
		s.supportSubmission(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "organisations" && parts[4] == "products":
		s.products(w, r, parts[3])
	case len(parts) == 4 && parts[2] == "products" && r.Method == http.MethodPatch:
		s.productSettings(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "description" && parts[5] == "rewrite" && r.Method == http.MethodPost:
		s.rewriteProductDescription(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "customer-accounts" && r.Method == http.MethodGet:
		s.customerAccounts(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "customer-accounts" && r.Method == http.MethodPatch:
		s.customerAccount(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "organisations" && parts[4] == "audit" && r.Method == http.MethodGet:
		s.auditEvents(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "environments":
		s.environments(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "distribution":
		s.distribution(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "sources":
		s.sources(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "sources" && parts[5] == "upload":
		s.uploadSource(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "visibility" && r.Method == http.MethodPatch:
		s.sourceVisibility(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawl" && r.Method == http.MethodPost:
		s.queueCrawl(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishSource(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "review" && r.Method == http.MethodGet:
		s.sourceReview(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "publications" && r.Method == http.MethodGet:
		s.sourcePublications(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "sources" && parts[6] == "crawls" && r.Method == http.MethodGet:
		s.crawlJobs(w, r, parts[3], parts[5])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "tool-builder":
		s.toolBuilder(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "tools":
		s.tools(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "tools":
		s.toolEditorResource(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "clone" && r.Method == http.MethodPost:
		s.cloneTool(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "dry-run" && r.Method == http.MethodPost:
		s.dryRunTool(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-confirmations" && r.Method == http.MethodPost:
		s.createToolTestConfirmation(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs":
		s.toolTestRuns(w, r, parts[3], parts[5])
	case len(parts) == 9 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs" && parts[8] == "analyse":
		s.analyseToolTestRun(w, r, parts[3], parts[5], parts[7])
	case len(parts) == 8 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "test-runs" && r.Method == http.MethodGet:
		s.toolTestRun(w, r, parts[3], parts[5], parts[7])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "retire" && r.Method == http.MethodPost:
		s.retireTool(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "mcp-connections":
		s.mcpConnections(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "inspect" && r.Method == http.MethodPost:
		s.inspectMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "mcp-connections" && parts[6] == "import" && r.Method == http.MethodPost:
		s.importMCPConnection(w, r, parts[3], parts[5])
	case len(parts) == 3 && parts[2] == "identity-provider":
		s.identityProvider(w, r)
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "tests":
		s.identityProviderTests(w, r)
	case len(parts) == 5 && parts[2] == "identity-provider" && parts[3] == "tests":
		s.identityProviderTest(w, r, parts[4])
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "activate":
		s.activateIdentityProvider(w, r)
	case len(parts) == 4 && parts[2] == "identity-provider" && parts[3] == "disable":
		s.disableIdentityProvider(w, r)
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "llm-profiles":
		s.llmProfiles(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "ai-profiles" && r.Method == http.MethodGet:
		s.aiWorkloadProfiles(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "ai-profiles" && r.Method == http.MethodPut:
		s.aiWorkloadProfile(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "analyses":
		s.integrationAnalyses(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "analyses":
		s.integrationAnalysis(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "analyses" && parts[6] == "recipes" && r.Method == http.MethodPost:
		s.generateRecipes(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "recipes":
		s.recipes(w, r, parts[3])
	case len(parts) == 6 && parts[2] == "products" && parts[4] == "recipes":
		s.recipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "rework" && r.Method == http.MethodPost:
		s.reworkRecipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "approve" && r.Method == http.MethodPost:
		s.approveRecipe(w, r, parts[3], parts[5])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "recipes" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishRecipe(w, r, parts[3], parts[5])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "attention" && r.Method == http.MethodGet:
		s.attention(w, r, parts[3])
	case len(parts) == 5 && parts[2] == "products" && parts[4] == "ai-usage" && r.Method == http.MethodGet:
		s.aiUsage(w, r, parts[3])
	case len(parts) == 7 && parts[2] == "products" && parts[4] == "tools" && parts[6] == "publish" && r.Method == http.MethodPost:
		s.publishTool(w, r, parts[3], parts[5])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Route not found.", nil)
	}
}
