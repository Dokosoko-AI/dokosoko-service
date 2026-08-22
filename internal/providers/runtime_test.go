package providers_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/providers"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type resolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f resolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestProviderContractCreatesProjectIssuesOnceAndRevokes(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, _ := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	service := platform.NewWithVault(memory, vault)
	provider, err := service.CreateProvider(ctx, platform.ProviderInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Acme provider", BaseURL: "https://provider.example", Credential: "service-secret", RequiredGrants: []string{"developer.pro"}, MaxTTLSeconds: 3600}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer service-secret" {
			t.Fatalf("provider credential missing: %v", request.Header)
		}
		calls[request.URL.Path]++
		switch request.URL.Path {
		case "/v1/authorize":
			return response(`{"allowed":true}`), nil
		case "/v1/projects":
			return response(`{"project_id":"vendor-project-7","state":"active"}`), nil
		case "/v1/credentials":
			return response(`{"credential_id":"vendor-credential-9","credential":"temporary-secret-value","expires_at":"` + time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339) + `"}`), nil
		case "/v1/credentials/vendor-credential-9/revoke":
			return response(`{}`), nil
		default:
			t.Fatalf("unexpected provider path: %s", request.URL.Path)
			return nil, nil
		}
	})
	runtime := providers.New(memory, vault, resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}), doer)
	principal := providers.Principal{Subject: "issuer|subject", ExternalCustomerID: "vendor-org", Grants: map[string]bool{"developer.pro": true}, RequestID: "request-1"}
	project, err := runtime.CreateProject(ctx, "prod_acme", provider.ID, providers.ProjectRequest{EnvironmentID: "env_prod", Name: "SDK test", IdempotencyKey: "project-idempotency-0001", TTLSeconds: 1800}, principal)
	if err != nil || project.ExternalID != "vendor-project-7" {
		t.Fatalf("project = %#v, err = %v", project, err)
	}
	issued, err := runtime.IssueCredential(ctx, "prod_acme", provider.ID, providers.CredentialRequest{EnvironmentID: "env_prod", ProjectID: project.ID, Scopes: []string{"api:write"}, IdempotencyKey: "credential-idempotency-01", TTLSeconds: 1800}, principal)
	if err != nil || issued.Credential != "temporary-secret-value" || issued.Lease.SecretFingerprint == "" {
		t.Fatalf("issued = %#v, err = %v", issued, err)
	}
	repeated, err := runtime.IssueCredential(ctx, "prod_acme", provider.ID, providers.CredentialRequest{EnvironmentID: "env_prod", ProjectID: project.ID, Scopes: []string{"api:write"}, IdempotencyKey: "credential-idempotency-01", TTLSeconds: 1800}, principal)
	if err != nil || !repeated.Existing || repeated.Credential != "" || calls["/v1/credentials"] != 1 {
		t.Fatalf("idempotent issue = %#v, calls = %#v, err = %v", repeated, calls, err)
	}
	revoked, err := runtime.RevokeCredential(ctx, "prod_acme", issued.Lease.ID, principal)
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("revoked = %#v, err = %v", revoked, err)
	}
	if stored, _ := memory.CredentialLease(ctx, "prod_acme", issued.Lease.ID); stored.SecretFingerprint == "temporary-secret-value" {
		t.Fatal("credential plaintext was persisted")
	}
}

func TestProviderOperationsFailClosedOnGrantAndPrivateResolution(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	vault, _ := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	provider, err := platform.NewWithVault(memory, vault).CreateProvider(ctx, platform.ProviderInput{OrganisationID: "org_acme", ProductID: "prod_acme", Name: "Provider", BaseURL: "https://provider.example", Credential: "service-secret", RequiredGrants: []string{"developer.pro"}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := providers.New(memory, vault, resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}), doerFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unsafe request executed"); return nil, nil }))
	request := providers.ProjectRequest{EnvironmentID: "env_prod", Name: "test", IdempotencyKey: "project-idempotency-0002", TTLSeconds: 1800}
	if _, err := runtime.CreateProject(ctx, "prod_acme", provider.ID, request, providers.Principal{Subject: "user", Grants: map[string]bool{}}); err == nil {
		t.Fatal("missing grant was accepted")
	}
	if _, err := runtime.CreateProject(ctx, "prod_acme", provider.ID, request, providers.Principal{Subject: "user", Grants: map[string]bool{"developer.pro": true}}); err == nil {
		t.Fatal("private provider resolution was accepted")
	}
}
