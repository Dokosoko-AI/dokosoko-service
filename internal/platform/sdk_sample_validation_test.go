package platform

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestStaticSDKSampleValidationUsesParsersAndKeepsStructuralChecksAdvisory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		language     string
		code         string
		completeFile bool
		machine      bool
	}{
		{name: "javascript", language: "javascript", code: `const client = new Client();`},
		{name: "typescript", language: "typescript", code: `const client: Client = new Client();`},
		{name: "python", language: "python", code: "if ready:\n    client.list()\n"},
		{name: "go file", language: "go", code: "package main\nfunc main() { client.List() }\n", completeFile: true, machine: true},
		{name: "go snippet", language: "go", code: `client.List()`, machine: true},
		{name: "json", language: "json", code: `{"mode":"test"}`, machine: true},
		{name: "java", language: "java", code: `var client = new Client();`},
		{name: "csharp alias", language: "c#", code: `var client = new Client();`},
		{name: "ruby", language: "ruby", code: `client = Client.new`},
		{name: "php", language: "php", code: `<?php $client = new Client();`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status, evidence := staticSDKSampleValidation(test.language, test.code, test.completeFile)
			sample := model.SDKCodeSample{ValidationStatus: status, ValidationEvidence: evidence}
			var body map[string]any
			if err := json.Unmarshal(evidence, &body); err != nil || body["no_execution"] != true || body["no_dependency_install"] != true {
				t.Fatalf("unsafe or incomplete evidence = %s, err=%v", evidence, err)
			}
			if test.machine {
				if status != model.SDKSampleSyntaxChecked || !sample.HasPositiveMachineValidationEvidence() || body["result"] != "passed" {
					t.Fatalf("parser result status=%q evidence=%s", status, evidence)
				}
			} else if status != model.SDKSampleNotChecked || sample.HasPositiveMachineValidationEvidence() || body["structure_passed"] != true || body["result"] != "not_checked" {
				t.Fatalf("advisory structure check became machine evidence: status=%q evidence=%s", status, evidence)
			}
		})
	}
}

func TestStaticSDKSampleValidationFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		language string
		code     string
	}{
		{name: "javascript unclosed delimiter", language: "javascript", code: `const client = (`},
		{name: "typescript missing operand", language: "typescript", code: `const client: Client = ;`},
		{name: "python missing block colon", language: "python", code: "if ready\n    client.list()\n"},
		{name: "go parser rejection", language: "go", code: `client.(`},
		{name: "java missing terminator", language: "java", code: `var client = new Client()`},
		{name: "csharp missing operand", language: "csharp", code: `var client = ;`},
		{name: "ruby unclosed block", language: "ruby", code: "class Client\n"},
		{name: "php missing terminator", language: "php", code: `<?php $client = new Client()`},
		{name: "balanced but invalid javascript", language: "javascript", code: `const answer = true false;`},
		{name: "unsupported language", language: "rust", code: `let client = Client::new();`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status, evidence := staticSDKSampleValidation(test.language, test.code, false)
			if status != model.SDKSampleNotChecked {
				t.Fatalf("status = %q, evidence = %s", status, evidence)
			}
			sample := model.SDKCodeSample{ValidationStatus: status, ValidationEvidence: evidence}
			if sample.HasPositiveMachineValidationEvidence() {
				t.Fatalf("not-checked evidence became approvable: %s", evidence)
			}
			var body map[string]any
			if err := json.Unmarshal(evidence, &body); err != nil || body["validated"] != false || body["result"] != "not_checked" || body["no_execution"] != true {
				t.Fatalf("fail-closed evidence = %s, err=%v", evidence, err)
			}
		})
	}
}

func TestBalancedInvalidJavaScriptCannotBecomeMachineApproved(t *testing.T) {
	t.Parallel()
	status, evidence := staticSDKSampleValidation("javascript", `const answer = true false;`, false)
	sample := model.SDKCodeSample{
		ID: "sample", SDKContentCandidateID: "candidate", ContentHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ValidationStatus: status, ValidationEvidence: evidence,
	}
	ordinal := 0
	selection := model.SDKContentPublicationSampleSelection{
		SDKContentCandidateID: "candidate", SDKCodeSampleID: "sample", ContentHash: sample.ContentHash,
		Decision: "approved", Ordinal: &ordinal,
	}
	if status != model.SDKSampleNotChecked || sample.HasPositiveMachineValidationEvidence() || selection.ValidFor(sample) {
		t.Fatalf("balanced invalid JavaScript became machine approvable: status=%q evidence=%s", status, evidence)
	}
	selection.ReviewEvidence = json.RawMessage(`{"summary":"Reviewer used an independently pinned grammar parser and inspected the diagnostics."}`)
	if !selection.ValidFor(sample) {
		t.Fatal("structured explicit review evidence did not enable the manual approval path")
	}
}
