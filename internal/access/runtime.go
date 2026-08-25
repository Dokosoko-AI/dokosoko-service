package access

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
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
