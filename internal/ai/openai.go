package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type OpenAIAdapter struct {
	clientFactory HTTPClientFactory
}

func NewOpenAIAdapter(factory HTTPClientFactory) *OpenAIAdapter {
	if factory == nil {
		factory = FixedHTTPSHTTPClient
	}
	return &OpenAIAdapter{clientFactory: factory}
}

func (a *OpenAIAdapter) GenerateStructured(ctx context.Context, request StructuredRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	schema, err := permissiveObjectSchema(request.Schema)
	if err != nil {
		return Result{}, err
	}
	name := strings.TrimSpace(request.SchemaName)
	if name == "" {
		name = "result"
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, &responses.ResponseTextConfigParam{Format: responses.ResponseFormatTextConfigParamOfJSONSchema(name, schema)})
}

func (a *OpenAIAdapter) GenerateText(ctx context.Context, request TextRequest) (Result, error) {
	if err := ValidateRequest(request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens); err != nil {
		return Result{}, err
	}
	return a.generate(ctx, request.Provider, request.Model, request.System, request.User, request.MaxOutputTokens, request.Temperature, nil)
}

func (a *OpenAIAdapter) generate(ctx context.Context, provider ProviderConfig, model, system, user string, maxOutputTokens int, temperature float64, textConfig *responses.ResponseTextConfigParam) (Result, error) {
	httpClient, err := a.clientFactory(ctx, provider.Endpoint)
	if err != nil {
		return Result{}, &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider, Cause: err}
	}
	client := openaisdk.NewClient(
		openaioption.WithAPIKey(provider.Credential),
		openaioption.WithBaseURL(strings.TrimRight(provider.Endpoint, "/")+"/v1"),
		openaioption.WithHTTPClient(httpClient),
		openaioption.WithMaxRetries(0),
	)
	params := responses.ResponseNewParams{
		Instructions:    openaisdk.String(system),
		Input:           responses.ResponseNewParamsInputUnion{OfString: openaisdk.String(user)},
		MaxOutputTokens: openaisdk.Int(int64(maxOutputTokens)),
		Model:           shared.ResponsesModel(model),
		Store:           openaisdk.Bool(false),
	}
	if temperature > 0 {
		params.Temperature = openaisdk.Float(temperature)
	}
	if textConfig != nil {
		params.Text = *textConfig
	}
	started := time.Now()
	response, err := client.Responses.New(ctx, params)
	duration := time.Since(started)
	if err != nil {
		var apiError *openaisdk.Error
		if errors.As(err, &apiError) {
			return Result{}, nativeHTTPError(provider.Provider, apiError.StatusCode, apiError.Code, apiError.Type, err)
		}
		return Result{}, nativeTransportError(provider.Provider, err)
	}
	if response == nil {
		return Result{}, &Error{Code: ErrorProviderUnavailable, Provider: provider.Provider, Retryable: true}
	}
	if finishErr := textFinishReason(firstNonEmpty(response.IncompleteDetails.Reason, string(response.Status))); finishErr != nil {
		finishErr.(*Error).Provider = provider.Provider
		return Result{}, finishErr
	}
	text := strings.TrimSpace(response.OutputText())
	result := Result{Text: text, Provider: provider.Provider, RequestedModel: model, ResolvedModel: string(response.Model), RequestID: response.ID, FinishReason: string(response.Status), InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, Duration: duration}
	if textConfig != nil {
		result.JSON = json.RawMessage(text)
	}
	return result, nil
}

var _ Adapter = (*OpenAIAdapter)(nil)
