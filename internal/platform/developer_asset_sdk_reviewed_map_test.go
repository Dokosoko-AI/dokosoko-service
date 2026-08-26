package platform_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func TestSDKPublicationMapProjectsOnlyReviewedFilesAndSamples(t *testing.T) {
	t.Parallel()
	memory, service, release, actor := sdkIngestionFixture(t)
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{
			{SourcePath: "examples/allowed.go", Content: "package examples\n\nfunc Allowed() {}\n", LicenseExpression: "MIT"},
			{SourcePath: "examples/rejected-secret.go", Content: "package examples\n\nfunc RejectedCandidateOnly() {}\n", LicenseExpression: "MIT"},
		},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidate.Files) != 2 || len(result.Candidate.Samples) != 2 || result.Candidate.Map == nil {
		t.Fatalf("candidate shape = files %d, samples %d, map %#v", len(result.Candidate.Files), len(result.Candidate.Samples), result.Candidate.Map)
	}
	if !strings.Contains(result.Candidate.Map.AgentMarkdown, "rejected-secret.go") {
		t.Fatalf("candidate map should prove the rejected evidence existed before review: %s", result.Candidate.Map.AgentMarkdown)
	}

	fileDecisions := make([]platform.DeveloperAssetReviewDecision, 0, len(result.Candidate.Files))
	for _, file := range result.Candidate.Files {
		decision := platform.DeveloperAssetReviewDecision{ID: file.ID, Decision: "included"}
		if strings.Contains(file.SourcePath, "rejected-secret") {
			decision.Decision, decision.Reason = "excluded", "not suitable for publication"
		}
		fileDecisions = append(fileDecisions, decision)
	}
	sampleDecisions := make([]platform.DeveloperAssetReviewDecision, 0, len(result.Candidate.Samples))
	approvedSampleID := ""
	for _, sample := range result.Candidate.Samples {
		decision := platform.DeveloperAssetReviewDecision{ID: sample.ID, Decision: "approved"}
		if strings.Contains(sample.SourcePath, "rejected-secret") {
			decision.Decision, decision.Reason = "excluded", "not suitable for publication"
		} else {
			approvedSampleID = sample.ID
		}
		sampleDecisions = append(sampleDecisions, decision)
	}
	publication, err := service.PublishSDKContentCandidate(t.Context(), release.ID, result.Candidate.Candidate.ID, platform.SDKContentCandidatePublicationInput{
		Files: fileDecisions, Samples: sampleDecisions, AcknowledgeReviewed: true,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := memory.SDKContentPublication(t.Context(), release.DeploymentID, publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Map == nil || stored.PublishedMap == nil || stored.Map.SDKMapID != stored.PublishedMap.ID || stored.Map.ContentHash != stored.PublishedMap.ContentHash {
		t.Fatalf("publication map projection/link = %#v / %#v", stored.PublishedMap, stored.Map)
	}
	if stored.PublishedMap.MapVersion != "sdk-reviewed-publication-map/1" || len(stored.PublishedMap.Map.Samples) != 1 || stored.PublishedMap.Map.Samples[0].ID != approvedSampleID {
		t.Fatalf("reviewed sample map = %#v", stored.PublishedMap.Map.Samples)
	}
	encodedMap, err := json.Marshal(stored.PublishedMap.Map)
	if err != nil {
		t.Fatal(err)
	}
	for _, rejectedMarker := range []string{"rejected-secret.go", "rejected-secret", "RejectedCandidateOnly"} {
		if strings.Contains(stored.PublishedMap.AgentMarkdown, rejectedMarker) || strings.Contains(string(encodedMap), rejectedMarker) {
			t.Fatalf("reviewed map leaked rejected evidence %q: %s\n%s", rejectedMarker, stored.PublishedMap.AgentMarkdown, encodedMap)
		}
	}
	if stored.Publication.Visibility != model.VisibilityPublic {
		t.Fatalf("publication visibility = %q", stored.Publication.Visibility)
	}
}
