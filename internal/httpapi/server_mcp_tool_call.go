package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type publishedRecipeSummary struct {
	URI             string     `json:"uri"`
	Slug            string     `json:"slug"`
	Title           string     `json:"title"`
	Outcome         string     `json:"outcome"`
	IntegrationID   string     `json:"integration_id"`
	ContractVersion string     `json:"contract_version"`
	RevisionID      string     `json:"revision_id"`
	PublishedAt     *time.Time `json:"published_at"`
}

const maxRecipeSelectionCandidates = 25

func recipeSummary(value model.Recipe) publishedRecipeSummary {
	return publishedRecipeSummary{
		URI:             value.StableURI,
		Slug:            value.Slug,
		Title:           value.Title,
		Outcome:         value.Outcome,
		IntegrationID:   value.IntegrationID,
		ContractVersion: value.ContractVersion,
		RevisionID:      value.CurrentRevisionID,
		PublishedAt:     value.PublishedAt,
	}
}

func sortedRecipeSummaries(values []model.Recipe) []publishedRecipeSummary {
	result := make([]publishedRecipeSummary, 0, len(values))
	for _, value := range values {
		result = append(result, recipeSummary(value))
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Title), strings.ToLower(result[j].Title)
		if left != right {
			return left < right
		}
		return result[i].URI < result[j].URI
	})
	return result
}

func boundedRecipeSummaries(values []model.Recipe) ([]publishedRecipeSummary, bool) {
	result := sortedRecipeSummaries(values)
	if len(result) <= maxRecipeSelectionCandidates {
		return result, false
	}
	return result[:maxRecipeSelectionCandidates], true
}

func recipeSummaryOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"uri":              map[string]any{"type": "string"},
			"slug":             map[string]any{"type": "string"},
			"title":            map[string]any{"type": "string"},
			"outcome":          map[string]any{"type": "string"},
			"integration_id":   map[string]any{"type": "string"},
			"contract_version": map[string]any{"type": "string", "const": model.RecipeContractProductIntegrationV2},
			"revision_id":      map[string]any{"type": "string"},
			"published_at":     map[string]any{"type": "string", "format": "date-time"},
		},
		"required": []string{"uri", "slug", "title", "outcome", "integration_id", "contract_version", "revision_id", "published_at"},
	}
}

func recipeListOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{"recipes": map[string]any{"type": "array", "items": recipeSummaryOutputSchema()}},
		"required":             []string{"recipes"},
	}
}

func recipePlanOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"recipe_uri":       map[string]any{"type": "string"},
			"title":            map[string]any{"type": "string"},
			"outcome":          map[string]any{"type": "string"},
			"integration_id":   map[string]any{"type": "string"},
			"contract_version": map[string]any{"type": "string", "const": model.RecipeContractProductIntegrationV2},
			"revision_id":      map[string]any{"type": "string"},
			"next_step":        map[string]any{"type": "string"},
		},
		"required": []string{"recipe_uri", "title", "outcome", "integration_id", "contract_version", "revision_id", "next_step"},
	}
}

func recipeCheckOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"recipe_uri":       map[string]any{"type": "string"},
			"integration_id":   map[string]any{"type": "string"},
			"contract_version": map[string]any{"type": "string", "const": model.RecipeContractProductIntegrationV2},
			"state":            map[string]any{"type": "string", "const": "published"},
			"current":          map[string]any{"type": "boolean", "const": true},
			"needs_attention":  map[string]any{"type": "boolean", "const": false},
			"revision_id":      map[string]any{"type": "string"},
			"published_at":     map[string]any{"type": "string", "format": "date-time"},
		},
		"required": []string{"recipe_uri", "integration_id", "contract_version", "state", "current", "needs_attention", "revision_id", "published_at"},
	}
}

func normalizeRecipeLookup(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(parts, " ")
}

// exactRecipeMatch deliberately does not rank or guess. A coding agent may
// select a recipe only when its request exactly identifies one published
// title, slug, or outcome after harmless punctuation and whitespace folding.
func exactRecipeMatch(values []model.Recipe, requested string) (*model.Recipe, []model.Recipe) {
	needle := normalizeRecipeLookup(requested)
	if needle == "" {
		return nil, nil
	}
	matches := make([]model.Recipe, 0, 1)
	for _, value := range values {
		if needle == normalizeRecipeLookup(value.Title) || needle == normalizeRecipeLookup(value.Slug) || needle == normalizeRecipeLookup(value.Outcome) {
			matches = append(matches, value)
		}
	}
	if len(matches) != 1 {
		return nil, matches
	}
	selected := matches[0]
	return &selected, matches
}

func exactRecipeToolStringArgument(arguments map[string]any, key string) (string, bool) {
	if len(arguments) != 1 {
		return "", false
	}
	value, ok := arguments[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" || len([]rune(value)) > 500 {
		return "", false
	}
	return value, true
}

func (s *Server) callTool(ctx context.Context, w http.ResponseWriter, request rpcRequest, productID string, public bool, productManifest model.ProductManifest, manifestErr error) {
	params, err := decodeToolCallParams(request.Params)
	if err != nil {
		writeRPCError(w, request.ID, -32602, "Invalid params")
		return
	}
	principal, _ := ctx.Value(principalKey).(identity.Principal)
	actorID := ""
	if !public {
		actorID = pseudonym(productID, principal)
	}
	scope := model.CatalogScope{Public: public}
	if manifestErr == nil && len(productManifest.Integrations) > 0 {
		_, generatedBindings := s.apiDefaultToolDefinitions(ctx, productID, productManifest, principal, public)
		if binding, ok := generatedBindings[params.Name]; ok {
			s.executeAPIDefaultTool(ctx, w, request, params, productID, binding, public, productManifest, scope, principal)
			return
		}
	}
	switch params.Name {
	case "developer_assets.search":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Developer-asset publication scope could not be resolved")
			return
		}
		var input platform.DeveloperAssetQueryLabInput
		if err := decodeArguments(params.Arguments, &input); err != nil {
			writeRPCError(w, request.ID, -32602, "Developer-asset search arguments are invalid")
			return
		}
		if input.Limit == 0 {
			input.Limit = 10
		}
		if input.Limit > 20 {
			writeRPCError(w, request.ID, -32602, "Developer-asset search limit cannot exceed 20")
			return
		}
		result, searchErr := s.callDeveloperAssetSearch(ctx, input, public, productManifest)
		if searchErr != nil {
			writeRPCErrorData(w, request.ID, -32004, "Developer-asset search could not resolve an exact published scope", map[string]any{"reason": searchErr.Error()})
			return
		}
		writeToolResult(w, request.ID, result)
	case "integration.recipes.list":
		if len(params.Arguments) != 0 {
			writeRPCError(w, request.ID, -32602, "Recipe list arguments must be empty")
			return
		}
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipes could not be listed")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipes": sortedRecipeSummaries(values)})
	case "integration.plan":
		outcome, valid := exactRecipeToolStringArgument(params.Arguments, "outcome")
		if !valid {
			writeRPCError(w, request.ID, -32602, "A valid integration outcome is required")
			return
		}
		values, err := s.publishedRecipes(ctx, productID, public)
		if err != nil {
			writeRPCError(w, request.ID, -32603, "Recipes could not be listed")
			return
		}
		selected, matches := exactRecipeMatch(values, outcome)
		if selected == nil {
			if len(matches) > 1 {
				candidates, truncated := boundedRecipeSummaries(matches)
				writeRPCErrorData(w, request.ID, -32009, "More than one published recipe exactly matches this outcome", map[string]any{"reason": "ambiguous_exact_match", "candidate_count": len(matches), "candidates_truncated": truncated, "candidates": candidates})
				return
			}
			candidates, truncated := boundedRecipeSummaries(values)
			writeRPCErrorData(w, request.ID, -32004, "No published recipe exactly matches this outcome", map[string]any{"reason": "no_exact_match", "candidate_count": len(values), "candidates_truncated": truncated, "candidates": candidates})
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": selected.StableURI, "title": selected.Title, "outcome": selected.Outcome, "integration_id": selected.IntegrationID, "contract_version": selected.ContractVersion, "revision_id": selected.CurrentRevisionID, "next_step": "Read the recipe resource, then implement and verify its minimal product-integration steps. MCP is already connected."})
	case "integration.check":
		recipeURI, valid := exactRecipeToolStringArgument(params.Arguments, "recipe_uri")
		if !valid {
			writeRPCError(w, request.ID, -32602, "A valid recipe URI is required")
			return
		}
		recipe, err := s.publishedRecipeByURI(ctx, productID, recipeURI, public)
		if err != nil {
			writeRPCError(w, request.ID, -32004, "Recipe resource not found")
			return
		}
		writeToolResult(w, request.ID, map[string]any{"recipe_uri": recipe.StableURI, "integration_id": recipe.IntegrationID, "contract_version": recipe.ContractVersion, "state": recipe.State, "current": recipe.State == "published" && !recipe.NeedsAttention, "needs_attention": recipe.NeedsAttention, "revision_id": recipe.CurrentRevisionID, "published_at": recipe.PublishedAt})
	case "deployment.get_manifest", "product.get_manifest":
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32603, "Deployment discovery failed")
			return
		}
		writeToolResult(w, request.ID, productManifest)
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
		writeToolResult(w, request.ID, reportToolResult(value))
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
		writeToolResult(w, request.ID, filtered)
	default:
		if public {
			writeRPCError(w, request.ID, -32601, "Tool is not available on Public MCP")
			return
		}
		if manifestErr != nil {
			writeRPCError(w, request.ID, -32003, "Tool execution requires a resolved deployment catalog")
			return
		}
		if s.toolRuntime != nil {
			requestID, _ := ctx.Value(requestIDKey).(string)
			principal, _ := ctx.Value(principalKey).(identity.Principal)
			selectedTool, lookupErr := s.executableTool(ctx, productID, params.Name, scope)
			if errors.Is(lookupErr, errToolCatalogExcluded) {
				writeRPCError(w, request.ID, -32003, "Tool is not included in a published API revision")
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
			writeRPCError(w, request.ID, -32603, "Tool execution failed safely; review the tool activity for the sanitized failure category")
			return
		}
		writeRPCError(w, request.ID, -32601, "Tool not found")
	}
}
