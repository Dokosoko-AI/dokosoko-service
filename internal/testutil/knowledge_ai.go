package testutil

import (
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func ProcessKnowledge(t testing.TB, backend store.Store, runID string) {
	t.Helper()
	service := platform.NewWithVaultAndProductBuilderDoer(backend, nil, &aitest.Knowledge{})
	if err := service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	for {
		status, err := service.ProcessKnowledgeBatch(t.Context(), runID, platform.Actor{ID: "processing-fixture"})
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "ready" {
			return
		}
	}
}
