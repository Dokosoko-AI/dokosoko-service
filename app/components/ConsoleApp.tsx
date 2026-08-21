"use client";

import {
  Activity,
  AlertCircle,
  ArrowLeft,
  BarChart3,
  BookOpen,
  Bot,
  Bug,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
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
  MessageSquareText,
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
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIAnalytics, APIAuditEvent, APIDeployment, APIEnvironment, APIError, APIIdentity, APIIntegration, APIIntegrationRevision, APIIntegrationRun, APILLMProfile, APIMCPCatalog, APIMCPConnection, APIProduct, APIProductBinding, APIProductBuild, APIProductBuildInput, APIProductComponent, APIProductDefinition, APIProductInstallation, APIProductVersion, APIProductVersionDiff, APIProductVersionImpact, APIProductVersionPin, APIProductVersionPinHistory, APIReportingConfig, APIReportSubmission, APIResourceSet, APISupportRoute, APITool, APIUser, APIWidgetSnippets, Distribution, SetupEnrollment, api } from "../lib/api";
import { ConsoleRoute, EntityKind, INTEGRATION_TABS, IntegrationTab, Section, entityPath, integrationPath, parseConsolePath, routeForSection, sectionPath } from "../lib/console-routes";
import { Badge, Button, Dialog, Switch } from "./catalyst";

type NavigationGroup = "overview" | "integrations" | "access" | "distribution" | "operations" | "insights";
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

const navigation: Array<{
  id: NavigationGroup;
  label: string;
  icon: typeof LayoutDashboard;
  defaultSection: Section;
  sections: Array<{ id: Section; label: string }>;
}> = [
  { id: "overview", label: "Overview", icon: LayoutDashboard, defaultSection: "overview", sections: [{ id: "overview", label: "Overview" }] },
  { id: "integrations", label: "Integrations", icon: Sparkles, defaultSection: "product", sections: [
    { id: "product", label: "Catalog" },
    { id: "sources", label: "Documentation" },
    { id: "packages", label: "Packages" },
    { id: "tools", label: "Tools" },
    { id: "connections", label: "Hooks & MCP" },
  ] },
  { id: "access", label: "Access", icon: KeyRound, defaultSection: "projects", sections: [{ id: "projects", label: "Access" }] },
  { id: "distribution", label: "Distribution", icon: Radio, defaultSection: "distribution", sections: [
    { id: "distribution", label: "MCP & widgets" },
    { id: "releases", label: "Connector releases" },
  ] },
  { id: "operations", label: "Operations", icon: Activity, defaultSection: "runs", sections: [
    { id: "runs", label: "Connector runs" },
    { id: "reporting", label: "Support reporting" },
  ] },
  { id: "insights", label: "Insights", icon: BarChart3, defaultSection: "analytics", sections: [
    { id: "analytics", label: "Analytics" },
    { id: "activity", label: "Activity & audit" },
  ] },
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
  { id: "tool_sandbox", organisation_id: "org_acme", product_id: "prod_acme", namespace: "access", name: "create_sandbox", description: "Create a sandbox", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_credentials", organisation_id: "org_acme", product_id: "prod_acme", namespace: "credentials", name: "issue", description: "Issue credentials", input_schema: {}, output_schema: {}, state: "draft", revision: 1, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_incidents", organisation_id: "org_acme", product_id: "prod_acme", namespace: "support", name: "create_incident", description: "Create a support incident", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "MCP", authorization_policy: { required_entitlements: ["support.write"] }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: "mcp_support", upstream_tool_name: "incidents.create", upstream_schema_hash: "sha256:8f44e6" },
];

const fixtureProduct: APIProduct = { id: "prod_acme", organisation_id: "org_acme", name: "Acme Platform", slug: "acme", description: "Build reliable voice and messaging experiences with versioned APIs, SDKs, documentation, and managed tools.", default_version_policy: "latest", catalog_revision: 12, require_promotion_approval: true, public_mcp_enabled: false, revision: 1 };
const fixtureDeployment: APIDeployment = { id: fixtureProduct.id, organisation_id: fixtureProduct.organisation_id, name: fixtureProduct.name, slug: fixtureProduct.slug, description: fixtureProduct.description, default_release_policy: fixtureProduct.default_version_policy, catalog_revision: fixtureProduct.catalog_revision, require_promotion_approval: fixtureProduct.require_promotion_approval, public_mcp_enabled: fixtureProduct.public_mcp_enabled, revision: fixtureProduct.revision };
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

const fixtureDiff: APIProductVersionDiff = { from_version_id: "version_2026_05", from_version: "2026.5", generated_at: "2026-08-19T12:20:00Z", summary: "1 added, 0 removed, 2 changed", added: [{ kind: "artifact", path: "capability/voice/artifact/tool/voice.calls.transfer", after: "v3" }], removed: [], changed: [{ kind: "artifact", path: "capability/voice/artifact/package/@acme/voice-node", before: "7.1.0", after: "7.2.1" }, { kind: "artifact", path: "capability/messages/artifact/package/@acme/messages", before: "5.0.8", after: "5.1.3" }] };
const fixtureProductVersions: APIProductVersion[] = [
  { id: "version_2026_08", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.8", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:81f4…b9c2", diff: fixtureDiff, release_stage: "active", rollout_percentage: 25, promotion_state: "approved", requested_latest: true, requested_lts: false, approved_by: "root_approver", approved_at: "2026-08-19T12:19:00Z", drift_status: "healthy", drift_details: [], drift_checked_at: "2026-08-19T12:19:30Z", is_latest: true, is_lts: false, revision: 2, published_at: "2026-08-19T12:20:00Z" },
  { id: "version_2026_05", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.5", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:17bd…81a0", diff: { ...fixtureDiff, from_version_id: undefined, from_version: undefined, summary: "Initial product release", added: [], changed: [] }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: true, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: true, revision: 3, published_at: "2026-05-10T09:00:00Z" },
  { id: "version_2025_11", organisation_id: "org_acme", product_id: "prod_acme", version: "2025.11", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:02aa…4d31", diff: { ...fixtureDiff, summary: "0 added, 1 removed, 1 changed" }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: false, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: false, deprecated_at: "2026-08-01T00:00:00Z", deprecation_message: "Move to 2026.5 LTS or 2026.8 latest.", replacement_version: "2026.5", sunset_at: "2026-12-01T00:00:00Z", revision: 4, published_at: "2025-11-12T09:00:00Z" },
];

const fixtureProductPins: APIProductVersionPin[] = [
  { id: "pin_contoso", organisation_id: "org_acme", product_id: "prod_acme", scope: "customer", scope_id: "contoso", customer_id: "contoso", product_version_id: "version_2026_05", product_version: "2026.5", reason: "Production stability window", revision: 1, created_at: "2026-08-19T12:30:00Z", updated_at: "2026-08-19T12:30:00Z" },
];

const fixtureInstallations: APIProductInstallation[] = [{ id: "installation_contoso_voice", organisation_id: "org_acme", product_id: "prod_acme", customer_id: "contoso", environment_id: "env_prod", external_id: "contoso-voice-prod", name: "Contoso voice production", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z" }];

function ConsoleLink({ path, onNavigate, className, children, ariaCurrent }: { path: string; onNavigate: (path: string) => void; className?: string; children: React.ReactNode; ariaCurrent?: "page" }) {
  return <a href={path} className={className} aria-current={ariaCurrent} onClick={(event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }}>{children}</a>;
}

function EntityLink({ entity, uid, onNavigate, className, children }: { entity: EntityKind; uid: string; onNavigate: (path: string) => void; className?: string; children: React.ReactNode }) {
  return <ConsoleLink path={entityPath(entity, uid)} onNavigate={onNavigate} className={className}>{children}</ConsoleLink>;
}

type EntityDetail = {
  eyebrow: string;
  title: string;
  description: string;
  fields: Array<{ label: string; value: string }>;
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

function deploymentAsLegacyProduct(value: APIDeployment): APIProduct {
  return { id: value.id, organisation_id: value.organisation_id, name: value.name, slug: value.slug, description: value.description, default_version_policy: value.default_release_policy, catalog_revision: value.catalog_revision, require_promotion_approval: value.require_promotion_approval, public_mcp_enabled: value.public_mcp_enabled, revision: value.revision };
}

export function ConsoleApp({ currentUser, currentDeployment, onLogout }: { currentUser?: APIUser | null; currentDeployment?: APIDeployment | null; onLogout?: () => void | Promise<void> }) {
	const [product, setProduct] = useState<APIProduct>(deploymentAsLegacyProduct(currentDeployment ?? fixtureDeployment));
	const [integrations, setIntegrations] = useState<APIIntegration[]>([]);
	const [resourceSets, setResourceSets] = useState<APIResourceSet[]>([]);
	const [accessDefinitions, setAccessDefinitions] = useState<APIAccessDefinition[]>([]);
	const [accessConnections, setAccessConnections] = useState<APIAccessConnection[]>([]);
	const [accessInstances, setAccessInstances] = useState<APIAccessInstance[]>([]);
	const [accessCredentials, setAccessCredentials] = useState<APIAccessCredential[]>([]);
	const [supportRoutes, setSupportRoutes] = useState<APISupportRoute[]>([]);
  const [consoleRoute, setConsoleRoute] = useState<ConsoleRoute>(() => routeForSection("overview"));
  const section = consoleRoute.section;
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtureDefinition);
  const [, setLatestProductBuild] = useState<APIProductBuild | null>(fixtureProductBuild);
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
  const [toolNamespace, setToolNamespace] = useState("access");
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
	  const [reportingConfig, setReportingConfig] = useState<APIReportingConfig | null>(null);
	  const [reportSubmissions, setReportSubmissions] = useState<APIReportSubmission[]>([]);
	  const [reportDetail, setReportDetail] = useState<APIReportSubmission | null>(null);
	  const [reportDetailBusy, setReportDetailBusy] = useState(false);
	  const [rootUsers, setRootUsers] = useState<APIUser[]>(currentUser ? [currentUser] : []);
	  const [identityOpen, setIdentityOpen] = useState(false);
	  const [identityBusy, setIdentityBusy] = useState(false);
	  const [idpIssuer, setIDPIssuer] = useState("");
	  const [idpClientID, setIDPClientID] = useState("");
	  const [idpClientSecret, setIDPClientSecret] = useState("");
	  const [idpScopes, setIDPScopes] = useState("openid, profile, email");
	  const [idpAudience, setIDPAudience] = useState("");
	  const [idpOrganisationClaim, setIDPOrganisationClaim] = useState("org_id");
	  const [idpInstallationClaim, setIDPInstallationClaim] = useState("installation_id");
	  const [idpEntitlementHook, setIDPEntitlementHook] = useState("");
	  const [idpAuthorizationHook, setIDPAuthorizationHook] = useState("");
	  const [idpAuthorizationCredential, setIDPAuthorizationCredential] = useState("");
	  const [idpUsageHook, setIDPUsageHook] = useState("");
	  const [idpUsageCredential, setIDPUsageCredential] = useState("");
	  const [idpRedirects, setIDPRedirects] = useState("");
	  const [rootOpen, setRootOpen] = useState(false);
	  const [rootBusy, setRootBusy] = useState(false);
	  const [rootEmail, setRootEmail] = useState("");
	  const [rootDisplayName, setRootDisplayName] = useState("");
	  const [rootPassword, setRootPassword] = useState("");
	  const [rootCode, setRootCode] = useState("");
	  const [rootEnrollment, setRootEnrollment] = useState<SetupEnrollment | null>(null);
	  const [rootRecoveryCodes, setRootRecoveryCodes] = useState<string[]>([]);
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
	  const [productCatalogOpen, setProductCatalogOpen] = useState(false);
	  const [productCatalogBusy, setProductCatalogBusy] = useState(false);
	  const [productDescription, setProductDescription] = useState(product.description);
	  const [defaultVersionPolicy, setDefaultVersionPolicy] = useState<"latest" | "lts">(product.default_version_policy);
	  const [requirePromotionApproval, setRequirePromotionApproval] = useState(product.require_promotion_approval);
	  const [productVersions, setProductVersions] = useState<APIProductVersion[]>(fixtureProductVersions);
	  const [productVersionPins, setProductVersionPins] = useState<APIProductVersionPin[]>(fixtureProductPins);
	  const [productInstallations, setProductInstallations] = useState<APIProductInstallation[]>(fixtureInstallations);
	  const [pinHistory, setPinHistory] = useState<APIProductVersionPinHistory[]>([]);
	  const [newProductVersion, setNewProductVersion] = useState("");
	  const [newProductProfile, setNewProductProfile] = useState(fixtureDefinition.profiles[0]?.id ?? "");
	  const [newVersionLatest, setNewVersionLatest] = useState(true);
	  const [newVersionLTS, setNewVersionLTS] = useState(false);
	  const [newVersionStage, setNewVersionStage] = useState<"preview" | "active">("active");
	  const [newVersionRollout, setNewVersionRollout] = useState(100);
	  const [pinScope, setPinScope] = useState<"customer" | "environment" | "installation">("customer");
	  const [pinCustomerID, setPinCustomerID] = useState("");
	  const [pinVersionID, setPinVersionID] = useState(fixtureProductVersions[0]?.id ?? "");
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
	  const [installationEnvironmentID, setInstallationEnvironmentID] = useState(fixtureEnvironment.id);

  useEffect(() => {
    const fixturePreview = process.env.NODE_ENV === "development" && new URLSearchParams(window.location.search).get("preview") === "fixtures";
    if (fixturePreview) document.documentElement.dataset.preview = "fixtures";
    return () => { delete document.documentElement.dataset.preview; };
  }, []);

  useEffect(() => {
    const syncRoute = () => setConsoleRoute(parseConsolePath(window.location.pathname));
    if (window.location.pathname === "/") {
      const preview = process.env.NODE_ENV === "development" && new URLSearchParams(window.location.search).get("preview") === "fixtures" ? window.location.search : "";
      window.history.replaceState(null, "", `${sectionPath("overview")}${preview}`);
    }
    syncRoute();
    window.addEventListener("popstate", syncRoute);
    return () => window.removeEventListener("popstate", syncRoute);
  }, []);

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
      setProduct(distributionValue.product);
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
	      setIDPIssuer(value.issuer); setIDPClientID(value.client_id); setIDPScopes(value.scopes.join(", ")); setIDPAudience(value.audience); setIDPOrganisationClaim(value.organisation_claim); setIDPInstallationClaim(value.installation_claim); setIDPEntitlementHook(value.entitlement_hook_url); setIDPAuthorizationHook(value.authorization_hook_url); setIDPUsageHook(value.usage_hook_url); setIDPRedirects(value.allowed_redirect_uris.join("\n"));
	    }).catch(() => {});
	    Promise.all([api.reporting(product.id), api.reportSubmissions(product.id)]).then(([config, submissions]) => {
	      if (cancelled) return;
	      setReportingConfig(config);
	      setReportSubmissions(submissions);
	    }).catch(() => {});
	    api.rootUsers().then((value) => { if (!cancelled) setRootUsers(value); }).catch(() => {});
	    Promise.all([api.integrations(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.supportRoutes()]).then(async ([integrationValues, setValues, definitionValues, connectionValues, routeValues]) => {
	      if (cancelled) return;
	      setIntegrations(integrationValues); setResourceSets(setValues); setAccessDefinitions(definitionValues); setAccessConnections(connectionValues); setSupportRoutes(routeValues);
	      const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
	      const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
	      if (!cancelled) { setAccessInstances(instanceGroups.flat()); setAccessCredentials(credentialGroups.flat()); }
	    }).catch(() => {});
	    api.llmProfiles(product.id).then((values) => { if (!cancelled) setLLMProfiles(values); }).catch(() => {});
	    api.productDefinition(product.id).then((value) => { if (!cancelled) setProductDefinition(value); }).catch((error) => { if (!cancelled && error instanceof APIError && error.status === 404) setProductDefinition(null); });
	    api.productBuilds(product.id).then((values) => { if (!cancelled) setLatestProductBuild(values[0] ?? null); }).catch(() => {});
	    api.productVersions(product.id).then((values) => { if (!cancelled) { setProductVersions(values); setPinVersionID(values.find((value) => value.is_latest)?.id ?? values[0]?.id ?? ""); } }).catch(() => {});
	    api.productVersionPins(product.id).then((values) => { if (!cancelled) setProductVersionPins(values); }).catch(() => {});
	    Promise.all([api.productInstallations(product.id), api.productVersionPinHistory(product.id)]).then(([installationValues, historyValues]) => { if (!cancelled) { setProductInstallations(installationValues); setPinHistory(historyValues); } }).catch(() => {});
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

	async function refreshCatalog() {
		const [integrationValues, setValues, definitionValues, connectionValues, routeValues] = await Promise.all([api.integrations(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.supportRoutes()]);
		setIntegrations(integrationValues);
		setResourceSets(setValues);
		setAccessDefinitions(definitionValues);
		setAccessConnections(connectionValues);
		setSupportRoutes(routeValues);
		const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
		const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
		setAccessInstances(instanceGroups.flat());
		setAccessCredentials(credentialGroups.flat());
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
		: { id: `version_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, version: newProductVersion.trim(), profile_id: newProductProfile, profile_name: profile?.name ?? "Compatibility profile", definition_revision: productDefinition?.revision ?? 1, manifest_hash: `sha256:preview-${Date.now()}`, diff: fixtureDiff, release_stage: requirePromotionApproval ? "preview" as const : newVersionStage, rollout_percentage: newVersionRollout, promotion_state: requirePromotionApproval ? "pending" as const : "not_required" as const, requested_latest: newVersionLatest, requested_lts: newVersionLTS, drift_status: "healthy" as const, drift_details: [], is_latest: requirePromotionApproval ? false : newVersionLatest || productVersions.length === 0, is_lts: requirePromotionApproval ? false : newVersionLTS, revision: 1, published_at: now };
      if (apiConnected) setProductVersions(await api.productVersions(product.id));
      else setProductVersions((current) => [value, ...current.map((candidate) => value.is_latest ? { ...candidate, is_latest: false } : candidate)]);
      setPinVersionID(value.id);
      setNewProductVersion("");
      setNewVersionLatest(false);
	  setNewVersionLTS(false);
	  setNewVersionStage("active");
	  setNewVersionRollout(100);
      showToast(`Connector release ${value.version} published as an immutable compatibility snapshot.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The connector release could not be published.");
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
		? await api.saveProductVersionPin(product.id, { scope: pinScope, scope_id: pinCustomerID.trim(), customer_id: installation?.customer_id, product_version_id: pinVersionID, reason: pinReason.trim(), revision: existing?.revision ?? 0 })
		: { id: existing?.id ?? `pin_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, scope: pinScope, scope_id: pinCustomerID.trim(), customer_id: pinScope === "customer" ? pinCustomerID.trim() : installation?.customer_id ?? "", environment_id: pinScope === "environment" ? pinCustomerID.trim() : installation?.environment_id, installation_id: installation?.id, product_version_id: pinVersionID, product_version: selected?.version ?? "", reason: pinReason.trim(), revision: (existing?.revision ?? 0) + 1, created_at: existing?.created_at ?? now, updated_at: now };
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
	  const value = apiConnected ? await api.saveProductInstallation(product.id, { customer_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active", revision: 0 }) : { id: `installation_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, customer_id: installationCustomerID.trim(), environment_id: installationEnvironmentID, external_id: installationExternalID.trim(), name: installationName.trim(), state: "active" as const, revision: 1, created_at: now, updated_at: now };
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
        : { ...fixtureProductBuild, id: fallbackBuildID, state: "review" as const, created_at: new Date().toISOString(), completed_at: new Date().toISOString(), inputs: [...fixtureProductBuild.inputs, ...additionalInputs], proposal: { ...fixtureDefinition, state: "draft" as const, source_build_id: fallbackBuildID } };
      setLatestProductBuild(value);
      setProductBuilderOpen(false);
      setProductBuilderInputs("");
      navigateToSection("product");
      showToast(`Integration catalog discovered from ${value.inputs.length} sources. Review ${value.unresolved.length || "no"} exception${value.unresolved.length === 1 ? "" : "s"}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The Integration catalog could not be discovered.");
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
		installation_claim: idpInstallationClaim,
        entitlement_hook_url: idpEntitlementHook,
		authorization_hook_url: idpAuthorizationHook,
		authorization_credential: idpAuthorizationCredential,
		usage_hook_url: idpUsageHook,
		usage_credential: idpUsageCredential,
        allowed_redirect_uris: idpRedirects.split(/\r?\n/).map((value) => value.trim()).filter(Boolean),
      };
      const value = apiConnected ? await api.configureIdentity(product.id, input) : { id: "idp_preview", product_id: product.id, revision: 1, ...input } as APIIdentity;
      setIdentityConfig(value);
      setIDPClientSecret("");
	  setIDPAuthorizationCredential("");
	  setIDPUsageCredential("");
      setIdentityOpen(false);
      showToast("Vendor identity and entitlement resolution are configured.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not configure vendor identity.");
    } finally {
      setIdentityBusy(false);
    }
  }

  async function retryReportSubmission(submission: APIReportSubmission) {
	try {
	  const value = apiConnected ? await api.retryReportSubmission(product.id, submission.id) : { ...submission, state: "pending" as const, last_error: undefined };
	  setReportSubmissions((items) => items.map((item) => item.id === value.id ? value : item));
	  showToast("Submission queued for another delivery attempt.");
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not retry this submission.");
	}
  }

  async function openReportSubmission(submission: APIReportSubmission) {
	setReportDetail(submission);
	if (!apiConnected) return;
	setReportDetailBusy(true);
	try {
	  setReportDetail(await api.reportSubmission(product.id, submission.id));
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

  const publicEndpoint = distribution?.public_mcp_endpoint ?? "/mcp/public";
  const publicSnippet = widgetSnippets?.public.snippet ?? `<script async src="/widgets/${product.id}/public.js" data-product="${product.id}"></script>`;
  const privateSnippet = widgetSnippets?.private.snippet ?? `<script async src="/widgets/${product.id}/private.js" data-product="${product.id}"></script>`;
  const mcpConnectionReady = Boolean(mcpName.trim() && mcpNamespace.trim() && mcpEndpoint.trim() && (mcpAuthMode !== "service" || mcpCredential.trim()) && (mcpAuthMode !== "delegated_oauth" || (mcpOAuthClientID.trim() && mcpOAuthClientSecret.trim() && mcpOAuthIssuer.trim() && mcpAuthorizationURL.trim() && mcpTokenURL.trim())));
  const activeNavigation = navigation.find((item) => item.sections.some((candidate) => candidate.id === section));
  const entityDetail = useMemo<EntityDetail | null>(() => {
    if (consoleRoute.kind !== "entity") return null;
    const date = (value?: string) => value ? new Date(value).toLocaleString() : "—";
    const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
    switch (consoleRoute.entity) {
      case "integration": {
        const value = integrations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Integration", title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([["UID", value.id], ["Lifecycle", value.lifecycle], ["Revision", value.revision], ["Resources", value.resources?.length ?? 0], ["Access connections", value.access_connection_ids?.length ?? 0], ["Sunset", date(value.sunset_at)]]) } : null;
      }
      case "resource-set": {
        const value = resourceSets.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Reusable resource set", title: value.name, description: value.description || "Reusable Integration resource configuration.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["Revision", value.latest_revision?.revision ?? value.revision], ["Integrations", value.integration_ids?.length ?? 0]]) } : null;
      }
      case "source": {
        const value = sources.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Documentation source", title: value.name, description: value.location, fields: fields([["UID", value.id], ["Kind", value.kind], ["Visibility", value.visibility], ["Crawl state", value.crawlState], ["Pages", value.pages], ["Revision", value.revision]]) } : null;
      }
      case "package": {
        const value = packages.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Package", title: value.name, description: `${value.ecosystem} package delivery`, fields: fields([["UID", value.id], ["Version", value.version], ["Mode", value.mode], ["Visibility", value.visibility], ["State", value.status], ["Revision", value.revision]]) } : null;
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
        return value ? { eyebrow: "Access connection", title: value.name, description: value.definition?.name || "Provider-owned service connection.", fields: fields([["UID", value.id], ["State", value.state], ["Region", value.region], ["Environment", value.environment_id], ["Integrations", value.integration_ids?.length ?? 0], ["Revision", value.revision]]) } : null;
      }
      case "installation": {
        const value = productInstallations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Installation", title: value.name, description: value.external_id, fields: fields([["UID", value.id], ["Customer", value.customer_id], ["Environment", value.environment_id], ["State", value.state], ["Revision", value.revision], ["Updated", date(value.updated_at)]]) } : null;
      }
      case "release": {
        const value = productVersions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Connector release", title: value.version, description: value.diff.summary, fields: fields([["UID", value.id], ["Profile", value.profile_name], ["Stage", value.release_stage], ["Promotion", value.promotion_state], ["Rollout", `${value.rollout_percentage}%`], ["Manifest", value.manifest_hash]]) } : null;
      }
      case "run": {
        const value = integrationRuns.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Connector run", title: value.requested_outcome, description: `Run ${value.id}`, fields: fields([["UID", value.id], ["State", value.state], ["Environment", value.environment_id], ["Reported success", value.reported_success], ["Validated success", value.validated_success], ["Started", date(value.started_at)], ["Finished", date(value.finished_at)]]) } : null;
      }
      case "support-route": {
        const value = supportRoutes.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Support route", title: value.name, description: value.is_default ? "Default route for unassigned Integrations." : "Integration-specific support delivery.", fields: fields([["UID", value.id], ["State", value.state], ["Bug reports", value.bug_reports_enabled ? "Enabled" : "Disabled"], ["Feedback", value.feedback_enabled ? "Enabled" : "Disabled"], ["Retention", `${value.retention_days} days`], ["Integrations", value.integration_ids?.length ?? 0]]) } : null;
      }
      case "report": {
        const value = reportSubmissions.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "Support submission", title: value.summary, description: "Sanitized submission metadata. Decrypted report content remains administrator-gated.", fields: fields([["UID", value.id], ["Kind", value.kind], ["State", value.state], ["Integration", value.trusted_integration?.display_name], ["Delivery attempts", value.attempts], ["Created", date(value.created_at)]]) } : null;
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
  }, [consoleRoute, integrations, resourceSets, sources, packages, tools, mcpConnections, accessDefinitions, accessConnections, productInstallations, productVersions, integrationRuns, supportRoutes, reportSubmissions, auditEvents, rootUsers]);

  function routeURL(path: string) {
    const preview = process.env.NODE_ENV === "development" && new URLSearchParams(window.location.search).get("preview") === "fixtures" ? window.location.search : "";
    return `${path}${preview}`;
  }

  function navigateToPath(path: string, replace = false) {
    const next = parseConsolePath(path);
    if (typeof window !== "undefined") {
      const method = replace ? "replaceState" : "pushState";
      if (window.location.pathname !== next.path || replace) window.history[method](null, "", routeURL(next.path));
      window.scrollTo({ top: 0, behavior: "auto" });
    }
    setConsoleRoute(next);
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

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><span className="brand-mark" aria-hidden="true">D</span><span>DokoSoko</span></div>
        <nav aria-label="Main navigation">
          {navigation.map((item) => {
            const Icon = item.icon;
            return <ConsoleLink key={item.id} path={sectionPath(item.defaultSection)} onNavigate={navigateToPath} className={`nav-item ${activeNavigation?.id === item.id ? "active" : ""}`} ariaCurrent={activeNavigation?.id === item.id ? "page" : undefined}><Icon /><span>{item.label}</span></ConsoleLink>;
          })}
        </nav>
        <div className="sidebar-bottom">
          <ConsoleLink path={sectionPath("settings")} onNavigate={navigateToPath} className={`nav-item ${section === "settings" ? "active" : ""}`} ariaCurrent={section === "settings" ? "page" : undefined}><Settings /><span>Settings</span></ConsoleLink>
          <div className="account"><span className="avatar">{(currentUser?.display_name ?? "Yuriy Admin").split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><strong>{currentUser?.display_name ?? "Yuriy"}</strong><small>{currentUser ? "Root administrator" : "Platform admin"}</small></span>{onLogout && <button type="button" className="logout-button" aria-label="Sign out" title="Sign out" onClick={onLogout}><LogOut /></button>}</div>
        </div>
      </aside>

      <main>
        <header className="topbar">
          <button type="button" className="product-switcher"><span className="product-logo">{product.name.slice(0, 1).toUpperCase()}</span><span><small>Deployment</small><strong>{product.name}</strong></span><ChevronDown /></button>
          <select className="mobile-navigation" aria-label="Console section" value={section === "settings" ? "settings" : activeNavigation?.id ?? "overview"} onChange={(event) => navigateToGroup(event.target.value as NavigationGroup | "settings")}>
            {navigation.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            <option value="settings">Settings</option>
          </select>
          <div className="environment"><span className="status-dot" />Production</div>
        </header>

        <div className="content">
          {activeNavigation && activeNavigation.id !== "integrations" && activeNavigation.sections.length > 1 && (
            <nav className="section-tabs" aria-label={`${activeNavigation.label} sections`}>
              {activeNavigation.sections.map((item) => <ConsoleLink key={item.id} path={sectionPath(item.id)} onNavigate={navigateToPath} className={`section-tab ${section === item.id ? "active" : ""}`} ariaCurrent={section === item.id ? "page" : undefined}>{item.label}</ConsoleLink>)}
            </nav>
          )}
          {consoleRoute.kind === "not-found" ? <ConsoleNotFoundView path={consoleRoute.path} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "integration" ? <IntegrationsView integrations={integrations} resourceSets={resourceSets} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} mcpConnections={mcpConnections} selectedIntegrationID={consoleRoute.uid} activeTab={consoleRoute.integrationTab} onBuild={() => setProductBuilderOpen(true)} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" ? <EntityDetailView route={consoleRoute} detail={entityDetail} onNavigate={navigateToPath} /> : <>
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
              onOpenSources={() => navigateToSection("sources")}
            />
          )}
          {section === "product" && <IntegrationsView integrations={integrations} resourceSets={resourceSets} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} mcpConnections={mcpConnections} onBuild={() => setProductBuilderOpen(true)} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "sources" && <SourcesView sources={sources} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} onNavigate={navigateToPath} />}
          {section === "packages" && <PackagesView packages={packages} onAdd={() => setAddPackageOpen(true)} onPublish={publishPackage} onVisibilityChange={(id) => requestVisibility("package", id)} onNavigate={navigateToPath} />}
          {section === "projects" && <AccessView definitions={accessDefinitions} connections={accessConnections} instances={accessInstances} credentials={accessCredentials} integrations={integrations} environments={environments} hookSets={resourceSets.filter((set) => set.kind === "hook")} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} onNavigate={navigateToPath} />}
          {section === "overview" && <OverviewView productName={product.name} sourceCount={sources.length} publishedSourceCount={sources.filter((source) => source.published).length} packageCount={packages.length} credentialPackageCount={packages.filter((pkg) => pkg.mode !== "public").length} publicResourceCount={publicResourceCount} analytics={analytics} onNavigate={navigateToSection} onStartRun={() => setRunOpen(true)} />}
          {section === "tools" && <ToolsView tools={tools} onAdd={() => setAddToolOpen(true)} onPublish={publishTool} onNavigate={navigateToPath} />}
          {section === "releases" && <ConnectorReleasesView versions={productVersions} integrations={integrations} onConfigure={openProductCatalog} onNavigate={navigateToPath} />}
          {section === "runs" && <IntegrationRunsView runs={integrationRuns} environments={environments} onStart={() => setRunOpen(true)} onComplete={completeIntegrationRun} onNavigate={navigateToPath} />}
          {section === "reporting" && <ReportingView config={reportingConfig} routes={supportRoutes} integrations={integrations} submissions={reportSubmissions} onChanged={refreshCatalog} onMessage={showToast} onView={openReportSubmission} onRetry={retryReportSubmission} onNavigate={navigateToPath} />}
          {section === "analytics" && <AnalyticsView publicEnabled={publicMCPEnabled} analytics={analytics} />}
          {section === "activity" && <ActivityView events={auditEvents} onNavigate={navigateToPath} />}
          {section === "settings" && <SettingsView product={product} versions={productVersions} pins={productVersionPins} identity={identityConfig} llmProfiles={llmProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onConfigureProduct={openProductCatalog} onConfigureIdentity={() => setIdentityOpen(true)} onConfigureLLM={() => setLLMOpen(true)} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
          </>}
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
        title="Discover integrations"
        description="DokoSoko will inspect the attached sources, infer versioned APIs, and create reviewable Integration records with explicit resource-set links."
        actions={<><Button outline onClick={() => setProductBuilderOpen(false)}>Cancel</Button><Button color="indigo" disabled={productBuilderBusy} onClick={buildProductAutomatically}>{productBuilderBusy ? "Discovering…" : "Discover integrations"}</Button></>}
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
        open={productCatalogOpen}
        onClose={setProductCatalogOpen}
        title="Deployment discovery & connector releases"
		description="Control the singleton deployment, immutable connector-release integrity, staged rollout, and scoped release resolution that agents receive."
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
            <div className="catalog-settings-heading"><span><strong>Publish connector release</strong><small>Creates an immutable deployment snapshot of tested Integration revisions.</small></span></div>
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
              {productVersions.length === 0 && <div className="empty-row">Discover and publish Integrations, add a deployment description, then create the first connector release.</div>}
            </div>
          </section>

          <section className="catalog-settings-section">
			<div className="catalog-settings-heading"><span><strong>Scoped version pins</strong><small>Use the narrowest scope. Authenticated installation identity wins over environment and customer assignments.</small></span></div>
			<div className="product-pin-create">
			  <label className="auth-field"><span>Scope</span><select value={pinScope} onChange={(event) => { setPinScope(event.target.value as typeof pinScope); setPinCustomerID(""); }}><option value="customer">Customer</option><option value="environment">Environment</option><option value="installation">Installation</option></select></label>
			  {pinScope === "customer" ? <label className="auth-field"><span>Vendor organisation ID</span><input value={pinCustomerID} onChange={(event) => setPinCustomerID(event.target.value)} placeholder="contoso" /></label> : <label className="auth-field"><span>{pinScope === "environment" ? "Environment" : "Installation"}</span><select value={pinCustomerID} onChange={(event) => setPinCustomerID(event.target.value)}><option value="">Select {pinScope}</option>{pinScope === "environment" ? environments.map((item) => <option key={item.id} value={item.id}>{item.name}</option>) : productInstallations.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.customer_id}</option>)}</select></label>}
              <label className="auth-field"><span>Connector release</span><select value={pinVersionID} onChange={(event) => setPinVersionID(event.target.value)}><option value="">Select release</option>{productVersions.filter((version) => !version.deprecated_at).map((version) => <option key={version.id} value={version.id}>{version.version}{version.is_lts ? " · LTS" : version.is_latest ? " · Latest" : ""}</option>)}</select></label>
              <label className="auth-field"><span>Reason</span><input value={pinReason} onChange={(event) => setPinReason(event.target.value)} placeholder="Production stability window" /></label>
              <Button color="indigo" disabled={productCatalogBusy || !pinCustomerID.trim() || !pinVersionID} onClick={pinCustomerVersion}>Save pin</Button>
            </div>
			<div className="product-pin-list">{productVersionPins.map((pin) => <article key={pin.id}><span><strong>{pin.scope}: {pin.scope_id}</strong><small>{pin.reason || "Explicit compatibility assignment"} · revision {pin.revision}</small></span><Badge color="violet">{pin.product_version}</Badge><Button outline onClick={() => removeProductVersionPin(pin)}>Remove</Button></article>)}{productVersionPins.length === 0 && <div className="empty-row">No exact pins. Resolution follows the {defaultVersionPolicy.toUpperCase()} channel.</div>}</div>
			{pinHistory.length > 0 && <small className="catalog-history-note">{pinHistory.length} immutable pin change{pinHistory.length === 1 ? "" : "s"} recorded in assignment history.</small>}
		  </section>

		  <section className="catalog-settings-section">
			<div className="catalog-settings-heading"><span><strong>Integration installations</strong><small>Register the authenticated installation claim and bind it to one customer and environment.</small></span></div>
			<div className="product-pin-create"><label className="auth-field"><span>Name</span><input value={installationName} onChange={(event) => setInstallationName(event.target.value)} placeholder="Contoso voice production" /></label><label className="auth-field"><span>Authenticated external ID</span><input value={installationExternalID} onChange={(event) => setInstallationExternalID(event.target.value)} placeholder="contoso-voice-prod" /></label><label className="auth-field"><span>Customer ID</span><input value={installationCustomerID} onChange={(event) => setInstallationCustomerID(event.target.value)} placeholder="contoso" /></label><label className="auth-field"><span>Environment</span><select value={installationEnvironmentID} onChange={(event) => setInstallationEnvironmentID(event.target.value)}>{environments.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><Button color="indigo" disabled={productCatalogBusy || !installationName.trim() || !installationExternalID.trim() || !installationCustomerID.trim()} onClick={saveInstallation}>Add installation</Button></div>
			<div className="product-pin-list">{productInstallations.map((item) => <article key={item.id}><span><EntityLink entity="installation" uid={item.id} onNavigate={navigateToPath} className="entity-link"><strong>{item.name}</strong></EntityLink><small>{item.external_id} · {item.customer_id} · {environments.find((environment) => environment.id === item.environment_id)?.name ?? item.environment_id}</small></span><Badge color={item.state === "active" ? "green" : "zinc"}>{item.state}</Badge></article>)}</div>
		  </section>
        </div>
      </Dialog>

      <Dialog
        open={versionLifecycleOpen}
        onClose={setVersionLifecycleOpen}
        title={`Lifecycle for ${editingProductVersion?.version ?? "connector release"}`}
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
          <p><strong>{publicResourceCount} published {publicResourceCount === 1 ? "resource is" : "resources are"} currently marked public.</strong> Private sources, private packages, API tools, provider resources, credentials, identities, and entitlement data remain excluded.</p>
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
        description={`DokoSoko brokers OAuth for this deployment. The downstream OAuth client_id is the deployment ID: ${product.id}`}
        actions={<><Button outline onClick={() => setIdentityOpen(false)}>Cancel</Button><Button color="indigo" disabled={identityBusy || !idpIssuer.trim() || !idpClientID.trim() || (!identityConfig && !idpClientSecret.trim()) || !idpRedirects.trim() || (Boolean(idpUsageHook.trim()) && !identityConfig?.usage_hook_url && !idpUsageCredential.trim())} onClick={saveIdentity}>{identityBusy ? "Verifying…" : "Save identity"}</Button></>}
      >
        <div className="auth-form compact-form">
          <label className="auth-field"><span>OIDC issuer</span><input type="url" value={idpIssuer} onChange={(event) => setIDPIssuer(event.target.value)} placeholder="https://identity.vendor.com" /></label>
          <div className="two-fields"><label className="auth-field"><span>OIDC client ID</span><input value={idpClientID} onChange={(event) => setIDPClientID(event.target.value)} /></label><label className="auth-field"><span>{identityConfig ? "Rotate client secret (optional)" : "OIDC client secret"}</span><input type="password" autoComplete="off" value={idpClientSecret} onChange={(event) => setIDPClientSecret(event.target.value)} /></label></div>
          <label className="auth-field"><span>Scopes</span><input value={idpScopes} onChange={(event) => setIDPScopes(event.target.value)} /></label>
		  <div className="two-fields"><label className="auth-field"><span>Audience (optional)</span><input value={idpAudience} onChange={(event) => setIDPAudience(event.target.value)} /></label><label className="auth-field"><span>Organisation claim</span><input value={idpOrganisationClaim} onChange={(event) => setIDPOrganisationClaim(event.target.value)} /></label></div>
		  <label className="auth-field"><span>Installation claim (optional)</span><input value={idpInstallationClaim} onChange={(event) => setIDPInstallationClaim(event.target.value)} placeholder="installation_id" /><small>When present, this authenticated claim selects a registered installation. DokoSoko never trusts an MCP tool argument for installation identity.</small></label>
          <label className="auth-field"><span>Entitlement hook</span><input type="url" value={idpEntitlementHook} onChange={(event) => setIDPEntitlementHook(event.target.value)} placeholder="https://api.vendor.com/dokosoko/entitlements" /><small>The vendor returns enabled and disabled feature keys during login. Hook errors deny private access.</small></label>
          <div className="two-fields"><label className="auth-field"><span>Per-operation authorization hook</span><input type="url" value={idpAuthorizationHook} onChange={(event) => setIDPAuthorizationHook(event.target.value)} placeholder="https://api.vendor.com/dokosoko/authorize" /></label><label className="auth-field"><span>{identityConfig?.authorization_hook_url ? "Rotate authorization credential" : "Authorization credential"}</span><input type="password" autoComplete="off" value={idpAuthorizationCredential} onChange={(event) => setIDPAuthorizationCredential(event.target.value)} /></label></div>
          <div className="two-fields"><label className="auth-field"><span>Usage report hook</span><input type="url" value={idpUsageHook} onChange={(event) => setIDPUsageHook(event.target.value)} placeholder="https://api.vendor.com/dokosoko/usage" /><small>Enables the read-only usage.get tool on Private MCP. Returned values are proxied without storage.</small></label><label className="auth-field"><span>{identityConfig?.usage_hook_url ? "Rotate usage credential" : "Usage credential"}</span><input type="password" autoComplete="off" value={idpUsageCredential} onChange={(event) => setIDPUsageCredential(event.target.value)} /></label></div>
          <label className="auth-field"><span>Allowed downstream redirect URIs</span><textarea value={idpRedirects} onChange={(event) => setIDPRedirects(event.target.value)} placeholder={"https://developer.vendor.com/dokosoko/callback\nhttp://localhost:3000/oauth/callback"} /><small>One exact URI per line. Wildcards are not accepted.</small></label>
          <div className="private-default-note"><ShieldCheck />Login-time entitlements control discovery, operation hooks reauthorize execution, and usage values remain an ephemeral vendor proxy. Credentials stay server-side.</div>
        </div>
      </Dialog>

	  <Dialog
		open={Boolean(reportDetail)}
		onClose={(open) => { if (!open) setReportDetail(null); }}
		title={reportDetail?.kind === "bug" ? "Bug report" : "Feedback submission"}
		description="Decrypted on demand for this authenticated administrative review."
		actions={<Button color="indigo" onClick={() => setReportDetail(null)}>Close</Button>}
	  >
		{reportDetailBusy ? <div className="empty-row">Decrypting submission…</div> : reportDetail && <div className="report-detail"><div className="report-detail-meta"><span><small>Status</small><Badge color={reportDetail.state === "delivered" ? "green" : reportDetail.state === "failed" ? "red" : reportDetail.state === "held" ? "amber" : "blue"}>{reportDetail.state}</Badge></span><span><small>Connector release</small><code>{reportDetail.trusted_context.product_version || "Unversioned"}</code></span><span><small>Created</small><strong>{new Date(reportDetail.created_at).toLocaleString()}</strong></span></div><pre>{JSON.stringify(reportDetail.content ?? { summary: reportDetail.summary }, null, 2)}</pre>{reportDetail.external_url && <a className="report-external-detail" href={reportDetail.external_url} target="_blank" rel="noreferrer"><ExternalLink />Open {reportDetail.external_id || "external ticket"}</a>}</div>}
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

function EntityDetailView({ route, detail, onNavigate }: { route: Extract<ConsoleRoute, { kind: "entity" }>; detail: EntityDetail | null; onNavigate: (path: string) => void }) {
  const parentPath = sectionPath(route.section);
  return <>
    <div className="entity-breadcrumb">
      <ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {route.section === "product" ? "Integrations" : route.section === "projects" ? "Access" : route.section}</ConsoleLink>
      <code>{route.path}</code>
    </div>
    {detail ? <>
      <PageHeading eyebrow={detail.eyebrow} title={detail.title} description={detail.description} />
      <section className="panel entity-detail-panel">
        <div className="panel-heading"><div><h2>Details</h2><p>Vendor and deployment data is shown read-only at this stable URL.</p></div><Badge color="violet">{route.entity}</Badge></div>
        <dl className="entity-detail-grid">{detail.fields.map((field) => <div key={field.label}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}</dl>
      </section>
    </> : <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Item unavailable</h1><p>No {route.entity.replaceAll("-", " ")} with UID <code>{route.uid}</code> is available in this deployment, or it is still loading.</p></div><ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to the directory</ConsoleLink></section>}
  </>;
}

function ConsoleNotFoundView({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">Navigation</p><h1>Page not found</h1><p><code>{path}</code> is not a recognised console URL.</p></div><ConsoleLink path={sectionPath("overview")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to overview</ConsoleLink></section>;
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
    <PageHeading eyebrow="Distribution" title="MCP & widgets" description="Control how agents and developers access your deployment knowledge." action={<Button outline><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
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
      <div className="section-heading"><div><h2>Copy widget</h2><p>Embed connector guidance in your developer portal or application. Snippets never contain a secret.</p></div></div>
      <div className="widget-grid">
        <article className={`widget-card ${!enabled ? "widget-disabled" : ""}`}>
          <WidgetPreview kind="public" />
          <div className="widget-copy"><Badge color="blue"><Globe2 />Public</Badge><h3>Public widget</h3><p>No sign-in. Answers only from public, published sources and packages.</p>{!enabled && <div className="inline-warning"><TriangleAlert />Enable Public MCP before embedding.</div>}<CopyButton text={publicSnippet} label="Copy public widget" disabled={!enabled} onCopied={onCopied} /></div>
        </article>
        <article className="widget-card">
          <WidgetPreview kind="private" />
          <div className="widget-copy"><Badge color="violet"><LockKeyhole />Private</Badge><h3>Private widget</h3><p>Uses your identity flow for private knowledge, tools, packages, provider resources, and credentials.</p><CopyButton text={privateSnippet} label="Copy private widget" onCopied={onCopied} /></div>
        </article>
      </div>
    </section>
  </>;
}

function WidgetPreview({ kind }: { kind: "public" | "private" }) {
  const privateWidget = kind === "private";
  return <div className={`widget-preview ${privateWidget ? "dark-preview" : ""}`}><div className="mini-chat"><span className={`mini-brand ${privateWidget ? "light" : ""}`}>D</span><span><strong>{privateWidget ? "Acme developer assistant" : "Ask Acme"}</strong><small>{privateWidget ? "Signed in as Alex" : "Powered by DokoSoko"}</small></span><button type="button" aria-label="Close widget preview">×</button></div><div className={`mini-message ${privateWidget ? "dark-message" : ""}`}>{privateWidget ? "Show my sandbox credentials" : "How do I create an API key?"}</div><div className={`mini-answer ${privateWidget ? "dark-answer" : ""}`}>{privateWidget ? "I can provision credentials after checking your access." : "I can help with Acme's public documentation and packages."}</div><div className={`mini-input ${privateWidget ? "dark-input" : ""}`}>Ask a question… <span>↑</span></div></div>;
}

function SourcesView({ sources, onAdd, onCrawl, onPublish, onVisibilityChange, onNavigate }: { sources: Source[]; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  return <>
    <PageHeading eyebrow="Knowledge" title="Sources" description="Manage ingestion, crawl state, review, publication, and anonymous visibility." action={<Button onClick={onAdd}><Plus data-slot="icon" />Add source</Button>} />
    <div className="summary-strip"><SummaryItem label="Pages indexed" value="378" icon={<Database />} /><SummaryItem label="Healthy sources" value="1 of 3" icon={<CheckCircle2 />} /><SummaryItem label="Needs attention" value="2" icon={<AlertCircle />} /></div>
    <div className="toolbar"><div className="search-field"><Search /><input aria-label="Search sources" placeholder="Search sources…" /></div><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />Crawl all</Button></div>
    <div className="resource-table">
      <div className="table-head source-columns"><span>Source</span><span>Crawl state</span><span>Content</span><span>Visibility</span><span /></div>
      {sources.map((source) => <div className="table-row source-columns" key={source.id}>
        <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.location} · {source.kind}</small></span></span>
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

function PackagesView({ packages, onAdd, onPublish, onVisibilityChange, onNavigate }: { packages: ProductPackage[]; onAdd: () => void; onPublish: (pkg: ProductPackage) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  return <>
    <PageHeading eyebrow="Distribution" title="Packages" description="Manage public packages and credential-backed proxy or fetch delivery." action={<Button onClick={onAdd}><Plus data-slot="icon" />Add package</Button>} />
    <div className="notice"><ShieldCheck /><span><strong>Credentials stay server-side.</strong> Proxy and fetch modes stream artifacts without exposing persistent upstream tokens.</span></div>
    <div className="resource-table">
      <div className="table-head package-columns"><span>Package</span><span>Ecosystem</span><span>Delivery</span><span>Visibility</span><span /></div>
      {packages.map((pkg) => <div className="table-row package-columns" key={pkg.id}>
        <span className="resource-name"><span className="resource-icon"><PackageIcon /></span><span><EntityLink entity="package" uid={pkg.id} onNavigate={onNavigate} className="entity-link"><strong>{pkg.name}</strong></EntityLink><small>v{pkg.version}</small></span></span>
        <span>{pkg.ecosystem}</span>
        <span><Badge color={pkg.mode === "public" ? "blue" : pkg.mode === "proxy" ? "violet" : "amber"}>{pkg.mode === "proxy" ? <Radio /> : pkg.mode === "fetch" ? <ExternalLink /> : <Globe2 />}{pkg.mode}</Badge></span>
        <span className="visibility-control"><Badge color={pkg.visibility === "public" ? "green" : "zinc"}>{pkg.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{pkg.visibility}</Badge><Switch checked={pkg.visibility === "public"} onChange={() => onVisibilityChange(pkg.id)} label={`Make ${pkg.name} ${pkg.visibility === "public" ? "private" : "public"}`} /></span>
        <span className="table-actions">{!pkg.published && <Button outline onClick={() => onPublish(pkg)}>Publish</Button>}<button type="button" className="more" aria-label={`Actions for ${pkg.name}`}><MoreHorizontal /></button></span>
      </div>)}
    </div>
  </>;
}

function IntegrationDirectoryView({ integrations, resourceSets, connections, supportRoutes, query, onQueryChange, onCreate, onBuild, onNavigate }: { integrations: APIIntegration[]; resourceSets: APIResourceSet[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onBuild: () => void; onNavigate: (path: string) => void }) {
  const active = integrations.filter((integration) => integration.lifecycle === "active").length;
  const sharedSets = resourceSets.filter((set) => (set.integration_ids?.length ?? 0) > 1).length;
  const normalizedQuery = query.trim().toLowerCase();
  const filteredIntegrations = integrations.filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)));
  const connectionCount = (integration: APIIntegration) => connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id)).length;
  const supportName = (integration: APIIntegration) => supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id))?.name ?? supportRoutes.find((route) => route.is_default)?.name ?? "Not configured";

  return <>
    <PageHeading eyebrow="API catalog" title="Integrations" description="Choose an Integration to manage its resources, tools, access, support, and immutable revisions." action={<span className="heading-actions"><Button outline onClick={onCreate}><Plus data-slot="icon" />New Integration</Button><Button onClick={onBuild}><Sparkles data-slot="icon" />Discover catalog</Button></span>} />
    <div className="metrics-grid"><Metric label="Integrations" value={String(integrations.length)} detail={`${active} active`} /><Metric label="Resource sets" value={String(resourceSets.length)} detail={`${sharedSets} deliberately shared`} /><Metric label="Access connections" value={String(connections.length)} detail="Attached explicitly" /><Metric label="Support routes" value={String(supportRoutes.length)} detail="Default or Integration-specific" /></div>
    <div className="toolbar integration-toolbar"><div className="search-field"><Search /><input aria-label="Search integrations" placeholder="Search Integrations…" value={query} onChange={(event) => onQueryChange(event.target.value)} /></div><span>{filteredIntegrations.length} of {integrations.length}</span></div>
    <div className="resource-table integration-directory">
      <div className="table-head integration-directory-columns"><span>Integration</span><span>Lifecycle</span><span>Resources</span><span>Access</span><span>Support</span><span /></div>
      {filteredIntegrations.map((integration) => <ConsoleLink key={integration.id} path={integrationPath(integration.id)} onNavigate={onNavigate} className="table-row integration-directory-columns integration-directory-row">
        <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span></span>
        <Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge>
        <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">attached sets</small></span>
        <span><strong className="cell-value">{connectionCount(integration)}</strong><small className="cell-note">connections</small></span>
        <span className="integration-support-cell">{supportName(integration)}</span>
        <ChevronRight />
      </ConsoleLink>)}
      {filteredIntegrations.length === 0 && <div className="empty-row">{integrations.length === 0 ? "No Integrations yet. Create one manually or discover the connector catalog." : "No Integrations match this search."}</div>}
    </div>
  </>;
}

function IntegrationWorkspaceView({ integration, integrationID, activeTab, loading, revisions, resourceSets, connections, supportRoutes, tools, mcpConnections, busy, onEdit, onPublish, onAttach, onCreateResource, onEditResource, onDuplicateResource, onDetachResource, onNavigate }: { integration: APIIntegration | null; integrationID: string; activeTab: IntegrationTab; loading: boolean; revisions: APIIntegrationRevision[]; resourceSets: APIResourceSet[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; tools: APITool[]; mcpConnections: APIMCPConnection[]; busy: boolean; onEdit: (integration: APIIntegration) => void; onPublish: (integration: APIIntegration) => void; onAttach: (integration: APIIntegration, kind?: APIResourceSet["kind"]) => void; onCreateResource: () => void; onEditResource: (resource: APIResourceSet) => void; onDuplicateResource: (resource: APIResourceSet) => void; onDetachResource: (integrationID: string, resourceSetID: string) => void; onNavigate: (path: string) => void }) {
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">Integration</p><h1>Loading Integration…</h1><p>Retrieving the Integration and its immutable revision history.</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">Integration</p><h1>Integration unavailable</h1><p>No Integration with UID <code>{integrationID}</code> is available in this deployment.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to Integrations</ConsoleLink></section>;

  const integrationConnections = connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id));
  const supportRoute = supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default);
  const attachedResources = integration.resources ?? [];
  const attachedHooks = attachedResources.filter((resource) => resource.kind === "hook");
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />All Integrations</ConsoleLink><code>{integrationPath(integration.id, activeTab)}</code></div>
    <PageHeading eyebrow={`${integration.family_key} · ${integration.version_key}`} title={integration.display_name} description={integration.description || "No description has been added for this Integration."} action={<span className="heading-actions"><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge><Button outline onClick={() => onEdit(integration)}>Edit</Button><Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />Publish revision</Button></span>} />
    <nav className="integration-tabs" aria-label={`${integration.display_name} sections`}>{INTEGRATION_TABS.map((tab) => <ConsoleLink key={tab.id} path={integrationPath(integration.id, tab.id)} onNavigate={onNavigate} className={`integration-tab ${activeTab === tab.id ? "active" : ""}`} ariaCurrent={activeTab === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</nav>

    {activeTab === "overview" && <div className="integration-tab-content">
      <div className="metrics-grid"><Metric label="Resources" value={String(attachedResources.length)} detail="Explicitly attached" /><Metric label="Access" value={String(integrationConnections.length)} detail="Allowed connections" /><Metric label="Support" value={supportRoute?.name ?? "—"} detail={supportRoute ? supportRoute.is_default ? "Using default route" : "Integration-specific route" : "Not configured"} /><Metric label="Published revisions" value={String(revisions.length)} detail={`Current draft r${integration.revision}`} /></div>
      <section className="panel"><div className="panel-heading"><div><h2>Integration identity</h2><p>The family and version identify this API independently from connector releases.</p></div><Badge color="violet">Stable UID</Badge></div><dl className="entity-detail-grid"><div><dt>UID</dt><dd>{integration.id}</dd></div><div><dt>API family</dt><dd>{integration.family_key}</dd></div><div><dt>API version</dt><dd>{integration.version_key}</dd></div><div><dt>Draft revision</dt><dd>{integration.revision}</dd></div><div><dt>Replacement</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>Sunset</dt><dd>{integration.sunset_at ? new Date(integration.sunset_at).toLocaleDateString() : "—"}</dd></div></dl></section>
      <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "resources")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>Resources</strong><small>Documentation, packages, and hooks attached to this Integration.</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={integrationPath(integration.id, "access")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><KeyRound /></span><span><strong>Access & support</strong><small>Review the vendor connections and reporting route for this Integration.</small></span><ChevronRight /></ConsoleLink></div>
    </div>}

    {activeTab === "resources" && <div className="integration-tab-content">
      <section className="panel"><div className="panel-heading"><div><h2>Attached resource sets</h2><p>Sets remain reusable objects. This page controls only their explicit attachment to this Integration.</p></div><span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create shared set</Button><Button disabled={resourceSets.every((set) => attachedResources.some((resource) => resource.resource_set_id === set.id))} onClick={() => onAttach(integration)}>Attach existing</Button></span></div>
        {attachedResources.map((resource) => { const source = resourceSets.find((set) => set.id === resource.resource_set_id); return <div className="integration-resource-row" key={resource.resource_set_id}><span className="settings-icon">{resource.kind === "documentation" ? <BookOpen /> : resource.kind === "package" ? <PackageIcon /> : <Wrench />}</span><span><EntityLink entity="resource-set" uid={resource.resource_set_id} onNavigate={onNavigate} className="entity-link"><strong>{resource.name}</strong></EntityLink><small>{resource.kind} · revision {resource.resolved_revision?.revision ?? "—"} · {resource.follow_latest ? "follows latest" : "pinned"}</small></span><Badge color={resource.kind === "documentation" ? "blue" : resource.kind === "package" ? "green" : "violet"}>{resource.kind}</Badge><span className="table-actions">{source && <Button outline onClick={() => onEditResource(source)}>New revision</Button>}{source && <Button outline onClick={() => onDuplicateResource(source)}>Duplicate</Button>}<button type="button" className="more" disabled={busy} title={`Detach ${resource.name}`} aria-label={`Detach ${resource.name}`} onClick={() => onDetachResource(integration.id, resource.resource_set_id)}><XCircle /></button></span></div>; })}
        {attachedResources.length === 0 && <div className="empty-row">No resources are attached. Attach an existing reusable set or create a new one.</div>}
      </section>
      <div className="integration-library-links"><ConsoleLink path={sectionPath("sources")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>Documentation library</strong><small>Manage the shared documentation source inventory.</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={sectionPath("packages")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><PackageIcon /></span><span><strong>Package library</strong><small>Manage shared package delivery records.</small></span><ChevronRight /></ConsoleLink></div>
    </div>}

    {activeTab === "tools" && <div className="integration-tab-content">
      <section className="panel"><div className="panel-heading"><div><h2>Integration hook sets</h2><p>Only hook resource sets attached here are part of this Integration revision.</p></div><Button disabled={!resourceSets.some((set) => set.kind === "hook" && !attachedResources.some((resource) => resource.resource_set_id === set.id))} onClick={() => onAttach(integration, "hook")}>Attach hook set</Button></div>{attachedHooks.map((resource) => <div className="lease-row" key={resource.resource_set_id}><span><EntityLink entity="resource-set" uid={resource.resource_set_id} onNavigate={onNavigate} className="entity-link"><strong>{resource.name}</strong></EntityLink><small>revision {resource.resolved_revision?.revision ?? "—"} · {resource.follow_latest ? "follows latest" : "pinned"}</small></span><Badge color="violet">Hook set</Badge></div>)}{attachedHooks.length === 0 && <div className="empty-row">No hook set is attached to this Integration.</div>}</section>
      <section className="panel"><div className="panel-heading"><div><h2>Shared agent tools</h2><p>Tools and MCP connections are deployment-level libraries. Attach hook sets above for Integration-specific routing.</p></div><span className="heading-actions"><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link">Tool library</ConsoleLink><ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className="entity-back-link">Hooks & MCP</ConsoleLink></span></div><div className="integration-library-summary"><span><strong>{tools.length}</strong><small>tool definitions</small></span><span><strong>{mcpConnections.length}</strong><small>MCP connections</small></span><span><strong>{tools.filter((tool) => tool.state === "published").length}</strong><small>published tools</small></span></div></section>
    </div>}

    {activeTab === "access" && <div className="integration-tab-content"><section className="panel"><div className="panel-heading"><div><h2>Allowed access connections</h2><p>These vendor accounts may issue credentials or expose provider resources for this Integration.</p></div><ConsoleLink path={sectionPath("projects")} onNavigate={onNavigate} className="entity-back-link">Manage access</ConsoleLink></div>{integrationConnections.map((connection) => <div className="provider-row integration-connection-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.definition?.name ?? "Access service"}{connection.region ? ` · ${connection.region}` : ""}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge><code>{connection.id}</code></div>)}{integrationConnections.length === 0 && <div className="empty-row">No access connection is attached to this Integration.</div>}</section></div>}

    {activeTab === "support" && <div className="integration-tab-content"><section className="panel"><div className="panel-heading"><div><h2>Bug reports & feedback</h2><p>An Integration-specific route wins; otherwise this Integration inherits the deployment default.</p></div><ConsoleLink path={sectionPath("reporting")} onNavigate={onNavigate} className="entity-back-link">Manage support routes</ConsoleLink></div>{supportRoute ? <><div className="support-route-summary"><span className="settings-icon"><Bug /></span><span><EntityLink entity="support-route" uid={supportRoute.id} onNavigate={onNavigate} className="entity-link"><strong>{supportRoute.name}</strong></EntityLink><small>{supportRoute.is_default ? "Deployment default" : "Integration-specific"} · {supportRoute.retention_days}-day encrypted retention</small></span><Badge color={supportRoute.state === "active" ? "green" : "zinc"}>{supportRoute.state}</Badge></div><dl className="entity-detail-grid"><div><dt>Bug reports</dt><dd>{supportRoute.bug_reports_enabled ? "Enabled" : "Disabled"}</dd></div><div><dt>Feedback</dt><dd>{supportRoute.feedback_enabled ? "Enabled" : "Disabled"}</dd></div><div><dt>Delivery</dt><dd>{supportRoute.bug_hook_url || supportRoute.feedback_hook_url ? "Webhook" : "Held locally"}</dd></div></dl></> : <div className="empty-row">No default or Integration-specific support route is configured.</div>}</section></div>}

    {activeTab === "revisions" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>Published revisions are immutable.</strong> Connector releases may select one of these Integration revisions alongside revisions from other Integrations.</span></div><section className="panel"><div className="panel-heading"><div><h2>Published revision history</h2><p>Each publish captures the Integration identity, resource attachments, access, and support routing.</p></div><ConsoleLink path={sectionPath("releases")} onNavigate={onNavigate} className="entity-back-link">Connector releases</ConsoleLink></div>{sortedRevisions.map((revision) => <div className="integration-revision-row" key={revision.id}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? new Date(revision.published_at ?? revision.created_at).toLocaleString() : "Date unavailable"}{revision.published_by ? ` · ${revision.published_by}` : ""}</small></span><code>{revision.manifest_hash}</code></div>)}{sortedRevisions.length === 0 && <div className="empty-row">No immutable revision has been published yet.</div>}</section></div>}
  </>;
}

function IntegrationsView({ integrations, resourceSets, supportRoutes, connections, tools, mcpConnections, selectedIntegrationID, activeTab = "overview", onBuild, onChanged, onMessage, onNavigate }: { integrations: APIIntegration[]; resourceSets: APIResourceSet[]; supportRoutes: APISupportRoute[]; connections: APIAccessConnection[]; tools: APITool[]; mcpConnections: APIMCPConnection[]; selectedIntegrationID?: string; activeTab?: IntegrationTab; onBuild: () => void; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<APIIntegration | null>(null);
  const [selectedRevisions, setSelectedRevisions] = useState<APIIntegrationRevision[]>([]);
  const [loadedIntegrationID, setLoadedIntegrationID] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editingIntegration, setEditingIntegration] = useState<APIIntegration | null>(null);
  const [familyKey, setFamilyKey] = useState("");
  const [versionKey, setVersionKey] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [lifecycle, setLifecycle] = useState<APIIntegration["lifecycle"]>("draft");
  const [replacementID, setReplacementID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [resourceOpen, setResourceOpen] = useState(false);
  const [editingSet, setEditingSet] = useState<APIResourceSet | null>(null);
  const [setKind, setSetKind] = useState<APIResourceSet["kind"]>("documentation");
  const [setName, setSetName] = useState("");
  const [resourceDescription, setResourceDescription] = useState("");
  const [setManifest, setSetManifest] = useState("[]");
  const [duplicateSet, setDuplicateSet] = useState<APIResourceSet | null>(null);
  const [duplicateName, setDuplicateName] = useState("");
  const [attachIntegration, setAttachIntegration] = useState<APIIntegration | null>(null);
  const [attachSetID, setAttachSetID] = useState("");
  const [attachKind, setAttachKind] = useState<APIResourceSet["kind"] | "">("");
  const [pinAttachedSet, setPinAttachedSet] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!selectedIntegrationID) return;
    let cancelled = false;
    api.integration(selectedIntegrationID).then((value) => {
      if (cancelled) return;
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setLoadedIntegrationID(selectedIntegrationID);
    }).catch(() => {
      if (cancelled) return;
      setSelectedDetail(null);
      setSelectedRevisions([]);
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
      setLoadedIntegrationID(integrationID);
    } catch {
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setLoadedIntegrationID(integrationID);
    }
  }

  function openIntegration(value?: APIIntegration) {
    setEditingIntegration(value ?? null);
    setFamilyKey(value?.family_key ?? ""); setVersionKey(value?.version_key ?? ""); setDisplayName(value?.display_name ?? ""); setDescription(value?.description ?? ""); setLifecycle(value?.lifecycle ?? "draft"); setReplacementID(value?.replacement_integration_id ?? ""); setSunsetAt(value?.sunset_at?.slice(0, 10) ?? "");
    setIntegrationOpen(true);
  }

  async function saveIntegration() {
    setBusy(true);
    try {
      const base = { family_key: familyKey, version_key: versionKey, display_name: displayName, description, lifecycle };
      const saved = editingIntegration
        ? await api.updateIntegration(editingIntegration.id, { ...base, replacement_integration_id: replacementID || undefined, sunset_at: sunsetAt ? new Date(`${sunsetAt}T00:00:00Z`).toISOString() : undefined, revision: editingIntegration.revision })
        : await api.createIntegration(base);
      setSelectedDetail(saved);
      await onChanged();
      setIntegrationOpen(false);
      onMessage(editingIntegration ? "Integration updated." : `Integration created with ${saved.lifecycle} lifecycle.`);
      if (!editingIntegration) onNavigate(integrationPath(saved.id));
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Integration could not be saved."); } finally { setBusy(false); }
  }

  async function publishIntegration(value: APIIntegration) {
    setBusy(true);
    try { await api.publishIntegration(value.id); await onChanged(); await refreshSelectedIntegration(value.id); onMessage("Immutable Integration revision published."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Integration could not be published."); } finally { setBusy(false); }
  }

  function openResource(value?: APIResourceSet) {
    setEditingSet(value ?? null); setSetKind(value?.kind ?? "documentation"); setSetName(value?.name ?? ""); setResourceDescription(value?.description ?? ""); setSetManifest(JSON.stringify(value?.latest_revision?.manifest ?? [], null, 2)); setResourceOpen(true);
  }

  async function saveResourceSet() {
    setBusy(true);
    try {
      const manifest = JSON.parse(setManifest) as Array<Record<string, unknown>>;
      if (!Array.isArray(manifest)) throw new Error("Manifest must be a JSON array.");
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
    try { await api.attachResourceSet(attachIntegration.id, resource.id, pinAttachedSet ? resource.latest_revision?.id ?? "" : ""); await onChanged(); await refreshSelectedIntegration(attachIntegration.id); setAttachIntegration(null); onMessage(pinAttachedSet ? "Resource revision pinned to Integration." : "Resource set attached and following latest."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be attached."); } finally { setBusy(false); }
  }

  async function detachResource(integrationID: string, setID: string) {
    setBusy(true);
    try { await api.detachResourceSet(integrationID, setID); await onChanged(); await refreshSelectedIntegration(integrationID); onMessage("Resource set detached from Integration."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be detached."); } finally { setBusy(false); }
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

  return <>
    {selectedIntegrationID ? <IntegrationWorkspaceView integration={selectedIntegration} integrationID={selectedIntegrationID} activeTab={activeTab} loading={selectedLoading} revisions={selectedRevisions} resourceSets={resourceSets} connections={connections} supportRoutes={supportRoutes} tools={tools} mcpConnections={mcpConnections} busy={busy} onEdit={openIntegration} onPublish={publishIntegration} onAttach={openAttachDialog} onCreateResource={() => openResource()} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(`${set.name} copy`); }} onDetachResource={detachResource} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} resourceSets={resourceSets} connections={connections} supportRoutes={supportRoutes} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onBuild={onBuild} onNavigate={onNavigate} />}

    <Dialog open={integrationOpen} onClose={setIntegrationOpen} title={editingIntegration ? "Edit Integration" : "Create Integration"} description="An Integration is exactly one API family and one API version." actions={<><Button outline onClick={() => setIntegrationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !familyKey.trim() || !versionKey.trim() || !displayName.trim()} onClick={saveIntegration}>{busy ? "Saving…" : "Save Integration"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>API family key</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} placeholder="voice-api" /></label><label className="auth-field"><span>API version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} placeholder="v2" /></label></div><label className="auth-field"><span>Display name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Voice API v2" /></label><label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Lifecycle</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="retired">Retired</option></select></label><label className="auth-field"><span>Replacement</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">None</option>{integrations.filter((value) => value.id !== editingIntegration?.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label></div>{(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>Sunset date</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}</div></Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? `Create revision for ${editingSet.name}` : "Create reusable resource set"} description="Sets are reusable by explicit attachment. Each save creates immutable content." actions={<><Button outline onClick={() => setResourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !setName.trim()} onClick={saveResourceSet}>{busy ? "Saving…" : editingSet ? "Create revision" : "Create set"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Kind</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">Documentation</option><option value="package">Package</option><option value="hook">Hook</option></select></label><label className="auth-field"><span>Name</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>Description</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label><label className="auth-field"><span>Manifest (JSON array)</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label></div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title="Duplicate resource set" description="Creates an independent copy so later edits do not affect Integrations using the original." actions={<><Button outline onClick={() => setDuplicateSet(null)}>Cancel</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>Duplicate</Button></>}><label className="auth-field"><span>New set name</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={`Attach ${attachKind ? `${attachKind} ` : ""}resources to ${attachIntegration?.display_name ?? "Integration"}`} description="Follow latest for deliberate sharing, or pin the current immutable revision." actions={<><Button outline onClick={() => setAttachIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>Attach</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Resource set</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">Select a set</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>Pin the current revision instead of following latest</span></label></div></Dialog>
  </>;
}

function ConnectorReleasesView({ versions, integrations, onConfigure, onNavigate }: { versions: APIProductVersion[]; integrations: APIIntegration[]; onConfigure: () => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Deployment channels" title="Connector releases" description="Publish immutable compatibility snapshots that select a tested combination of Integration revisions. This is a deployment release, not an API version." action={<Button onClick={onConfigure}><Settings data-slot="icon" />Release policy</Button>} /><div className="notice"><GitBranch /><span><strong>Integration versions stay independent.</strong> A connector release can combine Voice API v2 with Face API v3 without changing either Integration identity.</span></div><div className="metrics-grid"><Metric label="Connector releases" value={String(versions.length)} detail={`${versions.filter((version) => version.release_stage === "active").length} active`} /><Metric label="Integrations" value={String(integrations.length)} detail="Selected by immutable revision" /><Metric label="Latest" value={versions.find((version) => version.is_latest)?.version ?? "—"} detail="Default latest channel" /><Metric label="LTS" value={versions.find((version) => version.is_lts)?.version ?? "—"} detail="Stable channel" /></div><section className="panel"><div className="panel-heading"><div><h2>Published snapshots</h2><p>Customer, environment, and installation pins continue to override the default channel.</p></div></div>{versions.map((version) => <div className="provider-row" key={version.id}><span className="settings-icon"><GitBranch /></span><span><EntityLink entity="release" uid={version.id} onNavigate={onNavigate} className="entity-link"><strong>{version.version}</strong></EntityLink><small>{version.profile_name} · {version.manifest_hash}</small></span><span>{version.is_latest && <Badge color="blue">Latest</Badge>} {version.is_lts && <Badge color="violet">LTS</Badge>}</span><Badge color={version.deprecated_at ? "amber" : version.drift_status === "drifted" ? "red" : "green"}>{version.deprecated_at ? "Deprecated" : version.drift_status}</Badge></div>)}{versions.length === 0 && <div className="empty-row">No connector releases have been published.</div>}</section></>;
}

function AccessView({ definitions, connections, instances, credentials, integrations, environments, hookSets, onChanged, onMessage, onNavigate }: { definitions: APIAccessDefinition[]; connections: APIAccessConnection[]; instances: APIAccessInstance[]; credentials: APIAccessCredential[]; integrations: APIIntegration[]; environments: APIEnvironment[]; hookSets: APIResourceSet[]; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const activeCredentials = credentials.filter((credential) => credential.state === "active" && (!credential.expires_at || new Date(credential.expires_at) > new Date())).length;
  const [definitionOpen, setDefinitionOpen] = useState(false);
  const [connectionOpen, setConnectionOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [serviceKey, setServiceKey] = useState(""); const [serviceName, setServiceName] = useState("");
  const [cardinality, setCardinality] = useState<APIAccessDefinition["instance_cardinality"]>("one");
  const [singular, setSingular] = useState("account"); const [plural, setPlural] = useState("accounts");
  const [credentialScope, setCredentialScope] = useState<APIAccessDefinition["credential_scope"]>("connection");
  const [managementAuth, setManagementAuth] = useState<APIAccessDefinition["management_auth_type"]>("bearer");
  const [hookSetID, setHookSetID] = useState("");
  const [operations, setOperations] = useState('{\n  "required_entitlements": [],\n  "max_ttl_seconds": 3600,\n  "credential_storage_mode": "one_time",\n  "authorize": {"method": "POST", "path": "/v1/authorize"},\n  "credentials.create": {"method": "POST", "path": "/v1/credentials"},\n  "credentials.revoke": {"method": "POST", "path": "/v1/credentials/{credential_id}/revoke"}\n}');
  const [definitionID, setDefinitionID] = useState(""); const [connectionName, setConnectionName] = useState(""); const [environmentID, setEnvironmentID] = useState(""); const [region, setRegion] = useState(""); const [baseURL, setBaseURL] = useState(""); const [managementSecret, setManagementSecret] = useState(""); const [connectionConfig, setConnectionConfig] = useState("{}"); const [selectedIntegrations, setSelectedIntegrations] = useState<string[]>([]);

  async function saveDefinition() {
    setBusy(true);
    try { const parsed = JSON.parse(operations) as Record<string, unknown>; await api.createAccessDefinition({ service_key: serviceKey, name: serviceName, instance_cardinality: cardinality, instance_label_singular: singular, instance_label_plural: plural, credential_scope: cardinality === "one" ? "connection" : credentialScope, management_auth_type: managementAuth, hook_set_id: hookSetID || undefined, operations: parsed }); await onChanged(); setDefinitionOpen(false); onMessage("Provider access definition created."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access definition could not be created."); } finally { setBusy(false); }
  }

  async function saveConnection() {
    setBusy(true);
    try { const parsed = JSON.parse(connectionConfig) as Record<string, unknown>; await api.createAccessConnection({ access_definition_id: definitionID, environment_id: environmentID || undefined, name: connectionName, region: region || undefined, base_url: baseURL, management_secret: managementSecret || undefined, config: parsed, integration_ids: selectedIntegrations }); await onChanged(); setConnectionOpen(false); setManagementSecret(""); onMessage("Access connection created and attached."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access connection could not be created."); } finally { setBusy(false); }
  }

  function toggleIntegration(id: string) { setSelectedIntegrations((values) => values.includes(id) ? values.filter((value) => value !== id) : [...values, id]); }

  return <><PageHeading eyebrow="Provider-owned access" title="Access & API keys" description="The service definition decides whether users receive one fixed instance or may create multiple provider resources. API keys are scoped by that service, never by a DokoSoko project." action={<span className="heading-actions"><Button outline onClick={() => setDefinitionOpen(true)}><Plus data-slot="icon" />Service definition</Button><Button onClick={() => { setDefinitionID(definitions[0]?.id ?? ""); setEnvironmentID(environments[0]?.id ?? ""); setConnectionOpen(true); }}><KeyRound data-slot="icon" />Connect service</Button></span>} /><div className="notice"><ShieldCheck /><span><strong>Cardinality belongs to the connected service.</strong> A single-instance service exposes its configured account. Auth0-style services expose provider-owned tenants, projects, or workspaces and issue credentials at the configured scope.</span></div><div className="metrics-grid"><Metric label="Service definitions" value={String(definitions.length)} detail="Reusable operation contracts" /><Metric label="Connections" value={String(connections.length)} detail="Configured vendor accounts" /><Metric label="Provider resources" value={String(instances.length)} detail="Only for multi-instance services" /><Metric label="Active API keys" value={String(activeCredentials)} detail="Plaintext is never listed" positive /></div><section className="panel"><div className="panel-heading"><div><h2>Service definitions</h2><p>Reusable provider contracts used by access connections.</p></div></div>{definitions.map((definition) => <div className="lease-row" key={definition.id}><span><EntityLink entity="access-definition" uid={definition.id} onNavigate={onNavigate} className="entity-link"><strong>{definition.name}</strong></EntityLink><small>{definition.service_key} · {definition.instance_cardinality === "many" ? `Multiple ${definition.instance_label_plural}` : `Single ${definition.instance_label_singular}`}</small></span><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></div>)}{definitions.length === 0 && <div className="empty-row">No service definitions are configured.</div>}</section><section className="panel"><div className="panel-heading"><div><h2>Connections</h2><p>Every connection is explicitly attached to the Integrations allowed to use it.</p></div></div>{connections.map((connection) => { const definition = connection.definition ?? definitions.find((item) => item.id === connection.access_definition_id); const connectionInstances = instances.filter((item) => item.access_connection_id === connection.id); const connectionCredentials = credentials.filter((item) => item.access_connection_id === connection.id); const labels = (connection.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)).filter(Boolean).map((item) => `${item!.family_key} ${item!.version_key}`).join(", "); return <div className="provider-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{definition?.name ?? "Access service"} · {definition?.instance_cardinality === "many" ? `Multiple ${definition.instance_label_plural}` : `Single ${definition?.instance_label_singular ?? "instance"}`}</small><small>{labels || "No Integration attached"}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge><span><strong>{definition?.instance_cardinality === "many" ? connectionInstances.length : "1"} {definition?.instance_cardinality === "many" ? definition.instance_label_plural : definition?.instance_label_singular ?? "instance"}</strong><small>{connectionCredentials.length} credential record(s)</small></span></div>; })}{connections.length === 0 && <div className="empty-row">Create a service definition, then connect a vendor account and attach it to one or more Integrations.</div>}</section><section className="panel"><div className="panel-heading"><div><h2>Credential metadata</h2><p>Fingerprints, scope, and lifecycle only. One-time credential material is returned solely at creation.</p></div><Badge color="violet">Secrets hidden</Badge></div>{credentials.slice(0, 12).map((credential) => <div className="lease-row" key={credential.id}><span><strong>{credential.scopes.join(", ") || "Default scope"}</strong><small>{credential.secret_fingerprint.slice(0, 18)}… · {credential.storage_mode}</small></span><Badge color={credential.state === "active" ? "green" : "zinc"}>{credential.state}</Badge></div>)}{credentials.length === 0 && <div className="empty-row">No API keys have been created through an access connection.</div>}</section>
  <Dialog open={definitionOpen} onClose={setDefinitionOpen} title="Create service definition" description="The provider contract declares cardinality and credential scope; end users do not choose mono versus multi." actions={<><Button outline onClick={() => setDefinitionOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !serviceKey.trim() || !serviceName.trim() || !singular.trim() || !plural.trim()} onClick={saveDefinition}>{busy ? "Saving…" : "Create definition"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service key</span><input value={serviceKey} onChange={(event) => setServiceKey(event.target.value)} placeholder="auth0" /></label><label className="auth-field"><span>Name</span><input value={serviceName} onChange={(event) => setServiceName(event.target.value)} placeholder="Auth0 Management API" /></label></div><div className="two-fields"><label className="auth-field"><span>Provider instances</span><select value={cardinality} onChange={(event) => { const value = event.target.value as typeof cardinality; setCardinality(value); if (value === "one") setCredentialScope("connection"); }}><option value="one">One fixed instance</option><option value="many">Multiple provider resources</option></select></label><label className="auth-field"><span>Credential scope</span><select disabled={cardinality === "one"} value={credentialScope} onChange={(event) => setCredentialScope(event.target.value as typeof credentialScope)}><option value="connection">Connection</option><option value="instance">Provider resource</option></select></label></div><div className="two-fields"><label className="auth-field"><span>Singular label</span><input value={singular} onChange={(event) => setSingular(event.target.value)} placeholder="tenant" /></label><label className="auth-field"><span>Plural label</span><input value={plural} onChange={(event) => setPlural(event.target.value)} placeholder="tenants" /></label></div><div className="two-fields"><label className="auth-field"><span>Management authentication</span><select value={managementAuth} onChange={(event) => setManagementAuth(event.target.value as typeof managementAuth)}><option value="bearer">Bearer token</option><option value="api_key">API key</option><option value="oauth2_client_credentials">OAuth2 client credentials</option><option value="none">None</option></select></label><label className="auth-field"><span>Reusable hook set</span><select value={hookSetID} onChange={(event) => setHookSetID(event.target.value)}><option value="">None</option>{hookSets.map((set) => <option key={set.id} value={set.id}>{set.name}</option>)}</select></label></div><label className="auth-field"><span>Operations (JSON)</span><textarea className="code-input" value={operations} onChange={(event) => setOperations(event.target.value)} spellCheck={false} /></label></div></Dialog>
  <Dialog open={connectionOpen} onClose={setConnectionOpen} title="Connect vendor service" description="Credentials are encrypted server-side. The fixed HTTPS destination is validated again for every operation." actions={<><Button outline onClick={() => setConnectionOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !definitionID || !connectionName.trim() || !baseURL.trim() || selectedIntegrations.length === 0} onClick={saveConnection}>{busy ? "Connecting…" : "Connect service"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service definition</span><select value={definitionID} onChange={(event) => setDefinitionID(event.target.value)}><option value="">Select definition</option>{definitions.map((definition) => <option key={definition.id} value={definition.id}>{definition.name}</option>)}</select></label><label className="auth-field"><span>Connection name</span><input value={connectionName} onChange={(event) => setConnectionName(event.target.value)} /></label></div><div className="two-fields"><label className="auth-field"><span>Environment</span><select value={environmentID} onChange={(event) => setEnvironmentID(event.target.value)}><option value="">All environments</option>{environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}</option>)}</select></label><label className="auth-field"><span>Region</span><input value={region} onChange={(event) => setRegion(event.target.value)} placeholder="us-east-1" /></label></div><label className="auth-field"><span>Fixed HTTPS base URL</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://management.example.com" /></label><label className="auth-field"><span>Management credential</span><input type="password" autoComplete="off" value={managementSecret} onChange={(event) => setManagementSecret(event.target.value)} /></label><fieldset className="catalog-settings-section"><legend>Allowed Integrations</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={selectedIntegrations.includes(integration.id)} onChange={() => toggleIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset><label className="auth-field"><span>Connection configuration (JSON)</span><textarea className="code-input" value={connectionConfig} onChange={(event) => setConnectionConfig(event.target.value)} spellCheck={false} /></label></div></Dialog>
  </>;
}

function IntegrationRunsView({ runs, environments, onStart, onComplete, onNavigate }: { runs: APIIntegrationRun[]; environments: APIEnvironment[]; onStart: () => void; onComplete: (run: APIIntegrationRun, succeeded: boolean) => void; onNavigate: (path: string) => void }) {
  const environmentName = (id: string) => environments.find((environment) => environment.id === id)?.name ?? id;
  const completed = runs.filter((run) => run.finished_at);
  const validatedSuccess = completed.filter((run) => run.validated_success).length;
  return <><PageHeading eyebrow="Outcomes" title="Connector runs" description="Track requested outcomes and close each connector run with deterministic validation." action={<Button onClick={onStart}><Plus data-slot="icon" />Start run</Button>} /><div className="metrics-grid"><Metric label="Runs" value={String(runs.length)} detail={`${runs.filter((run) => run.state === "running").length} active`} /><Metric label="Validated" value={String(completed.length)} detail="Completed with evidence" /><Metric label="Successful" value={String(validatedSuccess)} detail="Validated outcomes" positive={validatedSuccess > 0} /><Metric label="First-pass rate" value={completed.length ? `${(validatedSuccess * 100 / completed.length).toFixed(1)}%` : "—"} detail="Feeds Analytics" /></div><section className="panel"><div className="panel-heading"><div><h2>Recent runs</h2><p>Requested outcome text is visible only to administrators and the owning principal.</p></div><Badge color="violet">Private only</Badge></div>{runs.map((run) => <div className="root-row run-row" key={run.id}><span className="settings-icon">{run.state === "running" ? <Clock3 /> : run.validated_success ? <CheckCircle2 /> : <XCircle />}</span><span><EntityLink entity="run" uid={run.id} onNavigate={onNavigate} className="entity-link"><strong>{run.requested_outcome}</strong></EntityLink><small>{environmentName(run.environment_id)} · started {new Date(run.started_at).toLocaleString()}{run.failure_code ? ` · ${run.failure_code}` : ""}</small></span><Badge color={run.state === "running" ? "blue" : run.validated_success ? "green" : "red"}>{run.state}</Badge>{run.state === "running" ? <span className="run-actions"><Button outline onClick={() => onComplete(run, false)}>Failed</Button><Button color="indigo" onClick={() => onComplete(run, true)}>Validated</Button></span> : <span />}</div>)}{runs.length === 0 && <div className="empty-row">No connector runs yet. Start one from this page or Private MCP.</div>}</section></>;
}

function ReportingView({ config, routes, integrations, submissions, onChanged, onMessage, onView, onRetry, onNavigate }: { config: APIReportingConfig | null; routes: APISupportRoute[]; integrations: APIIntegration[]; submissions: APIReportSubmission[]; onChanged: () => Promise<void>; onMessage: (message: string) => void; onView: (submission: APIReportSubmission) => void; onRetry: (submission: APIReportSubmission) => void; onNavigate: (path: string) => void }) {
  const bugCount = submissions.filter((submission) => submission.kind === "bug").length;
  const feedbackCount = submissions.filter((submission) => submission.kind === "feedback").length;
  const pendingCount = submissions.filter((submission) => submission.state === "pending" || submission.state === "delivering").length;
  const failedCount = submissions.filter((submission) => submission.state === "failed").length;
  const statusColor = (state: APIReportSubmission["state"]): "zinc" | "blue" | "green" | "red" | "amber" => state === "delivered" ? "green" : state === "failed" ? "red" : state === "held" ? "amber" : "blue";
  const defaultRoute = routes.find((route) => route.is_default);
  const hasHook = (submission: APIReportSubmission) => submission.kind === "bug" ? Boolean(defaultRoute?.bug_hook_url || config?.bug_hook_url) : Boolean(defaultRoute?.feedback_hook_url || config?.feedback_hook_url);
  const [routeOpen, setRouteOpen] = useState(false);
  const [editingRoute, setEditingRoute] = useState<APISupportRoute | null>(null);
  const [routeName, setRouteName] = useState(""); const [routeDefault, setRouteDefault] = useState(false); const [routeBugEnabled, setRouteBugEnabled] = useState(true); const [routeFeedbackEnabled, setRouteFeedbackEnabled] = useState(true); const [routeBugURL, setRouteBugURL] = useState(""); const [routeBugCredential, setRouteBugCredential] = useState(""); const [routeFeedbackURL, setRouteFeedbackURL] = useState(""); const [routeFeedbackCredential, setRouteFeedbackCredential] = useState(""); const [routeRetention, setRouteRetention] = useState("30"); const [routeIntegrations, setRouteIntegrations] = useState<string[]>([]); const [busy, setBusy] = useState(false);
  const reportingAvailable = routes.some((route) => route.state === "active" && (route.bug_reports_enabled || route.feedback_enabled)) || Boolean(config?.bug_reports_enabled || config?.feedback_enabled);

  function openRoute(value?: APISupportRoute) {
    setEditingRoute(value ?? null); setRouteName(value?.name ?? ""); setRouteDefault(value?.is_default ?? routes.length === 0); setRouteBugEnabled(value?.bug_reports_enabled ?? true); setRouteFeedbackEnabled(value?.feedback_enabled ?? true); setRouteBugURL(value?.bug_hook_url ?? ""); setRouteBugCredential(""); setRouteFeedbackURL(value?.feedback_hook_url ?? ""); setRouteFeedbackCredential(""); setRouteRetention(String(value?.retention_days ?? 30)); setRouteIntegrations(value?.integration_ids ?? []); setRouteOpen(true);
  }

  function toggleRouteIntegration(id: string) { setRouteIntegrations((values) => values.includes(id) ? values.filter((value) => value !== id) : [...values, id]); }

  async function saveRoute() {
    setBusy(true);
    try {
      const input = { name: routeName, is_default: routeDefault, bug_reports_enabled: routeBugEnabled, feedback_enabled: routeFeedbackEnabled, bug_hook_url: routeBugURL, bug_hook_credential: routeBugCredential || undefined, feedback_hook_url: routeFeedbackURL, feedback_hook_credential: routeFeedbackCredential || undefined, retention_days: Number(routeRetention), state: "active" as const, integration_ids: routeDefault ? [] : routeIntegrations };
      if (editingRoute) await api.updateSupportRoute(editingRoute.id, { ...input, revision: editingRoute.revision }); else await api.createSupportRoute(input);
      await onChanged(); setRouteOpen(false); onMessage(editingRoute ? "Support route updated." : "Support route created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Support route could not be saved."); } finally { setBusy(false); }
  }
  return <>
    <PageHeading eyebrow="Support" title="Bug reports & feedback" description="Consent-gated Private MCP submissions routed by Integration, with encrypted holding and durable ticket delivery." action={<Button onClick={() => openRoute()}><Plus data-slot="icon" />New support route</Button>} />
    <div className="notice"><ShieldCheck /><span><strong>Fixed agent policy.</strong> Agents must preview the exact sanitized report and obtain explicit approval. Server confirmation, schema limits, secret detection, encryption, and Private MCP isolation are enforced independently.</span></div>
    <div className="metrics-grid"><Metric label="Bug reports" value={String(bugCount)} detail={(defaultRoute?.bug_reports_enabled ?? config?.bug_reports_enabled) ? (defaultRoute?.bug_hook_url || config?.bug_hook_url ? "Enabled · delivery hook configured" : "Enabled · held locally") : "Tool disabled"} /><Metric label="Feedback" value={String(feedbackCount)} detail={(defaultRoute?.feedback_enabled ?? config?.feedback_enabled) ? (defaultRoute?.feedback_hook_url || config?.feedback_hook_url ? "Enabled · delivery hook configured" : "Enabled · held locally") : "Tool disabled"} /><Metric label="Support routes" value={String(routes.length)} detail={`${integrations.length} Integration(s)`} /><Metric label="Needs attention" value={String(failedCount)} detail={`${defaultRoute?.retention_days ?? config?.retention_days ?? 30}-day encrypted retention · ${pendingCount} delivering`} positive={failedCount === 0} /></div>
    <section className="panel reporting-policy"><div className="panel-heading"><div><h2>Agent-facing tools</h2><p>These built-ins appear only on authenticated Private MCP when enabled for at least one applicable route.</p></div><Badge color={reportingAvailable ? "green" : "zinc"}>{reportingAvailable ? "Available" : "Disabled"}</Badge></div><div className="reporting-tool-grid"><span><Bug /><strong>support.report_bug</strong><small>{routes.some((route) => route.bug_reports_enabled) || config?.bug_reports_enabled ? "Preview + consent required" : "Disabled"}</small></span><span><MessageSquareText /><strong>support.submit_feedback</strong><small>{routes.some((route) => route.feedback_enabled) || config?.feedback_enabled ? "Preview + consent required" : "Disabled"}</small></span></div></section>
    <section className="panel"><div className="panel-heading"><div><h2>Routing directory</h2><p>Use one default route plus optional Integration-specific overrides. Empty hook URLs hold encrypted reports locally.</p></div><Badge color="violet">{routes.length} route{routes.length === 1 ? "" : "s"}</Badge></div>{routes.map((route) => <div className="provider-row" key={route.id}><span className="settings-icon"><MessageSquareText /></span><span><EntityLink entity="support-route" uid={route.id} onNavigate={onNavigate} className="entity-link"><strong>{route.name}</strong></EntityLink><small>{route.is_default ? "Default for unassigned Integrations" : (route.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)?.display_name ?? id).join(", ")}</small></span><span>{route.bug_reports_enabled && <Badge color="blue">Bugs</Badge>} {route.feedback_enabled && <Badge color="violet">Feedback</Badge>}</span><span className="table-actions"><small>{route.bug_hook_url || route.feedback_hook_url ? "Ticket hook configured" : "Encrypted local holding"} · {route.retention_days} days</small><Button outline onClick={() => openRoute(route)}>Edit</Button></span></div>)}{routes.length === 0 && <div className="empty-row">Create a default route to enable the tools globally, or assign a route to specific Integrations.</div>}</section>
    <section className="panel report-inbox"><div className="panel-heading"><div><h2>Submission inbox</h2><p>Only administrators can open decrypted content. Delivery metadata never contains the report body.</p></div><Badge color="violet">Encrypted at rest</Badge></div><div className="resource-table"><div className="table-head report-columns"><span>Submission</span><span>Integration context</span><span>Delivery</span><span /></div>{submissions.map((submission) => <div className="table-row report-columns" key={submission.id}><span className="resource-name"><span className="resource-icon">{submission.kind === "bug" ? <Bug /> : <MessageSquareText />}</span><span><EntityLink entity="report" uid={submission.id} onNavigate={onNavigate} className="entity-link"><strong title={submission.summary}>{submission.summary}</strong></EntityLink><small>{submission.kind} · {new Date(submission.created_at).toLocaleString()}</small></span></span><span><strong className="cell-value">{submission.trusted_integration ? `${submission.trusted_integration.display_name} ${submission.trusted_integration.version_key}` : submission.related_tool || submission.trusted_context.product_version || "Legacy connector"}</strong><small className="cell-note">{submission.trusted_integration?.manifest_hash || submission.trusted_context.environment_id || submission.trusted_context.selection_source || "Authenticated account"}</small></span><span><Badge color={statusColor(submission.state)}>{submission.state}</Badge><small className="cell-note">{submission.external_id || (submission.attempts ? `${submission.attempts} attempt${submission.attempts === 1 ? "" : "s"}` : "Not delivered")}</small></span><span className="table-actions"><Button outline onClick={() => onView(submission)}>View</Button>{submission.external_url && <a className="report-ticket-link" href={submission.external_url} target="_blank" rel="noreferrer" aria-label="Open external ticket"><ExternalLink /></a>}{(submission.state === "failed" || submission.state === "held") && hasHook(submission) && <Button outline onClick={() => onRetry(submission)}><RefreshCw data-slot="icon" />Retry</Button>}</span></div>)}{submissions.length === 0 && <div className="empty-row">Approved bug reports and feedback will appear here.</div>}</div></section>
    <Dialog open={routeOpen} onClose={setRouteOpen} title={editingRoute ? "Edit support route" : "Create support route"} description="Bug reports and feedback are held locally when their delivery hook is empty." actions={<><Button outline onClick={() => setRouteOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !routeName.trim() || (!routeDefault && routeIntegrations.length === 0)} onClick={saveRoute}>{busy ? "Saving…" : "Save route"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Route name</span><input value={routeName} onChange={(event) => setRouteName(event.target.value)} placeholder="Voice API support" /></label><label className="compact-check"><input type="checkbox" checked={routeDefault} onChange={(event) => setRouteDefault(event.target.checked)} /><span>Default route for Integrations without an override</span></label>{!routeDefault && <fieldset className="catalog-settings-section"><legend>Assigned Integrations</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={routeIntegrations.includes(integration.id)} onChange={() => toggleRouteIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset>}<div className="two-fields"><label className="compact-check"><input type="checkbox" checked={routeBugEnabled} onChange={(event) => setRouteBugEnabled(event.target.checked)} /><span>Enable bug reports</span></label><label className="compact-check"><input type="checkbox" checked={routeFeedbackEnabled} onChange={(event) => setRouteFeedbackEnabled(event.target.checked)} /><span>Enable feedback</span></label></div><label className="auth-field"><span>Bug ticket hook (optional HTTPS)</span><input type="url" value={routeBugURL} onChange={(event) => setRouteBugURL(event.target.value)} placeholder="Leave empty to hold locally" /></label>{routeBugURL && <label className="auth-field"><span>Bug hook credential</span><input type="password" autoComplete="off" value={routeBugCredential} onChange={(event) => setRouteBugCredential(event.target.value)} /><small>Required for a new or changed destination; leave blank to retain the existing credential.</small></label>}<label className="auth-field"><span>Feedback ticket hook (optional HTTPS)</span><input type="url" value={routeFeedbackURL} onChange={(event) => setRouteFeedbackURL(event.target.value)} placeholder="Leave empty to hold locally" /></label>{routeFeedbackURL && <label className="auth-field"><span>Feedback hook credential</span><input type="password" autoComplete="off" value={routeFeedbackCredential} onChange={(event) => setRouteFeedbackCredential(event.target.value)} /><small>Required for a new or changed destination; leave blank to retain the existing credential.</small></label>}<label className="auth-field"><span>Encrypted retention (days)</span><input type="number" min={1} max={365} value={routeRetention} onChange={(event) => setRouteRetention(event.target.value)} /></label></div></Dialog>
  </>;
}

function ActivityView({ events, onNavigate }: { events: APIAuditEvent[]; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Operations" title="Activity & audit" description="Append-only administrative and policy decisions, kept separate from deployment analytics." /><section className="panel"><div className="panel-heading"><div><h2>Audit events</h2><p>Actor, action, target, request ID, and timestamp. Secret values are never recorded.</p></div><Badge color="green">Append-only</Badge></div>{events.map((event) => <div className="root-row audit-row" key={event.id}><span className="settings-icon"><ShieldCheck /></span><span><EntityLink entity="audit-event" uid={event.id} onNavigate={onNavigate} className="entity-link"><strong>{event.action}</strong></EntityLink><small>{event.target_type} · {event.target_id} · {new Date(event.created_at).toLocaleString()}</small></span><code>{event.actor_id}</code><code>{event.request_id}</code></div>)}{events.length === 0 && <div className="empty-row">Audit activity appears after the first configuration change.</div>}</section></>;
}

function productBindingIcon(binding: APIProductBinding) {
  if (binding.kind === "openapi") return <FileJson2 />;
  if (binding.kind === "docs" || binding.kind === "git") return <BookOpen />;
  if (binding.kind === "package") return <PackageIcon />;
  if (binding.kind === "mcp") return <Share2 />;
  return <Wrench />;
}

// Retained temporarily as a read-only compatibility view for legacy manifests.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
function ProductDefinitionView({ product, versions, definition, build, busy, onBuild, onPublish, onConfigure }: { product: APIProduct; versions: APIProductVersion[]; definition: APIProductDefinition | null; build: APIProductBuild | null; busy: boolean; onBuild: () => void; onPublish: () => void; onConfigure: () => void }) {
  const reviewing = build?.state === "review";
  const activeDefinition = reviewing ? build.proposal : definition;
  if (!activeDefinition) {
    return <>
      <PageHeading eyebrow="Auto-magic" title="Product definition" description="Give DokoSoko your specs, documentation, packages, repositories, or MCP endpoints. It will assemble the version graph for you." action={<span className="heading-actions"><Button outline onClick={onConfigure}><Settings data-slot="icon" />Discovery settings</Button><Button onClick={onBuild}><Sparkles data-slot="icon" />Build automatically</Button></span>} />
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
    <PageHeading eyebrow="Auto-magic" title="Product definition" description="One product catalog with independently versioned APIs and evidence-backed bindings." action={<span className="heading-actions"><Button outline onClick={onConfigure}><Settings data-slot="icon" />Discovery settings</Button><Button outline onClick={onBuild}><Sparkles data-slot="icon" />{reviewing ? "Rebuild automatically" : "Scan for changes"}</Button></span>} />
    <section className={`ai-build-banner ${reviewing ? "reviewing" : "published"}`}>
      <span className="ai-build-icon">{reviewing ? <Bot /> : <CheckCircle2 />}</span>
      <span className="ai-build-copy"><strong>{reviewing ? "Product draft built automatically" : "Product definition published"}</strong><small>{reviewing ? `${build.inputs.length} sources analyzed · ${activeDefinition.components.length} APIs · ${bindingCount} relationships` : `Revision ${activeDefinition.revision} · scoped version pins remain unchanged`}</small></span>
      <Badge color={reviewing ? (blocking ? "red" : unresolved.length ? "amber" : "green") : "green"}>{reviewing ? (blocking ? "Blocked" : unresolved.length ? `${unresolved.length} to review` : "Ready to publish") : "Published"}</Badge>
      {reviewing && <Button color="indigo" disabled={busy || blocking} onClick={onPublish}>{busy ? "Publishing…" : "Publish definition"}</Button>}
    </section>

    <section className="panel product-identity-panel">
      <span className="product-definition-mark">{activeDefinition.name.slice(0, 1).toUpperCase()}</span>
      <span className="product-definition-name"><small>Product</small><strong>{activeDefinition.name}</strong><code>{activeDefinition.slug}</code><span className="product-discovery-description">{product.description || "Add the product description agents should use during discovery."}</span></span>
      <span className="definition-property"><small>Version strategy</small><strong>Independent API tracks</strong></span>
      <span className="definition-property"><small>Product releases</small><strong>{versions.find((version) => version.is_latest)?.version ? `Latest ${versions.find((version) => version.is_latest)?.version}` : "No latest release"}{versions.some((version) => version.is_lts) ? ` · LTS ${versions.find((version) => version.is_lts)?.version}` : ""}</strong></span>
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
    <div className="overview-grid"><section className="panel"><div className="panel-heading"><div><h2>Connector readiness</h2><p>Required configuration for production.</p></div><Badge color="amber">5 of 7</Badge></div><ChecklistItem done label="Root administrator and MFA" /><ChecklistItem done label="Database and encryption" /><ChecklistItem done label="Deployment and production environment" /><ChecklistItem done label="Vendor identity provider" /><ChecklistItem done label="First knowledge release published" /><ChecklistItem label="Authorization hook tested" /><ChecklistItem label="Package gateway health verified" /></section><section className="panel"><div className="panel-heading"><div><h2>Quick actions</h2><p>Continue configuring this deployment.</p></div></div><QuickAction icon={<BookOpen />} title="Review source changes" detail="94 API records need review" onClick={() => onNavigate("sources")} /><QuickAction icon={<Radio />} title="Configure agent access" detail="Private MCP ready; Public MCP off" onClick={() => onNavigate("distribution")} /><QuickAction icon={<Wrench />} title="Publish a custom tool" detail="Define schema, API hook, and policy" onClick={() => onNavigate("tools")} /></section></div>
  </>;
}

function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect, onNavigate }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void; onNavigate: (path: string) => void }) {
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
        return <article className="mcp-connection-row" key={connection.id}><span className="connection-mark"><Share2 /></span><span className="connection-main"><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge></span><code>{connection.endpoint}</code><small>{connection.namespace}.* · {connection.protocol_version} · {authLabel(connection.auth_mode)}</small></span><span className="connection-stat"><strong>{connectionTools.length}</strong><small>imported tools</small></span><span className="connection-stat"><strong>{connection.last_synced_at ? new Date(connection.last_synced_at).toLocaleDateString() : "Never"}</strong><small>last inspected</small></span><Button outline disabled={busy} onClick={() => onInspect(connection)}><RefreshCw data-slot="icon" />Inspect & import</Button></article>;
      })}
      {connections.length === 0 && <div className="empty-row">No upstream MCP is connected. Add one to inspect and review its catalog.</div>}
    </section>
    <div className="identity-flow"><span><LockKeyhole /><strong>1 · DokoSoko identity</strong><small>Authenticate the user and resolve vendor entitlements.</small></span><span><ShieldCheck /><strong>2 · Post-authz policy</strong><small>Validate schema, confirmation, entitlement, and operation hook.</small></span><span><Users /><strong>3 · Upstream identity</strong><small>Use a separate user grant or encrypted service credential—never the inbound token.</small></span></div>
  </>;
}

function ToolsView({ tools, onAdd, onPublish, onNavigate }: { tools: APITool[]; onAdd: () => void; onPublish: (tool: APITool) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Actions" title="Tools" description="Publish reviewed HTTP actions and imported Stateless MCPv2 tools behind one authorization boundary." action={<Button onClick={onAdd}><Plus data-slot="icon" />Create API tool</Button>} /><div className="notice"><ShieldCheck /><span><strong>Policy-wrapped execution.</strong> Every call is schema validated, entitlement-scoped, reauthorized, rate-limited, and audited before a fixed backend is reached.</span></div><div className="tool-grid">{tools.map((tool) => <article className={`panel tool-card ${tool.upstream_drifted ? "drifted" : ""}`} key={tool.id}><span className="tool-icon">{tool.backend_kind === "mcp" ? <Share2 /> : tool.namespace === "credentials" ? <KeyRound /> : <TerminalSquare />}</span><div><span className="tool-badges"><Badge color={tool.state === "published" ? "green" : "amber"}>{tool.state}</Badge><Badge color={tool.backend_kind === "mcp" ? "violet" : "zinc"}>{tool.backend_kind === "mcp" ? "Stateless MCPv2" : "HTTP"}</Badge>{tool.upstream_drifted && <Badge color="red">Schema drift</Badge>}</span><h3><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link">{tool.namespace}.{tool.name}</EntityLink></h3><code>{tool.backend_kind === "mcp" ? `upstream · ${tool.upstream_tool_name}` : `${tool.http_method} · fixed API hook`}</code>{tool.state === "draft" && !tool.upstream_drifted && <Button outline className="publish-tool" onClick={() => onPublish(tool)}>Publish</Button>}{tool.upstream_drifted && <small className="drift-warning">Re-inspect and review before republishing.</small>}</div><button className="more" type="button" aria-label={`Actions for ${tool.name}`}><MoreHorizontal /></button></article>)}<button type="button" className="new-tool-card" onClick={onAdd}><Plus /><strong>Add API tool</strong><span>Definition → schema → API action → authz → test → publish</span></button></div></>;
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

function SettingsView({ product, versions, pins, identity, llmProfiles, rootUsers, currentUser, onDoctor, onConfigureProduct, onConfigureIdentity, onConfigureLLM, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; versions: APIProductVersion[]; pins: APIProductVersionPin[]; identity: APIIdentity | null; llmProfiles: APILLMProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onConfigureProduct: () => void; onConfigureIdentity: () => void; onConfigureLLM: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <><PageHeading eyebrow="Administration" title="Platform settings" description="Configure deployment, storage, AI, identity, security, and operations." action={<Button outline onClick={onDoctor}><Activity data-slot="icon" />Run System Doctor</Button>} /><div className="settings-grid"><button type="button" className="settings-button" onClick={onConfigureProduct}><SettingsCard icon={<GitBranch />} title="Deployment discovery & releases" detail={`${versions.length} connector release${versions.length === 1 ? "" : "s"} · ${pins.length} scoped pin${pins.length === 1 ? "" : "s"} · default ${product.default_version_policy.toUpperCase()}`} status={product.description ? "Discoverable" : "Required"} /></button><SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted local object storage" status="Healthy" /><button type="button" className="settings-button" onClick={onConfigureLLM}><SettingsCard icon={<Bot />} title="LLM profiles & hardening" detail={`${llmProfiles.length} optional profile${llmProfiles.length === 1 ? "" : "s"} · model authority disabled`} status="Enforced" /></button><button type="button" className="settings-button" onClick={onConfigureIdentity}><SettingsCard icon={<Users />} title="Vendor identity" detail={identity ? `${identity.issuer} · ${identity.allowed_redirect_uris.length} redirect URI(s)` : "Configure OIDC and entitlement resolution"} status={identity ? "Configured" : "Required"} /></button><SettingsCard icon={<ShieldCheck />} title="Root users & audit" detail={`${activeRoots.length} MFA-protected root administrator${activeRoots.length === 1 ? "" : "s"} · append-only audit`} status="Secure" /></div><section className="panel identity-contract"><div className="panel-heading"><div><h2>OAuth and account hooks</h2><p>MCP or widget → DokoSoko → vendor identity, entitlements, policy, and ephemeral usage</p></div><Button onClick={onConfigureIdentity}>{identity ? "Edit identity" : "Configure identity"}</Button></div><div className="contract-grid"><span><small>Downstream client ID</small><code>{identity?.product_id ?? "Deployment ID"}</code></span><span><small>Vendor issuer</small><code>{identity?.issuer ?? "Not configured"}</code></span><span><small>Installation claim</small><code>{identity?.installation_claim || "Not configured"}</code></span><span><small>Entitlement hook</small><code>{identity?.entitlement_hook_url || "No hook configured"}</code></span><span><small>Usage hook</small><code>{identity?.usage_hook_url || "No hook configured"}</code></span></div></section><section className="panel root-management"><div className="panel-heading"><div><h2>Root administrators</h2><p>Root access is independent from vendor identities and always requires MFA.</p></div><Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button></div>{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section></>;
}

function WarningContent({ children }: { children: React.ReactNode }) { return <div className="warning-content"><div className="warning-icon"><TriangleAlert /></div><div>{children}</div></div>; }
function Confirmation({ checked, onChange, children }: { checked: boolean; onChange: (checked: boolean) => void; children: React.ReactNode }) { return <label className="confirmation"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span className="check-box">{checked && <Check />}</span><span>{children}</span></label>; }
function SummaryItem({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) { return <div className="summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>; }
function Metric({ label, value, detail, positive }: { label: string; value: string; detail: string; positive?: boolean }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small className={positive ? "positive" : ""}>{detail}</small></article>; }
function ChecklistItem({ done = false, label }: { done?: boolean; label: string }) { return <div className="checklist-item"><span className={done ? "done" : ""}>{done && <Check />}</span><p>{label}</p>{done ? <Badge color="green">Complete</Badge> : <Badge color="zinc">Required</Badge>}</div>; }
function QuickAction({ icon, title, detail, onClick }: { icon: React.ReactNode; title: string; detail: string; onClick: () => void }) { return <button type="button" className="quick-action" onClick={onClick}><span>{icon}</span><span><strong>{title}</strong><small>{detail}</small></span><ExternalLink /></button>; }
function ChannelRow({ label, value, percent, color }: { label: string; value: string; percent: number; color: string }) { return <div className="channel-row"><div><span>{label}</span><strong>{value}</strong></div><div className="progress"><span className={color} style={{ width: `${percent}%` }} /></div><small>{percent}%</small></div>; }
function SettingsCard({ icon, title, detail, status }: { icon: React.ReactNode; title: string; detail: string; status: string }) { return <article className="panel settings-card"><span className="settings-icon">{icon}</span><div><h3>{title}</h3><p>{detail}</p></div><Badge color="green">{status}</Badge><ExternalLink /></article>; }
