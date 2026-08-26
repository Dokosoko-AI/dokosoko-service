package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicAdapter struct {
	clientFactory HTTPClientFactory
}

func NewAnthropicAdapter(factory HTTPClientFactory) *AnthropicAdapter {
	if factory == nil {
		factory = FixedHTTPSHTTPClient
	}
	return &AnthropicAdapter{clientFactory: factory}
}

func (a *AnthropicAdapter) GenerateStructured(ctx context.Context, request StructuredRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	schema, err := permissiveObjectSchema(request.Schema)
	if err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, schema)
}

func (a *AnthropicAdapter) generate(ctx context.Context, provider ProviderConfig, model, system, user string, maxOutputTokens int, temperature float64, schema map[string]any) (Result, error) {
	httpClient, err := a.clientFactory(ctx, provider.Endpoint)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	httpClient, err = boundedNativeHTTPClient(httpClient)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	client := anthropicsdk.NewClient(
		anthropicoption.WithoutEnvironmentDefaults(),
		anthropicoption.WithAPIKey(provider.Credential),
		anthropicoption.WithBaseURL(strings.TrimRight(provider.Endpoint, "/")),
		anthropicoption.WithHTTPClient(httpClient),
		anthropicoption.WithMaxRetries(0),
	)
	params := anthropicsdk.MessageNewParams{
		MaxTokens: int64(maxOutputTokens),
		Messages:  []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(user))},
		Model:     anthropicsdk.Model(model),
		System:    []anthropicsdk.TextBlockParam{{Text: system}},
	}
	if temperature > 0 {
		params.Temperature = anthropicsdk.Float(temperature)
	}
	params.OutputConfig = anthropicsdk.OutputConfigParam{Format: anthropicsdk.JSONOutputFormatParam{Schema: schema}}
	started := time.Now()
	response, err := client.Messages.New(ctx, params)
	duration := time.Since(started)
	if err != nil {
		if errors.Is(err, errProviderResponseTooLarge) {
			return Result{}, nativeTransportError(provider.Provider, err)
		}
		var apiError *anthropicsdk.Error
		if errors.As(err, &apiError) {
			return Result{}, nativeHTTPError(provider.Provider, apiError.StatusCode, "", string(apiError.Type()), err)
		}
		return Result{}, nativeTransportError(provider.Provider, err)
	}
	if response == nil {
		return Result{}, &Error{Code: ErrorProviderUnavailable, Provider: provider.Provider, Retryable: true}
	}
	if finishErr := validateStructuredFinishReason(provider.Provider, string(response.StopReason), "end_turn"); finishErr != nil {
		return Result{}, finishErr
	}
	var output strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			output.WriteString(block.Text)
		}
	}
	text := strings.TrimSpace(output.String())
	result := Result{Text: text, Provider: provider.Provider, RequestedModel: model, ResolvedModel: string(response.Model), RequestID: response.ID, FinishReason: string(response.StopReason), InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, Duration: duration}
	result.JSON = json.RawMessage(text)
	return result, nil
}

var _ Adapter = (*AnthropicAdapter)(nil)
