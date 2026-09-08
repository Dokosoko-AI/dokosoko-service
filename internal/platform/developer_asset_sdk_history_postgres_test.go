package platform

import (
	"context"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A historical applicability advisory is a database lineage fixture. It is
// deliberately not an AI-quality result or a compatibility assertion.
func insertSDKHistoryAdvisory(t *testing.T, pool *pgxpool.Pool, publication model.APIDeveloperAssetPublication, release taskRetrievalRelease, bindingID, suffix string) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO developer_asset_ai_advisory_runs
		(deployment_id,prompt_key,prompt_version,scope_kind,scope_id,scope_visibility,
		ingestion_run_id,sdk_package_id,sdk_release_id,sdk_content_candidate_id,sdk_content_publication_id,
		integration_id,api_developer_asset_publication_id,api_sdk_binding_id,allowed_evidence_ids,
		evidence_hash,input_hash,result,result_hash,created_by)
		VALUES($1,'sdk.applicability_suggestion','history-fixture-v1','sdk_api_binding',$2,'public',
		$3,$4,$5,$6,$7,$8,$9,$2,'["fixture-evidence"]',$10,$11,'{}',$12,'history-fixture')`,
		publication.DeploymentID, bindingID, release.candidate.Candidate.IngestionRunID,
		release.release.SDKPackageID, release.release.ID, release.candidate.Candidate.ID,
		release.publication.ID, publication.APIID, publication.ID, contentHash([]byte("fixture-evidence")),
		contentHash([]byte(bindingID+suffix)), contentHash([]byte("{}")))
	if err != nil {
		t.Fatalf("insert historical advisory %s: %v", suffix, err)
	}
}

func assertSDKBindingHistoryGuards(t *testing.T, pool *pgxpool.Pool, oldPublication, currentPublication model.APIDeveloperAssetPublication, old taskRetrievalRelease, bindingID string) {
	t.Helper()
	ctx := t.Context()
	// New advisories may still cite an old immutable publication after the
	// attachment advances. The existing lineage trigger resolves that snapshot.
	insertSDKHistoryAdvisory(t, pool, oldPublication, old, bindingID, "after-upgrade")
	var oldReleaseID, oldContentID string
	if err := pool.QueryRow(ctx, `SELECT sdk_release_id::text,sdk_content_publication_id::text
		FROM api_publication_sdk_assets WHERE api_developer_asset_publication_id=$1 AND api_sdk_binding_id=$2`, oldPublication.ID, bindingID).Scan(&oldReleaseID, &oldContentID); err != nil || oldReleaseID != old.release.ID || oldContentID != old.publication.ID {
		t.Fatalf("historical exact selections changed: %s %s %v", oldReleaseID, oldContentID, err)
	}
	duplicate := `INSERT INTO api_publication_sdk_assets SELECT * FROM api_publication_sdk_assets
		WHERE api_developer_asset_publication_id=$1 AND api_sdk_binding_id=$2 ON CONFLICT DO NOTHING`
	// BEFORE INSERT must reject a stale selection even though the primary key
	// already exists. The current selection is a positive control for this SQL.
	if _, err := pool.Exec(ctx, duplicate, currentPublication.ID, bindingID); err != nil {
		t.Fatalf("current exact selection rejected: %v", err)
	}
	_, err := pool.Exec(ctx, duplicate, oldPublication.ID, bindingID)
	requireSDKHistoryDatabaseError(t, err, "23514", "API SDK snapshot must match the exact current attachment selection")
	_, err = pool.Exec(ctx, `INSERT INTO api_publication_sdk_assets
		SELECT (jsonb_populate_record(NULL::api_publication_sdk_assets,
		to_jsonb(asset)||jsonb_build_object('sdk_release_id',$3::text))).*
		FROM api_publication_sdk_assets asset
		WHERE api_developer_asset_publication_id=$1 AND api_sdk_binding_id=$2 ON CONFLICT DO NOTHING`, currentPublication.ID, bindingID, old.release.ID)
	requireSDKHistoryDatabaseError(t, err, "23514", "API SDK snapshot must match the exact current attachment selection")
	_, err = pool.Exec(ctx, `UPDATE api_publication_sdk_assets SET selector=selector
		WHERE api_developer_asset_publication_id=$1`, oldPublication.ID)
	requireSDKHistoryDatabaseError(t, err, "55000", "")
	_, err = pool.Exec(ctx, `DELETE FROM api_sdk_bindings WHERE id=$1`, bindingID)
	requireSDKHistoryDatabaseError(t, err, "23503", "")
	// A publication must hold its attachment selection stable until commit.
	publicationTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = publicationTx.Rollback(context.Background()) }()
	if _, err := publicationTx.Exec(ctx, duplicate, currentPublication.ID, bindingID); err != nil {
		t.Fatal(err)
	}
	updateTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = updateTx.Rollback(context.Background()) }()
	if _, err := updateTx.Exec(ctx, `SET LOCAL lock_timeout = '100ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = updateTx.Exec(ctx, `UPDATE api_sdk_bindings SET revision=revision+1 WHERE id=$1`, bindingID)
	requireSDKHistoryDatabaseError(t, err, "55P03", "")
}

func requireSDKHistoryDatabaseError(t *testing.T, err error, code, message string) {
	t.Helper()
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != code || (message != "" && pgError.Message != message) {
		t.Fatalf("expected database error %s %q, received %v", code, message, err)
	}
}
