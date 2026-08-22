package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

func (r *Registry) Register(provider string, adapter Adapter) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "" && adapter != nil {
		r.adapters[provider] = adapter
	}
}

func (r *Registry) adapter(provider string) (Adapter, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	adapter, ok := r.adapters[provider]
	if !ok {
		return nil, &Error{Code: ErrorInvalidConfiguration, Provider: provider}
	}
	return adapter, nil
}

func (r *Registry) GenerateStructured(ctx context.Context, request StructuredRequest) (Result, error) {
	adapter, err := r.adapter(request.Provider.Provider)
	if err != nil {
		return Result{}, err
	}
	result, err := adapter.GenerateStructured(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if len(result.JSON) == 0 {
		result.JSON = json.RawMessage(strings.TrimSpace(result.Text))
	}
	if !json.Valid(result.JSON) {
		return Result{}, &Error{Code: ErrorInvalidStructuredOutput, Provider: request.Provider.Provider}
	}
	return result, nil
}

func (r *Registry) GenerateText(ctx context.Context, request TextRequest) (Result, error) {
	adapter, err := r.adapter(request.Provider.Provider)
	if err != nil {
		return Result{}, err
	}
	return adapter.GenerateText(ctx, request)
}

func (r *Registry) StreamText(ctx context.Context, request TextRequest) (TextStream, error) {
	result, err := r.GenerateText(ctx, request)
	if err != nil {
		return nil, err
	}
	return &singleResultStream{result: result}, nil
}

type singleResultStream struct {
	result Result
	read   bool
	err    error
}

func (s *singleResultStream) Next() bool {
	if s.read || s.err != nil {
		return false
	}
	s.read = true
	return true
}

func (s *singleResultStream) Result() Result { return s.result }
func (s *singleResultStream) Err() error     { return s.err }
func (s *singleResultStream) Close() error {
	s.read = true
	return nil
}

var _ Runtime = (*Registry)(nil)

func ValidateRequest(provider ProviderConfig, model, system, user string, maxOutputTokens int) error {
	if strings.TrimSpace(provider.Provider) == "" || strings.TrimSpace(provider.Endpoint) == "" || strings.TrimSpace(provider.Credential) == "" || strings.TrimSpace(model) == "" {
		return &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider}
	}
	if strings.TrimSpace(system) == "" || strings.TrimSpace(user) == "" || maxOutputTokens < 1 || maxOutputTokens > 32768 {
		return &Error{Code: ErrorInvalidConfiguration, Provider: provider.Provider}
	}
	return nil
}

func IsContextError(err error) bool {
	var value *Error
	return errors.As(err, &value) && value.Code == ErrorContextTooLarge
}
