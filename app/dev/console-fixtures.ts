import type { APICustomerAccount, APIDeployment, APIMCPConnection, APITool } from "../lib/api";

export type FixtureSource = {
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
};

export const fixtureSources: FixtureSource[] = [
  { id: "src_docs", name: "Developer documentation", kind: "Website", location: "docs.acme.dev", visibility: "private", published: true, quarantined: false, crawlState: "synced", pages: 284, lastCrawl: "12 min ago", revision: 1 },
  { id: "src_api", name: "Platform API", kind: "OpenAPI", location: "api/openapi.yaml", visibility: "private", published: false, quarantined: false, crawlState: "review", pages: 94, lastCrawl: "2 hours ago", revision: 1 },
];

export const fixtureTools: APITool[] = [
  { id: "tool_lookup", organisation_id: "org_acme", product_id: "prod_acme", namespace: "accounts", name: "lookup", description: "Look up an account", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "GET", authorization_policy: {}, timeout_ms: 10000, backend_kind: "http" },
  { id: "tool_incidents", organisation_id: "org_acme", product_id: "prod_acme", namespace: "support", name: "create_incident", description: "Create a support incident", input_schema: {}, output_schema: {}, state: "draft", revision: 1, http_method: "MCP", authorization_policy: { required_grants: ["support.write"] }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: "mcp_support", upstream_tool_name: "incidents.create", upstream_schema_hash: "sha256:8f44e6" },
];

export const fixtureDeployment: APIDeployment = { id: "prod_acme", organisation_id: "org_acme", name: "Acme Platform", slug: "acme", description: "Documentation, exact SDK references, runtime credentials, recipes, and reviewed tools through one MCP connector.", feedback_submission_url: "", error_submission_url: "", catalog_revision: 12, public_mcp_enabled: false, revision: 1 };

export const fixtureMCPConnections: APIMCPConnection[] = [{ id: "mcp_support", organisation_id: "org_acme", product_id: "prod_acme", name: "Support operations", namespace: "support", endpoint: "https://mcp.support.example/v2", protocol_version: "2026-07-28", auth_mode: "access_token", state: "active", last_synced_at: "2026-08-19T11:48:00Z", last_catalog_hash: "sha256:48f2a81d", config: {}, revision: 2, created_at: "2026-08-19T11:00:00Z", updated_at: "2026-08-19T11:48:00Z", forward_user_identity: true }];

export const fixtureCustomerAccounts: APICustomerAccount[] = [{ id: "account_contoso", organisation_id: "org_acme", product_id: "prod_acme", issuer: "https://identity.acme.example", external_id: "contoso", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z", last_authenticated_at: "2026-08-19T12:24:00Z" }];

export type ConsoleFixtures = {
  deployment: APIDeployment;
  mcpConnections: APIMCPConnection[];
  sources: FixtureSource[];
  tools: APITool[];
  customerAccounts: APICustomerAccount[];
};

export const consoleFixtures: ConsoleFixtures = {
  deployment: fixtureDeployment,
  mcpConnections: fixtureMCPConnections,
  sources: fixtureSources,
  tools: fixtureTools,
  customerAccounts: fixtureCustomerAccounts,
};
