"use client";

import { useTranslation } from "react-i18next";
import { APIError } from "../../../lib/api-client";
import { developerAssetError } from "./developer-asset-ui";
import { useRef, useState } from "react";
import { api } from "../../../lib/api";
import { assetCreationAttempt } from "../../../lib/asset-creation-request";
import { browserReviewStorage, clearReviewDraft } from "../../../lib/review-draft";
import type { SourceCreationAttempt } from "../../../lib/source-creation-request";

// Keep the request until the complete create/attach transition succeeds. A
// reload stores no names, content, selected evidence or approval, only a digest
// and random request key. The user reviews the same input again to resume it.
export function useAssetCreation(reviewerID: string, context: string) {
  const { t } = useTranslation();
  const pending = useRef<{ scope: string; attempt: SourceCreationAttempt } | null>(null);
  const [recoveryUnavailable, setRecoveryUnavailable] = useState(false);
  async function create<T extends { id: string; deployment_id: string; revision: number }>(kind: "api_contract" | "documentation_collection", input: unknown, save: (key: string) => Promise<T>) {
    const deployment = await api.deployment();
    const value = await assetCreationAttempt(browserReviewStorage, { deploymentID: deployment.id, reviewerID, kind, context }, input, pending.current?.attempt);
    pending.current = value;
    setRecoveryUnavailable(!value.persisted);
    const result = await save(value.attempt.key);
    if (!result || typeof result.id !== "string" || !result.id || result.deployment_id !== deployment.id || !Number.isInteger(result.revision) || result.revision < 1) throw new Error(t("assetCreation.retrySameInput"));
    return result;
  }
  function complete() {
    if (pending.current) clearReviewDraft(browserReviewStorage, pending.current.scope);
    pending.current = null;
    setRecoveryUnavailable(false);
  }
  function error(problem: unknown, fallback: string) {
    return problem instanceof TypeError || problem instanceof APIError && problem.status >= 500 ? t("assetCreation.retrySameInput") : developerAssetError(problem, fallback);
  }
  return { create, complete, error, recoveryUnavailable };
}
