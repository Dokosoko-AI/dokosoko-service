package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const taskURI = "dokosoko://products/example/recipes/payment-status"
const evidenceURI = "dokosoko://developer-assets/apis/payments/publications/publication-1/evidence/sample-1"
const taskText = "# Receive payment status\n\nUse SDK 1.2.3 and verify one completed payment.\n"
const evidenceText = "const status = 'completed';\n"

func reviewedTaskPlan(endpoint string) TaskPlan {
	return TaskPlan{SchemaVersion: TaskPlanVersion, Endpoint: endpoint,
		Task: TaskSelection{Title: "Receive payment status", Outcome: "One payment status is received and checked.", ResourceURI: taskURI, RevisionID: "recipe-revision-1"},
		Resources: []ResourceExpectation{
			{URI: taskURI, Discover: true, TextSHA256: textHash([]byte(taskText)), MIMEType: "text/markdown", Metadata: map[string]any{"revision_id": "recipe-revision-1", "integration_ids": []string{"payments"}}},
			{URI: evidenceURI, TextSHA256: textHash([]byte(evidenceText)), MIMEType: "text/markdown", Metadata: map[string]any{"api_id": "payments", "source_publication_id": "sdk-publication-1"}},
		},
	}
}

func TestTaskPlanChecksExactDiscoveryAndReadsWithoutClaimingImplementation(t *testing.T) {
	tests := []struct {
		name   string
		change func(string, map[string]any)
		fail   bool
	}{
		{name: "exact task plus a targeted evidence read"},
		{name: "unrelated resource returned", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				result["contents"].([]map[string]any)[0]["uri"] = "dokosoko://wrong-scope"
			}
		}},
		{name: "missing content", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				delete(result, "contents")
			}
		}},
		{name: "empty content", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				result["contents"].([]map[string]any)[0]["text"] = ""
			}
		}},
		{name: "new content at the same URI", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				result["contents"].([]map[string]any)[0]["text"] = "untrusted-private-response-that-must-not-be-logged"
			}
		}},
		{name: "wrong release metadata with matching text", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				result["contents"].([]map[string]any)[0]["_meta"] = map[string]any{"revision_id": "wrong-revision"}
			}
		}},
		{name: "wrong media type", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				result["contents"].([]map[string]any)[0]["mimeType"] = "text/html"
			}
		}},
		{name: "ambiguous content items", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/read" {
				values := result["contents"].([]map[string]any)
				result["contents"] = append(values, values[0])
			}
		}},
		{name: "task no longer advertised", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/list" {
				result["resources"] = []map[string]any{}
			}
		}},
		{name: "ambiguous advertised task", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/list" {
				values := result["resources"].([]map[string]any)
				result["resources"] = append(values, values[0])
			}
		}},
		{name: "discovery revision changed before correct read", fail: true, change: func(method string, result map[string]any) {
			if method == "resources/list" {
				result["resources"].([]map[string]any)[0]["_meta"] = map[string]any{"revision_id": "another-revision"}
			}
		}},
	}
	for _, scenario := range tests {
		t.Run(scenario.name, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
					return
				}
				w.Header().Set("X-Request-ID", request.ID)
				result := map[string]any{}
				switch request.Method {
				case "server/discover":
					result["supportedVersions"] = []string{ProtocolVersion}
				case "resources/list":
					result["resources"] = []map[string]any{{"uri": taskURI, "mimeType": "text/markdown", "_meta": map[string]any{"revision_id": "recipe-revision-1", "integration_ids": []string{"payments"}}}}
				case "resources/read":
					reads.Add(1)
					uri, _ := request.Params["uri"].(string)
					text, metadata := taskText, map[string]any{"revision_id": "recipe-revision-1", "integration_ids": []string{"payments"}}
					if uri == evidenceURI {
						text, metadata = evidenceText, map[string]any{"api_id": "payments", "source_publication_id": "sdk-publication-1"}
					} else if uri != taskURI {
						t.Errorf("unexpected read %q", uri)
					}
					result["contents"] = []map[string]any{{"uri": uri, "mimeType": "text/markdown", "text": text, "_meta": metadata}}
				case "tools/list":
					result["tools"] = []map[string]any{}
				default:
					t.Errorf("task check attempted unexpected method %q", request.Method)
				}
				if scenario.change != nil {
					scenario.change(request.Method, result)
				}
				writeTestRPC(w, request.ID, result, nil)
			}))
			defer server.Close()
			plan := reviewedTaskPlan(server.URL)
			report, err := Run(context.Background(), Config{Endpoint: server.URL, AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL), TaskPlan: &plan})
			if err != nil {
				t.Fatal(err)
			}
			if report.Accepted() == scenario.fail {
				t.Fatalf("accepted=%t, checks=%+v", report.Accepted(), report.Checks)
			}
			if report.Task == nil || (report.Task.RetrievalStatus == Pass) == scenario.fail || report.Task.ImplementationStatus != "not_run" {
				t.Fatalf("incorrect task outcome: %+v", report.Task)
			}
			if reads.Load() != 2 {
				t.Fatalf("read %d resources, want exact task and evidence", reads.Load())
			}
			if report.EvidenceOrigin != "acceptance_client_observation" || report.ClientVersion != ClientVersion {
				t.Fatalf("client identity was not recorded: %+v", report)
			}
			data, _ := json.Marshal(report)
			for _, privateBody := range []string{taskText, evidenceText, "untrusted-private-response-that-must-not-be-logged"} {
				if strings.Contains(string(data), privateBody) {
					t.Fatal("report included resource body")
				}
			}
			if !scenario.fail {
				for _, check := range report.Checks {
					if strings.HasPrefix(check.Name, "resources/read:") && (check.ContentSHA256 == "" || check.ContentBytes == 0 || check.RequestID == "") {
						t.Fatalf("missing exact read evidence: %+v", check)
					}
				}
			}
		})
	}
}

func TestTaskPlanRejectsIncompleteOrDifferentSelectionBeforeNetwork(t *testing.T) {
	tests := []struct {
		name   string
		change func(*TaskPlan)
	}{
		{"different endpoint", func(p *TaskPlan) { p.Endpoint = "https://different.example/mcp" }},
		{"unknown schema", func(p *TaskPlan) { p.SchemaVersion = "future" }},
		{"missing outcome", func(p *TaskPlan) { p.Task.Outcome = "" }},
		{"missing task resource", func(p *TaskPlan) { p.Resources = p.Resources[1:] }},
		{"duplicate resource", func(p *TaskPlan) { p.Resources = append(p.Resources, p.Resources[0]) }},
		{"floating revision", func(p *TaskPlan) { delete(p.Resources[0].Metadata, "revision_id") }},
		{"different revision", func(p *TaskPlan) { p.Task.RevisionID = "another" }},
		{"task discovery disabled", func(p *TaskPlan) { p.Resources[0].Discover = false }},
		{"missing text hash", func(p *TaskPlan) { p.Resources[0].TextSHA256 = "" }},
		{"URI with user info", func(p *TaskPlan) { p.Resources[1].URI = "https://secret@wrong.example/resource" }},
		{"oversized metadata", func(p *TaskPlan) { p.Resources[1].Metadata["too_large"] = strings.Repeat("x", maxTaskPlanBytes) }},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, scenario := range tests {
		t.Run(scenario.name, func(t *testing.T) {
			plan := reviewedTaskPlan(server.URL)
			scenario.change(&plan)
			if _, err := Run(context.Background(), Config{Endpoint: server.URL, AllowedLoopbackHTTP: testLoopbackHTTP(t, server.URL), Token: "never-send", TaskPlan: &plan}); err == nil {
				t.Fatal("invalid task plan was accepted")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("invalid task plan made a network request")
	}
}

func TestLoadTaskPlanRejectsUnsupportedOrTrailingInput(t *testing.T) {
	plan := reviewedTaskPlan("https://example.test/mcp")
	data, _ := json.Marshal(plan)
	for name, value := range map[string][]byte{
		"valid":     data,
		"unknown":   append([]byte(`{"ignored_field":true,`), data[1:]...),
		"trailing":  append(append([]byte(nil), data...), []byte(` {}`)...),
		"oversized": []byte(strings.Repeat(" ", maxTaskPlanBytes+1)),
		"null":      []byte("null"),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan.json")
			if err := os.WriteFile(path, value, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadTaskPlan(path)
			if name != "valid" {
				if err == nil {
					t.Fatal("invalid plan accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if loaded.fingerprint() != plan.fingerprint() {
				t.Fatal("round trip changed plan fingerprint")
			}
		})
	}
}
