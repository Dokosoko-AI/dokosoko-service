package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	secretvault "github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var ErrAIUnavailable = errors.New("AI workload is unavailable")

func newAIRuntime(doer ProductBuilderDoer) airuntime.Runtime {
	var factory airuntime.ClientFactory
	if doer != nil {
		factory = func(_ context.Context, raw string) (airuntime.HTTPDoer, *url.URL, error) {
			if !validHTTPSBaseOrigin(strings.TrimRight(strings.TrimSpace(raw), "/")) {
				return nil, nil, errors.New("provider endpoint must be a fixed HTTPS origin")
			}
			endpoint, _ := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
			endpoint.Path = "/v1/chat/completions"
			return doer, endpoint, nil
		}
	}
	compatible := airuntime.NewCompatibleAdapter(factory)
	registry := airuntime.NewRegistry()
	registry.Register("openai", airuntime.NewOpenAIAdapter(nil))
	registry.Register("google", airuntime.NewGoogleAdapter(nil))
	registry.Register("anthropic", airuntime.NewAnthropicAdapter(nil))
	registry.Register("openai-compatible", compatible)
	return registry
}

type aiInvocation struct {
	Product       model.Product
	Workload      airuntime.Workload
	Action        string
	PromptVersion string
	System        string
	User          string
	SchemaName    string
	Schema        json.RawMessage
	MaxOutput     int
	Temperature   float64
	ActorKind     string
}

func (s *Service) aiWorkloadConfiguration(ctx context.Context, product model.Product, workload airuntime.Workload) (model.AIWorkloadProfile, model.AIProviderConnection, []byte, error) {
	profile, err := s.store.AIWorkloadProfile(ctx, product.ID, string(workload))
	if err != nil || !profile.Enabled {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
	}
	connection, err := s.store.AIProviderConnection(ctx, product.ID, profile.ProviderConnectionID)
	if err != nil || !connection.Enabled {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
	}
	credential, err := s.aiConnectionCredential(ctx, product, connection)
	if err != nil {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, err
	}
	return profile, connection, credential, nil
}

func (s *Service) aiConnectionCredential(ctx context.Context, product model.Product, connection model.AIProviderConnection) ([]byte, error) {
	if connection.ManagedBy == "environment" {
		if credential := strings.TrimSpace(s.aiEnvironmentCredentials[connection.Provider]); credential != "" {
			return []byte(credential), nil
		}
		return nil, ErrAIUnavailable
	}
	if s.vault == nil || connection.CredentialID == "" {
		return nil, ErrAIUnavailable
	}
	secret, err := s.store.Secret(ctx, product.OrganisationID, connection.CredentialID)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	credential, err := s.vault.Decrypt(secretvault.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, Fingerprint: secret.Fingerprint, KeyVersion: secret.KeyVersion}, product.OrganisationID+":ai:"+connection.CredentialID)
	if err != nil {
		// Existing llm_profiles used this authenticated-encryption context. The
		// fallback is migration-only and can be removed after credentials rotate.
		credential, err = s.vault.Decrypt(secretvault.Encrypted{Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, Fingerprint: secret.Fingerprint, KeyVersion: secret.KeyVersion}, product.OrganisationID+":llm:"+connection.CredentialID)
	}
	if err != nil {
		return nil, ErrAIUnavailable
	}
	return credential, nil
}

func (s *Service) reserveAI(ctx context.Context, invocation aiInvocation, profile model.AIWorkloadProfile) (model.AIBudgetReservation, error) {
	inputEstimate := estimateAITokens(invocation.System + "\n" + invocation.User)
	if profile.MaxInputTokens > 0 && inputEstimate > int64(profile.MaxInputTokens) {
		return model.AIBudgetReservation{}, &airuntime.Error{Code: airuntime.ErrorContextTooLarge}
	}
	reservationID, err := randomUUID()
	if err != nil {
		return model.AIBudgetReservation{}, err
	}
	now := s.now().UTC()
	value := model.AIBudgetReservation{ID: reservationID, ProductID: invocation.Product.ID, Workload: string(invocation.Workload), Day: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), ReservedTokens: inputEstimate + int64(invocation.MaxOutput), ExpiresAt: now.Add(2 * time.Minute)}
	ok, err := s.store.ReserveAIBudget(ctx, value, profile.DailyTokenBudget)
	if err != nil {
		return model.AIBudgetReservation{}, err
	}
	if !ok {
		return model.AIBudgetReservation{}, &airuntime.Error{Code: airuntime.ErrorBudgetExhausted}
	}
	return value, nil
}

func (s *Service) finishAI(ctx context.Context, invocation aiInvocation, reservation model.AIBudgetReservation, connection model.AIProviderConnection, profile model.AIWorkloadProfile, result airuntime.Result, runErr error) {
	outcome, errorCode := "succeeded", ""
	if runErr != nil {
		outcome, errorCode = "failed", string(airuntime.Code(runErr))
	}
	if runErr == nil && result.InputTokens+result.OutputTokens == 0 {
		result.InputTokens = estimateAITokens(invocation.System + "\n" + invocation.User)
		result.OutputTokens = estimateAITokens(result.Text)
	}
	eventID, _ := randomUUID()
	event := model.AIUsageEvent{ID: eventID, OrganisationID: invocation.Product.OrganisationID, ProductID: invocation.Product.ID, Workload: string(invocation.Workload), Action: invocation.Action, Provider: connection.Provider, RequestedModel: profile.Model, ResolvedModel: result.ResolvedModel, ProviderRequestID: result.RequestID, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens, Duration: result.Duration, DurationMS: result.Duration.Milliseconds(), Outcome: outcome, ErrorCode: errorCode, PromptVersion: invocation.PromptVersion, CreatedAt: s.now()}
	_ = s.store.FinishAIUsage(ctx, reservation.ID, event)
	if runErr == nil {
		legacyRole := string(invocation.Workload)
		if invocation.Workload == airuntime.WorkloadSupport || invocation.Workload == airuntime.WorkloadAuthoring {
			legacyRole = "assistant"
		} else if invocation.Workload == airuntime.WorkloadReview {
			legacyRole = "evaluation"
		}
		_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: invocation.Product.OrganisationID, ProductID: invocation.Product.ID, EventName: "llm.tokens", ActorKind: invocation.ActorKind, Dimensions: map[string]any{"role": legacyRole, "workload": invocation.Workload, "action": invocation.Action, "provider": connection.Provider, "model": profile.Model, "prompt_version": invocation.PromptVersion}, Value: float64(result.InputTokens + result.OutputTokens), CreatedAt: s.now()})
	}
}

func (s *Service) generateAIStructured(ctx context.Context, invocation aiInvocation) (airuntime.Result, error) {
	profile, connection, credential, err := s.aiWorkloadConfiguration(ctx, invocation.Product, invocation.Workload)
	if err != nil {
		return airuntime.Result{}, err
	}
	defer zeroBytes(credential)
	if invocation.MaxOutput <= 0 || invocation.MaxOutput > profile.MaxOutputTokens {
		invocation.MaxOutput = profile.MaxOutputTokens
	}
	reservation, err := s.reserveAI(ctx, invocation, profile)
	if err != nil {
		return airuntime.Result{}, err
	}
	result, runErr := s.aiRuntime.GenerateStructured(ctx, airuntime.StructuredRequest{Provider: airuntime.ProviderConfig{Provider: connection.Provider, Endpoint: connection.Endpoint, Credential: string(credential)}, Model: profile.Model, System: invocation.System, User: invocation.User, SchemaName: invocation.SchemaName, Schema: invocation.Schema, MaxOutputTokens: invocation.MaxOutput, Temperature: invocation.Temperature})
	s.finishAI(ctx, invocation, reservation, connection, profile, result, runErr)
	return result, runErr
}

func (s *Service) generateAIText(ctx context.Context, invocation aiInvocation) (airuntime.Result, error) {
	profile, connection, credential, err := s.aiWorkloadConfiguration(ctx, invocation.Product, invocation.Workload)
	if err != nil {
		return airuntime.Result{}, err
	}
	defer zeroBytes(credential)
	if invocation.MaxOutput <= 0 || invocation.MaxOutput > profile.MaxOutputTokens {
		invocation.MaxOutput = profile.MaxOutputTokens
	}
	reservation, err := s.reserveAI(ctx, invocation, profile)
	if err != nil {
		return airuntime.Result{}, err
	}
	result, runErr := s.aiRuntime.GenerateText(ctx, airuntime.TextRequest{Provider: airuntime.ProviderConfig{Provider: connection.Provider, Endpoint: connection.Endpoint, Credential: string(credential)}, Model: profile.Model, System: invocation.System, User: invocation.User, MaxOutputTokens: invocation.MaxOutput, Temperature: invocation.Temperature})
	s.finishAI(ctx, invocation, reservation, connection, profile, result, runErr)
	return result, runErr
}

func estimateAITokens(value string) int64 {
	if value == "" {
		return 0
	}
	return int64((len(value) + 3) / 4)
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func aiProviderOrigin(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com"
	case "google":
		return "https://generativelanguage.googleapis.com"
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return ""
	}
}

func aiDefaultModel(provider string, workload airuntime.Workload) string {
	defaults := map[string]map[airuntime.Workload]string{
		"openai":    {airuntime.WorkloadExtraction: "gpt-5.6-luna", airuntime.WorkloadAuthoring: "gpt-5.6-terra", airuntime.WorkloadReview: "gpt-5.6-sol", airuntime.WorkloadSupport: "gpt-5.6-terra"},
		"google":    {airuntime.WorkloadExtraction: "gemini-3.5-flash-lite", airuntime.WorkloadAuthoring: "gemini-3.6-flash", airuntime.WorkloadReview: "gemini-3.5-flash", airuntime.WorkloadSupport: "gemini-3.6-flash"},
		"anthropic": {airuntime.WorkloadExtraction: "claude-haiku-4-5", airuntime.WorkloadAuthoring: "claude-sonnet-5", airuntime.WorkloadReview: "claude-opus-5", airuntime.WorkloadSupport: "claude-sonnet-5"},
	}
	return defaults[provider][workload]
}

func isAIStoreNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
