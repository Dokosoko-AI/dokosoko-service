"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APIIntegration } from "../../../lib/api";
import { integrationPath, sectionPath } from "../../../lib/console-routes";
import { developerAssetsApi, type APIResourceBindings, type ReviewDecision, type SDKContentCandidate, type SDKContentCandidateRecord, type SDKContentPublication, type SDKContentPublicationRecord, type SDKIngestionFile, type SDKPackage, type SDKRelease, type SDKReleaseLifecycleState } from "../../../lib/developer-assets-api";
import { browserReviewStorage, clearReviewDraft, readReviewDraft, reviewDraftKey, writeReviewDraft } from "../../../lib/review-draft";
import { fingerprintSDKInput, finishSDKSetup, recoverSDKInput, SDKSetupChangedError, sdkPublicationDecisions, sdkSetupBinding, sdkSetupBindingMatches, sdkSetupPath, validSDKPendingInput, validSDKSetupProgress, type SDKSetupSelection } from "../../../lib/sdk-setup";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader } from "../../core/layout";
import { ConsoleLink } from "../console-link";
import { developerAssetError, enumLabel, LoadingPanel, PrettyJSON, ProblemPanel, recordString } from "./developer-asset-ui";
import { decisionsComplete, decisionPayload, isSDKReviewDraft, type SDKDecisionState } from "./sdk-catalog-helpers";
import { SDKGuidanceInput } from "./sdk-guidance-input";
import { SDKGuidanceReview } from "./sdk-guidance-review";
import { sdkUsages, type SDKUsage } from "./developer-asset-usage";
import { KnowledgeProcessingPanel } from "./knowledge-processing-panel";

type SetupData = { usedBy: SDKUsage[]; pkg: SDKPackage; releases: SDKRelease[]; release?: SDKRelease; lifecycle?: SDKReleaseLifecycleState; origin?: APIIntegration; resources?: APIResourceBindings; candidates: SDKContentCandidate[]; publications: SDKContentPublication[]; record?: SDKContentCandidateRecord; publication?: SDKContentPublicationRecord; previous?: SDKContentCandidateRecord };
function decisions(values: ReviewDecision[]): SDKDecisionState {
  return Object.fromEntries(values.map((value) => [value.id, { decision: value.decision, reason: value.reason ?? "", reviewEvidence: value.review_evidence?.summary ?? "" }]));
}
export function SDKSetupWorkspace({ selection, reviewerID, integrations, onNavigate, onMessage }: { integrations: APIIntegration[]; selection: SDKSetupSelection; reviewerID: string; onNavigate: (path: string) => void; onMessage: (message: string) => void }) {
  const { t } = useTranslation();
  const [data, setData] = useState<SetupData | null>(null);
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
  const [actionProblem, setActionProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState(false);
  const [fileDecisions, setFileDecisions] = useState<SDKDecisionState>({});
  const [sampleDecisions, setSampleDecisions] = useState<SDKDecisionState>({});
  const [acknowledged, setAcknowledged] = useState(false);
  const [publicAcknowledged, setPublicAcknowledged] = useState(false);
  const [processingReadyRunID, setProcessingReadyRunID] = useState("");
  const [complete, setComplete] = useState(false);
  const requestID = useRef(0);
  const mounted = useRef(false);
  const progress = useRef(selection);
  const progressKey = useRef("");
  const pendingKey = useRef("");
  const scope = `${selection.package}:${selection.release}:${selection.api}`;
  const invalidateRequest = useCallback(() => { requestID.current++; }, []);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; invalidateRequest(); }; }, [invalidateRequest]);
  useEffect(() => { progress.current = selection; }, [selection]);
  function remember(next: Partial<SDKSetupSelection>, navigate = true) {
    progress.current = { ...progress.current, ...next };
    if (!writeReviewDraft(browserReviewStorage, progressKey.current, "sdk-setup-v1", progress.current)) onMessage(t("sdkCatalog.reviewDraftNotSaved"));
    if (navigate && mounted.current) onNavigate(sdkSetupPath(progress.current));
  }
  const load = useCallback(async () => {
    const request = ++requestID.current;
    setLoading(true); setProblem(""); setAcknowledged(false); setPublicAcknowledged(false); setProcessingReadyRunID("");
    try {
      const [pkg, releases, origin, resources, usage] = await Promise.all([
        developerAssetsApi.sdkPackage(selection.package), developerAssetsApi.sdkReleases(selection.package),
        selection.api ? api.integration(selection.api).then((value) => value.integration) : undefined,
        selection.api ? developerAssetsApi.apiResources(selection.api) : undefined, developerAssetsApi.usage(),
      ]);
      if (request !== requestID.current || !mounted.current) return;
      const usedBy = sdkUsages(usage, integrations, pkg.id);
      if (origin && origin.deployment_id !== pkg.deployment_id) throw new SDKSetupChangedError();
      const release = releases.find((value) => value.id === selection.release);
      if (selection.release && (!release || release.sdk_package_id !== pkg.id || release.deployment_id !== pkg.deployment_id)) throw new SDKSetupChangedError();
      progressKey.current = reviewDraftKey(pkg.deployment_id, reviewerID, "sdk-setup", scope);
      pendingKey.current = reviewDraftKey(pkg.deployment_id, reviewerID, "sdk-pending-input", scope);
      const saved = readReviewDraft(browserReviewStorage, progressKey.current, "sdk-setup-v1", validSDKSetupProgress);
      if (release && !selection.step && !selection.candidate && !selection.publication && saved && saved.package === pkg.id && saved.release === release.id && saved.api === selection.api && (saved.candidate || saved.step === "input")) { onNavigate(sdkSetupPath(saved)); return; }
      if (!release) { setData({ usedBy, pkg, releases, origin, resources, candidates: [], publications: [] }); return; }
      const [candidates, publications, lifecycle] = await Promise.all([developerAssetsApi.sdkContentCandidates(release.id), developerAssetsApi.sdkContentPublications(release.id), developerAssetsApi.sdkReleaseLifecycle(pkg.id, release.id)]);
      if (request !== requestID.current || !mounted.current) return;
      const ordered = [...publications].sort((a, b) => b.revision - a.revision);
      let candidate = selection.candidate ? candidates.find((value) => value.id === selection.candidate) : undefined;
      const selectedPublication = selection.publication ? ordered.find((value) => value.id === selection.publication) : undefined;
      if ((selection.candidate && !candidate) || (selection.publication && !selectedPublication)) throw new SDKSetupChangedError();
      if (selectedPublication) {
        if (candidate && candidate.id !== selectedPublication.sdk_content_candidate_id) throw new SDKSetupChangedError();
        candidate = candidates.find((value) => value.id === selectedPublication.sdk_content_candidate_id);
        if (!candidate) throw new SDKSetupChangedError();
      }
      const pending = readReviewDraft(browserReviewStorage, pendingKey.current, release.release_hash, validSDKPendingInput);
      if (!candidate && pending) candidate = recoverSDKInput(candidates, pending);
      if (!candidate && !pending && selection.step !== "input") candidate = candidates[0];
      if (candidate && (candidate.sdk_release_id !== release.id || candidate.deployment_id !== pkg.deployment_id)) throw new SDKSetupChangedError();
      const published = ordered.find((value) => value.sdk_content_candidate_id === candidate?.id);
      if (candidate && !selection.candidate) { onNavigate(sdkSetupPath({ ...selection, candidate: candidate.id, step: "review", publication: published?.id ?? "" })); return; }
      const [record, publication] = await Promise.all([candidate ? developerAssetsApi.sdkContentCandidate(release.id, candidate.id) : undefined, published ? developerAssetsApi.sdkContentPublication(release.id, published.id) : undefined]);
      if (request !== requestID.current || !mounted.current) return;
      if (publication && (publication.publication.id !== published?.id || publication.publication.sdk_content_candidate_id !== candidate?.id || publication.publication.sdk_release_id !== release.id || publication.publication.content_hash !== candidate?.content_hash || publication.publication.deployment_id !== pkg.deployment_id)) throw new SDKSetupChangedError();
      if (record && (record.candidate.id !== candidate?.id || record.candidate.content_hash !== candidate.content_hash)) throw new SDKSetupChangedError();
      const older = candidate ? ordered.find((value) => { const prior = candidates.find((item) => item.id === value.sdk_content_candidate_id); return prior && prior.id !== candidate.id && Date.parse(recordString(prior, "created_at")) < Date.parse(recordString(candidate, "created_at")); }) : undefined;
      let previous: SDKContentCandidateRecord | undefined;
      if (older) {
        const [prior, priorPublication] = await Promise.all([developerAssetsApi.sdkContentCandidate(release.id, older.sdk_content_candidate_id), developerAssetsApi.sdkContentPublication(release.id, older.id)]);
        const included = new Set(sdkPublicationDecisions(priorPublication).files.filter((value) => value.decision === "included").map((value) => value.id));
        previous = { ...prior, files: prior.files.filter((value) => included.has(recordString(value, "id"))) };
      }
      if (request !== requestID.current || !mounted.current) return;
      const draft = record ? readReviewDraft(browserReviewStorage, reviewDraftKey(pkg.deployment_id, reviewerID, "sdk", record.candidate.id), record.candidate.content_hash, isSDKReviewDraft) : null;
      const reviewed = publication ? sdkPublicationDecisions(publication) : undefined;
      setFileDecisions(reviewed ? decisions(reviewed.files) : draft?.files ?? {}); setSampleDecisions(reviewed ? decisions(reviewed.samples) : draft?.samples ?? {});
      setData({ usedBy, pkg, releases, release, origin, resources, candidates, publications: ordered, lifecycle, record, publication, previous });
      const binding = resources?.sdks.find((value) => value.sdk_package_id === pkg.id && value.state !== "detached");
      const finished = Boolean(publication && (!origin || sdkSetupBindingMatches(binding, origin, sdkSetupBinding(pkg, release, publication.publication, origin, binding))));
      setComplete(finished);
      if (finished) setActionProblem("");
    } catch (error) { if (request === requestID.current && mounted.current) setProblem(error instanceof SDKSetupChangedError ? t("sdkSetup.changedSelection") : developerAssetError(error, t("sdkSetup.loadFailed"))); }
    finally { if (request === requestID.current && mounted.current) setLoading(false); }
  }, [integrations, onNavigate, reviewerID, scope, selection, t]);
  useEffect(() => { const timer = window.setTimeout(() => { void load(); }, 0); return () => { clearTimeout(timer); invalidateRequest(); }; }, [attempt, invalidateRequest, load]);
  useEffect(() => {
    if (!data?.record || loading || data.publication) return;
    const key = reviewDraftKey(data.pkg.deployment_id, reviewerID, "sdk", data.record.candidate.id);
    if (!writeReviewDraft(browserReviewStorage, key, data.record.candidate.content_hash, { files: fileDecisions, samples: sampleDecisions })) onMessage(t("sdkCatalog.reviewDraftNotSaved"));
  }, [data, fileDecisions, loading, onMessage, reviewerID, sampleDecisions, t]);
  async function ingest(files: SDKIngestionFile[]) {
    if (!data?.release) return;
    const fingerprint = await fingerprintSDKInput(files);
    if (!writeReviewDraft(browserReviewStorage, pendingKey.current, data.release.release_hash, fingerprint)) onMessage(t("sdkCatalog.reviewDraftNotSaved"));
    const result = await developerAssetsApi.ingestSDKContent(data.release.id, { files });
    if (result.candidate.candidate.sdk_release_id !== data.release.id) throw new SDKSetupChangedError();
    clearReviewDraft(browserReviewStorage, pendingKey.current);
    remember({ candidate: result.candidate.candidate.id, publication: "", step: "review" });
  }
  async function finish() {
    if (!data?.record || !data.release || !allowed || !ready || !acknowledged || busy || (data.origin?.visibility === "public" && !publicAcknowledged)) return;
    setBusy(true); setActionProblem("");
    try {
      const files = data.publication ? sdkPublicationDecisions(data.publication).files : decisionPayload(data.record.files, fileDecisions, "file");
      const samples = data.publication ? sdkPublicationDecisions(data.publication).samples : decisionPayload(data.record.samples, sampleDecisions, "sample");
      const publication = await finishSDKSetup({ package: data.pkg, release: data.release, candidate: data.record.candidate, api: data.origin, files, samples, expectedBinding: data.resources?.sdks.find((value) => value.sdk_package_id === data.pkg.id && value.state !== "detached") }, {
        integration: (id) => api.integration(id).then((value) => value.integration), package: developerAssetsApi.sdkPackage, release: developerAssetsApi.sdkRelease, lifecycle: developerAssetsApi.sdkReleaseLifecycle,
        publications: developerAssetsApi.sdkContentPublications, publication: developerAssetsApi.sdkContentPublication, publish: developerAssetsApi.publishSDKContentCandidate,
        resources: developerAssetsApi.apiResources, attach: developerAssetsApi.attachAPISDK, change: developerAssetsApi.changeAPISDK,
      }, (value) => remember({ publication: value.id, step: "review" }, false));
      clearReviewDraft(browserReviewStorage, reviewDraftKey(data.pkg.deployment_id, reviewerID, "sdk", data.record.candidate.id));
      remember({ publication: publication.id, step: "complete" });
    } catch (error) {
      if (mounted.current) {
        if (progress.current.publication) setAttempt((value) => value + 1);
        setActionProblem(error instanceof SDKSetupChangedError ? t("sdkSetup.changedSelection") : developerAssetError(error, t("sdkSetup.finishFailed")));
        // Keep the current review open; a saved publication will be recovered on
        // reload or retry without repeating the publish operation.
      }
    } finally { if (mounted.current) setBusy(false); }
  }
  if (loading) return <LoadingPanel label={t("sdkSetup.loading")} />;
  if (problem || !data) return <ProblemPanel message={problem || t("sdkSetup.loadFailed")} onRetry={() => setAttempt((value) => value + 1)} />;
  const { pkg, release, record, publication, lifecycle, origin } = data;
  const allowed = pkg.lifecycle !== "archived" && (!release || lifecycle?.selectable) && (!origin || origin.visibility !== "public" || pkg.visibility === "public" && release?.visibility === "public");
  const reviewComplete = Boolean(record && decisionsComplete(record.files, fileDecisions, "file") && decisionsComplete(record.samples, sampleDecisions, "sample") && Object.values(fileDecisions).some((value) => value.decision === "included"));
  const ready = Boolean(publication || record && processingReadyRunID === record.candidate.ingestion_run_id && reviewComplete);
  const oldBinding = data.resources?.sdks.find((value) => value.sdk_package_id === pkg.id && value.state !== "detached");
  return <>
    <div className="heading-actions"><ConsoleLink path={sectionPath("sdks")} onNavigate={onNavigate}>{t("sdkSetup.allPackages")}</ConsoleLink>{origin && <ConsoleLink path={integrationPath(origin.id, "documentation")} onNavigate={onNavigate}>{t("sdkCatalog.returnToAPI")}</ConsoleLink>}</div>
    <PageHeader eyebrow={t("sdkSetup.packageGuidance")} title={pkg.name} description={origin ? t("sdkSetup.forAPI", { name: origin.display_name }) : pkg.description} />
    <section className="panel sdk-setup-panel"><div className="sdk-release-facts"><strong>{pkg.display_coordinate}{release ? ` · ${release.exact_version}` : ""}</strong><Badge>{t(`common.${release?.visibility ?? pkg.visibility}`)}</Badge>{release && <Badge color={complete ? "green" : "amber"}>{t(complete ? "sdkSetup.published" : publication ? "sdkSetup.attachNext" : record ? "sdkSetup.reviewNeeded" : "sdkSetup.guidanceNeeded")}</Badge>}</div>
      {release && <code className="sdk-install-command">{release.install_command}</code>}
      {!allowed && <p className="auth-problem" role="alert">{t(origin?.visibility === "public" && (pkg.visibility !== "public" || release?.visibility !== "public") ? "sdkSetup.publicMismatch" : "sdkSetup.releaseUnavailable")}</p>}
      {release && <div className="heading-actions">{record && <Button outline disabled={busy || !allowed} onClick={() => { clearReviewDraft(browserReviewStorage, pendingKey.current); remember({ candidate: "", publication: "", step: "input" }); }}>{t("sdkSetup.updateGuidance")}</Button>}<ConsoleLink path={`${sdkSetupPath({ ...selection, package: pkg.id, release: release.id, candidate: record?.candidate.id ?? "", publication: publication?.publication.id ?? "" })}&advanced=1`} onNavigate={onNavigate}>{t("sdkSetup.managePackage")}</ConsoleLink></div>}
    </section>
    {!release ? <section className="panel sdk-setup-panel"><h2>{t("sdkSetup.chooseExactRelease")}</h2>{data.releases.map((value) => <div className="sdk-version-row" key={value.id}><span><strong>{value.exact_version}</strong><small>{t(`common.${value.visibility}`)} · {enumLabel(t, value.lifecycle)}</small></span><Button outline onClick={() => onNavigate(sdkSetupPath({ package: pkg.id, release: value.id, api: selection.api }))}>{t("sdkSetup.openRelease")}</Button></div>)}{data.releases.length === 0 && <p>{t("sdkSetup.noReleases")}</p>}<ConsoleLink path={`${sdkSetupPath({ package: pkg.id, api: selection.api })}&advanced=1`} onNavigate={onNavigate}>{t("sdkSetup.managePackage")}</ConsoleLink></section> : <>
      {!record ? <SDKGuidanceInput key={release.id} disabled={!allowed} onSubmit={ingest} /> : <section className="panel sdk-setup-panel">
        <PanelHeader title={publication ? t("sdkSetup.publishedGuidance") : t("sdkSetup.reviewGuidance")} description={t(publication ? "sdkSetup.publishedHelp" : "sdkSetup.reviewHelp")} />
        {!publication && <KnowledgeProcessingPanel compactAI key={record.candidate.ingestion_run_id} runID={record.candidate.ingestion_run_id} onReady={setProcessingReadyRunID} />}
        <SDKGuidanceReview key={record.candidate.id} record={record} previous={data.previous} files={fileDecisions} samples={sampleDecisions} readOnly={Boolean(publication)} disabled={busy} onChange={(kind, id, value) => { setAcknowledged(false); setPublicAcknowledged(false); (kind === "file" ? setFileDecisions : setSampleDecisions)((current) => ({ ...current, [id]: value })); }} />
        <p>{publication ? t("sdkSetup.reviewedNotTested") : t("sdkCatalog.reviewDraftDescription")}</p>
        {complete ? <div><p>{t(origin ? "sdkSetup.attachedComplete" : "sdkSetup.publishedComplete")}</p>{origin && <Button onClick={() => onNavigate(integrationPath(origin.id))}>{t("sdkSetup.reviewAPI")}</Button>}</div> : <>
          {oldBinding && (oldBinding.sdk_release_id !== release.id || oldBinding.sdk_content_publication_id !== publication?.publication.id) && <p>{t("sdkSetup.replacesBinding")}</p>}
          {origin?.visibility === "public" && <label className="compact-check"><input type="checkbox" checked={publicAcknowledged} disabled={busy} onChange={(event) => setPublicAcknowledged(event.target.checked)} /><span>{t("apiResources.confirmPublicAttachment")}</span></label>}
          <label className="compact-check"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t(origin ? "sdkSetup.confirmPublishAttach" : "sdkSetup.confirmPublish")}</span></label>
          {actionProblem && <p className="auth-problem" role="alert">{actionProblem}</p>}
          <div className="heading-actions"><Button disabled={!allowed || !ready || !acknowledged || busy || (origin?.visibility === "public" && !publicAcknowledged)} onClick={() => void finish()}>{t(busy ? "sdkSetup.finishing" : publication && origin ? "sdkSetup.attachExact" : origin ? "sdkSetup.publishAttach" : "sdkSetup.publish")}</Button><Button outline disabled={busy} onClick={() => setAttempt((value) => value + 1)}>{t("sdkSetup.refresh")}</Button></div>
        </>}
        <details><summary>{t("sdkSetup.processingDetails")}</summary>{publication && <KnowledgeProcessingPanel compactAI key={record.candidate.ingestion_run_id} runID={record.candidate.ingestion_run_id} onReady={setProcessingReadyRunID} />}<PrettyJSON value={{ candidate: record.candidate, map: record.map, symbols: record.symbols, publication: publication?.publication }} /></details>
      </section>}
      <section className="panel sdk-setup-panel"><PanelHeader title={t("sdkSetup.usedByAPIs")} description={t("sdkSetup.usageHelp")} />{data.usedBy.map(({ integration, binding }) => <div className="sdk-version-row" key={binding.id}><span><strong>{integration.display_name} · {integration.version_key}</strong><small>{t("sdkSetup.boundRelease", { version: data.releases.find((value) => value.id === binding.sdk_release_id)?.exact_version ?? t("sdkSetup.otherRelease") })} · {enumLabel(t, binding.state)} · {t(`apiResources.${binding.assurance}`)}</small></span><ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate}>{t("sdkCatalog.returnToAPI")}</ConsoleLink></div>)}{data.usedBy.length === 0 && <p>{t("sdkCatalog.thisPackageIsNotAttachedToAnAPI")}</p>}</section>
      <details className="panel sdk-setup-panel"><summary>{t("sdkSetup.versionsAndHistory")}</summary>{data.releases.map((value) => <div className="sdk-version-row" key={value.id}><span>{value.exact_version}</span><ConsoleLink path={sdkSetupPath({ package: pkg.id, release: value.id, api: selection.api })} onNavigate={onNavigate}>{t("sdkSetup.openRelease")}</ConsoleLink></div>)}{data.candidates.map((value, index) => { const pub = data.publications.find((item) => item.sdk_content_candidate_id === value.id); return <div className="sdk-version-row" key={value.id}><span>{t("sdkSetup.importNumber", { number: data.candidates.length - index })} · {pub ? t("sdkSetup.contentRevision", { revision: pub.revision }) : t("sdkSetup.reviewNeeded")}</span><ConsoleLink path={sdkSetupPath({ package: pkg.id, release: release.id, api: selection.api, candidate: value.id, publication: pub?.id, step: "review" })} onNavigate={onNavigate}>{t("sdkSetup.openGuidance")}</ConsoleLink></div>; })}</details>
    </>}
  </>;
}
