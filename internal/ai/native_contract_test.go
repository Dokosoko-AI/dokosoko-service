package ai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func fixtureHTTPFactory(body string, inspect func(*http.Request, []byte)) HTTPClientFactory {
	return func(context.Context, string) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestBody, _ := io.ReadAll(request.Body)
			if inspect != nil {
				inspect(request, requestBody)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})}, nil
	}
}

func fixtureHTTPErrorFactory(status int, body string) HTTPClientFactory {
	return func(context.Context, string) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})}, nil
	}
}

func fixtureHTTPTransportErrorFactory(err error) HTTPClientFactory {
	return func(context.Context, string) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, err })}, nil
	}
}

func TestNativeProvidersSatisfyOneStructuredContract(t *testing.T) {
	tests := []struct {
		provider string
		adapter  Adapter
	}{
		{
			provider: "openai",
			adapter: NewOpenAIAdapter(fixtureHTTPFactory(`{"id":"resp_1","model":"gpt-test-resolved","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"ok\":true}","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`, func(request *http.Request, body []byte) {
				if request.Header.Get("Authorization") != "Bearer provider-secret" || bytes.Contains(body, []byte("provider-secret")) || request.URL.Path != "/v1/responses" {
					t.Errorf("OpenAI request was not credential-safe: %s %s", request.URL.Path, body)
				}
			})),
		},
		{
			provider: "anthropic",
			adapter: NewAnthropicAdapter(fixtureHTTPFactory(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test-resolved","content":[{"type":"text","text":"{\"ok\":true}"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":3}}`, func(request *http.Request, body []byte) {
				if request.Header.Get("X-Api-Key") != "provider-secret" || bytes.Contains(body, []byte("provider-secret")) || request.URL.Path != "/v1/messages" {
					t.Errorf("Anthropic request was not credential-safe: %s %s", request.URL.Path, body)
				}
			})),
		},
		{
			provider: "google",
			adapter: NewGoogleAdapter(fixtureHTTPFactory(`{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"ok\":true}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13},"modelVersion":"gemini-test-resolved","responseId":"response-1"}`, func(request *http.Request, body []byte) {
				if request.Header.Get("X-Goog-Api-Key") != "provider-secret" || bytes.Contains(body, []byte("provider-secret")) || !strings.Contains(request.URL.Path, ":generateContent") {
					t.Errorf("Google request was not credential-safe: %s %s", request.URL.Path, body)
				}
			})),
		},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			registry := NewRegistry()
			registry.Register(test.provider, test.adapter)
			result, err := registry.GenerateStructured(context.Background(), StructuredRequest{Provider: ProviderConfig{Provider: test.provider, Endpoint: "https://provider.example", Credential: "provider-secret"}, Model: "requested-model", System: "Return JSON.", User: "Return ok.", SchemaName: "ok", Schema: []byte(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`), MaxOutputTokens: 32})
			if err != nil {
				t.Fatal(err)
			}
			if string(result.JSON) != `{"ok":true}` || result.InputTokens != 10 || result.OutputTokens != 3 || result.ResolvedModel == "" || result.RequestID == "" {
				t.Fatalf("normalized result = %#v", result)
			}
		})
	}
}

func TestNativeProvidersNormalizeRateLimitsAndTimeouts(t *testing.T) {
	providers := []struct {
		name      string
		rateLimit func(HTTPClientFactory) Adapter
	}{
		{name: "openai", rateLimit: func(factory HTTPClientFactory) Adapter { return NewOpenAIAdapter(factory) }},
		{name: "anthropic", rateLimit: func(factory HTTPClientFactory) Adapter { return NewAnthropicAdapter(factory) }},
		{name: "google", rateLimit: func(factory HTTPClientFactory) Adapter { return NewGoogleAdapter(factory) }},
	}
	rateLimitBodies := map[string]string{
		"openai":    `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
		"anthropic": `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`,
		"google":    `{"error":{"code":429,"message":"slow down","status":"RESOURCE_EXHAUSTED"}}`,
	}
	request := TextRequest{Model: "fixture-model", System: "Return text.", User: "Hello.", MaxOutputTokens: 16}
	for _, provider := range providers {
		t.Run(provider.name+"_rate_limit", func(t *testing.T) {
			adapter := provider.rateLimit(fixtureHTTPErrorFactory(http.StatusTooManyRequests, rateLimitBodies[provider.name]))
			request.Provider = ProviderConfig{Provider: provider.name, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := adapter.GenerateText(context.Background(), request)
			if Code(err) != ErrorRateLimited || !Retryable(err) {
				t.Fatalf("normalized rate limit = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
		})
		t.Run(provider.name+"_timeout", func(t *testing.T) {
			adapter := provider.rateLimit(fixtureHTTPTransportErrorFactory(context.DeadlineExceeded))
			request.Provider = ProviderConfig{Provider: provider.name, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := adapter.GenerateText(context.Background(), request)
			if Code(err) != ErrorTimeout || !Retryable(err) {
				t.Fatalf("normalized timeout = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
		})
	}
}
