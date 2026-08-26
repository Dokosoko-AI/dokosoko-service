"use client";

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
import { CatalogNavigation, DocumentationNavigation } from "./developer-asset-navigation";
import { developerAssetError, ExactVersionNotice, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, recordTitle, ReviewStateBadge } from "./developer-asset-ui";
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
      setProblem(developerAssetError(error, "API contracts could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [live]);

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
      onMessage(developerAssetError(error, "Contract candidates and revisions could not be loaded."));
    }
  }, [live, onMessage]);

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
    developerAssetsApi.apiContractCandidate(selectedID, selectedCandidateID).then((value) => { if (!cancelled) setCandidateRecord(value); }).catch((error) => { if (!cancelled) { setCandidateRecord(null); onMessage(developerAssetError(error, "The candidate review record could not be loaded.")); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedCandidateID, selectedID]);

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
      onMessage(editingRoot ? "API contract root metadata updated. Published revisions and exact API pins remain unchanged." : "API contract root created. Attach a source, ingest it, and review a candidate before attaching it to an API.");
    } catch (error) {
      onMessage(developerAssetError(error, `API contract could not be ${editingRoot ? "updated" : "created"}.`));
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
      onMessage("API contract root archived without deleting candidates, immutable revisions, attachments, maps, or audit history.");
    } catch (error) {
      onMessage(developerAssetError(error, "API contract could not be archived."));
    } finally { setBusy(false); }
  }

  async function attachSource() {
    if (!selected || !sourceID) return;
    setBusy(true);
    try {
      await developerAssetsApi.attachAPIContractSource(selected.id, sourceID, sourceRole);
      setSourceOpen(false);
      await loadContractDetail(selected.id);
      onMessage("Fixed deployment source attached. This does not publish or approve a contract candidate.");
    } catch (error) {
      onMessage(developerAssetError(error, "Contract source could not be attached."));
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
      onMessage(`Reviewed immutable contract revision r${result.revision.revision} published.`);
    } catch (error) {
      onMessage(developerAssetError(error, "Contract candidate could not be published."));
    } finally {
      setBusy(false);
    }
  }

  const renderRecordList = (records: DeveloperAssetRecord[], empty: string) => <div className="developer-asset-record-list">{records.map((record, index) => <article key={String(record.id ?? record.operation_key ?? record.name ?? index)}><strong>{recordTitle(record, `Record ${index + 1}`)}</strong><PrettyJSON value={record} /></article>)}{records.length === 0 && <p className="empty-row">{empty}</p>}</div>;
  const active: Section = "contracts";

  return <>
    <PageHeader eyebrow="Catalog · Documentation" title="API contracts" description="Review normalized OpenAPI candidates and publish immutable revisions that any API can attach." action={<Button onClick={() => openRootEditor()}><Plus data-slot="icon" />Create contract</Button>} />
    <CatalogNavigation active={active} onNavigate={onNavigate} />
    <DocumentationNavigation active={active} onNavigate={onNavigate} />
    <ExactVersionNotice>Validation is deterministic, but publication still requires human review. APIs attach reviewed contract revisions; roots and candidates are never copied into an API.</ExactVersionNotice>
    {loading ? <LoadingPanel label="Loading API contracts" /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label="API contract catalog" className="developer-asset-directory">
        <DataTableHeader className="developer-contract-columns"><span>Contract</span><span>Candidates</span><span>Published</span></DataTableHeader>
        {contracts.map((contract) => <DataTableRow className={`developer-contract-columns developer-asset-selectable ${contract.id === selectedID ? "selected" : ""}`} key={contract.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedID(contract.id); setTab("summary"); }}><span className="resource-icon"><FileCode2 /></span><span><strong>{contract.name}</strong><small>OpenAPI · {contract.slug}</small></span></button>
          <span><strong className="cell-value">{contract.id === selectedID ? candidates.length : "—"}</strong><small className="cell-note">review queue</small></span>
          <span><strong className="cell-value">{contract.id === selectedID ? revisions.length : "—"}</strong><small className="cell-note">immutable</small></span>
        </DataTableRow>)}
        {contracts.length === 0 && <DataTableEmpty columns={3}>No deployment-owned API contract exists yet.</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selected ? <>
          <PanelHeader title={selected.name} description={selected.description || "Reusable OpenAPI contract root."} action={<span className="heading-actions"><ReviewStateBadge state={selected.lifecycle} /><Button outline onClick={() => openRootEditor(selected)}><Pencil data-slot="icon" />Edit root</Button>{selected.lifecycle !== "archived" && <><Button outline onClick={() => { setRootAcknowledged(false); setArchiveOpen(true); }}><Archive data-slot="icon" />Archive</Button><Button outline onClick={() => { setSourceID(sources[0]?.id ?? ""); setSourceRole("primary"); setSourceOpen(true); }}><Link2 data-slot="icon" />Attach source</Button>{selectedCandidate && <Button disabled={!candidateValid(selectedCandidate)} onClick={() => { setAcknowledged(false); setPublishOpen(true); }}><ShieldCheck data-slot="icon" />Review candidate</Button>}</>}</span>} />
          <div className="developer-contract-evidence"><div><strong>Sources</strong><span>{sourceBindings.length || "None"}</span></div><div><strong>Candidates</strong><span>{candidates.length}</span></div><div><strong>Published revisions</strong><span>{revisions.length}</span></div></div>
          <div className="developer-asset-candidate-picker"><label><span>Candidate</span><select value={selectedCandidateID} onChange={(event) => { setSelectedCandidateID(event.target.value); setTab("summary"); }}>{candidates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.id} · {candidate.openapi_version || "OpenAPI"}</option>)}</select></label>{selectedCandidate && <ReviewStateBadge state={candidateValid(selectedCandidate) ? "valid" : "needs_review"} />}</div>
          {candidateRecord ? <>
            <div className="developer-asset-inspector-tabs"><SegmentedControl label="Contract candidate review" value={tab} onChange={setTab} items={[
              { id: "summary", label: "Review" }, { id: "operations", label: "Operations", count: candidateRecord.operations.length }, { id: "schemas", label: "Schemas", count: candidateRecord.schemas.length }, { id: "examples", label: "Examples", count: candidateRecord.examples.length }, { id: "map", label: "Contract Map" }, { id: "contract", label: "Normalized" }, { id: "diagnostics", label: "Diagnostics" },
            ]} /></div>
            <div className="developer-asset-inspector-body">
              {tab === "summary" && <><dl className="entity-detail-grid"><div><dt>Candidate ID</dt><dd><code>{candidateRecord.candidate.id}</code></dd></div><div><dt>Content hash</dt><dd><code>{candidateRecord.candidate.content_hash}</code></dd></div><div><dt>Ingestion run</dt><dd><code>{candidateRecord.candidate.ingestion_run_id}</code></dd></div><div><dt>Format</dt><dd>{candidateRecord.candidate.source_format || "—"}</dd></div></dl><PrettyJSON value={candidateRecord.candidate.validation_result} label="Contract validation evidence" /></>}
              {tab === "operations" && renderRecordList(candidateRecord.operations, "No operations were normalized.")}
              {tab === "schemas" && renderRecordList(candidateRecord.schemas, "No schemas were normalized.")}
              {tab === "examples" && renderRecordList(candidateRecord.examples, "No examples were normalized.")}
              {tab === "map" && <>{candidateRecord.map ? <><MarkdownEvidence label="Contract Map agent markdown">{candidateRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={candidateRecord.map.map} label="Contract Map data" /></> : <p className="empty-row">No Contract Map is stored for this candidate.</p>}</>}
              {tab === "contract" && <PrettyJSON value={candidateRecord.candidate.normalized_contract} label="Normalized OpenAPI contract" />}
              {tab === "diagnostics" && <PrettyJSON value={candidateRecord.candidate.diagnostics} label="Contract candidate diagnostics" />}
            </div>
          </> : <p className="empty-row">No candidate is ready for review. Attach a fixed source and run its acquisition workflow.</p>}
          {revisions.length > 0 && <div className="developer-asset-publication-list"><PanelHeader level={3} title="Immutable revisions" />{revisions.map((revision) => <div key={revision.id}><span><GitBranch /><span><strong>Revision {revision.revision}</strong><small>{revision.id}</small></span></span><span><Badge color="green"><Check />reviewed</Badge><code>{revision.content_hash}</code></span></div>)}</div>}
          <div className="developer-asset-used-by"><PanelHeader level={3} title="Used by APIs" description="These APIs attach this contract. Publishing a new revision will not move their exact pins." />{usedBy.map(({ integration, binding }) => <div className="entity-related-row" key={binding.id}><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} · exact revision {binding.pinned_revision_id || "unresolved"}</small></span><Badge color={binding.primary ? "violet" : "blue"}>{binding.primary ? "primary" : "attached"}</Badge></div>)}{usedBy.length === 0 && <p className="empty-row">This contract is not attached to an API.</p>}</div>
        </> : <div className="developer-asset-inspector-empty"><FileCode2 /><strong>Select an API contract</strong><small>Sources, candidates, deterministic validation, maps, and reviewed revisions will appear here.</small></div>}
      </section>
    </div>}
    <Dialog open={createOpen} onClose={setCreateOpen} title={editingRoot ? "Edit API contract root" : "Create API contract"} description={editingRoot ? "Update root metadata only. Immutable contract revisions and exact API pins are not rewritten." : "Create a reusable OpenAPI identity. It is not attachable until a candidate is validated and explicitly reviewed."} actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !name.trim() || !slug.trim() || Boolean(editingRoot && !rootAcknowledged)} onClick={() => void saveContract()}>{busy ? "Saving…" : editingRoot ? "Save root metadata" : "Create contract"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Name</span><input value={name} onChange={(event) => { setName(event.target.value); if (!editingRoot) setSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>Slug</span><input value={slug} onChange={(event) => setSlug(slugify(event.target.value))} /></label></div><label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><label className="auth-field"><span>Visibility</span><select value={visibility} onChange={(event) => setVisibility(event.target.value as APIContract["visibility"])}><option value="private">Private</option><option value="public">Public</option></select><small>Public visibility is explicit; candidate publication still requires review.</small></label>{editingRoot && <><div className="notice"><GitBranch /><span><strong>{usedBy.length} affected API attachment{usedBy.length === 1 ? "" : "s"}.</strong> Root metadata may be visible to those operators, but their exact contract revision pins will not change.</span></div><label className="compact-check"><input type="checkbox" checked={rootAcknowledged} onChange={(event) => setRootAcknowledged(event.target.checked)} /><span>I reviewed the affected APIs and this root metadata change.</span></label></>}</div></Dialog>
    <Dialog open={archiveOpen} onClose={setArchiveOpen} title={`Archive ${selected?.name ?? "API contract"}`} description="Archive the reusable root without deleting candidates, immutable revisions, maps, citations, bindings, or audit history." actions={<><Button outline onClick={() => setArchiveOpen(false)}>Cancel</Button><Button color="red" disabled={busy || !rootAcknowledged} onClick={() => void archiveContract()}>{busy ? "Archiving…" : "Archive root"}</Button></>}><div className="auth-form compact-form"><div className="notice"><Archive /><span><strong>{usedBy.length} affected API attachment{usedBy.length === 1 ? "" : "s"}.</strong> Existing exact pins remain recorded; review each API’s Resources tab before archiving.</span></div><label className="compact-check"><input type="checkbox" checked={rootAcknowledged} onChange={(event) => setRootAcknowledged(event.target.checked)} /><span>I reviewed every affected API and understand this does not detach or delete history.</span></label></div></Dialog>
    <Dialog open={sourceOpen} onClose={setSourceOpen} title={`Attach source to ${selected?.name ?? "contract"}`} description="This records a fixed acquisition source. It does not approve or publish any candidate." actions={<><Button outline onClick={() => setSourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !sourceID} onClick={() => void attachSource()}>{busy ? "Attaching…" : "Attach source"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Deployment source</span><select value={sourceID} onChange={(event) => setSourceID(event.target.value)}><option value="">Select a source</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name} · {source.kind}</option>)}</select></label><label className="auth-field"><span>Role</span><select value={sourceRole} onChange={(event) => setSourceRole(event.target.value as "primary" | "supplemental")}><option value="primary">Primary</option><option value="supplemental">Supplemental</option></select></label></div></Dialog>
    <Dialog open={publishOpen} onClose={setPublishOpen} title="Review and publish contract candidate" description="Confirm the normalized OpenAPI, operations, schemas, examples, diagnostics, and Contract Map before creating an immutable revision." actions={<><Button outline onClick={() => setPublishOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !acknowledged || !selectedCandidate || !candidateValid(selectedCandidate)} onClick={() => void publishCandidate()}>{busy ? "Publishing…" : "Publish reviewed revision"}</Button></>}><div className="developer-asset-review-confirmation"><dl className="entity-detail-grid"><div><dt>Candidate</dt><dd><code>{selectedCandidate?.id}</code></dd></div><div><dt>Content hash</dt><dd><code>{selectedCandidate?.content_hash}</code></dd></div><div><dt>Operations</dt><dd>{candidateRecord?.operations.length ?? 0}</dd></div><div><dt>Schemas</dt><dd>{candidateRecord?.schemas.length ?? 0}</dd></div></dl><div className="notice"><GitBranch /><span><strong>{usedBy.length} affected API attachment{usedBy.length === 1 ? "" : "s"}.</strong> Their exact revision pins will not change. Any upgrade must be explicit in each API’s Resources tab.</span></div><label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I reviewed this exact validated candidate and its citations. I understand publication is immutable.</span></label></div></Dialog>
  </>;
}
