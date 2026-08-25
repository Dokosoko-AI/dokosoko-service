package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request, organisationID string) {
	values, err := s.service.Store().AuditEvents(r.Context(), organisationID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if len(values) > 500 {
		values = values[:500]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) integrationRuns(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().IntegrationRuns(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			EnvironmentID    string `json:"environment_id"`
			RequestedOutcome string `json:"requested_outcome"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.StartIntegrationRun(r.Context(), productID, input.EnvironmentID, input.RequestedOutcome, actor(r))
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

func (s *Server) completeIntegrationRun(w http.ResponseWriter, r *http.Request, productID, runID string) {
	var input struct {
		ReportedSuccess  *bool  `json:"reported_success"`
		ValidatedSuccess *bool  `json:"validated_success"`
		FailureCode      string `json:"failure_code"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.CompleteIntegrationRun(r.Context(), productID, runID, input.ReportedSuccess, input.ValidatedSuccess, input.FailureCode, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "integration_run_complete", "The integration run was already completed.", nil)
			return
		}
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
