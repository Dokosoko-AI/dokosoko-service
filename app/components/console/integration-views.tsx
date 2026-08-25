import {
  ArrowLeft, BookOpen, Bug, Check, CheckCircle2, ChevronRight,
  GitBranch, KeyRound, Plus, RefreshCw, Search,
  ShieldCheck, Sparkles, TerminalSquare, TriangleAlert, XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIAccessConnection, APIError, APIIdentity, APIIntegration, APIIntegrationAnalysis,
  APIIntegrationPublishStatus, APIIntegrationRevision, APIResourceSet, APISourcePublication,
  APISupportRoute, APITool, Distribution, api,
} from "../../lib/api";
import {
  INTEGRATION_RESOURCE_TABS, IntegrationResourceTab, IntegrationTab,
  integrationPath, integrationValidationPath, sectionPath,
} from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader } from "../core/layout";
import { IntegrationAgentGuide } from "../integrations/IntegrationAgentGuide";
import { IntegrationNavigation } from "../integrations/IntegrationNavigation";
import { IntegrationQuickStart } from "../integrations/IntegrationQuickStart";
import { IntegrationRuntimeAccess } from "../integrations/IntegrationRuntimeAccess";
import {
  ConsoleLink, DocumentationAttachmentResult, EntityLink, Source, analysisMatchesIntegration,
  apiFamilyKeyFromName, integrationIncludesSourcePublication,
} from "./shared";
import { AuthorizationPolicyWorkspace } from "./integrations/authorization-policy-workspace";
import { IntegrationPackagesWorkspace } from "./integrations/packages-workspace";
import { IntegrationTestWorkspace } from "./integrations/test-workspace";
import { IntegrationToolsWorkspace } from "./integrations/tools-workspace";

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
