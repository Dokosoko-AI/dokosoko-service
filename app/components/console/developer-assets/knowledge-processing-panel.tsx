"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { developerAssetsApi, type KnowledgeProcessingStatus } from "../../../lib/developer-assets-api";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { Button } from "../../core/control";
import { developerAssetError } from "./developer-asset-ui";

// Parents key this panel by the exact run and compare that run with onReady's
// value. A response from a prior selection can never enable its publication.
export function KnowledgeProcessingPanel({ runID, onReady, compactAI = false }: { compactAI?: boolean; runID: string; onReady: (runID: string) => void }) {
  const { t } = useTranslation();
  const [canProcess, setCanProcess] = useState(false);
  const [status, setStatus] = useState<KnowledgeProcessingStatus | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [pausing, setPausing] = useState(false);
  const mounted = useRef(false);
  const continuing = useRef(false);
  const pending = useRef(false);
  const publishStatus = useCallback((value: KnowledgeProcessingStatus) => {
    if (!mounted.current) return;
    setStatus(value);
    onReady(value.state === "ready" ? runID : "");
  }, [onReady, runID]);

  const refresh = useCallback(async () => {
    try {
      const value = await developerAssetsApi.knowledgeProcessing(runID);
      publishStatus(value);
      if (mounted.current) setProblem("");
    } catch (error) {
      if (mounted.current) {
        onReady("");
        setProblem(developerAssetError(error, t("knowledgeProcessing.failed")));
      }
    }
  }, [onReady, publishStatus, runID, t]);

  useEffect(() => {
    mounted.current = true;
    queueMicrotask(() => { if (mounted.current) void refresh(); });
    return () => { mounted.current = false; continuing.current = false; };
  }, [refresh]);

  // A reload can encounter another request's still-running batch. Observe it;
  // starting subsequent batches remains an explicit operator action.
  useEffect(() => {
    if (status?.state !== "running" || busy) return;
    const timer = window.setInterval(() => void refresh(), 3000);
    return () => window.clearInterval(timer);
  }, [busy, refresh, status?.state]);

  async function process() {
    if (pending.current || !canProcess) return;
    pending.current = true;
    continuing.current = true;
    setBusy(true);
    setPausing(false);
    setProblem("");
    try {
      while (mounted.current && continuing.current) {
        const value = await developerAssetsApi.processKnowledgeBatch(runID);
        publishStatus(value);
        if (value.state === "ready") break;
      }
    } catch (error) {
      if (mounted.current) {
        setProblem(developerAssetError(error, t("knowledgeProcessing.failed")));
        // Keep partial progress visible even when the failed POST returned an
        // error envelope. Do not replace the actionable error with a refresh.
        try { publishStatus(await developerAssetsApi.knowledgeProcessing(runID)); } catch { /* Keep the original failure. */ }
      }
    } finally {
      pending.current = false;
      continuing.current = false;
      if (mounted.current) { setBusy(false); setPausing(false); }
    }
  }

  return <section className="knowledge-processing" aria-label={t("knowledgeProcessing.title")}>
    <h3>{t("knowledgeProcessing.title")}</h3>
    {status?.state !== "ready" && <AIReadinessPanel compact={compactAI} preserveForm onCanProcess={setCanProcess} refreshKey={status?.error_code ?? ""} />}
    <p>{t("knowledgeProcessing.description")}</p>
    <p aria-live="polite">{status ? status.state === "ready" ? t("knowledgeProcessing.ready") : t("knowledgeProcessing.progress", { completed: status.completed_batches, total: status.total_batches }) : t("common.loading")}</p>
    {problem && <p className="auth-problem" role="alert">{problem}</p>}
    {!problem && status?.error_code && <p className="auth-problem">{t("knowledgeProcessing.previousFailure", { code: status.error_code })}</p>}
    <div className="knowledge-processing-actions">
      {busy ? <Button outline disabled={pausing} onClick={() => { continuing.current = false; setPausing(true); }}>{t(pausing ? "knowledgeProcessing.pausing" : "knowledgeProcessing.pause")}</Button> : status?.state !== "ready" && <Button disabled={!status || !canProcess || status.state === "running"} onClick={() => void process()}>{t(status?.completed_batches ? "knowledgeProcessing.resume" : "knowledgeProcessing.start")}</Button>}
      {!busy && <Button outline onClick={() => void refresh()}>{t("knowledgeProcessing.refresh")}</Button>}
    </div>
    {Boolean(status?.assessments.length) && <details>
      <summary>{t("knowledgeProcessing.results", { count: status?.assessments.length ?? 0 })}</summary>
      <p>{t("knowledgeProcessing.reviewAid")}</p>
      {status?.assessments.map((assessment) => <details key={assessment.id}>
        <summary>{assessment.title} · {t("knowledgeProcessing.part", { part: assessment.part, total: assessment.part_count })}</summary>
        <p>{assessment.summary}</p>
        <blockquote>{assessment.evidence_quote}</blockquote>
        {assessment.findings.length > 0 && <ul>{assessment.findings.map((finding) => <li key={finding}>{t(`knowledgeProcessing.findings.${finding}`)}</li>)}</ul>}
      </details>)}
    </details>}
  </section>;
}
