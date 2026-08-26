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
	beforeDrift, err := memory.Deployment(ctx)
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
	afterDrift, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterDrift.CatalogRevision != beforeDrift.CatalogRevision+1 {
		t.Fatalf("drift catalog revision = %d, want %d", afterDrift.CatalogRevision, beforeDrift.CatalogRevision+1)
	}
	afterDriftProduct, err := memory.Product(ctx, published.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if afterDriftProduct.CatalogRevision != afterDrift.CatalogRevision {
		t.Fatalf("product catalog revision = %d, deployment = %d", afterDriftProduct.CatalogRevision, afterDrift.CatalogRevision)
	}
	if _, err := memory.MarkImportedToolDrift(ctx, published.ProductID, published.ID, true); err != nil {
		t.Fatal(err)
	}
	afterNoop, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterNoop.CatalogRevision != afterDrift.CatalogRevision {
		t.Fatalf("no-op drift changed catalog revision to %d", afterNoop.CatalogRevision)
	}
	recovered, err := memory.MarkImportedToolDrift(ctx, published.ProductID, published.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.UpstreamDrifted || recovered.Revision != published.Revision {
		t.Fatalf("recovered tool = %#v, published revision = %d", recovered, published.Revision)
	}
	afterRecovery, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterRecovery.CatalogRevision != afterDrift.CatalogRevision+1 {
		t.Fatalf("recovery catalog revision = %d, want %d", afterRecovery.CatalogRevision, afterDrift.CatalogRevision+1)
	}
}

func TestImportedToolDriftBumpsOnlyAVisibleCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	product, err := memory.CreateProduct(ctx, model.Product{ID: "prod_standalone_drift", OrganisationID: "org_acme", Name: "Standalone drift", Slug: "standalone-drift"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_standalone_drift", OrganisationID: product.OrganisationID, ProductID: product.ID, Namespace: "support", Name: "incident_create", BackendKind: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.MarkImportedToolDrift(ctx, product.ID, draft.ID, true); err != nil {
		t.Fatal(err)
	}
	afterDraft, err := memory.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterDraft.CatalogRevision != product.CatalogRevision {
		t.Fatalf("draft drift changed catalog revision to %d", afterDraft.CatalogRevision)
	}
	draft, err = memory.MarkImportedToolDrift(ctx, product.ID, draft.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	published, err := memory.PublishTool(ctx, product.ID, draft.ID, draft.Revision, "root")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.MarkImportedToolDrift(ctx, product.ID, published.ID, true); err != nil {
		t.Fatal(err)
	}
	afterPublished, err := memory.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterPublished.CatalogRevision != product.CatalogRevision+1 {
		t.Fatalf("published drift catalog revision = %d, want %d", afterPublished.CatalogRevision, product.CatalogRevision+1)
	}
}
