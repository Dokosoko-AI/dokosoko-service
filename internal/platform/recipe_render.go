package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"sort"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

// Recipe grounding is deliberately narrower than integration analysis. Only
// the exact product facts selected for one recipe participate in authoring and
// drift; DokoSoko's MCP delivery and administration contracts never do.
func recipeReferences(evidence []model.IntegrationEvidence) []model.RecipeReference {
	values := make([]model.RecipeReference, 0)
	seenURL := make(map[string]bool)
	seenID := make(map[string]bool)
	add := func(reference model.RecipeReference) {
		reference.Label = recipeLinkLabel(reference.Label)
		reference.URL = strings.TrimSpace(reference.URL)
		parsed, err := url.Parse(reference.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || seenURL[reference.URL] {
			return
		}
		if reference.ResourceID != "" && seenID[reference.ResourceID] {
			return
		}
		seenURL[reference.URL] = true
		seenID[reference.ResourceID] = reference.ResourceID != ""
		values = append(values, reference)
	}
	for _, item := range recipeProductEvidence(evidence) {
		if item.Location != "" {
			add(model.RecipeReference{Label: item.Label, URL: item.Location, Kind: recipeReferenceKind(item.Label, item.Location), ResourceID: item.ResourceID})
		}
		for _, reference := range item.References {
			add(reference)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].ResourceID == values[j].ResourceID {
			return values[i].URL < values[j].URL
		}
		return values[i].ResourceID < values[j].ResourceID
	})
	return values
}

func recipeDependencies(evidence []model.IntegrationEvidence) []model.RecipeDependency {
	values := make([]model.RecipeDependency, 0, len(evidence))
	for _, item := range evidence {
		version := item.Fingerprint
		if recipeDeveloperAssetSupportingKind(item.Kind) {
			// Developer-asset versions intentionally retain the exact nested
			// publication ID, revision, publication hash, attachment hash, and
			// selected unit hash rather than collapsing them into an opaque digest.
			version = item.Version
		}
		values = append(values, model.RecipeDependency{Kind: item.Kind, ResourceID: item.ResourceID, Version: version})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].ResourceID != values[j].ResourceID {
			return values[i].ResourceID < values[j].ResourceID
		}
		return values[i].Version < values[j].Version
	})
	return values
}

func recipeSelectedEvidence(evidence []model.IntegrationEvidence, evidenceIDs []string) ([]model.IntegrationEvidence, bool) {
	if len(evidenceIDs) == 0 {
		return nil, false
	}
	byID, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(evidence))
	selected := make([]model.IntegrationEvidence, 0, len(evidenceIDs))
	seen := make(map[string]bool, len(evidenceIDs))
	for _, rawID := range evidenceIDs {
		id := strings.TrimSpace(rawID)
		item, exists := byID[id]
		if id == "" || !exists || ambiguous[id] || seen[id] {
			return nil, false
		}
		selected = append(selected, item)
		seen[id] = true
	}
	return selected, true
}

// recipeEvidenceForDependencies recovers the exact persisted product snapshot.
// The synthetic authoring dependency is checked separately and can never widen
// the evidence selection.
func recipeEvidenceForDependencies(evidence []model.IntegrationEvidence, dependencies []model.RecipeDependency) ([]model.IntegrationEvidence, bool) {
	byDependency := make(map[model.RecipeDependency]model.IntegrationEvidence, len(evidence))
	ambiguous := make(map[model.RecipeDependency]bool)
	for _, item := range recipeProductEvidence(evidence) {
		version := item.Fingerprint
		if recipeDeveloperAssetSupportingKind(item.Kind) {
			version = item.Version
		}
		dependency := model.RecipeDependency{Kind: item.Kind, ResourceID: item.ResourceID, Version: version}
		if _, exists := byDependency[dependency]; exists {
			ambiguous[dependency] = true
		}
		byDependency[dependency] = item
	}
	selected := make([]model.IntegrationEvidence, 0, len(dependencies))
	seen := make(map[model.RecipeDependency]bool, len(dependencies))
	authoringDependencies := 0
	for _, dependency := range dependencies {
		if dependency.Kind == recipeAuthoringInputDependencyKind {
			authoringDependencies++
			if authoringDependencies > 1 || strings.TrimSpace(dependency.ResourceID) == "" || strings.TrimSpace(dependency.Version) == "" {
				return nil, false
			}
			continue
		}
		item, exists := byDependency[dependency]
		if !exists || ambiguous[dependency] || seen[dependency] {
			return nil, false
		}
		selected = append(selected, item)
		seen[dependency] = true
	}
	return selected, len(selected) > 0 && authoringDependencies == 1
}

func recipeEvidenceIDsForDependencies(evidence []model.IntegrationEvidence, dependencies []model.RecipeDependency) ([]string, bool) {
	selected, ok := recipeEvidenceForDependencies(evidence, dependencies)
	if !ok {
		return nil, false
	}
	ids := make([]string, 0, len(selected))
	for _, item := range selected {
		ids = append(ids, item.ResourceID)
	}
	sort.Strings(ids)
	return ids, true
}

func recipeAnalysisWithEvidence(analysis model.IntegrationAnalysis, evidence []model.IntegrationEvidence) model.IntegrationAnalysis {
	analysis.Evidence = evidence
	return analysis
}

func recipeGroundingDependenciesForContract(analysis model.IntegrationAnalysis, seed model.RecipeSeed, authoringContract string) []model.RecipeDependency {
	selected, ok := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs)
	if !ok {
		return nil
	}
	values := recipeDependencies(selected)
	normalizedSeed := seed
	normalizedSeed.EndpointIDs = nil
	normalizedSeed.CapabilityIDs = append([]string(nil), seed.CapabilityIDs...)
	normalizedSeed.EvidenceIDs = append([]string(nil), seed.EvidenceIDs...)
	sort.Strings(normalizedSeed.CapabilityIDs)
	sort.Strings(normalizedSeed.EvidenceIDs)
	integrationIDs := integrationScopeIDs(analysis.Evidence)
	input, _ := json.Marshal(struct {
		AuthoringContract string           `json:"authoring_contract"`
		IntegrationIDs    []string         `json:"integration_ids"`
		Seed              model.RecipeSeed `json:"seed"`
	}{AuthoringContract: authoringContract, IntegrationIDs: integrationIDs, Seed: normalizedSeed})
	return append(values, model.RecipeDependency{Kind: recipeAuthoringInputDependencyKind, ResourceID: normalizedSeed.Slug, Version: evidenceFingerprint(recipeAuthoringInputDependencyKind, string(input))})
}

func recipeGroundingDependencies(analysis model.IntegrationAnalysis, seed model.RecipeSeed) []model.RecipeDependency {
	return recipeGroundingDependenciesForContract(analysis, seed, recipeAuthoringContractVersion)
}

func recipeDependencySetsMatch(actual, expected []model.RecipeDependency) bool {
	if len(actual) != len(expected) {
		return false
	}
	remaining := make(map[model.RecipeDependency]int, len(expected))
	for _, dependency := range expected {
		remaining[dependency]++
	}
	for _, dependency := range actual {
		if remaining[dependency] == 0 {
			return false
		}
		remaining[dependency]--
	}
	return true
}

func recipeGroundingMatches(recipe model.Recipe, analysis model.IntegrationAnalysis, seed model.RecipeSeed) bool {
	integrationIDs := integrationScopeIDs(analysis.Evidence)
	expectedSpecVersion := model.RecipeSpecVersion3
	if recipe.ContractVersion == model.RecipeContractProductIntegrationV2 {
		expectedSpecVersion = model.RecipeSpecVersion2
	}
	if len(integrationIDs) == 0 || (recipe.ContractVersion != model.RecipeContractDeploymentV3 && recipe.ContractVersion != model.RecipeContractProductIntegrationV2) || !slices.Equal(recipeIntegrationIDs(recipe), integrationIDs) || recipe.AnalysisID != analysis.ID || recipe.CurrentRevisionID == "" || recipe.CurrentRevision == nil || recipe.CurrentRevision.SpecVersion != expectedSpecVersion || recipe.Title != seed.Title || recipe.Outcome != seed.Outcome || recipe.Audience != "coding_agent" {
		return false
	}
	if _, ok := recipeResolveProductSelection(analysis, seed); !ok {
		return false
	}
	return recipeDependencySetsMatch(recipe.Dependencies, recipeGroundingDependenciesForContract(analysis, seed, recipe.ContractVersion))
}

// recipeDependenciesMatchCurrentContract prevents the synthetic authoring
// dependency from becoming an inert marker. It is recomputed from the stored
// immutable recipe fields and exact selected evidence so a contract change or
// dependency tamper deterministically makes the revision stale.
func recipeDependenciesMatchCurrentContract(recipe model.Recipe, spec model.RecipeSpec, selectedEvidence []model.IntegrationEvidence) bool {
	evidenceIDs := make([]string, 0, len(selectedEvidence))
	for _, item := range selectedEvidence {
		if strings.TrimSpace(item.ResourceID) == "" {
			return false
		}
		evidenceIDs = append(evidenceIDs, item.ResourceID)
	}
	scopeEvidence := make([]model.IntegrationEvidence, 0, len(recipeIntegrationIDs(recipe))+len(selectedEvidence))
	for _, integrationID := range recipeIntegrationIDs(recipe) {
		scopeEvidence = append(scopeEvidence, model.IntegrationEvidence{Kind: integrationScopeEvidenceKind, ResourceID: integrationID})
	}
	analysis := model.IntegrationAnalysis{Evidence: append(scopeEvidence, selectedEvidence...)}
	seed := model.RecipeSeed{
		Slug:          recipe.Slug,
		Title:         recipe.Title,
		Outcome:       recipe.Outcome,
		Audience:      "coding_agent",
		CapabilityIDs: append([]string(nil), spec.CapabilityIDs...),
		SDKID:         spec.SDKID,
		EvidenceIDs:   evidenceIDs,
	}
	return recipeDependencySetsMatch(recipe.Dependencies, recipeGroundingDependenciesForContract(analysis, seed, recipe.ContractVersion))
}

func recipeEvidenceField(excerpt, name string) string {
	prefix := name + ":"
	for _, raw := range strings.Split(excerpt, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func recipeCode(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = strings.ReplaceAll(value, "`", "'")
	return "`" + value + "`"
}

func recipeLinkLabel(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = strings.NewReplacer("[", "(", "]", ")").Replace(value)
	return firstNonEmpty(value, "Reference")
}

func recipeGroundedURLs(analysis model.IntegrationAnalysis) []string {
	values := make([]string, 0)
	seen := make(map[string]bool)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && !seen[candidate] {
			values = append(values, candidate)
			seen[candidate] = true
		}
	}
	for _, item := range recipeProductEvidence(analysis.Evidence) {
		add(item.Location)
		add(recipeEvidenceField(item.Excerpt, "Fixed endpoint"))
		for _, reference := range item.References {
			add(reference.URL)
		}
	}
	return values
}

func recipeMarkdownSections(markdown string) ([]string, []string, map[string]string) {
	sections := make(map[string]string)
	order := make([]string, 0)
	titles := make([]string, 0, 1)
	current := ""
	var body strings.Builder
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(body.String())
		}
		body.Reset()
	}
	for _, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			titles = append(titles, strings.TrimSpace(strings.TrimPrefix(line, "# ")))
		}
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			if _, duplicate := sections[current]; duplicate {
				current = ""
				continue
			}
			order = append(order, current)
			continue
		}
		if current != "" {
			body.WriteString(raw)
			body.WriteByte('\n')
		}
	}
	flush()
	return titles, order, sections
}

func containsRecipeRawHTML(markdown string) bool {
	lower := strings.ToLower(markdown)
	if strings.Contains(lower, "<!--") || strings.Contains(lower, "<!") || strings.Contains(lower, "<?") || strings.Contains(lower, "<%") {
		return true
	}
	for index := 0; index < len(markdown); index++ {
		if markdown[index] != '<' {
			continue
		}
		cursor := index + 1
		closingTag := false
		if cursor < len(markdown) && markdown[cursor] == '/' {
			closingTag = true
			cursor++
		}
		if cursor >= len(markdown) || !isASCIIAlpha(markdown[cursor]) {
			continue
		}
		nameStart := cursor
		for cursor < len(markdown) && isHTMLTagNameByte(markdown[cursor]) {
			cursor++
		}
		if cursor >= len(markdown) {
			continue
		}
		switch markdown[cursor] {
		case ' ', '\t', '\r', '\n', '\f', '>', '/':
			return true
		case ':':
			if closingTag || markdown[nameStart:cursor] != "https" || cursor+2 >= len(markdown) || markdown[cursor:cursor+3] != "://" {
				return true
			}
		}
	}
	return false
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isHTMLTagNameByte(value byte) bool {
	return isASCIIAlpha(value) || value >= '0' && value <= '9' || value == '-'
}

func validateRecipeMarkdown(markdown, expectedTitle string, references []model.RecipeReference, groundedURLs ...string) []model.RecipeValidationFinding {
	findings := make([]model.RecipeValidationFinding, 0)
	trimmed, lower := strings.TrimSpace(markdown), strings.ToLower(markdown)
	if len(trimmed) < 80 {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "recipe_too_short", Message: "Include a concrete outcome, ordered implementation steps, and observable verification."})
	}
	if len([]byte(markdown)) > maxRecipeMarkdownBytes {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "recipe_too_long", Message: "Keep the rendered recipe under 12 KiB."})
	}
	titles, order, sections := recipeMarkdownSections(markdown)
	if len(titles) != 1 || titles[0] == "" {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "missing_title", Message: "Use exactly one non-empty level-one title."})
	} else if expectedTitle != "" && titles[0] != expectedTitle {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "title_mismatch", Message: "Keep the rendered title identical to the structured recipe title."})
	}
	allowedSections := map[string]bool{"outcome": true, "prerequisites": true, "steps": true, "verify": true, "references": true}
	for _, heading := range order {
		if !allowedSections[heading] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unexpected_section", Message: "Recipes may contain only Outcome, Prerequisites, Steps, Verify, and References sections."})
			break
		}
	}
	last := -1
	for _, heading := range []string{"outcome", "steps", "verify"} {
		body, exists := sections[heading]
		index := -1
		for candidateIndex, candidate := range order {
			if candidate == heading {
				index = candidateIndex
				break
			}
		}
		if !exists || body == "" {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "missing_section", Message: "Add the required non-empty " + heading + " section."})
			continue
		}
		if index <= last {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "section_order", Message: "Keep Outcome, Steps, and Verify in that order."})
			break
		}
		last = index
	}
	unsafeContent := containsRecipeRawHTML(markdown) || recipeContainsUnsupportedURI(markdown) || containsToolBuilderSecretText(markdown)
	for _, unsafe := range []string{"javascript:", "data:", "authorization: bearer", "-----begin private key-----"} {
		unsafeContent = unsafeContent || strings.Contains(lower, unsafe)
	}
	if unsafeContent {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_content", Message: "Remove raw HTML, executable markup, insecure links, or credential-like content."})
	}
	allowedURLs := make(map[string]bool, len(references)+len(groundedURLs))
	for _, reference := range references {
		parsed, err := url.Parse(reference.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_reference", Message: "Recipe references must use fixed HTTPS URLs."})
			continue
		}
		allowedURLs[reference.URL] = true
		if reference.Anchor != "" {
			allowedURLs[reference.URL+"#"+reference.Anchor] = true
		}
	}
	for _, groundedURL := range groundedURLs {
		allowedURLs[groundedURL] = true
	}
	for _, match := range recipeMarkdownLinkPattern.FindAllStringSubmatch(markdown, -1) {
		if len(match) < 2 {
			continue
		}
		destination := strings.TrimSpace(match[1])
		if strings.HasPrefix(match[0], "!") || !allowedURLs[destination] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_reference", Message: "Markdown links must select an exact reviewed HTTPS reference; images are not allowed."})
			break
		}
	}
	for _, raw := range recipeURLPattern.FindAllString(markdown, -1) {
		candidate := strings.TrimRight(raw, ".,;:`")
		if !allowedURLs[candidate] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unverified_reference", Message: "Every HTTPS URL must come from the selected reviewed product evidence."})
			break
		}
	}
	return findings
}

func hasRecipeErrors(findings []model.RecipeValidationFinding) bool {
	for _, finding := range findings {
		if finding.Level == "error" {
			return true
		}
	}
	return false
}

type authoredRecipe struct {
	Spec          model.RecipeSpec
	Markdown      string
	References    []model.RecipeReference
	GeneratedBy   string
	Model         string
	PromptVersion string
	PromptHash    string
}

func recipeAIRequestHash(invocation aiInvocation) string {
	encoded, _ := json.Marshal(struct {
		System     string          `json:"system"`
		User       string          `json:"user"`
		SchemaName string          `json:"schema_name"`
		Schema     json.RawMessage `json:"schema"`
	}{System: invocation.System, User: invocation.User, SchemaName: invocation.SchemaName, Schema: invocation.Schema})
	return contentHash(encoded)
}

func selectRecipeReferences(ids []string, allowed []model.RecipeReference) ([]model.RecipeReference, bool) {
	byID := make(map[string]model.RecipeReference, len(allowed))
	ambiguous := make(map[string]bool)
	for _, reference := range allowed {
		if reference.ResourceID == "" {
			continue
		}
		if _, exists := byID[reference.ResourceID]; exists {
			ambiguous[reference.ResourceID] = true
		}
		byID[reference.ResourceID] = reference
	}
	selected := make([]model.RecipeReference, 0, len(ids))
	seen := make(map[string]bool)
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		reference, exists := byID[id]
		if id == "" || !exists || ambiguous[id] || seen[id] {
			return nil, false
		}
		selected = append(selected, reference)
		seen[id] = true
	}
	return selected, true
}

func (s *Service) authorRecipe(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string) (authoredRecipe, error) {
	return s.authorRecipeForContract(ctx, product, analysis, seed, instruction, model.RecipeContractDeploymentV3)
}

func (s *Service) authorRecipeForContract(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction, contractVersion string) (authoredRecipe, error) {
	selectedEvidence, selectionOK := recipeResolveProductSelection(analysis, seed)
	if !selectionOK {
		return authoredRecipe{}, ErrRecipeNeedsInput
	}
	selectedAnalysis := recipeAnalysisWithEvidence(analysis, selectedEvidence)
	canonicalSpec, err := deterministicRecipeSpecForContract(analysis, seed, contractVersion)
	if err != nil {
		return authoredRecipe{}, ErrRecipeNeedsInput
	}
	allowedReferences := recipeReferences(selectedEvidence)
	allowedReferenceIDs := make([]string, 0, len(allowedReferences))
	for _, reference := range allowedReferences {
		if reference.ResourceID != "" {
			allowedReferenceIDs = append(allowedReferenceIDs, reference.ResourceID)
		}
	}
	prompt, _ := json.Marshal(map[string]any{
		"product":               map[string]string{"name": product.Name, "slug": product.Slug},
		"recipe":                seed,
		"product_evidence":      selectedEvidence,
		"allowed_evidence_ids":  evidenceIDs(selectedEvidence),
		"allowed_reference_ids": allowedReferenceIDs,
		"editor_instruction":    strings.TrimSpace(instruction),
	})
	invocation := aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_authoring", PromptKey: AIPromptKeyRecipeAuthoring, User: string(prompt), SchemaName: "recipe", Schema: recipeAuthoringSchema, MaxOutput: 8192, Temperature: 0.1}
	prepared, prepareErr := s.prepareAIInvocation(ctx, invocation)
	if prepareErr == nil {
		result, generateErr := s.generateAIStructured(ctx, prepared)
		if generateErr == nil {
			var response recipeAuthoringResponse
			if decodeStrictAIResult(result.JSON, &response) == nil && response.Status == "ready" && len(response.Gaps) == 0 {
				references, referencesOK := selectRecipeReferences(response.ReferenceIDs, allowedReferences)
				if referencesOK {
					spec := canonicalSpec
					spec.ReferenceIDs = append([]string(nil), response.ReferenceIDs...)
					recipe := recipeForSpecValidation(spec, contractVersion, seed)
					findings := validateRecipeSpec(spec, recipe, selectedEvidence)
					markdown := renderRecipeSpec(spec, references)
					findings = append(findings, validateRecipeMarkdown(markdown, seed.Title, references, recipeGroundedURLs(selectedAnalysis)...)...)
					if !hasRecipeErrors(findings) {
						return authoredRecipe{Spec: spec, Markdown: markdown, References: references, GeneratedBy: "ai", Model: firstNonEmpty(result.ResolvedModel, result.RequestedModel), PromptVersion: prepared.PromptVersion, PromptHash: recipeAIRequestHash(prepared)}, nil
					}
				}
			}
		}
	}

	spec := canonicalSpec
	// Deterministic output includes no optional reading list. Its factual URLs
	// are direct, server-owned operation facts and are validated as grounded.
	markdown := renderRecipeSpec(spec, nil)
	recipe := recipeForSpecValidation(spec, contractVersion, seed)
	findings := validateRecipeSpec(spec, recipe, selectedEvidence)
	findings = append(findings, validateRecipeMarkdown(markdown, seed.Title, nil, recipeGroundedURLs(selectedAnalysis)...)...)
	if hasRecipeErrors(findings) {
		return authoredRecipe{}, ErrRecipeNeedsInput
	}
	return authoredRecipe{Spec: spec, Markdown: markdown, GeneratedBy: "deterministic"}, nil
}

func recipeForSpecValidation(spec model.RecipeSpec, contractVersion string, seed model.RecipeSeed) model.Recipe {
	value := model.Recipe{ContractVersion: contractVersion, Title: seed.Title, Outcome: seed.Outcome, Audience: "coding_agent"}
	if contractVersion == model.RecipeContractProductIntegrationV2 {
		value.IntegrationID = spec.IntegrationID
	} else {
		value.APIAttachments = append([]model.RecipeAPIAttachment(nil), spec.APIAttachments...)
	}
	return value
}

func (s *Service) reviewRecipe(ctx context.Context, product model.Product, spec model.RecipeSpec, markdown string, selectedEvidence []model.IntegrationEvidence, findings []model.RecipeValidationFinding) (string, []model.RecipeValidationFinding) {
	reviewInput := map[string]any{
		"recipe_spec":            spec,
		"rendered_markdown":      markdown,
		"product_evidence":       selectedEvidence,
		"allowed_evidence_ids":   evidenceIDs(selectedEvidence),
		"deterministic_findings": findings,
	}
	prompt, _ := json.Marshal(reviewInput)
	result, err := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_review", PromptKey: AIPromptKeyRecipeReview, User: string(prompt), SchemaName: "recipe_review", Schema: recipeReviewSchema, MaxOutput: 2048, Temperature: 0})
	if err != nil {
		return "AI review was unavailable; human review is required.", append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_unavailable", Message: "The advisory review did not complete. Review every claim before approval."})
	}
	var response recipeReviewResponse
	if decodeStrictAIResult(result.JSON, &response) != nil {
		return "AI review returned an invalid result; human review is required.", append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_invalid", Message: "The advisory review was invalid. Review every claim before approval."})
	}
	advisoryFindings, valid := recipeReviewValidationFindings(response, selectedEvidence)
	if !valid {
		return "AI review returned an invalid result; human review is required.", append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_invalid", Message: "The advisory review was invalid. Review every claim before approval."})
	}
	findings = append(findings, advisoryFindings...)
	if response.Recommendation == "revise" && len(advisoryFindings) == 0 {
		findings = append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_revise", Message: "The advisory reviewer recommends revision but returned no usable finding; inspect every claim before approval."})
	}
	if response.Recommendation == "revise" {
		return "Advisory AI review recommends revision; inspect the server-owned findings before approval.", findings
	}
	return "Advisory AI review found no additional issue; human review is still required.", findings
}

var recipeReviewFindingMessages = map[string]string{
	"delivery_scope":        "The plan includes connector-delivery or platform-administration work instead of only the product integration.",
	"multiple_capabilities": "The selected capabilities do not form one coherent minimal workflow, or include an API the workflow does not require.",
	"sdk_scope":             "The plan makes an SDK claim without an exact reviewed operation-to-SDK binding.",
	"non_actionable_step":   "At least one step does not make a tangible change in the consuming project.",
	"unobservable_check":    "At least one verification check lacks an observable pass condition.",
	"unsupported_claim":     "At least one material implementation claim is not stated by its selected evidence.",
	"unsafe_content":        "The plan contains unsafe content or requests credential handling.",
	"not_minimal":           "The plan contains redundant or nonessential work.",
	"evidence_gap":          "The selected evidence is insufficient or conflicting for a material instruction.",
}

// recipeReviewValidationFindings converts the model's closed selection into
// server-owned findings. Model prose never crosses this boundary, and every
// persisted finding carries the exact immutable evidence fingerprint that was
// reviewed. Duplicate codes are merged without changing their first-seen
// order; malformed or ambiguous evidence fails the whole advisory result.
func recipeReviewValidationFindings(response recipeReviewResponse, selectedEvidence []model.IntegrationEvidence) ([]model.RecipeValidationFinding, bool) {
	if response.Recommendation != "pass" && response.Recommendation != "revise" {
		return nil, false
	}
	if len(response.Findings) > 9 || response.Recommendation == "pass" && len(response.Findings) != 0 {
		return nil, false
	}

	byID, ambiguous := recipeUniqueEvidenceByID(selectedEvidence)
	if len(byID) == 0 {
		return nil, false
	}
	for _, item := range selectedEvidence {
		id := strings.TrimSpace(item.ResourceID)
		if id == "" || id != item.ResourceID || ambiguous[id] || strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Fingerprint) == "" {
			return nil, false
		}
	}
	byCode := make(map[string]*model.RecipeValidationFinding, len(response.Findings))
	seenEvidenceByCode := make(map[string]map[string]bool, len(response.Findings))
	orderedCodes := make([]string, 0, len(response.Findings))
	for _, selection := range response.Findings {
		message, supported := recipeReviewFindingMessages[selection.Code]
		if !supported || len(selection.EvidenceIDs) == 0 || len(selection.EvidenceIDs) > 8 {
			return nil, false
		}
		finding, exists := byCode[selection.Code]
		if !exists {
			orderedCodes = append(orderedCodes, selection.Code)
			finding = &model.RecipeValidationFinding{Level: "warning", Code: "ai_" + selection.Code, Message: message}
			byCode[selection.Code] = finding
			seenEvidenceByCode[selection.Code] = make(map[string]bool, len(selection.EvidenceIDs))
		}
		seenInSelection := make(map[string]bool, len(selection.EvidenceIDs))
		for _, evidenceID := range selection.EvidenceIDs {
			item, found := byID[evidenceID]
			if evidenceID == "" || evidenceID != strings.TrimSpace(evidenceID) || !found || ambiguous[evidenceID] || seenInSelection[evidenceID] || strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Fingerprint) == "" {
				return nil, false
			}
			seenInSelection[evidenceID] = true
			if seenEvidenceByCode[selection.Code][evidenceID] {
				return nil, false
			}
			if len(finding.Evidence) == 8 {
				return nil, false
			}
			finding.Evidence = append(finding.Evidence, model.RecipeEvidenceRef{Kind: item.Kind, ResourceID: item.ResourceID, Fingerprint: item.Fingerprint})
			seenEvidenceByCode[selection.Code][evidenceID] = true
		}
	}

	result := make([]model.RecipeValidationFinding, 0, len(orderedCodes))
	for _, code := range orderedCodes {
		result = append(result, *byCode[code])
	}
	return result, true
}
