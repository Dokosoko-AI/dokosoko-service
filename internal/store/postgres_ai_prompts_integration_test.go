package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPostgresAIPromptStateOptimisticConcurrency(t *testing.T) {
	_, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "AI prompt store", Slug: "ai-prompt-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "AI prompt store", Slug: "ai-prompt"}); err != nil {
		t.Fatal(err)
	}
	states, err := postgres.AIPromptStates(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Fatalf("initial persisted prompt states = %#v", states)
	}
	for _, key := range []string{
		"documentation.map_enrichment",
		"sdk.map_enrichment",
		"sdk.applicability_suggestion",
		"sdk.sample_review",
	} {
		state, saveErr := postgres.SaveAIPromptState(ctx, model.AIPromptState{
			ProductID:    productID,
			Key:          key,
			Instructions: "Use exact evidence IDs and report uncertainty.",
		}, 1)
		if saveErr != nil {
			t.Fatalf("save developer-asset prompt %q: %v", key, saveErr)
		}
		if state.Key != key || state.Revision != 2 {
			t.Fatalf("developer-asset prompt state = %#v", state)
		}
	}
	atomicKey := "recipe.review"
	invalidAudit := model.AuditEvent{
		ID:             "audit_invalid_" + productID,
		OrganisationID: organisationID,
		ProductID:      productID,
		Current:        map[string]any{"invalid": make(chan struct{})},
	}
	if _, err = postgres.SaveAIPromptStateAndAudit(ctx, model.AIPromptState{ProductID: productID, Key: atomicKey, Instructions: "Review exact evidence."}, 1, invalidAudit); err == nil {
		t.Fatal("invalid audit unexpectedly succeeded")
	}
	if _, err = postgres.AIPromptState(ctx, productID, atomicKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prompt state persisted without audit: %v", err)
	}

	audit := model.AuditEvent{
		ID:             "audit_prompt_" + productID,
		OrganisationID: organisationID,
		ProductID:      productID,
		Action:         "ai.prompt.saved",
		TargetType:     "ai_prompt",
		TargetID:       "recipe.authoring",
		CreatedAt:      time.Now().UTC(),
	}
	override, err := postgres.SaveAIPromptStateAndAudit(ctx, model.AIPromptState{ProductID: productID, Key: "recipe.authoring", Instructions: "Use exact reviewed evidence."}, 1, audit)
	if err != nil {
		t.Fatal(err)
	}
	if override.Revision != 2 || override.UpdatedAt.IsZero() {
		t.Fatalf("first persisted prompt state = %#v", override)
	}
	events, err := postgres.AuditEvents(ctx, organisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != audit.ID {
		t.Fatalf("atomic audit events = %#v", events)
	}
	if _, err = postgres.SaveAIPromptState(ctx, model.AIPromptState{ProductID: productID, Key: "recipe.authoring", Instructions: "Stale edit."}, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save error = %v, want conflict", err)
	}

	reset, err := postgres.SaveAIPromptState(ctx, model.AIPromptState{ProductID: productID, Key: "recipe.authoring"}, override.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Revision != 3 || reset.Instructions != "" {
		t.Fatalf("reset prompt state = %#v", reset)
	}
	if _, err = postgres.SaveAIPromptState(ctx, model.AIPromptState{ProductID: productID, Key: "recipe.authoring", Instructions: "ABA edit."}, override.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("pre-reset revision became valid again: %v", err)
	}
	stored, err := postgres.AIPromptState(ctx, productID, "recipe.authoring")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != reset.Revision || stored.Instructions != "" {
		t.Fatalf("stored reset state = %#v", stored)
	}
}
