package platform

import (
	"context"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type RecipeReferenceEvidence struct {
	Kind        string           `json:"kind"`
	ResourceID  string           `json:"resource_id"`
	Label       string           `json:"label"`
	Excerpt     string           `json:"excerpt,omitempty"`
	Version     string           `json:"version,omitempty"`
	Visibility  model.Visibility `json:"visibility"`
	Fingerprint string           `json:"fingerprint"`
}

type RecipeReferenceOption struct {
	Reference model.RecipeReference     `json:"reference"`
	Evidence  []RecipeReferenceEvidence `json:"evidence"`
}

type RecipeReferenceOptions struct {
	ProductID         string                  `json:"product_id"`
	RecipeID          string                  `json:"recipe_id"`
	RecipeRevision    int64                   `json:"recipe_revision"`
	CurrentRevisionID string                  `json:"current_revision_id"`
	Items             []RecipeReferenceOption `json:"items"`
}

func (s *Service) recipeReferenceEvidence(ctx context.Context, product model.Product, recipe model.Recipe) (model.IntegrationAnalysis, []model.IntegrationEvidence, error) {
	analysis, err := s.store.IntegrationAnalysis(ctx, product.ID, recipe.AnalysisID)
	if err != nil {
		return analysis, nil, err
	}
	analysis, err = s.relevantRecipeAnalysis(ctx, product, analysis, recipe.Outcome)
	if err != nil {
		return analysis, nil, err
	}
	selected, ok := recipeEvidenceForDependencies(analysis.Evidence, recipe.Dependencies)
	if !ok {
		return analysis, nil, ErrRecipeGroundingChanged
	}
	return analysis, selected, nil
}

// Reference options are a read-only view of the exact dependency evidence used
// by UpdateRecipeReferences. They never widen selection to the current catalog,
// run AI, or mark a recipe outdated as a side effect of opening the picker.
func (s *Service) RecipeReferenceOptions(ctx context.Context, productID, recipeID string, expectedRevision int64, expectedCurrentRevisionID string) (RecipeReferenceOptions, error) {
	product, err := s.store.Product(ctx, productID)
	if err != nil {
		return RecipeReferenceOptions{}, err
	}
	recipe, err := s.store.Recipe(ctx, productID, recipeID)
	if err != nil {
		return RecipeReferenceOptions{}, err
	}
	if err := requireExpectedRecipeRevision(recipe, expectedRevision, expectedCurrentRevisionID); err != nil {
		return RecipeReferenceOptions{}, err
	}
	current, err := s.recipeGroundingCurrent(ctx, product, recipe, nil)
	if err != nil {
		return RecipeReferenceOptions{}, err
	}
	if !current {
		return RecipeReferenceOptions{}, ErrRecipeGroundingChanged
	}
	_, selected, err := s.recipeReferenceEvidence(ctx, product, recipe)
	if err != nil {
		return RecipeReferenceOptions{}, err
	}
	result := RecipeReferenceOptions{ProductID: product.ID, RecipeID: recipe.ID, RecipeRevision: recipe.Revision, CurrentRevisionID: recipe.CurrentRevisionID, Items: []RecipeReferenceOption{}}
	for _, reference := range recipeReferences(selected) {
		if reference.ResourceID == "" {
			continue
		}
		option := RecipeReferenceOption{Reference: reference, Evidence: []RecipeReferenceEvidence{}}
		for _, item := range selected {
			for _, supported := range recipeReferences([]model.IntegrationEvidence{item}) {
				if supported.ResourceID == reference.ResourceID && supported.URL == reference.URL {
					option.Evidence = append(option.Evidence, RecipeReferenceEvidence{Kind: item.Kind, ResourceID: item.ResourceID, Label: item.Label, Excerpt: item.Excerpt, Version: item.Version, Visibility: item.Visibility, Fingerprint: item.Fingerprint})
					break
				}
			}
		}
		result.Items = append(result.Items, option)
	}
	return result, nil
}
