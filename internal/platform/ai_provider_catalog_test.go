package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

type providerCatalogDoer struct {
	request *http.Request
	body    string
	result  string
}

type providerCatalogFailoverDoer struct {
	requests []string
}

func (d *providerCatalogFailoverDoer) Do(request *http.Request) (*http.Response, error) {
	d.requests = append(d.requests, request.URL.String())
	status := http.StatusOK
	body := `{"id":"chat_backup","model":"backup-model","choices":[{"finish_reason":"stop","message":{"content":"{\"description\":\"Grounded backup provider answer.\"}"}}],"usage":{"prompt_tokens":8,"completion_tokens":4}}`
	if request.URL.Host == "primary.example.com" {
		status = http.StatusServiceUnavailable
		body = `{"error":{"type":"provider_error"}}`
	} else if request.URL.Host == "api.x.ai" {
		body = `{"id":"resp_backup","model":"grok-4.6","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"description\":\"Grounded backup provider answer.\"}","annotations":[]}]}],"usage":{"input_tokens":8,"output_tokens":4,"total_tokens":12}}`
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestFirstClassProvidersSaveWithFixedOrigins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	memory := store.NewMemory()
	vault, err := secretvault.New(bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithVault(memory, vault)
	product, err := memory.Product(ctx, "prod_acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"digitalocean", "xai", "deepseek"} {
		connection, saveErr := service.SaveAIProviderConnection(ctx, AIProviderConnectionInput{
			OrganisationID: product.OrganisationID,
			DeploymentID:   product.ID,
			Provider:       provider,
			Credential:     "provider-secret",
			Enabled:        true,
		}, Actor{ID: "root"})
		if saveErr != nil {
			t.Fatalf("save %s: %v", provider, saveErr)
		}
		if connection.Endpoint != aiProviderOrigin(provider) || connection.Provider != provider || connection.CredentialID == "" {
			t.Fatalf("saved %s connection = %#v", provider, connection)
		}
	}
}

func TestFirstClassProvidersCanServeAsAuditedBackups(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{"digitalocean", "xai", "deepseek"} {
		t.Run(provider, func(t *testing.T) {
			ctx := context.Background()
			memory := store.NewMemory()
			vault, err := secretvault.New(bytes.Repeat([]byte{0x62}, 32))
			if err != nil {
				t.Fatal(err)
			}
			doer := &providerCatalogFailoverDoer{}
			service := NewWithVaultAndProductBuilderDoer(memory, vault, doer)
			product, err := memory.Product(ctx, "prod_acme")
			if err != nil {
				t.Fatal(err)
			}
			primary, err := service.SaveAIProviderConnection(ctx, AIProviderConnectionInput{OrganisationID: product.OrganisationID, DeploymentID: product.ID, Provider: "openai-compatible", Endpoint: "https://primary.example.com", Credential: "primary-secret", Enabled: true}, Actor{ID: "root"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.SaveAIProviderConnection(ctx, AIProviderConnectionInput{OrganisationID: product.OrganisationID, DeploymentID: product.ID, Provider: provider, Credential: "backup-secret", Enabled: true, IsBackup: true, BackupModels: map[string]string{"analysis": aiDefaultModel(provider)}}, Actor{ID: "root"}); err != nil {
				t.Fatal(err)
			}
			if _, err = service.SaveAIWorkloadProfile(ctx, AIWorkloadProfileInput{OrganisationID: product.OrganisationID, ProductID: product.ID, Workload: "analysis", ProviderConnectionID: primary.ID, Model: "primary-analysis", MaxInputTokens: 4096, MaxOutputTokens: 1024, DailyTokenBudget: 10000, Enabled: true}, Actor{ID: "root"}); err != nil {
				t.Fatal(err)
			}

			description, err := service.RewriteProductDescription(ctx, product.ID, "Describe the API from known evidence.", Actor{ID: "root"})
			if err != nil || !strings.Contains(description, "backup provider") || len(doer.requests) != 2 {
				t.Fatalf("description=%q requests=%#v err=%v", description, doer.requests, err)
			}
			events, err := memory.AIUsageEvents(ctx, product.ID, time.Now().UTC().Add(-time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			foundBackup := false
			for _, event := range events {
				foundBackup = foundBackup || (event.Provider == provider && event.ProviderRole == "backup" && event.FallbackReason == "provider_unavailable" && event.Outcome == "succeeded")
			}
			if !foundBackup {
				t.Fatalf("missing audited %s backup event: %#v", provider, events)
			}
		})
	}
}

func (d *providerCatalogDoer) Do(request *http.Request) (*http.Response, error) {
	d.request = request
	encoded, _ := io.ReadAll(request.Body)
	d.body = string(encoded)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(d.result)),
		Request:    request,
	}, nil
}

func TestFirstClassCompatibleProviderCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider      string
		origin        string
		analysisModel string
		path          string
		result        string
	}{
		{provider: "digitalocean", origin: "https://inference.do-ai.run", analysisModel: "openai-gpt-5.6-terra", path: "/v1/chat/completions", result: `{"id":"chat_do","model":"openai-gpt-5.6-terra","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":8,"completion_tokens":3}}`},
		{provider: "xai", origin: "https://api.x.ai", analysisModel: "grok-4.6", path: "/v1/responses", result: `{"id":"resp_xai","model":"grok-4.6","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"ok\":true}","annotations":[]}]}],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}`},
		{provider: "deepseek", origin: "https://api.deepseek.com", analysisModel: "deepseek-v4-pro", path: "/v1/chat/completions", result: `{"id":"chat_deepseek","model":"deepseek-v4-pro","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":8,"completion_tokens":3}}`},
	}
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)

	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			doer := &providerCatalogDoer{result: test.result}
			runtime := newAIRuntime(doer)
			result, err := runtime.GenerateStructured(context.Background(), airuntime.StructuredRequest{
				Provider:        airuntime.ProviderConfig{Provider: test.provider, Endpoint: test.origin, Credential: "provider-secret"},
				Model:           test.analysisModel,
				System:          "Return JSON only.",
				User:            `Return {"ok":true}.`,
				SchemaName:      "provider_test",
				Schema:          schema,
				MaxOutputTokens: 256,
			})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.JSON) != `{"ok":true}` {
				t.Fatalf("structured result = %q", result.JSON)
			}
			if doer.request == nil || doer.request.URL.Host != strings.TrimPrefix(test.origin, "https://") || doer.request.URL.Path != test.path {
				t.Fatalf("request URL = %v, want %s%s", doer.request.URL, test.origin, test.path)
			}
			if doer.request.Header.Get("Authorization") != "Bearer provider-secret" || strings.Contains(doer.body, "provider-secret") {
				t.Fatal("provider credential was not confined to the authorization header")
			}
			if test.provider == "digitalocean" && (!strings.Contains(doer.body, `"max_completion_tokens":256`) || strings.Contains(doer.body, `"max_tokens"`)) {
				t.Fatalf("DigitalOcean token limit contract = %s", doer.body)
			}
			if test.provider == "deepseek" && !strings.Contains(doer.body, `"max_tokens":256`) {
				t.Fatalf("DeepSeek token limit contract = %s", doer.body)
			}
			if aiDefaultModel(test.provider) != test.analysisModel {
				t.Fatal("provider defaults do not match the catalog contract")
			}
		})
	}
}
