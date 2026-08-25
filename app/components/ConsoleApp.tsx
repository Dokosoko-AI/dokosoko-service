"use client";

import {
  ArrowLeft,
  BookOpen,
  Bot,
  Check,
  ChevronRight,
  Eye,
  ExternalLink,
  KeyRound,
  LockKeyhole,
  RefreshCw,
  Search,
  Share2,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  TriangleAlert,
  Users,
  Wrench,
  XCircle,
} from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIBackendConnection, APICustomerAccount, APIDeployment, APIError, APIGrantDefinition, APIIdentity, APIIntegration, APIMCPConnection, APIProduct, APIResourceSet, APISupportRoute, APITool, APIToolBuilderProposal, APIToolTestAnalysisProposal, APIUser, APIWidget, api } from "../lib/api";
import { sectionPath, settingsPath, toolBuilderPath } from "../lib/console-routes";
import type { ConsoleFixtures } from "../dev/console-fixtures";
import { Badge, Button, Dialog, Switch } from "./core/control";
import { Input, Select } from "./core";
import { ViewStack } from "./core/layout";
import { OIDCIdentitySetup } from "./OIDCIdentitySetup";
import { ToolBuilderView } from "./ToolBuilderView";
import { WidgetPreviewLauncher } from "./WidgetPreviewLauncher";
import { IntegrationToolBuilderRoute } from "./integrations/IntegrationToolBuilderRoute";
import { Confirmation, ConsoleLink, CopyButton, EntityLink, Source, WarningContent, aiModelOptions, aiProviderLabel, aiProviders, aiWorkloads, buildAgentSetupEmbedHTML } from "./console/shared";
import { ConsoleSidebar, ConsoleTopbar, navigation } from "./console/workspace-navigation";
import { useConsoleNavigation } from "./console/use-console-navigation";
import { useEntityDetail } from "./console/use-entity-detail";
import { useAdminActivityWorkspace } from "./console/use-admin-activity-workspace";
import { useAIWorkspaceState } from "./console/use-ai-workspace";
import { useMCPWorkspaceState } from "./console/use-mcp-workspace";
import { useProductReleaseState } from "./console/use-product-release-workspace";
import { usePublicationWorkflow } from "./console/use-publication-workflow";
import { useSourceWorkflow, type SourceKind } from "./console/use-source-workflow";
import { useWidgetWorkflow } from "./console/use-widget-workflow";

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
  const {
    mcpConnections, setMCPConnections,
    mcpConnectionOpen, setMCPConnectionOpen,
    mcpImportOpen, setMCPImportOpen,
    mcpBusy,
    mcpCatalog,
    mcpSelectedTools, setMCPSelectedTools,
    mcpImportFailures,
    mcpName, setMCPName,
    mcpNamespace, setMCPNamespace,
    mcpEndpoint, setMCPEndpoint,
    mcpAuthMode, setMCPAuthMode,
    mcpCredential, setMCPCredential,
    mcpOAuthClientID, setMCPOAuthClientID,
    mcpOAuthClientSecret, setMCPOAuthClientSecret,
    mcpOAuthIssuer, setMCPOAuthIssuer,
    mcpAuthorizationURL, setMCPAuthorizationURL,
    mcpTokenURL, setMCPTokenURL,
    mcpScopes, setMCPScopes,
    mcpGrants, setMCPGrants,
    mcpConfirmationRequired, setMCPConfirmationRequired,
    publicMCPEnabled, setPublicMCPEnabled,
    distribution, setDistribution,
    inspectMCPConnection,
    createMCPConnection,
    importMCPTools,
  } = useMCPWorkspaceState({ fixtures, product, apiConnected, setTools, showToast });
  const {
    widgetCreateOpen, setWidgetCreateOpen,
    widgetBusy,
    widgetName, setWidgetName,
    widgetOrigins, setWidgetOrigins,
    widgetIntegrationIDs, setWidgetIntegrationIDs,
    widgetCredential, setWidgetCredential,
    createWidget,
    updateWidget,
    setWidgetState,
    rotateWidgetSecret,
  } = useWidgetWorkflow({ setWidgets, onNavigate: navigateToPath, showToast });
  const {
    productDefinition, setProductDefinition,
    latestProductBuild, setLatestProductBuild,
    productBuilderOpen, setProductBuilderOpen,
    productBuildReviewOpen, setProductBuildReviewOpen,
    productBuilderBusy,
    productBuilderInputs, setProductBuilderInputs,
    pendingPublication, setPendingPublication,
    pendingMCPEnable, setPendingMCPEnable,
    acknowledged, setAcknowledged,
    setProductRevision,
    buildProductAutomatically,
    publishImportedAPIs,
    requestVisibility,
    confirmPublication,
    requestMCPChange,
    confirmMCPEnable,
  } = usePublicationWorkflow({
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
  const [resourceFilter, setResourceFilter] = useState<"all" | "public" | "private">("all");
		  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
		  const [identityLoading, setIdentityLoading] = useState(true);
		  const [identityLoadError, setIdentityLoadError] = useState("");
      const {
        analytics, setAnalytics,
        reportSubmissions, setReportSubmissions,
        reportDetail, setReportDetail,
        reportDetailBusy,
        rootUsers, setRootUsers,
        rootOpen, setRootOpen,
        rootBusy,
        rootEmail, setRootEmail,
        rootDisplayName, setRootDisplayName,
        rootPassword, setRootPassword,
        rootCode, setRootCode,
        rootEnrollment,
        rootRecoveryCodes, setRootRecoveryCodes,
        environments, setEnvironments,
        integrationRuns, setIntegrationRuns,
        auditEvents, setAuditEvents,
        runOpen, setRunOpen,
        runBusy,
        runEnvironmentID, setRunEnvironmentID,
        runOutcome, setRunOutcome,
        createSupportDeliveryAttempt,
        openSupportSubmission,
        beginRootUser,
        completeRootUser,
        revokeRootUser,
        startIntegrationRun,
        completeIntegrationRun,
      } = useAdminActivityWorkspace({ product, fixtures, currentUser, apiConnected, showToast });
      const {
        aiConnections,
        aiProfiles,
        analyses,
        recipes,
        aiProviderUsage,
        recipeBusy,
        llmOpen, setLLMOpen,
        llmBusy,
        llmRole,
        llmConnectionID,
        providerOpen, setProviderOpen,
        providerPickerOpen, setProviderPickerOpen,
        providerBusy,
        providerEnabled, setProviderEnabled,
        providerIsBackup, setProviderIsBackup,
        providerBackupAnalysisModel, setProviderBackupAnalysisModel,
        providerBackupAssistantModel, setProviderBackupAssistantModel,
        llmProvider,
        llmEndpoint, setLLMEndpoint,
        llmModel, setLLMModel,
        llmCredential, setLLMCredential,
        llmInputTokens, setLLMInputTokens,
        llmOutputTokens, setLLMOutputTokens,
        llmDailyBudget, setLLMDailyBudget,
        llmEnabled, setLLMEnabled,
        openAIConnection,
        openLLMProfile,
        changeLLMConnection,
        saveAIConnection,
        testAIConnection,
        saveLLMProfile,
        saveAIWorkloadSelection,
        createRecipe,
        generateRecipesFromEvidence,
        generateIntegrationAgentGuide,
        reworkRecipe,
        editRecipe,
        approveRecipe,
        publishRecipe,
        runSystemDoctor,
      } = useAIWorkspaceState({
        product,
        fixturePreview,
        onLoadProblem: recordWorkspaceLoadProblem,
        showToast,
      });
      const {
        productCatalogOpen, setProductCatalogOpen,
        productCatalogBusy,
        productDescription, setProductDescription,
        defaultVersionPolicy, setDefaultVersionPolicy,
        requirePromotionApproval, setRequirePromotionApproval,
        productVersions,
        productVersionPins,
        customerAccountLoad, setCustomerAccountLoad,
        customerAccounts,
        customerAccountsStatus,
        customerAccountsHaveMore,
        productInstallations,
        pinHistory,
        newProductVersion, setNewProductVersion,
        newProductProfile, setNewProductProfile,
        newVersionLatest, setNewVersionLatest,
        newVersionLTS, setNewVersionLTS,
        newVersionStage, setNewVersionStage,
        newVersionRollout, setNewVersionRollout,
        pinScope, setPinScope,
        pinCustomerID, setPinCustomerID,
        pinVersionID, setPinVersionID,
        pinReason, setPinReason,
        versionLifecycleOpen, setVersionLifecycleOpen,
        editingProductVersion,
        lifecycleLatest, setLifecycleLatest,
        lifecycleLTS, setLifecycleLTS,
        lifecycleDeprecated, setLifecycleDeprecated,
        lifecycleMessage, setLifecycleMessage,
        lifecycleReplacement, setLifecycleReplacement,
        lifecycleSunset, setLifecycleSunset,
        lifecycleRollout, setLifecycleRollout,
        lifecycleImpact,
        lifecycleImpactAcknowledged, setLifecycleImpactAcknowledged,
        installationName, setInstallationName,
        installationExternalID, setInstallationExternalID,
        installationCustomerID, setInstallationCustomerID,
        installationEnvironmentID, setInstallationEnvironmentID,
        openProductCatalog,
        saveProductDiscoverySettings,
        rewriteDescriptionWithAI,
        publishProductVersion,
        editProductVersion,
        saveProductVersionLifecycle,
        pinCustomerVersion,
        saveInstallation,
        reconcileVersion,
        promoteVersion,
        removeProductVersionPin,
      } = useProductReleaseState({
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
	    Promise.all([api.distribution(product.id), api.sources(product.id), api.tools(product.id), api.mcpConnections(product.id)]).then(async ([distributionValue, remoteSources, remoteTools, remoteMCPConnections]) => {
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

  const {
    addSourceOpen, setAddSourceOpen,
    sourceName, setSourceName,
    sourceKind,
    sourceLocation, setSourceLocation,
    sourceFile,
    sourceFileError,
    sourceFileInput,
    sourceBusy,
    sourceReview,
    sourceReviewSelection, setSourceReviewSelection,
    sourceReviewAcknowledged, setSourceReviewAcknowledged,
    sourceReviewBusy,
    sourceReviewAttachIntegrationID,
    closeSourceDialog,
    selectSourceKind,
    selectSourceFile,
    createSource,
    crawlSource,
    attachReviewedSourcePublication,
    publishSource,
    closeSourceReview,
    confirmSourcePublication,
  } = useSourceWorkflow({ product, apiConnected, sources, setSources, refreshCatalog, showToast });

  async function refreshTools() {
    const toolValues = await api.tools(product.id);
    setTools(toolValues);
    const eventValues = await api.auditEvents(product.organisation_id).catch(() => null);
    if (eventValues) setAuditEvents(eventValues);
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
    if (target.backend_kind === "mcp" || target.state !== "draft") {
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
    : consoleRoute.uid && (!selectedToolBuilderTool || selectedToolBuilderTool.backend_kind === "mcp") ? <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>HTTP tool draft unavailable</h1><p>This tool does not exist or is managed through an MCP connection.</p></div><ConsoleLink path={sectionPath("tools")} onNavigate={navigateToPath} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink></section>
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
          {section === "tools" && <ToolsView tools={tools} integrations={integrations} connections={mcpConnections} onNavigate={navigateToPath} />}
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

      <Dialog
        open={widgetCreateOpen}
        onClose={setWidgetCreateOpen}
        title="Create widget"
        description="Start with one authenticated widget, then connect only the APIs it should expose."
        actions={<><Button outline onClick={() => setWidgetCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={widgetBusy || !widgetName.trim() || !widgetOrigins.trim() || widgetIntegrationIDs.length === 0} onClick={createWidget}>{widgetBusy ? "Creating…" : "Create widget"}</Button></>}
      >
        <div className="auth-form compact-form">
          <label className="auth-field"><span>Name</span><input value={widgetName} maxLength={120} onChange={(event) => setWidgetName(event.target.value)} placeholder="Customer assistant" /></label>
          <label className="auth-field"><span>Allowed application origins</span><textarea value={widgetOrigins} onChange={(event) => setWidgetOrigins(event.target.value)} placeholder={"https://app.example.com\nhttp://localhost:3000"} /><small>One exact origin per line. Paths and wildcard domains are not accepted.</small></label>
          <fieldset className="widget-api-picker"><legend>APIs this widget can use</legend>{integrations.filter((integration) => integration.lifecycle === "active").map((integration) => <label key={integration.id}><input aria-label={`Allow ${integration.display_name}`} type="checkbox" checked={widgetIntegrationIDs.includes(integration.id)} onChange={(event) => setWidgetIntegrationIDs((values) => event.target.checked ? [...values, integration.id] : values.filter((id) => id !== integration.id))} /><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span></label>)}{integrations.filter((integration) => integration.lifecycle === "active").length === 0 && <p className="empty-picker">Publish an API before creating a widget.</p>}</fieldset>
        </div>
      </Dialog>

      <Dialog
        open={Boolean(widgetCredential)}
        onClose={(open) => { if (!open) setWidgetCredential(null); }}
        title="Save the widget secret"
        description="This server-only credential is shown once. DokoSoko stores only its hash."
        actions={<Button color="indigo" onClick={() => setWidgetCredential(null)}>I saved it</Button>}
      >
        <div className="one-time-secret"><div><KeyRound /><span><strong>Server only</strong><small>Never place this value in browser code or NEXT_PUBLIC variables.</small></span></div><code>{widgetCredential?.secret}</code><CopyButton text={widgetCredential?.secret ?? ""} label="Copy secret" onCopied={showToast} /></div>
      </Dialog>

      <Dialog
        open={Boolean(pendingPublication)}
        onClose={(open) => { if (!open) setPendingPublication(null); }}
        title={`Make ${pendingPublication?.name ?? "resource"} public?`}
        description="This is a security-sensitive publication change. Private is the default for every new source."
        actions={<><Button outline onClick={() => setPendingPublication(null)}>Keep private</Button><Button color="red" disabled={!acknowledged} onClick={confirmPublication}>Make public</Button></>}
      >
        <WarningContent>
          <p><strong>{pendingPublication?.detail}</strong> Public MCP does not require users to sign in.</p>
          <p>DokoSoko will record your identity, the prior revision, and this decision in the audit log.</p>
          <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand this published {pendingPublication?.kind} will be available without authentication.</Confirmation>
        </WarningContent>
      </Dialog>

      <Dialog
        open={productBuilderOpen}
        onClose={setProductBuilderOpen}
        title="Import APIs"
		description="Add specs, documentation, repositories, or MCP endpoints. DokoSoko will group them into reviewable APIs."
        actions={<><Button outline onClick={() => setProductBuilderOpen(false)}>Cancel</Button><Button color="indigo" disabled={productBuilderBusy} onClick={buildProductAutomatically}>{productBuilderBusy ? "Scanning…" : "Import APIs"}</Button></>}
      >
        <div className="product-builder-form">
          <div className="builder-source-summary">
            <span><BookOpen /><strong>{sources.length}</strong><small>docs & specs</small></span>
            <span><Share2 /><strong>{mcpConnections.length}</strong><small>MCP upstreams</small></span>
            <span><Wrench /><strong>{tools.length}</strong><small>tools</small></span>
          </div>
          <label className="auth-field"><span>Anything else?</span><textarea value={productBuilderInputs} onChange={(event) => setProductBuilderInputs(event.target.value)} placeholder={"Paste specs, documentation, repositories, or MCP endpoints—one per line.\nhttps://api.example.com/voice/v3/openapi.yaml\nhttps://github.com/acme/voice-examples"} /><small>Optional. DokoSoko automatically classifies each input and never retrieves credentials embedded in a URL.</small></label>
          <div className="builder-magic-note"><Sparkles /><span><strong>Review exceptions, not configuration.</strong> Exact matches are joined automatically. Ambiguous version relationships stay unresolved and cannot silently fall back.</span></div>
          {latestProductBuild?.state === "review" && <button type="button" className="panel-footer-link builder-review-link" onClick={() => { setProductBuilderOpen(false); setProductBuildReviewOpen(true); }}>Review the latest unpublished proposal <ChevronRight /></button>}
        </div>
      </Dialog>

      <Dialog
        open={productBuildReviewOpen}
        onClose={setProductBuildReviewOpen}
        title="Review imported APIs"
        description="Nothing is added to the catalogue until this exact proposal is reviewed and published."
        actions={<><Button outline onClick={() => setProductBuildReviewOpen(false)}>Keep as draft</Button><Button color="indigo" disabled={productBuilderBusy || !latestProductBuild || latestProductBuild.state !== "review" || latestProductBuild.unresolved.some((finding) => finding.level === "error")} onClick={publishImportedAPIs}>{productBuilderBusy ? "Publishing…" : "Publish proposal"}</Button></>}
      >
        {latestProductBuild ? <div className="product-build-review">
          <div className="builder-source-summary">
            <span><BookOpen /><strong>{latestProductBuild.inputs.length}</strong><small>inputs scanned</small></span>
            <span><TerminalSquare /><strong>{latestProductBuild.proposal.components.length}</strong><small>APIs proposed</small></span>
            <span><TriangleAlert /><strong>{latestProductBuild.unresolved.length}</strong><small>exceptions</small></span>
          </div>
          <div className="product-build-components">
            {latestProductBuild.proposal.components.map((component) => <section className="catalog-settings-section" key={component.id}>
              <div className="catalog-settings-heading"><span><strong>{component.name}</strong><small>{component.releases.length} independently versioned release{component.releases.length === 1 ? "" : "s"}</small></span><Badge color="violet">API</Badge></div>
              {component.releases.map((release) => <div className="build-release-row" key={release.id}><span><strong>{release.version}</strong><small>{release.bindings.map((binding) => binding.name).join(" · ") || "No bindings"}</small></span><Badge color={release.bindings.every((binding) => binding.verified) ? "green" : "amber"}>{release.bindings.filter((binding) => binding.verified).length}/{release.bindings.length} verified</Badge></div>)}
            </section>)}
          </div>
          {latestProductBuild.unresolved.length > 0 && <section className="catalog-settings-section"><div className="catalog-settings-heading"><span><strong>Resolve before publishing</strong><small>Ambiguous relationships never silently fall back.</small></span></div>{latestProductBuild.unresolved.map((finding, index) => <div className={`publish-validation ${finding.level}`} key={`${finding.code}:${index}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code}</strong><small>{finding.message}</small></span></div>)}</section>}
        </div> : <div className="empty-row">No import proposal is ready for review.</div>}
      </Dialog>

      <Dialog
        open={productCatalogOpen}
        onClose={setProductCatalogOpen}
        title="Advanced publishing"
		description="Manage immutable compatibility snapshots, staged rollout, and scoped version resolution."
        actions={<><Button outline onClick={() => setProductCatalogOpen(false)}>Close</Button><Button color="indigo" disabled={productCatalogBusy || !productDescription.trim()} onClick={saveProductDiscoverySettings}>{productCatalogBusy ? "Saving…" : "Save discovery settings"}</Button></>}
      >
        <div className="product-catalog-settings">
          <section className="catalog-settings-section">
            <div className="catalog-settings-heading"><span><strong>Agent-facing deployment</strong><small>This description is returned verbatim during deployment discovery.</small></span><Button outline disabled={productCatalogBusy || !productDescription.trim()} onClick={rewriteDescriptionWithAI}><Sparkles data-slot="icon" />Rewrite for agents</Button></div>
            <label className="auth-field"><span>Deployment description</span><textarea maxLength={1000} value={productDescription} onChange={(event) => setProductDescription(event.target.value)} placeholder="What does this DokoSoko deployment enable, for whom, and within what boundaries?" /><small>{productDescription.length}/1000 · AI produces an editable draft and never saves automatically.</small></label>
			<div className="two-fields"><label className="auth-field"><span>Unpinned customer default</span><select value={defaultVersionPolicy} onChange={(event) => setDefaultVersionPolicy(event.target.value as "latest" | "lts")}><option value="latest">Latest channel</option><option value="lts">LTS channel</option></select><small>Resolution: installation → environment → customer → channel → healthy fallback.</small></label><label className="auth-field"><span>Catalog revision</span><input value={`r${product.catalog_revision}`} readOnly /><small>Agents compare this revision and the effective manifest hash to invalidate cached discovery.</small></label></div>
			<label className="compact-check"><input type="checkbox" checked={requirePromotionApproval} onChange={(event) => setRequirePromotionApproval(event.target.checked)} /><span>Require a different administrator to approve active/Latest/LTS promotion</span></label>
          </section>

          <section className="catalog-settings-section">
            <div className="catalog-settings-heading"><span><strong>Publish compatibility snapshot</strong><small>Creates an immutable deployment snapshot of tested API versions.</small></span></div>
            <div className="product-version-create">
              <label className="auth-field"><span>Version</span><input value={newProductVersion} onChange={(event) => setNewProductVersion(event.target.value)} placeholder="2026.8" /></label>
              <label className="auth-field"><span>Compatibility profile</span><select value={newProductProfile} onChange={(event) => setNewProductProfile(event.target.value)}><option value="">Select profile</option>{productDefinition?.profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.name}</option>)}</select></label>
			  <label className="auth-field"><span>Stage</span><select value={newVersionStage} onChange={(event) => setNewVersionStage(event.target.value as "preview" | "active")}><option value="active">Active</option><option value="preview">Preview</option></select></label>
			  <label className="auth-field"><span>Latest rollout</span><input type="number" min={1} max={100} value={newVersionRollout} onChange={(event) => setNewVersionRollout(Number(event.target.value))} /><small>Deterministic % by installation/customer.</small></label>
			  <label className="compact-check"><input type="checkbox" checked={newVersionLatest} onChange={(event) => setNewVersionLatest(event.target.checked)} /><span>Latest</span></label>
			  <label className="compact-check"><input type="checkbox" checked={newVersionLTS} onChange={(event) => setNewVersionLTS(event.target.checked)} /><span>LTS</span></label>
              <Button color="indigo" disabled={productCatalogBusy || !newProductVersion.trim() || !newProductProfile} onClick={publishProductVersion}>Publish version</Button>
            </div>
            <div className="product-version-list">
			  {productVersions.map((version) => <article className="product-version-row" key={version.id}><span className="version-name"><strong>{version.version}</strong><small>{version.profile_name} · {version.diff.summary}</small><code>{version.manifest_hash}</code></span><span className="version-labels">{version.is_latest && <Badge color="blue">Latest · {version.rollout_percentage}%</Badge>}{version.is_lts && <Badge color="violet">LTS</Badge>}{version.release_stage === "preview" && <Badge color="amber">Preview</Badge>}{version.promotion_state === "pending" && <Badge color="amber">Approval pending</Badge>}{version.drift_status === "healthy" ? <Badge color="green">Healthy</Badge> : <Badge color="red">Drifted</Badge>}{version.deprecated_at && <Badge color="red">Deprecated</Badge>}</span><Button outline onClick={() => editProductVersion(version)}>Review release</Button></article>)}
              {productVersions.length === 0 && <div className="empty-row">Publish APIs, add a deployment description, then create the first compatibility snapshot.</div>}
            </div>
          </section>

          <section className="catalog-settings-section">
			<div className="catalog-settings-heading"><span><strong>Scoped version pins</strong><small>Use the narrowest scope. Authenticated installation identity wins over environment and customer assignments.</small></span></div>
			<div className="product-pin-create">
			  <label className="auth-field"><span>Scope</span><select value={pinScope} onChange={(event) => { setPinScope(event.target.value as typeof pinScope); setPinCustomerID(""); }}><option value="customer">Customer</option><option value="environment">Environment</option><option value="installation">Installation</option></select></label>
			  {pinScope === "customer" ? <label className="auth-field"><span>Customer account</span><select value={pinCustomerID} onChange={(event) => setPinCustomerID(event.target.value)}><option value="">Select customer account</option>{customerAccounts.map((item) => <option key={item.id} value={item.id}>{item.external_id} · {item.state}</option>)}</select></label> : <label className="auth-field"><span>{pinScope === "environment" ? "Environment" : "Installation"}</span><select value={pinCustomerID} onChange={(event) => setPinCustomerID(event.target.value)}><option value="">Select {pinScope}</option>{pinScope === "environment" ? environments.map((item) => <option key={item.id} value={item.id}>{item.name}</option>) : productInstallations.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.customer_account_id}</option>)}</select></label>}
              <label className="auth-field"><span>Compatibility snapshot</span><select value={pinVersionID} onChange={(event) => setPinVersionID(event.target.value)}><option value="">Select snapshot</option>{productVersions.filter((version) => !version.deprecated_at).map((version) => <option key={version.id} value={version.id}>{version.version}{version.is_lts ? " · LTS" : version.is_latest ? " · Latest" : ""}</option>)}</select></label>
              <label className="auth-field"><span>Reason</span><input value={pinReason} onChange={(event) => setPinReason(event.target.value)} placeholder="Production stability window" /></label>
              <Button color="indigo" disabled={productCatalogBusy || !pinCustomerID.trim() || !pinVersionID} onClick={pinCustomerVersion}>Save pin</Button>
            </div>
			<div className="product-pin-list">{productVersionPins.map((pin) => <article key={pin.id}><span><strong>{pin.scope}: {pin.scope_id}</strong><small>{pin.reason || "Explicit compatibility assignment"} · revision {pin.revision}</small></span><Badge color="violet">{pin.product_version}</Badge><Button outline onClick={() => removeProductVersionPin(pin)}>Remove</Button></article>)}{productVersionPins.length === 0 && <div className="empty-row">No exact pins. Resolution follows the {defaultVersionPolicy.toUpperCase()} channel.</div>}</div>
			{pinHistory.length > 0 && <small className="catalog-history-note">{pinHistory.length} immutable pin change{pinHistory.length === 1 ? "" : "s"} recorded in assignment history.</small>}
		  </section>

		  <section className="catalog-settings-section">
			<div className="catalog-settings-heading"><span><strong>Customer installations</strong><small>Register the authenticated installation claim and bind it to one customer and environment.</small></span></div>
			<div className="product-pin-create"><label className="auth-field"><span>Name</span><input value={installationName} onChange={(event) => setInstallationName(event.target.value)} placeholder="Contoso voice production" /></label><label className="auth-field"><span>Authenticated external ID</span><input value={installationExternalID} onChange={(event) => setInstallationExternalID(event.target.value)} placeholder="contoso-voice-prod" /></label><label className="auth-field"><span>Customer account</span><select value={installationCustomerID} onChange={(event) => setInstallationCustomerID(event.target.value)}><option value="">Select customer account</option>{customerAccounts.filter((item) => item.state === "active").map((item) => <option key={item.id} value={item.id}>{item.external_id}</option>)}</select></label><label className="auth-field"><span>Environment</span><select value={installationEnvironmentID} onChange={(event) => setInstallationEnvironmentID(event.target.value)}>{environments.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><Button color="indigo" disabled={productCatalogBusy || !installationName.trim() || !installationExternalID.trim() || !installationCustomerID.trim()} onClick={saveInstallation}>Add installation</Button></div>
			<div className="product-pin-list">{productInstallations.map((item) => <article key={item.id}><span><EntityLink entity="installation" uid={item.id} onNavigate={navigateToPath} className="entity-link"><strong>{item.name}</strong></EntityLink><small>{item.external_id} · {customerAccounts.find((account) => account.id === item.customer_account_id)?.external_id ?? item.customer_account_id} · {environments.find((environment) => environment.id === item.environment_id)?.name ?? item.environment_id}</small></span><Badge color={item.state === "active" ? "green" : "zinc"}>{item.state}</Badge></article>)}</div>
		  </section>
        </div>
      </Dialog>

      <Dialog
        open={versionLifecycleOpen}
        onClose={setVersionLifecycleOpen}
        title={`Lifecycle for ${editingProductVersion?.version ?? "compatibility snapshot"}`}
		description="Review the generated release diff, integrity, artifact health, promotion, rollout, and deprecation impact."
		actions={<><Button outline onClick={() => setVersionLifecycleOpen(false)}>Close</Button>{editingProductVersion?.release_stage === "preview" && editingProductVersion.promotion_state !== "pending" && <Button outline disabled={productCatalogBusy} onClick={() => promoteVersion(editingProductVersion, "request")}>Request promotion</Button>}{editingProductVersion?.promotion_state === "pending" && <Button color="indigo" disabled={productCatalogBusy} onClick={() => promoteVersion(editingProductVersion, "approve")}>Approve promotion</Button>}<Button color="indigo" disabled={productCatalogBusy || (lifecycleDeprecated && (!lifecycleMessage.trim() || Boolean(lifecycleImpact && !lifecycleImpactAcknowledged)))} onClick={saveProductVersionLifecycle}>Save lifecycle</Button></>}
      >
		<div className="version-lifecycle-form">
		  {editingProductVersion && <div className="release-integrity-card"><span><small>Immutable manifest</small><code>{editingProductVersion.manifest_hash}</code></span><span><small>Generated diff from {editingProductVersion.diff.from_version || "initial release"}</small><strong>{editingProductVersion.diff.summary}</strong></span><span><small>Artifact health</small><Badge color={editingProductVersion.drift_status === "healthy" ? "green" : "red"}>{editingProductVersion.drift_status}</Badge></span><Button outline disabled={productCatalogBusy} onClick={() => reconcileVersion(editingProductVersion)}><RefreshCw data-slot="icon" />Recheck artifacts</Button></div>}
		  {editingProductVersion && (editingProductVersion.diff.added.length + editingProductVersion.diff.removed.length + editingProductVersion.diff.changed.length > 0) && <div className="release-diff-list">{editingProductVersion.diff.added.map((change) => <span key={`a-${change.path}`}><Badge color="green">Added</Badge><code>{change.path}</code><small>{change.after}</small></span>)}{editingProductVersion.diff.removed.map((change) => <span key={`r-${change.path}`}><Badge color="red">Removed</Badge><code>{change.path}</code><small>{change.before}</small></span>)}{editingProductVersion.diff.changed.map((change) => <span key={`c-${change.path}`}><Badge color="amber">Changed</Badge><code>{change.path}</code><small>{change.before || "—"} → {change.after || "—"}</small></span>)}</div>}
		  {editingProductVersion?.promotion_state === "pending" && <div className="builder-magic-note"><ShieldCheck /><span><strong>Independent approval required.</strong> The publishing administrator cannot approve this preview. Approval rechecks artifact health before activating the requested channel labels.</span></div>}
		  <div className="lifecycle-toggles"><label className="compact-check"><input type="checkbox" disabled={lifecycleDeprecated} checked={lifecycleLatest} onChange={(event) => setLifecycleLatest(event.target.checked)} /><span>Latest</span></label><label className="compact-check"><input type="checkbox" disabled={lifecycleDeprecated} checked={lifecycleLTS} onChange={(event) => setLifecycleLTS(event.target.checked)} /><span>LTS</span></label><label className="compact-check danger"><input type="checkbox" checked={lifecycleDeprecated} onChange={(event) => { setLifecycleDeprecated(event.target.checked); if (event.target.checked) { setLifecycleLatest(false); setLifecycleLTS(false); } }} /><span>Deprecated</span></label></div>
		  <label className="auth-field"><span>Latest rollout percentage</span><input type="range" min={1} max={100} value={lifecycleRollout} onChange={(event) => setLifecycleRollout(Number(event.target.value))} /><small>{lifecycleRollout}% of deterministic installation/customer buckets; the rest remain on the prior healthy release.</small></label>
		  {lifecycleDeprecated && <><label className="auth-field"><span>Agent migration guidance</span><textarea maxLength={500} value={lifecycleMessage} onChange={(event) => setLifecycleMessage(event.target.value)} placeholder="Explain why this version is deprecated and what the agent should use instead." /></label><div className="two-fields"><label className="auth-field"><span>Replacement version</span><select value={lifecycleReplacement} onChange={(event) => setLifecycleReplacement(event.target.value)}><option value="">No replacement</option>{productVersions.filter((version) => version.id !== editingProductVersion?.id && !version.deprecated_at).map((version) => <option key={version.id} value={version.version}>{version.version}</option>)}</select></label><label className="auth-field"><span>Sunset date</span><input type="date" value={lifecycleSunset} onChange={(event) => setLifecycleSunset(event.target.value)} /></label></div></>}
		  {lifecycleDeprecated && lifecycleImpact && <div className="deprecation-impact"><strong>30-day impact</strong><span>{lifecycleImpact.customer_pins} customer · {lifecycleImpact.environment_pins} environment · {lifecycleImpact.installation_pins} installation pins</span><span>{lifecycleImpact.requests_30_days.toLocaleString()} MCP requests · {lifecycleImpact.tool_calls_30_days.toLocaleString()} tool calls</span><label className="compact-check danger"><input type="checkbox" checked={lifecycleImpactAcknowledged} onChange={(event) => setLifecycleImpactAcknowledged(event.target.checked)} /><span>I reviewed affected assignments and migration guidance.</span></label></div>}
		  <div className="builder-magic-note"><ShieldCheck /><span><strong>No silent migration.</strong> Deprecation changes recommendations and warnings; it never rewrites an existing scoped pin.</span></div>
        </div>
      </Dialog>

      <Dialog
        open={addSourceOpen}
        onClose={closeSourceDialog}
        title="Add knowledge source"
        description="Add a URL-backed source or upload one text document. Every source starts private and draft for review before publication."
        actions={<><Button outline disabled={sourceBusy} onClick={() => closeSourceDialog(false)}>Cancel</Button><Button color="indigo" disabled={sourceBusy || !sourceName.trim() || (sourceKind === "upload" ? !sourceFile || Boolean(sourceFileError) : !sourceLocation.trim())} onClick={createSource}>{sourceBusy ? "Adding…" : sourceKind === "upload" ? "Upload source" : "Add source"}</Button></>}
      >
        <div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Developer documentation" /></label><label className="auth-field"><span>Type</span><select value={sourceKind} onChange={(event) => selectSourceKind(event.target.value as SourceKind)}><option value="website">Website</option><option value="openapi">OpenAPI</option><option value="git">Git repository</option><option value="upload">Upload a file</option></select></label>{sourceKind === "upload" ? <label className="auth-field"><span>File</span><input ref={sourceFileInput} type="file" accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml,text/plain,text/markdown,text/html,application/json,application/yaml,text/yaml" aria-invalid={Boolean(sourceFileError)} aria-describedby={`source-upload-guidance${sourceFileError ? " source-upload-error" : ""}`} onChange={(event) => selectSourceFile(event.target.files?.[0] ?? null)} /><small id="source-upload-guidance">UTF-8 .md, .mdx, .txt, .html, .htm, .json, .yaml, or .yml; up to 5 MB in this setup. Content is treated as untrusted text, and embedded scripts are never executed.</small>{sourceFileError && <small id="source-upload-error" className="source-upload-error" role="alert">{sourceFileError}</small>}</label> : <label className="auth-field"><span>Location</span><input type="url" value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder={sourceKind === "git" ? "https://github.com/vendor/docs" : sourceKind === "openapi" ? "https://api.example.com/openapi.yaml" : "https://docs.example.com"} /></label>}<div className="private-default-note"><LockKeyhole />Private and draft by default. Making a reviewed source public is a separate guarded action.</div></div>
      </Dialog>

      <Dialog
        open={Boolean(sourceReview)}
		onClose={(open) => { if (!open && !sourceReviewBusy) closeSourceReview(); }}
        title={`Review ${sourceReview?.source.name ?? "documentation"}`}
		description={sourceReviewAttachIntegrationID ? "Approve the exact crawl generation. DokoSoko will publish the selected immutable pages, create or reuse a reviewed documentation set, and pin its exact revision to this API." : "Approve the exact completed crawl generation and only the immutable pages that should be available to APIs."}
		actions={<><Button outline disabled={sourceReviewBusy} onClick={closeSourceReview}>Cancel</Button><Button color="indigo" disabled={sourceReviewBusy || Boolean(sourceReview?.publication) || Boolean(sourceReview?.source.quarantined) || !sourceReviewAcknowledged || sourceReviewSelection.length === 0} onClick={confirmSourcePublication}>{sourceReviewBusy ? "Publishing…" : sourceReviewAttachIntegrationID ? "Publish & attach" : "Publish reviewed generation"}</Button></>}
      >
        {sourceReview && <div className="mcp-import-review">
		  {sourceReviewAttachIntegrationID && <div className="private-default-note"><LockKeyhole />Only the reviewed publication is attached, and the API receives a pin to its exact immutable resource-set revision.</div>}
          <div className="import-summary"><Badge color={sourceReview.publication ? "green" : "amber"}>{sourceReview.publication ? `Published r${sourceReview.publication.revision}` : "Needs review"}</Badge><code>{sourceReview.crawl_job.id}</code><span>{sourceReview.documents.length} fetched · {sourceReview.crawl_job.changed_count} changed</span></div>
          <div className="catalog-list">{sourceReview.documents.map((document) => {
            const safe = (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0;
            const selected = sourceReviewSelection.includes(document.id);
            return <label className="catalog-tool" key={document.id}><input type="checkbox" disabled={!safe || Boolean(sourceReview.publication)} checked={selected} onChange={(event) => setSourceReviewSelection((items) => event.target.checked ? [...items, document.id] : items.filter((id) => id !== document.id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{document.title}</strong><code>{document.canonical_url}</code><small>{document.changed ? "Changed in this generation" : "Unchanged snapshot reused"} · trust {document.trust_level}/100</small>{document.injection_indicators.length > 0 && <small>Classifier indicators: {document.injection_indicators.join(", ")}</small>}</span><Badge color={safe ? "zinc" : "red"}>{safe ? document.state : "quarantined"}</Badge></label>;
          })}</div>
          {!sourceReview.publication && <Confirmation checked={sourceReviewAcknowledged} onChange={setSourceReviewAcknowledged}>I reviewed generation {sourceReview.crawl_job.id}, its changed and unchanged pages, and the exact {sourceReviewSelection.length} selected document{sourceReviewSelection.length === 1 ? "" : "s"}.</Confirmation>}
          <div className="private-default-note"><ShieldCheck />Publishing creates an immutable source-publication ID and content hash. Later crawls require a new review; failed or partial generations cannot replace this one.</div>
        </div>}
      </Dialog>

      <Dialog
        open={mcpConnectionOpen}
        onClose={setMCPConnectionOpen}
        title="Connect third-party MCP"
        description="Register one fixed upstream, inspect its complete catalog, then explicitly select the tools DokoSoko may expose."
        actions={<><Button outline onClick={() => setMCPConnectionOpen(false)}>Cancel</Button><Button color="indigo" disabled={mcpBusy || !mcpConnectionReady} onClick={createMCPConnection}>{mcpBusy ? "Inspecting…" : "Connect & inspect"}</Button></>}
      >
        <div className="auth-form compact-form">
          <a className="protocol-policy" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><ShieldCheck /><span><strong>Stateless MCPv2 Only</strong><small>Protocol revision 2026-07-28 · no logical live sessions</small></span><ExternalLink /></a>
          <div className="two-fields"><label className="auth-field"><span>Connection name</span><input value={mcpName} onChange={(event) => setMCPName(event.target.value)} placeholder="Support operations" /></label><label className="auth-field"><span>Local namespace</span><input value={mcpNamespace} onChange={(event) => setMCPNamespace(event.target.value)} placeholder="support" pattern="[a-z][a-z0-9_]*" /></label></div>
          <label className="auth-field"><span>Fixed HTTPS MCP endpoint</span><input type="url" value={mcpEndpoint} onChange={(event) => setMCPEndpoint(event.target.value)} placeholder="https://mcp.vendor.com/v2" /><small>Default HTTPS port only. Redirects and private-network destinations are denied.</small></label>
          <label className="auth-field"><span>Upstream identity mode</span><select value={mcpAuthMode} onChange={(event) => setMCPAuthMode(event.target.value as APIMCPConnection["auth_mode"])}><option value="delegated_oauth">Delegated OAuth per user</option><option value="service">Service credential</option><option value="none">No upstream credential</option></select></label>
          {mcpAuthMode === "service" && <label className="auth-field"><span>Service bearer credential</span><input type="password" autoComplete="off" value={mcpCredential} onChange={(event) => setMCPCredential(event.target.value)} /><small>Encrypted server-side and never returned to agents or browsers.</small></label>}
          {mcpAuthMode === "delegated_oauth" && <><div className="two-fields"><label className="auth-field"><span>OAuth client ID</span><input value={mcpOAuthClientID} onChange={(event) => setMCPOAuthClientID(event.target.value)} /></label><label className="auth-field"><span>OAuth client secret</span><input type="password" autoComplete="off" value={mcpOAuthClientSecret} onChange={(event) => setMCPOAuthClientSecret(event.target.value)} /></label></div><label className="auth-field"><span>OAuth issuer</span><input type="url" value={mcpOAuthIssuer} onChange={(event) => setMCPOAuthIssuer(event.target.value)} placeholder="https://identity.vendor.com" /><small>Pinned for RFC 9207 issuer validation before code redemption.</small></label><label className="auth-field"><span>Authorization URL</span><input type="url" value={mcpAuthorizationURL} onChange={(event) => setMCPAuthorizationURL(event.target.value)} placeholder="https://identity.vendor.com/oauth/authorize" /></label><label className="auth-field"><span>Token URL</span><input type="url" value={mcpTokenURL} onChange={(event) => setMCPTokenURL(event.target.value)} placeholder="https://identity.vendor.com/oauth/token" /></label><label className="auth-field"><span>OAuth scopes</span><input value={mcpScopes} onChange={(event) => setMCPScopes(event.target.value)} placeholder="incidents.read incidents.write" /></label></>}
          <div className="private-default-note"><Users />The inbound DokoSoko token is never forwarded. Delegated mode stores a separate encrypted upstream grant bound to the authenticated DokoSoko subject.</div>
        </div>
      </Dialog>

      <Dialog
        open={mcpImportOpen}
        onClose={setMCPImportOpen}
        title={`Review tools from ${mcpCatalog?.connection.name ?? "upstream MCP"}`}
        description="Upstream names, descriptions, schemas, and annotations are untrusted input. Only checked tools become local drafts."
        actions={<><Button outline onClick={() => setMCPImportOpen(false)}>Cancel</Button><Button color="indigo" disabled={mcpBusy || mcpSelectedTools.length === 0} onClick={importMCPTools}>{mcpBusy ? "Pinning schemas…" : `Import ${mcpSelectedTools.length} draft${mcpSelectedTools.length === 1 ? "" : "s"}`}</Button></>}
      >
        <div className="mcp-import-review">
          <div className="import-summary"><Badge color="violet">Stateless MCPv2 Only</Badge><code>{mcpCatalog?.connection.namespace}.*</code><span>{mcpCatalog?.tools.length ?? 0} catalog tools</span></div>
          {Object.keys(mcpImportFailures).length > 0 && <div className="capability-unavailable mcp-import-failures" role="alert"><TriangleAlert /><span><strong>Some tools were rejected.</strong><small>Close this dialog, resolve the local conflict, then inspect the connection again.</small>{Object.entries(mcpImportFailures).map(([name, reason]) => <span key={name}><code>{name}</code><small>{reason}</small></span>)}</span></div>}
          <div className="catalog-list">{mcpCatalog?.tools.map((tool) => <label className="catalog-tool" key={tool.name}><input type="checkbox" checked={mcpSelectedTools.includes(tool.name)} onChange={(event) => setMCPSelectedTools((items) => event.target.checked ? [...items, tool.name] : items.filter((name) => name !== tool.name))} /><span className="check-box">{mcpSelectedTools.includes(tool.name) && <Check />}</span><span><strong>{tool.title || tool.name}</strong><code>{tool.name}</code><small>{tool.description || "No upstream description"}</small></span><Badge color="zinc">{tool.schema_hash.slice(0, 12)}</Badge></label>)}</div>
          <label className="auth-field"><span>Required grants</span><input value={mcpGrants} onChange={(event) => setMCPGrants(event.target.value)} placeholder="support.write, developer.pro" /><small>Evaluated before every upstream call. Access-evaluation failures deny execution.</small></label>
          <Switch checked={mcpConfirmationRequired} onChange={setMCPConfirmationRequired} label="Require user confirmation before execution" />
          <div className="private-default-note"><LockKeyhole />Import pins each schema hash. Published tools fail closed if a later catalog inspection detects drift.</div>
        </div>
      </Dialog>

      <Dialog
	    open={runOpen}
	    onClose={setRunOpen}
	    title="Start API run"
	    description="Track an intended developer outcome through a deterministic validation result."
	    actions={<><Button outline onClick={() => setRunOpen(false)}>Cancel</Button><Button color="indigo" disabled={runBusy || !runEnvironmentID || !runOutcome.trim()} onClick={startIntegrationRun}>{runBusy ? "Starting…" : "Start run"}</Button></>}
	  >
	    <div className="auth-form compact-form"><label className="auth-field"><span>Environment</span><select value={runEnvironmentID} onChange={(event) => setRunEnvironmentID(event.target.value)}>{environments.map((environment) => <option value={environment.id} key={environment.id}>{environment.name}</option>)}</select></label><label className="auth-field"><span>Requested outcome</span><textarea maxLength={500} value={runOutcome} onChange={(event) => setRunOutcome(event.target.value)} placeholder="Install the SDK, authenticate, and verify a first API request" /></label><div className="private-default-note"><ShieldCheck />The run records outcome and validation state. Secret inputs, raw prompts, and tool payloads are excluded.</div></div>
	  </Dialog>

      <Dialog
        open={pendingMCPEnable}
        onClose={setPendingMCPEnable}
        title="Enable authentication-free Public MCP?"
        description="Anyone with the endpoint can query resources that you have explicitly marked public."
        actions={<><Button outline onClick={() => setPendingMCPEnable(false)}>Cancel</Button><Button color="red" disabled={!acknowledged} onClick={confirmMCPEnable}>Enable Public MCP</Button></>}
      >
        <WarningContent>
          <p><strong>{publicResourceCount} published {publicResourceCount === 1 ? "resource is" : "resources are"} currently marked public.</strong> Private sources, API tools, provider resources, credentials, identities, and customer access data remain excluded.</p>
          <p>Anonymous requests are rate-limited and logged as aggregate security events. You can turn this endpoint off immediately.</p>
          <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand Public MCP is authentication-less and exposes public resources anonymously.</Confirmation>
        </WarningContent>
      </Dialog>

	  <Dialog
		open={Boolean(reportDetail)}
		onClose={(open) => { if (!open) setReportDetail(null); }}
		title={reportDetail?.kind === "bug" ? "Bug report" : "Feedback submission"}
		description="Decrypted on demand for this authenticated administrative review."
		actions={<Button color="indigo" onClick={() => setReportDetail(null)}>Close</Button>}
	  >
		{reportDetailBusy ? <div className="empty-row">Decrypting submission…</div> : reportDetail && <div className="report-detail"><div className="report-detail-meta"><span><small>Status</small><Badge color={reportDetail.state === "delivered" ? "green" : reportDetail.state === "failed" ? "red" : reportDetail.state === "held" ? "amber" : "blue"}>{reportDetail.state}</Badge></span><span><small>Compatibility snapshot</small><code>{reportDetail.trusted_context.product_version || "Unversioned"}</code></span><span><small>Created</small><strong>{new Date(reportDetail.created_at).toLocaleString()}</strong></span></div><pre>{JSON.stringify(reportDetail.content ?? { summary: reportDetail.summary }, null, 2)}</pre>{reportDetail.external_url && <a className="report-external-detail" href={reportDetail.external_url} target="_blank" rel="noreferrer"><ExternalLink />Open {reportDetail.external_id || "external ticket"}</a>}</div>}
	  </Dialog>

      <Dialog
        open={rootOpen}
        onClose={setRootOpen}
        title="Add root administrator"
        description="Every root administrator has a unique strong password, TOTP enrollment, recovery codes, and independently revocable sessions."
        actions={rootRecoveryCodes.length ? <Button color="indigo" onClick={() => { setRootOpen(false); setRootRecoveryCodes([]); setRootEmail(""); setRootDisplayName(""); setRootPassword(""); }}>I saved the recovery codes</Button> : rootEnrollment ? <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || rootCode.length !== 6} onClick={completeRootUser}>{rootBusy ? "Verifying…" : "Create root"}</Button></> : <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || !rootEmail.trim() || !rootDisplayName.trim() || rootPassword.length < 14} onClick={beginRootUser}>{rootBusy ? "Preparing…" : "Continue to MFA"}</Button></>}
      >
        {rootRecoveryCodes.length ? <div className="auth-form compact-form"><div className="private-default-note"><ShieldCheck />These one-time recovery codes are shown once. Store them in a secure password manager.</div><div className="recovery-grid">{rootRecoveryCodes.map((code) => <code key={code}>{code}</code>)}</div></div> : rootEnrollment ? <div className="auth-form compact-form"><label className="auth-field"><span>Authenticator secret</span><input readOnly value={rootEnrollment.totp_secret} onFocus={(event) => event.currentTarget.select()} /><small>Add this secret to the new administrator&apos;s authenticator. Enrollment expires in 15 minutes.</small></label><label className="auth-field"><span>6-digit verification code</span><input inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={rootCode} onChange={(event) => setRootCode(event.target.value.replace(/\D/g, ""))} /></label></div> : <div className="auth-form compact-form"><label className="auth-field"><span>Email</span><input type="email" value={rootEmail} onChange={(event) => setRootEmail(event.target.value)} /></label><label className="auth-field"><span>Display name</span><input value={rootDisplayName} onChange={(event) => setRootDisplayName(event.target.value)} /></label><label className="auth-field"><span>Initial password</span><input type="password" autoComplete="new-password" value={rootPassword} onChange={(event) => setRootPassword(event.target.value)} /><small>At least 14 characters with upper/lower-case and a number.</small></label></div>}
      </Dialog>

      <Dialog
        open={llmOpen}
        onClose={setLLMOpen}
        title={`Configure ${aiWorkloads.find((workload) => workload.role === llmRole)?.name ?? llmRole}`}
        description={aiWorkloads.find((workload) => workload.role === llmRole)?.description ?? "Choose the provider and model for this workload."}
        actions={<><Button outline onClick={() => setLLMOpen(false)}>Cancel</Button><Button color="indigo" disabled={llmBusy || !llmConnectionID || !llmModel.trim()} onClick={saveLLMProfile}>{llmBusy ? "Saving…" : "Save workload"}</Button></>}
      >
        <div className="auth-form compact-form ai-model-form">
          <div className="ai-dialog-workload"><span className="settings-icon">{(() => { const Icon = aiWorkloads.find((workload) => workload.role === llmRole)?.icon ?? Bot; return <Icon />; })()}</span><span><small>Workload</small><strong>{aiWorkloads.find((workload) => workload.role === llmRole)?.name ?? llmRole}</strong></span><Switch checked={llmEnabled} onChange={setLLMEnabled} label="Enabled" /></div>
          <div className="two-fields"><label className="auth-field"><span>Provider connection</span><Select name="llm-connection" value={llmConnectionID} onChange={(event) => changeLLMConnection(event.target.value)}><option value="">Choose a connection</option>{aiConnections.filter((connection) => connection.enabled && !connection.is_backup).map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider)}{connection.managed_by === "environment" ? " · environment" : ""}</option>)}</Select></label><label className="auth-field"><span>Model</span>{(() => { const provider = aiConnections.find((connection) => connection.id === llmConnectionID)?.provider; return provider && provider !== "openai-compatible" ? <Select name="llm-model" value={llmModel} onChange={(event) => setLLMModel(event.target.value)}>{aiModelOptions[provider].map((model) => <option key={model} value={model}>{model}</option>)}</Select> : <Input name="llm-model" autoComplete="off" value={llmModel} onChange={(event) => setLLMModel(event.target.value)} placeholder="Provider model ID" />; })()}</label></div>
          {aiConnections.length === 0 && <div className="private-default-note"><KeyRound />Connect one provider before enabling a workload.</div>}
	          <details className="advanced-details ai-model-advanced"><summary>Limits and budget</summary><div className="ai-model-advanced-body"><div className="two-fields"><label className="auth-field" htmlFor="ai-max-input-tokens"><span>Max input tokens</span><Input id="ai-max-input-tokens" type="number" min={256} max={1000000} value={llmInputTokens} onChange={(event) => setLLMInputTokens(event.target.value)} /></label><label className="auth-field" htmlFor="ai-max-output-tokens"><span>Max output tokens</span><Input id="ai-max-output-tokens" type="number" min={1} max={32768} value={llmOutputTokens} onChange={(event) => setLLMOutputTokens(event.target.value)} /></label></div><label className="auth-field" htmlFor="ai-daily-token-budget"><span>Daily token budget</span><Input id="ai-daily-token-budget" type="number" min={0} max={10000000000} value={llmDailyBudget} onChange={(event) => setLLMDailyBudget(event.target.value)} /><small>Set to 0 for no daily cap. Budget reservations are atomic across concurrent jobs.</small></label></div></details>
          <div className="ai-dialog-safeguards"><span><ShieldCheck />Untrusted context</span><span><LockKeyhole />No authorization</span><span><TerminalSquare />No tool calls</span><span><BookOpen />Citations required</span></div>
        </div>
      </Dialog>

      <Dialog
        open={providerPickerOpen}
        onClose={setProviderPickerOpen}
        title="Add provider"
        description="Choose the provider that will own this encrypted credential. You can connect each provider once."
        actions={<Button outline onClick={() => setProviderPickerOpen(false)}>Cancel</Button>}
      >
        <div className="ai-provider-picker">
          {aiProviders.map((provider) => {
            const connected = aiConnections.some((connection) => connection.provider === provider.id);
            return <button type="button" key={provider.id} aria-label={`${provider.name}${connected ? " (connected)" : ""}`} onClick={() => openAIConnection(provider.id)}><AIProviderLogo provider={provider.id} /><strong>{provider.name}</strong><ChevronRight /></button>;
          })}
        </div>
      </Dialog>

      <Dialog
        open={providerOpen}
        onClose={setProviderOpen}
        title={`Connect ${aiProviderLabel(llmProvider)}`}
        description="One provider connection owns one credential. Workloads reuse it without copying secrets."
	        actions={<><Button outline onClick={() => setProviderOpen(false)}>Cancel</Button><Button color="indigo" disabled={providerBusy || !llmEndpoint.trim() || (providerIsBackup && (!providerBackupAnalysisModel.trim() || !providerBackupAssistantModel.trim())) || (providerEnabled && !llmCredential.trim() && !aiConnections.some((connection) => connection.provider === llmProvider)) || aiConnections.some((connection) => connection.provider === llmProvider && connection.managed_by === "environment")} onClick={saveAIConnection}>{providerBusy ? "Saving…" : "Save connection"}</Button></>}
      >
        <div className="auth-form compact-form ai-model-form">
          <div className="ai-dialog-workload"><AIProviderLogo provider={llmProvider} /><span><small>Provider</small><strong>{aiProviderLabel(llmProvider)}</strong></span><Switch checked={providerEnabled} onChange={setProviderEnabled} label="Enabled" /></div>
          {aiConnections.some((connection) => connection.provider === llmProvider && connection.managed_by === "environment") ? <div className="private-default-note"><TerminalSquare />This connection is managed by DOKOSOKO_AI_* environment variables. Change it in deployment configuration and restart DokoSoko.</div> : <>
            <label className="auth-field"><span>Provider origin</span><Input name="ai-provider-endpoint" type="url" autoComplete="off" readOnly={llmProvider !== "openai-compatible"} value={llmEndpoint} onChange={(event) => setLLMEndpoint(event.target.value)} placeholder="https://api.provider.com" /><small>{llmProvider === "openai-compatible" ? "A fixed public HTTPS origin. Private-network destinations, redirects, paths, and non-default ports are rejected." : "The native provider origin is fixed by DokoSoko."}</small></label>
	            <label className="auth-field" htmlFor="ai-provider-credential"><span>API credential</span><Input id="ai-provider-credential" name="ai-provider-credential" type="password" autoComplete="new-password" value={llmCredential} onChange={(event) => setLLMCredential(event.target.value)} placeholder={aiConnections.some((connection) => connection.provider === llmProvider) ? "Leave blank to keep the stored credential" : "Required before enabling"} /><small>Encrypted at rest, redacted from every response, and shared only with the selected provider.</small></label>
            <div className="ai-backup-control"><span><strong>Backup provider</strong><small>Retry this provider once when the selected provider times out, is rate-limited, or is unavailable.</small></span><Switch checked={providerIsBackup} onChange={setProviderIsBackup} label="Use as backup provider" /></div>
            {providerIsBackup && <div className="two-fields"><label className="auth-field"><span>Analysis backup model</span>{llmProvider === "openai-compatible" ? <Input value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)} placeholder="Provider model ID" /> : <Select value={providerBackupAnalysisModel} onChange={(event) => setProviderBackupAnalysisModel(event.target.value)}>{aiModelOptions[llmProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label><label className="auth-field"><span>Assistant backup model</span>{llmProvider === "openai-compatible" ? <Input value={providerBackupAssistantModel} onChange={(event) => setProviderBackupAssistantModel(event.target.value)} placeholder="Provider model ID" /> : <Select value={providerBackupAssistantModel} onChange={(event) => setProviderBackupAssistantModel(event.target.value)}>{aiModelOptions[llmProvider].map((model) => <option key={model} value={model}>{model}</option>)}</Select>}</label></div>}
          </>}
        </div>
      </Dialog>

      {widgetsEnabled && <WidgetPreviewLauncher widgets={widgets} currentWidgetID={consoleRoute.kind === "entity" && consoleRoute.entity === "widget" ? consoleRoute.uid : undefined} onOpenWidgets={() => navigateToSection("widgets")} />}
      {toast && <div className="toast" role="status"><Check />{toast}</div>}
    </div>
    </Suspense>
  );
}
