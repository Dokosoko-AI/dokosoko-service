package store

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func cloneReportSubmission(value model.ReportSubmission) model.ReportSubmission {
	value.IdempotencyDigest = append([]byte(nil), value.IdempotencyDigest...)
	value.Payload = append([]byte(nil), value.Payload...)
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
		if current.ActorPseudonym == value.ActorPseudonym && current.Kind == value.Kind && bytes.Equal(current.IdempotencyDigest, value.IdempotencyDigest) {
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

func cloneAuthorizationUsageEvent(value model.AuthorizationUsageEvent) model.AuthorizationUsageEvent {
	value.AuthConfig = append([]byte(nil), value.AuthConfig...)
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

func (m *Memory) CreateAuthorizationUsageEvent(_ context.Context, value model.AuthorizationUsageEvent) (model.AuthorizationUsageEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.authorizationUsageEvents[value.ID]; exists {
		return model.AuthorizationUsageEvent{}, ErrConflict
	}
	if _, ok := m.products[value.ProductID]; !ok {
		return model.AuthorizationUsageEvent{}, ErrNotFound
	}
	now := time.Now().UTC()
	value.State = "queued"
	value.Attempts = 0
	if value.AvailableAt.IsZero() {
		value.AvailableAt = now
	}
	value.CreatedAt, value.UpdatedAt = now, now
	m.authorizationUsageEvents[value.ID] = cloneAuthorizationUsageEvent(value)
	return cloneAuthorizationUsageEvent(value), nil
}

func (m *Memory) ClaimAuthorizationUsageEvents(_ context.Context, owner string, leaseUntil time.Time, limit int) ([]model.AuthorizationUsageEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit < 1 || limit > 100 {
		limit = 25
	}
	now := time.Now().UTC()
	ids := make([]string, 0, len(m.authorizationUsageEvents))
	for id, value := range m.authorizationUsageEvents {
		queued := value.State == "queued" && !value.AvailableAt.After(now)
		abandoned := value.State == "delivering" && value.LeasedUntil != nil && value.LeasedUntil.Before(now)
		if queued || abandoned {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := m.authorizationUsageEvents[ids[i]], m.authorizationUsageEvents[ids[j]]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.ID < right.ID
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	result := make([]model.AuthorizationUsageEvent, 0, len(ids))
	for _, id := range ids {
		value := m.authorizationUsageEvents[id]
		value.State, value.LeaseOwner, value.LeasedUntil = "delivering", owner, &leaseUntil
		value.Attempts++
		value.UpdatedAt = now
		m.authorizationUsageEvents[id] = value
		result = append(result, cloneAuthorizationUsageEvent(value))
	}
	return result, nil
}

func (m *Memory) CompleteAuthorizationUsageEvent(_ context.Context, id, owner string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.authorizationUsageEvents[id]
	if !ok {
		return ErrNotFound
	}
	if value.State != "delivering" || value.LeaseOwner != owner {
		return ErrConflict
	}
	value.State, value.LeaseOwner, value.LeasedUntil, value.LastError = "delivered", "", nil, ""
	value.UpdatedAt = now
	m.authorizationUsageEvents[id] = value
	return nil
}

func (m *Memory) RetryAuthorizationUsageEvent(_ context.Context, id, owner string, availableAt time.Time, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.authorizationUsageEvents[id]
	if !ok {
		return ErrNotFound
	}
	if value.State != "delivering" || value.LeaseOwner != owner {
		return ErrConflict
	}
	value.State, value.LeaseOwner, value.LeasedUntil = "queued", "", nil
	value.AvailableAt, value.LastError, value.UpdatedAt = availableAt, lastError, time.Now().UTC()
	m.authorizationUsageEvents[id] = value
	return nil
}
