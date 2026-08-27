import type { APIIntegration, APIRecipe, APIRecipeRevision } from "../../../lib/api";

export type RecipeApprovalReview = Readonly<{
  recipe: APIRecipe;
  revision: APIRecipeRevision;
  currentRevisionID: string;
  integrations: APIIntegration[];
}>;

export function createRecipeApprovalReview(recipe: APIRecipe, integrations: APIIntegration[]): RecipeApprovalReview | null {
  const revision = recipe.current_revision;
  if (!revision || revision.id !== recipe.current_revision_id || revision.recipe_id !== recipe.id) return null;

  const recipeIDs = recipe.contract_version === "product-integration-v2"
    ? [recipe.integration_id].filter((value): value is string => Boolean(value))
    : (recipe.api_attachments ?? []).map((item) => item.integration_id).sort();
  const specIDs = revision.spec_version === 2
    ? [revision.spec.integration_id]
    : (revision.spec.api_attachments ?? []).map((item) => item.integration_id).sort();
  const bindingIDs = revision.spec_version === 2
    ? [revision.integration_revision_id ? revision.spec.integration_id : ""].filter(Boolean)
    : (revision.api_bindings ?? []).map((item) => item.integration_id).sort();
  if (recipeIDs.length === 0 || recipeIDs.join("\u0000") !== specIDs.join("\u0000") || recipeIDs.join("\u0000") !== bindingIDs.join("\u0000")) return null;
  const resolvedIntegrations = recipeIDs.map((id) => integrations.find((integration) => integration.id === id)).filter((value): value is APIIntegration => Boolean(value));
  if (resolvedIntegrations.length !== recipeIDs.length) return null;
  return {
    recipe,
    revision,
    currentRevisionID: recipe.current_revision_id,
    integrations: resolvedIntegrations,
  };
}
