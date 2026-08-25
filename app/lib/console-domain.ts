import { APIError } from "./api";
import type {
  APIIntegration,
  APIIntegrationAnalysis,
  APIRecipe,
  APISourcePublication,
  APITool,
} from "./api";

export type Source = {
  id: string;
  name: string;
  kind: string;
  location: string;
  visibility: "private" | "public";
  published: boolean;
  quarantined: boolean;
  crawlState: "draft" | "queued" | "running" | "synced" | "review" | "failed" | "cancelled";
  pages: number;
  lastCrawl: string;
  revision: number;
  latestPublication?: APISourcePublication;
};

const integrationRecipeScopeKind = "integration_scope";

export function analysisMatchesIntegration(analysis: APIIntegrationAnalysis, integrationID?: string) {
  const scopes = analysis.evidence.filter((item) => item.kind === integrationRecipeScopeKind);
  return integrationID ? scopes.some((item) => item.resource_id === integrationID) : scopes.length === 0;
}

export function recipeMatchesIntegration(recipe: APIRecipe, integrationID?: string) {
  const scopes = recipe.dependencies.filter((item) => item.kind === integrationRecipeScopeKind);
  return integrationID ? scopes.some((item) => item.resource_id === integrationID) : scopes.length === 0;
}

export function apiFamilyKeyFromName(value: string) {
  return value.normalize("NFKD").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 63) || "api";
}

export function sourcePublicationManifestEntry(source: Source, publication: APISourcePublication): Record<string, unknown> {
  return {
    source_publication_id: publication.id,
    source_id: source.id,
    revision: publication.revision,
    content_hash: publication.content_hash,
    name: source.name,
  };
}

export function manifestIncludesSourcePublication(manifest: Array<Record<string, unknown>> | undefined, publicationID: string) {
  return Boolean(manifest?.some((entry) => entry.source_publication_id === publicationID));
}

export function integrationIncludesSourcePublication(integration: APIIntegration, publicationID: string) {
  return Boolean(integration.resources?.some((link) => link.kind === "documentation" && manifestIncludesSourcePublication(link.resolved_revision?.manifest, publicationID)));
}

export function toolPolicy(tool: APITool) {
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

export function toolStateLabel(tool: APITool) {
  return `${tool.state[0].toUpperCase()}${tool.state.slice(1)}: Rev ${tool.revision}`;
}

export function unavailableConsoleCapability(error: unknown) {
  return error instanceof APIError && [404, 405, 501].includes(error.status);
}

export function widgetOriginLabel(origin: string): string {
  try { return new URL(origin).host; } catch { return "Invalid origin"; }
}

function escapeEmbedHTML(value: string) {
  return value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character] ?? character);
}

export const agentClients = [
  { id: "codex", name: "Codex", file: "codex.svg" },
  { id: "claude-code", name: "Claude Code", file: "claude-code.svg" },
  { id: "cursor", name: "Cursor", file: "cursor.svg" },
  { id: "opencode", name: "OpenCode", file: "opencode.svg" },
] as const;

export function agentClientAssetURL(setupURL: string, filename: string) {
  let origin = "";
  try {
    origin = new URL(setupURL).origin;
  } catch {
    if (typeof window !== "undefined") origin = window.location.origin;
  }
  return `${origin}/agent-client-icons/${filename}`;
}

export function buildAgentSetupEmbedHTML(tenantName: string, setupURL: string, kind: "public" | "private") {
  const name = escapeEmbedHTML(tenantName);
  const url = escapeEmbedHTML(setupURL);
  const label = kind === "public" ? "Public" : "Private";
  const chipColor = kind === "public" ? "#4338ca" : "#3f3f46";
  const chipBackground = kind === "public" ? "#eef2ff" : "#f4f4f5";
  const clients = agentClients.map((client) => `<img src="${escapeEmbedHTML(agentClientAssetURL(setupURL, client.file))}" alt="${client.name}" title="${client.name}" data-agent-client="${client.id}" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain">`).join("");
  return `<a href="${url}" target="_blank" rel="noopener noreferrer" data-dokosoko-agent-setup="${kind}" aria-label="Connect your agent to ${name} using ${kind} MCP" style="display:inline-flex;align-items:center;gap:10px;min-height:52px;padding:0 18px;border:1px solid #d4d4d8;border-radius:999px;color:#18181b;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.08);font:600 16px/1.2 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont;&quot;Segoe UI&quot;,sans-serif;text-decoration:none"><span>Connect your agent to ${name}</span><span style="padding:4px 8px;border-radius:999px;color:${chipColor};background:${chipBackground};font-size:11px;font-weight:700;letter-spacing:.04em;text-transform:uppercase">${label}</span>${clients}</a>`;
}
