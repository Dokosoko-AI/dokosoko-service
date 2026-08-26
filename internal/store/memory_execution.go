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
