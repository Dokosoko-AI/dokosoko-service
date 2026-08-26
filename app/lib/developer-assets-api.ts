import { request } from "./api-client";
import type { APIVisibility } from "./api-contracts";
import type { ApiDeveloperAssetPublication as GeneratedAPIDeveloperAssetPublication } from "./control-plane.generated";

export type DeveloperAssetKind = "documentation" | "contract" | "sdk";
export type DeveloperAssetScope = "global" | "api" | "combined";
export type DeveloperAssetRecord = Record<string, unknown>;

export type DeveloperAssetProcessorVersions = {
  pipeline: string;
  parser: string;
  normalizer: string;
  mapper: string;
};

export type DeveloperAssetIngestionRun = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  asset_kind: DeveloperAssetKind;
  target_id?: string;
  target_key: string;
  source_id?: string;
  resolved_source_uri?: string;
  resolved_source_revision?: string;
  resolved_source_hash?: string;
  state: "queued" | "running" | "review_ready" | "failed" | "cancelled" | "published";
  attempt: number;
  versions: DeveloperAssetProcessorVersions;
  diagnostics: DeveloperAssetRecord;
  discovered_count: number;
  acquired_count: number;
  failed_count: number;
  skipped_count: number;
  quarantined_count: number;
  queued_at: string;
  started_at?: string;
  finished_at?: string;
};

export type DeveloperAssetIngestionSummary = {
  run: DeveloperAssetIngestionRun;
  stages: Array<{
    id: string;
    ingestion_run_id: string;
    stage_name: string;
    attempt: number;
    state: string;
    input_hash?: string;
    output_hash?: string;
    checkpoint: DeveloperAssetRecord;
    diagnostics: DeveloperAssetRecord;
  }>;
};

export type DocumentationDocument = {
  id: string;
  deployment_id: string;
  ingestion_run_id: string;
  source_path: string;
  canonical_url?: string;
  title: string;
  document_kind: string;
  language?: string;
  media_type: string;
  normalized_markdown: string;
  content_hash: string;
  visibility: APIVisibility;
  ordinal: number;
  metadata: DeveloperAssetRecord;
};

export type DocumentationSection = {
  id: string;
  documentation_document_id: string;
  parent_section_id?: string;
  ordinal: number;
  heading?: string;
  anchor?: string;
  breadcrumb: string[];
  content_kind: string;
  normalized_text: string;
  code_language?: string;
  token_estimate: number;
  content_hash: string;
};

export type SourcePublicationDocumentSelection = {
  source_publication_id: string;
  deployment_id: string;
  documentation_document_id: string;
  decision: "included" | "excluded" | "quarantined";
  reason?: string;
  ordinal?: number;
  content_hash: string;
  reviewed_by: string;
  reviewed_at: string;
  created_at: string;
};

export type DocumentationCandidateRecord = {
  document: DocumentationDocument;
  sections: DocumentationSection[];
  run: DeveloperAssetIngestionRun;
  documentation_map?: DeveloperAssetMapArtifact;
  source_publication_selections: SourcePublicationDocumentSelection[];
};

export type DocumentationCollectionMemberInput = {
  kind: "source_publication" | "document" | "section";
  id: string;
  include_descendants?: boolean;
  selector?: DeveloperAssetRecord;
};

export type DocumentationCollectionInput = {
  name: string;
  slug: string;
  description?: string;
  visibility?: APIVisibility;
  lifecycle?: "active" | "archived";
  revision?: number;
  members: DocumentationCollectionMemberInput[];
  acknowledge_reviewed: true;
};

export type DocumentationCollection = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  name: string;
  slug: string;
  description: string;
  visibility: APIVisibility;
  lifecycle: "active" | "archived";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type DocumentationCollectionRevision = {
  id: string;
  deployment_id: string;
  documentation_collection_id: string;
  revision: number;
  visibility: APIVisibility;
  content_hash: string;
  selection_manifest: DeveloperAssetRecord[];
  reviewed_by: string;
  reviewed_at: string;
  published_at: string;
};

export type DeveloperAssetMapArtifact = {
  id: string;
  deployment_id?: string;
  ingestion_run_id?: string;
  documentation_collection_revision_id?: string;
  map_version: string;
  map: DeveloperAssetRecord;
  agent_markdown: string;
  content_hash: string;
  visibility?: APIVisibility;
  created_at?: string;
};

export type DocumentationCollectionRevisionRecord = {
  revision: DocumentationCollectionRevision;
  members: Array<DeveloperAssetRecord & {
    id: string;
    member_kind: "source_publication" | "document" | "section";
  }>;
  map?: DeveloperAssetMapArtifact;
};

export type DeploymentDocumentationPublication = {
  id: string;
  deployment_id: string;
  revision: number;
  visibility: APIVisibility;
  snapshot_schema_version: "deployment-documentation-v1";
  snapshot_hash: string;
  members: Array<{
    documentation_collection_revision_id: string;
    ordinal: number;
    content_hash: string;
    visibility: APIVisibility;
  }>;
  published_by: string;
  published_at: string;
  created_at: string;
};

export type APIContractInput = {
  name: string;
  slug: string;
  description?: string;
  visibility?: APIVisibility;
  lifecycle?: "active" | "archived";
  revision?: number;
};

export type APIContract = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  name: string;
  slug: string;
  description: string;
  contract_kind: "openapi";
  visibility: APIVisibility;
  lifecycle: "active" | "archived";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type APIContractSource = {
  id: string;
  deployment_id: string;
  api_contract_id: string;
  source_id: string;
  source_role: "primary" | "supplemental";
  lifecycle: "attached" | "detached";
  revision: number;
  created_by: string;
};

export type APIContractCandidate = {
  id: string;
  api_contract_id: string;
  ingestion_run_id: string;
  openapi_version?: string;
  source_format?: string;
  normalized_contract: DeveloperAssetRecord;
  source_hash?: string;
  content_hash: string;
  validation_result: DeveloperAssetRecord;
  visibility: APIVisibility;
  diagnostics: DeveloperAssetRecord;
};

export type APIContractCandidateRecord = {
  candidate: APIContractCandidate;
  operations: DeveloperAssetRecord[];
  schemas: DeveloperAssetRecord[];
  examples: DeveloperAssetRecord[];
  map?: DeveloperAssetMapArtifact;
};

export type APIContractRevision = {
  id: string;
  deployment_id: string;
  api_contract_id: string;
  api_contract_candidate_id: string;
  revision: number;
  content_hash: string;
  visibility: APIVisibility;
  reviewed_by: string;
  reviewed_at: string;
  published_at: string;
};

export type SDKPackageInput = {
  ecosystem: string;
  coordinate: string;
  name: string;
  description?: string;
  registry_url?: string;
  source_url?: string;
  language?: string;
  platform?: string;
  visibility?: APIVisibility;
  lifecycle?: "draft" | "active" | "deprecated" | "archived";
  replacement_sdk_package_id?: string;
  deprecation_message?: string;
  revision?: number;
};

export type SDKPackage = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  ecosystem: string;
  canonical_coordinate: string;
  display_coordinate: string;
  name: string;
  description: string;
  registry_url?: string;
  source_url?: string;
  language?: string;
  platform?: string;
  visibility: APIVisibility;
  lifecycle: "draft" | "active" | "deprecated" | "archived";
  replacement_sdk_package_id?: string;
  deprecation_message?: string;
  revision: number;
  created_at: string;
  updated_at: string;
};

export type SDKReleaseInput = {
  exact_version: string;
  install_command?: string;
  documentation_url?: string;
  source_url?: string;
  resolved_source_revision?: string;
  upstream_digest?: string;
  identity_assurance?: "metadata_only" | "resolved_source" | "verified_digest";
  visibility?: APIVisibility;
  lifecycle?: "active" | "deprecated" | "yanked" | "archived";
};

export type SDKRelease = {
  id: string;
  deployment_id: string;
  sdk_package_id: string;
  exact_version: string;
  install_command: string;
  documentation_url?: string;
  source_url?: string;
  resolved_source_revision?: string;
  upstream_digest?: string;
  identity_assurance: "metadata_only" | "resolved_source" | "verified_digest";
  visibility: APIVisibility;
  lifecycle: "active" | "deprecated" | "yanked" | "archived";
  release_hash: string;
  published_at?: string;
  created_at: string;
};

export type SDKReleaseLifecycleEventInput = {
  lifecycle: SDKRelease["lifecycle"];
  reason: string;
  observed_source_uri?: string;
  observed_at?: string;
};

export type SDKReleaseLifecycleEvent = {
  id: string;
  sdk_release_id: string;
  lifecycle: SDKRelease["lifecycle"];
  reason?: string;
  observed_source_uri?: string;
  observed_at: string;
  recorded_by: string;
  created_at: string;
};

export type SDKReleaseLifecycleState = {
  sdk_release_id: string;
  initial_lifecycle: SDKRelease["lifecycle"];
  effective_lifecycle: SDKRelease["lifecycle"];
  selectable: boolean;
  effective_event?: SDKReleaseLifecycleEvent;
  events: SDKReleaseLifecycleEvent[];
};

export type SDKContentCandidate = {
  id: string;
  deployment_id: string;
  sdk_release_id: string;
  ingestion_run_id: string;
  versions: DeveloperAssetProcessorVersions;
  map_version: string;
  source_manifest: DeveloperAssetRecord[];
  content_hash: string;
  visibility: APIVisibility;
  diagnostics: DeveloperAssetRecord;
};

export type SDKContentCandidateRecord = {
  candidate: SDKContentCandidate;
  files: DeveloperAssetRecord[];
  sections: DeveloperAssetRecord[];
  symbols: DeveloperAssetRecord[];
  samples: DeveloperAssetRecord[];
  map?: DeveloperAssetMapArtifact;
  sample_refs: DeveloperAssetRecord[];
};

export type SDKContentPublication = {
  id: string;
  deployment_id: string;
  sdk_release_id: string;
  sdk_content_candidate_id: string;
  revision: number;
  content_hash: string;
  visibility: APIVisibility;
  reviewed_by: string;
  reviewed_at: string;
  published_at: string;
};

export type ReviewDecision = {
  id: string;
  decision: "included" | "approved" | "excluded" | "quarantined";
  reason?: string;
  review_evidence?: { summary: string; [key: string]: unknown };
};

export type APIDocumentationBinding = {
  id: string;
  deployment_id: string;
  api_id: string;
  documentation_collection_id: string;
  follow_latest: boolean;
  pinned_revision_id?: string;
  selector: DeveloperAssetRecord;
  visibility: APIVisibility;
  lifecycle: "attached" | "detached";
  revision: number;
};

export type APIContractBinding = {
  id: string;
  deployment_id: string;
  api_id: string;
  api_contract_id: string;
  follow_latest: boolean;
  pinned_revision_id?: string;
  primary: boolean;
  visibility: APIVisibility;
  lifecycle: "attached" | "detached";
  revision: number;
};

export type APISDKBinding = {
  id: string;
  deployment_id: string;
  api_id: string;
  sdk_package_id: string;
  sdk_release_id: string;
  sdk_content_publication_id?: string;
  api_contract_revision_id?: string;
  compatibility_assertion_id?: string;
  state: "draft" | "ready" | "detached";
  coverage: "full" | "partial" | "unknown";
  assurance: "related" | "documented" | "reviewed" | "tested" | "verified";
  applicable_modules: string[];
  applicable_capabilities: string[];
  applicable_operation_keys: string[];
  selector: DeveloperAssetRecord;
  selector_hash: string;
  visibility: APIVisibility;
  revision: number;
};

export type APIResourceBindings = {
  documentation: APIDocumentationBinding[];
  contracts: APIContractBinding[];
  sdks: APISDKBinding[];
};

export type APIDeveloperAssetPublication = GeneratedAPIDeveloperAssetPublication;

export type DeveloperAssetCatalog = {
  documentation: DocumentationCollection[];
  contracts: APIContract[];
  sdk_packages: SDKPackage[];
};

export type QueryLabInput = {
  scope?: DeveloperAssetScope;
  api_id?: string;
  deployment_documentation_publication_id?: string;
  api_developer_asset_publication_id?: string;
  query: string;
  asset_kinds?: DeveloperAssetKind[];
  languages?: string[];
  ecosystems?: string[];
  sdk_release_ids?: string[];
  exact_versions?: string[];
  limit?: number;
  context_token_limit?: number;
};

export type QueryLabResult = {
  rank: number;
  unit: {
    id: string;
    unit_kind: string;
    source_publication_kind: string;
    source_publication_id: string;
    source_entity_id: string;
    title?: string;
    breadcrumb: string[];
    content: string;
    language?: string;
    ecosystem?: string;
    identifiers: string[];
    visibility: APIVisibility;
    citation: DeveloperAssetRecord;
    metadata: DeveloperAssetRecord;
    content_hash: string;
  };
  excerpt: string;
  lexical_score: number;
  semantic_score: number;
  rerank_score: number;
  selected: true;
};

export type QueryLabResponse = {
  trace_id: string;
  resolved_scope: {
    scope: DeveloperAssetScope;
    api_id?: string;
    deployment_documentation_publication_id?: string;
    api_developer_asset_publication_id?: string;
  };
  results: QueryLabResult[];
  context_tokens: number;
  diagnostics: DeveloperAssetRecord;
};

export type DeveloperAssetAIAdvisoryPromptKey =
  | "documentation.map_enrichment"
  | "sdk.map_enrichment"
  | "sdk.applicability_suggestion"
  | "sdk.sample_review";

export type DeveloperAssetAIAdvisoryGapCode =
  | "missing_evidence"
  | "ambiguous_evidence"
  | "conflicting_evidence"
  | "version_boundary"
  | "truncated_evidence";

export type DeveloperAssetAIAdvisoryGap = {
  code: DeveloperAssetAIAdvisoryGapCode;
  evidence_ids: string[];
};

export type DeveloperAssetAIMapResult = {
  status: "ready" | "uncertain";
  entries: Array<{
    kind:
      | "topic" | "workflow" | "authentication" | "error" | "example" | "version" | "language"
      | "installation" | "initialization" | "module" | "symbol" | "sample" | "pagination"
      | "retry" | "webhook" | "deprecation" | "migration";
    title: string;
    summary: string;
    evidence_ids: string[];
  }>;
  gaps: DeveloperAssetAIAdvisoryGap[];
};

export type DeveloperAssetAIApplicabilityResult = {
  status: "suggested" | "uncertain" | "unsupported";
  coverage: "partial" | "unknown";
  selectors: Array<{
    kind: "module" | "operation" | "sample";
    value: string;
    evidence_ids: string[];
  }>;
  gaps: DeveloperAssetAIAdvisoryGap[];
};

export type DeveloperAssetAISampleReviewResult = {
  recommendation: "pass" | "revise" | "uncertain";
  findings: Array<{
    code:
      | "version_mismatch" | "unsupported_import" | "unsupported_symbol" | "unsupported_operation"
      | "unsupported_field" | "authentication_assumption" | "missing_prerequisite" | "unobservable_result"
      | "unsafe_placeholder" | "secret_like_content" | "destructive_behavior" | "hidden_network_assumption"
      | "cross_api_evidence" | "mixed_release" | "uncited_claim" | "insufficient_evidence";
    evidence_ids: string[];
  }>;
};

export type DeveloperAssetAIAdvisoryResult =
  | DeveloperAssetAIMapResult
  | DeveloperAssetAIApplicabilityResult
  | DeveloperAssetAISampleReviewResult;

export type DeveloperAssetAIAdvisoryRun = {
  id: string;
  deployment_id: string;
  prompt_key: DeveloperAssetAIAdvisoryPromptKey;
  prompt_version: string;
  scope_kind: "documentation_publication" | "sdk_content_publication" | "sdk_api_binding" | "sdk_sample";
  scope_id: string;
  scope_visibility: APIVisibility;
  ingestion_run_id?: string;
  source_publication_id?: string;
  sdk_package_id?: string;
  sdk_release_id?: string;
  sdk_content_candidate_id?: string;
  sdk_content_publication_id?: string;
  api_id?: string;
  api_developer_asset_publication_id?: string;
  api_sdk_binding_id?: string;
  sdk_code_sample_id?: string;
  allowed_evidence_ids: string[];
  evidence_hash: string;
  input_hash: string;
  result: DeveloperAssetAIAdvisoryResult;
  result_hash: string;
  created_by: string;
  created_at: string;
};

export type DeveloperAssetAIAdvisoryInput =
  | { prompt_key: "documentation.map_enrichment"; source_publication_id: string }
  | { prompt_key: "sdk.map_enrichment"; sdk_content_publication_id: string }
  | {
    prompt_key: "sdk.applicability_suggestion";
    api_id: string;
    api_developer_asset_publication_id: string;
    api_sdk_binding_id: string;
    sdk_content_publication_id: string;
  }
  | {
    prompt_key: "sdk.sample_review";
    api_id: string;
    api_developer_asset_publication_id: string;
    api_sdk_binding_id: string;
    sdk_content_publication_id: string;
    sdk_code_sample_id: string;
  };

const developerAssetsPath = "/api/v1/developer-assets";
const encode = encodeURIComponent;
const items = async <T>(promise: Promise<{ items: T[] }>) => (await promise).items;

export const developerAssetsApi = {
  catalog: () => request<DeveloperAssetCatalog>(developerAssetsPath),
  ingestionRuns: (assetKind?: DeveloperAssetKind, targetKey = "") => {
    const query = new URLSearchParams();
    if (assetKind) query.set("asset_kind", assetKind);
    if (targetKey) query.set("target_key", targetKey);
    return items(request<{ items: DeveloperAssetIngestionRun[] }>(`${developerAssetsPath}/ingestion-runs${query.size ? `?${query}` : ""}`));
  },
  ingestionRun: (runID: string) => request<DeveloperAssetIngestionSummary>(`${developerAssetsPath}/ingestion-runs/${encode(runID)}`),
  documentationDocuments: (filters: { ingestion_run_id?: string; source_id?: string; source_publication_id?: string; query?: string; limit?: number; offset?: number } = {}) => {
    const query = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => { if (value !== undefined && value !== "") query.set(key, String(value)); });
    return request<{ items: DocumentationCandidateRecord[]; total: number; has_more: boolean }>(`${developerAssetsPath}/documentation/documents${query.size ? `?${query}` : ""}`);
  },
  documentationDocument: (documentID: string) => request<DocumentationCandidateRecord>(`${developerAssetsPath}/documentation/documents/${encode(documentID)}`),
  documentationCollections: () => items(request<{ items: DocumentationCollection[] }>(`${developerAssetsPath}/documentation-collections`)),
  createDocumentationCollection: (input: DocumentationCollectionInput) => request<DocumentationCollection>(`${developerAssetsPath}/documentation-collections`, { method: "POST", body: JSON.stringify(input) }),
  reviseDocumentationCollection: (collectionID: string, input: DocumentationCollectionInput) => request<DocumentationCollection>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  documentationCollectionRevisions: (collectionID: string) => items(request<{ items: DocumentationCollectionRevision[] }>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}/revisions`)),
  documentationCollectionRevision: (collectionID: string, revisionID: string) => request<DocumentationCollectionRevisionRecord>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}/revisions/${encode(revisionID)}`),
  documentationPublications: () => items(request<{ items: DeploymentDocumentationPublication[] }>(`${developerAssetsPath}/documentation-publications`)),
  publishDocumentation: (collectionRevisionIDs: string[], visibility: APIVisibility, expectedHeadRevision: number) => request<DeploymentDocumentationPublication>(`${developerAssetsPath}/documentation-publications`, { method: "POST", body: JSON.stringify({ collection_revision_ids: collectionRevisionIDs, visibility, expected_head_revision: expectedHeadRevision, acknowledge_reviewed: true }) }),
  documentationPublication: (publicationID: string) => request<DeploymentDocumentationPublication>(`${developerAssetsPath}/documentation-publications/${encode(publicationID)}`),
  apiContracts: () => items(request<{ items: APIContract[] }>(`${developerAssetsPath}/api-contracts`)),
  createAPIContract: (input: APIContractInput) => request<APIContract>(`${developerAssetsPath}/api-contracts`, { method: "POST", body: JSON.stringify(input) }),
  updateAPIContract: (contractID: string, input: APIContractInput) => request<APIContract>(`${developerAssetsPath}/api-contracts/${encode(contractID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  archiveAPIContract: (contractID: string, revision: number) => request<APIContract>(`${developerAssetsPath}/api-contracts/${encode(contractID)}`, { method: "DELETE", body: JSON.stringify({ revision }) }),
  apiContractSources: (contractID: string) => items(request<{ items: APIContractSource[] }>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/sources`)),
  attachAPIContractSource: (contractID: string, sourceID: string, sourceRole: APIContractSource["source_role"] = "primary") => request<APIContractSource>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/sources`, { method: "POST", body: JSON.stringify({ source_id: sourceID, source_role: sourceRole }) }),
  apiContractCandidates: (contractID: string) => items(request<{ items: APIContractCandidate[] }>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates`)),
  apiContractCandidate: (contractID: string, candidateID: string) => request<APIContractCandidateRecord>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates/${encode(candidateID)}`),
  publishAPIContractCandidate: (contractID: string, candidateID: string, contractRevision: number) => request<{ contract: APIContract; revision: APIContractRevision }>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates/${encode(candidateID)}/publish`, { method: "POST", body: JSON.stringify({ contract_revision: contractRevision, acknowledge_reviewed: true }) }),
  apiContractRevisions: (contractID: string) => items(request<{ items: APIContractRevision[] }>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/revisions`)),
  sdkPackages: () => items(request<{ items: SDKPackage[] }>(`${developerAssetsPath}/sdk-packages`)),
  createSDKPackage: (input: SDKPackageInput) => request<SDKPackage>(`${developerAssetsPath}/sdk-packages`, { method: "POST", body: JSON.stringify(input) }),
  updateSDKPackage: (packageID: string, input: SDKPackageInput) => request<SDKPackage>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  sdkReleases: (packageID: string) => items(request<{ items: SDKRelease[] }>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases`)),
  createSDKRelease: (packageID: string, input: SDKReleaseInput) => request<SDKRelease>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases`, { method: "POST", body: JSON.stringify(input) }),
  sdkReleaseLifecycle: (packageID: string, releaseID: string) => request<SDKReleaseLifecycleState>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases/${encode(releaseID)}/lifecycle-events`),
  appendSDKReleaseLifecycleEvent: (packageID: string, releaseID: string, input: SDKReleaseLifecycleEventInput) => request<SDKReleaseLifecycleState>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases/${encode(releaseID)}/lifecycle-events`, { method: "POST", body: JSON.stringify(input) }),
  sdkContentCandidates: (releaseID: string) => items(request<{ items: SDKContentCandidate[] }>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates`)),
  sdkContentCandidate: (releaseID: string, candidateID: string) => request<SDKContentCandidateRecord>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates/${encode(candidateID)}`),
  ingestSDKContent: (releaseID: string, input: { files: Array<{ source_path: string; content: string; media_type?: string; language?: string; role?: string }>; resolved_source_uri?: string; resolved_source_revision?: string; expected_source_hash?: string }) => request<{ run: DeveloperAssetIngestionRun; candidate: SDKContentCandidateRecord; already_ingested: boolean }>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/ingestions`, { method: "POST", body: JSON.stringify(input) }),
  publishSDKContentCandidate: (releaseID: string, candidateID: string, files: ReviewDecision[], samples: ReviewDecision[]) => request<SDKContentPublication>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates/${encode(candidateID)}/publish`, { method: "POST", body: JSON.stringify({ files, samples, acknowledge_reviewed: true }) }),
  sdkContentPublications: (releaseID: string) => items(request<{ items: SDKContentPublication[] }>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-publications`)),
  apiResources: (apiID: string) => request<APIResourceBindings>(`/api/v1/integrations/${encode(apiID)}/resources`),
  apiResourcePublications: (apiID: string) => items(request<{ items: APIDeveloperAssetPublication[] }>(`/api/v1/integrations/${encode(apiID)}/resources/publications`)),
  apiResourcePublication: (apiID: string, publicationID: string) => request<APIDeveloperAssetPublication>(`/api/v1/integrations/${encode(apiID)}/resources/publications/${encode(publicationID)}`),
  attachAPIDocumentation: (apiID: string, input: { documentation_collection_id: string; pinned_revision_id: string; selector?: DeveloperAssetRecord; visibility?: APIVisibility }) => request<APIDocumentationBinding>(`/api/v1/integrations/${encode(apiID)}/resources/documentation`, { method: "POST", body: JSON.stringify({ ...input, follow_latest: false, lifecycle: "attached" }) }),
  changeAPIDocumentation: (apiID: string, bindingID: string, input: { documentation_collection_id: string; pinned_revision_id: string; selector?: DeveloperAssetRecord; visibility?: APIVisibility; revision: number }) => request<APIDocumentationBinding>(`/api/v1/integrations/${encode(apiID)}/resources/documentation/${encode(bindingID)}`, { method: "PATCH", body: JSON.stringify({ ...input, follow_latest: false, lifecycle: "attached" }) }),
  detachAPIDocumentation: (apiID: string, bindingID: string, revision: number) => request<APIDocumentationBinding>(`/api/v1/integrations/${encode(apiID)}/resources/documentation/${encode(bindingID)}`, { method: "DELETE", body: JSON.stringify({ revision }) }),
  attachAPIContract: (apiID: string, input: { api_contract_id: string; pinned_revision_id: string; primary?: boolean; visibility?: APIVisibility }) => request<APIContractBinding>(`/api/v1/integrations/${encode(apiID)}/resources/contracts`, { method: "POST", body: JSON.stringify({ ...input, follow_latest: false, lifecycle: "attached" }) }),
  changeAPIContract: (apiID: string, bindingID: string, input: { api_contract_id: string; pinned_revision_id: string; primary?: boolean; visibility?: APIVisibility; revision: number }) => request<APIContractBinding>(`/api/v1/integrations/${encode(apiID)}/resources/contracts/${encode(bindingID)}`, { method: "PATCH", body: JSON.stringify({ ...input, follow_latest: false, lifecycle: "attached" }) }),
  detachAPIContract: (apiID: string, bindingID: string, revision: number) => request<APIContractBinding>(`/api/v1/integrations/${encode(apiID)}/resources/contracts/${encode(bindingID)}`, { method: "DELETE", body: JSON.stringify({ revision }) }),
  attachAPISDK: (apiID: string, input: Omit<Partial<APISDKBinding>, "id" | "deployment_id" | "api_id" | "revision" | "selector_hash"> & Pick<APISDKBinding, "sdk_package_id" | "sdk_release_id">) => request<APISDKBinding>(`/api/v1/integrations/${encode(apiID)}/resources/sdks`, { method: "POST", body: JSON.stringify(input) }),
  changeAPISDK: (apiID: string, bindingID: string, input: Omit<Partial<APISDKBinding>, "id" | "deployment_id" | "api_id" | "selector_hash"> & Pick<APISDKBinding, "sdk_package_id" | "sdk_release_id" | "revision">) => request<APISDKBinding>(`/api/v1/integrations/${encode(apiID)}/resources/sdks/${encode(bindingID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  detachAPISDK: (apiID: string, bindingID: string, revision: number) => request<APISDKBinding>(`/api/v1/integrations/${encode(apiID)}/resources/sdks/${encode(bindingID)}`, { method: "DELETE", body: JSON.stringify({ revision }) }),
  aiAdvisories: (filters: { prompt_key?: DeveloperAssetAIAdvisoryPromptKey; scope_id?: string; limit?: number } = {}) => {
    const query = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => { if (value !== undefined && value !== "") query.set(key, String(value)); });
    return items(request<{ items: DeveloperAssetAIAdvisoryRun[] }>(`${developerAssetsPath}/ai-advisories${query.size ? `?${query}` : ""}`));
  },
  aiAdvisory: (runID: string) => request<DeveloperAssetAIAdvisoryRun>(`${developerAssetsPath}/ai-advisories/${encode(runID)}`),
  runAIAdvisory: (input: DeveloperAssetAIAdvisoryInput) => request<DeveloperAssetAIAdvisoryRun>(`${developerAssetsPath}/ai-advisories`, { method: "POST", body: JSON.stringify(input) }),
  queryLab: (input: QueryLabInput) => request<QueryLabResponse>(`${developerAssetsPath}/query-lab`, { method: "POST", body: JSON.stringify(input) }),
};
