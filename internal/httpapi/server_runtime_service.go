package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

type runtimeSetupRequest struct {
	EnvironmentID         string                                      `json:"environment_id"`
	ConnectionName        string                                      `json:"connection_name"`
	ConnectionDescription string                                      `json:"connection_description"`
	BaseURL               string                                      `json:"base_url"`
	AuthenticationType    string                                      `json:"authentication_type"`
	AuthConfig            json.RawMessage                             `json:"auth_config"`
	AuthorizationID       string                                      `json:"authorization_id"`
	EnvironmentVariable   string                                      `json:"environment_variable"`
	HeaderName            string                                      `json:"header_name"`
	KeyManagementURL      string                                      `json:"key_management_url"`
	AccessEvaluationURL   string                                      `json:"access_evaluation_url"`
	UsageURL              string                                      `json:"usage_url"`
	Credential            string                                      `json:"credential"`
	AdditionalHeaders     *[]platform.RuntimeAuthorizationHeaderInput `json:"additional_headers"`
	CredentialExpiresAt   *time.Time                                  `json:"credential_expires_at"`
}

func (s *Server) integrationAuthorization(w http.ResponseWriter, r *http.Request, integrationID string) {
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
		value, err := s.service.ConfigureRuntimeSetup(r.Context(), integrationID, platform.RuntimeSetupInput{EnvironmentID: input.EnvironmentID, ConnectionName: input.ConnectionName, ConnectionDescription: input.ConnectionDescription, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, AuthConfig: input.AuthConfig, AuthorizationID: input.AuthorizationID, EnvironmentVariable: input.EnvironmentVariable, HeaderName: input.HeaderName, KeyManagementURL: input.KeyManagementURL, AccessEvaluationURL: input.AccessEvaluationURL, UsageURL: input.UsageURL, Credential: input.Credential, AdditionalHeaders: input.AdditionalHeaders, CredentialExpiresAt: input.CredentialExpiresAt}, actor(r))
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

func (s *Server) authorizations(w http.ResponseWriter, r *http.Request) {
	values, err := s.service.RuntimeAuthorizationProfiles(r.Context())
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) authorization(w http.ResponseWriter, r *http.Request, authorizationID string) {
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.service.Store().RuntimeCredentialSet(r.Context(), deployment.ID, authorizationID)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPatch:
		var input struct {
			EnvironmentVariable string                                      `json:"environment_variable"`
			HeaderName          string                                      `json:"header_name"`
			AuthConfig          json.RawMessage                             `json:"auth_config"`
			KeyManagementURL    string                                      `json:"key_management_url"`
			AccessEvaluationURL string                                      `json:"access_evaluation_url"`
			UsageURL            string                                      `json:"usage_url"`
			Credential          string                                      `json:"credential"`
			AdditionalHeaders   *[]platform.RuntimeAuthorizationHeaderInput `json:"additional_headers"`
			State               string                                      `json:"state"`
			Revision            int64                                       `json:"revision"`
		}
		if err := decodeJSON(r.Body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		value, err := s.service.UpdateRuntimeAuthorization(r.Context(), authorizationID, platform.RuntimeAuthorizationUpdateInput{EnvironmentVariable: input.EnvironmentVariable, HeaderName: input.HeaderName, AuthConfig: input.AuthConfig, KeyManagementURL: input.KeyManagementURL, AccessEvaluationURL: input.AccessEvaluationURL, UsageURL: input.UsageURL, Credential: input.Credential, AdditionalHeaders: input.AdditionalHeaders, State: input.State, Revision: input.Revision}, actor(r))
		if err != nil {
			s.creationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) authorizationUsage(w http.ResponseWriter, r *http.Request, credentialSetID string) {
	values, err := s.service.RuntimeCredentialUsage(r.Context(), credentialSetID)
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values, "count": len(values)})
}

func (s *Server) rotateAuthorizationCredential(w http.ResponseWriter, r *http.Request, credentialSetID string) {
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

func (s *Server) revokeAuthorizationCredential(w http.ResponseWriter, r *http.Request, credentialSetID, versionID string) {
	value, err := s.service.RevokeRuntimeCredentialVersion(r.Context(), credentialSetID, versionID, actor(r))
	if err != nil {
		s.creationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
