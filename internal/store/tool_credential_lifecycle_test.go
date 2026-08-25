package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestMemoryUpdateToolAtomicallyReplacesAndRemovesCredentials(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	oldSecret, err := memory.CreateSecret(ctx, model.Secret{ID: "secret_old", OrganisationID: "org_acme", Name: "tool-connection-connection_credentials-secret_old", Purpose: "tool_upstream"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := memory.CreateTool(ctx, model.Tool{
		ID:                    "tool_credentials",
		OrganisationID:        "org_acme",
		ProductID:             "prod_acme",
		Namespace:             "credentials",
		Name:                  "rotate",
		Description:           "Rotate a credential.",
		InputSchema:           json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:          json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		APIConnectionID:       "connection_credentials",
		BaseURL:               "https://api.example.test/credentials",
		HTTPMethod:            "POST",
		UpstreamAuth:          json.RawMessage(`{"type":"bearer"}`),
		CredentialID:          oldSecret.ID,
		CredentialFingerprint: "old-fingerprint",
		AuthorizationPolicy:   json.RawMessage(`{"required_grants":[]}`),
		BackendKind:           "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := memory.CreateSecret(ctx, model.Secret{ID: "secret_new", OrganisationID: "org_acme", Name: "tool-connection-connection_credentials-secret_new", Purpose: "tool_upstream"})
	if err != nil {
		t.Fatal(err)
	}

	replacement := tool
	replacement.CredentialID, replacement.CredentialFingerprint = newSecret.ID, "new-fingerprint"
	replaced, err := memory.UpdateTool(ctx, replacement, tool.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.CredentialID != newSecret.ID {
		t.Fatalf("credential = %q, want %q", replaced.CredentialID, newSecret.ID)
	}
	if _, err := memory.Secret(ctx, "org_acme", oldSecret.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old credential lookup error = %v", err)
	}
	if _, err := memory.Secret(ctx, "org_acme", newSecret.ID); err != nil {
		t.Fatalf("new credential lookup error = %v", err)
	}

	withoutCredential := replaced
	withoutCredential.CredentialID, withoutCredential.CredentialFingerprint = "", ""
	withoutCredential.UpstreamAuth = json.RawMessage(`{"type":"none"}`)
	removed, err := memory.UpdateTool(ctx, withoutCredential, replaced.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if removed.CredentialID != "" {
		t.Fatalf("removed tool retained credential %q", removed.CredentialID)
	}
	if _, err := memory.Secret(ctx, "org_acme", newSecret.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed credential lookup error = %v", err)
	}
}

func TestMemoryUpdateToolRollsBackWhenPriorCredentialCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	tool, err := memory.CreateTool(ctx, model.Tool{
		ID:                    "tool_missing_credential",
		OrganisationID:        "org_acme",
		ProductID:             "prod_acme",
		Namespace:             "credentials",
		Name:                  "missing",
		Description:           "Reference a missing credential.",
		InputSchema:           json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:          json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		APIConnectionID:       "connection_missing_credential",
		BaseURL:               "https://api.example.test/credentials",
		HTTPMethod:            "POST",
		UpstreamAuth:          json.RawMessage(`{"type":"bearer"}`),
		CredentialID:          "secret_missing",
		CredentialFingerprint: "missing-fingerprint",
		AuthorizationPolicy:   json.RawMessage(`{"required_grants":[]}`),
		BackendKind:           "http",
	})
	if err != nil {
		t.Fatal(err)
	}

	update := tool
	update.CredentialID, update.CredentialFingerprint = "", ""
	if _, err := memory.UpdateTool(ctx, update, tool.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("update error = %v, want conflict", err)
	}
	stored, err := memory.Tool(ctx, tool.ProductID, tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CredentialID != tool.CredentialID || stored.Revision != tool.Revision {
		t.Fatalf("failed cleanup mutated tool: %#v", stored)
	}
}

func TestMemoryRetireToolNeverDeletesAnUnboundSecret(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	other, err := memory.CreateSecret(ctx, model.Secret{ID: "secret_other_purpose", OrganisationID: "org_acme", Name: "ai-provider-secret", Purpose: "ai_provider"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := memory.CreateTool(ctx, model.Tool{
		ID: "tool_wrong_secret", OrganisationID: "org_acme", ProductID: "prod_acme",
		Namespace: "credentials", Name: "wrong_secret", Description: "Reject an unrelated secret.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		APIConnectionID: "connection_wrong_secret", BaseURL: "https://api.example.test/credentials", HTTPMethod: "GET",
		UpstreamAuth: json.RawMessage(`{"type":"bearer"}`), CredentialID: other.ID, AuthorizationPolicy: json.RawMessage(`{"required_grants":[]}`), BackendKind: "http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.RetireTool(ctx, tool.ProductID, tool.ID, tool.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("retire error = %v, want conflict", err)
	}
	if _, err := memory.Secret(ctx, "org_acme", other.ID); err != nil {
		t.Fatalf("unbound secret was deleted: %v", err)
	}
}
