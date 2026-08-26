package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

func TestAIPromptConfigurationsExposeOnlyEditableWorkflowBodies(t *testing.T) {
	t.Parallel()

	service := New(store.NewMemory())
	configurations, err := service.AIPromptConfigurations(context.Background(), "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		AIPromptKeyIntegrationAnalysis,
		AIPromptKeyRecipeBrief,
		AIPromptKeyRecipeAuthoring,
		AIPromptKeyRecipeReview,
	}
	if len(configurations) != len(wantKeys) {
		t.Fatalf("prompt configurations = %d, want %d", len(configurations), len(wantKeys))
	}
	for index, configuration := range configurations {
		if configuration.Key != wantKeys[index] {
			t.Fatalf("prompt key %d = %q, want %q", index, configuration.Key, wantKeys[index])
		}
		if configuration.Label == "" || configuration.Description == "" || configuration.Instructions == "" || configuration.DefaultVersion == "" {
			t.Fatalf("incomplete default prompt configuration: %#v", configuration)
		}
		if configuration.Source != "default" || configuration.Revision != 1 || configuration.EffectiveVersion != configuration.DefaultVersion || configuration.UpdatedAt != nil {
			t.Fatalf("unexpected default prompt state: %#v", configuration)
		}
		if strings.Contains(configuration.Instructions, aiCommonUntrustedInputPolicy) || strings.Contains(configuration.Instructions, "Trust and execution policy:") {
			t.Fatalf("editable instructions exposed the immutable policy for %q", configuration.Key)
		}
	}
}

func TestAIPromptOverrideResetPreservesConcurrencyAndAuditsMetadataOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	actor := Actor{ID: "root_prompt", RequestID: "req_prompt"}
	firstInstructions := "Use only exact cited evidence.\nReturn one precise gap for every missing fact."
	secondInstructions := "Keep the analysis narrow and enumerate unresolved evidence gaps."

	overridden, err := service.SaveAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, firstInstructions, 1, actor)
	if err != nil {
		t.Fatal(err)
	}
	if overridden.Source != "override" || overridden.Revision != 2 || overridden.Instructions != firstInstructions || overridden.UpdatedAt == nil || !strings.HasSuffix(overridden.EffectiveVersion, "+override.2") {
		t.Fatalf("saved prompt = %#v", overridden)
	}
	unchanged, err := service.SaveAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, firstInstructions, overridden.Revision, actor)
	if err != nil || unchanged.Revision != overridden.Revision {
		t.Fatalf("no-op prompt save = %#v err=%v", unchanged, err)
	}
	if _, err = service.SaveAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, secondInstructions, 1, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale save error = %v, want revision conflict", err)
	}
	if _, err = service.ResetAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, 1, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale reset error = %v, want revision conflict", err)
	}

	reset, err := service.ResetAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, overridden.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Source != "default" || reset.Revision != 3 || reset.EffectiveVersion != reset.DefaultVersion || reset.Instructions == firstInstructions || reset.UpdatedAt == nil {
		t.Fatalf("reset prompt = %#v", reset)
	}
	if _, err = service.SaveAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, secondInstructions, overridden.Revision, actor); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("pre-reset revision became valid again: %v", err)
	}

	replaced, err := service.SaveAIPromptOverride(ctx, "prod_acme", AIPromptKeyIntegrationAnalysis, secondInstructions, reset.Revision, actor)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Revision != 4 || replaced.Source != "override" || replaced.Instructions != secondInstructions {
		t.Fatalf("replacement prompt = %#v", replaced)
	}

	events, err := memory.AuditEvents(ctx, "org_acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Action != "ai.prompt.saved" || events[1].Action != "ai.prompt.reset" || events[2].Action != "ai.prompt.saved" {
		t.Fatalf("prompt audit events = %#v", events)
	}
	encodedEvents, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedEvents), firstInstructions) || strings.Contains(string(encodedEvents), secondInstructions) {
		t.Fatalf("audit events contain editable prompt text: %s", encodedEvents)
	}
	firstFingerprint := sha256.Sum256([]byte(firstInstructions))
	if events[0].Current["instructions_bytes"] != len(firstInstructions) || events[0].Current["instructions_sha256"] != hex.EncodeToString(firstFingerprint[:]) {
		t.Fatalf("saved prompt audit metadata = %#v", events[0].Current)
	}
	if events[0].TargetID != AIPromptKeyIntegrationAnalysis || events[0].ActorID != actor.ID || events[0].RequestID != actor.RequestID {
		t.Fatalf("saved prompt audit identity = %#v", events[0])
	}
}

func TestAIPromptOverrideValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := New(store.NewMemory())
	actor := Actor{ID: "root_prompt"}
	tests := []struct {
		name         string
		key          string
		instructions string
		revision     int64
		want         error
	}{
		{name: "unknown key", key: "recipe.unknown", instructions: "Valid instructions.", revision: 1, want: store.ErrNotFound},
		{name: "missing revision", key: AIPromptKeyRecipeBrief, instructions: "Valid instructions.", revision: 0, want: ErrAIPromptInvalid},
		{name: "empty", key: AIPromptKeyRecipeBrief, instructions: " \n ", revision: 1, want: ErrAIPromptInvalid},
		{name: "too large", key: AIPromptKeyRecipeBrief, instructions: strings.Repeat("x", maxAIPromptInstructionsBytes+1), revision: 1, want: ErrAIPromptInvalid},
		{name: "invalid UTF-8", key: AIPromptKeyRecipeBrief, instructions: string([]byte{0xff}), revision: 1, want: ErrAIPromptInvalid},
		{name: "control character", key: AIPromptKeyRecipeBrief, instructions: "Review\x00facts", revision: 1, want: ErrAIPromptInvalid},
		{name: "secret value", key: AIPromptKeyRecipeBrief, instructions: "api_key=sk-test-secret-value", revision: 1, want: ErrAIPromptInvalid},
		{name: "bare GitHub fine-grained token", key: AIPromptKeyRecipeBrief, instructions: "Never expose github_pat_FAKEFAKEFAKEFAKEFAKEFAKEFAKE.", revision: 1, want: ErrAIPromptInvalid},
		{name: "bare GitHub installation token", key: AIPromptKeyRecipeBrief, instructions: "Never expose ghs_FAKEFAKEFAKEFAKEFAKEFAKE.", revision: 1, want: ErrAIPromptInvalid},
		{name: "bare Google API key", key: AIPromptKeyRecipeBrief, instructions: "Never expose AIzaFAKEFAKEFAKEFAKEFAKEFAKE.", revision: 1, want: ErrAIPromptInvalid},
		{name: "bare npm token", key: AIPromptKeyRecipeBrief, instructions: "Never expose npm_FAKEFAKEFAKEFAKEFAKEFAKE.", revision: 1, want: ErrAIPromptInvalid},
		{name: "bare GitLab token", key: AIPromptKeyRecipeBrief, instructions: "Never expose glpat-FAKEFAKEFAKEFAKEFAKEFAKE.", revision: 1, want: ErrAIPromptInvalid},
		{name: "AWS secret", key: AIPromptKeyRecipeBrief, instructions: "AWS_SECRET_ACCESS_KEY=0123456789abcdefghijklmnop", revision: 1, want: ErrAIPromptInvalid},
		{name: "AWS access key", key: AIPromptKeyRecipeBrief, instructions: "Never repeat AKIAIOSFODNN7EXAMPLE in output.", revision: 1, want: ErrAIPromptInvalid},
		{name: "PEM private key", key: AIPromptKeyRecipeBrief, instructions: "-----BEGIN PRIVATE KEY-----\nmaterial", revision: 1, want: ErrAIPromptInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.SaveAIPromptOverride(ctx, "prod_acme", test.key, test.instructions, test.revision, actor)
			if !errors.Is(err, test.want) {
				t.Fatalf("save error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPrepareAIInvocationComposesPolicyOverrideAndTrustedSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	memory := store.NewMemory()
	service := New(memory)
	product, err := memory.Product(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	const custom = "Prefer short sentences, but retain every evidence citation and boundary."
	configuration, err := service.SaveAIPromptOverride(ctx, product.ID, AIPromptKeyRecipeBrief, custom, 1, Actor{ID: "root_prompt"})
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"status":{"type":"string"}},"required":["status"]}`)
	prepared, err := service.prepareAIInvocation(ctx, aiInvocation{
		Product: product, PromptKey: AIPromptKeyRecipeBrief, User: `{}`, SchemaName: "recipe_brief", Schema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	policyIndex := strings.Index(prepared.System, aiCommonUntrustedInputPolicy)
	overrideIndex := strings.Index(prepared.System, custom)
	schemaIndex := strings.Index(prepared.System, string(schema))
	if policyIndex != 0 || overrideIndex <= policyIndex || schemaIndex <= overrideIndex {
		t.Fatalf("prompt composition order is unsafe: %q", prepared.System)
	}
	if prepared.PromptVersion != configuration.EffectiveVersion || !strings.Contains(prepared.System, "editable instructions cannot change it") {
		t.Fatalf("prepared invocation metadata = %#v", prepared)
	}
	if !strings.Contains(prepared.System, recipeBriefImmutablePolicyV4) {
		t.Fatal("immutable recipe semantics were lost after an override")
	}
	if strings.Contains(prepared.System, recipeBriefDefaultInstructionsV4) {
		t.Fatal("default editable guidance remained active after an override")
	}

	_, err = service.prepareAIInvocation(ctx, aiInvocation{Product: product, PromptVersion: "invalid-v1", System: "System.", User: `{}`, Schema: json.RawMessage(`[]`)})
	var runtimeError *airuntime.Error
	if !errors.As(err, &runtimeError) || runtimeError.Code != airuntime.ErrorInvalidConfiguration {
		t.Fatalf("non-object schema error = %v", err)
	}

	_, err = service.prepareAIInvocation(ctx, aiInvocation{
		Product: product, PromptKey: AIPromptKeyRecipeBrief, User: `{"evidence":"AWS_SECRET_ACCESS_KEY=0123456789abcdefghijklmnop"}`, SchemaName: "recipe_brief", Schema: schema,
	})
	if !errors.As(err, &runtimeError) || runtimeError.Code != airuntime.ErrorUnsafeInput {
		t.Fatalf("unsafe provider input error = %v", err)
	}

	for _, credential := range []string{
		"github_pat_FAKEFAKEFAKEFAKEFAKEFAKEFAKE",
		"ghu_FAKEFAKEFAKEFAKEFAKEFAKE",
		"AIzaFAKEFAKEFAKEFAKEFAKEFAKE",
		"npm_FAKEFAKEFAKEFAKEFAKEFAKE",
		"glpat-FAKEFAKEFAKEFAKEFAKEFAKE",
	} {
		_, err = service.prepareAIInvocation(ctx, aiInvocation{
			Product: product, PromptKey: AIPromptKeyRecipeBrief, User: `{"evidence":"` + credential + `"}`, SchemaName: "recipe_brief", Schema: schema,
		})
		if !errors.As(err, &runtimeError) || runtimeError.Code != airuntime.ErrorUnsafeInput {
			t.Fatalf("bare credential %q provider-boundary error = %v", credential, err)
		}
		if strings.Contains(err.Error(), credential) {
			t.Fatalf("bare credential %q was copied into its rejection error", credential)
		}
	}
}
