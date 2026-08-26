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
	productEvidence := unambiguousIntegrationProductEvidence(evidence)
	allowedCapabilityIDs := []string(nil)
	allowedSDKIDs := []string(nil)
	if selectedIntegration != nil {
		allowedCapabilityIDs = viableIntegrationRecipeCapabilityIDs(productEvidence, selectedIntegration.ID)
		allowedSDKIDs = viableIntegrationRecipeSDKIDs(productEvidence)
	}
	promptInput := map[string]any{
		"product":                map[string]any{"name": product.Name, "slug": product.Slug, "description": product.Description},
		"evidence":               productEvidence,
		"unknowns":               integrationProductRecipeUnknowns(unknowns),
		"allowed_capability_ids": allowedCapabilityIDs,
		"allowed_sdk_ids":        allowedSDKIDs,
		"allowed_evidence_ids":   evidenceIDs(productEvidence),
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
				analysis.GeneratedBy = "ai_assisted"
			}
		} else {
			analysis.ErrorCode = string(airuntime.ErrorInvalidStructuredOutput)
		}
	} else {
		analysis.ErrorCode = string(airuntime.Code(aiErr))
	}
	if len(analysis.Plan.Recipes) == 0 {
		analysis.Unknowns = ensureMissingIntegrationRecipeUnknown(analysis.Unknowns)
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

func viableIntegrationRecipeCapabilityIDs(evidence []model.IntegrationEvidence, integrationID string) []string {
	byID, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(evidence))
	result := make([]string, 0)
	for _, id := range recipeProductCapabilityIDs(evidence) {
		item, exists := byID[id]
		if exists && !ambiguous[id] && integrationRecipeCapabilityViable(item, integrationID) {
			result = append(result, id)
		}
	}
	return result
}

func viableIntegrationRecipeSDKIDs(evidence []model.IntegrationEvidence) []string {
	byID, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(evidence))
	result := make([]string, 0)
	for _, id := range recipeProductSDKIDs(evidence) {
		item, exists := byID[id]
		if exists && !ambiguous[id] && integrationRecipeSDKViable(item) {
			result = append(result, id)
		}
	}
	return result
}

func unambiguousIntegrationProductEvidence(evidence []model.IntegrationEvidence) []model.IntegrationEvidence {
	productEvidence := recipeProductEvidence(evidence)
	_, ambiguous := recipeUniqueEvidenceByID(productEvidence)
	result := make([]model.IntegrationEvidence, 0, len(productEvidence))
	for _, item := range productEvidence {
		id := strings.TrimSpace(item.ResourceID)
		if id != "" && !ambiguous[id] {
			result = append(result, item)
		}
	}
	return result
}

func ensureMissingIntegrationRecipeUnknown(values []model.IntegrationUnknown) []model.IntegrationUnknown {
	for _, value := range values {
		if value.ID == "integration-scope" || value.ID == "product-capability" {
			return values
		}
	}
	return append(values, model.IntegrationUnknown{ID: "product-capability", Question: "Which exact product operation or API contract should the recipe implement?", Why: "No candidate selected exactly one viable product capability with its exact reviewed evidence.", Blocking: true})
}

func integrationProductRecipeUnknowns(values []model.IntegrationUnknown) []model.IntegrationUnknown {
	result := make([]model.IntegrationUnknown, 0, len(values))
	for _, value := range values {
		if value.ID == "integration-scope" || value.ID == "product-capability" {
			result = append(result, value)
		}
	}
	return result
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

func recipeSeedForCapability(values []model.RecipeSeed, capabilityID string) (model.RecipeSeed, bool) {
	for _, value := range values {
		if len(value.CapabilityIDs) == 1 && value.CapabilityIDs[0] == capabilityID {
			return value, true
		}
	}
	return model.RecipeSeed{}, false
}

func integrationAnalysisResponsePlan(response integrationAnalysisAIResponse, fallback model.IntegrationPlan, evidence []model.IntegrationEvidence) (model.IntegrationPlan, bool) {
	if len(response.Recipes) > 12 {
		return model.IntegrationPlan{}, false
	}
	// The model may select among server-derived operation candidates, but it
	// does not author product facts. Titles, outcomes, slugs, and the summary
	// remain canonical server output derived from exact structured evidence.
	plan := fallback
	if len(response.Recipes) == 0 {
		return plan, true
	}
	plan.Recipes = make([]model.RecipeSeed, 0, len(response.Recipes))
	seenCapabilities := make(map[string]bool, len(response.Recipes))
	for _, candidate := range response.Recipes {
		seed := model.RecipeSeed{
			Audience:      "coding_agent",
			CapabilityIDs: append([]string(nil), candidate.CapabilityIDs...),
			SDKID:         strings.TrimSpace(candidate.SDKID),
			EvidenceIDs:   append([]string(nil), candidate.EvidenceIDs...),
		}
		for index := range seed.CapabilityIDs {
			seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
		}
		for index := range seed.EvidenceIDs {
			seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
		}
		if len(seed.CapabilityIDs) != 1 || seenCapabilities[seed.CapabilityIDs[0]] {
			return model.IntegrationPlan{}, false
		}
		if _, valid := integrationRecipeSelection(evidence, seed); !valid {
			return model.IntegrationPlan{}, false
		}
		canonical, exists := recipeSeedForCapability(fallback.Recipes, seed.CapabilityIDs[0])
		if !exists {
			return model.IntegrationPlan{}, false
		}
		canonical.SDKID = seed.SDKID
		canonical.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
		seenCapabilities[seed.CapabilityIDs[0]] = true
		plan.Recipes = append(plan.Recipes, canonical)
	}
	return plan, true
}
