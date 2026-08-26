package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestRemovedAIEndpointsReturnGone(t *testing.T) {
	t.Parallel()

	handler := newServer()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		code   string
	}{
		{name: "list legacy LLM profiles", method: http.MethodGet, path: "/api/v1/products/prod_acme/llm-profiles", code: "llm_profiles_removed"},
		{name: "save legacy LLM profile", method: http.MethodPut, path: "/api/v1/products/prod_acme/llm-profiles", body: `{}`, code: "llm_profiles_removed"},
		{name: "answer analysis unknown", method: http.MethodPatch, path: "/api/v1/products/prod_acme/analyses/analysis_retired", body: `{"answers":{"gap":"asserted by an operator"}}`, code: "analysis_answers_removed"},
		{name: "read attention inbox", method: http.MethodGet, path: "/api/v1/products/prod_acme/attention", code: "attention_endpoint_removed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, test.method, test.path, "doko_admin_demo", test.body)
			if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("response = %d: %s, want 410 with %q", response.Code, response.Body.String(), test.code)
			}
		})
	}
}
