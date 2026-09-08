import assert from "node:assert/strict";
import test from "node:test";
import { createInstance } from "i18next";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { I18nextProvider, initReactI18next } from "react-i18next";
import { register } from "node:module";
import { i18nOptions } from "../app/i18n/options";
import type { SDKContentCandidateRecord } from "../app/lib/developer-assets-api";
register("./helpers/vinext-loader.mjs", import.meta.url);
const { SDKGuidanceReview } = await import("../app/components/console/developer-assets/sdk-guidance-review");
const i18n = createInstance();
await i18n.use(initReactI18next).init(i18nOptions("en"));

test("pending SDK saves keep draft decisions disabled and show exact Markdown sample code", () => {
  const record = { candidate: { id: "candidate", deployment_id: "deployment", sdk_release_id: "release" }, files: [], samples: [{ id: "sample", title: "Markdown example", language: "markdown", code: "# Exact sample\n<script>alert('untrusted')</script>", validation_status: "not_checked" }] } as unknown as SDKContentCandidateRecord;
  const render = (readOnly: boolean) => renderToStaticMarkup(createElement(I18nextProvider, { i18n }, createElement(SDKGuidanceReview, { record, files: {}, samples: { sample: { decision: "approved", reason: "", reviewEvidence: "Manual review only" } }, disabled: true, readOnly, onChange: () => undefined })));
  const pending = render(false);
  assert.match(pending, /<pre[^>]*><code># Exact sample/);
  assert.match(pending, /&lt;script&gt;/); assert.doesNotMatch(pending, /<script/);
  assert.match(pending, /<select[^>]*disabled/); assert.match(pending, /<textarea[^>]*disabled/);
  assert.doesNotMatch(pending, /Published decision/);
  const published = render(true); assert.match(published, /Published decision/); assert.doesNotMatch(published, /<select/);
});

test("SDK file comparisons do not select another release or an ambiguous source path", () => {
  const record = { candidate: { id: "candidate", deployment_id: "deployment", sdk_release_id: "release" }, files: [{ id: "file", source_path: "README.md", normalized_content: "Current guidance", language: "markdown" }], samples: [] } as unknown as SDKContentCandidateRecord;
  const prior = { ...record, candidate: { ...record.candidate, id: "prior" }, files: [{ ...record.files[0], id: "old", normalized_content: "Previous guidance" }] };
  const render = (previous: SDKContentCandidateRecord) => renderToStaticMarkup(createElement(I18nextProvider, { i18n }, createElement(SDKGuidanceReview, { record, previous, files: {}, samples: {}, readOnly: false, onChange: () => undefined })));
  assert.match(render(prior), /Compare with previous publication/);
  assert.doesNotMatch(render({ ...prior, files: [...prior.files, { ...prior.files[0], id: "ambiguous" }] }), /Compare with previous publication/);
  const foreign = { ...prior, candidate: { ...prior.candidate, sdk_release_id: "other" }, files: [{ ...prior.files[0], source_path: "foreign.md", normalized_content: "Foreign release private body" }] };
  assert.doesNotMatch(render(foreign), /Compare with previous publication|Foreign release private body/);
});

test("same-title SDK samples expose distinct source lines without inventing locations for curated samples", () => {
  const sample = { title: "Quickstart", language: "javascript", source_path: "README.md", origin: "extracted", code: "console.log('example');", validation_status: "not_checked" };
  const record = { candidate: { id: "candidate", deployment_id: "deployment", sdk_release_id: "release" }, files: [], samples: [
    { ...sample, id: "first", source_start: 0, source_end: 1 },
    { ...sample, id: "second", source_start: 6, source_end: 8 },
    { ...sample, id: "curated", origin: "curated", source_start: 99, source_end: 101 },
  ] } as unknown as SDKContentCandidateRecord;
  const markup = renderToStaticMarkup(createElement(I18nextProvider, { i18n }, createElement(SDKGuidanceReview, { record, files: {}, samples: {}, readOnly: false, onChange: () => undefined })));
  assert.match(markup, /javascript · README.md · Lines 1–1/);
  assert.match(markup, /javascript · README.md · Lines 7–8/);
  assert.doesNotMatch(markup, /Lines 100–101/);
});
