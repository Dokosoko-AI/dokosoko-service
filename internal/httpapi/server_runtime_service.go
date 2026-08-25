package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type runtimeSetupRequest struct {
	EnvironmentID           string          `json:"environment_id"`
	ConnectionName          string          `json:"connection_name"`
	ConnectionDescription   string          `json:"connection_description"`
	BaseURL                 string          `json:"base_url"`
	AuthenticationType      string          `json:"authentication_type"`
	AuthConfig              json.RawMessage `json:"auth_config"`
	ExistingCredentialSetID string          `json:"existing_credential_set_id"`
	CredentialScope         string          `json:"credential_scope"`
	CredentialName          string          `json:"credential_name"`
	EnvironmentVariable     string          `json:"environment_variable"`
	HeaderName              string          `json:"header_name"`
	Credential              string          `json:"credential"`
	CredentialExpiresAt     *time.Time      `json:"credential_expires_at"`
}

func (s *Server) integrationRuntimeSetup(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.RuntimeSetup(r.Context(), integrationID)
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		var input runtimeSetupRequest
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.ConfigureRuntimeSetup(r.Context(), integrationID, platform.RuntimeSetupInput{EnvironmentID: input.EnvironmentID, ConnectionName: input.ConnectionName, ConnectionDescription: input.ConnectionDescription, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, AuthConfig: input.AuthConfig, ExistingCredentialSetID: input.ExistingCredentialSetID, CredentialScope: input.CredentialScope, CredentialName: input.CredentialName, EnvironmentVariable: input.EnvironmentVariable, HeaderName: input.HeaderName, Credential: input.Credential, CredentialExpiresAt: input.CredentialExpiresAt}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) integrationRuntimeConnections(w http.ResponseWriter, r *http.Request, integrationID string) {
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.RuntimeSetup(r.Context(), integrationID)
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": value.Connections})
	case http.MethodPost:
		var input struct {
			Name               string          `json:"name"`
			Description        string          `json:"description"`
			EnvironmentID      string          `json:"environment_id"`
			BaseURL            string          `json:"base_url"`
			AuthenticationType string          `json:"authentication_type"`
			CredentialSetID    string          `json:"credential_set_id"`
			AuthConfig         json.RawMessage `json:"auth_config"`
			State              string          `json:"state"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.ConfigureRuntimeServiceConnection(r.Context(), integrationID, platform.RuntimeServiceConnectionInput{Name: input.Name, Description: input.Description, EnvironmentID: input.EnvironmentID, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, CredentialSetID: input.CredentialSetID, AuthConfig: input.AuthConfig, State: input.State}, actor(r))
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

func (s *Server) createIntegrationRuntimeCredentialSet(w http.ResponseWriter, r *http.Request, integrationID string) {
	var input struct {
		EnvironmentID       string     `json:"environment_id"`
		Scope               string     `json:"scope"`
		Name                string     `json:"name"`
		EnvironmentVariable string     `json:"environment_variable"`
		AuthenticationType  string     `json:"authentication_type"`
		HeaderName          string     `json:"header_name"`
		Credential          string     `json:"credential"`
		ExpiresAt           *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.CreateRuntimeCredentialSet(r.Context(), integrationID, platform.RuntimeCredentialSetInput{EnvironmentID: input.EnvironmentID, Scope: input.Scope, Name: input.Name, EnvironmentVariable: input.EnvironmentVariable, AuthenticationType: input.AuthenticationType, HeaderName: input.HeaderName, Credential: input.Credential, ExpiresAt: input.ExpiresAt}, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) runtimeCredentialSet(w http.ResponseWriter, r *http.Request, credentialSetID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.Store().RuntimeCredentialSet(r.Context(), deployment.ID, credentialSetID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) runtimeCredentialUsage(w http.ResponseWriter, r *http.Request, credentialSetID string) {
	values, err := s.service.RuntimeCredentialUsage(r.Context(), credentialSetID)
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "count": len(values)})
}

func (s *Server) checkRuntimeServiceConnection(w http.ResponseWriter, r *http.Request, connectionID string) {
	value, err := s.service.RuntimeServiceConnectionReadiness(r.Context(), connectionID)
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) rotateRuntimeCredential(w http.ResponseWriter, r *http.Request, credentialSetID string) {
	var input struct {
		Credential string     `json:"credential"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r.Body, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	value, err := s.service.RotateRuntimeCredential(r.Context(), credentialSetID, input.Credential, input.ExpiresAt, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) revokeRuntimeCredential(w http.ResponseWriter, r *http.Request, credentialSetID, versionID string) {
	value, err := s.service.RevokeRuntimeCredentialVersion(r.Context(), credentialSetID, versionID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
