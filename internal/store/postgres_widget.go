package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

const widgetColumns = `id::text, deployment_id::text, organisation_id::text, name, state, allowed_origins, integration_ids::text[], integration_bindings, knowledge_bindings, appearance, revision, activated_at, created_at, updated_at`

func scanWidget(row interface{ Scan(...any) error }) (model.Widget, error) {
	var value model.Widget
	var bindings json.RawMessage
	var knowledgeBindings json.RawMessage
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.State, &value.AllowedOrigins, &value.IntegrationIDs, &bindings, &knowledgeBindings, &value.Appearance, &value.Revision, &value.ActivatedAt, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(bindings, &value.IntegrationBindings)
	}
	if err == nil {
		err = json.Unmarshal(knowledgeBindings, &value.KnowledgeBindings)
	}
	return value, databaseError(err)
}

func (p *Postgres) Widgets(ctx context.Context, deploymentID string) ([]model.Widget, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+widgetColumns+` FROM widgets WHERE deployment_id=$1 ORDER BY created_at`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.Widget, 0)
	for rows.Next() {
		value, scanErr := scanWidget(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) Widget(ctx context.Context, deploymentID, id string) (model.Widget, error) {
	return scanWidget(p.pool.QueryRow(ctx, `SELECT `+widgetColumns+` FROM widgets WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateWidget(ctx context.Context, value model.Widget) (model.Widget, error) {
	bindings, err := json.Marshal(value.IntegrationBindings)
	if err != nil {
		return model.Widget{}, err
	}
	knowledgeBindings, err := json.Marshal(value.KnowledgeBindings)
	if err != nil {
		return model.Widget{}, err
	}
	return scanWidget(p.pool.QueryRow(ctx, `INSERT INTO widgets(id,deployment_id,organisation_id,name,state,allowed_origins,integration_ids,integration_bindings,knowledge_bindings,appearance) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+widgetColumns, value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.State, value.AllowedOrigins, value.IntegrationIDs, bindings, knowledgeBindings, value.Appearance))
}

func (p *Postgres) UpdateWidget(ctx context.Context, value model.Widget, expected int64) (model.Widget, error) {
	bindings, marshalErr := json.Marshal(value.IntegrationBindings)
	if marshalErr != nil {
		return model.Widget{}, marshalErr
	}
	knowledgeBindings, marshalErr := json.Marshal(value.KnowledgeBindings)
	if marshalErr != nil {
		return model.Widget{}, marshalErr
	}
	updated, err := scanWidget(p.pool.QueryRow(ctx, `UPDATE widgets SET name=$3,state=$4,allowed_origins=$5,integration_ids=$6,integration_bindings=$7,knowledge_bindings=$8,appearance=$9,activated_at=$10,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$11 RETURNING `+widgetColumns, value.DeploymentID, value.ID, value.Name, value.State, value.AllowedOrigins, value.IntegrationIDs, bindings, knowledgeBindings, value.Appearance, value.ActivatedAt, expected))
	if err == ErrNotFound {
		if _, lookupErr := p.Widget(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.Widget{}, ErrConflict
		}
	}
	return updated, err
}

func scanWidgetSecret(row interface{ Scan(...any) error }) (model.WidgetSecret, error) {
	var value model.WidgetSecret
	err := row.Scan(&value.ID, &value.WidgetID, &value.Digest, &value.Fingerprint, &value.LastUsedAt, &value.RevokedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) CreateWidgetSecret(ctx context.Context, value model.WidgetSecret) (model.WidgetSecret, error) {
	return scanWidgetSecret(p.pool.QueryRow(ctx, `INSERT INTO widget_secrets(id,widget_id,secret_digest,fingerprint) VALUES($1,$2,$3,$4) RETURNING id::text,widget_id::text,secret_digest,fingerprint,last_used_at,revoked_at,created_at`, value.ID, value.WidgetID, value.Digest, value.Fingerprint))
}

func (p *Postgres) WidgetSecrets(ctx context.Context, widgetID string) ([]model.WidgetSecret, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,widget_id::text,secret_digest,fingerprint,last_used_at,revoked_at,created_at FROM widget_secrets WHERE widget_id=$1 ORDER BY created_at DESC`, widgetID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.WidgetSecret, 0)
	for rows.Next() {
		value, scanErr := scanWidgetSecret(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		value.Digest = nil
		values = append(values, value)
	}
	return values, rows.Err()
}

func (p *Postgres) WidgetSecretByDigest(ctx context.Context, widgetID string, digest []byte) (model.WidgetSecret, error) {
	return scanWidgetSecret(p.pool.QueryRow(ctx, `UPDATE widget_secrets SET last_used_at=now() WHERE widget_id=$1 AND secret_digest=$2 AND revoked_at IS NULL RETURNING id::text,widget_id::text,secret_digest,fingerprint,last_used_at,revoked_at,created_at`, widgetID, digest))
}

func (p *Postgres) RevokeWidgetSecret(ctx context.Context, widgetID, id string, now time.Time) (model.WidgetSecret, error) {
	return scanWidgetSecret(p.pool.QueryRow(ctx, `UPDATE widget_secrets SET revoked_at=coalesce(revoked_at,$3) WHERE widget_id=$1 AND id=$2 RETURNING id::text,widget_id::text,secret_digest,fingerprint,last_used_at,revoked_at,created_at`, widgetID, id, now))
}

func (p *Postgres) CreateWidgetBootstrap(ctx context.Context, value model.WidgetBootstrap) error {
	if value.Kind == "" {
		value.Kind = model.WidgetSessionKindCustomer
	}
	sessionContext, err := json.Marshal(value.Context)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO widget_bootstrap_tokens(token_digest,widget_id,kind,user_id,customer_organisation_id,session_context,origin,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.Digest, value.WidgetID, value.Kind, value.UserID, value.CustomerOrganisationID, sessionContext, value.Origin, value.ExpiresAt)
	return databaseError(err)
}

func (p *Postgres) WidgetBootstrap(ctx context.Context, digest []byte) (model.WidgetBootstrap, error) {
	var value model.WidgetBootstrap
	var sessionContext json.RawMessage
	err := p.pool.QueryRow(ctx, `SELECT token_digest,widget_id::text,kind,user_id,customer_organisation_id,session_context,origin,expires_at,used_at,created_at FROM widget_bootstrap_tokens WHERE token_digest=$1`, digest).Scan(&value.Digest, &value.WidgetID, &value.Kind, &value.UserID, &value.CustomerOrganisationID, &sessionContext, &value.Origin, &value.ExpiresAt, &value.UsedAt, &value.CreatedAt)
	if err == nil {
		err = json.Unmarshal(sessionContext, &value.Context)
	}
	return value, databaseError(err)
}

func (p *Postgres) ConsumeWidgetBootstrap(ctx context.Context, digest []byte, now time.Time) (model.WidgetBootstrap, error) {
	var value model.WidgetBootstrap
	var sessionContext json.RawMessage
	err := p.pool.QueryRow(ctx, `UPDATE widget_bootstrap_tokens SET used_at=$2 WHERE token_digest=$1 AND used_at IS NULL AND expires_at>$2 RETURNING token_digest,widget_id::text,kind,user_id,customer_organisation_id,session_context,origin,expires_at,used_at,created_at`, digest, now).Scan(&value.Digest, &value.WidgetID, &value.Kind, &value.UserID, &value.CustomerOrganisationID, &sessionContext, &value.Origin, &value.ExpiresAt, &value.UsedAt, &value.CreatedAt)
	if err == nil {
		err = json.Unmarshal(sessionContext, &value.Context)
	}
	return value, databaseError(err)
}

func scanWidgetSession(row interface{ Scan(...any) error }) (model.WidgetSession, error) {
	var value model.WidgetSession
	var sessionContext json.RawMessage
	err := row.Scan(&value.ID, &value.WidgetID, &value.Kind, &value.Digest, &value.UserID, &value.CustomerOrganisationID, &sessionContext, &value.Origin, &value.ExpiresAt, &value.RevokedAt, &value.CreatedAt, &value.LastSeenAt)
	if err == nil {
		err = json.Unmarshal(sessionContext, &value.Context)
	}
	return value, databaseError(err)
}

func (p *Postgres) CreateWidgetSession(ctx context.Context, value model.WidgetSession) (model.WidgetSession, error) {
	if value.Kind == "" {
		value.Kind = model.WidgetSessionKindCustomer
	}
	sessionContext, err := json.Marshal(value.Context)
	if err != nil {
		return model.WidgetSession{}, err
	}
	return scanWidgetSession(p.pool.QueryRow(ctx, `INSERT INTO widget_sessions(id,widget_id,kind,token_digest,user_id,customer_organisation_id,session_context,origin,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text,widget_id::text,kind,token_digest,user_id,customer_organisation_id,session_context,origin,expires_at,revoked_at,created_at,last_seen_at`, value.ID, value.WidgetID, value.Kind, value.Digest, value.UserID, value.CustomerOrganisationID, sessionContext, value.Origin, value.ExpiresAt))
}

func (p *Postgres) WidgetSessions(ctx context.Context, widgetID string) ([]model.WidgetSession, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,widget_id::text,kind,token_digest,user_id,customer_organisation_id,session_context,origin,expires_at,revoked_at,created_at,last_seen_at FROM widget_sessions WHERE widget_id=$1 ORDER BY created_at DESC LIMIT 100`, widgetID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.WidgetSession, 0)
	for rows.Next() {
		value, scanErr := scanWidgetSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		value.Digest = nil
		values = append(values, value)
	}
	return values, databaseError(rows.Err())
}

func (p *Postgres) WidgetSessionByDigest(ctx context.Context, digest []byte, now time.Time) (model.WidgetSession, error) {
	return scanWidgetSession(p.pool.QueryRow(ctx, `UPDATE widget_sessions SET last_seen_at=$2 WHERE token_digest=$1 AND revoked_at IS NULL AND expires_at>$2 RETURNING id::text,widget_id::text,kind,token_digest,user_id,customer_organisation_id,session_context,origin,expires_at,revoked_at,created_at,last_seen_at`, digest, now))
}

func (p *Postgres) RevokeWidgetSession(ctx context.Context, widgetID, id string, now time.Time) (model.WidgetSession, error) {
	value, err := scanWidgetSession(p.pool.QueryRow(ctx, `UPDATE widget_sessions SET revoked_at=coalesce(revoked_at,$3) WHERE widget_id=$1 AND id=$2 RETURNING id::text,widget_id::text,kind,token_digest,user_id,customer_organisation_id,session_context,origin,expires_at,revoked_at,created_at,last_seen_at`, widgetID, id, now))
	if err == nil {
		_, err = p.pool.Exec(ctx, `DELETE FROM widget_agent_messages WHERE session_id=$1`, id)
	}
	return value, databaseError(err)
}

func (p *Postgres) RevokeWidgetSessions(ctx context.Context, widgetID string, now time.Time) error {
	_, err := p.pool.Exec(ctx, `WITH revoked AS (UPDATE widget_sessions SET revoked_at=$2 WHERE widget_id=$1 AND revoked_at IS NULL RETURNING id) DELETE FROM widget_agent_messages WHERE session_id IN (SELECT id FROM revoked)`, widgetID, now)
	return databaseError(err)
}

func (p *Postgres) WidgetAgentMessages(ctx context.Context, sessionID string, limit int) ([]model.WidgetAgentMessage, error) {
	if limit < 1 {
		return []model.WidgetAgentMessage{}, nil
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := p.pool.Query(ctx, `SELECT id::text,session_id::text,role,content,created_at FROM widget_agent_messages WHERE session_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, sessionID, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	values := make([]model.WidgetAgentMessage, 0, limit)
	for rows.Next() {
		var value model.WidgetAgentMessage
		if err := rows.Scan(&value.ID, &value.SessionID, &value.Role, &value.Content, &value.CreatedAt); err != nil {
			return nil, databaseError(err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
	return values, nil
}

func (p *Postgres) AppendWidgetAgentMessages(ctx context.Context, values []model.WidgetAgentMessage) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, value := range values {
		if _, err = tx.Exec(ctx, `INSERT INTO widget_agent_messages(id,session_id,role,content,created_at) VALUES($1,$2,$3,$4,$5)`, value.ID, value.SessionID, value.Role, value.Content, value.CreatedAt); err != nil {
			return databaseError(err)
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM widget_agent_messages WHERE session_id=$1 AND id NOT IN (SELECT id FROM widget_agent_messages WHERE session_id=$1 ORDER BY created_at DESC,id DESC LIMIT 20)`, values[0].SessionID)
	if err != nil {
		return databaseError(err)
	}
	return databaseError(tx.Commit(ctx))
}
