package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	ErrDenied                = errors.New("tool execution denied by authorization policy")
	ErrConfirmation          = errors.New("tool execution requires explicit confirmation")
	ErrInvalidIdempotencyKey = errors.New("tool execution idempotency key is invalid")
	ErrRateLimited           = errors.New("tool upstream connection rate limit exceeded")
	ErrUnsafeDestination     = errors.New("tool destination is not safe")
)

const (
	minIdempotencyKeyLength   = 16
	maxIdempotencyKeyLength   = 200
	oauthTokenRefreshSkew     = 30 * time.Second
	maxOAuthTokenCacheEntries = 256
	upstreamConnectionLimit   = 60
	upstreamConnectionWindow  = time.Minute
	maxUpstreamRateWindows    = 4096
	maxUpstreamConcurrency    = 64
	maxConnectionConcurrency  = 8
)

type Store interface {
	Tools(context.Context, string, bool) ([]model.Tool, error)
	AuthorizationPoint(context.Context, string, string) (model.AuthorizationPoint, error)
	GrantDefinitions(context.Context, string) ([]model.GrantDefinition, error)
	AppendAudit(context.Context, model.AuditEvent) error
}

type Principal struct {
	Subject              string
	Issuer               string
	CustomerAccountID    string
	ExternalCustomerID   string
	InstallationID       string
	EnvironmentID        string
	Grants               map[string]bool
	AccessEvaluationID   string
	AccessEvaluatedAt    time.Time
	DelegatedAPIOrigin   string
	DelegatedAccessToken string
	Confirmed            bool
	RequestID            string
	IdempotencyKey       string
}

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type CredentialResolver interface {
	ResolveToolCredential(context.Context, model.Tool) ([]byte, error)
}

type upstreamAuth struct {
	Type                    string   `json:"type"`
	Scheme                  string   `json:"scheme,omitempty"`
	HeaderName              string   `json:"header_name,omitempty"`
	QueryName               string   `json:"query_name,omitempty"`
	Prefix                  string   `json:"prefix,omitempty"`
	Username                string   `json:"username,omitempty"`
	ClientID                string   `json:"client_id,omitempty"`
	TokenURL                string   `json:"token_url,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	Scopes                  []string `json:"scopes,omitempty"`
	Audience                string   `json:"audience,omitempty"`
	Resource                string   `json:"resource,omitempty"`
}

type requestMapping struct {
	ParameterLocations map[string]string `json:"parameter_locations,omitempty"`
}

type responseMapping struct {
	ResultPath string `json:"result_path,omitempty"`
}

type cachedOAuthToken struct {
	AccessToken []byte
	TokenType   string
	ExpiresAt   time.Time
}

type oauthTokenFlight struct {
	done chan struct{}
	err  error
}

type connectionRateWindow struct {
	Started time.Time
	Count   int
}

type executionTrace struct {
	Category             string
	Phase                string
	NetworkCallPerformed bool
	StatusCode           int
	ResponseBytes        int64
	ResponseShape        *model.JSONShape
}

// DraftTestReport is a sanitized execution observation. It deliberately does
// not carry the request, response, destination, headers, or any scalar value.
type DraftTestReport struct {
	AuthenticationType   string
	Outcome              string
	Phase                string
	NetworkCallPerformed bool
	UpstreamStatusCode   int
	ResponseBytes        int64
	RequestShape         model.JSONShape
	ResponseShape        *model.JSONShape
	Findings             []model.ToolTestFinding
	DurationMS           int64
}

const (
	maxTestShapeDepth      = 5
	maxTestShapeKeys       = 64
	maxTestShapeArrayItems = 8
	maxTestShapeKeyBytes   = 128
)

func jsonShape(value any) model.JSONShape {
	remaining := maxTestShapeKeys
	return jsonShapeBounded(value, nil, 0, &remaining)
}

func SanitizedJSONShape(value any) model.JSONShape { return jsonShape(value) }

func jsonShapeForSchema(value any, rawSchema json.RawMessage) model.JSONShape {
	if ValidateSchema(rawSchema) != nil {
		return jsonShape(value)
	}
	var schema map[string]any
	if json.Unmarshal(rawSchema, &schema) != nil {
		return jsonShape(value)
	}
	remaining := maxTestShapeKeys
	return jsonShapeBounded(value, schema, 0, &remaining)
}

func safeUnexpectedShapeKey(properties map[string]any, existing map[string]model.JSONShape, index int) string {
	for {
		candidate := fmt.Sprintf("[unexpected-property-%d]", index)
		_, declared := properties[candidate]
		_, used := existing[candidate]
		if !declared && !used {
			return candidate
		}
		index++
	}
}

func safeDeclaredShapeKey(value string) bool {
	if len(value) == 0 || len(value) > maxTestShapeKeyBytes {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		if index == 0 {
			if !letter {
				return false
			}
			continue
		}
		if !letter && !(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	normalized := strings.ToLower(value)
	normalized = strings.NewReplacer("_", "", "-", "", ".", "").Replace(normalized)
	for _, credentialPattern := range []string{
		"authorization", "authentication", "bearer", "credential", "password", "passwd",
		"secret", "token", "apikey", "accesskey", "privatekey", "signingkey", "sessionkey", "cookie",
	} {
		if strings.Contains(normalized, credentialPattern) {
			return false
		}
	}
	return true
}

func safeSchemaShapeKey(existing map[string]model.JSONShape, index int) string {
	for {
		candidate := fmt.Sprintf("[schema-property-%d]", index)
		if _, used := existing[candidate]; !used {
			return candidate
		}
		index++
	}
}

func jsonShapeBounded(value any, schema map[string]any, depth int, remaining *int) model.JSONShape {
	if depth >= maxTestShapeDepth || *remaining <= 0 {
		return model.JSONShape{Type: jsonType(value), Truncated: true}
	}
	*remaining--
	switch current := value.(type) {
	case map[string]any:
		result := model.JSONShape{Type: "object", Properties: make(map[string]model.JSONShape)}
		properties, _ := schema["properties"].(map[string]any)
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		unexpectedIndex := 1
		schemaIndex := 1
		for _, key := range keys {
			if *remaining <= 0 {
				result.Truncated = true
				break
			}
			childSchema, declared := properties[key].(map[string]any)
			evidenceKey := key
			if !declared {
				evidenceKey = safeUnexpectedShapeKey(properties, result.Properties, unexpectedIndex)
				unexpectedIndex++
			} else if !safeDeclaredShapeKey(evidenceKey) {
				evidenceKey = safeSchemaShapeKey(result.Properties, schemaIndex)
				schemaIndex++
				result.Truncated = true
			}
			result.Properties[evidenceKey] = jsonShapeBounded(current[key], childSchema, depth+1, remaining)
		}
		if len(result.Properties) == 0 {
			result.Properties = nil
		}
		return result
	case []any:
		result := model.JSONShape{Type: "array", Length: len(current)}
		itemSchema, _ := schema["items"].(map[string]any)
		limit := len(current)
		if limit > maxTestShapeArrayItems {
			limit = maxTestShapeArrayItems
			result.Truncated = true
		}
		for index := 0; index < limit; index++ {
			result.Items = append(result.Items, jsonShapeBounded(current[index], itemSchema, depth+1, remaining))
		}
		return result
	default:
		return model.JSONShape{Type: jsonType(value)}
	}
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return "unknown"
	}
}

func unsafeIP(address net.IP) bool {
	if address == nil || address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return true
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28", "2001:db8::/32", "2002::/16", "fc00::/7", "fec0::/10", "fe80::/10"} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(address) {
			return true
		}
	}
	return false
}

type Runtime struct {
	store                        Store
	resolver                     Resolver
	doer                         Doer
	mcpExecutor                  MCPExecutor
	credentials                  CredentialResolver
	privateLocalhostDestinations map[string]struct{}
	tokenMu                      sync.Mutex
	tokens                       map[string]cachedOAuthToken
	tokenFlights                 map[string]*oauthTokenFlight
	rateMu                       sync.Mutex
	rates                        map[string]connectionRateWindow
	concurrencyMu                sync.Mutex
	globalInFlight               int
	connectionInFlight           map[string]int
	now                          func() time.Time
}

// BoundAuthorization is the exact immutable Integration action contract
// selected for one tool. The current authorization point is included so the
// runtime can reject a changed, deprecated, or otherwise stale binding.
type BoundAuthorization struct {
	IntegrationID              string
	ToolID                     string
	ToolRevision               int64
	AuthorizationPoint         model.AuthorizationPoint
	AuthorizationPointRevision int64
}

type MCPExecutor interface {
	ExecuteMCP(context.Context, model.Tool, map[string]any, Principal) (MCPCallResult, error)
}

type MCPCallResult struct {
	Result map[string]any
}

func NewRuntime(store Store, resolver Resolver, doer Doer) *Runtime {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Runtime{store: store, resolver: resolver, doer: doer, tokens: make(map[string]cachedOAuthToken), tokenFlights: make(map[string]*oauthTokenFlight), rates: make(map[string]connectionRateWindow), connectionInFlight: make(map[string]int), now: func() time.Time { return time.Now().UTC() }}
}

func (r *Runtime) SetMCPExecutor(executor MCPExecutor)               { r.mcpExecutor = executor }
func (r *Runtime) SetCredentialResolver(resolver CredentialResolver) { r.credentials = resolver }

// SetPrivateLocalhostHosts configures the exact development destinations that
// may resolve to loopback or private addresses. Entries are hostname:port; a
// host-only legacy entry grants only the HTTP default port 80, never every
// local service listening on that hostname.
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

func (r *Runtime) Published(ctx context.Context, productID string) ([]model.Tool, error) {
	return r.store.Tools(ctx, productID, true)
}

func (r *Runtime) Available(ctx context.Context, productID string, grants map[string]bool) ([]model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return nil, err
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		if !value.UpstreamDrifted && grantsAllow(value, grants) {
			result = append(result, value)
		}
	}
	return result, nil
}

func (r *Runtime) authorizeBound(ctx context.Context, tool model.Tool, binding BoundAuthorization, principal Principal, enforceConfirmation bool) error {
	publishedPoint := binding.AuthorizationPoint
	if binding.IntegrationID == "" || binding.ToolID != tool.ID || binding.ToolRevision != tool.Revision || publishedPoint.ID == "" || publishedPoint.IntegrationID != binding.IntegrationID || publishedPoint.Revision != binding.AuthorizationPointRevision || publishedPoint.State != "active" {
		return ErrDenied
	}
	point, err := r.store.AuthorizationPoint(ctx, binding.IntegrationID, publishedPoint.ID)
	if err != nil || point.ID != publishedPoint.ID || point.IntegrationID != binding.IntegrationID || point.Revision != binding.AuthorizationPointRevision || point.State != "active" {
		return ErrDenied
	}
	definitions, err := r.store.GrantDefinitions(ctx, point.DeploymentID)
	if err != nil {
		return ErrDenied
	}
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		active[definition.Key] = definition.State == "active"
	}
	var toolPolicy struct {
		RequiredGrants []string `json:"required_grants"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &toolPolicy); err != nil {
		return ErrDenied
	}
	requiredGrants := append(append([]string(nil), point.RequiredGrants...), toolPolicy.RequiredGrants...)
	for _, required := range requiredGrants {
		if !active[required] || !principal.Grants[required] {
			return ErrDenied
		}
	}
	if point.DecisionTTLSeconds <= 0 || strings.TrimSpace(principal.AccessEvaluationID) == "" || principal.AccessEvaluatedAt.IsZero() {
		return ErrDenied
	}
	age := r.now().Sub(principal.AccessEvaluatedAt)
	if age < 0 || age > time.Duration(point.DecisionTTLSeconds)*time.Second {
		return ErrDenied
	}
	if enforceConfirmation && point.ConfirmationRequired && !principal.Confirmed {
		return ErrConfirmation
	}
	return nil
}

// AvailableBound returns only tools whose legacy tool policy and exact
// Integration authorization action both allow discovery. Confirmation is
// advertised to clients but is enforced only on execution.
func (r *Runtime) AvailableBound(ctx context.Context, productID string, bindings []BoundAuthorization, principal Principal) ([]model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return nil, err
	}
	byTool := make(map[string][]BoundAuthorization, len(bindings))
	for _, binding := range bindings {
		byTool[binding.ToolID] = append(byTool[binding.ToolID], binding)
	}
	result := make([]model.Tool, 0, len(values))
	for _, value := range values {
		candidates := byTool[value.ID]
		if value.UpstreamDrifted || len(candidates) != 1 || !grantsAllow(value, principal.Grants) || r.authorizeBound(ctx, value, candidates[0], principal, false) != nil {
			continue
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *Runtime) find(ctx context.Context, productID, fullName string) (model.Tool, error) {
	values, err := r.Published(ctx, productID)
	if err != nil {
		return model.Tool{}, err
	}
	for _, value := range values {
		if value.Namespace+"."+value.Name == fullName {
			return value, nil
		}
	}
	return model.Tool{}, errors.New("published tool not found")
}

func authorize(tool model.Tool, principal Principal) error {
	var policy struct {
		RequiredGrants       []string `json:"required_grants"`
		ConfirmationRequired bool     `json:"confirmation_required"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return ErrDenied
	}
	for _, required := range policy.RequiredGrants {
		if !principal.Grants[required] {
			return ErrDenied
		}
	}
	if policy.ConfirmationRequired && !principal.Confirmed {
		return ErrConfirmation
	}
	return nil
}

func grantsAllow(tool model.Tool, grants map[string]bool) bool {
	var policy struct {
		RequiredGrants []string `json:"required_grants"`
	}
	if err := json.Unmarshal(tool.AuthorizationPolicy, &policy); err != nil {
		return false
	}
	for _, required := range policy.RequiredGrants {
		if !grants[required] {
			return false
		}
	}
	return true
}

func (r *Runtime) safeDestination(ctx context.Context, raw string) (*url.URL, net.IP, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, nil, ErrUnsafeDestination
	}
	hostname := strings.ToLower(parsed.Hostname())
	localDevelopment := identity.IsLocalDevelopmentHostname(hostname)
	if hostname == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (localDevelopment && parsed.Scheme != "http") || (!localDevelopment && (parsed.Scheme != "https" || (parsed.Port() != "" && parsed.Port() != "443"))) {
		return nil, nil, ErrUnsafeDestination
	}
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	_, localDestinationAllowed := r.privateLocalhostDestinations[net.JoinHostPort(hostname, port)]
	if localDevelopment && !localDestinationAllowed {
		return nil, nil, ErrUnsafeDestination
	}
	addresses, err := r.resolver.LookupIP(ctx, "ip", hostname)
	if err != nil || len(addresses) == 0 {
		return nil, nil, ErrUnsafeDestination
	}
	for _, address := range addresses {
		if localDevelopment {
			if address == nil || !address.IsLoopback() && !address.IsPrivate() {
				return nil, nil, ErrUnsafeDestination
			}
			continue
		}
		if unsafeIP(address) {
			return nil, nil, ErrUnsafeDestination
		}
	}
	return parsed, addresses[0], nil
}

func (r *Runtime) client(parsed *url.URL, address net.IP, timeout time.Duration) Doer {
	if r.doer != nil {
		return r.doer
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	localDevelopment := identity.IsLocalDevelopmentHostname(parsed.Hostname())
	port := parsed.Port()
	if port == "" {
		port = "443"
		if localDevelopment {
			port = "80"
		}
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
	}, DisableCompression: true, DisableKeepAlives: true, ResponseHeaderTimeout: timeout}
	if !localDevelopment {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func auditID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return "audit_" + hex.EncodeToString(buffer)
}

func (r *Runtime) Execute(ctx context.Context, productID, fullName string, arguments map[string]any, principal Principal) (any, error) {
	tool, err := r.find(ctx, productID, fullName)
	if err != nil {
		return nil, err
	}
	if tool.UpstreamDrifted {
		return nil, ErrDenied
	}
	if err := ValidateArguments(tool.InputSchema, arguments); err != nil {
		return nil, err
	}
	if err := authorize(tool, principal); err != nil {
		return nil, err
	}
	return r.executeAuthorized(ctx, productID, fullName, tool, arguments, principal)
}

func toolUpstreamAuth(tool model.Tool) (upstreamAuth, error) {
	if len(tool.UpstreamAuth) == 0 {
		return upstreamAuth{Type: "delegated_oauth"}, nil
	}
	var value upstreamAuth
	if err := json.Unmarshal(tool.UpstreamAuth, &value); err != nil || value.Type == "" {
		return upstreamAuth{}, ErrDenied
	}
	if value.Type == "authorization_scheme" && !validAuthorizationScheme(value.Scheme) {
		return upstreamAuth{}, ErrDenied
	}
	if value.Type == "api_key_header" || value.Type == "custom_header" {
		if len(value.Prefix) > 64 || strings.ContainsAny(value.Prefix, "\r\n\x00") {
			return upstreamAuth{}, ErrDenied
		}
		value.Prefix = strings.TrimSpace(value.Prefix)
	}
	return value, nil
}

func prefixedCredential(prefix string, credential []byte) string {
	if prefix == "" {
		return string(credential)
	}
	return prefix + " " + string(credential)
}

func validAuthorizationScheme(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

func toolRequestMapping(tool model.Tool) (requestMapping, error) {
	if len(tool.RequestMapping) == 0 {
		return requestMapping{}, nil
	}
	var value requestMapping
	if err := json.Unmarshal(tool.RequestMapping, &value); err != nil {
		return requestMapping{}, ErrDenied
	}
	return value, nil
}

func toolResponseMapping(tool model.Tool) (responseMapping, error) {
	if len(tool.ResponseMapping) == 0 {
		return responseMapping{}, nil
	}
	var value responseMapping
	if err := json.Unmarshal(tool.ResponseMapping, &value); err != nil {
		return responseMapping{}, ErrDenied
	}
	return value, nil
}

func wipe(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func (r *Runtime) toolCredential(ctx context.Context, tool model.Tool) ([]byte, error) {
	if r.credentials == nil || tool.CredentialID == "" {
		return nil, ErrDenied
	}
	value, err := r.credentials.ResolveToolCredential(ctx, tool)
	if err != nil || len(value) == 0 {
		wipe(value)
		return nil, ErrDenied
	}
	return value, nil
}

func (r *Runtime) purgeExpiredOAuthTokensLocked(now time.Time) {
	for key, cached := range r.tokens {
		if now.Add(oauthTokenRefreshSkew).Before(cached.ExpiresAt) {
			continue
		}
		wipe(cached.AccessToken)
		delete(r.tokens, key)
	}
}

func (r *Runtime) cacheOAuthTokenLocked(cacheKey, tokenType, accessToken string, expiresAt time.Time) {
	if r.tokens == nil {
		r.tokens = make(map[string]cachedOAuthToken)
	}
	if previous, ok := r.tokens[cacheKey]; ok {
		wipe(previous.AccessToken)
	} else {
		for len(r.tokens) >= maxOAuthTokenCacheEntries {
			var evictionKey string
			var eviction cachedOAuthToken
			for key, cached := range r.tokens {
				if evictionKey == "" || cached.ExpiresAt.Before(eviction.ExpiresAt) || cached.ExpiresAt.Equal(eviction.ExpiresAt) && key < evictionKey {
					evictionKey = key
					eviction = cached
				}
			}
			wipe(eviction.AccessToken)
			delete(r.tokens, evictionKey)
		}
	}
	r.tokens[cacheKey] = cachedOAuthToken{AccessToken: []byte(accessToken), TokenType: tokenType, ExpiresAt: expiresAt}
}

func (r *Runtime) exchangeOAuthClientToken(ctx context.Context, tool model.Tool, auth upstreamAuth, trace *executionTrace) (string, string, time.Time, error) {
	credential, err := r.toolCredential(ctx, tool)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer wipe(credential)
	parsed, address, err := r.safeDestination(ctx, auth.TokenURL)
	if err != nil {
		return "", "", time.Time{}, err
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(auth.Scopes) > 0 {
		form.Set("scope", strings.Join(auth.Scopes, " "))
	}
	if auth.Audience != "" {
		form.Set("audience", auth.Audience)
	}
	if auth.Resource != "" {
		form.Set("resource", auth.Resource)
	}
	if auth.TokenEndpointAuthMethod == "" {
		auth.TokenEndpointAuthMethod = "client_secret_basic"
	}
	if auth.TokenEndpointAuthMethod == "client_secret_post" {
		form.Set("client_id", auth.ClientID)
		form.Set("client_secret", string(credential))
	} else if auth.TokenEndpointAuthMethod != "client_secret_basic" {
		return "", "", time.Time{}, ErrDenied
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", time.Time{}, errors.New("upstream OAuth token request is invalid")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if auth.TokenEndpointAuthMethod == "client_secret_basic" {
		request.SetBasicAuth(url.QueryEscape(auth.ClientID), url.QueryEscape(string(credential)))
	}
	if trace != nil {
		trace.NetworkCallPerformed = true
		trace.Phase = "token_exchange"
	}
	response, err := r.client(parsed, address, min(time.Duration(tool.TimeoutMS)*time.Millisecond, 15*time.Second)).Do(request)
	if err != nil {
		return "", "", time.Time{}, errors.New("upstream OAuth token exchange failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", time.Time{}, errors.New("upstream OAuth token exchange failed")
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
	if err != nil || len(encoded) > 64<<10 {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	defer wipe(encoded)
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := decoder.Decode(&token); err != nil || strings.TrimSpace(token.AccessToken) == "" || strings.ContainsAny(token.AccessToken, "\r\n\x00") {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", "", time.Time{}, errors.New("upstream OAuth token response is invalid")
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if !strings.EqualFold(token.TokenType, "Bearer") {
		return "", "", time.Time{}, errors.New("upstream OAuth token type is unsupported")
	}
	if token.ExpiresIn < 1 || token.ExpiresIn > 86400 {
		token.ExpiresIn = 300
	}
	expiresAt := r.now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return "Bearer", token.AccessToken, expiresAt, nil
}

func (r *Runtime) oauthClientToken(ctx context.Context, tool model.Tool, auth upstreamAuth) (string, string, error) {
	return r.oauthClientTokenTraced(ctx, tool, auth, nil)
}

func (r *Runtime) oauthClientTokenTraced(ctx context.Context, tool model.Tool, auth upstreamAuth, trace *executionTrace) (string, string, error) {
	cacheKey := tool.APIConnectionID + "\x00" + tool.CredentialID
	if tool.RuntimeServiceConnectionID != "" {
		cacheKey = tool.RuntimeServiceConnectionID + "\x00" + tool.RuntimeCredentialVersionID
	}
	for {
		r.tokenMu.Lock()
		r.purgeExpiredOAuthTokensLocked(r.now())
		if cached, ok := r.tokens[cacheKey]; ok {
			tokenType, accessToken := cached.TokenType, string(cached.AccessToken)
			r.tokenMu.Unlock()
			return tokenType, accessToken, nil
		}
		if flight, ok := r.tokenFlights[cacheKey]; ok {
			r.tokenMu.Unlock()
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-flight.done:
				if flight.err != nil {
					return "", "", flight.err
				}
				continue
			}
		}
		if r.tokenFlights == nil {
			r.tokenFlights = make(map[string]*oauthTokenFlight)
		}
		flight := &oauthTokenFlight{done: make(chan struct{})}
		r.tokenFlights[cacheKey] = flight
		r.tokenMu.Unlock()

		tokenType, accessToken, expiresAt, err := r.exchangeOAuthClientToken(ctx, tool, auth, trace)
		r.tokenMu.Lock()
		if err == nil {
			r.purgeExpiredOAuthTokensLocked(r.now())
			r.cacheOAuthTokenLocked(cacheKey, tokenType, accessToken, expiresAt)
		}
		flight.err = err
		delete(r.tokenFlights, cacheKey)
		close(flight.done)
		r.tokenMu.Unlock()
		return tokenType, accessToken, err
	}
}

func requestScalarText(value any) (string, error) {
	switch current := value.(type) {
	case string:
		return current, nil
	case bool:
		return strconv.FormatBool(current), nil
	case json.Number:
		return current.String(), nil
	case float64:
		return strconv.FormatFloat(current, 'g', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(current), 'g', -1, 32), nil
	case int:
		return strconv.Itoa(current), nil
	case int8:
		return strconv.FormatInt(int64(current), 10), nil
	case int16:
		return strconv.FormatInt(int64(current), 10), nil
	case int32:
		return strconv.FormatInt(int64(current), 10), nil
	case int64:
		return strconv.FormatInt(current, 10), nil
	case uint:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(current), 10), nil
	case uint64:
		return strconv.FormatUint(current, 10), nil
	default:
		return "", errors.New("request mapping requires a scalar value")
	}
}

func applyPathArgument(path, name string, value any) (string, error) {
	text, err := requestScalarText(value)
	if err != nil {
		return "", err
	}
	if text == "" || text == "." || text == ".." || strings.ContainsAny(text, "/\\?#\r\n\x00") {
		return "", fmt.Errorf("path argument %s is unsafe", name)
	}
	placeholder := "{" + name + "}"
	if !strings.Contains(path, placeholder) {
		return "", fmt.Errorf("path argument %s has no endpoint placeholder", name)
	}
	return strings.ReplaceAll(path, placeholder, text), nil
}

func extractMappedResult(value any, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return value, nil
	}
	var segments []string
	if strings.HasPrefix(raw, "/") {
		for _, segment := range strings.Split(strings.TrimPrefix(raw, "/"), "/") {
			segments = append(segments, strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~"))
		}
	} else {
		segments = strings.Split(raw, ".")
	}
	current := value
	for _, segment := range segments {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New("response result path does not resolve to an object field")
		}
		current, ok = object[segment]
		if !ok {
			return nil, errors.New("response result path was not found")
		}
	}
	return current, nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < minIdempotencyKeyLength || len(value) > maxIdempotencyKeyLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func ValidIdempotencyKey(value string) bool { return validIdempotencyKey(value) }

func upstreamIdempotencyKey(productID string, tool model.Tool, principal Principal) string {
	digest := sha256.New()
	for _, field := range []string{
		"dokosoko-http-tool-idempotency-v1",
		productID,
		tool.ID,
		strconv.FormatInt(tool.Revision, 10),
		principal.Issuer,
		principal.Subject,
		principal.CustomerAccountID,
		principal.InstallationID,
		principal.IdempotencyKey,
	} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(field))
	}
	return "doko_" + hex.EncodeToString(digest.Sum(nil))
}

// allowUpstreamConnection is intentionally scoped to one Runtime process.
// Deployments with multiple replicas multiply the aggregate allowance. The
// fixed map cap prevents unbounded memory growth; a new connection fails
// closed when all slots are occupied by active windows.
func (r *Runtime) allowUpstreamConnection(productID string, tool model.Tool) bool {
	key := upstreamConnectionKey(productID, tool)
	now := r.now()
	r.rateMu.Lock()
	defer r.rateMu.Unlock()
	window, exists := r.rates[key]
	if !exists {
		for currentKey, current := range r.rates {
			if current.Started.IsZero() || !now.Before(current.Started) && now.Sub(current.Started) >= upstreamConnectionWindow {
				delete(r.rates, currentKey)
			}
		}
		if len(r.rates) >= maxUpstreamRateWindows {
			return false
		}
	}
	if window.Started.IsZero() || now.Sub(window.Started) >= upstreamConnectionWindow || now.Before(window.Started) {
		window = connectionRateWindow{Started: now}
	}
	if window.Count >= upstreamConnectionLimit {
		r.rates[key] = window
		return false
	}
	window.Count++
	r.rates[key] = window
	return true
}

func upstreamConnectionKey(productID string, tool model.Tool) string {
	if tool.RuntimeServiceConnectionID != "" {
		return productID + "\x00runtime:" + tool.RuntimeServiceConnectionID + "\x00" + tool.RuntimeConnectionRevisionID
	}
	key := productID + "\x00" + tool.APIConnectionID
	if tool.APIConnectionID == "" {
		key = productID + "\x00tool:" + tool.ID
	}
	return key
}

func prepareRuntimeTool(tool model.Tool, environmentID string) (model.Tool, error) {
	if tool.RuntimeServiceConnectionID == "" {
		return tool, nil
	}
	var selected *model.ToolRuntimeTarget
	for index := range tool.RuntimeTargets {
		candidate := &tool.RuntimeTargets[index]
		if candidate.RuntimeServiceConnectionID != tool.RuntimeServiceConnectionID {
			return model.Tool{}, ErrDenied
		}
		if environmentID != "" && candidate.EnvironmentID == environmentID {
			selected = candidate
			break
		}
	}
	if environmentID == "" && len(tool.RuntimeTargets) == 1 {
		selected = &tool.RuntimeTargets[0]
	}
	if selected == nil || selected.BaseURL == "" || tool.HTTPPath == "" || !strings.HasPrefix(tool.HTTPPath, "/") || strings.HasPrefix(tool.HTTPPath, "//") || strings.ContainsAny(tool.HTTPPath, "?#\\\r\n\x00") {
		return model.Tool{}, ErrDenied
	}
	authConfig := map[string]any{}
	if len(selected.AuthConfig) > 0 && json.Unmarshal(selected.AuthConfig, &authConfig) != nil {
		return model.Tool{}, ErrDenied
	}
	authConfig["type"] = selected.AuthenticationType
	if selected.AuthenticationType == "api_key_header" || selected.AuthenticationType == "custom_header" {
		authConfig["header_name"] = selected.HeaderName
	}
	authRaw, err := json.Marshal(authConfig)
	if err != nil {
		return model.Tool{}, ErrDenied
	}
	tool.BaseURL = strings.TrimRight(selected.BaseURL, "/") + tool.HTTPPath
	tool.UpstreamAuth = authRaw
	tool.CredentialID = selected.CredentialSecretID
	tool.CredentialFingerprint = selected.CredentialFingerprint
	tool.CredentialPresent = selected.CredentialSetID == "" || selected.CredentialSecretID != ""
	tool.RuntimeConnectionRevisionID = selected.ConnectionRevisionID
	tool.RuntimeCredentialSetID = selected.CredentialSetID
	tool.RuntimeCredentialVersionID = selected.CredentialVersionID
	return tool, nil
}

// acquireUpstreamSlot places an independent hard ceiling on concurrent
// outbound work. The fixed-window limiter remains a per-process best-effort
// request bound; this cap prevents a burst (or many connection windows) from
// exhausting sockets and goroutines at once.
func (r *Runtime) acquireUpstreamSlot(productID string, tool model.Tool) bool {
	key := upstreamConnectionKey(productID, tool)
	r.concurrencyMu.Lock()
	defer r.concurrencyMu.Unlock()
	if r.globalInFlight >= maxUpstreamConcurrency || r.connectionInFlight[key] >= maxConnectionConcurrency {
		return false
	}
	r.globalInFlight++
	r.connectionInFlight[key]++
	return true
}

func (r *Runtime) releaseUpstreamSlot(productID string, tool model.Tool) {
	key := upstreamConnectionKey(productID, tool)
	r.concurrencyMu.Lock()
	defer r.concurrencyMu.Unlock()
	if count := r.connectionInFlight[key]; count <= 1 {
		delete(r.connectionInFlight, key)
	} else {
		r.connectionInFlight[key] = count - 1
	}
	if r.globalInFlight > 0 {
		r.globalInFlight--
	}
}

func (r *Runtime) executeAuthorized(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal) (any, error) {
	return r.executeAuthorizedTraced(ctx, productID, fullName, tool, arguments, principal, nil, true)
}

func tracePhase(category, existing string) string {
	if category == "upstream_authentication_failed" && existing == "token_exchange" {
		return existing
	}
	switch category {
	case "upstream_authentication_failed":
		return "auth"
	case "transport_failed":
		return "transport"
	case "upstream_status":
		return "upstream_status"
	case "response_read_failed", "response_invalid":
		return "json"
	case "response_mapping_failed":
		return "response_mapping"
	case "response_schema_mismatch":
		return "output_schema"
	case "success":
		return "success"
	default:
		return "preflight"
	}
}

func (r *Runtime) executeAuthorizedTraced(ctx context.Context, productID, fullName string, tool model.Tool, arguments map[string]any, principal Principal, trace *executionTrace, recordAudit bool) (any, error) {
	if tool.BackendKind == "mcp" {
		if r.mcpExecutor == nil {
			return nil, errors.New("Stateless MCPv2 bridge is unavailable")
		}
		return r.mcpExecutor.ExecuteMCP(ctx, tool, arguments, principal)
	}
	var err error
	tool, err = prepareRuntimeTool(tool, principal.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if SchemaContainsSensitiveFields(tool.InputSchema) || SchemaContainsSensitiveFields(tool.OutputSchema) || ValueContainsSensitiveFields(arguments) {
		return nil, ErrDenied
	}
	method := strings.ToUpper(tool.HTTPMethod)
	var policy struct {
		IdempotencyRequired bool `json:"idempotency_required"`
	}
	if json.Unmarshal(tool.AuthorizationPolicy, &policy) != nil {
		return nil, ErrDenied
	}
	if principal.IdempotencyKey != "" && !validIdempotencyKey(principal.IdempotencyKey) {
		return nil, ErrInvalidIdempotencyKey
	}
	if method != http.MethodGet && policy.IdempotencyRequired && !validIdempotencyKey(principal.IdempotencyKey) {
		return nil, ErrInvalidIdempotencyKey
	}
	auditCategory, auditOutcome, auditStatusCode := "preflight_failed", "failure", 0
	defer func() {
		if trace != nil {
			trace.Category = auditCategory
			trace.Phase = tracePhase(auditCategory, trace.Phase)
			trace.StatusCode = auditStatusCode
		}
		if !recordAudit {
			return
		}
		current := map[string]any{"tool": fullName, "category": auditCategory}
		if auditStatusCode != 0 {
			current["status_code"] = auditStatusCode
		}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = r.store.AppendAudit(auditCtx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "tool.executed", TargetType: "tool", TargetID: tool.ID, Current: current, RequestID: principal.RequestID, Outcome: auditOutcome, CreatedAt: time.Now().UTC()})
	}()
	auditCategory = "rate_limited"
	if !r.acquireUpstreamSlot(productID, tool) {
		return nil, ErrRateLimited
	}
	defer r.releaseUpstreamSlot(productID, tool)
	if !r.allowUpstreamConnection(productID, tool) {
		return nil, ErrRateLimited
	}
	auditCategory = "unsafe_destination"
	parsed, address, err := r.safeDestination(ctx, tool.BaseURL)
	if err != nil {
		return nil, err
	}
	auditCategory = "configuration_invalid"
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		return nil, err
	}
	mapping, err := toolRequestMapping(tool)
	if err != nil {
		return nil, err
	}
	auditCategory = "request_mapping_failed"
	query := parsed.Query()
	headers := make(http.Header)
	bodyArguments := make(map[string]any)
	for key, value := range arguments {
		location := mapping.ParameterLocations[key]
		if location == "" {
			if method == http.MethodGet {
				location = "query"
			} else {
				location = "body"
			}
		}
		switch location {
		case "path":
			parsed.Path, err = applyPathArgument(parsed.Path, key, value)
			if err != nil {
				return nil, err
			}
		case "query":
			text, scalarErr := requestScalarText(value)
			if scalarErr != nil {
				return nil, scalarErr
			}
			query.Set(key, text)
		case "header":
			headerName := strings.ReplaceAll(key, "_", "-")
			headerValue, scalarErr := requestScalarText(value)
			if scalarErr != nil {
				return nil, scalarErr
			}
			if strings.ContainsAny(headerValue, "\r\n\x00") {
				return nil, errors.New("mapped request header value is invalid")
			}
			headers.Set(headerName, headerValue)
		case "body":
			if method == http.MethodGet {
				return nil, errors.New("GET tools cannot send a request body")
			}
			bodyArguments[key] = value
		default:
			return nil, ErrDenied
		}
	}
	if strings.ContainsAny(parsed.Path, "{}") {
		return nil, errors.New("required path argument is missing")
	}
	parsed.RawQuery = query.Encode()
	var body io.Reader
	if method != http.MethodGet && len(bodyArguments) > 0 {
		encoded, _ := json.Marshal(bodyArguments)
		body = bytes.NewReader(encoded)
	}
	if auth.Type == "api_key_query" {
		credential, credentialErr := r.toolCredential(ctx, tool)
		if credentialErr != nil {
			return nil, credentialErr
		}
		query := parsed.Query()
		query.Set(auth.QueryName, string(credential))
		parsed.RawQuery = query.Encode()
		wipe(credential)
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, errors.New("tool API request configuration is invalid")
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		request.Header[name] = append([]string(nil), values...)
	}
	auditCategory = "upstream_authentication_failed"
	switch auth.Type {
	case "delegated_oauth":
		if principal.DelegatedAPIOrigin == "" || principal.DelegatedAccessToken == "" {
			return nil, ErrDenied
		}
		if !sameOrigin(parsed.String(), principal.DelegatedAPIOrigin) {
			return nil, ErrUnsafeDestination
		}
		request.Header.Set("Authorization", "Bearer "+principal.DelegatedAccessToken)
	case "none":
	case "bearer", "authorization_scheme", "api_key_header", "basic", "custom_header":
		credential, credentialErr := r.toolCredential(ctx, tool)
		if credentialErr != nil {
			return nil, credentialErr
		}
		switch auth.Type {
		case "bearer":
			request.Header.Set("Authorization", "Bearer "+string(credential))
		case "authorization_scheme":
			request.Header.Set("Authorization", auth.Scheme+" "+string(credential))
		case "api_key_header":
			request.Header.Set(auth.HeaderName, prefixedCredential(auth.Prefix, credential))
		case "basic":
			request.SetBasicAuth(auth.Username, string(credential))
		case "custom_header":
			request.Header.Set(auth.HeaderName, prefixedCredential(auth.Prefix, credential))
		}
		wipe(credential)
	case "api_key_query":
	case "oauth_client_credentials":
		if trace != nil {
			trace.Phase = "token_exchange"
		}
		tokenType, token, tokenErr := r.oauthClientTokenTraced(ctx, tool, auth, trace)
		if tokenErr != nil {
			return nil, tokenErr
		}
		request.Header.Set("Authorization", tokenType+" "+token)
	default:
		return nil, ErrDenied
	}
	if method != http.MethodGet && policy.IdempotencyRequired {
		request.Header.Set("Idempotency-Key", upstreamIdempotencyKey(productID, tool, principal))
	}
	auditCategory = "transport_failed"
	if trace != nil {
		trace.NetworkCallPerformed = true
	}
	response, err := r.client(parsed, address, time.Duration(tool.TimeoutMS)*time.Millisecond).Do(request)
	if err != nil {
		return nil, errors.New("tool API request failed")
	}
	defer response.Body.Close()
	auditStatusCode = response.StatusCode
	auditCategory = "upstream_status"
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("tool API returned %s", response.Status)
	}
	auditCategory = "response_read_failed"
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(encoded) > 1<<20 {
		return nil, errors.New("tool API response exceeds the 1 MiB limit")
	}
	if trace != nil {
		trace.ResponseBytes = int64(len(encoded))
	}
	auditCategory = "response_invalid"
	var output any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		return nil, fmt.Errorf("tool API returned invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("tool API returned multiple JSON values")
	}
	if trace != nil {
		shape := jsonShape(output)
		trace.ResponseShape = &shape
	}
	auditCategory = "response_mapping_failed"
	responseMap, err := toolResponseMapping(tool)
	if err != nil {
		return nil, err
	}
	output, err = extractMappedResult(output, responseMap.ResultPath)
	if err != nil {
		return nil, err
	}
	if trace != nil {
		shape := jsonShapeForSchema(output, tool.OutputSchema)
		trace.ResponseShape = &shape
	}
	auditCategory = "response_schema_mismatch"
	object, ok := output.(map[string]any)
	if !ok {
		return nil, errors.New("tool output schema mismatch: response must resolve to an object")
	}
	if err := ValidateArguments(tool.OutputSchema, object); err != nil {
		return nil, fmt.Errorf("tool output schema mismatch: %w", err)
	}
	output = object
	auditCategory, auditOutcome = "success", "success"
	return output, nil
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func validationPaths(err error, rawSchema json.RawMessage) (string, string) {
	if err == nil {
		return "", ""
	}
	var currentSchema map[string]any
	if ValidateSchema(rawSchema) == nil {
		_ = json.Unmarshal(rawSchema, &currentSchema)
	}
	message := err.Error()
	if start := strings.Index(message, "arguments"); start >= 0 {
		message = message[start:]
	}
	end := strings.IndexByte(message, ' ')
	if end < 0 {
		end = len(message)
	}
	path := message[:end]
	if !strings.HasPrefix(path, "arguments") {
		return "", ""
	}
	path = strings.TrimPrefix(path, "arguments")
	instancePath, schemaPath := "", ""
	for len(path) > 0 {
		switch path[0] {
		case '.':
			path = path[1:]
			end = len(path)
			if index := strings.IndexAny(path, ".[ "); index >= 0 {
				end = index
			}
			name := path[:end]
			if name == "" {
				return instancePath, schemaPath
			}
			properties, _ := currentSchema["properties"].(map[string]any)
			childSchema, declared := properties[name].(map[string]any)
			if !declared {
				instancePath += "/[unexpected-property]"
				schemaPath += "/additionalProperties"
				return instancePath, schemaPath
			}
			if safeDeclaredShapeKey(name) {
				encoded := escapeJSONPointer(name)
				instancePath += "/" + encoded
				schemaPath += "/properties/" + encoded
			} else {
				instancePath += "/[schema-property]"
				schemaPath += "/properties/[schema-property]"
			}
			currentSchema = childSchema
			path = path[end:]
		case '[':
			closing := strings.IndexByte(path, ']')
			if closing < 2 {
				return instancePath, schemaPath
			}
			index := path[1:closing]
			if _, parseErr := strconv.Atoi(index); parseErr != nil {
				return instancePath, schemaPath
			}
			instancePath += "/" + index
			schemaPath += "/items"
			currentSchema, _ = currentSchema["items"].(map[string]any)
			path = path[closing+1:]
		default:
			return instancePath, schemaPath
		}
	}
	for _, keyword := range []string{"additionalProperties", "required", "minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems", "uniqueItems", "enum", "type"} {
		if strings.Contains(message, keyword) || keyword == "type" && strings.Contains(message, " must be ") {
			schemaPath += "/" + keyword
			break
		}
	}
	return instancePath, schemaPath
}

func testFailureFinding(trace executionTrace, err error, outputSchema json.RawMessage) model.ToolTestFinding {
	finding := model.ToolTestFinding{Phase: tracePhase(trace.Category, trace.Phase)}
	switch trace.Category {
	case "rate_limited":
		finding.Code, finding.Message = "rate_limited", "The upstream connection test rate limit was exceeded."
	case "unsafe_destination":
		finding.Code, finding.Message = "unsafe_destination", "The destination failed the network safety policy."
	case "configuration_invalid":
		finding.Code, finding.Message = "configuration_invalid", "The stored HTTP tool configuration is invalid."
	case "request_mapping_failed":
		finding.Code, finding.Message = "request_mapping_failed", "The request could not be constructed from the declared mapping."
	case "upstream_authentication_failed":
		if finding.Phase == "token_exchange" {
			finding.Code, finding.Message = "token_exchange_failed", "The OAuth client-credentials token exchange failed safely."
		} else {
			finding.Code, finding.Message = "upstream_authentication_failed", "The configured upstream authentication could not be applied."
		}
	case "transport_failed":
		finding.Code, finding.Message = "transport_failed", "The one-shot upstream request failed at the transport boundary."
	case "upstream_status":
		finding.Code, finding.Message = "upstream_status_rejected", "The upstream API returned a non-success status."
	case "response_read_failed":
		finding.Code, finding.Message = "response_size_or_read_failed", "The response could not be read within the 1 MiB safety limit."
	case "response_invalid":
		finding.Code, finding.Message = "invalid_json_response", "The upstream response was not exactly one valid JSON value."
	case "response_mapping_failed":
		finding.Code, finding.Message = "response_mapping_failed", "The declared response mapping did not resolve."
	case "response_schema_mismatch":
		finding.Code, finding.Message = "output_schema_mismatch", "The mapped response did not match the declared output schema."
		finding.InstancePath, finding.SchemaPath = validationPaths(err, outputSchema)
	default:
		finding.Phase = "preflight"
		finding.Code, finding.Message = "preflight_failed", "The tool test failed deterministic preflight validation."
	}
	return finding
}

// ExecuteHTTPDraftTest executes the supplied exact stored draft through the
// same hardened HTTP path as a published call. It deliberately skips public
// discovery and authorization lookup, never retries, and returns only
// sanitized evidence suitable for short-lived administrator diagnostics.
func (r *Runtime) ExecuteHTTPDraftTest(ctx context.Context, productID string, tool model.Tool, arguments map[string]any, principal Principal) DraftTestReport {
	started := time.Now()
	report := DraftTestReport{Outcome: "failure", Phase: "preflight", RequestShape: jsonShapeForSchema(arguments, tool.InputSchema), Findings: []model.ToolTestFinding{}}
	finish := func() DraftTestReport {
		report.DurationMS = time.Since(started).Milliseconds()
		if report.DurationMS < 0 {
			report.DurationMS = 0
		}
		return report
	}
	if tool.BackendKind != "http" {
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "http_tool_required", Message: "Live test runs are available only for stored HTTP tools."})
		return finish()
	}
	if arguments == nil {
		arguments = map[string]any{}
		report.RequestShape = jsonShapeForSchema(arguments, tool.InputSchema)
	}
	auth, err := toolUpstreamAuth(tool)
	if err != nil {
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "authentication_configuration_invalid", Message: "The stored upstream authentication configuration is invalid."})
		return finish()
	}
	report.AuthenticationType = auth.Type
	if err := ValidateArguments(tool.InputSchema, arguments); err != nil {
		instancePath, schemaPath := validationPaths(err, tool.InputSchema)
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "preflight", Code: "input_schema_mismatch", Message: "The supplied arguments did not match the declared input schema.", InstancePath: instancePath, SchemaPath: schemaPath})
		return finish()
	}
	if auth.Type == "delegated_oauth" {
		report.Phase = "auth"
		report.Findings = append(report.Findings, model.ToolTestFinding{Phase: "auth", Code: "test_authorization_unavailable", Message: "Delegated OAuth test authorization is required and cannot be supplied as raw token material."})
		return finish()
	}
	trace := executionTrace{Category: "preflight_failed", Phase: "preflight"}
	_, err = r.executeAuthorizedTraced(ctx, productID, tool.Namespace+"."+tool.Name, tool, arguments, principal, &trace, false)
	report.Phase = trace.Phase
	report.NetworkCallPerformed = trace.NetworkCallPerformed
	report.UpstreamStatusCode = trace.StatusCode
	report.ResponseBytes = trace.ResponseBytes
	report.ResponseShape = trace.ResponseShape
	if err != nil {
		report.Findings = append(report.Findings, testFailureFinding(trace, err, tool.OutputSchema))
		return finish()
	}
	report.Outcome, report.Phase = "success", "success"
	return finish()
}

// ExecuteBound executes one tool only when the selected Integration action is
// still the exact active point revision that was published with the tool.
func (r *Runtime) ExecuteBound(ctx context.Context, productID, fullName string, arguments map[string]any, principal Principal, binding BoundAuthorization) (any, error) {
	tool, err := r.find(ctx, productID, fullName)
	if err != nil {
		return nil, err
	}
	if tool.UpstreamDrifted {
		return nil, ErrDenied
	}
	if err := ValidateArguments(tool.InputSchema, arguments); err != nil {
		return nil, err
	}
	if err := authorize(tool, principal); err != nil {
		return nil, err
	}
	if err := r.authorizeBound(ctx, tool, binding, principal, true); err != nil {
		return nil, err
	}
	return r.executeAuthorized(ctx, productID, fullName, tool, arguments, principal)
}

func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
