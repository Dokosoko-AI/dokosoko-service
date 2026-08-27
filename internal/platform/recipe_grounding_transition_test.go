package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type recipeGroundingTransitionFixture struct {
	ctx           context.Context
	memory        *store.Memory
	service       *platform.Service
	actor         platform.Actor
	integrationID string
	recipe        model.Recipe
}

type recipeGroundingRaceStore struct {
	store.Store
	beforeSave func(model.Recipe)
}

type recipeDeleteCatalogConflictStore struct {
	store.Store
	deleteCalls int
}

func (r *recipeDeleteCatalogConflictStore) DeleteRecipe(ctx context.Context, productID, recipeID string, mutation store.RecipeMutation) error {
	r.deleteCalls++
	if r.deleteCalls == 1 {
		return store.ErrCatalogConflict
	}
	return r.Store.DeleteRecipe(ctx, productID, recipeID, mutation)
}

func (r *recipeGroundingRaceStore) SaveRecipeTransition(ctx context.Context, recipe model.Recipe, mutation store.RecipeMutation) (model.Recipe, error) {
	if r.beforeSave != nil {
		hook := r.beforeSave
		r.beforeSave = nil
		hook(recipe)
	}
	return r.Store.SaveRecipeTransition(ctx, recipe, mutation)
}

func createTransitionRecipe(t *testing.T, backend store.Store) recipeGroundingTransitionFixture {
	t.Helper()
	ctx := context.Background()
	memory, ok := backend.(*store.Memory)
	if !ok {
		if racing, racingOK := backend.(*recipeGroundingRaceStore); racingOK {
			memory = racing.Store.(*store.Memory)
		}
	}
	service := platform.New(backend)
	actor := platform.Actor{ID: "grounding-reviewer", RequestID: "recipe-grounding-transition"}
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "grounding-api", VersionKey: "v1", DisplayName: "Grounding API", Description: "Create readiness checks.", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "grounding.read", Name: "Read grounding state", Description: "Read grounding state.", ActionType: "read", DecisionTTLSeconds: 60, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := service.CreateTool(ctx, platform.ToolInput{ProductID: "prod_acme", Scope: model.ToolScopeAPI, OwnerIntegrationID: integration.ID, Namespace: "grounding", Name: "readiness", Description: "Read product readiness.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`), Endpoint: "https://api.example.test/readiness", HTTPMethod: "GET", UpstreamAuth: json.RawMessage(`{"type":"none"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 1000}, actor)
	if err != nil {
		t.Fatal(err)
	}
	tool, err = service.PublishTool(ctx, "prod_acme", tool.ID, tool.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, integration.ID, []platform.ToolRevisionSelection{{ToolID: tool.ID, Revision: tool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishIntegration(ctx, integration.ID, actor); err != nil {
		t.Fatal(err)
	}
	analysis, err := service.AnalyseIntegrationFor(ctx, "prod_acme", integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := service.GenerateRecipesForIntegration(ctx, "prod_acme", analysis.ID, integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 1 || recipes[0].CurrentRevision == nil {
		t.Fatalf("generated recipes = %#v", recipes)
	}
	return recipeGroundingTransitionFixture{ctx: ctx, memory: memory, service: service, actor: actor, integrationID: integration.ID, recipe: recipes[0]}
}

func newRecipeGroundingTransitionFixture(t *testing.T) recipeGroundingTransitionFixture {
	return createTransitionRecipe(t, store.NewMemory())
}

func newRecipeGroundingRaceFixture(t *testing.T) (recipeGroundingTransitionFixture, *recipeGroundingRaceStore) {
	memory := store.NewMemory()
	racing := &recipeGroundingRaceStore{Store: memory}
	return createTransitionRecipe(t, racing), racing
}

func (fixture recipeGroundingTransitionFixture) changeEvidence(t *testing.T) {
	t.Helper()
	integration, err := fixture.memory.Integration(fixture.ctx, fixture.recipe.ProductID, fixture.integrationID)
	if err != nil {
		t.Fatal(err)
	}
	integration, err = fixture.service.UpdateIntegration(fixture.ctx, integration.ID, platform.IntegrationInput{FamilyKey: integration.FamilyKey, VersionKey: integration.VersionKey, DisplayName: integration.DisplayName, Description: integration.Description + " Changed.", Visibility: integration.Visibility, Lifecycle: integration.Lifecycle, Revision: integration.Revision}, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.PublishIntegration(fixture.ctx, integration.ID, fixture.actor); err != nil {
		t.Fatal(err)
	}
}

func (fixture recipeGroundingTransitionFixture) requireStoredOutdated(t *testing.T, originalRevisionID string) {
	t.Helper()
	stored, err := fixture.memory.Recipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "outdated" || !stored.NeedsAttention || stored.CurrentRevisionID != originalRevisionID || stored.ApprovedAt != nil || stored.PublishedAt != nil {
		t.Fatalf("stale recipe transition was not denied safely: %#v", stored)
	}
}

func TestRecipeEditApprovalAndPublicationCheckCurrentGrounding(t *testing.T) {
	t.Run("structured edit", func(t *testing.T) {
		fixture := newRecipeGroundingTransitionFixture(t)
		originalRevisionID := fixture.recipe.CurrentRevisionID
		fixture.changeEvidence(t)
		var spec model.RecipeSpec
		if err := json.Unmarshal(fixture.recipe.CurrentRevision.Spec, &spec); err != nil {
			t.Fatal(err)
		}
		updated, err := fixture.service.UpdateRecipeReferences(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, spec.ReferenceIDs, fixture.recipe.Visibility, fixture.actor)
		if !errors.Is(err, platform.ErrRecipeGroundingChanged) || updated.State != "outdated" {
			t.Fatalf("edit result=%#v err=%v", updated, err)
		}
		fixture.requireStoredOutdated(t, originalRevisionID)
	})

	t.Run("approval", func(t *testing.T) {
		fixture := newRecipeGroundingTransitionFixture(t)
		originalRevisionID := fixture.recipe.CurrentRevisionID
		fixture.changeEvidence(t)
		approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
		if !errors.Is(err, platform.ErrRecipeGroundingChanged) || approved.State != "outdated" {
			t.Fatalf("approval result=%#v err=%v", approved, err)
		}
		fixture.requireStoredOutdated(t, originalRevisionID)
	})

	t.Run("publication", func(t *testing.T) {
		fixture := newRecipeGroundingTransitionFixture(t)
		approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
		if err != nil {
			t.Fatal(err)
		}
		fixture.recipe = approved
		originalRevisionID := approved.CurrentRevisionID
		fixture.changeEvidence(t)
		published, err := fixture.service.PublishRecipe(fixture.ctx, approved.ProductID, approved.ID, approved.Revision, approved.CurrentRevisionID, fixture.actor)
		if !errors.Is(err, platform.ErrRecipeGroundingChanged) || published.State != "outdated" {
			t.Fatalf("publication result=%#v err=%v", published, err)
		}
		fixture.requireStoredOutdated(t, originalRevisionID)
	})
}

func TestRecipeApprovalDoesNotRegressPublishedLifecycle(t *testing.T) {
	fixture := newRecipeGroundingTransitionFixture(t)
	approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	published, err := fixture.service.PublishRecipe(fixture.ctx, approved.ProductID, approved.ID, approved.Revision, approved.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}

	result, err := fixture.service.ApproveRecipe(fixture.ctx, published.ProductID, published.ID, published.Revision, published.CurrentRevisionID, fixture.actor)
	if err == nil {
		t.Fatalf("published recipe was moved backward to approval: %#v", result)
	}
	stored, lookupErr := fixture.memory.Recipe(fixture.ctx, published.ProductID, published.ID)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	if stored.State != "published" || stored.PublishedAt == nil || stored.Revision != published.Revision {
		t.Fatalf("rejected approval mutated the publication: %#v", stored)
	}
}

func TestDeleteRecipeRejectsCurrentV2AndRemovesOutdatedAggregate(t *testing.T) {
	fixture := newRecipeGroundingTransitionFixture(t)
	current := fixture.recipe
	if err := fixture.service.DeleteRecipe(fixture.ctx, current.ProductID, current.ID, current.Revision, current.CurrentRevisionID, fixture.actor); !errors.Is(err, platform.ErrRecipeDeletionNotAllowed) {
		t.Fatalf("current v2 recipe deletion error = %v, want deletion not allowed", err)
	}
	if _, err := fixture.memory.Recipe(fixture.ctx, current.ProductID, current.ID); err != nil {
		t.Fatalf("rejected deletion removed the current recipe: %v", err)
	}

	fixture.changeEvidence(t)
	values, err := fixture.service.ReconcileRecipeDrift(fixture.ctx, current.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	var outdated model.Recipe
	for _, value := range values {
		if value.ID == current.ID {
			outdated = value
			break
		}
	}
	if outdated.State != "outdated" || outdated.CurrentRevisionID == "" {
		t.Fatalf("recipe was not reconciled to an outdated deletable record: %#v", outdated)
	}
	if err := fixture.service.DeleteRecipe(fixture.ctx, outdated.ProductID, outdated.ID, outdated.Revision, outdated.CurrentRevisionID, fixture.actor); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.memory.Recipe(fixture.ctx, outdated.ProductID, outdated.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted recipe lookup error = %v, want not found", err)
	}
	if _, err := fixture.memory.RecipeRevisions(fixture.ctx, outdated.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted revisions lookup error = %v, want not found", err)
	}
	events, err := fixture.memory.AuditEvents(fixture.ctx, outdated.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	foundDelete := false
	for _, event := range events {
		foundDelete = foundDelete || event.Action == "recipe.deleted" && event.TargetID == outdated.ID
	}
	if !foundDelete {
		t.Fatalf("recipe deletion audit was not retained: %#v", events)
	}
}

func TestDeleteRecipeRetriesCatalogConflictWhenRecipeRevisionIsStillCurrent(t *testing.T) {
	fixture := newRecipeGroundingTransitionFixture(t)
	fixture.changeEvidence(t)
	values, err := fixture.service.ReconcileRecipeDrift(fixture.ctx, fixture.recipe.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	var outdated model.Recipe
	for _, value := range values {
		if value.ID == fixture.recipe.ID {
			outdated = value
			break
		}
	}
	if outdated.State != "outdated" {
		t.Fatalf("recipe was not reconciled to an outdated record: %#v", outdated)
	}

	conflicting := &recipeDeleteCatalogConflictStore{Store: fixture.memory}
	service := platform.New(conflicting)
	if err := service.DeleteRecipe(fixture.ctx, outdated.ProductID, outdated.ID, outdated.Revision, outdated.CurrentRevisionID, fixture.actor); err != nil {
		t.Fatal(err)
	}
	if conflicting.deleteCalls != 2 {
		t.Fatalf("delete calls = %d, want one catalog-conflict retry", conflicting.deleteCalls)
	}
	if _, err := fixture.memory.Recipe(fixture.ctx, outdated.ProductID, outdated.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted recipe lookup error = %v, want not found", err)
	}
}

func TestRecipeDecisionsRejectStaleOperatorRevision(t *testing.T) {
	fixture := newRecipeGroundingTransitionFixture(t)
	original := fixture.recipe
	var spec model.RecipeSpec
	if err := json.Unmarshal(original.CurrentRevision.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	updated, err := fixture.service.UpdateRecipeReferences(fixture.ctx, original.ProductID, original.ID, original.Revision, original.CurrentRevisionID, spec.ReferenceIDs, original.Visibility, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}

	if result, approveErr := fixture.service.ApproveRecipe(fixture.ctx, original.ProductID, original.ID, original.Revision, original.CurrentRevisionID, fixture.actor); !errors.Is(approveErr, store.ErrConflict) {
		t.Fatalf("stale approval result=%#v err=%v", result, approveErr)
	}
	stored, err := fixture.memory.Recipe(fixture.ctx, original.ProductID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "review" || stored.CurrentRevisionID != updated.CurrentRevisionID || stored.Revision != updated.Revision {
		t.Fatalf("stale approval changed the recipe: %#v", stored)
	}

	approved, err := fixture.service.ApproveRecipe(fixture.ctx, updated.ProductID, updated.ID, updated.Revision, updated.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	var approvedSpec model.RecipeSpec
	if err := json.Unmarshal(approved.CurrentRevision.Spec, &approvedSpec); err != nil {
		t.Fatal(err)
	}
	reworked, err := fixture.service.UpdateRecipeReferences(fixture.ctx, approved.ProductID, approved.ID, approved.Revision, approved.CurrentRevisionID, approvedSpec.ReferenceIDs, approved.Visibility, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	bumped, err := fixture.service.ApproveRecipe(fixture.ctx, reworked.ProductID, reworked.ID, reworked.Revision, reworked.CurrentRevisionID, fixture.actor)
	if err != nil {
		t.Fatal(err)
	}
	if result, publishErr := fixture.service.PublishRecipe(fixture.ctx, approved.ProductID, approved.ID, approved.Revision, approved.CurrentRevisionID, fixture.actor); !errors.Is(publishErr, store.ErrConflict) {
		t.Fatalf("stale publication result=%#v err=%v", result, publishErr)
	}
	stored, err = fixture.memory.Recipe(fixture.ctx, original.ProductID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "approved" || stored.Revision != bumped.Revision || stored.CurrentRevisionID != bumped.CurrentRevisionID {
		t.Fatalf("stale publication changed the recipe: %#v", stored)
	}
}

func TestRecipeTransitionsDetectIntegrationChangeDuringSave(t *testing.T) {
	for _, transition := range []string{"approve", "publish"} {
		t.Run(transition, func(t *testing.T) {
			fixture, racing := newRecipeGroundingRaceFixture(t)
			if transition == "publish" {
				approved, err := fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
				if err != nil {
					t.Fatal(err)
				}
				fixture.recipe = approved
			}
			originalRevisionID := fixture.recipe.CurrentRevisionID
			action := "recipe." + transition + "d"
			if transition == "publish" {
				action = "recipe.published"
			}
			auditBefore := fixture.recipeAuditActionCount(t, action)
			outdatedAuditBefore := fixture.recipeAuditActionCount(t, "recipe.outdated")
			var catalogAfterEvidence int64
			racing.beforeSave = func(recipe model.Recipe) {
				fixture.changeEvidence(t)
				product, productErr := fixture.memory.Product(fixture.ctx, fixture.recipe.ProductID)
				if productErr != nil {
					t.Fatal(productErr)
				}
				catalogAfterEvidence = product.CatalogRevision
			}
			var result model.Recipe
			var err error
			if transition == "approve" {
				result, err = fixture.service.ApproveRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
			} else {
				result, err = fixture.service.PublishRecipe(fixture.ctx, fixture.recipe.ProductID, fixture.recipe.ID, fixture.recipe.Revision, fixture.recipe.CurrentRevisionID, fixture.actor)
			}
			if !errors.Is(err, platform.ErrRecipeGroundingChanged) || result.State != "outdated" {
				t.Fatalf("raced %s result=%#v err=%v", transition, result, err)
			}
			fixture.requireStoredOutdated(t, originalRevisionID)
			if got := fixture.recipeAuditActionCount(t, action); got != auditBefore {
				t.Fatalf("raced %s wrote a false lifecycle audit: count %d -> %d", transition, auditBefore, got)
			}
			if got := fixture.recipeAuditActionCount(t, "recipe.outdated"); got != outdatedAuditBefore+1 {
				t.Fatalf("raced %s did not atomically audit withdrawal: count %d -> %d", transition, outdatedAuditBefore, got)
			}
			product, productErr := fixture.memory.Product(fixture.ctx, fixture.recipe.ProductID)
			if productErr != nil {
				t.Fatal(productErr)
			}
			if product.CatalogRevision != catalogAfterEvidence {
				t.Fatalf("raced %s added a recipe catalog bump: got %d want %d", transition, product.CatalogRevision, catalogAfterEvidence)
			}
		})
	}
}

func (fixture recipeGroundingTransitionFixture) recipeAuditActionCount(t *testing.T, action string) int {
	t.Helper()
	events, err := fixture.memory.AuditEvents(fixture.ctx, "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.TargetType == "recipe" && event.TargetID == fixture.recipe.ID && event.Action == action {
			count++
		}
	}
	return count
}
