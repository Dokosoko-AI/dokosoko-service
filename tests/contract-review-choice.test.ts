import assert from "node:assert/strict";
import test from "node:test";
import type { APIIntegration, APISourceReview } from "../app/lib/api";
import type { APIContract, APIContractCandidate } from "../app/lib/developer-assets-api";
import { contractReviewChoiceScope, validContractReviewChoice } from "../app/lib/contract-review-choice";
import { readReviewDraft, writeReviewDraft } from "../app/lib/review-draft";

function fixture() {
  const input = {
    contract: { id: "contract", visibility: "private", lifecycle: "active", revision: 1 } as APIContract,
    candidate: { id: "candidate", api_contract_id: "contract", deployment_id: "deployment", ingestion_run_id: "run", content_hash: "hash", visibility: "private" } as APIContractCandidate,
    sourceReview: { source: { id: "source", product_id: "deployment", visibility: "private", quarantined: false, revision: 1 }, crawl_job: { id: "run", failed_count: 0, skipped_count: 0 }, documents: [{ id: "document", state: "validated", content_hash: "source-hash", injection_indicators: [] }] } as unknown as APISourceReview,
    api: { id: "api", deployment_id: "deployment", visibility: "private" } as APIIntegration,
  };
  const values = new Map<string, string>();
  const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
  return { input, storage, values };
}

test("contract attachment choice survives publication and reload without storing approval", () => {
  const { input, storage, values } = fixture();
  const scope = contractReviewChoiceScope(input, "reviewer");
  assert(writeReviewDraft(storage, scope.key, scope.fingerprint, { primary: false }));
  input.contract.revision++;
  input.sourceReview.source.revision++;
  input.sourceReview.documents[0].state = "published";
  const resumed = contractReviewChoiceScope(input, "reviewer");
  assert.deepEqual(readReviewDraft(storage, resumed.key, resumed.fingerprint, validContractReviewChoice), { primary: false });
  assert.doesNotMatch(values.get(scope.key)!, /acknowledged|normalized_contract|body|reviewed_by/);
  assert.equal(validContractReviewChoice({ primary: false, acknowledged: true }), false);
  assert.equal(validContractReviewChoice({ primary: "false" }), false);
});

test("changed contract review or attachment context cannot recover an old choice", () => {
  const { input, storage } = fixture();
  const scope = contractReviewChoiceScope(input, "reviewer");
  writeReviewDraft(storage, scope.key, scope.fingerprint, { primary: false });
  const mutations: Array<(value: typeof input) => void> = [
    (v) => { v.candidate.id = "other"; }, (v) => { v.candidate.content_hash = "changed"; },
    (v) => { v.candidate.visibility = "public"; }, (v) => { v.candidate.deployment_id = "other"; },
    (v) => { v.sourceReview.documents[0].content_hash = "changed"; }, (v) => { v.sourceReview.documents[0].state = "quarantined"; },
    (v) => { v.sourceReview.crawl_job.failed_count = 1; }, (v) => { v.sourceReview.source.visibility = "public"; },
    (v) => { v.api.id = "other"; }, (v) => { v.api.visibility = "public"; },
  ];
  for (const mutate of mutations) {
    const changed = structuredClone(input); mutate(changed);
    const fresh = contractReviewChoiceScope(changed, "reviewer");
    assert.equal(readReviewDraft(storage, fresh.key, fresh.fingerprint, validContractReviewChoice), null);
  }
  const foreign = contractReviewChoiceScope(input, "other-reviewer");
  assert.equal(readReviewDraft(storage, foreign.key, foreign.fingerprint, validContractReviewChoice), null);
});
