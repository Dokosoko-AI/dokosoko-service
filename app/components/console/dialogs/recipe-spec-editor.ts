import type { APIRecipe, APIRecipeReferenceOptions, APIRecipeSpec } from "../../../lib/api";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function recipeEditableSpec(recipe: APIRecipe): APIRecipeSpec | undefined {
  const revision = recipe.current_revision;
  const version = recipe.contract_version === "product-integration-v2" ? 2
    : recipe.contract_version === "deployment-recipe-v3" ? 3 : undefined;
  if (!version || !revision || revision.id !== recipe.current_revision_id
    || revision.recipe_id !== recipe.id || revision.spec_version !== version
    || !isRecord(revision.spec) || revision.spec.schema_version !== version) return undefined;
  return revision.spec as unknown as APIRecipeSpec;
}

export function recipeReferenceOptionsMatch(recipe: APIRecipe, options: APIRecipeReferenceOptions): boolean {
  if (!recipeEditableSpec(recipe) || options.product_id !== recipe.product_id || options.recipe_id !== recipe.id
    || options.recipe_revision !== recipe.revision || options.current_revision_id !== recipe.current_revision_id
    || !Array.isArray(options.items)) return false;
  const ids = new Set<string>();
  return options.items.every(({ reference, evidence }) => {
    const id = reference?.resource_id;
    if (!id || id !== id.trim() || ids.has(id) || !Array.isArray(evidence) || evidence.length === 0) return false;
    try {
      const url = new URL(reference.url);
      if (url.protocol !== "https:" || !url.hostname || url.username || url.password) return false;
    } catch { return false; }
    ids.add(id);
    return true;
  });
}

export function validRecipeReferenceSelection(selected: readonly string[], options: APIRecipeReferenceOptions): boolean {
  const allowed = new Set(options.items.map(({ reference }) => reference.resource_id));
  return selected.length <= 8 && new Set(selected).size === selected.length
    && selected.every((id) => typeof id === "string" && Boolean(id) && id === id.trim() && allowed.has(id));
}
