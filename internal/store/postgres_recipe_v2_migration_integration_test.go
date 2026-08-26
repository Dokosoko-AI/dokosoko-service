package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRecipeV2MigrationPreservesAndWithdrawsLegacyRevisions(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Skip("DOKOSOKO_TEST_DATABASE_URL or TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	t.Cleanup(admin.Close)

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("recipe_v2_migration_%x", random)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Skipf("cannot create isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
	})

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 16)); err != nil {
		t.Fatalf("migrate through 0016: %v", err)
	}
	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO organisations(id,name,slug) VALUES ($1,'Recipe v2','recipe-v2')`, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products(id,organisation_id,name,slug,description) VALUES ($1,$2,'Recipe v2','recipe-v2','Migration fixture')`, productID, organisationID); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 55)); err != nil {
		t.Fatalf("migrate through 0055: %v", err)
	}

	integrationID, integrationRevisionID := storeTestUUID(t), storeTestUUID(t)
	manifestHash := "sha256:" + strings.Repeat("a", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO integrations(id,deployment_id,organisation_id,family_key,version_key,display_name,lifecycle) VALUES($1,$2,$3,'payments','v1','Payments','active')`, integrationID, productID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integration_revisions(id,integration_id,revision,state,snapshot,manifest_hash,published_at) VALUES($1,$2,1,'published','{"family_key":"payments"}'::jsonb,$3,now())`, integrationRevisionID, integrationID, manifestHash); err != nil {
		t.Fatal(err)
	}

	analysisID, legacyRecipeID, legacyRevisionID := storeTestUUID(t), storeTestUUID(t), storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO integration_analyses(id,organisation_id,product_id,state,generated_by) VALUES($1,$2,$3,'review','deterministic')`, analysisID, organisationID, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO recipes(id,organisation_id,product_id,analysis_id,slug,title,outcome,audience,state,generated,needs_attention,visibility,stable_uri,approved_by,approved_at,published_at,revision) VALUES($1,$2,$3,$4,'connect-via-mcp','Connect via MCP','Connect the product through MCP.','developer','published',true,false,'private','dokosoko://products/recipe-v2/recipes/connect-via-mcp','legacy-reviewer',now(),now(),7)`, legacyRecipeID, organisationID, productID, analysisID); err != nil {
		t.Fatal(err)
	}
	legacyMarkdown := "# Connect via MCP\n\nHistorical content must remain unchanged.\n"
	if _, err := pool.Exec(ctx, `INSERT INTO recipe_revisions(id,recipe_id,revision,markdown,reference_items,validation,review,generated_by,model,created_by) VALUES($1,$2,1,$3,'[{"label":"Historical","url":"https://docs.example.test","kind":"documentation"}]'::jsonb,'[]'::jsonb,'legacy review','ai','legacy-model','legacy-author')`, legacyRevisionID, legacyRecipeID, legacyMarkdown); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE recipes SET current_revision_id=$2 WHERE id=$1`, legacyRecipeID, legacyRevisionID); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 56)); err != nil {
		t.Fatalf("migrate through 0056: %v", err)
	}
	var (
		contractVersion, state, approvedBy, currentRevisionID, integrationBinding string
		needsAttention                                                            bool
		approvedAt, publishedAt                                                   *time.Time
		recipeRevision                                                            int64
	)
	if err := pool.QueryRow(ctx, `SELECT contract_version,state,needs_attention,approved_by,approved_at,published_at,revision,current_revision_id::text,coalesce(integration_id::text,'') FROM recipes WHERE id=$1`, legacyRecipeID).Scan(&contractVersion, &state, &needsAttention, &approvedBy, &approvedAt, &publishedAt, &recipeRevision, &currentRevisionID, &integrationBinding); err != nil {
		t.Fatal(err)
	}
	if contractVersion != model.RecipeContractLegacyMCPV1 || state != "outdated" || !needsAttention || approvedBy != "" || approvedAt != nil || publishedAt != nil || recipeRevision != 8 || currentRevisionID != legacyRevisionID || integrationBinding != "" {
		t.Fatalf("withdrawn legacy recipe = contract %q state %q attention %t approved %q/%v published %v revision %d current %q integration %q", contractVersion, state, needsAttention, approvedBy, approvedAt, publishedAt, recipeRevision, currentRevisionID, integrationBinding)
	}
	var (
		storedMarkdown, storedReview, storedModel, storedAuthor, storedManifestHash string
		storedSpec                                                                  []byte
		storedSpecVersion, historicalCount                                          int
		storedIntegrationRevisionID                                                 *string
	)
	if err := pool.QueryRow(ctx, `SELECT markdown,review,model,created_by,spec_version,spec,coalesce(integration_manifest_hash,''),integration_revision_id::text FROM recipe_revisions WHERE id=$1`, legacyRevisionID).Scan(&storedMarkdown, &storedReview, &storedModel, &storedAuthor, &storedSpecVersion, &storedSpec, &storedManifestHash, &storedIntegrationRevisionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recipe_revisions WHERE recipe_id=$1`, legacyRecipeID).Scan(&historicalCount); err != nil {
		t.Fatal(err)
	}
	if historicalCount != 1 || storedMarkdown != legacyMarkdown || storedReview != "legacy review" || storedModel != "legacy-model" || storedAuthor != "legacy-author" || storedSpecVersion != 1 || string(storedSpec) != `{}` || storedManifestHash != "" || storedIntegrationRevisionID != nil {
		t.Fatalf("legacy revision changed: count=%d markdown=%q review=%q model=%q author=%q spec_version=%d spec=%s manifest=%q integration_revision=%v", historicalCount, storedMarkdown, storedReview, storedModel, storedAuthor, storedSpecVersion, storedSpec, storedManifestHash, storedIntegrationRevisionID)
	}

	postgres := NewPostgres(pool, "https://dokosoko.example")
	v2RecipeID, v2RevisionID := storeTestUUID(t), storeTestUUID(t)
	v2RecipeInput := model.Recipe{
		ID:              v2RecipeID,
		OrganisationID:  organisationID,
		ProductID:       productID,
		IntegrationID:   integrationID,
		ContractVersion: model.RecipeContractProductIntegrationV2,
		Slug:            "create-payment",
		Title:           "Create a payment",
		Outcome:         "The application creates one payment.",
		Audience:        "coding agent",
		State:           "draft",
		NeedsAttention:  true,
		Visibility:      model.VisibilityPrivate,
		StableURI:       "dokosoko://products/recipe-v2/recipes/create-payment",
	}
	spec, err := json.Marshal(model.RecipeSpec{SchemaVersion: model.RecipeSpecVersion2, IntegrationID: integrationID, Title: v2RecipeInput.Title, Outcome: v2RecipeInput.Outcome, CapabilityIDs: []string{"payments.create"}, Steps: []model.RecipeInstruction{{Action: "Call the create-payment operation."}}})
	if err != nil {
		t.Fatal(err)
	}
	promptHash := "sha256:" + strings.Repeat("b", 64)
	v2RevisionInput := model.RecipeRevision{
		ID:                      v2RevisionID,
		RecipeID:                v2RecipeInput.ID,
		SpecVersion:             model.RecipeSpecVersion2,
		Spec:                    spec,
		Markdown:                "# Create a payment\n",
		GeneratedBy:             "ai",
		Model:                   "authoring-model",
		IntegrationRevisionID:   integrationRevisionID,
		IntegrationManifestHash: manifestHash,
		PromptVersion:           "recipe-authoring-v9",
		PromptHash:              promptHash,
		CreatedBy:               "recipe-author",
	}
	v2Recipe, err := postgres.CreateRecipeWithRevision(ctx, v2RecipeInput, v2RevisionInput)
	if err != nil {
		t.Fatal(err)
	}
	if v2Recipe.Revision != 1 || v2Recipe.CurrentRevisionID != v2RevisionID || v2Recipe.CurrentRevision == nil || v2Recipe.CurrentRevision.Revision != 1 || v2Recipe.CurrentRevision.SpecVersion != model.RecipeSpecVersion2 || v2Recipe.CurrentRevision.IntegrationRevisionID != integrationRevisionID || v2Recipe.CurrentRevision.IntegrationManifestHash != manifestHash || v2Recipe.CurrentRevision.PromptVersion != "recipe-authoring-v9" || v2Recipe.CurrentRevision.PromptHash != promptHash || !json.Valid(v2Recipe.CurrentRevision.Spec) {
		t.Fatalf("v2 recipe revision did not round-trip: %#v", v2Recipe.CurrentRevision)
	}
	roundTrip, err := postgres.Recipe(ctx, productID, v2RecipeID)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.CurrentRevision == nil || roundTrip.CurrentRevision.ID != v2RevisionID {
		t.Fatalf("created recipe was not hydrated: %#v", roundTrip)
	}

	rolledBackRecipe := v2RecipeInput
	rolledBackRecipe.ID = storeTestUUID(t)
	rolledBackRecipe.Slug = "create-payment-rolled-back"
	rolledBackRecipe.StableURI = "dokosoko://products/recipe-v2/recipes/create-payment-rolled-back"
	rolledBackRevision := v2RevisionInput
	rolledBackRevision.RecipeID = rolledBackRecipe.ID
	if _, err := postgres.CreateRecipeWithRevision(ctx, rolledBackRecipe, rolledBackRevision); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate immutable revision ID error = %v, want conflict", err)
	}
	if _, err := postgres.Recipe(ctx, productID, rolledBackRecipe.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed transaction left a recipe behind: %v", err)
	}

	var deploymentCatalogBefore, productCatalogBefore int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1`, productID).Scan(&deploymentCatalogBefore); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1`, productID).Scan(&productCatalogBefore); err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC()
	v2Recipe.State = "published"
	v2Recipe.PublishedAt = &publishedAt
	transitionAudit := model.AuditEvent{
		ID:             "audit:" + storeTestUUID(t),
		OrganisationID: organisationID,
		ProductID:      productID,
		ActorID:        "recipe-reviewer",
		Action:         "recipe.published",
		TargetType:     "recipe",
		TargetID:       v2Recipe.ID,
		Prior:          map[string]any{"state": "draft"},
		Current:        map[string]any{"state": "published"},
		RequestID:      "recipe-transition-test",
		CreatedAt:      publishedAt,
	}
	published, err := postgres.SaveRecipeTransition(ctx, v2Recipe, v2Recipe.Revision, true, &transitionAudit)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "published" || published.Revision != v2Recipe.Revision+1 || published.CurrentRevision == nil || published.CurrentRevision.ID != v2RevisionID {
		t.Fatalf("published recipe aggregate = %#v", published)
	}
	var deploymentCatalogAfter, productCatalogAfter int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1`, productID).Scan(&deploymentCatalogAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1`, productID).Scan(&productCatalogAfter); err != nil {
		t.Fatal(err)
	}
	if deploymentCatalogAfter != deploymentCatalogBefore+1 || productCatalogAfter != deploymentCatalogAfter || productCatalogAfter != productCatalogBefore+1 {
		t.Fatalf("catalog revisions = deployment %d->%d product %d->%d", deploymentCatalogBefore, deploymentCatalogAfter, productCatalogBefore, productCatalogAfter)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_key=$1`, transitionAudit.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("transition audit count = %d, want 1", auditCount)
	}

	failedTransition := published
	failedTransition.State = "outdated"
	staleAudit := transitionAudit
	staleAudit.ID = "audit:" + storeTestUUID(t)
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, v2Recipe.Revision, true, &staleAudit); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition error = %v, want conflict", err)
	}
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, published.Revision, true, &transitionAudit); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate-audit transition error = %v, want conflict", err)
	}
	invalidAudit := staleAudit
	invalidAudit.ID = "audit:" + storeTestUUID(t)
	invalidAudit.Current = map[string]any{"invalid": make(chan int)}
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, published.Revision, true, &invalidAudit); err == nil {
		t.Fatal("transition accepted a non-JSON audit")
	}
	unchanged, err := postgres.Recipe(ctx, productID, published.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != published.Revision || unchanged.State != published.State {
		t.Fatalf("failed transition changed recipe: got %#v want %#v", unchanged, published)
	}
	var deploymentCatalogRolledBack, productCatalogRolledBack, staleAuditCount int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1`, productID).Scan(&deploymentCatalogRolledBack); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1`, productID).Scan(&productCatalogRolledBack); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_key=$1`, staleAudit.ID).Scan(&staleAuditCount); err != nil {
		t.Fatal(err)
	}
	if deploymentCatalogRolledBack != deploymentCatalogAfter || productCatalogRolledBack != productCatalogAfter || staleAuditCount != 0 {
		t.Fatalf("failed transition leaked state: deployment=%d product=%d stale_audits=%d", deploymentCatalogRolledBack, productCatalogRolledBack, staleAuditCount)
	}
}
