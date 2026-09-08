import type { APICrawlJob, APIIntegration, APISourceReview } from "./api";
import { sectionPath } from "./console-routes";
import type { APIContract, APIContractBinding, APIContractCandidate, APIContractRevision, APIResourceBindings } from "./developer-assets-api";

export type ContractSetupSelection = { contract: string; api: string; source: string; run: string; candidate: string; revision: string; input: string; queue: string; after: string };
const selectionFields = ["contract", "api", "source", "run", "candidate", "revision", "input", "queue", "after"] as const;
export function parseContractSetupSelection(search = ""): ContractSetupSelection {
  const params = new URLSearchParams(search);
  return Object.fromEntries(selectionFields.map((key) => [key, (params.get(key) ?? "").trim()])) as ContractSetupSelection;
}
export function contractSetupPath(selection: Partial<ContractSetupSelection>) {
  const params = new URLSearchParams();
  for (const key of selectionFields) if (selection[key]) params.set(key, selection[key]!);
  return `${sectionPath("contracts")}${params.size ? `?${params}` : ""}`;
}
export function pendingContractImport(jobs: APICrawlJob[], selection: Pick<ContractSetupSelection, "queue" | "after">) {
  if (selection.queue !== "pending") return undefined;
  const boundary = selection.after ? jobs.findIndex((job) => job.id === selection.after) : jobs.length;
  if (boundary < 0) throw new ContractSetupChangedError();
  return boundary > 0 ? jobs[0] : undefined;
}

export function contractCandidateValid(candidate: APIContractCandidate) {
  return candidate.validation_result.valid === true && (candidate.validation_result.errors == null || Array.isArray(candidate.validation_result.errors) && candidate.validation_result.errors.length === 0);
}
export function contractSourceFingerprint(review: APISourceReview) {
  return JSON.stringify([review.source.product_id, review.source.id, review.source.visibility, review.source.quarantined, review.crawl_job.id,
    review.crawl_job.failed_count, review.crawl_job.skipped_count,
    review.documents.map((document) => [document.id, document.content_hash, document.state === "published" ? "validated" : document.state, document.injection_indicators]).sort((a, b) => String(a[0]).localeCompare(String(b[0])))]);
}

export function contractAttachmentVisibility(api: APIIntegration, binding?: APIContractBinding) {
  return api.visibility === "public" ? "public" : binding?.visibility ?? api.visibility;
}
export function contractBindingMatches(binding: APIContractBinding | undefined, revision: APIContractRevision, api: APIIntegration, primary: boolean, visibility: APIIntegration["visibility"]) {
  return binding?.deployment_id === api.deployment_id && binding.api_id === api.id && binding.lifecycle === "attached" &&
    binding.api_contract_id === revision.api_contract_id && binding.pinned_revision_id === revision.id && binding.follow_latest === false &&
    binding.primary === primary && binding.visibility === visibility && (api.visibility !== "public" || visibility === "public");
}

export class ContractSetupChangedError extends Error {
  constructor() { super("contract_setup_changed"); }
}

type ContractSetupTransport = {
  integration: (id: string) => Promise<APIIntegration>;
  contract: (id: string) => Promise<APIContract>;
  revisions: (id: string) => Promise<APIContractRevision[]>;
  review: (productID: string, sourceID: string, runID: string) => Promise<APISourceReview>;
  publishSource: (review: APISourceReview) => Promise<unknown>;
  publishContract: (contractID: string, candidateID: string, revision: number) => Promise<{ revision: APIContractRevision }>;
  resources: (apiID: string) => Promise<APIResourceBindings>;
  attach: (apiID: string, contractID: string, revisionID: string, primary: boolean, visibility: APIIntegration["visibility"]) => Promise<unknown>;
  change: (apiID: string, binding: APIContractBinding, revisionID: string, primary: boolean, visibility: APIIntegration["visibility"]) => Promise<unknown>;
};

// Each publication remains a separate immutable record. Re-reading before each
// transition makes a retry reuse records committed before a lost response.
export async function finishContractSetup(input: {
  contract: APIContract;
  candidate: APIContractCandidate;
  sourceReview: APISourceReview;
  api?: APIIntegration;
  expectedBinding?: APIContractBinding;
  primary: boolean;
}, transport: ContractSetupTransport, checkpoint: (revision: APIContractRevision) => void) {
  const { candidate, sourceReview, api, expectedBinding } = input;
  const checkAPI = async () => {
    if (!api) return;
    const current = await transport.integration(api.id);
    if (current.id !== api.id || current.deployment_id !== api.deployment_id || current.visibility !== api.visibility) throw new ContractSetupChangedError();
  };
  await checkAPI();
  const contract = await transport.contract(input.contract.id);
  if (contract.id !== candidate.api_contract_id || contract.deployment_id !== candidate.deployment_id || contract.visibility !== input.contract.visibility || contract.lifecycle !== "active" || candidate.ingestion_run_id !== sourceReview.crawl_job.id || candidate.deployment_id !== sourceReview.source.product_id || candidate.visibility !== sourceReview.source.visibility || (api?.visibility === "public" && candidate.visibility !== "public") || !contractCandidateValid(candidate) || (api && api.deployment_id !== contract.deployment_id)) throw new ContractSetupChangedError();
  let published = (await transport.revisions(contract.id)).find((revision) => revision.api_contract_candidate_id === candidate.id && revision.content_hash === candidate.content_hash && revision.api_contract_id === contract.id);
  if (!published) {
    if (contract.revision !== input.contract.revision) throw new ContractSetupChangedError();
    const review = await transport.review(contract.deployment_id, sourceReview.source.id, candidate.ingestion_run_id);
    if (contractSourceFingerprint(review) !== contractSourceFingerprint(sourceReview) || review.source.quarantined || review.crawl_job.failed_count > 0 || review.crawl_job.skipped_count > 0 || review.documents.length === 0 || review.documents.some((document) => !["validated", "published"].includes(document.state) || document.injection_indicators.length > 0)) throw new ContractSetupChangedError();
    if (!review.publication) await transport.publishSource(review);
    published = (await transport.publishContract(contract.id, candidate.id, contract.revision)).revision;
  }
  if (published.deployment_id !== contract.deployment_id || published.api_contract_id !== contract.id || published.api_contract_candidate_id !== candidate.id || published.content_hash !== candidate.content_hash || published.visibility !== candidate.visibility) throw new ContractSetupChangedError();
  checkpoint(published);
  if (api) {
    await checkAPI();
    const visibility = contractAttachmentVisibility(api, expectedBinding);
    const resources = await transport.resources(api.id);
    const existing = resources.contracts.find((binding) => binding.lifecycle === "attached" && binding.api_contract_id === contract.id);
    // A committed attachment may be observed after a failed/lost response.
    if (contractBindingMatches(existing, published, api, input.primary, visibility)) return published;
    if (existing?.id !== expectedBinding?.id || existing?.revision !== expectedBinding?.revision) throw new ContractSetupChangedError();
    if (existing && (existing.deployment_id !== api.deployment_id || existing.api_id !== api.id)) throw new ContractSetupChangedError();
    if (existing) await transport.change(api.id, existing, published.id, input.primary, visibility);
    else await transport.attach(api.id, contract.id, published.id, input.primary, api.visibility);
    const saved = (await transport.resources(api.id)).contracts.find((binding) => binding.lifecycle === "attached" && binding.api_contract_id === contract.id);
    if (!contractBindingMatches(saved, published, api, input.primary, visibility)) throw new ContractSetupChangedError();
  }
  return published;
}
