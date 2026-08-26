package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func sdkIngestionStage(runID string, attempt int, name model.IngestionStageName, state, inputHash, outputHash string, checkpoint, diagnostics any) (model.DeveloperAssetIngestionStage, error) {
	id, err := randomUUID()
	if err != nil {
		return model.DeveloperAssetIngestionStage{}, err
	}
	checkpointJSON, _ := json.Marshal(checkpoint)
	diagnosticsJSON, _ := json.Marshal(diagnostics)
	return model.DeveloperAssetIngestionStage{ID: id, IngestionRunID: runID, Name: name, Attempt: attempt, State: state, InputHash: inputHash, OutputHash: outputHash, Checkpoint: checkpointJSON, Diagnostics: diagnosticsJSON}, nil
}

func (s *Service) failSDKIngestionRun(ctx context.Context, run model.DeveloperAssetIngestionRun, cause error) {
	if run.State != model.DeveloperAssetIngestionRunning {
		return
	}
	run.State = model.DeveloperAssetIngestionFailed
	run.ErrorCode = "sdk_ingestion_failed"
	run.ErrorMessage = cause.Error()
	now := s.now()
	run.FinishedAt = &now
	_, _ = s.store.TransitionDeveloperAssetIngestionRun(ctx, run, model.DeveloperAssetIngestionRunning)
}

// IngestSDKReleaseContent deterministically turns bounded raw text files into
// an immutable SDK candidate. It performs no network access, package install,
// compilation, or code execution. Publication remains a separate explicit
// human review action.
func (s *Service) IngestSDKReleaseContent(ctx context.Context, releaseID string, input SDKContentIngestionInput, actor Actor) (SDKContentIngestionResult, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	release, err := s.store.SDKRelease(ctx, deployment.ID, strings.TrimSpace(releaseID))
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	packageValue, err := s.store.SDKPackage(ctx, deployment.ID, release.SDKPackageID)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	input.ResolvedSourceURI = strings.TrimSpace(input.ResolvedSourceURI)
	input.ResolvedSourceRevision = strings.TrimSpace(input.ResolvedSourceRevision)
	input.ExpectedSourceHash = strings.ToLower(strings.TrimSpace(input.ExpectedSourceHash))
	if input.ResolvedSourceURI == "" {
		input.ResolvedSourceURI = release.SourceURL
		if input.ResolvedSourceURI == "" {
			input.ResolvedSourceURI = release.DocumentationURL
		}
	}
	if !validSDKURL(input.ResolvedSourceURI) {
		return SDKContentIngestionResult{}, errors.New("resolved_source_uri must be a fixed public HTTPS URL")
	}
	if input.ResolvedSourceRevision == "" {
		input.ResolvedSourceRevision = release.ResolvedSourceRevision
	}
	if input.ResolvedSourceRevision == "" {
		input.ResolvedSourceRevision = "release:" + release.ExactVersion
	}
	if release.ResolvedSourceRevision != "" && input.ResolvedSourceRevision != release.ResolvedSourceRevision {
		return SDKContentIngestionResult{}, errors.New("resolved_source_revision must match the exact SDK release")
	}
	if input.ExpectedSourceHash != "" && !sdkSourceHashPattern.MatchString(input.ExpectedSourceHash) {
		return SDKContentIngestionResult{}, errors.New("expected_source_hash must be a lowercase sha256 digest")
	}
	build, err := buildSDKContentCandidate(deployment.ID, packageValue, release, input)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	existing, err := s.store.SDKContentCandidates(ctx, deployment.ID, release.ID)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	for _, candidate := range existing {
		if candidate.ContentHash != build.contentHash {
			continue
		}
		record, lookupErr := s.store.SDKContentCandidate(ctx, deployment.ID, candidate.ID)
		if lookupErr != nil {
			return SDKContentIngestionResult{}, lookupErr
		}
		run, lookupErr := s.store.DeveloperAssetIngestionRun(ctx, deployment.ID, candidate.IngestionRunID)
		if lookupErr != nil {
			return SDKContentIngestionResult{}, lookupErr
		}
		return SDKContentIngestionResult{Run: run, Candidate: record, AlreadyIngested: true}, nil
	}
	runID, err := randomUUID()
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	now := s.now()
	manifestJSON, _ := json.Marshal(build.manifest)
	diagnosticJSON, _ := json.Marshal(map[string]any{"items": build.diagnostics, "code_execution": false})
	quarantined := 0
	for _, file := range build.manifest {
		if file.SuggestedDisposition == "quarantined" {
			quarantined++
		}
	}
	run := model.DeveloperAssetIngestionRun{
		ID: runID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		AssetKind: model.DeveloperAssetSDK, TargetID: release.ID, TargetKey: "sdk_release:" + release.ID,
		ResolvedSourceURI: input.ResolvedSourceURI, ResolvedSourceRevision: input.ResolvedSourceRevision,
		ResolvedSourceHash: build.manifestHash, State: model.DeveloperAssetIngestionQueued, Attempt: 1,
		Versions: build.record.Candidate.Versions, RawManifest: manifestJSON, RawManifestHash: build.manifestHash,
		Diagnostics: diagnosticJSON, DiscoveredCount: len(build.manifest), AcquiredCount: len(build.manifest),
		QuarantinedCount: quarantined, QueuedAt: now,
	}
	run, err = s.store.CreateDeveloperAssetIngestionRun(ctx, run)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	run.State, run.StartedAt = model.DeveloperAssetIngestionRunning, &now
	run, err = s.store.TransitionDeveloperAssetIngestionRun(ctx, run, model.DeveloperAssetIngestionQueued)
	if err != nil {
		return SDKContentIngestionResult{}, err
	}
	build.record.Candidate.IngestionRunID = run.ID
	for index := range build.record.Files {
		build.record.Files[index].SDKContentCandidateID = build.record.Candidate.ID
	}
	for index := range build.record.Sections {
		build.record.Sections[index].SDKContentCandidateID = build.record.Candidate.ID
	}
	for index := range build.record.Symbols {
		build.record.Symbols[index].SDKContentCandidateID = build.record.Candidate.ID
	}
	for index := range build.record.Samples {
		build.record.Samples[index].SDKContentCandidateID = build.record.Candidate.ID
	}
	if build.record.Map != nil {
		build.record.Map.SDKContentCandidateID = build.record.Candidate.ID
	}
	stageSpecs := []struct {
		name       model.IngestionStageName
		state      string
		inputHash  string
		outputHash string
		checkpoint any
	}{
		{model.IngestionStageAcquire, "succeeded", build.manifestHash, build.manifestHash, map[string]any{"file_count": len(build.manifest)}},
		{model.IngestionStageValidate, "succeeded", build.manifestHash, build.manifestHash, map[string]any{"quarantined_count": quarantined}},
		{model.IngestionStageParse, "succeeded", build.manifestHash, build.contentHash, map[string]any{"parser_version": sdkIngestionParserVersion}},
		{model.IngestionStageNormalize, "succeeded", build.manifestHash, build.contentHash, map[string]any{"normalizer_version": sdkIngestionNormalizerVersion}},
		{model.IngestionStageSegment, "succeeded", build.contentHash, build.contentHash, map[string]any{"section_count": len(build.record.Sections)}},
		{model.IngestionStageExtract, "succeeded", build.contentHash, build.contentHash, map[string]any{"symbol_count": len(build.record.Symbols), "sample_count": len(build.record.Samples)}},
		{model.IngestionStageMap, "succeeded", build.contentHash, build.mapHash, map[string]any{"map_version": sdkIngestionMapVersion}},
		{model.IngestionStageAIEnrich, "skipped", build.mapHash, "", map[string]any{"reason": "deterministic output does not require AI enrichment"}},
		{model.IngestionStageQualityCheck, "succeeded", build.contentHash, build.contentHash, map[string]any{"diagnostic_count": len(build.diagnostics)}},
		{model.IngestionStageBuildIndex, "skipped", build.mapHash, "", map[string]any{"reason": "indexes are built only from reviewed publications"}},
		{model.IngestionStageReview, "succeeded", build.mapHash, build.mapHash, map[string]any{"state": "review_ready"}},
	}
	stages := make([]model.DeveloperAssetIngestionStage, 0, len(stageSpecs))
	for _, spec := range stageSpecs {
		stage, stageErr := sdkIngestionStage(run.ID, run.Attempt, spec.name, spec.state, spec.inputHash, spec.outputHash, spec.checkpoint, map[string]any{})
		if stageErr != nil {
			s.failSDKIngestionRun(ctx, run, stageErr)
			return SDKContentIngestionResult{}, stageErr
		}
		stages = append(stages, stage)
	}
	finished := s.now()
	reviewReadyRun := run
	reviewReadyRun.State, reviewReadyRun.FinishedAt = model.DeveloperAssetIngestionReviewReady, &finished
	created, finalizedRun, err := s.store.FinalizeSDKContentIngestion(ctx, store.SDKContentIngestionFinalization{
		Candidate: build.record, Stages: stages, Run: reviewReadyRun, ExpectedRunState: model.DeveloperAssetIngestionRunning,
	})
	if err != nil {
		// The aggregate finalization either committed all three parts or none.
		// Marking the still-running attempt failed makes a clean retry possible;
		// an ambiguous response after commit safely leaves review_ready unchanged.
		s.failSDKIngestionRun(ctx, run, err)
		return SDKContentIngestionResult{}, err
	}
	build.record.Candidate = created
	run = finalizedRun
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "sdk_content.candidate_created", "sdk_content_candidate", created.ID, map[string]any{
		"sdk_release_id": release.ID, "ingestion_run_id": run.ID, "content_hash": created.ContentHash,
		"file_count": len(build.record.Files), "section_count": len(build.record.Sections),
		"symbol_count": len(build.record.Symbols), "sample_count": len(build.record.Samples), "code_execution": false,
	}); err != nil {
		return SDKContentIngestionResult{}, err
	}
	return SDKContentIngestionResult{Run: run, Candidate: build.record}, nil
}
