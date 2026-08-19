package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestPrivateDefaultsAndGuardedPublication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)

	product, err := memory.Product(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	if product.PublicMCPEnabled {
		t.Fatal("Public MCP must default to off")
	}

	source, err := memory.Source(ctx, product.ID, "src_docs")
	if err != nil {
		t.Fatal(err)
	}
	if source.Visibility != model.VisibilityPrivate {
		t.Fatalf("new source visibility = %q, want private", source.Visibility)
	}

	_, err = service.SetSourceVisibility(ctx, product.ID, source.ID, model.VisibilityPublic, false, source.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed publication error = %v", err)
	}

	source, err = service.SetSourceVisibility(ctx, product.ID, source.ID, model.VisibilityPublic, true, source.Revision, platform.Actor{ID: "root", RequestID: "req_1"})
	if err != nil {
		t.Fatal(err)
	}
	if source.Visibility != model.VisibilityPublic {
		t.Fatalf("visibility = %q", source.Visibility)
	}

	events, err := memory.AuditEvents(ctx, product.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "source.visibility.changed" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestPackageCredentialIsEncryptedAndExcludedFromAPIRepresentation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	pkg, err := service.CreatePackage(ctx, platform.PackageInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "@acme/private", Ecosystem: "npm", Version: "1.0.0", Mode: "proxy", UpstreamURL: "https://registry.example.com/acme.tgz", Credential: "upstream-secret-token"}, platform.Actor{ID: "root", RequestID: "req_package"})
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Visibility != model.VisibilityPrivate || pkg.Published {
		t.Fatalf("unsafe defaults: %#v", pkg)
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("upstream-secret-token")) || bytes.Contains(encoded, []byte("registry.example.com")) || bytes.Contains(encoded, []byte(pkg.CredentialID)) {
		t.Fatalf("internal delivery data leaked in JSON: %s", encoded)
	}
	stored, err := memory.Secret(ctx, "org_acme", pkg.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored.Ciphertext, []byte("upstream-secret-token")) {
		t.Fatal("credential was stored in plaintext")
	}
}

func TestPublicMCPRequiresConfirmationAndPrivateTransitionDoesNot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	product, _ := memory.Product(ctx, "prod_acme")

	_, err := service.SetPublicMCP(ctx, product.ID, true, false, product.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed enable error = %v", err)
	}

	product, err = service.SetPublicMCP(ctx, product.ID, true, true, product.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if !product.PublicMCPEnabled {
		t.Fatal("Public MCP was not enabled")
	}

	product, err = service.SetPublicMCP(ctx, product.ID, false, false, product.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if product.PublicMCPEnabled {
		t.Fatal("Public MCP was not disabled")
	}
}

func TestCredentialBackedPackagePublicationIsStillGuarded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	pkg, _ := memory.Package(ctx, "prod_acme", "pkg_node")

	_, err := service.SetPackageVisibility(ctx, pkg.ProductID, pkg.ID, model.VisibilityPublic, false, pkg.Revision, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed proxy package publication error = %v", err)
	}
	updated, err := service.SetPackageVisibility(ctx, pkg.ProductID, pkg.ID, model.VisibilityPublic, true, pkg.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Visibility != model.VisibilityPublic {
		t.Fatalf("visibility = %q", updated.Visibility)
	}
}
