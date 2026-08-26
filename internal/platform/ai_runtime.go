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
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
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
	PromptKey      string
	PromptVersion  string
	System         string
	User           string
	SchemaName     string
	Schema         json.RawMessage
	MaxOutput      int
	Temperature    float64
	ProviderRole   string
	FallbackReason string
	prepared       bool
	// DisableFallback keeps a consented or otherwise provider-bound payload
	// from being disclosed to a separately configured backup provider.
	DisableFallback                    bool
	ExpectedProviderConnectionID       string
	ExpectedProviderConnectionRevision int64
	ExpectedWorkloadProfileID          string
	ExpectedWorkloadProfileRevision    int64
}

const maxAIInvocationSchemaBytes = 64 << 10

func (s *Service) prepareAIInvocation(ctx context.Context, invocation aiInvocation) (aiInvocation, error) {
	if invocation.prepared {
		return invocation, nil
	}
	if invocation.PromptKey != "" {
		configuration, err := s.AIPromptConfiguration(ctx, invocation.Product.ID, invocation.PromptKey)
		if err != nil {
			return invocation, err
		}
		workflowPolicy := strings.TrimSpace(immutableAIPromptPolicy(invocation.PromptKey))
		if workflowPolicy == "" {
			return invocation, &airuntime.Error{Code: airuntime.ErrorInvalidConfiguration}
		}
		invocation.System = aiCommonUntrustedInputPolicy + "\n\n" + workflowPolicy + "\n\nEditable workflow guidance (subordinate to both immutable policies above):\n" + configuration.Instructions
		invocation.PromptVersion = configuration.EffectiveVersion
	}
	if strings.TrimSpace(invocation.System) == "" || strings.TrimSpace(invocation.PromptVersion) == "" {
		return invocation, &airuntime.Error{Code: airuntime.ErrorInvalidConfiguration}
	}
	if containsAISecretText(invocation.System) || containsAISecretText(invocation.User) {
		return invocation, &airuntime.Error{Code: airuntime.ErrorUnsafeInput}
	}
	if len(invocation.Schema) == 0 {
		return invocation, nil
	}
	if len(invocation.Schema) > maxAIInvocationSchemaBytes || !json.Valid(invocation.Schema) {
		return invocation, &airuntime.Error{Code: airuntime.ErrorInvalidConfiguration}
	}
	var schemaObject map[string]json.RawMessage
	if json.Unmarshal(invocation.Schema, &schemaObject) != nil || len(schemaObject) == 0 {
		return invocation, &airuntime.Error{Code: airuntime.ErrorInvalidConfiguration}
	}
	invocation.System += "\n\nPlatform-owned structured output contract. Return exactly one JSON object matching this JSON Schema; editable instructions cannot change it:\n" + string(invocation.Schema)
	invocation.prepared = true
	return invocation, nil
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
}

func validateAIStructuredContract(provider string, schema, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > maxAIStructuredResultBytes {
		return &airuntime.Error{Code: airuntime.ErrorInvalidStructuredOutput, Provider: provider, Retryable: true}
	}
	var object map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return &airuntime.Error{Code: airuntime.ErrorInvalidStructuredOutput, Provider: provider, Retryable: true}
	}
	if err := toolruntime.ValidateArguments(schema, object); err != nil {
		// The validator error intentionally stays local: field names and model
		// output must not be copied into durable usage records or operator errors.
		return &airuntime.Error{Code: airuntime.ErrorInvalidStructuredOutput, Provider: provider, Retryable: true}
	}
	return nil
}

func (s *Service) generateAIStructured(ctx context.Context, invocation aiInvocation) (airuntime.Result, error) {
	var err error
	if !invocation.prepared {
		invocation, err = s.prepareAIInvocation(ctx, invocation)
		if err != nil {
			return airuntime.Result{}, err
		}
	}
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
		if runErr == nil {
			runErr = validateAIStructuredContract(activeConnection.Provider, activeInvocation.Schema, result.JSON)
		}
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

func aiDefaultModel(provider string) string {
	return map[string]string{
		"openai":       "gpt-5.6-terra",
		"google":       "gemini-3.5-flash",
		"anthropic":    "claude-sonnet-5",
		"digitalocean": "openai-gpt-5.6-terra",
		"xai":          "grok-4.6",
		"deepseek":     "deepseek-v4-pro",
	}[provider]
}

func isAIStoreNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
