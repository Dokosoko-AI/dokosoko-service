package platform

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrRecipeNeedsInput = errors.New("the requested recipe is not supported by the current reviewed product evidence")
var ErrRecipeGroundingChanged = errors.New("recipe product evidence changed; analyse and regenerate the recipe before continuing")
var ErrRecipeAnalysisScope = errors.New("the analysis is not scoped to the requested integration")
var errPublicRecipeEvidence = errors.New("public recipes can only depend on public evidence and reference published, non-quarantined public sources")

func recipeProductIntentTextValid(title, outcome string) bool {
	if title == "" || title != strings.TrimSpace(title) || len(title) > 160 || outcome == "" || outcome != strings.TrimSpace(outcome) || len(outcome) > 1000 {
		return false
	}
	combined := title + " " + outcome
	if containsToolBuilderSecretText(combined) || containsRecipeRawHTML(combined) || recipeContainsURI(combined) {
		return false
	}
	lower := strings.ToLower(combined)
	if strings.Contains(lower, "dokosoko") {
		return false
	}
	for _, forbidden := range []string{"connect to dokosoko", "connect to mcp", "mcp transport", "mcp discovery", "protected-resource metadata", "pkce", "catalog revision", "publication revision"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

func recipeSeedTextValid(seed model.RecipeSeed) bool {
	if seed.Slug == "" || len(seed.CapabilityIDs) != 1 || len(seed.EvidenceIDs) == 0 || len(seed.EvidenceIDs) > 24 || !recipeProductIntentTextValid(seed.Title, seed.Outcome) {
		return false
	}
	return true
}

func recipeBriefResponseSeed(response recipeBriefAIResponse, analysis model.IntegrationAnalysis) (model.RecipeSeed, bool) {
	if response.Status != "ready" || len(response.Gaps) != 0 {
		return model.RecipeSeed{}, false
	}
	seed := model.RecipeSeed{
		CapabilityIDs: append([]string(nil), response.CapabilityIDs...),
		SDKID:         strings.TrimSpace(response.SDKID),
		EvidenceIDs:   append([]string(nil), response.EvidenceIDs...),
	}
	for index := range seed.CapabilityIDs {
		seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
	}
	for index := range seed.EvidenceIDs {
		seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
	}
	if len(seed.CapabilityIDs) != 1 || len(seed.EvidenceIDs) == 0 || len(seed.EvidenceIDs) > 24 {
		return model.RecipeSeed{}, false
	}
	selected, ok := recipeResolveProductSelection(analysis, seed)
	if !ok {
		return model.RecipeSeed{}, false
	}
	if _, valid := integrationRecipeSelection(analysis.Evidence, seed); !valid {
		return model.RecipeSeed{}, false
	}
	canonical, ok := recipeSeedForCapability(analysis.Plan.Recipes, seed.CapabilityIDs[0])
	if !ok {
		return model.RecipeSeed{}, false
	}
	canonical.SDKID = seed.SDKID
	canonical.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
	_ = selected
	return canonical, true
}

func appendUniqueRecipeEvidenceIDs(seed model.RecipeSeed, evidence []model.IntegrationEvidence) model.RecipeSeed {
	seen := make(map[string]bool, len(seed.EvidenceIDs))
	for _, id := range seed.EvidenceIDs {
		seen[id] = true
	}
	for _, item := range evidence {
		if item.Kind != "source_publication" || item.ResourceID == "" || seen[item.ResourceID] || len(seed.EvidenceIDs) == 24 {
			continue
		}
		seed.EvidenceIDs = append(seed.EvidenceIDs, item.ResourceID)
		seen[item.ResourceID] = true
	}
	return seed
}

func recipeAnalysisWithoutPublicationEvidence(analysis model.IntegrationAnalysis) model.IntegrationAnalysis {
	evidence := make([]model.IntegrationEvidence, 0, len(analysis.Evidence))
	for _, item := range analysis.Evidence {
		if item.Kind != "source_publication" {
			evidence = append(evidence, item)
		}
	}
	analysis.Evidence = evidence
	return analysis
}

// relevantRecipeAnalysis replaces broad documentation excerpts with the small,
// source-diverse set relevant to one concrete outcome. The publication ID and
// fingerprint remain the immutable grounding boundary; retrieved text is only
// a bounded view over that already-reviewed publication.
func (s *Service) relevantRecipeAnalysis(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, outcome string) (model.IntegrationAnalysis, error) {
	publicationEvidence := make(map[string]model.IntegrationEvidence)
	publicationIDs := make([]string, 0)
	for _, item := range analysis.Evidence {
		if item.Kind == "source_publication" && item.ResourceID != "" {
			publicationEvidence[item.ResourceID] = item
			publicationIDs = append(publicationIDs, item.ResourceID)
		}
	}
	if len(publicationIDs) == 0 {
		return analysis, nil
	}
	sort.Strings(publicationIDs)
	records, err := s.store.RelevantPrivateKnowledge(ctx, product.ID, publicationIDs, outcome, 12)
	if err != nil {
		return analysis, err
	}
	if len(records) == 0 {
		// Attached documentation is supporting evidence, not a mandatory
		// dependency for an otherwise exact product operation. Exclude unrelated
		// prose rather than widening the query or blocking a tool/schema-grounded
		// recipe. A seed that explicitly selected one of these publications will
		// still fail exact-selection validation below.
		return recipeAnalysisWithoutPublicationEvidence(analysis), nil
	}
	excerpts := integrationSourceExcerpts(records)
	publicationBySource := make(map[string]string, len(publicationIDs))
	publications := make(map[string]model.SourcePublication, len(publicationIDs))
	for _, publicationID := range publicationIDs {
		publication, lookupErr := s.store.SourcePublication(ctx, product.ID, publicationID)
		if lookupErr != nil {
			return analysis, lookupErr
		}
		if previous := publicationBySource[publication.SourceID]; previous != "" && previous != publication.ID {
			return analysis, ErrRecipeNeedsInput
		}
		publicationBySource[publication.SourceID] = publication.ID
		publications[publication.ID] = publication
	}
	selected := make(map[string]integrationSourceExcerpt)
	for sourceID, excerpt := range excerpts {
		if publicationID := publicationBySource[sourceID]; publicationID != "" && excerpt.Text != "" {
			selected[publicationID] = excerpt
		}
	}
	if len(selected) == 0 {
		return recipeAnalysisWithoutPublicationEvidence(analysis), nil
	}
	values := make([]model.IntegrationEvidence, 0, len(analysis.Evidence))
	for _, item := range analysis.Evidence {
		if item.Kind != "source_publication" {
			values = append(values, item)
			continue
		}
		excerpt, ok := selected[item.ResourceID]
		if !ok {
			continue
		}
		item.Excerpt = excerpt.Text
		item.References = excerpt.References
		values = append(values, item)
	}
	analysis.Evidence = values
	return analysis, nil
}

func (s *Service) prepareRecipeSeed(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed) (model.IntegrationAnalysis, model.RecipeSeed, error) {
	seed.SDKID = strings.TrimSpace(seed.SDKID)
	for index := range seed.CapabilityIDs {
		seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
	}
	for index := range seed.EvidenceIDs {
		seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
	}
	integrationID, scoped := integrationScopeID(analysis.Evidence)
	if !scoped || integrationID == "" || len(seed.CapabilityIDs) != 1 {
		return analysis, seed, ErrRecipeNeedsInput
	}
	integration, err := s.store.Integration(ctx, product.ID, integrationID)
	if err != nil {
		return analysis, seed, err
	}
	if integration.Lifecycle == "retired" {
		return analysis, seed, ErrRecipeNeedsInput
	}
	canonical, ok := recipeSeedForCapability(deterministicIntegrationRecipeSeeds(product, integration, analysis.Evidence), seed.CapabilityIDs[0])
	if !ok {
		return analysis, seed, ErrRecipeNeedsInput
	}
	canonical.SDKID = seed.SDKID
	canonical.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
	seed = canonical
	if !recipeSeedTextValid(seed) {
		return analysis, seed, ErrRecipeNeedsInput
	}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, seed.Outcome)
	if err != nil {
		return analysis, seed, err
	}
	seed = appendUniqueRecipeEvidenceIDs(seed, analysis.Evidence)
	selected, ok := recipeResolveProductSelection(analysis, seed)
	if !ok {
		return analysis, seed, ErrRecipeNeedsInput
	}
	allowed, _ := recipeUniqueEvidenceByID(selected)
	if !recipeIntentMatchesProductSelection(seed.Title, seed.Outcome, recipeSelectedCapabilitySupportsMCP(seed, allowed)) {
		return analysis, seed, ErrRecipeNeedsInput
	}
	return analysis, seed, nil
}

func (s *Service) currentPublishedRecipeIntegration(ctx context.Context, integrationID string) (model.IntegrationRevision, error) {
	status, err := s.IntegrationPublishStatus(ctx, integrationID)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	if !status.Ready || status.HasChanges || status.LatestRevision == nil || status.LatestRevision.State != "published" || status.LatestRevision.ManifestHash == "" || status.LatestRevision.ManifestHash != status.CurrentManifestHash {
		return model.IntegrationRevision{}, ErrRecipeNeedsInput
	}
	return *status.LatestRevision, nil
}

func recipeSpecJSON(spec model.RecipeSpec) (json.RawMessage, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func decodeRecipeSpec(revision *model.RecipeRevision) (model.RecipeSpec, error) {
	if revision == nil || revision.SpecVersion != model.RecipeSpecVersion2 || len(revision.Spec) == 0 {
		return model.RecipeSpec{}, errors.New("recipe revision has no product-integration v2 spec")
	}
	var spec model.RecipeSpec
	if err := strictJSON(revision.Spec, &spec); err != nil {
		return model.RecipeSpec{}, errors.New("recipe revision spec is invalid")
	}
	return spec, nil
}

func equalRecipeReferences(left, right []model.RecipeReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Service) createRecipeRevision(ctx context.Context, product model.Product, recipe model.Recipe, analysis model.IntegrationAnalysis, draft authoredRecipe, review string, actor Actor) (model.Recipe, error) {
	selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	if draft.Spec.SchemaVersion != model.RecipeSpecVersion2 || draft.Spec.IntegrationID != recipe.IntegrationID || draft.Spec.Title != recipe.Title || draft.Spec.Outcome != recipe.Outcome {
		return recipe, errors.New("recipe spec cannot change its integration, title, outcome, or schema")
	}
	references, referencesOK := selectRecipeReferences(draft.Spec.ReferenceIDs, recipeReferences(selectedEvidence))
	if !referencesOK || !equalRecipeReferences(references, draft.References) {
		return recipe, errors.New("recipe references must select exact reviewed product documents")
	}
	draft.References = references
	draft.Markdown = renderRecipeSpec(draft.Spec, draft.References)
	findings := validateRecipeSpec(draft.Spec, recipe, selectedEvidence)
	findings = append(findings, validateRecipeMarkdown(draft.Markdown, recipe.Title, draft.References, recipeGroundedURLs(recipeAnalysisWithEvidence(analysis, selectedEvidence))...)...)
	if hasRecipeErrors(findings) {
		return recipe, errors.New("recipe spec failed deterministic product-integration validation")
	}
	if review == "" {
		review, findings = s.reviewRecipe(ctx, product, draft.Spec, draft.Markdown, selectedEvidence, findings)
	}
	binding, err := s.currentPublishedRecipeIntegration(ctx, recipe.IntegrationID)
	if err != nil {
		return recipe, err
	}
	specJSON, err := recipeSpecJSON(draft.Spec)
	if err != nil {
		return recipe, err
	}
	revisionID, err := randomUUID()
	if err != nil {
		return recipe, err
	}
	recipe.State, recipe.NeedsAttention = "review", true
	recipe.ApprovedAt, recipe.ApprovedBy, recipe.PublishedAt = nil, "", nil
	value := model.RecipeRevision{
		ID:                      revisionID,
		RecipeID:                recipe.ID,
		SpecVersion:             model.RecipeSpecVersion2,
		Spec:                    specJSON,
		Markdown:                draft.Markdown,
		References:              draft.References,
		Validation:              findings,
		Review:                  review,
		GeneratedBy:             draft.GeneratedBy,
		Model:                   draft.Model,
		IntegrationRevisionID:   binding.ID,
		IntegrationManifestHash: binding.ManifestHash,
		PromptVersion:           draft.PromptVersion,
		PromptHash:              draft.PromptHash,
		CreatedBy:               actor.ID,
	}
	if recipe.Revision == 0 {
		recipe, err = s.store.CreateRecipeWithRevision(ctx, recipe, value)
	} else {
		recipe, err = s.store.SaveRecipeRevision(ctx, recipe, value, recipe.Revision)
	}
	if err != nil {
		return recipe, err
	}
	return s.requireCurrentRecipeGrounding(ctx, product, recipe)
}

func (s *Service) createPreparedRecipe(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string, actor Actor) (model.Recipe, error) {
	integrationID, scoped := integrationScopeID(analysis.Evidence)
	if !scoped || integrationID == "" {
		return model.Recipe{}, ErrRecipeNeedsInput
	}
	if _, err := s.currentPublishedRecipeIntegration(ctx, integrationID); err != nil {
		return model.Recipe{}, err
	}
	recipeID, err := randomUUID()
	if err != nil {
		return model.Recipe{}, err
	}
	if seed.Slug == "" {
		seed.Slug = "recipe"
	}
	if existing, lookupErr := s.store.RecipeBySlug(ctx, product.ID, seed.Slug); lookupErr == nil {
		if existing.IntegrationID == integrationID {
			return model.Recipe{}, store.ErrConflict
		}
		seed.Slug = scopedRecipeCollisionSlug(seed.Slug, integrationID, seed.CapabilityIDs[0])
		if _, collisionErr := s.store.RecipeBySlug(ctx, product.ID, seed.Slug); collisionErr == nil {
			return model.Recipe{}, store.ErrConflict
		} else if !errors.Is(collisionErr, store.ErrNotFound) {
			return model.Recipe{}, collisionErr
		}
	} else if !errors.Is(lookupErr, store.ErrNotFound) {
		return model.Recipe{}, lookupErr
	}
	draft, err := s.authorRecipe(ctx, product, analysis, seed, instruction)
	if err != nil {
		return model.Recipe{}, err
	}
	recipe := model.Recipe{
		ID:              recipeID,
		OrganisationID:  product.OrganisationID,
		ProductID:       product.ID,
		IntegrationID:   integrationID,
		AnalysisID:      analysis.ID,
		ContractVersion: model.RecipeContractProductIntegrationV2,
		Slug:            seed.Slug,
		Title:           seed.Title,
		Outcome:         seed.Outcome,
		Audience:        "coding_agent",
		State:           "draft",
		Generated:       true,
		NeedsAttention:  true,
		Visibility:      model.VisibilityPrivate,
		Dependencies:    recipeGroundingDependencies(analysis, seed),
		StableURI:       "dokosoko://products/" + product.Slug + "/recipes/" + seed.Slug,
	}
	if len(recipe.Dependencies) == 0 {
		return model.Recipe{}, ErrRecipeNeedsInput
	}
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", actor)
}

func scopedRecipeCollisionSlug(slug, integrationID, capabilityID string) string {
	suffix := evidenceFingerprint("recipe-integration-scope", integrationID, capabilityID)[:8]
	return boundedIntegrationRecipeSlug(slug, 151) + "-" + suffix
}

func (s *Service) createRecipeFromSeed(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string, actor Actor) (model.Recipe, error) {
	preparedAnalysis, preparedSeed, err := s.prepareRecipeSeed(ctx, product, analysis, seed)
	if err != nil {
		return model.Recipe{}, err
	}
	return s.createPreparedRecipe(ctx, product, preparedAnalysis, preparedSeed, instruction, actor)
}

func (s *Service) CreateRecipeFromPrompt(ctx context.Context, productID, instruction string, actor Actor) (model.Recipe, error) {
	return s.CreateRecipeFromPromptFor(ctx, productID, "", instruction, actor)
}

func (s *Service) resolveRecipeIntegrationID(ctx context.Context, productID, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested, nil
	}
	integrations, err := s.store.Integrations(ctx, productID)
	if err != nil {
		return "", err
	}
	eligible := make([]string, 0, len(integrations))
	for _, integration := range integrations {
		if integration.Lifecycle != "retired" {
			eligible = append(eligible, integration.ID)
		}
	}
	if len(eligible) != 1 {
		return "", ErrRecipeNeedsInput
	}
	return eligible[0], nil
}

func (s *Service) CreateRecipeFromPromptFor(ctx context.Context, productID, integrationID, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len(instruction) > 4000 || containsToolBuilderSecretText(instruction) {
		return recipe, errors.New("describe one non-secret product integration outcome in 1 to 4,000 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	integrationID, err = s.resolveRecipeIntegrationID(ctx, productID, integrationID)
	if err != nil {
		return recipe, err
	}
	analysis, err := s.AnalyseIntegrationFor(ctx, productID, integrationID, actor)
	if err != nil {
		return recipe, err
	}
	for _, unknown := range analysis.Unknowns {
		if unknown.Blocking {
			return recipe, ErrRecipeNeedsInput
		}
	}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, instruction)
	if err != nil {
		return recipe, err
	}
	productEvidence := recipeProductEvidence(analysis.Evidence)
	prompt, _ := json.Marshal(map[string]any{
		"request":                instruction,
		"product":                map[string]string{"name": product.Name, "description": product.Description},
		"product_evidence":       productEvidence,
		"allowed_capability_ids": recipeProductCapabilityIDs(productEvidence),
		"allowed_sdk_ids":        recipeProductSDKIDs(productEvidence),
		"allowed_evidence_ids":   evidenceIDs(productEvidence),
	})
	result, aiErr := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_brief", PromptKey: AIPromptKeyRecipeBrief, User: string(prompt), SchemaName: "recipe_brief", Schema: recipeBriefSchema, MaxOutput: 2048, Temperature: 0.1})
	if aiErr != nil {
		return recipe, ErrRecipeNeedsInput
	}
	var response recipeBriefAIResponse
	if decodeStrictAIResult(result.JSON, &response) != nil {
		return recipe, ErrRecipeNeedsInput
	}
	seed, valid := recipeBriefResponseSeed(response, analysis)
	if !valid {
		return recipe, ErrRecipeNeedsInput
	}
	recipe, runErr = s.createRecipeFromSeed(ctx, product, analysis, seed, instruction, actor)
	if runErr == nil {
		runErr = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipe.created", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"analysis_id": analysis.ID, "integration_id": integrationID, "contract_version": recipe.ContractVersion, "generated": true}, RequestID: actor.RequestID, CreatedAt: s.now()})
	}
	return recipe, runErr
}

func (s *Service) GenerateRecipes(ctx context.Context, productID, analysisID string, actor Actor) ([]model.Recipe, error) {
	return s.generateRecipes(ctx, productID, analysisID, "", actor)
}

func (s *Service) GenerateRecipesForIntegration(ctx context.Context, productID, analysisID, integrationID string, actor Actor) ([]model.Recipe, error) {
	if strings.TrimSpace(integrationID) == "" {
		return nil, errors.New("integration_id is required for integration-scoped recipe generation")
	}
	return s.generateRecipes(ctx, productID, analysisID, integrationID, actor)
}

func (s *Service) generateRecipes(ctx context.Context, productID, analysisID, requestedIntegrationID string, actor Actor) ([]model.Recipe, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return nil, err
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, analysisID)
	if err != nil {
		return nil, err
	}
	integrationID, scoped := integrationScopeID(analysis.Evidence)
	if !scoped || integrationID == "" {
		return nil, ErrRecipeNeedsInput
	}
	if requestedIntegrationID != "" && integrationID != strings.TrimSpace(requestedIntegrationID) {
		return nil, ErrRecipeAnalysisScope
	}
	currentEvidence, _, err := s.scopedIntegrationEvidence(ctx, product, integrationID)
	if err != nil {
		return nil, err
	}
	if !recipeDependencySetsMatch(recipeDependencies(recipeProductEvidence(analysis.Evidence)), recipeDependencies(recipeProductEvidence(currentEvidence))) {
		return nil, errors.New("analysis product evidence changed; analyse the integration again before generating recipes")
	}
	if _, err := s.currentPublishedRecipeIntegration(ctx, integrationID); err != nil {
		return nil, err
	}
	for _, unknown := range analysis.Unknowns {
		if unknown.Blocking {
			return nil, ErrRecipeNeedsInput
		}
	}
	if len(analysis.Plan.Recipes) == 0 {
		return nil, ErrRecipeNeedsInput
	}
	recipes := make([]model.Recipe, 0, len(analysis.Plan.Recipes))
	for _, rawSeed := range analysis.Plan.Recipes {
		preparedAnalysis, seed, prepareErr := s.prepareRecipeSeed(ctx, product, analysis, rawSeed)
		if prepareErr != nil {
			return recipes, prepareErr
		}
		existing, lookupErr := s.store.RecipeBySlug(ctx, productID, seed.Slug)
		if lookupErr == nil && existing.IntegrationID != integrationID {
			seed.Slug = scopedRecipeCollisionSlug(seed.Slug, integrationID, seed.CapabilityIDs[0])
			existing, lookupErr = s.store.RecipeBySlug(ctx, productID, seed.Slug)
			if lookupErr == nil && existing.IntegrationID != integrationID {
				return recipes, store.ErrConflict
			}
		}
		if errors.Is(lookupErr, store.ErrNotFound) {
			created, createErr := s.createPreparedRecipe(ctx, product, preparedAnalysis, seed, "", actor)
			if createErr != nil {
				return recipes, createErr
			}
			recipes = append(recipes, created)
			continue
		}
		if lookupErr != nil {
			return recipes, lookupErr
		}
		if existing.State != "outdated" && recipeGroundingMatches(existing, preparedAnalysis, seed) {
			recipes = append(recipes, existing)
			continue
		}
		draft, authorErr := s.authorRecipe(ctx, product, preparedAnalysis, seed, "")
		if authorErr != nil {
			return recipes, authorErr
		}
		existing.IntegrationID = integrationID
		existing.AnalysisID = analysis.ID
		existing.ContractVersion = model.RecipeContractProductIntegrationV2
		existing.Title = seed.Title
		existing.Outcome = seed.Outcome
		existing.Audience = "coding_agent"
		existing.Dependencies = recipeGroundingDependencies(preparedAnalysis, seed)
		regrounded, refreshErr := s.createRecipeRevision(ctx, product, existing, preparedAnalysis, draft, "", actor)
		if refreshErr != nil {
			if errors.Is(refreshErr, store.ErrConflict) {
				winner, winnerErr := s.store.RecipeBySlug(ctx, productID, seed.Slug)
				if winnerErr == nil && recipeGroundingMatches(winner, preparedAnalysis, seed) {
					recipes = append(recipes, winner)
					continue
				}
			}
			return recipes, refreshErr
		}
		recipes = append(recipes, regrounded)
		if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipe.regrounded", TargetType: "recipe", TargetID: regrounded.ID, Current: map[string]any{"analysis_id": analysis.ID, "integration_id": integrationID, "revision": regrounded.Revision}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
			return nil, err
		}
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "recipes.generated", TargetType: "integration_analysis", TargetID: analysis.ID, Current: map[string]any{"integration_id": integrationID, "recipe_count": len(recipes)}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return nil, err
	}
	return recipes, nil
}

func (s *Service) ReworkRecipe(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID, instruction string, actor Actor) (model.Recipe, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len(instruction) > 4000 || containsToolBuilderSecretText(instruction) {
		return model.Recipe{}, errors.New("describe a non-secret recipe change in 1 to 4,000 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Recipe{}, err
	}
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
		return recipe, err
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	spec, err := decodeRecipeSpec(recipe.CurrentRevision)
	if err != nil {
		return recipe, err
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, recipe.AnalysisID)
	if err != nil {
		return recipe, err
	}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, recipe.Outcome)
	if err != nil {
		return recipe, err
	}
	evidenceIDs, ok := recipeEvidenceIDsForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	seed := model.RecipeSeed{Slug: recipe.Slug, Title: recipe.Title, Outcome: recipe.Outcome, Audience: "coding_agent", CapabilityIDs: append([]string(nil), spec.CapabilityIDs...), SDKID: spec.SDKID, EvidenceIDs: evidenceIDs}
	draft, err := s.authorRecipe(ctx, product, analysis, seed, instruction)
	if err != nil {
		return recipe, err
	}
	recipe.Dependencies = recipeGroundingDependencies(analysis, seed)
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", actor)
}

// UpdateRecipeSpec is the only human edit path for v2. Markdown is a canonical
// rendering and therefore cannot become an independent, drifting source of
// truth.
func (s *Service) UpdateRecipeSpec(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string, spec model.RecipeSpec, visibility model.Visibility, actor Actor) (model.Recipe, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.Recipe{}, err
	}
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
		return recipe, err
	}
	if visibility != model.VisibilityPrivate && visibility != model.VisibilityPublic {
		return recipe, errors.New("recipe visibility must be public or private")
	}
	recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	if err != nil {
		return recipe, err
	}
	currentSpec, err := decodeRecipeSpec(recipe.CurrentRevision)
	if err != nil {
		return recipe, err
	}
	if spec.IntegrationID != currentSpec.IntegrationID || spec.Title != currentSpec.Title || spec.Outcome != currentSpec.Outcome || spec.SDKID != currentSpec.SDKID || !equalStrings(spec.CapabilityIDs, currentSpec.CapabilityIDs) {
		return recipe, errors.New("edit reference_ids or visibility only; integration, title, outcome, capability, and SDK are immutable")
	}
	analysis, err := s.store.IntegrationAnalysis(ctx, productID, recipe.AnalysisID)
	if err != nil {
		return recipe, err
	}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, recipe.Outcome)
	if err != nil {
		return recipe, err
	}
	selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	canonical, canonicalErr := canonicalRecipeInstructions(spec, selectedEvidence)
	if canonicalErr != nil || !equalRecipeInstructions(spec.Prerequisites, canonical.Prerequisites) || !equalRecipeInstructions(spec.Steps, canonical.Steps) || !equalRecipeInstructions(spec.Checks, canonical.Checks) {
		return recipe, errors.New("recipe instructions are derived from the reviewed operation; edit reference_ids or visibility only")
	}
	references, ok := selectRecipeReferences(spec.ReferenceIDs, recipeReferences(selectedEvidence))
	if !ok {
		return recipe, errors.New("recipe references must select exact reviewed product documents")
	}
	recipe.Visibility = visibility
	draft := authoredRecipe{Spec: spec, References: references, GeneratedBy: "human"}
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", actor)
}

func requireExpectedRecipeRevision(recipe model.Recipe, expectedRevision int64, expectedCurrentRevisionID string) error {
	expectedCurrentRevisionID = strings.TrimSpace(expectedCurrentRevisionID)
	if expectedRevision < 1 || expectedCurrentRevisionID == "" {
		return errors.New("recipe revision and current revision ID are required")
	}
	if recipe.Revision != expectedRevision || recipe.CurrentRevisionID != expectedCurrentRevisionID {
		return store.ErrConflict
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Service) currentRecipeEvidence(ctx context.Context, product model.Product, recipe model.Recipe, evidenceByScope map[string][]model.IntegrationEvidence) ([]model.IntegrationEvidence, bool, error) {
	if recipe.ContractVersion != model.RecipeContractProductIntegrationV2 || strings.TrimSpace(recipe.IntegrationID) == "" {
		return nil, false, nil
	}
	cacheKey := recipe.IntegrationID + "\x00" + recipe.Outcome
	if evidenceByScope != nil {
		if evidence, ok := evidenceByScope[cacheKey]; ok {
			return evidence, true, nil
		}
	}
	evidence, _, err := s.scopedIntegrationEvidence(ctx, product, recipe.IntegrationID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, ErrRecipeNeedsInput) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	analysis := model.IntegrationAnalysis{Evidence: evidence}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, recipe.Outcome)
	if errors.Is(err, ErrRecipeNeedsInput) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if evidenceByScope != nil {
		evidenceByScope[cacheKey] = analysis.Evidence
	}
	return analysis.Evidence, true, nil
}

func (s *Service) recipeGroundingCurrent(ctx context.Context, product model.Product, recipe model.Recipe, evidenceByScope map[string][]model.IntegrationEvidence) (bool, error) {
	currentEvidence, resolved, err := s.currentRecipeEvidence(ctx, product, recipe, evidenceByScope)
	if err != nil || !resolved || recipe.CurrentRevision == nil {
		return false, err
	}
	selectedEvidence, selected := recipeEvidenceForDependencies(currentEvidence, recipe.Dependencies)
	if !selected {
		return false, nil
	}
	binding, bindingErr := s.currentPublishedRecipeIntegration(ctx, recipe.IntegrationID)
	if bindingErr != nil {
		if errors.Is(bindingErr, ErrRecipeNeedsInput) || errors.Is(bindingErr, store.ErrNotFound) {
			return false, nil
		}
		return false, bindingErr
	}
	if recipe.CurrentRevision.IntegrationRevisionID != binding.ID || recipe.CurrentRevision.IntegrationManifestHash != binding.ManifestHash {
		return false, nil
	}
	spec, specErr := decodeRecipeSpec(recipe.CurrentRevision)
	if specErr != nil {
		return false, nil
	}
	if !recipeDependenciesMatchCurrentContract(recipe, spec, selectedEvidence) {
		return false, nil
	}
	references, referencesOK := selectRecipeReferences(spec.ReferenceIDs, recipeReferences(selectedEvidence))
	if !referencesOK || !equalRecipeReferences(references, recipe.CurrentRevision.References) {
		return false, nil
	}
	findings := validateRecipeSpec(spec, recipe, selectedEvidence)
	canonical := renderRecipeSpec(spec, references)
	if canonical != recipe.CurrentRevision.Markdown {
		return false, nil
	}
	findings = append(findings, validateRecipeMarkdown(canonical, recipe.Title, references, recipeGroundedURLs(model.IntegrationAnalysis{Evidence: selectedEvidence})...)...)
	if hasRecipeErrors(findings) {
		return false, nil
	}
	if recipe.State == "published" && recipe.Visibility == model.VisibilityPublic {
		if err := s.validatePublicRecipeEvidence(ctx, recipe.ProductID, recipe, selectedEvidence); err != nil {
			if errors.Is(err, errPublicRecipeEvidence) || errors.Is(err, store.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
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
	recipe, saveErr := s.markRecipeOutdated(ctx, recipe)
	if saveErr != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, saveErr)
	}
	return recipe, ErrRecipeGroundingChanged
}

func (s *Service) ApproveRecipe(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string, actor Actor) (model.Recipe, error) {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
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
	if recipe.State != "review" {
		return recipe, errors.New("only a recipe revision in review can be approved")
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
		err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.approved", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"revision_id": recipe.CurrentRevisionID, "integration_revision_id": recipe.CurrentRevision.IntegrationRevisionID}, RequestID: actor.RequestID, CreatedAt: now})
	}
	return recipe, err
}

func (s *Service) validatePublicRecipeEvidence(ctx context.Context, productID string, recipe model.Recipe, selectedEvidence []model.IntegrationEvidence) error {
	for _, item := range selectedEvidence {
		if item.Visibility != model.VisibilityPublic {
			return errPublicRecipeEvidence
		}
	}
	sources, err := s.store.Sources(ctx, productID)
	if err != nil {
		return err
	}
	public := make(map[string]bool)
	for _, source := range sources {
		public[source.ID] = source.Visibility == model.VisibilityPublic && source.Published && !source.Quarantined
	}
	publicationIDs := evidenceSourcePublicationIDs(selectedEvidence)
	for _, publicationID := range publicationIDs {
		publication, lookupErr := s.store.SourcePublication(ctx, productID, publicationID)
		if lookupErr != nil {
			return lookupErr
		}
		public[publication.ID] = publication.Visibility == model.VisibilityPublic && public[publication.SourceID]
	}
	if len(publicationIDs) > 0 {
		knowledge, knowledgeErr := s.store.PrivateKnowledge(ctx, productID, publicationIDs, "")
		if knowledgeErr != nil {
			return knowledgeErr
		}
		for _, record := range knowledge {
			public[record.ID] = record.Published && record.Visibility == model.VisibilityPublic && public[record.SourceID]
		}
	}
	for _, item := range selectedEvidence {
		if item.Kind == "source_publication" && !public[item.ResourceID] {
			return errPublicRecipeEvidence
		}
	}
	for _, reference := range recipe.CurrentRevision.References {
		if !public[reference.ResourceID] {
			return errPublicRecipeEvidence
		}
	}
	return nil
}

func (s *Service) PublishRecipe(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string, actor Actor) (model.Recipe, error) {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
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
		if err := s.validatePublicRecipeEvidence(ctx, productID, recipe, selectedEvidence); err != nil {
			return recipe, err
		}
	}
	now := s.now()
	recipe.State, recipe.PublishedAt, recipe.NeedsAttention = "published", &now, false
	recipe, err = s.store.SaveRecipe(ctx, recipe, recipe.Revision)
	if err == nil {
		recipe, err = s.requireCurrentRecipeGrounding(ctx, product, recipe)
	}
	if err == nil {
		if _, bumpErr := s.store.BumpProductCatalogRevision(ctx, productID); bumpErr != nil {
			return model.Recipe{}, bumpErr
		}
		err = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.published", TargetType: "recipe", TargetID: recipe.ID, Current: map[string]any{"visibility": recipe.Visibility, "stable_uri": recipe.StableURI, "contract_version": recipe.ContractVersion}, RequestID: actor.RequestID, CreatedAt: now})
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
		current, currentErr := s.recipeGroundingCurrent(ctx, product, recipes[index], evidenceByScope)
		if currentErr != nil {
			return nil, currentErr
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
