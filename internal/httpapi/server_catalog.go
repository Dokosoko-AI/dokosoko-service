package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
)

func (s *Server) deployment(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().Deployment(r.Context())
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
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
		writeJSON(w, http.StatusCreated, value)
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
		writeJSON(w, http.StatusOK, value)
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
			Lifecycle                string     `json:"lifecycle"`
			ReplacementIntegrationID string     `json:"replacement_integration_id"`
			SunsetAt                 *time.Time `json:"sunset_at"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateIntegration(r.Context(), platform.IntegrationInput{FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: input.Description, Lifecycle: input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt}, actor(r))
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
		revisions, _ := s.service.Store().IntegrationRevisions(r.Context(), integrationID)
		publishStatus, err := s.service.IntegrationPublishStatus(r.Context(), integrationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"integration": value, "revisions": revisions, "publish_status": publishStatus})
	case http.MethodPatch:
		var input struct {
			FamilyKey                string     `json:"family_key"`
			VersionKey               string     `json:"version_key"`
			DisplayName              string     `json:"display_name"`
			Description              string     `json:"description"`
			Lifecycle                string     `json:"lifecycle"`
			ReplacementIntegrationID string     `json:"replacement_integration_id"`
			SunsetAt                 *time.Time `json:"sunset_at"`
			Revision                 int64      `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateIntegration(r.Context(), integrationID, platform.IntegrationInput{FamilyKey: input.FamilyKey, VersionKey: input.VersionKey, DisplayName: input.DisplayName, Description: input.Description, Lifecycle: input.Lifecycle, ReplacementIntegrationID: input.ReplacementIntegrationID, SunsetAt: input.SunsetAt, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
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
	value, err := s.service.PublishIntegration(r.Context(), integrationID, actor(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "integration_publish_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) resourceSets(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().ResourceSets(r.Context(), deployment.ID, strings.TrimSpace(r.URL.Query().Get("kind")))
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Kind        string          `json:"kind"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Manifest    json.RawMessage `json:"manifest"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateResourceSet(r.Context(), platform.ResourceSetInput{Kind: input.Kind, Name: input.Name, Description: input.Description, State: "active", Manifest: input.Manifest}, actor(r))
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

func (s *Server) resourceSet(w http.ResponseWriter, r *http.Request, setID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().ResourceSet(r.Context(), deployment.ID, setID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			State       string          `json:"state"`
			Manifest    json.RawMessage `json:"manifest"`
			Revision    int64           `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateResourceSet(r.Context(), setID, platform.ResourceSetInput{Name: input.Name, Description: input.Description, State: input.State, Manifest: input.Manifest, Revision: input.Revision}, actor(r))
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) duplicateResourceSet(w http.ResponseWriter, r *http.Request, setID string) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.DuplicateResourceSet(r.Context(), setID, input.Name, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) resourceSetRevisions(w http.ResponseWriter, r *http.Request, setID string) {
	values, err := s.service.Store().ResourceSetRevisions(r.Context(), setID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) attachResourceSet(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		ResourceSetID    string `json:"resource_set_id"`
		PinnedRevisionID string `json:"pinned_revision_id"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.AttachResourceSet(r.Context(), integrationID, input.ResourceSetID, input.PinnedRevisionID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) detachResourceSet(w http.ResponseWriter, r *http.Request, integrationID, setID string) {
	if err := s.service.DetachResourceSet(r.Context(), integrationID, setID, actor(r)); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) accessDefinitions(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().AccessDefinitions(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			ServiceKey            string          `json:"service_key"`
			Name                  string          `json:"name"`
			InstanceCardinality   string          `json:"instance_cardinality"`
			InstanceLabelSingular string          `json:"instance_label_singular"`
			InstanceLabelPlural   string          `json:"instance_label_plural"`
			CredentialScope       string          `json:"credential_scope"`
			ManagementAuthType    string          `json:"management_auth_type"`
			HookSetID             string          `json:"hook_set_id"`
			Operations            json.RawMessage `json:"operations"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateAccessDefinition(r.Context(), platform.AccessDefinitionInput{ServiceKey: input.ServiceKey, Name: input.Name, InstanceCardinality: input.InstanceCardinality, InstanceLabelSingular: input.InstanceLabelSingular, InstanceLabelPlural: input.InstanceLabelPlural, CredentialScope: input.CredentialScope, ManagementAuthType: input.ManagementAuthType, HookSetID: input.HookSetID, Operations: input.Operations}, actor(r))
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

func (s *Server) accessConnections(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().AccessConnections(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			AccessDefinitionID string          `json:"access_definition_id"`
			EnvironmentID      string          `json:"environment_id"`
			Name               string          `json:"name"`
			Region             string          `json:"region"`
			BaseURL            string          `json:"base_url"`
			ManagementSecret   string          `json:"management_secret"`
			Config             json.RawMessage `json:"config"`
			IntegrationIDs     []string        `json:"integration_ids"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateAccessConnection(r.Context(), platform.AccessConnectionInput{AccessDefinitionID: input.AccessDefinitionID, EnvironmentID: input.EnvironmentID, Name: input.Name, Region: input.Region, BaseURL: input.BaseURL, ManagementSecret: input.ManagementSecret, Config: input.Config, IntegrationIDs: input.IntegrationIDs}, actor(r))
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

func (s *Server) accessInstances(w http.ResponseWriter, r *http.Request, connectionID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	values, err := s.service.Store().AccessInstances(r.Context(), deployment.ID, connectionID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) accessCredentials(w http.ResponseWriter, r *http.Request, connectionID, instanceID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	if instanceID != "" {
		instance, err := s.service.Store().AccessInstance(r.Context(), deployment.ID, instanceID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		connectionID = instance.AccessConnectionID
	}
	values, err := s.service.Store().AccessCredentials(r.Context(), deployment.ID, connectionID, instanceID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

type supportRouteRequest struct {
	Name                   string   `json:"name"`
	IsDefault              bool     `json:"is_default"`
	BugReportsEnabled      bool     `json:"bug_reports_enabled"`
	FeedbackEnabled        bool     `json:"feedback_enabled"`
	BugHookURL             string   `json:"bug_hook_url"`
	BugHookCredential      string   `json:"bug_hook_credential"`
	FeedbackHookURL        string   `json:"feedback_hook_url"`
	FeedbackHookCredential string   `json:"feedback_hook_credential"`
	RetentionDays          int      `json:"retention_days"`
	State                  string   `json:"state"`
	IntegrationIDs         []string `json:"integration_ids"`
	Revision               int64    `json:"revision"`
}

func (s *Server) supportRoutes(w http.ResponseWriter, r *http.Request) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Support routing is not configured.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().SupportRoutes(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input supportRouteRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, "", routeInput(input), actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) supportRoute(w http.ResponseWriter, r *http.Request, routeID string) {
	if s.reporting == nil {
		writeError(w, http.StatusServiceUnavailable, "reporting_unavailable", "Support routing is not configured.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().SupportRoute(r.Context(), deployment.ID, routeID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input supportRouteRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, routeID, routeInput(input), actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.productCatalogError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func routeInput(input supportRouteRequest) reporting.RouteInput {
	return reporting.RouteInput{Name: input.Name, IsDefault: input.IsDefault, BugReportsEnabled: input.BugReportsEnabled, FeedbackEnabled: input.FeedbackEnabled, BugHookURL: input.BugHookURL, BugHookCredential: input.BugHookCredential, FeedbackHookURL: input.FeedbackHookURL, FeedbackHookCredential: input.FeedbackHookCredential, RetentionDays: input.RetentionDays, State: input.State, IntegrationIDs: input.IntegrationIDs, Revision: input.Revision}
}
