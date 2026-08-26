package platform_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type recipeV2Fixture struct {
	memory      *store.Memory
	service     *platform.Service
	actor       platform.Actor
	integration model.Integration
	tool        model.Tool
	endpoint    string
	publication model.IntegrationRevision
}

func configureRecipeV2Fixture(t *testing.T, memory *store.Memory, service *platform.Service) recipeV2Fixture {
	t.Helper()
	actor := platform.Actor{ID: "root_recipe_v2", RequestID: "req_recipe_v2"}
	integration, err := service.CreateIntegration(t.Context(), platform.IntegrationInput{
		FamilyKey:   "payments-api",
		VersionKey:  "v2",
		DisplayName: "Payments API",
		Description: "Create and inspect payments.",
		Visibility:  model.VisibilityPrivate,
		Lifecycle:   "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	configurePrivateIntegrationFoundations(t, service, memory, integration, actor)
	configurePrivateIntegrationRuntimePolicyTool(t, service, integration, actor)
	bindings, err := service.IntegrationToolBindings(t.Context(), integration.ID)
	if err != nil || len(bindings) != 1 || bindings[0].Tool == nil {
		t.Fatalf("integration tool binding = %#v, err = %v", bindings, err)
	}
	tool, err := memory.Tool(t.Context(), integration.DeploymentID, bindings[0].ToolID)
	if err != nil || len(tool.RuntimeTargets) != 1 {
		t.Fatalf("resolved integration tool = %#v, err = %v", tool, err)
	}
	endpoint := strings.TrimRight(tool.RuntimeTargets[0].BaseURL, "/") + tool.HTTPPath
	publication, err := service.PublishIntegration(t.Context(), integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	return recipeV2Fixture{memory: memory, service: service, actor: actor, integration: integration, tool: tool, endpoint: endpoint, publication: publication}
}

func newRecipeV2Fixture(t *testing.T) recipeV2Fixture {
	t.Helper()
	memory := store.NewMemory()
	return configureRecipeV2Fixture(t, memory, platform.New(memory))
}

func analyseAndGenerateRecipeV2(t *testing.T, fixture recipeV2Fixture) (model.IntegrationAnalysis, model.Recipe) {
	t.Helper()
	analysis, err := fixture.service.AnalyseIntegrationFor(t.Context(), fixture.integration.DeploymentID, fixture.integration.ID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Plan.Recipes) != 1 {
		t.Fatalf("analysis recipes = %#v", analysis.Plan.Recipes)
	}
	recipes, err := fixture.service.GenerateRecipesForIntegration(t.Context(), fixture.integration.DeploymentID, analysis.ID, fixture.integration.ID, fixture.actor)
	if err != nil || len(recipes) != 1 || recipes[0].CurrentRevision == nil {
		t.Fatalf("generated recipes = %#v, err = %v", recipes, err)
	}
	return analysis, recipes[0]
}

func TestUnscopedIntegrationAnalysisRequiresSelectedProductAPI(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := platform.New(memory)
	analysis, err := service.AnalyseIntegration(t.Context(), "prod_acme", platform.Actor{ID: "root_unscoped"})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.State != "review" || analysis.SchemaVersion != 2 || len(analysis.Evidence) == 0 || len(analysis.Plan.Recipes) != 0 {
		t.Fatalf("unscoped analysis = %#v", analysis)
	}
	foundBlocker := false
	for _, unknown := range analysis.Unknowns {
		foundBlocker = foundBlocker || unknown.ID == "integration-scope" && unknown.Blocking
	}
	if !foundBlocker {
		t.Fatalf("unscoped analysis omitted selected-API blocker: %#v", analysis.Unknowns)
	}
	if _, err = service.GenerateRecipes(t.Context(), "prod_acme", analysis.ID, platform.Actor{ID: "root_unscoped"}); !errors.Is(err, platform.ErrRecipeNeedsInput) {
		t.Fatalf("unscoped generation error = %v", err)
	}
}

func TestScopedRecipeV2GenerationIsProductOnlyAndRevisionBound(t *testing.T) {
	t.Parallel()
	fixture := newRecipeV2Fixture(t)
	analysis, recipe := analyseAndGenerateRecipeV2(t, fixture)

	foundScope, foundTool := false, false
	for _, evidence := range analysis.Evidence {
		foundScope = foundScope || evidence.Kind == "integration_scope" && evidence.ResourceID == fixture.integration.ID
		foundTool = foundTool || evidence.Kind == "tool" && evidence.ResourceID == fixture.tool.ID && evidence.Version != "" && evidence.Visibility == fixture.integration.Visibility && strings.Contains(evidence.Excerpt, fixture.endpoint)
		if evidence.Kind == "mcp_oauth" || evidence.Kind == "automatic_tool" {
			t.Fatalf("platform delivery evidence entered product analysis: %#v", evidence)
		}
	}
	if !foundScope || !foundTool {
		t.Fatalf("scoped evidence omitted exact product ownership: %#v", analysis.Evidence)
	}
	seed := analysis.Plan.Recipes[0]
	if len(seed.CapabilityIDs) != 1 || seed.CapabilityIDs[0] != fixture.tool.ID || len(seed.EvidenceIDs) != 1 || seed.EvidenceIDs[0] != fixture.tool.ID || seed.SDKID != "" || len(seed.EndpointIDs) != 0 {
		t.Fatalf("recipe seed is not one exact product operation: %#v", seed)
	}
	if lower := strings.ToLower(seed.Title + " " + seed.Outcome); strings.Contains(lower, "dokosoko") || strings.Contains(lower, "mcp") {
		t.Fatalf("recipe seed describes its delivery channel: %#v", seed)
	}
	if recipe.ContractVersion != model.RecipeContractProductIntegrationV2 || recipe.IntegrationID != fixture.integration.ID || recipe.Audience != "coding_agent" || recipe.CurrentRevision.SpecVersion != model.RecipeSpecVersion2 || recipe.CurrentRevision.IntegrationRevisionID != fixture.publication.ID || recipe.CurrentRevision.IntegrationManifestHash != fixture.publication.ManifestHash {
		t.Fatalf("generated recipe contract = %#v", recipe)
	}
	var spec model.RecipeSpec
	if err := json.Unmarshal(recipe.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.CapabilityIDs) != 1 || spec.CapabilityIDs[0] != fixture.tool.ID || spec.IntegrationID != fixture.integration.ID || len(spec.Steps) < 2 || len(spec.Checks) < 1 {
		t.Fatalf("recipe spec = %#v", spec)
	}
	lowerMarkdown := strings.ToLower(recipe.CurrentRevision.Markdown)
	if !strings.Contains(recipe.CurrentRevision.Markdown, fixture.endpoint) || strings.Contains(lowerMarkdown, "dokosoko") || strings.Contains(lowerMarkdown, "mcp transport") || strings.Contains(lowerMarkdown, "mcp discovery") {
		t.Fatalf("recipe Markdown is not product-only:\n%s", recipe.CurrentRevision.Markdown)
	}
	selectedToolDependency := false
	for _, dependency := range recipe.Dependencies {
		selectedToolDependency = selectedToolDependency || dependency.Kind == "tool" && dependency.ResourceID == fixture.tool.ID
		if dependency.Kind == "identity_provider" || dependency.Kind == "authorization_point" || dependency.Kind == "resource_set" {
			t.Fatalf("unselected evidence widened recipe dependencies: %#v", recipe.Dependencies)
		}
	}
	if !selectedToolDependency {
		t.Fatalf("exact tool evidence was not persisted: %#v", recipe.Dependencies)
	}

	approved, err := fixture.service.ApproveRecipe(t.Context(), fixture.integration.DeploymentID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	published, err := fixture.service.PublishRecipe(t.Context(), fixture.integration.DeploymentID, approved.ID, approved.Revision, approved.CurrentRevisionID, fixture.actor)
	if err != nil || published.State != "published" {
		t.Fatalf("published recipe = %#v, err = %v", published, err)
	}

	current, err := fixture.memory.Integration(t.Context(), fixture.integration.DeploymentID, fixture.integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err = fixture.service.UpdateIntegration(t.Context(), current.ID, platform.IntegrationInput{
		FamilyKey: current.FamilyKey, VersionKey: current.VersionKey, DisplayName: current.DisplayName,
		Description: "Create, inspect, and reconcile payments.", Visibility: current.Visibility,
		Lifecycle: current.Lifecycle, Revision: current.Revision,
	}, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	newPublication, err := fixture.service.PublishIntegration(t.Context(), current.ID, fixture.actor)
	if err != nil || newPublication.ID == fixture.publication.ID {
		t.Fatalf("new integration publication = %#v, err = %v", newPublication, err)
	}
	reconciled, err := fixture.service.ReconcileRecipeDrift(t.Context(), fixture.integration.DeploymentID)
	if err != nil || len(reconciled) != 1 || reconciled[0].State != "outdated" {
		t.Fatalf("integration revision drift was not detected: %#v, err = %v", reconciled, err)
	}

	regroundAnalysis, regrounded := analyseAndGenerateRecipeV2(t, fixture)
	if regrounded.ID != recipe.ID || regrounded.AnalysisID != regroundAnalysis.ID || regrounded.State != "review" || regrounded.CurrentRevisionID == recipe.CurrentRevisionID || regrounded.CurrentRevision.IntegrationRevisionID != newPublication.ID {
		t.Fatalf("stable recipe was not regrounded to the exact publication: %#v", regrounded)
	}
	repeated, err := fixture.service.GenerateRecipesForIntegration(t.Context(), fixture.integration.DeploymentID, regroundAnalysis.ID, fixture.integration.ID, fixture.actor)
	if err != nil || len(repeated) != 1 || repeated[0].ID != regrounded.ID || repeated[0].CurrentRevisionID != regrounded.CurrentRevisionID || repeated[0].Revision != regrounded.Revision {
		t.Fatalf("same-analysis generation was not idempotent: %#v, err = %v", repeated, err)
	}
}

type adversarialRecipeV2Doer struct {
	bodies []string
}

func (d *adversarialRecipeV2Doer) Do(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	d.bodies = append(d.bodies, string(body))
	content := `{"status":"ok"}`
	switch {
	case strings.Contains(string(body), "Integration analysis contract:"):
		content = `{"summary":"Connect the product through DokoSoko MCP.","summary_evidence_ids":["platform-mcp"],"recipes":[]}`
	case strings.Contains(string(body), "Recipe authoring contract:"):
		content = `{"status":"ready","prerequisites":[],"steps":[{"action":"Connect the project to DokoSoko MCP.","expected_result":"MCP discovery succeeds.","evidence_ids":["platform-mcp"]},{"action":"Publish the connector.","expected_result":"The connector is public.","evidence_ids":["platform-mcp"]}],"checks":[{"action":"Inspect MCP discovery.","expected_result":"The tool appears.","evidence_ids":["platform-mcp"]}],"reference_ids":[],"gaps":[]}`
	case strings.Contains(string(body), "Recipe review contract:"):
		content = `{"summary":"Review the deterministic product implementation.","recommendation":"pass","findings":[]}`
	}
	payload, _ := json.Marshal(map[string]any{
		"id": "resp_recipe_v2", "model": "fixture",
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
	})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
}

func TestRecipeV2RejectsPlatformAIOutputAndUsesProductOnlyFallback(t *testing.T) {
	doer := &adversarialRecipeV2Doer{}
	memory := store.NewMemory()
	service := platform.NewWithVaultAndProductBuilderDoer(memory, nil, doer)
	if err := service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{
		Provider: "openai-compatible", APIKey: "fixture-secret", Endpoint: "https://llm.example.com",
		Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	connections, err := memory.AIProviderConnections(t.Context(), "prod_acme")
	if err != nil || len(connections) != 1 {
		t.Fatalf("AI connections = %#v, err = %v", connections, err)
	}
	actor := platform.Actor{ID: "root_recipe_ai", RequestID: "req_recipe_ai"}
	if _, err = service.TestAIProviderConnection(t.Context(), "prod_acme", connections[0].ID, actor); err != nil {
		t.Fatal(err)
	}
	fixture := configureRecipeV2Fixture(t, memory, service)
	analysis, recipe := analyseAndGenerateRecipeV2(t, fixture)
	if analysis.GeneratedBy != "deterministic" || analysis.ErrorCode != "invalid_structured_output" {
		t.Fatalf("adversarial analysis replaced deterministic product planning: %#v", analysis)
	}
	if recipe.CurrentRevision.GeneratedBy != "deterministic" {
		t.Fatalf("adversarial authoring did not fall back deterministically: %#v", recipe.CurrentRevision)
	}
	lower := strings.ToLower(recipe.CurrentRevision.Markdown)
	if strings.Contains(lower, "dokosoko") || strings.Contains(lower, "mcp discovery") || strings.Contains(lower, "platform-mcp") {
		t.Fatalf("platform delivery output survived validation:\n%s", recipe.CurrentRevision.Markdown)
	}
	foundAnalysisPrompt := false
	for _, body := range doer.bodies {
		if !strings.Contains(body, "Integration analysis contract:") {
			continue
		}
		foundAnalysisPrompt = true
		for _, forbidden := range []string{"platform_contract", "allowed_endpoint_ids", "identity_provider", "authorization_point", "mcp_oauth", "automatic_tool"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("analysis AI received platform-only field %q: %s", forbidden, body)
			}
		}
		if !strings.Contains(body, "allowed_capability_ids") || !strings.Contains(body, fixture.tool.ID) {
			t.Errorf("analysis AI omitted exact product capability evidence: %s", body)
		}
	}
	if !foundAnalysisPrompt {
		t.Fatalf("analysis AI was not invoked: %#v", doer.bodies)
	}
}

func TestPublicRecipeV2RejectsPrivateSelectedEvidence(t *testing.T) {
	t.Parallel()
	fixture := newRecipeV2Fixture(t)
	_, recipe := analyseAndGenerateRecipeV2(t, fixture)
	var spec model.RecipeSpec
	if err := json.Unmarshal(recipe.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	publicRecipe, err := fixture.service.UpdateRecipeSpec(t.Context(), fixture.integration.DeploymentID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID, spec, model.VisibilityPublic, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	publicRecipe, err = fixture.service.ApproveRecipe(t.Context(), fixture.integration.DeploymentID, publicRecipe.ID, publicRecipe.Revision, publicRecipe.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.PublishRecipe(t.Context(), fixture.integration.DeploymentID, publicRecipe.ID, publicRecipe.Revision, publicRecipe.CurrentRevisionID, fixture.actor); err == nil || !strings.Contains(err.Error(), "public evidence") {
		t.Fatalf("public recipe with private selected evidence was accepted: %v", err)
	}
}

func TestRetiredIntegrationCannotProduceOrKeepCurrentRecipes(t *testing.T) {
	fixture := newRecipeV2Fixture(t)
	_, recipe := analyseAndGenerateRecipeV2(t, fixture)
	integration, err := fixture.memory.Integration(t.Context(), fixture.integration.DeploymentID, fixture.integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.UpdateIntegration(t.Context(), integration.ID, platform.IntegrationInput{
		FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName,
		Description: integration.Description, Visibility: integration.Visibility, Lifecycle: "retired", Revision: integration.Revision,
	}, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AnalyseIntegrationFor(t.Context(), fixture.integration.DeploymentID, fixture.integration.ID, fixture.actor); !errors.Is(err, platform.ErrRecipeNeedsInput) {
		t.Fatalf("retired Integration analysis error = %v", err)
	}
	reconciled, err := fixture.service.ReconcileRecipeDrift(t.Context(), fixture.integration.DeploymentID)
	if err != nil || len(reconciled) != 1 || reconciled[0].ID != recipe.ID || reconciled[0].State != "outdated" || !reconciled[0].NeedsAttention {
		t.Fatalf("retired Integration recipe reconciliation = %#v, err=%v", reconciled, err)
	}
}
