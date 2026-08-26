package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func (s *Server) developerAssetQueryLab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		developerAssetMethodNotAllowed(w, http.MethodPost)
		return
	}
	var input platform.DeveloperAssetQueryLabInput
	if err := decodeDeveloperAssetJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RunDeveloperAssetQueryLab(r.Context(), input)
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) developerAssetQueryTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	limit, err := developerAssetQueryLimit(r, 100, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		since, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "since must be an RFC 3339 timestamp.", nil)
			return
		}
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().RetrievalQueryTraces(r.Context(), deploymentID, since, limit)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) developerAssetQueryTrace(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	deploymentID, err := s.developerAssetDeploymentID(r)
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().RetrievalQueryTrace(r.Context(), deploymentID, traceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
