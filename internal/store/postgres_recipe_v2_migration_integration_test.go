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

	backfillRecipeID, backfillRevisionID := storeTestUUID(t), storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO recipes(id,organisation_id,product_id,integration_id,contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,stable_uri) VALUES($1,$2,$3,$4,'product-integration-v2','backfill-payment','Backfill a payment','The application backfills one payment.','coding_agent','review',true,true,'private','dokosoko://products/recipe-v2/recipes/backfill-payment')`, backfillRecipeID, organisationID, productID, integrationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO recipe_revisions(id,recipe_id,revision,spec_version,spec,markdown,reference_items,validation,generated_by,integration_revision_id,integration_manifest_hash,created_by) VALUES($1,$2,1,2,$3::jsonb,'# Backfill a payment','[]'::jsonb,'[]'::jsonb,'deterministic',$4,$5,'migration-fixture')`, backfillRevisionID, backfillRecipeID, `{"schema_version":2,"integration_id":"`+integrationID+`","title":"Backfill a payment","outcome":"The application backfills one payment.","capability_ids":["payments.backfill"],"prerequisites":[],"steps":[],"checks":[]}`, integrationRevisionID, manifestHash); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE recipes SET current_revision_id=$2 WHERE id=$1`, backfillRecipeID, backfillRevisionID); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 65)); err != nil {
		t.Fatalf("migrate through 0065: %v", err)
	}
	var backfilledAttachmentCount, backfilledBindingCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recipe_api_attachments WHERE recipe_id=$1 AND deployment_id=$2 AND integration_id=$3`, backfillRecipeID, productID, integrationID).Scan(&backfilledAttachmentCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recipe_revision_api_bindings WHERE recipe_revision_id=$1 AND recipe_id=$2 AND deployment_id=$3 AND integration_id=$4 AND integration_revision_id=$5 AND integration_manifest_hash=$6`, backfillRevisionID, backfillRecipeID, productID, integrationID, integrationRevisionID, manifestHash).Scan(&backfilledBindingCount); err != nil {
		t.Fatal(err)
	}
	if backfilledAttachmentCount != 1 || backfilledBindingCount != 1 {
		t.Fatalf("v2 recipe projection backfill = attachments %d bindings %d", backfilledAttachmentCount, backfilledBindingCount)
	}

	postgres := NewPostgres(pool, "https://dokosoko.example")
	v2RecipeID, v2RevisionID := storeTestUUID(t), storeTestUUID(t)
	v2ApprovedAt := time.Now().UTC()
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
		State:           "approved",
		ApprovedBy:      "recipe-reviewer",
		ApprovedAt:      &v2ApprovedAt,
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
	var catalogAtCreate int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1`, productID).Scan(&catalogAtCreate); err != nil {
		t.Fatal(err)
	}
	createAudit := recipeTestAudit(v2RecipeInput, "audit:"+storeTestUUID(t), "recipe.created")
	v2Recipe, err := postgres.CreateRecipeWithRevision(ctx, v2RecipeInput, v2RevisionInput, RecipeMutation{ExpectedCatalogRevision: catalogAtCreate, Audit: &createAudit})
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
	secondIntegrationID, secondIntegrationRevisionID := storeTestUUID(t), storeTestUUID(t)
	secondManifestHash := "sha256:" + strings.Repeat("c", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO integrations(id,deployment_id,organisation_id,family_key,version_key,display_name,lifecycle) VALUES($1,$2,$3,'customers','v1','Customers','active')`, secondIntegrationID, productID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integration_revisions(id,integration_id,revision,state,snapshot,manifest_hash,published_at) VALUES($1,$2,1,'published','{"family_key":"customers"}'::jsonb,$3,now())`, secondIntegrationRevisionID, secondIntegrationID, secondManifestHash); err != nil {
		t.Fatal(err)
	}
	v3Recipe := model.Recipe{
		ID:              storeTestUUID(t),
		OrganisationID:  organisationID,
		ProductID:       productID,
		ContractVersion: model.RecipeContractDeploymentV3,
		APIAttachments: []model.RecipeAPIAttachment{
			{IntegrationID: secondIntegrationID},
			{IntegrationID: integrationID},
		},
		Slug:           "create-customer-payment",
		Title:          "Create a customer payment",
		Outcome:        "The application creates a customer and payment.",
		Audience:       "coding_agent",
		State:          "review",
		Generated:      true,
		NeedsAttention: true,
		Visibility:     model.VisibilityPrivate,
		StableURI:      "dokosoko://products/recipe-v2/recipes/create-customer-payment",
	}
	v3Spec, err := json.Marshal(model.RecipeSpec{
		SchemaVersion:  model.RecipeSpecVersion3,
		APIAttachments: append([]model.RecipeAPIAttachment(nil), v3Recipe.APIAttachments...),
		Title:          v3Recipe.Title,
		Outcome:        v3Recipe.Outcome,
		CapabilityIDs:  []string{"customers.create", "payments.create"},
		Steps:          []model.RecipeInstruction{{Action: "Create the customer."}, {Action: "Create the payment."}},
		Checks:         []model.RecipeInstruction{{Action: "Verify both resources."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v3Revision := model.RecipeRevision{
		ID:          storeTestUUID(t),
		RecipeID:    v3Recipe.ID,
		SpecVersion: model.RecipeSpecVersion3,
		Spec:        v3Spec,
		Markdown:    "# Create a customer payment\n",
		GeneratedBy: "human",
		CreatedBy:   "recipe-author",
		APIBindings: []model.RecipeAPIBinding{
			{IntegrationID: secondIntegrationID, IntegrationRevisionID: secondIntegrationRevisionID, IntegrationManifestHash: secondManifestHash},
			{IntegrationID: integrationID, IntegrationRevisionID: integrationRevisionID, IntegrationManifestHash: manifestHash},
		},
	}
	v3Audit := recipeTestAudit(v3Recipe, "audit:"+storeTestUUID(t), "recipe.created")
	v3Saved, err := postgres.CreateRecipeWithRevision(ctx, v3Recipe, v3Revision, RecipeMutation{ExpectedCatalogRevision: catalogAtCreate, Audit: &v3Audit})
	if err != nil {
		t.Fatal(err)
	}
	if v3Saved.IntegrationID != "" || len(v3Saved.APIAttachments) != 2 || v3Saved.CurrentRevision == nil || len(v3Saved.CurrentRevision.APIBindings) != 2 {
		t.Fatalf("v3 recipe did not return its multi-API projections: %#v", v3Saved)
	}
	v3RoundTrip, err := postgres.Recipe(ctx, productID, v3Recipe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v3RoundTrip.APIAttachments) != 2 || v3RoundTrip.CurrentRevision == nil || len(v3RoundTrip.CurrentRevision.APIBindings) != 2 {
		t.Fatalf("v3 recipe projections did not round-trip: %#v", v3RoundTrip)
	}
	product, err := postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	product.Description = "Updated recipe discovery description."
	product, err = postgres.UpdateProduct(ctx, product, product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var mirroredDeploymentRevision, mirroredDeploymentCatalog int64
	var mirroredDeploymentDescription string
	if err := pool.QueryRow(ctx, `SELECT revision,catalog_revision,description FROM deployments WHERE id=$1`, productID).Scan(&mirroredDeploymentRevision, &mirroredDeploymentCatalog, &mirroredDeploymentDescription); err != nil {
		t.Fatal(err)
	}
	if mirroredDeploymentRevision != product.Revision || mirroredDeploymentCatalog != product.CatalogRevision || mirroredDeploymentDescription != product.Description {
		t.Fatalf("PostgreSQL deployment/product mirror diverged: deployment revision=%d catalog=%d description=%q product=%#v", mirroredDeploymentRevision, mirroredDeploymentCatalog, mirroredDeploymentDescription, product)
	}

	rolledBackRecipe := v2RecipeInput
	rolledBackRecipe.ID = storeTestUUID(t)
	rolledBackRecipe.Slug = "create-payment-rolled-back"
	rolledBackRecipe.StableURI = "dokosoko://products/recipe-v2/recipes/create-payment-rolled-back"
	rolledBackRevision := v2RevisionInput
	rolledBackRevision.RecipeID = rolledBackRecipe.ID
	rolledBackAudit := recipeTestAudit(rolledBackRecipe, "audit:"+storeTestUUID(t), "recipe.created")
	if _, err := postgres.CreateRecipeWithRevision(ctx, rolledBackRecipe, rolledBackRevision, RecipeMutation{ExpectedCatalogRevision: product.CatalogRevision, Audit: &rolledBackAudit}); !errors.Is(err, ErrConflict) {
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
	transitionAt := time.Now().UTC()
	v2Recipe.State = "published"
	v2Recipe.PublishedAt = &transitionAt
	transitionAudit := model.AuditEvent{
		ID:             "audit:" + storeTestUUID(t),
		OrganisationID: organisationID,
		ProductID:      productID,
		ActorID:        "recipe-reviewer",
		Action:         "recipe.published",
		TargetType:     "recipe",
		TargetID:       v2Recipe.ID,
		Prior:          map[string]any{"state": "approved"},
		Current:        map[string]any{"state": "published"},
		RequestID:      "recipe-transition-test",
		CreatedAt:      transitionAt,
	}
	published, err := postgres.SaveRecipeTransition(ctx, v2Recipe, RecipeMutation{ExpectedRevision: v2Recipe.Revision, ExpectedCatalogRevision: productCatalogBefore, Audit: &transitionAudit})
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
	staleAudit.Action = "recipe.outdated"
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, RecipeMutation{ExpectedRevision: v2Recipe.Revision, ExpectedCatalogRevision: productCatalogAfter, Audit: &staleAudit}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition error = %v, want conflict", err)
	}
	duplicateAudit := transitionAudit
	duplicateAudit.Action = "recipe.outdated"
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, RecipeMutation{ExpectedRevision: published.Revision, ExpectedCatalogRevision: productCatalogAfter, Audit: &duplicateAudit}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate-audit transition error = %v, want conflict", err)
	}
	invalidAudit := staleAudit
	invalidAudit.ID = "audit:" + storeTestUUID(t)
	invalidAudit.Current = map[string]any{"invalid": make(chan int)}
	if _, err := postgres.SaveRecipeTransition(ctx, failedTransition, RecipeMutation{ExpectedRevision: published.Revision, ExpectedCatalogRevision: productCatalogAfter, Audit: &invalidAudit}); err == nil {
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

	review := published
	review.State = "review"
	review.NeedsAttention = true
	review.ApprovedBy = ""
	review.ApprovedAt = nil
	review.PublishedAt = nil
	reviewRevision := v2RevisionInput
	reviewRevision.ID = storeTestUUID(t)
	reviewRevision.Revision = 0
	reviewRevision.Markdown = "# Create a payment\n\nRevised.\n"
	revisionAudit := recipeTestAudit(review, "audit:"+storeTestUUID(t), "recipe.reworked")
	revised, err := postgres.SaveRecipeRevision(ctx, review, reviewRevision, RecipeMutation{ExpectedRevision: published.Revision, ExpectedCatalogRevision: productCatalogAfter, Audit: &revisionAudit})
	if err != nil {
		t.Fatal(err)
	}
	if revised.State != "review" || revised.CurrentRevision == nil || revised.CurrentRevision.ID != reviewRevision.ID {
		t.Fatalf("revised recipe aggregate = %#v", revised)
	}
	var deploymentCatalogAfterRevision, productCatalogAfterRevision int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1`, productID).Scan(&deploymentCatalogAfterRevision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM products WHERE id=$1`, productID).Scan(&productCatalogAfterRevision); err != nil {
		t.Fatal(err)
	}
	if deploymentCatalogAfterRevision != deploymentCatalogAfter+1 || productCatalogAfterRevision != deploymentCatalogAfterRevision {
		t.Fatalf("published edit catalog revisions = deployment %d product %d", deploymentCatalogAfterRevision, productCatalogAfterRevision)
	}
	duplicateRevisionAudit := recipeTestAudit(revised, "audit:"+storeTestUUID(t), "recipe.reworked")
	if _, err := postgres.SaveRecipeRevision(ctx, revised, reviewRevision, RecipeMutation{ExpectedRevision: revised.Revision, ExpectedCatalogRevision: productCatalogAfterRevision, Audit: &duplicateRevisionAudit}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate revision error = %v, want conflict", err)
	}
	var deploymentCatalogAfterFailedRevision int64
	if err := pool.QueryRow(ctx, `SELECT catalog_revision FROM deployments WHERE id=$1`, productID).Scan(&deploymentCatalogAfterFailedRevision); err != nil {
		t.Fatal(err)
	}
	if deploymentCatalogAfterFailedRevision != deploymentCatalogAfterRevision {
		t.Fatalf("failed revision bumped catalog to %d, want %d", deploymentCatalogAfterFailedRevision, deploymentCatalogAfterRevision)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET catalog_revision=catalog_revision+5 WHERE id=$1`, productID); err != nil {
		t.Fatal(err)
	}
	productAtDelete, err := postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := postgres.Recipe(ctx, productID, legacyRecipeID)
	if err != nil {
		t.Fatal(err)
	}
	deleteAudit := recipeTestAudit(legacy, "audit:"+storeTestUUID(t), "recipe.deleted")
	deleteAudit.Prior = map[string]any{"state": legacy.State, "current_revision_id": legacy.CurrentRevisionID}
	deleteAudit.Current = map[string]any{"deleted": true}
	if err := postgres.DeleteRecipe(ctx, productID, legacyRecipeID, RecipeMutation{ExpectedRevision: legacy.Revision, ExpectedCatalogRevision: productAtDelete.CatalogRevision, Audit: &deleteAudit}); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.Recipe(ctx, productID, legacyRecipeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted PostgreSQL recipe lookup error = %v, want not found", err)
	}
	if _, err := postgres.RecipeRevisions(ctx, legacyRecipeID); err != nil {
		t.Fatalf("deleted PostgreSQL revision list error = %v", err)
	}
	var deletedRevisionCount, deleteAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recipe_revisions WHERE recipe_id=$1`, legacyRecipeID).Scan(&deletedRevisionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_key=$1 AND action='recipe.deleted'`, deleteAudit.ID).Scan(&deleteAuditCount); err != nil {
		t.Fatal(err)
	}
	if deletedRevisionCount != 0 || deleteAuditCount != 1 {
		t.Fatalf("PostgreSQL deletion cascade/audit = revisions %d audit %d", deletedRevisionCount, deleteAuditCount)
	}
}

func TestPostgresDeleteLegacyRecipeIgnoresDeploymentCatalogMirrorDrift(t *testing.T) {
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
	schema := fmt.Sprintf("recipe_delete_drift_%x", random)
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
	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 9999)); err != nil {
		t.Fatal(err)
	}

	organisationID, productID := storeTestUUID(t), storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO organisations(id,name,slug) VALUES ($1,'Recipe delete drift','recipe-delete-drift')`, organisationID); err != nil {
		t.Fatal(err)
	}
	postgres := NewPostgres(pool, "https://dokosoko.example")
	if _, err := postgres.CreateDeployment(ctx, model.Deployment{ID: productID, OrganisationID: organisationID, Name: "Recipe delete drift", Slug: "recipe-delete-drift"}); err != nil {
		t.Fatal(err)
	}

	recipeID, revisionID := storeTestUUID(t), storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO recipes(id,organisation_id,product_id,contract_version,slug,title,outcome,audience,state,generated,needs_attention,visibility,stable_uri,revision) VALUES($1,$2,$3,'legacy-mcp-v1','legacy-delete-drift','Legacy delete drift','Delete an outdated legacy recipe.','operator','outdated',true,true,'private','dokosoko://products/recipe-delete-drift/recipes/legacy-delete-drift',7)`, recipeID, organisationID, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO recipe_revisions(id,recipe_id,revision,spec_version,spec,markdown,reference_items,validation,generated_by,created_by) VALUES($1,$2,1,1,'{}'::jsonb,'# Legacy delete drift','[]'::jsonb,'[]'::jsonb,'human','root')`, revisionID, recipeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE recipes SET current_revision_id=$2 WHERE id=$1`, recipeID, revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET catalog_revision=catalog_revision+5 WHERE id=$1`, productID); err != nil {
		t.Fatal(err)
	}

	product, err := postgres.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := postgres.Recipe(ctx, productID, recipeID)
	if err != nil {
		t.Fatal(err)
	}
	audit := recipeTestAudit(recipe, "audit:"+storeTestUUID(t), "recipe.deleted")
	audit.Prior = map[string]any{"state": recipe.State, "current_revision_id": recipe.CurrentRevisionID}
	audit.Current = map[string]any{"deleted": true}
	if err := postgres.DeleteRecipe(ctx, productID, recipeID, RecipeMutation{ExpectedRevision: recipe.Revision, ExpectedCatalogRevision: product.CatalogRevision, Audit: &audit}); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.Recipe(ctx, productID, recipeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted recipe lookup error = %v, want not found", err)
	}
	var revisionCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recipe_revisions WHERE recipe_id=$1`, recipeID).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_key=$1 AND action='recipe.deleted'`, audit.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != 0 || auditCount != 1 {
		t.Fatalf("PostgreSQL deletion cascade/audit = revisions %d audit %d", revisionCount, auditCount)
	}
}
