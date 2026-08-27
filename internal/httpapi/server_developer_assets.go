package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	maxDeveloperAssetRequestBytes            = 1 << 20
	maxSDKContentIngestionRequestBytes int64 = 24 << 20
)

// decodeDeveloperAssetJSON is deliberately stricter than the historical
// control-plane decoder: one JSON value is required, unknown fields and
// trailing values are rejected, and the body is bounded.
func decodeDeveloperAssetJSON(reader io.Reader, value any) error {
	return decodeDeveloperAssetJSONLimit(reader, value, maxDeveloperAssetRequestBytes)
}

func decodeDeveloperAssetJSONLimit(reader io.Reader, value any, maximum int64) error {
	limited := &io.LimitedReader{R: reader, N: maximum + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: request body must contain exactly one value")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("invalid JSON: request body exceeds %d bytes", maximum)
	}
	return nil
}

func developerAssetMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
}

func (s *Server) developerAssetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrSourceReviewRequired):
		writeError(w, http.StatusUnprocessableEntity, "human_review_required", "Explicit human review is required before publication.", nil)
	case errors.Is(err, platform.ErrUnsafeForPublic):
		writeError(w, http.StatusUnprocessableEntity, "unsafe_for_public", err.Error(), nil)
	case errors.Is(err, platform.ErrConfirmationRequired):
		writeError(w, http.StatusConflict, "confirmation_required", err.Error(), nil)
	case errors.Is(err, platform.ErrSDKReleaseUnavailable):
		writeError(w, http.StatusConflict, "sdk_release_unavailable", err.Error(), nil)
	case errors.Is(err, platform.ErrSDKImportConflict):
		writeError(w, http.StatusConflict, "sdk_import_conflict", err.Error(), nil)
	case errors.Is(err, platform.ErrInvalidVisibility):
		writeError(w, http.StatusBadRequest, "invalid_visibility", err.Error(), nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_developer_asset", err.Error(), nil)
	}
}

func developerAssetQueryLimit(r *http.Request, defaultValue, maximum int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return value, nil
}

func developerAssetQueryOffset(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("offset"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("offset must be zero or greater")
	}
	return value, nil
}

func (s *Server) developerAssetDeploymentID(r *http.Request) (string, error) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		return "", err
	}
	return deployment.ID, nil
}

func (s *Server) developerAssetCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetCatalog(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) developerAssetUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetUsage(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) developerAssetIngestionRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	kind := model.DeveloperAssetKind(strings.TrimSpace(r.URL.Query().Get("asset_kind")))
	targetKey := strings.TrimSpace(r.URL.Query().Get("target_key"))
	if len(targetKey) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_request", "target_key must be no more than 500 characters.", nil)
		return
	}
	values, err := s.service.DeveloperAssetIngestionRuns(r.Context(), kind, targetKey)
	if err != nil {
		s.developerAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) developerAssetIngestionRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		developerAssetMethodNotAllowed(w, http.MethodGet)
		return
	}
	value, err := s.service.DeveloperAssetIngestion(r.Context(), runID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
