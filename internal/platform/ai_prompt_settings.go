package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/store"
)

const (
	AIPromptKeyIntegrationAnalysis = "integration.analysis"
	AIPromptKeyRecipeBrief         = "recipe.brief"
	AIPromptKeyRecipeAuthoring     = "recipe.authoring"
	AIPromptKeyRecipeReview        = "recipe.review"

	maxAIPromptInstructionsBytes = 32 << 10
)

var ErrAIPromptInvalid = errors.New("AI prompt configuration is invalid")

type aiPromptDefinition struct {
	key            string
	label          string
	description    string
	instructions   string
	defaultVersion string
}

var aiPromptDefinitions = []aiPromptDefinition{
	{
		key:            AIPromptKeyIntegrationAnalysis,
		label:          "Integration analysis",
		description:    "Plans the smallest evidence-grounded integration and recipe set from reviewed catalog and documentation evidence.",
		instructions:   editableAIPromptBody(integrationAnalysisSystemPromptV2),
		defaultVersion: integrationAnalysisPromptVersionV2,
	},
	{
		key:            AIPromptKeyRecipeBrief,
		label:          "Recipe brief",
		description:    "Maps one operator outcome to a narrow, reviewable recipe brief.",
		instructions:   editableAIPromptBody(recipeBriefSystemPromptV2),
		defaultVersion: recipeBriefPromptVersionV2,
	},
	{
		key:            AIPromptKeyRecipeAuthoring,
		label:          "Recipe authoring",
		description:    "Writes one evidence-grounded Markdown recipe from server-owned selections.",
		instructions:   editableAIPromptBody(recipeAuthoringSystemPromptV8),
		defaultVersion: recipeAuthoringPromptVersionV8,
	},
	{
		key:            AIPromptKeyRecipeReview,
		label:          "Recipe review",
		description:    "Adversarially checks a draft for unsupported claims and missing boundaries.",
		instructions:   editableAIPromptBody(recipeReviewSystemPromptV2),
		defaultVersion: recipeReviewPromptVersionV2,
	},
}

func editableAIPromptBody(systemPrompt string) string {
	body, ok := strings.CutPrefix(systemPrompt, aiCommonUntrustedInputPolicy)
	if !ok {
		panic("AI workflow prompt is missing the immutable safety policy")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		panic("AI workflow prompt has no editable instructions")
	}
	return body
}

func aiPromptDefinitionForKey(key string) (aiPromptDefinition, bool) {
	for _, definition := range aiPromptDefinitions {
		if definition.key == key {
			return definition, true
		}
	}
	return aiPromptDefinition{}, false
}

func effectiveAIPromptConfiguration(definition aiPromptDefinition, state model.AIPromptState, persisted bool) model.AIPromptConfiguration {
	configuration := model.AIPromptConfiguration{
		Key:              definition.key,
		Label:            definition.label,
		Description:      definition.description,
		Instructions:     definition.instructions,
		DefaultVersion:   definition.defaultVersion,
		EffectiveVersion: definition.defaultVersion,
		Source:           "default",
		Revision:         1,
	}
	if !persisted {
		return configuration
	}
	configuration.Revision = state.Revision
	updatedAt := state.UpdatedAt
	configuration.UpdatedAt = &updatedAt
	if state.Instructions != "" {
		configuration.Instructions = state.Instructions
		configuration.EffectiveVersion = fmt.Sprintf("%s+override.%d", definition.defaultVersion, state.Revision)
		configuration.Source = "override"
	}
	return configuration
}

func (s *Service) AIPromptConfigurations(ctx context.Context, productID string) ([]model.AIPromptConfiguration, error) {
	productID = strings.TrimSpace(productID)
	if _, err := s.store.Product(ctx, productID); err != nil {
		return nil, err
	}
	states, err := s.store.AIPromptStates(ctx, productID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	byKey := make(map[string]model.AIPromptState, len(states))
	for _, state := range states {
		if _, supported := aiPromptDefinitionForKey(state.Key); !supported {
			return nil, fmt.Errorf("unsupported persisted AI prompt key %q", state.Key)
		}
		byKey[state.Key] = state
	}
	result := make([]model.AIPromptConfiguration, 0, len(aiPromptDefinitions))
	for _, definition := range aiPromptDefinitions {
		state, persisted := byKey[definition.key]
		result = append(result, effectiveAIPromptConfiguration(definition, state, persisted))
	}
	return result, nil
}

func (s *Service) AIPromptConfiguration(ctx context.Context, productID, key string) (model.AIPromptConfiguration, error) {
	productID = strings.TrimSpace(productID)
	key = strings.TrimSpace(key)
	definition, supported := aiPromptDefinitionForKey(key)
	if !supported {
		return model.AIPromptConfiguration{}, store.ErrNotFound
	}
	if _, err := s.store.Product(ctx, productID); err != nil {
		return model.AIPromptConfiguration{}, err
	}
	state, err := s.store.AIPromptState(ctx, productID, key)
	if errors.Is(err, store.ErrNotFound) {
		return effectiveAIPromptConfiguration(definition, model.AIPromptState{}, false), nil
	}
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	return effectiveAIPromptConfiguration(definition, state, true), nil
}

func normalizeAIPromptInstructions(instructions string) (string, error) {
	instructions = strings.ReplaceAll(instructions, "\r\n", "\n")
	instructions = strings.ReplaceAll(instructions, "\r", "\n")
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return "", fmt.Errorf("%w: instructions are required", ErrAIPromptInvalid)
	}
	if len(instructions) > maxAIPromptInstructionsBytes {
		return "", fmt.Errorf("%w: instructions must be no larger than 32 KiB", ErrAIPromptInvalid)
	}
	if !utf8.ValidString(instructions) {
		return "", fmt.Errorf("%w: instructions must be valid UTF-8", ErrAIPromptInvalid)
	}
	if strings.IndexFunc(instructions, func(value rune) bool {
		return unicode.IsControl(value) && value != '\n' && value != '\t'
	}) >= 0 {
		return "", fmt.Errorf("%w: instructions contain unsupported control characters", ErrAIPromptInvalid)
	}
	if containsAISecretText(instructions) {
		return "", fmt.Errorf("%w: instructions must not contain credentials or secret values", ErrAIPromptInvalid)
	}
	return instructions, nil
}

func validateAIPromptMutation(productID, key string, expectedRevision int64) (string, string, aiPromptDefinition, error) {
	productID = strings.TrimSpace(productID)
	key = strings.TrimSpace(key)
	definition, supported := aiPromptDefinitionForKey(key)
	if !supported {
		return "", "", aiPromptDefinition{}, store.ErrNotFound
	}
	if expectedRevision < 1 {
		return "", "", aiPromptDefinition{}, fmt.Errorf("%w: revision must be at least 1", ErrAIPromptInvalid)
	}
	return productID, key, definition, nil
}

func (s *Service) SaveAIPromptOverride(ctx context.Context, productID, key, instructions string, expectedRevision int64, actor Actor) (model.AIPromptConfiguration, error) {
	productID, key, definition, err := validateAIPromptMutation(productID, key, expectedRevision)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	instructions, err = normalizeAIPromptInstructions(instructions)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	prior, err := s.AIPromptConfiguration(ctx, productID, key)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	if prior.Revision != expectedRevision {
		return model.AIPromptConfiguration{}, store.ErrConflict
	}
	if prior.Instructions == instructions {
		return prior, nil
	}
	predicted := model.AIPromptState{ProductID: productID, Key: key, Instructions: instructions, Revision: expectedRevision + 1}
	current := effectiveAIPromptConfiguration(definition, predicted, true)
	state, err := s.store.SaveAIPromptStateAndAudit(ctx, model.AIPromptState{ProductID: productID, Key: key, Instructions: instructions}, expectedRevision, model.AuditEvent{
		ID:             randomID("audit"),
		OrganisationID: product.OrganisationID,
		ProductID:      product.ID,
		ActorID:        actor.ID,
		Action:         "ai.prompt.saved",
		TargetType:     "ai_prompt",
		TargetID:       key,
		Prior:          aiPromptAuditState(prior),
		Current:        aiPromptAuditState(current),
		RequestID:      actor.RequestID,
		CreatedAt:      s.now(),
	})
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	return effectiveAIPromptConfiguration(definition, state, true), nil
}

func (s *Service) ResetAIPromptOverride(ctx context.Context, productID, key string, expectedRevision int64, actor Actor) (model.AIPromptConfiguration, error) {
	productID, key, definition, err := validateAIPromptMutation(productID, key, expectedRevision)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	prior, err := s.AIPromptConfiguration(ctx, productID, key)
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	if prior.Revision != expectedRevision {
		return model.AIPromptConfiguration{}, store.ErrConflict
	}
	if prior.Source == "default" {
		return prior, nil
	}
	predicted := model.AIPromptState{ProductID: productID, Key: key, Revision: expectedRevision + 1}
	current := effectiveAIPromptConfiguration(definition, predicted, true)
	state, err := s.store.SaveAIPromptStateAndAudit(ctx, model.AIPromptState{ProductID: productID, Key: key}, expectedRevision, model.AuditEvent{
		ID:             randomID("audit"),
		OrganisationID: product.OrganisationID,
		ProductID:      product.ID,
		ActorID:        actor.ID,
		Action:         "ai.prompt.reset",
		TargetType:     "ai_prompt",
		TargetID:       key,
		Prior:          aiPromptAuditState(prior),
		Current:        aiPromptAuditState(current),
		RequestID:      actor.RequestID,
		CreatedAt:      s.now(),
	})
	if err != nil {
		return model.AIPromptConfiguration{}, err
	}
	return effectiveAIPromptConfiguration(definition, state, true), nil
}

func aiPromptAuditState(configuration model.AIPromptConfiguration) map[string]any {
	fingerprint := sha256.Sum256([]byte(configuration.Instructions))
	return map[string]any{
		"key":                 configuration.Key,
		"source":              configuration.Source,
		"revision":            configuration.Revision,
		"default_version":     configuration.DefaultVersion,
		"effective_version":   configuration.EffectiveVersion,
		"instructions_bytes":  len(configuration.Instructions),
		"instructions_sha256": hex.EncodeToString(fingerprint[:]),
	}
}
