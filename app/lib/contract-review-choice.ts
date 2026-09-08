import type { APIIntegration, APISourceReview } from "./api";
import type { APIContract, APIContractCandidate } from "./developer-assets-api";
import { contractSourceFingerprint } from "./contract-setup";
import { reviewDraftKey } from "./review-draft";

export type ContractReviewChoice = { primary: boolean };
export function validContractReviewChoice(value: unknown): value is ContractReviewChoice {
  return !!value && typeof value === "object" && "primary" in value && typeof value.primary === "boolean" && Object.keys(value).length === 1;
}

// Publication increments source/root revisions without changing this review.
// Bind the choice to its immutable evidence, originating API and audience.
// Binding changes require a new acknowledgement and current-revision check;
// they must not erase the intended primary choice or imply it was completed.
// The fingerprint contains only identities/hashes/status, never source bodies.
export function contractReviewChoiceScope(input: {
  contract: APIContract; candidate: APIContractCandidate; sourceReview: APISourceReview;
  api: APIIntegration;
}, reviewerID: string) {
  const { contract, candidate, sourceReview, api } = input;
  return {
    key: reviewDraftKey(candidate.deployment_id, reviewerID, "contract-review", JSON.stringify([contract.id, candidate.id, api.id])),
    fingerprint: JSON.stringify([contract.id, contract.visibility, contract.lifecycle,
      candidate.id, candidate.api_contract_id, candidate.deployment_id, candidate.ingestion_run_id, candidate.content_hash, candidate.visibility,
      contractSourceFingerprint(sourceReview), api.id, api.deployment_id, api.visibility]),
  };
}
