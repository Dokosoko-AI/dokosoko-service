"use client";

import { Archive, BookOpen, Check, ChevronRight, GitBranch, Plus, Radio } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import type { Section } from "../../../lib/console-routes";
import {
  developerAssetsApi,
  type DeploymentDocumentationPublication,
  type DocumentationCollection,
  type DocumentationCollectionMemberInput,
  type DocumentationCollectionRevision,
  type DocumentationCollectionRevisionRecord,
} from "../../../lib/developer-assets-api";
import { Badge, Button, Dialog } from "../../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { developerAssetError, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";
import { documentationUsages, type DocumentationUsage } from "./developer-asset-usage";

type RevisionTab = "members" | "map" | "manifest";

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

export function DocumentationCollectionsView({ live, integrations, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [collections, setCollections] = useState<DocumentationCollection[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [revisions, setRevisions] = useState<DocumentationCollectionRevision[]>([]);
  const [selectedRevisionID, setSelectedRevisionID] = useState("");
  const [revisionRecord, setRevisionRecord] = useState<DocumentationCollectionRevisionRecord | null>(null);
  const [publications, setPublications] = useState<DeploymentDocumentationPublication[]>([]);
  const [publicationOptions, setPublicationOptions] = useState<DocumentationCollectionRevision[]>([]);
  const [selectedPublicationRevisionIDs, setSelectedPublicationRevisionIDs] = useState<string[]>([]);
  const [publicationVisibility, setPublicationVisibility] = useState<"private" | "public">("private");
  const [publicationOpen, setPublicationOpen] = useState(false);
  const [publicationAcknowledged, setPublicationAcknowledged] = useState(false);
  const [usedBy, setUsedBy] = useState<DocumentationUsage[]>([]);
  const [revisionTab, setRevisionTab] = useState<RevisionTab>("members");
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<DocumentationCollection | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"private" | "public">("private");
  const [collectionLifecycle, setCollectionLifecycle] = useState<DocumentationCollection["lifecycle"]>("active");
  const [memberKind, setMemberKind] = useState<DocumentationCollectionMemberInput["kind"]>("source_publication");
  const [memberID, setMemberID] = useState("");
  const [includeDescendants, setIncludeDescendants] = useState(true);
  const [collectionMembers, setCollectionMembers] = useState<DocumentationCollectionMemberInput[]>([]);
  const [acknowledged, setAcknowledged] = useState(false);

  const load = useCallback(async () => {
    if (!live) return;
    setLoading(true);
    setProblem("");
    try {
      const [values, publicationValues] = await Promise.all([developerAssetsApi.documentationCollections(), developerAssetsApi.documentationPublications()]);
      setCollections(values);
      setPublications([...publicationValues].sort((left, right) => right.revision - left.revision));
      setSelectedID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      setProblem(developerAssetError(error, "Documentation collections could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [live]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  const selected = useMemo(() => collections.find((item) => item.id === selectedID) ?? null, [collections, selectedID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID) {
      queueMicrotask(() => {
        if (!cancelled) { setRevisions([]); setRevisionRecord(null); }
      });
      return () => { cancelled = true; };
    }
    developerAssetsApi.documentationCollectionRevisions(selectedID).then((values) => {
      if (cancelled) return;
      const sorted = [...values].sort((left, right) => right.revision - left.revision);
      setRevisions(sorted);
      setSelectedRevisionID((current) => sorted.some((item) => item.id === current) ? current : sorted[0]?.id ?? "");
    }).catch((error) => { if (!cancelled) onMessage(developerAssetError(error, "Collection revisions could not be loaded.")); });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID || !selectedRevisionID) {
      queueMicrotask(() => { if (!cancelled) setRevisionRecord(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.documentationCollectionRevision(selectedID, selectedRevisionID).then((value) => { if (!cancelled) setRevisionRecord(value); }).catch((error) => { if (!cancelled) { setRevisionRecord(null); onMessage(developerAssetError(error, "The exact collection revision could not be read.")); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedID, selectedRevisionID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID || integrations.length === 0) {
      queueMicrotask(() => { if (!cancelled) setUsedBy([]); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.usage()
      .then((value) => { if (!cancelled) setUsedBy(documentationUsages(value, integrations, selectedID)); })
      .catch(() => { if (!cancelled) setUsedBy([]); });
    return () => { cancelled = true; };
  }, [integrations, live, selectedID]);

  function openEditor(value?: DocumentationCollection, nextLifecycle?: DocumentationCollection["lifecycle"]) {
    const existingMembers = value ? (revisionRecord?.members ?? []).map((member) => ({
      kind: member.member_kind,
      id: String(member.source_publication_id ?? member.documentation_document_id ?? member.documentation_section_id ?? member.id),
      include_descendants: member.include_descendants === true,
      selector: member.selector && typeof member.selector === "object" ? member.selector as Record<string, unknown> : {},
    })) : [];
    setEditing(value ?? null);
    setName(value?.name ?? "");
    setSlug(value?.slug ?? "");
    setDescription(value?.description ?? "");
    setVisibility(value?.visibility ?? "private");
    setCollectionLifecycle(nextLifecycle ?? value?.lifecycle ?? "active");
    setMemberKind("source_publication");
    setMemberID("");
    setIncludeDescendants(true);
    setCollectionMembers(existingMembers);
    setAcknowledged(false);
    setDialogOpen(true);
  }

  async function save() {
    const members = [...collectionMembers, ...(memberID.trim() ? [{ kind: memberKind, id: memberID.trim(), include_descendants: includeDescendants, selector: {} }] : [])];
    if (!acknowledged || !name.trim() || !slug.trim() || members.length === 0) return;
    setBusy(true);
    try {
      const input = {
        name: name.trim(), slug: slug.trim(), description: description.trim(), visibility,
        lifecycle: collectionLifecycle,
        ...(editing ? { revision: editing.revision } : {}),
        members,
        acknowledge_reviewed: true as const,
      };
      const saved = editing
        ? await developerAssetsApi.reviseDocumentationCollection(editing.id, input)
        : await developerAssetsApi.createDocumentationCollection(input);
      setDialogOpen(false);
      await load();
      setSelectedID(saved.id);
      onMessage(editing ? collectionLifecycle === "archived" ? "Documentation collection archived in a new immutable reviewed revision." : "A new immutable documentation revision was created." : "Reviewed documentation collection created.");
    } catch (error) {
      onMessage(developerAssetError(error, "Documentation collection could not be saved."));
    } finally {
      setBusy(false);
    }
  }

  function queueMember() {
    if (!memberID.trim()) return;
    setCollectionMembers((current) => [...current, { kind: memberKind, id: memberID.trim(), include_descendants: includeDescendants, selector: {} }]);
    setMemberID("");
  }

  async function openPublication() {
    setPublicationAcknowledged(false);
    setPublicationVisibility(publications[0]?.visibility ?? "private");
    setPublicationOpen(true);
    try {
      const values = (await Promise.all(collections.filter((collection) => collection.lifecycle === "active").map((collection) => developerAssetsApi.documentationCollectionRevisions(collection.id)))).flat();
      const latestByCollection = new Map<string, DocumentationCollectionRevision>();
      values.forEach((revision) => {
        const current = latestByCollection.get(revision.documentation_collection_id);
        if (!current || current.revision < revision.revision) latestByCollection.set(revision.documentation_collection_id, revision);
      });
      setPublicationOptions(values.sort((left, right) => left.documentation_collection_id.localeCompare(right.documentation_collection_id) || right.revision - left.revision));
      setSelectedPublicationRevisionIDs([...latestByCollection.values()].map((revision) => revision.id));
    } catch (error) {
      onMessage(developerAssetError(error, "Exact collection revisions could not be loaded for publication."));
    }
  }

  async function publishGlobalDocumentation() {
    if (!publicationAcknowledged || selectedPublicationRevisionIDs.length === 0) return;
    setBusy(true);
    try {
      const published = await developerAssetsApi.publishDocumentation(selectedPublicationRevisionIDs, publicationVisibility, publications[0]?.revision ?? 0);
      setPublicationOpen(false);
      await load();
      onMessage(`Global documentation snapshot r${published.revision} published from exact reviewed collection revisions.`);
    } catch (error) {
      onMessage(developerAssetError(error, "Global documentation snapshot could not be published."));
    } finally { setBusy(false); }
  }

  const active: Section = "collections";
  return <>
    <PageHeader eyebrow="Docs" title="Collections" action={<Button onClick={() => openEditor()}><Plus data-slot="icon" />Create collection</Button>} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <section className="panel developer-global-publication"><PanelHeader title="Global documentation publication" description="Query Lab global scope resolves this immutable deployment snapshot." action={<Button onClick={() => void openPublication()}><Radio data-slot="icon" />Publish snapshot</Button>} />{publications[0] ? <div className="developer-global-active"><span><Badge color="green">active snapshot</Badge><strong>Revision {publications[0].revision}</strong><code>{publications[0].id}</code></span><span><small>{publications[0].members.length} exact collection revision{publications[0].members.length === 1 ? "" : "s"}</small><code>{publications[0].snapshot_hash}</code></span></div> : <p className="empty-row">No global documentation snapshot is published. Global Query Lab scope will remain unavailable.</p>}<details className="advanced-details"><summary>Immutable publication history</summary><div className="developer-asset-publication-history">{publications.map((publication) => <div key={publication.id}><span><strong>Revision {publication.revision}</strong><small>{new Date(publication.published_at).toLocaleString()} · {publication.visibility}</small></span><span><code>{publication.snapshot_hash}</code><small>{publication.members.length} members</small></span></div>)}{publications.length === 0 && <small>No publication history.</small>}</div></details></section>
    {loading ? <LoadingPanel label="Loading documentation collections" /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label="Documentation collections" className="developer-asset-directory">
        <DataTableHeader className="developer-collection-columns"><span>Collection</span><span>Revision</span><span>State</span></DataTableHeader>
        {collections.map((collection) => <DataTableRow className={`developer-collection-columns developer-asset-selectable ${collection.id === selectedID ? "selected" : ""}`} key={collection.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => setSelectedID(collection.id)}><span className="resource-icon"><BookOpen /></span><span><strong>{collection.name}</strong><small>{collection.slug}</small></span></button>
          <span><strong className="cell-value">r{collection.revision}</strong><small className="cell-note">root head</small></span>
          <span><ReviewStateBadge state={collection.lifecycle} /></span>
        </DataTableRow>)}
        {collections.length === 0 && <DataTableEmpty columns={3}>No reviewed documentation collection exists yet.</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.name} description={selected.description || "Reusable reviewed documentation."} action={<span className="heading-actions"><Button outline onClick={() => openEditor(selected, "active")}><GitBranch data-slot="icon" />Create revision</Button>{selected.lifecycle !== "archived" && <Button outline onClick={() => openEditor(selected, "archived")}><Archive data-slot="icon" />Archive</Button>}</span>} />
          <div className="developer-asset-revision-layout">
            <aside className="developer-asset-revisions" aria-label={`${selected.name} revisions`}>
              {revisions.map((revision) => <button type="button" className={revision.id === selectedRevisionID ? "active" : ""} key={revision.id} onClick={() => { setSelectedRevisionID(revision.id); setRevisionTab("members"); }}><span><strong>Revision {revision.revision}</strong><small>{new Date(revision.published_at).toLocaleString()}</small></span><ChevronRight /></button>)}
              {revisions.length === 0 && <small>No immutable revisions are available.</small>}
            </aside>
            <div className="developer-asset-revision-detail">
              {revisionRecord ? <>
                <div className="developer-asset-revision-summary"><span><strong>Exact revision r{revisionRecord.revision.revision}</strong><code>{revisionRecord.revision.id}</code></span><Badge color="green"><Check />reviewed</Badge></div>
                <SegmentedControl label="Collection revision detail" value={revisionTab} onChange={setRevisionTab} items={[{ id: "members", label: "Members", count: revisionRecord.members.length }, { id: "map", label: "Documentation Map" }, { id: "manifest", label: "Manifest" }]} />
                {revisionTab === "members" && <div className="developer-asset-member-list">{revisionRecord.members.map((member, index) => <div key={String(member.id ?? index)}><span><strong>{member.member_kind}</strong><small>{String(member.source_publication_id ?? member.documentation_document_id ?? member.documentation_section_id ?? member.id)}</small></span><Badge>{String(member.include_descendants ?? false) === "true" ? "with descendants" : "exact member"}</Badge></div>)}</div>}
                {revisionTab === "map" && <>{revisionRecord.map ? <><MarkdownEvidence label="Documentation Map agent markdown">{revisionRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={revisionRecord.map.map} label="Documentation Map data" /></> : <p className="empty-row">No map artifact is stored for this exact revision.</p>}</>}
                {revisionTab === "manifest" && <><dl className="entity-detail-grid"><div><dt>Content hash</dt><dd><code>{revisionRecord.revision.content_hash}</code></dd></div><div><dt>Reviewed by</dt><dd>{revisionRecord.revision.reviewed_by}</dd></div><div><dt>Reviewed</dt><dd>{new Date(revisionRecord.revision.reviewed_at).toLocaleString()}</dd></div><div><dt>Visibility</dt><dd>{revisionRecord.revision.visibility}</dd></div></dl><PrettyJSON value={revisionRecord.revision.selection_manifest} label="Exact collection selection manifest" /></>}
              </> : <p className="empty-row">Select an immutable revision to inspect its review evidence.</p>}
            </div>
          </div>
          <div className="developer-asset-used-by"><PanelHeader level={3} title="Used by APIs" description="These APIs attach this collection. Creating a revision will not upgrade their exact pins." />{usedBy.map(({ integration, binding }) => <div className="entity-related-row" key={binding.id}><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} · exact revision {binding.pinned_revision_id || "unresolved"}</small></span><Badge color="blue">attached</Badge></div>)}{usedBy.length === 0 && <p className="empty-row">This collection is not attached to an API.</p>}</div>
        </> : <div className="developer-asset-inspector-empty"><BookOpen /><strong>Select a collection</strong><small>Its revision history, members, map, hashes, and reviewer citation will appear here.</small></div>}
      </section>
    </div>}
    <Dialog open={dialogOpen} onClose={setDialogOpen} title={editing ? collectionLifecycle === "archived" ? `Archive ${editing.name}` : `Create revision for ${editing.name}` : "Create documentation collection"} description="Choose exact reviewed evidence members. This action records your review and creates immutable content." actions={<><Button outline onClick={() => setDialogOpen(false)}>Cancel</Button><Button color={collectionLifecycle === "archived" ? "red" : "indigo"} disabled={busy || !acknowledged || !name.trim() || !slug.trim() || (collectionMembers.length === 0 && !memberID.trim())} onClick={() => void save()}>{busy ? "Saving…" : editing ? collectionLifecycle === "archived" ? "Archive reviewed root" : "Create revision" : "Create collection"}</Button></>}>
      <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Name</span><input value={name} onChange={(event) => { setName(event.target.value); if (!editing) setSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>Slug</span><input value={slug} onChange={(event) => setSlug(slugify(event.target.value))} /></label></div><label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Visibility</span><select value={visibility} onChange={(event) => setVisibility(event.target.value as "private" | "public")}><option value="private">Private</option><option value="public">Public</option></select></label><label className="auth-field"><span>Lifecycle</span><select value={collectionLifecycle} onChange={(event) => setCollectionLifecycle(event.target.value as DocumentationCollection["lifecycle"])}><option value="active">Active</option><option value="archived">Archived</option></select></label></div>{collectionMembers.length > 0 && <div className="developer-member-queue"><strong>{collectionMembers.length} exact member{collectionMembers.length === 1 ? "" : "s"}</strong>{collectionMembers.map((member, index) => <div key={`${member.kind}-${member.id}-${index}`}><span><Badge>{member.kind.replaceAll("_", " ")}</Badge><code>{member.id}</code></span><Button outline onClick={() => setCollectionMembers((current) => current.filter((_, itemIndex) => itemIndex !== index))}>Remove</Button></div>)}</div>}<label className="auth-field"><span>Member kind</span><select value={memberKind} onChange={(event) => setMemberKind(event.target.value as DocumentationCollectionMemberInput["kind"])}><option value="source_publication">Source publication</option><option value="document">Document</option><option value="section">Section</option></select></label><label className="auth-field"><span>Exact reviewed evidence ID</span><input value={memberID} onChange={(event) => setMemberID(event.target.value)} placeholder="source-publication-…" /><small>Use an immutable publication, normalized document, or section ID from the explorer.</small></label><label className="compact-check"><input type="checkbox" checked={includeDescendants} onChange={(event) => setIncludeDescendants(event.target.checked)} /><span>Include reviewed descendants selected by this member</span></label><Button type="button" outline disabled={!memberID.trim()} onClick={queueMember}><Plus data-slot="icon" />Queue another member</Button>{editing && <div className="notice"><GitBranch /><span><strong>{usedBy.length} affected API attachment{usedBy.length === 1 ? "" : "s"}.</strong> Exact pins will remain unchanged. Any deliberate change must be made from each API’s Resources tab.</span></div>}<label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I reviewed this exact evidence selection, visibility, lifecycle, and every affected API.</span></label></div>
    </Dialog>
    <Dialog open={publicationOpen} onClose={setPublicationOpen} title="Publish global documentation snapshot" description="Select exact reviewed collection revisions. The new deployment-global snapshot is immutable and never follows later collection changes." actions={<><Button outline onClick={() => setPublicationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !publicationAcknowledged || selectedPublicationRevisionIDs.length === 0} onClick={() => void publishGlobalDocumentation()}>{busy ? "Publishing…" : "Publish immutable snapshot"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Visibility</span><select value={publicationVisibility} onChange={(event) => setPublicationVisibility(event.target.value as "private" | "public")}><option value="private">Private</option><option value="public">Public</option></select></label><div className="auth-field"><span>Exact collection revisions</span><div className="developer-publication-options">{publicationOptions.map((revision) => { const collection = collections.find((item) => item.id === revision.documentation_collection_id); const checked = selectedPublicationRevisionIDs.includes(revision.id); return <label aria-label="Select exact collection revision" key={revision.id}><input type="checkbox" checked={checked} onChange={(event) => setSelectedPublicationRevisionIDs((current) => event.target.checked ? [...current, revision.id] : current.filter((id) => id !== revision.id))} /><span><strong>{collection?.name ?? revision.documentation_collection_id} · r{revision.revision}</strong><small><code>{revision.id}</code> · {revision.content_hash}</small></span></label>; })}{publicationOptions.length === 0 && <p className="empty-row">No reviewed collection revisions are available.</p>}</div></div><label className="compact-check"><input type="checkbox" checked={publicationAcknowledged} onChange={(event) => setPublicationAcknowledged(event.target.checked)} /><span>I reviewed every exact member, hash, visibility, and gap in this global snapshot.</span></label></div></Dialog>
  </>;
}
