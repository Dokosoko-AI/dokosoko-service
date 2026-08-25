package store_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestMemoryToolTestRetentionCleanupIsBoundedAndPreservesLiveData(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	tool, err := memory.CreateTool(ctx, model.Tool{ID: "tool-retention", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "retention", Name: "test", BackendKind: "http"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	argumentHash := bytes.Repeat([]byte{0x44}, 32)
	for index := 0; index < 2; index++ {
		if err := memory.CreateToolTestConfirmation(ctx, model.ToolTestConfirmation{
			ID: "expired-confirmation-" + string(rune('a'+index)), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
			ArgumentHash: argumentHash, NonceDigest: bytes.Repeat([]byte{byte(index + 1)}, 32), ActorID: "root", ExpiresAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := memory.AppendToolTestRun(ctx, model.ToolTestRun{
			ID: "expired-run-" + string(rune('a'+index)), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
			ArgumentHash: argumentHash, ExpiresAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	liveDigest := bytes.Repeat([]byte{0x7f}, 32)
	if err := memory.CreateToolTestConfirmation(ctx, model.ToolTestConfirmation{
		ID: "live-confirmation", OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
		ArgumentHash: argumentHash, NonceDigest: liveDigest, ActorID: "root", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendToolTestRun(ctx, model.ToolTestRun{
		ID: "live-run", OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
		ArgumentHash: argumentHash, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for cleanup := 0; cleanup < 2; cleanup++ {
		deleted, err := memory.DeleteExpiredToolTestData(ctx, now, 1)
		if err != nil || deleted != 2 {
			t.Fatalf("cleanup %d deleted=%d err=%v", cleanup, deleted, err)
		}
	}
	if deleted, err := memory.DeleteExpiredToolTestData(ctx, now, 1); err != nil || deleted != 0 {
		t.Fatalf("final cleanup deleted=%d err=%v", deleted, err)
	}
	if _, err := memory.ConsumeToolTestConfirmation(ctx, liveDigest, tool.ProductID, tool.ID, tool.Revision, argumentHash, "root", "consume-live", now); err != nil {
		t.Fatalf("live confirmation was removed: %v", err)
	}
	if value, err := memory.ToolTestRun(ctx, tool.ProductID, tool.ID, "live-run", now); err != nil || value.ID != "live-run" {
		t.Fatalf("live run=%#v err=%v", value, err)
	}
}
