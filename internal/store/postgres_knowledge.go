package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/docreview"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
	"strings"
)

func scanSource(row interface{ Scan(...any) error }) (model.Source, error) {
	var value model.Source
	var state string
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Kind, &value.Location, &value.Visibility, &state, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	value.Published = state == "published"
	value.Quarantined = state == "quarantined"
	return value, databaseError(err)
}

func (p *Postgres) Sources(ctx context.Context, productID string) ([]model.Source, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at FROM sources WHERE product_id = $1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Source, 0)
	for rows.Next() {
		value, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Source(ctx context.Context, productID, id string) (model.Source, error) {
	return scanSource(p.pool.QueryRow(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at FROM sources WHERE product_id = $1 AND id = $2`, productID, id))
}

func (p *Postgres) CreateSource(ctx context.Context, value model.Source) (model.Source, error) {
	return scanSource(p.pool.QueryRow(ctx, `INSERT INTO sources(id, organisation_id, product_id, name, kind, location, visibility, state) VALUES ($1, $2, $3, $4, $5, $6, 'private', 'draft') RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Kind, value.Location))
}

func (p *Postgres) UpdateSource(ctx context.Context, value model.Source, expected int64) (model.Source, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanSource(tx.QueryRow(ctx, `UPDATE sources SET visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $4 RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, value.ProductID, value.ID, value.Visibility, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Source(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Source{}, ErrConflict
		}
	}
	if err != nil {
		return model.Source{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND source_id = $2 AND state = 'published'`, value.ProductID, value.ID, value.Visibility); err != nil {
		return model.Source{}, err
	}
	return updated, tx.Commit(ctx)
}

func scanSourcePublication(row interface{ Scan(...any) error }) (model.SourcePublication, error) {
	var value model.SourcePublication
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.SourceID, &value.CrawlJobID, &value.Revision, &value.Visibility, &value.ContentHash, &value.DocumentCount, &value.ReviewedBy, &value.ReviewedAt, &value.PublishedAt)
	return value, databaseError(err)
}

const sourcePublicationSelect = `SELECT id::text, organisation_id::text, product_id::text, source_id::text, crawl_job_id::text, revision, visibility::text, content_hash, document_count, reviewed_by, reviewed_at, published_at FROM source_publications`

func (p *Postgres) SourcePublications(ctx context.Context, productID, sourceID string) ([]model.SourcePublication, error) {
	rows, err := p.pool.Query(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND source_id = $2 ORDER BY revision DESC`, productID, sourceID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SourcePublication, 0)
	for rows.Next() {
		value, scanErr := scanSourcePublication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SourcePublication(ctx context.Context, productID, publicationID string) (model.SourcePublication, error) {
	return scanSourcePublication(p.pool.QueryRow(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND id = $2`, productID, publicationID))
}

func (p *Postgres) SourceReview(ctx context.Context, productID, sourceID, crawlJobID string) (model.SourceReview, error) {
	source, err := p.Source(ctx, productID, sourceID)
	if err != nil {
		return model.SourceReview{}, err
	}
	jobQuery := crawlJobSelect + ` WHERE product_id = $1 AND source_id = $2`
	args := []any{productID, sourceID}
	if strings.TrimSpace(crawlJobID) != "" {
		jobQuery += ` AND id = $3`
		args = append(args, crawlJobID)
	}
	jobQuery += ` ORDER BY queued_at DESC, id DESC LIMIT 1`
	job, err := scanCrawlJob(p.pool.QueryRow(ctx, jobQuery, args...))
	if err != nil {
		return model.SourceReview{}, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT kd.id::text, cjd.crawl_job_id::text, kd.snapshot_id::text, kd.title,
		       kd.canonical_url, cjd.assessment_state::text, cjd.assessment_trust_level,
		       cjd.assessment_injection_indicators,
		       'sha256:' || encode(ss.content_sha256, 'hex'), cjd.changed
		FROM crawl_job_documents cjd
		JOIN knowledge_documents kd ON kd.id = cjd.knowledge_document_id
		JOIN source_snapshots ss ON ss.id = kd.snapshot_id
		WHERE cjd.crawl_job_id = $1 AND kd.product_id = $2 AND kd.source_id = $3
		ORDER BY kd.canonical_url, kd.id`, job.ID, productID, sourceID)
	if err != nil {
		return model.SourceReview{}, databaseError(err)
	}
	defer rows.Close()
	documents := make([]model.CrawlReviewDocument, 0)
	for rows.Next() {
		var value model.CrawlReviewDocument
		if scanErr := rows.Scan(&value.ID, &value.CrawlJobID, &value.SnapshotID, &value.Title, &value.CanonicalURL, &value.State, &value.TrustLevel, &value.InjectionIndicators, &value.ContentHash, &value.Changed); scanErr != nil {
			return model.SourceReview{}, scanErr
		}
		documents = append(documents, value)
	}
	if err := rows.Err(); err != nil {
		return model.SourceReview{}, err
	}
	review := model.SourceReview{Source: source, CrawlJob: job, Documents: documents}
	publication, err := scanSourcePublication(p.pool.QueryRow(ctx, sourcePublicationSelect+` WHERE product_id = $1 AND source_id = $2 AND crawl_job_id = $3`, productID, sourceID, job.ID))
	if err == nil {
		review.Publication = &publication
	} else if !errors.Is(err, ErrNotFound) {
		return model.SourceReview{}, err
	}
	return review, nil
}

func (p *Postgres) PublishSource(ctx context.Context, productID, sourceID string, expected int64, publication model.SourcePublication, documentIDs []string) (model.Source, model.SourcePublication, error) {
	if len(documentIDs) == 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// All crawler paths that can create or reassess evidence take this row
	// first. Holding it through commit makes the checks below one serial view.
	lockedSource, err := scanSource(tx.QueryRow(ctx, `
		SELECT id::text, organisation_id::text, product_id::text, name, kind, location,
		       visibility::text, state::text, revision, created_at, updated_at
		FROM sources
		WHERE product_id = $1 AND id = $2
		FOR UPDATE`, productID, sourceID))
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if lockedSource.Revision != expected || lockedSource.Quarantined {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}

	latestJob, err := scanCrawlJob(tx.QueryRow(ctx, crawlJobSelect+`
		WHERE product_id = $1 AND source_id = $2
		ORDER BY queued_at DESC, id DESC
		LIMIT 1
		FOR UPDATE`, productID, sourceID))
	if errors.Is(err, ErrNotFound) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if latestJob.ID != publication.CrawlJobID || latestJob.FinishedAt == nil ||
		(latestJob.State != "review" && latestJob.State != "succeeded") ||
		latestJob.FetchedCount == 0 || latestJob.FailedCount != 0 || latestJob.SkippedCount != 0 {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}

	rows, err := tx.Query(ctx, `
		SELECT kd.id::text, cjd.crawl_job_id::text, kd.snapshot_id::text, kd.title,
		       kd.canonical_url, cjd.assessment_state::text, cjd.assessment_trust_level,
		       cjd.assessment_injection_indicators,
		       'sha256:' || encode(ss.content_sha256, 'hex'), cjd.changed,
		       kd.state::text, kd.injection_indicators
		FROM crawl_job_documents cjd
		JOIN knowledge_documents kd ON kd.id = cjd.knowledge_document_id
		JOIN source_snapshots ss ON ss.id = kd.snapshot_id
		WHERE cjd.crawl_job_id = $1
		  AND kd.product_id = $2 AND kd.source_id = $3
		  AND kd.id = ANY($4::uuid[])
		ORDER BY kd.id
		FOR UPDATE OF cjd, kd`, publication.CrawlJobID, productID, sourceID, documentIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, databaseError(err)
	}
	lockedDocuments := make([]model.CrawlReviewDocument, 0, len(documentIDs))
	for rows.Next() {
		var document model.CrawlReviewDocument
		var liveState string
		var liveIndicators json.RawMessage
		if scanErr := rows.Scan(
			&document.ID, &document.CrawlJobID, &document.SnapshotID, &document.Title,
			&document.CanonicalURL, &document.State, &document.TrustLevel,
			&document.InjectionIndicators, &document.ContentHash, &document.Changed,
			&liveState, &liveIndicators,
		); scanErr != nil {
			rows.Close()
			return model.Source{}, model.SourcePublication{}, scanErr
		}
		if !docreview.SafeAssessment(document.State, document.InjectionIndicators) ||
			!docreview.SafeAssessment(liveState, liveIndicators) {
			rows.Close()
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
		lockedDocuments = append(lockedDocuments, document)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return model.Source{}, model.SourcePublication{}, rowsErr
	}
	if len(lockedDocuments) != len(documentIDs) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	lockedHash, err := docreview.PublicationContentHash(lockedDocuments)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if publication.ContentHash != lockedHash {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	publication.OrganisationID = lockedSource.OrganisationID
	publication.ProductID = productID
	publication.SourceID = sourceID
	publication.Visibility = lockedSource.Visibility
	publication.DocumentCount = len(documentIDs)

	var documentationReview *SourcePublicationDocumentationReview
	typedRun, runErr := scanDeveloperAssetIngestionRun(tx.QueryRow(ctx, developerAssetIngestionRunSelect+`
		WHERE deployment_id=$1 AND id=$2 AND source_id=$3
		FOR UPDATE`, productID, publication.CrawlJobID, sourceID))
	if errors.Is(runErr, ErrNotFound) {
		// An unbound OpenAPI source may be reviewed as legacy evidence before
		// being attached and recrawled for an exact contract target. An uploaded
		// contract source can also have no new typed run when normalization is
		// unchanged. Documentation-producing sources fail closed without one.
		var contractSource bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM api_contract_sources binding
			JOIN api_contracts contract ON contract.id=binding.api_contract_id AND contract.deployment_id=binding.deployment_id
			WHERE binding.deployment_id=$1 AND binding.source_id=$2 AND binding.lifecycle='attached' AND contract.lifecycle='active'
		)`, productID, sourceID).Scan(&contractSource); lookupErr != nil {
			return model.Source{}, model.SourcePublication{}, databaseError(lookupErr)
		}
		if lockedSource.Kind != "openapi" && !contractSource {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
	} else if runErr != nil {
		return model.Source{}, model.SourcePublication{}, runErr
	} else {
		if typedRun.DeploymentID != productID || typedRun.OrganisationID != lockedSource.OrganisationID ||
			typedRun.ID != publication.CrawlJobID || typedRun.SourceID != sourceID ||
			typedRun.State != model.DeveloperAssetIngestionReviewReady ||
			typedRun.AcquiredCount != latestJob.FetchedCount || typedRun.FailedCount != 0 || typedRun.SkippedCount != 0 {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
		switch typedRun.AssetKind {
		case model.DeveloperAssetDocumentation:
			legacyRows, queryErr := tx.Query(ctx, `
				SELECT kd.id::text,cjd.crawl_job_id::text,cjd.assessment_state::text,
				       cjd.assessment_injection_indicators
				FROM crawl_job_documents cjd
				JOIN knowledge_documents kd ON kd.id=cjd.knowledge_document_id
				WHERE cjd.crawl_job_id=$1 AND kd.product_id=$2 AND kd.source_id=$3
				ORDER BY kd.id
				FOR UPDATE OF cjd,kd`, publication.CrawlJobID, productID, sourceID)
			if queryErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(queryErr)
			}
			legacyDocuments := make(map[string]model.CrawlReviewDocument)
			for legacyRows.Next() {
				var document model.CrawlReviewDocument
				if scanErr := legacyRows.Scan(&document.ID, &document.CrawlJobID, &document.State, &document.InjectionIndicators); scanErr != nil {
					legacyRows.Close()
					return model.Source{}, model.SourcePublication{}, databaseError(scanErr)
				}
				if legacyDocuments[document.ID].ID != "" {
					legacyRows.Close()
					return model.Source{}, model.SourcePublication{}, ErrConflict
				}
				legacyDocuments[document.ID] = document
			}
			legacyRowsErr := legacyRows.Err()
			legacyRows.Close()
			if legacyRowsErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(legacyRowsErr)
			}

			typedRows, queryErr := tx.Query(ctx, documentationDocumentSelect+`
				WHERE deployment_id=$1 AND ingestion_run_id=$2
				ORDER BY ordinal,id
				FOR UPDATE`, productID, typedRun.ID)
			if queryErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(queryErr)
			}
			typedDocuments := make([]model.DocumentationDocument, 0, typedRun.AcquiredCount)
			for typedRows.Next() {
				document, scanErr := scanDocumentationDocument(typedRows)
				if scanErr != nil {
					typedRows.Close()
					return model.Source{}, model.SourcePublication{}, scanErr
				}
				typedDocuments = append(typedDocuments, document)
			}
			typedRowsErr := typedRows.Err()
			typedRows.Close()
			if typedRowsErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(typedRowsErr)
			}

			mapRows, queryErr := tx.Query(ctx, documentationMapSelect+`
				WHERE deployment_id=$1 AND ingestion_run_id=$2
				ORDER BY created_at,id
				FOR UPDATE`, productID, typedRun.ID)
			if queryErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(queryErr)
			}
			maps := make([]model.DocumentationMap, 0, 1)
			for mapRows.Next() {
				value, scanErr := scanDocumentationMap(mapRows)
				if scanErr != nil {
					mapRows.Close()
					return model.Source{}, model.SourcePublication{}, scanErr
				}
				maps = append(maps, value)
			}
			mapRowsErr := mapRows.Err()
			mapRows.Close()
			if mapRowsErr != nil {
				return model.Source{}, model.SourcePublication{}, databaseError(mapRowsErr)
			}
			if len(maps) != 1 {
				return model.Source{}, model.SourcePublication{}, ErrConflict
			}
			review, bridgeErr := buildSourcePublicationDocumentationReview(
				productID, typedRun, publication, typedDocuments, &maps[0], legacyDocuments, documentIDs,
			)
			if bridgeErr != nil {
				return model.Source{}, model.SourcePublication{}, bridgeErr
			}
			documentationReview = &review
		case model.DeveloperAssetContract:
			// The exact contract candidate publication consumes this legacy source
			// publication and owns the contract run's published transition.
		default:
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
	}

	updated, err := scanSource(tx.QueryRow(ctx, `UPDATE sources SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id = $1 AND id = $2 AND revision = $3 AND state <> 'quarantined' RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, productID, sourceID, expected))
	if errors.Is(err, ErrNotFound) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	publication, err = scanSourcePublication(tx.QueryRow(ctx, `
		INSERT INTO source_publications(id, organisation_id, product_id, source_id, crawl_job_id, revision, visibility, content_hash, document_count, reviewed_by, reviewed_at, published_at)
		SELECT $1, $2, $3, $4, $5, coalesce(max(revision), 0) + 1, $6, $7, $8, $9, $10, $11
		FROM source_publications WHERE source_id = $4
		RETURNING id::text, organisation_id::text, product_id::text, source_id::text, crawl_job_id::text, revision, visibility::text, content_hash, document_count, reviewed_by, reviewed_at, published_at`, publication.ID, updated.OrganisationID, productID, sourceID, publication.CrawlJobID, updated.Visibility, publication.ContentHash, len(documentIDs), publication.ReviewedBy, publication.ReviewedAt, publication.PublishedAt))
	if err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO source_publication_documents(source_publication_id, knowledge_document_id)
		SELECT $1, value::uuid FROM unnest($2::text[]) AS selected(value)`, publication.ID, documentIDs)
	if err != nil {
		return model.Source{}, model.SourcePublication{}, databaseError(err)
	}
	if result.RowsAffected() != int64(len(documentIDs)) {
		return model.Source{}, model.SourcePublication{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET state = 'published', visibility = $3, revision = revision + 1, updated_at = now() WHERE product_id = $1 AND source_id = $2 AND id = ANY($4::uuid[]) AND state = 'validated' AND injection_indicators = '[]'::jsonb`, productID, sourceID, updated.Visibility, documentIDs); err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	if documentationReview != nil {
		if err := insertSourcePublicationDocumentationReviewTx(ctx, tx, *documentationReview); err != nil {
			return model.Source{}, model.SourcePublication{}, err
		}
		transition, transitionErr := tx.Exec(ctx, `UPDATE developer_asset_ingestion_runs
			SET state='published',lease_owner='',lease_expires_at=NULL,heartbeat_at=NULL,finished_at=coalesce(finished_at,now())
			WHERE id=$1 AND deployment_id=$2 AND asset_kind='documentation' AND source_id=$3 AND target_id=$3 AND state='review_ready'`,
			typedRun.ID, productID, sourceID)
		if transitionErr != nil {
			return model.Source{}, model.SourcePublication{}, databaseError(transitionErr)
		}
		if transition.RowsAffected() != 1 {
			return model.Source{}, model.SourcePublication{}, ErrConflict
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Source{}, model.SourcePublication{}, err
	}
	return updated, publication, nil
}

func scanCrawlJob(row interface{ Scan(...any) error }) (model.CrawlJob, error) {
	var value model.CrawlJob
	err := row.Scan(
		&value.ID, &value.OrganisationID, &value.ProductID, &value.SourceID,
		&value.State, &value.Attempt, &value.DiscoveredCount, &value.FetchedCount,
		&value.ChangedCount, &value.FailedCount, &value.SkippedCount,
		&value.RedirectedCount, &value.LeaseOwner, &value.LeaseExpiresAt,
		&value.HeartbeatAt, &value.PipelineVersion, &value.RawManifestHash,
		&value.Diagnostics, &value.ErrorCode, &value.ErrorMessage, &value.QueuedAt,
		&value.StartedAt, &value.FinishedAt,
	)
	return value, databaseError(err)
}

const crawlJobSelect = `SELECT id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, failed_count, skipped_count, redirected_count, lease_owner, lease_expires_at, heartbeat_at, pipeline_version, raw_manifest_hash, diagnostics, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at FROM crawl_jobs`

func (p *Postgres) CrawlJobs(ctx context.Context, productID, sourceID string) ([]model.CrawlJob, error) {
	rows, err := p.pool.Query(ctx, crawlJobSelect+` WHERE product_id = $1 AND source_id = $2 ORDER BY queued_at DESC, id DESC LIMIT 50`, productID, sourceID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.CrawlJob, 0)
	for rows.Next() {
		value, err := scanCrawlJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type pgxRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func createCrawlJobWithSourceLock(ctx context.Context, query pgxRowQuerier, value model.CrawlJob) (model.CrawlJob, error) {
	var lockedSourceID string
	err := query.QueryRow(ctx, `
		SELECT id::text FROM sources
		WHERE id = $1 AND organisation_id = $2 AND product_id = $3
		FOR UPDATE`, value.SourceID, value.OrganisationID, value.ProductID).Scan(&lockedSourceID)
	if err != nil {
		return model.CrawlJob{}, databaseError(err)
	}
	return scanCrawlJob(query.QueryRow(ctx, `INSERT INTO crawl_jobs(id, organisation_id, product_id, source_id, state) VALUES ($1, $2, $3, $4, 'queued') RETURNING id::text, organisation_id::text, product_id::text, source_id::text, state, attempt, discovered_count, fetched_count, changed_count, failed_count, skipped_count, redirected_count, lease_owner, lease_expires_at, heartbeat_at, pipeline_version, raw_manifest_hash, diagnostics, coalesce(error_code, ''), coalesce(error_message, ''), queued_at, started_at, finished_at`, value.ID, value.OrganisationID, value.ProductID, value.SourceID))
}

func (p *Postgres) CreateCrawlJob(ctx context.Context, value model.CrawlJob) (model.CrawlJob, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.CrawlJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createCrawlJobWithSourceLock(ctx, tx, value)
	if err != nil {
		return model.CrawlJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.CrawlJob{}, err
	}
	return created, nil
}

func scanSecret(row pgx.Row) (model.Secret, error) {
	var value model.Secret
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Purpose, &value.Ciphertext, &value.Nonce, &value.KeyVersion, &value.Fingerprint, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) CreateSecret(ctx context.Context, value model.Secret) (model.Secret, error) {
	return scanSecret(p.pool.QueryRow(ctx, `INSERT INTO secrets(id, organisation_id, name, purpose, ciphertext, nonce, key_version, fingerprint) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text, organisation_id::text, name, purpose, ciphertext, nonce, key_version, fingerprint, created_at`, value.ID, value.OrganisationID, value.Name, value.Purpose, value.Ciphertext, value.Nonce, value.KeyVersion, value.Fingerprint))
}

func (p *Postgres) Secret(ctx context.Context, organisationID, id string) (model.Secret, error) {
	return scanSecret(p.pool.QueryRow(ctx, `SELECT id::text, organisation_id::text, name, purpose, ciphertext, nonce, key_version, fingerprint, created_at FROM secrets WHERE organisation_id = $1 AND id = $2`, organisationID, id))
}

func (p *Postgres) DeleteSecret(ctx context.Context, organisationID, id string) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM secrets WHERE organisation_id=$1 AND id=$2`, organisationID, id)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
