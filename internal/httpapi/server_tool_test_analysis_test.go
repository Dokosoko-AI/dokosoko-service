package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type toolTestAnalysisHTTPDoer struct{ calls int }

func (d *toolTestAnalysisHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	structured, _ := json.Marshal(map[string]any{"reply": "Review the sanitized response shape.", "findings": []any{}, "proposal_json": ""})
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(structured)}}}, "usage": map[string]any{"total_tokens": 12}})
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
}

func TestToolTestAnalysisHTTPRequiresStrictConsentAndExactEvidenceBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secrets.New(bytes.Repeat([]byte{0x38}, 32))
	if err != nil {
		t.Fatal(err)
	}
	doer := &toolTestAnalysisHTTPDoer{}
	service := platform.NewWithVaultAndProductBuilderDoer(memory, vault, doer)
	actor := platform.Actor{ID: "root", RequestID: "setup"}
	connection, err := service.SaveAIProviderConnection(ctx, platform.AIProviderConnectionInput{OrganisationID: "org_acme", DeploymentID: "prod_acme", Provider: "openai-compatible", Endpoint: "https://analysis.example.test", Credential: "provider-secret", Enabled: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAIWorkloadProfile(ctx, platform.AIWorkloadProfileInput{OrganisationID: "org_acme", ProductID: "prod_acme", Workload: "analysis", ProviderConnectionID: connection.ID, Model: "analysis-model", MaxInputTokens: 4096, MaxOutputTokens: 2048, DailyTokenBudget: 20000, Enabled: true}, actor); err != nil {
		t.Fatal(err)
	}
	tool, err := service.CreateTool(ctx, platform.ToolInput{
		ProductID: "prod_acme", Namespace: "health", Name: "check", Description: "Check service health.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"healthy":{"type":"boolean"}}}`),
		Endpoint: "https://api.example.test/health", HTTPMethod: http.MethodGet, UpstreamAuth: json.RawMessage(`{"type":"none"}`), AuthorizationPolicy: json.RawMessage(`{"required_grants":[]}`), TimeoutMS: 1000,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	argumentHash := sha256.Sum256([]byte("discarded"))
	responseShape := model.JSONShape{Type: "object", Properties: map[string]model.JSONShape{"healthy": {Type: "boolean"}}}
	run := model.ToolTestRun{
		ID: "analysis-http-run", OrganisationID: tool.OrganisationID, ProductID: tool.ProductID, ToolID: tool.ID, ToolRevision: tool.Revision,
		ToolName: "health.check", ArgumentHash: argumentHash[:], Method: http.MethodGet, AuthenticationType: "none", Outcome: "success", Phase: "complete",
		NetworkCallPerformed: true, UpstreamStatusCode: http.StatusOK, ResponseBytes: 16, RequestShape: model.JSONShape{Type: "object"}, ResponseShape: &responseShape,
		Findings: []model.ToolTestFinding{}, DurationMS: 8, CreatedAt: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := memory.AppendToolTestRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(service, "https://dokosoko.example")
	path := "/api/v1/products/prod_acme/tools/" + tool.ID + "/test-runs/" + run.ID + "/analyse"
	hash := platform.ToolTestAnalysisEvidenceHash(run)

	response := request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"revision":1,"evidence_hash":"`+hash+`","question":"What does the sanitized evidence show?"}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") || doer.calls != 0 {
		t.Fatalf("missing explicit consent = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"revision":1,"evidence_hash":"`+hash+`","consent_to_analysis_provider":false,"question":"What does the sanitized evidence show?"}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "tool_test_analysis_consent_required") || response.Header().Get("Cache-Control") != "no-store" || doer.calls != 0 {
		t.Fatalf("declined consent = %d calls=%d headers=%v: %s", response.Code, doer.calls, response.Header(), response.Body.String())
	}
	response = request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"revision":1,"evidence_hash":"`+hash+`","consent_to_analysis_provider":true,"question":"What does the sanitized evidence show?","unexpected":true}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") || doer.calls != 0 {
		t.Fatalf("unknown field = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"revision":1,"evidence_hash":"sha256:`+strings.Repeat("0", 64)+`","consent_to_analysis_provider":true,"question":"What does the sanitized evidence show?"}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "tool_test_analysis_evidence_mismatch") || doer.calls != 0 {
		t.Fatalf("mismatched hash = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodPost, path, "doko_admin_demo", `{"revision":1,"evidence_hash":"`+hash+`","consent_to_analysis_provider":true,"question":"What does the sanitized evidence show?","history":[{"role":"user","content":"Only structural evidence is in scope."}]}`)
	if response.Code != http.StatusOK || doer.calls != 1 || !strings.Contains(response.Body.String(), `"evidence_hash":"`+hash+`"`) || !strings.Contains(response.Body.String(), `"advisory":true`) || strings.Contains(response.Body.String(), run.ID) || strings.Contains(response.Body.String(), tool.ID) {
		t.Fatalf("analysis = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodGet, path, "doko_admin_demo", "")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET exact analysis route = %d headers=%v: %s", response.Code, response.Header(), response.Body.String())
	}
}
