package store

import (
	"context"

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
