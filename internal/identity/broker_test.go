package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type fakeUpstream struct{}

func (fakeUpstream) AuthorizationURL(_ context.Context, _ identity.VendorConfig, state, nonce, challenge, _ string) (string, error) {
	values := url.Values{"state": {state}, "nonce": {nonce}, "challenge": {challenge}}
	return "https://idp.example/authorize?" + values.Encode(), nil
}

func (fakeUpstream) ExchangeAndVerify(_ context.Context, config identity.VendorConfig, code, verifier, nonce, _, _ string) (identity.Claims, string, error) {
	if code != "vendor-code" || verifier == "" || nonce == "" {
		return identity.Claims{}, "", errors.New("invalid upstream exchange")
	}
	return identity.Claims{Issuer: config.Issuer, Subject: "vendor-user-42", Email: "alex@example.com", DisplayName: "Alex", VendorOrganisation: "vendor-org-7"}, "vendor-access", nil
}

type fakeEntitlements struct{ failure error }

func (f fakeEntitlements) Resolve(_ context.Context, _ identity.VendorConfig, claims identity.Claims, token string) (map[string]bool, error) {
	if f.failure != nil {
		return nil, f.failure
	}
	if claims.Subject != "vendor-user-42" || token != "vendor-access" {
		return nil, errors.New("identity was not forwarded")
	}
	return map[string]bool{"developer.pro": true, "admin": false}, nil
}

func configuredMemory(t *testing.T) *store.Memory {
	t.Helper()
	memory := store.NewMemory()
	_, err := memory.SaveVendorIdentity(context.Background(), identity.VendorConfig{ID: "idp-1", OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: "https://idp.example", ClientID: "upstream-client", Scopes: []string{"openid"}, AllowedRedirectURIs: []string{"https://client.example/callback"}})
	if err != nil {
		t.Fatal(err)
	}
	return memory
}

func challenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestBrokerOAuthPKCEAndProductBinding(t *testing.T) {
	memory := configuredMemory(t)
	broker := identity.NewBroker(memory, nil, "https://doko.example", fakeUpstream{}, fakeEntitlements{})
	verifier := strings.Repeat("v", 48)
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: "prod_acme", ClientID: "prod_acme", RedirectURI: "https://client.example/callback", State: "client-state", CodeChallenge: challenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(upstreamURL)
	result, err := broker.Callback(context.Background(), "prod_acme", parsed.Query().Get("state"), "vendor-code")
	if err != nil {
		t.Fatal(err)
	}
	downstream, _ := url.Parse(result.RedirectURI)
	if downstream.Query().Get("state") != "client-state" || downstream.Query().Get("code") == "" {
		t.Fatalf("invalid downstream callback: %s", result.RedirectURI)
	}
	token, err := broker.Exchange(context.Background(), downstream.Query().Get("code"), verifier, "prod_acme", "https://client.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := broker.Authenticate(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ProductID != "prod_acme" || principal.Subject != "vendor-user-42" || !principal.Entitlements["developer.pro"] || principal.Entitlements["admin"] {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	if _, err := broker.Exchange(context.Background(), downstream.Query().Get("code"), verifier, "prod_acme", "https://client.example/callback"); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("authorization code was reusable: %v", err)
	}
}

func TestBrokerRejectsUnregisteredRedirectAndFailsClosed(t *testing.T) {
	memory := configuredMemory(t)
	broker := identity.NewBroker(memory, nil, "https://doko.example", fakeUpstream{}, fakeEntitlements{failure: errors.New("vendor unavailable")})
	verifier := strings.Repeat("v", 48)
	if _, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: "prod_acme", ClientID: "prod_acme", RedirectURI: "https://attacker.example/callback", State: "state", CodeChallenge: challenge(verifier)}); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("unregistered redirect accepted: %v", err)
	}
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: "prod_acme", ClientID: "prod_acme", RedirectURI: "https://client.example/callback", State: "state", CodeChallenge: challenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(upstreamURL)
	if _, err := broker.Callback(context.Background(), "prod_acme", parsed.Query().Get("state"), "vendor-code"); err == nil {
		t.Fatal("entitlement failure did not fail closed")
	}
}

type resolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f resolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestHookEntitlementsRejectsPrivateResolution(t *testing.T) {
	hook := identity.HookEntitlements{Resolver: resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})}
	_, err := hook.Resolve(context.Background(), identity.VendorConfig{ProductID: "prod", EntitlementHookURL: "https://hooks.example/entitlements"}, identity.Claims{Subject: "user"}, "vendor-token")
	if err == nil {
		t.Fatal("private entitlement hook resolution was accepted")
	}
}

func TestHookEntitlementsUsesVendorBearerAndBoundPayload(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if request.Header.Get("Authorization") != "Bearer vendor-token" || !strings.Contains(string(body), `"product_id":"prod"`) {
			t.Fatalf("unexpected hook request: headers=%v body=%s", request.Header, body)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"entitlements":{"feature.x":true}}`))}, nil
	})}
	hook := identity.HookEntitlements{Client: client, Resolver: resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})}
	values, err := hook.Resolve(context.Background(), identity.VendorConfig{ProductID: "prod", EntitlementHookURL: "https://hooks.example/entitlements"}, identity.Claims{Subject: "user"}, "vendor-token")
	if err != nil || !values["feature.x"] {
		t.Fatalf("entitlements = %#v, err = %v", values, err)
	}
}

func TestOperationAuthorizationUsesServiceCredentialAndRedactsValues(t *testing.T) {
	ctx := context.Background()
	memory := configuredMemory(t)
	vault, _ := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	encrypted, _ := vault.Encrypt([]byte("authorization-service-secret"), "org_acme:authorization:auth-secret")
	_, err := memory.CreateSecret(ctx, model.Secret{ID: "auth-secret", OrganisationID: "org_acme", Name: "auth-secret", Purpose: "vendor_authorization", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	config, _ := memory.VendorIdentity(ctx, "prod_acme")
	config.AuthorizationHookURL = "https://hooks.example/authorize"
	config.AuthorizationCredentialID = "auth-secret"
	if _, err := memory.SaveVendorIdentity(ctx, config); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if request.Header.Get("Authorization") != "Bearer authorization-service-secret" || !strings.Contains(string(body), `"argument_keys":["api_key","region"]`) || strings.Contains(string(body), "sensitive-value") {
			t.Fatalf("unsafe authorization request: headers=%v body=%s", request.Header, body)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"allowed":true}`))}, nil
	})}
	hook := identity.NewHookAuthorization(memory, vault)
	hook.Client = client
	hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	err = hook.Authorize(ctx, "prod_acme", model.Tool{Namespace: "projects", Name: "create"}, map[string]any{"api_key": "sensitive-value", "region": "nz"}, toolruntime.Principal{Subject: "issuer|subject", VendorOrganisation: "vendor-org"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUsageHookProxiesCustomerDefinedScalarValues(t *testing.T) {
	ctx := context.Background()
	memory := configuredMemory(t)
	vault, _ := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	encrypted, _ := vault.Encrypt([]byte("usage-service-secret"), "org_acme:usage:usage-secret")
	_, err := memory.CreateSecret(ctx, model.Secret{ID: "usage-secret", OrganisationID: "org_acme", Name: "usage-secret", Purpose: "vendor_usage", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	config, _ := memory.VendorIdentity(ctx, "prod_acme")
	config.UsageHookURL = "https://hooks.example/usage"
	config.UsageCredentialID = "usage-secret"
	if _, err := memory.SaveVendorIdentity(ctx, config); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		requestBody := string(body)
		if request.Header.Get("Authorization") != "Bearer usage-service-secret" ||
			!strings.Contains(requestBody, `"product_id":"prod_acme"`) ||
			!strings.Contains(requestBody, `"subject":"https://idp.example|vendor-user-42"`) ||
			!strings.Contains(requestBody, `"vendor_organisation_id":"vendor-org-7"`) {
			t.Fatalf("unexpected usage request: headers=%v body=%s", request.Header, body)
		}
		response := `{"as_of":"2026-08-19T05:30:00Z","items":[{"key":"credits_used","label":"Used","value":720,"format":"number","unit":"credits"},{"key":"next_renewal","label":"Next renewal","value":"2026-09-01T00:00:00Z","format":"datetime"},{"key":"trial_active","label":"Trial active","value":true}]}`
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	hook := identity.NewHookUsage(memory, vault)
	hook.Client = client
	hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	report, err := hook.Report(ctx, "prod_acme", identity.Principal{Issuer: "https://idp.example", Subject: "vendor-user-42", VendorOrganisation: "vendor-org-7"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(report)
	if len(report.Items) != 3 || !strings.Contains(string(encoded), `"value":720`) || !strings.Contains(string(encoded), `"value":true`) {
		t.Fatalf("usage report was not proxied: %s", encoded)
	}
}

func TestUsageHookRejectsInvalidReportsAndDenial(t *testing.T) {
	ctx := context.Background()
	memory := configuredMemory(t)
	vault, _ := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	encrypted, _ := vault.Encrypt([]byte("usage-service-secret"), "org_acme:usage:usage-secret")
	_, _ = memory.CreateSecret(ctx, model.Secret{ID: "usage-secret", OrganisationID: "org_acme", Name: "usage-secret", Purpose: "vendor_usage", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint})
	config, _ := memory.VendorIdentity(ctx, "prod_acme")
	config.UsageHookURL, config.UsageCredentialID = "https://hooks.example/usage", "usage-secret"
	_, _ = memory.SaveVendorIdentity(ctx, config)

	for name, response := range map[string]string{
		"duplicate keys": `{"as_of":"2026-08-19T05:30:00Z","items":[{"key":"used","label":"Used","value":1},{"key":"used","label":"Again","value":2}]}`,
		"nested value":   `{"as_of":"2026-08-19T05:30:00Z","items":[{"key":"used","label":"Used","value":{"raw":1}}]}`,
		"unknown field":  `{"as_of":"2026-08-19T05:30:00Z","items":[],"account_secret":"must-not-pass"}`,
		"invalid time":   `{"as_of":"today","items":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			hook := identity.NewHookUsage(memory, vault)
			hook.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
			})}
			hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
			if _, err := hook.Report(ctx, "prod_acme", identity.Principal{Subject: "user"}); !errors.Is(err, identity.ErrUsageUpstream) {
				t.Fatalf("error = %v, want ErrUsageUpstream", err)
			}
		})
	}

	hook := identity.NewHookUsage(memory, vault)
	hook.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"denied"}`))}, nil
	})}
	hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	if _, err := hook.Report(ctx, "prod_acme", identity.Principal{Subject: "user"}); !errors.Is(err, identity.ErrUsageDenied) {
		t.Fatalf("error = %v, want ErrUsageDenied", err)
	}

	hook = identity.NewHookUsage(memory, vault)
	hook.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("usage request reached a privately resolved destination")
		return nil, nil
	})}
	hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	if _, err := hook.Report(ctx, "prod_acme", identity.Principal{Subject: "user"}); !errors.Is(err, identity.ErrUsageUpstream) {
		t.Fatalf("private destination error = %v, want ErrUsageUpstream", err)
	}

	hook = identity.NewHookUsage(memory, vault)
	hook.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", (1<<20)+1)))}, nil
	})}
	hook.Resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	if _, err := hook.Report(ctx, "prod_acme", identity.Principal{Subject: "user"}); !errors.Is(err, identity.ErrUsageUpstream) {
		t.Fatalf("oversized response error = %v, want ErrUsageUpstream", err)
	}
}
