package nativeplugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type testPlugin struct {
	manifest nativeplugin.Manifest
	open     func(nativeplugin.Host) (nativeplugin.Instance, error)
}

func (p testPlugin) Describe() nativeplugin.Manifest { return p.manifest }
func (p testPlugin) Open(_ context.Context, host nativeplugin.Host) (nativeplugin.Instance, error) {
	if p.open != nil {
		return p.open(host)
	}
	return testInstance{}, nil
}

type testInstance struct {
	invoke func(nativeplugin.Invocation) (nativeplugin.Result, error)
	close  func() error
}

func (i testInstance) Invoke(_ context.Context, call nativeplugin.Invocation) (nativeplugin.Result, error) {
	if i.invoke != nil {
		return i.invoke(call)
	}
	return nativeplugin.Result{Structured: map[string]any{"ok": true}}, nil
}
func (i testInstance) Close(context.Context) error {
	if i.close != nil {
		return i.close()
	}
	return nil
}

func testManifest(id, version string, identity nativeplugin.IdentityRequirement, scope nativeplugin.StateScope) nativeplugin.Manifest {
	return nativeplugin.Manifest{
		ID: id, Version: version, SDKVersion: nativeplugin.SDKVersion,
		Description: "A test-only native plugin manifest.", StateVersion: 1,
		Tools: []nativeplugin.ToolSpec{{
			ID: "check", Namespace: "native_test", Name: id, Description: "Run the test native tool.",
			InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
			Effect:       nativeplugin.EffectRead, Identity: identity, StateScope: scope,
			Idempotency: nativeplugin.IdempotencySupported, Timeout: time.Second, MaxConcurrency: 2, MaxResultBytes: 4096,
		}},
	}
}

func newTestManager(t *testing.T, memory *store.Memory, plugins ...nativeplugin.Plugin) *Manager {
	t.Helper()
	manager, err := New(plugins, Options{State: memory, IdentityKey: []byte("01234567890123456789012345678901"), Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	return manager
}

func modelTool(t *testing.T, manager *Manager, pluginID, toolID string) model.Tool {
	t.Helper()
	manager.mu.RLock()
	entry := manager.entries[pluginID]
	manager.mu.RUnlock()
	spec, ok := entry.manifest.Tool(toolID)
	if !ok {
		t.Fatal("tool missing")
	}
	value, err := catalogTool(model.Deployment{ID: "prod_acme", OrganisationID: "org_acme"}, entry.manifest, entry.manifestHash, spec)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestManagerProjectsOpaqueIdentityAndIsolatesCustomerState(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("identity_test", "1.0.0", nativeplugin.IdentityCustomerRequired, nativeplugin.StateCustomer)
	manifest.Tools[0].OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"},"count":{"type":"integer"},"customer":{"type":"string"}},"required":["ok","count","customer"]}`)
	var mu sync.Mutex
	plugin := testPlugin{manifest: manifest, open: func(nativeplugin.Host) (nativeplugin.Instance, error) {
		return testInstance{invoke: func(call nativeplugin.Invocation) (nativeplugin.Result, error) {
			mu.Lock()
			defer mu.Unlock()
			count := int64(0)
			prior, err := call.State.Get(context.Background(), "count")
			expected := int64(0)
			if err == nil {
				expected = prior.Revision
				_ = json.Unmarshal(prior.Value, &count)
			} else if !errors.Is(err, nativeplugin.ErrStateNotFound) {
				return nativeplugin.Result{}, err
			}
			count++
			encoded, _ := json.Marshal(count)
			if _, err := call.State.Put(context.Background(), "count", encoded, nativeplugin.PutOptions{ExpectedRevision: expected}); err != nil {
				return nativeplugin.Result{}, err
			}
			return nativeplugin.Result{Structured: map[string]any{"ok": true, "count": count, "customer": call.Identity.Customer.ID}}, nil
		}}, nil
	}}
	manager := newTestManager(t, memory, plugin)
	tool := modelTool(t, manager, manifest.ID, "check")

	call := func(customer string) map[string]any {
		result, err := manager.ExecuteNative(context.Background(), tool, map[string]any{}, toolruntime.Principal{Subject: "raw-actor", Issuer: "https://issuer.example", CustomerAccountID: customer})
		if err != nil {
			t.Fatal(err)
		}
		return result.(map[string]any)
	}
	first, second, isolated := call("raw-customer-a"), call("raw-customer-a"), call("raw-customer-b")
	if first["count"] != int64(1) || second["count"] != int64(2) || isolated["count"] != int64(1) {
		t.Fatalf("counts = %#v %#v %#v", first, second, isolated)
	}
	customerID, _ := first["customer"].(string)
	if !strings.HasPrefix(customerID, "cus_") || strings.Contains(customerID, "raw-customer") || customerID == isolated["customer"] {
		t.Fatalf("opaque customer ids = %q, %q", customerID, isolated["customer"])
	}
}

func TestManagerConfigurationStatusNeverContainsValues(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("config_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	manifest.Config = []nativeplugin.ConfigSpec{{Key: "TOKEN", Type: nativeplugin.ConfigSecret, Required: true, Description: "Test secret."}}
	seen := ""
	plugin := testPlugin{manifest: manifest, open: func(host nativeplugin.Host) (nativeplugin.Instance, error) {
		secret, ok := host.Config().Secret("TOKEN")
		if !ok {
			return nil, errors.New("secret missing")
		}
		seen = secret.Reveal()
		return testInstance{}, nil
	}}
	manager, err := New([]nativeplugin.Plugin{plugin}, Options{State: memory, IdentityKey: []byte("01234567890123456789012345678901"), Environment: func(key string) (string, bool) {
		return "super-sensitive-value", key == "DOKOSOKO_PLUGIN_CONFIG_TEST_TOKEN"
	}, Logger: log.New(io.Discard, "", 0)})
	if err != nil || manager.Start(context.Background()) != nil {
		t.Fatalf("start: %v", err)
	}
	defer manager.Close(context.Background())
	encoded, _ := json.Marshal(manager.Statuses())
	if seen != "super-sensitive-value" || strings.Contains(string(encoded), seen) || !manager.Statuses()[0].Configuration[0].Configured {
		t.Fatalf("secret handling failed: seen=%q status=%s", seen, encoded)
	}
}

func TestManagerRedactsConfiguredSecretsFromLogsAndInvocationErrors(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("redaction_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	manifest.Config = []nativeplugin.ConfigSpec{{Key: "TOKEN", Type: nativeplugin.ConfigSecret, Required: true, Description: "Test secret."}}
	secret := "secret-value-that-must-not-leak"
	var logs bytes.Buffer
	plugin := testPlugin{manifest: manifest, open: func(host nativeplugin.Host) (nativeplugin.Instance, error) {
		host.Logger().Info("configured "+secret, nativeplugin.F("detail", "credential="+secret))
		return testInstance{invoke: func(nativeplugin.Invocation) (nativeplugin.Result, error) {
			return nativeplugin.Result{}, errors.New("upstream rejected " + secret)
		}}, nil
	}}
	manager, err := New([]nativeplugin.Plugin{plugin}, Options{
		State: memory, IdentityKey: []byte("01234567890123456789012345678901"), Logger: log.New(&logs, "", 0),
		Environment: func(key string) (string, bool) { return secret, key == "DOKOSOKO_PLUGIN_REDACTION_TEST_TOKEN" },
	})
	if err != nil || manager.Start(context.Background()) != nil {
		t.Fatalf("start: %v", err)
	}
	defer manager.Close(context.Background())
	if strings.Contains(logs.String(), secret) || !strings.Contains(logs.String(), "[REDACTED]") {
		t.Fatalf("log redaction failed: %q", logs.String())
	}
	_, err = manager.ExecuteNative(context.Background(), modelTool(t, manager, manifest.ID, "check"), nil, toolruntime.Principal{})
	if err == nil || strings.Contains(err.Error(), secret) || err.Error() != "Native tool failed safely" {
		t.Fatalf("safe error = %q", err)
	}
}

func TestManagerPreservesOnlyValidatedSafeCallErrors(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("safe_error_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	plugin := testPlugin{manifest: manifest, open: func(nativeplugin.Host) (nativeplugin.Instance, error) {
		return testInstance{invoke: func(nativeplugin.Invocation) (nativeplugin.Result, error) {
			return nativeplugin.Result{}, nativeplugin.Fail(nativeplugin.ErrorInvalidArgument, "Use a positive count", errors.New("private diagnostic"))
		}}, nil
	}}
	manager := newTestManager(t, memory, plugin)
	_, err := manager.ExecuteNative(context.Background(), modelTool(t, manager, manifest.ID, "check"), nil, toolruntime.Principal{})
	var callError *nativeplugin.CallError
	if !errors.As(err, &callError) || callError.Code != nativeplugin.ErrorInvalidArgument || err.Error() != "Use a positive count" || callError.Cause != nil {
		t.Fatalf("call error = %#v", err)
	}
}

func TestManagerDrainsInvocationsBeforeClose(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("lifecycle_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	invoked, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	plugin := testPlugin{manifest: manifest, open: func(nativeplugin.Host) (nativeplugin.Instance, error) {
		return testInstance{
			invoke: func(nativeplugin.Invocation) (nativeplugin.Result, error) {
				close(invoked)
				<-release
				return nativeplugin.Result{Structured: map[string]any{"ok": true}}, nil
			},
			close: func() error { close(closed); return nil },
		}, nil
	}}
	manager := newTestManager(t, memory, plugin)
	tool := modelTool(t, manager, manifest.ID, "check")
	callDone := make(chan error, 1)
	go func() {
		_, err := manager.ExecuteNative(context.Background(), tool, nil, toolruntime.Principal{})
		callDone <- err
	}()
	<-invoked
	disableDone := make(chan error, 1)
	go func() {
		_, err := manager.SetEnabled(context.Background(), manifest.ID, false)
		disableDone <- err
	}()
	select {
	case <-closed:
		t.Fatal("Close ran before Invoke drained")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-callDone; err != nil {
		t.Fatal(err)
	}
	if err := <-disableDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("Close did not run after Invoke drained")
	}
}

func TestManagerDisablesEvenWhenPluginCloseFails(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("close_failure_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	plugin := testPlugin{manifest: manifest, open: func(nativeplugin.Host) (nativeplugin.Instance, error) {
		return testInstance{close: func() error { return errors.New("private close diagnostic") }}, nil
	}}
	manager := newTestManager(t, memory, plugin)
	status, err := manager.SetEnabled(context.Background(), manifest.ID, false)
	if err != nil || status.State != StatusDisabled {
		t.Fatalf("disable status=%#v err=%v", status, err)
	}
	if manager.AvailableNative(modelTool(t, manager, manifest.ID, "check")) {
		t.Fatal("plugin remained executable after Close failed")
	}
	enabled, err := nativepluginstate.Enabled(context.Background(), memory, manifest.ID)
	if err != nil || enabled {
		t.Fatalf("persisted enabled=%v err=%v", enabled, err)
	}
}

func TestCatalogStagesSourceChangesForReview(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	firstManifest := testManifest("catalog_test", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	first := newTestManager(t, memory, testPlugin{manifest: firstManifest})
	deployment, err := memory.Deployment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SyncCatalog(ctx, memory, deployment); err != nil {
		t.Fatal(err)
	}
	tools, err := memory.Tools(ctx, deployment.ID, false)
	if err != nil || len(tools) == 0 {
		t.Fatalf("tools=%#v err=%v", tools, err)
	}
	var nativeTool model.Tool
	for _, candidate := range tools {
		if candidate.NativePluginID == firstManifest.ID {
			nativeTool = candidate
		}
	}
	service := platform.New(memory)
	service.SetNativeToolCatalog(first)
	published, err := service.PublishTool(ctx, deployment.ID, nativeTool.ID, nativeTool.Revision, platform.Actor{ID: "root"})
	if err != nil || published.State != "published" {
		t.Fatalf("publish=%#v err=%v", published, err)
	}

	secondManifest := testManifest("catalog_test", "1.1.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	secondManifest.Tools[0].Description = "A reviewed source change that must return to draft."
	second := newTestManager(t, memory, testPlugin{manifest: secondManifest})
	if err := second.SyncCatalog(ctx, memory, deployment); err != nil {
		t.Fatal(err)
	}
	staged, err := memory.Tool(ctx, deployment.ID, nativeTool.ID)
	if err != nil || staged.State != "draft" || staged.Revision != published.Revision+1 || staged.NativePluginVersion != "1.1.0" {
		t.Fatalf("staged=%#v err=%v", staged, err)
	}
	if first.AvailableNative(staged) || !second.AvailableNative(staged) {
		t.Fatalf("source pin availability did not move to the reviewed draft")
	}
}

func TestMissingConfigurationFailsClosedWithoutLeakingDiagnostic(t *testing.T) {
	memory := store.NewMemory()
	manifest := testManifest("missing_config", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	manifest.Config = []nativeplugin.ConfigSpec{{Key: "TOKEN", Type: nativeplugin.ConfigSecret, Required: true, Description: "Required token."}}
	manager := newTestManager(t, memory, testPlugin{manifest: manifest})
	status := manager.Statuses()[0]
	if status.State != StatusMisconfigured || status.LastErrorCode != "configuration_invalid" || strings.Contains(status.LastError, "TOKEN") {
		t.Fatalf("status = %#v", status)
	}
	tool := modelTool(t, manager, manifest.ID, "check")
	if manager.AvailableNative(tool) {
		t.Fatal("misconfigured plugin was available")
	}
}

func TestURLConfigurationRejectsCredentialChannelsAndIPLiteral(t *testing.T) {
	for _, raw := range []string{
		"http://api.example.com",
		"https://user:pass@api.example.com",
		"https://api.example.com?token=value",
		"https://api.example.com#fragment",
		"https://api.example.com:8443",
		"https://192.0.2.10",
	} {
		if err := validateConfigValue(nativeplugin.ConfigURL, raw); err == nil {
			t.Fatalf("unsafe URL configuration %q was accepted", raw)
		}
	}
	if err := validateConfigValue(nativeplugin.ConfigURL, "https://api.example.com:443/base"); err != nil {
		t.Fatalf("safe URL was rejected: %v", err)
	}
}

func TestStringConfigurationPreservesWhitespace(t *testing.T) {
	manifest := testManifest("string_config", "1.0.0", nativeplugin.IdentityNone, nativeplugin.StateNone)
	manifest.Config = []nativeplugin.ConfigSpec{{Key: "PREFIX", Type: nativeplugin.ConfigString, Description: "Greeting prefix."}}
	config, _, err := resolveConfig(manifest, func(string) (string, bool) { return "Hello ", true })
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := config.String("PREFIX"); !ok || value != "Hello " {
		t.Fatalf("string configuration = %q, %v", value, ok)
	}
}
