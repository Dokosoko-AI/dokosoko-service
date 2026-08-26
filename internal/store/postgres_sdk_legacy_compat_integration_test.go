package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

func postgresLegacySDKDeployment(t *testing.T, ctx context.Context) (*Postgres, string, string) {
	t.Helper()
	pool, postgres := migratedPostgresForStoreTest(t)
	var deploymentID, organisationID string
	err := pool.QueryRow(ctx, `SELECT id::text,organisation_id::text FROM deployments LIMIT 1`).Scan(&deploymentID, &organisationID)
	if err == nil {
		return postgres, deploymentID, organisationID
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	organisationID, deploymentID = storeTestUUID(t), storeTestUUID(t)
	slug := "legacy-sdk-" + strings.ReplaceAll(deploymentID, "-", "")[:12]
	if _, err := pool.Exec(ctx, `INSERT INTO organisations(id,name,slug) VALUES($1,'Legacy SDK compatibility',$2)`, organisationID, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products(id,organisation_id,name,slug,description)
		VALUES($1,$2,'Legacy SDK compatibility',$3,'PostgreSQL compatibility fixture')`, deploymentID, organisationID, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organisation_id,name,slug,description)
		VALUES($1,$2,'Legacy SDK compatibility',$3,'PostgreSQL compatibility fixture')`, deploymentID, organisationID, slug); err != nil {
		t.Fatal(err)
	}
	return postgres, deploymentID, organisationID
}

func insertPostgresLegacySDKAPI(t *testing.T, ctx context.Context, postgres *Postgres, deploymentID, organisationID, suffix string) string {
	t.Helper()
	id := storeTestUUID(t)
	key := "legacy-sdk-" + suffix + "-" + strings.ReplaceAll(id, "-", "")[:8]
	if _, err := postgres.pool.Exec(ctx, `INSERT INTO integrations(
		id,deployment_id,organisation_id,family_key,version_key,display_name,visibility,lifecycle
	) VALUES($1,$2,$3,$4,'v1',$4,'private','active')`, id, deploymentID, organisationID, key); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPostgresLegacySDKCompatibilityOwnsExactTypedRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	postgres, deploymentID, organisationID := postgresLegacySDKDeployment(t, ctx)
	apiA := insertPostgresLegacySDKAPI(t, ctx, postgres, deploymentID, organisationID, "a")
	apiB := insertPostgresLegacySDKAPI(t, ctx, postgres, deploymentID, organisationID, "b")
	apiConflict := insertPostgresLegacySDKAPI(t, ctx, postgres, deploymentID, organisationID, "conflict")
	coordinate := "legacy-sdk-" + strings.ReplaceAll(storeTestUUID(t), "-", "")[:12]
	input := model.SDKReference{
		ID: storeTestUUID(t), DeploymentID: deploymentID, OrganisationID: organisationID, IntegrationID: apiA,
		Ecosystem: "npm", Coordinate: coordinate, ExactVersion: "1.2.3",
		InstallCommand:   "npm install " + coordinate + "@1.2.3",
		DocumentationURL: "https://docs.example.test/" + coordinate + "/1.2.3",
		SourceURL:        "https://github.com/acme/" + coordinate + "/tree/v1.2.3",
		Checksum:         "sha256:" + strings.Repeat("a", 64), Visibility: model.VisibilityPrivate,
	}
	referenceA, err := postgres.SaveSDKReference(ctx, input, 0)
	if err != nil {
		t.Fatalf("save first legacy reference: %v", err)
	}
	input.ID, input.IntegrationID = storeTestUUID(t), apiB
	referenceB, err := postgres.SaveSDKReference(ctx, input, 0)
	if err != nil {
		t.Fatalf("save identical legacy reference: %v", err)
	}

	var packageID, releaseID string
	if err := postgres.pool.QueryRow(ctx, `SELECT package.id::text,release.id::text
		FROM sdk_packages package JOIN sdk_releases release ON release.sdk_package_id=package.id
		WHERE package.deployment_id=$1 AND package.ecosystem='npm' AND package.canonical_coordinate=$2`,
		deploymentID, coordinate).Scan(&packageID, &releaseID); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []model.SDKReference{referenceA, referenceB} {
		var bindingReleaseID string
		if err := postgres.pool.QueryRow(ctx, `SELECT sdk_release_id::text FROM api_sdk_bindings WHERE id=$1`, reference.ID).Scan(&bindingReleaseID); err != nil {
			t.Fatalf("binding %s missing: %v", reference.ID, err)
		}
		if bindingReleaseID != releaseID {
			t.Fatalf("binding %s release=%s, want shared exact release %s", reference.ID, bindingReleaseID, releaseID)
		}
	}

	conflicting := input
	conflicting.ID, conflicting.IntegrationID = storeTestUUID(t), apiConflict
	conflicting.SourceURL = "https://github.com/acme/" + coordinate + "/tree/conflicting"
	if _, err := postgres.SaveSDKReference(ctx, conflicting, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting release metadata error=%v, want ErrConflict", err)
	}
	var conflictRows int
	if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM sdk_references WHERE id=$1`, conflicting.ID).Scan(&conflictRows); err != nil || conflictRows != 0 {
		t.Fatalf("conflicting write persisted %d legacy rows, err=%v", conflictRows, err)
	}

	if err := postgres.DeleteSDKReference(ctx, apiA, referenceA.ID); err != nil {
		t.Fatalf("delete legacy reference: %v", err)
	}
	var bindingState string
	if err := postgres.pool.QueryRow(ctx, `SELECT binding_state FROM api_sdk_bindings WHERE id=$1`, referenceA.ID).Scan(&bindingState); err != nil || bindingState != "detached" {
		t.Fatalf("binding state=%q, err=%v", bindingState, err)
	}
	var packageCount, releaseCount int
	if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM sdk_packages WHERE id=$1`, packageID).Scan(&packageCount); err != nil {
		t.Fatal(err)
	}
	if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM sdk_releases WHERE id=$1`, releaseID).Scan(&releaseCount); err != nil {
		t.Fatal(err)
	}
	if packageCount != 1 || releaseCount != 1 {
		t.Fatalf("legacy delete removed typed truth: package=%d release=%d", packageCount, releaseCount)
	}
}

func TestPostgresLegacySDKConflictLedgerRequiresManualReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	postgres, deploymentID, organisationID := postgresLegacySDKDeployment(t, ctx)
	apiID := insertPostgresLegacySDKAPI(t, ctx, postgres, deploymentID, organisationID, "ledger")
	coordinate := "ledger-sdk-" + strings.ReplaceAll(storeTestUUID(t), "-", "")[:12]
	packageID, referenceID := storeTestUUID(t), storeTestUUID(t)
	if _, err := postgres.pool.Exec(ctx, `INSERT INTO sdk_packages(
		id,deployment_id,organisation_id,ecosystem,canonical_coordinate,display_coordinate,name,visibility,lifecycle
	) VALUES($1,$2,$3,'npm',$4,$4,$4,'private','active')`, packageID, deploymentID, organisationID, coordinate); err != nil {
		t.Fatal(err)
	}
	value := model.SDKReference{
		ID: referenceID, DeploymentID: deploymentID, OrganisationID: organisationID, IntegrationID: apiID,
		Ecosystem: "npm", Coordinate: coordinate, ExactVersion: "1.0.0",
		InstallCommand:   "npm install " + coordinate + "@1.0.0",
		DocumentationURL: "https://docs.example.test/" + coordinate,
		SourceURL:        "https://github.com/acme/" + coordinate, Visibility: model.VisibilityPrivate,
	}
	if _, err := postgres.pool.Exec(ctx, `INSERT INTO sdk_references(
		id,deployment_id,organisation_id,integration_id,ecosystem,coordinate,exact_version,
		install_command,documentation_url,source_url,checksum,visibility
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'',$11)`, value.ID, value.DeploymentID, value.OrganisationID,
		value.IntegrationID, value.Ecosystem, value.Coordinate, value.ExactVersion, value.InstallCommand,
		value.DocumentationURL, value.SourceURL, value.Visibility); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.pool.Exec(ctx, `INSERT INTO legacy_sdk_reference_migration_ledger(
		legacy_sdk_reference_id,deployment_id,integration_id,sdk_package_id,status,conflict_code,legacy_snapshot
	) VALUES($1,$2,$3,$4,'conflict','conflicting_release_metadata','{}'::jsonb)`,
		referenceID, deploymentID, apiID, packageID); err != nil {
		t.Fatal(err)
	}
	value.SourceURL += "/changed"
	if _, err := postgres.SaveSDKReference(ctx, value, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict-ledger legacy mutation error=%v, want ErrConflict", err)
	}
	var bindingCount int
	if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM api_sdk_bindings WHERE id=$1`, referenceID).Scan(&bindingCount); err != nil || bindingCount != 0 {
		t.Fatalf("manual-review conflict materialized %d bindings, err=%v", bindingCount, err)
	}
	var sourceURL string
	if err := postgres.pool.QueryRow(ctx, `SELECT source_url FROM sdk_references WHERE id=$1`, referenceID).Scan(&sourceURL); err != nil {
		t.Fatal(err)
	}
	if sourceURL == value.SourceURL {
		t.Fatal("conflict-ledger legacy snapshot was silently rewritten")
	}
}
