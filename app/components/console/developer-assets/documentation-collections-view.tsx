"use client";


import { useTranslation } from "react-i18next";
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
import { developerAssetError, enumLabel, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";
import { documentationUsages, type DocumentationUsage } from "./developer-asset-usage";

type RevisionTab = "members" | "map" | "manifest";

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

export function DocumentationCollectionsView({ live, integrations, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
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
      setProblem(developerAssetError(error, t("documentationCollections.documentationCollectionsCouldNotBeLoaded")));
    } finally {
      setLoading(false);
    }
  }, [live, t]);

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
    }).catch((error) => { if (!cancelled) onMessage(developerAssetError(error, t("documentationCollections.collectionRevisionsCouldNotBeLoaded"))); });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedID, t]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID || !selectedRevisionID) {
      queueMicrotask(() => { if (!cancelled) setRevisionRecord(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.documentationCollectionRevision(selectedID, selectedRevisionID).then((value) => { if (!cancelled) setRevisionRecord(value); }).catch((error) => { if (!cancelled) { setRevisionRecord(null); onMessage(developerAssetError(error, t("documentationCollections.theExactCollectionRevisionCouldNotBeRead"))); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedID, selectedRevisionID, t]);

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
      onMessage(editing ? collectionLifecycle === "archived" ? t("documentationCollections.documentationCollectionArchivedInANewImmutableReviewedRevision") : t("documentationCollections.aNewImmutableDocumentationRevisionWasCreated") : t("documentationCollections.reviewedDocumentationCollectionCreated"));
    } catch (error) {
      onMessage(developerAssetError(error, t("documentationCollections.documentationCollectionCouldNotBeSaved")));
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
      onMessage(developerAssetError(error, t("documentationCollections.exactCollectionRevisionsCouldNotBeLoadedForPublication")));
    }
  }

  async function publishGlobalDocumentation() {
    if (!publicationAcknowledged || selectedPublicationRevisionIDs.length === 0) return;
    setBusy(true);
    try {
      const published = await developerAssetsApi.publishDocumentation(selectedPublicationRevisionIDs, publicationVisibility, publications[0]?.revision ?? 0);
      setPublicationOpen(false);
      await load();
      onMessage(t("documentationCollections.globalDocumentationSnapshotRPublishedFromExactReviewedCollection", { revision: String(published.revision) }));
    } catch (error) {
      onMessage(developerAssetError(error, t("documentationCollections.globalDocumentationSnapshotCouldNotBePublished")));
    } finally { setBusy(false); }
  }

  const active: Section = "collections";
  return <>
    <PageHeader eyebrow={t("navigation.docs")} title={t("navigation.collections")} action={<Button onClick={() => openEditor()}><Plus data-slot="icon" />{t("documentationCollections.createCollection")}</Button>} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <section className="panel developer-global-publication"><PanelHeader title={t("documentationCollections.globalDocumentationPublication")} description={t("documentationCollections.queryLabGlobalScopeResolvesThisImmutableDeploymentSnapshot")} action={<Button onClick={() => void openPublication()}><Radio data-slot="icon" />{t("documentationCollections.publishSnapshot")}</Button>} />{publications[0] ? <div className="developer-global-active"><span><Badge color="green">{t("documentationCollections.activeSnapshot")}</Badge><strong>{t("documentationCollections.revision")} {publications[0].revision}</strong><code>{publications[0].id}</code></span><span><small>{publications[0].members.length} {t("documentationCollections.exactCollectionRevision")}{publications[0].members.length === 1 ? "" : t("documentationCollections.s")}</small><code>{publications[0].snapshot_hash}</code></span></div> : <p className="empty-row">{t("documentationCollections.noGlobalDocumentationSnapshotIsPublishedGlobalQueryLab")}</p>}<details className="advanced-details"><summary>{t("documentationCollections.immutablePublicationHistory")}</summary><div className="developer-asset-publication-history">{publications.map((publication) => <div key={publication.id}><span><strong>{t("documentationCollections.revision")} {publication.revision}</strong><small>{t("format.dateTime", { value: new Date(publication.published_at) })} · {publication.visibility}</small></span><span><code>{publication.snapshot_hash}</code><small>{t("documentationCollections.members", { count: publication.members.length })}</small></span></div>)}{publications.length === 0 && <small>{t("documentationCollections.noPublicationHistory")}</small>}</div></details></section>
    {loading ? <LoadingPanel label={t("documentationCollections.loadingDocumentationCollections")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label={t("documentationCollections.documentationCollections")} className="developer-asset-directory">
        <DataTableHeader className="developer-collection-columns"><span>{t("documentationCollections.collection")}</span><span>{t("documentationCollections.revision")}</span><span>{t("documentationCollections.state")}</span></DataTableHeader>
        {collections.map((collection) => <DataTableRow className={`developer-collection-columns developer-asset-selectable ${collection.id === selectedID ? "selected" : ""}`} key={collection.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => setSelectedID(collection.id)}><span className="resource-icon"><BookOpen /></span><span><strong>{collection.name}</strong><small>{collection.slug}</small></span></button>
          <span><strong className="cell-value">r{collection.revision}</strong><small className="cell-note">{t("documentationCollections.rootHead")}</small></span>
          <span><ReviewStateBadge state={collection.lifecycle} /></span>
        </DataTableRow>)}
        {collections.length === 0 && <DataTableEmpty columns={3}>{t("documentationCollections.noReviewedDocumentationCollectionExistsYet")}</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.name} description={selected.description || t("documentationCollections.reusableReviewedDocumentation")} action={<span className="heading-actions"><Button outline onClick={() => openEditor(selected, "active")}><GitBranch data-slot="icon" />{t("documentationCollections.createRevision")}</Button>{selected.lifecycle !== "archived" && <Button outline onClick={() => openEditor(selected, "archived")}><Archive data-slot="icon" />{t("documentationCollections.archive")}</Button>}</span>} />
          <div className="developer-asset-revision-layout">
            <aside className="developer-asset-revisions" aria-label={t("documentationCollections.revisions", { name: String(selected.name) })}>
              {revisions.map((revision) => <button type="button" className={revision.id === selectedRevisionID ? "active" : ""} key={revision.id} onClick={() => { setSelectedRevisionID(revision.id); setRevisionTab("members"); }}><span><strong>{t("documentationCollections.revision")} {revision.revision}</strong><small>{t("format.dateTime", { value: new Date(revision.published_at) })}</small></span><ChevronRight /></button>)}
              {revisions.length === 0 && <small>{t("documentationCollections.noImmutableRevisionsAreAvailable")}</small>}
            </aside>
            <div className="developer-asset-revision-detail">
              {revisionRecord ? <>
                <div className="developer-asset-revision-summary"><span><strong>{t("documentationCollections.exactRevisionR")}{revisionRecord.revision.revision}</strong><code>{revisionRecord.revision.id}</code></span><Badge color="green"><Check />{t("documentationCollections.reviewed")}</Badge></div>
                <SegmentedControl label={t("documentationCollections.collectionRevisionDetail")} value={revisionTab} onChange={setRevisionTab} items={[{ id: "members", label: t("common.members"), count: revisionRecord.members.length }, { id: "map", label: t("common.map") }, { id: "manifest", label: t("common.manifest") }]} />
                {revisionTab === "members" && <div className="developer-asset-member-list">{revisionRecord.members.map((member, index) => <div key={String(member.id ?? index)}><span><strong>{enumLabel(t, String(member.member_kind))}</strong><small>{String(member.source_publication_id ?? member.documentation_document_id ?? member.documentation_section_id ?? member.id)}</small></span><Badge>{String(member.include_descendants ?? false) === "true" ? t("documentationCollections.withDescendants") : t("documentationCollections.exactMember")}</Badge></div>)}</div>}
                {revisionTab === "map" && <>{revisionRecord.map ? <><MarkdownEvidence label={t("documentationCollections.documentationMapAgentMarkdown")}>{revisionRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={revisionRecord.map.map} label={t("documentationCollections.documentationMapData")} /></> : <p className="empty-row">{t("documentationCollections.noMapArtifactIsStoredForThisExactRevision")}</p>}</>}
                {revisionTab === "manifest" && <><dl className="entity-detail-grid"><div><dt>{t("documentationCollections.contentHash")}</dt><dd><code>{revisionRecord.revision.content_hash}</code></dd></div><div><dt>{t("documentationCollections.reviewedBy")}</dt><dd>{revisionRecord.revision.reviewed_by}</dd></div><div><dt>{t("documentationCollections.reviewed")}</dt><dd>{t("format.dateTime", { value: new Date(revisionRecord.revision.reviewed_at) })}</dd></div><div><dt>{t("documentationCollections.visibility")}</dt><dd>{revisionRecord.revision.visibility}</dd></div></dl><PrettyJSON value={revisionRecord.revision.selection_manifest} label={t("documentationCollections.exactCollectionSelectionManifest")} /></>}
              </> : <p className="empty-row">{t("documentationCollections.selectAnImmutableRevisionToInspectItsReviewEvidence")}</p>}
            </div>
          </div>
          <div className="developer-asset-used-by"><PanelHeader level={3} title={t("documentationCollections.usedByAPIs")} description={t("documentationCollections.theseAPIsAttachThisCollectionCreatingARevisionWill")} />{usedBy.map(({ integration, binding }) => <div className="entity-related-row" key={binding.id}><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} {t("documentationCollections.exactRevision")} {binding.pinned_revision_id || t("documentationCollections.unresolved")}</small></span><Badge color="blue">{t("documentationCollections.attached")}</Badge></div>)}{usedBy.length === 0 && <p className="empty-row">{t("documentationCollections.thisCollectionIsNotAttachedToAnAPI")}</p>}</div>
        </> : <div className="developer-asset-inspector-empty"><BookOpen /><strong>{t("documentationCollections.selectACollection")}</strong><small>{t("documentationCollections.itsRevisionHistoryMembersMapHashesAndReviewerCitation")}</small></div>}
      </section>
    </div>}
    <Dialog open={dialogOpen} onClose={setDialogOpen} title={editing ? collectionLifecycle === "archived" ? t("documentationCollections.archive2", { name: String(editing.name) }) : t("documentationCollections.createRevisionFor", { name: String(editing.name) }) : t("documentationCollections.createDocumentationCollection")} description={t("documentationCollections.chooseExactReviewedEvidenceMembersThisActionRecordsYour")} actions={<><Button outline onClick={() => setDialogOpen(false)}>{t("common.cancel")}</Button><Button color={collectionLifecycle === "archived" ? "red" : "indigo"} disabled={busy || !acknowledged || !name.trim() || !slug.trim() || (collectionMembers.length === 0 && !memberID.trim())} onClick={() => void save()}>{busy ? t("common.saving") : editing ? collectionLifecycle === "archived" ? t("documentationCollections.archiveReviewedRoot") : t("documentationCollections.createRevision") : t("documentationCollections.createCollection")}</Button></>}>
      <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("documentationCollections.name")}</span><input value={name} onChange={(event) => { setName(event.target.value); if (!editing) setSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>{t("documentationCollections.slug")}</span><input value={slug} onChange={(event) => setSlug(slugify(event.target.value))} /></label></div><label className="auth-field"><span>{t("documentationCollections.description")}</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>{t("documentationCollections.visibility")}</span><select value={visibility} onChange={(event) => setVisibility(event.target.value as "private" | "public")}><option value="private">{t("documentationCollections.private")}</option><option value="public">{t("documentationCollections.public")}</option></select></label><label className="auth-field"><span>{t("documentationCollections.lifecycle")}</span><select value={collectionLifecycle} onChange={(event) => setCollectionLifecycle(event.target.value as DocumentationCollection["lifecycle"])}><option value="active">{t("documentationCollections.active")}</option><option value="archived">{t("documentationCollections.archived")}</option></select></label></div>{collectionMembers.length > 0 && <div className="developer-member-queue"><strong>{collectionMembers.length} {t("documentationCollections.exactMember")}{collectionMembers.length === 1 ? "" : t("documentationCollections.s")}</strong>{collectionMembers.map((member, index) => <div key={`${member.kind}-${member.id}-${index}`}><span><Badge>{enumLabel(t, member.kind)}</Badge><code>{member.id}</code></span><Button outline onClick={() => setCollectionMembers((current) => current.filter((_, itemIndex) => itemIndex !== index))}>{t("documentationCollections.remove")}</Button></div>)}</div>}<label className="auth-field"><span>{t("documentationCollections.memberKind")}</span><select value={memberKind} onChange={(event) => setMemberKind(event.target.value as DocumentationCollectionMemberInput["kind"])}><option value="source_publication">{t("documentationCollections.sourcePublication")}</option><option value="document">{t("documentationCollections.document")}</option><option value="section">{t("documentationCollections.section")}</option></select></label><label className="auth-field"><span>{t("documentationCollections.exactReviewedEvidenceID")}</span><input value={memberID} onChange={(event) => setMemberID(event.target.value)} placeholder={t("documentationCollections.sourcePublication2")} /><small>{t("documentationCollections.useAnImmutablePublicationNormalizedDocumentOrSectionID")}</small></label><label className="compact-check"><input type="checkbox" checked={includeDescendants} onChange={(event) => setIncludeDescendants(event.target.checked)} /><span>{t("documentationCollections.includeReviewedDescendantsSelectedByThisMember")}</span></label><Button type="button" outline disabled={!memberID.trim()} onClick={queueMember}><Plus data-slot="icon" />{t("documentationCollections.queueAnotherMember")}</Button>{editing && <div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("documentationCollections.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("documentationCollections.s")}.</strong> {t("documentationCollections.exactPinsWillRemainUnchangedAnyDeliberateChangeMust")}</span></div>}<label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("documentationCollections.iReviewedThisExactEvidenceSelectionVisibilityLifecycleAnd")}</span></label></div>
    </Dialog>
    <Dialog open={publicationOpen} onClose={setPublicationOpen} title={t("documentationCollections.publishGlobalDocumentationSnapshot")} description={t("documentationCollections.selectExactReviewedCollectionRevisionsTheNewDeploymentGlobal")} actions={<><Button outline onClick={() => setPublicationOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !publicationAcknowledged || selectedPublicationRevisionIDs.length === 0} onClick={() => void publishGlobalDocumentation()}>{busy ? t("documentationCollections.publishing") : t("documentationCollections.publishImmutableSnapshot")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("documentationCollections.visibility")}</span><select value={publicationVisibility} onChange={(event) => setPublicationVisibility(event.target.value as "private" | "public")}><option value="private">{t("documentationCollections.private")}</option><option value="public">{t("documentationCollections.public")}</option></select></label><div className="auth-field"><span>{t("documentationCollections.exactCollectionRevisions")}</span><div className="developer-publication-options">{publicationOptions.map((revision) => { const collection = collections.find((item) => item.id === revision.documentation_collection_id); const checked = selectedPublicationRevisionIDs.includes(revision.id); return <label aria-label={t("documentationCollections.selectExactCollectionRevision")} key={revision.id}><input type="checkbox" checked={checked} onChange={(event) => setSelectedPublicationRevisionIDs((current) => event.target.checked ? [...current, revision.id] : current.filter((id) => id !== revision.id))} /><span><strong>{collection?.name ?? revision.documentation_collection_id} {t("documentationCollections.r")}{revision.revision}</strong><small><code>{revision.id}</code> · {revision.content_hash}</small></span></label>; })}{publicationOptions.length === 0 && <p className="empty-row">{t("documentationCollections.noReviewedCollectionRevisionsAreAvailable")}</p>}</div></div><label className="compact-check"><input type="checkbox" checked={publicationAcknowledged} onChange={(event) => setPublicationAcknowledged(event.target.checked)} /><span>{t("documentationCollections.iReviewedEveryExactMemberHashVisibilityAndGap")}</span></label></div></Dialog>
  </>;
}
