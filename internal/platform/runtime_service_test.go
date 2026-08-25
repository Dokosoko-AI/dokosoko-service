package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

func TestRuntimeSetupCreatesDedicatedCredentialAndImmutableConnectionRevision(t *testing.T) {
	t.Parallel()
	service, memory := newRuntimeServiceTest(t)
	integrationID := createRuntimeTestIntegration(t, service, "voice-api", "Voice API")

	configured, err := service.ConfigureRuntimeSetup(context.Background(), integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://voice.example.test", AuthenticationType: "api_key_header", CredentialScope: "dedicated", Credential: "voice-secret-value"}, Actor{ID: "root-test", RequestID: "request-voice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(configured.CredentialSets) != 1 || configured.CredentialSets[0].EnvironmentVariable != "VOICE_API_KEY" || configured.CredentialSets[0].Scope != "dedicated" || configured.CredentialSets[0].OwnerIntegrationID != integrationID {
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

	again, err := service.ConfigureRuntimeSetup(context.Background(), integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", ConnectionName: "Default", BaseURL: "https://voice.example.test", AuthenticationType: "api_key_header", ExistingCredentialSetID: configured.CredentialSets[0].ID}, Actor{ID: "root-test"})
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

func TestRuntimeSharedCredentialCanServeTwoAPIsAndRotatesWithoutRepublishingConnections(t *testing.T) {
	t.Parallel()
	service, memory := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	faceID := createRuntimeTestIntegration(t, service, "face", "Face API")

	shared, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "shared", AuthenticationType: "bearer", Credential: "service-key-one"}, Actor{ID: "root-test"})
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

	setup, err := service.ConfigureRuntimeSetup(ctx, integrationID, RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://voice-one.example.test", AuthenticationType: "bearer", CredentialScope: "dedicated", Credential: "publication-secret"}, actor)
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
