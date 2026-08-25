package access

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (r *Runtime) connectionAndDefinition(ctx context.Context, deploymentID, connectionID, integrationID string, principal Principal) (model.AccessConnection, model.AccessDefinition, definitionConfig, error) {
	connection, err := r.store.AccessConnection(ctx, deploymentID, connectionID)
	if err != nil || connection.State != "active" || !contains(connection.IntegrationIDs, integrationID) {
		return model.AccessConnection{}, model.AccessDefinition{}, definitionConfig{}, ErrDenied
	}
	definition, err := r.store.AccessDefinition(ctx, deploymentID, connection.AccessDefinitionID)
	if err != nil || definition.State != "active" {
		return model.AccessConnection{}, model.AccessDefinition{}, definitionConfig{}, ErrDenied
	}
	cfg, err := parseDefinition(definition)
	if err != nil || !allowed(cfg, principal) {
		return model.AccessConnection{}, model.AccessDefinition{}, definitionConfig{}, ErrDenied
	}
	return connection, definition, cfg, nil
}

func (r *Runtime) CreateInstance(ctx context.Context, deploymentID, connectionID string, request InstanceRequest, principal Principal) (model.AccessInstance, error) {
	connection, definition, cfg, err := r.connectionAndDefinition(ctx, deploymentID, connectionID, request.IntegrationID, principal)
	if err != nil {
		return model.AccessInstance{}, err
	}
	create, ok := cfg.Operations["instances.create"]
	if definition.InstanceCardinality != "many" || !ok {
		return model.AccessInstance{}, ErrUnsupported
	}
	request.DisplayName, request.IdempotencyKey = strings.TrimSpace(request.DisplayName), strings.TrimSpace(request.IdempotencyKey)
	if request.EnvironmentID == "" || request.DisplayName == "" || len(request.DisplayName) > 160 || len(request.IdempotencyKey) < 16 || !validTTL(request.TTLSeconds, cfg.MaxTTLSeconds) || (connection.EnvironmentID != "" && connection.EnvironmentID != request.EnvironmentID) {
		return model.AccessInstance{}, ErrInvalidRequest
	}
	instances, err := r.store.AccessInstances(ctx, deploymentID, connectionID)
	if err != nil {
		return model.AccessInstance{}, err
	}
	for _, existing := range instances {
		if existing.IdempotencyKey == request.IdempotencyKey {
			return existing, nil
		}
	}
	if err := r.authorize(ctx, connection, definition, cfg, principal, "instances.create", map[string]any{"integration_id": request.IntegrationID, "environment_id": request.EnvironmentID, "ttl_seconds": request.TTLSeconds}); err != nil {
		return model.AccessInstance{}, err
	}
	ownerType, ownerID := "user", principal.Subject
	if principal.InstallationID != "" {
		ownerType, ownerID = "installation", principal.InstallationID
	}
	var response struct {
		InstanceID       string          `json:"instance_id"`
		ExternalID       string          `json:"external_id"`
		DisplayName      string          `json:"display_name"`
		State            string          `json:"state"`
		ProviderMetadata json.RawMessage `json:"provider_metadata"`
		ExpiresAt        *time.Time      `json:"expires_at"`
	}
	err = r.call(ctx, connection, definition, create, map[string]any{"deployment_id": deploymentID, "integration_id": request.IntegrationID, "environment_id": request.EnvironmentID, "display_name": request.DisplayName, "owner": map[string]string{"type": ownerType, "id": ownerID}, "idempotency_key": request.IdempotencyKey, "ttl_seconds": request.TTLSeconds}, &response)
	if err != nil {
		return model.AccessInstance{}, err
	}
	if response.ExternalID == "" {
		response.ExternalID = response.InstanceID
	}
	if response.ExternalID == "" {
		return model.AccessInstance{}, ErrInvalidRequest
	}
	if response.DisplayName == "" {
		response.DisplayName = request.DisplayName
	}
	if response.State == "" {
		response.State = "active"
	}
	if len(response.ProviderMetadata) == 0 {
		response.ProviderMetadata = json.RawMessage(`{}`)
	}
	value, err := r.store.CreateAccessInstance(ctx, model.AccessInstance{ID: randomUUID(), DeploymentID: deploymentID, OrganisationID: connection.OrganisationID, AccessConnectionID: connectionID, EnvironmentID: request.EnvironmentID, OwnerType: ownerType, OwnerID: ownerID, ExternalID: response.ExternalID, DisplayName: response.DisplayName, IdempotencyKey: request.IdempotencyKey, State: response.State, ProviderMetadata: response.ProviderMetadata, ExpiresAt: response.ExpiresAt, IntegrationIDs: []string{request.IntegrationID}})
	if err == nil {
		if err := r.store.AppendAudit(ctx, model.AuditEvent{ID: "audit_" + randomUUID(), OrganisationID: connection.OrganisationID, ProductID: deploymentID, ActorID: principal.Subject, Action: "access_instance.created", TargetType: "access_instance", TargetID: value.ID, Current: map[string]any{"connection_id": connectionID, "integration_id": request.IntegrationID, "external_id": value.ExternalID, "state": value.State}, RequestID: principal.RequestID, CreatedAt: r.now()}); err != nil {
			return model.AccessInstance{}, err
		}
	}
	return value, err
}

func ownsInstance(value model.AccessInstance, principal Principal) bool {
	switch value.OwnerType {
	case "user":
		return value.OwnerID == principal.Subject
	case "installation":
		return value.OwnerID != "" && value.OwnerID == principal.InstallationID
	case "organisation":
		return value.OwnerID != "" && value.OwnerID == principal.ExternalCustomerID
	default:
		return false
	}
}

// ListInstances returns only resources owned by the calling identity and bound
// to the requested Integration. Provider inventory is never exposed across
// connections or Integration boundaries.
func (r *Runtime) ListInstances(ctx context.Context, deploymentID, connectionID, integrationID string, principal Principal) ([]model.AccessInstance, error) {
	_, definition, _, err := r.connectionAndDefinition(ctx, deploymentID, connectionID, integrationID, principal)
	if err != nil {
		return nil, err
	}
	if definition.InstanceCardinality != "many" {
		return []model.AccessInstance{}, nil
	}
	values, err := r.store.AccessInstances(ctx, deploymentID, connectionID)
	if err != nil {
		return nil, err
	}
	result := make([]model.AccessInstance, 0, len(values))
	for _, value := range values {
		if ownsInstance(value, principal) && contains(value.IntegrationIDs, integrationID) {
			result = append(result, value)
		}
	}
	return result, nil
}

// ListCredentials exposes metadata only. The model intentionally contains no
// plaintext credential material, and ownership is enforced again at runtime.
func (r *Runtime) ListCredentials(ctx context.Context, deploymentID, connectionID, integrationID, instanceID string, principal Principal) ([]model.AccessCredential, error) {
	_, definition, _, err := r.connectionAndDefinition(ctx, deploymentID, connectionID, integrationID, principal)
	if err != nil {
		return nil, err
	}
	if definition.CredentialScope == "connection" && instanceID != "" {
		return nil, ErrInvalidRequest
	}
	if definition.CredentialScope == "instance" {
		if instanceID == "" {
			return nil, ErrInvalidRequest
		}
		instance, lookupErr := r.store.AccessInstance(ctx, deploymentID, instanceID)
		if lookupErr != nil || instance.AccessConnectionID != connectionID || !contains(instance.IntegrationIDs, integrationID) || !ownsInstance(instance, principal) {
			return nil, ErrDenied
		}
	}
	values, err := r.store.AccessCredentials(ctx, deploymentID, connectionID, instanceID)
	if err != nil {
		return nil, err
	}
	result := make([]model.AccessCredential, 0, len(values))
	for _, value := range values {
		if value.SubjectID == principal.Subject {
			result = append(result, value)
		}
	}
	return result, nil
}
