package platform_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/testutil"
)

func TestRecipeReferenceOptionsUseExactEvidenceWithoutMutation(t *testing.T) {
	memory := store.NewMemory()
	service := testutil.NewRecipeService(t, memory, testutil.RecipeAI{})
	actor := platform.Actor{ID: "reference-reviewer"}
	integration, tool, publication := configureMultiAPIRecipeIntegration(t, memory, service, "api-key-api", "api_key", "API key", actor)
	_, generated := analyseAndGenerateRecipeV2(t, recipeV2Fixture{memory: memory, service: service, actor: actor, integration: integration, tool: tool, publication: publication})
	fixture := recipeGroundingTransitionFixture{ctx: t.Context(), memory: memory, service: service, actor: actor, integrationID: integration.ID, recipe: generated}
	recipe := fixture.recipe
	options, err := fixture.service.RecipeReferenceOptions(fixture.ctx, recipe.ProductID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if options.ProductID != recipe.ProductID || options.RecipeID != recipe.ID || options.RecipeRevision != recipe.Revision || options.CurrentRevisionID != recipe.CurrentRevisionID || options.Items == nil {
		t.Fatalf("options identity = %#v", options)
	}
	if len(options.Items) == 0 {
		t.Fatalf("fixture should offer product references: %#v", options)
	}
	dependencies := map[string]bool{}
	for _, item := range recipe.Dependencies {
		dependencies[item.Kind+":"+item.ResourceID] = true
	}
	ids := []string{}
	for _, option := range options.Items {
		if option.Reference.ResourceID == "" || option.Reference.Label == "" || option.Reference.URL == "" || len(option.Evidence) == 0 {
			t.Fatalf("unreadable option = %#v", option)
		}
		for _, evidence := range option.Evidence {
			if !dependencies[evidence.Kind+":"+evidence.ResourceID] || evidence.Fingerprint == "" {
				t.Fatalf("option outside exact recipe dependencies: %#v", evidence)
			}
		}
		ids = append(ids, option.Reference.ResourceID)
	}
	stored, err := fixture.memory.Recipe(fixture.ctx, recipe.ProductID, recipe.ID)
	if err != nil || !reflect.DeepEqual(stored, recipe) {
		t.Fatalf("reading changed recipe: err=%v", err)
	}
	updated, err := fixture.service.UpdateRecipeReferences(fixture.ctx, recipe.ProductID, recipe.ID, recipe.Revision, recipe.CurrentRevisionID, ids, recipe.Visibility, fixture.actor)
	if err != nil {
		t.Fatalf("select offered references: %v", err)
	}
	var spec model.RecipeSpec
	if err := json.Unmarshal(updated.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(spec.ReferenceIDs, ids) || updated.CurrentRevisionID == recipe.CurrentRevisionID || updated.ApprovedAt != nil || updated.PublishedAt != nil {
		t.Fatalf("selection did not create an unapproved revision: %#v", updated)
	}
	removed, err := fixture.service.UpdateRecipeReferences(fixture.ctx, recipe.ProductID, recipe.ID, updated.Revision, updated.CurrentRevisionID, []string{}, recipe.Visibility, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	restoredOptions, err := fixture.service.RecipeReferenceOptions(fixture.ctx, recipe.ProductID, recipe.ID, removed.Revision, removed.CurrentRevisionID)
	if err != nil || !reflect.DeepEqual(options.Items, restoredOptions.Items) {
		t.Fatalf("removed references cannot be restored: options=%#v err=%v", restoredOptions, err)
	}
	_, err = fixture.service.UpdateRecipeReferences(fixture.ctx, recipe.ProductID, recipe.ID, removed.Revision, removed.CurrentRevisionID, []string{"unrelated-document"}, recipe.Visibility, fixture.actor)
	if err == nil {
		t.Fatal("accepted reference outside exact evidence")
	}
}

func TestRecipeReferenceOptionsRejectStaleOrForeignScopeWithoutMutation(t *testing.T) {
	fixture := newRecipeGroundingTransitionFixture(t)
	recipe := fixture.recipe
	for _, input := range []struct {
		product  string
		revision int64
		current  string
		expected error
	}{
		{recipe.ProductID, recipe.Revision + 1, recipe.CurrentRevisionID, store.ErrConflict},
		{recipe.ProductID, recipe.Revision, "different-revision", store.ErrConflict},
		{"other-deployment", recipe.Revision, recipe.CurrentRevisionID, store.ErrNotFound},
	} {
		_, err := fixture.service.RecipeReferenceOptions(fixture.ctx, input.product, recipe.ID, input.revision, input.current)
		if !errors.Is(err, input.expected) {
			t.Fatalf("scope=%#v error=%v", input, err)
		}
	}
	fixture.changeEvidence(t)
	before, err := fixture.memory.Recipe(fixture.ctx, recipe.ProductID, recipe.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.RecipeReferenceOptions(fixture.ctx, recipe.ProductID, recipe.ID, before.Revision, before.CurrentRevisionID)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("stale evidence allowed: %v", err)
	}
	after, err := fixture.memory.Recipe(fixture.ctx, recipe.ProductID, recipe.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read changed recipe after evidence drift: err=%v", err)
	}
}
