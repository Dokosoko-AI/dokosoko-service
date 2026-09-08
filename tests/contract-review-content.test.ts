import assert from "node:assert/strict";
import test from "node:test";
import { createInstance } from "i18next";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { I18nextProvider, initReactI18next } from "react-i18next";
import { i18nOptions } from "../app/i18n/options";
import { register } from "node:module";
register("./helpers/vinext-loader.mjs", import.meta.url);
const { ContractReviewContent } = await import("../app/components/console/developer-assets/contract-review-content");
import type { APIContractCandidateRecord } from "../app/lib/developer-assets-api";

const i18n = createInstance();
await i18n.use(initReactI18next).init(i18nOptions("en"));
test("contract review exposes inherited and operation parameters, structural changes, and inert source text", () => {
  const normalized = {
    info: { title: "Payments", description: "<script>alert('unsafe')</script>" },
    servers: [{ url: "https://api.example.test" }],
    paths: {
      "/payments/{id}": {
        parameters: [{ name: "id", in: "path" }],
        get: { parameters: [{ name: "expand", in: "query" }], responses: { "200": { description: "Payment" } } },
      },
    },
  };
  const current = { candidate: { normalized_contract: normalized, validation_result: { valid: true } }, operations: [{ method: "get", path_template: "/payments/{id}", content_hash: "same" }], schemas: [], examples: [] } as unknown as APIContractCandidateRecord;
  const previous = { ...current, candidate: { ...current.candidate, normalized_contract: { ...normalized, servers: [{ url: "https://old.example.test" }], paths: { "/payments/{id}": { ...normalized.paths["/payments/{id}"], parameters: [{ name: "old_id", in: "path" }] } } } }, operations: [...current.operations, { method: "delete", path_template: "/payments/{id}", content_hash: "removed" }] };
  const html = renderToStaticMarkup(createElement(I18nextProvider, { i18n }, createElement(ContractReviewContent, { record: current, previous, previousRevision: 2 })));
  for (const expected of ["GET /payments/{id}", "DELETE /payments/{id}", "Changed", "Removed", "pathParameters", "operationParameters", "expand", "Changes to schemas, servers, security, and contract metadata", "Previous definition"]) assert.ok(html.includes(expected), expected);
  assert.doesNotMatch(html, /<script|onclick=/i);
});
