import type { TFunction } from "i18next";

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
const recipeAnalysisRunningTimeoutMS = 5 * 60 * 1000;

function recipeDependencyScopeIDs(recipe: APIRecipe) {
  return recipe.dependencies
    .filter((item) => item.kind === integrationRecipeScopeKind)
    .map((item) => item.resource_id);
}

export function analysisMatchesIntegration(analysis: APIIntegrationAnalysis, integrationID?: string) {
  const scopes = analysis.evidence.filter((item) => item.kind === integrationRecipeScopeKind);
  return integrationID ? scopes.length === 1 && scopes[0].resource_id === integrationID : scopes.length === 0;
}

export function recipeMatchesIntegration(recipe: APIRecipe, integrationID?: string) {
  const scopes = recipeScopeIDs(recipe);
  if (recipe.contract_version === "deployment-recipe-v3") return integrationID ? scopes.includes(integrationID) : true;
  return integrationID ? scopes.length === 1 && scopes[0] === integrationID : scopes.length === 0;
}

export function recipeScopeIDs(recipe: APIRecipe) {
  if (recipe.contract_version === "product-integration-v2") {
    const integrationID = recipe.integration_id?.trim();
    return integrationID ? [integrationID] : [];
  }
  if (recipe.contract_version === "deployment-recipe-v3") {
    return [...new Set((recipe.api_attachments ?? []).map((item) => item.integration_id.trim()).filter(Boolean))].sort();
  }
  return recipeDependencyScopeIDs(recipe);
}

export function recipeHasScopeDependencyMismatch(recipe: APIRecipe) {
  const scopeIDs = recipeScopeIDs(recipe);
  const revision = recipe.current_revision;
  if (!revision) return true;
  if (recipe.contract_version === "product-integration-v2") {
    const dependencyIDs = recipeDependencyScopeIDs(recipe);
    return revision.spec_version !== 2 || revision.spec.integration_id !== scopeIDs[0] || dependencyIDs.length !== 1 || dependencyIDs[0] !== scopeIDs[0];
  }
  if (recipe.contract_version !== "deployment-recipe-v3" || revision.spec_version !== 3) return true;
  const specIDs = [...new Set((revision.spec.api_attachments ?? []).map((item) => item.integration_id.trim()).filter(Boolean))].sort();
  const bindingIDs = [...new Set((revision.api_bindings ?? []).map((item) => item.integration_id.trim()).filter(Boolean))].sort();
  return scopeIDs.length === 0 || scopeIDs.join("\u0000") !== specIDs.join("\u0000") || scopeIDs.join("\u0000") !== bindingIDs.join("\u0000");
}

export function activeRecipeIntegrationID(integrations: APIIntegration[], selectedIntegrationID: string) {
  if (integrations.some((integration) => integration.id === selectedIntegrationID)) return selectedIntegrationID;
  return integrations.length === 1 ? integrations[0].id : "";
}

export function recipeAnalysisIsFreshlyRunning(analysis: APIIntegrationAnalysis | undefined, now = Date.now()) {
  if (analysis?.state !== "running") return false;
  const startedAt = Date.parse(analysis.created_at);
  return Number.isFinite(startedAt) && now - startedAt <= recipeAnalysisRunningTimeoutMS;
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

export function toolStateLabel(tool: APITool, t: TFunction) {
  const state = tool.state === "draft" ? t("enumLabels.draft") : tool.state === "published" ? t("enumLabels.published") : tool.state === "retired" ? t("enumLabels.retired") : tool.state;
  return t("settings.toolStateRevision", { state: String(state), revision: tool.revision });
}

export function unavailableConsoleCapability(error: unknown) {
  return error instanceof APIError && [404, 405, 501].includes(error.status);
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

export function agentSetupButtonScriptURL(setupURL: string) {
  try {
    return new URL("/agent-setup/button.js", setupURL).toString();
  } catch {
    if (typeof window !== "undefined") return new URL("/agent-setup/button.js", window.location.origin).toString();
  }
  return "/agent-setup/button.js";
}

export function buildAgentSetupEmbedCode(setupURL: string, kind: "public" | "private") {
  const scriptURL = escapeEmbedHTML(agentSetupButtonScriptURL(setupURL));
  return `<script async src="${scriptURL}"></script>\n<dokosoko-mcp-button kind="${kind}" lang="auto"></dokosoko-mcp-button>`;
}

export function buildAgentSetupEmbedHTML(setupURL: string, kind: "public" | "private", copy: { deploymentName: string; kindLabel: string; connectLabel: string; ariaLabel: string }) {
  const url = escapeEmbedHTML(setupURL);
  const name = escapeEmbedHTML(copy.deploymentName);
  const connectLabel = escapeEmbedHTML(copy.connectLabel);
  const ariaLabel = escapeEmbedHTML(copy.connectLabel);
  const clients = agentClients.map((client) => `<img src="${escapeEmbedHTML(agentClientAssetURL(setupURL, client.file))}" alt="${client.name}" title="${client.name}" data-agent-client="${client.id}" referrerpolicy="no-referrer" width="25" height="25" style="display:block;width:25px;height:25px;object-fit:contain">`).join("");
  return `<a href="${url}" target="_blank" rel="noopener noreferrer" data-dokosoko-agent-setup="${kind}" data-dokosoko-deployment="${name}" aria-label="${ariaLabel}" style="display:inline-flex;align-items:center;gap:10px;min-height:52px;padding:0 18px;border:1px solid #d4d4d8;border-radius:999px;color:#18181b;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.08);font:600 16px/1.2 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont;&quot;Segoe UI&quot;,sans-serif;text-decoration:none"><span>${connectLabel}</span>${clients}</a>`;
}
