import type { ConsoleRoute } from "./console-routes";

// This is a read dependency list for the active screen, not an access policy.
// Each endpoint still applies its own authorization and publication checks.
export function consoleDataNeeds(route: ConsoleRoute) {
  const section = route.section;
  const entity = route.kind === "entity" ? route.entity : undefined;
  const integrationTab = route.kind === "entity" && route.entity === "integration" ? route.integrationTab : undefined;
  const settingsTab = route.kind === "section" && section === "settings" ? route.settingsTab ?? "overview" : undefined;
  const identityTab = route.kind === "section" && section === "identity" ? route.identityTab ?? "sign-in" : undefined;
  const tools = section === "tools" || section === "connections" || integrationTab === "tools";
  return {
    integrations: ["product", "documents", "contracts", "sdks", "query-lab", "recipes", "tools"].includes(section) || entity === "resource-set",
    sources: ["documents", "contracts", "sources", "distribution"].includes(section) || entity === "source",
    sourceHistory: section === "sources" || entity === "source",
    tools,
    connections: tools || entity === "connection",
    nativePlugins: section === "tools" && route.kind === "section",
    resourceSets: entity === "resource-set",
    identity: identityTab === "sign-in" || section === "distribution" || section === "mcp-preview" || integrationTab === "overview",
    distribution: section === "distribution" || section === "mcp-preview" || integrationTab === "test",
    reports: section === "reporting" || entity === "report",
    roots: settingsTab === "overview" || settingsTab === "root" || entity === "root-user",
    audit: section === "reporting" || entity === "audit-event" || entity === "tool",
    accounts: identityTab === "customer-accounts",
    aiConfiguration: settingsTab === "overview" || settingsTab === "ai",
    aiContent: section === "recipes",
  };
}
