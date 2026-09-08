package platform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/model"
)

// AISetupError retains errors.Is(ErrAIUnavailable) for existing callers while
// telling setup and processing exactly which local prerequisite is missing.
type AISetupError struct{ Code string }

func (e *AISetupError) Error() string { return "AI setup required: " + e.Code }
func (e *AISetupError) Unwrap() error { return ErrAIUnavailable }

// AIProcessingReadiness does not invoke a provider or reserve tokens. It describes
// whether a request can be attempted; the runtime still checks and reserves the
// exact input/output budget before each actual call.
func (s *Service) AIProcessingReadiness(ctx context.Context) (model.AIProcessingReadiness, error) {
	deployment, err := s.store.Deployment(ctx)
	if err != nil {
		return model.AIProcessingReadiness{}, err
	}
	product, err := s.store.Product(ctx, deployment.ID)
	if err != nil {
		return model.AIProcessingReadiness{}, err
	}
	now := s.now().UTC()
	value := model.AIProcessingReadiness{Workload: "analysis", CheckedAt: now, Blockers: []string{}}
	profile, connection, err := s.aiWorkloadTarget(ctx, product, ai.WorkloadAnalysis)
	value.Model, value.ProviderConnectionID = profile.Model, profile.ProviderConnectionID
	value.Provider, value.ManagedBy = connection.Provider, connection.ManagedBy
	// A provider test predating the selected workload revision cannot establish
	// that the current model settings were tested.
	if connection.LastTestedAt != nil && !connection.LastTestedAt.Before(profile.UpdatedAt) {
		value.LastTestedAt, value.LastTestErrorCode = connection.LastTestedAt, connection.LastErrorCode
	}
	if err != nil {
		var setup *AISetupError
		if errors.As(err, &setup) {
			value.Blockers = append(value.Blockers, setup.Code)
			return value, nil
		}
		return value, err
	}
	credential, err := s.aiConnectionCredential(ctx, product, connection)
	zeroBytes(credential)
	if err != nil {
		if errors.Is(err, ErrAIUnavailable) {
			value.Blockers = append(value.Blockers, "credential_unavailable")
		} else {
			return value, err
		}
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	usage, err := s.store.AIBudgetStatus(ctx, product.ID, "analysis", day)
	if err != nil {
		return value, err
	}
	value.Budget = &model.AIProcessingBudget{Limited: profile.DailyTokenBudget > 0, DailyLimit: profile.DailyTokenBudget, Used: usage.Used, Reserved: usage.Reserved, ResetsAt: day.AddDate(0, 0, 1)}
	if value.Budget.Limited {
		remaining := max(int64(0), profile.DailyTokenBudget-usage.Used-usage.Reserved)
		value.Budget.Remaining = &remaining
		if remaining == 0 {
			value.Blockers = append(value.Blockers, "budget_exhausted")
		}
	}
	value.CanProcess = len(value.Blockers) == 0
	return value, nil
}

func validAIWorkloadTarget(profile model.AIWorkloadProfile, connection model.AIProviderConnection) bool {
	return strings.TrimSpace(profile.Model) != "" && len(profile.Model) <= 160 && strings.IndexFunc(profile.Model, func(value rune) bool { return value < 0x20 || value == 0x7f }) < 0 && profile.MaxInputTokens >= 256 && profile.MaxInputTokens <= 1_000_000 && profile.MaxOutputTokens > 0 && profile.MaxOutputTokens <= 32_768 && profile.DailyTokenBudget >= 0 && profile.DailyTokenBudget <= 10_000_000_000 && !connection.IsBackup && supportedAIProviders[connection.Provider] && validHTTPSBaseOrigin(connection.Endpoint) && (connection.Provider == "openai-compatible" || connection.Endpoint == aiProviderOrigin(connection.Provider))
}
