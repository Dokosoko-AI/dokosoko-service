package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/runtimeauth"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
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
	AuthConfig          json.RawMessage
	KeyManagementURL    string
	AccessEvaluationURL string
	UsageURL            string
	Credential          string
	AdditionalHeaders   *[]RuntimeAuthorizationHeaderInput
	ExpiresAt           *time.Time
	credentialMaterial  []byte
}

// RuntimeAuthorizationHeaderInput is write-only request material. The name is
// retained in non-secret auth_config; the value is stored only in the encrypted
// active credential version. An empty value on update preserves the current
// value for the same case-insensitive header name.
type RuntimeAuthorizationHeaderInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RuntimeAuthorizationUpdateInput updates operator-controlled, non-secret
// Authorization metadata. Environment and authentication method are immutable;
// changing either creates a new Authorization instead of silently changing all
// APIs that share it.
type RuntimeAuthorizationUpdateInput struct {
	EnvironmentVariable string
	HeaderName          string
	AuthConfig          json.RawMessage
	KeyManagementURL    string
	AccessEvaluationURL string
	UsageURL            string
	Credential          string
	AdditionalHeaders   *[]RuntimeAuthorizationHeaderInput
	State               string
	Revision            int64
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

// RuntimeSetupInput binds one reusable Authorization to an API endpoint.
type RuntimeSetupInput struct {
	EnvironmentID         string
	ConnectionName        string
	ConnectionDescription string
	BaseURL               string
	AuthenticationType    string
	AuthConfig            json.RawMessage
	AuthorizationID       string
	EnvironmentVariable   string
	HeaderName            string
	KeyManagementURL      string
	AccessEvaluationURL   string
	UsageURL              string
	Credential            string
	AdditionalHeaders     *[]RuntimeAuthorizationHeaderInput
	CredentialExpiresAt   *time.Time
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

// validOutboundHookURI mirrors the outbound runtime network policy so stored
// delivery and Authorization hooks cannot target an address execution denies.
func validOutboundHookURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	local := identity.IsLocalDevelopmentHostname(parsed.Hostname())
	if local {
		return parsed.Scheme == "http"
	}
	return parsed.Scheme == "https" && (parsed.Port() == "" || parsed.Port() == "443")
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
	allowed := map[string]bool{"headers": true}
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
	if rawHeaders, ok := object["headers"]; ok {
		values, ok := rawHeaders.([]any)
		if !ok || len(values) > runtimeauth.MaxHeaders {
			return nil, fmt.Errorf("service authentication headers must contain at most %d names", runtimeauth.MaxHeaders)
		}
		seen := make(map[string]bool, len(values))
		headers := make([]string, 0, len(values))
		for _, raw := range values {
			name, ok := raw.(string)
			name = strings.TrimSpace(name)
			key := strings.ToLower(name)
			if !ok || !runtimeauth.SafeHeaderName(name) {
				return nil, errors.New("service authentication headers must use safe HTTP header names")
			}
			if seen[key] {
				return nil, errors.New("service authentication header names must be unique")
			}
			seen[key] = true
			headers = append(headers, name)
		}
		if len(headers) == 0 {
			delete(object, "headers")
		} else {
			object["headers"] = headers
		}
	}
	if rawTokenURL, ok := object["token_url"].(string); ok && !validOutboundHookURI(strings.TrimSpace(rawTokenURL)) {
		return nil, errors.New("OAuth token URL must be HTTPS or a localhost HTTP URL")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func runtimeAuthConfigHeaderNames(value json.RawMessage) []string {
	var object struct {
		Headers []string `json:"headers"`
	}
	if json.Unmarshal(value, &object) != nil {
		return nil
	}
	return append([]string(nil), object.Headers...)
}

func withRuntimeAuthConfigHeaderNames(value json.RawMessage, names []string) (json.RawMessage, error) {
	object := map[string]any{}
	if len(value) > 0 && string(value) != "null" {
		if err := json.Unmarshal(value, &object); err != nil || object == nil {
			return nil, errors.New("service authentication configuration must be a JSON object")
		}
	}
	if len(names) == 0 {
		delete(object, "headers")
	} else {
		object["headers"] = append([]string(nil), names...)
	}
	return json.Marshal(object)
}

func normalizeRuntimeAuthorizationHeaderInputs(values []RuntimeAuthorizationHeaderInput, existing map[string][]byte, preserveEmpty bool) ([]runtimeauth.Header, error) {
	if len(values) > runtimeauth.MaxHeaders {
		return nil, fmt.Errorf("at most %d additional Authorization headers are allowed", runtimeauth.MaxHeaders)
	}
	result := make([]runtimeauth.Header, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		key := strings.ToLower(name)
		if !runtimeauth.SafeHeaderName(name) {
			return nil, fmt.Errorf("Authorization header %q is not allowed", name)
		}
		if seen[key] {
			return nil, fmt.Errorf("Authorization header %q is duplicated", name)
		}
		seen[key] = true
		headerValue := []byte(value.Value)
		if len(headerValue) == 0 && preserveEmpty {
			headerValue = bytes.Clone(existing[key])
		}
		if len(headerValue) == 0 || len(headerValue) > runtimeauth.MaxCredentialBytes || bytes.ContainsAny(headerValue, "\r\n\x00") {
			return nil, fmt.Errorf("enter a value for Authorization header %q", name)
		}
		result = append(result, runtimeauth.Header{Name: name, Value: headerValue})
	}
	return result, nil
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
	input.EnvironmentVariable = strings.ToUpper(strings.TrimSpace(input.EnvironmentVariable))
	input.HeaderName = strings.TrimSpace(input.HeaderName)
	input.KeyManagementURL = strings.TrimSpace(input.KeyManagementURL)
	input.AccessEvaluationURL = strings.TrimSpace(input.AccessEvaluationURL)
	input.UsageURL = strings.TrimSpace(input.UsageURL)
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
	if input.AuthenticationType == "api_key_header" || input.AuthenticationType == "custom_header" {
		if input.HeaderName == "" {
			input.HeaderName = "X-API-Key"
		}
		if len(input.HeaderName) > 100 || !runtimeHeaderNamePattern.MatchString(input.HeaderName) {
			return input, errors.New("credential header name is invalid")
		}
	} else {
		input.HeaderName = ""
	}
	if input.AdditionalHeaders != nil {
		headers, headerErr := normalizeRuntimeAuthorizationHeaderInputs(*input.AdditionalHeaders, nil, false)
		if headerErr != nil {
			return input, headerErr
		}
		names := make([]string, len(headers))
		for index, header := range headers {
			if input.HeaderName != "" && strings.EqualFold(input.HeaderName, header.Name) {
				return input, errors.New("the primary and additional Authorization header names must be unique")
			}
			names[index] = header.Name
		}
		input.AuthConfig, err = withRuntimeAuthConfigHeaderNames(input.AuthConfig, names)
		if err != nil {
			return input, err
		}
	}
	input.AuthConfig, err = normalizeRuntimeAuthConfig(input.AuthConfig, input.AuthenticationType)
	if err != nil {
		return input, err
	}
	if input.AdditionalHeaders == nil && len(runtimeAuthConfigHeaderNames(input.AuthConfig)) > 0 {
		return input, errors.New("additional Authorization header values are required")
	}
	if _, _, err = runtimeTargetAuth(input.AuthenticationType, input.AuthConfig, input.HeaderName); err != nil {
		return input, fmt.Errorf("authorization profile: %w", err)
	}
	if len(input.KeyManagementURL) > 2048 || input.KeyManagementURL != "" && !validHTTPSURI(input.KeyManagementURL) {
		return input, errors.New("key management URL must be a credential-free HTTPS URL or localhost HTTP URL")
	}
	if input.Scope == "shared" && input.KeyManagementURL == "" {
		return input, errors.New("shared Authorization profiles require a key management URL")
	}
	if input.Scope == "shared" && input.AccessEvaluationURL == "" {
		return input, errors.New("Authorization profiles require an access evaluation URL")
	}
	if input.Scope == "shared" && input.UsageURL == "" {
		return input, errors.New("Authorization profiles require a usage URL")
	}
	if len(input.AccessEvaluationURL) > 2048 || input.AccessEvaluationURL != "" && !validOutboundHookURI(input.AccessEvaluationURL) {
		return input, errors.New("access evaluation URL must be a credential-free HTTPS URL or localhost HTTP URL")
	}
	if len(input.UsageURL) > 2048 || input.UsageURL != "" && !validOutboundHookURI(input.UsageURL) {
		return input, errors.New("usage URL must be a credential-free HTTPS URL or localhost HTTP URL")
	}
	if strings.TrimSpace(input.Credential) == "" {
		return input, errors.New("credential value is required")
	}
	if len(input.Credential) > 16*1024 {
		return input, errors.New("credential value must not exceed 16 KB")
	}
	if strings.ContainsAny(input.Credential, "\r\n\x00") {
		return input, errors.New("credential value contains forbidden control characters")
	}
	headers := []runtimeauth.Header(nil)
	if input.AdditionalHeaders != nil {
		headers, err = normalizeRuntimeAuthorizationHeaderInputs(*input.AdditionalHeaders, nil, false)
		if err != nil {
			return input, err
		}
	}
	input.credentialMaterial, err = runtimeauth.Encode([]byte(input.Credential), headers)
	if err != nil {
		return input, err
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

// RuntimeAuthorizationProfiles returns only deployment-shared, active
// Authorization profiles. Dedicated legacy credentials remain API-local and
// cannot be selected by the new API configurator.
func (s *Service) RuntimeAuthorizationProfiles(ctx context.Context) ([]model.RuntimeCredentialSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return nil, err
	}
	values, err := s.store.RuntimeCredentialSets(ctx, deployment.ID, "")
	if err != nil {
		return nil, err
	}
	profiles := make([]model.RuntimeCredentialSet, 0, len(values))
	for _, value := range values {
		if value.Scope == "shared" && value.OwnerIntegrationID == "" && value.State == "active" && value.CredentialPresent {
			profiles = append(profiles, value)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].EnvironmentID != profiles[j].EnvironmentID {
			return profiles[i].EnvironmentID < profiles[j].EnvironmentID
		}
		if profiles[i].Name != profiles[j].Name {
			return profiles[i].Name < profiles[j].Name
		}
		return profiles[i].ID < profiles[j].ID
	})
	return profiles, nil
}

func (s *Service) UpdateRuntimeAuthorization(ctx context.Context, authorizationID string, input RuntimeAuthorizationUpdateInput, actor Actor) (model.RuntimeCredentialSet, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	current, err := s.store.RuntimeCredentialSet(ctx, deployment.ID, strings.TrimSpace(authorizationID))
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if current.Scope != "shared" || current.OwnerIntegrationID != "" {
		return model.RuntimeCredentialSet{}, errors.New("only reusable Authorizations can be updated")
	}
	input.EnvironmentVariable = strings.ToUpper(strings.TrimSpace(input.EnvironmentVariable))
	input.HeaderName = strings.TrimSpace(input.HeaderName)
	input.KeyManagementURL = strings.TrimSpace(input.KeyManagementURL)
	input.AccessEvaluationURL = strings.TrimSpace(input.AccessEvaluationURL)
	input.UsageURL = strings.TrimSpace(input.UsageURL)
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	if !runtimeEnvironmentVariablePattern.MatchString(input.EnvironmentVariable) {
		return model.RuntimeCredentialSet{}, errors.New("environment variable must use upper-case letters, numbers, and underscores")
	}
	if input.State != "active" && input.State != "disabled" {
		return model.RuntimeCredentialSet{}, errors.New("Authorization state must be active or disabled")
	}
	if current.AuthenticationType == "api_key_header" || current.AuthenticationType == "custom_header" {
		if input.HeaderName == "" || len(input.HeaderName) > 100 || !runtimeHeaderNamePattern.MatchString(input.HeaderName) {
			return model.RuntimeCredentialSet{}, errors.New("Authorization header name is invalid")
		}
		if !runtimeauth.SafeHeaderName(input.HeaderName) {
			return model.RuntimeCredentialSet{}, errors.New("Authorization header name is not allowed")
		}
	} else {
		input.HeaderName = ""
	}
	var nextCredentialMaterial []byte
	rotateCredential := input.Credential != "" || input.AdditionalHeaders != nil
	currentHeaderNames := runtimeAuthConfigHeaderNames(current.AuthConfig)
	if rotateCredential {
		currentMaterial, materialErr := s.runtimeCredentialMaterial(ctx, current)
		if materialErr != nil {
			return model.RuntimeCredentialSet{}, materialErr
		}
		primary, currentHeaders, _, decodeErr := runtimeauth.Decode(currentMaterial)
		if decodeErr != nil {
			wipeBytes(currentMaterial)
			return model.RuntimeCredentialSet{}, decodeErr
		}
		defer wipeBytes(currentMaterial)
		defer wipeBytes(primary)
		defer wipeRuntimeAuthorizationHeaders(currentHeaders)
		existing := make(map[string][]byte, len(currentHeaders)+1)
		for _, header := range currentHeaders {
			existing[strings.ToLower(header.Name)] = bytes.Clone(header.Value)
		}
		if current.HeaderName != "" {
			existing[strings.ToLower(current.HeaderName)] = bytes.Clone(primary)
		}
		defer func() {
			for _, value := range existing {
				wipeBytes(value)
			}
		}()
		if input.Credential != "" {
			if len(input.Credential) > runtimeauth.MaxCredentialBytes || strings.ContainsAny(input.Credential, "\r\n\x00") {
				return model.RuntimeCredentialSet{}, errors.New("credential value is invalid")
			}
			wipeBytes(primary)
			primary = []byte(input.Credential)
		} else if input.AdditionalHeaders != nil && current.HeaderName != "" && !strings.EqualFold(input.HeaderName, current.HeaderName) {
			promoted := existing[strings.ToLower(input.HeaderName)]
			if len(promoted) == 0 {
				return model.RuntimeCredentialSet{}, errors.New("enter a value for the new primary Authorization header")
			}
			wipeBytes(primary)
			primary = bytes.Clone(promoted)
		}
		nextHeaders := currentHeaders
		if input.AdditionalHeaders != nil {
			nextHeaders, err = normalizeRuntimeAuthorizationHeaderInputs(*input.AdditionalHeaders, existing, true)
			if err != nil {
				return model.RuntimeCredentialSet{}, err
			}
			defer wipeRuntimeAuthorizationHeaders(nextHeaders)
		}
		for _, header := range nextHeaders {
			if input.HeaderName != "" && strings.EqualFold(input.HeaderName, header.Name) {
				return model.RuntimeCredentialSet{}, errors.New("the primary and additional Authorization header names must be unique")
			}
		}
		currentHeaderNames = make([]string, len(nextHeaders))
		for index, header := range nextHeaders {
			currentHeaderNames[index] = header.Name
		}
		nextCredentialMaterial, err = runtimeauth.Encode(primary, nextHeaders)
		if err != nil {
			return model.RuntimeCredentialSet{}, err
		}
		defer wipeBytes(nextCredentialMaterial)
		rotateCredential = !bytes.Equal(nextCredentialMaterial, currentMaterial)
	}
	input.AuthConfig, err = withRuntimeAuthConfigHeaderNames(input.AuthConfig, currentHeaderNames)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	input.AuthConfig, err = normalizeRuntimeAuthConfig(input.AuthConfig, current.AuthenticationType)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if _, _, err = runtimeTargetAuth(current.AuthenticationType, input.AuthConfig, input.HeaderName); err != nil {
		return model.RuntimeCredentialSet{}, fmt.Errorf("Authorization: %w", err)
	}
	if input.KeyManagementURL == "" || len(input.KeyManagementURL) > 2048 || !validHTTPSURI(input.KeyManagementURL) {
		return model.RuntimeCredentialSet{}, errors.New("key management URL must be a credential-free HTTPS URL or localhost HTTP URL")
	}
	for label, value := range map[string]string{"access evaluation URL": input.AccessEvaluationURL, "usage URL": input.UsageURL} {
		if value == "" || len(value) > 2048 || !validOutboundHookURI(value) {
			return model.RuntimeCredentialSet{}, fmt.Errorf("%s must be a credential-free HTTPS URL or localhost HTTP URL", label)
		}
	}
	if input.Revision < 1 || input.Revision != current.Revision {
		return model.RuntimeCredentialSet{}, store.ErrConflict
	}
	previous := current
	current.EnvironmentVariable = input.EnvironmentVariable
	current.HeaderName = input.HeaderName
	current.AuthConfig = input.AuthConfig
	current.KeyManagementURL = input.KeyManagementURL
	current.AccessEvaluationURL = input.AccessEvaluationURL
	current.UsageURL = input.UsageURL
	current.State = input.State
	updated, err := s.store.UpdateRuntimeCredentialSet(ctx, current, input.Revision)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	if rotateCredential {
		updated, err = s.rotateRuntimeCredentialMaterial(ctx, deployment, updated, nextCredentialMaterial, nil, actor)
		if err != nil {
			// Header names are non-secret metadata. If activating their matching
			// encrypted values fails, runtime name/value verification denies
			// execution instead of sending a partial Authorization.
			return model.RuntimeCredentialSet{}, err
		}
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "authorization.updated", TargetType: "authorization", TargetID: updated.ID, Prior: map[string]any{"environment_variable": previous.EnvironmentVariable, "key_management_url": previous.KeyManagementURL, "access_evaluation_url": previous.AccessEvaluationURL, "usage_url": previous.UsageURL, "state": previous.State}, Current: map[string]any{"environment_variable": updated.EnvironmentVariable, "key_management_url": updated.KeyManagementURL, "access_evaluation_url": updated.AccessEvaluationURL, "usage_url": updated.UsageURL, "state": updated.State}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	return updated, nil
}

func (s *Service) storeRuntimeCredential(ctx context.Context, organisationID, credentialSetID string, credential []byte) (model.Secret, string, error) {
	if s.vault == nil {
		return model.Secret{}, "", errors.New("runtime credential encryption is not configured")
	}
	secretID, err := randomUUID()
	if err != nil {
		return model.Secret{}, "", err
	}
	encrypted, err := s.vault.Encrypt(credential, organisationID+":runtime_credential:"+secretID)
	if err != nil {
		return model.Secret{}, "", err
	}
	secret, err := s.store.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: organisationID, Name: "runtime-credential-" + credentialSetID + "-" + secretID, Purpose: "runtime_service_credential", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	return secret, secretID, err
}

func wipeBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func wipeRuntimeAuthorizationHeaders(values []runtimeauth.Header) {
	for index := range values {
		wipeBytes(values[index].Value)
	}
}

func (s *Service) runtimeCredentialMaterial(ctx context.Context, credentialSet model.RuntimeCredentialSet) ([]byte, error) {
	var active model.RuntimeCredentialVersion
	for _, version := range credentialSet.Versions {
		if version.State == "active" && (version.ExpiresAt == nil || version.ExpiresAt.After(s.now())) {
			active = version
			break
		}
	}
	if active.ID == "" || active.SecretID == "" {
		return nil, errors.New("active Authorization credential is unavailable")
	}
	secret, err := s.store.Secret(ctx, credentialSet.OrganisationID, active.SecretID)
	if err != nil || secret.Purpose != "runtime_service_credential" {
		return nil, errors.New("active Authorization credential is unavailable")
	}
	return decryptRuntimeCredential(s.vault, secret)
}

func (s *Service) rotateRuntimeCredentialMaterial(ctx context.Context, deployment model.Deployment, credentialSet model.RuntimeCredentialSet, credential []byte, expiresAt *time.Time, actor Actor) (model.RuntimeCredentialSet, error) {
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
	secret, secretID, err := s.storeRuntimeCredential(ctx, deployment.OrganisationID, setID, input.credentialMaterial)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	created, err := s.store.CreateRuntimeCredentialSet(ctx, model.RuntimeCredentialSet{ID: setID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, EnvironmentID: input.EnvironmentID, Scope: input.Scope, OwnerIntegrationID: input.OwnerIntegrationID, Name: input.Name, EnvironmentVariable: input.EnvironmentVariable, AuthenticationType: input.AuthenticationType, HeaderName: input.HeaderName, AuthConfig: input.AuthConfig, KeyManagementURL: input.KeyManagementURL, AccessEvaluationURL: input.AccessEvaluationURL, UsageURL: input.UsageURL, State: "active"})
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
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, ActorID: actor.ID, Action: "authorization.created", TargetType: "authorization", TargetID: result.ID, Current: map[string]any{"environment_id": result.EnvironmentID, "environment_variable": result.EnvironmentVariable, "authentication_type": result.AuthenticationType, "key_management_url": result.KeyManagementURL, "access_evaluation_url": result.AccessEvaluationURL, "usage_url": result.UsageURL, "fingerprint": result.ActiveFingerprint}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
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
	if strings.ContainsAny(credential, "\r\n\x00") {
		return model.RuntimeCredentialSet{}, errors.New("credential value contains forbidden control characters")
	}
	currentMaterial, err := s.runtimeCredentialMaterial(ctx, credentialSet)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	primary, headers, _, err := runtimeauth.Decode(currentMaterial)
	wipeBytes(currentMaterial)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	wipeBytes(primary)
	defer wipeRuntimeAuthorizationHeaders(headers)
	nextMaterial, err := runtimeauth.Encode([]byte(credential), headers)
	if err != nil {
		return model.RuntimeCredentialSet{}, err
	}
	defer wipeBytes(nextMaterial)
	return s.rotateRuntimeCredentialMaterial(ctx, deployment, credentialSet, nextMaterial, expiresAt, actor)
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
	if !runtimeAuthenticationNeedsCredential(input.AuthenticationType) && len(runtimeAuthConfigHeaderNames(input.AuthConfig)) > 0 {
		return input, errors.New("additional Authorization headers require a secret-bearing authentication type")
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
	credentialSetID := strings.TrimSpace(input.AuthorizationID)
	authenticationType := ""
	if credentialSetID != "" {
		profile, profileErr := s.store.RuntimeCredentialSet(ctx, deployment.ID, credentialSetID)
		if profileErr != nil {
			return model.RuntimeSetup{}, profileErr
		}
		if profile.Scope != "shared" || profile.OwnerIntegrationID != "" || profile.EnvironmentID != strings.TrimSpace(input.EnvironmentID) || profile.State != "active" || !profile.CredentialPresent {
			return model.RuntimeSetup{}, errors.New("selected authorization profile is not active or reusable in this environment")
		}
		authenticationType = profile.AuthenticationType
		input.AuthConfig = profile.AuthConfig
	} else {
		authenticationType, err = normalizeRuntimeAuthenticationType(input.AuthenticationType)
		if err != nil {
			return model.RuntimeSetup{}, err
		}
	}
	if runtimeAuthenticationNeedsCredential(authenticationType) && credentialSetID == "" {
		created, createErr := s.CreateRuntimeCredentialSet(ctx, integration.ID, RuntimeCredentialSetInput{EnvironmentID: input.EnvironmentID, Scope: "shared", Name: integration.DisplayName, EnvironmentVariable: input.EnvironmentVariable, AuthenticationType: authenticationType, HeaderName: input.HeaderName, AuthConfig: input.AuthConfig, KeyManagementURL: input.KeyManagementURL, AccessEvaluationURL: input.AccessEvaluationURL, UsageURL: input.UsageURL, Credential: input.Credential, AdditionalHeaders: input.AdditionalHeaders, ExpiresAt: input.CredentialExpiresAt}, actor)
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
