package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestIntegrationEvidenceExcerptsStayWithinProviderBoundary(t *testing.T) {
	t.Parallel()
	records := make([]model.KnowledgeRecord, 0, 30)
	for source := 0; source < 5; source++ {
		for document := 0; document < 6; document++ {
			records = append(records, model.KnowledgeRecord{
				ID:        fmt.Sprintf("doc-%d-%d", source, document),
				SourceID:  fmt.Sprintf("source-%d", source),
				Title:     "Sample code",
				Text:      strings.Repeat("é", 5_000),
				URL:       fmt.Sprintf("https://docs.example.com/%d/%d", source, document),
				Published: true,
			})
		}
	}
	excerpts := integrationSourceExcerpts(records)
	total := 0
	for sourceID, excerpt := range excerpts {
		length := len([]rune(excerpt.Text))
		if length > maxAnalysisSourceExcerptRunes {
			t.Fatalf("%s excerpt has %d runes, max %d", sourceID, length, maxAnalysisSourceExcerptRunes)
		}
		if len(excerpt.References) > maxAnalysisDocumentsPerSource {
			t.Fatalf("%s has %d references, max %d", sourceID, len(excerpt.References), maxAnalysisDocumentsPerSource)
		}
		total += length
	}
	if total > maxAnalysisKnowledgeRunes {
		t.Fatalf("knowledge evidence has %d runes, max %d", total, maxAnalysisKnowledgeRunes)
	}

	manifest := json.RawMessage(`{"paths":{"/calls":` + `"` + strings.Repeat("x", 10_000) + `"}}`)
	integration := model.Integration{FamilyKey: "voice", VersionKey: "v1", Lifecycle: "active", Description: strings.Repeat("d", 10_000), Resources: []model.IntegrationResourceLink{{Name: "Voice API", Kind: "openapi", ResolvedRevision: &model.ResourceSetRevision{Manifest: manifest}}}}
	if length := len([]rune(integrationCatalogExcerpt(integration, maxAnalysisIntegrationItem))); length > maxAnalysisIntegrationItem {
		t.Fatalf("integration evidence has %d runes, max %d", length, maxAnalysisIntegrationItem)
	}
	tool := model.Tool{Description: strings.Repeat("d", 5_000), InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)}
	if length := len([]rune(toolCatalogExcerpt(tool, maxAnalysisToolItem))); length > maxAnalysisToolItem {
		t.Fatalf("tool evidence has %d runes, max %d", length, maxAnalysisToolItem)
	}
}

func TestRecipeGroundingMatchIncludesAnalysisDependenciesAndSeed(t *testing.T) {
	t.Parallel()
	seed := model.RecipeSeed{Slug: "connect-docs", Title: "Connect docs", Outcome: "Use the reviewed docs.", Audience: "developer", EndpointIDs: []string{"mcp"}}
	analysis := model.IntegrationAnalysis{ID: "analysis-current", Evidence: []model.IntegrationEvidence{{Kind: "resource_set", ResourceID: "docs", Fingerprint: "docs-r2"}}}
	revision := model.RecipeRevision{ID: "recipe-revision-2"}
	recipe := model.Recipe{
		AnalysisID:        analysis.ID,
		Title:             seed.Title,
		Outcome:           seed.Outcome,
		Audience:          seed.Audience,
		Dependencies:      recipeGroundingDependencies(analysis, seed),
		CurrentRevisionID: revision.ID,
		CurrentRevision:   &revision,
	}
	if !recipeGroundingMatches(recipe, analysis, seed) {
		t.Fatal("identical grounding did not match")
	}
	changedSeed := seed
	changedSeed.Outcome = "Use a different reviewed outcome."
	if recipeGroundingMatches(recipe, analysis, changedSeed) {
		t.Fatal("changed recipe seed was treated as idempotent")
	}
	changedEndpoints := seed
	changedEndpoints.EndpointIDs = []string{"mcp", "public-mcp"}
	if recipeGroundingMatches(recipe, analysis, changedEndpoints) {
		t.Fatal("changed recipe endpoint IDs were treated as idempotent")
	}
	changedDependencies := recipe
	changedDependencies.Dependencies = append([]model.RecipeDependency(nil), recipe.Dependencies...)
	changedDependencies.Dependencies[0].Version = "docs-r1"
	if recipeGroundingMatches(changedDependencies, analysis, seed) {
		t.Fatal("changed exact dependencies were treated as idempotent")
	}
	changedAnalysis := analysis
	changedAnalysis.ID = "analysis-other"
	if recipeGroundingMatches(recipe, changedAnalysis, seed) {
		t.Fatal("a different analysis binding was treated as idempotent")
	}
}

func TestRecipeAuthoringContractVersionChangesGrounding(t *testing.T) {
	t.Parallel()
	seed := model.RecipeSeed{Slug: "check-readiness", Title: "Check readiness", Outcome: "Verify readiness.", Audience: "developer", EndpointIDs: []string{"mcp"}}
	analysis := model.IntegrationAnalysis{
		ID:       "analysis-current",
		Plan:     model.IntegrationPlan{Summary: "Readiness integration."},
		Evidence: []model.IntegrationEvidence{{Kind: "tool", ResourceID: "tool-readiness", Fingerprint: "tool-r3"}},
	}
	revision := model.RecipeRevision{ID: "recipe-revision-8"}
	recipe := model.Recipe{
		AnalysisID:        analysis.ID,
		Title:             seed.Title,
		Outcome:           seed.Outcome,
		Audience:          seed.Audience,
		Dependencies:      recipeGroundingDependenciesForContract(analysis, seed, "recipe-authoring-v5"),
		CurrentRevisionID: revision.ID,
		CurrentRevision:   &revision,
	}
	if recipeGroundingMatches(recipe, analysis, seed) {
		t.Fatal("a recipe authored under the old deterministic contract was treated as current")
	}
	recipe.Dependencies = recipeGroundingDependencies(analysis, seed)
	if !recipeGroundingMatches(recipe, analysis, seed) {
		t.Fatal("the current authoring contract did not match its own grounding fingerprint")
	}
}

func TestDeterministicRecipeMarkdownUsesExactPrivateIntegrationEvidence(t *testing.T) {
	t.Parallel()
	seed := model.RecipeSeed{
		Slug:        "check-platform-readiness",
		Title:       "Check platform readiness",
		Outcome:     "A developer can invoke the exact readiness capability and validate its response.",
		Audience:    "developer",
		EndpointIDs: []string{"mcp"},
	}
	analysis := model.IntegrationAnalysis{
		ID: "analysis-readiness",
		Plan: model.IntegrationPlan{
			Identity: model.IntegrationIdentityPlan{Mode: "oauth2", Issuer: "https://identity.example.test", Audience: "https://mcp.example.test", Explanation: "DokoSoko brokers customer sign-in through the configured OIDC provider and keeps vendor access tokens out of MCP clients."},
			Endpoints: []model.IntegrationEndpointPlan{
				{Name: "mcp", Method: "POST", Path: "/mcp", Purpose: "Private MCP discovery and tool execution.", Identity: "oauth2"},
				{Name: "public-mcp", Method: "POST", Path: "/mcp/public", Purpose: "Anonymous published content.", Identity: "none"},
			},
		},
		Evidence: []model.IntegrationEvidence{
			{Kind: "identity_provider", ResourceID: "identity-provider", Label: "Customer identity boundary", Version: "7", Excerpt: "Issuer: https://identity.example.test\nAudience: https://mcp.example.test\nOAuth resource: https://mcp.example.test\nScopes: openid, platform.readiness\nOrganisation claim: tenant_uid\nInstallation claim: installation_uid\nState: active"},
			{Kind: "resource_set", ResourceID: "docs-set-current", Label: "Integration documentation", Version: "2", Excerpt: "Kind: documentation\nBinding: pinned exact revision\nRevision: 2\nRevision ID: docs-revision-r2"},
			{Kind: "resource_set", ResourceID: "api-set-current", Label: "Readiness contract", Version: "5", Excerpt: "Kind: api\nBinding: pinned exact revision\nRevision: 5\nRevision ID: api-revision-r5"},
			{Kind: "source_publication", ResourceID: "293b-source-publication", Label: "Platform documentation", Version: "1", Location: "https://docs.example.test/readiness"},
			{Kind: "authorization_point", ResourceID: "authorization-readiness", Label: "platform.readiness.check", Version: "4", Excerpt: "Description: Read readiness.\nAction: read\nRequired grants: platform.readiness\nConfirmation required: false\nState: active"},
			{Kind: "tool", ResourceID: "tool-readiness", Label: "platform.check_readiness", Version: "3", Excerpt: "Exact bound tool revision: 3\nDescription: Check readiness.\nBackend: http\nMethod: GET\nFixed endpoint: https://api.example.test/ready\nInput schema: {\"type\":\"object\",\"additionalProperties\":false}\nOutput schema: {\"type\":\"object\",\"additionalProperties\":false,\"properties\":{\"ready\":{\"type\":\"boolean\"}},\"required\":[\"ready\"]}\nAuthorization policy: {\"required_grants\":[\"platform.readiness\"],\"confirmation_required\":false}"},
		},
	}
	references := recipeReferences(analysis.Evidence)
	markdown := deterministicRecipeMarkdown(model.Product{Name: "Platform"}, analysis, seed, references)

	for _, exact := range []string{
		"private endpoint `mcp`: `POST` `/mcp` with identity mode `oauth2`",
		"Resolve endpoint path `/mcp` against the DokoSoko deployment origin supplied by the operator",
		"DokoSoko broker configuration: issuer `https://identity.example.test`, audience `https://mcp.example.test`",
		"MCP client authenticates to the private endpoint through DokoSoko",
		"identity provider declares scopes `openid` `platform.readiness`",
		"Bound MCP tool `platform.check_readiness` declares required grant `platform.readiness` in its authorization policy",
		"configured identity provider identifies `tenant_uid` as its organisation claim",
		"Authorization point `platform.readiness.check` (ID `authorization-readiness`) governs action `read` and requires grant `platform.readiness`",
		"MCP client does not integrate directly with the configured issuer or handle the vendor access token",
		"grant `platform.readiness`",
		"MCP tool `platform.check_readiness`",
		"`GET` `https://api.example.test/ready`",
		`"ready"`,
		"the `documentation` resource set `Integration documentation` (ID `docs-set-current`), exact revision `2`",
		"the `api` resource set `Readiness contract` (ID `api-set-current`), exact revision `5`",
		"source publication `Platform documentation` (ID `293b-source-publication`), publication revision `1`",
	} {
		if !strings.Contains(markdown, exact) {
			t.Errorf("deterministic recipe omitted exact grounded detail %q:\n%s", exact, markdown)
		}
	}
	lower := strings.ToLower(markdown)
	for _, invented := range []string{"whether this flow is public", "public or", "when the client requests", "request id", "request_id", "no undocumented setup", "ready is true", "request succeeded", "obtain an oauth", "request an access token", "authenticate through issuer", "resolve authorization point", "oauth challenge", "consent", "has the required grant", "derives and enforces organization", "tenant overrides", "evaluates authorization point", "localhost", "chatgpt.com"} {
		if strings.Contains(lower, invented) {
			t.Errorf("deterministic recipe contains unsupported claim %q:\n%s", invented, markdown)
		}
	}
	if strings.Contains(markdown, "/mcp/public") {
		t.Errorf("an endpoint outside the seed leaked into the private recipe:\n%s", markdown)
	}
	for _, finding := range validateRecipeMarkdown(markdown, references, recipeGroundedURLs(analysis)...) {
		if finding.Level == "error" {
			t.Errorf("grounded deterministic recipe failed validation: %#v\n%s", finding, markdown)
		}
	}
}

func TestDeterministicRecipeMarkdownFailsClosedWithoutEndpointSelection(t *testing.T) {
	t.Parallel()
	analysis := model.IntegrationAnalysis{
		Plan: model.IntegrationPlan{
			Identity: model.IntegrationIdentityPlan{Mode: "oauth2", Issuer: "https://identity.example.test"},
			Endpoints: []model.IntegrationEndpointPlan{
				{Name: "mcp", Method: "POST", Path: "/mcp", Identity: "oauth2"},
				{Name: "public-mcp", Method: "POST", Path: "/mcp/public", Identity: "none"},
			},
		},
		Evidence: []model.IntegrationEvidence{{Kind: "tool", ResourceID: "tool-readiness", Label: "platform.check_readiness", Version: "3", Excerpt: "Method: GET\nFixed endpoint: https://api.example.test/ready"}},
	}
	seed := model.RecipeSeed{Slug: "unselected", Title: "Unselected endpoint", Outcome: "Choose a reviewed endpoint.", Audience: "developer"}
	markdown := deterministicRecipeMarkdown(model.Product{}, analysis, seed, nil)
	for _, leaked := range []string{"`POST` `/mcp`", "/mcp/public", "platform.check_readiness", "https://api.example.test/ready"} {
		if strings.Contains(markdown, leaked) {
			t.Errorf("empty endpoint selection widened into grounded operation %q:\n%s", leaked, markdown)
		}
	}
	if !strings.Contains(markdown, "No exact IntegrationPlan endpoint is selected") || !strings.Contains(markdown, "Keep this recipe in review") {
		t.Fatalf("empty endpoint selection did not fail closed:\n%s", markdown)
	}
	found := false
	for _, finding := range validateRecipeMarkdown(markdown, nil, recipeGroundedURLs(analysis)...) {
		found = found || finding.Level == "error" && finding.Code == "missing_endpoint_selection"
	}
	if !found {
		t.Fatalf("empty endpoint selection did not create a review-blocking finding:\n%s", markdown)
	}
}
