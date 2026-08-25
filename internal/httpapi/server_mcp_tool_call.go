package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

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
