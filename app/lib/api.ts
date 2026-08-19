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
  customer_id: string;
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
  customer_id: string;
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
  kind: "openapi" | "docs" | "git" | "package" | "mcp" | "tool";
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
  kind: "auto" | "openapi" | "docs" | "git" | "package" | "mcp" | "tool";
  name?: string;
  location: string;
  version?: string;
  ecosystem?: string;
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

export type APIPackage = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  ecosystem: string;
  version: string;
  mode: "public" | "proxy" | "fetch";
  visibility: APIVisibility;
  published: boolean;
  revision: number;
};

export type Distribution = {
  product: APIProduct;
  public_mcp_endpoint: string;
  private_mcp_endpoint: string;
  public_sources: number;
  public_packages: number;
};

export type APIWidgetSnippet = {
  enabled: boolean;
  snippet: string;
  contains_secret: false;
};

export type APIWidgetSnippets = {
  public: APIWidgetSnippet;
  private: APIWidgetSnippet;
};

export type APIIdentity = {
  id: string;
  organisation_id: string;
  product_id: string;
  issuer: string;
  client_id: string;
  scopes: string[];
  audience: string;
  organisation_claim: string;
  installation_claim: string;
  entitlement_hook_url: string;
  authorization_hook_url: string;
  usage_hook_url: string;
  allowed_redirect_uris: string[];
  revision: number;
};

export type APIReportingConfig = {
  id?: string;
  organisation_id: string;
  product_id: string;
  bug_reports_enabled: boolean;
  feedback_enabled: boolean;
  bug_hook_url: string;
  feedback_hook_url: string;
  retention_days: number;
  revision: number;
};

export type APIReportSubmission = {
  id: string;
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
};

export type APIAnalytics = {
  active_developers: number;
  authorized_users: number;
  mcp_requests: number;
  tool_calls: number;
  package_downloads: number;
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

export type APIProvider = {
  id: string;
  organisation_id: string;
  product_id: string;
  name: string;
  kind: "remote" | "builtin" | "proxy";
  config: { contract_version?: string; required_entitlements?: string[]; max_ttl_seconds?: number };
  revision: number;
};

export type APIProject = {
  id: string;
  provider_id: string;
  environment_id: string;
  external_id: string;
  state: string;
  expires_at?: string;
  created_at: string;
};

export type APICredentialLease = {
  id: string;
  provider_id: string;
  project_id?: string;
  subject_id: string;
  scopes: string[];
  secret_fingerprint: string;
  expires_at: string;
  revoked_at?: string;
  created_at: string;
};

export type APILLMProfile = {
  id: string;
  role: "embedding" | "extraction" | "reranking" | "evaluation" | "assistant";
  provider: string;
  model: string;
  embedding_dimensions?: number;
  max_input_tokens: number;
  max_output_tokens: number;
  daily_token_budget: number;
  hardening: Record<string, boolean>;
  enabled: boolean;
  revision: number;
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
  beginSetup: (setupToken: string, input: { email: string; display_name: string; password: string }) => request<SetupEnrollment>("/api/v1/setup/begin", {
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
  saveProductVersionPin: (productID: string, input: { scope: "customer" | "environment" | "installation"; scope_id: string; customer_id?: string; product_version_id: string; reason: string; revision: number }) => request<APIProductVersionPin>(`${productPath(productID)}/version-pins`, { method: "POST", body: JSON.stringify(input) }),
  deleteProductVersionPin: (productID: string, pinID: string) => request<void>(`${productPath(productID)}/version-pins/${encodeURIComponent(pinID)}`, { method: "DELETE" }),
  productVersionPinHistory: async (productID: string) => (await request<{ items: APIProductVersionPinHistory[] }>(`${productPath(productID)}/version-pins/history`)).items,
  productInstallations: async (productID: string) => (await request<{ items: APIProductInstallation[] }>(`${productPath(productID)}/installations`)).items,
  saveProductInstallation: (productID: string, input: { id?: string; customer_id: string; environment_id: string; external_id: string; name: string; state: "active" | "paused"; revision: number }) => request<APIProductInstallation>(`${productPath(productID)}/installations`, { method: "POST", body: JSON.stringify(input) }),
  productDefinition: (productID: string) => request<APIProductDefinition>(`${productPath(productID)}/definition`),
  productBuilds: async (productID: string) => (await request<{ items: APIProductBuild[] }>(`${productPath(productID)}/product-builds`)).items,
  buildProduct: (productID: string, inputs: APIProductBuildInput[]) => request<APIProductBuild>(`${productPath(productID)}/product-builds`, { method: "POST", body: JSON.stringify({ inputs }) }),
  publishProductBuild: (productID: string, buildID: string) => request<APIProductDefinition>(`${productPath(productID)}/product-builds/${encodeURIComponent(buildID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  environments: async (productID: string) => (await request<{ items: APIEnvironment[] }>(`${productPath(productID)}/environments`)).items,
  createEnvironment: (productID: string, organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>(`${productPath(productID)}/environments`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  distribution: (productID: string) => request<Distribution>(`${productPath(productID)}/distribution`),
  widgets: (productID: string) => request<APIWidgetSnippets>(`${productPath(productID)}/widgets`),
  identity: (productID: string) => request<APIIdentity>(`${productPath(productID)}/identity`),
  configureIdentity: (productID: string, input: { organisation_id: string; issuer: string; client_id: string; client_secret: string; scopes: string[]; audience: string; organisation_claim: string; installation_claim: string; entitlement_hook_url: string; authorization_hook_url: string; authorization_credential: string; usage_hook_url: string; usage_credential: string; allowed_redirect_uris: string[] }) => request<APIIdentity>(`${productPath(productID)}/identity`, { method: "PUT", body: JSON.stringify(input) }),
  reporting: (productID: string) => request<APIReportingConfig>(`${productPath(productID)}/reporting`),
  configureReporting: (productID: string, input: { bug_reports_enabled: boolean; feedback_enabled: boolean; bug_hook_url: string; bug_hook_credential: string; feedback_hook_url: string; feedback_hook_credential: string; retention_days: number; revision: number }) => request<APIReportingConfig>(`${productPath(productID)}/reporting`, { method: "PUT", body: JSON.stringify(input) }),
  reportSubmissions: async (productID: string) => (await request<{ items: APIReportSubmission[] }>(`${productPath(productID)}/report-submissions`)).items,
  reportSubmission: (productID: string, submissionID: string) => request<APIReportSubmission>(`${productPath(productID)}/report-submissions/${encodeURIComponent(submissionID)}`),
  retryReportSubmission: (productID: string, submissionID: string) => request<APIReportSubmission>(`${productPath(productID)}/report-submissions/${encodeURIComponent(submissionID)}/retry`, { method: "POST", body: JSON.stringify({}) }),
  analytics: (productID: string, days = 30) => request<APIAnalytics>(`${productPath(productID)}/analytics?days=${days}`),
  integrationRuns: async (productID: string) => (await request<{ items: APIIntegrationRun[] }>(`${productPath(productID)}/integration-runs`)).items,
  startIntegrationRun: (productID: string, environmentID: string, requestedOutcome: string) => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs`, { method: "POST", body: JSON.stringify({ environment_id: environmentID, requested_outcome: requestedOutcome }) }),
  completeIntegrationRun: (productID: string, runID: string, reportedSuccess: boolean, validatedSuccess: boolean, failureCode = "") => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs/${encodeURIComponent(runID)}/complete`, { method: "POST", body: JSON.stringify({ reported_success: reportedSuccess, validated_success: validatedSuccess, failure_code: failureCode }) }),
  auditEvents: async (organisationID: string) => (await request<{ items: APIAuditEvent[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/audit`)).items,
  providers: async (productID: string) => (await request<{ items: APIProvider[] }>(`${productPath(productID)}/providers`)).items,
  createProvider: (productID: string, input: { organisation_id: string; name: string; base_url: string; credential: string; required_entitlements: string[]; max_ttl_seconds: number }) => request<APIProvider>(`${productPath(productID)}/providers`, { method: "POST", body: JSON.stringify(input) }),
  projects: async (productID: string) => (await request<{ items: APIProject[] }>(`${productPath(productID)}/projects`)).items,
  credentials: async (productID: string) => (await request<{ items: APICredentialLease[] }>(`${productPath(productID)}/credentials`)).items,
  llmProfiles: async (productID: string) => (await request<{ items: APILLMProfile[] }>(`${productPath(productID)}/llm-profiles`)).items,
  saveLLMProfile: (productID: string, input: { organisation_id: string; role: string; provider: string; endpoint: string; model: string; credential: string; embedding_dimensions: number; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean }) => request<APILLMProfile>(`${productPath(productID)}/llm-profiles`, { method: "PUT", body: JSON.stringify(input) }),
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
  importMCPTools: (productID: string, connectionID: string, input: { tool_names: string[]; required_entitlements: string[]; confirmation_required: boolean; timeout_ms: number }) => request<APIMCPImportResult>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/import`, { method: "POST", body: JSON.stringify(input) }),
  packages: async (productID: string) => (await request<{ items: APIPackage[] }>(`${productPath(productID)}/packages`)).items,
  createPackage: (productID: string, input: { organisation_id: string; name: string; ecosystem: string; version: string; mode: "public" | "proxy" | "fetch"; upstream_url: string; fetch_hook_url: string; credential: string; checksum_sha256: string; expected_size: number }) => request<APIPackage>(`${productPath(productID)}/packages`, { method: "POST", body: JSON.stringify(input) }),
  publishPackage: (productID: string, packageID: string, revision: number) => request<APIPackage>(`${productPath(productID)}/packages/${encodeURIComponent(packageID)}/publish`, { method: "POST", body: JSON.stringify({ revision }) }),
  setPublicMCP: (productID: string, enabled: boolean, revision: number, acknowledgePublic: boolean) => request<APIProduct>(`${productPath(productID)}/distribution`, {
    method: "PATCH",
    body: JSON.stringify({ public_mcp_enabled: enabled, revision, acknowledge_public: acknowledgePublic }),
  }),
  setSourceVisibility: (productID: string, id: string, visibility: APIVisibility, revision: number, acknowledgePublic: boolean) => request<APISource>(`${productPath(productID)}/sources/${encodeURIComponent(id)}/visibility`, {
    method: "PATCH",
    body: JSON.stringify({ visibility, revision, acknowledge_public: acknowledgePublic }),
  }),
  setPackageVisibility: (productID: string, id: string, visibility: APIVisibility, revision: number, acknowledgePublic: boolean) => request<APIPackage>(`${productPath(productID)}/packages/${encodeURIComponent(id)}/visibility`, {
    method: "PATCH",
    body: JSON.stringify({ visibility, revision, acknowledge_public: acknowledgePublic }),
  }),
};
