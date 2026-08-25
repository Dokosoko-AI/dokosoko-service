package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const backendConnectionSelect = `SELECT connection.id::text,connection.deployment_id::text,connection.organisation_id::text,connection.name,connection.base_url,connection.authentication_type,coalesce(connection.credential_secret_id::text,''),coalesce(secret.fingerprint,''),connection.state,connection.revision,connection.created_at,connection.updated_at FROM backend_connections connection LEFT JOIN secrets secret ON secret.id=connection.credential_secret_id`

func scanBackendConnection(row pgx.Row) (model.BackendConnection, error) {
	var value model.BackendConnection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.BaseURL, &value.AuthenticationType, &value.CredentialSecretID, &value.CredentialFingerprint, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) BackendConnections(ctx context.Context, deploymentID string) ([]model.BackendConnection, error) {
	rows, err := p.pool.Query(ctx, backendConnectionSelect+` WHERE connection.deployment_id=$1 ORDER BY connection.name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.BackendConnection, 0)
	for rows.Next() {
		value, err := scanBackendConnection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) BackendConnection(ctx context.Context, deploymentID, id string) (model.BackendConnection, error) {
	return scanBackendConnection(p.pool.QueryRow(ctx, backendConnectionSelect+` WHERE connection.deployment_id=$1 AND connection.id=$2`, deploymentID, id))
}

func (p *Postgres) CreateBackendConnection(ctx context.Context, value model.BackendConnection) (model.BackendConnection, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO backend_connections(id,deployment_id,organisation_id,name,base_url,authentication_type,credential_secret_id,state) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid,$8)`, value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.BaseURL, value.AuthenticationType, value.CredentialSecretID, value.State)
	if err != nil {
		return model.BackendConnection{}, databaseError(err)
	}
	return p.BackendConnection(ctx, value.DeploymentID, value.ID)
}

func (p *Postgres) UpdateBackendConnection(ctx context.Context, value model.BackendConnection, expected int64) (model.BackendConnection, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE backend_connections SET name=$3,base_url=$4,authentication_type=$5,credential_secret_id=nullif($6,'')::uuid,state=$7,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$8`, value.DeploymentID, value.ID, value.Name, value.BaseURL, value.AuthenticationType, value.CredentialSecretID, value.State, expected)
	if err != nil {
		return model.BackendConnection{}, databaseError(err)
	}
	if tag.RowsAffected() == 0 {
		if _, lookupErr := p.BackendConnection(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.BackendConnection{}, ErrConflict
		}
		return model.BackendConnection{}, ErrNotFound
	}
	return p.BackendConnection(ctx, value.DeploymentID, value.ID)
}

const supportRouteSelect = `SELECT id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,coalesce(backend_connection_id::text,''),retention_days,state,revision,created_at,updated_at FROM support_routes`

func scanSupportRoute(row pgx.Row) (model.SupportRoute, error) {
	var value model.SupportRoute
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.IsDefault, &value.BugReportsEnabled, &value.FeedbackEnabled, &value.BackendConnectionID, &value.RetentionDays, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichSupportRoute(ctx context.Context, value model.SupportRoute) (model.SupportRoute, error) {
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_support_bindings WHERE support_route_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.SupportRoute{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) SupportRoutes(ctx context.Context, deploymentID string) ([]model.SupportRoute, error) {
	rows, err := p.pool.Query(ctx, supportRouteSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.SupportRoute, 0)
	for rows.Next() {
		value, err := scanSupportRoute(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.SupportRoute, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichSupportRoute(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) SupportRoute(ctx context.Context, deploymentID, id string) (model.SupportRoute, error) {
	value, err := scanSupportRoute(p.pool.QueryRow(ctx, supportRouteSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, value)
}

func (p *Postgres) SupportRouteForIntegration(ctx context.Context, deploymentID, integrationID string) (model.SupportRoute, error) {
	value, err := scanSupportRoute(p.pool.QueryRow(ctx, supportRouteSelect+` route WHERE route.deployment_id=$1 AND route.state='active' AND (EXISTS(SELECT 1 FROM integration_support_bindings binding WHERE binding.support_route_id=route.id AND binding.integration_id=nullif($2,'')::uuid) OR (route.is_default AND NOT EXISTS(SELECT 1 FROM integration_support_bindings binding WHERE binding.integration_id=nullif($2,'')::uuid))) ORDER BY route.is_default LIMIT 1`, deploymentID, integrationID))
	if err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, value)
}

func (p *Postgres) SaveSupportRoute(ctx context.Context, value model.SupportRoute, expected int64) (model.SupportRoute, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SupportRoute{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if value.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE support_routes SET is_default=false,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id<>$2 AND is_default`, value.DeploymentID, value.ID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
	}
	var saved model.SupportRoute
	if expected == 0 {
		saved, err = scanSupportRoute(tx.QueryRow(ctx, `INSERT INTO support_routes(id,deployment_id,organisation_id,name,is_default,bug_reports_enabled,feedback_enabled,backend_connection_id,retention_days,state) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,$9,$10) RETURNING id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,coalesce(backend_connection_id::text,''),retention_days,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.IsDefault, value.BugReportsEnabled, value.FeedbackEnabled, value.BackendConnectionID, value.RetentionDays, value.State))
	} else {
		saved, err = scanSupportRoute(tx.QueryRow(ctx, `UPDATE support_routes SET name=$3,is_default=$4,bug_reports_enabled=$5,feedback_enabled=$6,backend_connection_id=nullif($7,'')::uuid,retention_days=$8,state=$9,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$10 RETURNING id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,coalesce(backend_connection_id::text,''),retention_days,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.IsDefault, value.BugReportsEnabled, value.FeedbackEnabled, value.BackendConnectionID, value.RetentionDays, value.State, expected))
	}
	if err != nil {
		return model.SupportRoute{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM integration_support_bindings WHERE support_route_id=$1`, value.ID); err != nil {
		return model.SupportRoute{}, databaseError(err)
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `DELETE FROM integration_support_bindings WHERE integration_id=$1 AND support_route_id<>$2`, integrationID, value.ID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO integration_support_bindings(integration_id,support_route_id) SELECT id,$2 FROM integrations WHERE id=$1 AND deployment_id=$3 ON CONFLICT DO NOTHING`, integrationID, value.ID, value.DeploymentID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, saved)
}

func (p *Postgres) SetIntegrationSupportRoute(ctx context.Context, deploymentID, integrationID, routeID, createdBy string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1 AND deployment_id=$2)`, integrationID, deploymentID).Scan(&exists); err != nil {
		return databaseError(err)
	}
	if !exists {
		return ErrNotFound
	}
	if routeID != "" {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM support_routes WHERE id=$1 AND deployment_id=$2 AND state='active')`, routeID, deploymentID).Scan(&exists); err != nil {
			return databaseError(err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM integration_support_bindings WHERE integration_id=$1`, integrationID); err != nil {
		return databaseError(err)
	}
	if routeID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO integration_support_bindings(integration_id,support_route_id,created_by) VALUES($1,$2,$3)`, integrationID, routeID, createdBy); err != nil {
			return databaseError(err)
		}
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
