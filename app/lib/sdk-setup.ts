import type { APIIntegration } from "./api";
import { sectionPath } from "./console-routes";
import type { APISDKBinding, APIResourceBindings, ReviewDecision, SDKContentCandidate, SDKContentPublication, SDKContentPublicationRecord, SDKIngestionFile, SDKPackage, SDKRelease, SDKReleaseLifecycleState } from "./developer-assets-api";

const fields = ["package", "release", "api", "candidate", "publication", "step"] as const;
export type SDKSetupSelection = Record<(typeof fields)[number], string>;
export function parseSDKSetupSelection(search = ""): SDKSetupSelection {
  const params = new URLSearchParams(search);
  return Object.fromEntries(fields.map((key) => [key, (params.get(key) ?? "").trim()])) as SDKSetupSelection;
}
export function sdkSetupPath(selection: Partial<SDKSetupSelection>) {
  const params = new URLSearchParams();
  for (const key of fields) if (selection[key]) params.set(key, selection[key]!);
  return sectionPath("sdks") + (params.size ? `?${params}` : "");
}
export function validSDKSetupProgress(value: unknown): value is SDKSetupSelection {
  return !!value && typeof value === "object" && fields.every((key) => typeof (value as Record<string, unknown>)[key] === "string" && String((value as Record<string, unknown>)[key]).length <= 200);
}
export class SDKSetupChangedError extends Error {
  constructor() { super("sdk_setup_changed"); }
}

export type SDKPendingInput = { files: { source_path: string; raw_hash: string; language: string; media_type: string; role: string }[] };
export function validSDKPendingInput(value: unknown): value is SDKPendingInput {
  if (!value || typeof value !== "object" || !Array.isArray((value as SDKPendingInput).files)) return false;
  const files = (value as SDKPendingInput).files;
  return files.length > 0 && files.length <= 500 && files.every((file) => file && ["source_path", "raw_hash", "language", "media_type", "role"].every((key) => typeof file[key as keyof typeof file] === "string"));
}
export async function fingerprintSDKInput(files: SDKIngestionFile[]): Promise<SDKPendingInput> {
  const aliases: Record<string, string> = { ts: "typescript", tsx: "typescript", js: "javascript", jsx: "javascript", py: "python", golang: "go", cs: "csharp", "c#": "csharp", rb: "ruby", sh: "shell", bash: "shell", yml: "yaml", md: "markdown" };
  return { files: await Promise.all(files.map(async (file) => {
    const hash = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(file.content));
    return { source_path: file.source_path.trim(), raw_hash: `sha256:${Array.from(new Uint8Array(hash), (byte) => byte.toString(16).padStart(2, "0")).join("")}`, language: aliases[file.language] ?? file.language, media_type: file.media_type, role: file.role };
  })) };
}
export function recoverSDKInput(candidates: SDKContentCandidate[], pending: SDKPendingInput) {
  const matches = candidates.filter((candidate) => candidate.source_manifest.length === pending.files.length && pending.files.every((file) => candidate.source_manifest.some((entry) => Object.entries(file).every(([key, value]) => entry[key] === value))));
  if (matches.length > 1) throw new SDKSetupChangedError();
  return matches[0];
}

function canonicalEvidence(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalEvidence);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonicalEvidence(item)]));
  return value;
}
function normalizedDecisions(decisions: ReviewDecision[]) {
  return decisions.map((value) => [value.id, value.decision, value.reason?.trim() ?? "", JSON.stringify(canonicalEvidence(value.review_evidence ?? null))]).sort((a, b) => a[0].localeCompare(b[0]));
}
export function sdkPublicationDecisions(record: SDKContentPublicationRecord): { files: ReviewDecision[]; samples: ReviewDecision[] } {
  return {
    files: record.file_selections.map((value) => ({ id: String(value.sdk_publication_file_id), decision: String(value.decision) as ReviewDecision["decision"], reason: typeof value.reason === "string" ? value.reason : undefined })),
    samples: record.sample_selections.map((value) => ({ id: String(value.sdk_code_sample_id), decision: value.decision!, reason: value.reason, review_evidence: value.review_evidence })),
  };
}
export function sdkReviewMatchesPublication(record: SDKContentPublicationRecord, files: ReviewDecision[], samples: ReviewDecision[]) {
  const saved = sdkPublicationDecisions(record);
  return JSON.stringify(normalizedDecisions(saved.files)) === JSON.stringify(normalizedDecisions(files)) && JSON.stringify(normalizedDecisions(saved.samples)) === JSON.stringify(normalizedDecisions(samples));
}

type SDKSetupTransport = {
  integration: (id: string) => Promise<APIIntegration>;
  package: (id: string) => Promise<SDKPackage>;
  release: (packageID: string, releaseID: string) => Promise<SDKRelease>;
  lifecycle: (packageID: string, releaseID: string) => Promise<SDKReleaseLifecycleState>;
  publications: (releaseID: string) => Promise<SDKContentPublication[]>;
  publication: (releaseID: string, publicationID: string) => Promise<SDKContentPublicationRecord>;
  publish: (releaseID: string, candidateID: string, files: ReviewDecision[], samples: ReviewDecision[]) => Promise<SDKContentPublication>;
  resources: (apiID: string) => Promise<APIResourceBindings>;
  attach: (apiID: string, input: Partial<APISDKBinding> & Pick<APISDKBinding, "sdk_package_id" | "sdk_release_id">) => Promise<unknown>;
  change: (apiID: string, bindingID: string, input: Partial<APISDKBinding> & Pick<APISDKBinding, "sdk_package_id" | "sdk_release_id" | "revision">) => Promise<unknown>;
};

type SDKSetupBinding = Partial<APISDKBinding> & Pick<APISDKBinding, "sdk_package_id" | "sdk_release_id">;
export function sdkSetupBinding(pkg: SDKPackage, release: SDKRelease, publication: SDKContentPublication, api: APIIntegration, reviewed?: APISDKBinding): SDKSetupBinding {
  const sameEvidence = reviewed?.sdk_release_id === release.id && reviewed.sdk_content_publication_id === publication.id;
  // Compatibility/test claims and selectors apply to their exact evidence.
  const claims = sameEvidence ? { coverage: reviewed.coverage, assurance: reviewed.assurance, compatibility_assertion_id: reviewed.compatibility_assertion_id, api_contract_revision_id: reviewed.api_contract_revision_id, applicable_modules: reviewed.applicable_modules, applicable_capabilities: reviewed.applicable_capabilities, applicable_operation_keys: reviewed.applicable_operation_keys, selector: reviewed.selector }
    : { coverage: "unknown" as const, assurance: "related" as const, compatibility_assertion_id: "", api_contract_revision_id: "", applicable_modules: [], applicable_capabilities: [], applicable_operation_keys: [], selector: {} };
  return { sdk_package_id: pkg.id, sdk_release_id: release.id, sdk_content_publication_id: publication.id, state: "ready", ...claims, visibility: sameEvidence && api.visibility !== "public" ? reviewed.visibility : api.visibility };
}
export function sdkSetupBindingMatches(binding: APISDKBinding | undefined, api: APIIntegration, expected: SDKSetupBinding) {
  if (!binding || binding.deployment_id !== api.deployment_id || binding.api_id !== api.id) return false;
  const selection = (value: SDKSetupBinding) => [value.sdk_package_id, value.sdk_release_id, value.sdk_content_publication_id, value.state, value.visibility,
    value.coverage ?? "unknown", value.assurance ?? "related", value.compatibility_assertion_id ?? "", value.api_contract_revision_id ?? "",
    value.applicable_modules ?? [], value.applicable_capabilities ?? [], value.applicable_operation_keys ?? [], canonicalEvidence(value.selector ?? {})];
  return JSON.stringify(selection(binding)) === JSON.stringify(selection(expected));
}

// Publication and attachment remain separate, immutable/audited server writes.
// A lost response is recovered by reading the exact publication and decisions.
export async function finishSDKSetup(input: {
  package: SDKPackage; release: SDKRelease; candidate: SDKContentCandidate;
  files: ReviewDecision[]; samples: ReviewDecision[]; api?: APIIntegration;
  expectedBinding?: APISDKBinding;
}, transport: SDKSetupTransport, checkpoint: (publication: SDKContentPublication) => void) {
  const { candidate, api, expectedBinding } = input;
  const checkAPI = async () => {
    if (!api) return;
    const current = await transport.integration(api.id);
    if (current.id !== api.id || current.deployment_id !== api.deployment_id || current.visibility !== api.visibility) throw new SDKSetupChangedError();
  };
  await checkAPI();
  const [pkg, release, lifecycle] = await Promise.all([transport.package(input.package.id), transport.release(input.package.id, input.release.id), transport.lifecycle(input.package.id, input.release.id)]);
  if (pkg.id !== input.package.id || pkg.deployment_id !== input.package.deployment_id || pkg.visibility !== input.package.visibility || pkg.lifecycle === "archived" || release.id !== input.release.id || release.release_hash !== input.release.release_hash || release.sdk_package_id !== pkg.id || release.deployment_id !== pkg.deployment_id || candidate.sdk_release_id !== release.id || candidate.deployment_id !== pkg.deployment_id || candidate.visibility !== release.visibility || lifecycle.sdk_release_id !== release.id || !lifecycle.selectable || (api && api.deployment_id !== pkg.deployment_id) || (api?.visibility === "public" && (pkg.visibility !== "public" || release.visibility !== "public"))) throw new SDKSetupChangedError();
  let publication = (await transport.publications(release.id)).find((value) => value.sdk_content_candidate_id === candidate.id);
  if (publication) {
    const record = await transport.publication(release.id, publication.id);
    if (record.publication.id !== publication.id || !sdkReviewMatchesPublication(record, input.files, input.samples)) throw new SDKSetupChangedError();
    publication = record.publication;
  } else {
    publication = await transport.publish(release.id, candidate.id, input.files, input.samples);
  }
  if (publication.sdk_release_id !== release.id || publication.sdk_content_candidate_id !== candidate.id || publication.deployment_id !== pkg.deployment_id || publication.content_hash !== candidate.content_hash || publication.visibility !== candidate.visibility) throw new SDKSetupChangedError();
  checkpoint(publication);
  if (!api) return publication;
  await checkAPI();
  const binding = sdkSetupBinding(pkg, release, publication, api, expectedBinding);
  const resources = await transport.resources(api.id);
  const existing = resources.sdks.find((value) => value.sdk_package_id === pkg.id && value.state !== "detached");
  if (sdkSetupBindingMatches(existing, api, binding)) return publication;
  if (existing?.id !== expectedBinding?.id || existing?.revision !== expectedBinding?.revision) throw new SDKSetupChangedError();
  if (existing && (existing.api_id !== api.id || existing.deployment_id !== api.deployment_id)) throw new SDKSetupChangedError();
  if (existing) await transport.change(api.id, existing.id, { ...binding, revision: existing.revision });
  else await transport.attach(api.id, binding);
  const saved = (await transport.resources(api.id)).sdks.find((value) => value.sdk_package_id === pkg.id && value.state !== "detached");
  if (!sdkSetupBindingMatches(saved, api, binding)) throw new SDKSetupChangedError();
  return publication;
}
