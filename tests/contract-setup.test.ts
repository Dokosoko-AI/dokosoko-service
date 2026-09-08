import assert from "node:assert/strict";
import test from "node:test";
import type { APICrawlJob, APIIntegration, APISourceReview } from "../app/lib/api";
import type { APIContract, APIContractBinding, APIContractCandidate, APIContractRevision, APIResourceBindings } from "../app/lib/developer-assets-api";
import { contractBindingMatches, contractCandidateValid, contractSetupPath, ContractSetupChangedError, finishContractSetup, parseContractSetupSelection, pendingContractImport } from "../app/lib/contract-setup";

function fixture() {
  const contract = { id: "contract", deployment_id: "deployment", visibility: "private", lifecycle: "active", revision: 1 } as APIContract;
  const candidate = { id: "candidate", created_at: "2026-09-07T00:00:00Z", normalized_contract: {}, diagnostics: {}, api_contract_id: contract.id, deployment_id: contract.deployment_id, ingestion_run_id: "run", content_hash: "hash", visibility: "private", validation_result: { valid: true, errors: [] } } as APIContractCandidate;
  const review = { source: { id: "source", product_id: "deployment", visibility: "private", quarantined: false, revision: 1 }, crawl_job: { id: "run", failed_count: 0, skipped_count: 0 }, documents: [{ id: "doc", content_hash: "body", state: "validated", injection_indicators: [] }] } as unknown as APISourceReview;
  const origin = { id: "api", deployment_id: "deployment", visibility: "private" } as APIIntegration;
  const revision = { id: "revision", deployment_id: contract.deployment_id, visibility: candidate.visibility, api_contract_candidate_id: candidate.id, api_contract_id: contract.id, content_hash: candidate.content_hash, revision: 1 } as APIContractRevision;
  const state = { origin: structuredClone(origin), contract: structuredClone(contract), review: structuredClone(review), revisions: [] as APIContractRevision[], resources: { contracts: [], documentation: [], sdks: [] } as APIResourceBindings, sourceCalls: 0, publishCalls: 0, attachCalls: 0, changeCalls: 0, checkpoint: [] as string[] };
  const transport = {
    integration: async () => state.origin,
    contract: async () => state.contract,
    revisions: async () => state.revisions,
    review: async () => state.review,
    publishSource: async () => { state.sourceCalls++; state.review.publication = { id: "source-publication" } as APISourceReview["publication"]; state.review.source.revision++; state.review.documents[0].state = "published"; },
    publishContract: async () => { state.publishCalls++; state.revisions = [revision]; state.contract.revision++; return { revision }; },
    resources: async () => state.resources,
    attach: async (apiID: string, contractID: string, revisionID: string, primary: boolean, visibility: APIIntegration["visibility"]) => { state.attachCalls++; state.resources.contracts = [{ id: "binding", deployment_id: origin.deployment_id, api_id: apiID, api_contract_id: contractID, pinned_revision_id: revisionID, follow_latest: false, primary, visibility, lifecycle: "attached", revision: 1 } as APIContractBinding]; },
    change: async (_apiID: string, binding: APIContractBinding, revisionID: string, primary: boolean, visibility: APIIntegration["visibility"]) => { state.changeCalls++; state.resources.contracts = [{ ...binding, pinned_revision_id: revisionID, primary, visibility, follow_latest: false, revision: binding.revision + 1 }]; },
  };
  const input = { contract, candidate, sourceReview: review, api: origin, primary: true };
  const checkpoint = (value: APIContractRevision) => { state.checkpoint.push(value.id); };
  return { input, transport, state, revision, checkpoint };
}

test("contract setup round-trips exact route context and a pending queue boundary", () => {
  const selection = parseContractSetupSelection("?contract=a%2Fb&api=origin&source=source&queue=pending&after=job");
  assert.deepEqual(parseContractSetupSelection(contractSetupPath(selection).split("?")[1]), selection);
  assert.equal(parseContractSetupSelection("?contract=missing&input=new").contract, "missing");
  assert.equal(parseContractSetupSelection("?input=new").input, "new");
  const completed = { id: "new-job", state: "review" } as APICrawlJob;
  const baseline = { id: "job" } as APICrawlJob;
  assert.equal(pendingContractImport([completed, baseline], selection), completed, "a lost response recovers even a finished job");
  assert.equal(pendingContractImport([baseline], selection), undefined);
  assert.throws(() => pendingContractImport([completed], selection), ContractSetupChangedError);
  assert.equal(pendingContractImport([completed], { queue: "pending", after: "" }), completed);
});

test("contract completion requires the exact attachment scope, audience, pin and primary choice", async () => {
  const { input, transport, state, checkpoint, revision } = fixture();
  await finishContractSetup(input, transport, checkpoint);
  const saved = state.resources.contracts[0];
  assert.equal(contractBindingMatches(saved, revision, input.api, true, "private"), true);
  for (const patch of [{ api_id: "other" }, { deployment_id: "other" }, { api_contract_id: "other" }, { pinned_revision_id: "other" }, { follow_latest: true }, { primary: false }, { visibility: "public" }, { lifecycle: "detached" }] as Partial<APIContractBinding>[]) {
    assert.equal(contractBindingMatches({ ...saved, ...patch }, revision, input.api, true, "private"), false, JSON.stringify(patch));
  }
});

test("changing an existing contract pin recovers a lost response with the chosen primary setting", async () => {
  const { input, transport, state, checkpoint, revision } = fixture();
  const binding = { id: "binding", deployment_id: "deployment", api_id: "api", api_contract_id: "contract", pinned_revision_id: "old", follow_latest: false, primary: true, visibility: "private", lifecycle: "attached", revision: 1 } as APIContractBinding;
  state.resources.contracts = [binding];
  const choice = { ...input, primary: false, expectedBinding: binding };
  await assert.rejects(finishContractSetup(choice, { ...transport, change: async (...args) => { await transport.change(...args); throw Error("response lost"); } }, checkpoint), /response lost/);
  await finishContractSetup(choice, transport, checkpoint);
  assert.deepEqual([state.sourceCalls, state.publishCalls, state.changeCalls], [1, 1, 1]);
  assert.equal(contractBindingMatches(state.resources.contracts[0], revision, input.api, false, "private"), true);
});

test("contract setup checks attachment readback and retains a published revision after failure", async () => {
  const { input, transport, state, checkpoint } = fixture();
  await assert.rejects(finishContractSetup(input, { ...transport, attach: async (...args) => { await transport.attach(...args); state.resources.contracts[0].primary = false; } }, checkpoint), ContractSetupChangedError);
  assert.deepEqual(state.checkpoint, ["revision"]);
  assert.equal(state.revisions.length, 1);
});

test("API audience changes before or during publication require renewed review", async () => {
  for (const stage of ["before", "during"] as const) {
    const { input, transport, state, checkpoint } = fixture();
    if (stage === "before") state.origin.visibility = "public";
    await assert.rejects(finishContractSetup(input, { ...transport, publishContract: async () => { const result = await transport.publishContract(); state.origin.visibility = "public"; return result; } }, checkpoint), ContractSetupChangedError);
    assert.equal(state.attachCalls, 0);
    assert.equal(state.publishCalls, stage === "before" ? 0 : 1);
  }
});

test("a reviewed public API attachment uses the public audience when replacing an older private binding", async () => {
  const { input, transport, state, checkpoint, revision } = fixture();
  input.api.visibility = state.origin.visibility = input.contract.visibility = state.contract.visibility = input.candidate.visibility = input.sourceReview.source.visibility = state.review.source.visibility = revision.visibility = "public";
  const binding = { id: "binding", deployment_id: "deployment", api_id: "api", api_contract_id: "contract", pinned_revision_id: "old", follow_latest: false, primary: false, visibility: "private", lifecycle: "attached", revision: 1 } as APIContractBinding;
  state.resources.contracts = [binding];
  await finishContractSetup({ ...input, primary: false, expectedBinding: binding }, transport, checkpoint);
  assert.equal(state.changeCalls, 1);
  assert.equal(contractBindingMatches(state.resources.contracts[0], revision, input.api, false, "public"), true);
});

test("contract setup requires explicit deterministic validation", () => {
  const { input } = fixture();
  assert.equal(contractCandidateValid(input.candidate), true);
  for (const validation_result of [{ status: "passed" }, { valid: true, errors: ["bad"] }, { valid: true, errors: "malformed" }, { valid: false }]) assert.equal(contractCandidateValid({ ...input.candidate, validation_result }), false);
});

test("contract setup publishes source then contract, checkpoints, then attaches exactly", async () => {
  const { input, transport, state, checkpoint, revision } = fixture();
  const result = await finishContractSetup(input, { ...transport, attach: async (...args) => { assert.deepEqual(state.checkpoint, [revision.id]); await transport.attach(...args); } }, checkpoint);
  assert.equal(result.id, revision.id);
  assert.deepEqual([state.sourceCalls, state.publishCalls, state.attachCalls], [1, 1, 1]);
  assert.equal(state.resources.contracts[0].pinned_revision_id, revision.id);
});

for (const phase of ["publishSource", "publishContract", "attach"] as const) {
  test(`lost ${phase} response reuses committed records without duplicating work`, async () => {
    const { input, transport, state, checkpoint } = fixture();
    const broken = { ...transport };
    if (phase === "publishSource") broken.publishSource = async () => { await transport.publishSource(); throw Error("lost response"); };
    if (phase === "publishContract") broken.publishContract = async () => { await transport.publishContract(); throw Error("lost response"); };
    if (phase === "attach") broken.attach = async (...args) => { await transport.attach(...args); throw Error("lost response"); };
    await assert.rejects(finishContractSetup(input, broken, checkpoint), /lost response/);
    await finishContractSetup(input, transport, checkpoint);
    assert.deepEqual([state.sourceCalls, state.publishCalls, state.attachCalls], [1, 1, 1]);
  });
}

test("attachment outage after publication retains the exact revision across retry", async () => {
  const { input, transport, state, checkpoint } = fixture();
  await assert.rejects(finishContractSetup(input, { ...transport, attach: async () => { throw Error("offline"); } }, checkpoint));
  assert.deepEqual(state.checkpoint, ["revision"]);
  await finishContractSetup(input, transport, checkpoint);
  assert.deepEqual([state.sourceCalls, state.publishCalls, state.attachCalls], [1, 1, 1]);
});

test("concurrent API attachment change is not overwritten", async () => {
  const { input, transport, state, checkpoint } = fixture();
  const expected = { id: "binding", api_contract_id: "contract", pinned_revision_id: "old", primary: true, visibility: "private", lifecycle: "attached", revision: 1 } as APIContractBinding;
  state.resources.contracts = [{ ...expected, revision: 2, pinned_revision_id: "someone-else" }];
  await assert.rejects(finishContractSetup({ ...input, expectedBinding: expected }, transport, checkpoint), ContractSetupChangedError);
  assert.equal(state.changeCalls, 0);
  assert.equal(state.resources.contracts[0].pinned_revision_id, "someone-else");
});

for (const mutation of ["content", "audience", "quarantine", "failed", "root-revision", "foreign-deployment", "private-for-public"] as const) {
  test(`contract setup rejects changed ${mutation} before publishing`, async () => {
    const { input, transport, state, checkpoint } = fixture();
    if (mutation === "content") state.review.documents[0].content_hash = "new-body";
    if (mutation === "audience") state.review.source.visibility = "public";
    if (mutation === "quarantine") state.review.source.quarantined = true;
    if (mutation === "failed") state.review.crawl_job.failed_count = 1;
    if (mutation === "root-revision") state.contract.revision++;
    if (mutation === "foreign-deployment") input.api.deployment_id = "other";
    if (mutation === "private-for-public") input.api.visibility = "public";
    await assert.rejects(finishContractSetup(input, transport, checkpoint), ContractSetupChangedError);
    assert.deepEqual([state.sourceCalls, state.publishCalls, state.attachCalls], [0, 0, 0]);
  });
}
