package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/tools"
)

type toolLiveResolver struct{}

func (toolLiveResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

type toolLiveDoer struct{ calls int }

func (d *toolLiveDoer) Do(*http.Request) (*http.Response, error) {
	d.calls++
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}

type exactIntegerToolLiveDoer struct {
	calls int
	body  string
}

func (d *exactIntegerToolLiveDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	encoded, _ := io.ReadAll(request.Body)
	d.body = string(encoded)
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
}

func TestToolLiveTestHTTPConfirmationRunReplayAndEvidenceLookup(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "http_test", Description: "Test one existing API operation.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"label":{"type":"string"}},"required":["label"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodPost, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"high","idempotency_required":true}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	doer := &toolLiveDoer{}
	runtime := tools.NewRuntime(memory, toolLiveResolver{}, doer)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, ToolRuntime: runtime})
	arguments := map[string]any{"label": "private-request-value"}
	confirmationBody, _ := json.Marshal(map[string]any{"revision": tool.Revision, "arguments": arguments, "typed_tool_name": "live.http_test", "acknowledge_side_effects": true})
	response := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-confirmations", "doko_admin_demo", string(confirmationBody)+` {}`)
	if response.Code != http.StatusBadRequest || doer.calls != 0 {
		t.Fatalf("trailing confirmation JSON = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-confirmations", "doko_admin_demo", string(confirmationBody))
	if response.Code != http.StatusCreated {
		t.Fatalf("confirmation = %d: %s", response.Code, response.Body.String())
	}
	var confirmation struct {
		Nonce string `json:"confirmation_nonce"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &confirmation); err != nil || confirmation.Nonce == "" {
		t.Fatalf("confirmation response = %s err=%v", response.Body.String(), err)
	}
	runBody, _ := json.Marshal(map[string]any{"revision": tool.Revision, "arguments": arguments, "confirmation_nonce": confirmation.Nonce, "idempotency_key": "http-live-test-0001"})
	response = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-runs", "doko_admin_demo", string(runBody))
	if response.Code != http.StatusCreated || doer.calls != 1 {
		t.Fatalf("run = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private-request-value") || strings.Contains(response.Body.String(), confirmation.Nonce) || strings.Contains(response.Body.String(), "api.example.test") {
		t.Fatalf("run response leaked request material: %s", response.Body.String())
	}
	var run struct {
		ID           string `json:"id"`
		EvidenceHash string `json:"evidence_hash"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil || run.ID == "" || !strings.HasPrefix(run.EvidenceHash, "sha256:") || len(run.EvidenceHash) != 71 {
		t.Fatalf("run response = %s err=%v", response.Body.String(), err)
	}
	response = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-runs", "doko_admin_demo", string(runBody))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "tool_test_confirmation_replayed") || doer.calls != 1 {
		t.Fatalf("replay = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	response = request(t, handler, http.MethodGet, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-runs/"+run.ID, "doko_admin_demo", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"request_shape"`) || !strings.Contains(response.Body.String(), `"evidence_hash":"`+run.EvidenceHash+`"`) {
		t.Fatalf("lookup = %d: %s", response.Code, response.Body.String())
	}
}

func TestToolLiveTestHTTPPreservesExactLargeIntegerArguments(t *testing.T) {
	memory := store.NewMemory()
	service := platform.New(memory)
	tool, err := service.CreateTool(context.Background(), platform.ToolInput{
		ProductID: "prod_acme", Namespace: "live", Name: "large_integer", Description: "Test one exact integer payload.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"count":{"type":"integer"}},"required":["count"]}`),
		OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
		Endpoint:     "https://api.example.test/items", HTTPMethod: http.MethodPost, UpstreamAuth: json.RawMessage(`{"type":"none"}`),
		AuthorizationPolicy: json.RawMessage(`{"required_grants":[],"confirmation_required":true,"risk":"high","idempotency_required":true}`), TimeoutMS: 1000,
	}, platform.Actor{ID: "root", RequestID: "create"})
	if err != nil {
		t.Fatal(err)
	}
	doer := &exactIntegerToolLiveDoer{}
	runtime := tools.NewRuntime(memory, toolLiveResolver{}, doer)
	handler := httpapi.NewWithOptions(service, httpapi.Options{BaseURL: "https://dokosoko.example", AllowDemoTokens: true, ToolRuntime: runtime})
	confirmationBody := fmt.Sprintf(`{"revision":%d,"arguments":{"count":9007199254740993},"typed_tool_name":"live.large_integer","acknowledge_side_effects":true}`, tool.Revision)
	response := request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-confirmations", "doko_admin_demo", confirmationBody)
	if response.Code != http.StatusCreated {
		t.Fatalf("confirmation = %d: %s", response.Code, response.Body.String())
	}
	var confirmation struct {
		Nonce string `json:"confirmation_nonce"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &confirmation); err != nil || confirmation.Nonce == "" {
		t.Fatalf("confirmation response = %s err=%v", response.Body.String(), err)
	}
	mismatchedRunBody := fmt.Sprintf(`{"revision":%d,"arguments":{"count":9007199254740992},"confirmation_nonce":"%s","idempotency_key":"large-integer-test-01"}`, tool.Revision, confirmation.Nonce)
	response = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-runs", "doko_admin_demo", mismatchedRunBody)
	if response.Code != http.StatusConflict || doer.calls != 0 || !strings.Contains(response.Body.String(), "tool_test_confirmation_invalid") {
		t.Fatalf("mismatched integer run = %d calls=%d: %s", response.Code, doer.calls, response.Body.String())
	}
	exactRunBody := fmt.Sprintf(`{"revision":%d,"arguments":{"count":9007199254740993},"confirmation_nonce":"%s","idempotency_key":"large-integer-test-01"}`, tool.Revision, confirmation.Nonce)
	response = request(t, handler, http.MethodPost, "/api/v1/products/prod_acme/tools/"+tool.ID+"/test-runs", "doko_admin_demo", exactRunBody)
	if response.Code != http.StatusCreated || doer.calls != 1 || doer.body != `{"count":9007199254740993}` {
		t.Fatalf("exact integer run = %d calls=%d body=%q: %s", response.Code, doer.calls, doer.body, response.Body.String())
	}
}
