package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestRuntimeSetupHTTPShortestPathMasksCredential(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x66}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	integration, err := service.CreateIntegration(context.Background(), platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice API contract.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "http://localhost:8080")
	const credential = "must-never-appear-in-a-response"
	configured := request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/authorization", "doko_admin_demo", `{"environment_id":"env_prod","base_url":"http://voice.complicatedauth.localhost:38080","authentication_type":"api_key_header","environment_variable":"VOICE_API_KEY","key_management_url":"https://dashboard.example.test/keys","access_evaluation_url":"https://hooks.example.test/access","usage_url":"https://hooks.example.test/usage","credential":"`+credential+`"}`)
	if configured.Code != http.StatusOK {
		t.Fatalf("configure status=%d body=%s", configured.Code, configured.Body.String())
	}
	if strings.Contains(configured.Body.String(), credential) || strings.Contains(configured.Body.String(), "secret_id") || strings.Contains(configured.Body.String(), "ciphertext") {
		t.Fatalf("configure response leaked secret material: %s", configured.Body.String())
	}
	var setup struct {
		CredentialSets []model.RuntimeCredentialSet     `json:"authorizations"`
		Connections    []model.RuntimeServiceConnection `json:"endpoint_bindings"`
	}
	if err := json.Unmarshal(configured.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if len(setup.CredentialSets) != 1 || setup.CredentialSets[0].Name != integration.DisplayName || setup.CredentialSets[0].EnvironmentVariable != "VOICE_API_KEY" || !setup.CredentialSets[0].CredentialPresent {
		t.Fatalf("credential setup = %#v", setup.CredentialSets)
	}
	if len(setup.Connections) != 1 || setup.Connections[0].CurrentRevisions[0].BaseURL != "http://voice.complicatedauth.localhost:38080" {
		t.Fatalf("service connection = %#v", setup.Connections)
	}

	read := request(t, handler, http.MethodGet, "/api/v1/integrations/"+integration.ID+"/authorization", "doko_admin_demo", "")
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), credential) || strings.Contains(read.Body.String(), "secret_id") {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}

	usage := request(t, handler, http.MethodGet, "/api/v1/authorizations/"+setup.CredentialSets[0].ID+"/usage", "doko_admin_demo", "")
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"count":1`) {
		t.Fatalf("usage status=%d body=%s", usage.Code, usage.Body.String())
	}
}

func TestRuntimeAuthorizationProfilesHTTPReturnsOnlySharedRedactedProfiles(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	integration, err := service.CreateIntegration(context.Background(), platform.IntegrationInput{FamilyKey: "profiles", VersionKey: "v1", DisplayName: "Profiles API", Lifecycle: "active"}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	const credential = "shared-profile-secret"
	profile, err := service.CreateRuntimeCredentialSet(context.Background(), integration.ID, platform.RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "shared", Name: "Shared Authorization", EnvironmentVariable: "SHARED_API_KEY", AuthenticationType: "api_key_header", HeaderName: "X-Service-Key", KeyManagementURL: "https://dashboard.example.test/keys", AccessEvaluationURL: "https://hooks.example.test/access", UsageURL: "https://hooks.example.test/usage", Credential: credential}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "http://localhost:8080")
	response := request(t, handler, http.MethodGet, "/api/v1/authorizations", "doko_admin_demo", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), profile.ID) || !strings.Contains(response.Body.String(), `"key_management_url":"https://dashboard.example.test/keys"`) || strings.Contains(response.Body.String(), credential) || strings.Contains(response.Body.String(), "secret_id") {
		t.Fatalf("profiles status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeAccessHTTPCredentialLifecycleContract(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x67}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	integration, err := service.CreateIntegration(context.Background(), platform.IntegrationInput{FamilyKey: "face", VersionKey: "v1", DisplayName: "Face API", Description: "Face API contract.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, platform.Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "http://localhost:8080")

	const initialCredential = "initial-runtime-credential-must-stay-private"
	created := request(t, handler, http.MethodPut, "/api/v1/integrations/"+integration.ID+"/authorization", "doko_admin_demo", `{"environment_id":"env_prod","base_url":"http://face.complicatedauth.localhost:38080","authentication_type":"api_key_header","environment_variable":"FACE_API_KEY","header_name":"X-Face-API-Key","key_management_url":"https://dashboard.example.test/keys","access_evaluation_url":"https://hooks.example.test/access","usage_url":"https://hooks.example.test/usage","credential":"`+initialCredential+`"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create credential status=%d body=%s", created.Code, created.Body.String())
	}
	assertRuntimeAccessResponseRedacted(t, created.Body.String(), initialCredential)
	var setup model.RuntimeSetup
	if err := json.Unmarshal(created.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if len(setup.CredentialSets) != 1 || len(setup.Connections) != 1 {
		t.Fatalf("Authorization setup = %#v", setup)
	}
	credentialSet := setup.CredentialSets[0]
	if credentialSet.ID == "" || !credentialSet.CredentialPresent || len(credentialSet.Versions) != 1 {
		t.Fatalf("created credential set = %#v", credentialSet)
	}
	if credentialSet.Versions[0].SecretID != "" {
		t.Fatal("credential version exposed an internal vault identifier")
	}
	initialVersionID := credentialSet.Versions[0].ID

	read := request(t, handler, http.MethodGet, "/api/v1/authorizations/"+credentialSet.ID, "doko_admin_demo", "")
	if read.Code != http.StatusOK {
		t.Fatalf("read credential status=%d body=%s", read.Code, read.Body.String())
	}
	assertRuntimeAccessResponseRedacted(t, read.Body.String(), initialCredential)
	rename := request(t, handler, http.MethodPatch, "/api/v1/authorizations/"+credentialSet.ID, "doko_admin_demo", `{"name":"Renamed Authorization"}`)
	if rename.Code != http.StatusBadRequest {
		t.Fatalf("Authorization name must be immutable, status=%d body=%s", rename.Code, rename.Body.String())
	}

	serviceConnection := setup.Connections[0]
	if serviceConnection.ID == "" || len(serviceConnection.CurrentRevisions) != 1 || serviceConnection.CurrentRevisions[0].CredentialSetID != credentialSet.ID {
		t.Fatalf("created service connection = %#v", serviceConnection)
	}

	usage := request(t, handler, http.MethodGet, "/api/v1/authorizations/"+credentialSet.ID+"/usage", "doko_admin_demo", "")
	if usage.Code != http.StatusOK || !strings.Contains(usage.Body.String(), `"count":1`) || !strings.Contains(usage.Body.String(), serviceConnection.ID) {
		t.Fatalf("credential usage status=%d body=%s", usage.Code, usage.Body.String())
	}
	assertRuntimeAccessResponseRedacted(t, usage.Body.String(), initialCredential)

	const replacementCredential = "replacement-runtime-credential-must-stay-private"
	rotated := request(t, handler, http.MethodPost, "/api/v1/authorizations/"+credentialSet.ID+"/rotate", "doko_admin_demo", `{"credential":"`+replacementCredential+`"}`)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate credential status=%d body=%s", rotated.Code, rotated.Body.String())
	}
	assertRuntimeAccessResponseRedacted(t, rotated.Body.String(), initialCredential, replacementCredential)
	var rotatedSet model.RuntimeCredentialSet
	if err := json.Unmarshal(rotated.Body.Bytes(), &rotatedSet); err != nil {
		t.Fatal(err)
	}
	if !rotatedSet.CredentialPresent || len(rotatedSet.Versions) != 2 {
		t.Fatalf("rotated credential set = %#v", rotatedSet)
	}
	initialState := ""
	for _, version := range rotatedSet.Versions {
		if version.SecretID != "" {
			t.Fatal("rotated credential version exposed an internal vault identifier")
		}
		if version.ID == initialVersionID {
			initialState = version.State
		}
	}
	if initialState != "retiring" {
		t.Fatalf("initial version state after rotation = %q", initialState)
	}

	revoked := request(t, handler, http.MethodPost, "/api/v1/authorizations/"+credentialSet.ID+"/versions/"+initialVersionID+"/revoke", "doko_admin_demo", "")
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke credential status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	assertRuntimeAccessResponseRedacted(t, revoked.Body.String(), initialCredential, replacementCredential)
	var revokedSet model.RuntimeCredentialSet
	if err := json.Unmarshal(revoked.Body.Bytes(), &revokedSet); err != nil {
		t.Fatal(err)
	}
	revokedState := ""
	for _, version := range revokedSet.Versions {
		if version.ID == initialVersionID {
			revokedState = version.State
		}
	}
	if revokedState != "revoked" || !revokedSet.CredentialPresent {
		t.Fatalf("credential set after revoke = %#v", revokedSet)
	}
}

func assertRuntimeAccessResponseRedacted(t *testing.T, body string, credentials ...string) {
	t.Helper()
	for _, credential := range credentials {
		if strings.Contains(body, credential) {
			t.Fatalf("runtime Access response leaked credential value: %s", body)
		}
	}
	for _, field := range []string{`"secret_id"`, `"secretId"`, `"SecretID"`, `"ciphertext"`, `"nonce"`} {
		if strings.Contains(body, field) {
			t.Fatalf("runtime Access response exposed sensitive field %s: %s", field, body)
		}
	}
}
