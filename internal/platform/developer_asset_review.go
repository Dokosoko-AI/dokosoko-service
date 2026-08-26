package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func reviewableDeveloperAssetRun(run model.DeveloperAssetIngestionRun, kind model.DeveloperAssetKind, targetID string) bool {
	return run.AssetKind == kind && run.TargetID == targetID && run.State == model.DeveloperAssetIngestionReviewReady &&
		run.AcquiredCount > 0 && run.FailedCount == 0 && run.SkippedCount == 0 && run.QuarantinedCount == 0 &&
		run.FinishedAt != nil
}

type APIContractCandidatePublicationInput struct {
	ContractRevision    int64
	AcknowledgeReviewed bool
}

func contractCandidateValidated(raw json.RawMessage) bool {
	var result struct {
		Valid  bool              `json:"valid"`
		Errors []json.RawMessage `json:"errors"`
	}
	return json.Unmarshal(raw, &result) == nil && result.Valid && len(result.Errors) == 0
}

func (s *Service) PublishAPIContractCandidate(ctx context.Context, contractID, candidateID string, input APIContractCandidatePublicationInput, actor Actor) (model.APIContract, model.APIContractRevision, error) {
	if !input.AcknowledgeReviewed {
		return model.APIContract{}, model.APIContractRevision{}, ErrSourceReviewRequired
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	contract, err := s.store.APIContract(ctx, deployment.ID, strings.TrimSpace(contractID))
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	record, err := s.store.APIContractCandidate(ctx, deployment.ID, strings.TrimSpace(candidateID))
	if err != nil || record.Candidate.APIContractID != contract.ID {
		return model.APIContract{}, model.APIContractRevision{}, errors.New("contract candidate does not belong to the selected contract")
	}
	run, err := s.store.DeveloperAssetIngestionRun(ctx, deployment.ID, record.Candidate.IngestionRunID)
	if err != nil || !reviewableDeveloperAssetRun(run, model.DeveloperAssetContract, contract.ID) {
		return model.APIContract{}, model.APIContractRevision{}, ErrSourceReviewRequired
	}
	if !contractCandidateValidated(record.Candidate.ValidationResult) {
		return model.APIContract{}, model.APIContractRevision{}, errors.New("contract candidate has not passed deterministic validation")
	}
	if record.Map == nil || strings.TrimSpace(record.Map.AgentMarkdown) == "" || record.Map.ContentHash == "" {
		return model.APIContract{}, model.APIContractRevision{}, errors.New("contract candidate is missing its reviewed Contract Map")
	}
	if record.Candidate.Visibility == model.VisibilityPublic && contract.Visibility != model.VisibilityPublic {
		return model.APIContract{}, model.APIContractRevision{}, errors.New("public candidate cannot widen private contract visibility")
	}
	if input.ContractRevision != contract.Revision {
		return model.APIContract{}, model.APIContractRevision{}, store.ErrConflict
	}
	id, err := randomUUID()
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	now := s.now()
	revision := model.APIContractRevision{
		ID: id, DeploymentID: deployment.ID, APIContractID: contract.ID, APIContractCandidateID: record.Candidate.ID,
		APIContractName: contract.Name, APIContractSlug: contract.Slug,
		APIContractDescription: contract.Description, APIContractKind: contract.Kind,
		ContentHash: record.Candidate.ContentHash, Visibility: record.Candidate.Visibility,
		ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now,
	}
	publications, err := s.store.SourcePublications(ctx, deployment.ID, run.SourceID)
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	var exactPublication *model.SourcePublication
	for index := range publications {
		if publications[index].CrawlJobID == run.ID {
			exactPublication = &publications[index]
			break
		}
	}
	if exactPublication == nil {
		return model.APIContract{}, model.APIContractRevision{}, errors.New("contract candidate requires a reviewed source publication from its exact ingestion generation")
	}
	sourceEvidence := &model.APIContractRevisionSourcePublication{
		APIContractRevisionID: revision.ID, DeploymentID: deployment.ID,
		APIContractCandidateID: revision.APIContractCandidateID, SourcePublicationID: exactPublication.ID,
		ContentHash: exactPublication.ContentHash,
	}
	updated, revision, err := s.store.PublishAPIContractCandidate(ctx, contract, input.ContractRevision, revision, sourceEvidence)
	if err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api_contract.revision_published", "api_contract_revision", revision.ID, map[string]any{
		"api_contract_id": contract.ID, "api_contract_candidate_id": record.Candidate.ID,
		"revision": revision.Revision, "content_hash": revision.ContentHash, "visibility": revision.Visibility,
	}); err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	return updated, revision, nil
}

type DeveloperAssetReviewDecision struct {
	ID             string          `json:"id"`
	Decision       string          `json:"decision"`
	Reason         string          `json:"reason,omitempty"`
	ReviewEvidence json.RawMessage `json:"review_evidence,omitempty"`
}

type SDKContentCandidatePublicationInput struct {
	Files               []DeveloperAssetReviewDecision `json:"files"`
	Samples             []DeveloperAssetReviewDecision `json:"samples"`
	AcknowledgeReviewed bool                           `json:"acknowledge_reviewed"`
}

func indexedReviewDecisions(decisions []DeveloperAssetReviewDecision, allowed map[string]bool) (map[string]DeveloperAssetReviewDecision, error) {
	result := make(map[string]DeveloperAssetReviewDecision, len(decisions))
	for _, decision := range decisions {
		decision.ID = strings.TrimSpace(decision.ID)
		decision.Decision = strings.TrimSpace(decision.Decision)
		decision.Reason = strings.TrimSpace(decision.Reason)
		decision.ReviewEvidence = bytes.TrimSpace(decision.ReviewEvidence)
		if bytes.Equal(decision.ReviewEvidence, []byte("null")) {
			decision.ReviewEvidence = nil
		}
		if decision.ID == "" || !allowed[decision.Decision] || result[decision.ID].ID != "" {
			return nil, errors.New("review decisions must contain unique IDs and supported decisions")
		}
		if (decision.Decision == "included" || decision.Decision == "approved") != (decision.Reason == "") {
			return nil, errors.New("included or approved decisions cannot have a reason; exclusions and quarantines require one")
		}
		if len(decision.ReviewEvidence) > 0 && !model.ValidSDKSampleReviewEvidence(decision.ReviewEvidence) {
			return nil, errors.New("review evidence must be a bounded object with a non-empty summary")
		}
		result[decision.ID] = decision
	}
	return result, nil
}

func (s *Service) PublishSDKContentCandidate(ctx context.Context, releaseID, candidateID string, input SDKContentCandidatePublicationInput, actor Actor) (model.SDKContentPublication, error) {
	if !input.AcknowledgeReviewed {
		return model.SDKContentPublication{}, ErrSourceReviewRequired
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	release, err := s.store.SDKRelease(ctx, deployment.ID, strings.TrimSpace(releaseID))
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	packageValue, err := s.store.SDKPackage(ctx, deployment.ID, release.SDKPackageID)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	if err := s.ensureSDKReleaseSelectable(ctx, deployment.ID, release); err != nil {
		return model.SDKContentPublication{}, err
	}
	record, err := s.store.SDKContentCandidate(ctx, deployment.ID, strings.TrimSpace(candidateID))
	if err != nil || record.Candidate.SDKReleaseID != release.ID {
		return model.SDKContentPublication{}, errors.New("SDK content candidate does not belong to the selected exact release")
	}
	run, err := s.store.DeveloperAssetIngestionRun(ctx, deployment.ID, record.Candidate.IngestionRunID)
	if err != nil || !reviewableDeveloperAssetRun(run, model.DeveloperAssetSDK, release.ID) {
		return model.SDKContentPublication{}, ErrSourceReviewRequired
	}
	if record.Map == nil || strings.TrimSpace(record.Map.AgentMarkdown) == "" || record.Map.ContentHash == "" {
		return model.SDKContentPublication{}, errors.New("SDK candidate is missing its reviewed SDK Map")
	}
	if record.Candidate.Visibility == model.VisibilityPublic && release.Visibility != model.VisibilityPublic {
		return model.SDKContentPublication{}, errors.New("public SDK content cannot widen private release visibility")
	}
	fileDecisions, err := indexedReviewDecisions(input.Files, map[string]bool{"included": true, "excluded": true, "quarantined": true})
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	sampleDecisions, err := indexedReviewDecisions(input.Samples, map[string]bool{"approved": true, "excluded": true, "quarantined": true})
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	if len(fileDecisions) != len(record.Files) || len(sampleDecisions) != len(record.Samples) {
		return model.SDKContentPublication{}, errors.New("review must explicitly decide every SDK file and code sample")
	}
	id, err := randomUUID()
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	now := s.now()
	publicationRecord := store.SDKContentPublicationRecord{Publication: model.SDKContentPublication{
		ID: id, DeploymentID: deployment.ID, SDKReleaseID: release.ID, SDKContentCandidateID: record.Candidate.ID,
		ContentHash: record.Candidate.ContentHash, Visibility: record.Candidate.Visibility,
		ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now,
	}}
	fileOrdinal := 0
	includedFileOrdinals := make(map[string]int)
	for _, file := range record.Files {
		decision, ok := fileDecisions[file.ID]
		if !ok {
			return model.SDKContentPublication{}, fmt.Errorf("missing review decision for SDK file %s", file.ID)
		}
		var ordinal *int
		if decision.Decision == "included" {
			value := fileOrdinal
			ordinal = &value
			includedFileOrdinals[file.ID] = value
			fileOrdinal++
		}
		publicationRecord.FileSelections = append(publicationRecord.FileSelections, model.SDKContentPublicationFileSelection{
			SDKContentPublicationID: id, DeploymentID: deployment.ID, SDKContentCandidateID: record.Candidate.ID,
			SDKPublicationFileID: file.ID, Decision: decision.Decision, Reason: decision.Reason,
			Ordinal: ordinal, ContentHash: file.ContentHash,
		})
	}
	if fileOrdinal == 0 {
		return model.SDKContentPublication{}, errors.New("an SDK content publication must include at least one reviewed file")
	}
	sampleOrdinal := 0
	for _, sample := range record.Samples {
		decision, ok := sampleDecisions[sample.ID]
		if !ok {
			return model.SDKContentPublication{}, fmt.Errorf("missing review decision for SDK sample %s", sample.ID)
		}
		if decision.Decision == "approved" && !sample.HasPositiveMachineValidationEvidence() && !model.ValidSDKSampleReviewEvidence(decision.ReviewEvidence) {
			return model.SDKContentPublication{}, fmt.Errorf("SDK sample %s requires positive machine validation evidence or explicit review evidence before approval", sample.ID)
		}
		if decision.Decision == "approved" && sample.SDKPublicationFileID != "" {
			if _, included := includedFileOrdinals[sample.SDKPublicationFileID]; !included {
				return model.SDKContentPublication{}, fmt.Errorf("SDK sample %s cannot be approved because its source file was not included", sample.ID)
			}
		}
		var ordinal *int
		if decision.Decision == "approved" {
			value := sampleOrdinal
			ordinal = &value
			sampleOrdinal++
		}
		publicationRecord.SampleSelections = append(publicationRecord.SampleSelections, model.SDKContentPublicationSampleSelection{
			SDKContentPublicationID: id, DeploymentID: deployment.ID, SDKContentCandidateID: record.Candidate.ID,
			SDKCodeSampleID: sample.ID, Decision: decision.Decision, Reason: decision.Reason,
			ReviewEvidence: append(json.RawMessage(nil), decision.ReviewEvidence...), Ordinal: ordinal,
			ReviewedBy: actor.ID, ReviewedAt: now, ContentHash: sample.ContentHash,
		})
	}
	publishedMap, err := store.BuildReviewedSDKPublicationMap(packageValue, release, record, publicationRecord)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	publicationRecord.PublishedMap = publishedMap
	publicationRecord.Map = &model.SDKContentPublicationMap{
		SDKContentPublicationID: id, DeploymentID: deployment.ID, SDKContentCandidateID: record.Candidate.ID,
		SDKMapID: publishedMap.ID, ContentHash: publishedMap.ContentHash,
	}
	publication, err := s.store.PublishSDKContentCandidate(ctx, publicationRecord)
	if err != nil {
		return model.SDKContentPublication{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "sdk_content.publication_created", "sdk_content_publication", publication.ID, map[string]any{
		"sdk_release_id": release.ID, "sdk_content_candidate_id": record.Candidate.ID,
		"revision": publication.Revision, "content_hash": publication.ContentHash,
		"included_files": fileOrdinal, "approved_samples": sampleOrdinal, "visibility": publication.Visibility,
	}); err != nil {
		return model.SDKContentPublication{}, err
	}
	return publication, nil
}
