import { useTranslation } from "react-i18next";
import {
  ArrowLeft, BookOpen, Check, CheckCircle2, ChevronRight,
  GitBranch, Plus, RefreshCw, Search,
  ShieldCheck, TriangleAlert, XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIError, APIIdentity, APIIntegration, APIIntegrationAnalysis,
  APIIntegrationPublishStatus, APIIntegrationRevision, APIResourceSet, APISourcePublication,
  APITool, Distribution, api,
} from "../../lib/api";
import {
  IntegrationResourceTab, IntegrationTab,
  integrationPath, integrationValidationPath, sectionPath,
} from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PanelHeader } from "../core/layout";
import { IntegrationNavigation } from "../integrations/IntegrationNavigation";
import { IntegrationQuickStart } from "../integrations/IntegrationQuickStart";
import { IntegrationRuntimeAccess } from "../integrations/IntegrationRuntimeAccess";
import {
  ConsoleLink, DocumentationAttachmentResult, Source, apiFamilyKeyFromName,
} from "./shared";
import { AuthorizationPolicyWorkspace } from "./integrations/authorization-policy-workspace";
import { IntegrationTestWorkspace } from "./integrations/test-workspace";
import { IntegrationToolsWorkspace } from "./integrations/tools-workspace";
import { APIResourcesWorkspace } from "./developer-assets/api-resources-workspace";
import { APIResourcePublicationHistory } from "./developer-assets/api-resource-publication-history";

function IntegrationDirectoryView({ integrations, query, onQueryChange, onCreate, onNavigate }: { integrations: APIIntegration[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [showRetired, setShowRetired] = useState(false);
  const normalizedQuery = query.trim().toLowerCase();
  const retiredCount = integrations.filter((integration) => integration.lifecycle === "retired").length;
  const setupIssueCount = (integration: APIIntegration) => Number((integration.resources?.length ?? 0) === 0);
  const filteredIntegrations = integrations
    .filter((integration) => showRetired || integration.lifecycle !== "retired")
    .filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)))
    .sort((left, right) => left.display_name.localeCompare(right.display_name) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true }));

  return <>
    <PageHeading eyebrow={t("navigation.apis")} title={t("integrations.apiDirectory")} action={<Button onClick={onCreate}><Plus data-slot="icon" />{t("integrations.addAPI")}</Button>} />
    <div className="toolbar integration-toolbar">
      <div className="search-field"><Search /><input aria-label={t("integrations.searchAPIs")} placeholder={t("integrations.searchAPIs2")} value={query} onChange={(event) => onQueryChange(event.target.value)} /></div>
      <span className="toolbar-count">{filteredIntegrations.length} API{filteredIntegrations.length === 1 ? "" : t("integrations.s")}</span>
    </div>
    <div className="integration-directory-wrap">
      <DataTable label={t("navigation.apis")} className="integration-directory">
        <DataTableHeader className="integration-directory-columns"><span>API</span><span>{t("integrations.lifecycle")}</span><span>{t("integrations.setup")}</span><span>{t("integrations.resources")}</span><span>{t("integrations.open")}</span></DataTableHeader>
        {filteredIntegrations.map((integration) => { const issues = setupIssueCount(integration); return <DataTableRow key={integration.id} className="integration-directory-columns integration-directory-row">
          <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-link"><strong>{integration.display_name}</strong></ConsoleLink><small>{integration.version_key}</small></span></span>
          <span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge> <Badge color={integration.visibility === "public" ? "blue" : "zinc"}>{integration.visibility}</Badge></span>
          <span><Badge color={issues === 0 ? "green" : "amber"}>{issues === 0 ? t("integrations.ready") : t("integrations.stepsLeft", { count: issues })}</Badge></span>
          <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">{t("integrations.attachedSets")}</small></span>
          <span className="table-open-cell"><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${integration.display_name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>; })}
        {filteredIntegrations.length === 0 && <DataTableEmpty columns={5}>{integrations.length === 0 ? t("integrations.noAPIsYetAddOneManuallyOrImportYour") : retiredCount === integrations.length && !showRetired && !normalizedQuery ? t("integrations.noCurrentAPIsShowRetiredAPIsToViewThe") : t("integrations.noAPIsMatchThisSearch")}</DataTableEmpty>}
      </DataTable>
      {retiredCount > 0 && <button type="button" className="retired-directory-toggle" aria-pressed={showRetired} onClick={() => setShowRetired((visible) => !visible)}>{showRetired ? t("integrations.hideRetired") : t("integrations.showRetired", { retiredCount: String(retiredCount) })}</button>}
    </div>
  </>;
}

type IntegrationWorkspaceViewProps = {
  integration: APIIntegration | null;
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  activeTab: IntegrationTab;
  activeResourceTab: IntegrationResourceTab;
  live: boolean;
  loading: boolean;
  revisions: APIIntegrationRevision[];
  publishStatus: APIIntegrationPublishStatus | null;
  identity: APIIdentity | null;
  resourceSets: APIResourceSet[];
  sources: Source[];
  distribution: Distribution | null;
  busy: boolean;
  onEdit: (integration: APIIntegration) => void;
  onPublish: (integration: APIIntegration) => void;
  onAttach: (integration: APIIntegration, kind?: APIResourceSet["kind"]) => void;
  onCreateResource: () => void;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source, attachIntegrationID?: string) => void;
  onAttachPublishedSource: (integration: APIIntegration, source: Source) => Promise<void>;
  onGenerateSetupGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onEditResource: (resource: APIResourceSet) => void;
  onDuplicateResource: (resource: APIResourceSet) => void;
  onDetachResource: (integrationID: string, resourceSetID: string) => void;
  onInspectRevision: (revision: APIIntegrationRevision) => void;
  onRuntimeChanged: () => void | Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

function IntegrationWorkspaceView({ integration, tools, activeTab, loading, revisions, publishStatus, identity, distribution, busy, onEdit, onPublish, onInspectRevision, onRuntimeChanged, onMessage, onNavigate, live }: IntegrationWorkspaceViewProps) {
  const { t } = useTranslation();
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">API</p><h1>{t("integrations.loadingAPI")}</h1><p>{t("integrations.retrievingItsConfigurationAndPublishedHistory")}</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">API</p><h1>{t("integrations.apiUnavailable")}</h1><p>{t("integrations.thisAPIIsNotAvailableInTheCurrentDeployment")}</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("integrations.returnToAPIs")}</ConsoleLink></section>;

  const attachedResources = integration.resources ?? [];
  const documentationResources = attachedResources.filter((resource) => resource.kind === "documentation");
  const contractResources = attachedResources.filter((resource) => resource.kind === "api");
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);
  const publishValidationCodes = new Set(publishStatus?.validations.map((validation) => validation.code) ?? []);
  const setupSteps: Array<{ label: string; detail: string; ready: boolean; path: string }> = [
    { label: t("integrations.configureRuntimeAccess"), detail: t("integrations.configureRuntimeAccessDetail"), ready: Boolean(publishStatus && !publishValidationCodes.has("access_missing")), path: integrationPath(integration.id, "access") },
    { label: t("integrations.addTrustedDocumentation"), detail: t("integrations.addTrustedDocumentationDetail"), ready: documentationResources.length > 0, path: integrationPath(integration.id, "documentation") },
    { label: t("integrations.attachAPIContract"), detail: t("integrations.attachAPIContractDetail"), ready: contractResources.length > 0, path: integrationPath(integration.id, "documentation") },
    { label: t("integrations.configureCustomerAccess"), detail: t("integrations.configureCustomerAccessDetail"), ready: Boolean(identity?.configured && identity.state === "active" && publishStatus && !publishValidationCodes.has("authorization_missing")), path: integrationPath(integration.id, "access") },
    { label: t("integrations.exposeTools"), detail: t("integrations.exposeToolsDetail"), ready: Boolean(publishStatus && !publishValidationCodes.has("tools_missing")), path: integrationPath(integration.id, "tools") },
    { label: t("integrations.validateConfiguration"), detail: t("integrations.validateConfigurationDetail"), ready: Boolean(publishStatus?.ready), path: integrationPath(integration.id, "test") },
  ];
  const setupValidationCodes = new Set(["resources_missing", "authorization_missing", "tools_missing", "access_missing", "support_inherited"]);
  const actionableValidations = publishStatus?.validations.filter((validation) => !setupValidationCodes.has(validation.code)) ?? [];
  const hasChanges = Boolean(publishStatus?.has_changes);
  const canPublish = Boolean(publishStatus?.ready && hasChanges);
  const setupComplete = setupSteps.filter((step) => step.ready).length;
  const validationPath = (tab: string) => integrationValidationPath(integration.id, tab);
  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{t("integrations.allAPIs")}</ConsoleLink></div>
    <PageHeading eyebrow={t("integrations.copy", { family_key: String(integration.family_key), version_key: String(integration.version_key) })} title={integration.display_name} action={<span className="heading-actions"><Button outline onClick={() => onEdit(integration)}>{t("integrations.edit")}</Button>{!publishStatus ? <span className="published-state checking"><RefreshCw />{t("integrations.checking")}</span> : canPublish ? <Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />{t("integrations.publish")}</Button> : hasChanges && !publishStatus.ready ? <Badge color="amber">{t("integrations.setupRequired")}</Badge> : <span className="published-state"><CheckCircle2 />{t("integrations.published")}</span>}</span>} />
    <IntegrationNavigation integrationID={integration.id} integrationName={integration.display_name} activeTab={activeTab} onNavigate={onNavigate} />

    {activeTab === "overview" && <IntegrationQuickStart
      lifecycle={integration.lifecycle}
      status={!publishStatus ? "checking" : publishStatus.ready ? hasChanges ? "ready" : "published" : "setup"}
      statusDetail={!publishStatus ? t("integrations.loadingLatestPublicationState") : hasChanges ? t("integrations.setupStepsWithUnpublishedChanges", { count: publishStatus.changes.length, completed: setupComplete, total: setupSteps.length }) : t("integrations.setupStepsReady", { completed: setupComplete, total: setupSteps.length })}
      steps={setupSteps}
      validations={actionableValidations.map((validation) => ({ ...validation, path: validationPath(validation.tab) }))}
      onNavigate={onNavigate}
      advanced={<>
        <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>{t("integrations.resources")}</strong><small>{t("integrations.exactDocumentationAPIContractAndSDKAttachments")}</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={sectionPath("identity")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><ShieldCheck /></span><span><strong>{t("integrations.customerIdentity")}</strong><small>{identity?.configured && identity.state === "active" ? t("integrations.oidcCustomerSignInActive") : identity?.configured ? t("integrations.oidcDraftNotActive") : t("integrations.oidcNotConfigured")}</small></span><ChevronRight /></ConsoleLink></div>
        <section className="panel"><PanelHeader title={t("integrations.apiDetails")} /><dl className="entity-detail-grid"><div><dt>{t("integrations.apiID")}</dt><dd>{integration.id}</dd></div><div><dt>{t("integrations.family")}</dt><dd>{integration.family_key}</dd></div><div><dt>{t("integrations.version")}</dt><dd>{integration.version_key}</dd></div><div><dt>{t("integrations.draftRevision")}</dt><dd>{integration.revision}</dd></div><div><dt>{t("integrations.replacement")}</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>{t("integrations.sunset")}</dt><dd>{integration.sunset_at ? t("format.date", { value: new Date(integration.sunset_at) }) : "—"}</dd></div></dl></section>
      </>}
    />}

    {activeTab === "documentation" && <div className="integration-tab-content"><APIResourcesWorkspace integration={integration} live={live} onMessage={onMessage} onNavigate={onNavigate} /></div>}

    {activeTab === "access" && <div className="integration-tab-content">
      <IntegrationRuntimeAccess integration={integration} key={integration.id} onMessage={onMessage} onNavigate={onNavigate} onChanged={onRuntimeChanged} />
      <section className="panel">
        <PanelHeader title={t("integrations.customerAuthorization")} description={t("integrations.thisAPIInheritsCustomerSignInFromTheDeployment")} action={<ConsoleLink path={sectionPath("identity")} onNavigate={onNavigate} className="entity-back-link">{t("integrations.openIdentity")}</ConsoleLink>} />
        <div className="integration-identity-summary"><span className="settings-icon"><ShieldCheck /></span><span><strong>{identity?.configured && identity.state === "active" ? t("integrations.centralCustomerIdentityIsActive") : t("integrations.customerIdentityNeedsSetup")}</strong><small>{identity?.configured && identity.state === "active" ? t("integrations.reviewToolPermissionsBelowBeforePublishingExecutableActions") : t("integrations.connectAndTestTheDeploymentOIDCProviderBeforeEnabling")}</small></span><Badge color={identity?.configured && identity.state === "active" ? "green" : "amber"}>{identity?.configured && identity.state === "active" ? t("integrations.active") : t("integrations.setup")}</Badge></div>
      </section>
      <AuthorizationPolicyWorkspace integration={integration} onMessage={onMessage} />
    </div>}

    {activeTab === "tools" && <div className="integration-tab-content"><IntegrationToolsWorkspace integration={integration} tools={tools} onMessage={onMessage} onNavigate={onNavigate} /></div>}

    {activeTab === "test" && <IntegrationTestWorkspace key={`${integration.id}:${publishStatus?.current_manifest_hash ?? ""}`} integration={integration} distribution={distribution} onNavigate={onNavigate} />}

    {activeTab === "history" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>{t("integrations.publishedHistoryIsImmutable")}</strong> {t("integrations.eachEntryPreservesTheExactDocumentationSDKsAccessAnd")}</span></div><section className="panel"><PanelHeader title={t("integrations.publishedHistory")} />{sortedRevisions.map((revision) => <button type="button" className="integration-revision-row" key={revision.id} onClick={() => onInspectRevision(revision)}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? t("format.dateTime", { value: new Date(revision.published_at ?? revision.created_at) }) : t("integrations.dateUnavailable")}</small></span><ChevronRight /></button>)}{sortedRevisions.length === 0 && <div className="empty-row">{t("integrations.nothingHasBeenPublishedYet")}</div>}</section><APIResourcePublicationHistory integrationID={integration.id} live={live} onMessage={onMessage} /></div>}
  </>;
}

type IntegrationsViewProps = {
  live?: boolean;
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  identity: APIIdentity | null;
  distribution: Distribution | null;
  selectedIntegrationID?: string;
  activeTab?: IntegrationTab;
  activeResourceTab?: IntegrationResourceTab;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source, attachIntegrationID?: string) => void;
  onAttachPublishedSource: (integrationID: string, source: Source, publication: APISourcePublication) => Promise<DocumentationAttachmentResult>;
  onGenerateSetupGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onChanged: () => Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

export function IntegrationsView({ live = true, integrations, analyses, tools, resourceSets, sources, identity, distribution, selectedIntegrationID, activeTab = "overview", activeResourceTab = "documentation", onAddSource, onCrawlSource, onPublishSource, onAttachPublishedSource, onGenerateSetupGuide, onChanged, onMessage, onNavigate }: IntegrationsViewProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<APIIntegration | null>(null);
  const [selectedRevisions, setSelectedRevisions] = useState<APIIntegrationRevision[]>([]);
  const [selectedPublishStatus, setSelectedPublishStatus] = useState<APIIntegrationPublishStatus | null>(null);
  const [loadedIntegrationID, setLoadedIntegrationID] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editingIntegration, setEditingIntegration] = useState<APIIntegration | null>(null);
  const [familyKey, setFamilyKey] = useState("");
  const [versionKey, setVersionKey] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [integrationVisibility, setIntegrationVisibility] = useState<APIIntegration["visibility"]>("private");
  const [integrationPublicAcknowledged, setIntegrationPublicAcknowledged] = useState(false);
  const [lifecycle, setLifecycle] = useState<APIIntegration["lifecycle"]>("draft");
  const [replacementID, setReplacementID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [resourceOpen, setResourceOpen] = useState(false);
  const [editingSet, setEditingSet] = useState<APIResourceSet | null>(null);
  const [setKind, setSetKind] = useState<APIResourceSet["kind"]>("documentation");
  const [setName, setSetName] = useState("");
  const [resourceDescription, setResourceDescription] = useState("");
  const [setManifest, setSetManifest] = useState("[]");
  const [selectedSourcePublicationIDs, setSelectedSourcePublicationIDs] = useState<string[]>([]);
  const [duplicateSet, setDuplicateSet] = useState<APIResourceSet | null>(null);
  const [duplicateName, setDuplicateName] = useState("");
  const [attachIntegration, setAttachIntegration] = useState<APIIntegration | null>(null);
  const [attachSetID, setAttachSetID] = useState("");
  const [attachKind, setAttachKind] = useState<APIResourceSet["kind"] | "">("");
  const [pinAttachedSet, setPinAttachedSet] = useState(false);
  const [publishCandidate, setPublishCandidate] = useState<APIIntegration | null>(null);
  const [inspectedRevision, setInspectedRevision] = useState<APIIntegrationRevision | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!selectedIntegrationID) return;
    let cancelled = false;
    api.integration(selectedIntegrationID).then((value) => {
      if (cancelled) return;
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(selectedIntegrationID);
    }).catch(() => {
      if (cancelled) return;
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
      setLoadedIntegrationID(selectedIntegrationID);
    });
    return () => { cancelled = true; };
  }, [selectedIntegrationID]);

  async function refreshSelectedIntegration(integrationID = selectedIntegrationID) {
    if (!integrationID) return;
    try {
      const value = await api.integration(integrationID);
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(integrationID);
    } catch {
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
      setLoadedIntegrationID(integrationID);
    }
  }

  function openIntegration(value?: APIIntegration) {
    setEditingIntegration(value ?? null);
    setFamilyKey(value?.family_key ?? ""); setVersionKey(value?.version_key ?? "v1"); setDisplayName(value?.display_name ?? ""); setDescription(value?.description ?? ""); setIntegrationVisibility(value?.visibility ?? "private"); setIntegrationPublicAcknowledged(false); setLifecycle(value?.lifecycle ?? "draft"); setReplacementID(value?.replacement_integration_id ?? ""); setSunsetAt(value?.sunset_at?.slice(0, 10) ?? "");
    setIntegrationOpen(true);
  }

  async function saveIntegration() {
    setBusy(true);
    try {
      const base = { family_key: editingIntegration ? familyKey : apiFamilyKeyFromName(displayName), version_key: versionKey, display_name: displayName, description: editingIntegration ? description : "", visibility: editingIntegration ? integrationVisibility : "private" as const, acknowledge_public: editingIntegration ? integrationPublicAcknowledged : false, lifecycle: editingIntegration ? lifecycle : "draft" as const };
      const saved = editingIntegration
        ? await api.updateIntegration(editingIntegration.id, { ...base, replacement_integration_id: replacementID || undefined, sunset_at: sunsetAt ? new Date(`${sunsetAt}T00:00:00Z`).toISOString() : undefined, revision: editingIntegration.revision })
        : await api.createIntegration(base);
      setSelectedDetail(saved);
      await onChanged();
      if (editingIntegration) await refreshSelectedIntegration(saved.id);
      setIntegrationOpen(false);
      onMessage(editingIntegration ? t("integrations.apiUpdated") : t("integrations.apiCreatedWithLifecycle", { lifecycle: String(saved.lifecycle) }));
      if (!editingIntegration) onNavigate(integrationPath(saved.id));
    } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.apiCouldNotBeSaved")); } finally { setBusy(false); }
  }

  async function publishIntegration() {
    if (!publishCandidate) return;
    setBusy(true);
    try {
      const preflight = await api.preflightIntegration(publishCandidate.id);
      if (!preflight.ready) {
        const failed = preflight.checks.find((check) => check.required && check.status !== "pass");
        throw new Error(failed?.message ?? t("integrations.theServerPreflightFoundARequiredConfigurationGap"));
      }
      await api.publishIntegration(publishCandidate.id, preflight.candidate_revision, preflight.candidate_manifest_hash);
      await onChanged();
      await refreshSelectedIntegration(publishCandidate.id);
      setPublishCandidate(null);
      onMessage(t("integrations.apiPublishedFromTheExactPreflightCandidate"));
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.apiCouldNotBePublished")); } finally { setBusy(false); }
  }

  function openResource(value?: APIResourceSet) {
    const manifest = value?.latest_revision?.manifest ?? [];
    setEditingSet(value ?? null); setSetKind(value?.kind ?? "documentation"); setSetName(value?.name ?? ""); setResourceDescription(value?.description ?? ""); setSetManifest(JSON.stringify(manifest, null, 2)); setSelectedSourcePublicationIDs(manifest.map((entry) => typeof entry.source_publication_id === "string" ? entry.source_publication_id : "").filter(Boolean)); setResourceOpen(true);
  }

  async function saveResourceSet() {
    setBusy(true);
    try {
      const latestPublicationEntries = sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []);
      const parsedManifest = JSON.parse(setManifest) as unknown;
      if (!Array.isArray(parsedManifest)) throw new Error(t("integrations.manifestMustBeAJSONArray"));
      const existingEntries = parsedManifest as Array<Record<string, unknown>>;
      const options = new Map([...existingEntries, ...latestPublicationEntries].map((entry) => [String(entry.source_publication_id ?? ""), entry]));
      const manifest = setKind === "documentation" ? selectedSourcePublicationIDs.map((id) => options.get(id)).filter((entry): entry is Record<string, unknown> => Boolean(entry)) : existingEntries;
      if (setKind === "documentation" && manifest.length !== selectedSourcePublicationIDs.length) throw new Error(t("integrations.everySelectedDocumentationPublicationMustStillExist"));
      if (editingSet) await api.updateResourceSet(editingSet.id, { name: setName, description: resourceDescription, state: editingSet.state, manifest, revision: editingSet.revision });
      else await api.createResourceSet({ kind: setKind, name: setName, description: resourceDescription, manifest });
      await onChanged(); setResourceOpen(false); onMessage(editingSet ? t("integrations.newImmutableResourceSetRevisionCreated") : t("integrations.reusableResourceSetCreated"));
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.resourceSetCouldNotBeSaved")); } finally { setBusy(false); }
  }

  async function duplicateResource() {
    if (!duplicateSet) return;
    setBusy(true);
    try { await api.duplicateResourceSet(duplicateSet.id, duplicateName); await onChanged(); setDuplicateSet(null); onMessage(t("integrations.independentResourceSetCopyCreated")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeDuplicated")); } finally { setBusy(false); }
  }

  async function attachResource() {
    const resource = resourceSets.find((value) => value.id === attachSetID);
    if (!attachIntegration || !resource) return;
    setBusy(true);
    try { await api.attachResourceSet(attachIntegration.id, resource.id, pinAttachedSet ? resource.latest_revision?.id ?? "" : ""); await onChanged(); await refreshSelectedIntegration(attachIntegration.id); setAttachIntegration(null); onMessage(pinAttachedSet ? t("integrations.resourceRevisionPinnedToAPI") : t("integrations.resourceSetAttachedAndFollowingLatest")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeAttached")); } finally { setBusy(false); }
  }

  async function attachPublishedSource(integration: APIIntegration, source: Source) {
	if (!source.latestPublication) return;
	setBusy(true);
	try {
	  const result = await onAttachPublishedSource(integration.id, source, source.latestPublication);
	  await refreshSelectedIntegration(integration.id);
	  onMessage(result.attached ? t("integrations.rWasPinnedToThisAPI", { name: String(source.name), revision: String(source.latestPublication.revision) }) : t("integrations.rIsAlreadyAttached", { name: String(source.name), revision: String(source.latestPublication.revision) }));
	} catch (error) {
	  onMessage(error instanceof APIError || error instanceof Error ? error.message : t("integrations.reviewedDocumentationCouldNotBeAttached"));
	} finally {
	  setBusy(false);
	}
  }

  async function detachResource(integrationID: string, setID: string) {
    setBusy(true);
    try { await api.detachResourceSet(integrationID, setID); await onChanged(); await refreshSelectedIntegration(integrationID); onMessage(t("integrations.resourceSetDetachedFromAPI")); } catch (error) { onMessage(error instanceof APIError ? error.message : t("integrations.resourceSetCouldNotBeDetached")); } finally { setBusy(false); }
  }

  const selectedIntegration = selectedDetail?.id === selectedIntegrationID ? selectedDetail : integrations.find((integration) => integration.id === selectedIntegrationID) ?? null;
  const selectedLoading = Boolean(selectedIntegrationID && loadedIntegrationID !== selectedIntegrationID);

  function openAttachDialog(integration: APIIntegration, kind: APIResourceSet["kind"] | "" = "") {
    const availableSets = resourceSets.filter((set) => (!kind || set.kind === kind) && !(integration.resources ?? []).some((resource) => resource.resource_set_id === set.id));
    setAttachIntegration(integration);
    setAttachKind(kind);
    setAttachSetID(availableSets[0]?.id ?? "");
    setPinAttachedSet(false);
  }

  let currentResourceManifest: Array<Record<string, unknown>> = [];
  try {
    const parsed = JSON.parse(setManifest) as unknown;
    if (Array.isArray(parsed)) currentResourceManifest = parsed as Array<Record<string, unknown>>;
  } catch {
    currentResourceManifest = [];
  }
  const documentationPublicationOptions = Array.from(new Map([
    ...currentResourceManifest.filter((entry) => typeof entry.source_publication_id === "string"),
    ...sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []),
  ].map((entry) => [String(entry.source_publication_id), entry])).values());

  return <>
    {selectedIntegrationID ? <IntegrationWorkspaceView live={live} integration={selectedIntegration} analyses={analyses} tools={tools} activeTab={activeTab} activeResourceTab={activeResourceTab} loading={selectedLoading} revisions={selectedRevisions} publishStatus={selectedPublishStatus} identity={identity} resourceSets={resourceSets} sources={sources} distribution={distribution} busy={busy} onEdit={openIntegration} onPublish={setPublishCandidate} onAttach={openAttachDialog} onCreateResource={() => openResource()} onAddSource={onAddSource} onCrawlSource={onCrawlSource} onPublishSource={onPublishSource} onAttachPublishedSource={attachPublishedSource} onGenerateSetupGuide={onGenerateSetupGuide} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(t("integrations.duplicateName", { name: set.name })); }} onDetachResource={detachResource} onInspectRevision={setInspectedRevision} onRuntimeChanged={async () => { await onChanged(); await refreshSelectedIntegration(selectedIntegrationID); }} onMessage={onMessage} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onNavigate={onNavigate} />}

    <Dialog
      open={integrationOpen}
      onClose={setIntegrationOpen}
      title={editingIntegration ? t("integrations.editAPI") : t("integrations.addAPI")}
      description={editingIntegration ? t("integrations.updateThisAPISIdentityPublishingAndLifecycleSettings") : t("integrations.createAPrivateDraftYouCanAddDocumentationAccess")}
      actions={<><Button outline onClick={() => setIntegrationOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !versionKey.trim() || !displayName.trim() || Boolean(editingIntegration && (!familyKey.trim() || (integrationVisibility === "public" && editingIntegration.visibility !== "public" && !integrationPublicAcknowledged)))} onClick={saveIntegration}>{busy ? t("common.saving") : editingIntegration ? t("integrations.saveChanges") : t("integrations.createAPI")}</Button></>}
    >
      <div className="auth-form compact-form">
        {!editingIntegration ? <>
          <label className="auth-field"><span>{t("integrations.apiName")}</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder={t("integrations.exVoiceAPI")} /></label>
          <label className="auth-field"><span>{t("integrations.version")}</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} placeholder="v1" /><small>{t("integrations.startWithV1UnlessThisAPIAlreadyHasA")}</small></label>
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>{t("integrations.apiFamilyKey")}</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} /></label><label className="auth-field"><span>{t("integrations.apiVersion")}</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} /></label></div>
          <label className="auth-field"><span>{t("integrations.displayName")}</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
          <label className="auth-field"><span>{t("integrations.description")}</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          <div className="two-fields"><label className="auth-field"><span>{t("integrations.visibility")}</span><select value={integrationVisibility} onChange={(event) => { setIntegrationVisibility(event.target.value as APIIntegration["visibility"]); setIntegrationPublicAcknowledged(false); }}><option value="private">{t("integrations.private")}</option><option value="public">{t("integrations.public")}</option></select><small>{t("integrations.publicExposesOnlyThePublishedReadOnlyAPIManifest")}</small></label><label className="auth-field"><span>{t("integrations.lifecycle")}</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">{t("integrations.draft")}</option><option value="active">{t("integrations.active")}</option><option value="deprecated">{t("integrations.deprecated")}</option><option value="retired">{t("integrations.retired")}</option></select></label></div>
          {integrationVisibility === "public" && editingIntegration.visibility !== "public" && <label className="compact-check"><input type="checkbox" checked={integrationPublicAcknowledged} onChange={(event) => setIntegrationPublicAcknowledged(event.target.checked)} /><span>{t("integrations.iUnderstandThisPublishedAPIMetadataWillBeAnonymously")}</span></label>}
          <label className="auth-field"><span>{t("integrations.replacement")}</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">{t("integrations.none")}</option>{integrations.filter((value) => value.id !== editingIntegration.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label>
          {(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>{t("integrations.sunsetDate")}</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}
        </>}
      </div>
    </Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? t("integrations.createRevisionFor", { name: String(editingSet.name) }) : t("integrations.createReusableResourceSet")} description={t("integrations.setsAreReusableByExplicitAttachmentEachSaveCreates")} actions={<><Button outline onClick={() => setResourceOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !setName.trim() || (setKind === "documentation" && selectedSourcePublicationIDs.length === 0)} onClick={saveResourceSet}>{busy ? t("common.saving") : editingSet ? t("integrations.createRevision") : t("integrations.createSet")}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>{t("integrations.kind")}</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">{t("integrations.documentation")}</option><option value="api">{t("integrations.apiContract")}</option></select></label><label className="auth-field"><span>{t("integrations.name")}</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>{t("integrations.description")}</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label>{setKind === "documentation" ? <div className="auth-field"><span>{t("integrations.reviewedSourcePublications")}</span><div className="catalog-list">{documentationPublicationOptions.map((entry) => { const id = String(entry.source_publication_id); const selected = selectedSourcePublicationIDs.includes(id); return <label className="catalog-tool" key={id}><input type="checkbox" checked={selected} onChange={(event) => setSelectedSourcePublicationIDs((items) => event.target.checked ? [...items, id] : items.filter((value) => value !== id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{String(entry.name ?? entry.source_id)}</strong><code>{id}</code><small>{t("integrations.publicationR")}{String(entry.revision)} · {String(entry.content_hash)}</small></span><Badge color="green">{t("integrations.reviewed")}</Badge></label>; })}{documentationPublicationOptions.length === 0 && <div className="empty-row">{t("integrations.publishAReviewedDocumentationGenerationBeforeCreatingThisSet")}</div>}</div><small>{t("integrations.eachSelectionPinsOneImmutableSourcePublicationRevisionAnd")}</small></div> : <label className="auth-field"><span>{t("integrations.apiContractManifestJSONArray")}</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label>}</div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title={t("integrations.duplicateResourceSet")} description={t("integrations.createsAnIndependentCopySoLaterEditsDoNot")} actions={<><Button outline onClick={() => setDuplicateSet(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>{t("integrations.duplicate")}</Button></>}><label className="auth-field"><span>{t("integrations.newSetName")}</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={t("integrations.attachResourcesTo", { value1: String(attachIntegration?.display_name ?? "API") })} description={t("integrations.followLatestForDeliberateSharingOrPinTheCurrent")} actions={<><Button outline onClick={() => setAttachIntegration(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>{t("integrations.attach")}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>{t("integrations.resourceSet")}</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">{t("integrations.selectASet")}</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind === "api" ? t("integrations.apiContract") : t("integrations.documentation2")} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>{t("integrations.pinTheCurrentRevisionInsteadOfFollowingLatest")}</span></label></div></Dialog>
    <Dialog open={Boolean(publishCandidate)} onClose={(open) => { if (!open) setPublishCandidate(null); }} title={t("integrations.publish2", { value1: String(publishCandidate?.display_name ?? "API") })} description={t("integrations.reviewWhatChangedBeforeCreatingANewImmutableVersion")} actions={<><Button outline onClick={() => setPublishCandidate(null)}>{t("common.cancel")}</Button><Button color="indigo" disabled={busy || !selectedPublishStatus?.ready || !selectedPublishStatus.has_changes} onClick={publishIntegration}>{busy ? t("integrations.publishing") : t("integrations.publish")}</Button></>}><div className="publish-review">{selectedPublishStatus?.validations.map((validation) => <div key={validation.code} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level}</strong><small>{validation.message}</small></span></div>)}<div className="publish-diff-list">{selectedPublishStatus?.changes.map((change) => <div className="publish-diff" key={change.field}><strong>{change.field}</strong><span><small>{t("integrations.published")}</small><code>{change.before === undefined ? "—" : JSON.stringify(change.before)}</code></span><ChevronRight /><span><small>{t("integrations.draft")}</small><code>{change.after === undefined ? "—" : JSON.stringify(change.after)}</code></span></div>)}</div><details className="advanced-details"><summary>{t("integrations.technicalDetails")}</summary><code>{selectedPublishStatus?.current_manifest_hash ?? "—"}</code></details></div></Dialog>
    <Dialog open={Boolean(inspectedRevision)} onClose={(open) => { if (!open) setInspectedRevision(null); }} title={t("integrations.publishedVersionR", { value1: String(inspectedRevision?.revision ?? "") })} description={t("integrations.thisImmutableTechnicalSnapshotIsKeptForAuditAnd")} actions={<Button outline onClick={() => setInspectedRevision(null)}>{t("common.close")}</Button>}><div className="revision-inspector"><dl className="entity-detail-grid"><div><dt>{t("integrations.versionID")}</dt><dd>{inspectedRevision?.id}</dd></div><div><dt>{t("integrations.state")}</dt><dd>{inspectedRevision?.state}</dd></div><div><dt>{t("integrations.published")}</dt><dd>{inspectedRevision ? t("format.dateTime", { value: new Date(inspectedRevision.published_at ?? inspectedRevision.created_at) }) : "—"}</dd></div><div><dt>{t("integrations.publishedBy")}</dt><dd>{inspectedRevision?.published_by || "—"}</dd></div><div><dt>{t("integrations.manifestHash")}</dt><dd><code>{inspectedRevision?.manifest_hash}</code></dd></div></dl><pre className="usage-contract"><code>{JSON.stringify(inspectedRevision?.snapshot ?? {}, null, 2)}</code></pre></div></Dialog>
  </>;
}
