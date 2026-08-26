package store

import (
	"context"
	"sort"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func (m *Memory) IntegrationAnalyses(_ context.Context, productID string) ([]model.IntegrationAnalysis, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.integrationAnalyses[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.IntegrationAnalysis, 0, len(values))
	for _, value := range values {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) IntegrationAnalysis(_ context.Context, productID, id string) (model.IntegrationAnalysis, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.integrationAnalyses[productID][id]
	if !ok {
		return model.IntegrationAnalysis{}, ErrNotFound
	}
	return memoryClone(value), nil
}

func (m *Memory) SaveIntegrationAnalysis(_ context.Context, value model.IntegrationAnalysis, expected int64) (model.IntegrationAnalysis, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.IntegrationAnalysis{}, ErrNotFound
	}
	current, exists := m.integrationAnalyses[value.ProductID][value.ID]
	if exists && current.Revision != expected {
		return model.IntegrationAnalysis{}, ErrConflict
	}
	if !exists && expected != 0 {
		return model.IntegrationAnalysis{}, ErrConflict
	}
	if exists {
		value.CreatedAt = current.CreatedAt
		value.Revision = current.Revision + 1
	} else {
		value.Revision = 1
		if value.CreatedAt.IsZero() {
			value.CreatedAt = time.Now().UTC()
		}
	}
	m.integrationAnalyses[value.ProductID][value.ID] = memoryClone(value)
	return memoryClone(value), nil
}

func (m *Memory) hydrateRecipeLocked(value model.Recipe) model.Recipe {
	if revision, ok := m.recipeRevisions[value.ID][value.CurrentRevisionID]; ok {
		copy := memoryClone(revision)
		value.CurrentRevision = &copy
	}
	return memoryClone(value)
}

func (m *Memory) Recipes(_ context.Context, productID string) ([]model.Recipe, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.recipes[productID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.Recipe, 0, len(values))
	for _, value := range values {
		result = append(result, m.hydrateRecipeLocked(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (m *Memory) Recipe(_ context.Context, productID, id string) (model.Recipe, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.recipes[productID][id]
	if !ok {
		return model.Recipe{}, ErrNotFound
	}
	return m.hydrateRecipeLocked(value), nil
}

func (m *Memory) RecipeBySlug(_ context.Context, productID, slug string) (model.Recipe, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, value := range m.recipes[productID] {
		if value.Slug == slug {
			return m.hydrateRecipeLocked(value), nil
		}
	}
	return model.Recipe{}, ErrNotFound
}

func (m *Memory) SaveRecipe(_ context.Context, value model.Recipe, expected int64) (model.Recipe, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.products[value.ProductID]; !ok {
		return model.Recipe{}, ErrNotFound
	}
	for id, candidate := range m.recipes[value.ProductID] {
		if id != value.ID && candidate.Slug == value.Slug {
			return model.Recipe{}, ErrConflict
		}
	}
	current, exists := m.recipes[value.ProductID][value.ID]
	now := time.Now().UTC()
	if exists && current.Revision != expected {
		return model.Recipe{}, ErrConflict
	}
	if !exists && expected != 0 {
		return model.Recipe{}, ErrConflict
	}
	value.CurrentRevision = nil
	if exists {
		value.CreatedAt = current.CreatedAt
		value.Revision = current.Revision + 1
	} else {
		value.Revision = 1
		if value.CreatedAt.IsZero() {
			value.CreatedAt = now
		}
	}
	value.UpdatedAt = now
	m.recipes[value.ProductID][value.ID] = memoryClone(value)
	return m.hydrateRecipeLocked(value), nil
}

func (m *Memory) RecipeRevisions(_ context.Context, recipeID string) ([]model.RecipeRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.recipeRevisions[recipeID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]model.RecipeRevision, 0, len(values))
	for _, value := range values {
		result = append(result, memoryClone(value))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Revision > result[j].Revision })
	return result, nil
}

func (m *Memory) SaveRecipeRevision(_ context.Context, recipe model.Recipe, value model.RecipeRevision, expected int64) (model.Recipe, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	current, exists := m.recipes[recipe.ProductID][recipe.ID]
	if !exists {
		return model.Recipe{}, ErrNotFound
	}
	if current.Revision != expected {
		return model.Recipe{}, ErrConflict
	}
	for id, candidate := range m.recipes[recipe.ProductID] {
		if id != recipe.ID && candidate.Slug == recipe.Slug {
			return model.Recipe{}, ErrConflict
		}
	}
	if m.recipeRevisions[recipe.ID] == nil {
		m.recipeRevisions[recipe.ID] = make(map[string]model.RecipeRevision)
	}
	if _, exists := m.recipeRevisions[recipe.ID][value.ID]; exists {
		return model.Recipe{}, ErrConflict
	}
	if value.Revision == 0 {
		value.Revision = 1
		for _, revision := range m.recipeRevisions[recipe.ID] {
			if revision.Revision >= value.Revision {
				value.Revision = revision.Revision + 1
			}
		}
	}
	for _, revision := range m.recipeRevisions[recipe.ID] {
		if revision.Revision == value.Revision {
			return model.Recipe{}, ErrConflict
		}
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	recipe.CurrentRevisionID, recipe.CurrentRevision = value.ID, nil
	recipe.CreatedAt = current.CreatedAt
	recipe.Revision = current.Revision + 1
	recipe.UpdatedAt = now
	m.recipeRevisions[recipe.ID][value.ID] = memoryClone(value)
	m.recipes[recipe.ProductID][recipe.ID] = memoryClone(recipe)
	return m.hydrateRecipeLocked(recipe), nil
}
