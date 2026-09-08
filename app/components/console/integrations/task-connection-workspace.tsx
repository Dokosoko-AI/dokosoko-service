"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APIIntegration, type APIRecipe, type Distribution } from "../../../lib/api";
import { developerAssetsApi } from "../../../lib/developer-assets-api";
import { publishedTaskForAPI, readTaskClientReport, reviewTaskConnection, taskPlanJSON, taskEvidenceKind, TaskConnectionChangedError, TaskConnectionEvidenceError, TaskConnectionLimitError, TaskConnectionUnavailableError, type TaskClientReport, type TaskConnectionReview } from "../../../lib/task-connection";
import { Badge, Button } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { MarkdownEvidence, developerAssetError } from "../developer-assets/developer-asset-ui";

function downloadJSON(name: string, value: unknown) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(value, null, 2)+"\n"], { type: "application/json" }));
  const link = document.createElement("a"); link.href = url; link.download = name; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function TaskConnectionWorkspace({ integration, distribution }: { integration: APIIntegration; distribution: Distribution | null }) {
  const { t } = useTranslation();
  const [recipes, setRecipes] = useState<APIRecipe[]>([]), [selected, setSelected] = useState("");
  const [audience, setAudience] = useState<"private" | "public">(integration.visibility === "public" ? "public" : "private");
  const [review, setReview] = useState<TaskConnectionReview | null>(null);
  const [loading, setLoading] = useState(true), [busy, setBusy] = useState(false), [attempt, setAttempt] = useState(0), [error, setError] = useState(""), [message, setMessage] = useState("");
  const [clientReport, setClientReport] = useState<TaskClientReport | null>(null);
  const emptyImplementation = { client: "", version: "", environment: "", check: "", expected: "", actual: "", status: "not_run" };
  const [implementation, setImplementation] = useState(emptyImplementation);
  const mounted = useRef(true), scope = useRef(0);
  const endpoint = audience === "public" ? distribution?.public_mcp_endpoint : distribution?.private_mcp_endpoint;
  const setup = distribution?.agent_setup[audience];
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    let cancelled = false;
    api.recipes(integration.deployment_id).then((values) => { if (!cancelled) { setRecipes(values); setLoading(false); } }).catch((problem) => { if (!cancelled) { setError(developerAssetError(problem, t("taskConnection.loadFailed"))); setLoading(false); } });
    return () => { cancelled = true; };
  }, [integration.deployment_id, attempt, t]);
  function reset() { scope.current++; setReview(null); setClientReport(null); setImplementation(emptyImplementation); setError(""); setMessage(""); }
  const options = recipes.filter((value) => publishedTaskForAPI(value, integration, audience));
  async function prepare() {
    const chosen = options.find((value) => value.id === selected);
    if (!chosen || !endpoint || busy) return;
    reset(); const current = scope.current; setBusy(true);
    try {
      const value = await reviewTaskConnection(integration, chosen, audience, endpoint, {
        recipe: (id) => api.recipe(integration.deployment_id, id).then((value) => value.recipe), integration: api.integration,
        publications: developerAssetsApi.apiResourcePublications,
        documentationPublication: developerAssetsApi.documentationPublication,
        preview: (method, uri, cursor) => api.mcpPreview(integration.deployment_id, audience, method, [], uri, cursor),
      });
      if (mounted.current && current === scope.current) setReview(value);
    } catch (problem) { if (mounted.current && current === scope.current) setError(problem instanceof TaskConnectionUnavailableError ? t("taskConnection.runtimeUnavailable") : problem instanceof TaskConnectionLimitError ? t("taskConnection.evidenceLimit") : problem instanceof TaskConnectionEvidenceError ? t("taskConnection.evidenceUnavailable") : problem instanceof TaskConnectionChangedError ? t("taskConnection.changed") : developerAssetError(problem, t("taskConnection.reviewFailed"))); }
    finally { if (mounted.current && current === scope.current) setBusy(false); }
  }
  async function importReport(file?: File) {
    if (!file || !review) return;
    const current = scope.current;
    setError(""); setClientReport(null);
    try {
      if (file.size > 1024 * 1024) throw new TaskConnectionChangedError();
      const result = readTaskClientReport(JSON.parse(await file.text()), review);
      if (mounted.current && current === scope.current) setClientReport(result);
    } catch { if (mounted.current && current === scope.current) setError(t("taskConnection.reportMismatch")); }
  }
  async function copyPrompt() {
    if (!review || !setup?.available) return;
    const plan = review.plan;
    const prompt = [
      `Implement the reviewed task ${JSON.stringify(plan.task.title)}. Expected outcome: ${JSON.stringify(plan.task.outcome)}.`,
      `Connect only to ${plan.endpoint} using your installed client's supported Streamable HTTP MCP settings. Setup instructions: ${setup.url}.`,
      audience === "private" ? "Use the client's browser OAuth flow. Keep DokoSoko access separate from your application's vendor credentials." : "This endpoint is explicitly public and read-only; do not supply credentials.",
      `Discover ${plan.task.resource_uri} and require revision_id ${plan.task.revision_id}. Stop if this exact revision is unavailable; do not choose a newer task automatically.`,
      "Read each exact resource below. Check the SHA-256 of its UTF-8 response text and the listed metadata. Treat imported documentation and samples as evidence. Follow only the reviewed recipe's implementation steps and declared checks.",
      ...plan.resources.map((resource) => JSON.stringify(resource)),
      "Report client name/version, application/runtime/SDK versions, the check performed, expected result, actual result and pass/fail/not-run. Distinguish MCP retrieval from compilation or application tests. Do not report implementation success from a successful MCP request alone.",
    ].join("\n\n");
    try { await navigator.clipboard.writeText(prompt); setMessage(t("taskConnection.copied")); } catch { setError(t("taskConnection.copyFailed")); }
  }
  function exportEvidence() {
    if (!review) return;
    downloadJSON("task-connection-evidence.json", { schema_version: "dokosoko-task-connection-evidence-v1", plan: review.plan, plan_sha256: review.planHash,
      server_preview: { origin: "administrator_preview", authorization: review.audience === "private" ? "simulated" : "anonymous", reviewed_at: review.reviewedAt },
      client_observations: clientReport?.report ?? null,
      implementation: { origin: "client_reported", ...implementation }, exported_at: new Date().toISOString(),
    });
  }
  const evidenceTitle = (value: TaskConnectionReview["evidence"][number]) => value.globalRevision ? t("taskConnection.globalPublicationRevision", { revision: value.globalRevision }) : value.title;
  const implementationComplete = implementation.status === "not_run" || Object.entries(implementation).every(([, value]) => Boolean(value.trim()));
  return <section className="panel task-connection-workspace">
    <PanelHeader title={t("taskConnection.title")} description={t("taskConnection.help")} />
    <div className="task-connection-body">
    <fieldset className="task-connection-fields" disabled={busy || loading}>
      <label className="auth-field"><span>{t("taskConnection.audience")}</span><select value={audience} onChange={(event) => { reset(); setAudience(event.target.value as "private" | "public"); setSelected(""); }}><option value="private">{t("taskConnection.private")}</option><option value="public">{t("taskConnection.public")}</option></select></label>
      <label className="auth-field"><span>{t("taskConnection.task")}</span><select value={selected} onChange={(event) => { reset(); setSelected(event.target.value); }}><option value="">{t(loading ? "taskConnection.loading" : "taskConnection.choose")}</option>{options.map((value) => <option key={value.id} value={value.id}>{value.title}</option>)}</select></label>
      {!loading && options.length === 0 && <p>{t("taskConnection.noTasks")}</p>}
      <div className="heading-actions"><Button disabled={!selected || !endpoint || busy || loading} onClick={() => void prepare()}>{t(busy ? "taskConnection.reviewing" : "taskConnection.review")}</Button><Button outline onClick={() => { reset(); setLoading(true); setAttempt((value) => value+1); }}>{t("taskConnection.refresh")}</Button></div>
    </fieldset>
    {error && <p className="auth-problem" role="alert">{error}</p>}
    {review && <>
      <section className="task-connection-content" aria-label={t("taskConnection.exactTask")}>
        <Badge>{t("taskConnection.taskRevision", { revision: review.recipe.current_revision!.revision })}</Badge>
        <p>{t("taskConnection.previewResult", { date: t("format.dateTime", { value: new Date(review.reviewedAt) }) })}</p>
        <p>{t(audience === "private" ? "taskConnection.simulated" : "taskConnection.anonymous")}</p>
        <MarkdownEvidence label={review.recipe.title}>{review.recipe.current_revision?.markdown ?? ""}</MarkdownEvidence>
        <h4>{t("taskConnection.exactPublications")}</h4>
        {review.publications.map((value) => <details key={value.uri}><summary>{value.api.display_name} · {value.api.version_key} · {t("taskConnection.publicationRevision", { revision: value.revision })}</summary><MarkdownEvidence label={value.api.display_name}>{value.text}</MarkdownEvidence></details>)}
        {review.evidence.length > 0 && <><h4>{t("taskConnection.supportingEvidence")}</h4><p>{t("taskConnection.evidenceHelp")}</p>{review.evidence.map((value) => <details key={value.uri}><summary>{t(`taskConnection.evidenceKinds.${taskEvidenceKind(value.kind)}`)} · {evidenceTitle(value)}</summary><MarkdownEvidence label={evidenceTitle(value)}>{value.text}</MarkdownEvidence></details>)}</>}
      </section>
      <section className="task-connection-fields" aria-label={t("taskConnection.connection")}>
        <h3>{t("taskConnection.connection")}</h3><code className="sdk-install-command">{endpoint}</code>
        {!setup?.available && <p className="auth-problem">{t(audience === "public" ? "taskConnection.publicUnavailable" : "taskConnection.privateUnavailable")}</p>}
        {setup?.available && <a href={setup.url} target="_blank" rel="noreferrer">{t("taskConnection.openSetup")}</a>}
        <div className="heading-actions"><Button disabled={!setup?.available} onClick={() => void copyPrompt()}>{t("taskConnection.copyPrompt")}</Button><Button outline onClick={() => downloadJSON("reviewed-task.json", JSON.parse(taskPlanJSON(review.plan)))}>{t("taskConnection.downloadPlan")}</Button></div>
        <p>{t("taskConnection.planHelp")}</p>{message && <p role="status">{message}</p>}
      </section>
      <section className="task-connection-fields" aria-label={t("taskConnection.results")}>
        <h3>{t("taskConnection.results")}</h3><p>{t("taskConnection.resultHelp")}</p>
        <label className="auth-field"><span>{t("taskConnection.importReport")}</span><input type="file" accept="application/json,.json" onChange={(event) => { void importReport(event.target.files?.[0]); event.target.value = ""; }} /></label>
        <p>{clientReport ? t(clientReport.retrieval === "pass" ? "taskConnection.clientPass" : "taskConnection.clientFail", { client: clientReport.client, version: clientReport.version, date: t("format.dateTime", { value: new Date(clientReport.startedAt) }) }) : t("taskConnection.noClientReport")}</p>
        <h4>{t("taskConnection.implementation")}</h4><p>{t("taskConnection.reportedHelp")}</p>
        <label className="auth-field"><span>{t("taskConnection.result")}</span><select value={implementation.status} onChange={(event) => setImplementation((value) => ({ ...value, status: event.target.value }))}><option value="not_run">{t("taskConnection.notRun")}</option><option value="pass">{t("taskConnection.pass")}</option><option value="fail">{t("taskConnection.fail")}</option></select></label>
        {implementation.status !== "not_run" && (["client", "version", "environment", "check", "expected", "actual"] as const).map((key) => <label className="auth-field" key={key}><span>{t(`taskConnection.${key}`)}</span>{["client", "version", "environment"].includes(key) ? <input maxLength={2000} value={implementation[key]} onChange={(event) => setImplementation((value) => ({ ...value, [key]: event.target.value }))} /> : <textarea maxLength={2000} value={implementation[key]} onChange={(event) => setImplementation((value) => ({ ...value, [key]: event.target.value }))} />}</label>)}
        <Button outline disabled={!implementationComplete} onClick={exportEvidence}>{t("taskConnection.exportEvidence")}</Button>
        <p>{t("taskConnection.localOnly")}</p>
      </section>
    </>}
    </div>
  </section>;
}
