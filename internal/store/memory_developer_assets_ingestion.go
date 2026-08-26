package store

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) DeveloperAssetIngestionRuns(_ context.Context, deploymentID string, kind model.DeveloperAssetKind, targetKey string) ([]model.DeveloperAssetIngestionRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.DeveloperAssetIngestionRun, 0)
	for _, value := range m.developerAssets.ingestionRuns {
		if value.DeploymentID == deploymentID && (kind == "" || value.AssetKind == kind) && (targetKey == "" || value.TargetKey == targetKey) {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].QueuedAt.After(result[j].QueuedAt) })
	return result, nil
}

func (m *Memory) DeveloperAssetIngestionRun(_ context.Context, deploymentID, id string) (model.DeveloperAssetIngestionRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.ingestionRuns[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.DeveloperAssetIngestionRun{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func developerAssetRunActive(state model.DeveloperAssetIngestionState) bool {
	return state == model.DeveloperAssetIngestionQueued || state == model.DeveloperAssetIngestionRunning
}

func developerAssetRunShapeValid(value model.DeveloperAssetIngestionRun) bool {
	return value.Valid() && value.DiscoveredCount >= 0 && value.AcquiredCount >= 0 && value.FailedCount >= 0 &&
		value.SkippedCount >= 0 && value.QuarantinedCount >= 0 && (value.FinishedAt == nil || value.StartedAt != nil)
}

func (m *Memory) validateDeveloperAssetRunTargetLocked(value model.DeveloperAssetIngestionRun) error {
	switch value.AssetKind {
	case model.DeveloperAssetDocumentation:
		if value.TargetID != value.SourceID {
			return ErrConflict
		}
		if _, ok := m.sources[value.DeploymentID][value.SourceID]; !ok {
			return ErrNotFound
		}
	case model.DeveloperAssetContract:
		if _, ok := m.sources[value.DeploymentID][value.SourceID]; !ok {
			return ErrNotFound
		}
		attached := false
		for _, binding := range m.developerAssets.contractSources {
			if binding.DeploymentID == value.DeploymentID && binding.APIContractID == value.TargetID && binding.SourceID == value.SourceID && binding.Lifecycle == "attached" {
				attached = true
				break
			}
		}
		if !attached {
			return ErrConflict
		}
	case model.DeveloperAssetSDK:
		release, ok := m.developerAssets.sdkReleases[value.TargetID]
		if !ok || release.DeploymentID != value.DeploymentID {
			return ErrNotFound
		}
	default:
		return ErrConflict
	}
	return nil
}

func (m *Memory) CreateDeveloperAssetIngestionRun(_ context.Context, value model.DeveloperAssetIngestionRun) (model.DeveloperAssetIngestionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.DeploymentID || m.deployment.OrganisationID != value.OrganisationID {
		return model.DeveloperAssetIngestionRun{}, ErrNotFound
	}
	if value.State == "" {
		value.State = model.DeveloperAssetIngestionQueued
	}
	if value.Attempt == 0 {
		value.Attempt = 1
	}
	if value.QueuedAt.IsZero() {
		value.QueuedAt = time.Now().UTC()
	}
	if !developerAssetRunShapeValid(value) {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if err := m.validateDeveloperAssetRunTargetLocked(value); err != nil {
		return model.DeveloperAssetIngestionRun{}, err
	}
	if _, exists := m.developerAssets.ingestionRuns[value.ID]; exists {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if developerAssetRunActive(value.State) {
		for _, current := range m.developerAssets.ingestionRuns {
			if current.DeploymentID == value.DeploymentID && current.AssetKind == value.AssetKind && current.TargetKey == value.TargetKey && developerAssetRunActive(current.State) {
				return model.DeveloperAssetIngestionRun{}, ErrConflict
			}
		}
	}
	m.developerAssets.ingestionRuns[value.ID] = memoryClone(value)
	return value, nil
}

func (m *Memory) TransitionDeveloperAssetIngestionRun(_ context.Context, value model.DeveloperAssetIngestionRun, expected model.DeveloperAssetIngestionState) (model.DeveloperAssetIngestionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transitionDeveloperAssetIngestionRunLocked(value, expected)
}

func (m *Memory) transitionDeveloperAssetIngestionRunLocked(value model.DeveloperAssetIngestionRun, expected model.DeveloperAssetIngestionState) (model.DeveloperAssetIngestionRun, error) {
	current, ok := m.developerAssets.ingestionRuns[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.DeveloperAssetIngestionRun{}, ErrNotFound
	}
	if current.State != expected {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if value.State != current.State && !current.State.CanTransitionTo(value.State) {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if value.DeploymentID != current.DeploymentID || value.OrganisationID != current.OrganisationID || value.AssetKind != current.AssetKind ||
		value.TargetID != current.TargetID || value.TargetKey != current.TargetKey || value.SourceID != current.SourceID ||
		value.Attempt != current.Attempt || value.Versions != current.Versions {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	resolvedInputChanged := value.ResolvedSourceURI != current.ResolvedSourceURI ||
		value.ResolvedSourceRevision != current.ResolvedSourceRevision || value.ResolvedSourceHash != current.ResolvedSourceHash ||
		!bytes.Equal(value.RawManifest, current.RawManifest) || value.RawManifestHash != current.RawManifestHash
	if resolvedInputChanged {
		_, hasDocumentationOutput := m.developerAssets.documentationOutputs[current.ID]
		hasCandidateOutput := hasDocumentationOutput
		for _, candidate := range m.developerAssets.contractCandidates {
			hasCandidateOutput = hasCandidateOutput || candidate.Candidate.IngestionRunID == current.ID
		}
		for _, candidate := range m.developerAssets.sdkCandidates {
			hasCandidateOutput = hasCandidateOutput || candidate.Candidate.IngestionRunID == current.ID
		}
		terminalReview := current.State == model.DeveloperAssetIngestionReviewReady || current.State == model.DeveloperAssetIngestionFailed ||
			current.State == model.DeveloperAssetIngestionCancelled || current.State == model.DeveloperAssetIngestionPublished
		if hasCandidateOutput || terminalReview {
			return model.DeveloperAssetIngestionRun{}, ErrConflict
		}
	}
	if !developerAssetRunShapeValid(value) {
		return model.DeveloperAssetIngestionRun{}, ErrConflict
	}
	if developerAssetRunActive(value.State) && !developerAssetRunActive(current.State) {
		for id, other := range m.developerAssets.ingestionRuns {
			if id != value.ID && other.DeploymentID == value.DeploymentID && other.AssetKind == value.AssetKind && other.TargetKey == value.TargetKey && developerAssetRunActive(other.State) {
				return model.DeveloperAssetIngestionRun{}, ErrConflict
			}
		}
	}
	value.QueuedAt = current.QueuedAt
	m.developerAssets.ingestionRuns[value.ID] = memoryClone(value)
	return value, nil
}

func (m *Memory) DeveloperAssetIngestionStages(_ context.Context, runID string) ([]model.DeveloperAssetIngestionStage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.developerAssets.ingestionRuns[runID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.DeveloperAssetIngestionStage, 0, len(m.developerAssets.ingestionStages[runID]))
	for _, value := range m.developerAssets.ingestionStages[runID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].Attempt < result[j].Attempt
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (m *Memory) SaveDeveloperAssetIngestionStage(_ context.Context, value model.DeveloperAssetIngestionStage, expectedState string) (model.DeveloperAssetIngestionStage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveDeveloperAssetIngestionStageLocked(value, expectedState)
}

func (m *Memory) saveDeveloperAssetIngestionStageLocked(value model.DeveloperAssetIngestionStage, expectedState string) (model.DeveloperAssetIngestionStage, error) {
	if _, ok := m.developerAssets.ingestionRuns[value.IngestionRunID]; !ok {
		return model.DeveloperAssetIngestionStage{}, ErrNotFound
	}
	stages := m.developerAssets.ingestionStages[value.IngestionRunID]
	if stages == nil {
		stages = make(map[string]model.DeveloperAssetIngestionStage)
		m.developerAssets.ingestionStages[value.IngestionRunID] = stages
	}
	now := time.Now().UTC()
	current, exists := stages[value.ID]
	if !exists {
		if expectedState != "" {
			return model.DeveloperAssetIngestionStage{}, ErrNotFound
		}
		for _, other := range stages {
			if other.Name == value.Name && other.Attempt == value.Attempt {
				return model.DeveloperAssetIngestionStage{}, ErrConflict
			}
		}
		value.CreatedAt, value.UpdatedAt = now, now
	} else {
		if current.State != expectedState {
			return model.DeveloperAssetIngestionStage{}, ErrConflict
		}
		value.IngestionRunID, value.Name, value.Attempt = current.IngestionRunID, current.Name, current.Attempt
		value.CreatedAt, value.UpdatedAt = current.CreatedAt, now
	}
	stages[value.ID] = memoryClone(value)
	return value, nil
}

func (m *Memory) DocumentationIngestionOutput(_ context.Context, deploymentID, runID string) (DocumentationIngestionOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.developerAssets.ingestionRuns[runID]
	if !ok || run.DeploymentID != deploymentID {
		return DocumentationIngestionOutput{}, ErrNotFound
	}
	value, ok := m.developerAssets.documentationOutputs[runID]
	if !ok {
		return DocumentationIngestionOutput{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveDocumentationIngestionOutput(_ context.Context, deploymentID string, value DocumentationIngestionOutput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value.Documents) == 0 {
		return ErrConflict
	}
	runID := value.Documents[0].IngestionRunID
	run, ok := m.developerAssets.ingestionRuns[runID]
	if !ok || run.DeploymentID != deploymentID || run.AssetKind != model.DeveloperAssetDocumentation {
		return ErrNotFound
	}
	if _, exists := m.developerAssets.documentationOutputs[runID]; exists {
		return ErrConflict
	}
	documents := make(map[string]bool, len(value.Documents))
	for _, document := range value.Documents {
		if document.DeploymentID != deploymentID || document.IngestionRunID != runID || documents[document.ID] {
			return ErrConflict
		}
		documents[document.ID] = true
	}
	sections := make(map[string]bool, len(value.Sections))
	for _, section := range value.Sections {
		if section.DeploymentID != deploymentID || !documents[section.DocumentationDocumentID] || sections[section.ID] {
			return ErrConflict
		}
		sections[section.ID] = true
	}
	for _, section := range value.Sections {
		if section.ParentSectionID != "" && !sections[section.ParentSectionID] {
			return ErrConflict
		}
	}
	if value.Map != nil && (value.Map.DeploymentID != deploymentID || value.Map.IngestionRunID != runID || value.Map.DocumentationCollectionRevisionID != "") {
		return ErrConflict
	}
	if value.Map != nil && value.Map.CreatedAt.IsZero() {
		value.Map.CreatedAt = time.Now().UTC()
	}
	m.developerAssets.documentationOutputs[runID] = memoryClone(value)
	return nil
}

func (m *Memory) documentationCandidateRecordLocked(document model.DocumentationDocument) DocumentationCandidateRecord {
	output := m.developerAssets.documentationOutputs[document.IngestionRunID]
	record := DocumentationCandidateRecord{
		Document:                    memoryClone(document),
		Sections:                    []model.DocumentationSection{},
		Run:                         memoryClone(m.developerAssets.ingestionRuns[document.IngestionRunID]),
		SourcePublicationSelections: []model.SourcePublicationDocumentSelection{},
	}
	if output.Map != nil {
		value := memoryClone(*output.Map)
		record.DocumentationMap = &value
	}
	for _, section := range output.Sections {
		if section.DocumentationDocumentID == document.ID {
			record.Sections = append(record.Sections, memoryClone(section))
		}
	}
	sort.Slice(record.Sections, func(i, j int) bool { return record.Sections[i].Ordinal < record.Sections[j].Ordinal })
	for publicationID, review := range m.developerAssets.sourcePublicationReviews {
		publication, exists := m.sourcePublications[document.DeploymentID][publicationID]
		if !exists || publication.ProductID != document.DeploymentID {
			continue
		}
		for _, selection := range review.Selections {
			if selection.DeploymentID == document.DeploymentID && selection.DocumentationDocumentID == document.ID && selection.SourcePublicationID == publicationID {
				record.SourcePublicationSelections = append(record.SourcePublicationSelections, memoryClone(selection))
			}
		}
	}
	sortSourcePublicationSelectionsNewestFirst(record.SourcePublicationSelections)
	return record
}

func (m *Memory) DocumentationCandidateDocuments(_ context.Context, query DocumentationCandidateQuery) (DocumentationCandidatePage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	limit := boundedDeveloperAssetResultLimit(query.Limit)
	if limit == 0 {
		return DocumentationCandidatePage{Items: []DocumentationCandidateRecord{}}, nil
	}
	publicationDocuments := map[string]bool(nil)
	if query.SourcePublicationID != "" {
		review, ok := m.developerAssets.sourcePublicationReviews[query.SourcePublicationID]
		if !ok {
			return DocumentationCandidatePage{}, ErrNotFound
		}
		publicationDocuments = make(map[string]bool, len(review.Selections))
		for _, selection := range review.Selections {
			publicationDocuments[selection.DocumentationDocumentID] = true
		}
	}
	needle := strings.ToLower(strings.TrimSpace(query.QueryText))
	result := make([]DocumentationCandidateRecord, 0)
	for runID, output := range m.developerAssets.documentationOutputs {
		run := m.developerAssets.ingestionRuns[runID]
		if run.DeploymentID != query.DeploymentID || (query.IngestionRunID != "" && runID != query.IngestionRunID) || (query.SourceID != "" && run.SourceID != query.SourceID) {
			continue
		}
		for _, document := range output.Documents {
			if publicationDocuments != nil && !publicationDocuments[document.ID] {
				continue
			}
			matched := needle == "" || strings.Contains(strings.ToLower(document.Title+" "+document.NormalizedMarkdown+" "+document.SourcePath), needle)
			if !matched {
				for _, section := range output.Sections {
					if section.DocumentationDocumentID == document.ID && strings.Contains(strings.ToLower(section.Heading+" "+section.NormalizedText), needle) {
						matched = true
						break
					}
				}
			}
			if matched {
				result = append(result, m.documentationCandidateRecordLocked(document))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Run.QueuedAt.Equal(result[j].Run.QueuedAt) {
			if result[i].Run.ID == result[j].Run.ID {
				if result[i].Document.Ordinal == result[j].Document.Ordinal {
					return result[i].Document.ID < result[j].Document.ID
				}
				return result[i].Document.Ordinal < result[j].Document.Ordinal
			}
			return result[i].Run.ID > result[j].Run.ID
		}
		return result[i].Run.QueuedAt.After(result[j].Run.QueuedAt)
	})
	total := len(result)
	offset := max(query.Offset, 0)
	if offset >= total {
		return DocumentationCandidatePage{Items: []DocumentationCandidateRecord{}, Total: total}, nil
	}
	end := min(offset+limit, total)
	return DocumentationCandidatePage{Items: result[offset:end], Total: total, HasMore: end < total}, nil
}

func (m *Memory) DocumentationCandidateDocument(_ context.Context, deploymentID, documentID string) (DocumentationCandidateRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, output := range m.developerAssets.documentationOutputs {
		for _, document := range output.Documents {
			if document.ID == documentID && document.DeploymentID == deploymentID {
				return m.documentationCandidateRecordLocked(document), nil
			}
		}
	}
	return DocumentationCandidateRecord{}, ErrNotFound
}

func (m *Memory) DocumentationCandidateSection(_ context.Context, deploymentID, sectionID string) (model.DocumentationSection, DocumentationCandidateRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, output := range m.developerAssets.documentationOutputs {
		for _, section := range output.Sections {
			if section.ID != sectionID || section.DeploymentID != deploymentID {
				continue
			}
			for _, document := range output.Documents {
				if document.ID == section.DocumentationDocumentID {
					return memoryClone(section), m.documentationCandidateRecordLocked(document), nil
				}
			}
		}
	}
	return model.DocumentationSection{}, DocumentationCandidateRecord{}, ErrNotFound
}

func (m *Memory) SourcePublicationDocumentationReview(_ context.Context, deploymentID, publicationID string) (SourcePublicationDocumentationReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	publication, ok := m.sourcePublications[deploymentID][publicationID]
	if !ok || publication.ProductID != deploymentID {
		return SourcePublicationDocumentationReview{}, ErrNotFound
	}
	value, ok := m.developerAssets.sourcePublicationReviews[publicationID]
	if !ok {
		return SourcePublicationDocumentationReview{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveSourcePublicationDocumentationReview(_ context.Context, deploymentID string, value SourcePublicationDocumentationReview) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value.Selections) == 0 {
		return ErrConflict
	}
	publicationID := value.Selections[0].SourcePublicationID
	publication, ok := m.sourcePublications[deploymentID][publicationID]
	if !ok {
		return ErrNotFound
	}
	if _, exists := m.developerAssets.sourcePublicationReviews[publicationID]; exists {
		return ErrConflict
	}
	selected := make(map[string]bool, len(value.Selections))
	includedOrdinals := make(map[int]bool)
	runID := ""
	for _, selection := range value.Selections {
		if selection.DeploymentID != deploymentID || selection.SourcePublicationID != publicationID || selected[selection.DocumentationDocumentID] || strings.TrimSpace(selection.ReviewedBy) == "" || selection.ReviewedAt.IsZero() {
			return ErrConflict
		}
		switch selection.Decision {
		case "included":
			if selection.Ordinal == nil || selection.Reason != "" || includedOrdinals[*selection.Ordinal] {
				return ErrConflict
			}
			includedOrdinals[*selection.Ordinal] = true
		case "excluded", "quarantined":
			if selection.Ordinal != nil || strings.TrimSpace(selection.Reason) == "" {
				return ErrConflict
			}
		default:
			return ErrConflict
		}
		selected[selection.DocumentationDocumentID] = true
		matched := false
		for candidateRunID, output := range m.developerAssets.documentationOutputs {
			for _, document := range output.Documents {
				if document.ID == selection.DocumentationDocumentID {
					if selection.ContentHash != document.ContentHash || (publication.Visibility == model.VisibilityPublic && document.Visibility != model.VisibilityPublic) {
						return ErrConflict
					}
					if runID != "" && runID != candidateRunID {
						return ErrConflict
					}
					runID, matched = candidateRunID, true
					break
				}
			}
		}
		if !matched {
			return ErrNotFound
		}
	}
	output := m.developerAssets.documentationOutputs[runID]
	if len(selected) != len(output.Documents) {
		return ErrConflict
	}
	if output.Map == nil && value.MapLink != nil {
		return ErrConflict
	}
	if output.Map != nil {
		if value.MapLink == nil || value.MapLink.DeploymentID != deploymentID || value.MapLink.SourcePublicationID != publicationID ||
			value.MapLink.DocumentationMapID != output.Map.ID || value.MapLink.ContentHash != output.Map.ContentHash ||
			(publication.Visibility == model.VisibilityPublic && output.Map.Visibility != model.VisibilityPublic) {
			return ErrConflict
		}
	}
	run := m.developerAssets.ingestionRuns[runID]
	if run.State != model.DeveloperAssetIngestionReviewReady || run.AssetKind != model.DeveloperAssetDocumentation || run.SourceID != publication.SourceID || run.TargetID != publication.SourceID || run.FailedCount != 0 || run.SkippedCount != 0 || run.QuarantinedCount != 0 {
		return ErrConflict
	}
	m.developerAssets.sourcePublicationReviews[publicationID] = memoryClone(value)
	now := time.Now().UTC()
	run.State, run.LeaseOwner, run.LeaseExpiresAt, run.HeartbeatAt = model.DeveloperAssetIngestionPublished, "", nil, nil
	run.FinishedAt = &now
	m.developerAssets.ingestionRuns[run.ID] = memoryClone(run)
	return nil
}
