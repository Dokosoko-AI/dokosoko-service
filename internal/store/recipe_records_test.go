package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryCreateRecipeWithRevisionPersistsProductIntegrationContractAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	integration, err := memory.CreateIntegration(ctx, model.Integration{
		ID:             "integration_recipe_v2",
		DeploymentID:   "prod_acme",
		OrganisationID: "org_acme",
		FamilyKey:      "payments",
		VersionKey:     "v1",
		DisplayName:    "Payments",
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := "sha256:" + strings.Repeat("a", 64)
	publishedAt := time.Now().UTC()
	integrationRevision, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{
		ID:            "integration_revision_recipe_v2",
		IntegrationID: integration.ID,
		Revision:      1,
		State:         "published",
		Snapshot:      json.RawMessage(`{"family_key":"payments"}`),
		ManifestHash:  manifestHash,
		PublishedAt:   &publishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	recipe := model.Recipe{
		ID:              "recipe_v2",
		OrganisationID:  "org_acme",
		ProductID:       "prod_acme",
		IntegrationID:   integration.ID,
		ContractVersion: model.RecipeContractProductIntegrationV2,
		Slug:            "create-payment",
		Title:           "Create a payment",
		Outcome:         "The application creates one payment.",
		Audience:        "coding agent",
		State:           "draft",
		NeedsAttention:  true,
		Visibility:      model.VisibilityPrivate,
		StableURI:       "dokosoko://products/acme/recipes/create-payment",
	}
	spec, err := json.Marshal(model.RecipeSpec{
		SchemaVersion: model.RecipeSpecVersion2,
		IntegrationID: integration.ID,
		Title:         recipe.Title,
		Outcome:       recipe.Outcome,
		CapabilityIDs: []string{"payments.create"},
		Steps: []model.RecipeInstruction{{
			Action: "Call the create-payment operation from the checkout adapter.",
			Evidence: []model.RecipeEvidenceRef{{
				Kind:        "tool",
				ResourceID:  "payments.create",
				Fingerprint: "tool-v1",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	promptHash := "sha256:" + strings.Repeat("b", 64)
	revision := model.RecipeRevision{
		ID:                      "recipe_revision_v2",
		RecipeID:                recipe.ID,
		SpecVersion:             model.RecipeSpecVersion2,
		Spec:                    spec,
		Markdown:                "# Create a payment\n",
		GeneratedBy:             "ai",
		Model:                   "authoring-model",
		IntegrationRevisionID:   integrationRevision.ID,
		IntegrationManifestHash: manifestHash,
		PromptVersion:           "recipe-authoring-v9",
		PromptHash:              promptHash,
	}
	saved, err := memory.CreateRecipeWithRevision(ctx, recipe, revision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || saved.IntegrationID != integration.ID || saved.ContractVersion != model.RecipeContractProductIntegrationV2 || saved.CurrentRevisionID != revision.ID || saved.CurrentRevision == nil || saved.CurrentRevision.Revision != 1 {
		t.Fatalf("saved recipe binding = %#v", saved)
	}
	current := saved.CurrentRevision
	if current.SpecVersion != model.RecipeSpecVersion2 || current.IntegrationRevisionID != integrationRevision.ID || current.IntegrationManifestHash != manifestHash || current.PromptVersion != "recipe-authoring-v9" || current.PromptHash != promptHash || !json.Valid(current.Spec) {
		t.Fatalf("saved revision provenance = %#v", current)
	}
	roundTrip, err := memory.Recipe(ctx, recipe.ProductID, recipe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.CurrentRevision == nil || roundTrip.CurrentRevision.ID != revision.ID {
		t.Fatalf("round-trip recipe was not hydrated: %#v", roundTrip)
	}

	conflictingRecipe := recipe
	conflictingRecipe.ID = "recipe_v2_rolled_back"
	conflictingRecipe.Slug = "create-payment-rolled-back"
	conflictingRecipe.StableURI = "dokosoko://products/acme/recipes/create-payment-rolled-back"
	conflictingRevision := revision
	conflictingRevision.RecipeID = conflictingRecipe.ID
	if _, err := memory.CreateRecipeWithRevision(ctx, conflictingRecipe, conflictingRevision); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate immutable revision ID error = %v, want conflict", err)
	}
	if _, err := memory.Recipe(ctx, conflictingRecipe.ProductID, conflictingRecipe.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed aggregate left a recipe behind: %v", err)
	}
	if _, err := memory.RecipeRevisions(ctx, conflictingRecipe.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed aggregate left a revision set behind: %v", err)
	}
}

func TestMemorySaveRecipeTransitionIsAtomicWithCatalogAndAudit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	recipe, err := memory.CreateRecipeWithRevision(ctx, model.Recipe{
		ID:              "recipe_transition",
		OrganisationID:  "org_acme",
		ProductID:       "prod_acme",
		ContractVersion: model.RecipeContractLegacyMCPV1,
		Slug:            "transition-recipe",
		Title:           "Transition recipe",
		Outcome:         "Exercise atomic transitions.",
		Audience:        "operator",
		State:           "approved",
		Generated:       true,
		Visibility:      model.VisibilityPrivate,
		StableURI:       "dokosoko://products/acme/recipes/transition-recipe",
	}, model.RecipeRevision{
		ID:          "recipe_transition_revision",
		RecipeID:    "recipe_transition",
		SpecVersion: 1,
		Spec:        json.RawMessage(`{}`),
		Markdown:    "# Transition recipe\n",
		GeneratedBy: "human",
		CreatedBy:   "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	deploymentBefore, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	productBefore, err := memory.Product(ctx, recipe.ProductID)
	if err != nil {
		t.Fatal(err)
	}

	recipe.State = "published"
	now := time.Now().UTC()
	recipe.PublishedAt = &now
	audit := model.AuditEvent{
		ID:             "audit_recipe_transition",
		OrganisationID: recipe.OrganisationID,
		ProductID:      recipe.ProductID,
		ActorID:        "root",
		Action:         "recipe.published",
		TargetType:     "recipe",
		TargetID:       recipe.ID,
		Prior:          map[string]any{"state": "approved"},
		Current:        map[string]any{"state": "published"},
		RequestID:      "request-transition",
		CreatedAt:      now,
	}
	published, err := memory.SaveRecipeTransition(ctx, recipe, recipe.Revision, true, &audit)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" || published.Revision != recipe.Revision+1 || published.CurrentRevision == nil || published.CurrentRevision.ID != recipe.CurrentRevisionID {
		t.Fatalf("published aggregate = %#v", published)
	}
	deploymentAfter, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	productAfter, err := memory.Product(ctx, recipe.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if deploymentAfter.CatalogRevision != deploymentBefore.CatalogRevision+1 || productAfter.CatalogRevision != deploymentAfter.CatalogRevision || productAfter.CatalogRevision != productBefore.CatalogRevision+1 {
		t.Fatalf("catalog revisions = deployment %d->%d product %d->%d", deploymentBefore.CatalogRevision, deploymentAfter.CatalogRevision, productBefore.CatalogRevision, productAfter.CatalogRevision)
	}
	events, err := memory.AuditEvents(ctx, recipe.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != audit.ID || events[0].Outcome != "success" {
		t.Fatalf("transition audit = %#v", events)
	}

	failed := published
	failed.State = "outdated"
	rollbackAudit := audit
	rollbackAudit.ID = "audit_recipe_transition_stale"
	if _, err := memory.SaveRecipeTransition(ctx, failed, recipe.Revision, true, &rollbackAudit); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition error = %v, want conflict", err)
	}
	assertMemoryRecipeTransitionUnchanged(t, memory, published, deploymentAfter.CatalogRevision, len(events))

	if _, err := memory.SaveRecipeTransition(ctx, failed, published.Revision, true, &audit); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate audit transition error = %v, want conflict", err)
	}
	assertMemoryRecipeTransitionUnchanged(t, memory, published, deploymentAfter.CatalogRevision, len(events))

	invalidAudit := rollbackAudit
	invalidAudit.ID = "audit_recipe_transition_invalid"
	invalidAudit.Current = map[string]any{"invalid": make(chan int)}
	if _, err := memory.SaveRecipeTransition(ctx, failed, published.Revision, true, &invalidAudit); err == nil {
		t.Fatal("transition accepted a non-JSON audit")
	}
	assertMemoryRecipeTransitionUnchanged(t, memory, published, deploymentAfter.CatalogRevision, len(events))
}

func assertMemoryRecipeTransitionUnchanged(t *testing.T, memory *Memory, want model.Recipe, catalogRevision int64, auditCount int) {
	t.Helper()
	ctx := context.Background()
	got, err := memory.Recipe(ctx, want.ProductID, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != want.Revision || got.State != want.State || got.CurrentRevisionID != want.CurrentRevisionID {
		t.Fatalf("recipe changed after failed transition: got %#v want %#v", got, want)
	}
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	product, err := memory.Product(ctx, want.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.CatalogRevision != catalogRevision || product.CatalogRevision != catalogRevision {
		t.Fatalf("catalog changed after failed transition: deployment=%d product=%d want=%d", deployment.CatalogRevision, product.CatalogRevision, catalogRevision)
	}
	events, err := memory.AuditEvents(ctx, want.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != auditCount {
		t.Fatalf("audit count after failed transition = %d, want %d", len(events), auditCount)
	}
}

func TestProductIntegrationRecipeRevisionRejectsInexactBindings(t *testing.T) {
	t.Parallel()
	recipe := model.Recipe{IntegrationID: "integration-a", ContractVersion: model.RecipeContractProductIntegrationV2}
	validSpec, err := json.Marshal(model.RecipeSpec{SchemaVersion: model.RecipeSpecVersion2, IntegrationID: recipe.IntegrationID})
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := "sha256:" + strings.Repeat("a", 64)
	promptHash := "sha256:" + strings.Repeat("b", 64)
	base := model.RecipeRevision{
		SpecVersion:             model.RecipeSpecVersion2,
		Spec:                    validSpec,
		GeneratedBy:             "ai",
		Model:                   "authoring-model",
		IntegrationRevisionID:   "revision-a",
		IntegrationManifestHash: manifestHash,
		PromptVersion:           "recipe-authoring-v9",
		PromptHash:              promptHash,
	}
	tests := []struct {
		name   string
		mutate func(*model.RecipeRevision)
	}{
		{name: "missing revision", mutate: func(value *model.RecipeRevision) { value.IntegrationRevisionID = "" }},
		{name: "wrong manifest digest", mutate: func(value *model.RecipeRevision) { value.IntegrationManifestHash = "sha256:not-a-digest" }},
		{name: "missing prompt provenance", mutate: func(value *model.RecipeRevision) { value.PromptVersion = "" }},
		{name: "missing model provenance", mutate: func(value *model.RecipeRevision) { value.Model = "" }},
		{name: "legacy spec version", mutate: func(value *model.RecipeRevision) { value.SpecVersion = 1 }},
		{name: "wrong integration", mutate: func(value *model.RecipeRevision) {
			value.Spec = json.RawMessage(`{"schema_version":2,"integration_id":"integration-b"}`)
		}},
		{name: "unknown spec field", mutate: func(value *model.RecipeRevision) {
			value.Spec = json.RawMessage(`{"schema_version":2,"integration_id":"integration-a","unexpected":true}`)
		}},
		{name: "non-object spec", mutate: func(value *model.RecipeRevision) { value.Spec = json.RawMessage(`[]`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if _, err := prepareRecipeRevisionRecord(recipe, value); err == nil {
				t.Fatal("invalid recipe revision binding was accepted")
			}
		})
	}
}

func TestLegacyRecipeRecordDefaultsRemainExplicit(t *testing.T) {
	t.Parallel()
	recipe, err := prepareRecipeRecord(model.Recipe{})
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ContractVersion != model.RecipeContractLegacyMCPV1 {
		t.Fatalf("legacy contract = %q", recipe.ContractVersion)
	}
	revision, err := prepareRecipeRevisionRecord(recipe, model.RecipeRevision{})
	if err != nil {
		t.Fatal(err)
	}
	if revision.SpecVersion != 1 || string(revision.Spec) != `{}` || revision.IntegrationRevisionID != "" || revision.IntegrationManifestHash != "" {
		t.Fatalf("legacy revision defaults = %#v", revision)
	}
}

func TestRecipeRecordRejectsUnknownOrInconsistentContracts(t *testing.T) {
	t.Parallel()
	for _, recipe := range []model.Recipe{
		{ContractVersion: "future-contract", IntegrationID: "integration-a"},
		{ContractVersion: model.RecipeContractLegacyMCPV1, IntegrationID: "integration-a"},
		{ContractVersion: model.RecipeContractProductIntegrationV2},
	} {
		if _, err := prepareRecipeRecord(recipe); err == nil {
			t.Fatalf("invalid recipe contract was accepted: %#v", recipe)
		}
	}
}
