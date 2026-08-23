package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

func scanAIProviderConnection(row interface{ Scan(...any) error }) (model.AIProviderConnection, error) {
	var value model.AIProviderConnection
	err := row.Scan(&value.ID, &value.OrganisationID, &value.DeploymentID, &value.Provider, &value.Endpoint, &value.CredentialID, &value.ManagedBy, &value.Enabled, &value.IsBackup, &value.BackupModels, &value.LastTestedAt, &value.LastErrorCode, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const aiProviderConnectionColumns = `id::text,organisation_id::text,deployment_id::text,provider,endpoint,coalesce(credential_secret_id::text,''),managed_by,enabled,is_backup,backup_models,last_tested_at,last_error_code,revision,created_at,updated_at`

func (p *Postgres) AIProviderConnections(ctx context.Context, deploymentID string) ([]model.AIProviderConnection, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+aiProviderConnectionColumns+` FROM ai_provider_connections WHERE deployment_id=$1 ORDER BY provider`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AIProviderConnection, 0)
	for rows.Next() {
		value, scanErr := scanAIProviderConnection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AIProviderConnection(ctx context.Context, deploymentID, id string) (model.AIProviderConnection, error) {
	return scanAIProviderConnection(p.pool.QueryRow(ctx, `SELECT `+aiProviderConnectionColumns+` FROM ai_provider_connections WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) SaveAIProviderConnection(ctx context.Context, value model.AIProviderConnection, expectedRevision int64) (model.AIProviderConnection, error) {
	if len(value.BackupModels) == 0 {
		value.BackupModels = json.RawMessage(`{}`)
	}
	updated, err := scanAIProviderConnection(p.pool.QueryRow(ctx, `INSERT INTO ai_provider_connections(id,organisation_id,deployment_id,provider,endpoint,credential_secret_id,managed_by,enabled,is_backup,backup_models,last_tested_at,last_error_code) VALUES ($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (deployment_id,provider) DO UPDATE SET endpoint=excluded.endpoint,credential_secret_id=excluded.credential_secret_id,managed_by=excluded.managed_by,enabled=excluded.enabled,is_backup=excluded.is_backup,backup_models=excluded.backup_models,last_tested_at=excluded.last_tested_at,last_error_code=excluded.last_error_code,revision=ai_provider_connections.revision+1,updated_at=now()
		WHERE ai_provider_connections.revision=$13
		RETURNING `+aiProviderConnectionColumns, value.ID, value.OrganisationID, value.DeploymentID, value.Provider, value.Endpoint, value.CredentialID, value.ManagedBy, value.Enabled, value.IsBackup, value.BackupModels, value.LastTestedAt, value.LastErrorCode, expectedRevision))
	if errors.Is(err, ErrNotFound) {
		connections, lookupErr := p.AIProviderConnections(ctx, value.DeploymentID)
		if lookupErr == nil {
			for _, connection := range connections {
				if connection.Provider == value.Provider {
					return model.AIProviderConnection{}, ErrConflict
				}
			}
		}
	}
	return updated, err
}

func scanAIWorkloadProfile(row interface{ Scan(...any) error }) (model.AIWorkloadProfile, error) {
	var value model.AIWorkloadProfile
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Workload, &value.ProviderConnectionID, &value.Model, &value.MaxInputTokens, &value.MaxOutputTokens, &value.DailyTokenBudget, &value.Hardening, &value.Enabled, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const aiWorkloadProfileColumns = `id::text,organisation_id::text,product_id::text,workload,provider_connection_id::text,model,max_input_tokens,max_output_tokens,daily_token_budget,hardening,enabled,revision,created_at,updated_at`

func (p *Postgres) AIWorkloadProfiles(ctx context.Context, productID string) ([]model.AIWorkloadProfile, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+aiWorkloadProfileColumns+` FROM ai_workload_profiles WHERE product_id=$1 ORDER BY workload`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AIWorkloadProfile, 0)
	for rows.Next() {
		value, scanErr := scanAIWorkloadProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AIWorkloadProfile(ctx context.Context, productID, workload string) (model.AIWorkloadProfile, error) {
	return scanAIWorkloadProfile(p.pool.QueryRow(ctx, `SELECT `+aiWorkloadProfileColumns+` FROM ai_workload_profiles WHERE product_id=$1 AND workload=$2`, productID, workload))
}

func (p *Postgres) SaveAIWorkloadProfile(ctx context.Context, value model.AIWorkloadProfile, expectedRevision int64) (model.AIWorkloadProfile, error) {
	updated, err := scanAIWorkloadProfile(p.pool.QueryRow(ctx, `INSERT INTO ai_workload_profiles(id,organisation_id,product_id,workload,provider_connection_id,model,max_input_tokens,max_output_tokens,daily_token_budget,hardening,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (product_id,workload) DO UPDATE SET provider_connection_id=excluded.provider_connection_id,model=excluded.model,max_input_tokens=excluded.max_input_tokens,max_output_tokens=excluded.max_output_tokens,daily_token_budget=excluded.daily_token_budget,hardening=excluded.hardening,enabled=excluded.enabled,revision=ai_workload_profiles.revision+1,updated_at=now()
		WHERE ai_workload_profiles.revision=$12
		RETURNING `+aiWorkloadProfileColumns, value.ID, value.OrganisationID, value.ProductID, value.Workload, value.ProviderConnectionID, value.Model, value.MaxInputTokens, value.MaxOutputTokens, value.DailyTokenBudget, value.Hardening, value.Enabled, expectedRevision))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.AIWorkloadProfile(ctx, value.ProductID, value.Workload); lookupErr == nil {
			return model.AIWorkloadProfile{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) ReserveAIBudget(ctx context.Context, value model.AIBudgetReservation, dailyBudget int64) (bool, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `DELETE FROM ai_budget_reservations WHERE expires_at<=now()`); err != nil {
		return false, databaseError(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_budget_days(product_id,workload,day) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, value.ProductID, value.Workload, value.Day); err != nil {
		return false, databaseError(err)
	}
	var used, reserved int64
	if err = tx.QueryRow(ctx, `SELECT used_tokens FROM ai_budget_days WHERE product_id=$1 AND workload=$2 AND day=$3 FOR UPDATE`, value.ProductID, value.Workload, value.Day).Scan(&used); err != nil {
		return false, databaseError(err)
	}
	if err = tx.QueryRow(ctx, `SELECT coalesce(sum(reserved_tokens),0) FROM ai_budget_reservations WHERE product_id=$1 AND workload=$2 AND day=$3 AND expires_at>now()`, value.ProductID, value.Workload, value.Day).Scan(&reserved); err != nil {
		return false, databaseError(err)
	}
	if dailyBudget > 0 && used+reserved+value.ReservedTokens > dailyBudget {
		if err = tx.Commit(ctx); err != nil {
			return false, databaseError(err)
		}
		return false, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_budget_reservations(id,product_id,workload,day,reserved_tokens,expires_at) VALUES ($1,$2,$3,$4,$5,$6)`, value.ID, value.ProductID, value.Workload, value.Day, value.ReservedTokens, value.ExpiresAt); err != nil {
		return false, databaseError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, databaseError(err)
	}
	return true, nil
}

func (p *Postgres) FinishAIUsage(ctx context.Context, reservationID string, event model.AIUsageEvent) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var productID, workload string
	var day time.Time
	if err = tx.QueryRow(ctx, `DELETE FROM ai_budget_reservations WHERE id=$1 RETURNING product_id::text,workload,day`, reservationID).Scan(&productID, &workload, &day); err != nil {
		return databaseError(err)
	}
	actual := event.InputTokens + event.OutputTokens
	if actual < 0 {
		actual = 0
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_budget_days(product_id,workload,day,used_tokens) VALUES ($1,$2,$3,$4) ON CONFLICT (product_id,workload,day) DO UPDATE SET used_tokens=ai_budget_days.used_tokens+excluded.used_tokens,updated_at=now()`, productID, workload, day, actual); err != nil {
		return databaseError(err)
	}
	if event.DurationMS == 0 && event.Duration > 0 {
		event.DurationMS = event.Duration.Milliseconds()
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_usage_events(id,organisation_id,product_id,workload,action,provider,provider_role,fallback_reason,requested_model,resolved_model,provider_request_id,input_tokens,output_tokens,duration_ms,outcome,error_code,prompt_version,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, event.ID, event.OrganisationID, event.ProductID, event.Workload, event.Action, event.Provider, event.ProviderRole, event.FallbackReason, event.RequestedModel, event.ResolvedModel, event.ProviderRequestID, event.InputTokens, event.OutputTokens, event.DurationMS, event.Outcome, event.ErrorCode, event.PromptVersion, event.CreatedAt); err != nil {
		return databaseError(err)
	}
	return databaseError(tx.Commit(ctx))
}

func scanAIUsageEvent(row interface{ Scan(...any) error }) (model.AIUsageEvent, error) {
	var value model.AIUsageEvent
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Workload, &value.Action, &value.Provider, &value.ProviderRole, &value.FallbackReason, &value.RequestedModel, &value.ResolvedModel, &value.ProviderRequestID, &value.InputTokens, &value.OutputTokens, &value.DurationMS, &value.Outcome, &value.ErrorCode, &value.PromptVersion, &value.CreatedAt)
	value.Duration = time.Duration(value.DurationMS) * time.Millisecond
	return value, databaseError(err)
}

func (p *Postgres) AIUsageEvents(ctx context.Context, productID string, since time.Time) ([]model.AIUsageEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,organisation_id::text,product_id::text,workload,action,provider,provider_role,fallback_reason,requested_model,resolved_model,provider_request_id,input_tokens,output_tokens,duration_ms,outcome,error_code,prompt_version,created_at FROM ai_usage_events WHERE product_id=$1 AND created_at>=$2 ORDER BY created_at DESC`, productID, since)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AIUsageEvent, 0)
	for rows.Next() {
		value, scanErr := scanAIUsageEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
