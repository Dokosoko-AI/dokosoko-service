package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) SDKReferences(_ context.Context, integrationID string) ([]model.SDKReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.integrations[integrationID]; !ok {
		return nil, ErrNotFound
	}
	result := make([]model.SDKReference, 0, len(m.sdkReferences[integrationID]))
	for _, value := range m.sdkReferences[integrationID] {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ecosystem == result[j].Ecosystem {
			return result[i].Coordinate < result[j].Coordinate
		}
		return result[i].Ecosystem < result[j].Ecosystem
	})
	return result, nil
}

func (m *Memory) SDKReference(_ context.Context, integrationID, id string) (model.SDKReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.sdkReferences[integrationID][id]
	if !ok {
		return model.SDKReference{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveSDKReference(_ context.Context, value model.SDKReference, expected int64) (model.SDKReference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	integration, ok := m.integrations[value.IntegrationID]
	if !ok || integration.DeploymentID != value.DeploymentID || integration.OrganisationID != value.OrganisationID {
		return model.SDKReference{}, ErrNotFound
	}
	values := m.sdkReferences[value.IntegrationID]
	currentReference, referenceExists := values[value.ID]
	if (expected == 0 && referenceExists) || (expected != 0 && (!referenceExists || currentReference.Revision != expected)) {
		return model.SDKReference{}, ErrConflict
	}
	for id, current := range values {
		if id != value.ID && current.Ecosystem == value.Ecosystem && model.CanonicalSDKCoordinate(current.Ecosystem, current.Coordinate) == model.CanonicalSDKCoordinate(value.Ecosystem, value.Coordinate) {
			return model.SDKReference{}, ErrConflict
		}
	}

	canonicalCoordinate := model.CanonicalSDKCoordinate(value.Ecosystem, value.Coordinate)
	var packageValue model.SDKPackage
	packageExists := false
	for _, current := range m.developerAssets.sdkPackages {
		if current.DeploymentID == value.DeploymentID && current.Ecosystem == value.Ecosystem && current.CanonicalCoordinate == canonicalCoordinate {
			packageValue, packageExists = current, true
			break
		}
	}
	if packageExists && value.Visibility == model.VisibilityPublic && packageValue.Visibility != model.VisibilityPublic {
		return model.SDKReference{}, ErrConflict
	}
	if !packageExists {
		packageID, err := newStoreUUID()
		if err != nil {
			return model.SDKReference{}, err
		}
		packageValue = model.SDKPackage{
			ID: packageID, DeploymentID: value.DeploymentID, OrganisationID: value.OrganisationID,
			Ecosystem: value.Ecosystem, CanonicalCoordinate: canonicalCoordinate, DisplayCoordinate: value.Coordinate,
			Name: value.Coordinate, Visibility: value.Visibility, Lifecycle: "active", Revision: 1,
		}
	}

	var release model.SDKRelease
	releaseExists := false
	for _, current := range m.developerAssets.sdkReleases {
		if current.SDKPackageID == packageValue.ID && current.ExactVersion == value.ExactVersion {
			release, releaseExists = current, true
			break
		}
	}
	if releaseExists && !legacySDKReleaseMatches(release, value) {
		return model.SDKReference{}, ErrConflict
	}
	if !releaseExists {
		releaseID, err := newStoreUUID()
		if err != nil {
			return model.SDKReference{}, err
		}
		releaseHash, err := legacySDKReleaseHash(packageValue.ID, value)
		if err != nil {
			return model.SDKReference{}, err
		}
		release = model.SDKRelease{
			ID: releaseID, DeploymentID: value.DeploymentID, SDKPackageID: packageValue.ID,
			ExactVersion: value.ExactVersion, InstallCommand: value.InstallCommand,
			DocumentationURL: value.DocumentationURL, SourceURL: value.SourceURL,
			UpstreamDigest: value.Checksum, IdentityAssurance: legacySDKIdentityAssurance(value.Checksum),
			Visibility: value.Visibility, Lifecycle: "active", ReleaseHash: releaseHash,
		}
	}

	currentBinding, bindingExists := m.developerAssets.sdkBindings[value.ID]
	if bindingExists {
		if currentBinding.DeploymentID != value.DeploymentID || currentBinding.APIID != value.IntegrationID ||
			currentBinding.Revision != expected || currentBinding.State != "legacy_metadata" {
			return model.SDKReference{}, ErrConflict
		}
	} else if expected != 0 && !referenceExists {
		return model.SDKReference{}, ErrConflict
	}
	for id, current := range m.developerAssets.sdkBindings {
		if id != value.ID && current.APIID == value.IntegrationID && current.SDKPackageID == packageValue.ID && current.State != "detached" {
			return model.SDKReference{}, ErrConflict
		}
	}

	now := time.Now().UTC()
	if !packageExists {
		packageValue.CreatedAt, packageValue.UpdatedAt = now, now
		m.developerAssets.sdkPackages[packageValue.ID] = memoryClone(packageValue)
	}
	if !releaseExists {
		release.CreatedAt = now
		m.developerAssets.sdkReleases[release.ID] = memoryClone(release)
		m.developerAssets.sdkReleaseIDs[packageValue.ID] = append(m.developerAssets.sdkReleaseIDs[packageValue.ID], release.ID)
	}

	bindingCreatedAt := now
	if bindingExists {
		bindingCreatedAt = currentBinding.CreatedAt
	} else if referenceExists {
		bindingCreatedAt = currentReference.CreatedAt
	}
	bindingRevision := int64(1)
	if expected != 0 {
		bindingRevision = expected + 1
	}
	binding := model.APISDKBinding{
		ID: value.ID, DeploymentID: value.DeploymentID, APIID: value.IntegrationID,
		SDKPackageID: packageValue.ID, SDKReleaseID: release.ID, State: "legacy_metadata",
		Coverage: model.SDKCoverageUnknown, Assurance: model.SDKAssuranceRelated,
		ApplicableModules: []string{}, ApplicableCapabilities: []string{}, ApplicableOperationKeys: []string{},
		Selector: json.RawMessage(`{}`), SelectorHash: legacySDKSelectorHash(), Visibility: value.Visibility,
		Revision: bindingRevision, CreatedAt: bindingCreatedAt, UpdatedAt: now,
	}
	m.developerAssets.sdkBindings[value.ID] = memoryClone(binding)
	if values == nil {
		values = make(map[string]model.SDKReference)
		m.sdkReferences[value.IntegrationID] = values
	}
	m.saveLegacySDKProjectionLocked(binding)
	m.bumpDeveloperAssetCatalogRevisionLocked()
	return memoryClone(m.sdkReferences[value.IntegrationID][value.ID]), nil
}

func (m *Memory) DeleteSDKReference(_ context.Context, integrationID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sdkReferences[integrationID][id]; !ok {
		return ErrNotFound
	}
	if binding, ok := m.developerAssets.sdkBindings[id]; ok {
		if binding.APIID != integrationID || binding.State == "detached" {
			return ErrNotFound
		}
		binding.State = "detached"
		binding.Revision++
		binding.UpdatedAt = time.Now().UTC()
		m.developerAssets.sdkBindings[id] = memoryClone(binding)
		m.bumpDeveloperAssetCatalogRevisionLocked()
	}
	delete(m.sdkReferences[integrationID], id)
	return nil
}
