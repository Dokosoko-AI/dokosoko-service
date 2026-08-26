import type { APIIntegration, APIRecipe, APIRecipeRevision } from "../../../lib/api";

export type RecipeApprovalReview = Readonly<{
  recipe: APIRecipe;
  revision: APIRecipeRevision;
  currentRevisionID: string;
  integration?: APIIntegration;
}>;

export function createRecipeApprovalReview(recipe: APIRecipe, integrations: APIIntegration[]): RecipeApprovalReview | null {
  const revision = recipe.current_revision;
  if (!revision || revision.id !== recipe.current_revision_id || revision.recipe_id !== recipe.id) return null;

  if (revision.spec_version === 2 && revision.spec.integration_id !== recipe.integration_id) return null;

  const integrationID = revision.spec_version === 2 ? revision.spec.integration_id : recipe.integration_id;
  return {
    recipe,
    revision,
    currentRevisionID: recipe.current_revision_id,
    integration: integrations.find((integration) => integration.id === integrationID),
  };
}
