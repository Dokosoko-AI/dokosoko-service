package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const runtimeServiceConnectionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,integration_id::text,name,description,state,revision,created_at,updated_at FROM runtime_service_connections`

func scanRuntimeServiceConnection(row interface{ Scan(...any) error }) (model.RuntimeServiceConnection, error) {
	var value model.RuntimeServiceConnection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.IntegrationID, &value.Name, &value.Description, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func scanRuntimeServiceConnectionRevision(row interface{ Scan(...any) error }) (model.RuntimeServiceConnectionRevision, error) {
	var value model.RuntimeServiceConnectionRevision
	var authConfig []byte
	err := row.Scan(&value.ID, &value.ConnectionID, &value.EnvironmentID, &value.BaseURL, &value.AuthenticationType, &value.CredentialSetID, &authConfig, &value.ContentHash, &value.Revision, &value.Current, &value.CreatedBy, &value.CreatedAt)
	value.AuthConfig = append(json.RawMessage(nil), authConfig...)
	return value, databaseError(err)
}

const runtimeServiceConnectionRevisionSelect = `SELECT id::text,connection_id::text,environment_id::text,base_url,authentication_type,coalesce(credential_set_id::text,''),auth_config,content_hash,revision,is_current,coalesce(created_by::text,''),created_at FROM runtime_service_connection_revisions`

func (p *Postgres) enrichRuntimeServiceConnection(ctx context.Context, value model.RuntimeServiceConnection) (model.RuntimeServiceConnection, error) {
	revisions, err := p.RuntimeServiceConnectionRevisions(ctx, value.ID, "")
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	for _, revision := range revisions {
		if revision.Current {
			value.CurrentRevisions = append(value.CurrentRevisions, revision)
		}
	}
	return value, nil
}

func (p *Postgres) RuntimeServiceConnections(ctx context.Context, deploymentID, integrationID string) ([]model.RuntimeServiceConnection, error) {
	query := runtimeServiceConnectionSelect + ` WHERE deployment_id=$1`
	args := []any{deploymentID}
	if integrationID != "" {
		query += ` AND integration_id=$2`
		args = append(args, integrationID)
	}
	query += ` ORDER BY name,id`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.RuntimeServiceConnection, 0)
	for rows.Next() {
		value, scanErr := scanRuntimeServiceConnection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	result := make([]model.RuntimeServiceConnection, 0, len(base))
	for _, value := range base {
		enriched, enrichErr := p.enrichRuntimeServiceConnection(ctx, value)
		if enrichErr != nil {
			return nil, enrichErr
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) RuntimeServiceConnection(ctx context.Context, deploymentID, id string) (model.RuntimeServiceConnection, error) {
	value, err := scanRuntimeServiceConnection(p.pool.QueryRow(ctx, runtimeServiceConnectionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	return p.enrichRuntimeServiceConnection(ctx, value)
}

func (p *Postgres) CreateRuntimeServiceConnection(ctx context.Context, value model.RuntimeServiceConnection) (model.RuntimeServiceConnection, error) {
	created, err := scanRuntimeServiceConnection(p.pool.QueryRow(ctx, `INSERT INTO runtime_service_connections(id,deployment_id,organisation_id,integration_id,name,description,state)
		SELECT $1,$2,$3,integration.id,$5,$6,$7 FROM integrations integration
		WHERE integration.id=$4 AND integration.deployment_id=$2 AND integration.organisation_id=$3
		RETURNING id::text,deployment_id::text,organisation_id::text,integration_id::text,name,description,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.IntegrationID, value.Name, value.Description, value.State))
	return created, err
}

func (p *Postgres) UpdateRuntimeServiceConnection(ctx context.Context, value model.RuntimeServiceConnection, expected int64) (model.RuntimeServiceConnection, error) {
	updated, err := scanRuntimeServiceConnection(p.pool.QueryRow(ctx, `UPDATE runtime_service_connections SET name=$3,description=$4,state=$5,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND integration_id=$6 AND organisation_id=$7 AND revision=$8
		RETURNING id::text,deployment_id::text,organisation_id::text,integration_id::text,name,description,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.Description, value.State, value.IntegrationID, value.OrganisationID, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.RuntimeServiceConnection(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.RuntimeServiceConnection{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) RuntimeServiceConnectionRevisions(ctx context.Context, connectionID, environmentID string) ([]model.RuntimeServiceConnectionRevision, error) {
	query := runtimeServiceConnectionRevisionSelect + ` WHERE connection_id=$1`
	args := []any{connectionID}
	if environmentID != "" {
		query += ` AND environment_id=$2`
		args = append(args, environmentID)
	}
	query += ` ORDER BY environment_id,revision DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RuntimeServiceConnectionRevision, 0)
	for rows.Next() {
		value, scanErr := scanRuntimeServiceConnectionRevision(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, databaseError(rows.Err())
}

func (p *Postgres) CreateRuntimeServiceConnectionRevision(ctx context.Context, value model.RuntimeServiceConnectionRevision) (model.RuntimeServiceConnectionRevision, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RuntimeServiceConnectionRevision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var deploymentID, organisationID, integrationID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text,organisation_id::text,integration_id::text FROM runtime_service_connections WHERE id=$1 FOR UPDATE`, value.ConnectionID).Scan(&deploymentID, &organisationID, &integrationID); err != nil {
		return model.RuntimeServiceConnectionRevision{}, databaseError(err)
	}
	var environmentExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE id=$1 AND product_id=$2 AND organisation_id=$3)`, value.EnvironmentID, deploymentID, organisationID).Scan(&environmentExists); err != nil || !environmentExists {
		if err != nil {
			return model.RuntimeServiceConnectionRevision{}, databaseError(err)
		}
		return model.RuntimeServiceConnectionRevision{}, ErrNotFound
	}
	if value.CredentialSetID != "" {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM runtime_credential_sets credential
			WHERE credential.id=$1 AND credential.deployment_id=$2 AND credential.organisation_id=$3 AND credential.environment_id=$4
			AND (credential.scope='shared' OR credential.owner_integration_id=$5)
		)`, value.CredentialSetID, deploymentID, organisationID, value.EnvironmentID, integrationID).Scan(&allowed); err != nil || !allowed {
			if err != nil {
				return model.RuntimeServiceConnectionRevision{}, databaseError(err)
			}
			return model.RuntimeServiceConnectionRevision{}, ErrConflict
		}
	}
	var next int64
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(revision),0)+1 FROM runtime_service_connection_revisions WHERE connection_id=$1 AND environment_id=$2`, value.ConnectionID, value.EnvironmentID).Scan(&next); err != nil {
		return model.RuntimeServiceConnectionRevision{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE runtime_service_connection_revisions SET is_current=false WHERE connection_id=$1 AND environment_id=$2 AND is_current`, value.ConnectionID, value.EnvironmentID); err != nil {
		return model.RuntimeServiceConnectionRevision{}, databaseError(err)
	}
	authConfig := value.AuthConfig
	if len(authConfig) == 0 {
		authConfig = json.RawMessage(`{}`)
	}
	created, err := scanRuntimeServiceConnectionRevision(tx.QueryRow(ctx, `INSERT INTO runtime_service_connection_revisions(id,connection_id,environment_id,base_url,authentication_type,credential_set_id,auth_config,content_hash,revision,is_current,created_by)
		VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9,true,nullif($10,'')::uuid)
		RETURNING id::text,connection_id::text,environment_id::text,base_url,authentication_type,coalesce(credential_set_id::text,''),auth_config,content_hash,revision,is_current,coalesce(created_by::text,''),created_at`, value.ID, value.ConnectionID, value.EnvironmentID, value.BaseURL, value.AuthenticationType, value.CredentialSetID, authConfig, value.ContentHash, next, value.CreatedBy))
	if err != nil {
		return model.RuntimeServiceConnectionRevision{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runtime_service_connections SET revision=revision+1,updated_at=now() WHERE id=$1`, value.ConnectionID); err != nil {
		return model.RuntimeServiceConnectionRevision{}, databaseError(err)
	}
	return created, tx.Commit(ctx)
}

const runtimeCredentialSetSelect = `SELECT credential.id::text,credential.deployment_id::text,credential.organisation_id::text,credential.environment_id::text,credential.scope,coalesce(credential.owner_integration_id::text,''),credential.name,credential.environment_variable,credential.authentication_type,credential.header_name,credential.state,(active.id IS NOT NULL),coalesce(active.fingerprint,''),credential.revision,credential.created_at,credential.updated_at
	FROM runtime_credential_sets credential
	LEFT JOIN LATERAL (SELECT id,fingerprint FROM runtime_credential_versions WHERE credential_set_id=credential.id AND state='active' LIMIT 1) active ON true`

func scanRuntimeCredentialSet(row interface{ Scan(...any) error }) (model.RuntimeCredentialSet, error) {
	var value model.RuntimeCredentialSet
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.EnvironmentID, &value.Scope, &value.OwnerIntegrationID, &value.Name, &value.EnvironmentVariable, &value.AuthenticationType, &value.HeaderName, &value.State, &value.CredentialPresent, &value.ActiveFingerprint, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const runtimeCredentialVersionSelect = `SELECT id::text,credential_set_id::text,secret_id::text,fingerprint,state,coalesce(created_by::text,''),activated_at,retires_at,revoked_at,expires_at,created_at FROM runtime_credential_versions`

func scanToolRuntimeTarget(row interface{ Scan(...any) error }) (model.ToolRuntimeTarget, error) {
	var value model.ToolRuntimeTarget
	var authConfig []byte
	err := row.Scan(&value.EnvironmentID, &value.RuntimeServiceConnectionID, &value.ConnectionRevisionID, &value.BaseURL, &value.AuthenticationType, &value.CredentialSetID, &authConfig, &value.CredentialVersionID, &value.CredentialSecretID, &value.CredentialFingerprint, &value.HeaderName)
	value.AuthConfig = append(json.RawMessage(nil), authConfig...)
	return value, databaseError(err)
}

// enrichToolRuntimeTargets resolves immutable configuration and the currently
// active credential separately. Published tools read configuration through
// their release pins; drafts preview the logical connection's current config.
func (p *Postgres) enrichToolRuntimeTargets(ctx context.Context, value model.Tool) (model.Tool, error) {
	if value.RuntimeServiceConnectionID == "" {
		return value, nil
	}
	selectColumns := `SELECT revision.environment_id::text,revision.connection_id::text,revision.id::text,revision.base_url,revision.authentication_type,coalesce(revision.credential_set_id::text,''),revision.auth_config,coalesce(active.id::text,''),coalesce(active.secret_id::text,''),coalesce(active.fingerprint,''),coalesce(credential.header_name,'') `
	joins := ` LEFT JOIN runtime_credential_sets credential ON credential.id=revision.credential_set_id LEFT JOIN LATERAL (SELECT version.id,version.secret_id,version.fingerprint FROM runtime_credential_versions version WHERE version.credential_set_id=credential.id AND version.state='active' AND (version.expires_at IS NULL OR version.expires_at>now()) LIMIT 1) active ON true `
	query := ""
	args := []any{value.ID, value.Revision}
	if value.State == "published" {
		query = selectColumns + `FROM tool_releases release JOIN tool_release_runtime_targets target ON target.tool_release_id=release.id JOIN runtime_service_connection_revisions revision ON revision.id=target.connection_revision_id` + joins + `WHERE release.tool_definition_id=$1 AND release.version=$2 ORDER BY revision.environment_id`
	} else {
		query = selectColumns + `FROM runtime_service_connection_revisions revision` + joins + `WHERE revision.connection_id=$3 AND revision.is_current ORDER BY revision.environment_id`
		args = append(args, value.RuntimeServiceConnectionID)
	}
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	defer rows.Close()
	value.RuntimeTargets = nil
	value.CredentialPresent = true
	for rows.Next() {
		target, scanErr := scanToolRuntimeTarget(rows)
		if scanErr != nil {
			return model.Tool{}, scanErr
		}
		if target.CredentialSetID != "" && target.CredentialSecretID == "" {
			value.CredentialPresent = false
		}
		value.RuntimeTargets = append(value.RuntimeTargets, target)
	}
	if err := rows.Err(); err != nil {
		return model.Tool{}, databaseError(err)
	}
	return value, nil
}

func scanRuntimeCredentialVersion(row interface{ Scan(...any) error }) (model.RuntimeCredentialVersion, error) {
	var value model.RuntimeCredentialVersion
	err := row.Scan(&value.ID, &value.CredentialSetID, &value.SecretID, &value.Fingerprint, &value.State, &value.CreatedBy, &value.ActivatedAt, &value.RetiresAt, &value.RevokedAt, &value.ExpiresAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichRuntimeCredentialSet(ctx context.Context, value model.RuntimeCredentialSet) (model.RuntimeCredentialSet, error) {
	versions, err := p.RuntimeCredentialVersions(ctx, value.ID)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	value.Versions = versions
	return value, nil
}

func (p *Postgres) RuntimeCredentialSets(ctx context.Context, deploymentID, environmentID string) ([]model.RuntimeCredentialSet, error) {
	query := runtimeCredentialSetSelect + ` WHERE credential.deployment_id=$1`
	args := []any{deploymentID}
	if environmentID != "" {
		query += ` AND credential.environment_id=$2`
		args = append(args, environmentID)
	}
	query += ` ORDER BY credential.name,credential.id`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.RuntimeCredentialSet, 0)
	for rows.Next() {
		value, scanErr := scanRuntimeCredentialSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	result := make([]model.RuntimeCredentialSet, 0, len(base))
	for _, value := range base {
		enriched, enrichErr := p.enrichRuntimeCredentialSet(ctx, value)
		if enrichErr != nil {
			return nil, enrichErr
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) RuntimeCredentialSet(ctx context.Context, deploymentID, id string) (model.RuntimeCredentialSet, error) {
	value, err := scanRuntimeCredentialSet(p.pool.QueryRow(ctx, runtimeCredentialSetSelect+` WHERE credential.deployment_id=$1 AND credential.id=$2`, deploymentID, id))
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	return p.enrichRuntimeCredentialSet(ctx, value)
}

func (p *Postgres) CreateRuntimeCredentialSet(ctx context.Context, value model.RuntimeCredentialSet) (model.RuntimeCredentialSet, error) {
	created, err := scanRuntimeCredentialSet(p.pool.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO runtime_credential_sets(id,deployment_id,organisation_id,environment_id,scope,owner_integration_id,name,environment_variable,authentication_type,header_name,state)
		SELECT $1,$2,$3,environment.id,$5,nullif($6,'')::uuid,$7,$8,$9,$10,$11
		FROM environments environment
		WHERE environment.id=$4 AND environment.product_id=$2 AND environment.organisation_id=$3
		RETURNING *
	) SELECT inserted.id::text,inserted.deployment_id::text,inserted.organisation_id::text,inserted.environment_id::text,inserted.scope,coalesce(inserted.owner_integration_id::text,''),inserted.name,inserted.environment_variable,inserted.authentication_type,inserted.header_name,inserted.state,false,'',inserted.revision,inserted.created_at,inserted.updated_at FROM inserted`, value.ID, value.DeploymentID, value.OrganisationID, value.EnvironmentID, value.Scope, value.OwnerIntegrationID, value.Name, value.EnvironmentVariable, value.AuthenticationType, value.HeaderName, value.State))
	return created, err
}

func (p *Postgres) UpdateRuntimeCredentialSet(ctx context.Context, value model.RuntimeCredentialSet, expected int64) (model.RuntimeCredentialSet, error) {
	updated, err := scanRuntimeCredentialSet(p.pool.QueryRow(ctx, `WITH updated AS (
		UPDATE runtime_credential_sets SET name=$3,environment_variable=$4,header_name=$5,state=$6,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND environment_id=$7 AND scope=$8 AND coalesce(owner_integration_id::text,'')=$9 AND authentication_type=$10 AND revision=$11
		RETURNING *
	) SELECT updated.id::text,updated.deployment_id::text,updated.organisation_id::text,updated.environment_id::text,updated.scope,coalesce(updated.owner_integration_id::text,''),updated.name,updated.environment_variable,updated.authentication_type,updated.header_name,updated.state,
		EXISTS(SELECT 1 FROM runtime_credential_versions WHERE credential_set_id=updated.id AND state='active'),
		coalesce((SELECT fingerprint FROM runtime_credential_versions WHERE credential_set_id=updated.id AND state='active' LIMIT 1),''),updated.revision,updated.created_at,updated.updated_at FROM updated`, value.DeploymentID, value.ID, value.Name, value.EnvironmentVariable, value.HeaderName, value.State, value.EnvironmentID, value.Scope, value.OwnerIntegrationID, value.AuthenticationType, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.RuntimeCredentialSet(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.RuntimeCredentialSet{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) RuntimeCredentialVersions(ctx context.Context, credentialSetID string) ([]model.RuntimeCredentialVersion, error) {
	rows, err := p.pool.Query(ctx, runtimeCredentialVersionSelect+` WHERE credential_set_id=$1 ORDER BY created_at DESC,id DESC`, credentialSetID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.RuntimeCredentialVersion, 0)
	for rows.Next() {
		value, scanErr := scanRuntimeCredentialVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, databaseError(rows.Err())
}

func (p *Postgres) CreateRuntimeCredentialVersion(ctx context.Context, value model.RuntimeCredentialVersion) (model.RuntimeCredentialVersion, error) {
	return scanRuntimeCredentialVersion(p.pool.QueryRow(ctx, `INSERT INTO runtime_credential_versions(id,credential_set_id,secret_id,fingerprint,state,created_by,expires_at)
		SELECT $1,credential.id,secret.id,$4,'pending',nullif($5,'')::uuid,$6
		FROM runtime_credential_sets credential JOIN secrets secret ON secret.id=$3 AND secret.organisation_id=credential.organisation_id
		WHERE credential.id=$2
		RETURNING id::text,credential_set_id::text,secret_id::text,fingerprint,state,coalesce(created_by::text,''),activated_at,retires_at,revoked_at,expires_at,created_at`, value.ID, value.CredentialSetID, value.SecretID, value.Fingerprint, value.CreatedBy, value.ExpiresAt))
}

func (p *Postgres) ActivateRuntimeCredentialVersion(ctx context.Context, deploymentID, credentialSetID, versionID string, now time.Time) (model.RuntimeCredentialVersion, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RuntimeCredentialVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM runtime_credential_sets WHERE deployment_id=$1 AND id=$2 FOR UPDATE`, deploymentID, credentialSetID); err != nil {
		return model.RuntimeCredentialVersion{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE runtime_credential_versions SET state='retiring',retires_at=$3 WHERE credential_set_id=$1 AND id<>$2 AND state='active'`, credentialSetID, versionID, now); err != nil {
		return model.RuntimeCredentialVersion{}, databaseError(err)
	}
	activated, err := scanRuntimeCredentialVersion(tx.QueryRow(ctx, `UPDATE runtime_credential_versions SET state='active',activated_at=$3,retires_at=NULL,revoked_at=NULL WHERE credential_set_id=$1 AND id=$2 AND state='pending'
		RETURNING id::text,credential_set_id::text,secret_id::text,fingerprint,state,coalesce(created_by::text,''),activated_at,retires_at,revoked_at,expires_at,created_at`, credentialSetID, versionID, now))
	if err != nil {
		return model.RuntimeCredentialVersion{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runtime_credential_sets SET revision=revision+1,updated_at=$2 WHERE id=$1`, credentialSetID, now); err != nil {
		return model.RuntimeCredentialVersion{}, databaseError(err)
	}
	return activated, tx.Commit(ctx)
}

func (p *Postgres) RevokeRuntimeCredentialVersion(ctx context.Context, deploymentID, credentialSetID, versionID string, now time.Time) (model.RuntimeCredentialVersion, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.RuntimeCredentialVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentState string
	if err := tx.QueryRow(ctx, `SELECT version.state FROM runtime_credential_versions version JOIN runtime_credential_sets credential ON credential.id=version.credential_set_id WHERE credential.deployment_id=$1 AND credential.id=$2 AND version.id=$3 FOR UPDATE`, deploymentID, credentialSetID, versionID).Scan(&currentState); err != nil {
		return model.RuntimeCredentialVersion{}, databaseError(err)
	}
	if currentState != "revoked" {
		if _, err := tx.Exec(ctx, `UPDATE runtime_credential_versions SET state='revoked',revoked_at=$3 WHERE credential_set_id=$1 AND id=$2`, credentialSetID, versionID, now); err != nil {
			return model.RuntimeCredentialVersion{}, databaseError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE runtime_credential_sets SET revision=revision+1,updated_at=$2 WHERE id=$1`, credentialSetID, now); err != nil {
			return model.RuntimeCredentialVersion{}, databaseError(err)
		}
	}
	value, err := scanRuntimeCredentialVersion(tx.QueryRow(ctx, runtimeCredentialVersionSelect+` WHERE credential_set_id=$1 AND id=$2`, credentialSetID, versionID))
	if err != nil {
		return model.RuntimeCredentialVersion{}, err
	}
	return value, tx.Commit(ctx)
}
