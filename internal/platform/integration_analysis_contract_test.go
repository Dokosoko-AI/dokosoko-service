package platform

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func productAnalysisScope(integrationID string) model.IntegrationEvidence {
	return model.IntegrationEvidence{Kind: integrationScopeEvidenceKind, ResourceID: integrationID, Label: "Selected API", Version: "3"}
}

func productAnalysisIntegration(integrationID string) model.IntegrationEvidence {
	return model.IntegrationEvidence{Kind: "integration", ResourceID: integrationID, Label: "Payments API", Version: "3", Fingerprint: "integration-r3"}
}

func productAnalysisTool(integrationID, toolID string) model.IntegrationEvidence {
	return model.IntegrationEvidence{
		Kind:       "tool",
		ResourceID: toolID,
		Label:      "payments.create_charge",
		Version:    "7",
		Excerpt: "Exact bound tool revision: 7\n" +
			"Description: Create one charge.\n" +
			"Scope: api\n" +
			"Owner integration ID: " + integrationID + "\n" +
			"Backend: http\n" +
			"Method: POST\n" +
			"Fixed endpoint: https://payments.example.test/v2/charges\n" +
			`Input schema: {"type":"object","required":["amount"]}` + "\n" +
			`Output schema: {"type":"object","required":["id"]}`,
		Fingerprint: "tool-r7",
	}
}

func productAnalysisAPI(resourceID string) model.IntegrationEvidence {
	return model.IntegrationEvidence{
		Kind:        "resource_set",
		ResourceID:  resourceID,
		Label:       "Payments contract",
		Version:     "5",
		Excerpt:     `Kind: api` + "\nBinding: pinned exact revision\nRevision: 5\nRevision ID: api-r5\nContent hash: sha256:api-r5\n" + `Manifest: [{"method":"POST","path":"/v2/charges"}]`,
		Fingerprint: "api-r5",
	}
}

func productAnalysisSDK(id string) model.IntegrationEvidence {
	return model.IntegrationEvidence{
		Kind:        "sdk",
		ResourceID:  id,
		Label:       "@acme/payments",
		Version:     "2.4.1",
		Excerpt:     "Ecosystem: npm\nCoordinate: @acme/payments\nExact version: 2.4.1\nInstall: npm install @acme/payments@2.4.1",
		Fingerprint: "sdk-2.4.1",
	}
}

func TestDeterministicIntegrationRecipeSeedsSelectExactProductCapabilities(t *testing.T) {
	t.Parallel()
	integrationID := "integration-payments-v2"
	evidence := []model.IntegrationEvidence{
		productAnalysisScope(integrationID),
		productAnalysisIntegration(integrationID),
		productAnalysisTool(integrationID, "tool-create-charge"),
		productAnalysisAPI("api-payments-r5"),
		productAnalysisSDK("sdk-payments-node"),
		{Kind: "identity_provider", ResourceID: "identity", Label: "Platform identity"},
		{Kind: "mcp_oauth", ResourceID: "platform-mcp", Label: "Platform MCP delivery"},
		{Kind: "automatic_tool", ResourceID: "automatic-search", Label: "Generated knowledge search"},
	}
	seeds := deterministicIntegrationRecipeSeeds(
		model.Product{Slug: "acme"},
		model.Integration{ID: integrationID, FamilyKey: "payments", VersionKey: "v2"},
		evidence,
	)
	if len(seeds) != 1 {
		t.Fatalf("recipe seeds = %#v", seeds)
	}
	for _, seed := range seeds {
		if seed.Audience != "coding_agent" {
			t.Fatalf("recipe seed audience = %q", seed.Audience)
		}
		if len(seed.CapabilityIDs) != 1 || seed.CapabilityIDs[0] != "tool-create-charge" {
			t.Fatalf("seed did not select exactly one product capability: %#v", seed)
		}
		if seed.SDKID != "sdk-payments-node" || !reflect.DeepEqual(seed.EvidenceIDs, []string{seed.CapabilityIDs[0], "sdk-payments-node"}) {
			t.Fatalf("seed did not retain its exact capability/SDK evidence: %#v", seed)
		}
		if len(seed.EndpointIDs) != 0 {
			t.Fatalf("product recipe retained a DokoSoko delivery endpoint: %#v", seed)
		}
		if lower := strings.ToLower(seed.Title + " " + seed.Outcome); strings.Contains(lower, "dokosoko") || strings.Contains(lower, "mcp") {
			t.Fatalf("product recipe became a platform delivery guide: %#v", seed)
		}
	}
	apiOnlyEvidence := []model.IntegrationEvidence{productAnalysisScope(integrationID), productAnalysisIntegration(integrationID), productAnalysisAPI("api-payments-r5")}
	apiSeeds := deterministicIntegrationRecipeSeeds(model.Product{Slug: "acme"}, model.Integration{ID: integrationID, FamilyKey: "payments", VersionKey: "v2"}, apiOnlyEvidence)
	if len(apiSeeds) != 0 {
		t.Fatalf("whole API resource was treated as one callable operation: %#v", apiSeeds)
	}
}

func TestDeterministicIntegrationRecipeSeedsDoNotGuessSDKOrCapability(t *testing.T) {
	t.Parallel()
	integrationID := "integration-payments-v2"
	base := []model.IntegrationEvidence{
		productAnalysisScope(integrationID),
		productAnalysisIntegration(integrationID),
		productAnalysisTool(integrationID, "tool-create-charge"),
		productAnalysisSDK("sdk-payments-node"),
		productAnalysisSDK("sdk-payments-go"),
	}
	seeds := deterministicIntegrationRecipeSeeds(model.Product{Slug: "acme"}, model.Integration{ID: integrationID, FamilyKey: "payments", VersionKey: "v2"}, base)
	if len(seeds) != 1 || seeds[0].SDKID != "" || !reflect.DeepEqual(seeds[0].EvidenceIDs, []string{"tool-create-charge"}) {
		t.Fatalf("ambiguous SDK was guessed: %#v", seeds)
	}

	noCapability := []model.IntegrationEvidence{
		productAnalysisScope(integrationID),
		productAnalysisIntegration(integrationID),
		{Kind: "resource_set", ResourceID: "api-unresolved", Label: "Unresolved API", Version: "unresolved", Excerpt: "Kind: api\nBinding: follow latest"},
		{Kind: "tool", ResourceID: "common-tool", Label: "common.status", Version: "2", Excerpt: "Exact bound tool revision: 2\nScope: common\nBackend: http\nMethod: GET\nFixed endpoint: https://status.example.test\nInput schema: {}\nOutput schema: {}"},
	}
	service := New(store.NewMemory())
	plan, unknowns := service.deterministicIntegrationPlan(context.Background(), model.Product{ID: "prod_acme", Name: "Acme"}, noCapability, &model.Integration{ID: integrationID, DisplayName: "Payments API"})
	if len(plan.Recipes) != 0 {
		t.Fatalf("unresolved or non-API-owned evidence produced recipes: %#v", plan.Recipes)
	}
	foundBlocker := false
	for _, unknown := range unknowns {
		foundBlocker = foundBlocker || unknown.ID == "product-capability" && unknown.Blocking
	}
	if !foundBlocker {
		t.Fatalf("zero viable recipes did not produce a blocking unknown: %#v", unknowns)
	}
}

func TestIntegrationAnalysisResponseRequiresExactProductSelectionAndAllowsZero(t *testing.T) {
	t.Parallel()
	integrationID := "integration-payments-v2"
	evidence := []model.IntegrationEvidence{
		productAnalysisScope(integrationID),
		productAnalysisIntegration(integrationID),
		productAnalysisTool(integrationID, "tool-create-charge"),
		productAnalysisAPI("api-payments-r5"),
		productAnalysisSDK("sdk-payments-node"),
		{Kind: "identity_provider", ResourceID: "identity", Label: "Platform identity"},
	}
	fallback := model.IntegrationPlan{
		Summary:   "Server summary.",
		Identity:  model.IntegrationIdentityPlan{Mode: "oauth2", Issuer: "https://identity.example.test"},
		Endpoints: []model.IntegrationEndpointPlan{{Name: "mcp", Method: "POST", Path: "/mcp", Identity: "oauth2"}},
		Recipes:   deterministicIntegrationRecipeSeeds(model.Product{Slug: "acme"}, model.Integration{ID: integrationID, FamilyKey: "payments", VersionKey: "v2"}, evidence),
	}
	valid := integrationAnalysisAIResponse{
		Recipes: []integrationAnalysisAIRecipe{{
			CapabilityIDs: []string{"tool-create-charge"},
			SDKID:         "sdk-payments-node",
			EvidenceIDs:   []string{"tool-create-charge", "sdk-payments-node"},
		}},
	}
	plan, ok := integrationAnalysisResponsePlan(valid, fallback, evidence)
	if !ok || len(plan.Recipes) != 1 {
		t.Fatalf("valid product analysis response was rejected: plan=%#v ok=%t", plan, ok)
	}
	if !reflect.DeepEqual(plan.Identity, fallback.Identity) || !reflect.DeepEqual(plan.Endpoints, fallback.Endpoints) {
		t.Fatalf("AI changed server-owned setup fields: %#v", plan)
	}
	if got := plan.Recipes[0]; len(got.CapabilityIDs) != 1 || got.CapabilityIDs[0] != "tool-create-charge" || got.SDKID != "sdk-payments-node" || len(got.EndpointIDs) != 0 {
		t.Fatalf("validated selection was not retained exactly: %#v", got)
	}
	if plan.Recipes[0].Title != fallback.Recipes[0].Title || plan.Recipes[0].Outcome != fallback.Recipes[0].Outcome || plan.Recipes[0].Slug != fallback.Recipes[0].Slug || plan.Summary != fallback.Summary {
		t.Fatalf("AI replaced server-owned recipe facts: %#v", plan)
	}

	zero := integrationAnalysisAIResponse{Recipes: []integrationAnalysisAIRecipe{}}
	zeroPlan, ok := integrationAnalysisResponsePlan(zero, fallback, evidence)
	if !ok || len(zeroPlan.Recipes) != len(fallback.Recipes) {
		t.Fatalf("advisory zero-candidate analysis suppressed deterministic candidates: plan=%#v ok=%t", zeroPlan, ok)
	}
	zeroPlan = normalizeIntegrationPlan(zeroPlan, fallback, evidence)
	if len(zeroPlan.Recipes) != len(fallback.Recipes) {
		t.Fatalf("normalization lost deterministic candidates: %#v", zeroPlan.Recipes)
	}

	invalid := []integrationAnalysisAIResponse{
		{
			Recipes: []integrationAnalysisAIRecipe{{CapabilityIDs: []string{"tool-create-charge", "api-payments-r5"}, EvidenceIDs: []string{"tool-create-charge", "api-payments-r5"}}},
		},
		{
			Recipes: []integrationAnalysisAIRecipe{{CapabilityIDs: []string{"tool-create-charge"}, EvidenceIDs: []string{integrationID}}},
		},
		{
			Recipes: []integrationAnalysisAIRecipe{
				{CapabilityIDs: []string{"tool-create-charge"}, EvidenceIDs: []string{"tool-create-charge"}},
				{CapabilityIDs: []string{"tool-create-charge"}, EvidenceIDs: []string{"tool-create-charge"}},
			},
		},
	}
	for index, response := range invalid {
		if _, accepted := integrationAnalysisResponsePlan(response, fallback, evidence); accepted {
			t.Errorf("invalid product selection %d was accepted: %#v", index, response)
		}
	}
}

func TestIntegrationAnalysisProductEvidenceExcludesPlatformAndAmbiguity(t *testing.T) {
	t.Parallel()
	values := []model.IntegrationEvidence{
		productAnalysisIntegration("integration-payments-v2"),
		productAnalysisAPI("api-payments-r5"),
		{Kind: "identity_provider", ResourceID: "identity"},
		{Kind: "authorization_point", ResourceID: "authorization"},
		{Kind: "integration_tool_binding", ResourceID: "common-tool"},
		{Kind: "source_publication", ResourceID: "duplicate-doc"},
		{Kind: "source_publication", ResourceID: "duplicate-doc"},
	}
	got := evidenceIDs(unambiguousIntegrationProductEvidence(values))
	want := []string{"integration-payments-v2", "api-payments-r5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AI-visible product evidence IDs = %#v, want %#v", got, want)
	}
}
