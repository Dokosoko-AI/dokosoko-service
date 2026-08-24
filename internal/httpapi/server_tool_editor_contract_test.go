package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestToolEditorPUTRequiresCompleteReplacementAndPreservesDraft(t *testing.T) {
	t.Parallel()
	handler := newCatalogServer(t)
	created := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools", "doko_admin_demo", `{"organisation_id":"org_acme","namespace":"platform","name":"readiness","description":"Check readiness.","input_schema":{"type":"object","additionalProperties":false,"properties":{}},"output_schema":{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string"}},"required":["status"]},"endpoint":"https://api.vendor.example/health/ready","http_method":"GET","authorization_policy":{"required_grants":[],"confirmation_required":false,"risk":"low"},"timeout_ms":5000}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create tool = %d: %s", created.Code, created.Body.String())
	}
	var original model.Tool
	if err := json.Unmarshal(created.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}

	complete := map[string]any{
		"description":          "Changed readiness description.",
		"input_schema":         map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}},
		"output_schema":        map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"status": map[string]any{"type": "string"}}, "required": []string{"status"}},
		"endpoint":             "https://api.vendor.example/health/ready-v2",
		"http_method":          "GET",
		"authorization_policy": map[string]any{"required_grants": []string{}, "confirmation_required": false, "risk": "low"},
		"timeout_ms":           6000,
		"revision":             original.Revision,
	}
	for _, field := range []string{"description", "input_schema", "output_schema", "authorization_policy", "timeout_ms", "revision", "endpoint", "http_method"} {
		t.Run("missing_"+field, func(t *testing.T) {
			input := make(map[string]any, len(complete)-1)
			for key, value := range complete {
				if key != field {
					input[key] = value
				}
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			response := request(t, handler, http.MethodPut, "/api/v1/products/prod_acme/tools/"+original.ID, "doko_admin_demo", string(body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("missing %s = %d: %s", field, response.Code, response.Body.String())
			}
		})
	}

	unchangedResponse := request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/tools/"+original.ID, "doko_admin_demo", "")
	if unchangedResponse.Code != http.StatusOK {
		t.Fatalf("get unchanged tool = %d: %s", unchangedResponse.Code, unchangedResponse.Body.String())
	}
	var unchanged model.Tool
	if err := json.Unmarshal(unchangedResponse.Body.Bytes(), &unchanged); err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != original.Revision || unchanged.Description != original.Description || unchanged.BaseURL != original.BaseURL || unchanged.HTTPMethod != original.HTTPMethod || unchanged.TimeoutMS != original.TimeoutMS {
		t.Fatalf("incomplete replacements changed draft: original=%#v unchanged=%#v", original, unchanged)
	}
}
