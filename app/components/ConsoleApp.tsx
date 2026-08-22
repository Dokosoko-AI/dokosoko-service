"use client";

import {
  Activity,
  AlertCircle,
  ArrowLeft,
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
  GitBranch,
  Globe2,
  KeyRound,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  MoreHorizontal,
  MessageSquareText,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Server,
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
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIAnalytics, APIAuditEvent, APIBackendConnection, APICustomerAccount, APIDeployment, APIEnvironment, APIError, APIIdentity, APIIntegration, APIIntegrationPublishStatus, APIIntegrationRevision, APIIntegrationRun, APILLMProfile, APIMCPCatalog, APIMCPConnection, APIProduct, APIProductBuild, APIProductBuildInput, APIProductDefinition, APIProductInstallation, APIProductVersion, APIProductVersionDiff, APIProductVersionImpact, APIProductVersionPin, APIProductVersionPinHistory, APIResourceSet, APISupportRoute, APISupportSubmission, APITool, APIUser, APIWidgetSnippets, Distribution, SetupEnrollment, api } from "../lib/api";
import { ConsoleRoute, EntityKind, INTEGRATION_TABS, IntegrationTab, Section, entityPath, integrationPath, parseConsolePath, routeForSection, sectionPath } from "../lib/console-routes";
import { Badge, Button, Dialog, Switch } from "./catalyst";

type NavigationGroup = "apis" | "agent-access" | "activity";
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
  { id: "agent-access", label: "Agent access", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", label: "Agent access" }] },
  { id: "activity", label: "Activity", icon: Activity, defaultSection: "runs", sections: [{ id: "runs", label: "Activity" }] },
];

const initialSources: Source[] = [
  { id: "src_docs", name: "Developer documentation", kind: "Website", location: "docs.acme.dev", visibility: "private", published: true, quarantined: false, crawlState: "synced", pages: 284, lastCrawl: "12 min ago", revision: 1 },
  { id: "src_api", name: "Platform API", kind: "OpenAPI", location: "api/openapi.yaml", visibility: "private", published: false, quarantined: false, crawlState: "review", pages: 94, lastCrawl: "2 hours ago", revision: 1 },
  { id: "src_examples", name: "SDK examples", kind: "Git repository", location: "acme/sdk-examples", visibility: "private", published: false, quarantined: false, crawlState: "failed", pages: 0, lastCrawl: "1 day ago", revision: 1 },
];

const initialTools: APITool[] = [
  { id: "tool_sandbox", organisation_id: "org_acme", product_id: "prod_acme", namespace: "access", name: "create_sandbox", description: "Create a sandbox", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_credentials", organisation_id: "org_acme", product_id: "prod_acme", namespace: "credentials", name: "issue", description: "Issue credentials", input_schema: {}, output_schema: {}, state: "draft", revision: 1, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_incidents", organisation_id: "org_acme", product_id: "prod_acme", namespace: "support", name: "create_incident", description: "Create a support incident", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "MCP", authorization_policy: { required_grants: ["support.write"] }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: "mcp_support", upstream_tool_name: "incidents.create", upstream_schema_hash: "sha256:8f44e6" },
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
    { kind: "openapi", name: "Messages OpenAPI", location: "https://api.acme.dev/messages/v2/openapi.yaml", version: "v2" },
    { kind: "docs", name: "Messages documentation", location: "https://docs.acme.dev/messages/v2", version: "v2" },
    { kind: "mcp", name: "Acme tools", location: "https://mcp.acme.dev/v2", version: "2026-07-28" },
  ],
  proposal: fixtureDefinition,
  unresolved: [],
  created_at: "2026-08-19T12:00:00Z",
  completed_at: "2026-08-19T12:00:08Z",
};

const fixtureDiff: APIProductVersionDiff = { from_version_id: "version_2026_05", from_version: "2026.5", generated_at: "2026-08-19T12:20:00Z", summary: "1 added, 0 removed, 2 changed", added: [{ kind: "artifact", path: "capability/voice/artifact/tool/voice.calls.transfer", after: "v3" }], removed: [], changed: [{ kind: "artifact", path: "capability/voice/artifact/openapi", before: "v2", after: "v3" }, { kind: "artifact", path: "capability/messages/artifact/docs", before: "v1", after: "v2" }] };
const fixtureProductVersions: APIProductVersion[] = [
  { id: "version_2026_08", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.8", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:81f4…b9c2", diff: fixtureDiff, release_stage: "active", rollout_percentage: 25, promotion_state: "approved", requested_latest: true, requested_lts: false, approved_by: "root_approver", approved_at: "2026-08-19T12:19:00Z", drift_status: "healthy", drift_details: [], drift_checked_at: "2026-08-19T12:19:30Z", is_latest: true, is_lts: false, revision: 2, published_at: "2026-08-19T12:20:00Z" },
  { id: "version_2026_05", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.5", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:17bd…81a0", diff: { ...fixtureDiff, from_version_id: undefined, from_version: undefined, summary: "Initial product release", added: [], changed: [] }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: true, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: true, revision: 3, published_at: "2026-05-10T09:00:00Z" },
  { id: "version_2025_11", organisation_id: "org_acme", product_id: "prod_acme", version: "2025.11", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:02aa…4d31", diff: { ...fixtureDiff, summary: "0 added, 1 removed, 1 changed" }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: false, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: false, deprecated_at: "2026-08-01T00:00:00Z", deprecation_message: "Move to 2026.5 LTS or 2026.8 latest.", replacement_version: "2026.5", sunset_at: "2026-12-01T00:00:00Z", revision: 4, published_at: "2025-11-12T09:00:00Z" },
];

const fixtureProductPins: APIProductVersionPin[] = [
  { id: "pin_contoso", organisation_id: "org_acme", product_id: "prod_acme", scope: "customer", scope_id: "account_contoso", customer_account_id: "account_contoso", product_version_id: "version_2026_05", product_version: "2026.5", reason: "Production stability window", revision: 1, created_at: "2026-08-19T12:30:00Z", updated_at: "2026-08-19T12:30:00Z" },
];

const fixtureCustomerAccounts: APICustomerAccount[] = [{ id: "account_contoso", organisation_id: "org_acme", product_id: "prod_acme", issuer: "https://identity.acme.example", external_id: "contoso", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z", last_authenticated_at: "2026-08-19T12:24:00Z" }];
const fixtureInstallations: APIProductInstallation[] = [{ id: "installation_contoso_voice", organisation_id: "org_acme", product_id: "prod_acme", customer_account_id: "account_contoso", environment_id: "env_prod", external_id: "contoso-voice-prod", name: "Contoso voice production", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z" }];

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

function escapeEmbedHTML(value: string) {
  return value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character] ?? character);
}

const agentClients = [
  { id: "codex", name: "Codex", file: "codex.svg" },
  { id: "claude-code", name: "Claude Code", file: "claude-code.svg" },
  { id: "cursor", name: "Cursor", file: "cursor.svg" },
  { id: "opencode", name: "OpenCode", file: "opencode.svg" },
] as const;

function agentClientAssetURL(setupURL: string, filename: string) {
  let origin = "";
  try {
    origin = new URL(setupURL).origin;
  } catch {
    if (typeof window !== "undefined") origin = window.location.origin;
  }
  return `${origin}/agent-client-icons/${filename}`;
}

function buildAgentSetupEmbedHTML(tenantName: string, setupURL: string, kind: "public" | "private") {
  const name = escapeEmbedHTML(tenantName);
  const url = escapeEmbedHTML(setupURL);
  const label = kind === "public" ? "Public" : "Private";
  const chipColor = kind === "public" ? "#4338ca" : "#3f3f46";
  const chipBackground = kind === "public" ? "#eef2ff" : "#f4f4f5";
  const clients = agentClients.map((client) => `<img src="${escapeEmbedHTML(agentClientAssetURL(setupURL, client.file))}" alt="${client.name}" title="${client.name}" data-agent-client="${client.id}" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain">`).join("");
  return `<a href="${url}" target="_blank" rel="noopener noreferrer" data-dokosoko-agent-setup="${kind}" aria-label="Connect your agent to ${name} using ${kind} MCP" style="display:inline-flex;align-items:center;gap:10px;min-height:52px;padding:0 18px;border:1px solid #d4d4d8;border-radius:999px;color:#18181b;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.08);font:600 16px/1.2 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont;&quot;Segoe UI&quot;,sans-serif;text-decoration:none"><span>Connect your agent to ${name}</span><span style="padding:4px 8px;border-radius:999px;color:${chipColor};background:${chipBackground};font-size:11px;font-weight:700;letter-spacing:.04em;text-transform:uppercase">${label}</span>${clients}</a>`;
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
	const [backendConnections, setBackendConnections] = useState<APIBackendConnection[]>([]);
	const [accessInstances, setAccessInstances] = useState<APIAccessInstance[]>([]);
	const [accessCredentials, setAccessCredentials] = useState<APIAccessCredential[]>([]);
	const [supportRoutes, setSupportRoutes] = useState<APISupportRoute[]>([]);
  const [consoleRoute, setConsoleRoute] = useState<ConsoleRoute>(() => routeForSection("product"));
  const section = consoleRoute.section;
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtureDefinition);
  const [, setLatestProductBuild] = useState<APIProductBuild | null>(fixtureProductBuild);
  const [productBuilderOpen, setProductBuilderOpen] = useState(false);
  const [productBuilderBusy, setProductBuilderBusy] = useState(false);
  const [productBuilderInputs, setProductBuilderInputs] = useState("");
  const [sources, setSources] = useState(initialSources);
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
  const [mcpGrants, setMCPGrants] = useState("");
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
  const [addToolOpen, setAddToolOpen] = useState(false);
  const [toolNamespace, setToolNamespace] = useState("access");
  const [toolName, setToolName] = useState("");
  const [toolDescription, setToolDescription] = useState("");
  const [toolMethod, setToolMethod] = useState("POST");
  const [toolEndpoint, setToolEndpoint] = useState("");
  const [toolGrants, setToolGrants] = useState("");
  const [toolInputSchema, setToolInputSchema] = useState(`{"type":"object","additionalProperties":false,"properties":{},"required":[]}`);
  const [toolOutputSchema, setToolOutputSchema] = useState(`{"type":"object","additionalProperties":false,"properties":{}}`);
	  const [toolBusy, setToolBusy] = useState(false);
	  const [analytics, setAnalytics] = useState<APIAnalytics | null>(null);
	  const [identityConfig, setIdentityConfig] = useState<APIIdentity | null>(null);
	  const [reportSubmissions, setReportSubmissions] = useState<APISupportSubmission[]>([]);
	  const [reportDetail, setReportDetail] = useState<APISupportSubmission | null>(null);
	  const [reportDetailBusy, setReportDetailBusy] = useState(false);
	  const [rootUsers, setRootUsers] = useState<APIUser[]>(currentUser ? [currentUser] : []);
	  const [identityOpen, setIdentityOpen] = useState(false);
	  const [identityBusy, setIdentityBusy] = useState(false);
	  const [idpIssuer, setIDPIssuer] = useState("");
	  const [idpClientID, setIDPClientID] = useState("");
	  const [idpClientSecret, setIDPClientSecret] = useState("");
	  const [idpScopes, setIDPScopes] = useState("openid, profile, email");
	  const [idpAudience, setIDPAudience] = useState("");
	  const [idpOAuthResource, setIDPOAuthResource] = useState("");
	  const [idpOrganisationClaim, setIDPOrganisationClaim] = useState("org_id");
	  const [idpInstallationClaim, setIDPInstallationClaim] = useState("installation_id");
	  const [delegatedAPIOrigin, setDelegatedAPIOrigin] = useState("");
	  const [identityState, setIdentityState] = useState<APIIdentity["state"]>("active");
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
	  const [customerAccounts, setCustomerAccounts] = useState<APICustomerAccount[]>(fixtureCustomerAccounts);
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
    const syncRoute = () => {
      const next = parseConsolePath(window.location.pathname);
      if (next.kind !== "not-found" && window.location.pathname !== next.path) {
        window.history.replaceState(null, "", `${next.path}${window.location.search}${window.location.hash}`);
      }
      setConsoleRoute(next);
    };
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
	    Promise.all([api.distribution(product.id), api.widgets(product.id), api.sources(product.id), api.tools(product.id), api.mcpConnections(product.id)]).then(([distributionValue, widgetValues, remoteSources, remoteTools, remoteMCPConnections]) => {
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
      setTools(remoteTools);
      setMCPConnections(remoteMCPConnections);
      setAPIConnected(true);
	    }).catch(() => {
      // The standalone static preview intentionally keeps its local fixture. In the
      // service deployment, same-origin session authentication hydrates live state.
	    });
	    api.analytics(product.id).then((value) => { if (!cancelled) setAnalytics(value); }).catch(() => {});
	    api.identity().then((value) => {
	      if (cancelled) return;
	      setIdentityConfig(value);
	      setIDPIssuer(value.issuer); setIDPClientID(value.client_id); setIDPScopes(value.scopes.join(", ")); setIDPAudience(value.audience ?? ""); setIDPOAuthResource(value.oauth_resource ?? ""); setIDPOrganisationClaim(value.organisation_claim); setIDPInstallationClaim(value.installation_claim); setDelegatedAPIOrigin(value.delegated_api_origin); setIdentityState(value.state);
	    }).catch(() => {});
	    api.supportSubmissions().then((submissions) => {
	      if (!cancelled) setReportSubmissions(submissions);
	    }).catch(() => {});
	    api.rootUsers().then((value) => { if (!cancelled) setRootUsers(value); }).catch(() => {});
	    Promise.all([api.integrations(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.backendConnections(), api.supportRoutes()]).then(async ([integrationValues, setValues, definitionValues, connectionValues, backendValues, routeValues]) => {
	      if (cancelled) return;
	      setIntegrations(integrationValues); setResourceSets(setValues); setAccessDefinitions(definitionValues); setAccessConnections(connectionValues); setBackendConnections(backendValues); setSupportRoutes(routeValues);
	      const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
	      const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
	      if (!cancelled) { setAccessInstances(instanceGroups.flat()); setAccessCredentials(credentialGroups.flat()); }
	    }).catch(() => {});
	    api.llmProfiles(product.id).then((values) => { if (!cancelled) setLLMProfiles(values); }).catch(() => {});
	    api.productDefinition(product.id).then((value) => { if (!cancelled) setProductDefinition(value); }).catch((error) => { if (!cancelled && error instanceof APIError && error.status === 404) setProductDefinition(null); });
	    api.productBuilds(product.id).then((values) => { if (!cancelled) setLatestProductBuild(values[0] ?? null); }).catch(() => {});
	    api.productVersions(product.id).then((values) => { if (!cancelled) { setProductVersions(values); setPinVersionID(values.find((value) => value.is_latest)?.id ?? values[0]?.id ?? ""); } }).catch(() => {});
	    api.productVersionPins(product.id).then((values) => { if (!cancelled) setProductVersionPins(values); }).catch(() => {});
	    Promise.all([api.productInstallations(product.id), api.productVersionPinHistory(product.id), api.customerAccounts(product.id)]).then(([installationValues, historyValues, accountValues]) => { if (!cancelled) { setProductInstallations(installationValues); setPinHistory(historyValues); setCustomerAccounts(accountValues); } }).catch(() => {});
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
		const [integrationValues, setValues, definitionValues, connectionValues, backendValues, routeValues] = await Promise.all([api.integrations(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.backendConnections(), api.supportRoutes()]);
		setIntegrations(integrationValues);
		setResourceSets(setValues);
		setAccessDefinitions(definitionValues);
		setAccessConnections(connectionValues);
		setBackendConnections(backendValues);
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
        : { ...fixtureProductBuild, id: fallbackBuildID, state: "review" as const, created_at: new Date().toISOString(), completed_at: new Date().toISOString(), inputs: [...fixtureProductBuild.inputs, ...additionalInputs], proposal: { ...fixtureDefinition, state: "draft" as const, source_build_id: fallbackBuildID } };
      setLatestProductBuild(value);
      setProductBuilderOpen(false);
      setProductBuilderInputs("");
      navigateToSection("product");
      showToast(`${value.inputs.length} sources scanned. Review ${value.unresolved.length || "no"} exception${value.unresolved.length === 1 ? "" : "s"}.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "The APIs could not be imported.");
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

  async function createTool() {
    setToolBusy(true);
    try {
      const inputSchema = JSON.parse(toolInputSchema) as Record<string, unknown>;
      const outputSchema = JSON.parse(toolOutputSchema) as Record<string, unknown>;
      const authorizationPolicy = { required_grants: toolGrants.split(",").map((value) => value.trim()).filter(Boolean), confirmation_required: false };
      const created = apiConnected ? await api.createTool(product.id, { organisation_id: product.organisation_id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, endpoint: toolEndpoint, http_method: toolMethod, authorization_policy: authorizationPolicy, timeout_ms: 10000 }) : { id: `tool_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, state: "draft" as const, revision: 1, http_method: toolMethod, authorization_policy: authorizationPolicy, timeout_ms: 10000 };
      setTools((items) => [...items, created as APITool]);
      setAddToolOpen(false);
      setToolName(""); setToolDescription(""); setToolEndpoint(""); setToolGrants("");
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
      const grants = mcpGrants.split(",").map((value) => value.trim()).filter(Boolean);
      if (apiConnected) {
        const result = await api.importMCPTools(product.id, mcpCatalog.connection.id, { tool_names: mcpSelectedTools, required_grants: grants, confirmation_required: mcpConfirmationRequired, timeout_ms: 10000 });
        const changed = [...result.created, ...result.updated, ...result.unchanged, ...result.drifted];
        setTools((items) => [...items.filter((item) => !changed.some((candidate) => candidate.id === item.id)), ...changed]);
        setMCPConnections((items) => items.map((item) => item.id === result.connection.id ? result.connection : item));
      } else {
        const imported = mcpCatalog.tools.filter((item) => mcpSelectedTools.includes(item.name)).map((item, index): APITool => ({ id: `tool_mcp_${index}`, organisation_id: product.organisation_id, product_id: product.id, namespace: mcpCatalog.connection.namespace, name: item.name.replace(/[^A-Za-z0-9_]/g, "_"), description: item.description ?? item.title ?? item.name, input_schema: item.input_schema, output_schema: item.output_schema ?? {}, state: "draft", revision: 1, http_method: "MCP", authorization_policy: { required_grants: grants, confirmation_required: mcpConfirmationRequired }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: mcpCatalog.connection.id, upstream_tool_name: item.name, upstream_schema_hash: item.schema_hash }));
        setTools((items) => [...items, ...imported]);
      }
      setMCPImportOpen(false);
      setMCPGrants("");
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
        issuer: idpIssuer,
        client_id: idpClientID,
        client_secret: idpClientSecret,
        scopes: idpScopes.split(",").map((value) => value.trim()).filter(Boolean),
        audience: idpAudience,
		oauth_resource: idpOAuthResource,
		organisation_claim: idpOrganisationClaim,
		installation_claim: idpInstallationClaim,
		delegated_api_origin: delegatedAPIOrigin,
		state: identityState,
        revision: identityConfig?.revision ?? 0,
      };
      const value = apiConnected ? await api.configureIdentity(input) : { id: "idp_preview", organisation_id: product.organisation_id, deployment_id: product.id, ...input } as APIIdentity;
      setIdentityConfig(value);
      setIDPClientSecret("");
      setIdentityOpen(false);
      showToast("Customer identity integration is configured.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not configure vendor identity.");
    } finally {
      setIdentityBusy(false);
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

  const publicEndpoint = distribution?.public_mcp_endpoint ?? "/mcp/public";
  const publicSnippet = widgetSnippets?.public.snippet ?? `<script async src="/widgets/${product.id}/public.js" data-product="${product.id}"></script>`;
  const privateSnippet = widgetSnippets?.private.snippet ?? `<script async src="/widgets/${product.id}/private.js" data-product="${product.id}"></script>`;
  const publicAgentSetupURL = distribution?.agent_setup?.public.url ?? "/agent-setup/public/prompt.md";
  const privateAgentSetupURL = distribution?.agent_setup?.private.url ?? "/agent-setup/private/prompt.md";
  const publicAgentSetup = distribution?.agent_setup?.public ?? { available: publicMCPEnabled, unavailable_reason: "public_mcp_disabled" as const, url: publicAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(product.name, publicAgentSetupURL, "public"), contains_secret: false as const };
  const privateAgentSetup = distribution?.agent_setup?.private ?? { available: identityConfig?.state === "active", unavailable_reason: "identity_unavailable" as const, url: privateAgentSetupURL, embed_html: buildAgentSetupEmbedHTML(product.name, privateAgentSetupURL, "private"), contains_secret: false as const };
  const mcpConnectionReady = Boolean(mcpName.trim() && mcpNamespace.trim() && mcpEndpoint.trim() && (mcpAuthMode !== "service" || mcpCredential.trim()) && (mcpAuthMode !== "delegated_oauth" || (mcpOAuthClientID.trim() && mcpOAuthClientSecret.trim() && mcpOAuthIssuer.trim() && mcpAuthorizationURL.trim() && mcpTokenURL.trim())));
  const activeNavigation = navigation.find((item) => item.sections.some((candidate) => candidate.id === section));
  const entityDetail = useMemo<EntityDetail | null>(() => {
    if (consoleRoute.kind !== "entity") return null;
    const date = (value?: string) => value ? new Date(value).toLocaleString() : "—";
    const fields = (values: Array<[string, unknown]>) => values.map(([label, value]) => ({ label, value: value === undefined || value === null || value === "" ? "—" : String(value) }));
    switch (consoleRoute.entity) {
      case "integration": {
        const value = integrations.find((item) => item.id === consoleRoute.uid);
        return value ? { eyebrow: "API", title: value.display_name, description: `${value.family_key} · ${value.version_key}`, fields: fields([["API ID", value.id], ["Lifecycle", value.lifecycle], ["Revision", value.revision], ["Resources", value.resources?.length ?? 0], ["Access connections", value.access_connection_ids?.length ?? 0], ["Sunset", date(value.sunset_at)]]) } : null;
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
  }, [consoleRoute, integrations, resourceSets, sources, tools, mcpConnections, accessDefinitions, accessConnections, productInstallations, productVersions, integrationRuns, supportRoutes, reportSubmissions, auditEvents, rootUsers]);

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
          <select className="mobile-navigation" aria-label="Console section" value={section === "settings" ? "settings" : activeNavigation?.id ?? "apis"} onChange={(event) => navigateToGroup(event.target.value as NavigationGroup | "settings")}>
            {navigation.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            <option value="settings">Settings</option>
          </select>
          <div className="environment"><span className="status-dot" />Production</div>
        </header>

        <div className="content">
          {consoleRoute.kind === "not-found" ? <ConsoleNotFoundView path={consoleRoute.path} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "integration" ? <IntegrationsView integrations={integrations} resourceSets={resourceSets} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} mcpConnections={mcpConnections} identity={identityConfig} selectedIntegrationID={consoleRoute.uid} activeTab={consoleRoute.integrationTab} onBuild={() => setProductBuilderOpen(true)} onAddTool={() => setAddToolOpen(true)} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" ? <EntityDetailView route={consoleRoute} detail={entityDetail} onNavigate={navigateToPath} /> : <>
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
              tenantName={product.name}
              publicAgentSetup={publicAgentSetup}
              privateAgentSetup={privateAgentSetup}
              onConfigureIdentity={() => navigateToSection("settings")}
              onOpenSources={() => navigateToSection("sources")}
            />
          )}
          {section === "product" && <IntegrationsView integrations={integrations} resourceSets={resourceSets} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} mcpConnections={mcpConnections} identity={identityConfig} onBuild={() => setProductBuilderOpen(true)} onAddTool={() => setAddToolOpen(true)} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "sources" && <SourcesView sources={sources} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} onNavigate={navigateToPath} />}
          {section === "projects" && <AccessView definitions={accessDefinitions} connections={accessConnections} instances={accessInstances} credentials={accessCredentials} integrations={integrations} environments={environments} apiResourceSets={resourceSets.filter((set) => set.kind === "api")} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} onNavigate={navigateToPath} />}
          {section === "tools" && <ToolsView tools={tools} onAdd={() => setAddToolOpen(true)} onPublish={publishTool} onNavigate={navigateToPath} />}
          {section === "releases" && <ConnectorReleasesView versions={productVersions} integrations={integrations} onConfigure={openProductCatalog} onNavigate={navigateToPath} />}
          {section === "runs" && <ActivityHubView runs={integrationRuns} environments={environments} submissions={reportSubmissions} events={auditEvents} analytics={analytics} supportRoutes={supportRoutes} onStart={() => setRunOpen(true)} onComplete={completeIntegrationRun} onView={openSupportSubmission} onRetry={createSupportDeliveryAttempt} onNavigate={navigateToPath} />}
          {section === "reporting" && <ReportingView routes={supportRoutes} integrations={integrations} backendConnections={backendConnections} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "settings" && <SettingsView product={product} versions={productVersions} pins={productVersionPins} customerAccounts={customerAccounts} identity={identityConfig} llmProfiles={llmProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onConfigureProduct={openProductCatalog} onConfigureIdentity={() => setIdentityOpen(true)} onConfigureLLM={() => setLLMOpen(true)} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
          </>}
        </div>
      </main>

      <Dialog
        open={Boolean(pendingPublication)}
        onClose={(open) => { if (!open) setPendingPublication(null); }}
        title={`Make ${pendingPublication?.name ?? "resource"} public?`}
        description="This is a security-sensitive publication change. Private is the default for every new source."
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
        </div>
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
        onClose={setAddSourceOpen}
        title="Add knowledge source"
        description="The source starts private and draft. A crawl produces immutable snapshots for review before publication."
        actions={<><Button outline onClick={() => setAddSourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={sourceBusy || !sourceName.trim() || !sourceLocation.trim()} onClick={createSource}>{sourceBusy ? "Adding…" : "Add source"}</Button></>}
      >
        <div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Developer documentation" /></label><label className="auth-field"><span>Type</span><select value={sourceKind} onChange={(event) => setSourceKind(event.target.value)}><option value="website">Website</option><option value="openapi">OpenAPI</option><option value="git">Git repository</option></select></label><label className="auth-field"><span>Location</span><input type="url" value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder="https://docs.example.com" /></label><div className="private-default-note"><LockKeyhole />Private by default. Making it public is a separate guarded action.</div></div>
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
        open={addToolOpen}
        onClose={setAddToolOpen}
        title="Create API tool"
        description="Define the MCP contract and one fixed API action. Agents cannot alter the host or authorization header."
        actions={<><Button outline onClick={() => setAddToolOpen(false)}>Cancel</Button><Button color="indigo" disabled={toolBusy || !toolName.trim() || !toolDescription.trim() || !toolEndpoint.trim()} onClick={createTool}>{toolBusy ? "Validating…" : "Save draft"}</Button></>}
      >
        <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Namespace</span><input value={toolNamespace} onChange={(event) => setToolNamespace(event.target.value)} /></label><label className="auth-field"><span>Tool name</span><input value={toolName} onChange={(event) => setToolName(event.target.value)} placeholder="create_sandbox" /></label></div><label className="auth-field"><span>Description</span><input value={toolDescription} onChange={(event) => setToolDescription(event.target.value)} /></label><label className="auth-field"><span>Input JSON Schema</span><textarea value={toolInputSchema} onChange={(event) => setToolInputSchema(event.target.value)} /></label><label className="auth-field"><span>Output JSON Schema</span><textarea value={toolOutputSchema} onChange={(event) => setToolOutputSchema(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Method</span><select value={toolMethod} onChange={(event) => setToolMethod(event.target.value)}>{["GET", "POST", "PUT", "PATCH", "DELETE"].map((value) => <option key={value}>{value}</option>)}</select></label><label className="auth-field"><span>Vendor API endpoint</span><input type="url" value={toolEndpoint} onChange={(event) => setToolEndpoint(event.target.value)} placeholder="https://api.vendor.com/v1/action" /><small>Must use the configured vendor API origin. The developer&apos;s delegated bearer token is forwarded.</small></label></div><label className="auth-field"><span>Required grants</span><input value={toolGrants} onChange={(event) => setToolGrants(event.target.value)} placeholder="sandboxes.create, developer.pro" /><small>Comma-separated grant keys. Access-evaluation failures deny execution.</small></label></div>
      </Dialog>

      <Dialog
        open={identityOpen}
        onClose={setIdentityOpen}
        title="Customer identity integration"
        description="Optional OIDC and delegated-user API configuration. Backend delivery credentials are managed separately."
        actions={<><Button outline onClick={() => setIdentityOpen(false)}>Cancel</Button><Button color="indigo" disabled={identityBusy || !idpIssuer.trim() || !idpClientID.trim() || (!identityConfig && !idpClientSecret.trim()) || !delegatedAPIOrigin.trim()} onClick={saveIdentity}>{identityBusy ? "Verifying…" : "Save identity"}</Button></>}
      >
        <div className="auth-form compact-form">
          <label className="auth-field"><span>OIDC issuer</span><input type="url" value={idpIssuer} onChange={(event) => setIDPIssuer(event.target.value)} placeholder="https://identity.vendor.com" /></label>
          <div className="two-fields"><label className="auth-field"><span>OIDC client ID</span><input value={idpClientID} onChange={(event) => setIDPClientID(event.target.value)} /></label><label className="auth-field"><span>{identityConfig ? "Rotate client secret (optional)" : "OIDC client secret"}</span><input type="password" autoComplete="off" value={idpClientSecret} onChange={(event) => setIDPClientSecret(event.target.value)} /></label></div>
          <label className="auth-field"><span>Scopes</span><input value={idpScopes} onChange={(event) => setIDPScopes(event.target.value)} /></label>
		  <div className="two-fields"><label className="auth-field"><span>Audience (optional)</span><input value={idpAudience} onChange={(event) => setIDPAudience(event.target.value)} /></label><label className="auth-field"><span>OAuth resource (optional)</span><input type="url" value={idpOAuthResource} onChange={(event) => setIDPOAuthResource(event.target.value)} /></label></div>
		  <label className="auth-field"><span>Organisation claim</span><input value={idpOrganisationClaim} onChange={(event) => setIDPOrganisationClaim(event.target.value)} /></label>
		  <label className="auth-field"><span>Installation claim (optional)</span><input value={idpInstallationClaim} onChange={(event) => setIDPInstallationClaim(event.target.value)} placeholder="installation_id" /><small>When present, this authenticated claim must select a registered installation for the resolved customer account.</small></label>
          <div className="two-fields"><label className="auth-field"><span>Delegated API origin</span><input type="url" value={delegatedAPIOrigin} onChange={(event) => setDelegatedAPIOrigin(event.target.value)} placeholder="https://customer-api.vendor.com" /><small>Used only with the customer&apos;s delegated token for access evaluation and tools.</small></label><label className="auth-field"><span>State</span><select value={identityState} onChange={(event) => setIdentityState(event.target.value as APIIdentity["state"])}><option value="active">Active</option><option value="disabled">Disabled</option></select></label></div>
          <div className="private-default-note"><ShieldCheck />OIDC creates durable customer accounts from the configured organisation claim. Access is evaluated through the fixed vendor API contract, and tool calls use the developer&apos;s delegated token.</div>
        </div>
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
      <ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {route.section === "product" ? "APIs" : route.section === "projects" ? "Service connections" : route.section}</ConsoleLink>
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
  return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">Navigation</p><h1>Page not found</h1><p><code>{path}</code> is not a recognised console URL.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;
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
  tenantName,
  publicAgentSetup,
  privateAgentSetup,
  onConfigureIdentity,
  onOpenSources,
}: {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  resources: Array<{ id: string; name: string; resourceType: "source"; type: string; detail: string; visibility: Visibility }>;
  resourceFilter: "all" | "public" | "private";
  setResourceFilter: (filter: "all" | "public" | "private") => void;
  publicResourceCount: number;
  onVisibilityChange: (kind: "source", id: string) => void;
  onCopied: (label: string) => void;
  publicSnippet: string;
  privateSnippet: string;
  publicEndpoint: string;
  tenantName: string;
  publicAgentSetup: Distribution["agent_setup"]["public"];
  privateAgentSetup: Distribution["agent_setup"]["private"];
  onConfigureIdentity: () => void;
  onOpenSources: () => void;
}) {
  return <>
    <PageHeading eyebrow="Delivery" title="Agent access" description="Control how authenticated and public agents reach your APIs and knowledge." action={<Button outline disabled={!privateAgentSetup.available} onClick={() => window.open(privateAgentSetup.url, "_blank", "noopener,noreferrer")}><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>Public MCP</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? "Live" : "Off"}</Badge></div><p>Offer an authentication-free, read-only MCP endpoint. Its server-side policy can retrieve only published sources that you explicitly mark public.</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label="Copy public MCP endpoint" onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied("Public MCP endpoint copied."); }}><Copy />Copy</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label="Enable Public MCP" /><small>{enabled ? "Accepting anonymous requests" : "Disabled by default"}</small></div>
    </section>

    <section className="section-block agent-setup-section">
      <div className="section-heading"><div><h2>Copy MCP button</h2><p>Add a secret-free MCP connection button to your developer portal. It opens exact setup instructions for Codex, Claude Code, Cursor, and OpenCode.</p></div></div>
      <div className="agent-setup-grid">
        <AgentSetupCard kind="public" tenantName={tenantName} setup={publicAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
        <AgentSetupCard kind="private" tenantName={tenantName} setup={privateAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
      </div>
    </section>

    <section className="section-block">
      <div className="section-heading"><div><h2>Resource visibility</h2><p>{publicResourceCount} public. Private is the default; changing to public always requires confirmation.</p></div><Button outline onClick={onOpenSources}>Manage sources</Button></div>
      <div className="filter-tabs" role="group" aria-label="Filter resources">
        {(["all", "public", "private"] as const).map((filter) => <button type="button" key={filter} className={resourceFilter === filter ? "active" : ""} onClick={() => setResourceFilter(filter)}>{filter[0].toUpperCase() + filter.slice(1)}</button>)}
      </div>
      <div className="resource-table">
        <div className="table-head resource-columns"><span>Resource</span><span>Type</span><span>Visibility</span><span /></div>
        {resources.map((resource) => <div className="table-row resource-columns" key={`${resource.resourceType}-${resource.id}`}>
          <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><strong>{resource.name}</strong><small>{resource.detail}</small></span></span>
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
          <div className="widget-copy"><Badge color="blue"><Globe2 />Public</Badge><h3>Public widget</h3><p>No sign-in. Answers only from public, published sources.</p>{!enabled && <div className="inline-warning"><TriangleAlert />Enable Public MCP before embedding.</div>}<CopyButton text={publicSnippet} label="Copy public widget" disabled={!enabled} onCopied={onCopied} /></div>
        </article>
        <article className="widget-card">
          <WidgetPreview kind="private" />
          <div className="widget-copy"><Badge color="violet"><LockKeyhole />Private</Badge><h3>Private widget</h3><p>Uses your identity flow for private knowledge, tools, provider resources, and credentials.</p><CopyButton text={privateSnippet} label="Copy private widget" onCopied={onCopied} /></div>
        </article>
      </div>
    </section>
  </>;
}

function AgentSetupCard({ kind, tenantName, setup, onCopied, onConfigureIdentity }: { kind: "public" | "private"; tenantName: string; setup: Distribution["agent_setup"]["public"]; onCopied: (label: string) => void; onConfigureIdentity: () => void }) {
  const isPublic = kind === "public";
  const title = isPublic ? "Public MCP button" : "Private MCP button";
  return <article className={`agent-setup-card ${!setup.available ? "agent-setup-disabled" : ""}`}>
    <div className={`agent-setup-preview ${isPublic ? "public-agent-preview" : "private-agent-preview"}`}>
      <a href={setup.available ? setup.url : undefined} target="_blank" rel="noopener noreferrer" aria-disabled={!setup.available} aria-label={`Connect your agent to ${tenantName} using ${kind} MCP`} onClick={(event) => { if (!setup.available) event.preventDefault(); }}>
        <span className="agent-setup-label">Connect your agent to {tenantName}</span>
        <span className={`agent-access-chip ${kind}`}>{isPublic ? "Public" : "Private"}</span>
        {/* eslint-disable-next-line @next/next/no-img-element -- These tiny vendor SVG marks are served unchanged from the public asset contract. */}
        {agentClients.map((client) => <img key={client.id} className="agent-client-mark" src={`/agent-client-icons/${client.file}`} alt={client.name} title={client.name} data-agent-client={client.id} />)}
      </a>
    </div>
    <div className="agent-setup-copy">
      <Badge color={isPublic ? "blue" : "violet"}>{isPublic ? <Globe2 /> : <LockKeyhole />}{isPublic ? "Public" : "Private"}</Badge>
      <h3>{title}</h3>
      <p>{isPublic ? "Anonymous, read-only access to explicitly public resources." : "Customer access through the configured identity provider and browser OAuth."}</p>
      {setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />Open setup instructions</a> : <div className="inline-warning"><TriangleAlert />{isPublic ? "Enable Public MCP before distributing this button." : "Configure and activate customer identity before distributing this button."}</div>}
      {!isPublic && !setup.available && <Button outline className="agent-identity-action" onClick={onConfigureIdentity}>Configure identity</Button>}
      <CopyButton text={setup.embed_html} label={`Copy ${kind} MCP button`} disabled={!setup.available} onCopied={() => onCopied(`${isPublic ? "Public" : "Private"} MCP button copied.`)} />
    </div>
  </article>;
}

function WidgetPreview({ kind }: { kind: "public" | "private" }) {
  const privateWidget = kind === "private";
  return <div className={`widget-preview ${privateWidget ? "dark-preview" : ""}`}><div className="mini-chat"><span className={`mini-brand ${privateWidget ? "light" : ""}`}>D</span><span><strong>{privateWidget ? "Acme developer assistant" : "Ask Acme"}</strong><small>{privateWidget ? "Signed in as Alex" : "Powered by DokoSoko"}</small></span><button type="button" aria-label="Close widget preview">×</button></div><div className={`mini-message ${privateWidget ? "dark-message" : ""}`}>{privateWidget ? "Show my sandbox credentials" : "How do I create an API key?"}</div><div className={`mini-answer ${privateWidget ? "dark-answer" : ""}`}>{privateWidget ? "I can provision credentials after checking your access." : "I can help with Acme&apos;s public documentation."}</div><div className={`mini-input ${privateWidget ? "dark-input" : ""}`}>Ask a question… <span>↑</span></div></div>;
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

function IntegrationDirectoryView({ integrations, connections, supportRoutes, query, onQueryChange, onCreate, onBuild, onNavigate }: { integrations: APIIntegration[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onBuild: () => void; onNavigate: (path: string) => void }) {
  const normalizedQuery = query.trim().toLowerCase();
  const connectionCount = (integration: APIIntegration) => connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id)).length;
  const hasSupport = (integration: APIIntegration) => Boolean(supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default));
  const setupIssueCount = (integration: APIIntegration) => Number((integration.resources?.length ?? 0) === 0) + Number(connectionCount(integration) === 0) + Number(!hasSupport(integration));
  const families = [...new Set(integrations.map((integration) => integration.family_key))].sort();
  const filteredIntegrations = integrations.filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)));
  const groupedIntegrations = families
    .map((family) => ({ family, integrations: filteredIntegrations.filter((integration) => integration.family_key === family).sort((left, right) => left.version_key.localeCompare(right.version_key, undefined, { numeric: true })) }))
    .filter((group) => group.integrations.length > 0);

  return <>
    <PageHeading eyebrow="Catalog" title="APIs" description="Choose an API to configure what developers and agents can use." action={<span className="heading-actions"><Button outline onClick={onCreate}><Plus data-slot="icon" />Add API</Button><Button onClick={onBuild}><Sparkles data-slot="icon" />Import APIs</Button></span>} />
    <div className="toolbar integration-toolbar">
      <div className="search-field"><Search /><input aria-label="Search APIs" placeholder="Search APIs…" value={query} onChange={(event) => onQueryChange(event.target.value)} /></div>
      <span className="toolbar-count">{filteredIntegrations.length} API{filteredIntegrations.length === 1 ? "" : "s"}</span>
    </div>
    <div className="integration-family-groups">
      {groupedIntegrations.map((group) => <section className="resource-table integration-directory" key={group.family}>
        <div className="integration-family-heading"><span><strong>{group.family}</strong><small>{group.integrations.length} version{group.integrations.length === 1 ? "" : "s"}</small></span></div>
        <div className="table-head integration-directory-columns"><span>API</span><span>Lifecycle</span><span>Setup</span><span>Resources</span><span /></div>
        {group.integrations.map((integration) => { const issues = setupIssueCount(integration); return <ConsoleLink key={integration.id} path={integrationPath(integration.id)} onNavigate={onNavigate} className="table-row integration-directory-columns integration-directory-row">
          <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.version_key}</small></span></span>
          <span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge> <Badge color={integration.visibility === "public" ? "blue" : "zinc"}>{integration.visibility}</Badge></span>
          <Badge color={issues === 0 ? "green" : "amber"}>{issues === 0 ? "Ready" : `${issues} step${issues === 1 ? "" : "s"} left`}</Badge>
          <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">attached sets</small></span>
          <ChevronRight />
        </ConsoleLink>; })}
      </section>)}
      {filteredIntegrations.length === 0 && <div className="resource-table"><div className="empty-row">{integrations.length === 0 ? "No APIs yet. Add one manually or import your existing API sources." : "No APIs match this search."}</div></div>}
    </div>
  </>;
}

function IntegrationSwitcher({ integrations, integration, activeTab, onNavigate }: { integrations: APIIntegration[]; integration: APIIntegration; activeTab: IntegrationTab; onNavigate: (path: string) => void }) {
  const optionLabel = (value: APIIntegration) => `${value.display_name} · ${value.version_key} · ${value.family_key}`;
  const [value, setValue] = useState(optionLabel(integration));

  function selectIntegration(nextValue: string) {
    setValue(nextValue);
    const selected = integrations.find((candidate) => candidate.id === nextValue || optionLabel(candidate) === nextValue);
    if (selected && selected.id !== integration.id) onNavigate(integrationPath(selected.id, activeTab));
  }

  if (integrations.length <= 1) return null;
  return <div className="integration-workspace-switcher"><label htmlFor="integration-switcher">Switch API</label><div className="integration-switcher-input"><Search /><input id="integration-switcher" list="integration-switcher-options" value={value} onChange={(event) => selectIntegration(event.target.value)} onBlur={() => { if (!integrations.some((candidate) => optionLabel(candidate) === value)) setValue(optionLabel(integration)); }} /><datalist id="integration-switcher-options">{[...integrations].sort((left, right) => left.family_key.localeCompare(right.family_key) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true })).map((candidate) => <option key={candidate.id} value={optionLabel(candidate)} />)}</datalist></div></div>;
}

function IntegrationWorkspaceView({ integration, integrations, activeTab, loading, revisions, publishStatus, identity, resourceSets, connections, supportRoutes, tools, mcpConnections, busy, onEdit, onPublish, onAttach, onCreateResource, onAddTool, onEditResource, onDuplicateResource, onDetachResource, onManageAccess, onManageSupport, onInspectRevision, onNavigate }: { integration: APIIntegration | null; integrations: APIIntegration[]; activeTab: IntegrationTab; loading: boolean; revisions: APIIntegrationRevision[]; publishStatus: APIIntegrationPublishStatus | null; identity: APIIdentity | null; resourceSets: APIResourceSet[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; tools: APITool[]; mcpConnections: APIMCPConnection[]; busy: boolean; onEdit: (integration: APIIntegration) => void; onPublish: (integration: APIIntegration) => void; onAttach: (integration: APIIntegration, kind?: APIResourceSet["kind"]) => void; onCreateResource: () => void; onAddTool: () => void; onEditResource: (resource: APIResourceSet) => void; onDuplicateResource: (resource: APIResourceSet) => void; onDetachResource: (integrationID: string, resourceSetID: string) => void; onManageAccess: (integration: APIIntegration) => void; onManageSupport: (integration: APIIntegration) => void; onInspectRevision: (revision: APIIntegrationRevision) => void; onNavigate: (path: string) => void }) {
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">API</p><h1>Loading API…</h1><p>Retrieving its configuration and published history.</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">API</p><h1>API unavailable</h1><p>This API is not available in the current deployment.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;

  const integrationConnections = connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id));
  const supportRoute = supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default);
  const attachedResources = integration.resources ?? [];
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);
  const setupSteps = [
    { label: "Add resources", detail: "Attach documentation or an API contract.", ready: attachedResources.length > 0, tab: "resources" as IntegrationTab },
    { label: "Connect access", detail: "Choose which service connections this API may use.", ready: integrationConnections.length > 0, tab: "access" as IntegrationTab },
  ].filter((step) => !step.ready);
  const setupValidationCodes = new Set(["resources_missing", "access_missing", "support_inherited"]);
  const actionableValidations = publishStatus?.validations.filter((validation) => !setupValidationCodes.has(validation.code)) ?? [];
  const hasChanges = Boolean(publishStatus?.has_changes);
  const canPublish = Boolean(publishStatus?.ready && hasChanges);
  const resourceLabel = (kind: APIResourceSet["kind"]) => kind === "api" ? "API contract" : "documentation";

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />All APIs</ConsoleLink></div>
    <IntegrationSwitcher key={integration.id} integrations={integrations} integration={integration} activeTab={activeTab} onNavigate={onNavigate} />
    <PageHeading eyebrow={`${integration.family_key} · ${integration.version_key}`} title={integration.display_name} description={integration.description || "No description has been added for this API."} action={<span className="heading-actions"><Button outline onClick={() => onEdit(integration)}>Edit</Button>{!publishStatus ? <span className="published-state checking"><RefreshCw />Checking…</span> : canPublish ? <Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />Publish</Button> : hasChanges && !publishStatus.ready ? <Badge color="amber">Setup required</Badge> : <span className="published-state"><CheckCircle2 />Published</span>}</span>} />
    <nav className="integration-tabs" aria-label={`${integration.display_name} sections`}>{INTEGRATION_TABS.map((tab) => <ConsoleLink key={tab.id} path={integrationPath(integration.id, tab.id)} onNavigate={onNavigate} className={`integration-tab ${activeTab === tab.id ? "active" : ""}`} ariaCurrent={activeTab === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</nav>

    {activeTab === "overview" && <div className="integration-tab-content">
      <div className="api-status-bar"><span><span className={`status-dot${publishStatus ? "" : " checking"}`} /><strong>{!publishStatus ? "Checking status" : publishStatus.ready ? hasChanges ? "Ready to publish" : "Published" : "Needs setup"}</strong><small>{!publishStatus ? "Loading the latest publication state…" : hasChanges ? `${publishStatus.changes.length} unpublished change${publishStatus.changes.length === 1 ? "" : "s"}` : `${attachedResources.length} resource set${attachedResources.length === 1 ? "" : "s"} attached`}</small></span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge></div>
      {(setupSteps.length > 0 || !supportRoute || actionableValidations.length > 0) && <section className="panel compact-setup-panel"><div className="panel-heading"><div><h2>Finish setup</h2><p>Only unresolved actions appear here.</p></div></div>{setupSteps.map((step) => <ConsoleLink key={step.label} path={integrationPath(integration.id, step.tab)} onNavigate={onNavigate} className="integration-health-check"><span className="health-icon"><AlertCircle /></span><span><strong>{step.label}</strong><small>{step.detail}</small></span><ChevronRight /></ConsoleLink>)}{!supportRoute && <button type="button" className="integration-health-check" onClick={() => onManageSupport(integration)}><span className="health-icon"><AlertCircle /></span><span><strong>Configure bug reports and feedback</strong><small>Choose secure delivery or encrypted local holding.</small></span><ChevronRight /></button>}{actionableValidations.map((validation) => <ConsoleLink key={validation.code} path={integrationPath(integration.id, validation.tab === "resources" || validation.tab === "access" ? validation.tab : "overview")} onNavigate={onNavigate} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level === "error" ? "Resolve before publishing" : "Review before publishing"}</strong><small>{validation.message}</small></span><ChevronRight /></ConsoleLink>)}</section>}
      <section className="panel"><div className="panel-heading"><div><h2>Bug reports & feedback</h2><p>Consent-gated agent reports use this API’s delivery policy.</p></div><Button outline onClick={() => onManageSupport(integration)}>{supportRoute ? "Change" : "Configure"}</Button></div>{supportRoute ? <div className="support-route-summary"><span className="settings-icon"><Bug /></span><span><strong>{supportRoute.name}</strong><small>{supportRoute.is_default ? "Uses the deployment default" : "Configured for this API"} · {supportRoute.retention_days}-day encrypted retention</small></span><Badge color={supportRoute.state === "active" ? "green" : "zinc"}>{supportRoute.bug_reports_enabled || supportRoute.feedback_enabled ? "Available" : "Off"}</Badge></div> : <div className="empty-row">Not configured. Reports remain unavailable until you choose a policy.</div>}</section>
      <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "resources")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>Resources</strong><small>Documentation, API contracts, and tools.</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={sectionPath("settings")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><ShieldCheck /></span><span><strong>Customer access</strong><small>{identity?.state === "active" ? "Delegated identity active" : identity ? "Identity disabled" : "Identity setup optional"}</small></span><ChevronRight /></ConsoleLink></div>
      <details className="panel advanced-details"><summary>Advanced details</summary><dl className="entity-detail-grid"><div><dt>API ID</dt><dd>{integration.id}</dd></div><div><dt>Family</dt><dd>{integration.family_key}</dd></div><div><dt>Version</dt><dd>{integration.version_key}</dd></div><div><dt>Draft revision</dt><dd>{integration.revision}</dd></div><div><dt>Replacement</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>Sunset</dt><dd>{integration.sunset_at ? new Date(integration.sunset_at).toLocaleDateString() : "—"}</dd></div></dl></details>
    </div>}

    {activeTab === "resources" && <div className="integration-tab-content">
      <section className="panel"><div className="panel-heading"><div><h2>Resources</h2><p>Everything agents need to understand or act on this API.</p></div><span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create resource set</Button><Button disabled={resourceSets.every((set) => attachedResources.some((resource) => resource.resource_set_id === set.id))} onClick={() => onAttach(integration)}>Attach existing</Button></span></div>
        {attachedResources.map((resource) => { const source = resourceSets.find((set) => set.id === resource.resource_set_id); return <div className="integration-resource-row" key={resource.resource_set_id}><span className="settings-icon">{resource.kind === "documentation" ? <BookOpen /> : <TerminalSquare />}</span><span><EntityLink entity="resource-set" uid={resource.resource_set_id} onNavigate={onNavigate} className="entity-link"><strong>{resource.name}</strong></EntityLink><small>{resourceLabel(resource.kind)} · {resource.follow_latest ? "follows latest" : "pinned"}</small></span><Badge color={resource.kind === "documentation" ? "blue" : "violet"}>{resourceLabel(resource.kind)}</Badge><span className="table-actions">{source && <Button outline onClick={() => onEditResource(source)}>New revision</Button>}{source && <Button outline onClick={() => onDuplicateResource(source)}>Duplicate</Button>}<button type="button" className="more" disabled={busy} title={`Detach ${resource.name}`} aria-label={`Detach ${resource.name}`} onClick={() => onDetachResource(integration.id, resource.resource_set_id)}><XCircle /></button></span></div>; })}
        {attachedResources.length === 0 && <div className="empty-row">No resources are attached yet.</div>}
      </section>
      <section className="panel"><div className="panel-heading"><div><h2>Agent tools</h2><p>Published actions and their fixed, policy-checked backends.</p></div><Button outline onClick={onAddTool}><Plus data-slot="icon" />Add tool</Button></div>{tools.map((tool) => <div className="provider-row" key={tool.id}><span className="settings-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{tool.backend_kind === "mcp" ? "MCP tool backend" : "HTTPS tool backend"}</small></span><Badge color={tool.state === "published" ? "green" : "amber"}>{tool.state}</Badge></div>)}{tools.length === 0 && <div className="empty-row">No tools are available.</div>}</section>
      {mcpConnections.length > 0 && <details className="panel advanced-details"><summary>Tool backend connections</summary>{mcpConnections.map((connection) => <div className="lease-row" key={connection.id}><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.protocol_version} · {connection.auth_mode}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge></div>)}<ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className="entity-back-link">Manage tool backends</ConsoleLink></details>}
    </div>}

    {activeTab === "access" && <div className="integration-tab-content"><section className="panel"><div className="panel-heading"><div><h2>Service connections</h2><p>Vendor accounts this API may use to authorize access or issue credentials.</p></div><span className="heading-actions"><ConsoleLink path={sectionPath("projects")} onNavigate={onNavigate} className="entity-back-link">Manage shared connections</ConsoleLink><Button onClick={() => onManageAccess(integration)}>Choose connections</Button></span></div>{integrationConnections.map((connection) => <div className="provider-row integration-connection-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.definition?.name ?? "Service connection"}{connection.region ? ` · ${connection.region}` : ""}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge></div>)}{integrationConnections.length === 0 && <div className="empty-row">No service connection is attached to this API.</div>}</section></div>}

    {activeTab === "history" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>Published history is immutable.</strong> Each entry preserves the exact resources, access, and reporting policy used by agents.</span></div><section className="panel"><div className="panel-heading"><div><h2>Published history</h2><p>Open an entry only when you need its complete technical snapshot.</p></div></div>{sortedRevisions.map((revision) => <button type="button" className="integration-revision-row" key={revision.id} onClick={() => onInspectRevision(revision)}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? new Date(revision.published_at ?? revision.created_at).toLocaleString() : "Date unavailable"}</small></span><ChevronRight /></button>)}{sortedRevisions.length === 0 && <div className="empty-row">Nothing has been published yet.</div>}</section></div>}
  </>;
}

function IntegrationsView({ integrations, resourceSets, supportRoutes, connections, tools, mcpConnections, identity, selectedIntegrationID, activeTab = "overview", onBuild, onAddTool, onChanged, onMessage, onNavigate }: { integrations: APIIntegration[]; resourceSets: APIResourceSet[]; supportRoutes: APISupportRoute[]; connections: APIAccessConnection[]; tools: APITool[]; mcpConnections: APIMCPConnection[]; identity: APIIdentity | null; selectedIntegrationID?: string; activeTab?: IntegrationTab; onBuild: () => void; onAddTool: () => void; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<APIIntegration | null>(null);
  const [selectedRevisions, setSelectedRevisions] = useState<APIIntegrationRevision[]>([]);
  const [selectedPublishStatus, setSelectedPublishStatus] = useState<APIIntegrationPublishStatus | null>(null);
  const [loadedIntegrationID, setLoadedIntegrationID] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editingIntegration, setEditingIntegration] = useState<APIIntegration | null>(null);
  const [familyKey, setFamilyKey] = useState("");
  const [versionKey, setVersionKey] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [integrationVisibility, setIntegrationVisibility] = useState<APIIntegration["visibility"]>("private");
  const [integrationPublicAcknowledged, setIntegrationPublicAcknowledged] = useState(false);
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
  const [accessIntegration, setAccessIntegration] = useState<APIIntegration | null>(null);
  const [accessSelection, setAccessSelection] = useState<string[]>([]);
  const [supportIntegration, setSupportIntegration] = useState<APIIntegration | null>(null);
  const [supportSelection, setSupportSelection] = useState("");
  const [publishCandidate, setPublishCandidate] = useState<APIIntegration | null>(null);
  const [inspectedRevision, setInspectedRevision] = useState<APIIntegrationRevision | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!selectedIntegrationID) return;
    let cancelled = false;
    api.integration(selectedIntegrationID).then((value) => {
      if (cancelled) return;
      setSelectedDetail(value.integration);
      setSelectedRevisions(value.revisions);
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(selectedIntegrationID);
    }).catch(() => {
      if (cancelled) return;
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
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
      setSelectedPublishStatus(value.publish_status);
      setLoadedIntegrationID(integrationID);
    } catch {
      setSelectedDetail(null);
      setSelectedRevisions([]);
      setSelectedPublishStatus(null);
      setLoadedIntegrationID(integrationID);
    }
  }

  function openIntegration(value?: APIIntegration) {
    setEditingIntegration(value ?? null);
    setFamilyKey(value?.family_key ?? ""); setVersionKey(value?.version_key ?? ""); setDisplayName(value?.display_name ?? ""); setDescription(value?.description ?? ""); setIntegrationVisibility(value?.visibility ?? "private"); setIntegrationPublicAcknowledged(false); setLifecycle(value?.lifecycle ?? "draft"); setReplacementID(value?.replacement_integration_id ?? ""); setSunsetAt(value?.sunset_at?.slice(0, 10) ?? "");
    setIntegrationOpen(true);
  }

  async function saveIntegration() {
    setBusy(true);
    try {
      const base = { family_key: familyKey, version_key: versionKey, display_name: displayName, description, visibility: integrationVisibility, acknowledge_public: integrationPublicAcknowledged, lifecycle };
      const saved = editingIntegration
        ? await api.updateIntegration(editingIntegration.id, { ...base, replacement_integration_id: replacementID || undefined, sunset_at: sunsetAt ? new Date(`${sunsetAt}T00:00:00Z`).toISOString() : undefined, revision: editingIntegration.revision })
        : await api.createIntegration(base);
      setSelectedDetail(saved);
      await onChanged();
      if (editingIntegration) await refreshSelectedIntegration(saved.id);
      setIntegrationOpen(false);
      onMessage(editingIntegration ? "API updated." : `API created with ${saved.lifecycle} lifecycle.`);
      if (!editingIntegration) onNavigate(integrationPath(saved.id));
    } catch (error) { onMessage(error instanceof APIError ? error.message : "API could not be saved."); } finally { setBusy(false); }
  }

  async function publishIntegration() {
    if (!publishCandidate) return;
    setBusy(true);
    try { await api.publishIntegration(publishCandidate.id); await onChanged(); await refreshSelectedIntegration(publishCandidate.id); setPublishCandidate(null); onMessage("API published."); } catch (error) { onMessage(error instanceof APIError ? error.message : "API could not be published."); } finally { setBusy(false); }
  }

  function openAccessDialog(value: APIIntegration) {
    setAccessIntegration(value);
    setAccessSelection(value.access_connection_ids ?? connections.filter((connection) => connection.integration_ids?.includes(value.id)).map((connection) => connection.id));
  }

  async function saveAccessConnections() {
    if (!accessIntegration) return;
    setBusy(true);
    try {
      await api.setIntegrationAccessConnections(accessIntegration.id, accessSelection);
      await onChanged();
      await refreshSelectedIntegration(accessIntegration.id);
      setAccessIntegration(null);
      onMessage("API access connections updated.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Access connections could not be updated."); } finally { setBusy(false); }
  }

  function openSupportDialog(value: APIIntegration) {
    setSupportIntegration(value);
    setSupportSelection(value.support_route_id ?? supportRoutes.find((route) => !route.is_default && route.integration_ids?.includes(value.id))?.id ?? "");
  }

  async function saveSupportRoute() {
    if (!supportIntegration) return;
    setBusy(true);
    try {
      await api.setIntegrationSupportRoute(supportIntegration.id, supportSelection);
      await onChanged();
      await refreshSelectedIntegration(supportIntegration.id);
      setSupportIntegration(null);
      onMessage(supportSelection ? "API reporting policy updated." : "API now inherits the default reporting policy.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Reporting policy could not be updated."); } finally { setBusy(false); }
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
    try { await api.attachResourceSet(attachIntegration.id, resource.id, pinAttachedSet ? resource.latest_revision?.id ?? "" : ""); await onChanged(); await refreshSelectedIntegration(attachIntegration.id); setAttachIntegration(null); onMessage(pinAttachedSet ? "Resource revision pinned to API." : "Resource set attached and following latest."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be attached."); } finally { setBusy(false); }
  }

  async function detachResource(integrationID: string, setID: string) {
    setBusy(true);
    try { await api.detachResourceSet(integrationID, setID); await onChanged(); await refreshSelectedIntegration(integrationID); onMessage("Resource set detached from API."); } catch (error) { onMessage(error instanceof APIError ? error.message : "Resource set could not be detached."); } finally { setBusy(false); }
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
    {selectedIntegrationID ? <IntegrationWorkspaceView integration={selectedIntegration} integrations={integrations} activeTab={activeTab} loading={selectedLoading} revisions={selectedRevisions} publishStatus={selectedPublishStatus} identity={identity} resourceSets={resourceSets} connections={connections} supportRoutes={supportRoutes} tools={tools} mcpConnections={mcpConnections} busy={busy} onEdit={openIntegration} onPublish={setPublishCandidate} onAttach={openAttachDialog} onCreateResource={() => openResource()} onAddTool={onAddTool} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(`${set.name} copy`); }} onDetachResource={detachResource} onManageAccess={openAccessDialog} onManageSupport={openSupportDialog} onInspectRevision={setInspectedRevision} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} connections={connections} supportRoutes={supportRoutes} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onBuild={onBuild} onNavigate={onNavigate} />}

    <Dialog open={integrationOpen} onClose={setIntegrationOpen} title={editingIntegration ? "Edit API" : "Add API"} description="Each API record represents one family and one version. Private is the default." actions={<><Button outline onClick={() => setIntegrationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !familyKey.trim() || !versionKey.trim() || !displayName.trim() || (integrationVisibility === "public" && editingIntegration?.visibility !== "public" && !integrationPublicAcknowledged)} onClick={saveIntegration}>{busy ? "Saving…" : "Save API"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>API family key</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} placeholder="voice-api" /></label><label className="auth-field"><span>API version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} placeholder="v2" /></label></div><label className="auth-field"><span>Display name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Voice API v2" /></label><label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Visibility</span><select value={integrationVisibility} onChange={(event) => { setIntegrationVisibility(event.target.value as APIIntegration["visibility"]); setIntegrationPublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public exposes only the published read-only Integration manifest.</small></label><label className="auth-field"><span>Lifecycle</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="retired">Retired</option></select></label></div>{integrationVisibility === "public" && editingIntegration?.visibility !== "public" && <label className="compact-check"><input type="checkbox" checked={integrationPublicAcknowledged} onChange={(event) => setIntegrationPublicAcknowledged(event.target.checked)} /><span>I understand this published API metadata will be anonymously discoverable while Public MCP is enabled.</span></label>}<label className="auth-field"><span>Replacement</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">None</option>{integrations.filter((value) => value.id !== editingIntegration?.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label>{(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>Sunset date</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}</div></Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? `Create revision for ${editingSet.name}` : "Create reusable resource set"} description="Sets are reusable by explicit attachment. Each save creates immutable content." actions={<><Button outline onClick={() => setResourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !setName.trim()} onClick={saveResourceSet}>{busy ? "Saving…" : editingSet ? "Create revision" : "Create set"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Kind</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">Documentation</option><option value="api">API contract</option></select></label><label className="auth-field"><span>Name</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>Description</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label><label className="auth-field"><span>Manifest (JSON array)</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label></div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title="Duplicate resource set" description="Creates an independent copy so later edits do not affect APIs using the original." actions={<><Button outline onClick={() => setDuplicateSet(null)}>Cancel</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>Duplicate</Button></>}><label className="auth-field"><span>New set name</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={`Attach resources to ${attachIntegration?.display_name ?? "API"}`} description="Follow latest for deliberate sharing, or pin the current immutable revision." actions={<><Button outline onClick={() => setAttachIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>Attach</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Resource set</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">Select a set</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind === "api" ? "API contract" : "documentation"} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>Pin the current revision instead of following latest</span></label></div></Dialog>
    <Dialog open={Boolean(accessIntegration)} onClose={(open) => { if (!open) setAccessIntegration(null); }} title={`Access for ${accessIntegration?.display_name ?? "API"}`} description="Choose the service connections this API may use. Credentials remain encrypted and are never copied into the API." actions={<><Button outline onClick={() => setAccessIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveAccessConnections}>{busy ? "Saving…" : "Save connections"}</Button></>}><div className="auth-form compact-form"><fieldset className="catalog-settings-section"><legend>Allowed connections</legend>{connections.map((connection) => <label className="compact-check" key={connection.id}><input type="checkbox" aria-label={`Allow ${connection.name}`} checked={accessSelection.includes(connection.id)} onChange={() => setAccessSelection((values) => values.includes(connection.id) ? values.filter((id) => id !== connection.id) : [...values, connection.id])} /><span><strong>{connection.name}</strong><small>{connection.definition?.name ?? "Service connection"} · {connection.state}</small></span></label>)}{connections.length === 0 && <p className="dialog-empty-copy">No service connections exist yet. Create one in Settings, then return here.</p>}</fieldset></div></Dialog>
    <Dialog open={Boolean(supportIntegration)} onClose={(open) => { if (!open) setSupportIntegration(null); }} title={`Bug reports & feedback for ${supportIntegration?.display_name ?? "API"}`} description="Choose an API-specific policy, or inherit the deployment default." actions={<><Button outline onClick={() => setSupportIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveSupportRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Reporting policy</span><select value={supportSelection} onChange={(event) => setSupportSelection(event.target.value)}><option value="">Inherit deployment default</option>{supportRoutes.filter((route) => !route.is_default && route.state === "active").map((route) => <option key={route.id} value={route.id}>{route.name} · {route.retention_days} days</option>)}</select><small>{supportRoutes.find((route) => route.is_default) ? `Current default: ${supportRoutes.find((route) => route.is_default)?.name}` : "No default policy exists. Configure one in Settings."}</small></label></div></Dialog>
    <Dialog open={Boolean(publishCandidate)} onClose={(open) => { if (!open) setPublishCandidate(null); }} title={`Publish ${publishCandidate?.display_name ?? "API"}`} description="Review what changed before creating a new immutable version." actions={<><Button outline onClick={() => setPublishCandidate(null)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedPublishStatus?.ready || !selectedPublishStatus.has_changes} onClick={publishIntegration}>{busy ? "Publishing…" : "Publish"}</Button></>}><div className="publish-review">{selectedPublishStatus?.validations.map((validation) => <div key={validation.code} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level}</strong><small>{validation.message}</small></span></div>)}<div className="publish-diff-list">{selectedPublishStatus?.changes.map((change) => <div className="publish-diff" key={change.field}><strong>{change.field}</strong><span><small>Published</small><code>{change.before === undefined ? "—" : JSON.stringify(change.before)}</code></span><ChevronRight /><span><small>Draft</small><code>{change.after === undefined ? "—" : JSON.stringify(change.after)}</code></span></div>)}</div><details className="advanced-details"><summary>Technical details</summary><code>{selectedPublishStatus?.current_manifest_hash ?? "—"}</code></details></div></Dialog>
    <Dialog open={Boolean(inspectedRevision)} onClose={(open) => { if (!open) setInspectedRevision(null); }} title={`Published version r${inspectedRevision?.revision ?? ""}`} description="This immutable technical snapshot is kept for audit and deterministic agent delivery." actions={<Button outline onClick={() => setInspectedRevision(null)}>Close</Button>}><div className="revision-inspector"><dl className="entity-detail-grid"><div><dt>Version ID</dt><dd>{inspectedRevision?.id}</dd></div><div><dt>State</dt><dd>{inspectedRevision?.state}</dd></div><div><dt>Published</dt><dd>{inspectedRevision ? new Date(inspectedRevision.published_at ?? inspectedRevision.created_at).toLocaleString() : "—"}</dd></div><div><dt>Published by</dt><dd>{inspectedRevision?.published_by || "—"}</dd></div><div><dt>Manifest hash</dt><dd><code>{inspectedRevision?.manifest_hash}</code></dd></div></dl><pre className="usage-contract"><code>{JSON.stringify(inspectedRevision?.snapshot ?? {}, null, 2)}</code></pre></div></Dialog>
  </>;
}

function ConnectorReleasesView({ versions, integrations, onConfigure, onNavigate }: { versions: APIProductVersion[]; integrations: APIIntegration[]; onConfigure: () => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Advanced publishing" title="Compatibility snapshots" description="Publish immutable compatibility snapshots that select a tested combination of API versions. This is a deployment release, not an API version." action={<Button onClick={onConfigure}><Settings data-slot="icon" />Release policy</Button>} /><div className="notice"><GitBranch /><span><strong>API versions stay independent.</strong> A compatibility snapshot can combine Voice API v2 with Face API v3 without changing either API identity.</span></div><div className="metrics-grid"><Metric label="Compatibility snapshots" value={String(versions.length)} detail={`${versions.filter((version) => version.release_stage === "active").length} active`} /><Metric label="APIs" value={String(integrations.length)} detail="Selected by immutable revision" /><Metric label="Latest" value={versions.find((version) => version.is_latest)?.version ?? "—"} detail="Default latest channel" /><Metric label="LTS" value={versions.find((version) => version.is_lts)?.version ?? "—"} detail="Stable channel" /></div><section className="panel"><div className="panel-heading"><div><h2>Published snapshots</h2><p>Customer, environment, and installation pins continue to override the default channel.</p></div></div>{versions.map((version) => <div className="provider-row" key={version.id}><span className="settings-icon"><GitBranch /></span><span><EntityLink entity="release" uid={version.id} onNavigate={onNavigate} className="entity-link"><strong>{version.version}</strong></EntityLink><small>{version.profile_name} · {version.manifest_hash}</small></span><span>{version.is_latest && <Badge color="blue">Latest</Badge>} {version.is_lts && <Badge color="violet">LTS</Badge>}</span><Badge color={version.deprecated_at ? "amber" : version.drift_status === "drifted" ? "red" : "green"}>{version.deprecated_at ? "Deprecated" : version.drift_status}</Badge></div>)}{versions.length === 0 && <div className="empty-row">No compatibility snapshots have been published.</div>}</section></>;
}

function AccessView({ definitions, connections, instances, credentials, integrations, environments, apiResourceSets, onChanged, onMessage, onNavigate }: { definitions: APIAccessDefinition[]; connections: APIAccessConnection[]; instances: APIAccessInstance[]; credentials: APIAccessCredential[]; integrations: APIIntegration[]; environments: APIEnvironment[]; apiResourceSets: APIResourceSet[]; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const activeCredentials = credentials.filter((credential) => credential.state === "active" && (!credential.expires_at || new Date(credential.expires_at) > new Date())).length;
  const [definitionOpen, setDefinitionOpen] = useState(false);
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

  async function saveDefinition() {
    setBusy(true);
    try { const parsed = JSON.parse(operations) as Record<string, unknown>; await api.createAccessDefinition({ service_key: serviceKey, name: serviceName, instance_cardinality: cardinality, instance_label_singular: singular, instance_label_plural: plural, credential_scope: cardinality === "one" ? "connection" : credentialScope, management_auth_type: managementAuth, api_resource_set_id: apiResourceSetID || undefined, operations: parsed }); await onChanged(); setDefinitionOpen(false); onMessage("Provider access definition created."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access definition could not be created."); } finally { setBusy(false); }
  }

  async function saveConnection() {
    setBusy(true);
    try { const parsed = JSON.parse(connectionConfig) as Record<string, unknown>; await api.createAccessConnection({ access_definition_id: definitionID, environment_id: environmentID || undefined, name: connectionName, region: region || undefined, base_url: baseURL, management_secret: managementSecret || undefined, config: parsed, integration_ids: selectedIntegrations }); await onChanged(); setConnectionOpen(false); setManagementSecret(""); onMessage("Access connection created and attached."); } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Access connection could not be created."); } finally { setBusy(false); }
  }

  function toggleIntegration(id: string) { setSelectedIntegrations((values) => values.includes(id) ? values.filter((value) => value !== id) : [...values, id]); }

  return <>
    <PageHeading eyebrow="Shared configuration" title="Service connections" description="Connect vendor services once, then allow each API to use only the connections it needs." action={<Button onClick={() => { setDefinitionID(definitions[0]?.id ?? ""); setEnvironmentID(environments[0]?.id ?? ""); setConnectionOpen(true); }}><KeyRound data-slot="icon" />Connect service</Button>} />
    <section className="panel"><div className="panel-heading"><div><h2>Connections</h2><p>Credentials stay encrypted and every API assignment is explicit.</p></div></div>{connections.map((connection) => { const definition = connection.definition ?? definitions.find((item) => item.id === connection.access_definition_id); const connectionInstances = instances.filter((item) => item.access_connection_id === connection.id); const connectionCredentials = credentials.filter((item) => item.access_connection_id === connection.id); const labels = (connection.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)).filter(Boolean).map((item) => `${item!.display_name} ${item!.version_key}`).join(", "); return <div className="provider-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{definition?.name ?? "Service"} · {labels || "No API attached"}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge><span><strong>{definition?.instance_cardinality === "many" ? connectionInstances.length : "1"} {definition?.instance_cardinality === "many" ? definition.instance_label_plural : definition?.instance_label_singular ?? "instance"}</strong><small>{connectionCredentials.length} credential record{connectionCredentials.length === 1 ? "" : "s"}</small></span></div>; })}{connections.length === 0 && <div className="empty-row">No service connections yet. Connect a vendor service to make it available to APIs.</div>}</section>
    <details className="panel advanced-details"><summary>Advanced service setup</summary><div className="advanced-details-body"><div className="panel-heading"><div><h2>Service types</h2><p>Reusable operation contracts for unusual provider models.</p></div><Button outline onClick={() => setDefinitionOpen(true)}><Plus data-slot="icon" />New service type</Button></div>{definitions.map((definition) => <div className="lease-row" key={definition.id}><span><EntityLink entity="access-definition" uid={definition.id} onNavigate={onNavigate} className="entity-link"><strong>{definition.name}</strong></EntityLink><small>{definition.instance_cardinality === "many" ? `Multiple ${definition.instance_label_plural}` : `Single ${definition.instance_label_singular}`}</small></span><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></div>)}{definitions.length === 0 && <div className="empty-row">No service types are configured.</div>}<div className="panel-heading advanced-subheading"><div><h2>Credential records</h2><p>Fingerprints and lifecycle only. Plaintext credentials are never listed.</p></div><Badge color="violet">{activeCredentials} active</Badge></div>{credentials.slice(0, 12).map((credential) => <div className="lease-row" key={credential.id}><span><strong>{credential.scopes.join(", ") || "Default scope"}</strong><small>{credential.secret_fingerprint.slice(0, 18)}… · {credential.storage_mode}</small></span><Badge color={credential.state === "active" ? "green" : "zinc"}>{credential.state}</Badge></div>)}{credentials.length === 0 && <div className="empty-row">No credential records yet.</div>}</div></details>
  <Dialog open={definitionOpen} onClose={setDefinitionOpen} title="Create service type" description="The provider contract declares cardinality and credential scope; end users do not choose mono versus multi." actions={<><Button outline onClick={() => setDefinitionOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !serviceKey.trim() || !serviceName.trim() || !singular.trim() || !plural.trim()} onClick={saveDefinition}>{busy ? "Saving…" : "Create service type"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service key</span><input value={serviceKey} onChange={(event) => setServiceKey(event.target.value)} placeholder="auth0" /></label><label className="auth-field"><span>Name</span><input value={serviceName} onChange={(event) => setServiceName(event.target.value)} placeholder="Auth0 Management API" /></label></div><div className="two-fields"><label className="auth-field"><span>Provider instances</span><select value={cardinality} onChange={(event) => { const value = event.target.value as typeof cardinality; setCardinality(value); if (value === "one") setCredentialScope("connection"); }}><option value="one">One fixed instance</option><option value="many">Multiple provider resources</option></select></label><label className="auth-field"><span>Credential scope</span><select disabled={cardinality === "one"} value={credentialScope} onChange={(event) => setCredentialScope(event.target.value as typeof credentialScope)}><option value="connection">Connection</option><option value="instance">Provider resource</option></select></label></div><div className="two-fields"><label className="auth-field"><span>Singular label</span><input value={singular} onChange={(event) => setSingular(event.target.value)} placeholder="tenant" /></label><label className="auth-field"><span>Plural label</span><input value={plural} onChange={(event) => setPlural(event.target.value)} placeholder="tenants" /></label></div><div className="two-fields"><label className="auth-field"><span>Management authentication</span><select value={managementAuth} onChange={(event) => setManagementAuth(event.target.value as typeof managementAuth)}><option value="bearer">Bearer token</option><option value="api_key">API key</option><option value="oauth2_client_credentials">OAuth2 client credentials</option><option value="none">None</option></select></label><label className="auth-field"><span>API contract set</span><select value={apiResourceSetID} onChange={(event) => setAPIResourceSetID(event.target.value)}><option value="">None</option>{apiResourceSets.map((set) => <option key={set.id} value={set.id}>{set.name}</option>)}</select></label></div><label className="auth-field"><span>Operations (JSON)</span><textarea className="code-input" value={operations} onChange={(event) => setOperations(event.target.value)} spellCheck={false} /></label></div></Dialog>
  <Dialog open={connectionOpen} onClose={setConnectionOpen} title="Connect vendor service" description="Credentials are encrypted server-side. The fixed HTTPS destination is validated again for every operation." actions={<><Button outline onClick={() => setConnectionOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !definitionID || !connectionName.trim() || !baseURL.trim() || selectedIntegrations.length === 0} onClick={saveConnection}>{busy ? "Connecting…" : "Connect service"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Service type</span><select value={definitionID} onChange={(event) => setDefinitionID(event.target.value)}><option value="">Select definition</option>{definitions.map((definition) => <option key={definition.id} value={definition.id}>{definition.name}</option>)}</select></label><label className="auth-field"><span>Connection name</span><input value={connectionName} onChange={(event) => setConnectionName(event.target.value)} /></label></div><div className="two-fields"><label className="auth-field"><span>Environment</span><select value={environmentID} onChange={(event) => setEnvironmentID(event.target.value)}><option value="">All environments</option>{environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}</option>)}</select></label><label className="auth-field"><span>Region</span><input value={region} onChange={(event) => setRegion(event.target.value)} placeholder="us-east-1" /></label></div><label className="auth-field"><span>Fixed HTTPS base URL</span><input type="url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://management.example.com" /></label><label className="auth-field"><span>Management credential</span><input type="password" autoComplete="off" value={managementSecret} onChange={(event) => setManagementSecret(event.target.value)} /></label><fieldset className="catalog-settings-section"><legend>Allowed APIs</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={selectedIntegrations.includes(integration.id)} onChange={() => toggleIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset><label className="auth-field"><span>Connection configuration (JSON)</span><textarea className="code-input" value={connectionConfig} onChange={(event) => setConnectionConfig(event.target.value)} spellCheck={false} /></label></div></Dialog>
  </>;
}


function ActivityHubView({ runs, environments, submissions, events, analytics, supportRoutes, onStart, onComplete, onView, onRetry, onNavigate }: { runs: APIIntegrationRun[]; environments: APIEnvironment[]; submissions: APISupportSubmission[]; events: APIAuditEvent[]; analytics: APIAnalytics | null; supportRoutes: APISupportRoute[]; onStart: () => void; onComplete: (run: APIIntegrationRun, succeeded: boolean) => void; onView: (submission: APISupportSubmission) => void; onRetry: (submission: APISupportSubmission) => void; onNavigate: (path: string) => void }) {
  const [filter, setFilter] = useState<"all" | "runs" | "reports" | "audit">("all");
  const environmentName = (id: string) => environments.find((environment) => environment.id === id)?.name ?? id;
  const statusColor = (state: APISupportSubmission["state"]): "zinc" | "blue" | "green" | "red" | "amber" => state === "delivered" ? "green" : state === "failed" ? "red" : state === "held" ? "amber" : "blue";
  const canRetry = (submission: APISupportSubmission) => supportRoutes.some((route) => route.id === submission.support_route_id && route.state === "active" && (submission.kind === "bug" ? route.bug_reports_enabled : route.feedback_enabled));
  const show = (kind: typeof filter) => filter === "all" || filter === kind;

  return <>
    <PageHeading eyebrow="Operations" title="Activity" description="Runs, developer reports, and administrative changes in one place." action={<Button onClick={onStart}><Plus data-slot="icon" />Start run</Button>} />
    <div className="activity-toolbar" role="group" aria-label="Filter activity"><button type="button" className={filter === "all" ? "active" : ""} onClick={() => setFilter("all")}>All</button><button type="button" className={filter === "runs" ? "active" : ""} onClick={() => setFilter("runs")}>Runs <span>{runs.length}</span></button><button type="button" className={filter === "reports" ? "active" : ""} onClick={() => setFilter("reports")}>Bug reports & feedback <span>{submissions.length}</span></button><button type="button" className={filter === "audit" ? "active" : ""} onClick={() => setFilter("audit")}>Audit <span>{events.length}</span></button></div>
    {analytics && <div className="activity-summary"><strong>Last 30 days</strong><span>{analytics.integration_runs} runs</span><span>{analytics.tool_calls} tool calls</span><span>{analytics.first_pass_rate.toFixed(1)}% first-pass success</span></div>}

    {show("runs") && <section className="panel"><div className="panel-heading"><div><h2>API runs</h2><p>Requested outcomes with deterministic completion evidence.</p></div></div>{runs.map((run) => <div className="root-row run-row" key={run.id}><span className="settings-icon">{run.state === "running" ? <Clock3 /> : run.validated_success ? <CheckCircle2 /> : <XCircle />}</span><span><EntityLink entity="run" uid={run.id} onNavigate={onNavigate} className="entity-link"><strong>{run.requested_outcome}</strong></EntityLink><small>{environmentName(run.environment_id)} · {new Date(run.started_at).toLocaleString()}{run.failure_code ? ` · ${run.failure_code}` : ""}</small></span><Badge color={run.state === "running" ? "blue" : run.validated_success ? "green" : "red"}>{run.state}</Badge>{run.state === "running" ? <span className="run-actions"><Button outline onClick={() => onComplete(run, false)}>Failed</Button><Button color="indigo" onClick={() => onComplete(run, true)}>Validated</Button></span> : <span />}</div>)}{runs.length === 0 && <div className="empty-row">No API runs yet.</div>}</section>}

    {show("reports") && <section className="panel report-inbox"><div className="panel-heading"><div><h2>Bug reports & feedback</h2><p>Consent-gated submissions. Report content is decrypted only when an administrator opens it.</p></div><Badge color="violet">Encrypted at rest</Badge></div><div className="resource-table"><div className="table-head report-columns"><span>Submission</span><span>API</span><span>Delivery</span><span /></div>{submissions.map((submission) => <div className="table-row report-columns" key={submission.id}><span className="resource-name"><span className="resource-icon">{submission.kind === "bug" ? <Bug /> : <MessageSquareText />}</span><span><EntityLink entity="report" uid={submission.id} onNavigate={onNavigate} className="entity-link"><strong title={submission.summary}>{submission.summary}</strong></EntityLink><small>{submission.kind} · {new Date(submission.created_at).toLocaleString()}</small></span></span><span><strong className="cell-value">{submission.trusted_integration ? `${submission.trusted_integration.display_name} ${submission.trusted_integration.version_key}` : submission.related_tool || "Deployment"}</strong></span><span><Badge color={statusColor(submission.state)}>{submission.state}</Badge><small className="cell-note">{submission.external_id || (submission.attempts ? `${submission.attempts} attempt${submission.attempts === 1 ? "" : "s"}` : "Not delivered")}</small></span><span className="table-actions"><Button outline onClick={() => onView(submission)}>View</Button>{submission.external_url && <a className="report-ticket-link" href={submission.external_url} target="_blank" rel="noreferrer" aria-label="Open external ticket"><ExternalLink /></a>}{(submission.state === "failed" || submission.state === "held") && canRetry(submission) && <Button outline onClick={() => onRetry(submission)}><RefreshCw data-slot="icon" />Retry</Button>}</span></div>)}{submissions.length === 0 && <div className="empty-row">Approved bug reports and feedback will appear here.</div>}</div></section>}

    {show("audit") && <section className="panel"><div className="panel-heading"><div><h2>Audit</h2><p>Append-only security and configuration events. Secrets are never recorded.</p></div><Badge color="green">Append-only</Badge></div>{events.map((event) => <div className="root-row audit-row compact-audit-row" key={event.id}><span className="settings-icon"><ShieldCheck /></span><span><EntityLink entity="audit-event" uid={event.id} onNavigate={onNavigate} className="entity-link"><strong>{event.action}</strong></EntityLink><small>{event.target_type} · {new Date(event.created_at).toLocaleString()}</small></span><code>{event.actor_id}</code></div>)}{events.length === 0 && <div className="empty-row">Audit activity appears after the first configuration change.</div>}</section>}
  </>;
}

function ReportingView({ routes, integrations, backendConnections, onChanged, onMessage, onNavigate }: { routes: APISupportRoute[]; integrations: APIIntegration[]; backendConnections: APIBackendConnection[]; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
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
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("settings")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Settings</ConsoleLink></div>
    <PageHeading eyebrow="Settings" title="Bug reports & feedback" description="Configure consent-gated reporting and secure delivery. View submissions in Activity." action={<Button onClick={() => openRoute()}><Plus data-slot="icon" />New policy</Button>} />
    <div className="notice"><ShieldCheck /><span><strong>Consent is enforced.</strong> Agents preview the sanitized report and obtain explicit approval. Secret detection, encryption, and Private MCP isolation are enforced independently.</span></div>
    <section className="panel"><div className="panel-heading"><div><h2>Backend connections</h2><p>Service-to-service origins and bearer credentials are independent of customer identity.</p></div><Button outline onClick={() => openBackend()}><Plus data-slot="icon" />New connection</Button></div>{backendConnections.map((connection) => <div className="provider-row" key={connection.id}><span className="settings-icon"><Server /></span><span><strong>{connection.name}</strong><small>{connection.base_url} · bearer · {connection.credential_fingerprint ? `credential ${connection.credential_fingerprint}` : "no credential"}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><Button outline onClick={() => openBackend(connection)}>Edit</Button></div>)}{backendConnections.length === 0 && <div className="empty-row">Create a backend connection before enabling support delivery.</div>}</section>
    <section className="panel"><div className="panel-heading"><div><h2>Delivery policies</h2><p>Use one default policy and add API-specific exceptions only when necessary.</p></div><Badge color="violet">{routes.length} polic{routes.length === 1 ? "y" : "ies"}</Badge></div>{routes.map((route) => <div className="provider-row" key={route.id}><span className="settings-icon"><MessageSquareText /></span><span><EntityLink entity="support-route" uid={route.id} onNavigate={onNavigate} className="entity-link"><strong>{route.name}</strong></EntityLink><small>{route.is_default ? "Default for all APIs" : (route.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)?.display_name ?? id).join(", ")}</small></span><span>{route.bug_reports_enabled && <Badge color="blue">Bugs</Badge>} {route.feedback_enabled && <Badge color="violet">Feedback</Badge>}</span><span className="table-actions"><small>{route.bug_reports_enabled || route.feedback_enabled ? backendConnections.find((connection) => connection.id === route.backend_connection_id)?.name ?? "Backend unavailable" : "Delivery disabled"} · {route.retention_days} days</small><Button outline onClick={() => openRoute(route)}>Edit</Button></span></div>)}{routes.length === 0 && <div className="empty-row">Create a default policy to enable bug reports and feedback.</div>}</section>
    <Dialog open={routeOpen} onClose={setRouteOpen} title={editingRoute ? "Edit reporting policy" : "Create reporting policy"} description="Approved submissions are delivered to /v1/support-submissions through a separately authenticated backend connection." actions={<><Button outline onClick={() => setRouteOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !routeName.trim() || (!routeDefault && routeIntegrations.length === 0) || ((routeBugEnabled || routeFeedbackEnabled) && !backendConnections.some((connection) => connection.id === routeBackendID && connection.state === "active"))} onClick={saveRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Policy name</span><input value={routeName} onChange={(event) => setRouteName(event.target.value)} placeholder="Default reporting" /></label><label className="compact-check"><input type="checkbox" checked={routeDefault} onChange={(event) => setRouteDefault(event.target.checked)} /><span>Use as the default for all APIs</span></label>{!routeDefault && <fieldset className="catalog-settings-section"><legend>Assigned APIs</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={routeIntegrations.includes(integration.id)} onChange={() => toggleRouteIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset>}<div className="two-fields"><label className="compact-check"><input type="checkbox" checked={routeBugEnabled} onChange={(event) => setRouteBugEnabled(event.target.checked)} /><span>Enable bug reports</span></label><label className="compact-check"><input type="checkbox" checked={routeFeedbackEnabled} onChange={(event) => setRouteFeedbackEnabled(event.target.checked)} /><span>Enable feedback</span></label></div><label className="auth-field"><span>Backend connection</span><select value={routeBackendID} onChange={(event) => setRouteBackendID(event.target.value)}><option value="">No delivery connection</option>{backendConnections.map((connection) => <option key={connection.id} value={connection.id} disabled={connection.state !== "active"}>{connection.name} · {connection.state}</option>)}</select><small>The route stores only this reference; credentials rotate on the connection.</small></label><label className="auth-field"><span>Encrypted retention (days)</span><input type="number" min={1} max={365} value={routeRetention} onChange={(event) => setRouteRetention(event.target.value)} /></label></div></Dialog>
		<Dialog open={backendOpen} onClose={setBackendOpen} title={editingBackend ? "Edit backend connection" : "Create backend connection"} description="This credential is used only for service-to-service delivery, never for customer access or tool calls." actions={<><Button outline onClick={() => setBackendOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !backendName.trim() || !backendBaseURL.trim() || (backendState === "active" && !backendCredential.trim() && !editingBackend?.credential_fingerprint)} onClick={saveBackend}>{busy ? "Saving…" : "Save connection"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={backendName} onChange={(event) => setBackendName(event.target.value)} placeholder="Support delivery" /></label><label className="auth-field"><span>HTTPS origin</span><input type="url" value={backendBaseURL} onChange={(event) => setBackendBaseURL(event.target.value)} placeholder="https://backend.vendor.com" /><small>DokoSoko appends only /v1/support-submissions.</small></label><div className="two-fields"><label className="auth-field"><span>Authentication</span><input value="Bearer" disabled /></label><label className="auth-field"><span>State</span><select value={backendState} onChange={(event) => setBackendState(event.target.value as APIBackendConnection["state"])}><option value="disabled">Disabled</option><option value="active">Active</option></select></label></div><label className="auth-field"><span>{editingBackend ? "Rotate bearer credential (optional)" : "Bearer credential"}</span><input type="password" autoComplete="off" value={backendCredential} onChange={(event) => setBackendCredential(event.target.value)} /><small>Submitted once, encrypted immediately, and never returned.</small></label></div></Dialog>
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
    <div className="identity-flow"><span><LockKeyhole /><strong>1 · DokoSoko identity</strong><small>Authenticate the user and resolve a durable customer account.</small></span><span><ShieldCheck /><strong>2 · Access policy</strong><small>Validate schema, confirmation, grants, and the vendor access evaluation.</small></span><span><Users /><strong>3 · Upstream identity</strong><small>Use a separate user grant or encrypted service credential—never the inbound token.</small></span></div>
  </>;
}

function ToolsView({ tools, onAdd, onPublish, onNavigate }: { tools: APITool[]; onAdd: () => void; onPublish: (tool: APITool) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Actions" title="Tools" description="Publish reviewed HTTP actions and imported Stateless MCPv2 tools behind one authorization boundary." action={<Button onClick={onAdd}><Plus data-slot="icon" />Create API tool</Button>} /><div className="notice"><ShieldCheck /><span><strong>Policy-wrapped execution.</strong> Every call is schema validated, grant-scoped, reauthorized, rate-limited, and audited before a fixed backend is reached.</span></div><div className="tool-grid">{tools.map((tool) => <article className={`panel tool-card ${tool.upstream_drifted ? "drifted" : ""}`} key={tool.id}><span className="tool-icon">{tool.backend_kind === "mcp" ? <Share2 /> : tool.namespace === "credentials" ? <KeyRound /> : <TerminalSquare />}</span><div><span className="tool-badges"><Badge color={tool.state === "published" ? "green" : "amber"}>{tool.state}</Badge><Badge color={tool.backend_kind === "mcp" ? "violet" : "zinc"}>{tool.backend_kind === "mcp" ? "Stateless MCPv2" : "HTTP"}</Badge>{tool.upstream_drifted && <Badge color="red">Schema drift</Badge>}</span><h3><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link">{tool.namespace}.{tool.name}</EntityLink></h3><code>{tool.backend_kind === "mcp" ? `upstream · ${tool.upstream_tool_name}` : `${tool.http_method} · tool backend`}</code>{tool.state === "draft" && !tool.upstream_drifted && <Button outline className="publish-tool" onClick={() => onPublish(tool)}>Publish</Button>}{tool.upstream_drifted && <small className="drift-warning">Re-inspect and review before republishing.</small>}</div><button className="more" type="button" aria-label={`Actions for ${tool.name}`}><MoreHorizontal /></button></article>)}<button type="button" className="new-tool-card" onClick={onAdd}><Plus /><strong>Add API tool</strong><span>Definition → schema → API action → authz → test → publish</span></button></div></>;
}


function SettingsView({ product, versions, pins, customerAccounts, identity, llmProfiles, rootUsers, currentUser, onDoctor, onConfigureProduct, onConfigureIdentity, onConfigureLLM, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; versions: APIProductVersion[]; pins: APIProductVersionPin[]; customerAccounts: APICustomerAccount[]; identity: APIIdentity | null; llmProfiles: APILLMProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onConfigureProduct: () => void; onConfigureIdentity: () => void; onConfigureLLM: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <>
    <PageHeading eyebrow="Administration" title="Settings" description="Shared configuration for identity, customer data, service connections, and security." action={<Button outline onClick={onDoctor}><Activity data-slot="icon" />Run System Doctor</Button>} />
    <div className="settings-grid">
      <button type="button" className="settings-button" onClick={onConfigureIdentity}><SettingsCard icon={<Users />} title="Customer identity (optional)" detail={identity ? `OIDC ${identity.state} · ${customerAccounts.length} account${customerAccounts.length === 1 ? "" : "s"}` : "Configure delegated customer identity only when private access is needed"} status={identity ? identity.state : "Optional"} /></button>
      <button type="button" className="settings-button" onClick={() => onNavigate(sectionPath("projects"))}><SettingsCard icon={<KeyRound />} title="Service connections" detail="Encrypted vendor credentials shared explicitly with APIs" status="Manage" /></button>
      <button type="button" className="settings-button" onClick={() => onNavigate(sectionPath("reporting"))}><SettingsCard icon={<MessageSquareText />} title="Bug reports & feedback" detail="Consent-gated reporting policies and secure delivery endpoints" status="Manage" /></button>
      <SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted local object storage" status="Healthy" />
      <button type="button" className="settings-button" onClick={onConfigureLLM}><SettingsCard icon={<Bot />} title="LLM profiles & hardening" detail={`${llmProfiles.length} optional profile${llmProfiles.length === 1 ? "" : "s"} · model authority disabled`} status="Enforced" /></button>
      <SettingsCard icon={<ShieldCheck />} title="Root access" detail={`${activeRoots.length} MFA-protected administrator${activeRoots.length === 1 ? "" : "s"} · append-only audit`} status="Secure" />
    </div>
    <section className="panel identity-contract"><div className="panel-heading"><div><h2>Customer identity contract</h2><p>The optional OIDC organisation claim resolves to a durable internal account. Suspended accounts and a disabled identity provider fail closed immediately.</p></div><Button onClick={onConfigureIdentity}>{identity ? "Configure" : "Get started"}</Button></div><div className="contract-grid"><span><small>Customer accounts</small><strong>{customerAccounts.length}</strong></span><span><small>Active</small><strong>{customerAccounts.filter((account) => account.state === "active").length}</strong></span><span><small>Access evaluation</small><strong>POST /v1/access/evaluations</strong></span><span><small>Tool identity</small><strong>Delegated user token</strong></span></div><details className="advanced-details inline-advanced"><summary>Identity and API details</summary><div className="contract-grid"><span><small>OIDC issuer</small><code>{identity?.issuer ?? "Not configured"}</code></span><span><small>Organisation claim</small><code>{identity?.organisation_claim || "Not configured"}</code></span><span><small>Installation claim</small><code>{identity?.installation_claim || "Not configured"}</code></span><span><small>Delegated API origin</small><code>{identity?.delegated_api_origin || "Not configured"}</code></span></div></details></section>
    <details className="panel advanced-details"><summary>Advanced publishing</summary><div className="advanced-details-body"><div className="panel-heading"><div><h2>Publishing snapshots</h2><p>Immutable compatibility snapshots and scoped pins are retained for deterministic delivery. Most teams do not need to manage these directly.</p></div><Button outline onClick={onConfigureProduct}>Open advanced publishing</Button></div><div className="activity-summary"><span>{versions.length} published snapshot{versions.length === 1 ? "" : "s"}</span><span>{pins.length} scoped pin{pins.length === 1 ? "" : "s"}</span><span>Default {product.default_version_policy.toUpperCase()}</span></div></div></details>
    <section className="panel root-management"><div className="panel-heading"><div><h2>Root administrators</h2><p>Root access is independent from vendor identities and always requires MFA.</p></div><Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button></div>{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section>
  </>;
}

function WarningContent({ children }: { children: React.ReactNode }) { return <div className="warning-content"><div className="warning-icon"><TriangleAlert /></div><div>{children}</div></div>; }
function Confirmation({ checked, onChange, children }: { checked: boolean; onChange: (checked: boolean) => void; children: React.ReactNode }) { return <label className="confirmation"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span className="check-box">{checked && <Check />}</span><span>{children}</span></label>; }
function SummaryItem({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) { return <div className="summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>; }
function Metric({ label, value, detail, positive }: { label: string; value: string; detail: string; positive?: boolean }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small className={positive ? "positive" : ""}>{detail}</small></article>; }
function SettingsCard({ icon, title, detail, status }: { icon: React.ReactNode; title: string; detail: string; status: string }) {
  const statusColor: "amber" | "zinc" | "green" = status === "Required" ? "amber" : status === "Manage" ? "zinc" : "green";
  return <article className="panel settings-card"><span className="settings-icon">{icon}</span><div><h3>{title}</h3><p>{detail}</p></div><Badge color={statusColor}>{status}</Badge></article>;
}
