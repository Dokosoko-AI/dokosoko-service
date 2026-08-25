package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"net/url"
	"time"
)

const toolSelect = `SELECT t.id::text, t.organisation_id::text, t.product_id::text, t.scope, coalesce(t.owner_integration_id::text, ''), coalesce(t.runtime_service_connection_id::text,''), t.http_path, t.namespace, t.name, t.description, t.input_schema, t.output_schema, t.state::text, t.revision, coalesce(t.api_connection_id::text, ''), coalesce(c.base_url, ''), t.http_method, CASE WHEN t.backend_kind = 'mcp' THEN '{"type":"none"}'::jsonb ELSE coalesce(c.auth_config || jsonb_build_object('type', c.authentication_type), '{"type":"delegated_oauth"}'::jsonb) END, coalesce(c.credential_secret_id::text, ''), coalesce(secret.fingerprint, ''), t.request_mapping, t.response_mapping, t.request_example, t.response_example, t.authorization_policy, t.timeout_ms, t.backend_kind, coalesce(t.mcp_connection_id::text, ''), t.upstream_tool_name, t.upstream_schema_hash, t.upstream_annotations, t.upstream_drifted, t.created_at, t.updated_at FROM tool_definitions t LEFT JOIN api_connections c ON c.id = t.api_connection_id LEFT JOIN secrets secret ON secret.id = c.credential_secret_id`

func scanTool(row interface{ Scan(...any) error }) (model.Tool, error) {
	var value model.Tool
	var requestExample, responseExample []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.Scope, &value.OwnerIntegrationID, &value.RuntimeServiceConnectionID, &value.HTTPPath, &value.Namespace, &value.Name, &value.Description, &value.InputSchema, &value.OutputSchema, &value.State, &value.Revision, &value.APIConnectionID, &value.BaseURL, &value.HTTPMethod, &value.UpstreamAuth, &value.CredentialID, &value.CredentialFingerprint, &value.RequestMapping, &value.ResponseMapping, &requestExample, &responseExample, &value.AuthorizationPolicy, &value.TimeoutMS, &value.BackendKind, &value.MCPConnectionID, &value.UpstreamToolName, &value.UpstreamSchemaHash, &value.UpstreamAnnotations, &value.UpstreamDrifted, &value.CreatedAt, &value.UpdatedAt)
	value.RequestExample = append(json.RawMessage(nil), requestExample...)
	value.ResponseExample = append(json.RawMessage(nil), responseExample...)
	value.CredentialPresent = value.CredentialID != ""
	return value, databaseError(err)
}

func nullableToolExample(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}

func (p *Postgres) Tools(ctx context.Context, productID string, publishedOnly bool) ([]model.Tool, error) {
	query := toolSelect + ` WHERE t.product_id = $1`
	if publishedOnly {
		query += ` AND t.state = 'published'`
	}
	query += ` ORDER BY t.namespace, t.name`
	rows, err := p.pool.Query(ctx, query, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.Tool, 0)
	for rows.Next() {
		value, err := scanTool(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	for index := range result {
		result[index], err = p.enrichToolRuntimeTargets(ctx, result[index])
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *Postgres) Tool(ctx context.Context, productID, id string) (model.Tool, error) {
	value, err := scanTool(p.pool.QueryRow(ctx, toolSelect+` WHERE t.product_id = $1 AND t.id = $2`, productID, id))
	if err != nil {
		return model.Tool{}, err
	}
	return p.enrichToolRuntimeTargets(ctx, value)
}

func (p *Postgres) CreateTool(ctx context.Context, value model.Tool) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value.Scope, value.OwnerIntegrationID, err = normalizeToolOwnership(value.Scope, value.OwnerIntegrationID)
	if err != nil {
		return model.Tool{}, err
	}
	if value.Scope == model.ToolScopeAPI {
		var ownerExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3)`, value.OwnerIntegrationID, value.ProductID, value.OrganisationID).Scan(&ownerExists); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if !ownerExists {
			return model.Tool{}, ErrConflict
		}
	}
	if value.BackendKind == "" {
		value.BackendKind = "http"
	}
	if value.BackendKind == "http" && value.RuntimeServiceConnectionID == "" {
		authenticationType := "delegated_oauth"
		var upstreamAuth struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(value.UpstreamAuth, &upstreamAuth) == nil && upstreamAuth.Type != "" {
			authenticationType = upstreamAuth.Type
		}
		parsedBase := value.BaseURL
		allowedHost := ""
		if parsed, parseErr := url.Parse(value.BaseURL); parseErr == nil {
			allowedHost = parsed.Hostname()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_connections(id, organisation_id, product_id, name, base_url, allowed_hosts, authentication_type, auth_config, credential_secret_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid)`, value.APIConnectionID, value.OrganisationID, value.ProductID, value.Namespace+"."+value.Name, parsedBase, []string{allowedHost}, authenticationType, value.UpstreamAuth, value.CredentialID); err != nil {
			return model.Tool{}, databaseError(err)
		}
	}
	if value.RuntimeServiceConnectionID != "" {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtime_service_connections WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3 AND integration_id=$4 AND state='active' AND EXISTS(SELECT 1 FROM runtime_service_connection_revisions WHERE connection_id=$1 AND is_current))`, value.RuntimeServiceConnectionID, value.ProductID, value.OrganisationID, value.OwnerIntegrationID).Scan(&valid); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if !valid || value.Scope != model.ToolScopeAPI || value.APIConnectionID != "" || value.HTTPPath == "" {
			return model.Tool{}, ErrConflict
		}
	}
	requestMapping, responseMapping := value.RequestMapping, value.ResponseMapping
	if len(requestMapping) == 0 {
		requestMapping = json.RawMessage(`{}`)
	}
	if len(responseMapping) == 0 {
		responseMapping = json.RawMessage(`{}`)
	}
	_, err = tx.Exec(ctx, `INSERT INTO tool_definitions(id, organisation_id, product_id, scope, owner_integration_id, runtime_service_connection_id, http_path, namespace, name, description, input_schema, output_schema, state, api_connection_id, http_method, request_mapping, response_mapping, request_example, response_example, authorization_policy, timeout_ms, backend_kind, mcp_connection_id, upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted) VALUES ($1,$2,$3,$4,nullif($5,'')::uuid,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,'draft',nullif($13,'')::uuid,$14,$15,$16,$17,$18,$19,$20,$21,nullif($22,'')::uuid,$23,$24,$25,$26)`, value.ID, value.OrganisationID, value.ProductID, value.Scope, value.OwnerIntegrationID, value.RuntimeServiceConnectionID, value.HTTPPath, value.Namespace, value.Name, value.Description, value.InputSchema, value.OutputSchema, value.APIConnectionID, value.HTTPMethod, requestMapping, responseMapping, nullableToolExample(value.RequestExample), nullableToolExample(value.ResponseExample), value.AuthorizationPolicy, value.TimeoutMS, value.BackendKind, value.MCPConnectionID, value.UpstreamToolName, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	created, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.id = $1`, value.ID))
	if err != nil {
		return model.Tool{}, err
	}
	return created, tx.Commit(ctx)
}

func (p *Postgres) UpdateImportedTool(ctx context.Context, value model.Tool, expected int64) (model.Tool, error) {
	updated, err := scanTool(p.pool.QueryRow(ctx, `UPDATE tool_definitions SET description=$4, input_schema=$5, output_schema=$6, authorization_policy=$7, timeout_ms=$8, upstream_schema_hash=$9, upstream_annotations=$10, upstream_drifted=$11, revision=revision+1, updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND backend_kind='mcp' AND state='draft' RETURNING id::text, organisation_id::text, product_id::text, scope, coalesce(owner_integration_id::text, ''), '', '', namespace, name, description, input_schema, output_schema, state::text, revision, coalesce(api_connection_id::text, ''), '', http_method, '{"type":"none"}'::jsonb, '', '', request_mapping, response_mapping, request_example, response_example, authorization_policy, timeout_ms, backend_kind, coalesce(mcp_connection_id::text, ''), upstream_tool_name, upstream_schema_hash, upstream_annotations, upstream_drifted, created_at, updated_at`, value.ProductID, value.ID, expected, value.Description, value.InputSchema, value.OutputSchema, value.AuthorizationPolicy, value.TimeoutMS, value.UpstreamSchemaHash, value.UpstreamAnnotations, value.UpstreamDrifted))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.Tool(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Tool{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) MarkImportedToolDrift(ctx context.Context, productID, id string, drifted bool) (model.Tool, error) {
	query := `WITH updated AS (UPDATE tool_definitions SET upstream_drifted=$3,updated_at=now() WHERE product_id=$1 AND id=$2 AND backend_kind='mcp' RETURNING id) ` + toolSelect + ` WHERE t.id IN (SELECT id FROM updated)`
	return scanTool(p.pool.QueryRow(ctx, query, productID, id, drifted))
}

func (p *Postgres) PublishTool(ctx context.Context, productID, id string, expected int64, actorID string) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var organisationID, scope, ownerIntegrationID, backendKind, connectionID, mcpConnectionID, upstreamToolName, upstreamSchemaHash, runtimeConnectionID, httpPath string
	var outputSchema, requestMapping, responseMapping, authorizationPolicy []byte
	var timeoutMS int
	if err := tx.QueryRow(ctx, `SELECT organisation_id::text, scope, coalesce(owner_integration_id::text,''), backend_kind, coalesce(api_connection_id::text,''), coalesce(mcp_connection_id::text,''), upstream_tool_name, upstream_schema_hash, output_schema, request_mapping, response_mapping, authorization_policy, timeout_ms, coalesce(runtime_service_connection_id::text,''), http_path FROM tool_definitions WHERE product_id = $1 AND id = $2 AND revision = $3 AND state='draft' FOR UPDATE`, productID, id, expected).Scan(&organisationID, &scope, &ownerIntegrationID, &backendKind, &connectionID, &mcpConnectionID, &upstreamToolName, &upstreamSchemaHash, &outputSchema, &requestMapping, &responseMapping, &authorizationPolicy, &timeoutMS, &runtimeConnectionID, &httpPath); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if runtimeConnectionID != "" {
		var lockedOwner, state string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text,state FROM runtime_service_connections WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3 FOR SHARE`, runtimeConnectionID, productID, organisationID).Scan(&lockedOwner, &state); err != nil {
			return model.Tool{}, databaseError(err)
		}
		if scope != model.ToolScopeAPI || lockedOwner != ownerIntegrationID || state != "active" || connectionID != "" {
			return model.Tool{}, ErrConflict
		}
	}
	var releaseID string
	if err := tx.QueryRow(ctx, `INSERT INTO tool_releases(organisation_id,product_id,tool_definition_id,api_connection_id,version,request_mapping,output_schema,response_mapping,authorization_policy,timeout_ms,rate_limit,published_by,published_at,backend_kind,mcp_connection_id,upstream_tool_name,upstream_schema_hash,scope,owner_integration_id,runtime_service_connection_id,http_path)
		VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,'{"requests":60,"window_seconds":60}',nullif($11,'')::uuid,now(),$12,nullif($13,'')::uuid,$14,$15,$16,nullif($17,'')::uuid,nullif($18,'')::uuid,$19)
		RETURNING id::text`, organisationID, productID, id, connectionID, expected+1, requestMapping, outputSchema, responseMapping, authorizationPolicy, timeoutMS, actorID, backendKind, mcpConnectionID, upstreamToolName, upstreamSchemaHash, scope, ownerIntegrationID, runtimeConnectionID, httpPath).Scan(&releaseID); err != nil {
		return model.Tool{}, databaseError(err)
	}
	if runtimeConnectionID != "" {
		result, err := tx.Exec(ctx, `INSERT INTO tool_release_runtime_targets(tool_release_id,environment_id,connection_revision_id,runtime_service_connection_id)
			SELECT $1,environment_id,id,connection_id FROM runtime_service_connection_revisions WHERE connection_id=$2 AND is_current`, releaseID, runtimeConnectionID)
		if err != nil {
			return model.Tool{}, databaseError(err)
		}
		if result.RowsAffected() == 0 {
			return model.Tool{}, ErrConflict
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE tool_definitions SET state = 'published', revision = revision + 1, updated_at = now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND state='draft'`, productID, id, expected); err != nil {
		return model.Tool{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Tool{}, err
	}
	return p.Tool(ctx, productID, id)
}

func (p *Postgres) CreateToolTestConfirmation(ctx context.Context, value model.ToolTestConfirmation) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO tool_test_confirmations(id,organisation_id,product_id,tool_id,tool_revision,argument_hash,nonce_digest,actor_id,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10
		FROM tool_definitions
		WHERE product_id=$3 AND id=$4 AND revision=$5`, value.ID, value.OrganisationID, value.ProductID, value.ToolID, value.ToolRevision, value.ArgumentHash, value.NonceDigest, value.ActorID, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) ConsumeToolTestConfirmation(ctx context.Context, digest []byte, productID, toolID string, revision int64, argumentHash []byte, actorID, consumptionID string, now time.Time) (model.ToolTestConfirmation, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ToolTestConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.ToolTestConfirmation
	err = tx.QueryRow(ctx, `SELECT c.id::text,c.organisation_id::text,c.product_id::text,c.tool_id::text,c.tool_revision,c.argument_hash,c.nonce_digest,c.actor_id,c.expires_at,c.created_at
		FROM tool_test_confirmations c
		JOIN tool_definitions t ON t.product_id=c.product_id AND t.id=c.tool_id AND t.revision=c.tool_revision
		WHERE c.nonce_digest=$1 AND c.product_id=$2 AND c.tool_id=$3 AND c.tool_revision=$4 AND c.argument_hash=$5 AND c.actor_id=$6 AND c.expires_at>$7
		FOR UPDATE OF c`, digest, productID, toolID, revision, argumentHash, actorID, now).Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ToolID, &value.ToolRevision, &value.ArgumentHash, &value.NonceDigest, &value.ActorID, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ToolTestConfirmation{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tool_test_confirmation_consumptions(id,confirmation_id,consumed_at) VALUES($1,$2,$3)`, consumptionID, value.ID, now); err != nil {
		return model.ToolTestConfirmation{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ToolTestConfirmation{}, err
	}
	return value, nil
}

func (p *Postgres) CreateManagedOperationConfirmation(ctx context.Context, value model.ManagedOperationConfirmation) error {
	result, err := p.pool.Exec(ctx, `INSERT INTO managed_operation_confirmations(id,organisation_id,product_id,operation_key,argument_hash,nonce_digest,actor_id,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9
		FROM products
		WHERE id=$3 AND organisation_id=$2`, value.ID, value.OrganisationID, value.ProductID, value.OperationKey, value.ArgumentHash, value.NonceDigest, value.ActorID, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (p *Postgres) ConsumeManagedOperationConfirmation(ctx context.Context, digest []byte, productID, operationKey string, argumentHash []byte, actorID, consumptionID string, now time.Time) (model.ManagedOperationConfirmation, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ManagedOperationConfirmation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var value model.ManagedOperationConfirmation
	err = tx.QueryRow(ctx, `SELECT id::text,organisation_id::text,product_id::text,operation_key,argument_hash,nonce_digest,actor_id,expires_at,created_at
		FROM managed_operation_confirmations
		WHERE nonce_digest=$1 AND product_id=$2 AND operation_key=$3 AND argument_hash=$4 AND actor_id=$5 AND expires_at>$6
		FOR UPDATE`, digest, productID, operationKey, argumentHash, actorID, now).Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.OperationKey, &value.ArgumentHash, &value.NonceDigest, &value.ActorID, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ManagedOperationConfirmation{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO managed_operation_confirmation_consumptions(id,confirmation_id,consumed_at) VALUES($1,$2,$3)`, consumptionID, value.ID, now); err != nil {
		return model.ManagedOperationConfirmation{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ManagedOperationConfirmation{}, err
	}
	return value, nil
}

func (p *Postgres) DeleteExpiredToolTestData(ctx context.Context, now time.Time, limit int) (int64, error) {
	limit = boundedToolTestCleanupLimit(limit)
	if limit == 0 {
		return 0, nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	confirmationResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM tool_test_confirmations WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM tool_test_confirmations AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	managedConfirmationResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM managed_operation_confirmations WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM managed_operation_confirmations AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	runResult, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM tool_test_runs WHERE expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) DELETE FROM tool_test_runs AS target USING expired WHERE target.id=expired.id`, now, limit)
	if err != nil {
		return 0, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return confirmationResult.RowsAffected() + managedConfirmationResult.RowsAffected() + runResult.RowsAffected(), nil
}

func (p *Postgres) AppendToolTestRun(ctx context.Context, value model.ToolTestRun) error {
	requestShape, err := json.Marshal(value.RequestShape)
	if err != nil {
		return err
	}
	responseShape, err := json.Marshal(value.ResponseShape)
	if err != nil {
		return err
	}
	findings, err := json.Marshal(value.Findings)
	if err != nil {
		return err
	}
	result, err := p.pool.Exec(ctx, `INSERT INTO tool_test_runs(id,organisation_id,product_id,tool_id,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,upstream_status_code,response_bytes,request_shape,response_shape,findings,duration_ms,expires_at,created_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,nullif($15,0),$16,$17,CASE WHEN $18::jsonb='null'::jsonb THEN NULL ELSE $18::jsonb END,$19,$20,$21,$22
		FROM tool_definitions
		WHERE product_id=$3 AND id=$4`, value.ID, value.OrganisationID, value.ProductID, value.ToolID, value.ToolRevision, value.ToolName, value.ActorID, value.RequestID, value.ArgumentHash, value.Method, value.AuthenticationType, value.Outcome, value.Phase, value.NetworkCallPerformed, value.UpstreamStatusCode, value.ResponseBytes, requestShape, responseShape, findings, value.DurationMS, value.ExpiresAt, value.CreatedAt)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func scanToolTestRun(row interface{ Scan(...any) error }) (model.ToolTestRun, error) {
	var value model.ToolTestRun
	var requestShape, responseShape, findings []byte
	err := row.Scan(&value.ID, &value.OrganisationID, &value.ProductID, &value.ToolID, &value.ToolRevision, &value.ToolName, &value.ActorID, &value.RequestID, &value.ArgumentHash, &value.Method, &value.AuthenticationType, &value.Outcome, &value.Phase, &value.NetworkCallPerformed, &value.UpstreamStatusCode, &value.ResponseBytes, &requestShape, &responseShape, &findings, &value.DurationMS, &value.ExpiresAt, &value.CreatedAt)
	if err != nil {
		return model.ToolTestRun{}, databaseError(err)
	}
	if err := json.Unmarshal(requestShape, &value.RequestShape); err != nil {
		return model.ToolTestRun{}, err
	}
	if len(responseShape) > 0 && string(responseShape) != "null" {
		var shape model.JSONShape
		if err := json.Unmarshal(responseShape, &shape); err != nil {
			return model.ToolTestRun{}, err
		}
		value.ResponseShape = &shape
	}
	if err := json.Unmarshal(findings, &value.Findings); err != nil {
		return model.ToolTestRun{}, err
	}
	return value, nil
}

func (p *Postgres) ToolTestRuns(ctx context.Context, productID, toolID string, now time.Time) ([]model.ToolTestRun, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,organisation_id::text,product_id::text,tool_id::text,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,coalesce(upstream_status_code,0),coalesce(response_bytes,0),request_shape,coalesce(response_shape,'null'::jsonb),findings,duration_ms,expires_at,created_at
		FROM tool_test_runs WHERE product_id=$1 AND ($2='' OR tool_id::text=$2) AND expires_at>$3 ORDER BY created_at DESC LIMIT 100`, productID, toolID, now)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ToolTestRun, 0)
	for rows.Next() {
		value, err := scanToolTestRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) ToolTestRun(ctx context.Context, productID, toolID, runID string, now time.Time) (model.ToolTestRun, error) {
	return scanToolTestRun(p.pool.QueryRow(ctx, `SELECT id::text,organisation_id::text,product_id::text,tool_id::text,tool_revision,tool_name,actor_id,request_id,argument_hash,method,authentication_type,outcome,phase,network_call_performed,coalesce(upstream_status_code,0),coalesce(response_bytes,0),request_shape,coalesce(response_shape,'null'::jsonb),findings,duration_ms,expires_at,created_at
		FROM tool_test_runs WHERE product_id=$1 AND tool_id=$2 AND id=$3 AND expires_at>$4`, productID, toolID, runID, now))
}
