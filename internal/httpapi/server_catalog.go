package httpapi

import (
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type deploymentFeatures struct {
	Widgets bool `json:"widgets"`
}

type deploymentResponse struct {
	model.Deployment
	Features deploymentFeatures `json:"features"`
}

func (s *Server) deploymentResponse(value model.Deployment) deploymentResponse {
	return deploymentResponse{Deployment: value, Features: deploymentFeatures{Widgets: s.widgetsEnabled}}
}

func (s *Server) deployment(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.deploymentResponse(value))
	case http.MethodPost:
		var input struct {
			OrganisationID           string `json:"organisation_id"`
			Name                     string `json:"name"`
			Slug                     string `json:"slug"`
			Description              string `json:"description"`
			DefaultReleasePolicy     string `json:"default_release_policy"`
			RequirePromotionApproval bool   `json:"require_promotion_approval"`
			PublicMCPEnabled         bool   `json:"public_mcp_enabled"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateDeployment(r.Context(), platform.DeploymentInput{OrganisationID: input.OrganisationID, Name: input.Name, Slug: input.Slug, Description: input.Description, DefaultReleasePolicy: input.DefaultReleasePolicy, RequirePromotionApproval: input.RequirePromotionApproval, PublicMCPEnabled: input.PublicMCPEnabled}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, s.deploymentResponse(value))
	case http.MethodPatch:
		var input struct {
			Name                     string `json:"name"`
			Slug                     string `json:"slug"`
			Description              string `json:"description"`
			DefaultReleasePolicy     string `json:"default_release_policy"`
			RequirePromotionApproval bool   `json:"require_promotion_approval"`
			PublicMCPEnabled         bool   `json:"public_mcp_enabled"`
			Revision                 int64  `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateDeployment(r.Context(), platform.DeploymentInput{Name: input.Name, Slug: input.Slug, Description: input.Description, DefaultReleasePolicy: input.DefaultReleasePolicy, RequirePromotionApproval: input.RequirePromotionApproval, PublicMCPEnabled: input.PublicMCPEnabled, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.deploymentResponse(value))
	default:
		w.Header().Set("Allow", "GET, POST, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) deploymentEnvironments(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	s.environments(w, r, deployment.ID)
}
