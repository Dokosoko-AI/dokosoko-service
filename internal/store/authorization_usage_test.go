package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryAuthorizationUsageLeaseLifecycle(t *testing.T) {
	memory := NewMemory()
	now := time.Now().UTC()
	created, err := memory.CreateAuthorizationUsageEvent(context.Background(), model.AuthorizationUsageEvent{ID: "event-1", OrganisationID: "org_acme", ProductID: "prod_acme", IntegrationID: "integration-1", AuthorizationID: "authorization-1", URL: "https://hooks.example.test/usage", Payload: json.RawMessage(`{"event_id":"event-1"}`), AvailableAt: now})
	if err != nil || created.State != "queued" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	claimed, err := memory.ClaimAuthorizationUsageEvents(context.Background(), "worker-1", now.Add(-time.Minute), 10)
	if err != nil || len(claimed) != 1 || claimed[0].State != "delivering" || claimed[0].Attempts != 1 {
		t.Fatalf("claimed=%#v err=%v", claimed, err)
	}
	reclaimed, err := memory.ClaimAuthorizationUsageEvents(context.Background(), "worker-2", now.Add(time.Minute), 10)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].LeaseOwner != "worker-2" || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed abandoned delivery=%#v err=%v", reclaimed, err)
	}
	if err := memory.RetryAuthorizationUsageEvent(context.Background(), created.ID, "worker-2", now.Add(time.Minute), "transport failed"); err != nil {
		t.Fatal(err)
	}
	claimed, err = memory.ClaimAuthorizationUsageEvents(context.Background(), "worker-3", now.Add(2*time.Minute), 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("early retry claimed=%#v err=%v", claimed, err)
	}
}
