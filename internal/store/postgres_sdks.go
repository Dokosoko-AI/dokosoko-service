package store

import (
	"context"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
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

func (p *Postgres) SaveSDKReference(ctx context.Context, value model.SDKReference, expected int64) (model.SDKReference, error) {
	if expected == 0 {
		return scanSDKReference(p.pool.QueryRow(ctx, `INSERT INTO sdk_references(id,deployment_id,organisation_id,integration_id,ecosystem,coordinate,exact_version,install_command,documentation_url,source_url,checksum,visibility) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM integrations integration WHERE integration.id=$4 AND integration.deployment_id=$2 AND integration.organisation_id=$3 RETURNING `+sdkReferenceColumns, value.ID, value.DeploymentID, value.OrganisationID, value.IntegrationID, value.Ecosystem, value.Coordinate, value.ExactVersion, value.InstallCommand, value.DocumentationURL, value.SourceURL, value.Checksum, value.Visibility))
	}
	updated, err := scanSDKReference(p.pool.QueryRow(ctx, `UPDATE sdk_references SET ecosystem=$3,coordinate=$4,exact_version=$5,install_command=$6,documentation_url=$7,source_url=$8,checksum=$9,visibility=$10,revision=revision+1,updated_at=now() WHERE integration_id=$1 AND id=$2 AND revision=$11 RETURNING `+sdkReferenceColumns, value.IntegrationID, value.ID, value.Ecosystem, value.Coordinate, value.ExactVersion, value.InstallCommand, value.DocumentationURL, value.SourceURL, value.Checksum, value.Visibility, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.SDKReference(ctx, value.IntegrationID, value.ID); lookupErr == nil {
			return model.SDKReference{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) DeleteSDKReference(ctx context.Context, integrationID, id string) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM sdk_references WHERE integration_id=$1 AND id=$2`, integrationID, id)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
