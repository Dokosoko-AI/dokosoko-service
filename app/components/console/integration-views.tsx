import {
  ArrowLeft, BookOpen, Bug, Check, CheckCircle2, ChevronRight, Database,
  ExternalLink, GitBranch, KeyRound, Plus, RefreshCw, Search, Share2,
  ShieldCheck, Sparkles, TerminalSquare, TriangleAlert, XCircle,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import {
  APIAccessConnection, APIAuthorizationPoint, APIError, APIGrantDefinition, APIIdentity,
  APIIntegration, APIIntegrationAnalysis, APIIntegrationPackageBinding, APIIntegrationPreflight,
  APIIntegrationPublishStatus, APIIntegrationRevision, APIIntegrationToolBinding,
  APIPackageArtifact, APIPackageRelease, APIResourceSet, APISourcePublication,
  APISupportRoute, APITool, APIVisibility, Distribution, api,
} from "../../lib/api";
import {
  INTEGRATION_RESOURCE_TABS, IntegrationResourceTab, IntegrationTab,
  integrationPath, integrationToolBuilderPath, integrationValidationPath, sectionPath,
} from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader, SectionHeader } from "../core/layout";
import { IntegrationAgentGuide } from "../integrations/IntegrationAgentGuide";
import { IntegrationNavigation } from "../integrations/IntegrationNavigation";
import { IntegrationQuickStart } from "../integrations/IntegrationQuickStart";
import { IntegrationRuntimeAccess } from "../integrations/IntegrationRuntimeAccess";
import { partitionIntegrationTools, toolCanAttachToIntegration, toolIsCommon, toolIsOwnedByIntegration } from "../integrations/tool-scope";
import {
  ConsoleLink, DocumentationAttachmentResult, EntityLink, Source, analysisMatchesIntegration,
  apiFamilyKeyFromName, integrationIncludesSourcePublication, unavailableConsoleCapability,
} from "./shared";

function IntegrationDirectoryView({ integrations, connections, supportRoutes, query, onQueryChange, onCreate, onBuild, onNavigate }: { integrations: APIIntegration[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onBuild: () => void; onNavigate: (path: string) => void }) {
  const [showRetired, setShowRetired] = useState(false);
  const normalizedQuery = query.trim().toLowerCase();
  const retiredCount = integrations.filter((integration) => integration.lifecycle === "retired").length;
  const connectionCount = (integration: APIIntegration) => connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id)).length;
  const hasSupport = (integration: APIIntegration) => Boolean(supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default));
  const setupIssueCount = (integration: APIIntegration) => Number((integration.resources?.length ?? 0) === 0) + Number(connectionCount(integration) === 0) + Number(!hasSupport(integration));
  const filteredIntegrations = integrations
    .filter((integration) => showRetired || integration.lifecycle !== "retired")
    .filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)))
    .sort((left, right) => left.display_name.localeCompare(right.display_name) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true }));

  return <>
    <PageHeading eyebrow="Catalog" title="APIs" action={<span className="heading-actions"><Button outline onClick={onCreate}><Plus data-slot="icon" />Add API</Button><Button onClick={onBuild}><Sparkles data-slot="icon" />Import APIs</Button></span>} />
    <div className="toolbar integration-toolbar">
      <div className="search-field"><Search /><input aria-label="Search APIs" placeholder="Search APIs…" value={query} onChange={(event) => onQueryChange(event.target.value)} /></div>
      <span className="toolbar-count">{filteredIntegrations.length} API{filteredIntegrations.length === 1 ? "" : "s"}</span>
    </div>
    <div className="integration-directory-wrap">
      <DataTable label="APIs" className="integration-directory">
        <DataTableHeader className="integration-directory-columns"><span>API</span><span>Lifecycle</span><span>Setup</span><span>Resources</span><span>Open</span></DataTableHeader>
        {filteredIntegrations.map((integration) => { const issues = setupIssueCount(integration); return <DataTableRow key={integration.id} className="integration-directory-columns integration-directory-row">
          <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-link"><strong>{integration.display_name}</strong></ConsoleLink><small>{integration.version_key}</small></span></span>
          <span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge> <Badge color={integration.visibility === "public" ? "blue" : "zinc"}>{integration.visibility}</Badge></span>
          <span><Badge color={issues === 0 ? "green" : "amber"}>{issues === 0 ? "Ready" : `${issues} step${issues === 1 ? "" : "s"} left`}</Badge></span>
          <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">attached sets</small></span>
          <span className="table-open-cell"><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${integration.display_name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>; })}
        {filteredIntegrations.length === 0 && <DataTableEmpty columns={5}>{integrations.length === 0 ? "No APIs yet. Add one manually or import your existing API sources." : retiredCount === integrations.length && !showRetired && !normalizedQuery ? "No current APIs. Show retired APIs to view the archive." : "No APIs match this search."}</DataTableEmpty>}
      </DataTable>
      {retiredCount > 0 && <button type="button" className="retired-directory-toggle" aria-pressed={showRetired} onClick={() => setShowRetired((visible) => !visible)}>{showRetired ? "Hide retired" : `Show retired (${retiredCount})`}</button>}
    </div>
  </>;
}

function IntegrationSwitcher({ integrations, integration, activeTab, activeResourceTab, onNavigate }: { integrations: APIIntegration[]; integration: APIIntegration; activeTab: IntegrationTab; activeResourceTab?: IntegrationResourceTab; onNavigate: (path: string) => void }) {
  const optionLabel = (value: APIIntegration) => `${value.display_name} · ${value.version_key} · ${value.family_key}`;
  const [value, setValue] = useState(optionLabel(integration));

  function selectIntegration(nextValue: string) {
    setValue(nextValue);
    const selected = integrations.find((candidate) => candidate.id === nextValue || optionLabel(candidate) === nextValue);
    if (selected && selected.id !== integration.id) onNavigate(integrationPath(selected.id, activeTab, activeResourceTab));
  }

  if (integrations.length <= 1) return null;
  return <div className="integration-workspace-switcher"><label htmlFor="integration-switcher">Switch API</label><div className="integration-switcher-input"><Search /><input id="integration-switcher" list="integration-switcher-options" value={value} onChange={(event) => selectIntegration(event.target.value)} onBlur={() => { if (!integrations.some((candidate) => optionLabel(candidate) === value)) setValue(optionLabel(integration)); }} /><datalist id="integration-switcher-options">{[...integrations].sort((left, right) => left.family_key.localeCompare(right.family_key) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true })).map((candidate) => <option key={candidate.id} value={optionLabel(candidate)} />)}</datalist></div></div>;
}

type IntegrationWorkspaceViewProps = {
  integration: APIIntegration | null;
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  activeTab: IntegrationTab;
  activeResourceTab: IntegrationResourceTab;
  loading: boolean;
  revisions: APIIntegrationRevision[];
  publishStatus: APIIntegrationPublishStatus | null;
  identity: APIIdentity | null;
  resourceSets: APIResourceSet[];
  sources: Source[];
  connections: APIAccessConnection[];
  supportRoutes: APISupportRoute[];
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
  onGenerateAgentGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onEditResource: (resource: APIResourceSet) => void;
  onDuplicateResource: (resource: APIResourceSet) => void;
  onDetachResource: (integrationID: string, resourceSetID: string) => void;
  onManageAccess: (integration: APIIntegration) => void;
  onManageSupport: (integration: APIIntegration) => void;
  onInspectRevision: (revision: APIIntegrationRevision) => void;
  onRuntimeChanged: () => void | Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

function IntegrationWorkspaceView({ integration, integrations, analyses, tools, activeTab, activeResourceTab, loading, revisions, publishStatus, identity, resourceSets, sources, connections, supportRoutes, distribution, busy, onEdit, onPublish, onAttach, onCreateResource, onAddSource, onCrawlSource, onPublishSource, onAttachPublishedSource, onGenerateAgentGuide, onEditResource, onDuplicateResource, onDetachResource, onManageAccess, onManageSupport, onInspectRevision, onRuntimeChanged, onMessage, onNavigate }: IntegrationWorkspaceViewProps) {
  const [guideBusy, setGuideBusy] = useState(false);
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">API</p><h1>Loading API…</h1><p>Retrieving its configuration and published history.</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">API</p><h1>API unavailable</h1><p>This API is not available in the current deployment.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;

  const integrationConnections = connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id));
  const supportRoute = supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default);
  const attachedResources = integration.resources ?? [];
  const documentationResources = attachedResources.filter((resource) => resource.kind === "documentation");
  const contractResources = attachedResources.filter((resource) => resource.kind === "api");
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);
  const agentGuideAnalysis = analyses
    .filter((analysis) => analysis.state === "review" && analysisMatchesIntegration(analysis, integration.id))
    .sort((left, right) => Date.parse(right.completed_at ?? right.created_at) - Date.parse(left.completed_at ?? left.created_at))[0];
  const publishValidationCodes = new Set(publishStatus?.validations.map((validation) => validation.code) ?? []);
  const setupSteps: Array<{ label: string; detail: string; ready: boolean; path: string }> = [
    { label: "Choose service access", detail: "Select an active service connection for this API.", ready: Boolean(publishStatus && !publishValidationCodes.has("access_missing")), path: integrationPath(integration.id, "access") },
    { label: "Add trusted documentation", detail: "Attach an exact reviewed documentation revision.", ready: documentationResources.length > 0, path: integrationPath(integration.id, "documentation", "documentation") },
    { label: "Attach the API contract", detail: "Pin the reviewed API reference agents should follow.", ready: contractResources.length > 0, path: integrationPath(integration.id, "documentation", "contracts") },
    { label: "Configure customer access", detail: "Activate customer identity and define this API's action policy.", ready: Boolean(identity?.configured && identity.state === "active" && publishStatus && !publishValidationCodes.has("authorization_missing")), path: integrationPath(integration.id, "access") },
    { label: "Expose tools", detail: "Attach reviewed API-owned or common tools to this API.", ready: Boolean(publishStatus && !publishValidationCodes.has("tools_missing")), path: integrationPath(integration.id, "tools") },
    { label: "Validate configuration", detail: "Review the server preflight and acceptance scenarios.", ready: Boolean(publishStatus?.ready), path: integrationPath(integration.id, "test") },
  ];
  const setupValidationCodes = new Set(["resources_missing", "authorization_missing", "tools_missing", "access_missing", "support_inherited"]);
  const actionableValidations = publishStatus?.validations.filter((validation) => !setupValidationCodes.has(validation.code)) ?? [];
  const hasChanges = Boolean(publishStatus?.has_changes);
  const canPublish = Boolean(publishStatus?.ready && hasChanges);
  const resourceLabel = (kind: APIResourceSet["kind"]) => kind === "api" ? "API contract" : "documentation";
  const setupComplete = setupSteps.filter((step) => step.ready).length;
  const validationPath = (tab: string) => integrationValidationPath(integration.id, tab);
  const integrationID = integration.id;

  async function generateAgentGuide() {
	setGuideBusy(true);
	try {
	  const analysis = await onGenerateAgentGuide(integrationID);
	  onMessage(analysis.generated_by === "ai_assisted" ? "Agent guide generated from this API's reviewed evidence." : "Agent guide generated deterministically; configure an analysis model for AI-assisted refinement.");
	} catch (error) {
	  onMessage(error instanceof APIError || error instanceof Error ? error.message : "The agent guide could not be generated.");
	} finally {
	  setGuideBusy(false);
	}
  }

  const renderResourceRows = (resources: typeof attachedResources) => <>
    {resources.map((resource) => {
      const source = resourceSets.find((set) => set.id === resource.resource_set_id);
      return <div className="integration-resource-row" key={resource.resource_set_id}><span className="settings-icon">{resource.kind === "documentation" ? <BookOpen /> : <TerminalSquare />}</span><span><EntityLink entity="resource-set" uid={resource.resource_set_id} onNavigate={onNavigate} className="entity-link"><strong>{resource.name}</strong></EntityLink><small>{resourceLabel(resource.kind)} · {resource.follow_latest ? "follows latest" : `pinned to revision ${resource.resolved_revision?.revision ?? "—"}`}</small></span><Badge color={resource.kind === "documentation" ? "blue" : "violet"}>{resourceLabel(resource.kind)}</Badge><span className="table-actions">{source && <Button outline onClick={() => onEditResource(source)}>New revision</Button>}{source && <Button outline onClick={() => onDuplicateResource(source)}>Duplicate</Button>}<button type="button" className="more" disabled={busy} title={`Detach ${resource.name}`} aria-label={`Detach ${resource.name}`} onClick={() => onDetachResource(integration.id, resource.resource_set_id)}><XCircle /></button></span></div>;
    })}
    {resources.length === 0 && <div className="empty-row">Nothing is attached here yet.</div>}
  </>;

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />All APIs</ConsoleLink></div>
    <IntegrationSwitcher key={integration.id} integrations={integrations} integration={integration} activeTab={activeTab} activeResourceTab={activeResourceTab} onNavigate={onNavigate} />
    <PageHeading eyebrow={`${integration.family_key} · ${integration.version_key}`} title={integration.display_name} action={<span className="heading-actions"><Button outline onClick={() => onEdit(integration)}>Edit</Button>{!publishStatus ? <span className="published-state checking"><RefreshCw />Checking…</span> : canPublish ? <Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />Publish</Button> : hasChanges && !publishStatus.ready ? <Badge color="amber">Setup required</Badge> : <span className="published-state"><CheckCircle2 />Published</span>}</span>} />
    <IntegrationNavigation integrationID={integration.id} integrationName={integration.display_name} activeTab={activeTab} onNavigate={onNavigate} />

    {activeTab === "overview" && <IntegrationQuickStart
      lifecycle={integration.lifecycle}
      status={!publishStatus ? "checking" : publishStatus.ready ? hasChanges ? "ready" : "published" : "setup"}
      statusDetail={!publishStatus ? "Loading the latest publication state…" : `${setupComplete}/${setupSteps.length} setup steps ready${hasChanges ? ` · ${publishStatus.changes.length} unpublished change${publishStatus.changes.length === 1 ? "" : "s"}` : ""}`}
      steps={setupSteps}
      validations={actionableValidations.map((validation) => ({ ...validation, path: validationPath(validation.tab) }))}
      onNavigate={onNavigate}
      advanced={<>
        <section className="panel"><PanelHeader title="Bug reports & feedback" action={<Button outline onClick={() => onManageSupport(integration)}>{supportRoute ? "Change" : "Configure"}</Button>} />{supportRoute ? <div className="support-route-summary"><span className="settings-icon"><Bug /></span><span><strong>{supportRoute.name}</strong><small>{supportRoute.is_default ? "Uses the deployment default" : "Configured for this API"} · {supportRoute.retention_days}-day encrypted retention</small></span><Badge color={supportRoute.state === "active" ? "green" : "zinc"}>{supportRoute.bug_reports_enabled || supportRoute.feedback_enabled ? "Available" : "Off"}</Badge></div> : <div className="empty-row">Not configured. Reports remain unavailable until you choose a policy.</div>}</section>
        <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>Documentation</strong><small>Agent guide, API reference, sources and packages.</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={sectionPath("identity")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><ShieldCheck /></span><span><strong>Customer identity</strong><small>{identity?.configured && identity.state === "active" ? "OIDC customer sign-in active" : identity?.configured ? "OIDC draft not active" : "OIDC not configured"}</small></span><ChevronRight /></ConsoleLink></div>
        <section className="panel"><PanelHeader title="API details" /><dl className="entity-detail-grid"><div><dt>API ID</dt><dd>{integration.id}</dd></div><div><dt>Family</dt><dd>{integration.family_key}</dd></div><div><dt>Version</dt><dd>{integration.version_key}</dd></div><div><dt>Draft revision</dt><dd>{integration.revision}</dd></div><div><dt>Replacement</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>Sunset</dt><dd>{integration.sunset_at ? new Date(integration.sunset_at).toLocaleDateString() : "—"}</dd></div></dl></section>
      </>}
    />}

    {activeTab === "documentation" && <div className="integration-tab-content">
      <PageTabs label="Documentation areas">{INTEGRATION_RESOURCE_TABS.map((tab) => <ConsoleLink key={tab.id} path={integrationPath(integration.id, "documentation", tab.id)} onNavigate={onNavigate} className={`page-tab resource-subtab ${activeResourceTab === tab.id ? "active" : ""}`} ariaCurrent={activeResourceTab === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</PageTabs>
      {activeResourceTab === "documentation" && <>
        <IntegrationAgentGuide analysis={agentGuideAnalysis} canGenerate={attachedResources.length > 0} busy={guideBusy} onGenerate={generateAgentGuide} />
        <section className="panel"><PanelHeader title="Documentation ingestion" description="Deployment sources are reusable; attach a reviewed resource revision to this API." action={<span className="heading-actions"><ConsoleLink path={sectionPath("sources")} onNavigate={onNavigate} className="entity-back-link">All documentation</ConsoleLink><Button onClick={onAddSource}><Plus data-slot="icon" />Add source</Button></span>} />{sources.map((source) => { const publicationAttached = Boolean(source.latestPublication && integrationIncludesSourcePublication(integration, source.latestPublication.id)); return <div className="provider-row documentation-source-row" key={source.id}><span className="settings-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.kind} · {source.location}</small></span><span className="tool-badges"><Badge color={source.quarantined || source.crawlState === "failed" ? "red" : source.crawlState === "synced" ? "green" : "amber"}>{source.quarantined ? "quarantined" : source.crawlState === "synced" ? `published r${source.latestPublication?.revision ?? 1}` : source.crawlState}</Badge><Badge color={source.visibility === "public" ? "blue" : "zinc"}>{source.visibility}</Badge></span><span className="table-actions">{source.crawlState !== "queued" && source.crawlState !== "running" && <Button outline disabled={busy} onClick={() => onCrawlSource(source.id)}>Crawl</Button>}{source.crawlState === "review" && <Button disabled={busy} onClick={() => onPublishSource(source, integration.id)}>{source.quarantined ? "Inspect" : "Review & attach"}</Button>}{source.crawlState === "synced" && source.latestPublication && (publicationAttached ? <Button outline disabled><Check data-slot="icon" />Attached</Button> : <Button disabled={busy} onClick={() => void onAttachPublishedSource(integration, source)}>Attach to API</Button>)}</span></div>; })}{sources.length === 0 && <div className="empty-row">No documentation source has been ingested.</div>}</section>
        <section className="panel"><PanelHeader title="Attached documentation" action={<span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create set</Button><Button onClick={() => onAttach(integration, "documentation")}>Attach reviewed set</Button></span>} />{renderResourceRows(documentationResources)}</section>
      </>}
      {activeResourceTab === "contracts" && <>
        <div className="notice"><TerminalSquare /><span><strong>Immutable API contracts.</strong> Review each manifest, then pin or follow a reusable contract revision.</span></div>
        <section className="panel"><PanelHeader title="Attached API contracts" action={<span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create contract set</Button><Button onClick={() => onAttach(integration, "api")}>Attach existing</Button></span>} />{renderResourceRows(contractResources)}</section>
      </>}
      {activeResourceTab === "packages" && <IntegrationPackagesWorkspace integration={integration} onMessage={onMessage} />}
    </div>}

    {activeTab === "access" && <div className="integration-tab-content">
      <IntegrationRuntimeAccess integration={integration} key={integration.id} onMessage={onMessage} onNavigate={onNavigate} onChanged={onRuntimeChanged} />
      <section className="panel">
        <PanelHeader title="Customer authorization" description="This API inherits customer sign-in from the deployment and applies an action policy to each exposed tool." action={<ConsoleLink path={sectionPath("identity")} onNavigate={onNavigate} className="entity-back-link">Open Identity</ConsoleLink>} />
        <div className="support-route-summary"><span className="settings-icon"><ShieldCheck /></span><span><strong>{identity?.configured && identity.state === "active" ? "Central customer identity is active" : "Customer identity needs setup"}</strong><small>{identity?.configured && identity.state === "active" ? "Review tool permissions below before publishing executable actions." : "Connect and test the deployment OIDC provider before enabling private agent access."}</small></span><Badge color={identity?.configured && identity.state === "active" ? "green" : "amber"}>{identity?.configured && identity.state === "active" ? "Active" : "Setup"}</Badge></div>
      </section>
      <AuthorizationPolicyWorkspace integration={integration} onMessage={onMessage} />
      <details className="panel advanced-details"><summary>Managed credential lifecycle — Advanced</summary><div className="advanced-details-body"><PanelHeader title="Provider management connections" description="Optional provider-owned issuance, rotation, and revocation. These are separate from the runtime API credential above." action={<Button outline onClick={() => onManageAccess(integration)}>Choose managed connections</Button>} />{integrationConnections.map((connection) => <div className="provider-row integration-connection-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.definition?.name ?? "Managed credential service"}{connection.region ? ` · ${connection.region}` : ""}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge></div>)}{integrationConnections.length === 0 && <div className="empty-row">No managed credential provider is attached. Most API-key integrations do not need one.</div>}</div></details>
    </div>}

    {activeTab === "tools" && <div className="integration-tab-content"><IntegrationToolsWorkspace integration={integration} tools={tools} providerManagementConnections={integrationConnections} onMessage={onMessage} onNavigate={onNavigate} /></div>}

    {activeTab === "test" && <IntegrationTestWorkspace key={`${integration.id}:${publishStatus?.current_manifest_hash ?? ""}`} integration={integration} distribution={distribution} onNavigate={onNavigate} />}

    {activeTab === "history" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>Published history is immutable.</strong> Each entry preserves the exact resources, access, and reporting policy used by agents.</span></div><section className="panel"><PanelHeader title="Published history" />{sortedRevisions.map((revision) => <button type="button" className="integration-revision-row" key={revision.id} onClick={() => onInspectRevision(revision)}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? new Date(revision.published_at ?? revision.created_at).toLocaleString() : "Date unavailable"}</small></span><ChevronRight /></button>)}{sortedRevisions.length === 0 && <div className="empty-row">Nothing has been published yet.</div>}</section></div>}
  </>;
}

const packageEcosystemPattern = /^[a-z][a-z0-9._-]{0,63}$/;

function packageSunsetPassed(value?: string) {
  if (!value) return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && timestamp <= Date.now();
}

function packageArtifactCanPublish(artifact: APIPackageArtifact) {
  return (artifact.lifecycle === "draft" || artifact.lifecycle === "active") && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanBind(artifact: APIPackageArtifact) {
  return artifact.lifecycle === "active" && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanPublishForIntegration(artifact: APIPackageArtifact, integration: APIIntegration) {
  return packageArtifactCanPublish(artifact) && (integration.visibility !== "public" || artifact.visibility === "public");
}

function packageLifecycleDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function IntegrationPackagesWorkspace({ integration, onMessage }: { integration: APIIntegration; onMessage: (message: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationPackageBinding[]>([]);
  const [catalog, setCatalog] = useState<APIPackageArtifact[]>([]);
  const [loading, setLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editingArtifact, setEditingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecateOpen, setDeprecateOpen] = useState(false);
  const [deprecatingArtifact, setDeprecatingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecationMessage, setDeprecationMessage] = useState("");
  const [replacementArtifactID, setReplacementArtifactID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [retireOpen, setRetireOpen] = useState(false);
  const [retiringArtifact, setRetiringArtifact] = useState<APIPackageArtifact | null>(null);
  const [retirementMessage, setRetirementMessage] = useState("");
  const [retirementReplacementID, setRetirementReplacementID] = useState("");
  const [publishReleaseOpen, setPublishReleaseOpen] = useState(false);
  const [publishingArtifact, setPublishingArtifact] = useState<APIPackageArtifact | null>(null);
  const [releasePublicAcknowledged, setReleasePublicAcknowledged] = useState(false);
  const [bindOpen, setBindOpen] = useState(false);
  const [selectedReleaseID, setSelectedReleaseID] = useState("");
  const [ecosystem, setEcosystem] = useState("npm");
  const [packageName, setPackageName] = useState("");
  const [packageDescription, setPackageDescription] = useState("");
  const [packageCoordinate, setPackageCoordinate] = useState("");
  const [artifactPURL, setArtifactPURL] = useState("");
  const [releasePURL, setReleasePURL] = useState("");
  const [registryURL, setRegistryURL] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [packageLanguage, setPackageLanguage] = useState("");
  const [packagePlatform, setPackagePlatform] = useState("");
  const [packageVisibility, setPackageVisibility] = useState<APIVisibility>("private");
  const [packagePublicAcknowledged, setPackagePublicAcknowledged] = useState(false);
  const [packageVersion, setPackageVersion] = useState("");
  const [installCommand, setInstallCommand] = useState("");
  const [integrityDigest, setIntegrityDigest] = useState("");
  const [sbomURL, setSBOMURL] = useState("");
  const [provenanceURL, setProvenanceURL] = useState("");
  const ecosystemOptionsID = `package-ecosystems-${integration.id}`;

  const loadPackages = useCallback(async () => {
    setLoading(true);
    const [bindingResult, catalogResult] = await Promise.allSettled([api.integrationPackages(integration.id), api.packageArtifacts()]);
    if (bindingResult.status === "fulfilled") {
      setBindings(bindingResult.value);
      setBindingsUnavailable(false);
    } else {
      setBindings([]);
      setBindingsUnavailable(unavailableConsoleCapability(bindingResult.reason));
    }
    if (catalogResult.status === "fulfilled") {
      const enriched = await Promise.all(catalogResult.value.map(async (artifact) => {
        try {
          const releases = await api.packageReleases(artifact.id);
          return { ...artifact, releases };
        } catch {
          return artifact;
        }
      }));
      setCatalog(enriched);
    } else setCatalog([]);
    setLoading(false);
  }, [integration.id]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadPackages(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadPackages]);

  const publishedReleases = catalog.flatMap((artifact) => {
    if (!packageArtifactCanBind(artifact)) return [];
    return (artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []))
      .filter((release) => integration.visibility !== "public" || release.visibility === "public")
      .map((release) => ({ artifact, release }));
  });
  const replacementCandidates = catalog.filter((artifact) => packageArtifactCanBind(artifact) && (artifact.latest_release || (artifact.releases?.length ?? 0) > 0));
  const ecosystemValid = packageEcosystemPattern.test(ecosystem.trim());

  function resetPackageMetadata() {
    setEcosystem("npm"); setPackageName(""); setPackageDescription(""); setPackageCoordinate(""); setArtifactPURL(""); setRegistryURL(""); setSourceURL(""); setPackageLanguage(""); setPackagePlatform(""); setPackageVisibility("private"); setPackagePublicAcknowledged(false);
  }

  function resetReleaseMetadata() {
    setPackageVersion(""); setReleasePURL(""); setInstallCommand(""); setIntegrityDigest(""); setSBOMURL(""); setProvenanceURL(""); setReleasePublicAcknowledged(false);
  }

  function packageArtifactInput() {
    return { ecosystem: ecosystem.trim().toLowerCase(), name: packageName.trim(), description: packageDescription.trim(), coordinate: packageCoordinate.trim(), purl: artifactPURL.trim(), registry_url: registryURL.trim(), source_url: sourceURL.trim() || undefined, language: packageLanguage.trim() || undefined, platform: packagePlatform.trim() || undefined, visibility: packageVisibility, acknowledge_public: packagePublicAcknowledged };
  }

  function openCreatePackage() {
    resetPackageMetadata();
    setPackageVisibility(integration.visibility);
    resetReleaseMetadata();
    setCreateOpen(true);
  }

  function openEditPackage(artifact: APIPackageArtifact) {
    setEditingArtifact(artifact);
    setEcosystem(artifact.ecosystem); setPackageName(artifact.name); setPackageDescription(artifact.description); setPackageCoordinate(artifact.coordinate); setArtifactPURL(artifact.purl); setRegistryURL(artifact.registry_url); setSourceURL(artifact.source_url ?? ""); setPackageLanguage(artifact.language ?? ""); setPackagePlatform(artifact.platform ?? ""); setPackageVisibility(artifact.visibility); setPackagePublicAcknowledged(false);
    setEditOpen(true);
  }

  function openPublishRelease(artifact: APIPackageArtifact) {
    resetReleaseMetadata();
    setPublishingArtifact(artifact);
    setPublishReleaseOpen(true);
  }

  function openDeprecatePackage(artifact: APIPackageArtifact) {
    setDeprecatingArtifact(artifact); setDeprecationMessage(""); setReplacementArtifactID(""); setSunsetAt(""); setDeprecateOpen(true);
  }

  function openRetirePackage(artifact: APIPackageArtifact) {
    setRetiringArtifact(artifact); setRetirementMessage(artifact.deprecation_message ?? ""); setRetirementReplacementID(artifact.replacement_package_artifact_id ?? ""); setRetireOpen(true);
  }

  async function bindSelectedRelease() {
    if (!selectedReleaseID) return;
    if (!publishedReleases.some(({ release }) => release.id === selectedReleaseID)) {
      onMessage("That release is no longer eligible to bind to this Integration.");
      return;
    }
    setBusy(true);
    try {
      await api.bindIntegrationPackage(integration.id, selectedReleaseID);
      await loadPackages();
      setBindOpen(false);
      setSelectedReleaseID("");
      onMessage("Exact package release bound to this API.");
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package bindings are not available in this deployment yet." : error instanceof APIError ? error.message : "Package release could not be bound.");
    } finally { setBusy(false); }
  }

  function packageFailureMessage(error: unknown, fallback: string) {
    if (unavailableConsoleCapability(error)) return "The SDK and package catalogue is not available in this deployment yet.";
    return error instanceof APIError ? error.message : fallback;
  }

  async function recoverPackageWorkflow(knownArtifact: APIPackageArtifact | null, knownRelease: APIPackageRelease | null, failure: string) {
    let artifact = knownArtifact;
    let release = knownRelease;
    try {
      const artifacts = await api.packageArtifacts();
      artifact = (knownArtifact ? artifacts.find((candidate) => candidate.id === knownArtifact.id) : undefined)
        ?? artifacts.find((candidate) => candidate.ecosystem === ecosystem.trim().toLowerCase() && candidate.coordinate === packageCoordinate.trim())
        ?? artifact;
      if (artifact) {
        const releases = await api.packageReleases(artifact.id);
        artifact = { ...artifact, releases };
        release = release ?? releases.find((candidate) => candidate.version === packageVersion.trim() && candidate.purl === releasePURL.trim()) ?? null;
      }
    } catch {
      // A best-effort refresh must not hide the original create, publish, or bind error.
    }
    await loadPackages();
    if (release && integration.visibility === "public" && release.visibility !== "public") {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID("");
      onMessage(`${failure} The private release was saved, but it cannot be bound to a public Integration; publish a public replacement artifact instead.`);
      return;
    }
    if (release) {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID(release.id);
      setBindOpen(true);
      onMessage(`${failure} The exact release was saved; finish its binding in Bind existing.`);
      return;
    }
    if (artifact) {
      setCreateOpen(false);
      if (integration.visibility === "public" && artifact.visibility !== "public") {
        setPublishReleaseOpen(false);
        setPublishingArtifact(null);
        onMessage(`${failure} The private artifact draft was saved, but it must be made public before it can publish and bind to this public Integration.`);
        return;
      }
      setPublishingArtifact(artifact);
      setReleasePublicAcknowledged(artifact.visibility === "public" && packagePublicAcknowledged);
      setPublishReleaseOpen(true);
      onMessage(`${failure} The reusable artifact draft was saved; review and retry its release.`);
      return;
    }
    onMessage(`${failure} Refreshing the catalogue did not find a saved artifact; correct the form or retry.`);
  }

  async function createPublishAndBindPackage() {
    if (integration.visibility === "public" && packageVisibility !== "public") {
      onMessage("A public Integration can only bind a public package release.");
      return;
    }
    let artifact: APIPackageArtifact | null = null;
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      artifact = await api.createPackageArtifact(packageArtifactInput());
      const published = await api.publishPackageRelease(artifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: artifact.revision, acknowledge_public: packageVisibility === "public" && packagePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setCreateOpen(false);
      resetPackageMetadata(); resetReleaseMetadata();
      onMessage(`${artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(artifact, release, packageFailureMessage(error, "Package could not be created, published, and bound."));
    } finally { setBusy(false); }
  }

  async function publishAndBindExistingArtifact() {
    if (!publishingArtifact) return;
    if (!packageArtifactCanPublishForIntegration(publishingArtifact, integration)) {
      onMessage(integration.visibility === "public" && publishingArtifact.visibility !== "public" ? "A private package artifact cannot publish and bind to a public Integration." : "This package artifact cannot publish another release.");
      return;
    }
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      const published = await api.publishPackageRelease(publishingArtifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: publishingArtifact.revision, acknowledge_public: publishingArtifact.visibility === "public" && releasePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setPublishReleaseOpen(false); setPublishingArtifact(null); resetReleaseMetadata();
      onMessage(`${published.artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(publishingArtifact, release, packageFailureMessage(error, "Package release could not be published and bound."));
    } finally { setBusy(false); }
  }

  async function savePackageEdits() {
    if (!editingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.updatePackageArtifact(editingArtifact.id, { ...packageArtifactInput(), revision: editingArtifact.revision });
      await loadPackages(); setEditOpen(false); setEditingArtifact(null); resetPackageMetadata();
      onMessage(`${updated.name} catalogue metadata updated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package metadata could not be updated."); } finally { setBusy(false); }
  }

  async function deprecatePackage() {
    if (!deprecatingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.deprecatePackageArtifact(deprecatingArtifact.id, { replacement_package_artifact_id: replacementArtifactID || undefined, message: deprecationMessage.trim(), sunset_at: sunsetAt ? new Date(sunsetAt).toISOString() : undefined, revision: deprecatingArtifact.revision });
      await loadPackages(); setDeprecateOpen(false); setDeprecatingArtifact(null);
      onMessage(`${updated.name} marked deprecated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package could not be deprecated."); } finally { setBusy(false); }
  }

  async function retirePackage() {
    if (!retiringArtifact) return;
    setBusy(true);
    try {
      const updated = await api.retirePackageArtifact(retiringArtifact.id, { replacement_package_artifact_id: retirementReplacementID || undefined, message: retirementMessage.trim(), revision: retiringArtifact.revision });
      await loadPackages(); setRetireOpen(false); setRetiringArtifact(null);
      onMessage(`${updated.name} retired. Existing immutable snapshots retain their exact release.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package retirement is not available in this deployment yet." : error instanceof APIError ? error.message : "Package could not be retired.");
    } finally { setBusy(false); }
  }

  async function unbind(binding: APIIntegrationPackageBinding) {
    setBusy(true);
    try {
      await api.unbindIntegrationPackage(integration.id, binding.package_artifact_id);
      setBindings((items) => items.filter((item) => item.package_artifact_id !== binding.package_artifact_id));
      onMessage("Package release removed from this API draft.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package release could not be removed."); } finally { setBusy(false); }
  }

  return <>
    <div className="notice"><Database /><span><strong>Developer artifact catalogue, not a package proxy or verifier.</strong> Registries deliver package bytes; DokoSoko records digest-identified metadata and binds exact releases to compatible API snapshots.</span></div>
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Package binding is not enabled on this deployment.</strong><small>The workspace remains usable and will activate automatically when the package endpoints are installed.</small></span></div>}
    <section className="panel"><PanelHeader title="Bound SDKs & packages" description="Every binding resolves to one exact published release." action={<span className="heading-actions"><Button outline disabled={loading || publishedReleases.length === 0 || bindingsUnavailable} onClick={() => setBindOpen(true)}>Bind existing</Button><Button disabled={bindingsUnavailable} onClick={openCreatePackage}><Plus data-slot="icon" />Add package</Button></span>} />
      {loading ? <div className="empty-row">Loading package catalogue…</div> : bindings.map((binding) => {
        const artifact = binding.artifact ?? catalog.find((candidate) => candidate.id === binding.package_artifact_id);
        const release = binding.release ?? artifact?.releases?.find((candidate) => candidate.id === binding.package_release_id) ?? artifact?.latest_release;
        const replacement = catalog.find((candidate) => candidate.id === artifact?.replacement_package_artifact_id);
        return <div className="provider-row package-binding-row" key={binding.id ?? `${binding.package_artifact_id}:${binding.package_release_id}`}><span className="settings-icon"><Database /></span><span><strong>{artifact?.name ?? binding.package_artifact_id}</strong><small>{artifact?.ecosystem ?? "package"} · {release?.coordinate ?? artifact?.coordinate ?? "—"}@{release?.version ?? binding.package_release_id} · compatible with {integration.version_key}</small>{artifact?.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact?.replacement_package_artifact_id && <small>Replacement: {replacement?.name ?? artifact.replacement_package_artifact_id}</small>}{artifact?.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color="green">exact release</Badge>{artifact && artifact.lifecycle !== "active" && <Badge color={artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge>}</span><span className="table-actions">{release?.digest && <code title={release.digest}>{release.digest.slice(0, 18)}…</code>}<button type="button" className="more" disabled={busy} aria-label={`Unbind ${artifact?.name ?? "package"}`} title="Unbind package" onClick={() => unbind(binding)}><XCircle /></button></span></div>;
      })}
      {!loading && bindings.length === 0 && <div className="empty-row">No SDK or package release is bound to this API.</div>}
    </section>
    <section className="panel"><PanelHeader title="Package catalogue" description="Draft artifact metadata can be edited. Publishing the first release freezes all artifact metadata; later corrections require a replacement artifact. Exact releases are always immutable." />
      {catalog.map((artifact) => {
        const replacement = catalog.find((candidate) => candidate.id === artifact.replacement_package_artifact_id);
        const releases = artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []);
        return <div className="provider-row package-binding-row" key={artifact.id}><span className="settings-icon"><Database /></span><span><strong>{artifact.name}</strong><small>{artifact.ecosystem} · {artifact.coordinate} · {releases.length} published release{releases.length === 1 ? "" : "s"}</small><small>Reusable PURL: {artifact.purl}</small>{artifact.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact.replacement_package_artifact_id && <small>Replacement: {replacement ? `${replacement.name} · ${replacement.coordinate}` : artifact.replacement_package_artifact_id}</small>}{artifact.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color={artifact.lifecycle === "active" ? "green" : artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge><Badge color={artifact.visibility === "public" ? "blue" : "zinc"}>{artifact.visibility}</Badge></span><span className="table-actions">{artifact.lifecycle === "draft" && <Button outline disabled={busy} onClick={() => openEditPackage(artifact)}>Edit draft</Button>}{packageArtifactCanPublishForIntegration(artifact, integration) && <Button outline disabled={busy} onClick={() => openPublishRelease(artifact)}>Publish release</Button>}{artifact.lifecycle === "active" && <Button outline disabled={busy} onClick={() => openDeprecatePackage(artifact)}>Deprecate</Button>}{artifact.lifecycle === "deprecated" && <Button outline disabled={busy} onClick={() => openRetirePackage(artifact)}>Retire</Button>}</span></div>;
      })}
      {!loading && catalog.length === 0 && <div className="empty-row">No reusable package artifacts have been created.</div>}
    </section>
    <Dialog open={bindOpen} onClose={setBindOpen} title="Bind a published package" description="Choose an exact release from an active, non-sunset artifact. Deprecated, retired, and sunset entries are excluded." actions={<><Button outline onClick={() => setBindOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedReleaseID} onClick={bindSelectedRelease}>{busy ? "Binding…" : "Bind release"}</Button></>}><label className="auth-field"><span>Package release</span><select value={selectedReleaseID} onChange={(event) => setSelectedReleaseID(event.target.value)}><option value="">Select an exact release</option>{publishedReleases.map(({ artifact, release }) => <option key={release.id} value={release.id}>{artifact.ecosystem} · {artifact.name}@{release.version}</option>)}</select></label></Dialog>
    <Dialog open={createOpen} onClose={setCreateOpen} title="Add SDK or package" description="Create a reusable artifact, publish one exact release, then bind it to this API. A saved draft remains recoverable if a later step fails." actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (packageVisibility === "public" && !packagePublicAcknowledged) || (integration.visibility === "public" && packageVisibility !== "public")} onClick={createPublishAndBindPackage}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} placeholder="npm or vendor.ecosystem" /><small>Choose a suggestion or enter a lowercase vendor ecosystem identifier.</small></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} placeholder="Vendor JavaScript SDK" /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} placeholder="What this developer artifact provides." /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} placeholder="@vendor/sdk" /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk" /><small>Stable package identity without a version, query, or fragment.</small></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} placeholder="https://registry.npmjs.org/…" /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder="https://github.com/vendor/sdk" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} placeholder="TypeScript" /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} placeholder="Node.js 22+" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk@3.2.1" /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>Required and syntax-validated. DokoSoko stores the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public makes this metadata eligible for public Integration discovery only after an exact public release is bound, the Integration is published, and Public MCP is enabled. It does not create a standalone public package catalogue.</small></label>
        {integration.visibility === "public" && packageVisibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>A public Integration requires a public package release.</strong><small>Choose Public before publishing and binding this package.</small></span></div>}
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public package and release metadata becomes discoverable through Public MCP only after public binding and Integration publication.</span></label>}
      </div>
    </Dialog>
    <Dialog open={publishReleaseOpen} onClose={setPublishReleaseOpen} title={`Publish release for ${publishingArtifact?.name ?? "package"}`} description={`Publish and bind a new immutable release of ${publishingArtifact?.purl ?? "this reusable artifact"}. If binding fails, the saved release remains available through Bind existing.`} actions={<><Button outline onClick={() => setPublishReleaseOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !publishingArtifact || !packageArtifactCanPublishForIntegration(publishingArtifact, integration) || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (publishingArtifact.visibility === "public" && !releasePublicAcknowledged)} onClick={publishAndBindExistingArtifact}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        {publishingArtifact && !packageArtifactCanPublish(publishingArtifact) && <div className="capability-unavailable"><TriangleAlert /><span><strong>This artifact cannot publish another release.</strong><small>Deprecated, retired, and sunset artifacts are immutable lifecycle records.</small></span></div>}
        {publishingArtifact && packageArtifactCanPublish(publishingArtifact) && integration.visibility === "public" && publishingArtifact.visibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>This private artifact cannot bind to a public Integration.</strong><small>Create a public replacement artifact instead.</small></span></div>}
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder={`${publishingArtifact?.purl ?? "pkg:npm/%40vendor/sdk"}@3.2.1`} /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>DokoSoko records the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        {publishingArtifact?.visibility === "public" && <><div className="notice"><ShieldCheck /><span><strong>Public discovery remains gated.</strong> This exact release is only eligible for discovery after public binding, Integration publication, and Public MCP enablement.</span></div><label className="auth-check"><input type="checkbox" checked={releasePublicAcknowledged} onChange={(event) => setReleasePublicAcknowledged(event.target.checked)} /><span>I explicitly confirm this exact release may become discoverable through Public MCP after those publication gates are satisfied.</span></label></>}
      </div>
    </Dialog>
    <Dialog open={editOpen} onClose={setEditOpen} title="Edit draft package" description="Draft catalogue metadata may be replaced until the first exact release is published." actions={<><Button outline onClick={() => setEditOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !editingArtifact || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || (packageVisibility === "public" && !packagePublicAcknowledged)} onClick={savePackageEdits}>{busy ? "Saving…" : "Save draft"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} /></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public eligibility still requires an exact public binding, a published public Integration, and Public MCP.</small></label>
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public metadata remains gated by public binding, Integration publication, and Public MCP.</span></label>}
      </div>
    </Dialog>
    <Dialog open={deprecateOpen} onClose={setDeprecateOpen} title={`Deprecate ${deprecatingArtifact?.name ?? "package"}`} description="Deprecation immediately blocks new releases, new bindings, and candidate publication. Historical published snapshots remain readable; an optional sunset is migration guidance only." actions={<><Button outline onClick={() => setDeprecateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !deprecatingArtifact || !deprecationMessage.trim()} onClick={deprecatePackage}>{busy ? "Deprecating…" : "Deprecate"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Deprecation message</span><textarea value={deprecationMessage} onChange={(event) => setDeprecationMessage(event.target.value)} placeholder="Explain the migration path." /></label><label className="auth-field"><span>Replacement artifact</span><select value={replacementArtifactID} onChange={(event) => setReplacementArtifactID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== deprecatingArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label><label className="auth-field"><span>Optional sunset</span><input type="datetime-local" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /><small>Guidance for migration timing; deprecation enforcement is immediate.</small></label></div>
    </Dialog>
    <Dialog open={retireOpen} onClose={setRetireOpen} title={`Retire ${retiringArtifact?.name ?? "package"}`} description="Retirement permanently prevents new releases and bindings. Existing immutable snapshots retain their exact release and lifecycle warning." actions={<><Button outline onClick={() => setRetireOpen(false)}>Cancel</Button><Button color="red" disabled={busy || !retiringArtifact || !retirementMessage.trim()} onClick={retirePackage}>{busy ? "Retiring…" : "Retire package"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Retirement message</span><textarea value={retirementMessage} onChange={(event) => setRetirementMessage(event.target.value)} placeholder="Explain why this artifact is retired and where consumers should migrate." /></label><label className="auth-field"><span>Replacement artifact</span><select value={retirementReplacementID} onChange={(event) => setRetirementReplacementID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== retiringArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label></div>
    </Dialog>
    <datalist id={ecosystemOptionsID}><option value="npm" /><option value="pypi" /><option value="maven" /><option value="nuget" /><option value="go" /><option value="docker" /><option value="oci" /></datalist>
  </>;
}

function AuthorizationPolicyWorkspace({ integration, onMessage }: { integration: APIIntegration; onMessage: (message: string) => void }) {
  const [definitions, setDefinitions] = useState<APIGrantDefinition[]>([]);
  const [points, setPoints] = useState<APIAuthorizationPoint[]>([]);
  const [catalogUnavailable, setCatalogUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [grantOpen, setGrantOpen] = useState(false);
  const [editingGrant, setEditingGrant] = useState<APIGrantDefinition | null>(null);
  const [grantKey, setGrantKey] = useState("");
  const [grantName, setGrantName] = useState("");
  const [grantDescription, setGrantDescription] = useState("");
  const [grantRisk, setGrantRisk] = useState<APIGrantDefinition["risk"]>("low");
  const [grantState, setGrantState] = useState<APIGrantDefinition["state"]>("active");
  const [pointOpen, setPointOpen] = useState(false);
  const [editingPoint, setEditingPoint] = useState<APIAuthorizationPoint | null>(null);
  const [pointKey, setPointKey] = useState("");
  const [pointName, setPointName] = useState("");
  const [pointDescription, setPointDescription] = useState("");
  const [pointAction, setPointAction] = useState<APIAuthorizationPoint["action_type"]>("read");
  const [pointGrants, setPointGrants] = useState<string[]>([]);
  const [pointConfirmation, setPointConfirmation] = useState(false);
  const [pointTTL, setPointTTL] = useState("300");
  const [pointState, setPointState] = useState<APIAuthorizationPoint["state"]>("draft");
  const integrationID = integration.id;

  const loadAuthorization = useCallback(async () => {
    setPoints([]);
    const pointRequest = integrationID ? api.authorizationPoints(integrationID) : Promise.resolve([] as APIAuthorizationPoint[]);
    const [definitionResult, pointResult] = await Promise.allSettled([api.grantDefinitions(), pointRequest]);
    if (definitionResult.status === "fulfilled") setDefinitions(definitionResult.value);
    if (pointResult.status === "fulfilled") setPoints(pointResult.value);
    const unavailable = (definitionResult.status === "rejected" && unavailableConsoleCapability(definitionResult.reason)) || (pointResult.status === "rejected" && unavailableConsoleCapability(pointResult.reason));
    setCatalogUnavailable(unavailable);
  }, [integrationID]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadAuthorization(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadAuthorization]);

  const registeredKeys = new Set(definitions.filter((definition) => definition.state === "active").map((definition) => definition.key));

  function openGrant(value?: APIGrantDefinition) {
    setEditingGrant(value ?? null); setGrantKey(value?.key ?? ""); setGrantName(value?.display_name ?? ""); setGrantDescription(value?.description ?? ""); setGrantRisk(value?.risk ?? "low"); setGrantState(value?.state ?? "active"); setGrantOpen(true);
  }

  async function saveGrant() {
    setBusy(true);
    try {
      const input = { key: grantKey.trim(), display_name: grantName.trim(), description: grantDescription.trim(), risk: grantRisk, state: grantState };
      if (editingGrant) await api.updateGrantDefinition(editingGrant.id, { ...input, revision: editingGrant.revision }); else await api.createGrantDefinition(input);
      await loadAuthorization(); setGrantOpen(false); onMessage(editingGrant ? "Grant definition updated." : "Grant registered for policy use.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Grant definition could not be saved."); } finally { setBusy(false); }
  }

  function openPoint(value?: APIAuthorizationPoint) {
    setEditingPoint(value ?? null); setPointKey(value?.key ?? ""); setPointName(value?.name ?? ""); setPointDescription(value?.description ?? ""); setPointAction(value?.action_type ?? "read"); setPointGrants(value?.required_grants ?? []); setPointConfirmation(value?.confirmation_required ?? false); setPointTTL(String(value?.decision_ttl_seconds ?? 300)); setPointState(value?.state ?? "draft"); setPointOpen(true);
  }

  async function savePoint() {
    setBusy(true);
    try {
      const input = { key: pointKey.trim(), name: pointName.trim(), description: pointDescription.trim(), action_type: pointAction, required_grants: pointGrants, confirmation_required: pointAction === "destructive" ? true : pointConfirmation, decision_ttl_seconds: Number(pointTTL), state: pointState };
      if (editingPoint) await api.updateAuthorizationPoint(integration.id, editingPoint.id, { ...input, revision: editingPoint.revision }); else await api.createAuthorizationPoint(integration.id, input);
      await loadAuthorization(); setPointOpen(false); onMessage(editingPoint ? "Action policy updated." : "Action policy created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Action policy could not be saved."); } finally { setBusy(false); }
  }

  return <>
    <SectionHeader title="Action policies" description={`Define the exact actions and grants ${integration.display_name} tools require.`} />
    <div className="notice authorization-policy-notice"><ShieldCheck /><span><strong>Policies do not authenticate customers.</strong> The configured identity provider resolves the customer first; these exact grant requirements narrow which published tools the customer may call.</span></div>
    {catalogUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Authorization catalogue unavailable.</strong><small>Existing tool policies remain visible, but grant and action-policy changes cannot be saved by this deployment.</small></span></div>}
    <section className="panel"><PanelHeader title="API action policies" description="Each tool binding pins one exact active policy revision so publication and runtime checks fail closed." action={<Button disabled={catalogUnavailable || definitions.every((definition) => definition.state !== "active")} onClick={() => openPoint()}><Plus data-slot="icon" />Add policy</Button>} />{points.map((point) => <div className="provider-row authorization-point-row" key={point.id}><span className="settings-icon"><ShieldCheck /></span><span><strong>{point.name}</strong><small><code>{point.key}</code> · {point.required_grants.join(", ") || "no grants"} · TTL {point.decision_ttl_seconds}s</small></span><span className="tool-badges"><Badge color={point.action_type === "destructive" ? "red" : point.action_type === "write" ? "amber" : "blue"}>{point.action_type}</Badge>{point.confirmation_required && <Badge color="violet">confirmation</Badge>}<Badge color={point.state === "active" ? "green" : "zinc"}>{point.state}</Badge></span><Button outline onClick={() => openPoint(point)}>Edit</Button></div>)}{points.length === 0 && <div className="empty-row">No action policy has been configured for {integration.display_name}. Register a grant in Advanced, then add the first policy.</div>}</section>
    <details className="panel advanced-details"><summary>Deployment grant registry — Advanced</summary><div className="advanced-details-body"><PanelHeader title="Grant registry" description="Deployment-owned names that your authorization API may return. Registering a name never grants access." action={<span className="heading-actions"><Badge color="violet">{definitions.length} grants</Badge><Button disabled={catalogUnavailable} onClick={() => openGrant()}><Plus data-slot="icon" />Register grant</Button></span>} />{definitions.map((definition) => <div className="provider-row grant-definition-row" key={definition.id}><span className="settings-icon"><KeyRound /></span><span><strong>{definition.display_name}</strong><small><code>{definition.key}</code> · {definition.description || "No description"}</small></span><span className="tool-badges"><Badge color={definition.risk === "critical" || definition.risk === "high" ? "red" : definition.risk === "medium" ? "amber" : "zinc"}>{definition.risk}</Badge><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></span><Button outline onClick={() => openGrant(definition)}>Edit</Button></div>)}{definitions.length === 0 && <div className="empty-row">Register the first grant returned by your authorization API.</div>}</div></details>
    <Dialog open={grantOpen} onClose={setGrantOpen} title={editingGrant ? "Edit grant definition" : "Register grant"} description="Grant keys are stable contract identifiers. Editing never grants a user access." actions={<><Button outline onClick={() => setGrantOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !grantKey.trim() || !grantName.trim()} onClick={saveGrant}>{busy ? "Saving…" : "Save grant"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Grant key</span><input disabled={Boolean(editingGrant)} value={grantKey} onChange={(event) => setGrantKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Display name</span><input value={grantName} onChange={(event) => setGrantName(event.target.value)} placeholder="Read customers" /></label><label className="auth-field"><span>Description</span><textarea value={grantDescription} onChange={(event) => setGrantDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Risk</span><select value={grantRisk} onChange={(event) => setGrantRisk(event.target.value as APIGrantDefinition["risk"])}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label><label className="auth-field"><span>State</span><select value={grantState} onChange={(event) => setGrantState(event.target.value as APIGrantDefinition["state"])}><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></div></Dialog>
    <Dialog open={pointOpen} onClose={setPointOpen} title={editingPoint ? "Edit action policy" : "Add action policy"} description="Configure a declarative API action policy. There is deliberately no hook URL or credential field." actions={<><Button outline onClick={() => setPointOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !pointKey.trim() || !pointName.trim() || pointGrants.some((grant) => !registeredKeys.has(grant))} onClick={savePoint}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Policy key</span><input disabled={Boolean(editingPoint)} value={pointKey} onChange={(event) => setPointKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Name</span><input value={pointName} onChange={(event) => setPointName(event.target.value)} placeholder="Read customer" /></label></div><label className="auth-field"><span>Description</span><textarea value={pointDescription} onChange={(event) => setPointDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Action type</span><select value={pointAction} onChange={(event) => { const value = event.target.value as APIAuthorizationPoint["action_type"]; setPointAction(value); if (value === "destructive") setPointConfirmation(true); }}><option value="read">Read</option><option value="write">Write</option><option value="destructive">Destructive</option></select></label><label className="auth-field"><span>Decision TTL (seconds)</span><input type="number" min={0} max={3600} value={pointTTL} onChange={(event) => setPointTTL(event.target.value)} /></label></div><fieldset className="catalog-settings-section"><legend>Required registered grants</legend>{definitions.map((definition) => { const selected = pointGrants.includes(definition.key); return <label className="compact-check" key={definition.id}><input type="checkbox" disabled={definition.state !== "active" && !selected} checked={selected} onChange={() => setPointGrants((current) => current.includes(definition.key) ? current.filter((key) => key !== definition.key) : [...current, definition.key])} /><span>{definition.display_name}<small>{definition.key} · {definition.risk}{definition.state === "deprecated" ? " · deprecated (remove before saving)" : ""}</small></span></label>; })}</fieldset><label className="compact-check"><input type="checkbox" disabled={pointAction === "destructive"} checked={pointConfirmation || pointAction === "destructive"} onChange={(event) => setPointConfirmation(event.target.checked)} /><span>Require explicit confirmation for this action</span></label><label className="auth-field"><span>State</span><select value={pointState} onChange={(event) => setPointState(event.target.value as APIAuthorizationPoint["state"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></Dialog>
  </>;
}

type IntegrationToolBindingSelection = { revision: number; authorizationPointID: string; authorizationPointRevision: number };

function integrationToolBindingSelectionSignature(selection: Record<string, IntegrationToolBindingSelection>) {
  return JSON.stringify(Object.entries(selection).sort(([left], [right]) => left.localeCompare(right)));
}

function IntegrationToolsWorkspace({ integration, tools, providerManagementConnections, onMessage, onNavigate }: { integration: APIIntegration; tools: APITool[]; providerManagementConnections: APIAccessConnection[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationToolBinding[]>([]);
  const [authorizationPoints, setAuthorizationPoints] = useState<APIAuthorizationPoint[]>([]);
  const [bindingSelection, setBindingSelection] = useState<Record<string, IntegrationToolBindingSelection>>({});
  const [savedSignature, setSavedSignature] = useState<string | null>(null);
  const [bindingsLoading, setBindingsLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [bindingBusy, setBindingBusy] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [attachOpen, setAttachOpen] = useState(false);
  const [attachToolID, setAttachToolID] = useState("");
  const [attachPointID, setAttachPointID] = useState("");

  const activePoints = authorizationPoints.filter((point) => point.state === "active");
  const availableTools = tools.filter((tool) => tool.state === "published" && !tool.upstream_drifted && !bindingSelection[tool.id] && toolCanAttachToIntegration(tool, integration.id));
  const dirty = savedSignature !== null && integrationToolBindingSelectionSignature(bindingSelection) !== savedSignature;

  useEffect(() => {
    let cancelled = false;
    Promise.all([api.integrationToolBindings(integration.id), api.authorizationPoints(integration.id)]).then(([values, points]) => {
      if (cancelled) return;
      const next = Object.fromEntries(values.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(values);
      setAuthorizationPoints(points);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      setBindingsLoading(false);
      setBindingsUnavailable(false);
    }).catch(() => {
      if (cancelled) return;
      setBindings([]);
      setAuthorizationPoints([]);
      setBindingSelection({});
      setSavedSignature(null);
      setBindingsLoading(false);
      setBindingsUnavailable(true);
    });
    return () => { cancelled = true; };
  }, [integration.id, loadAttempt]);

  function retryBindings() {
    setBindingsLoading(true);
    setBindingsUnavailable(false);
    setLoadAttempt((value) => value + 1);
  }

  function openAttachDialog(toolID?: string) {
    const defaultTool = availableTools.find((tool) => tool.id === toolID) ?? availableTools[0];
    const defaultPoint = activePoints[0];
    setAttachToolID(defaultTool?.id ?? "");
    setAttachPointID(defaultPoint?.id ?? "");
    setAttachOpen(true);
  }

  function attachTool() {
    const tool = tools.find((candidate) => candidate.id === attachToolID && candidate.state === "published" && !candidate.upstream_drifted && toolCanAttachToIntegration(candidate, integration.id));
    const point = activePoints.find((candidate) => candidate.id === attachPointID);
    if (!tool || !point) return;
    setBindingSelection((current) => ({ ...current, [tool.id]: { revision: tool.revision, authorizationPointID: point.id, authorizationPointRevision: point.revision } }));
    setAttachOpen(false);
  }

  function removeBinding(toolID: string) {
    setBindingSelection((current) => {
      const next = { ...current };
      delete next[toolID];
      return next;
    });
	requestAnimationFrame(() => document.getElementById("save-api-bindings")?.focus());
  }

  function selectAuthorizationPoint(toolID: string, pointID: string) {
    const point = activePoints.find((candidate) => candidate.id === pointID);
    if (!point) return;
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], authorizationPointID: point.id, authorizationPointRevision: point.revision } } : current);
  }

  function selectCurrentToolRevision(toolID: string, revision: number) {
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], revision } } : current);
  }

  function resolveBinding(toolID: string, selection: IntegrationToolBindingSelection) {
    const tool = tools.find((candidate) => candidate.id === toolID) ?? bindings.find((binding) => binding.tool_id === toolID)?.tool;
    const point = authorizationPoints.find((candidate) => candidate.id === selection.authorizationPointID);
    const toolCurrent = Boolean(tool && tool.state === "published" && !tool.upstream_drifted && tool.revision === selection.revision && toolCanAttachToIntegration(tool, integration.id));
    const pointCurrent = Boolean(point && point.state === "active" && point.revision === selection.authorizationPointRevision);
    const issues = [
      !tool ? "tool missing" : !toolCanAttachToIntegration(tool, integration.id) ? "owned by another API" : tool.state !== "published" ? `tool ${tool.state}` : tool.upstream_drifted ? "schema drift" : tool.revision !== selection.revision ? `tool is now r${tool.revision}` : "",
      !point ? "authorization point missing" : point.state !== "active" ? `authorization ${point.state}` : point.revision !== selection.authorizationPointRevision ? `authorization is now r${point.revision}` : "",
    ].filter(Boolean);
    return { tool, point, current: toolCurrent && pointCurrent, issues };
  }

  async function saveBindings() {
    setBindingBusy(true);
    try {
      const value = await api.setIntegrationToolBindings(integration.id, Object.entries(bindingSelection).map(([tool_id, selection]) => ({ tool_id, revision: selection.revision, authorization_point_id: selection.authorizationPointID, authorization_point_revision: selection.authorizationPointRevision })));
      const next = Object.fromEntries(value.items.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(value.items);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      onMessage(value.items.length === 0 ? "All tool bindings cleared from this API draft." : `${value.items.length} exact tool revision${value.items.length === 1 ? "" : "s"} bound to this API draft.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Exact API tool bindings are not enabled in this deployment yet." : error instanceof APIError ? error.message : "Tool bindings could not be saved.");
    } finally { setBindingBusy(false); }
  }

  const selectedBindings = Object.entries(bindingSelection);
  const staleBindingCount = selectedBindings.filter(([toolID, selection]) => !resolveBinding(toolID, selection).current).length;
  const boundToolIDs = new Set(selectedBindings.map(([toolID]) => toolID));
  const toolGroups = partitionIntegrationTools(tools, boundToolIDs, integration.id);
  const apiOwnedToolIDs = new Set(toolGroups.apiOwned.map((tool) => tool.id));
  const commonToolIDs = new Set(toolGroups.attachedCommon.map((tool) => tool.id));
  const apiOwnedBindings = selectedBindings.filter(([toolID]) => apiOwnedToolIDs.has(toolID));
  const commonBindings = selectedBindings.filter(([toolID]) => commonToolIDs.has(toolID));
  const invalidBindings = selectedBindings.filter(([toolID]) => !apiOwnedToolIDs.has(toolID) && !commonToolIDs.has(toolID));
  const unboundAPITools = toolGroups.apiOwned.filter((tool) => !boundToolIDs.has(tool.id));
  const availableAPITools = availableTools.filter((tool) => toolIsOwnedByIntegration(tool, integration.id));
  const availableCommonTools = availableTools.filter(toolIsCommon);
  const reviewedDocumentation = integration.resources?.find((resource) => resource.kind === "documentation" && Boolean(resource.resolved_revision));
  const apiAdminConnection = providerManagementConnections.find((connection) => {
    if (connection.state !== "active") return false;
    const operationKeys = Object.keys(connection.definition?.operations ?? {}).map((key) => key.toLowerCase());
    return operationKeys.some((key) => /(credential|api[_-]?key)/.test(key) && /(create|issue|rotate|revoke)/.test(key));
  });
  const configuredAdminConnection = providerManagementConnections.find((connection) => connection.state === "active");
  const configuredEnvironmentVariable = typeof apiAdminConnection?.config.environment_variable === "string" ? apiAdminConnection.config.environment_variable : "";
  const familyEnvironmentVariable = `${integration.family_key.toUpperCase().replace(/[^A-Z0-9]+/g, "_").replace(/^_+|_+$/g, "").replace(/_API$/, "") || "SERVICE"}_API_KEY`;
  const adminEnvironmentVariable = configuredEnvironmentVariable === "SERVICE_API_KEY" || apiAdminConnection?.config.credential_scope === "shared" || apiAdminConnection?.config.shared === true ? "SERVICE_API_KEY" : familyEnvironmentVariable;

  const renderBindingRows = (entries: Array<[string, IntegrationToolBindingSelection]>) => entries.map(([toolID, selection]) => {
    const resolution = resolveBinding(toolID, selection);
    const tool = resolution.tool;
    const pointCurrent = resolution.point?.state === "active" && resolution.point.revision === selection.authorizationPointRevision;
    return <div className={`integration-tool-binding-row ${resolution.current ? "" : "stale"}`} key={toolID}>
      <span className="settings-icon">{tool?.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span>
      <span className="integration-tool-binding-main">{tool ? <EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink> : <strong>{toolID}</strong>}<small>pinned tool revision {selection.revision}{tool ? ` · ${tool.backend_kind === "mcp" ? "MCP" : tool.http_method}` : ""}</small></span>
      <label className="tool-binding-action"><span className="sr-only">Authorization point for {tool ? `${tool.namespace}.${tool.name}` : toolID}</span><select disabled={bindingsLoading || bindingBusy} aria-label={`Authorization point for ${tool ? `${tool.namespace}.${tool.name}` : toolID}`} value={pointCurrent ? selection.authorizationPointID : ""} onChange={(event) => selectAuthorizationPoint(toolID, event.target.value)}>{!pointCurrent && <option value="" disabled>Choose a current authorization point</option>}{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · r{point.revision}</option>)}</select><small>pinned authorization revision {selection.authorizationPointRevision}</small></label>
      <span className="tool-badges"><Badge color={resolution.current ? "green" : "red"}>{resolution.current ? "Current" : "Stale / unresolved"}</Badge>{tool?.upstream_drifted && <Badge color="red">schema drift</Badge>}<small className="binding-issue">{resolution.issues.join(" · ")}</small></span>
      <span className="binding-row-actions">{tool && tool.state === "published" && !tool.upstream_drifted && tool.revision !== selection.revision && <Button outline disabled={bindingsLoading || bindingBusy} onClick={() => selectCurrentToolRevision(toolID, tool.revision)}>Use r{tool.revision}</Button>}<Button outline disabled={bindingsLoading || bindingBusy} aria-label={`Remove ${tool ? `${tool.namespace}.${tool.name}` : toolID} from this API draft`} onClick={() => removeBinding(toolID)}>Remove</Button></span>
    </div>;
  });

  return <div className="integration-tab-content">
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Exact tool binding is unavailable.</strong><small>The current API bindings and authorization points could not be loaded. No changes can be saved.</small></span><Button outline onClick={retryBindings}>Retry</Button></div>}
    {activePoints.length === 0 && !bindingsLoading && !bindingsUnavailable && <div className="capability-unavailable"><ShieldCheck /><span><strong>Create an active action policy first.</strong><small>Every exposed tool must pin an exact authorization policy revision.</small></span><ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">Open Access</ConsoleLink></div>}
    <section className="panel integration-tool-bindings">
      <PanelHeader title="Built-in tools" description="DokoSoko exposes these API-scoped capabilities automatically when their reviewed source configuration is ready. They are not custom Tool records and do not need manual attachment." />
      <div className="provider-row"><span className="settings-icon"><BookOpen /></span><span><strong>Knowledge</strong><small><code>{integration.family_key}.knowledge.search</code> · {reviewedDocumentation ? `grounded in ${reviewedDocumentation.name}` : "requires attached reviewed documentation"}</small></span><Badge color={reviewedDocumentation ? "green" : "amber"}>{reviewedDocumentation ? "Automatic" : "Unavailable"}</Badge>{!reviewedDocumentation && <ConsoleLink path={integrationPath(integration.id, "documentation")} onNavigate={onNavigate} className="entity-back-link">Add documentation</ConsoleLink>}</div>
      <div className="provider-row"><span className="settings-icon"><KeyRound /></span><span><strong>API Admin</strong><small>{apiAdminConnection ? `${apiAdminConnection.name} · returns ${adminEnvironmentVariable}` : configuredAdminConnection ? `${configuredAdminConnection.name} does not declare credential issue, rotate, or revoke operations` : "requires an active Advanced provider-management connection"}</small></span><Badge color={apiAdminConnection ? "green" : "amber"}>{apiAdminConnection ? "Automatic" : "Unavailable"}</Badge>{!apiAdminConnection && <ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">Open Access Advanced</ConsoleLink>}</div>
    </section>
    <section className="panel integration-tool-summary">
      <PanelHeader title="Tools for this API" description="API-owned tools stay with this API. Common tools are reusable deployment capabilities attached here at an exact revision." action={<span className="heading-actions">{dirty && <Badge color="amber">Unsaved changes</Badge>}<ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link">Open common catalog</ConsoleLink><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />Create API tool</Button><Button disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />Attach tool</Button></span>} />
      <dl className="compact-metrics integration-tool-scope-summary"><div className="compact-metric"><dt>API owned</dt><dd><strong>{toolGroups.apiOwned.length}</strong><small>{apiOwnedBindings.length} attached</small></dd></div><div className="compact-metric"><dt>Common</dt><dd><strong>{commonBindings.length}</strong><small>attached here</small></dd></div><div className="compact-metric"><dt>Authorization</dt><dd><strong>{activePoints.length}</strong><small>active polic{activePoints.length === 1 ? "y" : "ies"}</small></dd></div></dl>
      <div className="panel-footer-actions"><small>{bindingsLoading ? "Loading current API configuration…" : `${selectedBindings.length} selected · ${bindings.length} currently saved${staleBindingCount > 0 ? ` · ${staleBindingCount} stale or unresolved` : ""}`}</small><Button id="save-api-bindings" disabled={bindingsLoading || bindingsUnavailable || bindingBusy || staleBindingCount > 0 || !dirty} onClick={saveBindings}>{bindingBusy ? "Saving…" : "Save API bindings"}</Button></div>
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title="API tools" description={`Definitions owned by ${integration.display_name}. They cannot be attached to another API.`} action={<span className="heading-actions"><Badge color="violet">{toolGroups.apiOwned.length} owned</Badge><Button color="indigo" onClick={() => onNavigate(integrationToolBuilderPath(integration.id))}><Plus data-slot="icon" />Create API tool</Button></span>} />
      {renderBindingRows(apiOwnedBindings)}
      {unboundAPITools.map((tool) => <div className="provider-row api-owned-tool-row" key={tool.id}><span className="settings-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>Owned by this API · revision {tool.revision}</small></span><Badge color={tool.state === "published" && !tool.upstream_drifted ? "green" : tool.upstream_drifted ? "red" : "amber"}>{tool.upstream_drifted ? "Drifted" : tool.state}</Badge><Button outline disabled={bindingsLoading || bindingBusy || activePoints.length === 0 || !availableAPITools.some((candidate) => candidate.id === tool.id)} onClick={() => openAttachDialog(tool.id)}>Attach</Button></div>)}
      {!bindingsLoading && toolGroups.apiOwned.length === 0 && <div className="empty-row">No tool definition is owned by this API. Common tools can still be attached below.</div>}
      {bindingsLoading && <div className="empty-row">Loading API-owned tools…</div>}
    </section>
    <section className="panel integration-tool-bindings">
      <PanelHeader title="Attached common tools" description="Reusable deployment tools explicitly attached to this API draft." action={<Button outline disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableCommonTools.length === 0} onClick={() => openAttachDialog()}><Plus data-slot="icon" />Attach common tool</Button>} />
      {renderBindingRows(commonBindings)}
      {!bindingsLoading && commonBindings.length === 0 && <div className="empty-row">No common tool is attached to this API.</div>}
      {bindingsLoading && <div className="empty-row">Loading common tool bindings…</div>}
    </section>
    {invalidBindings.length > 0 && <details className="panel advanced-details"><summary>Bindings that need review ({invalidBindings.length})</summary><div className="advanced-details-body integration-tool-bindings">{renderBindingRows(invalidBindings)}</div></details>}
    <Dialog open={attachOpen} onClose={setAttachOpen} title="Attach published tool" description="Choose one deployment tool and pin it to one active authorization-point revision for this API draft." actions={<><Button outline disabled={bindingBusy} onClick={() => setAttachOpen(false)}>Cancel</Button><Button color="indigo" disabled={bindingBusy || !attachToolID || !attachPointID} onClick={attachTool}>Attach tool</Button></>}>
      <div className="auth-form compact-form">
        <label className="auth-field"><span>Published tool</span><select value={attachToolID} onChange={(event) => setAttachToolID(event.target.value)}><option value="" disabled>No eligible tool selected</option>{availableAPITools.length > 0 && <optgroup label="Owned by this API">{availableAPITools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} · r{tool.revision}</option>)}</optgroup>}{availableCommonTools.length > 0 && <optgroup label="Common tools">{availableCommonTools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} · r{tool.revision}</option>)}</optgroup>}</select><small>Only common tools and tools owned by this API are eligible. Draft, retired, drifted, or foreign API tools fail closed.</small></label>
        <label className="auth-field"><span>Authorization point</span><select value={attachPointID} onChange={(event) => setAttachPointID(event.target.value)}><option value="" disabled>No active point selected</option>{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · {point.action_type} · r{point.revision}</option>)}</select><small>The API pins this exact revision; later authorization changes make the binding stale until reviewed.</small></label>
      </div>
    </Dialog>
  </div>;
}
function IntegrationTestWorkspace({ integration, distribution, onNavigate }: { integration: APIIntegration; distribution: Distribution | null; onNavigate: (path: string) => void }) {
  const [preflight, setPreflight] = useState<APIIntegrationPreflight | null>(null);
  const [running, setRunning] = useState(false);
  const [preflightError, setPreflightError] = useState("");

  async function runPreflight() {
    setRunning(true);
    setPreflightError("");
    try {
      setPreflight(await api.preflightIntegration(integration.id));
    } catch (error) {
      setPreflight(null);
      setPreflightError(error instanceof APIError ? error.message : "Server preflight could not run.");
    } finally { setRunning(false); }
  }

  const pathForTab = (tab: string) => integrationValidationPath(integration.id, tab);
  const requiredChecks = preflight?.checks.filter((check) => check.required) ?? [];
  const passed = requiredChecks.filter((check) => check.status === "pass").length;
  return <div className="integration-tab-content">
    <div className="notice"><TerminalSquare /><span><strong>Vendor-neutral acceptance suite.</strong> Use the same OAuth + Stateless MCPv2 client against every integration; never special-case a vendor.</span></div>
    <section className="panel"><PanelHeader title="Configuration preflight" description="Server-backed checks over the exact candidate manifest and immutable bindings. This does not impersonate a user or call the vendor backend." action={<Button disabled={running} onClick={() => void runPreflight()}><RefreshCw data-slot="icon" />{running ? "Running…" : "Run preflight"}</Button>} />{preflightError && <div className="publish-validation error"><span><XCircle /></span><span><strong>Preflight failed</strong><small>{preflightError}</small></span></div>}{preflight?.checks.map((check) => { const ready = check.status === "pass"; const optional = check.status === "optional"; return <ConsoleLink key={check.code} path={pathForTab(check.tab)} onNavigate={onNavigate} className="integration-health-check"><span className={`health-icon ${ready ? "ready" : ""}`}>{ready ? <CheckCircle2 /> : optional ? <ShieldCheck /> : <XCircle />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span><Badge color={ready ? "green" : optional ? "zinc" : "red"}>{ready ? "Pass" : optional ? "Optional" : "Missing"}</Badge><ChevronRight /></ConsoleLink>; })}{!preflight && !preflightError && <div className="empty-row">Run preflight to ask the server to verify the current candidate.</div>}<div className="preflight-summary"><span><strong>{preflight ? `${passed}/${requiredChecks.length} required checks pass` : "Server result pending"}</strong><small>{preflight ? `Candidate r${preflight.candidate_revision} · ${preflight.candidate_manifest_hash} · ${new Date(preflight.generated_at).toLocaleString()}` : "No browser-only assumptions are used"}</small></span><Badge color={preflight?.ready ? "green" : "amber"}>{preflight?.ready ? preflight.matches_latest_published ? "Published & ready" : "Ready to publish" : "Action required"}</Badge></div></section>
    <section className="panel"><PanelHeader title="MCP client acceptance" description="Complete these live scenarios with a test tenant before publication." action={<ConsoleLink path={sectionPath("runs")} onNavigate={onNavigate} className="entity-back-link">Open activity</ConsoleLink>} /><ol className="acceptance-scenarios"><li><span>1</span><div><strong>Discover metadata and register a public client</strong><small>RFC 8414, protected-resource metadata, DCR, exact resource and PKCE S256.</small></div></li><li><span>2</span><div><strong>Authorize a real test customer</strong><small>OIDC callback, live vendor access evaluation, one-time code and bound token.</small></div></li><li><span>3</span><div><strong>List resources and tools</strong><small>Only published resources and grant-authorized exact tool revisions appear.</small></div></li><li><span>4</span><div><strong>Exercise positive and negative calls</strong><small>Valid call, missing grant, invalid schema, absent confirmation, revoked access and upstream drift.</small></div></li><li><span>5</span><div><strong>Verify widget and support isolation</strong><small>Origin allowlist, API allowlist, redaction, retention, delivery receipt and audit correlation.</small></div></li></ol>{distribution?.agent_setup.private.available ? <a className="panel-footer-link" href={distribution.agent_setup.private.url} target="_blank" rel="noreferrer">Open private MCP test-client setup <ExternalLink /></a> : <div className="empty-row">Private test-client setup becomes available after customer identity is active.</div>}</section>
  </div>;
}

type IntegrationsViewProps = {
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  tools: APITool[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  supportRoutes: APISupportRoute[];
  connections: APIAccessConnection[];
  identity: APIIdentity | null;
  distribution: Distribution | null;
  selectedIntegrationID?: string;
  activeTab?: IntegrationTab;
  activeResourceTab?: IntegrationResourceTab;
  onBuild: () => void;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source, attachIntegrationID?: string) => void;
  onAttachPublishedSource: (integrationID: string, source: Source, publication: APISourcePublication) => Promise<DocumentationAttachmentResult>;
  onGenerateAgentGuide: (integrationID: string) => Promise<APIIntegrationAnalysis>;
  onChanged: () => Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

export function IntegrationsView({ integrations, analyses, tools, resourceSets, sources, supportRoutes, connections, identity, distribution, selectedIntegrationID, activeTab = "overview", activeResourceTab = "documentation", onBuild, onAddSource, onCrawlSource, onPublishSource, onAttachPublishedSource, onGenerateAgentGuide, onChanged, onMessage, onNavigate }: IntegrationsViewProps) {
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
  const [accessIntegration, setAccessIntegration] = useState<APIIntegration | null>(null);
  const [accessSelection, setAccessSelection] = useState<string[]>([]);
  const [supportIntegration, setSupportIntegration] = useState<APIIntegration | null>(null);
  const [supportSelection, setSupportSelection] = useState("");
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
      onMessage(editingIntegration ? "API updated." : `API created with ${saved.lifecycle} lifecycle.`);
      if (!editingIntegration) onNavigate(integrationPath(saved.id));
    } catch (error) { onMessage(error instanceof APIError ? error.message : "API could not be saved."); } finally { setBusy(false); }
  }

  async function publishIntegration() {
    if (!publishCandidate) return;
    setBusy(true);
    try {
      const preflight = await api.preflightIntegration(publishCandidate.id);
      if (!preflight.ready) {
        const failed = preflight.checks.find((check) => check.required && check.status !== "pass");
        throw new Error(failed?.message ?? "The server preflight found a required configuration gap.");
      }
      await api.publishIntegration(publishCandidate.id, preflight.candidate_revision, preflight.candidate_manifest_hash);
      await onChanged();
      await refreshSelectedIntegration(publishCandidate.id);
      setPublishCandidate(null);
      onMessage("API published from the exact preflight candidate.");
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "API could not be published."); } finally { setBusy(false); }
  }

  function openAccessDialog(value: APIIntegration) {
    setAccessIntegration(value);
    setAccessSelection(value.access_connection_ids ?? connections.filter((connection) => connection.integration_ids?.includes(value.id)).map((connection) => connection.id));
  }

  async function saveAccessConnections() {
    if (!accessIntegration) return;
    setBusy(true);
    try {
      await api.setIntegrationAccessConnections(accessIntegration.id, accessSelection);
      await onChanged();
      await refreshSelectedIntegration(accessIntegration.id);
      setAccessIntegration(null);
      onMessage("API access connections updated.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Access connections could not be updated."); } finally { setBusy(false); }
  }

  function openSupportDialog(value: APIIntegration) {
    setSupportIntegration(value);
    setSupportSelection(value.support_route_id ?? supportRoutes.find((route) => !route.is_default && route.integration_ids?.includes(value.id))?.id ?? "");
  }

  async function saveSupportRoute() {
    if (!supportIntegration) return;
    setBusy(true);
    try {
      await api.setIntegrationSupportRoute(supportIntegration.id, supportSelection);
      await onChanged();
      await refreshSelectedIntegration(supportIntegration.id);
      setSupportIntegration(null);
      onMessage(supportSelection ? "API reporting policy updated." : "API now inherits the default reporting policy.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Reporting policy could not be updated."); } finally { setBusy(false); }
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
      if (!Array.isArray(parsedManifest)) throw new Error("Manifest must be a JSON array.");
      const existingEntries = parsedManifest as Array<Record<string, unknown>>;
      const options = new Map([...existingEntries, ...latestPublicationEntries].map((entry) => [String(entry.source_publication_id ?? ""), entry]));
      const manifest = setKind === "documentation" ? selectedSourcePublicationIDs.map((id) => options.get(id)).filter((entry): entry is Record<string, unknown> => Boolean(entry)) : existingEntries;
      if (setKind === "documentation" && manifest.length !== selectedSourcePublicationIDs.length) throw new Error("Every selected documentation publication must still exist.");
      if (editingSet) await api.updateResourceSet(editingSet.id, { name: setName, description: resourceDescription, state: editingSet.state, manifest, revision: editingSet.revision });
      else await api.createResourceSet({ kind: setKind, name: setName, description: resourceDescription, manifest });
      await onChanged(); setResourceOpen(false); onMessage(editingSet ? "New immutable resource-set revision created." : "Reusable resource set created.");
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Resource set could not be saved."); } finally { setBusy(false); }
  }

  async function duplicateResource() {
    if (!duplicateSet) return;
    setBusy(true);
    try { await api.duplicateResourceSet(duplicateSet.id, duplicateName); await onChanged(); setDuplicateSet(null); onMessage("Independent resource-set copy created."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be duplicated."); } finally { setBusy(false); }
  }

  async function attachResource() {
    const resource = resourceSets.find((value) => value.id === attachSetID);
    if (!attachIntegration || !resource) return;
    setBusy(true);
    try { await api.attachResourceSet(attachIntegration.id, resource.id, pinAttachedSet ? resource.latest_revision?.id ?? "" : ""); await onChanged(); await refreshSelectedIntegration(attachIntegration.id); setAttachIntegration(null); onMessage(pinAttachedSet ? "Resource revision pinned to API." : "Resource set attached and following latest."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be attached."); } finally { setBusy(false); }
  }

  async function attachPublishedSource(integration: APIIntegration, source: Source) {
	if (!source.latestPublication) return;
	setBusy(true);
	try {
	  const result = await onAttachPublishedSource(integration.id, source, source.latestPublication);
	  await refreshSelectedIntegration(integration.id);
	  onMessage(result.attached ? `${source.name} r${source.latestPublication.revision} was pinned to this API.` : `${source.name} r${source.latestPublication.revision} is already attached.`);
	} catch (error) {
	  onMessage(error instanceof APIError || error instanceof Error ? error.message : "Reviewed documentation could not be attached.");
	} finally {
	  setBusy(false);
	}
  }

  async function detachResource(integrationID: string, setID: string) {
    setBusy(true);
    try { await api.detachResourceSet(integrationID, setID); await onChanged(); await refreshSelectedIntegration(integrationID); onMessage("Resource set detached from API."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be detached."); } finally { setBusy(false); }
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
    {selectedIntegrationID ? <IntegrationWorkspaceView integration={selectedIntegration} integrations={integrations} analyses={analyses} tools={tools} activeTab={activeTab} activeResourceTab={activeResourceTab} loading={selectedLoading} revisions={selectedRevisions} publishStatus={selectedPublishStatus} identity={identity} resourceSets={resourceSets} sources={sources} connections={connections} supportRoutes={supportRoutes} distribution={distribution} busy={busy} onEdit={openIntegration} onPublish={setPublishCandidate} onAttach={openAttachDialog} onCreateResource={() => openResource()} onAddSource={onAddSource} onCrawlSource={onCrawlSource} onPublishSource={onPublishSource} onAttachPublishedSource={attachPublishedSource} onGenerateAgentGuide={onGenerateAgentGuide} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(`${set.name} copy`); }} onDetachResource={detachResource} onManageAccess={openAccessDialog} onManageSupport={openSupportDialog} onInspectRevision={setInspectedRevision} onRuntimeChanged={async () => { await onChanged(); await refreshSelectedIntegration(selectedIntegrationID); }} onMessage={onMessage} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} connections={connections} supportRoutes={supportRoutes} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onBuild={onBuild} onNavigate={onNavigate} />}

    <Dialog
      open={integrationOpen}
      onClose={setIntegrationOpen}
      title={editingIntegration ? "Edit API" : "Add API"}
      description={editingIntegration ? "Update this API’s identity, publishing, and lifecycle settings." : "Create a private draft. You can add documentation, access, and publishing settings next."}
      actions={<><Button outline onClick={() => setIntegrationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !versionKey.trim() || !displayName.trim() || Boolean(editingIntegration && (!familyKey.trim() || (integrationVisibility === "public" && editingIntegration.visibility !== "public" && !integrationPublicAcknowledged)))} onClick={saveIntegration}>{busy ? "Saving…" : editingIntegration ? "Save changes" : "Create API"}</Button></>}
    >
      <div className="auth-form compact-form">
        {!editingIntegration ? <>
          <label className="auth-field"><span>API name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Ex. Voice API" /></label>
          <label className="auth-field"><span>Version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} placeholder="v1" /><small>Start with v1 unless this API already has a public version.</small></label>
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>API family key</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} /></label><label className="auth-field"><span>API version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} /></label></div>
          <label className="auth-field"><span>Display name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
          <label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          <div className="two-fields"><label className="auth-field"><span>Visibility</span><select value={integrationVisibility} onChange={(event) => { setIntegrationVisibility(event.target.value as APIIntegration["visibility"]); setIntegrationPublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public exposes only the published read-only API manifest.</small></label><label className="auth-field"><span>Lifecycle</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="retired">Retired</option></select></label></div>
          {integrationVisibility === "public" && editingIntegration.visibility !== "public" && <label className="compact-check"><input type="checkbox" checked={integrationPublicAcknowledged} onChange={(event) => setIntegrationPublicAcknowledged(event.target.checked)} /><span>I understand this published API metadata will be anonymously discoverable while Public MCP is enabled.</span></label>}
          <label className="auth-field"><span>Replacement</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">None</option>{integrations.filter((value) => value.id !== editingIntegration.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label>
          {(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>Sunset date</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}
        </>}
      </div>
    </Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? `Create revision for ${editingSet.name}` : "Create reusable resource set"} description="Sets are reusable by explicit attachment. Each save creates immutable content." actions={<><Button outline onClick={() => setResourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !setName.trim() || (setKind === "documentation" && selectedSourcePublicationIDs.length === 0)} onClick={saveResourceSet}>{busy ? "Saving…" : editingSet ? "Create revision" : "Create set"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Kind</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">Documentation</option><option value="api">API contract</option></select></label><label className="auth-field"><span>Name</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>Description</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label>{setKind === "documentation" ? <div className="auth-field"><span>Reviewed source publications</span><div className="catalog-list">{documentationPublicationOptions.map((entry) => { const id = String(entry.source_publication_id); const selected = selectedSourcePublicationIDs.includes(id); return <label className="catalog-tool" key={id}><input type="checkbox" checked={selected} onChange={(event) => setSelectedSourcePublicationIDs((items) => event.target.checked ? [...items, id] : items.filter((value) => value !== id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{String(entry.name ?? entry.source_id)}</strong><code>{id}</code><small>Publication r{String(entry.revision)} · {String(entry.content_hash)}</small></span><Badge color="green">reviewed</Badge></label>; })}{documentationPublicationOptions.length === 0 && <div className="empty-row">Publish a reviewed documentation generation before creating this set.</div>}</div><small>Each selection pins one immutable source-publication revision and content hash. Arbitrary JSON is not accepted for documentation.</small></div> : <label className="auth-field"><span>API contract manifest (JSON array)</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label>}</div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title="Duplicate resource set" description="Creates an independent copy so later edits do not affect APIs using the original." actions={<><Button outline onClick={() => setDuplicateSet(null)}>Cancel</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>Duplicate</Button></>}><label className="auth-field"><span>New set name</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={`Attach resources to ${attachIntegration?.display_name ?? "API"}`} description="Follow latest for deliberate sharing, or pin the current immutable revision." actions={<><Button outline onClick={() => setAttachIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>Attach</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Resource set</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">Select a set</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind === "api" ? "API contract" : "documentation"} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>Pin the current revision instead of following latest</span></label></div></Dialog>
    <Dialog open={Boolean(accessIntegration)} onClose={(open) => { if (!open) setAccessIntegration(null); }} title={`Managed credential endpoints — Advanced`} description={`Choose optional provider-management connections for ${accessIntegration?.display_name ?? "this API"}. Runtime service endpoints and API keys are configured separately in Access.`} actions={<><Button outline onClick={() => setAccessIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveAccessConnections}>{busy ? "Saving…" : "Save provider connections"}</Button></>}><div className="auth-form compact-form"><fieldset className="catalog-settings-section"><legend>Provider management connections</legend>{connections.map((connection) => <label className="compact-check" key={connection.id}><input type="checkbox" aria-label={`Allow ${connection.name}`} checked={accessSelection.includes(connection.id)} onChange={() => setAccessSelection((values) => values.includes(connection.id) ? values.filter((id) => id !== connection.id) : [...values, connection.id])} /><span><strong>{connection.name}</strong><small>{connection.definition?.name ?? "Credential-management service"} · {connection.state}</small></span></label>)}{connections.length === 0 && <p className="dialog-empty-copy">No credential-management service exists yet. Create one in Settings, then return here.</p>}</fieldset></div></Dialog>
    <Dialog open={Boolean(supportIntegration)} onClose={(open) => { if (!open) setSupportIntegration(null); }} title={`Bug reports & feedback for ${supportIntegration?.display_name ?? "API"}`} description="Choose an API-specific policy, or inherit the deployment default." actions={<><Button outline onClick={() => setSupportIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveSupportRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Reporting policy</span><select value={supportSelection} onChange={(event) => setSupportSelection(event.target.value)}><option value="">Inherit deployment default</option>{supportRoutes.filter((route) => !route.is_default && route.state === "active").map((route) => <option key={route.id} value={route.id}>{route.name} · {route.retention_days} days</option>)}</select><small>{supportRoutes.find((route) => route.is_default) ? `Current default: ${supportRoutes.find((route) => route.is_default)?.name}` : "No default policy exists. Configure one in Settings."}</small></label></div></Dialog>
    <Dialog open={Boolean(publishCandidate)} onClose={(open) => { if (!open) setPublishCandidate(null); }} title={`Publish ${publishCandidate?.display_name ?? "API"}`} description="Review what changed before creating a new immutable version." actions={<><Button outline onClick={() => setPublishCandidate(null)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedPublishStatus?.ready || !selectedPublishStatus.has_changes} onClick={publishIntegration}>{busy ? "Publishing…" : "Publish"}</Button></>}><div className="publish-review">{selectedPublishStatus?.validations.map((validation) => <div key={validation.code} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level}</strong><small>{validation.message}</small></span></div>)}<div className="publish-diff-list">{selectedPublishStatus?.changes.map((change) => <div className="publish-diff" key={change.field}><strong>{change.field}</strong><span><small>Published</small><code>{change.before === undefined ? "—" : JSON.stringify(change.before)}</code></span><ChevronRight /><span><small>Draft</small><code>{change.after === undefined ? "—" : JSON.stringify(change.after)}</code></span></div>)}</div><details className="advanced-details"><summary>Technical details</summary><code>{selectedPublishStatus?.current_manifest_hash ?? "—"}</code></details></div></Dialog>
    <Dialog open={Boolean(inspectedRevision)} onClose={(open) => { if (!open) setInspectedRevision(null); }} title={`Published version r${inspectedRevision?.revision ?? ""}`} description="This immutable technical snapshot is kept for audit and deterministic agent delivery." actions={<Button outline onClick={() => setInspectedRevision(null)}>Close</Button>}><div className="revision-inspector"><dl className="entity-detail-grid"><div><dt>Version ID</dt><dd>{inspectedRevision?.id}</dd></div><div><dt>State</dt><dd>{inspectedRevision?.state}</dd></div><div><dt>Published</dt><dd>{inspectedRevision ? new Date(inspectedRevision.published_at ?? inspectedRevision.created_at).toLocaleString() : "—"}</dd></div><div><dt>Published by</dt><dd>{inspectedRevision?.published_by || "—"}</dd></div><div><dt>Manifest hash</dt><dd><code>{inspectedRevision?.manifest_hash}</code></dd></div></dl><pre className="usage-contract"><code>{JSON.stringify(inspectedRevision?.snapshot ?? {}, null, 2)}</code></pre></div></Dialog>
  </>;
}
