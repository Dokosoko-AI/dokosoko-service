package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type countingProviderBody struct {
	remaining int64
	read      int64
}

func (body *countingProviderBody) Read(buffer []byte) (int, error) {
	if body.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(buffer)) > body.remaining {
		buffer = buffer[:body.remaining]
	}
	for index := range buffer {
		buffer[index] = 'x'
	}
	body.remaining -= int64(len(buffer))
	body.read += int64(len(buffer))
	return len(buffer), nil
}

func (*countingProviderBody) Close() error { return nil }

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

func TestNativeProvidersRejectIncompleteAndUnknownStructuredResults(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		adapter  Adapter
	}{
		{
			name:     "openai truncated",
			provider: "openai",
			adapter:  NewOpenAIAdapter(fixtureHTTPFactory(`{"id":"resp_1","model":"gpt-test","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"ok\":true}","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`, nil)),
		},
		{
			name:     "openai unknown",
			provider: "openai",
			adapter:  NewOpenAIAdapter(fixtureHTTPFactory(`{"id":"resp_1","model":"gpt-test","status":"future_status","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"{\"ok\":true}","annotations":[]}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`, nil)),
		},
		{
			name:     "anthropic truncated",
			provider: "anthropic",
			adapter:  NewAnthropicAdapter(fixtureHTTPFactory(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"ok\":true}"}],"stop_reason":"max_tokens","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":3}}`, nil)),
		},
		{
			name:     "anthropic unknown",
			provider: "anthropic",
			adapter:  NewAnthropicAdapter(fixtureHTTPFactory(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"ok\":true}"}],"stop_reason":"future_reason","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":3}}`, nil)),
		},
		{
			name:     "google truncated",
			provider: "google",
			adapter:  NewGoogleAdapter(fixtureHTTPFactory(`{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"ok\":true}"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13},"modelVersion":"gemini-test","responseId":"response-1"}`, nil)),
		},
		{
			name:     "google unknown",
			provider: "google",
			adapter:  NewGoogleAdapter(fixtureHTTPFactory(`{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"ok\":true}"}]},"finishReason":"FUTURE_REASON"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13},"modelVersion":"gemini-test","responseId":"response-1"}`, nil)),
		},
	}
	request := StructuredRequest{Model: "fixture-model", System: "Return JSON.", User: "Return an object.", SchemaName: "result", Schema: json.RawMessage(`{"type":"object"}`), MaxOutputTokens: 16}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request.Provider = ProviderConfig{Provider: test.provider, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := test.adapter.GenerateStructured(context.Background(), request)
			if Code(err) != ErrorInvalidStructuredOutput || !Retryable(err) {
				t.Fatalf("finish reason was not rejected with bounded failover eligibility: err=%v code=%q retryable=%t", err, Code(err), Retryable(err))
			}
		})
	}
}

func TestStructuredSafetyAndRefusalReasonsRemainTerminal(t *testing.T) {
	for _, reason := range []string{"refusal", "safety", "image_safety", "prohibited_content", "blocklist", "content_filter", "recitation", "image_recitation", "spii"} {
		err := validateStructuredFinishReason("provider", reason, "stop")
		if Code(err) != ErrorRefusedOutput || Retryable(err) {
			t.Fatalf("safety reason %q = %v (code %q, retryable %v)", reason, err, Code(err), Retryable(err))
		}
	}
}

func TestNativeProviderSafetySignalsRemainTerminal(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		adapter  Adapter
	}{
		{
			name:     "openai refusal content",
			provider: "openai",
			adapter:  NewOpenAIAdapter(fixtureHTTPFactory(`{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"refusal","refusal":"safety policy"}]}],"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`, nil)),
		},
		{
			name:     "anthropic refusal stop",
			provider: "anthropic",
			adapter:  NewAnthropicAdapter(fixtureHTTPFactory(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":"refusal","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}`, nil)),
		},
		{
			name:     "google safety finish",
			provider: "google",
			adapter:  NewGoogleAdapter(fixtureHTTPFactory(`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"SAFETY"}],"modelVersion":"gemini-test","responseId":"response-1"}`, nil)),
		},
		{
			name:     "google prompt block",
			provider: "google",
			adapter:  NewGoogleAdapter(fixtureHTTPFactory(`{"candidates":[],"promptFeedback":{"blockReason":"JAILBREAK"},"modelVersion":"gemini-test","responseId":"response-1"}`, nil)),
		},
	}
	request := StructuredRequest{Model: "fixture-model", System: "Return JSON.", User: "Return an object.", SchemaName: "result", Schema: json.RawMessage(`{"type":"object"}`), MaxOutputTokens: 16}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request.Provider = ProviderConfig{Provider: test.provider, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := test.adapter.GenerateStructured(context.Background(), request)
			if Code(err) != ErrorRefusedOutput || Retryable(err) {
				t.Fatalf("safety result = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
		})
	}
}

func TestNativeProvidersBoundResponseBodiesBeforeSDKDecoding(t *testing.T) {
	providers := []struct {
		name    string
		adapter func(HTTPClientFactory) Adapter
	}{
		{name: "openai", adapter: func(factory HTTPClientFactory) Adapter { return NewOpenAIAdapter(factory) }},
		{name: "anthropic", adapter: func(factory HTTPClientFactory) Adapter { return NewAnthropicAdapter(factory) }},
		{name: "google", adapter: func(factory HTTPClientFactory) Adapter { return NewGoogleAdapter(factory) }},
	}
	request := StructuredRequest{Model: "fixture-model", System: "Return JSON.", User: "Return an object.", SchemaName: "result", Schema: json.RawMessage(`{"type":"object"}`), MaxOutputTokens: 16}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			body := &countingProviderBody{remaining: 4 * maxProviderResponse}
			factory := func(context.Context, string) (*http.Client, error) {
				return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: request}, nil
				})}, nil
			}
			request.Provider = ProviderConfig{Provider: provider.name, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := provider.adapter(factory).GenerateStructured(context.Background(), request)
			if Code(err) != ErrorProviderUnavailable || !Retryable(err) {
				t.Fatalf("oversized response error = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
			if body.read > maxProviderResponse+1 {
				t.Fatalf("SDK read %d bytes, limit is %d", body.read, maxProviderResponse)
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
	request := StructuredRequest{Model: "fixture-model", System: "Return JSON.", User: "Return an object.", SchemaName: "result", Schema: json.RawMessage(`{"type":"object"}`), MaxOutputTokens: 16}
	for _, provider := range providers {
		t.Run(provider.name+"_rate_limit", func(t *testing.T) {
			adapter := provider.rateLimit(fixtureHTTPErrorFactory(http.StatusTooManyRequests, rateLimitBodies[provider.name]))
			request.Provider = ProviderConfig{Provider: provider.name, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := adapter.GenerateStructured(context.Background(), request)
			if Code(err) != ErrorRateLimited || !Retryable(err) {
				t.Fatalf("normalized rate limit = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
		})
		t.Run(provider.name+"_timeout", func(t *testing.T) {
			adapter := provider.rateLimit(fixtureHTTPTransportErrorFactory(context.DeadlineExceeded))
			request.Provider = ProviderConfig{Provider: provider.name, Endpoint: "https://provider.example", Credential: "provider-secret"}
			_, err := adapter.GenerateStructured(context.Background(), request)
			if Code(err) != ErrorTimeout || !Retryable(err) {
				t.Fatalf("normalized timeout = %v (code %q, retryable %v)", err, Code(err), Retryable(err))
			}
		})
	}
}

func TestProviderHTTPErrorDoesNotTreatBadRequestsAsOutages(t *testing.T) {
	t.Parallel()

	if err := providerHTTPError("openai", http.StatusBadRequest, []byte(`{"error":{"type":"invalid_request_error","code":"invalid_json_schema"}}`)); Code(err) != ErrorInvalidConfiguration || Retryable(err) {
		t.Fatalf("invalid request normalized as %q (retryable %v)", Code(err), Retryable(err))
	}
	if err := providerHTTPError("openai", http.StatusBadRequest, []byte(`{"error":{"type":"invalid_request_error","code":"model_not_found"}}`)); Code(err) != ErrorUnsupportedModel || Retryable(err) {
		t.Fatalf("missing model normalized as %q (retryable %v)", Code(err), Retryable(err))
	}
}
