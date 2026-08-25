package access

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
)

var (
	ErrDenied            = errors.New("access operation denied")
	ErrInvalidRequest    = errors.New("invalid access operation request")
	ErrUnsupported       = errors.New("access operation is not supported")
	ErrUnsafeDestination = errors.New("access destination is unsafe")
)

type Store interface {
	AccessDefinitions(context.Context, string) ([]model.AccessDefinition, error)
	AccessDefinition(context.Context, string, string) (model.AccessDefinition, error)
	AccessConnections(context.Context, string) ([]model.AccessConnection, error)
	AccessConnection(context.Context, string, string) (model.AccessConnection, error)
	AccessInstances(context.Context, string, string) ([]model.AccessInstance, error)
	AccessInstance(context.Context, string, string) (model.AccessInstance, error)
	CreateAccessInstance(context.Context, model.AccessInstance) (model.AccessInstance, error)
	AccessCredentials(context.Context, string, string, string) ([]model.AccessCredential, error)
	AccessCredential(context.Context, string, string) (model.AccessCredential, error)
	CreateAccessCredential(context.Context, model.AccessCredential) (model.AccessCredential, error)
	RevokeAccessCredential(context.Context, string, string, time.Time) (model.AccessCredential, error)
	CreateSecret(context.Context, model.Secret) (model.Secret, error)
	Secret(context.Context, string, string) (model.Secret, error)
	AppendAudit(context.Context, model.AuditEvent) error
}

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type tokenCacheEntry struct {
	Token     string
	ExpiresAt time.Time
}

type Runtime struct {
	store                        Store
	vault                        *secrets.Vault
	resolver                     Resolver
	doer                         Doer
	now                          func() time.Time
	tokenMu                      sync.Mutex
	tokens                       map[string]tokenCacheEntry
	privateLocalhostDestinations map[string]struct{}
}

type Principal struct {
	Subject            string
	ExternalCustomerID string
	InstallationID     string
	Grants             map[string]bool
	RequestID          string
}

type InstanceRequest struct {
	IntegrationID  string `json:"integration_id"`
	EnvironmentID  string `json:"environment_id"`
	DisplayName    string `json:"display_name"`
	IdempotencyKey string `json:"idempotency_key"`
	TTLSeconds     int    `json:"ttl_seconds,omitempty"`
}

type CredentialRequest struct {
	IntegrationID           string   `json:"integration_id"`
	EnvironmentID           string   `json:"environment_id"`
	AccessInstanceID        string   `json:"access_instance_id,omitempty"`
	RotatedFromCredentialID string   `json:"rotated_from_credential_id,omitempty"`
	Scopes                  []string `json:"scopes"`
	IdempotencyKey          string   `json:"idempotency_key"`
	TTLSeconds              int      `json:"ttl_seconds,omitempty"`
}

type CredentialResult struct {
	Credential          model.AccessCredential `json:"credential"`
	CredentialMaterial  json.RawMessage        `json:"credential_material,omitempty"`
	Existing            bool                   `json:"existing"`
	EnvironmentVariable string                 `json:"environment_variable,omitempty"`
}

type Capability struct {
	ConnectionID        string   `json:"connection_id"`
	DefinitionID        string   `json:"definition_id"`
	ServiceKey          string   `json:"service_key"`
	Name                string   `json:"name"`
	InstanceCardinality string   `json:"instance_cardinality"`
	InstanceLabel       string   `json:"instance_label"`
	IntegrationIDs      []string `json:"integration_ids"`
	CanCreateInstance   bool     `json:"can_create_instance"`
	CanCreateCredential bool     `json:"can_create_credential"`
	CanRevokeCredential bool     `json:"can_revoke_credential"`
}

type operation struct {
	Method                         string `json:"method"`
	Path                           string `json:"path"`
	AcceptsRotatedFromCredentialID bool   `json:"accepts_rotated_from_credential_id,omitempty"`
}

type definitionConfig struct {
	Operations            map[string]operation
	RequiredGrants        []string
	MaxTTLSeconds         int
	CredentialStorageMode string
}

type connectionConfig struct {
	TokenURL     string   `json:"token_url"`
	Scopes       []string `json:"scopes"`
	Audience     string   `json:"audience"`
	APIKeyHeader string   `json:"api_key_header"`
}

func New(store Store, vault *secrets.Vault, resolver Resolver, doer Doer) *Runtime {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Runtime{store: store, vault: vault, resolver: resolver, doer: doer, now: func() time.Time { return time.Now().UTC() }, tokens: make(map[string]tokenCacheEntry)}
}

// SetPrivateLocalhostHosts configures exact HTTP development destinations.
// It never relaxes public-host SSRF checks and a host-only entry grants only
// the default HTTP port.
func (r *Runtime) SetPrivateLocalhostHosts(destinations []string) {
	r.privateLocalhostDestinations = make(map[string]struct{}, len(destinations))
	for _, destination := range destinations {
		hostname, port := localDevelopmentDestination(destination)
		if hostname != "" {
			r.privateLocalhostDestinations[net.JoinHostPort(hostname, port)] = struct{}{}
		}
	}
}

func localDevelopmentDestination(raw string) (string, string) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@") {
		return "", ""
	}
	hostname, port, err := net.SplitHostPort(raw)
	if err != nil {
		hostname, port = raw, "80"
	}
	hostname = strings.TrimSuffix(strings.Trim(hostname, "[]"), ".")
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 || !identity.IsLocalDevelopmentHostname(hostname) {
		return "", ""
	}
	return hostname, strconv.Itoa(parsedPort)
}

func (r *Runtime) privateLocalDevelopmentURL(destination *url.URL) bool {
	if destination.Scheme != "http" || !identity.IsLocalDevelopmentHostname(destination.Hostname()) {
		return false
	}
	port := destination.Port()
	if port == "" {
		port = "80"
	}
	_, ok := r.privateLocalhostDestinations[net.JoinHostPort(strings.ToLower(destination.Hostname()), port)]
	return ok
}

func randomUUID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func parseDefinition(value model.AccessDefinition) (definitionConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(value.Operations, &raw); err != nil {
		return definitionConfig{}, err
	}
	result := definitionConfig{Operations: make(map[string]operation), MaxTTLSeconds: 86400, CredentialStorageMode: "one_time"}
	for key, encoded := range raw {
		switch key {
		case "required_grants":
			_ = json.Unmarshal(encoded, &result.RequiredGrants)
		case "max_ttl_seconds":
			_ = json.Unmarshal(encoded, &result.MaxTTLSeconds)
		case "credential_storage_mode":
			_ = json.Unmarshal(encoded, &result.CredentialStorageMode)
		default:
			var candidate operation
			if json.Unmarshal(encoded, &candidate) == nil && candidate.Path != "" {
				candidate.Method = strings.ToUpper(strings.TrimSpace(candidate.Method))
				if candidate.Method == "" {
					candidate.Method = http.MethodPost
				}
				result.Operations[key] = candidate
			}
		}
	}
	if result.MaxTTLSeconds < 0 || result.MaxTTLSeconds > 31536000 {
		return definitionConfig{}, ErrInvalidRequest
	}
	if result.CredentialStorageMode != "one_time" && result.CredentialStorageMode != "managed" && result.CredentialStorageMode != "reference" {
		return definitionConfig{}, ErrInvalidRequest
	}
	return result, nil
}

func allowed(cfg definitionConfig, principal Principal) bool {
	if principal.Subject == "" {
		return false
	}
	for _, grant := range cfg.RequiredGrants {
		if !principal.Grants[grant] {
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (r *Runtime) Capabilities(ctx context.Context, deploymentID string, grants map[string]bool) []Capability {
	connections, err := r.store.AccessConnections(ctx, deploymentID)
	if err != nil {
		return nil
	}
	result := make([]Capability, 0, len(connections))
	for _, connection := range connections {
		if connection.State != "active" || connection.Definition == nil || connection.Definition.State != "active" {
			continue
		}
		cfg, err := parseDefinition(*connection.Definition)
		if err != nil || !allowed(cfg, Principal{Subject: "discovery", Grants: grants}) {
			continue
		}
		_, createInstance := cfg.Operations["instances.create"]
		_, createCredential := cfg.Operations["credentials.create"]
		_, revokeCredential := cfg.Operations["credentials.revoke"]
		result = append(result, Capability{ConnectionID: connection.ID, DefinitionID: connection.Definition.ID, ServiceKey: connection.Definition.ServiceKey, Name: connection.Name, InstanceCardinality: connection.Definition.InstanceCardinality, InstanceLabel: connection.Definition.InstanceLabelSingular, IntegrationIDs: connection.IntegrationIDs, CanCreateInstance: createInstance && connection.Definition.InstanceCardinality == "many", CanCreateCredential: createCredential, CanRevokeCredential: revokeCredential})
	}
	return result
}

func validOperationPath(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(parsed.Path, "/") || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	cleaned := path.Clean(parsed.Path)
	return cleaned == parsed.Path && !strings.Contains(parsed.Path, "..")
}

func (r *Runtime) clientForURL(ctx context.Context, destination *url.URL) (Doer, error) {
	localDevelopment := r.privateLocalDevelopmentURL(destination)
	if (destination.Scheme != "https" && !localDevelopment) || destination.Hostname() == "" || destination.User != nil || (destination.Scheme == "https" && destination.Port() != "") {
		return nil, ErrUnsafeDestination
	}
	addresses, err := r.resolver.LookupIP(ctx, "ip", destination.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, ErrUnsafeDestination
	}
	for _, address := range addresses {
		if netpolicy.UnsafeIP(address) && !localDevelopment {
			return nil, ErrUnsafeDestination
		}
	}
	if r.doer != nil {
		return r.doer, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	port := destination.Port()
	if port == "" {
		port = "443"
		if destination.Scheme == "http" {
			port = "80"
		}
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: destination.Hostname()}, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func (r *Runtime) destination(ctx context.Context, connection model.AccessConnection, operationPath string) (*url.URL, Doer, error) {
	base, err := url.Parse(connection.BaseURL)
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !validOperationPath(operationPath) {
		return nil, nil, ErrUnsafeDestination
	}
	base.Path = strings.TrimRight(base.Path, "/") + operationPath
	client, err := r.clientForURL(ctx, base)
	return base, client, err
}

func (r *Runtime) managementSecret(ctx context.Context, connection model.AccessConnection) ([]byte, error) {
	if r.vault == nil || connection.ManagementSecretID == "" {
		return nil, errors.New("access management credential unavailable")
	}
	value, err := r.store.Secret(ctx, connection.OrganisationID, connection.ManagementSecretID)
	if err != nil {
		return nil, err
	}
	encrypted := secrets.Encrypted{Ciphertext: value.Ciphertext, Nonce: value.Nonce, KeyVersion: value.KeyVersion, Fingerprint: value.Fingerprint}
	plaintext, err := r.vault.Decrypt(encrypted, connection.OrganisationID+":access:"+connection.ManagementSecretID)
	if err != nil && connection.LegacyProviderID != "" {
		plaintext, err = r.vault.Decrypt(encrypted, connection.OrganisationID+":provider:"+connection.ManagementSecretID)
	}
	return plaintext, err
}

func (r *Runtime) oauthToken(ctx context.Context, connection model.AccessConnection) (string, error) {
	r.tokenMu.Lock()
	if cached := r.tokens[connection.ID]; cached.Token != "" && cached.ExpiresAt.After(r.now().Add(30*time.Second)) {
		r.tokenMu.Unlock()
		return cached.Token, nil
	}
	r.tokenMu.Unlock()
	var cfg connectionConfig
	if err := json.Unmarshal(connection.Config, &cfg); err != nil || cfg.TokenURL == "" {
		return "", errors.New("OAuth management token endpoint is not configured")
	}
	tokenURL, err := url.Parse(cfg.TokenURL)
	if err != nil {
		return "", ErrUnsafeDestination
	}
	client, err := r.clientForURL(ctx, tokenURL)
	if err != nil {
		return "", err
	}
	credential, err := r.managementSecret(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() { clear(credential) }()
	var pair struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if json.Unmarshal(credential, &pair) != nil || pair.ClientID == "" || pair.ClientSecret == "" {
		return "", errors.New("OAuth management credential must contain client_id and client_secret")
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	if cfg.Audience != "" {
		form.Set("audience", cfg.Audience)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(pair.ClientID, pair.ClientSecret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("management OAuth endpoint returned %s", response.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil || result.AccessToken == "" {
		return "", errors.New("management OAuth endpoint returned an invalid token response")
	}
	if result.ExpiresIn <= 0 || result.ExpiresIn > 86400 {
		result.ExpiresIn = 3600
	}
	r.tokenMu.Lock()
	r.tokens[connection.ID] = tokenCacheEntry{Token: result.AccessToken, ExpiresAt: r.now().Add(time.Duration(result.ExpiresIn) * time.Second)}
	r.tokenMu.Unlock()
	return result.AccessToken, nil
}

func (r *Runtime) authenticate(ctx context.Context, request *http.Request, connection model.AccessConnection, definition model.AccessDefinition) error {
	switch definition.ManagementAuthType {
	case "none":
		return nil
	case "bearer":
		credential, err := r.managementSecret(ctx, connection)
		if err != nil {
			return err
		}
		defer clear(credential)
		request.Header.Set("Authorization", "Bearer "+string(credential))
		return nil
	case "api_key":
		credential, err := r.managementSecret(ctx, connection)
		if err != nil {
			return err
		}
		defer clear(credential)
		var cfg connectionConfig
		_ = json.Unmarshal(connection.Config, &cfg)
		if cfg.APIKeyHeader == "" {
			cfg.APIKeyHeader = "X-API-Key"
		}
		if strings.EqualFold(cfg.APIKeyHeader, "Authorization") || !httpgutsValidHeaderName(cfg.APIKeyHeader) {
			return ErrInvalidRequest
		}
		request.Header.Set(cfg.APIKeyHeader, string(credential))
		return nil
	case "oauth2_client_credentials":
		token, err := r.oauthToken(ctx, connection)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	default:
		return ErrUnsupported
	}
}

func httpgutsValidHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", character)) {
			return false
		}
	}
	return true
}

func (r *Runtime) call(ctx context.Context, connection model.AccessConnection, definition model.AccessDefinition, operation operation, input, output any) error {
	destination, client, err := r.destination(ctx, connection, operation.Path)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, destination.String(), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	if err := r.authenticate(ctx, request, connection, definition); err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("access provider returned %s", response.Status)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

func (r *Runtime) authorize(ctx context.Context, connection model.AccessConnection, definition model.AccessDefinition, cfg definitionConfig, principal Principal, operationName string, details map[string]any) error {
	if !allowed(cfg, principal) {
		return ErrDenied
	}
	authorize, configured := cfg.Operations["authorize"]
	if !configured {
		return nil
	}
	var response struct {
		Allowed bool `json:"allowed"`
	}
	err := r.call(ctx, connection, definition, authorize, map[string]any{"operation": operationName, "subject": principal.Subject, "external_customer_id": principal.ExternalCustomerID, "installation_id": principal.InstallationID, "deployment_id": connection.DeploymentID, "details": details}, &response)
	if errors.Is(err, ErrUnsafeDestination) {
		return err
	}
	if err != nil || !response.Allowed {
		return ErrDenied
	}
	return nil
}

func validTTL(value, maximum int) bool {
	if value == 0 {
		return true
	}
	return value >= 300 && (maximum == 0 || value <= maximum)
}

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
