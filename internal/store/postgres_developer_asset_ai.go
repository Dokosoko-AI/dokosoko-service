package store

import (
	"context"
	"errors"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/jackc/pgx/v5"
)

const developerAssetAIAdvisorySelect = `SELECT id::text,deployment_id::text,prompt_key,prompt_version,scope_kind,scope_id::text,scope_visibility,
	coalesce(ingestion_run_id::text,''),coalesce(source_publication_id::text,''),coalesce(sdk_package_id::text,''),
	coalesce(sdk_release_id::text,''),coalesce(sdk_content_candidate_id::text,''),coalesce(sdk_content_publication_id::text,''),
	coalesce(integration_id::text,''),coalesce(api_developer_asset_publication_id::text,''),coalesce(api_sdk_binding_id::text,''),
	coalesce(sdk_code_sample_id::text,''),allowed_evidence_ids,evidence_hash,input_hash,result,result_hash,created_by,created_at
	FROM developer_asset_ai_advisory_runs`

func scanDeveloperAssetAIAdvisory(row pgx.Row) (model.DeveloperAssetAIAdvisoryRun, error) {
	var value model.DeveloperAssetAIAdvisoryRun
	err := row.Scan(&value.ID, &value.DeploymentID, &value.PromptKey, &value.PromptVersion, &value.ScopeKind, &value.ScopeID,
		&value.ScopeVisibility, &value.IngestionRunID, &value.SourcePublicationID, &value.SDKPackageID, &value.SDKReleaseID,
		&value.SDKContentCandidateID, &value.SDKContentPublicationID, &value.APIID, &value.APIDeveloperAssetPublicationID,
		&value.APISDKBindingID, &value.SDKCodeSampleID, &value.AllowedEvidenceIDs, &value.EvidenceHash, &value.InputHash,
		&value.Result, &value.ResultHash, &value.CreatedBy, &value.CreatedAt)
	return value, databaseError(err)
}

func (p *Postgres) DeveloperAssetAIAdvisoryRuns(ctx context.Context, query DeveloperAssetAIAdvisoryQuery) ([]model.DeveloperAssetAIAdvisoryRun, error) {
	statement := developerAssetAIAdvisorySelect + ` WHERE deployment_id=$1`
	args := []any{query.DeploymentID}
	if query.PromptKey != "" {
		args = append(args, query.PromptKey)
		statement += ` AND prompt_key=$` + postgresArgument(len(args))
	}
	if query.ScopeID != "" {
		args = append(args, query.ScopeID)
		statement += ` AND scope_id=$` + postgresArgument(len(args))
	}
	limit := query.Limit
	if limit < 1 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)
	statement += ` ORDER BY created_at DESC,id DESC LIMIT $` + postgresArgument(len(args))
	rows, err := p.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]model.DeveloperAssetAIAdvisoryRun, 0)
	for rows.Next() {
		value, scanErr := scanDeveloperAssetAIAdvisory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) DeveloperAssetAIAdvisoryRun(ctx context.Context, deploymentID, id string) (model.DeveloperAssetAIAdvisoryRun, error) {
	return scanDeveloperAssetAIAdvisory(p.pool.QueryRow(ctx, developerAssetAIAdvisorySelect+` WHERE deployment_id=$1 AND id=$2`, deploymentID, id))
}

func (p *Postgres) DeveloperAssetAIAdvisoryRunByInputHash(ctx context.Context, deploymentID, promptKey, inputHash string) (model.DeveloperAssetAIAdvisoryRun, error) {
	return scanDeveloperAssetAIAdvisory(p.pool.QueryRow(ctx, developerAssetAIAdvisorySelect+` WHERE deployment_id=$1 AND prompt_key=$2 AND input_hash=$3`, deploymentID, promptKey, inputHash))
}

func (p *Postgres) CreateDeveloperAssetAIAdvisoryRun(ctx context.Context, value model.DeveloperAssetAIAdvisoryRun) (model.DeveloperAssetAIAdvisoryRun, error) {
	if !value.Valid() {
		return model.DeveloperAssetAIAdvisoryRun{}, ErrConflict
	}
	created, err := scanDeveloperAssetAIAdvisory(p.pool.QueryRow(ctx, `INSERT INTO developer_asset_ai_advisory_runs(
		id,deployment_id,prompt_key,prompt_version,scope_kind,scope_id,scope_visibility,ingestion_run_id,source_publication_id,
		sdk_package_id,sdk_release_id,sdk_content_candidate_id,sdk_content_publication_id,integration_id,
		api_developer_asset_publication_id,api_sdk_binding_id,sdk_code_sample_id,allowed_evidence_ids,evidence_hash,input_hash,
		result,result_hash,created_by,created_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,'')::uuid,nullif($9,'')::uuid,nullif($10,'')::uuid,nullif($11,'')::uuid,
		nullif($12,'')::uuid,nullif($13,'')::uuid,nullif($14,'')::uuid,nullif($15,'')::uuid,nullif($16,'')::uuid,
		nullif($17,'')::uuid,$18,$19,$20,$21,$22,$23,$24)
	ON CONFLICT(deployment_id,prompt_key,input_hash) DO NOTHING
	RETURNING id::text,deployment_id::text,prompt_key,prompt_version,scope_kind,scope_id::text,scope_visibility,
		coalesce(ingestion_run_id::text,''),coalesce(source_publication_id::text,''),coalesce(sdk_package_id::text,''),
		coalesce(sdk_release_id::text,''),coalesce(sdk_content_candidate_id::text,''),coalesce(sdk_content_publication_id::text,''),
		coalesce(integration_id::text,''),coalesce(api_developer_asset_publication_id::text,''),coalesce(api_sdk_binding_id::text,''),
		coalesce(sdk_code_sample_id::text,''),allowed_evidence_ids,evidence_hash,input_hash,result,result_hash,created_by,created_at`,
		value.ID, value.DeploymentID, value.PromptKey, value.PromptVersion, value.ScopeKind, value.ScopeID,
		value.ScopeVisibility, value.IngestionRunID, value.SourcePublicationID, value.SDKPackageID, value.SDKReleaseID,
		value.SDKContentCandidateID, value.SDKContentPublicationID, value.APIID, value.APIDeveloperAssetPublicationID,
		value.APISDKBindingID, value.SDKCodeSampleID, value.AllowedEvidenceIDs, value.EvidenceHash, value.InputHash,
		value.Result, value.ResultHash, value.CreatedBy, value.CreatedAt))
	if errors.Is(err, ErrNotFound) {
		return p.DeveloperAssetAIAdvisoryRunByInputHash(ctx, value.DeploymentID, value.PromptKey, value.InputHash)
	}
	return created, err
}
