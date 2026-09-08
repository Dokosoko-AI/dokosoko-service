"use client";


import { useTranslation } from "react-i18next";
import { FileText, Plus, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { Section } from "../../../lib/console-routes";
import type { Source } from "../../../lib/console-domain";
import type { APIIntegration, APISource } from "../../../lib/api";
import {
  developerAssetsApi,
  type DeveloperAssetIngestionSummary,
  type DocumentationCandidateRecord,
  type DocumentationLibraryItem,
  type DocumentationAttentionPage,
  type DocumentationCollectionMemberInput,
  type SourcePublicationDocumentSelection,
} from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { DocumentationCollectionsView } from "./documentation-collections-view";
import { DeveloperAssetAIAdvisoryButton } from "./developer-asset-ai-advisory";
import { evidenceChange } from "../../core/evidence-content";
import { developerAssetError, enumLabel, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel } from "./developer-asset-ui";
import { documentationSetupPath, parseDocumentationSetupSelection } from "../../../lib/documentation-setup";
import { DocumentationSetupWorkspace } from "./documentation-setup-workspace";


function DocumentationDecisionBadge({ decision }: { decision: SourcePublicationDocumentSelection["decision"] | "unreviewed" }) {
  const { t } = useTranslation();
  const color = decision === "included" ? "green" : decision === "quarantined" ? "red" : decision === "excluded" ? "amber" : "zinc";
  return <Badge color={color}>{decision === "unreviewed" ? t("documentationExplorer.unreviewed") : enumLabel(t, decision)}</Badge>;
}

export function DocumentationReviewHistory({ selections }: { selections: SourcePublicationDocumentSelection[] }) {
  const { t } = useTranslation();
  if (selections.length === 0) {
    return <section className="developer-document-review-empty" aria-label={t("documentationExplorer.sourcePublicationReviewState")}>
      <DocumentationDecisionBadge decision="unreviewed" />
      <span><strong>{t("documentationExplorer.unreviewed")}</strong><small>{t("documentationExplorer.noSourcePublicationHasRecordedADecisionForThis")}</small></span>
    </section>;
  }
  const latest = selections[0];
  return <section className="developer-document-review-history" aria-label={t("documentationExplorer.sourcePublicationReviewHistory")}>
    <div className="developer-document-review-current">
      <span><strong>{t("documentationExplorer.latestPersistedDecision")}</strong><small>{latest.reason || t("documentationExplorer.exactSourcePublication", { source_publication_id: String(latest.source_publication_id) })}</small></span>
      <DocumentationDecisionBadge decision={latest.decision} />
    </div>
    <header><span><strong>{t("documentationExplorer.sourcePublicationReviewHistory")}</strong><small>{selections.length} {t("documentationExplorer.immutableDecision")}{selections.length === 1 ? "" : t("documentationExplorer.s")}{t("documentationExplorer.newestFirst")}</small></span></header>
    <div className="developer-asset-record-list">
      {selections.map((selection, index) => <article key={`${selection.source_publication_id}:${selection.documentation_document_id}`}>
        <header><span><DocumentationDecisionBadge decision={selection.decision} />{index === 0 && <Badge>{t("documentationExplorer.latest")}</Badge>}</span><small>{t("format.dateTime", { value: new Date(selection.reviewed_at) })}</small></header>
        {(selection.decision === "excluded" || selection.decision === "quarantined") && <p className="developer-document-review-reason"><strong>{t("documentationExplorer.retainedReason")}</strong> {selection.reason}</p>}
        <dl className="entity-detail-grid">
          <div><dt>{t("documentationExplorer.sourcePublication")}</dt><dd><code>{selection.source_publication_id}</code></dd></div>
          <div><dt>{t("documentationExplorer.reviewer")}</dt><dd>{selection.reviewed_by}</dd></div>
          <div><dt>{t("documentationExplorer.publicationOrdinal")}</dt><dd>{selection.ordinal ?? t("documentationExplorer.notIncluded")}</dd></div>
          <div><dt>{t("documentationExplorer.reviewed")}</dt><dd>{t("format.dateTime", { value: new Date(selection.reviewed_at) })}</dd></div>
          <div><dt>{t("documentationExplorer.contentHash")}</dt><dd><code>{selection.content_hash}</code></dd></div>
        </dl>
      </article>)}
    </div>
  </section>;
}

type DocumentationExplorerProps = {
  live: boolean; sources: Source[]; integrations: APIIntegration[];
  setupSearch?: string; reviewerID?: string; onSourceChanged?: (source: APISource) => void;
  onMessage: (message: string) => void; onNavigate: (path: string) => void;
  onAddSource: () => void; onReviewSource: (source: Source) => void;
};
export function DocumentationExplorerView(props: DocumentationExplorerProps) {
  const selection = useMemo(() => parseDocumentationSetupSelection(props.setupSearch), [props.setupSearch]);
  if (props.live && selection.setup) return <DocumentationSetupWorkspace key={selection.api} selection={selection} reviewerID={props.reviewerID ?? ""} onSourceChanged={props.onSourceChanged} onNavigate={props.onNavigate} onMessage={props.onMessage} />;
  return <DocumentationLibraryView {...props} onAddSource={props.live ? () => props.onNavigate(documentationSetupPath({ setup: "new" })) : props.onAddSource} onReviewSource={props.live ? (source) => props.onNavigate(documentationSetupPath({ source: source.id })) : props.onReviewSource} />;
}

function DocumentationLibraryView({ live, sources, integrations, reviewerID, onMessage, onNavigate, onAddSource, onReviewSource }: DocumentationExplorerProps) {
  const { t } = useTranslation();
  const listRequest = useRef(0);
  const [detailAttempt, setDetailAttempt] = useState(0);
  const [documents, setDocuments] = useState<DocumentationLibraryItem[]>([]);
  const [view, setView] = useState<"reviewed" | "attention" | "history" | "sets">("reviewed");
  const attentionRequest = useRef(0);
  const [attention, setAttention] = useState<DocumentationAttentionPage | null>(null);
  const [attentionProblem, setAttentionProblem] = useState("");
  const [attentionLoading, setAttentionLoading] = useState(false);
  const [sourceID, setSourceID] = useState("");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [selectedRecord, setSelectedRecord] = useState<DocumentationCandidateRecord | null>(null);
  const [previous, setPrevious] = useState<DocumentationCandidateRecord | null>(null);
  const [compare, setCompare] = useState(false);
  const [detailProblem, setDetailProblem] = useState("");
  const [previousProblem, setPreviousProblem] = useState("");
  const [runSummary, setRunSummary] = useState<DeveloperAssetIngestionSummary | null>(null);
  const [selectedDocumentIDs, setSelectedDocumentIDs] = useState<string[]>([]);
  const [pendingSetMembers, setPendingSetMembers] = useState<DocumentationCollectionMemberInput[] | null>(null);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(live);
  const [loadingMore, setLoadingMore] = useState(false);
  const [problem, setProblem] = useState("");

  const load = useCallback(async (offset = 0, append = false) => {
    if (!live || view === "sets" || view === "attention") return;
    const request = ++listRequest.current;
    if (append) setLoadingMore(true); else setLoading(true);
    setProblem("");
    try {
      const page = await developerAssetsApi.documentationLibrary({ view, source_id: sourceID, query: submittedQuery, limit: 50, offset });
      if (request !== listRequest.current) return;
      setDetailProblem("");
      setDocuments((current) => append ? [...current, ...page.items.filter((item) => !current.some((other) => other.id === item.id))] : page.items);
      setTotal(page.total); setHasMore(page.has_more);
      if (!append) {
        setSelectedID((current) => page.items.some((item) => item.id === current) ? current : page.items[0]?.id ?? "");
        setSelectedDocumentIDs([]);
      }
    } catch (error) { if (request === listRequest.current) setProblem(developerAssetError(error, t("documentationExplorer.normalizedDocumentationCouldNotBeLoaded"))); }
    finally { if (request === listRequest.current) { setLoading(false); setLoadingMore(false); } }
  }, [live, sourceID, submittedQuery, t, view]);

  const loadAttention = useCallback(async (offset = 0) => {
    if (!live) return;
    const request = ++attentionRequest.current;
    setAttentionLoading(true); setAttentionProblem("");
    if (!offset) setAttention(null);
    try {
      const page = await developerAssetsApi.documentationAttention(sourceID, view === "attention" ? 50 : 1, offset);
      if (request === attentionRequest.current) setAttention((current) => offset && current ? { ...page, items: [...current.items, ...page.items.filter((item) => !current.items.some((other) => other.source_id === item.source_id))] } : page);
    } catch (error) {
      if (request === attentionRequest.current) setAttentionProblem(developerAssetError(error, t("documentationExplorer.attentionUnavailable")));
    } finally { if (request === attentionRequest.current) setAttentionLoading(false); }
  }, [live, sourceID, t, view]);
  const invalidateAttention = useCallback(() => { attentionRequest.current++; }, []);
  useEffect(() => {
    const timer = window.setTimeout(() => void loadAttention(), 0);
    const refresh = () => void loadAttention();
    window.addEventListener("focus", refresh);
    return () => { window.clearTimeout(timer); window.removeEventListener("focus", refresh); invalidateAttention(); };
  }, [invalidateAttention, loadAttention]);

  const invalidateList = useCallback(() => { listRequest.current++; }, []);
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    const refresh = () => void load();
    window.addEventListener("focus", refresh);
    return () => { window.clearTimeout(timer); window.removeEventListener("focus", refresh); invalidateList(); };
  }, [invalidateList, load]);
  const selected = documents.find((item) => item.id === selectedID);
  const source = sources.find((item) => item.id === selected?.source_id);
  const record = selectedRecord?.document.id === selectedID ? selectedRecord : null;
  const reviewedSourcePublicationID = record?.source_publication_selections.find((selection) => selection.decision === "included" && selection.content_hash === record.document.content_hash)?.source_publication_id;

  useEffect(() => {
    if (!live || !selectedID || view === "sets" || view === "attention") return;
    let cancelled = false;
    developerAssetsApi.documentationDocument(selectedID).then((value) => {
      if (!cancelled) { setSelectedRecord(value); setDetailProblem(""); }
    }).catch((error) => { if (!cancelled) setDetailProblem(developerAssetError(error, t("documentationExplorer.detailUnavailable"))); });
    return () => { cancelled = true; };
  }, [detailAttempt, live, selectedID, t, view]);

  useEffect(() => {
    if (view === "sets" || view === "attention" || !compare || !selected?.previous_document_id) return;
    let cancelled = false;
    developerAssetsApi.documentationDocument(selected.previous_document_id).then((value) => {
      if (!cancelled) { setPrevious(value); setPreviousProblem(""); }
    }).catch((error) => { if (!cancelled) setPreviousProblem(developerAssetError(error, t("documentationExplorer.detailUnavailable"))); });
    return () => { cancelled = true; };
  }, [compare, selected?.previous_document_id, t, view]);

  function selectDocument(id: string) {
    setSelectedID(id); setCompare(false); setPrevious(null); setPreviousProblem(""); setDetailProblem(""); setRunSummary(null);
  }
  function createSetFromSelection() {
    setPendingSetMembers(selectedDocumentIDs.map((id) => ({ kind: "document", id, include_descendants: true, selector: {} })));
    setView("sets");
  }
  const change = record && previous && previous.document.id === selected?.previous_document_id ? evidenceChange(previous.document.normalized_markdown, record.document.normalized_markdown) : null;
  const active: Section = "documents";
  const filtered = Boolean(sourceID || submittedQuery);

  return <>
    <PageHeader eyebrow={t("navigation.docs")} title={t("documentationExplorer.library")} description={t("documentationExplorer.libraryDescription")} action={<Button onClick={onAddSource}><Plus data-slot="icon" />{t("documentationExplorer.addContent")}</Button>} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <SegmentedControl label={t("documentationExplorer.libraryView")} value={view} onChange={(value) => { setView(value); setAttention(null); setAttentionLoading(live); setCompare(false); setDetailProblem(""); }} items={[
      { id: "reviewed", label: t("documentationExplorer.reviewedContent") }, { id: "attention", label: t("documentationExplorer.needsAttention"), count: attentionProblem || attentionLoading ? undefined : attention?.total },
    ]} />
    <details className="advanced-details"><summary>{t("documentationExplorer.historyAndSets")}</summary><div className="heading-actions">
      <Button outline onClick={() => { setView("history"); setCompare(false); }}>{t("documentationExplorer.importHistory")}</Button>
      <Button outline onClick={() => { setView("sets"); setCompare(false); }}>{t("documentationExplorer.documentationSets")}</Button>
    </div></details>
    {view === "reviewed" && attentionProblem && <ProblemPanel message={attentionProblem} onRetry={() => void loadAttention()} />}
    {view === "attention" ? <section className="panel documentation-attention-panel">
      <PanelHeader title={t("documentationExplorer.needsAttention")} description={t("documentationExplorer.attentionDescription")} action={<Button outline disabled={attentionLoading} onClick={() => void loadAttention()}>{t("documentationExplorer.refreshPending")}</Button>} />
      <select aria-label={t("documentationExplorer.sourceFilter")} value={sourceID} onChange={(event) => { setSourceID(event.target.value); setAttention(null); setAttentionLoading(live); }}><option value="">{t("documentationExplorer.allSources")}</option>{sources.filter((item) => item.kind === "website" || item.kind === "upload").map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
      {attentionProblem ? <ProblemPanel message={attentionProblem} onRetry={() => void loadAttention()} /> : attentionLoading && !attention ? <LoadingPanel label={t("common.loading")} /> : <>
        {attention?.items.map((item) => <article className="documentation-attention-row" key={item.source_id}>
          <div><h3>{item.name}</h3><p>{t(`documentationExplorer.attentionHelp.${item.status}`)}</p><small>{t("format.dateTime", { value: new Date(item.updated_at) })}</small></div>
          <div className="documentation-attention-actions"><Badge color={item.status === "blocked" || item.status === "import_failed" ? "red" : "amber"}>{t(`documentationExplorer.attentionStatus.${item.status}`)}</Badge><Button outline onClick={() => onNavigate(documentationSetupPath({ source: item.source_id, run: item.crawl_job_id }))}>{t("documentationExplorer.continueSetup")}</Button></div>
        </article>)}
        {!attentionLoading && attention?.total === 0 && <p>{t("documentationExplorer.noPendingImports")}</p>}
        {attention?.has_more && <Button outline disabled={attentionLoading} onClick={() => void loadAttention(attention.items.length)}>{attentionLoading ? t("common.loading") : t("documentationExplorer.loadMore")}</Button>}
      </>}
    </section> :
    view === "sets" ? <DocumentationCollectionsView reviewerID={reviewerID} live={live} integrations={integrations} onMessage={onMessage} onNavigate={onNavigate} embedded startCreate={pendingSetMembers !== null} initialMembers={pendingSetMembers ?? []} initialMemberLabels={Object.fromEntries(documents.map((document) => [document.id, document.title]))} onCreateStarted={() => setPendingSetMembers(null)} /> : <>
      <form className="toolbar developer-asset-search" onSubmit={(event) => { event.preventDefault(); setSubmittedQuery(query.trim()); }}>
        <div className="search-field"><Search /><input aria-label={t("documentationExplorer.searchDocuments")} placeholder={t("documentationExplorer.searchPathsTitlesAndContent")} value={query} onChange={(event) => setQuery(event.target.value)} /></div>
        <select aria-label={t("documentationExplorer.sourceFilter")} value={sourceID} onChange={(event) => { setSourceID(event.target.value); setAttention(null); setAttentionLoading(live); }}><option value="">{t("documentationExplorer.allSources")}</option>{sources.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <Button type="submit" outline>{t("documentationExplorer.search")}</Button>
        {submittedQuery && <Button type="button" outline onClick={() => { setQuery(""); setSubmittedQuery(""); }}>{t("documentationExplorer.clear")}</Button>}
        {selectedDocumentIDs.length > 0 && <Button type="button" outline onClick={createSetFromSelection}>{t("documentationExplorer.saveSelectionAsSet")}</Button>}
      </form>
      {loading ? <LoadingPanel label={t("documentationExplorer.loadingLibrary")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : documents.length === 0 ? <section className="panel developer-asset-inspector-empty">
        <FileText />
        <strong>{t(filtered ? "documentationExplorer.noMatchingDocuments" : view === "reviewed" ? "documentationExplorer.noReviewedFiles" : "documentationExplorer.noFiles")}</strong>
        <small>{t(filtered ? "documentationExplorer.noMatchingDocumentsHelp" : view === "reviewed" ? "documentationExplorer.reviewedEmptyHelp" : "documentationExplorer.addContentHelp")}</small>
        {filtered ? <Button outline onClick={() => { setQuery(""); setSubmittedQuery(""); setSourceID(""); }}>{t("documentationExplorer.clearFilters")}</Button> : <div className="heading-actions">
          {!attentionProblem && !attentionLoading && Boolean(attention?.total) && <Button onClick={() => { setView("attention"); setAttention(null); setAttentionLoading(live); }}>{t("documentationExplorer.needsAttention")}</Button>}
          <Button outline onClick={onAddSource}>{t("documentationExplorer.addContent")}</Button>
        </div>}
      </section> : <div className="developer-asset-explorer">
        <aside className="panel documentation-file-navigator" aria-label={t("documentationExplorer.fileNavigator")}>
          <header className="documentation-file-navigator-heading"><strong>{view === "history" ? t("documentationExplorer.importHistory") : t("documentationExplorer.reviewedContent")}</strong><small>{t("documentationExplorer.documentsShown", { shown: documents.length, count: total })}</small></header>
          <div className="documentation-file-tree">{documents.map((item) => <div className={`documentation-file-row ${item.id === selectedID ? "active" : ""}`} key={item.id}>
            <input type="checkbox" aria-label={t("documentationExplorer.selectDocument", { title: item.title })} checked={selectedDocumentIDs.includes(item.id)} onChange={(event) => setSelectedDocumentIDs((current) => event.target.checked ? [...current, item.id] : current.filter((id) => id !== item.id))} />
            <button type="button" onClick={() => selectDocument(item.id)}><FileText /><span><strong>{item.title || item.source_path}</strong><small>{sources.find((value) => value.id === item.source_id)?.name}{sources.find((value) => value.id === item.source_id)?.kind !== "upload" && <> · {item.source_path}</>}</small>{view === "history" && <small>{t("format.dateTime", { value: new Date(item.queued_at) })}</small>}</span><DocumentationDecisionBadge decision={item.decision} /></button>
          </div>)}</div>
          {hasMore && <Button outline disabled={loadingMore} onClick={() => void load(documents.length, true)}>{loadingMore ? t("common.loading") : t("documentationExplorer.loadMore")}</Button>}
        </aside>
        <section className="panel developer-asset-inspector">
          {selected ? <>
            <PanelHeader title={selected.title || selected.source_path} description={source?.kind === "upload" ? source.name : selected.source_path} action={<span className="heading-actions"><Badge>{selected.visibility}</Badge>{source && view === "reviewed" && <Button outline onClick={() => onReviewSource(source)}>{t("documentationExplorer.reviewLatestImport")}</Button>}</span>} />
            <div className="developer-asset-inspector-body">
              <p className="developer-asset-help">{t("documentationExplorer.reviewIsNotDelivery")}</p>
              {detailProblem ? <ProblemPanel message={detailProblem} onRetry={() => { setDetailProblem(""); setDetailAttempt((value) => value + 1); }} /> : !record ? <LoadingPanel label={t("documentationExplorer.loadingContent")} /> : <>
                {selected.previous_document_id && <Button outline onClick={() => setCompare((value) => !value)}>{compare ? t("documentationExplorer.readContent") : t("documentationExplorer.comparePrevious")}</Button>}
                {compare ? previousProblem ? <p role="alert">{previousProblem}</p> : !change ? <LoadingPanel label={t("documentationExplorer.loadingComparison")} /> : change.unchanged ? <p>{t("documentationExplorer.noContentChanges")}</p> : <div className="core-evidence-diff"><section><h3>{t("documentationExplorer.removedText")}</h3><pre><code>{change.removed || t("documentationExplorer.none")}</code></pre></section><section><h3>{t("documentationExplorer.addedText")}</h3><pre><code>{change.added || t("documentationExplorer.none")}</code></pre></section></div> : <MarkdownEvidence label={t("documentationExplorer.documentContent")}>{record.document.normalized_markdown}</MarkdownEvidence>}
                <details className="advanced-details"><summary>{t("documentationExplorer.reviewHistory")}</summary><DocumentationReviewHistory selections={record.source_publication_selections} /></details>
                <details className="advanced-details" onToggle={(event) => { if (event.currentTarget.open && runSummary?.run.id !== record.run.id) void developerAssetsApi.ingestionRun(record.run.id).then(setRunSummary).catch((error) => onMessage(developerAssetError(error, t("documentationExplorer.diagnosticsUnavailable")))); }}>
                  <summary>{t("documentationExplorer.processingDetails")}</summary>
                  <DeveloperAssetAIAdvisoryButton input={reviewedSourcePublicationID ? { prompt_key: "documentation.map_enrichment", source_publication_id: reviewedSourcePublicationID } : null} subject={t("documentationExplorer.reviewedSourcePublicationSubject", { name: source?.name ?? selected.title })} label={t("documentationExplorer.aiMapAdvisory")} unavailableReason={t("documentationExplorer.reviewedPublicationRequiredForAI")} />
                  <dl className="entity-detail-grid"><div><dt>{t("documentationExplorer.contentHash")}</dt><dd><code>{record.document.content_hash}</code></dd></div><div><dt>{t("documentationExplorer.ingestionRun")}</dt><dd><code>{record.run.id}</code></dd></div></dl>
                  {record.documentation_map && <details className="advanced-details"><summary>{t("documentationExplorer.navigationMap")}</summary><MarkdownEvidence label={t("documentationExplorer.documentationMapAgentMarkdown")}>{record.documentation_map.agent_markdown}</MarkdownEvidence></details>}
                  <PrettyJSON value={{ metadata: record.document.metadata, processing: runSummary?.run.id === record.run.id ? runSummary : record.run }} label={t("documentationExplorer.documentationDiagnosticsAndSourcePublicationReviewHistory")} />
                </details>
              </>}
            </div>
          </> : <div className="developer-asset-inspector-empty"><FileText /><strong>{t("documentationExplorer.selectAFile")}</strong></div>}
        </section>
      </div>}
    </>}
  </>;
}
