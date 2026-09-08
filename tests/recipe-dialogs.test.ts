import assert from "node:assert/strict";
import test from "node:test";

import { recipeEditableSpec, recipeReferenceOptionsMatch, validRecipeReferenceSelection } from "../app/components/console/dialogs/recipe-spec-editor";
import type { APIRecipe, APIRecipeReferenceOptions, APIRecipeSpec } from "../app/lib/api";

const evidence = [{ kind: "tool", resource_id: "tool_payments_read", fingerprint: "sha256:reviewed" }];
const original: APIRecipeSpec = {
  schema_version: 2,
  integration_id: "integration_payments",
  title: "Read payment status",
  outcome: "The application reads one payment status.",
  capability_ids: ["tool_payments_read"],
  prerequisites: [],
  steps: [
    { action: "Add a payment-status client operation.", expected_result: "The project has one explicit payment integration boundary.", evidence },
    { action: "Map the reviewed payment status response.", expected_result: "The result is bounded by the reviewed response schema.", evidence },
  ],
  checks: [
    { action: "Run a focused payment-status contract test.", expected_result: "The focused contract test passes.", evidence },
  ],
  reference_ids: ["doc_payments", "doc_errors"],
};


const recipe = {
  id: "recipe_payments", product_id: "prod_payments", revision: 7, current_revision_id: "revision_3",
  contract_version: "product-integration-v2",
  current_revision: { id: "revision_3", recipe_id: "recipe_payments", spec_version: 2, spec: original },
} as unknown as APIRecipe;
const options: APIRecipeReferenceOptions = {
  product_id: recipe.product_id, recipe_id: recipe.id, recipe_revision: recipe.revision,
  current_revision_id: recipe.current_revision_id,
  items: ["doc_payments", "doc_errors", "doc_retries"].map((id) => ({
    reference: { resource_id: id, label: id, kind: "documentation", url: `https://example.test/${id}` },
    evidence: [{ kind: "source_publication", resource_id: "publication_1", label: "Payment guide", visibility: "private", fingerprint: "sha256:exact", excerpt: "Use the payment status endpoint." }],
  })),
};

test("recipe picker accepts exact supported choices, including a removed reference", () => {
  assert.equal(recipeReferenceOptionsMatch(recipe, options), true);
  assert.equal(validRecipeReferenceSelection(["doc_retries"], options), true);
  assert.equal(validRecipeReferenceSelection([], options), true);
  assert.equal(recipeEditableSpec(recipe), original);
});

test("recipe picker rejects unknown, duplicate, whitespace and excessive choices", () => {
  for (const choices of [["unrelated"], ["doc_payments", "doc_payments"], [" doc_payments"], Array(9).fill("doc_payments")]) {
    assert.equal(validRecipeReferenceSelection(choices, options), false);
  }
});

test("recipe picker rejects evidence from a different recipe, deployment or revision", () => {
  for (const changed of [
    { product_id: "another-product" }, { recipe_id: "another-recipe" },
    { recipe_revision: 8 }, { current_revision_id: "revision_4" },
  ]) assert.equal(recipeReferenceOptionsMatch(recipe, { ...options, ...changed }), false);
  const stale = { ...recipe, current_revision_id: "revision_4" };
  assert.equal(recipeEditableSpec(stale), undefined);
  assert.equal(recipeReferenceOptionsMatch(stale, options), false);
});

test("recipe picker rejects unsafe links, ambiguous IDs and unsupported evidence", () => {
  const item = options.items[0];
  for (const url of ["javascript:alert(1)", "https://user:password@example.test/doc", "/relative"]) {
    assert.equal(recipeReferenceOptionsMatch(recipe, { ...options, items: [{ ...item, reference: { ...item.reference, url } }] }), false);
  }
  assert.equal(recipeReferenceOptionsMatch(recipe, { ...options, items: [item, item] }), false);
  assert.equal(recipeReferenceOptionsMatch(recipe, { ...options, items: [{ ...item, evidence: [] }] }), false);
});

test("recipe picker excludes legacy and mismatched instruction schemas", () => {
  assert.equal(recipeEditableSpec({ ...recipe, contract_version: "legacy-mcp-v1" }), undefined);
  assert.equal(recipeEditableSpec({ ...recipe, contract_version: "deployment-recipe-v3", api_attachments: [{ integration_id: "integration_payments" }] }), undefined);
  assert.equal(recipeReferenceOptionsMatch(recipe, { ...options, items: [] }), true);
});
