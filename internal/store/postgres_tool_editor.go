package store

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func postgresToolSecretBound(organisationID, purpose, name, expectedOrganisationID, connectionID string) bool {
	return organisationID == expectedOrganisationID && purpose == "tool_upstream" && strings.HasPrefix(name, "tool-connection-"+connectionID+"-")
}

func (p *Postgres) UpdateTool(ctx context.Context, value model.Tool, expected int64) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	priorCredentialID := ""
	if value.BackendKind == "http" && value.RuntimeServiceConnectionID == "" {
		authenticationType := "delegated_oauth"
		var upstreamAuth struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(value.UpstreamAuth, &upstreamAuth) == nil && upstreamAuth.Type != "" {
			authenticationType = upstreamAuth.Type
		}
		parsed, parseErr := url.Parse(value.BaseURL)
		if parseErr != nil {
			return model.Tool{}, parseErr
		}
		var connectionOrganisationID string
		if err := tx.QueryRow(ctx, `SELECT organisation_id::text,coalesce(credential_secret_id::text,'') FROM api_connections WHERE product_id=$1 AND id=$2 FOR UPDATE`, value.ProductID, value.APIConnectionID).Scan(&connectionOrganisationID, &priorCredentialID); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if connectionOrganisationID != value.OrganisationID {
			return model.Tool{}, ErrConflict
		}
		if value.CredentialID != "" {
			var credentialOrganisationID, credentialPurpose, credentialName string
			if err := tx.QueryRow(ctx, `SELECT organisation_id::text,purpose,name FROM secrets WHERE id=$1 FOR UPDATE`, value.CredentialID).Scan(&credentialOrganisationID, &credentialPurpose, &credentialName); err != nil {
				return model.Tool{}, databaseError(err)
			}
			if !postgresToolSecretBound(credentialOrganisationID, credentialPurpose, credentialName, value.OrganisationID, value.APIConnectionID) {
				return model.Tool{}, ErrConflict
			}
		}
		result, updateErr := tx.Exec(ctx, `UPDATE api_connections SET base_url=$3,allowed_hosts=$4,authentication_type=$5,auth_config=$6,credential_secret_id=nullif($7,'')::uuid,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2`, value.ProductID, value.APIConnectionID, value.BaseURL, []string{parsed.Hostname()}, authenticationType, value.UpstreamAuth, value.CredentialID)
		if updateErr != nil {
			return model.Tool{}, databaseError(updateErr)
		}
		if result.RowsAffected() != 1 {
			return model.Tool{}, ErrNotFound
		}
	}
	result, err := tx.Exec(ctx, `UPDATE tool_definitions SET description=$4,input_schema=$5,output_schema=$6,http_method=$7,request_mapping=$8,response_mapping=$9,request_example=$10,response_example=$11,authorization_policy=$12,timeout_ms=$13,http_path=$18,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND state='draft' AND backend_kind=$14 AND coalesce(api_connection_id::text,'')=$15 AND scope=$16 AND coalesce(owner_integration_id::text,'')=$17 AND coalesce(runtime_service_connection_id::text,'')=$19`, value.ProductID, value.ID, expected, value.Description, value.InputSchema, value.OutputSchema, value.HTTPMethod, value.RequestMapping, value.ResponseMapping, nullableToolExample(value.RequestExample), nullableToolExample(value.ResponseExample), value.AuthorizationPolicy, value.TimeoutMS, value.BackendKind, value.APIConnectionID, value.Scope, value.OwnerIntegrationID, value.HTTPPath, value.RuntimeServiceConnectionID)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	if result.RowsAffected() != 1 {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tool_definitions WHERE product_id=$1 AND id=$2)`, value.ProductID, value.ID).Scan(&exists); lookupErr != nil {
			return model.Tool{}, databaseError(lookupErr)
		}
		if exists {
			return model.Tool{}, ErrConflict
		}
		return model.Tool{}, ErrNotFound
	}
	if priorCredentialID != "" && priorCredentialID != value.CredentialID {
		var credentialOrganisationID, credentialPurpose, credentialName string
		if lookupErr := tx.QueryRow(ctx, `SELECT organisation_id::text,purpose,name FROM secrets WHERE id=$1 FOR UPDATE`, priorCredentialID).Scan(&credentialOrganisationID, &credentialPurpose, &credentialName); lookupErr != nil {
			return model.Tool{}, databaseError(lookupErr)
		}
		if !postgresToolSecretBound(credentialOrganisationID, credentialPurpose, credentialName, value.OrganisationID, value.APIConnectionID) {
			return model.Tool{}, ErrConflict
		}
		result, deleteErr := tx.Exec(ctx, `DELETE FROM secrets WHERE organisation_id=$1 AND id=$2`, value.OrganisationID, priorCredentialID)
		if deleteErr != nil {
			return model.Tool{}, databaseError(deleteErr)
		}
		if result.RowsAffected() != 1 {
			return model.Tool{}, ErrConflict
		}
	}
	updated, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.product_id=$1 AND t.id=$2`, value.ProductID, value.ID))
	if err != nil {
		return model.Tool{}, err
	}
	return updated, tx.Commit(ctx)
}

func (p *Postgres) RetireTool(ctx context.Context, productID, id string, expected int64) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Match UpdateTool's connection-before-definition lock order. The first
	// lookup is only a hint; the association is rechecked after both rows are
	// locked so concurrent edits fail closed without a deadlock.
	var hintedConnectionID string
	if err := tx.QueryRow(ctx, `SELECT coalesce(api_connection_id::text,'') FROM tool_definitions WHERE product_id=$1 AND id=$2`, productID, id).Scan(&hintedConnectionID); err != nil {
		return model.Tool{}, databaseError(err)
	}
	var connectionOrganisationID, credentialID string
	if hintedConnectionID != "" {
		if err := tx.QueryRow(ctx, `SELECT organisation_id::text,coalesce(credential_secret_id::text,'') FROM api_connections WHERE product_id=$1 AND id=$2 FOR UPDATE`, productID, hintedConnectionID).Scan(&connectionOrganisationID, &credentialID); err != nil {
			return model.Tool{}, databaseError(err)
		}
	}
	var organisationID, connectionID, state string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT organisation_id::text,coalesce(api_connection_id::text,''),state::text,revision FROM tool_definitions WHERE product_id=$1 AND id=$2 FOR UPDATE`, productID, id).Scan(&organisationID, &connectionID, &state, &revision); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if connectionID != hintedConnectionID || (connectionID != "" && connectionOrganisationID != organisationID) {
		return model.Tool{}, ErrConflict
	}
	if state == "retired" {
		value, scanErr := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.product_id=$1 AND t.id=$2`, productID, id))
		if scanErr != nil {
			return model.Tool{}, scanErr
		}
		return value, tx.Commit(ctx)
	}
	if revision != expected {
		return model.Tool{}, ErrConflict
	}
	if credentialID != "" {
		var credentialOrganisationID, credentialPurpose, credentialName string
		if lookupErr := tx.QueryRow(ctx, `SELECT organisation_id::text,purpose,name FROM secrets WHERE id=$1 FOR UPDATE`, credentialID).Scan(&credentialOrganisationID, &credentialPurpose, &credentialName); lookupErr != nil {
			return model.Tool{}, databaseError(lookupErr)
		}
		if !postgresToolSecretBound(credentialOrganisationID, credentialPurpose, credentialName, organisationID, connectionID) {
			return model.Tool{}, ErrConflict
		}
	}
	if connectionID != "" {
		if _, err := tx.Exec(ctx, `UPDATE api_connections SET authentication_type='none',auth_config='{"type":"none"}'::jsonb,credential_secret_id=NULL,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2`, productID, connectionID); err != nil {
			return model.Tool{}, databaseError(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE tool_definitions SET state='retired',revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2`, productID, id); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if credentialID != "" {
		result, deleteErr := tx.Exec(ctx, `DELETE FROM secrets WHERE organisation_id=$1 AND id=$2`, organisationID, credentialID)
		if deleteErr != nil {
			return model.Tool{}, databaseError(deleteErr)
		}
		if result.RowsAffected() != 1 {
			return model.Tool{}, ErrConflict
		}
	}
	value, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.product_id=$1 AND t.id=$2`, productID, id))
	if err != nil {
		return model.Tool{}, err
	}
	return value, tx.Commit(ctx)
}
