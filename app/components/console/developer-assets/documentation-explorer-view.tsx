"use client";


import { useTranslation } from "react-i18next";
import { BookOpen, FileText, RefreshCw, Search } from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";

import type { Section } from "../../../lib/console-routes";
import type { Source } from "../../../lib/console-domain";
import {
  developerAssetsApi,
  type DeveloperAssetIngestionSummary,
  type DocumentationCandidateRecord,
  type SourcePublicationDocumentSelection,
} from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { DeveloperAssetAIAdvisoryButton } from "./developer-asset-ai-advisory";
import { developerAssetError, enumLabel, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";

type InspectorTab = "detail" | "sections" | "map" | "diagnostics" | "run";

function DocumentationDecisionBadge({ decision }: { decision: SourcePublicationDocumentSelection["decision"] | "unreviewed" }) {
  const { t } = useTranslation();
  const color = decision === "included" ? "green" : decision === "quarantined" ? "red" : decision === "excluded" ? "amber" : "zinc";
  return <Badge color={color}>{decision === "unreviewed" ? t("documentationExplorer.unreviewed") : enumLabel(t, decision)}</Badge>;
}

function DocumentationDecisionCell({ record }: { record: DocumentationCandidateRecord }) {
  const { t } = useTranslation();
  const latest = record.source_publication_selections[0];
  return <span className="developer-document-decision-cell">
    <DocumentationDecisionBadge decision={latest?.decision ?? "unreviewed"} />
    <small className="cell-note">{latest ? latest.reason || t("documentationExplorer.publication", { source_publication_id: String(latest.source_publication_id) }) : t("documentationExplorer.noSourcePublicationDecision")}</small>
  </span>;
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

export function DocumentationExplorerView({ live, sources, onNavigate }: { live: boolean; sources: Source[]; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [documents, setDocuments] = useState<DocumentationCandidateRecord[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [tab, setTab] = useState<InspectorTab>("detail");
  const [runSummary, setRunSummary] = useState<DeveloperAssetIngestionSummary | null>(null);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [reviewedPublicationID, setReviewedPublicationID] = useState("");
  const [reviewCheckPending, setReviewCheckPending] = useState(false);

  const load = useCallback(async (offset = 0, append = false) => {
    if (!live) return;
    if (append) setLoadingMore(true);
    else setLoading(true);
    setProblem("");
    try {
      const page = await developerAssetsApi.documentationDocuments({ query: submittedQuery, limit: 100, offset });
      setDocuments((current) => append ? [...current, ...page.items.filter((item) => !current.some((existing) => existing.document.id === item.document.id))] : page.items);
      setTotal(page.total);
      setHasMore(page.has_more);
      setSelectedID((current) => append || page.items.some((item) => item.document.id === current) ? current : page.items[0]?.document.id ?? "");
    } catch (error) {
      setProblem(developerAssetError(error, t("documentationExplorer.normalizedDocumentationCouldNotBeLoaded")));
    } finally {
      if (append) setLoadingMore(false);
      else setLoading(false);
    }
  }, [live, submittedQuery, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(0); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  const selected = useMemo(() => documents.find((item) => item.document.id === selectedID) ?? null, [documents, selectedID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selected?.run.id) {
      queueMicrotask(() => { if (!cancelled) setRunSummary(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.ingestionRun(selected.run.id).then((value) => { if (!cancelled) setRunSummary(value); }).catch(() => { if (!cancelled) setRunSummary(null); });
    return () => { cancelled = true; };
  }, [live, selected?.run.id]);

  useEffect(() => {
    let cancelled = false;
    const latestPublication = sources.find((source) => source.id === selected?.run.source_id)?.latestPublication;
    if (!live || !selected || !latestPublication || !selected.documentation_map) {
      queueMicrotask(() => { if (!cancelled) { setReviewedPublicationID(""); setReviewCheckPending(false); } });
      return () => { cancelled = true; };
    }
    const selectedMapID = selected.documentation_map.id;
    const selectedMapHash = selected.documentation_map.content_hash;
    queueMicrotask(() => { if (!cancelled) { setReviewedPublicationID(""); setReviewCheckPending(true); } });
    developerAssetsApi.documentationDocuments({ source_publication_id: latestPublication.id, query: selected.document.source_path, limit: 100, offset: 0 }).then((page) => {
      if (cancelled) return;
      const reviewed = page.items.some((record) => record.document.id === selected.document.id && record.documentation_map?.id === selectedMapID && record.documentation_map?.content_hash === selectedMapHash);
      setReviewedPublicationID(reviewed ? latestPublication.id : "");
    }).catch(() => { if (!cancelled) setReviewedPublicationID(""); }).finally(() => { if (!cancelled) setReviewCheckPending(false); });
    return () => { cancelled = true; };
  }, [live, selected, sources]);

  function submitSearch(event: FormEvent) {
    event.preventDefault();
    setSubmittedQuery(query.trim());
  }

  const documentOutline = selected ? {
    document_id: selected.document.id,
    source_path: selected.document.source_path,
    sections: selected.sections.map((section) => ({ id: section.id, heading: section.heading, breadcrumb: section.breadcrumb, anchor: section.anchor })),
  } : {};
  const active: Section = "documents";

  return <>
    <PageHeader eyebrow={t("navigation.docs")} title={t("documentationExplorer.allFiles")} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <form className="toolbar developer-asset-search" onSubmit={submitSearch}>
      <div className="search-field"><Search /><input aria-label={t("documentationExplorer.searchAllNormalizedFiles")} placeholder={t("documentationExplorer.searchPathsTitlesAndContent")} value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <Button type="submit" outline>{t("documentationExplorer.search")}</Button>
      {submittedQuery && <Button type="button" outline onClick={() => { setQuery(""); setSubmittedQuery(""); }}>{t("documentationExplorer.clear")}</Button>}
      <span className="toolbar-count">{t("documentationExplorer.filesShown", { shown: documents.length, count: total })}</span>
      {hasMore && <Button type="button" outline disabled={loadingMore} onClick={() => void load(documents.length, true)}>{loadingMore ? t("common.loading") : t("documentationExplorer.loadMore")}</Button>}
    </form>
    {loading ? <LoadingPanel label={t("documentationExplorer.loadingNormalizedDocumentation")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label={t("documentationExplorer.normalizedDocumentationFiles")} className="developer-asset-directory">
        <DataTableHeader className="developer-document-columns"><span>{t("documentationExplorer.file")}</span><span>{t("documentationExplorer.latestDecision")}</span><span>{t("documentationExplorer.sections")}</span></DataTableHeader>
        {documents.map((record) => <DataTableRow className={`developer-document-columns developer-asset-selectable ${record.document.id === selectedID ? "selected" : ""}`} key={record.document.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedID(record.document.id); setTab("detail"); }}><span className="resource-icon"><FileText /></span><span><strong>{record.document.title || record.document.source_path}</strong><small>{record.document.source_path}</small></span></button>
          <DocumentationDecisionCell record={record} />
          <span><strong className="cell-value">{record.sections.length}</strong><small className="cell-note">{t("documentationExplorer.normalized")}</small></span>
        </DataTableRow>)}
        {documents.length === 0 && <DataTableEmpty columns={3}>{submittedQuery ? t("documentationExplorer.noNormalizedFilesMatchThisSearch") : t("documentationExplorer.noDocumentationFilesHaveBeenNormalizedYet")}</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.document.title || selected.document.source_path} description={t("documentationExplorer.copy", { document_kind: String(selected.document.document_kind), media_type: String(selected.document.media_type) })} action={<span className="heading-actions"><Badge color={selected.document.visibility === "public" ? "blue" : "zinc"}>{selected.document.visibility}</Badge><DeveloperAssetAIAdvisoryButton input={reviewedPublicationID ? { prompt_key: "documentation.map_enrichment", source_publication_id: reviewedPublicationID } : null} subject={t("documentationExplorer.reviewedSourcePublicationSubject", { name: selected.document.title || selected.document.source_path })} label={t("documentationExplorer.aiMapAdvisory")} unavailableReason={reviewCheckPending ? t("documentationExplorer.checkingExactReview") : t("documentationExplorer.reviewedPublicationRequiredForAI")} /></span>} />
          <div className="developer-asset-inspector-tabs"><SegmentedControl label={t("documentationExplorer.documentInspector")} value={tab} onChange={setTab} items={[
            { id: "detail", label: t("common.detail") }, { id: "sections", label: t("common.sections"), count: selected.sections.length }, { id: "map", label: t("common.map") }, { id: "diagnostics", label: t("common.diagnostics") }, { id: "run", label: t("common.runStatus") },
          ]} /></div>
          <div className="developer-asset-inspector-body">
            {tab === "detail" && <><DocumentationReviewHistory selections={selected.source_publication_selections} /><dl className="entity-detail-grid"><div><dt>{t("documentationExplorer.sourcePath")}</dt><dd><code>{selected.document.source_path}</code></dd></div><div><dt>{t("documentationExplorer.contentHash")}</dt><dd><code>{selected.document.content_hash}</code></dd></div><div><dt>{t("documentationExplorer.language")}</dt><dd>{selected.document.language || "—"}</dd></div><div><dt>{t("documentationExplorer.ingestionRun")}</dt><dd><code>{selected.run.id}</code></dd></div>{selected.document.canonical_url && <div><dt>{t("documentationExplorer.canonicalURL")}</dt><dd><a href={selected.document.canonical_url} target="_blank" rel="noreferrer">{t("documentationExplorer.openSource")}</a></dd></div>}</dl><pre className="developer-asset-markdown"><code>{selected.document.normalized_markdown}</code></pre></>}
            {tab === "sections" && <div className="developer-asset-section-list">{selected.sections.map((section) => <article key={section.id}><header><span><BookOpen /><strong>{section.heading || section.breadcrumb.at(-1) || t("documentationExplorer.untitledSection")}</strong></span><Badge>{t("documentationExplorer.tokens", { count: section.token_estimate })}</Badge></header><small>{section.breadcrumb.join(" / ")}{section.anchor ? t("documentationExplorer.copy2", { anchor: String(section.anchor) }) : ""}</small><pre><code>{section.normalized_text}</code></pre></article>)}{selected.sections.length === 0 && <p className="empty-row">{t("documentationExplorer.noSectionsWereEmittedForThisFile")}</p>}</div>}
            {tab === "map" && <>{selected.documentation_map ? <><p className="developer-asset-help">{t("documentationExplorer.thisPersistedMapIsAnInspectableNavigationArtifactIt")}</p><dl className="entity-detail-grid"><div><dt>{t("documentationExplorer.mapVersion")}</dt><dd>{selected.documentation_map.map_version}</dd></div><div><dt>{t("documentationExplorer.contentHash")}</dt><dd><code>{selected.documentation_map.content_hash}</code></dd></div><div><dt>{t("documentationExplorer.mapID")}</dt><dd><code>{selected.documentation_map.id}</code></dd></div><div><dt>{t("documentationExplorer.visibility")}</dt><dd>{selected.documentation_map.visibility ?? selected.document.visibility}</dd></div></dl><MarkdownEvidence label={t("documentationExplorer.documentationMapAgentMarkdown")}>{selected.documentation_map.agent_markdown}</MarkdownEvidence><PrettyJSON value={selected.documentation_map.map} label={t("documentationExplorer.documentationMapData")} /></> : <><p className="developer-asset-help">{t("documentationExplorer.noPersistedDocumentationMapIsAvailableForThisOlder")}</p><PrettyJSON value={documentOutline} label={t("documentationExplorer.derivedDocumentOutline")} /></>}</>}
            {tab === "diagnostics" && <PrettyJSON value={{ document: selected.document.metadata, run: selected.run.diagnostics, source_publication_review: { latest_decision: selected.source_publication_selections[0] ?? { decision: "unreviewed", reason: t("documentationExplorer.noPersistedDecision") }, history_newest_first: selected.source_publication_selections } }} label={t("documentationExplorer.documentationDiagnosticsAndSourcePublicationReviewHistory")} />}
            {tab === "run" && <div className="developer-asset-run"><div className="developer-asset-run-summary"><RefreshCw /><span><strong>{enumLabel(t, selected.run.state)}</strong><small>{selected.run.acquired_count} {t("documentationExplorer.acquired")} {selected.run.failed_count} {t("documentationExplorer.failed")} {t("documentationExplorer.quarantinedCount", { count: selected.run.quarantined_count })}</small></span><ReviewStateBadge state={selected.run.state} /></div><dl className="entity-detail-grid"><div><dt>{t("documentationExplorer.target")}</dt><dd>{selected.run.target_key}</dd></div><div><dt>{t("documentationExplorer.attempt")}</dt><dd>{selected.run.attempt}</dd></div><div><dt>{t("documentationExplorer.queued")}</dt><dd>{t("format.dateTime", { value: new Date(selected.run.queued_at) })}</dd></div><div><dt>{t("documentationExplorer.finished")}</dt><dd>{selected.run.finished_at ? t("format.dateTime", { value: new Date(selected.run.finished_at) }) : "—"}</dd></div></dl><div className="developer-asset-stage-list">{runSummary?.stages.map((stage) => <div key={stage.id}><span><strong>{stage.stage_name}</strong><small>{t("documentationExplorer.attempt")} {stage.attempt}</small></span><ReviewStateBadge state={stage.state} /></div>)}{!runSummary && <small>{t("documentationExplorer.stageCheckpointsAreUnavailable")}</small>}</div></div>}
          </div>
        </> : <div className="developer-asset-inspector-empty"><FileText /><strong>{t("documentationExplorer.selectAFile")}</strong><small>{t("documentationExplorer.itsExactContentSectionsMapDiagnosticsAndRunStatus")}</small></div>}
      </section>
    </div>}
  </>;
}
