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
	"sync/atomic"
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

type countingAccessEvaluator struct{ calls int }

func (f *countingAccessEvaluator) Resolve(context.Context, identity.ProviderConfig, identity.UpstreamIdentity) (identity.AccessEvaluation, error) {
	f.calls++
	return identity.AccessEvaluation{}, errors.New("identity tests must not evaluate access")
}

type blockingProviderTestUpstream struct {
	started   chan struct{}
	release   chan struct{}
	exchanges atomic.Int32
}

func (u *blockingProviderTestUpstream) AuthorizationURL(ctx context.Context, config identity.ProviderConfig, state, nonce, challenge, callback string) (string, error) {
	return (fakeUpstream{}).AuthorizationURL(ctx, config, state, nonce, challenge, callback)
}

func (u *blockingProviderTestUpstream) ExchangeAndVerify(ctx context.Context, config identity.ProviderConfig, code, verifier, nonce, callback string) (identity.UpstreamIdentity, error) {
	if u.exchanges.Add(1) == 1 {
		close(u.started)
	}
	select {
	case <-ctx.Done():
		return identity.UpstreamIdentity{}, ctx.Err()
	case <-u.release:
	}
	return (fakeUpstream{}).ExchangeAndVerify(ctx, config, code, verifier, nonce, callback)
}

type failingProviderDiscovery struct{}

func (failingProviderDiscovery) AuthorizationURL(context.Context, identity.ProviderConfig, string, string, string, string) (string, error) {
	return "", errors.New("OIDC discovery unavailable")
}

func (failingProviderDiscovery) ExchangeAndVerify(context.Context, identity.ProviderConfig, string, string, string, string) (identity.UpstreamIdentity, error) {
	return identity.UpstreamIdentity{}, errors.New("unexpected exchange")
}

type trackingBrokerRepository struct {
	*store.Memory
	identityReads          int
	staleIdentityRead      int
	delegatedSecretIDs     []string
	deletedDelegatedSecret []string
}

type revisionBumpingUpstream struct{ memory *store.Memory }

func (u revisionBumpingUpstream) AuthorizationURL(ctx context.Context, config identity.ProviderConfig, state, nonce, challenge, callback string) (string, error) {
	return (fakeUpstream{}).AuthorizationURL(ctx, config, state, nonce, challenge, callback)
}

func (u revisionBumpingUpstream) ExchangeAndVerify(ctx context.Context, config identity.ProviderConfig, code, verifier, nonce, callback string) (identity.UpstreamIdentity, error) {
	upstream, err := (fakeUpstream{}).ExchangeAndVerify(ctx, config, code, verifier, nonce, callback)
	if err != nil {
		return identity.UpstreamIdentity{}, err
	}
	current, err := u.memory.IdentityProvider(ctx, config.DeploymentID)
	if err != nil {
		return identity.UpstreamIdentity{}, err
	}
	if _, err := u.memory.SaveIdentityProvider(ctx, current); err != nil {
		return identity.UpstreamIdentity{}, err
	}
	return upstream, nil
}

func (r *trackingBrokerRepository) IdentityProvider(ctx context.Context, productID string) (identity.ProviderConfig, error) {
	r.identityReads++
	value, err := r.Memory.IdentityProvider(ctx, productID)
	if err == nil && r.staleIdentityRead == r.identityReads {
		value.Revision++
	}
	return value, err
}

func (r *trackingBrokerRepository) CreateSecret(ctx context.Context, value model.Secret) (model.Secret, error) {
	created, err := r.Memory.CreateSecret(ctx, value)
	if err == nil && value.Purpose == "vendor_delegated_access" {
		r.delegatedSecretIDs = append(r.delegatedSecretIDs, created.ID)
	}
	return created, err
}

func (r *trackingBrokerRepository) DeleteSecret(ctx context.Context, organisationID, id string) error {
	err := r.Memory.DeleteSecret(ctx, organisationID, id)
	if err == nil {
		r.deletedDelegatedSecret = append(r.deletedDelegatedSecret, id)
	}
	return err
}

type staticClientMetadata struct{ redirectURIs []string }

func (resolver staticClientMetadata) Resolve(_ context.Context, id string) (identity.ClientMetadata, error) {
	redirectURIs := resolver.redirectURIs
	if len(redirectURIs) == 0 {
		redirectURIs = []string{redirect}
	}
	return identity.ClientMetadata{ClientID: id, RedirectURIs: redirectURIs}, nil
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
		ClientSecretID:     "upstream-client-secret",
		Scopes:             []string{"openid"},
		Audience:           "https://api.vendor.example",
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

func TestProviderTestVerifiesIdentityWithoutCreatingRuntimeState(t *testing.T) {
	memory, vault := configuredMemory(t)
	provider, err := memory.IdentityProvider(context.Background(), productID)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := &countingAccessEvaluator{}
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, evaluator, staticClientMetadata{})
	test, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(test.AuthorizationURL)
	if err != nil || !identity.IsProviderTestState(parsed.Query().Get("state")) || test.Status != "pending" {
		t.Fatalf("test = %#v, url error = %v", test, err)
	}
	completed, err := broker.CompleteProviderTest(context.Background(), parsed.Query().Get("state"), "vendor-code", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "passed" || completed.ConfigurationRevision != provider.Revision || completed.Issuer != provider.Issuer || completed.Subject != "vendor-user-42" || completed.CustomerID != "vendor-account-7" || completed.AuthorizationURL != "" || completed.UpstreamVerifier != "" || completed.Nonce != "" {
		t.Fatalf("completed test = %#v", completed)
	}
	if evaluator.calls != 0 {
		t.Fatalf("access evaluator calls = %d", evaluator.calls)
	}
	accounts, _, err := memory.CustomerAccounts(context.Background(), productID, "", 50)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("identity test created customer accounts: %#v, err=%v", accounts, err)
	}
	if _, err := broker.CompleteProviderTest(context.Background(), parsed.Query().Get("state"), "vendor-code", ""); !errors.Is(err, identity.ErrProviderTest) {
		t.Fatalf("provider test callback was reusable: %v", err)
	}
}

func TestProviderTestDiscoveryFailureReturnsStoredResult(t *testing.T) {
	memory, vault := configuredMemory(t)
	provider, err := memory.IdentityProvider(context.Background(), productID)
	if err != nil {
		t.Fatal(err)
	}
	broker := identity.NewBroker(memory, vault, publicURL, failingProviderDiscovery{}, &countingAccessEvaluator{}, staticClientMetadata{})
	failed, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ID == "" || failed.Status != "failed" || failed.FailureCode != "oidc_authorization_failed" || failed.AuthorizationURL != "" || failed.CompletedAt == nil {
		t.Fatalf("failed discovery result = %#v", failed)
	}
	stored, err := memory.IdentityProviderTest(context.Background(), productID, failed.ID)
	if err != nil || stored.Status != "failed" || stored.FailureCode != failed.FailureCode || stored.UpstreamVerifier != "" || stored.Nonce != "" {
		t.Fatalf("stored discovery result = %#v, err=%v", stored, err)
	}
}

func TestProviderTestRejectsIncompleteLegacyConfiguration(t *testing.T) {
	memory := store.NewMemory()
	legacy, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "legacy-idp", OrganisationID: "org_acme", DeploymentID: productID, Issuer: "https://legacy-id.example.com/tenant", ClientID: "legacy-client", ClientSecretID: "legacy-secret", Scopes: []string{"profile"}, DelegatedAPIOrigin: "https://api.vendor.example", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	vault, err := secrets.New(bytes.Repeat([]byte{0x6c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, &countingAccessEvaluator{}, staticClientMetadata{})
	if _, err := broker.BeginProviderTest(context.Background(), productID, legacy.Revision); !errors.Is(err, identity.ErrProviderConfiguration) {
		t.Fatalf("incomplete legacy provider test error = %v", err)
	}
	if _, err := memory.LatestIdentityProviderTest(context.Background(), productID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("incomplete provider created a test transaction: %v", err)
	}
}

func TestProviderTestCallbackIsAtomicallyClaimed(t *testing.T) {
	memory, vault := configuredMemory(t)
	provider, err := memory.IdentityProvider(context.Background(), productID)
	if err != nil {
		t.Fatal(err)
	}
	upstream := &blockingProviderTestUpstream{started: make(chan struct{}), release: make(chan struct{})}
	broker := identity.NewBroker(memory, vault, publicURL, upstream, &countingAccessEvaluator{}, staticClientMetadata{})
	started, err := broker.BeginProviderTest(context.Background(), productID, provider.Revision)
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	rawState := authorizationURL.Query().Get("state")
	type completion struct {
		test identity.ProviderTest
		err  error
	}
	first := make(chan completion, 1)
	go func() {
		value, completeErr := broker.CompleteProviderTest(context.Background(), rawState, "vendor-code", "")
		first <- completion{test: value, err: completeErr}
	}()
	<-upstream.started
	if _, err := broker.CompleteProviderTest(context.Background(), rawState, "", "access_denied"); !errors.Is(err, identity.ErrProviderTest) {
		t.Fatalf("duplicate callback was not rejected while the exact transaction was processing: %v", err)
	}
	close(upstream.release)
	completed := <-first
	if completed.err != nil || completed.test.Status != "passed" || upstream.exchanges.Load() != 1 {
		t.Fatalf("claimed callback result=%#v err=%v exchanges=%d", completed.test, completed.err, upstream.exchanges.Load())
	}
	stored, err := memory.IdentityProviderTest(context.Background(), productID, started.ID)
	if err != nil || stored.Status != "passed" {
		t.Fatalf("stored callback result=%#v err=%v", stored, err)
	}
}

func TestProviderRevisionInvalidatesOAuthStateCodeAndAccessToken(t *testing.T) {
	bumpProvider := func(t *testing.T, memory *store.Memory) {
		t.Helper()
		provider, err := memory.IdentityProvider(context.Background(), productID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := memory.SaveIdentityProvider(context.Background(), provider); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("state", func(t *testing.T) {
		memory, vault := configuredMemory(t)
		broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
		upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, State: "state", CodeChallenge: challenge(strings.Repeat("v", 48))})
		if err != nil {
			t.Fatal(err)
		}
		parsed, _ := url.Parse(upstreamURL)
		bumpProvider(t, memory)
		if _, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code"); !errors.Is(err, identity.ErrProviderRevision) {
			t.Fatalf("stale state accepted: %v", err)
		}
	})

	t.Run("code", func(t *testing.T) {
		memory, vault := configuredMemory(t)
		broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
		verifier := strings.Repeat("v", 48)
		code := authorize(t, broker, verifier)
		bumpProvider(t, memory)
		if _, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource); !errors.Is(err, identity.ErrProviderRevision) {
			t.Fatalf("stale authorization code accepted: %v", err)
		}
	})

	t.Run("access token", func(t *testing.T) {
		memory, vault := configuredMemory(t)
		broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
		verifier := strings.Repeat("v", 48)
		code := authorize(t, broker, verifier)
		token, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource)
		if err != nil {
			t.Fatal(err)
		}
		bumpProvider(t, memory)
		if _, err := broker.Authenticate(context.Background(), token.AccessToken); !errors.Is(err, identity.ErrInvalidOAuth) {
			t.Fatalf("stale access token accepted: %v", err)
		}
	})
}

func TestCallbackAndExchangeCleanDelegatedTokenWhenOwnershipFails(t *testing.T) {
	t.Run("callback final revision race", func(t *testing.T) {
		memory, vault := configuredMemory(t)
		repository := &trackingBrokerRepository{Memory: memory}
		broker := identity.NewBroker(repository, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
		verifier := strings.Repeat("v", 48)
		upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, State: "state", CodeChallenge: challenge(verifier)})
		if err != nil {
			t.Fatal(err)
		}
		parsed, _ := url.Parse(upstreamURL)
		repository.identityReads = 0
		repository.staleIdentityRead = 5
		if _, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code"); !errors.Is(err, identity.ErrProviderRevision) {
			t.Fatalf("callback revision race error = %v", err)
		}
		if len(repository.delegatedSecretIDs) != 1 || len(repository.deletedDelegatedSecret) != 1 || repository.deletedDelegatedSecret[0] != repository.delegatedSecretIDs[0] {
			t.Fatalf("created=%v deleted=%v", repository.delegatedSecretIDs, repository.deletedDelegatedSecret)
		}
		if _, err := memory.Secret(context.Background(), "org_acme", repository.delegatedSecretIDs[0]); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("callback failure stranded delegated token: %v", err)
		}
	})

	t.Run("consumed code revision race", func(t *testing.T) {
		memory, vault := configuredMemory(t)
		repository := &trackingBrokerRepository{Memory: memory}
		broker := identity.NewBroker(repository, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{})
		verifier := strings.Repeat("v", 48)
		code := authorize(t, broker, verifier)
		if len(repository.delegatedSecretIDs) != 1 {
			t.Fatalf("delegated token secrets = %v", repository.delegatedSecretIDs)
		}
		provider, err := memory.IdentityProvider(context.Background(), productID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := memory.SaveIdentityProvider(context.Background(), provider); err != nil {
			t.Fatal(err)
		}
		if _, err := broker.Exchange(context.Background(), code, verifier, clientID, redirect, resource); !errors.Is(err, identity.ErrProviderRevision) {
			t.Fatalf("exchange revision race error = %v", err)
		}
		if len(repository.deletedDelegatedSecret) != 1 || repository.deletedDelegatedSecret[0] != repository.delegatedSecretIDs[0] {
			t.Fatalf("created=%v deleted=%v", repository.delegatedSecretIDs, repository.deletedDelegatedSecret)
		}
	})
}

func TestCallbackRechecksRevisionBeforeRuntimeIdentitySideEffects(t *testing.T) {
	memory, vault := configuredMemory(t)
	evaluator := &countingAccessEvaluator{}
	broker := identity.NewBroker(memory, vault, publicURL, revisionBumpingUpstream{memory: memory}, evaluator, staticClientMetadata{})
	upstreamURL, err := broker.Begin(context.Background(), identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: redirect, Resource: resource, State: "state", CodeChallenge: challenge(strings.Repeat("v", 48))})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(upstreamURL)
	if _, err := broker.Callback(context.Background(), parsed.Query().Get("state"), "vendor-code"); !errors.Is(err, identity.ErrProviderRevision) {
		t.Fatalf("callback revision race error = %v", err)
	}
	accounts, _, err := memory.CustomerAccounts(context.Background(), productID, "", 50)
	if err != nil || len(accounts) != 0 || evaluator.calls != 0 {
		t.Fatalf("stale callback side effects: accounts=%#v evaluator_calls=%d err=%v", accounts, evaluator.calls, err)
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
	if principal.ProductID != productID || principal.Resource != resource || principal.Subject != "vendor-user-42" || principal.ExternalCustomerID != "vendor-account-7" || principal.CustomerAccountID == "" || principal.UpstreamAccessToken != "vendor-access-token" || !principal.Grants["developer.pro"] || principal.AccessEvaluatedAt.IsZero() {
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

func TestBrokerAcceptsEphemeralPortForRegisteredLoopbackRedirect(t *testing.T) {
	memory, vault := configuredMemory(t)
	registered := "http://127.0.0.1/callback/codex"
	requested := "http://127.0.0.1:57742/callback/codex"
	broker := identity.NewBroker(memory, vault, publicURL, fakeUpstream{}, fakeAccessEvaluator{}, staticClientMetadata{redirectURIs: []string{registered}})
	verifier := strings.Repeat("v", 48)
	request := identity.AuthorizationRequest{ProductID: productID, ClientID: clientID, RedirectURI: requested, Resource: resource, Scope: "mcp:private", State: "state", CodeChallenge: challenge(verifier)}
	if _, err := broker.Begin(context.Background(), request); err != nil {
		t.Fatalf("ephemeral loopback callback port was rejected: %v", err)
	}
	for _, invalid := range []string{
		"http://127.0.0.1:57742/callback/other",
		"http://127.0.0.2:57742/callback/codex",
		"http://localhost:57742/callback/codex",
	} {
		request.RedirectURI = invalid
		if _, err := broker.Begin(context.Background(), request); !errors.Is(err, identity.ErrInvalidOAuth) {
			t.Fatalf("unregistered loopback callback %q was accepted: %v", invalid, err)
		}
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
		requestIDs = append(requestIDs, request.Header.Get("X-External-Request-ID"))
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
