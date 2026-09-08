"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, APIError, type APICrawlJob, type APIDeployment, type APIIntegration, type APISource, type APISourceReview } from "../../../lib/api";
import { integrationPath, sectionPath } from "../../../lib/console-routes";
import { documentationBindingMatches, documentationReviewFingerprint, documentationRevisionMatches, documentationSetupPath, documentationSourceSlug, DocumentationSetupChangedError, finishDocumentationSetup, parseDocumentationSetupSelection, pendingDocumentationImport, validDocumentationProgress, type DocumentationSetupSelection } from "../../../lib/documentation-setup";
import { developerAssetsApi, type APIDocumentationBinding, type DocumentationCollection, type DocumentationCollectionRevision } from "../../../lib/developer-assets-api";
import { browserReviewStorage, readReviewDraft, reviewDraftKey, writeReviewDraft } from "../../../lib/review-draft";
import { finishSourceCreationAttempt, sourceCreationAttempt, sourceCreationFingerprint, type SourceCreationAttempt } from "../../../lib/source-creation-request";
import { finishSourceInputReplacement, readSourceInputReplacement, sourceInputReplacementAttempt, type SourceInputReplacementAttempt } from "../../../lib/source-input-replacement";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader } from "../../core/layout";
import { AIReadinessPanel } from "../ai-readiness-panel";
import { KnowledgeProcessingPanel } from "./knowledge-processing-panel";
import { SourceReviewDocument } from "./source-review-document";
import { developerAssetError, LoadingPanel, PrettyJSON, ProblemPanel } from "./developer-asset-ui";

type SetupData = {
  deployment: APIDeployment; origin?: APIIntegration; sources: APISource[]; source?: APISource;
  jobs: APICrawlJob[]; job?: APICrawlJob; review?: APISourceReview;
  collection?: DocumentationCollection; revision?: DocumentationCollectionRevision; binding?: APIDocumentationBinding;
};

export function DocumentationSetupWorkspace({ selection, reviewerID, onNavigate, onMessage, onSourceChanged }: {
  onSourceChanged?: (source: APISource) => void;
  selection: DocumentationSetupSelection; reviewerID: string; onNavigate: (path: string) => void; onMessage: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [data, setData] = useState<SetupData | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [actionProblem, setActionProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [mode, setMode] = useState<"website" | "upload" | "existing">("website");
  const [location, setLocation] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [replacementInput, setReplacementInput] = useState<{ sourceID: string; file: File } | null>(null);
  const replacementFile = replacementInput?.sourceID === selection.source ? replacementInput.file : null;
  const replacementAttempt = useRef<SourceInputReplacementAttempt | null>(null);
  const [existingID, setExistingID] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [acknowledged, setAcknowledged] = useState(false);
  const [publicAcknowledged, setPublicAcknowledged] = useState(false);
  const [processingReady, setProcessingReady] = useState("");
  const [storageUnavailable, setStorageUnavailable] = useState(false);
  const mounted = useRef(false);
  const request = useRef(0);
  const progress = useRef(selection);
  const sourceCreation = useRef<SourceCreationAttempt | null>(null);
  const actionStage = useRef<"import" | "finish" | "">("");
  const context = selection.api || "library";
  const invalidate = useCallback(() => { request.current++; }, []);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; invalidate(); }; }, [invalidate]);
  useEffect(() => { progress.current = selection; }, [selection]);

  function remember(next: Partial<DocumentationSetupSelection>, deploymentID: string) {
    const value = { ...progress.current, ...next };
    progress.current = value;
    if (!writeReviewDraft(browserReviewStorage, reviewDraftKey(deploymentID, reviewerID, "documentation-setup", context), "documentation-setup-v1", value)) setStorageUnavailable(true);
    if (mounted.current) onNavigate(documentationSetupPath(value));
  }

  const load = useCallback(async () => {
    const current = ++request.current;
    setLoading(true); setProblem(""); setAcknowledged(false); setProcessingReady("");
    try {
      if (!validDocumentationProgress(selection)) throw new DocumentationSetupChangedError();
      const deployment = await api.deployment();
      const [sources, collections, origin, resources] = await Promise.all([
        api.sources(deployment.id), developerAssetsApi.documentationCollections(),
        selection.api ? api.integration(selection.api).then((value) => value.integration) : undefined,
        selection.api ? developerAssetsApi.apiResources(selection.api) : undefined,
      ]);
      if (origin && origin.deployment_id !== deployment.id) throw new DocumentationSetupChangedError();
      const saved = readReviewDraft(browserReviewStorage, reviewDraftKey(deployment.id, reviewerID, "documentation-setup", context), "documentation-setup-v1", validDocumentationProgress);
      if (!selection.source && selection.setup !== "new" && saved?.source && saved.api === selection.api) {
        if (current === request.current && mounted.current) onNavigate(documentationSetupPath(saved));
        return;
      }
      const source = sources.find((value) => value.id === selection.source);
      if (selection.source && (!source || source.product_id !== deployment.id || !["website", "upload"].includes(source.kind))) throw new DocumentationSetupChangedError();
      if (source?.kind === "upload") {
        const replacementScope = reviewDraftKey(deployment.id, reviewerID, "source-input-replacement", source.id);
        const pending = readSourceInputReplacement(browserReviewStorage, replacementScope);
        if (pending) {
          try {
            const recovered = await api.sourceUploadReplacement(deployment.id, source.id, pending.key);
            if (recovered.source.id !== source.id || recovered.source.product_id !== deployment.id || recovered.crawl_job.source_id !== source.id || recovered.crawl_job.product_id !== deployment.id) throw new DocumentationSetupChangedError();
            if (current !== request.current || !mounted.current) return;
            const next = { ...selection, setup: "content", run: recovered.crawl_job.id, queue: "", after: "" };
            writeReviewDraft(browserReviewStorage, reviewDraftKey(deployment.id, reviewerID, "documentation-setup", context), "documentation-setup-v1", next);
            finishSourceInputReplacement(browserReviewStorage, replacementScope); replacementAttempt.current = null; setReplacementInput(null); setActionProblem("");
            if (selection.run !== recovered.crawl_job.id) { onNavigate(documentationSetupPath(next)); return; }
          } catch (error) { if (!(error instanceof APIError && error.status === 404)) throw error; }
        }
      }
      const jobs = source ? await api.crawlJobs(deployment.id, source.id) : [];
      const pending = pendingDocumentationImport(jobs, selection);
      const job = selection.run ? jobs.find((value) => value.id === selection.run) : selection.queue === "pending" ? pending : jobs[0];
      if (selection.run && !job) throw new DocumentationSetupChangedError();
      const review = source && job && ["review", "succeeded"].includes(job.state) ? await api.sourceReview(deployment.id, source.id, job.id) : undefined;
      const collection = source ? collections.find((value) => value.slug === documentationSourceSlug(source.id)) : undefined;
      if (collection?.lifecycle === "archived") throw new DocumentationSetupChangedError();
      const revisions = collection ? await developerAssetsApi.documentationCollectionRevisions(collection.id) : [];
      const revision = review?.publication ? revisions.find((value) => value.documentation_collection_id === collection?.id && documentationRevisionMatches(value, review.publication!)) : undefined;
      const binding = resources?.documentation.find((value) => value.lifecycle === "attached" && value.documentation_collection_id === collection?.id);
      if (current !== request.current || !mounted.current) return;
      setData({ deployment, origin, sources, source, jobs, job, review, collection, revision, binding });
      if (source) onSourceChanged?.(source);
      if (job && actionStage.current === "import") setActionProblem("");
      if (revision && (!origin || documentationBindingMatches(binding, revision, binding?.visibility ?? origin.visibility))) setActionProblem("");
      if (review) {
        const decisions = readReviewDraft(browserReviewStorage, reviewDraftKey(deployment.id, reviewerID, "source", review.crawl_job.id), documentationReviewFingerprint(review), (value): value is string[] => Array.isArray(value) && value.length <= 10000 && value.every((id) => typeof id === "string"));
        const safe = review.documents.filter((document) => ["validated", "published"].includes(document.state) && !document.injection_indicators.length).map((document) => document.id);
        setSelected(review.publication ? review.published_document_ids ?? [] : decisions?.filter((id) => safe.includes(id)) ?? []);
      } else setSelected([]);
      // Pin the automatically discovered import before allowing review actions.
      if (job && (!selection.run || selection.queue)) onNavigate(documentationSetupPath({ ...selection, setup: "content", run: job.id, queue: "", after: "" }));
    } catch (error) {
      if (current === request.current && mounted.current) setProblem(error instanceof DocumentationSetupChangedError ? t("documentationSetup.changed") : developerAssetError(error, t("documentationSetup.loadFailed")));
    } finally { if (current === request.current && mounted.current) setLoading(false); }
  }, [context, onNavigate, onSourceChanged, reviewerID, selection, t]);
  useEffect(() => { const timer = window.setTimeout(() => void load(), 0); return () => { window.clearTimeout(timer); invalidate(); }; }, [attempt, invalidate, load]);
  useEffect(() => {
    if (!data?.job || !["queued", "running"].includes(data.job.state)) return;
    const timer = window.setTimeout(() => setAttempt((value) => value + 1), 3000);
    return () => window.clearTimeout(timer);
  }, [attempt, data?.job]);

  function chooseDocument(id: string, included: boolean) {
    if (!data?.review || data.review.publication) return;
    const next = included ? [...selected, id] : selected.filter((value) => value !== id);
    setSelected(next); setAcknowledged(false);
    if (!writeReviewDraft(browserReviewStorage, reviewDraftKey(data.deployment.id, reviewerID, "source", data.review.crawl_job.id), documentationReviewFingerprint(data.review), next)) setStorageUnavailable(true);
  }

  async function startImport() {
    if (!data || busy || data.origin?.visibility === "public" && !publicAcknowledged) return;
    setBusy(true); setActionProblem(""); setAcknowledged(false);
    actionStage.current = "import";
    try {
      let source = data.source;
      if (!source) {
        const scope = reviewDraftKey(data.deployment.id, reviewerID, "source-creation", `documentation:${context}`);
        if (mode !== "existing") {
          if (mode === "upload" && !file || mode === "website" && !location.trim()) throw new Error(t("documentationSetup.chooseInput"));
          const fingerprint = await sourceCreationFingerprint({ kind: mode, location, file: mode === "upload" ? file ?? undefined : undefined });
          const { attempt, persisted } = sourceCreationAttempt(browserReviewStorage, scope, fingerprint, sourceCreation.current);
          sourceCreation.current = attempt; setStorageUnavailable(!persisted);
        }
        source = mode === "existing" ? data.sources.find((value) => value.id === existingID) : mode === "upload" && file ? await api.uploadSource(data.deployment.id, data.deployment.organisation_id, file, undefined, sourceCreation.current?.key) : await api.createSource(data.deployment.id, data.deployment.organisation_id, "website", location.trim(), undefined, sourceCreation.current?.key);
        if (!source || !["website", "upload"].includes(source.kind)) throw new Error(t("documentationSetup.chooseInput"));
        remember({ setup: "content", source: source.id, run: "", queue: "", after: "" }, data.deployment.id);
        finishSourceCreationAttempt(browserReviewStorage, scope); sourceCreation.current = null;
      }
      if (!mounted.current) return;
      if (data.origin?.visibility === "public" && source.visibility !== "public") source = await api.setSourceVisibility(data.deployment.id, source.id, "public", source.revision, true);
      const jobs = await api.crawlJobs(data.deployment.id, source.id);
      let job = pendingDocumentationImport(jobs, progress.current) || jobs.find((value) => ["queued", "running"].includes(value.state));
      // Selecting an existing source opens its current review instead of
      // importing again. A subsequent Import again action is explicit.
      if (!data.source && mode === "existing" && !job) job = jobs[0];
      if (!job) {
        remember({ setup: "content", source: source.id, run: "", queue: "pending", after: jobs[0]?.id ?? "" }, data.deployment.id);
        job = await api.queueCrawl(data.deployment.id, source.id);
      }
      remember({ setup: "content", source: source.id, run: job.id, queue: "", after: "" }, data.deployment.id);
      if (mounted.current) setAttempt((value) => value + 1);
    } catch (error) { if (mounted.current) setActionProblem(error instanceof DocumentationSetupChangedError ? t("documentationSetup.changed") : developerAssetError(error, t("documentationSetup.importFailed"))); }
    finally { if (mounted.current) setBusy(false); }
  }

  async function replaceFile() {
    if (!data?.source || !replacementFile || busy || data.source.visibility === "public" && !publicAcknowledged) return;
    setBusy(true); setActionProblem(""); setAcknowledged(false);
    try {
      const scope = reviewDraftKey(data.deployment.id, reviewerID, "source-input-replacement", data.source.id);
      const fingerprint = await sourceCreationFingerprint({ kind: "upload", name: data.source.id, file: replacementFile });
      const prepared = sourceInputReplacementAttempt(browserReviewStorage, scope, fingerprint, data.source.revision, replacementAttempt.current);
      replacementAttempt.current = prepared.attempt; setStorageUnavailable(!prepared.persisted);
      const result = await api.replaceSourceUpload(data.deployment.id, data.source.id, data.source.organisation_id, prepared.attempt.revision, replacementFile, prepared.attempt.key);
      if (result.source.id !== data.source.id || result.source.product_id !== data.deployment.id || result.crawl_job.source_id !== data.source.id || result.crawl_job.product_id !== data.deployment.id) throw new DocumentationSetupChangedError();
      if (!mounted.current) return;
      remember({ setup: "content", source: result.source.id, run: result.crawl_job.id, queue: "", after: "" }, data.deployment.id);
      onSourceChanged?.(result.source); finishSourceInputReplacement(browserReviewStorage, scope); replacementAttempt.current = null; setReplacementInput(null); setPublicAcknowledged(false); setAttempt((value) => value + 1);
    } catch (error) { if (mounted.current) setActionProblem(developerAssetError(error, t("documentationSetup.replaceFailed"))); }
    finally { if (mounted.current) setBusy(false); }
  }

  async function finish() {
    if (!data?.review || busy || !acknowledged) return;
    setBusy(true); setActionProblem("");
    actionStage.current = "finish";
    try {
      const result = await finishDocumentationSetup({ review: data.review, selected, origin: data.origin, collection: data.collection, expectedBinding: data.binding }, {
        review: api.sourceReview, publishSource: (review, ids) => api.publishSource(data.deployment.id, review.source.id, { revision: review.source.revision, crawl_job_id: review.crawl_job.id, document_ids: ids, acknowledge_reviewed: true }),
        collections: developerAssetsApi.documentationCollections, revisions: developerAssetsApi.documentationCollectionRevisions,
        create: developerAssetsApi.createDocumentationCollection, revise: developerAssetsApi.reviseDocumentationCollection,
        integration: (id) => api.integration(id).then((value) => value.integration), resources: developerAssetsApi.apiResources,
        attach: (id, collectionID, revisionID, visibility) => developerAssetsApi.attachAPIDocumentation(id, { documentation_collection_id: collectionID, pinned_revision_id: revisionID, selector: {}, visibility }),
        change: (id, binding, revisionID) => developerAssetsApi.changeAPIDocumentation(id, binding.id, { documentation_collection_id: binding.documentation_collection_id, pinned_revision_id: revisionID, selector: {}, visibility: binding.visibility, revision: binding.revision }),
      });
      if (mounted.current) { onMessage(t("documentationSetup.saved", { revision: result.revision.revision })); setAcknowledged(false); setAttempt((value) => value + 1); }
    } catch (error) { if (mounted.current) { setActionProblem(error instanceof DocumentationSetupChangedError ? t("documentationSetup.changed") : developerAssetError(error, t("documentationSetup.finishFailed"))); setAttempt((value) => value + 1); } }
    finally { if (mounted.current) setBusy(false); }
  }

  const complete = Boolean(data?.revision && (!data.origin || documentationBindingMatches(data.binding, data.revision, data.binding?.visibility ?? data.origin.visibility)));
  const running = Boolean(data?.job && ["queued", "running"].includes(data.job.state));
  const blocked = Boolean(data?.review && (data.review.source.quarantined || data.review.crawl_job.failed_count || data.review.crawl_job.skipped_count));
  const historicalDraft = Boolean(data?.review && !data.review.publication && data.jobs[0]?.id !== data.review.crawl_job.id);
  const audienceBlocked = Boolean(data?.review && (data.origin?.visibility === "public" && (data.review.publication?.visibility ?? data.review.source.visibility) !== "public"));
  return <>
    <PageHeader eyebrow={t("navigation.knowledge")} title={data?.source?.name ?? t("documentationSetup.title")} description={data?.origin ? t("documentationSetup.forAPI", { name: data.origin.display_name }) : t("documentationSetup.description")} action={<Button outline disabled={busy} onClick={() => onNavigate(selection.api ? integrationPath(selection.api, "documentation") : sectionPath("documents"))}>{t(selection.api ? "contractSetup.backAPI" : "documentationSetup.backLibrary")}</Button>} />
    {loading ? <LoadingPanel label={t("common.loading")} /> : problem ? <><ProblemPanel message={problem} onRetry={() => setAttempt((value) => value + 1)} />{selection.source && <Button outline onClick={() => onNavigate(documentationSetupPath({ ...parseDocumentationSetupSelection(), source: selection.source, api: selection.api }))}>{t("documentationSetup.openCurrent")}</Button>}</> : data && <>
      {actionProblem && <p className="auth-problem" role="alert">{actionProblem}</p>}
      {storageUnavailable && <p role="status">{t("documentationSetup.storageUnavailable")}</p>}
      {!data.review && <AIReadinessPanel preserveForm compact />}
      {!data.source ? <section className="panel documentation-setup-panel"><PanelHeader title={t("documentationSetup.inputTitle")} description={t("documentationSetup.inputHelp")} /><div className="auth-form compact-form">
        <label className="auth-field"><span id="documentation-input-label">{t("documentationSetup.input")}</span><select aria-labelledby="documentation-input-label" value={mode} disabled={busy} onChange={(event) => { setMode(event.target.value as typeof mode); setPublicAcknowledged(false); setActionProblem(""); }}><option value="website">{t("sourceDialogs.website")}</option><option value="upload">{t("sourceDialogs.uploadAFile")}</option><option value="existing">{t("documentationSetup.existing")}</option></select></label>
        {mode === "website" ? <label className="auth-field"><span>{t("sourceDialogs.location")}</span><input type="url" value={location} disabled={busy} onChange={(event) => { setLocation(event.target.value); setPublicAcknowledged(false); }} placeholder="https://example.com/docs" /><small>{t("sourceDialogs.websitePathBoundary")}</small></label> : mode === "upload" ? <label className="auth-field"><span>{t("sourceDialogs.file")}</span><input type="file" accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml" disabled={busy} onChange={(event) => { setFile(event.target.files?.[0] ?? null); setPublicAcknowledged(false); }} /><small>{t("sourceDialogs.utfN8MdMdxTxtHtmlHtmJsonYaml")}</small></label> : <label className="auth-field"><span id="documentation-existing-label">{t("documentationSetup.existing")}</span><select aria-labelledby="documentation-existing-label" value={existingID} disabled={busy} onChange={(event) => { setExistingID(event.target.value); setPublicAcknowledged(false); }}><option value="">{t("documentationSetup.chooseInput")}</option>{data.sources.filter((source) => ["website", "upload"].includes(source.kind)).map((source) => <option key={source.id} value={source.id}>{source.name} · {t(`common.${source.visibility}`)}</option>)}</select></label>}
        {data.origin?.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("documentationSetup.publicInput")}</span></label>}
        <Button disabled={busy || (mode === "website" ? !location.trim() : mode === "upload" ? !file : !existingID) || data.origin?.visibility === "public" && !publicAcknowledged} onClick={() => void startImport()}>{busy ? t("common.saving") : t(mode === "existing" ? "documentationSetup.continueExisting" : "contractSetup.startImport")}</Button>
      </div></section> : <section className="panel documentation-setup-panel"><PanelHeader title={t("documentationSetup.importTitle")} description={t("documentationSetup.importHelp")} action={<Badge>{t(`common.${data.source.visibility}`)}</Badge>} />
        <p aria-live="polite">{blocked ? t("documentationSetup.blockedState") : data.job ? t("documentationSetup.importState", { state: t(`documentationSetup.importStates.${data.job.state}`), count: data.job.fetched_count }) : t("documentationSetup.readyToImport")}</p>
        {data.job?.state === "failed" && data.job.error_message && <p className="auth-problem" role="alert">{data.job.error_message}</p>}
        {!data.review && !running && <><p>{t("documentationSetup.retryImportHelp")}</p>{data.origin?.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("documentationSetup.publicInput")}</span></label>}<Button disabled={busy || data.origin?.visibility === "public" && !publicAcknowledged} onClick={() => void startImport()}>{t("contractSetup.startResumeImport")}</Button></>}
        {data.job && data.jobs[0] && data.job.id !== data.jobs[0].id && <Button outline disabled={busy} onClick={() => remember({ run: data.jobs[0].id, queue: "", after: "" }, data.deployment.id)}>{t("documentationSetup.openCurrent")}</Button>}
        {data.source.kind === "upload" && !running && data.job?.id === data.jobs[0]?.id && <details open={blocked || data.job?.state === "failed" || undefined}>
          <summary>{t("documentationSetup.replaceFile")}</summary>
          <p>{t("documentationSetup.replaceHelp", { audience: t(`common.${data.source.visibility}`) })}</p>
          <label className="auth-field"><span>{t("documentationSetup.cleanedFile")}</span><input key={data.source.id} type="file" accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml" disabled={busy} onChange={(event) => { const file = event.target.files?.[0]; setReplacementInput(file ? { sourceID: data.source!.id, file } : null); setPublicAcknowledged(false); }} /></label>
          {data.source.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} disabled={busy} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("documentationSetup.replacePublic")}</span></label>}
          <Button disabled={busy || !replacementFile || data.source.visibility === "public" && !publicAcknowledged} onClick={() => void replaceFile()}>{busy ? t("common.saving") : t("documentationSetup.replaceAndImport")}</Button>
        </details>}
        {blocked && data.source.kind === "website" && !running && data.job?.id === data.jobs[0]?.id && <>
          <p>{t("documentationSetup.cleanWebsiteHelp")}</p>
          {data.origin?.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("documentationSetup.publicInput")}</span></label>}
          <Button disabled={busy || data.origin?.visibility === "public" && !publicAcknowledged} onClick={() => void startImport()}>{t("documentationSetup.importCleanWebsite")}</Button>
        </>}
        <details><summary>{t("documentationSetup.importHistory")}</summary><label className="auth-field"><span id="documentation-import-label">{t("documentationSetup.importVersion")}</span><select aria-labelledby="documentation-import-label" value={data.job?.id ?? ""} disabled={busy} onChange={(event) => { if (event.target.value) remember({ run: event.target.value, queue: "", after: "" }, data.deployment.id); }}><option value="">{t("contractSetup.chooseImport")}</option>{data.jobs.map((job) => <option key={job.id} value={job.id}>{t("format.dateTime", { value: new Date(job.queued_at) })} · {t(`documentationSetup.importStates.${job.state}`)}</option>)}</select></label>{data.job && <PrettyJSON value={data.job.diagnostics} label={t("common.diagnostics")} />}{data.review && <><p>{t("documentationSetup.reimportHelp")}</p>{data.origin?.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("documentationSetup.publicInput")}</span></label>}<Button outline disabled={busy || running || data.origin?.visibility === "public" && !publicAcknowledged} onClick={() => void startImport()}>{t("documentationSetup.reimport")}</Button></>}</details>
      </section>}
      {data.review && <section className="panel documentation-setup-panel"><PanelHeader title={t("documentationSetup.reviewTitle")} description={t("documentationSetup.reviewHelp")} action={<Badge color={complete ? "green" : "amber"}>{t(complete ? "documentationSetup.complete" : "sourceDialogs.needsReview")}</Badge>} />
        {data.review.publication ? <details><summary>{t("contractSetup.processingEvidence")}</summary><KnowledgeProcessingPanel key={data.review.crawl_job.id} runID={data.review.crawl_job.id} onReady={setProcessingReady} compactAI /></details> : blocked || historicalDraft ? <p>{t("documentationSetup.processingAfterRecovery")}</p> : <KnowledgeProcessingPanel key={data.review.crawl_job.id} runID={data.review.crawl_job.id} onReady={setProcessingReady} compactAI />}
        {!data.review.publication && <p>{t("sdkCatalog.reviewDraftDescription")}</p>}
        <div className="source-review-documents">{data.review.documents.map((document) => <SourceReviewDocument key={`${document.crawl_job_id}:${document.id}:${document.content_hash}`} productID={data.deployment.id} sourceID={data.review!.source.id} document={document} selected={selected.includes(document.id)} disabled={busy || blocked} onSelected={(included) => chooseDocument(document.id, included)} publishedDecision={data.review!.publication ? data.review!.published_document_ids?.includes(document.id) ? "included" : "excluded" : undefined} />)}</div>
        {historicalDraft && <p className="auth-problem">{t("documentationSetup.historicalDraft")}</p>}{blocked && <p className="auth-problem">{t("documentationSetup.blocked")}</p>}{audienceBlocked && <p className="auth-problem">{t("documentationSetup.audienceBlocked")}</p>}
        {complete ? <div role="status"><p>{t(data.origin ? "documentationSetup.attached" : "documentationSetup.inLibrary")}</p><Button onClick={() => onNavigate(data.origin ? integrationPath(data.origin.id) : sectionPath("documents"))}>{t(data.origin ? "contractSetup.reviewAPI" : "documentationSetup.backLibrary")}</Button></div> : blocked || historicalDraft ? null : <><p>{t("documentationSetup.finishScope", { count: selected.length, audience: t(`common.${data.review.publication?.visibility ?? data.review.source.visibility}`) })}</p>{data.origin && <p>{t("documentationSetup.attachScope", { name: data.origin.display_name })}</p>}<label className="compact-check"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("documentationSetup.acknowledge")}</span></label><Button disabled={busy || blocked || historicalDraft || audienceBlocked || !selected.length || !acknowledged || !data.review.publication && processingReady !== data.review.crawl_job.id} onClick={() => void finish()}>{busy ? t("common.saving") : t(data.origin ? "documentationSetup.publishAttach" : "documentationSetup.publish")}</Button></>}
      </section>}
    </>}
  </>;
}
