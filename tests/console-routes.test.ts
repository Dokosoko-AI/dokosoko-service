import assert from "node:assert/strict";
import test from "node:test";

import {
  IDENTITY_TABS,
  INTEGRATION_TABS,
  INTEGRATION_PRIMARY_TABS,
  INTEGRATION_RESOURCE_TABS,
  SETTINGS_TABS,
  SECTION_PATHS,
  entityPath,
  identityPath,
  integrationPath,
  integrationToolBuilderPath,
  integrationValidationPath,
  parseConsolePath,
  routeForEntity,
  routeForIntegration,
  sectionPath,
  settingsPath,
  toolBuilderPath,
} from "../app/lib/console-routes";
import { TOOL_TEST_ANALYSIS_CHAT_LIMITS, boundedToolTestAnalysisHistory, toolBuilderFollowUpDraft, toolTestAnalysisEvidenceHash } from "../app/lib/api";
import type { APITool, APIToolBuilderDraft } from "../app/lib/api";
import { partitionIntegrationTools, toolCanAttachToIntegration } from "../app/components/integrations/tool-scope";
import { toolCredentialBinding, toolCredentialBindingMatches, versionedResponseIsCurrent } from "../app/lib/tool-builder-safety";
import { apiToolHTTPPath, apiToolHTTPPathProblem, apiToolPersistenceContext, lockAPIToolBuilderDraft } from "../app/lib/tool-builder-runtime-context";

function followUpTestDraft(overrides: Partial<APIToolBuilderDraft> = {}): APIToolBuilderDraft {
  return {
    namespace: "projects",
    name: "get_project",
    description: "Get one project.",
    http_method: "GET",
    endpoint: "https://api.example.test/v1/projects/{project_id}",
    timeout_ms: 10_000,
    input_schema: { type: "object", additionalProperties: false, properties: { project_id: { type: "string" } }, required: ["project_id"] },
    output_schema: { type: "object", additionalProperties: false, properties: { id: { type: "string" } }, required: ["id"] },
    upstream_auth: { type: "none" },
    request_mapping: { parameter_locations: { project_id: "path" } },
    response_mapping: {},
    authorization_policy: { required_grants: [], confirmation_required: false, risk: "low", idempotency_required: false },
    credential_present: false,
    ...overrides,
  };
}

test("live-test analysis trusts only the server-computed evidence binding", async () => {
  const serverHash = `sha256:${"0123456789abcdef".repeat(4)}`;
  assert.equal(await toolTestAnalysisEvidenceHash({ evidence_hash: serverHash } as never), serverHash);
  await assert.rejects(toolTestAnalysisEvidenceHash({ evidence_hash: "sha256:not-canonical" } as never), /valid evidence preview hash/);
});

test("live-test analysis keeps only one bounded chronological history suffix", () => {
  const history = Array.from({ length: 14 }, (_, index) => ({ role: index % 2 === 0 ? "user" as const : "assistant" as const, content: `turn ${index}` }));
  assert.deepEqual(boundedToolTestAnalysisHistory(history).map((message) => message.content), history.slice(-TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessages).map((message) => message.content));
  const oversized = "🙂".repeat(TOOL_TEST_ANALYSIS_CHAT_LIMITS.maxMessageBytes / 2 + 1);
  assert.deepEqual(boundedToolTestAnalysisHistory([{ role: "user", content: oversized }]), []);
  assert.deepEqual(
    boundedToolTestAnalysisHistory([
      { role: "user", content: "must not be resurrected" },
      { role: "assistant", content: oversized },
      { role: "user", content: "new contiguous turn" },
    ]).map((message) => message.content),
    ["new contiguous turn"],
  );
});

test("tool-builder follow-ups refine the pending complete candidate without reviving rejected fields", () => {
  const current = followUpTestDraft({ credential_present: true });
  const pending = followUpTestDraft({
    namespace: "suggested_namespace",
    name: "suggested_name",
    description: "Get one project with its readiness state.",
    endpoint: "https://api.example.test/v2/projects/{project_id}",
    upstream_auth: { type: "bearer" },
    credential_present: false,
  });
  const followUp = toolBuilderFollowUpDraft(current, pending, { endpoint: "rejected" }, false, true);

  assert.equal(followUp.description, pending.description, "undecided proposal fields should remain available to the next AI turn");
  assert.deepEqual(followUp.upstream_auth, pending.upstream_auth);
  assert.equal(followUp.endpoint, current.endpoint, "a field explicitly kept unchanged must not be revived");
  assert.equal(followUp.namespace, current.namespace, "existing tool identity remains immutable");
  assert.equal(followUp.name, current.name, "existing tool identity remains immutable");
  assert.equal(followUp.credential_present, current.credential_present, "only the non-secret current presence signal is retained");
  assert.equal(toolBuilderFollowUpDraft(current, pending, {}, true), current, "stale proposals must never become follow-up context");
});

test("browser-held tool credentials remain bound to one origin and exact auth configuration", () => {
  const auth = { type: "api_key_header" as const, header_name: "X-API-Key", prefix: "Token" };
  const binding = toolCredentialBinding("https://api.example.test/v1/projects", auth);
  assert.equal(toolCredentialBindingMatches(binding, "https://api.example.test/v2/projects", auth), true, "path-only edits keep an origin credential usable");
  assert.equal(toolCredentialBindingMatches(binding, "https://other.example.test/v1/projects", auth), false, "a different destination origin requires re-entry");
  assert.equal(toolCredentialBindingMatches(binding, "https://api.example.test/v1/projects", { ...auth, header_name: "X-Other-Key" }), false, "authentication configuration changes require re-entry");
  assert.equal(versionedResponseIsCurrent(7, 7), true);
  assert.equal(versionedResponseIsCurrent(7, 8), false);
});

test("primary console sections have canonical, round-trippable URLs", () => {
  for (const section of ["product", "sources", "documents", "contracts", "sdks", "query-lab", "identity", "recipes", "tools", "connections", "mcp-preview", "distribution", "reporting", "settings"] as const) {
    const path = SECTION_PATHS[section];
    assert.equal(sectionPath(section as keyof typeof SECTION_PATHS), path);
    assert.deepEqual(parseConsolePath(path), { kind: "section", section, path });
    assert.deepEqual(parseConsolePath(`${path}/`), { kind: "section", section, path });
  }
  assert.equal(sectionPath("identity"), "/identity");
  assert.deepEqual(parseConsolePath("/identity/"), { kind: "section", section: "identity", path: "/identity" });
  assert.deepEqual(IDENTITY_TABS, [
    { id: "sign-in", label: "navigation.customerSignIn" },
    { id: "customer-accounts", label: "navigation.customerAccounts" },
  ]);
  assert.equal(identityPath(), "/identity");
  assert.equal(identityPath("customer-accounts"), "/identity/customer-accounts");
  assert.deepEqual(parseConsolePath("/identity/customer-accounts"), { kind: "section", section: "identity", identityTab: "customer-accounts", path: "/identity/customer-accounts" });
  assert.deepEqual(parseConsolePath("/identity/customer-accounts/"), { kind: "section", section: "identity", identityTab: "customer-accounts", path: "/identity/customer-accounts" });
  assert.equal(parseConsolePath("/identity/sign-in").kind, "not-found");
  assert.equal(parseConsolePath("/identity/auth0").kind, "not-found");
  assert.equal(parseConsolePath("/identity/authorization").kind, "not-found");
  assert.equal(parseConsolePath("/developer-assets/documentation/collections").kind, "not-found");
});

test("tools have canonical catalog, connection, preview, and detail URLs", () => {
  assert.equal(sectionPath("tools"), "/tools");
  assert.deepEqual(parseConsolePath("/tools"), {
    kind: "section",
    section: "tools",
    path: "/tools",
  });
  assert.equal(sectionPath("connections"), "/tools/connections");
  assert.deepEqual(parseConsolePath("/tools/connections"), {
    kind: "section",
    section: "connections",
    path: "/tools/connections",
  });
  assert.equal(sectionPath("mcp-preview"), "/tools/preview");
  assert.deepEqual(parseConsolePath("/tools/preview"), {
    kind: "section",
    section: "mcp-preview",
    path: "/tools/preview",
  });

  const uid = "billing tool/v2";
  assert.equal(entityPath("tool", uid), "/tool/billing%20tool%2Fv2");
  assert.deepEqual(parseConsolePath(entityPath("tool", uid)), routeForEntity("tool", uid));
  assert.equal(routeForEntity("tool", uid).section, "tools");
  assert.equal(routeForEntity("connection", "mcp_vendor").section, "connections");
});

test("custom tool builder has canonical create and edit URLs", () => {
  assert.equal(toolBuilderPath(), "/tools/new");
  assert.deepEqual(parseConsolePath("/tools/new"), { kind: "tool-builder", section: "tools", path: "/tools/new" });
  assert.equal(toolBuilderPath("tool / 42"), "/tools/new/tool%20%2F%2042");
  assert.deepEqual(parseConsolePath("/tools/new/tool%20%2F%2042"), { kind: "tool-builder", section: "tools", uid: "tool / 42", path: "/tools/new/tool%20%2F%2042" });
});

test("API-owned tool builder has an ID-based canonical route", () => {
  const integrationID = "voice api/v3";
  const path = integrationToolBuilderPath(integrationID);
  assert.equal(path, "/integration/voice%20api%2Fv3/tools/new");
  assert.deepEqual(parseConsolePath(path), { kind: "tool-builder", section: "tools", integrationID, path });
  assert.deepEqual(parseConsolePath(`${path}/`), parseConsolePath(path));
});

test("API-owned assistance keeps the human-selected runtime boundary immutable", () => {
  const lock = {
    ownerIntegrationID: "voice-api",
    runtimeServiceConnectionID: "runtime-voice",
    baseURL: "https://voice.example.test/v1/",
    upstreamAuth: { type: "api_key_header" as const, header_name: "X-Voice-Key" },
    credentialPresent: true,
  };
  const candidate = followUpTestDraft({
    endpoint: "https://attacker.invalid/v2/voices/{voice_id}",
    upstream_auth: { type: "none" },
    credential_present: false,
  });
  const locked = lockAPIToolBuilderDraft(candidate, lock, apiToolHTTPPath(candidate.endpoint));

  assert.equal(locked.endpoint, "https://voice.example.test/v2/voices/%7Bvoice_id%7D", "a proposal may change only the relative path, never the selected host");
  assert.deepEqual(locked.upstream_auth, lock.upstreamAuth);
  assert.equal(locked.credential_present, true);
  assert.deepEqual(apiToolPersistenceContext(lock, "/v2/voices/{voice_id}"), {
    scope: "api",
    owner_integration_id: "voice-api",
    runtime_service_connection_id: "runtime-voice",
    http_path: "/v2/voices/{voice_id}",
  });
  assert.equal(apiToolHTTPPathProblem("/v2/voices/{voice_id}"), "");
  assert.match(apiToolHTTPPathProblem("//other.example/v2"), /cannot contain a host/);
});

test("the root canonicalizes to APIs without legacy aliases", () => {
  assert.deepEqual(parseConsolePath("/"), {
    kind: "section",
    section: "product",
    path: "/integrations",
  });
  assert.equal(parseConsolePath("/overview").kind, "not-found");
  assert.equal(parseConsolePath("/distribution").kind, "not-found");
  assert.equal(parseConsolePath("/operations").kind, "not-found");
  assert.equal(parseConsolePath("/integrations/tools").kind, "not-found");
  assert.equal(parseConsolePath("/integrations/mcp").kind, "not-found");
});

test("entity URLs encode UIDs and resolve to their owning section", () => {
  const uid = "voice api/v3";
  assert.equal(entityPath("integration", uid), "/integration/voice%20api%2Fv3");
  assert.deepEqual(parseConsolePath(entityPath("integration", uid)), routeForEntity("integration", uid));
  assert.equal(routeForEntity("source", "src_docs").section, "sources");
  assert.equal(routeForEntity("report", "report_123").section, "reporting");
  assert.equal(routeForEntity("audit-event", "event_123").section, "reporting");
});

test("API workspaces expose the task-oriented setup tabs with stable, round-trippable URLs", () => {
  assert.deepEqual(INTEGRATION_TABS, [
    { id: "overview", label: "routes.quickStart" },
    { id: "documentation", label: "routes.resources" },
    { id: "authorization", label: "routes.keysAccess" },
    { id: "tools", label: "routes.tools" },
    { id: "test", label: "routes.test" },
    { id: "history", label: "routes.history" },
  ]);
  assert.deepEqual(INTEGRATION_PRIMARY_TABS, [
    { id: "overview", label: "routes.quickStart" },
    { id: "documentation", label: "routes.resources" },
    { id: "authorization", label: "routes.keysAccess" },
    { id: "tools", label: "routes.tools" },
    { id: "test", label: "routes.test" },
  ], "History remains routable but lives behind the API More menu");
  const uid = "voice api/v3";
  for (const tab of INTEGRATION_TABS) {
    const path = integrationPath(uid, tab.id);
    assert.equal(path, tab.id === "overview" ? "/integration/voice%20api%2Fv3" : `/integration/voice%20api%2Fv3/${tab.id}`);
    assert.deepEqual(parseConsolePath(path), routeForIntegration(uid, tab.id));
    assert.deepEqual(parseConsolePath(`${path}/`), routeForIntegration(uid, tab.id));
  }
});

test("API tools distinguish common definitions from API ownership without namespace inference", () => {
  const common = { id: "common", scope: "common" } as APITool;
  const legacyCommon = { id: "legacy" } as APITool;
  const voice = { id: "voice", scope: "api", owner_integration_id: "voice-api" } as APITool;
  const face = { id: "face", scope: "api", owner_integration_id: "face-api" } as APITool;
  const groups = partitionIntegrationTools([common, legacyCommon, voice, face], new Set(["common", "voice", "face"]), "voice-api");

  assert.deepEqual(groups.apiOwned.map((tool) => tool.id), ["voice"]);
  assert.deepEqual(groups.attachedCommon.map((tool) => tool.id), ["common"]);
  assert.deepEqual(groups.foreignAPI.map((tool) => tool.id), ["face"]);
  assert.equal(toolCanAttachToIntegration(legacyCommon, "voice-api"), true, "pre-ownership tools remain common for compatibility");
  assert.equal(toolCanAttachToIntegration(face, "voice-api"), false, "another API's tool must fail closed");
});

test("API resource workspaces have stable nested sub-tab URLs", () => {
  const uid = "voice api/v3";
  for (const tab of INTEGRATION_RESOURCE_TABS) {
    const path = integrationPath(uid, "documentation", tab.id);
    assert.equal(path, tab.id === "documentation" ? "/integration/voice%20api%2Fv3/documentation" : `/integration/voice%20api%2Fv3/documentation/${tab.id}`);
    assert.deepEqual(parseConsolePath(path), routeForIntegration(uid, "documentation", tab.id));
    assert.deepEqual(parseConsolePath(`${path}/`), routeForIntegration(uid, "documentation", tab.id));
  }
});

test("API validation findings open the matching local setup area", () => {
  const uid = "voice-api";
  assert.equal(integrationValidationPath(uid, "resources"), `/integration/${uid}/documentation`);
  assert.equal(integrationValidationPath(uid, "authorization"), `/integration/${uid}/authorization`);
  assert.equal(integrationValidationPath(uid, "access"), `/integration/${uid}`);
  assert.equal(integrationValidationPath(uid, "tools"), `/integration/${uid}/tools`);
  assert.equal(integrationValidationPath(uid, "recipes"), "/recipes");
  assert.equal(integrationValidationPath(uid, "delivery"), "/agent-access");
  assert.equal(integrationValidationPath(uid, "unknown"), `/integration/${uid}`);
});

test("settings has stable routes for every overview area", () => {
  assert.deepEqual(SETTINGS_TABS, [
    { id: "overview", label: "routes.overview" },
    { id: "tenant", label: "routes.tenantSettings" },
    { id: "ai", label: "routes.aiConfiguration" },
    { id: "root", label: "routes.rootAccess" },
  ]);
  assert.equal(settingsPath(), "/settings");
  for (const tab of SETTINGS_TABS.filter((candidate) => candidate.id !== "overview")) {
    const path = settingsPath(tab.id);
    assert.equal(path, `/settings/${tab.id}`);
    assert.deepEqual(parseConsolePath(path), { kind: "section", section: "settings", settingsTab: tab.id, path });
  }
  assert.deepEqual(parseConsolePath("/settings/ai/"), parseConsolePath("/settings/ai"));
  assert.deepEqual(parseConsolePath("/settings/tenant/"), parseConsolePath("/settings/tenant"));
  assert.equal(parseConsolePath("/settings/storage").kind, "not-found");
  assert.equal(parseConsolePath("/settings/identity").kind, "not-found");
  assert.equal(parseConsolePath("/settings/models").kind, "not-found");
});

test("recipes have one stable product-level workspace", () => {
  assert.equal(sectionPath("recipes"), "/recipes");
  assert.deepEqual(parseConsolePath("/recipes"), {
    kind: "section",
    section: "recipes",
    path: "/recipes",
  });
  assert.deepEqual(parseConsolePath("/recipes/"), parseConsolePath("/recipes"));
});

test("unknown API tabs do not create compatibility aliases", () => {
  const uid = "voice-api";
  for (const tab of ["access", "resources", "recipes", "delivery", "usage", "support", "revisions"]) {
    assert.equal(parseConsolePath(`/integration/${uid}/${tab}`).kind, "not-found");
  }
  assert.equal(parseConsolePath(`/integration/${uid}/tools/packages`).kind, "not-found");
  assert.equal(parseConsolePath(`/integration/${uid}/documentation/not-a-resource-tab`).kind, "not-found");
});

test("unknown and malformed paths render the console not-found route", () => {
  assert.deepEqual(parseConsolePath("/not-a-section"), {
    kind: "not-found",
    section: "product",
    path: "/not-a-section",
  });
  assert.equal(parseConsolePath("/integration/%E0%A4%A").kind, "not-found");
  assert.equal(parseConsolePath("/integration").kind, "not-found");
  assert.equal(parseConsolePath("/integration/voice-api/not-a-tab").kind, "not-found");
  assert.equal(parseConsolePath("/tools/catalog").kind, "not-found");
  assert.equal(parseConsolePath("/tools/not-a-tools-area").kind, "not-found");
  assert.equal(parseConsolePath("/insights").kind, "not-found");
  assert.equal(parseConsolePath("/insights/activity").kind, "not-found");
});
