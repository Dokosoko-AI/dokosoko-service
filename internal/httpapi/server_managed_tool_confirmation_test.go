package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func managedToolConfirmationFixture(t *testing.T) (*Server, model.Tool, toolruntime.BoundAuthorization, identity.Principal, time.Time) {
	t.Helper()
	ctx := context.Background()
	memory := store.NewMemory()
	draft, err := memory.CreateTool(ctx, model.Tool{
		ID: "tool_confirmation", OrganisationID: "org_acme", ProductID: "prod_acme",
		Namespace: "records", Name: "update", Description: "Update a record.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"record_id":{"type":"string"},"enabled":{"type":"boolean"}},"required":["record_id","enabled"]}`),
		OutputSchema: json.RawMessage(`{"type":"object"}`), BaseURL: "https://api.vendor.example/records", HTTPMethod: "POST",
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"idempotency_required":true}`),
		TimeoutMS:           5000, BackendKind: "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := memory.PublishTool(ctx, draft.ProductID, draft.ID, draft.Revision, "root")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 24, 2, 0, 0, 0, time.UTC)
	point := model.AuthorizationPoint{
		ID: "point_confirmation", DeploymentID: tool.ProductID, IntegrationID: "integration_confirmation",
		ActionType: "write", ConfirmationRequired: true, DecisionTTLSeconds: 300, State: "active", Revision: 7,
	}
	binding := toolruntime.BoundAuthorization{
		IntegrationID: point.IntegrationID, ToolID: tool.ID, ToolRevision: tool.Revision,
		AuthorizationPoint: point, AuthorizationPointRevision: point.Revision,
	}
	principal := identity.Principal{
		Issuer: "https://id.vendor.example", Subject: "user-a", CustomerAccountID: "account-a", InstallationID: "installation-a",
		AccessEvaluationID: "evaluation-a", AccessEvaluatedAt: now, Grants: map[string]bool{},
	}
	return &Server{service: platform.New(memory)}, tool, binding, principal, now
}

func TestManagedToolConfirmationIsCanonicalAndOneTime(t *testing.T) {
	server, tool, binding, principal, now := managedToolConfirmationFixture(t)
	ctx := context.Background()
	arguments := map[string]any{"record_id": "record-1", "enabled": true}
	challenge, err := server.issueManagedToolConfirmation(ctx, tool.ProductID, tool, binding, principal, arguments, "stable-idempotency-001", now)
	if err != nil {
		t.Fatal(err)
	}
	reordered := map[string]any{"enabled": true, "record_id": "record-1"}
	if err := server.consumeManagedToolConfirmation(ctx, challenge.Nonce, tool.ProductID, tool, binding, principal, reordered, "stable-idempotency-001", now.Add(time.Second)); err != nil {
		t.Fatalf("exact canonical invocation was rejected: %v", err)
	}
	if err := server.consumeManagedToolConfirmation(ctx, challenge.Nonce, tool.ProductID, tool, binding, principal, arguments, "stable-idempotency-001", now.Add(2*time.Second)); err == nil {
		t.Fatal("consumed confirmation challenge was replayed")
	}
}

func TestManagedToolConfirmationRejectsExpiredOrMismatchedInvocation(t *testing.T) {
	for name, fixture := range map[string]struct {
		mutate    func(*model.Tool, *toolruntime.BoundAuthorization, *identity.Principal, map[string]any, *string)
		consumeAt func(managedToolConfirmationChallenge, time.Time) time.Time
	}{
		"expired": {
			consumeAt: func(challenge managedToolConfirmationChallenge, _ time.Time) time.Time { return challenge.ExpiresAt },
		},
		"actor": {
			mutate: func(_ *model.Tool, _ *toolruntime.BoundAuthorization, principal *identity.Principal, _ map[string]any, _ *string) {
				principal.Subject = "user-b"
			},
		},
		"access evaluation": {
			mutate: func(_ *model.Tool, _ *toolruntime.BoundAuthorization, principal *identity.Principal, _ map[string]any, _ *string) {
				principal.AccessEvaluationID = "evaluation-b"
			},
		},
		"arguments": {
			mutate: func(_ *model.Tool, _ *toolruntime.BoundAuthorization, _ *identity.Principal, arguments map[string]any, _ *string) {
				arguments["record_id"] = "record-2"
			},
		},
		"tool revision": {
			mutate: func(tool *model.Tool, _ *toolruntime.BoundAuthorization, _ *identity.Principal, _ map[string]any, _ *string) {
				tool.Revision++
			},
		},
		"authorization point revision": {
			mutate: func(_ *model.Tool, binding *toolruntime.BoundAuthorization, _ *identity.Principal, _ map[string]any, _ *string) {
				binding.AuthorizationPointRevision++
				binding.AuthorizationPoint.Revision++
			},
		},
		"idempotency key": {
			mutate: func(_ *model.Tool, _ *toolruntime.BoundAuthorization, _ *identity.Principal, _ map[string]any, key *string) {
				*key = "different-idempotency-002"
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			server, tool, binding, principal, now := managedToolConfirmationFixture(t)
			arguments := map[string]any{"record_id": "record-1", "enabled": true}
			idempotencyKey := "stable-idempotency-001"
			challenge, err := server.issueManagedToolConfirmation(context.Background(), tool.ProductID, tool, binding, principal, arguments, idempotencyKey, now)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.mutate != nil {
				fixture.mutate(&tool, &binding, &principal, arguments, &idempotencyKey)
			}
			at := now.Add(time.Second)
			if fixture.consumeAt != nil {
				at = fixture.consumeAt(challenge, now)
			}
			if err := server.consumeManagedToolConfirmation(context.Background(), challenge.Nonce, tool.ProductID, tool, binding, principal, arguments, idempotencyKey, at); err == nil {
				t.Fatal("mismatched or expired confirmation challenge was accepted")
			}
		})
	}
}
