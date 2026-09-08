import { clearReviewDraft, readReviewDraft, writeReviewDraft } from "./review-draft";

type DraftStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
export type SourceCreationAttempt = { key: string; fingerprint: string };

// Local recovery contains only a random key and digest, never source text,
// URLs, file bytes, credentials or an acknowledgement of publication.
export async function sourceCreationFingerprint(input: { kind: string; location?: string; file?: File; name?: string }): Promise<string> {
  if (input.file && (input.file.size === 0 || input.file.size > 5_000_000)) throw new Error("Source uploads must contain 1 to 5,000,000 bytes.");
  const hash = async (value: Uint8Array<ArrayBuffer>) => Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", value))).map((byte) => byte.toString(16).padStart(2, "0")).join("");
  const content = input.file ? await hash(new Uint8Array(await input.file.arrayBuffer())) : input.location?.trim() ?? "";
  return hash(new TextEncoder().encode(JSON.stringify([input.kind, input.name?.trim() ?? "", input.file?.name ?? "", content])));
}

export function sourceCreationAttempt(storage: DraftStorage, scope: string, fingerprint: string, current?: SourceCreationAttempt | null): { attempt: SourceCreationAttempt; persisted: boolean } {
  const valid = (value: unknown): value is SourceCreationAttempt => {
    if (typeof value !== "object" || value === null || !("key" in value) || !("fingerprint" in value)) return false;
    return typeof value.key === "string" && /^[a-f0-9-]{36}$/.test(value.key) && value.fingerprint === fingerprint;
  };
  const attempt = current?.fingerprint === fingerprint ? current
    : readReviewDraft(storage, scope, fingerprint, valid) ?? { key: crypto.randomUUID(), fingerprint };
  return { attempt, persisted: Boolean(scope) && writeReviewDraft(storage, scope, fingerprint, attempt) };
}

export function finishSourceCreationAttempt(storage: DraftStorage, scope: string) {
  clearReviewDraft(storage, scope);
}
