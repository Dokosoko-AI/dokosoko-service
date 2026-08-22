package identity_test

import (
	"bytes"
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
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	publicURL = "https://doko.example"
	productID = "prod_acme"
	resource  = publicURL + "/mcp"
	clientID  = "https://client.example/oauth/client-metadata.json"
	redirect  = "https://client.example/callback"
)

type fakeUpstream struct{ installationID string }

func (fakeUpstream) AuthorizationURL(_ context.Context, _ identity.ProviderConfig, state, nonce, challenge, _ string) (string, error) {
	values := url.Values{"state": {state}, "nonce": {nonce}, "challenge": {challenge}}
	return "https://idp.example/authorize?" + values.Encode(), nil
}

func (f fakeUpstream) ExchangeAndVerify(_ context.Context, config identity.ProviderConfig, code, verifier, nonce, _ string) (identity.UpstreamIdentity, error) {
	if code != "vendor-code" || verifier == "" || nonce == "" {
		return identity.UpstreamIdentity{}, errors.New("invalid upstream exchange")
	}
	return identity.UpstreamIdentity{
		Claims: identity.Claims{
			Issuer:             config.Issuer,
			Subject:            "vendor-user-42",
			Email:              "alex@example.com",
			DisplayName:        "Alex",
			ExternalCustomerID: "vendor-account-7",
			InstallationID:     f.installationID,
		},
		AccessToken: "vendor-access-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}, nil
}

type fakeAccessEvaluator struct{ failure error }

func (f fakeAccessEvaluator) Resolve(_ context.Context, _ identity.ProviderConfig, upstream identity.UpstreamIdentity) (identity.AccessEvaluation, error) {
	if f.failure != nil {
		return identity.AccessEvaluation{}, f.failure
	}
	if upstream.Claims.Subject != "vendor-user-42" || upstream.AccessToken != "vendor-access-token" {
		return identity.AccessEvaluation{}, errors.New("identity was not forwarded")
	}
	return identity.AccessEvaluation{ID: "eval_7", Grants: []string{"developer.pro"}, PolicyVersion: "2026-08", ExpiresAt: time.Now().UTC().Add(30 * time.Minute)}, nil
}

type staticClientMetadata struct{}

func (staticClientMetadata) Resolve(_ context.Context, id string) (identity.ClientMetadata, error) {
	return identity.ClientMetadata{ClientID: id, RedirectURIs: []string{redirect}}, nil
}

func configuredMemory(t *testing.T) (*store.Memory, *secrets.Vault) {
	t.Helper()
	memory := store.NewMemory()
	_, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{
		ID:                 "idp-1",
		OrganisationID:     "org_acme",
		DeploymentID:       productID,
		Issuer:             "https://idp.example",
		ClientID:           "upstream-client",
		Scopes:             []string{"openid"},
		OrganisationClaim:  "org_id",
		DelegatedAPIOrigin: "https://api.vendor.example",
		State:              "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New(bytes.Repeat([]byte{0x7a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return memory, vault
}

func challenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestDisabledIdentityProviderFailsClosed(t *testing.T) {
	memory, vault := configuredMemory(t)
	provider, err := memory.IdentityProvider(context.Background(), productID)
	if err != nil {
		t.Fatal(err)
	}
	provider.State = "disabled"
	if _, err := memory.SaveIdentityProvider(context.Background(), provider); err != nil {
		t.Fatal(err)
	}
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
	_, err = broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, Scope: "mcp:private", State: "client-state", CodeChallenge: challenge(strings.Repeat("v", 48))})
	if !errors.Is(err, identity.ErrIdentityDisabled) {
		t.Fatalf("disabled identity provider did not fail closed: %v", err)
	}
}

func authorize(t *testing.T, broker *identity.Broker, verifier string) string {
	t.Helper()
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{
		ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource,
		Scope: "mcp:private", State: "client-state", CodeChallenge: challenge(verifier),
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code")
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := url.Parse(result.RedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	if downstream.Query().Get("state") != "client-state" || downstream.Query().Get("code") == "" {
		t.Fatalf("invalid downstream callback: %s", result.RedirectURI)
	}
	return downstream.Query().Get("code")
}

func TestBrokerBindsOAuthToClientResourceAndCustomerAccount(t *testing.T) {
	memory, vault := configuredMemory(t)
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
	verifier := strings.Repeat("v", 48)
	code := authorize(t, broker, verifier)

	if _, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, publicURL+"/mcp/another"); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("authorization code accepted for another resource: %v", err)
	}
	// A failed exchange consumes the code. Authorization codes remain single-use
	// even when a client submits a malformed token request.
	code = authorize(t, broker, verifier)
	token, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := broker.Authenticate(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ProductID != productID || principal.Resource != resource || principal.Subject != "vendor-user-42" || principal.ExternalCustomerID != "vendor-account-7" || principal.CustomerAccountID == "" || principal.UpstreamAccessToken != "vendor-access-token" || !principal.Grants["developer.pro"] {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	accounts, _, err := memory.CustomerAccounts(context.Background(), productID, "", 50)
	if err != nil || len(accounts) != 1 || accounts[0].ID != principal.CustomerAccountID {
		t.Fatalf("customer accounts = %#v, err = %v", accounts, err)
	}
	if _, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("authorization code was reusable: %v", err)
	}
}

func TestSuspendingCustomerAccountInvalidatesExistingToken(t *testing.T) {
	memory, vault := configuredMemory(t)
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
	verifier := strings.Repeat("v", 48)
	code := authorize(t, broker, verifier)
	token, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := broker.Authenticate(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	account, err := memory.CustomerAccount(context.Background(), productID, principal.CustomerAccountID)
	if err != nil {
		t.Fatal(err)
	}
	account.State = "suspended"
	if _, err := memory.UpdateCustomerAccount(context.Background(), account, account.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Authenticate(context.Background(), token.AccessToken); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("suspended account retained access: %v", err)
	}
}

func TestAuthorizationRequiresInstallationToBelongToCustomer(t *testing.T) {
	memory, vault := configuredMemory(t)
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{installationID: "installation-9"}, fakeAccessEvaluator{}, staticClientMetadata{})
	verifier := strings.Repeat("v", 48)
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, Scope: "mcp:private", State: "state", CodeChallenge: challenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(upstreamURL)
	if _, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code"); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("unknown installation claim was accepted: %v", err)
	}

	account, err := memory.ResolveCustomerAccount(context.Background(), identity.CustomerAccount{ID: "account_7", OrganisationID: "org_acme", ProductID: productID, Issuer: "https://idp.example", ExternalID: "vendor-account-7", State: "active", LastAuthenticatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.SaveProductInstallation(context.Background(), model.ProductInstallation{ID: "installation_internal_9", OrganisationID: "org_acme", ProductID: productID, CustomerAccountID: account.ID, EnvironmentID: "env_prod", ExternalID: "installation-9", Name: "Customer production", State: "active"}, 0); err != nil {
		t.Fatal(err)
	}
	code := authorize(t, broker, verifier)
	token, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource)
	if err != nil || token.Principal.InstallationID != "installation-9" {
		t.Fatalf("registered installation authorization failed: token=%#v err=%v", token, err)
	}
}

func TestBrokerRequiresRegisteredClientAndFailsClosed(t *testing.T) {
	memory, vault := configuredMemory(t)
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{failure: errors.New("vendor unavailable")}, staticClientMetadata{})
	verifier := strings.Repeat("v", 48)
	if _, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: "https://attacker.example/callback", Resource: resource, State: "state", CodeChallenge: challenge(verifier)}); !errors.Is(err, identity.ErrInvalidOAuth) {
		t.Fatalf("unregistered redirect accepted: %v", err)
	}
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, State: "state", CodeChallenge: challenge(verifier)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(upstreamURL)
	if _, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code"); err == nil {
		t.Fatal("access evaluation failure did not fail closed")
	}
}

type resolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f resolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAccessEvaluationUsesDelegatedBearerAndRetrySafeHeaders(t *testing.T) {
	requests := 0
	var idempotencyKey string
	var requestIDs []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPost || request.URL.String() != "https://api.vendor.example/v1/access/evaluations" || request.Header.Get("Authorization") != "Bearer vendor-token" || string(body) != `{}` {
			t.Fatalf("unexpected request: method=%s url=%s headers=%v body=%s", request.Method, request.URL, request.Header, body)
		}
		if requests == 1 {
			idempotencyKey = request.Header.Get("Idempotency-Key")
		} else if request.Header.Get("Idempotency-Key") != idempotencyKey {
			t.Fatalf("idempotency key changed across retry")
		}
		requestIDs = append(requestIDs, request.Header.Get("X-DokoSoko-Request-ID"))
		if requests == 1 {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}, nil
		}
		expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"eval_1","grants":["projects.read"],"expires_at":"` + expires + `","policy_version":"v7"}`))}, nil
	})}
	evaluation := identity.HTTPAccessEvaluation{Client: client, Resolver: resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})}
	result, err := evaluation.Resolve(context.Background(), identity.ProviderConfig{DelegatedAPIOrigin: "https://api.vendor.example"}, identity.UpstreamIdentity{Claims: identity.Claims{Issuer: "https://idp.example", Subject: "user_1"}, AccessToken: "vendor-token", ExpiresAt: time.Now().UTC().Add(time.Hour), AccessEvaluationKey: "aeval_0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "eval_1" || len(result.Grants) != 1 || requests != 2 || idempotencyKey == "" || requestIDs[0] == requestIDs[1] {
		t.Fatalf("result=%#v requests=%d idempotency=%q request_ids=%v", result, requests, idempotencyKey, requestIDs)
	}
}

func TestAccessEvaluationRejectsPrivateResolution(t *testing.T) {
	evaluation := identity.HTTPAccessEvaluation{Resolver: resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})}
	_, err := evaluation.Resolve(context.Background(), identity.ProviderConfig{DelegatedAPIOrigin: "https://api.vendor.example"}, identity.UpstreamIdentity{AccessToken: "vendor-token", AccessEvaluationKey: "aeval_0123456789abcdef0123456789abcdef"})
	if err == nil {
		t.Fatal("private vendor API resolution was accepted")
	}
}

func TestBrokerAcceptsDurablyRegisteredPublicClient(t *testing.T) {
	memory, vault := configuredMemory(t)
	registeredID := "mcp_client_0123456789abcdefghijklmnopqrstuvwxyz"
	if _, err := memory.CreateOAuthClient(context.Background(), identity.OAuthClient{ClientID: registeredID, DeploymentID: productID, ClientName: "Cursor", RedirectURIs: []string{redirect}}); err != nil {
		t.Fatal(err)
	}
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, &identity.HTTPClientMetadataResolver{})
	redirectURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: registeredID, RedirectURI: redirect, Resource: resource, Scope: "mcp:private", State: "client-state", CodeChallenge: challenge(strings.Repeat("v", 48))})
	if err != nil || !strings.HasPrefix(redirectURL, "https://idp.example/authorize?") {
		t.Fatalf("registered client redirect = %q, error = %v", redirectURL, err)
	}
}

func TestClientMetadataResolverRequiresExactMetadata(t *testing.T) {
	metadataURL := "https://client.example/oauth/client-metadata.json"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := json.Marshal(identity.ClientMetadata{ClientID: metadataURL, RedirectURIs: []string{redirect}})
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})}
	resolver := identity.HTTPClientMetadataResolver{Client: client, Resolver: resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})}
	metadata, err := resolver.Resolve(context.Background(), metadataURL)
	if err != nil || metadata.ClientID != metadataURL || len(metadata.RedirectURIs) != 1 {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
}
