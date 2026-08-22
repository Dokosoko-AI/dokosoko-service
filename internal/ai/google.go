package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"google.golang.org/genai"
)

type GoogleAdapter struct {
	clientFactory HTTPClientFactory
}

func NewGoogleAdapter(factory HTTPClientFactory) *GoogleAdapter {
	if factory == nil {
		factory = FixedHTTPSHTTPClient
	}
	return &GoogleAdapter{clientFactory: factory}
}

func (a *GoogleAdapter) GenerateStructured(ctx context.Context, request StructuredRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	schema, err := permissiveObjectSchema(request.Schema)
	if err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, schema)
}

func (a *GoogleAdapter) GenerateText(ctx context.Context, request TextRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, nil)
}

func (a *GoogleAdapter) generate(ctx context.Context, provider ProviderConfig, model, system, user string, maxOutputTokens int, temperature float64, schema map[string]any) (Result, error) {
	httpClient, err := a.clientFactory(ctx, provider.Endpoint)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: provider.Credential, Backend: genai.BackendGeminiAPI, HTTPClient: httpClient, HTTPOptions: genai.HTTPOptions{BaseURL: strings.TrimRight(provider.Endpoint, "/") + "/", APIVersion: "v1beta"}})
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	maxOutput := int32(maxOutputTokens)
	config := &genai.GenerateContentConfig{SystemInstruction: genai.NewContentFromText(system, genai.RoleUser), MaxOutputTokens: maxOutput}
	if temperature > 0 {
		value := float32(temperature)
		config.Temperature = &value
	}
	if schema != nil {
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = schema
	}
	started := time.Now()
	response, err := client.Models.GenerateContent(ctx, model, genai.Text(user), config)
	duration := time.Since(started)
	if err != nil {
		var apiError genai.APIError
		if errors.As(err, &apiError) {
			return Result{}, nativeHTTPError(provider.Provider, apiError.Code, apiError.Status, apiError.Message, err)
		}
		return Result{}, nativeTransportError(provider.Provider, err)
	}
	if response == nil {
		return Result{}, &Error{Code: ErrorProviderUnavailable, Provider: provider.Provider, Retryable: true}
	}
	finishReason := ""
	if len(response.Candidates) > 0 && response.Candidates[0] != nil {
		finishReason = string(response.Candidates[0].FinishReason)
	}
	if finishErr := textFinishReason(finishReason); finishErr != nil {
		finishErr.(*Error).Provider = provider.Provider
		return Result{}, finishErr
	}
	text := strings.TrimSpace(response.Text())
	result := Result{Text: text, Provider: provider.Provider, RequestedModel: model, ResolvedModel: response.ModelVersion, RequestID: response.ResponseID, FinishReason: finishReason, Duration: duration}
	if response.UsageMetadata != nil {
		result.InputTokens = int64(response.UsageMetadata.PromptTokenCount)
		result.OutputTokens = int64(response.UsageMetadata.CandidatesTokenCount)
	}
	if schema != nil {
		result.JSON = json.RawMessage(text)
	}
	return result, nil
}

var _ Adapter = (*GoogleAdapter)(nil)
