package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const aiPromptStateColumns = `product_id::text,prompt_key,coalesce(instructions,''),revision,updated_at`

func scanAIPromptState(row interface{ Scan(...any) error }) (model.AIPromptState, error) {
	var value model.AIPromptState
	err := row.Scan(&value.ProductID, &value.Key, &value.Instructions, &value.Revision, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AIPromptStates(ctx context.Context, productID string) ([]model.AIPromptState, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+aiPromptStateColumns+` FROM ai_prompt_settings WHERE product_id=$1 ORDER BY prompt_key`, productID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AIPromptState, 0)
	for rows.Next() {
		value, scanErr := scanAIPromptState(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, databaseError(rows.Err())
}

func (p *Postgres) AIPromptState(ctx context.Context, productID, key string) (model.AIPromptState, error) {
	return scanAIPromptState(p.pool.QueryRow(ctx, `SELECT `+aiPromptStateColumns+` FROM ai_prompt_settings WHERE product_id=$1 AND prompt_key=$2`, productID, key))
}

type aiPromptQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func saveAIPromptState(ctx context.Context, queryer aiPromptQueryer, value model.AIPromptState, expectedRevision int64) (model.AIPromptState, error) {
	var (
		updated model.AIPromptState
		err     error
	)
	if expectedRevision == 1 {
		updated, err = scanAIPromptState(queryer.QueryRow(ctx, `INSERT INTO ai_prompt_settings(product_id,prompt_key,instructions,revision) VALUES ($1,$2,nullif($3,''),2)
			ON CONFLICT (product_id,prompt_key) DO NOTHING
			RETURNING `+aiPromptStateColumns, value.ProductID, value.Key, value.Instructions))
	} else {
		updated, err = scanAIPromptState(queryer.QueryRow(ctx, `UPDATE ai_prompt_settings
			SET instructions=nullif($3,''),revision=revision+1,updated_at=now()
			WHERE product_id=$1 AND prompt_key=$2 AND revision=$4
			RETURNING `+aiPromptStateColumns, value.ProductID, value.Key, value.Instructions, expectedRevision))
	}
	if !errors.Is(err, ErrNotFound) {
		return updated, err
	}
	var promptExists, productExists bool
	lookupErr := queryer.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM ai_prompt_settings WHERE product_id=$1 AND prompt_key=$2),
		EXISTS(SELECT 1 FROM products WHERE id=$1)`, value.ProductID, value.Key).Scan(&promptExists, &productExists)
	if lookupErr != nil {
		return model.AIPromptState{}, databaseError(lookupErr)
	}
	if promptExists || productExists {
		// A product with no row is the virtual revision-1 default. Any
		// other expected revision is stale.
		return model.AIPromptState{}, ErrConflict
	}
	return model.AIPromptState{}, ErrNotFound
}

func (p *Postgres) SaveAIPromptState(ctx context.Context, value model.AIPromptState, expectedRevision int64) (model.AIPromptState, error) {
	return saveAIPromptState(ctx, p.pool, value, expectedRevision)
}

func (p *Postgres) SaveAIPromptStateAndAudit(ctx context.Context, value model.AIPromptState, expectedRevision int64, event model.AuditEvent) (model.AIPromptState, error) {
	if strings.TrimSpace(event.ID) == "" {
		return model.AIPromptState{}, errors.New("audit event ID is required")
	}
	prior, err := json.Marshal(event.Prior)
	if err != nil {
		return model.AIPromptState{}, fmt.Errorf("marshal audit prior state: %w", err)
	}
	current, err := json.Marshal(event.Current)
	if err != nil {
		return model.AIPromptState{}, fmt.Errorf("marshal audit current state: %w", err)
	}
	outcome := event.Outcome
	if outcome == "" {
		outcome = "success"
	}

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.AIPromptState{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := saveAIPromptState(ctx, tx, value, expectedRevision)
	if err != nil {
		return model.AIPromptState{}, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO audit_events(event_key, organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES ($1, nullif($2, '')::uuid, nullif($3, '')::uuid, $4, 'root', $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT (event_key) DO NOTHING`, event.ID, event.OrganisationID, event.ProductID, event.ActorID, event.Action, event.TargetType, event.TargetID, prior, current, event.RequestID, outcome, event.CreatedAt)
	if err != nil {
		return model.AIPromptState{}, databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return model.AIPromptState{}, ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AIPromptState{}, databaseError(err)
	}
	return updated, nil
}
