package tools_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko/v2/internal/platform"
	"github.com/dokosoko/dokosoko/v2/internal/secrets"
	"github.com/dokosoko/dokosoko/v2/internal/store"
	"github.com/dokosoko/dokosoko/v2/internal/tools"
)

type runtimeResolver struct{ address net.IP }

func (r runtimeResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

type runtimeDoer func(*http.Request) (*http.Response, error)

func (function runtimeDoer) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRuntimePinsDefinitionValidatesSchemaAndEnforcesEntitlement(t *testing.T) {
	t.Parallel()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x57}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "projects", Name: "create_sandbox", Description: "Create an authorized sandbox.",
		InputSchema:  []byte(`{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string","maxLength":40}},"required":["name"]}`),
		OutputSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{"id":{"type":"string"}},"required":["id"]}`),
		APIHookURL:   "https://api.vendor.example/v1/sandboxes", HTTPMethod: "POST", Credential: "tool-token",
		AuthorizationPolicy: []byte(`{"required_entitlements":["sandboxes.create"],"confirmation_required":false}`), TimeoutMS: 5000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err = service.PublishTool(context.Background(), tool.ProductID, tool.ID, tool.Revision, platform.Actor{ID: "root", RequestID: "publish"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := tools.NewRuntime(memory, vault, runtimeResolver{net.ParseIP("8.8.8.8")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.String() != "https://api.vendor.example/v1/sandboxes" || request.Header.Get("Authorization") != "Bearer tool-token" {
			t.Fatalf("destination=%s auth=%q", request.URL, request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"sandbox-1"}`))}, nil
	}))
	if _, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo"}, tools.Principal{Subject: "user", Entitlements: map[string]bool{}}); err != tools.ErrDenied {
		t.Fatalf("missing entitlement error = %v", err)
	}
	if _, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo", "url": "https://attacker"}, tools.Principal{Subject: "user", Entitlements: map[string]bool{"sandboxes.create": true}}); err == nil {
		t.Fatal("unexpected schema argument was accepted")
	}
	value, err := runtime.Execute(context.Background(), tool.ProductID, "projects.create_sandbox", map[string]any{"name": "demo"}, tools.Principal{Subject: "user", Entitlements: map[string]bool{"sandboxes.create": true}, RequestID: "execute"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || value.(map[string]any)["id"] != "sandbox-1" {
		t.Fatalf("calls=%d value=%#v", calls, value)
	}
}
