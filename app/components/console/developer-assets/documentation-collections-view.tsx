"use client";

import { useAssetCreation } from "./use-asset-creation";


import { DocumentationEvidencePicker, type DocumentationEvidenceSelection } from "./documentation-evidence-picker";
import { api } from "../../../lib/api";
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

export function DocumentationCollectionsView({
  live,
  reviewerID = "",
  integrations,
  onMessage,
  onNavigate,
  embedded = false,
  selectedCollectionID,
  startCreate = false,
  initialMembers = [],
  initialMemberLabels = {},
  onCollectionsChange,
  onSelectedCollectionChange,
  onCreateStarted,
}: {
  live: boolean;
  reviewerID?: string;
  integrations: APIIntegration[];
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
  embedded?: boolean;
  selectedCollectionID?: string;
  startCreate?: boolean;
  initialMembers?: DocumentationCollectionMemberInput[];
  initialMemberLabels?: Record<string, string>;
  onCollectionsChange?: (collections: DocumentationCollection[]) => void;
  onSelectedCollectionChange?: (collectionID: string) => void;
  onCreateStarted?: () => void;
}) {
  const { t } = useTranslation();
  const creation = useAssetCreation(reviewerID, "catalog");
  const [collections, setCollections] = useState<DocumentationCollection[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [revisions, setRevisions] = useState<DocumentationCollectionRevision[]>([]);
  const [selectedRevisionID, setSelectedRevisionID] = useState("");
  const [revisionRecord, setRevisionRecord] = useState<DocumentationCollectionRevisionRecord | null>(null);
  const [publications, setPublications] = useState<DeploymentDocumentationPublication[]>([]);
  const [servingPublicationID, setServingPublicationID] = useState("");
  const [activationProblem, setActivationProblem] = useState("");
  const [publicationOptions, setPublicationOptions] = useState<DocumentationCollectionRevision[]>([]);
  const [selectedPublicationRevisionIDs, setSelectedPublicationRevisionIDs] = useState<string[]>([]);
  const [publicationVisibility, setPublicationVisibility] = useState<"private" | "public">("private");
  const [publicationOpen, setPublicationOpen] = useState(false);
  const [publicationAcknowledged, setPublicationAcknowledged] = useState(false);
  const [usedBy, setUsedBy] = useState<DocumentationUsage[]>([]);
  const [revisionTab, setRevisionTab] = useState<RevisionTab>("members");
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [dialogOpen, setDialogOpen] = useState(startCreate);
  const [editing, setEditing] = useState<DocumentationCollection | null>(null);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"private" | "public">("private");
  const [collectionLifecycle, setCollectionLifecycle] = useState<DocumentationCollection["lifecycle"]>("active");
  const [pendingMember, setPendingMember] = useState<DocumentationEvidenceSelection | null>(null);
  const [pickerEpoch, setPickerEpoch] = useState(0);
  const [memberLabels, setMemberLabels] = useState<Record<string, string>>({});
  const [sourceNames, setSourceNames] = useState<Record<string, string>>({});
  const memberID = pendingMember?.member.id ?? "";
  const memberKind = pendingMember?.member.kind ?? "source_publication";
  const [includeDescendants, setIncludeDescendants] = useState(true);
  const [collectionMembers, setCollectionMembers] = useState<DocumentationCollectionMemberInput[]>(startCreate ? initialMembers : []);
  const [acknowledged, setAcknowledged] = useState(false);

  const load = useCallback(async () => {
    if (!live) return;
    setLoading(true);
    setProblem("");
    try {
      const [values, publicationValues, sourceValues] = await Promise.all([developerAssetsApi.documentationCollections(), developerAssetsApi.documentationPublicationState(), api.deployment().then((deployment) => api.sources(deployment.id))]);
      setSourceNames(Object.fromEntries(sourceValues.map((source) => [source.id, source.name])));
      setCollections(values);
      onCollectionsChange?.(values);
      setServingPublicationID(publicationValues.serving_publication_id ?? "");
      setPublications([...publicationValues.items].sort((left, right) => right.revision - left.revision));
      if (!embedded) setSelectedID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      setServingPublicationID("");
      setProblem(developerAssetError(error, t("documentationCollections.documentationCollectionsCouldNotBeLoaded")));
    } finally {
      setLoading(false);
    }
  }, [embedded, live, onCollectionsChange, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  useEffect(() => {
    if (startCreate) onCreateStarted?.();
  }, [onCreateStarted, startCreate]);

  const activeSelectedID = selectedCollectionID ?? selectedID;
  const selected = useMemo(() => collections.find((item) => item.id === activeSelectedID) ?? null, [activeSelectedID, collections]);
  const currentRecord = revisionRecord?.revision.id === selectedRevisionID && revisionRecord.revision.documentation_collection_id === activeSelectedID ? revisionRecord : null;

  function selectCollection(collectionID: string) {
    if (selectedCollectionID === undefined) setSelectedID(collectionID);
    onSelectedCollectionChange?.(collectionID);
  }

  useEffect(() => {
    let cancelled = false;
    if (!live || !activeSelectedID) {
      queueMicrotask(() => {
        if (!cancelled) { setRevisions([]); setRevisionRecord(null); }
      });
      return () => { cancelled = true; };
    }
    developerAssetsApi.documentationCollectionRevisions(activeSelectedID).then((values) => {
      if (cancelled) return;
      const sorted = [...values].sort((left, right) => right.revision - left.revision);
      setRevisions(sorted);
      setSelectedRevisionID((current) => sorted.some((item) => item.id === current) ? current : sorted[0]?.id ?? "");
    }).catch((error) => { if (!cancelled) onMessage(developerAssetError(error, t("documentationCollections.collectionRevisionsCouldNotBeLoaded"))); });
    return () => { cancelled = true; };
  }, [activeSelectedID, live, onMessage, t]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !activeSelectedID || !selectedRevisionID) {
      queueMicrotask(() => { if (!cancelled) setRevisionRecord(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.documentationCollectionRevision(activeSelectedID, selectedRevisionID).then((value) => { if (!cancelled) setRevisionRecord(value); }).catch((error) => { if (!cancelled) { setRevisionRecord(null); onMessage(developerAssetError(error, t("documentationCollections.theExactCollectionRevisionCouldNotBeRead"))); } });
    return () => { cancelled = true; };
  }, [activeSelectedID, live, onMessage, selectedRevisionID, t]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !activeSelectedID || integrations.length === 0) {
      queueMicrotask(() => { if (!cancelled) setUsedBy([]); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.usage()
      .then((value) => { if (!cancelled) setUsedBy(documentationUsages(value, integrations, activeSelectedID)); })
      .catch(() => { if (!cancelled) setUsedBy([]); });
    return () => { cancelled = true; };
  }, [activeSelectedID, integrations, live]);

  function openEditor(value?: DocumentationCollection, nextLifecycle?: DocumentationCollection["lifecycle"]) {
    if (value && !currentRecord) return;
    const existingMembers = value ? (currentRecord?.members ?? []).map((member) => ({
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
    setPendingMember(null);
    setPickerEpoch((value) => value + 1);
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
        : await creation.create("documentation_collection", input, (key) => developerAssetsApi.createDocumentationCollection(input, key));
      if (!editing) creation.complete();
      setDialogOpen(false);
      await load();
      selectCollection(saved.id);
      onMessage(editing ? collectionLifecycle === "archived" ? t("documentationCollections.documentationCollectionArchivedInANewImmutableReviewedRevision") : t("documentationCollections.aNewImmutableDocumentationRevisionWasCreated") : t("documentationCollections.reviewedDocumentationCollectionCreated"));
    } catch (error) {
      onMessage((editing ? developerAssetError : creation.error)(error, t("documentationCollections.documentationCollectionCouldNotBeSaved")));
    } finally {
      setBusy(false);
    }
  }

  function queueMember() {
    if (!memberID.trim()) return;
    if (collectionMembers.some((member) => member.kind === memberKind && member.id === memberID)) { onMessage(t("evidencePicker.duplicate")); return; }
    setCollectionMembers((current) => [...current, { kind: memberKind, id: memberID, include_descendants: includeDescendants, selector: {} }]);
    setPendingMember(null);
    setAcknowledged(false);
    setPickerEpoch((value) => value + 1);
  }

  function memberName(member: DocumentationCollectionMemberInput, index: number) {
    const selectedName = memberLabels[`${member.kind}:${member.id}`] || initialMemberLabels[member.id];
    if (selectedName) return selectedName;
    const prefix = member.kind === "source_publication" ? "source-publication" : member.kind;
    const entries = currentRecord?.map?.map.documents;
    const entry = Array.isArray(entries) ? entries.find((value) => value && typeof value === "object" && value.id === `${prefix}:${member.id}`) : null;
    const title = entry && typeof entry.title === "string" ? entry.title : "";
    if (sourceNames[title]) return sourceNames[title];
    return title || t("evidencePicker.existingEvidence", { number: index + 1 });
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

  async function retryActivation(publicationID: string) {
    setBusy(true);
    setActivationProblem("");
    try {
      await developerAssetsApi.activateDocumentationPublication(publicationID);
      await load();
    } catch (error) {
      setActivationProblem(developerAssetError(error, t("documentationCollections.activationFailed")));
    } finally { setBusy(false); }
  }

  const servingPublication = publications.find((publication) => publication.id === servingPublicationID);
  const awaitingPublication = publications[0]?.id !== servingPublicationID ? publications[0] : null;
  const active: Section = "documents";
  return <div className={embedded ? "documentation-sets-pane" : "documentation-collections-workspace"}>
    {!embedded && <PageHeader eyebrow={t("navigation.docs")} title={t("documentationExplorer.documentationSets")} action={<Button onClick={() => openEditor()}><Plus data-slot="icon" />{t("documentationCollections.createCollection")}</Button>} />}
    {!embedded && <DocumentationNavigation active={active} onNavigate={onNavigate} />}
    <section className="panel developer-global-publication">
      <PanelHeader title={t("documentationCollections.globalDocumentationPublication")} description={t("documentationCollections.queryLabGlobalScopeResolvesThisImmutableDeploymentSnapshot")} action={<span className="heading-actions">{embedded && <Button outline onClick={() => openEditor()}><Plus data-slot="icon" />{t("documentationCollections.createCollection")}</Button>}<Button disabled={loading || !!problem || busy} onClick={() => void openPublication()}><Radio data-slot="icon" />{t("documentationCollections.publishSnapshot")}</Button></span>} />
      {!loading && !problem && <>
        {servingPublication ? <div className="developer-global-active"><span><Badge color="green">{t("documentationCollections.servingAgents")}</Badge><strong>{t("documentationCollections.revision")} {servingPublication.revision}</strong></span><small>{t("documentationCollections.members", { count: servingPublication.members.length })}</small></div> : <p className="empty-row">{t("documentationCollections.noServingPublication")}</p>}
        {awaitingPublication && <div className="developer-global-active"><span><Badge color="amber">{t("documentationCollections.awaitingActivation")}</Badge><strong>{t("documentationCollections.revision")} {awaitingPublication.revision}</strong><small>{t("documentationCollections.activationKeepsServing")}</small></span><Button outline disabled={busy} onClick={() => void retryActivation(awaitingPublication.id)}>{t("documentationCollections.retryActivation")}</Button></div>}
      </>}
      {activationProblem && <p className="form-error" role="alert">{activationProblem}</p>}
      <details className="advanced-details"><summary>{t("documentationCollections.immutablePublicationHistory")}</summary><div className="developer-asset-publication-history">{publications.map((publication) => <div key={publication.id}><span><strong>{t("documentationCollections.revision")} {publication.revision}</strong><small>{t("format.dateTime", { value: new Date(publication.published_at) })} · {publication.visibility}</small></span><span><code>{publication.snapshot_hash}</code><small>{t("documentationCollections.members", { count: publication.members.length })}</small></span></div>)}{publications.length === 0 && <small>{t("documentationCollections.noPublicationHistory")}</small>}</div></details>
    </section>
    {loading ? <LoadingPanel label={t("documentationCollections.loadingDocumentationCollections")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className={embedded ? "documentation-set-detail" : "developer-asset-explorer"}>
      {!embedded && <DataTable label={t("documentationCollections.documentationCollections")} className="developer-asset-directory">
        <DataTableHeader className="developer-collection-columns"><span>{t("documentationCollections.collection")}</span><span>{t("documentationCollections.revision")}</span><span>{t("documentationCollections.state")}</span></DataTableHeader>
        {collections.map((collection) => <DataTableRow className={`developer-collection-columns developer-asset-selectable ${collection.id === activeSelectedID ? "selected" : ""}`} key={collection.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => selectCollection(collection.id)}><span className="resource-icon"><BookOpen /></span><span><strong>{collection.name}</strong><small>{collection.slug}</small></span></button>
          <span><strong className="cell-value">r{collection.revision}</strong><small className="cell-note">{t("documentationCollections.rootHead")}</small></span>
          <span><ReviewStateBadge state={collection.lifecycle} /></span>
        </DataTableRow>)}
        {collections.length === 0 && <DataTableEmpty columns={3}>{t("documentationCollections.noReviewedDocumentationCollectionExistsYet")}</DataTableEmpty>}
      </DataTable>}
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.name} description={selected.description || t("documentationCollections.reusableReviewedDocumentation")} action={<span className="heading-actions"><Button outline disabled={!currentRecord} onClick={() => openEditor(selected, "active")}><GitBranch data-slot="icon" />{t("documentationCollections.createRevision")}</Button>{selected.lifecycle !== "archived" && <Button outline disabled={!currentRecord} onClick={() => openEditor(selected, "archived")}><Archive data-slot="icon" />{t("documentationCollections.archive")}</Button>}</span>} />
          <div className="developer-asset-revision-layout">
            <aside className="developer-asset-revisions" aria-label={t("documentationCollections.revisions", { name: String(selected.name) })}>
              {revisions.map((revision) => <button type="button" className={revision.id === selectedRevisionID ? "active" : ""} key={revision.id} onClick={() => { setSelectedRevisionID(revision.id); setRevisionTab("members"); }}><span><strong>{t("documentationCollections.revision")} {revision.revision}</strong><small>{t("format.dateTime", { value: new Date(revision.published_at) })}</small></span><ChevronRight /></button>)}
              {revisions.length === 0 && <small>{t("documentationCollections.noImmutableRevisionsAreAvailable")}</small>}
            </aside>
            <div className="developer-asset-revision-detail">
              {currentRecord ? <>
                <div className="developer-asset-revision-summary"><span><strong>{t("documentationCollections.exactRevisionR")}{currentRecord.revision.revision}</strong><small>{t("format.dateTime", { value: new Date(currentRecord.revision.published_at) })}</small></span><Badge color="green"><Check />{t("documentationCollections.reviewed")}</Badge></div>
                <SegmentedControl label={t("documentationCollections.collectionRevisionDetail")} value={revisionTab} onChange={setRevisionTab} items={[{ id: "members", label: t("common.members"), count: currentRecord.members.length }, { id: "map", label: t("common.map") }, { id: "manifest", label: t("common.manifest") }]} />
                {revisionTab === "members" && <div className="developer-asset-member-list">{currentRecord.members.map((member, index) => <div key={String(member.id ?? index)}><span><strong>{memberName({ kind: member.member_kind, id: String(member.source_publication_id ?? member.documentation_document_id ?? member.documentation_section_id ?? member.id), include_descendants: member.include_descendants, selector: {} }, index)}</strong><small>{enumLabel(t, String(member.member_kind))}</small></span><Badge>{String(member.include_descendants ?? false) === "true" ? t("documentationCollections.withDescendants") : t("documentationCollections.exactMember")}</Badge></div>)}</div>}
                {revisionTab === "map" && <>{currentRecord.map ? <><MarkdownEvidence label={t("documentationCollections.documentationMapAgentMarkdown")}>{currentRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={currentRecord.map.map} label={t("documentationCollections.documentationMapData")} /></> : <p className="empty-row">{t("documentationCollections.noMapArtifactIsStoredForThisExactRevision")}</p>}</>}
                {revisionTab === "manifest" && <><dl className="entity-detail-grid"><div><dt>{t("documentationCollections.contentHash")}</dt><dd><code>{currentRecord.revision.content_hash}</code></dd></div><div><dt>{t("documentationCollections.reviewedBy")}</dt><dd>{currentRecord.revision.reviewed_by}</dd></div><div><dt>{t("documentationCollections.reviewed")}</dt><dd>{t("format.dateTime", { value: new Date(currentRecord.revision.reviewed_at) })}</dd></div><div><dt>{t("documentationCollections.visibility")}</dt><dd>{currentRecord.revision.visibility}</dd></div></dl><PrettyJSON value={currentRecord.revision.selection_manifest} label={t("documentationCollections.exactCollectionSelectionManifest")} /></>}
              </> : <p className="empty-row">{t("documentationCollections.selectAnImmutableRevisionToInspectItsReviewEvidence")}</p>}
            </div>
          </div>
          <div className="developer-asset-used-by"><PanelHeader level={3} title={t("documentationCollections.usedByAPIs")} description={t("documentationCollections.theseAPIsAttachThisCollectionCreatingARevisionWill")} />{usedBy.map(({ integration, binding }) => <div className="entity-related-row" key={binding.id}><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} {t("documentationCollections.exactRevision")} {binding.pinned_revision_id || t("documentationCollections.unresolved")}</small></span><Badge color="blue">{t("documentationCollections.attached")}</Badge></div>)}{usedBy.length === 0 && <p className="empty-row">{t("documentationCollections.thisCollectionIsNotAttachedToAnAPI")}</p>}</div>
        </> : <div className="developer-asset-inspector-empty"><BookOpen /><strong>{t("documentationCollections.selectACollection")}</strong><small>{t("documentationCollections.itsRevisionHistoryMembersMapHashesAndReviewerCitation")}</small></div>}
      </section>
    </div>}
    <Dialog open={dialogOpen} onClose={setDialogOpen} title={editing ? collectionLifecycle === "archived" ? t("documentationCollections.archive2", { name: String(editing.name) }) : t("documentationCollections.createRevisionFor", { name: String(editing.name) }) : t("documentationCollections.createDocumentationCollection")} description={t("documentationCollections.chooseExactReviewedEvidenceMembersThisActionRecordsYour")} actions={<><Button outline onClick={() => setDialogOpen(false)}>{t("common.cancel")}</Button><Button color={collectionLifecycle === "archived" ? "red" : "indigo"} disabled={busy || !acknowledged || !name.trim() || !slug.trim() || (collectionMembers.length === 0 && !memberID.trim())} onClick={() => void save()}>{busy ? t("common.saving") : editing ? collectionLifecycle === "archived" ? t("documentationCollections.archiveReviewedRoot") : t("documentationCollections.createRevision") : t("documentationCollections.createCollection")}</Button></>}>
      <div className="auth-form compact-form">{creation.recoveryUnavailable && <p className="auth-problem">{t("assetCreation.recoveryUnavailable")}</p>}<div className="two-fields"><label className="auth-field"><span>{t("documentationCollections.name")}</span><input value={name} onChange={(event) => { setAcknowledged(false); setName(event.target.value); if (!editing) setSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>{t("documentationCollections.slug")}</span><input value={slug} onChange={(event) => { setAcknowledged(false); setSlug(slugify(event.target.value)); }} /></label></div><label className="auth-field"><span>{t("documentationCollections.description")}</span><textarea value={description} onChange={(event) => { setAcknowledged(false); setDescription(event.target.value); }} /></label><div className="two-fields"><label className="auth-field"><span>{t("documentationCollections.visibility")}</span><select value={visibility} onChange={(event) => { setVisibility(event.target.value as "private" | "public"); setPendingMember(null); setAcknowledged(false); setPickerEpoch((value) => value + 1); }}><option value="private">{t("documentationCollections.private")}</option><option value="public">{t("documentationCollections.public")}</option></select></label><label className="auth-field"><span>{t("documentationCollections.lifecycle")}</span><select value={collectionLifecycle} onChange={(event) => { setAcknowledged(false); setCollectionLifecycle(event.target.value as DocumentationCollection["lifecycle"]); }}><option value="active">{t("documentationCollections.active")}</option><option value="archived">{t("documentationCollections.archived")}</option></select></label></div>{collectionMembers.length > 0 && <div className="developer-member-queue"><strong>{collectionMembers.length} {t("documentationCollections.exactMember")}{collectionMembers.length === 1 ? "" : t("documentationCollections.s")}</strong>{collectionMembers.map((member, index) => <div key={`${member.kind}-${member.id}-${index}`}><span><Badge>{enumLabel(t, member.kind)}</Badge><strong>{memberName(member, index)}</strong></span><Button outline onClick={() => { setAcknowledged(false); setCollectionMembers((current) => current.filter((_, itemIndex) => itemIndex !== index)); }}>{t("documentationCollections.remove")}</Button></div>)}</div>}{dialogOpen && <DocumentationEvidencePicker key={pickerEpoch} publicOnly={visibility === "public"} onChange={(selection) => { setPendingMember(selection); setAcknowledged(false); if (selection) setMemberLabels((values) => ({ ...values, [`${selection.member.kind}:${selection.member.id}`]: selection.label })); }} />}<label className="compact-check"><input type="checkbox" checked={includeDescendants} onChange={(event) => { setAcknowledged(false); setIncludeDescendants(event.target.checked); }} /><span>{t("documentationCollections.includeReviewedDescendantsSelectedByThisMember")}</span></label><Button type="button" outline disabled={!memberID.trim()} onClick={queueMember}><Plus data-slot="icon" />{t("documentationCollections.queueAnotherMember")}</Button>{editing && <div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("documentationCollections.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("documentationCollections.s")}.</strong> {t("documentationCollections.exactPinsWillRemainUnchangedAnyDeliberateChangeMust")}</span></div>}<label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("documentationCollections.iReviewedThisExactEvidenceSelectionVisibilityLifecycleAnd")}</span></label></div>
    </Dialog>
    <Dialog open={publicationOpen} onClose={setPublicationOpen} title={t("documentationCollections.publishGlobalDocumentationSnapshot")} description={t("documentationCollections.selectExactReviewedCollectionRevisionsTheNewDeploymentGlobal")} actions={<><Button outline onClick={() => setPublicationOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !publicationAcknowledged || selectedPublicationRevisionIDs.length === 0} onClick={() => void publishGlobalDocumentation()}>{busy ? t("documentationCollections.publishing") : t("documentationCollections.publishImmutableSnapshot")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("documentationCollections.visibility")}</span><select value={publicationVisibility} onChange={(event) => setPublicationVisibility(event.target.value as "private" | "public")}><option value="private">{t("documentationCollections.private")}</option><option value="public">{t("documentationCollections.public")}</option></select></label><div className="auth-field"><span>{t("documentationCollections.exactCollectionRevisions")}</span><div className="developer-publication-options">{publicationOptions.map((revision) => { const collection = collections.find((item) => item.id === revision.documentation_collection_id); const checked = selectedPublicationRevisionIDs.includes(revision.id); return <label aria-label={t("documentationCollections.selectExactCollectionRevision")} key={revision.id}><input type="checkbox" checked={checked} onChange={(event) => setSelectedPublicationRevisionIDs((current) => event.target.checked ? [...current, revision.id] : current.filter((id) => id !== revision.id))} /><span><strong>{collection?.name ?? revision.documentation_collection_id} {t("documentationCollections.r")}{revision.revision}</strong><small><code>{revision.id}</code> · {revision.content_hash}</small></span></label>; })}{publicationOptions.length === 0 && <p className="empty-row">{t("documentationCollections.noReviewedCollectionRevisionsAreAvailable")}</p>}</div></div><label className="compact-check"><input type="checkbox" checked={publicationAcknowledged} onChange={(event) => setPublicationAcknowledged(event.target.checked)} /><span>{t("documentationCollections.iReviewedEveryExactMemberHashVisibilityAndGap")}</span></label></div></Dialog>
  </div>;
}
