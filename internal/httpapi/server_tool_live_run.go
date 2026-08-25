package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

const maxToolTestRequestBytes = 128 << 10

func decodeToolTestJSON(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxToolTestRequestBytes))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type toolTestConfirmationRequest struct {
	Revision               int64          `json:"revision"`
	Arguments              map[string]any `json:"arguments"`
	TypedToolName          string         `json:"typed_tool_name"`
	AcknowledgeSideEffects bool           `json:"acknowledge_side_effects"`
}

type toolTestRunRequest struct {
	Revision          int64          `json:"revision"`
	Arguments         map[string]any `json:"arguments"`
	ConfirmationNonce string         `json:"confirmation_nonce"`
	IdempotencyKey    string         `json:"idempotency_key"`
}

type toolTestRunResponse struct {
	model.ToolTestRun
	EvidenceHash string `json:"evidence_hash"`
}

func toolTestRunForResponse(value model.ToolTestRun) toolTestRunResponse {
	return toolTestRunResponse{ToolTestRun: value, EvidenceHash: platform.ToolTestAnalysisEvidenceHash(value)}
}

func (s *Server) toolTestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrToolTestRevisionStale):
		writeError(w, http.StatusConflict, "tool_test_revision_stale", "The tool draft changed. Refresh it before testing.", nil)
	case errors.Is(err, platform.ErrToolTestConfirmationReplayed):
		writeError(w, http.StatusConflict, "tool_test_confirmation_replayed", "This live-test confirmation has already been used.", nil)
	case errors.Is(err, platform.ErrToolTestConfirmationInvalid):
		writeError(w, http.StatusConflict, "tool_test_confirmation_invalid", "The live-test confirmation is invalid, expired, or does not match this exact test.", nil)
	case errors.Is(err, platform.ErrToolTestConsentInvalid):
		writeError(w, http.StatusBadRequest, "tool_test_consent_invalid", "Type the full tool name and acknowledge the live test before continuing.", nil)
	case errors.Is(err, toolruntime.ErrInvalidIdempotencyKey):
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "This mutation requires an explicit 16-200 character visible-ASCII idempotency key.", nil)
	case errors.Is(err, platform.ErrToolTestUnavailable):
		writeError(w, http.StatusServiceUnavailable, "tool_test_unavailable", "The hardened HTTP tool test runtime is unavailable.", nil)
	case errors.Is(err, platform.ErrToolTestNotEligible):
		writeError(w, http.StatusConflict, "tool_test_not_eligible", "This exact stored tool revision is not eligible for a live HTTP test.", nil)
	case errors.Is(err, platform.ErrToolTestRequiresReview):
		writeError(w, http.StatusUnprocessableEntity, "tool_test_requires_review", "Review and save the stored HTTP tool before running a live test.", nil)
	case errors.Is(err, platform.ErrToolTestOutcomeIndeterminate):
		writeError(w, http.StatusBadGateway, "tool_test_outcome_indeterminate", "The upstream may have completed the call, but DokoSoko could not durably record the outcome. Do not retry; inspect Activity and the upstream system.", nil)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusBadRequest, "tool_test_invalid", "The live tool test request is invalid or cannot be completed safely.", nil)
	}
}

func (s *Server) createToolTestConfirmation(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	w.Header().Set("Cache-Control", "no-store")
	var input toolTestConfirmationRequest
	if err := decodeToolTestJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the tool-test confirmation contract.", nil)
		return
	}
	if input.Arguments == nil {
		input.Arguments = map[string]any{}
	}
	value, err := s.service.CreateToolTestConfirmation(r.Context(), productID, toolID, platform.ToolTestConfirmationInput{Revision: input.Revision, Arguments: input.Arguments, TypedToolName: input.TypedToolName, AcknowledgeSideEffects: input.AcknowledgeSideEffects}, actor(r))
	if err != nil {
		s.toolTestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) toolTestRuns(w http.ResponseWriter, r *http.Request, productID, toolID string) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.ToolTestRuns(r.Context(), productID, toolID)
		if err != nil {
			s.toolTestError(w, err)
			return
		}
		items := make([]toolTestRunResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toolTestRunForResponse(value))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var input toolTestRunRequest
		if err := decodeToolTestJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the tool-test run contract.", nil)
			return
		}
		if input.Arguments == nil {
			input.Arguments = map[string]any{}
		}
		value, err := s.service.RunToolTest(r.Context(), s.toolRuntime, productID, toolID, platform.ToolTestRunInput{Revision: input.Revision, Arguments: input.Arguments, ConfirmationNonce: input.ConfirmationNonce, IdempotencyKey: input.IdempotencyKey}, actor(r))
		if err != nil {
			s.toolTestError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toolTestRunForResponse(value))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) toolTestRun(w http.ResponseWriter, r *http.Request, productID, toolID, runID string) {
	w.Header().Set("Cache-Control", "no-store")
	value, err := s.service.ToolTestRun(r.Context(), productID, toolID, runID)
	if err != nil {
		s.toolTestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toolTestRunForResponse(value))
}
