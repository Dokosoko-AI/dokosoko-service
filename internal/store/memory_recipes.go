package store

import (
	"context"
	"errors"
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

func (m *Memory) DeleteRecipe(_ context.Context, productID, recipeID string, mutation RecipeMutation) error {
	if err := validateRecipeMutation(mutation); err != nil {
		return err
	}
	if mutation.Audit == nil {
		return errors.New("recipe deletion requires an audit event")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.products[productID]; !exists {
		return ErrNotFound
	}
	stored, exists := m.recipes[productID][recipeID]
	if !exists {
		return ErrNotFound
	}
	if stored.Revision != mutation.ExpectedRevision {
		return ErrConflict
	}
	if !recipeDeletionAllowed(stored) {
		return ErrConflict
	}
	if m.recipeAuditExistsLocked(mutation.Audit.ID) {
		return ErrConflict
	}
	_, _, auditOutcome, err := prepareRecipeAudit(stored, mutation.Audit, "recipe.deleted")
	if err != nil {
		return err
	}

	delete(m.recipes[productID], recipeID)
	delete(m.recipeRevisions, recipeID)
	if stored.State == "published" {
		now := time.Now().UTC()
		if _, err := m.bumpProductCatalogRevisionLocked(productID, now); err != nil {
			return err
		}
	}
	event := memoryClone(*mutation.Audit)
	event.Outcome = auditOutcome
	m.audit = append(m.audit, event)
	return nil
}

func (m *Memory) CreateRecipeWithRevision(_ context.Context, recipe model.Recipe, revision model.RecipeRevision, mutation RecipeMutation) (model.Recipe, error) {
	var err error
	recipe, err = prepareRecipeRecord(recipe)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.ExpectedRevision != 0 {
		return model.Recipe{}, ErrConflict
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe creation requires an audit event")
	}
	if revision.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	revision, err = prepareRecipeRevisionRecord(recipe, revision)
	if err != nil {
		return model.Recipe{}, err
	}
	if revision.Revision != 0 && revision.Revision != 1 {
		return model.Recipe{}, ErrConflict
	}
	_, _, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, "recipe.created")
	if err != nil {
		return model.Recipe{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.validateRecipeCatalogLocked(recipe.ProductID, mutation.ExpectedCatalogRevision); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit != nil && m.recipeAuditExistsLocked(mutation.Audit.ID) {
		return model.Recipe{}, ErrConflict
	}
	if _, ok := m.products[recipe.ProductID]; !ok {
		return model.Recipe{}, ErrNotFound
	}
	if err := m.validateRecipeAPIBindingsLocked(recipe, revision); err != nil {
		return model.Recipe{}, err
	}
	for _, recipes := range m.recipes {
		if _, exists := recipes[recipe.ID]; exists {
			return model.Recipe{}, ErrConflict
		}
	}
	for _, candidate := range m.recipes[recipe.ProductID] {
		if candidate.Slug == recipe.Slug || candidate.StableURI == recipe.StableURI {
			return model.Recipe{}, ErrConflict
		}
	}
	for _, revisions := range m.recipeRevisions {
		if _, exists := revisions[revision.ID]; exists {
			return model.Recipe{}, ErrConflict
		}
	}

	now := time.Now().UTC()
	recipe.CurrentRevisionID, recipe.CurrentRevision = revision.ID, nil
	recipe.Revision = 1
	if recipe.CreatedAt.IsZero() {
		recipe.CreatedAt = now
	}
	recipe.UpdatedAt = now
	revision.Revision = 1
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = now
	}

	if m.recipes[recipe.ProductID] == nil {
		m.recipes[recipe.ProductID] = make(map[string]model.Recipe)
	}
	if m.recipeRevisions[recipe.ID] == nil {
		m.recipeRevisions[recipe.ID] = make(map[string]model.RecipeRevision)
	}
	m.recipeRevisions[recipe.ID][revision.ID] = memoryClone(revision)
	m.recipes[recipe.ProductID][recipe.ID] = memoryClone(recipe)
	if mutation.Audit != nil {
		event := memoryClone(*mutation.Audit)
		event.Outcome = auditOutcome
		m.audit = append(m.audit, event)
	}
	return m.hydrateRecipeLocked(recipe), nil
}

func (m *Memory) SaveRecipeTransition(_ context.Context, recipe model.Recipe, mutation RecipeMutation) (model.Recipe, error) {
	var err error
	recipe, err = prepareRecipeRecord(recipe)
	if err != nil {
		return model.Recipe{}, err
	}
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe transitions require an audit event")
	}
	_, _, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, recipeTransitionAuditActions(model.Recipe{}, recipe)...)
	if err != nil {
		return model.Recipe{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	product, err := m.validateRecipeCatalogLocked(recipe.ProductID, mutation.ExpectedCatalogRevision)
	if err != nil {
		return model.Recipe{}, err
	}
	current, exists := m.recipes[recipe.ProductID][recipe.ID]
	if !exists {
		return model.Recipe{}, ErrNotFound
	}
	if current.Revision != mutation.ExpectedRevision {
		return model.Recipe{}, ErrConflict
	}
	if err := validateRecipeTransition(current, recipe); err != nil {
		return model.Recipe{}, err
	}
	if err := m.validateRecipeAPIAttachmentsLocked(recipe); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit != nil && m.recipeAuditExistsLocked(mutation.Audit.ID) {
		return model.Recipe{}, ErrConflict
	}

	now := time.Now().UTC()
	recipe.CurrentRevision = nil
	recipe.CreatedAt = current.CreatedAt
	recipe.Revision = current.Revision + 1
	recipe.UpdatedAt = now
	m.recipes[recipe.ProductID][recipe.ID] = memoryClone(recipe)
	if recipeTransitionBumpsCatalog(current, recipe) {
		m.deployment.CatalogRevision++
		m.deployment.UpdatedAt = now
		product.CatalogRevision = m.deployment.CatalogRevision
		product.UpdatedAt = now
		m.products[recipe.ProductID] = product
	}
	if mutation.Audit != nil {
		event := memoryClone(*mutation.Audit)
		event.Outcome = auditOutcome
		m.audit = append(m.audit, event)
	}
	return m.hydrateRecipeLocked(recipe), nil
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

func (m *Memory) SaveRecipeRevision(_ context.Context, recipe model.Recipe, value model.RecipeRevision, mutation RecipeMutation) (model.Recipe, error) {
	if err := validateRecipeMutation(mutation); err != nil {
		return model.Recipe{}, err
	}
	if mutation.Audit == nil {
		return model.Recipe{}, errors.New("recipe revision changes require an audit event")
	}
	_, _, auditOutcome, err := prepareRecipeAudit(recipe, mutation.Audit, "recipe.regrounded", "recipe.reworked", "recipe.references.updated")
	if err != nil {
		return model.Recipe{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if value.RecipeID != recipe.ID {
		return model.Recipe{}, ErrConflict
	}
	value, err = prepareRecipeRevisionRecord(recipe, value)
	if err != nil {
		return model.Recipe{}, err
	}
	if _, err := m.validateRecipeCatalogLocked(recipe.ProductID, mutation.ExpectedCatalogRevision); err != nil {
		return model.Recipe{}, err
	}
	if err := m.validateRecipeAPIBindingsLocked(recipe, value); err != nil {
		return model.Recipe{}, err
	}
	current, exists := m.recipes[recipe.ProductID][recipe.ID]
	if !exists {
		return model.Recipe{}, ErrNotFound
	}
	if current.Revision != mutation.ExpectedRevision {
		return model.Recipe{}, ErrConflict
	}
	if err := validateRecipeRevisionChange(current, recipe); err != nil {
		return model.Recipe{}, err
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
	if mutation.Audit != nil && m.recipeAuditExistsLocked(mutation.Audit.ID) {
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
	if current.State == "published" {
		m.deployment.CatalogRevision++
		m.deployment.UpdatedAt = now
		product := m.products[recipe.ProductID]
		product.CatalogRevision = m.deployment.CatalogRevision
		product.UpdatedAt = now
		m.products[recipe.ProductID] = product
	}
	if mutation.Audit != nil {
		event := memoryClone(*mutation.Audit)
		event.Outcome = auditOutcome
		m.audit = append(m.audit, event)
	}
	return m.hydrateRecipeLocked(recipe), nil
}

func (m *Memory) validateRecipeCatalogLocked(productID string, expected int64) (model.Product, error) {
	product, ok := m.products[productID]
	if !ok || !m.hasDeployment || m.deployment.ID != productID {
		return model.Product{}, ErrNotFound
	}
	if product.CatalogRevision != expected || m.deployment.CatalogRevision != expected {
		return model.Product{}, ErrCatalogConflict
	}
	return product, nil
}

func (m *Memory) recipeAuditExistsLocked(id string) bool {
	for _, event := range m.audit {
		if event.ID == id {
			return true
		}
	}
	return false
}

func (m *Memory) validateRecipeAPIAttachmentsLocked(recipe model.Recipe) error {
	for _, attachment := range recipe.APIAttachments {
		integration, ok := m.integrations[attachment.IntegrationID]
		if !ok || integration.DeploymentID != recipe.ProductID {
			return ErrNotFound
		}
	}
	return nil
}

func (m *Memory) validateRecipeAPIBindingsLocked(recipe model.Recipe, revision model.RecipeRevision) error {
	if err := m.validateRecipeAPIAttachmentsLocked(recipe); err != nil {
		return err
	}
	for _, binding := range revision.APIBindings {
		bound, ok := m.integrationRevisions[binding.IntegrationID][binding.IntegrationRevisionID]
		if !ok {
			return ErrNotFound
		}
		if bound.State != "published" || bound.ManifestHash != binding.IntegrationManifestHash {
			return ErrConflict
		}
	}
	return nil
}
