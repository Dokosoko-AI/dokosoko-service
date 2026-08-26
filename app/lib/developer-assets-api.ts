import { request } from "./api-client";
import type { APIVisibility } from "./api-contracts";
import type * as Contract from "./control-plane.generated";

// Public DTOs come from the OpenAPI source of truth. Keep only UI-specific
// convenience shapes here so the console cannot silently lose immutable
// publication fields or weaken typed SDK evidence to unknown maps.
export type DeveloperAssetRecord = Record<string, unknown>;
export type DeveloperAssetKind = Contract.DeveloperAssetKind;
export type DeveloperAssetScope = NonNullable<Contract.DeveloperAssetQueryLabInput["scope"]>;
export type DeveloperAssetProcessorVersions = Contract.DeveloperAssetProcessorVersions;
export type DeveloperAssetIngestionRun = Contract.DeveloperAssetIngestionRun;
export type DeveloperAssetIngestionSummary = Contract.DeveloperAssetIngestionSummary;
export type DocumentationDocument = Contract.DocumentationDocument;
export type DocumentationSection = Contract.DocumentationSection;
export type SourcePublicationDocumentSelection = Contract.SourcePublicationDocumentSelection;
export type DocumentationCandidateRecord = Contract.DocumentationCandidateRecord;
export type DocumentationCollectionMemberInput = Contract.DocumentationCollectionMemberInput;
export type DocumentationCollectionInput = Contract.DocumentationCollectionInput;
export type DocumentationCollection = Contract.DocumentationCollection;
export type DocumentationCollectionRevision = Contract.DocumentationCollectionRevision;
export type DeveloperAssetMapArtifact = Contract.DeveloperAssetMapArtifact;
export type DocumentationCollectionRevisionRecord = Contract.DocumentationCollectionRevisionRecord;
export type DeploymentDocumentationPublication = Contract.DeploymentDocumentationPublication;
export type APIContractInput = Contract.ApiContractInput;
export type APIContract = Contract.ApiContract;
export type APIContractSource = Contract.ApiContractSource;
export type APIContractCandidate = Contract.ApiContractCandidate;
export type APIContractCandidateRecord = Contract.ApiContractCandidateRecord;
export type APIContractRevision = Contract.ApiContractRevision;
export type SDKPackageInput = Contract.SdkPackageInput;
export type SDKPackage = Contract.SdkPackage;
export type SDKReleaseInput = Contract.SdkReleaseInput;
export type SDKRelease = Contract.SdkRelease;
export type SDKReleaseLifecycleEventInput = Contract.SdkReleaseLifecycleEventInput;
export type SDKReleaseLifecycleEvent = Contract.SdkReleaseLifecycleEvent;
export type SDKReleaseLifecycleState = Contract.SdkReleaseLifecycleState;
export type SDKIngestionFile = Contract.SdkIngestionFile & Required<Pick<Contract.SdkIngestionFile, "media_type" | "language" | "role">>;
export type SDKContentCandidate = Contract.SdkContentCandidate;
export type SDKContentCandidateRecord = Contract.SdkContentCandidateRecord;
export type SDKContentPublication = Contract.SdkContentPublication;
export type ReviewDecision = Contract.DeveloperAssetReviewDecision;
export type APIDocumentationBinding = Contract.ApiDocumentationBinding;
export type APIContractBinding = Contract.ApiContractBinding;
export type APISDKBinding = Contract.ApisdkBinding;
export type APIResourceBindings = Contract.ApiResourceBindings;
export type APIDeveloperAssetPublication = Contract.ApiDeveloperAssetPublication;
export type DeveloperAssetCatalog = Contract.DeveloperAssetCatalog;
export type DeveloperAssetUsage = Contract.DeveloperAssetUsage;
export type QueryLabInput = Contract.DeveloperAssetQueryLabInput;
export type QueryLabResult = Contract.DeveloperAssetQueryLabResult;
export type QueryLabResponse = Contract.DeveloperAssetQueryLabResponse;
export type DeveloperAssetAIAdvisoryPromptKey = Contract.DeveloperAssetAiAdvisoryPromptKey;
export type DeveloperAssetAIAdvisoryGapCode = Contract.DeveloperAssetAiAdvisoryGapCode;
export type DeveloperAssetAIAdvisoryGap = Contract.DeveloperAssetAiAdvisoryGap;
export type DeveloperAssetAIMapResult = Contract.DeveloperAssetAiMapResult;
export type DeveloperAssetAIApplicabilityResult = Contract.DeveloperAssetAiApplicabilityResult;
export type DeveloperAssetAISampleReviewResult = Contract.DeveloperAssetAiSampleReviewResult;
export type DeveloperAssetAIAdvisoryResult =
  | DeveloperAssetAIMapResult
  | DeveloperAssetAIApplicabilityResult
  | DeveloperAssetAISampleReviewResult;
export type DeveloperAssetAIAdvisoryRun = Contract.DeveloperAssetAiAdvisoryRun;

// openapi-typescript flattens this discriminator into optional union members.
// Restore exact narrowing while sourcing every field name from the contract.
type AdvisoryRequest = Contract.DeveloperAssetAiAdvisoryInput;
export type DeveloperAssetAIAdvisoryInput =
  | Pick<AdvisoryRequest, "prompt_key" | "source_publication_id"> & {
    prompt_key: "documentation.map_enrichment";
    source_publication_id: string;
  }
  | Pick<AdvisoryRequest, "prompt_key" | "sdk_content_publication_id"> & {
    prompt_key: "sdk.map_enrichment";
    sdk_content_publication_id: string;
  }
  | Pick<AdvisoryRequest, "prompt_key" | "api_id" | "api_developer_asset_publication_id" | "api_sdk_binding_id" | "sdk_content_publication_id"> & {
    prompt_key: "sdk.applicability_suggestion";
    api_id: string;
    api_developer_asset_publication_id: string;
    api_sdk_binding_id: string;
    sdk_content_publication_id: string;
  }
  | Pick<AdvisoryRequest, "prompt_key" | "api_id" | "api_developer_asset_publication_id" | "api_sdk_binding_id" | "sdk_content_publication_id" | "sdk_code_sample_id"> & {
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
  usage: () => request<DeveloperAssetUsage>(`${developerAssetsPath}/usage`),
  ingestionRuns: (assetKind?: DeveloperAssetKind, targetKey = "") => {
    const query = new URLSearchParams();
    if (assetKind) query.set("asset_kind", assetKind);
    if (targetKey) query.set("target_key", targetKey);
    return items(request<Contract.DeveloperAssetIngestionRunList>(`${developerAssetsPath}/ingestion-runs${query.size ? `?${query}` : ""}`));
  },
  ingestionRun: (runID: string) => request<DeveloperAssetIngestionSummary>(`${developerAssetsPath}/ingestion-runs/${encode(runID)}`),
  documentationDocuments: (filters: { ingestion_run_id?: string; source_id?: string; source_publication_id?: string; query?: string; limit?: number; offset?: number } = {}) => {
    const query = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => { if (value !== undefined && value !== "") query.set(key, String(value)); });
    return request<Contract.DocumentationCandidateList>(`${developerAssetsPath}/documentation/documents${query.size ? `?${query}` : ""}`);
  },
  documentationDocument: (documentID: string) => request<DocumentationCandidateRecord>(`${developerAssetsPath}/documentation/documents/${encode(documentID)}`),
  documentationCollections: () => items(request<Contract.DocumentationCollectionList>(`${developerAssetsPath}/documentation-collections`)),
  createDocumentationCollection: (input: DocumentationCollectionInput) => request<DocumentationCollection>(`${developerAssetsPath}/documentation-collections`, { method: "POST", body: JSON.stringify(input) }),
  reviseDocumentationCollection: (collectionID: string, input: DocumentationCollectionInput) => request<DocumentationCollection>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  documentationCollectionRevisions: (collectionID: string) => items(request<Contract.DocumentationCollectionRevisionList>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}/revisions`)),
  documentationCollectionRevision: (collectionID: string, revisionID: string) => request<DocumentationCollectionRevisionRecord>(`${developerAssetsPath}/documentation-collections/${encode(collectionID)}/revisions/${encode(revisionID)}`),
  documentationPublications: () => items(request<Contract.DeploymentDocumentationPublicationList>(`${developerAssetsPath}/documentation-publications`)),
  publishDocumentation: (collectionRevisionIDs: string[], visibility: APIVisibility, expectedHeadRevision: number) => request<DeploymentDocumentationPublication>(`${developerAssetsPath}/documentation-publications`, { method: "POST", body: JSON.stringify({ collection_revision_ids: collectionRevisionIDs, visibility, expected_head_revision: expectedHeadRevision, acknowledge_reviewed: true }) }),
  documentationPublication: (publicationID: string) => request<DeploymentDocumentationPublication>(`${developerAssetsPath}/documentation-publications/${encode(publicationID)}`),
  apiContracts: () => items(request<Contract.ApiContractList>(`${developerAssetsPath}/api-contracts`)),
  createAPIContract: (input: APIContractInput) => request<APIContract>(`${developerAssetsPath}/api-contracts`, { method: "POST", body: JSON.stringify(input) }),
  updateAPIContract: (contractID: string, input: APIContractInput) => request<APIContract>(`${developerAssetsPath}/api-contracts/${encode(contractID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  archiveAPIContract: (contractID: string, revision: number) => request<APIContract>(`${developerAssetsPath}/api-contracts/${encode(contractID)}`, { method: "DELETE", body: JSON.stringify({ revision }) }),
  apiContractSources: (contractID: string) => items(request<Contract.ApiContractSourceList>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/sources`)),
  attachAPIContractSource: (contractID: string, sourceID: string, sourceRole: APIContractSource["source_role"] = "primary") => request<APIContractSource>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/sources`, { method: "POST", body: JSON.stringify({ source_id: sourceID, source_role: sourceRole }) }),
  apiContractCandidates: (contractID: string) => items(request<Contract.ApiContractCandidateList>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates`)),
  apiContractCandidate: (contractID: string, candidateID: string) => request<APIContractCandidateRecord>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates/${encode(candidateID)}`),
  publishAPIContractCandidate: (contractID: string, candidateID: string, contractRevision: number) => request<Contract.ApiContractCandidatePublicationResult>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/candidates/${encode(candidateID)}/publish`, { method: "POST", body: JSON.stringify({ contract_revision: contractRevision, acknowledge_reviewed: true }) }),
  apiContractRevisions: (contractID: string) => items(request<Contract.ApiContractRevisionList>(`${developerAssetsPath}/api-contracts/${encode(contractID)}/revisions`)),
  sdkPackages: () => items(request<Contract.SdkPackageList>(`${developerAssetsPath}/sdk-packages`)),
  createSDKPackage: (input: SDKPackageInput) => request<SDKPackage>(`${developerAssetsPath}/sdk-packages`, { method: "POST", body: JSON.stringify(input) }),
  updateSDKPackage: (packageID: string, input: SDKPackageInput) => request<SDKPackage>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  sdkReleases: (packageID: string) => items(request<Contract.SdkReleaseList>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases`)),
  createSDKRelease: (packageID: string, input: SDKReleaseInput) => request<SDKRelease>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases`, { method: "POST", body: JSON.stringify(input) }),
  sdkReleaseLifecycle: (packageID: string, releaseID: string) => request<SDKReleaseLifecycleState>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases/${encode(releaseID)}/lifecycle-events`),
  appendSDKReleaseLifecycleEvent: (packageID: string, releaseID: string, input: SDKReleaseLifecycleEventInput) => request<SDKReleaseLifecycleState>(`${developerAssetsPath}/sdk-packages/${encode(packageID)}/releases/${encode(releaseID)}/lifecycle-events`, { method: "POST", body: JSON.stringify(input) }),
  sdkContentCandidates: (releaseID: string) => items(request<Contract.SdkContentCandidateList>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates`)),
  sdkContentCandidate: (releaseID: string, candidateID: string) => request<SDKContentCandidateRecord>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates/${encode(candidateID)}`),
  ingestSDKContent: (releaseID: string, input: Contract.SdkContentIngestionInput) => request<Contract.SdkContentIngestionResult>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/ingestions`, { method: "POST", body: JSON.stringify(input) }),
  publishSDKContentCandidate: (releaseID: string, candidateID: string, files: ReviewDecision[], samples: ReviewDecision[]) => request<SDKContentPublication>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-candidates/${encode(candidateID)}/publish`, { method: "POST", body: JSON.stringify({ files, samples, acknowledge_reviewed: true }) }),
  sdkContentPublications: (releaseID: string) => items(request<Contract.SdkContentPublicationList>(`${developerAssetsPath}/sdk-releases/${encode(releaseID)}/content-publications`)),
  apiResources: (apiID: string) => request<APIResourceBindings>(`/api/v1/integrations/${encode(apiID)}/resources`),
  apiResourcePublications: (apiID: string) => items(request<Contract.ApiDeveloperAssetPublicationList>(`/api/v1/integrations/${encode(apiID)}/resources/publications`)),
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
    return items(request<Contract.DeveloperAssetAiAdvisoryRunList>(`${developerAssetsPath}/ai-advisories${query.size ? `?${query}` : ""}`));
  },
  aiAdvisory: (runID: string) => request<DeveloperAssetAIAdvisoryRun>(`${developerAssetsPath}/ai-advisories/${encode(runID)}`),
  runAIAdvisory: (input: DeveloperAssetAIAdvisoryInput) => request<DeveloperAssetAIAdvisoryRun>(`${developerAssetsPath}/ai-advisories`, { method: "POST", body: JSON.stringify(input) }),
  queryLab: (input: QueryLabInput) => request<QueryLabResponse>(`${developerAssetsPath}/query-lab`, { method: "POST", body: JSON.stringify(input) }),
};
