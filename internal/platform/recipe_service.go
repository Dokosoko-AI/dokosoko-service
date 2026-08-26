package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrRecipeNeedsInput = errors.New("the requested recipe is not supported by the current reviewed evidence")

func (s *Service) createRecipeFromSeed(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string, actor Actor) (model.Recipe, error) {
	selectedEvidence, selectionOK := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs)
	if !selectionOK || len(selectedEvidence) == 0 {
		return model.Recipe{}, ErrRecipeNeedsInput
	}
	recipeID, err := randomUUID()
	if err != nil {
		return model.Recipe{}, err
	}
	seed.Slug = slugify(seed.Slug)
	if seed.Slug == "" {
		seed.Slug = "recipe"
	}
	if _, lookupErr := s.store.RecipeBySlug(ctx, product.ID, seed.Slug); lookupErr == nil {
		seed.Slug += "-" + strings.ReplaceAll(recipeID, "-", "")[:8]
	} else if !errors.Is(lookupErr, store.ErrNotFound) {
		return model.Recipe{}, lookupErr
	}
	recipe := model.Recipe{ID: recipeID, OrganisationID: product.OrganisationID, ProductID: product.ID, AnalysisID: analysis.ID, Slug: seed.Slug, Title: seed.Title, Outcome: seed.Outcome, Audience: seed.Audience, State: "draft", Generated: true, NeedsAttention: true, Visibility: model.VisibilityPrivate, Dependencies: recipeGroundingDependencies(analysis, seed), StableURI: "dokosoko://products/" + product.Slug + "/recipes/" + seed.Slug}
	recipe, err = s.store.SaveRecipe(ctx, recipe, 0)
	if err != nil {
		return recipe, err
	}
	markdown, references, generatedBy, modelID := s.authorRecipe(ctx, product, analysis, seed, instruction)
	findings := validateRecipeMarkdown(markdown, recipe.Title, references, recipeGroundedURLs(recipeAnalysisWithEvidence(analysis, selectedEvidence))...)
	review, findings := s.reviewRecipe(ctx, product, recipe, markdown, findings)
	revisionID, err := randomUUID()
	if err != nil {
		return recipe, err
	}
	recipe.State = "review"
	recipe, err = s.store.SaveRecipeRevision(ctx, recipe, model.RecipeRevision{ID: revisionID, RecipeID: recipe.ID, Markdown: markdown, References: references, Validation: findings, Review: review, GeneratedBy: generatedBy, Model: modelID, CreatedBy: actor.ID}, recipe.Revision)
	if err != nil {
		return recipe, err
	}
	return s.requireCurrentRecipeGrounding(ctx, product, recipe)
}

func (s *Service) CreateRecipeFromPrompt(ctx context.Context, productID, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	return s.CreateRecipeFromPromptFor(ctx, productID, "", instruction, actor)
}

func recipePromptEndpointIDs(plan model.IntegrationPlan, instruction string) []string {
	instruction = strings.ToLower(instruction)
	for _, endpoint := range plan.Endpoints {
		if endpoint.Name == "mcp" && strings.Contains(instruction, "mcp") {
			return []string{endpoint.Name}
		}
	}
	if len(plan.Endpoints) == 1 {
		return []string{plan.Endpoints[0].Name}
	}
	return nil
}

func normalizeRecipePromptEndpointIDs(plan model.IntegrationPlan, requested, fallback []string) []string {
	allowed := make(map[string]bool, len(plan.Endpoints))
	for _, endpoint := range plan.Endpoints {
		allowed[endpoint.Name] = true
	}
	values := make([]string, 0, len(requested))
	seen := make(map[string]bool)
	for _, endpointID := range requested {
		endpointID = strings.TrimSpace(endpointID)
		if allowed[endpointID] && !seen[endpointID] {
			values, seen[endpointID] = append(values, endpointID), true
		}
	}
	if len(values) > 0 {
		return values
	}
	return append([]string(nil), fallback...)
}

func recipeBriefResponseSeed(response recipeBriefAIResponse, plan model.IntegrationPlan, evidence []model.IntegrationEvidence) (model.RecipeSeed, bool) {
	if response.Status != "ready" || len(response.Gaps) != 0 || !allowedUniqueEvidenceIDs(response.EvidenceIDs, evidence) {
		return model.RecipeSeed{}, false
	}
	seed := model.RecipeSeed{
		Slug:        slugify(response.Slug),
		Title:       strings.TrimSpace(response.Title),
		Outcome:     strings.TrimSpace(response.Outcome),
		Audience:    strings.TrimSpace(response.Audience),
		EndpointIDs: append([]string(nil), response.EndpointIDs...),
		EvidenceIDs: append([]string(nil), response.EvidenceIDs...),
	}
	if seed.Slug == "" || seed.Title == "" || len(seed.Title) > 160 || seed.Outcome == "" || len(seed.Outcome) > 1000 || seed.Audience == "" || len(seed.Audience) > 80 || containsToolBuilderSecretText(seed.Title+" "+seed.Outcome+" "+seed.Audience) {
		return model.RecipeSeed{}, false
	}
	allowedEndpoints := make(map[string]model.IntegrationEndpointPlan, len(plan.Endpoints))
	for _, endpoint := range plan.Endpoints {
		allowedEndpoints[endpoint.Name] = endpoint
	}
	seenEndpointIDs := make(map[string]bool, len(seed.EndpointIDs))
	endpointEvidence := make(map[string]bool)
	for index, endpointID := range seed.EndpointIDs {
		endpointID = strings.TrimSpace(endpointID)
		endpoint, ok := allowedEndpoints[endpointID]
		if !ok || seenEndpointIDs[endpointID] {
			return model.RecipeSeed{}, false
		}
		seed.EndpointIDs[index] = endpointID
		seenEndpointIDs[endpointID] = true
		for _, evidenceID := range endpoint.Evidence {
			endpointEvidence[evidenceID] = true
		}
	}
	if len(seed.EndpointIDs) == 0 {
		return model.RecipeSeed{}, false
	}
	for index, evidenceID := range seed.EvidenceIDs {
		evidenceID = strings.TrimSpace(evidenceID)
		if !endpointEvidence[evidenceID] {
			return model.RecipeSeed{}, false
		}
		seed.EvidenceIDs[index] = evidenceID
	}
	return seed, true
}

func validRecipeBriefGaps(response recipeBriefAIResponse) bool {
	if response.Status != "needs_input" || len(response.Gaps) == 0 || len(response.EndpointIDs) != 0 || len(response.EvidenceIDs) != 0 {
		return false
	}
	for _, gap := range response.Gaps {
		gap = strings.TrimSpace(gap)
		if gap == "" || len(gap) > 500 || containsToolBuilderSecretText(gap) {
			return false
		}
	}
	return true
}

// CreateRecipeFromPromptFor keeps the simple prompt-first flow while allowing
// the console to ground a recipe in one exact API when the administrator selects
// it. Without a selection, the existing deployment-wide behavior is preserved.
func (s *Service) CreateRecipeFromPromptFor(ctx context.Context, productID, integrationID, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len(instruction) > 4000 {
		return recipe, errors.New("describe the recipe in 1 to 4,000 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	var analysis model.IntegrationAnalysis
	if strings.TrimSpace(integrationID) == "" {
		analysis, err = s.AnalyseIntegration(ctx, productID, actor)
	} else {
		analysis, err = s.AnalyseIntegrationFor(ctx, productID, integrationID, actor)
	}
	if err != nil {
		return recipe, err
	}
	for _, unknown := range analysis.Unknowns {
		if unknown.Blocking {
			return recipe, ErrRecipeNeedsInput
		}
	}
	prompt, _ := json.Marshal(map[string]any{"request": instruction, "product": map[string]string{"name": product.Name, "description": product.Description}, "platform_contract": analysis.Plan, "evidence": analysis.Evidence, "allowed_endpoint_ids": integrationEndpointIDs(analysis.Plan), "allowed_evidence_ids": evidenceIDs(analysis.Evidence)})
	result, aiErr := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_brief", PromptKey: AIPromptKeyRecipeBrief, User: string(prompt), SchemaName: "recipe_brief", Schema: recipeBriefSchema, MaxOutput: 2048, Temperature: 0.1})
	if aiErr != nil {
		return recipe, ErrRecipeNeedsInput
	}
	var response recipeBriefAIResponse
	if decodeStrictAIResult(result.JSON, &response) != nil {
		return recipe, ErrRecipeNeedsInput
	}
	seed, valid := recipeBriefResponseSeed(response, analysis.Plan, analysis.Evidence)
	if !valid {
		return recipe, ErrRecipeNeedsInput
	}
	recipe, runErr = s.createRecipeFromSeed(ctx, product, analysis, seed, instruction, actor)
	if runErr == nil {
		if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipe.created", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"analysis_id": analysis.ID, "generated": true}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
			return model.Recipe{}, err
		}
	}
	return recipe, runErr
}

func (s *Service) GenerateRecipes(ctx context.Context, productID, analysisID string, actor Actor) ([]model.Recipe, error) {
	return s.generateRecipes(ctx, productID, analysisID, "", actor)
}

func (s *Service) GenerateRecipesForIntegration(ctx context.Context, productID, analysisID, integrationID string, actor Actor) ([]model.Recipe, error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return nil, errors.New("integration_id is required for integration-scoped recipe generation")
	}
	return s.generateRecipes(ctx, productID, analysisID, integrationID, actor)
}

func (s *Service) generateRecipes(ctx context.Context, productID, analysisID, integrationID string, actor Actor) (recipes []model.Recipe, runErr error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return nil, err
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, analysisID)
	if err != nil {
		return nil, err
	}
	if integrationID != "" {
		analysisIntegrationID, scoped := integrationScopeID(analysis.Evidence)
		if !scoped || analysisIntegrationID != integrationID {
			return nil, errors.New("analysis is not scoped to the selected integration")
		}
		currentEvidence, _, evidenceErr := s.scopedIntegrationEvidence(ctx, product, integrationID)
		if evidenceErr != nil {
			return nil, evidenceErr
		}
		if !recipeDependencySetsMatch(recipeDependencies(analysis.Evidence), recipeDependencies(currentEvidence)) {
			return nil, errors.New("analysis evidence no longer matches the selected integration; analyse it again before generating recipes")
		}
	}
	for _, unknown := range analysis.Unknowns {
		if unknown.Blocking {
			return nil, ErrRecipeNeedsInput
		}
	}
	existingBySlug := make(map[string]model.Recipe, len(analysis.Plan.Recipes))
	allExisting := len(analysis.Plan.Recipes) > 0
	for _, seed := range analysis.Plan.Recipes {
		existing, lookupErr := s.store.RecipeBySlug(ctx, productID, seed.Slug)
		switch {
		case lookupErr == nil:
			existingBySlug[seed.Slug] = existing
			if existing.State == "outdated" || !recipeGroundingMatches(existing, analysis, seed) {
				allExisting = false
			}
		case errors.Is(lookupErr, store.ErrNotFound):
			allExisting = false
		default:
			return nil, lookupErr
		}
	}
	if allExisting {
		for _, seed := range analysis.Plan.Recipes {
			recipes = append(recipes, existingBySlug[seed.Slug])
		}
		return recipes, nil
	}
	for _, seed := range analysis.Plan.Recipes {
		if existing, ok := existingBySlug[seed.Slug]; ok {
			if existing.State == "outdated" || !recipeGroundingMatches(existing, analysis, seed) {
				existing.AnalysisID = analysis.ID
				existing.Title = seed.Title
				existing.Outcome = seed.Outcome
				existing.Audience = seed.Audience
				existing.Dependencies = recipeGroundingDependencies(analysis, seed)
				markdown, references, generatedBy, modelID := s.authorRecipe(ctx, product, analysis, seed, "")
				regrounded, refreshErr := s.createRecipeRevision(ctx, product, existing, markdown, references, generatedBy, modelID, "", actor)
				if refreshErr != nil {
					if errors.Is(refreshErr, store.ErrConflict) {
						winner, lookupErr := s.store.RecipeBySlug(ctx, productID, seed.Slug)
						if lookupErr == nil && recipeGroundingMatches(winner, analysis, seed) {
							recipes = append(recipes, winner)
							continue
						}
					}
					return recipes, refreshErr
				}
				recipes = append(recipes, regrounded)
				if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipe.regrounded", TargetType: "recipe", TargetID: regrounded.ID, Current: map[string]any{"analysis_id": analysis.ID, "revision": regrounded.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
					return nil, err
				}
				continue
			}
			// Generation is idempotent. Return the already-grounded recipe so the
			// console never reports that zero recipes were generated merely because
			// the same reviewed analysis was submitted twice.
			recipes = append(recipes, existing)
			continue
		}
		recipe, err := s.createRecipeFromSeed(ctx, product, analysis, seed, "", actor)
		if err != nil {
			return recipes, err
		}
		recipes = append(recipes, recipe)
	}
	current := map[string]any{"recipe_count": len(recipes)}
	if integrationID != "" {
		current["integration_id"] = integrationID
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipes.generated", TargetType: "integration_analysis", TargetID: analysis.ID, Current: current, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return nil, err
	}
	return recipes, nil
}

func (s *Service) createRecipeRevision(ctx context.Context, product model.Product, recipe model.Recipe, markdown string, references []model.RecipeReference, generatedBy, modelID, review string, actor Actor) (model.Recipe, error) {
	analysis, err := s.store.IntegrationAnalysis(ctx, product.ID, recipe.AnalysisID)
	if err != nil {
		return recipe, err
	}
	selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	findings := validateRecipeMarkdown(markdown, recipe.Title, references, recipeGroundedURLs(recipeAnalysisWithEvidence(analysis, selectedEvidence))...)
	if review == "" {
		review, findings = s.reviewRecipe(ctx, product, recipe, markdown, findings)
	}
	id, err := randomUUID()
	if err != nil {
		return recipe, err
	}
	recipe.State, recipe.NeedsAttention = "review", true
	recipe.ApprovedAt, recipe.ApprovedBy, recipe.PublishedAt = nil, "", nil
	recipe, err = s.store.SaveRecipeRevision(ctx, recipe, model.RecipeRevision{ID: id, RecipeID: recipe.ID, Markdown: markdown, References: references, Validation: findings, Review: review, GeneratedBy: generatedBy, Model: modelID, CreatedBy: actor.ID}, recipe.Revision)
	if err != nil {
		return recipe, err
	}
	return s.requireCurrentRecipeGrounding(ctx, product, recipe)
}

func (s *Service) ReworkRecipe(ctx context.Context, productID, recipeID, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len(instruction) > 4000 {
		return recipe, errors.New("describe the recipe change in 1 to 4,000 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	recipe, err = s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, recipe.AnalysisID)
	if err != nil {
		return recipe, err
	}
	fallbackEndpointIDs := recipePromptEndpointIDs(analysis.Plan, recipe.Outcome+" "+instruction)
	selectedEvidenceIDs, ok := recipeEvidenceIDsForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	seed := model.RecipeSeed{Slug: recipe.Slug, Title: recipe.Title, Outcome: recipe.Outcome, Audience: recipe.Audience, EndpointIDs: fallbackEndpointIDs, EvidenceIDs: selectedEvidenceIDs}
	for _, candidate := range analysis.Plan.Recipes {
		if candidate.Slug == recipe.Slug {
			seed.EndpointIDs = normalizeRecipePromptEndpointIDs(analysis.Plan, candidate.EndpointIDs, fallbackEndpointIDs)
			break
		}
	}
	markdown, references, generatedBy, modelID := s.authorRecipe(ctx, product, analysis, seed, instruction)
	recipe, runErr = s.createRecipeRevision(ctx, product, recipe, markdown, references, generatedBy, modelID, "", actor)
	return recipe, runErr
}

func (s *Service) UpdateRecipeMarkdown(ctx context.Context, productID, recipeID, markdown string, references []model.RecipeReference, visibility model.Visibility, actor Actor) (model.Recipe, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Recipe{}, err
	}
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	if visibility != model.VisibilityPrivate && visibility != model.VisibilityPublic {
		return recipe, errors.New("recipe visibility must be public or private")
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, recipe.AnalysisID)
	if err != nil {
		return recipe, err
	}
	selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	allowed := recipeReferences(selectedEvidence)
	allowedByURL := make(map[string]model.RecipeReference, len(allowed))
	for _, reference := range allowed {
		allowedByURL[reference.URL] = reference
	}
	cleanReferences := make([]model.RecipeReference, 0, len(references))
	for _, reference := range references {
		if known, ok := allowedByURL[reference.URL]; ok {
			cleanReferences = append(cleanReferences, known)
		} else {
			return recipe, errors.New("recipe references must select an existing analysed source")
		}
	}
	recipe.Visibility = visibility
	return s.createRecipeRevision(ctx, product, recipe, markdown, cleanReferences, "human", "", "", actor)
}

var ErrRecipeGroundingChanged = errors.New("recipe evidence changed; analyse and regenerate the recipe before continuing")

var errPublicRecipeEvidence = errors.New("public recipes can only depend on public evidence and reference published, non-quarantined public sources")

func recipeGroundingScopeID(dependencies []model.RecipeDependency) (string, bool) {
	scopeID := ""
	found := false
	for _, dependency := range dependencies {
		if dependency.Kind != integrationScopeEvidenceKind {
			continue
		}
		candidate := strings.TrimSpace(dependency.ResourceID)
		if found || candidate == "" {
			return "", false
		}
		scopeID, found = candidate, true
	}
	return scopeID, true
}

func recipeDependenciesMatchEvidence(dependencies []model.RecipeDependency, evidence []model.IntegrationEvidence) bool {
	_, ok := recipeEvidenceForDependencies(evidence, dependencies)
	return ok
}

func (s *Service) currentRecipeEvidence(ctx context.Context, product model.Product, recipe model.Recipe, evidenceByScope map[string][]model.IntegrationEvidence) ([]model.IntegrationEvidence, bool, error) {
	scopeID, valid := recipeGroundingScopeID(recipe.Dependencies)
	if !valid {
		return nil, false, nil
	}
	// A selected-evidence recipe does not need to select the structural scope
	// record. Recover that scope from its immutable analysis binding so scoped
	// recipes never fall back to deployment-wide evidence.
	if scopeID == "" && recipe.AnalysisID != "" {
		analysis, err := s.store.IntegrationAnalysis(ctx, product.ID, recipe.AnalysisID)
		if err == nil {
			if analysisScopeID, scoped := integrationScopeID(analysis.Evidence); scoped {
				scopeID = analysisScopeID
			}
		} else if errors.Is(err, store.ErrNotFound) {
			return nil, false, nil
		} else {
			return nil, false, err
		}
	}
	if evidenceByScope != nil {
		if evidence, ok := evidenceByScope[scopeID]; ok {
			return evidence, true, nil
		}
	}
	var evidence []model.IntegrationEvidence
	var err error
	if scopeID == "" {
		evidence, err = s.integrationEvidence(ctx, product)
	} else {
		evidence, _, err = s.scopedIntegrationEvidence(ctx, product, scopeID)
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if evidenceByScope != nil {
		evidenceByScope[scopeID] = evidence
	}
	return evidence, true, nil
}

// recipeGroundingCurrent resolves the same exact evidence set used by drift
// reconciliation and synchronous recipe state transitions. A cache is optional
// and is used only by the bulk reconciliation path.
func (s *Service) recipeGroundingCurrent(ctx context.Context, product model.Product, recipe model.Recipe, evidenceByScope map[string][]model.IntegrationEvidence) (bool, error) {
	evidence, resolved, err := s.currentRecipeEvidence(ctx, product, recipe, evidenceByScope)
	if err != nil || !resolved {
		return false, err
	}
	return recipeDependenciesMatchEvidence(recipe.Dependencies, evidence), nil
}

func (s *Service) markRecipeOutdated(ctx context.Context, recipe model.Recipe) (model.Recipe, error) {
	if recipe.State == "outdated" && recipe.NeedsAttention && recipe.ApprovedAt == nil && recipe.ApprovedBy == "" && recipe.PublishedAt == nil {
		return recipe, nil
	}
	recipe.State, recipe.NeedsAttention = "outdated", true
	recipe.ApprovedAt, recipe.ApprovedBy, recipe.PublishedAt = nil, "", nil
	return s.store.SaveRecipe(ctx, recipe, recipe.Revision)
}

func (s *Service) requireCurrentRecipeGrounding(ctx context.Context, product model.Product, recipe model.Recipe) (model.Recipe, error) {
	current, err := s.recipeGroundingCurrent(ctx, product, recipe, nil)
	if err != nil {
		return recipe, err
	}
	if current {
		return recipe, nil
	}
	recipe, err = s.markRecipeOutdated(ctx, recipe)
	if err != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, err)
	}
	return recipe, ErrRecipeGroundingChanged
}

func (s *Service) ApproveRecipe(ctx context.Context, productID, recipeID string, actor Actor) (model.Recipe, error) {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	if recipe.CurrentRevision == nil || hasRecipeErrors(recipe.CurrentRevision.Validation) {
		return recipe, errors.New("resolve blocking recipe findings before approval")
	}
	now := s.now()
	recipe.State, recipe.NeedsAttention, recipe.ApprovedBy, recipe.ApprovedAt = "approved", false, actor.ID, &now
	recipe, err = s.store.SaveRecipe(ctx, recipe, recipe.Revision)
	if err == nil {
		recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	}
	if err == nil {
		if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.approved", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"revision_id": recipe.CurrentRevisionID}, RequestID: actor.RequestID, CreatedAt: now}); err != nil {
			return model.Recipe{}, err
		}
	}
	return recipe, err
}

func (s *Service) PublishRecipe(ctx context.Context, productID, recipeID string, actor Actor) (model.Recipe, error) {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	if recipe.State != "approved" || recipe.CurrentRevision == nil {
		return recipe, errors.New("approve the current recipe revision before publishing")
	}
	if recipe.Visibility == model.VisibilityPublic {
		currentEvidence, resolved, evidenceErr := s.currentRecipeEvidence(ctx, product, recipe, nil)
		if evidenceErr != nil {
			return recipe, evidenceErr
		}
		selectedEvidence, selected := recipeEvidenceForDependencies(currentEvidence, recipe.Dependencies)
		if !resolved || !selected {
			return recipe, ErrRecipeGroundingChanged
		}
		for _, item := range selectedEvidence {
			if item.Visibility != model.VisibilityPublic {
				return recipe, errPublicRecipeEvidence
			}
		}
		sources, sourceErr := s.store.Sources(ctx, productID)
		if sourceErr != nil {
			return recipe, sourceErr
		}
		public := make(map[string]bool)
		for _, source := range sources {
			public[source.ID] = source.Visibility == model.VisibilityPublic && source.Published && !source.Quarantined
		}
		publicationIDs := evidenceSourcePublicationIDs(selectedEvidence)
		if len(publicationIDs) == 0 {
			publicationIDs, err = s.latestSourcePublicationIDs(ctx, productID)
			if err != nil {
				return recipe, err
			}
		}
		for _, publicationID := range publicationIDs {
			publication, publicationErr := s.store.SourcePublication(ctx, productID, publicationID)
			if publicationErr != nil {
				return recipe, publicationErr
			}
			public[publication.ID] = publication.Visibility == model.VisibilityPublic && public[publication.SourceID]
		}
		knowledge, knowledgeErr := s.store.PrivateKnowledge(ctx, productID, publicationIDs, "")
		if knowledgeErr != nil {
			return recipe, knowledgeErr
		}
		for _, record := range knowledge {
			public[record.ID] = record.Published && record.Visibility == model.VisibilityPublic && public[record.SourceID]
		}
		for _, item := range selectedEvidence {
			if (item.Kind == "source" || item.Kind == "source_publication") && !public[item.ResourceID] {
				return recipe, errPublicRecipeEvidence
			}
		}
		for _, reference := range recipe.CurrentRevision.References {
			if !public[reference.ResourceID] {
				return recipe, errPublicRecipeEvidence
			}
		}
	}
	now := s.now()
	recipe.State, recipe.PublishedAt, recipe.NeedsAttention = "published", &now, false
	recipe, err = s.store.SaveRecipe(ctx, recipe, recipe.Revision)
	if err == nil {
		recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	}
	if err == nil {
		if _, err := s.store.BumpProductCatalogRevision(ctx, productID); err != nil {
			return model.Recipe{}, err
		}
		if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.published", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"visibility": recipe.Visibility, "stable_uri": recipe.StableURI}, RequestID: actor.RequestID, CreatedAt: now}); err != nil {
			return model.Recipe{}, err
		}
	}
	return recipe, err
}

func (s *Service) ReconcileRecipeDrift(ctx context.Context, productID string) ([]model.Recipe, error) {
	recipes, err := s.store.Recipes(ctx, productID)
	if err != nil {
		return nil, err
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return nil, err
	}
	evidenceByScope := make(map[string][]model.IntegrationEvidence)
	for index := range recipes {
		current, err := s.recipeGroundingCurrent(ctx, product, recipes[index], evidenceByScope)
		if err != nil {
			return nil, err
		}
		if !current && (recipes[index].State != "outdated" || !recipes[index].NeedsAttention) {
			updated, saveErr := s.markRecipeOutdated(ctx, recipes[index])
			if saveErr != nil {
				return nil, saveErr
			}
			recipes[index] = updated
		}
	}
	return recipes, nil
}
