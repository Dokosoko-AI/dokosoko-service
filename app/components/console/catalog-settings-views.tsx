import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Bot, BookOpen, Building2, ChevronRight, Copy, Plus, RefreshCw, ScrollText, Server, SlidersHorizontal,
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
  recipeHasScopeDependencyMismatch, recipeScopeIDs, toolPolicy, toolStateLabel,
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
  const [generateDialogOpen, setGenerateDialogOpen] = useState(false);
  const [generationIntegrationID, setGenerationIntegrationID] = useState("");
  const [approvalReview, setApprovalReview] = useState<RecipeApprovalReview | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<APIRecipe | null>(null);
  const [deleteAcknowledged, setDeleteAcknowledged] = useState(false);
  const selectedAnalysis = generationIntegrationID
    ? analyses
      .filter((analysis) => analysis.state === "review" && analysisMatchesIntegration(analysis, generationIntegrationID))
      .sort((left, right) => right.created_at.localeCompare(left.created_at))[0]
    : undefined;

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
        <Button outline disabled={busy || integrations.length === 0} onClick={() => setGenerateDialogOpen(true)}><Sparkles data-slot="icon" />{t("settings.generateFromEvidence")}</Button>
        <Button disabled={busy} onClick={onCreate}><Plus data-slot="icon" />{t("settings.createRecipe")}</Button>
      </span>}
    />
    <section className="panel">
      <PanelHeader title={t("settings.recipeCatalog")} description={t("settings.recipeCatalogDescription")} />
      {recipes.map(renderRecipe)}
      {recipes.length === 0 && <div className="empty-row">{t("settings.noRecipesYet")}</div>}
    </section>
    <Dialog
      open={generateDialogOpen}
      onClose={(open) => { if (!open && !busy) setGenerateDialogOpen(false); }}
      title={t("settings.generateFromEvidence")}
      description={t("settings.generateFromEvidenceDescription")}
      actions={<><Button outline disabled={busy} onClick={() => setGenerateDialogOpen(false)}>{t("common.cancel")}</Button><Button disabled={busy || !generationIntegrationID} onClick={() => { const integrationID = generationIntegrationID; setGenerateDialogOpen(false); setGenerationIntegrationID(""); onGenerate(integrationID); }}>{t("settings.generateFromEvidence")}</Button></>}
    >
      <div className="auth-form compact-form recipe-generation-dialog">
        <label className="auth-field"><span>{t("settings.recipeAPI")}</span><Select aria-label={t("settings.recipeAPI")} value={generationIntegrationID} onChange={(event) => setGenerationIntegrationID(event.target.value)}><option value="">{t("settings.chooseAnAPI")}</option>{integrations.map((integration) => <option key={integration.id} value={integration.id}>{integration.display_name} · {integration.version_key}</option>)}</Select></label>
        <IntegrationEvidenceGaps unknowns={selectedAnalysis?.unknowns ?? []} />
      </div>
    </Dialog>
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
  return <><PageHeading eyebrow={t("settings.operations")} title={t("navigation.supportOutbox")} /><section className="panel"><PanelHeader title={t("settings.queuedSubmissions")} />{submissions.map((submission) => { const label = submission.state === "queued" ? t("settings.queued") : submission.state === "delivering" ? t("settings.delivering") : submission.state === "delivered" ? t("settings.delivered") : t("agentAccess.failed"); const color = submission.state === "delivered" ? "green" : submission.state === "failed" ? "red" : submission.state === "delivering" ? "amber" : "blue"; return <button type="button" className="provider-row" key={submission.id} onClick={() => onView(submission)}><span className="settings-icon"><BookOpen /></span><span><strong>{submission.summary}</strong><small>{submission.trusted_integration?.display_name ?? t("settings.deployment")} · {t("format.dateTime", { value: new Date(submission.created_at) })}</small></span><Badge color={color}>{label}</Badge><ChevronRight /></button>; })}{submissions.length === 0 && <div className="empty-row">{t("settings.theOutboxIsEmpty")}</div>}</section><details className="panel advanced-details"><summary>{t("settings.recentAudit")}{events.length})</summary><div className="advanced-details-body">{events.slice(0, 30).map((event) => <ConsoleLink key={event.id} path={entityPath("audit-event", event.id)} onNavigate={onNavigate} className="provider-row audit-event-row"><span className="settings-icon"><ScrollText /></span><span><strong>{event.action}</strong><small>{event.target_type} · {event.target_id}</small></span><small className="audit-event-time">{t("format.dateTime", { value: new Date(event.created_at) })}</small><ChevronRight /></ConsoleLink>)}</div></details></>;
}

export function SettingsTabs({ active, onNavigate }: { active: SettingsTab; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <PageTabs label={t("settings.settingsAreas")}>{SETTINGS_TABS.map((tab) => <ConsoleLink key={tab.id} path={settingsPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`}>{t(tab.label)}</ConsoleLink>)}</PageTabs>;
}

export function SettingsView({ aiProfiles, rootUsers, onDoctor, onNavigate }: { aiProfiles: APIAIWorkloadProfile[]; rootUsers: APIUser[]; onDoctor: () => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  const activeWorkloads = aiProfiles.filter((profile) => profile.enabled).length;
  return <><PageHeading eyebrow={t("settings.administration")} title={t("navigation.settings")} action={<Button outline onClick={onDoctor}>{t("settings.runSystemDoctor")}</Button>} /><SettingsTabs active="overview" onNavigate={onNavigate} /><div className="settings-grid"><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("tenant"))}><SettingsCard icon={<Building2 />} title={t("routes.tenantSettings")} detail={t("tenantSettings.overviewDescription")} status={t("settings.manage")} statusColor="zinc" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("configuration"))}><SettingsCard icon={<SlidersHorizontal />} title={t("routes.configuration")} detail={t("configurationSettings.overviewDescription")} status={t("configurationSettings.readOnly")} statusColor="zinc" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("ai"))}><SettingsCard icon={<Bot />} title={t("settings.aiConfiguration")} detail={t("settings.activeWorkloadsAndVersionedPrompts", { count: activeWorkloads })} status={t("settings.manage")} statusColor="zinc" /></button><button type="button" className="settings-button" onClick={() => onNavigate(settingsPath("root"))}><SettingsCard icon={<ShieldCheck />} title={t("settings.rootAccess")} detail={t("settings.mfaProtectedAdministrators", { count: activeRoots.length })} status={t("settings.secure")} /></button></div></>;
}

function RootAccessPanel({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <section className="panel root-management"><PanelHeader title={t("settings.rootAdministrators")} action={<Button onClick={onAddRoot}><Plus data-slot="icon" />{t("settings.addRoot")}</Button>} />{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? t("settings.revoked") : t("settings.mfaActive")}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>{t("settings.revoke")}</Button> : <span />}</div>)}</section>;
}

export function RootAccessSettingsView(props: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <><PageHeading eyebrow={t("navigation.settings")} title={t("settings.rootAccess")} /><SettingsTabs active="root" onNavigate={props.onNavigate} /><RootAccessPanel {...props} /></>;
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
  return <>
    <PageHeading eyebrow={t("navigation.settings")} title={t("settings.aiConfiguration")} action={<Button onClick={onAddProvider}><Plus data-slot="icon" />{t("settings.addProvider")}</Button>} />
    <SettingsTabs active="ai" onNavigate={onNavigate} />
    <SectionHeader title={t("settings.workload")} />
    <div className="panel ai-table-panel">
      <Table label={t("settings.aiWorkload")} dense>
        <TableHead><TableRow><TableHeader>{t("settings.name")}</TableHeader><TableHeader>{t("settings.provider")}</TableHeader><TableHeader>{t("settings.model")}</TableHeader><TableHeader>{t("settings.actions")}</TableHeader></TableRow></TableHead>
        <TableBody>{aiWorkloads.map((workload) => { const profile = profiles.find((item) => item.workload === workload.role); const configurationKey = `${workload.role}:${profile?.revision ?? 0}:${profile?.provider_connection_id ?? ""}:${primary.map((connection) => connection.id).join(",")}`; return <AIWorkloadRow key={configurationKey} workload={workload} profile={profile} connections={primary} saving={saving} onSave={onSave} onConfigure={onConfigure} />; })}</TableBody>
      </Table>
    </div>
    <SectionHeader title={t("settings.providers")} />
    {connections.length === 0
      ? <div className="ai-provider-suggestions">{aiProviders.filter((provider) => provider.id !== "openai-compatible").map((provider) => <button type="button" key={provider.id} onClick={() => onConnect(provider.id)}><AIProviderLogo provider={provider.id} /><span><strong>{t("settings.connect")} {aiProviderLabel(provider.id, t)}</strong><small>{aiProviderDescription(provider.id, t)}</small></span><ChevronRight /></button>)}</div>
      : <section className="panel">{connections.map((connection) => { const stats = usage.find((item) => item.provider === connection.provider); return <div className="provider-row ai-provider-row" key={connection.id}><AIProviderLogo provider={connection.provider} /><span><strong>{aiProviderLabel(connection.provider, t)}</strong><small>{stats?.calls ?? 0} {t("settings.calls")} {stats?.input_tokens ?? 0} {t("settings.inputTokens")} {stats?.output_tokens ?? 0} {t("settings.outputTokens")}</small></span><span className="tool-badges">{connection.is_backup && <Badge color="violet">{t("settings.backup")}</Badge>}<Badge color={connection.enabled ? "green" : "zinc"}>{connection.enabled ? t("settings.connected") : t("settings.paused")}</Badge></span><span className="ai-provider-row-actions"><Button outline onClick={() => onTest(connection)}>{t("settings.test")}</Button><Button outline onClick={() => onConnect(connection.provider)}>{t("settings.manage")}</Button></span></div>; })}</section>}
    <AIWorkflowPromptsAdvanced prompts={prompts} onEditPrompt={onEditPrompt} />
  </>;
}

function AIWorkflowPromptsAdvanced({ prompts, onEditPrompt }: { prompts: APIAIWorkflowPrompt[]; onEditPrompt: (prompt: APIAIWorkflowPrompt) => void }) {
  const { t } = useTranslation();
  const orderedPrompts = [...prompts].sort((left, right) => aiPromptOrder.indexOf(left.key) - aiPromptOrder.indexOf(right.key));
  return <details className="panel advanced-details ai-workflow-prompts-advanced">
    <summary>{t("settings.advanced")}</summary>
    <div className="advanced-details-body ai-workflow-prompts-advanced-body">
      <SectionHeader title={t("settings.workflowPrompts")} description={t("settings.versionedInstructionsForAnalysisRecipeDeveloperAssetEnrichmentApplicabil")} />
      <div className="ai-table-panel ai-workflow-prompts-table">
        <Table label={t("settings.aiWorkflowPrompts")} dense>
          <TableHead><TableRow><TableHeader>{t("settings.workflow")}</TableHeader><TableHeader>{t("settings.sourceAndVersion")}</TableHeader><TableHeader>{t("settings.updated")}</TableHeader><TableHeader>{t("settings.actions")}</TableHeader></TableRow></TableHead>
          <TableBody>{orderedPrompts.map((prompt) => <TableRow key={prompt.key}><TableCell><strong>{prompt.label}</strong><small className="ai-table-subline">{prompt.description}</small></TableCell><TableCell><Badge color={prompt.source === "override" ? "violet" : "green"}>{prompt.source === "override" ? t("settings.override") : t("settings.default")} · {prompt.effective_version}</Badge><small className="ai-table-subline">{t("settings.default")} {prompt.default_version}</small></TableCell><TableCell>{prompt.updated_at ? t("format.dateTime", { value: new Date(prompt.updated_at) }) : t("settings.builtIn")}</TableCell><TableCell><Button outline onClick={() => onEditPrompt(prompt)}>{t("settings.editInstructions")}</Button></TableCell></TableRow>)}{orderedPrompts.length === 0 && <TableRow><TableCell colSpan={4}>{t("settings.workflowPromptsAreUnavailable")}</TableCell></TableRow>}</TableBody>
        </Table>
      </div>
    </div>
  </details>;
}

function AIWorkloadRow({ workload, profile, connections, saving, onSave, onConfigure }: { workload: (typeof aiWorkloads)[number]; profile?: APIAIWorkloadProfile; connections: APIAIProviderConnection[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void }) {
  const { t } = useTranslation();
  const initial = connections.find((connection) => connection.id === profile?.provider_connection_id) ?? connections[0];
  const [connectionID, setConnectionID] = useState(initial?.id ?? "");
  const [model, setModel] = useState(profile?.model ?? (initial ? aiModelDefaults[initial.provider][workload.role] : ""));
  const selected = connections.find((connection) => connection.id === connectionID);
  const models = selected ? aiModelOptions[selected.provider] : [];
  return <TableRow><TableCell><strong>{aiWorkloadName(workload.role, t)}</strong><small className="ai-table-subline">{aiWorkloadDescription(workload.role, t)}</small></TableCell><TableCell><span className={`ai-provider-select ${selected ? "has-provider" : ""}`}>{selected && <AIProviderLogo provider={selected.provider} />}<Select aria-label={t("settings.chooseProvider")} value={connectionID} onChange={(event) => { const id = event.target.value; const connection = connections.find((item) => item.id === id); setConnectionID(id); setModel(connection ? aiModelDefaults[connection.provider][workload.role] : ""); }}><option value="">{t("settings.chooseProvider")}</option>{connections.map((connection) => <option key={connection.id} value={connection.id}>{aiProviderLabel(connection.provider, t)}</option>)}</Select></span></TableCell><TableCell>{selected?.provider === "openai-compatible" ? <Input value={model} onChange={(event) => setModel(event.target.value)} /> : <Select value={model} onChange={(event) => setModel(event.target.value)}><option value="">{t("settings.chooseModel")}</option>{models.map((id) => <option value={id} key={id}>{id}</option>)}</Select>}</TableCell><TableCell><div className="ai-table-actions"><Button outline disabled={!profile} onClick={() => onConfigure(workload.role)}>{t("settings.limits")}</Button><Button disabled={saving || !connectionID || !model} onClick={() => void onSave(workload.role, connectionID, model)}>{t("common.save")}</Button></div></TableCell></TableRow>;
}

export function AIProviderLogo({ provider }: { provider: APIAIProviderConnection["provider"] }) {
  return <span className={`ai-provider-logo ${provider}`} aria-hidden="true">{provider === "openai" ? <OpenAIBlossom /> : provider === "google" ? <GoogleLogo /> : provider === "anthropic" ? <AnthropicLogo /> : provider === "digitalocean" ? <DigitalOceanLogo /> : provider === "xai" ? <XAILogo /> : provider === "deepseek" ? <DeepSeekLogo /> : <Server />}</span>;
}

function OpenAIBlossom() {
  return <svg viewBox="146.694 227.042 267.198 264.812" fill="none" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="M249.176 323.434V298.276C249.176 296.158 249.971 294.569 251.825 293.509L302.406 264.381C309.29 260.409 317.5 258.555 325.973 258.555C357.75 258.555 377.877 283.185 377.877 309.399C377.877 311.253 377.877 313.371 377.611 315.49L325.178 284.771C322.001 282.919 318.822 282.919 315.645 284.771L249.176 323.434ZM367.283 421.415V361.301C367.283 357.592 365.694 354.945 362.516 353.092L296.048 314.43L317.763 301.982C319.617 300.925 321.206 300.925 323.058 301.982L373.639 331.112C388.205 339.586 398.003 357.592 398.003 375.069C398.003 395.195 386.087 413.733 367.283 421.412V421.415ZM233.553 368.452L211.838 355.742C209.986 354.684 209.19 353.095 209.19 350.975V292.718C209.19 264.383 230.905 242.932 260.301 242.932C271.423 242.932 281.748 246.641 290.49 253.26L238.321 283.449C235.146 285.303 233.555 287.951 233.555 291.659V368.455L233.553 368.452ZM280.292 395.462L249.176 377.985V340.913L280.292 323.436L311.407 340.913V377.985L280.292 395.462ZM300.286 475.968C289.163 475.968 278.837 472.259 270.097 465.64L322.264 435.449C325.441 433.597 327.03 430.949 327.03 427.239V350.445L349.011 363.155C350.865 364.213 351.66 365.802 351.66 367.922V426.179C351.66 454.514 329.679 475.965 300.286 475.965V475.968ZM237.525 416.915L186.944 387.785C172.378 379.31 162.582 361.305 162.582 343.827C162.582 323.436 174.763 305.164 193.563 297.485V357.861C193.563 361.571 195.154 364.217 198.33 366.071L264.535 404.467L242.82 416.915C240.967 417.972 239.377 417.972 237.525 416.915ZM234.614 460.343C204.689 460.343 182.71 437.833 182.71 410.028C182.71 407.91 182.976 405.792 183.238 403.672L235.405 433.863C238.582 435.715 241.763 435.715 244.938 433.863L311.407 395.466V420.622C311.407 422.742 310.612 424.331 308.758 425.389L258.179 454.519C251.293 458.491 243.083 460.343 234.611 460.343H234.614ZM300.286 491.854C332.329 491.854 359.073 469.082 365.167 438.892C394.825 431.211 413.892 403.406 413.892 375.073C413.892 356.535 405.948 338.529 391.648 325.552C392.972 319.991 393.766 314.43 393.766 308.87C393.766 271.003 363.048 242.666 327.562 242.666C320.413 242.666 313.528 243.723 306.644 246.109C294.725 234.457 278.307 227.042 260.301 227.042C228.258 227.042 201.513 249.815 195.42 280.004C165.761 287.685 146.694 315.49 146.694 343.824C146.694 362.362 154.638 380.368 168.938 393.344C167.613 398.906 166.819 404.467 166.819 410.027C166.819 447.894 197.538 476.231 233.024 476.231C240.172 476.231 247.058 475.173 253.943 472.788C265.859 484.441 282.278 491.854 300.286 491.854Z" /></svg>;
}

function GoogleLogo() {
  return <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="M12.48 10.92v3.28h7.84c-.24 1.84-.853 3.187-1.787 4.133-1.147 1.147-2.933 2.4-6.053 2.4-4.827 0-8.6-3.893-8.6-8.72s3.773-8.72 8.6-8.72c2.6 0 4.507 1.027 5.907 2.347l2.307-2.307C18.747 1.44 16.133 0 12.48 0 5.867 0 .307 5.387.307 12s5.56 12 12.173 12c3.573 0 6.267-1.173 8.373-3.36 2.16-2.16 2.84-5.213 2.84-7.667 0-.76-.053-1.467-.173-2.053H12.48Z" /></svg>;
}

function AnthropicLogo() {
  return <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="M17.3041 3.541h-3.6718l6.696 16.918H24Zm-10.6082 0L0 20.459h3.7442l1.3693-3.5527h7.0052l1.3693 3.5528h3.7442L10.5363 3.5409Zm-.3712 10.2232 2.2914-5.9456 2.2914 5.9456Z" /></svg>;
}

function DigitalOceanLogo() {
  return <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="M12.04 0C5.408-.02.005 5.37.005 11.992h4.638c0-4.923 4.882-8.731 10.064-6.855a6.95 6.95 0 0 1 4.147 4.148c1.889 5.177-1.924 10.055-6.84 10.064v-4.61H7.391v4.623h4.61V24c7.86 0 13.967-7.588 11.397-15.83-1.115-3.59-3.985-6.446-7.575-7.575A12.8 12.8 0 0 0 12.039 0ZM7.39 19.362H3.828v3.564H7.39Zm-3.563 0v-2.978H.85v2.978Z" /></svg>;
}

function XAILogo() {
  return <svg viewBox="0 0 256 291" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="m.073 102.553 128.541 187.58h57.137L57.195 102.553Zm57.078 104.183L0 290.133h57.18l28.553-41.69ZM198.82 0l-98.788 144.154 28.582 41.721L256 0Zm10.347 89.2v200.933H256V20.861Z" /></svg>;
}

function DeepSeekLogo() {
  return <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path fill="currentColor" d="M23.748 4.651c-.254-.124-.364.113-.512.233-.051.04-.094.09-.137.137-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.155-.708-.311-.955-.65-.172-.24-.219-.509-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.094.172.187.129.323-.082.28-.18.553-.266.833-.055.179-.137.218-.328.14a5.5 5.5 0 0 1-1.737-1.179c-.857-.828-1.631-1.743-2.597-2.46a12 12 0 0 0-.689-.47c-.985-.957.13-1.743.387-1.836.27-.098.094-.433-.778-.428-.872.003-1.67.295-2.687.685a3 3 0 0 1-.465.136 9.6 9.6 0 0 0-2.883-.101c-1.885.21-3.39 1.1-4.497 2.622C.082 8.776-.231 10.854.152 13.02c.403 2.284 1.568 4.175 3.36 5.653 1.857 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.132-.284 4.994-1.86.47.234.962.328 1.78.398.629.058 1.235-.031 1.705-.129.735-.155.684-.836.418-.961-2.155-1.004-1.682-.595-2.112-.926 1.095-1.295 2.768-3.598 3.284-6.733.05-.346.115-.834.108-1.114-.004-.171.035-.238.23-.257a4.2 4.2 0 0 0 1.545-.475c1.397-.763 1.96-2.016 2.093-3.517.02-.23-.004-.467-.247-.588M11.58 18.168c-2.088-1.642-3.101-2.183-3.52-2.16-.39.024-.32.472-.234.763.09.288.207.487.371.74.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.168-1.361-.801-2.5-1.86-3.301-3.306-.775-1.393-1.225-2.888-1.299-4.482-.02-.385.094-.522.477-.592a4.7 4.7 0 0 1 1.53-.038c2.131.311 3.946 1.264 5.467 2.774.868.86 1.525 1.887 2.202 2.89.72 1.066 1.494 2.082 2.48 2.915.348.291.626.513.892.677-.802.09-2.14.109-3.055-.615Zm1.001-6.44a.306.306 0 0 1 .415-.287.3.3 0 0 1 .113.074.3.3 0 0 1 .086.214c0 .17-.136.307-.308.307a.303.303 0 0 1-.306-.307m3.11 1.596c-.2.081-.4.151-.591.16a1.25 1.25 0 0 1-.798-.254c-.274-.23-.47-.358-.551-.758a1.7 1.7 0 0 1 .015-.588c.07-.327-.007-.537-.238-.727-.188-.156-.426-.199-.689-.199a.6.6 0 0 1-.254-.078.253.253 0 0 1-.114-.358 1 1 0 0 1 .192-.21c.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.392.451.462.576.685.915.176.264.336.536.446.848.066.194-.02.353-.25.45" /></svg>;
}
