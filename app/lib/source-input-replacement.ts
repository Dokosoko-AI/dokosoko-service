import { clearReviewDraft, readReviewDraft, writeReviewDraft } from "./review-draft";

type DraftStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
export type SourceInputReplacementAttempt = { key: string; fingerprint: string; revision: number };
const format = "source-input-replacement-v1";

export function readSourceInputReplacement(storage: DraftStorage, scope: string): SourceInputReplacementAttempt | null {
  return readReviewDraft(storage, scope, format, (value): value is SourceInputReplacementAttempt => {
    if (typeof value !== "object" || value === null || !("key" in value) || !("fingerprint" in value) || !("revision" in value)) return false;
    return typeof value.key === "string" && /^[a-f0-9-]{36}$/.test(value.key) && typeof value.fingerprint === "string" && /^[a-f0-9]{64}$/.test(value.fingerprint) && typeof value.revision === "number" && Number.isSafeInteger(value.revision) && value.revision > 0;
  });
}

export function sourceInputReplacementAttempt(storage: DraftStorage, scope: string, fingerprint: string, revision: number, current?: SourceInputReplacementAttempt | null) {
  const saved = current ?? readSourceInputReplacement(storage, scope);
  const attempt = saved?.fingerprint === fingerprint ? saved : { key: crypto.randomUUID(), fingerprint, revision };
  return { attempt, persisted: Boolean(scope) && writeReviewDraft(storage, scope, format, attempt) };
}

export function finishSourceInputReplacement(storage: DraftStorage, scope: string) {
  clearReviewDraft(storage, scope);
}
