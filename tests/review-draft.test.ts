import assert from "node:assert/strict";
import test from "node:test";
import { clearReviewDraft, readReviewDraft, reviewDraftKey, writeReviewDraft } from "../app/lib/review-draft";
import { isSDKReviewDraft } from "../app/components/console/developer-assets/sdk-catalog-helpers";

test("review drafts resume only for the same operator, deployment, candidate and content", () => {
  const data = new Map<string, string>();
  const storage = { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => { data.set(key, value); }, removeItem: (key: string) => { data.delete(key); } };
  const key = reviewDraftKey("deployment", "alice", "sdk", "candidate");
  const draft = { files: { file: { decision: "included", reason: "", reviewEvidence: "" } }, samples: {} };
  assert.equal(writeReviewDraft(storage, key, "hash-a", draft), true);
  assert.deepEqual(readReviewDraft(storage, key, "hash-a", isSDKReviewDraft), draft);
  assert.equal(readReviewDraft(storage, key, "hash-b", isSDKReviewDraft), null);
  for (const other of [reviewDraftKey("deployment", "bob", "sdk", "candidate"), reviewDraftKey("another", "alice", "sdk", "candidate"), reviewDraftKey("deployment", "alice", "sdk", "new-candidate")]) assert.equal(readReviewDraft(storage, other, "hash-a", isSDKReviewDraft), null);
  assert.doesNotMatch(data.get(key)!, /acknowledged|approved_by/);
  clearReviewDraft(storage, key);
  assert.equal(readReviewDraft(storage, key, "hash-a", isSDKReviewDraft), null);
  assert.equal(reviewDraftKey("deployment", "", "sdk", "candidate"), "");
});

test("unavailable or malformed browser storage never approves or blocks a candidate", () => {
  const storage = { getItem: () => "{bad-json", setItem: () => { throw new Error("quota"); }, removeItem: () => {} };
  assert.equal(readReviewDraft(storage, "key", "hash", isSDKReviewDraft), null);
  assert.equal(writeReviewDraft(storage, "key", "hash", {}), false);
  assert.equal(isSDKReviewDraft({ files: { id: { decision: "publish", reason: "", reviewEvidence: "" } }, samples: {} }), false);
});
