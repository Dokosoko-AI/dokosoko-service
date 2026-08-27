package tools

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type authorizationHookResolver struct{}

func (authorizationHookResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

type authorizationHookDoer func(*http.Request) (*http.Response, error)

func (do authorizationHookDoer) Do(request *http.Request) (*http.Response, error) { return do(request) }

type authorizationHookCredentialResolver struct{}

func (authorizationHookCredentialResolver) ResolveToolCredential(context.Context, model.Tool) ([]byte, error) {
	return []byte("hook-secret"), nil
}

func hookResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestAuthorizationAccessEvaluationAllowsStrictDecision(t *testing.T) {
	memory := store.NewMemory()
	runtime := NewRuntime(memory, authorizationHookResolver{}, authorizationHookDoer(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://hooks.example.test/access" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer hook-secret" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var payload accessEvaluationRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.AuthorizationID != "authorization-1" || payload.APIID != "integration-1" || payload.Tool != "records.read" || payload.Subject != "customer-1" {
			t.Fatalf("payload = %#v", payload)
		}
		return hookResponse(http.StatusOK, `{"allow":true,"decision_id":"decision-1"}`), nil
	}))
	runtime.SetCredentialResolver(authorizationHookCredentialResolver{})
	tool := model.Tool{ID: "tool-1", OrganisationID: "org_acme", ProductID: "prod_acme", OwnerIntegrationID: "integration-1", Namespace: "records", Name: "read", Revision: 3, HTTPMethod: http.MethodGet, HTTPPath: "/records", RuntimeServiceConnectionID: "connection-1", RuntimeCredentialSetID: "authorization-1", RuntimeCredentialVersionID: "version-1", CredentialID: "secret-1", AccessEvaluationURL: "https://hooks.example.test/access", UpstreamAuth: json.RawMessage(`{"type":"bearer"}`)}
	decision, err := runtime.evaluateAuthorization(context.Background(), "prod_acme", "records.read", tool, Principal{Subject: "customer-1", RequestID: "request-1"}, BoundAuthorization{IntegrationID: "integration-1"})
	if err != nil || decision != "decision-1" {
		t.Fatalf("decision=%q err=%v", decision, err)
	}
}

func TestAuthorizationAccessEvaluationFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "denied", status: http.StatusOK, body: `{"allow":false,"decision_id":"decision-1"}`},
		{name: "missing decision", status: http.StatusOK, body: `{"allow":true,"decision_id":""}`},
		{name: "unknown field", status: http.StatusOK, body: `{"allow":true,"decision_id":"decision-1","reason":"extra"}`},
		{name: "invalid json", status: http.StatusOK, body: `{`},
		{name: "upstream status", status: http.StatusServiceUnavailable, body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(store.NewMemory(), authorizationHookResolver{}, authorizationHookDoer(func(*http.Request) (*http.Response, error) {
				return hookResponse(test.status, test.body), nil
			}))
			runtime.SetCredentialResolver(authorizationHookCredentialResolver{})
			tool := model.Tool{ID: "tool-1", OrganisationID: "org_acme", ProductID: "prod_acme", Revision: 1, HTTPMethod: http.MethodPost, HTTPPath: "/records", RuntimeServiceConnectionID: "connection-1", RuntimeCredentialSetID: "authorization-1", RuntimeCredentialVersionID: "version-1", CredentialID: "secret-1", AccessEvaluationURL: "https://hooks.example.test/access", UpstreamAuth: json.RawMessage(`{"type":"bearer"}`)}
			if _, err := runtime.evaluateAuthorization(context.Background(), "prod_acme", "records.write", tool, Principal{Subject: "customer-1"}, BoundAuthorization{IntegrationID: "integration-1"}); err != ErrDenied {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
