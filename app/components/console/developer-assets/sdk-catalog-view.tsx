"use client";


import { useTranslation } from "react-i18next";
import { Box, Code2, FileCode2, GitBranch, History, PackageSearch, Pencil, Plus, Search, ShieldCheck, TriangleAlert, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
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
import { developerAssetError, enumLabel, LoadingPanel, MarkdownEvidence, PrettyJSON, ProblemPanel, recordID, recordString, recordTitle, ReviewStateBadge } from "./developer-asset-ui";
import { sdkUsages, type SDKUsage } from "./developer-asset-usage";
import { SDKPackageImportDialog } from "./sdk-package-import-dialog";
import { decisionPayload, decisionsComplete, maxSDKIngestionFileBytes, maxSDKIngestionFiles, maxSDKIngestionTotalBytes, sampleValidated, sdkBufferLooksText, sdkExplorerRecordMatches, sdkLanguageForPath, sdkNormalizedLocalPath, sdkTextBytes, type SDKDecisionState } from "./sdk-catalog-helpers";

type SDKTab = "files" | "symbols" | "samples" | "map" | "diagnostics" | "used-by";
type SDKRejectedFile = { path: string; reason: string };

export function SDKCatalogView({ live, integrations, onMessage, onNavigate }: { live: boolean; integrations: APIIntegration[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
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
  const [importOpen, setImportOpen] = useState(false);
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
      setProblem(developerAssetError(error, t("sdkCatalog.sdkPackagesCouldNotBeLoaded")));
    } finally {
      setLoading(false);
    }
  }, [live, t]);

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
  const filteredFiles = useMemo(() => candidateRecord?.files.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, t("sdkCatalog.fileNumber", { number: index + 1 })))) ?? [], [candidateRecord, normalizedAssetQuery, t]);
  const filteredSymbols = useMemo(() => candidateRecord?.symbols.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, t("sdkCatalog.symbolNumber", { number: index + 1 })))) ?? [], [candidateRecord, normalizedAssetQuery, t]);
  const filteredSamples = useMemo(() => candidateRecord?.samples.filter((record, index) => sdkExplorerRecordMatches(record, normalizedAssetQuery, recordTitle(record, t("sdkCatalog.sampleNumber", { number: index + 1 })))) ?? [], [candidateRecord, normalizedAssetQuery, t]);
  const filteredAssetCount = tab === "files" ? filteredFiles.length : tab === "symbols" ? filteredSymbols.length : filteredSamples.length;
  const totalAssetCount = tab === "files" ? candidateRecord?.files.length ?? 0 : tab === "symbols" ? candidateRecord?.symbols.length ?? 0 : candidateRecord?.samples.length ?? 0;

  const loadReleases = useCallback(async (packageID: string) => {
    if (!live || !packageID) return;
    try {
      const values = await developerAssetsApi.sdkReleases(packageID);
      setReleases(values);
      setSelectedReleaseID((current) => values.some((item) => item.id === current) ? current : values[0]?.id ?? "");
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.exactSDKReleasesCouldNotBeLoaded")));
    }
  }, [live, onMessage, t]);

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
      onMessage(developerAssetError(error, t("sdkCatalog.sdkContentCandidatesCouldNotBeLoaded")));
    }
  }, [live, onMessage, t]);

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
        onMessage(developerAssetError(error, t("sdkCatalog.effectiveSDKReleaseLifecycleCouldNotBeLoaded")));
      }
    });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedPackageID, selectedReleaseID, t]);

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
    }).catch((error) => { if (!cancelled) { setCandidateRecord(null); onMessage(developerAssetError(error, t("sdkCatalog.theSDKReviewRecordCouldNotBeLoaded"))); } });
    return () => { cancelled = true; };
  }, [live, onMessage, selectedCandidateID, selectedReleaseID, t]);

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

  function openPackageEditor(value: SDKPackage) {
    setEditingPackage(value);
    setEcosystem(value.ecosystem);
    setCoordinate(value.display_coordinate);
    setPackageName(value.name);
    setPackageDescription(value.description);
    setLanguage(value.language ?? "");
    setPackageVisibility(value.visibility);
    setPackageLifecycle(value.lifecycle);
    setPackageAcknowledged(false);
    setPackageOpen(true);
  }

  async function savePackage() {
    if (!editingPackage || !coordinate.trim() || !packageName.trim() || !ecosystem.trim()) return;
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
        ...(editingPackage.registry_url ? { registry_url: editingPackage.registry_url } : {}),
        ...(editingPackage.source_url ? { source_url: editingPackage.source_url } : {}),
        ...(editingPackage.platform ? { platform: editingPackage.platform } : {}),
        ...(editingPackage.replacement_sdk_package_id ? { replacement_sdk_package_id: editingPackage.replacement_sdk_package_id } : {}),
        ...(editingPackage.deprecation_message ? { deprecation_message: editingPackage.deprecation_message } : {}),
      };
      const saved = await developerAssetsApi.updateSDKPackage(editingPackage.id, { ...input, revision: editingPackage.revision });
      setPackageOpen(false);
      await load();
      setSelectedPackageID(saved.id);
      onMessage(t("sdkCatalog.sdkPackageRootUpdatedExactReleasesAndAPIAttachment"));
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.sdkPackageCouldNotBe", { value1: "updated" })));
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
      onMessage(t("sdkCatalog.exactSDKReleaseCreatedItWillNeverFollowLatest", { exact_version: String(saved.exact_version) }));
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.exactSDKReleaseCouldNotBeCreated")));
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
      onMessage(t("sdkCatalog.effectiveLifecycleForIsNowTheImmutableReleaseIdentity", { exact_version: String(selectedRelease.exact_version), effective_lifecycle: String(value.effective_lifecycle) }));
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.theReviewedLifecycleEventCouldNotBeRecorded")));
    } finally { setBusy(false); }
  }

  async function ingestContent() {
    if (!selectedRelease) return;
    const currentFile = sourcePath.trim() && fileContent ? [ingestionFile(sourcePath, fileContent, fileLanguage)] : [];
    const files = [...queuedFiles, ...currentFile];
    if (files.length === 0) return;
    const fileBytes = files.map((file) => sdkTextBytes(file.content));
    if (files.length > maxSDKIngestionFiles || fileBytes.some((size) => size > maxSDKIngestionFileBytes) || fileBytes.reduce((total, size) => total + size, 0) > maxSDKIngestionTotalBytes) {
      onMessage(t("sdkCatalog.textFilesMustStayWithinN500FilesN2MiB"));
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
      onMessage(result.already_ingested ? t("sdkCatalog.thatExactSourceGenerationWasAlreadyIngested") : t("sdkCatalog.textContentNormalizedWithoutExecutingPackageCodeHumanReview"));
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.sdkContentCouldNotBeIngested")));
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
      onMessage(t("sdkCatalog.reviewedSDKContentPublicationRCreatedForExactRelease", { revision: String(publication.revision), exact_version: String(selectedRelease.exact_version) }));
    } catch (error) {
      onMessage(developerAssetError(error, t("sdkCatalog.sdkContentCandidateCouldNotBePublished")));
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
        <span><strong>{recordTitle(record, id)}</strong><small><code>{id}</code>{kind === "sample" && machineValidated ? t("sdkCatalog.machineEvidencePassed") : t("sdkCatalog.explicitReviewEvidenceRequired")}</small></span>
        <label><span>{t("sdkCatalog.decision")}</span><select aria-label={t("sdkCatalog.decisionFor", { kind: String(kind), value2: String(recordTitle(record, id)) })} value={current.decision} onChange={(event) => setDecision(kind, id, event.target.value as SDKDecisionState[string]["decision"])}><option value="">{t("sdkCatalog.choose")}</option>{kind === "file" ? <option value="included">{t("sdkCatalog.include")}</option> : <option value="approved">{t("sdkCatalog.approve")}</option>}<option value="excluded">{t("sdkCatalog.exclude")}</option><option value="quarantined">{t("sdkCatalog.quarantine")}</option></select></label>
        {(current.decision === "excluded" || current.decision === "quarantined") && <label><span>{t("sdkCatalog.reason")}</span><input value={current.reason} onChange={(event) => setDecision(kind, id, current.decision, event.target.value)} /></label>}
        {kind === "sample" && current.decision === "approved" && !machineValidated && <label><span>{t("sdkCatalog.explicitReviewEvidence")}</span><textarea value={current.reviewEvidence} onChange={(event) => setDecision(kind, id, current.decision, undefined, event.target.value)} placeholder={t("sdkCatalog.summarizeTheManualChecksThatJustifyApprovalOfThis")} /><small>{t("sdkCatalog.aStatusLabelAloneIsNotValidationThisStructured")}</small></label>}
        {kind === "sample" && <div className="developer-ai-sample-actions">{advisoryUsages.map(({ integration, binding, publication }) => publication && selectedContentPublication ? <DeveloperAssetAIAdvisoryButton key={`${binding.id}-${id}`} input={{ prompt_key: "sdk.sample_review", api_id: integration.id, api_developer_asset_publication_id: publication.id, api_sdk_binding_id: binding.id, sdk_content_publication_id: selectedContentPublication.id, sdk_code_sample_id: id }} subject={`${recordTitle(record, id)} · ${integration.display_name}`} label={t("sdkCatalog.aiAdvisory", { display_name: String(integration.display_name) })} /> : null)}{advisoryUsages.length === 0 && <DeveloperAssetAIAdvisoryButton input={null} subject={recordTitle(record, id)} unavailableReason={t("sdkCatalog.publishAttachAndSnapshotBeforeAI")} />}</div>}
      </div>;
    })}</div>;
  }

  const reviewComplete = Boolean(candidateRecord && decisionsComplete(candidateRecord.files, fileDecisions, "file") && decisionsComplete(candidateRecord.samples, sampleDecisions, "sample"));
  const lifecycleSourceInvalid = Boolean(lifecycleSourceURI.trim() && !/^https:\/\//i.test(lifecycleSourceURI.trim()));
  const lifecycleObservedInvalid = Boolean(lifecycleObservedAt && (!Number.isFinite(new Date(lifecycleObservedAt).getTime()) || new Date(lifecycleObservedAt).getTime() > Date.now()));
  const queuedFileBytes = queuedFiles.reduce((total, file) => total + sdkTextBytes(file.content), 0);
  const pastedFileBytes = fileContent ? sdkTextBytes(fileContent) : 0;
  const pendingPastedFile = Boolean(sourcePath.trim() && fileContent);
  const ingestionSizeInvalid = pastedFileBytes > maxSDKIngestionFileBytes || queuedFileBytes + pastedFileBytes > maxSDKIngestionTotalBytes || queuedFiles.length + (pendingPastedFile ? 1 : 0) > maxSDKIngestionFiles;

  return <>
    <PageHeader eyebrow={t("sdkCatalog.sdksAndPackages")} title={t("navigation.packages")} action={<Button onClick={() => setImportOpen(true)}><PackageSearch data-slot="icon" />{t("sdkImport.importPackage")}</Button>} />
    {loading ? <LoadingPanel label={t("sdkCatalog.loadingSDKPackages")} /> : problem ? <ProblemPanel message={problem} onRetry={() => void load()} /> : <div className="developer-asset-explorer">
      <DataTable label={t("sdkCatalog.sdkPackageCatalog")} className="developer-asset-directory">
        <DataTableHeader className="developer-sdk-columns"><span>{t("sdkCatalog.package")}</span><span>{t("sdkCatalog.ecosystem")}</span><span>{t("sdkCatalog.lifecycle")}</span></DataTableHeader>
        {packages.map((sdkPackage) => <DataTableRow className={`developer-sdk-columns developer-asset-selectable ${sdkPackage.id === selectedPackageID ? "selected" : ""}`} key={sdkPackage.id}>
          <button type="button" className="developer-asset-record-button" onClick={() => { setSelectedPackageID(sdkPackage.id); setTab("files"); setAssetQuery(""); }}><span className="resource-icon"><Box /></span><span><strong>{sdkPackage.name}</strong><small><code>{sdkPackage.display_coordinate}</code></small></span></button>
          <span><Badge color="violet">{sdkPackage.ecosystem}</Badge><small className="cell-note">{sdkPackage.language || t("sdkCatalog.languageUnspecified")}</small></span>
          <span><ReviewStateBadge state={sdkPackage.lifecycle} /></span>
        </DataTableRow>)}
        {packages.length === 0 && <DataTableEmpty columns={3}>{t("sdkCatalog.noDeploymentOwnedSDKPackageExistsYet")}</DataTableEmpty>}
      </DataTable>
      <section className="panel developer-asset-inspector">
        {selectedPackage ? <>
          <PanelHeader title={selectedPackage.name} description={t("sdkCatalog.copy2", { ecosystem: String(selectedPackage.ecosystem), display_coordinate: String(selectedPackage.display_coordinate) })} action={<span className="heading-actions"><Button outline onClick={() => openPackageEditor(selectedPackage)}><Pencil data-slot="icon" />{t("sdkCatalog.editPackage")}</Button>{selectedPackage.lifecycle !== "archived" && <Button onClick={() => { setExactVersion(""); setInstallCommand(""); setSourceURL(selectedPackage.source_url ?? ""); setSourceRevision(""); setReleaseOpen(true); }}><Plus data-slot="icon" />{t("sdkCatalog.addExactRelease")}</Button>}</span>} />
          <div className="developer-asset-candidate-picker"><label><span>{t("sdkCatalog.exactRelease")}</span><select value={selectedReleaseID} onChange={(event) => { setSelectedReleaseID(event.target.value); setTab("files"); setAssetQuery(""); }}>{releases.map((release) => <option key={release.id} value={release.id}>{release.exact_version} · {enumLabel(t, release.identity_assurance)}</option>)}</select></label>{selectedRelease && <><ReviewStateBadge state={selectedReleaseLifecycle?.effective_lifecycle ?? selectedRelease.lifecycle} /><Badge color={selectedReleaseLifecycle?.selectable === false ? "red" : "green"}>{selectedReleaseLifecycle?.selectable === false ? t("sdkCatalog.notSelectable") : t("sdkCatalog.selectable")}</Badge><code>{selectedRelease.release_hash}</code></>}</div>
          {selectedRelease ? <>
            <div className="developer-sdk-release-summary"><dl className="entity-detail-grid"><div><dt>{t("sdkCatalog.exactVersion")}</dt><dd><strong>{selectedRelease.exact_version}</strong></dd></div><div><dt>{t("sdkCatalog.install")}</dt><dd><code>{selectedRelease.install_command || "—"}</code></dd></div><div><dt>{t("sdkCatalog.effectiveLifecycle")}</dt><dd>{enumLabel(t, selectedReleaseLifecycle?.effective_lifecycle ?? selectedRelease.lifecycle)}</dd></div><div><dt>{t("sdkCatalog.immutableInitialLifecycle")}</dt><dd>{enumLabel(t, selectedReleaseLifecycle?.initial_lifecycle ?? selectedRelease.lifecycle)}</dd></div><div><dt>{t("sdkCatalog.identityAssurance")}</dt><dd>{enumLabel(t, selectedRelease.identity_assurance)}</dd></div><div><dt>{t("sdkCatalog.releaseID")}</dt><dd><code>{selectedRelease.id}</code></dd></div></dl><span className="heading-actions"><Button outline onClick={openLifecycleEvent}><History data-slot="icon" />{t("sdkCatalog.recordLifecycleEvent")}</Button><Button outline onClick={() => { setSourcePath("README.md"); setFileContent(""); setFileLanguage("markdown"); setQueuedFiles([]); setRejectedFiles([]); setSourceURL(selectedRelease.source_url ?? selectedPackage.source_url ?? ""); setSourceRevision(selectedRelease.resolved_source_revision ?? ""); setIngestOpen(true); }}><Upload data-slot="icon" />{t("sdkCatalog.ingestTextContent")}</Button>{candidateRecord && <Button disabled={selectedReleaseLifecycle?.selectable === false} title={selectedReleaseLifecycle?.selectable === false ? t("sdkCatalog.yankedOrArchivedReleasesCannotCreateNewContentPublications") : undefined} onClick={() => { setReviewOpen(true); setAcknowledged(false); }}><ShieldCheck data-slot="icon" />{t("sdkCatalog.reviewCandidate")}</Button>}</span></div>
            <details className="advanced-details developer-sdk-lifecycle"><summary>{t("sdkCatalog.effectiveLifecycleAndAppendOnlyHistory")}</summary><div className="developer-sdk-lifecycle-body"><div className="notice"><History /><span><strong>{t("sdkCatalog.historicalPublicationsRemainReadable")}</strong> {t("sdkCatalog.effectiveYankedOrArchivedReleasesCannotBeSelectedFor")}</span></div>{selectedReleaseLifecycle?.events.map((event) => <article key={event.id}><span><ReviewStateBadge state={event.lifecycle} /><strong>{event.reason || t("sdkCatalog.noReasonRecorded")}</strong><small>{t("sdkCatalog.observed")} {t("format.dateTime", { value: new Date(event.observed_at) })} {t("sdkCatalog.recordedBy")} {event.recorded_by}</small>{event.observed_source_uri && <a href={event.observed_source_uri} target="_blank" rel="noreferrer">{t("sdkCatalog.openObservationEvidence")}</a>}</span><code>{event.id}</code></article>)}{selectedReleaseLifecycle && selectedReleaseLifecycle.events.length === 0 && <p className="empty-row">{t("sdkCatalog.noLifecycleEventsEffectiveStateComesFromTheImmutable")}</p>}{!selectedReleaseLifecycle && <p className="empty-row">{t("sdkCatalog.effectiveLifecycleHistoryIsUnavailable")}</p>}</div></details>
            <div className="developer-asset-candidate-picker"><label><span>{t("sdkCatalog.contentCandidate")}</span><select value={selectedCandidateID} onChange={(event) => { setSelectedCandidateID(event.target.value); setTab("files"); setAssetQuery(""); }}>{candidates.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.id} · {candidate.content_hash}</option>)}</select></label><span className="heading-actions"><Badge color={selectedContentPublication ? "green" : "amber"}>{selectedContentPublication ? t("sdkCatalog.publishedR", { revision: String(selectedContentPublication.revision) }) : t("sdkCatalog.reviewRequired")}</Badge><DeveloperAssetAIAdvisoryButton input={selectedContentPublication ? { prompt_key: "sdk.map_enrichment", sdk_content_publication_id: selectedContentPublication.id } : null} subject={t("sdkCatalog.contentPublicationSubject", { name: selectedPackage.name, version: selectedRelease.exact_version })} label={t("sdkCatalog.aiSDKMap")} unavailableReason={t("sdkCatalog.publishExactContentBeforeAI")} /></span></div>
            {candidateRecord ? <>
              <div className="developer-asset-inspector-tabs"><SegmentedControl label={t("sdkCatalog.sdkContentReview")} value={tab} onChange={setTab} items={[{ id: "files", label: t("common.files"), count: candidateRecord.files.length }, { id: "symbols", label: t("common.symbols"), count: candidateRecord.symbols.length }, { id: "samples", label: t("common.samples"), count: candidateRecord.samples.length }, { id: "map", label: t("common.map") }, { id: "diagnostics", label: t("common.diagnostics") }, { id: "used-by", label: t("common.usedByApis"), count: usedBy.length }]} /></div>
              {(tab === "files" || tab === "symbols" || tab === "samples") && <div className="developer-sdk-local-filter"><div className="search-field"><Search /><input type="search" aria-label={t("sdkCatalog.filterSDK", { tab: String(tab) })} placeholder={t("sdkCatalog.filterPathTitleLanguageRoleOrHash")} value={assetQuery} onChange={(event) => setAssetQuery(event.target.value)} /></div><span>{t("sdkCatalog.filteredAssets", { shown: filteredAssetCount, count: totalAssetCount })}</span>{assetQuery && <Button outline onClick={() => setAssetQuery("")}>{t("sdkCatalog.clear")}</Button>}</div>}
              <div className="developer-asset-inspector-body">
                {tab === "files" && <div className="developer-asset-file-list">{filteredFiles.map((file, index) => <article key={recordID(file, `file-${index}`)}><header><span><FileCode2 /><strong>{recordTitle(file, t("sdkCatalog.fileNumber", { number: index + 1 }))}</strong></span><Badge>{recordString(file, "role", "language") || t("sdkCatalog.file")}</Badge></header><small>{recordString(file, "source_path", "path", "content_hash")}</small><PrettyJSON value={file} /></article>)}{candidateRecord.files.length === 0 ? <p className="empty-row">{t("sdkCatalog.noFilesWereNormalized")}</p> : filteredFiles.length === 0 && <p className="empty-row">{t("sdkCatalog.noFilesMatchThisLocalFilter")}</p>}</div>}
                {tab === "symbols" && <div className="developer-asset-record-list">{filteredSymbols.map((symbol, index) => <article key={recordID(symbol, `symbol-${index}`)}><strong>{recordTitle(symbol, t("sdkCatalog.symbolNumber", { number: index + 1 }))}</strong><PrettyJSON value={symbol} /></article>)}{candidateRecord.symbols.length === 0 ? <p className="empty-row">{t("sdkCatalog.noSymbolsWereExtracted")}</p> : filteredSymbols.length === 0 && <p className="empty-row">{t("sdkCatalog.noSymbolsMatchThisLocalFilter")}</p>}</div>}
                {tab === "samples" && <div className="developer-asset-sample-list">{filteredSamples.map((sample, index) => <article key={recordID(sample, `sample-${index}`)}><header><span><Code2 /><strong>{recordTitle(sample, t("sdkCatalog.sampleNumber", { number: index + 1 }))}</strong></span><Badge color={sampleValidated(sample) ? "green" : "amber"}>{sampleValidated(sample) ? t("sdkCatalog.machineEvidencePassed") : t("sdkCatalog.reviewEvidenceRequired")}</Badge></header><PrettyJSON value={sample} /></article>)}{candidateRecord.samples.length === 0 ? <p className="empty-row">{t("sdkCatalog.noCodeSamplesWereExtracted")}</p> : filteredSamples.length === 0 && <p className="empty-row">{t("sdkCatalog.noSamplesMatchThisLocalFilter")}</p>}</div>}
                {tab === "map" && <>{candidateRecord.map ? <><MarkdownEvidence label={t("sdkCatalog.sdkMapAgentMarkdown")}>{candidateRecord.map.agent_markdown}</MarkdownEvidence><PrettyJSON value={candidateRecord.map.map} label={t("sdkCatalog.sdkMapData")} /></> : <p className="empty-row">{t("sdkCatalog.noSDKMapIsStoredForThisExactCandidate")}</p>}</>}
                {tab === "diagnostics" && <PrettyJSON value={{ diagnostics: candidateRecord.candidate.diagnostics, versions: candidateRecord.candidate.versions, source_manifest: candidateRecord.candidate.source_manifest, sample_refs: candidateRecord.sample_refs }} label={t("sdkCatalog.sdkCandidateDiagnosticsAndCitations")} />}
                {tab === "used-by" && <div className="developer-asset-used-by">{usedBy.map(({ integration, binding }) => <ConsoleLink key={binding.id} path={`/integration/${encodeURIComponent(integration.id)}/documentation`} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key} {t("sdkCatalog.exactRelease2")} {binding.sdk_release_id}</small></span><Badge color={binding.state === "ready" ? "green" : "amber"}>{binding.state}</Badge></ConsoleLink>)}{usedBy.length === 0 && <p className="empty-row">{t("sdkCatalog.thisPackageIsNotAttachedToAnAPI")}</p>}</div>}
              </div>
            </> : <p className="empty-row">{t("sdkCatalog.noNormalizedContentCandidateExistsForThisExactRelease")}</p>}
          </> : <p className="empty-row">{t("sdkCatalog.createAnExactImmutableReleaseBeforeIngestingOrAttaching")}</p>}
        </> : <div className="developer-asset-inspector-empty"><Box /><strong>{t("sdkCatalog.selectAnSDKPackage")}</strong><small>{t("sdkCatalog.exactReleasesFilesSymbolsSamplesMapsReviewHistoryAnd")}</small></div>}
      </section>
    </div>}
    <SDKPackageImportDialog open={importOpen} onClose={setImportOpen} onMessage={onMessage} onImported={async (result) => { await load(); setSelectedPackageID(result.package.id); await loadReleases(result.package.id); setSelectedReleaseID(result.release.id); }} />
    <Dialog open={packageOpen} onClose={setPackageOpen} title={t("sdkCatalog.editSDKPackage")} description={t("sdkCatalog.updatePackageRootMetadataAndLifecycleExistingExactReleases")} actions={<><Button outline onClick={() => setPackageOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !editingPackage || !ecosystem.trim() || !coordinate.trim() || !packageName.trim() || !packageAcknowledged} onClick={() => void savePackage()}>{busy ? t("common.saving") : t("sdkCatalog.savePackageRoot")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("sdkCatalog.ecosystem")}</span><input value={ecosystem} disabled /></label><label className="auth-field"><span>{t("sdkCatalog.coordinate")}</span><input value={coordinate} disabled /></label></div><label className="auth-field"><span>{t("sdkCatalog.name")}</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} /></label><label className="auth-field"><span>{t("sdkCatalog.description")}</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>{t("sdkCatalog.language")}</span><input value={language} onChange={(event) => setLanguage(event.target.value)} /></label><label className="auth-field"><span>{t("sdkCatalog.visibility")}</span><select value={packageVisibility} onChange={(event) => setPackageVisibility(event.target.value as SDKPackage["visibility"])}><option value="private">{t("sdkCatalog.private")}</option><option value="public">{t("sdkCatalog.public")}</option></select></label></div><label className="auth-field"><span>{t("sdkCatalog.lifecycle")}</span><select value={packageLifecycle} onChange={(event) => setPackageLifecycle(event.target.value as SDKPackage["lifecycle"])}><option value="draft">{t("sdkCatalog.draft")}</option><option value="active">{t("sdkCatalog.active")}</option><option value="deprecated">{t("sdkCatalog.deprecated")}</option><option value="archived">{t("sdkCatalog.archived")}</option></select><small>{t("sdkCatalog.archivePreservesExactReleasesContentPublicationsAttachmentsAndAudit")}</small></label><div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("sdkCatalog.affectedAPIAttachment")}{usedBy.length === 1 ? "" : t("sdkCatalog.s")}.</strong> {t("sdkCatalog.reviewThemBeforeChangingVisibilityOrLifecycleTheirExact")}</span></div><label className="compact-check"><input type="checkbox" checked={packageAcknowledged} onChange={(event) => setPackageAcknowledged(event.target.checked)} /><span>{t("sdkCatalog.iReviewedTheAffectedAPIsAndThisPackageRoot")}</span></label></div></Dialog>
    <Dialog open={releaseOpen} onClose={setReleaseOpen} title={t("sdkCatalog.addExactReleaseTo", { value1: String(selectedPackage?.name ?? "SDK package") })} description={t("sdkCatalog.rangesAndLatestTagsAreRejectedThisReleaseIdentity")} actions={<><Button outline onClick={() => setReleaseOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !exactVersion.trim() || exactVersion.trim().toLowerCase() === "latest"} onClick={() => void createRelease()}>{busy ? t("common.creating") : t("sdkCatalog.createExactRelease")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("sdkCatalog.exactVersion")}</span><input value={exactVersion} onChange={(event) => setExactVersion(event.target.value)} placeholder="1.4.0" /></label><label className="auth-field"><span>{t("sdkCatalog.canonicalInstallCommand")}</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder={t("sdkCatalog.leaveBlankForTheServerCanonicalCommand")} /><small>{t("sdkCatalog.packageManagersUseDifferentCanonicalSyntaxEnterAVerified")}</small></label><label className="auth-field"><span>{t("sdkCatalog.resolvedSourceURL")}</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder="https://…" /></label><label className="auth-field"><span>{t("sdkCatalog.resolvedSourceRevision")}</span><input value={sourceRevision} onChange={(event) => setSourceRevision(event.target.value)} placeholder={t("sdkCatalog.commitOrImmutableTag")} /></label><div className="notice"><GitBranch /><span><strong>{usedBy.length} {t("sdkCatalog.apiAttachment")}{usedBy.length === 1 ? "" : t("sdkCatalog.s")} {t("sdkCatalog.currentlyUseThisPackage")}</strong> {t("sdkCatalog.creatingAnotherExactReleaseDoesNotChangeAnyAttachment")}</span></div></div></Dialog>
    <Dialog open={lifecycleOpen} onClose={setLifecycleOpen} title={t("sdkCatalog.recordLifecycleEventFor", { value1: String(selectedRelease?.exact_version ?? "exact release") })} description={t("sdkCatalog.appendAReviewedObservationTheImmutableReleaseIdentityAnd")} actions={<><Button outline onClick={() => setLifecycleOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !lifecycleReason.trim() || lifecycleSourceInvalid || lifecycleObservedInvalid} onClick={() => void appendLifecycleEvent()}>{busy ? t("sdkCatalog.recording") : t("sdkCatalog.recordAppendOnlyEvent")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("sdkCatalog.observedLifecycle")}</span><select value={lifecycleValue} onChange={(event) => setLifecycleValue(event.target.value as SDKRelease["lifecycle"])}><option value="active">{t("sdkCatalog.active")}</option><option value="deprecated">{t("sdkCatalog.deprecated")}</option><option value="yanked">{t("sdkCatalog.yanked")}</option><option value="archived">{t("sdkCatalog.archived")}</option></select></label><label className="auth-field"><span>{t("sdkCatalog.requiredReviewedReason")}</span><textarea maxLength={2000} value={lifecycleReason} onChange={(event) => setLifecycleReason(event.target.value)} placeholder={t("sdkCatalog.describeTheRegistryOrAdministrativeEvidenceForThisState")} /></label><label className="auth-field"><span>{t("sdkCatalog.publicHTTPSObservationURIOptional")}</span><input type="url" inputMode="url" aria-invalid={lifecycleSourceInvalid} value={lifecycleSourceURI} onChange={(event) => setLifecycleSourceURI(event.target.value)} placeholder="https://registry.example/package/version" /><small>{lifecycleSourceInvalid ? t("sdkCatalog.onlyAPublicHTTPSEvidenceURIIsAccepted") : t("sdkCatalog.secretsAndPrivateEvidenceURLsMustNotBeEntered")}</small></label><label className="auth-field"><span>{t("sdkCatalog.observedAtOptional")}</span><input type="datetime-local" max={new Date().toISOString().slice(0, 16)} aria-invalid={lifecycleObservedInvalid} value={lifecycleObservedAt} onChange={(event) => setLifecycleObservedAt(event.target.value)} /><small>{lifecycleObservedInvalid ? t("sdkCatalog.observationTimeCannotBeInTheFuture") : t("sdkCatalog.leaveBlankToUseTheServerTime")}</small></label><div className="notice"><TriangleAlert /><span><strong>{t("sdkCatalog.yankedAndArchivedBlockNewSelections")}</strong> {t("sdkCatalog.historicalAPIPublicationsRemainReadableAndNoBindingIs")}</span></div></div></Dialog>
    <Dialog open={ingestOpen} onClose={setIngestOpen} title={t("sdkCatalog.ingestContentFor", { value1: String(selectedRelease?.exact_version ?? "exact release") })} description={t("sdkCatalog.onlySuppliedBoundedUTFN8TextFilesAreNormalized")} actions={<><Button outline onClick={() => setIngestOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || filePickerBusy || ingestionSizeInvalid || (queuedFiles.length === 0 && !pendingPastedFile)} onClick={() => void ingestContent()}>{busy ? t("sdkCatalog.normalizing") : t("sdkCatalog.normalizeWithoutExecuting")}</Button></>}>
      <div className="auth-form compact-form">
        <label className="developer-local-file-picker"><Upload /><span><strong>{filePickerBusy ? t("sdkCatalog.readingTextFiles") : t("sdkCatalog.chooseMultipleTextFiles")}</strong><small>{t("sdkCatalog.relativePathsArePreservedWhenTheBrowserSuppliesThem")}</small></span><input type="file" multiple accept="text/*,.md,.mdx,.json,.yaml,.yml,.toml,.ts,.tsx,.js,.jsx,.py,.go,.rb,.rs,.java,.kt,.swift,.php,.cs,.xml,.html,.css" disabled={filePickerBusy || queuedFiles.length >= maxSDKIngestionFiles} onChange={(event) => { const input = event.currentTarget; void chooseLocalTextFiles(input.files).finally(() => { input.value = ""; }); }} /></label>
        {queuedFiles.length > 0 && <div className="developer-ingestion-file-queue"><strong>{t("sdkCatalog.queuedFiles", { count: queuedFiles.length, size: (queuedFileBytes / 1_048_576).toFixed(2) })}</strong>{queuedFiles.map((file, index) => <div key={`${file.source_path}-${index}`}><span><code>{file.source_path}</code><small>{file.language} · {t("sdkCatalog.bytes", { count: sdkTextBytes(file.content) })}</small></span><Button outline onClick={() => setQueuedFiles((current) => current.filter((_, itemIndex) => itemIndex !== index))}>{t("sdkCatalog.remove")}</Button></div>)}</div>}
        {rejectedFiles.length > 0 && <div className="developer-rejected-file-list"><header><span><TriangleAlert /><strong>{t("sdkCatalog.rejectedFiles", { count: rejectedFiles.length })}</strong></span><Button outline onClick={() => setRejectedFiles([])}>{t("sdkCatalog.dismiss")}</Button></header>{rejectedFiles.map((file, index) => <div key={`${file.path}-${index}`}><code>{file.path}</code><small>{file.reason}</small></div>)}</div>}
        <div className="developer-paste-divider"><span>{t("sdkCatalog.orPasteOneTextFile")}</span></div>
        <div className="two-fields"><label className="auth-field"><span>{t("sdkCatalog.relativeSourcePath")}</span><input maxLength={500} value={sourcePath} onChange={(event) => setSourcePath(event.target.value)} /></label><label className="auth-field"><span>{t("sdkCatalog.language")}</span><input value={fileLanguage} onChange={(event) => setFileLanguage(event.target.value)} /></label></div>
        <label className="auth-field"><span>{t("sdkCatalog.textContent")}</span><textarea className="code-input" maxLength={2_097_152} value={fileContent} onChange={(event) => setFileContent(event.target.value)} spellCheck={false} /><small>{t("format.number", { value: pastedFileBytes })} {t("sdkCatalog.bytesN1N500FilesAtMostN2MiBEach")}</small></label>
        <Button type="button" outline disabled={!pendingPastedFile || ingestionSizeInvalid || queuedFiles.length >= maxSDKIngestionFiles} onClick={queueCurrentFile}><Plus data-slot="icon" />{t("sdkCatalog.queuePastedFile")}</Button>
        {ingestionSizeInvalid && <div className="inline-warning"><TriangleAlert />{t("sdkCatalog.thePendingTextExceedsTheN2MiBFileN20")}</div>}
        <div className="notice"><ShieldCheck /><span><strong>{t("sdkCatalog.noCodeExecution")}</strong> {t("sdkCatalog.staticNormalizationAndValidationProduceACandidateThatRemains")}</span></div>
      </div>
    </Dialog>
    <Dialog open={reviewOpen} onClose={setReviewOpen} title={t("sdkCatalog.reviewSDKContentCandidate")} description={t("sdkCatalog.decideEveryFileAndSampleApprovalRequiresPositiveMachine")} actions={<><Button outline onClick={() => setReviewOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !reviewComplete || !acknowledged} onClick={() => void publishCandidate()}>{busy ? t("sdkCatalog.publishing") : t("sdkCatalog.publishReviewedContent")}</Button></>}><div className="developer-asset-sdk-review"><section><h3>{t("sdkCatalog.files")}</h3>{candidateRecord && reviewRows(candidateRecord.files, "file")}</section><section><h3>{t("sdkCatalog.samples")}</h3>{candidateRecord && reviewRows(candidateRecord.samples, "sample")}</section><label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("sdkCatalog.iReviewedEveryDecisionValidationResultCitationAndSDK")}</span></label></div></Dialog>
  </>;
}
