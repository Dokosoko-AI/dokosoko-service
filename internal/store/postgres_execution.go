package store

import (
	"context"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func scanReportSubmission(row interface{ Scan(...any) error }) (model.ReportSubmission, error) {
	var value model.ReportSubmission
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.Kind, &value.State, &value.DeliveryURL, &value.Attempts, &value.AvailableAt, &value.LeaseOwner, &value.LeasedUntil, &value.LastError, &value.DeliveredAt, &value.ActorPseudonym, &value.IdempotencyDigest, &value.Payload, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const reportSubmissionColumns = `submission.id::text, submission.organisation_id::text, submission.product_id::text, coalesce(submission.integration_id::text,''), submission.kind, submission.state, submission.delivery_url, submission.attempts, submission.available_at, submission.lease_owner, submission.leased_until, submission.last_error, submission.delivered_at, submission.actor_pseudonym, submission.idempotency_digest, submission.payload, submission.expires_at, submission.created_at, submission.updated_at`
const reportSubmissionSelect = `SELECT ` + reportSubmissionColumns + ` FROM report_submissions submission`

func (p *Postgres) ReportSubmissions(ctx context.Context, productID, startingAfter string, limit int) ([]model.ReportSubmission, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := reportSubmissionSelect + ` WHERE product_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`
	arguments := []any{productID, limit + 1}
	if startingAfter != "" {
		var cursorCreatedAt time.Time
		if err := p.pool.QueryRow(ctx, `SELECT created_at FROM report_submissions WHERE product_id=$1 AND id::text=$2`, productID, startingAfter).Scan(&cursorCreatedAt); err != nil {
			return nil, false, databaseError(err)
		}
		query = reportSubmissionSelect + ` WHERE product_id=$1 AND (created_at,id::text)<($3,$4) ORDER BY created_at DESC,id DESC LIMIT $2`
		arguments = append(arguments, cursorCreatedAt, startingAfter)
	}
	rows, err := p.pool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, false, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ReportSubmission, 0)
	for rows.Next() {
		value, scanErr := scanReportSubmission(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (p *Postgres) ReportSubmission(ctx context.Context, productID, id string) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, reportSubmissionSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateReportSubmission(ctx context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, `INSERT INTO report_submissions AS submission(id,organisation_id,product_id,integration_id,kind,state,delivery_url,available_at,actor_pseudonym,idempotency_digest,payload,expires_at)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,$5,'queued',$6,$7,$8,$9,$10,$11)
		ON CONFLICT (product_id,actor_pseudonym,kind,idempotency_digest) DO UPDATE SET updated_at=submission.updated_at
		RETURNING `+reportSubmissionColumns, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.Kind, value.DeliveryURL, value.AvailableAt, value.ActorPseudonym, value.IdempotencyDigest, value.Payload, value.ExpiresAt))
}

func (p *Postgres) ClaimReportSubmissions(ctx context.Context, owner string, leaseUntil time.Time, limit int) ([]model.ReportSubmission, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := p.pool.Query(ctx, `WITH candidates AS (
		SELECT id FROM report_submissions
		WHERE (state='queued' AND available_at<=now()) OR (state='delivering' AND leased_until<now())
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $1
	) UPDATE report_submissions submission
	SET state='delivering',lease_owner=$2,leased_until=$3,attempts=attempts+1,updated_at=now()
	FROM candidates WHERE submission.id=candidates.id RETURNING `+reportSubmissionColumns, limit, owner, leaseUntil)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.ReportSubmission, 0)
	for rows.Next() {
		value, scanErr := scanReportSubmission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, databaseError(rows.Err())
}

func (p *Postgres) CompleteReportSubmission(ctx context.Context, id, owner string, now time.Time) error {
	tag, err := p.pool.Exec(ctx, `UPDATE report_submissions SET state='delivered',lease_owner='',leased_until=NULL,last_error='',delivered_at=$3,updated_at=$3 WHERE id=$1 AND state='delivering' AND lease_owner=$2`, id, owner, now)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) RetryReportSubmission(ctx context.Context, id, owner string, availableAt time.Time, lastError string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE report_submissions SET state='queued',lease_owner='',leased_until=NULL,available_at=$3,last_error=$4,updated_at=now() WHERE id=$1 AND state='delivering' AND lease_owner=$2`, id, owner, availableAt, lastError)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) FailReportSubmission(ctx context.Context, id, owner, lastError string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE report_submissions SET state='failed',lease_owner='',leased_until=NULL,last_error=$3,updated_at=now() WHERE id=$1 AND state='delivering' AND lease_owner=$2`, id, owner, lastError)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

const authorizationUsageEventColumns = `event.id::text,event.organisation_id::text,event.product_id::text,event.integration_id::text,event.authorization_id::text,event.url,event.authentication_type,event.header_name,event.auth_config,event.credential_version_id::text,event.credential_secret_id::text,event.credential_fingerprint,event.payload,event.state,event.attempts,event.available_at,coalesce(event.lease_owner,''),event.leased_until,event.last_error,event.created_at,event.updated_at`

func scanAuthorizationUsageEvent(row interface{ Scan(...any) error }) (model.AuthorizationUsageEvent, error) {
	var value model.AuthorizationUsageEvent
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.AuthorizationID, &value.URL, &value.AuthenticationType, &value.HeaderName, &value.AuthConfig, &value.CredentialVersionID, &value.CredentialSecretID, &value.CredentialFingerprint, &value.Payload, &value.State, &value.Attempts, &value.AvailableAt, &value.LeaseOwner, &value.LeasedUntil, &value.LastError, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) CreateAuthorizationUsageEvent(ctx context.Context, value model.AuthorizationUsageEvent) (model.AuthorizationUsageEvent, error) {
	return scanAuthorizationUsageEvent(p.pool.QueryRow(ctx, `INSERT INTO authorization_usage_events AS event(id,organisation_id,product_id,integration_id,authorization_id,url,authentication_type,header_name,auth_config,credential_version_id,credential_secret_id,credential_fingerprint,payload,state,available_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'queued',$14)
		RETURNING `+authorizationUsageEventColumns, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.AuthorizationID, value.URL, value.AuthenticationType, value.HeaderName, value.AuthConfig, value.CredentialVersionID, value.CredentialSecretID, value.CredentialFingerprint, value.Payload, value.AvailableAt))
}

func (p *Postgres) ClaimAuthorizationUsageEvents(ctx context.Context, owner string, leaseUntil time.Time, limit int) ([]model.AuthorizationUsageEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := p.pool.Query(ctx, `WITH candidates AS (
		SELECT id FROM authorization_usage_events
		WHERE (state='queued' AND available_at<=now()) OR (state='delivering' AND leased_until<now())
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $1
	) UPDATE authorization_usage_events event
	SET state='delivering',lease_owner=$2,leased_until=$3,attempts=attempts+1,updated_at=now()
	FROM candidates WHERE event.id=candidates.id RETURNING `+authorizationUsageEventColumns, limit, owner, leaseUntil)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.AuthorizationUsageEvent, 0)
	for rows.Next() {
		value, scanErr := scanAuthorizationUsageEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, databaseError(rows.Err())
}

func (p *Postgres) CompleteAuthorizationUsageEvent(ctx context.Context, id, owner string, now time.Time) error {
	tag, err := p.pool.Exec(ctx, `UPDATE authorization_usage_events SET state='delivered',lease_owner='',leased_until=NULL,last_error='',updated_at=$3 WHERE id=$1 AND state='delivering' AND lease_owner=$2`, id, owner, now)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) RetryAuthorizationUsageEvent(ctx context.Context, id, owner string, availableAt time.Time, lastError string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE authorization_usage_events SET state='queued',lease_owner='',leased_until=NULL,available_at=$3,last_error=$4,updated_at=now() WHERE id=$1 AND state='delivering' AND lease_owner=$2`, id, owner, availableAt, lastError)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}
