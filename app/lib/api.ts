export type APIVisibility = "private" | "public";

export type APIProduct = {
  id: string;
  organisation_id: string;
  name: string;
  slug: string;
  description: string;
  default_version_policy: "latest" | "lts";
  catalog_revision: number;
  require_promotion_approval: boolean;
  public_mcp_enabled: boolean;
  revision: number;
};

export type APIDeployment = {
  id: string;
  organisation_id: string;
  name: string;
  slug: string;
  description: string;
  default_release_policy: "latest" | "lts";
  catalog_revision: number;
  require_promotion_approval: boolean;
  public_mcp_enabled: boolean;
  revision: number;
};

export type APIResourceSetRevision = {
  id: string;
  resource_set_id: string;
  revision: number;
  manifest: Array<Record<string, unknown>>;
  content_hash: string;
  created_by?: string;
  created_at: string;
};

export type APIIntegrationResourceLink = {
  id: string;
  integration_id: string;
  resource_set_id: string;
  kind: "documentation" | "api";
  name: string;
  follow_latest: boolean;
  pinned_revision_id?: string;
  resolved_revision?: APIResourceSetRevision;
};

export type APIIntegration = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  family_key: string;
  version_key: string;
  display_name: string;
  description: string;
  visibility: APIVisibility;
  lifecycle: "draft" | "active" | "deprecated" | "retired";
  replacement_integration_id?: string;
  sunset_at?: string;
  revision: number;
  resources?: APIIntegrationResourceLink[];
  access_connection_ids?: string[];
  support_route_id?: string;
};

export type APIIntegrationRevision = {
  id: string;
  integration_id: string;
  revision: number;
  state: string;
  snapshot: Record<string, unknown>;
  manifest_hash: string;
  published_by?: string;
  published_at?: string;
  created_at: string;
};

export type APIIntegrationPublishChange = {
  field: string;
  before?: unknown;
  after?: unknown;
};

export type APIIntegrationPublishValidation = {
  level: "warning" | "error";
  code: string;
  message: string;
  tab: string;
};

export type APIIntegrationPublishStatus = {
  ready: boolean;
  has_changes: boolean;
  current_manifest_hash: string;
  current_snapshot: Record<string, unknown>;
  latest_revision?: APIIntegrationRevision;
  changes: APIIntegrationPublishChange[];
  validations: APIIntegrationPublishValidation[];
};

export type APIIntegrationDetail = {
  integration: APIIntegration;
  revisions: APIIntegrationRevision[];
  publish_status: APIIntegrationPublishStatus;
};

export type APIWidgetAppearance = {
  theme: "auto" | "light" | "dark";
  accentColour?: string;
  launcherPosition: "left" | "right";
  greeting?: string;
};

export type APIWidget = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  name: string;
  state: "draft" | "active" | "disabled";
  allowed_origins: string[];
  integration_ids: string[];
  appearance: APIWidgetAppearance;
  revision: number;
  activated_at?: string;
  created_at: string;
  updated_at: string;
};

export type APIWidgetSecret = {
  id: string;
  widget_id: string;
  fingerprint: string;
  last_used_at?: string;
  revoked_at?: string;
  created_at: string;
};

export type APIWidgetSession = {
  id: string;
  widget_id: string;
  user_id: string;
  customer_organisation_id?: string;
  origin: string;
  expires_at: string;
  revoked_at?: string;
  created_at: string;
  last_seen_at?: string;
};

export type APIWidgetProvisioning = { widget: APIWidget; secret: string };
export type APIWidgetSecretProvisioning = { credential: APIWidgetSecret; secret: string };
export type APIWidgetInput = {
  name: string;
  allowed_origins: string[];
  integration_ids: string[];
  appearance: { theme: APIWidgetAppearance["theme"]; accent_colour?: string; launcher_position: APIWidgetAppearance["launcherPosition"]; greeting?: string };
};

export type APIResourceSet = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  kind: "documentation" | "api";
  name: string;
  description: string;
  state: "active" | "archived";
  revision: number;
  latest_revision?: APIResourceSetRevision;
  integration_ids?: string[];
};

export type APIAccessDefinition = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  service_key: string;
  name: string;
  instance_cardinality: "one" | "many";
  instance_label_singular: string;
  instance_label_plural: string;
  credential_scope: "connection" | "instance";
  management_auth_type: "none" | "bearer" | "api_key" | "oauth2_client_credentials";
  api_resource_set_id?: string;
  operations: Record<string, unknown>;
  state: "draft" | "active" | "archived";
  revision: number;
};

export type APIAccessConnection = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  access_definition_id: string;
  environment_id?: string;
  name: string;
  region?: string;
  config: Record<string, unknown>;
  state: "active" | "disabled" | "error";
  revision: number;
  definition?: APIAccessDefinition;
  integration_ids?: string[];
};

export type APIAccessInstance = {
  id: string;
  access_connection_id: string;
  environment_id: string;
  owner_type: "organisation" | "user" | "installation";
  external_id: string;
  display_name: string;
  state: string;
  integration_ids?: string[];
  expires_at?: string;
};

export type APIAccessCredential = {
  id: string;
  access_connection_id: string;
  access_instance_id?: string;
  environment_id: string;
  scopes: string[];
  secret_fingerprint: string;
  storage_mode: "one_time" | "managed" | "reference";
  state: "active" | "retiring" | "revoked" | "expired";
  expires_at?: string;
};

export type APISupportRoute = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  name: string;
  is_default: boolean;
  bug_reports_enabled: boolean;
  feedback_enabled: boolean;
  backend_connection_id?: string;
  retention_days: number;
  state: "active" | "archived";
  revision: number;
  integration_ids?: string[];
};

export type APIBackendConnection = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  name: string;
  base_url: string;
  authentication_type: "bearer";
  credential_fingerprint?: string;
  state: "active" | "disabled";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIBackendConnectionCredential = {
  id: string;
  backend_connection_id: string;
  fingerprint: string;
  connection_revision: number;
  created_at: string;
};

export type APIProductVersion = {
  id: string;
  organisation_id: string;
  product_id: string;
  version: string;
  profile_id: string;
  profile_name: string;
  definition_revision: number;
  manifest_hash: string;
  diff: APIProductVersionDiff;
  release_stage: "preview" | "active";
  rollout_percentage: number;
  promotion_state: "not_required" | "pending" | "approved" | "rejected";
  promotion_note?: string;
  requested_latest: boolean;
  requested_lts: boolean;
  publisher_actor_id?: string;
  promotion_requested_by?: string;
  approved_by?: string;
  approved_at?: string;
  drift_status: "unchecked" | "healthy" | "drifted";
  drift_details: APIProductArtifactDrift[];
  drift_checked_at?: string;
  is_latest: boolean;
  is_lts: boolean;
  deprecated_at?: string;
  deprecation_message?: string;
  replacement_version?: string;
  sunset_at?: string;
  revision: number;
  published_at: string;
};

export type APIProductVersionChange = { kind: string; path: string; before?: string; after?: string };
export type APIProductVersionDiff = {
  from_version_id?: string;
  from_version?: string;
  generated_at: string;
  summary: string;
  added: APIProductVersionChange[];
  removed: APIProductVersionChange[];
  changed: APIProductVersionChange[];
};
export type APIProductArtifactDrift = { kind: string; reference_id?: string; name: string; expected?: string; observed?: string; status: string; message: string };

export type APIProductVersionPin = {
  id: string;
  organisation_id: string;
  product_id: string;
  scope: "customer" | "environment" | "installation";
  scope_id: string;
  customer_account_id: string;
  environment_id?: string;
  installation_id?: string;
  product_version_id: string;
  product_version: string;
  reason?: string;
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIProductInstallation = {
  id: string;
  organisation_id: string;
  product_id: string;
  customer_account_id: string;
  environment_id: string;
  external_id: string;
  name: string;
  state: "active" | "paused";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIProductVersionPinHistory = {
  id: string;
  pin_id: string;
  scope: "customer" | "environment" | "installation";
  scope_id: string;
  prior_version?: string;
  product_version?: string;
  action: "created" | "updated" | "deleted";
  reason?: string;
  actor_id: string;
  created_at: string;
};

export type APIProductVersionImpact = {
  product_version_id: string;
  product_version: string;
  customer_pins: number;
  environment_pins: number;
  installation_pins: number;
  affected_customers: string[];
  affected_environments: string[];
  affected_installations: string[];
  requests_30_days: number;
  tool_calls_30_days: number;
};

export type APIProductBinding = {
  id: string;
  kind: "openapi" | "docs" | "git" | "mcp" | "tool";
  name: string;
  reference_id?: string;
  location?: string;
  version?: string;
  scope: "product" | "component" | "api_release";
  confidence: number;
  evidence: string[];
  verified: boolean;
};

export type APIProductRelease = {
  id: string;
  version: string;
  state: "draft" | "published";
  bindings: APIProductBinding[];
};

export type APIProductComponent = {
  id: string;
  kind: "api";
  name: string;
  slug: string;
  description?: string;
  version_strategy: "independent";
  releases: APIProductRelease[];
};

export type APIProductValidationFinding = {
  level: "info" | "warning" | "error";
  code: string;
  message: string;
  component_id?: string;
  binding_id?: string;
};

export type APIProductProfile = {
  id: string;
  name: string;
  state: "draft" | "published";
  selections: Array<{ component_id: string; release_id: string }>;
};

export type APIProductDefinition = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  slug: string;
  state: "draft" | "published";
  version_strategy: "independent_api_tracks";
  mcp_policy: "Stateless MCPv2 Only";
  components: APIProductComponent[];
  product_bindings: APIProductBinding[];
  profiles: APIProductProfile[];
  validation: APIProductValidationFinding[];
  generated_by: "automatic_product_builder" | "ai_product_builder";
  source_build_id?: string;
  revision: number;
  published_at?: string;
  created_at: string;
  updated_at: string;
};

export type APIProductBuildInput = {
  kind: "auto" | "openapi" | "docs" | "git" | "mcp" | "tool";
  name?: string;
  location: string;
  version?: string;
  metadata?: Record<string, string>;
};

export type APIProductBuild = {
  id: string;
  organisation_id: string;
  product_id: string;
  state: "running" | "review" | "published" | "failed";
  analysis_mode: "automatic" | "ai_assisted";
  inputs: APIProductBuildInput[];
  proposal: APIProductDefinition;
  unresolved: APIProductValidationFinding[];
  created_at: string;
  completed_at?: string;
};

export type APIOrganisation = {
  id: string;
  name: string;
  slug: string;
  revision: number;
};

export type APIEnvironment = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  slug: string;
  is_production: boolean;
  revision: number;
};

export type APISource = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  kind: string;
  location: string;
  visibility: APIVisibility;
  published: boolean;
  quarantined: boolean;
  revision: number;
};

export type APICrawlJob = {
  id: string;
  state: "queued" | "running" | "review" | "succeeded" | "failed" | "cancelled";
  discovered_count: number;
  fetched_count: number;
  changed_count: number;
  error_message?: string;
  queued_at: string;
  finished_at?: string;
};

export type APITool = {
  id: string;
  organisation_id: string;
  product_id: string;
  namespace: string;
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;
  state: "draft" | "published";
  revision: number;
  http_method: string;
  authorization_policy: Record<string, unknown>;
  timeout_ms: number;
  backend_kind?: "http" | "mcp";
  mcp_connection_id?: string;
  upstream_tool_name?: string;
  upstream_schema_hash?: string;
  upstream_drifted?: boolean;
};

export type APIMCPConnection = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  namespace: string;
  endpoint: string;
  protocol_version: "2026-07-28";
  auth_mode: "none" | "service" | "delegated_oauth";
  oauth_client_id?: string;
  oauth_issuer?: string;
  authorization_url?: string;
  token_url?: string;
  scopes: string[];
  state: "active" | "disabled";
  last_synced_at?: string;
  last_catalog_hash?: string;
  revision: number;
};

export type APIMCPCatalogTool = {
  name: string;
  title?: string;
  description?: string;
  input_schema: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
  annotations?: Record<string, unknown>;
  schema_hash: string;
};

export type APIMCPCatalog = {
  connection: APIMCPConnection;
  tools: APIMCPCatalogTool[];
  catalog_hash: string;
  ttl_ms?: number;
};

export type APIMCPImportResult = {
  connection: APIMCPConnection;
  created: APITool[];
  updated: APITool[];
  unchanged: APITool[];
  drifted: APITool[];
  rejected: Record<string, string>;
};

export type Distribution = {
  product: APIProduct;
  public_mcp_endpoint: string;
  private_mcp_endpoint: string;
  public_sources: number;
  agent_setup: APIAgentSetupLinks;
};

export type APIAgentSetupLink = {
  available: boolean;
  unavailable_reason?: "public_mcp_disabled" | "identity_unavailable";
  url: string;
  embed_html: string;
  contains_secret: false;
};

export type APIAgentSetupLinks = {
  public: APIAgentSetupLink;
  private: APIAgentSetupLink;
};

export type APIIdentity = {
  id: string;
  organisation_id: string;
  deployment_id: string;
  issuer: string;
  client_id: string;
  scopes: string[];
  audience?: string;
  oauth_resource?: string;
  organisation_claim: string;
  installation_claim: string;
  delegated_api_origin: string;
  state: "active" | "disabled";
  revision: number;
};

export type APICustomerAccount = {
  id: string;
  organisation_id: string;
  product_id: string;
  issuer: string;
  external_id: string;
  state: "active" | "suspended";
  revision: number;
  created_at: string;
  updated_at: string;
  last_authenticated_at: string;
};

export type APISupportSubmission = {
  id: string;
  support_route_id: string;
  kind: "bug" | "feedback";
  state: "held" | "pending" | "delivering" | "delivered" | "failed";
  summary: string;
  category?: string;
  rating?: number;
  related_tool?: string;
  attempts: number;
  last_error?: string;
  external_id?: string;
  external_url?: string;
  created_at: string;
  delivered_at?: string;
  expires_at: string;
  content?: Record<string, unknown>;
  trusted_context: {
    product_id: string;
    product_name: string;
    product_version_id?: string;
    product_version?: string;
    manifest_hash?: string;
    catalog_revision?: number;
    selection_source?: string;
    environment_id?: string;
    installation_id?: string;
  };
  trusted_integration?: {
    integration_id: string;
    family_key: string;
    version_key: string;
    display_name: string;
    lifecycle: string;
    revision: number;
    manifest_hash?: string;
  };
};

export type APIAnalytics = {
  active_developers: number;
  authorized_users: number;
  mcp_requests: number;
  tool_calls: number;
  integration_runs: number;
  validated_runs: number;
  validated_success: number;
  first_pass_rate: number;
  channels: Record<string, number>;
  versions: Record<string, number>;
  funnel: Record<string, number>;
  daily_requests: Array<{ date: string; count: number }>;
  since: string;
  generated_at: string;
};

export type APILLMProfile = {
  id: string;
  role: "embedding" | "extraction" | "reranking" | "evaluation" | "assistant";
  provider: string;
  endpoint: string;
  model: string;
  embedding_dimensions?: number;
  max_input_tokens: number;
  max_output_tokens: number;
  daily_token_budget: number;
  hardening: Record<string, boolean>;
  enabled: boolean;
  revision: number;
};

export type APIAIProviderConnection = {
  id: string;
  organisation_id: string;
  deployment_id: string;
  provider: "openai" | "google" | "anthropic" | "digitalocean" | "xai" | "deepseek" | "openai-compatible";
  endpoint: string;
  managed_by: "console" | "environment";
  enabled: boolean;
  is_backup: boolean;
  backup_models: Partial<Record<"analysis" | "assistant", string>>;
  last_tested_at?: string;
  last_error_code?: string;
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIAIWorkloadProfile = {
  id: string;
  organisation_id: string;
  product_id: string;
  workload: "analysis" | "assistant";
  provider_connection_id: string;
  model: string;
  max_input_tokens: number;
  max_output_tokens: number;
  daily_token_budget: number;
  hardening: Record<string, boolean>;
  enabled: boolean;
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIIntegrationAnalysis = {
  id: string;
  organisation_id: string;
  product_id: string;
  schema_version: number;
  state: "running" | "review" | "failed";
  generated_by: "deterministic" | "ai_assisted";
  evidence: Array<{ kind: string; resource_id: string; label: string; location?: string; excerpt?: string; references?: APIRecipeReference[]; version?: string; visibility: APIVisibility; fingerprint: string }>;
  plan: {
    summary: string;
    identity: { mode: string; issuer?: string; audience?: string; grants?: string[]; explanation: string };
    endpoints: Array<{ name: string; method: string; path: string; purpose: string; identity: string; evidence: string[] }>;
    recipes: Array<{ slug: string; title: string; outcome: string; audience: string; endpoint_ids?: string[] }>;
  };
  unknowns: Array<{ id: string; question: string; why: string; blocking: boolean; answer?: string }>;
  error_code?: string;
  revision: number;
  created_at: string;
  completed_at?: string;
};

export type APIRecipeReference = { label: string; url: string; kind: string; resource_id?: string; anchor?: string };
export type APIRecipeFinding = { level: "info" | "warning" | "error"; code: string; message: string };
export type APIRecipeRevision = { id: string; recipe_id: string; revision: number; markdown: string; references: APIRecipeReference[]; validation: APIRecipeFinding[]; review?: string; generated_by: "ai" | "human" | "deterministic"; model?: string; created_by: string; created_at: string };
export type APIRecipe = {
  id: string;
  organisation_id: string;
  product_id: string;
  analysis_id?: string;
  slug: string;
  title: string;
  outcome: string;
  audience: string;
  state: "draft" | "review" | "approved" | "published" | "outdated";
  generated: boolean;
  needs_attention: boolean;
  visibility: APIVisibility;
  dependencies: Array<{ kind: string; resource_id: string; version: string }>;
  current_revision_id: string;
  current_revision?: APIRecipeRevision;
  stable_uri: string;
  approved_by?: string;
  approved_at?: string;
  published_at?: string;
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIRecipePopularity = {
  recipe_id: string;
  recipe_slug: string;
  title: string;
  views: number;
  plan_selections: number;
};

export type APIAIWorkloadUsage = {
  workload: APIAIWorkloadProfile["workload"];
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
};

export type APIAIProviderUsage = {
  provider: APIAIProviderConnection["provider"];
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
  backup_calls: number;
  last_used_at: string;
};

export type APIIntegrationRun = {
  id: string;
  organisation_id: string;
  product_id: string;
  environment_id: string;
  requested_outcome: string;
  state: "running" | "succeeded" | "failed";
  reported_success?: boolean;
  validated_success?: boolean;
  failure_code?: string;
  started_at: string;
  finished_at?: string;
};

export type APIAuditEvent = {
  id: string;
  organisation_id: string;
  product_id?: string;
  actor_id: string;
  action: string;
  target_type: string;
  target_id: string;
  request_id: string;
  created_at: string;
};

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;

  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export type APIUser = {
  id: string;
  email: string;
  display_name: string;
  role: "root" | string;
  revoked_at?: string;
};

export type SetupEnrollment = {
  enrollment_id: string;
  totp_secret: string;
  provisioning_uri: string;
  expires_at: string;
};

export type AuthSession = {
  user: APIUser;
  expires_at: string;
};

function cookie(name: string): string {
  if (typeof document === "undefined") return "";
  const prefix = `${encodeURIComponent(name)}=`;
  const part = document.cookie.split("; ").find((value) => value.startsWith(prefix));
  return part ? decodeURIComponent(part.slice(prefix.length)) : "";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const csrfToken = !["GET", "HEAD", "OPTIONS"].includes(method) ? cookie("dokosoko_csrf") : "";
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(csrfToken ? { "X-CSRF-Token": csrfToken } : {}),
      ...init?.headers,
    },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = payload?.error ?? {};
    throw new APIError(response.status, error.code ?? "request_failed", error.message ?? "Request failed.", error.details);
  }
  return payload as T;
}

const productPath = (productID: string) => `/api/v1/products/${encodeURIComponent(productID)}`;

export const api = {
  setupStatus: () => request<{ setup_complete: boolean; requires_mfa: boolean }>("/api/v1/setup/status"),
  beginSetup: (setupToken: string, input: { email: string; password: string }) => request<SetupEnrollment>("/api/v1/setup/begin", {
    method: "POST",
    headers: { Authorization: `Bearer ${setupToken}` },
    body: JSON.stringify(input),
  }),
  completeSetup: (enrollmentID: string, code: string) => request<{ user: APIUser; csrf_token: string; recovery_codes: string[] }>("/api/v1/setup/complete", {
    method: "POST",
    body: JSON.stringify({ enrollment_id: enrollmentID, code }),
  }),
  session: () => request<AuthSession>("/api/v1/auth/session"),
  login: (email: string, password: string, code: string) => request<AuthSession & { csrf_token: string }>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password, code }),
  }),
  logout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),
  rootUsers: async () => (await request<{ items: APIUser[] }>("/api/v1/root/users")).items,
  systemDoctor: () => request<{ status: "ok" | "error"; checks: Array<{ name: string; status: string; message: string }>; generated_at: string }>("/api/v1/system/doctor"),
  beginRootUser: (input: { email: string; display_name: string; password: string }) => request<SetupEnrollment>("/api/v1/root/users", { method: "POST", body: JSON.stringify(input) }),
  completeRootUser: (enrollmentID: string, code: string) => request<{ user: APIUser; recovery_codes: string[] }>("/api/v1/root/users", { method: "PUT", body: JSON.stringify({ enrollment_id: enrollmentID, code }) }),
  revokeRootUser: (userID: string) => request<void>(`/api/v1/root/users/${encodeURIComponent(userID)}`, { method: "DELETE" }),
  organisations: async () => (await request<{ items: APIOrganisation[] }>("/api/v1/organisations")).items,
  createOrganisation: (name: string, slug: string) => request<APIOrganisation>("/api/v1/organisations", { method: "POST", body: JSON.stringify({ name, slug }) }),
  deployment: () => request<APIDeployment>("/api/v1/deployment"),
  createDeployment: (organisationID: string, name: string, slug: string) => request<APIDeployment>("/api/v1/deployment", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, default_release_policy: "latest" }) }),
  updateDeployment: (input: Partial<APIDeployment> & { revision: number }) => request<APIDeployment>("/api/v1/deployment", { method: "PATCH", body: JSON.stringify(input) }),
  deploymentEnvironments: async () => (await request<{ items: APIEnvironment[] }>("/api/v1/environments")).items,
  createDeploymentEnvironment: (organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>("/api/v1/environments", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  integrations: async () => (await request<{ items: APIIntegration[] }>("/api/v1/integrations")).items,
  widgets: async () => (await request<{ items: APIWidget[] }>("/api/v1/widgets")).items,
  widget: (widgetID: string) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}`),
  createWidget: (input: APIWidgetInput) => request<APIWidgetProvisioning>("/api/v1/widgets", { method: "POST", body: JSON.stringify(input) }),
  updateWidget: (widgetID: string, input: APIWidgetInput & { revision: number }) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  activateWidget: (widgetID: string, revision: number) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/activate`, { method: "POST", body: JSON.stringify({ revision }) }),
  disableWidget: (widgetID: string, revision: number) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/disable`, { method: "POST", body: JSON.stringify({ revision }) }),
  widgetSecrets: async (widgetID: string) => (await request<{ items: APIWidgetSecret[] }>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets`)).items,
  createWidgetSecret: (widgetID: string) => request<APIWidgetSecretProvisioning>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets`, { method: "POST" }),
  revokeWidgetSecret: (widgetID: string, secretID: string) => request<APIWidgetSecret>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets/${encodeURIComponent(secretID)}`, { method: "DELETE" }),
  widgetSessions: async (widgetID: string) => (await request<{ items: APIWidgetSession[] }>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/sessions`)).items,
  revokeWidgetSession: (widgetID: string, sessionID: string) => request<APIWidgetSession>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/sessions/${encodeURIComponent(sessionID)}`, { method: "DELETE" }),
  integration: (integrationID: string) => request<APIIntegrationDetail>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`),
  createIntegration: (input: { family_key: string; version_key: string; display_name: string; description: string; visibility?: APIVisibility; acknowledge_public?: boolean; lifecycle?: APIIntegration["lifecycle"] }) => request<APIIntegration>("/api/v1/integrations", { method: "POST", body: JSON.stringify(input) }),
  updateIntegration: (integrationID: string, input: Pick<APIIntegration, "family_key" | "version_key" | "display_name" | "description" | "visibility" | "lifecycle" | "revision"> & { acknowledge_public?: boolean; replacement_integration_id?: string; sunset_at?: string }) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`, { method: "PUT", body: JSON.stringify(input) }),
  publishIntegration: (integrationID: string) => request<APIIntegrationRevision>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  setIntegrationAccessConnections: (integrationID: string, accessConnectionIDs: string[]) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/access-connections`, { method: "PUT", body: JSON.stringify({ access_connection_ids: accessConnectionIDs }) }),
  setIntegrationSupportRoute: (integrationID: string, supportRouteID: string) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/support-route`, { method: "PUT", body: JSON.stringify({ support_route_id: supportRouteID }) }),
  resourceSets: async (kind = "") => (await request<{ items: APIResourceSet[] }>(`/api/v1/resource-sets${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`)).items,
  createResourceSet: (input: { kind: APIResourceSet["kind"]; name: string; description: string; manifest: Array<Record<string, unknown>> }) => request<APIResourceSet>("/api/v1/resource-sets", { method: "POST", body: JSON.stringify(input) }),
  updateResourceSet: (setID: string, input: { name: string; description: string; state: APIResourceSet["state"]; manifest: Array<Record<string, unknown>>; revision: number }) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  duplicateResourceSet: (setID: string, name: string) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}/duplicate`, { method: "POST", body: JSON.stringify({ name }) }),
  attachResourceSet: (integrationID: string, resourceSetID: string, pinnedRevisionID = "") => request<APIIntegrationResourceLink>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets`, { method: "POST", body: JSON.stringify({ resource_set_id: resourceSetID, pinned_revision_id: pinnedRevisionID }) }),
  detachResourceSet: (integrationID: string, resourceSetID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets/${encodeURIComponent(resourceSetID)}`, { method: "DELETE" }),
  accessDefinitions: async () => (await request<{ items: APIAccessDefinition[] }>("/api/v1/access-definitions")).items,
  createAccessDefinition: (input: { service_key: string; name: string; instance_cardinality: APIAccessDefinition["instance_cardinality"]; instance_label_singular: string; instance_label_plural: string; credential_scope: APIAccessDefinition["credential_scope"]; management_auth_type: APIAccessDefinition["management_auth_type"]; api_resource_set_id?: string; operations: Record<string, unknown> }) => request<APIAccessDefinition>("/api/v1/access-definitions", { method: "POST", body: JSON.stringify(input) }),
  accessConnections: async () => (await request<{ items: APIAccessConnection[] }>("/api/v1/access-connections")).items,
  createAccessConnection: (input: { access_definition_id: string; environment_id?: string; name: string; region?: string; base_url: string; management_secret?: string; config: Record<string, unknown>; integration_ids: string[] }) => request<APIAccessConnection>("/api/v1/access-connections", { method: "POST", body: JSON.stringify(input) }),
  accessInstances: async (connectionID: string) => (await request<{ items: APIAccessInstance[] }>(`/api/v1/access-connections/${encodeURIComponent(connectionID)}/instances`)).items,
  accessCredentials: async (connectionID: string) => (await request<{ items: APIAccessCredential[] }>(`/api/v1/access-connections/${encodeURIComponent(connectionID)}/credentials`)).items,
  backendConnections: async () => (await request<{ items: APIBackendConnection[] }>("/api/v1/backend-connections")).items,
  createBackendConnection: (input: { name: string; base_url: string; authentication_type?: "bearer"; credential?: string; state?: APIBackendConnection["state"] }) => request<APIBackendConnection>("/api/v1/backend-connections", { method: "POST", body: JSON.stringify(input) }),
  replaceBackendConnection: (connectionID: string, input: { name: string; base_url: string; authentication_type: "bearer"; state: APIBackendConnection["state"]; revision: number }) => request<APIBackendConnection>(`/api/v1/backend-connections/${encodeURIComponent(connectionID)}`, { method: "PUT", body: JSON.stringify(input) }),
  createBackendConnectionCredential: (connectionID: string, credential: string, revision: number) => request<APIBackendConnectionCredential>(`/api/v1/backend-connections/${encodeURIComponent(connectionID)}/credentials`, { method: "POST", body: JSON.stringify({ credential, revision }) }),
  supportRoutes: async () => (await request<{ items: APISupportRoute[] }>("/api/v1/support-routes")).items,
  createSupportRoute: (input: { name: string; is_default: boolean; bug_reports_enabled: boolean; feedback_enabled: boolean; backend_connection_id?: string; retention_days: number; state: APISupportRoute["state"]; integration_ids: string[] }) => request<APISupportRoute>("/api/v1/support-routes", { method: "POST", body: JSON.stringify(input) }),
  replaceSupportRoute: (routeID: string, input: { name: string; is_default: boolean; bug_reports_enabled: boolean; feedback_enabled: boolean; backend_connection_id?: string; retention_days: number; state: APISupportRoute["state"]; integration_ids: string[]; revision: number }) => request<APISupportRoute>(`/api/v1/support-routes/${encodeURIComponent(routeID)}`, { method: "PUT", body: JSON.stringify(input) }),
  products: async (organisationID: string) => (await request<{ items: APIProduct[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`)).items,
  createProduct: (organisationID: string, name: string, slug: string) => request<APIProduct>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`, { method: "POST", body: JSON.stringify({ name, slug }) }),
  updateProductSettings: (productID: string, description: string, defaultVersionPolicy: "latest" | "lts", requirePromotionApproval: boolean, revision: number) => request<APIProduct>(productPath(productID), { method: "PATCH", body: JSON.stringify({ description, default_version_policy: defaultVersionPolicy, require_promotion_approval: requirePromotionApproval, revision }) }),
  rewriteProductDescription: (productID: string, draft: string) => request<{ description: string }>(`${productPath(productID)}/description/rewrite`, { method: "POST", body: JSON.stringify({ draft }) }),
  productVersions: async (productID: string) => (await request<{ items: APIProductVersion[] }>(`${productPath(productID)}/versions`)).items,
  createProductVersion: (productID: string, input: { version: string; profile_id: string; is_latest: boolean; is_lts: boolean; release_stage: "preview" | "active"; rollout_percentage: number }) => request<APIProductVersion>(`${productPath(productID)}/versions`, { method: "POST", body: JSON.stringify(input) }),
  updateProductVersion: (productID: string, versionID: string, input: { is_latest: boolean; is_lts: boolean; deprecated: boolean; deprecation_message: string; replacement_version: string; sunset_at?: string; rollout_percentage: number; acknowledge_impact: boolean; revision: number }) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  productVersionImpact: (productID: string, versionID: string) => request<APIProductVersionImpact>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/impact`),
  productVersionDiff: (productID: string, versionID: string) => request<APIProductVersionDiff>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/diff`),
  reconcileProductVersion: (productID: string, versionID: string, revision: number) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/reconcile`, { method: "POST", body: JSON.stringify({ revision }) }),
  promoteProductVersion: (productID: string, versionID: string, action: "request" | "approve" | "reject", note: string, revision: number) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/promotion`, { method: "POST", body: JSON.stringify({ action, note, revision }) }),
  productVersionPins: async (productID: string) => (await request<{ items: APIProductVersionPin[] }>(`${productPath(productID)}/version-pins`)).items,
  saveProductVersionPin: (productID: string, input: { scope: "customer" | "environment" | "installation"; scope_id: string; customer_account_id?: string; product_version_id: string; reason: string; revision: number }) => request<APIProductVersionPin>(`${productPath(productID)}/version-pins`, { method: "POST", body: JSON.stringify(input) }),
  deleteProductVersionPin: (productID: string, pinID: string) => request<void>(`${productPath(productID)}/version-pins/${encodeURIComponent(pinID)}`, { method: "DELETE" }),
  productVersionPinHistory: async (productID: string) => (await request<{ items: APIProductVersionPinHistory[] }>(`${productPath(productID)}/version-pins/history`)).items,
  productInstallations: async (productID: string) => (await request<{ items: APIProductInstallation[] }>(`${productPath(productID)}/installations`)).items,
  saveProductInstallation: (productID: string, input: { id?: string; customer_account_id: string; environment_id: string; external_id: string; name: string; state: "active" | "paused"; revision: number }) => request<APIProductInstallation>(`${productPath(productID)}/installations`, { method: "POST", body: JSON.stringify(input) }),
  customerAccounts: async (productID: string) => (await request<{ items: APICustomerAccount[]; has_more: boolean }>(`${productPath(productID)}/customer-accounts?limit=200`)).items,
  updateCustomerAccount: (productID: string, accountID: string, state: APICustomerAccount["state"], revision: number) => request<APICustomerAccount>(`${productPath(productID)}/customer-accounts/${encodeURIComponent(accountID)}`, { method: "PATCH", body: JSON.stringify({ state, revision }) }),
  productDefinition: (productID: string) => request<APIProductDefinition>(`${productPath(productID)}/definition`),
  productBuilds: async (productID: string) => (await request<{ items: APIProductBuild[] }>(`${productPath(productID)}/product-builds`)).items,
  buildProduct: (productID: string, inputs: APIProductBuildInput[]) => request<APIProductBuild>(`${productPath(productID)}/product-builds`, { method: "POST", body: JSON.stringify({ inputs }) }),
  publishProductBuild: (productID: string, buildID: string) => request<APIProductDefinition>(`${productPath(productID)}/product-builds/${encodeURIComponent(buildID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  environments: async (productID: string) => (await request<{ items: APIEnvironment[] }>(`${productPath(productID)}/environments`)).items,
  createEnvironment: (productID: string, organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>(`${productPath(productID)}/environments`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  distribution: (productID: string) => request<Distribution>(`${productPath(productID)}/distribution`),
  identity: () => request<APIIdentity>("/api/v1/identity-provider"),
  configureIdentity: (input: { issuer: string; client_id: string; client_secret: string; scopes: string[]; audience: string; oauth_resource: string; organisation_claim: string; installation_claim: string; delegated_api_origin: string; state: APIIdentity["state"]; revision: number }) => request<APIIdentity>("/api/v1/identity-provider", { method: "PUT", body: JSON.stringify(input) }),
  supportSubmissions: async () => (await request<{ items: APISupportSubmission[]; has_more: boolean }>("/api/v1/support-submissions?limit=200")).items,
  supportSubmission: (submissionID: string) => request<APISupportSubmission>(`/api/v1/support-submissions/${encodeURIComponent(submissionID)}`),
  createSupportDeliveryAttempt: (submissionID: string) => request<APISupportSubmission>(`/api/v1/support-submissions/${encodeURIComponent(submissionID)}/delivery-attempts`, { method: "POST" }),
  analytics: (productID: string, days = 30) => request<APIAnalytics>(`${productPath(productID)}/analytics?days=${days}`),
  integrationRuns: async (productID: string) => (await request<{ items: APIIntegrationRun[] }>(`${productPath(productID)}/integration-runs`)).items,
  startIntegrationRun: (productID: string, environmentID: string, requestedOutcome: string) => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs`, { method: "POST", body: JSON.stringify({ environment_id: environmentID, requested_outcome: requestedOutcome }) }),
  completeIntegrationRun: (productID: string, runID: string, reportedSuccess: boolean, validatedSuccess: boolean, failureCode = "") => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs/${encodeURIComponent(runID)}/complete`, { method: "POST", body: JSON.stringify({ reported_success: reportedSuccess, validated_success: validatedSuccess, failure_code: failureCode }) }),
  auditEvents: async (organisationID: string) => (await request<{ items: APIAuditEvent[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/audit`)).items,
  llmProfiles: async (productID: string) => (await request<{ items: APILLMProfile[] }>(`${productPath(productID)}/llm-profiles`)).items,
  saveLLMProfile: (productID: string, input: { organisation_id: string; role: string; provider: string; endpoint: string; model: string; credential: string; embedding_dimensions: number; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean }) => request<APILLMProfile>(`${productPath(productID)}/llm-profiles`, { method: "PUT", body: JSON.stringify(input) }),
  aiConnections: async () => (await request<{ items: APIAIProviderConnection[] }>("/api/v1/ai/connections")).items,
  saveAIConnection: (input: { organisation_id: string; provider: APIAIProviderConnection["provider"]; endpoint: string; credential: string; enabled: boolean; is_backup: boolean; backup_models: Partial<Record<APIAIWorkloadProfile["workload"], string>>; revision: number }) => request<APIAIProviderConnection>("/api/v1/ai/connections", { method: "POST", body: JSON.stringify(input) }),
  testAIConnection: (connectionID: string) => request<APIAIProviderConnection>(`/api/v1/ai/connections/${encodeURIComponent(connectionID)}/test`, { method: "POST", body: JSON.stringify({}) }),
  aiProfiles: async (productID: string) => (await request<{ items: APIAIWorkloadProfile[] }>(`${productPath(productID)}/ai-profiles`)).items,
  saveAIProfile: (productID: string, workload: APIAIWorkloadProfile["workload"], input: { organisation_id: string; provider_connection_id: string; model: string; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean; revision: number }) => request<APIAIWorkloadProfile>(`${productPath(productID)}/ai-profiles/${encodeURIComponent(workload)}`, { method: "PUT", body: JSON.stringify(input) }),
  analyses: async (productID: string) => (await request<{ items: APIIntegrationAnalysis[] }>(`${productPath(productID)}/analyses`)).items,
  analyseIntegration: (productID: string) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses`, { method: "POST", body: JSON.stringify({}) }),
  answerAnalysis: (productID: string, analysisID: string, answers: Record<string, string>) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}`, { method: "PATCH", body: JSON.stringify({ answers }) }),
  generateRecipes: async (productID: string, analysisID: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}/recipes`, { method: "POST", body: JSON.stringify({}) })).items,
  recipes: async (productID: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/recipes`)).items,
  createRecipe: (productID: string, prompt: string) => request<APIRecipe>(`${productPath(productID)}/recipes`, { method: "POST", body: JSON.stringify({ prompt }) }),
  recipe: (productID: string, recipeID: string) => request<{ recipe: APIRecipe; revisions: APIRecipeRevision[] }>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`),
  updateRecipe: (productID: string, recipeID: string, markdown: string, references: APIRecipeReference[], visibility: APIVisibility) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`, { method: "PATCH", body: JSON.stringify({ markdown, references, visibility }) }),
  reworkRecipe: (productID: string, recipeID: string, instruction: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/rework`, { method: "POST", body: JSON.stringify({ instruction }) }),
  approveRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/approve`, { method: "POST", body: JSON.stringify({}) }),
  publishRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  recipeAnalytics: async (productID: string, days = 30) => (await request<{ items: APIRecipePopularity[] }>(`${productPath(productID)}/recipe-analytics?days=${days}`)).items,
  aiUsage: (productID: string, days = 30) => request<{ workloads: APIAIWorkloadUsage[]; providers: APIAIProviderUsage[] }>(`${productPath(productID)}/ai-usage?days=${days}`),
  sources: async (productID: string) => (await request<{ items: APISource[] }>(`${productPath(productID)}/sources`)).items,
  createSource: (productID: string, organisationID: string, name: string, kind: string, location: string) => request<APISource>(`${productPath(productID)}/sources`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, kind, location }) }),
  queueCrawl: (productID: string, sourceID: string) => request<APICrawlJob>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawl`, { method: "POST" }),
  crawlJobs: async (productID: string, sourceID: string) => (await request<{ items: APICrawlJob[] }>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawls`)).items,
  publishSource: (productID: string, sourceID: string, revision: number) => request<APISource>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/publish`, { method: "POST", body: JSON.stringify({ revision }) }),
  tools: async (productID: string) => (await request<{ items: APITool[] }>(`${productPath(productID)}/tools`)).items,
  createTool: (productID: string, input: Record<string, unknown>) => request<APITool>(`${productPath(productID)}/tools`, { method: "POST", body: JSON.stringify(input) }),
  publishTool: (productID: string, toolID: string, revision: number) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/publish`, { method: "POST", body: JSON.stringify({ revision }) }),
  mcpConnections: async (productID: string) => (await request<{ items: APIMCPConnection[] }>(`${productPath(productID)}/mcp-connections`)).items,
  createMCPConnection: (productID: string, input: { organisation_id: string; name: string; namespace: string; endpoint: string; auth_mode: APIMCPConnection["auth_mode"]; credential: string; oauth_client_id: string; oauth_client_secret: string; oauth_issuer: string; authorization_url: string; token_url: string; scopes: string[] }) => request<APIMCPConnection>(`${productPath(productID)}/mcp-connections`, { method: "POST", body: JSON.stringify(input) }),
  inspectMCPConnection: (productID: string, connectionID: string) => request<APIMCPCatalog>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/inspect`, { method: "POST" }),
  importMCPTools: (productID: string, connectionID: string, input: { tool_names: string[]; required_grants: string[]; confirmation_required: boolean; timeout_ms: number }) => request<APIMCPImportResult>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/import`, { method: "POST", body: JSON.stringify(input) }),
  setPublicMCP: (productID: string, enabled: boolean, revision: number, acknowledgePublic: boolean) => request<APIProduct>(`${productPath(productID)}/distribution`, {
    method: "PATCH",
    body: JSON.stringify({ public_mcp_enabled: enabled, revision, acknowledge_public: acknowledgePublic }),
  }),
  setSourceVisibility: (productID: string, id: string, visibility: APIVisibility, revision: number, acknowledgePublic: boolean) => request<APISource>(`${productPath(productID)}/sources/${encodeURIComponent(id)}/visibility`, {
    method: "PATCH",
    body: JSON.stringify({ visibility, revision, acknowledge_public: acknowledgePublic }),
  }),
};
