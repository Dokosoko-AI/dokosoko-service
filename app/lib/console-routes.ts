export type Section = "product" | "identity" | "recipes" | "sources" | "projects" | "connections" | "tools" | "releases" | "distribution" | "widgets" | "runs" | "reporting" | "settings";

export type IntegrationTab = "overview" | "documentation" | "access" | "tools" | "test" | "history";
export type IntegrationResourceTab = "documentation" | "contracts" | "packages";
export type SettingsTab = "overview" | "connections" | "reporting" | "storage" | "ai" | "root";

export type EntityKind =
  | "integration"
  | "widget"
  | "resource-set"
  | "source"
  | "tool"
  | "connection"
  | "access-definition"
  | "access-connection"
  | "installation"
  | "release"
  | "run"
  | "support-route"
  | "report"
  | "audit-event"
  | "root-user";

export type ConsoleRoute =
  | { kind: "section"; section: Section; path: string; settingsTab?: SettingsTab }
  | { kind: "tool-builder"; section: "tools"; uid?: string; integrationID?: string; path: string }
  | { kind: "entity"; section: "product"; entity: "integration"; uid: string; integrationTab: IntegrationTab; integrationResourceTab?: IntegrationResourceTab; path: string }
  | { kind: "entity"; section: Section; entity: Exclude<EntityKind, "integration">; uid: string; path: string }
  | { kind: "not-found"; section: "product"; path: string };

export const INTEGRATION_TABS: Array<{ id: IntegrationTab; label: string }> = [
  { id: "overview", label: "Quick Start" },
  { id: "documentation", label: "Documentation" },
  { id: "access", label: "Access" },
  { id: "tools", label: "Tools" },
  { id: "test", label: "Test" },
  { id: "history", label: "History" },
];

export const INTEGRATION_PRIMARY_TABS = INTEGRATION_TABS.filter(
  (tab): tab is { id: Exclude<IntegrationTab, "history">; label: string } => tab.id !== "history",
);

export const INTEGRATION_RESOURCE_TABS: Array<{ id: IntegrationResourceTab; label: string }> = [
  { id: "documentation", label: "Documentation" },
  { id: "contracts", label: "API contracts" },
  { id: "packages", label: "SDKs & Packages" },
];

export const SETTINGS_TABS: Array<{ id: SettingsTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "connections", label: "Service connections" },
  { id: "reporting", label: "Bug reports & feedback" },
  { id: "storage", label: "Database & storage" },
  { id: "ai", label: "AI providers" },
  { id: "root", label: "Root access" },
];

export const SECTION_PATHS: Record<Section, string> = {
  product: "/integrations",
  identity: "/identity",
  recipes: "/recipes",
  sources: "/integrations/documentation",
  tools: "/tools",
  connections: "/tools/connections",
  projects: "/access",
  distribution: "/agent-access",
  widgets: "/widgets",
  releases: "/distribution/releases",
  runs: "/activity",
  reporting: "/operations/reporting",
  settings: "/settings",
};

const ENTITY_SECTIONS: Record<EntityKind, Section> = {
  integration: "product",
  widget: "widgets",
  "resource-set": "product",
  source: "sources",
  tool: "tools",
  connection: "connections",
  "access-definition": "projects",
  "access-connection": "projects",
  installation: "projects",
  release: "releases",
  run: "runs",
  "support-route": "reporting",
  report: "reporting",
  "audit-event": "runs",
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
    case "authorization":
    case "access": return integrationPath(uid, "access");
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

  const section = (Object.entries(SECTION_PATHS) as Array<[Section, string]>).find(([, candidate]) => candidate === path)?.[0];
  if (section) return routeForSection(section);

	const integrationMatch = path.match(/^\/integration\/([^/]+)(?:\/([^/]+))?(?:\/([^/]+))?$/);
  if (integrationMatch) {
    const requestedTab = integrationMatch[2] ?? "overview";
    const tab = requestedTab as IntegrationTab;
    if (!INTEGRATION_TABS.some((candidate) => candidate.id === tab)) return { kind: "not-found", section: "product", path };
    const requestedResourceTab = integrationMatch[3];
    if (requestedResourceTab && tab !== "documentation") return { kind: "not-found", section: "product", path };
    const resourceTab = requestedResourceTab as IntegrationResourceTab | undefined;
    if (resourceTab && !INTEGRATION_RESOURCE_TABS.some((candidate) => candidate.id === resourceTab)) return { kind: "not-found", section: "product", path };
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
