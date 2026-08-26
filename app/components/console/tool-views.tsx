import {
  ArrowLeft, BookOpen, CheckCircle2, ChevronRight, Copy,
  GitBranch, KeyRound, LockKeyhole, MessageSquareText, Plus, RefreshCw, Search,
  Share2, ShieldCheck, Sparkles, TerminalSquare, TriangleAlert, Wrench, XCircle,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import {
  APIAuditEvent, APIError, APIIntegration, APIIntegrationToolBinding, APIMCPConnection,
  APIResourceSet, APIRuntimeSetup, APITool, APIToolDryRun, APIToolTestAnalysis,
  APIToolTestAnalysisMessage, APIToolTestAnalysisProposal, APIToolTestRun,
  TOOL_TEST_ANALYSIS_CHAT_LIMITS, api, boundedToolTestAnalysisHistory,
  toolTestAnalysisEvidenceHash, toolTestAnalysisEvidencePreview,
} from "../../lib/api";
import {
  ConsoleRoute, IntegrationResourceTab, entityPath, integrationPath, integrationToolBuilderPath, sectionPath,
  toolBuilderPath,
} from "../../lib/console-routes";
import { versionedResponseIsCurrent } from "../../lib/tool-builder-safety";
import { Badge, Button, Dialog } from "../core/control";
import { PageHeader as PageHeading, PanelHeader } from "../core/layout";
import { ConsoleLink, EntityDetail, toolPolicy, unavailableConsoleCapability } from "./shared";

export function EntityDetailView({ route, detail, onNavigate }: { route: Extract<ConsoleRoute, { kind: "entity" }>; detail: EntityDetail | null; onNavigate: (path: string) => void }) {
  const parentPath = sectionPath(route.section);
  return <>
    <div className="entity-breadcrumb">
      <ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {route.section === "product" ? "APIs" : route.section}</ConsoleLink>
      <code>{route.path}</code>
    </div>
    {detail ? <>
      <PageHeading eyebrow={detail.eyebrow} title={detail.title} description={detail.description || undefined} />
      <section className="panel entity-detail-panel">
        <PanelHeader title="Details" action={<Badge color="violet">{route.entity}</Badge>} />
        <dl className="entity-detail-grid">{detail.fields.map((field) => <div key={field.label}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}</dl>
      </section>
    </> : <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Item unavailable</h1><p>No {route.entity.replaceAll("-", " ")} with UID <code>{route.uid}</code> is available in this deployment, or it is still loading.</p></div><ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to the directory</ConsoleLink></section>}
  </>;
}

function resourceSetIntegrations(resource: APIResourceSet, integrations: APIIntegration[]) {
  return integrations.filter((integration) => resource.integration_ids?.includes(integration.id) || integration.resources?.some((item) => item.resource_set_id === resource.id));
}

function manifestString(entry: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    if (typeof entry[key] === "string" && entry[key]) return entry[key] as string;
  }
  return "";
}

function manifestEntryTitle(entry: Record<string, unknown>, index: number) {
  return manifestString(entry, ["title", "name", "operationId", "operation_id", "path", "url", "location"]) || `Contract entry ${index + 1}`;
}

function manifestEntrySummary(entry: Record<string, unknown>) {
  const method = manifestString(entry, ["method", "http_method"]).toUpperCase();
  const location = manifestString(entry, ["path", "url", "location"]);
  const description = manifestString(entry, ["description", "summary"]);
  return [method, location, description].filter(Boolean).join(" · ") || `${Object.keys(entry).length} configured field${Object.keys(entry).length === 1 ? "" : "s"}`;
}

export function ResourceSetDetailView({ resource, integrations, onNavigate }: { resource: APIResourceSet | null; integrations: APIIntegration[]; onNavigate: (path: string) => void }) {
  if (!resource) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Resource set unavailable</h1><p>This resource set does not exist or is still loading.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;
  const owners = resourceSetIntegrations(resource, integrations);
  const resourceTab: IntegrationResourceTab = resource.kind === "api" ? "contracts" : "documentation";
  const backPath = owners.length === 1 ? integrationPath(owners[0].id, "documentation", resourceTab) : sectionPath("product");
  const backLabel = owners.length === 1 ? owners[0].display_name : "APIs";
  const revision = resource.latest_revision;
  const entries = revision?.manifest ?? [];

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={backPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {backLabel}</ConsoleLink><Badge color={resource.kind === "api" ? "violet" : "blue"}>{resource.kind === "api" ? "API contract" : "Documentation"}</Badge></div>
    <PageHeading eyebrow="Reusable resource set" title={resource.name} description={resource.description || "Reusable resource configuration shared explicitly between APIs."} action={<Badge color={resource.state === "active" ? "green" : "zinc"}>{resource.state}</Badge>} />
    <dl className="compact-metrics resource-detail-metrics">
      <div className="compact-metric"><dt>Latest revision</dt><dd><strong>r{revision?.revision ?? resource.revision}</strong><small>Immutable snapshot</small></dd></div>
      <div className="compact-metric"><dt>Contract entries</dt><dd><strong>{entries.length}</strong><small>{resource.kind === "api" ? "API definitions" : "Documentation records"}</small></dd></div>
      <div className="compact-metric"><dt>Used by APIs</dt><dd><strong>{owners.length}</strong><small>Explicit attachments</small></dd></div>
      <div className="compact-metric"><dt>Updated</dt><dd><strong>{revision?.created_at ? new Date(revision.created_at).toLocaleDateString() : "—"}</strong><small>{revision?.created_at ? new Date(revision.created_at).toLocaleTimeString() : "No revision date"}</small></dd></div>
    </dl>
    <div className="entity-workspace-grid">
      <section className="panel entity-contract-panel">
        <PanelHeader title="Contract contents" description="What this reusable revision contributes when attached to an API." action={<Badge color="zinc">r{revision?.revision ?? resource.revision}</Badge>} />
        <div className="entity-contract-list">{entries.map((entry, index) => <article key={`${manifestEntryTitle(entry, index)}:${index}`}><span className="resource-icon">{resource.kind === "api" ? <TerminalSquare /> : <BookOpen />}</span><span><strong>{manifestEntryTitle(entry, index)}</strong><small>{manifestEntrySummary(entry)}</small></span><Badge color={resource.kind === "api" ? "violet" : "blue"}>{manifestString(entry, ["kind", "type", "method"]) || resource.kind}</Badge></article>)}{entries.length === 0 && <div className="empty-row">This revision contains no contract entries.</div>}</div>
        <details className="advanced-details inline-advanced"><summary>View revision JSON</summary><pre className="entity-contract-json">{JSON.stringify(entries, null, 2)}</pre></details>
      </section>
      <aside className="entity-workspace-rail">
        <section className="panel entity-related-panel"><PanelHeader title="Used by APIs" description="Open the exact API workspace tab that attaches this set." />{owners.map((integration) => <ConsoleLink key={integration.id} path={integrationPath(integration.id, "documentation", resourceTab)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span><Badge color={integration.lifecycle === "active" ? "green" : "zinc"}>{integration.lifecycle}</Badge><ChevronRight /></ConsoleLink>)}{owners.length === 0 && <div className="empty-row">This set is not attached to an API.</div>}</section>
        <section className="panel entity-detail-panel"><PanelHeader title="Revision identity" /><dl className="entity-detail-grid compact-detail-grid"><div><dt>Resource set ID</dt><dd>{resource.id}</dd></div><div><dt>Content hash</dt><dd>{revision?.content_hash || "—"}</dd></div><div><dt>Revision ID</dt><dd>{revision?.id || "—"}</dd></div><div><dt>Attachment policy</dt><dd>Explicit only</dd></div></dl></section>
      </aside>
    </div>
  </>;
}

type ToolDetailTab = "overview" | "contract" | "execution" | "authorization" | "tests" | "usage" | "history";

const TOOL_DETAIL_TABS: Array<{ id: ToolDetailTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "contract", label: "Contract" },
  { id: "execution", label: "Execution" },
  { id: "authorization", label: "Authorization" },
  { id: "tests", label: "Tests" },
  { id: "usage", label: "Usage" },
  { id: "history", label: "History" },
];

const toolUpstreamAuthCopy: Record<NonNullable<APITool["upstream_auth"]>["type"], { label: string; description: string; credentialRequired: boolean }> = {
  delegated_oauth: { label: "Delegated OAuth", description: "During an authorized end-user execution, the caller's delegated OAuth token is forwarded to the fixed endpoint and is never stored on the tool. Administrator live tests cannot accept that user token.", credentialRequired: false },
  none: { label: "No authentication", description: "No upstream credential is added to the request.", credentialRequired: false },
  bearer: { label: "Bearer token", description: "An encrypted bearer token is injected server-side.", credentialRequired: true },
  authorization_scheme: { label: "Authorization scheme", description: "An encrypted credential is combined with the configured fixed vendor scheme server-side.", credentialRequired: true },
  api_key_header: { label: "API key header", description: "An encrypted API key is injected into the configured fixed header.", credentialRequired: true },
  api_key_query: { label: "API key query parameter", description: "An encrypted API key is injected into the configured fixed query parameter.", credentialRequired: true },
  basic: { label: "HTTP Basic", description: "An encrypted password is combined with the configured username server-side.", credentialRequired: true },
  oauth_client_credentials: { label: "OAuth client credentials", description: "An encrypted client secret is exchanged at the fixed token URL server-side.", credentialRequired: true },
  custom_header: { label: "Custom secret header", description: "An encrypted value is injected into the configured fixed header.", credentialRequired: true },
};

function toolJSON(value: unknown, fallback: string) {
  if (value === undefined) return fallback;
  return JSON.stringify(value, null, 2) ?? fallback;
}

function parseToolTestArguments(value: string): Record<string, unknown> {
  const parsed = JSON.parse(value) as unknown;
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("JSON arguments must be an object.");
  return parsed as Record<string, unknown>;
}

function validToolTestIdempotencyKey(value: string) {
  return /^[\x21-\x7E]{16,200}$/.test(value);
}

function ToolLiveTestEvidence({ run }: { run: APIToolTestRun }) {
  const succeeded = run.outcome === "success";
  return <div className={`tool-live-test-evidence ${succeeded ? "passed" : "failed"}`}>
    <div className="tool-live-test-heading" role="status" aria-live="polite"><span><strong>{succeeded ? "Live test completed" : "Live test found an issue"}</strong><small>{run.tool_name} · exact revision {run.tool_revision} · {run.method} · {run.authentication_type}</small></span><Badge color={succeeded ? "green" : "red"}>{run.outcome}</Badge></div>
    <dl className="compact-metrics tool-live-test-metrics">
      <div className="compact-metric"><dt>Phase</dt><dd><strong>{run.phase}</strong><small>{run.network_call_performed ? "Upstream called" : "Stopped before network"}</small></dd></div>
      <div className="compact-metric"><dt>HTTP status</dt><dd><strong>{run.upstream_status_code ?? "—"}</strong><small>{run.upstream_status_code ? "Sanitized upstream status" : "No upstream response"}</small></dd></div>
      <div className="compact-metric"><dt>Response size</dt><dd><strong>{run.response_bytes === undefined ? "—" : `${run.response_bytes} B`}</strong><small>Body value discarded</small></dd></div>
      <div className="compact-metric"><dt>Duration</dt><dd><strong>{run.duration_ms} ms</strong><small>Server observed</small></dd></div>
    </dl>
    <div className="private-default-note"><ShieldCheck />Only structural evidence is retained. Raw bodies, headers, field values, and credentials are never returned or displayed.</div>
    <div className="tool-test-shapes">
      <section aria-labelledby={`tool-test-request-${run.id}`}><h3 id={`tool-test-request-${run.id}`}>Request shape</h3><pre>{JSON.stringify(run.request_shape, null, 2)}</pre></section>
      <section aria-labelledby={`tool-test-response-${run.id}`}><h3 id={`tool-test-response-${run.id}`}>Response shape</h3>{run.response_shape ? <pre>{JSON.stringify(run.response_shape, null, 2)}</pre> : <p>No response shape was retained.</p>}</section>
    </div>
    <section className="tool-test-findings" aria-labelledby={`tool-test-findings-${run.id}`}><h3 id={`tool-test-findings-${run.id}`}>Findings</h3>{run.findings.length > 0 ? run.findings.map((finding, index) => <div className="publish-validation" key={`${finding.phase}:${finding.code}:${index}`}><span>{succeeded ? <ShieldCheck /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.phase} · {finding.message}{finding.instance_path ? ` · instance ${finding.instance_path}` : ""}{finding.schema_path ? ` · schema ${finding.schema_path}` : ""}</small></span></div>) : <div className="empty-row">No structural or policy findings.</div>}</section>
    <footer><code>{run.id}</code><span>Created {new Date(run.created_at).toLocaleString()} · evidence expires {new Date(run.expires_at).toLocaleString()}</span></footer>
  </div>;
}

function ToolLiveTestAnalysis({ run, tool, onOpenBuilder, onClone, onMessage }: { run: APIToolTestRun; tool: APITool; onOpenBuilder: (proposal: APIToolTestAnalysisProposal) => void; onClone: (proposal: APIToolTestAnalysisProposal) => void; onMessage: (message: string) => void }) {
  const [evidenceHash, setEvidenceHash] = useState("");
  const [hashError, setHashError] = useState("");
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentChecked, setConsentChecked] = useState(false);
  const [consentGranted, setConsentGranted] = useState(false);
  const [question, setQuestion] = useState("What does this sanitized evidence show, and should the non-secret contract change?");
  const [transcript, setTranscript] = useState<APIToolTestAnalysisMessage[]>([]);
  const [analysis, setAnalysis] = useState<APIToolTestAnalysis | null>(null);
  const [analysisError, setAnalysisError] = useState("");
  const [busy, setBusy] = useState(false);
  const [expired] = useState(() => Date.parse(run.expires_at) <= Date.now());
  const preview = toolTestAnalysisEvidencePreview(run);
  const questionBytes = useMemo(() => new TextEncoder().encode(question.trim()).byteLength, [question]);
  const questionProblem = questionBytes > TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes ? `Keep the question within ${TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes.toLocaleString()} UTF-8 bytes.` : "";

  useEffect(() => {
    let cancelled = false;
    toolTestAnalysisEvidenceHash(run).then((value) => { if (!cancelled) setEvidenceHash(value); }).catch(() => { if (!cancelled) setHashError("The server evidence binding is missing or invalid."); });
    return () => { cancelled = true; };
  }, [run]);

  async function sendAnalysis(explicitConsent = consentGranted) {
    const latestQuestion = question.trim();
    if (!explicitConsent || !evidenceHash || !latestQuestion || expired || busy) return;
    if (questionProblem) {
      setAnalysisError(questionProblem);
      onMessage(questionProblem);
      return;
    }
    setBusy(true);
    setAnalysisError("");
    try {
      const result = await api.analyseToolTestRun(tool.product_id, tool.id, run.id, {
        revision: run.tool_revision,
        evidence_hash: evidenceHash,
        consent_to_analysis_provider: true,
        question: latestQuestion,
        history: transcript,
      });
      if (result.evidence_hash !== evidenceHash || result.tool_revision !== run.tool_revision || !result.advisory) throw new Error("The Analysis response was not bound to this exact evidence and revision.");
      if (result.proposal && (result.proposal.base_tool_id !== tool.id || result.proposal.base_revision !== run.tool_revision || result.proposal.requires_clone !== (tool.state === "published"))) throw new Error("The proposed changes were not bound to this exact tool revision and review boundary.");
      setAnalysis(result);
      setTranscript((messages) => boundedToolTestAnalysisHistory([...messages, { role: "user", content: latestQuestion }, { role: "assistant", content: result.reply }]));
      setQuestion("");
      onMessage(result.proposal ? "Analysis returned a locally validated proposal for human review." : "Analysis replied from the consented sanitized evidence.");
    } catch (error) {
      const message = unavailableConsoleCapability(error) ? "Live-test analysis is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "The sanitized evidence could not be analysed.";
      setAnalysisError(message);
      onMessage(message);
    } finally { setBusy(false); }
  }

  const reviewConsent = () => {
    setConsentChecked(false);
    setConsentOpen(true);
  };
  const acceptConsentAndSend = () => {
    if (!consentChecked) return;
    setConsentGranted(true);
    setConsentOpen(false);
    void sendAnalysis(true);
  };
  const proposal = analysis?.proposal;

  return <section className="tool-test-analysis" aria-labelledby={`tool-test-analysis-${run.id}`}>
    <header><span className="settings-icon"><Sparkles /></span><span><strong id={`tool-test-analysis-${run.id}`}>Ask Analysis about this run</strong><small>Advisory only · exact revision {run.tool_revision} · evidence expires {new Date(run.expires_at).toLocaleString()}</small></span><Badge color="violet">Optional AI</Badge></header>
    <p className="tool-test-analysis-intro">Nothing is shared until you review this boundary and explicitly consent. The server durably records that consent, binds the call to the current Analysis provider, and never fails this evidence over to a backup provider. The provider can reply or suggest a complete candidate, but it cannot save, publish, clone, bind, or call anything.</p>
    <div className="tool-test-analysis-boundary">
      <section><h3>Sent after consent</h3><ul><li>Shapes containing only schema-declared property names, JSON types, and array lengths; status, timing, byte count, and bounded finding codes</li><li>Structural non-secret contract: schemas without annotations or literal enum/const values; value-free enum cardinality and const-presence markers; mappings, policy, method, timeout, and authentication type</li><li>Your latest question and bounded user/assistant history</li></ul></section>
      <section><h3>Never sent</h3><ul><li>Raw values or bodies, response content, request arguments, examples, stored descriptions, or schema annotations/literal values</li><li>Unexpected upstream property names, diagnostic paths, headers, credentials, nonces, auth configuration, or credential-presence state</li><li>Destination origin, literal path, query, evidence hash, tool/run/product IDs, actor, or request ID</li></ul></section>
    </div>
    <div className="tool-test-analysis-hash"><span>Evidence preview hash · browser/server binding only</span>{evidenceHash ? <code>{evidenceHash}</code> : <small>{hashError || "Checking server-computed SHA-256 binding…"}</small>}</div>
    <details className="tool-test-analysis-preview"><summary>Review the exact sanitized evidence preview</summary><pre>{JSON.stringify(preview, null, 2)}</pre></details>
    {expired && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>Evidence expired</strong><small>Run a new exact-revision live test before requesting provider analysis.</small></span></div>}
    {transcript.length > 0 && <div className="tool-test-analysis-transcript" aria-live="polite">{transcript.map((message, index) => <article className={message.role} key={`${message.role}:${index}`}><span>{message.role === "assistant" ? <Sparkles /> : <MessageSquareText />}</span><div><strong>{message.role === "assistant" ? "Analysis" : "You"}</strong><p>{message.content}</p></div></article>)}</div>}
    <label className="auth-field tool-test-analysis-question" htmlFor={`tool-test-analysis-question-${run.id}`}><span>{transcript.length > 0 ? "Follow-up question" : "Question for Analysis"}</span><textarea id={`tool-test-analysis-question-${run.id}`} maxLength={TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes} value={question} aria-invalid={Boolean(questionProblem)} aria-describedby={`tool-test-analysis-question-guidance-${run.id}${questionProblem ? ` tool-test-analysis-question-error-${run.id}` : ""}`} onChange={(event) => setQuestion(event.target.value)} placeholder="Ask about the retained shapes, findings, or non-secret contract…" /><small id={`tool-test-analysis-question-guidance-${run.id}`}>{questionBytes}/{TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes} UTF-8 bytes. Do not include secrets, raw values, destination URLs, nonces, or internal IDs.</small>{questionProblem && <small className="error" id={`tool-test-analysis-question-error-${run.id}`} role="alert">{questionProblem}</small>}</label>
    {consentGranted && <label className="tool-test-analysis-consent"><input type="checkbox" checked={consentGranted} onChange={(event) => setConsentGranted(event.target.checked)} /><span>I continue to consent to sending the provider projection described above and each bounded chat turn to the configured Analysis provider.</span></label>}
    {analysisError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>Analysis unavailable</strong><small>{analysisError}</small></span></div>}
    <div className="tool-test-analysis-actions"><Button outline disabled={busy || expired || !evidenceHash || !question.trim() || Boolean(questionProblem)} onClick={consentGranted ? () => { void sendAnalysis(); } : reviewConsent}>{busy ? "Analysing…" : consentGranted ? transcript.length > 0 ? "Send follow-up" : "Ask Analysis" : "Review consent & ask"}</Button><small>{consentGranted ? "Consent applies only to this browser-held conversation, exact evidence hash, and exact configured provider; no backup receives it." : "The configured provider is contacted only after the consent dialog is accepted and durably recorded."}</small></div>
    {analysis && <div className="tool-test-analysis-result">
      <div className="analysis-summary"><span className="settings-icon"><Sparkles /></span><span><strong>Advisory reply</strong><small>{analysis.reply}</small></span><Badge color={analysis.provider_outcome === "succeeded" ? "green" : "amber"}>{analysis.provider_outcome}</Badge></div>
      {analysis.findings.length > 0 && <section className="tool-test-analysis-findings"><h3>Advisory findings</h3>{analysis.findings.map((finding, index) => <div className="publish-validation" key={`${finding.code}:${index}`}><span><TriangleAlert /></span><span><strong>{finding.code}</strong><small>{finding.message}{finding.suggestion ? ` · ${finding.suggestion}` : ""}</small></span></div>)}</section>}
      {proposal && <section className="tool-test-analysis-proposal"><div className="tool-test-analysis-proposal-heading"><span><strong>Reviewable contract proposal</strong><small>Bound to tool revision {proposal.base_revision} · {proposal.changes.length} changed top-level field{proposal.changes.length === 1 ? "" : "s"} · never applied automatically</small></span><Badge color={proposal.valid ? "green" : "red"}>{proposal.valid ? "Locally valid" : "Needs review"}</Badge></div>
        {proposal.changes.length > 0 ? <ul>{proposal.changes.map((change) => <li key={change.field}><span><code>{change.field}</code>{change.security_sensitive && <Badge color="amber">Security-sensitive</Badge>}</span><small>{change.rationale || "Review this proposed field change."}</small></li>)}</ul> : <div className="empty-row">The provider returned the unchanged exact-revision contract.</div>}
        {proposal.findings.length > 0 && <div className="tool-test-analysis-proposal-findings">{proposal.findings.map((finding, index) => <div className="publish-validation" key={`${finding.code}:${index}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.message}</small></span></div>)}</div>}
        <details className="tool-test-analysis-proposed-draft"><summary>Review the complete locally validated proposal</summary><pre>{JSON.stringify(proposal.draft, null, 2)}</pre></details>
        <footer>{proposal.requires_clone || tool.state === "published" ? <><div className="private-default-note"><LockKeyhole />Published revisions are immutable. This proposal cannot be applied in place; clone the tool first, then review changes in the new draft without copying credentials.</div><Button outline onClick={() => onClone(proposal)}>Clone & review proposal</Button></> : <><div className="private-default-note"><ShieldCheck />This proposal has not changed the draft. Open the exact base revision in Builder to accept or reject each suggested field before saving.</div><Button outline onClick={() => onOpenBuilder(proposal)}>Open Builder to review</Button></>}</footer>
      </section>}
    </div>}
    <Dialog open={consentOpen} onClose={setConsentOpen} title="Send sanitized evidence to Analysis?" description="Your configured Analysis provider is an external processing boundary. Review exactly what crosses it for this run; this evidence will not fail over to a backup provider." actions={<><Button outline onClick={() => setConsentOpen(false)}>Cancel</Button><Button color="indigo" disabled={!consentChecked || !evidenceHash || !question.trim() || Boolean(questionProblem) || expired || busy} onClick={acceptConsentAndSend}>Consent & ask Analysis</Button></>}><div className="tool-test-analysis-consent-dialog"><div className="private-default-note"><ShieldCheck />The server recomputes <code>{evidenceHash || "the pending SHA-256 hash"}</code>, enforces this tool/run/revision and expiry, durably records the consented provider call, and rejects stale or changed provider bindings.</div><p>Only schema-declared property names, JSON types, array lengths, value-free literal-constraint markers, bounded metrics/finding codes, the structural non-secret contract, latest question, and bounded transcript are sent. Unexpected upstream property names, diagnostic paths, raw or literal values/bodies, headers, credentials, destinations, examples, actors, and internal IDs remain excluded.</p><label><input type="checkbox" checked={consentChecked} onChange={(event) => setConsentChecked(event.target.checked)} /><span>I explicitly consent to send this sanitized evidence and bounded conversation to the current configured Analysis provider only, with no backup-provider fallback.</span></label></div></Dialog>
  </section>;
}

export function ToolDetailView({ productID, tool, connections, integrations, auditEvents, onChanged, onReviewProposal, onMessage, onNavigate }: { productID: string; tool: APITool | null; connections: APIMCPConnection[]; integrations: APIIntegration[]; auditEvents: APIAuditEvent[]; onChanged: () => Promise<void>; onReviewProposal: (tool: APITool, proposal: APIToolTestAnalysisProposal) => void; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const initialPolicy = tool ? toolPolicy(tool) : { requiredGrants: [], confirmationRequired: false, risk: "low", idempotencyRequired: false };
  const initialRisk = initialPolicy.risk === "medium" || initialPolicy.risk === "high" || initialPolicy.risk === "critical" ? initialPolicy.risk : "low";
  const toolID = tool?.id;
  const [activeTool, setActiveTool] = useState<APITool | null>(null);
  const [detailStatus, setDetailStatus] = useState<"loading" | "ready" | "error">(toolID ? "loading" : "error");
  const [detailLoadAttempt, setDetailLoadAttempt] = useState(0);
  const [activeTab, setActiveTab] = useState<ToolDetailTab>("overview");
  const [usages, setUsages] = useState<Array<{ integration: APIIntegration; binding: APIIntegrationToolBinding }>>([]);
  const [usageStatus, setUsageStatus] = useState<"loading" | "ready" | "partial">("loading");
  const [runtimeSetup, setRuntimeSetup] = useState<APIRuntimeSetup | null>(null);
  const [busy, setBusy] = useState(false);
  const [description, setDescription] = useState(tool?.description ?? "");
  const [endpoint, setEndpoint] = useState(tool?.endpoint ?? "");
  const [method, setMethod] = useState(tool?.http_method ?? "POST");
  const [inputSchema, setInputSchema] = useState(JSON.stringify(tool?.input_schema ?? {}, null, 2));
  const [outputSchema, setOutputSchema] = useState(JSON.stringify(tool?.output_schema ?? {}, null, 2));
  const [grants, setGrants] = useState(initialPolicy.requiredGrants.join(", "));
  const [risk, setRisk] = useState<"low" | "medium" | "high" | "critical">(initialRisk);
  const [confirmationRequired, setConfirmationRequired] = useState(initialPolicy.confirmationRequired);
  const [idempotencyRequired, setIdempotencyRequired] = useState(initialPolicy.idempotencyRequired);
  const [timeout, setTimeoutValue] = useState(String(tool?.timeout_ms ?? 10000));
  const [testInput, setTestInput] = useState("{}");
  const [testResult, setTestResult] = useState<APIToolDryRun | null>(null);
  const [contractCheckBusy, setContractCheckBusy] = useState(false);
  const [contractCheckError, setContractCheckError] = useState("");
  const [validatedTestInput, setValidatedTestInput] = useState<string | null>(null);
  const [liveTestResult, setLiveTestResult] = useState<APIToolTestRun | null>(null);
  const [liveTestError, setLiveTestError] = useState("");
  const [liveTestBusy, setLiveTestBusy] = useState(false);
  const [testIdempotencyKey, setTestIdempotencyKey] = useState("");
  const [testConfirmationOpen, setTestConfirmationOpen] = useState(false);
  const [testConfirmationName, setTestConfirmationName] = useState("");
  const [testSideEffectsAcknowledged, setTestSideEffectsAcknowledged] = useState(false);
  const [pendingTestArguments, setPendingTestArguments] = useState<Record<string, unknown> | null>(null);
  const testFormVersionRef = useRef(0);
  const pendingTestVersionRef = useRef(0);
  const pendingTestIdempotencyKeyRef = useRef("");
  const [cloneOpen, setCloneOpen] = useState(false);
  const [cloneNamespace, setCloneNamespace] = useState("");
  const [cloneName, setCloneName] = useState("");
  const [cloneCredential, setCloneCredential] = useState("");
  const [pendingCloneProposal, setPendingCloneProposal] = useState<APIToolTestAnalysisProposal | null>(null);
  const [retireOpen, setRetireOpen] = useState(false);
  const cloneIdentityValid = /^[a-z][a-z0-9_]{0,63}$/.test(cloneNamespace.trim()) && /^[a-z][a-z0-9_]{0,63}$/.test(cloneName.trim());

  useEffect(() => {
    if (!toolID) return;
    let cancelled = false;
    api.tool(productID, toolID).then((value) => {
      if (cancelled) return;
      const policy = toolPolicy(value);
      setActiveTool(value);
      setDescription(value.description);
      setEndpoint(value.endpoint ?? "");
      setMethod(value.http_method);
      setInputSchema(JSON.stringify(value.input_schema, null, 2));
      setOutputSchema(JSON.stringify(value.output_schema, null, 2));
      setGrants(policy.requiredGrants.join(", "));
      setRisk(policy.risk === "medium" || policy.risk === "high" || policy.risk === "critical" ? policy.risk : "low");
      setConfirmationRequired(policy.confirmationRequired);
      setIdempotencyRequired(policy.idempotencyRequired);
      setTimeoutValue(String(value.timeout_ms));
      setTestInput("{}");
      setTestResult(null);
      setContractCheckError("");
      setValidatedTestInput(null);
      setLiveTestResult(null);
      setLiveTestError("");
      setTestIdempotencyKey("");
      setTestConfirmationOpen(false);
      setTestConfirmationName("");
      setTestSideEffectsAcknowledged(false);
      setPendingTestArguments(null);
      testFormVersionRef.current += 1;
      pendingTestVersionRef.current = 0;
      pendingTestIdempotencyKeyRef.current = "";
      setDetailStatus("ready");
    }).catch(() => {
	  if (!cancelled) setDetailStatus("error");
	});
    return () => { cancelled = true; };
  }, [productID, toolID, detailLoadAttempt]);

  useEffect(() => {
    if (!toolID) return;
    let cancelled = false;
    Promise.all(integrations.map(async (integration) => {
      try { return { integration, bindings: await api.integrationToolBindings(integration.id), failed: false }; }
      catch { return { integration, bindings: [] as APIIntegrationToolBinding[], failed: true }; }
    })).then((results) => {
      if (cancelled) return;
      setUsages(results.flatMap(({ integration, bindings }) => bindings.filter((binding) => binding.tool_id === toolID).map((binding) => ({ integration, binding }))));
      setUsageStatus(results.some((result) => result.failed) ? "partial" : "ready");
    });
    return () => { cancelled = true; };
  }, [toolID, integrations]);

  useEffect(() => {
    const integrationID = activeTool?.owner_integration_id;
    if (!integrationID || !activeTool?.runtime_service_connection_id) return;
    let cancelled = false;
    void api.integrationRuntimeSetup(integrationID).then((value) => {
      if (!cancelled) setRuntimeSetup(value);
    }).catch(() => {
      if (!cancelled) setRuntimeSetup(null);
    });
    return () => { cancelled = true; };
  }, [activeTool?.owner_integration_id, activeTool?.runtime_service_connection_id]);

  if (!toolID) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Tool unavailable</h1><p>This tool could not be found in the deployment catalog.</p></div><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink></section>;
  if (!activeTool) return <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon">{detailStatus === "loading" ? <RefreshCw /> : <TriangleAlert />}</span><div><h1>{detailStatus === "loading" ? "Loading tool" : "Tool details unavailable"}</h1><p>{detailStatus === "loading" ? "Loading the complete contract and fixed execution target…" : "The complete tool contract could not be loaded. No editing or lifecycle action is available."}</p></div>{detailStatus === "error" ? <Button outline onClick={() => { setActiveTool(null); setDetailStatus("loading"); setDetailLoadAttempt((value) => value + 1); }}>Retry</Button> : <ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink>}</section>;

  const currentTool = activeTool;
  const backendKind = activeTool.backend_kind ?? "http";
  const apiOwned = activeTool.scope === "api" && Boolean(activeTool.owner_integration_id);
  const owningIntegration = apiOwned ? integrations.find((integration) => integration.id === activeTool.owner_integration_id) : undefined;
  const runtimeConnection = activeTool.runtime_service_connection_id ? runtimeSetup?.service_connections.find((candidate) => candidate.id === activeTool.runtime_service_connection_id) : undefined;
  const runtimeRevision = runtimeConnection?.current_revisions?.find((candidate) => candidate.current && runtimeSetup?.environments.find((environment) => environment.id === candidate.environment_id)?.is_production) ?? runtimeConnection?.current_revisions?.find((candidate) => candidate.current);
  const runtimeAuthentication = runtimeRevision ? toolUpstreamAuthCopy[runtimeRevision.authentication_type] ?? toolUpstreamAuthCopy.none : null;
  const connection = activeTool.mcp_connection_id ? connections.find((item) => item.id === activeTool.mcp_connection_id) : null;
  const upstreamAuthType = activeTool.upstream_auth?.type ?? "delegated_oauth";
  const upstreamAuth = toolUpstreamAuthCopy[upstreamAuthType] ?? toolUpstreamAuthCopy.delegated_oauth;
  const credentialStatus = activeTool.credential_present ? "Stored" : upstreamAuth.credentialRequired ? "Missing" : upstreamAuthType === "delegated_oauth" ? "Caller token; not stored" : "Not required";
  const cloneCredentialLabel = upstreamAuthType === "basic" ? "Password" : upstreamAuthType === "oauth_client_credentials" ? "Client secret" : upstreamAuthType === "bearer" ? "Bearer token" : "Secret value";
  const requestMappingEntries = Object.entries(activeTool.request_mapping?.parameter_locations ?? {});
  const requestMappingSummary = requestMappingEntries.length > 0 ? `${requestMappingEntries.length} explicit parameter mapping${requestMappingEntries.length === 1 ? "" : "s"}` : `Default ${method.toUpperCase() === "GET" ? "query" : "body"} mapping`;
  const responseMappingSummary = activeTool.response_mapping?.result_path ? `Result at ${activeTool.response_mapping.result_path}` : "Entire response document";
  const currentPolicy = toolPolicy(activeTool);
  const fullToolName = `${activeTool.namespace}.${activeTool.name}`;
  const normalizedTestMethod = method.toUpperCase();
  const mutationTest = normalizedTestMethod !== "GET";
  const effectiveAuthenticationType = runtimeRevision?.authentication_type ?? upstreamAuthType;
  const tokenExchangeTest = effectiveAuthenticationType === "oauth_client_credentials";
  const delegatedOAuthLiveTest = effectiveAuthenticationType === "delegated_oauth";
  const liveTestUnsupported = backendKind !== "http";
  const contractCheckPassed = Boolean(testResult?.valid && testResult.network_call_performed === false && testResult.revision === currentTool.revision && validatedTestInput === testInput);
  const testConfirmationRequired = mutationTest || currentPolicy.confirmationRequired;
  const testIdempotencyRequired = mutationTest && currentPolicy.idempotencyRequired;
  const testIdempotencyValid = !testIdempotencyRequired || validToolTestIdempotencyKey(testIdempotencyKey);
  const liveTestLimitation = backendKind === "mcp"
      ? "Imported MCP tools must be exercised through their reviewed MCP connection and a private MCP test client."
      : backendKind === "native"
        ? "Native tools are source-managed and must be exercised through an authorized Private MCP client."
      : delegatedOAuthLiveTest
        ? "Administrator live tests cannot accept an end-user delegated OAuth token. Stage 2 is disabled here and no upstream request will be made; exercise this tool through an authenticated end-user flow."
      : mutationTest && !currentPolicy.idempotencyRequired
        ? "Mutation live tests require idempotency metadata in the stored policy. Clone or edit this contract in Builder and enable idempotency before making a real upstream call."
      : !activeTool.runtime_service_connection_id && upstreamAuth.credentialRequired && !activeTool.credential_present
          ? "Add the required encrypted upstream credential in the tool builder before making a live call."
          : !contractCheckPassed
            ? "Run a successful Contract check for these exact arguments and revision first."
            : "";
  const toolEvents = auditEvents.filter((event) => event.target_type === "tool" && event.target_id === activeTool.id).sort((left, right) => right.created_at.localeCompare(left.created_at));
  const riskColor = risk === "critical" ? "red" : risk === "high" ? "amber" : risk === "medium" ? "violet" : "zinc";
  const refreshAfterMutation = async () => { try { await onChanged(); return true; } catch { return false; } };

  async function publishToolRevision() {
    if (currentTool.state !== "draft") return;
    setBusy(true);
    try {
      const published = await api.publishTool(productID, currentTool.id, currentTool.revision);
      setActiveTool(published);
      const refreshed = await refreshAfterMutation();
      onMessage(`${published.namespace}.${published.name} published and available for API binding.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Tool could not be published."); } finally { setBusy(false); }
  }

  async function dryRunTool() {
    const requestVersion = testFormVersionRef.current;
    const inputSnapshot = testInput;
    setContractCheckBusy(true);
    setTestResult(null);
    setValidatedTestInput(null);
    setContractCheckError("");
    setLiveTestResult(null);
    setLiveTestError("");
    try {
      const argumentsObject = parseToolTestArguments(inputSnapshot);
      const result = await api.dryRunTool(productID, currentTool.id, argumentsObject);
      if (!versionedResponseIsCurrent(requestVersion, testFormVersionRef.current)) {
        onMessage("Contract-check result discarded because the visible test inputs changed while it was running.");
        return;
      }
      setTestResult(result);
      if (result.valid && !result.network_call_performed && result.revision === currentTool.revision) {
        setValidatedTestInput(inputSnapshot);
        onMessage("Contract check passed without a network call.");
        return;
      }
      setContractCheckError("The persisted contract did not pass exact-revision validation.");
      onMessage("Contract check returned a controlled failure without calling the upstream API.");
    } catch (error) {
      if (!versionedResponseIsCurrent(requestVersion, testFormVersionRef.current)) {
        onMessage("Contract-check result discarded because the visible test inputs changed while it was running.");
        return;
      }
      const message = unavailableConsoleCapability(error) ? "Contract checking is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "Contract check could not run.";
      setContractCheckError(message);
      onMessage(message);
    } finally { setContractCheckBusy(false); }
  }

  async function executeLiveToolTest(argumentsObject: Record<string, unknown>, requestVersion: number, idempotencyKey: string, confirmationNonce = "") {
    const result = await api.runToolTest(productID, currentTool.id, {
      revision: currentTool.revision,
      arguments: argumentsObject,
      ...(confirmationNonce ? { confirmation_nonce: confirmationNonce } : {}),
      ...(testIdempotencyRequired ? { idempotency_key: idempotencyKey } : {}),
    });
    if (!versionedResponseIsCurrent(requestVersion, testFormVersionRef.current)) {
      onMessage("Live-test result retained by the server but hidden here because the visible test inputs changed while it was running.");
      return false;
    }
    setLiveTestResult(result);
    onMessage(result.outcome === "success" ? "Live upstream test completed with sanitized evidence." : `Live upstream test stopped safely during ${result.phase}.`);
    return true;
  }

  async function beginLiveToolTest() {
    setLiveTestError("");
    setLiveTestResult(null);
    if (liveTestLimitation) {
      setLiveTestError(liveTestLimitation);
      return;
    }
    if (!testIdempotencyValid) {
      setLiveTestError("Enter an idempotency key containing 16–200 visible ASCII characters.");
      return;
    }
    let argumentsObject: Record<string, unknown>;
    try { argumentsObject = parseToolTestArguments(testInput); }
    catch (error) { setLiveTestError(error instanceof Error ? error.message : "JSON arguments are invalid."); return; }
    const requestVersion = testFormVersionRef.current;
    const idempotencyKey = testIdempotencyKey;
    if (testConfirmationRequired) {
      setPendingTestArguments(argumentsObject);
      pendingTestVersionRef.current = requestVersion;
      pendingTestIdempotencyKeyRef.current = idempotencyKey;
      setTestConfirmationName("");
      setTestSideEffectsAcknowledged(false);
      setTestConfirmationOpen(true);
      return;
    }
    setLiveTestBusy(true);
    try { await executeLiveToolTest(argumentsObject, requestVersion, idempotencyKey); }
    catch (error) {
      const message = unavailableConsoleCapability(error) ? "Live upstream testing is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "Live upstream test could not run.";
      setLiveTestError(message);
      onMessage(message);
    } finally { setLiveTestBusy(false); }
  }

  async function confirmAndRunLiveToolTest() {
    if (!pendingTestArguments || testConfirmationName !== fullToolName || !testSideEffectsAcknowledged || !testIdempotencyValid) return;
    const requestVersion = pendingTestVersionRef.current;
    const idempotencyKey = pendingTestIdempotencyKeyRef.current;
    if (!versionedResponseIsCurrent(requestVersion, testFormVersionRef.current)) {
      setTestConfirmationOpen(false);
      setPendingTestArguments(null);
      pendingTestVersionRef.current = 0;
      pendingTestIdempotencyKeyRef.current = "";
      setLiveTestError("The visible test inputs changed. Run a new Contract check before requesting confirmation again.");
      return;
    }
    setLiveTestBusy(true);
    setLiveTestError("");
    setLiveTestResult(null);
    try {
      const confirmation = await api.createToolTestConfirmation(productID, currentTool.id, {
        revision: currentTool.revision,
        arguments: pendingTestArguments,
        typed_tool_name: testConfirmationName,
        acknowledge_side_effects: testSideEffectsAcknowledged,
      });
      if (confirmation.tool_id !== currentTool.id || confirmation.tool_revision !== currentTool.revision) throw new Error("The server did not bind confirmation to this exact tool revision.");
      await executeLiveToolTest(pendingTestArguments, requestVersion, idempotencyKey, confirmation.confirmation_nonce);
      setTestConfirmationOpen(false);
      setPendingTestArguments(null);
      pendingTestVersionRef.current = 0;
      pendingTestIdempotencyKeyRef.current = "";
    } catch (error) {
      const message = unavailableConsoleCapability(error) ? "Live upstream testing is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "Live upstream test could not run.";
      setLiveTestError(message);
      setTestConfirmationOpen(false);
      setPendingTestArguments(null);
      pendingTestVersionRef.current = 0;
      pendingTestIdempotencyKeyRef.current = "";
      onMessage(message);
    } finally { setLiveTestBusy(false); }
  }

  function handleToolTabKeyDown(event: ReactKeyboardEvent<HTMLButtonElement>) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const currentIndex = TOOL_DETAIL_TABS.findIndex((tab) => `tool-tab-${tab.id}` === document.activeElement?.id);
    if (currentIndex < 0) return;
    event.preventDefault();
    const nextIndex = event.key === "Home" ? 0 : event.key === "End" ? TOOL_DETAIL_TABS.length - 1 : event.key === "ArrowRight" ? (currentIndex + 1) % TOOL_DETAIL_TABS.length : (currentIndex - 1 + TOOL_DETAIL_TABS.length) % TOOL_DETAIL_TABS.length;
    const nextTab = TOOL_DETAIL_TABS[nextIndex];
    setActiveTab(nextTab.id);
    requestAnimationFrame(() => document.getElementById(`tool-tab-${nextTab.id}`)?.focus());
  }

  function openCloneTool(proposal: APIToolTestAnalysisProposal | null = null) {
	if ((currentTool.backend_kind ?? "http") !== "http") {
	  onMessage("Source-managed and imported tools cannot be cloned.");
	  return;
	}
    const suffix = "_next";
    setCloneNamespace(currentTool.namespace);
    setCloneName(`${currentTool.name.slice(0, Math.max(1, 64 - suffix.length))}${suffix}`);
    setCloneCredential("");
    setPendingCloneProposal(proposal);
    setCloneOpen(true);
  }

  async function cloneTool() {
    setBusy(true);
    try {
      const cloned = await api.cloneTool(productID, currentTool.id, currentTool.revision, cloneNamespace.trim(), cloneName.trim(), cloneCredential);
      const refreshed = await refreshAfterMutation();
      const proposalToReview = pendingCloneProposal;
      setCloneOpen(false);
      setCloneCredential("");
      setPendingCloneProposal(null);
      onMessage(`${cloned.namespace}.${cloned.name} created as an independent draft.${proposalToReview ? " The live-test proposal is ready for per-field review in Builder." : ""}${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
      if (proposalToReview) onReviewProposal(cloned, proposalToReview);
      else onNavigate(entityPath("tool", cloned.id));
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool cloning is not enabled by this service version yet." : error instanceof APIError ? error.message : "Tool could not be cloned."); } finally { setBusy(false); }
  }

  async function retireTool() {
    setBusy(true);
    try {
      const retired = await api.retireTool(productID, currentTool.id, currentTool.revision);
      setActiveTool(retired);
      const refreshed = await refreshAfterMutation();
      setRetireOpen(false);
      onMessage(`Tool retired. Existing exact API bindings are now unresolved and must be removed before publication.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool retirement is not enabled by this service version yet." : error instanceof APIError ? error.message : "Tool could not be retired."); } finally { setBusy(false); }
  }

  const readiness = [
    { label: "Agent contract", ready: Boolean(activeTool.description && Object.keys(activeTool.input_schema).length && Object.keys(activeTool.output_schema).length) },
    { label: backendKind === "native" ? "Pinned plugin source" : activeTool.runtime_service_connection_id ? "API service connection" : "Fixed execution target", ready: backendKind === "native" ? Boolean(activeTool.native_plugin_id && activeTool.native_contract_hash) : backendKind === "mcp" ? Boolean(connection && activeTool.upstream_tool_name) : activeTool.runtime_service_connection_id ? Boolean(activeTool.http_path && runtimeConnection) : Boolean(activeTool.endpoint) },
    { label: "Safety policy", ready: ["low", "medium", "high", "critical"].includes(currentPolicy.risk ?? "low") },
    { label: "Published for managed binding", ready: activeTool.state === "published" && !activeTool.upstream_drifted },
  ];

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={owningIntegration ? integrationPath(owningIntegration.id, "tools") : sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{owningIntegration ? `${owningIntegration.display_name} tools` : "Common tools"}</ConsoleLink><Badge color={apiOwned || activeTool.backend_kind !== "http" ? "violet" : "zinc"}>{apiOwned ? "API scoped" : activeTool.backend_kind === "native" ? "Native plugin" : activeTool.backend_kind === "mcp" ? "MCP" : "Common HTTP"}</Badge></div>
    <PageHeading eyebrow={owningIntegration ? `${owningIntegration.display_name} API tool` : "Common deployment tool"} title={`${activeTool.namespace}.${activeTool.name}`} action={<span className="heading-actions">{activeTool.state === "draft" && <>{activeTool.backend_kind === "http" ? <Button outline disabled={busy} onClick={() => onNavigate(toolBuilderPath(activeTool.id))}><Wrench data-slot="icon" />Edit in builder</Button> : activeTool.backend_kind === "mcp" && connection && <ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-back-link">Review connection</ConsoleLink>}<Button color="indigo" disabled={busy || activeTool.upstream_drifted} onClick={publishToolRevision}>Publish tool</Button></>}{activeTool.state === "published" && <>{activeTool.backend_kind === "http" && (owningIntegration ? <Button outline disabled={busy} onClick={() => onNavigate(integrationToolBuilderPath(owningIntegration.id))}><Plus data-slot="icon" />Create another API tool</Button> : <Button outline disabled={busy} onClick={() => openCloneTool()}><Copy data-slot="icon" />Clone as new tool</Button>)}{activeTool.backend_kind === "mcp" && connection && <ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-back-link">Review connection</ConsoleLink>}<Button outline disabled={busy} onClick={() => setRetireOpen(true)}>Retire</Button></>}{activeTool.state === "retired" && <Badge color="zinc">Retired</Badge>}</span>} />
    <div className="page-tabs" role="tablist" aria-label="Tool sections">{TOOL_DETAIL_TABS.map((tab) => <button type="button" role="tab" id={`tool-tab-${tab.id}`} aria-controls={`tool-panel-${tab.id}`} aria-selected={activeTab === tab.id} tabIndex={activeTab === tab.id ? 0 : -1} key={tab.id} className={`page-tab ${activeTab === tab.id ? "active" : ""}`} onKeyDown={handleToolTabKeyDown} onClick={() => setActiveTab(tab.id)}>{tab.label}</button>)}</div>

    {activeTab === "overview" && <div className="tool-detail-section" role="tabpanel" id="tool-panel-overview" aria-labelledby="tool-tab-overview" tabIndex={0}>
      <dl className="compact-metrics tool-detail-metrics"><div className="compact-metric"><dt>State</dt><dd><strong>{activeTool.state}</strong><small>revision {activeTool.revision}</small></dd></div><div className="compact-metric"><dt>Backend</dt><dd><strong>{activeTool.backend_kind === "native" ? "Native" : activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"}</strong><small>{activeTool.backend_kind === "native" ? `${activeTool.native_plugin_id}@${activeTool.native_plugin_version}` : activeTool.backend_kind === "mcp" ? activeTool.upstream_tool_name || "Upstream tool" : `${activeTool.http_method} request`}</small></dd></div><div className="compact-metric"><dt>Risk</dt><dd><strong>{currentPolicy.risk ?? "low"}</strong><small>{currentPolicy.confirmationRequired ? "Confirmation required" : "No confirmation"}</small></dd></div><div className="compact-metric"><dt>Current config</dt><dd><strong>{usageStatus === "loading" ? "…" : usages.length}</strong><small>API binding{usages.length === 1 ? "" : "s"}</small></dd></div></dl>
      <div className="entity-workspace-grid"><section className="panel"><PanelHeader title="Readiness" description={apiOwned ? "This definition remains owned by one API and inherits its environment-specific execution boundary." : "A published common tool becomes eligible for a managed API to attach; publication alone does not select it for that API."} />{readiness.map((item) => <div className="integration-health-check" key={item.label}><span className={`health-icon ${item.ready ? "ready" : ""}`}>{item.ready ? <CheckCircle2 /> : <XCircle />}</span><span><strong>{item.label}</strong><small>{item.ready ? "Ready" : "Action required"}</small></span><Badge color={item.ready ? "green" : "amber"}>{item.ready ? "Ready" : "Review"}</Badge></div>)}</section><aside className="entity-workspace-rail"><section className="panel entity-policy-panel"><PanelHeader title="Delivery boundary" /><div className="entity-policy-check"><span className="ready"><ShieldCheck /></span><span><strong>Private MCP</strong><small>{activeTool.state === "published" ? "Managed API discovery requires an exact tool and authorization binding." : "Publish before managed APIs can bind this tool."}</small></span></div></section><section className="panel entity-detail-panel"><PanelHeader title="Identity" /><dl className="entity-detail-grid compact-detail-grid"><div><dt>Tool ID</dt><dd>{activeTool.id}</dd></div><div><dt>Scope</dt><dd>{owningIntegration?.display_name ?? "Common"}</dd></div><div><dt>Revision</dt><dd>{activeTool.revision}</dd></div><div><dt>Drift</dt><dd>{activeTool.upstream_drifted ? "Detected" : "None"}</dd></div><div><dt>Lifecycle</dt><dd>{activeTool.state}</dd></div></dl></section></aside></div>
    </div>}

    {activeTab === "contract" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-contract" aria-labelledby="tool-tab-contract" tabIndex={0}><PanelHeader title="Agent contract" description="Read-only exact revision. Use the Tool Builder to change an HTTP draft; published tools remain immutable." /><label className="auth-field"><span>Purpose</span><textarea readOnly value={description} /></label><div className="two-fields tool-schema-fields"><label className="auth-field"><span>Input JSON Schema</span><textarea spellCheck={false} readOnly value={inputSchema} /></label><label className="auth-field"><span>Output JSON Schema</span><textarea spellCheck={false} readOnly value={outputSchema} /></label></div></section>}

    {activeTab === "execution" && <div className="entity-workspace-grid" role="tabpanel" id="tool-panel-execution" aria-labelledby="tool-tab-execution" tabIndex={0}>
      <section className="panel tool-editor-page">
        <PanelHeader title="Execution" description="The destination, authentication mode, and request mappings are fixed before publication and cannot be supplied by an agent." />
        <div className="two-fields"><label className="auth-field"><span>Backend</span><input value={activeTool.backend_kind === "native" ? "Native plugin" : activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"} readOnly /></label><label className="auth-field"><span>Timeout (ms)</span><input readOnly type="number" value={timeout} /></label></div>
        {activeTool.backend_kind === "http" ? activeTool.runtime_service_connection_id ? <>
          <div className="two-fields"><label className="auth-field"><span>Method</span><input value={method} readOnly /></label><label className="auth-field"><span>Relative path</span><input readOnly value={activeTool.http_path ?? ""} /></label></div>
          <div className="private-default-note"><LockKeyhole />The service host, authentication, and encrypted credential are inherited from this API&apos;s Access configuration. This tool stores no independent destination or secret.</div>
          <dl className="entity-detail-grid compact-detail-grid">
            <div><dt>Service connection</dt><dd>{runtimeConnection?.name ?? "Loading saved connection…"}</dd></div>
            <div><dt>Connection ID</dt><dd>{activeTool.runtime_service_connection_id}</dd></div>
            <div><dt>Authentication</dt><dd>{runtimeAuthentication?.label ?? "Inherited from API"}</dd></div>
            <div><dt>Request mapping</dt><dd>{requestMappingSummary}</dd></div>
            <div><dt>Response mapping</dt><dd>{responseMappingSummary}</dd></div>
          </dl>
          <details className="advanced-details inline-advanced"><summary>Mappings and examples</summary><div className="two-fields tool-schema-fields">
            <label className="auth-field"><span>Request mapping</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_mapping ?? { parameter_locations: {} }, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Response mapping</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_mapping ?? {}, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Request example</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_example, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Response example</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_example, "Not configured")} spellCheck={false} /></label>
          </div></details>
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>Method</span><input value={method} readOnly /></label><label className="auth-field"><span>Fixed endpoint</span><input readOnly type="url" value={endpoint} /></label></div>
          <div className="private-default-note"><LockKeyhole />{upstreamAuth.description} Agents cannot read stored credentials or change the configured destination.</div>
          <dl className="entity-detail-grid compact-detail-grid">
            <div><dt>Upstream authentication</dt><dd>{upstreamAuth.label}</dd></div>
            <div><dt>Credential</dt><dd>{credentialStatus}</dd></div>
            <div><dt>Request mapping</dt><dd>{requestMappingSummary}</dd></div>
            <div><dt>Response mapping</dt><dd>{responseMappingSummary}</dd></div>
          </dl>
          <details className="advanced-details inline-advanced"><summary>Authentication, mappings, and examples</summary><div className="two-fields tool-schema-fields">
            <label className="auth-field"><span>Upstream authentication</span><textarea className="code-input" readOnly value={toolJSON(activeTool.upstream_auth ?? { type: upstreamAuthType }, "Not configured")} spellCheck={false} /><small>Non-secret configuration only. Stored credential material is never returned.</small></label>
            <label className="auth-field"><span>Request mapping</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_mapping ?? { parameter_locations: {} }, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Response mapping</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_mapping ?? {}, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Request example</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_example, "Not configured")} spellCheck={false} /></label>
            <label className="auth-field"><span>Response example</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_example, "Not configured")} spellCheck={false} /></label>
          </div></details>
        </> : activeTool.backend_kind === "native" ? <><dl className="entity-detail-grid"><div><dt>Plugin</dt><dd>{activeTool.native_plugin_id}</dd></div><div><dt>Plugin version</dt><dd>{activeTool.native_plugin_version}</dd></div><div><dt>SDK version</dt><dd>{activeTool.native_sdk_version}</dd></div><div><dt>Tool ID</dt><dd>{activeTool.native_tool_id}</dd></div><div><dt>Identity</dt><dd>{activeTool.identity_requirement}</dd></div><div><dt>State scope</dt><dd>{activeTool.state_scope}</dd></div><div><dt>Effect</dt><dd>{activeTool.effect}</dd></div><div><dt>Idempotency</dt><dd>{activeTool.idempotency_mode}</dd></div></dl><div className="private-default-note"><ShieldCheck />This contract is source-managed. Execution requires the active plugin instance and exact manifest and tool hashes pinned by this revision.</div></> : <dl className="entity-detail-grid"><div><dt>Upstream tool</dt><dd>{activeTool.upstream_tool_name}</dd></div><div><dt>Schema hash</dt><dd>{activeTool.upstream_schema_hash}</dd></div></dl>}
      </section>
      <aside className="entity-workspace-rail">{connection ? <section className="panel entity-related-panel"><PanelHeader title="Connection" /><ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><Share2 /></span><span><strong>{connection.name}</strong><small>{connection.protocol_version} · {connection.auth_mode}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><ChevronRight /></ConsoleLink></section> : activeTool.runtime_service_connection_id && owningIntegration ? <section className="panel entity-related-panel"><PanelHeader title="API service access" /><ConsoleLink path={integrationPath(owningIntegration.id, "access")} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><KeyRound /></span><span><strong>{runtimeConnection?.name ?? "Service connection"}</strong><small>Endpoint and credential managed in Access</small></span><ChevronRight /></ConsoleLink></section> : <section className="panel entity-detail-panel"><PanelHeader title={activeTool.backend_kind === "native" ? "Trusted source boundary" : activeTool.backend_kind === "mcp" ? "Connection model" : "HTTP security boundary"} /><p className="entity-panel-copy">{activeTool.backend_kind === "native" ? "This tool executes trusted source compiled into DokoSoko and receives only its declared host services." : activeTool.backend_kind === "mcp" ? "This imported tool uses its reviewed MCP connection." : `${upstreamAuth.label} is applied server-side at execution time. Tool responses expose only whether a required encrypted credential is present.`}</p></section>}</aside>
    </div>}

    {activeTab === "authorization" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-authorization" aria-labelledby="tool-tab-authorization" tabIndex={0}><PanelHeader title="Baseline authorization" description="Read-only exact revision. An API authorization point may add stricter requirements but cannot weaken this policy." action={<Badge color={riskColor}>{risk} risk</Badge>} /><label className="auth-field"><span>Required registered grants</span><input readOnly value={grants} placeholder="No registered grants" /></label><div className="two-fields"><label className="auth-field"><span>Risk</span><input value={risk} readOnly /></label></div><dl className="entity-detail-grid compact-detail-grid readonly-policy"><div><dt>Explicit confirmation</dt><dd>{confirmationRequired || risk === "critical" ? "Required" : "Not required"}</dd></div><div><dt>Idempotency metadata</dt><dd>{idempotencyRequired ? "Required" : "Not required"}</dd></div></dl>{currentPolicy.requiredGrants.length > 0 && <div className="entity-grant-list">{currentPolicy.requiredGrants.map((grant) => <code key={grant}>{grant}</code>)}</div>}</section>}

    {activeTab === "tests" && <div className="tool-tests-workspace" role="tabpanel" id="tool-panel-tests" aria-labelledby="tool-tab-tests" tabIndex={0}>
      <section className="panel tool-editor-page tool-test-stage">
        <PanelHeader title="Contract check" description="Stage 1 · Validate the arguments, schema, fixed destination, and policy for this exact persisted revision. No network call is made." action={<Button outline disabled={busy || contractCheckBusy || liveTestBusy} onClick={dryRunTool}>{contractCheckBusy ? "Checking…" : "Run Contract check"}</Button>} />
        <label className="auth-field" htmlFor="tool-test-arguments"><span>JSON arguments</span><textarea id="tool-test-arguments" className="code-input" spellCheck={false} value={testInput} disabled={contractCheckBusy || liveTestBusy || testConfirmationOpen} onChange={(event) => { testFormVersionRef.current += 1; setTestInput(event.target.value); setTestResult(null); setValidatedTestInput(null); setContractCheckError(""); setLiveTestResult(null); setLiveTestError(""); }} /><small>Changing these arguments invalidates the Contract check and any prior live-test evidence.</small></label>
        {contractCheckError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>Contract check did not pass</strong><small>{contractCheckError}</small></span></div>}
        {testResult && <pre role="status" aria-live="polite" className={`tool-test-result ${contractCheckPassed ? "passed" : "failed"}`}>{JSON.stringify(testResult, null, 2)}</pre>}
      </section>

      <section className="panel tool-editor-page tool-test-stage" aria-busy={liveTestBusy}>
        <PanelHeader title="Live upstream test" description={delegatedOAuthLiveTest ? "Stage 2 · Unavailable for Delegated OAuth. Administrator live tests do not accept an end-user token, and no upstream request will be made." : `Stage 2 · ${testConfirmationRequired ? mutationTest ? "Review side effects, confirm the exact revision, then call" : "Review the explicit policy confirmation, then call" : "Call"} the fixed upstream endpoint only after the Contract check passes${tokenExchangeTest ? "; client-credentials authentication may first call its fixed token endpoint" : ""}.`} action={!liveTestUnsupported && <Button color="indigo" disabled={busy || contractCheckBusy || liveTestBusy || Boolean(liveTestLimitation) || !testIdempotencyValid} onClick={beginLiveToolTest}>{liveTestBusy ? "Running live test…" : delegatedOAuthLiveTest ? "Live test unavailable" : testConfirmationRequired ? "Review & run live test" : "Run live upstream test"}</Button>} />
        {liveTestLimitation && <div className="capability-unavailable"><TriangleAlert /><span><strong>Live test unavailable</strong><small>{liveTestLimitation}</small></span></div>}
        {testIdempotencyRequired && !liveTestUnsupported && !delegatedOAuthLiveTest && <label className="auth-field" htmlFor="tool-test-idempotency-key"><span>Idempotency key</span><input id="tool-test-idempotency-key" autoComplete="off" minLength={16} maxLength={200} disabled={liveTestBusy || testConfirmationOpen} aria-invalid={Boolean(testIdempotencyKey) && !testIdempotencyValid} aria-describedby="tool-test-idempotency-guidance" value={testIdempotencyKey} onChange={(event) => { testFormVersionRef.current += 1; setTestIdempotencyKey(event.target.value); setLiveTestResult(null); setLiveTestError(""); }} /><small id="tool-test-idempotency-guidance">Required for every mutation live test. Use 16–200 visible ASCII characters; the value is forwarded through the server&apos;s idempotency boundary and is not included in retained evidence.</small></label>}
        {liveTestError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>Live upstream test could not complete</strong><small>{liveTestError}</small></span></div>}
        {!liveTestResult && !liveTestError && !liveTestLimitation && <div className="private-default-note"><ShieldCheck />The server retains only status, timing, byte count, structural shapes, and sanitized findings. It discards raw request and response bodies, headers, scalar values, and credentials.</div>}
        {liveTestResult && <><ToolLiveTestEvidence run={liveTestResult} /><ToolLiveTestAnalysis key={liveTestResult.id} run={liveTestResult} tool={activeTool} onOpenBuilder={(proposal) => onReviewProposal(activeTool, proposal)} onClone={(proposal) => openCloneTool(proposal)} onMessage={onMessage} /></>}
      </section>
    </div>}

    {activeTab === "usage" && <section className="panel" role="tabpanel" id="tool-panel-usage" aria-labelledby="tool-tab-usage" tabIndex={0}><PanelHeader title="Current API configuration" description="Each current API draft pins an exact published tool revision and one exact authorization-point revision. Published snapshots are not counted here." action={<Badge color="violet">{usageStatus === "loading" ? "…" : usages.length}</Badge>} />{usageStatus === "partial" && <div className="capability-unavailable"><TriangleAlert /><span><strong>Some API bindings could not be loaded.</strong><small>The list below may be incomplete.</small></span></div>}{usages.map(({ integration, binding }) => { const point = binding.authorization_point; const current = binding.tool_revision === activeTool.revision && activeTool.state === "published" && !activeTool.upstream_drifted && Boolean(point && point.state === "active" && point.revision === binding.authorization_point_revision); return <ConsoleLink key={`${integration.id}:${binding.tool_revision}`} path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key} · tool r{binding.tool_revision} · authorization r{binding.authorization_point_revision}</small></span><Badge color={current ? "green" : "red"}>{current ? "Current" : "Stale / unresolved"}</Badge><ChevronRight /></ConsoleLink>; })}{usageStatus === "loading" && <div className="empty-row">Loading current API bindings…</div>}{usageStatus === "ready" && usages.length === 0 && <div className="empty-row">No current API configuration binds this tool.</div>}</section>}

    {activeTab === "history" && <section className="panel" role="tabpanel" id="tool-panel-history" aria-labelledby="tool-tab-history" tabIndex={0}><PanelHeader title="Tool activity" description="Append-only administrative and execution events loaded for this tool." action={<ConsoleLink path={sectionPath("reporting")} onNavigate={onNavigate} className="entity-back-link">Open audit</ConsoleLink>} />{toolEvents.map((event) => <div className="lease-row" key={event.id}><span><strong>{event.action}</strong><small>{event.actor_id || "system"} · {event.request_id || "no request ID"}</small></span><time>{new Date(event.created_at).toLocaleString()}</time></div>)}{toolEvents.length === 0 && <div className="empty-row">No loaded activity for this tool.</div>}</section>}

    {testConfirmationRequired && !liveTestUnsupported && !delegatedOAuthLiveTest && <Dialog open={testConfirmationOpen} onClose={(open) => { if (liveTestBusy) return; setTestConfirmationOpen(open); if (!open) { setPendingTestArguments(null); pendingTestVersionRef.current = 0; pendingTestIdempotencyKeyRef.current = ""; setTestConfirmationName(""); setTestSideEffectsAcknowledged(false); } }} title={`Confirm live ${normalizedTestMethod} test`} description={mutationTest ? `This will make a real ${normalizedTestMethod} request for ${fullToolName} revision ${currentTool.revision}${tokenExchangeTest ? " after a client-credentials token exchange when no cached token is available" : ""}. It may create, change, or delete upstream data.` : `This will make a real ${normalizedTestMethod} request for ${fullToolName} revision ${currentTool.revision}${tokenExchangeTest ? " after a client-credentials token exchange when no cached token is available" : ""}. Its stored policy requires explicit confirmation even for this read.`} actions={<><Button outline disabled={liveTestBusy} onClick={() => { setTestConfirmationOpen(false); setPendingTestArguments(null); pendingTestVersionRef.current = 0; pendingTestIdempotencyKeyRef.current = ""; setTestConfirmationName(""); setTestSideEffectsAcknowledged(false); }}>Cancel</Button><Button color="red" disabled={liveTestBusy || !pendingTestArguments || testConfirmationName !== fullToolName || !testSideEffectsAcknowledged || !testIdempotencyValid} onClick={confirmAndRunLiveToolTest}>{liveTestBusy ? "Confirming & running…" : "Confirm & run now"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="capability-unavailable"><TriangleAlert /><span><strong>This is not a simulation.</strong><small>{mutationTest ? `The fixed action endpoint will receive one real request using its configured server-side authentication${tokenExchangeTest ? "; the fixed token endpoint may also receive one client-credentials exchange" : ""}.` : `The policy requires you to confirm this real read request to the fixed upstream endpoint${tokenExchangeTest ? "; the fixed token endpoint may also receive one client-credentials exchange" : ""}.`}</small></span></div>
        <label className="auth-field" htmlFor="tool-test-confirm-name"><span>Type the full tool name</span><input id="tool-test-confirm-name" autoComplete="off" aria-invalid={Boolean(testConfirmationName) && testConfirmationName !== fullToolName} aria-describedby="tool-test-confirm-name-guidance" value={testConfirmationName} onChange={(event) => setTestConfirmationName(event.target.value)} /><small id="tool-test-confirm-name-guidance">Type <code>{fullToolName}</code> exactly to confirm revision {currentTool.revision}.</small></label>
        <label className="compact-check"><input type="checkbox" checked={testSideEffectsAcknowledged} onChange={(event) => setTestSideEffectsAcknowledged(event.target.checked)} /><span>{mutationTest ? `I understand this test can cause real upstream side effects${tokenExchangeTest ? " and may perform a real token exchange" : ""}.` : `I understand this test sends a real upstream request under the stored confirmation policy${tokenExchangeTest ? " and may perform a real token exchange" : ""}.`}</span></label>
        <div className="private-default-note"><LockKeyhole />Confirmation creates a short-lived, single-use nonce bound to this exact revision and arguments. DokoSoko uses it immediately and never exposes it in the evidence.</div>
      </div>
    </Dialog>}
    {backendKind === "http" && !apiOwned && <Dialog open={cloneOpen} onClose={(open) => { setCloneOpen(open); if (!open) { setCloneCredential(""); setPendingCloneProposal(null); } }} title="Clone as a new tool" description={pendingCloneProposal ? "Choose a distinct lower-case identity. The independent draft will open in Builder with the live-test proposal ready for per-field review; nothing is applied automatically." : "Choose a distinct lower-case identity. Stored credentials are never copied into the independent draft."} actions={<><Button outline onClick={() => { setCloneOpen(false); setCloneCredential(""); setPendingCloneProposal(null); }}>Cancel</Button><Button color="indigo" disabled={busy || !cloneIdentityValid || (upstreamAuth.credentialRequired && !cloneCredential)} onClick={cloneTool}>{busy ? "Cloning…" : pendingCloneProposal ? "Create draft & review" : "Create draft"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Namespace</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneNamespace) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneNamespace.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneNamespace} onChange={(event) => setCloneNamespace(event.target.value)} /></label><label className="auth-field"><span>Name</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneName) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneName.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneName} onChange={(event) => setCloneName(event.target.value)} /></label></div><small id="clone-tool-identity-guidance">Use 1–64 lower-case letters, numbers or underscores, starting with a letter.</small>{upstreamAuth.credentialRequired && <label className="auth-field"><span>{cloneCredentialLabel}</span><input type="password" autoComplete="new-password" value={cloneCredential} onChange={(event) => setCloneCredential(event.target.value)} /><small>Required for {upstreamAuth.label}. Enter a new value because the source credential is never copied.</small></label>}<div className="private-default-note"><KeyRound />The clone receives the non-secret contract only. Delegated OAuth and unauthenticated tools do not require a stored credential. {pendingCloneProposal ? "The proposal remains an in-memory review seed and is not saved with the clone." : ""}</div></div></Dialog>}
    <Dialog open={retireOpen} onClose={setRetireOpen} title={`Retire ${activeTool.namespace}.${activeTool.name}?`} description="Retirement removes the tool from discovery and prevents new bindings. API drafts using it must remove their binding before publication." actions={<><Button outline onClick={() => setRetireOpen(false)}>Cancel</Button><Button color="red" disabled={busy} onClick={retireTool}>{busy ? "Retiring…" : "Retire tool"}</Button></>}><div className="private-default-note"><TriangleAlert />This changes the deployment catalogue immediately. Existing published API snapshots remain audit evidence.</div></Dialog>
  </>;
}

export function ConsoleNotFoundView({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">Navigation</p><h1>Page not found</h1><p><code>{path}</code> is not a recognised console URL.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;
}
