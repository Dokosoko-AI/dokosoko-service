package tools

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/ratelimit"
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
	CreateAuthorizationUsageEvent(context.Context, model.AuthorizationUsageEvent) (model.AuthorizationUsageEvent, error)
	ClaimAuthorizationUsageEvents(context.Context, string, time.Time, int) ([]model.AuthorizationUsageEvent, error)
	CompleteAuthorizationUsageEvent(context.Context, string, string, time.Time) error
	RetryAuthorizationUsageEvent(context.Context, string, string, time.Time, string) error
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
	Headers                 []string `json:"headers,omitempty"`
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

type Runtime struct {
	store                        Store
	resolver                     Resolver
	doer                         Doer
	executorMu                   sync.RWMutex
	executors                    map[string]BackendExecutor
	credentials                  CredentialResolver
	privateLocalhostDestinations map[string]struct{}
	tokenMu                      sync.Mutex
	tokens                       map[string]cachedOAuthToken
	tokenFlights                 map[string]*oauthTokenFlight
	rateLimiter                  *ratelimit.FixedWindow
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

// BackendExecutor is the common runtime dispatch contract. Authorization,
// schema validation, confirmation and publication checks happen before it is
// called. Executors may apply additional backend-specific restrictions.
type BackendExecutor interface {
	Execute(context.Context, model.Tool, map[string]any, Principal) (any, error)
	Available(model.Tool) bool
}

type NativeExecutor interface {
	ExecuteNative(context.Context, model.Tool, map[string]any, Principal) (any, error)
	AvailableNative(model.Tool) bool
}

type MCPCallResult struct {
	Result map[string]any
}

func NewRuntime(store Store, resolver Resolver, doer Doer) *Runtime {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	runtime := &Runtime{store: store, resolver: resolver, doer: doer, executors: make(map[string]BackendExecutor), tokens: make(map[string]cachedOAuthToken), tokenFlights: make(map[string]*oauthTokenFlight), rateLimiter: ratelimit.NewFixedWindow(upstreamConnectionWindow, maxUpstreamRateWindows), connectionInFlight: make(map[string]int), now: func() time.Time { return time.Now().UTC() }}
	runtime.executors["http"] = backendExecutor{execute: runtime.executeHTTPAuthorized, available: func(model.Tool) bool { return true }}
	return runtime
}

func (r *Runtime) RegisterExecutor(kind string, executor BackendExecutor) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || executor == nil {
		return
	}
	r.executorMu.Lock()
	defer r.executorMu.Unlock()
	r.executors[kind] = executor
}

func (r *Runtime) SetMCPExecutor(executor MCPExecutor) {
	if executor == nil {
		return
	}
	r.RegisterExecutor("mcp", backendExecutor{execute: func(ctx context.Context, tool model.Tool, arguments map[string]any, principal Principal) (any, error) {
		return executor.ExecuteMCP(ctx, tool, arguments, principal)
	}, available: func(model.Tool) bool { return true }})
}

func (r *Runtime) SetNativeExecutor(executor NativeExecutor) {
	if executor == nil {
		return
	}
	r.RegisterExecutor("native", backendExecutor{execute: executor.ExecuteNative, available: executor.AvailableNative})
}

type backendExecutor struct {
	execute   func(context.Context, model.Tool, map[string]any, Principal) (any, error)
	available func(model.Tool) bool
}

func (e backendExecutor) Execute(ctx context.Context, tool model.Tool, arguments map[string]any, principal Principal) (any, error) {
	return e.execute(ctx, tool, arguments, principal)
}
func (e backendExecutor) Available(tool model.Tool) bool {
	return e.available == nil || e.available(tool)
}

func (r *Runtime) executor(tool model.Tool) (BackendExecutor, bool) {
	r.executorMu.RLock()
	defer r.executorMu.RUnlock()
	executor, ok := r.executors[strings.ToLower(strings.TrimSpace(tool.BackendKind))]
	return executor, ok && executor.Available(tool)
}
func (r *Runtime) SetCredentialResolver(resolver CredentialResolver) { r.credentials = resolver }

// SetPrivateLocalhostHosts configures the exact development destinations that
// may resolve to loopback or private addresses. Entries are hostname:port; a
// host-only legacy entry grants only the HTTP default port 80, never every
// local service listening on that hostname.
