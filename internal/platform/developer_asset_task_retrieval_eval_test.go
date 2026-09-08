package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type publishedTaskRetrievalCorpus struct {
	Version                 string `json:"version"`
	RetrievalProfileVersion string `json:"retrieval_profile_version"`
	Documents               []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"documents"`
	Cases []struct {
		Name            string   `json:"name"`
		Query           string   `json:"query"`
		ExpectedPaths   []string `json:"expected_paths"`
		RequiredPhrases []string `json:"required_phrases"`
	} `json:"cases"`
}

type taskRetrievalRelease struct {
	release     model.SDKRelease
	publication model.SDKContentPublication
	candidate   store.SDKContentCandidateRecord
}

// This exercises ingestion, mandatory AI processing (an explicit local fixture),
// human review, publication, index activation, store retrieval, reranking, context
// selection and durable traces. It does not execute SDK code or test an agent.
func TestPublishedTaskRetrievalEvaluation(t *testing.T) {
	for _, backendName := range []string{"memory", "postgres"} {
		t.Run(backendName, func(t *testing.T) {
			var backend store.Store = store.NewMemory()
			var pool *pgxpool.Pool
			if backendName == "postgres" {
				pool, backend = knowledgePostgresFixtureWithPool(t)
			}
			runPublishedTaskRetrievalEvaluation(t, backendName, backend, pool)
		})
	}
}

func runPublishedTaskRetrievalEvaluation(t *testing.T, backendName string, backend store.Store, pool *pgxpool.Pool) {
	t.Helper()
	raw, err := os.ReadFile("testdata/published-task-retrieval-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus publishedTaskRetrievalCorpus
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "published-task-retrieval-v1" || corpus.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion || len(corpus.Cases) != 5 || len(corpus.Documents) < 10 {
		t.Fatal("invalid task retrieval corpus metadata")
	}
	paths := map[string]bool{}
	for _, doc := range corpus.Documents {
		if doc.Path == "" || doc.Content == "" || paths[doc.Path] {
			t.Fatal("empty or duplicate corpus document")
		}
		paths[doc.Path] = true
	}
	for _, scenario := range corpus.Cases {
		if scenario.Name == "" || scenario.Query == "" || len(scenario.ExpectedPaths) == 0 || len(scenario.RequiredPhrases) == 0 {
			t.Fatal("incomplete evaluation scenario")
		}
		for _, path := range scenario.ExpectedPaths {
			if !paths[path] {
				t.Fatalf("unknown expected source: %s", path)
			}
		}
	}
	ctx := t.Context()
	provider := &aitest.Knowledge{}
	service := configuredKnowledgeService(t, backend, provider)
	actor := Actor{ID: "task-retrieval-fixture"}
	api, err := service.CreateIntegration(ctx, IntegrationInput{FamilyKey: "orders-retrieval", VersionKey: "v1", DisplayName: "Orders retrieval fixture", Visibility: model.VisibilityPublic, AcknowledgePublic: true, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := service.SaveSDKPackage(ctx, "", SDKPackageInput{Ecosystem: "npm", Coordinate: "@fixture/orders-retrieval", Name: "Orders SDK fixture", Visibility: model.VisibilityPublic, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	current := publishTaskRetrievalRelease(t, service, pkg, corpus, "1.2.3", "", true, actor)
	old := publishTaskRetrievalRelease(t, service, pkg, corpus, "0.9.0", "OBSOLETE_PROTOCOL_EVIDENCE: use X-Legacy-Key and /v0/orders instead.", true, actor)
	draft := publishTaskRetrievalRelease(t, service, pkg, corpus, "2.0.0", "UNPUBLISHED_PROTOCOL_EVIDENCE: use /v2/orders instead.", false, actor)
	binding, err := service.SaveAPISDKBinding(ctx, api.ID, "", APISDKBindingInput{SDKPackageID: pkg.ID, SDKReleaseID: old.release.ID, SDKContentPublicationID: old.publication.ID, State: "ready", Visibility: model.VisibilityPublic}, actor)
	if err != nil {
		t.Fatalf("attach old release: %v", err)
	}
	if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatalf("publish old API: %v", err)
	}
	oldPublication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pool != nil {
		insertSDKHistoryAdvisory(t, pool, oldPublication, old, binding.ID, "before-upgrade")
	}
	if _, err = service.SaveAPISDKBinding(ctx, api.ID, binding.ID, APISDKBindingInput{SDKPackageID: pkg.ID, SDKReleaseID: current.release.ID, SDKContentPublicationID: current.publication.ID, State: "ready", Visibility: model.VisibilityPublic, Revision: binding.Revision}, actor); err != nil {
		t.Fatalf("update exact release: %v", err)
	}
	if _, err = service.PublishIntegration(ctx, api.ID, actor); err != nil {
		t.Fatalf("publish selected API: %v", err)
	}
	publication, err := service.ReadyAPIDeveloperAssetPublication(ctx, api.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pool != nil {
		t.Run("sdk-history-database-guards", func(t *testing.T) {
			assertSDKBindingHistoryGuards(t, pool, oldPublication, publication, old, binding.ID)
		})
	}
	generation, err := service.BuildDeveloperAssetSearchIndex(ctx, "api", publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A second ready API contains a near-identical corpus. Its evidence must
	// never become a candidate for the selected API, even with the same version.
	otherAPI, err := service.CreateIntegration(ctx, IntegrationInput{FamilyKey: "other-retrieval", VersionKey: "v1", DisplayName: "Other private API fixture", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	otherPackage, err := service.SaveSDKPackage(ctx, "", SDKPackageInput{Ecosystem: "npm", Coordinate: "@fixture/other-retrieval", Name: "Other private SDK fixture", Visibility: model.VisibilityPrivate, Lifecycle: "active"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	other := publishTaskRetrievalRelease(t, service, otherPackage, corpus, "1.2.3", "OTHER_API_PRIVATE_EVIDENCE: private account instructions.", true, actor)
	if _, err = service.SaveAPISDKBinding(ctx, otherAPI.ID, "", APISDKBindingInput{SDKPackageID: otherPackage.ID, SDKReleaseID: other.release.ID, SDKContentPublicationID: other.publication.ID, State: "ready", Visibility: model.VisibilityPrivate}, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.PublishIntegration(ctx, otherAPI.ID, actor); err != nil {
		t.Fatalf("publish other API: %v", err)
	}
	otherPublication, err := service.ReadyAPIDeveloperAssetPublication(ctx, otherAPI.ID)
	if err != nil {
		t.Fatal(err)
	}
	if provider.Calls.Load() == 0 {
		t.Fatal("mandatory AI stage was not executed")
	}
	input := DeveloperAssetQueryLabInput{Scope: "api", APIID: api.ID, APIDeveloperAssetPublicationID: publication.ID, AssetKinds: []string{"sdk"}, Ecosystems: []string{"npm"}, ExactVersions: []string{"1.2.3"}, Limit: 5, ContextTokenLimit: 2000}
	rows := []map[string]any{}
	t.Run("historical-release-stays-retrievable", func(t *testing.T) {
		query := input
		query.Query, query.APIDeveloperAssetPublicationID = corpus.Cases[0].Query, oldPublication.ID
		query.ExactVersions = []string{old.release.ExactVersion}
		result, err := service.RunDeveloperAssetQueryLab(ctx, query)
		if err != nil || len(result.Results) == 0 {
			t.Fatalf("historical publication unavailable: %v", err)
		}
		oldGeneration, err := service.BuildDeveloperAssetSearchIndex(ctx, "api", oldPublication.ID)
		if err != nil {
			t.Fatal(err)
		}
		foundMarker := false
		for _, hit := range result.Results {
			assertTaskRetrievalCitation(t, hit.Unit, old, oldPublication, oldGeneration)
			foundMarker = foundMarker || strings.Contains(hit.Excerpt, "OBSOLETE_PROTOCOL_EVIDENCE")
		}
		if !foundMarker {
			t.Fatal("historical evidence silently replaced with current content")
		}
		rows = append(rows, map[string]any{"name": "historical-release-stays-retrievable", "api_publication_id": oldPublication.ID, "sdk_exact_version": old.release.ExactVersion, "trace_id": result.TraceID, "passed": !t.Failed()})
	})
	for _, scenario := range corpus.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			query := input
			query.Query = scenario.Query
			started := time.Now()
			result, err := service.RunDeveloperAssetQueryLab(ctx, query)
			latency := time.Since(started)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Results) > 5 || result.ContextTokens > query.ContextTokenLimit {
				t.Fatal("result or context budget exceeded")
			}
			found := map[string]bool{}
			relevant, excerpts := 0, ""
			returned := []map[string]any{}
			for _, hit := range result.Results {
				path := assertTaskRetrievalCitation(t, hit.Unit, current, publication, generation)
				if slices.Contains(scenario.ExpectedPaths, path) {
					found[path], relevant = true, relevant+1
					excerpts += "\n" + hit.Excerpt
				}
				returned = append(returned, map[string]any{"rank": hit.Rank, "source_path": path, "unit_id": hit.Unit.ID, "source_entity_id": hit.Unit.SourceEntityID, "content_hash": hit.Unit.ContentHash})
			}
			recall := float64(len(found)) / float64(len(scenario.ExpectedPaths))
			precision := float64(relevant) / float64(max(1, len(result.Results)))
			if recall < 0.9 {
				t.Errorf("recall@5=%.2f; expected %v, found %v; results=%v", recall, scenario.ExpectedPaths, found, returned)
			}
			for _, phrase := range scenario.RequiredPhrases {
				if !strings.Contains(excerpts, phrase) {
					t.Errorf("selected relevant excerpts omit required task evidence %q", phrase)
				}
			}
			trace, err := backend.RetrievalQueryTrace(ctx, api.DeploymentID, result.TraceID)
			if err != nil || trace.Trace.ResultCount != len(result.Results) || trace.Trace.ContextTokens != result.ContextTokens {
				t.Fatalf("trace does not match selection: %v", err)
			}
			// Check the entire candidate trace, including evidence excluded by
			// rank/context limits; a top-five-only check can hide scope leaks.
			for _, candidate := range trace.Results {
				if candidate.SourcePublicationID != current.publication.ID || candidate.SourcePublicationKind != "sdk" {
					t.Fatal("forbidden version or API reached candidate trace")
				}
			}
			rows = append(rows, map[string]any{"name": scenario.Name, "query": scenario.Query, "expected_paths": scenario.ExpectedPaths, "required_phrases": scenario.RequiredPhrases, "recall_at_5": recall, "precision_at_5": precision, "context_tokens": result.ContextTokens, "latency_ms": float64(latency.Microseconds()) / 1000, "trace_id": result.TraceID, "results": returned, "passed": !t.Failed()})
			t.Logf("recall@5=%.2f precision@5=%.2f context_tokens=%d", recall, precision, result.ContextTokens)
		})
	}
	for _, scenario := range []struct {
		name string
		edit func(*DeveloperAssetQueryLabInput)
	}{
		{"unpublished-version", func(q *DeveloperAssetQueryLabInput) { q.ExactVersions = []string{draft.release.ExactVersion} }},
		{"missing-version", func(q *DeveloperAssetQueryLabInput) { q.ExactVersions = []string{"99.0.0"} }},
		{"other-api-release", func(q *DeveloperAssetQueryLabInput) { q.SDKReleaseIDs = []string{other.release.ID} }},
		{"contradictory-version-and-release", func(q *DeveloperAssetQueryLabInput) { q.SDKReleaseIDs = []string{old.release.ID} }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			query := input
			query.Query = corpus.Cases[0].Query
			scenario.edit(&query)
			result, err := service.RunDeveloperAssetQueryLab(ctx, query)
			if err != nil || len(result.Results) != 0 || result.ContextTokens != 0 {
				t.Fatalf("unavailable exact evidence returned results: %d, %v", len(result.Results), err)
			}
			trace, err := backend.RetrievalQueryTrace(ctx, api.DeploymentID, result.TraceID)
			if err != nil || len(trace.Results) != 0 {
				t.Fatalf("forbidden evidence reached no-answer trace: %v", err)
			}
			rows = append(rows, map[string]any{"name": scenario.name, "expected_result_count": 0, "result_count": len(result.Results), "trace_id": result.TraceID, "passed": !t.Failed()})
		})
	}
	t.Run("cross-api-publication", func(t *testing.T) {
		query := input
		query.Query, query.APIDeveloperAssetPublicationID = corpus.Cases[0].Query, otherPublication.ID
		if _, err := service.RunDeveloperAssetQueryLab(ctx, query); err == nil || !strings.Contains(err.Error(), "does not belong") {
			t.Fatalf("cross-API publication not rejected: %v", err)
		}
		rows = append(rows, map[string]any{"name": "cross-api-publication", "expected_error": "publication does not belong to API", "passed": !t.Failed()})
	})
	report := map[string]any{
		"suite_version": corpus.Version, "corpus_sha256": contentHash(raw), "backend": backendName,
		"observed_at": time.Now().UTC(), "evidence_origin": "synthetic_published_retrieval_fixture",
		"external_ai_quality": "not_measured", "application_outcome": "not_run", "existing_documentation_baseline": "not_measured",
		"api_publication_id": publication.ID, "sdk_release_id": current.release.ID, "sdk_exact_version": current.release.ExactVersion,
		"sdk_publication_id": current.publication.ID, "sdk_candidate_hash": current.candidate.Candidate.ContentHash,
		"processor_versions": current.candidate.Candidate.Versions, "map_version": current.candidate.Candidate.MapVersion,
		"index_generation": generation, "required_recall_at_5": 0.9, "context_token_limit": input.ContextTokenLimit,
		"cases": rows, "passed": !t.Failed(),
	}
	if directory := os.Getenv("DOKOSOKO_ACCEPTANCE_EVIDENCE_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "published-task-retrieval-"+backendName+".json"), append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func publishTaskRetrievalRelease(t *testing.T, service *Service, pkg model.SDKPackage, corpus publishedTaskRetrievalCorpus, version, marker string, publish bool, actor Actor) taskRetrievalRelease {
	t.Helper()
	ctx := t.Context()
	release, err := service.CreateSDKRelease(ctx, pkg.ID, SDKReleaseInput{ExactVersion: version, SourceURL: "https://fixture.example.test/orders-sdk", Visibility: pkg.Visibility}, actor)
	if err != nil {
		t.Fatalf("create %s release: %v", version, err)
	}
	files := []SDKIngestionFile{}
	for _, doc := range corpus.Documents {
		content := strings.ReplaceAll(doc.Content, "1.2.3", version)
		if marker != "" {
			content += "\n\n" + marker
		}
		files = append(files, SDKIngestionFile{SourcePath: doc.Path, Content: content})
	}
	imported, err := service.IngestSDKReleaseContent(ctx, release.ID, SDKContentIngestionInput{Files: files}, actor)
	if err != nil {
		t.Fatalf("ingest %s release: %v", version, err)
	}
	value := taskRetrievalRelease{release: release, candidate: imported.Candidate}
	if !publish {
		return value
	}
	ready := false
	for range 16 {
		status, err := service.ProcessKnowledgeBatch(ctx, imported.Run.ID, actor)
		if err != nil {
			t.Fatalf("process %s release: %v", version, err)
		}
		if status.State == "ready" {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("required AI processing did not finish")
	}
	input := SDKContentCandidatePublicationInput{AcknowledgeReviewed: true}
	for _, file := range imported.Candidate.Files {
		input.Files = append(input.Files, DeveloperAssetReviewDecision{ID: file.ID, Decision: "included"})
	}
	if len(imported.Candidate.Samples) != 0 {
		t.Fatal("retrieval corpus must not imply executed SDK samples")
	}
	value.publication, err = service.PublishSDKContentCandidate(ctx, release.ID, imported.Candidate.Candidate.ID, input, actor)
	if err != nil {
		t.Fatalf("publish %s release: %v", version, err)
	}
	return value
}

func assertTaskRetrievalCitation(t *testing.T, unit model.KnowledgeUnit, release taskRetrievalRelease, publication model.APIDeveloperAssetPublication, generation model.SearchIndexGeneration) string {
	t.Helper()
	if unit.SourcePublicationKind != "sdk" || unit.SourcePublicationID != release.publication.ID || unit.SearchIndexGenerationID != generation.ID || unit.Visibility != model.VisibilityPublic {
		t.Fatalf("wrong exact publication or visibility: %#v", unit)
	}
	var citation map[string]any
	if err := json.Unmarshal(unit.Citation, &citation); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"publication_id": release.publication.ID, "index_publication_id": publication.ID, "sdk_release_id": release.release.ID, "exact_version": release.release.ExactVersion, "content_hash": unit.ContentHash} {
		if citation[field] != want {
			t.Fatalf("citation %s=%v, want %s", field, citation[field], want)
		}
	}
	if unit.Kind == "map" {
		return ""
	}
	path, _ := citation["source_path"].(string)
	for _, section := range release.candidate.Sections {
		if section.ID == unit.SourceEntityID {
			if unit.Kind != "sdk_section" || section.ContentHash != unit.ContentHash || strings.TrimSpace(section.NormalizedText) != unit.Content || citation["sdk_section_id"] != section.ID {
				t.Fatal("retrieved text/hash no longer agrees with ingested source section")
			}
			for _, file := range release.candidate.Files {
				if file.ID == section.SDKPublicationFileID && path == file.SourcePath && citation["sdk_publication_file_id"] == file.ID {
					return path
				}
			}
		}
	}
	t.Fatal("citation does not resolve to the reviewed source file and section")
	return ""
}
