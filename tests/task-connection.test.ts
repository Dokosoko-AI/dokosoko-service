import assert from "node:assert/strict";
import test from "node:test";
import type { APIIntegration, APIIntegrationDetail, APIMCPPreview, APIRecipe } from "../app/lib/api";
import type { APIDeveloperAssetPublication, DeploymentDocumentationPublication } from "../app/lib/developer-assets-api";
import { publishedTaskForAPI, readTaskClientReport, reviewTaskConnection, taskPlanJSON, taskTextHash, type TaskConnectionTransport, TaskConnectionChangedError, TaskConnectionEvidenceError, TaskConnectionLimitError, TaskConnectionUnavailableError } from "../app/lib/task-connection";

function mapFixture(uri: string | undefined, root: string, pins: Record<string, unknown>, entries: Record<string, unknown>[]) {
  const path = uri?.split("?")[0];
  if (path !== `${root}/map-v2` && path !== `${root}/map-v2/index`) return undefined;
  const metadata = { ...pins, map_version: 2, publication_uri: root, index_uri: `${root}/map-v2/index`, evidence_count: entries.length };
  if (path.endsWith("/index")) {
    const query = new URL(uri!).searchParams;
    const selected = entries.filter((entry) => ["source_publication_kind", "source_publication_id", "source_entity_id", "content_hash"].every((key) => !query.has(key) || query.get(key) === entry[key]));
    return { text: "# Exact selected evidence index\n", _meta: { ...metadata, evidence_resources: selected, match_count: selected.length } };
  }
  return { text: "# Compact publication map\n", _meta: metadata };
}

function fixture() {
  const origin = { id: "api", deployment_id: "deployment", display_name: "Orders", version_key: "v1", visibility: "private" } as APIIntegration;
  const recipe = { dependencies: [], id: "recipe", product_id: "deployment", state: "published", visibility: "private", published_at: "2026-09-07T00:00:00Z", revision: 3, current_revision_id: "recipe-revision", stable_uri: "dokosoko://products/example/recipes/orders", title: "Read an order", outcome: "One order status is checked.", api_attachments: [{ integration_id: "api" }], current_revision: { id: "recipe-revision", recipe_id: "recipe", revision: 1, spec_version: 3, markdown: "# Read an order\n\nUse SDK 1.2.3.\n", api_bindings: [{ integration_id: "api", integration_revision_id: "api-revision", integration_manifest_hash: "api-hash" }] } } as unknown as APIRecipe;
  const detail = { integration: origin, revisions: [{ id: "api-revision", integration_id: "api", revision: 2, manifest_hash: "api-hash", state: "published", published_at: "2026-09-07T00:00:00Z" }] } as APIIntegrationDetail;
  const publication = { id: "publication", api_id: "api", api_revision_id: "api-revision", deployment_id: "deployment", snapshot_hash: "snapshot" } as APIDeveloperAssetPublication;
  const uri = "dokosoko://developer-assets/apis/api/publications/publication";
  const transport: TaskConnectionTransport = {
    documentationPublication: async () => { throw new Error("Unexpected global publication lookup"); }, recipe: async () => structuredClone(recipe), integration: async () => structuredClone(detail), publications: async () => [structuredClone(publication)],
    preview: async (method, readURI) => ({ audience: "private", method, generated_at: "2026-09-07T00:00:00Z", protocol_version: "2026-07-28", endpoint: "/mcp", authorization: { mode: "simulated", grants: [] }, request: { method, params: { uri: readURI } }, response: { result: method === "resources/list" ? { resources: [{ uri: recipe.stable_uri, _meta: { revision_id: recipe.current_revision_id } }] } : { contents: [{ uri: readURI, mimeType: "text/markdown", ...(mapFixture(readURI, uri, { api_id: "api", api_developer_asset_publication_id: "publication", api_snapshot_hash: "snapshot", search_index_generation_id: "api-generation" }, []) ?? { text: recipe.current_revision!.markdown, _meta: { revision_id: recipe.current_revision_id, integration_ids: ["api"] } }) }] } } } as APIMCPPreview),
  };
  const run = () => reviewTaskConnection(origin, structuredClone(recipe), "private", "https://example.test/mcp", transport);
  return { origin, recipe, detail, publication, transport, run, uri };
}

test("task review pins its published recipe and exact API revision even when the map is not initially listed", async () => {
  const state = fixture();
  const review = await state.run();
  assert.equal(review.plan.task.revision_id, "recipe-revision");
  assert.equal(review.publications[0].revision, 2);
  assert.deepEqual(review.plan.resources.map((value) => value.discover), [true, false]);
  assert.equal(review.plan.resources[0].text_sha256, await taskTextHash(state.recipe.current_revision!.markdown));
  assert.equal(review.planHash, await taskTextHash(taskPlanJSON(review.plan)));
  assert(publishedTaskForAPI(state.recipe, state.origin, "private"));
  assert(!publishedTaskForAPI(state.recipe, state.origin, "public"));
  assert(!publishedTaskForAPI({ ...state.recipe, needs_attention: true }, state.origin, "private"));
});

test("task review follows discovery pages before checking one exact task revision", async () => {
  const state = fixture(), preview = state.transport.preview, cursors: (string | undefined)[] = [];
  state.transport.preview = async (method, uri, cursor) => {
    const result = await preview(method, uri, cursor);
    if (method === "resources/list") {
      cursors.push(cursor);
      result.request.params = { cursor };
      result.response.result = cursor === undefined
        ? { resources: [{ uri: state.uri }], catalogRevision: 8, nextCursor: "next-page" }
        : { resources: [{ uri: state.recipe.stable_uri, _meta: { revision_id: state.recipe.current_revision_id } }], catalogRevision: 8 };
    }
    return result;
  };
  const review = await state.run();
  assert.deepEqual(cursors, [undefined, "next-page"]);
  assert.equal(review.plan.task.revision_id, state.recipe.current_revision_id);
});

test("task review rejects repeated cursors, duplicate resources, changed catalogs and foreign continuations", async () => {
  for (const failure of ["cursor", "duplicate", "revision", "scope", "invalid-cursor", "stale"] as const) {
    const state = fixture(), preview = state.transport.preview;
    state.transport.preview = async (method, uri, cursor) => {
      const result = await preview(method, uri, cursor);
      if (method === "resources/list") {
        result.request.params = { cursor: failure === "scope" && cursor ? "foreign" : cursor };
        result.response.result = cursor === undefined
          ? { resources: [{ uri: state.uri }], catalogRevision: 8, nextCursor: "next-page" }
          : { resources: [{ uri: failure === "duplicate" ? state.uri : state.recipe.stable_uri, _meta: { revision_id: state.recipe.current_revision_id } }], catalogRevision: failure === "revision" ? 9 : 8, ...(failure === "cursor" ? { nextCursor: "next-page" } : failure === "invalid-cursor" ? { nextCursor: 2 } : {}) };
        if (failure === "stale" && cursor) result.response = { error: { code: -32602 } };
      }
      return result;
    };
    await assert.rejects(state.run, TaskConnectionChangedError, failure);
  }
});

test("task review stops discovery at its page and total byte budgets", async () => {
  for (const failure of ["pages", "bytes"] as const) {
    const state = fixture(), preview = state.transport.preview;
    let calls = 0;
    state.transport.preview = async (method, uri, cursor) => {
      const result = await preview(method, uri, cursor);
      assert.equal(method, "resources/list");
      result.request.params = { cursor };
      calls++;
      result.response.result = { resources: [{ uri: `dokosoko://fixture/${calls}`, description: failure === "bytes" ? "a".repeat(2 * 1024 * 1024) : "" }], nextCursor: String(calls) };
      return result;
    };
    await assert.rejects(state.run, TaskConnectionLimitError);
    assert.equal(calls, failure === "pages" ? 64 : 2);
  }
});

test("task review rejects changed recipes, foreign scope, wrong pins and incomplete runtime responses", async () => {
  const mutations: ((value: ReturnType<typeof fixture>) => void)[] = [
    (s) => { s.transport.recipe = async () => ({ ...s.recipe, current_revision_id: "new" }); },
    (s) => { s.detail.integration = { ...s.origin, deployment_id: "foreign" }; },
    (s) => { s.detail.revisions[0].manifest_hash = "different"; },
    (s) => { s.detail.revisions[0].state = "draft"; },
    (s) => { s.publication.api_revision_id = "other"; },
    (s) => { s.transport.publications = async () => [s.publication, s.publication]; },
    (s) => { const read = s.transport.preview; s.transport.preview = async (...args) => ({ ...await read(...args), audience: "public" }); },
    (s) => { const read = s.transport.preview; s.transport.preview = async (...args) => ({ ...await read(...args), response: { error: { code: -32004 } } }); },
    (s) => { const read = s.transport.preview; s.transport.preview = async (...args) => { const value = await read(...args); if (args[0] === "resources/read") (value.response.result as { contents: { uri: string }[] }).contents[0].uri = "wrong"; return value; }; },
    (s) => { const read = s.transport.preview; s.transport.preview = async (...args) => { const value = await read(...args); if (args[1] === s.recipe.stable_uri) (value.response.result as { contents: { text: string }[] }).contents[0].text = "Different task text"; return value; }; },
  ];
  for (const mutate of mutations) { const state = fixture(); mutate(state); await assert.rejects(state.run, TaskConnectionChangedError); }
});

test("imported client reports must identify this exact reviewed plan and all successful resource checks", async () => {
  const review = await evidenceFixture().run();
  const report = { endpoint: review.plan.endpoint, protocol_version: "2026-07-28", evidence_origin: "acceptance_client_observation", client_name: "DokoSoko MCP acceptance client", client_version: "0.2.0", started_at: "2026-09-07T00:00:00Z", task: { selection: review.plan.task, plan_sha256: review.planHash, retrieval_status: "pass", implementation_status: "not_run" }, checks: [
    ...["server/discover", "resources/list", `resource expected: ${review.plan.task.resource_uri}`].map((name) => ({ name, required: true, status: "pass" })),
    ...review.plan.resources.map((value) => ({ name: `resources/read: ${value.uri}`, required: true, status: "pass", resource_uri: value.uri, content_sha256: value.text_sha256, content_bytes: 32, request_id: "request", response_request_id: "request" })),
  ] };
  assert.equal(readTaskClientReport(report, review).retrieval, "pass");
  assert.throws(() => readTaskClientReport({ ...report, task: { ...report.task, plan_sha256: "another" } }, review), TaskConnectionChangedError);
  assert.throws(() => readTaskClientReport({ ...report, checks: report.checks.slice(0, -1) }, review), TaskConnectionChangedError);
  assert.throws(() => readTaskClientReport({ ...report, checks: [...report.checks, report.checks[0]] }, review), TaskConnectionChangedError);
  assert.throws(() => readTaskClientReport({ ...report, task: { ...report.task, implementation_status: "pass" } }, review), TaskConnectionChangedError);
  assert.throws(() => readTaskClientReport({ ...report, checks: [], task: { ...report.task, retrieval_status: "fail" } }, review), TaskConnectionChangedError);
  assert.equal(readTaskClientReport({ ...report, checks: report.checks.map((value) => ({ ...value, status: "fail" })), task: { ...report.task, retrieval_status: "fail" } }, review).retrieval, "fail");
});

function evidenceFixture() {
  const state = fixture(), hash = `sha256:${"a".repeat(64)}`;
  const units = [
    { apiID: "api", source_publication_kind: "documentation_collection", source_publication_id: "docs-revision", source_entity_id: "section", kind: "developer_asset_documentation", knowledge_unit_id: "docs-unit", title: "Order status guide" },
    { apiID: "api", source_publication_kind: "contract", source_publication_id: "contract-revision", source_entity_id: "operation", kind: "product_contract_operation", knowledge_unit_id: "contract-unit", title: "Read an order" },
    { apiID: "api", source_publication_kind: "sdk", source_publication_id: "sdk-publication", source_entity_id: "sample", kind: "developer_asset_sdk", knowledge_unit_id: "sdk-unit", title: "TypeScript order example" },
    { apiID: "", source_publication_kind: "documentation_collection", source_publication_id: "global-revision", source_entity_id: "authentication", kind: "developer_asset_documentation", knowledge_unit_id: "global-unit", title: "Authentication guide" },
  ];
  state.publication.deployment_documentation_publication_id = "global-publication";
  const globalURI = "dokosoko://developer-assets/global-documentation/global-publication";
  const globalPublication = { id: "global-publication", deployment_id: "deployment", visibility: "public", revision: 2, snapshot_hash: hash, published_at: "2026-09-07T00:00:00Z", members: [{ documentation_collection_revision_id: "global-revision", content_hash: hash, visibility: "public", ordinal: 0 }] } as DeploymentDocumentationPublication;
  state.transport.documentationPublication = async () => structuredClone(globalPublication);
  state.recipe.dependencies = units.map((unit) => ({ kind: unit.kind, resource_id: unit.kind === "product_contract_operation" ? `${unit.kind}:api:${unit.source_publication_id}:${unit.source_entity_id}` : `developer_asset:${unit.apiID ? "api:api" : "global_documentation:"}:${unit.source_publication_kind}:${unit.source_publication_id}:${unit.source_entity_id}`, version: unit.kind === "product_contract_operation" ? "c".repeat(64) : `${unit.source_publication_id}@1@${hash}@${hash}@${hash}` }));
  const entries = units.map((unit) => ({ ...unit, content_hash: hash, uri: `${unit.apiID ? state.uri : globalURI}/evidence/${unit.knowledge_unit_id}` }));
  const reads: string[] = [], preview = state.transport.preview;
  state.transport.preview = async (...args) => {
    const value = await preview(...args);
    if (args[0] !== "resources/read") return value;
    reads.push(args[1]!);
    const contents = (value.response.result as { contents: { text: string; _meta: Record<string, unknown> }[] }).contents;
    const global = args[1]?.startsWith(`${globalURI}/map-v2`);
    const map = mapFixture(args[1], global ? globalURI : state.uri, { ...(global ? { global_documentation_publication_id: "global-publication", snapshot_hash: hash, revision: 2 } : { api_id: "api", api_developer_asset_publication_id: "publication", api_snapshot_hash: "snapshot" }), search_index_generation_id: global ? "global-generation" : "api-generation" }, entries.filter((entry) => Boolean(entry.apiID) !== global));
    if (map) {
      Object.assign(contents[0], map);
    } else {
      const unit = entries.find((entry) => entry.uri === args[1]);
      if (unit) { contents[0]._meta = { ...unit, ...(unit.apiID ? { api_id: unit.apiID } : {}), search_index_generation_id: unit.apiID ? "api-generation" : "global-generation" }; contents[0].text = `# ${unit.title}\n\nExact reviewed content.`; }
    }
    return value;
  };
  return { ...state, units, entries, reads, hash, globalURI, globalPublication };
}

test("task checks retrieve selected documentation, contract operations, SDK samples and pinned global evidence", async () => {
  const state = evidenceFixture();
  // An unrelated unit remains in the map but must never be read or planned.
  state.entries.push({ ...state.entries[0], knowledge_unit_id: "unrelated", source_entity_id: "unrelated", uri: `${state.uri}/evidence/unrelated` });
  const review = await state.run();
  assert.equal(review.plan.resources.length, 7);
  assert.equal(review.evidence.length, 5); // Exact global map plus four units.
  for (const entry of state.entries.slice(0, 4)) {
    const planned = review.plan.resources.find((resource) => resource.uri === entry.uri)!;
    assert.equal(planned.discover, false);
    assert.equal(planned.metadata!.source_entity_id, entry.source_entity_id);
    assert.equal(planned.metadata!.content_hash, state.hash);
    assert.equal(planned.text_sha256, await taskTextHash(`# ${entry.title}\n\nExact reviewed content.`));
    assert.equal(state.reads.filter((uri) => uri === entry.uri).length, 1);
  }
  assert(!state.reads.some((uri) => uri.endsWith("/unrelated")));
  assert(review.plan.resources.some((resource) => resource.uri === `${state.globalURI}/map-v2`));
});

test("task evidence selection rejects absent, ambiguous, changed and foreign publication units", async () => {
  const mutations: ((state: ReturnType<typeof evidenceFixture>) => void)[] = [
    (s) => { s.entries.splice(0, 1); },
    (s) => { s.entries.push({ ...s.entries[0], knowledge_unit_id: "duplicate", uri: `${s.uri}/evidence/duplicate` }); },
    (s) => { s.entries[0].content_hash = `sha256:${"b".repeat(64)}`; },
    (s) => { s.entries[0].uri = "https://attacker.example/evidence"; },
    (s) => { s.entries[0].uri = `${s.uri}/evidence/../foreign`; s.entries[0].knowledge_unit_id = "../foreign"; },
    (s) => { s.recipe.dependencies![0].resource_id = s.recipe.dependencies![0].resource_id.replace(":api:api:", ":api:other:"); },
    (s) => { s.publication.deployment_documentation_publication_id = undefined; },
    (s) => { s.recipe.dependencies![0].version = "arbitrary-latest"; },
    (s) => { s.entries.push(s.entries[0]); },
  ];
  for (const mutate of mutations) { const state = evidenceFixture(); mutate(state); await assert.rejects(state.run, TaskConnectionEvidenceError); }
  const state = evidenceFixture(), original = state.transport.preview;
  state.transport.preview = async (...args) => {
    const value = await original(...args);
    if (args[1] === state.entries[0].uri) (value.response.result as { contents: { _meta: Record<string, unknown> }[] }).contents[0]._meta.search_index_generation_id = "different-generation";
    return value;
  };
  await assert.rejects(state.run, TaskConnectionChangedError);
});

test("public task checks use anonymous previews and exact public evidence", async () => {
  const state = evidenceFixture();
  state.origin.visibility = "public"; state.recipe.visibility = "public";
  const original = state.transport.preview;
  state.transport.preview = async (...args) => ({ ...await original(...args), audience: "public", endpoint: "/mcp/public", authorization: { mode: "anonymous", grants: [] } });
  const review = await reviewTaskConnection(state.origin, state.recipe, "public", "https://example.test/mcp/public", state.transport);
  assert.equal(review.audience, "public");
  assert.equal(review.plan.resources.length, 7);
  state.detail.integration = { ...state.origin, visibility: "private" };
  await assert.rejects(() => reviewTaskConnection(state.origin, state.recipe, "public", "https://example.test/mcp/public", state.transport), TaskConnectionChangedError);
});


test("task checks stop at the resource budget instead of exporting partial evidence", async () => {
  const state = evidenceFixture();
  for (let index = 0; index < 30; index++) {
    const entity = `extra-${index}`;
    state.entries.push({ ...state.entries[0], source_entity_id: entity, knowledge_unit_id: entity, uri: `${state.uri}/evidence/${entity}` });
    state.recipe.dependencies!.push({ ...state.recipe.dependencies![0], resource_id: `developer_asset:api:api:documentation_collection:docs-revision:${entity}` });
  }
  await assert.rejects(state.run, TaskConnectionLimitError);
  assert.equal(state.reads.filter((uri) => !uri.includes("/map-v2/index?")).length, 32);
  assert.equal(state.reads.filter((uri) => uri.includes("/map-v2/index?")).length, 29);
});

function mixedGlobalFixture() {
  const state = evidenceFixture();
  const otherAPI = { ...state.origin, id: "api-two", display_name: "Returns" };
  const otherPublication = { ...state.publication, id: "publication-two", api_id: otherAPI.id, api_revision_id: "revision-two", deployment_documentation_publication_id: "a-other-global" };
  const otherURI = "dokosoko://developer-assets/apis/api-two/publications/publication-two";
  state.recipe.api_attachments!.push({ integration_id: otherAPI.id });
  state.recipe.current_revision!.api_bindings!.push({ integration_id: otherAPI.id, integration_revision_id: "revision-two", integration_manifest_hash: "hash-two" });
  const getAPI = state.transport.integration, getPublications = state.transport.publications, preview = state.transport.preview;
  state.transport.integration = async (id) => id === otherAPI.id ? { integration: otherAPI, revisions: [{ ...state.detail.revisions[0], id: "revision-two", integration_id: otherAPI.id, manifest_hash: "hash-two" }] } as APIIntegrationDetail : getAPI(id);
  state.transport.publications = async (id) => id === otherAPI.id ? [otherPublication] : getPublications(id);
  const otherGlobal = { ...state.globalPublication, id: "a-other-global", visibility: "private", members: [{ ...state.globalPublication.members[0], documentation_collection_revision_id: "unrelated-revision", visibility: "private" }] } as DeploymentDocumentationPublication;
  const globals = [otherGlobal, state.globalPublication];
  state.transport.documentationPublication = async (id) => { const value = globals.find((publication) => publication.id === id); assert(value); return structuredClone(value); };
  state.transport.preview = async (...args) => {
    const response = await preview(...args);
    if (args[0] !== "resources/read") return response;
    const content = (response.response.result as { contents: { uri: string; text: string; _meta: Record<string, unknown> }[] }).contents[0];
    if (args[1] === state.recipe.stable_uri) content._meta.integration_ids = ["api", "api-two"];
    const map = mapFixture(args[1], otherURI, { api_id: otherAPI.id, api_developer_asset_publication_id: otherPublication.id, api_snapshot_hash: otherPublication.snapshot_hash, search_index_generation_id: "other-api-generation" }, []);
    if (map) Object.assign(content, map);
    for (const publication of globals) {
      const uri = `dokosoko://developer-assets/global-documentation/${publication.id}`, generation = `${publication.id}-generation`;
      const entries = state.entries.filter((entry) => !entry.apiID && publication.members.some((member) => member.documentation_collection_revision_id === entry.source_publication_id)).map((entry) => ({ ...entry, uri: `${uri}/evidence/${entry.knowledge_unit_id}` }));
      const map = mapFixture(args[1], uri, { global_documentation_publication_id: publication.id, snapshot_hash: publication.snapshot_hash, revision: publication.revision, search_index_generation_id: generation }, entries);
      if (map) Object.assign(content, map);
      const entry = entries.find((entry) => entry.uri === args[1]);
      if (entry) { content.text = `# ${entry.title}\n\nExact reviewed content.`; content._meta = { ...entry, search_index_generation_id: generation }; }
    }
    return response;
  };
  return { ...state, otherAPI, otherPublication, otherGlobal, globals };
}

test("mixed API task reads only global publications containing its exact selected revisions", async () => {
  const state = mixedGlobalFixture();
  const review = await state.run();
  assert.equal(review.publications.length, 2);
  assert(review.plan.resources.some((resource) => resource.uri === `${state.globalURI}/map-v2`));
  assert(!state.reads.some((uri) => uri.includes("a-other-global")));
  // A same-named nested revision with another snapshot hash is not equivalent.
  state.otherGlobal.members = [{ ...state.globalPublication.members[0], content_hash: `sha256:${"b".repeat(64)}` }];
  state.reads.length = 0;
  await state.run();
  assert(!state.reads.some((uri) => uri.includes("a-other-global")));
});

test("equivalent global wrappers use stable order and public checks ignore an unused private wrapper", async () => {
  const state = mixedGlobalFixture();
  state.otherGlobal.members = structuredClone(state.globalPublication.members);
  const privateReview = await state.run();
  assert(privateReview.plan.resources.some((resource) => resource.uri === "dokosoko://developer-assets/global-documentation/a-other-global/map-v2"));
  assert(!privateReview.plan.resources.some((resource) => resource.uri === `${state.globalURI}/map-v2`));
  state.origin.visibility = "public"; state.otherAPI.visibility = "public"; state.recipe.visibility = "public";
  const preview = state.transport.preview;
  state.transport.preview = async (...args) => ({ ...await preview(...args), audience: "public", endpoint: "/mcp/public", authorization: { mode: "anonymous", grants: [] } });
  state.reads.length = 0;
  const publicReview = await reviewTaskConnection(state.origin, state.recipe, "public", "https://example.test/mcp/public", state.transport);
  assert(publicReview.plan.resources.some((resource) => resource.uri === `${state.globalURI}/map-v2`));
  assert(!state.reads.some((uri) => uri.includes("a-other-global")));
});

test("mixed global requirements retain distinct exact publications and reject missing or foreign membership", async () => {
  const state = mixedGlobalFixture();
  state.entries.push({ ...state.entries[3], source_publication_id: "unrelated-revision", source_entity_id: "second-section", knowledge_unit_id: "second-global-unit", uri: "unused-by-fixture" });
  state.recipe.dependencies!.push({ kind: "developer_asset_documentation", resource_id: "developer_asset:global_documentation::documentation_collection:unrelated-revision:second-section", version: `unrelated-revision@1@${state.hash}@${state.hash}@${state.hash}` });
  const review = await state.run();
  assert(review.plan.resources.some((resource) => resource.uri.endsWith("/a-other-global/evidence/second-global-unit")));
  assert(review.plan.resources.some((resource) => resource.uri === `${state.globalURI}/map-v2`));
  state.otherGlobal.members = [];
  await assert.rejects(state.run, TaskConnectionEvidenceError);
  state.otherGlobal.deployment_id = "foreign";
  await assert.rejects(state.run, TaskConnectionEvidenceError);
});


test("runtime failures are reported as unavailable rather than a changed selection", async () => {
  const state = fixture(), original = state.transport.preview;
  state.transport.preview = async (...args) => ({ ...await original(...args), response: { error: { code: -32603, message: "Developer-asset resource could not be resolved" } } });
  await assert.rejects(state.run, TaskConnectionUnavailableError);
});


test("task review resolves selected references without downloading a large publication index", async () => {
  const state = evidenceFixture();
  for (let index = 0; index < 9000; index++) {
    state.entries.push({ ...state.entries[0], source_entity_id: `unrelated-${index}`, knowledge_unit_id: `unrelated-${index}`, uri: `${state.uri}/evidence/unrelated-${index}` });
  }
  const review = await state.run();
  assert.equal(review.plan.resources.length, 7);
  assert.equal(state.reads.length, 11);
  assert(!state.reads.includes(state.uri));
  const lookups = state.reads.filter((uri) => uri.includes("/map-v2/index?"));
  assert.equal(lookups.length, 4);
  assert(lookups.every((uri) => new URL(uri).searchParams.has("source_entity_id")));
  assert(!state.reads.some((uri) => uri.includes("unrelated-")));
});

test("task review rejects incomplete or inconsistent targeted index responses", async () => {
  for (const failure of ["next", "count", "generation", "root", "wide", "filter"] as const) {
    const state = evidenceFixture(), preview = state.transport.preview;
    state.transport.preview = async (...args) => {
      const response = await preview(...args);
      if (!args[1]?.includes("/map-v2/index?")) return response;
      const content = (response.response.result as { contents: { _meta: Record<string, unknown> }[] }).contents[0];
      if (failure === "next") content._meta.next_uri = args[1];
      if (failure === "count") content._meta.match_count = 2;
      if (failure === "generation") content._meta.search_index_generation_id = "different-generation";
      if (failure === "root") content._meta.publication_uri = "foreign";
      if (failure === "wide") content._meta.evidence_resources = [state.entries[0], state.entries[1]];
      if (failure === "filter") content._meta.evidence_resources = [{ ...state.entries[0], source_entity_id: "unrequested" }];
      return response;
    };
    await assert.rejects(state.run, failure === "generation" || failure === "root" ? TaskConnectionChangedError : TaskConnectionEvidenceError);
  }
});
