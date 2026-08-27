package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func compactRuntimeAuthorizationJSON(value []byte) []byte {
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return value
	}
	return compact.Bytes()
}

func TestPostgresRuntimeAuthorizationProfilePersistsOwnedConfiguration(t *testing.T) {
	_, postgres := migratedPostgresForStoreTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployment, err := postgres.Deployment(ctx)
	if errors.Is(err, ErrNotFound) {
		organisationID := storeTestUUID(t)
		suffix := organisationID[:8]
		if _, err = postgres.CreateOrganisation(ctx, model.Organisation{ID: organisationID, Name: "Runtime Authorization", Slug: "runtime-authorization-" + suffix}); err != nil {
			t.Fatal(err)
		}
		deployment, err = postgres.CreateDeployment(ctx, model.Deployment{ID: storeTestUUID(t), OrganisationID: organisationID, Name: "Runtime Authorization", Slug: "runtime-authorization-" + suffix})
	}
	if err != nil {
		t.Fatal(err)
	}

	suffix := storeTestUUID(t)
	environment, err := postgres.CreateEnvironment(ctx, model.Environment{
		ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		Name: "Authorization " + suffix[:8], Slug: "authorization-" + suffix[:8], IsProduction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	integration, err := postgres.CreateIntegration(ctx, model.Integration{
		ID: storeTestUUID(t), DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		FamilyKey: "authorization-" + suffix, VersionKey: "v1", DisplayName: "Authorization API",
		Visibility: model.VisibilityPrivate, Lifecycle: "draft",
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := postgres.CreateRuntimeCredentialSet(ctx, model.RuntimeCredentialSet{
		ID: storeTestUUID(t), DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID,
		EnvironmentID: environment.ID, Scope: "shared", Name: "Vendor Authorization", EnvironmentVariable: "VENDOR_API_KEY",
		AuthenticationType: "basic", AuthConfig: json.RawMessage(`{"username":"service-user"}`),
		KeyManagementURL: "https://dashboard.example.test/api-keys", AccessEvaluationURL: "https://hooks.example.test/access", UsageURL: "https://hooks.example.test/usage", State: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.OwnerIntegrationID != "" || created.EnvironmentID != environment.ID || !bytes.Equal(compactRuntimeAuthorizationJSON(created.AuthConfig), json.RawMessage(`{"username":"service-user"}`)) || created.KeyManagementURL != "https://dashboard.example.test/api-keys" || created.AccessEvaluationURL != "https://hooks.example.test/access" || created.UsageURL != "https://hooks.example.test/usage" {
		t.Fatalf("created Authorization profile = %#v", created)
	}

	created.Name = "Vendor Production Authorization"
	created.AuthConfig = json.RawMessage(`{"username":"rotated-user"}`)
	created.KeyManagementURL = "https://dashboard.example.test/credentials"
	created.AccessEvaluationURL = "https://hooks.example.test/access-v2"
	created.UsageURL = "https://hooks.example.test/usage-v2"
	updated, err := postgres.UpdateRuntimeCredentialSet(ctx, created, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != created.Revision+1 || !bytes.Equal(compactRuntimeAuthorizationJSON(updated.AuthConfig), compactRuntimeAuthorizationJSON(created.AuthConfig)) || updated.KeyManagementURL != created.KeyManagementURL || updated.AccessEvaluationURL != created.AccessEvaluationURL || updated.UsageURL != created.UsageURL {
		t.Fatalf("updated Authorization profile = %#v", updated)
	}

	values, err := postgres.RuntimeCredentialSets(ctx, deployment.ID, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range values {
		if value.ID == updated.ID {
			found = value.Scope == "shared" && value.OwnerIntegrationID == "" && value.EnvironmentVariable == "VENDOR_API_KEY" && bytes.Equal(compactRuntimeAuthorizationJSON(value.AuthConfig), compactRuntimeAuthorizationJSON(updated.AuthConfig)) && value.KeyManagementURL == updated.KeyManagementURL && value.AccessEvaluationURL == updated.AccessEvaluationURL && value.UsageURL == updated.UsageURL
		}
	}
	if !found {
		t.Fatalf("persisted Authorization profile not found: integration=%s values=%#v", integration.ID, values)
	}

	secret, err := postgres.CreateSecret(ctx, model.Secret{
		ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, Name: "Authorization usage credential",
		Purpose: "runtime_credential", Ciphertext: []byte("ciphertext"), Nonce: []byte("nonce"), KeyVersion: 1, Fingerprint: "0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := postgres.CreateRuntimeCredentialVersion(ctx, model.RuntimeCredentialVersion{
		ID: storeTestUUID(t), CredentialSetID: updated.ID, SecretID: secret.ID, Fingerprint: secret.Fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = postgres.ActivateRuntimeCredentialVersion(ctx, deployment.ID, updated.ID, version.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	event, err := postgres.CreateAuthorizationUsageEvent(ctx, model.AuthorizationUsageEvent{
		ID: storeTestUUID(t), OrganisationID: deployment.OrganisationID, ProductID: deployment.ID,
		IntegrationID: integration.ID, AuthorizationID: updated.ID, URL: updated.UsageURL,
		AuthenticationType: updated.AuthenticationType, AuthConfig: updated.AuthConfig,
		CredentialVersionID: version.ID, CredentialSecretID: secret.ID, CredentialFingerprint: secret.Fingerprint,
		Payload: json.RawMessage(`{"event_id":"postgres-usage"}`), AvailableAt: time.Now().Add(-time.Minute),
	})
	if err != nil || event.State != "queued" {
		t.Fatalf("created usage event=%#v err=%v", event, err)
	}
	claimed, err := postgres.ClaimAuthorizationUsageEvents(ctx, "worker-one", time.Now().Add(-time.Minute), 10)
	if err != nil || len(claimed) != 1 || claimed[0].ID != event.ID || claimed[0].Attempts != 1 {
		t.Fatalf("claimed usage events=%#v err=%v", claimed, err)
	}
	reclaimed, err := postgres.ClaimAuthorizationUsageEvents(ctx, "worker-two", time.Now().Add(time.Minute), 10)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != event.ID || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed abandoned usage event=%#v err=%v", reclaimed, err)
	}
	if err := postgres.CompleteAuthorizationUsageEvent(ctx, event.ID, "worker-two", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}
