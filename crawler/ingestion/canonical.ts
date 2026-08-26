import { createHash } from "node:crypto";
import type { SourceRange } from "./types";

export function sha256(value: string): string {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

/** Locale-independent ordering for hashes and publication payloads. */
export function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function canonicalValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalValue);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .filter(([, item]) => item !== undefined)
        .sort(([left], [right]) => compareText(left, right))
        .map(([key, item]) => [key, canonicalValue(item)]),
    );
  }
  return value;
}

export function canonicalJSON(value: unknown): string {
  return JSON.stringify(canonicalValue(value));
}

export function normalizeProse(value: string): string {
  return value
    .normalize("NFC")
    .replaceAll("\0", "")
    .replace(/\s+/g, " ")
    .trim();
}

export function normalizeCode(value: string): string {
  return value
    .normalize("NFC")
    .replaceAll("\0", "")
    .replace(/\r\n?/g, "\n")
    .replace(/[ \t]+$/gm, "")
    .replace(/^\n+|\n+$/g, "");
}

export function sourceRange(content: string, startOffset: number, endOffset: number): SourceRange {
  const boundedStart = Math.max(0, Math.min(content.length, startOffset));
  const boundedEnd = Math.max(boundedStart, Math.min(content.length, endOffset));
  return {
    startOffset: boundedStart,
    endOffset: boundedEnd,
    startLine: content.slice(0, boundedStart).split("\n").length,
    endLine: content.slice(0, boundedEnd).split("\n").length,
  };
}

export function slug(value: string): string {
  const normalized = value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return normalized || "section";
}

export function uniqueSlug(value: string, counts: Map<string, number>): string {
  const base = slug(value);
  const next = (counts.get(base) ?? 0) + 1;
  counts.set(base, next);
  return next === 1 ? base : `${base}-${next}`;
}

export function resolveLink(target: string, canonicalUrl: string): string | null {
  const cleaned = target.trim().replace(/^<|>$/g, "");
  if (!cleaned || /^(?:javascript|data|vbscript):/i.test(cleaned)) return null;
  try {
    const url = new URL(cleaned, canonicalUrl);
    return ["http:", "https:", "mailto:"].includes(url.protocol) ? url.toString() : null;
  } catch {
    return null;
  }
}
