package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const maxToolTestAnalysisRequestBytes = 32 << 10

type toolTestAnalysisRequest struct {
	Revision      int64                              `json:"revision"`
	EvidenceHash  string                             `json:"evidence_hash"`
	ConsentToSend *bool                              `json:"consent_to_analysis_provider"`
	Question      string                             `json:"question"`
	History       []platform.ToolTestAnalysisMessage `json:"history,omitempty"`
}

func (s *Server) toolTestAnalysisError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrToolTestAnalysisConsentRequired):
		writeError(w, http.StatusBadRequest, "tool_test_analysis_consent_required", "Explicit consent is required before sanitized evidence is sent to the configured Analysis provider.", nil)
	case errors.Is(err, platform.ErrToolTestAnalysisEvidenceMismatch):
		writeError(w, http.StatusConflict, "tool_test_analysis_evidence_mismatch", "The evidence preview no longer matches this exact live-test run. Refresh the evidence before analysing it.", nil)
	case errors.Is(err, platform.ErrToolTestAnalysisInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_tool_test_analysis", "The analysis question or bounded conversation history is invalid or contains forbidden material.", nil)
	case errors.Is(err, platform.ErrToolBuilderUnsafeInput):
		writeError(w, http.StatusBadRequest, "credential_material_forbidden", "Credential material cannot be sent with live-test analysis. Configure credentials separately.", nil)
	case errors.Is(err, platform.ErrAIUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "The Analysis AI workload is not configured or is temporarily unavailable.", nil)
	case errors.Is(err, platform.ErrToolTestRevisionStale):
		writeError(w, http.StatusConflict, "tool_test_revision_stale", "The tool revision changed. Run a new live test before requesting analysis.", nil)
	case errors.Is(err, platform.ErrToolTestNotEligible):
		writeError(w, http.StatusConflict, "tool_test_not_eligible", "This exact stored tool revision is no longer eligible for live-test analysis.", nil)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "tool_test_analysis_failed", "The sanitized live-test evidence could not be analysed safely.", nil)
	}
}

func (s *Server) analyseToolTestRun(w http.ResponseWriter, r *http.Request, productID, toolID, runID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input toolTestAnalysisRequest
	if err := decodeToolBuilderJSON(w, r, maxToolTestAnalysisRequestBytes, &input); err != nil || input.ConsentToSend == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object with an explicit consent boolean.", nil)
		return
	}
	value, err := s.service.AnalyseToolTestRun(r.Context(), productID, toolID, runID, platform.ToolTestAnalysisInput{
		Revision: input.Revision, EvidenceHash: input.EvidenceHash, ConsentToSend: *input.ConsentToSend, Question: input.Question, History: input.History,
	}, actor(r))
	if err != nil {
		s.toolTestAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
