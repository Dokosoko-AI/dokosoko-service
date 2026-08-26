package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (s *Service) AnalyseIntegration(ctx context.Context, productID string, actor Actor) (model.IntegrationAnalysis, error) {
	return s.analyseIntegration(ctx, productID, "", actor)
}

func (s *Service) AnalyseIntegrationFor(ctx context.Context, productID, integrationID string, actor Actor) (model.IntegrationAnalysis, error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return model.IntegrationAnalysis{}, errors.New("integration_id is required for an integration-scoped analysis")
	}
	return s.analyseIntegration(ctx, productID, integrationID, actor)
}

func (s *Service) analyseIntegration(ctx context.Context, productID, integrationID string, actor Actor) (analysis model.IntegrationAnalysis, runErr error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return analysis, err
	}
	var selectedIntegration *model.Integration
	var evidence []model.IntegrationEvidence
	if integrationID == "" {
		evidence, err = s.integrationEvidence(ctx, product)
	} else {
		var selected model.Integration
		evidence, selected, err = s.scopedIntegrationEvidence(ctx, product, integrationID)
		selectedIntegration = &selected
	}
	if err != nil {
		return analysis, err
	}
	fallback, unknowns := s.deterministicIntegrationPlan(ctx, product, evidence, selectedIntegration)
	id, err := randomUUID()
	if err != nil {
		return analysis, err
	}
	analysis = model.IntegrationAnalysis{ID: id, OrganisationID: product.OrganisationID, ProductID: product.ID, SchemaVersion: integrationAnalysisSchemaVersion, State: "running", GeneratedBy: "deterministic", Evidence: evidence, Plan: fallback, Unknowns: unknowns}
	analysis, err = s.store.SaveIntegrationAnalysis(ctx, analysis, 0)
	if err != nil {
		return analysis, err
	}
	promptInput := map[string]any{
		"product":              map[string]any{"name": product.Name, "slug": product.Slug, "description": product.Description, "public_mcp_enabled": product.PublicMCPEnabled},
		"platform_contract":    fallback,
		"evidence":             evidence,
		"unknowns":             unknowns,
		"allowed_endpoint_ids": integrationEndpointIDs(fallback),
		"allowed_evidence_ids": evidenceIDs(evidence),
	}
	if selectedIntegration != nil {
		promptInput["integration"] = map[string]any{"id": selectedIntegration.ID, "family_key": selectedIntegration.FamilyKey, "version_key": selectedIntegration.VersionKey, "display_name": selectedIntegration.DisplayName, "description": selectedIntegration.Description}
	}
	prompt, _ := json.Marshal(promptInput)
	result, aiErr := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "integration_analysis", PromptKey: AIPromptKeyIntegrationAnalysis, User: string(prompt), SchemaName: "integration_analysis", Schema: integrationAnalysisSchema, MaxOutput: 8192, Temperature: 0})
	if aiErr == nil {
		var response integrationAnalysisAIResponse
		if decodeStrictAIResult(result.JSON, &response) == nil {
			aiPlan, valid := integrationAnalysisResponsePlan(response, fallback, evidence)
			if !valid {
				analysis.ErrorCode = string(airuntime.ErrorInvalidStructuredOutput)
			} else {
				analysis.Plan = normalizeIntegrationPlan(aiPlan, fallback, evidence)
				if selectedIntegration != nil {
					analysis.Plan = namespaceIntegrationRecipes(analysis.Plan, product, *selectedIntegration)
				}
				analysis.GeneratedBy = "ai_assisted"
			}
		} else {
			analysis.ErrorCode = string(airuntime.ErrorInvalidStructuredOutput)
		}
	} else {
		analysis.ErrorCode = string(airuntime.Code(aiErr))
	}
	now := s.now()
	analysis.State, analysis.CompletedAt = "review", &now
	analysis, runErr = s.store.SaveIntegrationAnalysis(ctx, analysis, analysis.Revision)
	if runErr == nil {
		current := map[string]any{"generated_by": analysis.GeneratedBy, "evidence_count": len(analysis.Evidence), "unknown_count": len(analysis.Unknowns), "recipe_count": len(analysis.Plan.Recipes)}
		if selectedIntegration != nil {
			current["integration_id"] = selectedIntegration.ID
		}
		if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "integration.analysis.completed", TargetType: "integration_analysis", TargetID: analysis.ID, Current: current, RequestID: actor.RequestID, CreatedAt: now}); err != nil {
			return model.IntegrationAnalysis{}, err
		}
	}
	return analysis, runErr
}

func integrationEndpointIDs(plan model.IntegrationPlan) []string {
	values := make([]string, 0, len(plan.Endpoints))
	for _, endpoint := range plan.Endpoints {
		if endpoint.Name != "" {
			values = append(values, endpoint.Name)
		}
	}
	return values
}

func integrationAnalysisResponsePlan(response integrationAnalysisAIResponse, fallback model.IntegrationPlan, evidence []model.IntegrationEvidence) (model.IntegrationPlan, bool) {
	response.Summary = strings.TrimSpace(response.Summary)
	if response.Summary == "" || len(response.Summary) > 1000 || containsToolBuilderSecretText(response.Summary) || !allowedUniqueEvidenceIDs(response.SummaryEvidenceIDs, evidence) || len(response.Recipes) == 0 {
		return model.IntegrationPlan{}, false
	}
	allowedEndpoints := make(map[string]model.IntegrationEndpointPlan, len(fallback.Endpoints))
	for _, endpoint := range fallback.Endpoints {
		allowedEndpoints[endpoint.Name] = endpoint
	}
	plan := fallback
	plan.Summary = response.Summary
	plan.Recipes = make([]model.RecipeSeed, 0, len(response.Recipes))
	seenSlugs := make(map[string]bool, len(response.Recipes))
	for _, candidate := range response.Recipes {
		seed := candidate.RecipeSeed
		seed.EvidenceIDs = append([]string(nil), candidate.EvidenceIDs...)
		seed.Slug = slugify(seed.Slug)
		seed.Title = strings.TrimSpace(seed.Title)
		seed.Outcome = strings.TrimSpace(seed.Outcome)
		seed.Audience = strings.TrimSpace(seed.Audience)
		for index := range seed.EvidenceIDs {
			seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
		}
		candidate.Rationale = strings.TrimSpace(candidate.Rationale)
		if seed.Slug == "" || seenSlugs[seed.Slug] || seed.Title == "" || len(seed.Title) > 160 || seed.Outcome == "" || len(seed.Outcome) > 1000 || seed.Audience == "" || len(seed.Audience) > 80 || candidate.Rationale == "" || len(candidate.Rationale) > 1000 || containsToolBuilderSecretText(seed.Title+" "+seed.Outcome+" "+seed.Audience+" "+candidate.Rationale) || !allowedUniqueEvidenceIDs(candidate.EvidenceIDs, evidence) {
			return model.IntegrationPlan{}, false
		}
		seenEndpointIDs := make(map[string]bool, len(seed.EndpointIDs))
		endpointEvidence := make(map[string]bool)
		for index, endpointID := range seed.EndpointIDs {
			endpointID = strings.TrimSpace(endpointID)
			endpoint, ok := allowedEndpoints[endpointID]
			if !ok || seenEndpointIDs[endpointID] {
				return model.IntegrationPlan{}, false
			}
			seed.EndpointIDs[index] = endpointID
			seenEndpointIDs[endpointID] = true
			for _, evidenceID := range endpoint.Evidence {
				endpointEvidence[evidenceID] = true
			}
		}
		if len(seed.EndpointIDs) == 0 {
			return model.IntegrationPlan{}, false
		}
		for _, evidenceID := range seed.EvidenceIDs {
			if !endpointEvidence[evidenceID] {
				return model.IntegrationPlan{}, false
			}
		}
		seenSlugs[seed.Slug] = true
		plan.Recipes = append(plan.Recipes, seed)
	}
	return plan, true
}
