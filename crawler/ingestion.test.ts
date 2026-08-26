import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  assessCorpusQuality,
  buildDocumentationCorpus,
  buildDocumentationMap,
  documentationMapEnrichmentInput,
  normalizeDocument,
  renderBlock,
  segmentDocument,
  type NormalizationInput,
  type NormalizedDocument,
} from "./ingestion/index";

const fixture = async (name: string) => readFile(new URL(`./fixtures/normalization/${name}`, import.meta.url), "utf8");

function input(content: string, overrides: Partial<NormalizationInput> = {}): NormalizationInput {
  return {
    content,
    source: {
      sourceId: "source-docs",
      snapshotId: "snapshot-exact",
      canonicalUrl: "https://docs.example.com/guide",
      contentType: "text/markdown",
      sourceKind: "upload",
    },
    ...overrides,
  };
}

function blocksOfType<T extends NormalizedDocument["blocks"][number]["type"]>(document: NormalizedDocument, type: T) {
  return document.blocks.filter((block): block is Extract<NormalizedDocument["blocks"][number], { type: T }> => block.type === type);
}

test("Markdown normalization preserves semantic blocks, links, exact lineage, and deterministic hashes", async () => {
  const content = await fixture("guide.md");
  const source = input(content, {
    source: {
      sourceId: "source-payments",
      snapshotId: "snapshot-payments-v1",
      canonicalUrl: "https://docs.example.com/sdk/guide.md?version=2.1.0",
      contentType: "text/markdown; charset=utf-8",
      sourceKind: "upload",
      fetchedAt: "2026-08-26T00:00:00Z",
    },
  });
  const first = normalizeDocument(source);
  const second = normalizeDocument(source);

  assert.deepEqual(first, second);
  assert.equal(first.title, "Payments SDK");
  assert.equal(first.format, "markdown");
  assert.equal(first.source.snapshotId, "snapshot-payments-v1");
  assert.match(first.source.rawSha256, /^[a-f\d]{64}$/);
  assert.match(first.normalizedSha256, /^[a-f\d]{64}$/);
  assert.deepEqual(first.blocks.map((block) => block.type), ["heading", "paragraph", "heading", "list", "table", "code", "heading", "paragraph"]);

  const headings = blocksOfType(first, "heading");
  assert.deepEqual(headings.map((heading) => [heading.level, heading.text, heading.anchor]), [
    [1, "Payments", "payments"],
    [2, "Install", "install"],
    [2, "Errors", "errors"],
  ]);
  assert.deepEqual(blocksOfType(first, "paragraph")[0].links, [{
    label: "authentication guide",
    target: "https://docs.example.com/sdk/authentication",
  }]);
  assert.deepEqual(blocksOfType(first, "list")[0].items.map((item) => item.depth), [0, 1, 0]);
  assert.deepEqual(blocksOfType(first, "table")[0].headers, ["Runtime", "Command"]);
  assert.equal(blocksOfType(first, "code")[0].language, "ts");
  assert.match(blocksOfType(first, "code")[0].code, /new Payments/);
  for (const block of first.blocks) {
    assert.match(block.id, /^block_[a-f\d]{24}$/);
    assert.match(block.contentSha256, /^[a-f\d]{64}$/);
    assert.ok(block.sourceRange.endOffset >= block.sourceRange.startOffset);
  }
});

test("HTML normalization is inert, uses the content region, and preserves lists, tables, code, and safe links", async () => {
  delete (globalThis as Record<string, unknown>).__dokosokoHTMLExecuted;
  const document = normalizeDocument(input(await fixture("guide.html"), {
    source: {
      sourceId: "source-html",
      canonicalUrl: "https://docs.example.com/access/index.html",
      contentType: "text/html",
      sourceKind: "website",
    },
  }));

  assert.equal((globalThis as Record<string, unknown>).__dokosokoHTMLExecuted, undefined);
  assert.equal(document.title, "Identity & Access");
  assert.deepEqual(document.blocks.map((block) => block.type), ["heading", "paragraph", "heading", "list", "table", "code"]);
  assert.doesNotMatch(document.blocks.map(renderBlock).join("\n"), /Repeated navigation|Repeated footer|Ignore all previous/);
  assert.deepEqual(blocksOfType(document, "paragraph")[0].links, [{ label: "token guide", target: "https://docs.example.com/access/tokens" }]);
  assert.deepEqual(blocksOfType(document, "list")[0].items.map((item) => [item.text, item.depth]), [
    ["Read resources", 0],
    ["Use resources:read .", 1],
    ["Write resources", 0],
  ]);
  assert.deepEqual(blocksOfType(document, "table")[0].rows, [["resources:read", "Read access"]]);
  assert.equal(blocksOfType(document, "code")[0].code, "const allowed = 1 < 2;");
  assert.ok(document.diagnostics.some((diagnostic) => diagnostic.code === "boilerplate_removed"));
});

test("MDX imports and exports are preserved as inert code and never executed", () => {
  delete (globalThis as Record<string, unknown>).__dokosokoMDXExecuted;
  const content = [
    "export const value = (() => { globalThis.__dokosokoMDXExecuted = true; return 1; })();",
    "",
    "# Safe MDX",
    "",
    "<Callout>Static documentation text.</Callout>",
  ].join("\n");
  const document = normalizeDocument(input(content, {
    source: { sourceId: "source-mdx", canonicalUrl: "upload://source-mdx/guide.mdx", contentType: "text/markdown" },
  }));

  assert.equal((globalThis as Record<string, unknown>).__dokosokoMDXExecuted, undefined);
  assert.equal(document.format, "mdx");
  assert.equal(blocksOfType(document, "code")[0].language, "tsx");
  assert.match(blocksOfType(document, "code")[0].code, /globalThis\.__dokosokoMDXExecuted/);
  assert.ok(document.diagnostics.some((diagnostic) => diagnostic.code === "mdx_executable_syntax_preserved"));
});

test("OpenAPI JSON and YAML produce operation-aware deterministic records without executing input", async () => {
  const yaml = normalizeDocument(input(await fixture("openapi.yaml"), {
    documentKind: "openapi",
    source: {
      sourceId: "source-openapi-yaml",
      canonicalUrl: "https://api.example.com/openapi.yaml",
      contentType: "application/yaml",
      sourceKind: "openapi",
    },
  }));
  assert.equal(yaml.title, "Billing API");
  assert.equal(yaml.kind, "openapi");
  assert.equal(yaml.format, "openapi-yaml");
  assert.ok(blocksOfType(yaml, "heading").some((heading) => heading.text === "POST /charges"));
  assert.ok(blocksOfType(yaml, "heading").some((heading) => heading.text === "Authentication"));
  assert.match(yaml.blocks.map(renderBlock).join("\n"), /createCharge/);
  assert.ok(blocksOfType(yaml, "table").some((table) => table.links.some((link) => link.target === "https://api.example.com/billing-guide")));

  const jsonContent = JSON.stringify({
    openapi: "3.1.0",
    info: { title: "Users API", version: "1.0" },
    paths: { "/users": { get: { operationId: "listUsers", summary: "List users", responses: { "200": { description: "OK" } } } } },
  });
  const json = normalizeDocument(input(jsonContent, {
    source: { sourceId: "source-openapi-json", canonicalUrl: "https://api.example.com/openapi.json", contentType: "application/json" },
  }));
  assert.equal(json.kind, "openapi");
  assert.equal(json.format, "openapi-json");
  assert.ok(blocksOfType(json, "heading").some((heading) => heading.text === "GET /users"));
  assert.match(json.blocks.map(renderBlock).join("\n"), /listUsers/);
});

test("malformed structured input is retained as inert text with an explicit error", () => {
  const document = normalizeDocument(input('{ "openapi": ', {
    source: { sourceId: "source-bad-json", canonicalUrl: "upload://source-bad-json/openapi.json", contentType: "application/json" },
  }));
  assert.equal(document.blocks.length, 1);
  assert.equal(document.blocks[0].type, "code");
  assert.ok(document.diagnostics.some((diagnostic) => diagnostic.code === "structured_parse_failed" && diagnostic.severity === "error"));
});

test("section-aware segmentation splits prose but never splits tables or code fences", () => {
  const longProse = "This sentence is intentionally long enough to require deterministic segmentation. ".repeat(6);
  const longCode = `const values = [\n${"  1234567890,\n".repeat(12)}];`;
  const document = normalizeDocument(input(`# Large section\n\n${longProse}\n\n| Key | Value |\n| --- | --- |\n| alpha | ${"x".repeat(90)} |\n\n\`\`\`ts\n${longCode}\n\`\`\``));
  const result = segmentDocument(document, { targetCharacters: 120, oversizedCharacters: 180 });
  const code = blocksOfType(document, "code")[0];
  const table = blocksOfType(document, "table")[0];
  const renderedCode = renderBlock(code);
  const renderedTable = renderBlock(table);

  assert.ok(result.sections.length > 3);
  assert.equal(result.sections.filter((section) => section.content.includes(longCode)).length, 1);
  assert.equal(result.sections.filter((section) => section.content.includes("| alpha |")).length, 1);
  const codeSection = result.sections.find((section) => section.content.includes(longCode));
  const tableSection = result.sections.find((section) => section.content.includes("| alpha |"));
  assert.deepEqual(codeSection?.blockSlices, [{ blockId: code.id, start: 0, end: renderedCode.length }]);
  assert.deepEqual(tableSection?.blockSlices, [{ blockId: table.id, start: 0, end: renderedTable.length }]);
  assert.ok(result.diagnostics.some((diagnostic) => diagnostic.code === "section_oversized" && diagnostic.sectionId === codeSection?.id));
  assert.ok(result.sections.every((section) => section.source.rawSha256 === document.source.rawSha256));
});

test("quality diagnostics identify empty, missing-title, duplicate, and boilerplate-heavy documents", () => {
  const empty = normalizeDocument(input("", {
    source: { sourceId: "empty", canonicalUrl: "upload://empty/untitled.txt", contentType: "text/plain" },
  }));
  assert.ok(empty.diagnostics.some((diagnostic) => diagnostic.code === "document_empty"));
  assert.ok(empty.diagnostics.some((diagnostic) => diagnostic.code === "document_missing_title"));

  const sameContent = "# Shared guide\n\nThis is the same reviewed documentation body in two source locations.";
  const duplicateA = normalizeDocument(input(sameContent, { source: { sourceId: "a", canonicalUrl: "https://a.example.com/guide", contentType: "text/markdown" } }));
  const duplicateB = normalizeDocument(input(sameContent, { source: { sourceId: "b", canonicalUrl: "https://b.example.com/guide", contentType: "text/markdown" } }));
  const duplicateSections = [duplicateA, duplicateB].flatMap((document) => segmentDocument(document).sections);
  const duplicateDiagnostics = assessCorpusQuality([duplicateB, duplicateA], duplicateSections);
  assert.deepEqual(duplicateDiagnostics.filter((diagnostic) => diagnostic.code === "document_duplicate").map((diagnostic) => diagnostic.documentId), [duplicateB.id]);

  const repeated = "This repeated legal navigation and support message is deliberately long enough to be recognized as shared boilerplate across pages.";
  const boilerplateDocuments = ["Alpha", "Beta", "Gamma"].map((title, index) => normalizeDocument(input(`# ${title}\n\n${repeated}\n\nUnique ${title}.`, {
    source: { sourceId: `boilerplate-${index}`, canonicalUrl: `https://docs.example.com/${title.toLowerCase()}`, contentType: "text/markdown" },
  })));
  const boilerplateSections = boilerplateDocuments.flatMap((document) => segmentDocument(document).sections);
  const boilerplateDiagnostics = assessCorpusQuality(boilerplateDocuments, boilerplateSections);
  assert.equal(boilerplateDiagnostics.filter((diagnostic) => diagnostic.code === "document_boilerplate").length, 3);
});

test("Documentation Map and evidence contracts are deterministic, structured, and order-independent", () => {
  const auth = input("# Authentication\n\nUse OAuth access tokens.\n\n## Errors\n\nA 401 means the token is invalid.", {
    source: { sourceId: "auth", canonicalUrl: "https://docs.example.com/auth", contentType: "text/markdown" },
  });
  const setup = input("# Quickstart\n\nInstall the client.\n\n```python\nclient = Client(token)\n```", {
    source: { sourceId: "setup", canonicalUrl: "https://docs.example.com/setup", contentType: "text/markdown" },
  });
  const forward = buildDocumentationCorpus([auth, setup]);
  const reverse = buildDocumentationCorpus([setup, auth]);

  assert.deepEqual(forward.map, reverse.map);
  assert.match(forward.map.id, /^documentation_map_[a-f\d]{24}$/);
  assert.match(forward.map.mapSha256, /^[a-f\d]{64}$/);
  assert.equal(forward.map.overview.documentCount, 2);
  assert.deepEqual(forward.map.overview.languages, ["python"]);
  assert.ok(forward.map.entryPoints.some((entry) => entry.kind === "authentication"));
  assert.ok(forward.map.entryPoints.some((entry) => entry.kind === "setup"));
  assert.equal(forward.map.documents.find((document) => document.title === "Authentication")?.outline[0].children[0].title, "Errors");

  const rebuiltMap = buildDocumentationMap(forward.documents, forward.sections, forward.diagnostics);
  const evidence = documentationMapEnrichmentInput(rebuiltMap, forward.documents, forward.sections);
  assert.deepEqual(rebuiltMap, forward.map);
  assert.equal(evidence.mapId, rebuiltMap.id);
  assert.equal(evidence.mapSha256, rebuiltMap.mapSha256);
  assert.equal(new Set(evidence.evidence.map((item) => item.evidenceId)).size, evidence.evidence.length);
  assert.ok(evidence.evidence.some((item) => item.evidenceId.startsWith("document:document_") && item.contentSha256.length === 64));
  assert.ok(evidence.evidence.some((item) => item.evidenceId.startsWith("section:section_") && item.contentSha256.length === 64));
});
