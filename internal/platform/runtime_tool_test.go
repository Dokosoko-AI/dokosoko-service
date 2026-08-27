package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func runtimeHTTPToolInput(ownerID, connectionID string) ToolInput {
	return ToolInput{
		ProductID: "prod_acme", Scope: model.ToolScopeAPI, OwnerIntegrationID: ownerID,
		RuntimeServiceConnectionID: connectionID, HTTPPath: "/v1/status",
		Namespace: "voice", Name: "status_read", Description: "Read the voice service status.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ready":{"type":"boolean"}}}`),
		HTTPMethod:   "GET", AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false,"risk":"low"}`), TimeoutMS: 5000,
	}
}

func selectRuntimeCredential(tool model.Tool, target model.ToolRuntimeTarget) model.Tool {
	tool.CredentialID = target.CredentialSecretID
	tool.RuntimeConnectionRevisionID = target.ConnectionRevisionID
	tool.RuntimeCredentialSetID = target.CredentialSetID
	tool.RuntimeCredentialVersionID = target.CredentialVersionID
	return tool
}

func TestPublishedRuntimeToolPinsConfigurationAndFollowsCredentialRotation(t *testing.T) {
	service, memory := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	setup, err := service.ConfigureRuntimeSetup(context.Background(), voiceID, RuntimeSetupInput{EnvironmentID: "env_prod", ConnectionName: "Default", BaseURL: "https://voice-one.example.test", AuthenticationType: "bearer", KeyManagementURL: testKeyManagementURL, AccessEvaluationURL: testAccessEvaluationURL, UsageURL: testUsageURL, Credential: "voice-key-one"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	connection, credentialSet := setup.Connections[0], setup.CredentialSets[0]
	draft, err := service.CreateTool(context.Background(), runtimeHTTPToolInput(voiceID, connection.ID), Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishTool(context.Background(), draft.ProductID, draft.ID, draft.Revision, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(published.RuntimeTargets) != 1 || published.RuntimeTargets[0].BaseURL != "https://voice-one.example.test" {
		t.Fatalf("published runtime targets = %#v", published.RuntimeTargets)
	}
	pinnedRevisionID := published.RuntimeTargets[0].ConnectionRevisionID
	toolRevision := published.Revision
	oldCredential, err := service.ResolveToolCredential(context.Background(), selectRuntimeCredential(published, published.RuntimeTargets[0]))
	if err != nil || string(oldCredential) != "voice-key-one" {
		t.Fatalf("old credential = %q err=%v", oldCredential, err)
	}

	if _, err := service.ConfigureRuntimeServiceConnection(context.Background(), voiceID, RuntimeServiceConnectionInput{Name: "Default", EnvironmentID: "env_prod", BaseURL: "https://voice-two.example.test", AuthenticationType: "bearer", CredentialSetID: credentialSet.ID}, Actor{ID: "root-test"}); err != nil {
		t.Fatal(err)
	}
	afterConfig, err := memory.Tool(context.Background(), draft.ProductID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConfig.RuntimeTargets[0].BaseURL != "https://voice-one.example.test" || afterConfig.RuntimeTargets[0].ConnectionRevisionID != pinnedRevisionID {
		t.Fatalf("published configuration moved after connection revision: %#v", afterConfig.RuntimeTargets[0])
	}

	oldVersionID := afterConfig.RuntimeTargets[0].CredentialVersionID
	if _, err := service.RotateRuntimeCredential(context.Background(), credentialSet.ID, "voice-key-two", nil, Actor{ID: "root-test"}); err != nil {
		t.Fatal(err)
	}
	afterRotation, err := memory.Tool(context.Background(), draft.ProductID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRotation.Revision != toolRevision || afterRotation.RuntimeTargets[0].ConnectionRevisionID != pinnedRevisionID || afterRotation.RuntimeTargets[0].CredentialVersionID == oldVersionID {
		t.Fatalf("rotation republished or moved configuration: tool=%#v target=%#v", afterRotation, afterRotation.RuntimeTargets[0])
	}
	newCredential, err := service.ResolveToolCredential(context.Background(), selectRuntimeCredential(afterRotation, afterRotation.RuntimeTargets[0]))
	if err != nil || string(newCredential) != "voice-key-two" {
		t.Fatalf("rotated credential = %q err=%v", newCredential, err)
	}
}

func TestRuntimeToolRejectsCrossAPIOwnershipAndLegacyCommonToolIsUnchanged(t *testing.T) {
	service, _ := newRuntimeServiceTest(t)
	voiceID := createRuntimeTestIntegration(t, service, "voice", "Voice API")
	faceID := createRuntimeTestIntegration(t, service, "face", "Face API")
	setup, err := service.ConfigureRuntimeSetup(context.Background(), voiceID, RuntimeSetupInput{EnvironmentID: "env_prod", BaseURL: "https://voice.example.test", AuthenticationType: "none"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTool(context.Background(), runtimeHTTPToolInput(faceID, setup.Connections[0].ID), Actor{ID: "root-test"}); err == nil {
		t.Fatal("cross-API runtime service connection was accepted")
	}
	if _, err := service.CreateEnvironment(context.Background(), "org_acme", "prod_acme", "Staging", "staging", false, Actor{ID: "root-test"}); err != nil {
		t.Fatal(err)
	}
	dedicated, err := service.CreateRuntimeCredentialSet(context.Background(), voiceID, RuntimeCredentialSetInput{EnvironmentID: "env_prod", Scope: "dedicated", AuthenticationType: "bearer", Credential: "production-only"}, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := service.Store().Environments(context.Background(), "prod_acme")
	var stagingID string
	for _, environment := range environments {
		if environment.Slug == "staging" {
			stagingID = environment.ID
		}
	}
	if _, err := service.ConfigureRuntimeServiceConnection(context.Background(), voiceID, RuntimeServiceConnectionInput{Name: "Staging invalid", EnvironmentID: stagingID, BaseURL: "https://voice-staging.example.test", AuthenticationType: "bearer", CredentialSetID: dedicated.ID}, Actor{ID: "root-test"}); err == nil {
		t.Fatal("cross-environment runtime credential was accepted")
	}

	legacy := runtimeHTTPToolInput("", "")
	legacy.Scope, legacy.Namespace, legacy.Name = model.ToolScopeCommon, "common", "legacy_status"
	legacy.Endpoint, legacy.UpstreamAuth, legacy.HTTPPath = "https://legacy.example.test/v1/status", json.RawMessage(`{"type":"none"}`), ""
	created, err := service.CreateTool(context.Background(), legacy, Actor{ID: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if created.APIConnectionID == "" || created.RuntimeServiceConnectionID != "" || len(created.RuntimeTargets) != 0 {
		t.Fatalf("legacy tool changed storage model: %#v", created)
	}
	published, err := service.PublishTool(context.Background(), created.ProductID, created.ID, created.Revision, Actor{ID: "root-test"})
	if err != nil || published.APIConnectionID == "" || len(published.RuntimeTargets) != 0 {
		t.Fatalf("legacy publish changed behavior: %#v err=%v", published, err)
	}
}
