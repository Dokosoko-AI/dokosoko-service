package tools

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

var (
	ErrDenied            = errors.New("tool execution denied by authorization policy")
	ErrConfirmation      = errors.New("tool execution requires explicit confirmation")
	ErrUnsafeDestination = errors.New("tool destination is not safe")
)

type Store interface {
	Tools(context.Context, string, bool) ([]model.Tool, error)
	AppendAudit(context.Context, model.AuditEvent) error
}

type Principal struct {
	Subject              string
	Issuer               string
	CustomerAccountID    string
	ExternalCustomerID   string
	InstallationID       string
	Grants               map[string]bool
	DelegatedAPIOrigin   string
	DelegatedAccessToken string
	Confirmed            bool
	RequestID            string
}

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}
type Doer interface {
	Do(*http.Request) (*http.Response, error)
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

type Runtime struct {
	store       Store
	resolver    Resolver
	doer        Doer
	mcpExecutor MCPExecutor
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
	return &Runtime{store: store, resolver: resolver, doer: doer}
}

func (r *Runtime) SetMCPExecutor(executor MCPExecutor) { r.mcpExecutor = executor }

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
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return nil, nil, ErrUnsafeDestination
	}
	addresses, err := r.resolver.LookupIP(ctx, "ip", parsed.Hostname())
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

func (r *Runtime) client(parsed *url.URL, address net.IP, timeout time.Duration) Doer {
	if r.doer != nil {
		return r.doer
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), "443"))
	}, DisableCompression: true, ResponseHeaderTimeout: timeout}
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
	if tool.BackendKind == "mcp" {
		if r.mcpExecutor == nil {
			return nil, errors.New("Stateless MCPv2 bridge is unavailable")
		}
		return r.mcpExecutor.ExecuteMCP(ctx, tool, arguments, principal)
	}
	parsed, address, err := r.safeDestination(ctx, tool.BaseURL)
	if err != nil {
		return nil, err
	}
	if principal.DelegatedAPIOrigin == "" || principal.DelegatedAccessToken == "" {
		return nil, ErrDenied
	}
	if !sameOrigin(parsed.String(), principal.DelegatedAPIOrigin) {
		return nil, ErrUnsafeDestination
	}
	method := strings.ToUpper(tool.HTTPMethod)
	var body io.Reader
	if method == http.MethodGet {
		query := parsed.Query()
		for key, value := range arguments {
			query.Set(key, fmt.Sprint(value))
		}
		parsed.RawQuery = query.Encode()
	} else {
		encoded, _ := json.Marshal(arguments)
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+principal.DelegatedAccessToken)
	response, err := r.client(parsed, address, time.Duration(tool.TimeoutMS)*time.Millisecond).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("tool API returned %s", response.Status)
	}
	var output any
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		return nil, fmt.Errorf("tool API returned invalid JSON: %w", err)
	}
	if object, ok := output.(map[string]any); ok {
		// Re-marshal normalizes json.Number for the schema validator's numeric checks.
		normalized, _ := json.Marshal(object)
		_ = json.Unmarshal(normalized, &object)
		if err := ValidateArguments(tool.OutputSchema, object); err != nil {
			return nil, fmt.Errorf("tool output schema mismatch: %w", err)
		}
		output = object
	}
	_ = r.store.AppendAudit(ctx, model.AuditEvent{ID: auditID(), OrganisationID: tool.OrganisationID, ProductID: productID, ActorID: principal.Subject, Action: "tool.executed", TargetType: "tool", TargetID: tool.ID, Current: map[string]any{"tool": fullName, "status": response.StatusCode}, RequestID: principal.RequestID, CreatedAt: time.Now().UTC()})
	return output, nil
}

func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
