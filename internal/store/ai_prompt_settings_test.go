package store

import (
	"context"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryAIPromptStateUsesVirtualDefaultRevision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	memory := NewMemory()
	states, err := memory.AIPromptStates(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Fatalf("initial persisted prompt states = %#v", states)
	}
	if _, err = memory.AIPromptState(ctx, "prod_acme", "recipe.brief"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("initial prompt state error = %v, want not found", err)
	}

	override, err := memory.SaveAIPromptState(ctx, model.AIPromptState{ProductID: "prod_acme", Key: "recipe.brief", Instructions: "Use exact evidence."}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if override.Revision != 2 || override.UpdatedAt.IsZero() {
		t.Fatalf("first persisted prompt state = %#v", override)
	}
	if _, err = memory.SaveAIPromptState(ctx, model.AIPromptState{ProductID: "prod_acme", Key: "recipe.brief", Instructions: "Stale edit."}, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale prompt state save error = %v, want conflict", err)
	}

	reset, err := memory.SaveAIPromptState(ctx, model.AIPromptState{ProductID: "prod_acme", Key: "recipe.brief"}, override.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Revision != 3 || reset.Instructions != "" {
		t.Fatalf("reset prompt state = %#v", reset)
	}
	if _, err = memory.SaveAIPromptState(ctx, model.AIPromptState{ProductID: "prod_acme", Key: "recipe.brief", Instructions: "ABA edit."}, override.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("pre-reset revision became valid again: %v", err)
	}
}

func TestMemoryAIPromptStateAndAuditAreAtomic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	memory := NewMemory()
	state := model.AIPromptState{ProductID: "prod_acme", Key: "recipe.authoring", Instructions: "Use selected evidence only."}
	invalidAudit := model.AuditEvent{
		ID:             "audit_invalid",
		OrganisationID: "org_acme",
		ProductID:      "prod_acme",
		Current:        map[string]any{"invalid": make(chan struct{})},
	}
	if _, err := memory.SaveAIPromptStateAndAudit(ctx, state, 1, invalidAudit); err == nil {
		t.Fatal("invalid audit unexpectedly succeeded")
	}
	if _, err := memory.AIPromptState(ctx, state.ProductID, state.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prompt state persisted without audit: %v", err)
	}

	audit := model.AuditEvent{
		ID:             "audit_prompt_saved",
		OrganisationID: "org_acme",
		ProductID:      "prod_acme",
		Action:         "ai.prompt.saved",
		TargetType:     "ai_prompt",
		TargetID:       state.Key,
	}
	updated, err := memory.SaveAIPromptStateAndAudit(ctx, state, 1, audit)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision = %d, want 2", updated.Revision)
	}
	events, err := memory.AuditEvents(ctx, "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != audit.ID {
		t.Fatalf("audit events = %#v", events)
	}
}
