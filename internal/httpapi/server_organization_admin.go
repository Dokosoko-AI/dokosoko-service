package httpapi

import (
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type namedResourceInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) organisations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Organisations(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateOrganisation(r.Context(), input.Name, input.Slug, actor(r))
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

func (s *Server) products(w http.ResponseWriter, r *http.Request, organisationID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Products(r.Context(), organisationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input namedResourceInput
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateProduct(r.Context(), organisationID, input.Name, input.Slug, actor(r))
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

func (s *Server) environments(w http.ResponseWriter, r *http.Request, productID string) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().Environments(r.Context(), productID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			namedResourceInput
			OrganisationID string `json:"organisation_id"`
			IsProduction   bool   `json:"is_production"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateEnvironment(r.Context(), input.OrganisationID, productID, input.Name, input.Slug, input.IsProduction, actor(r))
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

func (s *Server) creationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "resource_conflict", "A resource with this slug or production role already exists.", nil)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		s.storeError(w, err)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_resource", err.Error(), nil)
}

func (s *Server) productCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		s.storeError(w, err)
	case errors.Is(err, platform.ErrProductDescriptionRequired):
		writeError(w, http.StatusUnprocessableEntity, "product_description_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDeprecated):
		writeError(w, http.StatusConflict, "product_version_deprecated", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionLifecycle):
		writeError(w, http.StatusUnprocessableEntity, "invalid_product_version_lifecycle", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionDrifted):
		writeError(w, http.StatusConflict, "product_version_drifted", err.Error(), nil)
	case errors.Is(err, platform.ErrPromotionApprovalRequired), errors.Is(err, platform.ErrPromotionSeparationOfDuties):
		writeError(w, http.StatusConflict, "promotion_approval_required", err.Error(), nil)
	case errors.Is(err, platform.ErrProductVersionImpact):
		writeError(w, http.StatusConflict, "product_version_impact_acknowledgement_required", err.Error(), nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_product_catalog", err.Error(), nil)
	}
}
