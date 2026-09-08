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
)

func TestKnowledgeProcessingHTTPGatesPublicationAndResumesExactImport(t *testing.T) {
	memory, service, handler := newDeveloperAssetServer()
	actor := platform.Actor{ID: "reviewer"}
	pkg, err := service.SaveSDKPackage(t.Context(), "", platform.SDKPackageInput{Ecosystem: "npm", Coordinate: "@acme/knowledge", Name: "Knowledge SDK", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(t.Context(), pkg.ID, platform.SDKReleaseInput{ExactVersion: "1.2.3", SourceURL: "https://example.test/sdk", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{Files: []platform.SDKIngestionFile{{SourcePath: "README.md", Content: "# SDK guide\n\nUse version 1.2.3.\n"}}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	route := "/api/v1/developer-assets/ingestion-runs/" + imported.Run.ID + "/processing"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		res := request(t, handler, method, route, "", "")
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s=%d %s", method, res.Code, res.Body.String())
		}
	}
	status := request(t, handler, http.MethodGet, route, "doko_admin_demo", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"required"`) {
		t.Fatalf("status=%d %s", status.Code, status.Body.String())
	}
	pubRoute := "/api/v1/developer-assets/sdk-releases/" + release.ID + "/content-candidates/" + imported.Candidate.Candidate.ID + "/publish"
	body, _ := json.Marshal(map[string]any{"acknowledge_reviewed": true, "files": []any{map[string]any{"id": imported.Candidate.Files[0].ID, "decision": "included"}}, "samples": []any{}})
	blocked := request(t, handler, http.MethodPost, pubRoute, "doko_admin_demo", string(body))
	if blocked.Code != http.StatusUnprocessableEntity || !strings.Contains(blocked.Body.String(), "knowledge_processing_required") {
		t.Fatalf("ungated publication=%d %s", blocked.Code, blocked.Body.String())
	}
	unavailable := request(t, handler, http.MethodPost, route, "doko_admin_demo", "")
	if unavailable.Code != http.StatusServiceUnavailable || !strings.Contains(unavailable.Body.String(), "ai_model_missing") {
		t.Fatalf("unconfigured=%d %s", unavailable.Code, unavailable.Body.String())
	}
	provider := &aitest.Knowledge{}
	service = platform.NewWithVaultAndProductBuilderDoer(memory, nil, provider)
	if err = service.ConfigureEnvironmentAI(t.Context(), platform.AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	handler = httpapi.New(service, "https://dokosoko.example")
	for range 2 {
		res := request(t, handler, http.MethodPost, route, "doko_admin_demo", "")
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"state":"ready"`) {
			t.Fatalf("process=%d %s", res.Code, res.Body.String())
		}
	}
	if provider.Calls.Load() != 1 {
		t.Fatalf("repeat calls=%d", provider.Calls.Load())
	}
	published := request(t, handler, http.MethodPost, pubRoute, "doko_admin_demo", string(body))
	if published.Code != http.StatusCreated {
		t.Fatalf("publish=%d %s", published.Code, published.Body.String())
	}
	unsupported := request(t, handler, http.MethodDelete, route, "doko_admin_demo", "")
	if unsupported.Code != http.StatusMethodNotAllowed || unsupported.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("unsupported=%d %s", unsupported.Code, unsupported.Body.String())
	}
	missing := request(t, handler, http.MethodGet, "/api/v1/developer-assets/ingestion-runs/missing/processing", "doko_admin_demo", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing=%d %s", missing.Code, missing.Body.String())
	}
}

func TestSDKDocumentationOnlyIngestionReturnsArraysOnCreateAndRetry(t *testing.T) {
	_, service, handler := newDeveloperAssetServer()
	actor := platform.Actor{ID: "reviewer"}
	pkg, err := service.SaveSDKPackage(t.Context(), "", platform.SDKPackageInput{Ecosystem: "npm", Coordinate: "@acme/readme-wire", Name: "README SDK", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(t.Context(), pkg.ID, platform.SDKReleaseInput{ExactVersion: "1.2.3", SourceURL: "https://example.test/sdk", Visibility: model.VisibilityPrivate}, actor)
	if err != nil {
		t.Fatal(err)
	}
	var candidateID string
	for _, expected := range []int{http.StatusCreated, http.StatusOK} {
		res := request(t, handler, http.MethodPost, "/api/v1/developer-assets/sdk-releases/"+release.ID+"/ingestions", "doko_admin_demo", `{"files":[{"source_path":"README.md","content":"# SDK guide\n\nUse version 1.2.3.\n"}]}`)
		var body struct {
			Candidate map[string]json.RawMessage `json:"candidate"`
		}
		if res.Code != expected || json.Unmarshal(res.Body.Bytes(), &body) != nil {
			t.Fatalf("ingestion=%d %s", res.Code, res.Body.String())
		}
		for _, key := range []string{"files", "sections", "symbols", "samples", "sample_refs"} {
			if len(body.Candidate[key]) == 0 || body.Candidate[key][0] != '[' {
				t.Fatalf("%s must be an array: %s", key, res.Body.String())
			}
		}
		var candidate model.SDKContentCandidate
		if err = json.Unmarshal(body.Candidate["candidate"], &candidate); err != nil {
			t.Fatal(err)
		}
		if candidateID != "" && candidate.ID != candidateID {
			t.Fatal("retry replaced candidate")
		}
		candidateID = candidate.ID
	}
}
