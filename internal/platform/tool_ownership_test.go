package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func ownershipToolInput(namespace, name, scope, ownerIntegrationID string) platform.ToolInput {
	return platform.ToolInput{
		ProductID:          "prod_acme",
		Scope:              scope,
		OwnerIntegrationID: ownerIntegrationID,
		Namespace:          namespace,
		Name:               name,
		Description:        "Read tool ownership status.",
		InputSchema:        json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema:       json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}},"required":["ready"]}`),
		Endpoint:           "https://api.example.test/v1/ownership",
		HTTPMethod:         "GET",
		UpstreamAuth:       json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(
			`{"required_grants":[],"confirmation_required":false,"risk":"low","idempotency_required":false}`,
		),
		TimeoutMS: 5_000,
	}
}

func TestCreateToolPersistsExplicitOwnershipAndClonePreservesIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_ownership", RequestID: "req_ownership"}
	owner, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice-api", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}

	common, err := service.CreateTool(ctx, ownershipToolInput("common", "status", model.ToolScopeCommon, ""), actor)
	if err != nil {
		t.Fatal(err)
	}
	if common.Scope != model.ToolScopeCommon || common.OwnerIntegrationID != "" {
		t.Fatalf("common ownership = scope %q owner %q", common.Scope, common.OwnerIntegrationID)
	}

	defaulted, err := service.CreateTool(ctx, ownershipToolInput("defaulted", "status", "", ""), actor)
	if err != nil {
		t.Fatal(err)
	}
	if defaulted.Scope != model.ToolScopeCommon || defaulted.OwnerIntegrationID != "" {
		t.Fatalf("default ownership = scope %q owner %q", defaulted.Scope, defaulted.OwnerIntegrationID)
	}

	apiTool, err := service.CreateTool(ctx, ownershipToolInput("voice", "status", model.ToolScopeAPI, owner.ID), actor)
	if err != nil {
		t.Fatal(err)
	}
	if apiTool.Scope != model.ToolScopeAPI || apiTool.OwnerIntegrationID != owner.ID {
		t.Fatalf("api ownership = scope %q owner %q", apiTool.Scope, apiTool.OwnerIntegrationID)
	}
	persisted, err := memory.Tool(ctx, apiTool.ProductID, apiTool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Scope != model.ToolScopeAPI || persisted.OwnerIntegrationID != owner.ID {
		t.Fatalf("persisted api ownership = scope %q owner %q", persisted.Scope, persisted.OwnerIntegrationID)
	}

	cloned, err := service.CloneTool(ctx, apiTool.ProductID, apiTool.ID, platform.ToolCloneInput{Namespace: "voice", Name: "status_clone", Revision: apiTool.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.Scope != model.ToolScopeAPI || cloned.OwnerIntegrationID != owner.ID {
		t.Fatalf("clone ownership = scope %q owner %q", cloned.Scope, cloned.OwnerIntegrationID)
	}

	tests := []struct {
		name  string
		scope string
		owner string
		want  string
	}{
		{name: "common_owner", scope: model.ToolScopeCommon, owner: owner.ID, want: "common tools cannot have an owner"},
		{name: "api_without_owner", scope: model.ToolScopeAPI, want: "require owner_integration_id"},
		{name: "api_unknown_owner", scope: model.ToolScopeAPI, owner: "00000000-0000-4000-8000-000000000099", want: "same deployment"},
		{name: "unknown_scope", scope: "deployment", want: "common or api"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := ownershipToolInput("invalid", test.name, test.scope, test.owner)
			if _, err := service.CreateTool(ctx, input, actor); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CreateTool error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestIntegrationToolBindingsEnforceOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "root_binding_ownership", RequestID: "req_binding_ownership"}

	voice, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "voice-api", VersionKey: "v1", DisplayName: "Voice API", Description: "Voice operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	face, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "face-api", VersionKey: "v1", DisplayName: "Face API", Description: "Face operations.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	createPoint := func(integration model.Integration, key string) model.AuthorizationPoint {
		t.Helper()
		point, pointErr := service.SaveAuthorizationPoint(ctx, integration.ID, "", platform.AuthorizationPointInput{Key: key, Name: "Read status", Description: "Authorize status reads.", ActionType: "read", DecisionTTLSeconds: 120, State: "active"}, actor)
		if pointErr != nil {
			t.Fatal(pointErr)
		}
		return point
	}
	voicePoint := createPoint(voice, "voice.status.read")
	facePoint := createPoint(face, "face.status.read")
	createPublished := func(input platform.ToolInput) model.Tool {
		t.Helper()
		draft, createErr := service.CreateTool(ctx, input, actor)
		if createErr != nil {
			t.Fatal(createErr)
		}
		published, publishErr := service.PublishTool(ctx, draft.ProductID, draft.ID, draft.Revision, actor)
		if publishErr != nil {
			t.Fatal(publishErr)
		}
		return published
	}
	common := createPublished(ownershipToolInput("shared", "status", model.ToolScopeCommon, ""))
	voiceOwned := createPublished(ownershipToolInput("voice", "owned_status", model.ToolScopeAPI, voice.ID))

	selectTool := func(tool model.Tool, point model.AuthorizationPoint) []platform.ToolRevisionSelection {
		return []platform.ToolRevisionSelection{{ToolID: tool.ID, Revision: tool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision}}
	}
	if _, err := service.SetIntegrationToolBindings(ctx, voice.ID, selectTool(common, voicePoint), actor); err != nil {
		t.Fatalf("bind common tool to voice: %v", err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, face.ID, selectTool(common, facePoint), actor); err != nil {
		t.Fatalf("bind common tool to face: %v", err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, voice.ID, selectTool(voiceOwned, voicePoint), actor); err != nil {
		t.Fatalf("bind API tool to owner: %v", err)
	}
	if _, err := service.SetIntegrationToolBindings(ctx, face.ID, selectTool(voiceOwned, facePoint), actor); err == nil || !strings.Contains(err.Error(), "owned by another integration") {
		t.Fatalf("bind API tool to non-owner error = %v", err)
	}
	faceBindings, err := service.IntegrationToolBindings(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(faceBindings) != 1 || faceBindings[0].ToolID != common.ID {
		t.Fatalf("failed replacement changed face bindings: %#v", faceBindings)
	}

	direct := []model.IntegrationToolBinding{{IntegrationID: face.ID, ToolID: voiceOwned.ID, ToolRevision: voiceOwned.Revision, AuthorizationPointID: facePoint.ID, AuthorizationPointRevision: facePoint.Revision, CreatedBy: actor.ID}}
	if _, err := memory.SaveIntegrationToolBindings(ctx, face.ID, direct); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("store accepted API-owned tool for non-owner: %v", err)
	}
}
