// Package testutil contains explicit, network-free service fixtures.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type RecipeAI struct {
	FailStage string
	Failure   error
}

func (d RecipeAI) Do(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if d.FailStage != "" && strings.Contains(string(body), d.FailStage) {
		return nil, d.Failure
	}
	content := `{"status":"ready","reference_ids":[],"gaps":[]}`
	switch {
	case strings.Contains(string(body), "Recipe review contract:"):
		content = `{"recommendation":"pass","findings":[]}`
	case strings.Contains(string(body), "Integration analysis contract:"):
		content = `{"summary":"No additional recommendations.","summary_evidence_ids":[],"recipes":[]}`
	}
	payload, err := json.Marshal(map[string]any{
		"id": "recipe-fixture", "model": "fixture",
		"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}},
		"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 10},
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
}

func NewRecipeService(t testing.TB, backend store.Store, provider RecipeAI) *platform.Service {
	t.Helper()
	service := platform.NewWithVaultAndProductBuilderDoer(backend, nil, provider)
	if err := service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{
		Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test",
		Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	return service
}
