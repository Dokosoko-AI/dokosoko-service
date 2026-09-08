import { sourceCreationAttempt, type SourceCreationAttempt } from "./source-creation-request";
import { reviewDraftKey } from "./review-draft";

function ordered(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(ordered);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, item]) => [key, ordered(item)]));
  return value;
}

export async function assetCreationAttempt(storage: Pick<Storage, "getItem" | "setItem" | "removeItem">, scope: { deploymentID: string; reviewerID: string; kind: "api_contract" | "documentation_collection"; context: string }, input: unknown, current?: SourceCreationAttempt | null) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(JSON.stringify(ordered(input))));
  const fingerprint = Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
  const key = reviewDraftKey(scope.deploymentID, scope.reviewerID, "asset-creation", `${scope.kind}:${scope.context}`);
  return { ...sourceCreationAttempt(storage, key, fingerprint, current), scope: key };
}
