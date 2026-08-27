import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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
  const { t } = useTranslation();
  const parentPath = sectionPath(route.section);
  return <>
    <div className="entity-breadcrumb">
      <ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.backTo")} {route.section === "product" ? t("navigation.apis") : route.section}</ConsoleLink>
      <code>{route.path}</code>
    </div>
    {detail ? <>
      <PageHeading eyebrow={detail.eyebrow} title={detail.title} description={detail.description || undefined} />
      <section className="panel entity-detail-panel">
        <PanelHeader title={t("tools.details")} action={<Badge color="violet">{route.entity}</Badge>} />
        <dl className="entity-detail-grid">{detail.fields.map((field) => <div key={field.label}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}</dl>
      </section>
    </> : <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>{t("tools.itemUnavailable")}</h1><p>{t("tools.no")} {route.entity.replaceAll("-", " ")} {t("tools.withUID")} <code>{route.uid}</code> {t("tools.isAvailableInThisDeploymentOrItIsStill")}</p></div><ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.returnToTheDirectory")}</ConsoleLink></section>}
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

function manifestEntryTitle(entry: Record<string, unknown>, index: number, t: TFunction) {
  return manifestString(entry, ["title", "name", "operationId", "operation_id", "path", "url", "location"]) || t("tools.contractEntry", { index: index + 1 });
}

function manifestEntrySummary(entry: Record<string, unknown>, t: TFunction) {
  const method = manifestString(entry, ["method", "http_method"]).toUpperCase();
  const location = manifestString(entry, ["path", "url", "location"]);
  const description = manifestString(entry, ["description", "summary"]);
  return [method, location, description].filter(Boolean).join(" · ") || t("tools.configuredFields", { count: Object.keys(entry).length });
}

export function ResourceSetDetailView({ resource, integrations, onNavigate }: { resource: APIResourceSet | null; integrations: APIIntegration[]; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  if (!resource) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>{t("tools.resourceSetUnavailable")}</h1><p>{t("tools.thisResourceSetDoesNotExistOrIsStill")}</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.returnToAPIs")}</ConsoleLink></section>;
  const owners = resourceSetIntegrations(resource, integrations);
  const resourceTab: IntegrationResourceTab = resource.kind === "api" ? "contracts" : "documentation";
  const backPath = owners.length === 1 ? integrationPath(owners[0].id, "documentation", resourceTab) : sectionPath("product");
  const backLabel = owners.length === 1 ? owners[0].display_name : t("navigation.apis");
  const revision = resource.latest_revision;
  const entries = revision?.manifest ?? [];

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={backPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.backTo")} {backLabel}</ConsoleLink><Badge color={resource.kind === "api" ? "violet" : "blue"}>{resource.kind === "api" ? t("tools.apiContract") : t("tools.documentation")}</Badge></div>
    <PageHeading eyebrow={t("tools.reusableResourceSet")} title={resource.name} description={resource.description || t("tools.reusableResourceConfigurationSharedExplicitlyBetweenAPIs")} action={<Badge color={resource.state === "active" ? "green" : "zinc"}>{resource.state}</Badge>} />
    <dl className="compact-metrics resource-detail-metrics">
      <div className="compact-metric"><dt>{t("tools.latestRevision")}</dt><dd><strong>r{revision?.revision ?? resource.revision}</strong><small>{t("tools.immutableSnapshot")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.contractEntries")}</dt><dd><strong>{entries.length}</strong><small>{resource.kind === "api" ? t("tools.apiDefinitions") : t("tools.documentationRecords")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.usedByAPIs")}</dt><dd><strong>{owners.length}</strong><small>{t("tools.explicitAttachments")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.updated")}</dt><dd><strong>{revision?.created_at ? t("format.date", { value: new Date(revision.created_at) }) : "—"}</strong><small>{revision?.created_at ? t("format.time", { value: new Date(revision.created_at) }) : t("tools.noRevisionDate")}</small></dd></div>
    </dl>
    <div className="entity-workspace-grid">
      <section className="panel entity-contract-panel">
        <PanelHeader title={t("tools.contractContents")} description={t("tools.whatThisReusableRevisionContributesWhenAttachedToAn")} action={<Badge color="zinc">r{revision?.revision ?? resource.revision}</Badge>} />
        <div className="entity-contract-list">{entries.map((entry, index) => <article key={`${manifestEntryTitle(entry, index, t)}:${index}`}><span className="resource-icon">{resource.kind === "api" ? <TerminalSquare /> : <BookOpen />}</span><span><strong>{manifestEntryTitle(entry, index, t)}</strong><small>{manifestEntrySummary(entry, t)}</small></span><Badge color={resource.kind === "api" ? "violet" : "blue"}>{manifestString(entry, ["kind", "type", "method"]) || resource.kind}</Badge></article>)}{entries.length === 0 && <div className="empty-row">{t("tools.thisRevisionContainsNoContractEntries")}</div>}</div>
        <details className="advanced-details inline-advanced"><summary>{t("tools.viewRevisionJSON")}</summary><pre className="entity-contract-json">{JSON.stringify(entries, null, 2)}</pre></details>
      </section>
      <aside className="entity-workspace-rail">
        <section className="panel entity-related-panel"><PanelHeader title={t("tools.usedByAPIs")} description={t("tools.openTheExactAPIWorkspaceTabThatAttachesThis")} />{owners.map((integration) => <ConsoleLink key={integration.id} path={integrationPath(integration.id, "documentation", resourceTab)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span><Badge color={integration.lifecycle === "active" ? "green" : "zinc"}>{integration.lifecycle}</Badge><ChevronRight /></ConsoleLink>)}{owners.length === 0 && <div className="empty-row">{t("tools.thisSetIsNotAttachedToAnAPI")}</div>}</section>
        <section className="panel entity-detail-panel"><PanelHeader title={t("tools.revisionIdentity")} /><dl className="entity-detail-grid compact-detail-grid"><div><dt>{t("tools.resourceSetID")}</dt><dd>{resource.id}</dd></div><div><dt>{t("tools.contentHash")}</dt><dd>{revision?.content_hash || "—"}</dd></div><div><dt>{t("tools.revisionID")}</dt><dd>{revision?.id || "—"}</dd></div><div><dt>{t("tools.attachmentPolicy")}</dt><dd>{t("tools.explicitOnly")}</dd></div></dl></section>
      </aside>
    </div>
  </>;
}

type ToolDetailTab = "overview" | "contract" | "execution" | "authorization" | "tests" | "usage" | "history";

const TOOL_DETAIL_TABS: ToolDetailTab[] = ["overview", "contract", "execution", "authorization", "tests", "usage", "history"];

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
  const { t } = useTranslation();
  const succeeded = run.outcome === "success";
  return <div className={`tool-live-test-evidence ${succeeded ? "passed" : "failed"}`}>
    <div className="tool-live-test-heading" role="status" aria-live="polite"><span><strong>{succeeded ? t("tools.liveTestCompleted") : t("tools.liveTestFoundAnIssue")}</strong><small>{run.tool_name} {t("tools.exactRevision")} {run.tool_revision} · {run.method} · {run.authentication_type}</small></span><Badge color={succeeded ? "green" : "red"}>{run.outcome}</Badge></div>
    <dl className="compact-metrics tool-live-test-metrics">
      <div className="compact-metric"><dt>{t("tools.phase")}</dt><dd><strong>{run.phase}</strong><small>{run.network_call_performed ? t("tools.upstreamCalled") : t("tools.stoppedBeforeNetwork")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.httpStatus")}</dt><dd><strong>{run.upstream_status_code ?? "—"}</strong><small>{run.upstream_status_code ? t("tools.sanitizedUpstreamStatus") : t("tools.noUpstreamResponse")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.responseSize")}</dt><dd><strong>{run.response_bytes === undefined ? "—" : t("tools.b", { response_bytes: String(run.response_bytes) })}</strong><small>{t("tools.bodyValueDiscarded")}</small></dd></div>
      <div className="compact-metric"><dt>{t("tools.duration")}</dt><dd><strong>{run.duration_ms} ms</strong><small>{t("tools.serverObserved")}</small></dd></div>
    </dl>
    <div className="private-default-note"><ShieldCheck />{t("tools.onlyStructuralEvidenceIsRetainedRawBodiesHeadersField")}</div>
    <div className="tool-test-shapes">
      <section aria-labelledby={`tool-test-request-${run.id}`}><h3 id={`tool-test-request-${run.id}`}>{t("tools.requestShape")}</h3><pre>{JSON.stringify(run.request_shape, null, 2)}</pre></section>
      <section aria-labelledby={`tool-test-response-${run.id}`}><h3 id={`tool-test-response-${run.id}`}>{t("tools.responseShape")}</h3>{run.response_shape ? <pre>{JSON.stringify(run.response_shape, null, 2)}</pre> : <p>{t("tools.noResponseShapeWasRetained")}</p>}</section>
    </div>
    <section className="tool-test-findings" aria-labelledby={`tool-test-findings-${run.id}`}><h3 id={`tool-test-findings-${run.id}`}>{t("tools.findings")}</h3>{run.findings.length > 0 ? run.findings.map((finding, index) => <div className="publish-validation" key={`${finding.phase}:${finding.code}:${index}`}><span>{succeeded ? <ShieldCheck /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.phase} · {finding.message}{finding.instance_path ? t("tools.instance", { instance_path: String(finding.instance_path) }) : ""}{finding.schema_path ? t("tools.schema", { schema_path: String(finding.schema_path) }) : ""}</small></span></div>) : <div className="empty-row">{t("tools.noStructuralOrPolicyFindings")}</div>}</section>
    <footer><code>{run.id}</code><span>{t("tools.created")} {t("format.dateTime", { value: new Date(run.created_at) })} {t("tools.evidenceExpires")} {t("format.dateTime", { value: new Date(run.expires_at) })}</span></footer>
  </div>;
}

function ToolLiveTestAnalysis({ run, tool, onOpenBuilder, onClone, onMessage }: { run: APIToolTestRun; tool: APITool; onOpenBuilder: (proposal: APIToolTestAnalysisProposal) => void; onClone: (proposal: APIToolTestAnalysisProposal) => void; onMessage: (message: string) => void }) {
  const { t } = useTranslation();
  const [evidenceHash, setEvidenceHash] = useState("");
  const [hashError, setHashError] = useState("");
  const [consentOpen, setConsentOpen] = useState(false);
  const [consentChecked, setConsentChecked] = useState(false);
  const [consentGranted, setConsentGranted] = useState(false);
  const defaultQuestion = t("tools.defaultAnalysisQuestion");
  const previousDefaultQuestion = useRef(defaultQuestion);
  const [question, setQuestion] = useState<string>(defaultQuestion);
  const [transcript, setTranscript] = useState<APIToolTestAnalysisMessage[]>([]);
  const [analysis, setAnalysis] = useState<APIToolTestAnalysis | null>(null);
  const [analysisError, setAnalysisError] = useState("");
  const [busy, setBusy] = useState(false);
  const [expired] = useState(() => Date.parse(run.expires_at) <= Date.now());
  const preview = toolTestAnalysisEvidencePreview(run);
  const questionBytes = useMemo(() => new TextEncoder().encode(question.trim()).byteLength, [question]);
  const questionProblem = questionBytes > TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes ? t("tools.questionByteLimit", { limit: TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes }) : "";

  useEffect(() => {
    setQuestion((current) => current === previousDefaultQuestion.current ? defaultQuestion : current);
    previousDefaultQuestion.current = defaultQuestion;
  }, [defaultQuestion]);

  useEffect(() => {
    let cancelled = false;
    toolTestAnalysisEvidenceHash(run).then((value) => { if (!cancelled) setEvidenceHash(value); }).catch(() => { if (!cancelled) setHashError(t("tools.serverEvidenceBindingInvalid")); });
    return () => { cancelled = true; };
  }, [run, t]);

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
      if (result.evidence_hash !== evidenceHash || result.tool_revision !== run.tool_revision || !result.advisory) throw new Error(t("tools.analysisResponseBindingInvalid"));
      if (result.proposal && (result.proposal.base_tool_id !== tool.id || result.proposal.base_revision !== run.tool_revision || result.proposal.requires_clone !== (tool.state === "published"))) throw new Error(t("tools.proposalBindingInvalid"));
      setAnalysis(result);
      setTranscript((messages) => boundedToolTestAnalysisHistory([...messages, { role: "user", content: latestQuestion }, { role: "assistant", content: result.reply }]));
      setQuestion("");
      onMessage(result.proposal ? t("tools.analysisReturnedALocallyValidatedProposalForHumanReview") : t("tools.analysisRepliedFromTheConsentedSanitizedEvidence"));
    } catch (error) {
      const message = unavailableConsoleCapability(error) ? t("tools.liveTestAnalysisUnavailable") : error instanceof APIError || error instanceof Error ? error.message : t("tools.sanitizedEvidenceAnalysisFailed");
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
    <header><span className="settings-icon"><Sparkles /></span><span><strong id={`tool-test-analysis-${run.id}`}>{t("tools.askAnalysisAboutThisRun")}</strong><small>{t("tools.advisoryOnlyExactRevision")} {run.tool_revision} {t("tools.evidenceExpires")} {t("format.dateTime", { value: new Date(run.expires_at) })}</small></span><Badge color="violet">{t("tools.optionalAI")}</Badge></header>
    <p className="tool-test-analysis-intro">{t("tools.nothingIsSharedUntilYouReviewThisBoundaryAnd")}</p>
    <div className="tool-test-analysis-boundary">
      <section><h3>{t("tools.sentAfterConsent")}</h3><ul><li>{t("tools.shapesContainingOnlySchemaDeclaredPropertyNamesJSONTypes")}</li><li>{t("tools.structuralNonSecretContractSchemasWithoutAnnotationsOrLiteral")}</li><li>{t("tools.yourLatestQuestionAndBoundedUserAssistantHistory")}</li></ul></section>
      <section><h3>{t("tools.neverSent")}</h3><ul><li>{t("tools.rawValuesOrBodiesResponseContentRequestArgumentsExamples")}</li><li>{t("tools.unexpectedUpstreamPropertyNamesDiagnosticPathsHeadersCredentialsNonces")}</li><li>{t("tools.destinationOriginLiteralPathQueryEvidenceHashToolRun")}</li></ul></section>
    </div>
    <div className="tool-test-analysis-hash"><span>{t("tools.evidencePreviewHashBrowserServerBindingOnly")}</span>{evidenceHash ? <code>{evidenceHash}</code> : <small>{hashError || t("tools.checkingServerComputedSHAN256Binding")}</small>}</div>
    <details className="tool-test-analysis-preview"><summary>{t("tools.reviewTheExactSanitizedEvidencePreview")}</summary><pre>{JSON.stringify(preview, null, 2)}</pre></details>
    {expired && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("tools.evidenceExpired")}</strong><small>{t("tools.runANewExactRevisionLiveTestBeforeRequesting")}</small></span></div>}
    {transcript.length > 0 && <div className="tool-test-analysis-transcript" aria-live="polite">{transcript.map((message, index) => <article className={message.role} key={`${message.role}:${index}`}><span>{message.role === "assistant" ? <Sparkles /> : <MessageSquareText />}</span><div><strong>{message.role === "assistant" ? t("tools.analysis") : t("tools.you")}</strong><p>{message.content}</p></div></article>)}</div>}
    <label className="auth-field tool-test-analysis-question" htmlFor={`tool-test-analysis-question-${run.id}`}><span>{transcript.length > 0 ? t("tools.followUpQuestion") : t("tools.questionForAnalysis")}</span><textarea id={`tool-test-analysis-question-${run.id}`} maxLength={TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes} value={question} aria-invalid={Boolean(questionProblem)} aria-describedby={`tool-test-analysis-question-guidance-${run.id}${questionProblem ? ` tool-test-analysis-question-error-${run.id}` : ""}`} onChange={(event) => setQuestion(event.target.value)} placeholder={t("tools.askAboutTheRetainedShapesFindingsOrNonSecret")} /><small id={`tool-test-analysis-question-guidance-${run.id}`}>{questionBytes}/{TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes} {t("tools.utfN8BytesDoNotIncludeSecretsRawValues")}</small>{questionProblem && <small className="error" id={`tool-test-analysis-question-error-${run.id}`} role="alert">{questionProblem}</small>}</label>
    {consentGranted && <label className="tool-test-analysis-consent"><input type="checkbox" checked={consentGranted} onChange={(event) => setConsentGranted(event.target.checked)} /><span>{t("tools.iContinueToConsentToSendingTheProviderProjection")}</span></label>}
    {analysisError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("tools.analysisUnavailable")}</strong><small>{analysisError}</small></span></div>}
    <div className="tool-test-analysis-actions"><Button outline disabled={busy || expired || !evidenceHash || !question.trim() || Boolean(questionProblem)} onClick={consentGranted ? () => { void sendAnalysis(); } : reviewConsent}>{busy ? t("tools.analysing") : consentGranted ? transcript.length > 0 ? t("tools.sendFollowUp") : t("tools.askAnalysis") : t("tools.reviewConsentAsk")}</Button><small>{consentGranted ? t("tools.consentAppliesOnlyToThisBrowserHeldConversationExact") : t("tools.theConfiguredProviderIsContactedOnlyAfterTheConsent")}</small></div>
    {analysis && <div className="tool-test-analysis-result">
      <div className="analysis-summary"><span className="settings-icon"><Sparkles /></span><span><strong>{t("tools.advisoryReply")}</strong><small>{analysis.reply}</small></span><Badge color={analysis.provider_outcome === "succeeded" ? "green" : "amber"}>{analysis.provider_outcome}</Badge></div>
      {analysis.findings.length > 0 && <section className="tool-test-analysis-findings"><h3>{t("tools.advisoryFindings")}</h3>{analysis.findings.map((finding, index) => <div className="publish-validation" key={`${finding.code}:${index}`}><span><TriangleAlert /></span><span><strong>{finding.code}</strong><small>{finding.message}{finding.suggestion ? t("tools.copy", { suggestion: String(finding.suggestion) }) : ""}</small></span></div>)}</section>}
      {proposal && <section className="tool-test-analysis-proposal"><div className="tool-test-analysis-proposal-heading"><span><strong>{t("tools.reviewableContractProposal")}</strong><small>{t("tools.boundToToolRevision")} {proposal.base_revision} · {proposal.changes.length} {t("tools.changedTopLevelField")}{proposal.changes.length === 1 ? "" : t("tools.s")} {t("tools.neverAppliedAutomatically")}</small></span><Badge color={proposal.valid ? "green" : "red"}>{proposal.valid ? t("tools.locallyValid") : t("tools.needsReview")}</Badge></div>
        {proposal.changes.length > 0 ? <ul>{proposal.changes.map((change) => <li key={change.field}><span><code>{change.field}</code>{change.security_sensitive && <Badge color="amber">{t("tools.securitySensitive")}</Badge>}</span><small>{change.rationale || t("tools.reviewThisProposedFieldChange")}</small></li>)}</ul> : <div className="empty-row">{t("tools.theProviderReturnedTheUnchangedExactRevisionContract")}</div>}
        {proposal.findings.length > 0 && <div className="tool-test-analysis-proposal-findings">{proposal.findings.map((finding, index) => <div className="publish-validation" key={`${finding.code}:${index}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.message}</small></span></div>)}</div>}
        <details className="tool-test-analysis-proposed-draft"><summary>{t("tools.reviewTheCompleteLocallyValidatedProposal")}</summary><pre>{JSON.stringify(proposal.draft, null, 2)}</pre></details>
        <footer>{proposal.requires_clone || tool.state === "published" ? <><div className="private-default-note"><LockKeyhole />{t("tools.publishedRevisionsAreImmutableThisProposalCannotBeApplied")}</div><Button outline onClick={() => onClone(proposal)}>{t("tools.cloneReviewProposal")}</Button></> : <><div className="private-default-note"><ShieldCheck />{t("tools.thisProposalHasNotChangedTheDraftOpenThe")}</div><Button outline onClick={() => onOpenBuilder(proposal)}>{t("tools.openBuilderToReview")}</Button></>}</footer>
      </section>}
    </div>}
    <Dialog open={consentOpen} onClose={setConsentOpen} title={t("tools.sendSanitizedEvidenceToAnalysis")} description={t("tools.yourConfiguredAnalysisProviderIsAnExternalProcessingBoundary")} actions={<><Button outline onClick={() => setConsentOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={!consentChecked || !evidenceHash || !question.trim() || Boolean(questionProblem) || expired || busy} onClick={acceptConsentAndSend}>{t("tools.consentAskAnalysis")}</Button></>}><div className="tool-test-analysis-consent-dialog"><div className="private-default-note"><ShieldCheck />{t("tools.theServerRecomputes")} <code>{evidenceHash || t("tools.pendingSHA256Hash")}</code>{t("tools.enforcesThisToolRunRevisionAndExpiryDurablyRecords")}</div><p>{t("tools.onlySchemaDeclaredPropertyNamesJSONTypesArrayLengths")}</p><label><input type="checkbox" checked={consentChecked} onChange={(event) => setConsentChecked(event.target.checked)} /><span>{t("tools.iExplicitlyConsentToSendThisSanitizedEvidenceAnd")}</span></label></div></Dialog>
  </section>;
}

export function ToolDetailView({ productID, tool, connections, integrations, auditEvents, onChanged, onReviewProposal, onMessage, onNavigate }: { productID: string; tool: APITool | null; connections: APIMCPConnection[]; integrations: APIIntegration[]; auditEvents: APIAuditEvent[]; onChanged: () => Promise<void>; onReviewProposal: (tool: APITool, proposal: APIToolTestAnalysisProposal) => void; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const toolUpstreamAuthCopy: Record<NonNullable<APITool["upstream_auth"]>["type"], { label: string; description: string; credentialRequired: boolean }> = {
    delegated_oauth: { label: t("tools.delegatedOAuth"), description: t("tools.delegatedOAuthDescription"), credentialRequired: false },
    none: { label: t("tools.noAuthentication"), description: t("tools.noAuthenticationDescription"), credentialRequired: false },
    bearer: { label: t("tools.bearerToken"), description: t("tools.bearerTokenDescription"), credentialRequired: true },
    authorization_scheme: { label: t("tools.authorizationScheme"), description: t("tools.authorizationSchemeDescription"), credentialRequired: true },
    api_key_header: { label: t("tools.apiKeyHeader"), description: t("tools.apiKeyHeaderDescription"), credentialRequired: true },
    api_key_query: { label: t("tools.apiKeyQueryParameter"), description: t("tools.apiKeyQueryParameterDescription"), credentialRequired: true },
    basic: { label: t("tools.httpBasic"), description: t("tools.httpBasicDescription"), credentialRequired: true },
    oauth_client_credentials: { label: t("tools.oauthClientCredentials"), description: t("tools.oauthClientCredentialsDescription"), credentialRequired: true },
    custom_header: { label: t("tools.customSecretHeader"), description: t("tools.customSecretHeaderDescription"), credentialRequired: true },
  };
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
    void api.integrationAuthorization(integrationID).then((value) => {
      if (!cancelled) setRuntimeSetup(value);
    }).catch(() => {
      if (!cancelled) setRuntimeSetup(null);
    });
    return () => { cancelled = true; };
  }, [activeTool?.owner_integration_id, activeTool?.runtime_service_connection_id]);

  if (!toolID) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>{t("tools.toolUnavailable")}</h1><p>{t("tools.thisToolCouldNotBeFoundInTheDeployment")}</p></div><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.returnToTools")}</ConsoleLink></section>;
  if (!activeTool) return <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon">{detailStatus === "loading" ? <RefreshCw /> : <TriangleAlert />}</span><div><h1>{detailStatus === "loading" ? t("tools.loadingTool") : t("tools.toolDetailsUnavailable")}</h1><p>{detailStatus === "loading" ? t("tools.loadingTheCompleteContractAndFixedExecutionTarget") : t("tools.theCompleteToolContractCouldNotBeLoadedNo")}</p></div>{detailStatus === "error" ? <Button outline onClick={() => { setActiveTool(null); setDetailStatus("loading"); setDetailLoadAttempt((value) => value + 1); }}>{t("common.retry")}</Button> : <ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.returnToTools")}</ConsoleLink>}</section>;

  const currentTool = activeTool;
  const backendKind = activeTool.backend_kind ?? "http";
  const apiOwned = activeTool.scope === "api" && Boolean(activeTool.owner_integration_id);
  const owningIntegration = apiOwned ? integrations.find((integration) => integration.id === activeTool.owner_integration_id) : undefined;
  const runtimeConnection = activeTool.runtime_service_connection_id ? runtimeSetup?.endpoint_bindings.find((candidate) => candidate.id === activeTool.runtime_service_connection_id) : undefined;
  const runtimeRevision = runtimeConnection?.current_revisions?.find((candidate) => candidate.current && runtimeSetup?.environments.find((environment) => environment.id === candidate.environment_id)?.is_production) ?? runtimeConnection?.current_revisions?.find((candidate) => candidate.current);
  const runtimeAuthentication = runtimeRevision ? toolUpstreamAuthCopy[runtimeRevision.authentication_type] ?? toolUpstreamAuthCopy.none : null;
  const connection = activeTool.mcp_connection_id ? connections.find((item) => item.id === activeTool.mcp_connection_id) : null;
  const upstreamAuthType = activeTool.upstream_auth?.type ?? "delegated_oauth";
  const upstreamAuth = toolUpstreamAuthCopy[upstreamAuthType] ?? toolUpstreamAuthCopy.delegated_oauth;
  const credentialStatus = activeTool.credential_present ? t("tools.stored") : upstreamAuth.credentialRequired ? t("tools.missing") : upstreamAuthType === "delegated_oauth" ? t("tools.callerTokenNotStored") : t("tools.notRequired");
  const cloneCredentialLabel = upstreamAuthType === "basic" ? t("tools.password") : upstreamAuthType === "oauth_client_credentials" ? t("tools.clientSecret") : upstreamAuthType === "bearer" ? t("tools.bearerToken") : t("tools.secretValue");
  const requestMappingEntries = Object.entries(activeTool.request_mapping?.parameter_locations ?? {});
  const requestMappingSummary = requestMappingEntries.length > 0 ? t("tools.explicitParameterMappings", { count: requestMappingEntries.length }) : t("tools.defaultRequestMapping", { location: method.toUpperCase() === "GET" ? t("tools.query") : t("tools.body") });
  const responseMappingSummary = activeTool.response_mapping?.result_path ? t("tools.resultAt", { path: activeTool.response_mapping.result_path }) : t("tools.entireResponseDocument");
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
      ? t("tools.importedMcpToolsLiveTestLimitation")
      : backendKind === "native"
        ? t("tools.nativeToolsLiveTestLimitation")
      : delegatedOAuthLiveTest
        ? t("tools.delegatedOAuthLiveTestLimitation")
      : mutationTest && !currentPolicy.idempotencyRequired
        ? t("tools.mutationLiveTestLimitation")
      : !activeTool.runtime_service_connection_id && upstreamAuth.credentialRequired && !activeTool.credential_present
          ? t("tools.missingCredentialLiveTestLimitation")
          : !contractCheckPassed
            ? t("tools.contractCheckLiveTestLimitation")
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
      onMessage(t(refreshed ? "tools.publishedAndAvailableForAPIBinding" : "tools.publishedAndAvailableForAPIBindingReload", { namespace: published.namespace, name: published.name }));
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : t("tools.toolCouldNotBePublished")); } finally { setBusy(false); }
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
        onMessage(t("tools.contractCheckResultDiscardedBecauseTheVisibleTestInputs"));
        return;
      }
      setTestResult(result);
      if (result.valid && !result.network_call_performed && result.revision === currentTool.revision) {
        setValidatedTestInput(inputSnapshot);
        onMessage(t("tools.contractCheckPassedWithoutANetworkCall"));
        return;
      }
      setContractCheckError(t("tools.persistedContractValidationFailed"));
      onMessage(t("tools.contractCheckReturnedAControlledFailureWithoutCallingThe"));
    } catch (error) {
      if (!versionedResponseIsCurrent(requestVersion, testFormVersionRef.current)) {
        onMessage(t("tools.contractCheckResultDiscardedBecauseTheVisibleTestInputs"));
        return;
      }
      const message = unavailableConsoleCapability(error) ? t("tools.contractCheckingUnavailable") : error instanceof APIError || error instanceof Error ? error.message : t("tools.contractCheckFailed");
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
      onMessage(t("tools.liveTestResultRetainedByTheServerButHidden"));
      return false;
    }
    setLiveTestResult(result);
    onMessage(result.outcome === "success" ? t("tools.liveUpstreamTestCompletedWithSanitizedEvidence") : t("tools.liveUpstreamTestStoppedSafelyDuring", { phase: String(result.phase) }));
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
      setLiveTestError(t("tools.invalidIdempotencyKey"));
      return;
    }
    let argumentsObject: Record<string, unknown>;
    try { argumentsObject = parseToolTestArguments(testInput); }
    catch { setLiveTestError(t("tools.jsonArgumentsInvalid")); return; }
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
      const message = unavailableConsoleCapability(error) ? t("tools.liveUpstreamTestingUnavailable") : error instanceof APIError || error instanceof Error ? error.message : t("tools.liveUpstreamTestFailed");
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
      setLiveTestError(t("tools.visibleTestInputsChanged"));
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
      if (confirmation.tool_id !== currentTool.id || confirmation.tool_revision !== currentTool.revision) throw new Error(t("tools.confirmationBindingInvalid"));
      await executeLiveToolTest(pendingTestArguments, requestVersion, idempotencyKey, confirmation.confirmation_nonce);
      setTestConfirmationOpen(false);
      setPendingTestArguments(null);
      pendingTestVersionRef.current = 0;
      pendingTestIdempotencyKeyRef.current = "";
    } catch (error) {
      const message = unavailableConsoleCapability(error) ? t("tools.liveUpstreamTestingUnavailable") : error instanceof APIError || error instanceof Error ? error.message : t("tools.liveUpstreamTestFailed");
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
    const currentIndex = TOOL_DETAIL_TABS.findIndex((tab) => `tool-tab-${tab}` === document.activeElement?.id);
    if (currentIndex < 0) return;
    event.preventDefault();
    const nextIndex = event.key === "Home" ? 0 : event.key === "End" ? TOOL_DETAIL_TABS.length - 1 : event.key === "ArrowRight" ? (currentIndex + 1) % TOOL_DETAIL_TABS.length : (currentIndex - 1 + TOOL_DETAIL_TABS.length) % TOOL_DETAIL_TABS.length;
    const nextTab = TOOL_DETAIL_TABS[nextIndex];
    setActiveTab(nextTab);
    requestAnimationFrame(() => document.getElementById(`tool-tab-${nextTab}`)?.focus());
  }

  function openCloneTool(proposal: APIToolTestAnalysisProposal | null = null) {
	if ((currentTool.backend_kind ?? "http") !== "http") {
	  onMessage(t("tools.sourceManagedAndImportedToolsCannotBeCloned"));
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
      const messageKey = proposalToReview
        ? refreshed ? "tools.createdDraftWithProposal" : "tools.createdDraftWithProposalReload"
        : refreshed ? "tools.createdAsAnIndependentDraft" : "tools.createdAsAnIndependentDraftReload";
      onMessage(t(messageKey, { namespace: cloned.namespace, name: cloned.name }));
      if (proposalToReview) onReviewProposal(cloned, proposalToReview);
      else onNavigate(entityPath("tool", cloned.id));
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? t("tools.toolCloningIsNotEnabledByThisServiceVersion") : error instanceof APIError ? error.message : t("tools.toolCouldNotBeCloned")); } finally { setBusy(false); }
  }

  async function retireTool() {
    setBusy(true);
    try {
      const retired = await api.retireTool(productID, currentTool.id, currentTool.revision);
      setActiveTool(retired);
      const refreshed = await refreshAfterMutation();
      setRetireOpen(false);
      onMessage(t(refreshed ? "tools.toolRetiredExistingExactAPIBindingsAreNowUnresolved" : "tools.toolRetiredExistingExactAPIBindingsAreNowUnresolvedReload"));
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? t("tools.toolRetirementIsNotEnabledByThisServiceVersion") : error instanceof APIError ? error.message : t("tools.toolCouldNotBeRetired")); } finally { setBusy(false); }
  }

  const readiness = [
    { label: t("tools.agentContract"), ready: Boolean(activeTool.description && Object.keys(activeTool.input_schema).length && Object.keys(activeTool.output_schema).length) },
    { label: backendKind === "native" ? t("tools.pinnedPluginSource") : activeTool.runtime_service_connection_id ? t("tools.apiServiceConnection") : t("tools.fixedExecutionTarget"), ready: backendKind === "native" ? Boolean(activeTool.native_plugin_id && activeTool.native_contract_hash) : backendKind === "mcp" ? Boolean(connection && activeTool.upstream_tool_name) : activeTool.runtime_service_connection_id ? Boolean(activeTool.http_path && runtimeConnection) : Boolean(activeTool.endpoint) },
    { label: t("tools.safetyPolicy"), ready: ["low", "medium", "high", "critical"].includes(currentPolicy.risk ?? "low") },
    { label: t("tools.publishedForManagedBinding"), ready: activeTool.state === "published" && !activeTool.upstream_drifted },
  ];
  const liveTestStageDescription = testConfirmationRequired
    ? mutationTest
      ? t(tokenExchangeTest ? "tools.liveTestStageReviewMutationWithTokenExchange" : "tools.liveTestStageReviewMutation")
      : t(tokenExchangeTest ? "tools.liveTestStageReviewReadWithTokenExchange" : "tools.liveTestStageReviewRead")
    : t(tokenExchangeTest ? "tools.liveTestStageCallWithTokenExchange" : "tools.liveTestStageCall");
  const liveTestConfirmationDescription = mutationTest
    ? t(tokenExchangeTest ? "tools.confirmMutationLiveTestWithTokenExchange" : "tools.confirmMutationLiveTest", { method: normalizedTestMethod, name: fullToolName, revision: currentTool.revision })
    : t(tokenExchangeTest ? "tools.confirmReadLiveTestWithTokenExchange" : "tools.confirmReadLiveTest", { method: normalizedTestMethod, name: fullToolName, revision: currentTool.revision });
  const liveTestRequestDescription = mutationTest
    ? t(tokenExchangeTest ? "tools.mutationRequestWithTokenExchange" : "tools.mutationRequest")
    : t(tokenExchangeTest ? "tools.readRequestWithTokenExchange" : "tools.readRequest");
  const liveTestAcknowledgement = mutationTest
    ? t(tokenExchangeTest ? "tools.acknowledgeMutationWithTokenExchange" : "tools.acknowledgeMutation")
    : t(tokenExchangeTest ? "tools.acknowledgeReadWithTokenExchange" : "tools.acknowledgeRead");

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={owningIntegration ? integrationPath(owningIntegration.id, "tools") : sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{owningIntegration ? t("tools.tools", { display_name: String(owningIntegration.display_name) }) : t("tools.commonTools")}</ConsoleLink><Badge color={apiOwned || activeTool.backend_kind !== "http" ? "violet" : "zinc"}>{apiOwned ? t("tools.apiScoped") : activeTool.backend_kind === "native" ? t("tools.nativePlugin") : activeTool.backend_kind === "mcp" ? "MCP" : t("tools.commonHTTP")}</Badge></div>
    <PageHeading eyebrow={owningIntegration ? t("tools.apiTool", { display_name: String(owningIntegration.display_name) }) : t("tools.commonDeploymentTool")} title={t("tools.copy2", { namespace: String(activeTool.namespace), name: String(activeTool.name) })} action={<span className="heading-actions">{activeTool.state === "draft" && <>{activeTool.backend_kind === "http" ? <Button outline disabled={busy} onClick={() => onNavigate(toolBuilderPath(activeTool.id))}><Wrench data-slot="icon" />{t("tools.editInBuilder")}</Button> : activeTool.backend_kind === "mcp" && connection && <ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-back-link">{t("tools.reviewConnection")}</ConsoleLink>}<Button color="indigo" disabled={busy || activeTool.upstream_drifted} onClick={publishToolRevision}>{t("tools.publishTool")}</Button></>}{activeTool.state === "published" && <>{activeTool.backend_kind === "http" && (owningIntegration ? <Button outline disabled={busy} onClick={() => onNavigate(integrationToolBuilderPath(owningIntegration.id))}><Plus data-slot="icon" />{t("tools.createAnotherAPITool")}</Button> : <Button outline disabled={busy} onClick={() => openCloneTool()}><Copy data-slot="icon" />{t("tools.cloneAsNewTool")}</Button>)}{activeTool.backend_kind === "mcp" && connection && <ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-back-link">{t("tools.reviewConnection")}</ConsoleLink>}<Button outline disabled={busy} onClick={() => setRetireOpen(true)}>{t("tools.retire")}</Button></>}{activeTool.state === "retired" && <Badge color="zinc">{t("tools.retired")}</Badge>}</span>} />
    <div className="page-tabs" role="tablist" aria-label={t("tools.toolSections")}>{TOOL_DETAIL_TABS.map((tab) => <button type="button" role="tab" id={`tool-tab-${tab}`} aria-controls={`tool-panel-${tab}`} aria-selected={activeTab === tab} tabIndex={activeTab === tab ? 0 : -1} key={tab} className={`page-tab ${activeTab === tab ? "active" : ""}`} onKeyDown={handleToolTabKeyDown} onClick={() => setActiveTab(tab)}>{tab === "overview" ? t("routes.overview") : tab === "contract" ? t("common.contract") : tab === "execution" ? t("common.execution") : tab === "authorization" ? t("common.authorization") : tab === "tests" ? t("common.tests") : tab === "usage" ? t("common.usage") : t("routes.history")}</button>)}</div>

    {activeTab === "overview" && <div className="tool-detail-section" role="tabpanel" id="tool-panel-overview" aria-labelledby="tool-tab-overview" tabIndex={0}>
      <dl className="compact-metrics tool-detail-metrics"><div className="compact-metric"><dt>{t("tools.state")}</dt><dd><strong>{activeTool.state}</strong><small>{t("tools.revisionNumber", { revision: activeTool.revision })}</small></dd></div><div className="compact-metric"><dt>{t("tools.backend")}</dt><dd><strong>{activeTool.backend_kind === "native" ? t("tools.native") : activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"}</strong><small>{activeTool.backend_kind === "native" ? t("tools.copy3", { native_plugin_id: String(activeTool.native_plugin_id), native_plugin_version: String(activeTool.native_plugin_version) }) : activeTool.backend_kind === "mcp" ? activeTool.upstream_tool_name || t("tools.upstreamTool") : t("tools.request", { http_method: String(activeTool.http_method) })}</small></dd></div><div className="compact-metric"><dt>{t("tools.risk")}</dt><dd><strong>{currentPolicy.risk ?? t("tools.low")}</strong><small>{currentPolicy.confirmationRequired ? t("tools.confirmationRequired") : t("tools.noConfirmation")}</small></dd></div><div className="compact-metric"><dt>{t("tools.currentConfig")}</dt><dd><strong>{usageStatus === "loading" ? "…" : usages.length}</strong><small>{t("tools.apiBindings", { count: usages.length })}</small></dd></div></dl>
      <div className="entity-workspace-grid"><section className="panel"><PanelHeader title={t("tools.readiness")} description={apiOwned ? t("tools.thisDefinitionRemainsOwnedByOneAPIAndInherits") : t("tools.aPublishedCommonToolBecomesEligibleForAManaged")} />{readiness.map((item) => <div className="integration-health-check" key={item.label}><span className={`health-icon ${item.ready ? "ready" : ""}`}>{item.ready ? <CheckCircle2 /> : <XCircle />}</span><span><strong>{item.label}</strong><small>{item.ready ? t("tools.ready") : t("tools.actionRequired")}</small></span><Badge color={item.ready ? "green" : "amber"}>{item.ready ? t("tools.ready") : t("tools.review")}</Badge></div>)}</section><aside className="entity-workspace-rail"><section className="panel entity-policy-panel"><PanelHeader title={t("tools.deliveryBoundary")} /><div className="entity-policy-check"><span className="ready"><ShieldCheck /></span><span><strong>{t("tools.privateMCP")}</strong><small>{activeTool.state === "published" ? t("tools.managedAPIDiscoveryRequiresAnExactToolAndAuthorization") : t("tools.publishBeforeManagedAPIsCanBindThisTool")}</small></span></div></section><section className="panel entity-detail-panel"><PanelHeader title={t("tools.identity")} /><dl className="entity-detail-grid compact-detail-grid"><div><dt>{t("tools.toolID")}</dt><dd>{activeTool.id}</dd></div><div><dt>{t("tools.scope")}</dt><dd>{owningIntegration?.display_name ?? t("tools.common")}</dd></div><div><dt>{t("tools.revision")}</dt><dd>{activeTool.revision}</dd></div><div><dt>{t("tools.drift")}</dt><dd>{activeTool.upstream_drifted ? t("tools.detected") : t("tools.none")}</dd></div><div><dt>{t("tools.lifecycle")}</dt><dd>{activeTool.state}</dd></div></dl></section></aside></div>
    </div>}

    {activeTab === "contract" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-contract" aria-labelledby="tool-tab-contract" tabIndex={0}><PanelHeader title={t("tools.agentContract")} description={t("tools.readOnlyExactRevisionUseTheToolBuilderTo")} /><label className="auth-field"><span>{t("tools.purpose")}</span><textarea readOnly value={description} /></label><div className="two-fields tool-schema-fields"><label className="auth-field"><span>{t("tools.inputJSONSchema")}</span><textarea spellCheck={false} readOnly value={inputSchema} /></label><label className="auth-field"><span>{t("tools.outputJSONSchema")}</span><textarea spellCheck={false} readOnly value={outputSchema} /></label></div></section>}

    {activeTab === "execution" && <div className="entity-workspace-grid" role="tabpanel" id="tool-panel-execution" aria-labelledby="tool-tab-execution" tabIndex={0}>
      <section className="panel tool-editor-page">
        <PanelHeader title={t("tools.execution")} description={t("tools.theDestinationAuthenticationModeAndRequestMappingsAreFixed")} />
        <div className="two-fields"><label className="auth-field"><span>{t("tools.backend")}</span><input value={activeTool.backend_kind === "native" ? t("tools.nativePlugin") : activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"} readOnly /></label><label className="auth-field"><span>{t("tools.timeoutMs")}</span><input readOnly type="number" value={timeout} /></label></div>
        {activeTool.backend_kind === "http" ? activeTool.runtime_service_connection_id ? <>
          <div className="two-fields"><label className="auth-field"><span>{t("tools.method")}</span><input value={method} readOnly /></label><label className="auth-field"><span>{t("tools.relativePath")}</span><input readOnly value={activeTool.http_path ?? ""} /></label></div>
          <div className="private-default-note"><LockKeyhole />{t("tools.theServiceHostAuthenticationAndEncryptedCredentialAreInherited")}</div>
          <dl className="entity-detail-grid compact-detail-grid">
            <div><dt>{t("tools.serviceConnection")}</dt><dd>{runtimeConnection?.name ?? t("tools.loadingSavedConnection")}</dd></div>
            <div><dt>{t("tools.connectionID")}</dt><dd>{activeTool.runtime_service_connection_id}</dd></div>
            <div><dt>{t("tools.authentication")}</dt><dd>{runtimeAuthentication?.label ?? t("tools.inheritedFromAPI")}</dd></div>
            <div><dt>{t("tools.requestMapping")}</dt><dd>{requestMappingSummary}</dd></div>
            <div><dt>{t("tools.responseMapping")}</dt><dd>{responseMappingSummary}</dd></div>
          </dl>
          <details className="advanced-details inline-advanced"><summary>{t("tools.mappingsAndExamples")}</summary><div className="two-fields tool-schema-fields">
            <label className="auth-field"><span>{t("tools.requestMapping")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_mapping ?? { parameter_locations: {} }, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.responseMapping")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_mapping ?? {}, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.requestExample")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_example, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.responseExample")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_example, t("tools.notConfigured"))} spellCheck={false} /></label>
          </div></details>
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>{t("tools.method")}</span><input value={method} readOnly /></label><label className="auth-field"><span>{t("tools.fixedEndpoint")}</span><input readOnly type="url" value={endpoint} /></label></div>
          <div className="private-default-note"><LockKeyhole />{upstreamAuth.description} {t("tools.agentsCannotReadStoredCredentialsOrChangeTheConfigured")}</div>
          <dl className="entity-detail-grid compact-detail-grid">
            <div><dt>{t("tools.upstreamAuthentication")}</dt><dd>{upstreamAuth.label}</dd></div>
            <div><dt>{t("tools.credential")}</dt><dd>{credentialStatus}</dd></div>
            <div><dt>{t("tools.requestMapping")}</dt><dd>{requestMappingSummary}</dd></div>
            <div><dt>{t("tools.responseMapping")}</dt><dd>{responseMappingSummary}</dd></div>
          </dl>
          <details className="advanced-details inline-advanced"><summary>{t("tools.authenticationMappingsAndExamples")}</summary><div className="two-fields tool-schema-fields">
            <label className="auth-field"><span>{t("tools.upstreamAuthentication")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.upstream_auth ?? { type: upstreamAuthType }, t("tools.notConfigured"))} spellCheck={false} /><small>{t("tools.nonSecretConfigurationOnlyStoredCredentialMaterialIsNever")}</small></label>
            <label className="auth-field"><span>{t("tools.requestMapping")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_mapping ?? { parameter_locations: {} }, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.responseMapping")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_mapping ?? {}, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.requestExample")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.request_example, t("tools.notConfigured"))} spellCheck={false} /></label>
            <label className="auth-field"><span>{t("tools.responseExample")}</span><textarea className="code-input" readOnly value={toolJSON(activeTool.response_example, t("tools.notConfigured"))} spellCheck={false} /></label>
          </div></details>
        </> : activeTool.backend_kind === "native" ? <><dl className="entity-detail-grid"><div><dt>{t("tools.plugin")}</dt><dd>{activeTool.native_plugin_id}</dd></div><div><dt>{t("tools.pluginVersion")}</dt><dd>{activeTool.native_plugin_version}</dd></div><div><dt>{t("tools.sdkVersion")}</dt><dd>{activeTool.native_sdk_version}</dd></div><div><dt>{t("tools.toolID")}</dt><dd>{activeTool.native_tool_id}</dd></div><div><dt>{t("tools.identity")}</dt><dd>{activeTool.identity_requirement}</dd></div><div><dt>{t("tools.stateScope")}</dt><dd>{activeTool.state_scope}</dd></div><div><dt>{t("tools.effect")}</dt><dd>{activeTool.effect}</dd></div><div><dt>{t("tools.idempotency")}</dt><dd>{activeTool.idempotency_mode}</dd></div></dl><div className="private-default-note"><ShieldCheck />{t("tools.thisContractIsSourceManagedExecutionRequiresTheActive")}</div></> : <dl className="entity-detail-grid"><div><dt>{t("tools.upstreamTool")}</dt><dd>{activeTool.upstream_tool_name}</dd></div><div><dt>{t("tools.schemaHash")}</dt><dd>{activeTool.upstream_schema_hash}</dd></div></dl>}
      </section>
      <aside className="entity-workspace-rail">{connection ? <section className="panel entity-related-panel"><PanelHeader title={t("tools.connection")} /><ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><Share2 /></span><span><strong>{connection.name}</strong><small>{connection.protocol_version} · {connection.auth_mode}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><ChevronRight /></ConsoleLink></section> : activeTool.runtime_service_connection_id && owningIntegration ? <section className="panel entity-related-panel"><PanelHeader title="API Authorization" /><ConsoleLink path={integrationPath(owningIntegration.id, "authorization")} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><KeyRound /></span><span><strong>{runtimeConnection?.name ?? t("tools.serviceConnection")}</strong><small>Endpoint and credential are managed by Authorization.</small></span><ChevronRight /></ConsoleLink></section> : <section className="panel entity-detail-panel"><PanelHeader title={activeTool.backend_kind === "native" ? t("tools.trustedSourceBoundary") : activeTool.backend_kind === "mcp" ? t("tools.connectionModel") : t("tools.httpSecurityBoundary")} /><p className="entity-panel-copy">{activeTool.backend_kind === "native" ? t("tools.thisToolExecutesTrustedSourceCompiledIntoDokoSokoAnd") : activeTool.backend_kind === "mcp" ? t("tools.thisImportedToolUsesItsReviewedMCPConnection") : t("tools.isAppliedServerSideAtExecutionTimeToolResponses", { label: String(upstreamAuth.label) })}</p></section>}</aside>
    </div>}

    {activeTab === "authorization" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-authorization" aria-labelledby="tool-tab-authorization" tabIndex={0}><PanelHeader title={t("tools.baselineAuthorization")} description={t("tools.readOnlyExactRevisionAnAPIAuthorizationPointMay")} action={<Badge color={riskColor}>{t("tools.riskValue", { risk })}</Badge>} /><label className="auth-field"><span>{t("tools.requiredRegisteredGrants")}</span><input readOnly value={grants} placeholder={t("tools.noRegisteredGrants")} /></label><div className="two-fields"><label className="auth-field"><span>{t("tools.risk")}</span><input value={risk} readOnly /></label></div><dl className="entity-detail-grid compact-detail-grid readonly-policy"><div><dt>{t("tools.explicitConfirmation")}</dt><dd>{confirmationRequired || risk === "critical" ? t("tools.required") : t("tools.notRequired")}</dd></div><div><dt>{t("tools.idempotencyMetadata")}</dt><dd>{idempotencyRequired ? t("tools.required") : t("tools.notRequired")}</dd></div></dl>{currentPolicy.requiredGrants.length > 0 && <div className="entity-grant-list">{currentPolicy.requiredGrants.map((grant) => <code key={grant}>{grant}</code>)}</div>}</section>}

    {activeTab === "tests" && <div className="tool-tests-workspace" role="tabpanel" id="tool-panel-tests" aria-labelledby="tool-tab-tests" tabIndex={0}>
      <section className="panel tool-editor-page tool-test-stage">
        <PanelHeader title={t("tools.contractCheck")} description={t("tools.stageN1ValidateTheArgumentsSchemaFixedDestinationAnd")} action={<Button outline disabled={busy || contractCheckBusy || liveTestBusy} onClick={dryRunTool}>{contractCheckBusy ? t("tools.checking") : t("tools.runContractCheck")}</Button>} />
        <label className="auth-field" htmlFor="tool-test-arguments"><span>{t("tools.jsonArguments")}</span><textarea id="tool-test-arguments" className="code-input" spellCheck={false} value={testInput} disabled={contractCheckBusy || liveTestBusy || testConfirmationOpen} onChange={(event) => { testFormVersionRef.current += 1; setTestInput(event.target.value); setTestResult(null); setValidatedTestInput(null); setContractCheckError(""); setLiveTestResult(null); setLiveTestError(""); }} /><small>{t("tools.changingTheseArgumentsInvalidatesTheContractCheckAndAny")}</small></label>
        {contractCheckError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("tools.contractCheckDidNotPass")}</strong><small>{contractCheckError}</small></span></div>}
        {testResult && <pre role="status" aria-live="polite" className={`tool-test-result ${contractCheckPassed ? "passed" : "failed"}`}>{JSON.stringify(testResult, null, 2)}</pre>}
      </section>

      <section className="panel tool-editor-page tool-test-stage" aria-busy={liveTestBusy}>
        <PanelHeader title={t("tools.liveUpstreamTest")} description={delegatedOAuthLiveTest ? t("tools.stageN2UnavailableForDelegatedOAuthAdministratorLiveTests") : liveTestStageDescription} action={!liveTestUnsupported && <Button color="indigo" disabled={busy || contractCheckBusy || liveTestBusy || Boolean(liveTestLimitation) || !testIdempotencyValid} onClick={beginLiveToolTest}>{liveTestBusy ? t("tools.runningLiveTest") : delegatedOAuthLiveTest ? t("tools.liveTestUnavailable") : testConfirmationRequired ? t("tools.reviewRunLiveTest") : t("tools.runLiveUpstreamTest")}</Button>} />
        {liveTestLimitation && <div className="capability-unavailable"><TriangleAlert /><span><strong>{t("tools.liveTestUnavailable")}</strong><small>{liveTestLimitation}</small></span></div>}
        {testIdempotencyRequired && !liveTestUnsupported && !delegatedOAuthLiveTest && <label className="auth-field" htmlFor="tool-test-idempotency-key"><span>{t("tools.idempotencyKey")}</span><input id="tool-test-idempotency-key" autoComplete="off" minLength={16} maxLength={200} disabled={liveTestBusy || testConfirmationOpen} aria-invalid={Boolean(testIdempotencyKey) && !testIdempotencyValid} aria-describedby="tool-test-idempotency-guidance" value={testIdempotencyKey} onChange={(event) => { testFormVersionRef.current += 1; setTestIdempotencyKey(event.target.value); setLiveTestResult(null); setLiveTestError(""); }} /><small id="tool-test-idempotency-guidance">{t("tools.requiredForEveryMutationLiveTestUseN16N200")}</small></label>}
        {liveTestError && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("tools.liveUpstreamTestCouldNotComplete")}</strong><small>{liveTestError}</small></span></div>}
        {!liveTestResult && !liveTestError && !liveTestLimitation && <div className="private-default-note"><ShieldCheck />{t("tools.theServerRetainsOnlyStatusTimingByteCountStructural")}</div>}
        {liveTestResult && <><ToolLiveTestEvidence run={liveTestResult} /><ToolLiveTestAnalysis key={liveTestResult.id} run={liveTestResult} tool={activeTool} onOpenBuilder={(proposal) => onReviewProposal(activeTool, proposal)} onClone={(proposal) => openCloneTool(proposal)} onMessage={onMessage} /></>}
      </section>
    </div>}

    {activeTab === "usage" && <section className="panel" role="tabpanel" id="tool-panel-usage" aria-labelledby="tool-tab-usage" tabIndex={0}><PanelHeader title={t("tools.currentAPIConfiguration")} description={t("tools.eachCurrentAPIDraftPinsAnExactPublishedTool")} action={<Badge color="violet">{usageStatus === "loading" ? "…" : usages.length}</Badge>} />{usageStatus === "partial" && <div className="capability-unavailable"><TriangleAlert /><span><strong>{t("tools.someAPIBindingsCouldNotBeLoaded")}</strong><small>{t("tools.theListBelowMayBeIncomplete")}</small></span></div>}{usages.map(({ integration, binding }) => { const point = binding.authorization_point; const current = binding.tool_revision === activeTool.revision && activeTool.state === "published" && !activeTool.upstream_drifted && Boolean(point && point.state === "active" && point.revision === binding.authorization_point_revision); return <ConsoleLink key={`${integration.id}:${binding.tool_revision}`} path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key} {t("tools.toolR")}{binding.tool_revision} {t("tools.authorizationR")}{binding.authorization_point_revision}</small></span><Badge color={current ? "green" : "red"}>{current ? t("tools.current") : t("tools.staleUnresolved")}</Badge><ChevronRight /></ConsoleLink>; })}{usageStatus === "loading" && <div className="empty-row">{t("tools.loadingCurrentAPIBindings")}</div>}{usageStatus === "ready" && usages.length === 0 && <div className="empty-row">{t("tools.noCurrentAPIConfigurationBindsThisTool")}</div>}</section>}

    {activeTab === "history" && <section className="panel" role="tabpanel" id="tool-panel-history" aria-labelledby="tool-tab-history" tabIndex={0}><PanelHeader title={t("tools.toolActivity")} description={t("tools.appendOnlyAdministrativeAndExecutionEventsLoadedForThis")} action={<ConsoleLink path={sectionPath("reporting")} onNavigate={onNavigate} className="entity-back-link">{t("tools.openAudit")}</ConsoleLink>} />{toolEvents.map((event) => <div className="lease-row" key={event.id}><span><strong>{event.action}</strong><small>{event.actor_id || t("tools.system")} · {event.request_id || t("tools.noRequestID")}</small></span><time>{t("format.dateTime", { value: new Date(event.created_at) })}</time></div>)}{toolEvents.length === 0 && <div className="empty-row">{t("tools.noLoadedActivityForThisTool")}</div>}</section>}

    {testConfirmationRequired && !liveTestUnsupported && !delegatedOAuthLiveTest && <Dialog open={testConfirmationOpen} onClose={(open) => { if (liveTestBusy) return; setTestConfirmationOpen(open); if (!open) { setPendingTestArguments(null); pendingTestVersionRef.current = 0; pendingTestIdempotencyKeyRef.current = ""; setTestConfirmationName(""); setTestSideEffectsAcknowledged(false); } }} title={t("tools.confirmLiveTest", { normalizedTestMethod: String(normalizedTestMethod) })} description={liveTestConfirmationDescription} actions={<><Button outline disabled={liveTestBusy} onClick={() => { setTestConfirmationOpen(false); setPendingTestArguments(null); pendingTestVersionRef.current = 0; pendingTestIdempotencyKeyRef.current = ""; setTestConfirmationName(""); setTestSideEffectsAcknowledged(false); }}>{t("common.cancel")}</Button><Button color="red" disabled={liveTestBusy || !pendingTestArguments || testConfirmationName !== fullToolName || !testSideEffectsAcknowledged || !testIdempotencyValid} onClick={confirmAndRunLiveToolTest}>{liveTestBusy ? t("tools.confirmingRunning") : t("tools.confirmRunNow")}</Button></>}>
      <div className="auth-form compact-form">
        <div className="capability-unavailable"><TriangleAlert /><span><strong>{t("tools.thisIsNotASimulation")}</strong><small>{liveTestRequestDescription}</small></span></div>
        <label className="auth-field" htmlFor="tool-test-confirm-name"><span>{t("tools.typeTheFullToolName")}</span><input id="tool-test-confirm-name" autoComplete="off" aria-invalid={Boolean(testConfirmationName) && testConfirmationName !== fullToolName} aria-describedby="tool-test-confirm-name-guidance" value={testConfirmationName} onChange={(event) => setTestConfirmationName(event.target.value)} /><small id="tool-test-confirm-name-guidance">{t("tools.type")} <code>{fullToolName}</code> {t("tools.exactlyToConfirmRevision")} {currentTool.revision}.</small></label>
        <label className="compact-check"><input type="checkbox" checked={testSideEffectsAcknowledged} onChange={(event) => setTestSideEffectsAcknowledged(event.target.checked)} /><span>{liveTestAcknowledgement}</span></label>
        <div className="private-default-note"><LockKeyhole />{t("tools.confirmationCreatesAShortLivedSingleUseNonceBound")}</div>
      </div>
    </Dialog>}
    {backendKind === "http" && !apiOwned && <Dialog open={cloneOpen} onClose={(open) => { setCloneOpen(open); if (!open) { setCloneCredential(""); setPendingCloneProposal(null); } }} title={t("tools.cloneAsANewTool")} description={pendingCloneProposal ? t("tools.chooseADistinctLowerCaseIdentityTheIndependentDraft") : t("tools.chooseADistinctLowerCaseIdentityStoredCredentialsAre")} actions={<><Button outline onClick={() => { setCloneOpen(false); setCloneCredential(""); setPendingCloneProposal(null); }}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !cloneIdentityValid || (upstreamAuth.credentialRequired && !cloneCredential)} onClick={cloneTool}>{busy ? t("tools.cloning") : pendingCloneProposal ? t("tools.createDraftReview") : t("tools.createDraft")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("tools.namespace")}</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneNamespace) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneNamespace.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneNamespace} onChange={(event) => setCloneNamespace(event.target.value)} /></label><label className="auth-field"><span>{t("tools.name")}</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneName) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneName.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneName} onChange={(event) => setCloneName(event.target.value)} /></label></div><small id="clone-tool-identity-guidance">{t("tools.useN1N64LowerCaseLettersNumbersOrUnderscores")}</small>{upstreamAuth.credentialRequired && <label className="auth-field"><span>{cloneCredentialLabel}</span><input type="password" autoComplete="new-password" value={cloneCredential} onChange={(event) => setCloneCredential(event.target.value)} /><small>{t("tools.requiredFor")} {upstreamAuth.label}{t("tools.enterANewValueBecauseTheSourceCredentialIs")}</small></label>}<div className="private-default-note"><KeyRound />{t("tools.theCloneReceivesTheNonSecretContractOnlyDelegated")} {pendingCloneProposal ? t("tools.theProposalRemainsAnInMemoryReviewSeedAnd") : ""}</div></div></Dialog>}
    <Dialog open={retireOpen} onClose={setRetireOpen} title={t("tools.retire2", { namespace: String(activeTool.namespace), name: String(activeTool.name) })} description={t("tools.retirementRemovesTheToolFromDiscoveryAndPreventsNew")} actions={<><Button outline onClick={() => setRetireOpen(false)}>{t("common.cancel")}</Button><Button color="red" disabled={busy} onClick={retireTool}>{busy ? t("tools.retiring") : t("tools.retireTool")}</Button></>}><div className="private-default-note"><TriangleAlert />{t("tools.thisChangesTheDeploymentCatalogueImmediatelyExistingPublishedAPI")}</div></Dialog>
  </>;
}

export function ConsoleNotFoundView({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">{t("tools.navigation")}</p><h1>{t("tools.pageNotFound")}</h1><p><code>{path}</code> {t("tools.isNotARecognisedConsoleURL")}</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("tools.returnToAPIs")}</ConsoleLink></section>;
}
