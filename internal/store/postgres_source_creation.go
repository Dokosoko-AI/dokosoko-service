package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) CreateSourceOnce(ctx context.Context, input SourceCreation) (model.Source, error) {
	if err := validateSourceCreation(input); err != nil {
		return model.Source{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Source{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value := input.Source
	// Serialize this short creation transaction with other source creations in
	// the deployment. Network fetching, parsing and AI never run under this lock.
	var productID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM products WHERE id=$1 AND organisation_id=$2 FOR UPDATE`, value.ProductID, value.OrganisationID).Scan(&productID); err != nil {
		return model.Source{}, databaseError(err)
	}
	var previous sourceCreationResult
	err = tx.QueryRow(ctx, `SELECT source_id::text, input_digest FROM source_creation_requests WHERE product_id=$1 AND request_digest=$2`, value.ProductID, input.RequestDigest).Scan(&previous.SourceID, &previous.InputDigest)
	if err == nil {
		if previous.InputDigest != input.InputDigest {
			return model.Source{}, ErrSourceCreationConflict
		}
		return scanSource(tx.QueryRow(ctx, `SELECT id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at FROM sources WHERE product_id=$1 AND id=$2`, value.ProductID, previous.SourceID))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Source{}, databaseError(err)
	}
	value, err = scanSource(tx.QueryRow(ctx, `INSERT INTO sources(id, organisation_id, product_id, name, kind, location, visibility, state) VALUES ($1,$2,$3,$4,$5,$6,'private','draft') RETURNING id::text, organisation_id::text, product_id::text, name, kind, location, visibility::text, state::text, revision, created_at, updated_at`, value.ID, value.OrganisationID, value.ProductID, value.Name, value.Kind, value.Location))
	if err != nil {
		return model.Source{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_creation_requests(product_id,request_digest,input_digest,source_id) VALUES ($1,$2,$3,$4)`, value.ProductID, input.RequestDigest, input.InputDigest, value.ID); err != nil {
		return model.Source{}, databaseError(err)
	}
	current, err := json.Marshal(input.Audit.Current)
	if err != nil {
		return model.Source{}, err
	}
	event := input.Audit
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(event_key, organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at) VALUES ($1,$2,$3,$4,'root',$5,$6,$7,'null',$8,$9,'success',$10)`, event.ID, value.OrganisationID, value.ProductID, event.ActorID, event.Action, event.TargetType, value.ID, current, event.RequestID, event.CreatedAt); err != nil {
		return model.Source{}, databaseError(err)
	}
	// A commit error can be ambiguous. Return the allocated identity so upload
	// handling retains a file that a committed source may already reference.
	return value, tx.Commit(ctx)
}
