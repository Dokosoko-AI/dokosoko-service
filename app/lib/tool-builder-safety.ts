import type { APIToolUpstreamAuth } from "./api";

function stableValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableValue).join(",")}]`;
  if (value && typeof value === "object") {
    const record = value as Record<string, unknown>;
    return `{${Object.keys(record).sort().map((key) => `${JSON.stringify(key)}:${stableValue(record[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

function endpointOrigin(value: string): string {
  try {
    return new URL(value).origin.toLowerCase();
  } catch {
    return "";
  }
}

/**
 * A browser-held replacement credential is usable only for the destination
 * origin and exact non-secret authentication configuration visible when the
 * operator entered it. Path-only changes intentionally keep the binding.
 */
export function toolCredentialBinding(endpoint: string, auth: APIToolUpstreamAuth): string {
  return `${endpointOrigin(endpoint)}\u0000${stableValue(auth)}`;
}

export function toolCredentialBindingMatches(binding: string, endpoint: string, auth: APIToolUpstreamAuth): boolean {
  return Boolean(binding) && binding === toolCredentialBinding(endpoint, auth);
}

export function versionedResponseIsCurrent(startedAt: number, current: number): boolean {
  return startedAt === current;
}
