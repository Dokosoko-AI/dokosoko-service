package platform

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestIntegrationSnapshotPinsCompleteNativeToolSourceContract(t *testing.T) {
	integration := model.Integration{ID: "integration_native", DeploymentID: "prod_native", FamilyKey: "native", VersionKey: "v1", DisplayName: "Native API", Visibility: model.VisibilityPrivate, Lifecycle: "active"}
	point := model.AuthorizationPoint{ID: "point_native", IntegrationID: integration.ID, DeploymentID: integration.DeploymentID, Key: "native.read", Name: "Read native data", ActionType: "read", State: "active", Revision: 2}
	tool := model.Tool{
		ID: "tool_native", ProductID: integration.DeploymentID, Scope: model.ToolScopeCommon, Namespace: "native", Name: "status", Description: "Return native status.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}}}`),
		State: "published", Revision: 3, HTTPMethod: "NATIVE", AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 1000,
		BackendKind: "native", Effect: "read", IdempotencyMode: "supported", IdentityRequirement: "optional", StateScope: "none", MaxConcurrency: 4, MaxResultBytes: 8192,
		NativePluginID: "example_native", NativeToolID: "status", NativePluginVersion: "1.2.3", NativeSDKVersion: 1, NativeManifestHash: "sha256:manifest", NativeContractHash: "sha256:contract-a",
	}
	binding := model.IntegrationToolBinding{IntegrationID: integration.ID, ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPointID: point.ID, AuthorizationPointRevision: point.Revision, Tool: &tool, AuthorizationPoint: &point}
	build := func(value model.Tool) integrationToolSnapshot {
		binding.Tool = &value
		raw, validations, err := buildIntegrationSnapshot(integration, integrationPublicationInputSet{AuthorizationPoints: []model.AuthorizationPoint{point}, ToolBindings: []model.IntegrationToolBinding{binding}}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		for _, validation := range validations {
			if validation.Level == "error" {
				t.Fatalf("snapshot validation: %#v", validation)
			}
		}
		var snapshot integrationSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil || len(snapshot.Tools) != 1 {
			t.Fatalf("snapshot=%s err=%v", raw, err)
		}
		return snapshot.Tools[0]
	}
	first := build(tool)
	if first.NativePluginID != tool.NativePluginID || first.NativePluginVersion != tool.NativePluginVersion || first.NativeManifestHash != tool.NativeManifestHash || first.NativeContractHash != tool.NativeContractHash || first.IdentityRequirement != "optional" {
		t.Fatalf("native source pins = %#v", first)
	}
	tool.NativeContractHash = "sha256:contract-b"
	second := build(tool)
	if first.ContentHash == second.ContentHash {
		t.Fatal("native contract hash change did not change the immutable Tool content hash")
	}
}
