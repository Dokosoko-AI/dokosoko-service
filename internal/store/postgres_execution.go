package store

import (
	"context"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
	"time"
)

func scanProvider(row pgx.Row) (model.Provider, error) {
	var value model.Provider
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Kind, &value.BaseURL, &value.CredentialID, &value.Config, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const providerSelect = `SELECT id::text, organisation_id::text, product_id::text, name, kind, coalesce(base_url,''), coalesce(credential_secret_id::text,''), config, revision, created_at, updated_at FROM providers`

func (p *Postgres) Providers(ctx context.Context, productID string) ([]model.Provider, error) {
	rows, err := p.pool.Query(ctx, providerSelect+` WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Provider, 0)
	for rows.Next() {
		value, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Provider(ctx context.Context, productID, id string) (model.Provider, error) {
	return scanProvider(p.pool.QueryRow(ctx, providerSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateProvider(ctx context.Context, value model.Provider) (model.Provider, error) {
	return scanProvider(p.pool.QueryRow(ctx, `INSERT INTO providers(id,organisation_id,product_id,name,kind,base_url,credential_secret_id,config) VALUES ($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,$8) RETURNING id::text, organisation_id::text, product_id::text, name, kind, coalesce(base_url,''), coalesce(credential_secret_id::text,''), config, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Kind, value.BaseURL, value.CredentialID, value.Config))
}

func scanProject(row pgx.Row) (model.Project, error) {
	var value model.Project
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.ProviderID, &value.OwnerType, &value.OwnerID, &value.ExternalID, &value.IdempotencyKey, &value.State, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const projectSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, provider_id::text, owner_type, owner_id, external_id, idempotency_key, state, expires_at, created_at, updated_at FROM projects`

func (p *Postgres) Projects(ctx context.Context, productID string) ([]model.Project, error) {
	rows, err := p.pool.Query(ctx, projectSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Project, 0)
	for rows.Next() {
		value, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) Project(ctx context.Context, productID, id string) (model.Project, error) {
	return scanProject(p.pool.QueryRow(ctx, projectSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateProject(ctx context.Context, value model.Project) (model.Project, error) {
	return scanProject(p.pool.QueryRow(ctx, `INSERT INTO projects(id,organisation_id,product_id,environment_id,provider_id,owner_type,owner_id,external_id,idempotency_key,state,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (provider_id,idempotency_key) DO UPDATE SET updated_at=projects.updated_at RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, provider_id::text, owner_type, owner_id, external_id, idempotency_key, state, expires_at, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.ProviderID, value.OwnerType, value.OwnerID, value.ExternalID, value.IdempotencyKey, value.State, value.ExpiresAt))
}

func scanCredentialLease(row pgx.Row) (model.CredentialLease, error) {
	var value model.CredentialLease
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.ProjectID, &value.ProviderID, &value.SubjectID, &value.ExternalID, &value.IdempotencyKey, &value.Scopes, &value.SecretFingerprint, &value.ExpiresAt, &value.RevokedAt, &value.CreatedAt)
	return value, databaseError(err)
}

const credentialLeaseSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at FROM credential_leases`

func (p *Postgres) CredentialLeases(ctx context.Context, productID string) ([]model.CredentialLease, error) {
	rows, err := p.pool.Query(ctx, credentialLeaseSelect+` WHERE product_id=$1 ORDER BY created_at DESC`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.CredentialLease, 0)
	for rows.Next() {
		value, err := scanCredentialLease(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CredentialLease(ctx context.Context, productID, id string) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, credentialLeaseSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateCredentialLease(ctx context.Context, value model.CredentialLease) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, `INSERT INTO credential_leases(id,organisation_id,product_id,environment_id,project_id,provider_id,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,expires_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (provider_id,idempotency_key) WHERE idempotency_key <> '' DO UPDATE SET idempotency_key=credential_leases.idempotency_key RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.ProjectID, value.ProviderID, value.SubjectID, value.ExternalID, value.IdempotencyKey, value.Scopes, value.SecretFingerprint, value.ExpiresAt))
}

func (p *Postgres) RevokeCredentialLease(ctx context.Context, productID, id string, revokedAt time.Time) (model.CredentialLease, error) {
	return scanCredentialLease(p.pool.QueryRow(ctx, `UPDATE credential_leases SET revoked_at=$3 WHERE product_id=$1 AND id=$2 AND revoked_at IS NULL RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(project_id::text,''), provider_id::text, subject_id, external_id, idempotency_key, scopes, secret_fingerprint, expires_at, revoked_at, created_at`, productID, id, revokedAt))
}

func scanIntegrationRun(row interface{ Scan(...any) error }) (model.IntegrationRun, error) {
	var value model.IntegrationRun
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.EnvironmentID, &value.UserID, &value.ActorPseudonym, &value.RequestedOutcome, &value.State, &value.ReportedSuccess, &value.ValidatedSuccess, &value.FailureCode, &value.StartedAt, &value.FinishedAt)
	return value, databaseError(err)
}

const integrationRunSelect = `SELECT id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at FROM integration_runs`

func (p *Postgres) IntegrationRuns(ctx context.Context, productID string) ([]model.IntegrationRun, error) {
	rows, err := p.pool.Query(ctx, integrationRunSelect+` WHERE product_id=$1 ORDER BY started_at DESC LIMIT 500`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.IntegrationRun, 0)
	for rows.Next() {
		value, err := scanIntegrationRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) IntegrationRun(ctx context.Context, productID, id string) (model.IntegrationRun, error) {
	return scanIntegrationRun(p.pool.QueryRow(ctx, integrationRunSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateIntegrationRun(ctx context.Context, value model.IntegrationRun) (model.IntegrationRun, error) {
	return scanIntegrationRun(p.pool.QueryRow(ctx, `INSERT INTO integration_runs(id,organisation_id,product_id,environment_id,user_id,actor_pseudonym,requested_outcome,state,started_at) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,'running',$8) RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at`, value.ID, value.OrganisationID, value.ProductID, value.EnvironmentID, value.UserID, value.ActorPseudonym, value.RequestedOutcome, value.StartedAt))
}

func (p *Postgres) CompleteIntegrationRun(ctx context.Context, productID, id string, reported, validated *bool, failureCode string, finishedAt time.Time) (model.IntegrationRun, error) {
	state := "failed"
	if validated != nil && *validated {
		state = "succeeded"
	}
	value, err := scanIntegrationRun(p.pool.QueryRow(ctx, `UPDATE integration_runs SET state=$3, reported_success=$4, validated_success=$5, failure_code=nullif($6,''), finished_at=$7 WHERE product_id=$1 AND id=$2 AND finished_at IS NULL RETURNING id::text, organisation_id::text, product_id::text, environment_id::text, coalesce(user_id::text,''), actor_pseudonym, requested_outcome, state, reported_success, validated_success, coalesce(failure_code,''), started_at, finished_at`, productID, id, state, reported, validated, failureCode, finishedAt))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.IntegrationRun(ctx, productID, id); lookupErr == nil {
			return model.IntegrationRun{}, ErrConflict
		}
	}
	return value, err
}

func scanReportSubmission(row interface{ Scan(...any) error }) (model.ReportSubmission, error) {
	var value model.ReportSubmission
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.IntegrationID, &value.IntegrationSnapshot, &value.SupportRouteID, &value.Kind, &value.State, &value.ActorPseudonym, &value.IdempotencyDigest, &value.PayloadCiphertext, &value.PayloadNonce, &value.PayloadKeyVersion, &value.PayloadFingerprint, &value.Attempts, &value.NextAttemptAt, &value.DeliveryStartedAt, &value.LastError, &value.ExternalID, &value.ExternalURL, &value.DeliveredAt, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const reportSubmissionColumns = `id::text, organisation_id::text, product_id::text, coalesce(integration_id::text,''), integration_snapshot, coalesce(support_route_id::text,''), kind, state, actor_pseudonym, idempotency_digest, payload_ciphertext, payload_nonce, payload_key_version, payload_fingerprint, attempts, next_attempt_at, delivery_started_at, last_error, external_id, external_url, delivered_at, expires_at, created_at, updated_at`
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
	return scanReportSubmission(p.pool.QueryRow(ctx, `INSERT INTO report_submissions(id,organisation_id,product_id,integration_id,integration_snapshot,support_route_id,kind,state,actor_pseudonym,idempotency_digest,payload_ciphertext,payload_nonce,payload_key_version,payload_fingerprint,next_attempt_at,expires_at)
		VALUES ($1,$2,$3,nullif($4,'')::uuid,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (product_id,actor_pseudonym,kind,idempotency_digest) DO UPDATE SET updated_at=report_submissions.updated_at
		RETURNING `+reportSubmissionColumns, value.ID, value.OrganisationID, value.ProductID, value.IntegrationID, value.IntegrationSnapshot, value.SupportRouteID, value.Kind, value.State, value.ActorPseudonym, value.IdempotencyDigest, value.PayloadCiphertext, value.PayloadNonce, value.PayloadKeyVersion, value.PayloadFingerprint, value.NextAttemptAt, value.ExpiresAt))
}

func (p *Postgres) ActivateHeldReportSubmissions(ctx context.Context, productID, routeID, kind string, now time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE report_submissions SET state='pending', next_attempt_at=$4, updated_at=$4 WHERE product_id=$1 AND support_route_id=$2 AND kind=$3 AND state='held' AND expires_at>$4`, productID, routeID, kind, now)
	return databaseError(err)
}

func (p *Postgres) ClaimReportSubmissions(ctx context.Context, now time.Time, limit int) ([]model.ReportSubmission, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := p.pool.Query(ctx, `WITH ready AS (
		SELECT id FROM report_submissions
		WHERE expires_at>$1 AND ((state='pending' AND (next_attempt_at IS NULL OR next_attempt_at<=$1)) OR (state='delivering' AND delivery_started_at<$1-interval '5 minutes'))
		ORDER BY coalesce(next_attempt_at,created_at), created_at
		FOR UPDATE SKIP LOCKED LIMIT $2
	) UPDATE report_submissions s SET state='delivering', attempts=s.attempts+1, delivery_started_at=$1, updated_at=$1 FROM ready WHERE s.id=ready.id
	RETURNING s.id::text, s.organisation_id::text, s.product_id::text, coalesce(s.integration_id::text,''), s.integration_snapshot, coalesce(s.support_route_id::text,''), s.kind, s.state, s.actor_pseudonym, s.idempotency_digest, s.payload_ciphertext, s.payload_nonce, s.payload_key_version, s.payload_fingerprint, s.attempts, s.next_attempt_at, s.delivery_started_at, s.last_error, s.external_id, s.external_url, s.delivered_at, s.expires_at, s.created_at, s.updated_at`, now, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ReportSubmission, 0)
	for rows.Next() {
		value, scanErr := scanReportSubmission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) UpdateReportSubmissionDelivery(ctx context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	return scanReportSubmission(p.pool.QueryRow(ctx, `UPDATE report_submissions SET state=$3, attempts=$4, next_attempt_at=$5, delivery_started_at=$6, last_error=$7, external_id=$8, external_url=$9, delivered_at=$10, updated_at=now() WHERE product_id=$1 AND id=$2
	RETURNING `+reportSubmissionColumns, value.ProductID, value.ID, value.State, value.Attempts, value.NextAttemptAt, value.DeliveryStartedAt, value.LastError, value.ExternalID, value.ExternalURL, value.DeliveredAt))
}

func (p *Postgres) RetryReportSubmission(ctx context.Context, productID, id string, now time.Time) (model.ReportSubmission, error) {
	value, err := scanReportSubmission(p.pool.QueryRow(ctx, `UPDATE report_submissions SET state='pending', next_attempt_at=$3, delivery_started_at=NULL, last_error='', updated_at=$3 WHERE product_id=$1 AND id=$2 AND state IN ('held','failed') AND expires_at>$3
	RETURNING `+reportSubmissionColumns, productID, id, now))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.ReportSubmission(ctx, productID, id); lookupErr == nil {
			return model.ReportSubmission{}, ErrConflict
		}
	}
	return value, err
}

func (p *Postgres) DeleteExpiredReportSubmissions(ctx context.Context, now time.Time) (int64, error) {
	result, err := p.pool.Exec(ctx, `DELETE FROM report_submissions WHERE expires_at<=$1`, now)
	if err != nil {
		return 0, databaseError(err)
	}
	return result.RowsAffected(), nil
}

func scanLLMProfile(row pgx.Row) (model.LLMProfile, error) {
	var value model.LLMProfile
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Role, &value.Provider, &value.Endpoint, &value.Model, &value.CredentialID, &value.EmbeddingDimensions, &value.MaxInputTokens, &value.MaxOutputTokens, &value.DailyTokenBudget, &value.Hardening, &value.Enabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const llmProfileSelect = `SELECT id::text, organisation_id::text, product_id::text, role, provider, endpoint, model, coalesce(credential_secret_id::text,''), coalesce(embedding_dimensions,0), max_input_tokens, max_output_tokens, daily_token_budget, hardening, enabled, revision, created_at, updated_at FROM llm_profiles`

func (p *Postgres) LLMProfiles(ctx context.Context, productID string) ([]model.LLMProfile, error) {
	rows, err := p.pool.Query(ctx, llmProfileSelect+` WHERE product_id=$1 ORDER BY role`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.LLMProfile, 0)
	for rows.Next() {
		value, err := scanLLMProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SaveLLMProfile(ctx context.Context, value model.LLMProfile) (model.LLMProfile, error) {
	return scanLLMProfile(p.pool.QueryRow(ctx, `INSERT INTO llm_profiles(id,organisation_id,product_id,role,provider,endpoint,model,credential_secret_id,embedding_dimensions,max_input_tokens,max_output_tokens,daily_token_budget,hardening,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,nullif($9,0),$10,$11,$12,$13,$14) ON CONFLICT (product_id,role) DO UPDATE SET provider=excluded.provider,endpoint=excluded.endpoint,model=excluded.model,credential_secret_id=excluded.credential_secret_id,embedding_dimensions=excluded.embedding_dimensions,max_input_tokens=excluded.max_input_tokens,max_output_tokens=excluded.max_output_tokens,daily_token_budget=excluded.daily_token_budget,hardening=excluded.hardening,enabled=excluded.enabled,revision=llm_profiles.revision+1,updated_at=now() RETURNING id::text, organisation_id::text, product_id::text, role, provider, endpoint, model, coalesce(credential_secret_id::text,''), coalesce(embedding_dimensions,0), max_input_tokens, max_output_tokens, daily_token_budget, hardening, enabled, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Role, value.Provider, value.Endpoint, value.Model, value.CredentialID, value.EmbeddingDimensions, value.MaxInputTokens, value.MaxOutputTokens, value.DailyTokenBudget, value.Hardening, value.Enabled))
}
