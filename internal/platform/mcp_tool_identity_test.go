package platform

import (
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestCanonicalMCPToolNameRequiresExactUnambiguousPublication(t *testing.T) {
	t.Parallel()
	tool := model.Tool{ID: "tool-charge", Revision: 7, State: "published", Scope: model.ToolScopeAPI, OwnerIntegrationID: "integration-v1", Namespace: "payments", Name: "create_charge", BackendKind: "mcp"}
	manifest := model.ProductManifest{Integrations: []model.IntegrationManifest{{
		ID: "integration-v1", FamilyKey: "payments", VersionKey: "v1",
		AuthorizationPoints: []model.IntegrationManifestAuthorizationPoint{{ID: "authorization-charge", Revision: 3}},
		Tools:               []model.IntegrationManifestTool{{ToolID: tool.ID, ToolRevision: tool.Revision, Namespace: tool.Namespace, Name: tool.Name, BackendKind: tool.BackendKind, AuthorizationPointID: "authorization-charge", AuthorizationPointRevision: 3}},
	}}}
	if got, ok := CanonicalMCPToolName(manifest, tool); !ok || got != "payments.custom.create_charge" {
		t.Fatalf("canonical name = %q, %t", got, ok)
	}
	identity, ok := canonicalMCPToolIdentityFor(manifest, tool)
	if !ok || !currentMCPToolAuthorization(identity,
		[]model.AuthorizationPoint{{ID: "authorization-charge", Revision: 3, State: "active", RequiredGrants: []string{"payments.charge"}}},
		[]model.GrantDefinition{{Key: "payments.charge", State: "active"}},
	) {
		t.Fatal("current authorization contract was not discoverable")
	}
	if currentMCPToolAuthorization(identity,
		[]model.AuthorizationPoint{{ID: "authorization-charge", Revision: 4, State: "active", RequiredGrants: []string{"payments.charge"}}},
		[]model.GrantDefinition{{Key: "payments.charge", State: "active"}},
	) {
		t.Fatal("changed authorization revision remained discoverable")
	}
	if currentMCPToolAuthorization(identity,
		[]model.AuthorizationPoint{{ID: "authorization-charge", Revision: 3, State: "active", RequiredGrants: []string{"payments.charge"}}},
		[]model.GrantDefinition{{Key: "payments.charge", State: "disabled"}},
	) {
		t.Fatal("inactive required grant remained discoverable")
	}

	secondVersion := manifest
	secondVersion.Integrations = append(append([]model.IntegrationManifest(nil), manifest.Integrations...), model.IntegrationManifest{ID: "integration-v2", FamilyKey: "payments", VersionKey: "v2"})
	if got, ok := CanonicalMCPToolName(secondVersion, tool); ok || got != "" {
		t.Fatalf("ambiguous family name = %q, %t", got, ok)
	}

	duplicate := manifest
	duplicate.Integrations = append([]model.IntegrationManifest(nil), manifest.Integrations...)
	duplicate.Integrations[0].Tools = append(append([]model.IntegrationManifestTool(nil), manifest.Integrations[0].Tools...), model.IntegrationManifestTool{ToolID: "tool-other", ToolRevision: 1, Name: tool.Name})
	if got, ok := CanonicalMCPToolName(duplicate, tool); ok || got != "" {
		t.Fatalf("colliding tool name = %q, %t", got, ok)
	}

	duplicateBinding := manifest
	duplicateBinding.Integrations = append(append([]model.IntegrationManifest(nil), manifest.Integrations...), model.IntegrationManifest{
		ID: "integration-orders", FamilyKey: "orders", VersionKey: "v1",
		AuthorizationPoints: []model.IntegrationManifestAuthorizationPoint{{ID: "authorization-orders", Revision: 1}},
		Tools:               []model.IntegrationManifestTool{{ToolID: tool.ID, ToolRevision: tool.Revision, Namespace: tool.Namespace, Name: tool.Name, BackendKind: tool.BackendKind, AuthorizationPointID: "authorization-orders", AuthorizationPointRevision: 1}},
	})
	if got, ok := CanonicalMCPToolName(duplicateBinding, tool); ok || got != "" {
		t.Fatalf("multiply-bound tool name = %q, %t", got, ok)
	}

	stale := tool
	stale.Revision++
	if got, ok := CanonicalMCPToolName(manifest, stale); ok || got != "" {
		t.Fatalf("stale tool revision name = %q, %t", got, ok)
	}

	drifted := tool
	drifted.UpstreamDrifted = true
	if got, ok := CanonicalMCPToolName(manifest, drifted); ok || got != "" {
		t.Fatalf("drifted tool name = %q, %t", got, ok)
	}

	draft := tool
	draft.State = "draft"
	if got, ok := CanonicalMCPToolName(manifest, draft); ok || got != "" {
		t.Fatalf("draft tool name = %q, %t", got, ok)
	}

	mismatched := manifest
	mismatched.Integrations = append([]model.IntegrationManifest(nil), manifest.Integrations...)
	mismatched.Integrations[0].Tools = append([]model.IntegrationManifestTool(nil), manifest.Integrations[0].Tools...)
	mismatched.Integrations[0].Tools[0].BackendKind = "http"
	if got, ok := CanonicalMCPToolName(mismatched, tool); ok || got != "" {
		t.Fatalf("mismatched manifest binding name = %q, %t", got, ok)
	}
}
