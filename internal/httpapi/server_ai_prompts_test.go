package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func decodeAIPromptConfiguration(t *testing.T, responseBody string) model.AIPromptConfiguration {
	t.Helper()
	var value model.AIPromptConfiguration
	if err := json.Unmarshal([]byte(responseBody), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAIWorkflowPromptAPIUsesOptimisticRevisions(t *testing.T) {
	t.Parallel()

	handler := newServer()
	listed := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/ai-prompts", "doko_admin_demo", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list prompts = %d: %s", listed.Code, listed.Body.String())
	}
	var collection struct {
		Items []model.AIPromptConfiguration `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &collection); err != nil {
		t.Fatal(err)
	}
	if len(collection.Items) != 4 {
		t.Fatalf("prompt count = %d: %s", len(collection.Items), listed.Body.String())
	}
	for _, item := range collection.Items {
		if item.Source != "default" || item.Revision != 1 || item.UpdatedAt != nil || strings.Contains(item.Instructions, "Trust and execution policy:") {
			t.Fatalf("unexpected default prompt response: %#v", item)
		}
	}

	instructions := "Use only exact cited evidence and report every material gap."
	encoded, err := json.Marshal(map[string]any{"instructions": instructions, "revision": 1})
	if err != nil {
		t.Fatal(err)
	}
	savedResponse := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/ai-prompts/integration.analysis", "doko_admin_demo", string(encoded))
	if savedResponse.Code != http.StatusOK {
		t.Fatalf("save prompt = %d: %s", savedResponse.Code, savedResponse.Body.String())
	}
	saved := decodeAIPromptConfiguration(t, savedResponse.Body.String())
	if saved.Source != "override" || saved.Revision != 2 || saved.Instructions != instructions || saved.UpdatedAt == nil {
		t.Fatalf("saved prompt response = %#v", saved)
	}

	staleResponse := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/ai-prompts/integration.analysis", "doko_admin_demo", string(encoded))
	if staleResponse.Code != http.StatusConflict || !strings.Contains(staleResponse.Body.String(), "revision_conflict") {
		t.Fatalf("stale save = %d: %s", staleResponse.Code, staleResponse.Body.String())
	}

	resetResponse := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/ai-prompts/integration.analysis/reset", "doko_admin_demo", `{"revision":2}`)
	if resetResponse.Code != http.StatusOK {
		t.Fatalf("reset prompt = %d: %s", resetResponse.Code, resetResponse.Body.String())
	}
	reset := decodeAIPromptConfiguration(t, resetResponse.Body.String())
	if reset.Source != "default" || reset.Revision != 3 || reset.Instructions == instructions || reset.EffectiveVersion != reset.DefaultVersion {
		t.Fatalf("reset prompt response = %#v", reset)
	}

	preResetRevision := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/ai-prompts/integration.analysis", "doko_admin_demo", `{"instructions":"Stale after reset.","revision":2}`)
	if preResetRevision.Code != http.StatusConflict {
		t.Fatalf("pre-reset revision save = %d: %s", preResetRevision.Code, preResetRevision.Body.String())
	}
}

func TestAIWorkflowPromptAPIRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	handler := newServer()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "unknown field", method: http.MethodPut, path: "/api/v1/products/prod_acme/ai-prompts/recipe.brief", body: `{"instructions":"Use evidence.","revision":1,"extra":true}`, status: http.StatusBadRequest, code: "invalid_request"},
		{name: "empty instructions", method: http.MethodPut, path: "/api/v1/products/prod_acme/ai-prompts/recipe.brief", body: `{"instructions":" ","revision":1}`, status: http.StatusUnprocessableEntity, code: "invalid_ai_prompt"},
		{name: "missing revision", method: http.MethodPut, path: "/api/v1/products/prod_acme/ai-prompts/recipe.brief", body: `{"instructions":"Use evidence."}`, status: http.StatusUnprocessableEntity, code: "invalid_ai_prompt"},
		{name: "unknown key", method: http.MethodPut, path: "/api/v1/products/prod_acme/ai-prompts/recipe.unknown", body: `{"instructions":"Use evidence.","revision":1}`, status: http.StatusNotFound, code: "not_found"},
		{name: "missing product", method: http.MethodGet, path: "/api/v1/products/missing/ai-prompts", status: http.StatusNotFound, code: "not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, test.method, test.path, "doko_admin_demo", test.body)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("response = %d: %s, want %d with %q", response.Code, response.Body.String(), test.status, test.code)
			}
		})
	}
}
