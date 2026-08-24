package docreview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestSafeAssessmentFailsClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		state      string
		indicators string
		want       bool
	}{
		{state: "validated", indicators: `[]`, want: true},
		{state: "published", indicators: `[]`, want: true},
		{state: "quarantined", indicators: `[]`},
		{state: "validated", indicators: `["instruction_override"]`},
		{state: "validated", indicators: `null`},
		{state: "validated", indicators: `{}`},
		{state: "validated", indicators: `not-json`},
	} {
		if got := SafeAssessment(test.state, json.RawMessage(test.indicators)); got != test.want {
			t.Errorf("SafeAssessment(%q, %q) = %v, want %v", test.state, test.indicators, got, test.want)
		}
	}
}

func TestPublicationContentHashIsReconstructibleFromFrozenReview(t *testing.T) {
	t.Parallel()
	documents := []model.CrawlReviewDocument{
		{ID: "b", CrawlJobID: "job", SnapshotID: "snapshot-b", Title: "B", CanonicalURL: "https://docs.test/b", State: "validated", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("b", 64), Changed: false},
		{ID: "a", CrawlJobID: "job", SnapshotID: "snapshot-a", Title: "A", CanonicalURL: "https://docs.test/a", State: "published", TrustLevel: 70, InjectionIndicators: json.RawMessage(`[]`), ContentHash: "sha256:" + strings.Repeat("a", 64), Changed: true},
	}
	want, err := PublicationContentHash(documents)
	if err != nil {
		t.Fatal(err)
	}
	got, err := PublicationContentHash([]model.CrawlReviewDocument{documents[1], documents[0]})
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !strings.HasPrefix(got, "sha256:") || len(got) != 71 {
		t.Fatalf("reconstructed hash = %q, want %q", got, want)
	}
}
