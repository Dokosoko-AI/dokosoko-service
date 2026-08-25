package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const accessDefinitionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,coalesce(api_resource_set_id::text,''),operations,state,revision,created_at,updated_at FROM access_definitions`

func scanAccessDefinition(row pgx.Row) (model.AccessDefinition, error) {
	var value model.AccessDefinition
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.ServiceKey, &value.Name, &value.InstanceCardinality, &value.InstanceLabelSingular, &value.InstanceLabelPlural, &value.CredentialScope, &value.ManagementAuthType, &value.APIResourceSetID, &value.Operations, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AccessDefinitions(ctx context.Context, deploymentID string) ([]model.AccessDefinition, error) {
	rows, err := p.pool.Query(ctx, accessDefinitionSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AccessDefinition, 0)
	for rows.Next() {
		value, err := scanAccessDefinition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AccessDefinition(ctx context.Context, deploymentID, id string) (model.AccessDefinition, error) {
	return scanAccessDefinition(p.pool.QueryRow(ctx, accessDefinitionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateAccessDefinition(ctx context.Context, value model.AccessDefinition) (model.AccessDefinition, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessDefinition(tx.QueryRow(ctx, `INSERT INTO access_definitions(id,deployment_id,organisation_id,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,api_resource_set_id,operations,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,nullif($11,'')::uuid,$12,$13) RETURNING id::text,deployment_id::text,organisation_id::text,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,coalesce(api_resource_set_id::text,''),operations,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.ServiceKey, value.Name, value.InstanceCardinality, value.InstanceLabelSingular, value.InstanceLabelPlural, value.CredentialScope, value.ManagementAuthType, value.APIResourceSetID, value.Operations, value.State))
	if err != nil {
		return model.AccessDefinition{}, err
	}
	snapshot, err := accessDefinitionSnapshot(created)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	digest := sha256.Sum256(snapshot)
	if _, err := tx.Exec(ctx, `INSERT INTO access_definition_revisions(access_definition_id,revision,snapshot,content_hash) VALUES($1,$2,$3,$4)`, created.ID, created.Revision, snapshot, "sha256:"+hex.EncodeToString(digest[:])); err != nil {
		return model.AccessDefinition{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.AccessDefinition{}, err
	}
	return created, tx.Commit(ctx)
}

func accessDefinitionSnapshot(value model.AccessDefinition) ([]byte, error) {
	return json.Marshal(map[string]any{"service_key": value.ServiceKey, "name": value.Name, "instance_cardinality": value.InstanceCardinality, "instance_label_singular": value.InstanceLabelSingular, "instance_label_plural": value.InstanceLabelPlural, "credential_scope": value.CredentialScope, "management_auth_type": value.ManagementAuthType, "api_resource_set_id": value.APIResourceSetID, "operations": json.RawMessage(value.Operations), "state": value.State})
}

func (p *Postgres) UpdateAccessDefinition(ctx context.Context, value model.AccessDefinition, expected int64) (model.AccessDefinition, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanAccessDefinition(tx.QueryRow(ctx, `UPDATE access_definitions SET name=$3,instance_label_singular=$4,instance_label_plural=$5,api_resource_set_id=nullif($6,'')::uuid,operations=$7,state=$8,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$9 RETURNING id::text,deployment_id::text,organisation_id::text,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,coalesce(api_resource_set_id::text,''),operations,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.InstanceLabelSingular, value.InstanceLabelPlural, value.APIResourceSetID, value.Operations, value.State, expected))
	if err != nil {
		return model.AccessDefinition{}, err
	}
	snapshot, err := accessDefinitionSnapshot(updated)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	digest := sha256.Sum256(snapshot)
	if _, err := tx.Exec(ctx, `INSERT INTO access_definition_revisions(access_definition_id,revision,snapshot,content_hash) VALUES($1,$2,$3,$4)`, updated.ID, updated.Revision, snapshot, "sha256:"+hex.EncodeToString(digest[:])); err != nil {
		return model.AccessDefinition{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.AccessDefinition{}, err
	}
	return updated, tx.Commit(ctx)
}

const accessConnectionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_definition_id::text,coalesce(environment_id::text,''),name,region,base_url,coalesce(management_secret_id::text,''),coalesce(legacy_provider_id::text,''),config,state,revision,created_at,updated_at FROM access_connections`

func scanAccessConnection(row pgx.Row) (model.AccessConnection, error) {
	var value model.AccessConnection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessDefinitionID, &value.EnvironmentID, &value.Name, &value.Region, &value.BaseURL, &value.ManagementSecretID, &value.LegacyProviderID, &value.Config, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichAccessConnection(ctx context.Context, value model.AccessConnection) (model.AccessConnection, error) {
	definition, err := p.AccessDefinition(ctx, value.DeploymentID, value.AccessDefinitionID)
	if err != nil {
		return model.AccessConnection{}, err
	}
	value.Definition = &definition
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_access_bindings WHERE access_connection_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.AccessConnection{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.AccessConnection{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) AccessConnections(ctx context.Context, deploymentID string) ([]model.AccessConnection, error) {
	rows, err := p.pool.Query(ctx, accessConnectionSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.AccessConnection, 0)
	for rows.Next() {
		value, err := scanAccessConnection(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.AccessConnection, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichAccessConnection(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) AccessConnection(ctx context.Context, deploymentID, id string) (model.AccessConnection, error) {
	value, err := scanAccessConnection(p.pool.QueryRow(ctx, accessConnectionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.AccessConnection{}, err
	}
	return p.enrichAccessConnection(ctx, value)
}

func (p *Postgres) CreateAccessConnection(ctx context.Context, value model.AccessConnection) (model.AccessConnection, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessConnection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessConnection(tx.QueryRow(ctx, `INSERT INTO access_connections(id,deployment_id,organisation_id,access_definition_id,environment_id,name,region,base_url,management_secret_id,config,state) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,nullif($9,'')::uuid,$10,$11) RETURNING id::text,deployment_id::text,organisation_id::text,access_definition_id::text,coalesce(environment_id::text,''),name,region,base_url,coalesce(management_secret_id::text,''),coalesce(legacy_provider_id::text,''),config,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessDefinitionID, value.EnvironmentID, value.Name, value.Region, value.BaseURL, value.ManagementSecretID, value.Config, value.State))
	if err != nil {
		return model.AccessConnection{}, err
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO integration_access_bindings(integration_id,access_connection_id) SELECT id,$2 FROM integrations WHERE id=$1 AND deployment_id=$3 ON CONFLICT DO NOTHING`, integrationID, value.ID, value.DeploymentID); err != nil {
			return model.AccessConnection{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AccessConnection{}, err
	}
	return p.enrichAccessConnection(ctx, created)
}

func (p *Postgres) SetIntegrationAccessConnections(ctx context.Context, deploymentID, integrationID string, connectionIDs []string, createdBy string) error {
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
	for _, connectionID := range connectionIDs {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM access_connections WHERE id=$1 AND deployment_id=$2)`, connectionID, deploymentID).Scan(&exists); err != nil {
			return databaseError(err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM integration_access_bindings WHERE integration_id=$1`, integrationID); err != nil {
		return databaseError(err)
	}
	for _, connectionID := range connectionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO integration_access_bindings(integration_id,access_connection_id,created_by) VALUES($1,$2,$3)`, integrationID, connectionID, createdBy); err != nil {
			return databaseError(err)
		}
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const accessInstanceSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_connection_id::text,environment_id::text,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at,created_at,updated_at FROM access_instances`

func scanAccessInstance(row pgx.Row) (model.AccessInstance, error) {
	var value model.AccessInstance
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessConnectionID, &value.EnvironmentID, &value.OwnerType, &value.OwnerID, &value.ExternalID, &value.DisplayName, &value.IdempotencyKey, &value.State, &value.ProviderMetadata, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichAccessInstance(ctx context.Context, value model.AccessInstance) (model.AccessInstance, error) {
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM access_instance_integration_bindings WHERE access_instance_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.AccessInstance{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.AccessInstance{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) AccessInstances(ctx context.Context, deploymentID, connectionID string) ([]model.AccessInstance, error) {
	query, args := accessInstanceSelect+` WHERE deployment_id=$1`, []any{deploymentID}
	if connectionID != "" {
		query += ` AND access_connection_id=$2`
		args = append(args, connectionID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.AccessInstance, 0)
	for rows.Next() {
		value, err := scanAccessInstance(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.AccessInstance, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichAccessInstance(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) AccessInstance(ctx context.Context, deploymentID, id string) (model.AccessInstance, error) {
	value, err := scanAccessInstance(p.pool.QueryRow(ctx, accessInstanceSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.AccessInstance{}, err
	}
	return p.enrichAccessInstance(ctx, value)
}

func (p *Postgres) CreateAccessInstance(ctx context.Context, value model.AccessInstance) (model.AccessInstance, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessInstance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessInstance(tx.QueryRow(ctx, `INSERT INTO access_instances(id,deployment_id,organisation_id,access_connection_id,environment_id,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(access_connection_id,idempotency_key) DO UPDATE SET updated_at=access_instances.updated_at RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,environment_id::text,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessConnectionID, value.EnvironmentID, value.OwnerType, value.OwnerID, value.ExternalID, value.DisplayName, value.IdempotencyKey, value.State, value.ProviderMetadata, value.ExpiresAt))
	if err != nil {
		return model.AccessInstance{}, err
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO access_instance_integration_bindings(access_instance_id,integration_id) SELECT $1,integration_id FROM integration_access_bindings WHERE access_connection_id=$2 AND integration_id=$3 ON CONFLICT DO NOTHING`, created.ID, value.AccessConnectionID, integrationID); err != nil {
			return model.AccessInstance{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AccessInstance{}, err
	}
	return p.enrichAccessInstance(ctx, created)
}

const accessCredentialSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at FROM access_credentials`

func scanAccessCredential(row pgx.Row) (model.AccessCredential, error) {
	var value model.AccessCredential
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessConnectionID, &value.AccessInstanceID, &value.EnvironmentID, &value.SubjectID, &value.ExternalID, &value.IdempotencyKey, &value.Scopes, &value.SecretFingerprint, &value.StorageMode, &value.EncryptedSecretID, &value.State, &value.ExpiresAt, &value.RotatedFromID, &value.RevokedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AccessCredentials(ctx context.Context, deploymentID, connectionID, instanceID string) ([]model.AccessCredential, error) {
	query, args, index := accessCredentialSelect+` WHERE deployment_id=$1`, []any{deploymentID}, 2
	if connectionID != "" {
		query += ` AND access_connection_id=$` + strconv.Itoa(index)
		args, index = append(args, connectionID), index+1
	}
	if instanceID != "" {
		query += ` AND access_instance_id=$` + strconv.Itoa(index)
		args = append(args, instanceID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AccessCredential, 0)
	for rows.Next() {
		value, err := scanAccessCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AccessCredential(ctx context.Context, deploymentID, id string) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, accessCredentialSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateAccessCredential(ctx context.Context, value model.AccessCredential) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, `INSERT INTO access_credentials(id,deployment_id,organisation_id,access_connection_id,access_instance_id,environment_id,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,encrypted_secret_id,state,expires_at,rotated_from_id) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,nullif($13,'')::uuid,$14,$15,nullif($16,'')::uuid) ON CONFLICT(access_connection_id,idempotency_key) WHERE idempotency_key<>'' DO UPDATE SET idempotency_key=access_credentials.idempotency_key RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessConnectionID, value.AccessInstanceID, value.EnvironmentID, value.SubjectID, value.ExternalID, value.IdempotencyKey, value.Scopes, value.SecretFingerprint, value.StorageMode, value.EncryptedSecretID, value.State, value.ExpiresAt, value.RotatedFromID))
}

func (p *Postgres) RevokeAccessCredential(ctx context.Context, deploymentID, id string, revokedAt time.Time) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, `UPDATE access_credentials SET state='revoked',revoked_at=$3 WHERE deployment_id=$1 AND id=$2 AND revoked_at IS NULL RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at`, deploymentID, id, revokedAt))
}
