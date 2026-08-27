package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type trackingSecretStore struct {
	*store.Memory
	active      map[string]bool
	deleteErr   error
	deleteCalls int
}

type cloneRevisionRaceStore struct {
	*trackingSecretStore
	blockNextTool atomic.Bool
	readStarted   chan struct{}
	resumeRead    chan struct{}
}

func (s *cloneRevisionRaceStore) Tool(ctx context.Context, productID, id string) (model.Tool, error) {
	if s.blockNextTool.CompareAndSwap(true, false) {
		close(s.readStarted)
		<-s.resumeRead
	}
	return s.trackingSecretStore.Tool(ctx, productID, id)
}

func newTrackingSecretStore() *trackingSecretStore {
	return &trackingSecretStore{Memory: store.NewMemory(), active: map[string]bool{}}
}

func (s *trackingSecretStore) CreateSecret(ctx context.Context, value model.Secret) (model.Secret, error) {
	created, err := s.Memory.CreateSecret(ctx, value)
	if err == nil {
		s.active[created.ID] = true
	}
	return created, err
}

func (s *trackingSecretStore) DeleteSecret(ctx context.Context, organisationID, id string) error {
	s.deleteCalls++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	err := s.Memory.DeleteSecret(ctx, organisationID, id)
	if err == nil {
		delete(s.active, id)
	}
	return err
}

func (s *trackingSecretStore) UpdateTool(ctx context.Context, value model.Tool, expected int64) (model.Tool, error) {
	prior, _ := s.Memory.Tool(ctx, value.ProductID, value.ID)
	updated, err := s.Memory.UpdateTool(ctx, value, expected)
	if err == nil && prior.CredentialID != "" && prior.CredentialID != updated.CredentialID {
		delete(s.active, prior.CredentialID)
	}
	return updated, err
}

func (s *trackingSecretStore) RetireTool(ctx context.Context, productID, id string, expected int64) (model.Tool, error) {
	prior, _ := s.Memory.Tool(ctx, productID, id)
	retired, err := s.Memory.RetireTool(ctx, productID, id, expected)
	if err == nil && prior.CredentialID != "" {
		delete(s.active, prior.CredentialID)
	}
	return retired, err
}

func staticToolInput() ToolInput {
	return ToolInput{
		OrganisationID:      "org_untrusted",
		ProductID:           "prod_acme",
		Namespace:           "status",
		Name:                "read",
		Description:         "Read service status.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`),
		Endpoint:            "https://api.example.test/v1/status",
		HTTPMethod:          "GET",
		UpstreamAuth:        json.RawMessage(`{"type":"bearer"}`),
		Credential:          "upstream-secret-value",
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:           5000,
	}
}

func TestCreateStaticHTTPToolDerivesTenantAndEncryptsCredential(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x4a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	input := staticToolInput()
	tool, err := service.CreateTool(context.Background(), input, Actor{ID: "root", RequestID: "create-static"})
	if err != nil {
		t.Fatal(err)
	}
	if tool.OrganisationID != "org_acme" {
		t.Fatalf("organisation = %q, want product organisation", tool.OrganisationID)
	}
	if tool.CredentialID == "" || tool.CredentialFingerprint == "" {
		t.Fatalf("credential metadata not recorded: %#v", tool)
	}
	encoded, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{input.Credential, tool.CredentialID, tool.CredentialFingerprint, "ciphertext"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public tool leaked %q: %s", forbidden, encoded)
		}
	}
	resolved, err := service.ResolveToolCredential(context.Background(), tool)
	if err != nil || string(resolved) != input.Credential {
		t.Fatalf("resolved credential = %q, err = %v", resolved, err)
	}
}

func TestCreateStaticHTTPToolRejectsReusableAuthorizationHeaders(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x4b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"bearer","headers":["X-Tenant-Key"]}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "reusable Authorizations") {
		t.Fatalf("additional header error = %v", err)
	}
}

func TestCreateHTTPToolRejectsEndpointSecretsAndInvalidMappings(t *testing.T) {
	memory := store.NewMemory()
	service := New(memory)
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	input.Credential = ""
	input.Endpoint = "https://api.example.test/v1/status?api_key=secret-value"
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "credential-free") {
		t.Fatalf("query endpoint error = %v", err)
	}
	input.Endpoint = "https://api.example.test/v1/status"
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"missing":"query"}}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "unknown input") {
		t.Fatalf("mapping error = %v", err)
	}
	input.RequestMapping = json.RawMessage(`{}`)
	input.Endpoint = "https://api.vendor.localhost/status"
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "fixed credential-free") {
		t.Fatalf("HTTPS localhost endpoint error = %v", err)
	}
}

func TestNormalizeToolUpstreamAuthRejectsUnsafeCustomHeaders(t *testing.T) {
	for _, header := range []string{"Authorization", "Host", "Cookie", "X-Forwarded-For", "X-HTTP-Method-Override", "X-Method-Override", "X-Original-URL", "X-Rewrite-URL", "Bad Header", "X-Āpi-Key"} {
		raw, _ := json.Marshal(ToolUpstreamAuth{Type: "custom_header", HeaderName: header})
		if _, _, _, err := normalizeToolUpstreamAuth(raw, nil, "", "secret-value"); err == nil {
			t.Fatalf("header %q was accepted", header)
		}
	}
}

func TestNormalizeToolUpstreamAuthSupportsFixedAuthorizationSchemes(t *testing.T) {
	for _, scheme := range []string{"Token", "ApiKey", "SSWS", "Vendor-V2"} {
		raw, _ := json.Marshal(ToolUpstreamAuth{Type: "authorization_scheme", Scheme: scheme})
		encoded, normalized, changed, err := normalizeToolUpstreamAuth(raw, nil, "", "encrypted-value")
		if err != nil || !changed || normalized.Scheme != scheme || normalized.Type != "authorization_scheme" {
			t.Fatalf("scheme %q normalization = %#v, changed=%v, err=%v", scheme, normalized, changed, err)
		}
		if bytes.Contains(encoded, []byte("encrypted-value")) {
			t.Fatalf("scheme %q leaked credential in public configuration: %s", scheme, encoded)
		}
	}
	for _, scheme := range []string{"", "Two Words", "Bad\rScheme", strings.Repeat("a", 65)} {
		raw, _ := json.Marshal(ToolUpstreamAuth{Type: "authorization_scheme", Scheme: scheme})
		if _, _, _, err := normalizeToolUpstreamAuth(raw, nil, "", "encrypted-value"); err == nil {
			t.Fatalf("invalid authorization scheme %q was accepted", scheme)
		}
	}
}

func TestNormalizeToolUpstreamAuthPreservesHeaderPrefixes(t *testing.T) {
	for _, authType := range []string{"api_key_header", "custom_header"} {
		raw, _ := json.Marshal(ToolUpstreamAuth{Type: authType, HeaderName: "X-Vendor-Key", Prefix: "  Token  "})
		encoded, normalized, changed, err := normalizeToolUpstreamAuth(raw, nil, "", "encrypted-value")
		if err != nil || !changed || normalized.Prefix != "Token" {
			t.Fatalf("%s prefix normalization = %#v, changed=%v, err=%v", authType, normalized, changed, err)
		}
		if !bytes.Contains(encoded, []byte(`"prefix":"Token"`)) || bytes.Contains(encoded, []byte("encrypted-value")) {
			t.Fatalf("%s public authentication configuration = %s", authType, encoded)
		}
	}
	for _, prefix := range []string{"Token\rInjected", "Token\nInjected", strings.Repeat("a", 65)} {
		raw, _ := json.Marshal(ToolUpstreamAuth{Type: "api_key_header", HeaderName: "X-Vendor-Key", Prefix: prefix})
		if _, _, _, err := normalizeToolUpstreamAuth(raw, nil, "", "encrypted-value"); err == nil {
			t.Fatalf("invalid API key header prefix %q was accepted", prefix)
		}
	}
}

func TestHTTPToolRejectsNonScalarURLAndHeaderMappings(t *testing.T) {
	memory := store.NewMemory()
	service := New(memory)
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	input.Credential = ""
	input.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"filters":{"type":"object","additionalProperties":false,"properties":{}}}}`)
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"filters":"query"}}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "scalar schema") {
		t.Fatalf("object query mapping error = %v", err)
	}
	input.HTTPMethod = "POST"
	input.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"values":{"type":"array","items":{"type":"string"}}}}`)
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"values":"header"}}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "scalar schema") {
		t.Fatalf("array header mapping error = %v", err)
	}
	input.HTTPMethod = "GET"
	input.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"x_http_method_override":{"type":"string"}}}`)
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"x_http_method_override":"header"}}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "unsafe header") {
		t.Fatalf("method override header mapping error = %v", err)
	}
}

func TestHTTPToolRequiresEveryPathPlaceholderInTheInputContract(t *testing.T) {
	service := New(store.NewMemory())
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	input.Credential = ""
	input.HTTPMethod = "DELETE"
	input.Endpoint = "https://api.example.test/v1/status/{status_id}"
	input.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status_id":{"type":"string"}}}`)
	input.RequestMapping = json.RawMessage(`{"parameter_locations":{"status_id":"path"}}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "must be required") {
		t.Fatalf("optional path placeholder error = %v", err)
	}
}

func TestHTTPToolExamplesAreValidatedAndPersisted(t *testing.T) {
	memory := store.NewMemory()
	service := New(memory)
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	input.Credential = ""
	input.RequestExample = json.RawMessage(`{}`)
	input.ResponseExample = json.RawMessage(`{"ready":true}`)

	created, err := service.CreateTool(context.Background(), input, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if string(created.RequestExample) != `{}` || string(created.ResponseExample) != `{"ready":true}` {
		t.Fatalf("examples were not persisted: request=%s response=%s", created.RequestExample, created.ResponseExample)
	}

	input.ResponseExample = json.RawMessage(`{"ready":"yes"}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "response example") {
		t.Fatalf("schema-invalid response example error = %v", err)
	}
	input.ResponseExample = json.RawMessage(`{"access_token":"secret-value"}`)
	if _, err := service.CreateTool(context.Background(), input, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "credential material") {
		t.Fatalf("credential-bearing response example error = %v", err)
	}
}

func toolUpdateInput(tool model.Tool) ToolInput {
	return ToolInput{
		ProductID:           tool.ProductID,
		Namespace:           tool.Namespace,
		Name:                tool.Name,
		Description:         tool.Description,
		InputSchema:         tool.InputSchema,
		OutputSchema:        tool.OutputSchema,
		Endpoint:            tool.BaseURL,
		HTTPMethod:          tool.HTTPMethod,
		UpstreamAuth:        tool.UpstreamAuth,
		RequestMapping:      tool.RequestMapping,
		ResponseMapping:     tool.ResponseMapping,
		RequestExample:      tool.RequestExample,
		ResponseExample:     tool.ResponseExample,
		AuthorizationPolicy: tool.AuthorizationPolicy,
		TimeoutMS:           tool.TimeoutMS,
	}
}

func TestStoredToolCredentialCannotBeRedirectedWithoutRotation(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x38}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	created, err := service.CreateTool(context.Background(), staticToolInput(), Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}

	update := toolUpdateInput(created)
	update.Endpoint = "https://attacker.example/v1/status"
	if _, err := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "re-enter") {
		t.Fatalf("redirected bearer credential error = %v", err)
	}

	update = toolUpdateInput(created)
	update.Endpoint = "https://api.example.test/v2/status"
	updated, err := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision, Actor{ID: "root"})
	if err != nil {
		t.Fatalf("same-origin path update failed: %v", err)
	}
	if updated.CredentialID != created.CredentialID {
		t.Fatal("same-origin path update unexpectedly rotated the credential")
	}
}

func TestOAuthToolCredentialCannotBeReusedForChangedClientConfiguration(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x39}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	input := staticToolInput()
	input.UpstreamAuth = json.RawMessage(`{"type":"oauth_client_credentials","client_id":"dokosoko","token_url":"https://identity.example.test/oauth/token","scopes":["status.read"]}`)
	created, err := service.CreateTool(context.Background(), input, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}

	update := toolUpdateInput(created)
	update.UpstreamAuth = json.RawMessage(`{"type":"oauth_client_credentials","client_id":"redirected","token_url":"https://identity.example.test/oauth/token","scopes":["status.read"]}`)
	if _, err := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "re-enter") {
		t.Fatalf("changed OAuth client credential error = %v", err)
	}
}

func TestToolCredentialLifecycleCleansFailedAndReplacedSecrets(t *testing.T) {
	memory := newTrackingSecretStore()
	vault, err := secrets.New(bytes.Repeat([]byte{0x3a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	created, err := service.CreateTool(context.Background(), staticToolInput(), Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.active) != 1 {
		t.Fatalf("active secrets after create = %d", len(memory.active))
	}
	if _, err := service.CreateTool(context.Background(), staticToolInput(), Actor{ID: "root"}); err == nil {
		t.Fatal("duplicate tool create unexpectedly succeeded")
	}
	if len(memory.active) != 1 {
		t.Fatalf("duplicate create leaked a secret; active = %d", len(memory.active))
	}

	update := toolUpdateInput(created)
	update.Credential = "rotated-secret"
	if _, err := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision+1, Actor{ID: "root"}); err == nil {
		t.Fatal("stale tool update unexpectedly succeeded")
	}
	if len(memory.active) != 1 {
		t.Fatalf("stale update leaked a secret; active = %d", len(memory.active))
	}

	memory.deleteCalls = 0
	updated, err := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.active) != 1 || memory.active[created.CredentialID] {
		t.Fatalf("rotation did not retire the prior secret: %#v", memory.active)
	}
	if memory.deleteCalls != 0 {
		t.Fatalf("successful rotation performed %d non-transactional secret deletes", memory.deleteCalls)
	}
	if _, err := memory.Secret(context.Background(), created.OrganisationID, created.CredentialID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old credential lookup error = %v", err)
	}

	update = toolUpdateInput(updated)
	update.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	update.Credential = ""
	if _, err := service.UpdateTool(context.Background(), updated.ProductID, updated.ID, update, updated.Revision, Actor{ID: "root"}); err != nil {
		t.Fatal(err)
	}
	if len(memory.active) != 0 {
		t.Fatalf("disabling stored authentication retained secrets: %#v", memory.active)
	}
	if memory.deleteCalls != 0 {
		t.Fatalf("successful credential removal performed %d non-transactional secret deletes", memory.deleteCalls)
	}
}

func TestToolCredentialCleanupFailuresAreJoinedWithWriteErrors(t *testing.T) {
	cleanupErr := errors.New("injected secret cleanup failure")
	for _, test := range []struct {
		name string
		run  func(context.Context, *Service, *trackingSecretStore, model.Tool) error
	}{
		{
			name: "create",
			run: func(ctx context.Context, service *Service, memory *trackingSecretStore, _ model.Tool) error {
				_, err := service.CreateTool(ctx, staticToolInput(), Actor{ID: "root"})
				return err
			},
		},
		{
			name: "update",
			run: func(ctx context.Context, service *Service, memory *trackingSecretStore, created model.Tool) error {
				input := toolUpdateInput(created)
				input.Credential = "replacement-that-cannot-be-attached"
				_, err := service.UpdateTool(ctx, created.ProductID, created.ID, input, created.Revision+1, Actor{ID: "root"})
				return err
			},
		},
		{
			name: "clone",
			run: func(ctx context.Context, service *Service, memory *trackingSecretStore, created model.Tool) error {
				input := ToolCloneInput{Namespace: "status", Name: "existing_clone", Credential: "first-clone-secret", Revision: created.Revision}
				if _, err := service.CloneTool(ctx, created.ProductID, created.ID, input, Actor{ID: "root"}); err != nil {
					return err
				}
				memory.deleteErr = cleanupErr
				input.Credential = "orphaned-clone-secret"
				_, err := service.CloneTool(ctx, created.ProductID, created.ID, input, Actor{ID: "root"})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			memory := newTrackingSecretStore()
			vault, err := secrets.New(bytes.Repeat([]byte{0x4c}, 32))
			if err != nil {
				t.Fatal(err)
			}
			service := NewWithVault(memory, vault)
			created, err := service.CreateTool(ctx, staticToolInput(), Actor{ID: "root"})
			if err != nil {
				t.Fatal(err)
			}
			if test.name != "clone" {
				memory.deleteErr = cleanupErr
			}
			memory.deleteCalls = 0
			err = test.run(ctx, service, memory, created)
			if !errors.Is(err, store.ErrConflict) || !errors.Is(err, cleanupErr) {
				t.Fatalf("error = %v, want write conflict joined with cleanup failure", err)
			}
			if memory.deleteCalls != 1 {
				t.Fatalf("cleanup attempts = %d, want 1", memory.deleteCalls)
			}
		})
	}
}

func TestCloneRequiresAnIndependentStaticCredential(t *testing.T) {
	memory := newTrackingSecretStore()
	vault, err := secrets.New(bytes.Repeat([]byte{0x3b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	created, err := service.CreateTool(context.Background(), staticToolInput(), Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	cloneInput := ToolCloneInput{Namespace: "status", Name: "read_copy", Revision: created.Revision}
	if _, err := service.CloneTool(context.Background(), created.ProductID, created.ID, cloneInput, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("credential-free static clone error = %v", err)
	}
	if len(memory.active) != 1 {
		t.Fatalf("failed clone changed active secrets: %#v", memory.active)
	}
	cloneInput.Credential = "independent-clone-secret"
	cloned, err := service.CloneTool(context.Background(), created.ProductID, created.ID, cloneInput, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if cloned.CredentialID == "" || cloned.CredentialID == created.CredentialID || len(memory.active) != 2 {
		t.Fatalf("clone credential isolation failed: source=%q clone=%q active=%#v", created.CredentialID, cloned.CredentialID, memory.active)
	}
	audits, err := memory.AuditEvents(context.Background(), created.OrganisationID)
	if err != nil {
		t.Fatal(err)
	}
	var cloneAudit *model.AuditEvent
	for index := range audits {
		if audits[index].Action == "tool.cloned" && audits[index].TargetID == cloned.ID {
			cloneAudit = &audits[index]
			break
		}
	}
	if cloneAudit == nil || cloneAudit.Current["source_revision"] != created.Revision || cloneAudit.Current["source_state"] != created.State {
		t.Fatalf("clone audit did not bind source revision/state: %#v", cloneAudit)
	}
}

func TestCloneRejectsConcurrentSourceRevisionBeforeCredentialCreation(t *testing.T) {
	memory := &cloneRevisionRaceStore{
		trackingSecretStore: newTrackingSecretStore(),
		readStarted:         make(chan struct{}),
		resumeRead:          make(chan struct{}),
	}
	vault, err := secrets.New(bytes.Repeat([]byte{0x3d}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	created, err := service.CreateTool(context.Background(), staticToolInput(), Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}

	memory.blockNextTool.Store(true)
	cloneErr := make(chan error, 1)
	go func() {
		_, cloneError := service.CloneTool(context.Background(), created.ProductID, created.ID, ToolCloneInput{
			Namespace: "status", Name: "stale_copy", Credential: "must-not-be-stored", Revision: created.Revision,
		}, Actor{ID: "cloning-admin"})
		cloneErr <- cloneError
	}()
	<-memory.readStarted

	update := toolUpdateInput(created)
	update.Endpoint = "https://changed.example.test/v2/status"
	update.UpstreamAuth = json.RawMessage(`{"type":"custom_header","header_name":"X-Changed-Key"}`)
	update.Credential = "updated-source-secret"
	updated, updateErr := service.UpdateTool(context.Background(), created.ProductID, created.ID, update, created.Revision, Actor{ID: "updating-admin"})
	close(memory.resumeRead)
	if updateErr != nil {
		t.Fatal(updateErr)
	}
	if updated.Revision == created.Revision {
		t.Fatal("concurrent source update did not advance the revision")
	}

	err = <-cloneErr
	if !errors.Is(err, ErrToolCloneRevisionStale) || !errors.Is(err, store.ErrConflict) {
		t.Fatalf("concurrent clone error = %v, want stale revision conflict", err)
	}
	if len(memory.active) != 1 || !memory.active[updated.CredentialID] {
		t.Fatalf("stale clone stored or disturbed credential material: %#v", memory.active)
	}
	tools, err := memory.Tools(context.Background(), created.ProductID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].ID != updated.ID {
		t.Fatalf("stale clone created a tool: %#v", tools)
	}
}

func TestRetireToolAtomicallyRevokesPurposeBoundCredential(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x3c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	created, err := service.CreateTool(ctx, staticToolInput(), Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	credentialID := created.CredentialID
	retired, err := service.RetireTool(ctx, created.ProductID, created.ID, created.Revision, Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if retired.State != "retired" || retired.CredentialID != "" || retired.CredentialPresent {
		t.Fatalf("retired tool retained credential metadata: %#v", retired)
	}
	if _, err := memory.Secret(ctx, created.OrganisationID, credentialID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("retired tool credential lookup error = %v", err)
	}
	if _, err := service.ResolveToolCredential(ctx, retired); err == nil {
		t.Fatal("retired tool credential unexpectedly resolved")
	}
}

func TestPublishAndDryRunRequireCurrentHTTPToolReview(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	legacy, err := memory.CreateTool(ctx, model.Tool{
		ID:                  "tool_legacy_schema",
		OrganisationID:      "org_acme",
		ProductID:           "prod_acme",
		Namespace:           "legacy",
		Name:                "lookup",
		Description:         "Legacy draft requiring review.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"string","pattern":"^x$"}}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		BaseURL:             "https://api.example.test/v1/legacy",
		HTTPMethod:          "GET",
		UpstreamAuth:        json.RawMessage(`{"type":"none"}`),
		RequestMapping:      json.RawMessage(`{}`),
		ResponseMapping:     json.RawMessage(`{}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:           5000,
		BackendKind:         "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishTool(ctx, legacy.ProductID, legacy.ID, legacy.Revision, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "requires review") {
		t.Fatalf("legacy publish error = %v", err)
	}
	if _, err := service.DryRunTool(ctx, legacy.ProductID, legacy.ID, map[string]any{"value": "x"}); err == nil || !strings.Contains(err.Error(), "requires review") {
		t.Fatalf("legacy dry-run error = %v", err)
	}
	stored, err := memory.Tool(ctx, legacy.ProductID, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "draft" || stored.Revision != legacy.Revision {
		t.Fatalf("failed preflight mutated legacy draft: %#v", stored)
	}

	canonicalFixture := model.Tool{
		OrganisationID:      "org_acme",
		ProductID:           "prod_acme",
		Namespace:           "legacy",
		Description:         "Legacy non-canonical draft.",
		InputSchema:         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		BaseURL:             "https://api.example.test/v1/legacy",
		HTTPMethod:          "GET",
		UpstreamAuth:        json.RawMessage(`{"type":"none"}`),
		RequestMapping:      json.RawMessage(`{}`),
		ResponseMapping:     json.RawMessage(`{}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`),
		TimeoutMS:           5000,
		BackendKind:         "http",
	}
	for name, mutate := range map[string]func(*model.Tool){
		"endpoint whitespace": func(value *model.Tool) {
			value.ID, value.Name, value.BaseURL = "tool_legacy_endpoint_whitespace", "endpoint_whitespace", " https://api.example.test/v1/legacy "
		},
		"auth whitespace": func(value *model.Tool) {
			value.ID, value.Name, value.UpstreamAuth = "tool_legacy_auth_whitespace", "auth_whitespace", json.RawMessage(`{"type":" none "}`)
		},
		"policy whitespace": func(value *model.Tool) {
			value.ID, value.Name, value.AuthorizationPolicy = "tool_legacy_policy_whitespace", "policy_whitespace", json.RawMessage(`{"required_grants":[" ITEMS.READ "],"confirmation_required":false,"risk":"low"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := canonicalFixture
			mutate(&fixture)
			created, err := memory.CreateTool(ctx, fixture)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.PublishTool(ctx, created.ProductID, created.ID, created.Revision, Actor{ID: "root"}); err == nil || !strings.Contains(err.Error(), "requires review") {
				t.Fatalf("non-canonical publish error = %v", err)
			}
		})
	}
}
