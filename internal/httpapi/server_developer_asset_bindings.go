package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type apiDocumentationBindingRequest struct {
	DocumentationCollectionID string           `json:"documentation_collection_id"`
	FollowLatest              bool             `json:"follow_latest"`
	PinnedRevisionID          string           `json:"pinned_revision_id"`
	Selector                  json.RawMessage  `json:"selector"`
	Visibility                model.Visibility `json:"visibility"`
	Lifecycle                 string           `json:"lifecycle"`
	Revision                  int64            `json:"revision"`
}

type apiContractBindingRequest struct {
	APIContractID    string           `json:"api_contract_id"`
	FollowLatest     bool             `json:"follow_latest"`
	PinnedRevisionID string           `json:"pinned_revision_id"`
	Primary          bool             `json:"primary"`
	Visibility       model.Visibility `json:"visibility"`
	Lifecycle        string           `json:"lifecycle"`
	Revision         int64            `json:"revision"`
}

type apiSDKBindingRequest struct {
	SDKPackageID             string                          `json:"sdk_package_id"`
	SDKReleaseID             string                          `json:"sdk_release_id"`
	SDKContentPublicationID  string                          `json:"sdk_content_publication_id"`
	APIContractRevisionID    string                          `json:"api_contract_revision_id"`
	CompatibilityAssertionID string                          `json:"compatibility_assertion_id"`
	State                    string                          `json:"state"`
	Coverage                 model.SDKCompatibilityCoverage  `json:"coverage"`
	Assurance                model.SDKCompatibilityAssurance `json:"assurance"`
	ApplicableModules        []string                        `json:"applicable_modules"`
	ApplicableCapabilities   []string                        `json:"applicable_capabilities"`
	ApplicableOperationKeys  []string                        `json:"applicable_operation_keys"`
	Selector                 json.RawMessage                 `json:"selector"`
	Visibility               model.Visibility                `json:"visibility"`
	Revision                 int64                           `json:"revision"`
}

func (s *Server) apiResourceBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.APIResourceBindings(r.Context(), apiID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) apiDeveloperAssetPublications(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err = s.service.Store().Integration(r.Context(), deploymentID, apiID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIDeveloperAssetPublications(r.Context(), deploymentID, apiID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiDeveloperAssetPublication(w http.ResponseWriter, r *http.Request, apiID, publicationID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIDeveloperAssetPublication(r.Context(), deploymentID, publicationID)
	if err != nil || value.APIID != apiID {
		if err == nil {
			err = store.ErrNotFound
		}
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) apiDocumentationBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPIDocumentationBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiDocumentationBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APIDocumentationBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPIDocumentationBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPIDocumentationBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPIDocumentationBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiDocumentationBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPIDocumentationBinding(r.Context(), apiID, bindingID, platform.APIDocumentationBindingInput{
		DocumentationCollectionID: input.DocumentationCollectionID, FollowLatest: input.FollowLatest,
		PinnedRevisionID: input.PinnedRevisionID, Selector: input.Selector, Visibility: input.Visibility,
		Lifecycle: input.Lifecycle, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func (s *Server) apiContractBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPIContractBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiContractBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APIContractBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPIContractBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPIContractBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPIContractBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiContractBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPIContractBinding(r.Context(), apiID, bindingID, platform.APIContractBindingInput{
		APIContractID: input.APIContractID, FollowLatest: input.FollowLatest, PinnedRevisionID: input.PinnedRevisionID,
		Primary: input.Primary, Visibility: input.Visibility, Lifecycle: input.Lifecycle, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func (s *Server) apiSDKBindings(w http.ResponseWriter, r *http.Request, apiID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	s.saveAPISDKBinding(w, r, apiID, "", http.StatusCreated)
}

func (s *Server) apiSDKBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string) {
	switch r.Method {
	case http.MethodGet:
		deploymentID, err := s.developerAssetDeploymentID(r)
		if err != nil {
			s.storeError(w, err)
			return
		}
		value, err := s.service.Store().APISDKBinding(r.Context(), deploymentID, apiID, bindingID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		s.saveAPISDKBinding(w, r, apiID, bindingID, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.DetachAPISDKBinding(r.Context(), apiID, bindingID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) saveAPISDKBinding(w http.ResponseWriter, r *http.Request, apiID, bindingID string, status int) {
	var input apiSDKBindingRequest
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if bindingID != "" && input.Revision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.SaveAPISDKBinding(r.Context(), apiID, bindingID, platform.APISDKBindingInput{
		SDKPackageID: input.SDKPackageID, SDKReleaseID: input.SDKReleaseID,
		SDKContentPublicationID: input.SDKContentPublicationID, APIContractRevisionID: input.APIContractRevisionID,
		CompatibilityAssertionID: input.CompatibilityAssertionID, State: input.State, Coverage: input.Coverage,
		Assurance: input.Assurance, ApplicableModules: input.ApplicableModules,
		ApplicableCapabilities: input.ApplicableCapabilities, ApplicableOperationKeys: input.ApplicableOperationKeys,
		Selector: input.Selector, Visibility: input.Visibility, Revision: input.Revision,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, status, value)
}
