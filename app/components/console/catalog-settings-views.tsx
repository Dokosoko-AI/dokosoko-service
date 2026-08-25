import {
  Activity, ArrowLeft, BookOpen, Bot, Bug, CheckCircle2, ChevronRight, Clock3,
  Database, ExternalLink, GitBranch, KeyRound, LockKeyhole, MessageSquareText,
  Plus, RefreshCw, Search, Server, Settings, Share2, ShieldCheck, Sparkles,
  TerminalSquare, TriangleAlert, Users, Wrench, XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance,
  APIAIProviderConnection, APIAIProviderUsage, APIAIWorkloadProfile, APIAnalytics,
  APIAuditEvent, APIBackendConnection, APIEnvironment, APIError, APIIntegration,
  APIIntegrationAnalysis, APIIntegrationRun, APIIntegrationToolBinding, APIMCPConnection,
  APIProduct, APIProductVersion, APIProductVersionPin, APIRecipe, APIRecipeReference,
  APIResourceSet, APISupportRoute, APISupportSubmission, APITool, APIUser, api,
} from "../../lib/api";
import { SETTINGS_TABS, SettingsTab, entityPath, sectionPath, settingsPath, toolBuilderPath } from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { Input, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../core";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader, SectionHeader, SegmentedControl } from "../core/layout";
import { toolIsCommon } from "../integrations/tool-scope";
import {
  AIWorkload, ConsoleLink, EntityLink, Metric, SettingsCard, aiModelDefaults,
  aiModelOptions, aiProviderLabel, aiProviders, aiWorkloads, toolPolicy, toolStateLabel,
} from "./shared";

export function ConnectorReleasesView({ versions, integrations, onConfigure, onNavigate }: { versions: APIProductVersion[]; integrations: APIIntegration[]; onConfigure: () => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Advanced publishing" title="Compatibility snapshots" action={<Button onClick={onConfigure}><Settings data-slot="icon" />Release policy</Button>} /><div className="notice"><GitBranch /><span><strong>API versions stay independent.</strong> A compatibility snapshot can combine Voice API v2 with Face API v3 without changing either API identity.</span></div><div className="metrics-grid"><Metric label="Compatibility snapshots" value={String(versions.length)} detail={`${versions.filter((version) => version.release_stage === "active").length} active`} /><Metric label="APIs" value={String(integrations.length)} detail="Selected by immutable revision" /><Metric label="Latest" value={versions.find((version) => version.is_latest)?.version ?? "—"} detail="Default latest channel" /><Metric label="LTS" value={versions.find((version) => version.is_lts)?.version ?? "—"} detail="Stable channel" /></div><section className="panel"><PanelHeader title="Published snapshots" description="Scoped pins override the default channel." />{versions.map((version) => <div className="provider-row" key={version.id}><span className="settings-icon"><GitBranch /></span><span><EntityLink entity="release" uid={version.id} onNavigate={onNavigate} className="entity-link"><strong>{version.version}</strong></EntityLink><small>{version.profile_name} · {version.manifest_hash}</small></span><span>{version.is_latest && <Badge color="blue">Latest</Badge>} {version.is_lts && <Badge color="violet">LTS</Badge>}</span><Badge color={version.deprecated_at ? "amber" : version.drift_status === "drifted" ? "red" : "green"}>{version.deprecated_at ? "Deprecated" : version.drift_status}</Badge></div>)}{versions.length === 0 && <div className="empty-row">No compatibility snapshots have been published.</div>}</section></>;
}

export function AccessView({ definitions, connections, instances, credentials, integrations, environments, apiResourceSets, settingsTab, onChanged, onMessage, onNavigate }: { definitions: APIAccessDefinition[]; connections: APIAccessConnection[]; instances: APIAccessInstance[]; credentials: APIAccessCredential[]; integrations: APIIntegration[]; environments: APIEnvironment[]; apiResourceSets: APIResourceSet[]; settingsTab?: Extract<SettingsTab, "connections">; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const activeCredentials = credentials.filter((credential) => credential.state === "active" && (!credential.expires_at || new Date(credential.expires_at) > new Date())).length;
  const [definitionOpen, setDefinitionOpen] = useState(false);
  const [editingDefinition, setEditingDefinition] = useState<APIAccessDefinition | null>(null);
  const [connectionOpen, setConnectionOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [serviceKey, setServiceKey] = useState(""); const [serviceName, setServiceName] = useState("");
  const [cardinality, setCardinality] = useState<APIAccessDefinition["instance_cardinality"]>("one");
  const [singular, setSingular] = useState("account"); const [plural, setPlural] = useState("accounts");
  const [credentialScope, setCredentialScope] = useState<APIAccessDefinition["credential_scope"]>("connection");
  const [managementAuth, setManagementAuth] = useState<APIAccessDefinition["management_auth_type"]>("bearer");
  const [apiResourceSetID, setAPIResourceSetID] = useState("");
  const [operations, setOperations] = useState('{\n  "required_grants": [],\n  "max_ttl_seconds": 3600,\n  "credential_storage_mode": "one_time",\n  "authorize": {"method": "POST", "path": "/v1/authorize"},\n  "credentials.create": {"method": "POST", "path": "/v1/credentials"},\n  "credentials.revoke": {"method": "POST", "path": "/v1/credentials/{credential_id}/revoke"}\n}');
  const [definitionID, setDefinitionID] = useState(""); const [connectionName, setConnectionName] = useState(""); const [environmentID, setEnvironmentID] = useState(""); const [region, setRegion] = useState(""); const [baseURL, setBaseURL] = useState(""); const [managementSecret, setManagementSecret] = useState(""); const [connectionConfig, setConnectionConfig] = useState("{}"); const [selectedIntegrations, setSelectedIntegrations] = useState<string[]>([]);

  function openDefinition(definition?: APIAccessDefinition) {
    setEditingDefinition(definition ?? null);
    setServiceKey(definition?.service_key ?? "");
    setServiceName(definition?.name ?? "");
    setCardinality(definition?.instance_cardinality ?? "one");
    setSingular(definition?.instance_label_singular ?? "account");
    setPlural(definition?.instance_label_plural ?? "accounts");
    setCredentialScope(definition?.credential_scope ?? "connection");
    setManagementAuth(definition?.management_auth_type ?? "bearer");
    setAPIResourceSetID(definition?.api_resource_set_id ?? "");
    setOperations(definition ? JSON.stringify(definition.operations, null, 2) : '{\n  "required_grants": [],\n  "max_ttl_seconds": 3600,\n  "credential_storage_mode": "one_time",\n  "authorize": {"method": "POST", "path": "/v1/authorize"},\n  "credentials.create": {"method": "POST", "path": "/v1/credentials"},\n  "credentials.revoke": {"method": "POST", "path": "/v1/credentials/{credential_id}/revoke"}\n}');
    setDefinitionOpen(true);
  }

  function closeDefinition() { setDefinitionOpen(false); setEditingDefinition(null); }

  async function saveDefinition() {
    setBusy(true);
    try { const parsed = JSON.parse(operations) as Record<string, unknown>; if (editingDefinition) { await api.updateAccessDefinition(editingDefinition.id, { name: serviceName, instance_label_singular: singular, instance_label_plural: plural, api_resource_set_id: apiResourceSetID || undefined, operations: parsed, revision: editingDefinition.revision }); } else { await api.createAccessDefinition({ service_key: serviceKey, name: serviceName, instance_cardinality: cardinality, instance_label_singular: singular, instance_label_plural: plural, credential_scope: cardinality === "one" ? "connection" : credentialScope, management_auth_type: managementAuth, api_resource_set_id: apiResourceSetID || undefined, operations: parsed }); } await onChanged(); closeDefinition(); onMessage(editingDefinition ? "Provider service type revision saved. Existing connections kept their encrypted credentials." : "Provider access definition created."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access definition could not be saved."); } finally { setBusy(false); }
  }

  async function saveConnection() {
    setBusy(true);
    try { const parsed = JSON.parse(connectionConfig) as Record<string, unknown>; await api.createAccessConnection({ access_definition_id: definitionID, environment_id: environmentID || undefined, name: connectionName, region: region || undefined, base_url: baseURL, management_secret: managementSecret || undefined, config: parsed, integration_ids: selectedIntegrations }); await onChanged(); setConnectionOpen(false); setManagementSecret(""); onMessage("Access connection created and attached."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access connection could not be created."); } finally { setBusy(false); }
  }

  function toggleIntegration(id: string) { setSelectedIntegrations((values) => values.includes(id) ? values.filter((value) => value !== id) : [...values, id]); }

  return <>
    <PageHeading eyebrow={settingsTab ? "Settings" : "Shared configuration"} title="Service connections" action={<Button onClick={() => { setDefinitionID(definitions[0]?.id ?? ""); setEnvironmentID(environments[0]?.id ?? ""); setConnectionOpen(true); }}><KeyRound data-slot="icon" />Connect service</Button>} />
    {settingsTab && <SettingsTabs active={settingsTab} onNavigate={onNavigate} />}
    <section className="panel"><PanelHeader title="Connections" />{connections.map((connection) => { const definition = connection.definition ?? definitions.find((item) => item.id === connection.access_definition_id); const connectionInstances = instances.filter((item) => item.access_connection_id === connection.id); const connectionCredentials = credentials.filter((item) => item.access_connection_id === connection.id); const labels = (connection.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)).filter(Boolean).map((item) => `${item!.display_name} ${item!.version_key}`).join(", "); return <div className="provider-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{definition?.name ?? "Service"} · {labels || "No API attached"}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge><span><strong>{definition?.instance_cardinality === "many" ? connectionInstances.length : "1"} {definition?.instance_cardinality === "many" ? definition.instance_label_plural : definition?.instance_label_singular ?? "instance"}</strong><small>{connectionCredentials.length} credential record{connectionCredentials.length === 1 ? "" : "s"}</small></span></div>; })}{connections.length === 0 && <div className="empty-row">No service connections yet. Connect a vendor service to make it available to APIs.</div>}</section>
    <details className="panel advanced-details"><summary>Advanced service setup</summary><div className="advanced-details-body"><PanelHeader title="Service types" action={<Button outline onClick={() => openDefinition()}><Plus data-slot="icon" />New service type</Button>} />{definitions.map((definition) => <div className="lease-row" key={definition.id}><span><EntityLink entity="access-definition" uid={definition.id} onNavigate={onNavigate} className="entity-link"><strong>{definition.name}</strong></EntityLink><small>{definition.instance_cardinality === "many" ? `Multiple ${definition.instance_label_plural}` : `Single ${definition.instance_label_singular}`} · revision {definition.revision}</small></span><span className="heading-actions"><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge><Button outline onClick={() => openDefinition(definition)}>Edit</Button></span></div>)}{definitions.length === 0 && <div className="empty-row">No service types are configured.</div>}<PanelHeader className="advanced-subheading" title="Credential records" description="Fingerprints and lifecycle only. Plaintext credentials are never listed." action={<Badge color="violet">{activeCredentials} active</Badge>} />{credentials.slice(0, 12).map((credential) => <div className="lease-row" key={credential.id}><span><strong>{credential.scopes.join(", ") || "Default scope"}</strong><small>{credential.secret_fingerprint.slice(0, 18)}… · {credential.storage_mode}</small></span><Badge color={credential.state === "active" ? "green" : "zinc"}>{credential.state}</Badge></div>)}{credentials.length === 0 && <div className="empty-row">No credential records yet.</div>}</div></details>
  <Dialog open={definitionOpen} onClose={(open) => { if (!open) closeDefinition(); }} title={editingDefinition ? `Revise ${editingDefinition.name}` : "Create service type"} description={editingDefinition ? "Update the contract and operations without replacing connections or encrypted credentials. Identity and authentication fields are locked." : "The provider contract declares cardinality and credential scope; end users do not choose mono versus multi."} actions={<><Button outline onClick={closeDefinition}>Cancel</Button><Button color="indigo" disabled={busy || !serviceKey.trim() || !serviceName.trim() || !singular.trim() || !plural.trim()} onClick={saveDefinition}>{busy ? "Saving…" : editingDefinition ? "Save revision" : "Create service type"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service key</span><input disabled={Boolean(editingDefinition)} value={serviceKey} onChange={(event) => setServiceKey(event.target.value)} placeholder="projecthub" /></label><label className="auth-field"><span>Name</span><input value={serviceName} onChange={(event) => setServiceName(event.target.value)} placeholder="ProjectHub Management API" /></label></div><div className="two-fields"><label className="auth-field"><span>Provider instances</span><select disabled={Boolean(editingDefinition)} value={cardinality} onChange={(event) => { const value = event.target.value as typeof cardinality; setCardinality(value); if (value === "one") setCredentialScope("connection"); }}><option value="one">One fixed instance</option><option value="many">Multiple provider resources</option></select></label><label className="auth-field"><span>Credential scope</span><select disabled={Boolean(editingDefinition) || cardinality === "one"} value={credentialScope} onChange={(event) => setCredentialScope(event.target.value as typeof credentialScope)}><option value="connection">Connection</option><option value="instance">Provider resource</option></select></label></div><div className="two-fields"><label className="auth-field"><span>Singular label</span><input value={singular} onChange={(event) => setSingular(event.target.value)} placeholder="workspace" /></label><label className="auth-field"><span>Plural label</span><input value={plural} onChange={(event) => setPlural(event.target.value)} placeholder="workspaces" /></label></div><div className="two-fields"><label className="auth-field"><span>Management authentication</span><select disabled={Boolean(editingDefinition)} value={managementAuth} onChange={(event) => setManagementAuth(event.target.value as typeof managementAuth)}><option value="bearer">Bearer token</option><option value="api_key">API key</option><option value="oauth2_client_credentials">OAuth2 client credentials</option><option value="none">None</option></select></label><label className="auth-field"><span>API contract set</span><select value={apiResourceSetID} onChange={(event) => setAPIResourceSetID(event.target.value)}><option value="">None</option>{apiResourceSets.map((set) => <option key={set.id} value={set.id}>{set.name}</option>)}</select></label></div><label className="auth-field"><span>Operations (JSON)</span><textarea className="code-input" value={operations} onChange={(event) => setOperations(event.target.value)} spellCheck={false} /></label></div></Dialog>
  <Dialog open={connectionOpen} onClose={setConnectionOpen} title="Connect vendor service" description="Credentials are encrypted server-side. The fixed HTTPS destination is validated again for every operation." actions={<><Button outline onClick={() => setConnectionOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !definitionID || !connectionName.trim() || !baseURL.trim() || selectedIntegrations.length === 0} onClick={saveConnection}>{busy ? "Connecting…" : "Connect service"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service type</span><select value={definitionID} onChange={(event) => setDefinitionID(event.target.value)}><option value="">Select definition</option>{definitions.map((definition) => <option key={definition.id} value={definition.id}>{definition.name}</option>)}</select></label><label className="auth-field"><span>Connection name</span><input value={connectionName} onChange={(event) => setConnectionName(event.target.value)} /></label></div><div className="two-fields"><label className="auth-field"><span>Environment</span><select value={environmentID} onChange={(event) => setEnvironmentID(event.target.value)}><option value="">All environments</option>{environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}</option>)}</select></label><label className="auth-field"><span>Region</span><input value={region} onChange={(event) => setRegion(event.target.value)} placeholder="us-east-1" /></label></div><label className="auth-field"><span>Fixed HTTPS base URL</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://management.example.com" /></label><label className="auth-field"><span>Management credential</span><input type="password" autoComplete="off" value={managementSecret} onChange={(event) => setManagementSecret(event.target.value)} /></label><fieldset className="catalog-settings-section"><legend>Allowed APIs</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={selectedIntegrations.includes(integration.id)} onChange={() => toggleIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset><label className="auth-field"><span>Connection configuration (JSON)</span><textarea className="code-input" value={connectionConfig} onChange={(event) => setConnectionConfig(event.target.value)} spellCheck={false} /></label></div></Dialog>
  </>;
}


export function ActivityHubView({ runs, environments, submissions, events, analytics, supportRoutes, onStart, onComplete, onView, onRetry, onNavigate }: { runs: APIIntegrationRun[]; environments: APIEnvironment[]; submissions: APISupportSubmission[]; events: APIAuditEvent[]; analytics: APIAnalytics | null; supportRoutes: APISupportRoute[]; onStart: () => void; onComplete: (run: APIIntegrationRun, succeeded: boolean) => void; onView: (submission: APISupportSubmission) => void; onRetry: (submission: APISupportSubmission) => void; onNavigate: (path: string) => void }) {
  const [filter, setFilter] = useState<"all" | "runs" | "reports" | "audit">("all");
  const environmentName = (id: string) => environments.find((environment) => environment.id === id)?.name ?? id;
  const statusColor = (state: APISupportSubmission["state"]): "zinc" | "blue" | "green" | "red" | "amber" => state === "delivered" ? "green" : state === "failed" ? "red" : state === "held" ? "amber" : "blue";
  const canRetry = (submission: APISupportSubmission) => supportRoutes.some((route) => route.id === submission.support_route_id && route.state === "active" && (submission.kind === "bug" ? route.bug_reports_enabled : route.feedback_enabled));
  const show = (kind: typeof filter) => filter === "all" || filter === kind;

  return <>
    <PageHeading eyebrow="Operations" title="Activity" action={<Button onClick={onStart}><Plus data-slot="icon" />Start run</Button>} />
    <SegmentedControl label="Filter activity" items={[{ id: "all", label: "All" }, { id: "runs", label: "Runs", count: runs.length }, { id: "reports", label: "Bug reports & feedback", count: submissions.length }, { id: "audit", label: "Audit", count: events.length }]} value={filter} onChange={setFilter} />
    {analytics && <div className="activity-summary"><strong>Last 30 days</strong><span>{analytics.integration_runs} runs</span><span>{analytics.tool_calls} tool calls</span><span>{analytics.first_pass_rate.toFixed(1)}% first-pass success</span></div>}

    {show("runs") && <section className="panel"><PanelHeader title="API runs" />{runs.map((run) => <div className="root-row run-row" key={run.id}><span className="settings-icon">{run.state === "running" ? <Clock3 /> : run.validated_success ? <CheckCircle2 /> : <XCircle />}</span><span><EntityLink entity="run" uid={run.id} onNavigate={onNavigate} className="entity-link"><strong>{run.requested_outcome}</strong></EntityLink><small>{environmentName(run.environment_id)} · {new Date(run.started_at).toLocaleString()}{run.failure_code ? ` · ${run.failure_code}` : ""}</small></span><Badge color={run.state === "running" ? "blue" : run.validated_success ? "green" : "red"}>{run.state}</Badge>{run.state === "running" ? <span className="run-actions"><Button outline onClick={() => onComplete(run, false)}>Failed</Button><Button color="indigo" onClick={() => onComplete(run, true)}>Validated</Button></span> : <span />}</div>)}{runs.length === 0 && <div className="empty-row">No API runs yet.</div>}</section>}

    {show("reports") && <section className="panel report-inbox"><PanelHeader title="Bug reports & feedback" action={<Badge color="violet">Encrypted at rest</Badge>} /><DataTable label="Bug reports and feedback"><DataTableHeader className="report-columns"><span>Submission</span><span>API</span><span>Delivery</span><span>Actions</span></DataTableHeader>{submissions.map((submission) => <DataTableRow className="report-columns" key={submission.id}><span className="resource-name"><span className="resource-icon">{submission.kind === "bug" ? <Bug /> : <MessageSquareText />}</span><span><EntityLink entity="report" uid={submission.id} onNavigate={onNavigate} className="entity-link"><strong title={submission.summary}>{submission.summary}</strong></EntityLink><small>{submission.kind} · {new Date(submission.created_at).toLocaleString()}</small></span></span><span><strong className="cell-value">{submission.trusted_integration ? `${submission.trusted_integration.display_name} ${submission.trusted_integration.version_key}` : submission.related_tool || "Deployment"}</strong></span><span><Badge color={statusColor(submission.state)}>{submission.state}</Badge><small className="cell-note">{submission.external_id || (submission.attempts ? `${submission.attempts} attempt${submission.attempts === 1 ? "" : "s"}` : "Not delivered")}</small></span><span className="table-actions"><Button outline onClick={() => onView(submission)}>View</Button>{submission.external_url && <a className="report-ticket-link" href={submission.external_url} target="_blank" rel="noreferrer" aria-label="Open external ticket"><ExternalLink /></a>}{(submission.state === "failed" || submission.state === "held") && canRetry(submission) && <Button outline onClick={() => onRetry(submission)}><RefreshCw data-slot="icon" />Retry</Button>}</span></DataTableRow>)}{submissions.length === 0 && <DataTableEmpty columns={4}>Approved bug reports and feedback will appear here.</DataTableEmpty>}</DataTable></section>}

    {show("audit") && <section className="panel"><PanelHeader title="Audit" description="Secrets are never recorded." action={<Badge color="green">Append-only</Badge>} />{events.map((event) => <div className="root-row audit-row compact-audit-row" key={event.id}><span className="settings-icon"><ShieldCheck /></span><span><EntityLink entity="audit-event" uid={event.id} onNavigate={onNavigate} className="entity-link"><strong>{event.action}</strong></EntityLink><small>{event.target_type} · {new Date(event.created_at).toLocaleString()}</small></span><code>{event.actor_id}</code></div>)}{events.length === 0 && <div className="empty-row">Audit activity appears after the first configuration change.</div>}</section>}
  </>;
}

export function ReportingView({ routes, integrations, backendConnections, settingsTab, onChanged, onMessage, onNavigate }: { routes: APISupportRoute[]; integrations: APIIntegration[]; backendConnections: APIBackendConnection[]; settingsTab?: Extract<SettingsTab, "reporting">; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [routeOpen, setRouteOpen] = useState(false);
  const [editingRoute, setEditingRoute] = useState<APISupportRoute | null>(null);
  const [routeName, setRouteName] = useState(""); const [routeDefault, setRouteDefault] = useState(false); const [routeBugEnabled, setRouteBugEnabled] = useState(true); const [routeFeedbackEnabled, setRouteFeedbackEnabled] = useState(true); const [routeBackendID, setRouteBackendID] = useState(""); const [routeRetention, setRouteRetention] = useState("30"); const [routeIntegrations, setRouteIntegrations] = useState<string[]>([]); const [busy, setBusy] = useState(false);
  const [backendOpen, setBackendOpen] = useState(false); const [editingBackend, setEditingBackend] = useState<APIBackendConnection | null>(null); const [backendName, setBackendName] = useState(""); const [backendBaseURL, setBackendBaseURL] = useState(""); const [backendState, setBackendState] = useState<APIBackendConnection["state"]>("disabled"); const [backendCredential, setBackendCredential] = useState("");

  function openRoute(value?: APISupportRoute) {
    setEditingRoute(value ?? null); setRouteName(value?.name ?? ""); setRouteDefault(value?.is_default ?? routes.length === 0); setRouteBugEnabled(value?.bug_reports_enabled ?? true); setRouteFeedbackEnabled(value?.feedback_enabled ?? true); setRouteBackendID(value?.backend_connection_id ?? backendConnections.find((connection) => connection.state === "active")?.id ?? ""); setRouteRetention(String(value?.retention_days ?? 30)); setRouteIntegrations(value?.integration_ids ?? []); setRouteOpen(true);
  }

  function toggleRouteIntegration(id: string) { setRouteIntegrations((values) => values.includes(id) ? values.filter((value) => value !== id) : [...values, id]); }

  async function saveRoute() {
    setBusy(true);
    try {
      const input = { name: routeName, is_default: routeDefault, bug_reports_enabled: routeBugEnabled, feedback_enabled: routeFeedbackEnabled, backend_connection_id: routeBackendID || undefined, retention_days: Number(routeRetention), state: "active" as const, integration_ids: routeDefault ? [] : routeIntegrations };
      if (editingRoute) await api.replaceSupportRoute(editingRoute.id, { ...input, revision: editingRoute.revision }); else await api.createSupportRoute(input);
      await onChanged(); setRouteOpen(false); onMessage(editingRoute ? "Reporting policy updated." : "Reporting policy created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Reporting policy could not be saved."); } finally { setBusy(false); }
  }

  function openBackend(value?: APIBackendConnection) {
    setEditingBackend(value ?? null); setBackendName(value?.name ?? ""); setBackendBaseURL(value?.base_url ?? ""); setBackendState(value?.state ?? "disabled"); setBackendCredential(""); setBackendOpen(true);
  }

  async function saveBackend() {
    setBusy(true);
    try {
      if (editingBackend) {
		let revision = editingBackend.revision;
		let rotatedBeforeActivation = false;
		if (backendState === "active" && !editingBackend.credential_fingerprint && backendCredential.trim()) {
			const credential = await api.createBackendConnectionCredential(editingBackend.id, backendCredential, revision);
			revision = credential.connection_revision;
			rotatedBeforeActivation = true;
		}
		const updated = await api.replaceBackendConnection(editingBackend.id, { name: backendName, base_url: backendBaseURL, authentication_type: "bearer", state: backendState, revision });
		if (backendCredential.trim() && !rotatedBeforeActivation) await api.createBackendConnectionCredential(updated.id, backendCredential, updated.revision);
      } else {
        await api.createBackendConnection({ name: backendName, base_url: backendBaseURL, authentication_type: "bearer", credential: backendCredential || undefined, state: backendState });
      }
      await onChanged(); setBackendOpen(false); onMessage(editingBackend ? "Backend connection updated." : "Backend connection created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Backend connection could not be saved."); } finally { setBusy(false); }
  }

  return <>
    {!settingsTab && <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("settings")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Settings</ConsoleLink></div>}
    <PageHeading eyebrow="Settings" title="Bug reports & feedback" action={<Button onClick={() => openRoute()}><Plus data-slot="icon" />New policy</Button>} />
    {settingsTab && <SettingsTabs active={settingsTab} onNavigate={onNavigate} />}
    <section className="panel"><PanelHeader title="Backend connections" description="Service-to-service origins and bearer credentials are independent of customer identity." action={<Button outline onClick={() => openBackend()}><Plus data-slot="icon" />New connection</Button>} />{backendConnections.map((connection) => <div className="provider-row" key={connection.id}><span className="settings-icon"><Server /></span><span><strong>{connection.name}</strong><small>{connection.base_url} · bearer · {connection.credential_fingerprint ? `credential ${connection.credential_fingerprint}` : "no credential"}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><Button outline onClick={() => openBackend(connection)}>Edit</Button></div>)}{backendConnections.length === 0 && <div className="empty-row">Create a backend connection before enabling support delivery.</div>}</section>
    <section className="panel"><PanelHeader title="Delivery policies" action={<Badge color="violet">{routes.length} polic{routes.length === 1 ? "y" : "ies"}</Badge>} />{routes.map((route) => <div className="provider-row" key={route.id}><span className="settings-icon"><MessageSquareText /></span><span><EntityLink entity="support-route" uid={route.id} onNavigate={onNavigate} className="entity-link"><strong>{route.name}</strong></EntityLink><small>{route.is_default ? "Default for all APIs" : (route.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)?.display_name ?? id).join(", ")}</small></span><span>{route.bug_reports_enabled && <Badge color="blue">Bugs</Badge>} {route.feedback_enabled && <Badge color="violet">Feedback</Badge>}</span><span className="table-actions"><small>{route.bug_reports_enabled || route.feedback_enabled ? backendConnections.find((connection) => connection.id === route.backend_connection_id)?.name ?? "Backend unavailable" : "Delivery disabled"} · {route.retention_days} days</small><Button outline onClick={() => openRoute(route)}>Edit</Button></span></div>)}{routes.length === 0 && <div className="empty-row">Create a default policy to enable bug reports and feedback.</div>}</section>
    <Dialog open={routeOpen} onClose={setRouteOpen} title={editingRoute ? "Edit reporting policy" : "Create reporting policy"} description="Approved submissions are delivered to /v1/support-submissions through a separately authenticated backend connection." actions={<><Button outline onClick={() => setRouteOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !routeName.trim() || (!routeDefault && routeIntegrations.length === 0) || ((routeBugEnabled || routeFeedbackEnabled) && !backendConnections.some((connection) => connection.id === routeBackendID && connection.state === "active"))} onClick={saveRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Policy name</span><input value={routeName} onChange={(event) => setRouteName(event.target.value)} placeholder="Default reporting" /></label><label className="compact-check"><input type="checkbox" checked={routeDefault} onChange={(event) => setRouteDefault(event.target.checked)} /><span>Use as the default for all APIs</span></label>{!routeDefault && <fieldset className="catalog-settings-section"><legend>Assigned APIs</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={routeIntegrations.includes(integration.id)} onChange={() => toggleRouteIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset>}<div className="two-fields"><label className="compact-check"><input type="checkbox" checked={routeBugEnabled} onChange={(event) => setRouteBugEnabled(event.target.checked)} /><span>Enable bug reports</span></label><label className="compact-check"><input type="checkbox" checked={routeFeedbackEnabled} onChange={(event) => setRouteFeedbackEnabled(event.target.checked)} /><span>Enable feedback</span></label></div><label className="auth-field"><span>Backend connection</span><select value={routeBackendID} onChange={(event) => setRouteBackendID(event.target.value)}><option value="">No delivery connection</option>{backendConnections.map((connection) => <option key={connection.id} value={connection.id} disabled={connection.state !== "active"}>{connection.name} · {connection.state}</option>)}</select><small>The route stores only this reference; credentials rotate on the connection.</small></label><label className="auth-field"><span>Encrypted retention (days)</span><input type="number" min={1} max={365} value={routeRetention} onChange={(event) => setRouteRetention(event.target.value)} /></label></div></Dialog>
		<Dialog open={backendOpen} onClose={setBackendOpen} title={editingBackend ? "Edit backend connection" : "Create backend connection"} description="This credential is used only for service-to-service delivery, never for customer access or tool calls." actions={<><Button outline onClick={() => setBackendOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !backendName.trim() || !backendBaseURL.trim() || (backendState === "active" && !backendCredential.trim() && !editingBackend?.credential_fingerprint)} onClick={saveBackend}>{busy ? "Saving…" : "Save connection"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={backendName} onChange={(event) => setBackendName(event.target.value)} placeholder="Support delivery" /></label><label className="auth-field"><span>HTTPS origin</span><input type="url" value={backendBaseURL} onChange={(event) => setBackendBaseURL(event.target.value)} placeholder="https://backend.vendor.com" /><small>DokoSoko appends only /v1/support-submissions.</small></label><div className="two-fields"><label className="auth-field"><span>Authentication</span><input value="Bearer" disabled /></label><label className="auth-field"><span>State</span><select value={backendState} onChange={(event) => setBackendState(event.target.value as APIBackendConnection["state"])}><option value="disabled">Disabled</option><option value="active">Active</option></select></label></div><label className="auth-field"><span>{editingBackend ? "Rotate bearer credential (optional)" : "Bearer credential"}</span><input type="password" autoComplete="off" value={backendCredential} onChange={(event) => setBackendCredential(event.target.value)} /><small>Submitted once, encrypted immediately, and never returned.</small></label></div></Dialog>
  </>;
}


function ToolsWorkspaceTabs({ active, onNavigate }: { active: "catalog" | "connections"; onNavigate: (path: string) => void }) {
  return <PageTabs label="Tool management sections">
    <ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className={`page-tab ${active === "catalog" ? "active" : ""}`} ariaCurrent={active === "catalog" ? "page" : undefined}>Catalog</ConsoleLink>
    <ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className={`page-tab ${active === "connections" ? "active" : ""}`} ariaCurrent={active === "connections" ? "page" : undefined}>Connections</ConsoleLink>
  </PageTabs>;
}

export function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect, onNavigate }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void; onNavigate: (path: string) => void }) {
  const imported = tools.filter((tool) => tool.backend_kind === "mcp");
  const delegated = connections.filter((connection) => connection.auth_mode === "delegated_oauth").length;
  const authLabel = (mode: APIMCPConnection["auth_mode"]) => mode === "delegated_oauth" ? "Delegated user OAuth" : mode === "service" ? "Service credential" : "No upstream auth";
  return <>
    <PageHeading eyebrow="Tools" title="Connections" description="Inspect upstream MCP catalogs and import reviewed definitions into the deployment tool catalog." action={<Button onClick={onAdd}><Plus data-slot="icon" />Connect MCP</Button>} />
    <ToolsWorkspaceTabs active="connections" onNavigate={onNavigate} />
    <a className="mcp-policy-banner" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><span className="mcp-policy-icon"><ShieldCheck /></span><span><strong>Stateless MCPv2 Only</strong><small>Protocol 2026-07-28 · self-contained requests · no logical live sessions</small></span><ExternalLink /></a>
    <div className="metrics-grid"><Metric label="Upstream connections" value={String(connections.length)} detail="Fixed HTTPS destinations" /><Metric label="Imported tools" value={String(imported.length)} detail={`${imported.filter((tool) => tool.state === "published").length} published`} /><Metric label="Delegated identities" value={String(delegated)} detail="Separate upstream grants" /><Metric label="Drifted schemas" value={String(imported.filter((tool) => tool.upstream_drifted).length)} detail="Published calls fail closed" positive={!imported.some((tool) => tool.upstream_drifted)} /></div>
    <section className="panel mcp-connections-panel">
      <PanelHeader title="Managed upstreams" description="Inspect returns a complete catalog; import always creates or updates local drafts." action={<Badge color="green">Pre-call authz</Badge>} />
      {connections.map((connection) => {
        const connectionTools = imported.filter((tool) => tool.mcp_connection_id === connection.id);
        return <article className="mcp-connection-row" key={connection.id}><span className="connection-mark"><Share2 /></span><span className="connection-main"><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge></span><code>{connection.endpoint}</code><small>{connection.namespace}.* · {connection.protocol_version} · {authLabel(connection.auth_mode)}</small></span><span className="connection-stat"><strong>{connectionTools.length}</strong><small>imported tools</small></span><span className="connection-stat"><strong>{connection.last_synced_at ? new Date(connection.last_synced_at).toLocaleDateString() : "Never"}</strong><small>last inspected</small></span><Button outline disabled={busy} onClick={() => onInspect(connection)}><RefreshCw data-slot="icon" />Inspect & import</Button></article>;
      })}
      {connections.length === 0 && <div className="empty-row">No upstream MCP is connected. Add one to inspect and review its catalog.</div>}
    </section>
    <div className="identity-flow"><span><LockKeyhole /><strong>1 · DokoSoko identity</strong><small>Authenticate the user and resolve a durable customer account.</small></span><span><ShieldCheck /><strong>2 · Access policy</strong><small>Validate schema, confirmation, grants, and the vendor access evaluation.</small></span><span><Users /><strong>3 · Upstream identity</strong><small>Use a separate user grant or encrypted service credential—never the inbound token.</small></span></div>
  </>;
}

type ToolCatalogFilter = "all" | "published" | "draft" | "drifted" | "retired";

export function ToolsView({ tools, integrations, connections, onNavigate }: { tools: APITool[]; integrations: APIIntegration[]; connections: APIMCPConnection[]; onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ToolCatalogFilter>("all");
  const [usageCounts, setUsageCounts] = useState<Record<string, number>>({});
  const [usageStatus, setUsageStatus] = useState<"loading" | "ready" | "partial">("loading");

  useEffect(() => {
    let cancelled = false;
    Promise.all(integrations.map(async (integration) => {
      try { return { bindings: await api.integrationToolBindings(integration.id), failed: false }; }
      catch { return { bindings: [] as APIIntegrationToolBinding[], failed: true }; }
    })).then((results) => {
      if (cancelled) return;
      const next: Record<string, number> = {};
      results.flatMap((result) => result.bindings).forEach((binding) => { next[binding.tool_id] = (next[binding.tool_id] ?? 0) + 1; });
      setUsageCounts(next);
      setUsageStatus(results.some((result) => result.failed) ? "partial" : "ready");
    });
    return () => { cancelled = true; };
  }, [integrations]);

  const commonTools = tools.filter(toolIsCommon);
  const normalizedQuery = query.trim().toLowerCase();
  const visibleTools = commonTools.filter((tool) => {
    const matchesQuery = !normalizedQuery || `${tool.namespace}.${tool.name} ${tool.description} ${tool.backend_kind ?? "http"} ${tool.upstream_tool_name ?? ""}`.toLowerCase().includes(normalizedQuery);
    const matchesFilter = filter === "all" || filter === "drifted" ? filter === "all" || Boolean(tool.upstream_drifted) : tool.state === filter;
    return matchesQuery && matchesFilter;
  });
  const connectionName = (tool: APITool) => connections.find((connection) => connection.id === tool.mcp_connection_id)?.name ?? "MCP upstream";

  return <>
    <PageHeading eyebrow="Capabilities" title="Common tools" description="Create reusable deployment capabilities once, then attach exact published revisions to the APIs that expose them. API-owned tools live only inside their API workspace." action={<span className="heading-actions"><Button outline onClick={() => onNavigate(sectionPath("connections"))}><Share2 data-slot="icon" />Import from MCP</Button><Button color="indigo" onClick={() => onNavigate(toolBuilderPath())}><Plus data-slot="icon" />Create common tool</Button></span>} />
    <ToolsWorkspaceTabs active="catalog" onNavigate={onNavigate} />
    <dl className="compact-metrics tool-catalog-metrics"><div className="compact-metric"><dt>Total</dt><dd><strong>{commonTools.length}</strong><small>common tools</small></dd></div><div className="compact-metric"><dt>Published</dt><dd><strong>{commonTools.filter((tool) => tool.state === "published").length}</strong><small>eligible to bind</small></dd></div><div className="compact-metric"><dt>Drafts</dt><dd><strong>{commonTools.filter((tool) => tool.state === "draft").length}</strong><small>editable contracts</small></dd></div><div className="compact-metric"><dt>Drifted</dt><dd><strong>{commonTools.filter((tool) => tool.upstream_drifted).length}</strong><small>blocked upstreams</small></dd></div></dl>
    <div className="table-toolbar tool-catalog-toolbar"><label className="table-search"><Search /><span className="sr-only">Search common tools</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search common tools" /></label><SegmentedControl label="Filter common tools" items={[{ id: "all", label: "All", count: commonTools.length }, { id: "published", label: "Published", count: commonTools.filter((tool) => tool.state === "published").length }, { id: "draft", label: "Drafts", count: commonTools.filter((tool) => tool.state === "draft").length }, { id: "drifted", label: "Drifted", count: commonTools.filter((tool) => tool.upstream_drifted).length }, { id: "retired", label: "Retired", count: commonTools.filter((tool) => tool.state === "retired").length }]} value={filter} onChange={setFilter} /></div>
    <span className="sr-only" role="status" aria-live="polite">{visibleTools.length} tool{visibleTools.length === 1 ? "" : "s"} shown.</span>
    <DataTable label="Deployment tools" className="tool-catalog-table">
      <DataTableHeader className="tool-catalog-columns"><span>Tool</span><span>Source</span><span>Risk &amp; access</span><span>State</span><span>Current APIs</span><span>Open</span></DataTableHeader>
      {visibleTools.map((tool) => {
        const policy = toolPolicy(tool);
        const risk = policy.risk === "critical" || policy.risk === "high" || policy.risk === "medium" ? policy.risk : "low";
        const riskColor = risk === "critical" ? "red" : risk === "high" ? "amber" : risk === "medium" ? "violet" : "zinc";
        return <DataTableRow className={`tool-catalog-columns ${tool.upstream_drifted ? "drifted" : ""}`} key={tool.id}>
          <span className="resource-name tool-catalog-name"><span className="resource-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{tool.description || "No purpose documented"}</small></span></span>
          <span><strong className="cell-value">{tool.backend_kind === "mcp" ? connectionName(tool) : "HTTP"}</strong><small className="cell-note">{tool.backend_kind === "mcp" ? tool.upstream_tool_name : `${tool.http_method} · fixed endpoint`}</small></span>
          <span className="tool-policy-cell"><span className="tool-badges"><Badge color={riskColor}>{risk} risk</Badge>{policy.confirmationRequired && <Badge color="amber">confirmation</Badge>}</span><small className="cell-note">{policy.requiredGrants.join(", ") || "No baseline grants"}</small></span>
          <span className="tool-state-cell"><Badge color={tool.state === "published" ? "green" : tool.state === "retired" ? "zinc" : "amber"}>{toolStateLabel(tool)}</Badge>{tool.upstream_drifted && <Badge color="red">schema drift</Badge>}</span>
          <span><strong className="cell-value">{usageStatus === "loading" ? "…" : usageStatus === "partial" ? `≥${usageCounts[tool.id] ?? 0}` : usageCounts[tool.id] ?? 0}</strong><small className="cell-note">current API draft{(usageCounts[tool.id] ?? 0) === 1 ? "" : "s"}{usageStatus === "partial" ? " · partial" : ""}</small></span>
          <span className="table-open-cell"><ConsoleLink path={entityPath("tool", tool.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${tool.namespace}.${tool.name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>;
      })}
      {visibleTools.length === 0 && <DataTableEmpty columns={6}><div className="tool-catalog-empty"><span className="entity-missing-icon"><Wrench /></span><div><h2>{commonTools.length === 0 ? "No common tools yet" : "No matching common tools"}</h2><p>{commonTools.length === 0 ? "Create a fixed HTTP tool or import a reviewed MCP definition for reuse across APIs." : "Change the search or lifecycle filter."}</p></div>{commonTools.length === 0 && <Button color="indigo" onClick={() => onNavigate(toolBuilderPath())}><Plus data-slot="icon" />Create common tool</Button>}</div></DataTableEmpty>}
    </DataTable>
  </>;
}


function SettingsTabs({ active, onNavigate }: { active: SettingsTab; onNavigate: (path: string) => void }) {
  return <PageTabs label="Settings sections">{SETTINGS_TABS.map((tab) => <ConsoleLink key={tab.id} path={settingsPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`} ariaCurrent={active === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</PageTabs>;
}

export function RecipesView({ integrations, analyses, recipes, busy, onCreate, onGenerate, onEdit, onRework, onApprove, onPublish }: {
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  busy: boolean;
  onCreate: (prompt: string, integrationID: string) => Promise<APIRecipe | null>;
  onGenerate: () => void;
  onEdit: (recipe: APIRecipe, markdown: string, references: APIRecipeReference[], visibility: APIRecipe["visibility"]) => void;
  onRework: (recipe: APIRecipe, instruction: string) => void;
  onApprove: (recipe: APIRecipe) => void;
  onPublish: (recipe: APIRecipe) => void;
}) {
  const [createOpen, setCreateOpen] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [createIntegrationID, setCreateIntegrationID] = useState(integrations.length === 1 ? integrations[0].id : "");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [instructions, setInstructions] = useState<Record<string, string>>({});
  const [visibilities, setVisibilities] = useState<Record<string, APIRecipe["visibility"]>>({});
  const [referenceSelections, setReferenceSelections] = useState<Record<string, string[]>>({});
  const selected = selectedID ? recipes.find((recipe) => recipe.id === selectedID) ?? null : null;

  async function createFromPrompt() {
    const value = await onCreate(prompt.trim(), createIntegrationID);
    if (!value) return;
    setPrompt("");
    setCreateOpen(false);
    setSelectedID(value.id);
  }

  if (!selected) {
    return <>
      <PageHeading eyebrow="Developer guidance" title="Recipes" action={<span className="heading-actions"><Button outline disabled={busy} onClick={onGenerate}><Sparkles data-slot="icon" />Refresh from evidence</Button><Button onClick={() => setCreateOpen(true)}><Plus data-slot="icon" />Add recipe</Button></span>} />
      <section className="panel recipe-library" aria-label="Recipes">
        {recipes.length > 0 ? <div className="recipe-library-list">
          {recipes.map((recipe) => <button type="button" className="recipe-library-row" key={recipe.id} onClick={() => setSelectedID(recipe.id)}>
            <span className="recipe-library-icon"><BookOpen /></span>
            <span className="recipe-library-copy"><strong>{recipe.title}</strong><small>{recipe.outcome}</small></span>
            <Badge color={recipe.state === "published" ? "green" : recipe.state === "approved" ? "blue" : recipe.state === "outdated" ? "red" : "amber"}>{recipe.state}</Badge>
            <span className="recipe-library-date">{new Date(recipe.updated_at).toLocaleDateString()}</span>
            <ChevronRight />
          </button>)}
        </div> : <div className="recipe-library-empty">
          <span className="empty-icon"><BookOpen /></span>
          <h2>No recipes yet</h2>
          <p>Describe a developer outcome and AI will build a grounded first draft from your documentation, APIs, connectors, and tools.</p>
          <Button onClick={() => setCreateOpen(true)}><Plus data-slot="icon" />Add recipe</Button>
        </div>}
      </section>
      <Dialog
        open={createOpen}
        onClose={setCreateOpen}
        title="Create recipe"
        description="Describe what a developer should accomplish. AI will inspect the product evidence already connected to DokoSoko and create an editable draft."
        actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !prompt.trim()} onClick={createFromPrompt}><Sparkles data-slot="icon" />{busy ? "Building…" : "Build recipe"}</Button></>}
      >
        <label className="auth-field">
          <span>API</span>
          <select value={createIntegrationID} onChange={(event) => setCreateIntegrationID(event.target.value)}>
            <option value="">Whole deployment</option>
            {integrations.filter((integration) => integration.lifecycle !== "retired").map((integration) => <option value={integration.id} key={integration.id}>{integration.display_name} · {integration.version_key}</option>)}
          </select>
          <small>Select one API to ground names, documentation, access, and automatic tools in its published contract.</small>
        </label>
        <label className="auth-field recipe-create-prompt">
          <span>What should this recipe help developers do?</span>
          <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="For example: Connect a customer’s Stripe account, sync invoices, and verify webhook delivery." />
          <small>Uses existing documentation, API definitions, service connections, MCP connectors, and tools as untrusted evidence. Unsupported claims are flagged for review.</small>
        </label>
      </Dialog>
    </>;
  }

  const revision = selected.current_revision;
  const analysis = analyses.find((value) => value.id === selected.analysis_id);
  const availableReferences = (analysis?.evidence ?? []).flatMap((evidence): APIRecipeReference[] => [
    ...(evidence.location?.startsWith("https://") ? [{ label: evidence.label, url: evidence.location, kind: evidence.location.includes("github.com") ? "code" : "documentation", resource_id: evidence.resource_id }] : []),
    ...(evidence.references ?? []),
  ]);
  const currentReferenceIDs = (revision?.references ?? []).map((reference) => reference.resource_id).filter(Boolean) as string[];
  const selectedReferenceIDs = referenceSelections[selected.id] ?? currentReferenceIDs;
  const markdown = drafts[selected.id] ?? revision?.markdown ?? "";
  const visibility = visibilities[selected.id] ?? selected.visibility;
  const references = [
    ...(revision?.references ?? []).filter((reference) => !reference.resource_id),
    ...availableReferences.filter((reference) => reference.resource_id && selectedReferenceIDs.includes(reference.resource_id)).map((reference) => revision?.references.find((current) => current.resource_id === reference.resource_id) ?? reference),
  ];
  const referencesChanged = [...selectedReferenceIDs].sort().join("\u0000") !== [...currentReferenceIDs].sort().join("\u0000");
  const dirty = markdown !== (revision?.markdown ?? "") || visibility !== selected.visibility || referencesChanged;
  const errors = revision?.validation.filter((finding) => finding.level === "error") ?? [];

  return <>
    <button type="button" className="recipe-editor-back" onClick={() => setSelectedID(null)}><ArrowLeft />All recipes</button>
    <PageHeading eyebrow="Recipe editor" title={selected.title} action={<Badge color={selected.state === "published" ? "green" : selected.state === "approved" ? "blue" : selected.state === "outdated" ? "red" : "amber"}>{selected.state}</Badge>} />
    <div className="recipe-editor-layout">
      <section className="panel recipe-document-editor">
        <div className="recipe-editor-toolbar">
          <span><strong>Markdown</strong><small>Revision {revision?.revision ?? "—"} · {revision?.generated_by === "ai" ? "AI generated" : "Human edited"}</small></span>
          <Button disabled={busy || !dirty || !markdown.trim()} onClick={() => onEdit(selected, markdown, references, visibility)}>{busy ? "Saving…" : "Save changes"}</Button>
        </div>
        <textarea className="recipe-markdown-input" aria-label="Recipe Markdown" value={markdown} onChange={(event) => setDrafts((values) => ({ ...values, [selected.id]: event.target.value }))} placeholder="Write the recipe in Markdown…" />
      </section>
      <aside className="recipe-editor-sidebar">
        <section className="panel recipe-editor-panel recipe-ai-rework">
          <label className="auth-field"><span>Ask AI to revise this recipe</span><textarea value={instructions[selected.id] ?? ""} onChange={(event) => setInstructions((values) => ({ ...values, [selected.id]: event.target.value }))} placeholder="Describe the change you want. AI will keep claims grounded in the same evidence." /></label>
          <Button outline disabled={busy || !(instructions[selected.id] ?? "").trim()} onClick={() => onRework(selected, instructions[selected.id])}><Sparkles data-slot="icon" />Create revision</Button>
        </section>
        <section className="panel recipe-editor-panel">
          <h2>Details</h2>
          <label className="auth-field"><span>Visibility</span><select value={visibility} onChange={(event) => setVisibilities((values) => ({ ...values, [selected.id]: event.target.value as APIRecipe["visibility"] }))}><option value="private">Private</option><option value="public">Public</option></select></label>
          <span className="recipe-editor-meta"><small>Stable URI</small><code>{selected.stable_uri}</code></span>
          <span className="recipe-editor-meta"><small>Audience</small><strong>{selected.audience}</strong></span>
        </section>
        <section className="panel recipe-editor-panel">
          <h2>Evidence</h2>
          {availableReferences.length > 0 ? <div className="recipe-editor-references">{availableReferences.map((reference) => <label className="compact-check" key={reference.resource_id ?? reference.url}><input type="checkbox" checked={Boolean(reference.resource_id && selectedReferenceIDs.includes(reference.resource_id))} onChange={() => { if (!reference.resource_id) return; setReferenceSelections((values) => ({ ...values, [selected.id]: selectedReferenceIDs.includes(reference.resource_id!) ? selectedReferenceIDs.filter((id) => id !== reference.resource_id) : [...selectedReferenceIDs, reference.resource_id!] })); }} /><span>{reference.label}<small>{reference.kind}</small></span></label>)}</div> : <p className="recipe-editor-muted">No external links are available. This private recipe remains grounded in {selected.dependencies.length} immutable catalog dependenc{selected.dependencies.length === 1 ? "y" : "ies"} from its analysis.</p>}
        </section>
        {(revision?.review || (revision?.validation.length ?? 0) > 0) && <section className="panel recipe-editor-panel">
          <h2>Review</h2>
          {revision?.review && <p className="recipe-review-summary">{revision.review}</p>}
          {revision?.validation.map((finding) => <div className={`recipe-editor-finding ${finding.level}`} key={`${finding.code}:${finding.message}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code.replaceAll("_", " ")}</strong><small>{finding.message}</small></span></div>)}
        </section>}
        <div className="recipe-editor-actions">
          {selected.state === "review" && <Button disabled={busy || errors.length > 0} onClick={() => onApprove(selected)}>Approve revision</Button>}
          {selected.state === "approved" && <Button color="indigo" disabled={busy} onClick={() => onPublish(selected)}>Publish recipe</Button>}
        </div>
      </aside>
    </div>
  </>;
}

export function SettingsView({ product, versions, pins, aiProfiles, rootUsers, currentUser, onDoctor, onConfigureProduct, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; versions: APIProductVersion[]; pins: APIProductVersionPin[]; aiProfiles: APIAIWorkloadProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onConfigureProduct: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <>
    <PageHeading eyebrow="Settings" title="Settings" action={<Button outline onClick={onDoctor}><Activity data-slot="icon" />Run System Doctor</Button>} />
    <SettingsTabs active="overview" onNavigate={onNavigate} />
    <div className="settings-grid">
      <button type="button" className="settings-button" aria-label="Open Service connections settings" onClick={() => onNavigate(settingsPath("connections"))}><SettingsCard icon={<KeyRound />} title="Service connections" detail="Encrypted vendor credentials shared explicitly with APIs" status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Bug reports and feedback settings" onClick={() => onNavigate(settingsPath("reporting"))}><SettingsCard icon={<MessageSquareText />} title="Bug reports & feedback" detail="Consent-gated reporting policies and secure delivery endpoints" status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Database and storage settings" onClick={() => onNavigate(settingsPath("storage"))}><SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted local object storage" status="Healthy" /></button>
      <button type="button" className="settings-button" aria-label="Open AI providers settings" onClick={() => onNavigate(settingsPath("ai"))}><SettingsCard icon={<Bot />} title="AI providers" detail={`${aiProfiles.filter((profile) => profile.enabled).length} active workload${aiProfiles.filter((profile) => profile.enabled).length === 1 ? "" : "s"} · one credential per provider`} status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Root access settings" onClick={() => onNavigate(settingsPath("root"))}><SettingsCard icon={<ShieldCheck />} title="Root access" detail={`${activeRoots.length} MFA-protected administrator${activeRoots.length === 1 ? "" : "s"} · append-only audit`} status="Secure" /></button>
    </div>
    <details className="panel advanced-details"><summary>Advanced publishing</summary><div className="advanced-details-body"><PanelHeader title="Publishing snapshots" action={<Button outline onClick={onConfigureProduct}>Open advanced publishing</Button>} /><div className="activity-summary"><span>{versions.length} published snapshot{versions.length === 1 ? "" : "s"}</span><span>{pins.length} scoped pin{pins.length === 1 ? "" : "s"}</span><span>Default {product.default_version_policy.toUpperCase()}</span></div></div></details>
    <RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} />
  </>;
}

function RootAccessPanel({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <section className="panel root-management"><PanelHeader title="Root administrators" description="Root access is independent from vendor identities and always requires MFA." action={<Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button>} />{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section>;
}

export function RootAccessSettingsView({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Root access" /><SettingsTabs active="root" onNavigate={onNavigate} /><RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} /></>;
}

export function StorageSettingsView({ onNavigate }: { onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Database & storage" /><SettingsTabs active="storage" onNavigate={onNavigate} /><section className="panel"><PanelHeader title="Storage status" action={<Badge color="green">Healthy</Badge>} /><div className="contract-grid"><span><small>Primary database</small><strong>Connected</strong></span><span><small>Object storage</small><strong>Available</strong></span><span><small>Encryption</small><strong>Active</strong></span><span><small>Schema</small><strong>Current</strong></span></div></section></>;
}

export function AISettingsView({ profiles, connections, usage, saving, onSave, onConfigure, onAddProvider, onConnect, onTest, onNavigate }: { profiles: APIAIWorkloadProfile[]; connections: APIAIProviderConnection[]; usage: APIAIProviderUsage[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void; onAddProvider: () => void; onConnect: (provider: APIAIProviderConnection["provider"]) => void; onTest: (connection: APIAIProviderConnection) => void; onNavigate: (path: string) => void }) {
  const primaryConnections = connections.filter((connection) => connection.enabled && !connection.is_backup);

  return <>
    <PageHeading eyebrow="Settings" title="AI providers" action={<Button onClick={onAddProvider}><Plus data-slot="icon" />Add provider</Button>} />
    <SettingsTabs active="ai" onNavigate={onNavigate} />

    <section className="ai-settings-section">
      <SectionHeader title="Workloads" />
      <div className="panel ai-table-panel">
		<Table label="AI workloads" dense className="ai-settings-table ai-workload-table">
		  <colgroup><col className="ai-workload-column" /><col className="ai-provider-column" /><col className="ai-model-column" /><col className="ai-actions-column" /></colgroup>
		  <TableHead><TableRow><TableHeader>Name</TableHeader><TableHeader>Provider</TableHeader><TableHeader>Model</TableHeader><TableHeader className="ai-actions-heading">Actions</TableHeader></TableRow></TableHead>
	          <TableBody>{aiWorkloads.map((workload) => { const profile = profiles.find((item) => item.workload === workload.role); return <AIWorkloadRow key={`${workload.role}:${profile?.revision ?? 0}:${primaryConnections.map((connection) => `${connection.id}-${connection.revision}`).join(",")}`} workload={workload} profile={profile} connections={primaryConnections} saving={saving} onSave={onSave} onConfigure={onConfigure} />; })}</TableBody>
        </Table>
      </div>
    </section>

    <section className="ai-settings-section">
      <SectionHeader title="Providers" action={<Button outline onClick={onAddProvider}><Plus data-slot="icon" />Add provider</Button>} />
      {connections.length === 0 ? <div className="ai-provider-suggestions">
        {aiProviders.filter((provider) => provider.id !== "openai-compatible").map((provider) => <button type="button" key={provider.id} onClick={() => onConnect(provider.id)}><AIProviderLogo provider={provider.id} /><span><strong>Connect {provider.name}</strong><small>{provider.description}</small></span><ChevronRight /></button>)}
      </div> : <div className="panel ai-table-panel">
        <Table label="AI providers" dense className="ai-settings-table ai-provider-table">
		  <colgroup><col className="ai-provider-identity-column" /><col className="ai-used-by-column" /><col className="ai-usage-column" /><col className="ai-errors-column" /><col className="ai-backup-column" /><col className="ai-provider-actions-column" /></colgroup>
		  <TableHead><TableRow><TableHeader>Provider</TableHeader><TableHeader>Used by</TableHeader><TableHeader>Usage</TableHeader><TableHeader>Errors</TableHeader><TableHeader>Backup</TableHeader><TableHeader className="ai-actions-heading">Actions</TableHeader></TableRow></TableHead>
          <TableBody>{connections.map((connection) => {
            const providerUsage = usage.find((value) => value.provider === connection.provider);
            const workloads = profiles.filter((profile) => profile.provider_connection_id === connection.id).map((profile) => aiWorkloads.find((workload) => workload.role === profile.workload)?.name ?? profile.workload);
            const canTest = connection.enabled && (connection.provider !== "openai-compatible" || workloads.length > 0 || connection.is_backup);
            return <TableRow key={connection.id}>
              <TableCell><div className="ai-provider-identity"><AIProviderLogo provider={connection.provider} /><span><strong>{aiProviderLabel(connection.provider)}</strong><small>{connection.managed_by === "environment" ? "Environment managed" : connection.enabled ? "Connected" : "Paused"}</small></span></div></TableCell>
              <TableCell>{workloads.length ? workloads.join(", ") : <span className="ai-table-muted">Not selected</span>}</TableCell>
              <TableCell><strong>{formatAIUsage(providerUsage?.input_tokens ?? 0, providerUsage?.output_tokens ?? 0)}</strong><small className="ai-table-subline">{providerUsage?.calls ?? 0} request{providerUsage?.calls === 1 ? "" : "s"}</small></TableCell>
              <TableCell>{providerUsage?.errors ?? 0}{connection.last_error_code && <small className="ai-table-subline error">{connection.last_error_code}</small>}</TableCell>
              <TableCell>{connection.is_backup ? <Badge color="violet">Backup</Badge> : <button type="button" className="ai-table-link" onClick={() => onConnect(connection.provider)}>Set up</button>}</TableCell>
              <TableCell><div className="ai-table-actions">{canTest && <Button outline onClick={() => onTest(connection)}>Test</Button>}<Button outline onClick={() => onConnect(connection.provider)}>Manage</Button></div></TableCell>
            </TableRow>;
          })}</TableBody>
        </Table>
      </div>}
      {connections.length > 0 && !connections.some((connection) => connection.is_backup) && <p className="ai-backup-hint"><ShieldCheck />Optional: designate one connected provider as a backup for transient outages.</p>}
    </section>
  </>;
}

function AIWorkloadRow({ workload, profile, connections, saving, onSave, onConfigure }: { workload: (typeof aiWorkloads)[number]; profile?: APIAIWorkloadProfile; connections: APIAIProviderConnection[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void }) {
  const initialConnection = connections.find((connection) => connection.id === profile?.provider_connection_id) ?? connections[0];
  const [connectionID, setConnectionID] = useState(initialConnection?.id ?? "");
  const [model, setModel] = useState(profile?.model ?? (initialConnection ? aiModelDefaults[initialConnection.provider][workload.role] : ""));
	  const selectedConnection = connections.find((connection) => connection.id === connectionID);
  const dirty = connectionID !== (profile?.provider_connection_id ?? "") || model !== (profile?.model ?? "") || !profile?.enabled;
  const modelOptions = selectedConnection ? aiModelOptions[selectedConnection.provider] : [];
  const visibleModels = model && !modelOptions.includes(model) ? [model, ...modelOptions] : modelOptions;
  const Icon = workload.icon;
  return <TableRow>
    <TableCell><div className="ai-workload-name"><span className="settings-icon"><Icon /></span><span><strong>{workload.name}</strong><small>{workload.description}</small></span></div></TableCell>
    <TableCell><Select aria-label={`${workload.name} provider`} disabled={connections.length === 0} value={connectionID} onChange={(event) => { const nextID = event.target.value; const next = connections.find((connection) => connection.id === nextID); setConnectionID(nextID); setModel(next ? aiModelDefaults[next.provider][workload.role] : ""); }}><option value="">Choose provider</option>{connections.map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider)}</option>)}</Select></TableCell>
    <TableCell>{selectedConnection?.provider === "openai-compatible" ? <Input aria-label={`${workload.name} model`} value={model} onChange={(event) => setModel(event.target.value)} placeholder="Provider model ID" /> : <Select aria-label={`${workload.name} model`} disabled={!selectedConnection} value={model} onChange={(event) => setModel(event.target.value)}><option value="">Choose model</option>{visibleModels.map((modelID) => <option value={modelID} key={modelID}>{modelID}</option>)}</Select>}</TableCell>
    <TableCell><div className="ai-table-actions"><Button outline disabled={!profile} onClick={() => onConfigure(workload.role)}>Limits</Button><Button color="indigo" disabled={saving || !dirty || !connectionID || !model.trim()} onClick={() => void onSave(workload.role, connectionID, model)}>{saving ? "Saving…" : "Save"}</Button></div></TableCell>
  </TableRow>;
}

export function AIProviderLogo({ provider }: { provider: APIAIProviderConnection["provider"] }) {
	  return <span className={`ai-provider-logo ${provider}`} aria-hidden="true">{provider === "openai" ? <OpenAIProviderMark /> : provider === "google" ? <GeminiProviderMark /> : provider === "anthropic" ? <ClaudeProviderMark /> : provider === "digitalocean" ? <DigitalOceanProviderMark /> : provider === "xai" ? <XAIProviderMark /> : provider === "deepseek" ? <DeepSeekProviderMark /> : <Server />}</span>;
}

function DigitalOceanProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="#0080ff"><path d="M12.04 0C5.408-.02.005 5.37.005 11.992h4.638c0-4.923 4.882-8.731 10.064-6.855a6.95 6.95 0 014.147 4.148c1.889 5.177-1.924 10.055-6.84 10.064v-4.61H7.391v4.623h4.61V24c7.86 0 13.967-7.588 11.397-15.83-1.115-3.59-3.985-6.446-7.575-7.575A12.8 12.8 0 0012.039 0zM7.39 19.362H3.828v3.564H7.39zm-3.563 0v-2.978H.85v2.978z" /></svg>;
}

function XAIProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="currentColor"><text x="1.25" y="17.2" fontSize="15.5" fontWeight="700" letterSpacing="-1.4">xAI</text></svg>;
}

function DeepSeekProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="#5786fe"><path d="M23.748 4.651c-.254-.124-.364.113-.512.233-.051.04-.094.09-.137.137-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.155-.708-.311-.955-.65-.172-.24-.219-.509-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.094.172.187.129.323-.082.28-.18.553-.266.833-.055.179-.137.218-.328.14a5.5 5.5 0 01-1.737-1.179c-.857-.828-1.631-1.743-2.597-2.46a12 12 0 00-.689-.47c-.985-.957.13-1.743.387-1.836.27-.098.094-.433-.778-.428-.872.003-1.67.295-2.687.685a3 3 0 01-.465.136 9.6 9.6 0 00-2.883-.101c-1.885.21-3.39 1.1-4.497 2.622C.082 8.776-.231 10.854.152 13.02c.403 2.284 1.568 4.175 3.36 5.653 1.857 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.132-.284 4.994-1.86.47.234.962.328 1.78.398.629.058 1.235-.031 1.705-.129.735-.155.684-.836.418-.961-2.155-1.004-1.682-.595-2.112-.926 1.095-1.295 2.768-3.598 3.284-6.733.05-.346.115-.834.108-1.114-.004-.171.035-.238.23-.257a4.2 4.2 0 001.545-.475c1.397-.763 1.96-2.016 2.093-3.517.02-.23-.004-.467-.247-.588M11.58 18.168c-2.088-1.642-3.101-2.183-3.52-2.16-.39.024-.32.472-.234.763.09.288.207.487.371.74.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.168-1.361-.801-2.5-1.86-3.301-3.306-.775-1.393-1.225-2.888-1.299-4.482-.02-.385.094-.522.477-.592a4.7 4.7 0 011.53-.038c2.131.311 3.946 1.264 5.467 2.774.868.86 1.525 1.887 2.202 2.89.72 1.066 1.494 2.082 2.48 2.915.348.291.626.513.892.677-.802.09-2.14.109-3.055-.615zm1.001-6.44a.306.306 0 01.415-.287.3.3 0 01.113.074.3.3 0 01.086.214c0 .17-.136.307-.308.307a.303.303 0 01-.306-.307" /></svg>;
}

function OpenAIProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="currentColor"><path fillRule="evenodd" d="M9.205 8.658v-2.26c0-.19.072-.333.238-.428l4.543-2.616c.619-.357 1.356-.523 2.117-.523 2.854 0 4.662 2.212 4.662 4.566 0 .167 0 .357-.024.547l-4.71-2.759a.797.797 0 00-.856 0l-5.97 3.473zm10.609 8.8V12.06c0-.333-.143-.57-.429-.737l-5.97-3.473 1.95-1.118a.433.433 0 01.476 0l4.543 2.617c1.309.76 2.189 2.378 2.189 3.948 0 1.808-1.07 3.473-2.76 4.163zM7.802 12.703l-1.95-1.142c-.167-.095-.239-.238-.239-.428V5.899c0-2.545 1.95-4.472 4.591-4.472 1 0 1.927.333 2.712.928L8.23 5.067c-.285.166-.428.404-.428.737v6.898zM12 15.128l-2.795-1.57v-3.33L12 8.658l2.795 1.57v3.33L12 15.128zm1.796 7.23c-1 0-1.927-.332-2.712-.927l4.686-2.712c.285-.166.428-.404.428-.737v-6.898l1.974 1.142c.167.095.238.238.238.428v5.233c0 2.545-1.974 4.472-4.614 4.472zm-5.637-5.303l-4.544-2.617c-1.308-.761-2.188-2.378-2.188-3.948A4.482 4.482 0 014.21 6.327v5.423c0 .333.143.571.428.738l5.947 3.449-1.95 1.118a.432.432 0 01-.476 0zm-.262 3.9c-2.688 0-4.662-2.021-4.662-4.519 0-.19.024-.38.047-.57l4.686 2.71c.286.167.571.167.856 0l5.97-3.448v2.26c0 .19-.07.333-.237.428l-4.543 2.616c-.619.357-1.356.523-2.117.523zm5.899 2.83a5.947 5.947 0 005.827-4.756C22.287 18.339 24 15.84 24 13.296c0-1.665-.713-3.282-1.998-4.448.119-.5.19-.999.19-1.498 0-3.401-2.759-5.947-5.946-5.947-.642 0-1.26.095-1.88.31A5.962 5.962 0 0010.205 0a5.947 5.947 0 00-5.827 4.757C1.713 5.447 0 7.945 0 10.49c0 1.666.713 3.283 1.998 4.448-.119.5-.19 1-.19 1.499 0 3.401 2.759 5.946 5.946 5.946.642 0 1.26-.095 1.88-.309a5.96 5.96 0 004.162 1.713z" /></svg>;
}

function GeminiProviderMark() {
	  return <svg viewBox="0 0 24 24"><path fill="#3186ff" d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" /></svg>;
}

function ClaudeProviderMark() {
	  return <svg viewBox="0 0 24 24"><path fill="#d97757" d="M4.709 15.955l4.72-2.647.08-.23-.08-.128H9.2l-.79-.048-2.698-.073-2.339-.097-2.266-.122-.571-.121L0 11.784l.055-.352.48-.321.686.06 1.52.103 2.278.158 1.652.097 2.449.255h.389l.055-.157-.134-.098-.103-.097-2.358-1.596-2.552-1.688-1.336-.972-.724-.491-.364-.462-.158-1.008.656-.722.881.06.225.061.893.686 1.908 1.476 2.491 1.833.365.304.145-.103.019-.073-.164-.274-1.355-2.446-1.446-2.49-.644-1.032-.17-.619a2.97 2.97 0 01-.104-.729L6.283.134 6.696 0l.996.134.42.364.62 1.414 1.002 2.229 1.555 3.03.456.898.243.832.091.255h.158V9.01l.128-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.584.28.48.685-.067.444-.286 1.851-.559 2.903-.364 1.942h.212l.243-.242.985-1.306 1.652-2.064.73-.82.85-.904.547-.431h1.033l.76 1.129-.34 1.166-1.064 1.347-.881 1.142-1.264 1.7-.79 1.36.073.11.188-.02 2.856-.606 1.543-.28 1.841-.315.833.388.091.395-.328.807-1.969.486-2.309.462-3.439.813-.042.03.049.061 1.549.146.662.036h1.622l3.02.225.79.522.474.638-.079.485-1.215.62-1.64-.389-3.829-.91-1.312-.329h-.182v.11l1.093 1.068 2.006 1.81 2.509 2.33.127.578-.322.455-.34-.049-2.205-1.657-.851-.747-1.926-1.62h-.128v.17l.444.649 2.345 3.521.122 1.08-.17.353-.608.213-.668-.122-1.374-1.925-1.415-2.167-1.143-1.943-.14.08-.674 7.254-.316.37-.729.28-.607-.461-.322-.747.322-1.476.389-1.924.315-1.53.286-1.9.17-.632-.012-.042-.14.018-1.434 1.967-2.18 2.945-1.726 1.845-.414.164-.717-.37.067-.662.401-.589 2.388-3.036 1.44-1.882.93-1.086-.006-.158h-.055L4.132 18.56l-1.13.146-.487-.456.061-.746.231-.243 1.908-1.312-.006.006z" /></svg>;
}

function formatAIUsage(input: number, output: number) {
  const total = input + output;
  if (total >= 1_000_000) return `${(total / 1_000_000).toFixed(total >= 10_000_000 ? 0 : 1)}M tokens`;
  if (total >= 1_000) return `${(total / 1_000).toFixed(total >= 10_000 ? 0 : 1)}K tokens`;
  return `${total} tokens`;
}
