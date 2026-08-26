package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) DocumentationCollections(_ context.Context, deploymentID string) ([]model.DocumentationCollection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.DocumentationCollection, 0)
	for _, value := range m.developerAssets.documentationCollections {
		if value.DeploymentID == deploymentID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) DocumentationCollection(_ context.Context, deploymentID, id string) (model.DocumentationCollection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.documentationCollections[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.DocumentationCollection{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) validateDocumentationRevisionLocked(collection model.DocumentationCollection, record DocumentationCollectionRevisionRecord, revision int64) error {
	if record.Revision.ID == "" || record.Revision.DeploymentID != collection.DeploymentID || record.Revision.DocumentationCollectionID != collection.ID || record.Revision.Revision != revision {
		return ErrConflict
	}
	for _, current := range m.developerAssets.documentationRevisions {
		if current.Revision.DocumentationCollectionID == collection.ID && current.Revision.ContentHash == record.Revision.ContentHash {
			return ErrConflict
		}
	}
	ordinals := make(map[int]bool, len(record.Members))
	for _, member := range record.Members {
		if member.DocumentationCollectionRevisionID != record.Revision.ID || ordinals[member.Ordinal] {
			return ErrConflict
		}
		ordinals[member.Ordinal] = true
		switch member.Kind {
		case "source_publication":
			if member.SourcePublicationID == "" || member.DocumentationDocumentID != "" || member.DocumentationSectionID != "" {
				return ErrConflict
			}
		case "document":
			if member.SourcePublicationID != "" || member.DocumentationDocumentID == "" || member.DocumentationSectionID != "" {
				return ErrConflict
			}
		case "section":
			if member.SourcePublicationID != "" || member.DocumentationDocumentID != "" || member.DocumentationSectionID == "" {
				return ErrConflict
			}
		default:
			return ErrConflict
		}
	}
	if record.Map != nil && (record.Map.DeploymentID != collection.DeploymentID || record.Map.DocumentationCollectionRevisionID != record.Revision.ID || record.Map.IngestionRunID != "") {
		return ErrConflict
	}
	return nil
}

func (m *Memory) CreateDocumentationCollection(_ context.Context, value model.DocumentationCollection, record DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID || value.OrganisationID != m.deployment.OrganisationID {
		return model.DocumentationCollection{}, ErrNotFound
	}
	for _, current := range m.developerAssets.documentationCollections {
		if current.DeploymentID == value.DeploymentID && (current.ID == value.ID || current.Slug == value.Slug) {
			return model.DocumentationCollection{}, ErrConflict
		}
	}
	value.Revision = 1
	if value.Lifecycle == "" {
		value.Lifecycle = "active"
	}
	if err := snapshotDocumentationCollectionIdentity(value, &record.Revision); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := m.validateDocumentationRevisionLocked(value, record, 1); err != nil {
		return model.DocumentationCollection{}, err
	}
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	record.Revision.CreatedAt = now
	if record.Map != nil && record.Map.CreatedAt.IsZero() {
		record.Map.CreatedAt = now
	}
	if record.Revision.PublishedAt.IsZero() {
		record.Revision.PublishedAt = now
	}
	m.developerAssets.documentationCollections[value.ID] = memoryClone(value)
	m.developerAssets.documentationRevisions[record.Revision.ID] = memoryClone(record)
	m.developerAssets.documentationRevisionIDs[value.ID] = append(m.developerAssets.documentationRevisionIDs[value.ID], record.Revision.ID)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) ReviseDocumentationCollection(_ context.Context, value model.DocumentationCollection, expected int64, record DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.developerAssets.documentationCollections[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.DocumentationCollection{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.DocumentationCollection{}, ErrConflict
	}
	for id, other := range m.developerAssets.documentationCollections {
		if id != value.ID && other.DeploymentID == value.DeploymentID && other.Slug == value.Slug {
			return model.DocumentationCollection{}, ErrConflict
		}
	}
	value.OrganisationID, value.CreatedAt, value.Revision = current.OrganisationID, current.CreatedAt, expected+1
	if err := snapshotDocumentationCollectionIdentity(value, &record.Revision); err != nil {
		return model.DocumentationCollection{}, err
	}
	if err := m.validateDocumentationRevisionLocked(value, record, value.Revision); err != nil {
		return model.DocumentationCollection{}, err
	}
	now := time.Now().UTC()
	value.UpdatedAt, record.Revision.CreatedAt = now, now
	if record.Map != nil && record.Map.CreatedAt.IsZero() {
		record.Map.CreatedAt = now
	}
	if record.Revision.PublishedAt.IsZero() {
		record.Revision.PublishedAt = now
	}
	m.developerAssets.documentationCollections[value.ID] = memoryClone(value)
	m.developerAssets.documentationRevisions[record.Revision.ID] = memoryClone(record)
	m.developerAssets.documentationRevisionIDs[value.ID] = append(m.developerAssets.documentationRevisionIDs[value.ID], record.Revision.ID)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) DocumentationCollectionRevisions(_ context.Context, deploymentID, collectionID string) ([]model.DocumentationCollectionRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	collection, ok := m.developerAssets.documentationCollections[collectionID]
	if !ok || collection.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.DocumentationCollectionRevision, 0, len(m.developerAssets.documentationRevisionIDs[collectionID]))
	for _, id := range m.developerAssets.documentationRevisionIDs[collectionID] {
		result = append(result, memoryClone(m.developerAssets.documentationRevisions[id].Revision))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) DocumentationCollectionRevision(_ context.Context, deploymentID, id string) (DocumentationCollectionRevisionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.documentationRevisions[id]
	if !ok || value.Revision.DeploymentID != deploymentID {
		return DocumentationCollectionRevisionRecord{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) DeploymentDocumentationPublications(_ context.Context, deploymentID string) ([]model.DeploymentDocumentationPublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.DeploymentDocumentationPublication, 0)
	for _, value := range m.developerAssets.documentationPublications {
		if value.DeploymentID == deploymentID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) DeploymentDocumentationPublication(_ context.Context, deploymentID, id string) (model.DeploymentDocumentationPublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.documentationPublications[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.DeploymentDocumentationPublication{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) ActiveDeploymentDocumentationPublication(_ context.Context, deploymentID string) (model.DeploymentDocumentationPublication, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	head, ok := m.developerAssets.documentationHeads[deploymentID]
	if !ok {
		return model.DeploymentDocumentationPublication{}, 0, ErrNotFound
	}
	value := m.developerAssets.documentationPublications[head.PublicationID]
	return memoryClone(value), head.Revision, nil
}

func (m *Memory) PublishDeploymentDocumentation(_ context.Context, value model.DeploymentDocumentationPublication, expectedHeadRevision int64) (model.DeploymentDocumentationPublication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.DeploymentID {
		return model.DeploymentDocumentationPublication{}, ErrNotFound
	}
	head, hasHead := m.developerAssets.documentationHeads[value.DeploymentID]
	if (!hasHead && expectedHeadRevision != 0) || (hasHead && head.Revision != expectedHeadRevision) {
		return model.DeploymentDocumentationPublication{}, ErrConflict
	}
	if _, exists := m.developerAssets.documentationPublications[value.ID]; exists {
		return model.DeploymentDocumentationPublication{}, ErrConflict
	}
	wantRevision := expectedHeadRevision + 1
	if value.Revision != wantRevision {
		return model.DeploymentDocumentationPublication{}, ErrConflict
	}
	ordinals := make(map[int]bool, len(value.Members))
	for _, member := range value.Members {
		record, ok := m.developerAssets.documentationRevisions[member.DocumentationCollectionRevisionID]
		if !ok || record.Revision.DeploymentID != value.DeploymentID || record.Revision.ContentHash != member.ContentHash || record.Revision.Visibility != member.Visibility || ordinals[member.Ordinal] {
			return model.DeploymentDocumentationPublication{}, ErrConflict
		}
		if value.Visibility == model.VisibilityPublic && member.Visibility != model.VisibilityPublic {
			return model.DeploymentDocumentationPublication{}, ErrConflict
		}
		ordinals[member.Ordinal] = true
	}
	now := time.Now().UTC()
	if value.PublishedAt.IsZero() {
		value.PublishedAt = now
	}
	value.CreatedAt = now
	m.developerAssets.documentationPublications[value.ID] = memoryClone(value)
	m.developerAssets.documentationHeads[value.DeploymentID] = memoryDocumentationHead{PublicationID: value.ID, Revision: wantRevision}
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APIContracts(_ context.Context, deploymentID string) ([]model.APIContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.APIContract, 0)
	for _, value := range m.developerAssets.contracts {
		if value.DeploymentID == deploymentID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) APIContract(_ context.Context, deploymentID, id string) (model.APIContract, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.contracts[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.APIContract{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveAPIContract(_ context.Context, value model.APIContract, expected int64) (model.APIContract, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || value.DeploymentID != m.deployment.ID || value.OrganisationID != m.deployment.OrganisationID {
		return model.APIContract{}, ErrNotFound
	}
	for id, current := range m.developerAssets.contracts {
		if current.DeploymentID == value.DeploymentID && current.Slug == value.Slug && id != value.ID {
			return model.APIContract{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := m.developerAssets.contracts[value.ID]
	if !exists {
		if expected != 0 {
			return model.APIContract{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
		if value.Lifecycle == "" {
			value.Lifecycle = "active"
		}
	} else {
		if current.Revision != expected {
			return model.APIContract{}, ErrConflict
		}
		value.OrganisationID, value.CreatedAt = current.OrganisationID, current.CreatedAt
		value.Revision, value.UpdatedAt = expected+1, now
	}
	m.developerAssets.contracts[value.ID] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APIContractSources(_ context.Context, deploymentID, contractID string) ([]model.APIContractSource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contract, ok := m.developerAssets.contracts[contractID]
	if !ok || contract.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIContractSource, 0)
	for _, value := range m.developerAssets.contractSources {
		if value.DeploymentID == deploymentID && value.APIContractID == contractID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SourceRole == result[j].SourceRole {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].SourceRole < result[j].SourceRole
	})
	return result, nil
}

func (m *Memory) APIContractSource(_ context.Context, deploymentID, id string) (model.APIContractSource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.contractSources[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.APIContractSource{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) ActiveAPIContractSourceBySource(_ context.Context, deploymentID, sourceID string) (model.APIContractSource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.developerAssets.contractSources {
		if value.DeploymentID == deploymentID && value.SourceID == sourceID && value.Lifecycle == "attached" {
			return memoryClone(value), nil
		}
	}
	return model.APIContractSource{}, ErrNotFound
}

func (m *Memory) SaveAPIContractSource(_ context.Context, value model.APIContractSource, expected int64) (model.APIContractSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	contract, contractOK := m.developerAssets.contracts[value.APIContractID]
	source, sourceOK := m.sources[value.DeploymentID][value.SourceID]
	if !contractOK || !sourceOK || contract.DeploymentID != value.DeploymentID {
		return model.APIContractSource{}, ErrNotFound
	}
	if source.Kind != "openapi" && source.Kind != "git" && source.Kind != "upload" {
		return model.APIContractSource{}, ErrConflict
	}
	if value.SourceRole == "" {
		value.SourceRole = "primary"
	}
	if value.Lifecycle == "" {
		value.Lifecycle = "attached"
	}
	if (value.SourceRole != "primary" && value.SourceRole != "supplemental") || (value.Lifecycle != "attached" && value.Lifecycle != "detached") {
		return model.APIContractSource{}, ErrConflict
	}
	for id, current := range m.developerAssets.contractSources {
		if id != value.ID && current.APIContractID == value.APIContractID && current.SourceID == value.SourceID {
			return model.APIContractSource{}, ErrConflict
		}
		if id != value.ID && value.Lifecycle == "attached" && current.DeploymentID == value.DeploymentID && current.SourceID == value.SourceID && current.Lifecycle == "attached" {
			return model.APIContractSource{}, ErrConflict
		}
		if id != value.ID && value.Lifecycle == "attached" && value.SourceRole == "primary" && current.APIContractID == value.APIContractID && current.SourceRole == "primary" && current.Lifecycle == "attached" {
			return model.APIContractSource{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := m.developerAssets.contractSources[value.ID]
	if !exists {
		if expected != 0 {
			return model.APIContractSource{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	} else {
		if current.Revision != expected {
			return model.APIContractSource{}, ErrConflict
		}
		if current.DeploymentID != value.DeploymentID || current.APIContractID != value.APIContractID || current.SourceID != value.SourceID {
			return model.APIContractSource{}, ErrConflict
		}
		value.CreatedBy, value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedBy, current.CreatedAt, expected+1, now
	}
	m.developerAssets.contractSources[value.ID] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) DetachAPIContractSource(_ context.Context, deploymentID, id string, expected int64) (model.APIContractSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.developerAssets.contractSources[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.APIContractSource{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.APIContractSource{}, ErrConflict
	}
	value.Lifecycle, value.Revision, value.UpdatedAt = "detached", expected+1, time.Now().UTC()
	m.developerAssets.contractSources[id] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APIContractCandidates(_ context.Context, deploymentID, contractID string) ([]model.APIContractCandidate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contract, ok := m.developerAssets.contracts[contractID]
	if !ok || contract.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIContractCandidate, 0, len(m.developerAssets.contractCandidateIDs[contractID]))
	for _, id := range m.developerAssets.contractCandidateIDs[contractID] {
		result = append(result, memoryClone(m.developerAssets.contractCandidates[id].Candidate))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) APIContractCandidate(_ context.Context, deploymentID, id string) (APIContractCandidateRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.contractCandidates[id]
	if !ok || value.Candidate.DeploymentID != deploymentID {
		return APIContractCandidateRecord{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateAPIContractCandidate(_ context.Context, value APIContractCandidateRecord) (model.APIContractCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	contract, ok := m.developerAssets.contracts[value.Candidate.APIContractID]
	run, runOK := m.developerAssets.ingestionRuns[value.Candidate.IngestionRunID]
	if !ok || !runOK || contract.DeploymentID != value.Candidate.DeploymentID || run.DeploymentID != value.Candidate.DeploymentID || run.AssetKind != model.DeveloperAssetContract || run.TargetID != value.Candidate.APIContractID {
		return model.APIContractCandidate{}, ErrNotFound
	}
	if _, exists := m.developerAssets.contractCandidates[value.Candidate.ID]; exists {
		return model.APIContractCandidate{}, ErrConflict
	}
	for _, current := range m.developerAssets.contractCandidates {
		if current.Candidate.APIContractID == value.Candidate.APIContractID && (current.Candidate.IngestionRunID == value.Candidate.IngestionRunID || current.Candidate.ContentHash == value.Candidate.ContentHash) {
			return model.APIContractCandidate{}, ErrConflict
		}
	}
	operationIDs := make(map[string]bool, len(value.Operations))
	for _, operation := range value.Operations {
		if operation.APIContractCandidateID != value.Candidate.ID || operationIDs[operation.ID] {
			return model.APIContractCandidate{}, ErrConflict
		}
		operationIDs[operation.ID] = true
	}
	for _, schema := range value.Schemas {
		if schema.APIContractCandidateID != value.Candidate.ID {
			return model.APIContractCandidate{}, ErrConflict
		}
	}
	for _, example := range value.Examples {
		if example.APIContractCandidateID != value.Candidate.ID || (example.APIContractOperationID != "" && !operationIDs[example.APIContractOperationID]) {
			return model.APIContractCandidate{}, ErrConflict
		}
	}
	if value.Map != nil && (value.Map.DeploymentID != value.Candidate.DeploymentID || value.Map.APIContractCandidateID != value.Candidate.ID) {
		return model.APIContractCandidate{}, ErrConflict
	}
	value.Candidate.CreatedAt = time.Now().UTC()
	m.developerAssets.contractCandidates[value.Candidate.ID] = memoryClone(value)
	m.developerAssets.contractCandidateIDs[value.Candidate.APIContractID] = append(m.developerAssets.contractCandidateIDs[value.Candidate.APIContractID], value.Candidate.ID)
	return value.Candidate, nil
}

func (m *Memory) APIContractRevisions(_ context.Context, deploymentID, contractID string) ([]model.APIContractRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	contract, ok := m.developerAssets.contracts[contractID]
	if !ok || contract.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIContractRevision, 0, len(m.developerAssets.contractRevisionIDs[contractID]))
	for _, id := range m.developerAssets.contractRevisionIDs[contractID] {
		result = append(result, memoryClone(m.developerAssets.contractRevisions[id]))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) APIContractRevision(_ context.Context, deploymentID, id string) (model.APIContractRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.contractRevisions[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.APIContractRevision{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) PublishAPIContractCandidate(_ context.Context, contract model.APIContract, expected int64, revision model.APIContractRevision, sourceEvidence *model.APIContractRevisionSourcePublication) (model.APIContract, model.APIContractRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.developerAssets.contracts[contract.ID]
	if !ok || current.DeploymentID != contract.DeploymentID {
		return model.APIContract{}, model.APIContractRevision{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	if err := snapshotAPIContractIdentity(contract, &revision); err != nil {
		return model.APIContract{}, model.APIContractRevision{}, err
	}
	candidate, ok := m.developerAssets.contractCandidates[revision.APIContractCandidateID]
	if !ok || candidate.Candidate.APIContractID != contract.ID || revision.APIContractID != contract.ID || revision.DeploymentID != contract.DeploymentID || revision.ContentHash != candidate.Candidate.ContentHash || revision.Visibility != candidate.Candidate.Visibility {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	run := m.developerAssets.ingestionRuns[candidate.Candidate.IngestionRunID]
	if run.State != model.DeveloperAssetIngestionReviewReady || run.AssetKind != model.DeveloperAssetContract || run.TargetID != contract.ID || run.FailedCount != 0 || run.SkippedCount != 0 || run.QuarantinedCount != 0 {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	if sourceEvidence == nil {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	if sourceEvidence != nil {
		publication, exists := m.sourcePublications[revision.DeploymentID][sourceEvidence.SourcePublicationID]
		if !exists || sourceEvidence.APIContractRevisionID != revision.ID || sourceEvidence.DeploymentID != revision.DeploymentID || sourceEvidence.APIContractCandidateID != revision.APIContractCandidateID || sourceEvidence.ContentHash != publication.ContentHash || publication.SourceID != run.SourceID || (revision.Visibility == model.VisibilityPublic && publication.Visibility != model.VisibilityPublic) {
			return model.APIContract{}, model.APIContractRevision{}, ErrConflict
		}
	}
	if _, exists := m.developerAssets.contractRevisions[revision.ID]; exists {
		return model.APIContract{}, model.APIContractRevision{}, ErrConflict
	}
	for _, existing := range m.developerAssets.contractRevisions {
		if existing.APIContractID == contract.ID && (existing.APIContractCandidateID == revision.APIContractCandidateID || existing.ContentHash == revision.ContentHash) {
			return model.APIContract{}, model.APIContractRevision{}, ErrConflict
		}
	}
	revision.Revision = int64(len(m.developerAssets.contractRevisionIDs[contract.ID]) + 1)
	now := time.Now().UTC()
	if revision.PublishedAt.IsZero() {
		revision.PublishedAt = now
	}
	revision.CreatedAt = now
	contract.OrganisationID, contract.CreatedAt = current.OrganisationID, current.CreatedAt
	contract.Revision, contract.UpdatedAt = expected+1, now
	m.developerAssets.contracts[contract.ID] = memoryClone(contract)
	m.developerAssets.contractRevisions[revision.ID] = memoryClone(revision)
	m.developerAssets.contractRevisionIDs[contract.ID] = append(m.developerAssets.contractRevisionIDs[contract.ID], revision.ID)
	run.State, run.LeaseOwner, run.LeaseExpiresAt, run.HeartbeatAt = model.DeveloperAssetIngestionPublished, "", nil, nil
	run.FinishedAt = &now
	m.developerAssets.ingestionRuns[run.ID] = memoryClone(run)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return contract, revision, nil
}
