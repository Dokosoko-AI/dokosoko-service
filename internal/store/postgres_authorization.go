package store

import (
	"context"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const grantDefinitionSelect = `SELECT id::text, deployment_id::text, organisation_id::text, key, display_name, description, risk, state, revision, created_at, updated_at FROM grant_definitions`

func scanGrantDefinition(row interface{ Scan(...any) error }) (model.GrantDefinition, error) {
	var value model.GrantDefinition
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Key, &value.DisplayName, &value.Description, &value.Risk, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) GrantDefinitions(ctx context.Context, deploymentID string) ([]model.GrantDefinition, error) {
	rows, err := p.pool.Query(ctx, grantDefinitionSelect+` WHERE deployment_id=$1 ORDER BY key`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.GrantDefinition, 0)
	for rows.Next() {
		value, scanErr := scanGrantDefinition(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) GrantDefinition(ctx context.Context, deploymentID, id string) (model.GrantDefinition, error) {
	return scanGrantDefinition(p.pool.QueryRow(ctx, grantDefinitionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) SaveGrantDefinition(ctx context.Context, value model.GrantDefinition, expected int64) (model.GrantDefinition, error) {
	if expected == 0 {
		return scanGrantDefinition(p.pool.QueryRow(ctx, `INSERT INTO grant_definitions(id,deployment_id,organisation_id,key,display_name,description,risk,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,deployment_id::text,organisation_id::text,key,display_name,description,risk,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.Key, value.DisplayName, value.Description, value.Risk, value.State))
	}
	updated, err := scanGrantDefinition(p.pool.QueryRow(ctx, `UPDATE grant_definitions SET display_name=$4,description=$5,risk=$6,state=$7,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$3 RETURNING id::text,deployment_id::text,organisation_id::text,key,display_name,description,risk,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, expected, value.DisplayName, value.Description, value.Risk, value.State))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.GrantDefinition(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.GrantDefinition{}, ErrConflict
		}
	}
	return updated, err
}

const authorizationPointSelect = `SELECT id::text, deployment_id::text, organisation_id::text, integration_id::text, key, name, description, action_type, required_grants, confirmation_required, decision_ttl_seconds, state, revision, created_at, updated_at FROM authorization_points`

func scanAuthorizationPoint(row interface{ Scan(...any) error }) (model.AuthorizationPoint, error) {
	var value model.AuthorizationPoint
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.IntegrationID, &value.Key, &value.Name, &value.Description, &value.ActionType, &value.RequiredGrants, &value.ConfirmationRequired, &value.DecisionTTLSeconds, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AuthorizationPoints(ctx context.Context, integrationID string) ([]model.AuthorizationPoint, error) {
	rows, err := p.pool.Query(ctx, authorizationPointSelect+` WHERE integration_id=$1 ORDER BY key`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.AuthorizationPoint, 0)
	for rows.Next() {
		value, scanErr := scanAuthorizationPoint(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) AuthorizationPoint(ctx context.Context, integrationID, id string) (model.AuthorizationPoint, error) {
	return scanAuthorizationPoint(p.pool.QueryRow(ctx, authorizationPointSelect+` WHERE integration_id=$1 AND id=$2`, integrationID, id))
}

func (p *Postgres) SaveAuthorizationPoint(ctx context.Context, value model.AuthorizationPoint, expected int64) (model.AuthorizationPoint, error) {
	if expected == 0 {
		return scanAuthorizationPoint(p.pool.QueryRow(ctx, `INSERT INTO authorization_points(id,deployment_id,organisation_id,integration_id,key,name,description,action_type,required_grants,confirmation_required,decision_ttl_seconds,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text,deployment_id::text,organisation_id::text,integration_id::text,key,name,description,action_type,required_grants,confirmation_required,decision_ttl_seconds,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.IntegrationID, value.Key, value.Name, value.Description, value.ActionType, value.RequiredGrants, value.ConfirmationRequired, value.DecisionTTLSeconds, value.State))
	}
	updated, err := scanAuthorizationPoint(p.pool.QueryRow(ctx, `UPDATE authorization_points SET name=$4,description=$5,action_type=$6,required_grants=$7,confirmation_required=$8,decision_ttl_seconds=$9,state=$10,revision=revision+1,updated_at=now() WHERE integration_id=$1 AND id=$2 AND revision=$3 RETURNING id::text,deployment_id::text,organisation_id::text,integration_id::text,key,name,description,action_type,required_grants,confirmation_required,decision_ttl_seconds,state,revision,created_at,updated_at`, value.IntegrationID, value.ID, expected, value.Name, value.Description, value.ActionType, value.RequiredGrants, value.ConfirmationRequired, value.DecisionTTLSeconds, value.State))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.AuthorizationPoint(ctx, value.IntegrationID, value.ID); lookupErr == nil {
			return model.AuthorizationPoint{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) IntegrationToolBindings(ctx context.Context, integrationID string) ([]model.IntegrationToolBinding, error) {
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text,tool_id::text,tool_revision,coalesce(authorization_point_id::text,''),coalesce(authorization_point_revision,0),created_by,created_at FROM integration_tool_bindings WHERE integration_id=$1 ORDER BY created_at,tool_id`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.IntegrationToolBinding, 0)
	for rows.Next() {
		var value model.IntegrationToolBinding
		if err := rows.Scan(&value.IntegrationID, &value.ToolID, &value.ToolRevision, &value.AuthorizationPointID, &value.AuthorizationPointRevision, &value.CreatedBy, &value.CreatedAt); err != nil {
			return nil, databaseError(err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range values {
		tool, lookupErr := scanTool(p.pool.QueryRow(ctx, toolSelect+` WHERE t.id=$1`, values[index].ToolID))
		if lookupErr == nil {
			values[index].Tool = &tool
		}
		if values[index].AuthorizationPointID != "" {
			point, pointErr := p.AuthorizationPoint(ctx, integrationID, values[index].AuthorizationPointID)
			if pointErr == nil {
				values[index].AuthorizationPoint = &point
			}
		}
	}
	return values, nil
}

func (p *Postgres) SaveIntegrationToolBindings(ctx context.Context, integrationID string, values []model.IntegrationToolBinding) ([]model.IntegrationToolBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM integration_tool_bindings WHERE integration_id=$1`, integrationID); err != nil {
		return nil, databaseError(err)
	}
	for _, value := range values {
		if _, err := tx.Exec(ctx, `INSERT INTO integration_tool_bindings(integration_id,tool_id,tool_revision,authorization_point_id,authorization_point_revision,created_by) VALUES($1,$2,$3,$4,$5,$6)`, integrationID, value.ToolID, value.ToolRevision, value.AuthorizationPointID, value.AuthorizationPointRevision, value.CreatedBy); err != nil {
			return nil, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, databaseError(err)
	}
	return p.IntegrationToolBindings(ctx, integrationID)
}
