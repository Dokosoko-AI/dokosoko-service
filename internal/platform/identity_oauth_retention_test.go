package platform_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type identityRetentionCall struct {
	deleted int64
	limit   int
	err     error
}

type identityRetentionTrackingStore struct {
	store.Store
	calls chan identityRetentionCall
}

func (s *identityRetentionTrackingStore) DeleteStaleOAuthArtifacts(ctx context.Context, productID string, now time.Time, limit int) (int64, error) {
	deleted, err := s.Store.DeleteStaleOAuthArtifacts(ctx, productID, now, limit)
	s.calls <- identityRetentionCall{deleted: deleted, limit: limit, err: err}
	return deleted, err
}

func seedRetentionProvider(t *testing.T, memory *store.Memory) identity.ProviderConfig {
	t.Helper()
	provider, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "retention-provider", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecretID: "client-secret", Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func appendRetentionAccessToken(t *testing.T, memory *store.Memory, id string, revision int64, expiresAt time.Time) {
	t.Helper()
	if _, err := memory.CreateSecret(context.Background(), model.Secret{ID: "retention-secret-" + id, OrganisationID: "org_acme", Name: "retention-secret-" + id, Purpose: "vendor_delegated_access", Ciphertext: []byte("ciphertext"), Nonce: []byte("nonce"), KeyVersion: 1, Fingerprint: id}); err != nil {
		t.Fatal(err)
	}
	if err := memory.CreateAccessToken(context.Background(), identity.AccessToken{Digest: []byte("retention-token-" + id), ProductID: "prod_acme", ProviderRevision: revision, UpstreamAccessSecretID: "retention-secret-" + id, ExpiresAt: expiresAt}); err != nil {
		t.Fatal(err)
	}
}

func advanceRetentionProvider(t *testing.T, memory *store.Memory, provider identity.ProviderConfig) identity.ProviderConfig {
	t.Helper()
	provider, err := memory.SaveIdentityProvider(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func nextIdentityRetentionCall(t *testing.T, calls <-chan identityRetentionCall) identityRetentionCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for identity OAuth retention cleanup")
		return identityRetentionCall{}
	}
}

func stopIdentityRetentionJanitor(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("identity OAuth retention janitor did not stop")
	}
}

func TestIdentityOAuthRetentionJanitorImmediatelyDrainsBoundedBatches(t *testing.T) {
	memory := store.NewMemory()
	provider := seedRetentionProvider(t, memory)
	tracking := &identityRetentionTrackingStore{Store: memory, calls: make(chan identityRetentionCall, 16)}
	service := platform.New(tracking)
	now := time.Now().UTC()
	for index := 0; index < 205; index++ {
		appendRetentionAccessToken(t, memory, fmt.Sprintf("stale-%03d", index), provider.Revision, now.Add(time.Hour))
	}
	provider = advanceRetentionProvider(t, memory, provider)
	appendRetentionAccessToken(t, memory, "current", provider.Revision, now.Add(time.Hour))
	if err := memory.CreateIdentityProviderTest(context.Background(), identity.ProviderTest{ID: "expired-provider-test", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: provider.Revision, StateDigest: make([]byte, 32), UpstreamVerifier: "expired-verifier", Nonce: "expired-nonce", Status: "pending", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.RunIdentityOAuthRetentionJanitor(ctx, time.Hour)
		close(done)
	}()
	for index, wantDeleted := range []int64{100, 100, 5, 0} {
		call := nextIdentityRetentionCall(t, tracking.calls)
		if call.err != nil || call.limit != 100 || call.deleted != wantDeleted {
			t.Fatalf("cleanup call %d = {deleted:%d limit:%d err:%v}, want deleted=%d", index, call.deleted, call.limit, call.err, wantDeleted)
		}
	}
	stopIdentityRetentionJanitor(t, cancel, done)

	if _, err := memory.Secret(context.Background(), "org_acme", "retention-secret-stale-204"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("stale delegated secret remains: %v", err)
	}
	if _, err := memory.Secret(context.Background(), "org_acme", "retention-secret-current"); err != nil {
		t.Fatalf("current delegated secret removed: %v", err)
	}
	expired, err := memory.IdentityProviderTest(context.Background(), "prod_acme", "expired-provider-test")
	if err != nil || expired.Status != "expired" || expired.UpstreamVerifier != "" || expired.Nonce != "" || expired.CompletedAt == nil {
		t.Fatalf("expired provider test = %#v err=%v", expired, err)
	}
}

func TestIdentityOAuthRetentionJanitorRunsAgainWithoutIdentityTraffic(t *testing.T) {
	memory := store.NewMemory()
	provider := seedRetentionProvider(t, memory)
	tracking := &identityRetentionTrackingStore{Store: memory, calls: make(chan identityRetentionCall, 32)}
	service := platform.New(tracking)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.RunIdentityOAuthRetentionJanitor(ctx, 10*time.Millisecond)
		close(done)
	}()
	first := nextIdentityRetentionCall(t, tracking.calls)
	if first.err != nil || first.deleted != 0 || first.limit != 100 {
		t.Fatalf("startup cleanup = %#v", first)
	}
	appendRetentionAccessToken(t, memory, "cold-stale", provider.Revision, time.Now().UTC().Add(time.Hour))
	advanceRetentionProvider(t, memory, provider)
	for {
		call := nextIdentityRetentionCall(t, tracking.calls)
		if call.err != nil || call.limit != 100 {
			t.Fatalf("periodic cleanup = %#v", call)
		}
		if call.deleted == 1 {
			break
		}
	}
	stopIdentityRetentionJanitor(t, cancel, done)
	if _, err := memory.Secret(context.Background(), "org_acme", "retention-secret-cold-stale"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cold stale delegated secret remains: %v", err)
	}
}
