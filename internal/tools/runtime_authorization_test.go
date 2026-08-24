package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type boundMCPExecutor struct{ calls int }

func (e *boundMCPExecutor) ExecuteMCP(_ context.Context, _ model.Tool, _ map[string]any, _ tools.Principal) (tools.MCPCallResult, error) {
	e.calls++
	return tools.MCPCallResult{Result: map[string]any{"resultType": "complete"}}, nil
}

func TestRuntimeEnforcesExactAuthorizationPointAndDecisionFreshness(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_bound", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "records", Name: "read", Description: "Read records.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object"}`), HTTPMethod: "MCP", AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := memory.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, "root")
	if err != nil {
		t.Fatal(err)
	}
	runtime := tools.NewRuntime(memory, nil, nil)
	executor := &boundMCPExecutor{}
	runtime.SetMCPExecutor(executor)
	integration, err := memory.CreateIntegration(ctx, model.Integration{ID: "integration_a", DeploymentID: "prod_acme", OrganisationID: "org_acme", FamilyKey: "records-api", VersionKey: "v1", DisplayName: "Records API", Lifecycle: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.SaveGrantDefinition(ctx, model.GrantDefinition{ID: "grant_records_read", DeploymentID: "prod_acme", OrganisationID: "org_acme", Key: "records.read", DisplayName: "Read records", State: "active"}, 0); err != nil {
		t.Fatal(err)
	}
	point, err := memory.SaveAuthorizationPoint(ctx, model.AuthorizationPoint{ID: "point_read", DeploymentID: "prod_acme", OrganisationID: "org_acme", IntegrationID: integration.ID, RequiredGrants: []string{"records.read"}, ConfirmationRequired: true, DecisionTTLSeconds: 60, State: "active"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	binding := tools.BoundAuthorization{IntegrationID: point.IntegrationID, ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPoint: point, AuthorizationPointRevision: point.Revision}
	fresh := tools.Principal{Subject: "user", Grants: map[string]bool{"records.read": true}, AccessEvaluationID: "evaluation_records_read", AccessEvaluatedAt: time.Now().UTC(), Confirmed: true}

	available, err := runtime.AvailableBound(ctx, "prod_acme", []tools.BoundAuthorization{binding}, fresh)
	if err != nil || len(available) != 1 || available[0].ID != tool.ID {
		t.Fatalf("fresh exact binding was not discoverable: values=%#v err=%v", available, err)
	}
	if _, err := runtime.ExecuteBound(ctx, "prod_acme", "records.read", map[string]any{}, tools.Principal{Subject: fresh.Subject, Grants: fresh.Grants, AccessEvaluationID: fresh.AccessEvaluationID, AccessEvaluatedAt: fresh.AccessEvaluatedAt}, binding); !errors.Is(err, tools.ErrConfirmation) {
		t.Fatalf("unconfirmed exact action error = %v", err)
	}
	if _, err := runtime.ExecuteBound(ctx, "prod_acme", "records.read", map[string]any{}, fresh, binding); err != nil || executor.calls != 1 {
		t.Fatalf("confirmed exact action err=%v calls=%d", err, executor.calls)
	}

	checks := []struct {
		name      string
		binding   tools.BoundAuthorization
		principal tools.Principal
	}{
		{name: "missing grant", binding: binding, principal: tools.Principal{Grants: map[string]bool{}, AccessEvaluationID: fresh.AccessEvaluationID, AccessEvaluatedAt: fresh.AccessEvaluatedAt}},
		{name: "stale evaluation", binding: binding, principal: tools.Principal{Grants: fresh.Grants, AccessEvaluationID: fresh.AccessEvaluationID, AccessEvaluatedAt: time.Now().UTC().Add(-61 * time.Second)}},
		{name: "unknown evaluation id", binding: binding, principal: tools.Principal{Grants: fresh.Grants, AccessEvaluatedAt: fresh.AccessEvaluatedAt}},
		{name: "unknown evaluation age", binding: binding, principal: tools.Principal{Grants: fresh.Grants, AccessEvaluationID: fresh.AccessEvaluationID}},
		{name: "point revision changed", binding: func() tools.BoundAuthorization { value := binding; value.AuthorizationPoint.Revision++; return value }(), principal: fresh},
		{name: "point deprecated", binding: func() tools.BoundAuthorization {
			value := binding
			value.AuthorizationPoint.State = "deprecated"
			return value
		}(), principal: fresh},
		{name: "ambiguous integration", binding: binding, principal: fresh},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			bindings := []tools.BoundAuthorization{check.binding}
			if check.name == "ambiguous integration" {
				other := check.binding
				other.IntegrationID, other.AuthorizationPoint.IntegrationID = "integration_b", "integration_b"
				bindings = append(bindings, other)
			}
			values, err := runtime.AvailableBound(ctx, "prod_acme", bindings, check.principal)
			if err != nil || len(values) != 0 {
				t.Fatalf("fail-closed discovery values=%#v err=%v", values, err)
			}
		})
	}
}
