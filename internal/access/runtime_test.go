package access_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	accessruntime "github.com/dokosoko/dokosoko-service/internal/access"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
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

func accessFixture(t *testing.T, cardinality, credentialScope string, doer doerFunc, resolver resolverFunc) (*accessruntime.Runtime, *store.Memory, model.Integration, model.AccessConnection) {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service := platform.NewWithVault(memory, vault)
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice", VersionKey: "v2", DisplayName: "Voice API", Lifecycle: "active"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	operations := `{"required_grants":["developer.pro"],"max_ttl_seconds":3600,"credential_storage_mode":"one_time","authorize":{"method":"POST","path":"/v1/authorize"},"instances.create":{"method":"POST","path":"/v1/instances"},"credentials.create":{"method":"POST","path":"/v1/credentials"},"credentials.revoke":{"method":"POST","path":"/v1/credentials/{credential_id}/revoke"}}`
	definition, err := service.CreateAccessDefinition(ctx, platform.AccessDefinitionInput{ServiceKey: "acme-voice", Name: "Acme Voice", InstanceCardinality: cardinality, InstanceLabelSingular: "Project", InstanceLabelPlural: "Projects", CredentialScope: credentialScope, ManagementAuthType: "bearer", Operations: []byte(operations)}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := service.CreateAccessConnection(ctx, platform.AccessConnectionInput{AccessDefinitionID: definition.ID, EnvironmentID: "env_prod", Name: "Acme production", BaseURL: "https://provider.example", ManagementSecret: "management-secret", IntegrationIDs: []string{integration.ID}}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(connection.IntegrationIDs) != 1 || connection.IntegrationIDs[0] != integration.ID || connection.Definition == nil {
		t.Fatalf("connection binding was not persisted: %#v", connection)
	}
	if resolver == nil {
		resolver = resolverFunc(func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil })
	}
	return accessruntime.New(memory, vault, resolver, doer), memory, integration, connection
}

func TestMultiInstanceAccessCreatesListsIssuesOnceAndRevokes(t *testing.T) {
	ctx := context.Background()
	calls := map[string]int{}
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer management-secret" {
			t.Fatalf("management credential missing: %#v", request.Header)
		}
		calls[request.URL.Path]++
		switch request.URL.Path {
		case "/v1/authorize":
			return response(`{"allowed":true}`), nil
		case "/v1/instances":
			var payload struct {
				Owner map[string]string `json:"owner"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload.Owner["type"] != "installation" || payload.Owner["id"] != "installation-1" {
				t.Fatalf("provider owner context = %#v, err = %v", payload.Owner, err)
			}
			return response(`{"instance_id":"project-7","display_name":"SDK project","state":"active"}`), nil
		case "/v1/credentials":
			return response(`{"credential_id":"key-9","credential_material":{"api_key":"temporary-value"}}`), nil
		case "/v1/credentials/key-9/revoke":
			return response(`{}`), nil
		default:
			t.Fatalf("unexpected provider operation: %s", request.URL.Path)
			return nil, nil
		}
	})
	runtime, _, integration, connection := accessFixture(t, "many", "instance", doer, nil)
	principal := accessruntime.Principal{Subject: "issuer|subject", ExternalCustomerID: "customer-1", InstallationID: "installation-1", Grants: map[string]bool{"developer.pro": true}, RequestID: "request-1"}
	capabilities := runtime.Capabilities(ctx, "prod_acme", principal.Grants)
	if len(capabilities) != 1 || !capabilities[0].CanCreateInstance || !capabilities[0].CanCreateCredential || capabilities[0].InstanceLabel != "Project" {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	instanceRequest := accessruntime.InstanceRequest{IntegrationID: integration.ID, EnvironmentID: "env_prod", DisplayName: "SDK project", IdempotencyKey: "instance-idempotency-0001", TTLSeconds: 1800}
	instance, err := runtime.CreateInstance(ctx, "prod_acme", connection.ID, instanceRequest, principal)
	if err != nil || instance.ExternalID != "project-7" {
		t.Fatalf("instance = %#v, calls = %#v, err = %v", instance, calls, err)
	}
	if _, err := runtime.CreateInstance(ctx, "prod_acme", connection.ID, instanceRequest, principal); err != nil || calls["/v1/instances"] != 1 {
		t.Fatalf("instance idempotency failed: calls=%#v err=%v", calls, err)
	}
	instances, err := runtime.ListInstances(ctx, "prod_acme", connection.ID, integration.ID, principal)
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances = %#v, err = %v", instances, err)
	}
	credentialRequest := accessruntime.CredentialRequest{IntegrationID: integration.ID, EnvironmentID: "env_prod", AccessInstanceID: instance.ID, Scopes: []string{"voice:write"}, IdempotencyKey: "credential-idempotency-0001", TTLSeconds: 1800}
	issued, err := runtime.IssueCredential(ctx, "prod_acme", connection.ID, credentialRequest, principal)
	if err != nil || len(issued.CredentialMaterial) == 0 || issued.Credential.SecretFingerprint == "" {
		t.Fatalf("issued = %#v, err = %v", issued, err)
	}
	repeated, err := runtime.IssueCredential(ctx, "prod_acme", connection.ID, credentialRequest, principal)
	if err != nil || !repeated.Existing || len(repeated.CredentialMaterial) != 0 || calls["/v1/credentials"] != 1 {
		t.Fatalf("idempotent credential = %#v, calls = %#v, err = %v", repeated, calls, err)
	}
	credentials, err := runtime.ListCredentials(ctx, "prod_acme", connection.ID, integration.ID, instance.ID, principal)
	if err != nil || len(credentials) != 1 || credentials[0].EncryptedSecretID != "" {
		t.Fatalf("credentials = %#v, err = %v", credentials, err)
	}
	revoked, err := runtime.RevokeCredential(ctx, "prod_acme", issued.Credential.ID, principal)
	if err != nil || revoked.RevokedAt == nil || calls["/v1/credentials/key-9/revoke"] != 1 {
		t.Fatalf("revoked = %#v, calls = %#v, err = %v", revoked, calls, err)
	}
}

func TestSingleInstanceServiceSuppressesResourceCreation(t *testing.T) {
	ctx := context.Background()
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/authorize":
			return response(`{"allowed":true}`), nil
		case "/v1/credentials":
			return response(`{"credential_id":"key-single","credential":"one-time-key"}`), nil
		default:
			t.Fatalf("unexpected provider operation: %s", request.URL.Path)
			return nil, nil
		}
	})
	runtime, _, integration, connection := accessFixture(t, "one", "connection", doer, nil)
	principal := accessruntime.Principal{Subject: "issuer|subject", Grants: map[string]bool{"developer.pro": true}}
	capabilities := runtime.Capabilities(ctx, "prod_acme", principal.Grants)
	if len(capabilities) != 1 || capabilities[0].CanCreateInstance || capabilities[0].InstanceCardinality != "one" {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if _, err := runtime.CreateInstance(ctx, "prod_acme", connection.ID, accessruntime.InstanceRequest{IntegrationID: integration.ID}, principal); err != accessruntime.ErrUnsupported {
		t.Fatalf("single-instance create error = %v", err)
	}
	issued, err := runtime.IssueCredential(ctx, "prod_acme", connection.ID, accessruntime.CredentialRequest{IntegrationID: integration.ID, EnvironmentID: "env_prod", Scopes: []string{"voice:read"}, IdempotencyKey: "single-credential-0001"}, principal)
	if err != nil || issued.Credential.AccessInstanceID != "" {
		t.Fatalf("issued = %#v, err = %v", issued, err)
	}
}

func TestAccessRuntimeRejectsPrivateDestinationResolution(t *testing.T) {
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe destination reached the HTTP client")
		return nil, nil
	})
	runtime, _, integration, connection := accessFixture(t, "one", "connection", doer, resolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}))
	_, err := runtime.IssueCredential(context.Background(), "prod_acme", connection.ID, accessruntime.CredentialRequest{IntegrationID: integration.ID, EnvironmentID: "env_prod", IdempotencyKey: "unsafe-credential-0001"}, accessruntime.Principal{Subject: "issuer|subject", Grants: map[string]bool{"developer.pro": true}})
	if err != accessruntime.ErrUnsafeDestination {
		t.Fatalf("unsafe destination error = %v", err)
	}
}
