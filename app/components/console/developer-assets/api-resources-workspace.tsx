"use client";


import { useTranslation } from "react-i18next";
import { BookOpen, Box, ExternalLink, FileCode2, Link2, Plus, RefreshCw, Unlink } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { APIIntegration } from "../../../lib/api";
import { sectionPath } from "../../../lib/console-routes";
import {
  developerAssetsApi,
  type APIDeveloperAssetPublication,
  type APIContractBinding,
  type APIDocumentationBinding,
  type APIResourceBindings,
  type APISDKBinding,
  type DeveloperAssetCatalog,
  type DocumentationCollectionRevision,
  type APIContractRevision,
  type SDKContentPublication,
  type SDKRelease,
} from "../../../lib/developer-assets-api";
import { Badge, Button, Dialog } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { ConsoleLink } from "../console-link";
import { DeveloperAssetAIAdvisoryButton } from "./developer-asset-ai-advisory";
import { developerAssetError, LoadingPanel, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";
import { SDKPackageImportDialog } from "./sdk-package-import-dialog";

type ResourceKind = "documentation" | "contract" | "sdk";
type ResourceBinding = APIDocumentationBinding | APIContractBinding | APISDKBinding;

const emptyBindings: APIResourceBindings = { documentation: [], contracts: [], sdks: [] };
const emptyCatalog: DeveloperAssetCatalog = { documentation: [], contracts: [], sdk_packages: [] };

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

export function APIResourcesWorkspace({ integration, live, onMessage, onNavigate }: { integration: APIIntegration; live: boolean; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [bindings, setBindings] = useState<APIResourceBindings>(emptyBindings);
  const [catalog, setCatalog] = useState<DeveloperAssetCatalog>(emptyCatalog);
  const [documentationRevisions, setDocumentationRevisions] = useState<Record<string, DocumentationCollectionRevision[]>>({});
  const [contractRevisions, setContractRevisions] = useState<Record<string, APIContractRevision[]>>({});
  const [sdkReleases, setSDKReleases] = useState<Record<string, SDKRelease[]>>({});
  const [contentPublications, setContentPublications] = useState<SDKContentPublication[]>([]);
  const [resourcePublications, setResourcePublications] = useState<APIDeveloperAssetPublication[]>([]);
  const [loading, setLoading] = useState(live);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [attachOpen, setAttachOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [sdkImportOpen, setSDKImportOpen] = useState(false);
  const [detachTarget, setDetachTarget] = useState<{ kind: ResourceKind; binding: ResourceBinding; label: string } | null>(null);
  const [kind, setKind] = useState<ResourceKind>("documentation");
  const [editing, setEditing] = useState<ResourceBinding | null>(null);
  const [assetID, setAssetID] = useState("");
  const [exactID, setExactID] = useState("");
  const [contentPublicationID, setContentPublicationID] = useState("");
  const [primary, setPrimary] = useState(false);
  const [coverage, setCoverage] = useState<APISDKBinding["coverage"]>("unknown");
  const [assurance, setAssurance] = useState<APISDKBinding["assurance"]>("related");
  const [resourceName, setResourceName] = useState("");
  const [resourceSlug, setResourceSlug] = useState("");
  const [memberID, setMemberID] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);

  const load = useCallback(async () => {
    if (!live) return;
    setLoading(true);
    setProblem("");
    try {
      const [resourceValues, catalogValues, publicationValues] = await Promise.all([developerAssetsApi.apiResources(integration.id), developerAssetsApi.catalog(), developerAssetsApi.apiResourcePublications(integration.id)]);
      setBindings({
        documentation: resourceValues.documentation.filter((item) => item.lifecycle === "attached"),
        contracts: resourceValues.contracts.filter((item) => item.lifecycle === "attached"),
        sdks: resourceValues.sdks.filter((item) => item.state !== "detached"),
      });
      setCatalog(catalogValues);
      setResourcePublications([...publicationValues].sort((left, right) => Date.parse(right.published_at) - Date.parse(left.published_at)));
      const documentationIDs = [...new Set(resourceValues.documentation.map((item) => item.documentation_collection_id))];
      const contractIDs = [...new Set(resourceValues.contracts.map((item) => item.api_contract_id))];
      const packageIDs = [...new Set(resourceValues.sdks.map((item) => item.sdk_package_id))];
      const [documentationEntries, contractEntries, releaseEntries] = await Promise.all([
        Promise.all(documentationIDs.map(async (id) => [id, await developerAssetsApi.documentationCollectionRevisions(id)] as const)),
        Promise.all(contractIDs.map(async (id) => [id, await developerAssetsApi.apiContractRevisions(id)] as const)),
        Promise.all(packageIDs.map(async (id) => [id, await developerAssetsApi.sdkReleases(id)] as const)),
      ]);
      setDocumentationRevisions(Object.fromEntries(documentationEntries));
      setContractRevisions(Object.fromEntries(contractEntries));
      setSDKReleases(Object.fromEntries(releaseEntries));
    } catch (error) {
      setProblem(developerAssetError(error, t("apiResources.apiResourceAttachmentsCouldNotBeLoaded")));
    } finally {
      setLoading(false);
    }
  }, [integration.id, live, t]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void load(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  const availableAssets = (kind === "documentation" ? catalog.documentation : kind === "contract" ? catalog.contracts : catalog.sdk_packages).filter((asset) => asset.lifecycle !== "archived" || asset.id === assetID);
  const exactOptions = kind === "documentation" ? documentationRevisions[assetID] ?? [] : kind === "contract" ? contractRevisions[assetID] ?? [] : (sdkReleases[assetID] ?? []).filter((release) => (release.lifecycle !== "archived" && release.lifecycle !== "yanked") || release.id === exactID);
  const selectedSDKRelease = kind === "sdk" ? (sdkReleases[assetID] ?? []).find((release) => release.id === exactID) : undefined;

  useEffect(() => {
    let cancelled = false;
    if (!live || kind !== "sdk" || !exactID) {
      queueMicrotask(() => { if (!cancelled) setContentPublications([]); });
      return () => { cancelled = true; };
    }
    developerAssetsApi.sdkContentPublications(exactID).then((values) => { if (!cancelled) setContentPublications(values); }).catch(() => { if (!cancelled) setContentPublications([]); });
    return () => { cancelled = true; };
  }, [exactID, kind, live]);

  async function ensureOptions(nextKind: ResourceKind, nextAssetID: string) {
    if (!nextAssetID) return [];
    if (nextKind === "documentation") {
      const values = documentationRevisions[nextAssetID] ?? await developerAssetsApi.documentationCollectionRevisions(nextAssetID);
      setDocumentationRevisions((current) => ({ ...current, [nextAssetID]: values }));
      return values;
    }
    if (nextKind === "contract") {
      const values = contractRevisions[nextAssetID] ?? await developerAssetsApi.apiContractRevisions(nextAssetID);
      setContractRevisions((current) => ({ ...current, [nextAssetID]: values }));
      return values;
    }
    const values = sdkReleases[nextAssetID] ?? await developerAssetsApi.sdkReleases(nextAssetID);
    setSDKReleases((current) => ({ ...current, [nextAssetID]: values }));
    return values;
  }

  async function chooseAsset(nextAssetID: string) {
    setAssetID(nextAssetID);
    setExactID("");
    setContentPublicationID("");
    if (!nextAssetID) return;
    try {
      const values = await ensureOptions(kind, nextAssetID);
      setExactID(values[0]?.id ?? "");
    } catch (error) {
      onMessage(developerAssetError(error, t("apiResources.exactRevisionsOrReleasesCouldNotBeLoaded")));
    }
  }

  async function openAttach(nextKind: ResourceKind, binding?: ResourceBinding) {
    setKind(nextKind);
    setEditing(binding ?? null);
    setPrimary(nextKind === "contract" && binding ? (binding as APIContractBinding).primary : false);
    setCoverage(nextKind === "sdk" && binding ? (binding as APISDKBinding).coverage : "unknown");
    setAssurance(nextKind === "sdk" && binding ? (binding as APISDKBinding).assurance : "related");
    const nextAssetID = binding
      ? nextKind === "documentation" ? (binding as APIDocumentationBinding).documentation_collection_id : nextKind === "contract" ? (binding as APIContractBinding).api_contract_id : (binding as APISDKBinding).sdk_package_id
      : nextKind === "documentation" ? catalog.documentation[0]?.id ?? "" : nextKind === "contract" ? catalog.contracts[0]?.id ?? "" : catalog.sdk_packages[0]?.id ?? "";
    setAssetID(nextAssetID);
    setExactID(binding ? nextKind === "sdk" ? (binding as APISDKBinding).sdk_release_id : (binding as APIDocumentationBinding | APIContractBinding).pinned_revision_id ?? "" : "");
    setContentPublicationID(nextKind === "sdk" && binding ? (binding as APISDKBinding).sdk_content_publication_id ?? "" : "");
    setAttachOpen(true);
    if (nextAssetID) {
      try {
        const values = await ensureOptions(nextKind, nextAssetID);
        if (!binding) setExactID(values[0]?.id ?? "");
      } catch (error) {
        onMessage(developerAssetError(error, t("apiResources.exactRevisionsOrReleasesCouldNotBeLoaded")));
      }
    }
  }

  function openCreate(nextKind: ResourceKind) {
    setKind(nextKind);
    if (nextKind === "sdk") {
      setSDKImportOpen(true);
      return;
    }
    setResourceName("");
    setResourceSlug("");
    setMemberID("");
    setAcknowledged(false);
    setCreateOpen(true);
  }

  async function saveAttachment() {
    if (!assetID || !exactID) return;
    setBusy(true);
    try {
      if (kind === "documentation") {
        if (editing) await developerAssetsApi.changeAPIDocumentation(integration.id, editing.id, { documentation_collection_id: assetID, pinned_revision_id: exactID, selector: {}, visibility: editing.visibility, revision: editing.revision });
        else await developerAssetsApi.attachAPIDocumentation(integration.id, { documentation_collection_id: assetID, pinned_revision_id: exactID, selector: {}, visibility: "private" });
      } else if (kind === "contract") {
        if (editing) await developerAssetsApi.changeAPIContract(integration.id, editing.id, { api_contract_id: assetID, pinned_revision_id: exactID, primary, visibility: editing.visibility, revision: editing.revision });
        else await developerAssetsApi.attachAPIContract(integration.id, { api_contract_id: assetID, pinned_revision_id: exactID, primary, visibility: "private" });
      } else {
        const current = editing as APISDKBinding | null;
        const input = {
          sdk_package_id: assetID,
          sdk_release_id: exactID,
          ...(contentPublicationID ? { sdk_content_publication_id: contentPublicationID } : {}),
          state: contentPublicationID ? "ready" as const : "draft" as const,
          coverage,
          assurance,
          applicable_modules: current?.applicable_modules ?? [],
          applicable_capabilities: current?.applicable_capabilities ?? [],
          applicable_operation_keys: current?.applicable_operation_keys ?? [],
          selector: current?.selector ?? {},
          visibility: current?.visibility ?? "private" as const,
        };
        if (current) await developerAssetsApi.changeAPISDK(integration.id, current.id, { ...input, revision: current.revision });
        else await developerAssetsApi.attachAPISDK(integration.id, input);
      }
      setAttachOpen(false);
      await load();
      onMessage(editing ? t("apiResources.exactResourceAttachmentChangedByExplicitRevisionedAction") : t("apiResources.exactDeploymentOwnedResourceAttachedToThisAPI"));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiResources.resourceAttachmentCouldNotBeSaved")));
    } finally { setBusy(false); }
  }

  async function createResource() {
    if (kind === "sdk" || !resourceName.trim() || !acknowledged) return;
    setBusy(true);
    try {
      if (kind === "documentation") {
        if (!resourceSlug.trim() || !memberID.trim()) return;
        const collection = await developerAssetsApi.createDocumentationCollection({ name: resourceName.trim(), slug: resourceSlug.trim(), description: t("apiResources.createdDocumentationDescription"), visibility: "private", lifecycle: "active", members: [{ kind: "source_publication", id: memberID.trim(), include_descendants: true, selector: {} }], acknowledge_reviewed: true });
        const revisions = await developerAssetsApi.documentationCollectionRevisions(collection.id);
        const exact = [...revisions].sort((left, right) => right.revision - left.revision)[0];
        if (!exact) throw new Error(t("apiResources.theReviewedCollectionRevisionWasNotReturned"));
        await developerAssetsApi.attachAPIDocumentation(integration.id, { documentation_collection_id: collection.id, pinned_revision_id: exact.id, selector: {}, visibility: "private" });
        onMessage(t("apiResources.reviewedCollectionCreatedAndItsExactFirstRevisionAttached"));
      } else {
        if (!resourceSlug.trim()) return;
        await developerAssetsApi.createAPIContract({ name: resourceName.trim(), slug: resourceSlug.trim(), description: t("apiResources.createdContractDescription"), visibility: "private", lifecycle: "active" });
        setCreateOpen(false);
        onMessage(t("apiResources.contractRootCreatedInCatalogNextAttachAnOpenAPI"));
        onNavigate(sectionPath("contracts"));
        return;
      }
      setCreateOpen(false);
      await load();
    } catch (error) {
      onMessage(developerAssetError(error, kind === "contract" ? t("apiResources.apiContractCouldNotBeCreatedInCatalog") : t("apiResources.resourceCouldNotBeCreatedAndAttached")));
    } finally { setBusy(false); }
  }

  async function detach() {
    if (!detachTarget) return;
    setBusy(true);
    try {
      if (detachTarget.kind === "documentation") await developerAssetsApi.detachAPIDocumentation(integration.id, detachTarget.binding.id, detachTarget.binding.revision);
      else if (detachTarget.kind === "contract") await developerAssetsApi.detachAPIContract(integration.id, detachTarget.binding.id, detachTarget.binding.revision);
      else await developerAssetsApi.detachAPISDK(integration.id, detachTarget.binding.id, detachTarget.binding.revision);
      setDetachTarget(null);
      await load();
      onMessage(t("apiResources.resourceDetachedTheBindingRemainsInAuditHistory"));
    } catch (error) {
      onMessage(developerAssetError(error, t("apiResources.resourceCouldNotBeDetached")));
    } finally { setBusy(false); }
  }

  const labels = useMemo(() => ({
    documentation: new Map(catalog.documentation.map((item) => [item.id, item.name])),
    contracts: new Map(catalog.contracts.map((item) => [item.id, item.name])),
    sdks: new Map(catalog.sdk_packages.map((item) => [item.id, item.name])),
  }), [catalog]);

  function resourcePanel(panelKind: ResourceKind, title: string, description: string, rows: ResourceBinding[]) {
    const catalogSection = panelKind === "documentation" ? "documents" : panelKind === "contract" ? "contracts" : "sdks";
    return <section className="panel api-resource-panel"><PanelHeader title={title} description={description} action={<span className="heading-actions"><ConsoleLink path={sectionPath(catalogSection)} onNavigate={onNavigate} className="entity-back-link"><ExternalLink />{t("apiResources.openCatalog")}</ConsoleLink><Button outline onClick={() => openCreate(panelKind)}><Plus data-slot="icon" />{panelKind === "contract" ? t("apiResources.createInCatalog") : t("apiResources.createAttach")}</Button><Button onClick={() => void openAttach(panelKind)}><Link2 data-slot="icon" />{t("apiResources.attachExisting")}</Button></span>} />
      <div className="api-resource-list">{rows.map((binding) => {
        const label = panelKind === "documentation" ? labels.documentation.get((binding as APIDocumentationBinding).documentation_collection_id) : panelKind === "contract" ? labels.contracts.get((binding as APIContractBinding).api_contract_id) : labels.sdks.get((binding as APISDKBinding).sdk_package_id);
        const exact = panelKind === "sdk"
          ? sdkReleases[(binding as APISDKBinding).sdk_package_id]?.find((release) => release.id === (binding as APISDKBinding).sdk_release_id)?.exact_version ?? (binding as APISDKBinding).sdk_release_id
          : (binding as APIDocumentationBinding | APIContractBinding).pinned_revision_id ?? t("apiResources.unpinned");
        const sdkBinding = panelKind === "sdk" ? binding as APISDKBinding : null;
        const advisoryPublication = sdkBinding?.sdk_content_publication_id ? resourcePublications.find((publication) => publication.sdks.some((asset) => asset.binding_id === sdkBinding.id && asset.sdk_content_publication_id === sdkBinding.sdk_content_publication_id)) : undefined;
        const advisoryInput = sdkBinding?.sdk_content_publication_id && advisoryPublication ? {
          prompt_key: "sdk.applicability_suggestion" as const,
          api_id: integration.id,
          api_developer_asset_publication_id: advisoryPublication.id,
          api_sdk_binding_id: sdkBinding.id,
          sdk_content_publication_id: sdkBinding.sdk_content_publication_id,
        } : null;
        return <div className="api-resource-row" key={binding.id}><span className="settings-icon">{panelKind === "documentation" ? <BookOpen /> : panelKind === "contract" ? <FileCode2 /> : <Box />}</span><span><strong>{label ?? t("apiResources.catalogAssetUnavailable")}</strong><small>{panelKind === "sdk" ? t("apiResources.exactVersion2", { exact: String(exact) }) : t("apiResources.exactRevision", { exact: String(exact) })}</small><code>{binding.id}</code></span><span className="tool-badges">{panelKind === "sdk" && <ReviewStateBadge state={(binding as APISDKBinding).state} />}<Badge color="zinc">{binding.visibility}</Badge></span><span className="table-actions">{panelKind === "sdk" && <DeveloperAssetAIAdvisoryButton input={advisoryInput} subject={t("apiResources.sdkApplicabilitySubject", { name: label ?? t("apiResources.sdk") })} label={t("apiResources.aiApplicability")} unavailableReason={t("apiResources.publishSnapshotBeforeApplicabilityAI")} />}<Button outline onClick={() => void openAttach(panelKind, binding)}><RefreshCw data-slot="icon" />{t("apiResources.changeExact")} {panelKind === "sdk" ? t("apiResources.version") : t("apiResources.revision")}</Button><Button outline onClick={() => setDetachTarget({ kind: panelKind, binding, label: label ?? binding.id })}><Unlink data-slot="icon" />{t("apiResources.detach")}</Button></span></div>;
      })}{rows.length === 0 && <div className="empty-row">{panelKind === "contract" ? t("apiResources.noAPIContractIsAttachedChooseAReviewedCatalog") : panelKind === "sdk" ? t("apiResources.noSDKReleasesAttached") : t("apiResources.noDocumentationAttached")}</div>}</div>
    </section>;
  }

  if (loading) return <LoadingPanel label={t("apiResources.loadingAPIResourceAttachments")} />;
  if (problem) return <ProblemPanel message={problem} onRetry={() => void load()} />;

  return <>
    <section className="panel api-resource-publications"><PanelHeader title={t("apiResources.publishedResourceHistory")} description={t("apiResources.everyAPIPublicationFreezesTheExactDeveloperAssetSnapshot")} />{resourcePublications[0] ? <div className="developer-global-active"><span><Badge color="green">{t("apiResources.latestPublication")}</Badge><code>{resourcePublications[0].id}</code><small>{t("apiResources.apiRevision")} {resourcePublications[0].api_revision_id}</small></span><span><code>{resourcePublications[0].snapshot_hash}</code><small>{resourcePublications[0].documentation.length} {t("apiResources.documentation")} {resourcePublications[0].contracts.length} {t("apiResources.contracts")} {resourcePublications[0].sdks.length} {t("apiResources.sdks")}</small></span></div> : <p className="empty-row">{t("apiResources.noImmutableResourceSnapshotHasBeenPublishedForThis")}</p>}<details className="advanced-details"><summary>{t("apiResources.exactPublicationIDsAndHashes")}</summary><div className="developer-asset-publication-history">{resourcePublications.map((publication) => <div key={publication.id}><span><strong>{t("format.dateTime", { value: new Date(publication.published_at) })}</strong><code>{publication.id}</code></span><span><code>{publication.snapshot_hash}</code><small>{publication.snapshot_schema_version}</small></span></div>)}{resourcePublications.length === 0 && <small>{t("apiResources.noPublicationHistory")}</small>}</div></details></section>
    {resourcePanel("documentation", t("apiResources.documentationTitle"), t("apiResources.documentationDescription"), bindings.documentation)}
    {resourcePanel("contract", t("apiResources.apiContractsTitle"), t("apiResources.apiContractsDescription"), bindings.contracts)}
    {resourcePanel("sdk", t("apiResources.sdkReleasesTitle"), t("apiResources.sdkReleasesDescription"), bindings.sdks)}
    <SDKPackageImportDialog
      open={sdkImportOpen}
      onClose={setSDKImportOpen}
      onMessage={() => undefined}
      onImported={async (result) => {
        await developerAssetsApi.attachAPISDK(integration.id, {
          sdk_package_id: result.package.id,
          sdk_release_id: result.release.id,
          state: "draft",
          coverage: "unknown",
          assurance: "related",
          applicable_modules: [],
          applicable_capabilities: [],
          applicable_operation_keys: [],
          selector: {},
          visibility: "private",
        });
        await load();
        onMessage(t("apiResources.sdkImportedAndAttachedAsDraft", { exact_version: String(result.release.exact_version) }));
      }}
    />
    <Dialog open={attachOpen} onClose={setAttachOpen} title={kind === "documentation" ? editing ? t("apiResources.changeDocumentation") : t("apiResources.attachDocumentation") : kind === "contract" ? editing ? t("apiResources.changeAPIContract") : t("apiResources.attachAPIContract") : editing ? t("apiResources.changeSDKRelease") : t("apiResources.attachSDKRelease")} description={kind === "sdk" ? t("apiResources.selectExactReviewedVersion") : t("apiResources.selectExactReviewedRevision")} actions={<><Button outline onClick={() => setAttachOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !assetID || !exactID} onClick={() => void saveAttachment()}>{busy ? t("common.saving") : editing ? kind === "sdk" ? t("apiResources.changeExactVersion") : t("apiResources.changeExactRevision") : t("apiResources.attachExactResource")}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>{t("apiResources.catalog")} {kind === "sdk" ? t("apiResources.package") : kind === "contract" ? t("apiResources.contract") : t("apiResources.collection")}</span><select disabled={Boolean(editing)} value={assetID} onChange={(event) => void chooseAsset(event.target.value)}><option value="">{t("apiResources.selectFromCatalog")}</option>{availableAssets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name}</option>)}</select></label><label className="auth-field"><span>{t("apiResources.exact")} {kind === "sdk" ? t("apiResources.release") : t("apiResources.reviewedRevision")}</span><select value={exactID} onChange={(event) => { setExactID(event.target.value); setContentPublicationID(""); }}><option value="">{t("apiResources.selectExact")} {kind === "sdk" ? t("apiResources.release") : t("apiResources.revision")}</option>{exactOptions.map((option) => <option key={option.id} value={option.id}>{kind === "sdk" ? (option as SDKRelease).exact_version : t("apiResources.r", { revision: String((option as DocumentationCollectionRevision | APIContractRevision).revision) })} · {option.id}</option>)}</select>{kind === "sdk" && selectedSDKRelease && <small><code>{selectedSDKRelease.release_hash}</code></small>}</label>{kind === "contract" && <label className="compact-check"><input type="checkbox" checked={primary} onChange={(event) => setPrimary(event.target.checked)} /><span>{t("apiResources.useAsThisAPISPrimaryContract")}</span></label>}{kind === "sdk" && <><label className="auth-field"><span>{t("apiResources.reviewedContentPublication")}</span><select value={contentPublicationID} onChange={(event) => setContentPublicationID(event.target.value)}><option value="">{t("apiResources.noneKeepAttachmentInDraft")}</option>{contentPublications.map((publication) => <option key={publication.id} value={publication.id}>r{publication.revision} · {publication.content_hash}</option>)}</select></label><div className="two-fields"><label className="auth-field"><span>{t("apiResources.coverage")}</span><select value={coverage} onChange={(event) => setCoverage(event.target.value as APISDKBinding["coverage"])}><option value="unknown">{t("apiResources.unknown")}</option><option value="partial">{t("apiResources.partial")}</option><option value="full">{t("apiResources.full")}</option></select></label><label className="auth-field"><span>{t("apiResources.assurance")}</span><select value={assurance} onChange={(event) => setAssurance(event.target.value as APISDKBinding["assurance"])}><option value="related">{t("apiResources.related")}</option><option value="documented">{t("apiResources.documented")}</option><option value="reviewed">{t("apiResources.reviewed")}</option><option value="tested">{t("apiResources.tested")}</option><option value="verified">{t("apiResources.verified")}</option></select></label></div></>}</div>
    </Dialog>
    <Dialog open={createOpen} onClose={setCreateOpen} title={kind === "documentation" ? t("apiResources.createDocumentationCollection") : t("apiResources.createAPIContractInCatalog")} description={kind === "contract" ? t("apiResources.thisCreatesOnlyTheReusableContractRootItDoes") : t("apiResources.createDocumentationDescription")} actions={<><Button outline onClick={() => setCreateOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !resourceName.trim() || !acknowledged || (kind === "documentation" && (!resourceSlug.trim() || !memberID.trim())) || (kind === "contract" && !resourceSlug.trim())} onClick={() => void createResource()}>{busy ? t("common.creating") : kind === "contract" ? t("apiResources.createInCatalog") : t("apiResources.createAttachExactResource")}</Button></>}>
      <div className="auth-form compact-form">{kind === "contract" && <div className="notice"><FileCode2 /><span><strong>{t("apiResources.nextStepsHappenInCatalog")}</strong> {t("apiResources.attachAnOpenAPISourceIngestAndValidateTheNormalized")}</span></div>}<label className="auth-field"><span>{t("apiResources.name")}</span><input value={resourceName} onChange={(event) => { setResourceName(event.target.value); setResourceSlug(slugify(event.target.value)); }} /></label><label className="auth-field"><span>{t("apiResources.slug")}</span><input value={resourceSlug} onChange={(event) => setResourceSlug(slugify(event.target.value))} /></label>{kind === "documentation" && <label className="auth-field"><span>{t("apiResources.exactReviewedSourcePublicationID")}</span><input value={memberID} onChange={(event) => setMemberID(event.target.value)} /><small>{t("apiResources.thisCreatesAnImmutableCollectionRevisionFromReviewedEvidence")}</small></label>}<label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{kind === "contract" ? t("apiResources.iUnderstandCreationDoesNotApprovePublishOrAttach") : t("apiResources.iReviewedTheExactIdentityAndUnderstandFutureCatalog")}</span></label></div>
    </Dialog>
    <Dialog open={Boolean(detachTarget)} onClose={(open) => { if (!open) setDetachTarget(null); }} title={t("apiResources.detach2", { value1: String(detachTarget?.label ?? "resource") })} description={t("apiResources.thisRemovesTheResourceFromFutureAPIPublicationsThe")} actions={<><Button outline onClick={() => setDetachTarget(null)}>{t("common.cancel")}</Button><Button color="red" disabled={busy} onClick={() => void detach()}>{busy ? t("apiResources.detaching") : t("apiResources.detachResource")}</Button></>}><div className="notice"><Unlink /><span><strong>{t("apiResources.noCatalogContentWillBeDeleted")}</strong> {t("apiResources.thisActionAffectsOnlyTheExplicitAttachmentTo")} {integration.display_name}.</span></div></Dialog>
  </>;
}
