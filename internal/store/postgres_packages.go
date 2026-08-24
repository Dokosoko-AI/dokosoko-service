package store

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const packageArtifactSelect = `SELECT id::text,deployment_id::text,organisation_id::text,name,description,ecosystem,coordinate,purl,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_package_artifact_id::text,''),deprecation_message,sunset_at,revision,created_at,updated_at FROM package_artifacts`

func scanPackageArtifact(row pgx.Row) (model.PackageArtifact, error) {
	var value model.PackageArtifact
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.Description, &value.Ecosystem, &value.Coordinate, &value.PURL, &value.RegistryURL, &value.SourceURL, &value.Language, &value.Platform, &value.Visibility, &value.Lifecycle, &value.ReplacementPackageArtifactID, &value.DeprecationMessage, &value.SunsetAt, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func packageArtifactWriteError(ctx context.Context, tx pgx.Tx, artifactID string, err error) error {
	if err != ErrNotFound {
		return err
	}
	var exists bool
	if queryErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM package_artifacts WHERE id=$1)`, artifactID).Scan(&exists); queryErr != nil {
		return databaseError(queryErr)
	}
	if exists {
		return ErrConflict
	}
	return ErrNotFound
}

const packageReleaseSelect = `SELECT id::text,package_artifact_id::text,artifact_name,ecosystem,coordinate,version,purl,registry_url,source_url,language,platform,install_command,digest,provenance_url,sbom_url,visibility,content_hash,published_by,published_at,created_at FROM package_releases`

func scanPackageRelease(row pgx.Row) (model.PackageRelease, error) {
	var value model.PackageRelease
	err := row.Scan(&value.ID, &value.PackageArtifactID, &value.ArtifactName, &value.Ecosystem, &value.Coordinate, &value.Version, &value.PURL, &value.RegistryURL, &value.SourceURL, &value.Language, &value.Platform, &value.InstallCommand, &value.Digest, &value.ProvenanceURL, &value.SBOMURL, &value.Visibility, &value.ContentHash, &value.PublishedBy, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichPackageArtifact(ctx context.Context, value model.PackageArtifact) (model.PackageArtifact, error) {
	latest, err := scanPackageRelease(p.pool.QueryRow(ctx, packageReleaseSelect+` WHERE package_artifact_id=$1 ORDER BY published_at DESC,id DESC LIMIT 1`, value.ID))
	if err == nil {
		value.LatestRelease = &latest
	} else if err != ErrNotFound {
		return model.PackageArtifact{}, err
	}
	rows, err := p.pool.Query(ctx, `SELECT integration_id::text FROM integration_package_bindings WHERE package_artifact_id=$1 ORDER BY integration_id`, value.ID)
	if err != nil {
		return model.PackageArtifact{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var integrationID string
		if err := rows.Scan(&integrationID); err != nil {
			return model.PackageArtifact{}, databaseError(err)
		}
		value.UsedBy = append(value.UsedBy, integrationID)
	}
	return value, rows.Err()
}

func (p *Postgres) PackageArtifacts(ctx context.Context, deploymentID string) ([]model.PackageArtifact, error) {
	rows, err := p.pool.Query(ctx, packageArtifactSelect+` WHERE deployment_id=$1 ORDER BY ecosystem,coordinate`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.PackageArtifact, 0)
	for rows.Next() {
		value, err := scanPackageArtifact(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	rows.Close()
	result := make([]model.PackageArtifact, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichPackageArtifact(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) PackageArtifact(ctx context.Context, deploymentID, id string) (model.PackageArtifact, error) {
	value, err := scanPackageArtifact(p.pool.QueryRow(ctx, packageArtifactSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.PackageArtifact{}, err
	}
	return p.enrichPackageArtifact(ctx, value)
}

func (p *Postgres) CreatePackageArtifact(ctx context.Context, value model.PackageArtifact) (model.PackageArtifact, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanPackageArtifact(tx.QueryRow(ctx, `INSERT INTO package_artifacts(id,deployment_id,organisation_id,name,description,ecosystem,coordinate,purl,registry_url,source_url,language,platform,visibility,lifecycle) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id::text,deployment_id::text,organisation_id::text,name,description,ecosystem,coordinate,purl,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_package_artifact_id::text,''),deprecation_message,sunset_at,revision,created_at,updated_at`, value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.Description, value.Ecosystem, value.Coordinate, value.PURL, value.RegistryURL, value.SourceURL, value.Language, value.Platform, value.Visibility, value.Lifecycle))
	if err != nil {
		return model.PackageArtifact{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.PackageArtifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PackageArtifact{}, err
	}
	return p.enrichPackageArtifact(ctx, created)
}

func (p *Postgres) UpdatePackageArtifact(ctx context.Context, value model.PackageArtifact, expected int64) (model.PackageArtifact, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.PackageArtifact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanPackageArtifact(tx.QueryRow(ctx, `UPDATE package_artifacts SET name=$3,description=$4,ecosystem=$5,coordinate=$6,purl=$7,registry_url=$8,source_url=$9,language=$10,platform=$11,visibility=$12,lifecycle=$13,replacement_package_artifact_id=nullif($14,'')::uuid,deprecation_message=$15,sunset_at=$16,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$17 RETURNING id::text,deployment_id::text,organisation_id::text,name,description,ecosystem,coordinate,purl,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_package_artifact_id::text,''),deprecation_message,sunset_at,revision,created_at,updated_at`, value.DeploymentID, value.ID, value.Name, value.Description, value.Ecosystem, value.Coordinate, value.PURL, value.RegistryURL, value.SourceURL, value.Language, value.Platform, value.Visibility, value.Lifecycle, value.ReplacementPackageArtifactID, value.DeprecationMessage, value.SunsetAt, expected))
	if err != nil {
		return model.PackageArtifact{}, packageArtifactWriteError(ctx, tx, value.ID, err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.PackageArtifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PackageArtifact{}, err
	}
	return p.enrichPackageArtifact(ctx, updated)
}

func (p *Postgres) PackageReleases(ctx context.Context, artifactID string) ([]model.PackageRelease, error) {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM package_artifacts WHERE id=$1)`, artifactID).Scan(&exists); err != nil {
		return nil, databaseError(err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := p.pool.Query(ctx, packageReleaseSelect+` WHERE package_artifact_id=$1 ORDER BY published_at DESC,id DESC`, artifactID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.PackageRelease, 0)
	for rows.Next() {
		value, err := scanPackageRelease(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) PackageRelease(ctx context.Context, deploymentID, id string) (model.PackageRelease, error) {
	return scanPackageRelease(p.pool.QueryRow(ctx, packageReleaseSelect+` WHERE id=$2 AND package_artifact_id IN (SELECT id FROM package_artifacts WHERE deployment_id=$1)`, deploymentID, id))
}

func (p *Postgres) CreatePackageRelease(ctx context.Context, deploymentID string, value model.PackageRelease, expectedArtifactRevision int64) (model.PackageArtifact, model.PackageRelease, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	artifact, err := scanPackageArtifact(tx.QueryRow(ctx, `UPDATE package_artifacts SET lifecycle=CASE WHEN lifecycle='draft' THEN 'active' ELSE lifecycle END,revision=revision+1,updated_at=now() WHERE deployment_id=$1 AND id=$2 AND revision=$3 AND lifecycle IN ('draft','active') AND (sunset_at IS NULL OR sunset_at > now()) RETURNING id::text,deployment_id::text,organisation_id::text,name,description,ecosystem,coordinate,purl,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_package_artifact_id::text,''),deprecation_message,sunset_at,revision,created_at,updated_at`, deploymentID, value.PackageArtifactID, expectedArtifactRevision))
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, packageArtifactWriteError(ctx, tx, value.PackageArtifactID, err)
	}
	created, err := scanPackageRelease(tx.QueryRow(ctx, `INSERT INTO package_releases(id,package_artifact_id,artifact_name,ecosystem,coordinate,version,purl,registry_url,source_url,language,platform,install_command,digest,provenance_url,sbom_url,visibility,content_hash,published_by,published_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id::text,package_artifact_id::text,artifact_name,ecosystem,coordinate,version,purl,registry_url,source_url,language,platform,install_command,digest,provenance_url,sbom_url,visibility,content_hash,published_by,published_at,created_at`, value.ID, value.PackageArtifactID, value.ArtifactName, value.Ecosystem, value.Coordinate, value.Version, value.PURL, value.RegistryURL, value.SourceURL, value.Language, value.Platform, value.InstallCommand, value.Digest, value.ProvenanceURL, value.SBOMURL, value.Visibility, value.ContentHash, value.PublishedBy, value.PublishedAt))
	if err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PackageArtifact{}, model.PackageRelease{}, err
	}
	enriched, err := p.enrichPackageArtifact(ctx, artifact)
	return enriched, created, err
}

const integrationPackageBindingSelect = `SELECT id::text,deployment_id::text,integration_id::text,package_artifact_id::text,package_release_id::text,created_by,created_at,updated_at FROM integration_package_bindings`

func scanIntegrationPackageBinding(row pgx.Row) (model.IntegrationPackageBinding, error) {
	var value model.IntegrationPackageBinding
	err := row.Scan(&value.ID, &value.DeploymentID, &value.IntegrationID, &value.PackageArtifactID, &value.PackageReleaseID, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichIntegrationPackageBinding(ctx context.Context, value model.IntegrationPackageBinding) (model.IntegrationPackageBinding, error) {
	var err error
	artifact, err := scanPackageArtifact(p.pool.QueryRow(ctx, packageArtifactSelect+` WHERE id=$1`, value.PackageArtifactID))
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	artifact, err = p.enrichPackageArtifact(ctx, artifact)
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	release, err := scanPackageRelease(p.pool.QueryRow(ctx, packageReleaseSelect+` WHERE id=$1 AND package_artifact_id=$2`, value.PackageReleaseID, value.PackageArtifactID))
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	value.Artifact, value.Release = &artifact, &release
	return value, nil
}

func (p *Postgres) IntegrationPackageBindings(ctx context.Context, integrationID string) ([]model.IntegrationPackageBinding, error) {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1)`, integrationID).Scan(&exists); err != nil {
		return nil, databaseError(err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := p.pool.Query(ctx, integrationPackageBindingSelect+` WHERE integration_id=$1 ORDER BY package_artifact_id`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.IntegrationPackageBinding, 0)
	for rows.Next() {
		value, err := scanIntegrationPackageBinding(rows)
		if err != nil {
			return nil, err
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseError(err)
	}
	rows.Close()
	result := make([]model.IntegrationPackageBinding, 0, len(base))
	for _, value := range base {
		enriched, err := p.enrichIntegrationPackageBinding(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) SaveIntegrationPackageBinding(ctx context.Context, value model.IntegrationPackageBinding) (model.IntegrationPackageBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM integrations WHERE id=$1`, value.IntegrationID).Scan(&deploymentID); err != nil {
		return model.IntegrationPackageBinding{}, databaseError(err)
	}
	created, err := scanIntegrationPackageBinding(tx.QueryRow(ctx, `INSERT INTO integration_package_bindings(id,deployment_id,integration_id,package_artifact_id,package_release_id,created_by) SELECT $1,$6,$2,$3,$4,$5 FROM package_releases release JOIN package_artifacts artifact ON artifact.id=release.package_artifact_id WHERE release.id=$4 AND artifact.id=$3 AND artifact.deployment_id=$6 AND artifact.lifecycle='active' AND (artifact.sunset_at IS NULL OR artifact.sunset_at > now()) ON CONFLICT (integration_id,package_artifact_id) DO UPDATE SET package_release_id=excluded.package_release_id,created_by=excluded.created_by,updated_at=now() RETURNING id::text,deployment_id::text,integration_id::text,package_artifact_id::text,package_release_id::text,created_by,created_at,updated_at`, value.ID, value.IntegrationID, value.PackageArtifactID, value.PackageReleaseID, value.CreatedBy, deploymentID))
	if err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.IntegrationPackageBinding{}, err
	}
	return p.enrichIntegrationPackageBinding(ctx, created)
}

func (p *Postgres) DeleteIntegrationPackageBinding(ctx context.Context, integrationID, artifactID string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var deploymentID string
	if err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM integrations WHERE id=$1`, integrationID).Scan(&deploymentID); err != nil {
		return databaseError(err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM integration_package_bindings WHERE integration_id=$1 AND package_artifact_id=$2`, integrationID, artifactID)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
