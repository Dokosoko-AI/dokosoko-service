"use client";

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
import { developerAssetError, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";

type InspectorTab = "detail" | "sections" | "map" | "diagnostics" | "run";

function DocumentationDecisionBadge({ decision }: { decision: SourcePublicationDocumentSelection["decision"] | "unreviewed" }) {
  const color = decision === "included" ? "green" : decision === "quarantined" ? "red" : decision === "excluded" ? "amber" : "zinc";
  return <Badge color={color}>{decision}</Badge>;
}

function DocumentationDecisionCell({ record }: { record: DocumentationCandidateRecord }) {
  const latest = record.source_publication_selections[0];
  return <span className="developer-document-decision-cell">
    <DocumentationDecisionBadge decision={latest?.decision ?? "unreviewed"} />
    <small className="cell-note">{latest ? latest.reason || `Publication ${latest.source_publication_id}` : "No source-publication decision"}</small>
  </span>;
}

export function DocumentationReviewHistory({ selections }: { selections: SourcePublicationDocumentSelection[] }) {
  if (selections.length === 0) {
    return <section className="developer-document-review-empty" aria-label="Source publication review state">
      <DocumentationDecisionBadge decision="unreviewed" />
      <span><strong>Unreviewed</strong><small>No source publication has recorded a decision for this exact persisted document.</small></span>
    </section>;
  }
  const latest = selections[0];
  return <section className="developer-document-review-history" aria-label="Source publication review history">
    <div className="developer-document-review-current">
      <span><strong>Latest persisted decision</strong><small>{latest.reason || `Exact source publication ${latest.source_publication_id}`}</small></span>
      <DocumentationDecisionBadge decision={latest.decision} />
    </div>
    <header><span><strong>Source publication review history</strong><small>{selections.length} immutable decision{selections.length === 1 ? "" : "s"}, newest first</small></span></header>
    <div className="developer-asset-record-list">
      {selections.map((selection, index) => <article key={`${selection.source_publication_id}:${selection.documentation_document_id}`}>
        <header><span><DocumentationDecisionBadge decision={selection.decision} />{index === 0 && <Badge>latest</Badge>}</span><small>{new Date(selection.reviewed_at).toLocaleString()}</small></header>
        {(selection.decision === "excluded" || selection.decision === "quarantined") && <p className="developer-document-review-reason"><strong>Retained reason:</strong> {selection.reason}</p>}
        <dl className="entity-detail-grid">
          <div><dt>Source publication</dt><dd><code>{selection.source_publication_id}</code></dd></div>
          <div><dt>Reviewer</dt><dd>{selection.reviewed_by}</dd></div>
          <div><dt>Publication ordinal</dt><dd>{selection.ordinal ?? "Not included"}</dd></div>
          <div><dt>Reviewed</dt><dd>{new Date(selection.reviewed_at).toLocaleString()}</dd></div>
          <div><dt>Content hash</dt><dd><code>{selection.content_hash}</code></dd></div>
        </dl>
      </article>)}
    </div>
  </section>;
}

export function DocumentationExplorerView({ live, sources, onNavigate }: { live: boolean; sources: Source[]; onNavigate: (path: string) => void }) {
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
      setProblem(developerAssetError(error, "Normalized documentation could not be loaded."));
    } finally {
      if (append) setLoadingMore(false);
      else setLoading(false);
    }
  }, [live, submittedQuery]);

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
    <PageHeader eyebrow="Docs" title="All files" />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <form className="toolbar developer-asset-search" onSubmit={submitSearch}>
      <div className="search-field"><Search /><input aria-label="Search all normalized files" placeholder="Search paths, titles, and content…" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <Button type="submit" outline>Search</Button>
      {submittedQuery && <Button type="button" outline onClick={() => { setQuery(""); setSubmittedQuery(""); }}>Clear</Button>}
      <span className="toolbar-count">{documents.length} of {total} file{total === 1 ? "" : "s"}</span>
      {hasMore && <Button type="button" outline disabled={loadingMore} onClick={() => void load(documents.length, true)}>{loadingMore ? "Loading…" : "Load more"}</Button>}
    </form>
    {loading ? <LoadingPanel label="Loading normalized documentation" /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label="Normalized documentation files" className="developer-asset-directory">
        <DataTableHeader className="developer-document-columns"><span>File</span><span>Latest decision</span><span>Sections</span></DataTableHeader>
        {documents.map((record) => <DataTableRow className={`developer-document-columns developer-asset-selectable ${record.document.id === selectedID ? "selected" : ""}`} key={record.document.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedID(record.document.id); setTab("detail"); }}><span className="resource-icon"><FileText /></span><span><strong>{record.document.title || record.document.source_path}</strong><small>{record.document.source_path}</small></span></button>
          <DocumentationDecisionCell record={record} />
          <span><strong className="cell-value">{record.sections.length}</strong><small className="cell-note">normalized</small></span>
        </DataTableRow>)}
        {documents.length === 0 && <DataTableEmpty columns={3}>{submittedQuery ? "No normalized files match this search." : "No documentation files have been normalized yet."}</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.document.title || selected.document.source_path} description={`${selected.document.document_kind} · ${selected.document.media_type}`} action={<span className="heading-actions"><Badge color={selected.document.visibility === "public" ? "blue" : "zinc"}>{selected.document.visibility}</Badge><DeveloperAssetAIAdvisoryButton input={reviewedPublicationID ? { prompt_key: "documentation.map_enrichment", source_publication_id: reviewedPublicationID } : null} subject={`${selected.document.title || selected.document.source_path} reviewed source publication`} label="AI map advisory" unavailableReason={reviewCheckPending ? "Checking this document against its exact immutable source publication review…" : "Only a document and map included in an immutable reviewed source publication can use advisory AI."} /></span>} />
          <div className="developer-asset-inspector-tabs"><SegmentedControl label="Document inspector" value={tab} onChange={setTab} items={[
            { id: "detail", label: "Detail" }, { id: "sections", label: "Sections", count: selected.sections.length }, { id: "map", label: "Map" }, { id: "diagnostics", label: "Diagnostics" }, { id: "run", label: "Run status" },
          ]} /></div>
          <div className="developer-asset-inspector-body">
            {tab === "detail" && <><DocumentationReviewHistory selections={selected.source_publication_selections} /><dl className="entity-detail-grid"><div><dt>Source path</dt><dd><code>{selected.document.source_path}</code></dd></div><div><dt>Content hash</dt><dd><code>{selected.document.content_hash}</code></dd></div><div><dt>Language</dt><dd>{selected.document.language || "—"}</dd></div><div><dt>Ingestion run</dt><dd><code>{selected.run.id}</code></dd></div>{selected.document.canonical_url && <div><dt>Canonical URL</dt><dd><a href={selected.document.canonical_url} target="_blank" rel="noreferrer">Open source</a></dd></div>}</dl><pre className="developer-asset-markdown"><code>{selected.document.normalized_markdown}</code></pre></>}
            {tab === "sections" && <div className="developer-asset-section-list">{selected.sections.map((section) => <article key={section.id}><header><span><BookOpen /><strong>{section.heading || section.breadcrumb.at(-1) || "Untitled section"}</strong></span><Badge>{section.token_estimate} tokens</Badge></header><small>{section.breadcrumb.join(" / ")}{section.anchor ? ` · #${section.anchor}` : ""}</small><pre><code>{section.normalized_text}</code></pre></article>)}{selected.sections.length === 0 && <p className="empty-row">No sections were emitted for this file.</p>}</div>}
            {tab === "map" && <>{selected.documentation_map ? <><p className="developer-asset-help">This persisted map is an inspectable navigation artifact. It does not approve the file or fill evidence gaps.</p><dl className="entity-detail-grid"><div><dt>Map version</dt><dd>{selected.documentation_map.map_version}</dd></div><div><dt>Content hash</dt><dd><code>{selected.documentation_map.content_hash}</code></dd></div><div><dt>Map ID</dt><dd><code>{selected.documentation_map.id}</code></dd></div><div><dt>Visibility</dt><dd>{selected.documentation_map.visibility ?? selected.document.visibility}</dd></div></dl><MarkdownEvidence label="Documentation Map agent markdown">{selected.documentation_map.agent_markdown}</MarkdownEvidence><PrettyJSON value={selected.documentation_map.map} label="Documentation Map data" /></> : <><p className="developer-asset-help">No persisted Documentation Map is available for this older record. This fallback outline is derived only from the stored sections and does not approve the file.</p><PrettyJSON value={documentOutline} label="Derived document outline" /></>}</>}
            {tab === "diagnostics" && <PrettyJSON value={{ document: selected.document.metadata, run: selected.run.diagnostics, source_publication_review: { latest_decision: selected.source_publication_selections[0] ?? { decision: "unreviewed", reason: "No persisted source-publication decision exists for this exact document." }, history_newest_first: selected.source_publication_selections } }} label="Documentation diagnostics and source-publication review history" />}
            {tab === "run" && <div className="developer-asset-run"><div className="developer-asset-run-summary"><RefreshCw /><span><strong>{selected.run.state.replaceAll("_", " ")}</strong><small>{selected.run.acquired_count} acquired · {selected.run.failed_count} failed · {selected.run.quarantined_count} quarantined</small></span><ReviewStateBadge state={selected.run.state} /></div><dl className="entity-detail-grid"><div><dt>Target</dt><dd>{selected.run.target_key}</dd></div><div><dt>Attempt</dt><dd>{selected.run.attempt}</dd></div><div><dt>Queued</dt><dd>{new Date(selected.run.queued_at).toLocaleString()}</dd></div><div><dt>Finished</dt><dd>{selected.run.finished_at ? new Date(selected.run.finished_at).toLocaleString() : "—"}</dd></div></dl><div className="developer-asset-stage-list">{runSummary?.stages.map((stage) => <div key={stage.id}><span><strong>{stage.stage_name}</strong><small>Attempt {stage.attempt}</small></span><ReviewStateBadge state={stage.state} /></div>)}{!runSummary && <small>Stage checkpoints are unavailable.</small>}</div></div>}
          </div>
        </> : <div className="developer-asset-inspector-empty"><FileText /><strong>Select a file</strong><small>Its exact content, sections, map, diagnostics, and run status will appear here.</small></div>}
      </section>
    </div>}
  </>;
}
