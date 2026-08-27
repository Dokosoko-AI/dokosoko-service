import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { createRecipeApprovalReview } from "../app/components/console/dialogs/recipe-approval-review";
import type { APIIntegration, APIRecipe, APIRecipeRevision } from "../app/lib/api";

const revision = {
  id: "recipe_revision_7",
  recipe_id: "recipe_1",
  revision: 7,
  spec_version: 2,
  spec: {
    schema_version: 2,
    integration_id: "integration_payments",
    title: "Read a payment",
    outcome: "The application reads one payment.",
    capability_ids: ["payments.read"],
    prerequisites: [],
    steps: [],
    checks: [],
  },
  markdown: "# Read a payment",
  references: [],
  validation: [],
  generated_by: "ai",
  model: "gpt-review",
  integration_revision_id: "integration_revision_3",
  integration_manifest_hash: "sha256:manifest",
  prompt_version: "recipe.authoring:4",
  prompt_hash: "sha256:prompt",
  created_by: "user_1",
  created_at: "2026-08-26T00:00:00Z",
} as unknown as APIRecipeRevision;

const integration = {
  id: "integration_payments",
  display_name: "Payments API",
} as unknown as APIIntegration;

function recipe(currentRevisionID: string, currentRevision: APIRecipeRevision | undefined): APIRecipe {
  return {
    id: "recipe_1",
    contract_version: "product-integration-v2",
    integration_id: "integration_payments",
    current_revision_id: currentRevisionID,
    current_revision: currentRevision,
  } as unknown as APIRecipe;
}

test("approval review is pinned to the exact current revision and its integration", () => {
  const review = createRecipeApprovalReview(recipe(revision.id, revision), [integration]);
  assert.ok(review);
  assert.equal(review.currentRevisionID, revision.id);
  assert.equal(review.revision, revision);
  assert.deepEqual(review.integrations, [integration]);
});

test("approval review fails closed for mismatched or missing current revision data", () => {
  assert.equal(createRecipeApprovalReview(recipe("recipe_revision_8", revision), [integration]), null);
  assert.equal(createRecipeApprovalReview(recipe(revision.id, undefined), [integration]), null);
  assert.equal(createRecipeApprovalReview({ ...recipe(revision.id, revision), id: "recipe_2" } as APIRecipe, [integration]), null);
  const mismatchedIntegrationRevision = {
    ...revision,
    spec: { ...revision.spec, integration_id: "integration_other" },
  } as APIRecipeRevision;
  assert.equal(createRecipeApprovalReview(recipe(mismatchedIntegrationRevision.id, mismatchedIntegrationRevision), [integration]), null);
});

test("approval review resolves every API on a deployment recipe", () => {
  const integrations = [
    { id: "integration_billing", display_name: "Billing API" },
    { id: "integration_customers", display_name: "Customers API" },
  ] as APIIntegration[];
  const multiRevision = {
    ...revision,
    id: "recipe_revision_multi",
    spec_version: 3,
    spec: {
      schema_version: 3,
      api_attachments: [{ integration_id: "integration_billing" }, { integration_id: "integration_customers" }],
      title: "Provision a customer",
      outcome: "The application provisions a customer and billing account.",
      capability_ids: ["customers.create", "billing.create"],
      prerequisites: [], steps: [], checks: [],
    },
    integration_revision_id: undefined,
    integration_manifest_hash: undefined,
    api_bindings: [
      { integration_id: "integration_customers", integration_revision_id: "customers-r1", integration_manifest_hash: "sha256:customers" },
      { integration_id: "integration_billing", integration_revision_id: "billing-r2", integration_manifest_hash: "sha256:billing" },
    ],
  } as unknown as APIRecipeRevision;
  const multiRecipe = {
    id: "recipe_1",
    contract_version: "deployment-recipe-v3",
    api_attachments: [{ integration_id: "integration_customers" }, { integration_id: "integration_billing" }],
    current_revision_id: multiRevision.id,
    current_revision: multiRevision,
  } as unknown as APIRecipe;

  const review = createRecipeApprovalReview(multiRecipe, integrations);
  assert.ok(review);
  assert.deepEqual(review.integrations.map((value) => value.id), ["integration_billing", "integration_customers"]);
  assert.equal(createRecipeApprovalReview({
    ...multiRecipe,
    current_revision: { ...multiRevision, api_bindings: multiRevision.api_bindings?.slice(0, 1) },
  } as APIRecipe, integrations), null);
});

test("recipe approval is exposed only from the exact-revision review dialog", async () => {
  const view = await readFile(new URL("../app/components/console/catalog-settings-views.tsx", import.meta.url), "utf8");
  const dialog = await readFile(new URL("../app/components/console/dialogs/recipe-approval-dialog.tsx", import.meta.url), "utf8");
  const review = await readFile(new URL("../app/components/console/dialogs/recipe-approval-review.ts", import.meta.url), "utf8");
  const styles = await readFile(new URL("../app/styles/console-catalog.css", import.meta.url), "utf8");

  assert.match(view, /const approvalCandidate = createRecipeApprovalReview\(recipe, integrations\)/);
  assert.match(view, /setApprovalReview\(approvalCandidate\)/);
  assert.doesNotMatch(view, /onClick=\{[^}]*onApprove/);
  assert.match(review, /revision\.id !== recipe\.current_revision_id/);
  assert.match(dialog, /await onApprove\(recipe\)/);
  assert.match(dialog, /recipeApproval\.approveRevision2/);

  for (const labelKey of [
    "canonicalMarkdown",
    "currentRevisionID",
    "integrationRevisionID",
    "integrationManifestHash",
    "generatedBy",
    "model",
    "promptVersion",
    "promptHash",
    "validationFindings",
    "references",
    "revisionNumber",
  ]) {
    assert.match(dialog, new RegExp(`recipeApproval\\.${labelKey}`));
  }
  assert.match(styles, /\.dialog-panel:has\(\.recipe-approval-review\)/);
  assert.match(dialog, /role="region" aria-label=\{t\("recipeApproval\.canonicalMarkdownForRevision"/);
});
