import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { CatalogNavigation, DocumentationNavigation } from "../app/components/console/developer-assets/developer-asset-navigation";
import { ConsoleSidebar } from "../app/components/console/workspace-navigation";
import type { APIIntegration } from "../app/lib/api";
import { INTEGRATION_TABS, SECTION_PATHS, integrationPath, parseConsolePath } from "../app/lib/console-routes";

const noop = () => {};
const integration = {
  id: "api-payments-v1",
  display_name: "Payments API",
  family_key: "payments",
  version_key: "v1",
  description: "Payments",
  visibility: "private",
  lifecycle: "active",
  revision: 3,
} as APIIntegration;

test("root Catalog routes expose APIs, Documentation, API contracts, SDKs, and Query Lab", () => {
  assert.equal(SECTION_PATHS.product, "/integrations");
  assert.equal(SECTION_PATHS.sources, "/integrations/documentation", "the existing source URL remains compatible");
  assert.equal(SECTION_PATHS.documents, "/developer-assets/documentation/documents");
  assert.equal(SECTION_PATHS.collections, "/developer-assets/documentation/collections");
  assert.equal(SECTION_PATHS.contracts, "/developer-assets/api-contracts");
  assert.equal(SECTION_PATHS.sdks, "/developer-assets/sdk-packages");
  assert.equal(SECTION_PATHS["query-lab"], "/developer-assets/query-lab");
  for (const section of ["product", "sources", "documents", "collections", "contracts", "sdks", "query-lab"] as const) {
    assert.equal(parseConsolePath(SECTION_PATHS[section]).section, section);
  }

  const catalog = renderToStaticMarkup(createElement(CatalogNavigation, { active: "contracts", onNavigate: noop }));
  for (const label of ["APIs", "Documentation", "SDKs", "Query Lab"]) assert.match(catalog, new RegExp(`>${label}</a>`));
  const documentation = renderToStaticMarkup(createElement(DocumentationNavigation, { active: "contracts", onNavigate: noop }));
  for (const label of ["Sources", "All files", "Collections", "API contracts"]) assert.match(documentation, new RegExp(`>${label}</a>`));

  const sidebar = renderToStaticMarkup(createElement(ConsoleSidebar, { section: "contracts", activeNavigationID: "catalog", onNavigate: noop }));
  assert.match(sidebar, />Catalog</);
  assert.match(sidebar, /aria-label="Catalog sections"/);
  for (const label of ["APIs", "Sources", "All files", "Collections", "API contracts", "SDKs", "Query Lab"]) assert.match(sidebar, new RegExp(`>${label}</a>`));
});

test("the compatible API documentation route is visibly Resources and attachment-only", async () => {
  assert.deepEqual(INTEGRATION_TABS.find((tab) => tab.id === "documentation"), { id: "documentation", label: "Resources" });
  assert.equal(integrationPath(integration.id, "documentation"), "/integration/api-payments-v1/documentation");
  const source = await readFile(new URL("../app/components/console/developer-assets/api-resources-workspace.tsx", import.meta.url), "utf8");
  assert.match(source, /attachment records only/i);
  assert.match(source, /Attach existing/);
  assert.match(source, /panelKind === "contract" \? "Create in Catalog" : "Create & attach"/, "contract creation must not claim attachment");
  assert.match(source, /kind === "contract" \? "Create in Catalog" : "Create & attach exact resource"/, "the contract dialog action creates only the Catalog root");
  assert.match(source, /This creates only the reusable contract root\. It does not ingest, approve, publish, or attach a contract to this API\./);
  assert.match(source, /Attach an OpenAPI source, ingest and validate the normalized candidate, review and publish an immutable revision, then return to this API’s Resources tab and use Attach existing\./);
  assert.match(source, /API contract could not be created in Catalog\./);
  assert.doesNotMatch(source, /Create and review in Catalog/);
  assert.match(source, /Open catalog/);
  assert.match(source, /Change exact/);
  assert.match(source, /no attachment upgrades automatically/i);
  assert.doesNotMatch(source, /Crawl all|Review &amp; attach|Documentation ingestion/);
});

test("Query Lab renders global, API, and combined scope controls, exact filters, scores, citations, and trace ID", async () => {
  const source = await readFile(new URL("../app/components/console/developer-assets/query-lab-view.tsx", import.meta.url), "utf8");
  for (const scope of ["Global", "API", "Combined"]) assert.match(source, new RegExp(`label: "${scope}"`));
  for (const filter of ["Asset kinds", "Languages", "Ecosystems", "Exact versions", "Exact SDK release IDs", "Context token limit"]) assert.match(source, new RegExp(filter));
  for (const scoreKey of ["item.lexical_score", "item.semantic_score", "item.rerank_score"]) assert.match(source, new RegExp(scoreKey.replace(".", "\\.")));
  assert.match(source, /Trace ID/);
  assert.match(source, /Citation and exact identity/);
  assert.match(source, /item\.unit\.citation/);
  assert.match(source, /source_publication_id/);
  assert.match(source, /content_hash/);
  assert.doesNotMatch(source, /mixed/i, "unsupported mixed units must not be offered as an asset-kind filter");
});

test("SDK review and publication history preserve exact validated identities", async () => {
  const sdk = await readFile(new URL("../app/components/console/developer-assets/sdk-catalog-view.tsx", import.meta.url), "utf8");
  const history = await readFile(new URL("../app/components/console/developer-assets/api-resource-publication-history.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/developer-assets-api.ts", import.meta.url), "utf8");

  assert.match(sdk, /validation_status/);
  assert.match(sdk, /positiveStatuses/);
  assert.match(sdk, /validation_evidence/);
  assert.match(sdk, /Explicit review evidence/);
  assert.match(sdk, /review_evidence: \{ summary:/);
  assert.doesNotMatch(sdk, /validation_status !== "unvalidated"/, "status labels alone must not make samples approvable");
  assert.match(client, /review_evidence\?: \{ summary: string/);
  assert.match(sdk, /Leave blank for the server canonical command/);
  assert.match(sdk, /installCommand\.trim\(\) \? \{ install_command:/);
  assert.match(sdk, /No code execution/);
  assert.match(sdk, /Used by APIs/);
  assert.match(sdk, /affected API attachment/);
  assert.match(history, /snapshot_hash/);
  assert.match(history, /publication\.id/);
  assert.match(client, /apiResourcePublications:/);
  assert.match(client, /apiResourcePublication:/);
  assert.match(client, /documentationPublications:/);
  assert.match(client, /publishDocumentation:/);
});

test("documentation explorer pages all files and renders exact maps plus immutable review history", async () => {
  const explorer = await readFile(new URL("../app/components/console/developer-assets/documentation-explorer-view.tsx", import.meta.url), "utf8");
  const client = await readFile(new URL("../app/lib/developer-assets-api.ts", import.meta.url), "utf8");
  const openapi = await readFile(new URL("../api/openapi.yaml", import.meta.url), "utf8");

  assert.match(explorer, /page\.total/);
  assert.match(explorer, /page\.has_more/);
  assert.match(explorer, /Load more/);
  assert.match(explorer, /selected\.documentation_map/);
  assert.match(explorer, /Documentation Map agent markdown/);
  assert.match(explorer, /Derived document outline/);
  assert.match(client, /offset\?: number/);
  assert.match(client, /has_more: boolean/);
  assert.match(explorer, /Latest decision/);
  assert.match(explorer, /Latest persisted decision/);
  assert.match(explorer, /Unreviewed/);
  assert.match(explorer, /Retained reason:/);
  assert.match(explorer, /history_newest_first/);
  assert.match(explorer, /source_publication_selections\[0\]/);
  assert.match(explorer, /source_publication_id: latestPublication\.id/);
  assert.match(explorer, /record\.documentation_map\?\.content_hash === selectedMapHash/, "AI advisory availability must retain the exact persisted map/publication check");
  assert.match(client, /export type SourcePublicationDocumentSelection/);
  assert.match(client, /decision: "included" \| "excluded" \| "quarantined"/);
  assert.match(client, /source_publication_selections: SourcePublicationDocumentSelection\[\]/);
  assert.match(openapi, /SourcePublicationDocumentSelection:/);
  assert.match(openapi, /source_publication_selections:/);
  assert.match(openapi, /Complete persisted history for this exact deployment\/document pair, ordered by reviewed_at and created_at newest first/);
});
