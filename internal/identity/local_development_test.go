package identity

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

type fixedIPResolver []net.IP

func (r fixedIPResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP(r), nil
}

type hostnameIPResolver map[string][]net.IP

func (r hostnameIPResolver) LookupIP(_ context.Context, _, hostname string) ([]net.IP, error) {
	addresses, ok := r[hostname]
	if !ok {
		return nil, errors.New("unexpected hostname")
	}
	return addresses, nil
}

type localRoundTripper func(*http.Request) (*http.Response, error)

func (f localRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSafeOutboundClientLocalhostSubdomainPolicy(t *testing.T) {
	localURL, _ := url.Parse("http://api.complicatedauth.localhost:38080/v1/access/evaluations")
	if _, err := SafeOutboundClient(context.Background(), localURL, nil, fixedIPResolver{net.ParseIP("192.168.65.254")}); err != nil {
		t.Fatalf("localhost-subdomain private development destination was rejected: %v", err)
	}
	if _, err := SafeOutboundClient(context.Background(), localURL, nil, fixedIPResolver{net.ParseIP("203.0.113.10")}); err == nil {
		t.Fatal("localhost-subdomain destination resolving publicly was accepted")
	}
	lookalike, _ := url.Parse("http://api.complicatedauth.localhost.example:38080")
	if _, err := SafeOutboundClient(context.Background(), lookalike, nil, fixedIPResolver{net.ParseIP("192.168.65.254")}); err == nil {
		t.Fatal("lookalike localhost hostname was accepted")
	}
	production, _ := url.Parse("https://api.vendor.example")
	if _, err := SafeOutboundClient(context.Background(), production, nil, fixedIPResolver{net.ParseIP("192.168.65.254")}); err == nil {
		t.Fatal("production destination resolving privately was accepted")
	}
}

func TestSafeOIDCClientPinsLocalMetadataEndpointsToIssuerAddress(t *testing.T) {
	issuer, _ := url.Parse("http://issuer.oidc.localhost:38080/tenant")
	resolver := hostnameIPResolver{
		"issuer.oidc.localhost":    {net.ParseIP("192.168.65.254")},
		"authorize.oidc.localhost": {net.ParseIP("192.168.65.254")},
		"token.oidc.localhost":     {net.ParseIP("192.168.65.253")},
	}
	delegated := 0
	provided := &http.Client{Transport: localRoundTripper(func(request *http.Request) (*http.Response, error) {
		delegated++
		return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody, Request: request}, nil
	})}
	client, boundary, err := safeOIDCClient(context.Background(), issuer, provided, resolver)
	if err != nil || len(boundary) != 1 || !boundary[0].Equal(net.ParseIP("192.168.65.254")) {
		t.Fatalf("local issuer boundary = %v, err=%v", boundary, err)
	}

	allowed, _ := http.NewRequest(http.MethodGet, "http://authorize.oidc.localhost:38081/authorize", nil)
	response, err := client.Do(allowed)
	if err != nil {
		t.Fatalf("same-boundary local metadata endpoint was rejected: %v", err)
	}
	response.Body.Close()
	if delegated != 1 {
		t.Fatalf("same-boundary request delegate calls = %d", delegated)
	}

	blocked, _ := http.NewRequest(http.MethodPost, "http://token.oidc.localhost:38082/oauth/token", nil)
	if response, err = client.Do(blocked); err == nil {
		response.Body.Close()
		t.Fatal("different-private-address local metadata endpoint was accepted")
	}
	if delegated != 1 {
		t.Fatalf("blocked request reached delegate; calls = %d", delegated)
	}
}
