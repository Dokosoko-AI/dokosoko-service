export type Section = "overview" | "product" | "sources" | "packages" | "projects" | "connections" | "tools" | "releases" | "distribution" | "runs" | "reporting" | "analytics" | "activity" | "settings";

export type EntityKind =
  | "integration"
  | "resource-set"
  | "source"
  | "package"
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
  | { kind: "entity"; section: Section; entity: EntityKind; uid: string; path: string }
  | { kind: "not-found"; section: "overview"; path: string };

export const SECTION_PATHS: Record<Section, string> = {
  overview: "/overview",
  product: "/integrations",
  sources: "/integrations/documentation",
  packages: "/integrations/packages",
  tools: "/integrations/tools",
  connections: "/integrations/hooks-mcp",
  projects: "/access",
  distribution: "/distribution",
  releases: "/distribution/releases",
  runs: "/operations",
  reporting: "/operations/reporting",
  analytics: "/insights",
  activity: "/insights/activity",
  settings: "/settings",
};

const ENTITY_SECTIONS: Record<EntityKind, Section> = {
  integration: "product",
  "resource-set": "product",
  source: "sources",
  package: "packages",
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

export function routeForSection(section: Section): ConsoleRoute {
  return { kind: "section", section, path: sectionPath(section) };
}

export function routeForEntity(entity: EntityKind, uid: string): ConsoleRoute {
  return { kind: "entity", entity, uid, section: ENTITY_SECTIONS[entity], path: entityPath(entity, uid) };
}

export function parseConsolePath(pathname: string): ConsoleRoute {
  const path = normalizePath(pathname);
  if (path === "/") return routeForSection("overview");

  const section = (Object.entries(SECTION_PATHS) as Array<[Section, string]>).find(([, candidate]) => candidate === path)?.[0];
  if (section) return routeForSection(section);

  const match = path.match(/^\/([^/]+)\/([^/]+)$/);
  if (match && ENTITY_PREFIXES.includes(match[1] as EntityKind)) {
    try {
      return routeForEntity(match[1] as EntityKind, decodeURIComponent(match[2]));
    } catch {
      return { kind: "not-found", section: "overview", path };
    }
  }

  return { kind: "not-found", section: "overview", path };
}
