import type * as Contract from "./control-plane.generated";

export type APIVisibility = Contract.Visibility;

export type APIProduct = Omit<Contract.Product, "created_at" | "updated_at">;

export type APIDeployment = Omit<Contract.Deployment, "description" | "created_at" | "updated_at"> & { description: string };

export type APIResourceSetRevision = Omit<Contract.ResourceSetRevision, "manifest"> & { manifest: Array<Record<string, unknown>> };

export type APIIntegrationResourceLink = Omit<Contract.IntegrationResourceLink, "resolved_revision"> & { resolved_revision?: APIResourceSetRevision };

export type APISDKReferenceInput = Contract.SdkReferenceInput;

export type APISDKReference = Contract.SdkReference;

export type APIIntegration = Omit<Contract.Integration, "resources" | "sdks" | "sunset_at" | "created_at" | "updated_at"> & {
  sunset_at?: string;
  resources?: APIIntegrationResourceLink[];
  sdks?: APISDKReference[];
};

export type APIIntegrationRevision = Omit<Contract.IntegrationRevision, "state" | "published_at"> & { state: string; published_at?: string };

export type APIIntegrationPublishChange = Contract.IntegrationPublishChange;

export type APIIntegrationPublishValidation = Omit<Contract.IntegrationPublishValidation, "tab"> & { tab: string };

export type APIIntegrationPublishStatus = Omit<Contract.IntegrationPublishStatus, "latest_revision" | "changes" | "validations"> & { latest_revision?: APIIntegrationRevision; changes: APIIntegrationPublishChange[]; validations: APIIntegrationPublishValidation[] };

export type APIIntegrationPreflightCheck = Contract.IntegrationPreflightCheck;

export type APIIntegrationPreflight = Omit<Contract.IntegrationPreflight, "checks"> & { checks: APIIntegrationPreflightCheck[] };

export type APIIntegrationDetail = {
  integration: APIIntegration;
  revisions: APIIntegrationRevision[];
  publish_status: APIIntegrationPublishStatus;
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

export type APIOrganisation = Contract.Organisation;

export type APIEnvironment = Omit<Contract.Environment, "created_at" | "updated_at">;

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

export type APISource = Contract.Source;

export type APICrawlJob = Contract.CrawlJob;

export type APICrawlReviewDocument = Contract.CrawlReviewDocument;

export type APISourcePublication = Contract.SourcePublication;

export type APISourceReview = Omit<Contract.SourceReview, "source" | "crawl_job" | "documents" | "publication"> & { source: APISource; crawl_job: APICrawlJob; documents: APICrawlReviewDocument[]; publication?: APISourcePublication };

export type APISourcePublishResult = Omit<Contract.SourcePublishResult, "source" | "publication"> & { source: APISource; publication: APISourcePublication };

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
  backend_kind?: "http" | "mcp" | "native";
  effect?: "read" | "write" | "destructive";
  idempotency_mode?: "none" | "supported" | "required";
  identity_requirement?: "none" | "optional" | "actor_required" | "customer_required" | "actor_and_customer_required" | "installation_required";
  state_scope?: "none" | "plugin" | "actor" | "customer" | "installation";
  max_concurrency?: number;
  max_result_bytes?: number;
  mcp_connection_id?: string;
  upstream_tool_name?: string;
  upstream_schema_hash?: string;
  upstream_drifted?: boolean;
  native_plugin_id?: string;
  native_tool_id?: string;
  native_plugin_version?: string;
  native_sdk_version?: number;
  native_manifest_hash?: string;
  native_contract_hash?: string;
  created_at?: string;
  updated_at?: string;
};

export type APINativePluginConfigStatus = {
  key: string;
  environment: string;
  type: "string" | "secret" | "boolean" | "integer" | "duration" | "url";
  required: boolean;
  secret: boolean;
  description: string;
  configured: boolean;
  source?: "environment";
};

export type APINativePluginToolStatus = {
  id: string;
  name: string;
  effect: "read" | "write" | "destructive";
  identity: "none" | "optional" | "actor_required" | "customer_required" | "actor_and_customer_required" | "installation_required";
  state_scope: "none" | "plugin" | "actor" | "customer" | "installation";
  confirmation_required: boolean;
  idempotency: "none" | "supported" | "required";
};

export type APINativePlugin = {
  id: string;
  version: string;
  sdk_version: number;
  description: string;
  state: "discovered" | "misconfigured" | "upgrading" | "active" | "failed" | "disabled" | "incompatible" | "missing";
  state_version: number;
  manifest_hash: string;
  required: boolean;
  managed_by_environment: boolean;
  configuration: APINativePluginConfigStatus[];
  tools: APINativePluginToolStatus[];
  network: Array<{ host?: string; config_key?: string }>;
  capabilities: string[];
  last_error_code?: string;
  last_error?: string;
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

export type APIIntegrationToolBinding = Omit<Contract.IntegrationToolBinding, "tool" | "authorization_point"> & {
  tool?: APITool;
  authorization_point?: APIAuthorizationPoint;
};

export type APIGrantDefinition = Contract.GrantDefinition;

export type APIAuthorizationPoint = Contract.AuthorizationPoint;

export type APIMCPConnection = Contract.McpConnection;

export type APIMCPCatalogTool = Contract.McpCatalogTool;

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

export type APISupportSubmission = Contract.SupportSubmission;

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

export type APIRecipeReference = Contract.RecipeReference;
export type APIRecipeFinding = Contract.RecipeValidationFinding;
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

export type APIAuditEvent = Contract.AuditEvent;
