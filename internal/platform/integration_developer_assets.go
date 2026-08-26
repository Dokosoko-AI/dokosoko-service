package platform

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const developerAssetSnapshotSchemaVersion = "developer-assets-v2"

type integrationDeveloperAssetSnapshot struct {
	SchemaVersion                    string                                   `json:"schema_version"`
	APIVisibility                    model.Visibility                         `json:"api_visibility"`
	GlobalDocumentationPublicationID string                                   `json:"global_documentation_publication_id,omitempty"`
	GlobalDocumentationSnapshotHash  string                                   `json:"global_documentation_snapshot_hash,omitempty"`
	Documentation                    []model.APIPublicationDocumentationAsset `json:"documentation"`
	Contracts                        []model.APIPublicationContractAsset      `json:"contracts"`
	SDKs                             []model.APIPublicationSDKAsset           `json:"sdks"`
}

func (s *Service) resolveIntegrationDeveloperAssets(ctx context.Context, integration model.Integration) (integrationDeveloperAssetSnapshot, []IntegrationPublishValidation, error) {
	result := integrationDeveloperAssetSnapshot{
		SchemaVersion: developerAssetSnapshotSchemaVersion,
		APIVisibility: integration.Visibility,
		Documentation: []model.APIPublicationDocumentationAsset{},
		Contracts:     []model.APIPublicationContractAsset{},
		SDKs:          []model.APIPublicationSDKAsset{},
	}
	validations := make([]IntegrationPublishValidation, 0)
	global, err := s.readyDeploymentDocumentationPublication(ctx, integration.DeploymentID)
	if err == nil {
		if integration.Visibility == model.VisibilityPublic && global.Visibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "global_documentation_not_public", Message: "The active global documentation publication is private and cannot be selected by a public API.", Tab: "resources"})
		} else {
			result.GlobalDocumentationPublicationID = global.ID
			result.GlobalDocumentationSnapshotHash = global.SnapshotHash
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return result, nil, err
	}

	documentationBindings, err := s.store.APIDocumentationBindings(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return result, nil, err
	}
	for _, binding := range documentationBindings {
		if binding.Lifecycle != "attached" {
			continue
		}
		var revision store.DocumentationCollectionRevisionRecord
		if binding.FollowLatest {
			revisions, lookupErr := s.store.DocumentationCollectionRevisions(ctx, integration.DeploymentID, binding.DocumentationCollectionID)
			if lookupErr != nil || len(revisions) == 0 {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_revision_unresolved", Message: "An attached documentation collection has no reviewed revision.", Tab: "resources"})
				continue
			}
			revision, err = s.store.DocumentationCollectionRevision(ctx, integration.DeploymentID, revisions[0].ID)
		} else {
			revision, err = s.store.DocumentationCollectionRevision(ctx, integration.DeploymentID, binding.PinnedRevisionID)
		}
		if err != nil || revision.Revision.DocumentationCollectionID != binding.DocumentationCollectionID {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_revision_unresolved", Message: "An attached documentation binding does not resolve to its exact reviewed revision.", Tab: "resources"})
			continue
		}
		effectiveVisibility, visibilityErr := developerAssetVisibility(binding.Visibility, revision.Revision.Visibility)
		if visibilityErr != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_visibility_invalid", Message: "A documentation binding or revision has invalid visibility.", Tab: "resources"})
			continue
		}
		if binding.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_visibility_widened", Message: "A public documentation binding cannot widen its selected private revision.", Tab: "resources"})
			continue
		}
		if integration.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_not_public", Message: "A public API can only select public documentation.", Tab: "resources"})
			continue
		}
		selector, selectorErr := normalizeJSONObject(binding.Selector)
		if selectorErr != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "documentation_selector_invalid", Message: "A documentation binding has an invalid selector.", Tab: "resources"})
			continue
		}
		result.Documentation = append(result.Documentation, model.APIPublicationDocumentationAsset{
			BindingID: binding.ID, DocumentationCollectionID: revision.Revision.DocumentationCollectionID,
			DocumentationCollectionName:        revision.Revision.DocumentationCollectionName,
			DocumentationCollectionSlug:        revision.Revision.DocumentationCollectionSlug,
			DocumentationCollectionDescription: revision.Revision.DocumentationCollectionDescription,
			DocumentationCollectionRevisionID:  revision.Revision.ID,
			Selector:                           selector, SelectorHash: binding.SelectorHash, ContentHash: revision.Revision.ContentHash,
			Visibility: effectiveVisibility,
		})
	}
	sort.Slice(result.Documentation, func(i, j int) bool { return result.Documentation[i].BindingID < result.Documentation[j].BindingID })
	for index := range result.Documentation {
		result.Documentation[index].Ordinal = index
	}

	contractBindings, err := s.store.APIContractBindings(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return result, nil, err
	}
	for _, binding := range contractBindings {
		if binding.Lifecycle != "attached" {
			continue
		}
		var revision model.APIContractRevision
		if binding.FollowLatest {
			revisions, lookupErr := s.store.APIContractRevisions(ctx, integration.DeploymentID, binding.APIContractID)
			if lookupErr != nil || len(revisions) == 0 {
				validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "contract_revision_unresolved", Message: "An attached API contract has no reviewed revision.", Tab: "resources"})
				continue
			}
			revision = revisions[0]
		} else {
			revision, err = s.store.APIContractRevision(ctx, integration.DeploymentID, binding.PinnedRevisionID)
		}
		if err != nil || revision.APIContractID != binding.APIContractID {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "contract_revision_unresolved", Message: "An attached contract binding does not resolve to its exact reviewed revision.", Tab: "resources"})
			continue
		}
		effectiveVisibility, visibilityErr := developerAssetVisibility(binding.Visibility, revision.Visibility)
		if visibilityErr != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "contract_visibility_invalid", Message: "A contract binding or revision has invalid visibility.", Tab: "resources"})
			continue
		}
		if binding.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "contract_visibility_widened", Message: "A public contract binding cannot widen its selected private revision.", Tab: "resources"})
			continue
		}
		if integration.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "contract_not_public", Message: "A public API can only select public contracts.", Tab: "resources"})
			continue
		}
		result.Contracts = append(result.Contracts, model.APIPublicationContractAsset{
			BindingID: binding.ID, APIContractID: revision.APIContractID,
			APIContractName: revision.APIContractName, APIContractSlug: revision.APIContractSlug,
			APIContractDescription: revision.APIContractDescription, APIContractKind: revision.APIContractKind,
			APIContractRevisionID: revision.ID, Primary: binding.Primary,
			ContentHash: revision.ContentHash, Visibility: effectiveVisibility,
		})
	}
	sort.Slice(result.Contracts, func(i, j int) bool { return result.Contracts[i].BindingID < result.Contracts[j].BindingID })
	for index := range result.Contracts {
		result.Contracts[index].Ordinal = index
	}

	sdkBindings, err := s.store.APISDKBindings(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return result, nil, err
	}
	for _, binding := range sdkBindings {
		if binding.State == "detached" {
			continue
		}
		release, releaseErr := s.store.SDKRelease(ctx, integration.DeploymentID, binding.SDKReleaseID)
		if releaseErr != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_release_unresolved", Message: "An SDK binding does not resolve to its exact release.", Tab: "resources"})
			continue
		}
		lifecycle, lifecycleErr := s.sdkReleaseLifecycleState(ctx, integration.DeploymentID, release)
		if lifecycleErr != nil {
			return result, nil, lifecycleErr
		}
		if !lifecycle.Selectable {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_release_unavailable", Message: "An attached SDK release is yanked or archived and cannot be selected by a new API publication.", Tab: "resources"})
			continue
		}
		if binding.State != "ready" || binding.SDKContentPublicationID == "" {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_binding_not_ready", Message: "An attached SDK release must have reviewed content before API publication.", Tab: "resources"})
			continue
		}
		packageValue, packageErr := s.store.SDKPackage(ctx, integration.DeploymentID, binding.SDKPackageID)
		publication, publicationErr := s.store.SDKContentPublication(ctx, integration.DeploymentID, binding.SDKContentPublicationID)
		if packageErr != nil || publicationErr != nil || release.SDKPackageID != packageValue.ID || publication.Publication.SDKReleaseID != release.ID {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_release_unresolved", Message: "An SDK binding does not resolve to one exact package release and reviewed content publication.", Tab: "resources"})
			continue
		}
		effectiveVisibility, visibilityErr := developerAssetVisibility(binding.Visibility, packageValue.Visibility, release.Visibility, publication.Publication.Visibility)
		if visibilityErr != nil {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_visibility_invalid", Message: "An SDK binding, package, release, or content publication has invalid visibility.", Tab: "resources"})
			continue
		}
		if binding.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_visibility_widened", Message: "A public SDK binding cannot widen its selected package, release, or content publication.", Tab: "resources"})
			continue
		}
		if integration.Visibility == model.VisibilityPublic && effectiveVisibility != model.VisibilityPublic {
			validations = append(validations, IntegrationPublishValidation{Level: "error", Code: "sdk_not_public", Message: "A public API can only select public SDK assets.", Tab: "resources"})
			continue
		}
		result.SDKs = append(result.SDKs, model.APIPublicationSDKAsset{
			BindingID: binding.ID, SDKPackageID: packageValue.ID,
			SDKPackageEcosystem: packageValue.Ecosystem, SDKPackageCoordinate: packageValue.CanonicalCoordinate, SDKPackageDisplayCoordinate: packageValue.DisplayCoordinate,
			SDKPackageDisplayName: packageValue.Name, SDKPackageLanguage: packageValue.Language, SDKPackagePlatform: packageValue.Platform,
			SDKReleaseID:            release.ID,
			SDKContentPublicationID: publication.Publication.ID, CompatibilityAssertionID: binding.CompatibilityAssertionID,
			Selector: append(json.RawMessage(nil), binding.Selector...), SelectorHash: binding.SelectorHash,
			ContentHash: publication.Publication.ContentHash, Visibility: effectiveVisibility,
		})
	}
	sort.Slice(result.SDKs, func(i, j int) bool { return result.SDKs[i].BindingID < result.SDKs[j].BindingID })
	for index := range result.SDKs {
		result.SDKs[index].Ordinal = index
	}
	return result, validations, nil
}

func (s *Service) ensureAPIDeveloperAssetPublication(ctx context.Context, integration model.Integration, revision model.IntegrationRevision, snapshot integrationDeveloperAssetSnapshot, actor Actor) (model.APIDeveloperAssetPublication, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	snapshotHash := contentHash(encoded)
	existing, err := s.store.APIDeveloperAssetPublications(ctx, integration.DeploymentID, integration.ID)
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	for _, publication := range existing {
		if publication.APIRevisionID == revision.ID {
			if publication.SnapshotHash != snapshotHash {
				return model.APIDeveloperAssetPublication{}, errors.New("API revision already has a different immutable developer-asset snapshot")
			}
			return publication, nil
		}
	}
	id, err := randomUUID()
	if err != nil {
		return model.APIDeveloperAssetPublication{}, err
	}
	publishedAt := s.now()
	if revision.PublishedAt != nil {
		publishedAt = *revision.PublishedAt
	}
	value := model.APIDeveloperAssetPublication{
		ID: id, DeploymentID: integration.DeploymentID, APIID: integration.ID, APIRevisionID: revision.ID,
		DeploymentDocumentationPublicationID: snapshot.GlobalDocumentationPublicationID,
		SnapshotSchemaVersion:                developerAssetSnapshotSchemaVersion, SnapshotHash: snapshotHash,
		Documentation: snapshot.Documentation, Contracts: snapshot.Contracts, SDKs: snapshot.SDKs,
		PublishedBy: actor.ID, PublishedAt: publishedAt,
	}
	value, err = s.store.CreateAPIDeveloperAssetPublication(ctx, value)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			publications, lookupErr := s.store.APIDeveloperAssetPublications(ctx, integration.DeploymentID, integration.ID)
			if lookupErr == nil {
				for _, publication := range publications {
					if publication.APIRevisionID == revision.ID && publication.SnapshotHash == snapshotHash {
						return publication, nil
					}
				}
			}
		}
		return model.APIDeveloperAssetPublication{}, err
	}
	return value, nil
}
