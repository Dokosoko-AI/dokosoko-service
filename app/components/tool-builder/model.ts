import { APIError } from "../../lib/api";
import type {
  APIGrantDefinition,
  APITool,
  APIToolBuilderChange,
  APIToolBuilderDraft,
  APIToolBuilderFinding,
  APIToolBuilderImportKind,
  APIToolHTTPMethod,
  APIToolRequestMapping,
  APIToolRisk,
  APIToolUpstreamAuthType,
} from "../../lib/api";
import { apiToolHTTPPath } from "../../lib/tool-builder-runtime-context";

export type ToolDraftForm = Omit<
  APIToolBuilderDraft,
  "input_schema" | "output_schema" | "request_example" | "response_example"
> & {
  input_schema_text: string;
  output_schema_text: string;
  request_example_text: string;
  response_example_text: string;
};

export type LocalValidation = {
  draft: APIToolBuilderDraft | null;
  findings: APIToolBuilderFinding[];
};

export type ReviewChange = {
  field: ReviewField;
  label: string;
  before: unknown;
  after: unknown;
  rationale?: string;
  securitySensitive: boolean;
};

export type ActiveProposal = {
  source: "ai" | "import" | "live-test";
  summary: string;
  draft: APIToolBuilderDraft;
  changes: ReviewChange[];
  findings: APIToolBuilderFinding[];
};

export type ProposalDecision = "accepted" | "rejected";

export const HTTP_METHODS: APIToolHTTPMethod[] = ["GET", "POST", "PUT", "PATCH", "DELETE"];
export const RISKS: APIToolRisk[] = ["low", "medium", "high", "critical"];
export const AUTH_TYPES: APIToolUpstreamAuthType[] = [
  "delegated_oauth",
  "none",
  "bearer",
  "authorization_scheme",
  "api_key_header",
  "api_key_query",
  "basic",
  "oauth_client_credentials",
  "custom_header",
];
export const IMPORT_KINDS: APIToolBuilderImportKind[] = ["openapi_document", "postman", "curl"];
export const CREDENTIAL_AUTH_TYPES = new Set<APIToolUpstreamAuthType>([
  "bearer",
  "authorization_scheme",
  "api_key_header",
  "api_key_query",
  "basic",
  "oauth_client_credentials",
  "custom_header",
]);
export const IDENTIFIER_PATTERN = /^[a-z][a-z0-9_]{0,63}$/;
export const PARAMETER_PATTERN = /^[A-Za-z][A-Za-z0-9_.-]{0,63}$/;
export const CREDENTIAL_MATERIAL_PATTERN = /(?:\b(?:authorization|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|client[-_ ]?secret|password|secret)\s*[:=]\s*["']?[^\s,"'}]{8,}|\beyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\b|\b(?:sk|pk|rk|ghp|gho|xox[baprs])[-_][A-Za-z0-9_-]{8,}\b|\bhttps?:\/\/[^\s/?#@]+@[^\s/?#]+)/i;
export const BEARER_MATERIAL_PATTERN = /\bbearer\s+([A-Za-z0-9._~+/=-]{8,})/ig;
export const BASIC_MATERIAL_PATTERN = /\bbasic\s+([A-Za-z0-9+/=]{8,})/ig;
export const NON_SECRET_AUTH_TERMS = new Set(["authentication", "authorization", "credential", "credentials", "token", "tokens"]);
export const DEFAULT_SCHEMA: Record<string, unknown> = { type: "object", additionalProperties: false, properties: {} };
export const REVIEW_FIELDS = [
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
] as const;
export type ReviewField = (typeof REVIEW_FIELDS)[number];
export const SECURITY_SENSITIVE_FIELDS = new Set<ReviewField>([
  "http_method",
  "endpoint",
  "upstream_auth",
  "request_mapping",
  "authorization_policy",
]);

export function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

export function stringValue(value: unknown, fallback = "") {
  return typeof value === "string" ? value : fallback;
}

export function stringArray(value: unknown, fallback: string[] = []) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : fallback;
}

export function httpMethod(value: unknown, fallback: APIToolHTTPMethod = "GET"): APIToolHTTPMethod {
  return HTTP_METHODS.includes(value as APIToolHTTPMethod) ? value as APIToolHTTPMethod : fallback;
}

export function riskValue(value: unknown, fallback: APIToolRisk = "low"): APIToolRisk {
  return RISKS.includes(value as APIToolRisk) ? value as APIToolRisk : fallback;
}

export function authType(value: unknown, fallback: APIToolUpstreamAuthType = "delegated_oauth"): APIToolUpstreamAuthType {
  return AUTH_TYPES.includes(value as APIToolUpstreamAuthType) ? value as APIToolUpstreamAuthType : fallback;
}

export function containsCredentialMaterial(value: string) {
  if (CREDENTIAL_MATERIAL_PATTERN.test(value)) return true;
  for (const pattern of [BEARER_MATERIAL_PATTERN, BASIC_MATERIAL_PATTERN]) {
    for (const match of value.matchAll(pattern)) {
      const candidate = (match[1] ?? "").toLowerCase().replace(/[.,;:!?]+$/, "");
      if (candidate && !NON_SECRET_AUTH_TERMS.has(candidate)) return true;
    }
  }
  return false;
}

export function utf8ByteLength(value: string) {
  return new TextEncoder().encode(value).byteLength;
}

export function defaultToolDraft(): APIToolBuilderDraft {
  return {
    namespace: "",
    name: "",
    description: "",
    http_method: "GET",
    endpoint: "",
    timeout_ms: 10000,
    input_schema: DEFAULT_SCHEMA,
    output_schema: DEFAULT_SCHEMA,
    upstream_auth: { type: "delegated_oauth" },
    request_mapping: { parameter_locations: {} },
    response_mapping: {},
    authorization_policy: {
      required_grants: [],
      confirmation_required: false,
      risk: "low",
      idempotency_required: false,
    },
    credential_present: false,
  };
}

export function toolDraftFromTool(tool?: APITool | null): APIToolBuilderDraft {
  const fallback = defaultToolDraft();
  if (!tool) return fallback;
  const policy = isRecord(tool.authorization_policy) ? tool.authorization_policy : {};
  return sanitizeDraft({
    namespace: tool.namespace,
    name: tool.name,
    description: tool.description,
    http_method: tool.http_method,
    endpoint: tool.endpoint,
    timeout_ms: tool.timeout_ms,
    input_schema: tool.input_schema,
    output_schema: tool.output_schema,
    upstream_auth: tool.upstream_auth,
    request_mapping: tool.request_mapping,
    response_mapping: tool.response_mapping,
    authorization_policy: {
      required_grants: policy.required_grants,
      confirmation_required: policy.confirmation_required,
      risk: policy.risk,
      idempotency_required: policy.idempotency_required,
    },
    request_example: tool.request_example,
    response_example: tool.response_example,
    credential_present: Boolean(tool.credential_present),
  }, fallback);
}

export function sanitizeDraft(value: unknown, fallback = defaultToolDraft()): APIToolBuilderDraft {
  const source = isRecord(value) ? value : {};
  const sourceAuth = isRecord(source.upstream_auth) ? source.upstream_auth : fallback.upstream_auth;
  const sourceRequestMapping = isRecord(source.request_mapping) ? source.request_mapping : fallback.request_mapping;
  const locations = isRecord(sourceRequestMapping.parameter_locations) ? sourceRequestMapping.parameter_locations : {};
  const parameterLocations: APIToolRequestMapping["parameter_locations"] = {};
  for (const [key, location] of Object.entries(locations)) {
    if (["path", "query", "header", "body"].includes(String(location))) {
      parameterLocations[key] = location as APIToolRequestMapping["parameter_locations"][string];
    }
  }
  const sourceResponseMapping = isRecord(source.response_mapping) ? source.response_mapping : fallback.response_mapping;
  const sourcePolicy = isRecord(source.authorization_policy) ? source.authorization_policy : fallback.authorization_policy;
  const inputSchema = isRecord(source.input_schema) ? source.input_schema : fallback.input_schema;
  const outputSchema = isRecord(source.output_schema) ? source.output_schema : fallback.output_schema;
  const requestExample = isRecord(source.request_example) ? source.request_example : undefined;
  const next: APIToolBuilderDraft = {
    namespace: stringValue(source.namespace, fallback.namespace),
    name: stringValue(source.name, fallback.name),
    description: stringValue(source.description, fallback.description),
    http_method: httpMethod(source.http_method, fallback.http_method),
    endpoint: stringValue(source.endpoint, fallback.endpoint),
    timeout_ms: typeof source.timeout_ms === "number" && Number.isFinite(source.timeout_ms) ? source.timeout_ms : fallback.timeout_ms,
    input_schema: { ...inputSchema },
    output_schema: { ...outputSchema },
    upstream_auth: {
      type: authType(sourceAuth.type, fallback.upstream_auth.type),
      ...(stringValue(sourceAuth.scheme) ? { scheme: stringValue(sourceAuth.scheme) } : {}),
      ...(stringValue(sourceAuth.header_name) ? { header_name: stringValue(sourceAuth.header_name) } : {}),
      ...(stringValue(sourceAuth.query_name) ? { query_name: stringValue(sourceAuth.query_name) } : {}),
      ...(stringValue(sourceAuth.prefix) ? { prefix: stringValue(sourceAuth.prefix) } : {}),
      ...(stringValue(sourceAuth.username) ? { username: stringValue(sourceAuth.username) } : {}),
      ...(stringValue(sourceAuth.client_id) ? { client_id: stringValue(sourceAuth.client_id) } : {}),
      ...(stringValue(sourceAuth.token_url) ? { token_url: stringValue(sourceAuth.token_url) } : {}),
      ...(sourceAuth.token_endpoint_auth_method === "client_secret_post" ? { token_endpoint_auth_method: "client_secret_post" as const } : sourceAuth.token_endpoint_auth_method === "client_secret_basic" ? { token_endpoint_auth_method: "client_secret_basic" as const } : {}),
      ...(stringArray(sourceAuth.scopes).length ? { scopes: stringArray(sourceAuth.scopes) } : {}),
      ...(stringValue(sourceAuth.audience) ? { audience: stringValue(sourceAuth.audience) } : {}),
      ...(stringValue(sourceAuth.resource) ? { resource: stringValue(sourceAuth.resource) } : {}),
    },
    request_mapping: { parameter_locations: parameterLocations },
    response_mapping: stringValue(sourceResponseMapping.result_path) ? { result_path: stringValue(sourceResponseMapping.result_path) } : {},
    authorization_policy: {
      required_grants: [...new Set(stringArray(sourcePolicy.required_grants))],
      confirmation_required: Boolean(sourcePolicy.confirmation_required),
      risk: riskValue(sourcePolicy.risk, fallback.authorization_policy.risk),
      idempotency_required: Boolean(sourcePolicy.idempotency_required),
    },
    ...(requestExample ? { request_example: { ...requestExample } } : {}),
    ...(source.response_example !== undefined ? { response_example: source.response_example } : {}),
    credential_present: typeof source.credential_present === "boolean" ? source.credential_present : fallback.credential_present,
  };
  if (next.authorization_policy.risk === "critical") next.authorization_policy.confirmation_required = true;
  return next;
}

export function draftToForm(draft: APIToolBuilderDraft): ToolDraftForm {
  const { input_schema, output_schema, request_example, response_example, ...rest } = draft;
  return {
    ...rest,
    input_schema_text: JSON.stringify(input_schema, null, 2),
    output_schema_text: JSON.stringify(output_schema, null, 2),
    request_example_text: request_example === undefined ? "" : JSON.stringify(request_example, null, 2),
    response_example_text: response_example === undefined ? "" : JSON.stringify(response_example, null, 2),
  };
}

export function draftForAssistance(form: ToolDraftForm): APIToolBuilderDraft {
  const parseObject = (value: string) => {
    try {
      const parsed: unknown = JSON.parse(value);
      return isRecord(parsed) ? parsed : undefined;
    } catch {
      return undefined;
    }
  };
  const requestExample = form.request_example_text.trim() ? parseObject(form.request_example_text) : undefined;
  let responseExample: unknown;
  if (form.response_example_text.trim()) {
    try {
      responseExample = JSON.parse(form.response_example_text);
    } catch {
      responseExample = undefined;
    }
  }
  return sanitizeDraft({
    ...form,
    input_schema: parseObject(form.input_schema_text) ?? DEFAULT_SCHEMA,
    output_schema: parseObject(form.output_schema_text) ?? DEFAULT_SCHEMA,
    ...(requestExample ? { request_example: requestExample } : {}),
    ...(responseExample !== undefined ? { response_example: responseExample } : {}),
  });
}

export function parseObjectJSON(text: string, field: string, findings: APIToolBuilderFinding[]) {
  try {
    const parsed: unknown = JSON.parse(text);
    if (!isRecord(parsed)) {
      findings.push({ level: "error", code: "object_required", field, message: field });
      return null;
    }
    return parsed;
  } catch {
    findings.push({ level: "error", code: "invalid_json", field, message: field });
    return null;
  }
}

export function localValidation(form: ToolDraftForm, grants: APIGrantDefinition[], credential: string): LocalValidation {
  const findings: APIToolBuilderFinding[] = [];
  if (!IDENTIFIER_PATTERN.test(form.namespace)) findings.push({ level: "error", code: "invalid_namespace", field: "namespace", message: "invalid_namespace" });
  if (!IDENTIFIER_PATTERN.test(form.name)) findings.push({ level: "error", code: "invalid_name", field: "name", message: "invalid_name" });
  if (!form.description.trim()) findings.push({ level: "error", code: "description_required", field: "description", message: "description_required" });
  if (form.description.trim().length > 500) findings.push({ level: "error", code: "description_too_long", field: "description", message: "description_too_long" });

  if (!Number.isInteger(form.timeout_ms) || form.timeout_ms < 100 || form.timeout_ms > 60000) {
    findings.push({ level: "error", code: "invalid_timeout", field: "timeout_ms", message: "invalid_timeout" });
  }

  if (!form.endpoint.trim()) {
    findings.push({ level: "error", code: "endpoint_required", field: "endpoint", message: "endpoint_required" });
  } else {
    try {
      const endpoint = new URL(form.endpoint);
      const localHostname = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || ["127.0.0.1", "::1", "[::1]"].includes(endpoint.hostname);
      const localDevelopment = endpoint.protocol === "http:" && localHostname;
      if (endpoint.protocol !== "https:" && !localDevelopment) findings.push({ level: "error", code: "https_required", field: "endpoint", message: "https_required" });
      if (endpoint.protocol === "https:" && localHostname) findings.push({ level: "error", code: "localhost_http_required", field: "endpoint", message: "localhost_http_required" });
      if (endpoint.protocol === "https:" && endpoint.port) findings.push({ level: "error", code: "default_https_port_required", field: "endpoint", message: "default_https_port_required" });
      if (endpoint.username || endpoint.password) findings.push({ level: "error", code: "endpoint_credentials", field: "endpoint", message: "endpoint_credentials" });
      if (endpoint.search) findings.push({ level: "error", code: "endpoint_query", field: "endpoint", message: "endpoint_query" });
      if (endpoint.hash) findings.push({ level: "error", code: "endpoint_fragment", field: "endpoint", message: "endpoint_fragment" });
    } catch {
      findings.push({ level: "error", code: "invalid_endpoint", field: "endpoint", message: "invalid_endpoint" });
    }
  }

  const inputSchema = parseObjectJSON(form.input_schema_text, "input_schema", findings);
  const outputSchema = parseObjectJSON(form.output_schema_text, "output_schema", findings);
  let requestExample: Record<string, unknown> | undefined;
  let responseExample: unknown;
  if (form.request_example_text.trim()) {
    requestExample = parseObjectJSON(form.request_example_text, "request_example", findings) ?? undefined;
  }
  if (form.response_example_text.trim()) {
    try {
      responseExample = JSON.parse(form.response_example_text);
    } catch {
      findings.push({ level: "error", code: "invalid_json", field: "response_example", message: "response_example" });
    }
  }

  for (const [parameter, location] of Object.entries(form.request_mapping.parameter_locations)) {
    if (!PARAMETER_PATTERN.test(parameter)) findings.push({ level: "error", code: "mapping_name_invalid", field: "request_mapping", message: parameter });
    if (location === "path" && !form.endpoint.includes(`{${parameter}}`)) findings.push({ level: "warning", code: "path_parameter_missing", field: "request_mapping", message: parameter });
    if (form.http_method === "GET" && location === "body") findings.push({ level: "error", code: "get_body_mapping", field: "request_mapping", message: parameter });
  }

  const upstreamAuth = form.upstream_auth;
  if (upstreamAuth.type === "authorization_scheme" && !/^[!#$%&'*+.^_`|~A-Za-z0-9-]{1,64}$/.test(upstreamAuth.scheme?.trim() ?? "")) findings.push({ level: "error", code: "authorization_scheme_required", field: "upstream_auth.scheme", message: "authorization_scheme_required" });
  if (["api_key_header", "custom_header"].includes(upstreamAuth.type) && !upstreamAuth.header_name?.trim()) findings.push({ level: "error", code: "header_name_required", field: "upstream_auth.header_name", message: "header_name_required" });
  if (upstreamAuth.type === "api_key_query" && !upstreamAuth.query_name?.trim()) findings.push({ level: "error", code: "query_name_required", field: "upstream_auth.query_name", message: "query_name_required" });
  if (upstreamAuth.type === "basic" && !upstreamAuth.username?.trim()) findings.push({ level: "error", code: "username_required", field: "upstream_auth.username", message: "username_required" });
  if (upstreamAuth.type === "oauth_client_credentials") {
    if (!upstreamAuth.client_id?.trim()) findings.push({ level: "error", code: "client_id_required", field: "upstream_auth.client_id", message: "client_id_required" });
    if (!upstreamAuth.token_url?.trim()) findings.push({ level: "error", code: "token_url_required", field: "upstream_auth.token_url", message: "token_url_required" });
    else {
      try {
        const tokenURL = new URL(upstreamAuth.token_url);
        const localTokenHost = tokenURL.hostname === "localhost" || tokenURL.hostname.endsWith(".localhost") || ["127.0.0.1", "::1", "[::1]"].includes(tokenURL.hostname);
        if ((localTokenHost && tokenURL.protocol !== "http:") || (!localTokenHost && (tokenURL.protocol !== "https:" || Boolean(tokenURL.port))) || tokenURL.username || tokenURL.password || tokenURL.search || tokenURL.hash) findings.push({ level: "error", code: "invalid_token_url", field: "upstream_auth.token_url", message: "invalid_token_url" });
      } catch {
        findings.push({ level: "error", code: "invalid_token_url", field: "upstream_auth.token_url", message: "invalid_token_url" });
      }
    }
  }
  const credentialPresent = CREDENTIAL_AUTH_TYPES.has(upstreamAuth.type) && (form.credential_present || Boolean(credential.trim()));
  if (CREDENTIAL_AUTH_TYPES.has(upstreamAuth.type) && !credentialPresent) findings.push({ level: "error", code: "credential_required", field: "credential", message: "credential_required" });

  const grantMap = new Map(grants.map((grant) => [grant.key, grant]));
  for (const key of form.authorization_policy.required_grants) {
    const grant = grantMap.get(key);
    if (!grant) findings.push({ level: "error", code: "unknown_grant", field: "authorization_policy.required_grants", message: key });
    else if (grant.state !== "active") findings.push({ level: "warning", code: "deprecated_grant", field: "authorization_policy.required_grants", message: key });
  }
  if (form.authorization_policy.risk === "critical" && !form.authorization_policy.confirmation_required) findings.push({ level: "error", code: "critical_confirmation", field: "authorization_policy.confirmation_required", message: "critical_confirmation" });
  if (form.http_method !== "GET" && !form.authorization_policy.idempotency_required) findings.push({ level: "error", code: "mutation_without_idempotency", field: "authorization_policy.idempotency_required", message: "mutation_without_idempotency" });

  if (findings.some((finding) => finding.level === "error") || !inputSchema || !outputSchema) return { draft: null, findings };
  return {
    draft: {
      namespace: form.namespace,
      name: form.name,
      description: form.description.trim(),
      http_method: form.http_method,
      endpoint: form.endpoint.trim(),
      timeout_ms: form.timeout_ms,
      input_schema: inputSchema,
      output_schema: outputSchema,
      upstream_auth: { ...form.upstream_auth },
      request_mapping: { parameter_locations: { ...form.request_mapping.parameter_locations } },
      response_mapping: form.response_mapping.result_path?.trim() ? { result_path: form.response_mapping.result_path.trim() } : {},
      authorization_policy: {
        ...form.authorization_policy,
        required_grants: [...new Set(form.authorization_policy.required_grants)].sort(),
        confirmation_required: form.authorization_policy.risk === "critical" || form.authorization_policy.confirmation_required,
      },
      ...(requestExample ? { request_example: requestExample } : {}),
      ...(form.response_example_text.trim() ? { response_example: responseExample } : {}),
      credential_present: credentialPresent,
    },
    findings,
  };
}

export function canonicalValue(draft: APIToolBuilderDraft, field: ReviewField): unknown {
  return draft[field];
}

export function stableValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableValue).join(",")}]`;
  if (isRecord(value)) return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableValue(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

export function endpointOrigin(value: string): string {
  try {
    return new URL(value).origin.toLowerCase();
  } catch {
    return "";
  }
}

export function reviewChanges(base: APIToolBuilderDraft, proposed: APIToolBuilderDraft, serverChanges: APIToolBuilderChange[], editing: boolean, apiScoped = false): ReviewChange[] {
  return REVIEW_FIELDS.flatMap((field) => {
    if (editing && (field === "namespace" || field === "name")) return [];
    const before = apiScoped && field === "endpoint" ? apiToolHTTPPath(base.endpoint) : canonicalValue(base, field);
    const after = apiScoped && field === "endpoint" ? apiToolHTTPPath(proposed.endpoint) : canonicalValue(proposed, field);
    if (stableValue(before) === stableValue(after)) return [];
    const serverChange = serverChanges.find((change) => change.field === field || change.field.startsWith(`${field}.`));
    return [{
      field,
      label: field,
      before,
      after,
      rationale: serverChange?.rationale ?? serverChange?.summary,
      securitySensitive: serverChange?.security_sensitive ?? SECURITY_SENSITIVE_FIELDS.has(field),
    }];
  });
}

export function formatReviewValue(value: unknown, emptyLabel = "—"): string {
  if (value === undefined || value === "") return emptyLabel;
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

export function errorMessage(error: unknown, fallback: string) {
  if (error instanceof APIError) return error.message;
  return error instanceof Error ? error.message : fallback;
}
