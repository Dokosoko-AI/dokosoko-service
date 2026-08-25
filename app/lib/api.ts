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
  features?: {
    widgets: boolean;
  };
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

export type APIIntegrationPreflightCheck = {
  code: string;
  label: string;
  message: string;
  status: "pass" | "fail" | "optional";
  tab: string;
  required: boolean;
};

export type APIIntegrationPreflight = {
  integration_id: string;
  candidate_revision: number;
  candidate_manifest_hash: string;
  latest_published_id?: string;
  latest_published_revision?: number;
  latest_published_hash?: string;
  matches_latest_published: boolean;
  ready: boolean;
  checks: APIIntegrationPreflightCheck[];
  generated_at: string;
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
  integration_bindings: Array<{
    integration_id: string;
    integration_revision_id: string;
    integration_revision: number;
    manifest_hash: string;
    snapshot: Record<string, unknown>;
    bound_at: string;
  }>;
  knowledge_bindings: Array<{
    recipe_id: string;
    recipe_revision_id: string;
    recipe_revision: number;
    integration_ids: string[];
    title: string;
    outcome: string;
    audience: string;
    stable_uri: string;
    markdown: string;
    references: APIRecipeReference[];
    content_hash: string;
    bound_at: string;
  }>;
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
  kind: "customer" | "admin_preview";
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
export type APIWidgetConfiguration = { widgetId: string; name: string; appearance: APIWidgetAppearance; protocolVersion: "1" };
export type APIWidgetBootstrap = { bootstrapToken: string; expiresAt: string };
export type APIWidgetRuntimeSession = { sessionToken: string; sessionId: string; expiresAt: string };
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

export type APIRuntimeAuthenticationType =
  | "none"
  | "delegated_oauth"
  | "bearer"
  | "authorization_scheme"
  | "api_key_header"
  | "api_key_query"
  | "basic"
  | "oauth_client_credentials"
  | "custom_header";

export type APIRuntimeServiceConnectionRevision = {
  id: string;
  connection_id: string;
  environment_id: string;
  base_url: string;
  authentication_type: APIRuntimeAuthenticationType;
  credential_set_id?: string;
  auth_config?: Record<string, unknown>;
  content_hash: string;
  revision: number;
  current: boolean;
  created_by?: string;
  created_at: string;
};

export type APIRuntimeServiceConnection = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  integration_id: string;
  name: string;
  description?: string;
  state: string;
  revision: number;
  current_revisions?: APIRuntimeServiceConnectionRevision[];
  created_at: string;
  updated_at: string;
};

export type APIRuntimeServiceConnectionReadiness = {
  connection_id: string;
  ready: boolean;
  checks: Array<{
    key: string;
    label: string;
    ready: boolean;
    message: string;
    environment_id?: string;
  }>;
};

// Runtime credentials are deliberately represented by masked metadata only.
// The backing vault identifier and secret value are never part of this client contract.
export type APIRuntimeCredentialVersion = {
  id: string;
  credential_set_id: string;
  fingerprint: string;
  state: string;
  created_by?: string;
  activated_at?: string;
  retires_at?: string;
  revoked_at?: string;
  expires_at?: string;
  created_at: string;
};

export type APIRuntimeCredentialSet = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  environment_id: string;
  scope: "dedicated" | "shared";
  owner_integration_id?: string;
  name: string;
  environment_variable: string;
  authentication_type: APIRuntimeAuthenticationType;
  header_name?: string;
  state: string;
  credential_present: boolean;
  active_fingerprint?: string;
  revision: number;
  versions?: APIRuntimeCredentialVersion[];
  created_at: string;
  updated_at: string;
};

export type APIRuntimeSetup = {
  integration: APIIntegration;
  environments: APIEnvironment[];
  service_connections: APIRuntimeServiceConnection[];
  credential_sets: APIRuntimeCredentialSet[];
};

export type APIRuntimeSetupInput = {
  environment_id: string;
  connection_name?: string;
  connection_description?: string;
  base_url: string;
  authentication_type: APIRuntimeAuthenticationType;
  auth_config?: Record<string, unknown>;
  existing_credential_set_id?: string;
  credential_scope?: "dedicated" | "shared";
  credential_name?: string;
  environment_variable?: string;
  header_name?: string;
  credential?: string;
  credential_expires_at?: string;
};

export type APIRuntimeServiceConnectionInput = {
  name: string;
  description?: string;
  environment_id: string;
  base_url: string;
  authentication_type: APIRuntimeAuthenticationType;
  credential_set_id?: string;
  auth_config?: Record<string, unknown>;
  state?: string;
};

export type APIRuntimeCredentialSetInput = {
  environment_id: string;
  scope: "dedicated" | "shared";
  name?: string;
  environment_variable?: string;
  authentication_type: Exclude<APIRuntimeAuthenticationType, "none" | "delegated_oauth">;
  header_name?: string;
  credential: string;
  expires_at?: string;
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

export type APICrawlReviewDocument = {
  id: string;
  crawl_job_id: string;
  snapshot_id: string;
  title: string;
  canonical_url: string;
  state: "validated" | "published" | "quarantined";
  trust_level: number;
  injection_indicators: string[];
  content_hash: string;
  changed: boolean;
};

export type APISourcePublication = {
  id: string;
  organisation_id: string;
  product_id: string;
  source_id: string;
  crawl_job_id: string;
  revision: number;
  visibility: APIVisibility;
  content_hash: string;
  document_count: number;
  reviewed_by: string;
  reviewed_at: string;
  published_at: string;
};

export type APISourceReview = {
  source: APISource;
  crawl_job: APICrawlJob;
  documents: APICrawlReviewDocument[];
  publication?: APISourcePublication;
};

export type APISourcePublishResult = {
  source: APISource;
  publication: APISourcePublication;
};

export type APIToolHTTPMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export type APIToolRisk = "low" | "medium" | "high" | "critical";

export type APIToolUpstreamAuthType =
  | "delegated_oauth"
  | "none"
  | "bearer"
  | "authorization_scheme"
  | "api_key_header"
  | "api_key_query"
  | "basic"
  | "oauth_client_credentials"
  | "custom_header";

export type APIToolUpstreamAuth = {
  type: APIToolUpstreamAuthType;
  scheme?: string;
  header_name?: string;
  query_name?: string;
  prefix?: string;
  username?: string;
  client_id?: string;
  token_url?: string;
  token_endpoint_auth_method?: "client_secret_basic" | "client_secret_post";
  scopes?: string[];
  audience?: string;
  resource?: string;
};

export type APIToolRequestMapping = {
  parameter_locations: Record<string, "path" | "query" | "header" | "body">;
};

export type APIToolResponseMapping = {
  result_path?: string;
};

export type APIToolAuthorizationPolicy = {
  required_grants: string[];
  confirmation_required: boolean;
  risk: APIToolRisk;
  idempotency_required: boolean;
};

/**
 * The complete non-secret working contract used by every tool-builder mode.
 * Credentials intentionally do not belong to this type so it is safe to send
 * to proposal, import, validation, and analysis endpoints.
 */
export type APIToolBuilderDraft = {
  namespace: string;
  name: string;
  description: string;
  http_method: APIToolHTTPMethod;
  endpoint: string;
  timeout_ms: number;
  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;
  upstream_auth: APIToolUpstreamAuth;
  request_mapping: APIToolRequestMapping;
  response_mapping: APIToolResponseMapping;
  authorization_policy: APIToolAuthorizationPolicy;
  request_example?: Record<string, unknown>;
  response_example?: unknown;
  credential_present: boolean;
};

const TOOL_BUILDER_FOLLOW_UP_FIELDS: Array<keyof APIToolBuilderDraft> = [
  "namespace",
  "name",
  "description",
  "http_method",
  "endpoint",
  "timeout_ms",
  "input_schema",
  "output_schema",
  "upstream_auth",
  "request_mapping",
  "response_mapping",
  "authorization_policy",
  "request_example",
  "response_example",
];

/**
 * Refines one complete pending candidate while preserving explicit human
 * rejections. Credential material is not part of either draft; only the
 * server-derived presence bit is carried from the current browser state.
 */
export function toolBuilderFollowUpDraft(
  current: APIToolBuilderDraft,
  pending: APIToolBuilderDraft | null,
  decisions: Readonly<Record<string, "accepted" | "rejected">>,
  stale: boolean,
  identityLocked = false,
): APIToolBuilderDraft {
  if (!pending || stale) return current;
  const followUp = { ...pending, credential_present: current.credential_present };
  for (const field of TOOL_BUILDER_FOLLOW_UP_FIELDS) {
    if (decisions[field] === "rejected" || (identityLocked && (field === "namespace" || field === "name"))) {
      Object.assign(followUp, { [field]: current[field] });
    }
  }
  return followUp;
}

export type APIToolBuilderFinding = {
  level: "error" | "warning" | "info";
  code: string;
  message: string;
  field?: string;
  suggestion?: string;
};

export type APIToolBuilderChange = {
  field: string;
  summary?: string;
  before?: unknown;
  after?: unknown;
  rationale?: string;
  security_sensitive?: boolean;
};

export type APIToolBuilderAnalysis = {
  draft?: APIToolBuilderDraft;
  valid?: boolean;
  network_call_performed?: false;
  reply?: string;
  summary?: string;
  findings: APIToolBuilderFinding[];
  generated_at?: string;
};

export type APIToolBuilderValidation = {
  valid: boolean;
  network_call_performed: false;
  findings: APIToolBuilderFinding[];
  normalized_draft?: APIToolBuilderDraft;
  checked_at?: string;
};

export type APIToolBuilderProposal = {
  proposal_id?: string;
  base_fingerprint?: string;
  reply?: string;
  summary?: string;
  valid?: boolean;
  draft: APIToolBuilderDraft;
  changes: APIToolBuilderChange[];
  findings: APIToolBuilderFinding[];
  analysis?: APIToolBuilderAnalysis;
  generated_at?: string;
};

export type APIToolBuilderContext = {
  draft: APIToolBuilderDraft;
  base_tool_id?: string;
  base_revision?: number;
  credential_will_be_supplied?: boolean;
};

export const TOOL_BUILDER_CHAT_LIMITS = {
  maxMessages: 12,
  maxMessageBytes: 2_048,
  maxHistoryBytes: 12_288,
} as const;

export type APIToolBuilderChatMessage = {
  role: "user" | "assistant";
  content: string;
};

function utf8Bytes(value: string) {
  return new TextEncoder().encode(value).byteLength;
}

/**
 * Retains one contiguous suffix that fits the public conversation contract.
 * The API applies the same limits authoritatively and rejects credential-like
 * content; this helper only keeps browser-held context bounded between turns.
 */
export function boundedToolBuilderChatHistory(history: readonly APIToolBuilderChatMessage[]): APIToolBuilderChatMessage[] {
  const result: APIToolBuilderChatMessage[] = [];
  let totalBytes = 0;
  for (let index = history.length - 1; index >= 0 && result.length < TOOL_BUILDER_CHAT_LIMITS.maxMessages; index--) {
    const message = history[index];
    const content = message.content.trim();
    const contentBytes = utf8Bytes(content);
    const messageBytes = utf8Bytes(message.role) + contentBytes;
    if (!content || contentBytes > TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes) break;
    if (totalBytes + messageBytes > TOOL_BUILDER_CHAT_LIMITS.maxHistoryBytes) break;
    result.unshift({ role: message.role, content });
    totalBytes += messageBytes;
  }
  return result;
}

export type APIToolBuilderProposalInput = APIToolBuilderContext & {
  instruction: string;
  history?: APIToolBuilderChatMessage[];
};

export type APIToolBuilderImportKind = "openapi_document" | "postman" | "curl";

export type APIToolBuilderImportInput = APIToolBuilderContext & {
  source: {
    kind: APIToolBuilderImportKind;
    value: string;
  };
};

export type APIToolBuilderImportCandidate = {
  title?: string;
  summary?: string;
  valid?: boolean;
  draft: APIToolBuilderDraft;
  changes?: APIToolBuilderChange[];
  findings?: APIToolBuilderFinding[];
};

export type APIToolBuilderImportResult = {
  candidates: APIToolBuilderImportCandidate[];
  findings: APIToolBuilderFinding[];
  generated_at?: string;
};

export type APIToolBuilderValidationInput = APIToolBuilderContext;
export type APIToolBuilderAnalysisInput = APIToolBuilderContext;

type APIToolPersistedDraftInput = {
  description: string;
  http_method: string;
  endpoint?: string;
  runtime_service_connection_id?: string;
  http_path?: string;
  timeout_ms: number;
  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;
  authorization_policy: APIToolAuthorizationPolicy | Record<string, unknown>;
  request_example?: Record<string, unknown> | null;
  response_example?: unknown | null;
} & Partial<Pick<
  APIToolBuilderDraft,
  "upstream_auth" | "request_mapping" | "response_mapping"
>>;

export type APIToolCreateInput = APIToolPersistedDraftInput & {
  organisation_id: string;
  scope?: "common" | "api";
  owner_integration_id?: string;
  namespace: string;
  name: string;
  credential?: string;
};

export type APIToolUpdateInput = APIToolPersistedDraftInput & {
  revision: number;
  credential?: string;
};

export type APITool = {
  id: string;
  organisation_id: string;
  product_id: string;
  scope?: "common" | "api";
  owner_integration_id?: string;
  runtime_service_connection_id?: string;
  http_path?: string;
  namespace: string;
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;
  state: "draft" | "published" | "retired";
  revision: number;
  http_method: string;
  authorization_policy: Record<string, unknown>;
  timeout_ms: number;
  endpoint?: string;
  endpoint_requires_review?: boolean;
  upstream_auth?: APIToolUpstreamAuth;
  request_mapping?: APIToolRequestMapping;
  response_mapping?: APIToolResponseMapping;
  request_example?: Record<string, unknown>;
  response_example?: unknown;
  credential_present?: boolean;
  backend_kind?: "http" | "mcp";
  mcp_connection_id?: string;
  upstream_tool_name?: string;
  upstream_schema_hash?: string;
  upstream_drifted?: boolean;
  created_at?: string;
  updated_at?: string;
};

export type APIToolDryRun = {
  tool_id: string;
  revision: number;
  valid: boolean;
  network_call_performed: false;
  method: string;
  destination_origin?: string;
  destination_path?: string;
  backend_kind: string;
  required_grants: string[];
  confirmation_required: boolean;
  risk: "low" | "medium" | "high" | "critical";
  idempotency_required: boolean;
  normalized_arguments: Record<string, unknown>;
  warnings: string[];
};

export type APIToolTestConfirmationInput = {
  revision: number;
  arguments: Record<string, unknown>;
  typed_tool_name: string;
  acknowledge_side_effects: boolean;
};

export type APIToolTestConfirmation = {
  confirmation_nonce: string;
  expires_at: string;
  tool_id: string;
  tool_revision: number;
};

export type APIToolTestRunInput = {
  revision: number;
  arguments: Record<string, unknown>;
  confirmation_nonce?: string;
  idempotency_key?: string;
};

export type APIToolTestShape = {
  type: string;
  properties?: Record<string, APIToolTestShape>;
  items?: APIToolTestShape[];
  length?: number;
  truncated?: boolean;
};

export type APIToolTestFinding = {
  phase: string;
  code: string;
  message: string;
  instance_path?: string;
  schema_path?: string;
};

/** Sanitized evidence from an exact-revision upstream test. */
export type APIToolTestRun = {
  id: string;
  organisation_id: string;
  product_id: string;
  tool_id: string;
  tool_revision: number;
  tool_name: string;
  method: string;
  authentication_type: string;
  outcome: "success" | "failure";
  phase: string;
  network_call_performed: boolean;
  upstream_status_code?: number;
  response_bytes?: number;
  duration_ms: number;
  request_shape: APIToolTestShape;
  response_shape?: APIToolTestShape;
  findings: APIToolTestFinding[];
  evidence_hash: string;
  expires_at: string;
  created_at: string;
};

export type APIToolTestAnalysisMessage = {
  role: "user" | "assistant";
  content: string;
};

export type APIToolTestAnalysisInput = {
  revision: number;
  evidence_hash: string;
  consent_to_analysis_provider: boolean;
  question: string;
  history?: APIToolTestAnalysisMessage[];
};

export type APIToolTestAnalysisProposal = {
  proposal_id: string;
  base_tool_id: string;
  base_revision: number;
  base_fingerprint: string;
  requires_clone: boolean;
  draft: APIToolBuilderDraft;
  changes: APIToolBuilderChange[];
  findings: APIToolBuilderFinding[];
  valid: boolean;
};

export type APIToolTestAnalysis = {
  tool_revision: number;
  evidence_hash: string;
  reply: string;
  findings: APIToolBuilderFinding[];
  proposal?: APIToolTestAnalysisProposal;
  provider_outcome: "succeeded" | "unusable";
  advisory: true;
  generated_at: string;
};

export const TOOL_TEST_ANALYSIS_CHAT_LIMITS = {
  maxMessages: 12,
  maxMessageBytes: 2_048,
  maxHistoryBytes: 12_288,
} as const;

export function boundedToolTestAnalysisHistory(history: readonly APIToolTestAnalysisMessage[]): APIToolTestAnalysisMessage[] {
  const result: APIToolTestAnalysisMessage[] = [];
  let totalBytes = 0;
  for (let index = history.length - 1; index >= 0 && result.length < TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessages; index--) {
    const message = history[index];
    const content = message.content.trim();
    const messageBytes = utf8Bytes(message.role) + utf8Bytes(content);
    // History is one contiguous chronological suffix. Once an invalid older
    // boundary is reached, do not skip over it and resurrect still-older turns.
    if (!content || utf8Bytes(content) > TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes) break;
    if (totalBytes + messageBytes > TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxHistoryBytes) break;
    result.unshift({ role: message.role, content });
    totalBytes += messageBytes;
  }
  return result;
}

/** Exact value-free evidence object shown before provider consent. */
export function toolTestAnalysisEvidencePreview(run: APIToolTestRun) {
  return {
    method: run.method.toUpperCase(),
    authentication_type: run.authentication_type,
    outcome: run.outcome,
    phase: run.phase,
    network_call_performed: run.network_call_performed,
    ...(run.upstream_status_code ? { upstream_status_code: run.upstream_status_code } : {}),
    ...(run.response_bytes ? { response_bytes: run.response_bytes } : {}),
    duration_ms: run.duration_ms,
    request_shape: run.request_shape,
    ...(run.response_shape ? { response_shape: run.response_shape } : {}),
    findings: run.findings ?? [],
  };
}

/** Server-computed consent binding; the browser never re-serializes evidence. */
export async function toolTestAnalysisEvidenceHash(run: APIToolTestRun): Promise<string> {
  const value = run.evidence_hash.toLowerCase().trim();
  if (!/^sha256:[0-9a-f]{64}$/.test(value)) throw new Error("The server did not return a valid evidence preview hash.");
  return value;
}

export type APIIntegrationToolBinding = {
  integration_id: string;
  tool_id: string;
  tool_revision: number;
  authorization_point_id: string;
  authorization_point_revision: number;
  tool?: APITool;
  authorization_point?: APIAuthorizationPoint;
  created_by?: string;
  created_at: string;
};

export type APIGrantDefinition = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  key: string;
  display_name: string;
  description: string;
  risk: "low" | "medium" | "high" | "critical";
  state: "active" | "deprecated";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIAuthorizationPoint = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  integration_id: string;
  key: string;
  name: string;
  description: string;
  action_type: "read" | "write" | "destructive";
  required_grants: string[];
  confirmation_required: boolean;
  decision_ttl_seconds: number;
  state: "draft" | "active" | "deprecated";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIPackageRelease = {
  id: string;
  package_artifact_id: string;
  artifact_name: string;
  ecosystem: string;
  coordinate: string;
  version: string;
  purl: string;
  registry_url: string;
  source_url?: string;
  language?: string;
  platform?: string;
  install_command: string;
  digest: string;
  sbom_url?: string;
  provenance_url?: string;
  visibility: APIVisibility;
  content_hash: string;
  published_by?: string;
  published_at: string;
  created_at: string;
};

export type APIPackageArtifact = {
  id: string;
  organisation_id?: string;
  deployment_id?: string;
  ecosystem: string;
  name: string;
  description: string;
  coordinate: string;
  registry_url: string;
  source_url?: string;
  purl: string;
  language?: string;
  platform?: string;
  visibility: APIVisibility;
  lifecycle: "draft" | "active" | "deprecated" | "retired";
  replacement_package_artifact_id?: string;
  deprecation_message?: string;
  sunset_at?: string;
  revision: number;
  latest_release?: APIPackageRelease;
  integration_ids?: string[];
  releases?: APIPackageRelease[]; // Client-side enrichment from the releases endpoint.
  created_at: string;
  updated_at: string;
};

export type APIPackageArtifactInput = Pick<APIPackageArtifact, "name" | "description" | "ecosystem" | "coordinate" | "purl" | "registry_url" | "visibility"> & {
  source_url?: string;
  language?: string;
  platform?: string;
  acknowledge_public: boolean;
};

export type APIIntegrationPackageBinding = {
  id?: string;
  integration_id: string;
  package_artifact_id: string;
  package_release_id: string;
  artifact?: APIPackageArtifact;
  release?: APIPackageRelease;
  created_by?: string;
  created_at: string;
  updated_at: string;
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

export type APIIdentityTest = {
  id: string;
  status: "pending" | "passed" | "failed" | "expired";
  configuration_revision: number;
  authorization_url?: string;
  failure_code?: string;
  issuer?: string;
  customer_id?: string;
  created_at: string;
  expires_at: string;
  completed_at?: string;
};

export type APIIdentity = {
  id?: string;
  organisation_id: string;
  deployment_id: string;
  provider: "oidc";
  configured: boolean;
  credential_present: boolean;
  callback_url: string;
  access_evaluation_url: string;
  issuer: string;
  client_id: string;
  scopes: string[];
  audience: string;
  oauth_resource: string;
  customer_account_claim: string;
  installation_claim: string;
  authorization_api_origin: string;
  state: "active" | "disabled";
  revision: number;
  last_test?: APIIdentityTest;
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

export type APICustomerAccountPage = {
  items: APICustomerAccount[];
  has_more: boolean;
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
  const credentials = init?.credentials ?? "same-origin";
  const csrfToken = credentials !== "omit" && !["GET", "HEAD", "OPTIONS"].includes(method) ? cookie("dokosoko_csrf") : "";
  const multipartBody = typeof FormData !== "undefined" && init?.body instanceof FormData;
  const response = await fetch(path, {
    ...init,
    credentials,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(init?.body && !multipartBody ? { "Content-Type": "application/json" } : {}),
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

export type APIWidgetAgentSource = { kind: "recipe" | "documentation"; title: string; uri?: string; revision?: number; integration?: string };
export type APIWidgetAgentTrace = { intent: string; promptVersion: string; plannerVersion?: string; recipeCount: number; documentationCount: number; historyMessages: number; contextFacts: number; mcpSuggestionAllowed: boolean };

export async function streamWidgetMessage(sessionToken: string, message: string, onText: (text: string) => void, signal?: AbortSignal, onSource?: (source: APIWidgetAgentSource) => void, onTrace?: (trace: APIWidgetAgentTrace) => void): Promise<void> {
  const response = await fetch("/v1/widget-chat", {
    method: "POST",
    credentials: "omit",
    cache: "no-store",
    signal,
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${sessionToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ message }),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    const error = payload?.error ?? {};
    throw new APIError(response.status, error.code ?? "widget_request_failed", error.message ?? "The widget could not answer.", error.details);
  }
  if (!response.body) throw new APIError(502, "widget_stream_unavailable", "The widget response stream is unavailable.");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finished = false;
  const consume = (event: string) => {
    const data = event.split("\n").filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
    if (!data) return;
    if (data === "[DONE]") {
      finished = true;
      return;
    }
    try {
      const payload = JSON.parse(data) as { type?: unknown; text?: unknown; source?: unknown; trace?: unknown };
      if (payload.type === "source" && payload.source && typeof payload.source === "object") onSource?.(payload.source as APIWidgetAgentSource);
      else if (payload.type === "trace" && payload.trace && typeof payload.trace === "object") onTrace?.(payload.trace as APIWidgetAgentTrace);
      else if (typeof payload.text === "string") onText(payload.text);
    } catch {
      throw new APIError(502, "widget_stream_invalid", "The widget returned an invalid response stream.");
    }
  };

  while (!finished) {
    const { done, value } = await reader.read();
    buffer = (buffer + decoder.decode(value, { stream: !done })).replaceAll("\r\n", "\n");
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      consume(buffer.slice(0, boundary));
      buffer = buffer.slice(boundary + 2);
      if (finished) break;
      boundary = buffer.indexOf("\n\n");
    }
    if (done) {
      if (buffer.trim()) consume(buffer);
      break;
    }
  }
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
  widgetConfiguration: (widgetID: string) => request<APIWidgetConfiguration>(`/v1/widgets/${encodeURIComponent(widgetID)}/configuration`, { credentials: "omit" }),
  widgetPreviewBootstrap: (widgetID: string) => request<APIWidgetBootstrap>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/preview-session`, { method: "POST" }),
  exchangeWidgetSession: (bootstrapToken: string, origin: string) => request<APIWidgetRuntimeSession>("/v1/widget-sessions/exchange", { method: "POST", credentials: "omit", body: JSON.stringify({ bootstrapToken, origin }) }),
  integration: (integrationID: string) => request<APIIntegrationDetail>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`),
  integrationRuntimeSetup: (integrationID: string) => request<APIRuntimeSetup>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-setup`),
  configureIntegrationRuntimeSetup: (integrationID: string, input: APIRuntimeSetupInput) => request<APIRuntimeSetup>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-setup`, { method: "PUT", body: JSON.stringify(input) }),
  integrationRuntimeConnections: async (integrationID: string) => (await request<{ items: APIRuntimeServiceConnection[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-connections`)).items,
  createIntegrationRuntimeConnection: (integrationID: string, input: APIRuntimeServiceConnectionInput) => request<APIRuntimeServiceConnection>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-connections`, { method: "POST", body: JSON.stringify(input) }),
  checkRuntimeServiceConnection: (connectionID: string) => request<APIRuntimeServiceConnectionReadiness>(`/api/v1/runtime-service-connections/${encodeURIComponent(connectionID)}/check`, { method: "POST" }),
  createIntegrationRuntimeCredentialSet: (integrationID: string, input: APIRuntimeCredentialSetInput) => request<APIRuntimeCredentialSet>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-credential-sets`, { method: "POST", body: JSON.stringify(input) }),
  runtimeCredentialSet: (credentialSetID: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}`),
  runtimeCredentialUsage: (credentialSetID: string) => request<{ items: APIRuntimeServiceConnection[]; count: number }>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/usage`),
  rotateRuntimeCredential: (credentialSetID: string, credential: string, expiresAt?: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/rotate`, { method: "POST", body: JSON.stringify({ credential, ...(expiresAt ? { expires_at: expiresAt } : {}) }) }),
  revokeRuntimeCredentialVersion: (credentialSetID: string, versionID: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/versions/${encodeURIComponent(versionID)}/revoke`, { method: "POST" }),
  createIntegration: (input: { family_key: string; version_key: string; display_name: string; description: string; visibility?: APIVisibility; acknowledge_public?: boolean; lifecycle?: APIIntegration["lifecycle"] }) => request<APIIntegration>("/api/v1/integrations", { method: "POST", body: JSON.stringify(input) }),
  updateIntegration: (integrationID: string, input: Pick<APIIntegration, "family_key" | "version_key" | "display_name" | "description" | "visibility" | "lifecycle" | "revision"> & { acknowledge_public?: boolean; replacement_integration_id?: string; sunset_at?: string }) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`, { method: "PUT", body: JSON.stringify(input) }),
  preflightIntegration: (integrationID: string) => request<APIIntegrationPreflight>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/preflight`, { method: "POST", body: JSON.stringify({}) }),
  publishIntegration: (integrationID: string, candidateRevision: number, candidateManifestHash: string) => request<APIIntegrationRevision>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/publish`, { method: "POST", body: JSON.stringify({ candidate_revision: candidateRevision, candidate_manifest_hash: candidateManifestHash }) }),
  setIntegrationAccessConnections: (integrationID: string, accessConnectionIDs: string[]) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/access-connections`, { method: "PUT", body: JSON.stringify({ access_connection_ids: accessConnectionIDs }) }),
  setIntegrationSupportRoute: (integrationID: string, supportRouteID: string) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/support-route`, { method: "PUT", body: JSON.stringify({ support_route_id: supportRouteID }) }),
  integrationToolBindings: async (integrationID: string) => (await request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`)).items,
  setIntegrationToolBindings: (integrationID: string, tools: Array<{ tool_id: string; revision: number; authorization_point_id: string; authorization_point_revision: number }>) => request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`, { method: "PUT", body: JSON.stringify({ tools }) }),
  grantDefinitions: async () => (await request<{ items: APIGrantDefinition[] }>("/api/v1/grant-definitions")).items,
  createGrantDefinition: (input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"] }) => request<APIGrantDefinition>("/api/v1/grant-definitions", { method: "POST", body: JSON.stringify(input) }),
  updateGrantDefinition: (grantID: string, input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"]; revision: number }) => request<APIGrantDefinition>(`/api/v1/grant-definitions/${encodeURIComponent(grantID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  authorizationPoints: async (integrationID: string) => (await request<{ items: APIAuthorizationPoint[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`)).items,
  createAuthorizationPoint: (integrationID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"] }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`, { method: "POST", body: JSON.stringify(input) }),
  updateAuthorizationPoint: (integrationID: string, pointID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"]; revision: number }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points/${encodeURIComponent(pointID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  integrationPackages: async (integrationID: string) => (await request<{ items: APIIntegrationPackageBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages`)).items,
  bindIntegrationPackage: (integrationID: string, packageReleaseID: string) => request<APIIntegrationPackageBinding>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages`, { method: "POST", body: JSON.stringify({ package_release_id: packageReleaseID }) }),
  replaceIntegrationPackage: (integrationID: string, artifactID: string, packageReleaseID: string) => request<APIIntegrationPackageBinding>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages/${encodeURIComponent(artifactID)}`, { method: "PUT", body: JSON.stringify({ package_release_id: packageReleaseID }) }),
  unbindIntegrationPackage: (integrationID: string, artifactID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages/${encodeURIComponent(artifactID)}`, { method: "DELETE" }),
  resourceSets: async (kind = "") => (await request<{ items: APIResourceSet[] }>(`/api/v1/resource-sets${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`)).items,
  createResourceSet: (input: { kind: APIResourceSet["kind"]; name: string; description: string; manifest: Array<Record<string, unknown>> }) => request<APIResourceSet>("/api/v1/resource-sets", { method: "POST", body: JSON.stringify(input) }),
  updateResourceSet: (setID: string, input: { name: string; description: string; state: APIResourceSet["state"]; manifest: Array<Record<string, unknown>>; revision: number }) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  duplicateResourceSet: (setID: string, name: string) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}/duplicate`, { method: "POST", body: JSON.stringify({ name }) }),
  attachResourceSet: (integrationID: string, resourceSetID: string, pinnedRevisionID = "") => request<APIIntegrationResourceLink>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets`, { method: "POST", body: JSON.stringify({ resource_set_id: resourceSetID, pinned_revision_id: pinnedRevisionID }) }),
  detachResourceSet: (integrationID: string, resourceSetID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets/${encodeURIComponent(resourceSetID)}`, { method: "DELETE" }),
  accessDefinitions: async () => (await request<{ items: APIAccessDefinition[] }>("/api/v1/access-definitions")).items,
  createAccessDefinition: (input: { service_key: string; name: string; instance_cardinality: APIAccessDefinition["instance_cardinality"]; instance_label_singular: string; instance_label_plural: string; credential_scope: APIAccessDefinition["credential_scope"]; management_auth_type: APIAccessDefinition["management_auth_type"]; api_resource_set_id?: string; operations: Record<string, unknown> }) => request<APIAccessDefinition>("/api/v1/access-definitions", { method: "POST", body: JSON.stringify(input) }),
  updateAccessDefinition: (definitionID: string, input: { name: string; instance_label_singular: string; instance_label_plural: string; api_resource_set_id?: string; operations: Record<string, unknown>; revision: number }) => request<APIAccessDefinition>(`/api/v1/access-definitions/${encodeURIComponent(definitionID)}`, { method: "PUT", body: JSON.stringify(input) }),
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
  customerAccounts: (productID: string, startingAfter = "") => request<APICustomerAccountPage>(`${productPath(productID)}/customer-accounts?limit=200${startingAfter ? `&starting_after=${encodeURIComponent(startingAfter)}` : ""}`),
  updateCustomerAccount: (productID: string, accountID: string, state: APICustomerAccount["state"], revision: number) => request<APICustomerAccount>(`${productPath(productID)}/customer-accounts/${encodeURIComponent(accountID)}`, { method: "PATCH", body: JSON.stringify({ state, revision }) }),
  productDefinition: (productID: string) => request<APIProductDefinition>(`${productPath(productID)}/definition`),
  productBuilds: async (productID: string) => (await request<{ items: APIProductBuild[] }>(`${productPath(productID)}/product-builds`)).items,
  buildProduct: (productID: string, inputs: APIProductBuildInput[]) => request<APIProductBuild>(`${productPath(productID)}/product-builds`, { method: "POST", body: JSON.stringify({ inputs }) }),
  publishProductBuild: (productID: string, buildID: string) => request<APIProductDefinition>(`${productPath(productID)}/product-builds/${encodeURIComponent(buildID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  environments: async (productID: string) => (await request<{ items: APIEnvironment[] }>(`${productPath(productID)}/environments`)).items,
  createEnvironment: (productID: string, organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>(`${productPath(productID)}/environments`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  distribution: (productID: string) => request<Distribution>(`${productPath(productID)}/distribution`),
  identity: () => request<APIIdentity>("/api/v1/identity-provider"),
  saveIdentityDraft: (input: { provider: "oidc"; issuer: string; client_id: string; client_secret: string; scopes: string[]; audience: string; oauth_resource: string; customer_account_claim: string; installation_claim: string; authorization_api_origin: string; revision: number }) => request<APIIdentity>("/api/v1/identity-provider", { method: "PUT", body: JSON.stringify(input) }),
  beginIdentityTest: (revision: number) => request<APIIdentityTest>("/api/v1/identity-provider/tests", { method: "POST", body: JSON.stringify({ revision }) }),
  identityTest: (testID: string) => request<APIIdentityTest>(`/api/v1/identity-provider/tests/${encodeURIComponent(testID)}`),
  activateIdentity: (revision: number, testID: string) => request<APIIdentity>("/api/v1/identity-provider/activate", { method: "POST", body: JSON.stringify({ revision, test_id: testID }) }),
  disableIdentity: (revision: number) => request<APIIdentity>("/api/v1/identity-provider/disable", { method: "POST", body: JSON.stringify({ revision }) }),
  disconnectIdentity: (revision: number) => request<APIIdentity>("/api/v1/identity-provider", { method: "DELETE", body: JSON.stringify({ revision }) }),
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
  analyseIntegration: (productID: string, integrationID?: string) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) }),
  answerAnalysis: (productID: string, analysisID: string, answers: Record<string, string>) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}`, { method: "PATCH", body: JSON.stringify({ answers }) }),
  generateRecipes: async (productID: string, analysisID: string, integrationID?: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}/recipes`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) })).items,
  recipes: async (productID: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/recipes`)).items,
  createRecipe: (productID: string, prompt: string, integrationID = "") => request<APIRecipe>(`${productPath(productID)}/recipes`, { method: "POST", body: JSON.stringify({ prompt, integration_id: integrationID }) }),
  recipe: (productID: string, recipeID: string) => request<{ recipe: APIRecipe; revisions: APIRecipeRevision[] }>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`),
  updateRecipe: (productID: string, recipeID: string, markdown: string, references: APIRecipeReference[], visibility: APIVisibility) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`, { method: "PATCH", body: JSON.stringify({ markdown, references, visibility }) }),
  reworkRecipe: (productID: string, recipeID: string, instruction: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/rework`, { method: "POST", body: JSON.stringify({ instruction }) }),
  approveRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/approve`, { method: "POST", body: JSON.stringify({}) }),
  publishRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  recipeAnalytics: async (productID: string, days = 30) => (await request<{ items: APIRecipePopularity[] }>(`${productPath(productID)}/recipe-analytics?days=${days}`)).items,
  aiUsage: (productID: string, days = 30) => request<{ workloads: APIAIWorkloadUsage[]; providers: APIAIProviderUsage[] }>(`${productPath(productID)}/ai-usage?days=${days}`),
  sources: async (productID: string) => (await request<{ items: APISource[] }>(`${productPath(productID)}/sources`)).items,
  createSource: (productID: string, organisationID: string, name: string, kind: string, location: string) => request<APISource>(`${productPath(productID)}/sources`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, kind, location }) }),
  uploadSource: (productID: string, organisationID: string, name: string, file: File) => {
    const body = new FormData();
    body.append("organisation_id", organisationID);
    body.append("name", name);
    body.append("file", file, file.name);
    return request<APISource>(`${productPath(productID)}/sources/upload`, { method: "POST", body });
  },
  queueCrawl: (productID: string, sourceID: string) => request<APICrawlJob>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawl`, { method: "POST" }),
  crawlJobs: async (productID: string, sourceID: string) => (await request<{ items: APICrawlJob[] }>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawls`)).items,
  sourceReview: (productID: string, sourceID: string, crawlJobID = "") => request<APISourceReview>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/review${crawlJobID ? `?crawl_job_id=${encodeURIComponent(crawlJobID)}` : ""}`),
  sourcePublications: async (productID: string, sourceID: string) => (await request<{ items: APISourcePublication[] }>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/publications`)).items,
  publishSource: (productID: string, sourceID: string, input: { revision: number; crawl_job_id: string; document_ids: string[]; acknowledge_reviewed: boolean }) => request<APISourcePublishResult>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/publish`, { method: "POST", body: JSON.stringify(input) }),
  tools: async (productID: string) => (await request<{ items: APITool[] }>(`${productPath(productID)}/tools`)).items,
  tool: (productID: string, toolID: string) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}`),
  createTool: (productID: string, input: APIToolCreateInput) => request<APITool>(`${productPath(productID)}/tools`, { method: "POST", body: JSON.stringify(input) }),
  updateTool: (productID: string, toolID: string, input: APIToolUpdateInput) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}`, { method: "PUT", body: JSON.stringify(input) }),
  proposeToolDraft: (productID: string, input: APIToolBuilderProposalInput) => request<APIToolBuilderProposal>(`${productPath(productID)}/tool-builder/propose`, { method: "POST", body: JSON.stringify({ ...input, ...(input.history ? { history: boundedToolBuilderChatHistory(input.history) } : {}) }) }),
  importToolDraft: (productID: string, input: APIToolBuilderImportInput) => request<APIToolBuilderImportResult>(`${productPath(productID)}/tool-builder/import`, { method: "POST", body: JSON.stringify(input) }),
  validateToolDraft: (productID: string, input: APIToolBuilderValidationInput) => request<APIToolBuilderValidation>(`${productPath(productID)}/tool-builder/validate`, { method: "POST", body: JSON.stringify(input) }),
  analyseToolDraft: (productID: string, input: APIToolBuilderAnalysisInput) => request<APIToolBuilderAnalysis>(`${productPath(productID)}/tool-builder/analyse`, { method: "POST", body: JSON.stringify(input) }),
  cloneTool: (productID: string, toolID: string, revision: number, namespace: string, name: string, credential = "") => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/clone`, { method: "POST", body: JSON.stringify({ revision, namespace, name, ...(credential ? { credential } : {}) }) }),
  dryRunTool: (productID: string, toolID: string, args: Record<string, unknown>) => request<APIToolDryRun>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/dry-run`, { method: "POST", body: JSON.stringify({ arguments: args }) }),
  createToolTestConfirmation: (productID: string, toolID: string, input: APIToolTestConfirmationInput) => request<APIToolTestConfirmation>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-confirmations`, { method: "POST", body: JSON.stringify(input) }),
  runToolTest: (productID: string, toolID: string, input: APIToolTestRunInput) => request<APIToolTestRun>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-runs`, { method: "POST", body: JSON.stringify(input) }),
  analyseToolTestRun: (productID: string, toolID: string, runID: string, input: APIToolTestAnalysisInput) => request<APIToolTestAnalysis>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-runs/${encodeURIComponent(runID)}/analyse`, { method: "POST", body: JSON.stringify({ ...input, ...(input.history ? { history: boundedToolTestAnalysisHistory(input.history) } : {}) }) }),
  retireTool: (productID: string, toolID: string, revision: number) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/retire`, { method: "POST", body: JSON.stringify({ revision }) }),
  publishTool: (productID: string, toolID: string, revision: number) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/publish`, { method: "POST", body: JSON.stringify({ revision }) }),
  packageArtifacts: async () => (await request<{ items: APIPackageArtifact[] }>("/api/v1/package-artifacts")).items,
  packageArtifact: (artifactID: string) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}`),
  createPackageArtifact: (input: APIPackageArtifactInput) => request<APIPackageArtifact>("/api/v1/package-artifacts", { method: "POST", body: JSON.stringify(input) }),
  updatePackageArtifact: (artifactID: string, input: APIPackageArtifactInput & { revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}`, { method: "PUT", body: JSON.stringify(input) }),
  packageReleases: async (artifactID: string) => (await request<{ items: APIPackageRelease[] }>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/releases`)).items,
  publishPackageRelease: (artifactID: string, input: { version: string; purl: string; install_command: string; digest: string; provenance_url?: string; sbom_url?: string; artifact_revision: number; acknowledge_public: boolean }) => request<{ artifact: APIPackageArtifact; release: APIPackageRelease }>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/publish`, { method: "POST", body: JSON.stringify(input) }),
  deprecatePackageArtifact: (artifactID: string, input: { replacement_package_artifact_id?: string; message: string; sunset_at?: string; revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/deprecate`, { method: "POST", body: JSON.stringify(input) }),
  retirePackageArtifact: (artifactID: string, input: { replacement_package_artifact_id?: string; message: string; revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/retire`, { method: "POST", body: JSON.stringify(input) }),
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
