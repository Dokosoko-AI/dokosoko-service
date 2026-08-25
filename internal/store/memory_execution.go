package store

import (
	"context"
	"encoding/hex"
	"github.com/dokosoko/dokosoko-service/internal/model"
	"sort"
	"time"
)

func (m *Memory) Providers(_ context.Context, productID string) ([]model.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Provider, 0, len(m.providers[productID]))
	for _, value := range m.providers[productID] {
		result = append(result, cloneProvider(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Memory) Provider(_ context.Context, productID, id string) (model.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.providers[productID][id]
	if !ok {
		return model.Provider{}, ErrNotFound
	}
	return cloneProvider(value), nil
}

func (m *Memory) CreateProvider(_ context.Context, value model.Provider) (model.Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Provider{}, ErrNotFound
	}
	value.Revision = 1
	value.CreatedAt, value.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	m.providers[value.ProductID][value.ID] = cloneProvider(value)
	return cloneProvider(value), nil
}

func (m *Memory) Projects(_ context.Context, productID string) ([]model.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.Project, 0, len(m.projects[productID]))
	for _, value := range m.projects[productID] {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) Project(_ context.Context, productID, id string) (model.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.projects[productID][id]
	if !ok {
		return model.Project{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateProject(_ context.Context, value model.Project) (model.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.projects[value.ProductID] {
		if existing.ProviderID == value.ProviderID && existing.IdempotencyKey == value.IdempotencyKey {
			return existing, nil
		}
	}
	value.CreatedAt, value.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	m.projects[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) CredentialLeases(_ context.Context, productID string) ([]model.CredentialLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.CredentialLease, 0, len(m.leases[productID]))
	for _, value := range m.leases[productID] {
		value.Scopes = append([]string(nil), value.Scopes...)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) CredentialLease(_ context.Context, productID, id string) (model.CredentialLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.leases[productID][id]
	if !ok {
		return model.CredentialLease{}, ErrNotFound
	}
	value.Scopes = append([]string(nil), value.Scopes...)
	return value, nil
}

func (m *Memory) CreateCredentialLease(_ context.Context, value model.CredentialLease) (model.CredentialLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.leases[value.ProductID] {
		if value.IdempotencyKey != "" && existing.ProviderID == value.ProviderID && existing.IdempotencyKey == value.IdempotencyKey {
			return existing, nil
		}
	}
	value.CreatedAt = time.Now().UTC()
	value.Scopes = append([]string(nil), value.Scopes...)
	m.leases[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) RevokeCredentialLease(_ context.Context, productID, id string, revokedAt time.Time) (model.CredentialLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.leases[productID][id]
	if !ok {
		return model.CredentialLease{}, ErrNotFound
	}
	value.RevokedAt = &revokedAt
	m.leases[productID][id] = value
	return value, nil
}

func (m *Memory) IntegrationRuns(_ context.Context, productID string) ([]model.IntegrationRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.integrationRuns[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationRun, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.After(result[j].StartedAt) })
	return result, nil
}

func (m *Memory) IntegrationRun(_ context.Context, productID, id string) (model.IntegrationRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.integrationRuns[productID][id]
	if !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) CreateIntegrationRun(_ context.Context, value model.IntegrationRun) (model.IntegrationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	if _, exists := m.integrationRuns[value.ProductID][value.ID]; exists {
		return model.IntegrationRun{}, ErrConflict
	}
	value.State = "running"
	value.StartedAt = time.Now().UTC()
	m.integrationRuns[value.ProductID][value.ID] = value
	return value, nil
}

func (m *Memory) CompleteIntegrationRun(_ context.Context, productID, id string, reported, validated *bool, failureCode string, finishedAt time.Time) (model.IntegrationRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.integrationRuns[productID][id]
	if !ok {
		return model.IntegrationRun{}, ErrNotFound
	}
	if value.FinishedAt != nil {
		return model.IntegrationRun{}, ErrConflict
	}
	value.ReportedSuccess, value.ValidatedSuccess = reported, validated
	value.FailureCode, value.FinishedAt = failureCode, &finishedAt
	value.State = "failed"
	if validated != nil && *validated {
		value.State = "succeeded"
	}
	m.integrationRuns[productID][id] = value
	return value, nil
}

func cloneReportSubmission(value model.ReportSubmission) model.ReportSubmission {
	value.IdempotencyDigest = append([]byte(nil), value.IdempotencyDigest...)
	value.PayloadCiphertext = append([]byte(nil), value.PayloadCiphertext...)
	value.PayloadNonce = append([]byte(nil), value.PayloadNonce...)
	value.IntegrationSnapshot = append([]byte(nil), value.IntegrationSnapshot...)
	return value
}

func (m *Memory) ReportSubmissions(_ context.Context, productID, startingAfter string, limit int) ([]model.ReportSubmission, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.reportSubmissions[productID]
	if !ok {
		return nil, false, ErrNotFound
	}
	result := make([]model.ReportSubmission, 0, len(values))
	for _, value := range values {
		result = append(result, cloneReportSubmission(value))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	start := 0
	if startingAfter != "" {
		start = -1
		for index := range result {
			if result[index].ID == startingAfter {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil, false, ErrNotFound
		}
	}
	result = result[start:]
	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (m *Memory) ReportSubmission(_ context.Context, productID, id string) (model.ReportSubmission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.reportSubmissions[productID][id]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	return cloneReportSubmission(value), nil
}

func (m *Memory) CreateReportSubmission(_ context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.reportSubmissions[value.ProductID]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	for _, current := range values {
		if current.ActorPseudonym == value.ActorPseudonym && current.Kind == value.Kind && hex.EncodeToString(current.IdempotencyDigest) == hex.EncodeToString(value.IdempotencyDigest) {
			return cloneReportSubmission(current), nil
		}
	}
	if _, exists := values[value.ID]; exists {
		return model.ReportSubmission{}, ErrConflict
	}
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	values[value.ID] = cloneReportSubmission(value)
	return cloneReportSubmission(value), nil
}

func (m *Memory) ActivateHeldReportSubmissions(_ context.Context, productID, routeID, kind string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.reportSubmissions[productID]
	if !ok {
		return ErrNotFound
	}
	for id, value := range values {
		if value.SupportRouteID == routeID && value.Kind == kind && value.State == "held" && value.ExpiresAt.After(now) {
			value.State, value.NextAttemptAt, value.UpdatedAt = "pending", &now, now
			values[id] = value
		}
	}
	return nil
}

func (m *Memory) ClaimReportSubmissions(_ context.Context, now time.Time, limit int) ([]model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 25
	}
	stale := now.Add(-5 * time.Minute)
	result := make([]model.ReportSubmission, 0, limit)
	for productID, values := range m.reportSubmissions {
		for id, value := range values {
			ready := value.State == "pending" && (value.NextAttemptAt == nil || !value.NextAttemptAt.After(now))
			recoverable := value.State == "delivering" && value.DeliveryStartedAt != nil && value.DeliveryStartedAt.Before(stale)
			if (!ready && !recoverable) || !value.ExpiresAt.After(now) || len(result) >= limit {
				continue
			}
			value.State, value.DeliveryStartedAt, value.UpdatedAt = "delivering", &now, now
			value.Attempts++
			m.reportSubmissions[productID][id] = value
			result = append(result, cloneReportSubmission(value))
		}
	}
	return result, nil
}

func (m *Memory) UpdateReportSubmissionDelivery(_ context.Context, value model.ReportSubmission) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.reportSubmissions[value.ProductID][value.ID]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	current.State = value.State
	current.Attempts = value.Attempts
	current.NextAttemptAt = value.NextAttemptAt
	current.DeliveryStartedAt = value.DeliveryStartedAt
	current.LastError = value.LastError
	current.ExternalID = value.ExternalID
	current.ExternalURL = value.ExternalURL
	current.DeliveredAt = value.DeliveredAt
	current.UpdatedAt = time.Now().UTC()
	m.reportSubmissions[value.ProductID][value.ID] = current
	return cloneReportSubmission(current), nil
}

func (m *Memory) RetryReportSubmission(_ context.Context, productID, id string, now time.Time) (model.ReportSubmission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.reportSubmissions[productID][id]
	if !ok {
		return model.ReportSubmission{}, ErrNotFound
	}
	if (value.State != "held" && value.State != "failed") || !value.ExpiresAt.After(now) {
		return model.ReportSubmission{}, ErrConflict
	}
	value.State, value.NextAttemptAt, value.DeliveryStartedAt = "pending", &now, nil
	value.LastError, value.UpdatedAt = "", now
	m.reportSubmissions[productID][id] = value
	return cloneReportSubmission(value), nil
}

func (m *Memory) DeleteExpiredReportSubmissions(_ context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted int64
	for _, values := range m.reportSubmissions {
		for id, value := range values {
			if !value.ExpiresAt.After(now) {
				delete(values, id)
				deleted++
			}
		}
	}
	return deleted, nil
}
