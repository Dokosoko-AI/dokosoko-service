package store

import (
	"context"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryAuditAppendIsIdempotent(t *testing.T) {
	memory := NewMemory()
	event := model.AuditEvent{ID: "audit-stable", OrganisationID: "org_acme", ActorID: "root", Action: "test", TargetType: "test", TargetID: "target", RequestID: "request", CreatedAt: time.Now().UTC()}
	if err := memory.AppendAudit(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendAudit(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	events, err := memory.AuditEvents(context.Background(), event.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events=%d, want 1", len(events))
	}
}

func TestAuditAppendRequiresIdempotencyKey(t *testing.T) {
	if err := NewMemory().AppendAudit(context.Background(), model.AuditEvent{}); err == nil {
		t.Fatal("empty audit event ID was accepted")
	}
}
