package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type AccessDefinitionInput struct {
	ServiceKey            string
	Name                  string
	InstanceCardinality   string
	InstanceLabelSingular string
	InstanceLabelPlural   string
	CredentialScope       string
	ManagementAuthType    string
	APIResourceSetID      string
	Operations            json.RawMessage
}

func normalizeObject(raw json.RawMessage, fallback string, maximum int) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(fallback)
	}
	if len(raw) > maximum {
		return nil, errors.New("configuration exceeds its size limit")
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New("configuration must be a JSON object")
	}
	return json.Marshal(value)
}

func normalizeAccessDefinition(input AccessDefinitionInput) (AccessDefinitionInput, error) {
	input.ServiceKey = strings.ToLower(strings.TrimSpace(input.ServiceKey))
	input.Name = strings.TrimSpace(input.Name)
	input.InstanceCardinality = strings.TrimSpace(input.InstanceCardinality)
	input.InstanceLabelSingular = strings.TrimSpace(input.InstanceLabelSingular)
	input.InstanceLabelPlural = strings.TrimSpace(input.InstanceLabelPlural)
	input.CredentialScope = strings.TrimSpace(input.CredentialScope)
	input.ManagementAuthType = strings.TrimSpace(input.ManagementAuthType)
	input.APIResourceSetID = strings.TrimSpace(input.APIResourceSetID)
	if !slugPattern.MatchString(input.ServiceKey) || len(input.ServiceKey) > 63 || input.Name == "" || len(input.Name) > 120 {
		return input, errors.New("access definition service key or name is invalid")
	}
	if input.InstanceCardinality != "one" && input.InstanceCardinality != "many" {
		return input, errors.New("instance cardinality must be one or many")
	}
	if input.InstanceLabelSingular == "" || input.InstanceLabelPlural == "" || len(input.InstanceLabelSingular) > 80 || len(input.InstanceLabelPlural) > 80 {
		return input, errors.New("provider-specific instance labels are required")
	}
	if input.CredentialScope != "connection" && input.CredentialScope != "instance" {
		return input, errors.New("credential scope must be connection or instance")
	}
	if input.InstanceCardinality == "one" && input.CredentialScope != "connection" {
		return input, errors.New("single-instance services require connection-scoped credentials")
	}
	if input.ManagementAuthType == "" {
		input.ManagementAuthType = "bearer"
	}
	switch input.ManagementAuthType {
	case "none", "bearer", "api_key", "oauth2_client_credentials":
	default:
		return input, errors.New("unsupported management authentication type")
	}
	operations, err := normalizeObject(input.Operations, `{}`, 1<<20)
	if err != nil {
		return input, err
	}
	input.Operations = operations
	return input, nil
}

func (s *Service) CreateAccessDefinition(ctx context.Context, input AccessDefinitionInput, actor Actor) (model.AccessDefinition, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	input, err = normalizeAccessDefinition(input)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	if input.APIResourceSetID != "" {
		set, err := s.store.ResourceSet(ctx, deployment.ID, input.APIResourceSetID)
		if err != nil || set.Kind != "api" {
			return model.AccessDefinition{}, errors.New("access definition API resource set must reference an API resource set")
		}
	}
	id, err := randomUUID()
	if err != nil {
		return model.AccessDefinition{}, err
	}
	value, err := s.store.CreateAccessDefinition(ctx, model.AccessDefinition{ID: id, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, ServiceKey: input.ServiceKey, Name: input.Name, InstanceCardinality: input.InstanceCardinality, InstanceLabelSingular: input.InstanceLabelSingular, InstanceLabelPlural: input.InstanceLabelPlural, CredentialScope: input.CredentialScope, ManagementAuthType: input.ManagementAuthType, APIResourceSetID: input.APIResourceSetID, Operations: input.Operations, State: "active"})
	if err != nil {
		return model.AccessDefinition{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "access_definition.created", TargetType: "access_definition", TargetID: value.ID, Current: map[string]any{"service_key": value.ServiceKey, "instance_cardinality": value.InstanceCardinality, "credential_scope": value.CredentialScope}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}

func (s *Service) UpdateAccessDefinition(ctx context.Context, id string, input AccessDefinitionInput, expectedRevision int64, actor Actor) (model.AccessDefinition, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	current, err := s.store.AccessDefinition(ctx, deployment.ID, strings.TrimSpace(id))
	if err != nil {
		return model.AccessDefinition{}, err
	}
	if expectedRevision < 1 {
		return model.AccessDefinition{}, errors.New("access definition revision is required")
	}

	// A revision may evolve the documented surface without changing the identity or
	// interpretation of an existing encrypted connection.
	input.ServiceKey = current.ServiceKey
	input.InstanceCardinality = current.InstanceCardinality
	input.CredentialScope = current.CredentialScope
	input.ManagementAuthType = current.ManagementAuthType
	input, err = normalizeAccessDefinition(input)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	if input.APIResourceSetID != "" {
		set, err := s.store.ResourceSet(ctx, deployment.ID, input.APIResourceSetID)
		if err != nil || set.Kind != "api" {
			return model.AccessDefinition{}, errors.New("access definition API resource set must reference an API resource set")
		}
	}

	value := current
	value.Name = input.Name
	value.InstanceLabelSingular = input.InstanceLabelSingular
	value.InstanceLabelPlural = input.InstanceLabelPlural
	value.APIResourceSetID = input.APIResourceSetID
	value.Operations = input.Operations
	updated, err := s.store.UpdateAccessDefinition(ctx, value, expectedRevision)
	if err != nil {
		return model.AccessDefinition{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "access_definition.updated", TargetType: "access_definition", TargetID: updated.ID, Prior: map[string]any{"revision": current.Revision, "api_resource_set_id": current.APIResourceSetID}, Current: map[string]any{"revision": updated.Revision, "api_resource_set_id": updated.APIResourceSetID}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return updated, nil
}

type AccessConnectionInput struct {
	AccessDefinitionID string
	EnvironmentID      string
	Name               string
	Region             string
	BaseURL            string
	ManagementSecret   string
	Config             json.RawMessage
	IntegrationIDs     []string
}

func (s *Service) CreateAccessConnection(ctx context.Context, input AccessConnectionInput, actor Actor) (model.AccessConnection, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.AccessConnection{}, err
	}
	input.AccessDefinitionID, input.EnvironmentID = strings.TrimSpace(input.AccessDefinitionID), strings.TrimSpace(input.EnvironmentID)
	input.Name, input.Region, input.BaseURL, input.ManagementSecret = strings.TrimSpace(input.Name), strings.TrimSpace(input.Region), strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), strings.TrimSpace(input.ManagementSecret)
	definition, err := s.store.AccessDefinition(ctx, deployment.ID, input.AccessDefinitionID)
	if err != nil {
		return model.AccessConnection{}, err
	}
	if input.Name == "" || len(input.Name) > 120 {
		return model.AccessConnection{}, errors.New("access connection name is required")
	}
	if input.BaseURL == "" || !validHTTPSOrigin(input.BaseURL) {
		return model.AccessConnection{}, errors.New("access connection requires a fixed HTTPS base URL on the default port")
	}
	if definition.ManagementAuthType != "none" && input.ManagementSecret == "" {
		return model.AccessConnection{}, errors.New("management credentials are required for this service")
	}
	config, err := normalizeObject(input.Config, `{}`, 64<<10)
	if err != nil {
		return model.AccessConnection{}, err
	}
	allowedIntegrations := make([]string, 0, len(input.IntegrationIDs))
	seen := make(map[string]bool)
	for _, integrationID := range input.IntegrationIDs {
		integrationID = strings.TrimSpace(integrationID)
		if integrationID == "" || seen[integrationID] {
			continue
		}
		if _, err := s.store.Integration(ctx, deployment.ID, integrationID); err != nil {
			return model.AccessConnection{}, errors.New("every access connection binding must reference an integration in this deployment")
		}
		seen[integrationID] = true
		allowedIntegrations = append(allowedIntegrations, integrationID)
	}
	if len(allowedIntegrations) == 0 {
		return model.AccessConnection{}, errors.New("attach the access connection to at least one integration")
	}
	connectionID, err := randomUUID()
	if err != nil {
		return model.AccessConnection{}, err
	}
	secretID := ""
	if input.ManagementSecret != "" {
		if s.vault == nil {
			return model.AccessConnection{}, errors.New("management credential encryption is not configured")
		}
		secretID, err = randomUUID()
		if err != nil {
			return model.AccessConnection{}, err
		}
		encrypted, err := s.vault.Encrypt([]byte(input.ManagementSecret), deployment.OrganisationID+":access:"+secretID)
		if err != nil {
			return model.AccessConnection{}, err
		}
		if _, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: deployment.OrganisationID, Name: "access-" + connectionID, Purpose: "access_management", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return model.AccessConnection{}, err
		}
	}
	value, err := s.store.CreateAccessConnection(ctx, model.AccessConnection{ID: connectionID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, AccessDefinitionID: definition.ID, EnvironmentID: input.EnvironmentID, Name: input.Name, Region: input.Region, BaseURL: input.BaseURL, ManagementSecretID: secretID, Config: config, State: "active", IntegrationIDs: allowedIntegrations})
	if err != nil {
		return model.AccessConnection{}, err
	}
	_ = s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "access_connection.created", TargetType: "access_connection", TargetID: value.ID, Current: map[string]any{"access_definition_id": definition.ID, "environment_id": value.EnvironmentID, "integration_ids": value.IntegrationIDs}, RequestID: actor.RequestID, CreatedAt: s.now()})
	return value, nil
}
