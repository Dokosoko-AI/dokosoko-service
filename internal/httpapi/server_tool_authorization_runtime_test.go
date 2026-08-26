package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type stagedToolLookupStore struct {
	store.Store
	calls      int
	firstTools []model.Tool
	firstErr   error
	laterTools []model.Tool
}

func (s *stagedToolLookupStore) Tools(context.Context, string, bool) ([]model.Tool, error) {
	s.calls++
	if s.calls == 1 {
		return s.firstTools, s.firstErr
	}
	return s.laterTools, nil
}

func TestExecutableToolLookupFailsClosedBeforeRuntimeSecondLookup(t *testing.T) {
	appearing := model.Tool{ID: "tool_appeared", ProductID: "prod_acme", Namespace: "records", Name: "read"}
	for name, fixture := range map[string]*stagedToolLookupStore{
		"store error":          {Store: store.NewMemory(), firstErr: errors.New("temporary lookup failure"), laterTools: []model.Tool{appearing}},
		"published after read": {Store: store.NewMemory(), firstTools: []model.Tool{}, laterTools: []model.Tool{appearing}},
	} {
		t.Run(name, func(t *testing.T) {
			server := &Server{service: platform.New(fixture)}
			selected, err := server.executableTool(context.Background(), "prod_acme", "records.read", model.CatalogScope{})
			if err == nil || selected.ID != "" || fixture.calls != 1 {
				t.Fatalf("selected=%#v err=%v lookup calls=%d", selected, err, fixture.calls)
			}
		})
	}
}

func TestIntegrationToolAuthorizationIsExactAndIntegrationIsolated(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	service := platform.New(memory)
	server := &Server{service: service}
	actor := platform.Actor{ID: "root_runtime_scope"}

	integrationA, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "records-a", VersionKey: "v1", DisplayName: "Records A", Description: "Customer A records.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	integrationB, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "records-b", VersionKey: "v1", DisplayName: "Records B", Description: "Customer B records.", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	pointA, err := service.SaveAuthorizationPoint(ctx, integrationA.ID, "", platform.AuthorizationPointInput{Key: "records.a.read", Name: "Read A", Description: "Read customer A records.", ActionType: "read", DecisionTTLSeconds: 60, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	pointB, err := service.SaveAuthorizationPoint(ctx, integrationB.ID, "", platform.AuthorizationPointInput{Key: "records.b.read", Name: "Read B", Description: "Read customer B records.", ActionType: "read", DecisionTTLSeconds: 60, State: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := memory.CreateTool(ctx, model.Tool{ID: "tool_records", OrganisationID: "org_acme", ProductID: "prod_acme", Namespace: "records", Name: "read", Description: "Read records.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), BaseURL: "https://api.vendor.example/records", HTTPMethod: "GET", UpstreamAuth: json.RawMessage(`{"type":"none"}`), RequestMapping: json.RawMessage(`{}`), ResponseMapping: json.RawMessage(`{}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 5000, BackendKind: "http"})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := service.PublishTool(ctx, "prod_acme", draft.ID, draft.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	manifest := model.ProductManifest{DeploymentID: "prod_acme", Integrations: []model.IntegrationManifest{{ID: integrationA.ID, AuthorizationPoints: []model.IntegrationManifestAuthorizationPoint{{ID: pointA.ID, Revision: pointA.Revision}}, Tools: []model.IntegrationManifestTool{{ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPointID: pointA.ID, AuthorizationPointRevision: pointA.Revision}}}, {ID: integrationB.ID, AuthorizationPoints: []model.IntegrationManifestAuthorizationPoint{{ID: pointB.ID, Revision: pointB.Revision}}, Tools: []model.IntegrationManifestTool{}}}}
	binding, managed, err := server.integrationToolAuthorization(ctx, manifest, tool)
	if err != nil || !managed || binding.IntegrationID != integrationA.ID || binding.AuthorizationPoint.ID != pointA.ID {
		t.Fatalf("unique Integration A binding=%#v managed=%t err=%v", binding, managed, err)
	}
	legacy := model.ProductManifest{DeploymentID: "prod_acme", Integrations: []model.IntegrationManifest{{ID: "legacy", Tools: nil}}}
	if _, managed, err := server.integrationToolAuthorization(ctx, legacy, tool); managed || err != nil {
		t.Fatalf("legacy unmanaged tool policy was not preserved: managed=%t err=%v", managed, err)
	}
	denyAll := model.ProductManifest{DeploymentID: "prod_acme", Integrations: []model.IntegrationManifest{{ID: integrationA.ID, Tools: []model.IntegrationManifestTool{}}}}
	if _, managed, err := server.integrationToolAuthorization(ctx, denyAll, tool); !managed || err == nil {
		t.Fatalf("managed empty binding set did not fail closed: managed=%t err=%v", managed, err)
	}
	filteredOut := model.ProductManifest{DeploymentID: "prod_acme", ManagedIntegrationTools: true, Integrations: []model.IntegrationManifest{}}
	if _, managed, err := server.integrationToolAuthorization(ctx, filteredOut, tool); !managed || err == nil {
		t.Fatalf("a selected profile with no applicable managed Integration fell back to legacy execution: managed=%t err=%v", managed, err)
	}

	pointA, err = service.SaveAuthorizationPoint(ctx, integrationA.ID, pointA.ID, platform.AuthorizationPointInput{Key: pointA.Key, Name: pointA.Name, Description: pointA.Description, ActionType: pointA.ActionType, RequiredGrants: pointA.RequiredGrants, ConfirmationRequired: pointA.ConfirmationRequired, DecisionTTLSeconds: pointA.DecisionTTLSeconds, State: pointA.State, Revision: pointA.Revision}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.integrationToolAuthorization(ctx, manifest, tool); err == nil {
		t.Fatal("an edited authorization point still authorized the old published binding")
	}
	manifest.Integrations[0].AuthorizationPoints[0].Revision = pointA.Revision
	manifest.Integrations[0].Tools[0].AuthorizationPointRevision = pointA.Revision
	if binding, _, err = server.integrationToolAuthorization(ctx, manifest, tool); err != nil || binding.AuthorizationPointRevision != pointA.Revision {
		t.Fatalf("republished exact point revision was not restored: binding=%#v err=%v", binding, err)
	}

	manifest.Integrations[1].Tools = []model.IntegrationManifestTool{{ToolID: tool.ID, ToolRevision: tool.Revision, AuthorizationPointID: pointB.ID, AuthorizationPointRevision: pointB.Revision}}
	if _, managed, err := server.integrationToolAuthorization(ctx, manifest, tool); !managed || err == nil {
		t.Fatalf("product-wide A/B union was accepted: managed=%t err=%v", managed, err)
	}
}
