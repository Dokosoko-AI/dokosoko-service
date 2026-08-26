package store

import (
	"context"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func scanReportSubmission(row interface{ Scan(...any) error }) (model.ReportSubmission, error) {
	var value model.ReportSubmission
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.Kind, &value.State, &value.ActorPseudonym, &value.IdempotencyDigest, &value.Payload, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const reportSubmissionColumns = `id::text, organisation_id::text, product_id::text, coalesce(integration_id::text,''), kind, state, actor_pseudonym, idempotency_digest, payload, expires_at, created_at, updated_at`
const reportSubmissionSelect = `SELECT ` + reportSubmissionColumns + ` FROM report_submissions`

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
	return scanReportSubmission(p.pool.QueryRow(ctx, `INSERT INTO report_submissions(id,organisation_id,product_id,integration_id,kind,state,actor_pseudonym,idempotency_digest,payload,expires_at)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (product_id,actor_pseudonym,kind,idempotency_digest) DO UPDATE SET updated_at=report_submissions.updated_at
		RETURNING `+reportSubmissionColumns, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.Kind, value.State, value.ActorPseudonym, value.IdempotencyDigest, value.Payload, value.ExpiresAt))
}
