package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func (s *Server) integrations(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Integrations(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			FamilyKey                string     `json:"family_key"`
			VersionKey               string     `json:"version_key"`
			DisplayName              string     `json:"display_name"`
			Description              string     `json:"description"`
			Visibility               string     `json:"visibility"`
			AcknowledgePublic        bool       `json:"acknowledge_public"`
			Lifecycle                string     `json:"lifecycle"`
			ReplacementIntegrationID string     `json:"replacement_integration_id"`
			SunsetAt                 *time.Time `json:"sunset_at"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateIntegration(r.Context(), platform.IntegrationInput{FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: input.Description, Visibility: model.Visibility(input.Visibility), AcknowledgePublic: input.AcknowledgePublic, Lifecycle: input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt}, actor(r))
		if err != nil {
			s.integrationError(w, err, true)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integration(w http.ResponseWriter, r *http.Request, integrationID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Integration(r.Context(), deployment.ID, integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		revisions, err := s.service.Store().IntegrationRevisions(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		publishStatus, err := s.service.IntegrationPublishStatus(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"integration": value, "revisions": revisions, "publish_status": publishStatus})
	case http.MethodPut:
		var input struct {
			FamilyKey                string     `json:"family_key"`
			VersionKey               string     `json:"version_key"`
			DisplayName              string     `json:"display_name"`
			Description              *string    `json:"description"`
			Visibility               *string    `json:"visibility"`
			AcknowledgePublic        bool       `json:"acknowledge_public"`
			Lifecycle                *string    `json:"lifecycle"`
			ReplacementIntegrationID string     `json:"replacement_integration_id"`
			SunsetAt                 *time.Time `json:"sunset_at"`
			Revision                 *int64     `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.FamilyKey == "" || input.VersionKey == "" || input.DisplayName == "" || input.Description == nil || input.Visibility == nil || input.Lifecycle == nil || input.Revision == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "family_key, version_key, display_name, description, visibility, lifecycle, and revision are required.", nil)
			return
		}
		value, err := s.service.UpdateIntegration(r.Context(), integrationID, platform.IntegrationInput{FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: *input.Description, Visibility: model.Visibility(*input.Visibility), AcknowledgePublic: input.AcknowledgePublic, Lifecycle: *input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt, Revision: *input.Revision}, actor(r))
		if err != nil {
			s.integrationError(w, err, false)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integrationError(w http.ResponseWriter, err error, creating bool) {
	if errors.Is(err, platform.ErrConfirmationRequired) || errors.Is(err, platform.ErrUnsafeForPublic) || errors.Is(err, platform.ErrInvalidVisibility) {
		s.platformError(w, err, "Confirm that this Integration may be exposed on the unauthenticated public catalog.")
		return
	}
	if creating {
		s.creationError(w, err)
		return
	}
	s.productCatalogError(w, err)
}

func (s *Server) integrationAccessConnections(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		AccessConnectionIDs []string `json:"access_connection_ids"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetIntegrationAccessConnections(r.Context(), integrationID, input.AccessConnectionIDs, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) integrationSupportRoute(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		SupportRouteID string `json:"support_route_id"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.SetIntegrationSupportRoute(r.Context(), integrationID, input.SupportRouteID, actor(r))
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) publishIntegration(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		CandidateRevision     int64  `json:"candidate_revision"`
		CandidateManifestHash string `json:"candidate_manifest_hash"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.PublishIntegrationCandidate(r.Context(), integrationID, input.CandidateRevision, input.CandidateManifestHash, actor(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "integration_publish_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) preflightIntegration(w http.ResponseWriter, r *http.Request, integrationID string) {
	value, err := s.service.IntegrationPreflight(r.Context(), integrationID)
	if err != nil {
		s.productCatalogError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}
