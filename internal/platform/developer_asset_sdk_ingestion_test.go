package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type sdkFinalizationFaultStore struct {
	*store.Memory
	failNext bool
}

func (s *sdkFinalizationFaultStore) FinalizeSDKContentIngestion(ctx context.Context, value store.SDKContentIngestionFinalization) (model.SDKContentCandidate, model.DeveloperAssetIngestionRun, error) {
	if s.failNext {
		s.failNext = false
		value.Stages = append([]model.DeveloperAssetIngestionStage(nil), value.Stages...)
		duplicate := value.Stages[0]
		duplicate.ID = "fault-injected-duplicate-stage"
		value.Stages = append(value.Stages, duplicate)
	}
	return s.Memory.FinalizeSDKContentIngestion(ctx, value)
}

func sdkIngestionFixture(t *testing.T) (*store.Memory, *platform.Service, model.SDKRelease, platform.Actor) {
	t.Helper()
	memory := store.NewMemory()
	service := platform.New(memory)
	actor := platform.Actor{ID: "sdk-reviewer", RequestID: "request-sdk-ingestion"}
	packageValue, err := service.SaveSDKPackage(t.Context(), "", platform.SDKPackageInput{
		Ecosystem: "go", Coordinate: "example.com/acme/sdk", Name: "Acme Go SDK",
		Description: "Deployment-owned exact SDK releases.", Visibility: model.VisibilityPublic, Lifecycle: "active",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	release, err := service.CreateSDKRelease(t.Context(), packageValue.ID, platform.SDKReleaseInput{
		ExactVersion: "v1.2.3", SourceURL: "https://example.com/acme/sdk",
		ResolvedSourceRevision: "commit-abc123", IdentityAssurance: "resolved_source",
		Visibility: model.VisibilityPublic,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	return memory, service, release, actor
}

func TestSDKIngestionNormalizesExtractsMapsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	input := platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{
			{SourcePath: "README.md", Content: "# Quickstart\r\n\r\nInstall the exact release.  \r\n\r\n```json\r\n{\"mode\":\"test\"}\r\n```\r\n", LicenseExpression: "MIT"},
			{SourcePath: "client.go", Content: "package sdk\n\n// Client calls Acme.\ntype Client struct{}\n\nfunc NewClient() *Client { return &Client{} }\n", LicenseExpression: "MIT"},
			{SourcePath: "examples/main.go", Content: "package main\n\nimport \"example.com/acme/sdk\"\n\nfunc main() { _ = sdk.NewClient() }\n", LicenseExpression: "MIT"},
			{SourcePath: "vendor/dependency.go", Content: "package dependency\n", LicenseExpression: "MIT"},
		},
	}
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, input, actor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.State != model.DeveloperAssetIngestionReviewReady || result.Run.FailedCount != 0 || result.Run.SkippedCount != 0 {
		t.Fatalf("run = %#v", result.Run)
	}
	if result.Candidate.Map == nil || !strings.Contains(result.Candidate.Map.AgentMarkdown, "## Table of contents") || !strings.Contains(result.Candidate.Map.AgentMarkdown, "never executed") {
		t.Fatalf("SDK map = %#v", result.Candidate.Map)
	}
	if len(result.Candidate.Sections) < 3 || len(result.Candidate.Symbols) < 2 || len(result.Candidate.Samples) != 2 {
		t.Fatalf("candidate counts: sections=%d symbols=%d samples=%d", len(result.Candidate.Sections), len(result.Candidate.Symbols), len(result.Candidate.Samples))
	}
	foundNormalized, foundExcluded := false, false
	for _, file := range result.Candidate.Files {
		if file.SourcePath == "README.md" {
			foundNormalized = !strings.Contains(file.NormalizedContent, "\r") && !strings.Contains(file.NormalizedContent, "  \n")
		}
		if file.SourcePath == "vendor/dependency.go" {
			foundExcluded = file.Role == "vendor" && file.SuggestedDisposition == "excluded" && file.ExclusionReason != ""
		}
	}
	if !foundNormalized || !foundExcluded {
		t.Fatalf("normalized=%v excluded=%v files=%#v", foundNormalized, foundExcluded, result.Candidate.Files)
	}
	validated := 0
	for _, sample := range result.Candidate.Samples {
		if sample.Origin != model.SDKSampleExtracted || sample.SourceRevision != "commit-abc123" || sample.ValidationStatus != model.SDKSampleSyntaxChecked {
			t.Fatalf("sample = %#v", sample)
		}
		if !strings.Contains(string(sample.ValidationEvidence), `"no_execution":true`) {
			t.Fatalf("validation evidence = %s", sample.ValidationEvidence)
		}
		validated++
	}
	if validated != 2 {
		t.Fatalf("validated samples = %d", validated)
	}
	retry, err := service.IngestSDKReleaseContent(t.Context(), release.ID, input, actor)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.AlreadyIngested || retry.Candidate.Candidate.ID != result.Candidate.Candidate.ID || retry.Candidate.Candidate.ContentHash != result.Candidate.Candidate.ContentHash {
		t.Fatalf("idempotent retry = %#v", retry)
	}
}

func TestSDKIngestionFinalizationRollsBackAndRetryRecovers(t *testing.T) {
	t.Parallel()
	memory, _, release, actor := sdkIngestionFixture(t)
	faultStore := &sdkFinalizationFaultStore{Memory: memory, failNext: true}
	service := platform.New(faultStore)
	input := platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{
			{SourcePath: "README.md", Content: "# Quickstart\n\nUse the exact release.\n", LicenseExpression: "MIT"},
			{SourcePath: "client.go", Content: "package sdk\n\ntype Client struct{}\n", LicenseExpression: "MIT"},
		},
	}
	if _, err := service.IngestSDKReleaseContent(t.Context(), release.ID, input, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("fault-injected finalization error = %v, want conflict", err)
	}
	candidates, err := memory.SDKContentCandidates(t.Context(), release.DeploymentID, release.ID)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("candidate aggregate leaked across rollback: candidates=%#v err=%v", candidates, err)
	}
	runs, err := memory.DeveloperAssetIngestionRuns(t.Context(), release.DeploymentID, model.DeveloperAssetSDK, "sdk_release:"+release.ID)
	if err != nil || len(runs) != 1 || runs[0].State != model.DeveloperAssetIngestionFailed {
		t.Fatalf("failed attempt was not recoverable evidence: runs=%#v err=%v", runs, err)
	}
	stages, err := memory.DeveloperAssetIngestionStages(t.Context(), runs[0].ID)
	if err != nil || len(stages) != 0 {
		t.Fatalf("stage evidence leaked across rollback: stages=%#v err=%v", stages, err)
	}

	retry, err := service.IngestSDKReleaseContent(t.Context(), release.ID, input, actor)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AlreadyIngested || retry.Run.State != model.DeveloperAssetIngestionReviewReady {
		t.Fatalf("retry result = %#v", retry)
	}
	stages, err = memory.DeveloperAssetIngestionStages(t.Context(), retry.Run.ID)
	if err != nil || len(stages) != 11 {
		t.Fatalf("committed retry stages=%d err=%v", len(stages), err)
	}
	candidates, err = memory.SDKContentCandidates(t.Context(), release.DeploymentID, release.ID)
	if err != nil || len(candidates) != 1 || candidates[0].ID != retry.Candidate.Candidate.ID {
		t.Fatalf("committed retry candidate=%#v err=%v", candidates, err)
	}
}

func TestSDKIngestionQuarantinesPossibleSecretsAndCannotPublish(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{{
			SourcePath: "examples/unsafe.go",
			Content:    "package main\n// -----BEGIN PRIVATE KEY-----\nfunc main() {}\n",
		}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.QuarantinedCount != 1 || len(result.Candidate.Files) != 1 || result.Candidate.Files[0].SuggestedDisposition != "quarantined" || result.Candidate.Files[0].NormalizedContent != "" {
		t.Fatalf("quarantined candidate = %#v, run = %#v", result.Candidate.Files, result.Run)
	}
	_, err = service.PublishSDKContentCandidate(t.Context(), release.ID, result.Candidate.Candidate.ID, platform.SDKContentCandidatePublicationInput{
		AcknowledgeReviewed: true,
		Files:               []platform.DeveloperAssetReviewDecision{{ID: result.Candidate.Files[0].ID, Decision: "quarantined", Reason: "possible secret"}},
	}, actor)
	if !errors.Is(err, platform.ErrSourceReviewRequired) {
		t.Fatalf("publish error = %v", err)
	}
}

func TestSDKIngestionRejectsUnsafeSourcePaths(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	_, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		Files: []platform.SDKIngestionFile{{SourcePath: "../outside.go", Content: "package outside\n"}},
	}, actor)
	if err == nil || !strings.Contains(err.Error(), "cannot escape") {
		t.Fatalf("unsafe path error = %v", err)
	}
}

func TestSDKIngestionKeepsNonParserLanguageChecksAdvisory(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{
			{SourcePath: "examples/client.js", Content: "const client = new Client();\n"},
			{SourcePath: "examples/client.ts", Content: "const client: Client = new Client();\n"},
			{SourcePath: "examples/client.py", Content: "if ready:\n    client.list()\n"},
			{SourcePath: "examples/client.go", Content: "package main\nfunc main() { client.List() }\n"},
			{SourcePath: "examples/Client.java", Content: "var client = new Client();\n"},
			{SourcePath: "examples/Client.cs", Content: "var client = new Client();\n"},
			{SourcePath: "examples/client.rb", Content: "client = Client.new\n"},
			{SourcePath: "examples/client.php", Content: "<?php $client = new Client();\n"},
			{SourcePath: "examples/client.rs", Content: "let client = Client::new();\n"},
		},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidate.Candidate.Versions.Parser != "sdk-static-parser/2" || len(result.Candidate.Samples) != 9 {
		t.Fatalf("candidate parser/samples = %#v / %d", result.Candidate.Candidate.Versions, len(result.Candidate.Samples))
	}
	statuses := make(map[string]model.SDKSampleValidation)
	for _, sample := range result.Candidate.Samples {
		statuses[sample.Language] = sample.ValidationStatus
		if !strings.Contains(string(sample.ValidationEvidence), `"no_execution":true`) || !strings.Contains(string(sample.ValidationEvidence), `"no_dependency_install":true`) {
			t.Fatalf("%s evidence does not preserve the no-execution boundary: %s", sample.Language, sample.ValidationEvidence)
		}
	}
	if statuses["go"] != model.SDKSampleSyntaxChecked {
		t.Fatalf("Go parser status = %q, all statuses=%#v", statuses["go"], statuses)
	}
	for _, language := range []string{"javascript", "typescript", "python", "java", "csharp", "ruby", "php", "rust"} {
		if statuses[language] != model.SDKSampleNotChecked {
			t.Fatalf("%s status = %q, want not_checked; all statuses=%#v", language, statuses[language], statuses)
		}
	}
}

func TestSDKSampleApprovalRequiresMachineOrStructuredReviewEvidence(t *testing.T) {
	t.Parallel()
	memory, service, release, actor := sdkIngestionFixture(t)
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{{
			SourcePath: "examples/client.rs", Content: "let client = Client::new();\n",
		}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidate.Files) != 1 || len(result.Candidate.Samples) != 1 || result.Candidate.Samples[0].ValidationStatus != model.SDKSampleNotChecked {
		t.Fatalf("candidate = %#v", result.Candidate)
	}
	fileID, sampleID := result.Candidate.Files[0].ID, result.Candidate.Samples[0].ID
	input := platform.SDKContentCandidatePublicationInput{
		AcknowledgeReviewed: true,
		Files:               []platform.DeveloperAssetReviewDecision{{ID: fileID, Decision: "included"}},
		Samples:             []platform.DeveloperAssetReviewDecision{{ID: sampleID, Decision: "approved"}},
	}
	if _, err = service.PublishSDKContentCandidate(t.Context(), release.ID, result.Candidate.Candidate.ID, input, actor); err == nil || !strings.Contains(err.Error(), "positive machine validation evidence or explicit review evidence") {
		t.Fatalf("approval without evidence error = %v", err)
	}
	input.Samples[0].ReviewEvidence = json.RawMessage(`{"summary":"Reviewer parsed the exact sample with the pinned internal Rust parser and inspected its diagnostics.","method":"manual_parse_review"}`)
	publication, err := service.PublishSDKContentCandidate(t.Context(), release.ID, result.Candidate.Candidate.ID, input, actor)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := memory.SDKContentPublication(t.Context(), release.DeploymentID, publication.ID)
	if err != nil || len(stored.SampleSelections) != 1 || !model.ValidSDKSampleReviewEvidence(stored.SampleSelections[0].ReviewEvidence) {
		t.Fatalf("stored review evidence = %#v, err=%v", stored.SampleSelections, err)
	}
}

func TestSDKReleaseUsesDatabaseLifecycleVocabulary(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	if release.IdentityAssurance != "resolved_source" || release.Lifecycle != "active" {
		t.Fatalf("release = %#v", release)
	}
	packageValue, err := service.DeveloperAssetCatalog(t.Context())
	if err != nil || len(packageValue.SDKPackages) != 1 {
		t.Fatalf("catalog = %#v, err = %v", packageValue, err)
	}
	if _, err := service.CreateSDKRelease(t.Context(), packageValue.SDKPackages[0].ID, platform.SDKReleaseInput{
		ExactVersion: "v1.2.4", IdentityAssurance: "declared", Lifecycle: "withdrawn", Visibility: model.VisibilityPublic,
	}, actor); err == nil {
		t.Fatal("legacy SDK lifecycle vocabulary was accepted")
	}
}
