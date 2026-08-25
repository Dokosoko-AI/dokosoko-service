package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/store"
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

func (s *Server) createSupportDeliveryAttempt(w http.ResponseWriter, r *http.Request, submissionID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Reporting is unavailable.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.reporting.Retry(r.Context(), deployment.ID, submissionID)
	if errors.Is(err, reporting.ErrDeliveryUnavailable) {
		writeError(w, http.StatusConflict, "reporting_delivery_unavailable", "Configure support delivery before retrying.", nil)
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "submission_not_retryable", "Only unexpired held or failed submissions can be retried.", nil)
		return
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	currentActor := actor(r)
	requestID, _ := r.Context().Value(requestIDKey).(string)
	if err := s.service.Store().AppendAudit(r.Context(), model.AuditEvent{ID: "audit_" + strconv.FormatInt(time.Now().UnixNano(), 10), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: currentActor.ID, Action: "support_submission.delivery_attempt_created", TargetType: "support_submission", TargetID: submissionID, Current: map[string]any{"kind": value.Kind, "state": value.State}, RequestID: requestID, CreatedAt: time.Now().UTC()}); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, value)
}
