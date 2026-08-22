package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/store"
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
		revisions, _ := s.service.Store().IntegrationRevisions(r.Context(), integrationID)
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
			APIResourceSetID      string          `json:"api_resource_set_id"`
			Operations            json.RawMessage `json:"operations"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateAccessDefinition(r.Context(), platform.AccessDefinitionInput{ServiceKey: input.ServiceKey, Name: input.Name, InstanceCardinality: input.InstanceCardinality, InstanceLabelSingular: input.InstanceLabelSingular, InstanceLabelPlural: input.InstanceLabelPlural, CredentialScope: input.CredentialScope, ManagementAuthType: input.ManagementAuthType, APIResourceSetID: input.APIResourceSetID, Operations: input.Operations}, actor(r))
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

func (s *Server) backendConnections(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := s.service.Store().BackendConnections(r.Context(), deployment.ID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": values})
	case http.MethodPost:
		var input struct {
			Name               string `json:"name"`
			BaseURL            string `json:"base_url"`
			AuthenticationType string `json:"authentication_type"`
			Credential         string `json:"credential"`
			State              string `json:"state"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.CreateBackendConnection(r.Context(), platform.BackendConnectionInput{Name: input.Name, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, Credential: input.Credential, State: input.State}, actor(r))
		if err != nil {
			s.backendConnectionError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) backendConnection(w http.ResponseWriter, r *http.Request, connectionID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().BackendConnection(r.Context(), deployment.ID, connectionID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var input struct {
			Name               string `json:"name"`
			BaseURL            string `json:"base_url"`
			AuthenticationType string `json:"authentication_type"`
			State              string `json:"state"`
			Revision           *int64 `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if input.Revision == nil || input.Name == "" || input.BaseURL == "" || input.AuthenticationType == "" || input.State == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "name, base_url, authentication_type, state, and revision are required.", nil)
			return
		}
		value, err := s.service.UpdateBackendConnection(r.Context(), connectionID, platform.BackendConnectionInput{Name: input.Name, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, State: input.State, Revision: *input.Revision}, actor(r))
		if err != nil {
			s.backendConnectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) createBackendConnectionCredential(w http.ResponseWriter, r *http.Request, connectionID string) {
	var input struct {
		Credential string `json:"credential"`
		Revision   *int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.RotateBackendConnectionCredential(r.Context(), connectionID, input.Credential, *input.Revision, actor(r))
	if err != nil {
		s.backendConnectionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) backendConnectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "backend_connection_conflict", "The backend connection name or revision conflicts with current state.", nil)
	default:
		writeError(w, http.StatusBadRequest, "invalid_backend_connection", err.Error(), nil)
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
	Name                string    `json:"name"`
	IsDefault           *bool     `json:"is_default"`
	BugReportsEnabled   *bool     `json:"bug_reports_enabled"`
	FeedbackEnabled     *bool     `json:"feedback_enabled"`
	BackendConnectionID string    `json:"backend_connection_id"`
	RetentionDays       *int      `json:"retention_days"`
	State               *string   `json:"state"`
	IntegrationIDs      *[]string `json:"integration_ids"`
	Revision            *int64    `json:"revision"`
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
		route, err := routeInput(input, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, "", route, actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.supportRouteError(w, err)
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
	case http.MethodPut:
		var input supportRouteRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		route, err := routeInput(input, true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.reporting.SaveRoute(r.Context(), deployment.ID, routeID, route, actor(r).ID, actor(r).RequestID)
		if err != nil {
			s.supportRouteError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func routeInput(input supportRouteRequest, replacing bool) (reporting.RouteInput, error) {
	if input.IsDefault == nil || input.BugReportsEnabled == nil || input.FeedbackEnabled == nil || input.RetentionDays == nil || input.State == nil || input.IntegrationIDs == nil {
		return reporting.RouteInput{}, errors.New("is_default, bug_reports_enabled, feedback_enabled, retention_days, state, and integration_ids are required")
	}
	if replacing && input.Revision == nil {
		return reporting.RouteInput{}, errors.New("revision is required when replacing a support route")
	}
	if !replacing && input.Revision != nil {
		return reporting.RouteInput{}, errors.New("revision is not allowed when creating a support route")
	}
	revision := int64(0)
	if input.Revision != nil {
		revision = *input.Revision
	}
	return reporting.RouteInput{Name: input.Name, IsDefault: *input.IsDefault, BugReportsEnabled: *input.BugReportsEnabled, FeedbackEnabled: *input.FeedbackEnabled, BackendConnectionID: input.BackendConnectionID, RetentionDays: *input.RetentionDays, State: *input.State, IntegrationIDs: *input.IntegrationIDs, Revision: revision}, nil
}

func (s *Server) supportRouteError(w http.ResponseWriter, err error) {
	if errors.Is(err, reporting.ErrInvalidReport) {
		writeError(w, http.StatusBadRequest, "invalid_support_route", err.Error(), nil)
		return
	}
	s.storeError(w, err)
}
