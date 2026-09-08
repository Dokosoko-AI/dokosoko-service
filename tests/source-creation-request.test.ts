import assert from "node:assert/strict";
import test from "node:test";
import { finishSourceCreationAttempt, sourceCreationAttempt, sourceCreationFingerprint } from "../app/lib/source-creation-request";
import { reviewDraftKey } from "../app/lib/review-draft";

function storage() {
  const values = new Map<string, string>();
  return { values, getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
}

test("source creation retry restores only its key and input digest after reload", async () => {
  const cache = storage();
  const scope = reviewDraftKey("deployment", "reviewer", "source-creation", "contract");
  const fingerprint = await sourceCreationFingerprint({ kind: "website", location: " https://example.test/private-guide " });
  const first = sourceCreationAttempt(cache, scope, fingerprint);
  const reloaded = sourceCreationAttempt(cache, scope, fingerprint);
  assert.deepEqual(reloaded.attempt, first.attempt);
  assert.equal(reloaded.persisted, true);
  assert.equal(cache.values.get(scope)?.includes("private-guide"), false);
  const otherReviewer = sourceCreationAttempt(cache, reviewDraftKey("deployment", "other", "source-creation", "contract"), fingerprint);
  assert.notEqual(otherReviewer.attempt.key, first.attempt.key);
  finishSourceCreationAttempt(cache, scope);
  assert.notEqual(sourceCreationAttempt(cache, scope, fingerprint).attempt.key, first.attempt.key);
});

test("source request identity changes for changed content, kind or filename", async () => {
  const input = { kind: "upload", file: new File(["Reviewed guidance"], "guide.md") };
  const fingerprint = await sourceCreationFingerprint(input);
  assert.equal(await sourceCreationFingerprint({ ...input, file: new File(["Reviewed guidance"], "guide.md") }), fingerprint);
  for (const changed of [
    { kind: "upload", file: new File(["Changed guidance"], "guide.md") },
    { kind: "upload", file: new File(["Reviewed guidance"], "guide.txt") },
    { kind: "openapi", location: "https://example.test/guide.md" },
  ]) assert.notEqual(await sourceCreationFingerprint(changed), fingerprint);
  const cache = storage();
  const first = sourceCreationAttempt(cache, "scope", fingerprint).attempt;
  assert.notEqual(sourceCreationAttempt(cache, "scope", "changed", first).attempt.key, first.key);
});

test("source request recovery reports unavailable storage while retaining the current attempt", () => {
  const unavailable = { getItem: () => { throw new Error("denied"); }, setItem: () => { throw new Error("denied"); }, removeItem: () => {} };
  const first = sourceCreationAttempt(unavailable, "scope", "fingerprint");
  const retry = sourceCreationAttempt(unavailable, "scope", "fingerprint", first.attempt);
  assert.equal(first.persisted, false);
  assert.deepEqual(retry.attempt, first.attempt);
});

test("source fingerprint rejects oversized files before reading their bytes", async () => {
  const oversized = { size: 5_000_001, arrayBuffer: () => { throw new Error("must not read"); } } as unknown as File;
  await assert.rejects(sourceCreationFingerprint({ kind: "upload", file: oversized }), /5,000,000/);
});
