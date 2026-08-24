package store_test

import (
	"context"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestImportedToolDriftDoesNotCreateAContractRevision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	draft, err := memory.CreateTool(ctx, model.Tool{
		ID: "tool_drift_revision", OrganisationID: "org_acme", ProductID: "prod_acme",
		Namespace: "support", Name: "incident_create", BackendKind: "mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := memory.PublishTool(ctx, draft.ProductID, draft.ID, draft.Revision, "root")
	if err != nil {
		t.Fatal(err)
	}
	drifted, err := memory.MarkImportedToolDrift(ctx, published.ProductID, published.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !drifted.UpstreamDrifted || drifted.Revision != published.Revision {
		t.Fatalf("drifted tool = %#v, published revision = %d", drifted, published.Revision)
	}
	recovered, err := memory.MarkImportedToolDrift(ctx, published.ProductID, published.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.UpstreamDrifted || recovered.Revision != published.Revision {
		t.Fatalf("recovered tool = %#v, published revision = %d", recovered, published.Revision)
	}
}
