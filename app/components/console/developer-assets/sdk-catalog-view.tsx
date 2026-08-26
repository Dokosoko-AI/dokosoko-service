"use client";

import { Box, Code2, FileCode2, GitBranch, History, PackagePlus, Pencil, Plus, Search, ShieldCheck, TriangleAlert, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import type { Section } from "../../../lib/console-routes";
import {
  developerAssetsApi,
  type DeveloperAssetRecord,
  type SDKContentCandidate,
  type SDKContentCandidateRecord,
  type SDKContentPublication,
  type SDKIngestionFile,
  type SDKPackage,
  type SDKRelease,
  type SDKReleaseLifecycleState,
} from "../../../lib/developer-assets-api";
import { Badge, Button, Dialog } from "../../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader, PanelHeader, SegmentedControl } from "../../core/layout";
import { ConsoleLink } from "../console-link";
import { DeveloperAssetAIAdvisoryButton } from "./developer-asset-ai-advisory";
import { CatalogNavigation } from "./developer-asset-navigation";
import { developerAssetError, ExactVersionNotice, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, recordID, recordString, recordTitle, ReviewStateBadge } from "./developer-asset-ui";
import { sdkUsages, type SDKUsage } from "./developer-asset-usage";
import { decisionPayload, decisionsComplete, maxSDKIngestionFileBytes, maxSDKIngestionFiles, maxSDKIngestionTotalBytes, sampleValidated, sdkBufferLooksText, sdkExplorerRecordMatches, sdkLanguageForPath, sdkNormalizedLocalPath, sdkTextBytes, type SDKDecisionState } from "./sdk-catalog-helpers";

type SDKTab = "files" | "symbols" | "samples" | "map" | "diagnostics" | "used-by";
type SDKRejectedFile = { path: string; reason: string };

export function SDKCatalogView({ live, integrations, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [packages, setPackages] = useState<SDKPackage[]>([]);
  const [selectedPackageID, setSelectedPackageID] = useState("");
  const [releases, setReleases] = useState<SDKRelease[]>([]);
  const [selectedReleaseID, setSelectedReleaseID] = useState("");
  const [releaseLifecycle, setReleaseLifecycle] = useState<SDKReleaseLifecycleState | null>(null);
  const [candidates, setCandidates] = useState<SDKContentCandidate[]>([]);
  const [publications, setPublications] = useState<SDKContentPublication[]>([]);
  const [selectedCandidateID, setSelectedCandidateID] = useState("");
  const [assetQuery, setAssetQuery] = useState("");
  const [candidateRecord, setCandidateRecord] = useState<SDKContentCandidateRecord | null>(null);
  const [usedBy, setUsedBy] = useState<SDKUsage[]>([]);
  const [tab, setTab] = useState<SDKTab>("files");
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [packageOpen, setPackageOpen] = useState(false);
  const [editingPackage, setEditingPackage] = useState<SDKPackage | null>(null);
  const [releaseOpen, setReleaseOpen] = useState(false);
  const [lifecycleOpen, setLifecycleOpen] = useState(false);
  const [ingestOpen, setIngestOpen] = useState(false);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [ecosystem, setEcosystem] = useState("npm");
  const [coordinate, setCoordinate] = useState("");
  const [packageName, setPackageName] = useState("");
  const [packageDescription, setPackageDescription] = useState("");
  const [language, setLanguage] = useState("TypeScript");
  const [packageVisibility, setPackageVisibility] = useState<SDKPackage["visibility"]>("private");
  const [packageLifecycle, setPackageLifecycle] = useState<SDKPackage["lifecycle"]>("draft");
  const [packageAcknowledged, setPackageAcknowledged] = useState(false);
  const [exactVersion, setExactVersion] = useState("");
  const [installCommand, setInstallCommand] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [sourceRevision, setSourceRevision] = useState("");
  const [lifecycleValue, setLifecycleValue] = useState<SDKRelease["lifecycle"]>("active");
  const [lifecycleReason, setLifecycleReason] = useState("");
  const [lifecycleSourceURI, setLifecycleSourceURI] = useState("");
  const [lifecycleObservedAt, setLifecycleObservedAt] = useState("");
  const [sourcePath, setSourcePath] = useState("README.md");
  const [fileContent, setFileContent] = useState("");
  const [fileLanguage, setFileLanguage] = useState("markdown");
  const [queuedFiles, setQueuedFiles] = useState<SDKIngestionFile[]>([]);
  const [rejectedFiles, setRejectedFiles] = useState<SDKRejectedFile[]>([]);
  const [filePickerBusy, setFilePickerBusy] = useState(false);
  const [fileDecisions, setFileDecisions] = useState<SDKDecisionState>({});
  const [sampleDecisions, setSampleDecisions] = useState<SDKDecisionState>({});
  const [acknowledged, setAcknowledged] = useState(false);

  const load = useCallback(async () => {
    if (!live) return;
    setLoading(true);
    setProblem("");
    try {
      const values = await developerAssetsApi.sdkPackages();
      setPackages(values);
      setSelectedPackageID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      setProblem(developerAssetError(error, "SDK packages could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [live]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);
  const selectedPackage = useMemo(() => packages.find((item) => item.id === selectedPackageID) ?? null, [packages, selectedPackageID]);
  const selectedRelease = useMemo(() => releases.find((item) => item.id === selectedReleaseID) ?? null, [releases, selectedReleaseID]);
  const selectedReleaseLifecycle = releaseLifecycle?.sdk_release_id === selectedReleaseID ? releaseLifecycle : null;
  const selectedContentPublication = useMemo(() => publications.find((publication) => publication.sdk_content_candidate_id === selectedCandidateID) ?? null, [publications, selectedCandidateID]);
  const advisoryUsages = useMemo(() => selectedContentPublication ? usedBy.filter(({ binding, publication }) => binding.sdk_content_publication_id === selectedContentPublication.id && Boolean(publication)) : [], [selectedContentPublication, usedBy]);
  const normalizedAssetQuery = assetQuery.trim().toLowerCase();
  const filteredFiles = useMemo(() => candidateRecord?.files.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, `File ${index + 1}`))) ?? [], [candidateRecord, normalizedAssetQuery]);
  const filteredSymbols = useMemo(() => candidateRecord?.symbols.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, `Symbol ${index + 1}`))) ?? [], [candidateRecord, normalizedAssetQuery]);
  const filteredSamples = useMemo(() => candidateRecord?.samples.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, `Sample ${index + 1}`))) ?? [], [candidateRecord, normalizedAssetQuery]);
  const filteredAssetCount = tab === "files" ? filteredFiles.length : tab === "symbols" ? filteredSymbols.length : filteredSamples.length;
  const totalAssetCount = tab === "files" ? candidateRecord?.files.length ?? 0 : tab === "symbols" ? candidateRecord?.symbols.length ?? 0 : candidateRecord?.samples.length ?? 0;

  const loadReleases = useCallback(async (packageID: string) => {
    if (!live || !packageID) return;
    try {
      const values = await developerAssetsApi.sdkReleases(packageID);
      setReleases(values);
      setSelectedReleaseID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      onMessage(developerAssetError(error, "Exact SDK releases could not be loaded."));
    }
  }, [live, onMessage]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void loadReleases(selectedPackageID); }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadReleases, selectedPackageID]);

  const loadReleaseContent = useCallback(async (releaseID: string) => {
    if (!live || !releaseID) { setCandidates([]); setPublications([]); return; }
    try {
      const [candidateValues, publicationValues] = await Promise.all([developerAssetsApi.sdkContentCandidates(releaseID), developerAssetsApi.sdkContentPublications(releaseID)]);
      setCandidates(candidateValues);
      setPublications([...publicationValues].sort((left, right) => right.revision - left.revision));
      setSelectedCandidateID((current) => candidateValues.some((item) => item.id === current) ? current : candidateValues[0]?.id ?? "");
    } catch (error) {
      onMessage(developerAssetError(error, "SDK content candidates could not be loaded."));
    }
  }, [live, onMessage]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void loadReleaseContent(selectedReleaseID); }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadReleaseContent, selectedReleaseID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedPackageID || !selectedReleaseID) {
      queueMicrotask(() => { if (!cancelled) setReleaseLifecycle(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.sdkReleaseLifecycle(selectedPackageID, selectedReleaseID).then((value) => {
      if (!cancelled) setReleaseLifecycle(value);
    }).catch((error) => {
      if (!cancelled) {
        setReleaseLifecycle(null);
        onMessage(developerAssetError(error, "Effective SDK release lifecycle could not be loaded."));
      }
    });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedPackageID, selectedReleaseID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedReleaseID || !selectedCandidateID) {
      queueMicrotask(() => { if (!cancelled) setCandidateRecord(null); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.sdkContentCandidate(selectedReleaseID, selectedCandidateID).then((value) => {
      if (cancelled) return;
      setCandidateRecord(value);
      setFileDecisions({});
      setSampleDecisions({});
      setAcknowledged(false);
    }).catch((error) => { if (!cancelled) { setCandidateRecord(null); onMessage(developerAssetError(error, "The SDK review record could not be loaded.")); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedCandidateID, selectedReleaseID]);

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedPackageID || integrations.length === 0) {
      queueMicrotask(() => { if (!cancelled) setUsedBy([]); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.usage()
      .then((value) => { if (!cancelled) setUsedBy(sdkUsages(value, integrations, selectedPackageID)); })
      .catch(() => { if (!cancelled) setUsedBy([]); });
    return () => { cancelled = true; };
  }, [integrations, live, selectedPackageID]);

  function openPackageEditor(value?: SDKPackage) {
    setEditingPackage(value ?? null);
    setEcosystem(value?.ecosystem ?? "npm");
    setCoordinate(value?.display_coordinate ?? "");
    setPackageName(value?.name ?? "");
    setPackageDescription(value?.description ?? "");
    setLanguage(value?.language ?? "");
    setPackageVisibility(value?.visibility ?? "private");
    setPackageLifecycle(value?.lifecycle ?? "draft");
    setPackageAcknowledged(false);
    setPackageOpen(true);
  }

  async function savePackage() {
    if (!coordinate.trim() || !packageName.trim() || !ecosystem.trim()) return;
    setBusy(true);
    try {
      const input = {
        ecosystem: ecosystem.trim(),
        coordinate: coordinate.trim(),
        name: packageName.trim(),
        description: packageDescription.trim(),
        language: language.trim(),
        visibility: packageVisibility,
        lifecycle: packageLifecycle,
        ...(editingPackage?.registry_url ? { registry_url: editingPackage.registry_url } : {}),
        ...(editingPackage?.source_url ? { source_url: editingPackage.source_url } : {}),
        ...(editingPackage?.platform ? { platform: editingPackage.platform } : {}),
        ...(editingPackage?.replacement_sdk_package_id ? { replacement_sdk_package_id: editingPackage.replacement_sdk_package_id } : {}),
        ...(editingPackage?.deprecation_message ? { deprecation_message: editingPackage.deprecation_message } : {}),
      };
      const saved = editingPackage
        ? await developerAssetsApi.updateSDKPackage(editingPackage.id, { ...input, revision: editingPackage.revision })
        : await developerAssetsApi.createSDKPackage(input);
      setPackageOpen(false);
      await load();
      setSelectedPackageID(saved.id);
      onMessage(editingPackage ? "SDK package root updated. Exact releases and API attachment versions remain unchanged." : "Deployment-owned SDK package created. Add an exact immutable release next.");
    } catch (error) {
      onMessage(developerAssetError(error, `SDK package could not be ${editingPackage ? "updated" : "created"}.`));
    } finally { setBusy(false); }
  }

  async function createRelease() {
    if (!selectedPackage || !exactVersion.trim()) return;
    setBusy(true);
    try {
      const saved = await developerAssetsApi.createSDKRelease(selectedPackage.id, {
        exact_version: exactVersion.trim(),
        ...(installCommand.trim() ? { install_command: installCommand.trim() } : {}),
        source_url: sourceURL.trim() || undefined,
        resolved_source_revision: sourceRevision.trim() || undefined,
        identity_assurance: sourceRevision.trim() ? "resolved_source" : "metadata_only",
        visibility: "private",
        lifecycle: "active",
      });
      setReleaseOpen(false);
      await loadReleases(selectedPackage.id);
      setSelectedReleaseID(saved.id);
      onMessage(`Exact SDK release ${saved.exact_version} created. It will never follow latest or auto-upgrade.`);
    } catch (error) {
      onMessage(developerAssetError(error, "Exact SDK release could not be created."));
    } finally { setBusy(false); }
  }

  function openLifecycleEvent() {
    setLifecycleValue(selectedReleaseLifecycle?.effective_lifecycle ?? selectedRelease?.lifecycle ?? "active");
    setLifecycleReason("");
    setLifecycleSourceURI("");
    setLifecycleObservedAt("");
    setLifecycleOpen(true);
  }

  async function appendLifecycleEvent() {
    if (!selectedPackage || !selectedRelease || !lifecycleReason.trim()) return;
    const sourceURI = lifecycleSourceURI.trim();
    const observedAt = lifecycleObservedAt ? new Date(lifecycleObservedAt) : null;
    if (sourceURI && !/^https:\/\//i.test(sourceURI)) return;
    if (observedAt && (!Number.isFinite(observedAt.getTime()) || observedAt.getTime() > Date.now())) return;
    setBusy(true);
    try {
      const value = await developerAssetsApi.appendSDKReleaseLifecycleEvent(selectedPackage.id, selectedRelease.id, {
        lifecycle: lifecycleValue,
        reason: lifecycleReason.trim(),
        ...(sourceURI ? { observed_source_uri: sourceURI } : {}),
        ...(observedAt ? { observed_at: observedAt.toISOString() } : {}),
      });
      setReleaseLifecycle(value);
      setLifecycleOpen(false);
      onMessage(`Effective lifecycle for ${selectedRelease.exact_version} is now ${value.effective_lifecycle}. The immutable release identity and historical publications were preserved.`);
    } catch (error) {
      onMessage(developerAssetError(error, "The reviewed lifecycle event could not be recorded."));
    } finally { setBusy(false); }
  }

  async function ingestContent() {
    if (!selectedRelease) return;
    const currentFile = sourcePath.trim() && fileContent ? [ingestionFile(sourcePath, fileContent, fileLanguage)] : [];
    const files = [...queuedFiles, ...currentFile];
    if (files.length === 0) return;
    const fileBytes = files.map((file) => sdkTextBytes(file.content));
    if (files.length > maxSDKIngestionFiles || fileBytes.some((size) => size > maxSDKIngestionFileBytes) || fileBytes.reduce((total, size) => total + size, 0) > maxSDKIngestionTotalBytes) {
      onMessage("Text files must stay within 500 files, 2 MiB per file, and 20 MiB total before normalization.");
      return;
    }
    setBusy(true);
    try {
      const result = await developerAssetsApi.ingestSDKContent(selectedRelease.id, { files, ...(sourceURL.trim() ? { resolved_source_uri: sourceURL.trim() } : {}), ...(sourceRevision.trim() ? { resolved_source_revision: sourceRevision.trim() } : {}) });
      setIngestOpen(false);
      setQueuedFiles([]);
      setRejectedFiles([]);
      await loadReleaseContent(selectedRelease.id);
      setSelectedCandidateID(result.candidate.candidate.id);
      onMessage(result.already_ingested ? "That exact source generation was already ingested." : "Text content normalized without executing package code. Human review is still required.");
    } catch (error) {
      onMessage(developerAssetError(error, "SDK content could not be ingested."));
    } finally { setBusy(false); }
  }

  function ingestionFile(path: string, content: string, fileLanguageValue: string): SDKIngestionFile {
    return { source_path: path.trim(), content, media_type: fileLanguageValue === "markdown" ? "text/markdown" : "text/plain", language: fileLanguageValue, role: path.toLowerCase().includes("readme") ? "readme" : "source" };
  }

  function queueCurrentFile() {
    if (!sourcePath.trim() || !fileContent || queuedFiles.length >= maxSDKIngestionFiles) return;
    setQueuedFiles((current) => [...current, ingestionFile(sourcePath, fileContent, fileLanguage)]);
    setSourcePath("");
    setFileContent("");
  }

  async function chooseLocalTextFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    setFilePickerBusy(true);
    const accepted: SDKIngestionFile[] = [];
    const rejected: SDKRejectedFile[] = [];
    const knownPaths = new Set(queuedFiles.map((file) => file.source_path.toLocaleLowerCase()));
    let totalBytes = queuedFiles.reduce((total, file) => total + sdkTextBytes(file.content), 0);
    try {
      for (const file of Array.from(files)) {
        const originalPath = file.webkitRelativePath || file.name;
        const path = sdkNormalizedLocalPath(originalPath);
        if (!path || path.length > 500) { rejected.push({ path: originalPath || file.name, reason: "path must be a bounded relative path without dot or traversal segments" }); continue; }
        if (queuedFiles.length + accepted.length >= maxSDKIngestionFiles) { rejected.push({ path, reason: "500-file limit reached" }); continue; }
        const pathKey = path.toLocaleLowerCase();
        if (knownPaths.has(pathKey)) { rejected.push({ path, reason: "duplicate relative path (case-insensitive)" }); continue; }
        if (file.size === 0) { rejected.push({ path, reason: "empty file" }); continue; }
        if (file.size > maxSDKIngestionFileBytes) { rejected.push({ path, reason: "larger than 2 MiB" }); continue; }
        if (totalBytes + file.size > maxSDKIngestionTotalBytes) { rejected.push({ path, reason: "20 MiB total limit reached" }); continue; }
        try {
          const buffer = await file.arrayBuffer();
          if (!sdkBufferLooksText(buffer)) { rejected.push({ path, reason: "binary or control-byte content" }); continue; }
          const content = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
          const languageValue = sdkLanguageForPath(path);
          accepted.push({
            source_path: path,
            content,
            media_type: file.type.startsWith("text/") ? file.type : languageValue === "markdown" ? "text/markdown" : languageValue === "json" ? "application/json" : "text/plain",
            language: languageValue,
            role: path.toLowerCase().includes("readme") ? "readme" : path.toLowerCase().includes("example") ? "example" : "source",
          });
          knownPaths.add(pathKey);
          totalBytes += file.size;
        } catch {
          rejected.push({ path, reason: "not valid UTF-8 text" });
        }
      }
      setQueuedFiles((current) => [...current, ...accepted]);
      setRejectedFiles(rejected);
    } finally { setFilePickerBusy(false); }
  }

  async function publishCandidate() {
    if (!selectedRelease || !candidateRecord || !acknowledged || !decisionsComplete(candidateRecord.files, fileDecisions, "file") || !decisionsComplete(candidateRecord.samples, sampleDecisions, "sample")) return;
    setBusy(true);
    try {
      const publication = await developerAssetsApi.publishSDKContentCandidate(selectedRelease.id, candidateRecord.candidate.id, decisionPayload(candidateRecord.files, fileDecisions, "file"), decisionPayload(candidateRecord.samples, sampleDecisions, "sample"));
      setReviewOpen(false);
      await loadReleaseContent(selectedRelease.id);
      onMessage(`Reviewed SDK content publication r${publication.revision} created for exact release ${selectedRelease.exact_version}.`);
    } catch (error) {
      onMessage(developerAssetError(error, "SDK content candidate could not be published."));
    } finally { setBusy(false); }
  }

  function setDecision(kind: "file" | "sample", id: string, decision: SDKDecisionState[string]["decision"], reason?: string, reviewEvidence?: string) {
    const setter = kind === "file" ? setFileDecisions : setSampleDecisions;
    setter((current) => ({
      ...current,
      [id]: {
        decision,
        reason: reason ?? current[id]?.reason ?? "",
        reviewEvidence: reviewEvidence ?? current[id]?.reviewEvidence ?? "",
      },
    }));
  }

  function reviewRows(records: DeveloperAssetRecord[], kind: "file" | "sample") {
    const decisions = kind === "file" ? fileDecisions : sampleDecisions;
    return <div className="developer-asset-decision-list">{records.map((record, index) => {
      const id = recordID(record, `${kind}-${index}`);
      const current = decisions[id] ?? { decision: "", reason: "", reviewEvidence: "" };
      const machineValidated = kind === "sample" && sampleValidated(record);
      return <div key={id}>
        <span><strong>{recordTitle(record, id)}</strong><small><code>{id}</code>{kind === "sample" && ` · ${machineValidated ? "machine evidence passed" : "explicit review evidence required for approval"}`}</small></span>
        <label><span>Decision</span><select aria-label={`${kind} decision for ${recordTitle(record, id)}`} value={current.decision} onChange={(event) => setDecision(kind, id, event.target.value as SDKDecisionState[string]["decision"])}><option value="">Choose…</option>{kind === "file" ? <option value="included">Include</option> : <option value="approved">Approve</option>}<option value="excluded">Exclude</option><option value="quarantined">Quarantine</option></select></label>
        {(current.decision === "excluded" || current.decision === "quarantined") && <label><span>Reason</span><input value={current.reason} onChange={(event) => setDecision(kind, id, current.decision, event.target.value)} /></label>}
        {kind === "sample" && current.decision === "approved" && !machineValidated && <label><span>Explicit review evidence</span><textarea value={current.reviewEvidence} onChange={(event) => setDecision(kind, id, current.decision, undefined, event.target.value)} placeholder="Summarize the manual checks that justify approval of this exact sample." /><small>A status label alone is not validation. This structured summary is stored with the immutable publication review.</small></label>}
        {kind === "sample" && <div className="developer-ai-sample-actions">{advisoryUsages.map(({ integration, binding, publication }) => publication && selectedContentPublication ? <DeveloperAssetAIAdvisoryButton key={`${binding.id}-${id}`} input={{ prompt_key: "sdk.sample_review", api_id: integration.id, api_developer_asset_publication_id: publication.id, api_sdk_binding_id: binding.id, sdk_content_publication_id: selectedContentPublication.id, sdk_code_sample_id: id }} subject={`${recordTitle(record, id)} · ${integration.display_name}`} label={`AI advisory · ${integration.display_name}`} /> : null)}{advisoryUsages.length === 0 && <DeveloperAssetAIAdvisoryButton input={null} subject={recordTitle(record, id)} unavailableReason="First publish this human-reviewed SDK candidate, attach that exact publication to an API, and publish the API resource snapshot." />}</div>}
      </div>;
    })}</div>;
  }

  const active: Section = "sdks";
  const reviewComplete = Boolean(candidateRecord && decisionsComplete(candidateRecord.files, fileDecisions, "file") && decisionsComplete(candidateRecord.samples, sampleDecisions, "sample"));
  const lifecycleSourceInvalid = Boolean(lifecycleSourceURI.trim() && !/^https:\/\//i.test(lifecycleSourceURI.trim()));
  const lifecycleObservedInvalid = Boolean(lifecycleObservedAt && (!Number.isFinite(new Date(lifecycleObservedAt).getTime()) || new Date(lifecycleObservedAt).getTime() > Date.now()));
  const queuedFileBytes = queuedFiles.reduce((total, file) => total + sdkTextBytes(file.content), 0);
  const pastedFileBytes = fileContent ? sdkTextBytes(fileContent) : 0;
  const pendingPastedFile = Boolean(sourcePath.trim() && fileContent);
  const ingestionSizeInvalid = pastedFileBytes > maxSDKIngestionFileBytes || queuedFileBytes + pastedFileBytes > maxSDKIngestionTotalBytes || queuedFiles.length + (pendingPastedFile ? 1 : 0) > maxSDKIngestionFiles;

  return <>
    <PageHeader eyebrow="Catalog" title="SDKs" description="Manage reusable package identities, exact immutable releases, and human-reviewed content publications." action={<Button onClick={() => openPackageEditor()}><PackagePlus data-slot="icon" />Create package</Button>} />
    <CatalogNavigation active={active} onNavigate={onNavigate} />
    <ExactVersionNotice>Packages have no implicit current version. Content ingestion is text-only and never executes package code; APIs bind one exact release by explicit action.</ExactVersionNotice>
    {loading ? <LoadingPanel label="Loading SDK packages" /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label="SDK package catalog" className="developer-asset-directory">
        <DataTableHeader className="developer-sdk-columns"><span>Package</span><span>Ecosystem</span><span>Lifecycle</span></DataTableHeader>
        {packages.map((sdkPackage) => <DataTableRow className={`developer-sdk-columns developer-asset-selectable ${sdkPackage.id === selectedPackageID ? "selected" : ""}`} key={sdkPackage.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedPackageID(sdkPackage.id); setTab("files"); setAssetQuery(""); }}><span className="resource-icon"><Box /></span><span><strong>{sdkPackage.name}</strong><small><code>{sdkPackage.display_coordinate}</code></small></span></button>
          <span><Badge color="violet">{sdkPackage.ecosystem}</Badge><small className="cell-note">{sdkPackage.language || "Language unspecified"}</small></span>
          <span><ReviewStateBadge state={sdkPackage.lifecycle} /></span>
        </DataTableRow>)}
        {packages.length === 0 && <DataTableEmpty columns={3}>No deployment-owned SDK package exists yet.</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selectedPackage ? <>
          <PanelHeader title={selectedPackage.name} description={`${selectedPackage.ecosystem} · ${selectedPackage.display_coordinate}`} action={<span className="heading-actions"><Button outline onClick={() => openPackageEditor(selectedPackage)}><Pencil data-slot="icon" />Edit package</Button>{selectedPackage.lifecycle !== "archived" && <Button onClick={() => { setExactVersion(""); setInstallCommand(""); setSourceURL(selectedPackage.source_url ?? ""); setSourceRevision(""); setReleaseOpen(true); }}><Plus data-slot="icon" />Add exact release</Button>}</span>} />
          <div className="developer-asset-candidate-picker"><label><span>Exact release</span><select value={selectedReleaseID} onChange={(event) => { setSelectedReleaseID(event.target.value); setTab("files"); setAssetQuery(""); }}>{releases.map((release) => <option key={release.id} value={release.id}>{release.exact_version} · {release.identity_assurance}</option>)}</select></label>{selectedRelease && <><ReviewStateBadge state={selectedReleaseLifecycle?.effective_lifecycle ?? selectedRelease.lifecycle} /><Badge color={selectedReleaseLifecycle?.selectable === false ? "red" : "green"}>{selectedReleaseLifecycle?.selectable === false ? "not selectable" : "selectable"}</Badge><code>{selectedRelease.release_hash}</code></>}</div>
          {selectedRelease ? <>
            <div className="developer-sdk-release-summary"><dl className="entity-detail-grid"><div><dt>Exact version</dt><dd><strong>{selectedRelease.exact_version}</strong></dd></div><div><dt>Install</dt><dd><code>{selectedRelease.install_command || "—"}</code></dd></div><div><dt>Effective lifecycle</dt><dd>{selectedReleaseLifecycle?.effective_lifecycle ?? selectedRelease.lifecycle}</dd></div><div><dt>Immutable initial lifecycle</dt><dd>{selectedReleaseLifecycle?.initial_lifecycle ?? selectedRelease.lifecycle}</dd></div><div><dt>Identity assurance</dt><dd>{selectedRelease.identity_assurance.replaceAll("_", " ")}</dd></div><div><dt>Release ID</dt><dd><code>{selectedRelease.id}</code></dd></div></dl><span className="heading-actions"><Button outline onClick={openLifecycleEvent}><History data-slot="icon" />Record lifecycle event</Button><Button outline onClick={() => { setSourcePath("README.md"); setFileContent(""); setFileLanguage("markdown"); setQueuedFiles([]); setRejectedFiles([]); setSourceURL(selectedRelease.source_url ?? selectedPackage.source_url ?? ""); setSourceRevision(selectedRelease.resolved_source_revision ?? ""); setIngestOpen(true); }}><Upload data-slot="icon" />Ingest text content</Button>{candidateRecord && <Button disabled={selectedReleaseLifecycle?.selectable === false} title={selectedReleaseLifecycle?.selectable === false ? "Yanked or archived releases cannot create new content publications." : undefined} onClick={() => { setReviewOpen(true); setAcknowledged(false); }}><ShieldCheck data-slot="icon" />Review candidate</Button>}</span></div>
            <details className="advanced-details developer-sdk-lifecycle"><summary>Effective lifecycle and append-only history</summary><div className="developer-sdk-lifecycle-body"><div className="notice"><History /><span><strong>Historical publications remain readable.</strong> Effective yanked or archived releases cannot be selected for new bindings or publications; no existing exact selection is upgraded, detached, or rewritten.</span></div>{selectedReleaseLifecycle?.events.map((event) => <article key={event.id}><span><ReviewStateBadge state={event.lifecycle} /><strong>{event.reason || "No reason recorded"}</strong><small>Observed {new Date(event.observed_at).toLocaleString()} · recorded by {event.recorded_by}</small>{event.observed_source_uri && <a href={event.observed_source_uri} target="_blank" rel="noreferrer">Open observation evidence</a>}</span><code>{event.id}</code></article>)}{selectedReleaseLifecycle && selectedReleaseLifecycle.events.length === 0 && <p className="empty-row">No lifecycle events. Effective state comes from the immutable release identity.</p>}{!selectedReleaseLifecycle && <p className="empty-row">Effective lifecycle history is unavailable.</p>}</div></details>
            <div className="developer-asset-candidate-picker"><label><span>Content candidate</span><select value={selectedCandidateID} onChange={(event) => { setSelectedCandidateID(event.target.value); setTab("files"); setAssetQuery(""); }}>{candidates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.id} · {candidate.content_hash}</option>)}</select></label><span className="heading-actions"><Badge color={selectedContentPublication ? "green" : "amber"}>{selectedContentPublication ? `published r${selectedContentPublication.revision}` : "review required"}</Badge><DeveloperAssetAIAdvisoryButton input={selectedContentPublication ? { prompt_key: "sdk.map_enrichment", sdk_content_publication_id: selectedContentPublication.id } : null} subject={`${selectedPackage.name} ${selectedRelease.exact_version} content publication`} label="AI SDK map" unavailableReason="Publish this exact SDK content candidate after human review before requesting advisory AI." /></span></div>
            {candidateRecord ? <>
              <div className="developer-asset-inspector-tabs"><SegmentedControl label="SDK content review" value={tab} onChange={setTab} items={[{ id: "files", label: "Files", count: candidateRecord.files.length }, { id: "symbols", label: "Symbols", count: candidateRecord.symbols.length }, { id: "samples", label: "Samples", count: candidateRecord.samples.length }, { id: "map", label: "SDK Map" }, { id: "diagnostics", label: "Diagnostics" }, { id: "used-by", label: "Used by APIs", count: usedBy.length }]} /></div>
              {(tab === "files" || tab === "symbols" || tab === "samples") && <div className="developer-sdk-local-filter"><div className="search-field"><Search /><input type="search" aria-label={`Filter SDK ${tab}`} placeholder="Filter path, title, language, role, or hash…" value={assetQuery} onChange={(event) => setAssetQuery(event.target.value)} /></div><span>{filteredAssetCount} of {totalAssetCount}</span>{assetQuery && <Button outline onClick={() => setAssetQuery("")}>Clear</Button>}</div>}
              <div className="developer-asset-inspector-body">
                {tab === "files" && <div className="developer-asset-file-list">{filteredFiles.map((file, index) => <article key={recordID(file, `file-${index}`)}><header><span><FileCode2 /><strong>{recordTitle(file, `File ${index + 1}`)}</strong></span><Badge>{recordString(file, "role", "language") || "file"}</Badge></header><small>{recordString(file, "source_path", "path", "content_hash")}</small><PrettyJSON value={file} /></article>)}{candidateRecord.files.length === 0 ? <p className="empty-row">No files were normalized.</p> : filteredFiles.length === 0 && <p className="empty-row">No files match this local filter.</p>}</div>}
                {tab === "symbols" && <div className="developer-asset-record-list">{filteredSymbols.map((symbol, index) => <article key={recordID(symbol, `symbol-${index}`)}><strong>{recordTitle(symbol, `Symbol ${index + 1}`)}</strong><PrettyJSON value={symbol} /></article>)}{candidateRecord.symbols.length === 0 ? <p className="empty-row">No symbols were extracted.</p> : filteredSymbols.length === 0 && <p className="empty-row">No symbols match this local filter.</p>}</div>}
                {tab === "samples" && <div className="developer-asset-sample-list">{filteredSamples.map((sample, index) => <article key={recordID(sample, `sample-${index}`)}><header><span><Code2 /><strong>{recordTitle(sample, `Sample ${index + 1}`)}</strong></span><Badge color={sampleValidated(sample) ? "green" : "amber"}>{sampleValidated(sample) ? "machine evidence passed" : "review evidence required"}</Badge></header><PrettyJSON value={sample} /></article>)}{candidateRecord.samples.length === 0 ? <p className="empty-row">No code samples were extracted.</p> : filteredSamples.length === 0 && <p className="empty-row">No samples match this local filter.</p>}</div>}
                {tab === "map" && <>{candidateRecord.map ? <><MarkdownEvidence label="SDK Map agent markdown">{candidateRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={candidateRecord.map.map} label="SDK Map data" /></> : <p className="empty-row">No SDK Map is stored for this exact candidate.</p>}</>}
                {tab === "diagnostics" && <PrettyJSON value={{ diagnostics: candidateRecord.candidate.diagnostics, versions: candidateRecord.candidate.versions, source_manifest: candidateRecord.candidate.source_manifest, sample_refs: candidateRecord.sample_refs }} label="SDK candidate diagnostics and citations" />}
                {tab === "used-by" && <div className="developer-asset-used-by">{usedBy.map(({ integration, binding }) => <ConsoleLink key={binding.id} path={`/integration/${encodeURIComponent(integration.id)}/documentation`} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} · exact release {binding.sdk_release_id}</small></span><Badge color={binding.state === "ready" ? "green" : "amber"}>{binding.state}</Badge></ConsoleLink>)}{usedBy.length === 0 && <p className="empty-row">This package is not attached to an API.</p>}</div>}
              </div>
            </> : <p className="empty-row">No normalized content candidate exists for this exact release.</p>}
          </> : <p className="empty-row">Create an exact immutable release before ingesting or attaching content.</p>}
        </> : <div className="developer-asset-inspector-empty"><Box /><strong>Select an SDK package</strong><small>Exact releases, files, symbols, samples, maps, review history, and API usage will appear here.</small></div>}
      </section>
    </div>}
    <Dialog open={packageOpen} onClose={setPackageOpen} title={editingPackage ? "Edit SDK package" : "Create SDK package"} description={editingPackage ? "Update package-root metadata and lifecycle. Existing exact releases and API pins are never rewritten." : "Create a reusable package identity. It does not imply a current or latest version."} actions={<><Button outline onClick={() => setPackageOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !ecosystem.trim() || !coordinate.trim() || !packageName.trim() || Boolean(editingPackage && !packageAcknowledged)} onClick={() => void savePackage()}>{busy ? "Saving…" : editingPackage ? "Save package root" : "Create package"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Ecosystem</span><input value={ecosystem} onChange={(event) => setEcosystem(event.target.value)} placeholder="npm" /></label><label className="auth-field"><span>Coordinate</span><input value={coordinate} onChange={(event) => setCoordinate(event.target.value)} placeholder="@acme/payments" /></label></div><label className="auth-field"><span>Name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} /></label><label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Language</span><input value={language} onChange={(event) => setLanguage(event.target.value)} /></label><label className="auth-field"><span>Visibility</span><select value={packageVisibility} onChange={(event) => setPackageVisibility(event.target.value as SDKPackage["visibility"])}><option value="private">Private</option><option value="public">Public</option></select></label></div><label className="auth-field"><span>Lifecycle</span><select value={packageLifecycle} onChange={(event) => setPackageLifecycle(event.target.value as SDKPackage["lifecycle"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="archived">Archived</option></select><small>Archive preserves exact releases, content publications, attachments, and audit history.</small></label>{editingPackage && <><div className="notice"><GitBranch /><span><strong>{usedBy.length} affected API attachment{usedBy.length === 1 ? "" : "s"}.</strong> Review them before changing visibility or lifecycle. Their exact releases will not move or auto-upgrade.</span></div><label className="compact-check"><input type="checkbox" checked={packageAcknowledged} onChange={(event) => setPackageAcknowledged(event.target.checked)} /><span>I reviewed the affected APIs and this package-root change.</span></label></>}</div></Dialog>
    <Dialog open={releaseOpen} onClose={setReleaseOpen} title={`Add exact release to ${selectedPackage?.name ?? "SDK package"}`} description="Ranges and latest tags are rejected. This release identity is immutable after creation." actions={<><Button outline onClick={() => setReleaseOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !exactVersion.trim() || exactVersion.trim().toLowerCase() === "latest"} onClick={() => void createRelease()}>{busy ? "Creating…" : "Create exact release"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Exact version</span><input value={exactVersion} onChange={(event) => setExactVersion(event.target.value)} placeholder="1.4.0" /></label><label className="auth-field"><span>Canonical install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="Leave blank for the server canonical command" /><small>Package managers use different canonical syntax. Enter a verified command or let the server derive it.</small></label><label className="auth-field"><span>Resolved source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder="https://…" /></label><label className="auth-field"><span>Resolved source revision</span><input value={sourceRevision} onChange={(event) => setSourceRevision(event.target.value)} placeholder="commit or immutable tag" /></label><div className="notice"><GitBranch /><span><strong>{usedBy.length} API attachment{usedBy.length === 1 ? "" : "s"} currently use this package.</strong> Creating another exact release does not change any attachment. APIs upgrade only after an explicit exact-release change.</span></div></div></Dialog>
    <Dialog open={lifecycleOpen} onClose={setLifecycleOpen} title={`Record lifecycle event for ${selectedRelease?.exact_version ?? "exact release"}`} description="Append a reviewed observation. The immutable release identity and earlier events are never rewritten." actions={<><Button outline onClick={() => setLifecycleOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !lifecycleReason.trim() || lifecycleSourceInvalid || lifecycleObservedInvalid} onClick={() => void appendLifecycleEvent()}>{busy ? "Recording…" : "Record append-only event"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Observed lifecycle</span><select value={lifecycleValue} onChange={(event) => setLifecycleValue(event.target.value as SDKRelease["lifecycle"])}><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="yanked">Yanked</option><option value="archived">Archived</option></select></label><label className="auth-field"><span>Required reviewed reason</span><textarea maxLength={2000} value={lifecycleReason} onChange={(event) => setLifecycleReason(event.target.value)} placeholder="Describe the registry or administrative evidence for this state." /></label><label className="auth-field"><span>Public HTTPS observation URI (optional)</span><input type="url" inputMode="url" aria-invalid={lifecycleSourceInvalid} value={lifecycleSourceURI} onChange={(event) => setLifecycleSourceURI(event.target.value)} placeholder="https://registry.example/package/version" /><small>{lifecycleSourceInvalid ? "Only a public HTTPS evidence URI is accepted." : "Secrets and private evidence URLs must not be entered."}</small></label><label className="auth-field"><span>Observed at (optional)</span><input type="datetime-local" max={new Date().toISOString().slice(0, 16)} aria-invalid={lifecycleObservedInvalid} value={lifecycleObservedAt} onChange={(event) => setLifecycleObservedAt(event.target.value)} /><small>{lifecycleObservedInvalid ? "Observation time cannot be in the future." : "Leave blank to use the server time."}</small></label><div className="notice"><TriangleAlert /><span><strong>Yanked and archived block new selections.</strong> Historical API publications remain readable, and no binding is automatically upgraded, detached, or rewritten.</span></div></div></Dialog>
    <Dialog open={ingestOpen} onClose={setIngestOpen} title={`Ingest content for ${selectedRelease?.exact_version ?? "exact release"}`} description="Only supplied, bounded UTF-8 text files are normalized. DokoSoko never executes package code or fetches a caller-selected URL." actions={<><Button outline onClick={() => setIngestOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || filePickerBusy || ingestionSizeInvalid || (queuedFiles.length === 0 && !pendingPastedFile)} onClick={() => void ingestContent()}>{busy ? "Normalizing…" : "Normalize without executing"}</Button></>}>
      <div className="auth-form compact-form">
        <label className="developer-local-file-picker"><Upload /><span><strong>{filePickerBusy ? "Reading text files…" : "Choose multiple text files"}</strong><small>Relative paths are preserved when the browser supplies them. Binary, invalid UTF-8, oversized, duplicate, and over-limit files are rejected locally.</small></span><input type="file" multiple accept="text/*,.md,.mdx,.json,.yaml,.yml,.toml,.ts,.tsx,.js,.jsx,.py,.go,.rb,.rs,.java,.kt,.swift,.php,.cs,.xml,.html,.css" disabled={filePickerBusy || queuedFiles.length >= maxSDKIngestionFiles} onChange={(event) => { const input = event.currentTarget; void chooseLocalTextFiles(input.files).finally(() => { input.value = ""; }); }} /></label>
        {queuedFiles.length > 0 && <div className="developer-ingestion-file-queue"><strong>{queuedFiles.length} file{queuedFiles.length === 1 ? "" : "s"} queued · {(queuedFileBytes / 1_048_576).toFixed(2)} MiB</strong>{queuedFiles.map((file, index) => <div key={`${file.source_path}-${index}`}><span><code>{file.source_path}</code><small>{file.language} · {sdkTextBytes(file.content).toLocaleString()} bytes</small></span><Button outline onClick={() => setQueuedFiles((current) => current.filter((_, itemIndex) => itemIndex !== index))}>Remove</Button></div>)}</div>}
        {rejectedFiles.length > 0 && <div className="developer-rejected-file-list"><header><span><TriangleAlert /><strong>{rejectedFiles.length} file{rejectedFiles.length === 1 ? "" : "s"} rejected</strong></span><Button outline onClick={() => setRejectedFiles([])}>Dismiss</Button></header>{rejectedFiles.map((file, index) => <div key={`${file.path}-${index}`}><code>{file.path}</code><small>{file.reason}</small></div>)}</div>}
        <div className="developer-paste-divider"><span>Or paste one text file</span></div>
        <div className="two-fields"><label className="auth-field"><span>Relative source path</span><input maxLength={500} value={sourcePath} onChange={(event) => setSourcePath(event.target.value)} /></label><label className="auth-field"><span>Language</span><input value={fileLanguage} onChange={(event) => setFileLanguage(event.target.value)} /></label></div>
        <label className="auth-field"><span>Text content</span><textarea className="code-input" maxLength={2_097_152} value={fileContent} onChange={(event) => setFileContent(event.target.value)} spellCheck={false} /><small>{pastedFileBytes.toLocaleString()} bytes · 1–500 files, at most 2 MiB each and 20 MiB total.</small></label>
        <Button type="button" outline disabled={!pendingPastedFile || ingestionSizeInvalid || queuedFiles.length >= maxSDKIngestionFiles} onClick={queueCurrentFile}><Plus data-slot="icon" />Queue pasted file</Button>
        {ingestionSizeInvalid && <div className="inline-warning"><TriangleAlert />The pending text exceeds the 2 MiB file, 20 MiB total, or 500-file limit.</div>}
        <div className="notice"><ShieldCheck /><span><strong>No code execution.</strong> Static normalization and validation produce a candidate that remains unpublished until every file and sample receives a human decision.</span></div>
      </div>
    </Dialog>
    <Dialog open={reviewOpen} onClose={setReviewOpen} title="Review SDK content candidate" description="Decide every file and sample. Approval requires positive machine evidence or an explicit review-evidence summary; excluded or quarantined evidence requires a reason." actions={<><Button outline onClick={() => setReviewOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !reviewComplete || !acknowledged} onClick={() => void publishCandidate()}>{busy ? "Publishing…" : "Publish reviewed content"}</Button></>}><div className="developer-asset-sdk-review"><section><h3>Files</h3>{candidateRecord && reviewRows(candidateRecord.files, "file")}</section><section><h3>Samples</h3>{candidateRecord && reviewRows(candidateRecord.samples, "sample")}</section><label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>I reviewed every decision, validation result, citation, and SDK Map entry for this exact release.</span></label></div></Dialog>
  </>;
}
