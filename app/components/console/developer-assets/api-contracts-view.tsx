"use client";


import { useTranslation } from "react-i18next";
import { Archive, Check, FileCode2, GitBranch, Link2, Pencil, Plus, ShieldCheck } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import type { Section } from "../../../lib/console-routes";
import {
  developerAssetsApi,
  type APIContract,
  type APIContractCandidate,
  type APIContractCandidateRecord,
  type APIContractRevision,
  type APIContractSource,
  type DeveloperAssetRecord,
} from "../../../lib/developer-assets-api";
import { Badge, Button, Dialog } from "../../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import type { Source } from "../shared";
import { DocumentationNavigation } from "./developer-asset-navigation";
import { developerAssetError, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, recordTitle, ReviewStateBadge } from "./developer-asset-ui";
import { contractUsages, type ContractUsage } from "./developer-asset-usage";

type CandidateTab = "summary" | "operations" | "schemas" | "examples" | "map" | "contract" | "diagnostics";

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function candidateValid(candidate: APIContractCandidate) {
  const explicit = candidate.validation_result.valid ?? candidate.validation_result.status;
  return explicit === true || explicit === "valid" || explicit === "pass" || explicit === "passed";
}

export function APIContractsView({ live, integrations, sources, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; sources: Source[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [contracts, setContracts] = useState<APIContract[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [candidates, setCandidates] = useState<APIContractCandidate[]>([]);
  const [revisions, setRevisions] = useState<APIContractRevision[]>([]);
  const [sourceBindings, setSourceBindings] = useState<APIContractSource[]>([]);
  const [selectedCandidateID, setSelectedCandidateID] = useState("");
  const [candidateRecord, setCandidateRecord] = useState<APIContractCandidateRecord | null>(null);
  const [usedBy, setUsedBy] = useState<ContractUsage[]>([]);
  const [tab, setTab] = useState<CandidateTab>("summary");
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const [editingRoot, setEditingRoot] = useState<APIContract | null>(null);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<APIContract["visibility"]>("private");
  const [rootAcknowledged, setRootAcknowledged] = useState(false);
  const [sourceID, setSourceID] = useState("");
  const [sourceRole, setSourceRole] = useState<"primary" | "supplemental">("primary");
  const [acknowledged, setAcknowledged] = useState(false);

  const load = useCallback(async () => {
    if (!live) return;
    setLoading(true);
    setProblem("");
    try {
      const values = await developerAssetsApi.apiContracts();
      setContracts(values);
      setSelectedID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      setProblem(developerAssetError(error, t("apiContracts.apiContractsCouldNotBeLoaded")));
    } finally {
      setLoading(false);
    }
  }, [live, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);
  const selected = useMemo(() => contracts.find((item) => item.id === selectedID) ?? null, [contracts, selectedID]);
  const selectedCandidate = useMemo(() => candidates.find((item) => item.id === selectedCandidateID) ?? null, [candidates, selectedCandidateID]);

  const loadContractDetail = useCallback(async (contractID: string) => {
    if (!live || !contractID) return;
    try {
      const [candidateValues, revisionValues, sourceValues] = await Promise.all([
        developerAssetsApi.apiContractCandidates(contractID),
        developerAssetsApi.apiContractRevisions(contractID),
        developerAssetsApi.apiContractSources(contractID),
      ]);
      setCandidates(candidateValues);
      setRevisions([...revisionValues].sort((left, right) => right.revision - left.revision));
      setSourceBindings(sourceValues.filter((item) => item.lifecycle === "attached"));
      setSelectedCandidateID((current) => candidateValues.some((item) => item.id === current) ? current : candidateValues[0]?.id ?? "");
    } catch (error) {
      onMessage(developerAssetError(error, t("apiContracts.contractCandidatesAndRevisionsCouldNotBeLoaded")));
    }
  }, [live, onMessage, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void loadContractDetail(selectedID); }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadContractDetail, selectedID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID || !selectedCandidateID) {
      queueMicrotask(() => { if (!cancelled) setCandidateRecord(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.apiContractCandidate(selectedID, selectedCandidateID).then((value) => { if (!cancelled) setCandidateRecord(value); }).catch((error) => { if (!cancelled) { setCandidateRecord(null); onMessage(developerAssetError(error, t("apiContracts.theCandidateReviewRecordCouldNotBeLoaded"))); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedCandidateID, selectedID, t]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedID || integrations.length === 0) {
      queueMicrotask(() => { if (!cancelled) setUsedBy([]); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.usage()
      .then((value) => { if (!cancelled) setUsedBy(contractUsages(value, integrations, selectedID)); })
      .catch(() => { if (!cancelled) setUsedBy([]); });
    return () => { cancelled = true; };
  }, [integrations, live, selectedID]);

  function openRootEditor(contract?: APIContract) {
    setEditingRoot(contract ?? null);
    setName(contract?.name ?? "");
    setSlug(contract?.slug ?? "");
    setDescription(contract?.description ?? "");
    setVisibility(contract?.visibility ?? "private");
    setRootAcknowledged(false);
    setCreateOpen(true);
  }

  async function saveContract() {
    if (!name.trim() || !slug.trim()) return;
    setBusy(true);
    try {
      const saved = editingRoot
        ? await developerAssetsApi.updateAPIContract(editingRoot.id, { name: name.trim(), slug: slug.trim(), description: description.trim(), visibility, lifecycle: editingRoot.lifecycle, revision: editingRoot.revision })
        : await developerAssetsApi.createAPIContract({ name: name.trim(), slug: slug.trim(), description: description.trim(), visibility, lifecycle: "active" });
      setCreateOpen(false);
      await load();
      setSelectedID(saved.id);
      onMessage(editingRoot ? t("apiContracts.apiContractRootMetadataUpdatedPublishedRevisionsAndExact") : t("apiContracts.apiContractRootCreatedAttachASourceIngestIt"));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiContracts.apiContractCouldNotBe", { value1: String(editingRoot ? "updated" : "created") })));
    } finally {
      setBusy(false);
    }
  }

  async function archiveContract() {
    if (!selected || !rootAcknowledged) return;
    setBusy(true);
    try {
      await developerAssetsApi.archiveAPIContract(selected.id, selected.revision);
      setArchiveOpen(false);
      await load();
      onMessage(t("apiContracts.apiContractRootArchivedWithoutDeletingCandidatesImmutableRevisions"));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiContracts.apiContractCouldNotBeArchived")));
    } finally { setBusy(false); }
  }

  async function attachSource() {
    if (!selected || !sourceID) return;
    setBusy(true);
    try {
      await developerAssetsApi.attachAPIContractSource(selected.id, sourceID, sourceRole);
      setSourceOpen(false);
      await loadContractDetail(selected.id);
      onMessage(t("apiContracts.fixedDeploymentSourceAttachedThisDoesNotPublishOr"));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiContracts.contractSourceCouldNotBeAttached")));
    } finally {
      setBusy(false);
    }
  }

  async function publishCandidate() {
    if (!selected || !selectedCandidate || !acknowledged || !candidateValid(selectedCandidate)) return;
    setBusy(true);
    try {
      const result = await developerAssetsApi.publishAPIContractCandidate(selected.id, selectedCandidate.id, selected.revision);
      setPublishOpen(false);
      setAcknowledged(false);
      await load();
      await loadContractDetail(selected.id);
      onMessage(t("apiContracts.reviewedImmutableContractRevisionRPublished", { revision: String(result.revision.revision) }));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiContracts.contractCandidateCouldNotBePublished")));
    } finally {
      setBusy(false);
    }
  }

  const renderRecordList = (records: DeveloperAssetRecord[], empty: string) => <div className="developer-asset-record-list">{records.map((record, index) => <article key={String(record.id ?? record.operation_key ?? record.name ?? index)}><strong>{recordTitle(record, `Record ${index + 1}`)}</strong><PrettyJSON value={record} /></article>)}{records.length === 0 && <p className="empty-row">{empty}</p>}</div>;
  const active: Section = "contracts";

  return <>
    <PageHeader eyebrow={t("navigation.docs")} title={t("navigation.apiContracts")} action={<Button onClick={() => openRootEditor()}><Plus data-slot="icon" />{t("apiContracts.createContract")}</Button>} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    {loading ? <LoadingPanel label={t("apiContracts.loadingAPIContracts")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label={t("apiContracts.apiContractCatalog")} className="developer-asset-directory">
        <DataTableHeader className="developer-contract-columns"><span>{t("apiContracts.contract")}</span><span>{t("apiContracts.candidates")}</span><span>{t("apiContracts.published")}</span></DataTableHeader>
        {contracts.map((contract) => <DataTableRow className={`developer-contract-columns developer-asset-selectable ${contract.id === selectedID ? "selected" : ""}`} key={contract.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedID(contract.id); setTab("summary"); }}><span className="resource-icon"><FileCode2 /></span><span><strong>{contract.name}</strong><small>{t("apiContracts.openapi")} {contract.slug}</small></span></button>
          <span><strong className="cell-value">{contract.id === selectedID ? candidates.length : "—"}</strong><small className="cell-note">{t("apiContracts.reviewQueue")}</small></span>
          <span><strong className="cell-value">{contract.id === selectedID ? revisions.length : "—"}</strong><small className="cell-note">{t("apiContracts.immutable")}</small></span>
        </DataTableRow>)}
        {contracts.length === 0 && <DataTableEmpty columns={3}>{t("apiContracts.noDeploymentOwnedAPIContractExistsYet")}</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.name} description={selected.description || t("apiContracts.reusableOpenAPIContractRoot")} action={<span className="heading-actions"><ReviewStateBadge state={selected.lifecycle} /><Button outline onClick={() => openRootEditor(selected)}><Pencil data-slot="icon" />{t("apiContracts.editRoot")}</Button>{selected.lifecycle !== "archived" && <><Button outline onClick={() => { setRootAcknowledged(false); setArchiveOpen(true); }}><Archive data-slot="icon" />{t("apiContracts.archive")}</Button><Button outline onClick={() => { setSourceID(sources[0]?.id ?? ""); setSourceRole("primary"); setSourceOpen(true); }}><Link2 data-slot="icon" />{t("apiContracts.attachSource")}</Button>{selectedCandidate && <Button disabled={!candidateValid(selectedCandidate)} onClick={() => { setAcknowledged(false); setPublishOpen(true); }}><ShieldCheck data-slot="icon" />{t("apiContracts.reviewCandidate")}</Button>}</>}</span>} />
          <div className="developer-contract-evidence"><div><strong>{t("navigation.sources")}</strong><span>{sourceBindings.length || t("apiContracts.none")}</span></div><div><strong>{t("apiContracts.candidates")}</strong><span>{candidates.length}</span></div><div><strong>{t("apiContracts.publishedRevisions")}</strong><span>{revisions.length}</span></div></div>
          <div className="developer-asset-candidate-picker"><label><span>{t("apiContracts.candidate")}</span><select value={selectedCandidateID} onChange={(event) => { setSelectedCandidateID(event.target.value); setTab("summary"); }}>{candidates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.id} · {candidate.openapi_version || t("apiContracts.openapi2")}</option>)}</select></label>{selectedCandidate && <ReviewStateBadge state={candidateValid(selectedCandidate) ? "valid" : "needs_review"} />}</div>
          {candidateRecord ? <>
            <div className="developer-asset-inspector-tabs"><SegmentedControl label={t("apiContracts.contractCandidateReview")} value={tab} onChange={setTab} items={[
              { id: "summary", label: t("common.review") }, { id: "operations", label: t("common.operations"), count: candidateRecord.operations.length }, { id: "schemas", label: t("common.schemas"), count: candidateRecord.schemas.length }, { id: "examples", label: t("common.examples"), count: candidateRecord.examples.length }, { id: "map", label: t("common.map") }, { id: "contract", label: t("common.normalized") }, { id: "diagnostics", label: t("common.diagnostics") },
            ]} /></div>
            <div className="developer-asset-inspector-body">
              {tab === "summary" && <><dl className="entity-detail-grid"><div><dt>{t("apiContracts.candidateID")}</dt><dd><code>{candidateRecord.candidate.id}</code></dd></div><div><dt>{t("apiContracts.contentHash")}</dt><dd><code>{candidateRecord.candidate.content_hash}</code></dd></div><div><dt>{t("apiContracts.ingestionRun")}</dt><dd><code>{candidateRecord.candidate.ingestion_run_id}</code></dd></div><div><dt>{t("apiContracts.format")}</dt><dd>{candidateRecord.candidate.source_format || "—"}</dd></div></dl><PrettyJSON value={candidateRecord.candidate.validation_result} label={t("apiContracts.contractValidationEvidence")} /></>}
              {tab === "operations" && renderRecordList(candidateRecord.operations, t("apiContracts.noOperationsWereNormalized"))}
              {tab === "schemas" && renderRecordList(candidateRecord.schemas, t("apiContracts.noSchemasWereNormalized"))}
              {tab === "examples" && renderRecordList(candidateRecord.examples, t("apiContracts.noExamplesWereNormalized"))}
              {tab === "map" && <>{candidateRecord.map ? <><MarkdownEvidence label={t("apiContracts.contractMapAgentMarkdown")}>{candidateRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={candidateRecord.map.map} label={t("apiContracts.contractMapData")} /></> : <p className="empty-row">{t("apiContracts.noContractMapIsStoredForThisCandidate")}</p>}</>}
              {tab === "contract" && <PrettyJSON value={candidateRecord.candidate.normalized_contract} label={t("apiContracts.normalizedOpenAPIContract")} />}
              {tab === "diagnostics" && <PrettyJSON value={candidateRecord.candidate.diagnostics} label={t("apiContracts.contractCandidateDiagnostics")} />}
            </div>
          </> : <p className="empty-row">{t("apiContracts.noCandidateIsReadyForReviewAttachAFixed")}</p>}
          {revisions.length > 0 && <div className="developer-asset-publication-list"><PanelHeader level={3} title={t("apiContracts.immutableRevisions")} />{revisions.map((revision) => <div key={revision.id}><span><GitBranch /><span><strong>{t("apiContracts.revision")} {revision.revision}</strong><small>{revision.id}</small></span></span><span><Badge color="green"><Check />{t("apiContracts.reviewed")}</Badge><code>{revision.content_hash}</code></span></div>)}</div>}
          <div className="developer-asset-used-by"><PanelHeader level={3} title={t("apiContracts.usedByAPIs")} description={t("apiContracts.theseAPIsAttachThisContractPublishingANewRevision")} />{usedBy.map(({ integration, binding }) => <div className="entity-related-row" key={binding.id}><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} {t("apiContracts.exactRevision")} {binding.pinned_revision_id || t("apiContracts.unresolved")}</small></span><Badge color={binding.primary ? "violet" : "blue"}>{binding.primary ? t("apiContracts.primary2") : t("apiContracts.attached")}</Badge></div>)}{usedBy.length === 0 && <p className="empty-row">{t("apiContracts.thisContractIsNotAttachedToAnAPI")}</p>}</div>
        </> : <div className="developer-asset-inspector-empty"><FileCode2 /><strong>{t("apiContracts.selectAnAPIContract")}</strong><small>{t("apiContracts.sourcesCandidatesDeterministicValidationMapsAndReviewedRevisionsWill")}</small></div>}
      </section>
    </div>}
    <Dialog open={createOpen} onClose={setCreateOpen} title={editingRoot ? t("apiContracts.editAPIContractRoot") : t("apiContracts.createAPIContract")} description={editingRoot ? t("apiContracts.updateRootMetadataOnlyImmutableContractRevisionsAndExact") : t("apiContracts.createAReusableOpenAPIIdentityItIsNotAttachable")} actions={<><Button outline onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !name.trim() || !slug.trim() || Boolean(editingRoot && !rootAcknowledged)} onClick={() => void saveContract()}>{busy ? t("common.saving") : editingRoot ? t("apiContracts.saveRootMetadata") : t("apiContracts.createContract")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("apiContracts.name")}</span><input value={name} onChange={(event) => { setName(event.target.value); if (!editingRoot) setSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>{t("apiContracts.slug")}</span><input value={slug} onChange={(event) => setSlug(slugify(event.target.value))} /></label></div><label className="auth-field"><span>{t("apiContracts.description")}</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><label className="auth-field"><span>{t("apiContracts.visibility")}</span><select value={visibility} onChange={(event) => setVisibility(event.target.value as APIContract["visibility"])}><option value="private">{t("apiContracts.private")}</option><option value="public">{t("apiContracts.public")}</option></select><small>{t("apiContracts.publicVisibilityIsExplicitCandidatePublicationStillRequiresReview")}</small></label>{editingRoot && <><div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("apiContracts.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("apiContracts.s")}.</strong> {t("apiContracts.rootMetadataMayBeVisibleToThoseOperatorsBut")}</span></div><label className="compact-check"><input type="checkbox" checked={rootAcknowledged} onChange={(event) => setRootAcknowledged(event.target.checked)} /><span>{t("apiContracts.iReviewedTheAffectedAPIsAndThisRootMetadata")}</span></label></>}</div></Dialog>
    <Dialog open={archiveOpen} onClose={setArchiveOpen} title={t("apiContracts.archive2", { value1: String(selected?.name ?? "API contract") })} description={t("apiContracts.archiveTheReusableRootWithoutDeletingCandidatesImmutableRevisions")} actions={<><Button outline onClick={() => setArchiveOpen(false)}>{t("common.cancel")}</Button><Button color="red" disabled={busy || !rootAcknowledged} onClick={() => void archiveContract()}>{busy ? t("apiContracts.archiving") : t("apiContracts.archiveRoot")}</Button></>}><div className="auth-form compact-form"><div className="notice"><Archive /><span><strong>{usedBy.length} {t("apiContracts.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("apiContracts.s")}.</strong> {t("apiContracts.existingExactPinsRemainRecordedReviewEachAPIS")}</span></div><label className="compact-check"><input type="checkbox" checked={rootAcknowledged} onChange={(event) => setRootAcknowledged(event.target.checked)} /><span>{t("apiContracts.iReviewedEveryAffectedAPIAndUnderstandThisDoes")}</span></label></div></Dialog>
    <Dialog open={sourceOpen} onClose={setSourceOpen} title={t("apiContracts.attachSourceTo", { value1: String(selected?.name ?? "contract") })} description={t("apiContracts.thisRecordsAFixedAcquisitionSourceItDoesNot")} actions={<><Button outline onClick={() => setSourceOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !sourceID} onClick={() => void attachSource()}>{busy ? t("apiContracts.attaching") : t("apiContracts.attachSource")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("apiContracts.deploymentSource")}</span><select value={sourceID} onChange={(event) => setSourceID(event.target.value)}><option value="">{t("apiContracts.selectASource")}</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name} · {source.kind}</option>)}</select></label><label className="auth-field"><span>{t("apiContracts.role")}</span><select value={sourceRole} onChange={(event) => setSourceRole(event.target.value as "primary" | "supplemental")}><option value="primary">{t("apiContracts.primary")}</option><option value="supplemental">{t("apiContracts.supplemental")}</option></select></label></div></Dialog>
    <Dialog open={publishOpen} onClose={setPublishOpen} title={t("apiContracts.reviewAndPublishContractCandidate")} description={t("apiContracts.confirmTheNormalizedOpenAPIOperationsSchemasExamplesDiagnosticsAnd")} actions={<><Button outline onClick={() => setPublishOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !acknowledged || !selectedCandidate || !candidateValid(selectedCandidate)} onClick={() => void publishCandidate()}>{busy ? t("apiContracts.publishing") : t("apiContracts.publishReviewedRevision")}</Button></>}><div className="developer-asset-review-confirmation"><dl className="entity-detail-grid"><div><dt>{t("apiContracts.candidate")}</dt><dd><code>{selectedCandidate?.id}</code></dd></div><div><dt>{t("apiContracts.contentHash")}</dt><dd><code>{selectedCandidate?.content_hash}</code></dd></div><div><dt>{t("apiContracts.operations")}</dt><dd>{candidateRecord?.operations.length ?? 0}</dd></div><div><dt>{t("apiContracts.schemas")}</dt><dd>{candidateRecord?.schemas.length ?? 0}</dd></div></dl><div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("apiContracts.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("apiContracts.s")}.</strong> {t("apiContracts.theirExactRevisionPinsWillNotChangeAnyUpgrade")}</span></div><label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("apiContracts.iReviewedThisExactValidatedCandidateAndItsCitations")}</span></label></div></Dialog>
  </>;
}
