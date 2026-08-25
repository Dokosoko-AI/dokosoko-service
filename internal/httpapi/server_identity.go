package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type identityProviderResponse struct {
	ID                  string                 `json:"id,omitempty"`
	OrganisationID      string                 `json:"organisation_id"`
	DeploymentID        string                 `json:"deployment_id"`
	Provider            string                 `json:"provider"`
	Configured          bool                   `json:"configured"`
	CredentialPresent   bool                   `json:"credential_present"`
	CallbackURL         string                 `json:"callback_url"`
	AccessEvaluationURL string                 `json:"access_evaluation_url"`
	Issuer              string                 `json:"issuer"`
	ClientID            string                 `json:"client_id"`
	Scopes              []string               `json:"scopes"`
	Audience            string                 `json:"audience"`
	OAuthResource       string                 `json:"oauth_resource"`
	OrganisationClaim   string                 `json:"customer_account_claim"`
	InstallationClaim   string                 `json:"installation_claim"`
	DelegatedAPIOrigin  string                 `json:"authorization_api_origin"`
	State               string                 `json:"state"`
	Revision            int64                  `json:"revision"`
	LastTest            *identity.ProviderTest `json:"last_test,omitempty"`
	CreatedAt           *time.Time             `json:"created_at,omitempty"`
	UpdatedAt           *time.Time             `json:"updated_at,omitempty"`
}

func visibleIdentityTest(value identity.ProviderTest, now time.Time) identity.ProviderTest {
	value.AuthorizationURL = ""
	if value.Status == "pending" && !value.ExpiresAt.After(now) {
		value.Status = "expired"
	}
	return value
}

func (s *Server) identityProviderView(ctx context.Context, deploymentID string, value identity.ProviderConfig, configured bool) (identityProviderResponse, error) {
	if !configured {
		deployment, err := s.service.Store().Deployment(ctx)
		if err != nil {
			return identityProviderResponse{}, err
		}
		value = identity.ProviderConfig{
			OrganisationID: deployment.OrganisationID,
			DeploymentID:   deployment.ID,
			Scopes:         []string{"openid", "profile", "email"},
			State:          "disabled",
		}
	}
	response := identityProviderResponse{
		ID:                 value.ID,
		OrganisationID:     value.OrganisationID,
		DeploymentID:       value.DeploymentID,
		Provider:           "oidc",
		Configured:         configured,
		CredentialPresent:  configured && value.ClientSecretID != "",
		CallbackURL:        s.baseURL + "/oauth/callback",
		Issuer:             value.Issuer,
		ClientID:           value.ClientID,
		Scopes:             value.Scopes,
		Audience:           value.Audience,
		OAuthResource:      value.OAuthResource,
		OrganisationClaim:  value.OrganisationClaim,
		InstallationClaim:  value.InstallationClaim,
		DelegatedAPIOrigin: value.DelegatedAPIOrigin,
		State:              value.State,
		Revision:           value.Revision,
	}
	if configured {
		createdAt, updatedAt := value.CreatedAt, value.UpdatedAt
		response.CreatedAt, response.UpdatedAt = &createdAt, &updatedAt
	}
	if value.DelegatedAPIOrigin != "" {
		response.AccessEvaluationURL = strings.TrimRight(value.DelegatedAPIOrigin, "/") + "/v1/access/evaluations"
	}
	if err := s.service.Store().ExpireIdentityProviderTests(ctx, deploymentID, time.Now().UTC()); err != nil {
		return identityProviderResponse{}, err
	}
	lastTest, err := s.service.Store().LatestIdentityProviderTest(ctx, deploymentID)
	if err == nil {
		visible := visibleIdentityTest(lastTest, time.Now().UTC())
		response.LastTest = &visible
	} else if !errors.Is(err, store.ErrNotFound) {
		return identityProviderResponse{}, err
	}
	return response, nil
}

func (s *Server) writeIdentityProvider(w http.ResponseWriter, r *http.Request, value identity.ProviderConfig, configured bool) {
	response, err := s.identityProviderView(r.Context(), value.DeploymentID, value, configured)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) identityProviderTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	if s.identityBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_broker_unavailable", "Identity testing is unavailable.", nil)
		return
	}
	var input struct {
		Revision *int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	test, err := s.identityBroker.BeginProviderTest(r.Context(), deployment.ID, *input.Revision)
	if err != nil {
		s.identityLifecycleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, test)
}

func (s *Server) identityProviderTest(w http.ResponseWriter, r *http.Request, testID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	if err := s.service.Store().ExpireIdentityProviderTests(r.Context(), deployment.ID, time.Now().UTC()); err != nil {
		s.storeError(w, err)
		return
	}
	test, err := s.service.Store().IdentityProviderTest(r.Context(), deployment.ID, testID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, visibleIdentityTest(test, time.Now().UTC()))
}

func (s *Server) activateIdentityProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input struct {
		Revision *int64 `json:"revision"`
		TestID   string `json:"test_id"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || input.Revision == nil || strings.TrimSpace(input.TestID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision and test_id are required.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.ActivateIdentityProvider(r.Context(), deployment.ID, input.TestID, *input.Revision, actor(r))
	if err != nil {
		s.identityLifecycleError(w, err)
		return
	}
	s.writeIdentityProvider(w, r, value, true)
}

func (s *Server) disableIdentityProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var input struct {
		Revision *int64 `json:"revision"`
	}
	if err := decodeJSON(r.Body, &input); err != nil || input.Revision == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision is required.", nil)
		return
	}
	deployment, err := s.service.Store().Deployment(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	value, err := s.service.DisableIdentityProvider(r.Context(), deployment.ID, *input.Revision, actor(r))
	if err != nil {
		s.identityLifecycleError(w, err)
		return
	}
	s.writeIdentityProvider(w, r, value, true)
}

func (s *Server) identityLifecycleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict), errors.Is(err, identity.ErrProviderRevision):
		writeError(w, http.StatusConflict, "identity_revision_conflict", "The identity configuration changed. Reload and try again.", nil)
	case errors.Is(err, platform.ErrIdentityDraftRequired), errors.Is(err, platform.ErrIdentityTestRequired), errors.Is(err, identity.ErrProviderTest):
		writeError(w, http.StatusConflict, "identity_test_required", "Save a disabled draft and complete a fresh identity test before activation.", nil)
	case errors.Is(err, identity.ErrProviderConfiguration):
		writeError(w, http.StatusConflict, "identity_configuration_incomplete", "Review and save the complete OIDC configuration before testing or activation.", nil)
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "identity_lifecycle_failed", "The identity configuration could not be updated.", nil)
	}
}

func (s *Server) identityConfigurationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "identity_revision_conflict", "The identity configuration changed. Reload it before saving again.", nil)
	case errors.Is(err, platform.ErrIdentityCredential):
		writeError(w, http.StatusBadRequest, "identity_credential_required", "Enter the OIDC client secret when connecting for the first time or changing the issuer or client ID.", nil)
	case errors.Is(err, platform.ErrIdentityConfigInvalid), errors.Is(err, identity.ErrProviderConfiguration):
		writeError(w, http.StatusUnprocessableEntity, "invalid_identity_configuration", "Enter a complete OIDC configuration. HTTPS is required except for HTTP localhost development issuers and API origins.", nil)
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "identity_configuration_failed", "The identity configuration could not be saved.", nil)
	}
}

func (s *Server) identityDisconnectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "identity_revision_conflict", "The identity configuration changed. Reload it before disconnecting.", nil)
	case errors.Is(err, platform.ErrIdentityDisableFirst):
		writeError(w, http.StatusConflict, "identity_disable_required", "Disable the OIDC connection before disconnecting it.", nil)
	case errors.Is(err, store.ErrNotFound):
		s.storeError(w, err)
	default:
		writeError(w, http.StatusInternalServerError, "identity_disconnect_failed", "The OIDC connection could not be disconnected.", nil)
	}
}
