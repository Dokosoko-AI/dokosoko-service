package ai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
)

type responseDoer struct {
	status        int
	body          string
	authorization string
	requestBody   []byte
}

func (d *responseDoer) Do(request *http.Request) (*http.Response, error) {
	d.authorization = request.Header.Get("Authorization")
	d.requestBody, _ = io.ReadAll(request.Body)
	return &http.Response{StatusCode: d.status, Header: http.Header{"X-Request-Id": []string{"provider-request-1"}}, Body: io.NopCloser(bytes.NewBufferString(d.body))}, nil
}

func fixedTestFactory(doer HTTPDoer) ClientFactory {
	return func(context.Context, string) (HTTPDoer, *url.URL, error) {
		endpoint, _ := url.Parse("https://provider.example/v1/chat/completions")
		return doer, endpoint, nil
	}
}

func TestCompatibleAdapterNormalizesStructuredResultsWithoutCredentialLeakage(t *testing.T) {
	doer := &responseDoer{status: http.StatusOK, body: `{"id":"completion-1","model":"resolved-model","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":12,"completion_tokens":4}}`}
	registry := NewRegistry()
	registry.Register("openai-compatible", NewCompatibleAdapter(fixedTestFactory(doer)))
	result, err := registry.GenerateStructured(context.Background(), StructuredRequest{Provider: ProviderConfig{Provider: "openai-compatible", Endpoint: "https://provider.example", Credential: "provider-secret"}, Model: "requested-model", System: "Return JSON.", User: "Return ok.", SchemaName: "ok", MaxOutputTokens: 32})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.JSON) != `{"ok":true}` || result.ResolvedModel != "resolved-model" || result.RequestID != "provider-request-1" || result.InputTokens != 12 || result.OutputTokens != 4 {
		t.Fatalf("normalized result = %#v", result)
	}
	if doer.authorization != "Bearer provider-secret" || bytes.Contains(doer.requestBody, []byte("provider-secret")) {
		t.Fatalf("credential handling = auth %q body %s", doer.authorization, doer.requestBody)
	}
}

func TestCompatibleAdapterNormalizesProviderErrors(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		code   ErrorCode
	}{
		{http.StatusUnauthorized, `{}`, ErrorInvalidCredential},
		{http.StatusNotFound, `{}`, ErrorUnsupportedModel},
		{http.StatusTooManyRequests, `{"error":{"code":"rate_limit"}}`, ErrorRateLimited},
		{http.StatusBadRequest, `{"error":{"code":"context_length_exceeded"}}`, ErrorContextTooLarge},
		{http.StatusPaymentRequired, `{"error":{"code":"insufficient_quota"}}`, ErrorQuotaExhausted},
	} {
		doer := &responseDoer{status: test.status, body: test.body}
		adapter := NewCompatibleAdapter(fixedTestFactory(doer))
		_, err := adapter.GenerateText(context.Background(), TextRequest{Provider: ProviderConfig{Provider: "openai-compatible", Endpoint: "https://provider.example", Credential: "secret"}, Model: "model", System: "System", User: "User", MaxOutputTokens: 32})
		if Code(err) != test.code {
			t.Errorf("status %d code = %q, want %q", test.status, Code(err), test.code)
		}
	}
}

func TestFixedHTTPSClientRejectsUnsafeOriginsBeforeNetworkAccess(t *testing.T) {
	for _, endpoint := range []string{"http://provider.example", "https://user:pass@provider.example", "https://provider.example/path", "https://provider.example:8443", "https://127.0.0.1"} {
		if _, _, err := FixedHTTPSClient(context.Background(), endpoint); err == nil {
			t.Errorf("unsafe endpoint %q was accepted", endpoint)
		}
	}
}
