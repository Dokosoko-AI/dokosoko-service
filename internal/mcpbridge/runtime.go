package mcpbridge

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var (
	ErrInvalidConnection = errors.New("invalid Stateless MCPv2 connection")
	ErrUnsupportedAuth   = errors.New("unsupported Stateless MCPv2 authorization mode")
	ErrGrantRequired     = errors.New("upstream user authorization is required")
	ErrUpstreamProtocol  = errors.New("upstream did not satisfy Stateless MCPv2")
	ErrUnsafeDestination = errors.New("upstream MCP destination is unsafe")
)

const maxMCPBody = 1 << 20

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
var upstreamToolPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,96}$`)

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Manager struct {
	store    store.Store
	vault    *secrets.Vault
	resolver Resolver
	doer     Doer
	baseURL  string
	now      func() time.Time
}

type ConnectionInput struct {
	OrganisationID    string
	ProductID         string
	Name              string
	Namespace         string
	Endpoint          string
	AuthMode          string
	Credential        string
	OAuthClientID     string
	OAuthClientSecret string
	OAuthIssuer       string
	AuthorizationURL  string
	TokenURL          string
	Scopes            []string
}

type Actor struct {
	ID        string
	RequestID string
}

type CatalogTool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	Annotations  json.RawMessage `json:"annotations,omitempty"`
	SchemaHash   string          `json:"schema_hash"`
}

type Catalog struct {
	Connection  model.MCPConnection `json:"connection"`
	Tools       []CatalogTool       `json:"tools"`
	CatalogHash string              `json:"catalog_hash"`
	TTLMS       int64               `json:"ttl_ms,omitempty"`
}

type ImportInput struct {
	ToolNames            []string
	RequiredGrants       []string
	ConfirmationRequired bool
	TimeoutMS            int
}

type ImportResult struct {
	Connection model.MCPConnection `json:"connection"`
	Created    []model.Tool        `json:"created"`
	Updated    []model.Tool        `json:"updated"`
	Unchanged  []model.Tool        `json:"unchanged"`
	Drifted    []model.Tool        `json:"drifted"`
	Rejected   map[string]string   `json:"rejected"`
}

func New(storage store.Store, vault *secrets.Vault, baseURL string, resolver Resolver, doer Doer) *Manager {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Manager{store: storage, vault: vault, resolver: resolver, doer: doer, baseURL: strings.TrimRight(baseURL, "/"), now: func() time.Time { return time.Now().UTC() }}
}

func randomToken(bytesCount int) (string, error) {
	raw := make([]byte, bytesCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}

func auditID() string {
	raw := make([]byte, 12)
	_, _ = rand.Read(raw)
	return "audit_" + hex.EncodeToString(raw)
}

func fixedHTTPS(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" && parsed.RawQuery == "" && (parsed.Port() == "" || parsed.Port() == "443")
}

func normalizeScopes(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && len(value) <= 120 && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (m *Manager) saveSecret(ctx context.Context, organisationID, name, purpose, plaintext, aad string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", nil
	}
	if m.vault == nil {
		return "", errors.New("credential encryption is not configured")
	}
	id, err := randomUUID()
	if err != nil {
		return "", err
	}
	encrypted, err := m.vault.Encrypt([]byte(plaintext), aad+id)
	if err != nil {
		return "", err
	}
	_, err = m.store.CreateSecret(ctx, model.Secret{ID: id, OrganisationID: organisationID, Name: name, Purpose: purpose, Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	return id, err
}

func (m *Manager) decryptSecret(ctx context.Context, organisationID, id, aad string) (string, error) {
	if id == "" {
		return "", nil
	}
	if m.vault == nil {
		return "", errors.New("credential encryption is not configured")
	}
	value, err := m.store.Secret(ctx, organisationID, id)
	if err != nil {
		return "", err
	}
	plaintext, err := m.vault.Decrypt(secrets.Encrypted{Ciphertext: value.Ciphertext, Nonce: value.Nonce, KeyVersion: value.KeyVersion, Fingerprint: value.Fingerprint}, aad+id)
	return string(plaintext), err
}

func (m *Manager) CreateConnection(ctx context.Context, input ConnectionInput, actor Actor) (model.MCPConnection, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Namespace = strings.TrimSpace(input.Namespace)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.AuthMode = strings.TrimSpace(input.AuthMode)
	input.AuthorizationURL = strings.TrimSpace(input.AuthorizationURL)
	input.TokenURL = strings.TrimSpace(input.TokenURL)
	input.OAuthIssuer = strings.TrimSpace(input.OAuthIssuer)
	input.OAuthClientID = strings.TrimSpace(input.OAuthClientID)
	if input.OrganisationID == "" || input.ProductID == "" || input.Name == "" || len(input.Name) > 120 || !namespacePattern.MatchString(input.Namespace) || !fixedHTTPS(input.Endpoint) {
		return model.MCPConnection{}, ErrInvalidConnection
	}
	switch input.AuthMode {
	case "none":
		if input.Credential != "" || input.OAuthClientID != "" || input.OAuthClientSecret != "" {
			return model.MCPConnection{}, ErrInvalidConnection
		}
	case "service":
		if strings.TrimSpace(input.Credential) == "" {
			return model.MCPConnection{}, ErrInvalidConnection
		}
	case "delegated_oauth":
		if input.OAuthClientID == "" || input.OAuthClientSecret == "" || !fixedHTTPS(input.OAuthIssuer) || !fixedHTTPS(input.AuthorizationURL) || !fixedHTTPS(input.TokenURL) {
			return model.MCPConnection{}, ErrInvalidConnection
		}
	default:
		return model.MCPConnection{}, ErrUnsupportedAuth
	}
	id, err := randomUUID()
	if err != nil {
		return model.MCPConnection{}, err
	}
	credentialID, err := m.saveSecret(ctx, input.OrganisationID, "mcp-service-"+input.Namespace, "mcp_upstream_service", input.Credential, input.OrganisationID+":mcp-connection:"+id+":service:")
	if err != nil {
		return model.MCPConnection{}, err
	}
	clientSecretID, err := m.saveSecret(ctx, input.OrganisationID, "mcp-oauth-client-"+input.Namespace, "mcp_upstream_oauth_client", input.OAuthClientSecret, input.OrganisationID+":mcp-connection:"+id+":oauth-client:")
	if err != nil {
		return model.MCPConnection{}, err
	}
	value, err := m.store.CreateMCPConnection(ctx, model.MCPConnection{ID: id, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Name: input.Name, Namespace: input.Namespace, Endpoint: input.Endpoint, ProtocolVersion: model.StatelessMCPv2Protocol, AuthMode: input.AuthMode, CredentialID: credentialID, OAuthClientID: input.OAuthClientID, OAuthClientSecretID: clientSecretID, OAuthIssuer: input.OAuthIssuer, AuthorizationURL: input.AuthorizationURL, TokenURL: input.TokenURL, Scopes: normalizeScopes(input.Scopes), Config: json.RawMessage(`{"transport":"streamable_http","live_subscription":false}`)})
	if err != nil {
		return model.MCPConnection{}, err
	}
	if err := m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: value.OrganisationID, ProductID: value.ProductID, ActorID: actor.ID, Action: "mcp.connection.created", TargetType: "mcp_connection", TargetID: value.ID, Current: map[string]any{"name": value.Name, "namespace": value.Namespace, "endpoint_host": mustHost(value.Endpoint), "protocol_version": model.StatelessMCPv2Protocol, "auth_mode": value.AuthMode}, RequestID: actor.RequestID, CreatedAt: m.now()}); err != nil {
		return model.MCPConnection{}, err
	}
	return value, nil
}

func mustHost(raw string) string {
	parsed, _ := url.Parse(raw)
	return parsed.Hostname()
}

func (m *Manager) safeDestination(ctx context.Context, raw string) (*url.URL, net.IP, error) {
	if !fixedHTTPS(raw) {
		return nil, nil, ErrUnsafeDestination
	}
	parsed, _ := url.Parse(raw)
	addresses, err := m.resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, nil, ErrUnsafeDestination
	}
	for _, address := range addresses {
		if netpolicy.UnsafeIP(address) {
			return nil, nil, ErrUnsafeDestination
		}
	}
	return parsed, addresses[0], nil
}

func (m *Manager) client(parsed *url.URL, address net.IP, timeout time.Duration) Doer {
	if m.doer != nil {
		return m.doer
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), "443"))
	}, DisableCompression: true, ResponseHeaderTimeout: timeout}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
