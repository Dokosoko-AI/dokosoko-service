package store

import (
	"context"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const documentationCollectionSelect = `SELECT id::text,deployment_id::text,organisation_id::text,name,slug,description,visibility,lifecycle,revision,created_at,updated_at FROM documentation_collections`

func scanDocumentationCollection(row pgx.Row) (model.DocumentationCollection, error) {
	var value model.DocumentationCollection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.Slug, &value.Description,
		&value.Visibility, &value.Lifecycle, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

const documentationCollectionRevisionSelect = `SELECT id::text,deployment_id::text,documentation_collection_id::text,documentation_collection_name,documentation_collection_slug,documentation_collection_description,revision,visibility,content_hash,selection_manifest,reviewed_by,reviewed_at,published_at,created_at FROM documentation_collection_revisions`

func scanDocumentationCollectionRevision(row pgx.Row) (model.DocumentationCollectionRevision, error) {
	var value model.DocumentationCollectionRevision
	err := row.Scan(&value.ID, &value.DeploymentID, &value.DocumentationCollectionID, &value.DocumentationCollectionName,
		&value.DocumentationCollectionSlug, &value.DocumentationCollectionDescription, &value.Revision, &value.Visibility,
		&value.ContentHash, &value.SelectionManifest, &value.ReviewedBy, &value.ReviewedAt, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) DocumentationCollections(ctx context.Context, deploymentID string) ([]model.DocumentationCollection, error) {
	rows, err := p.pool.Query(ctx, documentationCollectionSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.DocumentationCollection, 0)
	for rows.Next() {
		value, scanErr := scanDocumentationCollection(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) DocumentationCollection(ctx context.Context, deploymentID, id string) (model.DocumentationCollection, error) {
	return scanDocumentationCollection(p.pool.QueryRow(ctx, documentationCollectionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func insertDocumentationCollectionRevisionTx(ctx context.Context, tx pgx.Tx, record DocumentationCollectionRevisionRecord) error {
	value := record.Revision
	_, err := tx.Exec(ctx, `INSERT INTO documentation_collection_revisions(id,deployment_id,documentation_collection_id,documentation_collection_name,documentation_collection_slug,documentation_collection_description,revision,visibility,content_hash,selection_manifest,reviewed_by,reviewed_at,published_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, value.ID, value.DeploymentID, value.DocumentationCollectionID,
		value.DocumentationCollectionName, value.DocumentationCollectionSlug, value.DocumentationCollectionDescription,
		value.Revision, value.Visibility, value.ContentHash, value.SelectionManifest, value.ReviewedBy, value.ReviewedAt, value.PublishedAt)
	if err != nil {
		return databaseError(err)
	}
	for _, member := range record.Members {
		_, err = tx.Exec(ctx, `INSERT INTO documentation_collection_members(id,deployment_id,documentation_collection_revision_id,source_publication_id,documentation_document_id,documentation_section_id,member_kind,ordinal,include_descendants,selector)
			VALUES($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,nullif($6,'')::uuid,$7,$8,$9,$10)`, member.ID, value.DeploymentID,
			member.DocumentationCollectionRevisionID, member.SourcePublicationID, member.DocumentationDocumentID, member.DocumentationSectionID,
			member.Kind, member.Ordinal, member.IncludeDescendants, member.Selector)
		if err != nil {
			return databaseError(err)
		}
	}
	if record.Map != nil {
		return insertDocumentationMapTx(ctx, tx, *record.Map)
	}
	return nil
}

func (p *Postgres) CreateDocumentationCollection(ctx context.Context, value model.DocumentationCollection, record DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanDocumentationCollection(tx.QueryRow(ctx, `INSERT INTO documentation_collections(id,deployment_id,organisation_id,name,slug,description,visibility,lifecycle)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,deployment_id::text,organisation_id::text,name,slug,description,visibility,lifecycle,revision,created_at,updated_at`,
		value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.Slug, value.Description, value.Visibility, value.Lifecycle))
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := snapshotDocumentationCollectionIdentity(created, &record.Revision); err != nil {
		return model.DocumentationCollection{}, err
	}
	record.Revision.Revision = created.Revision
	if err := insertDocumentationCollectionRevisionTx(ctx, tx, record); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.DocumentationCollection{}, err
	}
	return created, nil
}

func (p *Postgres) ReviseDocumentationCollection(ctx context.Context, value model.DocumentationCollection, expected int64, record DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.DocumentationCollection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanDocumentationCollection(tx.QueryRow(ctx, `UPDATE documentation_collections SET name=$3,slug=$4,description=$5,visibility=$6,lifecycle=$7,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$8 RETURNING id::text,deployment_id::text,organisation_id::text,name,slug,description,visibility,lifecycle,revision,created_at,updated_at`,
		value.DeploymentID, value.ID, value.Name, value.Slug, value.Description, value.Visibility, value.Lifecycle, expected))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM documentation_collections WHERE deployment_id=$1 AND id=$2`, value.DeploymentID, value.ID).Scan(&exists); lookupErr == nil {
				return model.DocumentationCollection{}, ErrConflict
			}
		}
		return model.DocumentationCollection{}, err
	}
	if err := snapshotDocumentationCollectionIdentity(updated, &record.Revision); err != nil {
		return model.DocumentationCollection{}, err
	}
	record.Revision.Revision = updated.Revision
	if err := insertDocumentationCollectionRevisionTx(ctx, tx, record); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.DocumentationCollection{}, err
	}
	return updated, nil
}

func (p *Postgres) DocumentationCollectionRevisions(ctx context.Context, deploymentID, collectionID string) ([]model.DocumentationCollectionRevision, error) {
	rows, err := p.pool.Query(ctx, documentationCollectionRevisionSelect+` WHERE deployment_id=$1 AND documentation_collection_id=$2 ORDER BY revision DESC`, deploymentID, collectionID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.DocumentationCollectionRevision, 0)
	for rows.Next() {
		value, scanErr := scanDocumentationCollectionRevision(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		if _, lookupErr := p.DocumentationCollection(ctx, deploymentID, collectionID); lookupErr != nil {
			return nil, lookupErr
		}
	}
	return result, rows.Err()
}

func (p *Postgres) DocumentationCollectionRevision(ctx context.Context, deploymentID, id string) (DocumentationCollectionRevisionRecord, error) {
	var result DocumentationCollectionRevisionRecord
	value, err := scanDocumentationCollectionRevision(p.pool.QueryRow(ctx, documentationCollectionRevisionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Revision = value
	rows, err := p.pool.Query(ctx, `SELECT id::text,documentation_collection_revision_id::text,member_kind,coalesce(source_publication_id::text,''),coalesce(documentation_document_id::text,''),coalesce(documentation_section_id::text,''),ordinal,include_descendants,selector
		FROM documentation_collection_members WHERE deployment_id=$1 AND documentation_collection_revision_id=$2 ORDER BY ordinal`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var member model.DocumentationCollectionMember
		if err := rows.Scan(&member.ID, &member.DocumentationCollectionRevisionID, &member.Kind, &member.SourcePublicationID,
			&member.DocumentationDocumentID, &member.DocumentationSectionID, &member.Ordinal, &member.IncludeDescendants, &member.Selector); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.Members = append(result.Members, member)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	mapValue, mapErr := scanDocumentationMap(p.pool.QueryRow(ctx, documentationMapSelect+` WHERE deployment_id=$1 AND documentation_collection_revision_id=$2 ORDER BY created_at DESC LIMIT 1`, deploymentID, id))
	if mapErr == nil {
		result.Map = &mapValue
	} else if !errors.Is(mapErr, ErrNotFound) {
		return result, mapErr
	}
	return result, nil
}

const deploymentDocumentationPublicationSelect = `SELECT id::text,deployment_id::text,revision,visibility,snapshot_schema_version,snapshot_hash,published_by,published_at,created_at FROM deployment_documentation_publications`

func scanDeploymentDocumentationPublication(row pgx.Row) (model.DeploymentDocumentationPublication, error) {
	var value model.DeploymentDocumentationPublication
	err := row.Scan(&value.ID, &value.DeploymentID, &value.Revision, &value.Visibility, &value.SnapshotSchemaVersion,
		&value.SnapshotHash, &value.PublishedBy, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) enrichDeploymentDocumentationPublication(ctx context.Context, value model.DeploymentDocumentationPublication) (model.DeploymentDocumentationPublication, error) {
	rows, err := p.pool.Query(ctx, `SELECT documentation_collection_revision_id::text,ordinal,content_hash,visibility FROM deployment_documentation_publication_members WHERE deployment_documentation_publication_id=$1 ORDER BY ordinal`, value.ID)
	if err != nil {
		return model.DeploymentDocumentationPublication{}, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var member model.DeploymentDocumentationPublicationMember
		if err := rows.Scan(&member.DocumentationCollectionRevisionID, &member.Ordinal, &member.ContentHash, &member.Visibility); err != nil {
			return model.DeploymentDocumentationPublication{}, databaseError(err)
		}
		value.Members = append(value.Members, member)
	}
	return value, rows.Err()
}

func (p *Postgres) DeploymentDocumentationPublications(ctx context.Context, deploymentID string) ([]model.DeploymentDocumentationPublication, error) {
	rows, err := p.pool.Query(ctx, deploymentDocumentationPublicationSelect+` WHERE deployment_id=$1 ORDER BY revision DESC`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	base := make([]model.DeploymentDocumentationPublication, 0)
	for rows.Next() {
		value, scanErr := scanDeploymentDocumentationPublication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		base = append(base, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]model.DeploymentDocumentationPublication, 0, len(base))
	for _, value := range base {
		enriched, enrichErr := p.enrichDeploymentDocumentationPublication(ctx, value)
		if enrichErr != nil {
			return nil, enrichErr
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (p *Postgres) DeploymentDocumentationPublication(ctx context.Context, deploymentID, id string) (model.DeploymentDocumentationPublication, error) {
	value, err := scanDeploymentDocumentationPublication(p.pool.QueryRow(ctx, deploymentDocumentationPublicationSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	return p.enrichDeploymentDocumentationPublication(ctx, value)
}

func (p *Postgres) ActiveDeploymentDocumentationPublication(ctx context.Context, deploymentID string) (model.DeploymentDocumentationPublication, int64, error) {
	var publicationID string
	var revision int64
	if err := p.pool.QueryRow(ctx, `SELECT deployment_documentation_publication_id::text,revision FROM deployment_documentation_heads WHERE deployment_id=$1`, deploymentID).Scan(&publicationID, &revision); err != nil {
		return model.DeploymentDocumentationPublication{}, 0, databaseError(err)
	}
	value, err := p.DeploymentDocumentationPublication(ctx, deploymentID, publicationID)
	return value, revision, err
}

func (p *Postgres) PublishDeploymentDocumentation(ctx context.Context, value model.DeploymentDocumentationPublication, expectedHeadRevision int64) (model.DeploymentDocumentationPublication, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanDeploymentDocumentationPublication(tx.QueryRow(ctx, `INSERT INTO deployment_documentation_publications(id,deployment_id,revision,visibility,snapshot_schema_version,snapshot_hash,published_by,published_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,deployment_id::text,revision,visibility,snapshot_schema_version,snapshot_hash,published_by,published_at,created_at`,
		value.ID, value.DeploymentID, value.Revision, value.Visibility, value.SnapshotSchemaVersion, value.SnapshotHash, value.PublishedBy, value.PublishedAt))
	if err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	for _, member := range value.Members {
		_, err = tx.Exec(ctx, `INSERT INTO deployment_documentation_publication_members(deployment_documentation_publication_id,deployment_id,documentation_collection_revision_id,ordinal,content_hash,visibility)
			VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.DeploymentID, member.DocumentationCollectionRevisionID, member.Ordinal, member.ContentHash, member.Visibility)
		if err != nil {
			return model.DeploymentDocumentationPublication{}, databaseError(err)
		}
	}
	if expectedHeadRevision == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO deployment_documentation_heads(deployment_id,deployment_documentation_publication_id,revision,updated_by) VALUES($1,$2,1,$3)`, value.DeploymentID, value.ID, value.PublishedBy)
	} else {
		var revision int64
		err = tx.QueryRow(ctx, `UPDATE deployment_documentation_heads SET deployment_documentation_publication_id=$2,revision=revision+1,updated_by=$3,updated_at=now() WHERE deployment_id=$1 AND revision=$4 RETURNING revision`, value.DeploymentID, value.ID, value.PublishedBy, expectedHeadRevision).Scan(&revision)
		if errors.Is(databaseError(err), ErrNotFound) {
			return model.DeploymentDocumentationPublication{}, ErrConflict
		}
	}
	if err != nil {
		return model.DeploymentDocumentationPublication{}, databaseError(err)
	}
	if err := bumpDeploymentCatalog(ctx, tx, value.DeploymentID); err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.DeploymentDocumentationPublication{}, err
	}
	created.Members = memoryClone(value.Members)
	return created, nil
}

const apiContractSelect = `SELECT id::text,deployment_id::text,organisation_id::text,name,slug,description,contract_kind,visibility,lifecycle,revision,created_at,updated_at FROM api_contracts`

func scanAPIContract(row pgx.Row) (model.APIContract, error) {
	var value model.APIContract
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Name, &value.Slug, &value.Description,
		&value.Kind, &value.Visibility, &value.Lifecycle, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) APIContracts(ctx context.Context, deploymentID string) ([]model.APIContract, error) {
	rows, err := p.pool.Query(ctx, apiContractSelect+` WHERE deployment_id=$1 ORDER BY name`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIContract, 0)
	for rows.Next() {
		value, scanErr := scanAPIContract(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIContract(ctx context.Context, deploymentID, id string) (model.APIContract, error) {
	return scanAPIContract(p.pool.QueryRow(ctx, apiContractSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) SaveAPIContract(ctx context.Context, value model.APIContract, expected int64) (model.APIContract, error) {
	if expected == 0 {
		return scanAPIContract(p.pool.QueryRow(ctx, `INSERT INTO api_contracts(id,deployment_id,organisation_id,name,slug,description,contract_kind,visibility,lifecycle)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text,deployment_id::text,organisation_id::text,name,slug,description,contract_kind,visibility,lifecycle,revision,created_at,updated_at`,
			value.ID, value.DeploymentID, value.OrganisationID, value.Name, value.Slug, value.Description, value.Kind, value.Visibility, value.Lifecycle))
	}
	updated, err := scanAPIContract(p.pool.QueryRow(ctx, `UPDATE api_contracts SET name=$3,slug=$4,description=$5,visibility=$6,lifecycle=$7,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$8 RETURNING id::text,deployment_id::text,organisation_id::text,name,slug,description,contract_kind,visibility,lifecycle,revision,created_at,updated_at`,
		value.DeploymentID, value.ID, value.Name, value.Slug, value.Description, value.Visibility, value.Lifecycle, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.APIContract(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.APIContract{}, ErrConflict
		}
	}
	return updated, err
}

const apiContractSourceSelect = `SELECT id::text,deployment_id::text,api_contract_id::text,source_id::text,source_role,lifecycle,revision,created_by,created_at,updated_at FROM api_contract_sources`

func scanAPIContractSource(row pgx.Row) (model.APIContractSource, error) {
	var value model.APIContractSource
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIContractID, &value.SourceID, &value.SourceRole,
		&value.Lifecycle, &value.Revision, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) APIContractSources(ctx context.Context, deploymentID, contractID string) ([]model.APIContractSource, error) {
	rows, err := p.pool.Query(ctx, apiContractSourceSelect+` WHERE deployment_id=$1 AND api_contract_id=$2 ORDER BY source_role,created_at`, deploymentID, contractID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIContractSource, 0)
	for rows.Next() {
		value, scanErr := scanAPIContractSource(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIContractSource(ctx context.Context, deploymentID, id string) (model.APIContractSource, error) {
	return scanAPIContractSource(p.pool.QueryRow(ctx, apiContractSourceSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) ActiveAPIContractSourceBySource(ctx context.Context, deploymentID, sourceID string) (model.APIContractSource, error) {
	return scanAPIContractSource(p.pool.QueryRow(ctx, apiContractSourceSelect+` WHERE deployment_id=$1 AND source_id=$2 AND lifecycle='attached'`, deploymentID, sourceID))
}

func (p *Postgres) SaveAPIContractSource(ctx context.Context, value model.APIContractSource, expected int64) (model.APIContractSource, error) {
	if expected == 0 {
		return scanAPIContractSource(p.pool.QueryRow(ctx, `INSERT INTO api_contract_sources(id,deployment_id,api_contract_id,source_id,source_role,lifecycle,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,deployment_id::text,api_contract_id::text,source_id::text,source_role,lifecycle,revision,created_by,created_at,updated_at`,
			value.ID, value.DeploymentID, value.APIContractID, value.SourceID, value.SourceRole, value.Lifecycle, value.CreatedBy))
	}
	updated, err := scanAPIContractSource(p.pool.QueryRow(ctx, `UPDATE api_contract_sources SET source_role=$3,lifecycle=$4,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$5 RETURNING id::text,deployment_id::text,api_contract_id::text,source_id::text,source_role,lifecycle,revision,created_by,created_at,updated_at`,
		value.DeploymentID, value.ID, value.SourceRole, value.Lifecycle, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.APIContractSource(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.APIContractSource{}, ErrConflict
		}
	}
	return updated, err
}

func (p *Postgres) DetachAPIContractSource(ctx context.Context, deploymentID, id string, expected int64) (model.APIContractSource, error) {
	value, err := scanAPIContractSource(p.pool.QueryRow(ctx, `UPDATE api_contract_sources SET lifecycle='detached',revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$3 RETURNING id::text,deployment_id::text,api_contract_id::text,source_id::text,source_role,lifecycle,revision,created_by,created_at,updated_at`, deploymentID, id, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.APIContractSource(ctx, deploymentID, id); lookupErr == nil {
			return model.APIContractSource{}, ErrConflict
		}
	}
	return value, err
}

const apiContractCandidateSelect = `SELECT id::text,deployment_id::text,api_contract_id::text,ingestion_run_id::text,openapi_version,source_format,normalized_contract,source_hash,content_hash,validation_result,parser_version,visibility,diagnostics,created_at FROM api_contract_candidates`

func scanAPIContractCandidate(row pgx.Row) (model.APIContractCandidate, error) {
	var value model.APIContractCandidate
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIContractID, &value.IngestionRunID,
		&value.OpenAPIVersion, &value.SourceFormat, &value.NormalizedContract, &value.SourceHash, &value.ContentHash,
		&value.ValidationResult, &value.ParserVersion, &value.Visibility, &value.Diagnostics, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) APIContractCandidates(ctx context.Context, deploymentID, contractID string) ([]model.APIContractCandidate, error) {
	rows, err := p.pool.Query(ctx, apiContractCandidateSelect+` WHERE deployment_id=$1 AND api_contract_id=$2 ORDER BY created_at DESC`, deploymentID, contractID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIContractCandidate, 0)
	for rows.Next() {
		value, scanErr := scanAPIContractCandidate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIContractCandidate(ctx context.Context, deploymentID, id string) (APIContractCandidateRecord, error) {
	var result APIContractCandidateRecord
	value, err := scanAPIContractCandidate(p.pool.QueryRow(ctx, apiContractCandidateSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Candidate = value
	rows, err := p.pool.Query(ctx, `SELECT id::text,api_contract_candidate_id::text,operation_key,operation_id,method,path_template,tags,summary,description,security,request_schema_refs,response_schema_refs,content_hash,ordinal FROM api_contract_operations WHERE deployment_id=$1 AND api_contract_candidate_id=$2 ORDER BY ordinal`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.APIContractOperation
		if err := rows.Scan(&item.ID, &item.APIContractCandidateID, &item.OperationKey, &item.OperationID, &item.Method, &item.PathTemplate,
			&item.Tags, &item.Summary, &item.Description, &item.Security, &item.RequestSchemaRefs, &item.ResponseSchemaRefs, &item.ContentHash, &item.Ordinal); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.Operations = append(result.Operations, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,api_contract_candidate_id::text,schema_key,schema_document,content_hash FROM api_contract_schemas WHERE deployment_id=$1 AND api_contract_candidate_id=$2 ORDER BY schema_key`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.APIContractSchema
		if err := rows.Scan(&item.ID, &item.APIContractCandidateID, &item.SchemaKey, &item.Schema, &item.ContentHash); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.Schemas = append(result.Schemas, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,api_contract_candidate_id::text,coalesce(api_contract_operation_id::text,''),name,example_kind,media_type,status_code,value,content_hash FROM api_contract_examples WHERE deployment_id=$1 AND api_contract_candidate_id=$2 ORDER BY name`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.APIContractExample
		if err := rows.Scan(&item.ID, &item.APIContractCandidateID, &item.APIContractOperationID, &item.Name, &item.Kind, &item.MediaType, &item.StatusCode, &item.Value, &item.ContentHash); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.Examples = append(result.Examples, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	var mapValue model.APIContractMap
	err = p.pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,api_contract_candidate_id::text,map_version,structured_map,agent_markdown,content_hash,created_at FROM api_contract_maps WHERE deployment_id=$1 AND api_contract_candidate_id=$2 ORDER BY created_at DESC LIMIT 1`, deploymentID, id).
		Scan(&mapValue.ID, &mapValue.DeploymentID, &mapValue.APIContractCandidateID, &mapValue.MapVersion, &mapValue.Map, &mapValue.AgentMarkdown, &mapValue.ContentHash, &mapValue.CreatedAt)
	if err == nil {
		result.Map = &mapValue
	} else if databaseError(err) != ErrNotFound {
		return result, databaseError(err)
	}
	return result, nil
}

func (p *Postgres) CreateAPIContractCandidate(ctx context.Context, value APIContractCandidateRecord) (model.APIContractCandidate, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIContractCandidate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	candidate := value.Candidate
	created, err := scanAPIContractCandidate(tx.QueryRow(ctx, `INSERT INTO api_contract_candidates(id,deployment_id,api_contract_id,ingestion_run_id,openapi_version,source_format,normalized_contract,source_hash,content_hash,validation_result,parser_version,visibility,diagnostics)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text,deployment_id::text,api_contract_id::text,ingestion_run_id::text,openapi_version,source_format,normalized_contract,source_hash,content_hash,validation_result,parser_version,visibility,diagnostics,created_at`,
		candidate.ID, candidate.DeploymentID, candidate.APIContractID, candidate.IngestionRunID,
		candidate.OpenAPIVersion, candidate.SourceFormat, candidate.NormalizedContract, candidate.SourceHash, candidate.ContentHash,
		candidate.ValidationResult, candidate.ParserVersion, candidate.Visibility, candidate.Diagnostics))
	if err != nil {
		return model.APIContractCandidate{}, err
	}
	for _, item := range value.Operations {
		_, err = tx.Exec(ctx, `INSERT INTO api_contract_operations(id,deployment_id,api_contract_candidate_id,operation_key,operation_id,method,path_template,tags,summary,description,security,request_schema_refs,response_schema_refs,content_hash,ordinal)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, item.ID, candidate.DeploymentID, item.APIContractCandidateID,
			item.OperationKey, item.OperationID, item.Method, item.PathTemplate, item.Tags, item.Summary, item.Description, item.Security,
			item.RequestSchemaRefs, item.ResponseSchemaRefs, item.ContentHash, item.Ordinal)
		if err != nil {
			return model.APIContractCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.Schemas {
		_, err = tx.Exec(ctx, `INSERT INTO api_contract_schemas(id,deployment_id,api_contract_candidate_id,schema_key,schema_document,content_hash) VALUES($1,$2,$3,$4,$5,$6)`,
			item.ID, candidate.DeploymentID, item.APIContractCandidateID, item.SchemaKey, item.Schema, item.ContentHash)
		if err != nil {
			return model.APIContractCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.Examples {
		_, err = tx.Exec(ctx, `INSERT INTO api_contract_examples(id,deployment_id,api_contract_candidate_id,api_contract_operation_id,name,example_kind,media_type,status_code,value,content_hash)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10)`, item.ID, candidate.DeploymentID, item.APIContractCandidateID,
			item.APIContractOperationID, item.Name, item.Kind, item.MediaType, item.StatusCode, item.Value, item.ContentHash)
		if err != nil {
			return model.APIContractCandidate{}, databaseError(err)
		}
	}
	if value.Map != nil {
		item := *value.Map
		_, err = tx.Exec(ctx, `INSERT INTO api_contract_maps(id,deployment_id,api_contract_candidate_id,map_version,structured_map,agent_markdown,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			item.ID, item.DeploymentID, item.APIContractCandidateID, item.MapVersion, item.Map, item.AgentMarkdown, item.ContentHash)
		if err != nil {
			return model.APIContractCandidate{}, databaseError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIContractCandidate{}, err
	}
	return created, nil
}

const apiContractRevisionSelect = `SELECT id::text,deployment_id::text,api_contract_id::text,api_contract_name,api_contract_slug,api_contract_description,api_contract_kind,api_contract_candidate_id::text,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at,created_at FROM api_contract_revisions`

func scanAPIContractRevision(row pgx.Row) (model.APIContractRevision, error) {
	var value model.APIContractRevision
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIContractID, &value.APIContractName, &value.APIContractSlug,
		&value.APIContractDescription, &value.APIContractKind, &value.APIContractCandidateID, &value.Revision,
		&value.ContentHash, &value.Visibility, &value.ReviewedBy, &value.ReviewedAt, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) APIContractRevisions(ctx context.Context, deploymentID, contractID string) ([]model.APIContractRevision, error) {
	rows, err := p.pool.Query(ctx, apiContractRevisionSelect+` WHERE deployment_id=$1 AND api_contract_id=$2 ORDER BY revision DESC`, deploymentID, contractID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.APIContractRevision, 0)
	for rows.Next() {
		value, scanErr := scanAPIContractRevision(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) APIContractRevision(ctx context.Context, deploymentID, id string) (model.APIContractRevision, error) {
	return scanAPIContractRevision(p.pool.QueryRow(ctx, apiContractRevisionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) PublishAPIContractCandidate(ctx context.Context, contract model.APIContract, expected int64, revision model.APIContractRevision, sourceEvidence *model.APIContractRevisionSourcePublication) (model.APIContract, model.APIContractRevision, error) {
	if sourceEvidence == nil {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	updated, err := scanAPIContract(tx.QueryRow(ctx, `UPDATE api_contracts SET name=$3,slug=$4,description=$5,visibility=$6,lifecycle=$7,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$8 RETURNING id::text,deployment_id::text,organisation_id::text,name,slug,description,contract_kind,visibility,lifecycle,revision,created_at,updated_at`,
		contract.DeploymentID, contract.ID, contract.Name, contract.Slug, contract.Description, contract.Visibility, contract.Lifecycle, expected))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			var exists bool
			if lookupErr := tx.QueryRow(ctx, `SELECT true FROM api_contracts WHERE deployment_id=$1 AND id=$2`, contract.DeploymentID, contract.ID).Scan(&exists); lookupErr == nil {
				return model.APIContract{}, model.APIContractRevision{}, ErrConflict
			}
		}
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	if err := snapshotAPIContractIdentity(updated, &revision); err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	created, err := scanAPIContractRevision(tx.QueryRow(ctx, `INSERT INTO api_contract_revisions(id,deployment_id,api_contract_id,api_contract_name,api_contract_slug,api_contract_description,api_contract_kind,api_contract_candidate_id,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,coalesce((SELECT max(revision)+1 FROM api_contract_revisions WHERE api_contract_id=$3),1),$9,$10,$11,$12,$13
		FROM api_contract_candidates candidate WHERE candidate.id=$8 AND candidate.api_contract_id=$3 AND candidate.deployment_id=$2 AND candidate.content_hash=$9 AND candidate.visibility=$10
		RETURNING id::text,deployment_id::text,api_contract_id::text,api_contract_name,api_contract_slug,api_contract_description,api_contract_kind,api_contract_candidate_id::text,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at,created_at`,
		revision.ID, revision.DeploymentID, revision.APIContractID, revision.APIContractName, revision.APIContractSlug,
		revision.APIContractDescription, revision.APIContractKind, revision.APIContractCandidateID, revision.ContentHash,
		revision.Visibility, revision.ReviewedBy, revision.ReviewedAt, revision.PublishedAt))
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	if sourceEvidence != nil {
		_, err = tx.Exec(ctx, `INSERT INTO api_contract_revision_source_publications(api_contract_revision_id,deployment_id,api_contract_candidate_id,source_publication_id,content_hash)
			VALUES($1,$2,$3,$4,$5)`, created.ID, sourceEvidence.DeploymentID, created.APIContractCandidateID, sourceEvidence.SourcePublicationID, sourceEvidence.ContentHash)
		if err != nil {
			return model.APIContract{}, model.APIContractRevision{}, databaseError(err)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE developer_asset_ingestion_runs run SET state='published',lease_owner='',lease_expires_at=NULL,heartbeat_at=NULL,finished_at=coalesce(finished_at,now())
		FROM api_contract_candidates candidate WHERE candidate.id=$1 AND run.id=candidate.ingestion_run_id AND run.deployment_id=$2
		AND run.asset_kind='contract' AND run.target_id=$3 AND run.state='review_ready' AND run.failed_count=0 AND run.skipped_count=0 AND run.quarantined_count=0`,
		revision.APIContractCandidateID, revision.DeploymentID, revision.APIContractID)
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	if err := bumpDeploymentCatalog(ctx, tx, contract.DeploymentID); err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	return updated, created, nil
}
