package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func configuredKnowledgeService(t *testing.T, backend store.Store, provider *aitest.Knowledge) *Service {
	t.Helper()
	s := NewWithVaultAndProductBuilderDoer(backend, nil, provider)
	if err := s.ConfigureEnvironmentAI(t.Context(), AIEnvironmentConfig{Provider: "openai-compatible", APIKey: "fixture-only", Endpoint: "https://llm.example.test", Models: map[ai.Workload]string{ai.WorkloadAnalysis: "fixture"}}); err != nil {
		t.Fatal(err)
	}
	return s
}

func seedKnowledgeDocuments(t *testing.T, backend store.Store, runID string, contents []string) {
	t.Helper()
	deployment, err := backend.Deployment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := randomUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = backend.CreateSource(t.Context(), model.Source{ID: sourceID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Name: "Knowledge fixture " + sourceID, Kind: "upload", Location: "knowledge.md", Visibility: model.VisibilityPrivate}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = backend.CreateDeveloperAssetIngestionRun(t.Context(), model.DeveloperAssetIngestionRun{ID: runID, DeploymentID: deployment.ID, OrganisationID: deployment.OrganisationID, AssetKind: model.DeveloperAssetDocumentation, TargetID: sourceID, TargetKey: "source:" + sourceID, SourceID: sourceID, State: model.DeveloperAssetIngestionReviewReady, Attempt: 1, AcquiredCount: len(contents), FinishedAt: &now, StartedAt: &now, QueuedAt: now, Versions: model.ProcessorVersions{Pipeline: "v1", Parser: "v1", Normalizer: "v1", Mapper: "v1"}, RawManifest: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	documents := []model.DocumentationDocument{}
	for index, content := range contents {
		documentID, err := randomUUID()
		if err != nil {
			t.Fatal(err)
		}
		documents = append(documents, model.DocumentationDocument{ID: documentID, DeploymentID: deployment.ID, IngestionRunID: runID, Kind: "guide", MediaType: "text/markdown", Title: fmt.Sprintf("Guide %d", index), SourcePath: fmt.Sprintf("guide%d.md", index), ContentHash: contentHash([]byte(content)), NormalizedMarkdown: content, Visibility: model.VisibilityPrivate, Metadata: json.RawMessage(`{}`)})
	}
	if err = backend.SaveDocumentationIngestionOutput(t.Context(), deployment.ID, store.DocumentationIngestionOutput{Documents: documents}); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeProcessingRequiresAIAndPersistsFailure(t *testing.T) {
	memory := store.NewMemory()
	seedKnowledgeDocuments(t, memory, "required", []string{"Install the exact SDK release."})
	s := New(memory)
	if _, err := s.requireKnowledgeProcessing(t.Context(), "required"); !errors.Is(err, ErrKnowledgeProcessingRequired) {
		t.Fatalf("gate=%v", err)
	}
	if _, err := s.ProcessKnowledgeBatch(t.Context(), "required", Actor{ID: "reviewer"}); !errors.Is(err, ErrAIUnavailable) {
		t.Fatalf("unconfigured=%v", err)
	}
	status, err := s.KnowledgeProcessingStatus(t.Context(), "required")
	if err != nil || status.State != "required" || status.CompletedBatches != 0 || status.ErrorCode == "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	stages, _ := memory.DeveloperAssetIngestionStages(t.Context(), "required")
	if len(stages) != 1 || stages[0].State != "failed" || !strings.Contains(string(stages[0].Checkpoint), `"processed_by":"reviewer"`) {
		t.Fatalf("stages=%#v", stages)
	}
}

func TestKnowledgeProcessingResumesOnlyMissingExactBatches(t *testing.T) {
	memory := store.NewMemory()
	contents := []string{}
	for index := 0; index < 9; index++ {
		contents = append(contents, fmt.Sprintf("Guide %d: install version 1.2.3.", index))
	}
	seedKnowledgeDocuments(t, memory, "resumable", contents)
	provider := &aitest.Knowledge{Transform: func(call int, raw json.RawMessage) (json.RawMessage, error) {
		if call == 2 {
			return json.RawMessage(`{"items":[]}`), nil
		}
		return raw, nil
	}}
	s := configuredKnowledgeService(t, memory, provider)
	status, err := s.ProcessKnowledgeBatch(t.Context(), "resumable", Actor{ID: "reviewer"})
	if err != nil || status.CompletedBatches != 1 || status.TotalBatches != 2 || status.State == "ready" {
		t.Fatalf("first=%#v %v", status, err)
	}
	firstStage := status.StageIDs[0]
	if _, err = s.ProcessKnowledgeBatch(t.Context(), "resumable", Actor{}); ai.Code(err) != ai.ErrorInvalidStructuredOutput {
		t.Fatalf("malformed=%v", err)
	}
	if _, err = s.requireKnowledgeProcessing(t.Context(), "resumable"); !errors.Is(err, ErrKnowledgeProcessingRequired) {
		t.Fatalf("partial gate=%v", err)
	}
	// Recreate the service to prove progress comes from durable checkpoints.
	s = configuredKnowledgeService(t, memory, provider)
	status, err = s.ProcessKnowledgeBatch(t.Context(), "resumable", Actor{})
	if err != nil || status.State != "ready" || len(status.Assessments) != 9 || status.StageIDs[0] != firstStage || provider.Calls.Load() != 3 {
		t.Fatalf("resumed=%#v %v calls=%d", status, err, provider.Calls.Load())
	}
	if _, err = s.ProcessKnowledgeBatch(t.Context(), "resumable", Actor{}); err != nil || provider.Calls.Load() != 3 {
		t.Fatalf("repeat called provider: %v", err)
	}
	// A new import with identical bytes requires its own processing and lineage.
	seedKnowledgeDocuments(t, memory, "other_import", contents)
	if _, err = s.requireKnowledgeProcessing(t.Context(), "other_import"); !errors.Is(err, ErrKnowledgeProcessingRequired) {
		t.Fatalf("other import accepted: %v", err)
	}
}

func TestKnowledgeProcessingRejectsInventedAndIncompleteEvidence(t *testing.T) {
	for _, mode := range []string{"unknown_id", "duplicate_id", "wrong_quote", "missing_item", "unknown_finding", "extra_property"} {
		t.Run(mode, func(t *testing.T) {
			memory := store.NewMemory()
			seedKnowledgeDocuments(t, memory, "invalid", []string{"Use the exact release.", "Configure the callback URL."})
			provider := &aitest.Knowledge{Transform: func(_ int, raw json.RawMessage) (json.RawMessage, error) {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				items := body["items"].([]any)
				item := items[0].(map[string]any)
				switch mode {
				case "unknown_id":
					item["id"] = "not-supplied"
				case "duplicate_id":
					items[1] = items[0]
				case "wrong_quote":
					item["evidence_quote"] = "A claim absent from this source."
				case "missing_item":
					body["items"] = items[:1]
				case "unknown_finding":
					item["findings"] = []string{"tested"}
				case "extra_property":
					item["approved"] = true
				}
				return json.Marshal(body)
			}}
			s := configuredKnowledgeService(t, memory, provider)
			if _, err := s.ProcessKnowledgeBatch(t.Context(), "invalid", Actor{}); ai.Code(err) != ai.ErrorInvalidStructuredOutput {
				t.Fatalf("accepted invalid evidence: %v", err)
			}
			if status, err := s.KnowledgeProcessingStatus(t.Context(), "invalid"); err != nil || status.CompletedBatches != 0 || len(status.Assessments) != 0 {
				t.Fatalf("invalid retained: %#v %v", status, err)
			}
		})
	}
}

func TestKnowledgeProcessingSerializesProviderCalls(t *testing.T) {
	memory := store.NewMemory()
	seedKnowledgeDocuments(t, memory, "concurrent", []string{"Use version 1.2.3."})
	entered, release := make(chan struct{}), make(chan struct{})
	provider := &aitest.Knowledge{Before: func(r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}}
	s := configuredKnowledgeService(t, memory, provider)
	done := make(chan error, 1)
	go func() { _, err := s.ProcessKnowledgeBatch(t.Context(), "concurrent", Actor{}); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not entered")
	}
	_, err := s.ProcessKnowledgeBatch(t.Context(), "concurrent", Actor{})
	close(release)
	if !errors.Is(err, ErrKnowledgeProcessingBusy) {
		t.Fatalf("concurrent=%v", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if provider.Calls.Load() != 1 {
		t.Fatalf("provider calls=%d", provider.Calls.Load())
	}
}

type knowledgeSaveFailureStore struct {
	*store.Memory
	fail bool
}

func (s *knowledgeSaveFailureStore) SaveDeveloperAssetIngestionStage(ctx context.Context, stage model.DeveloperAssetIngestionStage, expected string) (model.DeveloperAssetIngestionStage, error) {
	if s.fail {
		return model.DeveloperAssetIngestionStage{}, errors.New("fixture disk unavailable")
	}
	return s.Memory.SaveDeveloperAssetIngestionStage(ctx, stage, expected)
}

func TestKnowledgeProcessingDoesNotReportSuccessWhenCheckpointSaveFails(t *testing.T) {
	backend := &knowledgeSaveFailureStore{Memory: store.NewMemory(), fail: true}
	seedKnowledgeDocuments(t, backend, "save_failure", []string{"Use the SDK."})
	s := configuredKnowledgeService(t, backend, &aitest.Knowledge{})
	if _, err := s.ProcessKnowledgeBatch(t.Context(), "save_failure", Actor{}); err == nil {
		t.Fatal("reported success without durable result")
	}
	if status, err := s.KnowledgeProcessingStatus(t.Context(), "save_failure"); err != nil || status.State == "ready" || status.CompletedBatches != 0 {
		t.Fatalf("status=%#v %v", status, err)
	}
	// A crashed worker lease expires; the same import can be processed again.
	backend.fail = false
	s.now = func() time.Time { return time.Now().UTC().Add(4 * time.Minute) }
	if status, err := s.ProcessKnowledgeBatch(t.Context(), "save_failure", Actor{}); err != nil || status.State != "ready" {
		t.Fatalf("recovery=%#v %v", status, err)
	}
}

func TestKnowledgeProcessingBoundsAndPreservesUnicodeEvidence(t *testing.T) {
	memory := store.NewMemory()
	content := strings.Repeat("<東京> integration\n", 1800)
	seedKnowledgeDocuments(t, memory, "unicode", []string{content})
	s := New(memory)
	scope, err := s.knowledgeProcessingScope(t.Context(), "unicode")
	if err != nil {
		t.Fatal(err)
	}
	byPart := map[int]string{}
	parts := 0
	for _, batch := range scope.batches {
		encoded, _ := json.Marshal(batch.evidence)
		if len(encoded) > knowledgeBatchBytes+2 {
			t.Fatalf("batch bytes=%d", len(encoded))
		}
		for _, item := range batch.evidence {
			byPart[item.Part] = item.Content
			parts++
		}
	}
	var restored strings.Builder
	for part := 1; part <= parts; part++ {
		restored.WriteString(byPart[part])
	}
	if restored.String() != content {
		t.Fatal("split lost or changed source text")
	}
}
