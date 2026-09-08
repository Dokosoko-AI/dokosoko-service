package store

import (
	"context"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) CreateSourceOnce(_ context.Context, input SourceCreation) (model.Source, error) {
	if err := validateSourceCreation(input); err != nil {
		return model.Source{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value := input.Source
	product, exists := m.products[value.ProductID]
	if !exists || product.OrganisationID != value.OrganisationID {
		return model.Source{}, ErrNotFound
	}
	key := value.ProductID + ":" + input.RequestDigest
	if previous, exists := m.sourceCreationRequests[key]; exists {
		if previous.InputDigest != input.InputDigest {
			return model.Source{}, ErrSourceCreationConflict
		}
		stored, exists := m.sources[value.ProductID][previous.SourceID]
		if !exists {
			return model.Source{}, ErrNotFound
		}
		return stored, nil
	}
	if _, exists := m.sources[value.ProductID][value.ID]; exists {
		return model.Source{}, ErrConflict
	}
	for _, event := range m.audit {
		if event.ID == input.Audit.ID {
			return model.Source{}, ErrConflict
		}
	}
	if m.sourceCreationRequests == nil {
		m.sourceCreationRequests = make(map[string]sourceCreationResult)
	}
	if m.sources[value.ProductID] == nil {
		m.sources[value.ProductID] = make(map[string]model.Source)
	}
	value.Visibility = model.VisibilityPrivate
	value.Published, value.Quarantined = false, false
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	m.sources[value.ProductID][value.ID] = value
	m.sourceCreationRequests[key] = sourceCreationResult{SourceID: value.ID, InputDigest: input.InputDigest}
	event := input.Audit
	event.Outcome = "success"
	m.audit = append(m.audit, memoryClone(event))
	return value, nil
}
