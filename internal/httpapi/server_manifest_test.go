package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

func TestCustomToolDefinitionPublishesConfirmationAndRevisionMetadata(t *testing.T) {
	t.Parallel()
	tool := model.Tool{ID: "tool_1", Revision: 7, Namespace: "billing", Name: "delete_invoice", Description: "Delete one invoice.", HTTPMethod: "DELETE", InputSchema: json.RawMessage(`{"type":"object"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":["billing.delete"],"confirmation_required":true,"risk":"critical","idempotency_required":true}`)}
	manifest := model.ProductManifest{Integrations: []model.IntegrationManifest{{ID: "integration_1", Tools: []model.IntegrationManifestTool{{ToolID: tool.ID, ToolRevision: tool.Revision}}}}}
	definition := customToolDefinition(manifest, tool)
	if definition["name"] != "billing.delete_invoice" {
		t.Fatalf("name = %#v", definition["name"])
	}
	metadata, _ := definition["_meta"].(map[string]any)
	if metadata["com.dokosoko/toolRevision"] != int64(7) || metadata["com.dokosoko/confirmationRequired"] != true || metadata["com.dokosoko/risk"] != "critical" || metadata["com.dokosoko/idempotencyKeyRequired"] != true || metadata["com.dokosoko/idempotencyKeyMetaField"] != "idempotency_key" {
		t.Fatalf("metadata = %#v", metadata)
	}
	annotations, _ := definition["annotations"].(map[string]any)
	if annotations["destructiveHint"] != true || annotations["idempotentHint"] != true || annotations["readOnlyHint"] != false {
		t.Fatalf("annotations = %#v", annotations)
	}
}

func TestCustomToolDefinitionPublishesExactAuthorizationActionMetadata(t *testing.T) {
	t.Parallel()
	tool := model.Tool{ID: "tool_1", Revision: 7, Namespace: "billing", Name: "update_invoice", Description: "Update one invoice.", HTTPMethod: "POST", InputSchema: json.RawMessage(`{"type":"object"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":["billing.base"],"confirmation_required":false}`)}
	point := model.AuthorizationPoint{ID: "point_update", IntegrationID: "integration_1", ActionType: "write", RequiredGrants: []string{"billing.write"}, ConfirmationRequired: true, DecisionTTLSeconds: 45, State: "active", Revision: 3}
	binding := toolruntime.BoundAuthorization{IntegrationID: point.IntegrationID, ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPoint: point, AuthorizationPointRevision: point.Revision}
	definition := customToolDefinitionForAuthorization(model.ProductManifest{}, tool, binding, true)
	metadata, _ := definition["_meta"].(map[string]any)
	if metadata["com.dokosoko/authorizationPointId"] != point.ID || metadata["com.dokosoko/authorizationPointRevision"] != point.Revision || metadata["com.dokosoko/authorizationDecisionTtlSeconds"] != 45 || metadata["com.dokosoko/confirmationRequired"] != true {
		t.Fatalf("authorization metadata = %#v", metadata)
	}
	required, _ := metadata["com.dokosoko/requiredGrants"].([]string)
	if len(required) != 2 || required[0] != "billing.base" || required[1] != "billing.write" {
		t.Fatalf("combined required grants = %#v", required)
	}
	annotations, _ := definition["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != false || annotations["destructiveHint"] != false {
		t.Fatalf("authorization action annotations = %#v", annotations)
	}
}

func TestCustomToolDefinitionUsesConservativeMethodAndAuthorizationAnnotations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		method          string
		risk            string
		action          string
		wantReadOnly    bool
		wantDestructive bool
	}{
		{name: "unmanaged GET is read only", method: "GET", risk: "low", wantReadOnly: true},
		{name: "read GET is read only", method: "GET", risk: "low", action: "read", wantReadOnly: true},
		{name: "write action strengthens GET", method: "GET", risk: "low", action: "write"},
		{name: "destructive action strengthens GET", method: "GET", risk: "low", action: "destructive", wantDestructive: true},
		{name: "stale read action cannot mask POST", method: "POST", risk: "medium", action: "read"},
		{name: "stale read action cannot mask PUT", method: "PUT", risk: "medium", action: "read"},
		{name: "stale read action cannot mask PATCH", method: "PATCH", risk: "medium", action: "read"},
		{name: "stale read action cannot mask DELETE", method: "DELETE", risk: "critical", action: "read", wantDestructive: true},
		{name: "critical policy remains destructive", method: "POST", risk: "critical", action: "write", wantDestructive: true},
		{name: "destructive action remains destructive", method: "POST", risk: "medium", action: "destructive", wantDestructive: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tool := model.Tool{ID: "tool_annotations", Revision: 1, Namespace: "safety", Name: "operation", Description: "Exercise annotation derivation.", HTTPMethod: test.method, InputSchema: json.RawMessage(`{"type":"object"}`), AuthorizationPolicy: json.RawMessage(`{"risk":"` + test.risk + `"}`)}
			binding := toolruntime.BoundAuthorization{}
			managed := test.action != ""
			if managed {
				binding = toolruntime.BoundAuthorization{IntegrationID: "integration_1", AuthorizationPoint: model.AuthorizationPoint{ID: "point_1", ActionType: test.action}, AuthorizationPointRevision: 1}
			}
			definition := customToolDefinitionForAuthorization(model.ProductManifest{}, tool, binding, managed)
			annotations, _ := definition["annotations"].(map[string]any)
			if annotations["readOnlyHint"] != test.wantReadOnly || annotations["destructiveHint"] != test.wantDestructive {
				t.Fatalf("annotations = %#v, want readOnly=%t destructive=%t", annotations, test.wantReadOnly, test.wantDestructive)
			}
		})
	}
}
