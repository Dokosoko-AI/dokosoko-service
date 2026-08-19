"use client";

import {
  Activity,
  AlertCircle,
  BarChart3,
  BookOpen,
  Bot,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Copy,
  Database,
  ExternalLink,
  FileJson2,
  GitBranch,
  Globe2,
  KeyRound,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  MoreHorizontal,
  Package as PackageIcon,
  Plus,
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
import { useEffect, useMemo, useState } from "react";
import { APIAnalytics, APIAuditEvent, APICredentialLease, APIEnvironment, APIError, APIIdentity, APIIntegrationRun, APILLMProfile, APIMCPCatalog, APIMCPConnection, APIProduct, APIProductBinding, APIProductBuild, APIProductBuildInput, APIProductComponent, APIProductDefinition, APIProject, APIProvider, APITool, APIUser, APIWidgetSnippets, Distribution, SetupEnrollment, api } from "../lib/api";
import { Badge, Button, Dialog, Switch } from "./catalyst";

type Section = "overview" | "product" | "sources" | "packages" | "projects" | "connections" | "tools" | "distribution" | "runs" | "analytics" | "activity" | "settings";
type Visibility = "private" | "public";

type Source = {
  id: string;
  name: string;
  kind: string;
  location: string;
  visibility: Visibility;
  published: boolean;
  quarantined: boolean;
  crawlState: "queued" | "running" | "synced" | "review" | "failed";
  pages: number;
  lastCrawl: string;
  revision: number;
};

type ProductPackage = {
  id: string;
  name: string;
  ecosystem: string;
  version: string;
  mode: "public" | "proxy" | "fetch";
  visibility: Visibility;
  published: boolean;
  status: "ready" | "checking";
  revision: number;
};

type PendingPublication = {
  kind: "source" | "package";
  id: string;
  name: string;
  detail: string;
};

const nav: Array<{ id: Section; label: string; icon: typeof LayoutDashboard }> = [
  { id: "overview", label: "Overview", icon: LayoutDashboard },
  { id: "product", label: "Product definition", icon: Sparkles },
  { id: "sources", label: "Sources", icon: BookOpen },
  { id: "packages", label: "Packages", icon: PackageIcon },
  { id: "projects", label: "Projects & credentials", icon: KeyRound },
  { id: "connections", label: "MCP connections", icon: Share2 },
  { id: "tools", label: "Tools", icon: Wrench },
  { id: "distribution", label: "MCP & widgets", icon: Radio },
  { id: "runs", label: "Integration runs", icon: Activity },
  { id: "analytics", label: "Analytics", icon: BarChart3 },
  { id: "activity", label: "Activity & audit", icon: ShieldCheck },
];

const initialSources: Source[] = [
  { id: "src_docs", name: "Developer documentation", kind: "Website", location: "docs.acme.dev", visibility: "private", published: true, quarantined: false, crawlState: "synced", pages: 284, lastCrawl: "12 min ago", revision: 1 },
  { id: "src_api", name: "Platform API", kind: "OpenAPI", location: "api/openapi.yaml", visibility: "private", published: false, quarantined: false, crawlState: "review", pages: 94, lastCrawl: "2 hours ago", revision: 1 },
  { id: "src_examples", name: "SDK examples", kind: "Git repository", location: "acme/sdk-examples", visibility: "private", published: false, quarantined: false, crawlState: "failed", pages: 0, lastCrawl: "1 day ago", revision: 1 },
];

const initialPackages: ProductPackage[] = [
  { id: "pkg_node", name: "@acme/node", ecosystem: "npm", version: "2.4.1", mode: "proxy", visibility: "private", published: true, status: "ready", revision: 1 },
  { id: "pkg_go", name: "go.acme.dev/sdk", ecosystem: "Go", version: "1.8.0", mode: "fetch", visibility: "private", published: true, status: "ready", revision: 1 },
  { id: "pkg_swift", name: "AcmeKit", ecosystem: "Swift", version: "3.1.0", mode: "public", visibility: "private", published: false, status: "checking", revision: 1 },
];

const initialTools: APITool[] = [
  { id: "tool_sandbox", organisation_id: "org_acme", product_id: "prod_acme", namespace: "projects", name: "create_sandbox", description: "Create a sandbox", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_credentials", organisation_id: "org_acme", product_id: "prod_acme", namespace: "credentials", name: "issue", description: "Issue credentials", input_schema: {}, output_schema: {}, state: "draft", revision: 1, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_incidents", organisation_id: "org_acme", product_id: "prod_acme", namespace: "support", name: "create_incident", description: "Create a support incident", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "MCP", authorization_policy: { required_entitlements: ["support.write"] }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: "mcp_support", upstream_tool_name: "incidents.create", upstream_schema_hash: "sha256:8f44e6" },
];

const fixtureProduct: APIProduct = { id: "prod_acme", organisation_id: "org_acme", name: "Acme Platform", slug: "acme", public_mcp_enabled: false, revision: 1 };
const fixtureEnvironment: APIEnvironment = { id: "env_prod", organisation_id: "org_acme", product_id: "prod_acme", name: "Production", slug: "production", is_production: true, revision: 1 };
const fixtureMCPConnections: APIMCPConnection[] = [{ id: "mcp_support", organisation_id: "org_acme", product_id: "prod_acme", name: "Support operations", namespace: "support", endpoint: "https://mcp.support.example/v2", protocol_version: "2026-07-28", auth_mode: "delegated_oauth", oauth_client_id: "dokosoko-production", oauth_issuer: "https://identity.support.example", authorization_url: "https://identity.support.example/oauth/authorize", token_url: "https://identity.support.example/oauth/token", scopes: ["incidents.read", "incidents.write"], state: "active", last_synced_at: "2026-08-19T11:48:00Z", last_catalog_hash: "sha256:48f2a81d", revision: 2 }];

const fixtureDefinition: APIProductDefinition = {
  id: "definition_acme",
  organisation_id: "org_acme",
  product_id: "prod_acme",
  name: "Acme Platform",
  slug: "acme",
  state: "draft",
  version_strategy: "independent_api_tracks",
  mcp_policy: "Stateless MCPv2 Only",
  generated_by: "ai_product_builder",
  source_build_id: "build_acme",
  revision: 0,
  created_at: "2026-08-19T12:00:00Z",
  updated_at: "2026-08-19T12:00:08Z",
  product_bindings: [],
  validation: [],
  components: [
    {
      id: "component_voice",
      kind: "api",
      name: "Voice API",
      slug: "voice",
      version_strategy: "independent",
      releases: [{ id: "release_voice-v3", version: "v3", state: "draft", bindings: [
        { id: "binding_voice_spec", kind: "openapi", name: "Voice OpenAPI", location: "api.acme.dev/voice/v3/openapi.yaml", version: "v3", scope: "api_release", confidence: 0.99, evidence: ["OpenAPI title matches Voice API", "Explicit /v3 path"], verified: true },
        { id: "binding_voice_docs", kind: "docs", name: "Voice documentation", location: "docs.acme.dev/voice/v3", version: "v3", scope: "api_release", confidence: 0.97, evidence: ["Versioned documentation path"], verified: true },
        { id: "binding_voice_package", kind: "package", name: "@acme/voice-node", location: "npm:@acme/voice-node@7.2.1", version: "7.2.1", scope: "api_release", confidence: 0.96, evidence: ["Package metadata declares Voice API v3"], verified: true },
        { id: "binding_voice_tools", kind: "mcp", name: "voice.calls.*", location: "mcp.acme.dev/v2", version: "2026-07-28", scope: "api_release", confidence: 0.94, evidence: ["Tool namespace matches Voice API"], verified: true },
      ] }],
    },
    {
      id: "component_messages",
      kind: "api",
      name: "Messages API",
      slug: "messages",
      version_strategy: "independent",
      releases: [{ id: "release_messages-v2", version: "v2", state: "draft", bindings: [
        { id: "binding_messages_spec", kind: "openapi", name: "Messages OpenAPI", location: "api.acme.dev/messages/v2/openapi.yaml", version: "v2", scope: "api_release", confidence: 0.99, evidence: ["OpenAPI title matches Messages API", "Explicit /v2 path"], verified: true },
        { id: "binding_messages_docs", kind: "docs", name: "Messages documentation", location: "docs.acme.dev/messages/v2", version: "v2", scope: "api_release", confidence: 0.97, evidence: ["Versioned documentation path"], verified: true },
        { id: "binding_messages_package", kind: "package", name: "@acme/messages", location: "npm:@acme/messages@5.1.3", version: "5.1.3", scope: "api_release", confidence: 0.96, evidence: ["Package metadata declares Messages API v2"], verified: true },
      ] }],
    },
  ],
  profiles: [{ id: "profile_communications_202608", name: "Voice v3 + Messages v2", state: "draft", selections: [{ component_id: "component_voice", release_id: "release_voice-v3" }, { component_id: "component_messages", release_id: "release_messages-v2" }] }],
};

const fixtureProductBuild: APIProductBuild = {
  id: "build_acme",
  organisation_id: "org_acme",
  product_id: "prod_acme",
  state: "review",
  analysis_mode: "ai_assisted",
  inputs: [
    { kind: "openapi", name: "Voice OpenAPI", location: "https://api.acme.dev/voice/v3/openapi.yaml", version: "v3" },
    { kind: "docs", name: "Voice documentation", location: "https://docs.acme.dev/voice/v3", version: "v3" },
    { kind: "package", name: "@acme/voice-node", location: "npm:@acme/voice-node@7.2.1", version: "7.2.1" },
    { kind: "openapi", name: "Messages OpenAPI", location: "https://api.acme.dev/messages/v2/openapi.yaml", version: "v2" },
    { kind: "docs", name: "Messages documentation", location: "https://docs.acme.dev/messages/v2", version: "v2" },
    { kind: "package", name: "@acme/messages", location: "npm:@acme/messages@5.1.3", version: "5.1.3" },
    { kind: "mcp", name: "Acme tools", location: "https://mcp.acme.dev/v2", version: "2026-07-28" },
  ],
  proposal: fixtureDefinition,
  unresolved: [],
  created_at: "2026-08-19T12:00:00Z",
  completed_at: "2026-08-19T12:00:08Z",
};

function CopyButton({ text, label, disabled = false, onCopied }: { text: string; label: string; disabled?: boolean; onCopied: (label: string) => void }) {
  async function copy() {
    if (disabled) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
    onCopied(label);
  }

  return <Button outline className="full" disabled={disabled} onClick={copy}><Copy data-slot="icon" />{label}</Button>;
}

export function ConsoleApp({ currentUser, currentProduct, onLogout }: { currentUser?: APIUser | null; currentProduct?: APIProduct | null; onLogout?: () => void | Promise<void> }) {
	const product = currentProduct ?? fixtureProduct;
  const [section, setSection] = useState<Section>("distribution");
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtureDefinition);
  const [latestProductBuild, setLatestProductBuild] = useState<APIProductBuild | null>(fixtureProductBuild);
  const [productBuilderOpen, setProductBuilderOpen] = useState(false);
  const [productBuilderBusy, setProductBuilderBusy] = useState(false);
  const [productBuilderInputs, setProductBuilderInputs] = useState("");
  const [sources, setSources] = useState(initialSources);
  const [packages, setPackages] = useState(initialPackages);
  const [tools, setTools] = useState(initialTools);
  const [mcpConnections, setMCPConnections] = useState<APIMCPConnection[]>(fixtureMCPConnections);
  const [mcpConnectionOpen, setMCPConnectionOpen] = useState(false);
  const [mcpImportOpen, setMCPImportOpen] = useState(false);
  const [mcpBusy, setMCPBusy] = useState(false);
  const [mcpCatalog, setMCPCatalog] = useState<APIMCPCatalog | null>(null);
  const [mcpSelectedTools, setMCPSelectedTools] = useState<string[]>([]);
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
  const [mcpEntitlements, setMCPEntitlements] = useState("");
  const [mcpConfirmationRequired, setMCPConfirmationRequired] = useState(true);
  const [publicMCPEnabled, setPublicMCPEnabled] = useState(false);
  const [distribution, setDistribution] = useState<Distribution | null>(null);
  const [widgetSnippets, setWidgetSnippets] = useState<APIWidgetSnippets | null>(null);
  const [pendingPublication, setPendingPublication] = useState<PendingPublication | null>(null);
  const [pendingMCPEnable, setPendingMCPEnable] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [resourceFilter, setResourceFilter] = useState<"all" | "public" | "private">("all");
  const [productRevision, setProductRevision] = useState(1);
  const [apiConnected, setAPIConnected] = useState(false);
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [sourceName, setSourceName] = useState("");
  const [sourceKind, setSourceKind] = useState("website");
  const [sourceLocation, setSourceLocation] = useState("");
  const [sourceBusy, setSourceBusy] = useState(false);
  const [addPackageOpen, setAddPackageOpen] = useState(false);
  const [packageName, setPackageName] = useState("");
  const [packageVersion, setPackageVersion] = useState("");
  const [packageEcosystem, setPackageEcosystem] = useState("npm");
  const [packageMode, setPackageMode] = useState<"public" | "proxy" | "fetch">("proxy");
  const [packageEndpoint, setPackageEndpoint] = useState("");
  const [packageCredential, setPackageCredential] = useState("");
  const [packageChecksum, setPackageChecksum] = useState("");
  const [packageBusy, setPackageBusy] = useState(false);
  const [addToolOpen, setAddToolOpen] = useState(false);
  const [toolNamespace, setToolNamespace] = useState("projects");
  const [toolName, setToolName] = useState("");
  const [toolDescription, setToolDescription] = useState("");
  const [toolMethod, setToolMethod] = useState("POST");
  const [toolHook, setToolHook] = useState("");
  const [toolCredential, setToolCredential] = useState("");
  const [toolEntitlements, setToolEntitlements] = useState("");
  const [toolInputSchema, setToolInputSchema] = useState(`{"type":"object","additionalProperties":false,"properties":{},"required":[]}`);
  const [toolOutputSchema, setToolOutputSchema] = useState(`{"type":"object","additionalProperties":false,"properties":{}}`);
	  const [toolBusy, setToolBusy] = useState(false);
	  const [analytics, setAnalytics] = useState<APIAnalytics | null>(null);
	  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
	  const [rootUsers, setRootUsers] = useState<APIUser[]>(currentUser ? [currentUser] : []);
	  const [identityOpen, setIdentityOpen] = useState(false);
	  const [identityBusy, setIdentityBusy] = useState(false);
	  const [idpIssuer, setIDPIssuer] = useState("");
	  const [idpClientID, setIDPClientID] = useState("");
	  const [idpClientSecret, setIDPClientSecret] = useState("");
	  const [idpScopes, setIDPScopes] = useState("openid, profile, email");
	  const [idpAudience, setIDPAudience] = useState("");
	  const [idpOrganisationClaim, setIDPOrganisationClaim] = useState("org_id");
	  const [idpEntitlementHook, setIDPEntitlementHook] = useState("");
	  const [idpAuthorizationHook, setIDPAuthorizationHook] = useState("");
	  const [idpAuthorizationCredential, setIDPAuthorizationCredential] = useState("");
	  const [idpRedirects, setIDPRedirects] = useState("");
	  const [rootOpen, setRootOpen] = useState(false);
	  const [rootBusy, setRootBusy] = useState(false);
	  const [rootEmail, setRootEmail] = useState("");
	  const [rootDisplayName, setRootDisplayName] = useState("");
	  const [rootPassword, setRootPassword] = useState("");
	  const [rootCode, setRootCode] = useState("");
	  const [rootEnrollment, setRootEnrollment] = useState<SetupEnrollment | null>(null);
	  const [rootRecoveryCodes, setRootRecoveryCodes] = useState<string[]>([]);
	  const [providers, setProviders] = useState<APIProvider[]>([]);
	  const [projects, setProjects] = useState<APIProject[]>([]);
	  const [credentialLeases, setCredentialLeases] = useState<APICredentialLease[]>([]);
	  const [providerOpen, setProviderOpen] = useState(false);
	  const [providerBusy, setProviderBusy] = useState(false);
	  const [providerName, setProviderName] = useState("");
	  const [providerURL, setProviderURL] = useState("");
	  const [providerCredential, setProviderCredential] = useState("");
	  const [providerEntitlements, setProviderEntitlements] = useState("");
	  const [providerTTL, setProviderTTL] = useState("3600");
	  const [llmProfiles, setLLMProfiles] = useState<APILLMProfile[]>([]);
	  const [llmOpen, setLLMOpen] = useState(false);
	  const [llmBusy, setLLMBusy] = useState(false);
	  const [llmRole, setLLMRole] = useState("embedding");
	  const [llmProvider, setLLMProvider] = useState("openai-compatible");
	  const [llmEndpoint, setLLMEndpoint] = useState("");
	  const [llmModel, setLLMModel] = useState("");
	  const [llmCredential, setLLMCredential] = useState("");
	  const [llmDimensions, setLLMDimensions] = useState("1536");
	  const [llmInputTokens, setLLMInputTokens] = useState("8192");
	  const [llmOutputTokens, setLLMOutputTokens] = useState("1024");
	  const [llmDailyBudget, setLLMDailyBudget] = useState("1000000");
	  const [llmEnabled, setLLMEnabled] = useState(false);
	  const [environments, setEnvironments] = useState<APIEnvironment[]>([fixtureEnvironment]);
	  const [integrationRuns, setIntegrationRuns] = useState<APIIntegrationRun[]>([]);
	  const [auditEvents, setAuditEvents] = useState<APIAuditEvent[]>([]);
	  const [runOpen, setRunOpen] = useState(false);
	  const [runBusy, setRunBusy] = useState(false);
	  const [runEnvironmentID, setRunEnvironmentID] = useState("env_prod");
	  const [runOutcome, setRunOutcome] = useState("");

  useEffect(() => {
    if (
      process.env.NODE_ENV === "development" &&
      new URLSearchParams(window.location.search).get("preview") === "fixtures"
    ) {
      return;
    }

    let cancelled = false;
	    Promise.all([api.distribution(product.id), api.widgets(product.id), api.sources(product.id), api.packages(product.id), api.tools(product.id), api.mcpConnections(product.id)]).then(([distributionValue, widgetValues, remoteSources, remotePackages, remoteTools, remoteMCPConnections]) => {
      if (cancelled) return;
      setDistribution(distributionValue);
      setWidgetSnippets(widgetValues);
      setPublicMCPEnabled(distributionValue.product.public_mcp_enabled);
      setProductRevision(distributionValue.product.revision);
      setSources((current) => remoteSources.map((source) => {
        const local = current.find((item) => item.id === source.id);
        return {
          id: source.id,
          name: source.name,
          kind: source.kind,
          location: source.location,
          visibility: source.visibility,
          published: source.published,
          quarantined: source.quarantined,
          crawlState: local?.crawlState ?? "synced",
          pages: local?.pages ?? 0,
          lastCrawl: local?.lastCrawl ?? "Not crawled",
          revision: source.revision,
        };
      }));
      setPackages((current) => remotePackages.map((pkg) => {
        const local = current.find((item) => item.id === pkg.id);
        return { id: pkg.id, name: pkg.name, ecosystem: pkg.ecosystem, version: pkg.version, mode: pkg.mode, visibility: pkg.visibility, published: pkg.published, status: local?.status ?? "ready", revision: pkg.revision };
      }));
      setTools(remoteTools);
      setMCPConnections(remoteMCPConnections);
      setAPIConnected(true);
	    }).catch(() => {
      // The standalone static preview intentionally keeps its local fixture. In the
      // service deployment, same-origin session authentication hydrates live state.
	    });
	    api.analytics(product.id).then((value) => { if (!cancelled) setAnalytics(value); }).catch(() => {});
	    api.identity(product.id).then((value) => {
	      if (cancelled) return;
	      setIdentityConfig(value);
	      setIDPIssuer(value.issuer); setIDPClientID(value.client_id); setIDPScopes(value.scopes.join(", ")); setIDPAudience(value.audience); setIDPOrganisationClaim(value.organisation_claim); setIDPEntitlementHook(value.entitlement_hook_url); setIDPAuthorizationHook(value.authorization_hook_url); setIDPRedirects(value.allowed_redirect_uris.join("\n"));
	    }).catch(() => {});
	    api.rootUsers().then((value) => { if (!cancelled) setRootUsers(value); }).catch(() => {});
	    Promise.all([api.providers(product.id), api.projects(product.id), api.credentials(product.id)]).then(([providerValues, projectValues, credentialValues]) => { if (!cancelled) { setProviders(providerValues); setProjects(projectValues); setCredentialLeases(credentialValues); } }).catch(() => {});
	    api.llmProfiles(product.id).then((values) => { if (!cancelled) setLLMProfiles(values); }).catch(() => {});
	    api.productDefinition(product.id).then((value) => { if (!cancelled) setProductDefinition(value); }).catch((error) => { if (!cancelled && error instanceof APIError && error.status === 404) setProductDefinition(null); });
	    api.productBuilds(product.id).then((values) => { if (!cancelled) setLatestProductBuild(values[0] ?? null); }).catch(() => {});
	    Promise.all([api.environments(product.id), api.integrationRuns(product.id), api.auditEvents(product.organisation_id)]).then(([environmentValues, runValues, eventValues]) => {
	      if (cancelled) return;
	      setEnvironments(environmentValues);
	      setRunEnvironmentID(environmentValues.find((environment) => environment.is_production)?.id ?? environmentValues[0]?.id ?? "");
	      setIntegrationRuns(runValues);
	      setAuditEvents(eventValues);
	    }).catch(() => {});
    return () => { cancelled = true; };
  }, [product.id, product.organisation_id]);

  const publicSources = sources.filter((item) => item.visibility === "public");
  const publicPackages = packages.filter((item) => item.visibility === "public");
  const publicResourceCount = publicSources.length + publicPackages.length;
  const allResources = useMemo(() => [
    ...sources.map((item) => ({ ...item, resourceType: "source" as const, type: item.kind, detail: item.location })),
    ...packages.map((item) => ({ ...item, resourceType: "package" as const, type: `${item.ecosystem} package`, detail: `${item.mode[0].toUpperCase()}${item.mode.slice(1)} mode` })),
  ], [sources, packages]);
  const visibleResources = resourceFilter === "all" ? allResources : allResources.filter((item) => item.visibility === resourceFilter);

  function showToast(message: string) {
    setToast(message);
    window.setTimeout(() => setToast(null), 2200);
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
        : { ...fixtureProductBuild, id: fallbackBuildID, state: "review" as const, created_at: new Date().toISOString(), completed_at: new Date().toISOString(), inputs: [...fixtureProductBuild.inputs, ...additionalInputs], proposal: { ...fixtureDefinition, state: "draft" as const, source_build_id: fallbackBuildID } };
      setLatestProductBuild(value);
      setProductBuilderOpen(false);
      setProductBuilderInputs("");
      setSection("product");
      showToast(`Product draft built from ${value.inputs.length} sources. Review ${value.unresolved.length || "no"} exception${value.unresolved.length === 1 ? "" : "s"}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The product could not be built automatically.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function publishProductDefinition() {
    if (!latestProductBuild || latestProductBuild.state !== "review") return;
    setProductBuilderBusy(true);
    try {
      const publishedAt = new Date().toISOString();
      const value = apiConnected
        ? await api.publishProductBuild(product.id, latestProductBuild.id)
        : {
            ...latestProductBuild.proposal,
            state: "published" as const,
            revision: Math.max(1, latestProductBuild.proposal.revision),
            published_at: publishedAt,
            updated_at: publishedAt,
            components: latestProductBuild.proposal.components.map((component) => ({ ...component, releases: component.releases.map((release) => ({ ...release, state: "published" as const })) })),
            profiles: latestProductBuild.proposal.profiles.map((profile) => ({ ...profile, state: "published" as const })),
          };
      setProductDefinition(value);
      setLatestProductBuild({ ...latestProductBuild, state: "published", proposal: value });
      showToast("Product definition published. Existing customer version pins were not changed.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The product definition could not be published.");
    } finally {
      setProductBuilderBusy(false);
    }
  }

  async function requestVisibility(kind: "source" | "package", id: string) {
    const list = kind === "source" ? sources : packages;
    const item = list.find((candidate) => candidate.id === id);
    if (!item) return;
    if (item.visibility === "public") {
      try {
        if (apiConnected && kind === "source") {
          const updated = await api.setSourceVisibility(product.id, id, "private", item.revision, false);
          setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: updated.visibility, revision: updated.revision } : candidate));
        } else if (apiConnected && kind === "package") {
          const updated = await api.setPackageVisibility(product.id, id, "private", item.revision, false);
          setPackages((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: updated.visibility, revision: updated.revision } : candidate));
        } else if (kind === "source") setSources((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: "private" } : candidate));
        else setPackages((items) => items.map((candidate) => candidate.id === id ? { ...candidate, visibility: "private" } : candidate));
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
      detail: kind === "source" ? "Its currently published knowledge will become anonymously searchable." : "Its metadata and downloads may be accessed anonymously.",
    });
  }

  async function confirmPublication() {
    if (!pendingPublication || !acknowledged) return;
    const { kind, id, name } = pendingPublication;
    const current = kind === "source" ? sources.find((item) => item.id === id) : packages.find((item) => item.id === id);
    if (!current) return;
    try {
      if (apiConnected && kind === "source") {
        const updated = await api.setSourceVisibility(product.id, id, "public", current.revision, true);
        setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: updated.visibility, revision: updated.revision } : item));
      } else if (apiConnected && kind === "package") {
        const updated = await api.setPackageVisibility(product.id, id, "public", current.revision, true);
        setPackages((items) => items.map((item) => item.id === id ? { ...item, visibility: updated.visibility, revision: updated.revision } : item));
      } else if (kind === "source") setSources((items) => items.map((item) => item.id === id ? { ...item, visibility: "public" } : item));
      else setPackages((items) => items.map((item) => item.id === id ? { ...item, visibility: "public" } : item));
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

  async function createSource() {
    setSourceBusy(true);
    try {
      const created = apiConnected
        ? await api.createSource(product.id, product.organisation_id, sourceName, sourceKind, sourceLocation)
        : { id: `src_${Date.now()}`, name: sourceName, kind: sourceKind, location: sourceLocation, visibility: "private" as const, published: false, quarantined: false, revision: 1 };
      setSources((items) => [...items, { id: created.id, name: created.name, kind: created.kind, location: created.location, visibility: created.visibility, published: created.published, quarantined: created.quarantined, crawlState: "review", pages: 0, lastCrawl: "Not crawled", revision: created.revision }]);
      setAddSourceOpen(false);
      setSourceName("");
      setSourceLocation("");
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
	  const crawlState: Source["crawlState"] = latest.state === "failed" ? "failed" : latest.state === "review" || latest.state === "succeeded" ? "review" : latest.state === "running" ? "running" : "queued";
	  setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState, pages: latest.fetched_count, lastCrawl: latest.finished_at ? new Date(latest.finished_at).toLocaleString() : latest.state } : item));
	  if ((latest.state === "queued" || latest.state === "running") && attempt < 40) {
		window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
		return;
	  }
	  if (latest.state === "review" || latest.state === "succeeded") {
		const refreshed = (await api.sources(product.id)).find((source) => source.id === id);
		if (refreshed) setSources((items) => items.map((item) => item.id === id ? { ...item, revision: refreshed.revision, published: refreshed.published, quarantined: refreshed.quarantined, crawlState: refreshed.quarantined ? "review" : item.crawlState } : item));
	  }
	} catch {
	  if (attempt < 5) window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
	}
  }

  async function publishSource(source: Source) {
	try {
	  const value = apiConnected ? await api.publishSource(product.id, source.id, source.revision) : { ...source, published: true, revision: source.revision + 1 };
	  setSources((items) => items.map((item) => item.id === source.id ? { ...item, published: value.published, revision: value.revision, crawlState: "synced" } : item));
	  showToast(`${source.name} was atomically published.`);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not publish source.");
	}
  }

  async function publishPackage(pkg: ProductPackage) {
	try {
	  const value = apiConnected ? await api.publishPackage(product.id, pkg.id, pkg.revision) : { ...pkg, published: true, revision: pkg.revision + 1 };
	  setPackages((items) => items.map((item) => item.id === pkg.id ? { ...item, published: value.published, revision: value.revision, status: "ready" } : item));
	  showToast(`${pkg.name} was published.`);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not publish package.");
	}
  }

  async function createPackage() {
    setPackageBusy(true);
    try {
      const created = apiConnected
        ? await api.createPackage(product.id, { organisation_id: product.organisation_id, name: packageName, ecosystem: packageEcosystem, version: packageVersion, mode: packageMode, upstream_url: packageMode === "proxy" || packageMode === "public" ? packageEndpoint : "", fetch_hook_url: packageMode === "fetch" ? packageEndpoint : "", credential: packageMode === "public" ? "" : packageCredential, checksum_sha256: packageChecksum, expected_size: 0 })
        : { id: `pkg_${Date.now()}`, name: packageName, ecosystem: packageEcosystem, version: packageVersion, mode: packageMode, visibility: "private" as const, published: false, revision: 1 };
      setPackages((items) => [...items, { id: created.id, name: created.name, ecosystem: created.ecosystem, version: created.version, mode: created.mode, visibility: created.visibility, published: created.published, status: "checking", revision: created.revision }]);
      setAddPackageOpen(false);
      setPackageName(""); setPackageVersion(""); setPackageEndpoint(""); setPackageCredential(""); setPackageChecksum("");
      showToast(`${created.name} was added privately; credentials remain server-side.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not add package.");
    } finally {
      setPackageBusy(false);
    }
  }

  async function createTool() {
    setToolBusy(true);
    try {
      const inputSchema = JSON.parse(toolInputSchema) as Record<string, unknown>;
      const outputSchema = JSON.parse(toolOutputSchema) as Record<string, unknown>;
      const authorizationPolicy = { required_entitlements: toolEntitlements.split(",").map((value) => value.trim()).filter(Boolean), confirmation_required: false };
      const created = apiConnected ? await api.createTool(product.id, { organisation_id: product.organisation_id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, api_hook_url: toolHook, http_method: toolMethod, credential: toolCredential, authorization_policy: authorizationPolicy, timeout_ms: 10000 }) : { id: `tool_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, state: "draft" as const, revision: 1, http_method: toolMethod, authorization_policy: authorizationPolicy, timeout_ms: 10000 };
      setTools((items) => [...items, created as APITool]);
      setAddToolOpen(false);
      setToolName(""); setToolDescription(""); setToolHook(""); setToolCredential(""); setToolEntitlements("");
      showToast(`${created.namespace}.${created.name} was saved as a draft.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Schema or tool configuration is invalid.");
    } finally {
      setToolBusy(false);
    }
  }

  async function publishTool(tool: APITool) {
    try {
      const updated = apiConnected ? await api.publishTool(product.id, tool.id, tool.revision) : { ...tool, state: "published" as const, revision: tool.revision + 1 };
      setTools((items) => items.map((item) => item.id === tool.id ? updated : item));
      showToast(`${updated.namespace}.${updated.name} is published to Private MCP.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not publish tool.");
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
      const entitlements = mcpEntitlements.split(",").map((value) => value.trim()).filter(Boolean);
      if (apiConnected) {
        const result = await api.importMCPTools(product.id, mcpCatalog.connection.id, { tool_names: mcpSelectedTools, required_entitlements: entitlements, confirmation_required: mcpConfirmationRequired, timeout_ms: 10000 });
        const changed = [...result.created, ...result.updated, ...result.unchanged, ...result.drifted];
        setTools((items) => [...items.filter((item) => !changed.some((candidate) => candidate.id === item.id)), ...changed]);
        setMCPConnections((items) => items.map((item) => item.id === result.connection.id ? result.connection : item));
      } else {
        const imported = mcpCatalog.tools.filter((item) => mcpSelectedTools.includes(item.name)).map((item, index): APITool => ({ id: `tool_mcp_${index}`, organisation_id: product.organisation_id, product_id: product.id, namespace: mcpCatalog.connection.namespace, name: item.name.replace(/[^A-Za-z0-9_]/g, "_"), description: item.description ?? item.title ?? item.name, input_schema: item.input_schema, output_schema: item.output_schema ?? {}, state: "draft", revision: 1, http_method: "MCP", authorization_policy: { required_entitlements: entitlements, confirmation_required: mcpConfirmationRequired }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: mcpCatalog.connection.id, upstream_tool_name: item.name, upstream_schema_hash: item.schema_hash }));
        setTools((items) => [...items, ...imported]);
      }
      setMCPImportOpen(false);
      setMCPEntitlements("");
      showToast(`${mcpSelectedTools.length} upstream tool${mcpSelectedTools.length === 1 ? "" : "s"} imported as reviewed drafts.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not import the selected MCP tools.");
    } finally {
      setMCPBusy(false);
    }
  }

  async function saveIdentity() {
    setIdentityBusy(true);
    try {
      const input = {
        organisation_id: product.organisation_id,
        issuer: idpIssuer,
        client_id: idpClientID,
        client_secret: idpClientSecret,
        scopes: idpScopes.split(",").map((value) => value.trim()).filter(Boolean),
        audience: idpAudience,
        organisation_claim: idpOrganisationClaim,
        entitlement_hook_url: idpEntitlementHook,
		authorization_hook_url: idpAuthorizationHook,
		authorization_credential: idpAuthorizationCredential,
        allowed_redirect_uris: idpRedirects.split(/\r?\n/).map((value) => value.trim()).filter(Boolean),
      };
      const value = apiConnected ? await api.configureIdentity(product.id, input) : { id: "idp_preview", product_id: product.id, revision: 1, ...input } as APIIdentity;
      setIdentityConfig(value);
      setIDPClientSecret("");
	  setIDPAuthorizationCredential("");
      setIdentityOpen(false);
      showToast("Vendor identity and entitlement resolution are configured.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not configure vendor identity.");
    } finally {
      setIdentityBusy(false);
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

  async function createProvider() {
    setProviderBusy(true);
    try {
      const value = await api.createProvider(product.id, { organisation_id: product.organisation_id, name: providerName, base_url: providerURL, credential: providerCredential, required_entitlements: providerEntitlements.split(",").map((item) => item.trim()).filter(Boolean), max_ttl_seconds: Number(providerTTL) });
      setProviders((items) => [...items, value]);
      setProviderOpen(false);
      setProviderName(""); setProviderURL(""); setProviderCredential(""); setProviderEntitlements("");
      showToast("Provider API configured. Project and credential operations now use its authorization hook.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not configure provider API.");
    } finally {
      setProviderBusy(false);
    }
  }

  async function saveLLMProfile() {
    setLLMBusy(true);
    try {
      const value = await api.saveLLMProfile(product.id, { organisation_id: product.organisation_id, role: llmRole, provider: llmProvider, endpoint: llmEndpoint, model: llmModel, credential: llmCredential, embedding_dimensions: llmRole === "embedding" ? Number(llmDimensions) : 0, max_input_tokens: Number(llmInputTokens), max_output_tokens: Number(llmOutputTokens), daily_token_budget: Number(llmDailyBudget), enabled: llmEnabled });
      setLLMProfiles((items) => [...items.filter((item) => item.role !== value.role), value].sort((a, b) => a.role.localeCompare(b.role)));
      setLLMCredential("");
      setLLMOpen(false);
      showToast(`${value.role} model profile saved with mandatory untrusted-context hardening.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not save LLM profile.");
    } finally {
      setLLMBusy(false);
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
	  showToast("Integration run started. Deterministic validation will feed the success funnel.");
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not start integration run.");
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
	  showToast(error instanceof APIError ? error.message : "Could not complete integration run.");
	}
  }

  const publicEndpoint = distribution?.public_mcp_endpoint ?? `/mcp/public/${product.id}`;
  const publicSnippet = widgetSnippets?.public.snippet ?? `<script async src="/widgets/${product.id}/public.js" data-product="${product.id}"></script>`;
  const privateSnippet = widgetSnippets?.private.snippet ?? `<script async src="/widgets/${product.id}/private.js" data-product="${product.id}"></script>`;
  const mcpConnectionReady = Boolean(mcpName.trim() && mcpNamespace.trim() && mcpEndpoint.trim() && (mcpAuthMode !== "service" || mcpCredential.trim()) && (mcpAuthMode !== "delegated_oauth" || (mcpOAuthClientID.trim() && mcpOAuthClientSecret.trim() && mcpOAuthIssuer.trim() && mcpAuthorizationURL.trim() && mcpTokenURL.trim())));

  return (
    <div className={`app-shell${
      typeof window !== "undefined" &&
      process.env.NODE_ENV === "development" &&
      new URLSearchParams(window.location.search).get("preview") === "fixtures"
        ? " fixture-preview"
        : ""
    }`}>
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark" aria-hidden="true">D</span><span>DokoSoko</span></div>
        <nav aria-label="Main navigation">
          {nav.map((item) => {
            const Icon = item.icon;
            return <button key={item.id} type="button" className={`nav-item ${section === item.id ? "active" : ""}`} onClick={() => setSection(item.id)}><Icon /><span>{item.label}</span></button>;
          })}
        </nav>
        <div className="sidebar-bottom">
          <button type="button" className={`nav-item ${section === "settings" ? "active" : ""}`} onClick={() => setSection("settings")}><Settings /><span>Settings</span></button>
          <div className="account"><span className="avatar">{(currentUser?.display_name ?? "Yuriy Admin").split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><strong>{currentUser?.display_name ?? "Yuriy"}</strong><small>{currentUser ? "Root administrator" : "Platform admin"}</small></span>{onLogout && <button type="button" className="logout-button" aria-label="Sign out" title="Sign out" onClick={onLogout}><LogOut /></button>}</div>
        </div>
      </aside>

      <main>
        <header className="topbar">
          <button type="button" className="product-switcher"><span className="product-logo">{product.name.slice(0, 1).toUpperCase()}</span><span><small>Product</small><strong>{product.name}</strong></span><ChevronDown /></button>
          <div className="environment"><span className="status-dot" />Production</div>
        </header>

        <div className="content">
          {section === "distribution" && (
            <DistributionView
              enabled={publicMCPEnabled}
              onEnabledChange={requestMCPChange}
              resources={visibleResources}
              resourceFilter={resourceFilter}
              setResourceFilter={setResourceFilter}
              publicResourceCount={publicResourceCount}
              onVisibilityChange={requestVisibility}
              onCopied={showToast}
              publicSnippet={publicSnippet}
              privateSnippet={privateSnippet}
              publicEndpoint={publicEndpoint}
              onOpenSources={() => setSection("sources")}
            />
          )}
          {section === "product" && <ProductDefinitionView definition={productDefinition} build={latestProductBuild} busy={productBuilderBusy} onBuild={() => setProductBuilderOpen(true)} onPublish={publishProductDefinition} />}
          {section === "sources" && <SourcesView sources={sources} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} />}
          {section === "packages" && <PackagesView packages={packages} onAdd={() => setAddPackageOpen(true)} onPublish={publishPackage} onVisibilityChange={(id) => requestVisibility("package", id)} />}
          {section === "projects" && <ProjectsView providers={providers} projects={projects} credentials={credentialLeases} onAddProvider={() => setProviderOpen(true)} />}
          {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} />}
          {section === "overview" && <OverviewView productName={product.name} sourceCount={sources.length} publishedSourceCount={sources.filter((source) => source.published).length} packageCount={packages.length} credentialPackageCount={packages.filter((pkg) => pkg.mode !== "public").length} publicResourceCount={publicResourceCount} analytics={analytics} onNavigate={setSection} onStartRun={() => setRunOpen(true)} />}
          {section === "tools" && <ToolsView tools={tools} onAdd={() => setAddToolOpen(true)} onPublish={publishTool} />}
          {section === "runs" && <IntegrationRunsView runs={integrationRuns} environments={environments} onStart={() => setRunOpen(true)} onComplete={completeIntegrationRun} />}
          {section === "analytics" && <AnalyticsView publicEnabled={publicMCPEnabled} analytics={analytics} />}
          {section === "activity" && <ActivityView events={auditEvents} />}
          {section === "settings" && <SettingsView identity={identityConfig} llmProfiles={llmProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onConfigureIdentity={() => setIdentityOpen(true)} onConfigureLLM={() => setLLMOpen(true)} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} />}
        </div>
      </main>

      <Dialog
        open={Boolean(pendingPublication)}
        onClose={(open) => { if (!open) setPendingPublication(null); }}
        title={`Make ${pendingPublication?.name ?? "resource"} public?`}
        description="This is a security-sensitive publication change. Private is the default for every new source and package."
        actions={<><Button outline onClick={() => setPendingPublication(null)}>Keep private</Button><Button color="red" disabled={!acknowledged} onClick={confirmPublication}>Make public</Button></>}
      >
        <WarningContent>
          <p><strong>{pendingPublication?.detail}</strong> Public MCP and the public widget do not require users to sign in.</p>
          <p>DokoSoko will record your identity, the prior revision, and this decision in the audit log.</p>
          <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand this published {pendingPublication?.kind} will be available without authentication.</Confirmation>
        </WarningContent>
      </Dialog>

      <Dialog
        open={productBuilderOpen}
        onClose={setProductBuilderOpen}
        title="Build product automatically"
        description="DokoSoko will inspect everything already attached, infer API capabilities and versions, and create one reviewable Product Definition."
        actions={<><Button outline onClick={() => setProductBuilderOpen(false)}>Cancel</Button><Button color="indigo" disabled={productBuilderBusy} onClick={buildProductAutomatically}>{productBuilderBusy ? "Building product…" : "Build automatically"}</Button></>}
      >
        <div className="product-builder-form">
          <div className="builder-source-summary">
            <span><BookOpen /><strong>{sources.length}</strong><small>docs & specs</small></span>
            <span><PackageIcon /><strong>{packages.length}</strong><small>packages</small></span>
            <span><Share2 /><strong>{mcpConnections.length}</strong><small>MCP upstreams</small></span>
            <span><Wrench /><strong>{tools.length}</strong><small>tools</small></span>
          </div>
          <label className="auth-field"><span>Anything else?</span><textarea value={productBuilderInputs} onChange={(event) => setProductBuilderInputs(event.target.value)} placeholder={"Paste URLs, package coordinates, repositories, or MCP endpoints—one per line.\nhttps://api.example.com/voice/v3/openapi.yaml\nnpm:@acme/voice-node@7.2.1"} /><small>Optional. DokoSoko automatically classifies each input and never retrieves credentials embedded in a URL.</small></label>
          <div className="builder-magic-note"><Sparkles /><span><strong>Review exceptions, not configuration.</strong> Exact matches are joined automatically. Ambiguous version relationships stay unresolved and cannot silently fall back.</span></div>
        </div>
      </Dialog>

      <Dialog
        open={addSourceOpen}
        onClose={setAddSourceOpen}
        title="Add knowledge source"
        description="The source starts private and draft. A crawl produces immutable snapshots for review before publication."
        actions={<><Button outline onClick={() => setAddSourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={sourceBusy || !sourceName.trim() || !sourceLocation.trim()} onClick={createSource}>{sourceBusy ? "Adding…" : "Add source"}</Button></>}
      >
        <div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Developer documentation" /></label><label className="auth-field"><span>Type</span><select value={sourceKind} onChange={(event) => setSourceKind(event.target.value)}><option value="website">Website</option><option value="openapi">OpenAPI</option><option value="git">Git repository</option><option value="sdk">SDK reference</option></select></label><label className="auth-field"><span>Location</span><input type="url" value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder="https://docs.example.com" /></label><div className="private-default-note"><LockKeyhole />Private by default. Making it public is a separate guarded action.</div></div>
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
          <div className="catalog-list">{mcpCatalog?.tools.map((tool) => <label className="catalog-tool" key={tool.name}><input type="checkbox" checked={mcpSelectedTools.includes(tool.name)} onChange={(event) => setMCPSelectedTools((items) => event.target.checked ? [...items, tool.name] : items.filter((name) => name !== tool.name))} /><span className="check-box">{mcpSelectedTools.includes(tool.name) && <Check />}</span><span><strong>{tool.title || tool.name}</strong><code>{tool.name}</code><small>{tool.description || "No upstream description"}</small></span><Badge color="zinc">{tool.schema_hash.slice(0, 12)}</Badge></label>)}</div>
          <label className="auth-field"><span>Required DokoSoko entitlements</span><input value={mcpEntitlements} onChange={(event) => setMCPEntitlements(event.target.value)} placeholder="support.write, developer.pro" /><small>Evaluated before every upstream call. Authorization hook failures deny execution.</small></label>
          <Switch checked={mcpConfirmationRequired} onChange={setMCPConfirmationRequired} label="Require user confirmation before execution" />
          <div className="private-default-note"><LockKeyhole />Import pins each schema hash. Published tools fail closed if a later catalog inspection detects drift.</div>
        </div>
      </Dialog>

      <Dialog
	    open={runOpen}
	    onClose={setRunOpen}
	    title="Start integration run"
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
          <p><strong>{publicResourceCount} published {publicResourceCount === 1 ? "resource is" : "resources are"} currently marked public.</strong> Private sources, private packages, API tools, projects, credentials, identities, and entitlement data remain excluded.</p>
          <p>Anonymous requests are rate-limited and logged as aggregate security events. You can turn this endpoint off immediately.</p>
          <Confirmation checked={acknowledged} onChange={setAcknowledged}>I understand Public MCP is authentication-less and exposes public resources anonymously.</Confirmation>
        </WarningContent>
      </Dialog>

      <Dialog
        open={addPackageOpen}
        onClose={setAddPackageOpen}
        title="Add package"
        description="Configure public metadata, a credential-backed proxy, or a fetch hook. Every package starts private."
        actions={<><Button outline onClick={() => setAddPackageOpen(false)}>Cancel</Button><Button color="indigo" disabled={packageBusy || !packageName.trim() || !packageVersion.trim() || (packageMode !== "public" && (!packageEndpoint.trim() || !packageCredential.trim()))} onClick={createPackage}>{packageBusy ? "Encrypting…" : "Add package"}</Button></>}
      >
        <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} placeholder="@acme/sdk" /></label><label className="auth-field"><span>Version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="1.0.0" /></label></div><div className="two-fields"><label className="auth-field"><span>Ecosystem</span><select value={packageEcosystem} onChange={(event) => setPackageEcosystem(event.target.value)}>{["npm", "go", "git", "maven", "android", "swift", "nuget"].map((value) => <option key={value} value={value}>{value}</option>)}</select></label><label className="auth-field"><span>Delivery mode</span><select value={packageMode} onChange={(event) => setPackageMode(event.target.value as "public" | "proxy" | "fetch")}><option value="public">Public link</option><option value="proxy">Proxy</option><option value="fetch">Fetch hook</option></select></label></div><label className="auth-field"><span>{packageMode === "fetch" ? "Fetch hook URL" : "Upstream artifact URL"}</span><input type="url" value={packageEndpoint} onChange={(event) => setPackageEndpoint(event.target.value)} placeholder="https://packages.example.com/artifact" /></label>{packageMode !== "public" && <label className="auth-field"><span>Upstream credential</span><input type="password" autoComplete="off" value={packageCredential} onChange={(event) => setPackageCredential(event.target.value)} /><small>Encrypted before storage and never returned to the browser or agent.</small></label>}<label className="auth-field"><span>Expected SHA-256 (optional)</span><input value={packageChecksum} onChange={(event) => setPackageChecksum(event.target.value)} placeholder="64 hexadecimal characters" /></label><div className="private-default-note"><LockKeyhole />Proxy and fetch packages can only become anonymous through a separate confirmed publication.</div></div>
      </Dialog>

      <Dialog
        open={addToolOpen}
        onClose={setAddToolOpen}
        title="Create API tool"
        description="Define the MCP contract and one fixed API action. Agents cannot alter the host or authorization header."
        actions={<><Button outline onClick={() => setAddToolOpen(false)}>Cancel</Button><Button color="indigo" disabled={toolBusy || !toolName.trim() || !toolDescription.trim() || !toolHook.trim()} onClick={createTool}>{toolBusy ? "Validating…" : "Save draft"}</Button></>}
      >
        <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Namespace</span><input value={toolNamespace} onChange={(event) => setToolNamespace(event.target.value)} /></label><label className="auth-field"><span>Tool name</span><input value={toolName} onChange={(event) => setToolName(event.target.value)} placeholder="create_sandbox" /></label></div><label className="auth-field"><span>Description</span><input value={toolDescription} onChange={(event) => setToolDescription(event.target.value)} /></label><label className="auth-field"><span>Input JSON Schema</span><textarea value={toolInputSchema} onChange={(event) => setToolInputSchema(event.target.value)} /></label><label className="auth-field"><span>Output JSON Schema</span><textarea value={toolOutputSchema} onChange={(event) => setToolOutputSchema(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Method</span><select value={toolMethod} onChange={(event) => setToolMethod(event.target.value)}>{["GET", "POST", "PUT", "PATCH", "DELETE"].map((value) => <option key={value}>{value}</option>)}</select></label><label className="auth-field"><span>Fixed API hook</span><input type="url" value={toolHook} onChange={(event) => setToolHook(event.target.value)} placeholder="https://api.vendor.com/v1/action" /></label></div><label className="auth-field"><span>API credential (optional)</span><input type="password" autoComplete="off" value={toolCredential} onChange={(event) => setToolCredential(event.target.value)} /></label><label className="auth-field"><span>Required entitlements</span><input value={toolEntitlements} onChange={(event) => setToolEntitlements(event.target.value)} placeholder="sandboxes.create, developer.pro" /><small>Comma-separated vendor entitlement keys. Missing or unavailable entitlements fail closed.</small></label></div>
      </Dialog>

      <Dialog
        open={identityOpen}
        onClose={setIdentityOpen}
        title="Vendor identity & entitlements"
        description={`DokoSoko brokers OAuth for this product. The downstream OAuth client_id is the product ID: ${product.id}`}
        actions={<><Button outline onClick={() => setIdentityOpen(false)}>Cancel</Button><Button color="indigo" disabled={identityBusy || !idpIssuer.trim() || !idpClientID.trim() || (!identityConfig && !idpClientSecret.trim()) || !idpRedirects.trim()} onClick={saveIdentity}>{identityBusy ? "Verifying…" : "Save identity"}</Button></>}
      >
        <div className="auth-form compact-form">
          <label className="auth-field"><span>OIDC issuer</span><input type="url" value={idpIssuer} onChange={(event) => setIDPIssuer(event.target.value)} placeholder="https://identity.vendor.com" /></label>
          <div className="two-fields"><label className="auth-field"><span>OIDC client ID</span><input value={idpClientID} onChange={(event) => setIDPClientID(event.target.value)} /></label><label className="auth-field"><span>{identityConfig ? "Rotate client secret (optional)" : "OIDC client secret"}</span><input type="password" autoComplete="off" value={idpClientSecret} onChange={(event) => setIDPClientSecret(event.target.value)} /></label></div>
          <label className="auth-field"><span>Scopes</span><input value={idpScopes} onChange={(event) => setIDPScopes(event.target.value)} /></label>
          <div className="two-fields"><label className="auth-field"><span>Audience (optional)</span><input value={idpAudience} onChange={(event) => setIDPAudience(event.target.value)} /></label><label className="auth-field"><span>Organisation claim</span><input value={idpOrganisationClaim} onChange={(event) => setIDPOrganisationClaim(event.target.value)} /></label></div>
          <label className="auth-field"><span>Entitlement hook</span><input type="url" value={idpEntitlementHook} onChange={(event) => setIDPEntitlementHook(event.target.value)} placeholder="https://api.vendor.com/dokosoko/entitlements" /><small>The vendor returns enabled and disabled feature keys during login. Hook errors deny private access.</small></label>
          <div className="two-fields"><label className="auth-field"><span>Per-operation authorization hook</span><input type="url" value={idpAuthorizationHook} onChange={(event) => setIDPAuthorizationHook(event.target.value)} placeholder="https://api.vendor.com/dokosoko/authorize" /></label><label className="auth-field"><span>{identityConfig?.authorization_hook_url ? "Rotate authorization credential" : "Authorization credential"}</span><input type="password" autoComplete="off" value={idpAuthorizationCredential} onChange={(event) => setIDPAuthorizationCredential(event.target.value)} /></label></div>
          <label className="auth-field"><span>Allowed downstream redirect URIs</span><textarea value={idpRedirects} onChange={(event) => setIDPRedirects(event.target.value)} placeholder={"https://developer.vendor.com/dokosoko/callback\nhttp://localhost:3000/oauth/callback"} /><small>One exact URI per line. Wildcards are not accepted.</small></label>
          <div className="private-default-note"><ShieldCheck />Login-time entitlements control discovery; the per-operation hook reauthorizes custom tool execution. Both fail closed without exposing credentials.</div>
        </div>
      </Dialog>

      <Dialog
        open={rootOpen}
        onClose={setRootOpen}
        title="Add root administrator"
        description="Every root administrator has a unique strong password, TOTP enrollment, recovery codes, and independently revocable sessions."
        actions={rootRecoveryCodes.length ? <Button color="indigo" onClick={() => { setRootOpen(false); setRootRecoveryCodes([]); setRootEmail(""); setRootDisplayName(""); setRootPassword(""); }}>I saved the recovery codes</Button> : rootEnrollment ? <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || rootCode.length !== 6} onClick={completeRootUser}>{rootBusy ? "Verifying…" : "Create root"}</Button></> : <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || !rootEmail.trim() || !rootDisplayName.trim() || rootPassword.length < 14} onClick={beginRootUser}>{rootBusy ? "Preparing…" : "Continue to MFA"}</Button></>}
      >
        {rootRecoveryCodes.length ? <div className="auth-form compact-form"><div className="private-default-note"><ShieldCheck />These one-time recovery codes are shown once. Store them in a secure password manager.</div><div className="recovery-grid">{rootRecoveryCodes.map((code) => <code key={code}>{code}</code>)}</div></div> : rootEnrollment ? <div className="auth-form compact-form"><label className="auth-field"><span>Authenticator secret</span><input readOnly value={rootEnrollment.totp_secret} onFocus={(event) => event.currentTarget.select()} /><small>Add this secret to the new administrator&apos;s authenticator. Enrollment expires in 15 minutes.</small></label><label className="auth-field"><span>6-digit verification code</span><input inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={rootCode} onChange={(event) => setRootCode(event.target.value.replace(/\D/g, ""))} /></label></div> : <div className="auth-form compact-form"><label className="auth-field"><span>Email</span><input type="email" value={rootEmail} onChange={(event) => setRootEmail(event.target.value)} /></label><label className="auth-field"><span>Display name</span><input value={rootDisplayName} onChange={(event) => setRootDisplayName(event.target.value)} /></label><label className="auth-field"><span>Initial password</span><input type="password" autoComplete="new-password" value={rootPassword} onChange={(event) => setRootPassword(event.target.value)} /><small>At least 14 characters with upper/lower-case, a number, and a symbol.</small></label></div>}
      </Dialog>

      <Dialog
        open={providerOpen}
        onClose={setProviderOpen}
        title="Connect Provider API"
        description="Use the standard DokoSoko Provider contract for authorization, project creation, short-lived credential issuance, and revocation."
        actions={<><Button outline onClick={() => setProviderOpen(false)}>Cancel</Button><Button color="indigo" disabled={providerBusy || !providerName.trim() || !providerURL.trim() || !providerCredential.trim()} onClick={createProvider}>{providerBusy ? "Encrypting…" : "Connect provider"}</Button></>}
      >
        <div className="auth-form compact-form"><label className="auth-field"><span>Provider name</span><input value={providerName} onChange={(event) => setProviderName(event.target.value)} placeholder="Production developer platform" /></label><label className="auth-field"><span>Fixed HTTPS base URL</span><input type="url" value={providerURL} onChange={(event) => setProviderURL(event.target.value)} placeholder="https://api.vendor.com/dokosoko" /></label><label className="auth-field"><span>Service credential</span><input type="password" autoComplete="off" value={providerCredential} onChange={(event) => setProviderCredential(event.target.value)} /><small>Encrypted server-side. DokoSoko never forwards an end-user access token to provider operations.</small></label><label className="auth-field"><span>Required vendor entitlements</span><input value={providerEntitlements} onChange={(event) => setProviderEntitlements(event.target.value)} placeholder="developer.pro, projects.create" /></label><label className="auth-field"><span>Maximum lease TTL (seconds)</span><input type="number" min={300} max={86400} value={providerTTL} onChange={(event) => setProviderTTL(event.target.value)} /></label><div className="private-default-note"><ShieldCheck />Every mutation first calls POST /v1/authorize and fails closed. Credentials are returned once; only their fingerprint and lease metadata are retained.</div></div>
      </Dialog>

      <Dialog
        open={llmOpen}
        onClose={setLLMOpen}
        title="Configure LLM profile"
        description="Models are optional accelerators for embedding, extraction, reranking, evaluation, or assistance. They never authorize tools or choose network destinations."
        actions={<><Button outline onClick={() => setLLMOpen(false)}>Cancel</Button><Button color="indigo" disabled={llmBusy || !llmEndpoint.trim() || !llmModel.trim() || (llmEnabled && !llmCredential.trim() && !llmProfiles.some((profile) => profile.role === llmRole))} onClick={saveLLMProfile}>{llmBusy ? "Validating…" : "Save profile"}</Button></>}
      >
        <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Role</span><select value={llmRole} onChange={(event) => setLLMRole(event.target.value)}>{["embedding", "extraction", "reranking", "evaluation", "assistant"].map((role) => <option key={role}>{role}</option>)}</select></label><label className="auth-field"><span>Provider type</span><input value={llmProvider} onChange={(event) => setLLMProvider(event.target.value)} /></label></div><label className="auth-field"><span>Fixed HTTPS endpoint</span><input type="url" value={llmEndpoint} onChange={(event) => setLLMEndpoint(event.target.value)} placeholder="https://api.provider.com/v1" /></label><div className="two-fields"><label className="auth-field"><span>Model</span><input value={llmModel} onChange={(event) => setLLMModel(event.target.value)} /></label>{llmRole === "embedding" && <label className="auth-field"><span>Embedding dimensions</span><input type="number" min={64} max={8192} value={llmDimensions} onChange={(event) => setLLMDimensions(event.target.value)} /></label>}</div><label className="auth-field"><span>Provider credential</span><input type="password" autoComplete="off" value={llmCredential} onChange={(event) => setLLMCredential(event.target.value)} /><small>Required when enabling a new profile; leave blank on an existing role to retain its encrypted credential.</small></label><div className="two-fields"><label className="auth-field"><span>Max input tokens</span><input type="number" value={llmInputTokens} onChange={(event) => setLLMInputTokens(event.target.value)} /></label><label className="auth-field"><span>Max output tokens</span><input type="number" value={llmOutputTokens} onChange={(event) => setLLMOutputTokens(event.target.value)} /></label></div><label className="auth-field"><span>Daily token budget</span><input type="number" value={llmDailyBudget} onChange={(event) => setLLMDailyBudget(event.target.value)} /></label><Switch checked={llmEnabled} onChange={setLLMEnabled} label="Enable this profile" /><div className="private-default-note"><ShieldCheck />Mandatory: context is untrusted, model tool calls and authorization decisions are disabled, citations are required, and low-confidence retrieval returns no answer.</div></div>
      </Dialog>

      {toast && <div className="toast" role="status"><Check />{toast}</div>}
    </div>
  );
}

function PageHeading({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) {
  return <div className="page-heading"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>{action}</div>;
}

function DistributionView({
  enabled,
  onEnabledChange,
  resources,
  resourceFilter,
  setResourceFilter,
  publicResourceCount,
  onVisibilityChange,
  onCopied,
  publicSnippet,
  privateSnippet,
  publicEndpoint,
  onOpenSources,
}: {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  resources: Array<{ id: string; name: string; resourceType: "source" | "package"; type: string; detail: string; visibility: Visibility }>;
  resourceFilter: "all" | "public" | "private";
  setResourceFilter: (filter: "all" | "public" | "private") => void;
  publicResourceCount: number;
  onVisibilityChange: (kind: "source" | "package", id: string) => void;
  onCopied: (label: string) => void;
  publicSnippet: string;
  privateSnippet: string;
  publicEndpoint: string;
  onOpenSources: () => void;
}) {
  return <>
    <PageHeading eyebrow="Distribution" title="MCP & widgets" description="Control how agents and developers access your product knowledge." action={<Button outline><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>Public MCP</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? "Live" : "Off"}</Badge></div><p>Offer an authentication-free, read-only MCP endpoint. Its server-side policy can retrieve only published sources and packages that you explicitly mark public.</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label="Copy public MCP endpoint" onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied("Public MCP endpoint copied."); }}><Copy />Copy</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label="Enable Public MCP" /><small>{enabled ? "Accepting anonymous requests" : "Disabled by default"}</small></div>
    </section>

    <section className="section-block">
      <div className="section-heading"><div><h2>Resource visibility</h2><p>{publicResourceCount} public. Private is the default; changing to public always requires confirmation.</p></div><Button outline onClick={onOpenSources}>Manage sources</Button></div>
      <div className="filter-tabs" role="group" aria-label="Filter resources">
        {(["all", "public", "private"] as const).map((filter) => <button type="button" key={filter} className={resourceFilter === filter ? "active" : ""} onClick={() => setResourceFilter(filter)}>{filter[0].toUpperCase() + filter.slice(1)}</button>)}
      </div>
      <div className="resource-table">
        <div className="table-head resource-columns"><span>Resource</span><span>Type</span><span>Visibility</span><span /></div>
        {resources.map((resource) => <div className="table-row resource-columns" key={`${resource.resourceType}-${resource.id}`}>
          <span className="resource-name"><span className="resource-icon">{resource.resourceType === "source" ? <BookOpen /> : <PackageIcon />}</span><span><strong>{resource.name}</strong><small>{resource.detail}</small></span></span>
          <span>{resource.type}</span>
          <span className="visibility-control"><Badge color={resource.visibility === "public" ? "green" : "zinc"}>{resource.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{resource.visibility[0].toUpperCase() + resource.visibility.slice(1)}</Badge><Switch checked={resource.visibility === "public"} onChange={() => onVisibilityChange(resource.resourceType, resource.id)} label={`Make ${resource.name} ${resource.visibility === "public" ? "private" : "public"}`} /></span>
          <button type="button" className="more" aria-label={`Actions for ${resource.name}`}><MoreHorizontal /></button>
        </div>)}
        {resources.length === 0 && <div className="empty-row">No resources match this filter.</div>}
      </div>
    </section>

    <section className="section-block widgets-section">
      <div className="section-heading"><div><h2>Copy widget</h2><p>Embed product guidance in your developer portal or application. Snippets never contain a secret.</p></div></div>
      <div className="widget-grid">
        <article className={`widget-card ${!enabled ? "widget-disabled" : ""}`}>
          <WidgetPreview kind="public" />
          <div className="widget-copy"><Badge color="blue"><Globe2 />Public</Badge><h3>Public widget</h3><p>No sign-in. Answers only from public, published sources and packages.</p>{!enabled && <div className="inline-warning"><TriangleAlert />Enable Public MCP before embedding.</div>}<CopyButton text={publicSnippet} label="Copy public widget" disabled={!enabled} onCopied={onCopied} /></div>
        </article>
        <article className="widget-card">
          <WidgetPreview kind="private" />
          <div className="widget-copy"><Badge color="violet"><LockKeyhole />Private</Badge><h3>Private widget</h3><p>Uses your identity flow for private knowledge, tools, packages, projects, and credentials.</p><CopyButton text={privateSnippet} label="Copy private widget" onCopied={onCopied} /></div>
        </article>
      </div>
    </section>
  </>;
}

function WidgetPreview({ kind }: { kind: "public" | "private" }) {
  const privateWidget = kind === "private";
  return <div className={`widget-preview ${privateWidget ? "dark-preview" : ""}`}><div className="mini-chat"><span className={`mini-brand ${privateWidget ? "light" : ""}`}>D</span><span><strong>{privateWidget ? "Acme developer assistant" : "Ask Acme"}</strong><small>{privateWidget ? "Signed in as Alex" : "Powered by DokoSoko"}</small></span><button type="button" aria-label="Close widget preview">×</button></div><div className={`mini-message ${privateWidget ? "dark-message" : ""}`}>{privateWidget ? "Show my sandbox credentials" : "How do I create an API key?"}</div><div className={`mini-answer ${privateWidget ? "dark-answer" : ""}`}>{privateWidget ? "I can provision credentials after checking your access." : "I can help with Acme's public documentation and packages."}</div><div className={`mini-input ${privateWidget ? "dark-input" : ""}`}>Ask a question… <span>↑</span></div></div>;
}

function SourcesView({ sources, onAdd, onCrawl, onPublish, onVisibilityChange }: { sources: Source[]; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void }) {
  return <>
    <PageHeading eyebrow="Knowledge" title="Sources" description="Manage ingestion, crawl state, review, publication, and anonymous visibility." action={<Button onClick={onAdd}><Plus data-slot="icon" />Add source</Button>} />
    <div className="summary-strip"><SummaryItem label="Pages indexed" value="378" icon={<Database />} /><SummaryItem label="Healthy sources" value="1 of 3" icon={<CheckCircle2 />} /><SummaryItem label="Needs attention" value="2" icon={<AlertCircle />} /></div>
    <div className="toolbar"><div className="search-field"><Search /><input aria-label="Search sources" placeholder="Search sources…" /></div><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />Crawl all</Button></div>
    <div className="resource-table">
      <div className="table-head source-columns"><span>Source</span><span>Crawl state</span><span>Content</span><span>Visibility</span><span /></div>
      {sources.map((source) => <div className="table-row source-columns" key={source.id}>
        <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><strong>{source.name}</strong><small>{source.location} · {source.kind}</small></span></span>
        <span><CrawlBadge state={source.crawlState} /><small className="cell-note">{source.lastCrawl}</small></span>
        <span><strong className="cell-value">{source.pages}</strong><small className="cell-note">pages</small></span>
        <span className="visibility-control"><Badge color={source.visibility === "public" ? "green" : "zinc"}>{source.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{source.visibility}</Badge><Switch checked={source.visibility === "public"} onChange={() => onVisibilityChange(source.id)} label={`Make ${source.name} ${source.visibility === "public" ? "private" : "public"}`} /></span>
        <span className="table-actions">{!source.published && source.crawlState === "review" && !source.quarantined && <Button outline onClick={() => onPublish(source)}>Publish</Button>}<button type="button" className="more" aria-label={`Crawl ${source.name}`} title="Queue crawl" onClick={() => onCrawl(source.id)}><RefreshCw /></button></span>
      </div>)}
    </div>
  </>;
}

function CrawlBadge({ state }: { state: Source["crawlState"] }) {
  if (state === "queued" || state === "running") return <Badge color="blue"><RefreshCw />{state}</Badge>;
  if (state === "synced") return <Badge color="green"><CheckCircle2 />Synced</Badge>;
  if (state === "review") return <Badge color="amber"><Clock3 />Needs review</Badge>;
  return <Badge color="red"><XCircle />Failed</Badge>;
}

function PackagesView({ packages, onAdd, onPublish, onVisibilityChange }: { packages: ProductPackage[]; onAdd: () => void; onPublish: (pkg: ProductPackage) => void; onVisibilityChange: (id: string) => void }) {
  return <>
    <PageHeading eyebrow="Distribution" title="Packages" description="Manage public packages and credential-backed proxy or fetch delivery." action={<Button onClick={onAdd}><Plus data-slot="icon" />Add package</Button>} />
    <div className="notice"><ShieldCheck /><span><strong>Credentials stay server-side.</strong> Proxy and fetch modes stream artifacts without exposing persistent upstream tokens.</span></div>
    <div className="resource-table">
      <div className="table-head package-columns"><span>Package</span><span>Ecosystem</span><span>Delivery</span><span>Visibility</span><span /></div>
      {packages.map((pkg) => <div className="table-row package-columns" key={pkg.id}>
        <span className="resource-name"><span className="resource-icon"><PackageIcon /></span><span><strong>{pkg.name}</strong><small>v{pkg.version}</small></span></span>
        <span>{pkg.ecosystem}</span>
        <span><Badge color={pkg.mode === "public" ? "blue" : pkg.mode === "proxy" ? "violet" : "amber"}>{pkg.mode === "proxy" ? <Radio /> : pkg.mode === "fetch" ? <ExternalLink /> : <Globe2 />}{pkg.mode}</Badge></span>
        <span className="visibility-control"><Badge color={pkg.visibility === "public" ? "green" : "zinc"}>{pkg.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{pkg.visibility}</Badge><Switch checked={pkg.visibility === "public"} onChange={() => onVisibilityChange(pkg.id)} label={`Make ${pkg.name} ${pkg.visibility === "public" ? "private" : "public"}`} /></span>
        <span className="table-actions">{!pkg.published && <Button outline onClick={() => onPublish(pkg)}>Publish</Button>}<button type="button" className="more" aria-label={`Actions for ${pkg.name}`}><MoreHorizontal /></button></span>
      </div>)}
    </div>
  </>;
}

function ProjectsView({ providers, projects, credentials, onAddProvider }: { providers: APIProvider[]; projects: APIProject[]; credentials: APICredentialLease[]; onAddProvider: () => void }) {
  return <><PageHeading eyebrow="Privileged delivery" title="Projects & credentials" description="Connect the standard Provider API; issue short-lived resources through authenticated, entitlement-scoped MCP operations." action={<Button onClick={onAddProvider}><Plus data-slot="icon" />Connect provider</Button>} /><div className="notice"><ShieldCheck /><span><strong>No customer code changes inside DokoSoko.</strong> Vendors implement the versioned authorization, project, issuance, and revocation HTTP contract. Custom one-off actions remain custom MCP tools.</span></div><div className="metrics-grid"><Metric label="Provider connections" value={String(providers.length)} detail="Fixed destinations and encrypted service credentials" /><Metric label="Projects" value={String(projects.length)} detail={`${projects.filter((project) => project.state === "active").length} active`} /><Metric label="Credential leases" value={String(credentials.length)} detail={`${credentials.filter((lease) => !lease.revoked_at && new Date(lease.expires_at) > new Date()).length} active`} /><Metric label="Persistent secrets" value="0" detail="Issued credentials are shown once" positive /></div><section className="panel"><div className="panel-heading"><div><h2>Provider contract</h2><p>POST /v1/authorize → /v1/projects or /v1/credentials → explicit revoke</p></div><Badge color={providers.length ? "green" : "amber"}>{providers.length ? "Connected" : "Required"}</Badge></div>{providers.map((provider) => <div className="provider-row" key={provider.id}><span className="settings-icon"><KeyRound /></span><span><strong>{provider.name}</strong><small>Contract {provider.config.contract_version ?? "2026-08-01"} · max TTL {provider.config.max_ttl_seconds ?? 3600}s</small></span><Badge color="violet">{provider.kind}</Badge><code>{provider.config.required_entitlements?.join(", ") || "Authenticated users"}</code></div>)}{providers.length === 0 && <div className="empty-row">Connect a Provider API to expose project and credential tools on Private MCP.</div>}</section><div className="project-grid"><section className="panel"><div className="panel-heading"><div><h2>Recent projects</h2><p>Idempotent, environment-scoped vendor resources.</p></div></div>{projects.slice(0, 8).map((project) => <div className="lease-row" key={project.id}><span><strong>{project.external_id}</strong><small>{project.environment_id}</small></span><Badge color={project.state === "active" ? "green" : "zinc"}>{project.state}</Badge></div>)}{projects.length === 0 && <div className="empty-row">No projects issued yet.</div>}</section><section className="panel"><div className="panel-heading"><div><h2>Credential leases</h2><p>Metadata and fingerprints only; no credential plaintext.</p></div></div>{credentials.slice(0, 8).map((lease) => <div className="lease-row" key={lease.id}><span><strong>{lease.scopes.join(", ") || "Default scope"}</strong><small>{lease.secret_fingerprint.slice(0, 16)}… · expires {new Date(lease.expires_at).toLocaleString()}</small></span><Badge color={lease.revoked_at ? "zinc" : new Date(lease.expires_at) < new Date() ? "amber" : "green"}>{lease.revoked_at ? "Revoked" : new Date(lease.expires_at) < new Date() ? "Expired" : "Active"}</Badge></div>)}{credentials.length === 0 && <div className="empty-row">No credentials issued yet.</div>}</section></div></>;
}

function IntegrationRunsView({ runs, environments, onStart, onComplete }: { runs: APIIntegrationRun[]; environments: APIEnvironment[]; onStart: () => void; onComplete: (run: APIIntegrationRun, succeeded: boolean) => void }) {
  const environmentName = (id: string) => environments.find((environment) => environment.id === id)?.name ?? id;
  const completed = runs.filter((run) => run.finished_at);
  const validatedSuccess = completed.filter((run) => run.validated_success).length;
  return <><PageHeading eyebrow="Outcomes" title="Integration runs" description="Track requested outcomes and close each run with deterministic validation." action={<Button onClick={onStart}><Plus data-slot="icon" />Start run</Button>} /><div className="metrics-grid"><Metric label="Runs" value={String(runs.length)} detail={`${runs.filter((run) => run.state === "running").length} active`} /><Metric label="Validated" value={String(completed.length)} detail="Completed with evidence" /><Metric label="Successful" value={String(validatedSuccess)} detail="Validated outcomes" positive={validatedSuccess > 0} /><Metric label="First-pass rate" value={completed.length ? `${(validatedSuccess * 100 / completed.length).toFixed(1)}%` : "—"} detail="Feeds Analytics" /></div><section className="panel"><div className="panel-heading"><div><h2>Recent runs</h2><p>Requested outcome text is visible only to administrators and the owning principal.</p></div><Badge color="violet">Private only</Badge></div>{runs.map((run) => <div className="root-row run-row" key={run.id}><span className="settings-icon">{run.state === "running" ? <Clock3 /> : run.validated_success ? <CheckCircle2 /> : <XCircle />}</span><span><strong>{run.requested_outcome}</strong><small>{environmentName(run.environment_id)} · started {new Date(run.started_at).toLocaleString()}{run.failure_code ? ` · ${run.failure_code}` : ""}</small></span><Badge color={run.state === "running" ? "blue" : run.validated_success ? "green" : "red"}>{run.state}</Badge>{run.state === "running" ? <span className="run-actions"><Button outline onClick={() => onComplete(run, false)}>Failed</Button><Button color="indigo" onClick={() => onComplete(run, true)}>Validated</Button></span> : <span />}</div>)}{runs.length === 0 && <div className="empty-row">No integration runs yet. Start one from this page or Private MCP.</div>}</section></>;
}

function ActivityView({ events }: { events: APIAuditEvent[] }) {
  return <><PageHeading eyebrow="Operations" title="Activity & audit" description="Append-only administrative and policy decisions, kept separate from product analytics." /><section className="panel"><div className="panel-heading"><div><h2>Audit events</h2><p>Actor, action, target, request ID, and timestamp. Secret values are never recorded.</p></div><Badge color="green">Append-only</Badge></div>{events.map((event) => <div className="root-row audit-row" key={event.id}><span className="settings-icon"><ShieldCheck /></span><span><strong>{event.action}</strong><small>{event.target_type} · {event.target_id} · {new Date(event.created_at).toLocaleString()}</small></span><code>{event.actor_id}</code><code>{event.request_id}</code></div>)}{events.length === 0 && <div className="empty-row">Audit activity appears after the first configuration change.</div>}</section></>;
}

function productBindingIcon(binding: APIProductBinding) {
  if (binding.kind === "openapi") return <FileJson2 />;
  if (binding.kind === "docs" || binding.kind === "git") return <BookOpen />;
  if (binding.kind === "package") return <PackageIcon />;
  if (binding.kind === "mcp") return <Share2 />;
  return <Wrench />;
}

function ProductDefinitionView({ definition, build, busy, onBuild, onPublish }: { definition: APIProductDefinition | null; build: APIProductBuild | null; busy: boolean; onBuild: () => void; onPublish: () => void }) {
  const reviewing = build?.state === "review";
  const activeDefinition = reviewing ? build.proposal : definition;
  if (!activeDefinition) {
    return <>
      <PageHeading eyebrow="Auto-magic" title="Product definition" description="Give DokoSoko your specs, documentation, packages, repositories, or MCP endpoints. It will assemble the version graph for you." action={<Button onClick={onBuild}><Sparkles data-slot="icon" />Build automatically</Button>} />
      <section className="panel product-definition-empty"><span className="definition-empty-icon"><Sparkles /></span><div><h2>Start with what you already have</h2><p>DokoSoko finds API capabilities, release versions, compatible packages, versioned documentation, and tools—then asks only about ambiguous relationships.</p></div><Button color="indigo" onClick={onBuild}><Sparkles data-slot="icon" />Build product automatically</Button></section>
    </>;
  }

  const unresolved = reviewing ? build.unresolved : activeDefinition.validation.filter((finding) => finding.level !== "info");
  const blocking = unresolved.some((finding) => finding.level === "error");
  const bindingCount = activeDefinition.components.reduce((total, component) => total + component.releases.reduce((releaseTotal, release) => releaseTotal + release.bindings.length, 0), activeDefinition.product_bindings.length);
  const selectionLabel = (componentID: string, releaseID: string) => {
    const component = activeDefinition.components.find((candidate) => candidate.id === componentID);
    const release = component?.releases.find((candidate) => candidate.id === releaseID);
    return `${component?.name ?? componentID} ${release?.version ?? ""}`.trim();
  };

  return <>
    <PageHeading eyebrow="Auto-magic" title="Product definition" description="One product catalog with independently versioned APIs and evidence-backed bindings." action={<Button outline onClick={onBuild}><Sparkles data-slot="icon" />{reviewing ? "Rebuild automatically" : "Scan for changes"}</Button>} />
    <section className={`ai-build-banner ${reviewing ? "reviewing" : "published"}`}>
      <span className="ai-build-icon">{reviewing ? <Bot /> : <CheckCircle2 />}</span>
      <span className="ai-build-copy"><strong>{reviewing ? "Product draft built automatically" : "Product definition published"}</strong><small>{reviewing ? `${build.inputs.length} sources analyzed · ${activeDefinition.components.length} APIs · ${bindingCount} relationships` : `Revision ${activeDefinition.revision} · customer version pins remain unchanged`}</small></span>
      <Badge color={reviewing ? (blocking ? "red" : unresolved.length ? "amber" : "green") : "green"}>{reviewing ? (blocking ? "Blocked" : unresolved.length ? `${unresolved.length} to review` : "Ready to publish") : "Published"}</Badge>
      {reviewing && <Button color="indigo" disabled={busy || blocking} onClick={onPublish}>{busy ? "Publishing…" : "Publish definition"}</Button>}
    </section>

    <section className="panel product-identity-panel">
      <span className="product-definition-mark">{activeDefinition.name.slice(0, 1).toUpperCase()}</span>
      <span className="product-definition-name"><small>Product</small><strong>{activeDefinition.name}</strong><code>{activeDefinition.slug}</code></span>
      <span className="definition-property"><small>Version strategy</small><strong>Independent API tracks</strong></span>
      <a className="definition-property policy" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><small>MCP policy</small><strong>{activeDefinition.mcp_policy}</strong></a>
    </section>

    <div className="definition-columns">
      <section className="panel definition-capabilities">
        <div className="panel-heading"><div><h2>Discovered APIs</h2><p>Every release owns its exact specs, docs, packages, and tools.</p></div><Badge color="violet">{activeDefinition.components.length} independent</Badge></div>
        {activeDefinition.components.map((component) => <ProductCapability key={component.id} component={component} />)}
      </section>
      <div className="definition-side">
        <section className="panel definition-profile-panel">
          <div className="panel-heading"><div><h2>Compatibility profile</h2><p>Known-good combinations for customer integrations.</p></div></div>
          {activeDefinition.profiles.map((profile) => <article className="definition-profile" key={profile.id}><span className="profile-icon"><GitBranch /></span><span><strong>{profile.name}</strong><small>{profile.selections.map((selection) => selectionLabel(selection.component_id, selection.release_id)).join(" · ")}</small></span><Badge color={profile.state === "published" ? "green" : "amber"}>{profile.state}</Badge></article>)}
          {activeDefinition.profiles.length === 0 && <div className="empty-row">A profile appears after at least one API release is identified.</div>}
        </section>
        <section className="panel definition-validation-panel">
          <div className="panel-heading"><div><h2>Review exceptions</h2><p>Automatic matches stay out of your way.</p></div><Badge color={blocking ? "red" : unresolved.length ? "amber" : "green"}>{unresolved.length ? unresolved.length : "None"}</Badge></div>
          {unresolved.map((finding) => <div className="definition-finding" key={`${finding.code}-${finding.message}`}><span className={finding.level}><AlertCircle /></span><span><strong>{finding.code.replaceAll("_", " ")}</strong><small>{finding.message}</small></span></div>)}
          {unresolved.length === 0 && <div className="definition-all-clear"><CheckCircle2 /><span><strong>Everything joined cleanly</strong><small>No silent version fallback or unresolved compatibility edge.</small></span></div>}
        </section>
      </div>
    </div>
  </>;
}

function ProductCapability({ component }: { component: APIProductComponent }) {
  return <article className="product-capability">
    <div className="capability-heading"><span className="capability-icon"><GitBranch /></span><span><strong>{component.name}</strong><small>Independent release track</small></span><span className="capability-versions">{component.releases.map((release) => <Badge color="violet" key={release.id}>{release.version}</Badge>)}</span></div>
    {component.releases.map((release) => <div className="capability-release" key={release.id}><div className="release-label"><small>Release</small><strong>{release.version}</strong></div><div className="release-bindings">{release.bindings.map((binding) => <div className="release-binding" key={binding.id}><span className="binding-icon">{productBindingIcon(binding)}</span><span className="binding-copy"><strong>{binding.name}</strong><small>{binding.version || binding.kind} · {Math.round(binding.confidence * 100)}% confidence</small></span>{binding.verified ? <CheckCircle2 className="binding-verified" /> : <AlertCircle className="binding-pending" />}</div>)}</div></div>)}
  </article>;
}

function OverviewView({ productName, sourceCount, publishedSourceCount, packageCount, credentialPackageCount, publicResourceCount, analytics, onNavigate, onStartRun }: { productName: string; sourceCount: number; publishedSourceCount: number; packageCount: number; credentialPackageCount: number; publicResourceCount: number; analytics: APIAnalytics | null; onNavigate: (section: Section) => void; onStartRun: () => void }) {
  return <>
    <PageHeading eyebrow={productName} title="Connector overview" description="The health and delivery posture of your production agent connector." action={<Button onClick={onStartRun}><Activity data-slot="icon" />Start integration run</Button>} />
    <div className="metrics-grid"><Metric label="Published sources" value={String(publishedSourceCount)} detail={`${sourceCount} configured sources`} /><Metric label="Packages" value={String(packageCount)} detail={`${credentialPackageCount} credential-backed`} /><Metric label="Public resources" value={String(publicResourceCount)} detail="Private by default" /><Metric label="Validated success" value={analytics?.validated_runs ? `${analytics.first_pass_rate.toFixed(1)}%` : "—"} detail={analytics?.validated_runs ? `${analytics.validated_success} of ${analytics.validated_runs} runs` : "No validated runs yet"} positive={Boolean(analytics?.validated_success)} /></div>
    <div className="overview-grid"><section className="panel"><div className="panel-heading"><div><h2>Connector readiness</h2><p>Required configuration for production.</p></div><Badge color="amber">5 of 7</Badge></div><ChecklistItem done label="Root administrator and MFA" /><ChecklistItem done label="Database and encryption" /><ChecklistItem done label="Product and production environment" /><ChecklistItem done label="Vendor identity provider" /><ChecklistItem done label="First knowledge release published" /><ChecklistItem label="Authorization hook tested" /><ChecklistItem label="Package gateway health verified" /></section><section className="panel"><div className="panel-heading"><div><h2>Quick actions</h2><p>Continue configuring this product.</p></div></div><QuickAction icon={<BookOpen />} title="Review source changes" detail="94 API records need review" onClick={() => onNavigate("sources")} /><QuickAction icon={<Radio />} title="Configure agent access" detail="Private MCP ready; Public MCP off" onClick={() => onNavigate("distribution")} /><QuickAction icon={<Wrench />} title="Publish a custom tool" detail="Define schema, API hook, and policy" onClick={() => onNavigate("tools")} /></section></div>
  </>;
}

function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void }) {
  const imported = tools.filter((tool) => tool.backend_kind === "mcp");
  const delegated = connections.filter((connection) => connection.auth_mode === "delegated_oauth").length;
  const authLabel = (mode: APIMCPConnection["auth_mode"]) => mode === "delegated_oauth" ? "Delegated user OAuth" : mode === "service" ? "Service credential" : "No upstream auth";
  return <>
    <PageHeading eyebrow="Managed bridges" title="MCP connections" description="Import selected third-party MCP tools behind DokoSoko identity, policy, schema, and audit controls." action={<Button onClick={onAdd}><Plus data-slot="icon" />Connect MCP</Button>} />
    <a className="mcp-policy-banner" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><span className="mcp-policy-icon"><ShieldCheck /></span><span><strong>Stateless MCPv2 Only</strong><small>Protocol 2026-07-28 · self-contained requests · no logical live sessions</small></span><ExternalLink /></a>
    <div className="metrics-grid"><Metric label="Upstream connections" value={String(connections.length)} detail="Fixed HTTPS destinations" /><Metric label="Imported tools" value={String(imported.length)} detail={`${imported.filter((tool) => tool.state === "published").length} published`} /><Metric label="Delegated identities" value={String(delegated)} detail="Separate upstream grants" /><Metric label="Drifted schemas" value={String(imported.filter((tool) => tool.upstream_drifted).length)} detail="Published calls fail closed" positive={!imported.some((tool) => tool.upstream_drifted)} /></div>
    <section className="panel mcp-connections-panel">
      <div className="panel-heading"><div><h2>Managed upstreams</h2><p>Inspect returns a complete catalog; import always creates or updates local drafts.</p></div><Badge color="green">Pre-call authz</Badge></div>
      {connections.map((connection) => {
        const connectionTools = imported.filter((tool) => tool.mcp_connection_id === connection.id);
        return <article className="mcp-connection-row" key={connection.id}><span className="connection-mark"><Share2 /></span><span className="connection-main"><span><strong>{connection.name}</strong><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge></span><code>{connection.endpoint}</code><small>{connection.namespace}.* · {connection.protocol_version} · {authLabel(connection.auth_mode)}</small></span><span className="connection-stat"><strong>{connectionTools.length}</strong><small>imported tools</small></span><span className="connection-stat"><strong>{connection.last_synced_at ? new Date(connection.last_synced_at).toLocaleDateString() : "Never"}</strong><small>last inspected</small></span><Button outline disabled={busy} onClick={() => onInspect(connection)}><RefreshCw data-slot="icon" />Inspect & import</Button></article>;
      })}
      {connections.length === 0 && <div className="empty-row">No upstream MCP is connected. Add one to inspect and review its catalog.</div>}
    </section>
    <div className="identity-flow"><span><LockKeyhole /><strong>1 · DokoSoko identity</strong><small>Authenticate the user and resolve vendor entitlements.</small></span><span><ShieldCheck /><strong>2 · Post-authz policy</strong><small>Validate schema, confirmation, entitlement, and operation hook.</small></span><span><Users /><strong>3 · Upstream identity</strong><small>Use a separate user grant or encrypted service credential—never the inbound token.</small></span></div>
  </>;
}

function ToolsView({ tools, onAdd, onPublish }: { tools: APITool[]; onAdd: () => void; onPublish: (tool: APITool) => void }) {
  return <><PageHeading eyebrow="Actions" title="Tools" description="Publish reviewed HTTP actions and imported Stateless MCPv2 tools behind one authorization boundary." action={<Button onClick={onAdd}><Plus data-slot="icon" />Create API tool</Button>} /><div className="notice"><ShieldCheck /><span><strong>Policy-wrapped execution.</strong> Every call is schema validated, entitlement-scoped, reauthorized, rate-limited, and audited before a fixed backend is reached.</span></div><div className="tool-grid">{tools.map((tool) => <article className={`panel tool-card ${tool.upstream_drifted ? "drifted" : ""}`} key={tool.id}><span className="tool-icon">{tool.backend_kind === "mcp" ? <Share2 /> : tool.namespace === "credentials" ? <KeyRound /> : <TerminalSquare />}</span><div><span className="tool-badges"><Badge color={tool.state === "published" ? "green" : "amber"}>{tool.state}</Badge><Badge color={tool.backend_kind === "mcp" ? "violet" : "zinc"}>{tool.backend_kind === "mcp" ? "Stateless MCPv2" : "HTTP"}</Badge>{tool.upstream_drifted && <Badge color="red">Schema drift</Badge>}</span><h3>{tool.namespace}.{tool.name}</h3><code>{tool.backend_kind === "mcp" ? `upstream · ${tool.upstream_tool_name}` : `${tool.http_method} · fixed API hook`}</code>{tool.state === "draft" && !tool.upstream_drifted && <Button outline className="publish-tool" onClick={() => onPublish(tool)}>Publish</Button>}{tool.upstream_drifted && <small className="drift-warning">Re-inspect and review before republishing.</small>}</div><button className="more" type="button" aria-label={`Actions for ${tool.name}`}><MoreHorizontal /></button></article>)}<button type="button" className="new-tool-card" onClick={onAdd}><Plus /><strong>Add API tool</strong><span>Definition → schema → API action → authz → test → publish</span></button></div></>;
}

function AnalyticsView({ publicEnabled, analytics }: { publicEnabled: boolean; analytics: APIAnalytics | null }) {
  const format = (value: number) => new Intl.NumberFormat().format(value);
  const channels = analytics?.channels ?? {};
  const privateMCP = channels.private_mcp ?? 0;
  const publicMCP = channels.public_mcp ?? 0;
  const channelTotal = Math.max(1, privateMCP + publicMCP + (channels.private_widget ?? 0) + (channels.public_widget ?? 0));
  const daily = analytics?.daily_requests ?? [];
  const maxDaily = Math.max(1, ...daily.map((point) => point.count));
  const validationDetail = analytics?.validated_runs ? `${analytics.validated_success} of ${analytics.validated_runs} validated runs succeeded` : "No completed validation runs yet";
  return <><PageHeading eyebrow="Outcomes" title="Analytics" description="Measure adoption, delivery reliability, and validated integration success." action={<Button outline>Last 30 days<ChevronDown data-slot="icon" /></Button>} /><div className="metrics-grid"><Metric label="Active developers" value={format(analytics?.active_developers ?? 0)} detail={`${format(analytics?.authorized_users ?? 0)} authorized identities`} /><Metric label="Integration runs" value={format(analytics?.integration_runs ?? 0)} detail={validationDetail} /><Metric label="First-pass success" value={`${(analytics?.first_pass_rate ?? 0).toFixed(1)}%`} detail="Deterministically validated" positive={Boolean(analytics?.validated_success)} /><Metric label="Tool calls" value={format(analytics?.tool_calls ?? 0)} detail={`${format(analytics?.package_downloads ?? 0)} package downloads`} /></div><div className="analytics-grid"><section className="panel chart-panel"><div className="panel-heading"><div><h2>MCP request volume</h2><p>Append-only private and anonymous request events. Query text is not stored.</p></div><Badge color="blue">{format(analytics?.mcp_requests ?? 0)} requests</Badge></div><div className="live-chart" aria-label="Daily MCP request volume">{daily.length ? daily.map((point) => <div className="live-bar-column" key={point.date} title={`${point.date}: ${point.count}`}><span className="live-bar" style={{ height: `${Math.max(4, point.count / maxDaily * 100)}%` }} /><small>{point.date.slice(5)}</small></div>) : <div className="analytics-empty">Usage will appear after the first MCP request.</div>}</div></section><section className="panel channel-panel"><div className="panel-heading"><div><h2>Access channels</h2><p>Private and anonymous agent usage.</p></div></div><ChannelRow label="Private MCP" value={format(privateMCP)} percent={Math.round(privateMCP / channelTotal * 100)} color="indigo" /><ChannelRow label="Private widget" value={format(channels.private_widget ?? 0)} percent={Math.round((channels.private_widget ?? 0) / channelTotal * 100)} color="violet" /><ChannelRow label="Public MCP & widget" value={publicEnabled ? format(publicMCP + (channels.public_widget ?? 0)) : "Off"} percent={publicEnabled ? Math.round((publicMCP + (channels.public_widget ?? 0)) / channelTotal * 100) : 0} color="cyan" /></section></div></>;
}

function SettingsView({ identity, llmProfiles, rootUsers, currentUser, onDoctor, onConfigureIdentity, onConfigureLLM, onAddRoot, onRevokeRoot }: { identity: APIIdentity | null; llmProfiles: APILLMProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onConfigureIdentity: () => void; onConfigureLLM: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <><PageHeading eyebrow="Administration" title="Platform settings" description="Configure deployment, storage, AI, identity, security, and operations." action={<Button outline onClick={onDoctor}><Activity data-slot="icon" />Run System Doctor</Button>} /><div className="settings-grid"><SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted local object storage" status="Healthy" /><button type="button" className="settings-button" onClick={onConfigureLLM}><SettingsCard icon={<Bot />} title="LLM profiles & hardening" detail={`${llmProfiles.length} optional profile${llmProfiles.length === 1 ? "" : "s"} · model authority disabled`} status="Enforced" /></button><button type="button" className="settings-button" onClick={onConfigureIdentity}><SettingsCard icon={<Users />} title="Vendor identity" detail={identity ? `${identity.issuer} · ${identity.allowed_redirect_uris.length} redirect URI(s)` : "Configure OIDC and entitlement resolution"} status={identity ? "Configured" : "Required"} /></button><SettingsCard icon={<ShieldCheck />} title="Root users & audit" detail={`${activeRoots.length} MFA-protected root administrator${activeRoots.length === 1 ? "" : "s"} · append-only audit`} status="Secure" /></div><section className="panel identity-contract"><div className="panel-heading"><div><h2>OAuth contract</h2><p>MCP or widget → DokoSoko → vendor IdP → vendor entitlements → product-bound DokoSoko token</p></div><Button onClick={onConfigureIdentity}>{identity ? "Edit identity" : "Configure identity"}</Button></div><div className="contract-grid"><span><small>Downstream client ID</small><code>{identity?.product_id ?? "Product ID"}</code></span><span><small>Vendor issuer</small><code>{identity?.issuer ?? "Not configured"}</code></span><span><small>Entitlement hook</small><code>{identity?.entitlement_hook_url || "No hook configured"}</code></span></div></section><section className="panel root-management"><div className="panel-heading"><div><h2>Root administrators</h2><p>Root access is independent from vendor identities and always requires MFA.</p></div><Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button></div>{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><strong>{user.display_name}</strong><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section></>;
}

function WarningContent({ children }: { children: React.ReactNode }) { return <div className="warning-content"><div className="warning-icon"><TriangleAlert /></div><div>{children}</div></div>; }
function Confirmation({ checked, onChange, children }: { checked: boolean; onChange: (checked: boolean) => void; children: React.ReactNode }) { return <label className="confirmation"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span className="check-box">{checked && <Check />}</span><span>{children}</span></label>; }
function SummaryItem({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) { return <div className="summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>; }
function Metric({ label, value, detail, positive }: { label: string; value: string; detail: string; positive?: boolean }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small className={positive ? "positive" : ""}>{detail}</small></article>; }
function ChecklistItem({ done = false, label }: { done?: boolean; label: string }) { return <div className="checklist-item"><span className={done ? "done" : ""}>{done && <Check />}</span><p>{label}</p>{done ? <Badge color="green">Complete</Badge> : <Badge color="zinc">Required</Badge>}</div>; }
function QuickAction({ icon, title, detail, onClick }: { icon: React.ReactNode; title: string; detail: string; onClick: () => void }) { return <button type="button" className="quick-action" onClick={onClick}><span>{icon}</span><span><strong>{title}</strong><small>{detail}</small></span><ExternalLink /></button>; }
function ChannelRow({ label, value, percent, color }: { label: string; value: string; percent: number; color: string }) { return <div className="channel-row"><div><span>{label}</span><strong>{value}</strong></div><div className="progress"><span className={color} style={{ width: `${percent}%` }} /></div><small>{percent}%</small></div>; }
function SettingsCard({ icon, title, detail, status }: { icon: React.ReactNode; title: string; detail: string; status: string }) { return <article className="panel settings-card"><span className="settings-icon">{icon}</span><div><h3>{title}</h3><p>{detail}</p></div><Badge color="green">{status}</Badge><ExternalLink /></article>; }
