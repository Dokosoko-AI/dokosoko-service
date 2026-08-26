package store

import (
	"context"
	"errors"
	"sort"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const developerAssetIngestionRunSelect = `SELECT id::text,deployment_id::text,organisation_id::text,asset_kind,coalesce(target_id::text,''),target_key,coalesce(source_id::text,''),resolved_source_uri,resolved_source_revision,resolved_source_hash,state,attempt,pipeline_version,parser_version,normalizer_version,mapper_version,raw_manifest,raw_manifest_hash,diagnostics,discovered_count,acquired_count,failed_count,skipped_count,quarantined_count,lease_owner,lease_expires_at,heartbeat_at,error_code,error_message,queued_at,started_at,finished_at FROM developer_asset_ingestion_runs`

func scanDeveloperAssetIngestionRun(row pgx.Row) (model.DeveloperAssetIngestionRun, error) {
	var value model.DeveloperAssetIngestionRun
	err := row.Scan(
		&value.ID, &value.DeploymentID, &value.OrganisationID, &value.AssetKind, &value.TargetID, &value.TargetKey, &value.SourceID,
		&value.ResolvedSourceURI, &value.ResolvedSourceRevision, &value.ResolvedSourceHash, &value.State, &value.Attempt,
		&value.Versions.Pipeline, &value.Versions.Parser, &value.Versions.Normalizer, &value.Versions.Mapper,
		&value.RawManifest, &value.RawManifestHash, &value.Diagnostics, &value.DiscoveredCount, &value.AcquiredCount,
		&value.FailedCount, &value.SkippedCount, &value.QuarantinedCount, &value.LeaseOwner, &value.LeaseExpiresAt,
		&value.HeartbeatAt, &value.ErrorCode, &value.ErrorMessage, &value.QueuedAt, &value.StartedAt, &value.FinishedAt,
	)
	return value, databaseError(err)
}

func (p *Postgres) DeveloperAssetIngestionRuns(ctx context.Context, deploymentID string, kind model.DeveloperAssetKind, targetKey string) ([]model.DeveloperAssetIngestionRun, error) {
	query := developerAssetIngestionRunSelect + ` WHERE deployment_id=$1`
	args := []any{deploymentID}
	if kind != "" {
		args = append(args, kind)
		query += ` AND asset_kind=$` + postgresArgument(len(args))
	}
	if targetKey != "" {
		args = append(args, targetKey)
		query += ` AND target_key=$` + postgresArgument(len(args))
	}
	query += ` ORDER BY queued_at DESC`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.DeveloperAssetIngestionRun, 0)
	for rows.Next() {
		value, scanErr := scanDeveloperAssetIngestionRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func postgresArgument(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	return string(digits[value/10]) + string(digits[value%10])
}

func (p *Postgres) DeveloperAssetIngestionRun(ctx context.Context, deploymentID, id string) (model.DeveloperAssetIngestionRun, error) {
	return scanDeveloperAssetIngestionRun(p.pool.QueryRow(ctx, developerAssetIngestionRunSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateDeveloperAssetIngestionRun(ctx context.Context, value model.DeveloperAssetIngestionRun) (model.DeveloperAssetIngestionRun, error) {
	return scanDeveloperAssetIngestionRun(p.pool.QueryRow(ctx, `INSERT INTO developer_asset_ingestion_runs(
		id,deployment_id,organisation_id,asset_kind,target_id,target_key,source_id,resolved_source_uri,resolved_source_revision,resolved_source_hash,state,attempt,
		pipeline_version,parser_version,normalizer_version,mapper_version,raw_manifest,raw_manifest_hash,diagnostics,discovered_count,acquired_count,failed_count,
		skipped_count,quarantined_count,lease_owner,lease_expires_at,heartbeat_at,error_code,error_message,queued_at,started_at,finished_at
	) VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,nullif($7,'')::uuid,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32)
	RETURNING `+developerAssetIngestionRunSelect[len("SELECT "):len(developerAssetIngestionRunSelect)-len(" FROM developer_asset_ingestion_runs")],
		value.ID, value.DeploymentID, value.OrganisationID, value.AssetKind, value.TargetID, value.TargetKey, value.SourceID,
		value.ResolvedSourceURI, value.ResolvedSourceRevision, value.ResolvedSourceHash, value.State, value.Attempt,
		value.Versions.Pipeline, value.Versions.Parser, value.Versions.Normalizer, value.Versions.Mapper, value.RawManifest,
		value.RawManifestHash, value.Diagnostics, value.DiscoveredCount, value.AcquiredCount, value.FailedCount, value.SkippedCount,
		value.QuarantinedCount, value.LeaseOwner, value.LeaseExpiresAt, value.HeartbeatAt, value.ErrorCode, value.ErrorMessage,
		value.QueuedAt, value.StartedAt, value.FinishedAt))
}

func (p *Postgres) TransitionDeveloperAssetIngestionRun(ctx context.Context, value model.DeveloperAssetIngestionRun, expected model.DeveloperAssetIngestionState) (model.DeveloperAssetIngestionRun, error) {
	updated, err := scanDeveloperAssetIngestionRun(p.pool.QueryRow(ctx, `UPDATE developer_asset_ingestion_runs SET
		resolved_source_uri=$3,resolved_source_revision=$4,resolved_source_hash=$5,state=$6,attempt=$7,pipeline_version=$8,parser_version=$9,
		normalizer_version=$10,mapper_version=$11,raw_manifest=$12,raw_manifest_hash=$13,diagnostics=$14,discovered_count=$15,
		acquired_count=$16,failed_count=$17,skipped_count=$18,quarantined_count=$19,lease_owner=$20,lease_expires_at=$21,
		heartbeat_at=$22,error_code=$23,error_message=$24,started_at=$25,finished_at=$26
	WHERE deployment_id=$1 AND id=$2 AND state=$27 RETURNING `+developerAssetIngestionRunSelect[len("SELECT "):len(developerAssetIngestionRunSelect)-len(" FROM developer_asset_ingestion_runs")],
		value.DeploymentID, value.ID, value.ResolvedSourceURI, value.ResolvedSourceRevision, value.ResolvedSourceHash, value.State,
		value.Attempt, value.Versions.Pipeline, value.Versions.Parser, value.Versions.Normalizer, value.Versions.Mapper, value.RawManifest,
		value.RawManifestHash, value.Diagnostics, value.DiscoveredCount, value.AcquiredCount, value.FailedCount, value.SkippedCount,
		value.QuarantinedCount, value.LeaseOwner, value.LeaseExpiresAt, value.HeartbeatAt, value.ErrorCode, value.ErrorMessage,
		value.StartedAt, value.FinishedAt, expected))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.DeveloperAssetIngestionRun(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.DeveloperAssetIngestionRun{}, ErrConflict
		}
	}
	return updated, err
}

const developerAssetIngestionStageSelect = `SELECT id::text,ingestion_run_id::text,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at,created_at,updated_at FROM developer_asset_ingestion_stages`

func scanDeveloperAssetIngestionStage(row pgx.Row) (model.DeveloperAssetIngestionStage, error) {
	var value model.DeveloperAssetIngestionStage
	err := row.Scan(&value.ID, &value.IngestionRunID, &value.Name, &value.Attempt, &value.State, &value.InputHash, &value.OutputHash,
		&value.Checkpoint, &value.Diagnostics, &value.ErrorCode, &value.ErrorMessage, &value.StartedAt, &value.FinishedAt, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) DeveloperAssetIngestionStages(ctx context.Context, runID string) ([]model.DeveloperAssetIngestionStage, error) {
	rows, err := p.pool.Query(ctx, developerAssetIngestionStageSelect+` WHERE ingestion_run_id=$1 ORDER BY created_at,attempt`, runID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.DeveloperAssetIngestionStage, 0)
	for rows.Next() {
		value, scanErr := scanDeveloperAssetIngestionStage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SaveDeveloperAssetIngestionStage(ctx context.Context, value model.DeveloperAssetIngestionStage, expectedState string) (model.DeveloperAssetIngestionStage, error) {
	if expectedState == "" {
		return scanDeveloperAssetIngestionStage(p.pool.QueryRow(ctx, `INSERT INTO developer_asset_ingestion_stages(
			id,ingestion_run_id,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text,ingestion_run_id::text,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at,created_at,updated_at`,
			value.ID, value.IngestionRunID, value.Name, value.Attempt, value.State, value.InputHash, value.OutputHash,
			value.Checkpoint, value.Diagnostics, value.ErrorCode, value.ErrorMessage, value.StartedAt, value.FinishedAt))
	}
	updated, err := scanDeveloperAssetIngestionStage(p.pool.QueryRow(ctx, `UPDATE developer_asset_ingestion_stages SET state=$3,input_hash=$4,output_hash=$5,checkpoint=$6,diagnostics=$7,error_code=$8,error_message=$9,started_at=$10,finished_at=$11,updated_at=now()
		WHERE ingestion_run_id=$1 AND id=$2 AND state=$12 RETURNING id::text,ingestion_run_id::text,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at,created_at,updated_at`,
		value.IngestionRunID, value.ID, value.State, value.InputHash, value.OutputHash, value.Checkpoint, value.Diagnostics,
		value.ErrorCode, value.ErrorMessage, value.StartedAt, value.FinishedAt, expectedState))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		lookupErr := p.pool.QueryRow(ctx, `SELECT true FROM developer_asset_ingestion_stages WHERE ingestion_run_id=$1 AND id=$2`, value.IngestionRunID, value.ID).Scan(&exists)
		if lookupErr == nil {
			return model.DeveloperAssetIngestionStage{}, ErrConflict
		}
	}
	return updated, err
}

const documentationDocumentSelect = `SELECT id::text,deployment_id::text,ingestion_run_id::text,coalesce(legacy_knowledge_document_id::text,''),source_path,canonical_url,title,document_kind,language,media_type,normalized_markdown,content_hash,visibility,ordinal,metadata,created_at FROM documentation_documents`

func scanDocumentationDocument(row pgx.Row) (model.DocumentationDocument, error) {
	var value model.DocumentationDocument
	err := row.Scan(&value.ID, &value.DeploymentID, &value.IngestionRunID, &value.LegacyKnowledgeDocumentID, &value.SourcePath, &value.CanonicalURL,
		&value.Title, &value.Kind, &value.Language, &value.MediaType, &value.NormalizedMarkdown, &value.ContentHash,
		&value.Visibility, &value.Ordinal, &value.Metadata, &value.CreatedAt)
	return value, databaseError(err)
}

const documentationSectionSelect = `SELECT id::text,deployment_id::text,documentation_document_id::text,coalesce(parent_section_id::text,''),ordinal,heading_level,heading,anchor,breadcrumb,content_kind,normalized_text,code_language,token_estimate,source_start,source_end,content_hash,metadata,created_at FROM documentation_sections`

func scanDocumentationSection(row pgx.Row) (model.DocumentationSection, error) {
	var value model.DocumentationSection
	err := row.Scan(&value.ID, &value.DeploymentID, &value.DocumentationDocumentID, &value.ParentSectionID, &value.Ordinal,
		&value.HeadingLevel, &value.Heading, &value.Anchor, &value.Breadcrumb, &value.ContentKind, &value.NormalizedText,
		&value.CodeLanguage, &value.TokenEstimate, &value.SourceStart, &value.SourceEnd, &value.ContentHash, &value.Metadata, &value.CreatedAt)
	return value, databaseError(err)
}

const documentationMapSelect = `SELECT id::text,deployment_id::text,coalesce(ingestion_run_id::text,''),coalesce(documentation_collection_revision_id::text,''),map_version,structured_map,agent_markdown,content_hash,visibility,created_at FROM documentation_maps`

func scanDocumentationMap(row pgx.Row) (model.DocumentationMap, error) {
	var value model.DocumentationMap
	err := row.Scan(&value.ID, &value.DeploymentID, &value.IngestionRunID, &value.DocumentationCollectionRevisionID, &value.MapVersion,
		&value.Map, &value.AgentMarkdown, &value.ContentHash, &value.Visibility, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) DocumentationIngestionOutput(ctx context.Context, deploymentID, runID string) (DocumentationIngestionOutput, error) {
	var result DocumentationIngestionOutput
	rows, err := p.pool.Query(ctx, documentationDocumentSelect+` WHERE deployment_id=$1 AND ingestion_run_id=$2 ORDER BY ordinal`, deploymentID, runID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanDocumentationDocument(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Documents = append(result.Documents, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Documents) == 0 {
		return result, ErrNotFound
	}
	rows, err = p.pool.Query(ctx, documentationSectionSelect+` WHERE deployment_id=$1 AND documentation_document_id IN (SELECT id FROM documentation_documents WHERE ingestion_run_id=$2) ORDER BY documentation_document_id,ordinal`, deploymentID, runID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanDocumentationSection(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Sections = append(result.Sections, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	value, mapErr := scanDocumentationMap(p.pool.QueryRow(ctx, documentationMapSelect+` WHERE deployment_id=$1 AND ingestion_run_id=$2 ORDER BY created_at DESC LIMIT 1`, deploymentID, runID))
	if mapErr == nil {
		result.Map = &value
	} else if !errors.Is(mapErr, ErrNotFound) {
		return result, mapErr
	}
	return result, nil
}

func insertDocumentationMapTx(ctx context.Context, tx pgx.Tx, value model.DocumentationMap) error {
	_, err := tx.Exec(ctx, `INSERT INTO documentation_maps(id,deployment_id,ingestion_run_id,documentation_collection_revision_id,map_version,structured_map,agent_markdown,content_hash,visibility)
		VALUES($1,$2,nullif($3,'')::uuid,nullif($4,'')::uuid,$5,$6,$7,$8,$9)`, value.ID, value.DeploymentID, value.IngestionRunID,
		value.DocumentationCollectionRevisionID, value.MapVersion, value.Map, value.AgentMarkdown, value.ContentHash, value.Visibility)
	return databaseError(err)
}

func (p *Postgres) SaveDocumentationIngestionOutput(ctx context.Context, deploymentID string, value DocumentationIngestionOutput) error {
	if len(value.Documents) == 0 {
		return ErrConflict
	}
	runID := value.Documents[0].IngestionRunID
	documentIDs := make(map[string]bool, len(value.Documents))
	for _, document := range value.Documents {
		if document.DeploymentID != deploymentID || document.IngestionRunID != runID || documentIDs[document.ID] {
			return ErrConflict
		}
		documentIDs[document.ID] = true
	}
	sectionIDs := make(map[string]bool, len(value.Sections))
	for _, section := range value.Sections {
		if section.DeploymentID != deploymentID || !documentIDs[section.DocumentationDocumentID] || sectionIDs[section.ID] {
			return ErrConflict
		}
		sectionIDs[section.ID] = true
	}
	for _, section := range value.Sections {
		if section.ParentSectionID != "" && !sectionIDs[section.ParentSectionID] {
			return ErrConflict
		}
	}
	if value.Map != nil && (value.Map.DeploymentID != deploymentID || value.Map.IngestionRunID != runID || value.Map.DocumentationCollectionRevisionID != "") {
		return ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, document := range value.Documents {
		if document.DeploymentID != deploymentID {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `INSERT INTO documentation_documents(id,deployment_id,ingestion_run_id,legacy_knowledge_document_id,source_path,canonical_url,title,document_kind,language,media_type,normalized_markdown,content_hash,visibility,ordinal,metadata)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, document.ID, document.DeploymentID,
			document.IngestionRunID, document.LegacyKnowledgeDocumentID, document.SourcePath, document.CanonicalURL, document.Title,
			document.Kind, document.Language, document.MediaType, document.NormalizedMarkdown, document.ContentHash, document.Visibility,
			document.Ordinal, document.Metadata)
		if err != nil {
			return databaseError(err)
		}
	}
	for _, section := range value.Sections {
		_, err = tx.Exec(ctx, `INSERT INTO documentation_sections(id,deployment_id,documentation_document_id,parent_section_id,ordinal,heading_level,heading,anchor,breadcrumb,content_kind,normalized_text,code_language,token_estimate,source_start,source_end,content_hash,metadata)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, section.ID, section.DeploymentID,
			section.DocumentationDocumentID, section.ParentSectionID, section.Ordinal, section.HeadingLevel, section.Heading, section.Anchor,
			section.Breadcrumb, section.ContentKind, section.NormalizedText, section.CodeLanguage, section.TokenEstimate, section.SourceStart,
			section.SourceEnd, section.ContentHash, section.Metadata)
		if err != nil {
			return databaseError(err)
		}
	}
	if value.Map != nil {
		if err := insertDocumentationMapTx(ctx, tx, *value.Map); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func postgresDocumentationCandidateFilter(query DocumentationCandidateQuery) (string, []any) {
	sql := ` FROM documentation_documents document JOIN developer_asset_ingestion_runs run ON run.id=document.ingestion_run_id WHERE document.deployment_id=$1`
	args := []any{query.DeploymentID}
	if query.IngestionRunID != "" {
		args = append(args, query.IngestionRunID)
		sql += ` AND document.ingestion_run_id=$` + postgresArgument(len(args))
	}
	if query.SourceID != "" {
		args = append(args, query.SourceID)
		sql += ` AND run.source_id=$` + postgresArgument(len(args))
	}
	if query.SourcePublicationID != "" {
		args = append(args, query.SourcePublicationID)
		sql += ` AND EXISTS (SELECT 1 FROM source_publication_document_selections selection WHERE selection.source_publication_id=$` + postgresArgument(len(args)) + ` AND selection.documentation_document_id=document.id)`
	}
	if query.QueryText != "" {
		args = append(args, "%"+query.QueryText+"%")
		placeholder := `$` + postgresArgument(len(args))
		sql += ` AND (document.title ILIKE ` + placeholder + ` OR document.source_path ILIKE ` + placeholder + ` OR document.normalized_markdown ILIKE ` + placeholder + ` OR EXISTS (SELECT 1 FROM documentation_sections section WHERE section.documentation_document_id=document.id AND (section.heading ILIKE ` + placeholder + ` OR section.normalized_text ILIKE ` + placeholder + `)))`
	}
	return sql, args
}

func (p *Postgres) DocumentationCandidateDocuments(ctx context.Context, query DocumentationCandidateQuery) (DocumentationCandidatePage, error) {
	limit := boundedDeveloperAssetResultLimit(query.Limit)
	if limit == 0 {
		return DocumentationCandidatePage{Items: []DocumentationCandidateRecord{}}, nil
	}
	offset := max(query.Offset, 0)
	filter, args := postgresDocumentationCandidateFilter(query)
	var total int
	if err := p.pool.QueryRow(ctx, `SELECT count(*)`+filter, args...).Scan(&total); err != nil {
		return DocumentationCandidatePage{}, databaseError(err)
	}
	if offset >= total {
		return DocumentationCandidatePage{Items: []DocumentationCandidateRecord{}, Total: total}, nil
	}
	args = append(args, limit)
	limitArgument := postgresArgument(len(args))
	args = append(args, offset)
	sql := `SELECT document.id::text` + filter + ` ORDER BY run.queued_at DESC,run.id DESC,document.ordinal,document.id LIMIT $` + limitArgument + ` OFFSET $` + postgresArgument(len(args))
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return DocumentationCandidatePage{}, databaseError(err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return DocumentationCandidatePage{}, databaseError(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return DocumentationCandidatePage{}, err
	}
	result := make([]DocumentationCandidateRecord, 0, len(ids))
	for _, id := range ids {
		value, lookupErr := p.DocumentationCandidateDocument(ctx, query.DeploymentID, id)
		if lookupErr != nil {
			return DocumentationCandidatePage{}, lookupErr
		}
		result = append(result, value)
	}
	return DocumentationCandidatePage{Items: result, Total: total, HasMore: offset+len(result) < total}, nil
}

func (p *Postgres) DocumentationCandidateDocument(ctx context.Context, deploymentID, documentID string) (DocumentationCandidateRecord, error) {
	result := DocumentationCandidateRecord{
		Sections:                    []model.DocumentationSection{},
		SourcePublicationSelections: []model.SourcePublicationDocumentSelection{},
	}
	document, err := scanDocumentationDocument(p.pool.QueryRow(ctx, documentationDocumentSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, documentID))
	if err != nil {
		return result, err
	}
	result.Document = document
	run, err := p.DeveloperAssetIngestionRun(ctx, deploymentID, document.IngestionRunID)
	if err != nil {
		return result, err
	}
	result.Run = run
	mapValue, mapErr := scanDocumentationMap(p.pool.QueryRow(ctx, documentationMapSelect+` WHERE deployment_id=$1 AND ingestion_run_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, deploymentID, document.IngestionRunID))
	if mapErr == nil {
		result.DocumentationMap = &mapValue
	} else if !errors.Is(mapErr, ErrNotFound) {
		return result, mapErr
	}
	rows, err := p.pool.Query(ctx, documentationSectionSelect+` WHERE deployment_id=$1 AND documentation_document_id=$2 ORDER BY ordinal`, deploymentID, documentID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		section, scanErr := scanDocumentationSection(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Sections = append(result.Sections, section)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return result, rowsErr
	}
	rows, err = p.pool.Query(ctx, sourcePublicationDocumentSelectionSelect+`
		WHERE deployment_id=$1 AND documentation_document_id=$2
		ORDER BY reviewed_at DESC,created_at DESC,source_publication_id DESC`, deploymentID, documentID)
	if err != nil {
		return result, databaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		selection, scanErr := scanSourcePublicationDocumentSelection(rows)
		if scanErr != nil {
			return result, scanErr
		}
		result.SourcePublicationSelections = append(result.SourcePublicationSelections, selection)
	}
	return result, rows.Err()
}

func (p *Postgres) DocumentationCandidateSection(ctx context.Context, deploymentID, sectionID string) (model.DocumentationSection, DocumentationCandidateRecord, error) {
	section, err := scanDocumentationSection(p.pool.QueryRow(ctx, documentationSectionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, sectionID))
	if err != nil {
		return model.DocumentationSection{}, DocumentationCandidateRecord{}, err
	}
	record, err := p.DocumentationCandidateDocument(ctx, deploymentID, section.DocumentationDocumentID)
	return section, record, err
}

const sourcePublicationDocumentSelectionSelect = `SELECT source_publication_id::text,deployment_id::text,documentation_document_id::text,decision,reason,ordinal,content_hash,reviewed_by,reviewed_at,created_at FROM source_publication_document_selections`

func scanSourcePublicationDocumentSelection(row pgx.Row) (model.SourcePublicationDocumentSelection, error) {
	var value model.SourcePublicationDocumentSelection
	err := row.Scan(&value.SourcePublicationID, &value.DeploymentID, &value.DocumentationDocumentID, &value.Decision, &value.Reason,
		&value.Ordinal, &value.ContentHash, &value.ReviewedBy, &value.ReviewedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func sortSourcePublicationSelectionsNewestFirst(values []model.SourcePublicationDocumentSelection) {
	sort.SliceStable(values, func(i, j int) bool {
		if !values[i].ReviewedAt.Equal(values[j].ReviewedAt) {
			return values[i].ReviewedAt.After(values[j].ReviewedAt)
		}
		if !values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].CreatedAt.After(values[j].CreatedAt)
		}
		return values[i].SourcePublicationID > values[j].SourcePublicationID
	})
}

func (p *Postgres) SourcePublicationDocumentationReview(ctx context.Context, deploymentID, publicationID string) (SourcePublicationDocumentationReview, error) {
	var result SourcePublicationDocumentationReview
	rows, err := p.pool.Query(ctx, sourcePublicationDocumentSelectionSelect+` WHERE deployment_id=$1 AND source_publication_id=$2 ORDER BY ordinal NULLS LAST,documentation_document_id`, deploymentID, publicationID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		value, scanErr := scanSourcePublicationDocumentSelection(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Selections = append(result.Selections, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Selections) == 0 {
		return result, ErrNotFound
	}
	var link model.SourcePublicationDocumentationMap
	err = p.pool.QueryRow(ctx, `SELECT source_publication_id::text,deployment_id::text,documentation_map_id::text,content_hash,created_at FROM source_publication_documentation_maps WHERE deployment_id=$1 AND source_publication_id=$2`, deploymentID, publicationID).
		Scan(&link.SourcePublicationID, &link.DeploymentID, &link.DocumentationMapID, &link.ContentHash, &link.CreatedAt)
	if err == nil {
		result.MapLink = &link
	} else if databaseError(err) != ErrNotFound {
		return result, databaseError(err)
	}
	return result, nil
}

func insertSourcePublicationDocumentationReviewTx(ctx context.Context, tx pgx.Tx, value SourcePublicationDocumentationReview) error {
	for _, selection := range value.Selections {
		_, err := tx.Exec(ctx, `INSERT INTO source_publication_document_selections(source_publication_id,deployment_id,documentation_document_id,decision,reason,ordinal,content_hash,reviewed_by,reviewed_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, selection.SourcePublicationID, selection.DeploymentID, selection.DocumentationDocumentID,
			selection.Decision, selection.Reason, selection.Ordinal, selection.ContentHash, selection.ReviewedBy, selection.ReviewedAt)
		if err != nil {
			return databaseError(err)
		}
	}
	if value.MapLink != nil {
		_, err := tx.Exec(ctx, `INSERT INTO source_publication_documentation_maps(source_publication_id,deployment_id,documentation_map_id,content_hash) VALUES($1,$2,$3,$4)`,
			value.MapLink.SourcePublicationID, value.MapLink.DeploymentID, value.MapLink.DocumentationMapID, value.MapLink.ContentHash)
		if err != nil {
			return databaseError(err)
		}
	}
	return nil
}

func (p *Postgres) SaveSourcePublicationDocumentationReview(ctx context.Context, deploymentID string, value SourcePublicationDocumentationReview) error {
	if len(value.Selections) == 0 {
		return ErrConflict
	}
	publicationID := value.Selections[0].SourcePublicationID
	for _, selection := range value.Selections {
		if selection.DeploymentID != deploymentID || selection.SourcePublicationID != publicationID {
			return ErrConflict
		}
	}
	if value.MapLink != nil && (value.MapLink.DeploymentID != deploymentID || value.MapLink.SourcePublicationID != publicationID) {
		return ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := insertSourcePublicationDocumentationReviewTx(ctx, tx, value); err != nil {
		return err
	}
	var runID, sourceID string
	var runCount int
	err = tx.QueryRow(ctx, `SELECT min(document.ingestion_run_id)::text,count(DISTINCT document.ingestion_run_id),publication.source_id::text
		FROM source_publication_document_selections selection
		JOIN documentation_documents document ON document.id=selection.documentation_document_id
		JOIN source_publications publication ON publication.id=selection.source_publication_id
		WHERE selection.source_publication_id=$1 AND selection.deployment_id=$2 GROUP BY publication.source_id`, publicationID, deploymentID).
		Scan(&runID, &runCount, &sourceID)
	if err != nil {
		return databaseError(err)
	}
	if runCount != 1 {
		return ErrConflict
	}
	var candidateDocumentCount, candidateMapCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM documentation_documents WHERE deployment_id=$1 AND ingestion_run_id=$2`, deploymentID, runID).Scan(&candidateDocumentCount); err != nil {
		return databaseError(err)
	}
	if candidateDocumentCount != len(value.Selections) {
		return ErrConflict
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM documentation_maps WHERE deployment_id=$1 AND ingestion_run_id=$2`, deploymentID, runID).Scan(&candidateMapCount); err != nil {
		return databaseError(err)
	}
	if (candidateMapCount == 0) != (value.MapLink == nil) {
		return ErrConflict
	}
	transition, err := tx.Exec(ctx, `UPDATE developer_asset_ingestion_runs SET state='published',lease_owner='',lease_expires_at=NULL,heartbeat_at=NULL,finished_at=coalesce(finished_at,now())
		WHERE id=$1 AND deployment_id=$2 AND asset_kind='documentation' AND source_id=$3 AND target_id=$3 AND state='review_ready'
		AND failed_count=0 AND skipped_count=0 AND quarantined_count=0`, runID, deploymentID, sourceID)
	if err != nil {
		return databaseError(err)
	}
	if transition.RowsAffected() == 0 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}
