package store

import (
	"context"
	"errors"
	"net/url"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (p *Postgres) UpdateTool(ctx context.Context, value model.Tool, expected int64) (model.Tool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Tool{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if value.BackendKind == "http" {
		parsed, parseErr := url.Parse(value.BaseURL)
		if parseErr != nil {
			return model.Tool{}, parseErr
		}
		result, updateErr := tx.Exec(ctx, `UPDATE api_connections SET base_url=$3,allowed_hosts=$4,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2`, value.ProductID, value.APIConnectionID, value.BaseURL, []string{parsed.Hostname()})
		if updateErr != nil {
			return model.Tool{}, databaseError(updateErr)
		}
		if result.RowsAffected() != 1 {
			return model.Tool{}, ErrNotFound
		}
	}
	result, err := tx.Exec(ctx, `UPDATE tool_definitions SET description=$4,input_schema=$5,output_schema=$6,http_method=$7,authorization_policy=$8,timeout_ms=$9,revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND state='draft'`, value.ProductID, value.ID, expected, value.Description, value.InputSchema, value.OutputSchema, value.HTTPMethod, value.AuthorizationPolicy, value.TimeoutMS)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	if result.RowsAffected() != 1 {
		if _, lookupErr := p.Tool(ctx, value.ProductID, value.ID); lookupErr == nil {
			return model.Tool{}, ErrConflict
		}
		return model.Tool{}, ErrNotFound
	}
	updated, err := scanTool(tx.QueryRow(ctx, toolSelect+` WHERE t.product_id=$1 AND t.id=$2`, value.ProductID, value.ID))
	if err != nil {
		return model.Tool{}, err
	}
	return updated, tx.Commit(ctx)
}

func (p *Postgres) RetireTool(ctx context.Context, productID, id string, expected int64) (model.Tool, error) {
	result, err := p.pool.Exec(ctx, `UPDATE tool_definitions SET state='retired',revision=revision+1,updated_at=now() WHERE product_id=$1 AND id=$2 AND revision=$3 AND state<>'retired'`, productID, id, expected)
	if err != nil {
		return model.Tool{}, databaseError(err)
	}
	if result.RowsAffected() != 1 {
		current, lookupErr := p.Tool(ctx, productID, id)
		if lookupErr != nil {
			return model.Tool{}, lookupErr
		}
		if current.State == "retired" {
			return current, nil
		}
		return model.Tool{}, ErrConflict
	}
	value, err := p.Tool(ctx, productID, id)
	if errors.Is(err, ErrNotFound) {
		return model.Tool{}, ErrConflict
	}
	return value, err
}
