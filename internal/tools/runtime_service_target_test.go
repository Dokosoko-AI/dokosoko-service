package tools

import (
	"encoding/json"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestPrepareRuntimeToolSelectsExactEnvironmentAndComposesTarget(t *testing.T) {
	tool := model.Tool{
		ID: "tool-1", RuntimeServiceConnectionID: "connection-1", HTTPPath: "/v1/status", HTTPMethod: "GET",
		RuntimeTargets: []model.ToolRuntimeTarget{
			{EnvironmentID: "production", RuntimeServiceConnectionID: "connection-1", ConnectionRevisionID: "revision-prod", BaseURL: "https://prod.example.test", AuthenticationType: "none", AuthConfig: json.RawMessage(`{}`)},
			{EnvironmentID: "staging", RuntimeServiceConnectionID: "connection-1", ConnectionRevisionID: "revision-stage", BaseURL: "https://stage.example.test/", AuthenticationType: "api_key_header", CredentialSetID: "credential-set", CredentialVersionID: "credential-version", CredentialSecretID: "secret", CredentialFingerprint: "fingerprint", HeaderName: "X-Voice-Key", AuthConfig: json.RawMessage(`{}`)},
		},
	}
	prepared, err := prepareRuntimeTool(tool, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.BaseURL != "https://stage.example.test/v1/status" || prepared.RuntimeConnectionRevisionID != "revision-stage" || prepared.RuntimeCredentialVersionID != "credential-version" || prepared.CredentialID != "secret" {
		t.Fatalf("prepared tool = %#v", prepared)
	}
	var auth upstreamAuth
	if err := json.Unmarshal(prepared.UpstreamAuth, &auth); err != nil || auth.Type != "api_key_header" || auth.HeaderName != "X-Voice-Key" {
		t.Fatalf("prepared auth = %#v err=%v", auth, err)
	}
	if _, err := prepareRuntimeTool(tool, "unknown"); err != ErrDenied {
		t.Fatalf("unknown environment error = %v", err)
	}
	if _, err := prepareRuntimeTool(tool, ""); err != ErrDenied {
		t.Fatalf("ambiguous environment error = %v", err)
	}
}

func TestPrepareRuntimeToolLeavesLegacyToolUnchanged(t *testing.T) {
	legacy := model.Tool{ID: "legacy", APIConnectionID: "api-connection", BaseURL: "https://legacy.example.test/v1/status"}
	prepared, err := prepareRuntimeTool(legacy, "production")
	if err != nil || prepared.BaseURL != legacy.BaseURL || prepared.APIConnectionID != legacy.APIConnectionID {
		t.Fatalf("legacy tool = %#v err=%v", prepared, err)
	}
}
