package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type sdkReferenceRequest struct {
	Ecosystem        string           `json:"ecosystem"`
	Coordinate       string           `json:"coordinate"`
	ExactVersion     string           `json:"exact_version"`
	InstallCommand   string           `json:"install_command"`
	DocumentationURL string           `json:"documentation_url"`
	SourceURL        string           `json:"source_url"`
	Checksum         string           `json:"checksum"`
	Visibility       model.Visibility `json:"visibility"`
	Revision         int64            `json:"revision"`
}

func sdkReferenceInput(input sdkReferenceRequest) platform.SDKReferenceInput {
	return platform.SDKReferenceInput{Ecosystem: input.Ecosystem, Coordinate: input.Coordinate, ExactVersion: input.ExactVersion, InstallCommand: input.InstallCommand, DocumentationURL: input.DocumentationURL, SourceURL: input.SourceURL, Checksum: input.Checksum, Visibility: input.Visibility, Revision: input.Revision}
}

func (s *Server) integrationSDKs(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SDKReferences(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input sdkReferenceRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.SaveSDKReference(r.Context(), integrationID, "", sdkReferenceInput(input), actor(r))
		if err != nil {
			s.sdkReferenceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integrationSDK(w http.ResponseWriter, r *http.Request, integrationID, referenceID string) {
	switch r.Method {
	case http.MethodPut:
		var input sdkReferenceRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
			return
		}
		value, err := s.service.SaveSDKReference(r.Context(), integrationID, referenceID, sdkReferenceInput(input), actor(r))
		if err != nil {
			s.sdkReferenceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodDelete:
		if err := s.service.DeleteSDKReference(r.Context(), integrationID, referenceID, actor(r)); err != nil {
			s.sdkReferenceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) sdkReferenceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusBadRequest, "invalid_sdk_reference", err.Error(), nil)
	}
}
