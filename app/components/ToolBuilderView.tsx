"use client";

import {
  ArrowLeft,
  Bot,
  Check,
  CheckCircle2,
  KeyRound,
  Plus,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  TriangleAlert,
  Wrench,
  XCircle,
} from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import {
  APIError,
  api,
  boundedToolBuilderChatHistory,
  toolBuilderFollowUpDraft,
  TOOL_BUILDER_CHAT_LIMITS,
} from "../lib/api";
import type {
  APIGrantDefinition,
  APIIntegration,
  APIProduct,
  APIRuntimeSetup,
  APITool,
  APIToolAuthorizationPolicy,
  APIToolBuilderAnalysis,
  APIToolBuilderChange,
  APIToolBuilderChatMessage,
  APIToolBuilderDraft,
  APIToolBuilderFinding,
  APIToolBuilderImportCandidate,
  APIToolBuilderImportKind,
  APIToolBuilderProposal,
  APIToolBuilderValidation,
  APIToolHTTPMethod,
  APIToolRequestMapping,
  APIToolRisk,
  APIToolUpstreamAuth,
  APIToolUpstreamAuthType,
} from "../lib/api";
import { integrationPath } from "../lib/console-routes";
import {
  apiToolHTTPPath,
  apiToolHTTPPathProblem,
  apiToolPersistenceContext,
  lockAPIToolBuilderDraft,
  runtimeLockForConnection,
} from "../lib/tool-builder-runtime-context";
import {
  toolCredentialBinding,
  toolCredentialBindingMatches,
  versionedResponseIsCurrent,
} from "../lib/tool-builder-safety";
import { Badge, Button } from "./core/control";
import { PageHeader, PanelHeader, SegmentedControl } from "./core/layout";

export type ToolBuilderMode = "ai" | "import" | "manual";

export type ToolBuilderViewProps = {
  product: APIProduct;
  grants: APIGrantDefinition[];
  tool?: APITool | null;
  initialProposal?: APIToolBuilderProposal | null;
  aiAvailable: boolean;
  initialMode?: ToolBuilderMode;
  apiContext?: { integration: APIIntegration; setup: APIRuntimeSetup };
  onNavigate: (path: string) => void;
  onMessage: (message: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
  onSaved?: (tool: APITool) => void | Promise<void>;
};

type ToolDraftForm = Omit<
  APIToolBuilderDraft,
  "input_schema" | "output_schema" | "request_example" | "response_example"
> & {
  input_schema_text: string;
  output_schema_text: string;
  request_example_text: string;
  response_example_text: string;
};

type LocalValidation = {
  draft: APIToolBuilderDraft | null;
  findings: APIToolBuilderFinding[];
};

type ReviewChange = {
  field: ReviewField;
  label: string;
  before: unknown;
  after: unknown;
  rationale?: string;
  securitySensitive: boolean;
};

type ActiveProposal = {
  source: "ai" | "import" | "live-test";
  summary: string;
  draft: APIToolBuilderDraft;
  changes: ReviewChange[];
  findings: APIToolBuilderFinding[];
};

type ProposalDecision = "accepted" | "rejected";

const HTTP_METHODS: APIToolHTTPMethod[] = ["GET", "POST", "PUT", "PATCH", "DELETE"];
const RISKS: APIToolRisk[] = ["low", "medium", "high", "critical"];
const AUTH_TYPES: Array<{ value: APIToolUpstreamAuthType; label: string; description: string }> = [
  { value: "delegated_oauth", label: "Delegated OAuth", description: "Forward the current user's delegated access token." },
  { value: "none", label: "No authentication", description: "Call an endpoint that intentionally requires no upstream credential." },
  { value: "bearer", label: "Bearer token", description: "Store one encrypted bearer token for this tool." },
  { value: "authorization_scheme", label: "Authorization scheme", description: "Send an encrypted credential with a fixed vendor scheme such as Token, ApiKey, or SSWS." },
  { value: "api_key_header", label: "API key header", description: "Send one encrypted API key in a fixed header." },
  { value: "api_key_query", label: "API key query parameter", description: "Send one encrypted API key in a fixed query parameter." },
  { value: "basic", label: "HTTP Basic", description: "Store an encrypted password for the configured username." },
  { value: "oauth_client_credentials", label: "OAuth client credentials", description: "Exchange an encrypted client secret at the fixed token URL." },
  { value: "custom_header", label: "Custom secret header", description: "Send one encrypted value in a fixed custom header." },
];
const IMPORT_KINDS: Array<{ value: APIToolBuilderImportKind; label: string }> = [
  { value: "openapi_document", label: "OpenAPI document" },
  { value: "postman", label: "Postman collection" },
  { value: "curl", label: "cURL command" },
];
const CREDENTIAL_AUTH_TYPES = new Set<APIToolUpstreamAuthType>([
  "bearer",
  "authorization_scheme",
  "api_key_header",
  "api_key_query",
  "basic",
  "oauth_client_credentials",
  "custom_header",
]);
const IDENTIFIER_PATTERN = /^[a-z][a-z0-9_]{0,63}$/;
const PARAMETER_PATTERN = /^[A-Za-z][A-Za-z0-9_.-]{0,63}$/;
const CREDENTIAL_MATERIAL_PATTERN = /(?:\b(?:authorization|api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|client[-_ ]?secret|password|secret)\s*[:=]\s*["']?[^\s,"'}]{8,}|\beyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\b|\b(?:sk|pk|rk|ghp|gho|xox[baprs])[-_][A-Za-z0-9_-]{8,}\b|\bhttps?:\/\/[^\s/?#@]+@[^\s/?#]+)/i;
const BEARER_MATERIAL_PATTERN = /\bbearer\s+([A-Za-z0-9._~+/=-]{8,})/ig;
const BASIC_MATERIAL_PATTERN = /\bbasic\s+([A-Za-z0-9+/=]{8,})/ig;
const NON_SECRET_AUTH_TERMS = new Set(["authentication", "authorization", "credential", "credentials", "token", "tokens"]);
const DEFAULT_SCHEMA: Record<string, unknown> = { type: "object", additionalProperties: false, properties: {} };
const REVIEW_FIELDS = [
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
type ReviewField = (typeof REVIEW_FIELDS)[number];
const REVIEW_FIELD_LABELS: Record<ReviewField, string> = {
  namespace: "Namespace",
  name: "Tool name",
  description: "Purpose",
  http_method: "HTTP method",
  endpoint: "Endpoint",
  timeout_ms: "Timeout",
  input_schema: "Input schema",
  output_schema: "Output schema",
  upstream_auth: "Upstream authentication",
  request_mapping: "Request mapping",
  response_mapping: "Response mapping",
  authorization_policy: "Authorization policy",
  request_example: "Request example",
  response_example: "Response example",
};
const SECURITY_SENSITIVE_FIELDS = new Set<ReviewField>([
  "http_method",
  "endpoint",
  "upstream_auth",
  "request_mapping",
  "authorization_policy",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringValue(value: unknown, fallback = "") {
  return typeof value === "string" ? value : fallback;
}

function stringArray(value: unknown, fallback: string[] = []) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : fallback;
}

function httpMethod(value: unknown, fallback: APIToolHTTPMethod = "GET"): APIToolHTTPMethod {
  return HTTP_METHODS.includes(value as APIToolHTTPMethod) ? value as APIToolHTTPMethod : fallback;
}

function riskValue(value: unknown, fallback: APIToolRisk = "low"): APIToolRisk {
  return RISKS.includes(value as APIToolRisk) ? value as APIToolRisk : fallback;
}

function authType(value: unknown, fallback: APIToolUpstreamAuthType = "delegated_oauth"): APIToolUpstreamAuthType {
  return AUTH_TYPES.some((candidate) => candidate.value === value) ? value as APIToolUpstreamAuthType : fallback;
}

function containsCredentialMaterial(value: string) {
  if (CREDENTIAL_MATERIAL_PATTERN.test(value)) return true;
  for (const pattern of [BEARER_MATERIAL_PATTERN, BASIC_MATERIAL_PATTERN]) {
    for (const match of value.matchAll(pattern)) {
      const candidate = (match[1] ?? "").toLowerCase().replace(/[.,;:!?]+$/, "");
      if (candidate && !NON_SECRET_AUTH_TERMS.has(candidate)) return true;
    }
  }
  return false;
}

function utf8ByteLength(value: string) {
  return new TextEncoder().encode(value).byteLength;
}

function defaultToolDraft(): APIToolBuilderDraft {
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

function toolDraftFromTool(tool?: APITool | null): APIToolBuilderDraft {
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

function sanitizeDraft(value: unknown, fallback = defaultToolDraft()): APIToolBuilderDraft {
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

function draftToForm(draft: APIToolBuilderDraft): ToolDraftForm {
  const { input_schema, output_schema, request_example, response_example, ...rest } = draft;
  return {
    ...rest,
    input_schema_text: JSON.stringify(input_schema, null, 2),
    output_schema_text: JSON.stringify(output_schema, null, 2),
    request_example_text: request_example === undefined ? "" : JSON.stringify(request_example, null, 2),
    response_example_text: response_example === undefined ? "" : JSON.stringify(response_example, null, 2),
  };
}

function draftForAssistance(form: ToolDraftForm): APIToolBuilderDraft {
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

function parseObjectJSON(text: string, field: string, label: string, findings: APIToolBuilderFinding[]) {
  try {
    const parsed: unknown = JSON.parse(text);
    if (!isRecord(parsed)) {
      findings.push({ level: "error", code: "object_required", field, message: `${label} must be a JSON object.` });
      return null;
    }
    return parsed;
  } catch {
    findings.push({ level: "error", code: "invalid_json", field, message: `${label} is not valid JSON.` });
    return null;
  }
}

function localValidation(form: ToolDraftForm, grants: APIGrantDefinition[], credential: string): LocalValidation {
  const findings: APIToolBuilderFinding[] = [];
  if (!IDENTIFIER_PATTERN.test(form.namespace)) findings.push({ level: "error", code: "invalid_namespace", field: "namespace", message: "Use 1–64 lower-case letters, numbers, or underscores, starting with a letter." });
  if (!IDENTIFIER_PATTERN.test(form.name)) findings.push({ level: "error", code: "invalid_name", field: "name", message: "Use 1–64 lower-case letters, numbers, or underscores, starting with a letter." });
  if (!form.description.trim()) findings.push({ level: "error", code: "description_required", field: "description", message: "Describe the single action this tool performs." });
  if (form.description.trim().length > 500) findings.push({ level: "error", code: "description_too_long", field: "description", message: "Purpose must be 500 characters or fewer." });

  if (!Number.isInteger(form.timeout_ms) || form.timeout_ms < 100 || form.timeout_ms > 60000) {
    findings.push({ level: "error", code: "invalid_timeout", field: "timeout_ms", message: "Timeout must be a whole number from 100 to 60,000 milliseconds." });
  }

  if (!form.endpoint.trim()) {
    findings.push({ level: "error", code: "endpoint_required", field: "endpoint", message: "Enter the fixed upstream endpoint." });
  } else {
    try {
      const endpoint = new URL(form.endpoint);
      const localHostname = endpoint.hostname === "localhost" || endpoint.hostname.endsWith(".localhost") || ["127.0.0.1", "::1", "[::1]"].includes(endpoint.hostname);
      const localDevelopment = endpoint.protocol === "http:" && localHostname;
      if (endpoint.protocol !== "https:" && !localDevelopment) findings.push({ level: "error", code: "https_required", field: "endpoint", message: "Use HTTPS, except for a localhost development endpoint." });
      if (endpoint.protocol === "https:" && localHostname) findings.push({ level: "error", code: "localhost_http_required", field: "endpoint", message: "Use HTTP for localhost development endpoints." });
      if (endpoint.protocol === "https:" && endpoint.port) findings.push({ level: "error", code: "default_https_port_required", field: "endpoint", message: "Public HTTPS endpoints must use the default port." });
      if (endpoint.username || endpoint.password) findings.push({ level: "error", code: "endpoint_credentials", field: "endpoint", message: "Do not put credentials in the endpoint URL." });
      if (endpoint.search) findings.push({ level: "error", code: "endpoint_query", field: "endpoint", message: "Move fixed query values into request mapping; the endpoint URL cannot contain a query string." });
      if (endpoint.hash) findings.push({ level: "error", code: "endpoint_fragment", field: "endpoint", message: "Remove the URL fragment from the endpoint." });
    } catch {
      findings.push({ level: "error", code: "invalid_endpoint", field: "endpoint", message: "Enter a valid absolute URL." });
    }
  }

  const inputSchema = parseObjectJSON(form.input_schema_text, "input_schema", "Input schema", findings);
  const outputSchema = parseObjectJSON(form.output_schema_text, "output_schema", "Output schema", findings);
  let requestExample: Record<string, unknown> | undefined;
  let responseExample: unknown;
  if (form.request_example_text.trim()) {
    requestExample = parseObjectJSON(form.request_example_text, "request_example", "Request example", findings) ?? undefined;
  }
  if (form.response_example_text.trim()) {
    try {
      responseExample = JSON.parse(form.response_example_text);
    } catch {
      findings.push({ level: "error", code: "invalid_json", field: "response_example", message: "Response example is not valid JSON." });
    }
  }

  for (const [parameter, location] of Object.entries(form.request_mapping.parameter_locations)) {
    if (!PARAMETER_PATTERN.test(parameter)) findings.push({ level: "error", code: "mapping_name_invalid", field: "request_mapping", message: `Request parameter “${parameter || "(empty)"}” must start with a letter and use only letters, numbers, dots, dashes, or underscores.` });
    if (location === "path" && !form.endpoint.includes(`{${parameter}}`)) findings.push({ level: "warning", code: "path_parameter_missing", field: "request_mapping", message: `Path parameter “${parameter}” does not appear as {${parameter}} in the endpoint.` });
    if (form.http_method === "GET" && location === "body") findings.push({ level: "error", code: "get_body_mapping", field: "request_mapping", message: `GET parameter “${parameter}” cannot be mapped to the request body.` });
  }

  const upstreamAuth = form.upstream_auth;
  if (upstreamAuth.type === "authorization_scheme" && !/^[!#$%&'*+.^_`|~A-Za-z0-9-]{1,64}$/.test(upstreamAuth.scheme?.trim() ?? "")) findings.push({ level: "error", code: "authorization_scheme_required", field: "upstream_auth.scheme", message: "Enter a valid fixed authorization scheme, such as Token, ApiKey, or SSWS." });
  if (["api_key_header", "custom_header"].includes(upstreamAuth.type) && !upstreamAuth.header_name?.trim()) findings.push({ level: "error", code: "header_name_required", field: "upstream_auth.header_name", message: "Enter the fixed header name." });
  if (upstreamAuth.type === "api_key_query" && !upstreamAuth.query_name?.trim()) findings.push({ level: "error", code: "query_name_required", field: "upstream_auth.query_name", message: "Enter the fixed query parameter name." });
  if (upstreamAuth.type === "basic" && !upstreamAuth.username?.trim()) findings.push({ level: "error", code: "username_required", field: "upstream_auth.username", message: "Enter the Basic authentication username." });
  if (upstreamAuth.type === "oauth_client_credentials") {
    if (!upstreamAuth.client_id?.trim()) findings.push({ level: "error", code: "client_id_required", field: "upstream_auth.client_id", message: "Enter the OAuth client ID." });
    if (!upstreamAuth.token_url?.trim()) findings.push({ level: "error", code: "token_url_required", field: "upstream_auth.token_url", message: "Enter the fixed OAuth token URL." });
    else {
      try {
        const tokenURL = new URL(upstreamAuth.token_url);
        const localTokenHost = tokenURL.hostname === "localhost" || tokenURL.hostname.endsWith(".localhost") || ["127.0.0.1", "::1", "[::1]"].includes(tokenURL.hostname);
        if ((localTokenHost && tokenURL.protocol !== "http:") || (!localTokenHost && (tokenURL.protocol !== "https:" || Boolean(tokenURL.port))) || tokenURL.username || tokenURL.password || tokenURL.search || tokenURL.hash) findings.push({ level: "error", code: "invalid_token_url", field: "upstream_auth.token_url", message: "Use a fixed public HTTPS token URL, or HTTP on localhost for development." });
      } catch {
        findings.push({ level: "error", code: "invalid_token_url", field: "upstream_auth.token_url", message: "Enter a valid absolute OAuth token URL." });
      }
    }
  }
  const credentialPresent = CREDENTIAL_AUTH_TYPES.has(upstreamAuth.type) && (form.credential_present || Boolean(credential.trim()));
  if (CREDENTIAL_AUTH_TYPES.has(upstreamAuth.type) && !credentialPresent) findings.push({ level: "error", code: "credential_required", field: "credential", message: "Enter the upstream credential. It is sent only when you save the draft." });

  const grantMap = new Map(grants.map((grant) => [grant.key, grant]));
  for (const key of form.authorization_policy.required_grants) {
    const grant = grantMap.get(key);
    if (!grant) findings.push({ level: "error", code: "unknown_grant", field: "authorization_policy.required_grants", message: `Grant “${key}” is not registered.` });
    else if (grant.state !== "active") findings.push({ level: "warning", code: "deprecated_grant", field: "authorization_policy.required_grants", message: `Grant “${key}” is deprecated.` });
  }
  if (form.authorization_policy.risk === "critical" && !form.authorization_policy.confirmation_required) findings.push({ level: "error", code: "critical_confirmation", field: "authorization_policy.confirmation_required", message: "Critical tools must require explicit confirmation." });
  if (form.http_method !== "GET" && !form.authorization_policy.idempotency_required) findings.push({ level: "error", code: "mutation_without_idempotency", field: "authorization_policy.idempotency_required", message: "Mutation tools require idempotency metadata before they can be saved or published." });

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

function canonicalValue(draft: APIToolBuilderDraft, field: ReviewField): unknown {
  return draft[field];
}

function stableValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableValue).join(",")}]`;
  if (isRecord(value)) return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableValue(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

function endpointOrigin(value: string): string {
  try {
    return new URL(value).origin.toLowerCase();
  } catch {
    return "";
  }
}

function reviewChanges(base: APIToolBuilderDraft, proposed: APIToolBuilderDraft, serverChanges: APIToolBuilderChange[], editing: boolean, apiScoped = false): ReviewChange[] {
  return REVIEW_FIELDS.flatMap((field) => {
    if (editing && (field === "namespace" || field === "name")) return [];
    const before = apiScoped && field === "endpoint" ? apiToolHTTPPath(base.endpoint) : canonicalValue(base, field);
    const after = apiScoped && field === "endpoint" ? apiToolHTTPPath(proposed.endpoint) : canonicalValue(proposed, field);
    if (stableValue(before) === stableValue(after)) return [];
    const serverChange = serverChanges.find((change) => change.field === field || change.field.startsWith(`${field}.`));
    return [{
      field,
      label: apiScoped && field === "endpoint" ? "Relative path" : REVIEW_FIELD_LABELS[field],
      before,
      after,
      rationale: serverChange?.rationale ?? serverChange?.summary,
      securitySensitive: serverChange?.security_sensitive ?? SECURITY_SENSITIVE_FIELDS.has(field),
    }];
  });
}

function formatReviewValue(value: unknown): string {
  if (value === undefined || value === "") return "Not set";
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

function errorMessage(error: unknown) {
  if (error instanceof APIError) return error.message;
  return error instanceof Error ? error.message : "The request could not be completed.";
}

function BuilderLink({ path, onNavigate, className, children }: { path: string; onNavigate: (path: string) => void; className?: string; children: ReactNode }) {
  return <a href={path} className={className} onClick={(event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }}>{children}</a>;
}

function FindingList({ findings, onOpen }: { findings: APIToolBuilderFinding[]; onOpen: (field?: string) => void }) {
  if (findings.length === 0) return <div className="tool-builder-ready"><CheckCircle2 /><span><strong>No findings</strong><small>The current fields pass available checks.</small></span></div>;
  return <div className="tool-builder-findings">{findings.map((finding, index) => <button type="button" className={`tool-builder-finding ${finding.level}`} key={`${finding.code}:${finding.field ?? "general"}:${index}`} onClick={() => onOpen(finding.field)}><span>{finding.level === "error" ? <XCircle /> : finding.level === "warning" ? <TriangleAlert /> : <CheckCircle2 />}</span><span><strong>{finding.code.replaceAll("_", " ")}</strong><small>{finding.message}</small></span></button>)}</div>;
}

export function ToolBuilderView({ product, grants, tool = null, initialProposal = null, aiAvailable, initialMode = "ai", apiContext, onNavigate, onMessage, onDirtyChange, onSaved }: ToolBuilderViewProps) {
  const generatedID = useId().replaceAll(":", "");
  const apiScoped = Boolean(apiContext);
  const runtimeConnections = apiContext?.setup.service_connections ?? [];
  const initialRuntimeConnectionID = tool?.runtime_service_connection_id ?? runtimeConnections[0]?.id ?? "";
  const initialRuntimeConnection = runtimeConnections.find((connection) => connection.id === initialRuntimeConnectionID);
  const initialRuntimeLock = apiContext ? runtimeLockForConnection(apiContext.setup, initialRuntimeConnection) : null;
  const unscopedInitialCanonical = useMemo(() => {
    const draft = toolDraftFromTool(tool);
    if (!tool && apiContext) {
      const namespace = apiContext.integration.family_key.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
      return { ...draft, namespace: IDENTIFIER_PATTERN.test(namespace) ? namespace : "api" };
    }
    return draft;
  }, [apiContext, tool]);
  const initialHTTPPath = tool?.http_path || apiToolHTTPPath(unscopedInitialCanonical.endpoint, "/");
  const initialCanonical = useMemo(() => initialRuntimeLock
    ? lockAPIToolBuilderDraft(unscopedInitialCanonical, initialRuntimeLock, initialHTTPPath)
    : unscopedInitialCanonical,
  [initialHTTPPath, initialRuntimeLock, unscopedInitialCanonical]);
  const initialForm = useMemo(() => draftToForm(initialCanonical), [initialCanonical]);
  const editing = Boolean(tool);
  const seededProposal = useMemo<ActiveProposal | null>(() => {
    if (!initialProposal) return null;
    const sanitized = sanitizeDraft(initialProposal.draft, initialCanonical);
    const proposalPath = apiScoped ? apiToolHTTPPath(sanitized.endpoint, initialHTTPPath) : "";
    const draft = initialRuntimeLock ? lockAPIToolBuilderDraft(sanitized, initialRuntimeLock, proposalPath) : sanitized;
    const changes = reviewChanges(initialCanonical, draft, initialProposal.changes ?? [], editing, apiScoped);
    return {
      source: "live-test",
      summary: initialProposal.summary || initialProposal.reply || "Suggested from consented sanitized live-test evidence. Nothing has been applied or saved.",
      draft,
      changes,
      findings: initialProposal.findings ?? [],
    };
  }, [apiScoped, editing, initialCanonical, initialHTTPPath, initialProposal, initialRuntimeLock]);
  const [form, setForm] = useState<ToolDraftForm>(initialForm);
  const [runtimeConnectionID, setRuntimeConnectionID] = useState(initialRuntimeConnectionID);
  const [runtimeHTTPPath, setRuntimeHTTPPath] = useState(initialHTTPPath);
  const [credential, setCredential] = useState("");
  const [credentialBinding, setCredentialBinding] = useState("");
  const [mode, setMode] = useState<ToolBuilderMode>(initialMode);
  const [instruction, setInstruction] = useState("");
  const [chatHistory, setChatHistory] = useState<APIToolBuilderChatMessage[]>([]);
  const [importKind, setImportKind] = useState<APIToolBuilderImportKind>("openapi_document");
  const [importSource, setImportSource] = useState("");
  const [importCandidates, setImportCandidates] = useState<APIToolBuilderImportCandidate[]>([]);
  const [proposal, setProposal] = useState<ActiveProposal | null>(seededProposal);
  const [proposalDecisions, setProposalDecisions] = useState<Record<string, ProposalDecision>>({});
  const [proposalStale, setProposalStale] = useState(false);
  const [validation, setValidation] = useState<APIToolBuilderValidation | null>(null);
  const [analysis, setAnalysis] = useState<APIToolBuilderAnalysis | null>(null);
  const [busy, setBusy] = useState<"propose" | "import" | "validate" | "analyse" | "save" | null>(null);
  const [status, setStatus] = useState(seededProposal ? `Live-test proposal ready with ${seededProposal.changes.length} change${seededProposal.changes.length === 1 ? "" : "s"}. Review each field; nothing has been saved.` : "");
  const proposalHeadingRef = useRef<HTMLHeadingElement>(null);
  // A seeded proposal is a distinct in-memory draft state. Subsequent request
  // guards compare this monotonically increasing version before using results.
  const draftVersionRef = useRef(seededProposal ? 1 : 0);
  const importInputVersionRef = useRef(0);
  const editable = !tool || tool.state === "draft";
  const runtimeConnection = runtimeConnections.find((connection) => connection.id === runtimeConnectionID);
  const runtimeLock = apiContext ? runtimeLockForConnection(apiContext.setup, runtimeConnection) : null;
  const runtimePathProblem = apiScoped ? apiToolHTTPPathProblem(runtimeHTTPPath) : "";
  const contextualForm = useMemo(() => runtimeLock
    ? draftToForm(lockAPIToolBuilderDraft(draftForAssistance(form), runtimeLock, runtimeHTTPPath))
    : form,
  [form, runtimeHTTPPath, runtimeLock]);
  const storedCredentialReusable = apiScoped || !tool || !form.credential_present || !CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) || (
    endpointOrigin(form.endpoint) === endpointOrigin(initialForm.endpoint)
    && stableValue(form.upstream_auth) === stableValue(initialForm.upstream_auth)
  );
  const enteredCredentialReusable = !credential || toolCredentialBindingMatches(
    credentialBinding,
    form.endpoint,
    form.upstream_auth,
  );
  const validationForm = useMemo(() => storedCredentialReusable ? contextualForm : { ...contextualForm, credential_present: false }, [contextualForm, storedCredentialReusable]);
  const validationCredential = enteredCredentialReusable ? credential : "";
  const local = useMemo(() => {
    const result = localValidation(validationForm, grants, validationCredential);
    if (!apiScoped) return result;
    const runtimeFindings: APIToolBuilderFinding[] = [
      ...(!runtimeLock ? [{ level: "error" as const, code: "runtime_connection_required", field: "runtime_service_connection_id", message: "Choose a configured service connection from this API's Access setup." }] : []),
      ...(runtimePathProblem ? [{ level: "error" as const, code: "invalid_http_path", field: "endpoint", message: runtimePathProblem }] : []),
    ];
    return { draft: runtimeFindings.length > 0 ? null : result.draft, findings: [...result.findings, ...runtimeFindings] };
  }, [apiScoped, grants, runtimeLock, runtimePathProblem, validationCredential, validationForm]);
  const assistanceDraft = useMemo(() => draftForAssistance(validationForm), [validationForm]);
  const followUpDraft = useMemo(() => toolBuilderFollowUpDraft(
    assistanceDraft,
    proposal?.draft ?? null,
    proposalDecisions,
    proposalStale,
    editing,
  ), [assistanceDraft, editing, proposal, proposalDecisions, proposalStale]);
  const instructionBytes = useMemo(() => utf8ByteLength(instruction.trim()), [instruction]);
  const instructionProblem = useMemo(() => {
    if (containsCredentialMaterial(instruction)) return "Remove credential-like material and enter it only in the separate credential field.";
    if (instructionBytes > TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes) return `Keep each message within ${TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} UTF-8 bytes.`;
    return "";
  }, [instruction, instructionBytes]);
  const dirty = useMemo(() => stableValue(form) !== stableValue(initialForm) || Boolean(credential) || runtimeConnectionID !== initialRuntimeConnectionID || runtimeHTTPPath !== initialHTTPPath, [credential, form, initialForm, initialHTTPPath, initialRuntimeConnectionID, runtimeConnectionID, runtimeHTTPPath]);
  const findings = useMemo(() => {
    const combined = [...local.findings, ...(validation?.findings ?? [])];
    const seen = new Set<string>();
    return combined.filter((finding) => {
      const key = `${finding.level}:${finding.code}:${finding.field ?? ""}:${finding.message}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [local.findings, validation]);
  const errors = findings.filter((finding) => finding.level === "error");
  const warnings = findings.filter((finding) => finding.level === "warning");

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);

  function domID(field: string) {
    return `${generatedID}-tool-${field.replace(/[^a-z0-9_-]/gi, "-")}`;
  }

  function focusID(field: string) {
    const root = field === "credential_present" ? "credential" : field.split(".", 1)[0];
    return domID(root);
  }

  function describedBy(field: string, helpID?: string) {
    const hasFinding = findings.some((finding) => finding.field === field || finding.field?.startsWith(`${field}.`) || (field === "credential" && finding.field === "credential_present"));
    return [helpID, hasFinding ? `${domID(field)}-finding` : ""].filter(Boolean).join(" ") || undefined;
  }

  function fieldFindings(field: string) {
    return findings.filter((finding) => finding.field === field || finding.field?.startsWith(`${field}.`) || (field === "credential" && finding.field === "credential_present"));
  }

  function renderFieldFindings(field: string) {
    const values = fieldFindings(field);
    if (values.length === 0) return null;
    return <span className="tool-builder-inline-findings" id={`${domID(field)}-finding`}>{values.map((finding) => <small className={finding.level} key={`${finding.code}:${finding.message}`}>{finding.message}</small>)}</span>;
  }

  function markDraftChanged(updater: (current: ToolDraftForm) => ToolDraftForm) {
    if (busy === "save") return;
    const nextForm = updater(form);
    draftVersionRef.current += 1;
    setForm(nextForm);
    if (credential && !toolCredentialBindingMatches(credentialBinding, nextForm.endpoint, nextForm.upstream_auth)) {
      setCredential("");
      setCredentialBinding("");
      setStatus("The entered credential was cleared because its destination or authentication configuration changed.");
    }
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  function acceptCurrentDraftResponse(version: number, label: string) {
    if (draftVersionRef.current === version) return true;
    setStatus(`${label} result discarded because the draft or credential changed while the request was running.`);
    return false;
  }

  function acceptImportResponse(draftVersion: number, inputVersion: number) {
    if (!acceptCurrentDraftResponse(draftVersion, "Import")) return false;
    if (versionedResponseIsCurrent(inputVersion, importInputVersionRef.current)) return true;
    setStatus("Import result discarded because the source or import format changed while the request was running.");
    return false;
  }

  function builderContext(draft: APIToolBuilderDraft) {
    const contextDraft = runtimeLock
      ? lockAPIToolBuilderDraft(draft, runtimeLock, apiToolHTTPPath(draft.endpoint, runtimeHTTPPath))
      : draft;
    return {
      draft: contextDraft,
      credential_will_be_supplied: apiScoped ? false : Boolean(validationCredential.trim()),
      ...(tool ? { base_tool_id: tool.id, base_revision: tool.revision } : {}),
    };
  }

  function openFinding(field?: string) {
    if (!field) return;
    const target = document.getElementById(focusID(field));
    target?.focus();
    target?.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  function setActiveProposal(source: "ai" | "import", value: APIToolBuilderProposal | APIToolBuilderImportCandidate, reviewBase: APIToolBuilderDraft, candidateFallback = reviewBase) {
    const sanitized = sanitizeDraft(value.draft, candidateFallback);
    const proposalPath = apiScoped ? apiToolHTTPPath(sanitized.endpoint, runtimeHTTPPath) : "";
    const proposedDraft = runtimeLock ? lockAPIToolBuilderDraft(sanitized, runtimeLock, proposalPath) : sanitized;
    const lockedReviewBase = runtimeLock ? lockAPIToolBuilderDraft(reviewBase, runtimeLock, runtimeHTTPPath) : reviewBase;
    const changes = reviewChanges(lockedReviewBase, proposedDraft, value.changes ?? [], editing, apiScoped);
    if (source === "ai" && changes.length === 0) {
      // A newer assistant turn without field changes supersedes any older
      // proposal. Leaving the prior diff actionable would make the visible
      // conversation and the review state disagree.
      setProposal(null);
      setProposalDecisions({});
      setProposalStale(false);
      return 0;
    }
    setProposal({
      source,
      summary: value.summary || (source === "ai" && "reply" in value ? value.reply : "") || `${changes.length} proposed field change${changes.length === 1 ? "" : "s"}.`,
      draft: proposedDraft,
      changes,
      findings: value.findings ?? [],
    });
    setProposalDecisions({});
    setProposalStale(false);
    setStatus(`${source === "ai" ? "AI" : "Import"} proposal ready with ${changes.length} change${changes.length === 1 ? "" : "s"}.`);
    requestAnimationFrame(() => proposalHeadingRef.current?.focus());
    return changes.length;
  }

  async function proposeDraft() {
    const userMessage = instruction.trim();
    if (!userMessage || !aiAvailable) return;
    if (instructionProblem) {
      const message = instructionProblem;
      setStatus(message);
      onMessage(message);
      return;
    }
    const history = boundedToolBuilderChatHistory(chatHistory);
    const requestVersion = draftVersionRef.current;
    setBusy("propose");
    setStatus(proposal && !proposalStale ? "Sending a credential-free follow-up about the pending proposal…" : "Sending a credential-free message to the assistant…");
    try {
      const result = await api.proposeToolDraft(product.id, { ...builderContext(followUpDraft), instruction: userMessage, history });
      if (!acceptCurrentDraftResponse(requestVersion, "Assistant")) return;
      const assistantMessage = (result.reply || result.summary || "I reviewed the current draft and have no further details to add.").trim();
      setChatHistory(boundedToolBuilderChatHistory([...history, { role: "user", content: userMessage }, { role: "assistant", content: assistantMessage }]));
      setInstruction("");
      const changeCount = setActiveProposal("ai", result, assistanceDraft, followUpDraft);
      if (changeCount === 0) setStatus("Assistant replied without proposing field changes. The draft remains unchanged.");
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, "Assistant")) return;
      const message = errorMessage(error);
      setStatus(`Proposal failed: ${message}`);
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function importDraft() {
    if (!importSource.trim()) return;
    const requestVersion = draftVersionRef.current;
    const importInputVersion = importInputVersionRef.current;
    const source = importSource.trim();
    const kind = importKind;
    setBusy("import");
    setStatus("Inspecting the import as untrusted input…");
    try {
      // Import is a valid starting point for a brand-new tool, so use the
      // canonical assistance draft while the empty manual form has expected
      // validation errors. Candidates remain proposals and must still pass
      // complete validation before they can be saved.
      const result = await api.importToolDraft(product.id, { ...builderContext(assistanceDraft), source: { kind, value: source } });
      if (!acceptImportResponse(requestVersion, importInputVersion)) return;
      const importFindings = result.findings ?? [];
      const sourceFindings = importFindings.filter((finding) => finding.field === "source");
      const candidates = result.candidates.map((candidate) => ({
        ...candidate,
        findings: [...(candidate.findings ?? []), ...sourceFindings],
      }));
      // The raw source may have contained a credential that the server stripped.
      // Retain only normalized candidates and findings after a successful parse.
      setImportSource("");
      importInputVersionRef.current += 1;
      setImportCandidates(candidates);
      setValidation({ valid: !importFindings.some((finding) => finding.level === "error"), network_call_performed: false, findings: importFindings });
      if (candidates.length === 1) setActiveProposal("import", candidates[0], assistanceDraft);
      else setStatus(`${candidates.length} import candidates are ready for review.`);
    } catch (error) {
      if (!acceptImportResponse(requestVersion, importInputVersion)) return;
      const message = errorMessage(error);
      setStatus(`Import failed: ${message}`);
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function analyseDraft() {
    if (!local.draft || !aiAvailable) return;
    const requestVersion = draftVersionRef.current;
    setBusy("analyse");
    setStatus("Analyzing the credential-free draft…");
    try {
      const result = await api.analyseToolDraft(product.id, builderContext(local.draft));
      if (!acceptCurrentDraftResponse(requestVersion, "Analysis")) return;
      if (result.network_call_performed !== false) throw new Error("Analysis did not confirm that upstream execution was disabled.");
      setAnalysis(result);
      setStatus("Draft analysis complete.");
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, "Analysis")) return;
      const message = errorMessage(error);
      setStatus(`Analysis failed: ${message}`);
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  async function validateDraft() {
    if (!local.draft) {
      setStatus("Resolve the local errors before checking the draft.");
      openFinding(local.findings.find((finding) => finding.level === "error")?.field);
      return null;
    }
    const requestVersion = draftVersionRef.current;
    setBusy("validate");
    setStatus("Checking the draft without calling the upstream endpoint…");
    try {
      const result = await api.validateToolDraft(product.id, builderContext(local.draft));
      if (!acceptCurrentDraftResponse(requestVersion, "Validation")) return null;
      if (result.network_call_performed !== false) throw new Error("Validation did not confirm that upstream execution was disabled.");
      setValidation(result);
      setStatus(result.valid ? "Draft validation passed without an upstream call." : "Draft validation found issues to resolve.");
      if (!result.valid) openFinding(result.findings.find((finding) => finding.level === "error")?.field);
      return result;
    } catch (error) {
      if (!acceptCurrentDraftResponse(requestVersion, "Validation")) return null;
      const message = errorMessage(error);
      setStatus(`Validation failed: ${message}`);
      onMessage(message);
      return null;
    } finally {
      setBusy(null);
    }
  }

  async function saveDraft(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editable || !local.draft || busy) {
      openFinding(local.findings.find((finding) => finding.level === "error")?.field);
      return;
    }
    setBusy("save");
    setStatus("Validating before saving…");
    try {
      const checked = await api.validateToolDraft(product.id, builderContext(local.draft));
      if (checked.network_call_performed !== false) throw new Error("Validation did not confirm that upstream execution was disabled.");
      setValidation(checked);
      if (!checked.valid || checked.findings.some((finding) => finding.level === "error")) {
        setStatus("Resolve the validation errors before saving.");
        openFinding(checked.findings.find((finding) => finding.level === "error")?.field);
        return;
      }
      const commonPersistence = {
        endpoint: local.draft.endpoint,
        upstream_auth: local.draft.upstream_auth,
        ...(CREDENTIAL_AUTH_TYPES.has(local.draft.upstream_auth.type) && enteredCredentialReusable && credential.trim() ? { credential: credential } : {}),
      };
      const runtimeContext = runtimeLock ? apiToolPersistenceContext(runtimeLock, runtimeHTTPPath) : null;
      const runtimePersistence = runtimeContext ? {
        runtime_service_connection_id: runtimeContext.runtime_service_connection_id,
        http_path: runtimeContext.http_path,
      } : null;
      const persisted = {
        description: local.draft.description,
        http_method: local.draft.http_method,
        timeout_ms: local.draft.timeout_ms,
        input_schema: local.draft.input_schema,
        output_schema: local.draft.output_schema,
        request_mapping: local.draft.request_mapping,
        response_mapping: local.draft.response_mapping,
        authorization_policy: local.draft.authorization_policy,
        request_example: local.draft.request_example ?? null,
        response_example: local.draft.response_example ?? null,
        ...(runtimePersistence ?? commonPersistence),
      };
      const saved = tool
        ? await api.updateTool(product.id, tool.id, { ...persisted, revision: tool.revision })
        : await api.createTool(product.id, { ...persisted, ...(runtimeContext ?? { scope: "common" as const }), organisation_id: product.organisation_id, namespace: local.draft.namespace, name: local.draft.name });
      setCredentialBinding("");
      setCredential("");
      setStatus("Draft saved.");
      onMessage(`${saved.namespace}.${saved.name} saved as a draft.`);
      await onSaved?.(saved);
      onDirtyChange?.(false);
      onNavigate(`/tool/${encodeURIComponent(saved.id)}`);
    } catch (error) {
      const message = errorMessage(error);
      setStatus(`Save failed: ${message}`);
      onMessage(message);
    } finally {
      setBusy(null);
    }
  }

  function applyProposalField(change: ReviewChange) {
    if (!proposal || proposalStale) return;
    if (busy === "save") return;
    const proposedForm = draftToForm(proposal.draft);
    draftVersionRef.current += 1;
    const nextForm: ToolDraftForm = (() => {
      switch (change.field) {
        case "endpoint":
          if (apiScoped) return form;
          return { ...form, endpoint: proposedForm.endpoint };
        case "input_schema": return { ...form, input_schema_text: proposedForm.input_schema_text };
        case "output_schema": return { ...form, output_schema_text: proposedForm.output_schema_text };
        case "request_example": return { ...form, request_example_text: proposedForm.request_example_text };
        case "response_example": return { ...form, response_example_text: proposedForm.response_example_text };
        default: return { ...form, [change.field]: proposedForm[change.field] };
      }
    })();
    if (apiScoped && change.field === "endpoint") setRuntimeHTTPPath(apiToolHTTPPath(proposal.draft.endpoint, runtimeHTTPPath));
    const credentialCleared = Boolean(credential && !toolCredentialBindingMatches(credentialBinding, nextForm.endpoint, nextForm.upstream_auth));
    setForm(nextForm);
    if (credentialCleared) {
      setCredential("");
      setCredentialBinding("");
    }
    setValidation(null);
    setAnalysis(null);
    setProposalDecisions((current) => ({ ...current, [change.field]: "accepted" }));
    setStatus(`${change.label} accepted. The draft has not been saved.${credentialCleared ? " The entered credential was cleared because its destination or authentication configuration changed." : ""}`);
  }

  function rejectProposalField(change: ReviewChange) {
    if (busy === "save") return;
    setProposalDecisions((current) => ({ ...current, [change.field]: "rejected" }));
    setStatus(`${change.label} kept unchanged.`);
  }

  function updateAuth(next: Partial<APIToolUpstreamAuth>) {
    markDraftChanged((current) => ({ ...current, upstream_auth: { ...current.upstream_auth, ...next } }));
  }

  function updatePolicy(next: Partial<APIToolAuthorizationPolicy>) {
    markDraftChanged((current) => {
      const policy = { ...current.authorization_policy, ...next };
      if (policy.risk === "critical") policy.confirmation_required = true;
      return { ...current, authorization_policy: policy };
    });
  }

  function toggleGrant(key: string) {
    const selected = form.authorization_policy.required_grants.includes(key);
    updatePolicy({ required_grants: selected ? form.authorization_policy.required_grants.filter((item) => item !== key) : [...form.authorization_policy.required_grants, key] });
  }

  function addRequestMapping() {
    let index = Object.keys(form.request_mapping.parameter_locations).length + 1;
    let name = `parameter_${index}`;
    while (name in form.request_mapping.parameter_locations) name = `parameter_${++index}`;
    markDraftChanged((current) => ({ ...current, request_mapping: { parameter_locations: { ...current.request_mapping.parameter_locations, [name]: "body" } } }));
  }

  function renameRequestMapping(previous: string, next: string) {
    markDraftChanged((current) => {
      const values = { ...current.request_mapping.parameter_locations };
      const location = values[previous];
      delete values[previous];
      values[next] = location;
      return { ...current, request_mapping: { parameter_locations: values } };
    });
  }

  function setRequestMappingLocation(parameter: string, location: APIToolRequestMapping["parameter_locations"][string]) {
    markDraftChanged((current) => ({ ...current, request_mapping: { parameter_locations: { ...current.request_mapping.parameter_locations, [parameter]: location } } }));
  }

  function removeRequestMapping(parameter: string) {
    markDraftChanged((current) => {
      const values = { ...current.request_mapping.parameter_locations };
      delete values[parameter];
      return { ...current, request_mapping: { parameter_locations: values } };
    });
  }

  function changeRuntimeConnection(nextConnectionID: string) {
    if (editing || busy === "save") return;
    draftVersionRef.current += 1;
    setRuntimeConnectionID(nextConnectionID);
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  function changeRuntimeHTTPPath(nextPath: string) {
    if (busy === "save") return;
    draftVersionRef.current += 1;
    setRuntimeHTTPPath(nextPath);
    setValidation(null);
    setAnalysis(null);
    if (proposal) setProposalStale(true);
  }

  const activeAuth = AUTH_TYPES.find((candidate) => candidate.value === contextualForm.upstream_auth.type)!;
  const credentialLabel = form.upstream_auth.type === "basic" ? "Password" : form.upstream_auth.type === "oauth_client_credentials" ? "Client secret" : form.upstream_auth.type === "bearer" ? "Bearer token" : "Secret value";
  const saveDisabled = Boolean(busy) || !editable || errors.length > 0;
  const formLocked = !editable || busy === "save";

  return <form className="tool-builder" aria-busy={Boolean(busy)} onSubmit={saveDraft} noValidate>
    <div className="tool-builder-breadcrumb"><BuilderLink path={apiContext ? integrationPath(apiContext.integration.id, "tools") : "/tools"} onNavigate={onNavigate} className="entity-back-link"><ArrowLeft />{apiContext ? `Back to ${apiContext.integration.display_name} tools` : "Back to tools"}</BuilderLink><code>{apiContext ? `/integration/${apiContext.integration.id}/tools/${editing ? tool?.id : "new"}` : editing ? `/tool/${tool?.id}` : "/tools/new"}</code></div>
    <PageHeader
      eyebrow={apiContext ? `${apiContext.integration.display_name} API tool` : editing ? "Edit common tool" : "New common tool"}
      title={editing ? `${tool?.namespace}.${tool?.name}` : apiContext ? "Build an API tool" : "Build a custom tool"}
      description={apiContext ? "Turn one operation from this API into an agent-facing capability. The API service connection supplies the destination and authentication; AI and imports can only propose the relative operation contract." : "Define one reusable, agent-facing HTTP capability. AI and imports can propose fields; only you can accept and save them."}
      action={<span className="heading-actions tool-builder-heading-actions"><Badge color={editable ? "amber" : "zinc"}>{tool?.state ?? "unsaved"}</Badge><Button color="indigo" type="submit" disabled={saveDisabled}>{busy === "save" ? "Saving…" : "Save draft"}</Button></span>}
    />

    {!editable && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>This revision is not editable.</strong><small>Published and retired tools are immutable. Create a new draft or clone the tool before changing its contract.</small></span></div>}

    {apiContext && <div className="notice"><ShieldCheck /><span><strong>API-owned execution boundary.</strong> This tool stays with {apiContext.integration.display_name}. Endpoint hosts, authentication, and credentials are inherited from Access and cannot be supplied or changed here.</span></div>}

    {tool?.endpoint_requires_review && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>The stored endpoint contained unsafe URL metadata.</strong><small>Its query, user information, or fragment was redacted. Enter a fixed credential-free endpoint, rotate any exposed upstream credential, and save this draft before publishing.</small></span></div>}

    <div className="tool-builder-mode-row">
      <span><strong>Start from</strong><small>Every mode edits the same unsaved draft.</small></span>
      <SegmentedControl label="Tool builder mode" items={[{ id: "ai", label: "AI assist" }, { id: "import", label: "Import" }, { id: "manual", label: "Manual" }]} value={mode} onChange={setMode} />
    </div>

    <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">{status}</p>

    <div className="tool-builder-layout">
      <div className="tool-builder-main">
        {mode === "ai" && <section className="panel tool-builder-source-panel" aria-labelledby={`${generatedID}-ai-title`}>
          <PanelHeader title={<span id={`${generatedID}-ai-title`}><Sparkles />AI-assisted draft</span>} description="Ask questions and refine the capability over several turns. Any suggested field modifications remain a separate reviewable proposal." action={<Badge color="violet">Proposal only</Badge>} />
          {aiAvailable ? <div className="tool-builder-chat-shell">
            <ol className="tool-builder-chat-transcript" role="log" aria-label="Conversation with the tool builder assistant" aria-live="polite" aria-relevant="additions">
              {chatHistory.map((message, index) => <li className={`tool-builder-chat-message ${message.role}`} key={`${index}:${message.role}:${message.content}`}><span className="tool-builder-chat-avatar" aria-hidden="true">{message.role === "assistant" ? <Bot /> : <Sparkles />}</span><span><strong>{message.role === "assistant" ? "Assistant" : "You"}</strong><p>{message.content}</p></span></li>)}
            </ol>
            {chatHistory.length === 0 && <div className="tool-builder-chat-empty"><Bot /><span><strong>Start with an outcome or a question</strong><small>The assistant can ask for missing details or propose a credential-free contract.</small></span></div>}
            <div className="tool-builder-chat-composer">
              <label className="auth-field" htmlFor={`${generatedID}-ai-instruction`}><span>Message the assistant</span><textarea id={`${generatedID}-ai-instruction`} value={instruction} maxLength={TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} aria-invalid={Boolean(instructionProblem)} aria-describedby={[`${generatedID}-ai-instruction-help`, instructionProblem ? `${generatedID}-ai-instruction-error` : ""].filter(Boolean).join(" ")} onChange={(event) => setInstruction(event.target.value)} placeholder="For example: Help me design a read-only readiness check. Which details do you still need?" /><small id={`${generatedID}-ai-instruction-help`}>{instructionBytes}/{TOOL_BUILDER_CHAT_LIMITS.maxMessageBytes} UTF-8 bytes. The last six exchanges are sent as non-secret context and are not saved with the tool. A follow-up refines the complete pending candidate; fields you kept unchanged remain at their current values.</small>{instructionProblem && <small className="error" id={`${generatedID}-ai-instruction-error`} role="alert">{instructionProblem}</small>}</label>
              <div className="tool-builder-source-actions"><Button type="button" color="indigo" disabled={Boolean(busy) || !instruction.trim() || Boolean(instructionProblem)} onClick={proposeDraft}><Sparkles data-slot="icon" />{busy === "propose" ? "Responding…" : "Send message"}</Button><Button type="button" outline disabled={Boolean(busy) || !local.draft} onClick={analyseDraft}><Bot data-slot="icon" />{busy === "analyse" ? "Analyzing…" : "Analyze current draft"}</Button></div>
              <p className="tool-builder-chat-boundary"><ShieldCheck />{apiScoped ? "Your message and a non-secret preview composed from the selected API service connection, relative path, schemas, policy, and examples are sent to the configured Analysis provider. The selected connection, host, authentication, and credential boundary remain locked by this form." : "Your message and current non-secret draft—including the endpoint, schemas, authentication metadata, and any examples—are sent to the configured Analysis provider. The separate credential field is excluded."} Use synthetic examples and keep secrets out of every field. Automated checks reject common credential patterns but cannot prove arbitrary text is secret-free. AI cannot accept its own diffs, save, publish, bind, or call the endpoint.</p>
            </div>
          </div> : <div className="tool-builder-source-empty"><Bot /><span><strong>AI assistance is unavailable</strong><small>Configure an analysis provider, or continue in Manual or Import mode.</small></span><BuilderLink path="/settings/ai" onNavigate={onNavigate} className="core-button core-button-outline">Configure AI</BuilderLink></div>}
          {analysis && <div className="tool-builder-analysis"><span><Bot /></span><span><strong>Analysis</strong><p>{analysis.reply || analysis.summary || "Analysis completed."}</p></span></div>}
        </section>}

        {mode === "import" && <section className="panel tool-builder-source-panel" aria-labelledby={`${generatedID}-import-title`}>
          <PanelHeader title={<span id={`${generatedID}-import-title`}><TerminalSquare />Import a request</span>} description="Inspect a pasted OpenAPI document, Postman collection, or cURL command as untrusted input, then review the proposed fields." action={<Badge color="amber">Untrusted</Badge>} />
          <div className="tool-builder-source-body">
            <div className="two-fields"><label className="auth-field" htmlFor={`${generatedID}-import-kind`}><span>Import format</span><select id={`${generatedID}-import-kind`} value={importKind} disabled={busy === "import"} onChange={(event) => { importInputVersionRef.current += 1; setImportKind(event.target.value as APIToolBuilderImportKind); setImportCandidates([]); if (proposal?.source === "import") setProposalStale(true); }}>{IMPORT_KINDS.map((kind) => <option key={kind.value} value={kind.value}>{kind.label}</option>)}</select></label><span className="tool-builder-import-hint"><ShieldCheck /><small>Paste reviewable text only. URL fetching and automatic publishing are disabled. Embedded credentials are detected and stripped; they never populate {apiScoped ? "this API tool" : "the separate secret field"}.</small></span></div>
            <label className="auth-field" htmlFor={`${generatedID}-import-source`}><span>{importKind === "curl" ? "cURL command" : importKind === "postman" ? "Postman collection JSON" : "OpenAPI JSON or YAML"}</span><textarea id={`${generatedID}-import-source`} className="code-input" value={importSource} disabled={busy === "import"} onChange={(event) => { importInputVersionRef.current += 1; setImportSource(event.target.value); setImportCandidates([]); if (proposal?.source === "import") setProposalStale(true); }} spellCheck={false} placeholder={importKind === "curl" ? "curl -X POST https://api.vendor.example/v1/projects" : importKind === "postman" ? "Paste an exported Postman Collection v2.1 document" : "openapi: 3.1.0"} /></label>
            <div className="tool-builder-source-actions"><Button type="button" color="indigo" disabled={Boolean(busy) || !importSource.trim()} onClick={importDraft}><TerminalSquare data-slot="icon" />{busy === "import" ? "Inspecting…" : "Review import"}</Button></div>
          </div>
          {importCandidates.length > 1 && <div className="tool-builder-candidates" aria-label="Import candidates"><h3>Choose an operation to review</h3>{importCandidates.map((candidate, index) => <button type="button" key={`${candidate.draft.namespace}.${candidate.draft.name}:${index}`} onClick={() => setActiveProposal("import", candidate, assistanceDraft)}><span><strong>{candidate.draft.namespace}.{candidate.draft.name}</strong><small>{candidate.summary || candidate.draft.description}</small></span><Badge color={candidate.valid === false ? "red" : "blue"}>{candidate.valid === false ? "needs work" : "review"}</Badge></button>)}</div>}
        </section>}

        {mode === "manual" && <section className="panel tool-builder-source-panel tool-builder-manual-intro" aria-labelledby={`${generatedID}-manual-title`}><span className="settings-icon"><Wrench /></span><span><h2 id={`${generatedID}-manual-title`}>Manual setup</h2><p>Complete the shared contract below. Changes remain local until validation passes and you choose Save draft.</p></span><Badge color="blue">You control every field</Badge></section>}

        {proposal && <section className={`panel tool-builder-proposal ${proposalStale ? "stale" : ""}`} aria-labelledby={`${generatedID}-proposal-title`}>
          <div className="tool-builder-proposal-heading"><span><small>{proposal.source === "live-test" ? "Live-test AI proposal" : proposal.source === "ai" ? "AI proposal" : "Import proposal"}</small><h2 id={`${generatedID}-proposal-title`} ref={proposalHeadingRef} tabIndex={-1}>Review before applying</h2><p>{proposal.summary}</p></span><Badge color={proposalStale ? "red" : "violet"}>{proposalStale ? "stale" : `${proposal.changes.length} changes`}</Badge></div>
          {proposalStale && <div className="capability-unavailable" role="alert"><TriangleAlert /><span><strong>The draft changed after this proposal.</strong><small>Dismiss it and generate or import a fresh proposal to avoid overwriting newer work.</small></span></div>}
          <div className="tool-builder-change-list">{proposal.changes.map((change) => {
            const decision = proposalDecisions[change.field];
            return <article className={`tool-builder-change ${decision ?? "pending"}`} key={change.field}>
              <header><span><strong>{change.label}</strong>{change.securitySensitive && <Badge color="amber"><ShieldCheck />Security-sensitive</Badge>}</span>{decision && <Badge color={decision === "accepted" ? "green" : "zinc"}>{decision}</Badge>}</header>
              {change.rationale && <p>{change.rationale}</p>}
              <div className="tool-builder-diff"><div><small>Before</small><pre>{formatReviewValue(change.before)}</pre></div><div><small>Proposed</small><pre>{formatReviewValue(change.after)}</pre></div></div>
              <div className="tool-builder-change-actions"><Button type="button" outline disabled={proposalStale || Boolean(decision) || busy === "save"} onClick={() => rejectProposalField(change)}><XCircle data-slot="icon" />Keep current</Button><Button type="button" color="indigo" disabled={proposalStale || Boolean(decision) || busy === "save"} onClick={() => applyProposalField(change)}><Check data-slot="icon" />Accept change</Button></div>
            </article>;
          })}{proposal.changes.length === 0 && <div className="tool-builder-ready"><CheckCircle2 /><span><strong>No field changes</strong><small>The proposal matches the current draft.</small></span></div>}</div>
          {proposal.findings.length > 0 && <div className="tool-builder-proposal-findings"><h3>Proposal findings</h3><FindingList findings={proposal.findings} onOpen={openFinding} /></div>}
          <div className="tool-builder-proposal-footer"><small>Accepted values update only this unsaved form. Credentials are never part of the proposal.</small><Button type="button" outline onClick={() => { setProposal(null); setProposalDecisions({}); setProposalStale(false); setStatus("Proposal dismissed. Any accepted changes remain in the unsaved draft."); }}>Dismiss proposal</Button></div>
        </section>}

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>Identity and purpose</legend><p>Choose a stable, agent-facing identity. Identity cannot change after the first save.</p>
          <div className="tool-builder-fields"><div className="two-fields"><label className="auth-field" htmlFor={domID("namespace")}><span>Namespace</span><input id={domID("namespace")} data-tool-field="namespace" maxLength={64} readOnly={editing} value={form.namespace} aria-invalid={fieldFindings("namespace").some((finding) => finding.level === "error")} aria-describedby={describedBy("namespace", `${domID("namespace")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, namespace: event.target.value.toLowerCase() }))} placeholder="platform" /><small id={`${domID("namespace")}-help`}>Lower-case contract group, such as <code>billing</code>.</small>{renderFieldFindings("namespace")}</label><label className="auth-field" htmlFor={domID("name")}><span>Tool name</span><input id={domID("name")} data-tool-field="name" maxLength={64} readOnly={editing} value={form.name} aria-invalid={fieldFindings("name").some((finding) => finding.level === "error")} aria-describedby={describedBy("name", `${domID("name")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, name: event.target.value.toLowerCase() }))} placeholder="check_readiness" /><small id={`${domID("name")}-help`}>Lower-case action name, starting with a letter.</small>{renderFieldFindings("name")}</label></div>
            <label className="auth-field" htmlFor={domID("description")}><span>Purpose</span><textarea id={domID("description")} data-tool-field="description" maxLength={500} value={form.description} aria-invalid={fieldFindings("description").some((finding) => finding.level === "error")} aria-describedby={describedBy("description", `${domID("description")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, description: event.target.value }))} placeholder="Describe one action, when an agent should use it, and the result it returns." /><small id={`${domID("description")}-help`}>{form.description.length}/500 characters. This description is visible to agents.</small>{renderFieldFindings("description")}</label></div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>Contract</legend><p>Define the JSON accepted and returned by the tool. Schema validation never calls the upstream API.</p>
          <div className="tool-builder-fields tool-builder-schema-grid"><label className="auth-field" htmlFor={domID("input_schema")}><span>Input JSON Schema</span><textarea id={domID("input_schema")} data-tool-field="input_schema" className="code-input" spellCheck={false} value={form.input_schema_text} aria-invalid={fieldFindings("input_schema").some((finding) => finding.level === "error")} aria-describedby={describedBy("input_schema")} onChange={(event) => markDraftChanged((current) => ({ ...current, input_schema_text: event.target.value }))} />{renderFieldFindings("input_schema")}</label><label className="auth-field" htmlFor={domID("output_schema")}><span>Output JSON Schema</span><textarea id={domID("output_schema")} data-tool-field="output_schema" className="code-input" spellCheck={false} value={form.output_schema_text} aria-invalid={fieldFindings("output_schema").some((finding) => finding.level === "error")} aria-describedby={describedBy("output_schema")} onChange={(event) => markDraftChanged((current) => ({ ...current, output_schema_text: event.target.value }))} />{renderFieldFindings("output_schema")}</label></div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>{apiScoped ? "API operation" : "Execution and authentication"}</legend><p>{apiScoped ? "Choose the saved API service connection and the relative operation path. Authentication and credentials remain managed once in Access." : "Fix the destination, request shape, and upstream credential strategy. Agents cannot alter these controls at runtime."}</p>
          <div className="tool-builder-fields"><div className="tool-builder-execution-grid"><label className="auth-field" htmlFor={domID("http_method")}><span>Method</span><select id={domID("http_method")} data-tool-field="http_method" value={form.http_method} onChange={(event) => {
              const method = event.target.value as APIToolHTTPMethod;
              markDraftChanged((current) => ({ ...current, http_method: method, authorization_policy: { ...current.authorization_policy, risk: method === "GET" ? "low" : method === "DELETE" ? "critical" : current.authorization_policy.risk === "low" || current.authorization_policy.risk === "critical" ? "medium" : current.authorization_policy.risk, confirmation_required: method === "DELETE" ? true : current.authorization_policy.confirmation_required } }));
            }}>{HTTP_METHODS.map((method) => <option key={method}>{method}</option>)}</select></label>{apiScoped ? <><label className="auth-field" htmlFor={domID("runtime_service_connection_id")}><span>Service connection</span><select id={domID("runtime_service_connection_id")} data-tool-field="runtime_service_connection_id" value={runtimeConnectionID} disabled={editing || runtimeConnections.length === 0} aria-invalid={!runtimeLock} aria-describedby={`${domID("runtime_service_connection_id")}-help`} onChange={(event) => changeRuntimeConnection(event.target.value)}><option value="">Choose a configured connection</option>{runtimeConnections.map((connection) => <option key={connection.id} value={connection.id}>{connection.name}</option>)}</select><small id={`${domID("runtime_service_connection_id")}-help`}>{editing ? "The service connection is immutable after the first save; clone to choose another." : runtimeConnections.length > 0 ? "Saved endpoint and authentication revisions are selected by environment at runtime." : "Configure this API's endpoint and authentication in Access first."}</small>{renderFieldFindings("runtime_service_connection_id")}</label><label className="auth-field" htmlFor={domID("endpoint")}><span>Relative path</span><input id={domID("endpoint")} data-tool-field="endpoint" value={runtimeHTTPPath} aria-invalid={Boolean(runtimePathProblem) || fieldFindings("endpoint").some((finding) => finding.level === "error")} aria-describedby={describedBy("endpoint", `${domID("endpoint")}-help`)} onChange={(event) => changeRuntimeHTTPPath(event.target.value)} placeholder="/v1/voices/{voice_id}" /><small id={`${domID("endpoint")}-help`}>Starts with <code>/</code>. The service host and authentication come from Access.</small>{renderFieldFindings("endpoint")}</label></> : <label className="auth-field" htmlFor={domID("endpoint")}><span>Fixed endpoint</span><input id={domID("endpoint")} data-tool-field="endpoint" type="url" value={form.endpoint} aria-invalid={fieldFindings("endpoint").some((finding) => finding.level === "error")} aria-describedby={describedBy("endpoint", `${domID("endpoint")}-help`)} onChange={(event) => markDraftChanged((current) => ({ ...current, endpoint: event.target.value }))} placeholder="https://api.vendor.example/v1/readiness" /><small id={`${domID("endpoint")}-help`}>HTTPS is required except for localhost development.</small>{renderFieldFindings("endpoint")}</label>}<label className="auth-field" htmlFor={domID("timeout_ms")}><span>Timeout (ms)</span><input id={domID("timeout_ms")} data-tool-field="timeout_ms" type="number" min={100} max={60000} step={100} value={form.timeout_ms} aria-invalid={fieldFindings("timeout_ms").some((finding) => finding.level === "error")} aria-describedby={describedBy("timeout_ms")} onChange={(event) => markDraftChanged((current) => ({ ...current, timeout_ms: Number(event.target.value) }))} />{renderFieldFindings("timeout_ms")}</label></div>

            {apiScoped ? runtimeLock ? <div className="runtime-current-summary"><span className="settings-icon"><ShieldCheck /></span><span><strong>{runtimeConnection?.name ?? "API service connection"}</strong><small>{runtimeLock.baseURL}</small></span><span><small>Authentication</small><strong>{activeAuth.label}</strong></span><span><small>Credential</small><strong>{runtimeLock.credentialPresent ? "Managed in Access" : "Not required"}</strong></span></div> : <div className="capability-unavailable"><TriangleAlert /><span><strong>Configure service access first</strong><small>An API tool cannot store its own endpoint or secret.</small></span><BuilderLink path={integrationPath(apiContext!.integration.id, "access")} onNavigate={onNavigate} className="entity-back-link">Open Access</BuilderLink></div> : <>
            <div className="tool-builder-subsection"><div><h3>Upstream authentication</h3><p>{activeAuth.description}</p></div><label className="auth-field" htmlFor={domID("upstream_auth")}><span>Authentication type</span><select id={domID("upstream_auth")} data-tool-field="upstream_auth" value={form.upstream_auth.type} onChange={(event) => markDraftChanged((current) => ({ ...current, upstream_auth: { type: event.target.value as APIToolUpstreamAuthType } }))}>{AUTH_TYPES.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}</select>{renderFieldFindings("upstream_auth")}</label></div>

            {form.upstream_auth.type === "authorization_scheme" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-scheme`}><span>Authorization scheme</span><input id={`${domID("upstream_auth")}-scheme`} data-tool-field="upstream_auth.scheme" value={form.upstream_auth.scheme ?? ""} onChange={(event) => updateAuth({ scheme: event.target.value })} placeholder="SSWS" /><small>The fixed request header becomes <code>Authorization: {form.upstream_auth.scheme?.trim() || "Scheme"} &lt;encrypted credential&gt;</code>.</small>{renderFieldFindings("upstream_auth.scheme")}</label>}
            {["api_key_header", "custom_header"].includes(form.upstream_auth.type) && <div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-header`}><span>Header name</span><input id={`${domID("upstream_auth")}-header`} value={form.upstream_auth.header_name ?? ""} onChange={(event) => updateAuth({ header_name: event.target.value })} placeholder={form.upstream_auth.type === "api_key_header" ? "X-API-Key" : "X-Vendor-Token"} /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-prefix`}><span>Value prefix (optional)</span><input id={`${domID("upstream_auth")}-prefix`} value={form.upstream_auth.prefix ?? ""} onChange={(event) => updateAuth({ prefix: event.target.value })} placeholder="Token" /><small>DokoSoko inserts one space between a non-empty prefix and the encrypted value.</small></label></div>}
            {form.upstream_auth.type === "api_key_query" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-query`}><span>Query parameter name</span><input id={`${domID("upstream_auth")}-query`} value={form.upstream_auth.query_name ?? ""} onChange={(event) => updateAuth({ query_name: event.target.value })} placeholder="api_key" /></label>}
            {form.upstream_auth.type === "basic" && <label className="auth-field" htmlFor={`${domID("upstream_auth")}-username`}><span>Username</span><input id={`${domID("upstream_auth")}-username`} value={form.upstream_auth.username ?? ""} onChange={(event) => updateAuth({ username: event.target.value })} autoComplete="off" /></label>}
            {form.upstream_auth.type === "oauth_client_credentials" && <><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-client`}><span>Client ID</span><input id={`${domID("upstream_auth")}-client`} value={form.upstream_auth.client_id ?? ""} onChange={(event) => updateAuth({ client_id: event.target.value })} autoComplete="off" /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-token-url`}><span>Token URL</span><input id={`${domID("upstream_auth")}-token-url`} type="url" value={form.upstream_auth.token_url ?? ""} onChange={(event) => updateAuth({ token_url: event.target.value })} placeholder="https://identity.vendor.example/oauth/token" /></label></div><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-auth-method`}><span>Token endpoint authentication</span><select id={`${domID("upstream_auth")}-auth-method`} value={form.upstream_auth.token_endpoint_auth_method ?? "client_secret_basic"} onChange={(event) => updateAuth({ token_endpoint_auth_method: event.target.value as "client_secret_basic" | "client_secret_post" })}><option value="client_secret_basic">Client secret Basic</option><option value="client_secret_post">Client secret POST</option></select></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-resource`}><span>Resource (optional)</span><input id={`${domID("upstream_auth")}-resource`} value={form.upstream_auth.resource ?? ""} onChange={(event) => updateAuth({ resource: event.target.value })} placeholder="https://api.vendor.example" /></label></div><div className="two-fields"><label className="auth-field" htmlFor={`${domID("upstream_auth")}-scopes`}><span>Scopes</span><input id={`${domID("upstream_auth")}-scopes`} value={(form.upstream_auth.scopes ?? []).join(" ")} onChange={(event) => updateAuth({ scopes: event.target.value.split(/[\s,]+/).filter(Boolean) })} placeholder="projects.read projects.write" /></label><label className="auth-field" htmlFor={`${domID("upstream_auth")}-audience`}><span>Audience (optional, provider-specific)</span><input id={`${domID("upstream_auth")}-audience`} value={form.upstream_auth.audience ?? ""} onChange={(event) => updateAuth({ audience: event.target.value })} /></label></div></>}

            {CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) && <div className="tool-builder-credential"><label className="auth-field" htmlFor={domID("credential")}><span>{credentialLabel}</span><input id={domID("credential")} data-tool-field="credential" type="password" autoComplete="new-password" value={credential} aria-invalid={fieldFindings("credential").some((finding) => finding.level === "error")} aria-describedby={describedBy("credential", `${domID("credential")}-help`)} onChange={(event) => { if (busy === "save") return; const nextCredential = event.target.value; draftVersionRef.current += 1; setCredentialBinding(nextCredential ? toolCredentialBinding(form.endpoint, form.upstream_auth) : ""); setCredential(nextCredential); setValidation(null); setAnalysis(null); if (proposal) setProposalStale(true); }} placeholder={form.credential_present && storedCredentialReusable ? "Leave blank to keep the stored credential" : "Required before saving"} /><small id={`${domID("credential")}-help`}>{form.credential_present && !storedCredentialReusable ? "Re-enter the credential because its destination or authentication configuration changed." : form.credential_present ? "A credential is already stored. Enter a value only to replace it." : "Encrypted only on final save."}</small>{renderFieldFindings("credential")}</label><div className="tool-builder-secret-boundary"><KeyRound /><span><strong>Local secret boundary</strong><small>This value is excluded from AI, import, analysis, and validation payloads.</small></span></div></div>}</>}

            <div className="tool-builder-subsection"><div><h3>Request mapping</h3><p>Map input properties to fixed request locations. With no explicit mapping, {form.http_method === "GET" ? "GET input is sent as query parameters" : "input is sent as one JSON request body"}.</p></div><Button type="button" outline onClick={addRequestMapping}><Plus data-slot="icon" />Add mapping</Button></div>
            <div id={domID("request_mapping")} data-tool-field="request_mapping" tabIndex={-1} className="tool-builder-mapping-list">{Object.entries(form.request_mapping.parameter_locations).map(([parameter, location], index) => <div className="tool-builder-mapping-row" key={`${parameter}:${index}`}><label className="auth-field"><span className="sr-only">Parameter name</span><input value={parameter} aria-label={`Mapping ${index + 1} parameter name`} onChange={(event) => renameRequestMapping(parameter, event.target.value)} /></label><label className="auth-field"><span className="sr-only">Request location</span><select aria-label={`Mapping ${parameter || index + 1} request location`} value={location} onChange={(event) => setRequestMappingLocation(parameter, event.target.value as APIToolRequestMapping["parameter_locations"][string])}><option value="path">Path</option><option value="query">Query</option><option value="header">Header</option><option value="body">Body</option></select></label><Button type="button" outline aria-label={`Remove mapping for ${parameter || `row ${index + 1}`}`} onClick={() => removeRequestMapping(parameter)}><XCircle data-slot="icon" />Remove</Button></div>)}{Object.keys(form.request_mapping.parameter_locations).length === 0 && <p className="tool-builder-empty-copy">No explicit mappings. {form.http_method === "GET" ? "Each input property becomes a query parameter." : "The input object becomes the JSON request body."}</p>}{renderFieldFindings("request_mapping")}</div>
            <label className="auth-field" htmlFor={domID("response_mapping")}><span>Response result path (optional)</span><input id={domID("response_mapping")} data-tool-field="response_mapping" value={form.response_mapping.result_path ?? ""} onChange={(event) => markDraftChanged((current) => ({ ...current, response_mapping: event.target.value ? { result_path: event.target.value } : {} }))} placeholder="data.result" /><small>Dot-separated path to the value returned to the agent. Leave blank to return the full response body.</small></label>
          </div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>Authorization policy</legend><p>Require registered deployment grants and explicit human safeguards before execution.</p>
          <div className="tool-builder-fields"><div id={domID("authorization_policy")} data-tool-field="authorization_policy" tabIndex={-1} className="tool-builder-grants" role="group" aria-labelledby={`${domID("authorization_policy")}-label`}><span id={`${domID("authorization_policy")}-label`}><strong>Required grants</strong><small>Select every grant a caller must hold.</small></span>{grants.length > 0 ? <div>{grants.map((grant) => <label className="compact-check" key={grant.id}><input type="checkbox" aria-label={`Require ` + grant.display_name + ` grant`} checked={form.authorization_policy.required_grants.includes(grant.key)} onChange={() => toggleGrant(grant.key)} /><span><strong>{grant.display_name}</strong><small><code>{grant.key}</code> · {grant.state}</small></span></label>)}</div> : <p className="tool-builder-empty-copy">No grants are registered. This tool will have no baseline grant requirement.</p>}{renderFieldFindings("authorization_policy.required_grants")}</div>
            <div className="two-fields"><label className="auth-field" htmlFor={`${domID("authorization_policy")}-risk`}><span>Risk</span><select id={`${domID("authorization_policy")}-risk`} value={form.authorization_policy.risk} onChange={(event) => updatePolicy({ risk: event.target.value as APIToolRisk })}>{RISKS.map((risk) => <option value={risk} key={risk}>{risk[0].toUpperCase() + risk.slice(1)}</option>)}</select></label><label className="compact-check"><input type="checkbox" disabled={form.authorization_policy.risk === "critical"} checked={form.authorization_policy.confirmation_required || form.authorization_policy.risk === "critical"} onChange={(event) => updatePolicy({ confirmation_required: event.target.checked })} /><span>Require explicit confirmation</span></label></div>
            <label className="compact-check" id={`${domID("authorization_policy")}-idempotency`}><input type="checkbox" checked={form.authorization_policy.idempotency_required} onChange={(event) => updatePolicy({ idempotency_required: event.target.checked })} /><span>Require idempotency metadata for mutation calls</span></label>{renderFieldFindings("authorization_policy.idempotency_required")}</div>
        </fieldset>

        <fieldset className="panel tool-builder-fieldset" disabled={formLocked}>
          <legend>Examples</legend><p>Optional examples make review easier and are checked without calling the upstream API. Use synthetic, non-sensitive values: AI assistance sends current examples to the configured Analysis provider.</p>
          <div className="tool-builder-fields tool-builder-schema-grid"><label className="auth-field" htmlFor={domID("request_example")}><span>Request example</span><textarea id={domID("request_example")} data-tool-field="request_example" className="code-input" spellCheck={false} value={form.request_example_text} aria-invalid={fieldFindings("request_example").some((finding) => finding.level === "error")} aria-describedby={describedBy("request_example")} onChange={(event) => markDraftChanged((current) => ({ ...current, request_example_text: event.target.value }))} placeholder={'{\n  "project_id": "project_123"\n}'} />{renderFieldFindings("request_example")}</label><label className="auth-field" htmlFor={domID("response_example")}><span>Response example</span><textarea id={domID("response_example")} data-tool-field="response_example" className="code-input" spellCheck={false} value={form.response_example_text} aria-invalid={fieldFindings("response_example").some((finding) => finding.level === "error")} aria-describedby={describedBy("response_example")} onChange={(event) => markDraftChanged((current) => ({ ...current, response_example_text: event.target.value }))} placeholder={'{\n  "ready": true\n}'} />{renderFieldFindings("response_example")}</label></div>
        </fieldset>
      </div>

      <aside className="tool-builder-rail" aria-label="Draft readiness">
        <section className="panel tool-builder-rail-panel"><PanelHeader title="Readiness" description="Local findings update immediately. Server checks never execute the tool." action={<Badge color={errors.length ? "red" : warnings.length ? "amber" : "green"}>{errors.length ? `${errors.length} errors` : warnings.length ? `${warnings.length} warnings` : "ready"}</Badge>} /><div className="tool-builder-rail-body"><FindingList findings={findings} onOpen={openFinding} /><Button type="button" outline className="full" disabled={Boolean(busy) || !local.draft} onClick={validateDraft}><ShieldCheck data-slot="icon" />{busy === "validate" ? "Checking…" : "Check draft"}</Button></div></section>
        <section className="panel tool-builder-rail-panel tool-builder-safety"><PanelHeader title="Safety boundary" /><div className="tool-builder-rail-body"><span><KeyRound /><small>{apiScoped ? "Credential managed once in API Access" : "Credential sent only on Save draft"}</small></span><span><Bot /><small>AI receives non-secret fields only</small></span><span><TerminalSquare /><small>Validation performs no upstream call</small></span><span><ShieldCheck /><small>Publishing remains a separate review</small></span></div></section>
        <section className="panel tool-builder-rail-panel"><PanelHeader title="Draft state" /><dl className="tool-builder-draft-state"><div><dt>Identity</dt><dd><code>{form.namespace || "namespace"}.{form.name || "tool"}</code></dd></div><div><dt>Scope</dt><dd>{apiContext ? apiContext.integration.display_name : "Common"}</dd></div><div><dt>Method</dt><dd>{form.http_method}</dd></div><div><dt>Authentication</dt><dd>{apiScoped ? "Inherited from API" : activeAuth.label}</dd></div><div><dt>Credential</dt><dd>{apiScoped ? runtimeLock?.credentialPresent ? "Managed in Access" : "Not required" : credential ? "Replacement entered" : form.credential_present && storedCredentialReusable ? "Stored" : CREDENTIAL_AUTH_TYPES.has(form.upstream_auth.type) ? "Missing" : "Not required"}</dd></div></dl></section>
      </aside>
    </div>
  </form>;
}
