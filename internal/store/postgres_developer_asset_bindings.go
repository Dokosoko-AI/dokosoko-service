package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const apiDocumentationBindingSelect = `SELECT id::text,deployment_id::text,integration_id::text,documentation_collection_id::text,follow_latest,coalesce(pinned_revision_id::text,''),selector,selector_hash,visibility,lifecycle,revision FROM api_documentation_bindings`

func scanAPIDocumentationBinding(row pgx.Row) (model.APIDocumentationBinding, error) {
	var value model.APIDocumentationBinding
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIID, &value.DocumentationCollectionID, &value.FollowLatest,
		&value.PinnedRevisionID, &value.Selector, &value.SelectorHash, &value.Visibility, &value.Lifecycle, &value.Revision)
	return value, databaseError(err)
}

func (p *Postgres) DeveloperAssetUsage(ctx context.Context, deploymentID string) (DeveloperAssetUsageRecord, error) {
	result := DeveloperAssetUsageRecord{
		Documentation: make([]model.APIDocumentationBinding, 0),
		Contracts:     make([]model.APIContractBinding, 0),
		SDKs:          make([]model.APISDKBinding, 0),
		Publications:  make([]model.APIDeveloperAssetPublication, 0),
	}

	rows, err := p.pool.Query(ctx, apiDocumentationBindingSelect+` WHERE deployment_id=$1 ORDER BY integration_id,created_at,id`, deploymentID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanAPIDocumentationBinding(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Documentation = append(result.Documentation, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, databaseError(err)
	}
	rows.Close()

	rows, err = p.pool.Query(ctx, apiContractBindingSelect+` WHERE deployment_id=$1 ORDER BY integration_id,created_at,id`, deploymentID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanAPIContractBinding(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Contracts = append(result.Contracts, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, databaseError(err)
	}
	rows.Close()

	rows, err = p.pool.Query(ctx, apiSDKBindingSelect+` WHERE deployment_id=$1 ORDER BY integration_id,created_at,id`, deploymentID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanAPISDKBinding(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.SDKs = append(result.SDKs, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, databaseError(err)
	}
	rows.Close()

	rows, err = p.pool.Query(ctx, apiDeveloperAssetPublicationSelect+` WHERE deployment_id=$1 ORDER BY published_at DESC`, deploymentID)
	if err != nil {
		return result, databaseError(err)
	}
	basePublications := make([]model.APIDeveloperAssetPublication, 0)
	for rows.Next() {
		value, scanErr := scanAPIDeveloperAssetPublication(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		basePublications = append(basePublications, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, databaseError(err)
	}
	rows.Close()
	for _, value := range basePublications {
		enriched, enrichErr := p.enrichAPIDeveloperAssetPublication(ctx, value)
		if enrichErr != nil {
			return result, enrichErr
		}
		result.Publications = append(result.Publications, enriched)
	}
	return result, nil
}

func (p *Postgres) APIDocumentationBindings(ctx context.Context, deploymentID, apiID string) ([]model.APIDocumentationBinding, error) {
	rows, err := p.pool.Query(ctx, apiDocumentationBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 ORDER BY created_at,id`, deploymentID, apiID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIDocumentationBinding, 0)
	for rows.Next() {
		value, scanErr := scanAPIDocumentationBinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIDocumentationBinding(ctx context.Context, deploymentID, apiID, id string) (model.APIDocumentationBinding, error) {
	return scanAPIDocumentationBinding(p.pool.QueryRow(ctx, apiDocumentationBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id))
}

func (p *Postgres) SaveAPIDocumentationBinding(ctx context.Context, value model.APIDocumentationBinding, expected int64) (model.APIDocumentationBinding, error) {
	if len(value.Selector) == 0 {
		value.Selector = json.RawMessage(`{}`)
	}
	if value.Lifecycle == "" {
		value.Lifecycle = "attached"
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var saved model.APIDocumentationBinding
	if expected == 0 {
		saved, err = scanAPIDocumentationBinding(tx.QueryRow(ctx, `INSERT INTO api_documentation_bindings(id,deployment_id,integration_id,documentation_collection_id,follow_latest,pinned_revision_id,selector,visibility,lifecycle)
			VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9) RETURNING id::text,deployment_id::text,integration_id::text,documentation_collection_id::text,follow_latest,coalesce(pinned_revision_id::text,''),selector,selector_hash,visibility,lifecycle,revision`,
			value.ID, value.DeploymentID, value.APIID, value.DocumentationCollectionID, value.FollowLatest, value.PinnedRevisionID,
			value.Selector, value.Visibility, value.Lifecycle))
	} else {
		saved, err = scanAPIDocumentationBinding(tx.QueryRow(ctx, `UPDATE api_documentation_bindings SET follow_latest=$4,pinned_revision_id=nullif($5,'')::uuid,selector=$6,visibility=$7,lifecycle=$8,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$9 RETURNING id::text,deployment_id::text,integration_id::text,documentation_collection_id::text,follow_latest,coalesce(pinned_revision_id::text,''),selector,selector_hash,visibility,lifecycle,revision`,
			value.DeploymentID, value.APIID, value.ID, value.FollowLatest, value.PinnedRevisionID, value.Selector, value.Visibility, value.Lifecycle, expected))
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_documentation_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, value.DeploymentID, value.APIID, value.ID).Scan(&exists); lookupErr == nil {
				return model.APIDocumentationBinding{}, ErrConflict
			}
		}
	}
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.APIDocumentationBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIDocumentationBinding{}, err
	}
	return saved, nil
}

func (p *Postgres) DetachAPIDocumentationBinding(ctx context.Context, deploymentID, apiID, id string, expected int64) (model.APIDocumentationBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := scanAPIDocumentationBinding(tx.QueryRow(ctx, `UPDATE api_documentation_bindings SET lifecycle='detached',revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$4 RETURNING id::text,deployment_id::text,integration_id::text,documentation_collection_id::text,follow_latest,coalesce(pinned_revision_id::text,''),selector,selector_hash,visibility,lifecycle,revision`, deploymentID, apiID, id, expected))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_documentation_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id).Scan(&exists); lookupErr == nil {
			return model.APIDocumentationBinding{}, ErrConflict
		}
	}
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.APIDocumentationBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIDocumentationBinding{}, err
	}
	return value, nil
}

const apiContractBindingSelect = `SELECT id::text,deployment_id::text,integration_id::text,api_contract_id::text,follow_latest,coalesce(pinned_revision_id::text,''),primary_contract,visibility,lifecycle,revision FROM api_contract_bindings`

func scanAPIContractBinding(row pgx.Row) (model.APIContractBinding, error) {
	var value model.APIContractBinding
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIID, &value.APIContractID, &value.FollowLatest,
		&value.PinnedRevisionID, &value.Primary, &value.Visibility, &value.Lifecycle, &value.Revision)
	return value, databaseError(err)
}

func (p *Postgres) APIContractBindings(ctx context.Context, deploymentID, apiID string) ([]model.APIContractBinding, error) {
	rows, err := p.pool.Query(ctx, apiContractBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 ORDER BY created_at,id`, deploymentID, apiID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIContractBinding, 0)
	for rows.Next() {
		value, scanErr := scanAPIContractBinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIContractBinding(ctx context.Context, deploymentID, apiID, id string) (model.APIContractBinding, error) {
	return scanAPIContractBinding(p.pool.QueryRow(ctx, apiContractBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id))
}

func (p *Postgres) SaveAPIContractBinding(ctx context.Context, value model.APIContractBinding, expected int64) (model.APIContractBinding, error) {
	if value.Lifecycle == "" {
		value.Lifecycle = "attached"
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var saved model.APIContractBinding
	if expected == 0 {
		saved, err = scanAPIContractBinding(tx.QueryRow(ctx, `INSERT INTO api_contract_bindings(id,deployment_id,integration_id,api_contract_id,follow_latest,pinned_revision_id,primary_contract,visibility,lifecycle)
			VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,$7,$8,$9) RETURNING id::text,deployment_id::text,integration_id::text,api_contract_id::text,follow_latest,coalesce(pinned_revision_id::text,''),primary_contract,visibility,lifecycle,revision`,
			value.ID, value.DeploymentID, value.APIID, value.APIContractID, value.FollowLatest, value.PinnedRevisionID,
			value.Primary, value.Visibility, value.Lifecycle))
	} else {
		saved, err = scanAPIContractBinding(tx.QueryRow(ctx, `UPDATE api_contract_bindings SET follow_latest=$4,pinned_revision_id=nullif($5,'')::uuid,primary_contract=$6,visibility=$7,lifecycle=$8,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$9 RETURNING id::text,deployment_id::text,integration_id::text,api_contract_id::text,follow_latest,coalesce(pinned_revision_id::text,''),primary_contract,visibility,lifecycle,revision`,
			value.DeploymentID, value.APIID, value.ID, value.FollowLatest, value.PinnedRevisionID, value.Primary, value.Visibility, value.Lifecycle, expected))
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_contract_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, value.DeploymentID, value.APIID, value.ID).Scan(&exists); lookupErr == nil {
				return model.APIContractBinding{}, ErrConflict
			}
		}
	}
	if err != nil {
		return model.APIContractBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.APIContractBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIContractBinding{}, err
	}
	return saved, nil
}

func (p *Postgres) DetachAPIContractBinding(ctx context.Context, deploymentID, apiID, id string, expected int64) (model.APIContractBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := scanAPIContractBinding(tx.QueryRow(ctx, `UPDATE api_contract_bindings SET lifecycle='detached',primary_contract=false,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$4 RETURNING id::text,deployment_id::text,integration_id::text,api_contract_id::text,follow_latest,coalesce(pinned_revision_id::text,''),primary_contract,visibility,lifecycle,revision`, deploymentID, apiID, id, expected))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_contract_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id).Scan(&exists); lookupErr == nil {
			return model.APIContractBinding{}, ErrConflict
		}
	}
	if err != nil {
		return model.APIContractBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.APIContractBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIContractBinding{}, err
	}
	return value, nil
}

const apiSDKBindingSelect = `SELECT id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at FROM api_sdk_bindings`

func scanAPISDKBinding(row pgx.Row) (model.APISDKBinding, error) {
	var value model.APISDKBinding
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIID, &value.SDKPackageID, &value.SDKReleaseID,
		&value.SDKContentPublicationID, &value.APIContractRevisionID, &value.CompatibilityAssertionID, &value.State,
		&value.Coverage, &value.Assurance, &value.ApplicableModules, &value.ApplicableCapabilities, &value.ApplicableOperationKeys,
		&value.Selector, &value.SelectorHash, &value.Visibility, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) APISDKBindings(ctx context.Context, deploymentID, apiID string) ([]model.APISDKBinding, error) {
	rows, err := p.pool.Query(ctx, apiSDKBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 ORDER BY created_at,id`, deploymentID, apiID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APISDKBinding, 0)
	for rows.Next() {
		value, scanErr := scanAPISDKBinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APISDKBinding(ctx context.Context, deploymentID, apiID, id string) (model.APISDKBinding, error) {
	return scanAPISDKBinding(p.pool.QueryRow(ctx, apiSDKBindingSelect+` WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id))
}

func syncLegacySDKReferenceTx(ctx context.Context, tx pgx.Tx, bindingID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO sdk_references(id,deployment_id,organisation_id,integration_id,ecosystem,coordinate,exact_version,install_command,documentation_url,source_url,checksum,visibility,revision,created_at,updated_at)
		SELECT binding.id,binding.deployment_id,package.organisation_id,binding.integration_id,package.ecosystem,package.display_coordinate,release.exact_version,release.install_command,release.documentation_url,release.source_url,release.upstream_digest,binding.visibility,binding.revision,binding.created_at,binding.updated_at
		FROM api_sdk_bindings binding JOIN sdk_packages package ON package.id=binding.sdk_package_id JOIN sdk_releases release ON release.id=binding.sdk_release_id
		WHERE binding.id=$1 AND binding.binding_state<>'detached'
		ON CONFLICT(id) DO UPDATE SET ecosystem=excluded.ecosystem,coordinate=excluded.coordinate,exact_version=excluded.exact_version,install_command=excluded.install_command,documentation_url=excluded.documentation_url,source_url=excluded.source_url,checksum=excluded.checksum,visibility=excluded.visibility,revision=excluded.revision,updated_at=excluded.updated_at`, bindingID)
	return databaseError(err)
}

func (p *Postgres) SaveAPISDKBinding(ctx context.Context, value model.APISDKBinding, expected int64) (model.APISDKBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var saved model.APISDKBinding
	if expected == 0 {
		saved, err = scanAPISDKBinding(tx.QueryRow(ctx, `INSERT INTO api_sdk_bindings(id,deployment_id,integration_id,sdk_package_id,sdk_release_id,sdk_content_publication_id,api_contract_revision_id,compatibility_assertion_id,binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility)
			VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,nullif($7,'')::uuid,nullif($8,'')::uuid,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			RETURNING id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at`,
			value.ID, value.DeploymentID, value.APIID, value.SDKPackageID, value.SDKReleaseID, value.SDKContentPublicationID,
			value.APIContractRevisionID, value.CompatibilityAssertionID, value.State, value.Coverage, value.Assurance,
			value.ApplicableModules, value.ApplicableCapabilities, value.ApplicableOperationKeys, value.Selector, value.SelectorHash, value.Visibility))
	} else {
		saved, err = scanAPISDKBinding(tx.QueryRow(ctx, `UPDATE api_sdk_bindings SET sdk_release_id=$4,sdk_content_publication_id=nullif($5,'')::uuid,api_contract_revision_id=nullif($6,'')::uuid,compatibility_assertion_id=nullif($7,'')::uuid,binding_state=$8,coverage=$9,assurance=$10,applicable_modules=$11,applicable_capabilities=$12,applicable_operation_keys=$13,selector=$14,selector_hash=$15,visibility=$16,revision=revision+1,updated_at=now()
			WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$17 RETURNING id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at`,
			value.DeploymentID, value.APIID, value.ID, value.SDKReleaseID, value.SDKContentPublicationID,
			value.APIContractRevisionID, value.CompatibilityAssertionID, value.State, value.Coverage, value.Assurance,
			value.ApplicableModules, value.ApplicableCapabilities, value.ApplicableOperationKeys, value.Selector, value.SelectorHash,
			value.Visibility, expected))
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_sdk_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, value.DeploymentID, value.APIID, value.ID).Scan(&exists); lookupErr == nil {
				return model.APISDKBinding{}, ErrConflict
			}
		}
	}
	if err != nil {
		return model.APISDKBinding{}, err
	}
	if err := syncLegacySDKReferenceTx(ctx, tx, saved.ID); err != nil {
		return model.APISDKBinding{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, saved.DeploymentID); err != nil {
		return model.APISDKBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APISDKBinding{}, err
	}
	return saved, nil
}

func (p *Postgres) DetachAPISDKBinding(ctx context.Context, deploymentID, apiID, id string, expected int64) (model.APISDKBinding, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APISDKBinding{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := scanAPISDKBinding(tx.QueryRow(ctx, `UPDATE api_sdk_bindings SET binding_state='detached',revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$4 RETURNING id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at`, deploymentID, apiID, id, expected))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_sdk_bindings WHERE deployment_id=$1 AND integration_id=$2 AND id=$3`, deploymentID, apiID, id).Scan(&exists); lookupErr == nil {
				return model.APISDKBinding{}, ErrConflict
			}
		}
		return model.APISDKBinding{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sdk_references WHERE id=$1`, id); err != nil {
		return model.APISDKBinding{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
		return model.APISDKBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APISDKBinding{}, err
	}
	return value, nil
}

const apiDeveloperAssetPublicationSelect = `SELECT id::text,deployment_id::text,integration_id::text,integration_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),snapshot_schema_version,snapshot_hash,published_by,published_at,created_at FROM api_developer_asset_publications`

func scanAPIDeveloperAssetPublication(row pgx.Row) (model.APIDeveloperAssetPublication, error) {
	var value model.APIDeveloperAssetPublication
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIID, &value.APIRevisionID,
		&value.DeploymentDocumentationPublicationID, &value.SnapshotSchemaVersion, &value.SnapshotHash,
		&value.PublishedBy, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichAPIDeveloperAssetPublication(ctx context.Context, value model.APIDeveloperAssetPublication) (model.APIDeveloperAssetPublication, error) {
	rows, err := p.pool.Query(ctx, `SELECT api_documentation_binding_id::text,documentation_collection_id::text,documentation_collection_name,documentation_collection_slug,documentation_collection_description,documentation_collection_revision_id::text,selector,selector_hash,content_hash,visibility,ordinal FROM api_publication_documentation_assets WHERE api_developer_asset_publication_id=$1 ORDER BY ordinal`, value.ID)
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var item model.APIPublicationDocumentationAsset
		if err := rows.Scan(&item.BindingID, &item.DocumentationCollectionID, &item.DocumentationCollectionName,
			&item.DocumentationCollectionSlug, &item.DocumentationCollectionDescription,
			&item.DocumentationCollectionRevisionID, &item.Selector, &item.SelectorHash,
			&item.ContentHash, &item.Visibility, &item.Ordinal); err != nil {
			rows.Close()
			return value, databaseError(err)
		}
		value.Documentation = append(value.Documentation, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return value, err
	}
	rows, err = p.pool.Query(ctx, `SELECT api_contract_binding_id::text,api_contract_id::text,api_contract_name,api_contract_slug,api_contract_description,api_contract_kind,api_contract_revision_id::text,primary_contract,content_hash,visibility,ordinal FROM api_publication_contract_assets WHERE api_developer_asset_publication_id=$1 ORDER BY ordinal`, value.ID)
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var item model.APIPublicationContractAsset
		if err := rows.Scan(&item.BindingID, &item.APIContractID, &item.APIContractName, &item.APIContractSlug,
			&item.APIContractDescription, &item.APIContractKind, &item.APIContractRevisionID,
			&item.Primary, &item.ContentHash, &item.Visibility, &item.Ordinal); err != nil {
			rows.Close()
			return value, databaseError(err)
		}
		value.Contracts = append(value.Contracts, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return value, err
	}
	rows, err = p.pool.Query(ctx, `SELECT api_sdk_binding_id::text,sdk_package_id::text,sdk_package_ecosystem,sdk_package_coordinate,sdk_package_display_coordinate,sdk_package_display_name,sdk_package_language,sdk_package_platform,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(compatibility_assertion_id::text,''),selector,selector_hash,content_hash,visibility,ordinal FROM api_publication_sdk_assets WHERE api_developer_asset_publication_id=$1 ORDER BY ordinal`, value.ID)
	if err != nil {
		return value, databaseError(err)
	}
	for rows.Next() {
		var item model.APIPublicationSDKAsset
		if err := rows.Scan(&item.BindingID, &item.SDKPackageID, &item.SDKPackageEcosystem, &item.SDKPackageCoordinate,
			&item.SDKPackageDisplayCoordinate, &item.SDKPackageDisplayName, &item.SDKPackageLanguage, &item.SDKPackagePlatform,
			&item.SDKReleaseID, &item.SDKContentPublicationID,
			&item.CompatibilityAssertionID, &item.Selector, &item.SelectorHash, &item.ContentHash, &item.Visibility, &item.Ordinal); err != nil {
			return value, databaseError(err)
		}
		value.SDKs = append(value.SDKs, item)
	}
	return value, rows.Err()
}

func (p *Postgres) APIDeveloperAssetPublications(ctx context.Context, deploymentID, apiID string) ([]model.APIDeveloperAssetPublication, error) {
	rows, err := p.pool.Query(ctx, apiDeveloperAssetPublicationSelect+` WHERE deployment_id=$1 AND integration_id=$2 ORDER BY published_at DESC`, deploymentID, apiID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.APIDeveloperAssetPublication, 0)
	for rows.Next() {
		value, scanErr := scanAPIDeveloperAssetPublication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.APIDeveloperAssetPublication, 0, len(base))
	for _, value := range base {
		enriched, enrichErr := p.enrichAPIDeveloperAssetPublication(ctx, value)
		if enrichErr != nil {
			return nil, enrichErr
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) APIDeveloperAssetPublication(ctx context.Context, deploymentID, id string) (model.APIDeveloperAssetPublication, error) {
	value, err := scanAPIDeveloperAssetPublication(p.pool.QueryRow(ctx, apiDeveloperAssetPublicationSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return value, err
	}
	return p.enrichAPIDeveloperAssetPublication(ctx, value)
}

func (p *Postgres) CreateAPIDeveloperAssetPublication(ctx context.Context, value model.APIDeveloperAssetPublication) (model.APIDeveloperAssetPublication, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanAPIDeveloperAssetPublication(tx.QueryRow(ctx, `INSERT INTO api_developer_asset_publications(id,deployment_id,integration_id,integration_revision_id,deployment_documentation_publication_id,snapshot_schema_version,snapshot_hash,published_by,published_at)
		VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9) RETURNING id::text,deployment_id::text,integration_id::text,integration_revision_id::text,coalesce(deployment_documentation_publication_id::text,''),snapshot_schema_version,snapshot_hash,published_by,published_at,created_at`,
		value.ID, value.DeploymentID, value.APIID, value.APIRevisionID, value.DeploymentDocumentationPublicationID,
		value.SnapshotSchemaVersion, value.SnapshotHash, value.PublishedBy, value.PublishedAt))
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	for index := range value.Documentation {
		item := &value.Documentation[index]
		revision, lookupErr := scanDocumentationCollectionRevision(tx.QueryRow(ctx, documentationCollectionRevisionSelect+` WHERE deployment_id=$1 AND id=$2`, created.DeploymentID, item.DocumentationCollectionRevisionID))
		if lookupErr != nil {
			return model.APIDeveloperAssetPublication{}, lookupErr
		}
		if normalizeErr := snapshotAPIDocumentationAssetIdentity(revision, item); normalizeErr != nil {
			return model.APIDeveloperAssetPublication{}, normalizeErr
		}
		result, insertErr := tx.Exec(ctx, `INSERT INTO api_publication_documentation_assets(api_developer_asset_publication_id,deployment_id,integration_id,api_documentation_binding_id,documentation_collection_id,documentation_collection_name,documentation_collection_slug,documentation_collection_description,documentation_collection_revision_id,selector,selector_hash,content_hash,visibility,ordinal)
			SELECT $1,$2,$3,$4,binding.documentation_collection_id,$5,$6,$7,$8,$9,$10,$11,$12,$13 FROM api_documentation_bindings binding WHERE binding.id=$4 AND binding.integration_id=$3 AND binding.documentation_collection_id=$14`,
			created.ID, created.DeploymentID, created.APIID, item.BindingID,
			item.DocumentationCollectionName, item.DocumentationCollectionSlug, item.DocumentationCollectionDescription,
			item.DocumentationCollectionRevisionID, item.Selector, item.SelectorHash, item.ContentHash, item.Visibility, item.Ordinal,
			item.DocumentationCollectionID)
		if insertErr != nil {
			return model.APIDeveloperAssetPublication{}, databaseError(insertErr)
		}
		if result.RowsAffected() != 1 {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
	}
	for index := range value.Contracts {
		item := &value.Contracts[index]
		revision, lookupErr := scanAPIContractRevision(tx.QueryRow(ctx, apiContractRevisionSelect+` WHERE deployment_id=$1 AND id=$2`, created.DeploymentID, item.APIContractRevisionID))
		if lookupErr != nil {
			return model.APIDeveloperAssetPublication{}, lookupErr
		}
		if normalizeErr := snapshotAPIContractAssetIdentity(revision, item); normalizeErr != nil {
			return model.APIDeveloperAssetPublication{}, normalizeErr
		}
		result, insertErr := tx.Exec(ctx, `INSERT INTO api_publication_contract_assets(api_developer_asset_publication_id,deployment_id,integration_id,api_contract_binding_id,api_contract_id,api_contract_name,api_contract_slug,api_contract_description,api_contract_kind,api_contract_revision_id,primary_contract,content_hash,visibility,ordinal)
			SELECT $1,$2,$3,$4,binding.api_contract_id,$5,$6,$7,$8,$9,$10,$11,$12,$13 FROM api_contract_bindings binding WHERE binding.id=$4 AND binding.integration_id=$3 AND binding.api_contract_id=$14`,
			created.ID, created.DeploymentID, created.APIID, item.BindingID,
			item.APIContractName, item.APIContractSlug, item.APIContractDescription, item.APIContractKind,
			item.APIContractRevisionID, item.Primary, item.ContentHash, item.Visibility, item.Ordinal, item.APIContractID)
		if insertErr != nil {
			return model.APIDeveloperAssetPublication{}, databaseError(insertErr)
		}
		if result.RowsAffected() != 1 {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
	}
	for _, item := range value.SDKs {
		_, err = tx.Exec(ctx, `INSERT INTO api_publication_sdk_assets(api_developer_asset_publication_id,deployment_id,integration_id,api_sdk_binding_id,sdk_package_id,sdk_package_ecosystem,sdk_package_coordinate,sdk_package_display_coordinate,sdk_package_display_name,sdk_package_language,sdk_package_platform,sdk_release_id,sdk_content_publication_id,compatibility_assertion_id,selector,selector_hash,content_hash,visibility,ordinal)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,nullif($13,'')::uuid,nullif($14,'')::uuid,$15,$16,$17,$18,$19)`, created.ID, created.DeploymentID,
			created.APIID, item.BindingID, item.SDKPackageID, item.SDKPackageEcosystem, item.SDKPackageCoordinate,
			item.SDKPackageDisplayCoordinate, item.SDKPackageDisplayName, item.SDKPackageLanguage, item.SDKPackagePlatform,
			item.SDKReleaseID, item.SDKContentPublicationID, item.CompatibilityAssertionID,
			item.Selector, item.SelectorHash, item.ContentHash, item.Visibility, item.Ordinal)
		if err != nil {
			return model.APIDeveloperAssetPublication{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	created.Documentation = memoryClone(value.Documentation)
	created.Contracts = memoryClone(value.Contracts)
	created.SDKs = memoryClone(value.SDKs)
	return created, nil
}
