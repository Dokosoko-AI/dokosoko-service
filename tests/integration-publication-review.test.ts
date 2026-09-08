import assert from "node:assert/strict";
import test from "node:test";
import type { APIIntegrationPublishStatus } from "../app/lib/api";
import type { developerAssetsApi } from "../app/lib/developer-assets-api";
import { integrationPublicationRows, resolvePublicationReviewRows, reviewedPublicationInput } from "../app/lib/integration-publication-review";

const status = { integration_id: "payments", candidate_revision: 4, current_manifest_hash: "reviewed-hash", ready: true, has_changes: true } as APIIntegrationPublishStatus;

test("publication action captures the reviewed API, revision and hash without substituting refreshed state", () => {
  const current = { ...status };
  const reviewed = reviewedPublicationInput(current, "payments");
  current.candidate_revision = 5; current.current_manifest_hash = "unreviewed-hash";
  assert.deepEqual(reviewed, { integrationID: "payments", candidateRevision: 4, candidateManifestHash: "reviewed-hash" });
  for (const value of [{ ...status, integration_id: "other" }, { ...status, ready: false }, { ...status, has_changes: false }, { ...status, candidate_revision: NaN }, { ...status, candidate_revision: 1.5 }, { ...status, candidate_revision: 0 }, { ...status, current_manifest_hash: " " }]) assert.throws(() => reviewedPublicationInput(value, "payments"), /publication_review_unavailable/);
});

test("publication review identifies changed and removed bindings, including selector-only changes and legacy contracts", () => {
  const document = { binding_id: "doc", documentation_collection_name: "Setup", selector: { sections: ["intro"] } };
  const sdk = { binding_id: "sdk", sdk_package_display_name: "Payments JS", sdk_release_id: "release" };
  const before = { developer_assets: { documentation: [document], sdks: [sdk] }, resource_sets: [{ set_id: "old", kind: "api", name: "Legacy contract", revision: 1 }] };
  const after = { developer_assets: { documentation: [{ ...document, selector: { sections: ["webhooks"] } }] }, tools: [{ tool_id: "tool", namespace: "payments", name: "inspect" }] };
  const rows = integrationPublicationRows(after, before);
  assert.deepEqual(rows.map((row) => [row.title, row.kind, row.state]), [["Setup", "documentation", "changed"], ["payments/inspect", "tools", "added"], ["Payments JS", "sdks", "removed"], ["Legacy contract", "contracts", "removed"]]);
  assert.deepEqual(rows[0].previous, document);
  assert.equal(integrationPublicationRows({ developer_assets: { documentation: [{ selector: document.selector, documentation_collection_name: "Setup", binding_id: "doc" }] } }, { developer_assets: { documentation: [document] } })[0].state, "unchanged");
});

test("publication review resolves exact immutable versions and preserves snapshot names without catalog head reads", async () => {
  const deployment = "deployment";
  const calls: string[][] = [];
  const transport = {
    documentationCollectionRevision: async (...args: string[]) => { calls.push(args); return { revision: { id: args[1], deployment_id: deployment, documentation_collection_id: args[0], revision: 2 } }; },
    apiContractRevision: async (...args: string[]) => { calls.push(args); return { id: args[1], deployment_id: deployment, api_contract_id: args[0], revision: 3 }; },
    sdkRelease: async (...args: string[]) => { calls.push(args); return { id: args[1], deployment_id: deployment, sdk_package_id: args[0], exact_version: args[1] === "old-release" ? "1.0.0" : "2.0.0" }; },
    sdkContentPublication: async (...args: string[]) => { calls.push(args); return { publication: { id: args[1], deployment_id: deployment, sdk_release_id: args[0], revision: args[1] === "old-guidance" ? 1 : 4 } }; },
    documentationPublication: async (id: string) => ({ id, deployment_id: deployment, snapshot_hash: "global-hash", revision: 5 }),
  } as unknown as Pick<typeof developerAssetsApi, "documentationCollectionRevision" | "apiContractRevision" | "sdkRelease" | "sdkContentPublication" | "documentationPublication">;
  const sdk = { binding_id: "sdk", sdk_package_id: "pkg", sdk_package_display_name: "Frozen SDK name", sdk_release_id: "release", sdk_content_publication_id: "guidance" };
  const rows = integrationPublicationRows({ developer_assets: {
    documentation: [{ binding_id: "doc", documentation_collection_id: "set", documentation_collection_revision_id: "doc-r2" }],
    contracts: [{ binding_id: "contract", api_contract_id: "contract", api_contract_revision_id: "contract-r3" }], sdks: [sdk],
    global_documentation_publication_id: "global-r5", global_documentation_snapshot_hash: "global-hash",
  } }, { developer_assets: { sdks: [{ ...sdk, sdk_release_id: "old-release", sdk_content_publication_id: "old-guidance" }] } });
  const resolved = await resolvePublicationReviewRows(rows, deployment, transport);
  assert.deepEqual(resolved.map((row) => row.selection), [{ revision: 2 }, { revision: 3 }, { version: "2.0.0", guidanceRevision: 4 }, { revision: 5 }]);
  assert.equal(resolved[2].title, "Frozen SDK name");
  assert.deepEqual(resolved[2].previousSelection, { version: "1.0.0", guidanceRevision: 1 });
  assert(calls.some((args) => args[0] === "old-release" && args[1] === "old-guidance"));
  await assert.rejects(resolvePublicationReviewRows(rows, "other-deployment", transport), /publication_selection_unavailable/);
  await assert.rejects(resolvePublicationReviewRows(rows.filter((row) => row.kind === "sdks"), deployment, { ...transport, sdkRelease: async () => ({ id: "release", deployment_id: deployment, sdk_package_id: "other" } as Awaited<ReturnType<typeof developerAssetsApi.sdkRelease>>) }), /publication_selection_unavailable/);
  await assert.rejects(resolvePublicationReviewRows(rows, deployment, { ...transport, apiContractRevision: async () => { throw Error("unavailable exact revision"); } }), /unavailable exact revision/);
});
