package platform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type recipeGroundingTransitionFixture struct {
	ctx     context.Context
	memory  *store.Memory
	service *platform.Service
	actor   platform.Actor
	recipe  model.Recipe
}

type recipeGroundingRaceStore struct {
	store.Store
	beforeSave func(model.Recipe)
}

func (r *recipeGroundingRaceStore) SaveRecipe(ctx context.Context, recipe model.Recipe, expectedRevision int64) (model.Recipe, error) {
	if r.beforeSave != nil {
		hook := r.beforeSave
		r.beforeSave = nil
		hook(recipe)
	}
	return r.Store.SaveRecipe(ctx, recipe, expectedRevision)
}

func newRecipeGroundingTransitionFixture(t *testing.T) recipeGroundingTransitionFixture {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "grounding-reviewer", RequestID: "recipe-grounding-transition"}
	analysis, err := service.AnalyseIntegration(ctx, "prod_acme", actor)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := service.GenerateRecipes(ctx, "prod_acme", analysis.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 1 || recipes[0].CurrentRevision == nil {
		t.Fatalf("generated recipes = %#v", recipes)
	}
	return recipeGroundingTransitionFixture{ctx: ctx, memory: memory, service: service, actor: actor, recipe: recipes[0]}
}

func newRecipeGroundingRaceFixture(t *testing.T) (recipeGroundingTransitionFixture, *recipeGroundingRaceStore) {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	racing := &recipeGroundingRaceStore{Store: memory}
	service := platform.New(racing)
	actor := platform.Actor{ID: "grounding-reviewer", RequestID: "recipe-grounding-race"}
	analysis, err := service.AnalyseIntegration(ctx, "prod_acme", actor)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := service.GenerateRecipes(ctx, "prod_acme", analysis.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 1 || recipes[0].CurrentRevision == nil {
		t.Fatalf("generated recipes = %#v", recipes)
	}
	return recipeGroundingTransitionFixture{ctx: ctx, memory: memory, service: service, actor: actor, recipe: recipes[0]}, racing
}

func (fixture recipeGroundingTransitionFixture) changeEvidence(t *testing.T) {
	t.Helper()
	source, err := fixture.memory.Source(fixture.ctx, fixture.recipe.ProductID, "src_docs")
	if err != nil {
		t.Fatal(err)
	}
	source.Name = "Developer documentation changed after recipe generation"
	if _, err := fixture.memory.UpdateSource(fixture.ctx, source, source.Revision); err != nil {
		t.Fatal(err)
	}
}

func (fixture recipeGroundingTransitionFixture) requireStoredOutdated(t *testing.T, originalRevisionID string) model.Recipe {
	t.Helper()
	stored, err := fixture.memory.Recipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "outdated" || !stored.NeedsAttention {
		t.Fatalf("stale recipe was not marked for review: %#v", stored)
	}
	if stored.CurrentRevisionID != originalRevisionID {
		t.Fatalf("denied transition changed recipe content: revision ID = %q, want %q", stored.CurrentRevisionID, originalRevisionID)
	}
	return stored
}

func TestRecipeEditChecksCurrentEvidenceSynchronously(t *testing.T) {
	t.Parallel()
	fixture := newRecipeGroundingTransitionFixture(t)
	originalRevisionID := fixture.recipe.CurrentRevisionID
	fixture.changeEvidence(t)

	updated, err := fixture.service.UpdateRecipeMarkdown(
		fixture.ctx,
		fixture.recipe.ProductID,
		fixture.recipe.ID,
		fixture.recipe.CurrentRevision.Markdown+"\n",
		fixture.recipe.CurrentRevision.References,
		fixture.recipe.Visibility,
		fixture.actor,
	)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("edit error = %v, want %v", err, platform.ErrRecipeGroundingChanged)
	}
	if updated.State != "outdated" || !updated.NeedsAttention {
		t.Fatalf("returned recipe = %#v", updated)
	}
	fixture.requireStoredOutdated(t, originalRevisionID)
}

func TestRecipeApprovalChecksCurrentEvidenceSynchronously(t *testing.T) {
	t.Parallel()
	fixture := newRecipeGroundingTransitionFixture(t)
	originalRevisionID := fixture.recipe.CurrentRevisionID
	fixture.changeEvidence(t)

	approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.actor)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("approval error = %v, want %v", err, platform.ErrRecipeGroundingChanged)
	}
	if approved.State != "outdated" || !approved.NeedsAttention || approved.ApprovedAt != nil {
		t.Fatalf("returned recipe = %#v", approved)
	}
	fixture.requireStoredOutdated(t, originalRevisionID)
}

func TestRecipePublicationChecksCurrentEvidenceSynchronously(t *testing.T) {
	t.Parallel()
	fixture := newRecipeGroundingTransitionFixture(t)
	approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	fixture.recipe = approved
	originalRevisionID := approved.CurrentRevisionID
	fixture.changeEvidence(t)

	published, err := fixture.service.PublishRecipe(fixture.ctx, approved.ProductID, approved.ID, fixture.actor)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("publication error = %v, want %v", err, platform.ErrRecipeGroundingChanged)
	}
	if published.State != "outdated" || !published.NeedsAttention || published.PublishedAt != nil {
		t.Fatalf("returned recipe = %#v", published)
	}
	stored := fixture.requireStoredOutdated(t, originalRevisionID)
	if stored.PublishedAt != nil {
		t.Fatalf("denied publication set published_at: %#v", stored)
	}
}

func TestRecipeApprovalDetectsEvidenceChangeDuringTransition(t *testing.T) {
	t.Parallel()
	fixture, racing := newRecipeGroundingRaceFixture(t)
	originalRevisionID := fixture.recipe.CurrentRevisionID
	racing.beforeSave = func(recipe model.Recipe) {
		if recipe.State != "approved" {
			t.Fatalf("intercepted recipe state = %q, want approved", recipe.State)
		}
		fixture.changeEvidence(t)
	}

	approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.actor)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("approval error = %v, want %v", err, platform.ErrRecipeGroundingChanged)
	}
	if approved.State != "outdated" || approved.ApprovedAt != nil {
		t.Fatalf("raced approval result = %#v", approved)
	}
	fixture.requireStoredOutdated(t, originalRevisionID)
}

func TestRecipePublicationDetectsEvidenceChangeDuringTransition(t *testing.T) {
	t.Parallel()
	fixture, racing := newRecipeGroundingRaceFixture(t)
	approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	fixture.recipe = approved
	originalRevisionID := approved.CurrentRevisionID
	racing.beforeSave = func(recipe model.Recipe) {
		if recipe.State != "published" {
			t.Fatalf("intercepted recipe state = %q, want published", recipe.State)
		}
		fixture.changeEvidence(t)
	}

	published, err := fixture.service.PublishRecipe(fixture.ctx, approved.ProductID, approved.ID, fixture.actor)
	if !errors.Is(err, platform.ErrRecipeGroundingChanged) {
		t.Fatalf("publication error = %v, want %v", err, platform.ErrRecipeGroundingChanged)
	}
	if published.State != "outdated" || published.PublishedAt != nil {
		t.Fatalf("raced publication result = %#v", published)
	}
	fixture.requireStoredOutdated(t, originalRevisionID)
}
