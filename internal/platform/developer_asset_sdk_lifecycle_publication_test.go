package platform_test

import (
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/platform"
)

func TestArchivedSDKReleaseCannotCreateContentPublication(t *testing.T) {
	t.Parallel()
	_, service, release, actor := sdkIngestionFixture(t)
	result, err := service.IngestSDKReleaseContent(t.Context(), release.ID, platform.SDKContentIngestionInput{
		ResolvedSourceRevision: "commit-abc123",
		Files: []platform.SDKIngestionFile{{
			SourcePath: "README.md", Content: "# Archived release\n\nThis reviewed file remains historical.\n", Role: "readme",
		}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendSDKReleaseLifecycleEvent(t.Context(), release.ID, platform.SDKReleaseLifecycleEventInput{
		Lifecycle: "archived", Reason: "This exact release is no longer available for new publications.",
	}, actor); err != nil {
		t.Fatal(err)
	}
	_, err = service.PublishSDKContentCandidate(t.Context(), release.ID, result.Candidate.Candidate.ID, platform.SDKContentCandidatePublicationInput{
		AcknowledgeReviewed: true,
		Files: []platform.DeveloperAssetReviewDecision{{
			ID: result.Candidate.Files[0].ID, Decision: "included",
		}},
	}, actor)
	if !errors.Is(err, platform.ErrSDKReleaseUnavailable) {
		t.Fatalf("archived SDK content publication error = %v", err)
	}
}
