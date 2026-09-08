"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APIAIProcessingReadiness } from "../../lib/api";
import { aiSettingsForSetup } from "../../lib/ai-setup-navigation";
import { settingsPath } from "../../lib/console-routes";
import { Badge, Button } from "../core/control";
import { developerAssetError } from "./developer-assets/developer-asset-ui";

// Fetch only the administrative readiness projection; never load provider
// credentials, all settings, or recipe histories into an import screen.
export function AIReadinessPanel({ onCanProcess, refreshKey = "", inSettings = false, preserveForm = false, compact = false }: {
  onCanProcess?: (ready: boolean) => void; refreshKey?: string; inSettings?: boolean; preserveForm?: boolean; compact?: boolean;
}) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<APIAIProcessingReadiness | null>(null);
  const [problem, setProblem] = useState("");
  const [testProblem, setTestProblem] = useState("");
  const [testing, setTesting] = useState(false);
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const [settingsURL, setSettingsURL] = useState(settingsPath("ai"));
  useEffect(() => {
    let cancelled = false;
    queueMicrotask(() => {
      if (cancelled) return;
      setLoading(true); onCanProcess?.(false);
      setSettingsURL(aiSettingsForSetup(window.location.pathname + window.location.search));
      api.aiReadiness().then((value) => {
        if (cancelled) return;
        setStatus(value); setProblem(""); onCanProcess?.(value.can_process);
      }).catch((error) => { if (!cancelled) { setStatus(null); setProblem(developerAssetError(error, t("aiReadiness.unavailable"))); } })
        .finally(() => { if (!cancelled) setLoading(false); });
    });
    return () => { cancelled = true; };
  }, [attempt, onCanProcess, refreshKey, t]);
  useEffect(() => {
    const refresh = () => setAttempt((value) => value + 1);
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, []);

  async function testConnection() {
    if (!status?.provider_connection_id || testing) return;
    setTesting(true); setTestProblem("");
    try { await api.testAIConnection(status.provider_connection_id); }
    catch (error) { setTestProblem(developerAssetError(error, t("aiReadiness.testFailed"))); }
    finally { setTesting(false); setAttempt((value) => value + 1); }
  }

  const details = <>
    <p>{t("aiReadiness.required")}</p>
    {problem && <p className="auth-problem" role="alert">{problem}</p>}
    {status && <>
      {status.blockers.length > 0 && <ul>{status.blockers.map((code) => <li key={code}>{t(`aiReadiness.blockers.${code}`)}</li>)}</ul>}
      {status.model && status.provider && <p>{t("aiReadiness.target", { provider: status.provider, model: status.model })}{status.managed_by === "environment" && <> {t("aiReadiness.deploymentManaged")}</>}</p>}
      {status.budget && <p>{status.budget.limited ? t("aiReadiness.budget", { remaining: status.budget.remaining ?? 0, total: status.budget.daily_limit, reserved: status.budget.reserved }) : t("aiReadiness.unlimited")}{status.budget.limited && <> {t("aiReadiness.resets", { date: t("format.dateTime", { value: new Date(status.budget.resets_at), formatParams: { value: { dateStyle: "medium", timeStyle: "short" } } }) })}</>}</p>}
      {status.can_process && !status.last_tested_at && <p>{t("aiReadiness.notTested")}</p>}
      {status.last_tested_at && <p>{t(status.last_test_error_code ? "aiReadiness.lastTestFailed" : "aiReadiness.lastTestPassed", { date: t("format.dateTime", { value: new Date(status.last_tested_at), formatParams: { value: { dateStyle: "medium", timeStyle: "short" } } }) })}</p>}
    </>}
    {testProblem && <p className="auth-problem" role="alert">{testProblem}</p>}
    <div className="ai-readiness-actions">
      {!inSettings && <a href={settingsURL} target={preserveForm ? "_blank" : undefined} rel={preserveForm ? "noopener noreferrer" : undefined}>{t(preserveForm ? "aiReadiness.settingsNewTab" : "aiReadiness.settings")}</a>}
      <Button outline disabled={loading || testing} onClick={() => setAttempt((value) => value + 1)}>{t("aiReadiness.refresh")}</Button>
      {!inSettings && status?.provider_connection_id && <Button outline disabled={loading || testing} onClick={() => void testConnection()}>{t(testing ? "aiReadiness.testing" : "aiReadiness.test")}</Button>}
    </div>
    <small>{t("aiReadiness.checkScope")}</small>
  </>;
  return <section className="ai-readiness-panel" aria-label={t("aiReadiness.title")} aria-busy={loading}>
    <div className="ai-readiness-heading"><h3>{t("aiReadiness.title")}</h3><Badge color={loading ? "zinc" : status?.last_test_error_code || !status?.can_process ? "amber" : status.last_tested_at ? "green" : "zinc"}>{t(loading ? "aiReadiness.checking" : !status?.can_process ? "aiReadiness.setupNeeded" : status.last_test_error_code ? "aiReadiness.checkConnection" : "aiReadiness.configured")}</Badge></div>
    {compact && status?.can_process && !status.last_test_error_code && !problem && !testProblem ? <details><summary>{t("aiReadiness.connectionDetails")}</summary>{details}</details> : details}
  </section>;
}
