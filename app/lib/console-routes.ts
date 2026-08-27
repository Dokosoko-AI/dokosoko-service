export type Section =
  | "product"
  | "sources"
  | "documents"
  | "contracts"
  | "sdks"
  | "query-lab"
  | "identity"
  | "recipes"
  | "connections"
  | "tools"
  | "mcp-preview"
  | "distribution"
  | "reporting"
  | "settings";

export type IntegrationTab = "overview" | "documentation" | "authorization" | "tools" | "test" | "history";
export type IntegrationResourceTab = "documentation" | "contracts" | "sdks";
export type SettingsTab = "overview" | "tenant" | "configuration" | "ai" | "root";
export type IdentityTab = "sign-in" | "customer-accounts";

export type RouteLabelKey =
  | "routes.quickStart"
  | "routes.resources"
  | "routes.keysAccess"
  | "routes.tools"
  | "routes.test"
  | "routes.history"
  | "routes.documentation"
  | "routes.apiContracts"
  | "routes.sdks"
  | "routes.overview"
  | "routes.tenantSettings"
  | "routes.configuration"
  | "routes.aiConfiguration"
  | "routes.rootAccess";

export type EntityKind =
  | "integration"
  | "resource-set"
  | "source"
  | "tool"
  | "connection"
  | "report"
  | "audit-event"
  | "root-user";

export type ConsoleRoute =
  | { kind: "section"; section: Section; path: string; settingsTab?: SettingsTab; identityTab?: IdentityTab }
  | { kind: "tool-builder"; section: "tools"; uid?: string; integrationID?: string; path: string }
  | { kind: "entity"; section: "product"; entity: "integration"; uid: string; integrationTab: IntegrationTab; integrationResourceTab?: IntegrationResourceTab; path: string }
  | { kind: "entity"; section: Section; entity: Exclude<EntityKind, "integration">; uid: string; path: string }
  | { kind: "not-found"; section: "product"; path: string };

export const INTEGRATION_TABS: Array<{ id: IntegrationTab; label: RouteLabelKey }> = [
  { id: "overview", label: "routes.quickStart" },
  { id: "documentation", label: "routes.resources" },
  { id: "authorization", label: "routes.keysAccess" },
  { id: "tools", label: "routes.tools" },
  { id: "test", label: "routes.test" },
  { id: "history", label: "routes.history" },
];

export const INTEGRATION_PRIMARY_TABS = INTEGRATION_TABS.filter(
  (tab): tab is { id: Exclude<IntegrationTab, "history">; label: RouteLabelKey } => tab.id !== "history",
);

export const INTEGRATION_RESOURCE_TABS: Array<{ id: IntegrationResourceTab; label: RouteLabelKey }> = [
  { id: "documentation", label: "routes.documentation" },
  { id: "contracts", label: "routes.apiContracts" },
  { id: "sdks", label: "routes.sdks" },
];

export const SETTINGS_TABS: Array<{ id: SettingsTab; label: RouteLabelKey }> = [
  { id: "overview", label: "routes.overview" },
  { id: "tenant", label: "routes.tenantSettings" },
  { id: "configuration", label: "routes.configuration" },
  { id: "ai", label: "routes.aiConfiguration" },
  { id: "root", label: "routes.rootAccess" },
];

export const IDENTITY_TABS: Array<{ id: IdentityTab; label: "navigation.customerSignIn" | "navigation.customerAccounts" }> = [
  { id: "sign-in", label: "navigation.customerSignIn" },
  { id: "customer-accounts", label: "navigation.customerAccounts" },
];

export const SECTION_PATHS: Record<Section, string> = {
  product: "/integrations",
  documents: "/developer-assets/documentation/documents",
  contracts: "/developer-assets/api-contracts",
  sdks: "/developer-assets/sdk-packages",
  "query-lab": "/developer-assets/query-lab",
  identity: "/identity",
  recipes: "/recipes",
  sources: "/integrations/documentation",
  tools: "/tools",
  connections: "/tools/connections",
  "mcp-preview": "/tools/preview",
  distribution: "/agent-access",
  reporting: "/operations/outbox",
  settings: "/settings",
};

const ENTITY_SECTIONS: Record<EntityKind, Section> = {
  integration: "product",
  "resource-set": "product",
  source: "sources",
  tool: "tools",
  connection: "connections",
  report: "reporting",
  "audit-event": "reporting",
  "root-user": "settings",
};

const ENTITY_PREFIXES = Object.keys(ENTITY_SECTIONS) as EntityKind[];

function normalizePath(pathname: string): string {
  const path = pathname.split(/[?#]/, 1)[0] || "/";
  if (path === "/") return path;
  return `/${path.replace(/^\/+|\/+$/g, "")}`;
}

export function sectionPath(section: Section): string {
  return SECTION_PATHS[section];
}

export function settingsPath(tab: SettingsTab = "overview"): string {
  return tab === "overview" ? SECTION_PATHS.settings : `${SECTION_PATHS.settings}/${tab}`;
}

export function identityPath(tab: IdentityTab = "sign-in"): string {
  return tab === "sign-in" ? SECTION_PATHS.identity : `${SECTION_PATHS.identity}/${tab}`;
}

export function entityPath(entity: EntityKind, uid: string): string {
  return `/${entity}/${encodeURIComponent(uid)}`;
}

export function toolBuilderPath(uid?: string): string {
  return uid ? `/tools/new/${encodeURIComponent(uid)}` : "/tools/new";
}

export function integrationToolBuilderPath(integrationID: string): string {
  return `/integration/${encodeURIComponent(integrationID)}/tools/new`;
}

export function integrationPath(uid: string, tab: IntegrationTab = "overview", resourceTab?: IntegrationResourceTab): string {
  const base = entityPath("integration", uid);
  if (tab === "overview") return base;
  if (tab === "documentation" && resourceTab && resourceTab !== "documentation") return `${base}/documentation/${resourceTab}`;
  return `${base}/${tab}`;
}

export function integrationValidationPath(uid: string, tab: string): string {
  switch (tab) {
    case "resources": return integrationPath(uid, "documentation");
    case "authorization": return integrationPath(uid, "authorization");
    case "tools": return integrationPath(uid, "tools");
    case "recipes": return sectionPath("recipes");
    case "delivery": return sectionPath("distribution");
    default: return integrationPath(uid);
  }
}

export function routeForSection(section: Section): ConsoleRoute {
  return { kind: "section", section, path: sectionPath(section) };
}

export function routeForEntity(entity: EntityKind, uid: string): ConsoleRoute {
  if (entity === "integration") return routeForIntegration(uid);
  return { kind: "entity", entity: entity as Exclude<EntityKind, "integration">, uid, section: ENTITY_SECTIONS[entity], path: entityPath(entity, uid) };
}

export function routeForIntegration(uid: string, integrationTab: IntegrationTab = "overview", integrationResourceTab?: IntegrationResourceTab): ConsoleRoute {
  const resourceTab = integrationTab === "documentation" ? integrationResourceTab ?? "documentation" : undefined;
  return { kind: "entity", entity: "integration", uid, integrationTab, ...(resourceTab ? { integrationResourceTab: resourceTab } : {}), section: "product", path: integrationPath(uid, integrationTab, resourceTab) };
}

export function parseConsolePath(pathname: string): ConsoleRoute {
  const path = normalizePath(pathname);
  if (path === "/") return routeForSection("product");

  const toolBuilderMatch = path.match(/^\/tools\/new(?:\/([^/]+))?$/);
  if (toolBuilderMatch) {
    try {
      const uid = toolBuilderMatch[1] ? decodeURIComponent(toolBuilderMatch[1]) : undefined;
      return { kind: "tool-builder", section: "tools", ...(uid ? { uid } : {}), path: toolBuilderPath(uid) };
    } catch {
      return { kind: "not-found", section: "product", path };
    }
  }

  const integrationToolBuilderMatch = path.match(/^\/integration\/([^/]+)\/tools\/new$/);
  if (integrationToolBuilderMatch) {
    try {
      const integrationID = decodeURIComponent(integrationToolBuilderMatch[1]);
      return { kind: "tool-builder", section: "tools", integrationID, path: integrationToolBuilderPath(integrationID) };
    } catch {
      return { kind: "not-found", section: "product", path };
    }
  }

  const settingsMatch = path.match(/^\/settings\/([^/]+)$/);
  if (settingsMatch) {
    const tab = settingsMatch[1] as SettingsTab;
    if (!SETTINGS_TABS.some((candidate) => candidate.id === tab) || tab === "overview") return { kind: "not-found", section: "product", path };
    return { kind: "section", section: "settings", settingsTab: tab, path: settingsPath(tab) };
  }

  const identityMatch = path.match(/^\/identity\/([^/]+)$/);
  if (identityMatch) {
    const tab = identityMatch[1] as IdentityTab;
    if (!IDENTITY_TABS.some((candidate) => candidate.id === tab) || tab === "sign-in") return { kind: "not-found", section: "product", path };
    return { kind: "section", section: "identity", identityTab: tab, path: identityPath(tab) };
  }

  const section = (Object.entries(SECTION_PATHS) as Array<[Section, string]>).find(([, candidate]) => candidate === path)?.[0];
  if (section) return routeForSection(section);

  const integrationMatch = path.match(/^\/integration\/([^/]+)(?:\/([^/]+))?(?:\/([^/]+))?$/);
  if (integrationMatch) {
    const tab = (integrationMatch[2] ?? "overview") as IntegrationTab;
    if (!INTEGRATION_TABS.some((candidate) => candidate.id === tab)) return { kind: "not-found", section: "product", path };
    const resourceTab = integrationMatch[3] as IntegrationResourceTab | undefined;
    if (resourceTab && (tab !== "documentation" || !INTEGRATION_RESOURCE_TABS.some((candidate) => candidate.id === resourceTab))) return { kind: "not-found", section: "product", path };
    try {
      return routeForIntegration(decodeURIComponent(integrationMatch[1]), tab, resourceTab);
    } catch {
      return { kind: "not-found", section: "product", path };
    }
  }

  const match = path.match(/^\/([^/]+)\/([^/]+)$/);
  if (match && ENTITY_PREFIXES.includes(match[1] as EntityKind)) {
    try {
      return routeForEntity(match[1] as EntityKind, decodeURIComponent(match[2]));
    } catch {
      return { kind: "not-found", section: "product", path };
    }
  }

  return { kind: "not-found", section: "product", path };
}
