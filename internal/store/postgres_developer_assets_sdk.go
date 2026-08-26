package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const sdkPackageSelect = `SELECT id::text,deployment_id::text,organisation_id::text,ecosystem,canonical_coordinate,display_coordinate,name,description,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_sdk_package_id::text,''),deprecation_message,revision,created_at,updated_at FROM sdk_packages`

func scanSDKPackage(row pgx.Row) (model.SDKPackage, error) {
	var value model.SDKPackage
	err := row.Scan(&value.ID, &value.DeploymentID, &value.OrganisationID, &value.Ecosystem, &value.CanonicalCoordinate,
		&value.DisplayCoordinate, &value.Name, &value.Description, &value.RegistryURL, &value.SourceURL, &value.Language,
		&value.Platform, &value.Visibility, &value.Lifecycle, &value.ReplacementSDKPackageID, &value.DeprecationMessage,
		&value.Revision, &value.CreatedAt, &value.UpdatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKPackages(ctx context.Context, deploymentID string) ([]model.SDKPackage, error) {
	rows, err := p.pool.Query(ctx, sdkPackageSelect+` WHERE deployment_id=$1 ORDER BY ecosystem,canonical_coordinate`, deploymentID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKPackage, 0)
	for rows.Next() {
		value, scanErr := scanSDKPackage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SDKPackage(ctx context.Context, deploymentID, id string) (model.SDKPackage, error) {
	return scanSDKPackage(p.pool.QueryRow(ctx, sdkPackageSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) SaveSDKPackage(ctx context.Context, value model.SDKPackage, expected int64) (model.SDKPackage, error) {
	if expected == 0 {
		return scanSDKPackage(p.pool.QueryRow(ctx, `INSERT INTO sdk_packages(id,deployment_id,organisation_id,ecosystem,canonical_coordinate,display_coordinate,name,description,registry_url,source_url,language,platform,visibility,lifecycle,replacement_sdk_package_id,deprecation_message)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,nullif($15,'')::uuid,$16)
			RETURNING id::text,deployment_id::text,organisation_id::text,ecosystem,canonical_coordinate,display_coordinate,name,description,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_sdk_package_id::text,''),deprecation_message,revision,created_at,updated_at`,
			value.ID, value.DeploymentID, value.OrganisationID, value.Ecosystem, value.CanonicalCoordinate, value.DisplayCoordinate,
			value.Name, value.Description, value.RegistryURL, value.SourceURL, value.Language, value.Platform, value.Visibility,
			value.Lifecycle, value.ReplacementSDKPackageID, value.DeprecationMessage))
	}
	updated, err := scanSDKPackage(p.pool.QueryRow(ctx, `UPDATE sdk_packages SET name=$3,description=$4,registry_url=$5,source_url=$6,language=$7,platform=$8,visibility=$9,lifecycle=$10,replacement_sdk_package_id=nullif($11,'')::uuid,deprecation_message=$12,revision=revision+1,updated_at=now()
		WHERE deployment_id=$1 AND id=$2 AND revision=$13
		  AND ecosystem=$14 AND canonical_coordinate=$15 AND display_coordinate=$16
		RETURNING id::text,deployment_id::text,organisation_id::text,ecosystem,canonical_coordinate,display_coordinate,name,description,registry_url,source_url,language,platform,visibility,lifecycle,coalesce(replacement_sdk_package_id::text,''),deprecation_message,revision,created_at,updated_at`,
		value.DeploymentID, value.ID, value.Name, value.Description, value.RegistryURL, value.SourceURL,
		value.Language, value.Platform, value.Visibility, value.Lifecycle, value.ReplacementSDKPackageID, value.DeprecationMessage,
		expected, value.Ecosystem, value.CanonicalCoordinate, value.DisplayCoordinate))
	if errors.Is(err, ErrNotFound) {
		if _, lookupErr := p.SDKPackage(ctx, value.DeploymentID, value.ID); lookupErr == nil {
			return model.SDKPackage{}, ErrConflict
		}
	}
	return updated, err
}

const sdkReleaseSelect = `SELECT id::text,deployment_id::text,sdk_package_id::text,exact_version,install_command,documentation_url,source_url,resolved_source_revision,upstream_digest,identity_assurance,visibility,lifecycle,release_hash,published_at,created_at FROM sdk_releases`

func scanSDKRelease(row pgx.Row) (model.SDKRelease, error) {
	var value model.SDKRelease
	err := row.Scan(&value.ID, &value.DeploymentID, &value.SDKPackageID, &value.ExactVersion, &value.InstallCommand,
		&value.DocumentationURL, &value.SourceURL, &value.ResolvedSourceRevision, &value.UpstreamDigest, &value.IdentityAssurance,
		&value.Visibility, &value.Lifecycle, &value.ReleaseHash, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKReleases(ctx context.Context, deploymentID, packageID string) ([]model.SDKRelease, error) {
	rows, err := p.pool.Query(ctx, sdkReleaseSelect+` WHERE deployment_id=$1 AND sdk_package_id=$2 ORDER BY created_at DESC,exact_version`, deploymentID, packageID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKRelease, 0)
	for rows.Next() {
		value, scanErr := scanSDKRelease(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SDKRelease(ctx context.Context, deploymentID, id string) (model.SDKRelease, error) {
	return scanSDKRelease(p.pool.QueryRow(ctx, sdkReleaseSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateSDKRelease(ctx context.Context, value model.SDKRelease) (model.SDKRelease, error) {
	return scanSDKRelease(p.pool.QueryRow(ctx, `INSERT INTO sdk_releases(id,deployment_id,sdk_package_id,exact_version,install_command,documentation_url,source_url,resolved_source_revision,upstream_digest,identity_assurance,visibility,lifecycle,release_hash,published_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id::text,deployment_id::text,sdk_package_id::text,exact_version,install_command,documentation_url,source_url,resolved_source_revision,upstream_digest,identity_assurance,visibility,lifecycle,release_hash,published_at,created_at`,
		value.ID, value.DeploymentID, value.SDKPackageID, value.ExactVersion, value.InstallCommand, value.DocumentationURL,
		value.SourceURL, value.ResolvedSourceRevision, value.UpstreamDigest, value.IdentityAssurance, value.Visibility,
		value.Lifecycle, value.ReleaseHash, value.PublishedAt))
}

func (p *Postgres) SDKReleaseLifecycleEvents(ctx context.Context, deploymentID, releaseID string) ([]model.SDKReleaseLifecycleEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,sdk_release_id::text,lifecycle,reason,observed_source_uri,observed_at,recorded_by,created_at FROM sdk_release_lifecycle_events WHERE deployment_id=$1 AND sdk_release_id=$2 ORDER BY observed_at DESC`, deploymentID, releaseID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKReleaseLifecycleEvent, 0)
	for rows.Next() {
		var value model.SDKReleaseLifecycleEvent
		if err := rows.Scan(&value.ID, &value.SDKReleaseID, &value.Lifecycle, &value.Reason, &value.ObservedSourceURI,
			&value.ObservedAt, &value.RecordedBy, &value.CreatedAt); err != nil {
			return nil, databaseError(err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AppendSDKReleaseLifecycleEvent(ctx context.Context, deploymentID string, mutation SDKReleaseLifecycleMutation) (model.SDKReleaseLifecycleEvent, error) {
	prior, current, outcome, err := prepareSDKReleaseLifecycleMutation(deploymentID, mutation)
	if err != nil {
		return model.SDKReleaseLifecycleEvent{}, err
	}
	value, audit := mutation.Event, mutation.Audit
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SDKReleaseLifecycleEvent{}, databaseError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existing model.SDKReleaseLifecycleEvent
	lookupErr := tx.QueryRow(ctx, `SELECT id::text,sdk_release_id::text,lifecycle,reason,observed_source_uri,observed_at,recorded_by,created_at
		FROM sdk_release_lifecycle_events WHERE deployment_id=$1 AND id=$2`, deploymentID, value.ID).
		Scan(&existing.ID, &existing.SDKReleaseID, &existing.Lifecycle, &existing.Reason, &existing.ObservedSourceURI,
			&existing.ObservedAt, &existing.RecordedBy, &existing.CreatedAt)
	if lookupErr == nil {
		if existing.SDKReleaseID != value.SDKReleaseID || existing.Lifecycle != value.Lifecycle || existing.Reason != value.Reason ||
			existing.ObservedSourceURI != value.ObservedSourceURI || !existing.ObservedAt.Equal(value.ObservedAt) || existing.RecordedBy != value.RecordedBy {
			return model.SDKReleaseLifecycleEvent{}, ErrConflict
		}
		var auditMatches bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_events WHERE event_key=$1 AND product_id=$2 AND action=$3 AND target_type=$4 AND target_id=$5 AND current->>'sdk_release_lifecycle_event_id'=$6)`,
			audit.ID, deploymentID, audit.Action, audit.TargetType, audit.TargetID, value.ID).Scan(&auditMatches); err != nil {
			return model.SDKReleaseLifecycleEvent{}, databaseError(err)
		}
		if !auditMatches {
			return model.SDKReleaseLifecycleEvent{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return model.SDKReleaseLifecycleEvent{}, databaseError(lookupErr)
	}
	var releaseExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sdk_releases WHERE deployment_id=$1 AND id=$2)`, deploymentID, value.SDKReleaseID).Scan(&releaseExists); err != nil {
		return model.SDKReleaseLifecycleEvent{}, databaseError(err)
	}
	if !releaseExists {
		return model.SDKReleaseLifecycleEvent{}, ErrNotFound
	}
	if err := tx.QueryRow(ctx, `INSERT INTO sdk_release_lifecycle_events(id,deployment_id,sdk_release_id,lifecycle,reason,observed_source_uri,observed_at,recorded_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,sdk_release_id::text,lifecycle,reason,observed_source_uri,observed_at,recorded_by,created_at`,
		value.ID, deploymentID, value.SDKReleaseID, value.Lifecycle, value.Reason, value.ObservedSourceURI, value.ObservedAt, value.RecordedBy).
		Scan(&value.ID, &value.SDKReleaseID, &value.Lifecycle, &value.Reason, &value.ObservedSourceURI, &value.ObservedAt, &value.RecordedBy, &value.CreatedAt); err != nil {
		return model.SDKReleaseLifecycleEvent{}, databaseError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(event_key, organisation_id, product_id, actor_id, actor_kind, action, target_type, target_id, prior, current, request_id, outcome, created_at)
		VALUES ($1, nullif($2, '')::uuid, nullif($3, '')::uuid, $4, 'root', $5, $6, $7, $8, $9, $10, $11, $12)`,
		audit.ID, audit.OrganisationID, audit.ProductID, audit.ActorID, audit.Action, audit.TargetType, audit.TargetID,
		prior, current, audit.RequestID, outcome, audit.CreatedAt); err != nil {
		return model.SDKReleaseLifecycleEvent{}, databaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SDKReleaseLifecycleEvent{}, databaseError(err)
	}
	return value, nil
}

const sdkContentCandidateSelect = `SELECT id::text,deployment_id::text,sdk_release_id::text,ingestion_run_id::text,pipeline_version,parser_version,normalizer_version,mapper_version,map_version,source_manifest,content_hash,visibility,diagnostics,created_at FROM sdk_content_candidates`

func scanSDKContentCandidate(row pgx.Row) (model.SDKContentCandidate, error) {
	var value model.SDKContentCandidate
	err := row.Scan(&value.ID, &value.DeploymentID, &value.SDKReleaseID, &value.IngestionRunID, &value.Versions.Pipeline,
		&value.Versions.Parser, &value.Versions.Normalizer, &value.Versions.Mapper, &value.MapVersion, &value.SourceManifest,
		&value.ContentHash, &value.Visibility, &value.Diagnostics, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKContentCandidates(ctx context.Context, deploymentID, releaseID string) ([]model.SDKContentCandidate, error) {
	rows, err := p.pool.Query(ctx, sdkContentCandidateSelect+` WHERE deployment_id=$1 AND sdk_release_id=$2 ORDER BY created_at DESC`, deploymentID, releaseID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKContentCandidate, 0)
	for rows.Next() {
		value, scanErr := scanSDKContentCandidate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanSDKPublicationFile(row pgx.Row) (model.SDKPublicationFile, error) {
	var value model.SDKPublicationFile
	err := row.Scan(&value.ID, &value.SDKContentCandidateID, &value.RawBlobID, &value.SourcePath, &value.Role, &value.MediaType,
		&value.Language, &value.SuggestedDisposition, &value.ExclusionReason, &value.NormalizedContent, &value.ContentHash,
		&value.ByteSize, &value.Metadata, &value.Ordinal)
	return value, databaseError(err)
}

func scanSDKSection(row pgx.Row) (model.SDKSection, error) {
	var value model.SDKSection
	err := row.Scan(&value.ID, &value.SDKContentCandidateID, &value.SDKPublicationFileID, &value.ParentSectionID, &value.Ordinal,
		&value.Heading, &value.Anchor, &value.Breadcrumb, &value.ContentKind, &value.NormalizedText, &value.CodeLanguage,
		&value.TokenEstimate, &value.SourceStart, &value.SourceEnd, &value.ContentHash, &value.Metadata)
	return value, databaseError(err)
}

func scanSDKSymbol(row pgx.Row) (model.SDKSymbol, error) {
	var value model.SDKSymbol
	err := row.Scan(&value.ID, &value.SDKContentCandidateID, &value.SDKPublicationFileID, &value.SDKSectionID, &value.Language,
		&value.Kind, &value.QualifiedName, &value.DisplayName, &value.Signature, &value.Documentation, &value.Identifiers,
		&value.SourceStart, &value.SourceEnd, &value.ContentHash, &value.Metadata)
	return value, databaseError(err)
}

func scanSDKCodeSample(row pgx.Row) (model.SDKCodeSample, error) {
	var value model.SDKCodeSample
	err := row.Scan(&value.ID, &value.DeploymentID, &value.SDKContentCandidateID, &value.SDKPublicationFileID, &value.SDKSectionID,
		&value.Language, &value.Title, &value.Intent, &value.Code, &value.Imports, &value.Prerequisites, &value.Origin,
		&value.SourceURI, &value.SourceRevision, &value.SourcePath, &value.SourceStart, &value.SourceEnd, &value.Attribution,
		&value.LicenseExpression, &value.ValidationStatus, &value.ValidationEvidence, &value.Visibility, &value.ContentHash, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKContentCandidate(ctx context.Context, deploymentID, id string) (SDKContentCandidateRecord, error) {
	var result SDKContentCandidateRecord
	value, err := scanSDKContentCandidate(p.pool.QueryRow(ctx, sdkContentCandidateSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Candidate = value
	rows, err := p.pool.Query(ctx, `SELECT id::text,sdk_content_candidate_id::text,coalesce(raw_blob_id::text,''),source_path,file_role,media_type,language,suggested_disposition,exclusion_reason,normalized_content,content_hash,byte_size,metadata,ordinal FROM sdk_publication_files WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 ORDER BY ordinal`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		item, scanErr := scanSDKPublicationFile(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Files = append(result.Files, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,sdk_content_candidate_id::text,sdk_publication_file_id::text,coalesce(parent_section_id::text,''),ordinal,heading,anchor,breadcrumb,content_kind,normalized_text,code_language,token_estimate,source_start,source_end,content_hash,metadata FROM sdk_sections WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 ORDER BY ordinal`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		item, scanErr := scanSDKSection(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Sections = append(result.Sections, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,sdk_content_candidate_id::text,coalesce(sdk_publication_file_id::text,''),coalesce(sdk_section_id::text,''),language,symbol_kind,qualified_name,display_name,signature,documentation,identifiers,source_start,source_end,content_hash,metadata FROM sdk_symbols WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 ORDER BY language,qualified_name`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		item, scanErr := scanSDKSymbol(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Symbols = append(result.Symbols, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,deployment_id::text,sdk_content_candidate_id::text,coalesce(sdk_publication_file_id::text,''),coalesce(sdk_section_id::text,''),language,title,intent,code,imports,prerequisites,origin,source_uri,source_revision,source_path,source_start,source_end,attribution,license_expression,validation_status,validation_evidence,visibility,content_hash,created_at FROM sdk_code_samples WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 ORDER BY title,id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		item, scanErr := scanSDKCodeSample(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Samples = append(result.Samples, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	var mapValue model.SDKMap
	err = p.pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,sdk_content_candidate_id::text,map_version,structured_map,agent_markdown,content_hash,created_at FROM sdk_maps WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 AND map_version=$3 LIMIT 1`, deploymentID, id, result.Candidate.MapVersion).
		Scan(&mapValue.ID, &mapValue.DeploymentID, &mapValue.SDKContentCandidateID, &mapValue.MapVersion, &mapValue.Map, &mapValue.AgentMarkdown, &mapValue.ContentHash, &mapValue.CreatedAt)
	if err == nil {
		result.Map = &mapValue
	} else if databaseError(err) != ErrNotFound {
		return result, databaseError(err)
	}
	rows, err = p.pool.Query(ctx, `SELECT id::text,sdk_code_sample_id::text,sdk_content_candidate_id::text,deployment_id::text,integration_id::text,coalesce(api_contract_revision_id::text,''),coalesce(api_contract_candidate_id::text,''),coalesce(api_contract_operation_id::text,''),coalesce(api_sdk_binding_id::text,''),reference_kind,created_at FROM sdk_sample_api_references WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 ORDER BY created_at,id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.SDKSampleAPIReference
		if err := rows.Scan(&item.ID, &item.SDKCodeSampleID, &item.SDKContentCandidateID, &item.DeploymentID, &item.APIID,
			&item.APIContractRevisionID, &item.APIContractCandidateID, &item.APIContractOperationID, &item.APISDKBindingID,
			&item.ReferenceKind, &item.CreatedAt); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.SampleRefs = append(result.SampleRefs, item)
	}
	return result, rows.Err()
}

func createSDKContentCandidateTx(ctx context.Context, tx pgx.Tx, value SDKContentCandidateRecord) (model.SDKContentCandidate, error) {
	if err := ValidateSDKContentCandidateGraph(value); err != nil {
		return model.SDKContentCandidate{}, ErrConflict
	}
	candidate := value.Candidate
	created, err := scanSDKContentCandidate(tx.QueryRow(ctx, `INSERT INTO sdk_content_candidates(id,deployment_id,sdk_release_id,ingestion_run_id,pipeline_version,parser_version,normalizer_version,mapper_version,map_version,source_manifest,content_hash,visibility,diagnostics)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text,deployment_id::text,sdk_release_id::text,ingestion_run_id::text,pipeline_version,parser_version,normalizer_version,mapper_version,map_version,source_manifest,content_hash,visibility,diagnostics,created_at`,
		candidate.ID, candidate.DeploymentID, candidate.SDKReleaseID, candidate.IngestionRunID, candidate.Versions.Pipeline,
		candidate.Versions.Parser, candidate.Versions.Normalizer, candidate.Versions.Mapper, candidate.MapVersion,
		candidate.SourceManifest, candidate.ContentHash, candidate.Visibility, candidate.Diagnostics))
	if err != nil {
		return model.SDKContentCandidate{}, err
	}
	for _, item := range value.Files {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_publication_files(id,deployment_id,sdk_content_candidate_id,raw_blob_id,source_path,file_role,media_type,language,suggested_disposition,exclusion_reason,normalized_content,content_hash,byte_size,metadata,ordinal)
			VALUES($1,$2,$3,nullif($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, item.ID, candidate.DeploymentID,
			item.SDKContentCandidateID, item.RawBlobID, item.SourcePath, item.Role, item.MediaType, item.Language,
			item.SuggestedDisposition, item.ExclusionReason, item.NormalizedContent, item.ContentHash, item.ByteSize, item.Metadata, item.Ordinal)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.Sections {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_sections(id,deployment_id,sdk_content_candidate_id,sdk_publication_file_id,parent_section_id,ordinal,heading,anchor,breadcrumb,content_kind,normalized_text,code_language,token_estimate,source_start,source_end,content_hash,metadata)
			VALUES($1,$2,$3,$4,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, item.ID, candidate.DeploymentID,
			item.SDKContentCandidateID, item.SDKPublicationFileID, item.ParentSectionID, item.Ordinal, item.Heading, item.Anchor,
			item.Breadcrumb, item.ContentKind, item.NormalizedText, item.CodeLanguage, item.TokenEstimate, item.SourceStart,
			item.SourceEnd, item.ContentHash, item.Metadata)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.Symbols {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_symbols(id,deployment_id,sdk_content_candidate_id,sdk_publication_file_id,sdk_section_id,language,symbol_kind,qualified_name,display_name,signature,documentation,identifiers,source_start,source_end,content_hash,metadata)
			VALUES($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, item.ID, candidate.DeploymentID,
			item.SDKContentCandidateID, item.SDKPublicationFileID, item.SDKSectionID, item.Language, item.Kind, item.QualifiedName,
			item.DisplayName, item.Signature, item.Documentation, item.Identifiers, item.SourceStart, item.SourceEnd, item.ContentHash, item.Metadata)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.Samples {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_code_samples(id,deployment_id,sdk_content_candidate_id,sdk_publication_file_id,sdk_section_id,language,title,intent,code,imports,prerequisites,origin,source_uri,source_revision,source_path,source_start,source_end,attribution,license_expression,validation_status,validation_evidence,visibility,content_hash)
			VALUES($1,$2,$3,nullif($4,'')::uuid,nullif($5,'')::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
			item.ID, item.DeploymentID, item.SDKContentCandidateID, item.SDKPublicationFileID, item.SDKSectionID, item.Language,
			item.Title, item.Intent, item.Code, item.Imports, item.Prerequisites, item.Origin, item.SourceURI, item.SourceRevision,
			item.SourcePath, item.SourceStart, item.SourceEnd, item.Attribution, item.LicenseExpression, item.ValidationStatus,
			item.ValidationEvidence, item.Visibility, item.ContentHash)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	if value.Map != nil {
		item := *value.Map
		_, err = tx.Exec(ctx, `INSERT INTO sdk_maps(id,deployment_id,sdk_content_candidate_id,map_version,structured_map,agent_markdown,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			item.ID, item.DeploymentID, item.SDKContentCandidateID, item.MapVersion, item.Map, item.AgentMarkdown, item.ContentHash)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	for _, item := range value.SampleRefs {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_sample_api_references(id,sdk_code_sample_id,sdk_content_candidate_id,deployment_id,integration_id,api_contract_revision_id,api_contract_candidate_id,api_contract_operation_id,api_sdk_binding_id,reference_kind)
			VALUES($1,$2,$3,$4,$5,nullif($6,'')::uuid,nullif($7,'')::uuid,nullif($8,'')::uuid,nullif($9,'')::uuid,$10)`,
			item.ID, item.SDKCodeSampleID, item.SDKContentCandidateID, item.DeploymentID, item.APIID, item.APIContractRevisionID,
			item.APIContractCandidateID, item.APIContractOperationID, item.APISDKBindingID, item.ReferenceKind)
		if err != nil {
			return model.SDKContentCandidate{}, databaseError(err)
		}
	}
	return created, nil
}

func (p *Postgres) CreateSDKContentCandidate(ctx context.Context, value SDKContentCandidateRecord) (model.SDKContentCandidate, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SDKContentCandidate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createSDKContentCandidateTx(ctx, tx, value)
	if err != nil {
		return model.SDKContentCandidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SDKContentCandidate{}, err
	}
	return created, nil
}

func (p *Postgres) FinalizeSDKContentIngestion(ctx context.Context, value SDKContentIngestionFinalization) (model.SDKContentCandidate, model.DeveloperAssetIngestionRun, error) {
	if value.ExpectedRunState != model.DeveloperAssetIngestionRunning ||
		value.Run.State != model.DeveloperAssetIngestionReviewReady ||
		value.Candidate.Candidate.IngestionRunID != value.Run.ID ||
		value.Candidate.Candidate.DeploymentID != value.Run.DeploymentID ||
		value.Candidate.Candidate.SDKReleaseID != value.Run.TargetID ||
		value.Run.FinishedAt == nil {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	for _, stage := range value.Stages {
		if stage.IngestionRunID != value.Run.ID || stage.Attempt != value.Run.Attempt {
			return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, ErrConflict
		}
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createSDKContentCandidateTx(ctx, tx, value.Candidate)
	if err != nil {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
	}
	for _, stage := range value.Stages {
		_, err = scanDeveloperAssetIngestionStage(tx.QueryRow(ctx, `INSERT INTO developer_asset_ingestion_stages(
			id,ingestion_run_id,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text,ingestion_run_id::text,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at,created_at,updated_at`,
			stage.ID, stage.IngestionRunID, stage.Name, stage.Attempt, stage.State, stage.InputHash, stage.OutputHash,
			stage.Checkpoint, stage.Diagnostics, stage.ErrorCode, stage.ErrorMessage, stage.StartedAt, stage.FinishedAt))
		if err != nil {
			return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
		}
	}
	run := value.Run
	updated, err := scanDeveloperAssetIngestionRun(tx.QueryRow(ctx, `UPDATE developer_asset_ingestion_runs SET
		resolved_source_uri=$3,resolved_source_revision=$4,resolved_source_hash=$5,state=$6,attempt=$7,pipeline_version=$8,parser_version=$9,
		normalizer_version=$10,mapper_version=$11,raw_manifest=$12,raw_manifest_hash=$13,diagnostics=$14,discovered_count=$15,
		acquired_count=$16,failed_count=$17,skipped_count=$18,quarantined_count=$19,lease_owner=$20,lease_expires_at=$21,
		heartbeat_at=$22,error_code=$23,error_message=$24,started_at=$25,finished_at=$26
	WHERE deployment_id=$1 AND id=$2 AND state=$27 RETURNING `+developerAssetIngestionRunSelect[len("SELECT "):len(developerAssetIngestionRunSelect)-len(" FROM developer_asset_ingestion_runs")],
		run.DeploymentID, run.ID, run.ResolvedSourceURI, run.ResolvedSourceRevision, run.ResolvedSourceHash, run.State,
		run.Attempt, run.Versions.Pipeline, run.Versions.Parser, run.Versions.Normalizer, run.Versions.Mapper, run.RawManifest,
		run.RawManifestHash, run.Diagnostics, run.DiscoveredCount, run.AcquiredCount, run.FailedCount, run.SkippedCount,
		run.QuarantinedCount, run.LeaseOwner, run.LeaseExpiresAt, run.HeartbeatAt, run.ErrorCode, run.ErrorMessage,
		run.StartedAt, run.FinishedAt, value.ExpectedRunState))
	if errors.Is(err, ErrNotFound) {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if err != nil {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
	}
	return created, updated, nil
}

const sdkContentPublicationSelect = `SELECT id::text,deployment_id::text,sdk_release_id::text,sdk_content_candidate_id::text,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at,created_at FROM sdk_content_publications`

func scanSDKContentPublication(row pgx.Row) (model.SDKContentPublication, error) {
	var value model.SDKContentPublication
	err := row.Scan(&value.ID, &value.DeploymentID, &value.SDKReleaseID, &value.SDKContentCandidateID, &value.Revision,
		&value.ContentHash, &value.Visibility, &value.ReviewedBy, &value.ReviewedAt, &value.PublishedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKContentPublications(ctx context.Context, deploymentID, releaseID string) ([]model.SDKContentPublication, error) {
	rows, err := p.pool.Query(ctx, sdkContentPublicationSelect+` WHERE deployment_id=$1 AND sdk_release_id=$2 ORDER BY revision DESC`, deploymentID, releaseID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKContentPublication, 0)
	for rows.Next() {
		value, scanErr := scanSDKContentPublication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SDKContentPublication(ctx context.Context, deploymentID, id string) (SDKContentPublicationRecord, error) {
	var result SDKContentPublicationRecord
	value, err := scanSDKContentPublication(p.pool.QueryRow(ctx, sdkContentPublicationSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
	if err != nil {
		return result, err
	}
	result.Publication = value
	rows, err := p.pool.Query(ctx, `SELECT sdk_content_publication_id::text,deployment_id::text,sdk_content_candidate_id::text,sdk_publication_file_id::text,decision,reason,ordinal,content_hash,created_at FROM sdk_content_publication_file_selections WHERE deployment_id=$1 AND sdk_content_publication_id=$2 ORDER BY ordinal NULLS LAST,sdk_publication_file_id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.SDKContentPublicationFileSelection
		if err := rows.Scan(&item.SDKContentPublicationID, &item.DeploymentID, &item.SDKContentCandidateID, &item.SDKPublicationFileID,
			&item.Decision, &item.Reason, &item.Ordinal, &item.ContentHash, &item.CreatedAt); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		result.FileSelections = append(result.FileSelections, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.pool.Query(ctx, `SELECT sdk_content_publication_id::text,deployment_id::text,sdk_content_candidate_id::text,sdk_code_sample_id::text,decision,reason,review_evidence,ordinal,reviewed_by,reviewed_at,content_hash,created_at FROM sdk_content_publication_sample_selections WHERE deployment_id=$1 AND sdk_content_publication_id=$2 ORDER BY ordinal NULLS LAST,sdk_code_sample_id`, deploymentID, id)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var item model.SDKContentPublicationSampleSelection
		if err := rows.Scan(&item.SDKContentPublicationID, &item.DeploymentID, &item.SDKContentCandidateID, &item.SDKCodeSampleID,
			&item.Decision, &item.Reason, &item.ReviewEvidence, &item.Ordinal, &item.ReviewedBy, &item.ReviewedAt, &item.ContentHash, &item.CreatedAt); err != nil {
			rows.Close()
			return result, databaseError(err)
		}
		if bytes.Equal(bytes.TrimSpace(item.ReviewEvidence), []byte("{}")) {
			item.ReviewEvidence = nil
		}
		result.SampleSelections = append(result.SampleSelections, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	var mapValue model.SDKContentPublicationMap
	err = p.pool.QueryRow(ctx, `SELECT sdk_content_publication_id::text,deployment_id::text,sdk_content_candidate_id::text,sdk_map_id::text,content_hash,created_at FROM sdk_content_publication_maps WHERE deployment_id=$1 AND sdk_content_publication_id=$2`, deploymentID, id).
		Scan(&mapValue.SDKContentPublicationID, &mapValue.DeploymentID, &mapValue.SDKContentCandidateID, &mapValue.SDKMapID, &mapValue.ContentHash, &mapValue.CreatedAt)
	if err == nil {
		result.Map = &mapValue
	} else if databaseError(err) != ErrNotFound {
		return result, databaseError(err)
	}
	if result.Map != nil {
		var publishedMap model.SDKMap
		err = p.pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,sdk_content_candidate_id::text,map_version,structured_map,agent_markdown,content_hash,created_at FROM sdk_maps WHERE deployment_id=$1 AND sdk_content_candidate_id=$2 AND id=$3`, deploymentID, value.SDKContentCandidateID, result.Map.SDKMapID).
			Scan(&publishedMap.ID, &publishedMap.DeploymentID, &publishedMap.SDKContentCandidateID, &publishedMap.MapVersion, &publishedMap.Map, &publishedMap.AgentMarkdown, &publishedMap.ContentHash, &publishedMap.CreatedAt)
		if err != nil {
			return result, databaseError(err)
		}
		result.PublishedMap = &publishedMap
	}
	return result, nil
}

func (p *Postgres) PublishSDKContentCandidate(ctx context.Context, value SDKContentPublicationRecord) (model.SDKContentPublication, error) {
	candidate, err := p.SDKContentCandidate(ctx, value.Publication.DeploymentID, value.Publication.SDKContentCandidateID)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	release, err := p.SDKRelease(ctx, value.Publication.DeploymentID, value.Publication.SDKReleaseID)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	packageValue, err := p.SDKPackage(ctx, value.Publication.DeploymentID, release.SDKPackageID)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	samples := make(map[string]model.SDKCodeSample, len(candidate.Samples))
	for _, sample := range candidate.Samples {
		samples[sample.ID] = sample
	}
	for _, selection := range value.SampleSelections {
		sample, ok := samples[selection.SDKCodeSampleID]
		if !ok || !selection.ValidFor(sample) {
			return model.SDKContentPublication{}, ErrConflict
		}
	}
	if (candidate.Map == nil) != (value.Map == nil) || (value.Map == nil) != (value.PublishedMap == nil) {
		return model.SDKContentPublication{}, ErrConflict
	}
	if value.Map != nil {
		publishedMap := value.PublishedMap
		if publishedMap == nil || publishedMap.ID == candidate.Map.ID || publishedMap.MapVersion == candidate.Map.MapVersion ||
			publishedMap.DeploymentID != value.Publication.DeploymentID || publishedMap.SDKContentCandidateID != candidate.Candidate.ID ||
			publishedMap.ID != value.Map.SDKMapID || publishedMap.ContentHash == "" || publishedMap.ContentHash != value.Map.ContentHash ||
			publishedMap.MapVersion == "" || publishedMap.AgentMarkdown == "" || value.Map.DeploymentID != value.Publication.DeploymentID ||
			value.Map.SDKContentPublicationID != value.Publication.ID || value.Map.SDKContentCandidateID != candidate.Candidate.ID {
			return model.SDKContentPublication{}, ErrConflict
		}
	}
	if err := ValidateReviewedSDKPublicationMap(packageValue, release, candidate, value); err != nil {
		return model.SDKContentPublication{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	publication := value.Publication
	var candidateFileCount, candidateSampleCount, candidateMapCount int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM sdk_publication_files WHERE deployment_id=$1 AND sdk_content_candidate_id=$2),
		(SELECT count(*) FROM sdk_code_samples WHERE deployment_id=$1 AND sdk_content_candidate_id=$2),
		(SELECT count(*) FROM sdk_maps WHERE deployment_id=$1 AND sdk_content_candidate_id=$2)`,
		publication.DeploymentID, publication.SDKContentCandidateID).Scan(&candidateFileCount, &candidateSampleCount, &candidateMapCount); err != nil {
		return model.SDKContentPublication{}, databaseError(err)
	}
	if candidateFileCount != len(value.FileSelections) || candidateSampleCount != len(value.SampleSelections) ||
		((candidateMapCount == 0) != (value.Map == nil)) {
		return model.SDKContentPublication{}, ErrConflict
	}
	created, err := scanSDKContentPublication(tx.QueryRow(ctx, `INSERT INTO sdk_content_publications(id,deployment_id,sdk_release_id,sdk_content_candidate_id,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at)
		SELECT $1,$2,$3,$4,coalesce((SELECT max(revision)+1 FROM sdk_content_publications WHERE sdk_release_id=$3),1),$5,$6,$7,$8,$9
		FROM sdk_content_candidates candidate WHERE candidate.id=$4 AND candidate.sdk_release_id=$3 AND candidate.deployment_id=$2 AND candidate.content_hash=$5 AND candidate.visibility=$6
		RETURNING id::text,deployment_id::text,sdk_release_id::text,sdk_content_candidate_id::text,revision,content_hash,visibility,reviewed_by,reviewed_at,published_at,created_at`,
		publication.ID, publication.DeploymentID, publication.SDKReleaseID, publication.SDKContentCandidateID,
		publication.ContentHash, publication.Visibility, publication.ReviewedBy, publication.ReviewedAt, publication.PublishedAt))
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	for _, item := range value.FileSelections {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_content_publication_file_selections(sdk_content_publication_id,deployment_id,sdk_content_candidate_id,sdk_publication_file_id,decision,reason,ordinal,content_hash)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, created.ID, item.DeploymentID, item.SDKContentCandidateID,
			item.SDKPublicationFileID, item.Decision, item.Reason, item.Ordinal, item.ContentHash)
		if err != nil {
			return model.SDKContentPublication{}, databaseError(err)
		}
	}
	for _, item := range value.SampleSelections {
		_, err = tx.Exec(ctx, `INSERT INTO sdk_content_publication_sample_selections(sdk_content_publication_id,deployment_id,sdk_content_candidate_id,sdk_code_sample_id,decision,reason,review_evidence,ordinal,reviewed_by,reviewed_at,content_hash)
			VALUES($1,$2,$3,$4,$5,$6,coalesce($7::jsonb,'{}'::jsonb),$8,$9,$10,$11)`, created.ID, item.DeploymentID, item.SDKContentCandidateID,
			item.SDKCodeSampleID, item.Decision, item.Reason, item.ReviewEvidence, item.Ordinal, item.ReviewedBy, item.ReviewedAt, item.ContentHash)
		if err != nil {
			return model.SDKContentPublication{}, databaseError(err)
		}
	}
	if value.PublishedMap != nil {
		item := *value.PublishedMap
		_, err = tx.Exec(ctx, `INSERT INTO sdk_maps(id,deployment_id,sdk_content_candidate_id,map_version,structured_map,agent_markdown,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			item.ID, item.DeploymentID, item.SDKContentCandidateID, item.MapVersion, item.Map, item.AgentMarkdown, item.ContentHash)
		if err != nil {
			return model.SDKContentPublication{}, databaseError(err)
		}
	}
	if value.Map != nil {
		item := *value.Map
		_, err = tx.Exec(ctx, `INSERT INTO sdk_content_publication_maps(sdk_content_publication_id,deployment_id,sdk_content_candidate_id,sdk_map_id,content_hash) VALUES($1,$2,$3,$4,$5)`,
			created.ID, item.DeploymentID, item.SDKContentCandidateID, item.SDKMapID, item.ContentHash)
		if err != nil {
			return model.SDKContentPublication{}, databaseError(err)
		}
	}
	result, err := tx.Exec(ctx, `UPDATE developer_asset_ingestion_runs run SET state='published',lease_owner='',lease_expires_at=NULL,heartbeat_at=NULL,finished_at=coalesce(finished_at,now())
		FROM sdk_content_candidates candidate WHERE candidate.id=$1 AND run.id=candidate.ingestion_run_id AND run.deployment_id=$2
		AND run.asset_kind='sdk' AND run.target_id=$3 AND run.state='review_ready' AND run.failed_count=0 AND run.skipped_count=0 AND run.quarantined_count=0`,
		created.SDKContentCandidateID, created.DeploymentID, created.SDKReleaseID)
	if err != nil {
		return model.SDKContentPublication{}, databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return model.SDKContentPublication{}, ErrConflict
	}
	if err := bumpDeploymentCatalog(ctx, tx, publication.DeploymentID); err != nil {
		return model.SDKContentPublication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SDKContentPublication{}, err
	}
	return created, nil
}

const sdkCompatibilityAssertionSelect = `SELECT id::text,deployment_id::text,integration_id::text,sdk_release_id::text,coalesce(api_contract_revision_id::text,''),coalesce(supersedes_assertion_id::text,''),coverage,assurance,assertion_state,applicable_modules,applicable_capabilities,applicable_operation_keys,known_gaps,evidence,content_hash,reviewed_by,reviewed_at,created_at FROM sdk_compatibility_assertions`

func scanSDKCompatibilityAssertion(row pgx.Row) (model.SDKCompatibilityAssertion, error) {
	var value model.SDKCompatibilityAssertion
	err := row.Scan(&value.ID, &value.DeploymentID, &value.APIID, &value.SDKReleaseID, &value.APIContractRevisionID,
		&value.SupersedesAssertionID, &value.Coverage, &value.Assurance, &value.State, &value.ApplicableModules,
		&value.ApplicableCapabilities, &value.ApplicableOperationKeys, &value.KnownGaps, &value.Evidence, &value.ContentHash,
		&value.ReviewedBy, &value.ReviewedAt, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) SDKCompatibilityAssertions(ctx context.Context, deploymentID, apiID, releaseID string) ([]model.SDKCompatibilityAssertion, error) {
	rows, err := p.pool.Query(ctx, sdkCompatibilityAssertionSelect+` WHERE deployment_id=$1 AND integration_id=$2 AND sdk_release_id=$3 ORDER BY created_at DESC`, deploymentID, apiID, releaseID)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.SDKCompatibilityAssertion, 0)
	for rows.Next() {
		value, scanErr := scanSDKCompatibilityAssertion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) SDKCompatibilityAssertion(ctx context.Context, deploymentID, id string) (model.SDKCompatibilityAssertion, error) {
	return scanSDKCompatibilityAssertion(p.pool.QueryRow(ctx, sdkCompatibilityAssertionSelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) CreateSDKCompatibilityAssertion(ctx context.Context, value model.SDKCompatibilityAssertion) (model.SDKCompatibilityAssertion, error) {
	return scanSDKCompatibilityAssertion(p.pool.QueryRow(ctx, `INSERT INTO sdk_compatibility_assertions(id,deployment_id,integration_id,sdk_release_id,api_contract_revision_id,supersedes_assertion_id,coverage,assurance,assertion_state,applicable_modules,applicable_capabilities,applicable_operation_keys,known_gaps,evidence,content_hash,reviewed_by,reviewed_at)
		VALUES($1,$2,$3,$4,nullif($5,'')::uuid,nullif($6,'')::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id::text,deployment_id::text,integration_id::text,sdk_release_id::text,coalesce(api_contract_revision_id::text,''),coalesce(supersedes_assertion_id::text,''),coverage,assurance,assertion_state,applicable_modules,applicable_capabilities,applicable_operation_keys,known_gaps,evidence,content_hash,reviewed_by,reviewed_at,created_at`,
		value.ID, value.DeploymentID, value.APIID, value.SDKReleaseID, value.APIContractRevisionID, value.SupersedesAssertionID,
		value.Coverage, value.Assurance, value.State, value.ApplicableModules, value.ApplicableCapabilities,
		value.ApplicableOperationKeys, value.KnownGaps, value.Evidence, value.ContentHash, value.ReviewedBy, value.ReviewedAt))
}
