package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"io"
	"net/http"
	"sort"
	"strings"
)

var errToolCatalogExcluded = errors.New("tool is excluded from the published API catalog")

// executableTool resolves the exact row checked against the current published
// API catalog. Callers must not fall through to Runtime's
// second lookup when this read fails or finds no row: doing so would create a
// publication race that bypasses the catalog check.
func (s *Server) executableTool(ctx context.Context, productID, fullName string, scope model.CatalogScope) (model.Tool, error) {
	_ = s.syncNativePlugins(ctx, productID)
	available, err := s.service.Store().Tools(ctx, productID, false)
	if err != nil {
		return model.Tool{}, err
	}
	manifest, manifestErr := s.service.ProductManifestFor(ctx, productID, scope)
	matches := make([]model.Tool, 0, 1)
	excluded := false
	for _, candidate := range available {
		legacyName := candidate.Namespace + "." + candidate.Name
		canonicalName, canonical := canonicalCustomToolName(manifest, candidate)
		if legacyName != fullName && (manifestErr != nil || !canonical || canonicalName != fullName) {
			continue
		}
		_, allowed, allowErr := s.service.CatalogAllowsTool(ctx, productID, scope, candidate)
		if allowErr != nil || !allowed {
			excluded = true
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return model.Tool{}, store.ErrConflict
	}
	if excluded {
		return model.Tool{}, errToolCatalogExcluded
	}
	return model.Tool{}, store.ErrNotFound
}

func (s *Server) knowledgePublicationIDs(ctx context.Context, productID, requestedIntegrationID string, manifest model.ProductManifest) ([]string, error) {
	requestedIntegrationID = strings.TrimSpace(requestedIntegrationID)
	type candidate struct {
		integrationID string
		publications  []string
	}
	candidates := make([]candidate, 0)
	for _, integration := range manifest.Integrations {
		publications := make([]string, 0)
		seen := make(map[string]bool)
		for _, resource := range integration.Resources {
			if resource.Kind != "documentation" {
				continue
			}
			for _, publication := range resource.SourcePublications {
				if publication.ID != "" && !seen[publication.ID] {
					seen[publication.ID] = true
					publications = append(publications, publication.ID)
				}
			}
		}
		if len(publications) > 0 {
			sort.Strings(publications)
			candidates = append(candidates, candidate{integrationID: integration.ID, publications: publications})
		}
	}
	if requestedIntegrationID != "" {
		for _, value := range candidates {
			if value.integrationID == requestedIntegrationID {
				return value.publications, nil
			}
		}
		return nil, store.ErrNotFound
	}
	if len(candidates) == 1 {
		return candidates[0].publications, nil
	}
	if len(manifest.Integrations) > 0 {
		return nil, store.ErrConflict
	}
	// Legacy deployments without an Integration catalog remain usable, but are
	// still limited to the latest explicitly reviewed publication per source.
	sources, err := s.service.Store().Sources(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		publications, publicationErr := s.service.Store().SourcePublications(ctx, productID, source.ID)
		if publicationErr != nil {
			if errors.Is(publicationErr, store.ErrNotFound) {
				continue
			}
			return nil, publicationErr
		}
		if len(publications) > 0 {
			result = append(result, publications[0].ID)
		}
	}
	if len(result) == 0 {
		return nil, store.ErrNotFound
	}
	sort.Strings(result)
	return result, nil
}

func (s *Server) integrationToolAuthorization(ctx context.Context, manifest model.ProductManifest, tool model.Tool) (toolruntime.BoundAuthorization, bool, error) {
	managed := manifest.ManagedIntegrationTools
	type candidate struct {
		integration model.IntegrationManifest
		tool        model.IntegrationManifestTool
	}
	candidates := make([]candidate, 0, 1)
	managedIntegrations := 0
	for _, integration := range manifest.Integrations {
		if integration.Tools == nil {
			continue
		}
		managed = true
		managedIntegrations++
		for _, binding := range integration.Tools {
			if binding.ToolID == tool.ID && binding.ToolRevision == tool.Revision {
				candidates = append(candidates, candidate{integration: integration, tool: binding})
			}
		}
	}
	if !managed {
		return toolruntime.BoundAuthorization{}, false, nil
	}
	_ = managedIntegrations
	if len(candidates) != 1 {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool must resolve to exactly one applicable Integration")
	}
	selected := candidates[0]
	if selected.tool.AuthorizationPointID == "" || selected.tool.AuthorizationPointRevision < 1 {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool has no exact authorization-point binding")
	}
	pointPublished := false
	for _, point := range selected.integration.AuthorizationPoints {
		if point.ID == selected.tool.AuthorizationPointID && point.Revision == selected.tool.AuthorizationPointRevision {
			pointPublished = true
			break
		}
	}
	if !pointPublished {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point is not part of the same published Integration revision")
	}
	point, err := s.service.Store().AuthorizationPoint(ctx, selected.integration.ID, selected.tool.AuthorizationPointID)
	if err != nil || point.State != "active" || point.Revision != selected.tool.AuthorizationPointRevision {
		return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point changed or is not active")
	}
	definitions, err := s.service.Store().GrantDefinitions(ctx, manifest.DeploymentID)
	if err != nil {
		return toolruntime.BoundAuthorization{}, true, errors.New("grant registry could not be resolved")
	}
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		active[definition.Key] = definition.State == "active"
	}
	for _, required := range point.RequiredGrants {
		if !active[required] {
			return toolruntime.BoundAuthorization{}, true, errors.New("tool authorization point requires an inactive grant")
		}
	}
	return toolruntime.BoundAuthorization{IntegrationID: selected.integration.ID, ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPoint: point, AuthorizationPointRevision: point.Revision}, true, nil
}

func customToolDefinition(manifest model.ProductManifest, tool model.Tool) map[string]any {
	return customToolDefinitionForAuthorization(manifest, tool, toolruntime.BoundAuthorization{}, false)
}

func customToolDefinitionForAuthorization(manifest model.ProductManifest, tool model.Tool, binding toolruntime.BoundAuthorization, managed bool) map[string]any {
	var policy struct {
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
		Risk                 string   `json:"risk"`
		IdempotencyRequired  bool     `json:"idempotency_required"`
	}
	_ = json.Unmarshal(tool.AuthorizationPolicy, &policy)
	if managed {
		seen := make(map[string]bool, len(policy.RequiredGrants)+len(binding.AuthorizationPoint.RequiredGrants))
		combined := make([]string, 0, len(policy.RequiredGrants)+len(binding.AuthorizationPoint.RequiredGrants))
		for _, required := range append(append([]string(nil), policy.RequiredGrants...), binding.AuthorizationPoint.RequiredGrants...) {
			if !seen[required] {
				seen[required] = true
				combined = append(combined, required)
			}
		}
		sort.Strings(combined)
		policy.RequiredGrants = combined
		policy.ConfirmationRequired = policy.ConfirmationRequired || binding.AuthorizationPoint.ConfirmationRequired
	}
	if policy.Risk == "" {
		policy.Risk = "low"
		if toolEffect(tool) == "write" {
			policy.Risk = "medium"
		}
		if toolEffect(tool) == "destructive" {
			policy.Risk = "critical"
		}
	}
	idempotencyKeyRequired := strings.EqualFold(strings.TrimSpace(tool.IdempotencyMode), "required") || policy.IdempotencyRequired
	description := tool.Description
	if policy.ConfirmationRequired {
		if managed {
			description += " The client must preview the exact invocation, obtain confirmation, and retry with the server-issued confirmation challenge; the retry is a client attestation, not independent server proof of human approval."
		} else {
			description += " The client must preview the exact invocation and attest that it obtained confirmation; this client attestation is not independent server proof of human approval."
		}
	}
	if idempotencyKeyRequired {
		description += " Supply one stable params._meta.idempotency_key for the invocation and reuse it across transport retries."
	}
	integrationIDs := make([]string, 0, 1)
	if managed {
		integrationIDs = append(integrationIDs, binding.IntegrationID)
	} else {
		for _, integration := range manifest.Integrations {
			for _, candidate := range integration.Tools {
				if candidate.ToolID == tool.ID && candidate.ToolRevision == tool.Revision {
					integrationIDs = append(integrationIDs, integration.ID)
					break
				}
			}
		}
	}
	actionType := ""
	pointID := ""
	pointRevision := int64(0)
	decisionTTL := 0
	if managed {
		actionType = binding.AuthorizationPoint.ActionType
		pointID = binding.AuthorizationPoint.ID
		pointRevision = binding.AuthorizationPointRevision
		decisionTTL = binding.AuthorizationPoint.DecisionTTLSeconds
	}
	effect := toolEffect(tool)
	actionType = strings.ToLower(strings.TrimSpace(actionType))
	// An authorization point may make an operation more restrictive, but it
	// must never erase the safety signal inherent in the tool effect or tool
	// policy. This also remains fail-safe if stale data contains an invalid
	// read binding for a mutation.
	readOnly := effect == "read" && (actionType == "" || actionType == "read")
	destructive := strings.EqualFold(strings.TrimSpace(policy.Risk), "critical") || effect == "destructive" || actionType == "destructive"
	metadata := map[string]any{
		"com.dokosoko/toolRevision":                    tool.Revision,
		"com.dokosoko/integrationIds":                  integrationIDs,
		"com.dokosoko/authorizationPointId":            pointID,
		"com.dokosoko/authorizationPointRevision":      pointRevision,
		"com.dokosoko/authorizationDecisionTtlSeconds": decisionTTL,
		"com.dokosoko/requiredGrants":                  policy.RequiredGrants,
		"com.dokosoko/confirmationRequired":            policy.ConfirmationRequired,
		"com.dokosoko/risk":                            policy.Risk,
		"com.dokosoko/idempotencyKeyRequired":          idempotencyKeyRequired,
		"com.dokosoko/idempotencyKeyMetaField":         "idempotency_key",
	}
	if managed && policy.ConfirmationRequired {
		metadata["com.dokosoko/confirmationChallengeMetaField"] = managedToolConfirmationMetaField
		metadata["com.dokosoko/confirmationAttestationMetaField"] = "confirmed"
	}
	return map[string]any{
		"name":        tool.Namespace + "." + tool.Name,
		"description": description,
		"inputSchema": tool.InputSchema,
		"annotations": map[string]any{
			"readOnlyHint":    readOnly,
			"destructiveHint": destructive,
			"idempotentHint":  effect == "read" || tool.IdempotencyMode == "supported" || idempotencyKeyRequired,
		},
		"_meta": metadata,
	}
}

func toolEffect(tool model.Tool) string {
	if effect := strings.ToLower(strings.TrimSpace(tool.Effect)); effect == "read" || effect == "write" || effect == "destructive" {
		return effect
	}
	switch strings.ToUpper(strings.TrimSpace(tool.HTTPMethod)) {
	case http.MethodGet:
		return "read"
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return "write"
	default:
		return "destructive"
	}
}

func reportProductContext(manifest model.ProductManifest) reporting.ProductContext {
	return reporting.ProductContext{ProductID: manifest.ProductID, ProductName: manifest.ProductName, CatalogRevision: manifest.CatalogRevision}
}

func (s *Server) reportIntegrationContext(ctx context.Context, deploymentID, integrationID string) (*reporting.IntegrationContext, error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return nil, nil
	}
	integration, err := s.service.Store().Integration(ctx, deploymentID, integrationID)
	if err != nil || integration.Lifecycle == "retired" {
		return nil, store.ErrNotFound
	}
	value := &reporting.IntegrationContext{IntegrationID: integration.ID, FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Lifecycle: integration.Lifecycle, Revision: integration.Revision}
	revisions, err := s.service.Store().IntegrationRevisions(ctx, integration.ID)
	if err != nil {
		return nil, err
	}
	for _, revision := range revisions {
		if revision.State == "published" {
			value.Revision, value.ManifestHash, value.Snapshot = revision.Revision, revision.ManifestHash, revision.Snapshot
			break
		}
	}
	return value, nil
}

func reportToolResult(value reporting.SubmissionView) map[string]any {
	return map[string]any{"submission_id": value.ID, "status": value.State}
}

func reportingRPCError(w http.ResponseWriter, id any, err error) {
	switch {
	case errors.Is(err, reporting.ErrSensitiveContent):
		writeRPCError(w, id, -32602, "Potential credential or secret detected; redact it and ask the user to approve the revised report")
	case errors.Is(err, reporting.ErrInvalidReport):
		writeRPCError(w, id, -32602, err.Error())
	case errors.Is(err, reporting.ErrDeliveryDisabled):
		writeRPCError(w, id, -32601, "Support submission delivery is disabled")
	default:
		writeRPCError(w, id, -32603, "The report could not be queued safely")
	}
}

func decodeArguments(arguments map[string]any, destination any) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func toolPrincipal(principal identity.Principal, confirmed bool, requestID, idempotencyKey string) toolruntime.Principal {
	return toolruntime.Principal{
		Subject:              principal.Subject,
		Issuer:               principal.Issuer,
		CustomerAccountID:    principal.CustomerAccountID,
		ExternalCustomerID:   principal.ExternalCustomerID,
		InstallationID:       principal.InstallationID,
		Grants:               principal.Grants,
		AccessEvaluationID:   principal.AccessEvaluationID,
		AccessEvaluatedAt:    principal.AccessEvaluatedAt,
		DelegatedAPIOrigin:   principal.DelegatedAPIOrigin,
		DelegatedAccessToken: principal.UpstreamAccessToken,
		Confirmed:            confirmed,
		RequestID:            requestID,
		IdempotencyKey:       idempotencyKey,
	}
}

func decodeJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func (s *Server) platformError(w http.ResponseWriter, err error, warning string) {
	switch {
	case errors.Is(err, platform.ErrConfirmationRequired):
		writeError(w, http.StatusConflict, "public_confirmation_required", warning, map[string]any{"requires": "acknowledge_public"})
	case errors.Is(err, platform.ErrUnsafeForPublic):
		writeError(w, http.StatusUnprocessableEntity, "unsafe_for_public", err.Error(), nil)
	case errors.Is(err, platform.ErrInvalidVisibility):
		writeError(w, http.StatusBadRequest, "invalid_visibility", err.Error(), nil)
	case errors.Is(err, platform.ErrSourceReviewRequired):
		writeError(w, http.StatusBadRequest, "source_review_required", err.Error(), nil)
	default:
		s.storeError(w, err)
	}
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found.", nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "The resource changed. Refresh and try again.", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed.", nil)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}

func writeRPC(w http.ResponseWriter, id, result any) {
	if source, ok := result.(map[string]any); ok {
		withMeta := make(map[string]any, len(source)+1)
		for key, value := range source {
			withMeta[key] = value
		}
		meta := map[string]any{}
		if sourceMeta, ok := source["_meta"].(map[string]any); ok {
			for key, value := range sourceMeta {
				meta[key] = value
			}
		}
		meta["io.modelcontextprotocol/serverInfo"] = map[string]any{"name": "DokoSoko", "version": "2.0.0"}
		withMeta["_meta"] = meta
		result = withMeta
	}
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeRPCError(w http.ResponseWriter, id any, code int, message string) {
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}

func writeRPCErrorData(w http.ResponseWriter, id any, code int, message string, data map[string]any) {
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message, "data": data}})
}

func writeToolResult(w http.ResponseWriter, id, value any) {
	encoded, _ := json.Marshal(value)
	writeRPC(w, id, map[string]any{"resultType": "complete", "content": []map[string]any{{"type": "text", "text": string(encoded)}}, "structuredContent": value, "isError": false})
}
