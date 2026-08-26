package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

func recipeReferences(evidence []model.IntegrationEvidence) []model.RecipeReference {
	values := make([]model.RecipeReference, 0)
	seen := make(map[string]bool)
	for _, item := range evidence {
		parsed, err := url.Parse(item.Location)
		if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && !seen[item.Location] {
			values = append(values, model.RecipeReference{Label: item.Label, URL: item.Location, Kind: recipeReferenceKind(item.Label, item.Location), ResourceID: item.ResourceID})
			seen[item.Location] = true
		}
		for _, reference := range item.References {
			parsed, err := url.Parse(reference.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || seen[reference.URL] {
				continue
			}
			values = append(values, reference)
			seen[reference.URL] = true
		}
	}
	return values
}

func recipeDependencies(evidence []model.IntegrationEvidence) []model.RecipeDependency {
	values := make([]model.RecipeDependency, 0, len(evidence))
	for _, item := range evidence {
		values = append(values, model.RecipeDependency{Kind: item.Kind, ResourceID: item.ResourceID, Version: item.Fingerprint})
	}
	return values
}

// recipeSelectedEvidence resolves an exact, server-issued evidence-ID
// selection. Empty selections are accepted only for records written before the
// selection contract existed and conservatively resolve to the full snapshot.
func recipeSelectedEvidence(evidence []model.IntegrationEvidence, evidenceIDs []string) ([]model.IntegrationEvidence, bool) {
	if len(evidenceIDs) == 0 {
		return append([]model.IntegrationEvidence(nil), evidence...), true
	}
	byID := make(map[string][]model.IntegrationEvidence, len(evidence))
	for _, item := range evidence {
		id := strings.TrimSpace(item.ResourceID)
		if id == "" {
			continue
		}
		byID[id] = append(byID[id], item)
	}
	selected := make([]model.IntegrationEvidence, 0, len(evidenceIDs))
	seen := make(map[string]bool, len(evidenceIDs))
	for _, rawID := range evidenceIDs {
		id := strings.TrimSpace(rawID)
		items, exists := byID[id]
		if id == "" || !exists || seen[id] {
			return nil, false
		}
		selected = append(selected, items...)
		seen[id] = true
	}
	return selected, true
}

// recipeEvidenceForDependencies recovers the persisted selection from a
// recipe. Exact kind, resource ID, and fingerprint matching prevents an old or
// missing resource from silently widening the recipe to other evidence.
func recipeEvidenceForDependencies(evidence []model.IntegrationEvidence, dependencies []model.RecipeDependency) ([]model.IntegrationEvidence, bool) {
	byDependency := make(map[model.RecipeDependency]model.IntegrationEvidence, len(evidence))
	for _, item := range evidence {
		dependency := model.RecipeDependency{Kind: item.Kind, ResourceID: item.ResourceID, Version: item.Fingerprint}
		if _, exists := byDependency[dependency]; exists {
			return nil, false
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
		if !exists || seen[dependency] {
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
	seen := make(map[string]bool, len(selected))
	for _, item := range selected {
		if !seen[item.ResourceID] {
			ids = append(ids, item.ResourceID)
			seen[item.ResourceID] = true
		}
	}
	return ids, true
}

func recipeAnalysisWithEvidence(analysis model.IntegrationAnalysis, evidence []model.IntegrationEvidence) model.IntegrationAnalysis {
	analysis.Evidence = evidence
	return analysis
}

func recipeGroundingDependenciesForContract(analysis model.IntegrationAnalysis, seed model.RecipeSeed, authoringContract string) []model.RecipeDependency {
	selected, ok := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs)
	if !ok {
		selected = nil
	}
	values := recipeDependencies(selected)
	input, _ := json.Marshal(struct {
		AuthoringContract string                      `json:"authoring_contract"`
		Plan              model.IntegrationPlan       `json:"plan"`
		Evidence          []model.IntegrationEvidence `json:"evidence"`
		Seed              model.RecipeSeed            `json:"seed"`
	}{AuthoringContract: authoringContract, Plan: analysis.Plan, Evidence: selected, Seed: seed})
	return append(values, model.RecipeDependency{Kind: recipeAuthoringInputDependencyKind, ResourceID: seed.Slug, Version: evidenceFingerprint(recipeAuthoringInputDependencyKind, string(input))})
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
	if recipe.AnalysisID != analysis.ID || recipe.CurrentRevisionID == "" || recipe.CurrentRevision == nil || recipe.Title != seed.Title || recipe.Outcome != seed.Outcome || recipe.Audience != seed.Audience {
		return false
	}
	if _, ok := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs); !ok {
		return false
	}
	return recipeDependencySetsMatch(recipe.Dependencies, recipeGroundingDependencies(analysis, seed))
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

func recipeSelectedEndpoints(plan model.IntegrationPlan, seed model.RecipeSeed) []model.IntegrationEndpointPlan {
	if len(seed.EndpointIDs) == 0 {
		return nil
	}
	byName := make(map[string]model.IntegrationEndpointPlan, len(plan.Endpoints))
	for _, endpoint := range plan.Endpoints {
		byName[endpoint.Name] = endpoint
	}
	values := make([]model.IntegrationEndpointPlan, 0, len(seed.EndpointIDs))
	seen := make(map[string]bool, len(seed.EndpointIDs))
	for _, endpointID := range seed.EndpointIDs {
		if endpoint, ok := byName[endpointID]; ok && !seen[endpointID] {
			values = append(values, endpoint)
			seen[endpointID] = true
		}
	}
	return values
}

func recipeJSONSchema(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !json.Valid([]byte(raw)) {
		return ""
	}
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return ""
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(formatted)
}

func recipeSchemaAcceptsEmptyObject(raw string) bool {
	var schema struct {
		Type          string                     `json:"type"`
		Required      []string                   `json:"required"`
		Properties    map[string]json.RawMessage `json:"properties"`
		MinProperties int                        `json:"minProperties"`
	}
	return json.Unmarshal([]byte(raw), &schema) == nil && schema.Type == "object" && len(schema.Required) == 0 && len(schema.Properties) == 0 && schema.MinProperties == 0
}

func writeRecipeIndentedJSON(builder *strings.Builder, value string) {
	for _, line := range strings.Split(value, "\n") {
		builder.WriteString("\n       " + line)
	}
}

func recipeCommaSeparatedValues(value string) []string {
	values := make([]string, 0)
	seen := make(map[string]bool)
	for _, raw := range strings.Split(value, ",") {
		item := strings.TrimSpace(raw)
		if item != "" && !seen[item] {
			values = append(values, item)
			seen[item] = true
		}
	}
	return values
}

func recipeGroundedURLs(analysis model.IntegrationAnalysis) []string {
	values := make([]string, 0)
	seen := make(map[string]bool)
	add := func(candidate string) {
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && !seen[candidate] {
			values = append(values, candidate)
			seen[candidate] = true
		}
	}
	add(analysis.Plan.Identity.Issuer)
	add(analysis.Plan.Identity.Audience)
	for _, item := range analysis.Evidence {
		if item.Kind != "tool" {
			continue
		}
		add(recipeEvidenceField(item.Excerpt, "Fixed endpoint"))
	}
	return values
}

type recipeIdentityEvidence struct {
	Issuer            string
	Audience          string
	Scopes            []string
	OrganisationClaim string
}

func recipeIdentityFromEvidence(analysis model.IntegrationAnalysis) recipeIdentityEvidence {
	value := recipeIdentityEvidence{Issuer: analysis.Plan.Identity.Issuer, Audience: analysis.Plan.Identity.Audience}
	for _, item := range analysis.Evidence {
		if item.Kind != "identity_provider" {
			continue
		}
		value.Issuer = firstNonEmpty(recipeEvidenceField(item.Excerpt, "Issuer"), value.Issuer)
		value.Audience = firstNonEmpty(recipeEvidenceField(item.Excerpt, "Audience"), value.Audience)
		value.Scopes = recipeCommaSeparatedValues(recipeEvidenceField(item.Excerpt, "Scopes"))
		value.OrganisationClaim = recipeEvidenceField(item.Excerpt, "Customer account claim")
		break
	}
	return value
}

func recipeToolRequiredGrants(item model.IntegrationEvidence) []string {
	if item.Kind != "tool" {
		return nil
	}
	var policy ToolPolicy
	if json.Unmarshal([]byte(recipeEvidenceField(item.Excerpt, "Authorization policy")), &policy) != nil {
		return nil
	}
	values := make([]string, 0, len(policy.RequiredGrants))
	seen := make(map[string]bool, len(policy.RequiredGrants))
	for _, grant := range policy.RequiredGrants {
		grant = strings.ToLower(strings.TrimSpace(grant))
		if grant != "" && !seen[grant] && authorizationKeyPattern.MatchString(grant) {
			values = append(values, grant)
			seen[grant] = true
		}
	}
	return values
}

func recipeToolEvidence(values []model.IntegrationEvidence) []model.IntegrationEvidence {
	result := make([]model.IntegrationEvidence, 0)
	for _, item := range values {
		if item.Kind == "tool" || item.Kind == "automatic_tool" {
			result = append(result, item)
		}
	}
	priority := func(item model.IntegrationEvidence) int {
		switch {
		case strings.HasSuffix(item.Label, ".knowledge.search"):
			return 10
		case item.Kind == "tool":
			return 20
		case strings.HasSuffix(item.Label, ".instances.list"):
			return 30
		case strings.HasSuffix(item.Label, ".credentials.list"):
			return 40
		case strings.HasSuffix(item.Label, ".credentials.rotate"):
			return 50
		case strings.HasSuffix(item.Label, ".credentials.revoke"):
			return 60
		default:
			return 70
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := priority(result[i]), priority(result[j])
		if left == right {
			return result[i].Label < result[j].Label
		}
		return left < right
	})
	return result
}

func deterministicRecipeMarkdown(product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, references []model.RecipeReference) string {
	selectedEvidence, ok := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs)
	if !ok {
		selectedEvidence = nil
	}
	analysis = recipeAnalysisWithEvidence(analysis, selectedEvidence)
	references = recipeReferences(selectedEvidence)
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n## Outcome\n\n%s\n\n## Before you start\n", seed.Title, seed.Outcome)

	endpoints := recipeSelectedEndpoints(analysis.Plan, seed)
	missingEndpoints := len(endpoints) == 0
	identityEvidence := recipeIdentityFromEvidence(analysis)
	if missingEndpoints {
		fmt.Fprintf(&builder, "\n%s\n\n- No exact IntegrationPlan endpoint is selected for this recipe seed. Select an endpoint ID before implementation.", recipeMissingEndpointMarker)
	}
	usesOAuth := false
	for _, endpoint := range endpoints {
		identity := strings.ToLower(strings.TrimSpace(endpoint.Identity))
		boundary := "private"
		if identity == "none" {
			boundary = "public"
		}
		if identity == "oauth2" {
			usesOAuth = true
		}
		fmt.Fprintf(&builder, "\n- Configure the MCP client for the %s endpoint %s: %s %s with identity mode %s.", boundary, recipeCode(endpoint.Name), recipeCode(strings.ToUpper(endpoint.Method)), recipeCode(endpoint.Path), recipeCode(identity))
		fmt.Fprintf(&builder, "\n- Resolve endpoint path %s against the DokoSoko deployment origin supplied by the operator; do not infer or hardcode a hostname.", recipeCode(endpoint.Path))
	}
	if usesOAuth {
		if identityEvidence.Issuer != "" || identityEvidence.Audience != "" {
			builder.WriteString("\n- Treat the OAuth 2.0/OIDC values as the configured upstream identity-provider boundary")
			if identityEvidence.Issuer != "" {
				fmt.Fprintf(&builder, ": issuer %s", recipeCode(identityEvidence.Issuer))
			}
			if identityEvidence.Audience != "" {
				fmt.Fprintf(&builder, ", audience %s", recipeCode(identityEvidence.Audience))
			}
			builder.WriteString(".")
		}
		builder.WriteString("\n- The MCP client authenticates to the private endpoint through DokoSoko; it does not integrate directly with the configured issuer or handle vendor access tokens.")
		if len(identityEvidence.Scopes) > 0 {
			builder.WriteString("\n- The configured identity provider declares scope")
			if len(identityEvidence.Scopes) > 1 {
				builder.WriteString("s")
			}
			for _, scope := range identityEvidence.Scopes {
				builder.WriteString(" " + recipeCode(scope))
			}
			builder.WriteString(".")
		}
	}
	for _, item := range analysis.Evidence {
		switch item.Kind {
		case "resource_set":
			resourceKind := firstNonEmpty(recipeEvidenceField(item.Excerpt, "Kind"), "catalog")
			fmt.Fprintf(&builder, "\n- Ground this implementation in the %s resource set %s (ID %s), exact revision %s.", recipeCode(resourceKind), recipeCode(item.Label), recipeCode(item.ResourceID), recipeCode(firstNonEmpty(item.Version, recipeEvidenceField(item.Excerpt, "Revision"))))
		case "source_publication":
			fmt.Fprintf(&builder, "\n- Use source publication %s (ID %s), publication revision %s.", recipeCode(item.Label), recipeCode(item.ResourceID), recipeCode(item.Version))
		case "tool":
			grants := recipeToolRequiredGrants(item)
			if len(grants) > 0 {
				fmt.Fprintf(&builder, "\n- Bound MCP tool %s declares required grant", recipeCode(item.Label))
				if len(grants) > 1 {
					builder.WriteString("s")
				}
				for _, grant := range grants {
					builder.WriteString(" " + recipeCode(grant))
				}
				builder.WriteString(" in its authorization policy.")
			}
		}
	}

	builder.WriteString("\n\n## Identity\n")
	if missingEndpoints {
		builder.WriteString("\n- No endpoint identity boundary is selected. Keep this recipe in review until an exact endpoint ID is chosen.")
	}
	for _, endpoint := range endpoints {
		identity := strings.ToLower(strings.TrimSpace(endpoint.Identity))
		if identity == "none" {
			fmt.Fprintf(&builder, "\n- %s %s is explicitly anonymous (%s).", recipeCode(strings.ToUpper(endpoint.Method)), recipeCode(endpoint.Path), recipeCode(endpoint.Name))
		} else {
			fmt.Fprintf(&builder, "\n- %s %s (%s) is private and requires %s.", recipeCode(strings.ToUpper(endpoint.Method)), recipeCode(endpoint.Path), recipeCode(endpoint.Name), recipeCode(identity))
		}
	}
	if usesOAuth {
		builder.WriteString("\n- DokoSoko is the MCP-facing authorization boundary and brokers customer sign-in through the configured upstream identity provider; the MCP client never handles the upstream access token")
		if identityEvidence.Issuer != "" {
			fmt.Fprintf(&builder, "; the configured upstream issuer is %s", recipeCode(identityEvidence.Issuer))
		}
		if identityEvidence.Audience != "" {
			fmt.Fprintf(&builder, " and its audience is %s", recipeCode(identityEvidence.Audience))
		}
		builder.WriteString(".")
		if identityEvidence.OrganisationClaim != "" {
			fmt.Fprintf(&builder, "\n- The configured identity provider identifies %s as its customer account claim.", recipeCode(identityEvidence.OrganisationClaim))
		}
	}
	for _, item := range analysis.Evidence {
		if missingEndpoints || item.Kind != "authorization_point" {
			continue
		}
		grants := recipeCommaSeparatedValues(recipeEvidenceField(item.Excerpt, "Required grants"))
		fmt.Fprintf(&builder, "\n- Authorization point %s (ID %s)", recipeCode(item.Label), recipeCode(item.ResourceID))
		if action := recipeEvidenceField(item.Excerpt, "Action"); action != "" {
			fmt.Fprintf(&builder, " governs action %s", recipeCode(action))
		}
		if len(grants) > 0 {
			builder.WriteString(" and requires grant")
			if len(grants) > 1 {
				builder.WriteString("s")
			}
			for index, grant := range grants {
				if index > 0 {
					builder.WriteString(",")
				}
				builder.WriteString(" " + recipeCode(grant))
			}
		}
		builder.WriteString(".")
	}

	builder.WriteString("\n\n## Implementation\n")
	step := 1
	if missingEndpoints {
		builder.WriteString("\n1. Select one exact endpoint ID from the reviewed IntegrationPlan before configuring transport, identity, authorization, or tools.")
		step++
	}
	for _, endpoint := range endpoints {
		fmt.Fprintf(&builder, "\n%d. Configure the MCP transport for %s by resolving %s against the DokoSoko deployment origin supplied by the operator, then use %s at that resolved endpoint.", step, recipeCode(endpoint.Name), recipeCode(endpoint.Path), recipeCode(strings.ToUpper(endpoint.Method)))
		step++
	}
	if usesOAuth {
		hasMCPContract := false
		for _, item := range analysis.Evidence {
			hasMCPContract = hasMCPContract || item.Kind == "mcp_oauth"
		}
		if hasMCPContract {
			fmt.Fprintf(&builder, "\n%d. Send an unauthenticated MCP request to obtain the protected-resource metadata URL, then resolve that metadata and the advertised DokoSoko authorization-server metadata. Register the loopback callback dynamically, start Authorization Code with PKCE `S256`, and exchange the returned code for a token scoped to the exact private MCP resource.", step)
		} else {
			fmt.Fprintf(&builder, "\n%d. Authenticate the private MCP request through DokoSoko. The MCP client does not integrate directly with the configured issuer or handle the upstream access token.", step)
		}
		step++
	}
	for _, item := range recipeToolEvidence(analysis.Evidence) {
		if missingEndpoints {
			continue
		}
		method := strings.ToUpper(recipeEvidenceField(item.Excerpt, "Method"))
		endpoint := recipeEvidenceField(item.Excerpt, "Fixed endpoint")
		inputSchema := recipeJSONSchema(recipeEvidenceField(item.Excerpt, "Input schema"))
		outputSchema := recipeJSONSchema(recipeEvidenceField(item.Excerpt, "Output schema"))
		if item.Kind == "automatic_tool" {
			fmt.Fprintf(&builder, "\n%d. Discover and invoke automatic MCP tool %s.", step, recipeCode(item.Label))
		} else {
			fmt.Fprintf(&builder, "\n%d. Discover and invoke the bound MCP tool %s.", step, recipeCode(item.Label))
		}
		if method != "" && endpoint != "" {
			fmt.Fprintf(&builder, " Its fixed backend operation is %s %s; invoke it through the MCP tool binding.", recipeCode(method), recipeCode(endpoint))
		}
		if grants := recipeToolRequiredGrants(item); len(grants) > 0 {
			builder.WriteString(" Its authorization policy requires grant")
			if len(grants) > 1 {
				builder.WriteString("s")
			}
			for _, grant := range grants {
				builder.WriteString(" " + recipeCode(grant))
			}
			builder.WriteString(".")
		}
		if inputSchema != "" {
			if recipeSchemaAcceptsEmptyObject(inputSchema) {
				builder.WriteString(" Send an empty object as the tool arguments.")
			} else {
				builder.WriteString(" Build tool arguments that validate against this exact input schema.")
			}
			builder.WriteString("\n")
			writeRecipeIndentedJSON(&builder, inputSchema)
		}
		if item.Kind == "automatic_tool" {
			details := strings.ToLower(item.Excerpt)
			switch {
			case strings.HasSuffix(item.Label, ".credentials.rotate"):
				builder.WriteString("\n\n   Choose the target environment, scopes, TTL, and optional prior credential ID, show those exact arguments to the operator, and proceed only after they approve issuance or replacement. Supply a stable `params._meta.idempotency_key`. The first mutation attempt returns a server-issued one-time confirmation challenge in error data; retry the exact same tool name, arguments, and idempotency key with `params._meta.confirmation_challenge` and `params._meta.confirmed=true`. Read credential material once from the successful retry, use the returned `environment_variable` name for the operator-selected secret destination, and never print or log the material.")
			case strings.HasSuffix(item.Label, ".credentials.revoke"):
				builder.WriteString("\n\n   After cutover, choose the exact credential ID, show the destructive target to the operator, and proceed only after approval. The first mutation attempt returns a separate one-time confirmation challenge in error data. Retry the exact same revoke arguments with `params._meta.confirmation_challenge` and `params._meta.confirmed=true`; never reuse a challenge from rotation.")
			case strings.HasSuffix(item.Label, ".credentials.list"):
				builder.WriteString("\n\n   Treat this result as metadata only: it contains states and fingerprints, never credential material.")
			case strings.Contains(details, "read-only"):
				builder.WriteString("\n\n   This operation is read-only and needs no confirmation or idempotency key.")
			}
		}
		if outputSchema != "" {
			builder.WriteString("\n\n   Validate the returned value against this exact output schema:\n")
			writeRecipeIndentedJSON(&builder, outputSchema)
		}
		step++
	}
	if step == 1 {
		builder.WriteString("\n1. Follow the exact endpoint and identity values in the reviewed integration plan; it does not contain a callable endpoint for this seed.")
	}

	builder.WriteString("\n\n## Verify\n")
	if missingEndpoints {
		builder.WriteString("\n- Keep this recipe in review until the seed selects an exact IntegrationPlan endpoint and its identity boundary.")
	}
	for _, endpoint := range endpoints {
		fmt.Fprintf(&builder, "\n- Confirm the client resolves %s against the operator-provided DokoSoko deployment origin and uses %s with identity mode %s.", recipeCode(endpoint.Path), recipeCode(strings.ToUpper(endpoint.Method)), recipeCode(strings.ToLower(endpoint.Identity)))
	}
	if usesOAuth {
		builder.WriteString("\n- Confirm protected-resource and authorization-server discovery resolve to DokoSoko, PKCE uses `S256`, the token is bound to the exact MCP resource, and the MCP client never handles the upstream identity-provider token.")
		if identityEvidence.OrganisationClaim != "" {
			fmt.Fprintf(&builder, "\n- Confirm the identity-provider configuration identifies %s as the customer account claim.", recipeCode(identityEvidence.OrganisationClaim))
		}
	}
	for _, item := range analysis.Evidence {
		if missingEndpoints {
			continue
		}
		if item.Kind == "authorization_point" {
			grants := recipeCommaSeparatedValues(recipeEvidenceField(item.Excerpt, "Required grants"))
			fmt.Fprintf(&builder, "\n- Confirm authorization-point configuration %s", recipeCode(item.Label))
			if action := recipeEvidenceField(item.Excerpt, "Action"); action != "" {
				fmt.Fprintf(&builder, " governs action %s", recipeCode(action))
			}
			if len(grants) > 0 {
				builder.WriteString(" and declares exact required grant set")
				for _, grant := range grants {
					builder.WriteString(" " + recipeCode(grant))
				}
			}
			builder.WriteString(".")
		}
		if item.Kind == "tool" || item.Kind == "automatic_tool" {
			fmt.Fprintf(&builder, "\n- Confirm MCP discovery exposes %s before invoking it; verify pinned revisions in DokoSoko's catalog rather than expecting a revision field in MCP discovery.", recipeCode(item.Label))
			if grants := recipeToolRequiredGrants(item); len(grants) > 0 {
				fmt.Fprintf(&builder, "\n- Confirm tool %s declares exact required grant set", recipeCode(item.Label))
				for _, grant := range grants {
					builder.WriteString(" " + recipeCode(grant))
				}
				builder.WriteString(" in its authorization policy.")
			}
			if outputSchema := recipeJSONSchema(recipeEvidenceField(item.Excerpt, "Output schema")); outputSchema != "" {
				fmt.Fprintf(&builder, "\n- Validate the observed %s result against the exact output schema above; report only values present in the actual response.", recipeCode(item.Label))
			}
			if item.Kind == "automatic_tool" && strings.HasSuffix(item.Label, ".credentials.rotate") {
				fmt.Fprintf(&builder, "\n- Confirm %s returns the expected environment-variable name and one-time material only after confirmation; redact the material from all verification output.", recipeCode(item.Label))
			}
			if item.Kind == "automatic_tool" && strings.HasSuffix(item.Label, ".credentials.revoke") {
				builder.WriteString("\n- List credential metadata again and confirm the selected credential is revoked without exposing material.")
			}
		}
	}
	if len(references) > 0 {
		builder.WriteString("\n## References\n")
		for _, reference := range references {
			fmt.Fprintf(&builder, "\n- [%s](%s)", recipeLinkLabel(reference.Label), reference.URL)
		}
		builder.WriteString("\n")
	}
	return builder.String()
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
			// Preserve standard Markdown URL autolinks such as <https://...>.
			// Other colon-qualified names are XML/HTML-like markup and fail closed.
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
	if len(trimmed) < 120 {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "recipe_too_short", Message: "Explain the outcome, implementation, and verification steps."})
	}
	if len(markdown) > 100_000 {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "recipe_too_long", Message: "Keep a recipe under 100,000 characters."})
	}
	if strings.Contains(markdown, recipeMissingEndpointMarker) {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "missing_endpoint_selection", Message: "Select an exact IntegrationPlan endpoint before implementing this recipe."})
	}
	titles, order, sections := recipeMarkdownSections(markdown)
	if len(titles) != 1 || titles[0] == "" {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "missing_title", Message: "Add one non-empty level-one recipe title."})
	} else if expectedTitle != "" && titles[0] != expectedTitle {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "title_mismatch", Message: "Keep the level-one title identical to the reviewed recipe title."})
	}
	requiredHeadings := []string{"outcome", "before you start", "identity", "implementation", "verify"}
	lastIndex := -1
	for _, heading := range requiredHeadings {
		body, exists := sections[heading]
		if !exists || body == "" {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "missing_section", Message: "Add the required non-empty " + heading + " section."})
			continue
		}
		index := -1
		for candidateIndex, candidate := range order {
			if candidate == heading {
				index = candidateIndex
				break
			}
		}
		if index <= lastIndex {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "section_order", Message: "Keep the required recipe sections in their documented order."})
			break
		}
		lastIndex = index
	}
	unsafeContent := containsRecipeRawHTML(markdown) || strings.Contains(lower, "http://") || containsToolBuilderSecretText(markdown)
	for _, unsafe := range []string{"javascript:", "data:", "authorization: bearer", "sk-proj-", "-----begin private key-----"} {
		if strings.Contains(lower, unsafe) {
			unsafeContent = true
			break
		}
	}
	if unsafeContent {
		findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_content", Message: "Remove raw HTML, executable markup, insecure links, or credential-like content."})
	}
	for _, reference := range references {
		parsed, err := url.Parse(reference.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_reference", Message: "Recipe references must use a fixed public HTTPS URL."})
		}
	}
	allowedURLs := make(map[string]bool, len(references)*2)
	for _, reference := range references {
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
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unsafe_reference", Message: "Markdown links must select an analysed HTTPS reference; embedded images are not allowed."})
			break
		}
	}
	for _, raw := range recipeURLPattern.FindAllString(markdown, -1) {
		candidate := strings.TrimRight(raw, ".,;:`")
		if !allowedURLs[candidate] {
			findings = append(findings, model.RecipeValidationFinding{Level: "error", Code: "unverified_reference", Message: "Every HTTPS URL in a recipe must be grounded in the analysis evidence or select an analysed source."})
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

func appendRecipeReferences(markdown string, references []model.RecipeReference) string {
	markdown = strings.TrimSpace(markdown)
	if len(references) == 0 {
		return markdown + "\n"
	}
	var builder strings.Builder
	builder.WriteString(markdown)
	builder.WriteString("\n\n## References\n")
	for _, reference := range references {
		fmt.Fprintf(&builder, "\n- [%s](%s)", recipeLinkLabel(reference.Label), reference.URL)
	}
	builder.WriteByte('\n')
	return builder.String()
}

func allowedUniqueEvidenceIDs(values []string, evidence []model.IntegrationEvidence) bool {
	if len(values) == 0 {
		return false
	}
	allowed := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		allowed[item.ResourceID] = item.ResourceID != ""
	}
	seen := make(map[string]bool, len(values))
	for _, id := range values {
		id = strings.TrimSpace(id)
		if id == "" || !allowed[id] || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func aiAuthoredRecipeMatchesSelection(markdown string, analysis model.IntegrationAnalysis, seed model.RecipeSeed) bool {
	selected := recipeSelectedEndpoints(analysis.Plan, seed)
	if len(selected) == 0 {
		return false
	}
	for _, endpoint := range selected {
		if !strings.Contains(markdown, endpoint.Path) || !strings.Contains(strings.ToUpper(markdown), strings.ToUpper(endpoint.Method)) || !strings.Contains(strings.ToLower(markdown), strings.ToLower(endpoint.Identity)) {
			return false
		}
	}
	selectedNames := make(map[string]bool, len(selected))
	for _, endpoint := range selected {
		selectedNames[endpoint.Name] = true
	}
	for _, endpoint := range analysis.Plan.Endpoints {
		if !selectedNames[endpoint.Name] && endpoint.Path != "/" && strings.Contains(markdown, endpoint.Path) {
			return false
		}
	}
	return true
}

func (s *Service) authorRecipe(ctx context.Context, product model.Product, analysis model.IntegrationAnalysis, seed model.RecipeSeed, instruction string) (string, []model.RecipeReference, string, string) {
	selectedEvidence, selectionOK := recipeSelectedEvidence(analysis.Evidence, seed.EvidenceIDs)
	selectedAnalysis := recipeAnalysisWithEvidence(analysis, selectedEvidence)
	allowed := recipeReferences(selectedEvidence)
	fallback := deterministicRecipeMarkdown(product, selectedAnalysis, seed, allowed)
	if !selectionOK {
		return fallback, nil, "deterministic", ""
	}
	if len(recipeSelectedEndpoints(analysis.Plan, seed)) == 0 {
		return fallback, allowed, "deterministic", ""
	}
	allowedEvidenceIDs := evidenceIDs(selectedEvidence)
	allowedReferenceIDs := make([]string, 0, len(allowed))
	for _, reference := range allowed {
		if reference.ResourceID != "" {
			allowedReferenceIDs = append(allowedReferenceIDs, reference.ResourceID)
		}
	}
	prompt, _ := json.Marshal(map[string]any{"product": map[string]string{"name": product.Name, "slug": product.Slug}, "platform_contract": analysis.Plan, "evidence": selectedEvidence, "recipe": seed, "allowed_evidence_ids": allowedEvidenceIDs, "allowed_reference_ids": allowedReferenceIDs, "editor_instruction": strings.TrimSpace(instruction)})
	result, err := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_authoring", PromptKey: AIPromptKeyRecipeAuthoring, User: string(prompt), SchemaName: "recipe", Schema: recipeAuthoringSchema, MaxOutput: 8192, Temperature: 0.1})
	if err != nil {
		return fallback, allowed, "deterministic", ""
	}
	var response recipeAuthoringResponse
	if decodeStrictAIResult(result.JSON, &response) != nil || strings.TrimSpace(response.Markdown) == "" || !allowedUniqueEvidenceIDs(response.EvidenceIDs, selectedEvidence) {
		return fallback, allowed, "deterministic", ""
	}
	allowedByID := make(map[string]model.RecipeReference, len(allowed))
	for _, reference := range allowed {
		allowedByID[reference.ResourceID] = reference
	}
	selected := make([]model.RecipeReference, 0, len(response.ReferenceIDs))
	seen := make(map[string]bool)
	for _, id := range response.ReferenceIDs {
		if reference, ok := allowedByID[id]; ok && !seen[id] {
			selected, seen[id] = append(selected, reference), true
		}
	}
	markdown := appendRecipeReferences(response.Markdown, selected)
	if !aiAuthoredRecipeMatchesSelection(markdown, analysis, seed) || hasRecipeErrors(validateRecipeMarkdown(markdown, seed.Title, selected, recipeGroundedURLs(selectedAnalysis)...)) {
		return fallback, allowed, "deterministic", ""
	}
	return markdown, selected, "ai", firstNonEmpty(result.ResolvedModel, result.RequestedModel)
}

func (s *Service) reviewRecipe(ctx context.Context, product model.Product, recipe model.Recipe, markdown string, findings []model.RecipeValidationFinding) (string, []model.RecipeValidationFinding) {
	reviewInput := map[string]any{"recipe": map[string]string{"title": recipe.Title, "outcome": recipe.Outcome, "audience": recipe.Audience}, "markdown": markdown, "deterministic_findings": findings}
	if analysis, analysisErr := s.store.IntegrationAnalysis(ctx, product.ID, recipe.AnalysisID); analysisErr == nil {
		reviewInput["integration_plan"] = analysis.Plan
		if selectedEvidence, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies); ok {
			reviewInput["evidence"] = selectedEvidence
		} else {
			reviewInput["evidence"] = []model.IntegrationEvidence{}
		}
	}
	prompt, _ := json.Marshal(reviewInput)
	result, err := s.generateAIStructured(ctx, aiInvocation{Product: product, Workload: airuntime.WorkloadAnalysis, Action: "recipe_review", PromptKey: AIPromptKeyRecipeReview, User: string(prompt), SchemaName: "recipe_review", Schema: recipeReviewSchema, MaxOutput: 4096, Temperature: 0})
	if err != nil {
		return "AI review was unavailable; human review is required.", append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_unavailable", Message: "The review workload did not complete. Review every claim before approval."})
	}
	var response recipeReviewResponse
	if decodeStrictAIResult(result.JSON, &response) != nil || (response.Recommendation != "pass" && response.Recommendation != "revise") || strings.TrimSpace(response.Summary) == "" || len(response.Summary) > 2000 {
		return "AI review returned an invalid result; human review is required.", append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_invalid", Message: "The review result was invalid. Review every claim before approval."})
	}
	seenFinding := make(map[string]bool)
	acceptedFindings := 0
	for _, finding := range response.Findings {
		finding.Level, finding.Code, finding.Message = strings.ToLower(strings.TrimSpace(finding.Level)), strings.ToLower(strings.TrimSpace(finding.Code)), strings.TrimSpace(finding.Message)
		if !map[string]bool{"info": true, "warning": true, "error": true}[finding.Level] || finding.Code == "" || len(finding.Code) > 80 || finding.Message == "" || len(finding.Message) > 500 || containsToolBuilderSecretText(finding.Message) {
			continue
		}
		if finding.Level == "error" {
			finding.Level = "warning"
		}
		finding.Code = "ai_" + strings.TrimPrefix(finding.Code, "ai_")
		key := finding.Level + "\x00" + finding.Code + "\x00" + finding.Message
		if seenFinding[key] {
			continue
		}
		seenFinding[key] = true
		findings = append(findings, finding)
		acceptedFindings++
	}
	if response.Recommendation == "revise" && acceptedFindings == 0 {
		findings = append(findings, model.RecipeValidationFinding{Level: "warning", Code: "ai_review_revise", Message: "The advisory reviewer recommends revision but did not return a usable finding; inspect every claim before approval."})
	}
	return strings.TrimSpace(response.Summary), findings
}
