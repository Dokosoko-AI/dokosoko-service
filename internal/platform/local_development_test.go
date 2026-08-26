package platform

import "testing"

func TestLocalhostSubdomainURLPolicy(t *testing.T) {
	if !validHTTPSOrigin("http://api.complicatedauth.localhost:38080") {
		t.Fatal("localhost-subdomain issuer was rejected")
	}
	if !validHTTPSBaseOrigin("http://api.complicatedauth.localhost:38080") {
		t.Fatal("localhost-subdomain API origin was rejected")
	}
	if !validHTTPSURI("http://api.complicatedauth.localhost:38080/resource") {
		t.Fatal("localhost-subdomain resource URI was rejected")
	}
	if !validOAuthRedirect("http://api.dokosoko.localhost:8080/oauth/callback") {
		t.Fatal("localhost-subdomain OAuth redirect was rejected")
	}
	if !validToolEndpoint("http://api.complicatedauth.localhost:38080/health/ready") {
		t.Fatal("localhost-subdomain tool endpoint was rejected")
	}
	for _, raw := range []string{"http://localhost.example:8080", "http://complicatedauth.localhost.example:8080"} {
		if validHTTPSBaseOrigin(raw) {
			t.Fatalf("lookalike local origin %q was accepted", raw)
		}
		if validToolEndpoint(raw + "/health/ready") {
			t.Fatalf("lookalike tool endpoint %q was accepted", raw)
		}
	}
}
