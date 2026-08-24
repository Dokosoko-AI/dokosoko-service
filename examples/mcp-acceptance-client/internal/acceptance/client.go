package acceptance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type mcpClient struct {
	endpoint   string
	origin     string
	token      string
	httpClient *http.Client
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return clientWithoutRedirects(nil, timeout)
}

func clientWithoutRedirects(source *http.Client, timeout time.Duration) *http.Client {
	clone := &http.Client{}
	var base http.RoundTripper = http.DefaultTransport
	if source != nil {
		*clone = *source
		if source.Transport != nil {
			base = source.Transport
		}
	}
	clone.Transport = transportBoundary{base: base, loopback: newDirectLoopbackTransport()}
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if clone.Timeout <= 0 {
		clone.Timeout = timeout
	}
	return clone
}

type transportBoundary struct {
	base     http.RoundTripper
	loopback http.RoundTripper
}

func (transport transportBoundary) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme == "http" && isLoopbackHost(strings.TrimSuffix(strings.ToLower(request.URL.Hostname()), ".")) {
		return transport.loopback.RoundTrip(request)
	}
	return transport.base.RoundTrip(request)
}

func newDirectLoopbackTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A proxy would receive the bearer credential in cleartext. Loopback HTTP
	// therefore always uses a direct connection, independent of proxy-related
	// environment variables.
	transport.Proxy = nil
	transport.DialContext = dialVerifiedLoopback
	return transport
}

func dialVerifiedLoopback(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("loopback HTTP dial address was invalid")
	}
	canonicalHost := strings.TrimSuffix(strings.ToLower(host), ".")
	if !isLoopbackHost(canonicalHost) {
		return nil, errors.New("loopback HTTP transport refused a non-loopback host")
	}
	addresses := []net.IPAddr{}
	if ip := net.ParseIP(canonicalHost); ip != nil {
		addresses = append(addresses, net.IPAddr{IP: ip})
	} else {
		addresses, err = net.DefaultResolver.LookupIPAddr(ctx, canonicalHost)
		if err != nil {
			return nil, errors.New("loopback HTTP hostname could not be resolved")
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("loopback HTTP hostname did not resolve")
	}
	for _, candidate := range addresses {
		if !candidate.IP.IsLoopback() {
			return nil, errors.New("loopback HTTP hostname resolved to a non-loopback address")
		}
	}
	dialer := net.Dialer{}
	var lastErr error
	for _, candidate := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	return nil, fmt.Errorf("connect to verified loopback endpoint: %w", lastErr)
}

func validateEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("endpoint must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("endpoint must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must not contain user information, a query, or a fragment")
	}
	return nil
}

// validateSecureEndpoint applies the client's network boundary. HTTPS is
// always accepted. Plain HTTP is accepted only for a loopback hostname and an
// exact host:port authority named by the caller. Requiring an authority instead
// of a boolean keeps a local-development exception from silently applying to a
// different service.
func validateSecureEndpoint(raw string, allowedLoopbackHTTP []string) error {
	if err := validateEndpoint(raw); err != nil {
		return err
	}
	allowed, err := normalizeAllowedLoopbackHTTP(allowedLoopbackHTTP)
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(raw)
	if parsed.Scheme == "https" {
		return nil
	}
	authority, err := loopbackHTTPAuthority(parsed)
	if err != nil {
		return err
	}
	if _, ok := allowed[authority]; !ok {
		return fmt.Errorf("plain HTTP endpoint %q is disabled; explicitly allow the exact loopback authority %q for local development", parsed.Host, authority)
	}
	return nil
}

func normalizeAllowedLoopbackHTTP(values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("allowed loopback HTTP authority must not be empty")
		}
		host, port, err := net.SplitHostPort(value)
		if err != nil || host == "" || port == "" {
			return nil, fmt.Errorf("allowed loopback HTTP authority %q must be an exact host:port without a scheme or path", value)
		}
		canonical, err := canonicalLoopbackAuthority(host, port)
		if err != nil {
			return nil, fmt.Errorf("allowed loopback HTTP authority %q: %w", value, err)
		}
		result[canonical] = struct{}{}
	}
	return result, nil
}

func loopbackHTTPAuthority(parsed *url.URL) (string, error) {
	if parsed.Scheme != "http" {
		return "", errors.New("endpoint must use HTTPS or explicitly allowed loopback HTTP")
	}
	if parsed.Port() == "" {
		return "", errors.New("a loopback HTTP endpoint must include an explicit port")
	}
	return canonicalLoopbackAuthority(parsed.Hostname(), parsed.Port())
}

func canonicalLoopbackAuthority(host, port string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if !isLoopbackHost(host) {
		return "", errors.New("host must be localhost, a .localhost name, or a loopback IP literal")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", errors.New("port must be a number from 1 through 65535")
	}
	return net.JoinHostPort(host, strconv.FormatUint(portNumber, 10)), nil
}

func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func randomID(prefix string) (string, error) {
	value, err := randomBytes(18)
	if err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	return value, nil
}

func (c mcpClient) call(ctx context.Context, method, name string, arguments map[string]any, confirmed bool) callOutcome {
	started := time.Now()
	requestID, err := randomID("mcpacc_")
	if err != nil {
		return callOutcome{TransportError: err, Duration: time.Since(started)}
	}
	meta := map[string]any{"io.modelcontextprotocol/protocolVersion": ProtocolVersion}
	params := map[string]any{"_meta": meta}
	if method == "tools/call" {
		params["name"] = name
		if arguments == nil {
			arguments = map[string]any{}
		}
		params["arguments"] = arguments
		meta["confirmed"] = confirmed
	}
	return c.callRaw(ctx, requestID, method, name, params, started)
}

func (c mcpClient) callRaw(ctx context.Context, requestID, method, name string, params map[string]any, started time.Time) callOutcome {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return callOutcome{RequestID: requestID, TransportError: err, Duration: time.Since(started)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return callOutcome{RequestID: requestID, TransportError: err, Duration: time.Since(started)}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	req.Header.Set("Mcp-Method", method)
	req.Header.Set("X-Request-ID", requestID)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	if c.origin != "" {
		req.Header.Set("Origin", strings.TrimRight(c.origin, "/"))
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return callOutcome{RequestID: requestID, TransportError: err, Duration: time.Since(started)}
	}
	defer response.Body.Close()
	outcome := callOutcome{
		RequestID:         requestID,
		ResponseRequestID: response.Header.Get("X-Request-ID"),
		HTTPStatus:        response.StatusCode,
		Duration:          time.Since(started),
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		outcome.TransportError = err
		return outcome
	}
	if len(body) > maxResponseBytes {
		outcome.TransportError = errors.New("response exceeded 4 MiB")
		return outcome
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Deliberately do not include the response body: an untrusted endpoint
		// must not be able to reflect credentials into logs or reports.
		outcome.TransportError = fmt.Errorf("HTTP status %d", response.StatusCode)
		return outcome
	}
	if err := json.Unmarshal(body, &outcome.Response); err != nil {
		outcome.TransportError = errors.New("response was not a JSON-RPC object")
	}
	return outcome
}

func outcomeCheck(name string, outcome callOutcome, expectErrorCode *int) Check {
	check := Check{
		Name:              name,
		Required:          true,
		RequestID:         outcome.RequestID,
		ResponseRequestID: outcome.ResponseRequestID,
		HTTPStatus:        outcome.HTTPStatus,
		DurationMS:        outcome.Duration.Milliseconds(),
	}
	if outcome.TransportError != nil {
		check.Status = Fail
		check.Detail = outcome.TransportError.Error()
		return check
	}
	if outcome.ResponseRequestID != "" && outcome.ResponseRequestID != outcome.RequestID {
		check.Status = Fail
		check.Detail = "response X-Request-ID did not match the request"
		return check
	}
	if outcome.Response.JSONRPC != "2.0" {
		check.Status = Fail
		check.Detail = "response did not declare JSON-RPC 2.0"
		return check
	}
	var responseID string
	if json.Unmarshal(outcome.Response.ID, &responseID) != nil || responseID != outcome.RequestID {
		check.Status = Fail
		check.Detail = "response id did not match request id"
		return check
	}
	if outcome.Response.Error != nil {
		code := outcome.Response.Error.Code
		check.RPCErrorCode = &code
	}
	if expectErrorCode != nil {
		if outcome.Response.Error == nil || outcome.Response.Error.Code != *expectErrorCode {
			check.Status = Fail
			check.Detail = fmt.Sprintf("expected JSON-RPC error %d", *expectErrorCode)
			return check
		}
		check.Status = Pass
		check.Detail = fmt.Sprintf("received expected JSON-RPC error %d", *expectErrorCode)
		return check
	}
	if outcome.Response.Error != nil {
		check.Status = Fail
		check.Detail = fmt.Sprintf("received JSON-RPC error %d", outcome.Response.Error.Code)
		return check
	}
	if len(outcome.Response.Result) == 0 || bytes.Equal(outcome.Response.Result, []byte("null")) {
		check.Status = Fail
		check.Detail = "response did not contain a result"
		return check
	}
	check.Status = Pass
	return check
}
