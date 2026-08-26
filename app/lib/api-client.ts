import type {
  APIAIProviderConnection,
  APIAIProviderUsage,
  APIAIWorkflowPrompt,
  APIAIWorkloadProfile,
  APIAIWorkloadUsage,
  APIAuditEvent,
  APIAuthorizationPoint,
  APICrawlJob,
  APICustomerAccount,
  APICustomerAccountPage,
  APIDeployment,
  APIEnvironment,
  APIGrantDefinition,
  APIIdentity,
  APIIdentityTest,
  APIIntegration,
  APIIntegrationAnalysis,
  APIIntegrationDetail,
  APIIntegrationPreflight,
  APIIntegrationResourceLink,
  APIIntegrationRevision,
  APIIntegrationToolBinding,
  APIMCPCatalog,
  APIMCPConnection,
  APIMCPImportResult,
  APIMCPPreview,
  APINativePlugin,
  APIOrganisation,
  APIProduct,
  APIRecipe,
  APIRecipeRevision,
  APIResourceSet,
  APIRuntimeCredentialSet,
  APIRuntimeCredentialSetInput,
  APIRuntimeServiceConnection,
  APIRuntimeServiceConnectionInput,
  APIRuntimeServiceConnectionReadiness,
  APIRuntimeSetup,
  APIRuntimeSetupInput,
  APISource,
  APISourcePublication,
  APISourcePublishResult,
  APISourceReview,
  APISupportSubmission,
  APISDKReference,
  APISDKReferenceInput,
  APITool,
  APIToolBuilderAnalysis,
  APIToolBuilderAnalysisInput,
  APIToolBuilderImportInput,
  APIToolBuilderImportResult,
  APIToolBuilderProposal,
  APIToolBuilderProposalInput,
  APIToolBuilderValidation,
  APIToolBuilderValidationInput,
  APIToolCreateInput,
  APIToolDryRun,
  APIToolTestAnalysis,
  APIToolTestAnalysisInput,
  APIToolTestConfirmation,
  APIToolTestConfirmationInput,
  APIToolTestRun,
  APIToolTestRunInput,
  APIToolUpdateInput,
  APIVisibility,
  Distribution
} from "./api-contracts";
import { boundedToolBuilderChatHistory, boundedToolTestAnalysisHistory } from "./api-contracts";

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;

  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export type APIUser = {
  id: string;
  email: string;
  display_name: string;
  role: "root" | string;
  revoked_at?: string;
};

export type SetupEnrollment = {
  enrollment_id: string;
  totp_secret: string;
  provisioning_uri: string;
  expires_at: string;
};

export type AuthSession = {
  user: APIUser;
  expires_at: string;
};

function cookie(name: string): string {
  if (typeof document === "undefined") return "";
  const prefix = `${encodeURIComponent(name)}=`;
  const part = document.cookie.split("; ").find((value) => value.startsWith(prefix));
  return part ? decodeURIComponent(part.slice(prefix.length)) : "";
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const credentials = init?.credentials ?? "same-origin";
  const csrfToken = credentials !== "omit" && !["GET", "HEAD", "OPTIONS"].includes(method) ? cookie("dokosoko_csrf") : "";
  const multipartBody = typeof FormData !== "undefined" && init?.body instanceof FormData;
  const response = await fetch(path, {
    ...init,
    credentials,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(init?.body && !multipartBody ? { "Content-Type": "application/json" } : {}),
      ...(csrfToken ? { "X-CSRF-Token": csrfToken } : {}),
      ...init?.headers,
    },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = payload?.error ?? {};
    throw new APIError(response.status, error.code ?? "request_failed", error.message ?? "Request failed.", error.details);
  }
  return payload as T;
}

const productPath = (productID: string) => `/api/v1/products/${encodeURIComponent(productID)}`;

export const api = {
  setupStatus: () => request<{ setup_complete: boolean; requires_mfa: boolean }>("/api/v1/setup/status"),
  beginSetup: (setupToken: string, input: { email: string; password: string }) => request<SetupEnrollment>("/api/v1/setup/begin", {
    method: "POST",
    headers: { Authorization: `Bearer ${setupToken}` },
    body: JSON.stringify(input),
  }),
  completeSetup: (enrollmentID: string, code: string) => request<{ user: APIUser; csrf_token: string; recovery_codes: string[] }>("/api/v1/setup/complete", {
    method: "POST",
    body: JSON.stringify({ enrollment_id: enrollmentID, code }),
  }),
  session: () => request<AuthSession>("/api/v1/auth/session"),
  login: (email: string, password: string, code: string) => request<AuthSession & { csrf_token: string }>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password, code }),
  }),
  logout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),
  rootUsers: async () => (await request<{ items: APIUser[] }>("/api/v1/root/users")).items,
  systemDoctor: () => request<{ status: "ok" | "error"; checks: Array<{ name: string; status: string; message: string }>; generated_at: string }>("/api/v1/system/doctor"),
  beginRootUser: (input: { email: string; display_name: string; password: string }) => request<SetupEnrollment>("/api/v1/root/users", { method: "POST", body: JSON.stringify(input) }),
  completeRootUser: (enrollmentID: string, code: string) => request<{ user: APIUser; recovery_codes: string[] }>("/api/v1/root/users", { method: "PUT", body: JSON.stringify({ enrollment_id: enrollmentID, code }) }),
  revokeRootUser: (userID: string) => request<void>(`/api/v1/root/users/${encodeURIComponent(userID)}`, { method: "DELETE" }),
  organisations: async () => (await request<{ items: APIOrganisation[] }>("/api/v1/organisations")).items,
  createOrganisation: (name: string, slug: string) => request<APIOrganisation>("/api/v1/organisations", { method: "POST", body: JSON.stringify({ name, slug }) }),
  deployment: () => request<APIDeployment>("/api/v1/deployment"),
  createDeployment: (organisationID: string, name: string, slug: string) => request<APIDeployment>("/api/v1/deployment", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug }) }),
  updateDeployment: (input: Partial<APIDeployment> & { revision: number }) => request<APIDeployment>("/api/v1/deployment", { method: "PATCH", body: JSON.stringify(input) }),
  deploymentEnvironments: async () => (await request<{ items: APIEnvironment[] }>("/api/v1/environments")).items,
  createDeploymentEnvironment: (organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>("/api/v1/environments", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  integrations: async () => (await request<{ items: APIIntegration[] }>("/api/v1/integrations")).items,
  integration: (integrationID: string) => request<APIIntegrationDetail>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`),
  integrationRuntimeSetup: (integrationID: string) => request<APIRuntimeSetup>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-setup`),
  configureIntegrationRuntimeSetup: (integrationID: string, input: APIRuntimeSetupInput) => request<APIRuntimeSetup>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-setup`, { method: "PUT", body: JSON.stringify(input) }),
  integrationRuntimeConnections: async (integrationID: string) => (await request<{ items: APIRuntimeServiceConnection[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-connections`)).items,
  createIntegrationRuntimeConnection: (integrationID: string, input: APIRuntimeServiceConnectionInput) => request<APIRuntimeServiceConnection>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-connections`, { method: "POST", body: JSON.stringify(input) }),
  checkRuntimeServiceConnection: (connectionID: string) => request<APIRuntimeServiceConnectionReadiness>(`/api/v1/runtime-service-connections/${encodeURIComponent(connectionID)}/check`, { method: "POST" }),
  createIntegrationRuntimeCredentialSet: (integrationID: string, input: APIRuntimeCredentialSetInput) => request<APIRuntimeCredentialSet>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/runtime-credential-sets`, { method: "POST", body: JSON.stringify(input) }),
  runtimeCredentialSet: (credentialSetID: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}`),
  runtimeCredentialUsage: (credentialSetID: string) => request<{ items: APIRuntimeServiceConnection[]; count: number }>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/usage`),
  rotateRuntimeCredential: (credentialSetID: string, credential: string, expiresAt?: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/rotate`, { method: "POST", body: JSON.stringify({ credential, ...(expiresAt ? { expires_at: expiresAt } : {}) }) }),
  revokeRuntimeCredentialVersion: (credentialSetID: string, versionID: string) => request<APIRuntimeCredentialSet>(`/api/v1/runtime-credential-sets/${encodeURIComponent(credentialSetID)}/versions/${encodeURIComponent(versionID)}/revoke`, { method: "POST" }),
  createIntegration: (input: { family_key: string; version_key: string; display_name: string; description: string; visibility?: APIVisibility; acknowledge_public?: boolean; lifecycle?: APIIntegration["lifecycle"] }) => request<APIIntegration>("/api/v1/integrations", { method: "POST", body: JSON.stringify(input) }),
  updateIntegration: (integrationID: string, input: Pick<APIIntegration, "family_key" | "version_key" | "display_name" | "description" | "visibility" | "lifecycle" | "revision"> & { acknowledge_public?: boolean; replacement_integration_id?: string; sunset_at?: string }) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}`, { method: "PUT", body: JSON.stringify(input) }),
  preflightIntegration: (integrationID: string) => request<APIIntegrationPreflight>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/preflight`, { method: "POST", body: JSON.stringify({}) }),
  publishIntegration: (integrationID: string, candidateRevision: number, candidateManifestHash: string) => request<APIIntegrationRevision>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/publish`, { method: "POST", body: JSON.stringify({ candidate_revision: candidateRevision, candidate_manifest_hash: candidateManifestHash }) }),
  integrationToolBindings: async (integrationID: string) => (await request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`)).items,
  setIntegrationToolBindings: (integrationID: string, tools: Array<{ tool_id: string; revision: number; authorization_point_id: string; authorization_point_revision: number }>) => request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`, { method: "PUT", body: JSON.stringify({ tools }) }),
  grantDefinitions: async () => (await request<{ items: APIGrantDefinition[] }>("/api/v1/grant-definitions")).items,
  createGrantDefinition: (input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"] }) => request<APIGrantDefinition>("/api/v1/grant-definitions", { method: "POST", body: JSON.stringify(input) }),
  updateGrantDefinition: (grantID: string, input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"]; revision: number }) => request<APIGrantDefinition>(`/api/v1/grant-definitions/${encodeURIComponent(grantID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  authorizationPoints: async (integrationID: string) => (await request<{ items: APIAuthorizationPoint[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`)).items,
  createAuthorizationPoint: (integrationID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"] }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`, { method: "POST", body: JSON.stringify(input) }),
  updateAuthorizationPoint: (integrationID: string, pointID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"]; revision: number }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points/${encodeURIComponent(pointID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  integrationSDKs: async (integrationID: string) => (await request<{ items: APISDKReference[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/sdks`)).items,
  createIntegrationSDK: (integrationID: string, input: APISDKReferenceInput) => request<APISDKReference>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/sdks`, { method: "POST", body: JSON.stringify(input) }),
  replaceIntegrationSDK: (integrationID: string, sdkID: string, input: APISDKReferenceInput) => request<APISDKReference>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/sdks/${encodeURIComponent(sdkID)}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteIntegrationSDK: (integrationID: string, sdkID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/sdks/${encodeURIComponent(sdkID)}`, { method: "DELETE" }),
  resourceSets: async (kind = "") => (await request<{ items: APIResourceSet[] }>(`/api/v1/resource-sets${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`)).items,
  createResourceSet: (input: { kind: APIResourceSet["kind"]; name: string; description: string; manifest: Array<Record<string, unknown>> }) => request<APIResourceSet>("/api/v1/resource-sets", { method: "POST", body: JSON.stringify(input) }),
  updateResourceSet: (setID: string, input: { name: string; description: string; state: APIResourceSet["state"]; manifest: Array<Record<string, unknown>>; revision: number }) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  duplicateResourceSet: (setID: string, name: string) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}/duplicate`, { method: "POST", body: JSON.stringify({ name }) }),
  attachResourceSet: (integrationID: string, resourceSetID: string, pinnedRevisionID = "") => request<APIIntegrationResourceLink>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets`, { method: "POST", body: JSON.stringify({ resource_set_id: resourceSetID, pinned_revision_id: pinnedRevisionID }) }),
  detachResourceSet: (integrationID: string, resourceSetID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets/${encodeURIComponent(resourceSetID)}`, { method: "DELETE" }),
  products: async (organisationID: string) => (await request<{ items: APIProduct[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`)).items,
  createProduct: (organisationID: string, name: string, slug: string) => request<APIProduct>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`, { method: "POST", body: JSON.stringify({ name, slug }) }),
  updateProductSettings: (productID: string, description: string, revision: number) => request<APIProduct>(productPath(productID), { method: "PATCH", body: JSON.stringify({ description, revision }) }),
  rewriteProductDescription: (productID: string, draft: string) => request<{ description: string }>(`${productPath(productID)}/description/rewrite`, { method: "POST", body: JSON.stringify({ draft }) }),
  customerAccounts: (productID: string, startingAfter = "") => request<APICustomerAccountPage>(`${productPath(productID)}/customer-accounts?limit=200${startingAfter ? `&starting_after=${encodeURIComponent(startingAfter)}` : ""}`),
  updateCustomerAccount: (productID: string, accountID: string, state: APICustomerAccount["state"], revision: number) => request<APICustomerAccount>(`${productPath(productID)}/customer-accounts/${encodeURIComponent(accountID)}`, { method: "PATCH", body: JSON.stringify({ state, revision }) }),
  environments: async (productID: string) => (await request<{ items: APIEnvironment[] }>(`${productPath(productID)}/environments`)).items,
  createEnvironment: (productID: string, organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>(`${productPath(productID)}/environments`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  distribution: (productID: string) => request<Distribution>(`${productPath(productID)}/distribution`),
  identity: () => request<APIIdentity>("/api/v1/identity-provider"),
  saveIdentityDraft: (input: { provider: "oidc"; issuer: string; client_id: string; client_secret: string; scopes: string[]; audience: string; oauth_resource: string; customer_account_claim: string; installation_claim: string; authorization_api_origin: string; revision: number }) => request<APIIdentity>("/api/v1/identity-provider", { method: "PUT", body: JSON.stringify(input) }),
  beginIdentityTest: (revision: number) => request<APIIdentityTest>("/api/v1/identity-provider/tests", { method: "POST", body: JSON.stringify({ revision }) }),
  identityTest: (testID: string) => request<APIIdentityTest>(`/api/v1/identity-provider/tests/${encodeURIComponent(testID)}`),
  activateIdentity: (revision: number, testID: string) => request<APIIdentity>("/api/v1/identity-provider/activate", { method: "POST", body: JSON.stringify({ revision, test_id: testID }) }),
  disableIdentity: (revision: number) => request<APIIdentity>("/api/v1/identity-provider/disable", { method: "POST", body: JSON.stringify({ revision }) }),
  disconnectIdentity: (revision: number) => request<APIIdentity>("/api/v1/identity-provider", { method: "DELETE", body: JSON.stringify({ revision }) }),
  supportSubmissions: async () => (await request<{ items: APISupportSubmission[]; has_more: boolean }>("/api/v1/support-submissions?limit=200")).items,
  supportSubmission: (submissionID: string) => request<APISupportSubmission>(`/api/v1/support-submissions/${encodeURIComponent(submissionID)}`),
  auditEvents: async (organisationID: string) => (await request<{ items: APIAuditEvent[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/audit`)).items,
  aiConnections: async () => (await request<{ items: APIAIProviderConnection[] }>("/api/v1/ai/connections")).items,
  saveAIConnection: (input: { organisation_id: string; provider: APIAIProviderConnection["provider"]; endpoint: string; credential: string; enabled: boolean; is_backup: boolean; backup_models: Partial<Record<APIAIWorkloadProfile["workload"], string>>; revision: number }) => request<APIAIProviderConnection>("/api/v1/ai/connections", { method: "POST", body: JSON.stringify(input) }),
  testAIConnection: (connectionID: string) => request<APIAIProviderConnection>(`/api/v1/ai/connections/${encodeURIComponent(connectionID)}/test`, { method: "POST", body: JSON.stringify({}) }),
  aiProfiles: async (productID: string) => (await request<{ items: APIAIWorkloadProfile[] }>(`${productPath(productID)}/ai-profiles`)).items,
  saveAIProfile: (productID: string, workload: APIAIWorkloadProfile["workload"], input: { organisation_id: string; provider_connection_id: string; model: string; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean; revision: number }) => request<APIAIWorkloadProfile>(`${productPath(productID)}/ai-profiles/${encodeURIComponent(workload)}`, { method: "PUT", body: JSON.stringify(input) }),
  aiPrompts: async (productID: string) => (await request<{ items: APIAIWorkflowPrompt[] }>(`${productPath(productID)}/ai-prompts`)).items,
  saveAIPrompt: (productID: string, promptKey: APIAIWorkflowPrompt["key"], instructions: string, revision: number) => request<APIAIWorkflowPrompt>(`${productPath(productID)}/ai-prompts/${encodeURIComponent(promptKey)}`, { method: "PUT", body: JSON.stringify({ instructions, revision }) }),
  resetAIPrompt: (productID: string, promptKey: APIAIWorkflowPrompt["key"], revision: number) => request<APIAIWorkflowPrompt>(`${productPath(productID)}/ai-prompts/${encodeURIComponent(promptKey)}/reset`, { method: "POST", body: JSON.stringify({ revision }) }),
  analyses: async (productID: string) => (await request<{ items: APIIntegrationAnalysis[] }>(`${productPath(productID)}/analyses`)).items,
  analyseIntegration: (productID: string, integrationID?: string) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) }),
  generateRecipes: async (productID: string, analysisID: string, integrationID?: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}/recipes`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) })).items,
  recipes: async (productID: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/recipes`)).items,
  createRecipe: (productID: string, prompt: string, integrationID = "") => request<APIRecipe>(`${productPath(productID)}/recipes`, { method: "POST", body: JSON.stringify(integrationID ? { prompt, integration_id: integrationID } : { prompt }) }),
  recipe: (productID: string, recipeID: string) => request<{ recipe: APIRecipe; revisions: APIRecipeRevision[] }>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`),
  updateRecipe: (productID: string, recipeID: string, revision: number, currentRevisionID: string, referenceIDs: string[], visibility: APIVisibility) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`, { method: "PATCH", body: JSON.stringify({ revision, current_revision_id: currentRevisionID, reference_ids: referenceIDs, visibility }) }),
  reworkRecipe: (productID: string, recipeID: string, revision: number, currentRevisionID: string, instruction: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/rework`, { method: "POST", body: JSON.stringify({ revision, current_revision_id: currentRevisionID, instruction }) }),
  approveRecipe: (productID: string, recipeID: string, revision: number, currentRevisionID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/approve`, { method: "POST", body: JSON.stringify({ revision, current_revision_id: currentRevisionID }) }),
  publishRecipe: (productID: string, recipeID: string, revision: number, currentRevisionID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/publish`, { method: "POST", body: JSON.stringify({ revision, current_revision_id: currentRevisionID }) }),
  aiUsage: (productID: string, days = 30) => request<{ workloads: APIAIWorkloadUsage[]; providers: APIAIProviderUsage[] }>(`${productPath(productID)}/ai-usage?days=${days}`),
  sources: async (productID: string) => (await request<{ items: APISource[] }>(`${productPath(productID)}/sources`)).items,
  createSource: (productID: string, organisationID: string, name: string, kind: string, location: string) => request<APISource>(`${productPath(productID)}/sources`, { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, kind, location }) }),
  uploadSource: (productID: string, organisationID: string, name: string, file: File) => {
    const body = new FormData();
    body.append("organisation_id", organisationID);
    body.append("name", name);
    body.append("file", file, file.name);
    return request<APISource>(`${productPath(productID)}/sources/upload`, { method: "POST", body });
  },
  queueCrawl: (productID: string, sourceID: string) => request<APICrawlJob>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawl`, { method: "POST" }),
  crawlJobs: async (productID: string, sourceID: string) => (await request<{ items: APICrawlJob[] }>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/crawls`)).items,
  sourceReview: (productID: string, sourceID: string, crawlJobID = "") => request<APISourceReview>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/review${crawlJobID ? `?crawl_job_id=${encodeURIComponent(crawlJobID)}` : ""}`),
  sourcePublications: async (productID: string, sourceID: string) => (await request<{ items: APISourcePublication[] }>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/publications`)).items,
  publishSource: (productID: string, sourceID: string, input: { revision: number; crawl_job_id: string; document_ids: string[]; acknowledge_reviewed: boolean }) => request<APISourcePublishResult>(`${productPath(productID)}/sources/${encodeURIComponent(sourceID)}/publish`, { method: "POST", body: JSON.stringify(input) }),
  tools: async (productID: string) => (await request<{ items: APITool[] }>(`${productPath(productID)}/tools`)).items,
  nativePlugins: async () => (await request<{ items: APINativePlugin[] }>("/api/v1/native-plugins")).items,
  setNativePluginEnabled: (pluginID: string, enabled: boolean) => request<APINativePlugin>(`/api/v1/native-plugins/${encodeURIComponent(pluginID)}/state`, { method: "PATCH", body: JSON.stringify({ enabled }) }),
  tool: (productID: string, toolID: string) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}`),
  createTool: (productID: string, input: APIToolCreateInput) => request<APITool>(`${productPath(productID)}/tools`, { method: "POST", body: JSON.stringify(input) }),
  updateTool: (productID: string, toolID: string, input: APIToolUpdateInput) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}`, { method: "PUT", body: JSON.stringify(input) }),
  proposeToolDraft: (productID: string, input: APIToolBuilderProposalInput) => request<APIToolBuilderProposal>(`${productPath(productID)}/tool-builder/propose`, { method: "POST", body: JSON.stringify({ ...input, ...(input.history ? { history: boundedToolBuilderChatHistory(input.history) } : {}) }) }),
  importToolDraft: (productID: string, input: APIToolBuilderImportInput) => request<APIToolBuilderImportResult>(`${productPath(productID)}/tool-builder/import`, { method: "POST", body: JSON.stringify(input) }),
  validateToolDraft: (productID: string, input: APIToolBuilderValidationInput) => request<APIToolBuilderValidation>(`${productPath(productID)}/tool-builder/validate`, { method: "POST", body: JSON.stringify(input) }),
  analyseToolDraft: (productID: string, input: APIToolBuilderAnalysisInput) => request<APIToolBuilderAnalysis>(`${productPath(productID)}/tool-builder/analyse`, { method: "POST", body: JSON.stringify(input) }),
  cloneTool: (productID: string, toolID: string, revision: number, namespace: string, name: string, credential = "") => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/clone`, { method: "POST", body: JSON.stringify({ revision, namespace, name, ...(credential ? { credential } : {}) }) }),
  dryRunTool: (productID: string, toolID: string, args: Record<string, unknown>) => request<APIToolDryRun>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/dry-run`, { method: "POST", body: JSON.stringify({ arguments: args }) }),
  createToolTestConfirmation: (productID: string, toolID: string, input: APIToolTestConfirmationInput) => request<APIToolTestConfirmation>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-confirmations`, { method: "POST", body: JSON.stringify(input) }),
  runToolTest: (productID: string, toolID: string, input: APIToolTestRunInput) => request<APIToolTestRun>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-runs`, { method: "POST", body: JSON.stringify(input) }),
  analyseToolTestRun: (productID: string, toolID: string, runID: string, input: APIToolTestAnalysisInput) => request<APIToolTestAnalysis>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/test-runs/${encodeURIComponent(runID)}/analyse`, { method: "POST", body: JSON.stringify({ ...input, ...(input.history ? { history: boundedToolTestAnalysisHistory(input.history) } : {}) }) }),
  retireTool: (productID: string, toolID: string, revision: number) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/retire`, { method: "POST", body: JSON.stringify({ revision }) }),
  publishTool: (productID: string, toolID: string, revision: number) => request<APITool>(`${productPath(productID)}/tools/${encodeURIComponent(toolID)}/publish`, { method: "POST", body: JSON.stringify({ revision }) }),
  mcpConnections: async (productID: string) => (await request<{ items: APIMCPConnection[] }>(`${productPath(productID)}/mcp-connections`)).items,
  createMCPConnection: (productID: string, input: { organisation_id: string; name: string; namespace: string; endpoint: string; access_token: string; forward_user_identity: boolean }) => request<APIMCPConnection>(`${productPath(productID)}/mcp-connections`, { method: "POST", body: JSON.stringify(input) }),
  inspectMCPConnection: (productID: string, connectionID: string) => request<APIMCPCatalog>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/inspect`, { method: "POST" }),
  importMCPTools: (productID: string, connectionID: string, input: { tool_names: string[]; required_grants: string[]; confirmation_required: boolean; timeout_ms: number }) => request<APIMCPImportResult>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/import`, { method: "POST", body: JSON.stringify(input) }),
  mcpPreview: (productID: string, audience: APIMCPPreview["audience"], method: APIMCPPreview["method"], grants: string[] = []) => {
    const query = new URLSearchParams({ audience, method });
    for (const grant of grants) query.append("grant", grant);
    return request<APIMCPPreview>(`${productPath(productID)}/mcp-preview?${query.toString()}`);
  },
  setPublicMCP: (productID: string, enabled: boolean, revision: number, acknowledgePublic: boolean) => request<APIProduct>(`${productPath(productID)}/distribution`, {
    method: "PATCH",
    body: JSON.stringify({ public_mcp_enabled: enabled, revision, acknowledge_public: acknowledgePublic }),
  }),
  setSourceVisibility: (productID: string, id: string, visibility: APIVisibility, revision: number, acknowledgePublic: boolean) => request<APISource>(`${productPath(productID)}/sources/${encodeURIComponent(id)}/visibility`, {
    method: "PATCH",
    body: JSON.stringify({ visibility, revision, acknowledge_public: acknowledgePublic }),
  }),
};
