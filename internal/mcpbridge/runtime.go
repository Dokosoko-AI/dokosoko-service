package mcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko/v2/internal/model"
	"github.com/dokosoko/dokosoko/v2/internal/secrets"
	"github.com/dokosoko/dokosoko/v2/internal/store"
	toolruntime "github.com/dokosoko/dokosoko/v2/internal/tools"
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
	RequiredEntitlements []string
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
	_ = m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: value.OrganisationID, ProductID: value.ProductID, ActorID: actor.ID, Action: "mcp.connection.created", TargetType: "mcp_connection", TargetID: value.ID, Current: map[string]any{"name": value.Name, "namespace": value.Namespace, "endpoint_host": mustHost(value.Endpoint), "protocol_version": model.StatelessMCPv2Protocol, "auth_mode": value.AuthMode}, RequestID: actor.RequestID, CreatedAt: m.now()})
	return value, nil
}

func mustHost(raw string) string {
	parsed, _ := url.Parse(raw)
	return parsed.Hostname()
}

func unsafeIP(address net.IP) bool {
	if address == nil || address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return true
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001:db8::/32", "fc00::/7", "fe80::/10"} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(address) {
			return true
		}
	}
	return false
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
		if unsafeIP(address) {
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

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func decodeRPCJSON(reader io.Reader) (rpcResponse, error) {
	var response rpcResponse
	encoded, err := io.ReadAll(io.LimitReader(reader, maxMCPBody+1))
	if err != nil || len(encoded) > maxMCPBody {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		return rpcResponse{}, err
	}
	if response.JSONRPC != "2.0" || (len(response.Result) == 0 && response.Error == nil) {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	return response, nil
}

func decodeRPCSSE(reader io.Reader) (rpcResponse, error) {
	limited := &io.LimitedReader{R: reader, N: maxMCPBody + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), maxMCPBody)
	data := make([]string, 0)
	var final rpcResponse
	found := false
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		encoded := strings.Join(data, "\n")
		data = data[:0]
		var candidate rpcResponse
		if err := json.Unmarshal([]byte(encoded), &candidate); err != nil {
			return err
		}
		if candidate.JSONRPC == "2.0" && (len(candidate.Result) > 0 || candidate.Error != nil) {
			final, found = candidate, true
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consume(); err != nil {
				return rpcResponse{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, err
	}
	if limited.N == 0 {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	if err := consume(); err != nil {
		return rpcResponse{}, err
	}
	if !found {
		return rpcResponse{}, ErrUpstreamProtocol
	}
	return final, nil
}

func (m *Manager) invoke(ctx context.Context, connection model.MCPConnection, method, name string, params map[string]any, bearer string, timeout time.Duration) (json.RawMessage, error) {
	parsed, address, err := m.safeDestination(ctx, connection.Endpoint)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    model.StatelessMCPv2Protocol,
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "DokoSoko", "version": "2.0.0"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	if params == nil {
		params = make(map[string]any)
	}
	params["_meta"] = meta
	id, _ := randomToken(12)
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", model.StatelessMCPv2Protocol)
	request.Header.Set("Mcp-Method", method)
	if name != "" {
		request.Header.Set("Mcp-Name", name)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := m.client(parsed, address, timeout).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream MCP returned %s", response.Status)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	var rpc rpcResponse
	if strings.Contains(contentType, "text/event-stream") {
		rpc, err = decodeRPCSSE(response.Body)
	} else if strings.Contains(contentType, "application/json") {
		rpc, err = decodeRPCJSON(response.Body)
	} else {
		return nil, ErrUpstreamProtocol
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamProtocol, err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("upstream MCP error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if responseID, ok := rpc.ID.(string); !ok || responseID != id {
		return nil, ErrUpstreamProtocol
	}
	var resultEnvelope struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(rpc.Result, &resultEnvelope) != nil {
		return nil, ErrUpstreamProtocol
	}
	var serverInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(resultEnvelope.Meta["io.modelcontextprotocol/serverInfo"], &serverInfo) != nil || serverInfo.Name == "" || serverInfo.Version == "" {
		return nil, ErrUpstreamProtocol
	}
	return rpc.Result, nil
}

func (m *Manager) connectionBearer(ctx context.Context, connection model.MCPConnection) (string, error) {
	return m.decryptSecret(ctx, connection.OrganisationID, connection.CredentialID, connection.OrganisationID+":mcp-connection:"+connection.ID+":service:")
}

func normalizeInputSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return nil, errors.New("input schema is missing or too large")
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("input schema is invalid JSON")
	}
	if schema["type"] != "object" {
		return nil, errors.New("input schema root must be an object")
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	if _, ok := schema["additionalProperties"]; !ok {
		schema["additionalProperties"] = false
	}
	encoded, _ := json.Marshal(schema)
	if err := toolruntime.ValidateSchema(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func normalizeOutputSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if len(raw) > 64<<10 {
		return nil, errors.New("output schema is too large")
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("output schema is invalid JSON")
	}
	if schema["type"] != "object" {
		return nil, errors.New("output schema root must be an object")
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	if _, ok := schema["additionalProperties"]; !ok {
		schema["additionalProperties"] = false
	}
	encoded, _ := json.Marshal(schema)
	if err := toolruntime.ValidateSchema(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func catalogToolHash(value CatalogTool) string {
	encoded, _ := json.Marshal(struct {
		Name   string
		Input  json.RawMessage
		Output json.RawMessage
	}{value.Name, value.InputSchema, value.OutputSchema})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (m *Manager) Inspect(ctx context.Context, productID, connectionID string) (Catalog, error) {
	connection, err := m.store.MCPConnection(ctx, productID, connectionID)
	if err != nil {
		return Catalog{}, err
	}
	if connection.ProtocolVersion != model.StatelessMCPv2Protocol || connection.State != "active" {
		return Catalog{}, ErrInvalidConnection
	}
	bearer, err := m.connectionBearer(ctx, connection)
	if err != nil {
		return Catalog{}, err
	}
	raw, err := m.invoke(ctx, connection, "tools/list", "", nil, bearer, 20*time.Second)
	if err != nil {
		return Catalog{}, err
	}
	var result struct {
		ResultType string `json:"resultType"`
		Tools      []struct {
			Name         string          `json:"name"`
			Title        string          `json:"title"`
			Description  string          `json:"description"`
			InputSchema  json.RawMessage `json:"inputSchema"`
			OutputSchema json.RawMessage `json:"outputSchema"`
			Annotations  json.RawMessage `json:"annotations"`
		} `json:"tools"`
		TTLMS int64 `json:"ttlMs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.ResultType != "complete" {
		return Catalog{}, ErrUpstreamProtocol
	}
	catalog := Catalog{Connection: connection, Tools: make([]CatalogTool, 0, len(result.Tools)), TTLMS: result.TTLMS}
	for _, upstream := range result.Tools {
		tool := CatalogTool{Name: upstream.Name, Title: upstream.Title, Description: upstream.Description, InputSchema: upstream.InputSchema, OutputSchema: upstream.OutputSchema, Annotations: upstream.Annotations}
		tool.SchemaHash = catalogToolHash(tool)
		catalog.Tools = append(catalog.Tools, tool)
	}
	sort.Slice(catalog.Tools, func(i, j int) bool { return catalog.Tools[i].Name < catalog.Tools[j].Name })
	encoded, _ := json.Marshal(catalog.Tools)
	digest := sha256.Sum256(encoded)
	catalog.CatalogHash = hex.EncodeToString(digest[:])
	return catalog, nil
}

func (m *Manager) Import(ctx context.Context, productID, connectionID string, input ImportInput, actor Actor) (ImportResult, error) {
	if len(input.ToolNames) == 0 {
		return ImportResult{}, errors.New("select at least one upstream tool")
	}
	if input.TimeoutMS == 0 {
		input.TimeoutMS = 15000
	}
	if input.TimeoutMS < 100 || input.TimeoutMS > 60000 {
		return ImportResult{}, errors.New("tool timeout must be between 100 and 60000 milliseconds")
	}
	catalog, err := m.Inspect(ctx, productID, connectionID)
	if err != nil {
		return ImportResult{}, err
	}
	selected := make(map[string]bool, len(input.ToolNames))
	for _, name := range input.ToolNames {
		selected[name] = true
	}
	existingValues, err := m.store.Tools(ctx, productID, false)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ImportResult{}, err
	}
	existing := make(map[string]model.Tool)
	for _, value := range existingValues {
		if value.BackendKind == "mcp" && value.MCPConnectionID == connectionID {
			existing[value.UpstreamToolName] = value
		}
	}
	result := ImportResult{Connection: catalog.Connection, Rejected: make(map[string]string)}
	policy, _ := json.Marshal(map[string]any{"required_entitlements": normalizeScopes(input.RequiredEntitlements), "confirmation_required": input.ConfirmationRequired})
	for _, upstream := range catalog.Tools {
		if !selected[upstream.Name] {
			continue
		}
		if !upstreamToolPattern.MatchString(upstream.Name) || len(catalog.Connection.Namespace)+1+len(upstream.Name) > 128 {
			result.Rejected[upstream.Name] = "tool name cannot be safely namespaced"
			continue
		}
		inputSchema, err := normalizeInputSchema(upstream.InputSchema)
		if err != nil {
			result.Rejected[upstream.Name] = err.Error()
			continue
		}
		outputSchema, err := normalizeOutputSchema(upstream.OutputSchema)
		if err != nil {
			result.Rejected[upstream.Name] = err.Error()
			continue
		}
		annotations := upstream.Annotations
		if len(annotations) == 0 {
			annotations = json.RawMessage(`{}`)
		}
		candidate := model.Tool{OrganisationID: catalog.Connection.OrganisationID, ProductID: productID, Namespace: catalog.Connection.Namespace, Name: upstream.Name, Description: strings.TrimSpace(upstream.Description), InputSchema: inputSchema, OutputSchema: outputSchema, HTTPMethod: "MCP", AuthorizationPolicy: policy, TimeoutMS: input.TimeoutMS, BackendKind: "mcp", MCPConnectionID: connectionID, UpstreamToolName: upstream.Name, UpstreamSchemaHash: upstream.SchemaHash, UpstreamAnnotations: annotations}
		if candidate.Description == "" {
			candidate.Description = upstream.Title
		}
		if candidate.Description == "" {
			candidate.Description = "Imported Stateless MCPv2 tool " + upstream.Name
		}
		current, ok := existing[upstream.Name]
		if !ok {
			candidate.ID, err = randomUUID()
			if err != nil {
				return ImportResult{}, err
			}
			created, err := m.store.CreateTool(ctx, candidate)
			if err != nil {
				result.Rejected[upstream.Name] = err.Error()
				continue
			}
			result.Created = append(result.Created, created)
			continue
		}
		if current.UpstreamSchemaHash == upstream.SchemaHash {
			if current.UpstreamDrifted {
				current, _ = m.store.MarkImportedToolDrift(ctx, productID, current.ID, false)
			}
			result.Unchanged = append(result.Unchanged, current)
			continue
		}
		if current.State == "published" {
			drifted, markErr := m.store.MarkImportedToolDrift(ctx, productID, current.ID, true)
			if markErr != nil {
				return ImportResult{}, markErr
			}
			result.Drifted = append(result.Drifted, drifted)
			continue
		}
		candidate.ID = current.ID
		updated, err := m.store.UpdateImportedTool(ctx, candidate, current.Revision)
		if err != nil {
			return ImportResult{}, err
		}
		result.Updated = append(result.Updated, updated)
	}
	syncedAt := m.now()
	result.Connection, err = m.store.UpdateMCPConnectionSync(ctx, productID, connectionID, catalog.CatalogHash, syncedAt)
	if err != nil {
		return ImportResult{}, err
	}
	_ = m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: result.Connection.OrganisationID, ProductID: productID, ActorID: actor.ID, Action: "mcp.connection.imported", TargetType: "mcp_connection", TargetID: connectionID, Current: map[string]any{"protocol_version": model.StatelessMCPv2Protocol, "created": len(result.Created), "updated": len(result.Updated), "drifted": len(result.Drifted), "rejected": len(result.Rejected), "catalog_hash": catalog.CatalogHash}, RequestID: actor.RequestID, CreatedAt: syncedAt})
	return result, nil
}

func subjectID(principal toolruntime.Principal) string {
	return principal.Subject
}

func (m *Manager) BeginAuthorization(ctx context.Context, productID, connectionID string, principal toolruntime.Principal) (string, error) {
	connection, err := m.store.MCPConnection(ctx, productID, connectionID)
	if err != nil {
		return "", err
	}
	if connection.AuthMode != "delegated_oauth" || principal.Subject == "" {
		return "", ErrUnsupportedAuth
	}
	state, err := randomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(state))
	if err := m.store.CreateMCPAuthorizationState(ctx, model.MCPAuthorizationState{Digest: digest[:], ConnectionID: connectionID, ProductID: productID, SubjectID: subjectID(principal), CodeVerifier: verifier, ExpiresAt: m.now().Add(10 * time.Minute)}); err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {connection.OAuthClientID},
		"redirect_uri":          {m.baseURL + "/oauth/upstream/callback"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"resource":              {connection.Endpoint},
	}
	if len(connection.Scopes) > 0 {
		values.Set("scope", strings.Join(connection.Scopes, " "))
	}
	parsed, _ := url.Parse(connection.AuthorizationURL)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (m *Manager) oauthToken(ctx context.Context, connection model.MCPConnection, values url.Values) (tokenResponse, error) {
	parsed, address, err := m.safeDestination(ctx, connection.TokenURL)
	if err != nil {
		return tokenResponse{}, err
	}
	clientSecret, err := m.decryptSecret(ctx, connection.OrganisationID, connection.OAuthClientSecretID, connection.OrganisationID+":mcp-connection:"+connection.ID+":oauth-client:")
	if err != nil {
		return tokenResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(connection.OAuthClientID, clientSecret)
	response, err := m.client(parsed, address, 15*time.Second).Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("upstream authorization server returned %s", response.Status)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&token); err != nil || token.AccessToken == "" || (token.TokenType != "" && !strings.EqualFold(token.TokenType, "Bearer")) {
		return tokenResponse{}, errors.New("upstream authorization server returned an invalid token")
	}
	return token, nil
}

func (m *Manager) saveGrant(ctx context.Context, connection model.MCPConnection, subject string, token tokenResponse) (model.MCPUserGrant, error) {
	accessID, err := m.saveSecret(ctx, connection.OrganisationID, "mcp-user-access-"+connection.Namespace, "mcp_upstream_user_access", token.AccessToken, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+subject+":access:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	refreshID, err := m.saveSecret(ctx, connection.OrganisationID, "mcp-user-refresh-"+connection.Namespace, "mcp_upstream_user_refresh", token.RefreshToken, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+subject+":refresh:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	id, _ := randomUUID()
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 || expiresIn > int64((30*24*time.Hour).Seconds()) {
		expiresIn = 3600
	}
	scopes := connection.Scopes
	if token.Scope != "" {
		scopes = strings.Fields(token.Scope)
	}
	return m.store.SaveMCPUserGrant(ctx, model.MCPUserGrant{ID: id, OrganisationID: connection.OrganisationID, ProductID: connection.ProductID, ConnectionID: connection.ID, SubjectID: subject, AccessSecretID: accessID, RefreshSecretID: refreshID, Scopes: normalizeScopes(scopes), ExpiresAt: m.now().Add(time.Duration(expiresIn) * time.Second)})
}

func (m *Manager) CompleteAuthorization(ctx context.Context, state, code, issuer string) (model.MCPUserGrant, error) {
	digest := sha256.Sum256([]byte(strings.TrimSpace(state)))
	stored, err := m.store.ConsumeMCPAuthorizationState(ctx, digest[:])
	if err != nil || strings.TrimSpace(code) == "" {
		return model.MCPUserGrant{}, errors.New("upstream authorization state is invalid or expired")
	}
	connection, err := m.store.MCPConnection(ctx, stored.ProductID, stored.ConnectionID)
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	if strings.TrimSpace(issuer) != connection.OAuthIssuer {
		return model.MCPUserGrant{}, errors.New("upstream authorization issuer did not match the pinned issuer")
	}
	token, err := m.oauthToken(ctx, connection, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {m.baseURL + "/oauth/upstream/callback"}, "code_verifier": {stored.CodeVerifier}, "resource": {connection.Endpoint}})
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	grant, err := m.saveGrant(ctx, connection, stored.SubjectID, token)
	if err == nil {
		_ = m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: connection.OrganisationID, ProductID: connection.ProductID, ActorID: stored.SubjectID, Action: "mcp.user_grant.connected", TargetType: "mcp_connection", TargetID: connection.ID, Current: map[string]any{"auth_mode": connection.AuthMode, "scopes": grant.Scopes, "expires_at": grant.ExpiresAt}, CreatedAt: m.now()})
	}
	return grant, err
}

func (m *Manager) refreshGrant(ctx context.Context, connection model.MCPConnection, grant model.MCPUserGrant) (model.MCPUserGrant, error) {
	if grant.RefreshSecretID == "" {
		return model.MCPUserGrant{}, ErrGrantRequired
	}
	refresh, err := m.decryptSecret(ctx, connection.OrganisationID, grant.RefreshSecretID, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+grant.SubjectID+":refresh:")
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	token, err := m.oauthToken(ctx, connection, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "resource": {connection.Endpoint}, "scope": {strings.Join(grant.Scopes, " ")}})
	if err != nil {
		return model.MCPUserGrant{}, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refresh
	}
	return m.saveGrant(ctx, connection, grant.SubjectID, token)
}

func (m *Manager) delegatedBearer(ctx context.Context, connection model.MCPConnection, principal toolruntime.Principal) (string, error) {
	grant, err := m.store.MCPUserGrant(ctx, connection.ID, subjectID(principal))
	if err != nil {
		return "", ErrGrantRequired
	}
	if m.now().Add(30 * time.Second).After(grant.ExpiresAt) {
		grant, err = m.refreshGrant(ctx, connection, grant)
		if err != nil {
			return "", ErrGrantRequired
		}
	}
	return m.decryptSecret(ctx, connection.OrganisationID, grant.AccessSecretID, connection.OrganisationID+":mcp-grant:"+connection.ID+":"+grant.SubjectID+":access:")
}

func validateStructuredOutput(schemaRaw json.RawMessage, result map[string]any) error {
	if len(schemaRaw) == 0 || string(schemaRaw) == "{}" {
		return nil
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return errors.New("structuredContent is required by the imported output schema")
	}
	var schema map[string]any
	if json.Unmarshal(schemaRaw, &schema) != nil || schema["type"] != "object" || schema["additionalProperties"] != false {
		return errors.New("imported output schema is invalid")
	}
	return toolruntime.ValidateArguments(schemaRaw, structured)
}

func (m *Manager) ExecuteMCP(ctx context.Context, tool model.Tool, arguments map[string]any, principal toolruntime.Principal) (toolruntime.MCPCallResult, error) {
	connection, err := m.store.MCPConnection(ctx, tool.ProductID, tool.MCPConnectionID)
	if err != nil || connection.State != "active" || connection.ProtocolVersion != model.StatelessMCPv2Protocol || tool.UpstreamDrifted {
		return toolruntime.MCPCallResult{}, ErrInvalidConnection
	}
	var bearer string
	switch connection.AuthMode {
	case "none":
	case "service":
		bearer, err = m.connectionBearer(ctx, connection)
	case "delegated_oauth":
		bearer, err = m.delegatedBearer(ctx, connection, principal)
	default:
		err = ErrUnsupportedAuth
	}
	if err != nil {
		return toolruntime.MCPCallResult{}, err
	}
	timeout := time.Duration(tool.TimeoutMS) * time.Millisecond
	raw, err := m.invoke(ctx, connection, "tools/call", tool.UpstreamToolName, map[string]any{"name": tool.UpstreamToolName, "arguments": arguments}, bearer, timeout)
	if err != nil {
		return toolruntime.MCPCallResult{}, err
	}
	if len(raw) > maxMCPBody {
		return toolruntime.MCPCallResult{}, ErrUpstreamProtocol
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil || result["resultType"] != "complete" {
		return toolruntime.MCPCallResult{}, ErrUpstreamProtocol
	}
	if err := validateStructuredOutput(tool.OutputSchema, result); err != nil {
		return toolruntime.MCPCallResult{}, fmt.Errorf("upstream tool output schema mismatch: %w", err)
	}
	_ = m.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ActorID: principal.Subject, Action: "mcp.tool.executed", TargetType: "tool", TargetID: tool.ID, Current: map[string]any{"connection_id": connection.ID, "upstream_tool": tool.UpstreamToolName, "protocol_version": model.StatelessMCPv2Protocol, "auth_mode": connection.AuthMode, "is_error": result["isError"] == true}, RequestID: principal.RequestID, CreatedAt: m.now()})
	return toolruntime.MCPCallResult{Result: result}, nil
}

func ParseExpiresIn(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}
