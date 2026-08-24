package main

import (
	"testing"

	"github.com/dokosoko/mcp-acceptance-client/internal/acceptance"
)

func TestExitCodeFailsRequiredSkipButAllowsOptionalSkip(t *testing.T) {
	t.Parallel()

	optional := acceptance.Report{}
	optional.Add(acceptance.Check{Name: "optional", Status: acceptance.Skip})
	if got := exitCodeForReport(optional); got != 0 {
		t.Fatalf("optional skip exit code = %d, want 0", got)
	}

	required := acceptance.Report{}
	required.Add(acceptance.Check{Name: "requested", Status: acceptance.Skip, Required: true})
	if got := exitCodeForReport(required); got != 1 {
		t.Fatalf("required skip exit code = %d, want 1", got)
	}
}
