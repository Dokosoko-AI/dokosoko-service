package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAIReadinessIsAdministrativeReadOnlyAndReflectsConfiguration(t *testing.T) {
	memory := store.NewMemory()
	provider := &aitest.Knowledge{}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, nil, provider)
	handler := httpapi.New(service, "https://dokosoko.example")
	path := "/api/v1/ai/readiness"
	if res := request(t, handler, http.MethodGet, path, "", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous=%d", res.Code)
	}
	if res := request(t, handler, http.MethodPost, path, "doko_admin_demo", ""); res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method=%d", res.Code)
	}
	missing := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if missing.Code != http.StatusOK || !strings.Contains(missing.Body.String(), "model_missing") || missing.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing=%d %s", missing.Code, missing.Body.String())
	}
	if err := service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "never-return-this-credential", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	configured := request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	var status model.AIProcessingReadiness
	if err := json.Unmarshal(configured.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if configured.Code != http.StatusOK || !status.CanProcess || status.Model != "fixture" || status.ManagedBy != "environment" || status.LastTestedAt != nil {
		t.Fatalf("configured=%d %s", configured.Code, configured.Body.String())
	}
	if strings.Contains(configured.Body.String(), "never-return-this-credential") || strings.Contains(configured.Body.String(), "llm.example.test") || provider.Calls.Load() != 0 {
		t.Fatal("readiness disclosed credential/endpoint or contacted provider")
	}
}
