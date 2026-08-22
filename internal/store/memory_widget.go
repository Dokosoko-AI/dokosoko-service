package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func cloneWidget(value model.Widget) model.Widget {
	return memoryClone(value)
}

func (m *Memory) Widgets(_ context.Context, deploymentID string) ([]model.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasDeployment || m.deployment.ID != deploymentID {
		return nil, ErrNotFound
	}
	values := make([]model.Widget, 0)
	for _, value := range m.widgets {
		if value.DeploymentID == deploymentID {
			values = append(values, cloneWidget(value))
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	return values, nil
}

func (m *Memory) Widget(_ context.Context, deploymentID, id string) (model.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.widgets[id]
	if !ok || value.DeploymentID != deploymentID {
		return model.Widget{}, ErrNotFound
	}
	return cloneWidget(value), nil
}

func (m *Memory) CreateWidget(_ context.Context, value model.Widget) (model.Widget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasDeployment || m.deployment.ID != value.DeploymentID {
		return model.Widget{}, ErrNotFound
	}
	if _, exists := m.widgets[value.ID]; exists {
		return model.Widget{}, ErrConflict
	}
	now := time.Now().UTC()
	value.Revision, value.CreatedAt, value.UpdatedAt = 1, now, now
	m.widgets[value.ID] = cloneWidget(value)
	return cloneWidget(value), nil
}

func (m *Memory) UpdateWidget(_ context.Context, value model.Widget, expected int64) (model.Widget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.widgets[value.ID]
	if !ok || current.DeploymentID != value.DeploymentID {
		return model.Widget{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Widget{}, ErrConflict
	}
	value.CreatedAt = current.CreatedAt
	value.Revision = expected + 1
	value.UpdatedAt = time.Now().UTC()
	m.widgets[value.ID] = cloneWidget(value)
	return cloneWidget(value), nil
}

func (m *Memory) CreateWidgetSecret(_ context.Context, value model.WidgetSecret) (model.WidgetSecret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.widgets[value.WidgetID]; !ok {
		return model.WidgetSecret{}, ErrNotFound
	}
	key := hex.EncodeToString(value.Digest)
	if _, exists := m.widgetSecretDigests[key]; exists {
		return model.WidgetSecret{}, ErrConflict
	}
	value.CreatedAt = time.Now().UTC()
	m.widgetSecrets[value.ID] = value
	m.widgetSecretDigests[key] = value.ID
	return value, nil
}

func (m *Memory) WidgetSecrets(_ context.Context, widgetID string) ([]model.WidgetSecret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.widgets[widgetID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.WidgetSecret, 0)
	for _, value := range m.widgetSecrets {
		if value.WidgetID == widgetID {
			value.Digest = nil
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.After(values[j].CreatedAt) })
	return values, nil
}

func (m *Memory) WidgetSecretByDigest(_ context.Context, widgetID string, digest []byte) (model.WidgetSecret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.widgetSecretDigests[hex.EncodeToString(digest)]
	value, exists := m.widgetSecrets[id]
	if !ok || !exists || value.WidgetID != widgetID || value.RevokedAt != nil || !bytes.Equal(value.Digest, digest) {
		return model.WidgetSecret{}, ErrNotFound
	}
	now := time.Now().UTC()
	value.LastUsedAt = &now
	m.widgetSecrets[id] = value
	return value, nil
}

func (m *Memory) RevokeWidgetSecret(_ context.Context, widgetID, id string, now time.Time) (model.WidgetSecret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.widgetSecrets[id]
	if !ok || value.WidgetID != widgetID {
		return model.WidgetSecret{}, ErrNotFound
	}
	if value.RevokedAt == nil {
		value.RevokedAt = &now
		m.widgetSecrets[id] = value
	}
	return value, nil
}

func (m *Memory) CreateWidgetBootstrap(_ context.Context, value model.WidgetBootstrap) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(value.Digest)
	if _, exists := m.widgetBootstraps[key]; exists {
		return ErrConflict
	}
	value.CreatedAt = time.Now().UTC()
	m.widgetBootstraps[key] = value
	return nil
}

func (m *Memory) WidgetBootstrap(_ context.Context, digest []byte) (model.WidgetBootstrap, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.widgetBootstraps[hex.EncodeToString(digest)]
	if !ok {
		return model.WidgetBootstrap{}, ErrNotFound
	}
	return value, nil
}

func (m *Memory) ConsumeWidgetBootstrap(_ context.Context, digest []byte, now time.Time) (model.WidgetBootstrap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.widgetBootstraps[key]
	if !ok || value.UsedAt != nil || !value.ExpiresAt.After(now) {
		return model.WidgetBootstrap{}, ErrNotFound
	}
	value.UsedAt = &now
	m.widgetBootstraps[key] = value
	return value, nil
}

func (m *Memory) CreateWidgetSession(_ context.Context, value model.WidgetSession) (model.WidgetSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(value.Digest)
	if _, exists := m.widgetSessions[key]; exists {
		return model.WidgetSession{}, ErrConflict
	}
	value.CreatedAt = time.Now().UTC()
	m.widgetSessions[key] = value
	return value, nil
}

func (m *Memory) WidgetSessions(_ context.Context, widgetID string) ([]model.WidgetSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.widgets[widgetID]; !ok {
		return nil, ErrNotFound
	}
	values := make([]model.WidgetSession, 0)
	for _, value := range m.widgetSessions {
		if value.WidgetID == widgetID {
			value.Digest = nil
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.After(values[j].CreatedAt) })
	return values, nil
}

func (m *Memory) WidgetSessionByDigest(_ context.Context, digest []byte, now time.Time) (model.WidgetSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(digest)
	value, ok := m.widgetSessions[key]
	if !ok || value.RevokedAt != nil || !value.ExpiresAt.After(now) {
		return model.WidgetSession{}, ErrNotFound
	}
	value.LastSeenAt = &now
	m.widgetSessions[key] = value
	return value, nil
}

func (m *Memory) RevokeWidgetSession(_ context.Context, widgetID, id string, now time.Time) (model.WidgetSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range m.widgetSessions {
		if value.ID == id && value.WidgetID == widgetID {
			if value.RevokedAt == nil {
				value.RevokedAt = &now
				m.widgetSessions[key] = value
			}
			return value, nil
		}
	}
	return model.WidgetSession{}, ErrNotFound
}

func (m *Memory) RevokeWidgetSessions(_ context.Context, widgetID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.widgets[widgetID]; !ok {
		return ErrNotFound
	}
	for key, value := range m.widgetSessions {
		if value.WidgetID == widgetID && value.RevokedAt == nil {
			value.RevokedAt = &now
			m.widgetSessions[key] = value
		}
	}
	return nil
}
