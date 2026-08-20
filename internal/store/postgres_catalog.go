package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

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

const integrationSelect = `SELECT id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at FROM integrations`

func scanIntegration(row pgx.Row) (model.Integration, error) {
	var value model.Integration
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.FamilyKey, &value.VersionKey, &value.DisplayName, &value.Description, &value.Lifecycle, &value.ReplacementIntegrationID, &value.SunsetAt, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichIntegration(ctx context.Context, value model.Integration) (model.Integration, error) {
	links, err := p.IntegrationResourceLinks(ctx, value.ID)
	if err != nil && err != ErrNotFound {
		return model.Integration{}, err
	}
	value.Resources = links
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
	created, err := scanIntegration(tx.QueryRow(ctx, `INSERT INTO integrations(id,deployment_id,organisation_id,family_key,version_key,display_name,description,lifecycle,replacement_integration_id,sunset_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid,$10) RETURNING id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.FamilyKey, value.VersionKey, value.DisplayName, value.Description, value.Lifecycle, value.ReplacementIntegrationID, value.SunsetAt))
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
	updated, err := scanIntegration(tx.QueryRow(ctx, `UPDATE integrations SET family_key=$3,version_key=$4,display_name=$5,description=$6,lifecycle=$7,replacement_integration_id=nullif($8,'')::uuid,sunset_at=$9,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$10 RETURNING id::text,deployment_id::text,organisation_id::text,family_key,version_key,display_name,description,lifecycle,coalesce(replacement_integration_id::text,''),sunset_at,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.FamilyKey, value.VersionKey, value.DisplayName, value.Description, value.Lifecycle, value.ReplacementIntegrationID, value.SunsetAt, expected))
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
	return scanIntegrationRevision(p.pool.QueryRow(ctx, `INSERT INTO integration_revisions(id,integration_id,revision,state,snapshot,manifest_hash,published_by,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,integration_id::text,revision,state,snapshot,manifest_hash,published_by,published_at,created_at`, value.ID, value.IntegrationID, value.Revision, value.State, value.Snapshot, value.ManifestHash, value.PublishedBy, value.PublishedAt))
}

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

const accessDefinitionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,coalesce(hook_set_id::text,''),operations,state,revision,created_at,updated_at FROM access_definitions`

func scanAccessDefinition(row pgx.Row) (model.AccessDefinition, error) {
	var value model.AccessDefinition
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.ServiceKey, &value.Name, &value.InstanceCardinality, &value.InstanceLabelSingular, &value.InstanceLabelPlural, &value.CredentialScope, &value.ManagementAuthType, &value.HookSetID, &value.Operations, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AccessDefinitions(ctx context.Context, deploymentID string) ([]model.AccessDefinition, error) {
	rows, err := p.pool.Query(ctx, accessDefinitionSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AccessDefinition, 0)
	for rows.Next() {
		value, err := scanAccessDefinition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AccessDefinition(ctx context.Context, deploymentID, id string) (model.AccessDefinition, error) {
	return scanAccessDefinition(p.pool.QueryRow(ctx, accessDefinitionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateAccessDefinition(ctx context.Context, value model.AccessDefinition) (model.AccessDefinition, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessDefinition(tx.QueryRow(ctx, `INSERT INTO access_definitions(id,deployment_id,organisation_id,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,hook_set_id,operations,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,nullif($11,'')::uuid,$12,$13) RETURNING id::text,deployment_id::text,organisation_id::text,service_key,name,instance_cardinality,instance_label_singular,instance_label_plural,credential_scope,management_auth_type,coalesce(hook_set_id::text,''),operations,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.ServiceKey, value.Name, value.InstanceCardinality, value.InstanceLabelSingular, value.InstanceLabelPlural, value.CredentialScope, value.ManagementAuthType, value.HookSetID, value.Operations, value.State))
	if err != nil {
		return model.AccessDefinition{}, err
	}
	snapshot, err := json.Marshal(map[string]any{"service_key": created.ServiceKey, "name": created.Name, "instance_cardinality": created.InstanceCardinality, "instance_label_singular": created.InstanceLabelSingular, "instance_label_plural": created.InstanceLabelPlural, "credential_scope": created.CredentialScope, "management_auth_type": created.ManagementAuthType, "hook_set_id": created.HookSetID, "operations": json.RawMessage(created.Operations), "state": created.State})
	if err != nil {
		return model.AccessDefinition{}, err
	}
	digest := sha256.Sum256(snapshot)
	if _, err := tx.Exec(ctx, `INSERT INTO access_definition_revisions(access_definition_id,revision,snapshot,content_hash) VALUES($1,$2,$3,$4)`, created.ID, created.Revision, snapshot, "sha256:"+hex.EncodeToString(digest[:])); err != nil {
		return model.AccessDefinition{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.AccessDefinition{}, err
	}
	return created, tx.Commit(ctx)
}

const accessConnectionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_definition_id::text,coalesce(environment_id::text,''),name,region,base_url,coalesce(management_secret_id::text,''),coalesce(legacy_provider_id::text,''),config,state,revision,created_at,updated_at FROM access_connections`

func scanAccessConnection(row pgx.Row) (model.AccessConnection, error) {
	var value model.AccessConnection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessDefinitionID, &value.EnvironmentID, &value.Name, &value.Region, &value.BaseURL, &value.ManagementSecretID, &value.LegacyProviderID, &value.Config, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichAccessConnection(ctx context.Context, value model.AccessConnection) (model.AccessConnection, error) {
	definition, err := p.AccessDefinition(ctx, value.DeploymentID, value.AccessDefinitionID)
	if err != nil {
		return model.AccessConnection{}, err
	}
	value.Definition = &definition
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_access_bindings WHERE access_connection_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.AccessConnection{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.AccessConnection{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) AccessConnections(ctx context.Context, deploymentID string) ([]model.AccessConnection, error) {
	rows, err := p.pool.Query(ctx, accessConnectionSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.AccessConnection, 0)
	for rows.Next() {
		value, err := scanAccessConnection(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.AccessConnection, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichAccessConnection(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) AccessConnection(ctx context.Context, deploymentID, id string) (model.AccessConnection, error) {
	value, err := scanAccessConnection(p.pool.QueryRow(ctx, accessConnectionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.AccessConnection{}, err
	}
	return p.enrichAccessConnection(ctx, value)
}

func (p *Postgres) CreateAccessConnection(ctx context.Context, value model.AccessConnection) (model.AccessConnection, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessConnection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessConnection(tx.QueryRow(ctx, `INSERT INTO access_connections(id,deployment_id,organisation_id,access_definition_id,environment_id,name,region,base_url,management_secret_id,config,state) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,nullif($9,'')::uuid,$10,$11) RETURNING id::text,deployment_id::text,organisation_id::text,access_definition_id::text,coalesce(environment_id::text,''),name,region,base_url,coalesce(management_secret_id::text,''),coalesce(legacy_provider_id::text,''),config,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessDefinitionID, value.EnvironmentID, value.Name, value.Region, value.BaseURL, value.ManagementSecretID, value.Config, value.State))
	if err != nil {
		return model.AccessConnection{}, err
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO integration_access_bindings(integration_id,access_connection_id) SELECT id,$2 FROM integrations WHERE id=$1 AND deployment_id=$3 ON CONFLICT DO NOTHING`, integrationID, value.ID, value.DeploymentID); err != nil {
			return model.AccessConnection{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AccessConnection{}, err
	}
	return p.enrichAccessConnection(ctx, created)
}

const accessInstanceSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_connection_id::text,environment_id::text,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at,created_at,updated_at FROM access_instances`

func scanAccessInstance(row pgx.Row) (model.AccessInstance, error) {
	var value model.AccessInstance
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessConnectionID, &value.EnvironmentID, &value.OwnerType, &value.OwnerID, &value.ExternalID, &value.DisplayName, &value.IdempotencyKey, &value.State, &value.ProviderMetadata, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichAccessInstance(ctx context.Context, value model.AccessInstance) (model.AccessInstance, error) {
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM access_instance_integration_bindings WHERE access_instance_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.AccessInstance{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.AccessInstance{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) AccessInstances(ctx context.Context, deploymentID, connectionID string) ([]model.AccessInstance, error) {
	query, args := accessInstanceSelect+` WHERE deployment_id=$1`, []any{deploymentID}
	if connectionID != "" {
		query += ` AND access_connection_id=$2`
		args = append(args, connectionID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.AccessInstance, 0)
	for rows.Next() {
		value, err := scanAccessInstance(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.AccessInstance, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichAccessInstance(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) AccessInstance(ctx context.Context, deploymentID, id string) (model.AccessInstance, error) {
	value, err := scanAccessInstance(p.pool.QueryRow(ctx, accessInstanceSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.AccessInstance{}, err
	}
	return p.enrichAccessInstance(ctx, value)
}

func (p *Postgres) CreateAccessInstance(ctx context.Context, value model.AccessInstance) (model.AccessInstance, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.AccessInstance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAccessInstance(tx.QueryRow(ctx, `INSERT INTO access_instances(id,deployment_id,organisation_id,access_connection_id,environment_id,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(access_connection_id,idempotency_key) DO UPDATE SET updated_at=access_instances.updated_at RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,environment_id::text,owner_type,owner_id,external_id,display_name,idempotency_key,state,provider_metadata,expires_at,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessConnectionID, value.EnvironmentID, value.OwnerType, value.OwnerID, value.ExternalID, value.DisplayName, value.IdempotencyKey, value.State, value.ProviderMetadata, value.ExpiresAt))
	if err != nil {
		return model.AccessInstance{}, err
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO access_instance_integration_bindings(access_instance_id,integration_id) SELECT $1,integration_id FROM integration_access_bindings WHERE access_connection_id=$2 AND integration_id=$3 ON CONFLICT DO NOTHING`, created.ID, value.AccessConnectionID, integrationID); err != nil {
			return model.AccessInstance{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AccessInstance{}, err
	}
	return p.enrichAccessInstance(ctx, created)
}

const accessCredentialSelect = `SELECT id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at FROM access_credentials`

func scanAccessCredential(row pgx.Row) (model.AccessCredential, error) {
	var value model.AccessCredential
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AccessConnectionID, &value.AccessInstanceID, &value.EnvironmentID, &value.SubjectID, &value.ExternalID, &value.IdempotencyKey, &value.Scopes, &value.SecretFingerprint, &value.StorageMode, &value.EncryptedSecretID, &value.State, &value.ExpiresAt, &value.RotatedFromID, &value.RevokedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) AccessCredentials(ctx context.Context, deploymentID, connectionID, instanceID string) ([]model.AccessCredential, error) {
	query, args, index := accessCredentialSelect+` WHERE deployment_id=$1`, []any{deploymentID}, 2
	if connectionID != "" {
		query += ` AND access_connection_id=$` + strconv.Itoa(index)
		args, index = append(args, connectionID), index+1
	}
	if instanceID != "" {
		query += ` AND access_instance_id=$` + strconv.Itoa(index)
		args = append(args, instanceID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.AccessCredential, 0)
	for rows.Next() {
		value, err := scanAccessCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AccessCredential(ctx context.Context, deploymentID, id string) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, accessCredentialSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateAccessCredential(ctx context.Context, value model.AccessCredential) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, `INSERT INTO access_credentials(id,deployment_id,organisation_id,access_connection_id,access_instance_id,environment_id,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,encrypted_secret_id,state,expires_at,rotated_from_id) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,nullif($13,'')::uuid,$14,$15,nullif($16,'')::uuid) ON CONFLICT(access_connection_id,idempotency_key) WHERE idempotency_key<>'' DO UPDATE SET idempotency_key=access_credentials.idempotency_key RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at`, value.ID, value.DeploymentID, value.OrganisationID, value.AccessConnectionID, value.AccessInstanceID, value.EnvironmentID, value.SubjectID, value.ExternalID, value.IdempotencyKey, value.Scopes, value.SecretFingerprint, value.StorageMode, value.EncryptedSecretID, value.State, value.ExpiresAt, value.RotatedFromID))
}

func (p *Postgres) RevokeAccessCredential(ctx context.Context, deploymentID, id string, revokedAt time.Time) (model.AccessCredential, error) {
	return scanAccessCredential(p.pool.QueryRow(ctx, `UPDATE access_credentials SET state='revoked',revoked_at=$3 WHERE deployment_id=$1 AND id=$2 AND revoked_at IS NULL RETURNING id::text,deployment_id::text,organisation_id::text,access_connection_id::text,coalesce(access_instance_id::text,''),environment_id::text,subject_id,external_id,idempotency_key,scopes,secret_fingerprint,storage_mode,coalesce(encrypted_secret_id::text,''),state,expires_at,coalesce(rotated_from_id::text,''),revoked_at,created_at`, deploymentID, id, revokedAt))
}

const supportRouteSelect = `SELECT id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,bug_hook_url,coalesce(bug_hook_credential_id::text,''),feedback_hook_url,coalesce(feedback_hook_credential_id::text,''),retention_days,state,revision,created_at,updated_at FROM support_routes`

func scanSupportRoute(row pgx.Row) (model.SupportRoute, error) {
	var value model.SupportRoute
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.IsDefault, &value.BugReportsEnabled, &value.FeedbackEnabled, &value.BugHookURL, &value.BugHookCredentialID, &value.FeedbackHookURL, &value.FeedbackHookCredentialID, &value.RetentionDays, &value.State, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichSupportRoute(ctx context.Context, value model.SupportRoute) (model.SupportRoute, error) {
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_support_bindings WHERE support_route_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.SupportRoute{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
		value.IntegrationIDs = append(value.IntegrationIDs, id)
	}
	return value, rows.Err()
}

func (p *Postgres) SupportRoutes(ctx context.Context, deploymentID string) ([]model.SupportRoute, error) {
	rows, err := p.pool.Query(ctx, supportRouteSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.SupportRoute, 0)
	for rows.Next() {
		value, err := scanSupportRoute(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.SupportRoute, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichSupportRoute(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) SupportRoute(ctx context.Context, deploymentID, id string) (model.SupportRoute, error) {
	value, err := scanSupportRoute(p.pool.QueryRow(ctx, supportRouteSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, value)
}

func (p *Postgres) SupportRouteForIntegration(ctx context.Context, deploymentID, integrationID string) (model.SupportRoute, error) {
	value, err := scanSupportRoute(p.pool.QueryRow(ctx, supportRouteSelect+` route WHERE route.deployment_id=$1 AND route.state='active' AND (EXISTS(SELECT 1 FROM integration_support_bindings binding WHERE binding.support_route_id=route.id AND binding.integration_id=nullif($2,'')::uuid) OR route.is_default) ORDER BY EXISTS(SELECT 1 FROM integration_support_bindings binding WHERE binding.support_route_id=route.id AND binding.integration_id=nullif($2,'')::uuid) DESC LIMIT 1`, deploymentID, integrationID))
	if err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, value)
}

func (p *Postgres) SaveSupportRoute(ctx context.Context, value model.SupportRoute, expected int64) (model.SupportRoute, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SupportRoute{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if value.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE support_routes SET is_default=false,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id<>$2 AND is_default`, value.DeploymentID, value.ID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
	}
	var saved model.SupportRoute
	if expected == 0 {
		saved, err = scanSupportRoute(tx.QueryRow(ctx, `INSERT INTO support_routes(id,deployment_id,organisation_id,name,is_default,bug_reports_enabled,feedback_enabled,bug_hook_url,bug_hook_credential_id,feedback_hook_url,feedback_hook_credential_id,retention_days,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid,$10,nullif($11,'')::uuid,$12,$13) RETURNING id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,bug_hook_url,coalesce(bug_hook_credential_id::text,''),feedback_hook_url,coalesce(feedback_hook_credential_id::text,''),retention_days,state,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.IsDefault, value.BugReportsEnabled, value.FeedbackEnabled, value.BugHookURL, value.BugHookCredentialID, value.FeedbackHookURL, value.FeedbackHookCredentialID, value.RetentionDays, value.State))
	} else {
		saved, err = scanSupportRoute(tx.QueryRow(ctx, `UPDATE support_routes SET name=$3,is_default=$4,bug_reports_enabled=$5,feedback_enabled=$6,bug_hook_url=$7,bug_hook_credential_id=nullif($8,'')::uuid,feedback_hook_url=$9,feedback_hook_credential_id=nullif($10,'')::uuid,retention_days=$11,state=$12,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$13 RETURNING id::text,deployment_id::text,organisation_id::text,name,is_default,bug_reports_enabled,feedback_enabled,bug_hook_url,coalesce(bug_hook_credential_id::text,''),feedback_hook_url,coalesce(feedback_hook_credential_id::text,''),retention_days,state,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.IsDefault, value.BugReportsEnabled, value.FeedbackEnabled, value.BugHookURL, value.BugHookCredentialID, value.FeedbackHookURL, value.FeedbackHookCredentialID, value.RetentionDays, value.State, expected))
	}
	if err != nil {
		return model.SupportRoute{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM integration_support_bindings WHERE support_route_id=$1`, value.ID); err != nil {
		return model.SupportRoute{}, databaseError(err)
	}
	for _, integrationID := range value.IntegrationIDs {
		if _, err := tx.Exec(ctx, `DELETE FROM integration_support_bindings WHERE integration_id=$1 AND support_route_id<>$2`, integrationID, value.ID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO integration_support_bindings(integration_id,support_route_id) SELECT id,$2 FROM integrations WHERE id=$1 AND deployment_id=$3 ON CONFLICT DO NOTHING`, integrationID, value.ID, value.DeploymentID); err != nil {
			return model.SupportRoute{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SupportRoute{}, err
	}
	return p.enrichSupportRoute(ctx, saved)
}
