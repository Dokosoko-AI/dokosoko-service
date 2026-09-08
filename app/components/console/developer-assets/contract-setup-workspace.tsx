"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APICrawlJob, type APIDeployment, type APIIntegration, type APISource, type APISourceReview } from "../../../lib/api";
import { contractAttachmentVisibility, contractBindingMatches, contractCandidateValid, contractSetupPath, pendingContractImport, ContractSetupChangedError, finishContractSetup, type ContractSetupSelection } from "../../../lib/contract-setup";
import { contractReviewChoiceScope, validContractReviewChoice } from "../../../lib/contract-review-choice";
import { integrationPath, sectionPath } from "../../../lib/console-routes";
import { browserReviewStorage, clearReviewDraft, readReviewDraft, reviewDraftKey, writeReviewDraft } from "../../../lib/review-draft";
import { finishSourceCreationAttempt, sourceCreationAttempt, sourceCreationFingerprint, type SourceCreationAttempt } from "../../../lib/source-creation-request";
import { developerAssetsApi, type APIContract, type APIContractCandidateRecord, type APIContractRevision, type APIContractSource, type APIResourceBindings } from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader } from "../../core/layout";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { SourceReviewDocument } from "./source-review-document";
import { ContractReviewContent } from "./contract-review-content";
import { developerAssetError, LoadingPanel, PrettyJSON, ProblemPanel } from "./developer-asset-ui";
import { KnowledgeProcessingPanel } from "./knowledge-processing-panel";

type SetupData = {
  deployment: APIDeployment; contract: APIContract; origin?: APIIntegration;
  sources: APISource[]; bindings: APIContractSource[]; resources?: APIResourceBindings;
  source?: APISource; jobs: APICrawlJob[]; job?: APICrawlJob; review?: APISourceReview;
  candidates: APIContractCandidateRecord["candidate"][]; record?: APIContractCandidateRecord;
  revisions: APIContractRevision[]; published?: APIContractRevision;
  previous?: APIContractCandidateRecord; previousRevision?: number;
};

function validProgress(value: unknown): value is ContractSetupSelection {
  return !!value && typeof value === "object" && ["contract", "api", "source", "run", "candidate", "revision", "input", "queue", "after"].every((key) => typeof (value as Record<string, unknown>)[key] === "string" && String((value as Record<string, unknown>)[key]).length <= 200);
}

export function ContractSetupWorkspace({ selection, reviewerID = "", onNavigate, onMessage }: { selection: ContractSetupSelection; reviewerID?: string; onNavigate: (path: string) => void; onMessage: (message: string) => void }) {
  const { t } = useTranslation();
  const [data, setData] = useState<SetupData | null>(null);
  const [problem, setProblem] = useState("");
  const [actionProblem, setActionProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const sourceCreation = useRef<SourceCreationAttempt | null>(null);
  const [sourceRecoveryUnavailable, setSourceRecoveryUnavailable] = useState(false);
  const [reviewRecoveryUnavailable, setReviewRecoveryUnavailable] = useState(false);
  const reviewChoice = useRef<{ key: string; fingerprint: string; primary: boolean } | null>(null);
  const [mode, setMode] = useState<"url" | "upload" | "existing">("url");
  const [location, setLocation] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [existingID, setExistingID] = useState("");
  const [publicAcknowledged, setPublicAcknowledged] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [primary, setPrimary] = useState(true);
  const [processingReadyRunID, setProcessingReadyRunID] = useState("");
  const mounted = useRef(false);
  const progress = useRef(selection);
  const requestID = useRef(0);
  const progressKey = useRef("");
  const fingerprint = `${selection.contract}:${selection.api}`;

  const invalidateRequest = useCallback(() => { requestID.current++; }, []);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; invalidateRequest(); }; }, [invalidateRequest]);
  useEffect(() => { progress.current = selection; }, [selection]);

  function remember(next: Partial<ContractSetupSelection>, deploymentID?: string, navigate = true) {
    const value = { ...progress.current, ...next };
    progress.current = value;
    const key = deploymentID ? reviewDraftKey(deploymentID, reviewerID, "contract-setup", fingerprint) : progressKey.current;
    // Progress contains only exact object identities. It cannot acknowledge a review.
    writeReviewDraft(browserReviewStorage, key, "contract-setup-v1", value);
    if (navigate && mounted.current) onNavigate(contractSetupPath(value));
  }

  const load = useCallback(async () => {
    const request = ++requestID.current;
    setLoading(true); setProblem(""); setAcknowledged(false); setProcessingReadyRunID("");
    try {
      const deployment = await api.deployment();
      const [contract, sources, bindings, candidates, revisions, runs, origin, resources] = await Promise.all([
        developerAssetsApi.apiContract(selection.contract), api.sources(deployment.id),
        developerAssetsApi.apiContractSources(selection.contract), developerAssetsApi.apiContractCandidates(selection.contract), developerAssetsApi.apiContractRevisions(selection.contract),
        developerAssetsApi.ingestionRuns("contract", `contract:${selection.contract}`),
        selection.api ? api.integration(selection.api).then((value) => value.integration) : undefined,
        selection.api ? developerAssetsApi.apiResources(selection.api) : undefined,
      ]);
      if (request !== requestID.current || !mounted.current) return;
      if (contract.lifecycle !== "active" || contract.deployment_id !== deployment.id || (origin && origin.deployment_id !== deployment.id)) throw new ContractSetupChangedError();
      progressKey.current = reviewDraftKey(deployment.id, reviewerID, "contract-setup", fingerprint);
      const saved = readReviewDraft(browserReviewStorage, progressKey.current, "contract-setup-v1", validProgress);
      if (selection.input !== "new" && !selection.source && !selection.run && !selection.candidate && saved && saved.contract === selection.contract && saved.api === selection.api && (saved.source || saved.candidate)) {
        onNavigate(contractSetupPath(saved)); return;
      }
      const activeBindings = bindings.filter((binding) => binding.lifecycle === "attached");
      let candidate = selection.candidate ? candidates.find((value) => value.id === selection.candidate) : undefined;
      if (selection.candidate && !candidate) throw new ContractSetupChangedError();
      const candidateRun = candidate ? runs.find((run) => run.id === candidate?.ingestion_run_id) : undefined;
      const sourceID = selection.input === "new" ? "" : selection.source || candidateRun?.source_id || activeBindings.find((binding) => binding.source_role === "primary")?.source_id || activeBindings[0]?.source_id;
      const source = sources.find((value) => value.id === sourceID);
      if (sourceID && !source) throw new ContractSetupChangedError();
      const jobs = source ? await api.crawlJobs(deployment.id, source.id) : [];
      const pending = pendingContractImport(jobs, selection);
      const runID = selection.input === "new" ? "" : selection.run || candidate?.ingestion_run_id || (selection.queue === "pending" ? pending?.id : jobs[0]?.id) || "";
      candidate ??= candidates.find((value) => value.ingestion_run_id === runID);
      const run = candidate ? runs.find((value) => value.id === candidate.ingestion_run_id) : undefined;
      if (candidate && (candidate.api_contract_id !== contract.id || (!run || run.source_id !== sourceID) || candidate.ingestion_run_id !== runID)) throw new ContractSetupChangedError();
      const job = jobs.find((value) => value.id === runID);
      if (selection.run && !job) throw new ContractSetupChangedError();
      const [record, review] = await Promise.all([
        candidate ? developerAssetsApi.apiContractCandidate(contract.id, candidate.id) : undefined,
        source && job && ["review", "succeeded"].includes(job.state) ? api.sourceReview(deployment.id, source.id, job.id) : undefined,
      ]);
      const published = revisions.find((revision) => revision.api_contract_candidate_id === candidate?.id && revision.content_hash === candidate?.content_hash);
      if (selection.revision && selection.revision !== published?.id) throw new ContractSetupChangedError();
      const older = candidate ? revisions.filter((revision) => {
        const prior = candidates.find((value) => value.id === revision.api_contract_candidate_id);
        return prior && prior.id !== candidate.id && prior.created_at < candidate.created_at;
      }).sort((left, right) => right.revision - left.revision)[0] : undefined;
      const previous = older ? await developerAssetsApi.apiContractCandidate(contract.id, older.api_contract_candidate_id) : undefined;
      if (request !== requestID.current || !mounted.current) return;
      setData({ deployment, contract, sources, bindings: activeBindings, candidates, revisions, origin, resources, source, jobs, job, record, review, published, previous, previousRevision: older?.revision });
      const currentBinding = resources?.contracts.find((binding) => binding.lifecycle === "attached" && binding.api_contract_id === contract.id);
      const choiceScope = record && review && origin ? contractReviewChoiceScope({ contract, candidate: record.candidate, sourceReview: review, api: origin }, reviewerID) : undefined;
      const choice = choiceScope ? reviewChoice.current?.key === choiceScope.key && reviewChoice.current.fingerprint === choiceScope.fingerprint
        ? reviewChoice.current : readReviewDraft(browserReviewStorage, choiceScope.key, choiceScope.fingerprint, validContractReviewChoice) : null;
      const intendedPrimary = choice?.primary ?? currentBinding?.primary ?? !resources?.contracts.some((binding) => binding.lifecycle === "attached" && binding.primary);
      setPrimary(intendedPrimary);
      if (published && (!origin || contractBindingMatches(currentBinding, published, origin, intendedPrimary, contractAttachmentVisibility(origin, currentBinding)))) setActionProblem("");
    } catch (error) { if (request === requestID.current && mounted.current) setProblem(error instanceof ContractSetupChangedError ? t("contractSetup.changedSelection") : developerAssetError(error, t("contractSetup.loadFailed"))); }
    finally { if (request === requestID.current && mounted.current) setLoading(false); }
  }, [fingerprint, onNavigate, reviewerID, selection, t]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => { window.clearTimeout(timer); invalidateRequest(); }; }, [attempt, invalidateRequest, load]);
  useEffect(() => {
    if (!data?.job || !["queued", "running"].includes(data.job.state)) return;
    const timer = window.setTimeout(() => setAttempt((value) => value + 1), 3000);
    return () => window.clearTimeout(timer);
  }, [data?.job, attempt]);

  function choosePrimary(value: boolean) {
    setPrimary(value); setAcknowledged(false);
    if (!data?.record || !data.review || !data.origin) return;
    const scope = contractReviewChoiceScope({ contract: data.contract, candidate: data.record.candidate, sourceReview: data.review, api: data.origin }, reviewerID);
    reviewChoice.current = { ...scope, primary: value };
    setReviewRecoveryUnavailable(!scope.key || !writeReviewDraft(browserReviewStorage, scope.key, scope.fingerprint, { primary: value }));
  }

  async function startImport() {
    if (!data || busy || (data.contract.visibility === "public" && !publicAcknowledged)) return;
    setBusy(true); setActionProblem("");
    try {
      const rememberedSourceID = progress.current.source || data.source?.id;
      let source = rememberedSourceID ? (await api.sources(data.deployment.id)).find((value) => value.id === rememberedSourceID) : undefined;
      if (rememberedSourceID && !source) throw new ContractSetupChangedError();
      if (!source) {
        const requestScope = reviewDraftKey(data.deployment.id, reviewerID, "source-creation", data.contract.id);
        if (mode !== "existing") {
          const fingerprint = await sourceCreationFingerprint({ kind: mode === "upload" ? "upload" : "openapi", location, file: mode === "upload" ? file ?? undefined : undefined });
          const { attempt, persisted } = sourceCreationAttempt(browserReviewStorage, requestScope, fingerprint, sourceCreation.current);
          sourceCreation.current = attempt; setSourceRecoveryUnavailable(!persisted);
        }
        source = mode === "existing" ? data.sources.find((value) => value.id === existingID) : mode === "upload" && file ? await api.uploadSource(data.deployment.id, data.deployment.organisation_id, file, undefined, sourceCreation.current?.key) : mode === "url" && location.trim() ? await api.createSource(data.deployment.id, data.deployment.organisation_id, "openapi", location.trim(), undefined, sourceCreation.current?.key) : undefined;
        if (!source) throw new Error(t("contractSetup.chooseInput"));
        remember({ source: source.id, run: "", candidate: "", revision: "", input: "", queue: "", after: "" }, data.deployment.id);
        finishSourceCreationAttempt(browserReviewStorage, requestScope); sourceCreation.current = null;
      }
      if (!mounted.current) return;
      if (!["openapi", "upload"].includes(source.kind)) throw new Error(t("contractSetup.supportedInput"));
      if (data.contract.visibility === "public" && source.visibility !== "public") source = await api.setSourceVisibility(data.deployment.id, source.id, "public", source.revision, true);
      const bindings = await developerAssetsApi.apiContractSources(data.contract.id);
      if (!bindings.some((binding) => binding.source_id === source.id && binding.lifecycle === "attached")) await developerAssetsApi.attachAPIContractSource(data.contract.id, source.id, bindings.some((binding) => binding.lifecycle === "attached" && binding.source_role === "primary") ? "supplemental" : "primary");
      if (!mounted.current) return;
      const jobs = await api.crawlJobs(data.deployment.id, source.id);
      // Persist the server's last job before queuing. If the response is lost,
      // reload can find the new job even after it finishes processing.
      let job = pendingContractImport(jobs, progress.current) || jobs.find((value) => ["queued", "running"].includes(value.state));
      if (!job) {
        remember({ source: source.id, input: "", run: "", candidate: "", revision: "", queue: "pending", after: jobs[0]?.id ?? "" }, data.deployment.id);
        job = await api.queueCrawl(data.deployment.id, source.id);
      }
      remember({ source: source.id, run: job.id, candidate: "", revision: "", input: "", queue: "", after: "" }, data.deployment.id);
      if (mounted.current) setAttempt((value) => value + 1);
    } catch (error) { if (mounted.current) setActionProblem(developerAssetError(error, t("contractSetup.importFailed"))); }
    finally { if (mounted.current) setBusy(false); }
  }

  async function finish() {
    if (!data?.record || !data.review || !acknowledged || busy) return;
    setBusy(true); setActionProblem("");
    try {
      const revision = await finishContractSetup({ contract: data.contract, candidate: data.record.candidate, sourceReview: data.review, api: data.origin, primary, expectedBinding: data.resources?.contracts.find((binding) => binding.lifecycle === "attached" && binding.api_contract_id === data.contract.id) }, {
        integration: (id) => api.integration(id).then((value) => value.integration), contract: developerAssetsApi.apiContract, revisions: developerAssetsApi.apiContractRevisions, review: api.sourceReview,
        publishSource: (review) => api.publishSource(data.deployment.id, review.source.id, { revision: review.source.revision, crawl_job_id: review.crawl_job.id, document_ids: review.documents.map((document) => document.id), acknowledge_reviewed: true }),
        publishContract: developerAssetsApi.publishAPIContractCandidate, resources: developerAssetsApi.apiResources,
        attach: (apiID, contractID, revisionID, isPrimary, visibility) => developerAssetsApi.attachAPIContract(apiID, { api_contract_id: contractID, pinned_revision_id: revisionID, primary: isPrimary, visibility }),
        change: (apiID, binding, revisionID, isPrimary, visibility) => developerAssetsApi.changeAPIContract(apiID, binding.id, { api_contract_id: binding.api_contract_id, pinned_revision_id: revisionID, primary: isPrimary, visibility, revision: binding.revision }),
      }, (revision) => remember({ source: data.review!.source.id, run: data.record!.candidate.ingestion_run_id, candidate: data.record!.candidate.id, revision: revision.id, input: "", queue: "", after: "" }, data.deployment.id, false));
      if (data.origin) clearReviewDraft(browserReviewStorage, contractReviewChoiceScope({ contract: data.contract, candidate: data.record.candidate, sourceReview: data.review, api: data.origin }, reviewerID).key);
      reviewChoice.current = null;
      if (mounted.current) { remember({ revision: revision.id }, data.deployment.id); setAcknowledged(false); onMessage(t("contractSetup.finished", { revision: revision.revision })); setAttempt((value) => value + 1); }
    } catch (error) { if (mounted.current) { setActionProblem(error instanceof ContractSetupChangedError ? t("contractSetup.changedSelection") : developerAssetError(error, t("contractSetup.finishFailed"))); setAttempt((value) => value + 1); } }
    finally { if (mounted.current) setBusy(false); }
  }

  const candidate = data?.record?.candidate;
  const eligibleSources = data?.sources.filter((source) => !source.quarantined && ["openapi", "upload"].includes(source.kind)) ?? [];
  const sourceBlocked = Boolean(data?.review && (data.review.source.quarantined || data.review.crawl_job.failed_count > 0 || data.review.crawl_job.skipped_count > 0 || data.review.documents.some((document) => !["validated", "published"].includes(document.state) || document.injection_indicators.length > 0)));
  const audienceBlocked = Boolean(candidate && (candidate.visibility !== data?.source?.visibility || data?.origin?.visibility === "public" && candidate.visibility !== "public"));
  const processingReady = Boolean(data?.published || candidate && processingReadyRunID === candidate.ingestion_run_id);
  const currentBinding = data?.resources?.contracts.find((binding) => binding.lifecycle === "attached" && binding.api_contract_id === data.contract.id);
  const attachmentVisibility = data?.origin ? contractAttachmentVisibility(data.origin, currentBinding) : undefined;
  const complete = Boolean(data?.published && (!data.origin || contractBindingMatches(currentBinding, data.published, data.origin, primary, attachmentVisibility!)));
  return <>
    <PageHeader eyebrow={t("navigation.apiContracts")} title={data?.contract.name ?? t("contractSetup.title")} description={data?.origin ? t("contractSetup.forAPI", { name: data.origin.display_name }) : t("contractSetup.description")} action={<Button outline onClick={() => onNavigate(selection.api ? integrationPath(selection.api, "documentation") : sectionPath("contracts"))}>{t(selection.api ? "contractSetup.backAPI" : "contractSetup.backCatalog")}</Button>} />
    {loading ? <LoadingPanel label={t("contractSetup.loading")} /> : problem ? <ProblemPanel message={problem} onRetry={() => setAttempt((value) => value + 1)} /> : data && <>
      {actionProblem && <div className="notice" role="alert"><p>{actionProblem}</p><Button outline onClick={() => setAttempt((value) => value + 1)}>{t("common.retry")}</Button></div>}
      {sourceRecoveryUnavailable && <p role="status">{t("sourceCreation.recoveryUnavailable")}</p>}
      {!data.record && <AIReadinessPanel />}
      <section className="panel contract-setup-panel">
        <PanelHeader title={t("contractSetup.importTitle")} description={t("contractSetup.importDescription")} action={<Badge>{data.contract.visibility}</Badge>} />
        {data.source ? <><p><strong>{data.source.name}</strong> · {data.source.kind}</p><p>{data.job ? t("contractSetup.importState", { state: data.job.state }) : t("contractSetup.readyToImport")}</p>{data.job && <details><summary>{t("common.diagnostics")}</summary><PrettyJSON value={data.job.diagnostics} /></details>}<div className="heading-actions">{(!data.record && !["queued", "running"].includes(data.job?.state ?? "")) && <Button disabled={busy || (data.contract.visibility === "public" && !publicAcknowledged)} onClick={() => void startImport()}>{t("contractSetup.startResumeImport")}</Button>}<Button outline disabled={busy} onClick={() => { remember({ source: "", run: "", candidate: "", revision: "", input: "new", queue: "", after: "" }); setExistingID(""); setFile(null); setLocation(""); setPublicAcknowledged(false); }}>{t("contractSetup.changeInput")}</Button></div></> : <div className="auth-form compact-form">
          <label className="auth-field"><span id="contract-input-type-label">{t("contractSetup.input")}</span><select aria-labelledby="contract-input-type-label" value={mode} disabled={busy} onChange={(event) => { setMode(event.target.value as typeof mode); setPublicAcknowledged(false); }}><option value="url">{t("contractSetup.url")}</option><option value="upload">{t("contractSetup.upload")}</option><option value="existing">{t("contractSetup.existing")}</option></select></label>
          {mode === "url" ? <label className="auth-field"><span>{t("contractSetup.url")}</span><input type="url" value={location} disabled={busy} onChange={(event) => { setLocation(event.target.value); setPublicAcknowledged(false); }} placeholder="https://api.example.com/openapi.json" /></label> : mode === "upload" ? <label className="auth-field"><span>{t("contractSetup.upload")}</span><input type="file" accept=".json,.yaml,.yml" disabled={busy} onChange={(event) => { setFile(event.target.files?.[0] ?? null); setPublicAcknowledged(false); }} /></label> : <label className="auth-field"><span id="contract-existing-source-label">{t("contractSetup.existing")}</span><select aria-labelledby="contract-existing-source-label" value={existingID} disabled={busy} onChange={(event) => { setExistingID(event.target.value); setPublicAcknowledged(false); }}><option value="">{t("apiContracts.selectASource")}</option>{eligibleSources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}</select></label>}
          <Button disabled={busy || (mode === "url" ? !location.trim() : mode === "upload" ? !file : !existingID) || (data.contract.visibility === "public" && !publicAcknowledged)} onClick={() => void startImport()}>{busy ? t("common.saving") : t("contractSetup.startImport")}</Button>
        </div>}
        {data.contract.visibility === "public" && !data.record && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("contractSetup.publicImport")}</span></label>}
      </section>
      {data.candidates.length > 0 && <label className="auth-field"><span id="contract-import-version-label">{t("contractSetup.importVersion")}</span><select aria-labelledby="contract-import-version-label" value={candidate?.id ?? ""} disabled={busy} onChange={(event) => { const value = data.candidates.find((item) => item.id === event.target.value); if (value) remember({ source: "", run: value.ingestion_run_id, candidate: value.id, revision: "", input: "", queue: "", after: "" }); }}><option value="">{t("contractSetup.chooseImport")}</option>{data.candidates.map((value) => <option key={value.id} value={value.id}>{t("format.dateTime", { value: new Date(value.created_at) })} · OpenAPI {value.openapi_version}</option>)}</select></label>}
      {data.record && candidate && <section className="panel contract-setup-panel">
        <PanelHeader title={t("contractSetup.reviewTitle")} description={t("contractSetup.reviewDescription")} action={<Badge color={data.published ? "green" : "amber"}>{data.published ? t("contractSetup.published", { revision: data.published.revision }) : t("sourceDialogs.needsReview")}</Badge>} />
        {data.published ? <details><summary>{t("contractSetup.processingEvidence")}</summary><KnowledgeProcessingPanel key={candidate.ingestion_run_id} runID={candidate.ingestion_run_id} onReady={setProcessingReadyRunID} /></details> : <KnowledgeProcessingPanel key={candidate.ingestion_run_id} runID={candidate.ingestion_run_id} onReady={setProcessingReadyRunID} />}
        <ContractReviewContent key={candidate.id} record={data.record} previous={data.previous} previousRevision={data.previousRevision} />
        {data.review && <details><summary>{t("contractSetup.sourceContent")}</summary>{data.review.documents.map((document) => <SourceReviewDocument key={document.id} productID={data.deployment.id} sourceID={data.review!.source.id} document={document} selected disabled onSelected={() => undefined} publishedDecision={data.review!.publication ? data.review!.published_document_ids?.includes(document.id) ? "included" : "excluded" : undefined} />)}</details>}
        {sourceBlocked && <p className="auth-problem">{t("contractSetup.sourceBlocked")}</p>}
        {audienceBlocked && <p className="auth-problem">{t("contractSetup.audienceBlocked")}</p>}
        {!contractCandidateValid(candidate) && <p className="auth-problem">{t("contractSetup.invalidContract")}</p>}
        {!data.review && <p className="auth-problem">{t("contractSetup.sourceReviewMissing")}</p>}
        {!complete && <><p>{t(data.published ? "contractSetup.publishedScope" : "contractSetup.publishScope", { audience: candidate.visibility })}</p>{data.origin && <><p>{t("contractSetup.attachScope", { name: data.origin.display_name, audience: t(attachmentVisibility === "public" ? "common.public" : "common.private") })}</p><label className="compact-check"><input type="checkbox" checked={primary} disabled={busy} onChange={(event) => choosePrimary(event.target.checked)} /><span>{t("apiResources.useAsThisAPISPrimaryContract")}</span></label><p role={reviewRecoveryUnavailable ? "status" : undefined}>{t(reviewRecoveryUnavailable ? "sdkCatalog.reviewDraftNotSaved" : "sdkCatalog.reviewDraftDescription")}</p></>}<label className="compact-check"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("contractSetup.acknowledge")}</span></label><Button disabled={busy || sourceBlocked || audienceBlocked || !acknowledged || !processingReady || !data.review || !contractCandidateValid(candidate)} onClick={() => void finish()}>{busy ? t("common.saving") : data.published ? t("contractSetup.attachExact") : data.origin ? t("contractSetup.publishAttach") : t("contractSetup.publish")}</Button></>}
        {complete && <div role="status"><p>{t(data.origin ? "contractSetup.attached" : "contractSetup.reviewedPublished")}</p>{data.origin && <Button onClick={() => onNavigate(integrationPath(data.origin!.id))}>{t("contractSetup.reviewAPI")}</Button>}</div>}
      </section>}
    </>}
  </>;
}
