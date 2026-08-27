package platform_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/identity"
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

func TestScopedRecipeGenerationIsDeploymentOwnedAndRevisionBound(t *testing.T) {
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
	if recipe.ContractVersion != model.RecipeContractDeploymentV3 || recipe.IntegrationID != "" || recipe.Audience != "coding_agent" || recipe.CurrentRevision.SpecVersion != model.RecipeSpecVersion3 || len(recipe.APIAttachments) != 1 || recipe.APIAttachments[0].IntegrationID != fixture.integration.ID || len(recipe.CurrentRevision.APIBindings) != 1 {
		t.Fatalf("generated recipe contract = %#v", recipe)
	}
	binding := recipe.CurrentRevision.APIBindings[0]
	if binding.IntegrationID != fixture.integration.ID || binding.IntegrationRevisionID != fixture.publication.ID || binding.IntegrationManifestHash != fixture.publication.ManifestHash || recipe.CurrentRevision.IntegrationRevisionID != "" || recipe.CurrentRevision.IntegrationManifestHash != "" {
		t.Fatalf("generated recipe API binding = %#v", recipe.CurrentRevision)
	}
	var spec model.RecipeSpec
	if err := json.Unmarshal(recipe.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.SchemaVersion != model.RecipeSpecVersion3 || spec.IntegrationID != "" || len(spec.APIAttachments) != 1 || spec.APIAttachments[0].IntegrationID != fixture.integration.ID || len(spec.CapabilityIDs) != 1 || spec.CapabilityIDs[0] != fixture.tool.ID || len(spec.Steps) < 2 || len(spec.Checks) < 1 {
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
	if regrounded.ID != recipe.ID || regrounded.AnalysisID != regroundAnalysis.ID || regrounded.State != "review" || regrounded.CurrentRevisionID == recipe.CurrentRevisionID || len(regrounded.CurrentRevision.APIBindings) != 1 || regrounded.CurrentRevision.APIBindings[0].IntegrationRevisionID != newPublication.ID || regrounded.CurrentRevision.APIBindings[0].IntegrationManifestHash != newPublication.ManifestHash {
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
		content = `{"recommendation":"pass","findings":[]}`
	}
	payload, _ := json.Marshal(map[string]any{
		"id": "resp_recipe_v2", "model": "fixture",
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
	})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
}

type multiAPIRecipeDoer struct {
	capabilityIDs []string
	bodies        []string
}

func (d *multiAPIRecipeDoer) Do(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	d.bodies = append(d.bodies, string(body))
	content := `{"status":"ready","reference_ids":[],"gaps":[]}`
	switch {
	case strings.Contains(string(body), "Recipe brief contract:"):
		encoded, _ := json.Marshal(map[string]any{"status": "ready", "capability_ids": d.capabilityIDs, "evidence_ids": d.capabilityIDs, "gaps": []string{}})
		content = string(encoded)
	case strings.Contains(string(body), "Recipe review contract:"):
		content = `{"recommendation":"pass","findings":[]}`
	}
	payload, _ := json.Marshal(map[string]any{
		"id": "resp_multi_api_recipe", "model": "fixture",
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10},
	})
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
}

func configureMultiAPIRecipeIntegration(t *testing.T, memory *store.Memory, service *platform.Service, familyKey, namespace, displayName string, actor platform.Actor) (model.Integration, model.Tool, model.IntegrationRevision) {
	t.Helper()
	ctx := t.Context()
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{
		FamilyKey: familyKey, VersionKey: "v1", DisplayName: displayName,
		Description: "Perform the reviewed " + displayName + " operation.", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatalf("create %s integration: %v", namespace, err)
	}
	publication, err := memory.SourcePublication(ctx, integration.DeploymentID, "pub_docs_seed")
	if err != nil {
		t.Fatalf("load %s documentation publication: %v", namespace, err)
	}
	documentationManifest, err := json.Marshal([]map[string]any{{"source_publication_id": publication.ID, "source_id": publication.SourceID, "revision": publication.Revision, "content_hash": publication.ContentHash, "name": "Reviewed documentation"}})
	if err != nil {
		t.Fatalf("encode %s documentation manifest: %v", namespace, err)
	}
	for _, resource := range []struct {
		kind     string
		name     string
		manifest json.RawMessage
	}{
		{kind: "documentation", name: displayName + " documentation", manifest: documentationManifest},
		{kind: "api", name: displayName + " contract", manifest: json.RawMessage(`[{"name":"perform","path":"/perform"}]`)},
	} {
		set, createErr := service.CreateResourceSet(ctx, platform.ResourceSetInput{Kind: resource.kind, Name: resource.name, Description: resource.name, State: "active", Manifest: resource.manifest}, actor)
		if createErr != nil {
			t.Fatalf("create %s %s resource: %v", namespace, resource.kind, createErr)
		}
		if _, attachErr := service.AttachResourceSet(ctx, integration.ID, set.ID, set.Latest.ID, actor); attachErr != nil {
			t.Fatalf("attach %s %s resource: %v", namespace, resource.kind, attachErr)
		}
	}
	provider, providerErr := memory.IdentityProvider(ctx, integration.DeploymentID)
	if errors.Is(providerErr, store.ErrNotFound) {
		provider = identity.ProviderConfig{ID: "idp_multi_api_recipe", OrganisationID: integration.OrganisationID, DeploymentID: integration.DeploymentID, Issuer: "https://identity.example.test", ClientID: "multi-api-client", Scopes: []string{"openid"}, Audience: "https://api.example.test", OAuthResource: "https://api.example.test", OrganisationClaim: "tenant_id", DelegatedAPIOrigin: "https://api.example.test", State: "active"}
	} else if providerErr != nil {
		t.Fatalf("load %s identity provider: %v", namespace, providerErr)
	}
	provider.Scopes = append(provider.Scopes, namespace+".read")
	if _, err = memory.SaveIdentityProvider(ctx, provider); err != nil {
		t.Fatalf("save %s identity provider: %v", namespace, err)
	}
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: namespace + ".read", DisplayName: "Use " + displayName, Description: "Perform one reviewed product operation.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatalf("save %s grant: %v", namespace, err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: namespace + ".perform", Name: "Use " + displayName, Description: "Perform one reviewed product operation.", ActionType: "read", RequiredGrants: []string{grant.Key}, DecisionTTLSeconds: 60, State: "active"}, actor)
	if err != nil {
		t.Fatalf("save %s authorization point: %v", namespace, err)
	}
	setup, err := service.ConfigureRuntimeSetup(ctx, integration.ID, platform.RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://" + namespace + ".api.example.test", AuthenticationType: "none"}, actor)
	if err != nil || len(setup.Connections) != 1 {
		t.Fatalf("runtime setup = %#v, err = %v", setup, err)
	}
	tool, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID: integration.DeploymentID, Scope: model.ToolScopeAPI, OwnerIntegrationID: integration.ID,
		RuntimeServiceConnectionID: setup.Connections[0].ID, HTTPPath: "/perform", Namespace: namespace, Name: "perform",
		Description:  "Perform the reviewed " + displayName + " operation.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"resource_id":{"type":"string"}},"required":["resource_id"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"id":{"type":"string"}},"required":["id"]}`),
		HTTPMethod:   http.MethodGet, AuthorizationPolicy: json.RawMessage(`{"required_grants":["` + grant.Key + `"],"confirmation_required":false,"risk":"low"}`), TimeoutMS: 1000,
	}, actor)
	if err != nil {
		t.Fatalf("create %s tool: %v", namespace, err)
	}
	tool, err = service.PublishTool(ctx, integration.DeploymentID, tool.ID, tool.Revision, actor)
	if err != nil {
		t.Fatalf("publish %s tool: %v", namespace, err)
	}
	if _, err = service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: tool.ID, Revision: tool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatalf("bind %s tool: %v", namespace, err)
	}
	apiPublication, err := service.PublishIntegration(ctx, integration.ID, actor)
	if err != nil {
		t.Fatalf("publish %s integration: %v", namespace, err)
	}
	return integration, tool, apiPublication
}

func TestAIRecipeGeneratorDetectsAndBindsMultipleAPIs(t *testing.T) {
	memory := store.NewMemory()
	setupService := platform.New(memory)
	actor := platform.Actor{ID: "root_multi_api_recipe", RequestID: "req_multi_api_recipe"}
	customers, customerTool, customerPublication := configureMultiAPIRecipeIntegration(t, memory, setupService, "customers-api", "customers", "Customers API", actor)
	billing, billingTool, billingPublication := configureMultiAPIRecipeIntegration(t, memory, setupService, "billing-api", "billing", "Billing API", actor)

	doer := &multiAPIRecipeDoer{capabilityIDs: []string{customerTool.ID, billingTool.ID}}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, nil, doer)
	if err := service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{
		Provider: "openai-compatible", APIKey: "fixture-secret", Endpoint: "https://llm.example.com",
		Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	recipe, err := service.CreateRecipeFromPrompt(t.Context(), "prod_acme", "Create a customer and provision their billing account.", actor)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ContractVersion != model.RecipeContractDeploymentV3 || recipe.IntegrationID != "" || recipe.CurrentRevision == nil || recipe.CurrentRevision.SpecVersion != model.RecipeSpecVersion3 {
		t.Fatalf("multi-API recipe contract = %#v", recipe)
	}
	wantAPIs := map[string]model.IntegrationRevision{customers.ID: customerPublication, billing.ID: billingPublication}
	if len(recipe.APIAttachments) != len(wantAPIs) || len(recipe.CurrentRevision.APIBindings) != len(wantAPIs) {
		t.Fatalf("multi-API attachment projection = %#v", recipe)
	}
	for _, attachment := range recipe.APIAttachments {
		if _, ok := wantAPIs[attachment.IntegrationID]; !ok {
			t.Fatalf("unexpected API attachment: %#v", attachment)
		}
	}
	for _, binding := range recipe.CurrentRevision.APIBindings {
		publication, ok := wantAPIs[binding.IntegrationID]
		if !ok || binding.IntegrationRevisionID != publication.ID || binding.IntegrationManifestHash != publication.ManifestHash {
			t.Fatalf("incorrect immutable API binding: %#v", binding)
		}
	}
	var spec model.RecipeSpec
	if err := json.Unmarshal(recipe.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.SchemaVersion != model.RecipeSpecVersion3 || len(spec.APIAttachments) != 2 || len(spec.CapabilityIDs) != 2 || len(spec.Steps) < 4 || len(spec.Checks) != 2 {
		t.Fatalf("multi-API recipe spec = %#v", spec)
	}
	analysis, err := memory.IntegrationAnalysis(t.Context(), recipe.ProductID, recipe.AnalysisID)
	if err != nil {
		t.Fatal(err)
	}
	scopes := make(map[string]bool)
	for _, evidence := range analysis.Evidence {
		if evidence.Kind == "integration_scope" {
			scopes[evidence.ResourceID] = true
		}
	}
	if len(scopes) != 2 || !scopes[customers.ID] || !scopes[billing.ID] {
		t.Fatalf("persisted auto-detection scope = %#v", scopes)
	}
	foundDetectionPrompt := false
	for _, body := range doer.bodies {
		if strings.Contains(body, "Recipe brief contract:") {
			foundDetectionPrompt = strings.Contains(body, `available_apis`) && strings.Contains(body, customers.ID) && strings.Contains(body, billing.ID) && strings.Contains(body, customerTool.ID) && strings.Contains(body, billingTool.ID)
		}
	}
	if !foundDetectionPrompt {
		t.Fatalf("recipe brief did not receive all eligible APIs and exact capabilities: %#v", doer.bodies)
	}
	reconciled, err := service.ReconcileRecipeDrift(t.Context(), recipe.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconciled) != 1 || reconciled[0].ID != recipe.ID || reconciled[0].State != "review" || reconciled[0].NeedsAttention != recipe.NeedsAttention || len(reconciled[0].APIAttachments) != 2 {
		t.Fatalf("current multi-API recipe was incorrectly reconciled: %#v", reconciled)
	}
	reworked, err := service.ReworkRecipe(t.Context(), recipe.ProductID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID, "Clarify the verification steps for both APIs.", actor)
	if err != nil {
		t.Fatal(err)
	}
	if reworked.CurrentRevision == nil || reworked.CurrentRevisionID == recipe.CurrentRevisionID || reworked.ContractVersion != model.RecipeContractDeploymentV3 || len(reworked.APIAttachments) != 2 || len(reworked.CurrentRevision.APIBindings) != 2 {
		t.Fatalf("reworked multi-API recipe lost its immutable API scope: %#v", reworked)
	}
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
	foundReviewPrompt := false
	for _, body := range doer.bodies {
		if strings.Contains(body, "Recipe review contract:") {
			foundReviewPrompt = true
			if !strings.Contains(body, "allowed_evidence_ids") || !strings.Contains(body, fixture.tool.ID) {
				t.Errorf("recipe review omitted its exact allowed evidence identifiers: %s", body)
			}
		}
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
	if !foundReviewPrompt {
		t.Fatalf("recipe review AI was not invoked: %#v", doer.bodies)
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
	publicRecipe, err := fixture.service.UpdateRecipeReferences(t.Context(), fixture.integration.DeploymentID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID, spec.ReferenceIDs, model.VisibilityPublic, fixture.actor)
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
