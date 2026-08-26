"use client";

import {
  ArrowLeft,
  Check,
  Eye,
  RefreshCw,
  Search,
  TriangleAlert,
} from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIBackendConnection, APICustomerAccount, APIDeployment, APIError, APIGrantDefinition, APIIdentity, APIIntegration, APINativePlugin, APIProduct, APIResourceSet, APISupportRoute, APITool, APIToolBuilderProposal, APIToolTestAnalysisProposal, APIUser, APIWidget, api } from "../lib/api";
import { sectionPath, settingsPath, toolBuilderPath } from "../lib/console-routes";
import type { ConsoleFixtures } from "../dev/console-fixtures";
import { Button } from "./core/control";
import { ViewStack } from "./core/layout";
import { OIDCIdentitySetup } from "./OIDCIdentitySetup";
import { ToolBuilderView } from "./ToolBuilderView";
import { WidgetPreviewLauncher } from "./WidgetPreviewLauncher";
import { IntegrationToolBuilderRoute } from "./integrations/IntegrationToolBuilderRoute";
import { ConsoleLink, Source, buildAgentSetupEmbedHTML } from "./console/shared";
import { ConsoleSidebar, ConsoleTopbar, navigation } from "./console/workspace-navigation";
import { useConsoleNavigation } from "./console/use-console-navigation";
import { useEntityDetail } from "./console/use-entity-detail";
import { useAdminActivityWorkspace } from "./console/use-admin-activity-workspace";
import { useAIWorkspaceState } from "./console/use-ai-workspace";
import { useMCPWorkspaceState } from "./console/use-mcp-workspace";
import { useProductReleaseState } from "./console/use-product-release-workspace";
import { usePublicationWorkflow } from "./console/use-publication-workflow";
import { useSourceWorkflow } from "./console/use-source-workflow";
import { useWidgetWorkflow } from "./console/use-widget-workflow";
import { AdminActivityDialogs } from "./console/dialogs/admin-activity-dialogs";
import { AIConfigurationDialogs } from "./console/dialogs/ai-configuration-dialogs";
import { MCPDialogs } from "./console/dialogs/mcp-dialogs";
import { ProductReleaseDialogs } from "./console/dialogs/product-release-dialogs";
import { PublicationDialogs } from "./console/dialogs/publication-dialogs";
import { SourceDialogs } from "./console/dialogs/source-dialogs";
import { WidgetDialogs } from "./console/dialogs/widget-dialogs";

const AccessView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.AccessView })));
const ActivityHubView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.ActivityHubView })));
const AIProviderLogo = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.AIProviderLogo })));
const AISettingsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.AISettingsView })));
const ConnectorReleasesView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.ConnectorReleasesView })));
const MCPConnectionsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.MCPConnectionsView })));
const RecipesView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.RecipesView })));
const ReportingView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.ReportingView })));
const RootAccessSettingsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.RootAccessSettingsView })));
const SettingsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.SettingsView })));
const StorageSettingsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.StorageSettingsView })));
const ToolsView = lazy(() => import("./console/catalog-settings-views").then((module) => ({ default: module.ToolsView })));
const ConsoleNotFoundView = lazy(() => import("./console/tool-views").then((module) => ({ default: module.ConsoleNotFoundView })));
const EntityDetailView = lazy(() => import("./console/tool-views").then((module) => ({ default: module.EntityDetailView })));
const ResourceSetDetailView = lazy(() => import("./console/tool-views").then((module) => ({ default: module.ResourceSetDetailView })));
const ToolDetailView = lazy(() => import("./console/tool-views").then((module) => ({ default: module.ToolDetailView })));
const DistributionView = lazy(() => import("./console/agent-access-views").then((module) => ({ default: module.DistributionView })));
const SourcesView = lazy(() => import("./console/agent-access-views").then((module) => ({ default: module.SourcesView })));
const WidgetDetailView = lazy(() => import("./console/agent-access-views").then((module) => ({ default: module.WidgetDetailView })));
const WidgetsView = lazy(() => import("./console/agent-access-views").then((module) => ({ default: module.WidgetsView })));
const IntegrationsView = lazy(() => import("./console/integration-views").then((module) => ({ default: module.IntegrationsView })));

function deploymentAsLegacyProduct(value: APIDeployment): APIProduct {
  return { id: value.id, organisation_id: value.organisation_id, name: value.name, slug: value.slug, description: value.description, default_version_policy: value.default_release_policy, catalog_revision: value.catalog_revision, require_promotion_approval: value.require_promotion_approval, public_mcp_enabled: value.public_mcp_enabled, revision: value.revision };
}

type ConsoleAppProps = {
  mode: "live" | "fixtures";
  fixtures?: ConsoleFixtures | null;
  currentUser?: APIUser | null;
  currentDeployment?: APIDeployment | null;
  onLogout?: () => void | Promise<void>;
};

export function ConsoleApp({ mode, fixtures, currentUser, currentDeployment, onLogout }: ConsoleAppProps) {
  if (mode === "fixtures") {
    if (!fixtures) return <section className="panel entity-missing" role="status"><div><h1>Loading fixture preview</h1><p>The development-only preview data is loading.</p></div></section>;
    return <ConsoleWorkspace fixturePreview fixtures={fixtures} currentUser={currentUser} currentDeployment={fixtures.deployment} onLogout={onLogout} />;
  }
  if (!currentDeployment) {
    return <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>Deployment unavailable</h1><p>The authenticated deployment was not loaded. Reload the console or check the service API.</p></div></section>;
  }
  return <ConsoleWorkspace fixturePreview={false} currentUser={currentUser} currentDeployment={currentDeployment} onLogout={onLogout} />;
}

function ConsoleWorkspace({ fixturePreview, fixtures, currentUser, currentDeployment, onLogout }: { fixturePreview: boolean; fixtures?: ConsoleFixtures; currentUser?: APIUser | null; currentDeployment: APIDeployment; onLogout?: () => void | Promise<void> }) {
	const widgetsEnabled = Boolean(currentDeployment.features?.widgets);
	const [product, setProduct] = useState<APIProduct>(deploymentAsLegacyProduct(currentDeployment));
	const [workspaceLoading, setWorkspaceLoading] = useState(!fixturePreview);
	const [workspaceLoadProblems, setWorkspaceLoadProblems] = useState<string[]>([]);
	const recordWorkspaceLoadProblem = useCallback((area: string, error?: unknown) => {
	  const detail = error instanceof APIError
	    ? error.message
	    : error instanceof Error
	      ? error.message
	      : "Request failed";
	  setWorkspaceLoadProblems((current) =>
	    current.includes(`${area}: ${detail}`)
	      ? current
	      : [...current, `${area}: ${detail}`],
	  );
	}, []);
	const [integrations, setIntegrations] = useState<APIIntegration[]>([]);
	const [widgets, setWidgets] = useState<APIWidget[]>([]);
	const [resourceSets, setResourceSets] = useState<APIResourceSet[]>([]);
	const [accessDefinitions, setAccessDefinitions] = useState<APIAccessDefinition[]>([]);
	const [accessConnections, setAccessConnections] = useState<APIAccessConnection[]>([]);
	const [backendConnections, setBackendConnections] = useState<APIBackendConnection[]>([]);
	const [accessInstances, setAccessInstances] = useState<APIAccessInstance[]>([]);
	const [accessCredentials, setAccessCredentials] = useState<APIAccessCredential[]>([]);
  const [supportRoutes, setSupportRoutes] = useState<APISupportRoute[]>([]);
  const [sources, setSources] = useState<Source[]>(fixtures?.sources ?? []);
  const [tools, setTools] = useState<APITool[]>(fixtures?.tools ?? []);
  const [nativePlugins, setNativePlugins] = useState<APINativePlugin[]>([]);
  const [grantDefinitions, setGrantDefinitions] = useState<APIGrantDefinition[]>([]);
  const [toolBuilderSelection, setToolBuilderSelection] = useState<{ uid: string; tool: APITool | null; failed: boolean } | null>(null);
  const [toolBuilderLoadAttempt, setToolBuilderLoadAttempt] = useState(0);
  const [toolBuilderSeed, setToolBuilderSeed] = useState<{ toolID: string; revision: number; proposal: APIToolBuilderProposal } | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const showToast = useCallback((message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(null), 2200);
  }, []);
  const clearToolBuilderSeed = useCallback(() => setToolBuilderSeed(null), []);
  const {
    consoleRoute,
    section,
    settingsTab,
    navigateToPath,
    navigateToSection,
    navigateToGroup,
    onToolBuilderDirtyChange: handleToolBuilderDirtyChange,
  } = useConsoleNavigation({ widgetsEnabled, onLeaveToolBuilder: clearToolBuilderSeed });
  const apiConnected = !fixturePreview;
  const mcpWorkspace = useMCPWorkspaceState({ fixtures, product, apiConnected, setTools, showToast });
  const {
    mcpConnections, setMCPConnections,
    setMCPConnectionOpen,
    mcpBusy,
    mcpName,
    mcpNamespace,
    mcpEndpoint,
    mcpAuthMode,
    mcpCredential,
    mcpOAuthClientID,
    mcpOAuthClientSecret,
    mcpOAuthIssuer,
    mcpAuthorizationURL,
    mcpTokenURL,
    publicMCPEnabled, setPublicMCPEnabled,
    distribution, setDistribution,
    inspectMCPConnection,
  } = mcpWorkspace;
  const widgetWorkspace = useWidgetWorkflow({ setWidgets, onNavigate: navigateToPath, showToast });
  const {
    setWidgetCreateOpen,
    widgetBusy,
    updateWidget,
    setWidgetState,
    rotateWidgetSecret,
  } = widgetWorkspace;
  const publicationWorkspace = usePublicationWorkflow({
    product,
    setProduct,
    fixtures,
    apiConnected,
    sources,
    setSources,
    setPublicMCPEnabled,
    refreshCatalog,
    navigateToProduct: () => navigateToSection("product"),
    showToast,
  });
  const {
    productDefinition, setProductDefinition,
    setLatestProductBuild,
    setProductBuilderOpen,
    setProductRevision,
    requestVisibility,
    requestMCPChange,
  } = publicationWorkspace;
  const [resourceFilter, setResourceFilter] = useState<"all" | "public" | "private">("all");
		  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
		  const [identityLoading, setIdentityLoading] = useState(true);
		  const [identityLoadError, setIdentityLoadError] = useState("");
      const adminWorkspace = useAdminActivityWorkspace({ product, fixtures, currentUser, apiConnected, showToast });
      const {
        analytics, setAnalytics,
        reportSubmissions, setReportSubmissions,
        rootUsers, setRootUsers,
        setRootOpen,
        setRootRecoveryCodes,
        environments, setEnvironments,
        integrationRuns, setIntegrationRuns,
        auditEvents, setAuditEvents,
        setRunOpen,
        setRunEnvironmentID,
        createSupportDeliveryAttempt,
        openSupportSubmission,
        revokeRootUser,
        completeIntegrationRun,
      } = adminWorkspace;
      const aiWorkspace = useAIWorkspaceState({
        product,
        fixturePreview,
        onLoadProblem: recordWorkspaceLoadProblem,
        showToast,
      });
      const productReleaseWorkspace = useProductReleaseState({
        product,
        fixtures,
        fixturePreview,
        onLoadProblem: recordWorkspaceLoadProblem,
        productDefinition,
        onProductChanged: (value) => {
          setProduct(value);
          setProductRevision(value.revision);
        },
        showToast,
      });
      const {
        aiConnections,
        aiProfiles,
        analyses,
        recipes,
        aiProviderUsage,
        recipeBusy,
        llmBusy,
        setProviderPickerOpen,
        openAIConnection,
        openLLMProfile,
        testAIConnection,
        saveAIWorkloadSelection,
        createRecipe,
        generateRecipesFromEvidence,
        generateIntegrationAgentGuide,
        reworkRecipe,
        editRecipe,
        approveRecipe,
        publishRecipe,
        runSystemDoctor,
      } = aiWorkspace;
      const {
        productVersions,
        productVersionPins,
        customerAccountLoad, setCustomerAccountLoad,
        customerAccounts,
        customerAccountsStatus,
        customerAccountsHaveMore,
        productInstallations,
        openProductCatalog,
      } = productReleaseWorkspace;
  const toolBuilderUID = consoleRoute.kind === "tool-builder" ? consoleRoute.uid : undefined;

	  useEffect(() => {
    if (fixturePreview) document.documentElement.dataset.preview = "fixtures";
    return () => { delete document.documentElement.dataset.preview; };
  }, [fixturePreview]);

  useEffect(() => {
    if (fixturePreview || consoleRoute.kind !== "tool-builder") return;
    let cancelled = false;
    api.grantDefinitions().then((values) => {
      if (!cancelled) setGrantDefinitions(values);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, [fixturePreview, consoleRoute.kind, consoleRoute.path]);

  useEffect(() => {
    if (fixturePreview || !toolBuilderUID) return;
    let cancelled = false;
    api.tool(product.id, toolBuilderUID).then((value) => {
      if (cancelled) return;
      setToolBuilderSelection({ uid: toolBuilderUID, tool: value, failed: false });
    }).catch(() => {
      if (cancelled) return;
      setToolBuilderSelection({ uid: toolBuilderUID, tool: null, failed: true });
    });
    return () => { cancelled = true; };
  }, [fixturePreview, product.id, toolBuilderUID, toolBuilderLoadAttempt]);

  useEffect(() => {
    if (fixturePreview) return;

    let cancelled = false;
	    const recordLoadProblem = (area: string, error?: unknown) => {
	      if (cancelled) return;
	      const detail = error instanceof APIError ? error.message : error instanceof Error ? error.message : "Request failed";
	      setWorkspaceLoadProblems((current) => current.includes(`${area}: ${detail}`) ? current : [...current, `${area}: ${detail}`]);
	    };
	    Promise.all([api.distribution(product.id), api.sources(product.id), api.tools(product.id), api.mcpConnections(product.id), api.nativePlugins()]).then(async ([distributionValue, remoteSources, remoteTools, remoteMCPConnections, remoteNativePlugins]) => {
	      const [crawlHistories, publicationHistories] = await Promise.all([
	        Promise.all(remoteSources.map((source) => api.crawlJobs(product.id, source.id).catch(() => []))),
	        Promise.all(remoteSources.map((source) => api.sourcePublications(product.id, source.id).catch(() => []))),
	      ]);
      if (cancelled) return;
      setDistribution(distributionValue);
      setProduct(distributionValue.product);
      setPublicMCPEnabled(distributionValue.product.public_mcp_enabled);
      setProductRevision(distributionValue.product.revision);
      setSources((current) => remoteSources.map((source, index) => {
        const local = current.find((item) => item.id === source.id);
	        const latest = crawlHistories[index]?.[0];
	        const latestPublication = publicationHistories[index]?.[0];
	        const generationPublished = Boolean(latest && latestPublication?.crawl_job_id === latest.id);
	        const crawlState: Source["crawlState"] = latest
	          ? latest.state === "failed"
	            ? "failed"
	            : latest.state === "cancelled"
	              ? "cancelled"
	            : latest.state === "review" || latest.state === "succeeded"
	              ? "review"
	              : latest.state === "running"
	                ? "running"
	                : "queued"
	          : source.published
	            ? "synced"
	            : local?.crawlState ?? "draft";
        return {
          id: source.id,
          name: source.name,
          kind: source.kind,
          location: source.location,
          visibility: source.visibility,
          published: source.published,
          quarantined: source.quarantined,
	          crawlState: generationPublished && crawlState === "review" ? "synced" : crawlState,
	          pages: latest?.fetched_count ?? local?.pages ?? 0,
	          lastCrawl: latest ? latest.finished_at ? new Date(latest.finished_at).toLocaleString() : latest.state : local?.lastCrawl ?? "Not crawled",
          revision: source.revision,
          latestPublication,
        };
      }));
      setTools(remoteTools);
      setMCPConnections(remoteMCPConnections);
      setNativePlugins(remoteNativePlugins);
	    }).catch((error) => recordLoadProblem("Catalog", error)).finally(() => { if (!cancelled) setWorkspaceLoading(false); });
		    api.analytics(product.id).then((value) => { if (!cancelled) setAnalytics(value); }).catch((error) => recordLoadProblem("Analytics", error));
		    api.identity().then((value) => {
		      if (cancelled) return;
		      setIdentityConfig(value);
		      setIdentityLoadError("");
		    }).catch((error) => {
		      if (!cancelled) setIdentityLoadError(error instanceof APIError ? error.message : "Identity settings could not be loaded.");
		    }).finally(() => {
		      if (!cancelled) setIdentityLoading(false);
		    });
	    api.supportSubmissions().then((submissions) => {
	      if (!cancelled) setReportSubmissions(submissions);
	    }).catch((error) => recordLoadProblem("Support submissions", error));
	    api.rootUsers().then((value) => { if (!cancelled) setRootUsers(value); }).catch((error) => recordLoadProblem("Root users", error));
	    Promise.all([api.integrations(), widgetsEnabled ? api.widgets() : Promise.resolve([] as APIWidget[]), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.backendConnections(), api.supportRoutes()]).then(async ([integrationValues, widgetValues, setValues, definitionValues, connectionValues, backendValues, routeValues]) => {
	      if (cancelled) return;
	      setIntegrations(integrationValues); setWidgets(widgetValues); setResourceSets(setValues); setAccessDefinitions(definitionValues); setAccessConnections(connectionValues); setBackendConnections(backendValues); setSupportRoutes(routeValues);
	      const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
	      const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
	      if (!cancelled) { setAccessInstances(instanceGroups.flat()); setAccessCredentials(credentialGroups.flat()); }
	    }).catch((error) => recordLoadProblem("Deployment configuration", error));
	    api.productDefinition(product.id).then((value) => { if (!cancelled) setProductDefinition(value); }).catch((error) => { if (!cancelled && error instanceof APIError && error.status === 404) setProductDefinition(null); else recordLoadProblem("Product definition", error); });
	    api.productBuilds(product.id).then((values) => { if (!cancelled) setLatestProductBuild(values[0] ?? null); }).catch((error) => recordLoadProblem("Product builds", error));
	    Promise.all([api.environments(product.id), api.integrationRuns(product.id), api.auditEvents(product.organisation_id)]).then(([environmentValues, runValues, eventValues]) => {
	      if (cancelled) return;
	      setEnvironments(environmentValues);
	      setRunEnvironmentID(environmentValues.find((environment) => environment.is_production)?.id ?? environmentValues[0]?.id ?? "");
	      setIntegrationRuns(runValues);
	      setAuditEvents(eventValues);
	    }).catch((error) => recordLoadProblem("Runtime activity", error));
    return () => { cancelled = true; };
  }, [
    fixturePreview,
    product.id,
    product.organisation_id,
    widgetsEnabled,
    setDistribution,
    setMCPConnections,
    setPublicMCPEnabled,
    setLatestProductBuild,
    setProductDefinition,
    setProductRevision,
    setAnalytics,
    setAuditEvents,
    setEnvironments,
    setIntegrationRuns,
    setReportSubmissions,
    setRootUsers,
    setRunEnvironmentID,
  ]);

  const publicSources = sources.filter((item) => item.visibility === "public");
  const publicResourceCount = publicSources.length;
  const allResources = useMemo(() => [
    ...sources.map((item) => ({ ...item, resourceType: "source" as const, type: item.kind, detail: item.location })),
  ], [sources]);
  const visibleResources = resourceFilter === "all" ? allResources : allResources.filter((item) => item.visibility === resourceFilter);

	async function refreshCatalog() {
		const auditRequest = api.auditEvents(product.organisation_id).catch(() => null);
		const [integrationValues, setValues, definitionValues, connectionValues, backendValues, routeValues, toolValues] = await Promise.all([api.integrations(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.backendConnections(), api.supportRoutes(), api.tools(product.id)]);
		setIntegrations(integrationValues);
		setResourceSets(setValues);
		setAccessDefinitions(definitionValues);
		setAccessConnections(connectionValues);
		setBackendConnections(backendValues);
		setSupportRoutes(routeValues);
		setTools(toolValues);
		const eventValues = await auditRequest;
		if (eventValues) setAuditEvents(eventValues);
		const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
		const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
		setAccessInstances(instanceGroups.flat());
		setAccessCredentials(credentialGroups.flat());
	}

  const sourceWorkspace = useSourceWorkflow({ product, apiConnected, sources, setSources, refreshCatalog, showToast });
  const {
    setAddSourceOpen,
    crawlSource,
    attachReviewedSourcePublication,
    publishSource,
  } = sourceWorkspace;

  async function refreshTools() {
    const toolValues = await api.tools(product.id);
    setTools(toolValues);
    const eventValues = await api.auditEvents(product.organisation_id).catch(() => null);
    if (eventValues) setAuditEvents(eventValues);
  }

  async function setNativePluginEnabled(pluginID: string, enabled: boolean) {
    try {
      const updated = await api.setNativePluginEnabled(pluginID, enabled);
      setNativePlugins((items) => items.map((item) => item.id === updated.id ? updated : item));
      await refreshTools();
      showToast(`${updated.id} is ${updated.state}.`);
    } catch (error) {
      showToast(error instanceof APIError || error instanceof Error ? error.message : "Native plugin state could not be changed.");
      throw error;
    }
  }

  async function refreshToolsAfterBuilderSave(savedTool: APITool) {
    setTools((items) => [...items.filter((item) => item.id !== savedTool.id), savedTool]);
    await refreshTools().catch(() => {});
  }

  async function updateCustomerAccountState(account: APICustomerAccount, state: APICustomerAccount["state"]): Promise<boolean> {
    const productID = product.id;
    try {
      const updated = await api.updateCustomerAccount(productID, account.id, state, account.revision);
      setCustomerAccountLoad((current) => current.productID === productID ? { ...current, status: "ready", items: current.items.map((item) => item.id === updated.id ? updated : item) } : current);
      showToast(state === "suspended" ? `${account.external_id} is suspended.` : `${account.external_id} can sign in again.`);
      return true;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Customer access could not be changed.");
      return false;
    }
  }

  async function loadMoreCustomerAccounts(): Promise<boolean> {
    const productID = product.id;
    const cursor = customerAccountLoad.productID === productID && customerAccountLoad.status === "ready" && customerAccountLoad.hasMore ? customerAccountLoad.items.at(-1)?.id ?? "" : "";
    if (!cursor) return false;
    try {
      const page = await api.customerAccounts(productID, cursor);
      setCustomerAccountLoad((current) => {
        if (current.productID !== productID) return current;
        const known = new Set(current.items.map((item) => item.id));
        return { ...current, items: [...current.items, ...page.items.filter((item) => !known.has(item.id))], hasMore: page.has_more };
      });
      return true;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "More customer accounts could not be loaded.");
      return false;
    }
  }

  function reviewToolTestProposal(target: APITool, proposal: APIToolTestAnalysisProposal) {
    if ((target.backend_kind ?? "http") !== "http" || target.state !== "draft") {
      showToast("Create an independent HTTP draft before reviewing this proposal in Builder.");
      return;
    }
    if (target.id === proposal.base_tool_id && target.revision !== proposal.base_revision) {
      showToast("The tool revision changed after analysis. Run a new live test before reviewing proposed changes.");
      return;
    }
    const seededDraft = {
      ...proposal.draft,
      namespace: target.namespace,
      name: target.name,
      endpoint: target.endpoint ?? "",
      upstream_auth: target.upstream_auth ?? proposal.draft.upstream_auth,
      request_example: target.request_example,
      response_example: target.response_example,
      credential_present: Boolean(target.credential_present),
    };
    setToolBuilderSelection({ uid: target.id, tool: target, failed: false });
    setToolBuilderSeed({
      toolID: target.id,
      revision: target.revision,
      proposal: {
        proposal_id: proposal.proposal_id,
        summary: "Suggested from consented sanitized live-test evidence. Accept or reject each field; nothing has been saved or published.",
        valid: proposal.valid,
        draft: seededDraft,
        changes: proposal.changes,
        findings: proposal.findings,
      },
    });
    navigateToPath(toolBuilderPath(target.id));
  }

  const publicEndpoint = distribution?.public_mcp_endpoint ?? "/mcp/public";
  const publicAgentSetupURL = distribution?.agent_setup?.public.url ?? "/agent-setup/public/prompt.md";
  const privateAgentSetupURL = distribution?.agent_setup?.private.url ?? "/agent-setup/private/prompt.md";
  const publicAgentSetup = distribution?.agent_setup?.public ?? { available: publicMCPEnabled, unavailable_reason: "public_mcp_disabled" as const, url: publicAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(product.name, publicAgentSetupURL, "public"), contains_secret: false as const };
  const privateAgentSetup = distribution?.agent_setup?.private ?? { available: identityConfig?.configured === true && identityConfig.state === "active", unavailable_reason: "identity_unavailable" as const, url: privateAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(product.name, privateAgentSetupURL, "private"), contains_secret: false as const };
  const mcpConnectionReady = Boolean(mcpName.trim() && mcpNamespace.trim() && mcpEndpoint.trim() && (mcpAuthMode !== "service" || mcpCredential.trim()) && (mcpAuthMode !== "delegated_oauth" || (mcpOAuthClientID.trim() && mcpOAuthClientSecret.trim() && mcpOAuthIssuer.trim() && mcpAuthorizationURL.trim() && mcpTokenURL.trim())));
  const activeNavigation = navigation.find((item) => item.sections.some((candidate) => candidate.id === section));
  const selectedToolBuilderTool = toolBuilderUID && toolBuilderSelection?.uid === toolBuilderUID ? toolBuilderSelection.tool : null;
  const toolBuilderLoadFailed = Boolean(toolBuilderUID && toolBuilderSelection?.uid === toolBuilderUID && toolBuilderSelection.failed);
  const activeToolBuilderSeed = selectedToolBuilderTool && toolBuilderSeed?.toolID === selectedToolBuilderTool.id && toolBuilderSeed.revision === selectedToolBuilderTool.revision ? toolBuilderSeed.proposal : null;
  const toolBuilderIntegrationID = consoleRoute.kind === "tool-builder" ? consoleRoute.integrationID ?? selectedToolBuilderTool?.owner_integration_id : undefined;
  const toolBuilderIntegration = toolBuilderIntegrationID ? integrations.find((integration) => integration.id === toolBuilderIntegrationID) : undefined;
  const toolBuilderContent = consoleRoute.kind !== "tool-builder" ? null
    : consoleRoute.uid && !selectedToolBuilderTool && !toolBuilderLoadFailed ? <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon"><RefreshCw /></span><div><h1>Loading HTTP tool draft</h1><p>Loading the complete endpoint, authentication policy, mappings, and examples…</p></div></section>
    : consoleRoute.uid && toolBuilderLoadFailed ? <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>Complete HTTP tool draft unavailable</h1><p>The complete redacted contract could not be loaded. Catalog summary data is never used as an editable fallback.</p></div><span className="heading-actions"><Button outline onClick={() => { setToolBuilderSelection(null); setToolBuilderLoadAttempt((value) => value + 1); }}><RefreshCw data-slot="icon" />Retry</Button><ConsoleLink path={sectionPath("tools")} onNavigate={navigateToPath} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink></span></section>
    : consoleRoute.uid && (!selectedToolBuilderTool || (selectedToolBuilderTool.backend_kind ?? "http") !== "http") ? <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>HTTP tool draft unavailable</h1><p>This tool does not exist or is source-managed outside the HTTP Tool Builder.</p></div><ConsoleLink path={sectionPath("tools")} onNavigate={navigateToPath} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink></section>
    : toolBuilderIntegrationID && !toolBuilderIntegration ? <section className="panel entity-missing" role="alert"><span className="entity-missing-icon"><TriangleAlert /></span><div><h1>Owning API unavailable</h1><p>This API-owned tool cannot be edited without its exact API context.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={navigateToPath} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>
    : toolBuilderIntegration ? <IntegrationToolBuilderRoute key={`${consoleRoute.path}:${selectedToolBuilderTool?.revision ?? 0}:${activeToolBuilderSeed?.proposal_id ?? "manual"}`} integration={toolBuilderIntegration} product={product} grants={grantDefinitions} tool={selectedToolBuilderTool} initialProposal={activeToolBuilderSeed} aiAvailable={aiProfiles.some((profile) => profile.workload === "analysis" && profile.enabled)} onSaved={refreshToolsAfterBuilderSave} onDirtyChange={handleToolBuilderDirtyChange} onMessage={showToast} onNavigate={navigateToPath} />
    : <ToolBuilderView key={`${consoleRoute.path}:${selectedToolBuilderTool?.revision ?? 0}:${activeToolBuilderSeed?.proposal_id ?? "manual"}`} product={product} grants={grantDefinitions} tool={selectedToolBuilderTool} initialProposal={activeToolBuilderSeed} aiAvailable={aiProfiles.some((profile) => profile.workload === "analysis" && profile.enabled)} onSaved={refreshToolsAfterBuilderSave} onDirtyChange={handleToolBuilderDirtyChange} onMessage={showToast} onNavigate={navigateToPath} />;
  const entityDetail = useEntityDetail({
    consoleRoute,
    integrations,
    widgets,
    resourceSets,
    sources,
    tools,
    mcpConnections,
    accessDefinitions,
    accessConnections,
    productInstallations,
    productVersions,
    integrationRuns,
    supportRoutes,
    reportSubmissions,
    auditEvents,
    rootUsers,
  });


  const workspaceClass = consoleRoute.kind === "tool-builder"
    ? "workspace-wide"
    : section === "identity" || section === "settings"
      ? "workspace-compact"
      : "workspace-default";

  return (
    <Suspense fallback={<div className="console-loading-boundary" role="status"><RefreshCw className="spin" /><span>Loading console workspace…</span></div>}>
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <ConsoleSidebar
        section={section}
        activeNavigationID={activeNavigation?.id}
        currentUser={currentUser}
        onLogout={onLogout}
        onNavigate={navigateToPath}
      />

      <main id="main-content" className={workspaceClass} tabIndex={-1}>
        <ConsoleTopbar
          productName={product.name}
          section={section}
          activeNavigationID={activeNavigation?.id}
          onGroupChange={navigateToGroup}
        />

        <div className="content">
          {fixturePreview && <div className="workspace-notice preview" role="status"><Eye /><span><strong>Fixture preview</strong><small>This development-only console uses sample data and does not represent a connected deployment.</small></span></div>}
          {workspaceLoading && <div className="workspace-notice loading" role="status"><RefreshCw className="spin" /><span><strong>Loading deployment data</strong><small>Catalog, access, runtime, and activity data are being loaded from the service.</small></span></div>}
          {workspaceLoadProblems.length > 0 && <div className="workspace-notice error" role="alert"><TriangleAlert /><span><strong>Some deployment data could not be loaded</strong><small>{workspaceLoadProblems.join(" · ")}</small></span><Button outline onClick={() => window.location.reload()}><RefreshCw data-slot="icon" />Reload</Button></div>}
          <ViewStack>
          {consoleRoute.kind === "not-found" ? <ConsoleNotFoundView path={consoleRoute.path} onNavigate={navigateToPath} /> : consoleRoute.kind === "tool-builder" ? toolBuilderContent : consoleRoute.kind === "entity" && consoleRoute.entity === "integration" ? <IntegrationsView integrations={integrations} analyses={analyses} tools={tools} resourceSets={resourceSets} sources={sources} supportRoutes={supportRoutes} connections={accessConnections} identity={identityConfig} distribution={distribution} selectedIntegrationID={consoleRoute.uid} activeTab={consoleRoute.integrationTab} activeResourceTab={consoleRoute.integrationResourceTab ?? "documentation"} onBuild={() => setProductBuilderOpen(true)} onAddSource={() => setAddSourceOpen(true)} onCrawlSource={crawlSource} onPublishSource={publishSource} onAttachPublishedSource={attachReviewedSourcePublication} onGenerateAgentGuide={generateIntegrationAgentGuide} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "widget" ? <WidgetDetailView key={`${consoleRoute.uid}:${widgets.find((item) => item.id === consoleRoute.uid)?.revision ?? 0}`} widget={widgets.find((item) => item.id === consoleRoute.uid) ?? null} integrations={integrations} recipes={recipes} assistantAvailable={aiProfiles.some((profile) => profile.workload === "assistant" && profile.enabled)} busy={widgetBusy} onUpdate={updateWidget} onSetState={setWidgetState} onRotateSecret={rotateWidgetSecret} onConfigureAssistant={() => { navigateToPath(settingsPath("ai")); openLLMProfile("assistant"); }} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "resource-set" ? <ResourceSetDetailView resource={resourceSets.find((item) => item.id === consoleRoute.uid) ?? null} integrations={integrations} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "tool" ? <ToolDetailView key={`${consoleRoute.uid}:${tools.find((item) => item.id === consoleRoute.uid)?.revision ?? 0}`} productID={product.id} tool={tools.find((item) => item.id === consoleRoute.uid) ?? null} connections={mcpConnections} integrations={integrations} auditEvents={auditEvents} onChanged={refreshTools} onReviewProposal={reviewToolTestProposal} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" ? <EntityDetailView route={consoleRoute} detail={entityDetail} onNavigate={navigateToPath} /> : <>
          {section === "distribution" && (
            <DistributionView
              enabled={publicMCPEnabled}
              onEnabledChange={requestMCPChange}
              resources={visibleResources}
              resourceFilter={resourceFilter}
              setResourceFilter={setResourceFilter}
              onVisibilityChange={requestVisibility}
              onCopied={showToast}
              publicEndpoint={publicEndpoint}
              tenantName={product.name}
              publicAgentSetup={publicAgentSetup}
              privateAgentSetup={privateAgentSetup}
              onConfigureIdentity={() => navigateToSection("identity")}
              customerAccounts={customerAccounts}
              customerAccountsStatus={customerAccountsStatus}
              customerAccountsHaveMore={customerAccountsHaveMore}
              onUpdateCustomerAccount={updateCustomerAccountState}
              onLoadMoreCustomerAccounts={loadMoreCustomerAccounts}
              onOpenSources={() => navigateToSection("sources")}
              widgetsEnabled={widgetsEnabled}
              widgetCount={widgets.length}
              onOpenWidgets={() => navigateToSection("widgets")}
            />
          )}
          {widgetsEnabled && section === "widgets" && <WidgetsView widgets={widgets} integrations={integrations} onCreate={() => setWidgetCreateOpen(true)} onNavigate={navigateToPath} />}
          {section === "product" && <IntegrationsView integrations={integrations} analyses={analyses} tools={tools} resourceSets={resourceSets} sources={sources} supportRoutes={supportRoutes} connections={accessConnections} identity={identityConfig} distribution={distribution} onBuild={() => setProductBuilderOpen(true)} onAddSource={() => setAddSourceOpen(true)} onCrawlSource={crawlSource} onPublishSource={publishSource} onAttachPublishedSource={attachReviewedSourcePublication} onGenerateAgentGuide={generateIntegrationAgentGuide} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
				{section === "identity" && <OIDCIdentitySetup key={identityLoading ? "loading" : identityConfig?.id || "identity"} identity={identityConfig} loading={identityLoading} loadError={identityLoadError} onChanged={setIdentityConfig} onMessage={showToast} />}
          {section === "recipes" && <RecipesView integrations={integrations} analyses={analyses} recipes={recipes} busy={recipeBusy} onCreate={createRecipe} onGenerate={() => generateRecipesFromEvidence()} onEdit={editRecipe} onRework={reworkRecipe} onApprove={approveRecipe} onPublish={publishRecipe} />}
          {section === "sources" && <SourcesView sources={sources} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} onNavigate={navigateToPath} />}
          {section === "projects" && <AccessView definitions={accessDefinitions} connections={accessConnections} instances={accessInstances} credentials={accessCredentials} integrations={integrations} environments={environments} apiResourceSets={resourceSets.filter((set) => set.kind === "api")} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} onNavigate={navigateToPath} />}
          {section === "tools" && <ToolsView tools={tools} integrations={integrations} connections={mcpConnections} nativePlugins={nativePlugins} onSetNativePluginEnabled={setNativePluginEnabled} onNavigate={navigateToPath} />}
          {section === "releases" && <ConnectorReleasesView versions={productVersions} integrations={integrations} onConfigure={openProductCatalog} onNavigate={navigateToPath} />}
          {section === "runs" && <ActivityHubView runs={integrationRuns} environments={environments} submissions={reportSubmissions} events={auditEvents} analytics={analytics} supportRoutes={supportRoutes} onStart={() => setRunOpen(true)} onComplete={completeIntegrationRun} onView={openSupportSubmission} onRetry={createSupportDeliveryAttempt} onNavigate={navigateToPath} />}
          {section === "reporting" && <ReportingView routes={supportRoutes} integrations={integrations} backendConnections={backendConnections} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "overview" && <SettingsView product={product} versions={productVersions} pins={productVersionPins} aiProfiles={aiProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onConfigureProduct={openProductCatalog} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "connections" && <AccessView definitions={accessDefinitions} connections={accessConnections} instances={accessInstances} credentials={accessCredentials} integrations={integrations} environments={environments} apiResourceSets={resourceSets.filter((set) => set.kind === "api")} settingsTab="connections" onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "reporting" && <ReportingView routes={supportRoutes} integrations={integrations} backendConnections={backendConnections} settingsTab="reporting" onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "storage" && <StorageSettingsView onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "ai" && <AISettingsView profiles={aiProfiles} connections={aiConnections} usage={aiProviderUsage} saving={llmBusy} onSave={saveAIWorkloadSelection} onConfigure={openLLMProfile} onAddProvider={() => setProviderPickerOpen(true)} onConnect={openAIConnection} onTest={testAIConnection} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "root" && <RootAccessSettingsView rootUsers={rootUsers} currentUser={currentUser ?? null} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
          </>}
        </ViewStack></div>
      </main>

      <WidgetDialogs workspace={widgetWorkspace} integrations={integrations} onMessage={showToast} />

      <PublicationDialogs workspace={publicationWorkspace} sources={sources} mcpConnections={mcpConnections} tools={tools} publicResourceCount={publicResourceCount} />

      <ProductReleaseDialogs workspace={productReleaseWorkspace} product={product} productDefinition={productDefinition} environments={environments} onNavigate={navigateToPath} />

      <SourceDialogs workspace={sourceWorkspace} />

      <MCPDialogs workspace={mcpWorkspace} connectionReady={mcpConnectionReady} />

      <AdminActivityDialogs workspace={adminWorkspace} />

      <AIConfigurationDialogs workspace={aiWorkspace} ProviderLogo={AIProviderLogo} />

      {widgetsEnabled && <WidgetPreviewLauncher widgets={widgets} currentWidgetID={consoleRoute.kind === "entity" && consoleRoute.entity === "widget" ? consoleRoute.uid : undefined} onOpenWidgets={() => navigateToSection("widgets")} />}
      {toast && <div className="toast" role="status"><Check />{toast}</div>}
    </div>
    </Suspense>
  );
}
