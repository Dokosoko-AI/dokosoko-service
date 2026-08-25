import type {
  APIRuntimeServiceConnection,
  APIRuntimeServiceConnectionRevision,
  APIRuntimeSetup,
  APIToolBuilderDraft,
  APIToolUpstreamAuth,
} from "./api";

export type APIToolBuilderRuntimeLock = {
  ownerIntegrationID: string;
  runtimeServiceConnectionID: string;
  baseURL: string;
  upstreamAuth: APIToolUpstreamAuth;
  credentialPresent: boolean;
};

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

/**
 * Picks one saved revision only as the non-secret preview used by Builder
 * assistance. Runtime execution still resolves the exact environment revision
 * server-side. Production is preferred so review copy matches the main path.
 */
export function runtimeConnectionPreview(
  setup: APIRuntimeSetup,
  connection?: APIRuntimeServiceConnection,
): APIRuntimeServiceConnectionRevision | undefined {
  if (!connection) return undefined;
  const productionIDs = new Set(setup.environments.filter((environment) => environment.is_production).map((environment) => environment.id));
  return connection.current_revisions?.find((revision) => revision.current && productionIDs.has(revision.environment_id))
    ?? connection.current_revisions?.find((revision) => revision.current);
}

export function runtimeLockForConnection(
  setup: APIRuntimeSetup,
  connection?: APIRuntimeServiceConnection,
): APIToolBuilderRuntimeLock | null {
  const revision = runtimeConnectionPreview(setup, connection);
  if (!connection || !revision) return null;
  const credential = revision.credential_set_id
    ? setup.credential_sets.find((candidate) => candidate.id === revision.credential_set_id)
    : undefined;
  const config = revision.auth_config ?? {};
  const upstreamAuth: APIToolUpstreamAuth = {
    type: revision.authentication_type,
    ...(stringValue(config.scheme) ? { scheme: stringValue(config.scheme) } : {}),
    ...(stringValue(config.header_name) || credential?.header_name ? { header_name: stringValue(config.header_name) || credential?.header_name } : {}),
    ...(stringValue(config.query_name) ? { query_name: stringValue(config.query_name) } : {}),
    ...(stringValue(config.prefix) ? { prefix: stringValue(config.prefix) } : {}),
    ...(stringValue(config.username) ? { username: stringValue(config.username) } : {}),
    ...(stringValue(config.client_id) ? { client_id: stringValue(config.client_id) } : {}),
    ...(stringValue(config.token_url) ? { token_url: stringValue(config.token_url) } : {}),
    ...(config.token_endpoint_auth_method === "client_secret_post" || config.token_endpoint_auth_method === "client_secret_basic"
      ? { token_endpoint_auth_method: config.token_endpoint_auth_method }
      : {}),
    ...(Array.isArray(config.scopes) ? { scopes: config.scopes.filter((value): value is string => typeof value === "string") } : {}),
    ...(stringValue(config.audience) ? { audience: stringValue(config.audience) } : {}),
    ...(stringValue(config.resource) ? { resource: stringValue(config.resource) } : {}),
  };
  return {
    ownerIntegrationID: setup.integration.id,
    runtimeServiceConnectionID: connection.id,
    baseURL: revision.base_url,
    upstreamAuth,
    credentialPresent: Boolean(credential?.credential_present),
  };
}

/** Convert an imported/AI absolute endpoint into the API-owned relative path. */
export function apiToolHTTPPath(value: string, fallback = "/"): string {
  const candidate = value.trim();
  if (!candidate) return fallback;
  try {
    const parsed = new URL(candidate);
    return `${parsed.pathname}${parsed.search}${parsed.hash}` || fallback;
  } catch {
    return candidate;
  }
}

export function apiToolHTTPPathProblem(value: string): string {
  const candidate = value.trim();
  if (!candidate) return "Enter a relative path, such as /v1/voices/{voice_id}.";
  if (!candidate.startsWith("/") || candidate.startsWith("//")) return "The path must start with one slash and cannot contain a host.";
  if (candidate.includes("?")) return "Move query values into request mapping; the relative path cannot contain a query string.";
  if (candidate.includes("#")) return "Remove the fragment from the relative path.";
  if (candidate.length > 2048 || /[\r\n\0]/.test(candidate)) return "The relative path is too long or contains unsupported control characters.";
  return "";
}

export function composeAPIToolPreviewEndpoint(baseURL: string, httpPath: string): string {
  try {
    const base = new URL(baseURL);
    // Prefix with the already selected origin so a malformed //host path can
    // never escape to a different preview destination.
    return new URL(`${base.origin}${httpPath.startsWith("/") ? httpPath : `/${httpPath}`}`).toString();
  } catch {
    return "";
  }
}

/**
 * Assistance may propose a path, but it cannot change API ownership, selected
 * connection, destination origin, authentication, or credential presence.
 */
export function lockAPIToolBuilderDraft(
  draft: APIToolBuilderDraft,
  lock: APIToolBuilderRuntimeLock,
  httpPath: string,
): APIToolBuilderDraft {
  return {
    ...draft,
    endpoint: composeAPIToolPreviewEndpoint(lock.baseURL, httpPath),
    upstream_auth: { ...lock.upstreamAuth },
    credential_present: lock.credentialPresent,
  };
}

export function apiToolPersistenceContext(lock: APIToolBuilderRuntimeLock, httpPath: string) {
  return {
    scope: "api" as const,
    owner_integration_id: lock.ownerIntegrationID,
    runtime_service_connection_id: lock.runtimeServiceConnectionID,
    http_path: httpPath.trim(),
  };
}
