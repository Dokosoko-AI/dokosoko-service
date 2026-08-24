package identity

import (
	"context"
	"net"
	"net/url"
	"testing"
)

type fixedIPResolver []net.IP

func (r fixedIPResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP(r), nil
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
