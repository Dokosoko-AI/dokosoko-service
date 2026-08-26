import type { DeveloperAssetRecord, ReviewDecision } from "../../../lib/developer-assets-api";

export type SDKDecisionState = Record<string, { decision: "" | ReviewDecision["decision"]; reason: string; reviewEvidence: string }>;

export const maxSDKIngestionFiles = 500;
export const maxSDKIngestionFileBytes = 2_097_152;
export const maxSDKIngestionTotalBytes = 20_971_520;

function sdkRecordID(record: DeveloperAssetRecord, fallback: string) {
  for (const key of ["id", "file_id", "sample_id", "symbol_id", "section_id", "source_path", "name"]) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return fallback;
}

export function sdkTextBytes(value: string) {
  return new TextEncoder().encode(value).byteLength;
}

export function sdkBufferLooksText(buffer: ArrayBuffer) {
  const bytes = new Uint8Array(buffer);
  if (bytes.length === 0) return false;
  let controls = 0;
  for (const byte of bytes.subarray(0, Math.min(bytes.length, 8192))) {
    if (byte === 0) return false;
    if (byte < 32 && byte !== 9 && byte !== 10 && byte !== 13) controls += 1;
  }
  return controls / Math.min(bytes.length, 8192) < 0.02;
}

export function sdkLanguageForPath(path: string) {
  const extension = path.toLowerCase().split(".").pop() ?? "";
  return ({ md: "markdown", mdx: "markdown", json: "json", yaml: "yaml", yml: "yaml", toml: "toml", ts: "typescript", tsx: "tsx", js: "javascript", jsx: "jsx", py: "python", go: "go", rb: "ruby", rs: "rust", java: "java", kt: "kotlin", swift: "swift", php: "php", cs: "csharp", xml: "xml", html: "html", css: "css", txt: "text" } as Record<string, string>)[extension] ?? "text";
}

export function sdkNormalizedLocalPath(value: string) {
  const path = value.replaceAll("\\", "/");
  if (!path || path.startsWith("/") || /^[a-z]:\//i.test(path) || path.includes("\0")) return "";
  const segments = path.split("/");
  if (segments.some((segment) => !segment || segment === "." || segment === "..")) return "";
  return segments.join("/");
}

export function sampleValidated(sample: DeveloperAssetRecord) {
  const positiveStatuses = new Set(["syntax_checked", "compiled", "contract_tested", "executed"]);
  if (typeof sample.validation_status !== "string" || !positiveStatuses.has(sample.validation_status)) return false;
  const evidence = sample.validation_evidence;
  if (!evidence || typeof evidence !== "object" || Array.isArray(evidence)) return false;
  const value = evidence as DeveloperAssetRecord;
  const positive = value.validated === true || value.passed === true;
  const identifiedValidator = (typeof value.validator === "string" && Boolean(value.validator.trim()))
    || (typeof value.evidence_id === "string" && Boolean(value.evidence_id.trim()));
  return positive && identifiedValidator;
}

export function decisionsComplete(records: DeveloperAssetRecord[], decisions: SDKDecisionState, kind: "file" | "sample") {
  return records.every((record, index) => {
    const current = decisions[sdkRecordID(record, `${kind}-${index}`)];
    if (!current?.decision) return false;
    if ((current.decision === "excluded" || current.decision === "quarantined") && !current.reason.trim()) return false;
    if (kind === "sample" && current.decision === "approved" && !sampleValidated(record) && !current.reviewEvidence.trim()) return false;
    return kind === "file" ? current.decision !== "approved" : current.decision !== "included";
  });
}

export function decisionPayload(records: DeveloperAssetRecord[], decisions: SDKDecisionState, kind: "file" | "sample"): ReviewDecision[] {
  return records.map((record, index) => {
    const id = sdkRecordID(record, `${kind}-${index}`);
    const current = decisions[id];
    return {
      id,
      decision: current.decision as ReviewDecision["decision"],
      ...((current.decision === "excluded" || current.decision === "quarantined") ? { reason: current.reason.trim() } : {}),
      ...(kind === "sample" && current.decision === "approved" && !sampleValidated(record) ? { review_evidence: { summary: current.reviewEvidence.trim() } } : {}),
    };
  });
}

const sdkExplorerSearchKeys = ["source_path", "path", "title", "name", "qualified_name", "symbol", "language", "code_language", "role", "content_hash"];

export function sdkExplorerRecordMatches(record: DeveloperAssetRecord, query: string, fallback: string) {
  if (!query) return true;
  const searchable = [fallback, ...sdkExplorerSearchKeys.map((key) => record[key]).filter((value): value is string => typeof value === "string")].join("\n").toLowerCase();
  return searchable.includes(query);
}
