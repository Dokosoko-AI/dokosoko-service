package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func createDelegatedSecret(t *testing.T, memory *store.Memory, id, purpose string) {
	t.Helper()
	if _, err := memory.CreateSecret(context.Background(), model.Secret{ID: id, OrganisationID: "org_acme", Name: id, Purpose: purpose, Ciphertext: []byte("ciphertext"), Nonce: []byte("nonce"), KeyVersion: 1, Fingerprint: id}); err != nil {
		t.Fatal(err)
	}
}

func seedActiveProvider(t *testing.T, memory *store.Memory, clientSecretID string) identity.ProviderConfig {
	t.Helper()
	provider, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "retention-provider", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecretID: clientSecretID, Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func advanceActiveProviderRevision(t *testing.T, memory *store.Memory, provider identity.ProviderConfig) identity.ProviderConfig {
	t.Helper()
	provider, err := memory.SaveIdentityProvider(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestDeleteStaleOAuthArtifactsRemovesOwnedSecretsAndKeepsCurrentArtifacts(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	provider := seedActiveProvider(t, memory, "provider-secret-not-stored")
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Second)
	revokedAt := now.Add(-time.Minute)

	for _, value := range []struct {
		id      string
		purpose string
	}{
		{id: "stale-code-secret", purpose: "vendor_delegated_access"},
		{id: "expired-code-secret", purpose: "vendor_delegated_access"},
		{id: "current-code-secret", purpose: "vendor_delegated_access"},
		{id: "stale-token-secret", purpose: "vendor_delegated_access"},
		{id: "expired-token-secret", purpose: "vendor_delegated_access"},
		{id: "revoked-token-secret", purpose: "vendor_delegated_access"},
		{id: "current-token-secret", purpose: "vendor_delegated_access"},
		{id: "wrong-purpose-secret", purpose: "another_purpose"},
	} {
		createDelegatedSecret(t, memory, value.id, value.purpose)
	}

	for _, value := range []identity.OAuthCode{
		{Digest: []byte("stale-code"), ProductID: "prod_acme", OrganisationID: "org_acme", ProviderRevision: 1, UpstreamAccessSecretID: "stale-code-secret", AccessExpiresAt: future, ExpiresAt: future},
	} {
		if err := memory.CreateOAuthCode(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []identity.AccessToken{
		{Digest: []byte("stale-token"), ProductID: "prod_acme", ProviderRevision: 1, UpstreamAccessSecretID: "stale-token-secret", ExpiresAt: future},
		{Digest: []byte("wrong-purpose-token"), ProductID: "prod_acme", ProviderRevision: 1, UpstreamAccessSecretID: "wrong-purpose-secret", ExpiresAt: future},
	} {
		if err := memory.CreateAccessToken(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []identity.OAuthState{
		{Digest: []byte("stale-state"), ProductID: "prod_acme", ProviderRevision: 1, ExpiresAt: future},
	} {
		if err := memory.CreateOAuthState(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	provider = advanceActiveProviderRevision(t, memory, provider)
	for _, value := range []identity.OAuthCode{
		{Digest: []byte("expired-code"), ProductID: "prod_acme", OrganisationID: "org_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "expired-code-secret", AccessExpiresAt: future, ExpiresAt: past},
		{Digest: []byte("current-code"), ProductID: "prod_acme", OrganisationID: "org_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "current-code-secret", AccessExpiresAt: future, ExpiresAt: future},
	} {
		if err := memory.CreateOAuthCode(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []identity.AccessToken{
		{Digest: []byte("expired-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "expired-token-secret", ExpiresAt: past},
		{Digest: []byte("revoked-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "revoked-token-secret", ExpiresAt: future, RevokedAt: &revokedAt},
		{Digest: []byte("current-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "current-token-secret", ExpiresAt: future},
	} {
		if err := memory.CreateAccessToken(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []identity.OAuthState{
		{Digest: []byte("expired-state"), ProductID: "prod_acme", ProviderRevision: provider.Revision, ExpiresAt: now},
		{Digest: []byte("current-state"), ProductID: "prod_acme", ProviderRevision: provider.Revision, ExpiresAt: future},
	} {
		if err := memory.CreateOAuthState(ctx, value); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := memory.DeleteStaleOAuthArtifacts(ctx, "prod_acme", now, 100)
	if err != nil || deleted != 8 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	for _, id := range []string{"stale-code-secret", "expired-code-secret", "stale-token-secret", "expired-token-secret", "revoked-token-secret"} {
		if _, err := memory.Secret(ctx, "org_acme", id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("stale delegated secret %q remains: %v", id, err)
		}
	}
	for _, id := range []string{"current-code-secret", "current-token-secret", "wrong-purpose-secret"} {
		if _, err := memory.Secret(ctx, "org_acme", id); err != nil {
			t.Fatalf("retained secret %q missing: %v", id, err)
		}
	}
	if _, err := memory.ConsumeOAuthState(ctx, []byte("current-state")); err != nil {
		t.Fatalf("current state removed: %v", err)
	}
	if _, err := memory.ConsumeOAuthCode(ctx, []byte("current-code")); err != nil {
		t.Fatalf("current code removed: %v", err)
	}
	if _, err := memory.AccessTokenByDigest(ctx, []byte("current-token")); err != nil {
		t.Fatalf("current access token removed: %v", err)
	}
}

func TestDeleteStaleOAuthArtifactsBoundsEachArtifactBatch(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	provider := seedActiveProvider(t, memory, "provider-secret-not-stored")
	now := time.Now().UTC()
	for index := 0; index < 5; index++ {
		id := string(rune('a' + index))
		createDelegatedSecret(t, memory, "bounded-secret-"+id, "vendor_delegated_access")
		if err := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: []byte("bounded-token-" + id), ProductID: "prod_acme", ProviderRevision: 1, UpstreamAccessSecretID: "bounded-secret-" + id, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	advanceActiveProviderRevision(t, memory, provider)
	deleted, err := memory.DeleteStaleOAuthArtifacts(ctx, "prod_acme", now, 2)
	if err != nil || deleted != 2 {
		t.Fatalf("bounded deleted=%d err=%v", deleted, err)
	}
}

func TestDeleteStaleOAuthArtifactsGracefullyRemovesBothBrokerOwnedOrphanPurposes(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	createdAt := time.Now().UTC()
	for _, value := range []struct {
		id      string
		purpose string
	}{
		{id: "orphan-vendor", purpose: "vendor_delegated_access"},
		{id: "orphan-provider", purpose: "identity_provider_oidc_client"},
		{id: "current-vendor", purpose: "vendor_delegated_access"},
		{id: "current-provider", purpose: "identity_provider_oidc_client"},
	} {
		createDelegatedSecret(t, memory, value.id, value.purpose)
	}
	provider := seedActiveProvider(t, memory, "current-provider")
	if err := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: []byte("current-owned-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "current-vendor", ExpiresAt: createdAt.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	deleted, err := memory.DeleteStaleOAuthArtifacts(ctx, "prod_acme", createdAt.Add(14*time.Minute), 100)
	if err != nil || deleted != 0 {
		t.Fatalf("pre-grace cleanup deleted=%d err=%v", deleted, err)
	}
	deleted, err = memory.DeleteStaleOAuthArtifacts(ctx, "prod_acme", createdAt.Add(16*time.Minute), 100)
	if err != nil || deleted != 2 {
		t.Fatalf("post-grace cleanup deleted=%d err=%v", deleted, err)
	}
	for _, id := range []string{"orphan-vendor", "orphan-provider"} {
		if _, err := memory.Secret(ctx, "org_acme", id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("orphan secret %q remains: %v", id, err)
		}
	}
	for _, id := range []string{"current-vendor", "current-provider"} {
		if _, err := memory.Secret(ctx, "org_acme", id); err != nil {
			t.Fatalf("owned secret %q was removed: %v", id, err)
		}
	}
}

func TestDeleteStaleOAuthArtifactsDerivesActiveRevisionAtDeletionTime(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	createDelegatedSecret(t, memory, "race-provider-secret", "identity_provider_oidc_client")
	provider, err := memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "race-provider", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecretID: "race-provider-secret", Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	// A janitor may have observed this disabled revision before activation. The
	// cleanup operation itself must derive the now-active revision atomically.
	provider.State = "active"
	provider, err = memory.SaveIdentityProvider(ctx, provider)
	if err != nil {
		t.Fatal(err)
	}
	createDelegatedSecret(t, memory, "race-current-vendor", "vendor_delegated_access")
	if err := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: []byte("race-current-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "race-current-vendor", ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	deleted, err := memory.DeleteStaleOAuthArtifacts(ctx, "prod_acme", time.Now().UTC(), 100)
	if err != nil || deleted != 0 {
		t.Fatalf("race-safe cleanup deleted=%d err=%v", deleted, err)
	}
	if _, err := memory.AccessTokenByDigest(ctx, []byte("race-current-token")); err != nil {
		t.Fatalf("fresh active-revision token was removed: %v", err)
	}
}

func TestDeleteIdentityProviderPurgesIdentityArtifactsAndCannotBeResurrectedByStaleUpdate(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	createDelegatedSecret(t, memory, "disconnect-provider-secret", "identity_provider_oidc_client")
	createDelegatedSecret(t, memory, "disconnect-code-secret", "vendor_delegated_access")
	createDelegatedSecret(t, memory, "disconnect-token-secret", "vendor_delegated_access")
	provider := seedActiveProvider(t, memory, "disconnect-provider-secret")
	now := time.Now().UTC()
	if err := memory.CreateIdentityProviderTest(ctx, identity.ProviderTest{ID: "disconnect-provider-test", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: provider.Revision, StateDigest: []byte("disconnect-test-state"), UpstreamVerifier: "verifier", Nonce: "nonce", Status: "pending", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := memory.CreateOAuthState(ctx, identity.OAuthState{Digest: []byte("disconnect-state"), ProductID: "prod_acme", ProviderRevision: provider.Revision, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := memory.CreateOAuthCode(ctx, identity.OAuthCode{Digest: []byte("disconnect-code"), ProductID: "prod_acme", OrganisationID: "org_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "disconnect-code-secret", AccessExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: []byte("disconnect-token"), ProductID: "prod_acme", ProviderRevision: provider.Revision, UpstreamAccessSecretID: "disconnect-token-secret", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	account, err := memory.ResolveCustomerAccount(ctx, identity.CustomerAccount{ID: "disconnect-account", OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: provider.Issuer, ExternalID: "external-customer", State: "active", LastAuthenticatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	provider.State = "disabled"
	provider, err = memory.SaveIdentityProvider(ctx, provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.DeleteIdentityProvider(ctx, "prod_acme", provider.Revision-1); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale disconnect revision = %v", err)
	}
	deleted, err := memory.DeleteIdentityProvider(ctx, "prod_acme", provider.Revision)
	if err != nil || deleted.ID != provider.ID {
		t.Fatalf("delete provider = %#v, err=%v", deleted, err)
	}
	if _, err := memory.SaveIdentityProvider(ctx, provider); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale provider update resurrected disconnected config: %v", err)
	}
	if _, err := memory.IdentityProvider(ctx, "prod_acme"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("provider remains: %v", err)
	}
	if _, err := memory.IdentityProviderTest(ctx, "prod_acme", "disconnect-provider-test"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("provider test remains: %v", err)
	}
	if _, err := memory.ConsumeOAuthState(ctx, []byte("disconnect-state")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("OAuth state remains: %v", err)
	}
	if _, err := memory.ConsumeOAuthCode(ctx, []byte("disconnect-code")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("OAuth code remains: %v", err)
	}
	if _, err := memory.AccessTokenByDigest(ctx, []byte("disconnect-token")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("access token remains: %v", err)
	}
	for _, secretID := range []string{"disconnect-provider-secret", "disconnect-code-secret", "disconnect-token-secret"} {
		if _, err := memory.Secret(ctx, "org_acme", secretID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("owned secret %q remains: %v", secretID, err)
		}
	}
	if preserved, err := memory.CustomerAccount(ctx, "prod_acme", account.ID); err != nil || preserved.ExternalID != account.ExternalID {
		t.Fatalf("customer account was not preserved: %#v, err=%v", preserved, err)
	}
}
