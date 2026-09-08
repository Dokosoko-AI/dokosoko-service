package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/internal/testutil"
)

// This crosses the standalone module boundary using its real built CLI. It is
// opt-in because the root module does not own or build example executables.
func TestPublishedTaskWithStandaloneAcceptanceClient(t *testing.T) {
	runPublishedTaskAcceptance(t, false, false)
}

func TestPublishedPublicTaskWithStandaloneAcceptanceClient(t *testing.T) {
	runPublishedTaskAcceptance(t, true, false)
}

func TestPublishedPagedTaskWithStandaloneAcceptanceClient(t *testing.T) {
	runPublishedTaskAcceptance(t, false, true)
}

func runPublishedTaskAcceptance(t *testing.T, public, paged bool) {
	t.Helper()
	binary := os.Getenv("DOKOSOKO_MCP_ACCEPTANCE_CLIENT")
	if binary == "" {
		t.Skip("set DOKOSOKO_MCP_ACCEPTANCE_CLIENT to the built standalone CLI")
	}
	ctx := context.Background()
	memory := store.NewMemory()
	overlay := &scopedMCPDeveloperAssetStore{Memory: memory, apiIndexes: map[string]store.SearchIndexGenerationRecord{}}
	service := platform.NewWithVaultAndProductBuilderDoer(overlay, nil, &taskConnectionAI{})
	if err := service.ConfigureEnvironmentAI(ctx, platform.AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	browserAddress := os.Getenv("DOKOSOKO_TASK_CONNECTION_BROWSER_ADDRESS")
	if browserAddress != "" {
		host, _, err := net.SplitHostPort(browserAddress)
		if err != nil || host != "127.0.0.1" {
			t.Fatal("browser fixture requires an exact 127.0.0.1 address")
		}
		server.Listener.Close()
		server.Listener, err = net.Listen("tcp", browserAddress)
		if err != nil {
			t.Fatal(err)
		}
	}
	baseURL := "http://" + server.Listener.Addr().String()
	options := httpapi.Options{BaseURL: baseURL, AllowDemoTokens: true}
	done := make(chan struct{})
	if browserAddress != "" {
		vault, err := secrets.New(bytes.Repeat([]byte{7}, 32))
		if err != nil {
			t.Fatal(err)
		}
		options.IdentityBroker = identity.NewBroker(memory, vault, baseURL, nil, nil, nil)
		options.UIDirectory, err = filepath.Abs("../../dist/client")
		if err != nil {
			t.Fatal(err)
		}
	}
	handler := httpapi.NewWithOptions(service, options)
	mux := http.NewServeMux()
	if browserAddress != "" {
		var once sync.Once
		mux.HandleFunc("POST /acceptance/stop", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer doko_admin_demo" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			once.Do(func() { close(done) })
		})
	}
	mux.Handle("/", handler)
	server.Config.Handler = mux
	server.Start()
	actor := platform.Actor{ID: "task-acceptance", RequestID: "task-acceptance"}
	visibility, mcpPath, token := model.VisibilityPrivate, "/mcp", "doko_private_demo"
	if public {
		visibility, mcpPath, token = model.VisibilityPublic, "/mcp/public", ""
		enablePublicMCPForDeveloperAssetTest(t, ctx, memory, "prod_acme")
	}
	integration, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: "orders-task", VersionKey: "v1", DisplayName: "Orders & returns", Description: "Read one order status.", Visibility: visibility, AcknowledgePublic: public, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if !public {
		prepareHTTPPrivateIntegrationFoundations(t, handler, integration.ID)
	}
	prepareTaskConnectionSDK(t, service, integration.ID, visibility, actor)
	if !public {
		prepareTaskConnectionContract(t, memory, service, integration.ID, actor)
	}
	var globalPublication model.DeploymentDocumentationPublication
	if public {
		globalPublication = prepareTaskGlobalDocumentation(t, memory, service, 1, actor)
	}
	preparePublishedRecipeHTTPIntegration(t, ctx, memory, service, integration, "orders", actor)
	analysis, err := service.AnalyseIntegrationFor(ctx, "prod_acme", integration.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := service.GenerateRecipesForIntegration(ctx, "prod_acme", analysis.ID, integration.ID, actor)
	if err != nil || len(recipes) != 1 {
		t.Fatalf("required AI recipe generation: recipes=%d err=%v", len(recipes), err)
	}
	recipe := recipes[0]
	if public {
		recipe, err = service.UpdateRecipeReferences(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, nil, model.VisibilityPublic, actor)
		if err != nil {
			t.Fatal(err)
		}
	}
	recipe, err = service.ApproveRecipe(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, actor)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err = service.PublishRecipe(ctx, "prod_acme", recipe.ID, recipe.Revision, recipe.CurrentRevisionID, actor)
	if err != nil {
		t.Fatal(err)
	}
	selectedSDK, selectedOperation := false, false
	for _, dependency := range recipe.Dependencies {
		selectedSDK = selectedSDK || dependency.Kind == "developer_asset_sdk"
		selectedOperation = selectedOperation || dependency.Kind == "product_contract_operation"
	}
	if !selectedSDK || (!public && !selectedOperation) {
		t.Fatal("task must select actual reviewed SDK evidence and a contract operation")
	}
	if public {
		selectedGlobal := false
		for _, dependency := range recipe.Dependencies {
			selectedGlobal = selectedGlobal || strings.HasPrefix(dependency.ResourceID, "developer_asset:global_documentation:")
		}
		if !selectedGlobal {
			t.Fatal("public task must select historical global evidence")
		}
		prepareTaskGlobalDocumentation(t, memory, service, 2, actor)
	}
	if paged {
		for index := 0; index < 34; index++ {
			extra, err := service.CreateIntegration(ctx, platform.IntegrationInput{FamilyKey: fmt.Sprintf("extra-page-%02d", index), VersionKey: "v1", DisplayName: "Additional published API", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
			if err != nil {
				t.Fatal(err)
			}
			attachMCPTestContract(t, service, extra.ID)
			if _, err = service.PublishIntegration(ctx, extra.ID, actor); err != nil {
				t.Fatal(err)
			}
		}
	}
	if browserAddress != "" {
		// Optional growth overlay exercises the real console and MCP path without
		// claiming acquisition or model-quality evidence for these synthetic units.
		if os.Getenv("DOKOSOKO_TASK_CONNECTION_LARGE_INDEX") == "1" {
			publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, integration.ID)
			if err != nil {
				t.Fatal(err)
			}
			record, err := service.ReadyDeveloperAssetSearchIndex(ctx, "api", publication.ID)
			if err != nil || len(record.Units) == 0 || len(record.APIScopes) == 0 {
				t.Fatal("growth fixture needs selected API evidence")
			}
			original, scope := record.Units[0], record.APIScopes[0]
			for index := 0; index < 9000; index++ {
				unit := original
				unit.ID = fmt.Sprintf("growth-unit-%04d", index)
				unit.SourceEntityID, unit.Title, unit.Ordinal = unit.ID, "Unrelated growth fixture evidence "+unit.ID, len(record.Units)
				unit.Content = "Unrelated documentation for a different integration task."
				unit.ContentHash = mcpAssetHash(unit.Content)
				record.Units = append(record.Units, unit)
				row := scope
				row.KnowledgeUnitID = unit.ID
				record.APIScopes = append(record.APIScopes, row)
			}
			record.Generation.UnitCount = len(record.Units)
			overlay.apiIndexes[publication.ID] = record
		}
		path := os.Getenv("DOKOSOKO_TASK_CONNECTION_BROWSER_FIXTURE")
		if path == "" {
			t.Fatal("browser fixture output path is required")
		}
		data, _ := json.Marshal(map[string]any{"base_url": baseURL, "api": integration, "recipe": recipe, "audience": visibility, "global_publication": globalPublication})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("Task connection browser fixture ready at " + baseURL)
		<-done
		return
	}

	type resourceCatalog struct {
		Result struct {
			Resources []struct {
				URI      string         `json:"uri"`
				Metadata map[string]any `json:"_meta"`
			} `json:"resources"`
			NextCursor string `json:"nextCursor"`
		} `json:"result"`
	}
	var catalog resourceCatalog
	cursor := ""
	for pageNumber := 0; pageNumber < 64; pageNumber++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/list", "params": params})
		list := request(t, handler, http.MethodPost, mcpPath, token, string(body))
		var page resourceCatalog
		if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil || page.Result.Resources == nil {
			t.Fatalf("resource discovery failed: %.1024s", list.Body.String())
		}
		if paged && pageNumber == 0 {
			if page.Result.NextCursor == "" {
				t.Fatal("large task fixture did not paginate")
			}
			for _, resource := range page.Result.Resources {
				if resource.URI == recipe.StableURI {
					t.Fatal("task must appear after the first discovery page")
				}
			}
		}
		catalog.Result.Resources = append(catalog.Result.Resources, page.Result.Resources...)
		cursor = page.Result.NextCursor
		if cursor == "" {
			break
		}
	}
	if cursor != "" {
		t.Fatal("fixture discovery exceeded page budget")
	}
	resourceURI := ""
	for _, resource := range catalog.Result.Resources {
		if resource.Metadata["api_id"] == integration.ID && resource.Metadata["api_developer_asset_publication_id"] != nil {
			resourceURI = resource.URI
			break
		}
	}
	if resourceURI == "" {
		t.Fatal("published API map was not discoverable")
	}
	readBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "resources/read", "params": map[string]any{"uri": resourceURI}})
	read := request(t, handler, http.MethodPost, mcpPath, token, string(readBody))
	var evidence struct {
		Result struct {
			Contents []struct {
				Text     string         `json:"text"`
				Metadata map[string]any `json:"_meta"`
			} `json:"contents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &evidence); err != nil || len(evidence.Result.Contents) != 1 {
		t.Fatalf("exact map read: %s", read.Body.String())
	}
	content := evidence.Result.Contents[0]
	if !strings.Contains(content.Text, integration.DisplayName) {
		t.Fatal("map did not identify selected API")
	}

	endpoint := baseURL + mcpPath
	hash := func(text string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(text))) }
	selectedResources := []map[string]any{}
	readIndex := func(uri string) []any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 40, "method": "resources/read", "params": map[string]any{"uri": uri + "/index"}})
		response := request(t, handler, http.MethodPost, mcpPath, token, string(body))
		var index struct {
			Result struct {
				Contents []struct {
					Metadata map[string]any `json:"_meta"`
				} `json:"contents"`
			} `json:"result"`
		}
		if json.Unmarshal(response.Body.Bytes(), &index) != nil || len(index.Result.Contents) != 1 {
			t.Fatal("compact evidence index unavailable")
		}
		metadata := index.Result.Contents[0].Metadata
		entries, ok := metadata["evidence_resources"].([]any)
		if !ok || metadata["next_uri"] != nil {
			t.Fatal("fixture's bounded evidence index was incomplete")
		}
		return entries
	}
	entries := readIndex(resourceURI)
	if public {
		uri := "dokosoko://developer-assets/global-documentation/" + globalPublication.ID + "/map-v2"
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "resources/read", "params": map[string]any{"uri": uri}})
		response := request(t, handler, http.MethodPost, mcpPath, token, string(body))
		if json.Unmarshal(response.Body.Bytes(), &evidence) != nil || len(evidence.Result.Contents) != 1 {
			t.Fatalf("historical global map: %s", response.Body.String())
		}
		value := evidence.Result.Contents[0]
		globalEntries := readIndex(uri)
		entries = append(entries, globalEntries...)
		selectedResources = append(selectedResources, map[string]any{"uri": uri, "discover": false, "text_sha256": hash(value.Text), "mime_type": "text/markdown", "metadata": map[string]any{"global_documentation_publication_id": globalPublication.ID, "snapshot_hash": globalPublication.SnapshotHash}})
		for _, resource := range catalog.Result.Resources {
			if resource.URI == uri {
				t.Fatal("historical global map unexpectedly advertised as current")
			}
		}
	}
	for _, dependency := range recipe.Dependencies {
		if !strings.HasPrefix(dependency.Kind, "developer_asset_") && dependency.Kind != "product_contract_operation" {
			continue
		}
		parts := strings.Split(dependency.ResourceID, ":")
		var sourceID, entityID string
		if dependency.Kind == "product_contract_operation" && len(parts) == 4 {
			sourceID, entityID = parts[2], parts[3]
		} else if len(parts) == 6 {
			sourceID, entityID = parts[4], parts[5]
		} else {
			t.Fatal("invalid task evidence dependency")
		}
		matches := 0
		for _, raw := range entries {
			entry := raw.(map[string]any)
			if entry["source_publication_id"] != sourceID || entry["source_entity_id"] != entityID {
				continue
			}
			matches++
			if dependency.Kind != "product_contract_operation" && !strings.HasSuffix(dependency.Version, "@"+entry["content_hash"].(string)) {
				t.Fatal("selected SDK hash changed")
			}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "resources/read", "params": map[string]any{"uri": entry["uri"]}})
			result := request(t, handler, http.MethodPost, mcpPath, token, string(body))
			var unit struct {
				Result struct {
					Contents []struct {
						Text string `json:"text"`
					} `json:"contents"`
				} `json:"result"`
			}
			if json.Unmarshal(result.Body.Bytes(), &unit) != nil || len(unit.Result.Contents) != 1 {
				t.Fatalf("SDK evidence read: %s", result.Body.String())
			}
			metadata := map[string]any{"knowledge_unit_id": entry["knowledge_unit_id"], "source_publication_kind": entry["source_publication_kind"], "source_publication_id": entry["source_publication_id"], "source_entity_id": entry["source_entity_id"], "content_hash": entry["content_hash"]}
			global := strings.HasPrefix(dependency.ResourceID, "developer_asset:global_documentation:")
			if !global {
				metadata["api_id"] = integration.ID
			}
			selectedResources = append(selectedResources, map[string]any{"uri": entry["uri"], "discover": false, "text_sha256": hash(unit.Result.Contents[0].Text), "mime_type": "text/markdown", "metadata": metadata})
		}
		if matches != 1 {
			t.Fatalf("SDK dependency matched %d resources", matches)
		}
	}
	makePlan := func() map[string]any {
		return map[string]any{"schema_version": "mcp-task-check-v1", "endpoint": endpoint,
			"task": map[string]any{"title": recipe.Title, "outcome": recipe.Outcome, "resource_uri": recipe.StableURI, "revision_id": recipe.CurrentRevisionID},
			"resources": append([]map[string]any{
				{"uri": recipe.StableURI, "discover": true, "text_sha256": hash(recipe.CurrentRevision.Markdown), "mime_type": "text/markdown", "metadata": map[string]any{"revision_id": recipe.CurrentRevisionID, "integration_ids": []string{integration.ID}}},
				{"uri": resourceURI, "discover": true, "text_sha256": hash(content.Text), "mime_type": "text/markdown", "metadata": map[string]any{"api_id": integration.ID, "api_developer_asset_publication_id": content.Metadata["api_developer_asset_publication_id"], "api_snapshot_hash": content.Metadata["api_snapshot_hash"]}},
			}, selectedResources...),
		}
	}
	parsed, _ := url.Parse(baseURL)
	for _, scenario := range []struct {
		name      string
		change    func(map[string]any)
		anonymous bool
		failed    bool
	}{
		{name: "exact_private_task"},
		{name: "wrong_guidance_bytes", failed: true, change: func(p map[string]any) {
			p["resources"].([]map[string]any)[1]["text_sha256"] = hash("wrong SDK version")
		}},
		{name: "wrong_recipe_revision", failed: true, change: func(p map[string]any) {
			p["task"].(map[string]any)["revision_id"] = "wrong-revision"
			p["resources"].([]map[string]any)[0]["metadata"].(map[string]any)["revision_id"] = "wrong-revision"
		}},
		{name: "wrong_sdk_evidence", failed: true, change: func(p map[string]any) {
			// Clone before changing: other scenario plans reuse the fixture facts.
			resources := p["resources"].([]map[string]any)
			for index, resource := range resources {
				if resource["metadata"].(map[string]any)["source_publication_kind"] != "sdk" {
					continue
				}
				value := map[string]any{}
				for key, item := range resource {
					value[key] = item
				}
				value["text_sha256"] = hash("unreviewed SDK sample")
				resources[index] = value
				return
			}
			t.Fatal("no SDK evidence to change")
		}},
		{name: "wrong_contract_operation", failed: true, change: func(p map[string]any) {
			resources := p["resources"].([]map[string]any)
			for index, resource := range resources {
				if resource["metadata"].(map[string]any)["source_entity_id"] != "f992f836-144e-43fc-af1c-bf0dd27a1f34" {
					continue
				}
				value := map[string]any{}
				for key, item := range resource {
					value[key] = item
				}
				value["text_sha256"] = hash("another contract operation")
				resources[index] = value
				return
			}
			t.Fatal("no contract operation to change")
		}},
		{name: "wrong_global_evidence", failed: true, change: func(p map[string]any) {
			resources := p["resources"].([]map[string]any)
			for index, resource := range resources {
				if resource["metadata"].(map[string]any)["source_publication_kind"] != "documentation_collection" {
					continue
				}
				value := map[string]any{}
				for key, item := range resource {
					value[key] = item
				}
				value["text_sha256"] = hash("guidance from a different global publication")
				resources[index] = value
				return
			}
			t.Fatal("no global evidence to change")
		}},
		{name: "anonymous_private_task", anonymous: true, failed: true},
	} {
		if !public && scenario.name == "wrong_global_evidence" {
			continue
		}
		if public && (scenario.name == "wrong_contract_operation" || scenario.name == "anonymous_private_task") {
			continue
		}
		if public {
			scenario.anonymous = true
			scenario.name = "public_" + scenario.name
			if scenario.name == "public_exact_private_task" {
				scenario.name = "exact_public_task"
			}
		}
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			plan := makePlan()
			if scenario.change != nil {
				scenario.change(plan)
			}
			planJSON, _ := json.Marshal(plan)
			planPath := filepath.Join(directory, "task-plan.json")
			if err := os.WriteFile(planPath, planJSON, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "--endpoint", endpoint, "--allow-loopback-http", parsed.Host, "--task-plan", planPath, "--format", "json", "--token-env", "DOKOSOKO_TASK_TEST_TOKEN", "--restricted-token-env", "DOKOSOKO_TASK_RESTRICTED_TEST_TOKEN"}
			if !scenario.anonymous {
				args = append(args, "--check-unauthenticated")
			}
			command := exec.CommandContext(t.Context(), binary, args...)
			command.Env = append(os.Environ(), "DOKOSOKO_TASK_TEST_TOKEN=", "DOKOSOKO_TASK_RESTRICTED_TEST_TOKEN=")
			if !scenario.anonymous {
				command.Env = append(command.Env, "DOKOSOKO_TASK_TEST_TOKEN=doko_private_demo")
			}
			output, runErr := command.CombinedOutput()
			if scenario.failed {
				exit, ok := runErr.(*exec.ExitError)
				if !ok || exit.ExitCode() != 1 {
					t.Fatalf("expected acceptance failure: %v: %s", runErr, output)
				}
			} else if runErr != nil {
				t.Fatalf("acceptance client: %v: %s", runErr, output)
			}
			var report struct {
				ClientName     string `json:"client_name"`
				ClientVersion  string `json:"client_version"`
				EvidenceOrigin string `json:"evidence_origin"`
				Checks         []struct {
					Name      string `json:"name"`
					RequestID string `json:"request_id"`
					Pages     []struct {
						RequestID string `json:"request_id"`
					} `json:"pages"`
				} `json:"checks"`
				Task struct {
					Retrieval      string `json:"retrieval_status"`
					Implementation string `json:"implementation_status"`
					PlanSHA256     string `json:"plan_sha256"`
				} `json:"task"`
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("invalid report: %s", output)
			}
			if report.ClientName != "DokoSoko MCP acceptance client" || report.ClientVersion != "0.3.0" || report.EvidenceOrigin != "acceptance_client_observation" || report.Task.Implementation != "not_run" || report.Task.PlanSHA256 == "" || (report.Task.Retrieval == "pass") == scenario.failed {
				t.Fatalf("incorrect report: %s", output)
			}
			if paged && !scenario.failed {
				lastPage, taskPage := "", ""
				for _, check := range report.Checks {
					if check.Name == "resources/list" && len(check.Pages) == 2 {
						lastPage = check.Pages[1].RequestID
					}
					if check.Name == "resource expected: "+recipe.StableURI {
						taskPage = check.RequestID
					}
				}
				if lastPage == "" || lastPage != taskPage {
					t.Fatal("client report did not tie later-page task discovery to its actual request")
				}
			}
			if directory := os.Getenv("DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				name := scenario.name
				if paged {
					name = "paged_" + name
				}
				if err := os.WriteFile(filepath.Join(directory, name+".json"), output, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Both mandatory AI stages execute through the normal service, with explicit
// network-free responses. These fixtures are not external model quality evidence.
type taskConnectionAI struct{ knowledge aitest.Knowledge }

func (d *taskConnectionAI) Do(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	if bytes.Contains(body, []byte("Knowledge processing contract:")) {
		return d.knowledge.Do(request)
	}
	return (testutil.RecipeAI{}).Do(request)
}

func prepareTaskConnectionSDK(t *testing.T, service *platform.Service, apiID string, visibility model.Visibility, actor platform.Actor) {
	t.Helper()
	ctx := t.Context()
	pkg, err := service.SaveSDKPackage(ctx, "", platform.SDKPackageInput{Ecosystem: "npm", Coordinate: "@acme/orders-task", Name: "Orders SDK", Visibility: visibility, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, pkg.ID, platform.SDKReleaseInput{ExactVersion: "1.2.3", SourceURL: "https://example.test/orders-sdk", Visibility: visibility}, actor)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := service.IngestSDKReleaseContent(ctx, release.ID, platform.SDKContentIngestionInput{Files: []platform.SDKIngestionFile{{SourcePath: "README.md", Content: "# Orders status SDK\n\nUse Orders SDK 1.2.3 to read one order status.\n\n```typescript\nconst order = await client.orders.getStatus(orderId);\n```\n"}}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	for range 16 {
		status, err := service.ProcessKnowledgeBatch(ctx, imported.Run.ID, actor)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "ready" {
			break
		}
	}
	input := platform.SDKContentCandidatePublicationInput{AcknowledgeReviewed: true}
	for _, file := range imported.Candidate.Files {
		input.Files = append(input.Files, platform.DeveloperAssetReviewDecision{ID: file.ID, Decision: "included"})
	}
	for _, sample := range imported.Candidate.Samples {
		input.Samples = append(input.Samples, platform.DeveloperAssetReviewDecision{ID: sample.ID, Decision: "approved", ReviewEvidence: json.RawMessage(`{"summary":"Fixture reviewer checked the literal order-status example; no compilation or application test was run."}`)})
	}
	publication, err := service.PublishSDKContentCandidate(ctx, release.ID, imported.Candidate.Candidate.ID, input, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAPISDKBinding(ctx, apiID, "", platform.APISDKBindingInput{SDKPackageID: pkg.ID, SDKReleaseID: release.ID, SDKContentPublicationID: publication.ID, State: "ready", Visibility: visibility}, actor); err != nil {
		t.Fatal(err)
	}
}

func prepareTaskConnectionContract(t *testing.T, memory *store.Memory, service *platform.Service, apiID string, actor platform.Actor) {
	t.Helper()
	ctx := t.Context()
	contract, err := service.SaveAPIContract(ctx, "", platform.APIContractInput{Name: "Orders contract", Slug: "task-orders-contract", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	const candidateID = "f992f836-144e-43fc-af1c-bf0dd27a1f32"
	const runID = "f992f836-144e-43fc-af1c-bf0dd27a1f33"
	const operationID = "f992f836-144e-43fc-af1c-bf0dd27a1f34"
	const mapID = "f992f836-144e-43fc-af1c-bf0dd27a1f35"
	const revisionID = "f992f836-144e-43fc-af1c-bf0dd27a1f36"
	if _, err = memory.SaveAPIContractSource(ctx, model.APIContractSource{ID: "f992f836-144e-43fc-af1c-bf0dd27a1f37", DeploymentID: "prod_acme", APIContractID: contract.ID, SourceID: "src_api", SourceRole: "primary", Lifecycle: "attached", CreatedBy: actor.ID}, 0); err != nil {
		t.Fatalf("attach fixture source: %v", err)
	}
	now := time.Now().UTC()
	if _, err = memory.CreateDeveloperAssetIngestionRun(ctx, model.DeveloperAssetIngestionRun{ID: runID, DeploymentID: "prod_acme", OrganisationID: "org_acme", AssetKind: model.DeveloperAssetContract, TargetID: contract.ID, TargetKey: "contract:" + contract.ID, SourceID: "src_api", State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, AcquiredCount: 1, StartedAt: &now, FinishedAt: &now, Versions: model.ProcessorVersions{Pipeline: "fixture", Parser: "fixture", Normalizer: "fixture", Mapper: "fixture"}, RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("create ingestion: %v", err)
	}
	hash := mcpAssetHash("task-orders-contract")
	if _, err = memory.CreateAPIContractCandidate(ctx, store.APIContractCandidateRecord{
		Candidate:  model.APIContractCandidate{ID: candidateID, DeploymentID: "prod_acme", APIContractID: contract.ID, IngestionRunID: runID, OpenAPIVersion: "3.1.0", SourceFormat: "json", NormalizedContract: json.RawMessage(`{"openapi":"3.1.0","info":{"title":"Orders","version":"v1"},"paths":{"/orders/{order_id}":{"get":{"operationId":"readOrderStatus","responses":{"200":{"description":"Order status"}}}}}}`), SourceHash: hash, ContentHash: hash, ValidationResult: json.RawMessage(`{"valid":true,"errors":[]}`), ParserVersion: "fixture", Visibility: model.VisibilityPrivate, Diagnostics: json.RawMessage(`{}`)},
		Operations: []model.APIContractOperation{{ID: operationID, APIContractCandidateID: candidateID, OperationKey: "GET /orders/{order_id}", OperationID: "readOrderStatus", Method: "GET", PathTemplate: "/orders/{order_id}", Summary: "Read an order status", Description: "Return one order status.", Security: json.RawMessage(`[]`), RequestSchemaRefs: []string{}, ResponseSchemaRefs: []string{}, ContentHash: mcpAssetHash("task-order-operation")}},
		Map:        &model.APIContractMap{ID: mapID, DeploymentID: "prod_acme", APIContractCandidateID: candidateID, MapVersion: "contract-map-v1", Map: model.ContractMapBody{Overview: "Read an order status."}, AgentMarkdown: "# Orders contract\n\nRead one order status.", ContentHash: mcpAssetHash("task-order-contract-map")},
	}); err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	status, err := service.ProcessKnowledgeBatch(ctx, runID, actor)
	if err != nil || status.State != "ready" {
		t.Fatalf("contract fixture AI processing: %#v %v", status, err)
	}
	// Seed the reviewed historical revision directly. Source acquisition is not
	// under test here; recipe generation, index construction and reads are real.
	source, err := memory.SourcePublication(ctx, "prod_acme", "pub_api_seed")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	_, revision, err := memory.PublishAPIContractCandidate(ctx, contract, contract.Revision, model.APIContractRevision{ID: revisionID, DeploymentID: "prod_acme", APIContractID: contract.ID, APIContractCandidateID: candidateID, ContentHash: hash, Visibility: model.VisibilityPrivate, ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now}, &model.APIContractRevisionSourcePublication{APIContractRevisionID: revisionID, DeploymentID: "prod_acme", APIContractCandidateID: candidateID, SourcePublicationID: source.ID, ContentHash: source.ContentHash})
	if err != nil {
		t.Fatalf("publish revision: %v", err)
	}
	if _, err = service.SaveAPIContractBinding(ctx, apiID, "", platform.APIContractBindingInput{APIContractID: contract.ID, PinnedRevisionID: revision.ID, Primary: true, Visibility: model.VisibilityPrivate}, actor); err != nil {
		t.Fatalf("attach contract: %v", err)
	}
}

// These collections are reviewed historical fixture inputs. Source acquisition
// is not under test; global publication, activation and recipe grounding are.
func prepareTaskGlobalDocumentation(t *testing.T, memory *store.Memory, service *platform.Service, number int, actor platform.Actor) model.DeploymentDocumentationPublication {
	t.Helper()
	ctx := t.Context()
	id := func(offset int) string { return fmt.Sprintf("e5c3c682-26ea-4c99-80ee-%012d", 100+number*10+offset) }
	title, body := "Order authentication", "Authenticate the application before reading an order status with Orders SDK 1.2.3."
	if number == 2 {
		title, body = "Shipping information", "Current unrelated guidance for shipment labels."
	}
	now := time.Now().UTC()
	_, err := memory.CreateDocumentationCollection(ctx, model.DocumentationCollection{ID: id(0), DeploymentID: "prod_acme", OrganisationID: "org_acme", Name: title, Slug: fmt.Sprintf("task-global-%d", number), Visibility: model.VisibilityPublic, Lifecycle: "active"}, store.DocumentationCollectionRevisionRecord{
		Revision: model.DocumentationCollectionRevision{ID: id(1), DeploymentID: "prod_acme", DocumentationCollectionID: id(0), Revision: 1, Visibility: model.VisibilityPublic, ContentHash: mcpAssetHash(body), SelectionManifest: json.RawMessage(`[]`), ReviewedBy: actor.ID, ReviewedAt: now, PublishedAt: now},
		Map:      &model.DocumentationMap{ID: id(2), DeploymentID: "prod_acme", DocumentationCollectionRevisionID: id(1), MapVersion: "documentation-map-v1", Map: model.DocumentationMapBody{Overview: body}, AgentMarkdown: "# " + title + "\n\n" + body + "\n", ContentHash: mcpAssetHash("map:" + body), Visibility: model.VisibilityPublic},
	})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := service.PublishDeploymentDocumentation(ctx, platform.DeploymentDocumentationPublicationInput{CollectionRevisionIDs: []string{id(1)}, Visibility: model.VisibilityPublic, ExpectedHeadRevision: int64(number - 1), AcknowledgeReviewed: true}, actor)
	if err != nil {
		t.Fatal(err)
	}
	return publication
}
