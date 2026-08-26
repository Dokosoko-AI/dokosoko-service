package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
)

type nativeExecutorStub struct {
	available bool
	calls     int
	result    any
	err       error
}

func (s *nativeExecutorStub) AvailableNative(model.Tool) bool { return s.available }
func (s *nativeExecutorStub) ExecuteNative(context.Context, model.Tool, map[string]any, toolruntime.Principal) (any, error) {
	s.calls++
	return s.result, s.err
}

func TestRuntimeDispatchesNativeToolsThroughCommonPolicy(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	created, err := memory.CreateTool(ctx, model.Tool{
		ID: "native_runtime_tool", OrganisationID: "org_acme", ProductID: "prod_acme", Scope: model.ToolScopeCommon,
		Namespace: "native_runtime", Name: "write", Description: "Test native dispatch.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		HTTPMethod:   "NATIVE", AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":false}`), TimeoutMS: 1000,
		BackendKind: "native", Effect: "write", IdempotencyMode: "required", IdentityRequirement: "none", StateScope: "none", MaxConcurrency: 1, MaxResultBytes: 4096,
		NativePluginID: "runtime_test", NativeToolID: "write", NativePluginVersion: "1.0.0", NativeSDKVersion: 1, NativeManifestHash: "sha256:manifest", NativeContractHash: "sha256:contract",
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := memory.PublishTool(ctx, created.ProductID, created.ID, created.Revision, "")
	if err != nil {
		t.Fatal(err)
	}
	executor := &nativeExecutorStub{available: true, result: map[string]any{"ok": true}}
	runtime := toolruntime.NewRuntime(memory, nil, nil)
	runtime.SetNativeExecutor(executor)

	if _, err := runtime.Execute(ctx, published.ProductID, "native_runtime.write", map[string]any{}, toolruntime.Principal{Subject: "actor"}); !errors.Is(err, toolruntime.ErrInvalidIdempotencyKey) || executor.calls != 0 {
		t.Fatalf("missing idempotency error=%v calls=%d", err, executor.calls)
	}
	result, err := runtime.Execute(ctx, published.ProductID, "native_runtime.write", map[string]any{}, toolruntime.Principal{Subject: "actor", IdempotencyKey: "stable-operation-key"})
	if err != nil || result.(map[string]any)["ok"] != true || executor.calls != 1 {
		t.Fatalf("result=%#v error=%v calls=%d", result, err, executor.calls)
	}
	executor.available = false
	available, err := runtime.Available(ctx, published.ProductID, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range available {
		if tool.ID == published.ID {
			t.Fatal("inactive native executor leaked into discovery")
		}
	}
}
