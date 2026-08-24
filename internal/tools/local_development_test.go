package tools_test

import (
	"bytes"
	"context"
	"errors"
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

func TestRuntimeAllowsExactLocalhostToolOriginForLocalAcceptance(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x58}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	const origin = "http://api.complicatedauth.localhost:38080"
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "identity-1", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: origin, ClientID: "doko", DelegatedAPIOrigin: origin, State: "active"}); err != nil {
		t.Fatal(err)
	}
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "platform", Name: "check_readiness", Description: "Check readiness.",
		InputSchema:  []byte(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`),
		Endpoint:     origin + "/health/ready", HTTPMethod: "GET",
		AuthorizationPolicy: []byte(`{"required_grants":["platform.readiness"],"confirmation_required":false}`), TimeoutMS: 5000,
	}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	registerGrant(t, service, "platform.readiness")
	tool, err = service.PublishTool(context.Background(), tool.ProductID, tool.ID, tool.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, runtimeResolver{net.ParseIP("127.0.0.1")}, runtimeDoer(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != origin+"/health/ready" || request.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatalf("destination=%s auth=%q", request.URL, request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ready":true}`))}, nil
	}))
	value, err := runtime.Execute(context.Background(), tool.ProductID, "platform.check_readiness", map[string]any{}, tools.Principal{Subject: "user", Grants: map[string]bool{"platform.readiness": true}, DelegatedAPIOrigin: origin, DelegatedAccessToken: "delegated-token"})
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["ready"] != true {
		t.Fatalf("unexpected result: %#v", value)
	}
}

func TestRuntimeStillRejectsPublicHostnameResolvingToLoopback(t *testing.T) {
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x59}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	const origin = "https://api.vendor.example"
	if _, err := memory.SaveIdentityProvider(context.Background(), identity.ProviderConfig{ID: "identity-1", OrganisationID: "org_acme", DeploymentID: "prod_acme", Issuer: "https://identity.vendor.example", ClientID: "doko", DelegatedAPIOrigin: origin, State: "active"}); err != nil {
		t.Fatal(err)
	}
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "platform", Name: "check_readiness", Description: "Check readiness.",
		InputSchema: []byte(`{"type":"object","additionalProperties":false,"properties":{}}`), Endpoint: origin + "/health/ready", HTTPMethod: "GET",
		AuthorizationPolicy: []byte(`{"required_grants":["platform.readiness"],"confirmation_required":false}`), TimeoutMS: 5000,
	}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	registerGrant(t, service, "platform.readiness")
	tool, err = service.PublishTool(context.Background(), tool.ProductID, tool.ID, tool.Revision, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, runtimeResolver{net.ParseIP("127.0.0.1")}, nil)
	_, err = runtime.Execute(context.Background(), tool.ProductID, "platform.check_readiness", map[string]any{}, tools.Principal{Subject: "user", Grants: map[string]bool{"platform.readiness": true}, DelegatedAPIOrigin: origin, DelegatedAccessToken: "delegated-token"})
	if !errors.Is(err, tools.ErrUnsafeDestination) {
		t.Fatalf("public hostname resolving to loopback returned %v", err)
	}
}
