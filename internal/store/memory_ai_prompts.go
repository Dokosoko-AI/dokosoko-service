package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) AIPromptStates(_ context.Context, productID string) ([]model.AIPromptState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.aiPromptStates[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.AIPromptState, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

func (m *Memory) AIPromptState(_ context.Context, productID, key string) (model.AIPromptState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.aiPromptStates[productID]
	if !ok {
		return model.AIPromptState{}, ErrNotFound
	}
	value, ok := values[key]
	if !ok {
		return model.AIPromptState{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) SaveAIPromptState(_ context.Context, value model.AIPromptState, expectedRevision int64) (model.AIPromptState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveAIPromptStateLocked(value, expectedRevision)
}

func (m *Memory) saveAIPromptStateLocked(value model.AIPromptState, expectedRevision int64) (model.AIPromptState, error) {
	values, ok := m.aiPromptStates[value.ProductID]
	if !ok {
		return model.AIPromptState{}, ErrNotFound
	}
	if current, exists := values[value.Key]; exists {
		if expectedRevision != current.Revision {
			return model.AIPromptState{}, ErrConflict
		}
		value.Revision = current.Revision + 1
	} else {
		// Revision 1 is the virtual, unmodified server default. The first
		// persisted mutation therefore becomes revision 2.
		if expectedRevision != 1 {
			return model.AIPromptState{}, ErrConflict
		}
		value.Revision = 2
	}
	value.UpdatedAt = time.Now().UTC()
	values[value.Key] = value
	return value, nil
}

func (m *Memory) SaveAIPromptStateAndAudit(_ context.Context, value model.AIPromptState, expectedRevision int64, event model.AuditEvent) (model.AIPromptState, error) {
	if strings.TrimSpace(event.ID) == "" {
		return model.AIPromptState{}, errors.New("audit event ID is required")
	}
	// Validate the persisted audit representation before taking the lock so a
	// malformed event cannot leave prompt state without its audit record.
	if _, err := json.Marshal(event.Prior); err != nil {
		return model.AIPromptState{}, err
	}
	if _, err := json.Marshal(event.Current); err != nil {
		return model.AIPromptState{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, current := range m.audit {
		if current.ID == event.ID {
			return model.AIPromptState{}, ErrConflict
		}
	}
	updated, err := m.saveAIPromptStateLocked(value, expectedRevision)
	if err != nil {
		return model.AIPromptState{}, err
	}
	m.audit = append(m.audit, event)
	return updated, nil
}
