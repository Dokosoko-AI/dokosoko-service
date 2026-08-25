package store

import (
	"context"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"time"
)

const mcpConnectionSelect = `SELECT id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at FROM mcp_connections`

func scanMCPConnection(row interface{ Scan(...any) error }) (model.MCPConnection, error) {
	var value model.MCPConnection
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Namespace, &value.Endpoint, &value.ProtocolVersion, &value.AuthMode, &value.CredentialID, &value.OAuthClientID, &value.OAuthClientSecretID, &value.OAuthIssuer, &value.AuthorizationURL, &value.TokenURL, &value.Scopes, &value.State, &value.LastSyncedAt, &value.LastCatalogHash, &value.Config, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) MCPConnections(ctx context.Context, productID string) ([]model.MCPConnection, error) {
	rows, err := p.pool.Query(ctx, mcpConnectionSelect+` WHERE product_id=$1 ORDER BY name`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.MCPConnection, 0)
	for rows.Next() {
		value, err := scanMCPConnection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) MCPConnection(ctx context.Context, productID, id string) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, mcpConnectionSelect+` WHERE product_id=$1 AND id=$2`, productID, id))
}

func (p *Postgres) CreateMCPConnection(ctx context.Context, value model.MCPConnection) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, `INSERT INTO mcp_connections(id,organisation_id,product_id,name,namespace,endpoint,protocol_version,auth_mode,credential_secret_id,oauth_client_id,oauth_client_secret_id,oauth_issuer,authorization_url,token_url,scopes,config) VALUES($1,$2,$3,$4,$5,$6,'2026-07-28',$7,nullif($8,'')::uuid,$9,nullif($10,'')::uuid,$11,$12,$13,$14,$15) RETURNING id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Namespace, value.Endpoint, value.AuthMode, value.CredentialID, value.OAuthClientID, value.OAuthClientSecretID, value.OAuthIssuer, value.AuthorizationURL, value.TokenURL, value.Scopes, value.Config))
}

func (p *Postgres) UpdateMCPConnectionSync(ctx context.Context, productID, id, catalogHash string, syncedAt time.Time) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, `UPDATE mcp_connections SET last_synced_at=$3,last_catalog_hash=$4,revision=revision+1,updated_at=$3 WHERE product_id=$1 AND id=$2 RETURNING id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), oauth_client_id, coalesce(oauth_client_secret_id::text,''), oauth_issuer, authorization_url, token_url, scopes, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at`, productID, id, syncedAt, catalogHash))
}

const mcpGrantSelect = `SELECT id::text,organisation_id::text,product_id::text,connection_id::text,subject_id,upstream_subject,access_secret_id::text,coalesce(refresh_secret_id::text,''),scopes,expires_at,revoked_at,created_at,updated_at FROM mcp_user_grants`

func scanMCPGrant(row interface{ Scan(...any) error }) (model.MCPUserGrant, error) {
	var value model.MCPUserGrant
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ConnectionID, &value.SubjectID, &value.UpstreamSubject, &value.AccessSecretID, &value.RefreshSecretID, &value.Scopes, &value.ExpiresAt, &value.RevokedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) MCPUserGrant(ctx context.Context, connectionID, subjectID string) (model.MCPUserGrant, error) {
	return scanMCPGrant(p.pool.QueryRow(ctx, mcpGrantSelect+` WHERE connection_id=$1 AND subject_id=$2 AND revoked_at IS NULL`, connectionID, subjectID))
}

func (p *Postgres) SaveMCPUserGrant(ctx context.Context, value model.MCPUserGrant) (model.MCPUserGrant, error) {
	return scanMCPGrant(p.pool.QueryRow(ctx, `INSERT INTO mcp_user_grants(id,organisation_id,product_id,connection_id,subject_id,upstream_subject,access_secret_id,refresh_secret_id,scopes,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,$9,$10,null) ON CONFLICT(connection_id,subject_id) DO UPDATE SET upstream_subject=excluded.upstream_subject,access_secret_id=excluded.access_secret_id,refresh_secret_id=excluded.refresh_secret_id,scopes=excluded.scopes,expires_at=excluded.expires_at,revoked_at=null,updated_at=now() RETURNING id::text,organisation_id::text,product_id::text,connection_id::text,subject_id,upstream_subject,access_secret_id::text,coalesce(refresh_secret_id::text,''),scopes,expires_at,revoked_at,created_at,updated_at`, value.ID, value.OrganisationID, value.ProductID, value.ConnectionID, value.SubjectID, value.UpstreamSubject, value.AccessSecretID, value.RefreshSecretID, value.Scopes, value.ExpiresAt))
}

func (p *Postgres) CreateMCPAuthorizationState(ctx context.Context, value model.MCPAuthorizationState) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO mcp_authorization_states(digest,connection_id,product_id,subject_id,code_verifier,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, value.Digest, value.ConnectionID, value.ProductID, value.SubjectID, value.CodeVerifier, value.ExpiresAt)
	return databaseError(err)
}

func (p *Postgres) ConsumeMCPAuthorizationState(ctx context.Context, digest []byte) (model.MCPAuthorizationState, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.MCPAuthorizationState{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.MCPAuthorizationState
	err = tx.QueryRow(ctx, `DELETE FROM mcp_authorization_states WHERE digest=$1 AND expires_at>now() RETURNING digest,connection_id::text,product_id::text,subject_id,code_verifier,expires_at`, digest).Scan(&value.Digest, &value.ConnectionID, &value.ProductID, &value.SubjectID, &value.CodeVerifier, &value.ExpiresAt)
	if err != nil {
		return model.MCPAuthorizationState{}, databaseError(err)
	}
	return value, tx.Commit(ctx)
}
