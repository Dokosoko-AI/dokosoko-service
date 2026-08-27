package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/runtimeauth"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func newRuntimeServiceTest(t *testing.T) (*Service, *store.Memory) {
	t.Helper()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x58}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return NewWithVault(memory, vault), memory
}

func createRuntimeTestIntegration(t *testing.T, service *Service, family, displayName string) string {
	t.Helper()
	value, err := service.CreateIntegration(context.Background(), IntegrationInput{FamilyKey: family, VersionKey: "v1", DisplayName: displayName, Description: displayName + " contract.", Lifecycle: "active"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	return value.ID
}

const (
	testKeyManagementURL    = "https://dashboard.example.test/credentials"
	testAccessEvaluationURL = "https://hooks.example.test/access"
	testUsageURL            = "https://hooks.example.test/usage"
)

func TestRuntimeSetupCreatesReusableAuthorizationAndImmutableEndpointBinding(t *testing.T) {
	t.Parallel()
	service, memory := newRuntimeServiceTest(t)
	integrationID := createRuntimeTestIntegration(t, service, "voice-api", "Voice API")

	configured, err := service.ConfigureRuntimeSetup(context.Background(), integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://voice.example.test", AuthenticationType: "api_key_header", EnvironmentVariable: "VOICE_API_KEY", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "voice-secret-value"}, Actor{ID: "root-test", RequestID: "request-voice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(configured.CredentialSets) != 1 || configured.CredentialSets[0].EnvironmentVariable != "VOICE_API_KEY" || configured.CredentialSets[0].Scope != "shared" || configured.CredentialSets[0].OwnerIntegrationID != "" {
		t.Fatalf("credential set = %#v", configured.CredentialSets)
	}
	if !configured.CredentialSets[0].CredentialPresent || configured.CredentialSets[0].ActiveFingerprint == "" {
		t.Fatalf("credential presence = %#v", configured.CredentialSets[0])
	}
	if len(configured.Connections) != 1 || len(configured.Connections[0].CurrentRevisions) != 1 {
		t.Fatalf("connections = %#v", configured.Connections)
	}
	revision := configured.Connections[0].CurrentRevisions[0]
	if revision.AuthenticationType != "api_key_header" || revision.CredentialSetID != configured.CredentialSets[0].ID || revision.Revision != 1 || !revision.Current {
		t.Fatalf("connection revision = %#v", revision)
	}

	encoded, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("voice-secret-value")) || bytes.Contains(encoded, []byte(`"secret_id"`)) {
		t.Fatalf("runtime setup leaked secret material: %s", encoded)
	}
	version := configured.CredentialSets[0].Versions[0]
	stored, err := memory.Secret(context.Background(), "org_acme", version.SecretID)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decryptRuntimeCredential(service.vault, stored)
	if err != nil || string(plaintext) != "voice-secret-value" {
		t.Fatalf("encrypted credential round trip failed: value=%q err=%v", plaintext, err)
	}

	again, err := service.ConfigureRuntimeSetup(context.Background(), integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", ConnectionName: "Default", BaseURL: "https://voice.example.test", AuthenticationType: "api_key_header", AuthorizationID: configured.CredentialSets[0].ID}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := memory.RuntimeServiceConnectionRevisions(context.Background(), again.Connections[0].ID, "env_prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("idempotent setup created %d revisions", len(history))
	}
	readiness, err := service.RuntimeServiceConnectionReadiness(context.Background(), again.Connections[0].ID)
	if err != nil || !readiness.Ready {
		t.Fatalf("readiness before revoke = %#v err=%v", readiness, err)
	}
	activeVersionID := configured.CredentialSets[0].Versions[0].ID
	if _, err := service.RevokeRuntimeCredentialVersion(context.Background(), configured.CredentialSets[0].ID, activeVersionID, Actor{ID: "root-test"}); err != nil {
		t.Fatal(err)
	}
	readiness, err = service.RuntimeServiceConnectionReadiness(context.Background(), again.Connections[0].ID)
	if err != nil || readiness.Ready {
		t.Fatalf("readiness after revoke = %#v err=%v", readiness, err)
	}
}

func TestRuntimeAuthorizationAdditionalHeadersAreEncryptedAndMutableByName(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	integrationID := createRuntimeTestIntegration(t, service, "header-api", "Header API")
	headers := []RuntimeAuthorizationHeaderInput{{Name: "X-Tenant-Key", Value: "tenant-one"}, {Name: "X-Region", Value: "nz"}}
	profile, err := service.CreateRuntimeCredentialSet(context.Background(), integrationID, RuntimeCredentialSetInput{
		EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", EnvironmentVariable: "HEADER_API_KEY",
		KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL,
		Credential: "primary-one", AdditionalHeaders: &headers,
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil || bytes.Contains(encoded, []byte("tenant-one")) || bytes.Contains(encoded, []byte(`"value"`)) || !bytes.Contains(encoded, []byte("X-Tenant-Key")) {
		t.Fatalf("redacted profile = %s err=%v", encoded, err)
	}

	updatedHeaders := []RuntimeAuthorizationHeaderInput{{Name: "X-Tenant-Key", Value: ""}, {Name: "X-Trace-Key", Value: "trace-two"}}
	updated, err := service.UpdateRuntimeAuthorization(context.Background(), profile.ID, RuntimeAuthorizationUpdateInput{
		EnvironmentVariable: profile.EnvironmentVariable, AuthConfig: profile.AuthConfig,
		KeyManagementURL: profile.KeyManagementURL, AccessEvaluationURL: profile.AccessEvaluationURL, UsageURL: profile.UsageURL,
		State: profile.State, Revision: profile.Revision, AdditionalHeaders: &updatedHeaders,
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := service.runtimeCredentialMaterial(context.Background(), updated)
	if err != nil {
		t.Fatal(err)
	}
	primary, storedHeaders, bundled, err := runtimeauth.Decode(material)
	if err != nil || !bundled || string(primary) != "primary-one" || len(storedHeaders) != 2 || storedHeaders[0].Name != "X-Tenant-Key" || string(storedHeaders[0].Value) != "tenant-one" || storedHeaders[1].Name != "X-Trace-Key" || string(storedHeaders[1].Value) != "trace-two" {
		t.Fatalf("material = primary=%q headers=%#v bundled=%t err=%v", primary, storedHeaders, bundled, err)
	}
	if bytes.Contains(updated.AuthConfig, []byte("X-Region")) || !bytes.Contains(updated.AuthConfig, []byte("X-Trace-Key")) {
		t.Fatalf("auth config = %s", updated.AuthConfig)
	}
}

func TestRuntimeAuthorizationPromotesStoredHeaderValueWhenPrimaryIsDeleted(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	integrationID := createRuntimeTestIntegration(t, service, "promoted-header-api", "Promoted Header API")
	headers := []RuntimeAuthorizationHeaderInput{{Name: "X-Tenant-Key", Value: "tenant-secret"}}
	profile, err := service.CreateRuntimeCredentialSet(context.Background(), integrationID, RuntimeCredentialSetInput{
		EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "api_key_header", EnvironmentVariable: "PROMOTED_HEADER_API_KEY",
		HeaderName: "X-API-Key", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL,
		Credential: "primary-secret", AdditionalHeaders: &headers,
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}

	remainingHeaders := []RuntimeAuthorizationHeaderInput{}
	updated, err := service.UpdateRuntimeAuthorization(context.Background(), profile.ID, RuntimeAuthorizationUpdateInput{
		EnvironmentVariable: profile.EnvironmentVariable, HeaderName: "X-Tenant-Key", AuthConfig: profile.AuthConfig,
		KeyManagementURL: profile.KeyManagementURL, AccessEvaluationURL: profile.AccessEvaluationURL, UsageURL: profile.UsageURL,
		State: profile.State, Revision: profile.Revision, AdditionalHeaders: &remainingHeaders,
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := service.runtimeCredentialMaterial(context.Background(), updated)
	if err != nil {
		t.Fatal(err)
	}
	primary, storedHeaders, _, err := runtimeauth.Decode(material)
	if err != nil || updated.HeaderName != "X-Tenant-Key" || string(primary) != "tenant-secret" || len(storedHeaders) != 0 {
		t.Fatalf("updated=%#v primary=%q headers=%#v err=%v", updated, primary, storedHeaders, err)
	}
}

func TestRuntimeSharedCredentialCanServeTwoAPIsAndRotatesWithoutRepublishingConnections(t *testing.T) {
	t.Parallel()
	service, memory := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	faceID := createRuntimeTestIntegration(t, service, "face", "Face API")

	shared, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "service-key-one"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if shared.EnvironmentVariable != "SERVICE_API_KEY" || shared.OwnerIntegrationID != "" {
		t.Fatalf("shared credential = %#v", shared)
	}
	voice, err := service.ConfigureRuntimeServiceConnection(context.Background(), voiceID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://voice.example.test", AuthenticationType: "bearer", CredentialSetID: shared.ID}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	face, err := service.ConfigureRuntimeServiceConnection(context.Background(), faceID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://face.example.test", AuthenticationType: "bearer", CredentialSetID: shared.ID}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := service.RuntimeCredentialUsage(context.Background(), shared.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(usage) != 2 {
		t.Fatalf("shared credential usage = %#v", usage)
	}
	voiceHash := voice.CurrentRevisions[0].ContentHash
	faceHash := face.CurrentRevisions[0].ContentHash
	rotated, err := service.RotateRuntimeCredential(context.Background(), shared.ID, "service-key-two", nil, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ActiveFingerprint == shared.ActiveFingerprint || len(rotated.Versions) != 2 {
		t.Fatalf("rotation did not create a new active version: %#v", rotated)
	}
	states := map[string]int{}
	for _, version := range rotated.Versions {
		states[version.State]++
	}
	if states["active"] != 1 || states["retiring"] != 1 {
		t.Fatalf("rotation states = %#v", states)
	}
	voiceAfter, _ := memory.RuntimeServiceConnection(context.Background(), "prod_acme", voice.ID)
	faceAfter, _ := memory.RuntimeServiceConnection(context.Background(), "prod_acme", face.ID)
	if voiceAfter.CurrentRevisions[0].ContentHash != voiceHash || faceAfter.CurrentRevisions[0].ContentHash != faceHash {
		t.Fatal("credential rotation rewrote pinned connection configuration")
	}
}

func TestRuntimeAuthorizationProfileOwnsReusableConfiguration(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice-auth-profile", "Voice Authorization API")
	faceID := createRuntimeTestIntegration(t, service, "face-auth-profile", "Face Authorization API")
	profile, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{
		EnvironmentID:       "env_prod",
		Scope:               "shared",
		Name:                "Vendor production Authorization",
		EnvironmentVariable: "VENDOR_API_KEY",
		AuthenticationType:  "oauth_client_credentials",
		AuthConfig:          json.RawMessage(`{"client_id":"client-one","token_url":"https://identity.example.test/oauth/token","scopes":["records.read"]}`),
		KeyManagementURL:    "https://dashboard.example.test/credentials",
		AccessEvaluationURL: testAccessEvaluationURL,
		UsageURL:            testUsageURL,
		Credential:          "client-secret-one",
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.KeyManagementURL != "https://dashboard.example.test/credentials" || profile.EnvironmentVariable != "VENDOR_API_KEY" || !bytes.Contains(profile.AuthConfig, []byte(`"client_id":"client-one"`)) {
		t.Fatalf("authorization profile = %#v", profile)
	}
	profiles, err := service.RuntimeAuthorizationProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].ID != profile.ID {
		t.Fatalf("reusable profiles = %#v err=%v", profiles, err)
	}
	configured, err := service.ConfigureRuntimeSetup(context.Background(), faceID, RuntimeSetupInput{
		EnvironmentID:      "env_prod",
		BaseURL:            "https://face.example.test",
		AuthenticationType: "bearer",
		AuthConfig:         json.RawMessage(`{"unexpected":"caller-copy"}`),
		AuthorizationID:    profile.ID,
	}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	revision := configured.Connections[0].CurrentRevisions[0]
	if revision.AuthenticationType != profile.AuthenticationType || !bytes.Equal(revision.AuthConfig, profile.AuthConfig) {
		t.Fatalf("connection did not inherit the exact profile: revision=%#v profile=%#v", revision, profile)
	}

	dedicated, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "dedicated", EnvironmentVariable: "VOICE_ONLY_KEY", AuthenticationType: "bearer", Credential: "voice-only"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	profiles, err = service.RuntimeAuthorizationProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].ID == dedicated.ID {
		t.Fatalf("dedicated credential leaked into reusable profiles: %#v err=%v", profiles, err)
	}
}

func TestRuntimeAuthorizationProfileRejectsUnsafeMetadata(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	integrationID := createRuntimeTestIntegration(t, service, "unsafe-profile", "Unsafe Profile API")
	for name, input := range map[string]RuntimeCredentialSetInput{
		"missing management URL":       {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", Credential: "secret"},
		"private management URL":       {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", KeyManagementURL: "http://10.0.0.1/keys", AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "secret"},
		"queried access hook":          {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL + "?tenant=one", UsageURL: testUsageURL, Credential: "secret"},
		"nonstandard remote hook port": {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: "https://hooks.example.test:8443/access", UsageURL: testUsageURL, Credential: "secret"},
		"unsafe header":                {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "custom_header", HeaderName: "Cookie", KeyManagementURL: "https://dashboard.example.test/keys", AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "secret"},
		"header injection":             {EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", KeyManagementURL: "https://dashboard.example.test/keys", AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "secret\r\nX-Evil: injected"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.CreateRuntimeCredentialSet(context.Background(), integrationID, input, Actor{ID: "root-test"}); err == nil {
				t.Fatal("unsafe Authorization profile was accepted")
			}
		})
	}
}

func TestRuntimeDedicatedCredentialCannotCrossAPIBoundary(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	faceID := createRuntimeTestIntegration(t, service, "face", "Face API")
	dedicated, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "dedicated", AuthenticationType: "bearer", Credential: "voice-only"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ConfigureRuntimeServiceConnection(context.Background(), faceID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://face.example.test", AuthenticationType: "bearer", CredentialSetID: dedicated.ID}, Actor{ID: "root-test"})
	if err == nil || errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-API dedicated credential error = %v", err)
	}
}

func TestRuntimeCredentialEnvironmentVariableIsUniquePerEnvironment(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	faceID := createRuntimeTestIntegration(t, service, "face", "Face API")
	if _, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "dedicated", EnvironmentVariable: "SERVICE_API_KEY", AuthenticationType: "bearer", Credential: "one"}, Actor{ID: "root-test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateRuntimeCredentialSet(context.Background(), faceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "dedicated", EnvironmentVariable: "SERVICE_API_KEY", AuthenticationType: "bearer", Credential: "two"}, Actor{ID: "root-test"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate environment variable error = %v", err)
	}
}

func TestRuntimeServiceAuthenticationConfigRejectsSecretFields(t *testing.T) {
	t.Parallel()
	service, _ := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	_, err := service.ConfigureRuntimeServiceConnection(context.Background(), voiceID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://voice.example.test", AuthenticationType: "none", AuthConfig: json.RawMessage(`{"api_key":"must-not-live-in-config"}`)}, Actor{ID: "root-test"})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("secret-bearing auth config error = %v", err)
	}
}

func TestRuntimeToolMakesServiceAccessRequiredAndPublishesExactRedactedConnection(t *testing.T) {
	service, _ := newRuntimeServiceTest(t)
	ctx := context.Background()
	actor := Actor{ID: "root-test"}
	integrationID := createRuntimeTestIntegration(t, service, "voice-publication", "Voice Publication API")

	preflight, err := service.IntegrationPreflight(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccess := integrationPreflightCheckByCode(t, preflight, "service_access")
	if serviceAccess.Required || serviceAccess.Status != preflightOptional {
		t.Fatalf("service access without a runtime tool = %#v", serviceAccess)
	}

	setup, err := service.ConfigureRuntimeSetup(ctx, integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://voice-one.example.test", AuthenticationType: "bearer", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "publication-secret"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	connection, credentialSet := setup.Connections[0], setup.CredentialSets[0]
	draft, err := service.CreateTool(ctx, runtimeHTTPToolInput(integrationID, connection.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	publishedTool, err := service.PublishTool(ctx, draft.ProductID, draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integrationID, "", AuthorizationPointInput{Key: "voice.status.read", Name: "Read voice status", Description: "Read the voice service status.", ActionType: "read", DecisionTTLSeconds: 300, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetIntegrationToolBindings(ctx, integrationID, []ToolRevisionSelection{{ToolID: publishedTool.ID, Revision: publishedTool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}, actor); err != nil {
		t.Fatal(err)
	}

	status, err := service.IntegrationPublishStatus(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot integrationSnapshot
	if err = json.Unmarshal(status.CurrentSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ServiceConnections) != 1 || snapshot.ServiceConnections[0].ConnectionID != connection.ID || snapshot.ServiceConnections[0].ConnectionRevision != connection.Revision || len(snapshot.ServiceConnections[0].CurrentRevisions) != 1 {
		t.Fatalf("runtime connection snapshot = %#v", snapshot.ServiceConnections)
	}
	revision := snapshot.ServiceConnections[0].CurrentRevisions[0]
	if revision.RevisionID != connection.CurrentRevisions[0].ID || revision.Revision != 1 || revision.EnvironmentID != "env_prod" || revision.BaseURL != "https://voice-one.example.test" || revision.AuthenticationType != "bearer" || revision.CredentialSetID != credentialSet.ID || revision.ContentHash != connection.CurrentRevisions[0].ContentHash || !revision.Current || !revision.CredentialReady {
		t.Fatalf("runtime connection revision snapshot = %#v", revision)
	}
	if bytes.Contains(status.CurrentSnapshot, []byte("publication-secret")) || bytes.Contains(status.CurrentSnapshot, []byte(`"secret_id"`)) || bytes.Contains(status.CurrentSnapshot, []byte(`"fingerprint"`)) {
		t.Fatalf("publication snapshot leaked credential material: %s", status.CurrentSnapshot)
	}
	preflight, err = service.IntegrationPreflight(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccess = integrationPreflightCheckByCode(t, preflight, "service_access")
	if !serviceAccess.Required || serviceAccess.Status != preflightPass {
		t.Fatalf("runtime tool service access = %#v", serviceAccess)
	}

	firstHash := status.CurrentManifestHash
	rotatedCredentialSet, err := service.RotateRuntimeCredential(ctx, credentialSet.ID, "rotated-publication-secret", nil, actor)
	if err != nil {
		t.Fatal(err)
	}
	activeVersionID := ""
	for _, version := range rotatedCredentialSet.Versions {
		if version.State == "active" {
			activeVersionID = version.ID
		}
	}
	if activeVersionID == "" {
		t.Fatalf("rotated credential has no active version: %#v", rotatedCredentialSet.Versions)
	}
	status, err = service.IntegrationPublishStatus(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentManifestHash != firstHash {
		t.Fatal("credential rotation changed the secret-free Integration manifest hash")
	}
	if bytes.Contains(status.CurrentSnapshot, []byte("rotated-publication-secret")) || bytes.Contains(status.CurrentSnapshot, []byte(rotatedCredentialSet.ActiveFingerprint)) {
		t.Fatalf("rotated credential metadata leaked into the Integration snapshot: %s", status.CurrentSnapshot)
	}

	connection, err = service.ConfigureRuntimeServiceConnection(ctx, integrationID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://voice-two.example.test", AuthenticationType: "bearer", CredentialSetID: credentialSet.ID}, actor)
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.IntegrationPublishStatus(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentManifestHash == firstHash {
		t.Fatal("runtime connection revision did not change the Integration manifest hash")
	}
	if err = json.Unmarshal(status.CurrentSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if got := snapshot.ServiceConnections[0].CurrentRevisions[0]; got.RevisionID != connection.CurrentRevisions[0].ID || got.Revision != 2 || got.BaseURL != "https://voice-two.example.test" {
		t.Fatalf("revised runtime connection snapshot = %#v", got)
	}

	if _, err = service.RevokeRuntimeCredentialVersion(ctx, credentialSet.ID, activeVersionID, actor); err != nil {
		t.Fatal(err)
	}
	status, err = service.IntegrationPublishStatus(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || !integrationPublishValidationExists(status.Validations, "access_missing") || !integrationPublishValidationExists(status.Validations, "runtime_service_credential_unavailable") {
		t.Fatalf("publication status after credential revoke = %#v", status.Validations)
	}
	if err = json.Unmarshal(status.CurrentSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ServiceConnections[0].CurrentRevisions[0].CredentialReady {
		t.Fatal("revoked runtime credential remained ready in the candidate snapshot")
	}
	preflight, err = service.IntegrationPreflight(ctx, integrationID)
	if err != nil {
		t.Fatal(err)
	}
	serviceAccess = integrationPreflightCheckByCode(t, preflight, "service_access")
	if !serviceAccess.Required || serviceAccess.Status != preflightFail {
		t.Fatalf("service access after credential revoke = %#v", serviceAccess)
	}
}

func integrationPreflightCheckByCode(t *testing.T, result IntegrationPreflightResult, code string) IntegrationPreflightCheck {
	t.Helper()
	for _, check := range result.Checks {
		if check.Code == code {
			return check
		}
	}
	t.Fatalf("preflight check %q not found in %#v", code, result.Checks)
	return IntegrationPreflightCheck{}
}

func integrationPublishValidationExists(values []IntegrationPublishValidation, code string) bool {
	for _, validation := range values {
		if validation.Code == code {
			return true
		}
	}
	return false
}
