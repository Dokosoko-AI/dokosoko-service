package platform

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func (s *Service) backendCredentialMatches(ctx context.Context, connection model.BackendConnection, credential string) bool {
	if credential == "" {
		return connection.CredentialSecretID == ""
	}
	if s.vault == nil || connection.CredentialSecretID == "" {
		return false
	}
	stored, err := s.store.Secret(ctx, connection.OrganisationID, connection.CredentialSecretID)
	if err != nil {
		return false
	}
	plaintext, err := s.vault.Decrypt(secretvault.Encrypted{Ciphertext: stored.Ciphertext, Nonce: stored.Nonce, KeyVersion: stored.KeyVersion, Fingerprint: stored.Fingerprint}, connection.OrganisationID+":backend_connection:"+connection.CredentialSecretID)
	return err == nil && subtle.ConstantTimeCompare(plaintext, []byte(credential)) == 1
}

func (s *Service) backendCredentialResource(ctx context.Context, connection model.BackendConnection) (model.BackendConnectionCredential, error) {
	secret, err := s.store.Secret(ctx, connection.OrganisationID, connection.CredentialSecretID)
	if err != nil {
		return model.BackendConnectionCredential{}, err
	}
	return model.BackendConnectionCredential{ID: secret.ID, BackendConnectionID: connection.ID, Fingerprint: connection.CredentialFingerprint, ConnectionRevision: connection.Revision, CreatedAt: secret.CreatedAt}, nil
}

type BackendConnectionInput struct {
	Name               string
	BaseURL            string
	AuthenticationType string
	Credential         string
	State              string
	Revision           int64
}

func normalizeBackendConnectionInput(input BackendConnectionInput) (BackendConnectionInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.AuthenticationType = strings.ToLower(strings.TrimSpace(input.AuthenticationType))
	input.Credential = strings.TrimSpace(input.Credential)
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	if input.Name == "" || len(input.Name) > 120 {
		return input, errors.New("backend connection name is required and must not exceed 120 characters")
	}
	if !validHTTPSBaseOrigin(input.BaseURL) {
		return input, errors.New("backend connection base URL must be a credential-free HTTPS origin on the default port")
	}
	if input.AuthenticationType == "" {
		input.AuthenticationType = "bearer"
	}
	if input.AuthenticationType != "bearer" {
		return input, errors.New("backend connection authentication type must be bearer")
	}
	if input.State == "" {
		input.State = "disabled"
	}
	if input.State != "active" && input.State != "disabled" {
		return input, errors.New("backend connection state must be active or disabled")
	}
	return input, nil
}

func (s *Service) storeBackendCredential(ctx context.Context, organisationID, connectionID, credential string) (model.Secret, error) {
	if s.vault == nil {
		return model.Secret{}, errors.New("backend credential encryption is not configured")
	}
	secretID, err := randomUUID()
	if err != nil {
		return model.Secret{}, err
	}
	encrypted, err := s.vault.Encrypt([]byte(credential), organisationID+":backend_connection:"+secretID)
	if err != nil {
		return model.Secret{}, err
	}
	secret, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: organisationID, Name: "backend-connection-" + connectionID + "-" + secretID, Purpose: "backend_connection_bearer", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	if err != nil {
		return model.Secret{}, err
	}
	return secret, nil
}

func (s *Service) CreateBackendConnection(ctx context.Context, input BackendConnectionInput, actor Actor) (model.BackendConnection, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.BackendConnection{}, err
	}
	input, err = normalizeBackendConnectionInput(input)
	if err != nil {
		return model.BackendConnection{}, err
	}
	if input.State == "active" && input.Credential == "" {
		return model.BackendConnection{}, errors.New("an active backend connection requires a credential")
	}
	existing, err := s.store.BackendConnections(ctx, deployment.ID)
	if err != nil {
		return model.BackendConnection{}, err
	}
	for _, current := range existing {
		if current.Name != input.Name {
			continue
		}
		if current.BaseURL == input.BaseURL && current.AuthenticationType == input.AuthenticationType && current.State == input.State && s.backendCredentialMatches(ctx, current, input.Credential) {
			return current, nil
		}
		return model.BackendConnection{}, store.ErrConflict
	}
	id, err := randomUUID()
	if err != nil {
		return model.BackendConnection{}, err
	}
	value := model.BackendConnection{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, Name: input.Name, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, State: input.State}
	if input.Credential != "" {
		secret, credentialErr := s.storeBackendCredential(ctx, deployment.OrganisationID, id, input.Credential)
		err = credentialErr
		if err != nil {
			return model.BackendConnection{}, err
		}
		value.CredentialSecretID, value.CredentialFingerprint = secret.ID, secret.Fingerprint
	}
	created, err := s.store.CreateBackendConnection(ctx, value)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			latest, listErr := s.store.BackendConnections(ctx, deployment.ID)
			if listErr == nil {
				for _, current := range latest {
					if current.Name == input.Name && current.BaseURL == input.BaseURL && current.AuthenticationType == input.AuthenticationType && current.State == input.State && s.backendCredentialMatches(ctx, current, input.Credential) {
						return current, nil
					}
				}
			}
		}
		return model.BackendConnection{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: created.OrganisationID, ProductID: created.DeploymentID, ActorID: actor.ID, Action: "backend_connection.created", TargetType: "backend_connection", TargetID: created.ID, Current: map[string]any{"name": created.Name, "base_url": created.BaseURL, "authentication_type": created.AuthenticationType, "state": created.State, "credential_configured": created.CredentialFingerprint != ""}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return created, nil
}

func (s *Service) UpdateBackendConnection(ctx context.Context, id string, input BackendConnectionInput, actor Actor) (model.BackendConnection, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.BackendConnection{}, err
	}
	if input.Credential != "" {
		return model.BackendConnection{}, errors.New("rotate credentials through the backend connection credentials resource")
	}
	input, err = normalizeBackendConnectionInput(input)
	if err != nil {
		return model.BackendConnection{}, err
	}
	current, err := s.store.BackendConnection(ctx, deployment.ID, id)
	if err != nil {
		return model.BackendConnection{}, err
	}
	if input.State == "active" && current.CredentialSecretID == "" {
		return model.BackendConnection{}, errors.New("an active backend connection requires a credential")
	}
	if current.Revision != input.Revision {
		if current.Name == input.Name && current.BaseURL == input.BaseURL && current.AuthenticationType == input.AuthenticationType && current.State == input.State {
			return current, nil
		}
		return model.BackendConnection{}, store.ErrConflict
	}
	current.Name, current.BaseURL, current.AuthenticationType, current.State = input.Name, input.BaseURL, input.AuthenticationType, input.State
	updated, err := s.store.UpdateBackendConnection(ctx, current, input.Revision)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			latest, latestErr := s.store.BackendConnection(ctx, deployment.ID, id)
			if latestErr == nil && latest.Name == input.Name && latest.BaseURL == input.BaseURL && latest.AuthenticationType == input.AuthenticationType && latest.State == input.State {
				return latest, nil
			}
		}
		return model.BackendConnection{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "backend_connection.updated", TargetType: "backend_connection", TargetID: updated.ID, Current: map[string]any{"name": updated.Name, "base_url": updated.BaseURL, "authentication_type": updated.AuthenticationType, "state": updated.State}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

func (s *Service) RotateBackendConnectionCredential(ctx context.Context, id, credential string, revision int64, actor Actor) (model.BackendConnectionCredential, error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return model.BackendConnectionCredential{}, errors.New("credential is required")
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.BackendConnectionCredential{}, err
	}
	current, err := s.store.BackendConnection(ctx, deployment.ID, id)
	if err != nil {
		return model.BackendConnectionCredential{}, err
	}
	if current.Revision != revision && s.backendCredentialMatches(ctx, current, credential) {
		return s.backendCredentialResource(ctx, current)
	}
	if current.Revision != revision {
		return model.BackendConnectionCredential{}, store.ErrConflict
	}
	secret, err := s.storeBackendCredential(ctx, current.OrganisationID, current.ID, credential)
	if err != nil {
		return model.BackendConnectionCredential{}, err
	}
	current.CredentialSecretID, current.CredentialFingerprint = secret.ID, secret.Fingerprint
	updated, err := s.store.UpdateBackendConnection(ctx, current, revision)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			latest, latestErr := s.store.BackendConnection(ctx, deployment.ID, id)
			if latestErr == nil && s.backendCredentialMatches(ctx, latest, credential) {
				return s.backendCredentialResource(ctx, latest)
			}
		}
		return model.BackendConnectionCredential{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: updated.OrganisationID, ProductID: updated.DeploymentID, ActorID: actor.ID, Action: "backend_connection.credential.created", TargetType: "backend_connection", TargetID: updated.ID, Current: map[string]any{"credential_fingerprint": updated.CredentialFingerprint}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return model.BackendConnectionCredential{ID: secret.ID, BackendConnectionID: updated.ID, Fingerprint: updated.CredentialFingerprint, ConnectionRevision: updated.Revision, CreatedAt: secret.CreatedAt}, nil
}
