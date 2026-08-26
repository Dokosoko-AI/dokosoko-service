package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) DeveloperAssetUsage(_ context.Context, deploymentID string) (DeveloperAssetUsageRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := DeveloperAssetUsageRecord{
		Documentation: make([]model.APIDocumentationBinding, 0),
		Contracts:     make([]model.APIContractBinding, 0),
		SDKs:          make([]model.APISDKBinding, 0),
		Publications:  make([]model.APIDeveloperAssetPublication, 0),
	}
	for _, value := range m.developerAssets.documentationBindings {
		if value.DeploymentID == deploymentID {
			result.Documentation = append(result.Documentation, memoryClone(value))
		}
	}
	for _, value := range m.developerAssets.contractBindings {
		if value.DeploymentID == deploymentID {
			result.Contracts = append(result.Contracts, memoryClone(value))
		}
	}
	for _, value := range m.developerAssets.sdkBindings {
		if value.DeploymentID == deploymentID {
			result.SDKs = append(result.SDKs, memoryClone(value))
		}
	}
	for _, value := range m.developerAssets.apiPublications {
		if value.DeploymentID == deploymentID {
			result.Publications = append(result.Publications, memoryClone(value))
		}
	}
	sort.Slice(result.Documentation, func(i, j int) bool {
		if result.Documentation[i].APIID == result.Documentation[j].APIID {
			return result.Documentation[i].ID < result.Documentation[j].ID
		}
		return result.Documentation[i].APIID < result.Documentation[j].APIID
	})
	sort.Slice(result.Contracts, func(i, j int) bool {
		if result.Contracts[i].APIID == result.Contracts[j].APIID {
			return result.Contracts[i].ID < result.Contracts[j].ID
		}
		return result.Contracts[i].APIID < result.Contracts[j].APIID
	})
	sort.Slice(result.SDKs, func(i, j int) bool {
		if result.SDKs[i].APIID == result.SDKs[j].APIID {
			return result.SDKs[i].ID < result.SDKs[j].ID
		}
		return result.SDKs[i].APIID < result.SDKs[j].APIID
	})
	sort.Slice(result.Publications, func(i, j int) bool {
		return result.Publications[i].PublishedAt.After(result.Publications[j].PublishedAt)
	})
	return result, nil
}

func (m *Memory) APIDocumentationBindings(_ context.Context, deploymentID, apiID string) ([]model.APIDocumentationBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	integration, ok := m.integrations[apiID]
	if !ok || integration.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIDocumentationBinding, 0)
	for _, value := range m.developerAssets.documentationBindings {
		if value.DeploymentID == deploymentID && value.APIID == apiID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (m *Memory) APIDocumentationBinding(_ context.Context, deploymentID, apiID, id string) (model.APIDocumentationBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.documentationBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APIDocumentationBinding{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveAPIDocumentationBinding(_ context.Context, value model.APIDocumentationBinding, expected int64) (model.APIDocumentationBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value.Selector) == 0 {
		value.Selector = json.RawMessage(`{}`)
	}
	selectorHash, err := documentationSelectorHash(value.Selector)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	value.SelectorHash = selectorHash
	integration, apiOK := m.integrations[value.APIID]
	collection, collectionOK := m.developerAssets.documentationCollections[value.DocumentationCollectionID]
	if !apiOK || !collectionOK || integration.DeploymentID != value.DeploymentID || collection.DeploymentID != value.DeploymentID {
		return model.APIDocumentationBinding{}, ErrNotFound
	}
	if !value.FollowLatest {
		revision, ok := m.developerAssets.documentationRevisions[value.PinnedRevisionID]
		if !ok || revision.Revision.DocumentationCollectionID != value.DocumentationCollectionID {
			return model.APIDocumentationBinding{}, ErrNotFound
		}
	}
	for id, current := range m.developerAssets.documentationBindings {
		if id != value.ID && current.APIID == value.APIID && current.DocumentationCollectionID == value.DocumentationCollectionID {
			return model.APIDocumentationBinding{}, ErrConflict
		}
	}
	current, exists := m.developerAssets.documentationBindings[value.ID]
	if !exists {
		if expected != 0 {
			return model.APIDocumentationBinding{}, ErrNotFound
		}
		value.Revision = 1
		if value.Lifecycle == "" {
			value.Lifecycle = "attached"
		}
	} else {
		if current.Revision != expected {
			return model.APIDocumentationBinding{}, ErrConflict
		}
		value.Revision = expected + 1
	}
	m.developerAssets.documentationBindings[value.ID] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) DetachAPIDocumentationBinding(_ context.Context, deploymentID, apiID, id string, expected int64) (model.APIDocumentationBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.developerAssets.documentationBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APIDocumentationBinding{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.APIDocumentationBinding{}, ErrConflict
	}
	value.Lifecycle, value.Revision = "detached", expected+1
	m.developerAssets.documentationBindings[id] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APIContractBindings(_ context.Context, deploymentID, apiID string) ([]model.APIContractBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	integration, ok := m.integrations[apiID]
	if !ok || integration.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIContractBinding, 0)
	for _, value := range m.developerAssets.contractBindings {
		if value.DeploymentID == deploymentID && value.APIID == apiID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (m *Memory) APIContractBinding(_ context.Context, deploymentID, apiID, id string) (model.APIContractBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.contractBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APIContractBinding{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveAPIContractBinding(_ context.Context, value model.APIContractBinding, expected int64) (model.APIContractBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value.Lifecycle == "" {
		value.Lifecycle = "attached"
	}
	integration, apiOK := m.integrations[value.APIID]
	contract, contractOK := m.developerAssets.contracts[value.APIContractID]
	if !apiOK || !contractOK || integration.DeploymentID != value.DeploymentID || contract.DeploymentID != value.DeploymentID {
		return model.APIContractBinding{}, ErrNotFound
	}
	if !value.FollowLatest {
		revision, ok := m.developerAssets.contractRevisions[value.PinnedRevisionID]
		if !ok || revision.APIContractID != value.APIContractID {
			return model.APIContractBinding{}, ErrNotFound
		}
	}
	for id, current := range m.developerAssets.contractBindings {
		if id != value.ID && current.APIID == value.APIID && current.APIContractID == value.APIContractID {
			return model.APIContractBinding{}, ErrConflict
		}
		if id != value.ID && value.Primary && value.Lifecycle == "attached" && current.APIID == value.APIID && current.Primary && current.Lifecycle == "attached" {
			return model.APIContractBinding{}, ErrConflict
		}
	}
	current, exists := m.developerAssets.contractBindings[value.ID]
	if !exists {
		if expected != 0 {
			return model.APIContractBinding{}, ErrNotFound
		}
		value.Revision = 1
	} else {
		if current.Revision != expected {
			return model.APIContractBinding{}, ErrConflict
		}
		value.Revision = expected + 1
	}
	m.developerAssets.contractBindings[value.ID] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) DetachAPIContractBinding(_ context.Context, deploymentID, apiID, id string, expected int64) (model.APIContractBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.developerAssets.contractBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APIContractBinding{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.APIContractBinding{}, ErrConflict
	}
	value.Lifecycle, value.Primary, value.Revision = "detached", false, expected+1
	m.developerAssets.contractBindings[id] = memoryClone(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APISDKBindings(_ context.Context, deploymentID, apiID string) ([]model.APISDKBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	integration, ok := m.integrations[apiID]
	if !ok || integration.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APISDKBinding, 0)
	for _, value := range m.developerAssets.sdkBindings {
		if value.DeploymentID == deploymentID && value.APIID == apiID {
			result = append(result, memoryClone(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (m *Memory) APISDKBinding(_ context.Context, deploymentID, apiID, id string) (model.APISDKBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.sdkBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APISDKBinding{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) validateAPISDKBindingLocked(value model.APISDKBinding) error {
	integration, apiOK := m.integrations[value.APIID]
	packageValue, packageOK := m.developerAssets.sdkPackages[value.SDKPackageID]
	release, releaseOK := m.developerAssets.sdkReleases[value.SDKReleaseID]
	if !apiOK || !packageOK || !releaseOK || integration.DeploymentID != value.DeploymentID || packageValue.DeploymentID != value.DeploymentID || release.SDKPackageID != value.SDKPackageID {
		return ErrNotFound
	}
	var contentPublication *model.SDKContentPublication
	if value.SDKContentPublicationID != "" {
		publication, ok := m.developerAssets.sdkPublications[value.SDKContentPublicationID]
		if !ok || publication.Publication.SDKReleaseID != value.SDKReleaseID {
			return ErrNotFound
		}
		copy := publication.Publication
		contentPublication = &copy
	}
	if value.APIContractRevisionID != "" {
		revision, ok := m.developerAssets.contractRevisions[value.APIContractRevisionID]
		if !ok || revision.DeploymentID != value.DeploymentID {
			return ErrNotFound
		}
		attached := false
		for _, binding := range m.developerAssets.contractBindings {
			if binding.APIID == value.APIID && binding.APIContractID == revision.APIContractID && binding.Lifecycle == "attached" {
				attached = true
				break
			}
		}
		if !attached {
			return ErrConflict
		}
	}
	if value.CompatibilityAssertionID != "" {
		assertion, ok := m.developerAssets.sdkAssertions[value.CompatibilityAssertionID]
		if !ok || assertion.APIID != value.APIID || assertion.SDKReleaseID != value.SDKReleaseID {
			return ErrNotFound
		}
		if assertion.State != "active" || assertion.APIContractRevisionID != value.APIContractRevisionID ||
			assertion.Coverage != value.Coverage || assertion.Assurance != value.Assurance {
			return ErrConflict
		}
	}
	if value.Visibility == model.VisibilityPublic &&
		(packageValue.Visibility != model.VisibilityPublic || release.Visibility != model.VisibilityPublic ||
			(contentPublication != nil && contentPublication.Visibility != model.VisibilityPublic)) {
		return ErrConflict
	}
	if integration.Visibility == model.VisibilityPublic && value.Visibility != model.VisibilityPublic {
		return ErrConflict
	}
	return nil
}

func (m *Memory) saveLegacySDKProjectionLocked(value model.APISDKBinding) {
	if value.State == "detached" {
		delete(m.sdkReferences[value.APIID], value.ID)
		return
	}
	packageValue := m.developerAssets.sdkPackages[value.SDKPackageID]
	release := m.developerAssets.sdkReleases[value.SDKReleaseID]
	if m.sdkReferences[value.APIID] == nil {
		m.sdkReferences[value.APIID] = make(map[string]model.SDKReference)
	}
	current, exists := m.sdkReferences[value.APIID][value.ID]
	createdAt := value.CreatedAt
	if exists {
		createdAt = current.CreatedAt
	}
	m.sdkReferences[value.APIID][value.ID] = model.SDKReference{
		ID: value.ID, DeploymentID: value.DeploymentID, OrganisationID: packageValue.OrganisationID,
		IntegrationID: value.APIID, Ecosystem: packageValue.Ecosystem, Coordinate: packageValue.DisplayCoordinate,
		ExactVersion: release.ExactVersion, InstallCommand: release.InstallCommand,
		DocumentationURL: release.DocumentationURL, SourceURL: release.SourceURL,
		Checksum: release.UpstreamDigest, Visibility: value.Visibility, Revision: value.Revision,
		CreatedAt: createdAt, UpdatedAt: value.UpdatedAt,
	}
}

func (m *Memory) SaveAPISDKBinding(_ context.Context, value model.APISDKBinding, expected int64) (model.APISDKBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.validateAPISDKBindingLocked(value); err != nil {
		return model.APISDKBinding{}, err
	}
	for id, current := range m.developerAssets.sdkBindings {
		if id != value.ID && current.APIID == value.APIID && current.SDKPackageID == value.SDKPackageID && current.State != "detached" && value.State != "detached" {
			return model.APISDKBinding{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	current, exists := m.developerAssets.sdkBindings[value.ID]
	if !exists {
		if expected != 0 {
			return model.APISDKBinding{}, ErrNotFound
		}
		value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
		if value.State == "" {
			value.State = "draft"
		}
	} else {
		if current.Revision != expected {
			return model.APISDKBinding{}, ErrConflict
		}
		value.CreatedAt, value.Revision, value.UpdatedAt = current.CreatedAt, expected+1, now
	}
	m.developerAssets.sdkBindings[value.ID] = memoryClone(value)
	m.saveLegacySDKProjectionLocked(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) DetachAPISDKBinding(_ context.Context, deploymentID, apiID, id string, expected int64) (model.APISDKBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.developerAssets.sdkBindings[id]
	if !ok || value.DeploymentID != deploymentID || value.APIID != apiID {
		return model.APISDKBinding{}, ErrNotFound
	}
	if value.Revision != expected {
		return model.APISDKBinding{}, ErrConflict
	}
	value.State, value.Revision, value.UpdatedAt = "detached", expected+1, time.Now().UTC()
	m.developerAssets.sdkBindings[id] = memoryClone(value)
	m.saveLegacySDKProjectionLocked(value)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return value, nil
}

func (m *Memory) APIDeveloperAssetPublications(_ context.Context, deploymentID, apiID string) ([]model.APIDeveloperAssetPublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	integration, ok := m.integrations[apiID]
	if !ok || integration.DeploymentID != deploymentID {
		return nil, ErrNotFound
	}
	result := make([]model.APIDeveloperAssetPublication, 0, len(m.developerAssets.apiPublicationIDs[apiID]))
	for _, id := range m.developerAssets.apiPublicationIDs[apiID] {
		result = append(result, memoryClone(m.developerAssets.apiPublications[id]))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PublishedAt.After(result[j].PublishedAt) })
	return result, nil
}

func (m *Memory) APIDeveloperAssetPublication(_ context.Context, deploymentID, id string) (model.APIDeveloperAssetPublication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.developerAssets.apiPublications[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.APIDeveloperAssetPublication{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) CreateAPIDeveloperAssetPublication(_ context.Context, value model.APIDeveloperAssetPublication) (model.APIDeveloperAssetPublication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.APIID]
	revision, revisionOK := m.integrationRevisions[value.APIID][value.APIRevisionID]
	if !ok || !revisionOK || integration.DeploymentID != value.DeploymentID || revision.State != "published" || revision.PublishedAt == nil {
		return model.APIDeveloperAssetPublication{}, ErrNotFound
	}
	if _, exists := m.developerAssets.apiPublications[value.ID]; exists {
		return model.APIDeveloperAssetPublication{}, ErrConflict
	}
	for _, current := range m.developerAssets.apiPublications {
		if current.APIRevisionID == value.APIRevisionID {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
	}
	if value.DeploymentDocumentationPublicationID != "" {
		global, exists := m.developerAssets.documentationPublications[value.DeploymentDocumentationPublicationID]
		if !exists || global.DeploymentID != value.DeploymentID || (integration.Visibility == model.VisibilityPublic && global.Visibility != model.VisibilityPublic) {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
	}
	docOrdinals, contractOrdinals, sdkOrdinals := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for index := range value.Documentation {
		asset := &value.Documentation[index]
		binding, bindingOK := m.developerAssets.documentationBindings[asset.BindingID]
		record, recordOK := m.developerAssets.documentationRevisions[asset.DocumentationCollectionRevisionID]
		resolvedRevisionID := binding.PinnedRevisionID
		if binding.FollowLatest {
			revisionIDs := m.developerAssets.documentationRevisionIDs[binding.DocumentationCollectionID]
			if len(revisionIDs) > 0 {
				resolvedRevisionID = revisionIDs[len(revisionIDs)-1]
			}
		}
		if !bindingOK || !recordOK || snapshotAPIDocumentationAssetIdentity(record.Revision, asset) != nil ||
			binding.APIID != value.APIID || binding.Lifecycle != "attached" ||
			asset.DocumentationCollectionRevisionID != resolvedRevisionID ||
			record.Revision.DocumentationCollectionID != binding.DocumentationCollectionID ||
			record.Revision.ContentHash != asset.ContentHash || !documentationSelectorsEqual(asset.Selector, binding.Selector) ||
			asset.SelectorHash != binding.SelectorHash || docOrdinals[asset.Ordinal] {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
		docOrdinals[asset.Ordinal] = true
	}
	for index := range value.Contracts {
		asset := &value.Contracts[index]
		binding, bindingOK := m.developerAssets.contractBindings[asset.BindingID]
		contractRevision, revisionOK := m.developerAssets.contractRevisions[asset.APIContractRevisionID]
		if !bindingOK || !revisionOK || snapshotAPIContractAssetIdentity(contractRevision, asset) != nil ||
			binding.APIID != value.APIID || binding.Lifecycle != "attached" || contractRevision.APIContractID != binding.APIContractID || contractRevision.ContentHash != asset.ContentHash || contractOrdinals[asset.Ordinal] {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
		contractOrdinals[asset.Ordinal] = true
	}
	for _, asset := range value.SDKs {
		binding, bindingOK := m.developerAssets.sdkBindings[asset.BindingID]
		packageValue, packageOK := m.developerAssets.sdkPackages[asset.SDKPackageID]
		release, releaseOK := m.developerAssets.sdkReleases[asset.SDKReleaseID]
		if !bindingOK || !packageOK || !releaseOK || binding.APIID != value.APIID || binding.State == "detached" ||
			binding.SDKPackageID != asset.SDKPackageID || binding.SDKReleaseID != asset.SDKReleaseID ||
			packageValue.DeploymentID != value.DeploymentID || asset.SDKPackageEcosystem != packageValue.Ecosystem ||
			asset.SDKPackageCoordinate != packageValue.CanonicalCoordinate || asset.SDKPackageDisplayCoordinate != packageValue.DisplayCoordinate ||
			asset.SDKPackageDisplayName != packageValue.Name || asset.SDKPackageLanguage != packageValue.Language ||
			asset.SDKPackagePlatform != packageValue.Platform || release.ReleaseHash == "" || sdkOrdinals[asset.Ordinal] {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
		if asset.SDKContentPublicationID != "" {
			publication, exists := m.developerAssets.sdkPublications[asset.SDKContentPublicationID]
			if !exists || publication.Publication.SDKReleaseID != asset.SDKReleaseID || publication.Publication.ContentHash != asset.ContentHash {
				return model.APIDeveloperAssetPublication{}, ErrConflict
			}
		} else if asset.ContentHash != release.ReleaseHash {
			return model.APIDeveloperAssetPublication{}, ErrConflict
		}
		sdkOrdinals[asset.Ordinal] = true
	}
	value.PublishedAt = *revision.PublishedAt
	value.CreatedAt = time.Now().UTC()
	m.developerAssets.apiPublications[value.ID] = memoryClone(value)
	m.developerAssets.apiPublicationIDs[value.APIID] = append(m.developerAssets.apiPublicationIDs[value.APIID], value.ID)
	return value, nil
}
