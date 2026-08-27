"use client";


import { useTranslation } from "react-i18next";
import { ArrowLeft, Check, Eye, RefreshCw, Search, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type { ConsoleFixtures } from "../dev/console-fixtures";
import {
  APIError, api, type APICustomerAccount, type APIDeployment, type APIGrantDefinition,
  type APIIdentity, type APIIntegration, type APINativePlugin, type APIProduct, type APIRecipe,
  type APIResourceSet, type APITool, type APIToolBuilderProposal,
  type APIToolTestAnalysisProposal, type APIUser,
} from "../lib/api";
import { sectionPath, toolBuilderPath } from "../lib/console-routes";
import { Button } from "./core/control";
import { ViewStack } from "./core/layout";
import { IntegrationToolBuilderRoute } from "./integrations/IntegrationToolBuilderRoute";
import { OIDCIdentitySetup } from "./OIDCIdentitySetup";
import { ToolBuilderView } from "./ToolBuilderView";
import { DistributionView, SourcesView } from "./console/agent-access-views";
import {
  AIProviderLogo,
  AISettingsView,
  MCPConnectionsView,
  MCPPreviewView,
  OutboxView,
  RecipesView,
  RootAccessSettingsView,
  SettingsView,
  StorageSettingsView,
  ToolsView,
} from "./console/catalog-settings-views";
import { APIContractsView } from "./console/developer-assets/api-contracts-view";
import { DocumentationCollectionsView } from "./console/developer-assets/documentation-collections-view";
import { DocumentationExplorerView } from "./console/developer-assets/documentation-explorer-view";
import { DocumentationNavigation } from "./console/developer-assets/developer-asset-navigation";
import { QueryLabView } from "./console/developer-assets/query-lab-view";
import { SDKCatalogView } from "./console/developer-assets/sdk-catalog-view";
import { AdminActivityDialogs } from "./console/dialogs/admin-activity-dialogs";
import { AIConfigurationDialogs } from "./console/dialogs/ai-configuration-dialogs";
import { MCPDialogs } from "./console/dialogs/mcp-dialogs";
import { PublicationDialogs } from "./console/dialogs/publication-dialogs";
import { RecipeDialogs, type RecipeDialogState } from "./console/dialogs/recipe-dialogs";
import { parseRecipeSpecEditor, recipeEditableSpec, recipeSpecEditorValue } from "./console/dialogs/recipe-spec-editor";
import { SourceDialogs } from "./console/dialogs/source-dialogs";
import { IntegrationsView } from "./console/integration-views";
import { ConsoleLink, type Source, buildAgentSetupEmbedHTML } from "./console/shared";
import { ConsoleNotFoundView, EntityDetailView, ResourceSetDetailView, ToolDetailView } from "./console/tool-views";
import { useAdminActivityWorkspace } from "./console/use-admin-activity-workspace";
import { useAIWorkspaceState } from "./console/use-ai-workspace";
import { useConsoleNavigation } from "./console/use-console-navigation";
import { useEntityDetail } from "./console/use-entity-detail";
import { useMCPWorkspaceState } from "./console/use-mcp-workspace";
import { usePublicationWorkflow } from "./console/use-publication-workflow";
import { useSourceWorkflow } from "./console/use-source-workflow";
import { ConsoleSidebar, ConsoleTopbar, navigation } from "./console/workspace-navigation";

function deploymentAsProduct(value: APIDeployment): APIProduct {
  return {
    id: value.id,
    organisation_id: value.organisation_id,
    name: value.name,
    slug: value.slug,
    description: value.description,
    catalog_revision: value.catalog_revision,
    public_mcp_enabled: value.public_mcp_enabled,
    revision: value.revision,
  };
}

type ConsoleAppProps = {
  mode: "live" | "fixtures";
  fixtures?: ConsoleFixtures | null;
  currentUser?: APIUser | null;
  currentDeployment?: APIDeployment | null;
  onLogout?: () => void | Promise<void>;
};

export function ConsoleApp({ mode, fixtures, currentUser, currentDeployment, onLogout }: ConsoleAppProps) {
  const { t } = useTranslation();
  if (mode === "fixtures") {
    if (!fixtures) return <section className="panel entity-missing" role="status"><div><h1>{t("console.loadingFixturePreview")}</h1></div></section>;
    return <ConsoleWorkspace fixturePreview fixtures={fixtures} currentUser={currentUser} currentDeployment={fixtures.deployment} onLogout={onLogout} />;
  }
  if (!currentDeployment) return <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>{t("console.deploymentUnavailable")}</h1><p>{t("console.reloadTheConsoleOrCheckTheServiceAPI")}</p></div></section>;
  return <ConsoleWorkspace fixturePreview={false} currentUser={currentUser} currentDeployment={currentDeployment} onLogout={onLogout} />;
}

function ConsoleWorkspace({ fixturePreview, fixtures, currentUser, currentDeployment, onLogout }: { fixturePreview: boolean; fixtures?: ConsoleFixtures; currentUser?: APIUser | null; currentDeployment: APIDeployment; onLogout?: () => void | Promise<void> }) {
  const { t } = useTranslation();
  const [product, setProduct] = useState<APIProduct>(deploymentAsProduct(currentDeployment));
  const [workspaceLoading, setWorkspaceLoading] = useState(!fixturePreview);
  const [workspaceLoadProblems, setWorkspaceLoadProblems] = useState<string[]>([]);
  const [integrations, setIntegrations] = useState<APIIntegration[]>([]);
  const [resourceSets, setResourceSets] = useState<APIResourceSet[]>([]);
  const [sources, setSources] = useState<Source[]>(fixtures?.sources ?? []);
  const [tools, setTools] = useState<APITool[]>(fixtures?.tools ?? []);
  const [nativePlugins, setNativePlugins] = useState<APINativePlugin[]>([]);
  const [grantDefinitions, setGrantDefinitions] = useState<APIGrantDefinition[]>([]);
  const [grantDefinitionsStatus, setGrantDefinitionsStatus] = useState<"loading" | "ready" | "unavailable">(fixturePreview ? "ready" : "loading");
  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
  const [identityLoading, setIdentityLoading] = useState(!fixturePreview);
  const [identityLoadError, setIdentityLoadError] = useState("");
  const [customerAccounts, setCustomerAccounts] = useState<APICustomerAccount[]>(fixtures?.customerAccounts ?? []);
  const [customerAccountsStatus, setCustomerAccountsStatus] = useState<"loading" | "ready" | "unavailable">(fixturePreview ? "ready" : "loading");
  const [customerAccountsHaveMore, setCustomerAccountsHaveMore] = useState(false);
  const [resourceFilter, setResourceFilter] = useState<"all" | "public" | "private">("all");
  const [toolBuilderSelection, setToolBuilderSelection] = useState<{ uid: string; tool: APITool | null; failed: boolean } | null>(null);
  const [toolBuilderLoadAttempt, setToolBuilderLoadAttempt] = useState(0);
  const [toolBuilderSeed, setToolBuilderSeed] = useState<{ toolID: string; revision: number; proposal: APIToolBuilderProposal } | null>(null);
  const [recipeDialog, setRecipeDialog] = useState<RecipeDialogState | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  const showToast = useCallback((message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(null), 2200);
  }, []);
  const recordWorkspaceLoadProblem = useCallback((area: string, error?: unknown) => {
    const detail = error instanceof APIError ? error.message : error instanceof Error ? error.message : t("console.requestFailed");
    setWorkspaceLoadProblems((current) => current.includes(`${area}: ${detail}`) ? current : [...current, `${area}: ${detail}`]);
  }, [t]);
  const clearToolBuilderSeed = useCallback(() => setToolBuilderSeed(null), []);
  const { consoleRoute, section, settingsTab, navigateToPath, navigateToSection, navigateToGroup, onToolBuilderDirtyChange } = useConsoleNavigation({ onLeaveToolBuilder: clearToolBuilderSeed });
  const apiConnected = !fixturePreview;

  const mcpWorkspace = useMCPWorkspaceState({ fixtures, product, apiConnected, setTools, showToast });
  const { mcpConnections, setMCPConnections, setMCPConnectionOpen, mcpBusy, mcpName, mcpNamespace, mcpEndpoint, mcpAccessToken, publicMCPEnabled, setPublicMCPEnabled, distribution, setDistribution, inspectMCPConnection } = mcpWorkspace;
  const publicationWorkspace = usePublicationWorkflow({ product, setProduct, apiConnected, sources, setSources, setPublicMCPEnabled, showToast });
  const { setProductRevision, requestVisibility, requestMCPChange } = publicationWorkspace;
  const adminWorkspace = useAdminActivityWorkspace({ currentUser, apiConnected, showToast });
  const { reportSubmissions, setReportSubmissions, rootUsers, setRootUsers, setRootOpen, setRootRecoveryCodes, auditEvents, setAuditEvents, openSupportSubmission, revokeRootUser } = adminWorkspace;
  const aiWorkspace = useAIWorkspaceState({ product, fixturePreview, onLoadProblem: recordWorkspaceLoadProblem, showToast });
  const { aiConnections, aiProfiles, aiPrompts, analyses, recipes, aiProviderUsage, recipeBusy, workloadBusy, setProviderPickerOpen, openAIConnection, openAIWorkload, openAIPrompt, testAIConnection, saveAIWorkloadSelection, createRecipe, generateRecipesFromEvidence, generateIntegrationSetupGuide, reworkRecipe, editRecipe, deleteRecipe, approveRecipe, publishRecipe, runSystemDoctor } = aiWorkspace;
  const sourceWorkspace = useSourceWorkflow({ product, apiConnected, sources, setSources, refreshCatalog, showToast });
  const { setAddSourceOpen, crawlSource, attachReviewedSourcePublication, publishSource } = sourceWorkspace;
  const toolBuilderUID = consoleRoute.kind === "tool-builder" ? consoleRoute.uid : undefined;

  useEffect(() => {
    if (fixturePreview) document.documentElement.dataset.preview = "fixtures";
    return () => { delete document.documentElement.dataset.preview; };
  }, [fixturePreview]);

  useEffect(() => {
    const needsGrants = consoleRoute.kind === "tool-builder" || (consoleRoute.kind === "section" && consoleRoute.section === "mcp-preview");
    if (fixturePreview || !needsGrants) return;
    let cancelled = false;
    api.grantDefinitions().then((values) => {
      if (!cancelled) {
        setGrantDefinitions(values);
        setGrantDefinitionsStatus("ready");
      }
    }).catch((error: unknown) => {
      if (!cancelled) {
        setGrantDefinitionsStatus("unavailable");
        recordWorkspaceLoadProblem(t("console.grantRegistry"), error);
      }
    });
    return () => { cancelled = true; };
  }, [fixturePreview, consoleRoute.kind, consoleRoute.path, consoleRoute.section, recordWorkspaceLoadProblem, t]);

  useEffect(() => {
    if (fixturePreview || !toolBuilderUID) return;
    let cancelled = false;
    api.tool(product.id, toolBuilderUID).then((value) => {
      if (!cancelled) setToolBuilderSelection({ uid: toolBuilderUID, tool: value, failed: false });
    }).catch(() => {
      if (!cancelled) setToolBuilderSelection({ uid: toolBuilderUID, tool: null, failed: true });
    });
    return () => { cancelled = true; };
  }, [fixturePreview, product.id, toolBuilderUID, toolBuilderLoadAttempt]);

  useEffect(() => {
    if (fixturePreview) return;
    let cancelled = false;
    const load = async () => {
      const settled = await Promise.allSettled([
        api.distribution(product.id), api.sources(product.id), api.tools(product.id), api.mcpConnections(product.id),
        api.nativePlugins(), api.integrations(), api.resourceSets(), api.identity(), api.supportSubmissions(),
        api.rootUsers(), api.auditEvents(product.organisation_id), api.customerAccounts(product.id),
      ]);
      if (cancelled) return;
      const [distributionResult, sourcesResult, toolsResult, mcpResult, nativeResult, integrationsResult, resourcesResult, identityResult, reportsResult, rootsResult, auditResult, accountsResult] = settled;
      if (distributionResult.status === "fulfilled") {
        setDistribution(distributionResult.value);
        setProduct(distributionResult.value.product);
        setPublicMCPEnabled(distributionResult.value.product.public_mcp_enabled);
        setProductRevision(distributionResult.value.product.revision);
      } else recordWorkspaceLoadProblem(t("console.distribution"), distributionResult.reason);
      if (sourcesResult.status === "fulfilled") {
        const remoteSources = sourcesResult.value;
        const [crawlHistories, publicationHistories] = await Promise.all([
          Promise.all(remoteSources.map((source) => api.crawlJobs(product.id, source.id).catch(() => []))),
          Promise.all(remoteSources.map((source) => api.sourcePublications(product.id, source.id).catch(() => []))),
        ]);
        if (!cancelled) setSources((current) => remoteSources.map((source, index) => {
          const local = current.find((item) => item.id === source.id);
          const latest = crawlHistories[index]?.[0];
          const latestPublication = publicationHistories[index]?.[0];
          const crawlState: Source["crawlState"] = latest ? latest.state === "failed" ? "failed" : latest.state === "cancelled" ? "cancelled" : latest.state === "review" || latest.state === "succeeded" ? "review" : latest.state === "running" ? "running" : "queued" : source.published ? "synced" : local?.crawlState ?? "draft";
          return { id: source.id, name: source.name, kind: source.kind, location: source.location, visibility: source.visibility, published: source.published, quarantined: source.quarantined, crawlState: latest && latestPublication?.crawl_job_id === latest.id && crawlState === "review" ? "synced" : crawlState, pages: latest?.fetched_count ?? local?.pages ?? 0, lastCrawl: latest ? latest.finished_at ? t("format.dateTime", { value: new Date(latest.finished_at) }) : latest.state : local?.lastCrawl ?? "not-crawled", revision: source.revision, latestPublication };
        }));
      } else recordWorkspaceLoadProblem(t("console.documentation"), sourcesResult.reason);
      if (toolsResult.status === "fulfilled") setTools(toolsResult.value); else recordWorkspaceLoadProblem(t("navigation.tools"), toolsResult.reason);
      if (mcpResult.status === "fulfilled") setMCPConnections(mcpResult.value); else recordWorkspaceLoadProblem(t("console.mcpConnections"), mcpResult.reason);
      if (nativeResult.status === "fulfilled") setNativePlugins(nativeResult.value); else recordWorkspaceLoadProblem(t("console.nativeTools"), nativeResult.reason);
      if (integrationsResult.status === "fulfilled") setIntegrations(integrationsResult.value); else recordWorkspaceLoadProblem(t("navigation.apis"), integrationsResult.reason);
      if (resourcesResult.status === "fulfilled") setResourceSets(resourcesResult.value); else recordWorkspaceLoadProblem(t("console.resources"), resourcesResult.reason);
      if (identityResult.status === "fulfilled") { setIdentityConfig(identityResult.value); setIdentityLoadError(""); } else setIdentityLoadError(identityResult.reason instanceof APIError ? identityResult.reason.message : t("console.identitySettingsLoadFailed"));
      setIdentityLoading(false);
      if (reportsResult.status === "fulfilled") setReportSubmissions(reportsResult.value); else recordWorkspaceLoadProblem(t("navigation.supportOutbox"), reportsResult.reason);
      if (rootsResult.status === "fulfilled") setRootUsers(rootsResult.value); else recordWorkspaceLoadProblem(t("console.rootUsers"), rootsResult.reason);
      if (auditResult.status === "fulfilled") setAuditEvents(auditResult.value); else recordWorkspaceLoadProblem(t("console.audit"), auditResult.reason);
      if (accountsResult.status === "fulfilled") { setCustomerAccounts(accountsResult.value.items); setCustomerAccountsHaveMore(accountsResult.value.has_more); setCustomerAccountsStatus("ready"); } else setCustomerAccountsStatus("unavailable");
      setWorkspaceLoading(false);
    };
    void load();
    return () => { cancelled = true; };
  }, [fixturePreview, product.id, product.organisation_id, recordWorkspaceLoadProblem, setAuditEvents, setDistribution, setMCPConnections, setProductRevision, setPublicMCPEnabled, setReportSubmissions, setRootUsers, t]);

  async function refreshCatalog() {
    const [integrationValues, setValues, toolValues, eventValues] = await Promise.all([api.integrations(), api.resourceSets(), api.tools(product.id), api.auditEvents(product.organisation_id).catch(() => null)]);
    setIntegrations(integrationValues);
    setResourceSets(setValues);
    setTools(toolValues);
    if (eventValues) setAuditEvents(eventValues);
  }

  async function refreshTools() {
    setTools(await api.tools(product.id));
    const events = await api.auditEvents(product.organisation_id).catch(() => null);
    if (events) setAuditEvents(events);
  }

  async function setNativePluginEnabled(pluginID: string, enabled: boolean) {
    const updated = await api.setNativePluginEnabled(pluginID, enabled);
    setNativePlugins((items) => items.map((item) => item.id === updated.id ? updated : item));
    await refreshTools();
    showToast(t("console.is", { id: String(updated.id), state: String(updated.state) }));
  }

  async function updateCustomerAccountState(account: APICustomerAccount, state: APICustomerAccount["state"]): Promise<boolean> {
    try {
      const updated = await api.updateCustomerAccount(product.id, account.id, state, account.revision);
      setCustomerAccounts((items) => items.map((item) => item.id === updated.id ? updated : item));
      showToast(t("console.is2", { external_id: String(account.external_id), state: String(state) }));
      return true;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("console.customerAccessCouldNotBeChanged"));
      return false;
    }
  }

  async function loadMoreCustomerAccounts(): Promise<boolean> {
    const cursor = customerAccountsHaveMore ? customerAccounts.at(-1)?.id ?? "" : "";
    if (!cursor) return false;
    try {
      const page = await api.customerAccounts(product.id, cursor);
      setCustomerAccounts((items) => [...items, ...page.items.filter((candidate) => !items.some((item) => item.id === candidate.id))]);
      setCustomerAccountsHaveMore(page.has_more);
      return true;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("console.moreCustomerAccountsCouldNotBeLoaded"));
      return false;
    }
  }

  function reviewToolTestProposal(target: APITool, proposal: APIToolTestAnalysisProposal) {
    if ((target.backend_kind ?? "http") !== "http" || target.state !== "draft") {
      showToast(t("console.createAnIndependentHTTPDraftBeforeReviewingThisProposal"));
      return;
    }
    if (target.id === proposal.base_tool_id && target.revision !== proposal.base_revision) {
      showToast(t("console.theToolRevisionChangedAfterAnalysisRunANew"));
      return;
    }
    setToolBuilderSelection({ uid: target.id, tool: target, failed: false });
    setToolBuilderSeed({ toolID: target.id, revision: target.revision, proposal: { proposal_id: proposal.proposal_id, summary: t("console.liveTestProposalReviewSummary"), valid: proposal.valid, draft: { ...proposal.draft, namespace: target.namespace, name: target.name, endpoint: target.endpoint ?? "", upstream_auth: target.upstream_auth ?? proposal.draft.upstream_auth, request_example: target.request_example, response_example: target.response_example, credential_present: Boolean(target.credential_present) }, changes: proposal.changes, findings: proposal.findings } });
    navigateToPath(toolBuilderPath(target.id));
  }

  function beginRecipeCreation() {
    setRecipeDialog({ kind: "create", value: "" });
  }

  function beginRecipeEdit(recipe: APIRecipe) {
    setRecipeDialog({ kind: "edit", recipe, value: recipeSpecEditorValue(recipe), visibility: recipe.visibility });
  }

  function beginRecipeRework(recipe: APIRecipe) {
    setRecipeDialog({ kind: "rework", recipe, value: "" });
  }

  async function submitRecipeDialog() {
    if (!recipeDialog || recipeBusy) return;
    const value = recipeDialog.value.trim();
    if (!value) return;
    let saved: APIRecipe | null;
    if (recipeDialog.kind === "create") {
      saved = await createRecipe(value);
    } else if (recipeDialog.kind === "edit") {
      const parsed = parseRecipeSpecEditor(value, recipeEditableSpec(recipeDialog.recipe));
      if (!parsed.ok) return;
      saved = await editRecipe(recipeDialog.recipe, parsed.referenceIDs, recipeDialog.visibility);
    } else {
      saved = await reworkRecipe(recipeDialog.recipe, value);
    }
    if (saved) setRecipeDialog(null);
  }

  const allResources = useMemo(() => sources.map((item) => ({ ...item, resourceType: "source" as const, type: item.kind, detail: item.location })), [sources]);
  const visibleResources = resourceFilter === "all" ? allResources : allResources.filter((item) => item.visibility === resourceFilter);
  const publicEndpoint = distribution?.public_mcp_endpoint ?? "/mcp/public";
  const publicAgentSetupURL = distribution?.agent_setup.public.url ?? "/agent-setup/public/prompt.md";
  const privateAgentSetupURL = distribution?.agent_setup.private.url ?? "/agent-setup/private/prompt.md";
  const publicAgentSetup = distribution?.agent_setup.public ?? { available: publicMCPEnabled, unavailable_reason: "public_mcp_disabled" as const, url: publicAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(publicAgentSetupURL, "public", { deploymentName: product.name, kindLabel: t("common.public"), connectLabel: t("agentAccess.connectYourAgentToName", { name: product.name }), ariaLabel: t("agentAccess.connectYourAgentUsingMCP", { name: product.name, kind: t("common.public") }) }), contains_secret: false as const };
  const privateAgentSetup = distribution?.agent_setup.private ?? { available: identityConfig?.configured === true && identityConfig.state === "active", unavailable_reason: "identity_unavailable" as const, url: privateAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(privateAgentSetupURL, "private", { deploymentName: product.name, kindLabel: t("common.private"), connectLabel: t("agentAccess.connectYourAgentToName", { name: product.name }), ariaLabel: t("agentAccess.connectYourAgentUsingMCP", { name: product.name, kind: t("common.private") }) }), contains_secret: false as const };
  const mcpConnectionReady = Boolean(mcpName.trim() && mcpNamespace.trim() && mcpEndpoint.trim() && mcpAccessToken.trim());
  const activeNavigation = navigation.find((item) => item.sections.some((candidate) => candidate.id === section));
  const selectedToolBuilderTool = toolBuilderUID && toolBuilderSelection?.uid === toolBuilderUID ? toolBuilderSelection.tool : null;
  const toolBuilderLoadFailed = Boolean(toolBuilderUID && toolBuilderSelection?.uid === toolBuilderUID && toolBuilderSelection.failed);
  const activeToolBuilderSeed = selectedToolBuilderTool && toolBuilderSeed?.toolID === selectedToolBuilderTool.id && toolBuilderSeed.revision === selectedToolBuilderTool.revision ? toolBuilderSeed.proposal : null;
  const toolBuilderIntegrationID = consoleRoute.kind === "tool-builder" ? consoleRoute.integrationID ?? selectedToolBuilderTool?.owner_integration_id : undefined;
  const toolBuilderIntegration = toolBuilderIntegrationID ? integrations.find((integration) => integration.id === toolBuilderIntegrationID) : undefined;
  const toolBuilderContent = consoleRoute.kind !== "tool-builder" ? null
    : consoleRoute.uid && !selectedToolBuilderTool && !toolBuilderLoadFailed ? <section className="panel entity-missing"><RefreshCw /><div><h1>{t("console.loadingHTTPToolDraft")}</h1></div></section>
    : consoleRoute.uid && toolBuilderLoadFailed ? <section className="panel entity-missing" role="alert"><TriangleAlert /><div><h1>{t("console.httpToolDraftUnavailable")}</h1></div><Button outline onClick={() => { setToolBuilderSelection(null); setToolBuilderLoadAttempt((value) => value + 1); }}>{t("common.retry")}</Button></section>
    : consoleRoute.uid && (!selectedToolBuilderTool || (selectedToolBuilderTool.backend_kind ?? "http") !== "http") ? <section className="panel entity-missing"><Search /><div><h1>{t("console.httpToolDraftUnavailable")}</h1></div><ConsoleLink path={sectionPath("tools")} onNavigate={navigateToPath} className="entity-back-link"><ArrowLeft />{t("console.returnToTools")}</ConsoleLink></section>
    : toolBuilderIntegrationID && !toolBuilderIntegration ? <section className="panel entity-missing"><TriangleAlert /><div><h1>{t("console.owningAPIUnavailable")}</h1></div></section>
    : toolBuilderIntegration ? <IntegrationToolBuilderRoute key={`${consoleRoute.path}:${selectedToolBuilderTool?.revision ?? 0}`} integration={toolBuilderIntegration} product={product} grants={grantDefinitions} tool={selectedToolBuilderTool} initialProposal={activeToolBuilderSeed} aiAvailable={aiProfiles.some((profile) => profile.workload === "analysis" && profile.enabled)} onSaved={async (saved) => { setTools((items) => [...items.filter((item) => item.id !== saved.id), saved]); await refreshTools().catch(() => {}); }} onDirtyChange={onToolBuilderDirtyChange} onMessage={showToast} onNavigate={navigateToPath} />
    : <ToolBuilderView key={`${consoleRoute.path}:${selectedToolBuilderTool?.revision ?? 0}`} product={product} grants={grantDefinitions} tool={selectedToolBuilderTool} initialProposal={activeToolBuilderSeed} aiAvailable={aiProfiles.some((profile) => profile.workload === "analysis" && profile.enabled)} onSaved={async (saved) => { setTools((items) => [...items.filter((item) => item.id !== saved.id), saved]); await refreshTools().catch(() => {}); }} onDirtyChange={onToolBuilderDirtyChange} onMessage={showToast} onNavigate={navigateToPath} />;
  const entityDetail = useEntityDetail({ consoleRoute, integrations, resourceSets, sources, tools, mcpConnections, reportSubmissions, auditEvents, rootUsers });
  const workspaceClass = consoleRoute.kind === "tool-builder" ? "workspace-wide" : section === "settings" ? "workspace-compact" : "workspace-default";
  const integrationViewProps = { live: apiConnected, integrations, analyses, tools, resourceSets, sources, identity: identityConfig, distribution, onAddSource: () => setAddSourceOpen(true), onCrawlSource: crawlSource, onPublishSource: publishSource, onAttachPublishedSource: attachReviewedSourcePublication, onGenerateSetupGuide: generateIntegrationSetupGuide, onChanged: refreshCatalog, onMessage: showToast, onNavigate: navigateToPath };

  return <div className="app-shell">
    <a className="skip-link" href="#main-content">{t("console.skipToContent")}</a>
    <ConsoleSidebar section={section} activeNavigationID={activeNavigation?.id} currentUser={currentUser} onLogout={onLogout} onNavigate={navigateToPath} />
    <main id="main-content" className={workspaceClass} tabIndex={-1}>
      <ConsoleTopbar productName={product.name} section={section} activeNavigationID={activeNavigation?.id} onGroupChange={navigateToGroup} />
      <div className="content">
        {fixturePreview && <div className="workspace-notice preview"><Eye /><span><strong>{t("console.fixturePreview")}</strong><small>{t("console.developmentOnlySampleData")}</small></span></div>}
        {workspaceLoading && <div className="workspace-notice loading"><RefreshCw className="spin" /><span><strong>{t("console.loadingDeploymentData")}</strong></span></div>}
        {workspaceLoadProblems.length > 0 && <div className="workspace-notice error"><TriangleAlert /><span><strong>{t("console.someDataCouldNotBeLoaded")}</strong><small>{workspaceLoadProblems.join(" · ")}</small></span><Button outline onClick={() => window.location.reload()}>{t("common.reload")}</Button></div>}
        <ViewStack>
          {consoleRoute.kind === "not-found" ? <ConsoleNotFoundView path={consoleRoute.path} onNavigate={navigateToPath} />
            : consoleRoute.kind === "tool-builder" ? toolBuilderContent
            : consoleRoute.kind === "entity" && consoleRoute.entity === "integration" ? <IntegrationsView {...integrationViewProps} selectedIntegrationID={consoleRoute.uid} activeTab={consoleRoute.integrationTab} activeResourceTab={consoleRoute.integrationResourceTab ?? "documentation"} />
            : consoleRoute.kind === "entity" && consoleRoute.entity === "resource-set" ? <ResourceSetDetailView resource={resourceSets.find((item) => item.id === consoleRoute.uid) ?? null} integrations={integrations} onNavigate={navigateToPath} />
            : consoleRoute.kind === "entity" && consoleRoute.entity === "tool" ? <ToolDetailView key={`${consoleRoute.uid}:${tools.find((item) => item.id === consoleRoute.uid)?.revision ?? 0}`} productID={product.id} tool={tools.find((item) => item.id === consoleRoute.uid) ?? null} connections={mcpConnections} integrations={integrations} auditEvents={auditEvents} onChanged={refreshTools} onReviewProposal={reviewToolTestProposal} onMessage={showToast} onNavigate={navigateToPath} />
            : consoleRoute.kind === "entity" ? <EntityDetailView route={consoleRoute} detail={entityDetail} onNavigate={navigateToPath} />
            : <>
              {section === "product" && <IntegrationsView {...integrationViewProps} />}
              {section === "documents" && <DocumentationExplorerView live={apiConnected} sources={sources} onNavigate={navigateToPath} />}
              {section === "collections" && <DocumentationCollectionsView live={apiConnected} integrations={integrations} onMessage={showToast} onNavigate={navigateToPath} />}
              {section === "contracts" && <APIContractsView live={apiConnected} integrations={integrations} sources={sources} onMessage={showToast} onNavigate={navigateToPath} />}
              {section === "sdks" && <SDKCatalogView live={apiConnected} integrations={integrations} onMessage={showToast} onNavigate={navigateToPath} />}
              {section === "query-lab" && <QueryLabView live={apiConnected} integrations={integrations} onMessage={showToast} onNavigate={navigateToPath} />}
              {section === "identity" && <OIDCIdentitySetup key={identityLoading ? "loading" : identityConfig?.id || "identity"} identity={identityConfig} loading={identityLoading} loadError={identityLoadError} onChanged={setIdentityConfig} onMessage={showToast} />}
              {section === "recipes" && <RecipesView integrations={integrations} analyses={analyses} recipes={recipes} busy={recipeBusy} onCreate={beginRecipeCreation} onGenerate={generateRecipesFromEvidence} onEdit={beginRecipeEdit} onRework={beginRecipeRework} onDelete={deleteRecipe} onApprove={approveRecipe} onPublish={publishRecipe} />}
              {section === "sources" && <SourcesView sources={sources} navigation={<DocumentationNavigation active="sources" onNavigate={navigateToPath} />} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} onNavigate={navigateToPath} />}
              {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} onNavigate={navigateToPath} />}
              {section === "tools" && <ToolsView tools={tools} integrations={integrations} connections={mcpConnections} nativePlugins={nativePlugins} onSetNativePluginEnabled={setNativePluginEnabled} onNavigate={navigateToPath} />}
              {section === "mcp-preview" && <MCPPreviewView product={product} grants={grantDefinitions} grantStatus={grantDefinitionsStatus} available={apiConnected} privateEndpointEnabled={identityConfig?.configured === true && identityConfig.state === "active"} onMessage={showToast} onNavigate={navigateToPath} />}
              {section === "distribution" && <DistributionView enabled={publicMCPEnabled} onEnabledChange={requestMCPChange} resources={visibleResources} resourceFilter={resourceFilter} setResourceFilter={setResourceFilter} onVisibilityChange={requestVisibility} onCopied={showToast} publicEndpoint={publicEndpoint} tenantName={product.name} publicAgentSetup={publicAgentSetup} privateAgentSetup={privateAgentSetup} onConfigureIdentity={() => navigateToSection("identity")} customerAccounts={customerAccounts} customerAccountsStatus={customerAccountsStatus} customerAccountsHaveMore={customerAccountsHaveMore} onUpdateCustomerAccount={updateCustomerAccountState} onLoadMoreCustomerAccounts={loadMoreCustomerAccounts} onOpenSources={() => navigateToSection("sources")} />}
              {section === "reporting" && <OutboxView submissions={reportSubmissions} events={auditEvents} onView={openSupportSubmission} onNavigate={navigateToPath} />}
              {section === "settings" && settingsTab === "overview" && <SettingsView product={product} aiProfiles={aiProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
              {section === "settings" && settingsTab === "storage" && <StorageSettingsView onNavigate={navigateToPath} />}
              {section === "settings" && settingsTab === "ai" && <AISettingsView profiles={aiProfiles} prompts={aiPrompts} connections={aiConnections} usage={aiProviderUsage} saving={workloadBusy} onSave={saveAIWorkloadSelection} onConfigure={openAIWorkload} onEditPrompt={openAIPrompt} onAddProvider={() => setProviderPickerOpen(true)} onConnect={openAIConnection} onTest={testAIConnection} onNavigate={navigateToPath} />}
              {section === "settings" && settingsTab === "root" && <RootAccessSettingsView rootUsers={rootUsers} currentUser={currentUser ?? null} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
            </>}
        </ViewStack>
      </div>
    </main>
    <PublicationDialogs workspace={publicationWorkspace} />
    <SourceDialogs workspace={sourceWorkspace} />
    <MCPDialogs workspace={mcpWorkspace} connectionReady={mcpConnectionReady} />
    <AdminActivityDialogs workspace={adminWorkspace} />
    <AIConfigurationDialogs workspace={aiWorkspace} ProviderLogo={AIProviderLogo} />
    <RecipeDialogs state={recipeDialog} busy={recipeBusy} onChange={(value) => setRecipeDialog((current) => current ? { ...current, value } : null)} onVisibilityChange={(visibility) => setRecipeDialog((current) => current?.kind === "edit" ? { ...current, visibility } : current)} onClose={() => setRecipeDialog(null)} onSubmit={() => void submitRecipeDialog()} />
    {toast && <div className="toast" role="status"><Check />{toast}</div>}
  </div>;
}
