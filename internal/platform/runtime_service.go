package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
)

var runtimeEnvironmentVariablePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
var runtimeHeaderNamePattern = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

var runtimeAuthenticationTypes = map[string]bool{
	"none":                     true,
	"delegated_oauth":          true,
	"bearer":                   true,
	"authorization_scheme":     true,
	"api_key_header":           true,
	"api_key_query":            true,
	"basic":                    true,
	"oauth_client_credentials": true,
	"custom_header":            true,
}

type RuntimeCredentialSetInput struct {
	EnvironmentID       string
	Scope               string
	OwnerIntegrationID  string
	Name                string
	EnvironmentVariable string
	AuthenticationType  string
	HeaderName          string
	Credential          string
	ExpiresAt           *time.Time
}

type RuntimeServiceConnectionInput struct {
	Name               string
	Description        string
	EnvironmentID      string
	BaseURL            string
	AuthenticationType string
	CredentialSetID    string
	AuthConfig         json.RawMessage
	State              string
	Revision           int64
}

// RuntimeSetupInput is the shortest-path configuration contract used by an
// API's Access screen. ExistingCredentialSetID selects a reviewed credential;
// otherwise CredentialScope creates a dedicated or shared set in one flow.
type RuntimeSetupInput struct {
	EnvironmentID           string
	ConnectionName          string
	ConnectionDescription   string
	BaseURL                 string
	AuthenticationType      string
	AuthConfig              json.RawMessage
	ExistingCredentialSetID string
	CredentialScope         string
	CredentialName          string
	EnvironmentVariable     string
	HeaderName              string
	Credential              string
	CredentialExpiresAt     *time.Time
}

func normalizeRuntimeAuthenticationType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "none"
	}
	if !runtimeAuthenticationTypes[value] {
		return "", errors.New("unsupported service authentication type")
	}
	return value, nil
}

func runtimeAuthenticationNeedsCredential(value string) bool {
	return value != "none" && value != "delegated_oauth"
}

func normalizeRuntimeAuthConfig(value json.RawMessage, authenticationType string) (json.RawMessage, error) {
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage(`{}`), nil
	}
	if len(value) > 32*1024 {
		return nil, errors.New("service authentication configuration is too large")
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return nil, errors.New("service authentication configuration must be a JSON object")
	}
	allowed := map[string]bool{}
	switch authenticationType {
	case "authorization_scheme":
		allowed["scheme"] = true
	case "api_key_query":
		allowed["query_name"] = true
	case "basic":
		allowed["username"] = true
	case "oauth_client_credentials":
		for _, key := range []string{"client_id", "token_url", "token_endpoint_auth_method", "scopes", "audience", "resource"} {
			allowed[key] = true
		}
	case "custom_header":
		allowed["prefix"] = true
	}
	for key := range object {
		if !allowed[key] {
			return nil, fmt.Errorf("service authentication configuration field %q is not allowed for %s", key, authenticationType)
		}
	}
	if rawTokenURL, ok := object["token_url"].(string); ok && !validHTTPSURI(strings.TrimSpace(rawTokenURL)) {
		return nil, errors.New("OAuth token URL must be HTTPS or a localhost HTTP URL")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

// RuntimeEnvironmentVariableForFamily applies the single canonical label rule
// used by runtime credentials and API-admin credential rotation results.
func RuntimeEnvironmentVariableForFamily(familyKey string, shared bool) string {
	if shared {
		return "SERVICE_API_KEY"
	}
	key := strings.ToUpper(familyKey)
	var result strings.Builder
	underscore := false
	for _, character := range key {
		if character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
			underscore = false
		} else if !underscore && result.Len() > 0 {
			result.WriteByte('_')
			underscore = true
		}
	}
	base := strings.Trim(result.String(), "_")
	base = strings.TrimSuffix(base, "_API")
	if base == "" {
		base = "SERVICE"
	}
	return base + "_API_KEY"
}

func defaultRuntimeEnvironmentVariable(integration model.Integration, scope string) string {
	return RuntimeEnvironmentVariableForFamily(integration.FamilyKey, scope == "shared")
}

func normalizeRuntimeCredentialSetInput(input RuntimeCredentialSetInput, integration model.Integration) (RuntimeCredentialSetInput, error) {
	input.EnvironmentID = strings.TrimSpace(input.EnvironmentID)
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	input.OwnerIntegrationID = strings.TrimSpace(input.OwnerIntegrationID)
	input.Name = strings.TrimSpace(input.Name)
	input.EnvironmentVariable = strings.ToUpper(strings.TrimSpace(input.EnvironmentVariable))
	input.HeaderName = strings.TrimSpace(input.HeaderName)
	if input.Scope == "" {
		input.Scope = "dedicated"
	}
	if input.Scope != "dedicated" && input.Scope != "shared" {
		return input, errors.New("credential scope must be dedicated or shared")
	}
	if input.Scope == "dedicated" {
		if input.OwnerIntegrationID == "" {
			input.OwnerIntegrationID = integration.ID
		}
		if input.OwnerIntegrationID != integration.ID {
			return input, errors.New("a dedicated credential must belong to this API")
		}
	} else {
		input.OwnerIntegrationID = ""
	}
	if input.Name == "" {
		if input.Scope == "shared" {
			input.Name = "Shared service credential"
		} else {
			input.Name = integration.DisplayName + " credential"
		}
	}
	if len(input.Name) > 120 {
		return input, errors.New("credential name must not exceed 120 characters")
	}
	if input.EnvironmentVariable == "" {
		input.EnvironmentVariable = defaultRuntimeEnvironmentVariable(integration, input.Scope)
	}
	if !runtimeEnvironmentVariablePattern.MatchString(input.EnvironmentVariable) {
		return input, errors.New("environment variable must use upper-case letters, numbers, and underscores")
	}
	var err error
	input.AuthenticationType, err = normalizeRuntimeAuthenticationType(input.AuthenticationType)
	if err != nil || !runtimeAuthenticationNeedsCredential(input.AuthenticationType) {
		if err != nil {
			return input, err
		}
		return input, errors.New("credential sets require a secret-bearing authentication type")
	}
	if (input.AuthenticationType == "api_key_header" || input.AuthenticationType == "custom_header") && input.HeaderName == "" {
		input.HeaderName = "X-API-Key"
	}
	if input.HeaderName != "" && !runtimeHeaderNamePattern.MatchString(input.HeaderName) {
		return input, errors.New("credential header name is invalid")
	}
	if strings.TrimSpace(input.Credential) == "" {
		return input, errors.New("credential value is required")
	}
	if len(input.Credential) > 16*1024 {
		return input, errors.New("credential value must not exceed 16 KB")
	}
	return input, nil
}

func (s *Service) RuntimeSetup(ctx context.Context, integrationID string) (model.RuntimeSetup, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	environments, err := s.store.Environments(ctx, deployment.ID)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	connections, err := s.store.RuntimeServiceConnections(ctx, deployment.ID, integration.ID)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	credentialSets, err := s.store.RuntimeCredentialSets(ctx, deployment.ID, "")
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	eligible := credentialSets[:0]
	for _, credentialSet := range credentialSets {
		if credentialSet.Scope == "shared" || credentialSet.OwnerIntegrationID == integration.ID {
			eligible = append(eligible, credentialSet)
		}
	}
	return model.RuntimeSetup{Integration: integration, Environments: environments, Connections: connections, CredentialSets: eligible}, nil
}

func (s *Service) storeRuntimeCredential(ctx context.Context, organisationID, credentialSetID, credential string) (model.Secret, string, error) {
	if s.vault == nil {
		return model.Secret{}, "", errors.New("runtime credential encryption is not configured")
	}
	secretID, err := randomUUID()
	if err != nil {
		return model.Secret{}, "", err
	}
	encrypted, err := s.vault.Encrypt([]byte(credential), organisationID+":runtime_credential:"+secretID)
	if err != nil {
		return model.Secret{}, "", err
	}
	secret, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: organisationID, Name: "runtime-credential-" + credentialSetID + "-" + secretID, Purpose: "runtime_service_credential", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	return secret, secretID, err
}

func (s *Service) cleanupRuntimeCredential(ctx context.Context, organisationID, secretID string, operationErr error) error {
	if secretID == "" {
		return operationErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := s.store.DeleteSecret(cleanupCtx, organisationID, secretID); err != nil {
		return errors.Join(operationErr, fmt.Errorf("stored runtime credential cleanup failed: %w", err))
	}
	return operationErr
}

func (s *Service) CreateRuntimeCredentialSet(ctx context.Context, integrationID string, input RuntimeCredentialSetInput, actor Actor) (model.RuntimeCredentialSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	input, err = normalizeRuntimeCredentialSetInput(input, integration)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if _, err := s.store.RuntimeCredentialSets(ctx, deployment.ID, input.EnvironmentID); err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	setID, err := randomUUID()
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	secret, secretID, err := s.storeRuntimeCredential(ctx, deployment.OrganisationID, setID, input.Credential)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	created, err := s.store.CreateRuntimeCredentialSet(ctx, model.RuntimeCredentialSet{ID: setID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, EnvironmentID: input.EnvironmentID, Scope: input.Scope, OwnerIntegrationID: input.OwnerIntegrationID, Name: input.Name, EnvironmentVariable: input.EnvironmentVariable, AuthenticationType: input.AuthenticationType, HeaderName: input.HeaderName, State: "active"})
	if err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	versionID, err := randomUUID()
	if err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	version, err := s.store.CreateRuntimeCredentialVersion(ctx, model.RuntimeCredentialVersion{ID: versionID, CredentialSetID: created.ID, SecretID: secret.ID, Fingerprint: secret.Fingerprint, CreatedBy: "", ExpiresAt: input.ExpiresAt})
	if err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	if _, err = s.store.ActivateRuntimeCredentialVersion(ctx, deployment.ID, created.ID, version.ID, s.now()); err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	result, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, created.ID)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "runtime_credential_set.created", TargetType: "runtime_credential_set", TargetID: result.ID, Current: map[string]any{"scope": result.Scope, "owner_integration_id": result.OwnerIntegrationID, "environment_id": result.EnvironmentID, "environment_variable": result.EnvironmentVariable, "authentication_type": result.AuthenticationType, "fingerprint": result.ActiveFingerprint}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	return result, nil
}

func (s *Service) RotateRuntimeCredential(ctx context.Context, credentialSetID, credential string, expiresAt *time.Time, actor Actor) (model.RuntimeCredentialSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	credentialSet, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, strings.TrimSpace(credentialSetID))
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if credentialSet.State != "active" {
		return model.RuntimeCredentialSet{}, errors.New("credential set must be active before rotation")
	}
	if strings.TrimSpace(credential) == "" {
		return model.RuntimeCredentialSet{}, errors.New("credential value is required")
	}
	if len(credential) > 16*1024 {
		return model.RuntimeCredentialSet{}, errors.New("credential value must not exceed 16 KB")
	}
	secret, secretID, err := s.storeRuntimeCredential(ctx, deployment.OrganisationID, credentialSet.ID, credential)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	versionID, err := randomUUID()
	if err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	version, err := s.store.CreateRuntimeCredentialVersion(ctx, model.RuntimeCredentialVersion{ID: versionID, CredentialSetID: credentialSet.ID, SecretID: secret.ID, Fingerprint: secret.Fingerprint, CreatedBy: "", ExpiresAt: expiresAt})
	if err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	if _, err = s.store.ActivateRuntimeCredentialVersion(ctx, deployment.ID, credentialSet.ID, version.ID, s.now()); err != nil {
		return model.RuntimeCredentialSet{}, s.cleanupRuntimeCredential(ctx, deployment.OrganisationID, secretID, err)
	}
	updated, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, credentialSet.ID)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "runtime_credential.rotated", TargetType: "runtime_credential_set", TargetID: updated.ID, Prior: map[string]any{"fingerprint": credentialSet.ActiveFingerprint}, Current: map[string]any{"fingerprint": updated.ActiveFingerprint, "affected_connections": s.runtimeCredentialConnectionCount(ctx, deployment.ID, updated.ID)}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	return updated, nil
}

func (s *Service) RevokeRuntimeCredentialVersion(ctx context.Context, credentialSetID, versionID string, actor Actor) (model.RuntimeCredentialSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	credentialSet, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, strings.TrimSpace(credentialSetID))
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	version, err := s.store.RevokeRuntimeCredentialVersion(ctx, deployment.ID, credentialSet.ID, strings.TrimSpace(versionID), s.now())
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	updated, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, credentialSet.ID)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "runtime_credential.revoked", TargetType: "runtime_credential_version", TargetID: version.ID, Current: map[string]any{"credential_set_id": credentialSet.ID, "state": version.State, "fingerprint": version.Fingerprint, "credential_present": updated.CredentialPresent}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	return updated, nil
}

func normalizeRuntimeServiceConnectionInput(input RuntimeServiceConnectionInput) (RuntimeServiceConnectionInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.EnvironmentID = strings.TrimSpace(input.EnvironmentID)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.CredentialSetID = strings.TrimSpace(input.CredentialSetID)
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	if input.Name == "" {
		input.Name = "Default"
	}
	if len(input.Name) > 120 || len(input.Description) > 500 {
		return input, errors.New("service connection name or description is too long")
	}
	if input.EnvironmentID == "" {
		return input, errors.New("environment is required")
	}
	if !validHTTPSBaseOrigin(input.BaseURL) {
		return input, errors.New("service base URL must be a credential-free HTTPS origin or localhost HTTP origin")
	}
	var err error
	input.AuthenticationType, err = normalizeRuntimeAuthenticationType(input.AuthenticationType)
	if err != nil {
		return input, err
	}
	input.AuthConfig, err = normalizeRuntimeAuthConfig(input.AuthConfig, input.AuthenticationType)
	if err != nil {
		return input, err
	}
	if runtimeAuthenticationNeedsCredential(input.AuthenticationType) != (input.CredentialSetID != "") {
		return input, errors.New("service credential selection does not match the authentication type")
	}
	if input.State == "" {
		input.State = "active"
	}
	if input.State != "active" && input.State != "disabled" {
		return input, errors.New("service connection state must be active or disabled")
	}
	return input, nil
}

func runtimeConnectionContentHash(input RuntimeServiceConnectionInput) (string, error) {
	canonical, err := json.Marshal(map[string]any{"environment_id": input.EnvironmentID, "base_url": input.BaseURL, "authentication_type": input.AuthenticationType, "credential_set_id": input.CredentialSetID, "auth_config": json.RawMessage(input.AuthConfig)})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (s *Service) ConfigureRuntimeServiceConnection(ctx context.Context, integrationID string, input RuntimeServiceConnectionInput, actor Actor) (model.RuntimeServiceConnection, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	input, err = normalizeRuntimeServiceConnectionInput(input)
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	if input.CredentialSetID != "" {
		credentialSet, credentialErr := s.store.RuntimeCredentialSet(ctx, deployment.ID, input.CredentialSetID)
		if credentialErr != nil {
			return model.RuntimeServiceConnection{}, credentialErr
		}
		if credentialSet.EnvironmentID != input.EnvironmentID || credentialSet.AuthenticationType != input.AuthenticationType || credentialSet.State != "active" || !credentialSet.CredentialPresent || credentialSet.Scope == "dedicated" && credentialSet.OwnerIntegrationID != integration.ID {
			return model.RuntimeServiceConnection{}, errors.New("selected credential is not eligible for this API, environment, or authentication type")
		}
	}
	connections, err := s.store.RuntimeServiceConnections(ctx, deployment.ID, integration.ID)
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	var connection model.RuntimeServiceConnection
	for _, candidate := range connections {
		if candidate.Name == input.Name {
			connection = candidate
			break
		}
	}
	if connection.ID == "" {
		connectionID, idErr := randomUUID()
		if idErr != nil {
			return model.RuntimeServiceConnection{}, idErr
		}
		connection, err = s.store.CreateRuntimeServiceConnection(ctx, model.RuntimeServiceConnection{ID: connectionID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, IntegrationID: integration.ID, Name: input.Name, Description: input.Description, State: input.State})
		if err != nil {
			return model.RuntimeServiceConnection{}, err
		}
	} else if connection.Description != input.Description || connection.State != input.State {
		connection.Description, connection.State = input.Description, input.State
		connection, err = s.store.UpdateRuntimeServiceConnection(ctx, connection, connection.Revision)
		if err != nil {
			return model.RuntimeServiceConnection{}, err
		}
	}
	contentHash, err := runtimeConnectionContentHash(input)
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	for _, current := range connection.CurrentRevisions {
		if current.EnvironmentID == input.EnvironmentID && current.ContentHash == contentHash {
			return connection, nil
		}
	}
	revisionID, err := randomUUID()
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	createdRevision, err := s.store.CreateRuntimeServiceConnectionRevision(ctx, model.RuntimeServiceConnectionRevision{ID: revisionID, ConnectionID: connection.ID, EnvironmentID: input.EnvironmentID, BaseURL: input.BaseURL, AuthenticationType: input.AuthenticationType, CredentialSetID: input.CredentialSetID, AuthConfig: input.AuthConfig, ContentHash: contentHash, CreatedBy: ""})
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	updated, err := s.store.RuntimeServiceConnection(ctx, deployment.ID, connection.ID)
	if err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "runtime_service_connection.configured", TargetType: "runtime_service_connection", TargetID: updated.ID, Current: map[string]any{"integration_id": integration.ID, "environment_id": input.EnvironmentID, "base_url": input.BaseURL, "authentication_type": input.AuthenticationType, "credential_set_id": input.CredentialSetID, "connection_revision_id": createdRevision.ID, "connection_revision": createdRevision.Revision, "content_hash": createdRevision.ContentHash}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.RuntimeServiceConnection{}, err
	}
	return updated, nil
}

func (s *Service) ConfigureRuntimeSetup(ctx context.Context, integrationID string, input RuntimeSetupInput, actor Actor) (model.RuntimeSetup, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	integration, err := s.store.Integration(ctx, deployment.ID, strings.TrimSpace(integrationID))
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	authenticationType, err := normalizeRuntimeAuthenticationType(input.AuthenticationType)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	credentialSetID := strings.TrimSpace(input.ExistingCredentialSetID)
	if runtimeAuthenticationNeedsCredential(authenticationType) && credentialSetID == "" {
		created, createErr := s.CreateRuntimeCredentialSet(ctx, integration.ID, RuntimeCredentialSetInput{EnvironmentID: input.EnvironmentID, Scope: input.CredentialScope, Name: input.CredentialName, EnvironmentVariable: input.EnvironmentVariable, AuthenticationType: authenticationType, HeaderName: input.HeaderName, Credential: input.Credential, ExpiresAt: input.CredentialExpiresAt}, actor)
		if createErr != nil {
			return model.RuntimeSetup{}, createErr
		}
		credentialSetID = created.ID
	}
	if !runtimeAuthenticationNeedsCredential(authenticationType) && credentialSetID != "" {
		return model.RuntimeSetup{}, errors.New("this authentication type does not use a service credential")
	}
	_, err = s.ConfigureRuntimeServiceConnection(ctx, integration.ID, RuntimeServiceConnectionInput{Name: input.ConnectionName, Description: input.ConnectionDescription, EnvironmentID: input.EnvironmentID, BaseURL: input.BaseURL, AuthenticationType: authenticationType, CredentialSetID: credentialSetID, AuthConfig: input.AuthConfig, State: "active"}, actor)
	if err != nil {
		return model.RuntimeSetup{}, err
	}
	return s.RuntimeSetup(ctx, integration.ID)
}

func (s *Service) runtimeCredentialConnectionCount(ctx context.Context, deploymentID, credentialSetID string) int {
	connections, err := s.store.RuntimeServiceConnections(ctx, deploymentID, "")
	if err != nil {
		return 0
	}
	count := 0
	for _, connection := range connections {
		for _, revision := range connection.CurrentRevisions {
			if revision.CredentialSetID == credentialSetID {
				count++
				break
			}
		}
	}
	return count
}

func (s *Service) RuntimeCredentialUsage(ctx context.Context, credentialSetID string) ([]model.RuntimeServiceConnection, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, strings.TrimSpace(credentialSetID)); err != nil {
		return nil, err
	}
	connections, err := s.store.RuntimeServiceConnections(ctx, deployment.ID, "")
	if err != nil {
		return nil, err
	}
	result := make([]model.RuntimeServiceConnection, 0)
	for _, connection := range connections {
		for _, revision := range connection.CurrentRevisions {
			if revision.CredentialSetID == credentialSetID {
				result = append(result, connection)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func runtimeCredentialSetIndex(values []model.RuntimeCredentialSet) map[string]model.RuntimeCredentialSet {
	result := make(map[string]model.RuntimeCredentialSet, len(values))
	for _, value := range values {
		if value.ID != "" {
			result[value.ID] = value
		}
	}
	return result
}

// normalizedRuntimeServiceConnectionRevision verifies that one stored current
// revision is the exact immutable representation its content hash claims. The
// returned input contains the canonical, non-secret authentication config used
// by publication snapshots.
func normalizedRuntimeServiceConnectionRevision(connection model.RuntimeServiceConnection, revision model.RuntimeServiceConnectionRevision) (RuntimeServiceConnectionInput, bool) {
	if connection.ID == "" || revision.ID == "" || revision.ConnectionID != connection.ID || revision.EnvironmentID == "" || revision.Revision < 1 || !revision.Current {
		return RuntimeServiceConnectionInput{}, false
	}
	input, err := normalizeRuntimeServiceConnectionInput(RuntimeServiceConnectionInput{Name: connection.Name, Description: connection.Description, EnvironmentID: revision.EnvironmentID, BaseURL: revision.BaseURL, AuthenticationType: revision.AuthenticationType, CredentialSetID: revision.CredentialSetID, AuthConfig: revision.AuthConfig, State: connection.State})
	if err != nil || input.BaseURL != revision.BaseURL || input.AuthenticationType != revision.AuthenticationType || input.CredentialSetID != revision.CredentialSetID {
		return RuntimeServiceConnectionInput{}, false
	}
	expectedHash, err := runtimeConnectionContentHash(input)
	if err != nil || expectedHash != revision.ContentHash {
		return RuntimeServiceConnectionInput{}, false
	}
	return input, true
}

func runtimeServiceCredentialReady(connection model.RuntimeServiceConnection, revision model.RuntimeServiceConnectionRevision, credentialSets map[string]model.RuntimeCredentialSet) bool {
	if !runtimeAuthenticationNeedsCredential(revision.AuthenticationType) {
		return revision.CredentialSetID == ""
	}
	credentialSet, ok := credentialSets[revision.CredentialSetID]
	if !ok || credentialSet.ID == "" || credentialSet.Revision < 1 || credentialSet.DeploymentID != connection.DeploymentID || credentialSet.OrganisationID != connection.OrganisationID || credentialSet.EnvironmentID != revision.EnvironmentID || credentialSet.AuthenticationType != revision.AuthenticationType || credentialSet.State != "active" || !credentialSet.CredentialPresent || credentialSet.ActiveFingerprint == "" {
		return false
	}
	switch credentialSet.Scope {
	case "shared":
		return credentialSet.OwnerIntegrationID == ""
	case "dedicated":
		return credentialSet.OwnerIntegrationID == connection.IntegrationID
	default:
		return false
	}
}

func runtimeServiceConnectionReadinessFrom(connection model.RuntimeServiceConnection, credentialSets map[string]model.RuntimeCredentialSet) model.RuntimeServiceConnectionReadiness {
	result := model.RuntimeServiceConnectionReadiness{ConnectionID: connection.ID, Ready: true}
	add := func(key, label string, ready bool, message, environmentID string) {
		result.Checks = append(result.Checks, model.RuntimeServiceConnectionCheck{Key: key, Label: label, Ready: ready, Message: message, EnvironmentID: environmentID})
		result.Ready = result.Ready && ready
	}
	connectionReady := connection.ID != "" && connection.DeploymentID != "" && connection.OrganisationID != "" && connection.IntegrationID != "" && connection.Revision > 0 && connection.State == "active"
	connectionMessage := "The service connection is active."
	if !connectionReady {
		connectionMessage = "Enable a valid API-owned service connection before publication."
	}
	add("connection_state", "Connection enabled", connectionReady, connectionMessage, "")
	if len(connection.CurrentRevisions) == 0 {
		add("current_revision", "Environment configured", false, "Add an endpoint for at least one environment.", "")
		return result
	}
	seenEnvironments := make(map[string]bool, len(connection.CurrentRevisions))
	for _, revision := range connection.CurrentRevisions {
		_, exact := normalizedRuntimeServiceConnectionRevision(connection, revision)
		if seenEnvironments[revision.EnvironmentID] {
			exact = false
		}
		seenEnvironments[revision.EnvironmentID] = true
		configurationMessage := "The exact immutable environment revision is current."
		if !exact {
			configurationMessage = "Save a valid current immutable revision for this environment."
		}
		add("current_revision", "Published configuration", exact, configurationMessage, revision.EnvironmentID)
		validTarget := exact && validHTTPSBaseOrigin(revision.BaseURL)
		endpointMessage := "The endpoint origin is valid."
		if !validTarget {
			endpointMessage = "Replace the endpoint with an HTTPS origin or localhost HTTP origin."
		}
		add("endpoint", "Endpoint", validTarget, endpointMessage, revision.EnvironmentID)
		credentialReady := exact && runtimeServiceCredentialReady(connection, revision, credentialSets)
		credentialMessage := "This authentication mode does not require a stored service credential."
		if runtimeAuthenticationNeedsCredential(revision.AuthenticationType) {
			credentialMessage = "An active encrypted credential is available."
		}
		if !credentialReady {
			credentialMessage = "Select an active compatible credential for this API and environment."
		}
		add("credential", "Credential", credentialReady, credentialMessage, revision.EnvironmentID)
	}
	return result
}

// RuntimeServiceConnectionReadiness performs a deterministic, value-free
// configuration check. Live HTTP behavior is validated through a tool test so
// DokoSoko never sends an unspecified probe to a customer service.
func (s *Service) RuntimeServiceConnectionReadiness(ctx context.Context, connectionID string) (model.RuntimeServiceConnectionReadiness, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeServiceConnectionReadiness{}, err
	}
	connection, err := s.store.RuntimeServiceConnection(ctx, deployment.ID, strings.TrimSpace(connectionID))
	if err != nil {
		return model.RuntimeServiceConnectionReadiness{}, err
	}
	credentialSets, err := s.store.RuntimeCredentialSets(ctx, deployment.ID, "")
	if err != nil {
		return model.RuntimeServiceConnectionReadiness{}, err
	}
	return runtimeServiceConnectionReadinessFrom(connection, runtimeCredentialSetIndex(credentialSets)), nil
}

func decryptRuntimeCredential(vault *secretvault.Vault, secret model.Secret) ([]byte, error) {
	if vault == nil {
		return nil, errors.New("runtime credential encryption is not configured")
	}
	return vault.Decrypt(secretvault.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, KeyVersion: secret.KeyVersion, Fingerprint: secret.Fingerprint}, secret.OrganisationID+":runtime_credential:"+secret.ID)
}
