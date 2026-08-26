package store

import (
	"context"
	"errors"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const sdkReferenceColumns = `id::text,deployment_id::text,organisation_id::text,integration_id::text,ecosystem,coordinate,exact_version,install_command,documentation_url,source_url,checksum,visibility,revision,created_at,updated_at`

func scanSDKReference(row interface{ Scan(...any) error }) (model.SDKReference, error) {
	var value model.SDKReference
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.IntegrationID, &value.Ecosystem, &value.Coordinate, &value.ExactVersion, &value.InstallCommand, &value.DocumentationURL, &value.SourceURL, &value.Checksum, &value.Visibility, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKReferences(ctx context.Context, integrationID string) ([]model.SDKReference, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+sdkReferenceColumns+` FROM sdk_references WHERE integration_id=$1 ORDER BY ecosystem,coordinate`, integrationID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKReference, 0)
	for rows.Next() {
		value, scanErr := scanSDKReference(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SDKReference(ctx context.Context, integrationID, id string) (model.SDKReference, error) {
	return scanSDKReference(p.pool.QueryRow(ctx, `SELECT `+sdkReferenceColumns+` FROM sdk_references WHERE integration_id=$1 AND id=$2`, integrationID, id))
}

func findOrCreateLegacySDKPackageTx(ctx context.Context, tx pgx.Tx, value model.SDKReference) (model.SDKPackage, error) {
	canonicalCoordinate := model.CanonicalSDKCoordinate(value.Ecosystem, value.Coordinate)
	packageValue, err := scanSDKPackage(tx.QueryRow(ctx, sdkPackageSelect+`
		WHERE deployment_id=$1 AND ecosystem=$2 AND canonical_coordinate=$3 FOR UPDATE`,
		value.DeploymentID, value.Ecosystem, canonicalCoordinate))
	if errors.Is(err, ErrNotFound) {
		packageValue, err = scanSDKPackage(tx.QueryRow(ctx, `INSERT INTO sdk_packages(
			deployment_id,organisation_id,ecosystem,canonical_coordinate,display_coordinate,name,visibility,lifecycle
		) VALUES($1,$2,$3,$4,$5,$5,$6,'active')
		ON CONFLICT (deployment_id,ecosystem,canonical_coordinate) DO NOTHING
		RETURNING id::text,deployment_id::text,organisation_id::text,ecosystem,canonical_coordinate,display_coordinate,name,description,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_sdk_package_id::text,''),deprecation_message,revision,created_at,updated_at`,
			value.DeploymentID, value.OrganisationID, value.Ecosystem, canonicalCoordinate, value.Coordinate, value.Visibility))
		if errors.Is(err, ErrNotFound) {
			packageValue, err = scanSDKPackage(tx.QueryRow(ctx, sdkPackageSelect+`
				WHERE deployment_id=$1 AND ecosystem=$2 AND canonical_coordinate=$3 FOR UPDATE`,
				value.DeploymentID, value.Ecosystem, canonicalCoordinate))
		}
	}
	if err != nil {
		return model.SDKPackage{}, err
	}
	if value.Visibility == model.VisibilityPublic && packageValue.Visibility != model.VisibilityPublic {
		return model.SDKPackage{}, ErrConflict
	}
	return packageValue, nil
}

func findOrCreateLegacySDKReleaseTx(ctx context.Context, tx pgx.Tx, packageValue model.SDKPackage, value model.SDKReference) (model.SDKRelease, error) {
	release, err := scanSDKRelease(tx.QueryRow(ctx, sdkReleaseSelect+`
		WHERE deployment_id=$1 AND sdk_package_id=$2 AND exact_version=$3 FOR UPDATE`,
		value.DeploymentID, packageValue.ID, value.ExactVersion))
	if errors.Is(err, ErrNotFound) {
		releaseHash, hashErr := legacySDKReleaseHash(packageValue.ID, value)
		if hashErr != nil {
			return model.SDKRelease{}, hashErr
		}
		release, err = scanSDKRelease(tx.QueryRow(ctx, `INSERT INTO sdk_releases(
			deployment_id,sdk_package_id,exact_version,install_command,documentation_url,source_url,
			upstream_digest,identity_assurance,visibility,lifecycle,release_hash
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'active',$10)
		ON CONFLICT (sdk_package_id,exact_version) DO NOTHING
		RETURNING id::text,deployment_id::text,sdk_package_id::text,exact_version,install_command,documentation_url,source_url,resolved_source_revision,upstream_digest,identity_assurance,visibility,lifecycle,release_hash,published_at,created_at`,
			value.DeploymentID, packageValue.ID, value.ExactVersion, value.InstallCommand,
			value.DocumentationURL, value.SourceURL, value.Checksum,
			legacySDKIdentityAssurance(value.Checksum), value.Visibility, releaseHash))
		if errors.Is(err, ErrNotFound) {
			release, err = scanSDKRelease(tx.QueryRow(ctx, sdkReleaseSelect+`
				WHERE deployment_id=$1 AND sdk_package_id=$2 AND exact_version=$3 FOR UPDATE`,
				value.DeploymentID, packageValue.ID, value.ExactVersion))
		}
	}
	if err != nil {
		return model.SDKRelease{}, err
	}
	if !legacySDKReleaseMatches(release, value) {
		return model.SDKRelease{}, ErrConflict
	}
	return release, nil
}

func (p *Postgres) SaveSDKReference(ctx context.Context, value model.SDKReference, expected int64) (model.SDKReference, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SDKReference{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var integrationExists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM integrations
		WHERE id=$1 AND deployment_id=$2 AND organisation_id=$3 FOR KEY SHARE`,
		value.IntegrationID, value.DeploymentID, value.OrganisationID).Scan(&integrationExists); err != nil {
		return model.SDKReference{}, databaseError(err)
	}

	currentReference, referenceErr := scanSDKReference(tx.QueryRow(ctx, `SELECT `+sdkReferenceColumns+`
		FROM sdk_references WHERE integration_id=$1 AND id=$2 FOR UPDATE`, value.IntegrationID, value.ID))
	referenceExists := referenceErr == nil
	if referenceErr != nil && !errors.Is(referenceErr, ErrNotFound) {
		return model.SDKReference{}, referenceErr
	}
	if (expected == 0 && referenceExists) || (expected != 0 && (!referenceExists || currentReference.Revision != expected)) {
		return model.SDKReference{}, ErrConflict
	}
	if referenceExists {
		var migrationStatus string
		migrationErr := tx.QueryRow(ctx, `SELECT status FROM legacy_sdk_reference_migration_ledger
			WHERE legacy_sdk_reference_id=$1`, value.ID).Scan(&migrationStatus)
		if migrationErr != nil && !errors.Is(migrationErr, pgx.ErrNoRows) {
			return model.SDKReference{}, databaseError(migrationErr)
		}
		if migrationStatus == "conflict" {
			return model.SDKReference{}, ErrConflict
		}
	}

	packageValue, err := findOrCreateLegacySDKPackageTx(ctx, tx, value)
	if err != nil {
		return model.SDKReference{}, err
	}
	release, err := findOrCreateLegacySDKReleaseTx(ctx, tx, packageValue, value)
	if err != nil {
		return model.SDKReference{}, err
	}

	currentBinding, bindingErr := scanAPISDKBinding(tx.QueryRow(ctx, apiSDKBindingSelect+`
		WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 FOR UPDATE`,
		value.DeploymentID, value.IntegrationID, value.ID))
	bindingExists := bindingErr == nil
	if bindingErr != nil && !errors.Is(bindingErr, ErrNotFound) {
		return model.SDKReference{}, bindingErr
	}
	if bindingExists && (currentBinding.Revision != expected || currentBinding.State != "legacy_metadata") {
		return model.SDKReference{}, ErrConflict
	}

	var saved model.APISDKBinding
	if bindingExists {
		saved, err = scanAPISDKBinding(tx.QueryRow(ctx, `UPDATE api_sdk_bindings SET
			sdk_package_id=$4,sdk_release_id=$5,sdk_content_publication_id=NULL,
			api_contract_revision_id=NULL,compatibility_assertion_id=NULL,
			binding_state='legacy_metadata',coverage='unknown',assurance='related',
			applicable_modules='{}',applicable_capabilities='{}',applicable_operation_keys='{}',
			selector='{}'::jsonb,selector_hash=$6,visibility=$7,revision=revision+1,updated_at=now()
			WHERE deployment_id=$1 AND integration_id=$2 AND id=$3 AND revision=$8
			RETURNING id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at`,
			value.DeploymentID, value.IntegrationID, value.ID, packageValue.ID, release.ID,
			legacySDKSelectorHash(), value.Visibility, expected))
	} else {
		bindingRevision := int64(1)
		createdAt := time.Now().UTC()
		if expected != 0 {
			bindingRevision = expected + 1
			createdAt = currentReference.CreatedAt
		}
		saved, err = scanAPISDKBinding(tx.QueryRow(ctx, `INSERT INTO api_sdk_bindings(
			id,deployment_id,integration_id,sdk_package_id,sdk_release_id,binding_state,
			coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,
			selector,selector_hash,visibility,revision,created_by,created_at
		) VALUES($1,$2,$3,$4,$5,'legacy_metadata','unknown','related','{}','{}','{}',
			'{}'::jsonb,$6,$7,$8,'legacy-sdk-reference',$9)
		RETURNING id::text,deployment_id::text,integration_id::text,sdk_package_id::text,sdk_release_id::text,coalesce(sdk_content_publication_id::text,''),coalesce(api_contract_revision_id::text,''),coalesce(compatibility_assertion_id::text,''),binding_state,coverage,assurance,applicable_modules,applicable_capabilities,applicable_operation_keys,selector,selector_hash,visibility,revision,created_at,updated_at`,
			value.ID, value.DeploymentID, value.IntegrationID, packageValue.ID, release.ID,
			legacySDKSelectorHash(), value.Visibility, bindingRevision, createdAt))
	}
	if err != nil {
		return model.SDKReference{}, databaseError(err)
	}
	if saved.ID != value.ID {
		return model.SDKReference{}, ErrConflict
	}
	if err := syncLegacySDKReferenceTx(ctx, tx, saved.ID); err != nil {
		return model.SDKReference{}, err
	}
	result, err := scanSDKReference(tx.QueryRow(ctx, `SELECT `+sdkReferenceColumns+`
		FROM sdk_references WHERE integration_id=$1 AND id=$2`, value.IntegrationID, value.ID))
	if err != nil {
		return model.SDKReference{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.SDKReference{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SDKReference{}, databaseError(err)
	}
	return result, nil
}

func (p *Postgres) DeleteSDKReference(ctx context.Context, integrationID, id string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var deploymentID string
	err = tx.QueryRow(ctx, `UPDATE api_sdk_bindings
		SET binding_state='detached',revision=revision+1,updated_at=now()
		WHERE integration_id=$1 AND id=$2 AND binding_state<>'detached'
		RETURNING deployment_id::text`, integrationID, id).Scan(&deploymentID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return databaseError(err)
	}
	bindingDetached := err == nil
	result, err := tx.Exec(ctx, `DELETE FROM sdk_references WHERE integration_id=$1 AND id=$2`, integrationID, id)
	if err != nil {
		return databaseError(err)
	}
	if !bindingDetached && result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if bindingDetached {
		if err := bumpDeploymentCatalog(ctx, tx, deploymentID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return databaseError(err)
	}
	return nil
}
