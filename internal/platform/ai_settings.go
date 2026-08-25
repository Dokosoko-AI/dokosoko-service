package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

var supportedAIProviders = map[string]bool{
	"openai":            true,
	"google":            true,
	"anthropic":         true,
	"digitalocean":      true,
	"xai":               true,
	"deepseek":          true,
	"openai-compatible": true,
}

type AIProviderConnectionInput struct {
	OrganisationID string
	DeploymentID   string
	Provider       string
	Endpoint       string
	Credential     string
	Enabled        bool
	IsBackup       bool
	BackupModels   map[string]string
	Revision       int64
}

func (s *Service) SaveAIProviderConnection(ctx context.Context, input AIProviderConnectionInput, actor Actor) (model.AIProviderConnection, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Endpoint = strings.TrimRight(strings.TrimSpace(input.Endpoint), "/")
	input.Credential = strings.TrimSpace(input.Credential)
	if !supportedAIProviders[input.Provider] {
		return model.AIProviderConnection{}, errors.New("choose a supported AI provider")
	}
	if input.Provider != "openai-compatible" {
		expected := aiProviderOrigin(input.Provider)
		if input.Endpoint == "" {
			input.Endpoint = expected
		}
		if input.Endpoint != expected {
			return model.AIProviderConnection{}, errors.New("native AI provider origins are fixed by DokoSoko")
		}
	}
	if !validHTTPSBaseOrigin(input.Endpoint) {
		return model.AIProviderConnection{}, errors.New("AI provider endpoint must be a fixed public HTTPS origin")
	}
	product, err := s.store.Product(ctx, input.DeploymentID)
	if err != nil || product.OrganisationID != input.OrganisationID {
		return model.AIProviderConnection{}, store.ErrNotFound
	}
	connections, err := s.store.AIProviderConnections(ctx, input.DeploymentID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return model.AIProviderConnection{}, err
	}
	var current model.AIProviderConnection
	for _, value := range connections {
		if value.Provider == input.Provider {
			current = value
			break
		}
	}
	if current.ID != "" && current.Revision != input.Revision {
		return model.AIProviderConnection{}, store.ErrConflict
	}
	if current.ManagedBy == "environment" {
		return model.AIProviderConnection{}, errors.New("this provider is managed by environment variables")
	}
	if input.IsBackup {
		for _, value := range connections {
			if value.IsBackup && value.ID != current.ID {
				return model.AIProviderConnection{}, errors.New("only one backup provider can be active")
			}
		}
		profiles, profileErr := s.store.AIWorkloadProfiles(ctx, input.DeploymentID)
		if profileErr != nil && !errors.Is(profileErr, store.ErrNotFound) {
			return model.AIProviderConnection{}, profileErr
		}
		for _, profile := range profiles {
			if current.ID != "" && profile.ProviderConnectionID == current.ID {
				return model.AIProviderConnection{}, errors.New("choose another primary provider before making this connection the backup")
			}
		}
	}
	connectionID := current.ID
	if connectionID == "" {
		connectionID, err = randomUUID()
		if err != nil {
			return model.AIProviderConnection{}, err
		}
	}
	credentialID := current.CredentialID
	if input.Credential != "" {
		if s.vault == nil {
			return model.AIProviderConnection{}, errors.New("AI credential encryption is not configured")
		}
		credentialID, err = randomUUID()
		if err != nil {
			return model.AIProviderConnection{}, err
		}
		encrypted, err := s.vault.Encrypt([]byte(input.Credential), input.OrganisationID+":ai:"+credentialID)
		if err != nil {
			return model.AIProviderConnection{}, err
		}
		if _, err = s.store.CreateSecret(ctx, model.Secret{ID: credentialID, OrganisationID: input.OrganisationID, Name: "ai-provider-" + input.Provider + "-" + credentialID, Purpose: "ai_provider", Ciphertext: encrypted.Ciphertext, Nonce: encrypted.Nonce, KeyVersion: encrypted.KeyVersion, Fingerprint: encrypted.Fingerprint}); err != nil {
			return model.AIProviderConnection{}, err
		}
	}
	if input.Enabled && credentialID == "" {
		return model.AIProviderConnection{}, errors.New("enter a provider API credential before enabling this connection")
	}
	backupModels := map[string]string{}
	if input.IsBackup {
		if !input.Enabled {
			return model.AIProviderConnection{}, errors.New("enable a provider before using it as backup")
		}
		for _, workload := range []airuntime.Workload{airuntime.WorkloadAnalysis, airuntime.WorkloadAssistant} {
			modelID := strings.TrimSpace(input.BackupModels[string(workload)])
			if modelID == "" {
				modelID = aiDefaultModel(input.Provider, workload)
			}
			if modelID == "" || len(modelID) > 160 || strings.IndexFunc(modelID, func(value rune) bool { return value < 0x20 || value == 0x7f }) >= 0 {
				return model.AIProviderConnection{}, errors.New("choose valid Analysis and Assistant models for the backup provider")
			}
			backupModels[string(workload)] = modelID
		}
	}
	backupModelsJSON, _ := json.Marshal(backupModels)
	value, err := s.store.SaveAIProviderConnection(ctx, model.AIProviderConnection{ID: connectionID, OrganisationID: input.OrganisationID, DeploymentID: input.DeploymentID, Provider: input.Provider, Endpoint: input.Endpoint, CredentialID: credentialID, ManagedBy: "console", Enabled: input.Enabled, IsBackup: input.IsBackup, BackupModels: backupModelsJSON, LastTestedAt: current.LastTestedAt, LastErrorCode: current.LastErrorCode}, input.Revision)
	if err != nil {
		return model.AIProviderConnection{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.DeploymentID, ActorID: actor.ID, Action: "ai.provider.saved", TargetType: "ai_provider_connection", TargetID: value.ID, Current: map[string]any{"provider": value.Provider, "managed_by": value.ManagedBy, "enabled": value.Enabled, "is_backup": value.IsBackup, "backup_models": backupModels, "credential_rotated": input.Credential != ""}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.AIProviderConnection{}, err
	}
	return value, nil
}

type AIWorkloadProfileInput struct {
	OrganisationID       string
	ProductID            string
	Workload             string
	ProviderConnectionID string
	Model                string
	MaxInputTokens       int
	MaxOutputTokens      int
	DailyTokenBudget     int64
	Enabled              bool
	Revision             int64
}

func (s *Service) SaveAIWorkloadProfile(ctx context.Context, input AIWorkloadProfileInput, actor Actor) (model.AIWorkloadProfile, error) {
	input.Workload = strings.ToLower(strings.TrimSpace(input.Workload))
	input.Model = strings.TrimSpace(input.Model)
	if !airuntime.ValidWorkload(input.Workload) {
		return model.AIWorkloadProfile{}, errors.New("AI workload must be analysis or assistant")
	}
	if input.MaxInputTokens < 256 || input.MaxInputTokens > 1_000_000 || input.MaxOutputTokens < 1 || input.MaxOutputTokens > 32_768 || input.DailyTokenBudget < 0 || input.DailyTokenBudget > 10_000_000_000 {
		return model.AIWorkloadProfile{}, errors.New("AI token limits or daily budget are outside supported bounds")
	}
	product, err := s.store.Product(ctx, input.ProductID)
	if err != nil || product.OrganisationID != input.OrganisationID {
		return model.AIWorkloadProfile{}, store.ErrNotFound
	}
	connection, err := s.store.AIProviderConnection(ctx, input.ProductID, input.ProviderConnectionID)
	if err != nil {
		return model.AIWorkloadProfile{}, err
	}
	if input.Enabled && !connection.Enabled {
		return model.AIWorkloadProfile{}, errors.New("enable the provider connection before enabling this workload")
	}
	if connection.IsBackup {
		return model.AIWorkloadProfile{}, errors.New("the backup provider cannot also be selected as a primary workload provider")
	}
	if input.Model == "" {
		input.Model = aiDefaultModel(connection.Provider, airuntime.Workload(input.Workload))
	}
	if input.Model == "" || len(input.Model) > 160 || strings.IndexFunc(input.Model, func(value rune) bool { return value < 0x20 || value == 0x7f }) >= 0 {
		return model.AIWorkloadProfile{}, errors.New("AI model ID is invalid")
	}
	current, err := s.store.AIWorkloadProfile(ctx, input.ProductID, input.Workload)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return model.AIWorkloadProfile{}, err
	}
	if current.ID != "" && current.Revision != input.Revision {
		return model.AIWorkloadProfile{}, store.ErrConflict
	}
	profileID := current.ID
	if profileID == "" {
		profileID, err = randomUUID()
		if err != nil {
			return model.AIWorkloadProfile{}, err
		}
	}
	hardening := json.RawMessage(`{"context_is_untrusted":true,"tool_calls_disabled":true,"authorization_disabled":true,"require_citations":true,"no_answer_on_low_confidence":true}`)
	value, err := s.store.SaveAIWorkloadProfile(ctx, model.AIWorkloadProfile{ID: profileID, OrganisationID: input.OrganisationID, ProductID: input.ProductID, Workload: input.Workload, ProviderConnectionID: connection.ID, Model: input.Model, MaxInputTokens: input.MaxInputTokens, MaxOutputTokens: input.MaxOutputTokens, DailyTokenBudget: input.DailyTokenBudget, Hardening: hardening, Enabled: input.Enabled}, input.Revision)
	if err != nil {
		return model.AIWorkloadProfile{}, err
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: input.OrganisationID, ProductID: input.ProductID, ActorID: actor.ID, Action: "ai.workload.saved", TargetType: "ai_workload_profile", TargetID: value.ID, Current: map[string]any{"workload": value.Workload, "provider_connection_id": value.ProviderConnectionID, "model": value.Model, "enabled": value.Enabled, "daily_token_budget": value.DailyTokenBudget}, RequestID: actor.RequestID, CreatedAt: s.now()}); err != nil {
		return model.AIWorkloadProfile{}, err
	}
	return value, nil
}

type AIEnvironmentConfig struct {
	Provider string
	APIKey   string
	Endpoint string
	Models   map[airuntime.Workload]string
}

func (s *Service) ConfigureEnvironmentAI(ctx context.Context, config AIEnvironmentConfig) error {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Endpoint = strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if config.Provider == "" && config.APIKey == "" {
		return nil
	}
	if !supportedAIProviders[config.Provider] || config.APIKey == "" {
		return errors.New("DOKOSOKO_AI_PROVIDER and DOKOSOKO_AI_API_KEY must name one supported provider")
	}
	if config.Endpoint == "" {
		config.Endpoint = aiProviderOrigin(config.Provider)
	}
	if config.Provider != "openai-compatible" && config.Endpoint != aiProviderOrigin(config.Provider) {
		return errors.New("native environment-managed AI providers use their fixed origin")
	}
	if !validHTTPSBaseOrigin(config.Endpoint) {
		return errors.New("DOKOSOKO_AI_ENDPOINT must be a fixed HTTPS origin")
	}
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return err
	}
	connections, err := s.store.AIProviderConnections(ctx, deployment.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	var current model.AIProviderConnection
	for _, value := range connections {
		if value.Provider == config.Provider {
			current = value
			break
		}
	}
	connectionID := current.ID
	if connectionID == "" {
		connectionID, err = randomUUID()
		if err != nil {
			return err
		}
	}
	connection, err := s.store.SaveAIProviderConnection(ctx, model.AIProviderConnection{ID: connectionID, OrganisationID: deployment.OrganisationID, DeploymentID: deployment.ID, Provider: config.Provider, Endpoint: config.Endpoint, ManagedBy: "environment", Enabled: true, BackupModels: json.RawMessage(`{}`), LastTestedAt: current.LastTestedAt, LastErrorCode: current.LastErrorCode}, current.Revision)
	if err != nil {
		return err
	}
	s.aiEnvironmentCredentials[config.Provider] = config.APIKey
	for _, workload := range []airuntime.Workload{airuntime.WorkloadAnalysis, airuntime.WorkloadAssistant} {
		currentProfile, profileErr := s.store.AIWorkloadProfile(ctx, deployment.ID, string(workload))
		if profileErr != nil && !errors.Is(profileErr, store.ErrNotFound) {
			return profileErr
		}
		profileID := currentProfile.ID
		if profileID == "" {
			profileID, err = randomUUID()
			if err != nil {
				return err
			}
		}
		modelID := strings.TrimSpace(config.Models[workload])
		if modelID == "" {
			modelID = aiDefaultModel(config.Provider, workload)
		}
		maxOutput := 4096
		if workload == airuntime.WorkloadAssistant {
			maxOutput = 1024
		}
		hardening := json.RawMessage(`{"context_is_untrusted":true,"tool_calls_disabled":true,"authorization_disabled":true,"require_citations":true,"no_answer_on_low_confidence":true}`)
		if _, err = s.store.SaveAIWorkloadProfile(ctx, model.AIWorkloadProfile{ID: profileID, OrganisationID: deployment.OrganisationID, ProductID: deployment.ID, Workload: string(workload), ProviderConnectionID: connection.ID, Model: modelID, MaxInputTokens: 128000, MaxOutputTokens: maxOutput, DailyTokenBudget: 0, Hardening: hardening, Enabled: true}, currentProfile.Revision); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) TestAIProviderConnection(ctx context.Context, deploymentID, connectionID string, actor Actor) (model.AIProviderConnection, error) {
	product, err := s.store.Product(ctx, deploymentID)
	if err != nil {
		return model.AIProviderConnection{}, err
	}
	connection, err := s.store.AIProviderConnection(ctx, deploymentID, connectionID)
	if err != nil {
		return model.AIProviderConnection{}, err
	}
	modelID := ""
	profiles, profileErr := s.store.AIWorkloadProfiles(ctx, deploymentID)
	if profileErr != nil && !errors.Is(profileErr, store.ErrNotFound) {
		return model.AIProviderConnection{}, profileErr
	}
	for _, profile := range profiles {
		if profile.ProviderConnectionID != connection.ID || strings.TrimSpace(profile.Model) == "" {
			continue
		}
		if modelID == "" || profile.Workload == string(airuntime.WorkloadAnalysis) {
			modelID = profile.Model
		}
		if profile.Workload == string(airuntime.WorkloadAnalysis) {
			break
		}
	}
	if connection.IsBackup {
		var backupModels map[string]string
		_ = json.Unmarshal(connection.BackupModels, &backupModels)
		if candidate := strings.TrimSpace(backupModels[string(airuntime.WorkloadAnalysis)]); candidate != "" {
			modelID = candidate
		} else if candidate := strings.TrimSpace(backupModels[string(airuntime.WorkloadAssistant)]); candidate != "" {
			modelID = candidate
		}
	}
	if modelID == "" {
		modelID = aiDefaultModel(connection.Provider, airuntime.WorkloadAnalysis)
	}
	if modelID == "" {
		return connection, errors.New("configure a workload model before testing an OpenAI-compatible provider")
	}
	credential, err := s.aiConnectionCredential(ctx, product, connection)
	if err != nil {
		return model.AIProviderConnection{}, err
	}
	defer zeroBytes(credential)
	connectionTestSchema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	_, testErr := s.aiRuntime.GenerateStructured(ctx, airuntime.StructuredRequest{Provider: airuntime.ProviderConfig{Provider: connection.Provider, Endpoint: connection.Endpoint, Credential: string(credential)}, Model: modelID, System: "Return only the JSON object requested by the user. Do not call tools.", User: `Return {"ok":true}.`, SchemaName: "connection_test", Schema: connectionTestSchema, MaxOutputTokens: 256})
	now := s.now()
	connection.LastTestedAt = &now
	connection.LastErrorCode = ""
	if testErr != nil {
		connection.LastErrorCode = string(airuntime.Code(testErr))
	}
	updated, saveErr := s.store.SaveAIProviderConnection(ctx, connection, connection.Revision)
	if saveErr != nil {
		return model.AIProviderConnection{}, saveErr
	}
	if err := s.store.AppendAudit(ctx, model.AuditEvent{ID: randomID("audit"), OrganisationID: product.OrganisationID, ProductID: product.ID, ActorID: actor.ID, Action: "ai.provider.tested", TargetType: "ai_provider_connection", TargetID: connection.ID, Current: map[string]any{"provider": connection.Provider, "outcome": map[bool]string{true: "failed", false: "succeeded"}[testErr != nil], "error_code": connection.LastErrorCode}, RequestID: actor.RequestID, CreatedAt: now}); err != nil {
		return model.AIProviderConnection{}, err
	}
	if testErr != nil {
		return updated, testErr
	}
	return updated, nil
}
