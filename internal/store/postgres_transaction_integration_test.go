package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAuditIdempotency(t *testing.T) {
	_, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Store contract", Slug: "store-contract-" + organisationID[:8]}); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.CreateProduct(ctx, model.Product{ID: productID, OrganisationID: organisationID, Name: "Store contract", Slug: "store-contract"}); err != nil {
		t.Fatal(err)
	}
	audit := model.AuditEvent{ID: "audit:" + storeTestUUID(t), OrganisationID: organisationID, ProductID: productID, ActorID: "root-test", Action: "store.contract.checked", TargetType: "product", TargetID: productID, Prior: map[string]any{"revision": 1}, Current: map[string]any{"revision": 2}, RequestID: "postgres-contract-test", CreatedAt: time.Now().UTC()}
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
