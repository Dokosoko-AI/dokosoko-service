package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

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

func (s *Server) accessDefinition(w http.ResponseWriter, r *http.Request, definitionID string) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input struct {
		Name                  string          `json:"name"`
		InstanceLabelSingular string          `json:"instance_label_singular"`
		InstanceLabelPlural   string          `json:"instance_label_plural"`
		APIResourceSetID      string          `json:"api_resource_set_id"`
		Operations            json.RawMessage `json:"operations"`
		Revision              *int64          `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	value, err := s.service.UpdateAccessDefinition(r.Context(), definitionID, platform.AccessDefinitionInput{Name: input.Name, InstanceLabelSingular: input.InstanceLabelSingular, InstanceLabelPlural: input.InstanceLabelPlural, APIResourceSetID: input.APIResourceSetID, Operations: input.Operations}, *input.Revision, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
			s.storeError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_access_definition", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, value)
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
