package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type authorizationResolver struct{}

func (authorizationResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("93.184.216.34")}, nil
}

type authorizationDoer struct {
	calls           int
	idempotencyKeys []string
}

func (d *authorizationDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.idempotencyKeys = append(d.idempotencyKeys, request.Header.Get("Idempotency-Key"))
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ready":true}`)), Request: request}, nil
}

func TestPrivateMCPListAndCallEnforceLiveExactAuthorizationPoint(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x29}, 32))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "idp_runtime", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://id.vendor.example", ClientID: "vendor-client", DelegatedAPIOrigin: "https://api.vendor.example", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := memory.ResolveCustomerAccount(ctx, identity.CustomerAccount{ID: "account_runtime", OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: "https://id.vendor.example", ExternalID: "customer-a", State: "active", LastAuthenticatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	secretID := "secret_runtime_upstream"
	encrypted, err := vault.Encrypt([]byte("vendor-access-token"), "org_acme:delegated:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: "org_acme", Name: "runtime-upstream", Purpose: "delegated_access_token", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	createToken := func(raw string, grants map[string]bool, evaluatedAt time.Time) string {
		t.Helper()
		digest := sha256.Sum256([]byte(raw))
		now := time.Now().UTC()
		if err := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: digest[:], ProductID: "prod_acme", ProviderRevision: provider.Revision, ClientID: "mcp-client", Resource: "https://dokosoko.example/mcp", Issuer: "https://id.vendor.example", Subject: "user-a", CustomerAccountID: account.ID, ExternalCustomerID: account.ExternalID, Grants: grants, AccessEvaluationID: "evaluation-1", AccessEvaluatedAt: evaluatedAt, PolicyVersion: "policy-1", UpstreamAccessSecretID: secretID, Scopes: []string{"mcp:private"}, ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		return "doko_at_" + raw
	}
	freshToken := createToken("fresh-runtime-token", map[string]bool{"platform.readiness": true}, time.Now().UTC())
	missingGrantToken := createToken("missing-runtime-token", map[string]bool{}, time.Now().UTC())
	staleToken := createToken("stale-runtime-token", map[string]bool{"platform.readiness": true}, time.Now().UTC().Add(-31*time.Second))

	service := platform.NewWithVault(memory, vault)
	actor := platform.Actor{ID: "root_runtime"}
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "runtime-api", VersionKey: "v1", DisplayName: "Runtime API", Description: "Runtime authorization acceptance.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := service.SaveGrantDefinition(ctx, "", platform.GrantDefinitionInput{Key: "platform.readiness", DisplayName: "Read readiness", Description: "Read readiness state.", Risk: "low", State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	point, err := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: "platform.readiness.check", Name: "Check readiness", Description: "Check runtime readiness.", ActionType: "write", RequiredGrants: []string{grant.Key}, ConfirmationRequired: true, DecisionTTLSeconds: 30, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_runtime_ready", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "platform", Name: "check_readiness", Description: "Check readiness.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`), BaseURL: "https://api.vendor.example/v1/ready", HTTPMethod: http.MethodPost, AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"idempotency_required":true}`), TimeoutMS: 5000, BackendKind: "http"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := service.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(map[string]any{"family_key": integration.FamilyKey, "version_key": integration.VersionKey, "display_name": integration.DisplayName, "description": integration.Description, "visibility": integration.Visibility, "lifecycle": integration.Lifecycle, "resource_sets": []any{}, "packages": []any{}, "authorization_points": []any{map[string]any{"id": point.ID, "key": point.Key, "name": point.Name, "action_type": point.ActionType, "required_grants": point.RequiredGrants, "confirmation_required": point.ConfirmationRequired, "decision_ttl_seconds": point.DecisionTTLSeconds, "revision": point.Revision}}, "tools": []any{map[string]any{"tool_id": tool.ID, "tool_revision": tool.Revision, "authorization_point_id": point.ID, "authorization_point_revision": point.Revision, "namespace": tool.Namespace, "name": tool.Name, "backend_kind": tool.BackendKind, "content_hash": "sha256:" + strings.Repeat("1", 64)}}, "access_connection_ids": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC()
	if _, err := memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{ID: "revision_runtime", IntegrationID: integration.ID, Revision: 1, State: "published", Snapshot: snapshot, ManifestHash: "sha256:" + strings.Repeat("2", 64), PublishedAt: &publishedAt}); err != nil {
		t.Fatal(err)
	}

	doer := &authorizationDoer{}
	runtime := toolruntime.NewRuntime(memory, authorizationResolver{}, doer)
	broker := identity.NewBroker(memory, vault, "https://dokosoko.example", nil, nil, nil)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", IdentityBroker: broker, ToolRuntime: runtime})

	listed := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"common.check_readiness"`) || !strings.Contains(listed.Body.String(), `"com.dokosoko/authorizationPointRevision":1`) || !strings.Contains(listed.Body.String(), `"com.dokosoko/confirmationRequired":true`) || !strings.Contains(listed.Body.String(), `"com.dokosoko/confirmationChallengeMetaField":"confirmation_challenge"`) || !strings.Contains(listed.Body.String(), `"com.dokosoko/idempotencyKeyRequired":true`) || !strings.Contains(listed.Body.String(), `"com.dokosoko/idempotencyKeyMetaField":"idempotency_key"`) {
		t.Fatalf("fresh exact discovery = %d: %s", listed.Code, listed.Body.String())
	}
	for name, token := range map[string]string{"missing grant": missingGrantToken, "stale decision": staleToken} {
		t.Run(name, func(t *testing.T) {
			response := request(t, handler, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
			if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"name":"common.check_readiness"`) {
				t.Fatalf("denied discovery = %d: %s", response.Code, response.Body.String())
			}
		})
	}
	unconfirmed := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"platform.check_readiness","arguments":{},"_meta":{"idempotency_key":"stable-mcp-call-001"}}}`)
	if unconfirmed.Code != http.StatusOK || !strings.Contains(unconfirmed.Body.String(), `"retry_metadata_field":"params._meta.confirmation_challenge"`) || !strings.Contains(unconfirmed.Body.String(), "does not independently prove that a human approved") || doer.calls != 0 {
		t.Fatalf("unconfirmed call = %d calls=%d: %s", unconfirmed.Code, doer.calls, unconfirmed.Body.String())
	}
	var confirmationEnvelope struct {
		Error struct {
			Data struct {
				Challenge string `json:"confirmation_challenge"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(unconfirmed.Body.Bytes(), &confirmationEnvelope); err != nil || confirmationEnvelope.Error.Data.Challenge == "" {
		t.Fatalf("confirmation challenge was not returned: err=%v body=%s", err, unconfirmed.Body.String())
	}
	rawBoolean := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"platform.check_readiness","arguments":{},"_meta":{"confirmed":true,"idempotency_key":"stable-mcp-call-001"}}}`)
	if rawBoolean.Code != http.StatusOK || !strings.Contains(rawBoolean.Body.String(), `"confirmation_challenge"`) || strings.Contains(rawBoolean.Body.String(), `"ready":true`) || doer.calls != 0 {
		t.Fatalf("raw confirmation boolean was sufficient: calls=%d body=%s", doer.calls, rawBoolean.Body.String())
	}
	confirmedBody := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"platform.check_readiness","arguments":{},"_meta":{"confirmed":true,"confirmation_challenge":"` + confirmationEnvelope.Error.Data.Challenge + `","idempotency_key":"stable-mcp-call-001"}}}`
	confirmed := request(t, handler, http.MethodPost, "/mcp", freshToken, confirmedBody)
	if confirmed.Code != http.StatusOK || !strings.Contains(confirmed.Body.String(), `"ready":true`) || doer.calls != 1 || len(doer.idempotencyKeys) != 1 || !strings.HasPrefix(doer.idempotencyKeys[0], "doko_") || doer.idempotencyKeys[0] == "stable-mcp-call-001" {
		t.Fatalf("confirmed call = %d calls=%d: %s", confirmed.Code, doer.calls, confirmed.Body.String())
	}
	replayed := request(t, handler, http.MethodPost, "/mcp", freshToken, confirmedBody)
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), "already used") || doer.calls != 1 {
		t.Fatalf("confirmation challenge replay was not denied: calls=%d body=%s", doer.calls, replayed.Body.String())
	}
	missingIdempotency := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"platform.check_readiness","arguments":{},"_meta":{"confirmed":true}}}`)
	if missingIdempotency.Code != http.StatusOK || !strings.Contains(missingIdempotency.Body.String(), "params._meta.idempotency_key") || doer.calls != 1 {
		t.Fatalf("missing idempotency call = %d calls=%d: %s", missingIdempotency.Code, doer.calls, missingIdempotency.Body.String())
	}

	point, err = service.SaveAuthorizationPoint(ctx, integration.ID, point.ID, platform.AuthorizationPointInput{Key: point.Key, Name: point.Name, Description: point.Description, ActionType: point.ActionType, RequiredGrants: point.RequiredGrants, ConfirmationRequired: point.ConfirmationRequired, DecisionTTLSeconds: point.DecisionTTLSeconds, State: "deprecated", Revision: point.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	afterChange := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{}}`)
	if afterChange.Code != http.StatusOK || strings.Contains(afterChange.Body.String(), `"name":"common.check_readiness"`) {
		t.Fatalf("changed point remained discoverable = %d: %s", afterChange.Code, afterChange.Body.String())
	}
	afterChangeCall := request(t, handler, http.MethodPost, "/mcp", freshToken, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"platform.check_readiness","arguments":{},"_meta":{"confirmed":true}}}`)
	if afterChangeCall.Code != http.StatusOK || !strings.Contains(afterChangeCall.Body.String(), "no unique exact authorization action") || doer.calls != 1 {
		t.Fatalf("changed point call = %d calls=%d: %s", afterChangeCall.Code, doer.calls, afterChangeCall.Body.String())
	}
}

func TestPrivateMCPScopesManagedToolsToPinnedCustomerIntegration(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x39}, 32))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := memory.SaveIdentityProvider(ctx, identity.ProviderConfig{ID: "idp_customer_scope", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://id.vendor.example", ClientID: "vendor-client", DelegatedAPIOrigin: "https://api.vendor.example", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	secretID := "secret_customer_scope"
	encrypted, err := vault.Encrypt([]byte("vendor-access-token"), "org_acme:delegated:"+secretID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.CreateSecret(ctx, model.Secret{ID: secretID, OrganisationID: "org_acme", Name: "customer-scope-upstream", Purpose: "delegated_access_token", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	actor := platform.Actor{ID: "root_customer_scope"}

	type integrationFixture struct {
		integration model.Integration
		point       model.AuthorizationPoint
		tool        model.Tool
	}
	createFixture := func(family, namespace string) integrationFixture {
		t.Helper()
		integration, createErr := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: family, VersionKey: "v1", DisplayName: namespace + " API", Description: "Customer-scoped " + namespace + " contract.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
		if createErr != nil {
			t.Fatal(createErr)
		}
		point, pointErr := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: namespace + ".records.read", Name: "Read " + namespace + " records", Description: "Read the selected customer's records.", ActionType: "read", DecisionTTLSeconds: 300, State: "active"}, actor)
		if pointErr != nil {
			t.Fatal(pointErr)
		}
		draft, toolErr := memory.CreateTool(ctx, model.Tool{ID: "tool_" + namespace, OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: namespace, Name: "records_read", Description: "Read records for " + namespace + ".", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`), BaseURL: "https://api.vendor.example/v1/records", HTTPMethod: http.MethodGet, AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http"})
		if toolErr != nil {
			t.Fatal(toolErr)
		}
		tool, toolErr := service.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, actor)
		if toolErr != nil {
			t.Fatal(toolErr)
		}
		snapshot, marshalErr := json.Marshal(map[string]any{"family_key": integration.FamilyKey, "version_key": integration.VersionKey, "display_name": integration.DisplayName, "description": integration.Description, "visibility": integration.Visibility, "lifecycle": integration.Lifecycle, "resource_sets": []any{}, "packages": []any{}, "authorization_points": []any{map[string]any{"id": point.ID, "key": point.Key, "name": point.Name, "action_type": point.ActionType, "required_grants": point.RequiredGrants, "confirmation_required": point.ConfirmationRequired, "decision_ttl_seconds": point.DecisionTTLSeconds, "revision": point.Revision}}, "tools": []any{map[string]any{"tool_id": tool.ID, "tool_revision": tool.Revision, "authorization_point_id": point.ID, "authorization_point_revision": point.Revision, "namespace": tool.Namespace, "name": tool.Name, "backend_kind": tool.BackendKind, "content_hash": "sha256:" + strings.Repeat("3", 64)}}, "access_connection_ids": []any{}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		publishedAt := time.Now().UTC()
		if _, createErr = memory.CreateIntegrationRevision(ctx, model.IntegrationRevision{ID: "revision_" + namespace, IntegrationID: integration.ID, Revision: 1, State: "published", Snapshot: snapshot, ManifestHash: "sha256:" + strings.Repeat("4", 64), PublishedAt: &publishedAt}); createErr != nil {
			t.Fatal(createErr)
		}
		return integrationFixture{integration: integration, point: point, tool: tool}
	}
	fixtureA := createFixture("customer-a-api", "customer_a")
	fixtureB := createFixture("customer-b-api", "customer_b")

	definition := model.ProductDefinition{
		ID:             "definition_customer_scope",
		OrganisationID: "org_acme",
		ProductID:      "prod_acme",
		State:          "published",
		Components: []model.ProductComponent{
			{ID: "component_customer_a", Slug: fixtureA.integration.FamilyKey, Releases: []model.ProductRelease{{ID: "release_customer_a", Version: fixtureA.integration.VersionKey, State: "published", Bindings: []model.ProductBinding{{ID: "binding_customer_a", Kind: "tool", ReferenceID: fixtureA.tool.ID, Name: fixtureA.tool.Namespace + "." + fixtureA.tool.Name, Scope: "component", Verified: true}}}}},
			{ID: "component_customer_b", Slug: fixtureB.integration.FamilyKey, Releases: []model.ProductRelease{{ID: "release_customer_b", Version: fixtureB.integration.VersionKey, State: "published", Bindings: []model.ProductBinding{{ID: "binding_customer_b", Kind: "tool", ReferenceID: fixtureB.tool.ID, Name: fixtureB.tool.Namespace + "." + fixtureB.tool.Name, Scope: "component", Verified: true}}}}},
		},
		Profiles: []model.ProductProfile{
			{ID: "profile_customer_a", Name: "Customer A", State: "published", Selections: []model.ProductProfileSelection{{ComponentID: "component_customer_a", ReleaseID: "release_customer_a"}}},
			{ID: "profile_customer_b", Name: "Customer B", State: "published", Selections: []model.ProductProfileSelection{{ComponentID: "component_customer_b", ReleaseID: "release_customer_b"}}},
		},
		Revision: 1,
	}
	versionA, err := memory.CreateProductVersion(ctx, model.ProductVersion{ID: "version_customer_a", OrganisationID: "org_acme", ProductID: "prod_acme", Version: "customer-a", ProfileID: "profile_customer_a", ProfileName: "Customer A", DefinitionRevision: 1, ReleaseStage: "active", RolloutPercentage: 100, PromotionState: "not_required", DriftStatus: "healthy", ManifestHash: "sha256:" + strings.Repeat("a", 64), Manifest: definition, IsLatest: true})
	if err != nil {
		t.Fatal(err)
	}
	versionB, err := memory.CreateProductVersion(ctx, model.ProductVersion{ID: "version_customer_b", OrganisationID: "org_acme", ProductID: "prod_acme", Version: "customer-b", ProfileID: "profile_customer_b", ProfileName: "Customer B", DefinitionRevision: 1, ReleaseStage: "active", RolloutPercentage: 100, PromotionState: "not_required", DriftStatus: "healthy", ManifestHash: "sha256:" + strings.Repeat("b", 64), Manifest: definition})
	if err != nil {
		t.Fatal(err)
	}

	createCustomerToken := func(accountID, externalID, raw string, version model.ProductVersion) string {
		t.Helper()
		account, accountErr := memory.ResolveCustomerAccount(ctx, identity.CustomerAccount{ID: accountID, OrganisationID: "org_acme", ProductID: "prod_acme", Issuer: "https://id.vendor.example", ExternalID: externalID, State: "active", LastAuthenticatedAt: time.Now().UTC()})
		if accountErr != nil {
			t.Fatal(accountErr)
		}
		if _, pinErr := service.SaveScopedProductVersionPin(ctx, "prod_acme", platform.ProductVersionPinInput{Scope: "customer", ScopeID: account.ID, ProductVersionID: version.ID, Reason: "Customer-specific Integration contract"}, actor); pinErr != nil {
			t.Fatal(pinErr)
		}
		digest := sha256.Sum256([]byte(raw))
		now := time.Now().UTC()
		if tokenErr := memory.CreateAccessToken(ctx, identity.AccessToken{Digest: digest[:], ProductID: "prod_acme", ProviderRevision: provider.Revision, ClientID: "mcp-client", Resource: "https://dokosoko.example/mcp", Issuer: "https://id.vendor.example", Subject: "user-" + externalID, CustomerAccountID: account.ID, ExternalCustomerID: account.ExternalID, Grants: map[string]bool{}, AccessEvaluationID: "evaluation-" + externalID, AccessEvaluatedAt: now, PolicyVersion: "policy-1", UpstreamAccessSecretID: secretID, Scopes: []string{"mcp:private"}, ExpiresAt: now.Add(time.Hour), CreatedAt: now}); tokenErr != nil {
			t.Fatal(tokenErr)
		}
		return "doko_at_" + raw
	}
	tokenA := createCustomerToken("account_customer_a", "customer-a", "customer-a-token", versionA)
	tokenB := createCustomerToken("account_customer_b", "customer-b", "customer-b-token", versionB)

	doer := &authorizationDoer{}
	runtime := toolruntime.NewRuntime(memory, authorizationResolver{}, doer)
	broker := identity.NewBroker(memory, vault, "https://dokosoko.example", nil, nil, nil)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", IdentityBroker: broker, ToolRuntime: runtime})

	for name, testCase := range map[string]struct {
		token   string
		present string
		absent  string
	}{
		"customer A": {token: tokenA, present: "customer_a.records_read", absent: "customer_b.records_read"},
		"customer B": {token: tokenB, present: "customer_b.records_read", absent: "customer_a.records_read"},
	} {
		t.Run(name+" discovery", func(t *testing.T) {
			listed := request(t, handler, http.MethodPost, "/mcp", testCase.token, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
			if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"`+testCase.present+`"`) || strings.Contains(listed.Body.String(), `"name":"`+testCase.absent+`"`) {
				t.Fatalf("scoped discovery = %d: %s", listed.Code, listed.Body.String())
			}
		})
	}

	denied := request(t, handler, http.MethodPost, "/mcp", tokenA, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"customer_b.records_read","arguments":{}}}`)
	if denied.Code != http.StatusOK || !strings.Contains(denied.Body.String(), "not included in the effective product version") || doer.calls != 0 {
		t.Fatalf("cross-customer call = %d calls=%d: %s", denied.Code, doer.calls, denied.Body.String())
	}
	allowed := request(t, handler, http.MethodPost, "/mcp", tokenA, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"customer_a.records_read","arguments":{}}}`)
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"ready":true`) || doer.calls != 1 {
		t.Fatalf("customer-scoped call = %d calls=%d: %s", allowed.Code, doer.calls, allowed.Body.String())
	}
}
