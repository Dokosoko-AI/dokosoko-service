package platform_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type trackingIdentityStore struct {
	*store.Memory
	createdSecretIDs []string
	deletedSecretIDs []string
	failNextSave     bool
}

func (s *trackingIdentityStore) SaveIdentityProvider(ctx context.Context, value identity.ProviderConfig) (identity.ProviderConfig, error) {
	if s.failNextSave {
		s.failNextSave = false
		return identity.ProviderConfig{}, store.ErrConflict
	}
	return s.Memory.SaveIdentityProvider(ctx, value)
}

func (s *trackingIdentityStore) CreateSecret(ctx context.Context, value model.Secret) (model.Secret, error) {
	created, err := s.Memory.CreateSecret(ctx, value)
	if err == nil {
		s.createdSecretIDs = append(s.createdSecretIDs, created.ID)
	}
	return created, err
}

func (s *trackingIdentityStore) DeleteSecret(ctx context.Context, organisationID, id string) error {
	err := s.Memory.DeleteSecret(ctx, organisationID, id)
	if err == nil {
		s.deletedSecretIDs = append(s.deletedSecretIDs, id)
	}
	return err
}

func TestIdentityDraftPreservesExactOIDCIssuerAndCleansUpRotatedCredentials(t *testing.T) {
	ctx := context.Background()
	storage := &trackingIdentityStore{Memory: store.NewMemory()}
	vault, err := secrets.New(bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(storage, vault)
	base := platform.IdentityInput{
		DeploymentID:       "prod_acme",
		Issuer:             "https://login.example.com:8443/tenant/",
		ClientID:           "oidc-client",
		ClientSecret:       "first-secret",
		Scopes:             []string{"openid", "profile"},
		OAuthResource:      "urn:complicatedauth:authorization",
		OrganisationClaim:  "https://complicatedauth.example/customer_id",
		DelegatedAPIOrigin: "http://api.complicatedauth.localhost:38080",
		Revision:           0,
	}
	first, err := service.ConfigureIdentity(ctx, base, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Issuer != base.Issuer || first.State != "disabled" || first.Revision != 1 || first.ClientSecretID == "" {
		t.Fatalf("first draft = %#v", first)
	}

	rotatedInput := base
	rotatedInput.ClientSecret = "second-secret"
	rotatedInput.Revision = first.Revision
	rotated, err := service.ConfigureIdentity(ctx, rotatedInput, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ClientSecretID == first.ClientSecretID || rotated.State != "disabled" || rotated.Revision != 2 {
		t.Fatalf("rotated draft = %#v", rotated)
	}
	if len(storage.deletedSecretIDs) != 1 || storage.deletedSecretIDs[0] != first.ClientSecretID {
		t.Fatalf("deleted secrets after rotation = %v", storage.deletedSecretIDs)
	}
	if _, err := storage.Secret(ctx, "org_acme", first.ClientSecretID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("retired secret remains readable: %v", err)
	}

	staleInput := base
	staleInput.ClientSecret = "orphan-on-conflict"
	staleInput.Revision = rotated.Revision
	storage.failNextSave = true
	if _, err := service.ConfigureIdentity(ctx, staleInput, platform.Actor{ID: "root"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale draft error = %v", err)
	}
	if len(storage.createdSecretIDs) != 3 || len(storage.deletedSecretIDs) != 2 || storage.deletedSecretIDs[1] != storage.createdSecretIDs[2] {
		t.Fatalf("created=%v deleted=%v", storage.createdSecretIDs, storage.deletedSecretIDs)
	}
	if _, err := storage.Secret(ctx, "org_acme", rotated.ClientSecretID); err != nil {
		t.Fatalf("current credential was removed: %v", err)
	}
}

func TestIdentityDraftRequiresExplicitCustomerClaimAndAllowsOmittedAudience(t *testing.T) {
	storage := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x62}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(storage, vault)
	input := platform.IdentityInput{DeploymentID: "prod_acme", Issuer: "https://login.example.com/tenant", ClientID: "client", ClientSecret: "secret", DelegatedAPIOrigin: "https://api.example.com"}
	if _, err := service.ConfigureIdentity(context.Background(), input, platform.Actor{ID: "root"}); err == nil {
		t.Fatal("identity draft accepted an implicit customer claim")
	}
	input.OrganisationClaim = "https://example.com/customer_id"
	configured, err := service.ConfigureIdentity(context.Background(), input, platform.Actor{ID: "root"})
	if err != nil || configured.Audience != "" {
		t.Fatalf("identity draft with omitted optional audience = %#v, err=%v", configured, err)
	}
}

func TestIdentityDraftRejectsInvalidResourceBeforeStoringCredential(t *testing.T) {
	storage := &trackingIdentityStore{Memory: store.NewMemory()}
	vault, err := secrets.New(bytes.Repeat([]byte{0x64}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(storage, vault)
	_, err = service.ConfigureIdentity(context.Background(), platform.IdentityInput{DeploymentID: "prod_acme", Issuer: "https://login.example.com:8443/tenant", ClientID: "client", ClientSecret: "must-not-be-stored", OAuthResource: "relative/resource", OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com"}, platform.Actor{ID: "root"})
	if !errors.Is(err, platform.ErrIdentityConfigInvalid) {
		t.Fatalf("invalid resource error = %v", err)
	}
	if len(storage.createdSecretIDs) != 0 {
		t.Fatalf("invalid draft stored client credentials: %v", storage.createdSecretIDs)
	}
}

func TestIdentityDraftCanRestoreLegacyOIDCRootSlashWithoutRotatingSecret(t *testing.T) {
	ctx := context.Background()
	storage := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x63}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secretID := "33333333-3333-4333-8333-333333333333"
	encrypted, err := vault.Encrypt([]byte("legacy-oidc-secret"), "org_acme:idp:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: "org_acme", Name: "legacy-oidc-secret", Purpose: "identity_provider_oidc_client", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	legacy, err := storage.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "44444444-4444-4444-8444-444444444444", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://login.example.com", ClientID: "oidc-client", ClientSecretID: secretID, Scopes: []string{"openid"}, OrganisationClaim: "https://example.com/customer_id", DelegatedAPIOrigin: "https://api.example.com", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(storage, vault)
	updated, err := service.ConfigureIdentity(ctx, platform.IdentityInput{DeploymentID: "prod_acme", Issuer: "https://login.example.com/", ClientID: legacy.ClientID, Audience: legacy.Audience, OrganisationClaim: legacy.OrganisationClaim, DelegatedAPIOrigin: legacy.DelegatedAPIOrigin, Revision: legacy.Revision}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Issuer != "https://login.example.com/" || updated.ClientSecretID != secretID || updated.State != "disabled" {
		t.Fatalf("updated legacy provider = %#v", updated)
	}
}

func TestIdentityActivationRejectsIncompleteLegacyConfiguration(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	legacy, err := memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "55555555-5555-4555-8555-555555555555", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://legacy-id.example.com/tenant", ClientID: "legacy-client", ClientSecretID: "legacy-secret", Scopes: []string{"openid"}, DelegatedAPIOrigin: "https://api.example.com", State: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	completedAt := time.Now().UTC()
	if err := memory.CreateIdentityProviderTest(ctx, identity.ProviderTest{ID: "66666666-6666-4666-8666-666666666666", OrganisationID: "org_acme", DeploymentID: "prod_acme", ConfigurationRevision: legacy.Revision, StateDigest: bytes.Repeat([]byte{0x42}, 32), Status: "passed", Issuer: legacy.Issuer, Subject: "legacy-oidc-user", CustomerID: "customer-42", CreatedAt: completedAt.Add(-time.Minute), ExpiresAt: completedAt.Add(time.Minute), CompletedAt: &completedAt}); err != nil {
		t.Fatal(err)
	}
	service := platform.New(memory)
	if _, err := service.ActivateIdentityProvider(ctx, "prod_acme", "66666666-6666-4666-8666-666666666666", legacy.Revision, platform.Actor{ID: "root"}); !errors.Is(err, identity.ErrProviderConfiguration) {
		t.Fatalf("incomplete legacy activation error = %v", err)
	}
	current, err := memory.IdentityProvider(ctx, "prod_acme")
	if err != nil || current.State != "disabled" || current.Revision != legacy.Revision {
		t.Fatalf("legacy provider changed after rejected activation: %#v err=%v", current, err)
	}
}
