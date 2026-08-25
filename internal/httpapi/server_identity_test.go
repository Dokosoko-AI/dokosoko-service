package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type identityLifecycleUpstream struct{}

func (identityLifecycleUpstream) AuthorizationURL(_ context.Context, _ identity.ProviderConfig, state, nonce, challenge, _ string) (string, error) {
	return "https://login.example.com/authorize?" + url.Values{"state": {state}, "nonce": {nonce}, "code_challenge": {challenge}}.Encode(), nil
}

func (identityLifecycleUpstream) ExchangeAndVerify(_ context.Context, config identity.ProviderConfig, code, verifier, nonce, _ string) (identity.UpstreamIdentity, error) {
	if code != "oidc-code" || verifier == "" || nonce == "" {
		return identity.UpstreamIdentity{}, errors.New("invalid test exchange")
	}
	return identity.UpstreamIdentity{Claims: identity.Claims{Issuer: config.Issuer, Subject: "oidc-customer-user", ExternalCustomerID: "customer-42"}, AccessToken: "short-lived-oidc-token", ExpiresAt: time.Now().UTC().Add(time.Hour)}, nil
}

type forbiddenIdentityTestEvaluator struct{}

func (forbiddenIdentityTestEvaluator) Resolve(context.Context, identity.ProviderConfig, identity.UpstreamIdentity) (identity.AccessEvaluation, error) {
	panic("identity provider test called access evaluation")
}

func TestIdentityProviderSetupShapeAndLifecycleHTTP(t *testing.T) {
	const baseURL = "https://dokosoko.example"
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	broker := identity.NewBroker(memory, vault, baseURL, identityLifecycleUpstream{}, forbiddenIdentityTestEvaluator{}, nil)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: baseURL, IdentityBroker: broker, AllowDemoTokens: true})

	response := request(t, handler, http.MethodGet, "/api/v1/identity-provider", "doko_admin_demo", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"provider":"oidc"`) || !strings.Contains(response.Body.String(), `"configured":false`) || !strings.Contains(response.Body.String(), `"callback_url":"https://dokosoko.example/oauth/callback"`) || !strings.Contains(response.Body.String(), `"revision":0`) {
		t.Fatalf("unconfigured response = %d %s", response.Code, response.Body.String())
	}

	draftBody := `{"provider":"oidc","issuer":"https://login.example.com/tenant/","client_id":"oidc-client","client_secret":"oidc-secret","scopes":["openid","profile","email"],"audience":"","oauth_resource":"urn:complicatedauth:authorization","customer_account_claim":"https://complicatedauth.example/customer_id","installation_claim":"","authorization_api_origin":"http://api.complicatedauth.localhost:38080","revision":0}`
	response = request(t, handler, http.MethodPut, "/api/v1/identity-provider", "doko_admin_demo", draftBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":true`) || !strings.Contains(response.Body.String(), `"credential_present":true`) || !strings.Contains(response.Body.String(), `"issuer":"https://login.example.com/tenant/"`) || !strings.Contains(response.Body.String(), `"audience":""`) || !strings.Contains(response.Body.String(), `"oauth_resource":"urn:complicatedauth:authorization"`) || !strings.Contains(response.Body.String(), `"customer_account_claim":"https://complicatedauth.example/customer_id"`) || !strings.Contains(response.Body.String(), `"authorization_api_origin":"http://api.complicatedauth.localhost:38080"`) || !strings.Contains(response.Body.String(), `"state":"disabled"`) || !strings.Contains(response.Body.String(), `"access_evaluation_url":"http://api.complicatedauth.localhost:38080/v1/access/evaluations"`) || strings.Contains(response.Body.String(), `"organisation_claim"`) || strings.Contains(response.Body.String(), `"delegated_api_origin"`) {
		t.Fatalf("draft response = %d %s", response.Code, response.Body.String())
	}
	savedDraft, err := memory.IdentityProvider(context.Background(), "prod_acme")
	if err != nil || savedDraft.ClientSecretID == "" {
		t.Fatalf("saved draft credential = %#v, err=%v", savedDraft, err)
	}
	response = request(t, handler, http.MethodPut, "/api/v1/identity-provider", "doko_admin_demo", draftBody)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"identity_revision_conflict"`) || !strings.Contains(response.Body.String(), "Reload") {
		t.Fatalf("stale identity draft = %d %s", response.Code, response.Body.String())
	}

	response = request(t, handler, http.MethodPost, "/api/v1/identity-provider/tests", "doko_admin_demo", `{"revision":1}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("begin test = %d %s", response.Code, response.Body.String())
	}
	var started identity.ProviderTest
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil || started.ID == "" || started.Status != "pending" {
		t.Fatalf("started test = %#v, err=%v", started, err)
	}
	rawState := authorizationURL.Query().Get("state")
	response = request(t, handler, http.MethodGet, "/oauth/callback?state="+url.QueryEscape(rawState)+"&code=oidc-code", "", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != baseURL+"/identity?identity_test_id="+started.ID {
		t.Fatalf("test callback = %d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	response = request(t, handler, http.MethodGet, "/api/v1/identity-provider/tests/"+started.ID, "doko_admin_demo", "")
	var exactTest identity.ProviderTest
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &exactTest) != nil || exactTest.ID != started.ID || exactTest.Status != "passed" {
		t.Fatalf("exact callback test = %d %#v body=%s", response.Code, exactTest, response.Body.String())
	}
	response = request(t, handler, http.MethodGet, "/oauth/callback?state="+url.QueryEscape(rawState)+"&code=oidc-code", "", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != baseURL+"/identity?identity_test_error=invalid_or_expired" {
		t.Fatalf("replayed callback = %d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	accounts, _, err := memory.CustomerAccounts(context.Background(), "prod_acme", "", 50)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("test callback created runtime customer accounts: %#v, err=%v", accounts, err)
	}

	response = request(t, handler, http.MethodPost, "/api/v1/identity-provider/activate", "doko_admin_demo", `{"revision":1,"test_id":"`+started.ID+`"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"active"`) || !strings.Contains(response.Body.String(), `"revision":2`) || !strings.Contains(response.Body.String(), `"configuration_revision":1`) {
		t.Fatalf("activate = %d %s", response.Code, response.Body.String())
	}

	response = request(t, handler, http.MethodPost, "/api/v1/identity-provider/tests", "doko_admin_demo", `{"revision":2}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("active re-test = %d %s", response.Code, response.Body.String())
	}
	var activeTest identity.ProviderTest
	if err := json.Unmarshal(response.Body.Bytes(), &activeTest); err != nil {
		t.Fatal(err)
	}
	activeURL, _ := url.Parse(activeTest.AuthorizationURL)
	response = request(t, handler, http.MethodGet, "/oauth/callback?state="+url.QueryEscape(activeURL.Query().Get("state"))+"&error=access_denied", "", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != baseURL+"/identity?identity_test_id="+activeTest.ID {
		t.Fatalf("denied active re-test callback = %d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}

	response = request(t, handler, http.MethodDelete, "/api/v1/identity-provider", "doko_admin_demo", `{"revision":2}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"identity_disable_required"`) {
		t.Fatalf("disconnect active provider = %d %s", response.Code, response.Body.String())
	}
	now := time.Now().UTC()
	account, err := memory.ResolveCustomerAccount(context.Background(), identity.CustomerAccount{ID: "customer-account-preserved", OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: savedDraft.Issuer, ExternalID: "customer-42", State: "active", LastAuthenticatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	response = request(t, handler, http.MethodPost, "/api/v1/identity-provider/disable", "doko_admin_demo", `{"revision":2}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"disabled"`) || !strings.Contains(response.Body.String(), `"revision":3`) {
		t.Fatalf("disable = %d %s", response.Code, response.Body.String())
	}
	response = request(t, handler, http.MethodDelete, "/api/v1/identity-provider", "doko_admin_demo", `{"revision":3}`)
	if response.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s", response.Code, response.Body.String())
	}
	var disconnected map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &disconnected); err != nil {
		t.Fatal(err)
	}
	if configured, ok := disconnected["configured"].(bool); !ok || configured {
		t.Fatalf("disconnect setup shape configured = %#v", disconnected)
	}
	for _, field := range []string{"id", "created_at", "updated_at", "last_test"} {
		if _, exists := disconnected[field]; exists {
			t.Fatalf("disconnect setup shape leaked %s: %#v", field, disconnected)
		}
	}
	if _, err := memory.IdentityProvider(context.Background(), "prod_acme"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("disconnected provider remains: %v", err)
	}
	if _, err := memory.Secret(context.Background(), "org_acme", savedDraft.ClientSecretID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("disconnected provider credential remains: %v", err)
	}
	for _, testID := range []string{started.ID, activeTest.ID} {
		if _, err := memory.IdentityProviderTest(context.Background(), "prod_acme", testID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("provider test %q remains after disconnect: %v", testID, err)
		}
	}
	if preserved, err := memory.CustomerAccount(context.Background(), "prod_acme", account.ID); err != nil || preserved.ExternalID != account.ExternalID {
		t.Fatalf("customer account was not preserved: %#v, err=%v", preserved, err)
	}
	audits, err := memory.AuditEvents(context.Background(), "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	foundDisconnectAudit := false
	for _, event := range audits {
		if event.Action == "identity_provider.disconnected" && event.TargetID == savedDraft.ID {
			foundDisconnectAudit = true
		}
	}
	if !foundDisconnectAudit {
		t.Fatalf("disconnect audit missing: %#v", audits)
	}
}

func TestIdentityProviderTestVisibilityExpiresOnlyPending(t *testing.T) {
	memory := store.NewMemory()
	now := time.Now().UTC()
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "55555555-5555-4555-8555-555555555555", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecretID: "secret", OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "disabled"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []identity.ProviderTest{
		{ID: "pending-test", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: 1, StateDigest: bytes.Repeat([]byte{1}, 32), UpstreamVerifier: "verifier", Nonce: "nonce", Status: "pending", Subject: "oidc-pending-user", CustomerID: "pending-customer", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute)},
		{ID: "passed-test", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: 1, StateDigest: bytes.Repeat([]byte{2}, 32), Status: "passed", Subject: "oidc-passed-user", CustomerID: "passed-customer", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), CompletedAt: &now},
	} {
		if err := memory.CreateIdentityProviderTest(context.Background(), test); err != nil {
			t.Fatal(err)
		}
	}
	handler := httpapi.New(platform.New(memory), "https://dokosoko.example")
	pending := request(t, handler, http.MethodGet, "/api/v1/identity-provider/tests/pending-test", "doko_admin_demo", "")
	passed := request(t, handler, http.MethodGet, "/api/v1/identity-provider/tests/passed-test", "doko_admin_demo", "")
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"status":"expired"`) {
		t.Fatalf("pending visibility = %d %s", pending.Code, pending.Body.String())
	}
	if passed.Code != http.StatusOK || !strings.Contains(passed.Body.String(), `"status":"passed"`) {
		t.Fatalf("passed visibility = %d %s", passed.Code, passed.Body.String())
	}
	activation := request(t, handler, http.MethodPost, "/api/v1/identity-provider/activate", "doko_admin_demo", `{"revision":1,"test_id":"passed-test"}`)
	if activation.Code != http.StatusConflict {
		t.Fatalf("expired passing test activated provider = %d %s", activation.Code, activation.Body.String())
	}
	storedPending, err := memory.IdentityProviderTest(context.Background(), "prod_acme", "pending-test")
	if err != nil || storedPending.Status != "expired" || storedPending.UpstreamVerifier != "" || storedPending.Nonce != "" || storedPending.Subject != "" || storedPending.CustomerID != "" || storedPending.CompletedAt == nil {
		t.Fatalf("expired pending test retained transaction secrets: %#v, err=%v", storedPending, err)
	}
	storedPassed, err := memory.IdentityProviderTest(context.Background(), "prod_acme", "passed-test")
	if err != nil || storedPassed.Status != "passed" || storedPassed.Subject != "" || storedPassed.CustomerID != "" {
		t.Fatalf("historical passed test retained expired identity data: %#v, err=%v", storedPassed, err)
	}
}

func TestExpiredIdentityProviderCallbackReturnsSafeRetryMarker(t *testing.T) {
	const baseURL = "https://dokosoko.example"
	memory := store.NewMemory()
	provider, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "expired-callback-provider", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecretID: "secret", Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	rawState := "idptest_expired-callback"
	stateDigest := sha256.Sum256([]byte(rawState))
	now := time.Now().UTC()
	if err := memory.CreateIdentityProviderTest(context.Background(), identity.ProviderTest{ID: "expired-callback-test", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: provider.Revision, StateDigest: stateDigest[:], UpstreamVerifier: "expired-verifier", Nonce: "expired-nonce", Status: "pending", Subject: "oidc-expired-user", CustomerID: "expired-customer", CreatedAt: now.Add(-20 * time.Minute), ExpiresAt: now.Add(-10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	broker := identity.NewBroker(memory, nil, baseURL, identityLifecycleUpstream{}, forbiddenIdentityTestEvaluator{}, nil)
	handler := httpapi.NewWithOptions(platform.New(memory), httpapi.Options{BaseURL: baseURL, IdentityBroker: broker, AllowDemoTokens: true})
	response := request(t, handler, http.MethodGet, "/oauth/callback?state="+url.QueryEscape(rawState)+"&code=too-late", "", "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != baseURL+"/identity?identity_test_error=invalid_or_expired" {
		t.Fatalf("expired callback = %d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	stored, err := memory.IdentityProviderTest(context.Background(), "prod_acme", "expired-callback-test")
	if err != nil || stored.Status != "expired" || stored.FailureCode != "test_expired" || stored.UpstreamVerifier != "" || stored.Nonce != "" || stored.Subject != "" || stored.CustomerID != "" || stored.CompletedAt == nil {
		t.Fatalf("expired callback transaction = %#v, err=%v", stored, err)
	}
}
