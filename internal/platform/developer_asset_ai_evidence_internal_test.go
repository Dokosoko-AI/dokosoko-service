package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestDeveloperAssetAIEvidenceBudgetReservesMandatoryLineage(t *testing.T) {
	t.Parallel()
	scope := newDeveloperAssetAIWorkflowScope(model.Product{ID: "prod_acme"}, AIPromptKeySDKApplicability)
	for index := 0; index < 300; index++ {
		scope.addEvidence(fmt.Sprintf("optional-%03d", index), "sdk_section", "Optional section", strings.Repeat("x", 1400), "", 1400)
	}
	mandatory := []string{"sdk-map", "api-publication", "api-sdk-binding", "target-sample", "target-sample-reference"}
	for _, id := range mandatory {
		scope.addMandatoryEvidence(id, "lineage", id, strings.Repeat("m", 800), "", 800)
	}
	if err := scope.finalizeEvidence(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(scope.evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > developerAssetAIMaxEvidenceBytes || len(scope.evidence) > developerAssetAIMaxEvidenceItems || !scope.truncated {
		t.Fatalf("bounded evidence bytes=%d items=%d truncated=%v", len(encoded), len(scope.evidence), scope.truncated)
	}
	for _, id := range mandatory {
		if !scope.allowedEvidence[id] {
			t.Errorf("mandatory evidence %q was displaced", id)
		}
	}
}
