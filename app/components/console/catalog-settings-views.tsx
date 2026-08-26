import {
  Bot, BookOpen, ChevronRight, Database, Plus, Server,
  Share2, ShieldCheck, Sparkles, TerminalSquare, Wrench,
} from "lucide-react";
import { useState } from "react";

import type {
  APIAIProviderConnection, APIAIProviderUsage, APIAIWorkflowPrompt, APIAIWorkloadProfile, APIAuditEvent,
  APIIntegration, APIIntegrationAnalysis, APIMCPConnection, APINativePlugin,
  APIProduct, APIRecipe, APISupportSubmission, APITool, APIUser,
} from "../../lib/api";
import { SETTINGS_TABS, type SettingsTab, entityPath, sectionPath, settingsPath, toolBuilderPath } from "../../lib/console-routes";
import { Badge, Button } from "../core/control";
import { Input, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../core";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader, SectionHeader } from "../core/layout";
import { IntegrationEvidenceGaps } from "../integrations/IntegrationEvidenceGaps";
import { toolIsCommon } from "../integrations/tool-scope";
import {
  type AIWorkload, ConsoleLink, EntityLink, SettingsCard, aiModelDefaults,
  activeRecipeIntegrationID, aiModelOptions, aiProviderLabel, aiProviders, aiWorkloads, analysisMatchesIntegration,
  recipeHasScopeDependencyMismatch, recipeMatchesIntegration, recipeScopeIDs, toolPolicy, toolStateLabel,
} from "./shared";

function ToolsWorkspaceTabs({ active, onNavigate }: { active: "catalog" | "connections"; onNavigate: (path: string) => void }) {
  return <PageTabs label="Tool areas"><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className={`page-tab ${active === "catalog" ? "active" : ""}`}>Catalog</ConsoleLink><ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className={`page-tab ${active === "connections" ? "active" : ""}`}>MCP connections</ConsoleLink></PageTabs>;
}

export function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect, onNavigate }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Tools" title="MCP connections" action={<Button onClick={onAdd}><Plus data-slot="icon" />Connect MCP</Button>} /><ToolsWorkspaceTabs active="connections" onNavigate={onNavigate} /><section className="panel"><PanelHeader title="Upstream MCP servers" description="Each fixed endpoint uses one encrypted access token and can optionally receive a signed user-identity envelope." />{connections.map((connection) => { const imported = tools.filter((tool) => tool.mcp_connection_id === connection.id); return <div className="provider-row" key={connection.id}><span className="settings-icon"><Share2 /></span><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.endpoint} · {imported.length} imported tool{imported.length === 1 ? "" : "s"}</small></span>{connection.forward_user_identity && <Badge color="violet">signed identity</Badge>}<Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><Button outline disabled={busy} onClick={() => onInspect(connection)}>Inspect tools</Button></div>; })}{connections.length === 0 && <div className="empty-row">No upstream MCP connection is configured.</div>}</section></>;
}

export function ToolsView({ tools, integrations, connections, nativePlugins, onSetNativePluginEnabled, onNavigate }: { tools: APITool[]; integrations: APIIntegration[]; connections: APIMCPConnection[]; nativePlugins: APINativePlugin[]; onSetNativePluginEnabled: (pluginID: string, enabled: boolean) => Promise<void>; onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const normalized = query.trim().toLowerCase();
  const visible = tools.filter((tool) => !normalized || `${tool.namespace}.${tool.name} ${tool.description}`.toLowerCase().includes(normalized));
  return <><PageHeading eyebrow="Execution" title="Tools" action={<ConsoleLink path={toolBuilderPath()} onNavigate={onNavigate} className="core-button core-button-dark"><Plus data-slot="icon" />Create HTTP tool</ConsoleLink>} /><ToolsWorkspaceTabs active="catalog" onNavigate={onNavigate} />
    <div className="toolbar"><div className="search-field"><input aria-label="Search tools" placeholder="Search tools…" value={query} onChange={(event) => setQuery(event.target.value)} /></div></div>
    <DataTable label="Tool catalog"><DataTableHeader className="tool-columns"><span>Tool</span><span>Backend</span><span>Policy</span><span>State</span><span>Open</span></DataTableHeader>{visible.map((tool) => { const policy = toolPolicy(tool); return <DataTableRow className="tool-columns" key={tool.id}><span className="resource-name"><span className="resource-icon"><TerminalSquare /></span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{tool.owner_integration_id ? integrations.find((item) => item.id === tool.owner_integration_id)?.display_name ?? "API-owned" : toolIsCommon(tool) ? "Common" : "Scoped"}</small></span></span><span>{tool.backend_kind === "mcp" ? connections.find((item) => item.id === tool.mcp_connection_id)?.name ?? "MCP" : tool.http_method}</span><span>{policy.risk} · {policy.requiredGrants.length} grant{policy.requiredGrants.length === 1 ? "" : "s"}</span><Badge color={tool.state === "published" && !tool.upstream_drifted ? "green" : tool.upstream_drifted ? "red" : "amber"}>{tool.upstream_drifted ? "drifted" : toolStateLabel(tool)}</Badge><ConsoleLink path={entityPath("tool", tool.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${tool.namespace}.${tool.name}`}><ChevronRight /></ConsoleLink></DataTableRow>; })}{visible.length === 0 && <DataTableEmpty columns={5}>No tools match this search.</DataTableEmpty>}</DataTable>
    <section className="panel"><PanelHeader title="Native tools" description="Reviewed in-process capabilities registered by the service." />{nativePlugins.map((plugin) => <div className="provider-row" key={plugin.id}><span className="settings-icon"><Wrench /></span><span><strong>{plugin.id}</strong><small>{plugin.description}</small></span><Badge color={plugin.state === "active" ? "green" : "zinc"}>{plugin.state}</Badge><Button outline onClick={() => void onSetNativePluginEnabled(plugin.id, plugin.state !== "active")}>{plugin.state === "active" ? "Disable" : "Enable"}</Button></div>)}{nativePlugins.length === 0 && <div className="empty-row">No native plugin is registered.</div>}</section>
  </>;
}

export function RecipesView({ integrations, analyses, recipes, busy, onCreate, onGenerate, onEdit, onRework, onApprove, onPublish }: {
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  busy: boolean;
  onCreate: (integrationID: string) => void;
  onGenerate: (integrationID: string) => void;
  onEdit: (recipe: APIRecipe) => void;
  onRework: (recipe: APIRecipe) => void;
  onApprove: (recipe: APIRecipe) => void;
  onPublish: (recipe: APIRecipe) => void;
}) {
  const [selectedIntegrationID, setSelectedIntegrationID] = useState("");
  const activeIntegrationID = activeRecipeIntegrationID(integrations, selectedIntegrationID);
  const selectedAnalysis = activeIntegrationID
    ? analyses
      .filter((analysis) => analysis.state === "review" && analysisMatchesIntegration(analysis, activeIntegrationID))
      .sort((left, right) => right.created_at.localeCompare(left.created_at))[0]
    : undefined;
  const unscopedOrInvalidRecipes = recipes.filter((recipe) => {
    const scopeIDs = recipeScopeIDs(recipe);
    return recipeHasScopeDependencyMismatch(recipe) || scopeIDs.length !== 1 || !integrations.some((integration) => integration.id === scopeIDs[0]);
  });
  const invalidRecipeIDs = new Set(unscopedOrInvalidRecipes.map((recipe) => recipe.id));
  const visibleRecipes = activeIntegrationID
    ? recipes.filter((recipe) => !invalidRecipeIDs.has(recipe.id) && recipeMatchesIntegration(recipe, activeIntegrationID))
    : [];

  function renderRecipe(recipe: APIRecipe) {
    const scopeIDs = recipeScopeIDs(recipe);
    const scopedIntegration = scopeIDs.length === 1 ? integrations.find((integration) => integration.id === scopeIDs[0]) : undefined;
    const invalidContract = recipe.contract_version !== "product-integration-v2" || recipe.current_revision?.spec_version !== 2;
    const dependencyMismatch = recipeHasScopeDependencyMismatch(recipe);
    const invalidScope = invalidContract || dependencyMismatch || scopeIDs.length !== 1 || !scopedIntegration;
    const scopeLabel = scopeIDs.length === 0
      ? "Deployment-wide"
      : scopeIDs.length > 1
        ? "Multiple API scopes"
        : scopedIntegration?.display_name ?? "Unknown API scope";
    return <div className="provider-row" key={recipe.id}>
      <span className="settings-icon"><BookOpen /></span>
      <span><strong>{recipe.title}</strong><small>{recipe.outcome} · {scopeLabel}</small></span>
      <span className="tool-badges">
        {invalidContract ? <Badge color="red">legacy contract</Badge> : dependencyMismatch ? <Badge color="red">scope dependency mismatch</Badge> : invalidScope && <Badge color="red">invalid scope</Badge>}
        {recipe.needs_attention && <Badge color="amber">needs review</Badge>}
        <Badge color={recipe.state === "published" ? "green" : recipe.state === "approved" ? "blue" : "zinc"}>{recipe.state}</Badge>
      </span>
      <span className="table-actions">
        <Button outline disabled={busy || invalidScope} onClick={() => onEdit(recipe)}>Edit</Button>
        {recipe.needs_attention && <Button outline disabled={busy || invalidScope} onClick={() => onRework(recipe)}>Rework</Button>}
        {recipe.state === "review" && <Button disabled={busy || invalidScope} onClick={() => onApprove(recipe)}>Approve</Button>}
        {recipe.state === "approved" && <Button disabled={busy || invalidScope} onClick={() => onPublish(recipe)}>Publish</Button>}
      </span>
    </div>;
  }

  return <>
    <PageHeading
      eyebrow="Authoring"
      title="Recipes"
      action={<span className="heading-actions">
        <Button outline disabled={busy || !activeIntegrationID} onClick={() => onGenerate(activeIntegrationID)}><Sparkles data-slot="icon" />Generate from evidence</Button>
        <Button disabled={busy || !activeIntegrationID} onClick={() => onCreate(activeIntegrationID)}><Plus data-slot="icon" />Create recipe</Button>
      </span>}
    />
    <section className="panel">
      <PanelHeader title="Recipe scope" description="Each recipe implements one tangible product capability and is grounded in this API's exact reviewed evidence." />
      <div className="recipe-scope-body">
        <label className="auth-field"><span>API</span><Select aria-label="Recipe API" value={activeIntegrationID} onChange={(event) => setSelectedIntegrationID(event.target.value)}><option value="">Choose an API</option>{integrations.map((integration) => <option key={integration.id} value={integration.id}>{integration.display_name} · {integration.version_key}</option>)}</Select></label>
        <IntegrationEvidenceGaps unknowns={selectedAnalysis?.unknowns ?? []} />
      </div>
    </section>
    <section className="panel">
      <PanelHeader title="Coding-agent implementation recipes" description="Minimal product-integration steps delivered after the coding agent connects through MCP." />
      {visibleRecipes.map(renderRecipe)}
      {visibleRecipes.length === 0 && <div className="empty-row">{activeIntegrationID ? "No recipes for this API yet." : "Choose an API to review its recipes."}</div>}
    </section>
    {unscopedOrInvalidRecipes.length > 0 && <section className="panel">
      <PanelHeader title="Deployment-wide and scope exceptions" description="Legacy deployment-wide recipes and records with an invalid or unavailable API scope remain visible for review." />
      {unscopedOrInvalidRecipes.map(renderRecipe)}
    </section>}
  </>;
}

export function OutboxView({ submissions, events, onView, onNavigate }: { submissions: APISupportSubmission[]; events: APIAuditEvent[]; onView: (submission: APISupportSubmission) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Operations" title="Support outbox" /><section className="panel"><PanelHeader title="Queued submissions" />{submissions.map((submission) => <button type="button" className="provider-row" key={submission.id} onClick={() => onView(submission)}><span className="settings-icon"><BookOpen /></span><span><strong>{submission.summary}</strong><small>{submission.trusted_integration?.display_name ?? "Deployment"} · {new Date(submission.created_at).toLocaleString()}</small></span><Badge color="blue">queued</Badge><ChevronRight /></button>)}{submissions.length === 0 && <div className="empty-row">The outbox is empty.</div>}</section><details className="panel advanced-details"><summary>Recent audit ({events.length})</summary><div className="advanced-details-body">{events.slice(0, 30).map((event) => <ConsoleLink key={event.id} path={entityPath("audit-event", event.id)} onNavigate={onNavigate} className="provider-row"><span><strong>{event.action}</strong><small>{event.target_type} · {event.target_id}</small></span><small>{new Date(event.created_at).toLocaleString()}</small></ConsoleLink>)}</div></details></>;
}

function SettingsTabs({ active, onNavigate }: { active: SettingsTab; onNavigate: (path: string) => void }) {
  return <PageTabs label="Settings areas">{SETTINGS_TABS.map((tab) => <ConsoleLink key={tab.id} path={settingsPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`}>{tab.label}</ConsoleLink>)}</PageTabs>;
}

export function SettingsView({ product, aiProfiles, rootUsers, currentUser, onDoctor, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; aiProfiles: APIAIWorkloadProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <><PageHeading eyebrow="Administration" title="Settings" action={<Button outline onClick={onDoctor}>Run system doctor</Button>} /><SettingsTabs active="overview" onNavigate={onNavigate} /><div className="settings-grid"><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("storage"))}><SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted secret storage" status="Healthy" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("ai"))}><SettingsCard icon={<Bot />} title="AI configuration" detail={`${aiProfiles.filter((profile) => profile.enabled).length} active workload${aiProfiles.filter((profile) => profile.enabled).length === 1 ? "" : "s"} · versioned prompts`} status="Manage" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("root"))}><SettingsCard icon={<ShieldCheck />} title="Root access" detail={`${activeRoots.length} MFA-protected administrator${activeRoots.length === 1 ? "" : "s"}`} status="Secure" /></button></div><section className="panel"><PanelHeader title="Deployment" /><dl className="entity-detail-grid"><div><dt>Name</dt><dd>{product.name}</dd></div><div><dt>Catalog revision</dt><dd>{product.catalog_revision}</dd></div><div><dt>Public MCP</dt><dd>{product.public_mcp_enabled ? "Enabled" : "Disabled"}</dd></div></dl></section><RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} /></>;
}

function RootAccessPanel({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <section className="panel root-management"><PanelHeader title="Root administrators" action={<Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button>} />{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section>;
}

export function RootAccessSettingsView(props: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Root access" /><SettingsTabs active="root" onNavigate={props.onNavigate} /><RootAccessPanel {...props} /></>;
}

export function StorageSettingsView({ onNavigate }: { onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Database & storage" /><SettingsTabs active="storage" onNavigate={onNavigate} /><section className="panel"><PanelHeader title="Storage status" action={<Badge color="green">Healthy</Badge>} /><div className="contract-grid"><span><small>Primary database</small><strong>Connected</strong></span><span><small>Secret storage</small><strong>Encrypted</strong></span><span><small>Schema</small><strong>Current</strong></span></div></section></>;
}

const aiPromptOrder: APIAIWorkflowPrompt["key"][] = [
  "integration.analysis",
  "recipe.brief",
  "recipe.authoring",
  "recipe.review",
];

export function AISettingsView({ profiles, prompts, connections, usage, saving, onSave, onConfigure, onEditPrompt, onAddProvider, onConnect, onTest, onNavigate }: { profiles: APIAIWorkloadProfile[]; prompts: APIAIWorkflowPrompt[]; connections: APIAIProviderConnection[]; usage: APIAIProviderUsage[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void; onEditPrompt: (prompt: APIAIWorkflowPrompt) => void; onAddProvider: () => void; onConnect: (provider: APIAIProviderConnection["provider"]) => void; onTest: (connection: APIAIProviderConnection) => void; onNavigate: (path: string) => void }) {
  const primary = connections.filter((connection) => connection.enabled && !connection.is_backup);
  const orderedPrompts = [...prompts].sort((left, right) => aiPromptOrder.indexOf(left.key) - aiPromptOrder.indexOf(right.key));
  return <><PageHeading eyebrow="Settings" title="AI configuration" action={<Button onClick={onAddProvider}><Plus data-slot="icon" />Add provider</Button>} /><SettingsTabs active="ai" onNavigate={onNavigate} /><SectionHeader title="Workload" /><div className="panel ai-table-panel"><Table label="AI workload" dense><TableHead><TableRow><TableHeader>Name</TableHeader><TableHeader>Provider</TableHeader><TableHeader>Model</TableHeader><TableHeader>Actions</TableHeader></TableRow></TableHead><TableBody>{aiWorkloads.map((workload) => { const profile = profiles.find((item) => item.workload === workload.role); const configurationKey = `${workload.role}:${profile?.revision ?? 0}:${profile?.provider_connection_id ?? ""}:${primary.map((connection) => connection.id).join(",")}`; return <AIWorkloadRow key={configurationKey} workload={workload} profile={profile} connections={primary} saving={saving} onSave={onSave} onConfigure={onConfigure} />; })}</TableBody></Table></div><SectionHeader title="Workflow prompts" description="Versioned instructions for the four core Analysis workflows. DokoSoko always applies its built-in safety policy separately." /><div className="panel ai-table-panel"><Table label="AI workflow prompts" dense><TableHead><TableRow><TableHeader>Workflow</TableHeader><TableHeader>Source and version</TableHeader><TableHeader>Updated</TableHeader><TableHeader>Actions</TableHeader></TableRow></TableHead><TableBody>{orderedPrompts.map((prompt) => <TableRow key={prompt.key}><TableCell><strong>{prompt.label}</strong><small className="ai-table-subline">{prompt.description}</small></TableCell><TableCell><Badge color={prompt.source === "override" ? "violet" : "green"}>{prompt.source === "override" ? "Override" : "Default"} · {prompt.effective_version}</Badge><small className="ai-table-subline">Default {prompt.default_version}</small></TableCell><TableCell>{prompt.updated_at ? new Date(prompt.updated_at).toLocaleString() : "Built in"}</TableCell><TableCell><Button outline onClick={() => onEditPrompt(prompt)}>Edit instructions</Button></TableCell></TableRow>)}{orderedPrompts.length === 0 && <TableRow><TableCell colSpan={4}>Workflow prompts are unavailable.</TableCell></TableRow>}</TableBody></Table></div><SectionHeader title="Providers" />{connections.length === 0 ? <div className="ai-provider-suggestions">{aiProviders.filter((provider) => provider.id !== "openai-compatible").map((provider) => <button type="button" key={provider.id} onClick={() => onConnect(provider.id)}><AIProviderLogo provider={provider.id} /><span><strong>Connect {provider.name}</strong><small>{provider.description}</small></span><ChevronRight /></button>)}</div> : <section className="panel">{connections.map((connection) => { const stats = usage.find((item) => item.provider === connection.provider); return <div className="provider-row" key={connection.id}><AIProviderLogo provider={connection.provider} /><span><strong>{aiProviderLabel(connection.provider)}</strong><small>{stats?.calls ?? 0} calls · {stats?.input_tokens ?? 0} input tokens · {stats?.output_tokens ?? 0} output tokens</small></span>{connection.is_backup && <Badge color="violet">Backup</Badge>}<Badge color={connection.enabled ? "green" : "zinc"}>{connection.enabled ? "Connected" : "Paused"}</Badge><Button outline onClick={() => onTest(connection)}>Test</Button><Button outline onClick={() => onConnect(connection.provider)}>Manage</Button></div>; })}</section>}</>;
}

function AIWorkloadRow({ workload, profile, connections, saving, onSave, onConfigure }: { workload: (typeof aiWorkloads)[number]; profile?: APIAIWorkloadProfile; connections: APIAIProviderConnection[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void }) {
  const initial = connections.find((connection) => connection.id === profile?.provider_connection_id) ?? connections[0];
  const [connectionID, setConnectionID] = useState(initial?.id ?? "");
  const [model, setModel] = useState(profile?.model ?? (initial ? aiModelDefaults[initial.provider][workload.role] : ""));
  const selected = connections.find((connection) => connection.id === connectionID);
  const models = selected ? aiModelOptions[selected.provider] : [];
  return <TableRow><TableCell><strong>{workload.name}</strong><small className="ai-table-subline">{workload.description}</small></TableCell><TableCell><Select value={connectionID} onChange={(event) => { const id = event.target.value; const connection = connections.find((item) => item.id === id); setConnectionID(id); setModel(connection ? aiModelDefaults[connection.provider][workload.role] : ""); }}><option value="">Choose provider</option>{connections.map((connection) => <option key={connection.id} value={connection.id}>{aiProviderLabel(connection.provider)}</option>)}</Select></TableCell><TableCell>{selected?.provider === "openai-compatible" ? <Input value={model} onChange={(event) => setModel(event.target.value)} /> : <Select value={model} onChange={(event) => setModel(event.target.value)}><option value="">Choose model</option>{models.map((id) => <option value={id} key={id}>{id}</option>)}</Select>}</TableCell><TableCell><div className="ai-table-actions"><Button outline disabled={!profile} onClick={() => onConfigure(workload.role)}>Limits</Button><Button disabled={saving || !connectionID || !model} onClick={() => void onSave(workload.role, connectionID, model)}>Save</Button></div></TableCell></TableRow>;
}

export function AIProviderLogo({ provider }: { provider: APIAIProviderConnection["provider"] }) {
  return <span className={`ai-provider-logo ${provider}`} aria-hidden="true">{provider === "openai" ? "◉" : provider === "google" ? "✦" : provider === "anthropic" ? "A" : provider === "digitalocean" ? "DO" : provider === "xai" ? "xAI" : provider === "deepseek" ? "DS" : <Server />}</span>;
}
