package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
	"github.com/jackc/pgx/v5"
)

type postgresNativeStateTx struct {
	ctx      context.Context
	tx       pgx.Tx
	pluginID string
	scope    nativepluginstate.Scope
	now      time.Time
}

func (p *Postgres) StateTransaction(ctx context.Context, pluginID string, scope nativepluginstate.Scope, fn func(nativepluginstate.Transaction) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pluginID); err != nil {
		return databaseError(err)
	}
	stateTx := &postgresNativeStateTx{ctx: ctx, tx: tx, pluginID: pluginID, scope: scope, now: time.Now().UTC()}
	if err := fn(stateTx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) PluginStateTransaction(ctx context.Context, pluginID string, fn func(nativepluginstate.PluginTransaction) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pluginID); err != nil {
		return databaseError(err)
	}
	pluginTx := &postgresNativePluginTx{postgresNativeStateTx: postgresNativeStateTx{ctx: ctx, tx: tx, pluginID: pluginID, scope: nativepluginstate.Scope{Kind: string(nativeplugin.StatePlugin)}, now: time.Now().UTC()}}
	if err := pluginTx.loadScopes(); err != nil {
		return err
	}
	if err := fn(pluginTx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (tx *postgresNativeStateTx) Get(key string) (nativeplugin.StateValue, error) {
	var value nativeplugin.StateValue
	var expiresAt *time.Time
	err := tx.tx.QueryRow(tx.ctx, `SELECT state_key,value,revision,expires_at FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key=$4 AND (expires_at IS NULL OR expires_at>$5)`, tx.pluginID, tx.scope.Kind, tx.scope.ID, key, tx.now).Scan(&value.Key, &value.Value, &value.Revision, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nativeplugin.StateValue{}, nativeplugin.ErrStateNotFound
	}
	value.ExpiresAt = expiresAt
	return value, databaseError(err)
}

func (tx *postgresNativeStateTx) Put(key string, raw json.RawMessage, options nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	if _, err := tx.tx.Exec(tx.ctx, `DELETE FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND expires_at IS NOT NULL AND expires_at<=$4`, tx.pluginID, tx.scope.Kind, tx.scope.ID, tx.now); err != nil {
		return nativeplugin.StateValue{}, databaseError(err)
	}
	if options.ExpectedRevision == 0 {
		var value nativeplugin.StateValue
		var expiresAt *time.Time
		err := tx.tx.QueryRow(tx.ctx, `INSERT INTO native_plugin_state(plugin_id,scope_kind,scope_id,state_key,value,revision,expires_at) SELECT $1,$2,$3,$4,$5,1,$6 WHERE (SELECT count(*) FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key NOT LIKE '__dokosoko/%' AND (expires_at IS NULL OR expires_at>$7)) < $8 ON CONFLICT DO NOTHING RETURNING state_key,value,revision,expires_at`, tx.pluginID, tx.scope.Kind, tx.scope.ID, key, raw, options.ExpiresAt, tx.now, nativepluginstate.MaxScopeRecords).Scan(&value.Key, &value.Value, &value.Revision, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if lookupErr := tx.tx.QueryRow(tx.ctx, `SELECT EXISTS(SELECT 1 FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key=$4 AND (expires_at IS NULL OR expires_at>$5))`, tx.pluginID, tx.scope.Kind, tx.scope.ID, key, tx.now).Scan(&exists); lookupErr != nil {
				return nativeplugin.StateValue{}, databaseError(lookupErr)
			}
			if exists {
				return nativeplugin.StateValue{}, nativeplugin.ErrStateConflict
			}
			return nativeplugin.StateValue{}, nativeplugin.ErrStateLimit
		}
		value.ExpiresAt = expiresAt
		return value, databaseError(err)
	}
	if options.ExpectedRevision > 0 {
		var value nativeplugin.StateValue
		var expiresAt *time.Time
		err := tx.tx.QueryRow(tx.ctx, `UPDATE native_plugin_state SET value=$5,revision=revision+1,expires_at=$6,updated_at=now() WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key=$4 AND revision=$7 AND (expires_at IS NULL OR expires_at>$8) RETURNING state_key,value,revision,expires_at`, tx.pluginID, tx.scope.Kind, tx.scope.ID, key, raw, options.ExpiresAt, options.ExpectedRevision, tx.now).Scan(&value.Key, &value.Value, &value.Revision, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nativeplugin.StateValue{}, nativeplugin.ErrStateConflict
		}
		value.ExpiresAt = expiresAt
		return value, databaseError(err)
	}
	var value nativeplugin.StateValue
	var expiresAt *time.Time
	err := tx.tx.QueryRow(tx.ctx, `INSERT INTO native_plugin_state(plugin_id,scope_kind,scope_id,state_key,value,revision,expires_at) SELECT $1,$2,$3,$4,$5,1,$6 WHERE $4 LIKE '__dokosoko/%' OR (SELECT count(*) FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key NOT LIKE '__dokosoko/%' AND (expires_at IS NULL OR expires_at>$7)) < $8 ON CONFLICT(plugin_id,scope_kind,scope_id,state_key) DO UPDATE SET value=excluded.value,revision=native_plugin_state.revision+1,expires_at=excluded.expires_at,updated_at=now() RETURNING state_key,value,revision,expires_at`, tx.pluginID, tx.scope.Kind, tx.scope.ID, key, raw, options.ExpiresAt, tx.now, nativepluginstate.MaxScopeRecords).Scan(&value.Key, &value.Value, &value.Revision, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nativeplugin.StateValue{}, nativeplugin.ErrStateLimit
	}
	value.ExpiresAt = expiresAt
	return value, databaseError(err)
}

func (tx *postgresNativeStateTx) Delete(key string, expected int64) error {
	query := `DELETE FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND state_key=$4 AND (expires_at IS NULL OR expires_at>$5)`
	args := []any{tx.pluginID, tx.scope.Kind, tx.scope.ID, key, tx.now}
	if expected > 0 {
		query += ` AND revision=$6`
		args = append(args, expected)
	}
	result, err := tx.tx.Exec(tx.ctx, query, args...)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		if expected > 0 {
			return nativeplugin.ErrStateConflict
		}
		return nativeplugin.ErrStateNotFound
	}
	return nil
}

func (tx *postgresNativeStateTx) List(prefix string, limit int) ([]nativeplugin.StateValue, error) {
	rows, err := tx.tx.Query(tx.ctx, `SELECT state_key,value,revision,expires_at FROM native_plugin_state WHERE plugin_id=$1 AND scope_kind=$2 AND scope_id=$3 AND left(state_key,length($4))=$4 AND state_key NOT LIKE '__dokosoko/%' AND (expires_at IS NULL OR expires_at>$5) ORDER BY state_key LIMIT $6`, tx.pluginID, tx.scope.Kind, tx.scope.ID, prefix, tx.now, limit)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]nativeplugin.StateValue, 0)
	for rows.Next() {
		var value nativeplugin.StateValue
		if err := rows.Scan(&value.Key, &value.Value, &value.Revision, &value.ExpiresAt); err != nil {
			return nil, databaseError(err)
		}
		result = append(result, value)
	}
	return result, databaseError(rows.Err())
}

type postgresNativePluginTx struct {
	postgresNativeStateTx
	scopes []nativepluginstate.Scope
}

func (tx *postgresNativePluginTx) loadScopes() error {
	rows, err := tx.tx.Query(tx.ctx, `SELECT DISTINCT scope_kind,scope_id FROM native_plugin_state WHERE plugin_id=$1 AND state_key NOT LIKE '__dokosoko/%' AND (expires_at IS NULL OR expires_at>$2)`, tx.pluginID, tx.now)
	if err != nil {
		return databaseError(err)
	}
	defer rows.Close()
	seen := map[nativepluginstate.Scope]bool{{Kind: string(nativeplugin.StatePlugin)}: true}
	for rows.Next() {
		var scope nativepluginstate.Scope
		if err := rows.Scan(&scope.Kind, &scope.ID); err != nil {
			return databaseError(err)
		}
		seen[scope] = true
	}
	if err := rows.Err(); err != nil {
		return databaseError(err)
	}
	for scope := range seen {
		tx.scopes = append(tx.scopes, scope)
	}
	sort.Slice(tx.scopes, func(i, j int) bool {
		return tx.scopes[i].Kind+"\x00"+tx.scopes[i].ID < tx.scopes[j].Kind+"\x00"+tx.scopes[j].ID
	})
	return nil
}

func (tx *postgresNativePluginTx) Scopes() []nativepluginstate.Scope {
	return append([]nativepluginstate.Scope(nil), tx.scopes...)
}

func (tx *postgresNativePluginTx) ForScope(scope nativepluginstate.Scope) nativepluginstate.Transaction {
	return &postgresNativeStateTx{ctx: tx.ctx, tx: tx.tx, pluginID: tx.pluginID, scope: scope, now: tx.now}
}

var _ nativepluginstate.Backend = (*Postgres)(nil)
