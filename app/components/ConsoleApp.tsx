"use client";

import {
  Activity,
  ArrowLeft,
  BookOpen,
  Bot,
  Check,
  ChevronRight,
  Eye,
  ExternalLink,
  KeyRound,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  Radio,
  RefreshCw,
  Search,
  Settings,
  Share2,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  TriangleAlert,
  Users,
  Wrench,
  XCircle,
} from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIAIProviderConnection, APIAIProviderUsage, APIAIWorkloadProfile, APIAnalytics, APIAuditEvent, APIBackendConnection, APICustomerAccount, APIDeployment, APIEnvironment, APIError, APIGrantDefinition, APIIdentity, APIIntegration, APIIntegrationAnalysis, APIIntegrationRun, APIMCPCatalog, APIMCPConnection, APIProduct, APIProductBuild, APIProductBuildInput, APIProductDefinition, APIProductInstallation, APIProductVersion, APIProductVersionImpact, APIProductVersionPin, APIProductVersionPinHistory, APIRecipe, APIRecipeReference, APIResourceSet, APISourcePublication, APISourceReview, APISupportRoute, APISupportSubmission, APITool, APIToolBuilderProposal, APIToolTestAnalysisProposal, APIUser, APIWidget, APIWidgetInput, Distribution, SetupEnrollment, api } from "../lib/api";
import { ConsoleRoute, Section, SettingsTab, entityPath, parseConsolePath, routeForSection, sectionPath, settingsPath, toolBuilderPath } from "../lib/console-routes";
import type { ConsoleFixtures } from "../dev/console-fixtures";
import { Badge, Button, Dialog, Switch } from "./core/control";
import { Input, Select } from "./core";
import { ViewStack } from "./core/layout";
import { ThemeToggle } from "./ThemeToggle";
import { OIDCIdentitySetup } from "./OIDCIdentitySetup";
import { ToolBuilderView } from "./ToolBuilderView";
import { WidgetPreviewLauncher } from "./WidgetPreviewLauncher";
import { IntegrationToolBuilderRoute } from "./integrations/IntegrationToolBuilderRoute";
import { AIWorkload, Confirmation, ConsoleLink, CopyButton, DocumentationAttachmentResult, EntityDetail, EntityLink, Source, WarningContent, aiModelDefaults, aiModelOptions, aiProviderLabel, aiProviderOrigin, aiProviders, aiWorkloads, analysisMatchesIntegration, buildAgentSetupEmbedHTML, integrationIncludesSourcePublication, manifestIncludesSourcePublication, recipeMatchesIntegration, sourcePublicationManifestEntry } from "./console/shared";

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

type NavigationGroup = "apis" | "identity" | "tools" | "recipes" | "agent-access" | "activity";
type SourceKind = "website" | "openapi" | "git" | "upload";

const sourceUploadMaxBytes = 5_000_000;
const sourceUploadExtensions = new Set([".md", ".mdx", ".txt", ".html", ".htm", ".json", ".yaml", ".yml"]);
function sourceUploadValidationError(file: File) {
  const extension = file.name.slice(file.name.lastIndexOf(".")).toLowerCase();
  if (!sourceUploadExtensions.has(extension)) return "Choose a Markdown, text, HTML, JSON, or YAML file.";
  if (file.size > sourceUploadMaxBytes) return "The selected file is larger than the 5 MB limit for this setup.";
  if (file.size === 0) return "The selected file is empty.";
  return "";
}

type PendingPublication = {
  kind: "source";
  id: string;
  name: string;
  detail: string;
};

const navigation: Array<{
  id: NavigationGroup;
  label: string;
  icon: typeof LayoutDashboard;
  defaultSection: Section;
  sections: Array<{ id: Section; label: string }>;
}> = [
  { id: "apis", label: "APIs", icon: Sparkles, defaultSection: "product", sections: [{ id: "product", label: "APIs" }] },
  { id: "identity", label: "Identity", icon: Users, defaultSection: "identity", sections: [{ id: "identity", label: "Customer sign-in" }] },
  { id: "tools", label: "Tools", icon: Wrench, defaultSection: "tools", sections: [{ id: "tools", label: "Catalog" }, { id: "connections", label: "Connections" }] },
  { id: "recipes", label: "Recipes", icon: BookOpen, defaultSection: "recipes", sections: [{ id: "recipes", label: "Recipes" }] },
  { id: "agent-access", label: "Agent access", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", label: "Agent access" }, { id: "widgets", label: "Widgets" }] },
  { id: "activity", label: "Activity", icon: Activity, defaultSection: "runs", sections: [{ id: "runs", label: "Activity" }] },
];

function deploymentAsLegacyProduct(value: APIDeployment): APIProduct {
  return { id: value.id, organisation_id: value.organisation_id, name: value.name, slug: value.slug, description: value.description, default_version_policy: value.default_release_policy, catalog_revision: value.catalog_revision, require_promotion_approval: value.require_promotion_approval, public_mcp_enabled: value.public_mcp_enabled, revision: value.revision };
}

function parseAvailableConsolePath(path: string, widgetsEnabled: boolean): ConsoleRoute {
  const route = parseConsolePath(path);
  const isWidgetRoute = (route.kind === "section" && route.section === "widgets") || (route.kind === "entity" && route.entity === "widget");
  return !widgetsEnabled && isWidgetRoute ? { kind: "not-found", section: "product", path: route.path } : route;
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
	const [integrations, setIntegrations] = useState<APIIntegration[]>([]);
	const [widgets, setWidgets] = useState<APIWidget[]>([]);
	const [resourceSets, setResourceSets] = useState<APIResourceSet[]>([]);
	const [accessDefinitions, setAccessDefinitions] = useState<APIAccessDefinition[]>([]);
	const [accessConnections, setAccessConnections] = useState<APIAccessConnection[]>([]);
	const [backendConnections, setBackendConnections] = useState<APIBackendConnection[]>([]);
	const [accessInstances, setAccessInstances] = useState<APIAccessInstance[]>([]);
	const [accessCredentials, setAccessCredentials] = useState<APIAccessCredential[]>([]);
  const [supportRoutes, setSupportRoutes] = useState<APISupportRoute[]>([]);
  const [consoleRoute, setConsoleRoute] = useState<ConsoleRoute>(() => routeForSection("product"));
  const consoleRouteRef = useRef(consoleRoute);
  const toolBuilderDirtyRef = useRef(false);
  const handleToolBuilderDirtyChange = useCallback((dirty: boolean) => {
    toolBuilderDirtyRef.current = dirty;
  }, []);
  const section = consoleRoute.section;
  const settingsTab: SettingsTab = consoleRoute.kind === "section" && consoleRoute.section === "settings" ? consoleRoute.settingsTab ?? "overview" : "overview";
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtures?.definition ?? null);
  const [latestProductBuild, setLatestProductBuild] = useState<APIProductBuild | null>(fixtures?.productBuild ?? null);
  const [productBuilderOpen, setProductBuilderOpen] = useState(false);
  const [productBuildReviewOpen, setProductBuildReviewOpen] = useState(false);
  const [productBuilderBusy, setProductBuilderBusy] = useState(false);
  const [productBuilderInputs, setProductBuilderInputs] = useState("");
  const [sources, setSources] = useState<Source[]>(fixtures?.sources ?? []);
  const [tools, setTools] = useState<APITool[]>(fixtures?.tools ?? []);
  const [grantDefinitions, setGrantDefinitions] = useState<APIGrantDefinition[]>([]);
  const [toolBuilderSelection, setToolBuilderSelection] = useState<{ uid: string; tool: APITool | null; failed: boolean } | null>(null);
  const [toolBuilderLoadAttempt, setToolBuilderLoadAttempt] = useState(0);
  const [toolBuilderSeed, setToolBuilderSeed] = useState<{ toolID: string; revision: number; proposal: APIToolBuilderProposal } | null>(null);
  const [mcpConnections, setMCPConnections] = useState<APIMCPConnection[]>(fixtures?.mcpConnections ?? []);
  const [mcpConnectionOpen, setMCPConnectionOpen] = useState(false);
  const [mcpImportOpen, setMCPImportOpen] = useState(false);
  const [mcpBusy, setMCPBusy] = useState(false);
  const [mcpCatalog, setMCPCatalog] = useState<APIMCPCatalog | null>(null);
  const [mcpSelectedTools, setMCPSelectedTools] = useState<string[]>([]);
  const [mcpImportFailures, setMCPImportFailures] = useState<Record<string, string>>({});
  const [mcpName, setMCPName] = useState("");
  const [mcpNamespace, setMCPNamespace] = useState("");
  const [mcpEndpoint, setMCPEndpoint] = useState("");
  const [mcpAuthMode, setMCPAuthMode] = useState<APIMCPConnection["auth_mode"]>("delegated_oauth");
  const [mcpCredential, setMCPCredential] = useState("");
  const [mcpOAuthClientID, setMCPOAuthClientID] = useState("");
  const [mcpOAuthClientSecret, setMCPOAuthClientSecret] = useState("");
  const [mcpOAuthIssuer, setMCPOAuthIssuer] = useState("");
  const [mcpAuthorizationURL, setMCPAuthorizationURL] = useState("");
  const [mcpTokenURL, setMCPTokenURL] = useState("");
  const [mcpScopes, setMCPScopes] = useState("");
  const [mcpGrants, setMCPGrants] = useState("");
  const [mcpConfirmationRequired, setMCPConfirmationRequired] = useState(true);
  const [publicMCPEnabled, setPublicMCPEnabled] = useState(false);
  const [distribution, setDistribution] = useState<Distribution | null>(null);
  const [widgetCreateOpen, setWidgetCreateOpen] = useState(false);
  const [widgetBusy, setWidgetBusy] = useState(false);
  const [widgetName, setWidgetName] = useState("Customer assistant");
  const [widgetOrigins, setWidgetOrigins] = useState("http://localhost:3000");
  const [widgetIntegrationIDs, setWidgetIntegrationIDs] = useState<string[]>([]);
  const [widgetCredential, setWidgetCredential] = useState<{ widgetID: string; secret: string } | null>(null);
  const [pendingPublication, setPendingPublication] = useState<PendingPublication | null>(null);
  const [pendingMCPEnable, setPendingMCPEnable] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [resourceFilter, setResourceFilter] = useState<"all" | "public" | "private">("all");
  const [productRevision, setProductRevision] = useState(1);
  const apiConnected = !fixturePreview;
  const [workspaceLoading, setWorkspaceLoading] = useState(!fixturePreview);
  const [workspaceLoadProblems, setWorkspaceLoadProblems] = useState<string[]>([]);
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [sourceName, setSourceName] = useState("");
  const [sourceKind, setSourceKind] = useState<SourceKind>("website");
  const [sourceLocation, setSourceLocation] = useState("");
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [sourceFileError, setSourceFileError] = useState("");
  const sourceFileInput = useRef<HTMLInputElement>(null);
  const [sourceBusy, setSourceBusy] = useState(false);
  const [sourceReview, setSourceReview] = useState<APISourceReview | null>(null);
  const [sourceReviewSelection, setSourceReviewSelection] = useState<string[]>([]);
  const [sourceReviewAcknowledged, setSourceReviewAcknowledged] = useState(false);
  const [sourceReviewBusy, setSourceReviewBusy] = useState(false);
	const [sourceReviewAttachIntegrationID, setSourceReviewAttachIntegrationID] = useState("");
	  const [analytics, setAnalytics] = useState<APIAnalytics | null>(null);
		  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
		  const [identityLoading, setIdentityLoading] = useState(true);
		  const [identityLoadError, setIdentityLoadError] = useState("");
	  const [reportSubmissions, setReportSubmissions] = useState<APISupportSubmission[]>([]);
	  const [reportDetail, setReportDetail] = useState<APISupportSubmission | null>(null);
	  const [reportDetailBusy, setReportDetailBusy] = useState(false);
	  const [rootUsers, setRootUsers] = useState<APIUser[]>(currentUser ? [currentUser] : []);
	  const [rootOpen, setRootOpen] = useState(false);
	  const [rootBusy, setRootBusy] = useState(false);
	  const [rootEmail, setRootEmail] = useState("");
	  const [rootDisplayName, setRootDisplayName] = useState("");
	  const [rootPassword, setRootPassword] = useState("");
	  const [rootCode, setRootCode] = useState("");
	  const [rootEnrollment, setRootEnrollment] = useState<SetupEnrollment | null>(null);
	  const [rootRecoveryCodes, setRootRecoveryCodes] = useState<string[]>([]);
	  const [aiConnections, setAIConnections] = useState<APIAIProviderConnection[]>([]);
	  const [aiProfiles, setAIProfiles] = useState<APIAIWorkloadProfile[]>([]);
	  const [analyses, setAnalyses] = useState<APIIntegrationAnalysis[]>([]);
	  const [recipes, setRecipes] = useState<APIRecipe[]>([]);
	  const [aiProviderUsage, setAIProviderUsage] = useState<APIAIProviderUsage[]>([]);
	  const [recipeBusy, setRecipeBusy] = useState(false);
	  const [llmOpen, setLLMOpen] = useState(false);
	  const [llmBusy, setLLMBusy] = useState(false);
	  const [llmRole, setLLMRole] = useState<AIWorkload>("analysis");
	  const [llmConnectionID, setLLMConnectionID] = useState("");
	  const [providerOpen, setProviderOpen] = useState(false);
	  const [providerPickerOpen, setProviderPickerOpen] = useState(false);
	  const [providerBusy, setProviderBusy] = useState(false);
	  const [providerEnabled, setProviderEnabled] = useState(true);
	  const [providerIsBackup, setProviderIsBackup] = useState(false);
	  const [providerBackupAnalysisModel, setProviderBackupAnalysisModel] = useState(aiModelDefaults.openai.analysis);
	  const [providerBackupAssistantModel, setProviderBackupAssistantModel] = useState(aiModelDefaults.openai.assistant);
	  const [llmProvider, setLLMProvider] = useState<APIAIProviderConnection["provider"]>("openai");
	  const [llmEndpoint, setLLMEndpoint] = useState(aiProviderOrigin("openai"));
	  const [llmModel, setLLMModel] = useState(aiModelDefaults.openai.analysis);
	  const [llmCredential, setLLMCredential] = useState("");
	  const [llmInputTokens, setLLMInputTokens] = useState("8192");
	  const [llmOutputTokens, setLLMOutputTokens] = useState("1024");
	  const [llmDailyBudget, setLLMDailyBudget] = useState("1000000");
	  const [llmEnabled, setLLMEnabled] = useState(false);
	  const [environments, setEnvironments] = useState<APIEnvironment[]>(fixtures ? [fixtures.environment] : []);
	  const [integrationRuns, setIntegrationRuns] = useState<APIIntegrationRun[]>([]);
	  const [auditEvents, setAuditEvents] = useState<APIAuditEvent[]>([]);
	  const [runOpen, setRunOpen] = useState(false);
	  const [runBusy, setRunBusy] = useState(false);
	  const [runEnvironmentID, setRunEnvironmentID] = useState(fixtures?.environment.id ?? "");
	  const [runOutcome, setRunOutcome] = useState("");
	  const [productCatalogOpen, setProductCatalogOpen] = useState(false);
	  const [productCatalogBusy, setProductCatalogBusy] = useState(false);
	  const [productDescription, setProductDescription] = useState(product.description);
	  const [defaultVersionPolicy, setDefaultVersionPolicy] = useState<"latest" | "lts">(product.default_version_policy);
	  const [requirePromotionApproval, setRequirePromotionApproval] = useState(product.require_promotion_approval);
	  const [productVersions, setProductVersions] = useState<APIProductVersion[]>(fixtures?.productVersions ?? []);
	  const [productVersionPins, setProductVersionPins] = useState<APIProductVersionPin[]>(fixtures?.productPins ?? []);
	  const [customerAccountLoad, setCustomerAccountLoad] = useState<{ productID: string; status: "loading" | "ready" | "unavailable"; items: APICustomerAccount[]; hasMore: boolean }>({ productID: product.id, status: "loading", items: [], hasMore: false });
	  const customerAccounts = customerAccountLoad.productID === product.id ? customerAccountLoad.items : [];
	  const customerAccountsStatus = customerAccountLoad.productID === product.id ? customerAccountLoad.status : "loading";
	  const customerAccountsHaveMore = customerAccountLoad.productID === product.id && customerAccountLoad.hasMore;
	  const [productInstallations, setProductInstallations] = useState<APIProductInstallation[]>(fixtures?.installations ?? []);
	  const [pinHistory, setPinHistory] = useState<APIProductVersionPinHistory[]>([]);
	  const [newProductVersion, setNewProductVersion] = useState("");
	  const [newProductProfile, setNewProductProfile] = useState(fixtures?.definition.profiles[0]?.id ?? "");
	  const [newVersionLatest, setNewVersionLatest] = useState(true);
	  const [newVersionLTS, setNewVersionLTS] = useState(false);
	  const [newVersionStage, setNewVersionStage] = useState<"preview" | "active">("active");
	  const [newVersionRollout, setNewVersionRollout] = useState(100);
	  const [pinScope, setPinScope] = useState<"customer" | "environment" | "installation">("customer");
	  const [pinCustomerID, setPinCustomerID] = useState("");
	  const [pinVersionID, setPinVersionID] = useState(fixtures?.productVersions[0]?.id ?? "");
	  const [pinReason, setPinReason] = useState("");
	  const [versionLifecycleOpen, setVersionLifecycleOpen] = useState(false);
	  const [editingProductVersion, setEditingProductVersion] = useState<APIProductVersion | null>(null);
	  const [lifecycleLatest, setLifecycleLatest] = useState(false);
	  const [lifecycleLTS, setLifecycleLTS] = useState(false);
	  const [lifecycleDeprecated, setLifecycleDeprecated] = useState(false);
	  const [lifecycleMessage, setLifecycleMessage] = useState("");
	  const [lifecycleReplacement, setLifecycleReplacement] = useState("");
	  const [lifecycleSunset, setLifecycleSunset] = useState("");
	  const [lifecycleRollout, setLifecycleRollout] = useState(100);
	  const [lifecycleImpact, setLifecycleImpact] = useState<APIProductVersionImpact | null>(null);
	  const [lifecycleImpactAcknowledged, setLifecycleImpactAcknowledged] = useState(false);
	  const [installationName, setInstallationName] = useState("");
	  const [installationExternalID, setInstallationExternalID] = useState("");
	  const [installationCustomerID, setInstallationCustomerID] = useState("");
	  const [installationEnvironmentID, setInstallationEnvironmentID] = useState(fixtures?.environment.id ?? "");
  const toolBuilderUID = consoleRoute.kind === "tool-builder" ? consoleRoute.uid : undefined;

	  useEffect(() => {
	    let cancelled = false;
	    const accountRequest = fixturePreview ? Promise.resolve({ items: fixtures?.customerAccounts ?? [], has_more: false }) : api.customerAccounts(product.id);
	    accountRequest.then((page) => {
	      if (!cancelled) setCustomerAccountLoad({ productID: product.id, status: "ready", items: page.items, hasMore: page.has_more });
	    }).catch(() => {
	      if (!cancelled) setCustomerAccountLoad({ productID: product.id, status: "unavailable", items: [], hasMore: false });
	    });
	    return () => { cancelled = true; };
	  }, [fixturePreview, fixtures?.customerAccounts, product.id]);

	  useEffect(() => {
    if (fixturePreview) document.documentElement.dataset.preview = "fixtures";
    return () => { delete document.documentElement.dataset.preview; };
  }, [fixturePreview]);

  useEffect(() => {
    consoleRouteRef.current = consoleRoute;
  }, [consoleRoute]);

  useEffect(() => {
    const syncRoute = () => {
      const current = consoleRouteRef.current;
      const next = parseAvailableConsolePath(window.location.pathname, widgetsEnabled);
      if (!confirmToolBuilderNavigation(next.path)) {
        window.history.pushState(null, "", routeURL(current.path));
        return;
      }
      if (current.path !== next.path) toolBuilderDirtyRef.current = false;
      if (next.kind !== "not-found" && window.location.pathname !== next.path) {
        window.history.replaceState(null, "", `${next.path}${window.location.search}${window.location.hash}`);
      }
      if (next.kind !== "tool-builder") setToolBuilderSeed(null);
      consoleRouteRef.current = next;
      setConsoleRoute(next);
    };
    syncRoute();
    window.addEventListener("popstate", syncRoute);
    return () => window.removeEventListener("popstate", syncRoute);
  }, [widgetsEnabled]);

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
	    Promise.all([api.aiConnections(), api.aiProfiles(product.id)]).then(([connections, profiles]) => { if (!cancelled) { setAIConnections(connections); setAIProfiles(profiles); } }).catch((error) => recordLoadProblem("AI configuration", error));
	    Promise.all([api.analyses(product.id), api.recipes(product.id), api.aiUsage(product.id)]).then(([analysisValues, recipeValues, usageValues]) => { if (!cancelled) { setAnalyses(analysisValues); setRecipes(recipeValues); setAIProviderUsage(usageValues.providers); } }).catch((error) => recordLoadProblem("AI content", error));
	    api.productDefinition(product.id).then((value) => { if (!cancelled) setProductDefinition(value); }).catch((error) => { if (!cancelled && error instanceof APIError && error.status === 404) setProductDefinition(null); else recordLoadProblem("Product definition", error); });
	    api.productBuilds(product.id).then((values) => { if (!cancelled) setLatestProductBuild(values[0] ?? null); }).catch((error) => recordLoadProblem("Product builds", error));
	    api.productVersions(product.id).then((values) => { if (!cancelled) { setProductVersions(values); setPinVersionID(values.find((value) => value.is_latest)?.id ?? values[0]?.id ?? ""); } }).catch((error) => recordLoadProblem("Product versions", error));
	    api.productVersionPins(product.id).then((values) => { if (!cancelled) setProductVersionPins(values); }).catch((error) => recordLoadProblem("Version pins", error));
	    Promise.all([api.productInstallations(product.id), api.productVersionPinHistory(product.id)]).then(([installationValues, historyValues]) => { if (!cancelled) { setProductInstallations(installationValues); setPinHistory(historyValues); } }).catch((error) => recordLoadProblem("Installations", error));
	    Promise.all([api.environments(product.id), api.integrationRuns(product.id), api.auditEvents(product.organisation_id)]).then(([environmentValues, runValues, eventValues]) => {
	      if (cancelled) return;
	      setEnvironments(environmentValues);
	      setRunEnvironmentID(environmentValues.find((environment) => environment.is_production)?.id ?? environmentValues[0]?.id ?? "");
	      setIntegrationRuns(runValues);
	      setAuditEvents(eventValues);
	    }).catch((error) => recordLoadProblem("Runtime activity", error));
    return () => { cancelled = true; };
  }, [fixturePreview, product.id, product.organisation_id, widgetsEnabled]);

  const publicSources = sources.filter((item) => item.visibility === "public");
  const publicResourceCount = publicSources.length;
  const allResources = useMemo(() => [
    ...sources.map((item) => ({ ...item, resourceType: "source" as const, type: item.kind, detail: item.location })),
  ], [sources]);
  const visibleResources = resourceFilter === "all" ? allResources : allResources.filter((item) => item.visibility === resourceFilter);

  function showToast(message: string) {
    setToast(message);
    window.setTimeout(() => setToast(null), 2200);
  }

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

  function openProductCatalog() {
    setProductDescription(product.description);
    setDefaultVersionPolicy(product.default_version_policy);
	setRequirePromotionApproval(product.require_promotion_approval);
    setNewProductProfile(productDefinition?.profiles[0]?.id ?? "");
    setProductCatalogOpen(true);
  }

  async function saveProductDiscoverySettings() {
    setProductCatalogBusy(true);
    try {
      const value = apiConnected
		? await api.updateProductSettings(product.id, productDescription, defaultVersionPolicy, requirePromotionApproval, product.revision)
		: { ...product, description: productDescription.trim(), default_version_policy: defaultVersionPolicy, require_promotion_approval: requirePromotionApproval, catalog_revision: product.catalog_revision + 1, revision: product.revision + 1 };
      setProduct(value);
      setProductRevision(value.revision);
      setProductDescription(value.description);
      showToast("Agent-facing deployment description and default release policy saved.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Deployment discovery settings could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function rewriteDescriptionWithAI() {
    setProductCatalogBusy(true);
    try {
      const value = apiConnected
        ? await api.rewriteProductDescription(product.id, productDescription)
        : { description: "Build reliable voice and messaging experiences with independently versioned APIs, compatible SDKs, API documentation, and policy-authorized tools." };
      setProductDescription(value.description);
      showToast("AI rewrite applied as an editable draft. Save to publish it to agents.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The assistant model could not rewrite the description.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function publishProductVersion() {
    if (!newProductVersion.trim() || !newProductProfile) return;
    setProductCatalogBusy(true);
    try {
      const now = new Date().toISOString();
      const profile = productDefinition?.profiles.find((candidate) => candidate.id === newProductProfile);
      const value = apiConnected
		? await api.createProductVersion(product.id, { version: newProductVersion.trim(), profile_id: newProductProfile, is_latest: newVersionLatest, is_lts: newVersionLTS, release_stage: newVersionStage, rollout_percentage: newVersionRollout })
		: { id: `version_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, version: newProductVersion.trim(), profile_id: newProductProfile, profile_name: profile?.name ?? "Compatibility profile", definition_revision: productDefinition?.revision ?? 1, manifest_hash: `sha256:preview-${Date.now()}`, diff: fixtures!.diff, release_stage: requirePromotionApproval ? "preview" as const : newVersionStage, rollout_percentage: newVersionRollout, promotion_state: requirePromotionApproval ? "pending" as const : "not_required" as const, requested_latest: newVersionLatest, requested_lts: newVersionLTS, drift_status: "healthy" as const, drift_details: [], is_latest: requirePromotionApproval ? false : newVersionLatest || productVersions.length === 0, is_lts: requirePromotionApproval ? false : newVersionLTS, revision: 1, published_at: now };
      if (apiConnected) setProductVersions(await api.productVersions(product.id));
      else setProductVersions((current) => [value, ...current.map((candidate) => value.is_latest ? { ...candidate, is_latest: false } : candidate)]);
      setPinVersionID(value.id);
      setNewProductVersion("");
      setNewVersionLatest(false);
	  setNewVersionLTS(false);
	  setNewVersionStage("active");
	  setNewVersionRollout(100);
      showToast(`Compatibility snapshot ${value.version} published.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The compatibility snapshot could not be published.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  function editProductVersion(version: APIProductVersion) {
    setEditingProductVersion(version);
    setLifecycleLatest(version.is_latest);
    setLifecycleLTS(version.is_lts);
    setLifecycleDeprecated(Boolean(version.deprecated_at));
    setLifecycleMessage(version.deprecation_message ?? "");
    setLifecycleReplacement(version.replacement_version ?? "");
    setLifecycleSunset(version.sunset_at?.slice(0, 10) ?? "");
	setLifecycleRollout(version.rollout_percentage);
	setLifecycleImpact(null);
	setLifecycleImpactAcknowledged(false);
	if (apiConnected) api.productVersionImpact(product.id, version.id).then(setLifecycleImpact).catch(() => {});
	else setLifecycleImpact({ product_version_id: version.id, product_version: version.version, customer_pins: productVersionPins.filter((pin) => pin.scope === "customer" && pin.product_version_id === version.id).length, environment_pins: productVersionPins.filter((pin) => pin.scope === "environment" && pin.product_version_id === version.id).length, installation_pins: productVersionPins.filter((pin) => pin.scope === "installation" && pin.product_version_id === version.id).length, affected_customers: [], affected_environments: [], affected_installations: [], requests_30_days: 1842, tool_calls_30_days: 327 });
    setVersionLifecycleOpen(true);
  }

  async function saveProductVersionLifecycle() {
    if (!editingProductVersion) return;
    setProductCatalogBusy(true);
    try {
	  const input = { is_latest: lifecycleDeprecated ? false : lifecycleLatest, is_lts: lifecycleDeprecated ? false : lifecycleLTS, deprecated: lifecycleDeprecated, deprecation_message: lifecycleDeprecated ? lifecycleMessage : "", replacement_version: lifecycleDeprecated ? lifecycleReplacement : "", sunset_at: lifecycleDeprecated && lifecycleSunset ? new Date(`${lifecycleSunset}T00:00:00Z`).toISOString() : undefined, rollout_percentage: lifecycleRollout, acknowledge_impact: lifecycleImpactAcknowledged, revision: editingProductVersion.revision };
      const value = apiConnected
        ? await api.updateProductVersion(product.id, editingProductVersion.id, input)
		: { ...editingProductVersion, is_latest: input.is_latest, is_lts: input.is_lts, rollout_percentage: input.rollout_percentage, deprecated_at: input.deprecated ? editingProductVersion.deprecated_at ?? new Date().toISOString() : undefined, deprecation_message: input.deprecation_message || undefined, replacement_version: input.replacement_version || undefined, sunset_at: input.sunset_at, revision: editingProductVersion.revision + 1 };
      if (apiConnected) setProductVersions(await api.productVersions(product.id));
      else setProductVersions((current) => current.map((candidate) => candidate.id === value.id ? value : value.is_latest ? { ...candidate, is_latest: false } : candidate));
      setVersionLifecycleOpen(false);
	      showToast(value.promotion_state === "pending" ? `${value.version} channel promotion is awaiting independent approval.` : `Lifecycle metadata for ${value.version} updated.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Version lifecycle settings could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function pinCustomerVersion() {
    if (!pinCustomerID.trim() || !pinVersionID) return;
    setProductCatalogBusy(true);
    try {
      const selected = productVersions.find((version) => version.id === pinVersionID);
      const now = new Date().toISOString();
	  const existing = productVersionPins.find((pin) => pin.scope === pinScope && pin.scope_id === pinCustomerID.trim());
	  const installation = pinScope === "installation" ? productInstallations.find((item) => item.id === pinCustomerID) : undefined;
	  const value = apiConnected
		? await api.saveProductVersionPin(product.id, { scope: pinScope, scope_id: pinCustomerID.trim(), customer_account_id: pinScope === "customer" ? pinCustomerID.trim() : installation?.customer_account_id, product_version_id: pinVersionID, reason: pinReason.trim(), revision: existing?.revision ?? 0 })
		: { id: existing?.id ?? `pin_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, scope: pinScope, scope_id: pinCustomerID.trim(), customer_account_id: pinScope === "customer" ? pinCustomerID.trim() : installation?.customer_account_id ?? "", environment_id: pinScope === "environment" ? pinCustomerID.trim() : installation?.environment_id, installation_id: installation?.id, product_version_id: pinVersionID, product_version: selected?.version ?? "", reason: pinReason.trim(), revision: (existing?.revision ?? 0) + 1, created_at: existing?.created_at ?? now, updated_at: now };
	  setProductVersionPins((current) => [value, ...current.filter((pin) => !(pin.scope === value.scope && pin.scope_id === value.scope_id))]);
      setPinCustomerID("");
      setPinReason("");
	  showToast(`${value.scope} ${value.scope_id} pinned to ${value.product_version}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The scoped version pin could not be saved.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function saveInstallation() {
	if (!installationName.trim() || !installationExternalID.trim() || !installationCustomerID.trim() || !installationEnvironmentID) return;
	setProductCatalogBusy(true);
	try {
	  const now = new Date().toISOString();
	  const value = apiConnected ? await api.saveProductInstallation(product.id, { customer_account_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active", revision: 0 }) : { id: `installation_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, customer_account_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active" as const, revision: 1, created_at: now, updated_at: now };
	  setProductInstallations((current) => [value, ...current]);
	  setInstallationName(""); setInstallationExternalID(""); setInstallationCustomerID("");
	  showToast(`${value.name} is now available for installation-scoped version resolution.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "The installation could not be saved."); }
	finally { setProductCatalogBusy(false); }
  }

  async function reconcileVersion(version: APIProductVersion) {
	setProductCatalogBusy(true);
	try {
	  const value = apiConnected ? await api.reconcileProductVersion(product.id, version.id, version.revision) : { ...version, drift_status: "healthy" as const, drift_details: [], drift_checked_at: new Date().toISOString(), revision: version.revision + 1 };
	  setProductVersions((current) => current.map((item) => item.id === value.id ? value : item));
	  setEditingProductVersion(value);
	  showToast(`Artifact health for ${value.version}: ${value.drift_status}.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "Artifact health could not be checked."); }
	finally { setProductCatalogBusy(false); }
  }

  async function promoteVersion(version: APIProductVersion, action: "request" | "approve" | "reject") {
	setProductCatalogBusy(true);
	try {
	  const note = action === "approve" ? "Generated diff and artifact health reviewed." : action === "reject" ? "Promotion rejected after review." : "Ready for independent review.";
	  const value = apiConnected ? await api.promoteProductVersion(product.id, version.id, action, note, version.revision) : { ...version, promotion_state: action === "approve" ? "approved" as const : action === "reject" ? "rejected" as const : "pending" as const, release_stage: action === "approve" ? "active" as const : "preview" as const, is_latest: action === "approve" ? version.requested_latest : false, is_lts: action === "approve" ? version.requested_lts : false, revision: version.revision + 1 };
	  setProductVersions((current) => current.map((item) => item.id === value.id ? value : value.is_latest ? { ...item, is_latest: false } : item));
	  setEditingProductVersion(value);
	  showToast(`${value.version} promotion is ${value.promotion_state}.`);
	} catch (error) { showToast(error instanceof APIError ? error.message : "Promotion state could not be changed."); }
	finally { setProductCatalogBusy(false); }
  }

  async function removeProductVersionPin(pin: APIProductVersionPin) {
    setProductCatalogBusy(true);
    try {
      if (apiConnected) await api.deleteProductVersionPin(product.id, pin.id);
      setProductVersionPins((current) => current.filter((candidate) => candidate.id !== pin.id));
	  showToast(`${pin.scope} ${pin.scope_id} will now follow the next resolution level.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The scoped version pin could not be removed.");
    } finally {
      setProductCatalogBusy(false);
    }
  }

  async function buildProductAutomatically() {
    setProductBuilderBusy(true);
    const additionalInputs: APIProductBuildInput[] = productBuilderInputs
      .split(/\r?\n/)
      .map((location) => location.trim())
      .filter(Boolean)
      .map((location) => ({ kind: "auto", location }));
    try {
      const fallbackBuildID = `build_${Date.now()}`;
      const value = apiConnected
        ? await api.buildProduct(product.id, additionalInputs)
        : { ...fixtures!.productBuild, id: fallbackBuildID, state: "review" as const, created_at: new Date().toISOString(), completed_at: new Date().toISOString(), inputs: [...fixtures!.productBuild.inputs, ...additionalInputs], proposal: { ...fixtures!.definition, state: "draft" as const, source_build_id: fallbackBuildID } };
      setLatestProductBuild(value);
      setProductBuilderOpen(false);
      setProductBuildReviewOpen(true);
      setProductBuilderInputs("");
      showToast(`${value.inputs.length} sources scanned. Review ${value.unresolved.length || "no"} exception${value.unresolved.length === 1 ? "" : "s"}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The APIs could not be imported.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function publishImportedAPIs() {
    if (!latestProductBuild || latestProductBuild.state !== "review") return;
    setProductBuilderBusy(true);
    try {
      const definition = apiConnected
        ? await api.publishProductBuild(product.id, latestProductBuild.id)
        : { ...latestProductBuild.proposal, state: "published" as const, revision: latestProductBuild.proposal.revision + 1, published_at: new Date().toISOString() };
      setProductDefinition(definition);
      setLatestProductBuild({ ...latestProductBuild, state: "published", proposal: definition, completed_at: latestProductBuild.completed_at ?? new Date().toISOString() });
      setProductBuildReviewOpen(false);
      await refreshCatalog().catch(() => {});
      navigateToSection("product");
      showToast(`${definition.components.length} API proposal${definition.components.length === 1 ? "" : "s"} published to the catalogue.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The imported API proposal could not be published.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function requestVisibility(kind: "source", id: string) {
    const item = sources.find((candidate) => candidate.id === id);
    if (!item) return;
    if (item.visibility === "public") {
      try {
        if (apiConnected) {
          const updated = await api.setSourceVisibility(product.id, id, "private", item.revision, false);
          setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: updated.visibility, revision: updated.revision } : candidate));
        } else setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: "private" } : candidate));
      } catch (error) {
        showToast(error instanceof APIError ? error.message : "Could not update visibility.");
        return;
      }
      showToast(`${item.name} is private. Anonymous access was removed immediately.`);
      return;
    }
    setAcknowledged(false);
    setPendingPublication({
      kind,
      id,
      name: item.name,
      detail: "Its currently published knowledge will become anonymously searchable.",
    });
  }

  async function confirmPublication() {
    if (!pendingPublication || !acknowledged) return;
    const { id, name } = pendingPublication;
    const current = sources.find((item) => item.id === id);
    if (!current) return;
    try {
      if (apiConnected) {
        const updated = await api.setSourceVisibility(product.id, id, "public", current.revision, true);
        setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: updated.visibility, revision: updated.revision } : item));
      } else setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: "public" } : item));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not publish this resource.");
      return;
    }
    setPendingPublication(null);
    setAcknowledged(false);
    showToast(`${name} is now public. The change was added to audit.`);
  }

  async function requestMCPChange(enabled: boolean) {
    if (!enabled) {
      try {
        if (apiConnected) {
          const updated = await api.setPublicMCP(product.id, false, productRevision, false);
          setProductRevision(updated.revision);
          setProduct(updated);
        }
        setPublicMCPEnabled(false);
      } catch (error) {
        showToast(error instanceof APIError ? error.message : "Could not disable Public MCP.");
        return;
      }
      showToast("Public MCP is off. Anonymous requests are no longer accepted.");
      return;
    }
    setAcknowledged(false);
    setPendingMCPEnable(true);
  }

  async function confirmMCPEnable() {
    if (!acknowledged) return;
    try {
      if (apiConnected) {
        const updated = await api.setPublicMCP(product.id, true, productRevision, true);
        setProductRevision(updated.revision);
        setProduct(updated);
      }
      setPublicMCPEnabled(true);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not enable Public MCP.");
      return;
    }
    setPendingMCPEnable(false);
    setAcknowledged(false);
    showToast("Public MCP is enabled and audit logged.");
  }

  function resetSourceForm() {
    setSourceName("");
    setSourceKind("website");
    setSourceLocation("");
    setSourceFile(null);
    setSourceFileError("");
    if (sourceFileInput.current) sourceFileInput.current.value = "";
  }

  function closeSourceDialog(open: boolean) {
    if (!open && sourceBusy) return;
    setAddSourceOpen(open);
    if (!open && !sourceBusy) resetSourceForm();
  }

  function selectSourceKind(kind: SourceKind) {
    setSourceKind(kind);
    setSourceFileError("");
    if (kind !== "upload") {
      setSourceFile(null);
      if (sourceFileInput.current) sourceFileInput.current.value = "";
    }
  }

  function selectSourceFile(file: File | null) {
    setSourceFile(file);
    setSourceFileError(file ? sourceUploadValidationError(file) : "Choose a file to upload.");
    if (file && !sourceName.trim()) setSourceName(file.name.replace(/\.[^.]+$/, ""));
  }

  async function createSource() {
    if (sourceKind === "upload" && !sourceFile) {
      setSourceFileError("Choose a file to upload.");
      return;
    }
    setSourceBusy(true);
    try {
      if (sourceKind === "upload" && sourceFile) {
        const validationError = sourceUploadValidationError(sourceFile);
        if (validationError) {
          setSourceFileError(validationError);
          return;
        }
        try {
          new TextDecoder("utf-8", { fatal: true }).decode(await sourceFile.arrayBuffer());
        } catch {
          setSourceFileError("The selected file is not valid UTF-8 text.");
          return;
        }
      }
      const created = apiConnected
        ? sourceKind === "upload" && sourceFile
          ? await api.uploadSource(product.id, product.organisation_id, sourceName.trim(), sourceFile)
          : await api.createSource(product.id, product.organisation_id, sourceName.trim(), sourceKind, sourceLocation.trim())
        : { id: `src_${Date.now()}`, name: sourceName.trim(), kind: sourceKind, location: sourceKind === "upload" ? sourceFile?.name ?? "uploaded file" : sourceLocation.trim(), visibility: "private" as const, published: false, quarantined: false, revision: 1 };
      setSources((items) => [...items, { id: created.id, name: created.name, kind: created.kind, location: created.location, visibility: created.visibility, published: created.published, quarantined: created.quarantined, crawlState: "draft", pages: 0, lastCrawl: "Not crawled", revision: created.revision }]);
      setAddSourceOpen(false);
      resetSourceForm();
      showToast(`${created.name} was added privately.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not add source.");
    } finally {
      setSourceBusy(false);
    }
  }

  async function crawlSource(id: string) {
    try {
	  if (apiConnected) {
		await api.queueCrawl(product.id, id);
		window.setTimeout(() => pollCrawl(id), 1500);
	  }
      setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState: "queued", lastCrawl: "Queued now" } : item));
      showToast("Crawl queued. The isolated worker will update review state.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not queue crawl.");
    }
  }

  async function pollCrawl(id: string, attempt = 0) {
	try {
	  const jobs = await api.crawlJobs(product.id, id);
	  const latest = jobs[0];
	  if (!latest) return;
	  const crawlState: Source["crawlState"] = latest.state === "failed" ? "failed" : latest.state === "cancelled" ? "cancelled" : latest.state === "review" || latest.state === "succeeded" ? "review" : latest.state === "running" ? "running" : "queued";
	  setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState, pages: latest.fetched_count, lastCrawl: latest.finished_at ? new Date(latest.finished_at).toLocaleString() : latest.state } : item));
	  if ((latest.state === "queued" || latest.state === "running") && attempt < 40) {
		window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
		return;
	  }
	  if (latest.state === "review" || latest.state === "succeeded") {
		const refreshed = (await api.sources(product.id)).find((source) => source.id === id);
		const review = await api.sourceReview(product.id, id, latest.id).catch(() => null);
		if (refreshed) setSources((items) => items.map((item) => item.id === id ? { ...item, revision: refreshed.revision, published: refreshed.published, quarantined: refreshed.quarantined, crawlState: review?.publication ? "synced" : "review", latestPublication: review?.publication ?? item.latestPublication } : item));
	  }
	} catch {
	  if (attempt < 5) window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
	}
  }

  async function attachReviewedSourcePublication(integrationID: string, source: Source, publication: APISourcePublication): Promise<DocumentationAttachmentResult> {
	const [{ integration }, documentationSets] = await Promise.all([api.integration(integrationID), api.resourceSets("documentation")]);
	if (integrationIncludesSourcePublication(integration, publication.id)) {
	  const current = integration.resources?.find((link) => link.kind === "documentation" && manifestIncludesSourcePublication(link.resolved_revision?.manifest, publication.id));
	  return { attached: false, resourceSetName: current?.name ?? source.name, revision: current?.resolved_revision?.revision ?? publication.revision };
	}

	const attachedSetIDs = new Set(integration.resources?.map((link) => link.resource_set_id) ?? []);
	let resource = documentationSets.find((set) => !attachedSetIDs.has(set.id) && manifestIncludesSourcePublication(set.latest_revision?.manifest, publication.id));
	if (!resource) {
	  resource = await api.createResourceSet({
		kind: "documentation",
		name: `${integration.display_name} · ${source.name}`.slice(0, 120),
		description: `Reviewed ${source.name} documentation for ${integration.display_name}.`,
		manifest: [sourcePublicationManifestEntry(source, publication)],
	  });
	}
	const revisionID = resource.latest_revision?.id;
	if (!revisionID) throw new Error("The reviewed documentation set has no immutable revision to pin.");
	await api.attachResourceSet(integration.id, resource.id, revisionID);
	await refreshCatalog();
	return { attached: true, resourceSetName: resource.name, revision: resource.latest_revision?.revision ?? publication.revision };
  }

  function closeSourceReview() {
	setSourceReview(null);
	setSourceReviewSelection([]);
	setSourceReviewAcknowledged(false);
	setSourceReviewAttachIntegrationID("");
  }

  async function publishSource(source: Source, attachIntegrationID = "") {
	if (!apiConnected) {
	  showToast("Generation review is available in the live console.");
	  return;
	}
	setSourceReviewBusy(true);
	try {
	  const review = await api.sourceReview(product.id, source.id);
	  const safe = review.documents.filter((document) => (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0).map((document) => document.id);
	  setSourceReview(review);
	  setSourceReviewSelection(safe);
	  setSourceReviewAcknowledged(false);
	  setSourceReviewAttachIntegrationID(attachIntegrationID);
	} catch (error) {
	  setSourceReviewAttachIntegrationID("");
	  showToast(error instanceof APIError ? error.message : "Could not load this crawl generation for review.");
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  async function confirmSourcePublication() {
	if (!sourceReview || !sourceReviewAcknowledged || sourceReviewSelection.length === 0) return;
	setSourceReviewBusy(true);
	try {
	  const result = await api.publishSource(product.id, sourceReview.source.id, { revision: sourceReview.source.revision, crawl_job_id: sourceReview.crawl_job.id, document_ids: sourceReviewSelection, acknowledge_reviewed: true });
	  const source = sources.find((item) => item.id === result.source.id) ?? {
		id: result.source.id,
		name: result.source.name,
		kind: result.source.kind,
		location: result.source.location,
		visibility: result.source.visibility,
		published: result.source.published,
		quarantined: result.source.quarantined,
		crawlState: "synced" as const,
		pages: result.publication.document_count,
		lastCrawl: result.publication.published_at,
		revision: result.source.revision,
	  };
	  setSources((items) => items.map((item) => item.id === result.source.id ? { ...item, published: result.source.published, quarantined: result.source.quarantined, revision: result.source.revision, crawlState: "synced", latestPublication: result.publication } : item));
	  let message = `${result.source.name} generation r${result.publication.revision} was atomically published.`;
	  if (sourceReviewAttachIntegrationID) {
		try {
		  const attachment = await attachReviewedSourcePublication(sourceReviewAttachIntegrationID, source, result.publication);
		  message = attachment.attached
			? `${message} Revision ${attachment.revision} was pinned to the API.`
			: `${message} That exact revision was already attached.`;
		} catch (error) {
		  message = `${message} Attachment still needs attention: ${error instanceof APIError || error instanceof Error ? error.message : "the reviewed set could not be attached"}`;
		}
	  }
	  closeSourceReview();
	  showToast(message);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not publish this reviewed generation.");
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  function fixtureCatalog(connection: APIMCPConnection): APIMCPCatalog {
    const schema = { type: "object", additionalProperties: false, properties: { title: { type: "string" } }, required: ["title"] };
    return { connection, catalog_hash: "sha256:48f2a81d", ttl_ms: 30000, tools: [
      { name: "incidents.create", title: "Create incident", description: "Create a support incident for the signed-in developer.", input_schema: schema, output_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" } }, required: ["incident_id"] }, annotations: { destructiveHint: false }, schema_hash: "sha256:8f44e6" },
      { name: "incidents.get", title: "Get incident", description: "Read one support incident.", input_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" } }, required: ["incident_id"] }, schema_hash: "sha256:1183ce" },
      { name: "incidents.comment", title: "Comment on incident", description: "Add a comment as the signed-in developer.", input_schema: { type: "object", additionalProperties: false, properties: { incident_id: { type: "string" }, body: { type: "string" } }, required: ["incident_id", "body"] }, annotations: { destructiveHint: false }, schema_hash: "sha256:211a40" },
    ] };
  }

  async function inspectMCPConnection(connection: APIMCPConnection) {
    setMCPBusy(true);
    try {
      const catalog = apiConnected ? await api.inspectMCPConnection(product.id, connection.id) : fixtureCatalog(connection);
      setMCPCatalog(catalog);
      setMCPSelectedTools(catalog.tools.map((tool) => tool.name));
      setMCPImportFailures({});
      setMCPImportOpen(true);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not inspect the upstream MCP catalog.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function createMCPConnection() {
    setMCPBusy(true);
    try {
      const input = {
        organisation_id: product.organisation_id,
        name: mcpName,
        namespace: mcpNamespace,
        endpoint: mcpEndpoint,
        auth_mode: mcpAuthMode,
        credential: mcpCredential,
        oauth_client_id: mcpOAuthClientID,
        oauth_client_secret: mcpOAuthClientSecret,
        oauth_issuer: mcpOAuthIssuer,
        authorization_url: mcpAuthorizationURL,
        token_url: mcpTokenURL,
        scopes: mcpScopes.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean),
      };
      const connection = apiConnected ? await api.createMCPConnection(product.id, input) : { id: `mcp_${Date.now()}`, product_id: product.id, protocol_version: "2026-07-28" as const, state: "active" as const, revision: 1, ...input };
      setMCPConnections((items) => [...items, connection]);
      setMCPConnectionOpen(false);
      setMCPName(""); setMCPNamespace(""); setMCPEndpoint(""); setMCPCredential(""); setMCPOAuthClientSecret("");
      const catalog = apiConnected ? await api.inspectMCPConnection(product.id, connection.id) : fixtureCatalog(connection);
      setMCPCatalog(catalog);
      setMCPSelectedTools(catalog.tools.map((tool) => tool.name));
      setMCPImportFailures({});
      setMCPImportOpen(true);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create the Stateless MCPv2 connection.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function importMCPTools() {
    if (!mcpCatalog || mcpSelectedTools.length === 0) return;
    setMCPBusy(true);
    try {
      const grants = mcpGrants.split(",").map((value) => value.trim()).filter(Boolean);
      if (apiConnected) {
        const result = await api.importMCPTools(product.id, mcpCatalog.connection.id, { tool_names: mcpSelectedTools, required_grants: grants, confirmation_required: mcpConfirmationRequired, timeout_ms: 10000 });
        const changed = [...result.created, ...result.updated, ...result.unchanged, ...result.drifted];
        setTools((items) => [...items.filter((item) => !changed.some((candidate) => candidate.id === item.id)), ...changed]);
        setMCPConnections((items) => items.map((item) => item.id === result.connection.id ? result.connection : item));
		const rejected = Object.entries(result.rejected);
		if (rejected.length > 0) {
		  setMCPImportFailures(result.rejected);
		  setMCPSelectedTools(rejected.map(([name]) => name));
		  const reviewed = result.created.length + result.updated.length + result.unchanged.length + result.drifted.length;
		  showToast(`${reviewed} tool${reviewed === 1 ? "" : "s"} reviewed; ${rejected.length} rejected. Review the reasons in this dialog.`);
		  return;
		}
		setMCPImportFailures({});
		setMCPImportOpen(false);
		setMCPGrants("");
		const messages = [
		  result.created.length > 0 ? `${result.created.length} draft${result.created.length === 1 ? "" : "s"} created` : "",
		  result.updated.length > 0 ? `${result.updated.length} draft${result.updated.length === 1 ? "" : "s"} updated` : "",
		  result.unchanged.length > 0 ? `${result.unchanged.length} already current` : "",
		  result.drifted.length > 0 ? `${result.drifted.length} published tool${result.drifted.length === 1 ? "" : "s"} blocked by schema drift` : "",
		].filter(Boolean);
		showToast(messages.length > 0 ? `${messages.join("; ")}.` : "No upstream tool changes were needed.");
      } else {
        const imported = mcpCatalog.tools.filter((item) => mcpSelectedTools.includes(item.name)).map((item, index): APITool => ({ id: `tool_mcp_${index}`, organisation_id: product.organisation_id, product_id: product.id, namespace: mcpCatalog.connection.namespace, name: item.name.replace(/[^A-Za-z0-9_]/g, "_"), description: item.description ?? item.title ?? item.name, input_schema: item.input_schema, output_schema: item.output_schema ?? {}, state: "draft", revision: 1, http_method: "MCP", authorization_policy: { required_grants: grants, confirmation_required: mcpConfirmationRequired }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: mcpCatalog.connection.id, upstream_tool_name: item.name, upstream_schema_hash: item.schema_hash }));
        setTools((items) => [...items, ...imported]);
		setMCPImportFailures({});
		setMCPImportOpen(false);
		setMCPGrants("");
		showToast(`${imported.length} upstream tool${imported.length === 1 ? "" : "s"} imported as reviewed drafts.`);
      }
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not import the selected MCP tools.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function createSupportDeliveryAttempt(submission: APISupportSubmission) {
	try {
	  const value = apiConnected ? await api.createSupportDeliveryAttempt(submission.id) : { ...submission, state: "pending" as const, last_error: undefined };
	  setReportSubmissions((items) => items.map((item) => item.id === value.id ? value : item));
	  showToast("Submission queued for another delivery attempt.");
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not retry this submission.");
	}
  }

  async function openSupportSubmission(submission: APISupportSubmission) {
	setReportDetail(submission);
	if (!apiConnected) return;
	setReportDetailBusy(true);
	try {
	  setReportDetail(await api.supportSubmission(submission.id));
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not decrypt this submission.");
	  setReportDetail(null);
	} finally {
	  setReportDetailBusy(false);
	}
  }

  async function beginRootUser() {
    setRootBusy(true);
    try {
      const value = await api.beginRootUser({ email: rootEmail, display_name: rootDisplayName, password: rootPassword });
      setRootEnrollment(value);
      showToast("Root account prepared. Enrol MFA to finish.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not prepare root administrator.");
    } finally {
      setRootBusy(false);
    }
  }

  async function completeRootUser() {
    if (!rootEnrollment) return;
    setRootBusy(true);
    try {
      const value = await api.completeRootUser(rootEnrollment.enrollment_id, rootCode);
      setRootUsers((items) => [...items, value.user]);
      setRootRecoveryCodes(value.recovery_codes);
      setRootEnrollment(null);
      setRootCode("");
      showToast("MFA-protected root administrator created.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "MFA verification failed.");
    } finally {
      setRootBusy(false);
    }
  }

  async function revokeRootUser(user: APIUser) {
    if (!window.confirm(`Revoke root access for ${user.email}? Their active sessions will end immediately.`)) return;
    try {
      await api.revokeRootUser(user.id);
      setRootUsers((items) => items.map((item) => item.id === user.id ? { ...item, revoked_at: new Date().toISOString() } : item));
      showToast(`${user.email} was revoked.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not revoke root administrator.");
    }
  }

	  function openAIConnection(provider: APIAIProviderConnection["provider"]) {
	    const connection = aiConnections.find((item) => item.provider === provider);
	    setLLMProvider(provider);
	    setLLMEndpoint(connection?.endpoint ?? aiProviderOrigin(provider));
	    setLLMCredential("");
	    setProviderEnabled(connection?.enabled ?? true);
	    setProviderIsBackup(connection?.is_backup ?? false);
	    setProviderBackupAnalysisModel(connection?.backup_models.analysis ?? aiModelDefaults[provider].analysis);
	    setProviderBackupAssistantModel(connection?.backup_models.assistant ?? aiModelDefaults[provider].assistant);
	    setProviderPickerOpen(false);
	    setProviderOpen(true);
	  }

	  function openLLMProfile(role: AIWorkload) {
	    const profile = aiProfiles.find((item) => item.workload === role);
	    const connection = aiConnections.find((item) => item.id === profile?.provider_connection_id) ?? aiConnections.find((item) => item.enabled && !item.is_backup);
	    setLLMRole(role);
	    setLLMConnectionID(connection?.id ?? "");
	    setLLMModel(profile?.model ?? (connection ? aiModelDefaults[connection.provider][role] : ""));
	    setLLMInputTokens(String(profile?.max_input_tokens ?? 128000));
	    setLLMOutputTokens(String(profile?.max_output_tokens ?? (role === "assistant" ? 1024 : 8192)));
	    setLLMDailyBudget(String(profile?.daily_token_budget ?? 0));
	    setLLMEnabled(profile?.enabled ?? false);
	    setLLMOpen(true);
	  }

	  function changeLLMConnection(connectionID: string) {
	    setLLMConnectionID(connectionID);
	    const connection = aiConnections.find((item) => item.id === connectionID);
	    if (connection) setLLMModel(aiModelDefaults[connection.provider][llmRole]);
	  }

	  async function saveAIConnection() {
	    setProviderBusy(true);
	    try {
	      const current = aiConnections.find((item) => item.provider === llmProvider);
	      const value = await api.saveAIConnection({ organisation_id: product.organisation_id, provider: llmProvider, endpoint: llmEndpoint, credential: llmCredential, enabled: providerEnabled, is_backup: providerIsBackup, backup_models: providerIsBackup ? { analysis: providerBackupAnalysisModel, assistant: providerBackupAssistantModel } : {}, revision: current?.revision ?? 0 });
	      setAIConnections((items) => [...items.filter((item) => item.id !== value.id && item.provider !== value.provider), value]);
	      setLLMCredential("");
	      setProviderOpen(false);
	      showToast(`${aiProviderLabel(value.provider)} connected.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not connect AI provider.");
	    } finally {
	      setProviderBusy(false);
	    }
	  }

	  async function testAIConnection(connection: APIAIProviderConnection) {
	    try {
	      const value = await api.testAIConnection(connection.id);
	      setAIConnections((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast(`${aiProviderLabel(value.provider)} connection works.`);
	    } catch (error) {
	      const updated = await api.aiConnections().catch(() => aiConnections);
	      setAIConnections(updated);
	      showToast(error instanceof APIError ? error.message : "Connection test failed.");
	    }
	  }

	  async function saveLLMProfile() {
	    setLLMBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === llmRole);
	      const value = await api.saveAIProfile(product.id, llmRole, { organisation_id: product.organisation_id, provider_connection_id: llmConnectionID, model: llmModel, max_input_tokens: Number(llmInputTokens), max_output_tokens: Number(llmOutputTokens), daily_token_budget: Number(llmDailyBudget), enabled: llmEnabled, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      setLLMOpen(false);
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
    } finally {
      setLLMBusy(false);
    }
	  }

	  async function saveAIWorkloadSelection(role: AIWorkload, connectionID: string, modelID: string) {
	    setLLMBusy(true);
	    try {
	      const current = aiProfiles.find((item) => item.workload === role);
	      const value = await api.saveAIProfile(product.id, role, { organisation_id: product.organisation_id, provider_connection_id: connectionID, model: modelID, max_input_tokens: current?.max_input_tokens ?? 128000, max_output_tokens: current?.max_output_tokens ?? (role === "assistant" ? 1024 : 8192), daily_token_budget: current?.daily_token_budget ?? 0, enabled: true, revision: current?.revision ?? 0 });
	      setAIProfiles((items) => [...items.filter((item) => item.workload !== value.workload), value].sort((a, b) => a.workload.localeCompare(b.workload)));
	      showToast(`${aiWorkloads.find((workload) => workload.role === value.workload)?.name ?? value.workload} workload saved.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not save AI model.");
	    } finally {
	      setLLMBusy(false);
	    }
	  }

	  async function createRecipe(prompt: string, integrationID: string): Promise<APIRecipe | null> {
	    setRecipeBusy(true);
	    try {
	      const value = await api.createRecipe(product.id, prompt, integrationID);
	      setRecipes((items) => [value, ...items.filter((item) => item.id !== value.id)]);
	      api.analyses(product.id).then(setAnalyses).catch(() => {});
	      showToast("Recipe draft created from current product evidence.");
	      return value;
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not create this recipe.");
	      return null;
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function generateRecipesFromEvidence(integrationID?: string) {
	    setRecipeBusy(true);
	    try {
	      const scopedAnalyses = analyses.filter((candidate) => analysisMatchesIntegration(candidate, integrationID));
	      const scopedRecipes = recipes.filter((candidate) => recipeMatchesIntegration(candidate, integrationID));
	      let analysis = [...scopedAnalyses].sort((left, right) => right.created_at.localeCompare(left.created_at))[0];
	      const runningSince = analysis?.state === "running" ? Date.parse(analysis.created_at) : Number.NaN;
	      const staleRunning = analysis?.state === "running" && (!Number.isFinite(runningSince) || Date.now() - runningSince > 5 * 60 * 1000);
	      const evidenceChanged = scopedRecipes.some((recipe) => recipe.state === "outdated");
	      if (!analysis || analysis.state === "failed" || staleRunning || evidenceChanged) {
	        analysis = await api.analyseIntegration(product.id, integrationID);
	        setAnalyses((items) => [analysis, ...items.filter((item) => item.id !== analysis.id)]);
	      }
	      const unansweredBlocker = analysis.unknowns.find((unknown) => unknown.blocking && !unknown.answer?.trim());
	      if (analysis.state !== "review" || unansweredBlocker) {
	        showToast(unansweredBlocker ? `Answer “${unansweredBlocker.question}” before generating recipes.` : "Evidence analysis is still running. Try again when it is ready for review.");
	        return;
	      }
	      const generated = await api.generateRecipes(product.id, analysis.id, integrationID);
	      setRecipes((items) => [...generated, ...items.filter((item) => !generated.some((candidate) => candidate.id === item.id))]);
	      showToast(`${generated.length} grounded recipe${generated.length === 1 ? "" : "s"} generated for review.`);
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Recipes could not be generated from the current evidence.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function generateIntegrationAgentGuide(integrationID: string) {
		const analysis = await api.analyseIntegration(product.id, integrationID);
		setAnalyses((items) => [analysis, ...items.filter((item) => item.id !== analysis.id)]);
		return analysis;
	  }

	  async function reworkRecipe(recipe: APIRecipe, instruction: string) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.reworkRecipe(product.id, recipe.id, instruction);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("A new recipe revision is ready for review.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not rework this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function editRecipe(recipe: APIRecipe, markdown: string, references: APIRecipeReference[], visibility: APIRecipe["visibility"]) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.updateRecipe(product.id, recipe.id, markdown, references, visibility);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Human-authored recipe revision saved for review.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not save this recipe revision.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function approveRecipe(recipe: APIRecipe) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.approveRecipe(product.id, recipe.id);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Current recipe revision approved.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not approve this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function publishRecipe(recipe: APIRecipe) {
	    setRecipeBusy(true);
	    try {
	      const value = await api.publishRecipe(product.id, recipe.id);
	      setRecipes((items) => items.map((item) => item.id === value.id ? value : item));
	      showToast("Recipe published to MCP resources.");
	    } catch (error) {
	      showToast(error instanceof APIError ? error.message : "Could not publish this recipe.");
	    } finally {
	      setRecipeBusy(false);
	    }
	  }

	  async function runSystemDoctor() {
    try {
      const value = await api.systemDoctor();
      const passing = value.checks.filter((check) => check.status === "ok").length;
      showToast(value.status === "ok" ? `System Doctor passed all ${passing} checks.` : `System Doctor found ${value.checks.length - passing} check(s) requiring attention.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "System Doctor could not run.");
    }
  }

  async function startIntegrationRun() {
	setRunBusy(true);
	try {
	  const value = apiConnected
	    ? await api.startIntegrationRun(product.id, runEnvironmentID, runOutcome)
	    : { id: `run_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, environment_id: runEnvironmentID, requested_outcome: runOutcome, state: "running" as const, started_at: new Date().toISOString() };
	  setIntegrationRuns((items) => [value, ...items]);
	  setRunOpen(false);
	  setRunOutcome("");
	  showToast("API run started.");
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not start API run.");
	} finally {
	  setRunBusy(false);
	}
  }

  async function completeIntegrationRun(run: APIIntegrationRun, succeeded: boolean) {
	const failureCode = succeeded ? "" : window.prompt("Failure code (for example validation_failed)", "validation_failed")?.trim() ?? "";
	if (!succeeded && !failureCode) return;
	try {
	  const value = apiConnected
	    ? await api.completeIntegrationRun(product.id, run.id, succeeded, succeeded, failureCode)
	    : { ...run, state: succeeded ? "succeeded" as const : "failed" as const, reported_success: succeeded, validated_success: succeeded, failure_code: failureCode || undefined, finished_at: new Date().toISOString() };
	  setIntegrationRuns((items) => items.map((item) => item.id === value.id ? value : item));
	  if (apiConnected) setAnalytics(await api.analytics(product.id));
	  showToast(succeeded ? "Validated success recorded." : "Validated failure recorded for diagnosis.");
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not complete API run.");
	}
  }

  async function createWidget() {
    const allowedOrigins = widgetOrigins.split(/[\n,]/).map((value) => value.trim()).filter(Boolean);
    if (!widgetName.trim() || allowedOrigins.length === 0 || widgetIntegrationIDs.length === 0) {
      showToast("Add a name, an allowed origin, and at least one API.");
      return;
    }
    setWidgetBusy(true);
    try {
      const input: APIWidgetInput = { name: widgetName.trim(), allowed_origins: allowedOrigins, integration_ids: widgetIntegrationIDs, appearance: { theme: "auto", launcher_position: "right", greeting: "How can I help?" } };
      const created = await api.createWidget(input);
      setWidgets((items) => [...items, created.widget]);
      setWidgetCreateOpen(false);
      setWidgetCredential({ widgetID: created.widget.id, secret: created.secret });
      setWidgetName("Customer assistant");
      setWidgetOrigins("http://localhost:3000");
      setWidgetIntegrationIDs([]);
      navigateToPath(entityPath("widget", created.widget.id));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create the widget.");
    } finally {
      setWidgetBusy(false);
    }
  }

  async function updateWidget(widget: APIWidget, input: APIWidgetInput): Promise<APIWidget | null> {
    setWidgetBusy(true);
    try {
      const updated = await api.updateWidget(widget.id, { ...input, revision: widget.revision });
      setWidgets((items) => items.map((item) => item.id === updated.id ? updated : item));
      showToast("Widget settings saved.");
      return updated;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not update the widget.");
      return null;
    } finally {
      setWidgetBusy(false);
    }
  }

  async function setWidgetState(widget: APIWidget, state: "active" | "disabled"): Promise<APIWidget | null> {
    setWidgetBusy(true);
    try {
      const updated = state === "active" ? await api.activateWidget(widget.id, widget.revision) : await api.disableWidget(widget.id, widget.revision);
      setWidgets((items) => items.map((item) => item.id === updated.id ? updated : item));
      showToast(state === "active" ? "Widget is live." : "Widget disabled immediately.");
      return updated;
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not change widget state.");
      return null;
    } finally {
      setWidgetBusy(false);
    }
  }

  async function rotateWidgetSecret(widget: APIWidget) {
    setWidgetBusy(true);
    try {
      const created = await api.createWidgetSecret(widget.id);
      setWidgetCredential({ widgetID: widget.id, secret: created.secret });
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not create a new widget secret.");
    } finally {
      setWidgetBusy(false);
    }
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
  const entityDetail = useMemo<EntityDetail | null>(() => {
    if (consoleRoute.kind !== "entity") return null;
    const date = (value?: string) => value ? new Date(value).toLocaleString() : "—";
    const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
    switch (consoleRoute.entity) {
      case "integration": {
        const value = integrations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "API", title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([["API ID", value.id], ["Lifecycle", value.lifecycle], ["Revision", value.revision], ["Resources", value.resources?.length ?? 0], ["Access connections", value.access_connection_ids?.length ?? 0], ["Sunset", date(value.sunset_at)]]) } : null;
      }
      case "widget": {
        const value = widgets.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Authenticated widget", title: value.name, description: `${value.integration_ids.length} APIs · ${value.allowed_origins.length} origins`, fields: fields([["UID", value.id], ["State", value.state], ["Revision", value.revision]]) } : null;
      }
      case "resource-set": {
        const value = resourceSets.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Reusable resource set", title: value.name, description: value.description || "Reusable API resource configuration.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["Revision", value.latest_revision?.revision ?? value.revision], ["APIs", value.integration_ids?.length ?? 0]]) } : null;
      }
      case "source": {
        const value = sources.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Documentation source", title: value.name, description: value.location, fields: fields([["UID", value.id], ["Kind", value.kind], ["Visibility", value.visibility], ["Crawl state", value.crawlState], ["Pages", value.pages], ["Revision", value.revision]]) } : null;
      }
      case "tool": {
        const value = tools.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Tool", title: `${value.namespace}.${value.name}`, description: value.description || "Agent-facing tool definition.", fields: fields([["UID", value.id], ["Backend", value.backend_kind ?? "http"], ["Method", value.http_method], ["State", value.state], ["Revision", value.revision], ["Timeout", `${value.timeout_ms} ms`]]) } : null;
      }
      case "connection": {
        const value = mcpConnections.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "MCP connection", title: value.name, description: value.endpoint, fields: fields([["UID", value.id], ["Namespace", value.namespace], ["Protocol", value.protocol_version], ["Authentication", value.auth_mode], ["State", value.state], ["Last inspected", date(value.last_synced_at)]]) } : null;
      }
      case "access-definition": {
        const value = accessDefinitions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Access definition", title: value.name, description: `${value.instance_label_plural} managed by the provider.`, fields: fields([["UID", value.id], ["Service key", value.service_key], ["Cardinality", value.instance_cardinality], ["Credential scope", value.credential_scope], ["Authentication", value.management_auth_type], ["State", value.state]]) } : null;
      }
      case "access-connection": {
        const value = accessConnections.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Service connection", title: value.name, description: value.definition?.name || "Provider-owned service connection.", fields: fields([["UID", value.id], ["State", value.state], ["Region", value.region], ["Environment", value.environment_id], ["APIs", value.integration_ids?.length ?? 0], ["Revision", value.revision]]) } : null;
      }
      case "installation": {
        const value = productInstallations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Installation", title: value.name, description: value.external_id, fields: fields([["UID", value.id], ["Customer account", value.customer_account_id], ["Environment", value.environment_id], ["State", value.state], ["Revision", value.revision], ["Updated", date(value.updated_at)]]) } : null;
      }
      case "release": {
        const value = productVersions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Compatibility snapshot", title: value.version, description: value.diff.summary, fields: fields([["UID", value.id], ["Profile", value.profile_name], ["Stage", value.release_stage], ["Promotion", value.promotion_state], ["Rollout", `${value.rollout_percentage}%`], ["Manifest", value.manifest_hash]]) } : null;
      }
      case "run": {
        const value = integrationRuns.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Connector run", title: value.requested_outcome, description: `Run ${value.id}`, fields: fields([["UID", value.id], ["State", value.state], ["Environment", value.environment_id], ["Reported success", value.reported_success], ["Validated success", value.validated_success], ["Started", date(value.started_at)], ["Finished", date(value.finished_at)]]) } : null;
      }
      case "support-route": {
        const value = supportRoutes.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Reporting policy", title: value.name, description: value.is_default ? "Default policy for unassigned APIs." : "API-specific support delivery.", fields: fields([["UID", value.id], ["State", value.state], ["Bug reports", value.bug_reports_enabled ? "Enabled" : "Disabled"], ["Feedback", value.feedback_enabled ? "Enabled" : "Disabled"], ["Retention", `${value.retention_days} days`], ["APIs", value.integration_ids?.length ?? 0]]) } : null;
      }
      case "report": {
        const value = reportSubmissions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Report submission", title: value.summary, description: "Sanitized submission metadata. Decrypted report content remains administrator-gated.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["API", value.trusted_integration?.display_name], ["Delivery attempts", value.attempts], ["Created", date(value.created_at)]]) } : null;
      }
      case "audit-event": {
        const value = auditEvents.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Audit event", title: value.action, description: `${value.target_type} · ${value.target_id}`, fields: fields([["UID", value.id], ["Actor", value.actor_id], ["Request", value.request_id], ["Created", date(value.created_at)]]) } : null;
      }
      case "root-user": {
        const value = rootUsers.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Root administrator", title: value.display_name, description: value.email, fields: fields([["UID", value.id], ["Role", value.role], ["Status", value.revoked_at ? "Revoked" : "MFA active"], ["Revoked", date(value.revoked_at)]]) } : null;
      }
    }
  }, [consoleRoute, integrations, widgets, resourceSets, sources, tools, mcpConnections, accessDefinitions, accessConnections, productInstallations, productVersions, integrationRuns, supportRoutes, reportSubmissions, auditEvents, rootUsers]);

  function routeURL(path: string) {
    const preview = process.env.NODE_ENV === "development" && new URLSearchParams(window.location.search).get("preview") === "fixtures" ? window.location.search : "";
    return `${path}${preview}`;
  }

  function confirmToolBuilderNavigation(nextPath: string) {
    const current = consoleRouteRef.current;
    if (!toolBuilderDirtyRef.current || current.kind !== "tool-builder" || current.path === nextPath) return true;
    return window.confirm("Discard your unsaved tool changes?");
  }

  function navigateToPath(path: string, replace = false) {
    const next = parseAvailableConsolePath(path, widgetsEnabled);
    const current = consoleRouteRef.current;
    if (!confirmToolBuilderNavigation(next.path)) return;
    if (current.path !== next.path) toolBuilderDirtyRef.current = false;
    if (next.kind !== "tool-builder") setToolBuilderSeed(null);
    if (typeof window !== "undefined") {
      const method = replace ? "replaceState" : "pushState";
      if (window.location.pathname !== next.path || replace) window.history[method](null, "", routeURL(next.path));
      window.scrollTo({ top: 0, behavior: "auto" });
    }
    consoleRouteRef.current = next;
    setConsoleRoute(next);
	requestAnimationFrame(() => document.getElementById("main-content")?.focus());
  }

  function navigateToSection(destination: Section) {
    navigateToPath(sectionPath(destination));
  }

  function navigateToGroup(group: NavigationGroup | "settings") {
    if (group === "settings") {
      navigateToSection("settings");
      return;
    }
    const destination = navigation.find((item) => item.id === group);
    if (destination) navigateToSection(destination.defaultSection);
  }

  const workspaceClass = consoleRoute.kind === "tool-builder"
    ? "workspace-wide"
    : section === "identity" || section === "settings"
      ? "workspace-compact"
      : "workspace-default";

  return (
    <Suspense fallback={<div className="console-loading-boundary" role="status"><RefreshCw className="spin" /><span>Loading console workspace…</span></div>}>
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark" aria-hidden="true">D</span><span className="brand-copy"><strong>DokoSoko</strong></span></div>
        <nav aria-label="Main navigation">
          {navigation.map((item) => {
            const Icon = item.icon;
            return <ConsoleLink key={item.id} path={sectionPath(item.defaultSection)} onNavigate={navigateToPath} className={`nav-item ${activeNavigation?.id === item.id ? "active" : ""}`} ariaCurrent={activeNavigation?.id === item.id ? "page" : undefined}><Icon /><span>{item.label}</span></ConsoleLink>;
          })}
        </nav>
        <div className="sidebar-bottom">
          <ThemeToggle />
          <ConsoleLink path={sectionPath("settings")} onNavigate={navigateToPath} className={`nav-item ${section === "settings" ? "active" : ""}`} ariaCurrent={section === "settings" ? "page" : undefined}><Settings /><span>Settings</span></ConsoleLink>
          <div className="account"><span className="avatar">{(currentUser?.display_name ?? "Yuriy Admin").split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><strong>{currentUser?.display_name ?? "Yuriy"}</strong><small>{currentUser ? "Root administrator" : "Platform admin"}</small></span>{onLogout && <button type="button" className="logout-button" aria-label="Sign out" title="Sign out" onClick={onLogout}><LogOut /></button>}</div>
        </div>
      </aside>

      <main id="main-content" className={workspaceClass} tabIndex={-1}>
        <header className="topbar">
          <div className="topbar-inner">
            <div className="product-switcher"><span className="product-logo">{product.name.slice(0, 1).toUpperCase()}</span><span><small>Deployment</small><strong>{product.name}</strong></span></div>
            <select className="mobile-navigation" aria-label="Console section" value={section === "settings" ? "settings" : activeNavigation?.id ?? "apis"} onChange={(event) => navigateToGroup(event.target.value as NavigationGroup | "settings")}>
              {navigation.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
              <option value="settings">Settings</option>
            </select>
            <div className="topbar-actions"><div className="mobile-theme-toggle"><ThemeToggle /></div></div>
          </div>
        </header>

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
