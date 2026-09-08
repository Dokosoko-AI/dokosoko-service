package platform

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/ai/aitest"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAIInputSecurityDecodesJSONBeforeInspectingSourceText(t *testing.T) {
	source := "this.apiKey = apiKey;\nthis.sleep = sleep;\n"
	if containsAISecretText(source) {
		t.Fatal("ordinary source variable assignment was classified as a credential")
	}
	for _, envelope := range []any{source, map[string]any{"evidence": []any{map[string]any{"content": source}}}, []any{source}} {
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if containsAISecretText(string(encoded)) {
			t.Fatalf("JSON encoding changed the source's secret classification: %s", encoded)
		}
	}
}

func TestKnowledgeProcessingRejectsEscapedAndDuplicateSecretsBeforeProvider(t *testing.T) {
	for name, content := range map[string]string{
		"escaped":   `{"content":"\u0041KIA0123456789ABCDEF"}`,
		"duplicate": `{"content":"api_key=unambiguously-secret-value","content":"ordinary guidance"}`,
	} {
		t.Run(name, func(t *testing.T) {
			backend := store.NewMemory()
			seedKnowledgeDocuments(t, backend, "credential-boundary", []string{content})
			provider := &aitest.Knowledge{}
			service := configuredKnowledgeService(t, backend, provider)
			_, err := service.ProcessKnowledgeBatch(t.Context(), "credential-boundary", Actor{ID: "reviewer"})
			var failure *ai.Error
			if !errors.As(err, &failure) || failure.Code != ai.ErrorUnsafeInput || provider.Calls.Load() != 0 || err.Error() != "unsafe_input" {
				t.Fatalf("credential crossed the provider boundary or leaked into the error: %v, calls=%d", err, provider.Calls.Load())
			}
		})
	}
}

func TestAIInputSecurityRetainsCredentialChecksAcrossJSONEnvelopes(t *testing.T) {
	for _, source := range []string{
		`api_key=unambiguously-secret-value`,
		`Authorization: Bearer fixtureCredential123`,
		`AWS_SECRET_ACCESS_KEY=0123456789abcdefghijklmnop`,
		`AKIA0123456789ABCDEF`,
		`-----BEGIN PRIVATE KEY-----`,
		`github_pat_FAKEFAKEFAKEFAKEFAKEFAKEFAKE`,
		`https://username:password@example.test`,
	} {
		for _, envelope := range []any{source, map[string]any{"evidence": []any{map[string]any{"content": source}}}} {
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if !containsAISecretText(source) || !containsAISecretText(string(encoded)) {
				t.Fatal("credential text was allowed through a JSON provider envelope")
			}
		}
	}
	for _, encoded := range []string{
		`{"api_key":"literal-credential-value"}`,
		`{"nested":{"client-secret":"literal-credential-value"}}`,
		`{"apiKey":"ordinary-looking-credential"}`,
		`{"AWS_SECRET_ACCESS_KEY":"ordinary-looking-credential"}`,
		`{"vendorApiKey":"ordinary-looking-credential"}`,
		`{"accountKey":"ordinary-looking-credential"}`,
		`{"content":"\u0041KIA0123456789ABCDEF"}`,
		`{"content":"api_\u006bey=unambiguously-secret-value"}`,
		`{"content":"github_\u0070at_FAKEFAKEFAKEFAKEFAKEFAKEFAKE"}`,
		`{"content":"api_key=unambiguously-secret-value","content":"ordinary guidance"}`,
		`{"outer":{"api_key":"literal-credential-value"},"outer":{"type":"string"}}`,
		`{"api_key=unambiguously-secret-value":"ordinary guidance"}`,
		`{"account_key":12345678901234567890}`,
	} {
		if !containsAISecretText(encoded) {
			t.Fatal("structural or escaped credential escaped validation")
		}
	}
	if containsAISecretText(`{"properties":{"api_key":{"type":"string","description":"Load a customer credential from application configuration."}}}`) {
		t.Fatal("credential field schema was treated as a credential value")
	}
	if containsAISecretText(`{"requiresAuthorization":false,"password":null,"api_key":{"required":true}}`) {
		t.Fatal("credential requirement metadata was treated as a literal credential")
	}
}
