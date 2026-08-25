package platform

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (s *Service) ImportToolDraft(ctx context.Context, productID string, input ToolDraftImportInput, actor Actor) (ToolDraftImportResult, error) {
	product, err := s.store.Product(ctx, strings.TrimSpace(productID))
	if err != nil {
		return ToolDraftImportResult{}, err
	}
	if len(input.Source.Value) == 0 || len(input.Source.Value) > maxToolBuilderImportBytes || !utf8.ValidString(input.Source.Value) {
		return ToolDraftImportResult{}, fmt.Errorf("%w: import source is required and must be no larger than 512 KiB", ErrToolBuilderInvalidInput)
	}
	kind := detectToolBuilderImportKind(input.Source.Kind, input.Source.Value)
	if kind == "openapi_url" {
		return ToolDraftImportResult{}, fmt.Errorf("%w: URL fetching is disabled; paste the OpenAPI document instead", ErrToolBuilderInvalidInput)
	}
	preparedBase, err := s.prepareToolDraftContext(ctx, product.ID, input.ToolDraftContext)
	if err != nil {
		return ToolDraftImportResult{}, err
	}
	base, baseFindings := normalizeToolBuilderDraft(preparedBase)
	var drafts []ToolDraft
	credentialDetected := containsToolBuilderSecretText(input.Source.Value)
	switch kind {
	case "curl":
		draft, detected, parseErr := buildToolDraftFromCurl(base, input.Source.Value)
		if parseErr != nil {
			return ToolDraftImportResult{}, parseErr
		}
		drafts, credentialDetected = []ToolDraft{draft}, credentialDetected || detected
	case "postman", "postman_collection":
		var detected bool
		drafts, detected, err = buildToolDraftsFromPostman(base, input.Source.Value)
		credentialDetected = credentialDetected || detected
	case "openapi", "openapi_json", "openapi_yaml", "openapi_document":
		if strings.Contains(input.Source.Value, "schema.getpostman.com") {
			var detected bool
			drafts, detected, err = buildToolDraftsFromPostman(base, input.Source.Value)
			credentialDetected = credentialDetected || detected
		} else {
			drafts, err = buildToolDraftsFromOpenAPI(base, input.Source.Value)
		}
	default:
		err = fmt.Errorf("%w: import format is not supported", ErrToolBuilderInvalidInput)
	}
	if err != nil {
		return ToolDraftImportResult{}, err
	}
	result := ToolDraftImportResult{Candidates: make([]ToolDraftImportCandidate, 0, len(drafts)), Findings: append(make([]ToolDraftFinding, 0, len(baseFindings)), baseFindings...), GeneratedAt: s.now()}
	if credentialDetected {
		result.Findings = append(result.Findings, toolBuilderFinding("warning", "credential_material_not_imported", "source", "Credential material was detected and excluded. Configure the credential separately after choosing a candidate."))
	}
	for _, draft := range drafts {
		candidateContext := input.ToolDraftContext
		candidateContext.Draft = draft
		validation, validateErr := s.ValidateToolDraftContext(ctx, product.ID, candidateContext)
		if validateErr != nil {
			return ToolDraftImportResult{}, validateErr
		}
		result.Candidates = append(result.Candidates, ToolDraftImportCandidate{Summary: "Imported a candidate HTTP operation for review.", Draft: validation.NormalizedDraft, Changes: toolBuilderChanges(base, validation.NormalizedDraft, "Updated from imported contract metadata."), Findings: validation.Findings, Valid: validation.Valid})
	}
	sortToolBuilderFindings(result.Findings)
	if err := s.appendToolBuilderAudit(ctx, product, actor, "tool_builder.imported", map[string]any{"format": kind, "candidate_count": len(result.Candidates), "credential_material_detected": credentialDetected}); err != nil {
		return ToolDraftImportResult{}, err
	}
	return result, nil
}
