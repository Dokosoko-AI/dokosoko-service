package platform

import (
	"context"
	"errors"
	"fmt"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"net/url"
	"strings"
	"time"
)

type IdentityInput struct {
	DeploymentID       string
	Issuer             string
	ClientID           string
	ClientSecret       string
	Scopes             []string
	Audience           string
	OAuthResource      string
	OrganisationClaim  string
	InstallationClaim  string
	DelegatedAPIOrigin string
	Revision           int64
}

func validHTTPSOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && parsed.RawQuery == ""
}

func validHTTPSBaseOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validHTTPSURI(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func validOIDCIssuer(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validOAuthResourceIdentifier(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && raw != "" && !strings.ContainsAny(raw, " \t\r\n") && parsed.IsAbs() && parsed.Scheme != "" && parsed.User == nil && parsed.Fragment == ""
}

func validOAuthRedirect(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && identity.IsLocalDevelopmentHostname(parsed.Hostname()))
}

func validToolEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	local := err == nil && identity.IsLocalDevelopmentHostname(parsed.Hostname())
	return err == nil && (parsed.Scheme == "https" && !local && parsed.Port() == "" || parsed.Scheme == "http" && local) && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

// Legacy ConfigureIdentity removed trailing slashes even when they were part
// of the provider's exact issuer. Treat adding that slash back as a
// normalization of the same credential boundary during upgrade; the saved
// value itself remains exact.
func equivalentOIDCIssuer(left, right string) bool {
	if left == right {
		return true
	}
	leftURL, leftErr := url.Parse(left)
	rightURL, rightErr := url.Parse(right)
	if leftErr != nil || rightErr != nil || leftURL.User != nil || rightURL.User != nil || leftURL.RawQuery != "" || rightURL.RawQuery != "" || leftURL.Fragment != "" || rightURL.Fragment != "" {
		return false
	}
	leftRoot := leftURL.EscapedPath() == "" || leftURL.EscapedPath() == "/"
	rightRoot := rightURL.EscapedPath() == "" || rightURL.EscapedPath() == "/"
	return leftRoot && rightRoot && strings.EqualFold(leftURL.Scheme, rightURL.Scheme) && strings.EqualFold(leftURL.Host, rightURL.Host)
}

func (s *Service) ConfigureIdentity(ctx context.Context, input IdentityInput, actor Actor) (identity.ProviderConfig, error) {
	input.DeploymentID = strings.TrimSpace(input.DeploymentID)
	input.Issuer, input.ClientID, input.ClientSecret = strings.TrimSpace(input.Issuer), strings.TrimSpace(input.ClientID), strings.TrimSpace(input.ClientSecret)
	input.Audience, input.OAuthResource = strings.TrimSpace(input.Audience), strings.TrimSpace(input.OAuthResource)
	input.OrganisationClaim, input.InstallationClaim = strings.TrimSpace(input.OrganisationClaim), strings.TrimSpace(input.InstallationClaim)
	input.DelegatedAPIOrigin = strings.TrimRight(strings.TrimSpace(input.DelegatedAPIOrigin), "/")
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if input.DeploymentID != deployment.ID || input.ClientID == "" || !validOIDCIssuer(input.Issuer) {
		return identity.ProviderConfig{}, fmt.Errorf("%w: deployment, exact OIDC issuer (HTTPS, or HTTP for localhost development), and client ID are required", ErrIdentityConfigInvalid)
	}
	if !validHTTPSBaseOrigin(input.DelegatedAPIOrigin) || (input.OAuthResource != "" && !validOAuthResourceIdentifier(input.OAuthResource)) {
		return identity.ProviderConfig{}, fmt.Errorf("%w: authorization API origin must be a credential-free HTTPS origin (or HTTP localhost), and OAuth resource must be an absolute URI without a fragment", ErrIdentityConfigInvalid)
	}
	if input.OrganisationClaim == "" {
		return identity.ProviderConfig{}, fmt.Errorf("%w: customer account claim is required", ErrIdentityConfigInvalid)
	}
	if len(input.Scopes) == 0 {
		input.Scopes = []string{"openid", "profile", "email"}
	}
	seenScopes := map[string]bool{}
	scopes := make([]string, 0, len(input.Scopes)+1)
	for _, scope := range input.Scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" && !seenScopes[scope] {
			seenScopes[scope] = true
			scopes = append(scopes, scope)
		}
	}
	if !seenScopes["openid"] {
		scopes = append([]string{"openid"}, scopes...)
	}
	current, currentErr := s.store.IdentityProvider(ctx, input.DeploymentID)
	if currentErr != nil && !errors.Is(currentErr, store.ErrNotFound) {
		return identity.ProviderConfig{}, currentErr
	}
	config := identity.ProviderConfig{OrganisationID: deployment.OrganisationID, DeploymentID: input.DeploymentID, Issuer: input.Issuer, ClientID: input.ClientID, Scopes: scopes, Audience: input.Audience, OAuthResource: input.OAuthResource, OrganisationClaim: input.OrganisationClaim, InstallationClaim: input.InstallationClaim, DelegatedAPIOrigin: input.DelegatedAPIOrigin, State: "disabled", Revision: input.Revision}
	if currentErr == nil {
		if input.Revision != current.Revision {
			return identity.ProviderConfig{}, store.ErrConflict
		}
		config.ID, config.ClientSecretID = current.ID, current.ClientSecretID
		if (!equivalentOIDCIssuer(current.Issuer, config.Issuer) || current.ClientID != config.ClientID) && input.ClientSecret == "" {
			return identity.ProviderConfig{}, fmt.Errorf("%w: changing the OIDC issuer or client ID requires a new client secret", ErrIdentityCredential)
		}
	} else {
		if input.Revision != 0 {
			return identity.ProviderConfig{}, store.ErrConflict
		}
		config.ID, err = randomUUID()
		if err != nil {
			return identity.ProviderConfig{}, err
		}
	}
	if config.ClientSecretID == "" && input.ClientSecret == "" {
		return identity.ProviderConfig{}, fmt.Errorf("%w for initial configuration", ErrIdentityCredential)
	}
	readiness := config
	if input.ClientSecret != "" {
		readiness.ClientSecretID = "pending-encrypted-client-credential"
	}
	if err := identity.ValidateProviderConfig(readiness); err != nil {
		return identity.ProviderConfig{}, fmt.Errorf("%w: %v", ErrIdentityConfigInvalid, err)
	}
	if input.ClientSecret != "" {
		if s.vault == nil {
			return identity.ProviderConfig{}, errors.New("identity credential encryption is not configured")
		}
		secretID, err := randomUUID()
		if err != nil {
			return identity.ProviderConfig{}, err
		}
		encrypted, err := s.vault.Encrypt([]byte(input.ClientSecret), deployment.OrganisationID+":idp:"+secretID)
		if err != nil {
			return identity.ProviderConfig{}, err
		}
		if _, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: deployment.OrganisationID, Name: "identity-provider-oidc-" + input.DeploymentID + "-" + secretID, Purpose: "identity_provider_oidc_client", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return identity.ProviderConfig{}, err
		}
		config.ClientSecretID = secretID
	}
	newSecretID := ""
	if input.ClientSecret != "" {
		newSecretID = config.ClientSecretID
	}
	updated, err := s.store.SaveIdentityProvider(ctx, config)
	if err != nil {
		return identity.ProviderConfig{}, s.cleanupFailedIdentityCredential(ctx, deployment.OrganisationID, newSecretID, err)
	}
	s.deleteStaleIdentityOAuthArtifacts(ctx, updated.DeploymentID)
	if currentErr == nil && newSecretID != "" && current.ClientSecretID != "" && current.ClientSecretID != newSecretID {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		cleanupErr := s.store.DeleteSecret(cleanupCtx, deployment.OrganisationID, current.ClientSecretID)
		cancel()
		if cleanupErr != nil {
			if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: input.DeploymentID, ActorID: actor.ID, Action: "identity_provider.credential.cleanup_failed", TargetType: "identity_provider", TargetID: updated.ID, Current: map[string]any{"retired_secret_id": current.ClientSecretID}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
				return identity.ProviderConfig{}, err
			}
		}
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: input.DeploymentID, ActorID: actor.ID, Action: "identity_provider.draft.saved", TargetType: "identity_provider", TargetID: updated.ID, Current: map[string]any{"provider": "oidc", "issuer": updated.Issuer, "client_id": updated.ClientID, "scopes": updated.Scopes, "audience": updated.Audience, "oauth_resource": updated.OAuthResource, "customer_account_claim": updated.OrganisationClaim, "installation_claim": updated.InstallationClaim, "authorization_api_origin": updated.DelegatedAPIOrigin, "state": updated.State, "revision": updated.Revision, "credential_rotated": input.ClientSecret != ""}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return identity.ProviderConfig{}, err
	}
	return updated, nil
}

func (s *Service) cleanupFailedIdentityCredential(ctx context.Context, organisationID, credentialID string, operationErr error) error {
	if credentialID == "" {
		return operationErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.store.DeleteSecret(cleanupCtx, organisationID, credentialID); err != nil {
		return errors.Join(operationErr, fmt.Errorf("stored identity credential cleanup failed: %w", err))
	}
	return operationErr
}

func (s *Service) deleteStaleIdentityOAuthArtifacts(ctx context.Context, deploymentID string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, _ = s.store.DeleteStaleOAuthArtifacts(cleanupCtx, deploymentID, s.now(), identityOAuthCleanupBatch)
}

func (s *Service) ActivateIdentityProvider(ctx context.Context, deploymentID, testID string, revision int64, actor Actor) (identity.ProviderConfig, error) {
	current, err := s.store.IdentityProvider(ctx, deploymentID)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if current.Revision != revision {
		return identity.ProviderConfig{}, store.ErrConflict
	}
	if current.State != "disabled" {
		return identity.ProviderConfig{}, ErrIdentityDraftRequired
	}
	if err := identity.ValidateProviderConfig(current); err != nil {
		return identity.ProviderConfig{}, err
	}
	test, err := s.store.IdentityProviderTest(ctx, deploymentID, strings.TrimSpace(testID))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return identity.ProviderConfig{}, ErrIdentityTestRequired
		}
		return identity.ProviderConfig{}, err
	}
	if test.ConfigurationRevision != current.Revision || test.Status != "passed" || test.CompletedAt == nil || !test.ExpiresAt.After(s.now()) || test.Issuer != current.Issuer || test.Subject == "" || test.CustomerID == "" {
		return identity.ProviderConfig{}, ErrIdentityTestRequired
	}
	current.State = "active"
	updated, err := s.store.SaveIdentityProvider(ctx, current)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	s.deleteStaleIdentityOAuthArtifacts(ctx, updated.DeploymentID)
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "identity_provider.activated", TargetType: "identity_provider", TargetID: updated.ID, Current: map[string]any{"state": updated.State, "tested_configuration_revision": test.ConfigurationRevision, "test_id": test.ID}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return identity.ProviderConfig{}, err
	}
	return updated, nil
}

func (s *Service) DisableIdentityProvider(ctx context.Context, deploymentID string, revision int64, actor Actor) (identity.ProviderConfig, error) {
	current, err := s.store.IdentityProvider(ctx, deploymentID)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if current.Revision != revision {
		return identity.ProviderConfig{}, store.ErrConflict
	}
	if current.State == "disabled" {
		s.deleteStaleIdentityOAuthArtifacts(ctx, current.DeploymentID)
		return current, nil
	}
	current.State = "disabled"
	updated, err := s.store.SaveIdentityProvider(ctx, current)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	s.deleteStaleIdentityOAuthArtifacts(ctx, updated.DeploymentID)
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "identity_provider.disabled", TargetType: "identity_provider", TargetID: updated.ID, Prior: map[string]any{"state": "active"}, Current: map[string]any{"state": updated.State}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return identity.ProviderConfig{}, err
	}
	return updated, nil
}

func (s *Service) DisconnectIdentityProvider(ctx context.Context, deploymentID string, revision int64, actor Actor) (identity.ProviderConfig, error) {
	current, err := s.store.IdentityProvider(ctx, deploymentID)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if current.Revision != revision {
		return identity.ProviderConfig{}, store.ErrConflict
	}
	if current.State != "disabled" {
		return identity.ProviderConfig{}, ErrIdentityDisableFirst
	}
	deleted, err := s.store.DeleteIdentityProvider(ctx, deploymentID, revision)
	if err != nil {
		return identity.ProviderConfig{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deleted.OrganisationID, ProductID: deleted.DeploymentID, ActorID: actor.ID, Action: "identity_provider.disconnected", TargetType: "identity_provider", TargetID: deleted.ID, Prior: map[string]any{"provider": "oidc", "issuer": deleted.Issuer, "client_id": deleted.ClientID, "audience": deleted.Audience, "oauth_resource": deleted.OAuthResource, "customer_account_claim": deleted.OrganisationClaim, "installation_claim": deleted.InstallationClaim, "authorization_api_origin": deleted.DelegatedAPIOrigin, "state": deleted.State, "revision": deleted.Revision}, Current: map[string]any{"configured": false}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return identity.ProviderConfig{}, err
	}
	return deleted, nil
}

func (s *Service) UpdateCustomerAccountState(ctx context.Context, productID, accountID, state string, revision int64, actor Actor) (identity.CustomerAccount, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "active" && state != "suspended" {
		return identity.CustomerAccount{}, errors.New("customer account state must be active or suspended")
	}
	current, err := s.store.CustomerAccount(ctx, productID, accountID)
	if err != nil {
		return identity.CustomerAccount{}, err
	}
	if current.State == state {
		if current.Revision != revision {
			return identity.CustomerAccount{}, store.ErrConflict
		}
		return current, nil
	}
	prior := current.State
	current.State = state
	updated, err := s.store.UpdateCustomerAccount(ctx, current, revision)
	if err != nil {
		return identity.CustomerAccount{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "customer_account.state.changed", TargetType: "customer_account", TargetID: updated.ID, Prior: map[string]any{"state": prior}, Current: map[string]any{"state": state}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return identity.CustomerAccount{}, err
	}
	return updated, nil
}
