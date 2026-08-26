import assert from "node:assert/strict";
import test from "node:test";

import { parseRecipeSpecEditor, recipeEditableSpec, recipeSpecEditorValue } from "../app/components/console/dialogs/recipe-spec-editor";
import type { APIRecipe, APIRecipeSpec } from "../app/lib/api";

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

test("recipe editor changes only reviewed reference IDs", () => {
  const result = parseRecipeSpecEditor(JSON.stringify(["doc_errors"]), original);
  assert.equal(result.ok, true);
  if (!result.ok) return;
  assert.deepEqual(result.spec.reference_ids, ["doc_errors"]);
  assert.deepEqual(result.spec.steps, original.steps);
  assert.equal(result.spec.title, original.title);
});

test("recipe editor rejects prose, unknown references, duplicates, and excess", () => {
  const prose = parseRecipeSpecEditor("# Read payment status", original);
  assert.equal(prose.ok, false);

  const object = parseRecipeSpecEditor(JSON.stringify(original), original);
  assert.equal(object.ok, false);
  if (!object.ok) assert.match(object.error, /JSON array/);

  const unknown = parseRecipeSpecEditor(JSON.stringify(["doc_not_reviewed"]), original);
  assert.equal(unknown.ok, false);
  if (!unknown.ok) assert.match(unknown.error, /reviewed for this revision/);

  const duplicate = parseRecipeSpecEditor(JSON.stringify(["doc_payments", "doc_payments"]), original);
  assert.equal(duplicate.ok, false);
  if (!duplicate.ok) assert.match(duplicate.error, /unique/);

  const excess = parseRecipeSpecEditor(JSON.stringify(Array.from({ length: 9 }, () => "doc_payments")), original);
  assert.equal(excess.ok, false);
  if (!excess.ok) assert.match(excess.error, /no more than 8/);
});

test("recipe editor value exposes references rather than server-owned instructions", () => {
  const recipe = {
    contract_version: "product-integration-v2",
    current_revision: { spec_version: 2, spec: original },
  } as unknown as APIRecipe;
  assert.equal(recipeSpecEditorValue(recipe), JSON.stringify(original.reference_ids, null, 2));
});

test("recipe editor excludes immutable legacy revision placeholders", () => {
  const legacy = {
    contract_version: "legacy-mcp-v1",
    current_revision: { spec_version: 1, spec: {} },
  } as unknown as APIRecipe;
  assert.equal(recipeEditableSpec(legacy), undefined);
  const unavailable = parseRecipeSpecEditor("[]", undefined);
  assert.equal(unavailable.ok, false);
});
