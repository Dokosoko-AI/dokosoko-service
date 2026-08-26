package store

import (
	"context"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryDeveloperAssetMutationsKeepDeploymentAndProductCatalogRevisionsAligned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := NewMemory()
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	product, err := memory.Product(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := deployment.CatalogRevision
	if _, err := memory.SaveSDKPackage(ctx, model.SDKPackage{
		ID: "10000000-0000-4000-8000-000000000010", DeploymentID: deployment.ID,
		OrganisationID: deployment.OrganisationID, Ecosystem: "npm", CanonicalCoordinate: "@acme/sdk",
		DisplayCoordinate: "@acme/sdk", Name: "Acme SDK", Visibility: model.VisibilityPrivate, Lifecycle: "active",
	}, 0); err != nil {
		t.Fatal(err)
	}
	deployment, err = memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	product, err = memory.Product(ctx, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.CatalogRevision != before+1 || product.CatalogRevision != deployment.CatalogRevision {
		t.Fatalf("deployment catalog revision=%d product catalog revision=%d, before=%d", deployment.CatalogRevision, product.CatalogRevision, before)
	}
	product.PublicMCPEnabled = true
	if _, err := memory.UpdateProduct(ctx, product, product.Revision); err != nil {
		t.Fatalf("product update conflicted after developer-asset mutation: %v", err)
	}
}
