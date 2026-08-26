package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func preparePublishedRecipeHTTPIntegration(t *testing.T, ctx context.Context, memory *store.Memory, service *platform.Service, integration model.Integration, namespace string, actor platform.Actor) model.Tool {
	t.Helper()
	grantKey := namespace + ".read"
	if _, err := memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "idp_" + namespace, OrganisationID: integration.OrganisationID, DeploymentID: integration.DeploymentID, Issuer: "https://identity.example.test", ClientID: namespace + "-client", Scopes: []string{"openid", grantKey}, Audience: "https://api.example.test", OAuthResource: "https://api.example.test", OrganisationClaim: "tenant_id", DelegatedAPIOrigin: "https://api.example.test", State: "active"}); err != nil {
		t.Fatal(err)
	}
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: grantKey, DisplayName: "Read " + namespace, Description: "Read one product status.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: namespace + ".status.read", Name: "Read product status", Description: "Read one product status.", ActionType: "read", RequiredGrants: []string{grant.Key}, DecisionTTLSeconds: 60, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := service.ConfigureRuntimeSetup(ctx, integration.ID, platform.RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://api.example.test", AuthenticationType: "none"}, actor)
	if err != nil || len(setup.Connections) != 1 {
		t.Fatalf("runtime setup=%#v err=%v", setup, err)
	}
	tool, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID:                  integration.DeploymentID,
		Scope:                      model.ToolScopeAPI,
		OwnerIntegrationID:         integration.ID,
		RuntimeServiceConnectionID: setup.Connections[0].ID,
		HTTPPath:                   "/" + namespace + "/status",
		Namespace:                  namespace,
		Name:                       "read_status",
		Description:                "Read one product status.",
		InputSchema:                json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"resource_id":{"type":"string"}},"required":["resource_id"]}`),
		OutputSchema:               json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string"}},"required":["status"]}`),
		HTTPMethod:                 http.MethodGet,
		AuthorizationPolicy:        json.RawMessage(`{"required_grants":["` + grantKey + `"],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:                  1000,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err = service.PublishTool(ctx, integration.DeploymentID, tool.ID, tool.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: tool.ID, Revision: tool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishIntegration(ctx, integration.ID, actor); err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestRecipeHTTPFlowCarriesSelectedIntegrationScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_recipe_http"}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})
	selected, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "payments-api", VersionKey: "v1", DisplayName: "Payments API", Description: "Payment operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	prepareHTTPPrivateIntegrationFoundations(t, handler, selected.ID)
	tool := preparePublishedRecipeHTTPIntegration(t, ctx, memory, service, selected, "payments", actor)
	other, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "messages-api", VersionKey: "v1", DisplayName: "Messages API", Description: "Message operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/analyses", "doko_admin_demo", `{"integration_id":"`+selected.ID+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("scoped analysis status=%d body=%s", w.Code, w.Body.String())
	}
	var analysis model.IntegrationAnalysis
	if err := json.Unmarshal(w.Body.Bytes(), &analysis); err != nil {
		t.Fatal(err)
	}
	if len(analysis.Plan.Recipes) != 1 || len(analysis.Plan.Recipes[0].CapabilityIDs) != 1 || analysis.Plan.Recipes[0].CapabilityIDs[0] != tool.ID || strings.Contains(strings.ToLower(analysis.Plan.Recipes[0].Slug), "mcp") {
		t.Fatalf("scoped plan = %#v", analysis.Plan)
	}
	foundScope := false
	for _, evidence := range analysis.Evidence {
		foundScope = foundScope || evidence.Kind == "integration_scope" && evidence.ResourceID == selected.ID
	}
	if !foundScope {
		t.Fatalf("analysis omitted selected scope: %#v", analysis.Evidence)
	}

	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/analyses/"+analysis.ID+"/recipes", "doko_admin_demo", `{"integration_id":"`+selected.ID+`"}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"contract_version":"product-integration-v2"`) || !strings.Contains(w.Body.String(), `"integration_id":"`+selected.ID+`"`) || strings.Contains(strings.ToLower(w.Body.String()), "connect-acme") {
		t.Fatalf("scoped generation status=%d body=%s", w.Code, w.Body.String())
	}
	var generated struct {
		Items []model.Recipe `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &generated); err != nil || len(generated.Items) != 1 {
		t.Fatalf("decode generated recipe: values=%#v err=%v", generated.Items, err)
	}
	recipe := generated.Items[0]
	if recipe.IntegrationID != selected.ID || recipe.ContractVersion != model.RecipeContractProductIntegrationV2 || recipe.CurrentRevision == nil {
		t.Fatalf("generated recipe v2 binding = %#v", recipe)
	}
	initialRevision := recipe.CurrentRevision
	if initialRevision.SpecVersion != model.RecipeSpecVersion2 || initialRevision.IntegrationRevisionID == "" || initialRevision.IntegrationManifestHash == "" {
		t.Fatalf("generated recipe revision provenance = %#v", initialRevision)
	}
	var spec model.RecipeSpec
	if err := json.Unmarshal(initialRevision.Spec, &spec); err != nil {
		t.Fatalf("decode generated recipe spec: %v", err)
	}
	if spec.IntegrationID != selected.ID || spec.SchemaVersion != model.RecipeSpecVersion2 || len(spec.CapabilityIDs) != 1 || len(spec.Steps) < 2 || len(spec.Checks) < 1 {
		t.Fatalf("generated recipe spec = %#v", spec)
	}

	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/recipes/"+recipe.ID, "doko_admin_demo", `{"markdown":"# Unstructured override","references":[],"visibility":"private"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `unknown field \"markdown\"`) {
		t.Fatalf("legacy Markdown update status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/recipes/"+recipe.ID, "doko_admin_demo", `{"visibility":"private"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "spec is required") {
		t.Fatalf("missing structured spec status=%d body=%s", w.Code, w.Body.String())
	}

	if spec.Prerequisites == nil {
		spec.Prerequisites = []model.RecipeInstruction{}
	}
	if spec.ReferenceIDs == nil {
		spec.ReferenceIDs = []string{}
	}
	patchBody, err := json.Marshal(map[string]any{"revision": recipe.Revision, "current_revision_id": recipe.CurrentRevisionID, "spec": spec, "visibility": model.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/recipes/"+recipe.ID, "doko_admin_demo", string(patchBody))
	if w.Code != http.StatusOK {
		t.Fatalf("structured recipe update status=%d body=%s", w.Code, w.Body.String())
	}
	var updated model.Recipe
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CurrentRevision == nil || updated.CurrentRevisionID == initialRevision.ID || updated.CurrentRevision.GeneratedBy != "human" || updated.CurrentRevision.SpecVersion != model.RecipeSpecVersion2 {
		t.Fatalf("updated structured revision = %#v", updated.CurrentRevision)
	}
	if updated.CurrentRevision.IntegrationRevisionID != initialRevision.IntegrationRevisionID || updated.CurrentRevision.IntegrationManifestHash != initialRevision.IntegrationManifestHash {
		t.Fatalf("updated recipe lost integration provenance: before=%#v after=%#v", initialRevision, updated.CurrentRevision)
	}
	var updatedSpec model.RecipeSpec
	if err := json.Unmarshal(updated.CurrentRevision.Spec, &updatedSpec); err != nil || updatedSpec.SchemaVersion != model.RecipeSpecVersion2 {
		t.Fatalf("updated recipe spec=%#v err=%v", updatedSpec, err)
	}
	w = request(t, handler, http.MethodPatch, "/api/v1/products/prod_acme/recipes/"+recipe.ID, "doko_admin_demo", string(patchBody))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "recipe_revision_conflict") {
		t.Fatalf("stale structured recipe update status=%d body=%s", w.Code, w.Body.String())
	}

	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/analyses/"+analysis.ID+"/recipes", "doko_admin_demo", `{"integration_id":"`+other.ID+`"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not scoped") {
		t.Fatalf("cross-integration generation status=%d body=%s", w.Code, w.Body.String())
	}
}
