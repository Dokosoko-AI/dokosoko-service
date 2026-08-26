package platform

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestRecipeReviewValidationFindingsBindStaticMessageToExactEvidence(t *testing.T) {
	t.Parallel()

	evidence := []model.IntegrationEvidence{
		{Kind: "tool", ResourceID: "tool-create-payment", Fingerprint: "sha256:tool-create-payment-v3"},
		{Kind: "source_publication", ResourceID: "doc-create-payment", Fingerprint: "sha256:doc-create-payment-v2"},
	}
	findings, valid := recipeReviewValidationFindings(recipeReviewResponse{
		Recommendation: "revise",
		Findings: []recipeReviewFindingSelection{
			{Code: "unsupported_claim", EvidenceIDs: []string{"tool-create-payment"}},
			{Code: "unsupported_claim", EvidenceIDs: []string{"doc-create-payment"}},
		},
	}, evidence)
	if !valid || len(findings) != 1 {
		t.Fatalf("findings = %#v, valid = %t", findings, valid)
	}
	wantMessage := "At least one material implementation claim is not stated by its selected evidence."
	if findings[0].Level != "warning" || findings[0].Code != "ai_unsupported_claim" || findings[0].Message != wantMessage {
		t.Fatalf("server-owned finding = %#v", findings[0])
	}
	wantEvidence := []model.RecipeEvidenceRef{
		{Kind: "tool", ResourceID: "tool-create-payment", Fingerprint: "sha256:tool-create-payment-v3"},
		{Kind: "source_publication", ResourceID: "doc-create-payment", Fingerprint: "sha256:doc-create-payment-v2"},
	}
	if len(findings[0].Evidence) != len(wantEvidence) {
		t.Fatalf("finding evidence = %#v", findings[0].Evidence)
	}
	for index := range wantEvidence {
		if findings[0].Evidence[index] != wantEvidence[index] {
			t.Fatalf("finding evidence %d = %#v, want %#v", index, findings[0].Evidence[index], wantEvidence[index])
		}
	}
}

func TestRecipeReviewValidationFindingsRejectUnboundEvidence(t *testing.T) {
	t.Parallel()

	validEvidence := []model.IntegrationEvidence{{Kind: "tool", ResourceID: "tool-create-payment", Fingerprint: "sha256:tool-create-payment-v3"}}
	tests := []struct {
		name     string
		response recipeReviewResponse
		evidence []model.IntegrationEvidence
	}{
		{
			name:     "unknown evidence",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "evidence_gap", EvidenceIDs: []string{"tool-refund"}}}},
			evidence: validEvidence,
		},
		{
			name:     "ambiguous evidence",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "evidence_gap", EvidenceIDs: []string{"tool-create-payment"}}}},
			evidence: append(append([]model.IntegrationEvidence(nil), validEvidence...), validEvidence[0]),
		},
		{
			name:     "duplicate evidence selection",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "evidence_gap", EvidenceIDs: []string{"tool-create-payment", "tool-create-payment"}}}},
			evidence: validEvidence,
		},
		{
			name:     "non-exact evidence identifier",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "evidence_gap", EvidenceIDs: []string{" tool-create-payment"}}}},
			evidence: validEvidence,
		},
		{
			name:     "missing immutable fingerprint",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "evidence_gap", EvidenceIDs: []string{"tool-create-payment"}}}},
			evidence: []model.IntegrationEvidence{{Kind: "tool", ResourceID: "tool-create-payment"}},
		},
		{
			name:     "unknown finding code",
			response: recipeReviewResponse{Recommendation: "revise", Findings: []recipeReviewFindingSelection{{Code: "write_anything", EvidenceIDs: []string{"tool-create-payment"}}}},
			evidence: validEvidence,
		},
		{
			name:     "pass with findings",
			response: recipeReviewResponse{Recommendation: "pass", Findings: []recipeReviewFindingSelection{{Code: "not_minimal", EvidenceIDs: []string{"tool-create-payment"}}}},
			evidence: validEvidence,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if findings, valid := recipeReviewValidationFindings(test.response, test.evidence); valid || findings != nil {
				t.Fatalf("findings = %#v, valid = %t", findings, valid)
			}
		})
	}
}

func TestRecipeReviewSchemaRejectsModelAuthoredProse(t *testing.T) {
	t.Parallel()

	tests := []json.RawMessage{
		json.RawMessage(`{"recommendation":"pass","findings":[],"summary":"Looks good."}`),
		json.RawMessage(`{"recommendation":"revise","findings":[{"code":"not_minimal","evidence_ids":["tool-create-payment"],"message":"Delete the repeated step."}]}`),
	}
	for _, raw := range tests {
		if err := validateAIStructuredContract("fixture", recipeReviewSchema, raw); err == nil {
			t.Fatalf("schema accepted model-authored prose: %s", raw)
		}
		var response recipeReviewResponse
		if err := decodeStrictAIResult(raw, &response); err == nil {
			t.Fatalf("strict decoder accepted model-authored prose: %s", raw)
		}
	}
}

func TestRecipeReviewClosedCodesHaveServerOwnedMessages(t *testing.T) {
	t.Parallel()

	codes := []string{
		"delivery_scope",
		"multiple_capabilities",
		"sdk_scope",
		"non_actionable_step",
		"unobservable_check",
		"unsupported_claim",
		"unsafe_content",
		"not_minimal",
		"evidence_gap",
	}
	if len(recipeReviewFindingMessages) != len(codes) {
		t.Fatalf("server-owned messages = %d, closed codes = %d", len(recipeReviewFindingMessages), len(codes))
	}
	for _, code := range codes {
		if recipeReviewFindingMessages[code] == "" {
			t.Errorf("closed code %q has no server-owned message", code)
		}
		raw, err := json.Marshal(recipeReviewResponse{
			Recommendation: "revise",
			Findings:       []recipeReviewFindingSelection{{Code: code, EvidenceIDs: []string{"evidence-1"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateAIStructuredContract("fixture", recipeReviewSchema, raw); err != nil {
			t.Errorf("closed code %q is missing from the platform schema: %v", code, err)
		}
	}
}
