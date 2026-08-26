"use client";

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
import { developerAssetError, ExactVersionNotice, LoadingPanel, ProblemPanel, ReviewStateBadge } from "./developer-asset-ui";

type ResourceKind = "documentation" | "contract" | "sdk";
type ResourceBinding = APIDocumentationBinding | APIContractBinding | APISDKBinding;

const emptyBindings: APIResourceBindings = { documentation: [], contracts: [], sdks: [] };
const emptyCatalog: DeveloperAssetCatalog = { documentation: [], contracts: [], sdk_packages: [] };

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

export function APIResourcesWorkspace({ integration, live, onMessage, onNavigate }: { integration: APIIntegration; live: boolean; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
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
  const [ecosystem, setEcosystem] = useState("npm");
  const [coordinate, setCoordinate] = useState("");
  const [exactVersion, setExactVersion] = useState("");
  const [installCommand, setInstallCommand] = useState("");
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
      setProblem(developerAssetError(error, "API resource attachments could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [integration.id, live]);

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
      onMessage(developerAssetError(error, "Exact revisions or releases could not be loaded."));
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
        onMessage(developerAssetError(error, "Exact revisions or releases could not be loaded."));
      }
    }
  }

  function openCreate(nextKind: ResourceKind) {
    setKind(nextKind);
    setResourceName("");
    setResourceSlug("");
    setMemberID("");
    setEcosystem("npm");
    setCoordinate("");
    setExactVersion("");
    setInstallCommand("");
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
      onMessage(editing ? "Exact resource attachment changed by explicit revisioned action." : "Exact deployment-owned resource attached to this API.");
    } catch (error) {
      onMessage(developerAssetError(error, "Resource attachment could not be saved."));
    } finally { setBusy(false); }
  }

  async function createResource() {
    if (!resourceName.trim() || !acknowledged) return;
    setBusy(true);
    try {
      if (kind === "documentation") {
        if (!resourceSlug.trim() || !memberID.trim()) return;
        const collection = await developerAssetsApi.createDocumentationCollection({ name: resourceName.trim(), slug: resourceSlug.trim(), description: "Created from API Resources for explicit attachment.", visibility: "private", lifecycle: "active", members: [{ kind: "source_publication", id: memberID.trim(), include_descendants: true, selector: {} }], acknowledge_reviewed: true });
        const revisions = await developerAssetsApi.documentationCollectionRevisions(collection.id);
        const exact = [...revisions].sort((left, right) => right.revision - left.revision)[0];
        if (!exact) throw new Error("The reviewed collection revision was not returned.");
        await developerAssetsApi.attachAPIDocumentation(integration.id, { documentation_collection_id: collection.id, pinned_revision_id: exact.id, selector: {}, visibility: "private" });
        onMessage("Reviewed collection created and its exact first revision attached.");
      } else if (kind === "sdk") {
        if (!coordinate.trim() || !exactVersion.trim() || exactVersion.trim().toLowerCase() === "latest") return;
        const sdkPackage = await developerAssetsApi.createSDKPackage({ ecosystem: ecosystem.trim(), coordinate: coordinate.trim(), name: resourceName.trim(), visibility: "private", lifecycle: "draft" });
        const release = await developerAssetsApi.createSDKRelease(sdkPackage.id, {
          exact_version: exactVersion.trim(),
          ...(installCommand.trim() ? { install_command: installCommand.trim() } : {}),
          identity_assurance: "metadata_only",
          visibility: "private",
          lifecycle: "active",
        });
        await developerAssetsApi.attachAPISDK(integration.id, { sdk_package_id: sdkPackage.id, sdk_release_id: release.id, state: "draft", coverage: "unknown", assurance: "related", applicable_modules: [], applicable_capabilities: [], applicable_operation_keys: [], selector: {}, visibility: "private" });
        onMessage(`SDK package and exact release ${release.exact_version} created and attached as a draft. Review content before marking it ready.`);
      } else {
        if (!resourceSlug.trim()) return;
        await developerAssetsApi.createAPIContract({ name: resourceName.trim(), slug: resourceSlug.trim(), description: "Created from API Resources.", visibility: "private", lifecycle: "active" });
        setCreateOpen(false);
        onMessage("Contract root created in Catalog. Next: attach an OpenAPI source, ingest it, review and publish the validated candidate, then return to this API’s Resources tab and attach that exact revision.");
        onNavigate(sectionPath("contracts"));
        return;
      }
      setCreateOpen(false);
      await load();
    } catch (error) {
      onMessage(developerAssetError(error, kind === "contract" ? "API contract could not be created in Catalog." : "Resource could not be created and attached."));
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
      onMessage("Resource detached. The binding remains in audit history.");
    } catch (error) {
      onMessage(developerAssetError(error, "Resource could not be detached."));
    } finally { setBusy(false); }
  }

  const labels = useMemo(() => ({
    documentation: new Map(catalog.documentation.map((item) => [item.id, item.name])),
    contracts: new Map(catalog.contracts.map((item) => [item.id, item.name])),
    sdks: new Map(catalog.sdk_packages.map((item) => [item.id, item.name])),
  }), [catalog]);

  function resourcePanel(panelKind: ResourceKind, title: string, description: string, rows: ResourceBinding[]) {
    const catalogSection = panelKind === "documentation" ? "collections" : panelKind === "contract" ? "contracts" : "sdks";
    return <section className="panel api-resource-panel"><PanelHeader title={title} description={description} action={<span className="heading-actions"><ConsoleLink path={sectionPath(catalogSection)} onNavigate={onNavigate} className="entity-back-link"><ExternalLink />Open catalog</ConsoleLink><Button outline onClick={() => openCreate(panelKind)}><Plus data-slot="icon" />{panelKind === "contract" ? "Create in Catalog" : "Create & attach"}</Button><Button onClick={() => void openAttach(panelKind)}><Link2 data-slot="icon" />Attach existing</Button></span>} />
      <div className="api-resource-list">{rows.map((binding) => {
        const label = panelKind === "documentation" ? labels.documentation.get((binding as APIDocumentationBinding).documentation_collection_id) : panelKind === "contract" ? labels.contracts.get((binding as APIContractBinding).api_contract_id) : labels.sdks.get((binding as APISDKBinding).sdk_package_id);
        const exact = panelKind === "sdk"
          ? sdkReleases[(binding as APISDKBinding).sdk_package_id]?.find((release) => release.id === (binding as APISDKBinding).sdk_release_id)?.exact_version ?? (binding as APISDKBinding).sdk_release_id
          : (binding as APIDocumentationBinding | APIContractBinding).pinned_revision_id ?? "Unpinned";
        const sdkBinding = panelKind === "sdk" ? binding as APISDKBinding : null;
        const advisoryPublication = sdkBinding?.sdk_content_publication_id ? resourcePublications.find((publication) => publication.sdks.some((asset) => asset.binding_id === sdkBinding.id && asset.sdk_content_publication_id === sdkBinding.sdk_content_publication_id)) : undefined;
        const advisoryInput = sdkBinding?.sdk_content_publication_id && advisoryPublication ? {
          prompt_key: "sdk.applicability_suggestion" as const,
          api_id: integration.id,
          api_developer_asset_publication_id: advisoryPublication.id,
          api_sdk_binding_id: sdkBinding.id,
          sdk_content_publication_id: sdkBinding.sdk_content_publication_id,
        } : null;
        return <div className="api-resource-row" key={binding.id}><span className="settings-icon">{panelKind === "documentation" ? <BookOpen /> : panelKind === "contract" ? <FileCode2 /> : <Box />}</span><span><strong>{label ?? "Catalog asset unavailable"}</strong><small>{panelKind === "sdk" ? `Exact version ${exact}` : `Exact revision ${exact}`}</small><code>{binding.id}</code></span><span className="tool-badges">{panelKind === "sdk" && <ReviewStateBadge state={(binding as APISDKBinding).state} />}<Badge color="zinc">{binding.visibility}</Badge></span><span className="table-actions">{panelKind === "sdk" && <DeveloperAssetAIAdvisoryButton input={advisoryInput} subject={`${label ?? "SDK"} applicability`} label="AI applicability" unavailableReason="Publish an API resource snapshot containing this exact SDK content binding before requesting an applicability advisory." />}<Button outline onClick={() => void openAttach(panelKind, binding)}><RefreshCw data-slot="icon" />Change exact {panelKind === "sdk" ? "version" : "revision"}</Button><Button outline onClick={() => setDetachTarget({ kind: panelKind, binding, label: label ?? binding.id })}><Unlink data-slot="icon" />Detach</Button></span></div>;
      })}{rows.length === 0 && <div className="empty-row">{panelKind === "contract" ? "No API contract is attached. Choose a reviewed Catalog revision, or create a contract root in Catalog and complete source attachment, ingestion, validation, review, and publication before returning here to attach it." : `No ${title.toLowerCase()} are attached. Choose an existing catalog asset or create one for explicit attachment.`}</div>}</div>
    </section>;
  }

  if (loading) return <LoadingPanel label="Loading API resource attachments" />;
  if (problem) return <ProblemPanel message={problem} onRetry={() => void load()} />;

  return <>
    <ExactVersionNotice>This page contains attachment records only. Sources, collections, contracts, packages, and releases remain deployment-owned in Catalog; no attachment upgrades automatically.</ExactVersionNotice>
    <section className="panel api-resource-publications"><PanelHeader title="Published resource history" description="Every API publication freezes the exact developer-asset snapshot used by retrieval and recipe evidence." />{resourcePublications[0] ? <div className="developer-global-active"><span><Badge color="green">latest publication</Badge><code>{resourcePublications[0].id}</code><small>API revision {resourcePublications[0].api_revision_id}</small></span><span><code>{resourcePublications[0].snapshot_hash}</code><small>{resourcePublications[0].documentation.length} documentation · {resourcePublications[0].contracts.length} contracts · {resourcePublications[0].sdks.length} SDKs</small></span></div> : <p className="empty-row">No immutable resource snapshot has been published for this API.</p>}<details className="advanced-details"><summary>Exact publication IDs and hashes</summary><div className="developer-asset-publication-history">{resourcePublications.map((publication) => <div key={publication.id}><span><strong>{new Date(publication.published_at).toLocaleString()}</strong><code>{publication.id}</code></span><span><code>{publication.snapshot_hash}</code><small>{publication.snapshot_schema_version}</small></span></div>)}{resourcePublications.length === 0 && <small>No publication history.</small>}</div></details></section>
    {resourcePanel("documentation", "Documentation", "Reviewed collection revisions available to this API.", bindings.documentation)}
    {resourcePanel("contract", "API contracts", "Attach an exact reviewed revision. New contract roots must first complete source attachment, ingestion, validation, review, and publication in Catalog.", bindings.contracts)}
    {resourcePanel("sdk", "SDK releases", "Exact package releases and their explicit applicability state.", bindings.sdks)}
    <Dialog open={attachOpen} onClose={setAttachOpen} title={`${editing ? "Change" : "Attach"} ${kind === "documentation" ? "documentation" : kind === "contract" ? "API contract" : "SDK release"}`} description={`Select one exact reviewed ${kind === "sdk" ? "version" : "revision"}. This revisioned action never follows latest.`} actions={<><Button outline onClick={() => setAttachOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !assetID || !exactID} onClick={() => void saveAttachment()}>{busy ? "Saving…" : editing ? `Change exact ${kind === "sdk" ? "version" : "revision"}` : "Attach exact resource"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Catalog {kind === "sdk" ? "package" : kind === "contract" ? "contract" : "collection"}</span><select disabled={Boolean(editing)} value={assetID} onChange={(event) => void chooseAsset(event.target.value)}><option value="">Select from Catalog</option>{availableAssets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name}</option>)}</select></label><label className="auth-field"><span>Exact {kind === "sdk" ? "release" : "reviewed revision"}</span><select value={exactID} onChange={(event) => { setExactID(event.target.value); setContentPublicationID(""); }}><option value="">Select exact {kind === "sdk" ? "release" : "revision"}</option>{exactOptions.map((option) => <option key={option.id} value={option.id}>{kind === "sdk" ? (option as SDKRelease).exact_version : `r${(option as DocumentationCollectionRevision | APIContractRevision).revision}`} · {option.id}</option>)}</select>{kind === "sdk" && selectedSDKRelease && <small><code>{selectedSDKRelease.release_hash}</code></small>}</label>{kind === "contract" && <label className="compact-check"><input type="checkbox" checked={primary} onChange={(event) => setPrimary(event.target.checked)} /><span>Use as this API’s primary contract</span></label>}{kind === "sdk" && <><label className="auth-field"><span>Reviewed content publication</span><select value={contentPublicationID} onChange={(event) => setContentPublicationID(event.target.value)}><option value="">None — keep attachment in draft</option>{contentPublications.map((publication) => <option key={publication.id} value={publication.id}>r{publication.revision} · {publication.content_hash}</option>)}</select></label><div className="two-fields"><label className="auth-field"><span>Coverage</span><select value={coverage} onChange={(event) => setCoverage(event.target.value as APISDKBinding["coverage"])}><option value="unknown">Unknown</option><option value="partial">Partial</option><option value="full">Full</option></select></label><label className="auth-field"><span>Assurance</span><select value={assurance} onChange={(event) => setAssurance(event.target.value as APISDKBinding["assurance"])}><option value="related">Related</option><option value="documented">Documented</option><option value="reviewed">Reviewed</option><option value="tested">Tested</option><option value="verified">Verified</option></select></label></div></>}</div>
    </Dialog>
    <Dialog open={createOpen} onClose={setCreateOpen} title={`Create ${kind === "documentation" ? "documentation collection" : kind === "contract" ? "API contract in Catalog" : "SDK package and release"}`} description={kind === "contract" ? "This creates only the reusable contract root. It does not ingest, approve, publish, or attach a contract to this API." : `Create deployment-owned content and attach its exact ${kind === "sdk" ? "release" : "first reviewed revision"}.`} actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !resourceName.trim() || !acknowledged || (kind === "documentation" && (!resourceSlug.trim() || !memberID.trim())) || (kind === "contract" && !resourceSlug.trim()) || (kind === "sdk" && (!ecosystem.trim() || !coordinate.trim() || !exactVersion.trim() || exactVersion.trim().toLowerCase() === "latest"))} onClick={() => void createResource()}>{busy ? "Creating…" : kind === "contract" ? "Create in Catalog" : "Create & attach exact resource"}</Button></>}>
      <div className="auth-form compact-form">{kind === "contract" && <div className="notice"><FileCode2 /><span><strong>Next steps happen in Catalog.</strong> Attach an OpenAPI source, ingest and validate the normalized candidate, review and publish an immutable revision, then return to this API’s Resources tab and use Attach existing.</span></div>}<label className="auth-field"><span>Name</span><input value={resourceName} onChange={(event) => { setResourceName(event.target.value); if (kind !== "sdk") setResourceSlug(slugify(event.target.value)); }} /></label>{kind !== "sdk" && <label className="auth-field"><span>Slug</span><input value={resourceSlug} onChange={(event) => setResourceSlug(slugify(event.target.value))} /></label>}{kind === "documentation" && <label className="auth-field"><span>Exact reviewed source-publication ID</span><input value={memberID} onChange={(event) => setMemberID(event.target.value)} /><small>This creates an immutable collection revision from reviewed evidence.</small></label>}{kind === "sdk" && <><div className="two-fields"><label className="auth-field"><span>Ecosystem</span><input value={ecosystem} onChange={(event) => setEcosystem(event.target.value)} /></label><label className="auth-field"><span>Coordinate</span><input value={coordinate} onChange={(event) => setCoordinate(event.target.value)} /></label></div><label className="auth-field"><span>Exact version</span><input value={exactVersion} onChange={(event) => setExactVersion(event.target.value)} /></label><label className="auth-field"><span>Canonical install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="Leave blank for the server canonical command" /><small>Enter only a verified ecosystem-specific command.</small></label></>}<label className="compact-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{kind === "contract" ? "I understand creation does not approve, publish, or attach a contract." : "I reviewed the exact identity and understand future catalog changes will not upgrade this API."}</span></label></div>
    </Dialog>
    <Dialog open={Boolean(detachTarget)} onClose={(open) => { if (!open) setDetachTarget(null); }} title={`Detach ${detachTarget?.label ?? "resource"}`} description="This removes the resource from future API publications. The detached binding remains in audit history and the catalog asset is not deleted." actions={<><Button outline onClick={() => setDetachTarget(null)}>Cancel</Button><Button color="red" disabled={busy} onClick={() => void detach()}>{busy ? "Detaching…" : "Detach resource"}</Button></>}><div className="notice"><Unlink /><span><strong>No catalog content will be deleted.</strong> This action affects only the explicit attachment to {integration.display_name}.</span></div></Dialog>
  </>;
}
