"use client";


import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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
  const { t } = useTranslation();
  return <pre className="developer-asset-markdown" aria-label={label}><code>{children || t("developerAssets.noGeneratedNavigationMarkdown")}</code></pre>;
}

export function LoadingPanel({ label }: { label: string }) {
  const { t } = useTranslation();
  return <section className="panel developer-asset-state" role="status"><RefreshCw className="spin" /><span><strong>{label}</strong><small>{t("developerAssets.readingDeploymentOwnedRecords")}</small></span></section>;
}

export function ProblemPanel({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const { t } = useTranslation();
  return <section className="panel developer-asset-state problem" role="alert"><TriangleAlert /><span><strong>{t("developerAssets.developerAssetsUnavailable")}</strong><small>{message}</small></span>{onRetry && <Button outline onClick={onRetry}>{t("common.retry")}</Button>}</section>;
}

export function EmptyPanel({ title, detail, action }: { title: string; detail: string; action?: ReactNode }) {
  return <section className="panel developer-asset-empty"><span><strong>{title}</strong><small>{detail}</small></span>{action}</section>;
}

const enumTranslationKeys = {
  active: "enumLabels.active",
  archived: "enumLabels.archived",
  attached: "enumLabels.attached",
  cancelled: "enumLabels.cancelled",
  deprecated: "enumLabels.deprecated",
  detached: "enumLabels.detached",
  draft: "enumLabels.draft",
  excluded: "enumLabels.excluded",
  failed: "enumLabels.failed",
  included: "enumLabels.included",
  needs_review: "enumLabels.needsReview",
  partial: "enumLabels.partial",
  published: "enumLabels.published",
  quarantined: "enumLabels.quarantined",
  queued: "enumLabels.queued",
  ready: "enumLabels.ready",
  review: "enumLabels.review",
  review_ready: "enumLabels.reviewReady",
  running: "enumLabels.running",
  suspended: "enumLabels.suspended",
  valid: "enumLabels.valid",
  validated: "enumLabels.validated",
  yanked: "enumLabels.yanked",
  metadata_only: "enumLabels.metadataOnly",
  resolved_source: "enumLabels.resolvedSource",
  source_publication: "enumLabels.sourcePublication",
  document: "enumLabels.document",
  section: "enumLabels.section",
} as const;

export function enumLabel(t: TFunction, value: string) {
  const key = enumTranslationKeys[value.toLowerCase() as keyof typeof enumTranslationKeys];
  return key ? t(key) : t("enumLabels.unknown", { value });
}

export function ReviewStateBadge({ state }: { state: string }) {
  const { t } = useTranslation();
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
  return <Badge color={color}>{enumLabel(t, state)}</Badge>;
}
