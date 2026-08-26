package store

import (
	"context"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"time"
)

const mcpConnectionSelect = `SELECT id::text, organisation_id::text, product_id::text, name, namespace, endpoint, protocol_version, auth_mode, coalesce(credential_secret_id::text,''), forward_user_identity, state, last_synced_at, last_catalog_hash, config, revision, created_at, updated_at FROM mcp_connections`

func scanMCPConnection(row interface{ Scan(...any) error }) (model.MCPConnection, error) {
	var value model.MCPConnection
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Name, &value.Namespace, &value.Endpoint, &value.ProtocolVersion, &value.AuthMode, &value.CredentialID, &value.ForwardUserIdentity, &value.State, &value.LastSyncedAt, &value.LastCatalogHash, &value.Config, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
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
	return scanMCPConnection(p.pool.QueryRow(ctx, `INSERT INTO mcp_connections(id,organisation_id,product_id,name,namespace,endpoint,protocol_version,auth_mode,credential_secret_id,forward_user_identity,config) VALUES($1,$2,$3,$4,$5,$6,'2026-07-28','access_token',$7,$8,$9) RETURNING `+mcpConnectionSelect[len("SELECT "):len(mcpConnectionSelect)-len(" FROM mcp_connections")], value.ID, value.OrganisationID, value.ProductID, value.Name, value.Namespace, value.Endpoint, value.CredentialID, value.ForwardUserIdentity, value.Config))
}

func (p *Postgres) UpdateMCPConnectionSync(ctx context.Context, productID, id, catalogHash string, syncedAt time.Time) (model.MCPConnection, error) {
	return scanMCPConnection(p.pool.QueryRow(ctx, `UPDATE mcp_connections SET last_synced_at=$3,last_catalog_hash=$4,revision=revision+1,updated_at=$3 WHERE product_id=$1 AND id=$2 RETURNING `+mcpConnectionSelect[len("SELECT "):len(mcpConnectionSelect)-len(" FROM mcp_connections")], productID, id, syncedAt, catalogHash))
}
