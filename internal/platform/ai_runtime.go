package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	var nativeFactory airuntime.HTTPClientFactory
	if doer != nil {
		factory = func(_ context.Context, raw string) (airuntime.HTTPDoer, *url.URL, error) {
			if !validHTTPSBaseOrigin(strings.TrimRight(strings.TrimSpace(raw), "/")) {
				return nil, nil, errors.New("provider endpoint must be a fixed HTTPS origin")
			}
			endpoint, _ := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
			endpoint.Path = "/v1/chat/completions"
			return doer, endpoint, nil
		}
		nativeFactory = func(_ context.Context, raw string) (*http.Client, error) {
			if !validHTTPSBaseOrigin(strings.TrimRight(strings.TrimSpace(raw), "/")) {
				return nil, errors.New("provider endpoint must be a fixed HTTPS origin")
			}
			return &http.Client{Transport: productBuilderRoundTripper{doer: doer}}, nil
		}
	}
	compatible := airuntime.NewCompatibleAdapter(factory)
	digitalOcean := airuntime.NewCompatibleAdapterWithMaxCompletionTokens(factory)
	registry := airuntime.NewRegistry()
	registry.Register("openai", airuntime.NewOpenAIAdapter(nativeFactory))
	registry.Register("google", airuntime.NewGoogleAdapter(nativeFactory))
	registry.Register("anthropic", airuntime.NewAnthropicAdapter(nativeFactory))
	registry.Register("digitalocean", digitalOcean)
	registry.Register("xai", airuntime.NewOpenAIAdapter(nativeFactory))
	registry.Register("deepseek", compatible)
	registry.Register("openai-compatible", compatible)
	return registry
}

type productBuilderRoundTripper struct {
	doer ProductBuilderDoer
}

func (transport productBuilderRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport.doer.Do(request)
}

type aiInvocation struct {
	Product        model.Product
	Workload       airuntime.Workload
	Action         string
	PromptVersion  string
	System         string
	User           string
	SchemaName     string
	Schema         json.RawMessage
	MaxOutput      int
	Temperature    float64
	ActorKind      string
	ProviderRole   string
	FallbackReason string
	// DisableFallback keeps a consented or otherwise provider-bound payload
	// from being disclosed to a separately configured backup provider.
	DisableFallback                    bool
	ExpectedProviderConnectionID       string
	ExpectedProviderConnectionRevision int64
	ExpectedWorkloadProfileID          string
	ExpectedWorkloadProfileRevision    int64
}

// aiInvocationTargetMatches binds a consented invocation to the exact
// workload and provider records that were disclosed in its durable intent.
// Revisions are part of the boundary because a stable record ID can still be
// repointed to another model, endpoint, or credential.
func aiInvocationTargetMatches(invocation aiInvocation, profile model.AIWorkloadProfile, connection model.AIProviderConnection) bool {
	if invocation.ExpectedProviderConnectionID != "" && connection.ID != invocation.ExpectedProviderConnectionID {
		return false
	}
	if invocation.ExpectedProviderConnectionRevision > 0 && connection.Revision != invocation.ExpectedProviderConnectionRevision {
		return false
	}
	if invocation.ExpectedWorkloadProfileID != "" && profile.ID != invocation.ExpectedWorkloadProfileID {
		return false
	}
	if invocation.ExpectedWorkloadProfileRevision > 0 && profile.Revision != invocation.ExpectedWorkloadProfileRevision {
		return false
	}
	return true
}

func (s *Service) aiWorkloadTarget(ctx context.Context, product model.Product, workload airuntime.Workload) (model.AIWorkloadProfile, model.AIProviderConnection, error) {
	profile, err := s.store.AIWorkloadProfile(ctx, product.ID, string(workload))
	if err != nil || !profile.Enabled {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, ErrAIUnavailable
	}
	connection, err := s.store.AIProviderConnection(ctx, product.ID, profile.ProviderConnectionID)
	if err != nil || !connection.Enabled {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, ErrAIUnavailable
	}
	return profile, connection, nil
}

func (s *Service) aiWorkloadConfiguration(ctx context.Context, product model.Product, workload airuntime.Workload) (model.AIWorkloadProfile, model.AIProviderConnection, []byte, error) {
	profile, connection, err := s.aiWorkloadTarget(ctx, product, workload)
	if err != nil {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, err
	}
	credential, err := s.aiConnectionCredential(ctx, product, connection)
	if err != nil {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, err
	}
	return profile, connection, credential, nil
}

func (s *Service) aiBackupConfiguration(ctx context.Context, product model.Product, workload airuntime.Workload, primary model.AIProviderConnection, profile model.AIWorkloadProfile) (model.AIWorkloadProfile, model.AIProviderConnection, []byte, error) {
	connections, err := s.store.AIProviderConnections(ctx, product.ID)
	if err != nil {
		return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
	}
	for _, connection := range connections {
		if !connection.Enabled || !connection.IsBackup || connection.ID == primary.ID {
			continue
		}
		var models map[string]string
		if json.Unmarshal(connection.BackupModels, &models) != nil {
			return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
		}
		modelID := strings.TrimSpace(models[string(workload)])
		if modelID == "" {
			return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
		}
		credential, credentialErr := s.aiConnectionCredential(ctx, product, connection)
		if credentialErr != nil {
			return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, credentialErr
		}
		profile.ProviderConnectionID = connection.ID
		profile.Model = modelID
		return profile, connection, credential, nil
	}
	return model.AIWorkloadProfile{}, model.AIProviderConnection{}, nil, ErrAIUnavailable
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
	providerRole := invocation.ProviderRole
	if providerRole == "" {
		providerRole = "primary"
	}
	event := model.AIUsageEvent{ID: eventID, OrganisationID: invocation.Product.OrganisationID, ProductID: invocation.Product.ID, Workload: string(invocation.Workload), Action: invocation.Action, Provider: connection.Provider, ProviderRole: providerRole, FallbackReason: invocation.FallbackReason, RequestedModel: profile.Model, ResolvedModel: result.ResolvedModel, ProviderRequestID: result.RequestID, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens, Duration: result.Duration, DurationMS: result.Duration.Milliseconds(), Outcome: outcome, ErrorCode: errorCode, PromptVersion: invocation.PromptVersion, CreatedAt: s.now()}
	_ = s.store.FinishAIUsage(ctx, reservation.ID, event)
	if runErr == nil {
		_ = s.store.AppendAnalytics(ctx, model.AnalyticsEvent{OrganisationID: invocation.Product.OrganisationID, ProductID: invocation.Product.ID, EventName: "llm.tokens", ActorKind: invocation.ActorKind, Dimensions: map[string]any{"role": string(invocation.Workload), "workload": string(invocation.Workload), "action": invocation.Action, "provider": connection.Provider, "provider_role": providerRole, "model": profile.Model, "prompt_version": invocation.PromptVersion}, Value: float64(result.InputTokens + result.OutputTokens), CreatedAt: s.now()})
	}
}

func (s *Service) generateAIStructured(ctx context.Context, invocation aiInvocation) (airuntime.Result, error) {
	profile, connection, credential, err := s.aiWorkloadConfiguration(ctx, invocation.Product, invocation.Workload)
	if err != nil {
		return airuntime.Result{}, err
	}
	if !aiInvocationTargetMatches(invocation, profile, connection) {
		zeroBytes(credential)
		return airuntime.Result{}, ErrAIUnavailable
	}
	if invocation.MaxOutput <= 0 || invocation.MaxOutput > profile.MaxOutputTokens {
		invocation.MaxOutput = profile.MaxOutputTokens
	}
	run := func(activeProfile model.AIWorkloadProfile, activeConnection model.AIProviderConnection, activeCredential []byte, activeInvocation aiInvocation) (airuntime.Result, error) {
		reservation, reserveErr := s.reserveAI(ctx, activeInvocation, activeProfile)
		if reserveErr != nil {
			return airuntime.Result{}, reserveErr
		}
		result, runErr := s.aiRuntime.GenerateStructured(ctx, airuntime.StructuredRequest{Provider: airuntime.ProviderConfig{Provider: activeConnection.Provider, Endpoint: activeConnection.Endpoint, Credential: string(activeCredential)}, Model: activeProfile.Model, System: activeInvocation.System, User: activeInvocation.User, SchemaName: activeInvocation.SchemaName, Schema: activeInvocation.Schema, MaxOutputTokens: activeInvocation.MaxOutput, Temperature: activeInvocation.Temperature})
		s.finishAI(ctx, activeInvocation, reservation, activeConnection, activeProfile, result, runErr)
		return result, runErr
	}
	result, runErr := run(profile, connection, credential, invocation)
	zeroBytes(credential)
	if runErr == nil || !airuntime.Retryable(runErr) || invocation.DisableFallback {
		return result, runErr
	}
	backupProfile, backupConnection, backupCredential, backupErr := s.aiBackupConfiguration(ctx, invocation.Product, invocation.Workload, connection, profile)
	if backupErr != nil {
		return result, runErr
	}
	defer zeroBytes(backupCredential)
	backupInvocation := invocation
	backupInvocation.ProviderRole = "backup"
	backupInvocation.FallbackReason = string(airuntime.Code(runErr))
	return run(backupProfile, backupConnection, backupCredential, backupInvocation)
}

func (s *Service) generateAIText(ctx context.Context, invocation aiInvocation) (airuntime.Result, error) {
	profile, connection, credential, err := s.aiWorkloadConfiguration(ctx, invocation.Product, invocation.Workload)
	if err != nil {
		return airuntime.Result{}, err
	}
	if !aiInvocationTargetMatches(invocation, profile, connection) {
		zeroBytes(credential)
		return airuntime.Result{}, ErrAIUnavailable
	}
	if invocation.MaxOutput <= 0 || invocation.MaxOutput > profile.MaxOutputTokens {
		invocation.MaxOutput = profile.MaxOutputTokens
	}
	run := func(activeProfile model.AIWorkloadProfile, activeConnection model.AIProviderConnection, activeCredential []byte, activeInvocation aiInvocation) (airuntime.Result, error) {
		reservation, reserveErr := s.reserveAI(ctx, activeInvocation, activeProfile)
		if reserveErr != nil {
			return airuntime.Result{}, reserveErr
		}
		result, runErr := s.aiRuntime.GenerateText(ctx, airuntime.TextRequest{Provider: airuntime.ProviderConfig{Provider: activeConnection.Provider, Endpoint: activeConnection.Endpoint, Credential: string(activeCredential)}, Model: activeProfile.Model, System: activeInvocation.System, User: activeInvocation.User, MaxOutputTokens: activeInvocation.MaxOutput, Temperature: activeInvocation.Temperature})
		s.finishAI(ctx, activeInvocation, reservation, activeConnection, activeProfile, result, runErr)
		return result, runErr
	}
	result, runErr := run(profile, connection, credential, invocation)
	zeroBytes(credential)
	if runErr == nil || !airuntime.Retryable(runErr) || invocation.DisableFallback {
		return result, runErr
	}
	backupProfile, backupConnection, backupCredential, backupErr := s.aiBackupConfiguration(ctx, invocation.Product, invocation.Workload, connection, profile)
	if backupErr != nil {
		return result, runErr
	}
	defer zeroBytes(backupCredential)
	backupInvocation := invocation
	backupInvocation.ProviderRole = "backup"
	backupInvocation.FallbackReason = string(airuntime.Code(runErr))
	return run(backupProfile, backupConnection, backupCredential, backupInvocation)
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
	case "digitalocean":
		return "https://inference.do-ai.run"
	case "xai":
		return "https://api.x.ai"
	case "deepseek":
		return "https://api.deepseek.com"
	default:
		return ""
	}
}

func aiDefaultModel(provider string, workload airuntime.Workload) string {
	defaults := map[string]map[airuntime.Workload]string{
		"openai":    {airuntime.WorkloadAnalysis: "gpt-5.6-terra", airuntime.WorkloadAssistant: "gpt-5.6-luna"},
		"google":    {airuntime.WorkloadAnalysis: "gemini-3.5-flash", airuntime.WorkloadAssistant: "gemini-3.5-flash-lite"},
		"anthropic": {airuntime.WorkloadAnalysis: "claude-sonnet-5", airuntime.WorkloadAssistant: "claude-haiku-4-5"},
		"digitalocean": {
			airuntime.WorkloadAnalysis:  "openai-gpt-5.6-terra",
			airuntime.WorkloadAssistant: "openai-gpt-5.6-luna",
		},
		"xai": {
			airuntime.WorkloadAnalysis:  "grok-4.6",
			airuntime.WorkloadAssistant: "grok-4.3",
		},
		"deepseek": {
			airuntime.WorkloadAnalysis:  "deepseek-v4-pro",
			airuntime.WorkloadAssistant: "deepseek-v4-flash",
		},
	}
	return defaults[provider][workload]
}

func isAIStoreNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
