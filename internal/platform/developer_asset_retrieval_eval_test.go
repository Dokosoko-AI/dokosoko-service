package platform

import (
	"encoding/json"
	"os"
	"testing"
)

type retrievalEvaluationSet struct {
	Version                 string                    `json:"version"`
	RetrievalProfileVersion string                    `json:"retrieval_profile_version"`
	Cases                   []retrievalEvaluationCase `json:"cases"`
}

type retrievalEvaluationCase struct {
	Name              string `json:"name"`
	Query             string `json:"query"`
	ExpectedEvidence  string `json:"expected_evidence"`
	ForbiddenEvidence string `json:"forbidden_evidence"`
}

func TestVersionedRetrievalEvaluationSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/retrieval-eval-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var evaluation retrievalEvaluationSet
	if err := json.Unmarshal(raw, &evaluation); err != nil {
		t.Fatal(err)
	}
	if evaluation.Version != "retrieval-eval-v1" || evaluation.RetrievalProfileVersion != DeveloperAssetRetrievalProfileVersion || len(evaluation.Cases) < 3 {
		t.Fatalf("invalid evaluation set metadata: %#v", evaluation)
	}
	for _, testCase := range evaluation.Cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			query := localDeveloperAssetEmbedding(testCase.Query)
			expected := cosineForTest(localDeveloperAssetEmbedding(testCase.ExpectedEvidence), query)
			forbidden := cosineForTest(localDeveloperAssetEmbedding(testCase.ForbiddenEvidence), query)
			if expected <= forbidden {
				t.Fatalf("expected evidence score %.4f must exceed forbidden evidence score %.4f", expected, forbidden)
			}
		})
	}
}
