package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func knowledgeStageVersion(stage model.DeveloperAssetIngestionStage) string {
	var checkpoint struct {
		Version string `json:"workflow_version"`
	}
	if json.Unmarshal(stage.Checkpoint, &checkpoint) != nil {
		return ""
	}
	return checkpoint.Version
}

func (m *Memory) ClaimKnowledgeProcessingStage(_ context.Context, deploymentID string, stage model.DeveloperAssetIngestionStage, staleBefore time.Time) (model.DeveloperAssetIngestionStage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.developerAssets.ingestionRuns[stage.IngestionRunID]
	if !ok || run.DeploymentID != deploymentID {
		return model.DeveloperAssetIngestionStage{}, ErrNotFound
	}
	version := knowledgeStageVersion(stage)
	if version == "" || stage.Name != model.IngestionStageAIEnrich || stage.State != "running" || stage.StartedAt == nil {
		return model.DeveloperAssetIngestionStage{}, ErrConflict
	}
	if _, exists := m.developerAssets.ingestionStages[run.ID][stage.ID]; exists {
		return model.DeveloperAssetIngestionStage{}, ErrConflict
	}
	attempt := 1
	for _, existing := range m.developerAssets.ingestionStages[run.ID] {
		if existing.Name != stage.Name {
			continue
		}
		attempt = max(attempt, existing.Attempt+1)
		if knowledgeStageVersion(existing) != version || existing.State != "running" {
			continue
		}
		if existing.StartedAt == nil || existing.StartedAt.After(staleBefore) {
			return model.DeveloperAssetIngestionStage{}, ErrConflict
		}
	}
	for _, existing := range m.developerAssets.ingestionStages[run.ID] {
		if existing.Name == stage.Name && knowledgeStageVersion(existing) == version && existing.State == "running" {
			existing.State = "failed"
			existing.ErrorCode = "interrupted"
			existing.FinishedAt = stage.StartedAt
			if _, err := m.saveDeveloperAssetIngestionStageLocked(existing, "running"); err != nil {
				return model.DeveloperAssetIngestionStage{}, err
			}
		}
	}
	stage.Attempt = attempt
	return m.saveDeveloperAssetIngestionStageLocked(stage, "")
}

func (p *Postgres) ClaimKnowledgeProcessingStage(ctx context.Context, deploymentID string, stage model.DeveloperAssetIngestionStage, staleBefore time.Time) (model.DeveloperAssetIngestionStage, error) {
	version := knowledgeStageVersion(stage)
	if version == "" || stage.Name != model.IngestionStageAIEnrich || stage.State != "running" || stage.StartedAt == nil {
		return model.DeveloperAssetIngestionStage{}, ErrConflict
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.DeveloperAssetIngestionStage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM developer_asset_ingestion_runs WHERE id=$1 AND deployment_id=$2 FOR UPDATE`, stage.IngestionRunID, deploymentID).Scan(&id); err != nil {
		return model.DeveloperAssetIngestionStage{}, databaseError(err)
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM developer_asset_ingestion_stages WHERE ingestion_run_id=$1 AND stage_name='ai_enrich' AND checkpoint->>'workflow_version'=$2 AND state='running' AND (started_at IS NULL OR started_at>$3))`, id, version, staleBefore).Scan(&active); err != nil {
		return model.DeveloperAssetIngestionStage{}, databaseError(err)
	}
	if active {
		return model.DeveloperAssetIngestionStage{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE developer_asset_ingestion_stages SET state='failed',error_code='interrupted',finished_at=$3,updated_at=now() WHERE ingestion_run_id=$1 AND stage_name='ai_enrich' AND checkpoint->>'workflow_version'=$2 AND state='running'`, id, version, stage.StartedAt); err != nil {
		return model.DeveloperAssetIngestionStage{}, databaseError(err)
	}
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(attempt),0)+1 FROM developer_asset_ingestion_stages WHERE ingestion_run_id=$1 AND stage_name='ai_enrich'`, id).Scan(&stage.Attempt); err != nil {
		return model.DeveloperAssetIngestionStage{}, databaseError(err)
	}
	created, err := scanDeveloperAssetIngestionStage(tx.QueryRow(ctx, `INSERT INTO developer_asset_ingestion_stages(id,ingestion_run_id,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,started_at)
 VALUES($1,$2,'ai_enrich',$3,'running',$4,'',$5,$6,$7)
 RETURNING id::text,ingestion_run_id::text,stage_name,attempt,state,input_hash,output_hash,checkpoint,diagnostics,error_code,error_message,started_at,finished_at,created_at,updated_at`, stage.ID, id, stage.Attempt, stage.InputHash, stage.Checkpoint, stage.Diagnostics, stage.StartedAt))
	if err != nil {
		return model.DeveloperAssetIngestionStage{}, databaseError(err)
	}
	return created, tx.Commit(ctx)
}
