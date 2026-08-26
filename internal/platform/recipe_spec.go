package platform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const maxRecipeMarkdownBytes = 12 << 10

func recipeProductEvidence(values []model.IntegrationEvidence) []model.IntegrationEvidence {
	result := make([]model.IntegrationEvidence, 0, len(values))
	for _, item := range values {
		switch item.Kind {
		case "integration", "resource_set", "source_publication", "sdk", "tool":
			result = append(result, item)
		}
	}
	return result
}

func recipeCapabilityEvidence(item model.IntegrationEvidence) bool {
	// A recipe needs one operation-shaped contract. A whole API resource set is
	// useful analysis context, but it does not identify one callable operation
	// and therefore cannot safely support implementation prose by itself.
	return item.Kind == "tool"
}

func recipeProductCapabilityIDs(values []model.IntegrationEvidence) []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, item := range recipeProductEvidence(values) {
		if recipeCapabilityEvidence(item) && item.ResourceID != "" && !seen[item.ResourceID] {
			result = append(result, item.ResourceID)
			seen[item.ResourceID] = true
		}
	}
	sort.Strings(result)
	return result
}

func recipeProductSDKIDs(values []model.IntegrationEvidence) []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, item := range recipeProductEvidence(values) {
		if item.Kind == "sdk" && item.ResourceID != "" && !seen[item.ResourceID] {
			result = append(result, item.ResourceID)
			seen[item.ResourceID] = true
		}
	}
	sort.Strings(result)
	return result
}

func recipeUniqueEvidenceByID(values []model.IntegrationEvidence) (map[string]model.IntegrationEvidence, map[string]bool) {
	byID := make(map[string]model.IntegrationEvidence, len(values))
	ambiguous := make(map[string]bool)
	for _, item := range values {
		id := strings.TrimSpace(item.ResourceID)
		if id == "" {
			continue
		}
		if _, exists := byID[id]; exists {
			ambiguous[id] = true
		}
		byID[id] = item
	}
	return byID, ambiguous
}

func recipeResolveProductSelection(analysis model.IntegrationAnalysis, seed model.RecipeSeed) ([]model.IntegrationEvidence, bool) {
	productEvidence := recipeProductEvidence(analysis.Evidence)
	byID, ambiguous := recipeUniqueEvidenceByID(productEvidence)
	selected := make([]model.IntegrationEvidence, 0, len(seed.EvidenceIDs))
	seen := make(map[string]bool)
	for _, rawID := range seed.EvidenceIDs {
		id := strings.TrimSpace(rawID)
		item, exists := byID[id]
		if id == "" || !exists || ambiguous[id] || seen[id] {
			return nil, false
		}
		selected = append(selected, item)
		seen[id] = true
	}
	if len(selected) == 0 {
		return nil, false
	}
	capabilitySet := make(map[string]bool, len(seed.CapabilityIDs))
	for _, rawID := range seed.CapabilityIDs {
		id := strings.TrimSpace(rawID)
		item, exists := byID[id]
		if id == "" || !exists || ambiguous[id] || !recipeCapabilityEvidence(item) || capabilitySet[id] || !seen[id] {
			return nil, false
		}
		capabilitySet[id] = true
	}
	if len(capabilitySet) != 1 {
		return nil, false
	}
	if seed.SDKID != "" {
		sdk, exists := byID[seed.SDKID]
		if !exists || ambiguous[seed.SDKID] || sdk.Kind != "sdk" || !seen[seed.SDKID] {
			return nil, false
		}
	}
	return selected, true
}

func recipeSelectedCapabilitySupportsMCP(seed model.RecipeSeed, allowed map[string]model.IntegrationEvidence) bool {
	for _, id := range seed.CapabilityIDs {
		item, ok := allowed[id]
		// Backend is a server-owned structured line in tool evidence. Never
		// infer this exception from labels or descriptions, which are untrusted
		// product prose and could contain an injected MCP keyword.
		if ok && item.Kind == "tool" && strings.EqualFold(recipeEvidenceField(item.Excerpt, "Backend"), "mcp") {
			return true
		}
	}
	return false
}

func recipeIntentMatchesProductSelection(title, outcome string, productMCP bool) bool {
	if !recipeProductIntentTextValid(title, outcome) {
		return false
	}
	return productMCP || !strings.Contains(strings.ToLower(title+" "+outcome), "mcp")
}

func recipeInstructionContainsInternalValue(value string, allowed map[string]model.IntegrationEvidence) bool {
	for _, item := range allowed {
		if item.ResourceID != "" && strings.Contains(value, item.ResourceID) {
			return true
		}
		if item.Fingerprint != "" && strings.Contains(value, item.Fingerprint) {
			return true
		}
	}
	return false
}

func recipeSDKPrerequisite(seed model.RecipeSeed, allowed map[string]model.IntegrationEvidence) (model.RecipeInstruction, bool) {
	if seed.SDKID == "" {
		return model.RecipeInstruction{}, false
	}
	item, ok := allowed[seed.SDKID]
	if !ok || item.Kind != "sdk" {
		return model.RecipeInstruction{}, false
	}
	coordinate := recipeEvidenceField(item.Excerpt, "Coordinate")
	version := recipeEvidenceField(item.Excerpt, "Exact version")
	command := recipeEvidenceField(item.Excerpt, "Install")
	if coordinate == "" || version == "" || command == "" {
		return model.RecipeInstruction{}, false
	}
	return model.RecipeInstruction{
		Action:         fmt.Sprintf("Install %s at exact version %s with %s.", recipeCode(coordinate), recipeCode(version), recipeCode(command)),
		ExpectedResult: fmt.Sprintf("The consuming project resolves %s at exactly %s.", recipeCode(coordinate), recipeCode(version)),
		Evidence:       []model.RecipeEvidenceRef{{Kind: item.Kind, ResourceID: item.ResourceID, Fingerprint: item.Fingerprint}},
	}, true
}

func recipeInstructionChangesProject(value model.RecipeInstruction) bool {
	lower := strings.ToLower(value.Action)
	for _, negated := range []string{"do not ", "don't ", "never ", "without ", "avoid "} {
		if strings.Contains(lower, negated) {
			return false
		}
	}
	for _, verb := range []string{"add ", "create ", "implement ", "install ", "configure ", "initialize ", "initialise ", "import ", "register ", "update ", "write ", "wire ", "replace ", "remove "} {
		if strings.Contains(lower, verb) {
			return true
		}
	}
	return false
}

func recipeInstructionGroundedURLs(allowed map[string]model.IntegrationEvidence) map[string]bool {
	result := make(map[string]bool)
	for _, item := range allowed {
		for _, value := range []string{item.Location, recipeEvidenceField(item.Excerpt, "Fixed endpoint")} {
			if strings.HasPrefix(value, "https://") {
				result[value] = true
			}
		}
		for _, reference := range item.References {
			if strings.HasPrefix(reference.URL, "https://") {
				result[reference.URL] = true
			}
		}
	}
	return result
}

func recipeInstructionTextValid(item model.RecipeInstruction, allowed map[string]model.IntegrationEvidence, requireExpected, productMCP bool) bool {
	action := strings.TrimSpace(item.Action)
	expected := strings.TrimSpace(item.ExpectedResult)
	if action == "" || action != strings.Join(strings.Fields(action), " ") || len(action) > 500 || requireExpected && expected == "" || expected != strings.Join(strings.Fields(expected), " ") || len(expected) > 500 {
		return false
	}
	combined := action + " " + expected
	lower := strings.ToLower(combined)
	if containsAISecretText(combined) || containsRecipeRawHTML(combined) || recipeContainsUnsupportedURI(combined) || recipeInstructionContainsInternalValue(combined, allowed) {
		return false
	}
	for _, forbidden := range []string{"dokosoko", "catalog revision", "publication revision", "integrationplan", "evidence id", "evidence_id", "mcp-facing"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	if !productMCP {
		for _, delivery := range []string{"connect to mcp", "mcp client", "mcp transport", "mcp discovery", "private mcp", "public mcp", "protected-resource metadata", "resources/list", "resources/read", "pkce", "`/mcp`", " /mcp"} {
			if strings.Contains(lower, delivery) {
				return false
			}
		}
	}
	for _, vague := range []string{"read the docs", "review the documentation", "discover tools", "review the catalog", "configure as needed", "choose an option", "follow best practices"} {
		if strings.Contains(lower, vague) {
			return false
		}
	}
	groundedURLs := recipeInstructionGroundedURLs(allowed)
	for _, raw := range recipeURLPattern.FindAllString(combined, -1) {
		if !groundedURLs[strings.TrimRight(raw, ".,;:`")] {
			return false
		}
	}
	return true
}

func deterministicRecipeSpec(analysis model.IntegrationAnalysis, seed model.RecipeSeed) (model.RecipeSpec, error) {
	selected, ok := recipeResolveProductSelection(analysis, seed)
	if !ok || len(seed.CapabilityIDs) != 1 {
		return model.RecipeSpec{}, ErrRecipeNeedsInput
	}
	allowed, ambiguous := recipeUniqueEvidenceByID(selected)
	for id := range ambiguous {
		delete(allowed, id)
	}
	integrationID, scoped := integrationScopeID(analysis.Evidence)
	if !scoped || integrationID == "" {
		return model.RecipeSpec{}, ErrRecipeNeedsInput
	}
	spec := model.RecipeSpec{SchemaVersion: model.RecipeSpecVersion2, IntegrationID: integrationID, Title: seed.Title, Outcome: seed.Outcome, SDKID: seed.SDKID, CapabilityIDs: append([]string(nil), seed.CapabilityIDs...)}
	if sdk, hasSDK := recipeSDKPrerequisite(seed, allowed); seed.SDKID != "" {
		if !hasSDK {
			return model.RecipeSpec{}, ErrRecipeNeedsInput
		}
		spec.Prerequisites = append(spec.Prerequisites, sdk)
	}
	for _, capabilityID := range seed.CapabilityIDs {
		item, exists := allowed[capabilityID]
		if !exists || item.Kind != "tool" {
			return model.RecipeSpec{}, ErrRecipeNeedsInput
		}
		backend := strings.ToLower(recipeEvidenceField(item.Excerpt, "Backend"))
		method := strings.ToUpper(recipeEvidenceField(item.Excerpt, "Method"))
		endpoint := recipeEvidenceField(item.Excerpt, "Fixed endpoint")
		ref := []model.RecipeEvidenceRef{{Kind: item.Kind, ResourceID: item.ResourceID, Fingerprint: item.Fingerprint}}
		implementation := ""
		switch backend {
		case "http":
			if method == "" || endpoint == "" {
				return model.RecipeSpec{}, ErrRecipeNeedsInput
			}
			implementation = fmt.Sprintf("Add a product client operation for %s that sends %s to %s.", recipeCode(item.Label), recipeCode(method), recipeCode(endpoint))
			if seed.SDKID != "" {
				sdk := allowed[seed.SDKID]
				implementation = fmt.Sprintf("Implement %s through %s at exact version %s and map it to %s %s.", recipeCode(item.Label), recipeCode(recipeEvidenceField(sdk.Excerpt, "Coordinate")), recipeCode(recipeEvidenceField(sdk.Excerpt, "Exact version")), recipeCode(method), recipeCode(endpoint))
				ref = append(ref, model.RecipeEvidenceRef{Kind: sdk.Kind, ResourceID: sdk.ResourceID, Fingerprint: sdk.Fingerprint})
			}
		case "mcp":
			implementation = fmt.Sprintf("Add a product client operation for %s that invokes the reviewed product tool with inputs matching its schema.", recipeCode(item.Label))
		default:
			return model.RecipeSpec{}, ErrRecipeNeedsInput
		}
		spec.Steps = append(spec.Steps,
			model.RecipeInstruction{Action: implementation, ExpectedResult: "The consuming project has one explicit product integration boundary for this operation.", Evidence: ref},
			model.RecipeInstruction{Action: fmt.Sprintf("Map the inputs and returned value for %s to the consuming project's existing types and error model.", recipeCode(item.Label)), ExpectedResult: "Inputs and results remain bounded by the reviewed operation schemas.", Evidence: []model.RecipeEvidenceRef{{Kind: item.Kind, ResourceID: item.ResourceID, Fingerprint: item.Fingerprint}}},
		)
		spec.Checks = append(spec.Checks, model.RecipeInstruction{Action: fmt.Sprintf("Run a focused test for %s with non-secret fixtures that match its reviewed input and output schemas.", recipeCode(item.Label)), ExpectedResult: "The test exercises the product operation and rejects an invalid result shape.", Evidence: []model.RecipeEvidenceRef{{Kind: item.Kind, ResourceID: item.ResourceID, Fingerprint: item.Fingerprint}}})
	}
	return spec, nil
}

func equalRecipeInstructions(left, right []model.RecipeInstruction) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Action != right[index].Action || left[index].ExpectedResult != right[index].ExpectedResult || len(left[index].Evidence) != len(right[index].Evidence) {
			return false
		}
		for evidenceIndex := range left[index].Evidence {
			if left[index].Evidence[evidenceIndex] != right[index].Evidence[evidenceIndex] {
				return false
			}
		}
	}
	return true
}

func canonicalRecipeInstructions(spec model.RecipeSpec, selectedEvidence []model.IntegrationEvidence) (model.RecipeSpec, error) {
	productEvidence := recipeProductEvidence(selectedEvidence)
	evidence := make([]model.IntegrationEvidence, 0, len(productEvidence)+1)
	evidence = append(evidence, model.IntegrationEvidence{Kind: integrationScopeEvidenceKind, ResourceID: spec.IntegrationID, Fingerprint: "validation-scope"})
	evidence = append(evidence, productEvidence...)
	evidenceIDs := make([]string, 0, len(productEvidence))
	for _, item := range productEvidence {
		if strings.TrimSpace(item.ResourceID) != "" {
			evidenceIDs = append(evidenceIDs, item.ResourceID)
		}
	}
	return deterministicRecipeSpec(model.IntegrationAnalysis{Evidence: evidence}, model.RecipeSeed{
		Slug:          "canonical-validation",
		Title:         spec.Title,
		Outcome:       spec.Outcome,
		Audience:      "coding_agent",
		CapabilityIDs: append([]string(nil), spec.CapabilityIDs...),
		SDKID:         spec.SDKID,
		EvidenceIDs:   evidenceIDs,
	})
}

func renderRecipeSpec(spec model.RecipeSpec, references []model.RecipeReference) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n## Outcome\n\n%s\n", spec.Title, spec.Outcome)
	if len(spec.Prerequisites) > 0 {
		builder.WriteString("\n## Prerequisites\n")
		for _, item := range spec.Prerequisites {
			fmt.Fprintf(&builder, "\n- %s", item.Action)
			if item.ExpectedResult != "" {
				fmt.Fprintf(&builder, " Expected: %s", item.ExpectedResult)
			}
		}
		builder.WriteByte('\n')
	}
	builder.WriteString("\n## Steps\n")
	for index, item := range spec.Steps {
		fmt.Fprintf(&builder, "\n%d. %s Expected: %s", index+1, item.Action, item.ExpectedResult)
	}
	builder.WriteString("\n\n## Verify\n")
	for _, item := range spec.Checks {
		fmt.Fprintf(&builder, "\n- %s Expected: %s", item.Action, item.ExpectedResult)
	}
	if len(references) > 0 {
		builder.WriteString("\n\n## References\n")
		for _, reference := range references {
			fmt.Fprintf(&builder, "\n- [%s](%s)", recipeLinkLabel(reference.Label), reference.URL)
		}
	}
	builder.WriteByte('\n')
	return builder.String()
}

func validateRecipeSpec(spec model.RecipeSpec, recipe model.Recipe, selectedEvidence []model.IntegrationEvidence) []model.RecipeValidationFinding {
	findings := make([]model.RecipeValidationFinding, 0)
	if spec.SchemaVersion != model.RecipeSpecVersion2 || spec.IntegrationID == "" || spec.IntegrationID != recipe.IntegrationID || spec.Title != recipe.Title || spec.Outcome != recipe.Outcome {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_spec", Message: "The structured recipe does not match its product integration, title, outcome, or current schema."})
	}
	if len(spec.Prerequisites) > 4 || len(spec.Steps) < 2 || len(spec.Steps) > 8 || len(spec.Checks) < 1 || len(spec.Checks) > 3 || len(spec.CapabilityIDs) != 1 || len(spec.ReferenceIDs) > 8 {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_shape", Message: "Keep the recipe to 0-4 prerequisites, 2-8 steps, 1-3 checks, at most 8 references, and exactly one product capability."})
	}
	allowed, ambiguous := recipeUniqueEvidenceByID(recipeProductEvidence(selectedEvidence))
	for id := range ambiguous {
		delete(allowed, id)
	}
	capabilities := make(map[string]bool, len(spec.CapabilityIDs))
	for _, id := range spec.CapabilityIDs {
		item, ok := allowed[id]
		if id == "" || capabilities[id] || !ok || !recipeCapabilityEvidence(item) {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_capability", Message: "Every recipe capability must select one exact reviewed product operation or API contract."})
			break
		}
		capabilities[id] = true
	}
	if spec.SDKID != "" {
		if item, ok := allowed[spec.SDKID]; !ok || item.Kind != "sdk" {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_sdk", Message: "The recipe SDK must be one exact reviewed SDK reference."})
		} else if expected, valid := recipeSDKPrerequisite(model.RecipeSeed{SDKID: spec.SDKID}, allowed); !valid || len(spec.Prerequisites) == 0 || spec.Prerequisites[0].Action != expected.Action || spec.Prerequisites[0].ExpectedResult != expected.ExpectedResult || len(spec.Prerequisites[0].Evidence) != 1 || spec.Prerequisites[0].Evidence[0] != expected.Evidence[0] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_sdk_install", Message: "The first prerequisite must use the exact reviewed SDK coordinate, version, and canonical install command."})
		}
	}
	canonical, canonicalErr := canonicalRecipeInstructions(spec, selectedEvidence)
	if canonicalErr != nil || !equalRecipeInstructions(spec.Prerequisites, canonical.Prerequisites) || !equalRecipeInstructions(spec.Steps, canonical.Steps) || !equalRecipeInstructions(spec.Checks, canonical.Checks) {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "noncanonical_recipe_instructions", Message: "Recipe instructions must use the server-owned operation template derived from the exact reviewed tool and SDK."})
	}
	changesProject := false
	coveredCapabilities := make(map[string]bool)
	seenInstructions := make(map[string]bool)
	productMCP := recipeSelectedCapabilitySupportsMCP(model.RecipeSeed{CapabilityIDs: spec.CapabilityIDs}, allowed)
	if !recipeIntentMatchesProductSelection(spec.Title, spec.Outcome, productMCP) {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_intent", Message: "The title and outcome must describe the selected product capability, not MCP delivery or platform setup."})
	}
	validateInstruction := func(item model.RecipeInstruction, requireExpected, implementationStep bool) {
		key := strings.ToLower(item.Action + "\x00" + item.ExpectedResult)
		combined := item.Action + " " + item.ExpectedResult
		if seenInstructions[key] || !recipeInstructionTextValid(item, allowed, requireExpected, productMCP) || len(item.Evidence) == 0 || len(item.Evidence) > 8 || strings.Contains(combined, spec.IntegrationID) {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_instruction", Message: "Every instruction must be concise, evidence-backed, and include an observable expected result where required."})
			return
		}
		seenInstructions[key] = true
		seenEvidence := make(map[model.RecipeEvidenceRef]bool, len(item.Evidence))
		for _, ref := range item.Evidence {
			evidence, ok := allowed[ref.ResourceID]
			if !ok || evidence.Kind != ref.Kind || evidence.Fingerprint != ref.Fingerprint || seenEvidence[ref] {
				findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_evidence", Message: "An instruction references evidence outside the exact selected product snapshot."})
				return
			}
			seenEvidence[ref] = true
			if implementationStep {
				coveredCapabilities[ref.ResourceID] = true
			}
		}
	}
	for _, item := range spec.Prerequisites {
		validateInstruction(item, false, false)
	}
	for _, item := range spec.Steps {
		validateInstruction(item, true, true)
		changesProject = changesProject || recipeInstructionChangesProject(item)
	}
	for _, item := range spec.Checks {
		validateInstruction(item, true, false)
	}
	if !changesProject {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "recipe_not_actionable", Message: "At least one step must change or configure the consuming project."})
	}
	for id := range capabilities {
		if !coveredCapabilities[id] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "uncovered_recipe_capability", Message: "Every selected product capability must support at least one implementation step."})
			break
		}
	}
	allowedReferences := make(map[string]bool)
	for _, reference := range recipeReferences(selectedEvidence) {
		allowedReferences[reference.ResourceID] = reference.ResourceID != ""
	}
	seenReferenceIDs := make(map[string]bool, len(spec.ReferenceIDs))
	for _, referenceID := range spec.ReferenceIDs {
		if referenceID == "" || !allowedReferences[referenceID] || seenReferenceIDs[referenceID] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "invalid_recipe_reference", Message: "Every reference must select one exact reviewed product document."})
			break
		}
		seenReferenceIDs[referenceID] = true
	}
	return findings
}
