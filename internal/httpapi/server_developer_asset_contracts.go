package httpapi

import (
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type apiContractRequest struct {
	Name        string           `json:"name"`
	Slug        string           `json:"slug"`
	Description string           `json:"description"`
	Visibility  model.Visibility `json:"visibility"`
	Lifecycle   string           `json:"lifecycle"`
	Revision    int64            `json:"revision"`
}

func apiContractInput(input apiContractRequest) platform.APIContractInput {
	return platform.APIContractInput{Name: input.Name, Slug: input.Slug, Description: input.Description, Visibility: input.Visibility, Lifecycle: input.Lifecycle, Revision: input.Revision}
}

func (s *Server) apiContracts(w http.ResponseWriter, r *http.Request) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().APIContracts(r.Context(), deploymentID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input apiContractRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), "", apiContractInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) apiContract(w http.ResponseWriter, r *http.Request, contractID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input apiContractRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), contractID, apiContractInput(input), actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		current, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContract(r.Context(), contractID, platform.APIContractInput{
			Name: current.Name, Slug: current.Slug, Description: current.Description,
			Visibility: current.Visibility, Lifecycle: "archived", Revision: input.Revision,
		}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

type apiContractSourceRequest struct {
	SourceID   string `json:"source_id"`
	SourceRole string `json:"source_role"`
	Revision   int64  `json:"revision"`
}

func (s *Server) apiContractSources(w http.ResponseWriter, r *http.Request, contractID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID); err != nil {
			s.storeError(w, err)
			return
		}
		values, err := s.service.Store().APIContractSources(r.Context(), deploymentID, contractID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input apiContractSourceRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveAPIContractSource(r.Context(), contractID, "", platform.APIContractSourceInput{SourceID: input.SourceID, SourceRole: input.SourceRole}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) apiContractSource(w http.ResponseWriter, r *http.Request, contractID, associationID string) {
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	current, err := s.service.Store().APIContractSource(r.Context(), deploymentID, associationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if current.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, current)
	case http.MethodPatch:
		var input apiContractSourceRequest
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveAPIContractSource(r.Context(), contractID, associationID, platform.APIContractSourceInput{SourceID: input.SourceID, SourceRole: input.SourceRole, Revision: input.Revision}, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
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
		value, err := s.service.DetachAPIContractSource(r.Context(), associationID, input.Revision, actor(r))
		if err != nil {
			s.developerAssetError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) apiContractCandidates(w http.ResponseWriter, r *http.Request, contractID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if _, err := s.service.Store().APIContract(r.Context(), deploymentID, contractID); err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIContractCandidates(r.Context(), deploymentID, contractID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiContractCandidate(w http.ResponseWriter, r *http.Request, contractID, candidateID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIContractCandidate(r.Context(), deploymentID, candidateID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.Candidate.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishAPIContractCandidate(w http.ResponseWriter, r *http.Request, contractID, candidateID string) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input struct {
		ContractRevision    int64 `json:"contract_revision"`
		AcknowledgeReviewed bool  `json:"acknowledge_reviewed"`
	}
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.ContractRevision < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "contract_revision is required.", nil)
		return
	}
	contract, revision, err := s.service.PublishAPIContractCandidate(r.Context(), contractID, candidateID, platform.APIContractCandidatePublicationInput{
		ContractRevision: input.ContractRevision, AcknowledgeReviewed: input.AcknowledgeReviewed,
	}, actor(r))
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"contract": contract, "revision": revision})
}

func (s *Server) apiContractRevisions(w http.ResponseWriter, r *http.Request, contractID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().APIContractRevisions(r.Context(), deploymentID, contractID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) apiContractRevision(w http.ResponseWriter, r *http.Request, contractID, revisionID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().APIContractRevision(r.Context(), deploymentID, revisionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if value.APIContractID != contractID {
		s.storeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
