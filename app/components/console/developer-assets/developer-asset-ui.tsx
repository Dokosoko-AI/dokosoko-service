"use client";

import { RefreshCw, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

import { APIError } from "../../../lib/api";
import type { DeveloperAssetRecord } from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";

export function developerAssetError(error: unknown, fallback: string) {
  return error instanceof APIError || error instanceof Error ? error.message : fallback;
}

export function recordString(record: DeveloperAssetRecord, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return "";
}

export function recordID(record: DeveloperAssetRecord, fallback: string) {
  return recordString(record, "id", "file_id", "sample_id", "symbol_id", "section_id", "source_path", "name") || fallback;
}

export function recordTitle(record: DeveloperAssetRecord, fallback: string) {
  return recordString(record, "title", "name", "heading", "symbol", "qualified_name", "source_path", "path", "id") || fallback;
}

export function PrettyJSON({ value, label }: { value: unknown; label?: string }) {
  return <pre className="developer-asset-json" aria-label={label}><code>{JSON.stringify(value ?? {}, null, 2)}</code></pre>;
}

export function MarkdownEvidence({ children, label }: { children: string; label: string }) {
  return <pre className="developer-asset-markdown" aria-label={label}><code>{children || "No generated navigation markdown is available for this exact revision."}</code></pre>;
}

export function LoadingPanel({ label }: { label: string }) {
  return <section className="panel developer-asset-state" role="status"><RefreshCw className="spin" /><span><strong>{label}</strong><small>Reading deployment-owned records…</small></span></section>;
}

export function ProblemPanel({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return <section className="panel developer-asset-state problem" role="alert"><TriangleAlert /><span><strong>Developer assets unavailable</strong><small>{message}</small></span>{onRetry && <Button outline onClick={onRetry}>Retry</Button>}</section>;
}

export function EmptyPanel({ title, detail, action }: { title: string; detail: string; action?: ReactNode }) {
  return <section className="panel developer-asset-empty"><span><strong>{title}</strong><small>{detail}</small></span>{action}</section>;
}

export function ReviewStateBadge({ state }: { state: string }) {
  const normalized = state.toLowerCase();
  const color = normalized === "published" || normalized === "ready" || normalized === "review_ready" || normalized === "active"
    ? "green"
    : normalized === "failed" || normalized === "quarantined" || normalized === "yanked"
      ? "red"
      : normalized === "running" || normalized === "queued"
        ? "blue"
        : normalized === "draft" || normalized === "deprecated"
          ? "amber"
          : "zinc";
  return <Badge color={color}>{state.replaceAll("_", " ")}</Badge>;
}
