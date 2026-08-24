package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestRecipeHTTPFlowCarriesSelectedIntegrationScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_recipe_http"}
	selected, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "payments-api", VersionKey: "v1", DisplayName: "Payments API", Description: "Payment operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "messages-api", VersionKey: "v1", DisplayName: "Messages API", Description: "Message operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true})

	w := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/analyses", "doko_admin_demo", `{"integration_id":"`+selected.ID+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("scoped analysis status=%d body=%s", w.Code, w.Body.String())
	}
	var analysis model.IntegrationAnalysis
	if err := json.Unmarshal(w.Body.Bytes(), &analysis); err != nil {
		t.Fatal(err)
	}
	if len(analysis.Plan.Recipes) != 1 || analysis.Plan.Recipes[0].Slug != "connect-acme-payments-api-v1-to-mcp" {
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
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"slug":"connect-acme-payments-api-v1-to-mcp"`) {
		t.Fatalf("scoped generation status=%d body=%s", w.Code, w.Body.String())
	}
	w = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/analyses/"+analysis.ID+"/recipes", "doko_admin_demo", `{"integration_id":"`+other.ID+`"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not scoped") {
		t.Fatalf("cross-integration generation status=%d body=%s", w.Code, w.Body.String())
	}
}
