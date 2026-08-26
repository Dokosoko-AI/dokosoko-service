import type {
  APIAIProviderConnection,
  APIAIProviderUsage,
  APIAIWorkloadProfile,
  APIAIWorkloadUsage,
  APIAccessConnection,
  APIAccessCredential,
  APIAccessDefinition,
  APIAccessInstance,
  APIAnalytics,
  APIAuditEvent,
  APIAuthorizationPoint,
  APIBackendConnection,
  APIBackendConnectionCredential,
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
  APIIntegrationPackageBinding,
  APIIntegrationPreflight,
  APIIntegrationResourceLink,
  APIIntegrationRevision,
  APIIntegrationRun,
  APIIntegrationToolBinding,
  APILLMProfile,
  APIMCPCatalog,
  APIMCPConnection,
  APIMCPImportResult,
  APINativePlugin,
  APIOrganisation,
  APIPackageArtifact,
  APIPackageArtifactInput,
  APIPackageRelease,
  APIProduct,
  APIProductBuild,
  APIProductBuildInput,
  APIProductDefinition,
  APIProductInstallation,
  APIProductVersion,
  APIProductVersionDiff,
  APIProductVersionImpact,
  APIProductVersionPin,
  APIProductVersionPinHistory,
  APIRecipe,
  APIRecipePopularity,
  APIRecipeReference,
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
  APISupportRoute,
  APISupportSubmission,
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
  APIWidget,
  APIWidgetBootstrap,
  APIWidgetConfiguration,
  APIWidgetInput,
  APIWidgetProvisioning,
  APIWidgetRuntimeSession,
  APIWidgetSecret,
  APIWidgetSecretProvisioning,
  APIWidgetSession,
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
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

export type APIWidgetAgentSource = { kind: "recipe" | "documentation"; title: string; uri?: string; revision?: number; integration?: string };
export type APIWidgetAgentTrace = { intent: string; promptVersion: string; plannerVersion?: string; recipeCount: number; documentationCount: number; historyMessages: number; contextFacts: number; mcpSuggestionAllowed: boolean };

export async function streamWidgetMessage(sessionToken: string, message: string, onText: (text: string) => void, signal?: AbortSignal, onSource?: (source: APIWidgetAgentSource) => void, onTrace?: (trace: APIWidgetAgentTrace) => void): Promise<void> {
  const response = await fetch("/v1/widget-chat", {
    method: "POST",
    credentials: "omit",
    cache: "no-store",
    signal,
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${sessionToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ message }),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    const error = payload?.error ?? {};
    throw new APIError(response.status, error.code ?? "widget_request_failed", error.message ?? "The widget could not answer.", error.details);
  }
  if (!response.body) throw new APIError(502, "widget_stream_unavailable", "The widget response stream is unavailable.");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finished = false;
  const consume = (event: string) => {
    const data = event.split("\n").filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
    if (!data) return;
    if (data === "[DONE]") {
      finished = true;
      return;
    }
    try {
      const payload = JSON.parse(data) as { type?: unknown; text?: unknown; source?: unknown; trace?: unknown };
      if (payload.type === "source" && payload.source && typeof payload.source === "object") onSource?.(payload.source as APIWidgetAgentSource);
      else if (payload.type === "trace" && payload.trace && typeof payload.trace === "object") onTrace?.(payload.trace as APIWidgetAgentTrace);
      else if (typeof payload.text === "string") onText(payload.text);
    } catch {
      throw new APIError(502, "widget_stream_invalid", "The widget returned an invalid response stream.");
    }
  };

  while (!finished) {
    const { done, value } = await reader.read();
    buffer = (buffer + decoder.decode(value, { stream: !done })).replaceAll("\r\n", "\n");
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      consume(buffer.slice(0, boundary));
      buffer = buffer.slice(boundary + 2);
      if (finished) break;
      boundary = buffer.indexOf("\n\n");
    }
    if (done) {
      if (buffer.trim()) consume(buffer);
      break;
    }
  }
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
  createDeployment: (organisationID: string, name: string, slug: string) => request<APIDeployment>("/api/v1/deployment", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, default_release_policy: "latest" }) }),
  updateDeployment: (input: Partial<APIDeployment> & { revision: number }) => request<APIDeployment>("/api/v1/deployment", { method: "PATCH", body: JSON.stringify(input) }),
  deploymentEnvironments: async () => (await request<{ items: APIEnvironment[] }>("/api/v1/environments")).items,
  createDeploymentEnvironment: (organisationID: string, name: string, slug: string, isProduction: boolean) => request<APIEnvironment>("/api/v1/environments", { method: "POST", body: JSON.stringify({ organisation_id: organisationID, name, slug, is_production: isProduction }) }),
  integrations: async () => (await request<{ items: APIIntegration[] }>("/api/v1/integrations")).items,
  widgets: async () => (await request<{ items: APIWidget[] }>("/api/v1/widgets")).items,
  widget: (widgetID: string) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}`),
  createWidget: (input: APIWidgetInput) => request<APIWidgetProvisioning>("/api/v1/widgets", { method: "POST", body: JSON.stringify(input) }),
  updateWidget: (widgetID: string, input: APIWidgetInput & { revision: number }) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  activateWidget: (widgetID: string, revision: number) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/activate`, { method: "POST", body: JSON.stringify({ revision }) }),
  disableWidget: (widgetID: string, revision: number) => request<APIWidget>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/disable`, { method: "POST", body: JSON.stringify({ revision }) }),
  widgetSecrets: async (widgetID: string) => (await request<{ items: APIWidgetSecret[] }>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets`)).items,
  createWidgetSecret: (widgetID: string) => request<APIWidgetSecretProvisioning>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets`, { method: "POST" }),
  revokeWidgetSecret: (widgetID: string, secretID: string) => request<APIWidgetSecret>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/secrets/${encodeURIComponent(secretID)}`, { method: "DELETE" }),
  widgetSessions: async (widgetID: string) => (await request<{ items: APIWidgetSession[] }>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/sessions`)).items,
  revokeWidgetSession: (widgetID: string, sessionID: string) => request<APIWidgetSession>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/sessions/${encodeURIComponent(sessionID)}`, { method: "DELETE" }),
  widgetConfiguration: (widgetID: string) => request<APIWidgetConfiguration>(`/v1/widgets/${encodeURIComponent(widgetID)}/configuration`, { credentials: "omit" }),
  widgetPreviewBootstrap: (widgetID: string) => request<APIWidgetBootstrap>(`/api/v1/widgets/${encodeURIComponent(widgetID)}/preview-session`, { method: "POST" }),
  exchangeWidgetSession: (bootstrapToken: string, origin: string) => request<APIWidgetRuntimeSession>("/v1/widget-sessions/exchange", { method: "POST", credentials: "omit", body: JSON.stringify({ bootstrapToken, origin }) }),
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
  setIntegrationAccessConnections: (integrationID: string, accessConnectionIDs: string[]) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/access-connections`, { method: "PUT", body: JSON.stringify({ access_connection_ids: accessConnectionIDs }) }),
  setIntegrationSupportRoute: (integrationID: string, supportRouteID: string) => request<APIIntegration>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/support-route`, { method: "PUT", body: JSON.stringify({ support_route_id: supportRouteID }) }),
  integrationToolBindings: async (integrationID: string) => (await request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`)).items,
  setIntegrationToolBindings: (integrationID: string, tools: Array<{ tool_id: string; revision: number; authorization_point_id: string; authorization_point_revision: number }>) => request<{ items: APIIntegrationToolBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/tools`, { method: "PUT", body: JSON.stringify({ tools }) }),
  grantDefinitions: async () => (await request<{ items: APIGrantDefinition[] }>("/api/v1/grant-definitions")).items,
  createGrantDefinition: (input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"] }) => request<APIGrantDefinition>("/api/v1/grant-definitions", { method: "POST", body: JSON.stringify(input) }),
  updateGrantDefinition: (grantID: string, input: { key: string; display_name: string; description: string; risk: APIGrantDefinition["risk"]; state: APIGrantDefinition["state"]; revision: number }) => request<APIGrantDefinition>(`/api/v1/grant-definitions/${encodeURIComponent(grantID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  authorizationPoints: async (integrationID: string) => (await request<{ items: APIAuthorizationPoint[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`)).items,
  createAuthorizationPoint: (integrationID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"] }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points`, { method: "POST", body: JSON.stringify(input) }),
  updateAuthorizationPoint: (integrationID: string, pointID: string, input: { key: string; name: string; description: string; action_type: APIAuthorizationPoint["action_type"]; required_grants: string[]; confirmation_required: boolean; decision_ttl_seconds: number; state: APIAuthorizationPoint["state"]; revision: number }) => request<APIAuthorizationPoint>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/authorization-points/${encodeURIComponent(pointID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  integrationPackages: async (integrationID: string) => (await request<{ items: APIIntegrationPackageBinding[] }>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages`)).items,
  bindIntegrationPackage: (integrationID: string, packageReleaseID: string) => request<APIIntegrationPackageBinding>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages`, { method: "POST", body: JSON.stringify({ package_release_id: packageReleaseID }) }),
  replaceIntegrationPackage: (integrationID: string, artifactID: string, packageReleaseID: string) => request<APIIntegrationPackageBinding>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages/${encodeURIComponent(artifactID)}`, { method: "PUT", body: JSON.stringify({ package_release_id: packageReleaseID }) }),
  unbindIntegrationPackage: (integrationID: string, artifactID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/packages/${encodeURIComponent(artifactID)}`, { method: "DELETE" }),
  resourceSets: async (kind = "") => (await request<{ items: APIResourceSet[] }>(`/api/v1/resource-sets${kind ? `?kind=${encodeURIComponent(kind)}` : ""}`)).items,
  createResourceSet: (input: { kind: APIResourceSet["kind"]; name: string; description: string; manifest: Array<Record<string, unknown>> }) => request<APIResourceSet>("/api/v1/resource-sets", { method: "POST", body: JSON.stringify(input) }),
  updateResourceSet: (setID: string, input: { name: string; description: string; state: APIResourceSet["state"]; manifest: Array<Record<string, unknown>>; revision: number }) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  duplicateResourceSet: (setID: string, name: string) => request<APIResourceSet>(`/api/v1/resource-sets/${encodeURIComponent(setID)}/duplicate`, { method: "POST", body: JSON.stringify({ name }) }),
  attachResourceSet: (integrationID: string, resourceSetID: string, pinnedRevisionID = "") => request<APIIntegrationResourceLink>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets`, { method: "POST", body: JSON.stringify({ resource_set_id: resourceSetID, pinned_revision_id: pinnedRevisionID }) }),
  detachResourceSet: (integrationID: string, resourceSetID: string) => request<void>(`/api/v1/integrations/${encodeURIComponent(integrationID)}/resource-sets/${encodeURIComponent(resourceSetID)}`, { method: "DELETE" }),
  accessDefinitions: async () => (await request<{ items: APIAccessDefinition[] }>("/api/v1/access-definitions")).items,
  createAccessDefinition: (input: { service_key: string; name: string; instance_cardinality: APIAccessDefinition["instance_cardinality"]; instance_label_singular: string; instance_label_plural: string; credential_scope: APIAccessDefinition["credential_scope"]; management_auth_type: APIAccessDefinition["management_auth_type"]; api_resource_set_id?: string; operations: Record<string, unknown> }) => request<APIAccessDefinition>("/api/v1/access-definitions", { method: "POST", body: JSON.stringify(input) }),
  updateAccessDefinition: (definitionID: string, input: { name: string; instance_label_singular: string; instance_label_plural: string; api_resource_set_id?: string; operations: Record<string, unknown>; revision: number }) => request<APIAccessDefinition>(`/api/v1/access-definitions/${encodeURIComponent(definitionID)}`, { method: "PUT", body: JSON.stringify(input) }),
  accessConnections: async () => (await request<{ items: APIAccessConnection[] }>("/api/v1/access-connections")).items,
  createAccessConnection: (input: { access_definition_id: string; environment_id?: string; name: string; region?: string; base_url: string; management_secret?: string; config: Record<string, unknown>; integration_ids: string[] }) => request<APIAccessConnection>("/api/v1/access-connections", { method: "POST", body: JSON.stringify(input) }),
  accessInstances: async (connectionID: string) => (await request<{ items: APIAccessInstance[] }>(`/api/v1/access-connections/${encodeURIComponent(connectionID)}/instances`)).items,
  accessCredentials: async (connectionID: string) => (await request<{ items: APIAccessCredential[] }>(`/api/v1/access-connections/${encodeURIComponent(connectionID)}/credentials`)).items,
  backendConnections: async () => (await request<{ items: APIBackendConnection[] }>("/api/v1/backend-connections")).items,
  createBackendConnection: (input: { name: string; base_url: string; authentication_type?: "bearer"; credential?: string; state?: APIBackendConnection["state"] }) => request<APIBackendConnection>("/api/v1/backend-connections", { method: "POST", body: JSON.stringify(input) }),
  replaceBackendConnection: (connectionID: string, input: { name: string; base_url: string; authentication_type: "bearer"; state: APIBackendConnection["state"]; revision: number }) => request<APIBackendConnection>(`/api/v1/backend-connections/${encodeURIComponent(connectionID)}`, { method: "PUT", body: JSON.stringify(input) }),
  createBackendConnectionCredential: (connectionID: string, credential: string, revision: number) => request<APIBackendConnectionCredential>(`/api/v1/backend-connections/${encodeURIComponent(connectionID)}/credentials`, { method: "POST", body: JSON.stringify({ credential, revision }) }),
  supportRoutes: async () => (await request<{ items: APISupportRoute[] }>("/api/v1/support-routes")).items,
  createSupportRoute: (input: { name: string; is_default: boolean; bug_reports_enabled: boolean; feedback_enabled: boolean; backend_connection_id?: string; retention_days: number; state: APISupportRoute["state"]; integration_ids: string[] }) => request<APISupportRoute>("/api/v1/support-routes", { method: "POST", body: JSON.stringify(input) }),
  replaceSupportRoute: (routeID: string, input: { name: string; is_default: boolean; bug_reports_enabled: boolean; feedback_enabled: boolean; backend_connection_id?: string; retention_days: number; state: APISupportRoute["state"]; integration_ids: string[]; revision: number }) => request<APISupportRoute>(`/api/v1/support-routes/${encodeURIComponent(routeID)}`, { method: "PUT", body: JSON.stringify(input) }),
  products: async (organisationID: string) => (await request<{ items: APIProduct[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`)).items,
  createProduct: (organisationID: string, name: string, slug: string) => request<APIProduct>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/products`, { method: "POST", body: JSON.stringify({ name, slug }) }),
  updateProductSettings: (productID: string, description: string, defaultVersionPolicy: "latest" | "lts", requirePromotionApproval: boolean, revision: number) => request<APIProduct>(productPath(productID), { method: "PATCH", body: JSON.stringify({ description, default_version_policy: defaultVersionPolicy, require_promotion_approval: requirePromotionApproval, revision }) }),
  rewriteProductDescription: (productID: string, draft: string) => request<{ description: string }>(`${productPath(productID)}/description/rewrite`, { method: "POST", body: JSON.stringify({ draft }) }),
  productVersions: async (productID: string) => (await request<{ items: APIProductVersion[] }>(`${productPath(productID)}/versions`)).items,
  createProductVersion: (productID: string, input: { version: string; profile_id: string; is_latest: boolean; is_lts: boolean; release_stage: "preview" | "active"; rollout_percentage: number }) => request<APIProductVersion>(`${productPath(productID)}/versions`, { method: "POST", body: JSON.stringify(input) }),
  updateProductVersion: (productID: string, versionID: string, input: { is_latest: boolean; is_lts: boolean; deprecated: boolean; deprecation_message: string; replacement_version: string; sunset_at?: string; rollout_percentage: number; acknowledge_impact: boolean; revision: number }) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  productVersionImpact: (productID: string, versionID: string) => request<APIProductVersionImpact>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/impact`),
  productVersionDiff: (productID: string, versionID: string) => request<APIProductVersionDiff>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/diff`),
  reconcileProductVersion: (productID: string, versionID: string, revision: number) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/reconcile`, { method: "POST", body: JSON.stringify({ revision }) }),
  promoteProductVersion: (productID: string, versionID: string, action: "request" | "approve" | "reject", note: string, revision: number) => request<APIProductVersion>(`${productPath(productID)}/versions/${encodeURIComponent(versionID)}/promotion`, { method: "POST", body: JSON.stringify({ action, note, revision }) }),
  productVersionPins: async (productID: string) => (await request<{ items: APIProductVersionPin[] }>(`${productPath(productID)}/version-pins`)).items,
  saveProductVersionPin: (productID: string, input: { scope: "customer" | "environment" | "installation"; scope_id: string; customer_account_id?: string; product_version_id: string; reason: string; revision: number }) => request<APIProductVersionPin>(`${productPath(productID)}/version-pins`, { method: "POST", body: JSON.stringify(input) }),
  deleteProductVersionPin: (productID: string, pinID: string) => request<void>(`${productPath(productID)}/version-pins/${encodeURIComponent(pinID)}`, { method: "DELETE" }),
  productVersionPinHistory: async (productID: string) => (await request<{ items: APIProductVersionPinHistory[] }>(`${productPath(productID)}/version-pins/history`)).items,
  productInstallations: async (productID: string) => (await request<{ items: APIProductInstallation[] }>(`${productPath(productID)}/installations`)).items,
  saveProductInstallation: (productID: string, input: { id?: string; customer_account_id: string; environment_id: string; external_id: string; name: string; state: "active" | "paused"; revision: number }) => request<APIProductInstallation>(`${productPath(productID)}/installations`, { method: "POST", body: JSON.stringify(input) }),
  customerAccounts: (productID: string, startingAfter = "") => request<APICustomerAccountPage>(`${productPath(productID)}/customer-accounts?limit=200${startingAfter ? `&starting_after=${encodeURIComponent(startingAfter)}` : ""}`),
  updateCustomerAccount: (productID: string, accountID: string, state: APICustomerAccount["state"], revision: number) => request<APICustomerAccount>(`${productPath(productID)}/customer-accounts/${encodeURIComponent(accountID)}`, { method: "PATCH", body: JSON.stringify({ state, revision }) }),
  productDefinition: (productID: string) => request<APIProductDefinition>(`${productPath(productID)}/definition`),
  productBuilds: async (productID: string) => (await request<{ items: APIProductBuild[] }>(`${productPath(productID)}/product-builds`)).items,
  buildProduct: (productID: string, inputs: APIProductBuildInput[]) => request<APIProductBuild>(`${productPath(productID)}/product-builds`, { method: "POST", body: JSON.stringify({ inputs }) }),
  publishProductBuild: (productID: string, buildID: string) => request<APIProductDefinition>(`${productPath(productID)}/product-builds/${encodeURIComponent(buildID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
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
  createSupportDeliveryAttempt: (submissionID: string) => request<APISupportSubmission>(`/api/v1/support-submissions/${encodeURIComponent(submissionID)}/delivery-attempts`, { method: "POST" }),
  analytics: (productID: string, days = 30) => request<APIAnalytics>(`${productPath(productID)}/analytics?days=${days}`),
  integrationRuns: async (productID: string) => (await request<{ items: APIIntegrationRun[] }>(`${productPath(productID)}/integration-runs`)).items,
  startIntegrationRun: (productID: string, environmentID: string, requestedOutcome: string) => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs`, { method: "POST", body: JSON.stringify({ environment_id: environmentID, requested_outcome: requestedOutcome }) }),
  completeIntegrationRun: (productID: string, runID: string, reportedSuccess: boolean, validatedSuccess: boolean, failureCode = "") => request<APIIntegrationRun>(`${productPath(productID)}/integration-runs/${encodeURIComponent(runID)}/complete`, { method: "POST", body: JSON.stringify({ reported_success: reportedSuccess, validated_success: validatedSuccess, failure_code: failureCode }) }),
  auditEvents: async (organisationID: string) => (await request<{ items: APIAuditEvent[] }>(`/api/v1/organisations/${encodeURIComponent(organisationID)}/audit`)).items,
  llmProfiles: async (productID: string) => (await request<{ items: APILLMProfile[] }>(`${productPath(productID)}/llm-profiles`)).items,
  saveLLMProfile: (productID: string, input: { organisation_id: string; role: string; provider: string; endpoint: string; model: string; credential: string; embedding_dimensions: number; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean }) => request<APILLMProfile>(`${productPath(productID)}/llm-profiles`, { method: "PUT", body: JSON.stringify(input) }),
  aiConnections: async () => (await request<{ items: APIAIProviderConnection[] }>("/api/v1/ai/connections")).items,
  saveAIConnection: (input: { organisation_id: string; provider: APIAIProviderConnection["provider"]; endpoint: string; credential: string; enabled: boolean; is_backup: boolean; backup_models: Partial<Record<APIAIWorkloadProfile["workload"], string>>; revision: number }) => request<APIAIProviderConnection>("/api/v1/ai/connections", { method: "POST", body: JSON.stringify(input) }),
  testAIConnection: (connectionID: string) => request<APIAIProviderConnection>(`/api/v1/ai/connections/${encodeURIComponent(connectionID)}/test`, { method: "POST", body: JSON.stringify({}) }),
  aiProfiles: async (productID: string) => (await request<{ items: APIAIWorkloadProfile[] }>(`${productPath(productID)}/ai-profiles`)).items,
  saveAIProfile: (productID: string, workload: APIAIWorkloadProfile["workload"], input: { organisation_id: string; provider_connection_id: string; model: string; max_input_tokens: number; max_output_tokens: number; daily_token_budget: number; enabled: boolean; revision: number }) => request<APIAIWorkloadProfile>(`${productPath(productID)}/ai-profiles/${encodeURIComponent(workload)}`, { method: "PUT", body: JSON.stringify(input) }),
  analyses: async (productID: string) => (await request<{ items: APIIntegrationAnalysis[] }>(`${productPath(productID)}/analyses`)).items,
  analyseIntegration: (productID: string, integrationID?: string) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) }),
  answerAnalysis: (productID: string, analysisID: string, answers: Record<string, string>) => request<APIIntegrationAnalysis>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}`, { method: "PATCH", body: JSON.stringify({ answers }) }),
  generateRecipes: async (productID: string, analysisID: string, integrationID?: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/analyses/${encodeURIComponent(analysisID)}/recipes`, { method: "POST", body: JSON.stringify(integrationID ? { integration_id: integrationID } : {}) })).items,
  recipes: async (productID: string) => (await request<{ items: APIRecipe[] }>(`${productPath(productID)}/recipes`)).items,
  createRecipe: (productID: string, prompt: string, integrationID = "") => request<APIRecipe>(`${productPath(productID)}/recipes`, { method: "POST", body: JSON.stringify({ prompt, integration_id: integrationID }) }),
  recipe: (productID: string, recipeID: string) => request<{ recipe: APIRecipe; revisions: APIRecipeRevision[] }>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`),
  updateRecipe: (productID: string, recipeID: string, markdown: string, references: APIRecipeReference[], visibility: APIVisibility) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}`, { method: "PATCH", body: JSON.stringify({ markdown, references, visibility }) }),
  reworkRecipe: (productID: string, recipeID: string, instruction: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/rework`, { method: "POST", body: JSON.stringify({ instruction }) }),
  approveRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/approve`, { method: "POST", body: JSON.stringify({}) }),
  publishRecipe: (productID: string, recipeID: string) => request<APIRecipe>(`${productPath(productID)}/recipes/${encodeURIComponent(recipeID)}/publish`, { method: "POST", body: JSON.stringify({}) }),
  recipeAnalytics: async (productID: string, days = 30) => (await request<{ items: APIRecipePopularity[] }>(`${productPath(productID)}/recipe-analytics?days=${days}`)).items,
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
  packageArtifacts: async () => (await request<{ items: APIPackageArtifact[] }>("/api/v1/package-artifacts")).items,
  packageArtifact: (artifactID: string) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}`),
  createPackageArtifact: (input: APIPackageArtifactInput) => request<APIPackageArtifact>("/api/v1/package-artifacts", { method: "POST", body: JSON.stringify(input) }),
  updatePackageArtifact: (artifactID: string, input: APIPackageArtifactInput & { revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}`, { method: "PUT", body: JSON.stringify(input) }),
  packageReleases: async (artifactID: string) => (await request<{ items: APIPackageRelease[] }>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/releases`)).items,
  publishPackageRelease: (artifactID: string, input: { version: string; purl: string; install_command: string; digest: string; provenance_url?: string; sbom_url?: string; artifact_revision: number; acknowledge_public: boolean }) => request<{ artifact: APIPackageArtifact; release: APIPackageRelease }>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/publish`, { method: "POST", body: JSON.stringify(input) }),
  deprecatePackageArtifact: (artifactID: string, input: { replacement_package_artifact_id?: string; message: string; sunset_at?: string; revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/deprecate`, { method: "POST", body: JSON.stringify(input) }),
  retirePackageArtifact: (artifactID: string, input: { replacement_package_artifact_id?: string; message: string; revision: number }) => request<APIPackageArtifact>(`/api/v1/package-artifacts/${encodeURIComponent(artifactID)}/retire`, { method: "POST", body: JSON.stringify(input) }),
  mcpConnections: async (productID: string) => (await request<{ items: APIMCPConnection[] }>(`${productPath(productID)}/mcp-connections`)).items,
  createMCPConnection: (productID: string, input: { organisation_id: string; name: string; namespace: string; endpoint: string; auth_mode: APIMCPConnection["auth_mode"]; credential: string; oauth_client_id: string; oauth_client_secret: string; oauth_issuer: string; authorization_url: string; token_url: string; scopes: string[] }) => request<APIMCPConnection>(`${productPath(productID)}/mcp-connections`, { method: "POST", body: JSON.stringify(input) }),
  inspectMCPConnection: (productID: string, connectionID: string) => request<APIMCPCatalog>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/inspect`, { method: "POST" }),
  importMCPTools: (productID: string, connectionID: string, input: { tool_names: string[]; required_grants: string[]; confirmation_required: boolean; timeout_ms: number }) => request<APIMCPImportResult>(`${productPath(productID)}/mcp-connections/${encodeURIComponent(connectionID)}/import`, { method: "POST", body: JSON.stringify(input) }),
  setPublicMCP: (productID: string, enabled: boolean, revision: number, acknowledgePublic: boolean) => request<APIProduct>(`${productPath(productID)}/distribution`, {
    method: "PATCH",
    body: JSON.stringify({ public_mcp_enabled: enabled, revision, acknowledge_public: acknowledgePublic }),
  }),
  setSourceVisibility: (productID: string, id: string, visibility: APIVisibility, revision: number, acknowledgePublic: boolean) => request<APISource>(`${productPath(productID)}/sources/${encodeURIComponent(id)}/visibility`, {
    method: "PATCH",
    body: JSON.stringify({ visibility, revision, acknowledge_public: acknowledgePublic }),
  }),
};
