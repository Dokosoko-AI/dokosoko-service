package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	maxToolBuilderRequestBytes       = 256 << 10
	maxToolBuilderImportRequestBytes = 600 << 10
)

func decodeToolBuilderJSON(w http.ResponseWriter, r *http.Request, limit int64, value any) error {
	reader := http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
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

func (s *Server) toolBuilderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrToolBuilderUnsafeInput):
		writeError(w, http.StatusBadRequest, "credential_material_forbidden", "Credential material cannot be submitted to tool-builder assistance endpoints. Configure credentials separately.", nil)
	case errors.Is(err, platform.ErrToolBuilderInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_tool_builder_input", "The tool-builder input is invalid or unsupported.", nil)
	case errors.Is(err, platform.ErrAIUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ai_unavailable", "The Analysis AI workload is not configured or is temporarily unavailable.", nil)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "tool_builder_failed", "The tool-builder request could not be completed safely.", nil)
	}
}

func (s *Server) toolBuilder(w http.ResponseWriter, r *http.Request, productID, action string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	switch action {
	case "validate":
		var input platform.ToolDraftContext
		if err := decodeToolBuilderJSON(w, r, maxToolBuilderRequestBytes, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the tool-draft contract.", nil)
			return
		}
		result, err := s.service.ValidateToolDraftContext(r.Context(), productID, input)
		if err != nil {
			s.toolBuilderError(w, err)
			return
		}
		if err := s.service.AuditToolDraftValidation(r.Context(), productID, result, actor(r)); err != nil {
			s.toolBuilderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "propose":
		var input platform.ToolDraftProposalInput
		if err := decodeToolBuilderJSON(w, r, maxToolBuilderRequestBytes, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the proposal contract.", nil)
			return
		}
		result, err := s.service.ProposeToolDraft(r.Context(), productID, input, actor(r))
		if err != nil {
			s.toolBuilderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "import":
		var input platform.ToolDraftImportInput
		if err := decodeToolBuilderJSON(w, r, maxToolBuilderImportRequestBytes, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the import contract.", nil)
			return
		}
		result, err := s.service.ImportToolDraft(r.Context(), productID, input, actor(r))
		if err != nil {
			s.toolBuilderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "analyse":
		var input platform.ToolDraftAnalysisInput
		if err := decodeToolBuilderJSON(w, r, maxToolBuilderRequestBytes, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Request body must be one strict, bounded JSON object matching the analysis contract.", nil)
			return
		}
		result, err := s.service.AnalyseToolDraft(r.Context(), productID, input, actor(r))
		if err != nil {
			s.toolBuilderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Tool-builder action not found.", nil)
	}
}
