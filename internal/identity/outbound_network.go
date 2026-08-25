package identity

import (
	"context"
	"crypto/hmac"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
)

type IPResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

func registeredRedirectMatches(values []string, requested string) bool {
	for _, value := range values {
		if hmac.Equal([]byte(value), []byte(requested)) {
			return true
		}
		registeredURL, registeredErr := url.Parse(value)
		requestedURL, requestedErr := url.Parse(requested)
		if registeredErr != nil || requestedErr != nil || registeredURL.Scheme != "http" || requestedURL.Scheme != "http" {
			continue
		}
		registeredIP := net.ParseIP(registeredURL.Hostname())
		requestedIP := net.ParseIP(requestedURL.Hostname())
		if registeredIP == nil || requestedIP == nil || !registeredIP.IsLoopback() || !requestedIP.IsLoopback() || !strings.EqualFold(registeredURL.Hostname(), requestedURL.Hostname()) {
			continue
		}
		if registeredURL.EscapedPath() == requestedURL.EscapedPath() && registeredURL.RawQuery == requestedURL.RawQuery && registeredURL.Fragment == requestedURL.Fragment {
			return true
		}
	}
	return false
}

// IsLocalDevelopmentHostname recognizes the RFC-reserved localhost namespace
// without accepting lookalike public suffixes such as localhost.example.com.
func IsLocalDevelopmentHostname(hostname string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || strings.HasSuffix(hostname, ".localhost")
}

func resolveSafeOIDCDestination(ctx context.Context, parsed *url.URL, resolver IPResolver, allowLocal bool, localBoundary []net.IP) ([]net.IP, error) {
	if parsed == nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("destination must be an absolute credential-free URL")
	}
	localDevelopment := IsLocalDevelopmentHostname(parsed.Hostname())
	if localDevelopment && !allowLocal {
		return nil, errors.New("public issuer metadata cannot select a local endpoint")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && localDevelopment) {
		return nil, errors.New("destination must use HTTPS, or HTTP only for local development")
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return nil, errors.New("destination port is invalid")
		}
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("destination did not resolve safely")
	}
	for _, address := range addresses {
		if localDevelopment && !netpolicy.LocalDevelopmentIP(address) || !localDevelopment && netpolicy.UnsafeIP(address) {
			return nil, errors.New("destination resolves outside its permitted network boundary")
		}
		if localDevelopment && len(localBoundary) > 0 && !ipInBoundary(address, localBoundary) {
			return nil, errors.New("local OIDC metadata endpoint leaves the configured issuer boundary")
		}
	}
	return addresses, nil
}

func ipInBoundary(address net.IP, boundary []net.IP) bool {
	for _, permitted := range boundary {
		if address.Equal(permitted) {
			return true
		}
	}
	return false
}

type oidcSafeTransport struct {
	resolver      IPResolver
	delegate      http.RoundTripper
	allowLocal    bool
	localBoundary []net.IP
}

func (t oidcSafeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	addresses, err := resolveSafeOIDCDestination(request.Context(), request.URL, t.resolver, t.allowLocal, t.localBoundary)
	if err != nil {
		return nil, err
	}
	if t.delegate != nil {
		return t.delegate.RoundTrip(request)
	}
	port := request.URL.Port()
	if port == "" {
		if request.URL.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 10 * time.Second,
		DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), port))
		},
	}
	if request.URL.Scheme == "https" {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: request.URL.Hostname()}
	}
	return transport.RoundTrip(request)
}

func safeOIDCClient(ctx context.Context, issuer *url.URL, provided *http.Client, resolver IPResolver) (*http.Client, []net.IP, error) {
	issuerLocal := issuer != nil && IsLocalDevelopmentHostname(issuer.Hostname())
	issuerAddresses, err := resolveSafeOIDCDestination(ctx, issuer, resolver, issuerLocal, nil)
	if err != nil {
		return nil, nil, err
	}
	var localBoundary []net.IP
	if issuerLocal {
		localBoundary = append(localBoundary, issuerAddresses...)
	}
	result := &http.Client{Timeout: 10 * time.Second}
	var delegate http.RoundTripper
	if provided != nil {
		clone := *provided
		result = &clone
		delegate = provided.Transport
		if delegate == nil {
			delegate = http.DefaultTransport
		}
		if result.Timeout <= 0 {
			result.Timeout = 10 * time.Second
		}
	}
	result.Transport = oidcSafeTransport{resolver: resolver, delegate: delegate, allowLocal: issuerLocal, localBoundary: localBoundary}
	result.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return result, localBoundary, nil
}

func safeClient(ctx context.Context, parsed *url.URL, provided *http.Client, resolver IPResolver) (*http.Client, error) {
	localDevelopment := IsLocalDevelopmentHostname(parsed.Hostname())
	if parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || (localDevelopment && parsed.Scheme != "http") || (!localDevelopment && (parsed.Scheme != "https" || (parsed.Port() != "" && parsed.Port() != "443"))) {
		return nil, errors.New("destination must be credential-free HTTPS on port 443")
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("destination did not resolve safely")
	}
	for _, address := range addresses {
		if (localDevelopment && !netpolicy.LocalDevelopmentIP(address)) || (!localDevelopment && netpolicy.UnsafeIP(address)) {
			return nil, errors.New("destination resolves to a non-public address")
		}
	}
	if provided != nil {
		return provided, nil
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	port := parsed.Port()
	if port == "" {
		port = "443"
		if localDevelopment {
			port = "80"
		}
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, ResponseHeaderTimeout: 10 * time.Second, DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), port))
	}}
	if !localDevelopment {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// SafeOutboundClient returns a DNS-pinned, redirect-disabled HTTPS client for every
// vendor-controlled or client-metadata destination.
func SafeOutboundClient(ctx context.Context, parsed *url.URL, client *http.Client, resolver IPResolver) (*http.Client, error) {
	return safeClient(ctx, parsed, client, resolver)
}
