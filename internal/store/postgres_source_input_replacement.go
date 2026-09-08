package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const sourceInputSourceSelect = `SELECT id::text,organisation_id::text,product_id::text,name,kind,location,visibility::text,state::text,revision,created_at,updated_at FROM sources`

func sourceInputReplacementResult(ctx context.Context, query pgxRowQuerier, productID, sourceID, jobID string) (SourceInputReplacementResult, error) {
	source, err := scanSource(query.QueryRow(ctx, sourceInputSourceSelect+` WHERE product_id=$1 AND id=$2`, productID, sourceID))
	if err != nil {
		return SourceInputReplacementResult{}, err
	}
	job, err := scanCrawlJob(query.QueryRow(ctx, crawlJobSelect+` WHERE product_id=$1 AND source_id=$2 AND id=$3`, productID, sourceID, jobID))
	if err != nil {
		return SourceInputReplacementResult{}, err
	}
	return SourceInputReplacementResult{Source: source, CrawlJob: job}, nil
}

func (p *Postgres) SourceInputReplacement(ctx context.Context, productID, sourceID, requestDigest string) (SourceInputReplacementResult, error) {
	var jobID string
	err := p.pool.QueryRow(ctx, `SELECT crawl_job_id::text FROM source_input_replacements WHERE product_id=$1 AND source_id=$2 AND request_digest=$3`, productID, sourceID, requestDigest).Scan(&jobID)
	if err != nil {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	return sourceInputReplacementResult(ctx, p.pool, productID, sourceID, jobID)
}

func (p *Postgres) ReplaceSourceInput(ctx context.Context, input SourceInputReplacement) (SourceInputReplacementResult, error) {
	if err := validateSourceInputReplacement(input); err != nil {
		return SourceInputReplacementResult{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// QueueCrawl uses this same source lock. The worker can only claim a queued
	// job; replacements reject every active job, including expired worker leases.
	source, err := scanSource(tx.QueryRow(ctx, sourceInputSourceSelect+` WHERE product_id=$1 AND id=$2 AND organisation_id=$3 FOR UPDATE`, input.ProductID, input.SourceID, input.Audit.OrganisationID))
	if err != nil {
		return SourceInputReplacementResult{}, err
	}
	var previous sourceInputReplacementRecord
	err = tx.QueryRow(ctx, `SELECT input_digest,expected_revision,crawl_job_id::text FROM source_input_replacements WHERE product_id=$1 AND source_id=$2 AND request_digest=$3`, input.ProductID, input.SourceID, input.RequestDigest).Scan(&previous.InputDigest, &previous.ExpectedRevision, &previous.CrawlJobID)
	if err == nil {
		if previous.InputDigest != input.InputDigest || previous.ExpectedRevision != input.ExpectedRevision {
			return SourceInputReplacementResult{}, ErrSourceInputChanged
		}
		return sourceInputReplacementResult(ctx, tx, input.ProductID, input.SourceID, previous.CrawlJobID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	if source.Kind != "upload" || source.Revision != input.ExpectedRevision {
		return SourceInputReplacementResult{}, ErrSourceInputChanged
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crawl_jobs WHERE product_id=$1 AND source_id=$2 AND state IN ('queued','running'))`, source.ProductID, source.ID).Scan(&active); err != nil {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	if active {
		return SourceInputReplacementResult{}, ErrSourceImportActive
	}
	// Only the future input and optimistic revision change. The crawler must
	// independently clear quarantine, and historical evidence stays immutable.
	source, err = scanSource(tx.QueryRow(ctx, `UPDATE sources SET location=$3,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 RETURNING id::text,organisation_id::text,product_id::text,name,kind,location,visibility::text,state::text,revision,created_at,updated_at`, input.ProductID, input.SourceID, input.Location))
	if err != nil {
		return SourceInputReplacementResult{}, err
	}
	job, err := scanCrawlJob(tx.QueryRow(ctx, `INSERT INTO crawl_jobs(id,organisation_id,product_id,source_id,state) VALUES($1,$2,$3,$4,'queued') RETURNING id::text,organisation_id::text,product_id::text,source_id::text,state,attempt,discovered_count,fetched_count,changed_count,failed_count,skipped_count,redirected_count,lease_owner,lease_expires_at,heartbeat_at,pipeline_version,raw_manifest_hash,diagnostics,coalesce(error_code,''),coalesce(error_message,''),queued_at,started_at,finished_at`, input.CrawlJobID, source.OrganisationID, source.ProductID, source.ID))
	if err != nil {
		return SourceInputReplacementResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO source_input_replacements(product_id,source_id,request_digest,input_digest,expected_revision,crawl_job_id) VALUES($1,$2,$3,$4,$5,$6)`, source.ProductID, source.ID, input.RequestDigest, input.InputDigest, input.ExpectedRevision, job.ID); err != nil {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	prior, _ := json.Marshal(map[string]any{"source_revision": input.ExpectedRevision})
	current, _ := json.Marshal(map[string]any{"source_revision": source.Revision, "crawl_job_id": job.ID, "visibility": source.Visibility})
	event := input.Audit
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(event_key,organisation_id,product_id,actor_id,actor_kind,action,target_type,target_id,prior,current,request_id,outcome,created_at) VALUES($1,$2,$3,$4,'root',$5,'source',$6,$7,$8,$9,'success',$10)`, event.ID, source.OrganisationID, source.ProductID, event.ActorID, event.Action, source.ID, prior, current, event.RequestID, event.CreatedAt); err != nil {
		return SourceInputReplacementResult{}, databaseError(err)
	}
	// Retain the newly allocated upload on an uncertain commit outcome.
	return SourceInputReplacementResult{Source: source, CrawlJob: job}, tx.Commit(ctx)
}
