import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Bot, BookOpen, ChevronRight, Copy, Database, Plus, RefreshCw, Server,
  Share2, ShieldCheck, Sparkles, TerminalSquare, Trash2, TriangleAlert, Wrench,
} from "lucide-react";
import { useEffect, useState } from "react";

import { APIError, api } from "../../lib/api";
import type {
  APIGrantDefinition,
  APIAIProviderConnection, APIAIProviderUsage, APIAIWorkflowPrompt, APIAIWorkloadProfile, APIAuditEvent,
  APIIntegration, APIIntegrationAnalysis, APIMCPConnection, APIMCPPreview, APINativePlugin,
  APIProduct, APIRecipe, APISupportSubmission, APITool, APIUser,
} from "../../lib/api";
import { SETTINGS_TABS, type SettingsTab, entityPath, sectionPath, settingsPath, toolBuilderPath } from "../../lib/console-routes";
import { Badge, Button, Dialog } from "../core/control";
import { Input, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../core";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader, SectionHeader } from "../core/layout";
import { IntegrationEvidenceGaps } from "../integrations/IntegrationEvidenceGaps";
import { toolIsCommon } from "../integrations/tool-scope";
import { RecipeApprovalDialog } from "./dialogs/recipe-approval-dialog";
import { createRecipeApprovalReview, type RecipeApprovalReview } from "./dialogs/recipe-approval-review";
import {
  type AIWorkload, ConsoleLink, EntityLink, SettingsCard, aiModelDefaults,
  aiModelOptions, aiProviderDescription, aiProviderLabel, aiProviders, aiWorkloadDescription, aiWorkloadName, aiWorkloads, analysisMatchesIntegration,
  recipeHasScopeDependencyMismatch, recipeMatchesIntegration, recipeScopeIDs, toolPolicy, toolStateLabel,
} from "./shared";

function ToolsWorkspaceTabs({ active, onNavigate }: { active: "catalog" | "connections" | "preview"; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <PageTabs label={t("settings.toolAreas")}><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className={`page-tab ${active === "catalog" ? "active" : ""}`}>{t("settings.catalog")}</ConsoleLink><ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className={`page-tab ${active === "connections" ? "active" : ""}`}>{t("settings.mcpConnections")}</ConsoleLink><ConsoleLink path={sectionPath("mcp-preview")} onNavigate={onNavigate} className={`page-tab ${active === "preview" ? "active" : ""}`}>{t("navigation.mcpPreview")}</ConsoleLink></PageTabs>;
}

export function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect, onNavigate }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <><PageHeading eyebrow={t("navigation.tools")} title={t("settings.mcpConnections")} action={<Button onClick={onAdd}><Plus data-slot="icon" />{t("settings.connectMCP")}</Button>} /><ToolsWorkspaceTabs active="connections" onNavigate={onNavigate} /><section className="panel"><PanelHeader title={t("settings.upstreamMCPServers")} description={t("settings.eachFixedEndpointUsesOneEncryptedAccessTokenAnd")} />{connections.map((connection) => { const imported = tools.filter((tool) => tool.mcp_connection_id === connection.id); return <div className="provider-row" key={connection.id}><span className="settings-icon"><Share2 /></span><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.endpoint} · {imported.length} {t("settings.importedTool")}{imported.length === 1 ? "" : t("settings.s")}</small></span>{connection.forward_user_identity && <Badge color="violet">{t("settings.signedIdentity")}</Badge>}<Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><Button outline disabled={busy} onClick={() => onInspect(connection)}>{t("settings.inspectTools")}</Button></div>; })}{connections.length === 0 && <div className="empty-row">{t("settings.noUpstreamMCPConnectionIsConfigured")}</div>}</section></>;
}

export function ToolsView({ tools, integrations, connections, nativePlugins, onSetNativePluginEnabled, onNavigate }: { tools: APITool[]; integrations: APIIntegration[]; connections: APIMCPConnection[]; nativePlugins: APINativePlugin[]; onSetNativePluginEnabled: (pluginID: string, enabled: boolean) => Promise<void>; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const normalized = query.trim().toLowerCase();
  const visible = tools.filter((tool) => !normalized || `${tool.namespace}.${tool.name} ${tool.description}`.toLowerCase().includes(normalized));
  return <><PageHeading eyebrow={t("navigation.tools")} title={t("navigation.tools")} action={<ConsoleLink path={toolBuilderPath()} onNavigate={onNavigate} className="core-button core-button-dark"><Plus data-slot="icon" />{t("settings.createHTTPTool")}</ConsoleLink>} /><ToolsWorkspaceTabs active="catalog" onNavigate={onNavigate} />
    <div className="toolbar"><div className="search-field"><input aria-label={t("settings.searchTools")} placeholder={t("settings.searchTools2")} value={query} onChange={(event) => setQuery(event.target.value)} /></div></div>
    <DataTable label={t("settings.toolCatalog")}><DataTableHeader className="tool-columns"><span>{t("settings.tool")}</span><span>{t("settings.backend")}</span><span>{t("settings.policy")}</span><span>{t("settings.state")}</span><span>{t("settings.open")}</span></DataTableHeader>{visible.map((tool) => { const policy = toolPolicy(tool); return <DataTableRow className="tool-columns" key={tool.id}><span className="resource-name"><span className="resource-icon"><TerminalSquare /></span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{tool.owner_integration_id ? integrations.find((item) => item.id === tool.owner_integration_id)?.display_name ?? t("settings.apiOwned") : toolIsCommon(tool) ? t("settings.common") : t("settings.scoped")}</small></span></span><span>{tool.backend_kind === "mcp" ? connections.find((item) => item.id === tool.mcp_connection_id)?.name ?? "MCP" : tool.http_method}</span><span>{t("settings.riskAndGrants", { risk: policy.risk, count: policy.requiredGrants.length })}</span><Badge color={tool.state === "published" && !tool.upstream_drifted ? "green" : tool.upstream_drifted ? "red" : "amber"}>{tool.upstream_drifted ? t("settings.drifted") : toolStateLabel(tool, t)}</Badge><ConsoleLink path={entityPath("tool", tool.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={t("settings.openTool", { name: `${tool.namespace}.${tool.name}` })}><ChevronRight /></ConsoleLink></DataTableRow>; })}{visible.length === 0 && <DataTableEmpty columns={5}>{t("settings.noToolsMatchThisSearch")}</DataTableEmpty>}</DataTable>
    <section className="panel"><PanelHeader title={t("settings.nativeTools")} description={t("settings.reviewedInProcessCapabilitiesRegisteredByTheService")} />{nativePlugins.map((plugin) => <div className="provider-row" key={plugin.id}><span className="settings-icon"><Wrench /></span><span><strong>{plugin.id}</strong><small>{plugin.description}</small></span><Badge color={plugin.state === "active" ? "green" : "zinc"}>{plugin.state}</Badge><Button outline onClick={() => void onSetNativePluginEnabled(plugin.id, plugin.state !== "active")}>{plugin.state === "active" ? t("settings.disable") : t("settings.enable")}</Button></div>)}{nativePlugins.length === 0 && <div className="empty-row">{t("settings.noNativePluginIsRegistered")}</div>}</section>
  </>;
}

const mcpPreviewMethods: APIMCPPreview["method"][] = ["server/discover", "tools/list", "resources/list", "resources/templates/list"];

function mcpPreviewMethodLabel(method: APIMCPPreview["method"], t: TFunction) {
  return method === "server/discover" ? t("settings.serverDiscovery")
    : method === "tools/list" ? t("settings.toolsList")
      : method === "resources/list" ? t("settings.resourcesList")
        : t("settings.resourceTemplates");
}

function previewCollectionCount(preview: APIMCPPreview | null): number | null {
  if (!preview) return null;
  const result = preview.response.result;
  if (!result || typeof result !== "object" || Array.isArray(result)) return null;
  const collectionKey = preview.method === "tools/list" ? "tools" : preview.method === "resources/list" ? "resources" : preview.method === "resources/templates/list" ? "resourceTemplates" : "";
  if (!collectionKey) return null;
  const collection = (result as Record<string, unknown>)[collectionKey];
  return Array.isArray(collection) ? collection.length : null;
}

export function MCPPreviewView({ product, grants, grantStatus, available, privateEndpointEnabled, onMessage, onNavigate }: {
  product: APIProduct;
  grants: APIGrantDefinition[];
  grantStatus: "loading" | "ready" | "unavailable";
  available: boolean;
  privateEndpointEnabled: boolean;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [audience, setAudience] = useState<APIMCPPreview["audience"]>("private");
  const [method, setMethod] = useState<APIMCPPreview["method"]>("tools/list");
  const [selectedGrants, setSelectedGrants] = useState<string[]>([]);
  const [preview, setPreview] = useState<APIMCPPreview | null>(null);
  const [loading, setLoading] = useState(available);
  const [error, setError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const activeGrants = grants.filter((grant) => grant.state === "active");
  const endpointEnabled = audience === "public" ? product.public_mcp_enabled : privateEndpointEnabled;
  const collectionCount = previewCollectionCount(preview);

  useEffect(() => {
    if (!available) return;
    let cancelled = false;
    void api.mcpPreview(product.id, audience, method, audience === "private" ? selectedGrants : []).then((value) => {
      if (!cancelled) {
        setPreview(value);
        setError("");
      }
    }).catch((reason: unknown) => {
      if (!cancelled) {
        setPreview(null);
        setError(reason instanceof APIError ? reason.message : t("settings.theMCPPreviewCouldNotBeRendered"));
      }
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [audience, available, method, product.id, refreshKey, selectedGrants, t]);

  function beginPreviewChange() {
    setPreview(null);
    setLoading(available);
    setError("");
  }

  function toggleGrant(key: string, checked: boolean) {
    beginPreviewChange();
    setSelectedGrants((current) => checked ? [...current, key].sort() : current.filter((value) => value !== key));
  }

  async function copyResponse() {
    if (!preview) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(preview.response, null, 2));
      onMessage(t("settings.mcpResponseCopied"));
    } catch {
      onMessage(t("settings.mcpResponseCouldNotBeCopied"));
    }
  }

  return <>
    <PageHeading eyebrow={t("navigation.tools")} title={t("navigation.mcpPreview")} action={<Button outline disabled={!available || loading} onClick={() => { beginPreviewChange(); setRefreshKey((value) => value + 1); }}><RefreshCw data-slot="icon" className={loading ? "spin" : ""} />{t("settings.refresh")}</Button>} />
    <ToolsWorkspaceTabs active="preview" onNavigate={onNavigate} />
    <section className="panel mcp-preview-context">
      <PanelHeader title={t("settings.previewContext")} description={t("settings.publicDiscoveryIsAnonymousPrivateDiscoveryUsesOnlyThe")} />
      <div className="mcp-preview-controls">
        <label className="auth-field"><span>{t("settings.audience")}</span><Select aria-label={t("settings.mcpPreviewAudience")} value={audience} onChange={(event) => { beginPreviewChange(); const next = event.target.value as APIMCPPreview["audience"]; setAudience(next); if (next === "public") setSelectedGrants([]); }}><option value="private">{t("settings.privateMCP")}</option><option value="public">{t("settings.publicMCP")}</option></Select><small>{audience === "private" ? t("settings.authenticatedSimulatedAuthorizationContext") : t("settings.anonymousPublicProjection")}</small></label>
        <label className="auth-field"><span>{t("settings.method")}</span><Select aria-label={t("settings.mcpPreviewMethod")} value={method} onChange={(event) => { beginPreviewChange(); setMethod(event.target.value as APIMCPPreview["method"]); }}>{mcpPreviewMethods.map((item) => <option key={item} value={item}>{mcpPreviewMethodLabel(item, t)} · {item}</option>)}</Select><small>{t("settings.onlyDiscoveryMethodsAreAvailableToolsCallIsNever")}</small></label>
      </div>
      {audience === "private" && <fieldset className="mcp-preview-grants"><legend>{t("settings.simulatedCustomerGrants")} <Badge color="violet">{t("settings.selectedGrants", { count: selectedGrants.length })}</Badge></legend>{grantStatus === "loading" ? <p>{t("settings.loadingTheActiveGrantRegistryUntilItIsReady")}</p> : grantStatus === "unavailable" ? <p>{t("settings.theActiveGrantRegistryIsUnavailableThisPreviewIs")}</p> : activeGrants.length > 0 ? <div>{activeGrants.map((grant) => { const controlID = `mcp-preview-grant-${grant.id}`; return <label key={grant.id} htmlFor={controlID}><input id={controlID} type="checkbox" aria-label={t("settings.copy", { display_name: String(grant.display_name), key: String(grant.key) })} checked={selectedGrants.includes(grant.key)} onChange={(event) => toggleGrant(grant.key, event.target.checked)} /><span><strong>{grant.display_name}</strong><code>{grant.key}</code></span></label>; })}</div> : <p>{t("settings.noActiveGrantsAreRegisteredThisIsAnExact")}</p>}</fieldset>}
    </section>
    <section className="panel mcp-preview-output">
      <PanelHeader title={t("settings.exactJSONRPCResponse")} description={preview ? t("settings.copy2", { method: String(preview.method), endpoint: String(preview.endpoint) }) : t("settings.selectAContextToRenderTheResponse")} action={<span className="heading-actions"><Badge color={endpointEnabled ? "green" : "amber"}>{endpointEnabled ? t("settings.endpointLive") : t("settings.endpointNotLive")}</Badge><Button outline disabled={!preview || loading} onClick={() => void copyResponse()}><Copy data-slot="icon" />{t("settings.copyJSON")}</Button></span>} />
      {preview && <dl className="mcp-preview-summary"><div><dt>{t("settings.catalogRevision")}</dt><dd>{product.catalog_revision}</dd></div><div><dt>{t("settings.protocol")}</dt><dd><code>{preview.protocol_version}</code></dd></div><div><dt>{t("settings.items")}</dt><dd>{collectionCount ?? "—"}</dd></div><div><dt>{t("settings.generated")}</dt><dd>{t("format.dateTime", { value: new Date(preview.generated_at) })}</dd></div></dl>}
      {!available && <div className="capability-unavailable" role="status"><TriangleAlert /><span><strong>{t("settings.livePreviewIsUnavailableInFixtureMode")}</strong><small>{t("settings.openTheConnectedConsoleToInspectTheCurrentMCP")}</small></span></div>}
      {error && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>{t("settings.previewUnavailable")}</strong><small>{error}</small></span></div>}
      {loading && !preview && <div className="empty-row"><RefreshCw className="spin" />{t("settings.renderingTheMCPResponse")}</div>}
      {preview && <pre className="mcp-preview-json" role="region" aria-label={t("settings.exactJSONRPCResponse2", { method: String(preview.method) })}><code>{JSON.stringify(preview.response, null, 2)}</code></pre>}
    </section>
    {preview && <details className="panel advanced-details mcp-preview-request"><summary>{t("settings.viewExactJSONRPCRequest")}</summary><pre className="mcp-preview-json"><code>{JSON.stringify(preview.request, null, 2)}</code></pre></details>}
  </>;
}

export function RecipesView({ integrations, analyses, recipes, busy, onCreate, onGenerate, onEdit, onRework, onDelete, onApprove, onPublish }: {
  integrations: APIIntegration[];
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  busy: boolean;
  onCreate: () => void;
  onGenerate: (integrationID: string) => void;
  onEdit: (recipe: APIRecipe) => void;
  onRework: (recipe: APIRecipe) => void;
  onDelete: (recipe: APIRecipe) => boolean | Promise<boolean>;
  onApprove: (recipe: APIRecipe) => void | Promise<void>;
  onPublish: (recipe: APIRecipe) => void;
}) {
  const { t } = useTranslation();
  const [selectedIntegrationID, setSelectedIntegrationID] = useState("");
  const [approvalReview, setApprovalReview] = useState<RecipeApprovalReview | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<APIRecipe | null>(null);
  const [deleteAcknowledged, setDeleteAcknowledged] = useState(false);
  const activeIntegrationID = integrations.some((integration) => integration.id === selectedIntegrationID) ? selectedIntegrationID : "";
  const selectedAnalysis = activeIntegrationID
    ? analyses
      .filter((analysis) => analysis.state === "review" && analysisMatchesIntegration(analysis, activeIntegrationID))
      .sort((left, right) => right.created_at.localeCompare(left.created_at))[0]
    : undefined;
  const unscopedOrInvalidRecipes = recipes.filter((recipe) => {
    const scopeIDs = recipeScopeIDs(recipe);
    return recipeHasScopeDependencyMismatch(recipe) || scopeIDs.length === 0 || scopeIDs.some((id) => !integrations.some((integration) => integration.id === id));
  });
  const invalidRecipeIDs = new Set(unscopedOrInvalidRecipes.map((recipe) => recipe.id));
  const visibleRecipes = activeIntegrationID
    ? recipes.filter((recipe) => !invalidRecipeIDs.has(recipe.id) && recipeMatchesIntegration(recipe, activeIntegrationID))
    : recipes.filter((recipe) => !invalidRecipeIDs.has(recipe.id));

  function renderRecipe(recipe: APIRecipe) {
    const scopeIDs = recipeScopeIDs(recipe);
    const scopedIntegrations = scopeIDs.map((id) => integrations.find((integration) => integration.id === id)).filter((value): value is APIIntegration => Boolean(value));
    const invalidContract = !(
      recipe.contract_version === "deployment-recipe-v3" && recipe.current_revision?.spec_version === 3
      || recipe.contract_version === "product-integration-v2" && recipe.current_revision?.spec_version === 2
    );
    const dependencyMismatch = recipeHasScopeDependencyMismatch(recipe);
    const exactCurrentRevisionAvailable = Boolean(recipe.current_revision && recipe.current_revision.id === recipe.current_revision_id && recipe.current_revision.recipe_id === recipe.id);
    const approvalCandidate = createRecipeApprovalReview(recipe, integrations);
    const invalidScope = invalidContract || dependencyMismatch || scopeIDs.length === 0 || scopedIntegrations.length !== scopeIDs.length || !approvalCandidate;
    const deletable = recipe.contract_version === "legacy-mcp-v1" || recipe.state === "outdated";
    const scopeLabel = scopeIDs.length === 0
      ? t("settings.deploymentWide")
      : scopedIntegrations.map((integration) => integration.display_name).join(" + ") || t("settings.unknownAPIScope");
    return <div className="provider-row" key={recipe.id}>
      <span className="settings-icon"><BookOpen /></span>
      <span><strong>{recipe.title}</strong><small>{recipe.outcome} · {scopeLabel}</small></span>
      <span className="tool-badges">
        {invalidContract ? <Badge color="red">{t("settings.legacyContract")}</Badge> : dependencyMismatch ? <Badge color="red">{t("settings.scopeDependencyMismatch")}</Badge> : !exactCurrentRevisionAvailable ? <Badge color="red">{t("settings.revisionUnavailable")}</Badge> : !approvalCandidate ? <Badge color="red">{t("settings.revisionBindingMismatch")}</Badge> : invalidScope && <Badge color="red">{t("settings.invalidScope")}</Badge>}
        {recipe.needs_attention && <Badge color="amber">{t("settings.needsReview")}</Badge>}
        <Badge color={recipe.state === "published" ? "green" : recipe.state === "approved" ? "blue" : "zinc"}>{recipe.state}</Badge>
      </span>
      <span className="table-actions">
        <Button outline disabled={busy || invalidScope} onClick={() => onEdit(recipe)}>{t("settings.edit")}</Button>
        {recipe.needs_attention && <Button outline disabled={busy || invalidScope} onClick={() => onRework(recipe)}>{t("settings.rework")}</Button>}
        {deletable && <Button outline className="recipe-delete-button" disabled={busy} onClick={() => { setDeleteTarget(recipe); setDeleteAcknowledged(false); }}><Trash2 data-slot="icon" />{t("settings.delete")}</Button>}
        {recipe.state === "review" && <Button disabled={busy || invalidScope} onClick={() => setApprovalReview(approvalCandidate)}>{t("settings.review")}</Button>}
        {recipe.state === "approved" && <Button disabled={busy || invalidScope} onClick={() => onPublish(recipe)}>{t("settings.publish")}</Button>}
      </span>
    </div>;
  }

  return <>
    <PageHeading
      eyebrow={t("settings.authoring")}
      title={t("navigation.recipes")}
      action={<span className="heading-actions">
        <Button outline disabled={busy || !activeIntegrationID} onClick={() => onGenerate(activeIntegrationID)}><Sparkles data-slot="icon" />{t("settings.generateFromEvidence")}</Button>
        <Button disabled={busy} onClick={onCreate}><Plus data-slot="icon" />{t("settings.createRecipe")}</Button>
      </span>}
    />
    <section className="panel">
      <PanelHeader title={t("settings.recipeScope")} description={t("settings.eachRecipeImplementsOneTangibleProductCapabilityAndIs")} />
      <div className="recipe-scope-body">
        <label className="auth-field"><span>API</span><Select aria-label={t("settings.recipeAPI")} value={activeIntegrationID} onChange={(event) => setSelectedIntegrationID(event.target.value)}><option value="">{t("integrations.allAPIs")}</option>{integrations.map((integration) => <option key={integration.id} value={integration.id}>{integration.display_name} · {integration.version_key}</option>)}</Select></label>
        <IntegrationEvidenceGaps unknowns={selectedAnalysis?.unknowns ?? []} />
      </div>
    </section>
    <section className="panel">
      <PanelHeader title={t("settings.codingAgentImplementationRecipes")} description={t("settings.minimalProductIntegrationStepsDeliveredAfterTheCodingAgent")} />
      {visibleRecipes.map(renderRecipe)}
      {visibleRecipes.length === 0 && <div className="empty-row">{activeIntegrationID ? t("settings.noRecipesForThisAPIYet") : t("settings.noRecipesYet")}</div>}
    </section>
    {unscopedOrInvalidRecipes.length > 0 && <section className="panel">
      <PanelHeader title={t("settings.deploymentWideAndScopeExceptions")} description={t("settings.legacyDeploymentWideRecipesAndRecordsWithAnInvalid")} />
      {unscopedOrInvalidRecipes.map(renderRecipe)}
    </section>}
    <RecipeApprovalDialog review={approvalReview} busy={busy} onClose={() => setApprovalReview(null)} onApprove={onApprove} />
    <Dialog
      open={Boolean(deleteTarget)}
      onClose={(open) => { if (!open && !busy) { setDeleteTarget(null); setDeleteAcknowledged(false); } }}
      title={t("settings.deleteRecipeTitle", { title: deleteTarget?.title ?? t("navigation.recipes") })}
      description={t("settings.deleteRecipeDescription")}
      actions={<><Button outline disabled={busy} onClick={() => { setDeleteTarget(null); setDeleteAcknowledged(false); }}>{t("common.cancel")}</Button><Button color="red" disabled={busy || !deleteAcknowledged || !deleteTarget} onClick={() => { if (!deleteTarget) return; void Promise.resolve(onDelete(deleteTarget)).then((shouldClose) => { if (shouldClose) { setDeleteTarget(null); setDeleteAcknowledged(false); } }); }}>{busy ? t("settings.deleting") : t("settings.deleteRecipe")}</Button></>}
    >
      <div className="auth-form compact-form"><div className="notice"><TriangleAlert /><span><strong>{t("settings.deleteRecipeCannotBeUndone")}</strong> {t("settings.deleteRecipeHistoryWarning")}</span></div><label className="compact-check"><input type="checkbox" checked={deleteAcknowledged} onChange={(event) => setDeleteAcknowledged(event.target.checked)} /><span>{t("settings.deleteRecipeAcknowledgement")}</span></label></div>
    </Dialog>
  </>;
}

export function OutboxView({ submissions, events, onView, onNavigate }: { submissions: APISupportSubmission[]; events: APIAuditEvent[]; onView: (submission: APISupportSubmission) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <><PageHeading eyebrow={t("settings.operations")} title={t("navigation.supportOutbox")} /><section className="panel"><PanelHeader title={t("settings.queuedSubmissions")} />{submissions.map((submission) => <button type="button" className="provider-row" key={submission.id} onClick={() => onView(submission)}><span className="settings-icon"><BookOpen /></span><span><strong>{submission.summary}</strong><small>{submission.trusted_integration?.display_name ?? t("settings.deployment")} · {t("format.dateTime", { value: new Date(submission.created_at) })}</small></span><Badge color="blue">{t("settings.queued")}</Badge><ChevronRight /></button>)}{submissions.length === 0 && <div className="empty-row">{t("settings.theOutboxIsEmpty")}</div>}</section><details className="panel advanced-details"><summary>{t("settings.recentAudit")}{events.length})</summary><div className="advanced-details-body">{events.slice(0, 30).map((event) => <ConsoleLink key={event.id} path={entityPath("audit-event", event.id)} onNavigate={onNavigate} className="provider-row"><span><strong>{event.action}</strong><small>{event.target_type} · {event.target_id}</small></span><small>{t("format.dateTime", { value: new Date(event.created_at) })}</small></ConsoleLink>)}</div></details></>;
}

function SettingsTabs({ active, onNavigate }: { active: SettingsTab; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <PageTabs label={t("settings.settingsAreas")}>{SETTINGS_TABS.map((tab) => <ConsoleLink key={tab.id} path={settingsPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`}>{t(tab.label)}</ConsoleLink>)}</PageTabs>;
}

export function SettingsView({ product, aiProfiles, rootUsers, currentUser, onDoctor, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; aiProfiles: APIAIWorkloadProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  const activeWorkloads = aiProfiles.filter((profile) => profile.enabled).length;
  return <><PageHeading eyebrow={t("settings.administration")} title={t("navigation.settings")} action={<Button outline onClick={onDoctor}>{t("settings.runSystemDoctor")}</Button>} /><SettingsTabs active="overview" onNavigate={onNavigate} /><div className="settings-grid"><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("storage"))}><SettingsCard icon={<Database />} title={t("settings.databaseStorage")} detail={t("settings.postgreSQLMigrationsAndEncryptedSecretStorage")} status={t("settings.healthy")} /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("ai"))}><SettingsCard icon={<Bot />} title={t("settings.aiConfiguration")} detail={t("settings.activeWorkloadsAndVersionedPrompts", { count: activeWorkloads })} status={t("settings.manage")} statusColor="zinc" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("root"))}><SettingsCard icon={<ShieldCheck />} title={t("settings.rootAccess")} detail={t("settings.mfaProtectedAdministrators", { count: activeRoots.length })} status={t("settings.secure")} /></button></div><section className="panel"><PanelHeader title={t("settings.deployment")} /><dl className="entity-detail-grid"><div><dt>{t("settings.name")}</dt><dd>{product.name}</dd></div><div><dt>{t("settings.catalogRevision")}</dt><dd>{product.catalog_revision}</dd></div><div><dt>{t("settings.publicMCP")}</dt><dd>{product.public_mcp_enabled ? t("settings.enabled") : t("settings.disabled")}</dd></div></dl></section><RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} /></>;
}

function RootAccessPanel({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <section className="panel root-management"><PanelHeader title={t("settings.rootAdministrators")} action={<Button onClick={onAddRoot}><Plus data-slot="icon" />{t("settings.addRoot")}</Button>} />{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? t("settings.revoked") : t("settings.mfaActive")}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>{t("settings.revoke")}</Button> : <span />}</div>)}</section>;
}

export function RootAccessSettingsView(props: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <><PageHeading eyebrow={t("navigation.settings")} title={t("settings.rootAccess")} /><SettingsTabs active="root" onNavigate={props.onNavigate} /><RootAccessPanel {...props} /></>;
}

export function StorageSettingsView({ onNavigate }: { onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <><PageHeading eyebrow={t("navigation.settings")} title={t("settings.databaseStorage")} /><SettingsTabs active="storage" onNavigate={onNavigate} /><section className="panel"><PanelHeader title={t("settings.storageStatus")} action={<Badge color="green">{t("settings.healthy")}</Badge>} /><div className="contract-grid"><span><small>{t("settings.primaryDatabase")}</small><strong>{t("settings.connected")}</strong></span><span><small>{t("settings.secretStorage")}</small><strong>{t("settings.encrypted")}</strong></span><span><small>{t("settings.schema")}</small><strong>{t("settings.current")}</strong></span></div></section></>;
}

const aiPromptOrder: APIAIWorkflowPrompt["key"][] = [
  "integration.analysis",
  "recipe.brief",
  "recipe.authoring",
  "recipe.review",
  "documentation.map_enrichment",
  "sdk.map_enrichment",
  "sdk.applicability_suggestion",
  "sdk.sample_review",
];

export function AISettingsView({ profiles, prompts, connections, usage, saving, onSave, onConfigure, onEditPrompt, onAddProvider, onConnect, onTest, onNavigate }: { profiles: APIAIWorkloadProfile[]; prompts: APIAIWorkflowPrompt[]; connections: APIAIProviderConnection[]; usage: APIAIProviderUsage[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void; onEditPrompt: (prompt: APIAIWorkflowPrompt) => void; onAddProvider: () => void; onConnect: (provider: APIAIProviderConnection["provider"]) => void; onTest: (connection: APIAIProviderConnection) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const primary = connections.filter((connection) => connection.enabled && !connection.is_backup);
  const orderedPrompts = [...prompts].sort((left, right) => aiPromptOrder.indexOf(left.key) - aiPromptOrder.indexOf(right.key));
  return <><PageHeading eyebrow={t("navigation.settings")} title={t("settings.aiConfiguration")} action={<Button onClick={onAddProvider}><Plus data-slot="icon" />{t("settings.addProvider")}</Button>} /><SettingsTabs active="ai" onNavigate={onNavigate} /><SectionHeader title={t("settings.workload")} /><div className="panel ai-table-panel"><Table label={t("settings.aiWorkload")} dense><TableHead><TableRow><TableHeader>{t("settings.name")}</TableHeader><TableHeader>{t("settings.provider")}</TableHeader><TableHeader>{t("settings.model")}</TableHeader><TableHeader>{t("settings.actions")}</TableHeader></TableRow></TableHead><TableBody>{aiWorkloads.map((workload) => { const profile = profiles.find((item) => item.workload === workload.role); const configurationKey = `${workload.role}:${profile?.revision ?? 0}:${profile?.provider_connection_id ?? ""}:${primary.map((connection) => connection.id).join(",")}`; return <AIWorkloadRow key={configurationKey} workload={workload} profile={profile} connections={primary} saving={saving} onSave={onSave} onConfigure={onConfigure} />; })}</TableBody></Table></div><SectionHeader title={t("settings.workflowPrompts")} description={t("settings.versionedInstructionsForAnalysisRecipeDeveloperAssetEnrichmentApplicabil")} /><div className="panel ai-table-panel"><Table label={t("settings.aiWorkflowPrompts")} dense><TableHead><TableRow><TableHeader>{t("settings.workflow")}</TableHeader><TableHeader>{t("settings.sourceAndVersion")}</TableHeader><TableHeader>{t("settings.updated")}</TableHeader><TableHeader>{t("settings.actions")}</TableHeader></TableRow></TableHead><TableBody>{orderedPrompts.map((prompt) => <TableRow key={prompt.key}><TableCell><strong>{prompt.label}</strong><small className="ai-table-subline">{prompt.description}</small></TableCell><TableCell><Badge color={prompt.source === "override" ? "violet" : "green"}>{prompt.source === "override" ? t("settings.override") : t("settings.default")} · {prompt.effective_version}</Badge><small className="ai-table-subline">{t("settings.default")} {prompt.default_version}</small></TableCell><TableCell>{prompt.updated_at ? t("format.dateTime", { value: new Date(prompt.updated_at) }) : t("settings.builtIn")}</TableCell><TableCell><Button outline onClick={() => onEditPrompt(prompt)}>{t("settings.editInstructions")}</Button></TableCell></TableRow>)}{orderedPrompts.length === 0 && <TableRow><TableCell colSpan={4}>{t("settings.workflowPromptsAreUnavailable")}</TableCell></TableRow>}</TableBody></Table></div><SectionHeader title={t("settings.providers")} />{connections.length === 0 ? <div className="ai-provider-suggestions">{aiProviders.filter((provider) => provider.id !== "openai-compatible").map((provider) => <button type="button" key={provider.id} onClick={() => onConnect(provider.id)}><AIProviderLogo provider={provider.id} /><span><strong>{t("settings.connect")} {aiProviderLabel(provider.id, t)}</strong><small>{aiProviderDescription(provider.id, t)}</small></span><ChevronRight /></button>)}</div> : <section className="panel">{connections.map((connection) => { const stats = usage.find((item) => item.provider === connection.provider); return <div className="provider-row" key={connection.id}><AIProviderLogo provider={connection.provider} /><span><strong>{aiProviderLabel(connection.provider, t)}</strong><small>{stats?.calls ?? 0} {t("settings.calls")} {stats?.input_tokens ?? 0} {t("settings.inputTokens")} {stats?.output_tokens ?? 0} {t("settings.outputTokens")}</small></span>{connection.is_backup && <Badge color="violet">{t("settings.backup")}</Badge>}<Badge color={connection.enabled ? "green" : "zinc"}>{connection.enabled ? t("settings.connected") : t("settings.paused")}</Badge><Button outline onClick={() => onTest(connection)}>{t("settings.test")}</Button><Button outline onClick={() => onConnect(connection.provider)}>{t("settings.manage")}</Button></div>; })}</section>}</>;
}

function AIWorkloadRow({ workload, profile, connections, saving, onSave, onConfigure }: { workload: (typeof aiWorkloads)[number]; profile?: APIAIWorkloadProfile; connections: APIAIProviderConnection[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void }) {
  const { t } = useTranslation();
  const initial = connections.find((connection) => connection.id === profile?.provider_connection_id) ?? connections[0];
  const [connectionID, setConnectionID] = useState(initial?.id ?? "");
  const [model, setModel] = useState(profile?.model ?? (initial ? aiModelDefaults[initial.provider][workload.role] : ""));
  const selected = connections.find((connection) => connection.id === connectionID);
  const models = selected ? aiModelOptions[selected.provider] : [];
  return <TableRow><TableCell><strong>{aiWorkloadName(workload.role, t)}</strong><small className="ai-table-subline">{aiWorkloadDescription(workload.role, t)}</small></TableCell><TableCell><Select value={connectionID} onChange={(event) => { const id = event.target.value; const connection = connections.find((item) => item.id === id); setConnectionID(id); setModel(connection ? aiModelDefaults[connection.provider][workload.role] : ""); }}><option value="">{t("settings.chooseProvider")}</option>{connections.map((connection) => <option key={connection.id} value={connection.id}>{aiProviderLabel(connection.provider, t)}</option>)}</Select></TableCell><TableCell>{selected?.provider === "openai-compatible" ? <Input value={model} onChange={(event) => setModel(event.target.value)} /> : <Select value={model} onChange={(event) => setModel(event.target.value)}><option value="">{t("settings.chooseModel")}</option>{models.map((id) => <option value={id} key={id}>{id}</option>)}</Select>}</TableCell><TableCell><div className="ai-table-actions"><Button outline disabled={!profile} onClick={() => onConfigure(workload.role)}>{t("settings.limits")}</Button><Button disabled={saving || !connectionID || !model} onClick={() => void onSave(workload.role, connectionID, model)}>{t("common.save")}</Button></div></TableCell></TableRow>;
}

export function AIProviderLogo({ provider }: { provider: APIAIProviderConnection["provider"] }) {
  const { t } = useTranslation();
  return <span className={`ai-provider-logo ${provider}`} aria-hidden="true">{provider === "openai" ? "◉" : provider === "google" ? "✦" : provider === "anthropic" ? t("settings.a") : provider === "digitalocean" ? t("settings.do") : provider === "xai" ? "xAI" : provider === "deepseek" ? t("settings.ds") : <Server />}</span>;
}
