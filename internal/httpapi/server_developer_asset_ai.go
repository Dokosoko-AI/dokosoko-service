package httpapi

import (
	"errors"
	"net/http"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) developerAssetAIAdvisoryError(w http.ResponseWriter, err error) {
	var providerError *airuntime.Error
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrAIUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "The Analysis AI workload is disabled or unconfigured.", nil)
	case errors.Is(err, platform.ErrDeveloperAssetAIAdvisoryInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_ai_advisory", "The advisory scope or structured suggestion is invalid.", nil)
	case errors.As(err, &providerError) && providerError.Code == airuntime.ErrorUnsafeInput:
		writeError(w, http.StatusUnprocessableEntity, "invalid_ai_advisory", "The selected evidence cannot be sent to the configured AI provider.", nil)
	case errors.As(err, &providerError):
		writeError(w, http.StatusBadGateway, "ai_advisory_failed", "The AI provider did not return a valid advisory result.", nil)
	default:
		writeError(w, http.StatusBadGateway, "ai_advisory_failed", "The developer-asset advisory could not be completed.", nil)
	}
}

func (s *Server) developerAssetAIAdvisories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, err := developerAssetQueryLimit(r, 100, 200)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		items, err := s.service.DeveloperAssetAIAdvisoryRuns(r.Context(), strings.TrimSpace(r.URL.Query().Get("prompt_key")), strings.TrimSpace(r.URL.Query().Get("scope_id")), limit)
		if err != nil {
			s.developerAssetAIAdvisoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var input platform.DeveloperAssetAIAdvisoryInput
		if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.RunDeveloperAssetAIAdvisory(r.Context(), input, actor(r))
		if err != nil {
			s.developerAssetAIAdvisoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		developerAssetMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) developerAssetAIAdvisory(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetAIAdvisoryRun(r.Context(), id)
	if err != nil {
		s.developerAssetAIAdvisoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
