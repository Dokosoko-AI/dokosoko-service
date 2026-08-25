package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/reporting"
)

type supportRouteRequest struct {
	Name                string    `json:"name"`
	IsDefault           *bool     `json:"is_default"`
	BugReportsEnabled   *bool     `json:"bug_reports_enabled"`
	FeedbackEnabled     *bool     `json:"feedback_enabled"`
	BackendConnectionID string    `json:"backend_connection_id"`
	RetentionDays       *int      `json:"retention_days"`
	State               *string   `json:"state"`
	IntegrationIDs      *[]string `json:"integration_ids"`
	Revision            *int64    `json:"revision"`
}

func (s *Server) supportRoutes(w http.ResponseWriter, r *http.Request) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Support routing is not configured.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SupportRoutes(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input supportRouteRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		route, err := routeInput(input, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, "", route, actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.supportRouteError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) supportRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Support routing is not configured.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().SupportRoute(r.Context(), deployment.ID, routeID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var input supportRouteRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		route, err := routeInput(input, true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, routeID, route, actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.supportRouteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func routeInput(input supportRouteRequest, replacing bool) (reporting.RouteInput, error) {
	if input.IsDefault == nil || input.BugReportsEnabled == nil || input.FeedbackEnabled == nil || input.RetentionDays == nil || input.State == nil || input.IntegrationIDs == nil {
		return reporting.RouteInput{}, errors.New("is_default, bug_reports_enabled, feedback_enabled, retention_days, state, and integration_ids are required")
	}
	if replacing && input.Revision == nil {
		return reporting.RouteInput{}, errors.New("revision is required when replacing a support route")
	}
	if !replacing && input.Revision != nil {
		return reporting.RouteInput{}, errors.New("revision is not allowed when creating a support route")
	}
	revision := int64(0)
	if input.Revision != nil {
		revision = *input.Revision
	}
	return reporting.RouteInput{Name: input.Name, IsDefault: *input.IsDefault, BugReportsEnabled: *input.BugReportsEnabled, FeedbackEnabled: *input.FeedbackEnabled, BackendConnectionID: input.BackendConnectionID, RetentionDays: *input.RetentionDays, State: *input.State, IntegrationIDs: *input.IntegrationIDs, Revision: revision}, nil
}

func (s *Server) supportRouteError(w http.ResponseWriter, err error) {
	if errors.Is(err, reporting.ErrInvalidReport) {
		writeError(w, http.StatusBadRequest, "invalid_support_route", err.Error(), nil)
		return
	}
	s.storeError(w, err)
}
