package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestToolHTTPAPIAcceptsAndReturnsExplicitOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	owner, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice-api", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, platform.Actor{ID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "https://dokosoko.example")
	body, err := json.Marshal(map[string]any{
		"scope":                model.ToolScopeAPI,
		"owner_integration_id": owner.ID,
		"namespace":            "voice",
		"name":                 "status",
		"description":          "Read voice API status.",
		"input_schema":         json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		"output_schema":        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`),
		"endpoint":             "https://api.example.test/v1/status",
		"http_method":          http.MethodGet,
		"upstream_auth":        json.RawMessage(`{"type":"none"}`),
		"authorization_policy": json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`),
		"timeout_ms":           5_000,
		"request_mapping":      json.RawMessage(`{}`),
		"response_mapping":     json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	createdResponse := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools", "doko_admin_demo", string(body))
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created model.Tool
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Scope != model.ToolScopeAPI || created.OwnerIntegrationID != owner.ID {
		t.Fatalf("created ownership = scope %q owner %q", created.Scope, created.OwnerIntegrationID)
	}

	readResponse := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/tools/"+created.ID, "doko_admin_demo", "")
	if readResponse.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", readResponse.Code, readResponse.Body.String())
	}
	var read map[string]any
	if err := json.Unmarshal(readResponse.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if read["scope"] != model.ToolScopeAPI || read["owner_integration_id"] != owner.ID {
		t.Fatalf("read ownership = %#v", read)
	}
}
