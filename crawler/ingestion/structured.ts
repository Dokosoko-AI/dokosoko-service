import { normalizeCode, normalizeProse, resolveLink, sourceRange } from "./canonical";
import type { DraftBlock, ParseResult } from "./parser-types";
import type { DocumentKind, NormalizedLink } from "./types";

type YAMLToken = {
  indent: number;
  sequence: boolean;
  key: string | null;
  value: string | null;
  literal: boolean;
  start: number;
  end: number;
};

function stripYAMLComment(value: string): string {
  let single = false;
  let double = false;
  let escaped = false;
  for (let index = 0; index < value.length; index++) {
    const character = value[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (character === "\\" && double) {
      escaped = true;
      continue;
    }
    if (character === "'" && !double) single = !single;
    else if (character === '"' && !single) double = !double;
    else if (character === "#" && !single && !double && (index === 0 || /\s/.test(value[index - 1]))) return value.slice(0, index).trimEnd();
  }
  return value.trimEnd();
}

function keyValue(value: string): { key: string; value: string } | null {
  let single = false;
  let double = false;
  let square = 0;
  let curly = 0;
  for (let index = 0; index < value.length; index++) {
    const character = value[index];
    if (character === "'" && !double) single = !single;
    else if (character === '"' && !single && value[index - 1] !== "\\") double = !double;
    else if (!single && !double) {
      if (character === "[") square++;
      else if (character === "]") square--;
      else if (character === "{") curly++;
      else if (character === "}") curly--;
      else if (character === ":" && square === 0 && curly === 0 && (index + 1 === value.length || /\s/.test(value[index + 1]))) {
        const rawKey = value.slice(0, index).trim();
        if (!rawKey) return null;
        return {
          key: rawKey.replace(/^['"]|['"]$/g, ""),
          value: value.slice(index + 1).trim(),
        };
      }
    }
  }
  return null;
}

function yamlTokens(content: string): YAMLToken[] {
  const rawLines: Array<{ text: string; start: number; end: number }> = [];
  let offset = 0;
  for (const raw of content.match(/.*(?:\n|$)/g) ?? []) {
    if (!raw) continue;
    const withoutNewline = raw.endsWith("\n") ? raw.slice(0, -1) : raw;
    const text = withoutNewline.endsWith("\r") ? withoutNewline.slice(0, -1) : withoutNewline;
    rawLines.push({ text, start: offset, end: offset + raw.length });
    offset += raw.length;
  }

  const tokens: YAMLToken[] = [];
  for (let index = 0; index < rawLines.length; index++) {
    const line = rawLines[index];
    if (!line.text.trim() || /^\s*#/.test(line.text) || /^\s*(?:---|\.\.\.)\s*$/.test(line.text)) continue;
    if (/^\s*\t/.test(line.text)) throw new Error("YAML indentation must use spaces.");
    const indent = line.text.match(/^ */)?.[0].length ?? 0;
    let value = stripYAMLComment(line.text.slice(indent));
    const sequence = /^-\s*(?:.*)$/.test(value);
    if (sequence) value = value.replace(/^-\s*/, "");
    const pair = keyValue(value);
    const rawValue = pair?.value ?? (pair ? "" : value);
    const literalMarker = /^[|>][+-]?$/.test(rawValue);
    let literalValue: string | null = null;
    let end = line.end;
    if (literalMarker) {
      const literalLines: string[] = [];
      let minimumIndent = Number.POSITIVE_INFINITY;
      let cursor = index + 1;
      for (; cursor < rawLines.length; cursor++) {
        const candidate = rawLines[cursor];
        const candidateIndent = candidate.text.match(/^ */)?.[0].length ?? 0;
        if (candidate.text.trim() && candidateIndent <= indent) break;
        if (candidate.text.trim()) minimumIndent = Math.min(minimumIndent, candidateIndent);
        literalLines.push(candidate.text);
        end = candidate.end;
      }
      const strip = Number.isFinite(minimumIndent) ? minimumIndent : indent + 1;
      const values = literalLines.map((item) => item.slice(Math.min(strip, item.length)));
      literalValue = rawValue.startsWith(">") ? normalizeProse(values.join(" ")) : normalizeCode(values.join("\n"));
      index = cursor - 1;
    }
    tokens.push({
      indent,
      sequence,
      key: pair?.key ?? null,
      value: literalValue ?? (rawValue || null),
      literal: literalMarker,
      start: line.start,
      end,
    });
  }
  return tokens;
}

function yamlScalar(raw: string | null, literal = false): unknown {
  if (raw === null) return null;
  if (literal) return raw;
  const value = raw.trim();
  if (/^(?:null|~)$/i.test(value)) return null;
  if (/^(?:true|false)$/i.test(value)) return value.toLowerCase() === "true";
  if (/^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:e[+-]?\d+)?$/i.test(value)) return Number(value);
  if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
    if (value.startsWith('"')) {
      try {
        return JSON.parse(value);
      } catch {
        return value.slice(1, -1);
      }
    }
    return value.slice(1, -1).replaceAll("''", "'");
  }
  if ((value.startsWith("[") && value.endsWith("]")) || (value.startsWith("{") && value.endsWith("}"))) {
    try {
      return JSON.parse(value);
    } catch {
      if (value.startsWith("[")) return value.slice(1, -1).split(",").map((item) => yamlScalar(item.trim()));
    }
  }
  return value;
}

function parseYAML(content: string): unknown {
  const tokens = yamlTokens(content);
  if (tokens.length === 0) return {};

  const parseBlock = (start: number, indent: number): { value: unknown; next: number } => {
    const sequence = tokens[start]?.sequence ?? false;
    if (sequence) {
      const result: unknown[] = [];
      let index = start;
      while (index < tokens.length && tokens[index].indent === indent && tokens[index].sequence) {
        const token = tokens[index];
        if (token.key !== null) {
          const object: Record<string, unknown> = {};
          if (token.value !== null) object[token.key] = yamlScalar(token.value, token.literal);
          else if (tokens[index + 1] && tokens[index + 1].indent > indent) {
            const nested = parseBlock(index + 1, tokens[index + 1].indent);
            object[token.key] = nested.value;
            index = nested.next - 1;
          } else object[token.key] = null;

          if (tokens[index + 1] && tokens[index + 1].indent > indent) {
            const nested = parseBlock(index + 1, tokens[index + 1].indent);
            if (nested.value && typeof nested.value === "object" && !Array.isArray(nested.value)) Object.assign(object, nested.value);
            else object.value = nested.value;
            index = nested.next - 1;
          }
          result.push(object);
        } else if (token.value !== null) {
          result.push(yamlScalar(token.value, token.literal));
          if (tokens[index + 1] && tokens[index + 1].indent > indent) {
            const nested = parseBlock(index + 1, tokens[index + 1].indent);
            result.push(nested.value);
            index = nested.next - 1;
          }
        } else if (tokens[index + 1] && tokens[index + 1].indent > indent) {
          const nested = parseBlock(index + 1, tokens[index + 1].indent);
          result.push(nested.value);
          index = nested.next - 1;
        } else result.push(null);
        index++;
      }
      return { value: result, next: index };
    }

    const result: Record<string, unknown> = {};
    let index = start;
    while (index < tokens.length && tokens[index].indent === indent && !tokens[index].sequence) {
      const token = tokens[index];
      if (token.key === null) throw new Error("Expected a YAML mapping key.");
      if (token.value !== null) result[token.key] = yamlScalar(token.value, token.literal);
      else if (tokens[index + 1] && tokens[index + 1].indent > indent) {
        const nested = parseBlock(index + 1, tokens[index + 1].indent);
        result[token.key] = nested.value;
        index = nested.next - 1;
      } else result[token.key] = null;
      index++;
    }
    return { value: result, next: index };
  };

  const parsed = parseBlock(0, tokens[0].indent);
  if (parsed.next !== tokens.length) throw new Error("YAML structure contains inconsistent indentation.");
  return parsed.value;
}

/**
 * Parse inert JSON or the deliberately bounded YAML subset accepted by the
 * crawler. This never evaluates tags, constructors, MDX, or executable code.
 */
export function parseStructuredValue(content: string, format: "json" | "yaml"): unknown {
  return format === "json" ? JSON.parse(content) : parseYAML(content);
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function scalarText(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "string") return normalizeProse(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return normalizeProse(JSON.stringify(value));
}

function displayKey(value: string): string {
  const spaced = value.replace(/[_-]+/g, " ").replace(/([a-z\d])([A-Z])/g, "$1 $2");
  return spaced ? spaced[0].toUpperCase() + spaced.slice(1) : "Section";
}

function structuredLinks(value: unknown, canonicalUrl: string): NormalizedLink[] {
  const result: NormalizedLink[] = [];
  const seen = new Set<string>();
  const add = (label: string, rawTarget: string) => {
    const target = resolveLink(rawTarget, canonicalUrl);
    const normalizedLabel = normalizeProse(label) || rawTarget;
    const key = `${normalizedLabel}\0${target}`;
    if (target && !seen.has(key)) {
      seen.add(key);
      result.push({ label: normalizedLabel, target });
    }
  };
  const visit = (candidate: unknown, field = "link") => {
    if (typeof candidate === "string") {
      for (const match of candidate.matchAll(/\[([^\]]+)]\(([^)\s]+)\)|\b((?:https?:\/\/|mailto:)[^\s<>'"]+)/gi)) {
        add(match[1] ?? field, match[2] ?? match[3]);
      }
      if (/^(?:url|href|externalDocs|documentation)$/i.test(field)) add(displayKey(field), candidate);
    } else if (Array.isArray(candidate)) candidate.forEach((item) => visit(item, field));
    else if (candidate && typeof candidate === "object") Object.entries(candidate as Record<string, unknown>).forEach(([key, item]) => visit(item, key));
  };
  visit(value);
  return result;
}

function structuredBlocks(value: unknown, content: string, canonicalUrl: string): DraftBlock[] {
  const blocks: DraftBlock[] = [];
  const range = sourceRange(content, 0, content.length);
  const addHeading = (level: number, text: string) => blocks.push({ type: "heading", level: Math.min(6, level), text, links: [], sourceRange: range });
  const walk = (candidate: unknown, level: number, key = "") => {
    if (Array.isArray(candidate)) {
      if (candidate.every((item) => item === null || ["string", "number", "boolean"].includes(typeof item))) {
        blocks.push({
          type: "list",
          ordered: false,
          items: candidate.map((item) => ({ text: scalarText(item), depth: 0, links: [] })),
          sourceRange: range,
        });
      } else {
        candidate.forEach((item, index) => {
          const itemObject = objectValue(item);
          const label = scalarText(itemObject?.name ?? itemObject?.title ?? itemObject?.operationId ?? `Item ${index + 1}`);
          addHeading(level, label);
          walk(item, level + 1);
        });
      }
      return;
    }
    const object = objectValue(candidate);
    if (!object) {
      const text = scalarText(candidate);
      if (text) blocks.push({ type: "paragraph", text, links: [], sourceRange: range });
      return;
    }
    const scalarRows: string[][] = [];
    for (const [field, item] of Object.entries(object)) {
      if (item === null || ["string", "number", "boolean"].includes(typeof item)) {
        const text = scalarText(item);
        if (/^(?:description|summary)$/i.test(field) && text) blocks.push({ type: "paragraph", text, links: structuredLinks(item, canonicalUrl), sourceRange: range });
        else scalarRows.push([displayKey(field), text]);
      }
    }
    if (scalarRows.length > 0) blocks.push({ type: "table", headers: ["Field", "Value"], rows: scalarRows, links: structuredLinks(object, canonicalUrl), sourceRange: range });
    for (const [field, item] of Object.entries(object)) {
      if (item !== null && typeof item === "object") {
        addHeading(level, displayKey(field));
        walk(item, level + 1, field);
      } else if (typeof item === "string" && item.includes("\n") && /(?:code|example|schema|request|response)/i.test(field || key)) {
        blocks.push({ type: "code", language: null, code: normalizeCode(item), sourceRange: range });
      }
    }
  };
  walk(value, 2);
  return blocks;
}

const httpMethods = new Set(["get", "put", "post", "delete", "options", "head", "patch", "trace"]);

function openAPIBlocks(value: Record<string, unknown>, content: string, canonicalUrl: string): DraftBlock[] {
  const range = sourceRange(content, 0, content.length);
  const blocks: DraftBlock[] = [];
  const info = objectValue(value.info) ?? {};
  const description = scalarText(info.description ?? "");
  blocks.push({ type: "heading", level: 2, text: "Overview", links: [], sourceRange: range });
  if (description) blocks.push({ type: "paragraph", text: description, links: structuredLinks(info.description, canonicalUrl), sourceRange: range });
  blocks.push({
    type: "table",
    headers: ["Field", "Value"],
    rows: [
      ["OpenAPI version", scalarText(value.openapi ?? value.swagger ?? "")],
      ["API version", scalarText(info.version ?? "")],
    ].filter((row) => row[1]),
    links: structuredLinks(value, canonicalUrl),
    sourceRange: range,
  });

  const components = objectValue(value.components);
  const securitySchemes = objectValue(components?.securitySchemes);
  if (securitySchemes && Object.keys(securitySchemes).length > 0) {
    blocks.push({ type: "heading", level: 2, text: "Authentication", links: [], sourceRange: range });
    for (const [name, scheme] of Object.entries(securitySchemes)) {
      blocks.push({ type: "heading", level: 3, text: name, links: [], sourceRange: range });
      blocks.push(...structuredBlocks(scheme, content, canonicalUrl).filter((block) => block.type !== "heading"));
    }
  }

  const paths = objectValue(value.paths) ?? {};
  blocks.push({ type: "heading", level: 2, text: "Operations", links: [], sourceRange: range });
  for (const [path, pathItem] of Object.entries(paths)) {
    const operations = objectValue(pathItem) ?? {};
    for (const [method, operationValue] of Object.entries(operations)) {
      if (!httpMethods.has(method.toLowerCase())) continue;
      const operation = objectValue(operationValue) ?? {};
      blocks.push({ type: "heading", level: 3, text: `${method.toUpperCase()} ${path}`, links: [], sourceRange: range });
      const summary = scalarText(operation.summary ?? operation.description ?? "");
      if (summary) blocks.push({ type: "paragraph", text: summary, links: [], sourceRange: range });
      const operationRows = [
        ["Operation ID", scalarText(operation.operationId ?? "")],
        ["Tags", Array.isArray(operation.tags) ? operation.tags.map(scalarText).join(", ") : ""],
        ["Deprecated", operation.deprecated === true ? "true" : ""],
      ].filter((row) => row[1]);
      if (operationRows.length > 0) blocks.push({ type: "table", headers: ["Field", "Value"], rows: operationRows, links: [], sourceRange: range });
      for (const field of ["parameters", "requestBody", "responses", "callbacks"] as const) {
        if (!(field in operation)) continue;
        blocks.push({ type: "heading", level: 4, text: displayKey(field), links: [], sourceRange: range });
        blocks.push({ type: "code", language: "json", code: JSON.stringify(operation[field], null, 2), sourceRange: range });
      }
    }
  }

  const schemas = objectValue(components?.schemas);
  if (schemas && Object.keys(schemas).length > 0) {
    blocks.push({ type: "heading", level: 2, text: "Schemas", links: [], sourceRange: range });
    for (const [name, schema] of Object.entries(schemas)) {
      blocks.push({ type: "heading", level: 3, text: name, links: [], sourceRange: range });
      blocks.push({ type: "code", language: "json", code: JSON.stringify(schema, null, 2), sourceRange: range });
    }
  }
  return blocks;
}

export function parseStructuredDocument(
  content: string,
  format: "json" | "yaml",
  requestedKind: DocumentKind,
  canonicalUrl: string,
): ParseResult & { kind: DocumentKind } {
  let value: unknown;
  try {
    value = parseStructuredValue(content, format);
  } catch {
    return {
      title: "",
      kind: requestedKind,
      blocks: [{ type: "code", language: format, code: normalizeCode(content), sourceRange: sourceRange(content, 0, content.length) }],
      diagnostics: [{
        code: "structured_parse_failed",
        severity: "error",
        scope: "document",
        message: `The ${format.toUpperCase()} source could not be parsed structurally and was preserved as inert text.`,
      }],
    };
  }
  const object = objectValue(value);
  const openapi = Boolean(object && (typeof object.openapi === "string" || typeof object.swagger === "string") && objectValue(object.paths));
  const kind = openapi ? "openapi" : requestedKind;
  const info = objectValue(object?.info);
  const title = scalarText(info?.title ?? object?.title ?? "");
  return {
    title,
    kind,
    blocks: openapi && object ? openAPIBlocks(object, content, canonicalUrl) : structuredBlocks(value, content, canonicalUrl),
    diagnostics: [],
  };
}
