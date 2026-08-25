package tools

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
)

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
		if netpolicy.UnsafeIP(address) {
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
