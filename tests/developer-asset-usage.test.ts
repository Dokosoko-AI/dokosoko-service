import assert from "node:assert/strict";
import test from "node:test";

import { contractUsages, documentationUsages, sdkUsages } from "../app/components/console/developer-assets/developer-asset-usage";
import type { APIIntegration } from "../app/lib/api";
import type { DeveloperAssetUsage } from "../app/lib/developer-assets-api";

const integrations = [
  { id: "api-a", display_name: "API A" },
  { id: "api-b", display_name: "API B" },
] as APIIntegration[];

const usage = {
  documentation: [
    { id: "doc-a", api_id: "api-a", documentation_collection_id: "docs", lifecycle: "attached" },
    { id: "doc-b", api_id: "api-b", documentation_collection_id: "docs", lifecycle: "detached" },
  ],
  contracts: [
    { id: "contract-a", api_id: "api-a", api_contract_id: "contract", lifecycle: "attached" },
    { id: "contract-unknown", api_id: "removed-api", api_contract_id: "contract", lifecycle: "attached" },
  ],
  sdks: [
    { id: "sdk-a", api_id: "api-a", sdk_package_id: "package", sdk_content_publication_id: "content-a", state: "ready" },
    { id: "sdk-b", api_id: "api-b", sdk_package_id: "package", sdk_content_publication_id: "content-b", state: "detached" },
  ],
  publications: [
    { id: "publication-old", api_id: "api-a", published_at: "2026-01-01T00:00:00Z", sdks: [{ binding_id: "sdk-a", sdk_content_publication_id: "content-a" }] },
    { id: "publication-new", api_id: "api-a", published_at: "2026-02-01T00:00:00Z", sdks: [{ binding_id: "sdk-a", sdk_content_publication_id: "content-a" }] },
  ],
} as DeveloperAssetUsage;

test("deployment usage resolves attached assets without one request per API", () => {
  assert.deepEqual(documentationUsages(usage, integrations, "docs").map(({ binding }) => binding.id), ["doc-a"]);
  assert.deepEqual(contractUsages(usage, integrations, "contract").map(({ binding }) => binding.id), ["contract-a"]);

  const sdk = sdkUsages(usage, integrations, "package");
  assert.equal(sdk.length, 1);
  assert.equal(sdk[0]?.binding.id, "sdk-a");
  assert.equal(sdk[0]?.publication?.id, "publication-new", "the newest immutable API publication is selected");
});
