package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) AIProviderConnections(_ context.Context, deploymentID string) ([]model.AIProviderConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.aiProviderConnections[deploymentID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.AIProviderConnection, 0, len(values))
	for _, value := range values {
		value.BackupModels = append([]byte(nil), value.BackupModels...)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Provider < result[j].Provider })
	return result, nil
}

func (m *Memory) AIProviderConnection(_ context.Context, deploymentID, id string) (model.AIProviderConnection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.aiProviderConnections[deploymentID] {
		if value.ID == id {
			value.BackupModels = append([]byte(nil), value.BackupModels...)
			return value, nil
		}
	}
	return model.AIProviderConnection{}, ErrNotFound
}

func (m *Memory) SaveAIProviderConnection(_ context.Context, value model.AIProviderConnection, expectedRevision int64) (model.AIProviderConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.aiProviderConnections[value.DeploymentID]
	if !ok {
		return model.AIProviderConnection{}, ErrNotFound
	}
	now := time.Now().UTC()
	if len(value.BackupModels) == 0 {
		value.BackupModels = []byte(`{}`)
	}
	if current, exists := values[value.Provider]; exists {
		if expectedRevision != current.Revision {
			return model.AIProviderConnection{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		if expectedRevision != 0 {
			return model.AIProviderConnection{}, ErrConflict
		}
		value.Revision, value.CreatedAt = 1, now
	}
	value.UpdatedAt = now
	value.BackupModels = append([]byte(nil), value.BackupModels...)
	values[value.Provider] = value
	return value, nil
}

func (m *Memory) AIWorkloadProfiles(_ context.Context, productID string) ([]model.AIWorkloadProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.aiWorkloadProfiles[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.AIWorkloadProfile, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Workload < result[j].Workload })
	return result, nil
}

func (m *Memory) AIWorkloadProfile(_ context.Context, productID, workload string) (model.AIWorkloadProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.aiWorkloadProfiles[productID][workload]
	if !ok {
		return model.AIWorkloadProfile{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) SaveAIWorkloadProfile(_ context.Context, value model.AIWorkloadProfile, expectedRevision int64) (model.AIWorkloadProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	values, ok := m.aiWorkloadProfiles[value.ProductID]
	if !ok {
		return model.AIWorkloadProfile{}, ErrNotFound
	}
	connectionFound := false
	for _, connection := range m.aiProviderConnections[value.ProductID] {
		if connection.ID == value.ProviderConnectionID {
			connectionFound = true
			break
		}
	}
	if !connectionFound {
		return model.AIWorkloadProfile{}, ErrNotFound
	}
	now := time.Now().UTC()
	if current, exists := values[value.Workload]; exists {
		if expectedRevision != current.Revision {
			return model.AIWorkloadProfile{}, ErrConflict
		}
		value.ID, value.CreatedAt, value.Revision = current.ID, current.CreatedAt, current.Revision+1
	} else {
		if expectedRevision != 0 {
			return model.AIWorkloadProfile{}, ErrConflict
		}
		value.Revision, value.CreatedAt = 1, now
	}
	value.UpdatedAt = now
	values[value.Workload] = value
	return value, nil
}

func budgetDayKey(productID, workload string, day time.Time) string {
	return fmt.Sprintf("%s:%s:%s", productID, workload, day.UTC().Format("2006-01-02"))
}

func (m *Memory) ReserveAIBudget(_ context.Context, value model.AIBudgetReservation, dailyBudget int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for id, reservation := range m.aiBudgetReservations {
		if !reservation.ExpiresAt.After(now) {
			delete(m.aiBudgetReservations, id)
		}
	}
	key := budgetDayKey(value.ProductID, value.Workload, value.Day)
	reserved := int64(0)
	for _, reservation := range m.aiBudgetReservations {
		if budgetDayKey(reservation.ProductID, reservation.Workload, reservation.Day) == key {
			reserved += reservation.ReservedTokens
		}
	}
	if dailyBudget > 0 && m.aiBudgetUsed[key]+reserved+value.ReservedTokens > dailyBudget {
		return false, nil
	}
	m.aiBudgetReservations[value.ID] = value
	return true, nil
}

func (m *Memory) FinishAIUsage(_ context.Context, reservationID string, event model.AIUsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	reservation, ok := m.aiBudgetReservations[reservationID]
	if !ok {
		return ErrNotFound
	}
	delete(m.aiBudgetReservations, reservationID)
	actual := event.InputTokens + event.OutputTokens
	if actual < 0 {
		actual = 0
	}
	m.aiBudgetUsed[budgetDayKey(reservation.ProductID, reservation.Workload, reservation.Day)] += actual
	if event.DurationMS == 0 && event.Duration > 0 {
		event.DurationMS = event.Duration.Milliseconds()
	}
	m.aiUsage = append(m.aiUsage, event)
	return nil
}

func (m *Memory) AIUsageEvents(_ context.Context, productID string, since time.Time) ([]model.AIUsageEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.AIUsageEvent, 0)
	for _, value := range m.aiUsage {
		if value.ProductID == productID && !value.CreatedAt.Before(since) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
