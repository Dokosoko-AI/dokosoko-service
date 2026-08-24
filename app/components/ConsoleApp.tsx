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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { APIAccessConnection, APIAccessCredential, APIAccessDefinition, APIAccessInstance, APIAIProviderConnection, APIAIProviderUsage, APIAIWorkloadProfile, APIAnalytics, APIAuditEvent, APIAuthorizationPoint, APIAuthorizationSimulation, APIBackendConnection, APICustomerAccount, APIDeployment, APIEnvironment, APIError, APIGrantDefinition, APIIdentity, APIIntegration, APIIntegrationAnalysis, APIIntegrationPackageBinding, APIIntegrationPreflight, APIIntegrationPublishStatus, APIIntegrationRevision, APIIntegrationRun, APIIntegrationToolBinding, APIMCPCatalog, APIMCPConnection, APIPackageArtifact, APIPackageRelease, APIProduct, APIProductBuild, APIProductBuildInput, APIProductDefinition, APIProductInstallation, APIProductVersion, APIProductVersionDiff, APIProductVersionImpact, APIProductVersionPin, APIProductVersionPinHistory, APIRecipe, APIRecipeReference, APIResourceSet, APISourcePublication, APISourceReview, APISupportRoute, APISupportSubmission, APITool, APIToolDryRun, APIUser, APIVisibility, APIWidget, APIWidgetInput, APIWidgetSecret, APIWidgetSession, Distribution, SetupEnrollment, api } from "../lib/api";
import { ConsoleRoute, EntityKind, INTEGRATION_RESOURCE_TABS, INTEGRATION_TABS, IntegrationResourceTab, IntegrationTab, SETTINGS_TABS, Section, SettingsTab, entityPath, integrationPath, parseConsolePath, routeForSection, sectionPath, settingsPath } from "../lib/console-routes";
import { Badge, Button, Dialog, Switch } from "./core/control";
import { Input, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./core";
import { DataTable, DataTableEmpty, DataTableHeader, DataTableRow, PageHeader as PageHeading, PageTabs, PanelHeader, SectionHeader, SegmentedControl, ViewStack } from "./core/layout";
import { ThemeToggle } from "./ThemeToggle";

type NavigationGroup = "apis" | "tools" | "recipes" | "agent-access" | "activity";
type Visibility = "private" | "public";
type AIWorkload = APIAIWorkloadProfile["workload"];
type SourceKind = "website" | "openapi" | "git" | "upload";

const sourceUploadMaxBytes = 5_000_000;
const sourceUploadExtensions = new Set([".md", ".mdx", ".txt", ".html", ".htm", ".json", ".yaml", ".yml"]);
const integrationRecipeScopeKind = "integration_scope";

function analysisMatchesIntegration(analysis: APIIntegrationAnalysis, integrationID?: string) {
  const scopes = analysis.evidence.filter((item) => item.kind === integrationRecipeScopeKind);
  return integrationID ? scopes.some((item) => item.resource_id === integrationID) : scopes.length === 0;
}

function recipeMatchesIntegration(recipe: APIRecipe, integrationID?: string) {
  const scopes = recipe.dependencies.filter((item) => item.kind === integrationRecipeScopeKind);
  return integrationID ? scopes.some((item) => item.resource_id === integrationID) : scopes.length === 0;
}

function sourceUploadValidationError(file: File) {
  const extension = file.name.slice(file.name.lastIndexOf(".")).toLowerCase();
  if (!sourceUploadExtensions.has(extension)) return "Choose a Markdown, text, HTML, JSON, or YAML file.";
  if (file.size > sourceUploadMaxBytes) return "The selected file is larger than the 5 MB limit for this setup.";
  if (file.size === 0) return "The selected file is empty.";
  return "";
}

const aiWorkloads: Array<{
  role: AIWorkload;
  name: string;
  description: string;
  icon: typeof Bot;
}> = [
  { role: "analysis", name: "Analysis", description: "Analyses evidence, writes recipes, and reviews every generated claim.", icon: Sparkles },
  { role: "assistant", name: "Assistant", description: "Answers quickly from evidence retrieved and authorized by DokoSoko.", icon: Bot },
];

const aiModelDefaults: Record<APIAIProviderConnection["provider"], Record<AIWorkload, string>> = {
  openai: { analysis: "gpt-5.6-terra", assistant: "gpt-5.6-luna" },
  google: { analysis: "gemini-3.5-flash", assistant: "gemini-3.5-flash-lite" },
  anthropic: { analysis: "claude-sonnet-5", assistant: "claude-haiku-4-5" },
  digitalocean: { analysis: "openai-gpt-5.6-terra", assistant: "openai-gpt-5.6-luna" },
  xai: { analysis: "grok-4.6", assistant: "grok-4.3" },
  deepseek: { analysis: "deepseek-v4-pro", assistant: "deepseek-v4-flash" },
  "openai-compatible": { analysis: "", assistant: "" },
};

const aiModelOptions: Record<APIAIProviderConnection["provider"], string[]> = {
  openai: ["gpt-5.6-terra", "gpt-5.6-sol", "gpt-5.6-luna"],
  google: ["gemini-3.5-flash", "gemini-3.6-flash", "gemini-3.5-flash-lite"],
  anthropic: ["claude-sonnet-5", "claude-opus-5", "claude-fable-5", "claude-haiku-4-5"],
  digitalocean: ["openai-gpt-5.6-terra", "openai-gpt-5.6-sol", "openai-gpt-5.6-luna", "deepseek-v4-pro", "deepseek-4-flash", "qwen3.8-max", "nvidia-nemotron-3-super-120b", "glm-5.2"],
  xai: ["grok-4.6", "grok-4.3", "grok-build-0.1"],
  deepseek: ["deepseek-v4-pro", "deepseek-v4-flash"],
  "openai-compatible": [],
};

const aiProviders: Array<{ id: APIAIProviderConnection["provider"]; name: string; description: string }> = [
  { id: "openai", name: "OpenAI", description: "Responses API with structured outputs." },
  { id: "google", name: "Google", description: "Gemini API with JSON-schema output." },
  { id: "anthropic", name: "Anthropic", description: "Claude Messages API with structured output." },
  { id: "digitalocean", name: "DigitalOcean", description: "Gradient serverless inference with one scoped model key." },
  { id: "xai", name: "xAI", description: "xAI Responses API with structured outputs." },
  { id: "deepseek", name: "DeepSeek", description: "DeepSeek chat API with JSON output." },
  { id: "openai-compatible", name: "Other OpenAPI compatible providers", description: "A fixed HTTPS OpenAI-compatible endpoint." },
];

function aiProviderLabel(provider: string) {
  return provider === "openai" ? "OpenAI" : provider === "google" ? "Google" : provider === "anthropic" ? "Anthropic" : provider === "digitalocean" ? "DigitalOcean" : provider === "xai" ? "xAI" : provider === "deepseek" ? "DeepSeek" : provider === "openai-compatible" ? "Other OpenAPI compatible providers" : provider;
}

function aiProviderOrigin(provider: APIAIProviderConnection["provider"]) {
  return provider === "openai" ? "https://api.openai.com" : provider === "google" ? "https://generativelanguage.googleapis.com" : provider === "anthropic" ? "https://api.anthropic.com" : provider === "digitalocean" ? "https://inference.do-ai.run" : provider === "xai" ? "https://api.x.ai" : provider === "deepseek" ? "https://api.deepseek.com" : "";
}

function apiFamilyKeyFromName(value: string) {
  return value.normalize("NFKD").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 63) || "api";
}

type Source = {
  id: string;
  name: string;
  kind: string;
  location: string;
  visibility: Visibility;
  published: boolean;
  quarantined: boolean;
  crawlState: "draft" | "queued" | "running" | "synced" | "review" | "failed" | "cancelled";
  pages: number;
  lastCrawl: string;
  revision: number;
  latestPublication?: APISourcePublication;
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
  { id: "tools", label: "Tools", icon: Wrench, defaultSection: "tools", sections: [{ id: "tools", label: "Catalog" }, { id: "connections", label: "Connections" }] },
  { id: "recipes", label: "Recipes", icon: BookOpen, defaultSection: "recipes", sections: [{ id: "recipes", label: "Recipes" }] },
  { id: "agent-access", label: "Agent access", icon: Radio, defaultSection: "distribution", sections: [{ id: "distribution", label: "Agent access" }, { id: "widgets", label: "Widgets" }] },
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

function ConsoleLink({ path, onNavigate, className, children, ariaCurrent, ariaLabel }: { path: string; onNavigate: (path: string) => void; className?: string; children: React.ReactNode; ariaCurrent?: "page"; ariaLabel?: string }) {
  return <a href={path} className={className} aria-current={ariaCurrent} aria-label={ariaLabel} onClick={(event) => {
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
	const [widgets, setWidgets] = useState<APIWidget[]>([]);
	const [resourceSets, setResourceSets] = useState<APIResourceSet[]>([]);
	const [accessDefinitions, setAccessDefinitions] = useState<APIAccessDefinition[]>([]);
	const [accessConnections, setAccessConnections] = useState<APIAccessConnection[]>([]);
	const [backendConnections, setBackendConnections] = useState<APIBackendConnection[]>([]);
	const [accessInstances, setAccessInstances] = useState<APIAccessInstance[]>([]);
	const [accessCredentials, setAccessCredentials] = useState<APIAccessCredential[]>([]);
	const [supportRoutes, setSupportRoutes] = useState<APISupportRoute[]>([]);
  const [consoleRoute, setConsoleRoute] = useState<ConsoleRoute>(() => routeForSection("product"));
  const section = consoleRoute.section;
  const settingsTab: SettingsTab = consoleRoute.kind === "section" && consoleRoute.section === "settings" ? consoleRoute.settingsTab ?? "overview" : "overview";
  const [productDefinition, setProductDefinition] = useState<APIProductDefinition | null>(fixtureDefinition);
  const [latestProductBuild, setLatestProductBuild] = useState<APIProductBuild | null>(fixtureProductBuild);
  const [productBuilderOpen, setProductBuilderOpen] = useState(false);
  const [productBuildReviewOpen, setProductBuildReviewOpen] = useState(false);
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
  const [apiConnected, setAPIConnected] = useState(false);
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
  const [addToolOpen, setAddToolOpen] = useState(false);
  const [toolNamespace, setToolNamespace] = useState("access");
  const [toolName, setToolName] = useState("");
  const [toolDescription, setToolDescription] = useState("");
  const [toolMethod, setToolMethod] = useState("POST");
  const [toolEndpoint, setToolEndpoint] = useState("");
  const [toolGrants, setToolGrants] = useState("");
  const [toolRisk, setToolRisk] = useState<"low" | "medium" | "high" | "critical">("medium");
  const [toolConfirmationRequired, setToolConfirmationRequired] = useState(false);
  const [toolIdempotencyRequired, setToolIdempotencyRequired] = useState(false);
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
	    Promise.all([api.integrations(), api.widgets(), api.resourceSets(), api.accessDefinitions(), api.accessConnections(), api.backendConnections(), api.supportRoutes()]).then(async ([integrationValues, widgetValues, setValues, definitionValues, connectionValues, backendValues, routeValues]) => {
	      if (cancelled) return;
	      setIntegrations(integrationValues); setWidgets(widgetValues); setResourceSets(setValues); setAccessDefinitions(definitionValues); setAccessConnections(connectionValues); setBackendConnections(backendValues); setSupportRoutes(routeValues);
	      const instanceGroups = await Promise.all(connectionValues.map((connection) => api.accessInstances(connection.id).catch(() => [])));
	      const credentialGroups = await Promise.all(connectionValues.map((connection) => api.accessCredentials(connection.id).catch(() => [])));
	      if (!cancelled) { setAccessInstances(instanceGroups.flat()); setAccessCredentials(credentialGroups.flat()); }
	    }).catch(() => {});
	    Promise.all([api.aiConnections(), api.aiProfiles(product.id)]).then(([connections, profiles]) => { if (!cancelled) { setAIConnections(connections); setAIProfiles(profiles); } }).catch(() => {});
	    Promise.all([api.analyses(product.id), api.recipes(product.id), api.aiUsage(product.id)]).then(([analysisValues, recipeValues, usageValues]) => { if (!cancelled) { setAnalyses(analysisValues); setRecipes(recipeValues); setAIProviderUsage(usageValues.providers); } }).catch(() => {});
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

  async function publishSource(source: Source) {
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
	} catch (error) {
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
	  setSources((items) => items.map((item) => item.id === result.source.id ? { ...item, published: result.source.published, quarantined: result.source.quarantined, revision: result.source.revision, crawlState: "synced", latestPublication: result.publication } : item));
	  setSourceReview(null);
	  setSourceReviewSelection([]);
	  setSourceReviewAcknowledged(false);
	  showToast(`${result.source.name} generation r${result.publication.revision} was atomically published.`);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not publish this reviewed generation.");
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  const newToolNamespaceValid = /^[a-z][a-z0-9_]{0,63}$/.test(toolNamespace.trim());
  const newToolNameValid = /^[a-z][a-z0-9_]{0,63}$/.test(toolName.trim());
  const newToolIdentityValid = newToolNamespaceValid && newToolNameValid;

  async function createTool() {
    if (!newToolIdentityValid) {
      showToast("Tool namespace and name must be lower-case identifiers.");
      return;
    }
    setToolBusy(true);
    try {
      const inputSchema = JSON.parse(toolInputSchema) as Record<string, unknown>;
      const outputSchema = JSON.parse(toolOutputSchema) as Record<string, unknown>;
      const authorizationPolicy = { required_grants: toolGrants.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean), confirmation_required: toolConfirmationRequired || toolRisk === "critical", risk: toolRisk, idempotency_required: toolIdempotencyRequired };
      const created = apiConnected ? await api.createTool(product.id, { organisation_id: product.organisation_id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, endpoint: toolEndpoint, http_method: toolMethod, authorization_policy: authorizationPolicy, timeout_ms: 10000 }) : { id: `tool_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, namespace: toolNamespace, name: toolName, description: toolDescription, input_schema: inputSchema, output_schema: outputSchema, state: "draft" as const, revision: 1, http_method: toolMethod, authorization_policy: authorizationPolicy, timeout_ms: 10000 };
      setTools((items) => [...items, created as APITool]);
      setAddToolOpen(false);
      setToolName(""); setToolDescription(""); setToolEndpoint(""); setToolGrants(""); setToolRisk("medium"); setToolConfirmationRequired(false); setToolIdempotencyRequired(false);
      navigateToPath(entityPath("tool", created.id));
      showToast(`${created.namespace}.${created.name} was saved as a draft.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Schema or tool configuration is invalid.");
    } finally {
      setToolBusy(false);
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

	  async function createRecipe(prompt: string): Promise<APIRecipe | null> {
	    setRecipeBusy(true);
	    try {
	      const value = await api.createRecipe(product.id, prompt);
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

  function navigateToPath(path: string, replace = false) {
    const next = parseConsolePath(path);
    if (typeof window !== "undefined") {
      const method = replace ? "replaceState" : "pushState";
      if (window.location.pathname !== next.path || replace) window.history[method](null, "", routeURL(next.path));
      window.scrollTo({ top: 0, behavior: "auto" });
    }
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

  return (
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

      <main id="main-content" tabIndex={-1}>
        <header className="topbar">
          <div className="product-switcher"><span className="product-logo">{product.name.slice(0, 1).toUpperCase()}</span><span><small>Deployment</small><strong>{product.name}</strong></span></div>
          <select className="mobile-navigation" aria-label="Console section" value={section === "settings" ? "settings" : activeNavigation?.id ?? "apis"} onChange={(event) => navigateToGroup(event.target.value as NavigationGroup | "settings")}>
            {navigation.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            <option value="settings">Settings</option>
          </select>
          <div className="topbar-actions"><div className="mobile-theme-toggle"><ThemeToggle /></div></div>
        </header>

        <div className="content"><ViewStack>
          {consoleRoute.kind === "not-found" ? <ConsoleNotFoundView path={consoleRoute.path} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "integration" ? <IntegrationsView integrations={integrations} resourceSets={resourceSets} sources={sources} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} identity={identityConfig} analyses={analyses} recipes={recipes} distribution={distribution} widgets={widgets} selectedIntegrationID={consoleRoute.uid} activeTab={consoleRoute.integrationTab} activeResourceTab={consoleRoute.integrationResourceTab ?? "documentation"} recipeBusy={recipeBusy} onBuild={() => setProductBuilderOpen(true)} onAddSource={() => setAddSourceOpen(true)} onCrawlSource={crawlSource} onPublishSource={publishSource} onGenerateRecipes={generateRecipesFromEvidence} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "widget" ? <WidgetDetailView key={`${consoleRoute.uid}:${widgets.find((item) => item.id === consoleRoute.uid)?.revision ?? 0}`} widget={widgets.find((item) => item.id === consoleRoute.uid) ?? null} integrations={integrations} assistantAvailable={aiProfiles.some((profile) => profile.workload === "assistant" && profile.enabled)} busy={widgetBusy} onUpdate={updateWidget} onSetState={setWidgetState} onRotateSecret={rotateWidgetSecret} onConfigureAssistant={() => { navigateToPath(settingsPath("ai")); openLLMProfile("assistant"); }} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "resource-set" ? <ResourceSetDetailView resource={resourceSets.find((item) => item.id === consoleRoute.uid) ?? null} integrations={integrations} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" && consoleRoute.entity === "tool" ? <ToolDetailView key={`${consoleRoute.uid}:${tools.find((item) => item.id === consoleRoute.uid)?.revision ?? 0}`} productID={product.id} tool={tools.find((item) => item.id === consoleRoute.uid) ?? null} connections={mcpConnections} integrations={integrations} auditEvents={auditEvents} onChanged={refreshTools} onMessage={showToast} onNavigate={navigateToPath} /> : consoleRoute.kind === "entity" ? <EntityDetailView route={consoleRoute} detail={entityDetail} onNavigate={navigateToPath} /> : <>
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
              onConfigureIdentity={() => navigateToSection("settings")}
              onOpenSources={() => navigateToSection("sources")}
              widgetCount={widgets.length}
              onOpenWidgets={() => navigateToSection("widgets")}
            />
          )}
          {section === "widgets" && <WidgetsView widgets={widgets} integrations={integrations} onCreate={() => setWidgetCreateOpen(true)} onNavigate={navigateToPath} />}
          {section === "product" && <IntegrationsView integrations={integrations} resourceSets={resourceSets} sources={sources} supportRoutes={supportRoutes} connections={accessConnections} tools={tools} identity={identityConfig} analyses={analyses} recipes={recipes} distribution={distribution} widgets={widgets} onBuild={() => setProductBuilderOpen(true)} onAddSource={() => setAddSourceOpen(true)} onCrawlSource={crawlSource} onPublishSource={publishSource} onGenerateRecipes={generateRecipesFromEvidence} recipeBusy={recipeBusy} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "recipes" && <RecipesView analyses={analyses} recipes={recipes} busy={recipeBusy} onCreate={createRecipe} onEdit={editRecipe} onRework={reworkRecipe} onApprove={approveRecipe} onPublish={publishRecipe} />}
          {section === "sources" && <SourcesView sources={sources} onAdd={() => setAddSourceOpen(true)} onCrawl={crawlSource} onPublish={publishSource} onVisibilityChange={(id) => requestVisibility("source", id)} onNavigate={navigateToPath} />}
          {section === "projects" && <AccessView definitions={accessDefinitions} connections={accessConnections} instances={accessInstances} credentials={accessCredentials} integrations={integrations} environments={environments} apiResourceSets={resourceSets.filter((set) => set.kind === "api")} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "connections" && <MCPConnectionsView connections={mcpConnections} tools={tools} busy={mcpBusy} onAdd={() => setMCPConnectionOpen(true)} onInspect={inspectMCPConnection} onNavigate={navigateToPath} />}
          {section === "tools" && <ToolsView tools={tools} integrations={integrations} connections={mcpConnections} onAdd={() => setAddToolOpen(true)} onNavigate={navigateToPath} />}
          {section === "releases" && <ConnectorReleasesView versions={productVersions} integrations={integrations} onConfigure={openProductCatalog} onNavigate={navigateToPath} />}
          {section === "runs" && <ActivityHubView runs={integrationRuns} environments={environments} submissions={reportSubmissions} events={auditEvents} analytics={analytics} supportRoutes={supportRoutes} onStart={() => setRunOpen(true)} onComplete={completeIntegrationRun} onView={openSupportSubmission} onRetry={createSupportDeliveryAttempt} onNavigate={navigateToPath} />}
          {section === "reporting" && <ReportingView routes={supportRoutes} integrations={integrations} backendConnections={backendConnections} onChanged={refreshCatalog} onMessage={showToast} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "overview" && <SettingsView product={product} versions={productVersions} pins={productVersionPins} customerAccounts={customerAccounts} identity={identityConfig} aiProfiles={aiProfiles} rootUsers={rootUsers} currentUser={currentUser ?? null} onDoctor={runSystemDoctor} onConfigureProduct={openProductCatalog} onConfigureIdentity={() => setIdentityOpen(true)} onAddRoot={() => { setRootRecoveryCodes([]); setRootOpen(true); }} onRevokeRoot={revokeRootUser} onNavigate={navigateToPath} />}
          {section === "settings" && settingsTab === "identity" && <CustomerIdentitySettingsView customerAccounts={customerAccounts} identity={identityConfig} onConfigure={() => setIdentityOpen(true)} onNavigate={navigateToPath} />}
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
        onClose={(open) => { if (!open && !sourceReviewBusy) { setSourceReview(null); setSourceReviewSelection([]); setSourceReviewAcknowledged(false); } }}
        title={`Review ${sourceReview?.source.name ?? "documentation"}`}
        description="Approve the exact completed crawl generation and only the immutable pages that should be available to Integrations."
        actions={<><Button outline disabled={sourceReviewBusy} onClick={() => setSourceReview(null)}>Cancel</Button><Button color="indigo" disabled={sourceReviewBusy || Boolean(sourceReview?.publication) || Boolean(sourceReview?.source.quarantined) || !sourceReviewAcknowledged || sourceReviewSelection.length === 0} onClick={confirmSourcePublication}>{sourceReviewBusy ? "Publishing…" : "Publish reviewed generation"}</Button></>}
      >
        {sourceReview && <div className="mcp-import-review">
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
        open={addToolOpen}
        onClose={setAddToolOpen}
        title="Create HTTP tool"
        description="Create one reusable deployment capability. APIs can attach the published tool later."
        actions={<><Button outline onClick={() => setAddToolOpen(false)}>Cancel</Button><Button color="indigo" disabled={toolBusy || !newToolIdentityValid || !toolDescription.trim() || !toolEndpoint.trim()} onClick={createTool}>{toolBusy ? "Validating…" : "Save draft"}</Button></>}
      >
	    <div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Namespace</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(toolNamespace) && !newToolNamespaceValid} aria-describedby="new-tool-identity-guidance" value={toolNamespace} onChange={(event) => setToolNamespace(event.target.value)} placeholder="platform" /></label><label className="auth-field"><span>Tool name</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(toolName) && !newToolNameValid} aria-describedby="new-tool-identity-guidance" value={toolName} onChange={(event) => setToolName(event.target.value)} placeholder="check_readiness" /></label></div><small id="new-tool-identity-guidance">Namespace and name use 1–64 lower-case letters, numbers or underscores, starting with a letter.</small><label className="auth-field"><span>Purpose</span><input value={toolDescription} onChange={(event) => setToolDescription(event.target.value)} placeholder="Describe one action for an agent." /></label><div className="two-fields"><label className="auth-field"><span>Method</span><select value={toolMethod} onChange={(event) => { const method = event.target.value; setToolMethod(method); if (method === "GET") setToolRisk("low"); else if (method === "DELETE") { setToolRisk("critical"); setToolConfirmationRequired(true); } else if (toolRisk === "low" || toolRisk === "critical") setToolRisk("medium"); }}>{["GET", "POST", "PUT", "PATCH", "DELETE"].map((value) => <option key={value}>{value}</option>)}</select></label><label className="auth-field"><span>Fixed endpoint</span><input type="url" value={toolEndpoint} onChange={(event) => setToolEndpoint(event.target.value)} placeholder="https://api.vendor.com/v1/action" /><small>Must use the configured delegated API origin. Agents cannot alter the host or authorization header.</small></label></div><label className="auth-field"><span>Required grants</span><input value={toolGrants} onChange={(event) => setToolGrants(event.target.value)} placeholder="platform.readiness developer.pro" /><small>Registered grant keys separated by spaces or commas.</small></label><div className="two-fields"><label className="auth-field"><span>Risk</span><select value={toolRisk} onChange={(event) => { const risk = event.target.value as typeof toolRisk; setToolRisk(risk); if (risk === "critical") setToolConfirmationRequired(true); }}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label><label className="compact-check"><input type="checkbox" disabled={toolRisk === "critical"} checked={toolConfirmationRequired || toolRisk === "critical"} onChange={(event) => setToolConfirmationRequired(event.target.checked)} /><span>Require explicit confirmation</span></label></div><label className="compact-check"><input type="checkbox" checked={toolIdempotencyRequired} onChange={(event) => setToolIdempotencyRequired(event.target.checked)} /><span>Require idempotency metadata for mutation calls</span></label><details className="inline-advanced"><summary>Contract schemas</summary><div className="two-fields tool-schema-fields"><label className="auth-field"><span>Input JSON Schema</span><textarea className="code-input" value={toolInputSchema} onChange={(event) => setToolInputSchema(event.target.value)} spellCheck={false} /></label><label className="auth-field"><span>Output JSON Schema</span><textarea className="code-input" value={toolOutputSchema} onChange={(event) => setToolOutputSchema(event.target.value)} spellCheck={false} /></label></div></details></div>
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

      {toast && <div className="toast" role="status"><Check />{toast}</div>}
    </div>
  );
}

function EntityDetailView({ route, detail, onNavigate }: { route: Extract<ConsoleRoute, { kind: "entity" }>; detail: EntityDetail | null; onNavigate: (path: string) => void }) {
  const parentPath = sectionPath(route.section);
  return <>
    <div className="entity-breadcrumb">
      <ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {route.section === "product" ? "APIs" : route.section === "projects" ? "Service connections" : route.section}</ConsoleLink>
      <code>{route.path}</code>
    </div>
    {detail ? <>
      <PageHeading eyebrow={detail.eyebrow} title={detail.title} description={detail.description || undefined} />
      <section className="panel entity-detail-panel">
        <PanelHeader title="Details" action={<Badge color="violet">{route.entity}</Badge>} />
        <dl className="entity-detail-grid">{detail.fields.map((field) => <div key={field.label}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}</dl>
      </section>
    </> : <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Item unavailable</h1><p>No {route.entity.replaceAll("-", " ")} with UID <code>{route.uid}</code> is available in this deployment, or it is still loading.</p></div><ConsoleLink path={parentPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to the directory</ConsoleLink></section>}
  </>;
}

function resourceSetIntegrations(resource: APIResourceSet, integrations: APIIntegration[]) {
  return integrations.filter((integration) => resource.integration_ids?.includes(integration.id) || integration.resources?.some((item) => item.resource_set_id === resource.id));
}

function manifestString(entry: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    if (typeof entry[key] === "string" && entry[key]) return entry[key] as string;
  }
  return "";
}

function manifestEntryTitle(entry: Record<string, unknown>, index: number) {
  return manifestString(entry, ["title", "name", "operationId", "operation_id", "path", "url", "location"]) || `Contract entry ${index + 1}`;
}

function manifestEntrySummary(entry: Record<string, unknown>) {
  const method = manifestString(entry, ["method", "http_method"]).toUpperCase();
  const location = manifestString(entry, ["path", "url", "location"]);
  const description = manifestString(entry, ["description", "summary"]);
  return [method, location, description].filter(Boolean).join(" · ") || `${Object.keys(entry).length} configured field${Object.keys(entry).length === 1 ? "" : "s"}`;
}

function ResourceSetDetailView({ resource, integrations, onNavigate }: { resource: APIResourceSet | null; integrations: APIIntegration[]; onNavigate: (path: string) => void }) {
  if (!resource) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Resource set unavailable</h1><p>This resource set does not exist or is still loading.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;
  const owners = resourceSetIntegrations(resource, integrations);
  const resourceTab: IntegrationResourceTab = resource.kind === "api" ? "contracts" : "documentation";
  const backPath = owners.length === 1 ? integrationPath(owners[0].id, "resources", resourceTab) : sectionPath("product");
  const backLabel = owners.length === 1 ? owners[0].display_name : "APIs";
  const revision = resource.latest_revision;
  const entries = revision?.manifest ?? [];

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={backPath} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to {backLabel}</ConsoleLink><Badge color={resource.kind === "api" ? "violet" : "blue"}>{resource.kind === "api" ? "API contract" : "Documentation"}</Badge></div>
    <PageHeading eyebrow="Reusable resource set" title={resource.name} description={resource.description || "Reusable resource configuration shared explicitly between APIs."} action={<Badge color={resource.state === "active" ? "green" : "zinc"}>{resource.state}</Badge>} />
    <dl className="compact-metrics resource-detail-metrics">
      <div className="compact-metric"><dt>Latest revision</dt><dd><strong>r{revision?.revision ?? resource.revision}</strong><small>Immutable snapshot</small></dd></div>
      <div className="compact-metric"><dt>Contract entries</dt><dd><strong>{entries.length}</strong><small>{resource.kind === "api" ? "API definitions" : "Documentation records"}</small></dd></div>
      <div className="compact-metric"><dt>Used by APIs</dt><dd><strong>{owners.length}</strong><small>Explicit attachments</small></dd></div>
      <div className="compact-metric"><dt>Updated</dt><dd><strong>{revision?.created_at ? new Date(revision.created_at).toLocaleDateString() : "—"}</strong><small>{revision?.created_at ? new Date(revision.created_at).toLocaleTimeString() : "No revision date"}</small></dd></div>
    </dl>
    <div className="entity-workspace-grid">
      <section className="panel entity-contract-panel">
        <PanelHeader title="Contract contents" description="What this reusable revision contributes when attached to an API." action={<Badge color="zinc">r{revision?.revision ?? resource.revision}</Badge>} />
        <div className="entity-contract-list">{entries.map((entry, index) => <article key={`${manifestEntryTitle(entry, index)}:${index}`}><span className="resource-icon">{resource.kind === "api" ? <TerminalSquare /> : <BookOpen />}</span><span><strong>{manifestEntryTitle(entry, index)}</strong><small>{manifestEntrySummary(entry)}</small></span><Badge color={resource.kind === "api" ? "violet" : "blue"}>{manifestString(entry, ["kind", "type", "method"]) || resource.kind}</Badge></article>)}{entries.length === 0 && <div className="empty-row">This revision contains no contract entries.</div>}</div>
        <details className="advanced-details inline-advanced"><summary>View revision JSON</summary><pre className="entity-contract-json">{JSON.stringify(entries, null, 2)}</pre></details>
      </section>
      <aside className="entity-workspace-rail">
        <section className="panel entity-related-panel"><PanelHeader title="Used by APIs" description="Open the exact API workspace tab that attaches this set." />{owners.map((integration) => <ConsoleLink key={integration.id} path={integrationPath(integration.id, "resources", resourceTab)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span><Badge color={integration.lifecycle === "active" ? "green" : "zinc"}>{integration.lifecycle}</Badge><ChevronRight /></ConsoleLink>)}{owners.length === 0 && <div className="empty-row">This set is not attached to an API.</div>}</section>
        <section className="panel entity-detail-panel"><PanelHeader title="Revision identity" /><dl className="entity-detail-grid compact-detail-grid"><div><dt>Resource set ID</dt><dd>{resource.id}</dd></div><div><dt>Content hash</dt><dd>{revision?.content_hash || "—"}</dd></div><div><dt>Revision ID</dt><dd>{revision?.id || "—"}</dd></div><div><dt>Attachment policy</dt><dd>Explicit only</dd></div></dl></section>
      </aside>
    </div>
  </>;
}

type ToolDetailTab = "overview" | "contract" | "execution" | "authorization" | "tests" | "usage" | "history";

const TOOL_DETAIL_TABS: Array<{ id: ToolDetailTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "contract", label: "Contract" },
  { id: "execution", label: "Execution" },
  { id: "authorization", label: "Authorization" },
  { id: "tests", label: "Tests" },
  { id: "usage", label: "Usage" },
  { id: "history", label: "History" },
];

function ToolDetailView({ productID, tool, connections, integrations, auditEvents, onChanged, onMessage, onNavigate }: { productID: string; tool: APITool | null; connections: APIMCPConnection[]; integrations: APIIntegration[]; auditEvents: APIAuditEvent[]; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const initialPolicy = tool ? toolPolicy(tool) : { requiredGrants: [], confirmationRequired: false, risk: "low", idempotencyRequired: false };
  const initialRisk = initialPolicy.risk === "medium" || initialPolicy.risk === "high" || initialPolicy.risk === "critical" ? initialPolicy.risk : "low";
  const toolID = tool?.id;
  const [activeTool, setActiveTool] = useState<APITool | null>(null);
  const [detailStatus, setDetailStatus] = useState<"loading" | "ready" | "error">(toolID ? "loading" : "error");
  const [detailLoadAttempt, setDetailLoadAttempt] = useState(0);
  const [activeTab, setActiveTab] = useState<ToolDetailTab>("overview");
  const [usages, setUsages] = useState<Array<{ integration: APIIntegration; binding: APIIntegrationToolBinding }>>([]);
  const [usageStatus, setUsageStatus] = useState<"loading" | "ready" | "partial">("loading");
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [description, setDescription] = useState(tool?.description ?? "");
  const [endpoint, setEndpoint] = useState(tool?.endpoint ?? "");
  const [method, setMethod] = useState(tool?.http_method ?? "POST");
  const [inputSchema, setInputSchema] = useState(JSON.stringify(tool?.input_schema ?? {}, null, 2));
  const [outputSchema, setOutputSchema] = useState(JSON.stringify(tool?.output_schema ?? {}, null, 2));
  const [grants, setGrants] = useState(initialPolicy.requiredGrants.join(", "));
  const [risk, setRisk] = useState<"low" | "medium" | "high" | "critical">(initialRisk);
  const [confirmationRequired, setConfirmationRequired] = useState(initialPolicy.confirmationRequired);
  const [idempotencyRequired, setIdempotencyRequired] = useState(initialPolicy.idempotencyRequired);
  const [timeout, setTimeoutValue] = useState(String(tool?.timeout_ms ?? 10000));
  const [testInput, setTestInput] = useState("{}");
  const [testResult, setTestResult] = useState<APIToolDryRun | null>(null);
  const [cloneOpen, setCloneOpen] = useState(false);
  const [cloneNamespace, setCloneNamespace] = useState("");
  const [cloneName, setCloneName] = useState("");
  const [retireOpen, setRetireOpen] = useState(false);
  const cloneIdentityValid = /^[a-z][a-z0-9_]{0,63}$/.test(cloneNamespace.trim()) && /^[a-z][a-z0-9_]{0,63}$/.test(cloneName.trim());

  useEffect(() => {
    if (!toolID) return;
    let cancelled = false;
    api.tool(productID, toolID).then((value) => {
      if (cancelled) return;
      const policy = toolPolicy(value);
      setActiveTool(value);
      setDescription(value.description);
      setEndpoint(value.endpoint ?? "");
      setMethod(value.http_method);
      setInputSchema(JSON.stringify(value.input_schema, null, 2));
      setOutputSchema(JSON.stringify(value.output_schema, null, 2));
      setGrants(policy.requiredGrants.join(", "));
      setRisk(policy.risk === "medium" || policy.risk === "high" || policy.risk === "critical" ? policy.risk : "low");
      setConfirmationRequired(policy.confirmationRequired);
      setIdempotencyRequired(policy.idempotencyRequired);
      setTimeoutValue(String(value.timeout_ms));
      setTestInput("{}");
      setTestResult(null);
      setDirty(false);
      setDetailStatus("ready");
    }).catch(() => {
	  if (!cancelled) setDetailStatus("error");
	});
    return () => { cancelled = true; };
  }, [productID, toolID, detailLoadAttempt]);

  useEffect(() => {
    if (!toolID) return;
    let cancelled = false;
    Promise.all(integrations.map(async (integration) => {
      try { return { integration, bindings: await api.integrationToolBindings(integration.id), failed: false }; }
      catch { return { integration, bindings: [] as APIIntegrationToolBinding[], failed: true }; }
    })).then((results) => {
      if (cancelled) return;
      setUsages(results.flatMap(({ integration, bindings }) => bindings.filter((binding) => binding.tool_id === toolID).map((binding) => ({ integration, binding }))));
      setUsageStatus(results.some((result) => result.failed) ? "partial" : "ready");
    });
    return () => { cancelled = true; };
  }, [toolID, integrations]);

  if (!toolID) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Tool unavailable</h1><p>This tool could not be found in the deployment catalog.</p></div><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink></section>;
  if (!activeTool) return <section className="panel entity-missing" aria-live="polite"><span className="entity-missing-icon">{detailStatus === "loading" ? <RefreshCw /> : <TriangleAlert />}</span><div><h1>{detailStatus === "loading" ? "Loading tool" : "Tool details unavailable"}</h1><p>{detailStatus === "loading" ? "Loading the complete contract and fixed execution target…" : "The complete tool contract could not be loaded. No editing or lifecycle action is available."}</p></div>{detailStatus === "error" ? <Button outline onClick={() => { setActiveTool(null); setDetailStatus("loading"); setDetailLoadAttempt((value) => value + 1); }}>Retry</Button> : <ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to tools</ConsoleLink>}</section>;

  const currentTool = activeTool;
  const canEdit = currentTool.state === "draft";
  const connection = activeTool.mcp_connection_id ? connections.find((item) => item.id === activeTool.mcp_connection_id) : null;
  const currentPolicy = toolPolicy(activeTool);
  const toolEvents = auditEvents.filter((event) => event.target_type === "tool" && event.target_id === activeTool.id).sort((left, right) => right.created_at.localeCompare(left.created_at));
  const riskColor = risk === "critical" ? "red" : risk === "high" ? "amber" : risk === "medium" ? "violet" : "zinc";
  const markDirty = () => setDirty(true);
  const refreshAfterMutation = async () => { try { await onChanged(); return true; } catch { return false; } };
  const draftPayload = () => ({ description: description.trim(), endpoint: endpoint.trim(), http_method: method, input_schema: JSON.parse(inputSchema), output_schema: JSON.parse(outputSchema), authorization_policy: { ...currentTool.authorization_policy, required_grants: grants.split(/[,\s]+/).filter(Boolean), confirmation_required: confirmationRequired || risk === "critical", risk, idempotency_required: idempotencyRequired }, timeout_ms: Number(timeout), revision: currentTool.revision });

  async function saveToolDraft() {
    if (!canEdit) return;
    setBusy(true);
    try {
      const updated = await api.updateTool(productID, currentTool.id, draftPayload());
      setActiveTool(updated);
      setDirty(false);
      const refreshed = await refreshAfterMutation();
      onMessage(`${updated.namespace}.${updated.name} draft saved.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool editing is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "Tool draft could not be saved."); } finally { setBusy(false); }
  }

  async function publishToolRevision() {
    if (!canEdit) return;
    setBusy(true);
    try {
      let target: APITool = currentTool;
      if (dirty) {
        target = await api.updateTool(productID, currentTool.id, draftPayload());
        setActiveTool(target);
        setDirty(false);
      }
      const published = await api.publishTool(productID, target.id, target.revision);
      setActiveTool(published);
      const refreshed = await refreshAfterMutation();
      onMessage(`${published.namespace}.${published.name} published and available for API binding.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "Tool could not be published."); } finally { setBusy(false); }
  }

  async function dryRunTool() {
    setBusy(true);
    setTestResult(null);
    try {
      if (dirty && canEdit) {
        onMessage("Save the draft before testing its persisted contract.");
        return;
      }
      const result = await api.dryRunTool(productID, currentTool.id, JSON.parse(testInput));
      setTestResult(result);
      onMessage(result.valid && !result.network_call_performed ? "Tool contract validation passed without a network call." : "Tool contract validation returned a controlled failure.");
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool testing is not enabled by this service version yet." : error instanceof APIError || error instanceof Error ? error.message : "Tool test could not run."); } finally { setBusy(false); }
  }

  function handleToolTabKeyDown(event: ReactKeyboardEvent<HTMLButtonElement>) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const currentIndex = TOOL_DETAIL_TABS.findIndex((tab) => `tool-tab-${tab.id}` === document.activeElement?.id);
    if (currentIndex < 0) return;
    event.preventDefault();
    const nextIndex = event.key === "Home" ? 0 : event.key === "End" ? TOOL_DETAIL_TABS.length - 1 : event.key === "ArrowRight" ? (currentIndex + 1) % TOOL_DETAIL_TABS.length : (currentIndex - 1 + TOOL_DETAIL_TABS.length) % TOOL_DETAIL_TABS.length;
    const nextTab = TOOL_DETAIL_TABS[nextIndex];
    setActiveTab(nextTab.id);
    requestAnimationFrame(() => document.getElementById(`tool-tab-${nextTab.id}`)?.focus());
  }

  function openCloneTool() {
	if (currentTool.backend_kind === "mcp") {
	  onMessage("Imported MCP tools are updated through their upstream connection and cannot be cloned.");
	  return;
	}
    const suffix = "_next";
    setCloneNamespace(currentTool.namespace);
    setCloneName(`${currentTool.name.slice(0, Math.max(1, 64 - suffix.length))}${suffix}`);
    setCloneOpen(true);
  }

  async function cloneTool() {
    setBusy(true);
    try {
      const cloned = await api.cloneTool(productID, currentTool.id, cloneNamespace.trim(), cloneName.trim());
      const refreshed = await refreshAfterMutation();
      setCloneOpen(false);
      onMessage(`${cloned.namespace}.${cloned.name} created as an independent draft.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
      onNavigate(entityPath("tool", cloned.id));
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool cloning is not enabled by this service version yet." : error instanceof APIError ? error.message : "Tool could not be cloned."); } finally { setBusy(false); }
  }

  async function retireTool() {
    setBusy(true);
    try {
      const retired = await api.retireTool(productID, currentTool.id, currentTool.revision);
      setActiveTool(retired);
      const refreshed = await refreshAfterMutation();
      setRetireOpen(false);
      onMessage(`Tool retired. Existing exact API bindings are now unresolved and must be removed before publication.${refreshed ? "" : " Reload to refresh the surrounding catalog."}`);
    } catch (error) { onMessage(unavailableConsoleCapability(error) ? "Tool retirement is not enabled by this service version yet." : error instanceof APIError ? error.message : "Tool could not be retired."); } finally { setBusy(false); }
  }

  const readiness = [
    { label: "Agent contract", ready: Boolean(activeTool.description && Object.keys(activeTool.input_schema).length && Object.keys(activeTool.output_schema).length) },
    { label: "Fixed execution target", ready: activeTool.backend_kind === "mcp" ? Boolean(connection && activeTool.upstream_tool_name) : Boolean(activeTool.endpoint) },
    { label: "Safety policy", ready: ["low", "medium", "high", "critical"].includes(currentPolicy.risk ?? "low") },
    { label: "Published for managed binding", ready: activeTool.state === "published" && !activeTool.upstream_drifted },
  ];

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />All tools</ConsoleLink><Badge color={activeTool.backend_kind === "mcp" ? "violet" : "zinc"}>{activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"}</Badge></div>
    <PageHeading eyebrow="Deployment tool" title={`${activeTool.namespace}.${activeTool.name}`} action={<span className="heading-actions">{activeTool.state === "draft" && <><Button outline disabled={busy || !dirty} onClick={saveToolDraft}>{busy ? "Saving…" : "Save draft"}</Button><Button color="indigo" disabled={busy || activeTool.upstream_drifted} onClick={publishToolRevision}>Publish tool</Button></>}{activeTool.state === "published" && <>{activeTool.backend_kind !== "mcp" && <Button outline disabled={busy} onClick={openCloneTool}><Copy data-slot="icon" />Clone as new tool</Button>}{activeTool.backend_kind === "mcp" && connection && <ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-back-link">Review connection</ConsoleLink>}<Button outline disabled={busy} onClick={() => setRetireOpen(true)}>Retire</Button></>}{activeTool.state === "retired" && <Badge color="zinc">Retired</Badge>}</span>} />
    <div className="page-tabs" role="tablist" aria-label="Tool sections">{TOOL_DETAIL_TABS.map((tab) => <button type="button" role="tab" id={`tool-tab-${tab.id}`} aria-controls={`tool-panel-${tab.id}`} aria-selected={activeTab === tab.id} tabIndex={activeTab === tab.id ? 0 : -1} key={tab.id} className={`page-tab ${activeTab === tab.id ? "active" : ""}`} onKeyDown={handleToolTabKeyDown} onClick={() => setActiveTab(tab.id)}>{tab.label}</button>)}</div>

    {activeTab === "overview" && <div className="tool-detail-section" role="tabpanel" id="tool-panel-overview" aria-labelledby="tool-tab-overview" tabIndex={0}>
      <dl className="compact-metrics tool-detail-metrics"><div className="compact-metric"><dt>State</dt><dd><strong>{activeTool.state}</strong><small>revision {activeTool.revision}</small></dd></div><div className="compact-metric"><dt>Backend</dt><dd><strong>{activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"}</strong><small>{activeTool.backend_kind === "mcp" ? activeTool.upstream_tool_name || "Upstream tool" : `${activeTool.http_method} request`}</small></dd></div><div className="compact-metric"><dt>Risk</dt><dd><strong>{currentPolicy.risk ?? "low"}</strong><small>{currentPolicy.confirmationRequired ? "Confirmation required" : "No confirmation"}</small></dd></div><div className="compact-metric"><dt>Current config</dt><dd><strong>{usageStatus === "loading" ? "…" : usages.length}</strong><small>API binding{usages.length === 1 ? "" : "s"}</small></dd></div></dl>
      <div className="entity-workspace-grid"><section className="panel"><PanelHeader title="Readiness" description="A published tool becomes eligible for a managed API to attach; publication alone does not select it for that API." />{readiness.map((item) => <div className="integration-health-check" key={item.label}><span className={`health-icon ${item.ready ? "ready" : ""}`}>{item.ready ? <CheckCircle2 /> : <XCircle />}</span><span><strong>{item.label}</strong><small>{item.ready ? "Ready" : "Action required"}</small></span><Badge color={item.ready ? "green" : "amber"}>{item.ready ? "Ready" : "Review"}</Badge></div>)}</section><aside className="entity-workspace-rail"><section className="panel entity-policy-panel"><PanelHeader title="Delivery boundary" /><div className="entity-policy-check"><span className="ready"><ShieldCheck /></span><span><strong>Private MCP</strong><small>{activeTool.state === "published" ? "Managed API discovery requires an exact tool and authorization binding." : "Publish before managed APIs can bind this tool."}</small></span></div><div className="entity-policy-check"><span className="neutral"><Bot /></span><span><strong>Widget</strong><small>Catalog explanation only; execution requires an authorized Private MCP client.</small></span></div></section><section className="panel entity-detail-panel"><PanelHeader title="Identity" /><dl className="entity-detail-grid compact-detail-grid"><div><dt>Tool ID</dt><dd>{activeTool.id}</dd></div><div><dt>Revision</dt><dd>{activeTool.revision}</dd></div><div><dt>Drift</dt><dd>{activeTool.upstream_drifted ? "Detected" : "None"}</dd></div><div><dt>Lifecycle</dt><dd>{activeTool.state}</dd></div></dl></section></aside></div>
    </div>}

    {activeTab === "contract" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-contract" aria-labelledby="tool-tab-contract" tabIndex={0}><PanelHeader title="Agent contract" description="The name is stable. Published tools are immutable; cloning creates a new HTTP tool identity." action={dirty && <Badge color="amber">Unsaved</Badge>} /><label className="auth-field"><span>Purpose</span><textarea readOnly={!canEdit} value={description} onChange={(event) => { setDescription(event.target.value); markDirty(); }} /></label><div className="two-fields tool-schema-fields"><label className="auth-field"><span>Input JSON Schema</span><textarea spellCheck={false} readOnly={!canEdit} value={inputSchema} onChange={(event) => { setInputSchema(event.target.value); markDirty(); }} /></label><label className="auth-field"><span>Output JSON Schema</span><textarea spellCheck={false} readOnly={!canEdit} value={outputSchema} onChange={(event) => { setOutputSchema(event.target.value); markDirty(); }} /></label></div></section>}

    {activeTab === "execution" && <div className="entity-workspace-grid" role="tabpanel" id="tool-panel-execution" aria-labelledby="tool-tab-execution" tabIndex={0}><section className="panel tool-editor-page"><PanelHeader title="Execution" description="The destination is fixed before publication and cannot be supplied by an agent." /><div className="two-fields"><label className="auth-field"><span>Backend</span><input value={activeTool.backend_kind === "mcp" ? "MCP" : "HTTP"} readOnly /></label><label className="auth-field"><span>Timeout (ms)</span><input readOnly={!canEdit} type="number" min={100} max={60000} value={timeout} onChange={(event) => { setTimeoutValue(event.target.value); markDirty(); }} /></label></div>{activeTool.backend_kind !== "mcp" ? <><div className="two-fields"><label className="auth-field"><span>Method</span>{canEdit ? <select value={method} onChange={(event) => { setMethod(event.target.value); markDirty(); }}>{["GET", "POST", "PUT", "PATCH", "DELETE"].map((value) => <option key={value}>{value}</option>)}</select> : <input value={method} readOnly />}</label><label className="auth-field"><span>Fixed endpoint</span><input readOnly={!canEdit} type="url" value={endpoint} onChange={(event) => { setEndpoint(event.target.value); markDirty(); }} /></label></div><div className="private-default-note"><LockKeyhole />The authenticated customer&apos;s delegated vendor token is forwarded only to this configured origin.</div></> : <dl className="entity-detail-grid"><div><dt>Upstream tool</dt><dd>{activeTool.upstream_tool_name}</dd></div><div><dt>Schema hash</dt><dd>{activeTool.upstream_schema_hash}</dd></div></dl>}</section><aside className="entity-workspace-rail">{connection ? <section className="panel entity-related-panel"><PanelHeader title="Connection" /><ConsoleLink path={entityPath("connection", connection.id)} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><Share2 /></span><span><strong>{connection.name}</strong><small>{connection.protocol_version} · {connection.auth_mode}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><ChevronRight /></ConsoleLink></section> : <section className="panel entity-detail-panel"><PanelHeader title="Connection model" /><p className="entity-panel-copy">HTTP tools use the deployment&apos;s delegated API origin. Credentials are never stored on the tool.</p></section>}</aside></div>}

    {activeTab === "authorization" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-authorization" aria-labelledby="tool-tab-authorization" tabIndex={0}><PanelHeader title="Baseline authorization" description="An API authorization point may add stricter requirements but cannot weaken this policy." action={<Badge color={riskColor}>{risk} risk</Badge>} /><label className="auth-field"><span>Required registered grants</span><input readOnly={!canEdit} value={grants} onChange={(event) => { setGrants(event.target.value); markDirty(); }} placeholder="customers.read, invoices.write" /></label><div className="two-fields"><label className="auth-field"><span>Risk</span>{canEdit ? <select value={risk} onChange={(event) => { const next = event.target.value as typeof risk; setRisk(next); if (next === "critical") setConfirmationRequired(true); markDirty(); }}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select> : <input value={risk} readOnly />}</label>{canEdit && <label className="compact-check"><input disabled={risk === "critical"} type="checkbox" checked={confirmationRequired || risk === "critical"} onChange={(event) => { setConfirmationRequired(event.target.checked); markDirty(); }} /><span>Require explicit user confirmation</span></label>}</div>{canEdit ? <label className="compact-check"><input type="checkbox" checked={idempotencyRequired} onChange={(event) => { setIdempotencyRequired(event.target.checked); markDirty(); }} /><span>Require idempotency metadata for mutation calls</span></label> : <dl className="entity-detail-grid compact-detail-grid readonly-policy"><div><dt>Explicit confirmation</dt><dd>{confirmationRequired || risk === "critical" ? "Required" : "Not required"}</dd></div><div><dt>Idempotency metadata</dt><dd>{idempotencyRequired ? "Required" : "Not required"}</dd></div></dl>}{currentPolicy.requiredGrants.length > 0 && <div className="entity-grant-list">{currentPolicy.requiredGrants.map((grant) => <code key={grant}>{grant}</code>)}</div>}</section>}

    {activeTab === "tests" && <section className="panel tool-editor-page" role="tabpanel" id="tool-panel-tests" aria-labelledby="tool-tab-tests" tabIndex={0}><PanelHeader title="Contract validation" description="Validates arguments against the persisted input schema, fixed destination and normalized policy. It performs no network or output validation." action={<Button outline disabled={busy || dirty} onClick={dryRunTool}>{busy ? "Checking…" : "Run validation"}</Button>} />{dirty && <div className="capability-unavailable"><TriangleAlert /><span><strong>Save this draft first.</strong><small>Validation always runs against the persisted revision shown in the header.</small></span></div>}<label className="auth-field"><span>JSON arguments</span><textarea spellCheck={false} value={testInput} onChange={(event) => setTestInput(event.target.value)} /></label>{testResult && <pre role="status" aria-live="polite" className={`tool-test-result ${testResult.valid && !testResult.network_call_performed ? "passed" : "failed"}`}>{JSON.stringify(testResult, null, 2)}</pre>}</section>}

    {activeTab === "usage" && <section className="panel" role="tabpanel" id="tool-panel-usage" aria-labelledby="tool-tab-usage" tabIndex={0}><PanelHeader title="Current API configuration" description="Each current API draft pins an exact published tool revision and one exact authorization-point revision. Published snapshots are not counted here." action={<Badge color="violet">{usageStatus === "loading" ? "…" : usages.length}</Badge>} />{usageStatus === "partial" && <div className="capability-unavailable"><TriangleAlert /><span><strong>Some API bindings could not be loaded.</strong><small>The list below may be incomplete.</small></span></div>}{usages.map(({ integration, binding }) => { const point = binding.authorization_point; const current = binding.tool_revision === activeTool.revision && activeTool.state === "published" && !activeTool.upstream_drifted && Boolean(point && point.state === "active" && point.revision === binding.authorization_point_revision); return <ConsoleLink key={`${integration.id}:${binding.tool_revision}`} path={integrationPath(integration.id, "tools")} onNavigate={onNavigate} className="entity-related-row"><span className="settings-icon"><GitBranch /></span><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key} · tool r{binding.tool_revision} · authorization r{binding.authorization_point_revision}</small></span><Badge color={current ? "green" : "red"}>{current ? "Current" : "Stale / unresolved"}</Badge><ChevronRight /></ConsoleLink>; })}{usageStatus === "loading" && <div className="empty-row">Loading current API bindings…</div>}{usageStatus === "ready" && usages.length === 0 && <div className="empty-row">No current API configuration binds this tool.</div>}</section>}

    {activeTab === "history" && <section className="panel" role="tabpanel" id="tool-panel-history" aria-labelledby="tool-tab-history" tabIndex={0}><PanelHeader title="Tool activity" description="Append-only administrative and execution events loaded for this tool. Same-identity contract revision editing is not enabled yet." action={<ConsoleLink path={sectionPath("runs")} onNavigate={onNavigate} className="entity-back-link">Open activity</ConsoleLink>} />{toolEvents.map((event) => <div className="lease-row" key={event.id}><span><strong>{event.action}</strong><small>{event.actor_id || "system"} · {event.request_id || "no request ID"}</small></span><time>{new Date(event.created_at).toLocaleString()}</time></div>)}{toolEvents.length === 0 && <div className="empty-row">No loaded activity for this tool.</div>}</section>}

    {activeTool.backend_kind !== "mcp" && <Dialog open={cloneOpen} onClose={setCloneOpen} title="Clone as a new tool" description="This service does not yet create a new revision under the same identity. Choose a distinct lower-case namespace and name for the independent draft." actions={<><Button outline onClick={() => setCloneOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !cloneIdentityValid} onClick={cloneTool}>{busy ? "Cloning…" : "Create draft"}</Button></>}><div className="two-fields"><label className="auth-field"><span>Namespace</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneNamespace) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneNamespace.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneNamespace} onChange={(event) => setCloneNamespace(event.target.value)} /></label><label className="auth-field"><span>Name</span><input maxLength={64} pattern="[a-z][a-z0-9_]{0,63}" aria-invalid={Boolean(cloneName) && !/^[a-z][a-z0-9_]{0,63}$/.test(cloneName.trim())} aria-describedby="clone-tool-identity-guidance" value={cloneName} onChange={(event) => setCloneName(event.target.value)} /></label></div><small id="clone-tool-identity-guidance">Use 1–64 lower-case letters, numbers or underscores, starting with a letter.</small></Dialog>}
    <Dialog open={retireOpen} onClose={setRetireOpen} title={`Retire ${activeTool.namespace}.${activeTool.name}?`} description="Retirement removes the tool from discovery and prevents new bindings. API drafts using it must remove their binding before publication." actions={<><Button outline onClick={() => setRetireOpen(false)}>Cancel</Button><Button color="red" disabled={busy} onClick={retireTool}>{busy ? "Retiring…" : "Retire tool"}</Button></>}><div className="private-default-note"><TriangleAlert />This changes the deployment catalogue immediately. Existing published API snapshots remain audit evidence.</div></Dialog>
  </>;
}

function ConsoleNotFoundView({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">Navigation</p><h1>Page not found</h1><p><code>{path}</code> is not a recognised console URL.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;
}

function widgetOriginLabel(origin: string): string {
  try { return new URL(origin).host; } catch { return "Invalid origin"; }
}

function WidgetsView({ widgets, integrations, onCreate, onNavigate }: { widgets: APIWidget[]; integrations: APIIntegration[]; onCreate: () => void; onNavigate: (path: string) => void }) {
  const integrationName = (id: string) => integrations.find((integration) => integration.id === id)?.display_name ?? id;
  return <>
    <PageHeading eyebrow="Agent access" title="Widgets" action={<Button color="indigo" onClick={onCreate}><Plus data-slot="icon" />Create widget</Button>} />
    <div className="widget-principle"><ShieldCheck /><span><strong>One identity boundary.</strong> Your backend authenticates the user; DokoSoko limits every session to the APIs configured here.</span></div>
    <DataTable label="Widgets" className="widget-directory">
      <DataTableHeader className="widget-columns"><span>Widget</span><span>Application</span><span>APIs</span><span>Status</span><span>Open</span></DataTableHeader>
      {widgets.map((widget) => <DataTableRow className="widget-columns" key={widget.id}>
        <span className="resource-name"><span className="resource-icon"><MessageSquareText /></span><span><ConsoleLink path={entityPath("widget", widget.id)} onNavigate={onNavigate} className="entity-link"><strong>{widget.name}</strong></ConsoleLink><small>{widget.id}</small></span></span>
        <span><strong className="cell-value">{widget.allowed_origins[0] ? widgetOriginLabel(widget.allowed_origins[0]) : "Not configured"}</strong><small className="cell-note">{widget.allowed_origins.length === 1 ? "1 allowed origin" : `${widget.allowed_origins.length} allowed origins`}</small></span>
        <span><strong className="cell-value">{widget.integration_ids.length}</strong><small className="cell-note">{widget.integration_ids.slice(0, 2).map(integrationName).join(", ") || "No access"}</small></span>
        <Badge color={widget.state === "active" ? "green" : widget.state === "disabled" ? "red" : "zinc"}>{widget.state}</Badge>
        <span className="table-open-cell"><ConsoleLink path={entityPath("widget", widget.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${widget.name}`}><ChevronRight /></ConsoleLink></span>
      </DataTableRow>)}
      {widgets.length === 0 && <DataTableEmpty columns={5}><div className="widget-empty"><span className="entity-missing-icon"><MessageSquareText /></span><div><h2>No widgets yet</h2><p>Create one authenticated widget, connect the APIs it needs, and verify the installation before going live.</p></div><Button color="indigo" onClick={onCreate}><Plus data-slot="icon" />Create widget</Button></div></DataTableEmpty>}
    </DataTable>
  </>;
}

function WidgetDetailView({ widget, integrations, assistantAvailable, busy, onUpdate, onSetState, onRotateSecret, onConfigureAssistant, onMessage, onNavigate }: { widget: APIWidget | null; integrations: APIIntegration[]; assistantAvailable: boolean; busy: boolean; onUpdate: (widget: APIWidget, input: APIWidgetInput) => Promise<APIWidget | null>; onSetState: (widget: APIWidget, state: "active" | "disabled") => Promise<APIWidget | null>; onRotateSecret: (widget: APIWidget) => void | Promise<void>; onConfigureAssistant: () => void; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [name, setName] = useState(widget?.name ?? "");
  const [origins, setOrigins] = useState(widget?.allowed_origins.join("\n") ?? "");
  const [integrationIDs, setIntegrationIDs] = useState<string[]>(widget?.integration_ids ?? []);
  const [theme, setTheme] = useState<"auto" | "light" | "dark">(widget?.appearance.theme ?? "auto");
  const [accent, setAccent] = useState(widget?.appearance.accentColour ?? "");
  const [greeting, setGreeting] = useState(widget?.appearance.greeting ?? "");
  const [secrets, setSecrets] = useState<APIWidgetSecret[]>([]);
  const [sessions, setSessions] = useState<APIWidgetSession[]>([]);
  const [securityRefresh, setSecurityRefresh] = useState(0);
  const [securityObservedAt, setSecurityObservedAt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    if (!widget) return;
    Promise.all([api.widgetSecrets(widget.id), api.widgetSessions(widget.id)])
      .then(([nextSecrets, nextSessions]) => { if (!cancelled) { setSecrets(nextSecrets); setSessions(nextSessions); setSecurityObservedAt(Date.now()); } })
      .catch(() => { if (!cancelled) { setSecrets([]); setSessions([]); } });
    return () => { cancelled = true; };
  }, [widget, securityRefresh]);
  if (!widget) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><h1>Widget unavailable</h1><p>This widget does not exist or is still loading.</p></div><ConsoleLink path={sectionPath("widgets")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to widgets</ConsoleLink></section>;
  const input: APIWidgetInput = {
    name: name.trim(),
    allowed_origins: origins.split(/[\n,]/).map((value) => value.trim()).filter(Boolean).sort(),
    integration_ids: [...integrationIDs].sort(),
    appearance: { theme, accent_colour: accent.trim() || undefined, launcher_position: widget.appearance.launcherPosition ?? "right", greeting: greeting.trim() || undefined },
  };
  const persistedInput: APIWidgetInput = {
    name: widget.name,
    allowed_origins: [...widget.allowed_origins].sort(),
    integration_ids: [...widget.integration_ids].sort(),
    appearance: { theme: widget.appearance.theme, accent_colour: widget.appearance.accentColour || undefined, launcher_position: widget.appearance.launcherPosition, greeting: widget.appearance.greeting || undefined },
  };
  const dirty = JSON.stringify(input) !== JSON.stringify(persistedInput);
  const activeSecrets = secrets.filter((secret) => !secret.revoked_at);
  const activeSessions = sessions.filter((session) => !session.revoked_at && new Date(session.expires_at).getTime() > securityObservedAt);
  const frontendSnippet = `import { mountWidget } from "@dokosoko/widget";\n\nmountWidget({\n  widgetId: "${widget.id}",\n  getToken: async () => {\n    const response = await fetch("/api/dokosoko/widget-token", {\n      method: "POST",\n      credentials: "same-origin",\n    });\n    if (!response.ok) throw new Error("Sign in required");\n    return response.json();\n  },\n});`;
  const backendSnippet = `import DokoSokoWidgetBackend from "@dokosoko/widget-backend";\n\nconst dokosoko = new DokoSokoWidgetBackend({\n  widgetSecret: process.env.DOKOSOKO_WIDGET_SECRET!,\n});\n\nexport async function POST(request: Request) {\n  const user = await requireAuthenticatedUser(request);\n  const token = await dokosoko.widgetSessions.create({\n    widgetId: "${widget.id}",\n    userId: user.id,\n    organizationId: user.organizationId,\n    origin: new URL(request.url).origin,\n  }, { idempotencyKey: crypto.randomUUID() });\n\n  return Response.json(token, {\n    headers: { "cache-control": "no-store" },\n  });\n}`;
  const save = () => onUpdate(widget, input);
  const activate = async () => {
    const saved = dirty ? await onUpdate(widget, input) : widget;
    if (saved) await onSetState(saved, "active");
  };
  const rotateSecret = async () => { await onRotateSecret(widget); setSecurityRefresh((value) => value + 1); };
  const revokeSecret = async (secret: APIWidgetSecret) => {
    try {
      const updated = await api.revokeWidgetSecret(widget.id, secret.id);
      setSecrets((values) => values.map((value) => value.id === updated.id ? updated : value));
      onMessage("Widget secret revoked.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Could not revoke the widget secret."); }
  };
  const revokeSession = async (session: APIWidgetSession) => {
    try {
      const updated = await api.revokeWidgetSession(widget.id, session.id);
      setSessions((values) => values.map((value) => value.id === updated.id ? updated : value));
      onMessage("Widget session revoked.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Could not revoke the widget session."); }
  };
  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("widgets")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Back to widgets</ConsoleLink><code>/widget/{widget.id}</code></div>
    <PageHeading eyebrow="Authenticated widget" title={widget.name} action={widget.state === "active" ? <Button outline disabled={busy} onClick={() => onSetState(widget, "disabled")}>Disable</Button> : !assistantAvailable ? <Button outline onClick={onConfigureAssistant}>Configure assistant</Button> : <Button color="indigo" disabled={busy || !input.name || input.allowed_origins.length === 0 || input.integration_ids.length === 0} onClick={activate}>{busy ? "Saving…" : dirty ? "Save and activate" : "Activate widget"}</Button>} />
    <div className="widget-status-line"><Badge color={widget.state === "active" ? "green" : widget.state === "disabled" ? "red" : "zinc"}>{widget.state}</Badge><code>{widget.id}</code><span>Revision {widget.revision}</span></div>
    <ol className="widget-setup-steps">
      <li className={input.allowed_origins.length ? "complete" : ""}><span>{input.allowed_origins.length ? <Check /> : "1"}</span><div><strong>Allow your application</strong><small>{input.allowed_origins.length ? `${input.allowed_origins.length} exact origin${input.allowed_origins.length === 1 ? "" : "s"}` : "Add the domains that may embed this widget."}</small></div></li>
      <li className="complete"><span><Check /></span><div><strong>Authenticate users</strong><small>A server-only widget secret was created. Use it only through the backend SDK.</small></div></li>
      <li className={input.integration_ids.length ? "complete" : ""}><span>{input.integration_ids.length ? <Check /> : "3"}</span><div><strong>Connect APIs</strong><small>{input.integration_ids.length ? `${input.integration_ids.length} API${input.integration_ids.length === 1 ? "" : "s"} allowed` : "No API access is granted by default."}</small></div></li>
      <li className={assistantAvailable ? "complete" : ""}><span>{assistantAvailable ? <Check /> : "4"}</span><div><strong>Connect assistant</strong><small>{assistantAvailable ? "The hardened assistant model is ready." : "Configure an assistant model in Settings."}</small></div></li>
      <li className={widget.state === "active" ? "complete" : ""}><span>{widget.state === "active" ? <Check /> : "5"}</span><div><strong>Go live</strong><small>{widget.state === "active" ? "New authenticated sessions are accepted." : "Activate after the installation is ready."}</small></div></li>
    </ol>
    <section className="panel widget-settings-panel"><PanelHeader title="Access and appearance" action={<Button color="indigo" disabled={busy || !dirty || !input.name || input.allowed_origins.length === 0 || (widget.state === "active" && input.integration_ids.length === 0)} onClick={save}>Save changes</Button>} /><div className="widget-settings-grid"><div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={name} maxLength={120} onChange={(event) => setName(event.target.value)} /></label><label className="auth-field"><span>Allowed origins</span><textarea value={origins} onChange={(event) => setOrigins(event.target.value)} /><small>Exact origins only; one per line.</small></label><fieldset className="widget-api-picker"><legend>Allowed APIs</legend>{integrations.filter((integration) => integration.lifecycle === "active").map((integration) => <label key={integration.id}><input aria-label={`Allow ${integration.display_name}`} type="checkbox" checked={integrationIDs.includes(integration.id)} onChange={(event) => setIntegrationIDs((values) => event.target.checked ? [...values, integration.id] : values.filter((id) => id !== integration.id))} /><span><strong>{integration.display_name}</strong><small>{integration.family_key} · {integration.version_key}</small></span></label>)}{integrations.filter((integration) => integration.lifecycle === "active").length === 0 && <p className="empty-picker">Publish an API before activating this widget.</p>}</fieldset></div><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Theme</span><select value={theme} onChange={(event) => setTheme(event.target.value as typeof theme)}><option value="auto">Automatic</option><option value="light">Light</option><option value="dark">Dark</option></select></label><label className="auth-field"><span>Accent</span><input value={accent} placeholder="#5b5cf0" onChange={(event) => setAccent(event.target.value)} /></label></div><label className="auth-field"><span>Greeting</span><input value={greeting} placeholder="How can I help?" maxLength={160} onChange={(event) => setGreeting(event.target.value)} /></label><div className="widget-live-preview" style={{ "--widget-accent": accent || "#5b5cf0" } as React.CSSProperties}><span>D</span><div><strong>{name || "Customer assistant"}</strong><small>{greeting || "How can I help?"}</small></div></div></div></div></section>
    <section className="panel widget-install-panel"><PanelHeader title="Install" /><div className="install-snippets"><article><div><strong>1. Browser</strong><CopyButton text={frontendSnippet} label="Copy browser code" onCopied={() => onMessage("Browser code copied.")} /></div><pre>{frontendSnippet}</pre></article><article><div><strong>2. Backend</strong><CopyButton text={backendSnippet} label="Copy backend code" onCopied={() => onMessage("Backend code copied.")} /></div><pre>{backendSnippet}</pre></article></div></section>
    <section className="panel widget-security-panel"><PanelHeader title="Security" action={<Button outline disabled={busy} onClick={rotateSecret}>Create new secret</Button>} /><div className="widget-security-grid"><article><div className="widget-security-title"><KeyRound /><span><strong>Backend secrets</strong><small>{activeSecrets.length} active</small></span></div><div className="widget-security-list">{secrets.map((secret) => <div key={secret.id}><span><code>••••{secret.fingerprint}</code><small>{secret.last_used_at ? `Last used ${new Date(secret.last_used_at).toLocaleString()}` : `Created ${new Date(secret.created_at).toLocaleString()}`}</small></span>{secret.revoked_at ? <Badge color="zinc">Revoked</Badge> : <Button outline disabled={busy || activeSecrets.length < 2} onClick={() => revokeSecret(secret)}>Revoke</Button>}</div>)}{secrets.length === 0 && <p className="empty-picker">Credential metadata is unavailable.</p>}</div></article><article><div className="widget-security-title"><ShieldCheck /><span><strong>Recent sessions</strong><small>{activeSessions.length} active</small></span></div><div className="widget-security-list">{sessions.slice(0, 8).map((session) => { const expired = new Date(session.expires_at).getTime() <= securityObservedAt; return <div key={session.id}><span><strong>{session.user_id}</strong><small>{widgetOriginLabel(session.origin)} · expires {new Date(session.expires_at).toLocaleString()}</small></span>{session.revoked_at || expired ? <Badge color="zinc">{session.revoked_at ? "Revoked" : "Expired"}</Badge> : <Button outline disabled={busy} onClick={() => revokeSession(session)}>Revoke</Button>}</div>; })}{sessions.length === 0 && <p className="empty-picker">No customer sessions yet.</p>}</div></article></div></section>
  </>;
}

function DistributionView({
  enabled,
  onEnabledChange,
  resources,
  resourceFilter,
  setResourceFilter,
  onVisibilityChange,
  onCopied,
  publicEndpoint,
  tenantName,
  publicAgentSetup,
  privateAgentSetup,
  onConfigureIdentity,
  onOpenSources,
  widgetCount,
  onOpenWidgets,
}: {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  resources: Array<{ id: string; name: string; resourceType: "source"; type: string; detail: string; visibility: Visibility }>;
  resourceFilter: "all" | "public" | "private";
  setResourceFilter: (filter: "all" | "public" | "private") => void;
  onVisibilityChange: (kind: "source", id: string) => void;
  onCopied: (label: string) => void;
  publicEndpoint: string;
  tenantName: string;
  publicAgentSetup: Distribution["agent_setup"]["public"];
  privateAgentSetup: Distribution["agent_setup"]["private"];
  onConfigureIdentity: () => void;
  onOpenSources: () => void;
  widgetCount: number;
  onOpenWidgets: () => void;
}) {
  return <>
    <PageHeading eyebrow="Delivery" title="Agent access" action={<Button outline disabled={!privateAgentSetup.available} onClick={() => window.open(privateAgentSetup.url, "_blank", "noopener,noreferrer")}><ExternalLink data-slot="icon" />Private MCP setup</Button>} />
    <section className={`public-mcp-card ${enabled ? "enabled" : ""}`}>
      <div className="public-mcp-copy"><div className="icon-tile"><Globe2 /></div><div><div className="title-row"><h2>Public MCP</h2><Badge color={enabled ? "green" : "zinc"}>{enabled ? "Live" : "Off"}</Badge></div><p>Offer an authentication-free, read-only MCP endpoint. Its server-side policy can retrieve only published sources that you explicitly mark public.</p><div className="endpoint"><code>{publicEndpoint}</code><button type="button" aria-label="Copy public MCP endpoint" onClick={() => { navigator.clipboard.writeText(publicEndpoint); onCopied("Public MCP endpoint copied."); }}><Copy />Copy</button></div></div></div>
      <div className="switch-stack"><Switch checked={enabled} onChange={onEnabledChange} label="Enable Public MCP" /><small>{enabled ? "Accepting anonymous requests" : "Disabled by default"}</small></div>
    </section>

    <section className="section-block agent-setup-section">
      <SectionHeader title="Copy MCP button" />
      <div className="agent-setup-grid">
        <AgentSetupCard kind="public" tenantName={tenantName} setup={publicAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
        <AgentSetupCard kind="private" tenantName={tenantName} setup={privateAgentSetup} onCopied={onCopied} onConfigureIdentity={onConfigureIdentity} />
      </div>
    </section>

    <section className="section-block">
      <SectionHeader title="Resource visibility" action={<Button outline onClick={onOpenSources}>Manage sources</Button>} />
      <SegmentedControl label="Filter resources" items={[{ id: "all", label: "All" }, { id: "public", label: "Public" }, { id: "private", label: "Private" }]} value={resourceFilter} onChange={setResourceFilter} />
      <DataTable label="Resource visibility">
        <DataTableHeader className="resource-columns"><span>Resource</span><span>Type</span><span>Visibility</span><span>Actions</span></DataTableHeader>
        {resources.map((resource) => <DataTableRow className="resource-columns" key={`${resource.resourceType}-${resource.id}`}>
          <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><strong>{resource.name}</strong><small>{resource.detail}</small></span></span>
          <span>{resource.type}</span>
          <span className="visibility-control"><Badge color={resource.visibility === "public" ? "green" : "zinc"}>{resource.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{resource.visibility[0].toUpperCase() + resource.visibility.slice(1)}</Badge><Switch checked={resource.visibility === "public"} onChange={() => onVisibilityChange(resource.resourceType, resource.id)} label={`Make ${resource.name} ${resource.visibility === "public" ? "private" : "public"}`} /></span>
          <span className="table-actions"><button type="button" className="more" aria-label={`Actions for ${resource.name}`}><MoreHorizontal /></button></span>
        </DataTableRow>)}
        {resources.length === 0 && <DataTableEmpty columns={4}>No resources match this filter.</DataTableEmpty>}
      </DataTable>
    </section>

    <section className="section-block widget-channel-card"><span className="icon-tile"><MessageSquareText /></span><div><h2>Embedded widgets</h2><p>Authenticated assistants for customer applications. Each widget has its own origins, server secret, and API allow-list.</p></div><Badge color={widgetCount > 0 ? "violet" : "zinc"}>{widgetCount}</Badge><Button outline onClick={onOpenWidgets}>{widgetCount > 0 ? "Manage widgets" : "Create widget"}<ChevronRight data-slot="icon" /></Button></section>
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
      {setup.available ? <a className="agent-setup-guide-link" href={setup.url} target="_blank" rel="noopener noreferrer"><ExternalLink />Open setup instructions</a> : <div className="inline-warning"><TriangleAlert />{isPublic ? "Enable Public MCP before distributing this button." : "Configure and activate customer identity before distributing this button."}</div>}
      {!isPublic && !setup.available && <Button outline className="agent-identity-action" onClick={onConfigureIdentity}>Configure identity</Button>}
      <CopyButton text={setup.embed_html} label={`Copy ${kind} MCP button`} disabled={!setup.available} onCopied={() => onCopied(`${isPublic ? "Public" : "Private"} MCP button copied.`)} />
    </div>
  </article>;
}

function SourcesView({ sources, onAdd, onCrawl, onPublish, onVisibilityChange, onNavigate }: { sources: Source[]; onAdd: () => void; onCrawl: (id: string) => void; onPublish: (source: Source) => void; onVisibilityChange: (id: string) => void; onNavigate: (path: string) => void }) {
  return <>
    <PageHeading eyebrow="Knowledge" title="Sources" action={<Button onClick={onAdd}><Plus data-slot="icon" />Add source</Button>} />
    <div className="summary-strip"><SummaryItem label="Pages indexed" value="378" icon={<Database />} /><SummaryItem label="Healthy sources" value="1 of 3" icon={<CheckCircle2 />} /><SummaryItem label="Needs attention" value="2" icon={<AlertCircle />} /></div>
    <div className="toolbar"><div className="search-field"><Search /><input aria-label="Search sources" placeholder="Search sources…" /></div><Button outline onClick={() => sources.forEach((source) => onCrawl(source.id))}><RefreshCw data-slot="icon" />Crawl all</Button></div>
    <DataTable label="Sources">
      <DataTableHeader className="source-columns"><span>Source</span><span>Crawl state</span><span>Content</span><span>Visibility</span><span>Actions</span></DataTableHeader>
      {sources.map((source) => <DataTableRow className="source-columns" key={source.id}>
        <span className="resource-name"><span className="resource-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.location} · {source.kind}</small></span></span>
        <span><CrawlBadge state={source.crawlState} /><small className="cell-note">{source.lastCrawl}</small></span>
        <span><strong className="cell-value">{source.pages}</strong><small className="cell-note">pages</small></span>
        <span className="visibility-control"><Badge color={source.visibility === "public" ? "green" : "zinc"}>{source.visibility === "public" ? <Globe2 /> : <LockKeyhole />}{source.visibility}</Badge><Switch checked={source.visibility === "public"} onChange={() => onVisibilityChange(source.id)} label={`Make ${source.name} ${source.visibility === "public" ? "private" : "public"}`} /></span>
        <span className="table-actions">{source.crawlState === "review" && <Button outline onClick={() => onPublish(source)}>{source.quarantined ? "Inspect" : "Review"}</Button>}<button type="button" className="more" aria-label={`Crawl ${source.name}`} title="Queue crawl" onClick={() => onCrawl(source.id)}><RefreshCw /></button></span>
      </DataTableRow>)}
    </DataTable>
  </>;
}

function CrawlBadge({ state }: { state: Source["crawlState"] }) {
  if (state === "queued" || state === "running") return <Badge color="blue"><RefreshCw />{state}</Badge>;
  if (state === "synced") return <Badge color="green"><CheckCircle2 />Synced</Badge>;
  if (state === "review") return <Badge color="amber"><Clock3 />Needs review</Badge>;
  if (state === "draft") return <Badge color="zinc"><Clock3 />Not crawled</Badge>;
  if (state === "cancelled") return <Badge color="zinc"><XCircle />Cancelled</Badge>;
  return <Badge color="red"><XCircle />Failed</Badge>;
}

function IntegrationDirectoryView({ integrations, connections, supportRoutes, query, onQueryChange, onCreate, onBuild, onNavigate }: { integrations: APIIntegration[]; connections: APIAccessConnection[]; supportRoutes: APISupportRoute[]; query: string; onQueryChange: (query: string) => void; onCreate: () => void; onBuild: () => void; onNavigate: (path: string) => void }) {
  const [showRetired, setShowRetired] = useState(false);
  const normalizedQuery = query.trim().toLowerCase();
  const retiredCount = integrations.filter((integration) => integration.lifecycle === "retired").length;
  const connectionCount = (integration: APIIntegration) => connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id)).length;
  const hasSupport = (integration: APIIntegration) => Boolean(supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default));
  const setupIssueCount = (integration: APIIntegration) => Number((integration.resources?.length ?? 0) === 0) + Number(connectionCount(integration) === 0) + Number(!hasSupport(integration));
  const filteredIntegrations = integrations
    .filter((integration) => showRetired || integration.lifecycle !== "retired")
    .filter((integration) => !normalizedQuery || [integration.display_name, integration.family_key, integration.version_key, integration.description].some((value) => value.toLowerCase().includes(normalizedQuery)))
    .sort((left, right) => left.display_name.localeCompare(right.display_name) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true }));

  return <>
    <PageHeading eyebrow="Catalog" title="APIs" action={<span className="heading-actions"><Button outline onClick={onCreate}><Plus data-slot="icon" />Add API</Button><Button onClick={onBuild}><Sparkles data-slot="icon" />Import APIs</Button></span>} />
    <div className="toolbar integration-toolbar">
      <div className="search-field"><Search /><input aria-label="Search APIs" placeholder="Search APIs…" value={query} onChange={(event) => onQueryChange(event.target.value)} /></div>
      <span className="toolbar-count">{filteredIntegrations.length} API{filteredIntegrations.length === 1 ? "" : "s"}</span>
    </div>
    <div className="integration-directory-wrap">
      <DataTable label="APIs" className="integration-directory">
        <DataTableHeader className="integration-directory-columns"><span>API</span><span>Lifecycle</span><span>Setup</span><span>Resources</span><span>Open</span></DataTableHeader>
        {filteredIntegrations.map((integration) => { const issues = setupIssueCount(integration); return <DataTableRow key={integration.id} className="integration-directory-columns integration-directory-row">
          <span className="resource-name"><span className="resource-icon"><GitBranch /></span><span><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="entity-link"><strong>{integration.display_name}</strong></ConsoleLink><small>{integration.version_key}</small></span></span>
          <span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge> <Badge color={integration.visibility === "public" ? "blue" : "zinc"}>{integration.visibility}</Badge></span>
          <span><Badge color={issues === 0 ? "green" : "amber"}>{issues === 0 ? "Ready" : `${issues} step${issues === 1 ? "" : "s"} left`}</Badge></span>
          <span><strong className="cell-value">{integration.resources?.length ?? 0}</strong><small className="cell-note">attached sets</small></span>
          <span className="table-open-cell"><ConsoleLink path={integrationPath(integration.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${integration.display_name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>; })}
        {filteredIntegrations.length === 0 && <DataTableEmpty columns={5}>{integrations.length === 0 ? "No APIs yet. Add one manually or import your existing API sources." : retiredCount === integrations.length && !showRetired && !normalizedQuery ? "No current APIs. Show retired APIs to view the archive." : "No APIs match this search."}</DataTableEmpty>}
      </DataTable>
      {retiredCount > 0 && <button type="button" className="retired-directory-toggle" aria-pressed={showRetired} onClick={() => setShowRetired((visible) => !visible)}>{showRetired ? "Hide retired" : `Show retired (${retiredCount})`}</button>}
    </div>
  </>;
}

function IntegrationSwitcher({ integrations, integration, activeTab, activeResourceTab, onNavigate }: { integrations: APIIntegration[]; integration: APIIntegration; activeTab: IntegrationTab; activeResourceTab?: IntegrationResourceTab; onNavigate: (path: string) => void }) {
  const optionLabel = (value: APIIntegration) => `${value.display_name} · ${value.version_key} · ${value.family_key}`;
  const [value, setValue] = useState(optionLabel(integration));

  function selectIntegration(nextValue: string) {
    setValue(nextValue);
    const selected = integrations.find((candidate) => candidate.id === nextValue || optionLabel(candidate) === nextValue);
    if (selected && selected.id !== integration.id) onNavigate(integrationPath(selected.id, activeTab, activeResourceTab));
  }

  if (integrations.length <= 1) return null;
  return <div className="integration-workspace-switcher"><label htmlFor="integration-switcher">Switch API</label><div className="integration-switcher-input"><Search /><input id="integration-switcher" list="integration-switcher-options" value={value} onChange={(event) => selectIntegration(event.target.value)} onBlur={() => { if (!integrations.some((candidate) => optionLabel(candidate) === value)) setValue(optionLabel(integration)); }} /><datalist id="integration-switcher-options">{[...integrations].sort((left, right) => left.family_key.localeCompare(right.family_key) || left.version_key.localeCompare(right.version_key, undefined, { numeric: true })).map((candidate) => <option key={candidate.id} value={optionLabel(candidate)} />)}</datalist></div></div>;
}

type IntegrationWorkspaceViewProps = {
  integration: APIIntegration | null;
  integrations: APIIntegration[];
  activeTab: IntegrationTab;
  activeResourceTab: IntegrationResourceTab;
  loading: boolean;
  revisions: APIIntegrationRevision[];
  publishStatus: APIIntegrationPublishStatus | null;
  identity: APIIdentity | null;
  resourceSets: APIResourceSet[];
  sources: Source[];
  connections: APIAccessConnection[];
  supportRoutes: APISupportRoute[];
  tools: APITool[];
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  distribution: Distribution | null;
  widgets: APIWidget[];
  recipeBusy: boolean;
  busy: boolean;
  onEdit: (integration: APIIntegration) => void;
  onPublish: (integration: APIIntegration) => void;
  onAttach: (integration: APIIntegration, kind?: APIResourceSet["kind"]) => void;
  onCreateResource: () => void;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source) => void;
  onGenerateRecipes: (integrationID?: string) => void;
  onEditResource: (resource: APIResourceSet) => void;
  onDuplicateResource: (resource: APIResourceSet) => void;
  onDetachResource: (integrationID: string, resourceSetID: string) => void;
  onManageAccess: (integration: APIIntegration) => void;
  onManageSupport: (integration: APIIntegration) => void;
  onInspectRevision: (revision: APIIntegrationRevision) => void;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

function IntegrationWorkspaceView({ integration, integrations, activeTab, activeResourceTab, loading, revisions, publishStatus, identity, resourceSets, sources, connections, supportRoutes, tools, analyses, recipes, distribution, widgets, recipeBusy, busy, onEdit, onPublish, onAttach, onCreateResource, onAddSource, onCrawlSource, onPublishSource, onGenerateRecipes, onEditResource, onDuplicateResource, onDetachResource, onManageAccess, onManageSupport, onInspectRevision, onMessage, onNavigate }: IntegrationWorkspaceViewProps) {
  if (loading && !integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><RefreshCw /></span><div><p className="eyebrow">API</p><h1>Loading API…</h1><p>Retrieving its configuration and published history.</p></div></section>;
  if (!integration) return <section className="panel entity-missing"><span className="entity-missing-icon"><Search /></span><div><p className="eyebrow">API</p><h1>API unavailable</h1><p>This API is not available in the current deployment.</p></div><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Return to APIs</ConsoleLink></section>;

  const integrationConnections = connections.filter((connection) => connection.integration_ids?.includes(integration.id) || integration.access_connection_ids?.includes(connection.id));
  const supportRoute = supportRoutes.find((route) => route.id === integration.support_route_id || route.integration_ids?.includes(integration.id)) ?? supportRoutes.find((route) => route.is_default);
  const attachedResources = integration.resources ?? [];
  const documentationResources = attachedResources.filter((resource) => resource.kind === "documentation");
  const contractResources = attachedResources.filter((resource) => resource.kind === "api");
  const sortedRevisions = [...revisions].sort((left, right) => right.revision - left.revision);
  const relevantWidgets = widgets.filter((widget) => widget.integration_ids.includes(integration.id));
  const integrationAnalyses = analyses.filter((analysis) => analysisMatchesIntegration(analysis, integration.id));
  const integrationRecipes = recipes.filter((recipe) => recipeMatchesIntegration(recipe, integration.id));
  const publishValidationCodes = new Set(publishStatus?.validations.map((validation) => validation.code) ?? []);
  const setupSteps: Array<{ label: string; detail: string; ready: boolean; tab: IntegrationTab; resourceTab?: IntegrationResourceTab }> = [
    { label: "Documentation", detail: "Ingest, review, publish and attach trusted documentation.", ready: documentationResources.length > 0 && sources.some((source) => source.published && !source.quarantined), tab: "resources", resourceTab: "documentation" },
    { label: "API contracts", detail: "Attach at least one immutable API contract revision.", ready: contractResources.length > 0, tab: "resources", resourceTab: "contracts" },
    { label: "Authorization", detail: "Activate a declarative point backed by registered grants.", ready: Boolean(publishStatus && !publishValidationCodes.has("authorization_missing")), tab: "authorization" },
    { label: "Tools", detail: "Bind at least one exact reviewed tool revision to this API.", ready: Boolean(publishStatus && !publishValidationCodes.has("tools_missing")), tab: "tools" },
    { label: "Recipes", detail: "Publish at least one evidence-grounded developer recipe.", ready: integrationRecipes.some((recipe) => recipe.state === "published"), tab: "recipes" },
    { label: "Delivery", detail: "Review MCP, widget and support delivery channels.", ready: Boolean(distribution?.private_mcp_endpoint && supportRoute), tab: "delivery" },
    { label: "Acceptance", detail: "Run the vendor-neutral setup preflight before publication.", ready: Boolean(publishStatus?.ready), tab: "test" },
  ];
  const setupValidationCodes = new Set(["resources_missing", "authorization_missing", "tools_missing", "access_missing", "support_inherited"]);
  const actionableValidations = publishStatus?.validations.filter((validation) => !setupValidationCodes.has(validation.code)) ?? [];
  const hasChanges = Boolean(publishStatus?.has_changes);
  const canPublish = Boolean(publishStatus?.ready && hasChanges);
  const resourceLabel = (kind: APIResourceSet["kind"]) => kind === "api" ? "API contract" : "documentation";
  const setupComplete = setupSteps.filter((step) => step.ready).length;

  const renderResourceRows = (resources: typeof attachedResources) => <>
    {resources.map((resource) => {
      const source = resourceSets.find((set) => set.id === resource.resource_set_id);
      return <div className="integration-resource-row" key={resource.resource_set_id}><span className="settings-icon">{resource.kind === "documentation" ? <BookOpen /> : <TerminalSquare />}</span><span><EntityLink entity="resource-set" uid={resource.resource_set_id} onNavigate={onNavigate} className="entity-link"><strong>{resource.name}</strong></EntityLink><small>{resourceLabel(resource.kind)} · {resource.follow_latest ? "follows latest" : `pinned to revision ${resource.resolved_revision?.revision ?? "—"}`}</small></span><Badge color={resource.kind === "documentation" ? "blue" : "violet"}>{resourceLabel(resource.kind)}</Badge><span className="table-actions">{source && <Button outline onClick={() => onEditResource(source)}>New revision</Button>}{source && <Button outline onClick={() => onDuplicateResource(source)}>Duplicate</Button>}<button type="button" className="more" disabled={busy} title={`Detach ${resource.name}`} aria-label={`Detach ${resource.name}`} onClick={() => onDetachResource(integration.id, resource.resource_set_id)}><XCircle /></button></span></div>;
    })}
    {resources.length === 0 && <div className="empty-row">Nothing is attached here yet.</div>}
  </>;

  return <>
    <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("product")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />All APIs</ConsoleLink></div>
    <IntegrationSwitcher key={integration.id} integrations={integrations} integration={integration} activeTab={activeTab} activeResourceTab={activeResourceTab} onNavigate={onNavigate} />
    <PageHeading eyebrow={`${integration.family_key} · ${integration.version_key}`} title={integration.display_name} action={<span className="heading-actions"><Button outline onClick={() => onEdit(integration)}>Edit</Button>{!publishStatus ? <span className="published-state checking"><RefreshCw />Checking…</span> : canPublish ? <Button color="indigo" disabled={busy} onClick={() => onPublish(integration)}><GitBranch data-slot="icon" />Publish</Button> : hasChanges && !publishStatus.ready ? <Badge color="amber">Setup required</Badge> : <span className="published-state"><CheckCircle2 />Published</span>}</span>} />
    <PageTabs label={`${integration.display_name} sections`}>{INTEGRATION_TABS.map((tab) => <ConsoleLink key={tab.id} path={integrationPath(integration.id, tab.id)} onNavigate={onNavigate} className={`page-tab ${activeTab === tab.id ? "active" : ""}`} ariaCurrent={activeTab === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</PageTabs>

    {activeTab === "overview" && <div className="integration-tab-content">
      <div className="api-status-bar"><span><span className={`status-dot${publishStatus ? "" : " checking"}`} /><strong>{!publishStatus ? "Checking status" : publishStatus.ready ? hasChanges ? "Ready to publish" : "Published" : "Needs setup"}</strong><small>{!publishStatus ? "Loading the latest publication state…" : `${setupComplete}/${setupSteps.length} onboarding gates complete${hasChanges ? ` · ${publishStatus.changes.length} unpublished change${publishStatus.changes.length === 1 ? "" : "s"}` : ""}`}</small></span><Badge color={integration.lifecycle === "active" ? "green" : integration.lifecycle === "deprecated" ? "amber" : "zinc"}>{integration.lifecycle}</Badge></div>
      <section className="panel onboarding-checklist"><PanelHeader title="Standard setup" description="Complete each vendor-neutral gate, then publish one immutable API snapshot." action={<Badge color={setupComplete === setupSteps.length ? "green" : "violet"}>{setupComplete}/{setupSteps.length}</Badge>} />{setupSteps.map((step, index) => <ConsoleLink key={step.label} path={integrationPath(integration.id, step.tab, step.resourceTab)} onNavigate={onNavigate} className="integration-health-check"><span className={`health-icon ${step.ready ? "ready" : ""}`}>{step.ready ? <CheckCircle2 /> : <span className="step-number">{index + 1}</span>}</span><span><strong>{step.label}</strong><small>{step.detail}</small></span><Badge color={step.ready ? "green" : "zinc"}>{step.ready ? "Ready" : "Setup"}</Badge><ChevronRight /></ConsoleLink>)}{actionableValidations.map((validation) => <ConsoleLink key={validation.code} path={integrationPath(integration.id, validation.tab === "resources" ? "resources" : validation.tab === "access" || validation.tab === "authorization" ? "authorization" : "overview")} onNavigate={onNavigate} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level === "error" ? "Resolve before publishing" : "Review before publishing"}</strong><small>{validation.message}</small></span><ChevronRight /></ConsoleLink>)}</section>
      <section className="panel"><PanelHeader title="Bug reports & feedback" action={<Button outline onClick={() => onManageSupport(integration)}>{supportRoute ? "Change" : "Configure"}</Button>} />{supportRoute ? <div className="support-route-summary"><span className="settings-icon"><Bug /></span><span><strong>{supportRoute.name}</strong><small>{supportRoute.is_default ? "Uses the deployment default" : "Configured for this API"} · {supportRoute.retention_days}-day encrypted retention</small></span><Badge color={supportRoute.state === "active" ? "green" : "zinc"}>{supportRoute.bug_reports_enabled || supportRoute.feedback_enabled ? "Available" : "Off"}</Badge></div> : <div className="empty-row">Not configured. Reports remain unavailable until you choose a policy.</div>}</section>
      <div className="integration-overview-grid"><ConsoleLink path={integrationPath(integration.id, "resources")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><BookOpen /></span><span><strong>Resources</strong><small>Documentation, contracts, SDKs and packages.</small></span><ChevronRight /></ConsoleLink><ConsoleLink path={integrationPath(integration.id, "authorization")} onNavigate={onNavigate} className="integration-shortcut"><span className="settings-icon"><ShieldCheck /></span><span><strong>Authorization</strong><small>{identity?.state === "active" ? "Delegated identity active" : identity ? "Identity disabled" : "Identity not configured"}</small></span><ChevronRight /></ConsoleLink></div>
      <details className="panel advanced-details"><summary>Advanced details</summary><dl className="entity-detail-grid"><div><dt>API ID</dt><dd>{integration.id}</dd></div><div><dt>Family</dt><dd>{integration.family_key}</dd></div><div><dt>Version</dt><dd>{integration.version_key}</dd></div><div><dt>Draft revision</dt><dd>{integration.revision}</dd></div><div><dt>Replacement</dt><dd>{integration.replacement_integration_id ?? "—"}</dd></div><div><dt>Sunset</dt><dd>{integration.sunset_at ? new Date(integration.sunset_at).toLocaleDateString() : "—"}</dd></div></dl></details>
    </div>}

    {activeTab === "resources" && <div className="integration-tab-content">
      <PageTabs label="Resource types">{INTEGRATION_RESOURCE_TABS.map((tab) => <ConsoleLink key={tab.id} path={integrationPath(integration.id, "resources", tab.id)} onNavigate={onNavigate} className={`page-tab resource-subtab ${activeResourceTab === tab.id ? "active" : ""}`} ariaCurrent={activeResourceTab === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</PageTabs>
      {activeResourceTab === "documentation" && <>
        <section className="panel"><PanelHeader title="Documentation ingestion" description="Deployment sources are reusable; attach a reviewed resource revision to this API." action={<span className="heading-actions"><ConsoleLink path={sectionPath("sources")} onNavigate={onNavigate} className="entity-back-link">All documentation</ConsoleLink><Button onClick={onAddSource}><Plus data-slot="icon" />Add source</Button></span>} />{sources.map((source) => <div className="provider-row documentation-source-row" key={source.id}><span className="settings-icon"><BookOpen /></span><span><EntityLink entity="source" uid={source.id} onNavigate={onNavigate} className="entity-link"><strong>{source.name}</strong></EntityLink><small>{source.kind} · {source.location}</small></span><span className="tool-badges"><Badge color={source.quarantined || source.crawlState === "failed" ? "red" : source.crawlState === "synced" ? "green" : "amber"}>{source.quarantined ? "quarantined" : source.crawlState === "synced" ? `published r${source.latestPublication?.revision ?? 1}` : source.crawlState}</Badge><Badge color={source.visibility === "public" ? "blue" : "zinc"}>{source.visibility}</Badge></span><span className="table-actions">{source.crawlState !== "queued" && source.crawlState !== "running" && <Button outline onClick={() => onCrawlSource(source.id)}>Crawl</Button>}{source.crawlState === "review" && <Button onClick={() => onPublishSource(source)}>{source.quarantined ? "Inspect" : "Review"}</Button>}</span></div>)}{sources.length === 0 && <div className="empty-row">No documentation source has been ingested.</div>}</section>
        <section className="panel"><PanelHeader title="Attached documentation" action={<span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create set</Button><Button onClick={() => onAttach(integration, "documentation")}>Attach reviewed set</Button></span>} />{renderResourceRows(documentationResources)}</section>
      </>}
      {activeResourceTab === "contracts" && <>
        <div className="notice"><TerminalSquare /><span><strong>Immutable API contracts.</strong> Review each manifest, then pin or follow a reusable contract revision.</span></div>
        <section className="panel"><PanelHeader title="Attached API contracts" action={<span className="heading-actions"><Button outline onClick={onCreateResource}><Plus data-slot="icon" />Create contract set</Button><Button onClick={() => onAttach(integration, "api")}>Attach existing</Button></span>} />{renderResourceRows(contractResources)}</section>
      </>}
      {activeResourceTab === "packages" && <IntegrationPackagesWorkspace integration={integration} onMessage={onMessage} />}
    </div>}

    {activeTab === "authorization" && <IntegrationAuthorizationWorkspace integration={integration} identity={identity} connections={integrationConnections} tools={tools} onManageAccess={() => onManageAccess(integration)} onMessage={onMessage} onNavigate={onNavigate} />}

    {activeTab === "tools" && <IntegrationToolsWorkspace key={integration.id} integration={integration} tools={tools} onMessage={onMessage} onNavigate={onNavigate} />}

    {activeTab === "recipes" && <IntegrationRecipesWorkspace analyses={integrationAnalyses} recipes={integrationRecipes} busy={recipeBusy} onGenerate={() => onGenerateRecipes(integration.id)} onNavigate={onNavigate} />}

    {activeTab === "delivery" && <IntegrationDeliveryWorkspace integration={integration} identity={identity} distribution={distribution} supportRoute={supportRoute} widgets={relevantWidgets} onManageSupport={() => onManageSupport(integration)} onNavigate={onNavigate} />}

    {activeTab === "test" && <IntegrationTestWorkspace key={`${integration.id}:${publishStatus?.current_manifest_hash ?? ""}`} integration={integration} distribution={distribution} onNavigate={onNavigate} />}

    {activeTab === "history" && <div className="integration-tab-content"><div className="notice"><GitBranch /><span><strong>Published history is immutable.</strong> Each entry preserves the exact resources, access, and reporting policy used by agents.</span></div><section className="panel"><PanelHeader title="Published history" />{sortedRevisions.map((revision) => <button type="button" className="integration-revision-row" key={revision.id} onClick={() => onInspectRevision(revision)}><span className="revision-number">r{revision.revision}</span><span><strong>{revision.state}</strong><small>{revision.published_at || revision.created_at ? new Date(revision.published_at ?? revision.created_at).toLocaleString() : "Date unavailable"}</small></span><ChevronRight /></button>)}{sortedRevisions.length === 0 && <div className="empty-row">Nothing has been published yet.</div>}</section></div>}
  </>;
}

function unavailableConsoleCapability(error: unknown) {
  return error instanceof APIError && [404, 405, 501].includes(error.status);
}

const packageEcosystemPattern = /^[a-z][a-z0-9._-]{0,63}$/;

function packageSunsetPassed(value?: string) {
  if (!value) return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && timestamp <= Date.now();
}

function packageArtifactCanPublish(artifact: APIPackageArtifact) {
  return (artifact.lifecycle === "draft" || artifact.lifecycle === "active") && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanBind(artifact: APIPackageArtifact) {
  return artifact.lifecycle === "active" && !packageSunsetPassed(artifact.sunset_at);
}

function packageArtifactCanPublishForIntegration(artifact: APIPackageArtifact, integration: APIIntegration) {
  return packageArtifactCanPublish(artifact) && (integration.visibility !== "public" || artifact.visibility === "public");
}

function packageLifecycleDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function IntegrationPackagesWorkspace({ integration, onMessage }: { integration: APIIntegration; onMessage: (message: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationPackageBinding[]>([]);
  const [catalog, setCatalog] = useState<APIPackageArtifact[]>([]);
  const [loading, setLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editingArtifact, setEditingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecateOpen, setDeprecateOpen] = useState(false);
  const [deprecatingArtifact, setDeprecatingArtifact] = useState<APIPackageArtifact | null>(null);
  const [deprecationMessage, setDeprecationMessage] = useState("");
  const [replacementArtifactID, setReplacementArtifactID] = useState("");
  const [sunsetAt, setSunsetAt] = useState("");
  const [retireOpen, setRetireOpen] = useState(false);
  const [retiringArtifact, setRetiringArtifact] = useState<APIPackageArtifact | null>(null);
  const [retirementMessage, setRetirementMessage] = useState("");
  const [retirementReplacementID, setRetirementReplacementID] = useState("");
  const [publishReleaseOpen, setPublishReleaseOpen] = useState(false);
  const [publishingArtifact, setPublishingArtifact] = useState<APIPackageArtifact | null>(null);
  const [releasePublicAcknowledged, setReleasePublicAcknowledged] = useState(false);
  const [bindOpen, setBindOpen] = useState(false);
  const [selectedReleaseID, setSelectedReleaseID] = useState("");
  const [ecosystem, setEcosystem] = useState("npm");
  const [packageName, setPackageName] = useState("");
  const [packageDescription, setPackageDescription] = useState("");
  const [packageCoordinate, setPackageCoordinate] = useState("");
  const [artifactPURL, setArtifactPURL] = useState("");
  const [releasePURL, setReleasePURL] = useState("");
  const [registryURL, setRegistryURL] = useState("");
  const [sourceURL, setSourceURL] = useState("");
  const [packageLanguage, setPackageLanguage] = useState("");
  const [packagePlatform, setPackagePlatform] = useState("");
  const [packageVisibility, setPackageVisibility] = useState<APIVisibility>("private");
  const [packagePublicAcknowledged, setPackagePublicAcknowledged] = useState(false);
  const [packageVersion, setPackageVersion] = useState("");
  const [installCommand, setInstallCommand] = useState("");
  const [integrityDigest, setIntegrityDigest] = useState("");
  const [sbomURL, setSBOMURL] = useState("");
  const [provenanceURL, setProvenanceURL] = useState("");
  const ecosystemOptionsID = `package-ecosystems-${integration.id}`;

  const loadPackages = useCallback(async () => {
    setLoading(true);
    const [bindingResult, catalogResult] = await Promise.allSettled([api.integrationPackages(integration.id), api.packageArtifacts()]);
    if (bindingResult.status === "fulfilled") {
      setBindings(bindingResult.value);
      setBindingsUnavailable(false);
    } else {
      setBindings([]);
      setBindingsUnavailable(unavailableConsoleCapability(bindingResult.reason));
    }
    if (catalogResult.status === "fulfilled") {
      const enriched = await Promise.all(catalogResult.value.map(async (artifact) => {
        try {
          const releases = await api.packageReleases(artifact.id);
          return { ...artifact, releases };
        } catch {
          return artifact;
        }
      }));
      setCatalog(enriched);
    } else setCatalog([]);
    setLoading(false);
  }, [integration.id]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadPackages(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadPackages]);

  const publishedReleases = catalog.flatMap((artifact) => {
    if (!packageArtifactCanBind(artifact)) return [];
    return (artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []))
      .filter((release) => integration.visibility !== "public" || release.visibility === "public")
      .map((release) => ({ artifact, release }));
  });
  const replacementCandidates = catalog.filter((artifact) => packageArtifactCanBind(artifact) && (artifact.latest_release || (artifact.releases?.length ?? 0) > 0));
  const ecosystemValid = packageEcosystemPattern.test(ecosystem.trim());

  function resetPackageMetadata() {
    setEcosystem("npm"); setPackageName(""); setPackageDescription(""); setPackageCoordinate(""); setArtifactPURL(""); setRegistryURL(""); setSourceURL(""); setPackageLanguage(""); setPackagePlatform(""); setPackageVisibility("private"); setPackagePublicAcknowledged(false);
  }

  function resetReleaseMetadata() {
    setPackageVersion(""); setReleasePURL(""); setInstallCommand(""); setIntegrityDigest(""); setSBOMURL(""); setProvenanceURL(""); setReleasePublicAcknowledged(false);
  }

  function packageArtifactInput() {
    return { ecosystem: ecosystem.trim().toLowerCase(), name: packageName.trim(), description: packageDescription.trim(), coordinate: packageCoordinate.trim(), purl: artifactPURL.trim(), registry_url: registryURL.trim(), source_url: sourceURL.trim() || undefined, language: packageLanguage.trim() || undefined, platform: packagePlatform.trim() || undefined, visibility: packageVisibility, acknowledge_public: packagePublicAcknowledged };
  }

  function openCreatePackage() {
    resetPackageMetadata();
    setPackageVisibility(integration.visibility);
    resetReleaseMetadata();
    setCreateOpen(true);
  }

  function openEditPackage(artifact: APIPackageArtifact) {
    setEditingArtifact(artifact);
    setEcosystem(artifact.ecosystem); setPackageName(artifact.name); setPackageDescription(artifact.description); setPackageCoordinate(artifact.coordinate); setArtifactPURL(artifact.purl); setRegistryURL(artifact.registry_url); setSourceURL(artifact.source_url ?? ""); setPackageLanguage(artifact.language ?? ""); setPackagePlatform(artifact.platform ?? ""); setPackageVisibility(artifact.visibility); setPackagePublicAcknowledged(false);
    setEditOpen(true);
  }

  function openPublishRelease(artifact: APIPackageArtifact) {
    resetReleaseMetadata();
    setPublishingArtifact(artifact);
    setPublishReleaseOpen(true);
  }

  function openDeprecatePackage(artifact: APIPackageArtifact) {
    setDeprecatingArtifact(artifact); setDeprecationMessage(""); setReplacementArtifactID(""); setSunsetAt(""); setDeprecateOpen(true);
  }

  function openRetirePackage(artifact: APIPackageArtifact) {
    setRetiringArtifact(artifact); setRetirementMessage(artifact.deprecation_message ?? ""); setRetirementReplacementID(artifact.replacement_package_artifact_id ?? ""); setRetireOpen(true);
  }

  async function bindSelectedRelease() {
    if (!selectedReleaseID) return;
    if (!publishedReleases.some(({ release }) => release.id === selectedReleaseID)) {
      onMessage("That release is no longer eligible to bind to this Integration.");
      return;
    }
    setBusy(true);
    try {
      await api.bindIntegrationPackage(integration.id, selectedReleaseID);
      await loadPackages();
      setBindOpen(false);
      setSelectedReleaseID("");
      onMessage("Exact package release bound to this API.");
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package bindings are not available in this deployment yet." : error instanceof APIError ? error.message : "Package release could not be bound.");
    } finally { setBusy(false); }
  }

  function packageFailureMessage(error: unknown, fallback: string) {
    if (unavailableConsoleCapability(error)) return "The SDK and package catalogue is not available in this deployment yet.";
    return error instanceof APIError ? error.message : fallback;
  }

  async function recoverPackageWorkflow(knownArtifact: APIPackageArtifact | null, knownRelease: APIPackageRelease | null, failure: string) {
    let artifact = knownArtifact;
    let release = knownRelease;
    try {
      const artifacts = await api.packageArtifacts();
      artifact = (knownArtifact ? artifacts.find((candidate) => candidate.id === knownArtifact.id) : undefined)
        ?? artifacts.find((candidate) => candidate.ecosystem === ecosystem.trim().toLowerCase() && candidate.coordinate === packageCoordinate.trim())
        ?? artifact;
      if (artifact) {
        const releases = await api.packageReleases(artifact.id);
        artifact = { ...artifact, releases };
        release = release ?? releases.find((candidate) => candidate.version === packageVersion.trim() && candidate.purl === releasePURL.trim()) ?? null;
      }
    } catch {
      // A best-effort refresh must not hide the original create, publish, or bind error.
    }
    await loadPackages();
    if (release && integration.visibility === "public" && release.visibility !== "public") {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID("");
      onMessage(`${failure} The private release was saved, but it cannot be bound to a public Integration; publish a public replacement artifact instead.`);
      return;
    }
    if (release) {
      setCreateOpen(false);
      setPublishReleaseOpen(false);
      setPublishingArtifact(null);
      setSelectedReleaseID(release.id);
      setBindOpen(true);
      onMessage(`${failure} The exact release was saved; finish its binding in Bind existing.`);
      return;
    }
    if (artifact) {
      setCreateOpen(false);
      if (integration.visibility === "public" && artifact.visibility !== "public") {
        setPublishReleaseOpen(false);
        setPublishingArtifact(null);
        onMessage(`${failure} The private artifact draft was saved, but it must be made public before it can publish and bind to this public Integration.`);
        return;
      }
      setPublishingArtifact(artifact);
      setReleasePublicAcknowledged(artifact.visibility === "public" && packagePublicAcknowledged);
      setPublishReleaseOpen(true);
      onMessage(`${failure} The reusable artifact draft was saved; review and retry its release.`);
      return;
    }
    onMessage(`${failure} Refreshing the catalogue did not find a saved artifact; correct the form or retry.`);
  }

  async function createPublishAndBindPackage() {
    if (integration.visibility === "public" && packageVisibility !== "public") {
      onMessage("A public Integration can only bind a public package release.");
      return;
    }
    let artifact: APIPackageArtifact | null = null;
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      artifact = await api.createPackageArtifact(packageArtifactInput());
      const published = await api.publishPackageRelease(artifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: artifact.revision, acknowledge_public: packageVisibility === "public" && packagePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setCreateOpen(false);
      resetPackageMetadata(); resetReleaseMetadata();
      onMessage(`${artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(artifact, release, packageFailureMessage(error, "Package could not be created, published, and bound."));
    } finally { setBusy(false); }
  }

  async function publishAndBindExistingArtifact() {
    if (!publishingArtifact) return;
    if (!packageArtifactCanPublishForIntegration(publishingArtifact, integration)) {
      onMessage(integration.visibility === "public" && publishingArtifact.visibility !== "public" ? "A private package artifact cannot publish and bind to a public Integration." : "This package artifact cannot publish another release.");
      return;
    }
    let release: APIPackageRelease | null = null;
    setBusy(true);
    try {
      const published = await api.publishPackageRelease(publishingArtifact.id, { version: packageVersion.trim(), purl: releasePURL.trim(), install_command: installCommand.trim(), digest: integrityDigest.trim(), sbom_url: sbomURL.trim() || undefined, provenance_url: provenanceURL.trim() || undefined, artifact_revision: publishingArtifact.revision, acknowledge_public: publishingArtifact.visibility === "public" && releasePublicAcknowledged });
      release = published.release;
      await api.bindIntegrationPackage(integration.id, release.id);
      await loadPackages();
      setPublishReleaseOpen(false); setPublishingArtifact(null); resetReleaseMetadata();
      onMessage(`${published.artifact.name}@${release.version} published and bound to ${integration.display_name}.`);
    } catch (error) {
      await recoverPackageWorkflow(publishingArtifact, release, packageFailureMessage(error, "Package release could not be published and bound."));
    } finally { setBusy(false); }
  }

  async function savePackageEdits() {
    if (!editingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.updatePackageArtifact(editingArtifact.id, { ...packageArtifactInput(), revision: editingArtifact.revision });
      await loadPackages(); setEditOpen(false); setEditingArtifact(null); resetPackageMetadata();
      onMessage(`${updated.name} catalogue metadata updated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package metadata could not be updated."); } finally { setBusy(false); }
  }

  async function deprecatePackage() {
    if (!deprecatingArtifact) return;
    setBusy(true);
    try {
      const updated = await api.deprecatePackageArtifact(deprecatingArtifact.id, { replacement_package_artifact_id: replacementArtifactID || undefined, message: deprecationMessage.trim(), sunset_at: sunsetAt ? new Date(sunsetAt).toISOString() : undefined, revision: deprecatingArtifact.revision });
      await loadPackages(); setDeprecateOpen(false); setDeprecatingArtifact(null);
      onMessage(`${updated.name} marked deprecated.`);
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package could not be deprecated."); } finally { setBusy(false); }
  }

  async function retirePackage() {
    if (!retiringArtifact) return;
    setBusy(true);
    try {
      const updated = await api.retirePackageArtifact(retiringArtifact.id, { replacement_package_artifact_id: retirementReplacementID || undefined, message: retirementMessage.trim(), revision: retiringArtifact.revision });
      await loadPackages(); setRetireOpen(false); setRetiringArtifact(null);
      onMessage(`${updated.name} retired. Existing immutable snapshots retain their exact release.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Package retirement is not available in this deployment yet." : error instanceof APIError ? error.message : "Package could not be retired.");
    } finally { setBusy(false); }
  }

  async function unbind(binding: APIIntegrationPackageBinding) {
    setBusy(true);
    try {
      await api.unbindIntegrationPackage(integration.id, binding.package_artifact_id);
      setBindings((items) => items.filter((item) => item.package_artifact_id !== binding.package_artifact_id));
      onMessage("Package release removed from this API draft.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Package release could not be removed."); } finally { setBusy(false); }
  }

  return <>
    <div className="notice"><Database /><span><strong>Developer artifact catalogue, not a package proxy or verifier.</strong> Registries deliver package bytes; DokoSoko records digest-identified metadata and binds exact releases to compatible API snapshots.</span></div>
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Package binding is not enabled on this deployment.</strong><small>The workspace remains usable and will activate automatically when the package endpoints are installed.</small></span></div>}
    <section className="panel"><PanelHeader title="Bound SDKs & packages" description="Every binding resolves to one exact published release." action={<span className="heading-actions"><Button outline disabled={loading || publishedReleases.length === 0 || bindingsUnavailable} onClick={() => setBindOpen(true)}>Bind existing</Button><Button disabled={bindingsUnavailable} onClick={openCreatePackage}><Plus data-slot="icon" />Add package</Button></span>} />
      {loading ? <div className="empty-row">Loading package catalogue…</div> : bindings.map((binding) => {
        const artifact = binding.artifact ?? catalog.find((candidate) => candidate.id === binding.package_artifact_id);
        const release = binding.release ?? artifact?.releases?.find((candidate) => candidate.id === binding.package_release_id) ?? artifact?.latest_release;
        const replacement = catalog.find((candidate) => candidate.id === artifact?.replacement_package_artifact_id);
        return <div className="provider-row package-binding-row" key={binding.id ?? `${binding.package_artifact_id}:${binding.package_release_id}`}><span className="settings-icon"><Database /></span><span><strong>{artifact?.name ?? binding.package_artifact_id}</strong><small>{artifact?.ecosystem ?? "package"} · {release?.coordinate ?? artifact?.coordinate ?? "—"}@{release?.version ?? binding.package_release_id} · compatible with {integration.version_key}</small>{artifact?.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact?.replacement_package_artifact_id && <small>Replacement: {replacement?.name ?? artifact.replacement_package_artifact_id}</small>}{artifact?.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color="green">exact release</Badge>{artifact && artifact.lifecycle !== "active" && <Badge color={artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge>}</span><span className="table-actions">{release?.digest && <code title={release.digest}>{release.digest.slice(0, 18)}…</code>}<button type="button" className="more" disabled={busy} aria-label={`Unbind ${artifact?.name ?? "package"}`} title="Unbind package" onClick={() => unbind(binding)}><XCircle /></button></span></div>;
      })}
      {!loading && bindings.length === 0 && <div className="empty-row">No SDK or package release is bound to this API.</div>}
    </section>
    <section className="panel"><PanelHeader title="Package catalogue" description="Draft artifact metadata can be edited. Publishing the first release freezes all artifact metadata; later corrections require a replacement artifact. Exact releases are always immutable." />
      {catalog.map((artifact) => {
        const replacement = catalog.find((candidate) => candidate.id === artifact.replacement_package_artifact_id);
        const releases = artifact.releases ?? (artifact.latest_release ? [artifact.latest_release] : []);
        return <div className="provider-row package-binding-row" key={artifact.id}><span className="settings-icon"><Database /></span><span><strong>{artifact.name}</strong><small>{artifact.ecosystem} · {artifact.coordinate} · {releases.length} published release{releases.length === 1 ? "" : "s"}</small><small>Reusable PURL: {artifact.purl}</small>{artifact.deprecation_message && <small>Lifecycle message: {artifact.deprecation_message}</small>}{artifact.replacement_package_artifact_id && <small>Replacement: {replacement ? `${replacement.name} · ${replacement.coordinate}` : artifact.replacement_package_artifact_id}</small>}{artifact.sunset_at && <small>Sunset: {packageLifecycleDate(artifact.sunset_at)}{packageSunsetPassed(artifact.sunset_at) ? " · passed" : ""}</small>}</span><span className="tool-badges"><Badge color={artifact.lifecycle === "active" ? "green" : artifact.lifecycle === "deprecated" ? "amber" : "zinc"}>{artifact.lifecycle}</Badge><Badge color={artifact.visibility === "public" ? "blue" : "zinc"}>{artifact.visibility}</Badge></span><span className="table-actions">{artifact.lifecycle === "draft" && <Button outline disabled={busy} onClick={() => openEditPackage(artifact)}>Edit draft</Button>}{packageArtifactCanPublishForIntegration(artifact, integration) && <Button outline disabled={busy} onClick={() => openPublishRelease(artifact)}>Publish release</Button>}{artifact.lifecycle === "active" && <Button outline disabled={busy} onClick={() => openDeprecatePackage(artifact)}>Deprecate</Button>}{artifact.lifecycle === "deprecated" && <Button outline disabled={busy} onClick={() => openRetirePackage(artifact)}>Retire</Button>}</span></div>;
      })}
      {!loading && catalog.length === 0 && <div className="empty-row">No reusable package artifacts have been created.</div>}
    </section>
    <Dialog open={bindOpen} onClose={setBindOpen} title="Bind a published package" description="Choose an exact release from an active, non-sunset artifact. Deprecated, retired, and sunset entries are excluded." actions={<><Button outline onClick={() => setBindOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedReleaseID} onClick={bindSelectedRelease}>{busy ? "Binding…" : "Bind release"}</Button></>}><label className="auth-field"><span>Package release</span><select value={selectedReleaseID} onChange={(event) => setSelectedReleaseID(event.target.value)}><option value="">Select an exact release</option>{publishedReleases.map(({ artifact, release }) => <option key={release.id} value={release.id}>{artifact.ecosystem} · {artifact.name}@{release.version}</option>)}</select></label></Dialog>
    <Dialog open={createOpen} onClose={setCreateOpen} title="Add SDK or package" description="Create a reusable artifact, publish one exact release, then bind it to this API. A saved draft remains recoverable if a later step fails." actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (packageVisibility === "public" && !packagePublicAcknowledged) || (integration.visibility === "public" && packageVisibility !== "public")} onClick={createPublishAndBindPackage}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} placeholder="npm or vendor.ecosystem" /><small>Choose a suggestion or enter a lowercase vendor ecosystem identifier.</small></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} placeholder="Vendor JavaScript SDK" /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} placeholder="What this developer artifact provides." /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} placeholder="@vendor/sdk" /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk" /><small>Stable package identity without a version, query, or fragment.</small></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} placeholder="https://registry.npmjs.org/…" /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} placeholder="https://github.com/vendor/sdk" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} placeholder="TypeScript" /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} placeholder="Node.js 22+" /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder="pkg:npm/%40vendor/sdk@3.2.1" /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>Required and syntax-validated. DokoSoko stores the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public makes this metadata eligible for public Integration discovery only after an exact public release is bound, the Integration is published, and Public MCP is enabled. It does not create a standalone public package catalogue.</small></label>
        {integration.visibility === "public" && packageVisibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>A public Integration requires a public package release.</strong><small>Choose Public before publishing and binding this package.</small></span></div>}
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public package and release metadata becomes discoverable through Public MCP only after public binding and Integration publication.</span></label>}
      </div>
    </Dialog>
    <Dialog open={publishReleaseOpen} onClose={setPublishReleaseOpen} title={`Publish release for ${publishingArtifact?.name ?? "package"}`} description={`Publish and bind a new immutable release of ${publishingArtifact?.purl ?? "this reusable artifact"}. If binding fails, the saved release remains available through Bind existing.`} actions={<><Button outline onClick={() => setPublishReleaseOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !publishingArtifact || !packageArtifactCanPublishForIntegration(publishingArtifact, integration) || !packageVersion.trim() || !releasePURL.trim() || !installCommand.trim() || !integrityDigest.trim() || (publishingArtifact.visibility === "public" && !releasePublicAcknowledged)} onClick={publishAndBindExistingArtifact}>{busy ? "Publishing…" : "Publish & bind"}</Button></>}>
      <div className="auth-form compact-form">
        {publishingArtifact && !packageArtifactCanPublish(publishingArtifact) && <div className="capability-unavailable"><TriangleAlert /><span><strong>This artifact cannot publish another release.</strong><small>Deprecated, retired, and sunset artifacts are immutable lifecycle records.</small></span></div>}
        {publishingArtifact && packageArtifactCanPublish(publishingArtifact) && integration.visibility === "public" && publishingArtifact.visibility !== "public" && <div className="capability-unavailable"><TriangleAlert /><span><strong>This private artifact cannot bind to a public Integration.</strong><small>Create a public replacement artifact instead.</small></span></div>}
        <div className="two-fields"><label className="auth-field"><span>Exact version</span><input value={packageVersion} onChange={(event) => setPackageVersion(event.target.value)} placeholder="3.2.1" /></label><label className="auth-field"><span>Exact release PURL</span><input value={releasePURL} onChange={(event) => setReleasePURL(event.target.value)} placeholder={`${publishingArtifact?.purl ?? "pkg:npm/%40vendor/sdk"}@3.2.1`} /><small>Must equal the reusable artifact PURL plus this exact version.</small></label></div>
        <label className="auth-field"><span>Install command</span><input value={installCommand} onChange={(event) => setInstallCommand(event.target.value)} placeholder="npm install @vendor/sdk@3.2.1" /></label>
        <label className="auth-field"><span>Integrity digest</span><input value={integrityDigest} onChange={(event) => setIntegrityDigest(event.target.value)} placeholder="sha256:…" /><small>DokoSoko records the supplied digest but does not download or verify package bytes.</small></label>
        <div className="two-fields"><label className="auth-field"><span>SBOM URL</span><input type="url" value={sbomURL} onChange={(event) => setSBOMURL(event.target.value)} /></label><label className="auth-field"><span>Provenance URL</span><input type="url" value={provenanceURL} onChange={(event) => setProvenanceURL(event.target.value)} /></label></div>
        {publishingArtifact?.visibility === "public" && <><div className="notice"><ShieldCheck /><span><strong>Public discovery remains gated.</strong> This exact release is only eligible for discovery after public binding, Integration publication, and Public MCP enablement.</span></div><label className="auth-check"><input type="checkbox" checked={releasePublicAcknowledged} onChange={(event) => setReleasePublicAcknowledged(event.target.checked)} /><span>I explicitly confirm this exact release may become discoverable through Public MCP after those publication gates are satisfied.</span></label></>}
      </div>
    </Dialog>
    <Dialog open={editOpen} onClose={setEditOpen} title="Edit draft package" description="Draft catalogue metadata may be replaced until the first exact release is published." actions={<><Button outline onClick={() => setEditOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !editingArtifact || !ecosystemValid || !packageName.trim() || !packageCoordinate.trim() || !artifactPURL.trim() || !registryURL.trim() || (packageVisibility === "public" && !packagePublicAcknowledged)} onClick={savePackageEdits}>{busy ? "Saving…" : "Save draft"}</Button></>}>
      <div className="auth-form compact-form">
        <div className="two-fields"><label className="auth-field"><span>Ecosystem identifier</span><input list={ecosystemOptionsID} pattern="[a-z][a-z0-9._-]{0,63}" value={ecosystem} onChange={(event) => setEcosystem(event.target.value.toLowerCase())} /></label><label className="auth-field"><span>Display name</span><input value={packageName} onChange={(event) => setPackageName(event.target.value)} /></label></div>
        <label className="auth-field"><span>Description</span><textarea value={packageDescription} onChange={(event) => setPackageDescription(event.target.value)} /></label>
        <div className="two-fields"><label className="auth-field"><span>Registry coordinate</span><input value={packageCoordinate} onChange={(event) => setPackageCoordinate(event.target.value)} /></label><label className="auth-field"><span>Reusable artifact PURL</span><input value={artifactPURL} onChange={(event) => setArtifactPURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Registry URL</span><input type="url" value={registryURL} onChange={(event) => setRegistryURL(event.target.value)} /></label><label className="auth-field"><span>Source URL</span><input type="url" value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} /></label></div>
        <div className="two-fields"><label className="auth-field"><span>Language</span><input value={packageLanguage} onChange={(event) => setPackageLanguage(event.target.value)} /></label><label className="auth-field"><span>Platform</span><input value={packagePlatform} onChange={(event) => setPackagePlatform(event.target.value)} /></label></div>
        <label className="auth-field"><span>Discovery visibility</span><select value={packageVisibility} onChange={(event) => { setPackageVisibility(event.target.value as APIVisibility); setPackagePublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public eligibility still requires an exact public binding, a published public Integration, and Public MCP.</small></label>
        {packageVisibility === "public" && <label className="auth-check"><input type="checkbox" checked={packagePublicAcknowledged} onChange={(event) => setPackagePublicAcknowledged(event.target.checked)} /><span>I understand public metadata remains gated by public binding, Integration publication, and Public MCP.</span></label>}
      </div>
    </Dialog>
    <Dialog open={deprecateOpen} onClose={setDeprecateOpen} title={`Deprecate ${deprecatingArtifact?.name ?? "package"}`} description="Deprecation immediately blocks new releases, new bindings, and candidate publication. Historical published snapshots remain readable; an optional sunset is migration guidance only." actions={<><Button outline onClick={() => setDeprecateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !deprecatingArtifact || !deprecationMessage.trim()} onClick={deprecatePackage}>{busy ? "Deprecating…" : "Deprecate"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Deprecation message</span><textarea value={deprecationMessage} onChange={(event) => setDeprecationMessage(event.target.value)} placeholder="Explain the migration path." /></label><label className="auth-field"><span>Replacement artifact</span><select value={replacementArtifactID} onChange={(event) => setReplacementArtifactID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== deprecatingArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label><label className="auth-field"><span>Optional sunset</span><input type="datetime-local" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /><small>Guidance for migration timing; deprecation enforcement is immediate.</small></label></div>
    </Dialog>
    <Dialog open={retireOpen} onClose={setRetireOpen} title={`Retire ${retiringArtifact?.name ?? "package"}`} description="Retirement permanently prevents new releases and bindings. Existing immutable snapshots retain their exact release and lifecycle warning." actions={<><Button outline onClick={() => setRetireOpen(false)}>Cancel</Button><Button color="red" disabled={busy || !retiringArtifact || !retirementMessage.trim()} onClick={retirePackage}>{busy ? "Retiring…" : "Retire package"}</Button></>}>
      <div className="auth-form compact-form"><label className="auth-field"><span>Retirement message</span><textarea value={retirementMessage} onChange={(event) => setRetirementMessage(event.target.value)} placeholder="Explain why this artifact is retired and where consumers should migrate." /></label><label className="auth-field"><span>Replacement artifact</span><select value={retirementReplacementID} onChange={(event) => setRetirementReplacementID(event.target.value)}><option value="">No replacement</option>{replacementCandidates.filter((artifact) => artifact.id !== retiringArtifact?.id).map((artifact) => <option value={artifact.id} key={artifact.id}>{artifact.name} · {artifact.coordinate}</option>)}</select></label></div>
    </Dialog>
    <datalist id={ecosystemOptionsID}><option value="npm" /><option value="pypi" /><option value="maven" /><option value="nuget" /><option value="go" /><option value="docker" /><option value="oci" /></datalist>
  </>;
}

function toolPolicy(tool: APITool) {
  const grants = Array.isArray(tool.authorization_policy?.required_grants) ? tool.authorization_policy.required_grants.filter((value): value is string => typeof value === "string") : [];
  const fallbackRisk = tool.http_method === "GET" ? "low" : tool.http_method === "DELETE" ? "critical" : "medium";
  const storedRisk = tool.authorization_policy?.risk;
  const risk = storedRisk === "low" || storedRisk === "medium" || storedRisk === "high" || storedRisk === "critical" ? storedRisk : fallbackRisk;
  return {
    requiredGrants: grants,
    confirmationRequired: tool.authorization_policy?.confirmation_required === true,
    risk,
    idempotencyRequired: tool.authorization_policy?.idempotency_required === true,
  };
}

function IntegrationAuthorizationWorkspace({ integration, identity, connections, tools, onManageAccess, onMessage, onNavigate }: { integration: APIIntegration; identity: APIIdentity | null; connections: APIAccessConnection[]; tools: APITool[]; onManageAccess: () => void; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [definitions, setDefinitions] = useState<APIGrantDefinition[]>([]);
  const [points, setPoints] = useState<APIAuthorizationPoint[]>([]);
  const [catalogUnavailable, setCatalogUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [grantOpen, setGrantOpen] = useState(false);
  const [editingGrant, setEditingGrant] = useState<APIGrantDefinition | null>(null);
  const [grantKey, setGrantKey] = useState("");
  const [grantName, setGrantName] = useState("");
  const [grantDescription, setGrantDescription] = useState("");
  const [grantRisk, setGrantRisk] = useState<APIGrantDefinition["risk"]>("low");
  const [grantState, setGrantState] = useState<APIGrantDefinition["state"]>("active");
  const [pointOpen, setPointOpen] = useState(false);
  const [editingPoint, setEditingPoint] = useState<APIAuthorizationPoint | null>(null);
  const [pointKey, setPointKey] = useState("");
  const [pointName, setPointName] = useState("");
  const [pointDescription, setPointDescription] = useState("");
  const [pointAction, setPointAction] = useState<APIAuthorizationPoint["action_type"]>("read");
  const [pointGrants, setPointGrants] = useState<string[]>([]);
  const [pointConfirmation, setPointConfirmation] = useState(false);
  const [pointTTL, setPointTTL] = useState("300");
  const [pointState, setPointState] = useState<APIAuthorizationPoint["state"]>("draft");
  const [selectedPointID, setSelectedPointID] = useState("");
  const [suppliedGrants, setSuppliedGrants] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [simulation, setSimulation] = useState<APIAuthorizationSimulation | null>(null);
  const [simulationSource, setSimulationSource] = useState<"server" | "configuration-only">("server");
  const [simulating, setSimulating] = useState(false);

  const loadAuthorization = useCallback(async () => {
    const [definitionResult, pointResult] = await Promise.allSettled([api.grantDefinitions(), api.authorizationPoints(integration.id)]);
    if (definitionResult.status === "fulfilled") setDefinitions(definitionResult.value);
    if (pointResult.status === "fulfilled") {
      setPoints(pointResult.value);
      setSelectedPointID((current) => current && pointResult.value.some((point) => point.id === current) ? current : pointResult.value[0]?.id ?? "");
    }
    const unavailable = (definitionResult.status === "rejected" && unavailableConsoleCapability(definitionResult.reason)) || (pointResult.status === "rejected" && unavailableConsoleCapability(pointResult.reason));
    setCatalogUnavailable(unavailable);
  }, [integration.id]);

  useEffect(() => {
    const task = window.setTimeout(() => { void loadAuthorization(); }, 0);
    return () => window.clearTimeout(task);
  }, [loadAuthorization]);

  const selectedPoint = points.find((point) => point.id === selectedPointID) ?? points[0];
  const registeredKeys = new Set(definitions.filter((definition) => definition.state === "active").map((definition) => definition.key));

  function openGrant(value?: APIGrantDefinition) {
    setEditingGrant(value ?? null); setGrantKey(value?.key ?? ""); setGrantName(value?.display_name ?? ""); setGrantDescription(value?.description ?? ""); setGrantRisk(value?.risk ?? "low"); setGrantState(value?.state ?? "active"); setGrantOpen(true);
  }

  async function saveGrant() {
    setBusy(true);
    try {
      const input = { key: grantKey.trim(), display_name: grantName.trim(), description: grantDescription.trim(), risk: grantRisk, state: grantState };
      if (editingGrant) await api.updateGrantDefinition(editingGrant.id, { ...input, revision: editingGrant.revision }); else await api.createGrantDefinition(input);
      await loadAuthorization(); setGrantOpen(false); onMessage(editingGrant ? "Grant definition updated." : "Grant registered for policy use.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Grant definition could not be saved."); } finally { setBusy(false); }
  }

  function openPoint(value?: APIAuthorizationPoint) {
    setEditingPoint(value ?? null); setPointKey(value?.key ?? ""); setPointName(value?.name ?? ""); setPointDescription(value?.description ?? ""); setPointAction(value?.action_type ?? "read"); setPointGrants(value?.required_grants ?? []); setPointConfirmation(value?.confirmation_required ?? false); setPointTTL(String(value?.decision_ttl_seconds ?? 300)); setPointState(value?.state ?? "draft"); setPointOpen(true);
  }

  async function savePoint() {
    setBusy(true);
    try {
      const input = { key: pointKey.trim(), name: pointName.trim(), description: pointDescription.trim(), action_type: pointAction, required_grants: pointGrants, confirmation_required: pointAction === "destructive" ? true : pointConfirmation, decision_ttl_seconds: Number(pointTTL), state: pointState };
      if (editingPoint) await api.updateAuthorizationPoint(integration.id, editingPoint.id, { ...input, revision: editingPoint.revision }); else await api.createAuthorizationPoint(integration.id, input);
      await loadAuthorization(); setPointOpen(false); onMessage(editingPoint ? "Authorization point updated." : "Authorization point created.");
    } catch (error) { onMessage(error instanceof APIError ? error.message : "Authorization point could not be saved."); } finally { setBusy(false); }
  }

  async function simulate() {
    if (!selectedPoint) return;
    setSimulating(true);
    const granted = suppliedGrants.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean);
    try {
      setSimulation(await api.simulateAuthorizationPoint(integration.id, selectedPoint.id, granted, confirmed));
      setSimulationSource("server");
    } catch (error) {
      if (!unavailableConsoleCapability(error)) { onMessage(error instanceof APIError ? error.message : "Authorization policy could not be simulated."); setSimulating(false); return; }
      const missing = selectedPoint.required_grants.filter((grant) => !granted.includes(grant));
      const confirmationMissing = selectedPoint.confirmation_required && !confirmed;
      setSimulation({ authorization_point_id: selectedPoint.id, allowed: selectedPoint.state === "active" && missing.length === 0 && !confirmationMissing, missing_grants: missing, confirmation_required: selectedPoint.confirmation_required, confirmation_missing: confirmationMissing, explanation: "Local comparison only; this does not prove live vendor access.", simulation_only: true });
      setSimulationSource("configuration-only");
    } finally { setSimulating(false); }
  }

  return <div className="integration-tab-content">
    <div className="notice"><ShieldCheck /><span><strong>One authorization vocabulary: grants.</strong> Declarative points have no URL or secret. Runtime identity resolves live vendor access through the fixed fail-closed evaluation contract.</span></div>
    {catalogUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Authorization catalogue unavailable.</strong><small>Existing tool policies remain visible, but grant and point changes cannot be saved by this deployment.</small></span></div>}
    <div className="integration-two-column">
      <section className="panel"><PanelHeader title="Customer identity" action={<ConsoleLink path={settingsPath("identity")} onNavigate={onNavigate} className="entity-back-link">Configure identity</ConsoleLink>} />{identity ? <div className="provider-row"><span className="settings-icon"><Users /></span><span><strong>{identity.issuer}</strong><small>Resource {identity.oauth_resource || "not set"} · claim {identity.organisation_claim}</small></span><Badge color={identity.state === "active" ? "green" : "amber"}>{identity.state}</Badge></div> : <div className="empty-row">Customer OIDC has not been configured.</div>}</section>
      <section className="panel"><PanelHeader title="Service connections" description="Reusable environment and credential boundaries for this API." action={<Button onClick={onManageAccess}>Choose connections</Button>} />{connections.map((connection) => <div className="provider-row integration-connection-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{connection.definition?.name ?? "Service connection"}{connection.region ? ` · ${connection.region}` : ""}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge></div>)}{connections.length === 0 && <div className="empty-row">No service connection is attached to this API.</div>}<ConsoleLink path={sectionPath("projects")} onNavigate={onNavigate} className="panel-footer-link">Manage reusable access definitions and connections <ChevronRight /></ConsoleLink></section>
    </div>
    <section className="panel"><PanelHeader title="Grant registry" description="Deployment-owned vocabulary that vendor evaluations may return and policies may require. A definition never grants access." action={<span className="heading-actions"><Badge color="violet">{definitions.length} grants</Badge><Button disabled={catalogUnavailable} onClick={() => openGrant()}><Plus data-slot="icon" />Register grant</Button></span>} />{definitions.map((definition) => <div className="provider-row grant-definition-row" key={definition.id}><span className="settings-icon"><KeyRound /></span><span><strong>{definition.display_name}</strong><small><code>{definition.key}</code> · {definition.description || "No description"}</small></span><span className="tool-badges"><Badge color={definition.risk === "critical" || definition.risk === "high" ? "red" : definition.risk === "medium" ? "amber" : "zinc"}>{definition.risk}</Badge><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></span><Button outline onClick={() => openGrant(definition)}>Edit</Button></div>)}{definitions.length === 0 && <div className="empty-row">No grant has been registered yet.</div>}</section>
    <section className="panel"><PanelHeader title="Authorization points" description="Map named API actions to registered grants, risk behavior, confirmation and bounded decision caching." action={<Button disabled={catalogUnavailable || definitions.every((definition) => definition.state !== "active")} onClick={() => openPoint()}><Plus data-slot="icon" />Add point</Button>} />{points.map((point) => <div className="provider-row authorization-point-row" key={point.id}><span className="settings-icon"><ShieldCheck /></span><span><strong>{point.name}</strong><small><code>{point.key}</code> · {point.required_grants.join(", ") || "no grants"} · TTL {point.decision_ttl_seconds}s</small></span><span className="tool-badges"><Badge color={point.action_type === "destructive" ? "red" : point.action_type === "write" ? "amber" : "blue"}>{point.action_type}</Badge>{point.confirmation_required && <Badge color="violet">confirmation</Badge>}<Badge color={point.state === "active" ? "green" : "zinc"}>{point.state}</Badge></span><Button outline onClick={() => openPoint(point)}>Edit</Button></div>)}{points.length === 0 && <div className="empty-row">No authorization point has been configured for this API.</div>}</section>
    <section className="panel"><PanelHeader title="Policy simulator" description="Server-side configuration preflight only. It never calls the vendor access-evaluation contract or grants runtime access." action={<Badge color="zinc">Simulation only</Badge>} />{points.length > 0 ? <div className="authorization-simulator"><div className="two-fields"><label className="auth-field"><span>Authorization point</span><select value={selectedPoint?.id ?? ""} onChange={(event) => { setSelectedPointID(event.target.value); setSimulation(null); }}>{points.map((point) => <option value={point.id} key={point.id}>{point.name} · {point.key}</option>)}</select></label><label className="auth-field"><span>Hypothetical granted grants</span><input value={suppliedGrants} onChange={(event) => { setSuppliedGrants(event.target.value); setSimulation(null); }} placeholder="customers.read, invoices.write" /></label></div><label className="compact-check"><input type="checkbox" checked={confirmed} onChange={(event) => { setConfirmed(event.target.checked); setSimulation(null); }} /><span>Simulate explicit confirmation for this exact action</span></label><div className="simulator-actions"><Button disabled={simulating || !selectedPoint} onClick={simulate}>{simulating ? "Evaluating…" : "Simulate policy"}</Button>{selectedPoint && <small>Requires {selectedPoint.required_grants.join(", ") || "no grants"}{selectedPoint.confirmation_required ? " and confirmation" : ""}. State must be active.</small>}</div>{simulation && <div className={`simulation-result ${simulation.allowed ? "allowed" : "denied"}`}><span>{simulation.allowed ? <CheckCircle2 /> : <XCircle />}</span><span><strong>{simulation.allowed ? "Would pass configured policy" : "Would be denied"}</strong><small>{simulation.explanation}{simulation.missing_grants.length ? ` Missing: ${simulation.missing_grants.join(", ")}.` : ""}</small></span><Badge color={simulationSource === "server" ? "green" : "zinc"}>{simulationSource === "server" ? "Server simulation" : "Configuration only"}</Badge></div>}</div> : <div className="empty-row">Create an authorization point before simulating policy.</div>}</section>
    <details className="panel advanced-details"><summary>Tool policy coverage</summary>{tools.map((tool) => { const policy = toolPolicy(tool); const missing = policy.requiredGrants.filter((grant) => !registeredKeys.has(grant)); return <div className="lease-row" key={tool.id}><span><strong>{tool.namespace}.{tool.name}</strong><small>{policy.requiredGrants.join(", ") || "No required grants"}</small></span><Badge color={missing.length ? "red" : "green"}>{missing.length ? `${missing.length} unregistered` : "registered"}</Badge></div>; })}{tools.length === 0 && <div className="empty-row">No tool policies exist yet.</div>}</details>
    <Dialog open={grantOpen} onClose={setGrantOpen} title={editingGrant ? "Edit grant definition" : "Register grant"} description="Grant keys are stable contract identifiers. Editing never grants a user access." actions={<><Button outline onClick={() => setGrantOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !grantKey.trim() || !grantName.trim()} onClick={saveGrant}>{busy ? "Saving…" : "Save grant"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Grant key</span><input disabled={Boolean(editingGrant)} value={grantKey} onChange={(event) => setGrantKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Display name</span><input value={grantName} onChange={(event) => setGrantName(event.target.value)} placeholder="Read customers" /></label><label className="auth-field"><span>Description</span><textarea value={grantDescription} onChange={(event) => setGrantDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Risk</span><select value={grantRisk} onChange={(event) => setGrantRisk(event.target.value as APIGrantDefinition["risk"])}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label><label className="auth-field"><span>State</span><select value={grantState} onChange={(event) => setGrantState(event.target.value as APIGrantDefinition["state"])}><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></div></Dialog>
    <Dialog open={pointOpen} onClose={setPointOpen} title={editingPoint ? "Edit authorization point" : "Add authorization point"} description="Configure a declarative action policy. There is deliberately no hook URL or credential field." actions={<><Button outline onClick={() => setPointOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !pointKey.trim() || !pointName.trim() || pointGrants.some((grant) => !registeredKeys.has(grant))} onClick={savePoint}>{busy ? "Saving…" : "Save point"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Point key</span><input disabled={Boolean(editingPoint)} value={pointKey} onChange={(event) => setPointKey(event.target.value)} placeholder="customers.read" /></label><label className="auth-field"><span>Name</span><input value={pointName} onChange={(event) => setPointName(event.target.value)} placeholder="Read customer" /></label></div><label className="auth-field"><span>Description</span><textarea value={pointDescription} onChange={(event) => setPointDescription(event.target.value)} /></label><div className="two-fields"><label className="auth-field"><span>Action type</span><select value={pointAction} onChange={(event) => { const value = event.target.value as APIAuthorizationPoint["action_type"]; setPointAction(value); if (value === "destructive") setPointConfirmation(true); }}><option value="read">Read</option><option value="write">Write</option><option value="destructive">Destructive</option></select></label><label className="auth-field"><span>Decision TTL (seconds)</span><input type="number" min={0} max={3600} value={pointTTL} onChange={(event) => setPointTTL(event.target.value)} /></label></div><fieldset className="catalog-settings-section"><legend>Required registered grants</legend>{definitions.map((definition) => { const selected = pointGrants.includes(definition.key); return <label className="compact-check" key={definition.id}><input type="checkbox" disabled={definition.state !== "active" && !selected} checked={selected} onChange={() => setPointGrants((current) => current.includes(definition.key) ? current.filter((key) => key !== definition.key) : [...current, definition.key])} /><span>{definition.display_name}<small>{definition.key} · {definition.risk}{definition.state === "deprecated" ? " · deprecated (remove before saving)" : ""}</small></span></label>; })}</fieldset><label className="compact-check"><input type="checkbox" disabled={pointAction === "destructive"} checked={pointConfirmation || pointAction === "destructive"} onChange={(event) => setPointConfirmation(event.target.checked)} /><span>Require explicit confirmation for this action</span></label><label className="auth-field"><span>State</span><select value={pointState} onChange={(event) => setPointState(event.target.value as APIAuthorizationPoint["state"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option></select></label></div></Dialog>
  </div>;
}

type IntegrationToolBindingSelection = { revision: number; authorizationPointID: string; authorizationPointRevision: number };

function integrationToolBindingSelectionSignature(selection: Record<string, IntegrationToolBindingSelection>) {
  return JSON.stringify(Object.entries(selection).sort(([left], [right]) => left.localeCompare(right)));
}

function IntegrationToolsWorkspace({ integration, tools, onMessage, onNavigate }: { integration: APIIntegration; tools: APITool[]; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
  const [bindings, setBindings] = useState<APIIntegrationToolBinding[]>([]);
  const [authorizationPoints, setAuthorizationPoints] = useState<APIAuthorizationPoint[]>([]);
  const [bindingSelection, setBindingSelection] = useState<Record<string, IntegrationToolBindingSelection>>({});
  const [savedSignature, setSavedSignature] = useState<string | null>(null);
  const [bindingsLoading, setBindingsLoading] = useState(true);
  const [bindingsUnavailable, setBindingsUnavailable] = useState(false);
  const [bindingBusy, setBindingBusy] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [attachOpen, setAttachOpen] = useState(false);
  const [attachToolID, setAttachToolID] = useState("");
  const [attachPointID, setAttachPointID] = useState("");

  const activePoints = authorizationPoints.filter((point) => point.state === "active");
  const availableTools = tools.filter((tool) => tool.state === "published" && !tool.upstream_drifted && !bindingSelection[tool.id]);
  const dirty = savedSignature !== null && integrationToolBindingSelectionSignature(bindingSelection) !== savedSignature;

  useEffect(() => {
    let cancelled = false;
    Promise.all([api.integrationToolBindings(integration.id), api.authorizationPoints(integration.id)]).then(([values, points]) => {
      if (cancelled) return;
      const next = Object.fromEntries(values.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(values);
      setAuthorizationPoints(points);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      setBindingsLoading(false);
      setBindingsUnavailable(false);
    }).catch(() => {
      if (cancelled) return;
      setBindings([]);
      setAuthorizationPoints([]);
      setBindingSelection({});
      setSavedSignature(null);
      setBindingsLoading(false);
      setBindingsUnavailable(true);
    });
    return () => { cancelled = true; };
  }, [integration.id, loadAttempt]);

  function retryBindings() {
    setBindingsLoading(true);
    setBindingsUnavailable(false);
    setLoadAttempt((value) => value + 1);
  }

  function openAttachDialog() {
    const defaultTool = availableTools[0];
    const defaultPoint = activePoints[0];
    setAttachToolID(defaultTool?.id ?? "");
    setAttachPointID(defaultPoint?.id ?? "");
    setAttachOpen(true);
  }

  function attachTool() {
    const tool = tools.find((candidate) => candidate.id === attachToolID && candidate.state === "published" && !candidate.upstream_drifted);
    const point = activePoints.find((candidate) => candidate.id === attachPointID);
    if (!tool || !point) return;
    setBindingSelection((current) => ({ ...current, [tool.id]: { revision: tool.revision, authorizationPointID: point.id, authorizationPointRevision: point.revision } }));
    setAttachOpen(false);
  }

  function removeBinding(toolID: string) {
    setBindingSelection((current) => {
      const next = { ...current };
      delete next[toolID];
      return next;
    });
	requestAnimationFrame(() => document.getElementById("save-api-bindings")?.focus());
  }

  function selectAuthorizationPoint(toolID: string, pointID: string) {
    const point = activePoints.find((candidate) => candidate.id === pointID);
    if (!point) return;
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], authorizationPointID: point.id, authorizationPointRevision: point.revision } } : current);
  }

  function selectCurrentToolRevision(toolID: string, revision: number) {
    setBindingSelection((current) => current[toolID] ? { ...current, [toolID]: { ...current[toolID], revision } } : current);
  }

  function resolveBinding(toolID: string, selection: IntegrationToolBindingSelection) {
    const tool = tools.find((candidate) => candidate.id === toolID) ?? bindings.find((binding) => binding.tool_id === toolID)?.tool;
    const point = authorizationPoints.find((candidate) => candidate.id === selection.authorizationPointID);
    const toolCurrent = Boolean(tool && tool.state === "published" && !tool.upstream_drifted && tool.revision === selection.revision);
    const pointCurrent = Boolean(point && point.state === "active" && point.revision === selection.authorizationPointRevision);
    const issues = [
      !tool ? "tool missing" : tool.state !== "published" ? `tool ${tool.state}` : tool.upstream_drifted ? "schema drift" : tool.revision !== selection.revision ? `tool is now r${tool.revision}` : "",
      !point ? "authorization point missing" : point.state !== "active" ? `authorization ${point.state}` : point.revision !== selection.authorizationPointRevision ? `authorization is now r${point.revision}` : "",
    ].filter(Boolean);
    return { tool, point, current: toolCurrent && pointCurrent, issues };
  }

  async function saveBindings() {
    setBindingBusy(true);
    try {
      const value = await api.setIntegrationToolBindings(integration.id, Object.entries(bindingSelection).map(([tool_id, selection]) => ({ tool_id, revision: selection.revision, authorization_point_id: selection.authorizationPointID, authorization_point_revision: selection.authorizationPointRevision })));
      const next = Object.fromEntries(value.items.map((binding) => [binding.tool_id, { revision: binding.tool_revision, authorizationPointID: binding.authorization_point_id, authorizationPointRevision: binding.authorization_point_revision }]));
      setBindings(value.items);
      setBindingSelection(next);
      setSavedSignature(integrationToolBindingSelectionSignature(next));
      onMessage(value.items.length === 0 ? "All tool bindings cleared from this API draft." : `${value.items.length} exact tool revision${value.items.length === 1 ? "" : "s"} bound to this API draft.`);
    } catch (error) {
      onMessage(unavailableConsoleCapability(error) ? "Exact API tool bindings are not enabled in this deployment yet." : error instanceof APIError ? error.message : "Tool bindings could not be saved.");
    } finally { setBindingBusy(false); }
  }

  const selectedBindings = Object.entries(bindingSelection);
  const staleBindingCount = selectedBindings.filter(([toolID, selection]) => !resolveBinding(toolID, selection).current).length;

  return <div className="integration-tab-content">
    {bindingsUnavailable && <div className="capability-unavailable"><TriangleAlert /><span><strong>Exact tool binding is unavailable.</strong><small>The current API bindings and authorization points could not be loaded. No changes can be saved.</small></span><Button outline onClick={retryBindings}>Retry</Button></div>}
    {activePoints.length === 0 && !bindingsLoading && !bindingsUnavailable && <div className="capability-unavailable"><ShieldCheck /><span><strong>Create an active authorization point first.</strong><small>Every exposed tool must pin an exact authorization policy revision.</small></span><ConsoleLink path={integrationPath(integration.id, "authorization")} onNavigate={onNavigate} className="entity-back-link">Open Authorization</ConsoleLink></div>}
    <section className="panel integration-tool-bindings">
      <PanelHeader title="Exposed tools" description="This API draft selects exact published tool and authorization-point revisions. Tool contracts are managed in the deployment catalog." action={<span className="heading-actions">{dirty && <Badge color="amber">Unsaved changes</Badge>}<ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className="entity-back-link">Open Tools catalog</ConsoleLink><Button disabled={bindingsLoading || bindingBusy || bindingsUnavailable || activePoints.length === 0 || availableTools.length === 0} onClick={openAttachDialog}><Plus data-slot="icon" />Attach published tool</Button></span>} />
      {selectedBindings.map(([toolID, selection]) => {
        const resolution = resolveBinding(toolID, selection);
        const tool = resolution.tool;
        const pointCurrent = resolution.point?.state === "active" && resolution.point.revision === selection.authorizationPointRevision;
        return <div className={`integration-tool-binding-row ${resolution.current ? "" : "stale"}`} key={toolID}>
          <span className="settings-icon">{tool?.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span>
          <span className="integration-tool-binding-main">{tool ? <EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink> : <strong>{toolID}</strong>}<small>pinned tool revision {selection.revision}{tool ? ` · ${tool.backend_kind === "mcp" ? "MCP" : tool.http_method}` : ""}</small></span>
          <label className="tool-binding-action"><span className="sr-only">Authorization point for {tool ? `${tool.namespace}.${tool.name}` : toolID}</span><select disabled={bindingsLoading || bindingBusy} aria-label={`Authorization point for ${tool ? `${tool.namespace}.${tool.name}` : toolID}`} value={pointCurrent ? selection.authorizationPointID : ""} onChange={(event) => selectAuthorizationPoint(toolID, event.target.value)}>{!pointCurrent && <option value="" disabled>Choose a current authorization point</option>}{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · r{point.revision}</option>)}</select><small>pinned authorization revision {selection.authorizationPointRevision}</small></label>
          <span className="tool-badges"><Badge color={resolution.current ? "green" : "red"}>{resolution.current ? "Current" : "Stale / unresolved"}</Badge>{tool?.upstream_drifted && <Badge color="red">schema drift</Badge>}<small className="binding-issue">{resolution.issues.join(" · ")}</small></span>
          <span className="binding-row-actions">{tool && tool.state === "published" && !tool.upstream_drifted && tool.revision !== selection.revision && <Button outline disabled={bindingsLoading || bindingBusy} onClick={() => selectCurrentToolRevision(toolID, tool.revision)}>Use r{tool.revision}</Button>}<Button outline disabled={bindingsLoading || bindingBusy} aria-label={`Remove ${tool ? `${tool.namespace}.${tool.name}` : toolID} from this API draft`} onClick={() => removeBinding(toolID)}>Remove</Button></span>
        </div>;
      })}
      {bindingsLoading ? <div className="empty-row">Loading exact tool and authorization bindings…</div> : selectedBindings.length === 0 && <div className="empty-row"><span>No tools are exposed by this API draft. Attach a published deployment tool when this API needs an executable capability.</span></div>}
      <div className="panel-footer-actions"><small>{bindingsLoading ? "Loading current API configuration…" : `${selectedBindings.length} selected · ${bindings.length} currently saved${staleBindingCount > 0 ? ` · ${staleBindingCount} stale or unresolved` : ""}`}</small><Button id="save-api-bindings" disabled={bindingsLoading || bindingsUnavailable || bindingBusy || staleBindingCount > 0 || !dirty} onClick={saveBindings}>{bindingBusy ? "Saving…" : "Save API bindings"}</Button></div>
    </section>
    <Dialog open={attachOpen} onClose={setAttachOpen} title="Attach published tool" description="Choose one deployment tool and pin it to one active authorization-point revision for this API draft." actions={<><Button outline disabled={bindingBusy} onClick={() => setAttachOpen(false)}>Cancel</Button><Button color="indigo" disabled={bindingBusy || !attachToolID || !attachPointID} onClick={attachTool}>Attach tool</Button></>}>
      <div className="auth-form compact-form">
        <label className="auth-field"><span>Published tool</span><select value={attachToolID} onChange={(event) => setAttachToolID(event.target.value)}><option value="" disabled>No eligible tool selected</option>{availableTools.map((tool) => <option key={tool.id} value={tool.id}>{tool.namespace}.{tool.name} · r{tool.revision}</option>)}</select><small>Draft, retired and schema-drifted tools are not eligible.</small></label>
        <label className="auth-field"><span>Authorization point</span><select value={attachPointID} onChange={(event) => setAttachPointID(event.target.value)}><option value="" disabled>No active point selected</option>{activePoints.map((point) => <option key={point.id} value={point.id}>{point.name} · {point.action_type} · r{point.revision}</option>)}</select><small>The API pins this exact revision; later authorization changes make the binding stale until reviewed.</small></label>
      </div>
    </Dialog>
  </div>;
}
function IntegrationRecipesWorkspace({ analyses, recipes, busy, onGenerate, onNavigate }: { analyses: APIIntegrationAnalysis[]; recipes: APIRecipe[]; busy: boolean; onGenerate: () => void; onNavigate: (path: string) => void }) {
  const latestAnalysis = [...analyses].sort((left, right) => right.created_at.localeCompare(left.created_at))[0];
  const blockers = latestAnalysis?.unknowns.filter((unknown) => unknown.blocking && !unknown.answer?.trim()) ?? [];
  return <div className="integration-tab-content">
    <div className="notice"><Sparkles /><span><strong>Generated only from published evidence.</strong> Recipes cite exact documentation, contract, package and tool revisions; unsupported claims stay visible for review.</span></div>
    <section className="panel"><PanelHeader title="Evidence analysis" description="Analyse the current deployment evidence, resolve blocking unknowns, then generate editable recipe drafts." action={<Button disabled={busy || blockers.length > 0} onClick={onGenerate}><Sparkles data-slot="icon" />{busy ? "Generating…" : latestAnalysis ? "Generate recipes" : "Analyse & generate"}</Button>} />{latestAnalysis ? <div className="analysis-summary"><span className="settings-icon"><Sparkles /></span><span><strong>{latestAnalysis.plan.summary || "Integration evidence analysed"}</strong><small>{latestAnalysis.evidence.length} evidence item{latestAnalysis.evidence.length === 1 ? "" : "s"} · {latestAnalysis.plan.endpoints.length} endpoint{latestAnalysis.plan.endpoints.length === 1 ? "" : "s"} · revision {latestAnalysis.revision}</small></span><Badge color={latestAnalysis.state === "review" ? "green" : latestAnalysis.state === "failed" ? "red" : "amber"}>{latestAnalysis.state}</Badge></div> : <div className="empty-row">No evidence analysis exists yet.</div>}{blockers.map((unknown) => <div className="publish-validation error" key={unknown.id}><span><XCircle /></span><span><strong>Blocking question</strong><small>{unknown.question} · {unknown.why}</small></span></div>)}</section>
    <section className="panel"><PanelHeader title="Recipe review queue" description="Recipes are deployment-level reusable guidance and remain human-reviewed before MCP publication." action={<ConsoleLink path={sectionPath("recipes")} onNavigate={onNavigate} className="entity-back-link">Open full editor</ConsoleLink>} />{recipes.map((recipe) => <div className="provider-row" key={recipe.id}><span className="settings-icon"><BookOpen /></span><span><strong>{recipe.title}</strong><small>{recipe.outcome} · revision {recipe.revision}</small></span><span className="tool-badges">{recipe.needs_attention && <Badge color="red">attention</Badge>}<Badge color={recipe.state === "published" ? "green" : recipe.state === "approved" ? "blue" : recipe.state === "outdated" ? "red" : "amber"}>{recipe.state}</Badge></span></div>)}{recipes.length === 0 && <div className="empty-row">No recipe has been generated from the current evidence.</div>}<ConsoleLink path={sectionPath("recipes")} onNavigate={onNavigate} className="panel-footer-link">Review, edit, approve and publish recipes <ChevronRight /></ConsoleLink></section>
  </div>;
}

function IntegrationDeliveryWorkspace({ integration, identity, distribution, supportRoute, widgets, onManageSupport, onNavigate }: { integration: APIIntegration; identity: APIIdentity | null; distribution: Distribution | null; supportRoute?: APISupportRoute; widgets: APIWidget[]; onManageSupport: () => void; onNavigate: (path: string) => void }) {
  return <div className="integration-tab-content">
    <div className="integration-delivery-grid">
      <section className="panel delivery-channel-card"><span className="delivery-channel-icon"><LockKeyhole /></span><div><h2>Private MCP</h2><p>Authenticated discovery and execution with live vendor grants.</p></div><Badge color={identity?.state === "active" && distribution?.private_mcp_endpoint ? "green" : "amber"}>{identity?.state === "active" && distribution?.private_mcp_endpoint ? "Ready" : "Setup"}</Badge>{distribution?.private_mcp_endpoint && <code>{distribution.private_mcp_endpoint}</code>}{distribution?.agent_setup.private.available && <a className="panel-footer-link" href={distribution.agent_setup.private.url} target="_blank" rel="noreferrer">Open agent setup <ExternalLink /></a>}</section>
      <section className="panel delivery-channel-card"><span className="delivery-channel-icon"><Globe2 /></span><div><h2>Public MCP</h2><p>Optional anonymous discovery for explicitly public evidence only.</p></div><Badge color={distribution?.product.public_mcp_enabled ? "green" : "zinc"}>{distribution?.product.public_mcp_enabled ? "Enabled" : "Off"}</Badge>{distribution?.public_mcp_endpoint && <code>{distribution.public_mcp_endpoint}</code>}<ConsoleLink path={sectionPath("distribution")} onNavigate={onNavigate} className="panel-footer-link">Configure agent access <ChevronRight /></ConsoleLink></section>
      <section className="panel delivery-channel-card"><span className="delivery-channel-icon"><MessageSquareText /></span><div><h2>Widget</h2><p>Origin-bound customer assistant with this API explicitly allowlisted.</p></div><Badge color={widgets.some((widget) => widget.state === "active") ? "green" : widgets.length ? "amber" : "zinc"}>{widgets.some((widget) => widget.state === "active") ? "Active" : widgets.length ? "Draft" : "Not configured"}</Badge>{widgets.map((widget) => <ConsoleLink key={widget.id} path={entityPath("widget", widget.id)} onNavigate={onNavigate} className="delivery-linked-item"><strong>{widget.name}</strong><small>{widget.allowed_origins.join(", ")}</small><ChevronRight /></ConsoleLink>)}<ConsoleLink path={sectionPath("widgets")} onNavigate={onNavigate} className="panel-footer-link">Manage widgets <ChevronRight /></ConsoleLink></section>
      <section className="panel delivery-channel-card"><span className="delivery-channel-icon"><Bug /></span><div><h2>Support delivery</h2><p>Consent, redaction, encrypted retention and bounded backend delivery.</p></div><Badge color={supportRoute?.state === "active" ? "green" : "amber"}>{supportRoute?.state === "active" ? "Ready" : "Setup"}</Badge>{supportRoute && <span className="delivery-linked-item"><strong>{supportRoute.name}</strong><small>{supportRoute.retention_days}-day retention · {supportRoute.bug_reports_enabled ? "bugs" : ""}{supportRoute.bug_reports_enabled && supportRoute.feedback_enabled ? " & " : ""}{supportRoute.feedback_enabled ? "feedback" : ""}</small></span>}<button type="button" className="panel-footer-link" onClick={onManageSupport}>{supportRoute ? "Change support policy" : "Configure support policy"} <ChevronRight /></button></section>
    </div>
    <section className="panel"><PanelHeader title="Channel scope" description="Every channel consumes the same immutable API snapshot; channels never redefine authorization." /><dl className="entity-detail-grid"><div><dt>API</dt><dd>{integration.display_name} {integration.version_key}</dd></div><div><dt>Visibility</dt><dd>{integration.visibility}</dd></div><div><dt>Private identity</dt><dd>{identity?.state ?? "not configured"}</dd></div><div><dt>Widgets</dt><dd>{widgets.length}</dd></div></dl></section>
  </div>;
}

function IntegrationTestWorkspace({ integration, distribution, onNavigate }: { integration: APIIntegration; distribution: Distribution | null; onNavigate: (path: string) => void }) {
  const [preflight, setPreflight] = useState<APIIntegrationPreflight | null>(null);
  const [running, setRunning] = useState(false);
  const [preflightError, setPreflightError] = useState("");

  async function runPreflight() {
    setRunning(true);
    setPreflightError("");
    try {
      setPreflight(await api.preflightIntegration(integration.id));
    } catch (error) {
      setPreflight(null);
      setPreflightError(error instanceof APIError ? error.message : "Server preflight could not run.");
    } finally { setRunning(false); }
  }

  const pathForTab = (tab: string) => tab === "resources" ? integrationPath(integration.id, "resources") : tab === "authorization" || tab === "access" ? integrationPath(integration.id, "authorization") : tab === "tools" ? integrationPath(integration.id, "tools") : tab === "recipes" ? integrationPath(integration.id, "recipes") : integrationPath(integration.id, "delivery");
  const requiredChecks = preflight?.checks.filter((check) => check.required) ?? [];
  const passed = requiredChecks.filter((check) => check.status === "pass").length;
  return <div className="integration-tab-content">
    <div className="notice"><TerminalSquare /><span><strong>Vendor-neutral acceptance suite.</strong> Use the same OAuth + Stateless MCPv2 client against every integration; never special-case a vendor.</span></div>
    <section className="panel"><PanelHeader title="Configuration preflight" description="Server-backed checks over the exact candidate manifest and immutable bindings. This does not impersonate a user or call the vendor backend." action={<Button disabled={running} onClick={() => void runPreflight()}><RefreshCw data-slot="icon" />{running ? "Running…" : "Run preflight"}</Button>} />{preflightError && <div className="publish-validation error"><span><XCircle /></span><span><strong>Preflight failed</strong><small>{preflightError}</small></span></div>}{preflight?.checks.map((check) => { const ready = check.status === "pass"; const optional = check.status === "optional"; return <ConsoleLink key={check.code} path={pathForTab(check.tab)} onNavigate={onNavigate} className="integration-health-check"><span className={`health-icon ${ready ? "ready" : ""}`}>{ready ? <CheckCircle2 /> : optional ? <ShieldCheck /> : <XCircle />}</span><span><strong>{check.label}</strong><small>{check.message}</small></span><Badge color={ready ? "green" : optional ? "zinc" : "red"}>{ready ? "Pass" : optional ? "Optional" : "Missing"}</Badge><ChevronRight /></ConsoleLink>; })}{!preflight && !preflightError && <div className="empty-row">Run preflight to ask the server to verify the current candidate.</div>}<div className="preflight-summary"><span><strong>{preflight ? `${passed}/${requiredChecks.length} required checks pass` : "Server result pending"}</strong><small>{preflight ? `Candidate r${preflight.candidate_revision} · ${preflight.candidate_manifest_hash} · ${new Date(preflight.generated_at).toLocaleString()}` : "No browser-only assumptions are used"}</small></span><Badge color={preflight?.ready ? "green" : "amber"}>{preflight?.ready ? preflight.matches_latest_published ? "Published & ready" : "Ready to publish" : "Action required"}</Badge></div></section>
    <section className="panel"><PanelHeader title="MCP client acceptance" description="Complete these live scenarios with a test tenant before publication." action={<ConsoleLink path={sectionPath("runs")} onNavigate={onNavigate} className="entity-back-link">Open activity</ConsoleLink>} /><ol className="acceptance-scenarios"><li><span>1</span><div><strong>Discover metadata and register a public client</strong><small>RFC 8414, protected-resource metadata, DCR, exact resource and PKCE S256.</small></div></li><li><span>2</span><div><strong>Authorize a real test customer</strong><small>OIDC callback, live vendor access evaluation, one-time code and bound token.</small></div></li><li><span>3</span><div><strong>List resources and tools</strong><small>Only published resources and grant-authorized exact tool revisions appear.</small></div></li><li><span>4</span><div><strong>Exercise positive and negative calls</strong><small>Valid call, missing grant, invalid schema, absent confirmation, revoked access and upstream drift.</small></div></li><li><span>5</span><div><strong>Verify widget and support isolation</strong><small>Origin allowlist, API allowlist, redaction, retention, delivery receipt and audit correlation.</small></div></li></ol>{distribution?.agent_setup.private.available ? <a className="panel-footer-link" href={distribution.agent_setup.private.url} target="_blank" rel="noreferrer">Open private MCP test-client setup <ExternalLink /></a> : <div className="empty-row">Private test-client setup becomes available after customer identity is active.</div>}</section>
  </div>;
}

type IntegrationsViewProps = {
  integrations: APIIntegration[];
  resourceSets: APIResourceSet[];
  sources: Source[];
  supportRoutes: APISupportRoute[];
  connections: APIAccessConnection[];
  tools: APITool[];
  identity: APIIdentity | null;
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  distribution: Distribution | null;
  widgets: APIWidget[];
  selectedIntegrationID?: string;
  activeTab?: IntegrationTab;
  activeResourceTab?: IntegrationResourceTab;
  recipeBusy: boolean;
  onBuild: () => void;
  onAddSource: () => void;
  onCrawlSource: (sourceID: string) => void;
  onPublishSource: (source: Source) => void;
  onGenerateRecipes: (integrationID?: string) => void;
  onChanged: () => Promise<void>;
  onMessage: (message: string) => void;
  onNavigate: (path: string) => void;
};

function IntegrationsView({ integrations, resourceSets, sources, supportRoutes, connections, tools, identity, analyses, recipes, distribution, widgets, selectedIntegrationID, activeTab = "overview", activeResourceTab = "documentation", recipeBusy, onBuild, onAddSource, onCrawlSource, onPublishSource, onGenerateRecipes, onChanged, onMessage, onNavigate }: IntegrationsViewProps) {
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
  const [selectedSourcePublicationIDs, setSelectedSourcePublicationIDs] = useState<string[]>([]);
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
    setFamilyKey(value?.family_key ?? ""); setVersionKey(value?.version_key ?? "v1"); setDisplayName(value?.display_name ?? ""); setDescription(value?.description ?? ""); setIntegrationVisibility(value?.visibility ?? "private"); setIntegrationPublicAcknowledged(false); setLifecycle(value?.lifecycle ?? "draft"); setReplacementID(value?.replacement_integration_id ?? ""); setSunsetAt(value?.sunset_at?.slice(0, 10) ?? "");
    setIntegrationOpen(true);
  }

  async function saveIntegration() {
    setBusy(true);
    try {
      const base = { family_key: editingIntegration ? familyKey : apiFamilyKeyFromName(displayName), version_key: versionKey, display_name: displayName, description: editingIntegration ? description : "", visibility: editingIntegration ? integrationVisibility : "private" as const, acknowledge_public: editingIntegration ? integrationPublicAcknowledged : false, lifecycle: editingIntegration ? lifecycle : "draft" as const };
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
    try {
      const preflight = await api.preflightIntegration(publishCandidate.id);
      if (!preflight.ready) {
        const failed = preflight.checks.find((check) => check.required && check.status !== "pass");
        throw new Error(failed?.message ?? "The server preflight found a required configuration gap.");
      }
      await api.publishIntegration(publishCandidate.id, preflight.candidate_revision, preflight.candidate_manifest_hash);
      await onChanged();
      await refreshSelectedIntegration(publishCandidate.id);
      setPublishCandidate(null);
      onMessage("API published from the exact preflight candidate.");
    } catch (error) { onMessage(error instanceof APIError || error instanceof Error ? error.message : "API could not be published."); } finally { setBusy(false); }
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
    const manifest = value?.latest_revision?.manifest ?? [];
    setEditingSet(value ?? null); setSetKind(value?.kind ?? "documentation"); setSetName(value?.name ?? ""); setResourceDescription(value?.description ?? ""); setSetManifest(JSON.stringify(manifest, null, 2)); setSelectedSourcePublicationIDs(manifest.map((entry) => typeof entry.source_publication_id === "string" ? entry.source_publication_id : "").filter(Boolean)); setResourceOpen(true);
  }

  async function saveResourceSet() {
    setBusy(true);
    try {
      const latestPublicationEntries = sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []);
      const parsedManifest = JSON.parse(setManifest) as unknown;
      if (!Array.isArray(parsedManifest)) throw new Error("Manifest must be a JSON array.");
      const existingEntries = parsedManifest as Array<Record<string, unknown>>;
      const options = new Map([...existingEntries, ...latestPublicationEntries].map((entry) => [String(entry.source_publication_id ?? ""), entry]));
      const manifest = setKind === "documentation" ? selectedSourcePublicationIDs.map((id) => options.get(id)).filter((entry): entry is Record<string, unknown> => Boolean(entry)) : existingEntries;
      if (setKind === "documentation" && manifest.length !== selectedSourcePublicationIDs.length) throw new Error("Every selected documentation publication must still exist.");
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

  let currentResourceManifest: Array<Record<string, unknown>> = [];
  try {
    const parsed = JSON.parse(setManifest) as unknown;
    if (Array.isArray(parsed)) currentResourceManifest = parsed as Array<Record<string, unknown>>;
  } catch {
    currentResourceManifest = [];
  }
  const documentationPublicationOptions = Array.from(new Map([
    ...currentResourceManifest.filter((entry) => typeof entry.source_publication_id === "string"),
    ...sources.flatMap((source) => source.latestPublication ? [{ source_publication_id: source.latestPublication.id, source_id: source.id, revision: source.latestPublication.revision, content_hash: source.latestPublication.content_hash, name: source.name }] : []),
  ].map((entry) => [String(entry.source_publication_id), entry])).values());

  return <>
    {selectedIntegrationID ? <IntegrationWorkspaceView integration={selectedIntegration} integrations={integrations} activeTab={activeTab} activeResourceTab={activeResourceTab} loading={selectedLoading} revisions={selectedRevisions} publishStatus={selectedPublishStatus} identity={identity} resourceSets={resourceSets} sources={sources} connections={connections} supportRoutes={supportRoutes} tools={tools} analyses={analyses} recipes={recipes} distribution={distribution} widgets={widgets} recipeBusy={recipeBusy} busy={busy} onEdit={openIntegration} onPublish={setPublishCandidate} onAttach={openAttachDialog} onCreateResource={() => openResource()} onAddSource={onAddSource} onCrawlSource={onCrawlSource} onPublishSource={onPublishSource} onGenerateRecipes={onGenerateRecipes} onEditResource={openResource} onDuplicateResource={(set) => { setDuplicateSet(set); setDuplicateName(`${set.name} copy`); }} onDetachResource={detachResource} onManageAccess={openAccessDialog} onManageSupport={openSupportDialog} onInspectRevision={setInspectedRevision} onMessage={onMessage} onNavigate={onNavigate} /> : <IntegrationDirectoryView integrations={integrations} connections={connections} supportRoutes={supportRoutes} query={query} onQueryChange={setQuery} onCreate={() => openIntegration()} onBuild={onBuild} onNavigate={onNavigate} />}

    <Dialog
      open={integrationOpen}
      onClose={setIntegrationOpen}
      title={editingIntegration ? "Edit API" : "Add API"}
      description={editingIntegration ? "Update this API’s identity, publishing, and lifecycle settings." : "Create a private draft. You can add documentation, access, and publishing settings next."}
      actions={<><Button outline onClick={() => setIntegrationOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !versionKey.trim() || !displayName.trim() || Boolean(editingIntegration && (!familyKey.trim() || (integrationVisibility === "public" && editingIntegration.visibility !== "public" && !integrationPublicAcknowledged)))} onClick={saveIntegration}>{busy ? "Saving…" : editingIntegration ? "Save changes" : "Create API"}</Button></>}
    >
      <div className="auth-form compact-form">
        {!editingIntegration ? <>
          <label className="auth-field"><span>API name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Ex. Voice API" /></label>
          <label className="auth-field"><span>Version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} placeholder="v1" /><small>Start with v1 unless this API already has a public version.</small></label>
        </> : <>
          <div className="two-fields"><label className="auth-field"><span>API family key</span><input value={familyKey} onChange={(event) => setFamilyKey(event.target.value)} /></label><label className="auth-field"><span>API version</span><input value={versionKey} onChange={(event) => setVersionKey(event.target.value)} /></label></div>
          <label className="auth-field"><span>Display name</span><input value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
          <label className="auth-field"><span>Description</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          <div className="two-fields"><label className="auth-field"><span>Visibility</span><select value={integrationVisibility} onChange={(event) => { setIntegrationVisibility(event.target.value as APIIntegration["visibility"]); setIntegrationPublicAcknowledged(false); }}><option value="private">Private</option><option value="public">Public</option></select><small>Public exposes only the published read-only API manifest.</small></label><label className="auth-field"><span>Lifecycle</span><select value={lifecycle} onChange={(event) => setLifecycle(event.target.value as APIIntegration["lifecycle"])}><option value="draft">Draft</option><option value="active">Active</option><option value="deprecated">Deprecated</option><option value="retired">Retired</option></select></label></div>
          {integrationVisibility === "public" && editingIntegration.visibility !== "public" && <label className="compact-check"><input type="checkbox" checked={integrationPublicAcknowledged} onChange={(event) => setIntegrationPublicAcknowledged(event.target.checked)} /><span>I understand this published API metadata will be anonymously discoverable while Public MCP is enabled.</span></label>}
          <label className="auth-field"><span>Replacement</span><select disabled={lifecycle !== "deprecated" && lifecycle !== "retired"} value={replacementID} onChange={(event) => setReplacementID(event.target.value)}><option value="">None</option>{integrations.filter((value) => value.id !== editingIntegration.id).map((value) => <option key={value.id} value={value.id}>{value.display_name} {value.version_key}</option>)}</select></label>
          {(lifecycle === "deprecated" || lifecycle === "retired") && <label className="auth-field"><span>Sunset date</span><input type="date" value={sunsetAt} onChange={(event) => setSunsetAt(event.target.value)} /></label>}
        </>}
      </div>
    </Dialog>
    <Dialog open={resourceOpen} onClose={setResourceOpen} title={editingSet ? `Create revision for ${editingSet.name}` : "Create reusable resource set"} description="Sets are reusable by explicit attachment. Each save creates immutable content." actions={<><Button outline onClick={() => setResourceOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !setName.trim() || (setKind === "documentation" && selectedSourcePublicationIDs.length === 0)} onClick={saveResourceSet}>{busy ? "Saving…" : editingSet ? "Create revision" : "Create set"}</Button></>}><div className="auth-form compact-form"><div className="two-fields"><label className="auth-field"><span>Kind</span><select disabled={Boolean(editingSet)} value={setKind} onChange={(event) => setSetKind(event.target.value as APIResourceSet["kind"])}><option value="documentation">Documentation</option><option value="api">API contract</option></select></label><label className="auth-field"><span>Name</span><input value={setName} onChange={(event) => setSetName(event.target.value)} /></label></div><label className="auth-field"><span>Description</span><textarea value={resourceDescription} onChange={(event) => setResourceDescription(event.target.value)} /></label>{setKind === "documentation" ? <div className="auth-field"><span>Reviewed source publications</span><div className="catalog-list">{documentationPublicationOptions.map((entry) => { const id = String(entry.source_publication_id); const selected = selectedSourcePublicationIDs.includes(id); return <label className="catalog-tool" key={id}><input type="checkbox" checked={selected} onChange={(event) => setSelectedSourcePublicationIDs((items) => event.target.checked ? [...items, id] : items.filter((value) => value !== id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{String(entry.name ?? entry.source_id)}</strong><code>{id}</code><small>Publication r{String(entry.revision)} · {String(entry.content_hash)}</small></span><Badge color="green">reviewed</Badge></label>; })}{documentationPublicationOptions.length === 0 && <div className="empty-row">Publish a reviewed documentation generation before creating this set.</div>}</div><small>Each selection pins one immutable source-publication revision and content hash. Arbitrary JSON is not accepted for documentation.</small></div> : <label className="auth-field"><span>API contract manifest (JSON array)</span><textarea className="code-input" value={setManifest} onChange={(event) => setSetManifest(event.target.value)} spellCheck={false} /></label>}</div></Dialog>
    <Dialog open={Boolean(duplicateSet)} onClose={(open) => { if (!open) setDuplicateSet(null); }} title="Duplicate resource set" description="Creates an independent copy so later edits do not affect APIs using the original." actions={<><Button outline onClick={() => setDuplicateSet(null)}>Cancel</Button><Button color="indigo" disabled={busy || !duplicateName.trim()} onClick={duplicateResource}>Duplicate</Button></>}><label className="auth-field"><span>New set name</span><input value={duplicateName} onChange={(event) => setDuplicateName(event.target.value)} /></label></Dialog>
    <Dialog open={Boolean(attachIntegration)} onClose={(open) => { if (!open) setAttachIntegration(null); }} title={`Attach resources to ${attachIntegration?.display_name ?? "API"}`} description="Follow latest for deliberate sharing, or pin the current immutable revision." actions={<><Button outline onClick={() => setAttachIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy || !attachSetID} onClick={attachResource}>Attach</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Resource set</span><select value={attachSetID} onChange={(event) => setAttachSetID(event.target.value)}><option value="">Select a set</option>{resourceSets.filter((set) => (!attachKind || set.kind === attachKind) && !(attachIntegration?.resources ?? []).some((link) => link.resource_set_id === set.id)).map((set) => <option key={set.id} value={set.id}>{set.kind === "api" ? "API contract" : "documentation"} · {set.name}</option>)}</select></label><label className="compact-check"><input type="checkbox" checked={pinAttachedSet} onChange={(event) => setPinAttachedSet(event.target.checked)} /><span>Pin the current revision instead of following latest</span></label></div></Dialog>
    <Dialog open={Boolean(accessIntegration)} onClose={(open) => { if (!open) setAccessIntegration(null); }} title={`Access for ${accessIntegration?.display_name ?? "API"}`} description="Choose the service connections this API may use. Credentials remain encrypted and are never copied into the API." actions={<><Button outline onClick={() => setAccessIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveAccessConnections}>{busy ? "Saving…" : "Save connections"}</Button></>}><div className="auth-form compact-form"><fieldset className="catalog-settings-section"><legend>Allowed connections</legend>{connections.map((connection) => <label className="compact-check" key={connection.id}><input type="checkbox" aria-label={`Allow ${connection.name}`} checked={accessSelection.includes(connection.id)} onChange={() => setAccessSelection((values) => values.includes(connection.id) ? values.filter((id) => id !== connection.id) : [...values, connection.id])} /><span><strong>{connection.name}</strong><small>{connection.definition?.name ?? "Service connection"} · {connection.state}</small></span></label>)}{connections.length === 0 && <p className="dialog-empty-copy">No service connections exist yet. Create one in Settings, then return here.</p>}</fieldset></div></Dialog>
    <Dialog open={Boolean(supportIntegration)} onClose={(open) => { if (!open) setSupportIntegration(null); }} title={`Bug reports & feedback for ${supportIntegration?.display_name ?? "API"}`} description="Choose an API-specific policy, or inherit the deployment default." actions={<><Button outline onClick={() => setSupportIntegration(null)}>Cancel</Button><Button color="indigo" disabled={busy} onClick={saveSupportRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Reporting policy</span><select value={supportSelection} onChange={(event) => setSupportSelection(event.target.value)}><option value="">Inherit deployment default</option>{supportRoutes.filter((route) => !route.is_default && route.state === "active").map((route) => <option key={route.id} value={route.id}>{route.name} · {route.retention_days} days</option>)}</select><small>{supportRoutes.find((route) => route.is_default) ? `Current default: ${supportRoutes.find((route) => route.is_default)?.name}` : "No default policy exists. Configure one in Settings."}</small></label></div></Dialog>
    <Dialog open={Boolean(publishCandidate)} onClose={(open) => { if (!open) setPublishCandidate(null); }} title={`Publish ${publishCandidate?.display_name ?? "API"}`} description="Review what changed before creating a new immutable version." actions={<><Button outline onClick={() => setPublishCandidate(null)}>Cancel</Button><Button color="indigo" disabled={busy || !selectedPublishStatus?.ready || !selectedPublishStatus.has_changes} onClick={publishIntegration}>{busy ? "Publishing…" : "Publish"}</Button></>}><div className="publish-review">{selectedPublishStatus?.validations.map((validation) => <div key={validation.code} className={`publish-validation ${validation.level}`}><span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{validation.level}</strong><small>{validation.message}</small></span></div>)}<div className="publish-diff-list">{selectedPublishStatus?.changes.map((change) => <div className="publish-diff" key={change.field}><strong>{change.field}</strong><span><small>Published</small><code>{change.before === undefined ? "—" : JSON.stringify(change.before)}</code></span><ChevronRight /><span><small>Draft</small><code>{change.after === undefined ? "—" : JSON.stringify(change.after)}</code></span></div>)}</div><details className="advanced-details"><summary>Technical details</summary><code>{selectedPublishStatus?.current_manifest_hash ?? "—"}</code></details></div></Dialog>
    <Dialog open={Boolean(inspectedRevision)} onClose={(open) => { if (!open) setInspectedRevision(null); }} title={`Published version r${inspectedRevision?.revision ?? ""}`} description="This immutable technical snapshot is kept for audit and deterministic agent delivery." actions={<Button outline onClick={() => setInspectedRevision(null)}>Close</Button>}><div className="revision-inspector"><dl className="entity-detail-grid"><div><dt>Version ID</dt><dd>{inspectedRevision?.id}</dd></div><div><dt>State</dt><dd>{inspectedRevision?.state}</dd></div><div><dt>Published</dt><dd>{inspectedRevision ? new Date(inspectedRevision.published_at ?? inspectedRevision.created_at).toLocaleString() : "—"}</dd></div><div><dt>Published by</dt><dd>{inspectedRevision?.published_by || "—"}</dd></div><div><dt>Manifest hash</dt><dd><code>{inspectedRevision?.manifest_hash}</code></dd></div></dl><pre className="usage-contract"><code>{JSON.stringify(inspectedRevision?.snapshot ?? {}, null, 2)}</code></pre></div></Dialog>
  </>;
}

function ConnectorReleasesView({ versions, integrations, onConfigure, onNavigate }: { versions: APIProductVersion[]; integrations: APIIntegration[]; onConfigure: () => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Advanced publishing" title="Compatibility snapshots" action={<Button onClick={onConfigure}><Settings data-slot="icon" />Release policy</Button>} /><div className="notice"><GitBranch /><span><strong>API versions stay independent.</strong> A compatibility snapshot can combine Voice API v2 with Face API v3 without changing either API identity.</span></div><div className="metrics-grid"><Metric label="Compatibility snapshots" value={String(versions.length)} detail={`${versions.filter((version) => version.release_stage === "active").length} active`} /><Metric label="APIs" value={String(integrations.length)} detail="Selected by immutable revision" /><Metric label="Latest" value={versions.find((version) => version.is_latest)?.version ?? "—"} detail="Default latest channel" /><Metric label="LTS" value={versions.find((version) => version.is_lts)?.version ?? "—"} detail="Stable channel" /></div><section className="panel"><PanelHeader title="Published snapshots" description="Scoped pins override the default channel." />{versions.map((version) => <div className="provider-row" key={version.id}><span className="settings-icon"><GitBranch /></span><span><EntityLink entity="release" uid={version.id} onNavigate={onNavigate} className="entity-link"><strong>{version.version}</strong></EntityLink><small>{version.profile_name} · {version.manifest_hash}</small></span><span>{version.is_latest && <Badge color="blue">Latest</Badge>} {version.is_lts && <Badge color="violet">LTS</Badge>}</span><Badge color={version.deprecated_at ? "amber" : version.drift_status === "drifted" ? "red" : "green"}>{version.deprecated_at ? "Deprecated" : version.drift_status}</Badge></div>)}{versions.length === 0 && <div className="empty-row">No compatibility snapshots have been published.</div>}</section></>;
}

function AccessView({ definitions, connections, instances, credentials, integrations, environments, apiResourceSets, settingsTab, onChanged, onMessage, onNavigate }: { definitions: APIAccessDefinition[]; connections: APIAccessConnection[]; instances: APIAccessInstance[]; credentials: APIAccessCredential[]; integrations: APIIntegration[]; environments: APIEnvironment[]; apiResourceSets: APIResourceSet[]; settingsTab?: Extract<SettingsTab, "connections">; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
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
    <PageHeading eyebrow={settingsTab ? "Settings" : "Shared configuration"} title="Service connections" action={<Button onClick={() => { setDefinitionID(definitions[0]?.id ?? ""); setEnvironmentID(environments[0]?.id ?? ""); setConnectionOpen(true); }}><KeyRound data-slot="icon" />Connect service</Button>} />
    {settingsTab && <SettingsTabs active={settingsTab} onNavigate={onNavigate} />}
    <section className="panel"><PanelHeader title="Connections" />{connections.map((connection) => { const definition = connection.definition ?? definitions.find((item) => item.id === connection.access_definition_id); const connectionInstances = instances.filter((item) => item.access_connection_id === connection.id); const connectionCredentials = credentials.filter((item) => item.access_connection_id === connection.id); const labels = (connection.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)).filter(Boolean).map((item) => `${item!.display_name} ${item!.version_key}`).join(", "); return <div className="provider-row" key={connection.id}><span className="settings-icon"><KeyRound /></span><span><EntityLink entity="access-connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><small>{definition?.name ?? "Service"} · {labels || "No API attached"}</small></span><Badge color={connection.state === "active" ? "green" : "amber"}>{connection.state}</Badge><span><strong>{definition?.instance_cardinality === "many" ? connectionInstances.length : "1"} {definition?.instance_cardinality === "many" ? definition.instance_label_plural : definition?.instance_label_singular ?? "instance"}</strong><small>{connectionCredentials.length} credential record{connectionCredentials.length === 1 ? "" : "s"}</small></span></div>; })}{connections.length === 0 && <div className="empty-row">No service connections yet. Connect a vendor service to make it available to APIs.</div>}</section>
    <details className="panel advanced-details"><summary>Advanced service setup</summary><div className="advanced-details-body"><PanelHeader title="Service types" action={<Button outline onClick={() => setDefinitionOpen(true)}><Plus data-slot="icon" />New service type</Button>} />{definitions.map((definition) => <div className="lease-row" key={definition.id}><span><EntityLink entity="access-definition" uid={definition.id} onNavigate={onNavigate} className="entity-link"><strong>{definition.name}</strong></EntityLink><small>{definition.instance_cardinality === "many" ? `Multiple ${definition.instance_label_plural}` : `Single ${definition.instance_label_singular}`}</small></span><Badge color={definition.state === "active" ? "green" : "zinc"}>{definition.state}</Badge></div>)}{definitions.length === 0 && <div className="empty-row">No service types are configured.</div>}<PanelHeader className="advanced-subheading" title="Credential records" description="Fingerprints and lifecycle only. Plaintext credentials are never listed." action={<Badge color="violet">{activeCredentials} active</Badge>} />{credentials.slice(0, 12).map((credential) => <div className="lease-row" key={credential.id}><span><strong>{credential.scopes.join(", ") || "Default scope"}</strong><small>{credential.secret_fingerprint.slice(0, 18)}… · {credential.storage_mode}</small></span><Badge color={credential.state === "active" ? "green" : "zinc"}>{credential.state}</Badge></div>)}{credentials.length === 0 && <div className="empty-row">No credential records yet.</div>}</div></details>
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
    <PageHeading eyebrow="Operations" title="Activity" action={<Button onClick={onStart}><Plus data-slot="icon" />Start run</Button>} />
    <SegmentedControl label="Filter activity" items={[{ id: "all", label: "All" }, { id: "runs", label: "Runs", count: runs.length }, { id: "reports", label: "Bug reports & feedback", count: submissions.length }, { id: "audit", label: "Audit", count: events.length }]} value={filter} onChange={setFilter} />
    {analytics && <div className="activity-summary"><strong>Last 30 days</strong><span>{analytics.integration_runs} runs</span><span>{analytics.tool_calls} tool calls</span><span>{analytics.first_pass_rate.toFixed(1)}% first-pass success</span></div>}

    {show("runs") && <section className="panel"><PanelHeader title="API runs" />{runs.map((run) => <div className="root-row run-row" key={run.id}><span className="settings-icon">{run.state === "running" ? <Clock3 /> : run.validated_success ? <CheckCircle2 /> : <XCircle />}</span><span><EntityLink entity="run" uid={run.id} onNavigate={onNavigate} className="entity-link"><strong>{run.requested_outcome}</strong></EntityLink><small>{environmentName(run.environment_id)} · {new Date(run.started_at).toLocaleString()}{run.failure_code ? ` · ${run.failure_code}` : ""}</small></span><Badge color={run.state === "running" ? "blue" : run.validated_success ? "green" : "red"}>{run.state}</Badge>{run.state === "running" ? <span className="run-actions"><Button outline onClick={() => onComplete(run, false)}>Failed</Button><Button color="indigo" onClick={() => onComplete(run, true)}>Validated</Button></span> : <span />}</div>)}{runs.length === 0 && <div className="empty-row">No API runs yet.</div>}</section>}

    {show("reports") && <section className="panel report-inbox"><PanelHeader title="Bug reports & feedback" action={<Badge color="violet">Encrypted at rest</Badge>} /><DataTable label="Bug reports and feedback"><DataTableHeader className="report-columns"><span>Submission</span><span>API</span><span>Delivery</span><span>Actions</span></DataTableHeader>{submissions.map((submission) => <DataTableRow className="report-columns" key={submission.id}><span className="resource-name"><span className="resource-icon">{submission.kind === "bug" ? <Bug /> : <MessageSquareText />}</span><span><EntityLink entity="report" uid={submission.id} onNavigate={onNavigate} className="entity-link"><strong title={submission.summary}>{submission.summary}</strong></EntityLink><small>{submission.kind} · {new Date(submission.created_at).toLocaleString()}</small></span></span><span><strong className="cell-value">{submission.trusted_integration ? `${submission.trusted_integration.display_name} ${submission.trusted_integration.version_key}` : submission.related_tool || "Deployment"}</strong></span><span><Badge color={statusColor(submission.state)}>{submission.state}</Badge><small className="cell-note">{submission.external_id || (submission.attempts ? `${submission.attempts} attempt${submission.attempts === 1 ? "" : "s"}` : "Not delivered")}</small></span><span className="table-actions"><Button outline onClick={() => onView(submission)}>View</Button>{submission.external_url && <a className="report-ticket-link" href={submission.external_url} target="_blank" rel="noreferrer" aria-label="Open external ticket"><ExternalLink /></a>}{(submission.state === "failed" || submission.state === "held") && canRetry(submission) && <Button outline onClick={() => onRetry(submission)}><RefreshCw data-slot="icon" />Retry</Button>}</span></DataTableRow>)}{submissions.length === 0 && <DataTableEmpty columns={4}>Approved bug reports and feedback will appear here.</DataTableEmpty>}</DataTable></section>}

    {show("audit") && <section className="panel"><PanelHeader title="Audit" description="Secrets are never recorded." action={<Badge color="green">Append-only</Badge>} />{events.map((event) => <div className="root-row audit-row compact-audit-row" key={event.id}><span className="settings-icon"><ShieldCheck /></span><span><EntityLink entity="audit-event" uid={event.id} onNavigate={onNavigate} className="entity-link"><strong>{event.action}</strong></EntityLink><small>{event.target_type} · {new Date(event.created_at).toLocaleString()}</small></span><code>{event.actor_id}</code></div>)}{events.length === 0 && <div className="empty-row">Audit activity appears after the first configuration change.</div>}</section>}
  </>;
}

function ReportingView({ routes, integrations, backendConnections, settingsTab, onChanged, onMessage, onNavigate }: { routes: APISupportRoute[]; integrations: APIIntegration[]; backendConnections: APIBackendConnection[]; settingsTab?: Extract<SettingsTab, "reporting">; onChanged: () => Promise<void>; onMessage: (message: string) => void; onNavigate: (path: string) => void }) {
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
    {!settingsTab && <div className="entity-breadcrumb"><ConsoleLink path={sectionPath("settings")} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />Settings</ConsoleLink></div>}
    <PageHeading eyebrow="Settings" title="Bug reports & feedback" action={<Button onClick={() => openRoute()}><Plus data-slot="icon" />New policy</Button>} />
    {settingsTab && <SettingsTabs active={settingsTab} onNavigate={onNavigate} />}
    <section className="panel"><PanelHeader title="Backend connections" description="Service-to-service origins and bearer credentials are independent of customer identity." action={<Button outline onClick={() => openBackend()}><Plus data-slot="icon" />New connection</Button>} />{backendConnections.map((connection) => <div className="provider-row" key={connection.id}><span className="settings-icon"><Server /></span><span><strong>{connection.name}</strong><small>{connection.base_url} · bearer · {connection.credential_fingerprint ? `credential ${connection.credential_fingerprint}` : "no credential"}</small></span><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge><Button outline onClick={() => openBackend(connection)}>Edit</Button></div>)}{backendConnections.length === 0 && <div className="empty-row">Create a backend connection before enabling support delivery.</div>}</section>
    <section className="panel"><PanelHeader title="Delivery policies" action={<Badge color="violet">{routes.length} polic{routes.length === 1 ? "y" : "ies"}</Badge>} />{routes.map((route) => <div className="provider-row" key={route.id}><span className="settings-icon"><MessageSquareText /></span><span><EntityLink entity="support-route" uid={route.id} onNavigate={onNavigate} className="entity-link"><strong>{route.name}</strong></EntityLink><small>{route.is_default ? "Default for all APIs" : (route.integration_ids ?? []).map((id) => integrations.find((item) => item.id === id)?.display_name ?? id).join(", ")}</small></span><span>{route.bug_reports_enabled && <Badge color="blue">Bugs</Badge>} {route.feedback_enabled && <Badge color="violet">Feedback</Badge>}</span><span className="table-actions"><small>{route.bug_reports_enabled || route.feedback_enabled ? backendConnections.find((connection) => connection.id === route.backend_connection_id)?.name ?? "Backend unavailable" : "Delivery disabled"} · {route.retention_days} days</small><Button outline onClick={() => openRoute(route)}>Edit</Button></span></div>)}{routes.length === 0 && <div className="empty-row">Create a default policy to enable bug reports and feedback.</div>}</section>
    <Dialog open={routeOpen} onClose={setRouteOpen} title={editingRoute ? "Edit reporting policy" : "Create reporting policy"} description="Approved submissions are delivered to /v1/support-submissions through a separately authenticated backend connection." actions={<><Button outline onClick={() => setRouteOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !routeName.trim() || (!routeDefault && routeIntegrations.length === 0) || ((routeBugEnabled || routeFeedbackEnabled) && !backendConnections.some((connection) => connection.id === routeBackendID && connection.state === "active"))} onClick={saveRoute}>{busy ? "Saving…" : "Save policy"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Policy name</span><input value={routeName} onChange={(event) => setRouteName(event.target.value)} placeholder="Default reporting" /></label><label className="compact-check"><input type="checkbox" checked={routeDefault} onChange={(event) => setRouteDefault(event.target.checked)} /><span>Use as the default for all APIs</span></label>{!routeDefault && <fieldset className="catalog-settings-section"><legend>Assigned APIs</legend>{integrations.map((integration) => <label className="compact-check" key={integration.id}><input type="checkbox" checked={routeIntegrations.includes(integration.id)} onChange={() => toggleRouteIntegration(integration.id)} /><span>{integration.display_name} {integration.version_key}</span></label>)}</fieldset>}<div className="two-fields"><label className="compact-check"><input type="checkbox" checked={routeBugEnabled} onChange={(event) => setRouteBugEnabled(event.target.checked)} /><span>Enable bug reports</span></label><label className="compact-check"><input type="checkbox" checked={routeFeedbackEnabled} onChange={(event) => setRouteFeedbackEnabled(event.target.checked)} /><span>Enable feedback</span></label></div><label className="auth-field"><span>Backend connection</span><select value={routeBackendID} onChange={(event) => setRouteBackendID(event.target.value)}><option value="">No delivery connection</option>{backendConnections.map((connection) => <option key={connection.id} value={connection.id} disabled={connection.state !== "active"}>{connection.name} · {connection.state}</option>)}</select><small>The route stores only this reference; credentials rotate on the connection.</small></label><label className="auth-field"><span>Encrypted retention (days)</span><input type="number" min={1} max={365} value={routeRetention} onChange={(event) => setRouteRetention(event.target.value)} /></label></div></Dialog>
		<Dialog open={backendOpen} onClose={setBackendOpen} title={editingBackend ? "Edit backend connection" : "Create backend connection"} description="This credential is used only for service-to-service delivery, never for customer access or tool calls." actions={<><Button outline onClick={() => setBackendOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !backendName.trim() || !backendBaseURL.trim() || (backendState === "active" && !backendCredential.trim() && !editingBackend?.credential_fingerprint)} onClick={saveBackend}>{busy ? "Saving…" : "Save connection"}</Button></>}><div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={backendName} onChange={(event) => setBackendName(event.target.value)} placeholder="Support delivery" /></label><label className="auth-field"><span>HTTPS origin</span><input type="url" value={backendBaseURL} onChange={(event) => setBackendBaseURL(event.target.value)} placeholder="https://backend.vendor.com" /><small>DokoSoko appends only /v1/support-submissions.</small></label><div className="two-fields"><label className="auth-field"><span>Authentication</span><input value="Bearer" disabled /></label><label className="auth-field"><span>State</span><select value={backendState} onChange={(event) => setBackendState(event.target.value as APIBackendConnection["state"])}><option value="disabled">Disabled</option><option value="active">Active</option></select></label></div><label className="auth-field"><span>{editingBackend ? "Rotate bearer credential (optional)" : "Bearer credential"}</span><input type="password" autoComplete="off" value={backendCredential} onChange={(event) => setBackendCredential(event.target.value)} /><small>Submitted once, encrypted immediately, and never returned.</small></label></div></Dialog>
  </>;
}


function ToolsWorkspaceTabs({ active, onNavigate }: { active: "catalog" | "connections"; onNavigate: (path: string) => void }) {
  return <PageTabs label="Tool management sections">
    <ConsoleLink path={sectionPath("tools")} onNavigate={onNavigate} className={`page-tab ${active === "catalog" ? "active" : ""}`} ariaCurrent={active === "catalog" ? "page" : undefined}>Catalog</ConsoleLink>
    <ConsoleLink path={sectionPath("connections")} onNavigate={onNavigate} className={`page-tab ${active === "connections" ? "active" : ""}`} ariaCurrent={active === "connections" ? "page" : undefined}>Connections</ConsoleLink>
  </PageTabs>;
}

function MCPConnectionsView({ connections, tools, busy, onAdd, onInspect, onNavigate }: { connections: APIMCPConnection[]; tools: APITool[]; busy: boolean; onAdd: () => void; onInspect: (connection: APIMCPConnection) => void; onNavigate: (path: string) => void }) {
  const imported = tools.filter((tool) => tool.backend_kind === "mcp");
  const delegated = connections.filter((connection) => connection.auth_mode === "delegated_oauth").length;
  const authLabel = (mode: APIMCPConnection["auth_mode"]) => mode === "delegated_oauth" ? "Delegated user OAuth" : mode === "service" ? "Service credential" : "No upstream auth";
  return <>
    <PageHeading eyebrow="Tools" title="Connections" description="Inspect upstream MCP catalogs and import reviewed definitions into the deployment tool catalog." action={<Button onClick={onAdd}><Plus data-slot="icon" />Connect MCP</Button>} />
    <ToolsWorkspaceTabs active="connections" onNavigate={onNavigate} />
    <a className="mcp-policy-banner" href="https://blog.modelcontextprotocol.io/posts/2026-07-28/" target="_blank" rel="noreferrer"><span className="mcp-policy-icon"><ShieldCheck /></span><span><strong>Stateless MCPv2 Only</strong><small>Protocol 2026-07-28 · self-contained requests · no logical live sessions</small></span><ExternalLink /></a>
    <div className="metrics-grid"><Metric label="Upstream connections" value={String(connections.length)} detail="Fixed HTTPS destinations" /><Metric label="Imported tools" value={String(imported.length)} detail={`${imported.filter((tool) => tool.state === "published").length} published`} /><Metric label="Delegated identities" value={String(delegated)} detail="Separate upstream grants" /><Metric label="Drifted schemas" value={String(imported.filter((tool) => tool.upstream_drifted).length)} detail="Published calls fail closed" positive={!imported.some((tool) => tool.upstream_drifted)} /></div>
    <section className="panel mcp-connections-panel">
      <PanelHeader title="Managed upstreams" description="Inspect returns a complete catalog; import always creates or updates local drafts." action={<Badge color="green">Pre-call authz</Badge>} />
      {connections.map((connection) => {
        const connectionTools = imported.filter((tool) => tool.mcp_connection_id === connection.id);
        return <article className="mcp-connection-row" key={connection.id}><span className="connection-mark"><Share2 /></span><span className="connection-main"><span><EntityLink entity="connection" uid={connection.id} onNavigate={onNavigate} className="entity-link"><strong>{connection.name}</strong></EntityLink><Badge color={connection.state === "active" ? "green" : "zinc"}>{connection.state}</Badge></span><code>{connection.endpoint}</code><small>{connection.namespace}.* · {connection.protocol_version} · {authLabel(connection.auth_mode)}</small></span><span className="connection-stat"><strong>{connectionTools.length}</strong><small>imported tools</small></span><span className="connection-stat"><strong>{connection.last_synced_at ? new Date(connection.last_synced_at).toLocaleDateString() : "Never"}</strong><small>last inspected</small></span><Button outline disabled={busy} onClick={() => onInspect(connection)}><RefreshCw data-slot="icon" />Inspect & import</Button></article>;
      })}
      {connections.length === 0 && <div className="empty-row">No upstream MCP is connected. Add one to inspect and review its catalog.</div>}
    </section>
    <div className="identity-flow"><span><LockKeyhole /><strong>1 · DokoSoko identity</strong><small>Authenticate the user and resolve a durable customer account.</small></span><span><ShieldCheck /><strong>2 · Access policy</strong><small>Validate schema, confirmation, grants, and the vendor access evaluation.</small></span><span><Users /><strong>3 · Upstream identity</strong><small>Use a separate user grant or encrypted service credential—never the inbound token.</small></span></div>
  </>;
}

type ToolCatalogFilter = "all" | "published" | "draft" | "drifted" | "retired";

function ToolsView({ tools, integrations, connections, onAdd, onNavigate }: { tools: APITool[]; integrations: APIIntegration[]; connections: APIMCPConnection[]; onAdd: () => void; onNavigate: (path: string) => void }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ToolCatalogFilter>("all");
  const [usageCounts, setUsageCounts] = useState<Record<string, number>>({});
  const [usageStatus, setUsageStatus] = useState<"loading" | "ready" | "partial">("loading");

  useEffect(() => {
    let cancelled = false;
    Promise.all(integrations.map(async (integration) => {
      try { return { bindings: await api.integrationToolBindings(integration.id), failed: false }; }
      catch { return { bindings: [] as APIIntegrationToolBinding[], failed: true }; }
    })).then((results) => {
      if (cancelled) return;
      const next: Record<string, number> = {};
      results.flatMap((result) => result.bindings).forEach((binding) => { next[binding.tool_id] = (next[binding.tool_id] ?? 0) + 1; });
      setUsageCounts(next);
      setUsageStatus(results.some((result) => result.failed) ? "partial" : "ready");
    });
    return () => { cancelled = true; };
  }, [integrations]);

  const normalizedQuery = query.trim().toLowerCase();
  const visibleTools = tools.filter((tool) => {
    const matchesQuery = !normalizedQuery || `${tool.namespace}.${tool.name} ${tool.description} ${tool.backend_kind ?? "http"} ${tool.upstream_tool_name ?? ""}`.toLowerCase().includes(normalizedQuery);
    const matchesFilter = filter === "all" || filter === "drifted" ? filter === "all" || Boolean(tool.upstream_drifted) : tool.state === filter;
    return matchesQuery && matchesFilter;
  });
  const connectionName = (tool: APITool) => connections.find((connection) => connection.id === tool.mcp_connection_id)?.name ?? "MCP upstream";

  return <>
    <PageHeading eyebrow="Capabilities" title="Tools" description="Create reusable agent-facing contracts once, then bind exact published revisions to the APIs that expose them." action={<span className="heading-actions"><Button outline onClick={() => onNavigate(sectionPath("connections"))}><Share2 data-slot="icon" />Import from MCP</Button><Button color="indigo" onClick={onAdd}><Plus data-slot="icon" />Create tool</Button></span>} />
    <ToolsWorkspaceTabs active="catalog" onNavigate={onNavigate} />
    <dl className="compact-metrics tool-catalog-metrics"><div className="compact-metric"><dt>Total</dt><dd><strong>{tools.length}</strong><small>deployment tools</small></dd></div><div className="compact-metric"><dt>Published</dt><dd><strong>{tools.filter((tool) => tool.state === "published").length}</strong><small>eligible to bind</small></dd></div><div className="compact-metric"><dt>Drafts</dt><dd><strong>{tools.filter((tool) => tool.state === "draft").length}</strong><small>editable contracts</small></dd></div><div className="compact-metric"><dt>Drifted</dt><dd><strong>{tools.filter((tool) => tool.upstream_drifted).length}</strong><small>blocked upstreams</small></dd></div></dl>
    <div className="table-toolbar tool-catalog-toolbar"><label className="table-search"><Search /><span className="sr-only">Search tools</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search tools" /></label><SegmentedControl label="Filter tools" items={[{ id: "all", label: "All", count: tools.length }, { id: "published", label: "Published", count: tools.filter((tool) => tool.state === "published").length }, { id: "draft", label: "Drafts", count: tools.filter((tool) => tool.state === "draft").length }, { id: "drifted", label: "Drifted", count: tools.filter((tool) => tool.upstream_drifted).length }, { id: "retired", label: "Retired", count: tools.filter((tool) => tool.state === "retired").length }]} value={filter} onChange={setFilter} /></div>
    <span className="sr-only" role="status" aria-live="polite">{visibleTools.length} tool{visibleTools.length === 1 ? "" : "s"} shown.</span>
    <DataTable label="Deployment tools" className="tool-catalog-table">
      <DataTableHeader className="tool-catalog-columns"><span>Tool</span><span>Source</span><span>Risk &amp; access</span><span>State</span><span>Current APIs</span><span>Open</span></DataTableHeader>
      {visibleTools.map((tool) => {
        const policy = toolPolicy(tool);
        const risk = policy.risk === "critical" || policy.risk === "high" || policy.risk === "medium" ? policy.risk : "low";
        const riskColor = risk === "critical" ? "red" : risk === "high" ? "amber" : risk === "medium" ? "violet" : "zinc";
        return <DataTableRow className={`tool-catalog-columns ${tool.upstream_drifted ? "drifted" : ""}`} key={tool.id}>
          <span className="resource-name tool-catalog-name"><span className="resource-icon">{tool.backend_kind === "mcp" ? <Share2 /> : <TerminalSquare />}</span><span><EntityLink entity="tool" uid={tool.id} onNavigate={onNavigate} className="entity-link"><strong>{tool.namespace}.{tool.name}</strong></EntityLink><small>{tool.description || "No purpose documented"}</small></span></span>
          <span><strong className="cell-value">{tool.backend_kind === "mcp" ? connectionName(tool) : "HTTP"}</strong><small className="cell-note">{tool.backend_kind === "mcp" ? tool.upstream_tool_name : `${tool.http_method} · fixed endpoint`}</small></span>
          <span className="tool-policy-cell"><span className="tool-badges"><Badge color={riskColor}>{risk} risk</Badge>{policy.confirmationRequired && <Badge color="amber">confirmation</Badge>}</span><small className="cell-note">{policy.requiredGrants.join(", ") || "No baseline grants"}</small></span>
          <span className="tool-state-cell"><Badge color={tool.state === "published" ? "green" : tool.state === "retired" ? "zinc" : "amber"}>{tool.state}</Badge><small className="cell-note">revision {tool.revision}</small>{tool.upstream_drifted && <Badge color="red">schema drift</Badge>}</span>
          <span><strong className="cell-value">{usageStatus === "loading" ? "…" : usageStatus === "partial" ? `≥${usageCounts[tool.id] ?? 0}` : usageCounts[tool.id] ?? 0}</strong><small className="cell-note">current API draft{(usageCounts[tool.id] ?? 0) === 1 ? "" : "s"}{usageStatus === "partial" ? " · partial" : ""}</small></span>
          <span className="table-open-cell"><ConsoleLink path={entityPath("tool", tool.id)} onNavigate={onNavigate} className="row-arrow" ariaLabel={`Open ${tool.namespace}.${tool.name}`}><ChevronRight /></ConsoleLink></span>
        </DataTableRow>;
      })}
      {visibleTools.length === 0 && <DataTableEmpty columns={6}><div className="tool-catalog-empty"><span className="entity-missing-icon"><Wrench /></span><div><h2>{tools.length === 0 ? "No tools yet" : "No matching tools"}</h2><p>{tools.length === 0 ? "Create a fixed HTTP tool or import a reviewed MCP definition." : "Change the search or lifecycle filter."}</p></div>{tools.length === 0 && <Button color="indigo" onClick={onAdd}><Plus data-slot="icon" />Create tool</Button>}</div></DataTableEmpty>}
    </DataTable>
  </>;
}


function SettingsTabs({ active, onNavigate }: { active: SettingsTab; onNavigate: (path: string) => void }) {
  return <PageTabs label="Settings sections">{SETTINGS_TABS.map((tab) => <ConsoleLink key={tab.id} path={settingsPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`} ariaCurrent={active === tab.id ? "page" : undefined}>{tab.label}</ConsoleLink>)}</PageTabs>;
}

function RecipesView({ analyses, recipes, busy, onCreate, onEdit, onRework, onApprove, onPublish }: {
  analyses: APIIntegrationAnalysis[];
  recipes: APIRecipe[];
  busy: boolean;
  onCreate: (prompt: string) => Promise<APIRecipe | null>;
  onEdit: (recipe: APIRecipe, markdown: string, references: APIRecipeReference[], visibility: APIRecipe["visibility"]) => void;
  onRework: (recipe: APIRecipe, instruction: string) => void;
  onApprove: (recipe: APIRecipe) => void;
  onPublish: (recipe: APIRecipe) => void;
}) {
  const [createOpen, setCreateOpen] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [instructions, setInstructions] = useState<Record<string, string>>({});
  const [visibilities, setVisibilities] = useState<Record<string, APIRecipe["visibility"]>>({});
  const [referenceSelections, setReferenceSelections] = useState<Record<string, string[]>>({});
  const selected = selectedID ? recipes.find((recipe) => recipe.id === selectedID) ?? null : null;

  async function createFromPrompt() {
    const value = await onCreate(prompt.trim());
    if (!value) return;
    setPrompt("");
    setCreateOpen(false);
    setSelectedID(value.id);
  }

  if (!selected) {
    return <>
      <PageHeading eyebrow="Developer guidance" title="Recipes" action={<Button onClick={() => setCreateOpen(true)}><Plus data-slot="icon" />Add recipe</Button>} />
      <section className="panel recipe-library" aria-label="Recipes">
        {recipes.length > 0 ? <div className="recipe-library-list">
          {recipes.map((recipe) => <button type="button" className="recipe-library-row" key={recipe.id} onClick={() => setSelectedID(recipe.id)}>
            <span className="recipe-library-icon"><BookOpen /></span>
            <span className="recipe-library-copy"><strong>{recipe.title}</strong><small>{recipe.outcome}</small></span>
            <Badge color={recipe.state === "published" ? "green" : recipe.state === "approved" ? "blue" : recipe.state === "outdated" ? "red" : "amber"}>{recipe.state}</Badge>
            <span className="recipe-library-date">{new Date(recipe.updated_at).toLocaleDateString()}</span>
            <ChevronRight />
          </button>)}
        </div> : <div className="recipe-library-empty">
          <span className="empty-icon"><BookOpen /></span>
          <h2>No recipes yet</h2>
          <p>Describe a developer outcome and AI will build a grounded first draft from your documentation, APIs, connectors, and tools.</p>
          <Button onClick={() => setCreateOpen(true)}><Plus data-slot="icon" />Add recipe</Button>
        </div>}
      </section>
      <Dialog
        open={createOpen}
        onClose={setCreateOpen}
        title="Create recipe"
        description="Describe what a developer should accomplish. AI will inspect the product evidence already connected to DokoSoko and create an editable draft."
        actions={<><Button outline onClick={() => setCreateOpen(false)}>Cancel</Button><Button color="indigo" disabled={busy || !prompt.trim()} onClick={createFromPrompt}><Sparkles data-slot="icon" />{busy ? "Building…" : "Build recipe"}</Button></>}
      >
        <label className="auth-field recipe-create-prompt">
          <span>What should this recipe help developers do?</span>
          <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="For example: Connect a customer’s Stripe account, sync invoices, and verify webhook delivery." />
          <small>Uses existing documentation, API definitions, service connections, MCP connectors, and tools as untrusted evidence. Unsupported claims are flagged for review.</small>
        </label>
      </Dialog>
    </>;
  }

  const revision = selected.current_revision;
  const analysis = analyses.find((value) => value.id === selected.analysis_id);
  const availableReferences = (analysis?.evidence ?? []).flatMap((evidence): APIRecipeReference[] => [
    ...(evidence.location?.startsWith("https://") ? [{ label: evidence.label, url: evidence.location, kind: evidence.location.includes("github.com") ? "code" : "documentation", resource_id: evidence.resource_id }] : []),
    ...(evidence.references ?? []),
  ]);
  const currentReferenceIDs = (revision?.references ?? []).map((reference) => reference.resource_id).filter(Boolean) as string[];
  const selectedReferenceIDs = referenceSelections[selected.id] ?? currentReferenceIDs;
  const markdown = drafts[selected.id] ?? revision?.markdown ?? "";
  const visibility = visibilities[selected.id] ?? selected.visibility;
  const references = [
    ...(revision?.references ?? []).filter((reference) => !reference.resource_id),
    ...availableReferences.filter((reference) => reference.resource_id && selectedReferenceIDs.includes(reference.resource_id)).map((reference) => revision?.references.find((current) => current.resource_id === reference.resource_id) ?? reference),
  ];
  const referencesChanged = [...selectedReferenceIDs].sort().join("\u0000") !== [...currentReferenceIDs].sort().join("\u0000");
  const dirty = markdown !== (revision?.markdown ?? "") || visibility !== selected.visibility || referencesChanged;
  const errors = revision?.validation.filter((finding) => finding.level === "error") ?? [];

  return <>
    <button type="button" className="recipe-editor-back" onClick={() => setSelectedID(null)}><ArrowLeft />All recipes</button>
    <PageHeading eyebrow="Recipe editor" title={selected.title} action={<Badge color={selected.state === "published" ? "green" : selected.state === "approved" ? "blue" : selected.state === "outdated" ? "red" : "amber"}>{selected.state}</Badge>} />
    <div className="recipe-editor-layout">
      <section className="panel recipe-document-editor">
        <div className="recipe-editor-toolbar">
          <span><strong>Markdown</strong><small>Revision {revision?.revision ?? "—"} · {revision?.generated_by === "ai" ? "AI generated" : "Human edited"}</small></span>
          <Button disabled={busy || !dirty || !markdown.trim()} onClick={() => onEdit(selected, markdown, references, visibility)}>{busy ? "Saving…" : "Save changes"}</Button>
        </div>
        <textarea className="recipe-markdown-input" aria-label="Recipe Markdown" value={markdown} onChange={(event) => setDrafts((values) => ({ ...values, [selected.id]: event.target.value }))} placeholder="Write the recipe in Markdown…" />
      </section>
      <aside className="recipe-editor-sidebar">
        <section className="panel recipe-editor-panel recipe-ai-rework">
          <label className="auth-field"><span>Ask AI to revise this recipe</span><textarea value={instructions[selected.id] ?? ""} onChange={(event) => setInstructions((values) => ({ ...values, [selected.id]: event.target.value }))} placeholder="Describe the change you want. AI will keep claims grounded in the same evidence." /></label>
          <Button outline disabled={busy || !(instructions[selected.id] ?? "").trim()} onClick={() => onRework(selected, instructions[selected.id])}><Sparkles data-slot="icon" />Create revision</Button>
        </section>
        <section className="panel recipe-editor-panel">
          <h2>Details</h2>
          <label className="auth-field"><span>Visibility</span><select value={visibility} onChange={(event) => setVisibilities((values) => ({ ...values, [selected.id]: event.target.value as APIRecipe["visibility"] }))}><option value="private">Private</option><option value="public">Public</option></select></label>
          <span className="recipe-editor-meta"><small>Stable URI</small><code>{selected.stable_uri}</code></span>
          <span className="recipe-editor-meta"><small>Audience</small><strong>{selected.audience}</strong></span>
        </section>
        <section className="panel recipe-editor-panel">
          <h2>Evidence</h2>
          {availableReferences.length > 0 ? <div className="recipe-editor-references">{availableReferences.map((reference) => <label className="compact-check" key={reference.resource_id ?? reference.url}><input type="checkbox" checked={Boolean(reference.resource_id && selectedReferenceIDs.includes(reference.resource_id))} onChange={() => { if (!reference.resource_id) return; setReferenceSelections((values) => ({ ...values, [selected.id]: selectedReferenceIDs.includes(reference.resource_id!) ? selectedReferenceIDs.filter((id) => id !== reference.resource_id) : [...selectedReferenceIDs, reference.resource_id!] })); }} /><span>{reference.label}<small>{reference.kind}</small></span></label>)}</div> : <p className="recipe-editor-muted">No external links are available. This private recipe remains grounded in {selected.dependencies.length} immutable catalog dependenc{selected.dependencies.length === 1 ? "y" : "ies"} from its analysis.</p>}
        </section>
        {(revision?.review || (revision?.validation.length ?? 0) > 0) && <section className="panel recipe-editor-panel">
          <h2>Review</h2>
          {revision?.review && <p className="recipe-review-summary">{revision.review}</p>}
          {revision?.validation.map((finding) => <div className={`recipe-editor-finding ${finding.level}`} key={`${finding.code}:${finding.message}`}><span>{finding.level === "error" ? <XCircle /> : <TriangleAlert />}</span><span><strong>{finding.code.replaceAll("_", " ")}</strong><small>{finding.message}</small></span></div>)}
        </section>}
        <div className="recipe-editor-actions">
          {selected.state === "review" && <Button disabled={busy || errors.length > 0} onClick={() => onApprove(selected)}>Approve revision</Button>}
          {selected.state === "approved" && <Button color="indigo" disabled={busy} onClick={() => onPublish(selected)}>Publish recipe</Button>}
        </div>
      </aside>
    </div>
  </>;
}

function SettingsView({ product, versions, pins, customerAccounts, identity, aiProfiles, rootUsers, currentUser, onDoctor, onConfigureProduct, onConfigureIdentity, onAddRoot, onRevokeRoot, onNavigate }: { product: APIProduct; versions: APIProductVersion[]; pins: APIProductVersionPin[]; customerAccounts: APICustomerAccount[]; identity: APIIdentity | null; aiProfiles: APIAIWorkloadProfile[]; rootUsers: APIUser[]; currentUser: APIUser | null; onDoctor: () => void; onConfigureProduct: () => void; onConfigureIdentity: () => void; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  const activeRoots = rootUsers.filter((user) => !user.revoked_at);
  return <>
    <PageHeading eyebrow="Settings" title="Settings" action={<Button outline onClick={onDoctor}><Activity data-slot="icon" />Run System Doctor</Button>} />
    <SettingsTabs active="overview" onNavigate={onNavigate} />
    <div className="settings-grid">
      <button type="button" className="settings-button" aria-label="Open Customer identity settings" onClick={() => onNavigate(settingsPath("identity"))}><SettingsCard icon={<Users />} title="Customer identity (optional)" detail={identity ? `OIDC ${identity.state} · ${customerAccounts.length} account${customerAccounts.length === 1 ? "" : "s"}` : "Configure delegated customer identity only when private access is needed"} status={identity ? identity.state : "Optional"} /></button>
      <button type="button" className="settings-button" aria-label="Open Service connections settings" onClick={() => onNavigate(settingsPath("connections"))}><SettingsCard icon={<KeyRound />} title="Service connections" detail="Encrypted vendor credentials shared explicitly with APIs" status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Bug reports and feedback settings" onClick={() => onNavigate(settingsPath("reporting"))}><SettingsCard icon={<MessageSquareText />} title="Bug reports & feedback" detail="Consent-gated reporting policies and secure delivery endpoints" status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Database and storage settings" onClick={() => onNavigate(settingsPath("storage"))}><SettingsCard icon={<Database />} title="Database & storage" detail="PostgreSQL migrations and encrypted local object storage" status="Healthy" /></button>
      <button type="button" className="settings-button" aria-label="Open AI providers settings" onClick={() => onNavigate(settingsPath("ai"))}><SettingsCard icon={<Bot />} title="AI providers" detail={`${aiProfiles.filter((profile) => profile.enabled).length} active workload${aiProfiles.filter((profile) => profile.enabled).length === 1 ? "" : "s"} · one credential per provider`} status="Manage" /></button>
      <button type="button" className="settings-button" aria-label="Open Root access settings" onClick={() => onNavigate(settingsPath("root"))}><SettingsCard icon={<ShieldCheck />} title="Root access" detail={`${activeRoots.length} MFA-protected administrator${activeRoots.length === 1 ? "" : "s"} · append-only audit`} status="Secure" /></button>
    </div>
    <IdentityContractPanel customerAccounts={customerAccounts} identity={identity} onConfigure={onConfigureIdentity} />
    <details className="panel advanced-details"><summary>Advanced publishing</summary><div className="advanced-details-body"><PanelHeader title="Publishing snapshots" action={<Button outline onClick={onConfigureProduct}>Open advanced publishing</Button>} /><div className="activity-summary"><span>{versions.length} published snapshot{versions.length === 1 ? "" : "s"}</span><span>{pins.length} scoped pin{pins.length === 1 ? "" : "s"}</span><span>Default {product.default_version_policy.toUpperCase()}</span></div></div></details>
    <RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} />
  </>;
}

function IdentityContractPanel({ customerAccounts, identity, onConfigure }: { customerAccounts: APICustomerAccount[]; identity: APIIdentity | null; onConfigure: () => void }) {
  return <section className="panel identity-contract"><PanelHeader title="Customer identity contract" description="The optional OIDC organisation claim resolves to a durable internal account. Suspended accounts and a disabled identity provider fail closed immediately." action={<Button onClick={onConfigure}>{identity ? "Configure" : "Get started"}</Button>} /><div className="contract-grid"><span><small>Customer accounts</small><strong>{customerAccounts.length}</strong></span><span><small>Active</small><strong>{customerAccounts.filter((account) => account.state === "active").length}</strong></span><span><small>Access evaluation</small><strong>POST /v1/access/evaluations</strong></span><span><small>Tool identity</small><strong>Delegated user token</strong></span></div><details className="advanced-details inline-advanced"><summary>Identity and API details</summary><div className="contract-grid"><span><small>OIDC issuer</small><code>{identity?.issuer ?? "Not configured"}</code></span><span><small>Organisation claim</small><code>{identity?.organisation_claim || "Not configured"}</code></span><span><small>Installation claim</small><code>{identity?.installation_claim || "Not configured"}</code></span><span><small>Delegated API origin</small><code>{identity?.delegated_api_origin || "Not configured"}</code></span></div></details></section>;
}

function CustomerIdentitySettingsView({ customerAccounts, identity, onConfigure, onNavigate }: { customerAccounts: APICustomerAccount[]; identity: APIIdentity | null; onConfigure: () => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Customer identity" /><SettingsTabs active="identity" onNavigate={onNavigate} /><IdentityContractPanel customerAccounts={customerAccounts} identity={identity} onConfigure={onConfigure} /></>;
}

function RootAccessPanel({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <section className="panel root-management"><PanelHeader title="Root administrators" description="Root access is independent from vendor identities and always requires MFA." action={<Button onClick={onAddRoot}><Plus data-slot="icon" />Add root</Button>} />{rootUsers.map((user) => <div className="root-row" key={user.id}><span className="avatar">{user.display_name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()}</span><span><EntityLink entity="root-user" uid={user.id} onNavigate={onNavigate} className="entity-link"><strong>{user.display_name}</strong></EntityLink><small>{user.email}</small></span><Badge color={user.revoked_at ? "zinc" : "green"}>{user.revoked_at ? "Revoked" : "MFA active"}</Badge>{!user.revoked_at && user.id !== currentUser?.id ? <Button outline onClick={() => onRevokeRoot(user)}>Revoke</Button> : <span />}</div>)}</section>;
}

function RootAccessSettingsView({ rootUsers, currentUser, onAddRoot, onRevokeRoot, onNavigate }: { rootUsers: APIUser[]; currentUser: APIUser | null; onAddRoot: () => void; onRevokeRoot: (user: APIUser) => void; onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Root access" /><SettingsTabs active="root" onNavigate={onNavigate} /><RootAccessPanel rootUsers={rootUsers} currentUser={currentUser} onAddRoot={onAddRoot} onRevokeRoot={onRevokeRoot} onNavigate={onNavigate} /></>;
}

function StorageSettingsView({ onNavigate }: { onNavigate: (path: string) => void }) {
  return <><PageHeading eyebrow="Settings" title="Database & storage" /><SettingsTabs active="storage" onNavigate={onNavigate} /><section className="panel"><PanelHeader title="Storage status" action={<Badge color="green">Healthy</Badge>} /><div className="contract-grid"><span><small>Primary database</small><strong>Connected</strong></span><span><small>Object storage</small><strong>Available</strong></span><span><small>Encryption</small><strong>Active</strong></span><span><small>Schema</small><strong>Current</strong></span></div></section></>;
}

function AISettingsView({ profiles, connections, usage, saving, onSave, onConfigure, onAddProvider, onConnect, onTest, onNavigate }: { profiles: APIAIWorkloadProfile[]; connections: APIAIProviderConnection[]; usage: APIAIProviderUsage[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void; onAddProvider: () => void; onConnect: (provider: APIAIProviderConnection["provider"]) => void; onTest: (connection: APIAIProviderConnection) => void; onNavigate: (path: string) => void }) {
  const primaryConnections = connections.filter((connection) => connection.enabled && !connection.is_backup);

  return <>
    <PageHeading eyebrow="Settings" title="AI providers" action={<Button onClick={onAddProvider}><Plus data-slot="icon" />Add provider</Button>} />
    <SettingsTabs active="ai" onNavigate={onNavigate} />

    <section className="ai-settings-section">
      <SectionHeader title="Workloads" />
      <div className="panel ai-table-panel">
		<Table label="AI workloads" dense className="ai-settings-table ai-workload-table">
		  <colgroup><col className="ai-workload-column" /><col className="ai-provider-column" /><col className="ai-model-column" /><col className="ai-actions-column" /></colgroup>
		  <TableHead><TableRow><TableHeader>Name</TableHeader><TableHeader>Provider</TableHeader><TableHeader>Model</TableHeader><TableHeader className="ai-actions-heading">Actions</TableHeader></TableRow></TableHead>
	          <TableBody>{aiWorkloads.map((workload) => { const profile = profiles.find((item) => item.workload === workload.role); return <AIWorkloadRow key={`${workload.role}:${profile?.revision ?? 0}:${primaryConnections.map((connection) => `${connection.id}-${connection.revision}`).join(",")}`} workload={workload} profile={profile} connections={primaryConnections} saving={saving} onSave={onSave} onConfigure={onConfigure} />; })}</TableBody>
        </Table>
      </div>
    </section>

    <section className="ai-settings-section">
      <SectionHeader title="Providers" action={<Button outline onClick={onAddProvider}><Plus data-slot="icon" />Add provider</Button>} />
      {connections.length === 0 ? <div className="ai-provider-suggestions">
        {aiProviders.filter((provider) => provider.id !== "openai-compatible").map((provider) => <button type="button" key={provider.id} onClick={() => onConnect(provider.id)}><AIProviderLogo provider={provider.id} /><span><strong>Connect {provider.name}</strong><small>{provider.description}</small></span><ChevronRight /></button>)}
      </div> : <div className="panel ai-table-panel">
        <Table label="AI providers" dense className="ai-settings-table ai-provider-table">
		  <colgroup><col className="ai-provider-identity-column" /><col className="ai-used-by-column" /><col className="ai-usage-column" /><col className="ai-errors-column" /><col className="ai-backup-column" /><col className="ai-provider-actions-column" /></colgroup>
		  <TableHead><TableRow><TableHeader>Provider</TableHeader><TableHeader>Used by</TableHeader><TableHeader>Usage</TableHeader><TableHeader>Errors</TableHeader><TableHeader>Backup</TableHeader><TableHeader className="ai-actions-heading">Actions</TableHeader></TableRow></TableHead>
          <TableBody>{connections.map((connection) => {
            const providerUsage = usage.find((value) => value.provider === connection.provider);
            const workloads = profiles.filter((profile) => profile.provider_connection_id === connection.id).map((profile) => aiWorkloads.find((workload) => workload.role === profile.workload)?.name ?? profile.workload);
            const canTest = connection.enabled && (connection.provider !== "openai-compatible" || workloads.length > 0 || connection.is_backup);
            return <TableRow key={connection.id}>
              <TableCell><div className="ai-provider-identity"><AIProviderLogo provider={connection.provider} /><span><strong>{aiProviderLabel(connection.provider)}</strong><small>{connection.managed_by === "environment" ? "Environment managed" : connection.enabled ? "Connected" : "Paused"}</small></span></div></TableCell>
              <TableCell>{workloads.length ? workloads.join(", ") : <span className="ai-table-muted">Not selected</span>}</TableCell>
              <TableCell><strong>{formatAIUsage(providerUsage?.input_tokens ?? 0, providerUsage?.output_tokens ?? 0)}</strong><small className="ai-table-subline">{providerUsage?.calls ?? 0} request{providerUsage?.calls === 1 ? "" : "s"}</small></TableCell>
              <TableCell>{providerUsage?.errors ?? 0}{connection.last_error_code && <small className="ai-table-subline error">{connection.last_error_code}</small>}</TableCell>
              <TableCell>{connection.is_backup ? <Badge color="violet">Backup</Badge> : <button type="button" className="ai-table-link" onClick={() => onConnect(connection.provider)}>Set up</button>}</TableCell>
              <TableCell><div className="ai-table-actions">{canTest && <Button outline onClick={() => onTest(connection)}>Test</Button>}<Button outline onClick={() => onConnect(connection.provider)}>Manage</Button></div></TableCell>
            </TableRow>;
          })}</TableBody>
        </Table>
      </div>}
      {connections.length > 0 && !connections.some((connection) => connection.is_backup) && <p className="ai-backup-hint"><ShieldCheck />Optional: designate one connected provider as a backup for transient outages.</p>}
    </section>
  </>;
}

function AIWorkloadRow({ workload, profile, connections, saving, onSave, onConfigure }: { workload: (typeof aiWorkloads)[number]; profile?: APIAIWorkloadProfile; connections: APIAIProviderConnection[]; saving: boolean; onSave: (role: AIWorkload, connectionID: string, model: string) => Promise<void>; onConfigure: (role: AIWorkload) => void }) {
  const initialConnection = connections.find((connection) => connection.id === profile?.provider_connection_id) ?? connections[0];
  const [connectionID, setConnectionID] = useState(initialConnection?.id ?? "");
  const [model, setModel] = useState(profile?.model ?? (initialConnection ? aiModelDefaults[initialConnection.provider][workload.role] : ""));
	  const selectedConnection = connections.find((connection) => connection.id === connectionID);
  const dirty = connectionID !== (profile?.provider_connection_id ?? "") || model !== (profile?.model ?? "") || !profile?.enabled;
  const modelOptions = selectedConnection ? aiModelOptions[selectedConnection.provider] : [];
  const visibleModels = model && !modelOptions.includes(model) ? [model, ...modelOptions] : modelOptions;
  const Icon = workload.icon;
  return <TableRow>
    <TableCell><div className="ai-workload-name"><span className="settings-icon"><Icon /></span><span><strong>{workload.name}</strong><small>{workload.description}</small></span></div></TableCell>
    <TableCell><Select aria-label={`${workload.name} provider`} disabled={connections.length === 0} value={connectionID} onChange={(event) => { const nextID = event.target.value; const next = connections.find((connection) => connection.id === nextID); setConnectionID(nextID); setModel(next ? aiModelDefaults[next.provider][workload.role] : ""); }}><option value="">Choose provider</option>{connections.map((connection) => <option value={connection.id} key={connection.id}>{aiProviderLabel(connection.provider)}</option>)}</Select></TableCell>
    <TableCell>{selectedConnection?.provider === "openai-compatible" ? <Input aria-label={`${workload.name} model`} value={model} onChange={(event) => setModel(event.target.value)} placeholder="Provider model ID" /> : <Select aria-label={`${workload.name} model`} disabled={!selectedConnection} value={model} onChange={(event) => setModel(event.target.value)}><option value="">Choose model</option>{visibleModels.map((modelID) => <option value={modelID} key={modelID}>{modelID}</option>)}</Select>}</TableCell>
    <TableCell><div className="ai-table-actions"><Button outline disabled={!profile} onClick={() => onConfigure(workload.role)}>Limits</Button><Button color="indigo" disabled={saving || !dirty || !connectionID || !model.trim()} onClick={() => void onSave(workload.role, connectionID, model)}>{saving ? "Saving…" : "Save"}</Button></div></TableCell>
  </TableRow>;
}

function AIProviderLogo({ provider }: { provider: APIAIProviderConnection["provider"] }) {
	  return <span className={`ai-provider-logo ${provider}`} aria-hidden="true">{provider === "openai" ? <OpenAIProviderMark /> : provider === "google" ? <GeminiProviderMark /> : provider === "anthropic" ? <ClaudeProviderMark /> : provider === "digitalocean" ? <DigitalOceanProviderMark /> : provider === "xai" ? <XAIProviderMark /> : provider === "deepseek" ? <DeepSeekProviderMark /> : <Server />}</span>;
}

function DigitalOceanProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="#0080ff"><path d="M12.04 0C5.408-.02.005 5.37.005 11.992h4.638c0-4.923 4.882-8.731 10.064-6.855a6.95 6.95 0 014.147 4.148c1.889 5.177-1.924 10.055-6.84 10.064v-4.61H7.391v4.623h4.61V24c7.86 0 13.967-7.588 11.397-15.83-1.115-3.59-3.985-6.446-7.575-7.575A12.8 12.8 0 0012.039 0zM7.39 19.362H3.828v3.564H7.39zm-3.563 0v-2.978H.85v2.978z" /></svg>;
}

function XAIProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="currentColor"><text x="1.25" y="17.2" fontSize="15.5" fontWeight="700" letterSpacing="-1.4">xAI</text></svg>;
}

function DeepSeekProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="#5786fe"><path d="M23.748 4.651c-.254-.124-.364.113-.512.233-.051.04-.094.09-.137.137-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.155-.708-.311-.955-.65-.172-.24-.219-.509-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.094.172.187.129.323-.082.28-.18.553-.266.833-.055.179-.137.218-.328.14a5.5 5.5 0 01-1.737-1.179c-.857-.828-1.631-1.743-2.597-2.46a12 12 0 00-.689-.47c-.985-.957.13-1.743.387-1.836.27-.098.094-.433-.778-.428-.872.003-1.67.295-2.687.685a3 3 0 01-.465.136 9.6 9.6 0 00-2.883-.101c-1.885.21-3.39 1.1-4.497 2.622C.082 8.776-.231 10.854.152 13.02c.403 2.284 1.568 4.175 3.36 5.653 1.857 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.132-.284 4.994-1.86.47.234.962.328 1.78.398.629.058 1.235-.031 1.705-.129.735-.155.684-.836.418-.961-2.155-1.004-1.682-.595-2.112-.926 1.095-1.295 2.768-3.598 3.284-6.733.05-.346.115-.834.108-1.114-.004-.171.035-.238.23-.257a4.2 4.2 0 001.545-.475c1.397-.763 1.96-2.016 2.093-3.517.02-.23-.004-.467-.247-.588M11.58 18.168c-2.088-1.642-3.101-2.183-3.52-2.16-.39.024-.32.472-.234.763.09.288.207.487.371.74.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.168-1.361-.801-2.5-1.86-3.301-3.306-.775-1.393-1.225-2.888-1.299-4.482-.02-.385.094-.522.477-.592a4.7 4.7 0 011.53-.038c2.131.311 3.946 1.264 5.467 2.774.868.86 1.525 1.887 2.202 2.89.72 1.066 1.494 2.082 2.48 2.915.348.291.626.513.892.677-.802.09-2.14.109-3.055-.615zm1.001-6.44a.306.306 0 01.415-.287.3.3 0 01.113.074.3.3 0 01.086.214c0 .17-.136.307-.308.307a.303.303 0 01-.306-.307" /></svg>;
}

function OpenAIProviderMark() {
	  return <svg viewBox="0 0 24 24" fill="currentColor"><path fillRule="evenodd" d="M9.205 8.658v-2.26c0-.19.072-.333.238-.428l4.543-2.616c.619-.357 1.356-.523 2.117-.523 2.854 0 4.662 2.212 4.662 4.566 0 .167 0 .357-.024.547l-4.71-2.759a.797.797 0 00-.856 0l-5.97 3.473zm10.609 8.8V12.06c0-.333-.143-.57-.429-.737l-5.97-3.473 1.95-1.118a.433.433 0 01.476 0l4.543 2.617c1.309.76 2.189 2.378 2.189 3.948 0 1.808-1.07 3.473-2.76 4.163zM7.802 12.703l-1.95-1.142c-.167-.095-.239-.238-.239-.428V5.899c0-2.545 1.95-4.472 4.591-4.472 1 0 1.927.333 2.712.928L8.23 5.067c-.285.166-.428.404-.428.737v6.898zM12 15.128l-2.795-1.57v-3.33L12 8.658l2.795 1.57v3.33L12 15.128zm1.796 7.23c-1 0-1.927-.332-2.712-.927l4.686-2.712c.285-.166.428-.404.428-.737v-6.898l1.974 1.142c.167.095.238.238.238.428v5.233c0 2.545-1.974 4.472-4.614 4.472zm-5.637-5.303l-4.544-2.617c-1.308-.761-2.188-2.378-2.188-3.948A4.482 4.482 0 014.21 6.327v5.423c0 .333.143.571.428.738l5.947 3.449-1.95 1.118a.432.432 0 01-.476 0zm-.262 3.9c-2.688 0-4.662-2.021-4.662-4.519 0-.19.024-.38.047-.57l4.686 2.71c.286.167.571.167.856 0l5.97-3.448v2.26c0 .19-.07.333-.237.428l-4.543 2.616c-.619.357-1.356.523-2.117.523zm5.899 2.83a5.947 5.947 0 005.827-4.756C22.287 18.339 24 15.84 24 13.296c0-1.665-.713-3.282-1.998-4.448.119-.5.19-.999.19-1.498 0-3.401-2.759-5.947-5.946-5.947-.642 0-1.26.095-1.88.31A5.962 5.962 0 0010.205 0a5.947 5.947 0 00-5.827 4.757C1.713 5.447 0 7.945 0 10.49c0 1.666.713 3.283 1.998 4.448-.119.5-.19 1-.19 1.499 0 3.401 2.759 5.946 5.946 5.946.642 0 1.26-.095 1.88-.309a5.96 5.96 0 004.162 1.713z" /></svg>;
}

function GeminiProviderMark() {
	  return <svg viewBox="0 0 24 24"><path fill="#3186ff" d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" /></svg>;
}

function ClaudeProviderMark() {
	  return <svg viewBox="0 0 24 24"><path fill="#d97757" d="M4.709 15.955l4.72-2.647.08-.23-.08-.128H9.2l-.79-.048-2.698-.073-2.339-.097-2.266-.122-.571-.121L0 11.784l.055-.352.48-.321.686.06 1.52.103 2.278.158 1.652.097 2.449.255h.389l.055-.157-.134-.098-.103-.097-2.358-1.596-2.552-1.688-1.336-.972-.724-.491-.364-.462-.158-1.008.656-.722.881.06.225.061.893.686 1.908 1.476 2.491 1.833.365.304.145-.103.019-.073-.164-.274-1.355-2.446-1.446-2.49-.644-1.032-.17-.619a2.97 2.97 0 01-.104-.729L6.283.134 6.696 0l.996.134.42.364.62 1.414 1.002 2.229 1.555 3.03.456.898.243.832.091.255h.158V9.01l.128-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.584.28.48.685-.067.444-.286 1.851-.559 2.903-.364 1.942h.212l.243-.242.985-1.306 1.652-2.064.73-.82.85-.904.547-.431h1.033l.76 1.129-.34 1.166-1.064 1.347-.881 1.142-1.264 1.7-.79 1.36.073.11.188-.02 2.856-.606 1.543-.28 1.841-.315.833.388.091.395-.328.807-1.969.486-2.309.462-3.439.813-.042.03.049.061 1.549.146.662.036h1.622l3.02.225.79.522.474.638-.079.485-1.215.62-1.64-.389-3.829-.91-1.312-.329h-.182v.11l1.093 1.068 2.006 1.81 2.509 2.33.127.578-.322.455-.34-.049-2.205-1.657-.851-.747-1.926-1.62h-.128v.17l.444.649 2.345 3.521.122 1.08-.17.353-.608.213-.668-.122-1.374-1.925-1.415-2.167-1.143-1.943-.14.08-.674 7.254-.316.37-.729.28-.607-.461-.322-.747.322-1.476.389-1.924.315-1.53.286-1.9.17-.632-.012-.042-.14.018-1.434 1.967-2.18 2.945-1.726 1.845-.414.164-.717-.37.067-.662.401-.589 2.388-3.036 1.44-1.882.93-1.086-.006-.158h-.055L4.132 18.56l-1.13.146-.487-.456.061-.746.231-.243 1.908-1.312-.006.006z" /></svg>;
}

function formatAIUsage(input: number, output: number) {
  const total = input + output;
  if (total >= 1_000_000) return `${(total / 1_000_000).toFixed(total >= 10_000_000 ? 0 : 1)}M tokens`;
  if (total >= 1_000) return `${(total / 1_000).toFixed(total >= 10_000 ? 0 : 1)}K tokens`;
  return `${total} tokens`;
}

function WarningContent({ children }: { children: React.ReactNode }) { return <div className="warning-content"><div className="warning-icon"><TriangleAlert /></div><div>{children}</div></div>; }
function Confirmation({ checked, onChange, children }: { checked: boolean; onChange: (checked: boolean) => void; children: React.ReactNode }) { return <label className="confirmation"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span className="check-box">{checked && <Check />}</span><span>{children}</span></label>; }
function SummaryItem({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) { return <div className="summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>; }
function Metric({ label, value, detail, positive }: { label: string; value: string; detail: string; positive?: boolean }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small className={positive ? "positive" : ""}>{detail}</small></article>; }
function SettingsCard({ icon, title, detail, status }: { icon: React.ReactNode; title: string; detail: string; status: string }) {
  const statusColor: "amber" | "zinc" | "green" = status === "Required" ? "amber" : status === "Manage" ? "zinc" : "green";
  return <span className="panel settings-card"><span className="settings-icon">{icon}</span><span className="settings-card-copy"><span className="settings-card-title">{title}</span><span className="settings-card-detail">{detail}</span></span><Badge color={statusColor}>{status}</Badge></span>;
}
