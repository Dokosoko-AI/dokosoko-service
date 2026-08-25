package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func (s *Server) resourceSets(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ResourceSets(r.Context(), deployment.ID, strings.TrimSpace(r.URL.Query().Get("kind")))
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Kind        string          `json:"kind"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Manifest    json.RawMessage `json:"manifest"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateResourceSet(r.Context(), platform.ResourceSetInput{Kind: input.Kind, Name: input.Name, Description: input.Description, State: "active", Manifest: input.Manifest}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) resourceSet(w http.ResponseWriter, r *http.Request, setID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().ResourceSet(r.Context(), deployment.ID, setID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			State       string          `json:"state"`
			Manifest    json.RawMessage `json:"manifest"`
			Revision    int64           `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateResourceSet(r.Context(), setID, platform.ResourceSetInput{Name: input.Name, Description: input.Description, State: input.State, Manifest: input.Manifest, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) duplicateResourceSet(w http.ResponseWriter, r *http.Request, setID string) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.DuplicateResourceSet(r.Context(), setID, input.Name, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) resourceSetRevisions(w http.ResponseWriter, r *http.Request, setID string) {
	values, err := s.service.Store().ResourceSetRevisions(r.Context(), setID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) attachResourceSet(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		ResourceSetID    string `json:"resource_set_id"`
		PinnedRevisionID string `json:"pinned_revision_id"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.AttachResourceSet(r.Context(), integrationID, input.ResourceSetID, input.PinnedRevisionID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) detachResourceSet(w http.ResponseWriter, r *http.Request, integrationID, setID string) {
	if err := s.service.DetachResourceSet(r.Context(), integrationID, setID, actor(r)); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
