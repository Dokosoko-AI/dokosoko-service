package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const deploymentSelect = `SELECT id::text,organisation_id::text,name,slug,description,public_mcp_enabled,default_release_policy,require_promotion_approval,catalog_revision,revision,created_at,updated_at FROM deployments`

func scanDeployment(row pgx.Row) (model.Deployment, error) {
	var value model.Deployment
	err := row.Scan(&value.ID, &value.OrganisationID, &value.Name, &value.Slug, &value.Description, &value.PublicMCPEnabled, &value.DefaultReleasePolicy, &value.RequirePromotionApproval, &value.CatalogRevision, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) Deployment(ctx context.Context) (model.Deployment, error) {
	return scanDeployment(p.pool.QueryRow(ctx, deploymentSelect+` WHERE singleton`))
}

func (p *Postgres) CreateDeployment(ctx context.Context, value model.Deployment) (model.Deployment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Deployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanDeployment(tx.QueryRow(ctx, `INSERT INTO deployments(id,organisation_id,name,slug,description,public_mcp_enabled,default_release_policy,require_promotion_approval) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,organisation_id::text,name,slug,description,public_mcp_enabled,default_release_policy,require_promotion_approval,catalog_revision,revision,created_at,updated_at`, value.ID, value.OrganisationID, value.Name, value.Slug, value.Description, value.PublicMCPEnabled, value.DefaultReleasePolicy, value.RequirePromotionApproval))
	if err != nil {
		return model.Deployment{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO products(id,organisation_id,slug,name,description,public_mcp_enabled,default_version_policy,require_promotion_approval,catalog_revision,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, created.ID, created.OrganisationID, created.Slug, created.Name, created.Description, created.PublicMCPEnabled, created.DefaultReleasePolicy, created.RequirePromotionApproval, created.CatalogRevision, created.Revision, created.CreatedAt, created.UpdatedAt); err != nil {
		return model.Deployment{}, databaseError(err)
	}
	return created, tx.Commit(ctx)
}

func (p *Postgres) UpdateDeployment(ctx context.Context, value model.Deployment, expected int64) (model.Deployment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Deployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanDeployment(tx.QueryRow(ctx, `UPDATE deployments SET name=$2,slug=$3,description=$4,public_mcp_enabled=$5,default_release_policy=$6,require_promotion_approval=$7,revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$8 RETURNING id::text,organisation_id::text,name,slug,description,public_mcp_enabled,default_release_policy,require_promotion_approval,catalog_revision,revision,created_at,updated_at`, value.ID, value.Name, value.Slug, value.Description, value.PublicMCPEnabled, value.DefaultReleasePolicy, value.RequirePromotionApproval, expected))
	if err != nil {
		return model.Deployment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE products SET name=$2,slug=$3,description=$4,public_mcp_enabled=$5,default_version_policy=$6,require_promotion_approval=$7,revision=$8,updated_at=$9 WHERE id=$1`, updated.ID, updated.Name, updated.Slug, updated.Description, updated.PublicMCPEnabled, updated.DefaultReleasePolicy, updated.RequirePromotionApproval, updated.Revision, updated.UpdatedAt); err != nil {
		return model.Deployment{}, databaseError(err)
	}
	return updated, tx.Commit(ctx)
}

func bumpDeploymentCatalog(ctx context.Context, tx pgx.Tx, deploymentID string) error {
	if _, err := tx.Exec(ctx, `UPDATE deployments SET catalog_revision=catalog_revision+1,updated_at=now() WHERE id=$1`, deploymentID); err != nil {
		return databaseError(err)
	}
	_, err := tx.Exec(ctx, `UPDATE products SET catalog_revision=catalog_revision+1,updated_at=now() WHERE id=$1`, deploymentID)
	return databaseError(err)
}

const integrationSelect = `SELECT id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,visibility,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at FROM integrations`

func scanIntegration(row pgx.Row) (model.Integration, error) {
	var value model.Integration
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.FamilyKey, &value.VersionKey, &value.DisplayName, &value.Description, &value.Visibility, &value.Lifecycle, &value.ReplacementIntegrationID, &value.SunsetAt, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichIntegration(ctx context.Context, value model.Integration) (model.Integration, error) {
	links, err := p.IntegrationResourceLinks(ctx, value.ID)
	if err != nil && err != ErrNotFound {
		return model.Integration{}, err
	}
	value.Resources = links
	packages, err := p.IntegrationPackageBindings(ctx, value.ID)
	if err != nil && err != ErrNotFound {
		return model.Integration{}, err
	}
	value.Packages = packages
	rows, err := p.pool.Query(ctx, `SELECT access_connection_id::text FROM integration_access_bindings WHERE integration_id=$1 ORDER BY access_connection_id`, value.ID)
	if err != nil {
		return model.Integration{}, databaseError(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return model.Integration{}, databaseError(err)
		}
		value.AccessConnections = append(value.AccessConnections, id)
	}
	rows.Close()
	err = p.pool.QueryRow(ctx, `SELECT support_route_id::text FROM integration_support_bindings WHERE integration_id=$1 ORDER BY created_at DESC LIMIT 1`, value.ID).Scan(&value.SupportRouteID)
	if err != nil && err != pgx.ErrNoRows {
		return model.Integration{}, databaseError(err)
	}
	return value, nil
}

func (p *Postgres) Integrations(ctx context.Context, deploymentID string) ([]model.Integration, error) {
	rows, err := p.pool.Query(ctx, integrationSelect+` WHERE deployment_id=$1 ORDER BY display_name,version_key`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.Integration, 0)
	for rows.Next() {
		value, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.Integration, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichIntegration(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) Integration(ctx context.Context, deploymentID, id string) (model.Integration, error) {
	value, err := scanIntegration(p.pool.QueryRow(ctx, integrationSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.Integration{}, err
	}
	return p.enrichIntegration(ctx, value)
}

func (p *Postgres) CreateIntegration(ctx context.Context, value model.Integration) (model.Integration, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanIntegration(tx.QueryRow(ctx, `INSERT INTO integrations(id,deployment_id,organisation_id,family_key,version_key,display_name,description,visibility,lifecycle,replacement_integration_id,sunset_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,'')::uuid,$11) RETURNING id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,visibility,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.FamilyKey, value.VersionKey, value.DisplayName, value.Description, value.Visibility, value.Lifecycle, value.ReplacementIntegrationID, value.SunsetAt))
	if err != nil {
		return model.Integration{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.Integration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Integration{}, err
	}
	return p.enrichIntegration(ctx, created)
}

func (p *Postgres) UpdateIntegration(ctx context.Context, value model.Integration, expected int64) (model.Integration, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Integration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanIntegration(tx.QueryRow(ctx, `UPDATE integrations SET family_key=$3,version_key=$4,display_name=$5,description=$6,visibility=$7,lifecycle=$8,replacement_integration_id=nullif($9,'')::uuid,sunset_at=$10,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$11 RETURNING id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,visibility,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.FamilyKey, value.VersionKey, value.DisplayName, value.Description, value.Visibility, value.Lifecycle, value.ReplacementIntegrationID, value.SunsetAt, expected))
	if err != nil {
		return model.Integration{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.Integration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Integration{}, err
	}
	return p.enrichIntegration(ctx, updated)
}

const integrationRevisionSelect = `SELECT id::text,integration_id::text,revision,state,snapshot,manifest_hash,published_by,published_at,created_at FROM integration_revisions`

func scanIntegrationRevision(row pgx.Row) (model.IntegrationRevision, error) {
	var value model.IntegrationRevision
	err := row.Scan(&value.ID, &value.IntegrationID, &value.Revision, &value.State, &value.Snapshot, &value.ManifestHash, &value.PublishedBy, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) IntegrationRevisions(ctx context.Context, integrationID string) ([]model.IntegrationRevision, error) {
	rows, err := p.pool.Query(ctx, integrationRevisionSelect+` WHERE integration_id=$1 ORDER BY revision DESC`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.IntegrationRevision, 0)
	for rows.Next() {
		value, err := scanIntegrationRevision(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateIntegrationRevision(ctx context.Context, value model.IntegrationRevision) (model.IntegrationRevision, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanIntegrationRevision(tx.QueryRow(ctx, `INSERT INTO integration_revisions(id,integration_id,revision,state,snapshot,manifest_hash,published_by,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,integration_id::text,revision,state,snapshot,manifest_hash,published_by,published_at,created_at`, value.ID, value.IntegrationID, value.Revision, value.State, value.Snapshot, value.ManifestHash, value.PublishedBy, value.PublishedAt))
	if err != nil {
		return model.IntegrationRevision{}, err
	}
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM integrations WHERE id=$1`, value.IntegrationID).Scan(&deploymentID); err != nil {
		return model.IntegrationRevision{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.IntegrationRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.IntegrationRevision{}, databaseError(err)
	}
	return created, nil
}
