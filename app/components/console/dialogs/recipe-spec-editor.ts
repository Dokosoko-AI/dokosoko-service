import type { APIRecipe, APIRecipeSpec } from "../../../lib/api";

export type RecipeSpecParseResult =
  | { ok: true; spec: APIRecipeSpec }
  | { ok: false; error: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseRecipeSpecEditor(value: string, original?: APIRecipeSpec): RecipeSpecParseResult {
  if (!original) return { ok: false, error: "This recipe has no editable product-integration spec." };

  let candidate: unknown;
  try {
    candidate = JSON.parse(value);
  } catch (error) {
    return { ok: false, error: `Enter a valid JSON array${error instanceof Error ? `: ${error.message}` : "."}` };
  }
  if (!Array.isArray(candidate)) return { ok: false, error: "Reference IDs must be one JSON array." };
  if (candidate.length > 8) return { ok: false, error: "Select no more than 8 reference IDs." };

  const referenceIDs = candidate.filter((item): item is string => (
    typeof item === "string"
    && item.length > 0
    && item.length <= 500
    && item === item.trim()
  ));
  if (referenceIDs.length !== candidate.length || new Set(referenceIDs).size !== referenceIDs.length) {
    return { ok: false, error: "Reference IDs must be unique, non-empty strings without surrounding whitespace." };
  }

  const reviewedReferenceIDs = new Set(original.reference_ids ?? []);
  if (referenceIDs.some((id) => !reviewedReferenceIDs.has(id))) {
    return { ok: false, error: "Reference IDs must reuse references reviewed for this revision." };
  }

  return { ok: true, spec: { ...original, reference_ids: referenceIDs } };
}

export function recipeEditableSpec(recipe: APIRecipe): APIRecipeSpec | undefined {
  const revision = recipe.current_revision;
  if (
    recipe.contract_version !== "product-integration-v2"
    || revision?.spec_version !== 2
    || !isRecord(revision.spec)
    || revision.spec.schema_version !== 2
  ) return undefined;
  return revision.spec as unknown as APIRecipeSpec;
}

export function recipeSpecEditorValue(recipe: APIRecipe) {
  const spec = recipeEditableSpec(recipe);
  if (!spec) return "";
  return JSON.stringify(Array.isArray(spec.reference_ids) ? spec.reference_ids : [], null, 2);
}
