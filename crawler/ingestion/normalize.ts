import path from "node:path";
import { canonicalJSON, normalizeProse, sha256, sourceRange, uniqueSlug } from "./canonical";
import { parseHTMLDocument } from "./html";
import { parseMarkdown } from "./markdown";
import type { DraftBlock, ParseResult } from "./parser-types";
import { blockPlainText } from "./render";
import { parseStructuredDocument } from "./structured";
import {
  NORMALIZED_DOCUMENT_SCHEMA_VERSION,
  type DocumentFormat,
  type DocumentKind,
  type NormalizationInput,
  type NormalizedBlock,
  type NormalizedDocument,
  type QualityDiagnostic,
} from "./types";

function detectFormat(input: NormalizationInput): DocumentFormat {
  if (input.format) return input.format;
  const contentType = input.source.contentType.split(";", 1)[0].trim().toLowerCase();
  let pathname = "";
  try {
    pathname = new URL(input.source.canonicalUrl).pathname.toLowerCase();
  } catch {
    pathname = input.source.canonicalUrl.toLowerCase();
  }
  if (contentType === "text/html" || contentType === "application/xhtml+xml" || /\.html?$/.test(pathname)) return "html";
  if (/\.mdx$/.test(pathname)) return "mdx";
  if (contentType === "text/markdown" || /\.md$/.test(pathname)) return "markdown";
  if (contentType.includes("json") || /\.json$/.test(pathname)) return input.documentKind === "openapi" ? "openapi-json" : "json";
  if (contentType.includes("yaml") || /\.ya?ml$/.test(pathname)) return input.documentKind === "openapi" ? "openapi-yaml" : "yaml";
  return "text";
}

function parseText(content: string): ParseResult {
  const blocks: DraftBlock[] = [];
  const pattern = /\S[\s\S]*?(?=\n\s*\n|$)/g;
  for (const match of content.matchAll(pattern)) {
    const text = normalizeProse(match[0]);
    if (!text) continue;
    blocks.push({
      type: "paragraph",
      text,
      links: [],
      sourceRange: sourceRange(content, match.index, match.index + match[0].length),
    });
  }
  return { title: "", blocks, diagnostics: [] };
}

function parsedDocument(input: NormalizationInput, format: DocumentFormat): ParseResult & { kind: DocumentKind } {
  const requestedKind = input.documentKind ?? (format.startsWith("openapi-") ? "openapi" : "documentation");
  switch (format) {
    case "html":
      return { ...parseHTMLDocument(input.content, input.source.canonicalUrl), kind: requestedKind };
    case "markdown":
      return { ...parseMarkdown(input.content, input.source.canonicalUrl), kind: requestedKind };
    case "mdx":
      return { ...parseMarkdown(input.content, input.source.canonicalUrl, true), kind: requestedKind };
    case "json":
    case "openapi-json":
      return parseStructuredDocument(input.content, "json", requestedKind, input.source.canonicalUrl);
    case "yaml":
    case "openapi-yaml":
      return parseStructuredDocument(input.content, "yaml", requestedKind, input.source.canonicalUrl);
    case "text":
      return { ...parseText(input.content), kind: requestedKind };
  }
}

function fallbackTitle(canonicalUrl: string): string {
  try {
    const url = new URL(canonicalUrl);
    const basename = path.posix.basename(url.pathname);
    return decodeURIComponent(basename || url.hostname) || "Untitled document";
  } catch {
    return path.basename(canonicalUrl) || "Untitled document";
  }
}

function blockPayload(block: DraftBlock): unknown {
  switch (block.type) {
    case "heading":
      return { type: block.type, level: block.level, text: block.text, anchor: block.anchor ?? null, links: block.links };
    case "paragraph":
    case "quote":
      return { type: block.type, text: block.text, links: block.links };
    case "list":
      return { type: block.type, ordered: block.ordered, items: block.items };
    case "table":
      return { type: block.type, headers: block.headers, rows: block.rows, links: block.links };
    case "code":
      return { type: block.type, language: block.language, code: block.code };
  }
}

function hasContent(block: DraftBlock): boolean {
  switch (block.type) {
    case "heading":
    case "paragraph":
    case "quote":
      return Boolean(block.text);
    case "list":
      return block.items.some((item) => item.text);
    case "table":
      return block.headers.some(Boolean) || block.rows.some((row) => row.some(Boolean));
    case "code":
      return Boolean(block.code);
  }
}

function finalizeBlocks(documentIdentity: string, drafts: readonly DraftBlock[]): NormalizedBlock[] {
  const anchors = new Map<string, number>();
  return drafts.filter(hasContent).map((draft, ordinal): NormalizedBlock => {
    const payload = blockPayload(draft);
    const contentSha256 = sha256(canonicalJSON(payload));
    const base = {
      ...draft,
      id: `block_${sha256(canonicalJSON({ documentIdentity, ordinal, contentSha256, range: draft.sourceRange })).slice(0, 24)}`,
      ordinal,
      contentSha256,
    };
    if (base.type !== "heading") return base;
    const requestedAnchor = base.anchor || base.text;
    return { ...base, anchor: uniqueSlug(requestedAnchor, anchors) };
  });
}

export function normalizeDocument(input: NormalizationInput): NormalizedDocument {
  const format = detectFormat(input);
  const rawSha256 = sha256(input.content);
  const documentIdentity = canonicalJSON({ sourceId: input.source.sourceId, canonicalUrl: input.source.canonicalUrl, rawSha256 });
  const id = `document_${sha256(documentIdentity).slice(0, 24)}`;
  const parsed = parsedDocument(input, format);
  const suppliedTitle = normalizeProse(input.title ?? "");
  const parsedTitle = normalizeProse(parsed.title);
  const missingTitle = !suppliedTitle && !parsedTitle;
  const title = suppliedTitle || parsedTitle || fallbackTitle(input.source.canonicalUrl);
  const blocks = finalizeBlocks(documentIdentity, parsed.blocks);
  const normalizedSha256 = sha256(canonicalJSON({
    title,
    format,
    kind: parsed.kind,
    blocks: blocks.map(blockPayload),
  }));
  const diagnostics: QualityDiagnostic[] = parsed.diagnostics.map((diagnostic) => ({ ...diagnostic, documentId: id }));
  if (missingTitle) diagnostics.push({
    code: "document_missing_title",
    severity: "warning",
    scope: "document",
    documentId: id,
    message: "The source has no explicit document title; a filename or host fallback is being used.",
  });
  if (blocks.every((block) => !blockPlainText(block).trim())) diagnostics.push({
    code: "document_empty",
    severity: "error",
    scope: "document",
    documentId: id,
    message: "The source produced no reviewable normalized content.",
  });

  return {
    schemaVersion: NORMALIZED_DOCUMENT_SCHEMA_VERSION,
    id,
    title,
    format: parsed.kind === "openapi" && format === "json" ? "openapi-json"
      : parsed.kind === "openapi" && format === "yaml" ? "openapi-yaml"
        : format,
    kind: parsed.kind,
    source: { ...input.source, rawSha256 },
    normalizedSha256,
    blocks,
    diagnostics,
  };
}
