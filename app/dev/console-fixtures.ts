import type { APICustomerAccount, APIDeployment, APIEnvironment, APIMCPConnection, APIProduct, APIProductBuild, APIProductDefinition, APIProductInstallation, APIProductVersion, APIProductVersionDiff, APIProductVersionPin, APITool } from "../lib/api";

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
  { id: "src_examples", name: "SDK examples", kind: "Git repository", location: "acme/sdk-examples", visibility: "private", published: false, quarantined: false, crawlState: "failed", pages: 0, lastCrawl: "1 day ago", revision: 1 },
];

export const fixtureTools: APITool[] = [
  { id: "tool_sandbox", organisation_id: "org_acme", product_id: "prod_acme", namespace: "access", name: "create_sandbox", description: "Create a sandbox", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_credentials", organisation_id: "org_acme", product_id: "prod_acme", namespace: "credentials", name: "issue", description: "Issue credentials", input_schema: {}, output_schema: {}, state: "draft", revision: 1, http_method: "POST", authorization_policy: {}, timeout_ms: 10000 },
  { id: "tool_incidents", organisation_id: "org_acme", product_id: "prod_acme", namespace: "support", name: "create_incident", description: "Create a support incident", input_schema: {}, output_schema: {}, state: "published", revision: 2, http_method: "MCP", authorization_policy: { required_grants: ["support.write"] }, timeout_ms: 10000, backend_kind: "mcp", mcp_connection_id: "mcp_support", upstream_tool_name: "incidents.create", upstream_schema_hash: "sha256:8f44e6" },
];

export const fixtureProduct: APIProduct = { id: "prod_acme", organisation_id: "org_acme", name: "Acme Platform", slug: "acme", description: "Build reliable voice and messaging experiences with versioned APIs, SDKs, documentation, and managed tools.", default_version_policy: "latest", catalog_revision: 12, require_promotion_approval: true, public_mcp_enabled: false, revision: 1 };
export const fixtureDeployment: APIDeployment = { id: fixtureProduct.id, organisation_id: fixtureProduct.organisation_id, name: fixtureProduct.name, slug: fixtureProduct.slug, description: fixtureProduct.description, default_release_policy: fixtureProduct.default_version_policy, catalog_revision: fixtureProduct.catalog_revision, require_promotion_approval: fixtureProduct.require_promotion_approval, public_mcp_enabled: fixtureProduct.public_mcp_enabled, revision: fixtureProduct.revision };
export const fixtureEnvironment: APIEnvironment = { id: "env_prod", organisation_id: "org_acme", product_id: "prod_acme", name: "Production", slug: "production", is_production: true, revision: 1 };
export const fixtureMCPConnections: APIMCPConnection[] = [{ id: "mcp_support", organisation_id: "org_acme", product_id: "prod_acme", name: "Support operations", namespace: "support", endpoint: "https://mcp.support.example/v2", protocol_version: "2026-07-28", auth_mode: "delegated_oauth", oauth_client_id: "dokosoko-production", oauth_issuer: "https://identity.support.example", authorization_url: "https://identity.support.example/oauth/authorize", token_url: "https://identity.support.example/oauth/token", scopes: ["incidents.read", "incidents.write"], state: "active", last_synced_at: "2026-08-19T11:48:00Z", last_catalog_hash: "sha256:48f2a81d", revision: 2 }];

export const fixtureDefinition: APIProductDefinition = {
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

export const fixtureProductBuild: APIProductBuild = {
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

export const fixtureDiff: APIProductVersionDiff = { from_version_id: "version_2026_05", from_version: "2026.5", generated_at: "2026-08-19T12:20:00Z", summary: "1 added, 0 removed, 2 changed", added: [{ kind: "artifact", path: "capability/voice/artifact/tool/voice.calls.transfer", after: "v3" }], removed: [], changed: [{ kind: "artifact", path: "capability/voice/artifact/openapi", before: "v2", after: "v3" }, { kind: "artifact", path: "capability/messages/artifact/docs", before: "v1", after: "v2" }] };
export const fixtureProductVersions: APIProductVersion[] = [
  { id: "version_2026_08", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.8", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:81f4…b9c2", diff: fixtureDiff, release_stage: "active", rollout_percentage: 25, promotion_state: "approved", requested_latest: true, requested_lts: false, approved_by: "root_approver", approved_at: "2026-08-19T12:19:00Z", drift_status: "healthy", drift_details: [], drift_checked_at: "2026-08-19T12:19:30Z", is_latest: true, is_lts: false, revision: 2, published_at: "2026-08-19T12:20:00Z" },
  { id: "version_2026_05", organisation_id: "org_acme", product_id: "prod_acme", version: "2026.5", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:17bd…81a0", diff: { ...fixtureDiff, from_version_id: undefined, from_version: undefined, summary: "Initial product release", added: [], changed: [] }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: true, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: true, revision: 3, published_at: "2026-05-10T09:00:00Z" },
  { id: "version_2025_11", organisation_id: "org_acme", product_id: "prod_acme", version: "2025.11", profile_id: "profile_communications_202608", profile_name: "Voice v3 + Messages v2", definition_revision: 1, manifest_hash: "sha256:02aa…4d31", diff: { ...fixtureDiff, summary: "0 added, 1 removed, 1 changed" }, release_stage: "active", rollout_percentage: 100, promotion_state: "approved", requested_latest: false, requested_lts: false, drift_status: "healthy", drift_details: [], is_latest: false, is_lts: false, deprecated_at: "2026-08-01T00:00:00Z", deprecation_message: "Move to 2026.5 LTS or 2026.8 latest.", replacement_version: "2026.5", sunset_at: "2026-12-01T00:00:00Z", revision: 4, published_at: "2025-11-12T09:00:00Z" },
];

export const fixtureProductPins: APIProductVersionPin[] = [
  { id: "pin_contoso", organisation_id: "org_acme", product_id: "prod_acme", scope: "customer", scope_id: "account_contoso", customer_account_id: "account_contoso", product_version_id: "version_2026_05", product_version: "2026.5", reason: "Production stability window", revision: 1, created_at: "2026-08-19T12:30:00Z", updated_at: "2026-08-19T12:30:00Z" },
];

export const fixtureCustomerAccounts: APICustomerAccount[] = [{ id: "account_contoso", organisation_id: "org_acme", product_id: "prod_acme", issuer: "https://identity.acme.example", external_id: "contoso", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z", last_authenticated_at: "2026-08-19T12:24:00Z" }];
export const fixtureInstallations: APIProductInstallation[] = [{ id: "installation_contoso_voice", organisation_id: "org_acme", product_id: "prod_acme", customer_account_id: "account_contoso", environment_id: "env_prod", external_id: "contoso-voice-prod", name: "Contoso voice production", state: "active", revision: 1, created_at: "2026-08-19T12:24:00Z", updated_at: "2026-08-19T12:24:00Z" }];

export type ConsoleFixtures = {
  deployment: APIDeployment;
  definition: APIProductDefinition;
  diff: APIProductVersionDiff;
  environment: APIEnvironment;
  installations: APIProductInstallation[];
  mcpConnections: APIMCPConnection[];
  productBuild: APIProductBuild;
  productPins: APIProductVersionPin[];
  productVersions: APIProductVersion[];
  sources: FixtureSource[];
  tools: APITool[];
  customerAccounts: APICustomerAccount[];
};

export const consoleFixtures: ConsoleFixtures = {
  deployment: fixtureDeployment,
  definition: fixtureDefinition,
  diff: fixtureDiff,
  environment: fixtureEnvironment,
  installations: fixtureInstallations,
  mcpConnections: fixtureMCPConnections,
  productBuild: fixtureProductBuild,
  productPins: fixtureProductPins,
  productVersions: fixtureProductVersions,
  sources: fixtureSources,
  tools: fixtureTools,
  customerAccounts: fixtureCustomerAccounts,
};
