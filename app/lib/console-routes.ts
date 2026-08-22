export type Section = "product" | "sources" | "projects" | "connections" | "tools" | "releases" | "distribution" | "runs" | "reporting" | "analytics" | "activity" | "settings";

export type IntegrationTab = "overview" | "resources" | "access" | "history";

export type EntityKind =
  | "integration"
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
  | { kind: "section"; section: Section; path: string }
  | { kind: "entity"; section: "product"; entity: "integration"; uid: string; integrationTab: IntegrationTab; path: string }
  | { kind: "entity"; section: Section; entity: Exclude<EntityKind, "integration">; uid: string; path: string }
  | { kind: "not-found"; section: "product"; path: string };

export const INTEGRATION_TABS: Array<{ id: IntegrationTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "resources", label: "Resources" },
  { id: "access", label: "Access" },
  { id: "history", label: "History" },
];

export const SECTION_PATHS: Record<Section, string> = {
  product: "/integrations",
  sources: "/integrations/documentation",
  tools: "/integrations/tools",
  connections: "/integrations/mcp",
  projects: "/access",
  distribution: "/agent-access",
  releases: "/distribution/releases",
  runs: "/activity",
  reporting: "/operations/reporting",
  analytics: "/insights",
  activity: "/insights/activity",
  settings: "/settings",
};

const ENTITY_SECTIONS: Record<EntityKind, Section> = {
  integration: "product",
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
  "audit-event": "activity",
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

export function entityPath(entity: EntityKind, uid: string): string {
  return `/${entity}/${encodeURIComponent(uid)}`;
}

export function integrationPath(uid: string, tab: IntegrationTab = "overview"): string {
  const base = entityPath("integration", uid);
  return tab === "overview" ? base : `${base}/${tab}`;
}

export function routeForSection(section: Section): ConsoleRoute {
  return { kind: "section", section, path: sectionPath(section) };
}

export function routeForEntity(entity: EntityKind, uid: string): ConsoleRoute {
  if (entity === "integration") return routeForIntegration(uid);
  return { kind: "entity", entity: entity as Exclude<EntityKind, "integration">, uid, section: ENTITY_SECTIONS[entity], path: entityPath(entity, uid) };
}

export function routeForIntegration(uid: string, integrationTab: IntegrationTab = "overview"): ConsoleRoute {
  return { kind: "entity", entity: "integration", uid, integrationTab, section: "product", path: integrationPath(uid, integrationTab) };
}

export function parseConsolePath(pathname: string): ConsoleRoute {
  const path = normalizePath(pathname);
	if (path === "/") return routeForSection("product");

  const section = (Object.entries(SECTION_PATHS) as Array<[Section, string]>).find(([, candidate]) => candidate === path)?.[0];
  if (section) return routeForSection(section);

  const integrationMatch = path.match(/^\/integration\/([^/]+)(?:\/([^/]+))?$/);
  if (integrationMatch) {
    const requestedTab = integrationMatch[2] ?? "overview";
    const tab = requestedTab as IntegrationTab;
    if (!INTEGRATION_TABS.some((candidate) => candidate.id === tab)) return { kind: "not-found", section: "product", path };
    try {
      return routeForIntegration(decodeURIComponent(integrationMatch[1]), tab);
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
