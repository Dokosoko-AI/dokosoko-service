package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/nativeplugins"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type apiNativePlugin struct{}

func (apiNativePlugin) Describe() nativeplugin.Manifest {
	return nativeplugin.Manifest{
		ID: "api_status", Version: "1.0.0", SDKVersion: nativeplugin.SDKVersion,
		Description: "Exercise the native plugin administrative API.", StateVersion: 1,
		Config: []nativeplugin.ConfigSpec{{Key: "TOKEN", Type: nativeplugin.ConfigSecret, Required: true, Description: "Test-only secret."}},
		Tools: []nativeplugin.ToolSpec{{
			ID: "status", Namespace: "native_api", Name: "status", Description: "Return API test status.",
			InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
			Effect:       nativeplugin.EffectRead, Identity: nativeplugin.IdentityNone, StateScope: nativeplugin.StateNone,
			Idempotency: nativeplugin.IdempotencySupported, Timeout: time.Second, MaxConcurrency: 1, MaxResultBytes: 4096,
		}},
	}
}

func (apiNativePlugin) Open(context.Context, nativeplugin.Host) (nativeplugin.Instance, error) {
	return nativeplugin.NopInstance{}, nil
}

func TestNativePluginAdminStatusAndStateNeverExposeConfigValues(t *testing.T) {
	memory := store.NewMemory()
	manager, err := nativeplugins.New([]nativeplugin.Plugin{apiNativePlugin{}}, nativeplugins.Options{
		State: memory, IdentityKey: []byte("01234567890123456789012345678901"), Logger: log.New(io.Discard, "", 0),
		Environment: func(key string) (string, bool) {
			return "must-never-be-returned", key == "DOKOSOKO_PLUGIN_API_STATUS_TOKEN"
		},
	})
	if err != nil || manager.Start(context.Background()) != nil {
		t.Fatalf("manager start: %v", err)
	}
	defer manager.Close(context.Background())
	service := platform.New(memory)
	service.SetNativeToolCatalog(manager)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, NativePlugins: manager})

	status := request(t, handler, http.MethodGet, "/api/v1/native-plugins", "doko_admin_demo", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), "DOKOSOKO_PLUGIN_API_STATUS_TOKEN") || !strings.Contains(status.Body.String(), `"configured":true`) {
		t.Fatalf("status = %d: %s", status.Code, status.Body.String())
	}
	if strings.Contains(status.Body.String(), "must-never-be-returned") {
		t.Fatalf("configuration value leaked: %s", status.Body.String())
	}

	disabled := request(t, handler, http.MethodPatch, "/api/v1/native-plugins/api_status/state", "doko_admin_demo", `{"enabled":false}`)
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"state":"disabled"`) {
		t.Fatalf("disable = %d: %s", disabled.Code, disabled.Body.String())
	}
	audits, err := memory.AuditEvents(context.Background(), "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range audits {
		found = found || event.Action == "native_plugin.state_changed" && event.TargetID == "api_status"
	}
	if !found {
		t.Fatal("native plugin state audit was not recorded")
	}
}
