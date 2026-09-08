import assert from "node:assert/strict";
import test from "node:test";
import type { APIIntegration } from "../app/lib/api";
import type { APISDKBinding, APIResourceBindings, SDKContentCandidate, SDKContentPublication, SDKContentPublicationRecord, SDKPackage, SDKRelease, SDKReleaseLifecycleState } from "../app/lib/developer-assets-api";
import { fingerprintSDKInput, finishSDKSetup, parseSDKSetupSelection, recoverSDKInput, SDKSetupChangedError, sdkSetupBinding, sdkSetupBindingMatches, sdkSetupPath, sdkReviewMatchesPublication } from "../app/lib/sdk-setup";

function fixture() {
 const pkg = { id: "package", deployment_id: "deployment", visibility: "private", lifecycle: "active", revision: 1 } as SDKPackage;
 const release = { id: "release", sdk_package_id: pkg.id, deployment_id: "deployment", release_hash: "release-hash", visibility: "private" } as SDKRelease;
 const candidate = { id: "candidate", sdk_release_id: release.id, deployment_id: "deployment", content_hash: "content-hash", visibility: "private" } as SDKContentCandidate;
 const origin = { id: "api", deployment_id: "deployment", visibility: "private" } as APIIntegration;
 const publication = { id: "publication", sdk_release_id: release.id, sdk_content_candidate_id: candidate.id, deployment_id: "deployment", content_hash: candidate.content_hash, visibility: "private", revision: 1 } as SDKContentPublication;
 const record = { publication, file_selections: [{ sdk_publication_file_id: "readme", decision: "included" }], sample_selections: [] } as unknown as SDKContentPublicationRecord;
 const state = { origin: structuredClone(origin), pkg: structuredClone(pkg), release: structuredClone(release), lifecycle: { sdk_release_id: release.id, selectable: true } as SDKReleaseLifecycleState, publications: [] as SDKContentPublication[], resources: { documentation: [], contracts: [], sdks: [] } as APIResourceBindings, record, publishCalls: 0, attachCalls: 0, changeCalls: 0, checkpoints: [] as string[] };
 const transport = {
  integration: async () => state.origin, package: async () => state.pkg, release: async () => state.release, lifecycle: async () => state.lifecycle,
  publications: async () => state.publications, publication: async () => state.record,
  publish: async () => { state.publishCalls++; state.publications = [publication]; return publication; },
  resources: async () => state.resources,
  attach: async (apiID: string, input: Partial<APISDKBinding>) => { state.attachCalls++; assert.deepEqual(state.checkpoints, [publication.id]); state.resources.sdks = [{ ...input, id: "binding", api_id: apiID, deployment_id: "deployment", revision: 1 } as APISDKBinding]; },
  change: async (apiID: string, bindingID: string, input: Partial<APISDKBinding>) => { state.changeCalls++; state.resources.sdks = [{ ...input, id: bindingID, api_id: apiID, deployment_id: "deployment", revision: (input.revision ?? 0) + 1 } as APISDKBinding]; },
 };
 const input = { package: pkg, release, candidate, api: origin, files: [{ id: "readme", decision: "included" as const }], samples: [], expectedBinding: undefined as APISDKBinding | undefined };
 const checkpoint = (value: SDKContentPublication) => { if (!state.checkpoints.includes(value.id)) state.checkpoints.push(value.id); };
 return { input, transport, state, publication, checkpoint };
}

test("SDK setup preserves exact route identities and explicit new-input state", () => {
 const value = parseSDKSetupSelection("?package=a%2Fb&release=exact&api=origin&candidate=candidate&publication=pub&step=input");
 assert.deepEqual(parseSDKSetupSelection(sdkSetupPath(value).split("?")[1]), value);
 assert.equal(parseSDKSetupSelection("?release=missing").release, "missing");
});
test("SDK input recovery uses raw file hashes and interpretation metadata without storing content", async () => {
 const input = [{ source_path: "README.md", content: "# Guide\nUse 1.2.3.", language: "markdown", media_type: "text/markdown", role: "readme" as const }];
 const pending = await fingerprintSDKInput(input);
 assert.equal(JSON.stringify(pending).includes("Use 1.2.3"), false);
 const candidate = { ...fixture().input.candidate, id: "exact", source_manifest: pending.files };
 assert.equal(recoverSDKInput([candidate], pending)?.id, "exact");
 assert.equal(recoverSDKInput([{ ...candidate, source_manifest: [{ ...pending.files[0], role: "example" }] }], pending), undefined);
 assert.equal(recoverSDKInput([{ ...candidate, source_manifest: [{ ...pending.files[0], raw_hash: "different" }] }], pending), undefined);
 assert.throws(() => recoverSDKInput([candidate, { ...candidate, id: "other-processor" }], pending), SDKSetupChangedError);
});
test("SDK setup publishes once, checkpoints, then attaches the exact reviewed content", async () => {
 const f = fixture(); await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.deepEqual([f.state.publishCalls, f.state.attachCalls], [1, 1]);
 assert.equal(f.state.resources.sdks[0].sdk_content_publication_id, f.publication.id);
 assert.equal(f.state.resources.sdks[0].assurance, "related");
});
for (const phase of ["publish", "attach"] as const) test(`SDK setup recovers a lost ${phase} response without repeating committed work`, async () => {
 const f = fixture(); const broken = { ...f.transport };
 if (phase === "publish") broken.publish = async () => { await f.transport.publish(); throw Error("lost response"); };
 else broken.attach = async (...args) => { await f.transport.attach(...args); throw Error("lost response"); };
 await assert.rejects(finishSDKSetup(f.input, broken, f.checkpoint), /lost response/);
 await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.deepEqual([f.state.publishCalls, f.state.attachCalls], [1, 1]);
});
test("SDK attachment outage retains publication and permits retry", async () => {
 const f = fixture(); await assert.rejects(finishSDKSetup(f.input, { ...f.transport, attach: async () => { throw Error("outage"); } }, f.checkpoint), /outage/);
 assert.deepEqual(f.state.checkpoints, [f.publication.id]);
 await finishSDKSetup(f.input, f.transport, f.checkpoint); assert.equal(f.state.publishCalls, 1);
});
test("SDK retry refuses a committed publication with different inclusion decisions", async () => {
 const f = fixture(); f.state.publications = [f.publication]; f.state.record.file_selections[0].decision = "excluded";
 assert.equal(sdkReviewMatchesPublication(f.state.record, f.input.files, []), false);
 await assert.rejects(finishSDKSetup(f.input, f.transport, f.checkpoint), SDKSetupChangedError);
 assert.deepEqual([f.state.publishCalls, f.state.attachCalls], [0, 0]);
});
test("SDK retry preserves compatibility/test evidence when already attached exactly", async () => {
 const f = fixture(); f.state.publications = [f.publication]; const binding = { id: "binding", deployment_id: "deployment", api_id: "api", sdk_package_id: f.input.package.id, sdk_release_id: f.input.release.id, sdk_content_publication_id: f.publication.id, visibility: "private", state: "ready", assurance: "tested", compatibility_assertion_id: "scenario-result", revision: 3 } as APISDKBinding;
 f.state.resources.sdks = [binding]; f.input.expectedBinding = binding; await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.equal(f.state.resources.sdks[0], binding); assert.equal(f.state.changeCalls, 0);
});
test("SDK setup refuses a concurrent attachment replacement", async () => {
 const f = fixture(); f.state.resources.sdks = [{ id: "other-binding", sdk_package_id: f.input.package.id, sdk_release_id: "other-release", state: "draft", revision: 2 } as APISDKBinding];
 await assert.rejects(finishSDKSetup(f.input, f.transport, f.checkpoint), SDKSetupChangedError); assert.equal(f.state.changeCalls, 0);
});
for (const reason of ["archived", "yanked", "scope", "release", "public"] as const) test(`SDK setup rejects changed ${reason} before publication`, async () => {
 const f = fixture();
 if (reason === "archived") f.state.pkg.lifecycle = "archived";
 if (reason === "yanked") f.state.lifecycle.selectable = false;
 if (reason === "scope") f.state.release.deployment_id = "other";
 if (reason === "release") f.state.release.release_hash = "other";
 if (reason === "public") f.input.api.visibility = "public";
 await assert.rejects(finishSDKSetup(f.input, f.transport, f.checkpoint), SDKSetupChangedError); assert.equal(f.state.publishCalls, 0);
});


test("SDK retry compares all structured review evidence and exact record scope", async () => {
 const f = fixture(); f.state.publications = [f.publication];
 f.state.record.sample_selections = [{ sdk_code_sample_id: "sample", decision: "approved", review_evidence: { summary: "Reviewed", method: "manual", reference: "check-a" } }];
 const same = [{ id: "sample", decision: "approved" as const, review_evidence: { reference: "check-a", method: "manual", summary: "Reviewed" } }];
 assert.equal(sdkReviewMatchesPublication(f.state.record, f.input.files, same), true);
 assert.equal(sdkReviewMatchesPublication(f.state.record, f.input.files, [{ ...same[0], review_evidence: { ...same[0].review_evidence, reference: "check-b" } }]), false);
 f.state.record.sample_selections = [];
 f.state.record.publication = { ...f.publication, sdk_release_id: "another-release" };
 await assert.rejects(finishSDKSetup(f.input, f.transport, f.checkpoint), SDKSetupChangedError);
 assert.equal(f.state.attachCalls, 0);
});
test("SDK setup makes an unchanged draft ready while preserving its exact compatibility assertion", async () => {
 const f = fixture(); f.state.publications = [f.publication];
 const binding = { id: "binding", sdk_package_id: f.input.package.id, sdk_release_id: f.input.release.id, sdk_content_publication_id: f.publication.id, deployment_id: "deployment", api_id: "api", applicable_capabilities: [], applicable_operation_keys: [], selector_hash: "selector-hash", visibility: "private", state: "draft", coverage: "full", assurance: "tested", compatibility_assertion_id: "scenario-result", applicable_modules: ["auth"], selector: { language: "typescript" }, revision: 3 } as APISDKBinding;
 f.state.resources.sdks = [binding]; f.input.expectedBinding = binding;
 await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.equal(f.state.resources.sdks[0].state, "ready");
 assert.equal(f.state.resources.sdks[0].compatibility_assertion_id, "scenario-result");
 assert.equal(f.state.resources.sdks[0].assurance, "tested");
 assert.deepEqual(f.state.resources.sdks[0].applicable_modules, ["auth"]);
});
test("SDK setup attaches explicitly public guidance to a public API", async () => {
 const f = fixture();
 for (const value of [f.input.package, f.input.release, f.input.candidate, f.input.api, f.state.origin, f.state.pkg, f.state.release, f.publication]) value.visibility = "public";
 await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.equal(f.state.resources.sdks[0].visibility, "public");
 assert.equal(f.state.resources.sdks[0].assurance, "related");
});

test("SDK completion checks attachment scope, selectors and evidence claims", async () => {
 const f = fixture(); await finishSDKSetup(f.input, f.transport, f.checkpoint);
 const expected = sdkSetupBinding(f.input.package, f.input.release, f.publication, f.input.api);
 const saved = f.state.resources.sdks[0];
 assert.equal(sdkSetupBindingMatches(saved, f.input.api, expected), true);
 for (const patch of [{ deployment_id: "foreign" }, { api_id: "foreign" }, { sdk_release_id: "other" }, { sdk_content_publication_id: "other" }, { visibility: "public" }, { state: "draft" }, { assurance: "tested" }, { compatibility_assertion_id: "unexpected-claim" }, { api_contract_revision_id: "unexpected-contract" }, { applicable_modules: ["unreviewed"] }, { selector: { source_paths: ["other.md"] } }] as Partial<APISDKBinding>[]) {
  assert.equal(sdkSetupBindingMatches({ ...saved, ...patch }, f.input.api, expected), false, JSON.stringify(patch));
 }
});

test("SDK setup checks the saved attachment instead of accepting an unrelated successful response", async () => {
 const f = fixture();
 await assert.rejects(finishSDKSetup(f.input, { ...f.transport, attach: async (...args) => { await f.transport.attach(...args); f.state.resources.sdks[0].selector = { source_paths: ["unreviewed.md"] }; } }, f.checkpoint), SDKSetupChangedError);
 assert.equal(f.state.publishCalls, 1); assert.deepEqual(f.state.checkpoints, [f.publication.id]);
});

test("SDK setup rechecks API audience before publication and attachment", async () => {
 for (const stage of ["before", "during"] as const) {
  const f = fixture();
  if (stage === "before") f.state.origin.visibility = "public";
  await assert.rejects(finishSDKSetup(f.input, { ...f.transport, publish: async () => { const publication = await f.transport.publish(); f.state.origin.visibility = "public"; return publication; } }, f.checkpoint), SDKSetupChangedError);
  assert.equal(f.state.attachCalls, 0); assert.equal(f.state.publishCalls, stage === "before" ? 0 : 1);
 }
});

test("changing SDK content resets old compatibility claims and recovers a lost change response", async () => {
 const f = fixture();
 const previous = { id: "binding", api_id: "api", deployment_id: "deployment", sdk_package_id: f.input.package.id, sdk_release_id: f.input.release.id, sdk_content_publication_id: "older", visibility: "private", state: "ready", coverage: "full", assurance: "tested", compatibility_assertion_id: "old-scenario", api_contract_revision_id: "old-contract", applicable_modules: ["auth"], applicable_capabilities: [], applicable_operation_keys: [], selector_hash: "old-selector", selector: { language: "typescript" }, revision: 1 } as APISDKBinding;
 f.input.expectedBinding = previous; f.state.resources.sdks = [previous];
 await assert.rejects(finishSDKSetup(f.input, { ...f.transport, change: async (...args) => { await f.transport.change(...args); throw Error("lost response"); } }, f.checkpoint), /lost response/);
 await finishSDKSetup(f.input, f.transport, f.checkpoint);
 assert.equal(f.state.changeCalls, 1); assert.equal(f.state.publishCalls, 1);
 const saved = f.state.resources.sdks[0]; assert.equal(saved.assurance, "related"); assert.equal(saved.compatibility_assertion_id, ""); assert.equal(saved.api_contract_revision_id, ""); assert.deepEqual(saved.selector, {});
});
