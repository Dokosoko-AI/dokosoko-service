package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAuditIdempotencyAndMutationTransactions(t *testing.T) {
	pool, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	organisationID := storeTestUUID(t)
	productID := storeTestUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Store contract", Slug: "store-contract-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organisations WHERE id=$1`, organisationID); err != nil {
			t.Errorf("remove PostgreSQL store contract fixture: %v", err)
		}
	})
	product, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "Store contract", Slug: "store-contract", DefaultVersionPolicy: "latest"})
	if err != nil {
		t.Fatal(err)
	}

	audit := model.AuditEvent{
		ID:             "audit:" + storeTestUUID(t),
		OrganisationID: organisationID,
		ProductID:      productID,
		ActorID:        "root-test",
		Action:         "store.contract.checked",
		TargetType:     "product",
		TargetID:       productID,
		Prior:          map[string]any{"revision": 1},
		Current:        map[string]any{"revision": 2},
		RequestID:      "postgres-contract-test",
		CreatedAt:      time.Now().UTC(),
	}
	canceledCtx, cancelAudit := context.WithCancel(ctx)
	cancelAudit()
	if err := postgres.AppendAudit(canceledCtx, audit); err != nil {
		t.Fatalf("persist audit after request cancellation: %v", err)
	}
	if err := postgres.AppendAudit(ctx, audit); err != nil {
		t.Fatalf("idempotent audit retry: %v", err)
	}
	events, err := postgres.AuditEvents(ctx, organisationID)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, event := range events {
		if event.ID == audit.ID {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("audit key %q appeared %d times", audit.ID, matches)
	}
	invalidAudit := audit
	invalidAudit.ID = "audit:" + storeTestUUID(t)
	invalidAudit.Current = map[string]any{"invalid": make(chan int)}
	if err := postgres.AppendAudit(ctx, invalidAudit); err == nil {
		t.Fatal("non-JSON audit state was accepted")
	}

	version, err := postgres.CreateProductVersion(ctx, model.ProductVersion{
		ID:                 storeTestUUID(t),
		OrganisationID:     organisationID,
		ProductID:          productID,
		Version:            "1.0.0",
		DefinitionRevision: 1,
		ManifestHash:       "sha256:" + strings.Repeat("1", 64),
		Diff:               model.ProductVersionDiff{GeneratedAt: time.Now().UTC(), Summary: "Initial release", Added: []model.ProductVersionChange{}, Removed: []model.ProductVersionChange{}, Changed: []model.ProductVersionChange{}},
		ReleaseStage:       "active",
		RolloutPercentage:  100,
		PromotionState:     "not_required",
		DriftStatus:        "healthy",
		IsLatest:           true,
		Manifest:           model.ProductDefinition{},
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err = postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if product.CatalogRevision != 2 {
		t.Fatalf("catalog revision after version create = %d, want 2", product.CatalogRevision)
	}

	version.ReleaseStage = "preview"
	if _, err := postgres.UpdateProductVersion(ctx, version, version.Revision+10); err == nil {
		t.Fatal("stale product version update was accepted")
	}
	afterConflict, err := postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConflict.CatalogRevision != product.CatalogRevision {
		t.Fatalf("failed version update changed catalog revision from %d to %d", product.CatalogRevision, afterConflict.CatalogRevision)
	}
	version, err = postgres.UpdateProductVersion(ctx, version, version.Revision)
	if err != nil {
		t.Fatal(err)
	}
	product, err = postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if product.CatalogRevision != 3 {
		t.Fatalf("catalog revision after version update = %d, want 3", product.CatalogRevision)
	}

	account, err := postgres.ResolveCustomerAccount(ctx, identity.CustomerAccount{
		ID:                  storeTestUUID(t),
		OrganisationID:      organisationID,
		ProductID:           productID,
		Issuer:              "https://issuer.example",
		ExternalID:          "customer-1",
		LastAuthenticatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	pinHistory := model.ProductVersionPinHistory{ID: storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, Scope: "customer", ScopeID: account.ID, ProductVersion: version.Version, Action: "created", ActorID: "root-test", CreatedAt: time.Now().UTC()}
	pin, err := postgres.SaveProductVersionPin(ctx, model.ProductVersionPin{ID: storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, Scope: "customer", ScopeID: account.ID, CustomerAccountID: account.ID, ProductVersionID: version.ID, Reason: "Contract test"}, 0, pinHistory)
	if err != nil {
		t.Fatal(err)
	}
	product, err = postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if product.CatalogRevision != 4 {
		t.Fatalf("catalog revision after pin create = %d, want 4", product.CatalogRevision)
	}
	if _, err := postgres.SaveProductVersionPin(ctx, pin, pin.Revision+10, model.ProductVersionPinHistory{ID: storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, Scope: pin.Scope, ScopeID: pin.ScopeID, ProductVersion: pin.ProductVersion, Action: "updated", ActorID: "root-test", CreatedAt: time.Now().UTC()}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale pin update error = %v, want conflict", err)
	}
	history, err := postgres.ProductVersionPinHistory(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("failed pin update persisted %d history rows, want 1 total", len(history))
	}
	afterConflict, err = postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConflict.CatalogRevision != product.CatalogRevision {
		t.Fatalf("failed pin update changed catalog revision from %d to %d", product.CatalogRevision, afterConflict.CatalogRevision)
	}
	deleteHistory := model.ProductVersionPinHistory{ID: storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, PinID: pin.ID, Scope: pin.Scope, ScopeID: pin.ScopeID, PriorVersion: pin.ProductVersion, Action: "deleted", ActorID: "root-test", CreatedAt: time.Now().UTC()}
	if err := postgres.DeleteProductVersionPin(ctx, productID, pin.ID, deleteHistory); err != nil {
		t.Fatal(err)
	}
	product, err = postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if product.CatalogRevision != 5 {
		t.Fatalf("catalog revision after pin delete = %d, want 5", product.CatalogRevision)
	}
	history, err = postgres.ProductVersionPinHistory(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("pin create/delete history rows = %d, want 2", len(history))
	}
}

func migratedPostgresForStoreTest(t *testing.T) (*pgxpool.Pool, *Postgres) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("DOKOSOKO_TEST_DATABASE_URL or TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	if err := Migrate(ctx, pool, filepath.Join("..", "..", "migrations")); err != nil {
		pool.Close()
		t.Fatalf("migrate PostgreSQL fixture: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, NewPostgres(pool, "https://dokosoko.example")
}

func storeTestUUID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}
