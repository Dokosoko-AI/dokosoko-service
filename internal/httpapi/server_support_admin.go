package httpapi

import (
	"errors"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"net/http"
	"strconv"
)

func (s *Server) supportSubmissions(w http.ResponseWriter, r *http.Request) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 200.", nil)
			return
		}
		limit = parsed
	}
	startingAfter := r.URL.Query().Get("starting_after")
	if len(startingAfter) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after is invalid.", nil)
		return
	}
	values, hasMore, err := s.reporting.Submissions(r.Context(), deployment.ID, startingAfter, limit)
	if startingAfter != "" && errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_request", "starting_after does not identify a support submission in this deployment.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "has_more": hasMore})
}

func (s *Server) supportSubmission(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Submission(r.Context(), deployment.ID, submissionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
