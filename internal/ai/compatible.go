package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
)

const maxProviderResponse = 2 << 20

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ClientFactory func(context.Context, string) (HTTPDoer, *url.URL, error)

type HTTPClientFactory func(context.Context, string) (*http.Client, error)

type CompatibleAdapter struct {
	clientFactory  ClientFactory
	maxTokensField string
}

func NewCompatibleAdapter(factory ClientFactory) *CompatibleAdapter {
	if factory == nil {
		factory = FixedHTTPSClient
	}
	return &CompatibleAdapter{clientFactory: factory, maxTokensField: "max_tokens"}
}

// NewCompatibleAdapterWithMaxCompletionTokens is for OpenAI-compatible
// gateways that have retired max_tokens from their Chat Completions contract.
func NewCompatibleAdapterWithMaxCompletionTokens(factory ClientFactory) *CompatibleAdapter {
	adapter := NewCompatibleAdapter(factory)
	adapter.maxTokensField = "max_completion_tokens"
	return adapter
}

func (a *CompatibleAdapter) GenerateStructured(ctx context.Context, request StructuredRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, true)
}

func (a *CompatibleAdapter) GenerateText(ctx context.Context, request TextRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, false)
}

func (a *CompatibleAdapter) generate(ctx context.Context, provider ProviderConfig, model, system, user string, maxOutputTokens int, temperature float64, structured bool) (Result, error) {
	client, endpoint, err := a.clientFactory(ctx, provider.Endpoint)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	payload := map[string]any{
		"model":       model,
		"temperature": temperature,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	payload[a.maxTokensField] = maxOutputTokens
	if structured {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+provider.Credential)
	httpRequest.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := client.Do(httpRequest)
	duration := time.Since(started)
	if err != nil || response == nil || response.Body == nil {
		code := ErrorProviderUnavailable
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = ErrorTimeout
		}
		return Result{}, &Error{Code: code, Provider: provider.Provider, Retryable: true, Cause: err}
	}
	defer response.Body.Close()
	encoded, readErr := io.ReadAll(io.LimitReader(response.Body, maxProviderResponse+1))
	if readErr != nil || len(encoded) > maxProviderResponse {
		return Result{}, &Error{Code: ErrorProviderUnavailable, Provider: provider.Provider, Retryable: true, Cause: readErr}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, providerHTTPError(provider.Provider, response.StatusCode, encoded)
	}
	var completion struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(encoded, &completion) != nil || len(completion.Choices) == 0 {
		return Result{}, &Error{Code: ErrorProviderUnavailable, Provider: provider.Provider}
	}
	choice := completion.Choices[0]
	if choice.Message.Refusal != "" {
		return Result{}, &Error{Code: ErrorRefusedOutput, Provider: provider.Provider}
	}
	result := Result{Text: strings.TrimSpace(choice.Message.Content), Provider: provider.Provider, RequestedModel: model, ResolvedModel: completion.Model, RequestID: firstNonEmpty(response.Header.Get("x-request-id"), completion.ID), FinishReason: choice.FinishReason, InputTokens: completion.Usage.PromptTokens, OutputTokens: completion.Usage.CompletionTokens, Duration: duration}
	if structured {
		result.JSON = json.RawMessage(result.Text)
	}
	return result, nil
}

func providerHTTPError(provider string, status int, body []byte) error {
	code, retryable := ErrorProviderUnavailable, status >= 500
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = ErrorInvalidConfiguration
	case http.StatusUnauthorized, http.StatusForbidden:
		code = ErrorInvalidCredential
	case http.StatusNotFound:
		code = ErrorUnsupportedModel
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		code, retryable = ErrorTimeout, true
	case http.StatusTooManyRequests:
		code, retryable = ErrorRateLimited, true
	case http.StatusRequestEntityTooLarge:
		code = ErrorContextTooLarge
	}
	var providerError struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &providerError)
	value := strings.ToLower(providerError.Error.Code + " " + providerError.Error.Type)
	if strings.Contains(value, "model") || strings.Contains(value, "unsupported") || strings.Contains(value, "not_found") {
		code, retryable = ErrorUnsupportedModel, false
	}
	if strings.Contains(value, "authentication") || strings.Contains(value, "api_key") || strings.Contains(value, "permission") {
		code, retryable = ErrorInvalidCredential, false
	}
	if strings.Contains(value, "quota") || strings.Contains(value, "billing") {
		code, retryable = ErrorQuotaExhausted, false
	}
	if strings.Contains(value, "context") && strings.Contains(value, "length") {
		code, retryable = ErrorContextTooLarge, false
	}
	return &Error{Code: code, Provider: provider, Retryable: retryable}
}

func FixedHTTPSClient(ctx context.Context, raw string) (HTTPDoer, *url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return nil, nil, errors.New("provider endpoint must be a fixed HTTPS origin on the default port")
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, nil, errors.New("provider endpoint could not be resolved safely")
	}
	for _, address := range addresses {
		if netpolicy.UnsafeIP(address) {
			return nil, nil, errors.New("provider endpoint resolved to a disallowed network")
		}
	}
	endpoint := *parsed
	endpoint.Path = "/v1/chat/completions"
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()},
		DialContext: func(dialContext context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(dialContext, network, net.JoinHostPort(addresses[0].String(), "443"))
		},
	}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, &endpoint, nil
}

func FixedHTTPSHTTPClient(ctx context.Context, raw string) (*http.Client, error) {
	client, _, err := FixedHTTPSClient(ctx, raw)
	if err != nil {
		return nil, err
	}
	value, ok := client.(*http.Client)
	if !ok {
		return nil, errors.New("provider HTTP client has an unexpected type")
	}
	return value, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ Adapter = (*CompatibleAdapter)(nil)
