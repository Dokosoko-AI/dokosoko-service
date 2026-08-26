package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) SDKPackages(_ context.Context, deploymentID string) ([]model.SDKPackage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.SDKPackage, 0)
	for _, value := range m.developerAssets.sdkPackages {
		if value.DeploymentID == deploymentID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ecosystem == result[j].Ecosystem {
			return result[i].CanonicalCoordinate < result[j].CanonicalCoordinate
		}
		return result[i].Ecosystem < result[j].Ecosystem
	})
	return result, nil
}

func (m *Memory) SDKPackage(_ context.Context, deploymentID, id string) (model.SDKPackage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkPackages[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.SDKPackage{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveSDKPackage(_ context.Context, value model.SDKPackage, expected int64) (model.SDKPackage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID || value.OrganisationID != m.deployment.OrganisationID {
		return model.SDKPackage{}, ErrNotFound
	}
	for id, current := range m.developerAssets.sdkPackages {
		if id != value.ID && current.DeploymentID == value.DeploymentID && current.Ecosystem == value.Ecosystem && current.CanonicalCoordinate == value.CanonicalCoordinate {
			return model.SDKPackage{}, ErrConflict
		}
	}
	if value.ReplacementSDKPackageID != "" {
		replacement, ok := m.developerAssets.sdkPackages[value.ReplacementSDKPackageID]
		if !ok || replacement.DeploymentID != value.DeploymentID || replacement.ID == value.ID {
			return model.SDKPackage{}, ErrNotFound
		}
	}
	now := time.Now().UTC()
	current, exists := m.developerAssets.sdkPackages[value.ID]
	if !exists {
		if expected != 0 {
			return model.SDKPackage{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
		if value.Lifecycle == "" {
			value.Lifecycle = "draft"
		}
	} else {
		if current.Revision != expected {
			return model.SDKPackage{}, ErrConflict
		}
		if current.Ecosystem != value.Ecosystem ||
			current.CanonicalCoordinate != value.CanonicalCoordinate ||
			current.DisplayCoordinate != value.DisplayCoordinate {
			return model.SDKPackage{}, ErrConflict
		}
		value.OrganisationID, value.CreatedAt = current.OrganisationID, current.CreatedAt
		value.Revision, value.UpdatedAt = expected+1, now
	}
	m.developerAssets.sdkPackages[value.ID] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) SDKReleases(_ context.Context, deploymentID, packageID string) ([]model.SDKRelease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	parent, ok := m.developerAssets.sdkPackages[packageID]
	if !ok || parent.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.SDKRelease, 0, len(m.developerAssets.sdkReleaseIDs[packageID]))
	for _, id := range m.developerAssets.sdkReleaseIDs[packageID] {
		result = append(result, memoryClone(m.developerAssets.sdkReleases[id]))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) SDKRelease(_ context.Context, deploymentID, id string) (model.SDKRelease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkReleases[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.SDKRelease{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateSDKRelease(_ context.Context, value model.SDKRelease) (model.SDKRelease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, ok := m.developerAssets.sdkPackages[value.SDKPackageID]
	if !ok || parent.DeploymentID != value.DeploymentID {
		return model.SDKRelease{}, ErrNotFound
	}
	if _, exists := m.developerAssets.sdkReleases[value.ID]; exists {
		return model.SDKRelease{}, ErrConflict
	}
	for _, current := range m.developerAssets.sdkReleases {
		if current.SDKPackageID == value.SDKPackageID && (current.ExactVersion == value.ExactVersion || current.ReleaseHash == value.ReleaseHash) {
			return model.SDKRelease{}, ErrConflict
		}
	}
	value.CreatedAt = time.Now().UTC()
	if value.Lifecycle == "" {
		value.Lifecycle = "active"
	}
	m.developerAssets.sdkReleases[value.ID] = memoryClone(value)
	m.developerAssets.sdkReleaseIDs[value.SDKPackageID] = append(m.developerAssets.sdkReleaseIDs[value.SDKPackageID], value.ID)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) SDKReleaseLifecycleEvents(_ context.Context, deploymentID, releaseID string) ([]model.SDKReleaseLifecycleEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	release, ok := m.developerAssets.sdkReleases[releaseID]
	if !ok || release.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := memoryClone(m.developerAssets.sdkReleaseEvents[releaseID])
	sort.Slice(result, func(i, j int) bool { return result[i].ObservedAt.After(result[j].ObservedAt) })
	return result, nil
}

func (m *Memory) AppendSDKReleaseLifecycleEvent(_ context.Context, deploymentID string, mutation SDKReleaseLifecycleMutation) (model.SDKReleaseLifecycleEvent, error) {
	_, _, outcome, err := prepareSDKReleaseLifecycleMutation(deploymentID, mutation)
	if err != nil {
		return model.SDKReleaseLifecycleEvent{}, err
	}
	value, audit := mutation.Event, mutation.Audit
	m.mu.Lock()
	defer m.mu.Unlock()
	release, ok := m.developerAssets.sdkReleases[value.SDKReleaseID]
	if !ok || release.DeploymentID != deploymentID {
		return model.SDKReleaseLifecycleEvent{}, ErrNotFound
	}
	var existingAudit *model.AuditEvent
	for _, current := range m.audit {
		if current.ID == audit.ID {
			copy := current
			existingAudit = &copy
			break
		}
	}
	for _, current := range m.developerAssets.sdkReleaseEvents[value.SDKReleaseID] {
		if current.ID == value.ID {
			auditEventID := ""
			if existingAudit != nil {
				auditEventID, _ = existingAudit.Current["sdk_release_lifecycle_event_id"].(string)
			}
			if existingAudit != nil && current.SDKReleaseID == value.SDKReleaseID && current.Lifecycle == value.Lifecycle &&
				current.Reason == value.Reason && current.ObservedSourceURI == value.ObservedSourceURI &&
				current.ObservedAt.Equal(value.ObservedAt) && current.RecordedBy == value.RecordedBy &&
				existingAudit.ProductID == deploymentID && existingAudit.Action == audit.Action &&
				existingAudit.TargetType == audit.TargetType && existingAudit.TargetID == audit.TargetID && auditEventID == value.ID {
				return memoryClone(current), nil
			}
			return model.SDKReleaseLifecycleEvent{}, ErrConflict
		}
		if current.Lifecycle == value.Lifecycle && current.ObservedAt.Equal(value.ObservedAt) {
			return model.SDKReleaseLifecycleEvent{}, ErrConflict
		}
	}
	if existingAudit != nil {
		return model.SDKReleaseLifecycleEvent{}, ErrConflict
	}
	now := time.Now().UTC()
	value.CreatedAt = now
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = now
	}
	audit.Outcome = outcome
	m.developerAssets.sdkReleaseEvents[value.SDKReleaseID] = append(m.developerAssets.sdkReleaseEvents[value.SDKReleaseID], memoryClone(value))
	m.audit = append(m.audit, memoryClone(audit))
	return value, nil
}

func (m *Memory) SDKContentCandidates(_ context.Context, deploymentID, releaseID string) ([]model.SDKContentCandidate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	release, ok := m.developerAssets.sdkReleases[releaseID]
	if !ok || release.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.SDKContentCandidate, 0, len(m.developerAssets.sdkCandidateIDs[releaseID]))
	for _, id := range m.developerAssets.sdkCandidateIDs[releaseID] {
		result = append(result, memoryClone(m.developerAssets.sdkCandidates[id].Candidate))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) SDKContentCandidate(_ context.Context, deploymentID, id string) (SDKContentCandidateRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkCandidates[id]
	if !ok || value.Candidate.DeploymentID != deploymentID {
		return SDKContentCandidateRecord{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateSDKContentCandidate(_ context.Context, value SDKContentCandidateRecord) (model.SDKContentCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createSDKContentCandidateLocked(value)
}

func (m *Memory) createSDKContentCandidateLocked(value SDKContentCandidateRecord) (model.SDKContentCandidate, error) {
	if err := ValidateSDKContentCandidateGraph(value); err != nil {
		return model.SDKContentCandidate{}, ErrConflict
	}
	release, ok := m.developerAssets.sdkReleases[value.Candidate.SDKReleaseID]
	run, runOK := m.developerAssets.ingestionRuns[value.Candidate.IngestionRunID]
	if !ok || !runOK || release.DeploymentID != value.Candidate.DeploymentID || run.DeploymentID != value.Candidate.DeploymentID || run.AssetKind != model.DeveloperAssetSDK || run.TargetID != value.Candidate.SDKReleaseID {
		return model.SDKContentCandidate{}, ErrNotFound
	}
	if _, exists := m.developerAssets.sdkCandidates[value.Candidate.ID]; exists {
		return model.SDKContentCandidate{}, ErrConflict
	}
	for _, current := range m.developerAssets.sdkCandidates {
		if current.Candidate.SDKReleaseID == value.Candidate.SDKReleaseID && (current.Candidate.IngestionRunID == value.Candidate.IngestionRunID || current.Candidate.ContentHash == value.Candidate.ContentHash) {
			return model.SDKContentCandidate{}, ErrConflict
		}
	}
	value.Candidate.CreatedAt = time.Now().UTC()
	m.developerAssets.sdkCandidates[value.Candidate.ID] = memoryClone(value)
	m.developerAssets.sdkCandidateIDs[value.Candidate.SDKReleaseID] = append(m.developerAssets.sdkCandidateIDs[value.Candidate.SDKReleaseID], value.Candidate.ID)
	return value.Candidate, nil
}

func (m *Memory) FinalizeSDKContentIngestion(_ context.Context, value SDKContentIngestionFinalization) (model.SDKContentCandidate, model.DeveloperAssetIngestionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// These are the only maps touched by the helpers below. Snapshot them as an
	// in-memory transaction savepoint while holding the mutex.
	candidatesBefore := make(map[string]SDKContentCandidateRecord, len(m.developerAssets.sdkCandidates))
	for id, candidate := range m.developerAssets.sdkCandidates {
		candidatesBefore[id] = memoryClone(candidate)
	}
	candidateIDsBefore := make(map[string][]string, len(m.developerAssets.sdkCandidateIDs))
	for releaseID, ids := range m.developerAssets.sdkCandidateIDs {
		candidateIDsBefore[releaseID] = append([]string(nil), ids...)
	}
	stagesBefore := make(map[string]model.DeveloperAssetIngestionStage, len(m.developerAssets.ingestionStages[value.Run.ID]))
	for id, stage := range m.developerAssets.ingestionStages[value.Run.ID] {
		stagesBefore[id] = memoryClone(stage)
	}
	_, stagesExisted := m.developerAssets.ingestionStages[value.Run.ID]
	runBefore, runExisted := m.developerAssets.ingestionRuns[value.Run.ID]
	rollback := func(err error) (model.SDKContentCandidate, model.DeveloperAssetIngestionRun, error) {
		m.developerAssets.sdkCandidates = candidatesBefore
		m.developerAssets.sdkCandidateIDs = candidateIDsBefore
		if stagesExisted {
			m.developerAssets.ingestionStages[value.Run.ID] = stagesBefore
		} else {
			delete(m.developerAssets.ingestionStages, value.Run.ID)
		}
		if runExisted {
			m.developerAssets.ingestionRuns[value.Run.ID] = runBefore
		} else {
			delete(m.developerAssets.ingestionRuns, value.Run.ID)
		}
		return model.SDKContentCandidate{}, model.DeveloperAssetIngestionRun{}, err
	}
	if value.ExpectedRunState != model.DeveloperAssetIngestionRunning ||
		value.Run.State != model.DeveloperAssetIngestionReviewReady ||
		value.Candidate.Candidate.IngestionRunID != value.Run.ID ||
		value.Candidate.Candidate.DeploymentID != value.Run.DeploymentID ||
		value.Candidate.Candidate.SDKReleaseID != value.Run.TargetID ||
		value.Run.FinishedAt == nil {
		return rollback(ErrConflict)
	}
	created, err := m.createSDKContentCandidateLocked(value.Candidate)
	if err != nil {
		return rollback(err)
	}
	for _, stage := range value.Stages {
		if stage.IngestionRunID != value.Run.ID || stage.Attempt != value.Run.Attempt {
			return rollback(ErrConflict)
		}
		if _, err := m.saveDeveloperAssetIngestionStageLocked(stage, ""); err != nil {
			return rollback(err)
		}
	}
	updated, err := m.transitionDeveloperAssetIngestionRunLocked(value.Run, value.ExpectedRunState)
	if err != nil {
		return rollback(err)
	}
	return created, updated, nil
}

func (m *Memory) SDKContentPublications(_ context.Context, deploymentID, releaseID string) ([]model.SDKContentPublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	release, ok := m.developerAssets.sdkReleases[releaseID]
	if !ok || release.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.SDKContentPublication, 0, len(m.developerAssets.sdkPublicationIDs[releaseID]))
	for _, id := range m.developerAssets.sdkPublicationIDs[releaseID] {
		result = append(result, memoryClone(m.developerAssets.sdkPublications[id].Publication))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) SDKContentPublication(_ context.Context, deploymentID, id string) (SDKContentPublicationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkPublications[id]
	if !ok || value.Publication.DeploymentID != deploymentID {
		return SDKContentPublicationRecord{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) PublishSDKContentCandidate(_ context.Context, value SDKContentPublicationRecord) (model.SDKContentPublication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate, ok := m.developerAssets.sdkCandidates[value.Publication.SDKContentCandidateID]
	if !ok || candidate.Candidate.DeploymentID != value.Publication.DeploymentID || candidate.Candidate.SDKReleaseID != value.Publication.SDKReleaseID || candidate.Candidate.ContentHash != value.Publication.ContentHash || candidate.Candidate.Visibility != value.Publication.Visibility {
		return model.SDKContentPublication{}, ErrNotFound
	}
	run := m.developerAssets.ingestionRuns[candidate.Candidate.IngestionRunID]
	if run.State != model.DeveloperAssetIngestionReviewReady || run.AssetKind != model.DeveloperAssetSDK || run.TargetID != value.Publication.SDKReleaseID || run.FailedCount != 0 || run.SkippedCount != 0 || run.QuarantinedCount != 0 {
		return model.SDKContentPublication{}, ErrConflict
	}
	if _, exists := m.developerAssets.sdkPublications[value.Publication.ID]; exists {
		return model.SDKContentPublication{}, ErrConflict
	}
	for _, current := range m.developerAssets.sdkPublications {
		if current.Publication.SDKReleaseID == value.Publication.SDKReleaseID && (current.Publication.SDKContentCandidateID == value.Publication.SDKContentCandidateID || current.Publication.ContentHash == value.Publication.ContentHash) {
			return model.SDKContentPublication{}, ErrConflict
		}
	}
	files := make(map[string]model.SDKPublicationFile, len(candidate.Files))
	for _, file := range candidate.Files {
		files[file.ID] = file
	}
	fileOrdinals := make(map[int]bool)
	selectedFiles := make(map[string]bool, len(value.FileSelections))
	for _, selection := range value.FileSelections {
		file, exists := files[selection.SDKPublicationFileID]
		if !exists || selectedFiles[selection.SDKPublicationFileID] || selection.DeploymentID != value.Publication.DeploymentID || selection.SDKContentCandidateID != candidate.Candidate.ID || selection.SDKContentPublicationID != value.Publication.ID || selection.ContentHash != file.ContentHash {
			return model.SDKContentPublication{}, ErrConflict
		}
		selectedFiles[selection.SDKPublicationFileID] = true
		if selection.Decision == "included" {
			if selection.Ordinal == nil || selection.Reason != "" || fileOrdinals[*selection.Ordinal] {
				return model.SDKContentPublication{}, ErrConflict
			}
			fileOrdinals[*selection.Ordinal] = true
		} else if (selection.Decision != "excluded" && selection.Decision != "quarantined") || selection.Ordinal != nil || selection.Reason == "" {
			return model.SDKContentPublication{}, ErrConflict
		}
	}
	if len(selectedFiles) != len(files) {
		return model.SDKContentPublication{}, ErrConflict
	}
	samples := make(map[string]model.SDKCodeSample, len(candidate.Samples))
	for _, sample := range candidate.Samples {
		samples[sample.ID] = sample
	}
	sampleOrdinals := make(map[int]bool)
	selectedSamples := make(map[string]bool, len(value.SampleSelections))
	for _, selection := range value.SampleSelections {
		sample, exists := samples[selection.SDKCodeSampleID]
		if !exists || selectedSamples[selection.SDKCodeSampleID] || selection.DeploymentID != value.Publication.DeploymentID || selection.SDKContentPublicationID != value.Publication.ID || !selection.ValidFor(sample) {
			return model.SDKContentPublication{}, ErrConflict
		}
		selectedSamples[selection.SDKCodeSampleID] = true
		if selection.Ordinal != nil {
			if sampleOrdinals[*selection.Ordinal] {
				return model.SDKContentPublication{}, ErrConflict
			}
			sampleOrdinals[*selection.Ordinal] = true
		}
	}
	if len(selectedSamples) != len(samples) {
		return model.SDKContentPublication{}, ErrConflict
	}
	if (candidate.Map == nil) != (value.Map == nil) || (value.Map == nil) != (value.PublishedMap == nil) {
		return model.SDKContentPublication{}, ErrConflict
	}
	if value.Map != nil {
		publishedMap := value.PublishedMap
		if candidate.Map == nil || publishedMap == nil || publishedMap.ID == candidate.Map.ID || publishedMap.MapVersion == candidate.Map.MapVersion ||
			publishedMap.DeploymentID != value.Publication.DeploymentID || publishedMap.SDKContentCandidateID != candidate.Candidate.ID ||
			publishedMap.ID != value.Map.SDKMapID || publishedMap.ContentHash == "" || publishedMap.ContentHash != value.Map.ContentHash ||
			publishedMap.MapVersion == "" || publishedMap.AgentMarkdown == "" || value.Map.DeploymentID != value.Publication.DeploymentID ||
			value.Map.SDKContentPublicationID != value.Publication.ID || value.Map.SDKContentCandidateID != candidate.Candidate.ID {
			return model.SDKContentPublication{}, ErrConflict
		}
	}
	release, releaseOK := m.developerAssets.sdkReleases[value.Publication.SDKReleaseID]
	packageValue, packageOK := m.developerAssets.sdkPackages[release.SDKPackageID]
	if !releaseOK || !packageOK || ValidateReviewedSDKPublicationMap(packageValue, release, candidate, value) != nil {
		return model.SDKContentPublication{}, ErrConflict
	}
	value.Publication.Revision = int64(len(m.developerAssets.sdkPublicationIDs[value.Publication.SDKReleaseID]) + 1)
	now := time.Now().UTC()
	if value.Publication.PublishedAt.IsZero() {
		value.Publication.PublishedAt = now
	}
	value.Publication.CreatedAt = now
	if value.PublishedMap != nil && value.PublishedMap.CreatedAt.IsZero() {
		value.PublishedMap.CreatedAt = now
	}
	m.developerAssets.sdkPublications[value.Publication.ID] = memoryClone(value)
	m.developerAssets.sdkPublicationIDs[value.Publication.SDKReleaseID] = append(m.developerAssets.sdkPublicationIDs[value.Publication.SDKReleaseID], value.Publication.ID)
	run.State, run.LeaseOwner, run.LeaseExpiresAt, run.HeartbeatAt = model.DeveloperAssetIngestionPublished, "", nil, nil
	run.FinishedAt = &now
	m.developerAssets.ingestionRuns[run.ID] = memoryClone(run)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value.Publication, nil
}

func sdkAssertionIndexKey(apiID, releaseID string) string { return apiID + "\x00" + releaseID }

func (m *Memory) SDKCompatibilityAssertions(_ context.Context, deploymentID, apiID, releaseID string) ([]model.SDKCompatibilityAssertion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if integration, ok := m.integrations[apiID]; !ok || integration.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.SDKCompatibilityAssertion, 0)
	for _, id := range m.developerAssets.sdkAssertionIDs[sdkAssertionIndexKey(apiID, releaseID)] {
		result = append(result, memoryClone(m.developerAssets.sdkAssertions[id]))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) SDKCompatibilityAssertion(_ context.Context, deploymentID, id string) (model.SDKCompatibilityAssertion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkAssertions[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.SDKCompatibilityAssertion{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateSDKCompatibilityAssertion(_ context.Context, value model.SDKCompatibilityAssertion) (model.SDKCompatibilityAssertion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, apiOK := m.integrations[value.APIID]
	release, releaseOK := m.developerAssets.sdkReleases[value.SDKReleaseID]
	if !apiOK || !releaseOK || integration.DeploymentID != value.DeploymentID || release.DeploymentID != value.DeploymentID {
		return model.SDKCompatibilityAssertion{}, ErrNotFound
	}
	if _, exists := m.developerAssets.sdkAssertions[value.ID]; exists {
		return model.SDKCompatibilityAssertion{}, ErrConflict
	}
	if value.SupersedesAssertionID != "" {
		previous, ok := m.developerAssets.sdkAssertions[value.SupersedesAssertionID]
		if !ok || previous.APIID != value.APIID || previous.SDKReleaseID != value.SDKReleaseID {
			return model.SDKCompatibilityAssertion{}, ErrNotFound
		}
	}
	value.CreatedAt = time.Now().UTC()
	m.developerAssets.sdkAssertions[value.ID] = memoryClone(value)
	key := sdkAssertionIndexKey(value.APIID, value.SDKReleaseID)
	m.developerAssets.sdkAssertionIDs[key] = append(m.developerAssets.sdkAssertionIDs[key], value.ID)
	return value, nil
}
