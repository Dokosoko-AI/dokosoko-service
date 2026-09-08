import type { APIIntegration, APIIntegrationDetail, APIMCPPreview, APIRecipe } from "./api";
import type { APIDeveloperAssetPublication, DeploymentDocumentationPublication } from "./developer-assets-api";

export type TaskResourceExpectation = { uri: string; discover: boolean; text_sha256: string; mime_type?: string; metadata?: Record<string, unknown> };
export type TaskCheckPlan = {
  schema_version: "mcp-task-check-v1";
  endpoint: string;
  task: { title: string; outcome: string; resource_uri: string; revision_id: string };
  resources: TaskResourceExpectation[];
};
export type TaskConnectionReview = { plan: TaskCheckPlan; planHash: string; recipe: APIRecipe; reviewedAt: string; audience: "private" | "public"; publications: { api: APIIntegration; revision: number; uri: string; text: string }[]; evidence: { title: string; kind: string; globalRevision?: number; uri: string; text: string }[] };
export function taskEvidenceKind(kind: string) {
  const kinds = { map: "map", sdk_section: "guidance", documentation_section: "guidance", sdk_sample: "example", contract_example: "example", sdk_symbol: "symbol", contract_operation: "operation", contract_schema: "schema" } as const;
  return Object.hasOwn(kinds, kind) ? kinds[kind as keyof typeof kinds] : "evidence";
}
export class TaskConnectionChangedError extends Error { constructor() { super("task_connection_changed"); } }
export class TaskConnectionEvidenceError extends Error { constructor() { super("task_connection_evidence_unavailable"); } }
export class TaskConnectionUnavailableError extends Error { constructor() { super("task_connection_unavailable"); } }
export class TaskConnectionLimitError extends Error { constructor() { super("task_connection_limit"); } }
export type TaskConnectionTransport = {
  recipe: (id: string) => Promise<APIRecipe>;
  integration: (id: string) => Promise<APIIntegrationDetail>;
  publications: (apiID: string) => Promise<APIDeveloperAssetPublication[]>;
  documentationPublication: (id: string) => Promise<DeploymentDocumentationPublication>;
  preview: (method: "resources/list" | "resources/read", uri?: string, cursor?: string) => Promise<APIMCPPreview>;
};
function object(value: unknown): Record<string, unknown> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function changed(): never { throw new TaskConnectionChangedError(); }
export function taskAPIIDs(recipe: APIRecipe) { return (recipe.api_attachments?.length ? recipe.api_attachments.map((value) => value.integration_id) : [recipe.integration_id ?? ""]).filter(Boolean).sort(); }
export function publishedTaskForAPI(recipe: APIRecipe, api: APIIntegration, audience: "private" | "public") {
  return recipe.product_id === api.deployment_id && recipe.state === "published" && !recipe.needs_attention && !!recipe.published_at && !!recipe.stable_uri && !!recipe.current_revision_id && taskAPIIDs(recipe).includes(api.id) && (audience !== "public" || recipe.visibility === "public");
}
export async function taskTextHash(text: string) {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return `sha256:${Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}
function ordered(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(ordered);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, item]) => [key, ordered(item)]));
  return value;
}
// Match the standalone Go client's struct field order, sorted metadata keys and
// encoding/json escaping. The reviewed expectations use fixed ASCII meta keys.
export function taskPlanJSON(plan: TaskCheckPlan) {
  const value = { schema_version: plan.schema_version, endpoint: plan.endpoint, task: { title: plan.task.title, outcome: plan.task.outcome, resource_uri: plan.task.resource_uri, revision_id: plan.task.revision_id }, resources: plan.resources.map((resource) => ({ uri: resource.uri, discover: resource.discover, text_sha256: resource.text_sha256, ...(resource.mime_type ? { mime_type: resource.mime_type } : {}), ...(resource.metadata && Object.keys(resource.metadata).length ? { metadata: ordered(resource.metadata) } : {}) })) };
  return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, (value) => `\\u${value.charCodeAt(0).toString(16).padStart(4, "0")}`);
}
function previewResult(value: APIMCPPreview, audience: "private" | "public", method: "resources/list" | "resources/read", uri?: string, cursor?: string) {
  const params = object(value.request.params);
  if (value.audience !== audience || value.method !== method || value.protocol_version !== "2026-07-28" || value.endpoint !== (audience === "public" ? "/mcp/public" : "/mcp") || value.authorization.mode !== (audience === "public" ? "anonymous" : "simulated") || value.authorization.grants.length || value.request.method !== method || (uri !== undefined && params.uri !== uri) || params.cursor !== cursor) return changed();
  if (value.response.error) {
    if (object(value.response.error).code === -32603) throw new TaskConnectionUnavailableError();
    return changed();
  }
  return object(value.response.result);
}
async function listTaskResources(transport: TaskConnectionTransport, audience: "private" | "public") {
  const resources: Record<string, unknown>[] = [], cursors = new Set<string>(), uris = new Set<string>();
  let cursor: string | undefined, bytes = 0, revision: unknown;
  for (let page = 0; page < 64; page++) {
    const listed = previewResult(await transport.preview("resources/list", undefined, cursor), audience, "resources/list", undefined, cursor);
    bytes += new TextEncoder().encode(JSON.stringify(listed)).length;
    if (bytes > 4 * 1024 * 1024) throw new TaskConnectionLimitError();
    if (!Array.isArray(listed.resources)) return changed();
    if (page === 0) revision = listed.catalogRevision;
    else if (revision !== listed.catalogRevision) return changed();
    for (const value of listed.resources.map(object)) {
      if (typeof value.uri !== "string" || !value.uri || uris.has(value.uri)) return changed();
      uris.add(value.uri); resources.push(value);
    }
    if (listed.nextCursor === undefined || listed.nextCursor === null) return resources;
    if (typeof listed.nextCursor !== "string" || listed.nextCursor.length > 512 || cursors.has(listed.nextCursor)) return changed();
    cursors.add(listed.nextCursor); cursor = listed.nextCursor;
  }
  throw new TaskConnectionLimitError();
}
async function readResource(transport: TaskConnectionTransport, audience: "private" | "public", uri: string, metadata: Record<string, unknown>, discover: boolean) {
  const response = previewResult(await transport.preview("resources/read", uri), audience, "resources/read", uri);
  if (!Array.isArray(response.contents) || response.contents.length !== 1) return changed();
  const content = object(response.contents[0]), actual = object(content._meta);
  if (content.uri !== uri || content.mimeType !== "text/markdown" || typeof content.text !== "string" || !content.text.length || new TextEncoder().encode(content.text).length > 512 * 1024 || Object.entries(metadata).some(([key, value]) => JSON.stringify(ordered(actual[key])) !== JSON.stringify(ordered(value)))) return changed();
  return { text: content.text, metadata: actual, expectation: { uri, discover, text_sha256: await taskTextHash(content.text), mime_type: "text/markdown", metadata } satisfies TaskResourceExpectation };
}
type EvidenceSelection = { apiID: string; publicationKind: string; publicationID: string; entityID: string; hash: string; publicationHash?: string };
function selectedEvidence(recipe: APIRecipe): EvidenceSelection[] {
  if (!Array.isArray(recipe.dependencies)) throw new TaskConnectionEvidenceError();
  if (recipe.dependencies.length > 128) throw new TaskConnectionLimitError();
  const result: EvidenceSelection[] = [], seen = new Set<string>();
  for (const dependency of recipe.dependencies) {
    if (!dependency.kind.startsWith("developer_asset_") && dependency.kind !== "product_contract_operation") continue;
    const parts = dependency.resource_id.split(":"), hash = dependency.version.split("@").at(-1) ?? "";
    let value: EvidenceSelection;
    if (dependency.kind === "product_contract_operation" && parts.length === 4 && parts[0] === dependency.kind) {
      // Operation dependencies store the canonical fact fingerprint, not the
      // operation content hash. The API binding pins its immutable contract
      // revision; the scoped index supplies the exact operation's content hash.
      value = { apiID: parts[1], publicationKind: "contract", publicationID: parts[2], entityID: parts[3], hash: "" };
      if (!value.apiID || !/^[0-9a-f]{64}$/.test(dependency.version)) throw new TaskConnectionEvidenceError();
    } else {
      const kinds: Record<string, string> = { developer_asset_documentation: "documentation_collection", developer_asset_contract: "contract", developer_asset_sdk: "sdk" };
      if (parts.length !== 6 || parts[0] !== "developer_asset" || !["api", "global_documentation"].includes(parts[1]) || parts[3] !== kinds[dependency.kind] || (parts[1] === "api" ? !parts[2] : Boolean(parts[2]) || parts[3] !== "documentation_collection")) throw new TaskConnectionEvidenceError();
      value = { apiID: parts[2], publicationKind: parts[3], publicationID: parts[4], entityID: parts[5], hash };
      if (!/^sha256:[0-9a-f]{64}$/.test(hash) || !dependency.version.startsWith(value.publicationID+"@")) throw new TaskConnectionEvidenceError();
      if (!value.apiID) {
        const version = dependency.version.split("@");
        if (version.length !== 5 || !/^sha256:[0-9a-f]{64}$/.test(version[2]) || version[2] !== version[3]) throw new TaskConnectionEvidenceError();
        value.publicationHash = version[2];
      }
    }
    if (!value.publicationID || !value.entityID || (value.apiID && !taskAPIIDs(recipe).includes(value.apiID))) throw new TaskConnectionEvidenceError();
    const key = JSON.stringify(value);
    if (!seen.has(key)) { seen.add(key); result.push(value); }
  }
  return result;
}
type EvidenceIndex = { apiID: string; uri: string; generation: string; pins: Record<string, unknown> };
function evidenceIndex(apiID: string, uri: string, metadata: Record<string, unknown>): EvidenceIndex {
  if (metadata.map_version !== 2 || metadata.publication_uri !== uri || metadata.index_uri !== `${uri}/map-v2/index` || !Number.isSafeInteger(metadata.evidence_count) || Number(metadata.evidence_count) < 0 || metadata.evidence_resources !== undefined || typeof metadata.search_index_generation_id !== "string" || !metadata.search_index_generation_id) throw new TaskConnectionEvidenceError();
  const pins = { map_version: 2, publication_uri: uri, search_index_generation_id: metadata.search_index_generation_id, ...(apiID ? { api_id: apiID, api_developer_asset_publication_id: metadata.api_developer_asset_publication_id, api_snapshot_hash: metadata.api_snapshot_hash } : { global_documentation_publication_id: metadata.global_documentation_publication_id, snapshot_hash: metadata.snapshot_hash, revision: metadata.revision }) };
  return { apiID, uri, generation: metadata.search_index_generation_id, pins };
}
async function lookupTaskEvidence(transport: TaskConnectionTransport, audience: "private" | "public", index: EvidenceIndex, unit: EvidenceSelection) {
  const query = new URLSearchParams({ source_publication_kind: unit.publicationKind, source_publication_id: unit.publicationID, source_entity_id: unit.entityID, ...(unit.hash ? { content_hash: unit.hash } : {}) });
  const uri = `${index.uri}/map-v2/index?${query}`;
  const response = await readResource(transport, audience, uri, index.pins, false);
  const metadata = response.metadata;
  if (new TextEncoder().encode(JSON.stringify({ text: response.text, metadata })).length > 128 * 1024) throw new TaskConnectionLimitError();
  if (metadata.index_uri !== `${index.uri}/map-v2/index` || metadata.next_uri !== undefined || metadata.match_count !== 1 || !Array.isArray(metadata.evidence_resources) || metadata.evidence_resources.length !== 1) throw new TaskConnectionEvidenceError();
  const entry = object(metadata.evidence_resources[0]);
  if (["uri", "title", "kind", "knowledge_unit_id", "source_publication_kind", "source_publication_id", "source_entity_id", "content_hash"].some((key) => typeof entry[key] !== "string" || !entry[key]) || entry.uri !== `${index.uri}/evidence/${entry.knowledge_unit_id}` || !/^[a-zA-Z0-9_-]+$/.test(String(entry.knowledge_unit_id)) || !/^sha256:[0-9a-f]{64}$/.test(String(entry.content_hash)) || entry.source_publication_kind !== unit.publicationKind || entry.source_publication_id !== unit.publicationID || entry.source_entity_id !== unit.entityID || (unit.hash && entry.content_hash !== unit.hash)) throw new TaskConnectionEvidenceError();
  return entry;
}
export async function reviewTaskConnection(origin: APIIntegration, selected: APIRecipe, audience: "private" | "public", endpoint: string, transport: TaskConnectionTransport): Promise<TaskConnectionReview> {
  const url = new URL(endpoint);
  if (!["https:", "http:"].includes(url.protocol) || url.username || url.password || url.hash || url.search || url.pathname !== (audience === "public" ? "/mcp/public" : "/mcp")) return changed();
  const recipe = await transport.recipe(selected.id);
  if (!publishedTaskForAPI(recipe, origin, audience) || recipe.current_revision_id !== selected.current_revision_id || recipe.revision !== selected.revision || !recipe.current_revision || recipe.current_revision.id !== recipe.current_revision_id || recipe.current_revision.recipe_id !== recipe.id) return changed();
  const revision = recipe.current_revision;
  const bindings = revision.spec_version === 3 ? revision.api_bindings ?? [] : recipe.integration_id && revision.integration_revision_id && revision.integration_manifest_hash ? [{ integration_id: recipe.integration_id, integration_revision_id: revision.integration_revision_id, integration_manifest_hash: revision.integration_manifest_hash }] : [];
  if (!bindings.length || bindings.length > 16 || new Set(bindings.map((value) => value.integration_id)).size !== bindings.length || JSON.stringify(bindings.map((value) => value.integration_id).sort()) !== JSON.stringify(taskAPIIDs(recipe))) return changed();
  const listed = await listTaskResources(transport, audience);
  const matches = listed.filter((value) => value.uri === recipe.stable_uri);
  if (matches.length !== 1 || object(matches[0]._meta).revision_id !== recipe.current_revision_id) return changed();
  const task = await readResource(transport, audience, recipe.stable_uri!, { revision_id: recipe.current_revision_id, integration_ids: taskAPIIDs(recipe) }, true);
  if (task.text !== revision.markdown) return changed();
  const resources = [task.expectation];
  const publications: TaskConnectionReview["publications"] = [];
  const evidence: TaskConnectionReview["evidence"] = [], indexes: EvidenceIndex[] = [];
  const selectedUnits = selectedEvidence(recipe), globalPublications = new Set<string>(), globalSelections = new Map<EvidenceSelection, string>();
  for (const binding of bindings) {
    const detail = await transport.integration(binding.integration_id);
    const api = detail.integration;
    const exact = detail.revisions.filter((value) => value.id === binding.integration_revision_id && value.integration_id === api.id && value.manifest_hash === binding.integration_manifest_hash && value.state === "published" && value.published_at);
    if (api.id !== binding.integration_id || api.deployment_id !== origin.deployment_id || exact.length !== 1 || (api.id === origin.id && api.visibility !== origin.visibility) || (audience === "public" && api.visibility !== "public")) return changed();
    const candidates = (await transport.publications(api.id)).filter((value) => value.api_id === api.id && value.api_revision_id === binding.integration_revision_id && value.deployment_id === origin.deployment_id);
    if (candidates.length !== 1) return changed();
    const publication = candidates[0];
    const uri = `dokosoko://developer-assets/apis/${api.id}/publications/${publication.id}`;
    const map = await readResource(transport, audience, `${uri}/map-v2`, { map_version: 2, publication_uri: uri, api_id: api.id, api_developer_asset_publication_id: publication.id, api_snapshot_hash: publication.snapshot_hash }, false);
    resources.push(map.expectation); publications.push({ api, revision: exact[0].revision, uri: `${uri}/map-v2`, text: map.text });
    const index = evidenceIndex(api.id, uri, map.metadata);
    if (selectedUnits.some((unit) => unit.apiID === api.id)) indexes.push(index);
    if (publication.deployment_documentation_publication_id) globalPublications.add(publication.deployment_documentation_publication_id);
  }
  const globalUnits = selectedUnits.filter((unit) => !unit.apiID);
  if (globalUnits.length) {
    const globals: DeploymentDocumentationPublication[] = [];
    for (const publicationID of [...globalPublications].sort()) {
      const value = await transport.documentationPublication(publicationID);
      if (value.id !== publicationID || value.deployment_id !== origin.deployment_id || !["private", "public"].includes(value.visibility) || !Number.isInteger(value.revision) || value.revision < 1 || !value.published_at || !/^sha256:[0-9a-f]{64}$/.test(value.snapshot_hash) || !Array.isArray(value.members) || new Set(value.members.map((member) => member.documentation_collection_revision_id)).size !== value.members.length) throw new TaskConnectionEvidenceError();
      globals.push(value);
    }
    const required = new Set<DeploymentDocumentationPublication>();
    for (const unit of globalUnits) {
      const value = globals.find((publication) => (audience !== "public" || publication.visibility === "public") && publication.members.some((member) => member.documentation_collection_revision_id === unit.publicationID && member.content_hash === unit.publicationHash && (audience !== "public" || member.visibility === "public")));
      if (!value) throw new TaskConnectionEvidenceError();
      required.add(value); globalSelections.set(unit, value.id);
    }
    for (const publication of globals.filter((value) => required.has(value))) {
      if (resources.length >= 32) throw new TaskConnectionLimitError();
      const uri = `dokosoko://developer-assets/global-documentation/${publication.id}`;
      const map = await readResource(transport, audience, `${uri}/map-v2`, { map_version: 2, publication_uri: uri, global_documentation_publication_id: publication.id, snapshot_hash: publication.snapshot_hash, revision: publication.revision }, false);
      indexes.push(evidenceIndex("", uri, map.metadata));
      resources.push(map.expectation); evidence.push({ title: "Global documentation publication", kind: "map", globalRevision: publication.revision, uri: `${uri}/map-v2`, text: map.text });
    }
  }
  const selectedURIs = new Set<string>();
  for (const unit of selectedUnits) {
    if (resources.length >= 32) throw new TaskConnectionLimitError();
    const matches = indexes.filter((index) => index.apiID === unit.apiID && (unit.apiID || index.uri === `dokosoko://developer-assets/global-documentation/${globalSelections.get(unit)}`));
    // Global membership selected one exact wrapper in stable publication order.
    // The targeted lookup must resolve exactly one unit inside that wrapper.
    if (matches.length !== 1) throw new TaskConnectionEvidenceError();
    const index = matches[0], entry = await lookupTaskEvidence(transport, audience, index, unit), uri = String(entry.uri);
    if (selectedURIs.has(uri)) continue;
    const value = await readResource(transport, audience, uri, { ...(unit.apiID ? { api_id: unit.apiID } : {}), knowledge_unit_id: entry.knowledge_unit_id, search_index_generation_id: index.generation, source_publication_kind: unit.publicationKind, source_publication_id: unit.publicationID, source_entity_id: unit.entityID, content_hash: entry.content_hash }, false);
    resources.push(value.expectation); evidence.push({ title: String(entry.title), kind: String(entry.kind), uri, text: value.text }); selectedURIs.add(uri);
  }
  const plan: TaskCheckPlan = { schema_version: "mcp-task-check-v1", endpoint, task: { title: recipe.title, outcome: recipe.outcome, resource_uri: recipe.stable_uri!, revision_id: recipe.current_revision_id! }, resources };
  return { plan, planHash: await taskTextHash(taskPlanJSON(plan)), recipe, publications, evidence, audience, reviewedAt: new Date().toISOString() };
}

export type TaskClientReport = { client: string; version: string; startedAt: string; retrieval: "pass" | "fail"; report: Record<string, unknown> };
export function readTaskClientReport(value: unknown, review: TaskConnectionReview): TaskClientReport {
  const report = object(value), task = object(report.task);
  if (report.endpoint !== review.plan.endpoint || report.protocol_version !== "2026-07-28" || report.evidence_origin !== "acceptance_client_observation" || task.plan_sha256 !== review.planHash || JSON.stringify(ordered(task.selection)) !== JSON.stringify(ordered(review.plan.task)) || task.implementation_status !== "not_run" || typeof report.client_name !== "string" || !report.client_name.trim() || report.client_name.length > 200 || typeof report.client_version !== "string" || !report.client_version.trim() || report.client_version.length > 100 || typeof report.started_at !== "string" || !Number.isFinite(Date.parse(report.started_at)) || !Array.isArray(report.checks) || report.checks.length > 256 || !["pass", "fail"].includes(String(task.retrieval_status))) return changed();
  const checks = report.checks.map(object);
  const required = ["server/discover", "resources/list", ...review.plan.resources.flatMap((resource) => [`resources/read: ${resource.uri}`, ...(resource.discover ? [`resource expected: ${resource.uri}`] : [])])];
  if (required.some((name) => { const matches = checks.filter((check) => check.name === name); return matches.length !== 1 || matches[0].required !== true || !["pass", "fail"].includes(String(matches[0].status)); })) return changed();
  const passed = (name: string) => { const matches = checks.filter((check) => check.name === name); return matches.length === 1 && matches[0].status === "pass" && matches[0].required === true ? matches[0] : undefined; };
  let complete = Boolean(passed("server/discover") && passed("resources/list"));
  for (const resource of review.plan.resources) {
    const read = passed(`resources/read: ${resource.uri}`);
    if (!read || read.resource_uri !== resource.uri || read.content_sha256 !== resource.text_sha256 || typeof read.content_bytes !== "number" || read.content_bytes <= 0 || typeof read.request_id !== "string" || !read.request_id || (read.response_request_id && read.response_request_id !== read.request_id) || (resource.discover && !passed(`resource expected: ${resource.uri}`))) complete = false;
  }
  if (task.retrieval_status === "pass" && !complete) return changed();
  return { client: report.client_name, version: report.client_version, startedAt: report.started_at, retrieval: task.retrieval_status as "pass" | "fail", report };
}
