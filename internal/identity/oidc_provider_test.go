package identity_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
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

func signedTestIDToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestRealOIDCProviderTestSupportsGenericDiscoveryClientAuthAndClaimProof(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const (
		keyID         = "identity-test-key"
		upstreamID    = "oidc-client-id"
		customerClaim = "https://complicatedauth.example/customer_id"
		issuer        = "http://issuer.oidc.localhost:38080/tenant"
		authorizeURL  = "http://authorize.oidc.localhost:38081/authorize"
		tokenURL      = "http://token.oidc.localhost:38082/oauth/token"
		jwksURL       = "http://keys.oidc.localhost:38083/jwks"
	)
	var expectedNonce string
	var tokenAudience = upstreamID
	var tokenNonce string
	var includeCustomer = true
	var tokenAuthMethods []string
	var expectedClientAuth = "basic"
	tokenRequests := 0
	credentialProofs := 0
	jsonResponse := func(request *http.Request, status int, payload any) (*http.Response, error) {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: request}, nil
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			metadata := map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                authorizeURL,
				"token_endpoint":                        tokenURL,
				"jwks_uri":                              jwksURL,
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			}
			if tokenAuthMethods != nil {
				metadata["token_endpoint_auth_methods_supported"] = tokenAuthMethods
			}
			return jsonResponse(r, http.StatusOK, metadata)
		case r.URL.String() == jwksURL:
			return jsonResponse(r, http.StatusOK, map[string]any{"keys": []any{map[string]any{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": keyID,
				"n": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
			}}})
		case r.URL.String() == tokenURL:
			tokenRequests++
			if err := r.ParseForm(); err != nil || r.Form.Get("code") == "" || r.Form.Get("code_verifier") == "" || r.Form.Get("resource") != "urn:complicatedauth:authorization" {
				return jsonResponse(r, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
			}
			username, password, basic := r.BasicAuth()
			credentialOK := false
			switch expectedClientAuth {
			case "basic":
				credentialOK = basic && username == upstreamID && password == "oidc-client-secret"
			case "post":
				credentialOK = !basic && r.Form.Get("client_id") == upstreamID && r.Form.Get("client_secret") == "oidc-client-secret"
			}
			if !credentialOK {
				return jsonResponse(r, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
			}
			credentialProofs++
			now := time.Now().UTC()
			claims := map[string]any{"iss": issuer, "sub": "oidc-test-user", "aud": tokenAudience, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": tokenNonce}
			if includeCustomer {
				claims[customerClaim] = "customer-42"
			}
			return jsonResponse(r, http.StatusOK, map[string]any{"access_token": "oidc-access-token", "token_type": "Bearer", "expires_in": 3600, "id_token": signedTestIDToken(t, privateKey, keyID, claims)})
		default:
			return jsonResponse(r, http.StatusNotFound, map[string]any{"error": "not_found"})
		}
	})}

	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x74}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secretID := "11111111-1111-4111-8111-111111111111"
	encrypted, err := vault.Encrypt([]byte("oidc-client-secret"), "org_acme:idp:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSecret(context.Background(), model.Secret{ID: secretID, OrganisationID: "org_acme", Name: "identity-provider-test", Purpose: "identity_provider_oidc_client", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	provider, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "22222222-2222-4222-8222-222222222222", OrganisationID: "org_acme", DeploymentID: productID, Issuer: issuer, ClientID: upstreamID, ClientSecretID: secretID, Scopes: []string{"openid", "profile", "email"}, OAuthResource: "urn:complicatedauth:authorization", OrganisationClaim: customerClaim, DelegatedAPIOrigin: "https://api.complicatedauth.example", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	repository := &trackingBrokerRepository{Memory: memory}
	upstream := identity.NewOIDCUpstream(repository, vault, client, resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}))
	broker := identity.NewBroker(repository, vault, publicURL, upstream, nil, nil)

	for _, testCase := range []struct {
		name            string
		idTokenAudience string
		authMethods     []string
		clientAuth      string
		nonce           func(string) string
		includeCustomer bool
		wantStatus      string
		wantFailure     string
	}{
		{name: "omitted auth metadata defaults to basic", idTokenAudience: upstreamID, clientAuth: "basic", nonce: func(value string) string { return value }, includeCustomer: true, wantStatus: "passed"},
		{name: "explicit basic", idTokenAudience: upstreamID, authMethods: []string{"client_secret_basic"}, clientAuth: "basic", nonce: func(value string) string { return value }, includeCustomer: true, wantStatus: "passed"},
		{name: "explicit post", idTokenAudience: upstreamID, authMethods: []string{"client_secret_post"}, clientAuth: "post", nonce: func(value string) string { return value }, includeCustomer: true, wantStatus: "passed"},
		{name: "wrong ID token audience", idTokenAudience: "another-client", authMethods: []string{"client_secret_basic"}, clientAuth: "basic", nonce: func(value string) string { return value }, includeCustomer: true, wantStatus: "failed", wantFailure: "oidc_verification_failed"},
		{name: "wrong nonce", idTokenAudience: upstreamID, authMethods: []string{"client_secret_basic"}, clientAuth: "basic", nonce: func(string) string { return "another-nonce" }, includeCustomer: true, wantStatus: "failed", wantFailure: "oidc_verification_failed"},
		{name: "missing customer claim", idTokenAudience: upstreamID, authMethods: []string{"client_secret_basic"}, clientAuth: "basic", nonce: func(value string) string { return value }, includeCustomer: false, wantStatus: "failed", wantFailure: "customer_claim_missing"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tokenAuthMethods = testCase.authMethods
			expectedClientAuth = testCase.clientAuth
			started, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
			if err != nil {
				t.Fatal(err)
			}
			authorizationURL, err := url.Parse(started.AuthorizationURL)
			if err != nil {
				t.Fatal(err)
			}
			if authorizationURL.Query().Get("audience") != "" || authorizationURL.Query().Get("resource") != provider.OAuthResource || authorizationURL.Query().Get("client_id") != upstreamID || authorizationURL.Host != "authorize.oidc.localhost:38081" {
				t.Fatalf("authorization query = %s", authorizationURL.RawQuery)
			}
			expectedNonce = authorizationURL.Query().Get("nonce")
			tokenAudience = testCase.idTokenAudience
			tokenNonce = testCase.nonce(expectedNonce)
			includeCustomer = testCase.includeCustomer
			completed, err := broker.CompleteProviderTest(context.Background(), authorizationURL.Query().Get("state"), "oidc-code", "")
			if err != nil {
				t.Fatal(err)
			}
			if completed.Status != testCase.wantStatus || completed.FailureCode != testCase.wantFailure {
				t.Fatalf("completed = %#v", completed)
			}
			if testCase.wantStatus == "passed" && (completed.Issuer != issuer || completed.CustomerID != "customer-42" || completed.Subject != "oidc-test-user") {
				t.Fatalf("verified claims = %#v", completed)
			}
		})
	}
	provider.Audience = "urn:provider-specific:api"
	provider, err = memory.SaveIdentityProvider(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	tokenAuthMethods = []string{"client_secret_basic"}
	withAudience, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil {
		t.Fatal(err)
	}
	withAudienceURL, err := url.Parse(withAudience.AuthorizationURL)
	if err != nil || withAudienceURL.Query().Get("audience") != provider.Audience {
		t.Fatalf("configured optional audience URL = %q, err=%v", withAudience.AuthorizationURL, err)
	}
	tokenAuthMethods = []string{"private_key_jwt"}
	unsupported, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil || unsupported.Status != "failed" || unsupported.FailureCode != "client_authentication_unsupported" || unsupported.AuthorizationURL != "" {
		t.Fatalf("unsupported client authentication result = %#v, err=%v", unsupported, err)
	}
	if len(repository.delegatedSecretIDs) != 0 {
		t.Fatalf("provider tests retained delegated access tokens: %v", repository.delegatedSecretIDs)
	}
	if tokenRequests != 6 || credentialProofs != tokenRequests {
		t.Fatalf("token requests=%d credential proofs=%d", tokenRequests, credentialProofs)
	}
	accounts, _, err := memory.CustomerAccounts(context.Background(), productID, "", 50)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("provider tests created customer accounts: %#v, err=%v", accounts, err)
	}
}

func TestPublicOIDCIssuerCannotPivotDiscoveryToLocalEndpoints(t *testing.T) {
	const issuer = "https://issuer.example.com"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, err := json.Marshal(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                "https://authorize.example.com/oauth/authorize",
			"token_endpoint":                        "http://token.attacker.localhost:38082/oauth/token",
			"jwks_uri":                              "https://keys.example.com/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(payload)), Request: request}, nil
	})}
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x75}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secretID := "77777777-7777-4777-8777-777777777777"
	encrypted, err := vault.Encrypt([]byte("oidc-client-secret"), "org_acme:idp:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSecret(context.Background(), model.Secret{ID: secretID, OrganisationID: "org_acme", Name: "public-issuer-secret", Purpose: "identity_provider_oidc_client", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	provider, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "88888888-8888-4888-8888-888888888888", OrganisationID: "org_acme", DeploymentID: productID, Issuer: issuer, ClientID: "oidc-client", ClientSecretID: secretID, Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	resolver := resolverFunc(func(_ context.Context, _, host string) ([]net.IP, error) {
		if identity.IsLocalDevelopmentHostname(host) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})
	upstream := identity.NewOIDCUpstream(memory, vault, client, resolver)
	broker := identity.NewBroker(memory, vault, publicURL, upstream, nil, nil)
	result, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil || result.Status != "failed" || result.FailureCode != "oidc_authorization_failed" || result.AuthorizationURL != "" {
		t.Fatalf("public-to-local discovery pivot result = %#v, err=%v", result, err)
	}
}
