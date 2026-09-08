package platform

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

// The subprocess executes only the fixed checked-in reference SDK/application.
// Imported evidence is compared with those bytes; it is never an executable
// path, command, module, endpoint or program supplied to the subprocess.
func TestPublishedTaskApplicationEvaluation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js 22.13+ is required for the application evaluation fixture")
	}
	ctx := t.Context()
	backend := store.NewMemory()
	provider := &aitest.Knowledge{}
	service := configuredKnowledgeService(t, backend, provider)
	actor := Actor{ID: "application-evaluation-reviewer"}
	rawCorpus, err := os.ReadFile("testdata/published-task-retrieval-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus publishedTaskRetrievalCorpus
	if err := json.Unmarshal(rawCorpus, &corpus); err != nil {
		t.Fatal(err)
	}
	applicationPath := filepath.Join("..", "..", "examples", "integration-evaluation")
	packageBytes, err := os.ReadFile(filepath.Join(applicationPath, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Name, Version string }
	if err := json.Unmarshal(packageBytes, &manifest); err != nil || manifest.Name != "@fixture/orders-retrieval" || manifest.Version != "1.2.3" {
		t.Fatalf("unexpected fixture package identity: %v", err)
	}
	api, err := service.CreateIntegration(ctx, IntegrationInput{FamilyKey: "orders-application", VersionKey: "v1", DisplayName: "Orders application evaluation", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := service.SaveSDKPackage(ctx, "", SDKPackageInput{Ecosystem: "npm", Coordinate: manifest.Name, Name: "Orders application SDK fixture", Visibility: model.VisibilityPublic, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(ctx, pkg.ID, SDKReleaseInput{ExactVersion: manifest.Version, SourceURL: "https://fixture.example.test/orders", Visibility: model.VisibilityPublic}, actor)
	if err != nil {
		t.Fatal(err)
	}
	files := []SDKIngestionFile{}
	for _, doc := range corpus.Documents {
		files = append(files, SDKIngestionFile{SourcePath: doc.Path, Content: doc.Content})
	}
	sourceHashes := map[string]string{}
	for _, name := range []string{"package.json", "orders-sdk.js", "webhook-app.js"} {
		body, err := os.ReadFile(filepath.Join(applicationPath, name))
		if err != nil {
			t.Fatal(err)
		}
		sourceHashes[name] = contentHash(body)
		files = append(files, SDKIngestionFile{SourcePath: name, Content: string(body)})
	}
	imported, err := service.IngestSDKReleaseContent(ctx, release.ID, SDKContentIngestionInput{Files: files}, actor)
	if err != nil {
		t.Fatal(err)
	}
	ready := false
	for range 32 {
		status, err := service.ProcessKnowledgeBatch(ctx, imported.Run.ID, actor)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "ready" {
			ready = true
			break
		}
	}
	if !ready || provider.Calls.Load() == 0 {
		t.Fatal("required AI processing did not execute and finish")
	}
	review := SDKContentCandidatePublicationInput{AcknowledgeReviewed: true}
	for _, file := range imported.Candidate.Files {
		review.Files = append(review.Files, DeveloperAssetReviewDecision{ID: file.ID, Decision: "included"})
	}
	for _, sample := range imported.Candidate.Samples {
		review.Samples = append(review.Samples, DeveloperAssetReviewDecision{ID: sample.ID, Decision: "approved", ReviewEvidence: json.RawMessage(`{"summary":"Fixture reviewer inspected the fixed local example. Application tests have not yet run; no compatibility or machine-validation claim is made."}`)})
	}
	sdkPublication, err := service.PublishSDKContentCandidate(ctx, release.ID, imported.Candidate.Candidate.ID, review, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveAPISDKBinding(ctx, api.ID, "", APISDKBindingInput{SDKPackageID: pkg.ID, SDKReleaseID: release.ID, SDKContentPublicationID: sdkPublication.ID, Visibility: model.VisibilityPublic, State: "ready"}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatal(err)
	}
	publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedPublication, err := backend.SDKContentPublication(ctx, api.DeploymentID, sdkPublication.ID)
	if err != nil {
		t.Fatal(err)
	}
	publishedFiles := []map[string]any{}
	for _, file := range imported.Candidate.Files {
		if want, ok := sourceHashes[file.SourcePath]; ok {
			if file.ContentHash != want || contentHash([]byte(file.NormalizedContent)) != want {
				t.Fatalf("published normalization differs from executable fixture bytes: %s", file.SourcePath)
			}
			selected := false
			for _, selection := range storedPublication.FileSelections {
				selected = selected || (selection.SDKPublicationFileID == file.ID && selection.Decision == "included" && selection.ContentHash == want)
			}
			if !selected {
				t.Fatal("executed source is not in the reviewed SDK publication")
			}
			publishedFiles = append(publishedFiles, map[string]any{"path": file.SourcePath, "file_id": file.ID, "content_hash": want})
		}
	}
	tasks := []map[string]any{}
	for _, scenario := range corpus.Cases {
		result, err := service.RunDeveloperAssetQueryLab(ctx, DeveloperAssetQueryLabInput{Scope: "api", APIID: api.ID, APIDeveloperAssetPublicationID: publication.ID, SDKReleaseIDs: []string{release.ID}, ExactVersions: []string{release.ExactVersion}, AssetKinds: []string{"sdk"}, Query: scenario.Query, Limit: 5, ContextTokenLimit: 2000})
		if err != nil {
			t.Fatal(err)
		}
		evidence := []map[string]any{}
		for _, hit := range result.Results {
			var citation map[string]any
			if err := json.Unmarshal(hit.Unit.Citation, &citation); err != nil {
				t.Fatal(err)
			}
			path, _ := citation["source_path"].(string)
			if !slices.Contains(scenario.ExpectedPaths, path) {
				continue
			}
			if hit.Unit.SourcePublicationID != sdkPublication.ID || citation["index_publication_id"] != publication.ID || citation["exact_version"] != release.ExactVersion {
				t.Fatal("application guidance has wrong publication identity")
			}
			evidence = append(evidence, map[string]any{"path": path, "text": hit.Excerpt, "text_sha256": contentHash([]byte(hit.Excerpt)), "content_hash": hit.Unit.ContentHash, "source_entity_id": hit.Unit.SourceEntityID, "publication_id": hit.Unit.SourcePublicationID, "api_publication_id": publication.ID, "exact_version": release.ExactVersion})
		}
		tasks = append(tasks, map[string]any{"name": scenario.Name, "trace_id": result.TraceID, "evidence": evidence})
	}
	input := map[string]any{
		"schema_version": "published-application-fixture-v1", "corpus_sha256": contentHash(rawCorpus),
		"api_publication_id": publication.ID, "api_snapshot_hash": publication.SnapshotHash,
		"sdk": map[string]any{"coordinate": pkg.CanonicalCoordinate, "exact_version": release.ExactVersion,
			"release_id": release.ID, "release_hash": release.ReleaseHash, "publication_id": sdkPublication.ID,
			"candidate_hash": imported.Candidate.Candidate.ContentHash, "processor_versions": imported.Candidate.Candidate.Versions, "files": publishedFiles},
		"tasks": tasks,
	}
	rawInput, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name  string
		edit  func(map[string]any)
		error string
	}{
		{name: "exact-published-application"},
		{name: "wrong-version", edit: func(input map[string]any) { input["sdk"].(map[string]any)["exact_version"] = "0.9.0" }, error: "sdk_version_mismatch"},
		{name: "changed-sdk-source", edit: func(input map[string]any) {
			input["sdk"].(map[string]any)["files"].([]any)[0].(map[string]any)["content_hash"] = "sha256:" + strings.Repeat("0", 64)
		}, error: "published_source_mismatch"},
		{name: "missing-task", edit: func(input map[string]any) { input["tasks"] = input["tasks"].([]any)[:4] }, error: "incomplete_task_evidence"},
		{name: "missing-guidance", edit: func(input map[string]any) { input["tasks"].([]any)[0].(map[string]any)["evidence"] = []any{} }, error: "missing_task_guidance"},
		{name: "changed-guidance", edit: func(input map[string]any) {
			input["tasks"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["text"] = "changed"
		}, error: "changed_task_evidence"},
		{name: "cross-publication-guidance", edit: func(input map[string]any) {
			input["tasks"].([]any)[0].(map[string]any)["evidence"].([]any)[0].(map[string]any)["publication_id"] = "other-publication"
		}, error: "changed_task_evidence"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			body := rawInput
			if scenario.edit != nil {
				var changed map[string]any
				if err := json.Unmarshal(rawInput, &changed); err != nil {
					t.Fatal(err)
				}
				scenario.edit(changed)
				body, err = json.Marshal(changed)
				if err != nil {
					t.Fatal(err)
				}
			}
			file := filepath.Join(t.TempDir(), "evidence.json")
			if err := os.WriteFile(file, body, 0600); err != nil {
				t.Fatal(err)
			}
			processCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(processCtx, node, filepath.Join(applicationPath, "check.mjs"), file)
			output, runErr := command.CombinedOutput()
			var report struct {
				Status        string `json:"application_status"`
				Error         string `json:"error"`
				Origin        string `json:"evidence_origin"`
				EvidenceHash  string `json:"evidence_sha256"`
				APIID         string `json:"api_publication_id"`
				CodingClient  string `json:"coding_client_implementation"`
				Documentation string `json:"existing_documentation_comparison"`
				Runtime       struct{ Name, Version string }
				Cases         []struct{ Task, Name, Status string }
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("invalid application report: %v\n%s", err, output)
			}
			if scenario.error != "" {
				if exit, ok := runErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 || report.Status != "not_run" || report.Error != scenario.error || len(report.Cases) != 0 {
					t.Fatalf("invalid evidence was executed or wrongly reported: %v\n%s", runErr, output)
				}
			} else {
				if runErr != nil || report.Status != "pass" || len(report.Cases) != 17 || report.Origin != "local_reference_application_execution" || report.EvidenceHash != contentHash(body) || report.APIID != publication.ID || report.Runtime.Name != "Node.js" || report.Runtime.Version == "" || report.CodingClient != "not_run" || report.Documentation != "not_measured" {
					t.Fatalf("application checks failed or overstated evidence: %v\n%s", runErr, output)
				}
				covered := map[string]bool{}
				for _, check := range report.Cases {
					if check.Status != "pass" {
						t.Fatalf("application case %s failed", check.Name)
					}
					covered[check.Task] = true
				}
				for _, task := range corpus.Cases {
					if !covered[task.Name] {
						t.Fatalf("application task %s was not tested", task.Name)
					}
				}
				t.Logf("17 application checks passed across five tasks with %s %s", report.Runtime.Name, report.Runtime.Version)
			}
			if directory := os.Getenv("DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				for name, value := range map[string][]byte{scenario.name + "-input.json": body, scenario.name + "-report.json": output} {
					if err := os.WriteFile(filepath.Join(directory, name), value, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
	bindings, err := backend.APISDKBindings(ctx, api.DeploymentID, api.ID)
	if err != nil || len(bindings) != 1 || bindings[0].Assurance != model.SDKAssuranceRelated || bindings[0].CompatibilityAssertionID != "" {
		t.Fatal("application fixture changed the publication's compatibility claim")
	}
}
