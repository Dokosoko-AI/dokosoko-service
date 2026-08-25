package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const resourceSetSelect = `SELECT id::text,deployment_id::text,organisation_id::text,kind,name,description,state,revision,created_at,updated_at FROM resource_sets`

func scanResourceSet(row pgx.Row) (model.ResourceSet, error) {
	var value model.ResourceSet
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Kind, &value.Name, &value.Description, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const resourceSetRevisionSelect = `SELECT id::text,resource_set_id::text,revision,manifest,content_hash,created_by,created_at FROM resource_set_revisions`

func scanResourceSetRevision(row pgx.Row) (model.ResourceSetRevision, error) {
	var value model.ResourceSetRevision
	err := row.Scan(&value.ID, &value.ResourceSetID, &value.Revision, &value.Manifest, &value.ContentHash, &value.CreatedBy, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichResourceSet(ctx context.Context, value model.ResourceSet) (model.ResourceSet, error) {
	latest, err := scanResourceSetRevision(p.pool.QueryRow(ctx, resourceSetRevisionSelect+` WHERE resource_set_id=$1 ORDER BY revision DESC LIMIT 1`, value.ID))
	if err == nil {
		value.Latest = &latest
	} else if err != ErrNotFound {
		return model.ResourceSet{}, err
	}
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_resource_bindings WHERE resource_set_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.ResourceSet{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.ResourceSet{}, databaseError(err)
		}
		value.UsedBy = append(value.UsedBy, id)
	}
	return value, rows.Err()
}

func (p *Postgres) ResourceSets(ctx context.Context, deploymentID, kind string) ([]model.ResourceSet, error) {
	query, args := resourceSetSelect+` WHERE deployment_id=$1`, []any{deploymentID}
	if kind != "" {
		query += ` AND kind=$2`
		args = append(args, kind)
	}
	query += ` ORDER BY name`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.ResourceSet, 0)
	for rows.Next() {
		value, err := scanResourceSet(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.ResourceSet, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichResourceSet(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) ResourceSet(ctx context.Context, deploymentID, id string) (model.ResourceSet, error) {
	value, err := scanResourceSet(p.pool.QueryRow(ctx, resourceSetSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.ResourceSet{}, err
	}
	return p.enrichResourceSet(ctx, value)
}

func (p *Postgres) CreateResourceSet(ctx context.Context, value model.ResourceSet, revision model.ResourceSetRevision) (model.ResourceSet, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ResourceSet{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanResourceSet(tx.QueryRow(ctx, `INSERT INTO resource_sets(id,deployment_id,organisation_id,kind,name,description,state) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,deployment_id::text,organisation_id::text,kind,name,description,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.Kind, value.Name, value.Description, value.State))
	if err != nil {
		return model.ResourceSet{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO resource_set_revisions(id,resource_set_id,revision,manifest,content_hash,created_by) VALUES($1,$2,1,$3,$4,$5)`, revision.ID, value.ID, revision.Manifest, revision.ContentHash, revision.CreatedBy); err != nil {
		return model.ResourceSet{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.ResourceSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ResourceSet{}, err
	}
	return p.enrichResourceSet(ctx, created)
}

func (p *Postgres) UpdateResourceSet(ctx context.Context, value model.ResourceSet, revision model.ResourceSetRevision, expected int64) (model.ResourceSet, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.ResourceSet{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanResourceSet(tx.QueryRow(ctx, `UPDATE resource_sets SET name=$3,description=$4,state=$5,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$6 RETURNING id::text,deployment_id::text,organisation_id::text,kind,name,description,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.Description, value.State, expected))
	if err != nil {
		return model.ResourceSet{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO resource_set_revisions(id,resource_set_id,revision,manifest,content_hash,created_by) VALUES($1,$2,$3,$4,$5,$6)`, revision.ID, value.ID, updated.Revision, revision.Manifest, revision.ContentHash, revision.CreatedBy); err != nil {
		return model.ResourceSet{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.ResourceSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ResourceSet{}, err
	}
	return p.enrichResourceSet(ctx, updated)
}

func (p *Postgres) ResourceSetRevisions(ctx context.Context, resourceSetID string) ([]model.ResourceSetRevision, error) {
	rows, err := p.pool.Query(ctx, resourceSetRevisionSelect+` WHERE resource_set_id=$1 ORDER BY revision DESC`, resourceSetID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.ResourceSetRevision, 0)
	for rows.Next() {
		value, err := scanResourceSetRevision(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

const integrationResourceLinkSelect = `SELECT binding.id::text,binding.integration_id::text,binding.resource_set_id::text,set.kind,set.name,binding.follow_latest,coalesce(binding.pinned_revision_id::text,''),coalesce(revision.id::text,''),coalesce(revision.resource_set_id::text,''),coalesce(revision.revision,0),coalesce(revision.manifest,'[]'::jsonb),coalesce(revision.content_hash,''),coalesce(revision.created_by,''),coalesce(revision.created_at,to_timestamp(0)),binding.created_at,binding.updated_at FROM integration_resource_bindings binding JOIN resource_sets set ON set.id=binding.resource_set_id LEFT JOIN LATERAL (SELECT value.* FROM resource_set_revisions value WHERE value.resource_set_id=binding.resource_set_id AND (binding.follow_latest OR value.id=binding.pinned_revision_id) ORDER BY CASE WHEN value.id=binding.pinned_revision_id THEN 0 ELSE 1 END,value.revision DESC LIMIT 1) revision ON true`

func scanIntegrationResourceLink(row pgx.Row) (model.IntegrationResourceLink, error) {
	var value model.IntegrationResourceLink
	var revision model.ResourceSetRevision
	err := row.Scan(&value.ID, &value.IntegrationID, &value.ResourceSetID, &value.Kind, &value.Name, &value.FollowLatest, &value.PinnedRevisionID, &revision.ID, &revision.ResourceSetID, &revision.Revision, &revision.Manifest, &revision.ContentHash, &revision.CreatedBy, &revision.CreatedAt, &value.CreatedAt, &value.UpdatedAt)
	if err == nil && revision.ID != "" {
		value.ResolvedRevision = &revision
	}
	return value, databaseError(err)
}

func (p *Postgres) IntegrationResourceLinks(ctx context.Context, integrationID string) ([]model.IntegrationResourceLink, error) {
	rows, err := p.pool.Query(ctx, integrationResourceLinkSelect+` WHERE binding.integration_id=$1 ORDER BY set.kind,set.name`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.IntegrationResourceLink, 0)
	for rows.Next() {
		value, err := scanIntegrationResourceLink(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SaveIntegrationResourceLink(ctx context.Context, value model.IntegrationResourceLink) (model.IntegrationResourceLink, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.IntegrationResourceLink{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM integrations WHERE id=$1`, value.IntegrationID).Scan(&deploymentID); err != nil {
		return model.IntegrationResourceLink{}, databaseError(err)
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO integration_resource_bindings(id,integration_id,resource_set_id,follow_latest,pinned_revision_id,created_by) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6) ON CONFLICT(integration_id,resource_set_id) DO UPDATE SET follow_latest=excluded.follow_latest,pinned_revision_id=excluded.pinned_revision_id,updated_at=now() RETURNING id::text`, value.ID, value.IntegrationID, value.ResourceSetID, value.FollowLatest, value.PinnedRevisionID, "").Scan(&id)
	if err != nil {
		return model.IntegrationResourceLink{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.IntegrationResourceLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.IntegrationResourceLink{}, err
	}
	return scanIntegrationResourceLink(p.pool.QueryRow(ctx, integrationResourceLinkSelect+` WHERE binding.id=$1`, id))
}

func (p *Postgres) DeleteIntegrationResourceLink(ctx context.Context, integrationID, resourceSetID string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `DELETE FROM integration_resource_bindings WHERE integration_id=$1 AND resource_set_id=$2`, integrationID, resourceSetID)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM integrations WHERE id=$1`, integrationID).Scan(&deploymentID); err != nil {
		return databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
