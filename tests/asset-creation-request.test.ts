import assert from "node:assert/strict";
import test from "node:test";
import { assetCreationAttempt } from "../app/lib/asset-creation-request";
import { clearReviewDraft } from "../app/lib/review-draft";

const scope = { deploymentID: "deployment", reviewerID: "reviewer", kind: "documentation_collection" as const, context: "api-one" };
const input = { name: "Private guide", visibility: "private", members: [{ id: "exact-reviewed-source", selector: { documents: ["document-one"], sections: [] } }], acknowledge_reviewed: true };
function storage() {
  const values = new Map<string, string>();
  return { values, getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
}

test("asset creation recovers after reload without storing content or approval and clears after completion", async () => {
  const cache = storage();
  const first = await assetCreationAttempt(cache, scope, input);
  const reordered = { acknowledge_reviewed: true, members: [{ selector: { sections: [], documents: ["document-one"] }, id: "exact-reviewed-source" }], visibility: "private", name: "Private guide" };
  const retry = await assetCreationAttempt(cache, scope, reordered);
  assert.deepEqual(retry, first);
  assert(first.persisted);
  for (const secret of ["Private guide", "exact-reviewed-source", "acknowledge_reviewed"]) assert(!cache.values.get(first.scope)?.includes(secret));
  clearReviewDraft(cache, first.scope);
  assert.notEqual((await assetCreationAttempt(cache, scope, input)).attempt.key, first.attempt.key);
});

test("creation identities isolate administrators, APIs, deployments, kinds and exact input", async () => {
  const cache = storage(), first = await assetCreationAttempt(cache, scope, input);
  for (const other of [{ ...scope, reviewerID: "other" }, { ...scope, deploymentID: "other" }, { ...scope, context: "api-two" }, { ...scope, kind: "api_contract" as const }]) assert.notEqual((await assetCreationAttempt(cache, other, input)).attempt.key, first.attempt.key);
  for (const changed of [{ ...input, name: "Changed" }, { ...input, visibility: "public" }, { ...input, members: [{ id: "different-reviewed-source" }] }]) assert.notEqual((await assetCreationAttempt(cache, scope, changed, first.attempt)).attempt.key, first.attempt.key);
});

test("storage denial retains an in-page creation key and reports reload recovery unavailable", async () => {
  const denied = { getItem() { throw new Error("denied"); }, setItem() { throw new Error("denied"); }, removeItem() {} };
  const first = await assetCreationAttempt(denied, scope, input);
  const retry = await assetCreationAttempt(denied, scope, input, first.attempt);
  assert(!first.persisted);
  assert.equal(retry.attempt.key, first.attempt.key);
});
