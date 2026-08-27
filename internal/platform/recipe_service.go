package platform

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrRecipeNeedsInput = errors.New("the requested recipe is not supported by the current reviewed product evidence")
var ErrRecipeGroundingChanged = errors.New("recipe product evidence changed; analyse and regenerate the recipe before continuing")
var ErrRecipeAnalysisScope = errors.New("the analysis is not scoped to the requested integration")
var ErrPublicMCPRecipe = errors.New("MCP-backed product recipes are private-only until public custom-tool exposure is supported")
var ErrRecipeDeletionNotAllowed = errors.New("only legacy or outdated recipes can be deleted")
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
	if seed.Slug == "" || len(seed.CapabilityIDs) < 1 || len(seed.CapabilityIDs) > 4 || len(seed.EvidenceIDs) == 0 || len(seed.EvidenceIDs) > 48 || !recipeProductIntentTextValid(seed.Title, seed.Outcome) {
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
		EvidenceIDs:   append([]string(nil), response.EvidenceIDs...),
	}
	for index := range seed.CapabilityIDs {
		seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
	}
	for index := range seed.EvidenceIDs {
		seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
	}
	if len(seed.CapabilityIDs) < 1 || len(seed.CapabilityIDs) > 4 || len(seed.EvidenceIDs) == 0 || len(seed.EvidenceIDs) > 48 {
		return model.RecipeSeed{}, false
	}
	selected, ok := recipeResolveProductSelection(analysis, seed)
	if !ok {
		return model.RecipeSeed{}, false
	}
	if len(seed.CapabilityIDs) == 1 {
		canonical, ok := recipeSeedForCapability(analysis.Plan.Recipes, seed.CapabilityIDs[0])
		if !ok {
			return model.RecipeSeed{}, false
		}
		canonical.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
		return canonical, true
	}
	canonical, ok := multiCapabilityRecipeSeed(seed, selected)
	if !ok {
		return model.RecipeSeed{}, false
	}
	return canonical, true
}

func multiCapabilityRecipeSeed(seed model.RecipeSeed, selected []model.IntegrationEvidence) (model.RecipeSeed, bool) {
	byID, ambiguous := recipeUniqueEvidenceByID(selected)
	capabilityIDs := append([]string(nil), seed.CapabilityIDs...)
	sort.Strings(capabilityIDs)
	labels := make([]string, 0, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		item, ok := byID[capabilityID]
		if !ok || ambiguous[capabilityID] || recipeCapabilityIntegrationID(item) == "" {
			return model.RecipeSeed{}, false
		}
		labels = append(labels, evidenceText(item.Label))
	}
	title := truncateRunes("Implement "+strings.Join(labels, " and "), 160)
	slug := boundedIntegrationRecipeSlug("workflow-"+strings.Join(labels, "-"), 151)
	if slug == "" {
		slug = "workflow-" + evidenceFingerprint(strings.Join(capabilityIDs, "\x00"))[:8]
	}
	return model.RecipeSeed{
		Slug:          slug,
		Title:         title,
		Outcome:       truncateRunes("The consuming project implements and verifies one reviewed workflow spanning "+strings.Join(labels, ", ")+".", 1000),
		Audience:      "coding_agent",
		CapabilityIDs: capabilityIDs,
		EvidenceIDs:   append([]string(nil), seed.EvidenceIDs...),
	}, true
}

func appendUniqueRecipeEvidenceIDs(seed model.RecipeSeed, evidence []model.IntegrationEvidence) model.RecipeSeed {
	seen := make(map[string]bool, len(seed.EvidenceIDs))
	for _, id := range seed.EvidenceIDs {
		seen[id] = true
	}
	for _, item := range evidence {
		supporting := item.Kind == "integration" || item.Kind == "source_publication" || recipeDeveloperAssetSupportingKind(item.Kind)
		if !supporting || item.ResourceID == "" || seen[item.ResourceID] || len(seed.EvidenceIDs) == 48 {
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
	var err error
	analysis, err = s.relevantRecipeDeveloperAssetAnalysis(ctx, product, analysis, outcome)
	if err != nil {
		return analysis, err
	}
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
	if seed.SDKID != "" {
		// SDK membership in an Integration does not prove that the SDK exposes
		// this exact operation. Recipes may use an SDK only after the product
		// model records a reviewed SDK-to-capability binding.
		return analysis, seed, ErrRecipeNeedsInput
	}
	for index := range seed.CapabilityIDs {
		seed.CapabilityIDs[index] = strings.TrimSpace(seed.CapabilityIDs[index])
	}
	for index := range seed.EvidenceIDs {
		seed.EvidenceIDs[index] = strings.TrimSpace(seed.EvidenceIDs[index])
	}
	integrationIDs := integrationScopeIDs(analysis.Evidence)
	if len(integrationIDs) == 0 || len(seed.CapabilityIDs) < 1 || len(seed.CapabilityIDs) > 4 {
		return analysis, seed, ErrRecipeNeedsInput
	}
	attached := make(map[string]model.Integration, len(integrationIDs))
	for _, integrationID := range integrationIDs {
		integration, err := s.store.Integration(ctx, product.ID, integrationID)
		if err != nil {
			return analysis, seed, err
		}
		if integration.Lifecycle == "retired" {
			return analysis, seed, ErrRecipeNeedsInput
		}
		attached[integrationID] = integration
	}
	byID, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(analysis.Evidence))
	for _, capabilityID := range seed.CapabilityIDs {
		item, ok := byID[capabilityID]
		ownerID := recipeCapabilityIntegrationID(item)
		if !ok || ambiguous[capabilityID] || ownerID == "" || !integrationRecipeCapabilityViable(item, ownerID) {
			return analysis, seed, ErrRecipeNeedsInput
		}
		if _, ok := attached[ownerID]; !ok {
			return analysis, seed, ErrRecipeNeedsInput
		}
	}
	if len(seed.CapabilityIDs) == 1 && len(integrationIDs) == 1 {
		canonical, ok := recipeSeedForCapability(deterministicIntegrationRecipeSeeds(product, attached[integrationIDs[0]], analysis.Evidence), seed.CapabilityIDs[0])
		if !ok {
			return analysis, seed, ErrRecipeNeedsInput
		}
		canonical.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
		seed = canonical
	} else {
		selected, ok := recipeResolveProductSelection(analysis, seed)
		if !ok {
			return analysis, seed, ErrRecipeNeedsInput
		}
		seed, ok = multiCapabilityRecipeSeed(seed, selected)
		if !ok {
			return analysis, seed, ErrRecipeNeedsInput
		}
	}
	if !recipeSeedTextValid(seed) {
		return analysis, seed, ErrRecipeNeedsInput
	}
	selectedDeveloperAssets := selectedRecipeDeveloperAssetEvidence(analysis.Evidence, seed.EvidenceIDs)
	if len(integrationIDs) == 1 {
		var err error
		analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, seed.Outcome)
		if err != nil {
			return analysis, seed, err
		}
	}
	analysis.Evidence = prioritizeRecipeDeveloperAssetEvidence(analysis.Evidence, selectedDeveloperAssets)
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

func (s *Service) currentPublishedRecipeAPIBindings(ctx context.Context, integrationIDs []string) ([]model.RecipeAPIBinding, error) {
	values := make([]model.RecipeAPIBinding, 0, len(integrationIDs))
	for _, integrationID := range integrationIDs {
		revision, err := s.currentPublishedRecipeIntegration(ctx, integrationID)
		if err != nil {
			return nil, err
		}
		values = append(values, model.RecipeAPIBinding{IntegrationID: integrationID, IntegrationRevisionID: revision.ID, IntegrationManifestHash: revision.ManifestHash})
	}
	return values, nil
}

func recipeSpecJSON(spec model.RecipeSpec) (json.RawMessage, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func decodeRecipeSpec(revision *model.RecipeRevision) (model.RecipeSpec, error) {
	if revision == nil || (revision.SpecVersion != model.RecipeSpecVersion2 && revision.SpecVersion != model.RecipeSpecVersion3) || len(revision.Spec) == 0 {
		return model.RecipeSpec{}, errors.New("recipe revision has no supported structured spec")
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

func (s *Service) createRecipeRevision(ctx context.Context, product model.Product, recipe model.Recipe, analysis model.IntegrationAnalysis, draft authoredRecipe, review, auditAction string, actor Actor) (model.Recipe, error) {
	selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return recipe, ErrRecipeGroundingChanged
	}
	if recipe.Visibility == model.VisibilityPublic && recipeEvidenceUsesMCP(selectedEvidence) {
		return recipe, ErrPublicMCPRecipe
	}
	expectedSpecVersion := model.RecipeSpecVersion3
	if recipe.ContractVersion == model.RecipeContractProductIntegrationV2 {
		expectedSpecVersion = model.RecipeSpecVersion2
	}
	if draft.Spec.SchemaVersion != expectedSpecVersion || !slices.Equal(recipeSpecIntegrationIDs(draft.Spec), recipeIntegrationIDs(recipe)) || draft.Spec.Title != recipe.Title || draft.Spec.Outcome != recipe.Outcome {
		return recipe, errors.New("recipe spec cannot change its API attachments, title, outcome, or schema")
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
	bindings, err := s.currentPublishedRecipeAPIBindings(ctx, recipeIntegrationIDs(recipe))
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
		ID:            revisionID,
		RecipeID:      recipe.ID,
		SpecVersion:   expectedSpecVersion,
		Spec:          specJSON,
		Markdown:      draft.Markdown,
		References:    draft.References,
		Validation:    findings,
		Review:        review,
		GeneratedBy:   draft.GeneratedBy,
		Model:         draft.Model,
		APIBindings:   bindings,
		PromptVersion: draft.PromptVersion,
		PromptHash:    draft.PromptHash,
		CreatedBy:     actor.ID,
	}
	if recipe.ContractVersion == model.RecipeContractProductIntegrationV2 && len(bindings) == 1 {
		value.IntegrationRevisionID = bindings[0].IntegrationRevisionID
		value.IntegrationManifestHash = bindings[0].IntegrationManifestHash
	}
	audit := model.AuditEvent{
		ID:             randomID("audit"),
		OrganisationID: recipe.OrganisationID,
		ProductID:      recipe.ProductID,
		ActorID:        actor.ID,
		Action:         auditAction,
		TargetType:     "recipe",
		TargetID:       recipe.ID,
		Current:        map[string]any{"analysis_id": analysis.ID, "integration_ids": recipeIntegrationIDs(recipe), "revision_id": value.ID},
		RequestID:      recipeAuditRequestID(actor),
		CreatedAt:      s.now(),
	}
	mutation := store.RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit}
	creating := recipe.Revision == 0
	if creating {
		recipe, err = s.store.CreateRecipeWithRevision(ctx, recipe, value, mutation)
	} else {
		recipe, err = s.store.SaveRecipeRevision(ctx, recipe, value, mutation)
	}
	if err != nil {
		if errors.Is(err, store.ErrCatalogConflict) {
			if creating {
				return recipe, ErrRecipeGroundingChanged
			}
			return s.resolveRecipeCatalogConflict(ctx, recipe.ProductID, recipe.ID)
		}
		return recipe, err
	}
	return recipe, nil
}

func (s *Service) createPreparedRecipe(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string, actor Actor) (model.Recipe, error) {
	integrationIDs := integrationScopeIDs(analysis.Evidence)
	if len(integrationIDs) == 0 {
		return model.Recipe{}, ErrRecipeNeedsInput
	}
	if _, err := s.currentPublishedRecipeAPIBindings(ctx, integrationIDs); err != nil {
		return model.Recipe{}, err
	}
	recipeID, err := randomUUID()
	if err != nil {
		return model.Recipe{}, err
	}
	if seed.Slug == "" {
		seed.Slug = "recipe"
	}
	if _, lookupErr := s.store.RecipeBySlug(ctx, product.ID, seed.Slug); lookupErr == nil {
		seed.Slug = scopedRecipeCollisionSlug(seed.Slug, strings.Join(integrationIDs, ":"), strings.Join(seed.CapabilityIDs, ":"))
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
		APIAttachments:  recipeAPIAttachments(integrationIDs),
		AnalysisID:      analysis.ID,
		ContractVersion: model.RecipeContractDeploymentV3,
		Slug:            seed.Slug,
		Title:           seed.Title,
		Outcome:         seed.Outcome,
		Audience:        "coding_agent",
		State:           "draft",
		Generated:       true,
		NeedsAttention:  true,
		Visibility:      model.VisibilityPrivate,
		Dependencies:    recipeGroundingDependenciesForContract(analysis, seed, model.RecipeContractDeploymentV3),
		StableURI:       "dokosoko://products/" + product.Slug + "/recipes/" + seed.Slug,
	}
	if len(recipe.Dependencies) == 0 {
		return model.Recipe{}, ErrRecipeNeedsInput
	}
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", "recipe.created", actor)
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
	return s.CreateRecipeFromPromptWithAPIs(ctx, productID, nil, instruction, actor)
}

func (s *Service) CreateRecipeFromPromptFor(ctx context.Context, productID, integrationID, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	integrationID = strings.TrimSpace(integrationID)
	if integrationID == "" {
		return s.CreateRecipeFromPromptWithAPIs(ctx, productID, nil, instruction, actor)
	}
	return s.CreateRecipeFromPromptWithAPIs(ctx, productID, []string{integrationID}, instruction, actor)
}

type recipeAPIOption struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description,omitempty"`
	CapabilityIDs []string `json:"capability_ids"`
}

func normalizeRequestedRecipeAPIs(values []string) ([]string, error) {
	if len(values) > 8 {
		return nil, errors.New("a recipe may attach at most 8 APIs")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			return nil, errors.New("recipe integration_ids must contain unique non-empty IDs")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func mergeRecipeAPIEvidence(values []model.IntegrationEvidence) []model.IntegrationEvidence {
	type evidenceKey struct{ kind, resourceID, fingerprint, version string }
	merged := make(map[evidenceKey]model.IntegrationEvidence)
	order := make([]evidenceKey, 0)
	for _, item := range values {
		key := evidenceKey{item.Kind, item.ResourceID, item.Fingerprint, item.Version}
		current, exists := merged[key]
		if !exists {
			current = item
			current.IntegrationIDs = nil
			order = append(order, key)
		}
		seen := make(map[string]bool, len(current.IntegrationIDs))
		for _, integrationID := range current.IntegrationIDs {
			seen[integrationID] = true
		}
		for _, integrationID := range item.IntegrationIDs {
			if integrationID != "" && !seen[integrationID] {
				current.IntegrationIDs = append(current.IntegrationIDs, integrationID)
				seen[integrationID] = true
			}
		}
		sort.Strings(current.IntegrationIDs)
		merged[key] = current
	}
	result := make([]model.IntegrationEvidence, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key])
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].ResourceID != result[j].ResourceID {
			return result[i].ResourceID < result[j].ResourceID
		}
		return result[i].Fingerprint < result[j].Fingerprint
	})
	return result
}

func recipeEvidenceForAPIs(values []model.IntegrationEvidence, integrationIDs []string) []model.IntegrationEvidence {
	selected := make(map[string]bool, len(integrationIDs))
	for _, integrationID := range integrationIDs {
		selected[integrationID] = true
	}
	result := make([]model.IntegrationEvidence, 0, len(values))
	for _, item := range values {
		include := false
		for _, integrationID := range item.IntegrationIDs {
			include = include || selected[integrationID]
		}
		if include {
			result = append(result, item)
		}
	}
	return mergeRecipeAPIEvidence(result)
}

func recipeCapabilityAPIIDs(analysis model.IntegrationAnalysis, capabilityIDs []string) ([]string, bool) {
	byID, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(analysis.Evidence))
	seen := make(map[string]bool)
	values := make([]string, 0)
	for _, capabilityID := range capabilityIDs {
		item, ok := byID[capabilityID]
		integrationID := recipeCapabilityIntegrationID(item)
		if !ok || ambiguous[capabilityID] || integrationID == "" {
			return nil, false
		}
		if !seen[integrationID] {
			seen[integrationID] = true
			values = append(values, integrationID)
		}
	}
	sort.Strings(values)
	return values, len(values) > 0
}

func (s *Service) recipeAPIDetectionAnalysis(ctx context.Context, product model.Product, requestedIDs []string, instruction string) (model.IntegrationAnalysis, []recipeAPIOption, error) {
	integrations, err := s.store.Integrations(ctx, product.ID)
	if err != nil {
		return model.IntegrationAnalysis{}, nil, err
	}
	requested := make(map[string]bool, len(requestedIDs))
	for _, integrationID := range requestedIDs {
		requested[integrationID] = true
	}
	seenRequested := make(map[string]bool, len(requestedIDs))
	analysis := model.IntegrationAnalysis{OrganisationID: product.OrganisationID, ProductID: product.ID, SchemaVersion: integrationAnalysisSchemaVersion + 1, State: "review", GeneratedBy: "ai_assisted"}
	options := make([]recipeAPIOption, 0)
	for _, integration := range integrations {
		if integration.Lifecycle == "retired" || len(requested) > 0 && !requested[integration.ID] {
			continue
		}
		seenRequested[integration.ID] = true
		if _, bindingErr := s.currentPublishedRecipeIntegration(ctx, integration.ID); bindingErr != nil {
			if len(requested) > 0 {
				return analysis, nil, bindingErr
			}
			continue
		}
		evidence, _, evidenceErr := s.scopedIntegrationEvidence(ctx, product, integration.ID)
		if evidenceErr != nil {
			if len(requested) > 0 {
				return analysis, nil, evidenceErr
			}
			continue
		}
		scoped := model.IntegrationAnalysis{Evidence: evidence}
		scoped, evidenceErr = s.relevantRecipeAnalysis(ctx, product, scoped, instruction)
		if evidenceErr != nil {
			return analysis, nil, evidenceErr
		}
		seeds := deterministicIntegrationRecipeSeeds(product, integration, scoped.Evidence)
		capabilityIDs := make([]string, 0, len(seeds))
		for _, seed := range seeds {
			capabilityIDs = append(capabilityIDs, seed.CapabilityIDs...)
		}
		if len(capabilityIDs) == 0 {
			if len(requested) > 0 {
				return analysis, nil, ErrRecipeNeedsInput
			}
			continue
		}
		analysis.Evidence = append(analysis.Evidence, scoped.Evidence...)
		analysis.Plan.Recipes = append(analysis.Plan.Recipes, seeds...)
		options = append(options, recipeAPIOption{ID: integration.ID, Name: integration.DisplayName, Version: integration.VersionKey, Description: integration.Description, CapabilityIDs: capabilityIDs})
	}
	for integrationID := range requested {
		if !seenRequested[integrationID] {
			return analysis, nil, store.ErrNotFound
		}
	}
	analysis.Evidence = mergeRecipeAPIEvidence(analysis.Evidence)
	analysis.Plan.Summary = "Select the exact reviewed APIs and capabilities required for one implementation workflow."
	if len(options) == 0 || len(analysis.Plan.Recipes) == 0 {
		return analysis, nil, ErrRecipeNeedsInput
	}
	sort.Slice(options, func(i, j int) bool { return options[i].ID < options[j].ID })
	return analysis, options, nil
}

func (s *Service) CreateRecipeFromPromptWithAPIs(ctx context.Context, productID string, requestedIntegrationIDs []string, instruction string, actor Actor) (recipe model.Recipe, runErr error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" || len(instruction) > 4000 || containsToolBuilderSecretText(instruction) {
		return recipe, errors.New("describe one non-secret product workflow in 1 to 4,000 characters")
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, err
	}
	requestedIntegrationIDs, err = normalizeRequestedRecipeAPIs(requestedIntegrationIDs)
	if err != nil {
		return recipe, err
	}
	analysis, availableAPIs, err := s.recipeAPIDetectionAnalysis(ctx, product, requestedIntegrationIDs, instruction)
	if err != nil {
		return recipe, err
	}
	productEvidence := recipeProductEvidence(analysis.Evidence)
	prompt, _ := json.Marshal(map[string]any{
		"request":                instruction,
		"product":                map[string]string{"name": product.Name, "description": product.Description},
		"available_apis":         availableAPIs,
		"product_evidence":       productEvidence,
		"allowed_capability_ids": recipeProductCapabilityIDs(productEvidence),
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
	selectedAPIIDs, valid := recipeCapabilityAPIIDs(analysis, seed.CapabilityIDs)
	if !valid {
		return recipe, ErrRecipeNeedsInput
	}
	analysis.Evidence = recipeEvidenceForAPIs(analysis.Evidence, selectedAPIIDs)
	selectedEvidenceSet := make(map[string]bool)
	for _, item := range analysis.Evidence {
		selectedEvidenceSet[item.ResourceID] = true
	}
	for _, evidenceID := range seed.EvidenceIDs {
		if !selectedEvidenceSet[evidenceID] {
			return recipe, ErrRecipeNeedsInput
		}
	}
	analysis.Plan.Recipes = []model.RecipeSeed{seed}
	analysis.ID, err = randomUUID()
	if err != nil {
		return recipe, err
	}
	completedAt := s.now()
	analysis.CompletedAt = &completedAt
	analysis, err = s.store.SaveIntegrationAnalysis(ctx, analysis, 0)
	if err != nil {
		return recipe, err
	}
	return s.createRecipeFromSeed(ctx, product, analysis, seed, instruction, actor)
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
		expectedIntegrationIDs := integrationScopeIDs(preparedAnalysis.Evidence)
		if lookupErr == nil && (existing.ContractVersion != model.RecipeContractDeploymentV3 || !slices.Equal(recipeIntegrationIDs(existing), expectedIntegrationIDs)) {
			seed.Slug = scopedRecipeCollisionSlug(seed.Slug, integrationID, seed.CapabilityIDs[0])
			existing, lookupErr = s.store.RecipeBySlug(ctx, productID, seed.Slug)
			if lookupErr == nil && (existing.ContractVersion != model.RecipeContractDeploymentV3 || !slices.Equal(recipeIntegrationIDs(existing), expectedIntegrationIDs)) {
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
		existing.IntegrationID = ""
		existing.APIAttachments = recipeAPIAttachments(expectedIntegrationIDs)
		existing.AnalysisID = analysis.ID
		existing.ContractVersion = model.RecipeContractDeploymentV3
		existing.Title = seed.Title
		existing.Outcome = seed.Outcome
		existing.Audience = "coding_agent"
		existing.Dependencies = recipeGroundingDependenciesForContract(preparedAnalysis, seed, model.RecipeContractDeploymentV3)
		regrounded, refreshErr := s.createRecipeRevision(ctx, product, existing, preparedAnalysis, draft, "", "recipe.regrounded", actor)
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
		product, err = s.store.Product(ctx, productID)
		if err != nil {
			return recipes, err
		}
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
	draft, err := s.authorRecipeForContract(ctx, product, analysis, seed, instruction, recipe.ContractVersion)
	if err != nil {
		return recipe, err
	}
	recipe.Dependencies = recipeGroundingDependenciesForContract(analysis, seed, recipe.ContractVersion)
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", "recipe.reworked", actor)
}

// UpdateRecipeReferences is the only human edit path for v2. The server keeps
// ownership of the capability and all instruction prose; an operator may only
// retain reviewed references and choose distribution visibility.
func (s *Service) UpdateRecipeReferences(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string, referenceIDs []string, visibility model.Visibility, actor Actor) (model.Recipe, error) {
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
	spec, err := decodeRecipeSpec(recipe.CurrentRevision)
	if err != nil {
		return recipe, err
	}
	if len(referenceIDs) > 8 {
		return recipe, errors.New("recipe reference_ids must contain at most 8 reviewed document IDs")
	}
	for _, referenceID := range referenceIDs {
		if referenceID == "" || referenceID != strings.TrimSpace(referenceID) {
			return recipe, errors.New("recipe reference_ids must be non-empty IDs without surrounding whitespace")
		}
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
	spec.ReferenceIDs = append([]string(nil), referenceIDs...)
	references, ok := selectRecipeReferences(spec.ReferenceIDs, recipeReferences(selectedEvidence))
	if !ok {
		return recipe, errors.New("recipe references must select exact reviewed product documents")
	}
	recipe.Visibility = visibility
	draft := authoredRecipe{Spec: spec, References: references, GeneratedBy: "human"}
	return s.createRecipeRevision(ctx, product, recipe, analysis, draft, "", "recipe.references.updated", actor)
}

func recipeAuditRequestID(actor Actor) string {
	if requestID := strings.TrimSpace(actor.RequestID); requestID != "" {
		return requestID
	}
	return randomID("request")
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

func (s *Service) currentRecipeEvidence(ctx context.Context, product model.Product, recipe model.Recipe, evidenceByScope map[string][]model.IntegrationEvidence) ([]model.IntegrationEvidence, bool, error) {
	if recipe.ContractVersion != model.RecipeContractProductIntegrationV2 && recipe.ContractVersion != model.RecipeContractDeploymentV3 {
		return nil, false, nil
	}
	integrationIDs := recipeIntegrationIDs(recipe)
	if len(integrationIDs) == 0 {
		return nil, false, nil
	}
	combined := make([]model.IntegrationEvidence, 0)
	for _, integrationID := range integrationIDs {
		cacheKey := integrationID + "\x00" + recipe.Outcome
		var evidence []model.IntegrationEvidence
		if evidenceByScope != nil {
			if cached, ok := evidenceByScope[cacheKey]; ok {
				evidence = append([]model.IntegrationEvidence(nil), cached...)
			}
		}
		if evidence == nil {
			var err error
			evidence, _, err = s.scopedIntegrationEvidence(ctx, product, integrationID)
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
			evidence = analysis.Evidence
			if evidenceByScope != nil {
				evidenceByScope[cacheKey] = append([]model.IntegrationEvidence(nil), evidence...)
			}
		}
		integration, err := s.store.Integration(ctx, product.ID, integrationID)
		if err != nil {
			return nil, false, err
		}
		evidence, err = s.restoreRecipeDeveloperAssetDependencies(ctx, integration, evidence, recipe.Dependencies)
		if err != nil {
			return nil, false, err
		}
		combined = append(combined, evidence...)
	}
	return mergeRecipeAPIEvidence(combined), true, nil
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
	// Revision-level API bindings remain immutable creation provenance.
	// Currentness is decided by the exact selected evidence below;
	// otherwise an unrelated attachment added to a later API publication would
	// invalidate every recipe for that API.
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
			if errors.Is(err, ErrPublicMCPRecipe) || errors.Is(err, errPublicRecipeEvidence) || errors.Is(err, store.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

func (s *Service) markRecipeOutdated(ctx context.Context, product model.Product, recipe model.Recipe) (model.Recipe, error) {
	if recipe.State == "outdated" && recipe.NeedsAttention && recipe.ApprovedAt == nil && recipe.ApprovedBy == "" && recipe.PublishedAt == nil {
		return recipe, nil
	}
	priorState := recipe.State
	recipe.State, recipe.NeedsAttention = "outdated", true
	recipe.ApprovedAt, recipe.ApprovedBy, recipe.PublishedAt = nil, "", nil
	now := s.now()
	audit := model.AuditEvent{
		ID:             randomID("audit"),
		OrganisationID: recipe.OrganisationID,
		ProductID:      recipe.ProductID,
		ActorID:        "system:recipe-grounding",
		Action:         "recipe.outdated",
		TargetType:     "recipe",
		TargetID:       recipe.ID,
		Prior:          map[string]any{"state": priorState},
		Current:        map[string]any{"state": "outdated", "revision_id": recipe.CurrentRevisionID},
		RequestID:      "system:recipe-grounding",
		CreatedAt:      now,
	}
	return s.store.SaveRecipeTransition(ctx, recipe, store.RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit})
}

func (s *Service) requireCurrentRecipeGrounding(ctx context.Context, product model.Product, recipe model.Recipe) (model.Recipe, error) {
	current, err := s.recipeGroundingCurrent(ctx, product, recipe, nil)
	if err != nil {
		return recipe, err
	}
	if current {
		return recipe, nil
	}
	recipe, saveErr := s.markRecipeOutdated(ctx, product, recipe)
	if saveErr != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, saveErr)
	}
	return recipe, ErrRecipeGroundingChanged
}

func (s *Service) resolveRecipeCatalogConflict(ctx context.Context, productID, recipeID string) (model.Recipe, error) {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, err)
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, err)
	}
	current, err := s.recipeGroundingCurrent(ctx, product, recipe, nil)
	if err != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, err)
	}
	if current {
		return recipe, store.ErrConflict
	}
	outdated, saveErr := s.markRecipeOutdated(ctx, product, recipe)
	if saveErr != nil {
		return recipe, errors.Join(ErrRecipeGroundingChanged, saveErr)
	}
	return outdated, ErrRecipeGroundingChanged
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
	audit := model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.approved", TargetType: "recipe", TargetID: recipe.ID, Prior: map[string]any{"state": "review"}, Current: map[string]any{"state": "approved", "revision_id": recipe.CurrentRevisionID, "api_bindings": recipe.CurrentRevision.APIBindings}, RequestID: recipeAuditRequestID(actor), CreatedAt: now}
	recipe, err = s.store.SaveRecipeTransition(ctx, recipe, store.RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit})
	if errors.Is(err, store.ErrCatalogConflict) {
		return s.resolveRecipeCatalogConflict(ctx, productID, recipeID)
	}
	return recipe, err
}

func (s *Service) validatePublicRecipeEvidence(ctx context.Context, productID string, recipe model.Recipe, selectedEvidence []model.IntegrationEvidence) error {
	if recipeEvidenceUsesMCP(selectedEvidence) {
		return ErrPublicMCPRecipe
	}
	for _, item := range selectedEvidence {
		if item.Visibility != model.VisibilityPublic {
			return errPublicRecipeEvidence
		}
		if err := s.validatePublicRecipeDeveloperAssetEvidence(ctx, productID, recipe, item); err != nil {
			return err
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
		if recipeDeveloperAssetSupportingKind(item.Kind) {
			public[item.ResourceID] = true
		}
	}
	for _, reference := range recipe.CurrentRevision.References {
		if !public[reference.ResourceID] {
			return errPublicRecipeEvidence
		}
	}
	return nil
}

func recipeEvidenceUsesMCP(evidence []model.IntegrationEvidence) bool {
	for _, item := range evidence {
		if item.Kind == "tool" && recipeEvidenceField(item.Excerpt, "Backend") == "mcp" {
			return true
		}
	}
	return false
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
	audit := model.AuditEvent{ID: randomID("audit"), OrganisationID: recipe.OrganisationID, ProductID: recipe.ProductID, ActorID: actor.ID, Action: "recipe.published", TargetType: "recipe", TargetID: recipe.ID, Prior: map[string]any{"state": "approved"}, Current: map[string]any{"state": "published", "visibility": recipe.Visibility, "stable_uri": recipe.StableURI, "contract_version": recipe.ContractVersion}, RequestID: recipeAuditRequestID(actor), CreatedAt: now}
	recipe, err = s.store.SaveRecipeTransition(ctx, recipe, store.RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit})
	if errors.Is(err, store.ErrCatalogConflict) {
		return s.resolveRecipeCatalogConflict(ctx, productID, recipeID)
	}
	return recipe, err
}

func (s *Service) DeleteRecipe(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string, actor Actor) error {
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
		return err
	}
	if recipe.ContractVersion != model.RecipeContractLegacyMCPV1 && recipe.State != "outdated" {
		return ErrRecipeDeletionNotAllowed
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return err
	}
	now := s.now()
	audit := model.AuditEvent{
		ID:             randomID("audit"),
		OrganisationID: recipe.OrganisationID,
		ProductID:      recipe.ProductID,
		ActorID:        actor.ID,
		Action:         "recipe.deleted",
		TargetType:     "recipe",
		TargetID:       recipe.ID,
		Prior: map[string]any{
			"contract_version":    recipe.ContractVersion,
			"state":               recipe.State,
			"title":               recipe.Title,
			"current_revision_id": recipe.CurrentRevisionID,
		},
		Current:   map[string]any{"deleted": true},
		RequestID: recipeAuditRequestID(actor),
		CreatedAt: now,
	}
	mutation := store.RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit}
	if err := s.store.DeleteRecipe(ctx, productID, recipeID, mutation); !errors.Is(err, store.ErrCatalogConflict) {
		return err
	}

	// A catalog publication may race with this confirmed deletion without
	// changing the recipe itself. Refresh both guards and retry once, but only
	// while the operator's exact recipe revision is still current.
	latest, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return err
	}
	if err := requireExpectedRecipeRevision(latest, expectedRevision, expectedCurrentRevisionID); err != nil {
		return err
	}
	product, err = s.store.Product(ctx, productID)
	if err != nil {
		return err
	}
	mutation.ExpectedRevision = latest.Revision
	mutation.ExpectedCatalogRevision = product.CatalogRevision
	return s.store.DeleteRecipe(ctx, productID, recipeID, mutation)
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
			updated, saveErr := s.markRecipeOutdated(ctx, product, recipes[index])
			if saveErr != nil {
				return nil, saveErr
			}
			recipes[index] = updated
			product, err = s.store.Product(ctx, productID)
			if err != nil {
				return nil, err
			}
		}
	}
	return recipes, nil
}
