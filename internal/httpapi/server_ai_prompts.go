package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) aiPromptConfigurations(w http.ResponseWriter, r *http.Request, productID string) {
	values, err := s.service.AIPromptConfigurations(r.Context(), productID)
	if err != nil {
		s.aiPromptError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) saveAIPromptOverride(w http.ResponseWriter, r *http.Request, productID, key string) {
	var input struct {
		Instructions string `json:"instructions"`
		Revision     int64  `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SaveAIPromptOverride(r.Context(), productID, key, input.Instructions, input.Revision, actor(r))
	if err != nil {
		s.aiPromptError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) resetAIPromptOverride(w http.ResponseWriter, r *http.Request, productID, key string) {
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.ResetAIPromptOverride(r.Context(), productID, key, input.Revision, actor(r))
	if err != nil {
		s.aiPromptError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) aiPromptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrAIPromptInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_ai_prompt", err.Error(), nil)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "The request could not be completed.", nil)
	}
}
