package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// APIResourceBindings is the attachment-only Resources view for one API.
// Root assets remain independently owned and may be attached to several APIs.
type APIResourceBindings struct {
	Documentation []model.APIDocumentationBinding `json:"documentation"`
	Contracts     []model.APIContractBinding      `json:"contracts"`
	SDKs          []model.APISDKBinding           `json:"sdks"`
}

// DeveloperAssetUsage is the deployment-wide inverse view of API Resources.
// Catalog pages filter this one response locally instead of loading every API
// separately for each selected asset.
type DeveloperAssetUsage struct {
	Documentation []model.APIDocumentationBinding      `json:"documentation"`
	Contracts     []model.APIContractBinding           `json:"contracts"`
	SDKs          []model.APISDKBinding                `json:"sdks"`
	Publications  []model.APIDeveloperAssetPublication `json:"publications"`
}

func (s *Service) DeveloperAssetUsage(ctx context.Context) (DeveloperAssetUsage, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return DeveloperAssetUsage{}, err
	}
	value, err := s.store.DeveloperAssetUsage(ctx, deployment.ID)
	if err != nil {
		return DeveloperAssetUsage{}, err
	}
	return DeveloperAssetUsage{
		Documentation: value.Documentation,
		Contracts:     value.Contracts,
		SDKs:          value.SDKs,
		Publications:  value.Publications,
	}, nil
}

func (s *Service) APIResourceBindings(ctx context.Context, apiID string) (APIResourceBindings, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return APIResourceBindings{}, err
	}
	apiID = strings.TrimSpace(apiID)
	if _, err := s.store.Integration(ctx, deployment.ID, apiID); err != nil {
		return APIResourceBindings{}, err
	}
	documentation, err := s.store.APIDocumentationBindings(ctx, deployment.ID, apiID)
	if err != nil {
		return APIResourceBindings{}, err
	}
	contracts, err := s.store.APIContractBindings(ctx, deployment.ID, apiID)
	if err != nil {
		return APIResourceBindings{}, err
	}
	sdks, err := s.store.APISDKBindings(ctx, deployment.ID, apiID)
	if err != nil {
		return APIResourceBindings{}, err
	}
	return APIResourceBindings{Documentation: documentation, Contracts: contracts, SDKs: sdks}, nil
}

type APIDocumentationBindingInput struct {
	DocumentationCollectionID string
	FollowLatest              bool
	PinnedRevisionID          string
	Selector                  json.RawMessage
	Visibility                model.Visibility
	Lifecycle                 string
	Revision                  int64
}

func (s *Service) SaveAPIDocumentationBinding(ctx context.Context, apiID, bindingID string, input APIDocumentationBindingInput, actor Actor) (model.APIDocumentationBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	api, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(apiID))
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	collection, err := s.store.DocumentationCollection(ctx, deployment.ID, strings.TrimSpace(input.DocumentationCollectionID))
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	input.PinnedRevisionID = strings.TrimSpace(input.PinnedRevisionID)
	if input.FollowLatest == (input.PinnedRevisionID != "") {
		return model.APIDocumentationBinding{}, errors.New("documentation binding must either follow the latest reviewed revision or pin one exact revision")
	}
	resolvedVisibility := collection.Visibility
	if input.PinnedRevisionID != "" {
		record, lookupErr := s.store.DocumentationCollectionRevision(ctx, deployment.ID, input.PinnedRevisionID)
		if lookupErr != nil || record.Revision.DocumentationCollectionID != collection.ID {
			return model.APIDocumentationBinding{}, errors.New("pinned documentation revision does not belong to the selected collection")
		}
		resolvedVisibility = record.Revision.Visibility
	} else {
		revisions, lookupErr := s.store.DocumentationCollectionRevisions(ctx, deployment.ID, collection.ID)
		if lookupErr != nil || len(revisions) == 0 {
			return model.APIDocumentationBinding{}, errors.New("documentation collection has no reviewed revision")
		}
		resolvedVisibility = revisions[0].Visibility
	}
	if input.Visibility == "" {
		input.Visibility = resolvedVisibility
	}
	if !input.Visibility.Valid() || (input.Visibility == model.VisibilityPublic && (collection.Visibility != model.VisibilityPublic || resolvedVisibility != model.VisibilityPublic)) {
		return model.APIDocumentationBinding{}, errors.New("documentation binding cannot widen collection or revision visibility")
	}
	if api.Visibility == model.VisibilityPublic && input.Visibility != model.VisibilityPublic {
		return model.APIDocumentationBinding{}, errors.New("a public API can only attach public documentation")
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "attached"
	}
	if input.Lifecycle != "attached" {
		return model.APIDocumentationBinding{}, errors.New("documentation binding lifecycle must be attached")
	}
	selector, err := normalizeJSONObject(input.Selector)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	bindingID = strings.TrimSpace(bindingID)
	if bindingID == "" {
		bindingID, err = randomUUID()
		if err != nil {
			return model.APIDocumentationBinding{}, err
		}
	}
	value := model.APIDocumentationBinding{
		ID: bindingID, DeploymentID: deployment.ID, APIID: api.ID, DocumentationCollectionID: collection.ID,
		FollowLatest: input.FollowLatest, PinnedRevisionID: input.PinnedRevisionID, Selector: selector,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle,
	}
	if !value.Valid() {
		return model.APIDocumentationBinding{}, errors.New("documentation binding is invalid")
	}
	value, err = s.store.SaveAPIDocumentationBinding(ctx, value, input.Revision)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.documentation_binding.saved", "api_documentation_binding", value.ID, map[string]any{
		"api_id": api.ID, "documentation_collection_id": collection.ID, "follow_latest": value.FollowLatest,
		"pinned_revision_id": value.PinnedRevisionID, "visibility": value.Visibility, "revision": value.Revision,
	}); err != nil {
		return model.APIDocumentationBinding{}, err
	}
	return value, nil
}

func (s *Service) DetachAPIDocumentationBinding(ctx context.Context, apiID, bindingID string, revision int64, actor Actor) (model.APIDocumentationBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	value, err := s.store.DetachAPIDocumentationBinding(ctx, deployment.ID, strings.TrimSpace(apiID), strings.TrimSpace(bindingID), revision)
	if err != nil {
		return model.APIDocumentationBinding{}, err
	}
	err = s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.documentation_binding.detached", "api_documentation_binding", value.ID, map[string]any{
		"api_id": value.APIID, "documentation_collection_id": value.DocumentationCollectionID, "revision": value.Revision,
	})
	return value, err
}

type APIContractBindingInput struct {
	APIContractID    string
	FollowLatest     bool
	PinnedRevisionID string
	Primary          bool
	Visibility       model.Visibility
	Lifecycle        string
	Revision         int64
}

func (s *Service) SaveAPIContractBinding(ctx context.Context, apiID, bindingID string, input APIContractBindingInput, actor Actor) (model.APIContractBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	api, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(apiID))
	if err != nil {
		return model.APIContractBinding{}, err
	}
	contract, err := s.store.APIContract(ctx, deployment.ID, strings.TrimSpace(input.APIContractID))
	if err != nil {
		return model.APIContractBinding{}, err
	}
	input.PinnedRevisionID = strings.TrimSpace(input.PinnedRevisionID)
	if input.FollowLatest == (input.PinnedRevisionID != "") {
		return model.APIContractBinding{}, errors.New("contract binding must either follow the latest reviewed revision or pin one exact revision")
	}
	resolvedVisibility := contract.Visibility
	if input.PinnedRevisionID != "" {
		revision, lookupErr := s.store.APIContractRevision(ctx, deployment.ID, input.PinnedRevisionID)
		if lookupErr != nil || revision.APIContractID != contract.ID {
			return model.APIContractBinding{}, errors.New("pinned contract revision does not belong to the selected contract")
		}
		resolvedVisibility = revision.Visibility
	} else {
		revisions, lookupErr := s.store.APIContractRevisions(ctx, deployment.ID, contract.ID)
		if lookupErr != nil || len(revisions) == 0 {
			return model.APIContractBinding{}, errors.New("API contract has no reviewed revision")
		}
		resolvedVisibility = revisions[0].Visibility
	}
	if input.Visibility == "" {
		input.Visibility = resolvedVisibility
	}
	if !input.Visibility.Valid() || (input.Visibility == model.VisibilityPublic && (contract.Visibility != model.VisibilityPublic || resolvedVisibility != model.VisibilityPublic)) {
		return model.APIContractBinding{}, errors.New("contract binding cannot widen contract or revision visibility")
	}
	if api.Visibility == model.VisibilityPublic && input.Visibility != model.VisibilityPublic {
		return model.APIContractBinding{}, errors.New("a public API can only attach public contract revisions")
	}
	if input.Lifecycle == "" {
		input.Lifecycle = "attached"
	}
	if input.Lifecycle != "attached" {
		return model.APIContractBinding{}, errors.New("contract binding lifecycle must be attached")
	}
	bindingID = strings.TrimSpace(bindingID)
	if bindingID == "" {
		bindingID, err = randomUUID()
		if err != nil {
			return model.APIContractBinding{}, err
		}
	}
	value := model.APIContractBinding{
		ID: bindingID, DeploymentID: deployment.ID, APIID: api.ID, APIContractID: contract.ID,
		FollowLatest: input.FollowLatest, PinnedRevisionID: input.PinnedRevisionID, Primary: input.Primary,
		Visibility: input.Visibility, Lifecycle: input.Lifecycle,
	}
	if !value.Valid() {
		return model.APIContractBinding{}, errors.New("contract binding is invalid")
	}
	value, err = s.store.SaveAPIContractBinding(ctx, value, input.Revision)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	if err := s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.contract_binding.saved", "api_contract_binding", value.ID, map[string]any{
		"api_id": api.ID, "api_contract_id": contract.ID, "follow_latest": value.FollowLatest,
		"pinned_revision_id": value.PinnedRevisionID, "primary": value.Primary, "visibility": value.Visibility, "revision": value.Revision,
	}); err != nil {
		return model.APIContractBinding{}, err
	}
	return value, nil
}

func (s *Service) DetachAPIContractBinding(ctx context.Context, apiID, bindingID string, revision int64, actor Actor) (model.APIContractBinding, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	value, err := s.store.DetachAPIContractBinding(ctx, deployment.ID, strings.TrimSpace(apiID), strings.TrimSpace(bindingID), revision)
	if err != nil {
		return model.APIContractBinding{}, err
	}
	err = s.appendDeveloperAssetAudit(ctx, deployment, actor, "api.contract_binding.detached", "api_contract_binding", value.ID, map[string]any{
		"api_id": value.APIID, "api_contract_id": value.APIContractID, "revision": value.Revision,
	})
	return value, err
}
