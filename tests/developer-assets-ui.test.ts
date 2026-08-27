import assert from "node:assert/strict";
import test from "node:test";

import { createInstance } from "i18next";
import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { I18nextProvider, initReactI18next } from "react-i18next";

import { DocumentationNavigation } from "../app/components/console/developer-assets/developer-asset-navigation";
import { decisionPayload, decisionsComplete, sampleValidated, sdkBufferLooksText, sdkNormalizedLocalPath } from "../app/components/console/developer-assets/sdk-catalog-helpers";
import { ConsoleSidebar } from "../app/components/console/workspace-navigation";
import { INTEGRATION_TABS, SECTION_PATHS, integrationPath, parseConsolePath } from "../app/lib/console-routes";
import type { DeveloperAssetRecord } from "../app/lib/developer-assets-api";
import { i18nOptions } from "../app/i18n/options";

const noop = () => {};
const testI18n = createInstance();
await testI18n.use(initReactI18next).init(i18nOptions("en"));

function render(element: ReactElement) {
  return renderToStaticMarkup(createElement(I18nextProvider, { i18n: testI18n }, element));
}

test("promotes APIs, Docs, and SDKs and packages to the primary navigation", () => {
  assert.equal(SECTION_PATHS.product, "/integrations");
  assert.equal(SECTION_PATHS.sources, "/integrations/documentation");
  assert.equal(SECTION_PATHS.documents, "/developer-assets/documentation/documents");
  assert.equal(SECTION_PATHS.contracts, "/developer-assets/api-contracts");
  assert.equal(SECTION_PATHS.sdks, "/developer-assets/sdk-packages");
  assert.equal(SECTION_PATHS["query-lab"], "/developer-assets/query-lab");
  assert.deepEqual(INTEGRATION_TABS.find((tab) => tab.id === "documentation"), { id: "documentation", label: "routes.resources" });
  assert.equal(integrationPath("api-payments-v1", "documentation"), "/integration/api-payments-v1/documentation");
  for (const section of ["product", "sources", "documents", "contracts", "sdks", "query-lab"] as const) {
    assert.equal(parseConsolePath(SECTION_PATHS[section]).section, section);
  }
  assert.equal(parseConsolePath("/developer-assets/documentation/collections").kind, "not-found");

  const documentation = render(createElement(DocumentationNavigation, { active: "contracts", onNavigate: noop }));
  for (const label of ["Sources", "Documents", "API contracts", "Query Lab"]) assert.match(documentation, new RegExp(`>${label}</a>`));
  assert.doesNotMatch(documentation, />Collections<|>All files</);
  assert.match(documentation, /href="\/developer-assets\/query-lab" class="page-tab docs-query-lab-tab">Query Lab<\/a>/);

  const sidebar = render(createElement(ConsoleSidebar, { section: "contracts", activeNavigationID: "docs", onNavigate: noop }));
  for (const [label, path] of [["APIs", "/integrations"], ["Docs", "/integrations/documentation"], ["SDKs and packages", "/developer-assets/sdk-packages"]]) {
    assert.match(sidebar, new RegExp(`href="${path}"[^>]*>[\\s\\S]*?<span>${label}</span>`));
  }
  assert.doesNotMatch(sidebar, /Catalog sections|Docs sections|nav-subsections/);
});

test("SDK review helpers require evidence and reject unsafe local files", () => {
  const unvalidated = { id: "sample", validation_status: "unvalidated", validation_evidence: {} } as DeveloperAssetRecord;
  const labelOnly = { id: "sample", validation_status: "compiled", validation_evidence: {} } as DeveloperAssetRecord;
  const validated = { id: "sample", validation_status: "compiled", validation_evidence: { passed: true, validator: "tsc" } } as DeveloperAssetRecord;
  assert.equal(sampleValidated(unvalidated), false);
  assert.equal(sampleValidated(labelOnly), false);
  assert.equal(sampleValidated(validated), true);

  const manual = { sample: { decision: "approved" as const, reason: "", reviewEvidence: "Reviewed imports and exact SDK version." } };
  assert.equal(decisionsComplete([unvalidated], manual, "sample"), true);
  assert.deepEqual(decisionPayload([unvalidated], manual, "sample")[0]?.review_evidence, { summary: "Reviewed imports and exact SDK version." });
  assert.equal(decisionsComplete([unvalidated], { sample: { ...manual.sample, reviewEvidence: "" } }, "sample"), false);
  assert.equal(decisionsComplete([validated], { sample: { decision: "approved", reason: "", reviewEvidence: "" } }, "sample"), true);

  assert.equal(sdkNormalizedLocalPath("guides/getting-started.md"), "guides/getting-started.md");
  for (const unsafe of ["../secret", "/etc/passwd", "C:\\secret.txt", "guides//empty.md"]) assert.equal(sdkNormalizedLocalPath(unsafe), "");
  assert.equal(sdkBufferLooksText(new TextEncoder().encode("# SDK\nUse it.").buffer), true);
  assert.equal(sdkBufferLooksText(new Uint8Array([0, 1, 2]).buffer), false);
});
