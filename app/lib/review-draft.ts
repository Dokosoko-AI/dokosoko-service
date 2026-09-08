type DraftStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const maxDraftAge = 7 * 24 * 60 * 60 * 1000;

export function reviewDraftKey(deploymentID: string, reviewerID: string, kind: string, candidateID: string) {
  return reviewerID && deploymentID && candidateID ? `dokosoko:review:v1:${[deploymentID, reviewerID, kind, candidateID].map(encodeURIComponent).join(":")}` : "";
}

export function readReviewDraft<T>(storage: DraftStorage, key: string, fingerprint: string, validate: (value: unknown) => value is T): T | null {
  if (!key) return null;
  try {
    const value = JSON.parse(storage.getItem(key) ?? "null");
    if (value?.version !== 1 || value.fingerprint !== fingerprint || typeof value.savedAt !== "number" || Date.now() - value.savedAt > maxDraftAge || !validate(value.decisions)) return null;
    return value.decisions;
  } catch { return null; }
}

// Drafts contain operator choices only. Final acknowledgment is deliberately
// absent and must always be renewed after opening the immutable candidate.
export function writeReviewDraft(storage: DraftStorage, key: string, fingerprint: string, decisions: unknown) {
  if (!key) return true;
  try {
    storage.setItem(key, JSON.stringify({ version: 1, fingerprint, decisions, savedAt: Date.now() }));
    return true;
  } catch { return false; }
}

export function clearReviewDraft(storage: DraftStorage, key: string) {
  if (!key) return;
  try { storage.removeItem(key); } catch { /* A retained draft can never acknowledge publication. */ }
}

export const browserReviewStorage: DraftStorage = {
  getItem: (key) => window.localStorage.getItem(key),
  setItem: (key, value) => window.localStorage.setItem(key, value),
  removeItem: (key) => window.localStorage.removeItem(key),
};
