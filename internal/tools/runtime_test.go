package tools_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type runtimeResolver struct{ address net.IP }

func (r runtimeResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

type runtimeDoer func(*http.Request) (*http.Response, error)

func (function runtimeDoer) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func registerGrant(t *testing.T, service *platform.Service, key string) {
	t.Helper()
	if _, err := service.SaveGrantDefinition(context.Background(), "", platform.GrantDefinitionInput{Key: key, DisplayName: key, Risk: "medium"}, platform.Actor{ID: "root"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePinsDefinitionValidatesSchemaAndEnforcesGrant(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x57}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "identity-1", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://identity.vendor.example", ClientID: "doko", DelegatedAPIOrigin: "https://api.vendor.example", State: "active"}); err != nil {
		t.Fatal(err)
	}
	registerGrant(t, service, "sandboxes.create")
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "projects", Name: "create_sandbox", Description: "Create an authorized sandbox.",
		InputSchema:  []byte(`{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string","maxLength":40}},"required":["name"]}`),
		OutputSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{"id":{"type":"string"}},"required":["id"]}`),
		Endpoint:     "https://api.vendor.example/v1/sandboxes", HTTPMethod: "POST",
		AuthorizationPolicy: []byte(`{"required_grants":["sandboxes.create"],"confirmation_required":false,"idempotency_required":true}`), TimeoutMS: 5000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err = service.PublishTool(context.Background(), tool.ProductID, tool.ID, tool.Revision, platform.Actor{ID: "root", RequestID: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, runtimeResolver{net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.String() != "https://api.vendor.example/v1/sandboxes" || request.Header.Get("Authorization") != "Bearer delegated-customer-token" {
			t.Fatalf("destination=%s auth=%q", request.URL, request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"sandbox-1"}`))}, nil
	}))
	if _, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo"}, tools.Principal{Subject: "user", Grants: map[string]bool{}, IdempotencyKey: "runtime-test-key-0001"}); err != tools.ErrDenied {
		t.Fatalf("missing grant error = %v", err)
	}
	if _, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo", "url": "https://attacker"}, tools.Principal{Subject: "user", Grants: map[string]bool{"sandboxes.create": true}, IdempotencyKey: "runtime-test-key-0002"}); err == nil {
		t.Fatal("unexpected schema argument was accepted")
	}
	if _, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo"}, tools.Principal{Subject: "user", Grants: map[string]bool{"sandboxes.create": true}, DelegatedAPIOrigin: "https://api.vendor.example", IdempotencyKey: "runtime-test-key-0003"}); err != tools.ErrDenied {
		t.Fatalf("missing delegated token error = %v", err)
	}
	value, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo"}, tools.Principal{Subject: "user", Grants: map[string]bool{"sandboxes.create": true}, DelegatedAPIOrigin: "https://api.vendor.example", DelegatedAccessToken: "delegated-customer-token", RequestID: "execute", IdempotencyKey: "runtime-test-key-0004"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || value.(map[string]any)["id"] != "sandbox-1" {
		t.Fatalf("calls=%d value=%#v", calls, value)
	}
}

func TestCreateHTTPToolRequiresConfiguredVendorOrigin(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	service := platform.New(memory)
	input := platform.ToolInput{OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "projects", Name: "create_sandbox", Description: "Create a sandbox.", InputSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{}}`), Endpoint: "https://other.example/v1/sandboxes", HTTPMethod: "POST"}
	if _, err := service.CreateTool(context.Background(), input, platform.Actor{ID: "root"}); err == nil {
		t.Fatal("HTTP tool was created without a configured vendor API origin")
	}
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "identity-1", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://identity.vendor.example", ClientID: "doko", DelegatedAPIOrigin: "https://api.vendor.example", State: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTool(context.Background(), input, platform.Actor{ID: "root"}); err == nil {
		t.Fatal("HTTP tool was created on a different origin")
	}
}
