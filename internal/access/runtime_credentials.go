package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (r *Runtime) IssueCredential(ctx context.Context, deploymentID, connectionID string, request CredentialRequest, principal Principal) (CredentialResult, error) {
	connection, definition, cfg, err := r.connectionAndDefinition(ctx, deploymentID, connectionID, request.IntegrationID, principal)
	if err != nil {
		return CredentialResult{}, err
	}
	create, ok := cfg.Operations["credentials.create"]
	if !ok {
		return CredentialResult{}, ErrUnsupported
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.EnvironmentID == "" || request.EnvironmentID != strings.TrimSpace(request.EnvironmentID) || len(request.EnvironmentID) > 200 || len(request.IdempotencyKey) < 16 || len(request.Scopes) > 20 || !validTTL(request.TTLSeconds, cfg.MaxTTLSeconds) || (connection.EnvironmentID != "" && connection.EnvironmentID != request.EnvironmentID) {
		return CredentialResult{}, ErrInvalidRequest
	}
	if definition.CredentialScope == "connection" && request.AccessInstanceID != "" {
		return CredentialResult{}, ErrInvalidRequest
	}
	if definition.CredentialScope == "instance" {
		if request.AccessInstanceID == "" {
			return CredentialResult{}, ErrInvalidRequest
		}
		instance, err := r.store.AccessInstance(ctx, deploymentID, request.AccessInstanceID)
		if err != nil || instance.AccessConnectionID != connectionID || !contains(instance.IntegrationIDs, request.IntegrationID) || !ownsInstance(instance, principal) {
			return CredentialResult{}, ErrDenied
		}
	}
	var rotatedFrom model.AccessCredential
	if request.RotatedFromCredentialID != "" {
		rotatedFrom, err = r.store.AccessCredential(ctx, deploymentID, request.RotatedFromCredentialID)
		if err != nil || rotatedFrom.SubjectID != principal.Subject || rotatedFrom.State != "active" || rotatedFrom.AccessConnectionID != connectionID || rotatedFrom.AccessInstanceID != request.AccessInstanceID || rotatedFrom.EnvironmentID != request.EnvironmentID {
			return CredentialResult{}, ErrDenied
		}
	}
	existingValues, err := r.store.AccessCredentials(ctx, deploymentID, connectionID, request.AccessInstanceID)
	if err != nil {
		return CredentialResult{}, err
	}
	for _, existing := range existingValues {
		if existing.IdempotencyKey == request.IdempotencyKey {
			if existing.RotatedFromID != request.RotatedFromCredentialID {
				return CredentialResult{}, ErrDenied
			}
			return CredentialResult{Credential: existing, Existing: true}, nil
		}
	}
	if err := r.authorize(ctx, connection, definition, cfg, principal, "credentials.create", map[string]any{"integration_id": request.IntegrationID, "environment_id": request.EnvironmentID, "access_instance_id": request.AccessInstanceID, "scopes": request.Scopes, "ttl_seconds": request.TTLSeconds}); err != nil {
		return CredentialResult{}, err
	}
	var response struct {
		CredentialID       string          `json:"credential_id"`
		Credential         string          `json:"credential"`
		CredentialMaterial json.RawMessage `json:"credential_material"`
		ExpiresAt          *time.Time      `json:"expires_at"`
	}
	providerRequest := map[string]any{"deployment_id": deploymentID, "integration_id": request.IntegrationID, "environment_id": request.EnvironmentID, "access_instance_id": request.AccessInstanceID, "subject": principal.Subject, "scopes": request.Scopes, "idempotency_key": request.IdempotencyKey, "ttl_seconds": request.TTLSeconds}
	if request.RotatedFromCredentialID != "" && create.AcceptsRotatedFromCredentialID {
		providerRequest["rotated_from_credential_id"] = rotatedFrom.ExternalID
	}
	err = r.call(ctx, connection, definition, create, providerRequest, &response)
	if err != nil {
		return CredentialResult{}, err
	}
	if len(response.CredentialMaterial) == 0 && response.Credential != "" {
		response.CredentialMaterial, _ = json.Marshal(response.Credential)
	}
	if response.CredentialID == "" || (cfg.CredentialStorageMode != "reference" && len(response.CredentialMaterial) == 0) {
		return CredentialResult{}, ErrInvalidRequest
	}
	if rotatedFrom.ID != "" && response.CredentialID == rotatedFrom.ExternalID {
		return CredentialResult{}, ErrInvalidRequest
	}
	if response.ExpiresAt != nil && (!response.ExpiresAt.After(r.now()) || (request.TTLSeconds > 0 && cfg.MaxTTLSeconds > 0 && response.ExpiresAt.After(r.now().Add(time.Duration(cfg.MaxTTLSeconds)*time.Second)))) {
		return CredentialResult{}, ErrInvalidRequest
	}
	fingerprint := sha256.Sum256(response.CredentialMaterial)
	secretID := ""
	if cfg.CredentialStorageMode == "managed" {
		if r.vault == nil {
			return CredentialResult{}, errors.New("managed credential encryption is unavailable")
		}
		secretID = randomUUID()
		encrypted, err := r.vault.Encrypt(response.CredentialMaterial, connection.OrganisationID+":access-credential:"+secretID)
		if err != nil {
			return CredentialResult{}, err
		}
		if _, err := r.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: connection.OrganisationID, Name: "access-credential-" + secretID, Purpose: "access_credential", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return CredentialResult{}, err
		}
	}
	credential, err := r.store.CreateAccessCredential(ctx, model.AccessCredential{ID: randomUUID(), DeploymentID: deploymentID, OrganisationID: connection.OrganisationID, AccessConnectionID: connectionID, AccessInstanceID: request.AccessInstanceID, EnvironmentID: request.EnvironmentID, SubjectID: principal.Subject, ExternalID: response.CredentialID, IdempotencyKey: request.IdempotencyKey, Scopes: request.Scopes, SecretFingerprint: hex.EncodeToString(fingerprint[:]), StorageMode: cfg.CredentialStorageMode, EncryptedSecretID: secretID, State: "active", ExpiresAt: response.ExpiresAt, RotatedFromID: request.RotatedFromCredentialID})
	if err != nil {
		return CredentialResult{}, err
	}
	action := "access_credential.created"
	if request.RotatedFromCredentialID != "" {
		action = "access_credential.rotated"
	}
	if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: connection.OrganisationID, ProductID: deploymentID, ActorID: principal.Subject, Action: action, TargetType: "access_credential", TargetID: credential.ID, Current: map[string]any{"connection_id": connectionID, "access_instance_id": request.AccessInstanceID, "integration_id": request.IntegrationID, "rotated_from_id": request.RotatedFromCredentialID, "prior_retained_active": request.RotatedFromCredentialID != "", "scopes": credential.Scopes, "storage_mode": credential.StorageMode, "expires_at": credential.ExpiresAt}, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
		return CredentialResult{}, err
	}
	return CredentialResult{Credential: credential, CredentialMaterial: response.CredentialMaterial}, nil
}

func (r *Runtime) RevokeCredential(ctx context.Context, deploymentID, credentialID string, principal Principal) (model.AccessCredential, error) {
	credential, err := r.store.AccessCredential(ctx, deploymentID, credentialID)
	if err != nil || credential.SubjectID != principal.Subject {
		return model.AccessCredential{}, ErrDenied
	}
	connection, err := r.store.AccessConnection(ctx, deploymentID, credential.AccessConnectionID)
	if err != nil {
		return model.AccessCredential{}, err
	}
	definition, err := r.store.AccessDefinition(ctx, deploymentID, connection.AccessDefinitionID)
	if err != nil {
		return model.AccessCredential{}, err
	}
	cfg, err := parseDefinition(definition)
	if err != nil {
		return model.AccessCredential{}, err
	}
	revoke, ok := cfg.Operations["credentials.revoke"]
	if !ok {
		return model.AccessCredential{}, ErrUnsupported
	}
	if err := r.authorize(ctx, connection, definition, cfg, principal, "credentials.revoke", map[string]any{"credential_id": credential.ExternalID, "access_instance_id": credential.AccessInstanceID}); err != nil {
		return model.AccessCredential{}, err
	}
	revoke.Path = strings.ReplaceAll(revoke.Path, "{credential_id}", url.PathEscape(credential.ExternalID))
	revoke.Path = strings.ReplaceAll(revoke.Path, "{instance_id}", url.PathEscape(credential.AccessInstanceID))
	if strings.Contains(revoke.Path, "{") || !validOperationPath(revoke.Path) {
		return model.AccessCredential{}, ErrInvalidRequest
	}
	if err := r.call(ctx, connection, definition, revoke, map[string]any{"deployment_id": deploymentID, "subject": principal.Subject}, nil); err != nil {
		return model.AccessCredential{}, err
	}
	updated, err := r.store.RevokeAccessCredential(ctx, deploymentID, credentialID, r.now())
	if err == nil {
		if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: connection.OrganisationID, ProductID: deploymentID, ActorID: principal.Subject, Action: "access_credential.revoked", TargetType: "access_credential", TargetID: credentialID, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
			return model.AccessCredential{}, err
		}
	}
	return updated, err
}

// RevokeCredentialBound is the API-scoped variant used by generated admin
// tools. The caller cannot redirect a credential ID to another published API
// or management connection.
func (r *Runtime) RevokeCredentialBound(ctx context.Context, deploymentID, connectionID, integrationID, credentialID string, principal Principal) (model.AccessCredential, error) {
	credential, err := r.store.AccessCredential(ctx, deploymentID, credentialID)
	if err != nil || credential.AccessConnectionID != connectionID || credential.SubjectID != principal.Subject {
		return model.AccessCredential{}, ErrDenied
	}
	if _, _, _, err := r.connectionAndDefinition(ctx, deploymentID, connectionID, integrationID, principal); err != nil {
		return model.AccessCredential{}, err
	}
	if credential.AccessInstanceID != "" {
		instance, lookupErr := r.store.AccessInstance(ctx, deploymentID, credential.AccessInstanceID)
		if lookupErr != nil || instance.AccessConnectionID != connectionID || !contains(instance.IntegrationIDs, integrationID) || !ownsInstance(instance, principal) {
			return model.AccessCredential{}, ErrDenied
		}
	}
	return r.RevokeCredential(ctx, deploymentID, credentialID, principal)
}
