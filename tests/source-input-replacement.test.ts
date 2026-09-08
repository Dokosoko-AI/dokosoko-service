import assert from "node:assert/strict";
import test from "node:test";
import { finishSourceInputReplacement, readSourceInputReplacement, sourceInputReplacementAttempt } from "../app/lib/source-input-replacement";

function storage() {
  const values = new Map<string, string>();
  return { values, getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
}

test("replacement recovery retains the original request and revision after the source advances", () => {
  const saved = storage(), scope = "deployment:reviewer:source", fingerprint = "a".repeat(64);
  const first = sourceInputReplacementAttempt(saved, scope, fingerprint, 7);
  assert(first.persisted);
  assert.deepEqual(readSourceInputReplacement(saved, scope), first.attempt);
  const retried = sourceInputReplacementAttempt(saved, scope, fingerprint, 8);
  assert.deepEqual(retried.attempt, first.attempt);
  assert.equal(retried.attempt.revision, 7);
  assert.equal(readSourceInputReplacement(saved, "another-reviewer"), null);
  const changed = sourceInputReplacementAttempt(saved, scope, "b".repeat(64), 8);
  assert.notEqual(changed.attempt.key, first.attempt.key);
  assert.equal(changed.attempt.revision, 8);
  finishSourceInputReplacement(saved, scope);
  assert.equal(readSourceInputReplacement(saved, scope), null);
});

test("replacement recovery handles storage failure while retaining an in-memory request", () => {
  const broken = { getItem: () => { throw Error("unavailable"); }, setItem: () => { throw Error("unavailable"); }, removeItem: () => {} };
  const first = sourceInputReplacementAttempt(broken, "scope", "a".repeat(64), 2);
  assert.equal(first.persisted, false);
  const retry = sourceInputReplacementAttempt(broken, "scope", "a".repeat(64), 3, first.attempt);
  assert.equal(retry.attempt.key, first.attempt.key);
  assert.equal(retry.attempt.revision, 2);
});
