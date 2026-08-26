package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeveloperAssetFoundationMigrationDeclaresRequiredBoundaries(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0057_developer_asset_foundation.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	requiredTables := []string{
		"developer_asset_ingestion_runs",
		"developer_asset_ingestion_stages",
		"documentation_documents",
		"documentation_sections",
		"documentation_collections",
		"documentation_collection_revisions",
		"documentation_collection_members",
		"source_publication_document_selections",
		"documentation_maps",
		"source_publication_documentation_maps",
		"deployment_documentation_publications",
		"deployment_documentation_publication_members",
		"deployment_documentation_heads",
		"api_contracts",
		"api_contract_sources",
		"api_contract_candidates",
		"api_contract_revisions",
		"api_contract_revision_source_publications",
		"api_contract_operations",
		"api_contract_schemas",
		"api_contract_examples",
		"api_contract_maps",
		"sdk_packages",
		"sdk_releases",
		"sdk_release_lifecycle_events",
		"sdk_content_candidates",
		"sdk_content_publications",
		"sdk_publication_files",
		"sdk_sections",
		"sdk_symbols",
		"sdk_code_samples",
		"sdk_maps",
		"sdk_content_publication_file_selections",
		"sdk_content_publication_sample_selections",
		"sdk_content_publication_maps",
		"sdk_compatibility_assertions",
		"api_documentation_bindings",
		"api_contract_bindings",
		"api_sdk_bindings",
		"api_developer_asset_publications",
		"search_index_generations",
		"knowledge_units",
		"knowledge_unit_api_scopes",
		"retrieval_query_traces",
		"retrieval_query_trace_results",
		"retrieval_evaluation_sets",
		"retrieval_evaluation_set_revisions",
		"retrieval_evaluation_cases",
		"retrieval_evaluation_runs",
		"retrieval_evaluation_case_results",
		"legacy_sdk_reference_migration_ledger",
	}
	for _, table := range requiredTables {
		if !strings.Contains(sql, "CREATE TABLE "+table+" (") {
			t.Errorf("migration does not declare %s", table)
		}
	}
	requiredFragments := []string{
		"CREATE UNIQUE INDEX developer_asset_ingestion_one_active_target_idx",
		"lower(exact_version) <> 'latest'",
		"CREATE FUNCTION reject_developer_asset_immutable_mutation()",
		"developer-asset publication requires an exact published API revision",
		"documentation candidate must belong to a source-backed documentation ingestion run",
		"contract candidate must belong to an ingestion run for that exact contract",
		"contract ingestion requires one active source-to-contract target",
		"SDK content candidate must belong to an ingestion run for that exact release",
		"reference.id,\n    reference.deployment_id,\n    reference.integration_id",
		"'conflicting_release_metadata'",
		"'documentation.map_enrichment'",
		"'sdk.sample_review'",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing required boundary %q", fragment)
		}
	}
	if strings.Contains(sql, "DROP TABLE sdk_references") {
		t.Fatal("compatibility migration must not remove the legacy SDK reference path")
	}
	if strings.Contains(sql, "DROP INDEX crawl_jobs_one_active_per_source_idx") {
		t.Fatal("crawl reliability columns must preserve the one-active-job uniqueness boundary")
	}
}

func TestSDKSampleValidationEvidenceMigrationIsFailClosed(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0061_sdk_sample_validation_evidence.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"'not_checked','unvalidated','syntax_checked'",
		"review_evidence jsonb NOT NULL DEFAULT '{}'::jsonb",
		"CREATE FUNCTION guard_sdk_sample_approval_evidence()",
		"sample_evidence->>'validated' = 'true'",
		"NEW.review_evidence->>'summary'",
		"CREATE TRIGGER sdk_sample_approval_evidence_guard",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("SDK sample evidence migration is missing %q", required)
		}
	}
}

func TestHistoricalDeveloperAssetRootIdentityMigrationRestoresImmutableGuards(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0062_historical_developer_asset_root_identity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"documentation_collection_name",
		"documentation_collection_slug",
		"documentation_collection_description",
		"api_contract_name",
		"api_contract_slug",
		"api_contract_description",
		"api_contract_kind",
		"DROP TRIGGER developer_asset_immutable_guard ON documentation_collection_revisions",
		"DROP TRIGGER developer_asset_visibility_guard ON api_contract_revisions",
		"DROP TRIGGER developer_asset_immutable_guard ON api_publication_documentation_assets",
		"DROP TRIGGER developer_asset_visibility_guard ON api_publication_contract_assets",
		"CREATE FUNCTION guard_documentation_collection_revision_identity_snapshot()",
		"CREATE FUNCTION guard_api_contract_revision_identity_snapshot()",
		"CREATE FUNCTION guard_api_publication_documentation_identity_snapshot()",
		"CREATE FUNCTION guard_api_publication_contract_identity_snapshot()",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("historical root identity migration is missing %q", required)
		}
	}
	if got := strings.Count(sql, "CREATE TRIGGER developer_asset_immutable_guard"); got != 4 {
		t.Errorf("historical root migration restores %d immutable guards, want 4", got)
	}
	if got := strings.Count(sql, "CREATE TRIGGER developer_asset_visibility_guard"); got != 4 {
		t.Errorf("historical root migration restores %d visibility guards, want 4", got)
	}
}

func TestPostgresDeveloperAssetFoundationMigratesOnlyUnambiguousLegacySDKs(t *testing.T) {
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
	schema := "developer_assets_" + strings.ReplaceAll(storeTestUUID(t), "-", "")
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

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 56)); err != nil {
		t.Fatalf("migrate through 0056: %v", err)
	}
	const (
		organisationID = "00000000-0000-0000-0000-000000000001"
		deploymentID   = "00000000-0000-0000-0000-000000000002"
		apiAID         = "00000000-0000-0000-0000-000000000003"
		apiBID         = "00000000-0000-0000-0000-000000000004"
		bindingAID     = "00000000-0000-0000-0000-000000000011"
		bindingBID     = "00000000-0000-0000-0000-000000000012"
		conflictAID    = "00000000-0000-0000-0000-000000000013"
		conflictBID    = "00000000-0000-0000-0000-000000000014"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO organisations(id,name,slug)
		VALUES ($1,'Developer assets','developer-assets')`, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products(id,organisation_id,name,slug,description)
		VALUES ($1,$2,'Developer assets','developer-assets','')`, deploymentID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organisation_id,name,slug,description)
		VALUES ($1,$2,'Developer assets','developer-assets','')`, deploymentID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integrations(
			id,deployment_id,organisation_id,family_key,version_key,
			display_name,visibility,lifecycle
		) VALUES
			($1,$3,$4,'api-a','v1','API A','public','active'),
			($2,$3,$4,'api-b','v1','API B','public','active')`, apiAID, apiBID, deploymentID, organisationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_references(
			id,deployment_id,organisation_id,integration_id,ecosystem,
			coordinate,exact_version,install_command,documentation_url,
			source_url,checksum,visibility
		) VALUES
			($5,$2,$1,$3,'pypi','Acme_SDK','2.1.0',
			 'pip install acme-sdk==2.1.0','https://docs.example/sdk',
			 'https://git.example/sdk','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','public'),
			($6,$2,$1,$4,'pypi','acme-sdk','2.1.0',
			 'pip install acme-sdk==2.1.0','https://docs.example/sdk',
			 'https://git.example/sdk','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','public'),
			($7,$2,$1,$3,'npm','bad-sdk','1.0.0','npm install bad-sdk@1.0.0',
			 'https://docs.example/bad','https://git.example/bad-a','','private'),
			($8,$2,$1,$4,'npm','bad-sdk','1.0.0','npm install bad-sdk@1.0.0',
			 'https://docs.example/bad','https://git.example/bad-b','','private')`,
		organisationID, deploymentID, apiAID, apiBID, bindingAID, bindingBID, conflictAID, conflictBID,
	); err != nil {
		t.Fatalf("seed legacy SDK references: %v", err)
	}

	if err := Migrate(ctx, pool, copyMigrationsThrough(t, 57)); err != nil {
		t.Fatalf("apply developer-asset foundation: %v", err)
	}

	var packageCount, releaseCount, bindingCount, migratedCount, conflictCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sdk_packages`).Scan(&packageCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sdk_releases`).Scan(&releaseCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM api_sdk_bindings`).Scan(&bindingCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sdk_reference_migration_ledger WHERE status='migrated'`).Scan(&migratedCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legacy_sdk_reference_migration_ledger WHERE status='conflict' AND conflict_code='conflicting_release_metadata'`).Scan(&conflictCount); err != nil {
		t.Fatal(err)
	}
	if packageCount != 2 || releaseCount != 1 || bindingCount != 2 || migratedCount != 2 || conflictCount != 2 {
		t.Fatalf("backfill counts packages=%d releases=%d bindings=%d migrated=%d conflicts=%d", packageCount, releaseCount, bindingCount, migratedCount, conflictCount)
	}

	for _, bindingID := range []string{bindingAID, bindingBID} {
		var migratedID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM api_sdk_bindings WHERE id=$1`, bindingID).Scan(&migratedID); err != nil {
			t.Fatalf("legacy binding ID %s was not preserved: %v", bindingID, err)
		}
	}
	var canonicalCoordinate string
	if err := pool.QueryRow(ctx, `SELECT canonical_coordinate FROM sdk_packages WHERE ecosystem='pypi'`).Scan(&canonicalCoordinate); err != nil {
		t.Fatal(err)
	}
	if canonicalCoordinate != "acme-sdk" {
		t.Fatalf("canonical PyPI coordinate = %q, want acme-sdk", canonicalCoordinate)
	}

	var releaseID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM sdk_releases`).Scan(&releaseID); err != nil {
		t.Fatal(err)
	}

	// Candidate outputs are queryable before approval; immutable publications
	// can select them only after the ingestion run reaches review_ready.
	sourceID := storeTestUUID(t)
	documentationRunID := storeTestUUID(t)
	documentID := storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO sources(
		id,organisation_id,product_id,name,kind,location,visibility
	) VALUES ($1,$2,$3,'Candidate docs','website','https://docs.example','public')`,
		sourceID, organisationID, deploymentID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO developer_asset_ingestion_runs(
		id,deployment_id,organisation_id,asset_kind,target_id,target_key,source_id,state,
		pipeline_version,parser_version,normalizer_version,mapper_version
	) VALUES ($1,$2,$3,'documentation',$4,$5,$4,'running','pipeline-v1','html-v1','docs-v1','map-v1')`,
		documentationRunID, deploymentID, organisationID, sourceID, "source:"+sourceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO documentation_documents(
		id,deployment_id,ingestion_run_id,source_path,title,normalized_markdown,
		content_hash,visibility,ordinal
	) VALUES ($1,$2,$3,'index.md','Candidate docs','# Candidate docs',$4,'public',0)`,
		documentID, deploymentID, documentationRunID, "sha256:"+strings.Repeat("b", 64),
	); err != nil {
		t.Fatalf("insert pre-publication documentation candidate: %v", err)
	}
	var candidateDocuments int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM documentation_documents WHERE ingestion_run_id=$1`, documentationRunID).Scan(&candidateDocuments); err != nil || candidateDocuments != 1 {
		t.Fatalf("documentation candidate preview count=%d err=%v", candidateDocuments, err)
	}
	documentationCrawlID := storeTestUUID(t)
	documentationPublicationID := storeTestUUID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO crawl_jobs(
		id,organisation_id,product_id,source_id,state
	) VALUES ($1,$2,$3,$4,'review')`, documentationCrawlID, organisationID, deploymentID, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_publications(
		id,organisation_id,product_id,source_id,crawl_job_id,revision,visibility,
		content_hash,document_count,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,$4,$5,1,'public',$6,1,'reviewer',now())`,
		documentationPublicationID, organisationID, deploymentID, sourceID, documentationCrawlID,
		"sha256:"+strings.Repeat("9", 64),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_publication_document_selections(
		source_publication_id,deployment_id,documentation_document_id,decision,ordinal,
		content_hash,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,'included',0,$4,'reviewer',now())`,
		documentationPublicationID, deploymentID, documentID, "sha256:"+strings.Repeat("b", 64),
	); err == nil {
		t.Fatal("running documentation candidate was selected into a source publication")
	}
	if _, err := pool.Exec(ctx, `UPDATE developer_asset_ingestion_runs
		SET state='review_ready', finished_at=now(), started_at=coalesce(started_at,now())
		WHERE id=$1`, documentationRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_publication_document_selections(
		source_publication_id,deployment_id,documentation_document_id,decision,ordinal,
		content_hash,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,'included',0,$4,'reviewer',now())`,
		documentationPublicationID, deploymentID, documentID, "sha256:"+strings.Repeat("b", 64),
	); err != nil {
		t.Fatalf("select review-ready documentation candidate: %v", err)
	}

	contractSourceID := storeTestUUID(t)
	contractID := storeTestUUID(t)
	contractSourceBindingID := storeTestUUID(t)
	contractRunID := storeTestUUID(t)
	contractCandidateID := storeTestUUID(t)
	contractRevisionID := storeTestUUID(t)
	contractSourceHash := "sha256:" + strings.Repeat("e", 64)
	contractContentHash := "sha256:" + strings.Repeat("f", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO sources(
		id,organisation_id,product_id,name,kind,location,visibility
	) VALUES ($1,$2,$3,'Candidate contract','openapi','https://api.example/openapi.json','public')`,
		contractSourceID, organisationID, deploymentID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_contracts(
		id,deployment_id,organisation_id,name,slug,visibility
	) VALUES ($1,$2,$3,'Candidate contract','candidate-contract','public')`,
		contractID, deploymentID, organisationID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO developer_asset_ingestion_runs(
		id,deployment_id,organisation_id,asset_kind,target_id,target_key,source_id,state,
		pipeline_version,parser_version,normalizer_version,mapper_version
	) VALUES ($1,$2,$3,'contract',$4,$5,$6,'running','pipeline-v1','openapi-v1','contract-v1','map-v1')`,
		contractRunID, deploymentID, organisationID, contractID, "contract:"+contractID, contractSourceID,
	); err == nil {
		t.Fatal("contract ingestion run was accepted without an active source target")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_contract_sources(
		id,deployment_id,api_contract_id,source_id,source_role,lifecycle
	) VALUES ($1,$2,$3,$4,'primary','attached')`,
		contractSourceBindingID, deploymentID, contractID, contractSourceID,
	); err != nil {
		t.Fatalf("attach contract source: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO developer_asset_ingestion_runs(
		id,deployment_id,organisation_id,asset_kind,target_id,target_key,source_id,state,
		pipeline_version,parser_version,normalizer_version,mapper_version
	) VALUES ($1,$2,$3,'contract',$4,$5,$6,'running','pipeline-v1','openapi-v1','contract-v1','map-v1')`,
		contractRunID, deploymentID, organisationID, contractID, "contract:"+contractID, contractSourceID,
	); err != nil {
		t.Fatalf("enqueue mapped contract source: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_contract_candidates(
		id,deployment_id,api_contract_id,ingestion_run_id,openapi_version,source_format,
		normalized_contract,source_hash,content_hash,parser_version,visibility
	) VALUES ($1,$2,$3,$4,'3.1.0','json','{"openapi":"3.1.0"}'::jsonb,$5,$6,'openapi-v1','public')`,
		contractCandidateID, deploymentID, contractID, contractRunID, contractSourceHash, contractContentHash,
	); err != nil {
		t.Fatalf("insert pre-publication contract candidate: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_contract_revisions(
		id,deployment_id,api_contract_id,api_contract_candidate_id,revision,
		content_hash,visibility,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,$4,1,$5,'public','reviewer',now())`,
		contractRevisionID, deploymentID, contractID, contractCandidateID, contractContentHash,
	); err == nil {
		t.Fatal("running contract candidate was published before review")
	}
	if _, err := pool.Exec(ctx, `UPDATE developer_asset_ingestion_runs
		SET state='review_ready', finished_at=now(), started_at=coalesce(started_at,now())
		WHERE id=$1`, contractRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_contract_revisions(
		id,deployment_id,api_contract_id,api_contract_candidate_id,revision,
		content_hash,visibility,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,$4,1,$5,'public','reviewer',now())`,
		contractRevisionID, deploymentID, contractID, contractCandidateID, contractContentHash,
	); err != nil {
		t.Fatalf("publish review-ready contract candidate: %v", err)
	}

	sdkRunID := storeTestUUID(t)
	sdkCandidateID := storeTestUUID(t)
	sdkSampleID := storeTestUUID(t)
	sdkUncheckedSampleID := storeTestUUID(t)
	sdkPublicationID := storeTestUUID(t)
	sdkCandidateHash := "sha256:" + strings.Repeat("c", 64)
	sdkSampleHash := "sha256:" + strings.Repeat("d", 64)
	sdkUncheckedSampleHash := "sha256:" + strings.Repeat("e", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO developer_asset_ingestion_runs(
		id,deployment_id,organisation_id,asset_kind,target_id,target_key,state,
		pipeline_version,parser_version,normalizer_version,mapper_version
	) VALUES ($1,$2,$3,'sdk',$4,$5,'running','pipeline-v1','sdk-v1','docs-v1','map-v1')`,
		sdkRunID, deploymentID, organisationID, releaseID, "sdk-release:"+releaseID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_candidates(
		id,deployment_id,sdk_release_id,ingestion_run_id,pipeline_version,
		parser_version,normalizer_version,mapper_version,map_version,
		content_hash,visibility
	) VALUES ($1,$2,$3,$4,'pipeline-v1','sdk-v1','docs-v1','map-v1','sdk-map-v1',$5,'public')`,
		sdkCandidateID, deploymentID, releaseID, sdkRunID, sdkCandidateHash,
	); err != nil {
		t.Fatalf("insert pre-publication SDK candidate: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_code_samples(
		id,deployment_id,sdk_content_candidate_id,language,title,intent,code,origin,
		validation_status,validation_evidence,visibility,content_hash
	) VALUES ($1,$2,$3,'python','List customers','List customers','client.customers.list()',
		'curated','syntax_checked','{"validated":true,"validator":"test/python-parser"}'::jsonb,'public',$4)`, sdkSampleID, deploymentID, sdkCandidateID, sdkSampleHash); err != nil {
		t.Fatalf("insert pre-publication SDK sample: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_code_samples(
		id,deployment_id,sdk_content_candidate_id,language,title,intent,code,origin,
		validation_status,validation_evidence,visibility,content_hash
	) VALUES ($1,$2,$3,'rust','List customers','List customers','client.customers().list()',
		'curated','not_checked','{"validated":false,"result":"not_checked"}'::jsonb,'public',$4)`, sdkUncheckedSampleID, deploymentID, sdkCandidateID, sdkUncheckedSampleHash); err != nil {
		t.Fatalf("insert not-checked SDK sample: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_publications(
		id,deployment_id,sdk_release_id,sdk_content_candidate_id,revision,
		content_hash,visibility,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,$4,1,$5,'public','reviewer',now())`,
		sdkPublicationID, deploymentID, releaseID, sdkCandidateID, sdkCandidateHash,
	); err == nil {
		t.Fatal("running SDK candidate was published before review")
	}
	if _, err := pool.Exec(ctx, `UPDATE developer_asset_ingestion_runs
		SET state='review_ready', finished_at=now(), started_at=coalesce(started_at,now())
		WHERE id=$1`, sdkRunID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_publications(
		id,deployment_id,sdk_release_id,sdk_content_candidate_id,revision,
		content_hash,visibility,reviewed_by,reviewed_at
	) VALUES ($1,$2,$3,$4,1,$5,'public','reviewer',now())`,
		sdkPublicationID, deploymentID, releaseID, sdkCandidateID, sdkCandidateHash,
	); err != nil {
		t.Fatalf("publish review-ready SDK candidate: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_publication_sample_selections(
		sdk_content_publication_id,deployment_id,sdk_content_candidate_id,
		sdk_code_sample_id,decision,ordinal,reviewed_by,reviewed_at,content_hash
	) VALUES ($1,$2,$3,$4,'approved',0,'reviewer',now(),$5)`,
		sdkPublicationID, deploymentID, sdkCandidateID, sdkSampleID, sdkSampleHash,
	); err != nil {
		t.Fatalf("approve validated SDK sample: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_publication_sample_selections(
		sdk_content_publication_id,deployment_id,sdk_content_candidate_id,
		sdk_code_sample_id,decision,ordinal,reviewed_by,reviewed_at,content_hash
	) VALUES ($1,$2,$3,$4,'approved',1,'reviewer',now(),$5)`,
		sdkPublicationID, deploymentID, sdkCandidateID, sdkUncheckedSampleID, sdkUncheckedSampleHash,
	); err == nil {
		t.Fatal("database approved a not-checked SDK sample without explicit review evidence")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sdk_content_publication_sample_selections(
		sdk_content_publication_id,deployment_id,sdk_content_candidate_id,
		sdk_code_sample_id,decision,review_evidence,ordinal,reviewed_by,reviewed_at,content_hash
	) VALUES ($1,$2,$3,$4,'approved','{"summary":"Reviewer used a pinned Rust grammar parser."}'::jsonb,1,'reviewer',now(),$5)`,
		sdkPublicationID, deploymentID, sdkCandidateID, sdkUncheckedSampleID, sdkUncheckedSampleHash,
	); err != nil {
		t.Fatalf("database rejected structured review evidence for a not-checked SDK sample: %v", err)
	}
	storedSDKPublication, err := NewPostgres(pool, "").SDKContentPublication(ctx, deploymentID, sdkPublicationID)
	if err != nil || len(storedSDKPublication.SampleSelections) != 2 ||
		len(storedSDKPublication.SampleSelections[0].ReviewEvidence) != 0 ||
		!model.ValidSDKSampleReviewEvidence(storedSDKPublication.SampleSelections[1].ReviewEvidence) {
		t.Fatalf("Postgres publication review evidence = %#v, err=%v", storedSDKPublication.SampleSelections, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE sdk_releases SET install_command='mutated' WHERE id=$1`, releaseID); err == nil {
		t.Fatal("immutable SDK release accepted an in-place update")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_prompt_settings(product_id,prompt_key) VALUES ($1,'sdk.map_enrichment')`, deploymentID); err != nil {
		t.Fatalf("new bounded prompt key was rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_prompt_settings(product_id,prompt_key) VALUES ($1,'sdk.unbounded_generation')`, deploymentID); err == nil {
		t.Fatal("unknown AI prompt key was accepted")
	}

	var indexDefinition string
	if err := pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=$1 AND indexname='crawl_jobs_one_active_per_source_idx'`, schema).Scan(&indexDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexDefinition, "state = ANY") && !strings.Contains(indexDefinition, "state IN") {
		t.Fatalf("active crawl uniqueness changed unexpectedly: %s", indexDefinition)
	}
	t.Logf("verified developer-asset migration in isolated schema %s", schema)
}
