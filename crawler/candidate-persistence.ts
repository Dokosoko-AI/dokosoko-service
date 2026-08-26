import { createHash } from "node:crypto";
import type pg from "pg";
import { CrawlerJobError } from "./security";
import type { IngestionDiagnostic, IngestionResult, Job, PageRecord } from "./sources";
import { canonicalJSON, compareText, sha256 } from "./ingestion/canonical";
import {
  buildDocumentationCorpus,
  buildOpenAPICandidate,
  DOCUMENTATION_MAP_SCHEMA_VERSION,
  NORMALIZED_DOCUMENT_SCHEMA_VERSION,
  renderBlock,
  type DocumentationMap,
  type NormalizedBlock,
  type NormalizedDocument,
  type NormalizedSection,
  type OpenAPICandidate,
  type QualityDiagnostic,
} from "./ingestion";

export const DOCUMENTATION_PROCESSOR_VERSIONS = Object.freeze({
  pipeline: "crawler-documentation-candidate/1",
  parser: `crawler-static-parser/${NORMALIZED_DOCUMENT_SCHEMA_VERSION}`,
  normalizer: `normalized-document/${NORMALIZED_DOCUMENT_SCHEMA_VERSION}`,
  mapper: `documentation-map/${DOCUMENTATION_MAP_SCHEMA_VERSION}+persisted-identities.2`,
});

export const OPENAPI_PROCESSOR_VERSIONS = Object.freeze({
  pipeline: "crawler-openapi-candidate/1",
  parser: "crawler-static-openapi-parser/1",
  normalizer: "canonical-openapi-json/1",
  mapper: "api-contract-map/2026-08-26",
});

const UUID_URL_NAMESPACE = "6ba7b811-9dad-11d1-80b4-00c04fd430c8";
const DOCUMENTATION_MAP_VERSION = `${DOCUMENTATION_MAP_SCHEMA_VERSION}+persisted-identities.2`;
const API_CONTRACT_MAP_VERSION = "2026-08-26";

export type LegacyDocumentReference = {
  knowledgeDocumentId: string;
  snapshotId: string;
};

export type CandidatePersistenceResult = {
  state: "persisted" | "already_persisted" | "pending_contract_target" | "unchanged_contract";
  diagnostics: readonly IngestionDiagnostic[];
};

export type PersistedKnowledgeMapEntry = {
  id: string;
  kind: string;
  title: string;
  summary: string;
  aliases?: readonly string[];
  children?: readonly PersistedKnowledgeMapEntry[];
};

export type PersistedKnowledgeMapGap = {
  kind: string;
  description: string;
  evidence_ids?: readonly string[];
};

/**
 * The exact JSON boundary consumed by model.DocumentationMapBody in Go.
 * Keep this snake_case shape deliberately small; the richer normalization map
 * remains an internal crawler value and must never be written directly to the
 * structured_map column.
 */
export type PersistedDocumentationMapBody = {
  overview: string;
  documents: readonly PersistedKnowledgeMapEntry[];
  topics: readonly PersistedKnowledgeMapEntry[];
  workflows: readonly PersistedKnowledgeMapEntry[];
  authentication?: readonly PersistedKnowledgeMapEntry[];
  errors?: readonly PersistedKnowledgeMapEntry[];
  examples?: readonly PersistedKnowledgeMapEntry[];
  versions?: readonly string[];
  languages?: readonly string[];
  gaps?: readonly PersistedKnowledgeMapGap[];
  quality_warnings?: readonly string[];
  excluded_source_ids?: readonly string[];
};

type ProcessorVersions = typeof DOCUMENTATION_PROCESSOR_VERSIONS | typeof OPENAPI_PROCESSOR_VERSIONS;

type DocumentationPersistenceIDs = {
  documents: ReadonlyMap<string, string>;
  sections: ReadonlyMap<string, string>;
};

type RawManifestEntry = {
  canonical_url: string;
  title: string;
  media_type: string;
  byte_size: number;
  response_status: number;
  rendered: boolean;
  raw_sha256: string;
  injection_indicators: readonly string[];
  legacy_knowledge_document_id: string | null;
  legacy_snapshot_id: string | null;
};

type RunShape = {
  assetKind: "documentation" | "contract";
  targetId: string;
  targetKey: string;
  versions: ProcessorVersions;
  rawManifest: readonly RawManifestEntry[];
  rawManifestHash: string;
  diagnostics: Readonly<Record<string, unknown>>;
};

type ExistingRun = {
  id: string;
  deployment_id: string;
  organisation_id: string;
  asset_kind: string;
  target_id: string | null;
  target_key: string;
  source_id: string | null;
  state: string;
  attempt: number;
  pipeline_version: string;
  parser_version: string;
  normalizer_version: string;
  mapper_version: string;
  raw_manifest_hash: string;
  resolved_source_hash: string;
};

type StageRow = {
  name: "acquire" | "validate" | "parse" | "normalize" | "segment" | "extract" | "map" | "ai_enrich" | "quality_check" | "build_index" | "review";
  state: "succeeded" | "skipped";
  inputHash: string;
  outputHash: string;
  checkpoint: Readonly<Record<string, unknown>>;
  diagnostics: Readonly<Record<string, unknown>>;
};

function uuidBytes(value: string): Buffer {
  const compact = value.replaceAll("-", "").toLowerCase();
  if (!/^[0-9a-f]{32}$/.test(compact)) throw new TypeError(`Invalid UUID namespace: ${value}`);
  return Buffer.from(compact, "hex");
}

/** RFC 4122 UUIDv5 mapping for deterministic logical ingestion IDs. */
export function deterministicIngestionUUID(kind: string, logicalId: string): string {
  const name = `https://dokosoko.dev/developer-assets/${kind}/${logicalId}`;
  const digest = createHash("sha1").update(uuidBytes(UUID_URL_NAMESPACE)).update(name, "utf8").digest().subarray(0, 16);
  digest[6] = (digest[6] & 0x0f) | 0x50;
  digest[8] = (digest[8] & 0x3f) | 0x80;
  const value = digest.toString("hex");
  return `${value.slice(0, 8)}-${value.slice(8, 12)}-${value.slice(12, 16)}-${value.slice(16, 20)}-${value.slice(20)}`;
}

function prefixedHash(value: string): string {
  return `sha256:${value}`;
}

function hashJSON(value: unknown): string {
  return prefixedHash(sha256(canonicalJSON(value)));
}

function uniqueSorted(values: readonly string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort(compareText);
}

function persistedDocumentationEvidenceID(
  value: string,
  identities?: DocumentationPersistenceIDs,
): string {
  if (!identities) return value;
  for (const [prefix, values] of [
    ["document:", identities.documents],
    ["section:", identities.sections],
  ] as const) {
    if (value.startsWith(prefix)) {
      const mapped = values.get(value.slice(prefix.length));
      return mapped ? `${prefix}${mapped}` : value;
    }
  }
  const direct = identities.documents.get(value) ?? identities.sections.get(value);
  if (direct) return direct;
  const anchor = value.indexOf("#");
  if (anchor > 0) {
    const mappedDocument = identities.documents.get(value.slice(0, anchor));
    if (mappedDocument) return `${mappedDocument}${value.slice(anchor)}`;
  }
  return value;
}

function documentationOutlineEntries(
  documentId: string,
  nodes: DocumentationMap["documents"][number]["outline"],
  identities?: DocumentationPersistenceIDs,
): PersistedKnowledgeMapEntry[] {
  return nodes.map((node) => {
    const aliases = uniqueSorted([node.anchor]);
    const children = documentationOutlineEntries(documentId, node.children, identities);
    return {
      id: persistedDocumentationEvidenceID(node.sectionIds[0] ?? `${documentId}#${node.anchor}`, identities),
      kind: "section",
      title: node.title,
      summary: `Level ${node.level} documentation heading in ${persistedDocumentationEvidenceID(documentId, identities)}.`,
      ...(aliases.length > 0 ? { aliases } : {}),
      ...(children.length > 0 ? { children } : {}),
    };
  });
}

function documentationEntryPoint(
  entry: DocumentationMap["entryPoints"][number],
  identities?: DocumentationPersistenceIDs,
): PersistedKnowledgeMapEntry {
  return {
    id: persistedDocumentationEvidenceID(entry.sectionId, identities),
    kind: entry.kind,
    title: entry.title,
    summary: `${entry.kind} entry point in documentation document ${persistedDocumentationEvidenceID(entry.documentId, identities)}.`,
    aliases: uniqueSorted([entry.kind]),
  };
}

/** Convert the rich crawler map into the exact Go persistence contract. */
export function persistedDocumentationMapBody(
  map: DocumentationMap,
  diagnostics: readonly QualityDiagnostic[] = [],
  identities?: DocumentationPersistenceIDs,
): PersistedDocumentationMapBody {
  const documents = map.documents.map((document) => {
    const aliases = uniqueSorted([document.canonicalUrl, ...document.topics]);
    const children = documentationOutlineEntries(document.documentId, document.outline, identities);
    return {
      id: persistedDocumentationEvidenceID(document.documentId, identities),
      kind: "document",
      title: document.title,
      summary: `${document.format} documentation with ${document.sectionCount} normalized section(s) at ${document.canonicalUrl}.`,
      ...(aliases.length > 0 ? { aliases } : {}),
      ...(children.length > 0 ? { children } : {}),
    } satisfies PersistedKnowledgeMapEntry;
  });
  const topicOwners = new Map<string, Set<string>>();
  for (const document of map.documents) {
    for (const topic of document.topics) {
      const normalized = topic.trim();
      if (!normalized) continue;
      const owners = topicOwners.get(normalized) ?? new Set<string>();
      owners.add(document.documentId);
      topicOwners.set(normalized, owners);
    }
  }
  const topics = [...topicOwners.entries()]
    .sort(([left], [right]) => compareText(left, right))
    .map(([title, owners]) => ({
      id: `topic_${sha256(title.toLocaleLowerCase("en-US")).slice(0, 24)}`,
      kind: "topic",
      title,
      summary: `Documentation topic present in ${owners.size} normalized document(s).`,
      aliases: [...owners].map((owner) => persistedDocumentationEvidenceID(owner, identities)).sort(compareText),
    }));
  const entryPoints = map.entryPoints.map((entry) => documentationEntryPoint(entry, identities));
  const entries = (kind: DocumentationMap["entryPoints"][number]["kind"]): PersistedKnowledgeMapEntry[] =>
    entryPoints.filter((_, index) => map.entryPoints[index]?.kind === kind);
  const workflows = entryPoints.filter((_, index) =>
    ["setup", "reference", "concepts"].includes(map.entryPoints[index]?.kind ?? ""));
  const gaps = diagnostics
    .filter((diagnostic) => diagnostic.severity !== "info")
    .map((diagnostic) => {
      const evidenceIds = uniqueSorted([
        ...(diagnostic.evidenceIds ?? []),
        diagnostic.sectionId ?? "",
        diagnostic.documentId ?? "",
      ].map((value) => persistedDocumentationEvidenceID(value, identities)));
      return {
        kind: diagnostic.code,
        description: diagnostic.message,
        ...(evidenceIds.length > 0 ? { evidence_ids: evidenceIds } : {}),
      } satisfies PersistedKnowledgeMapGap;
    });
  const qualityWarnings = uniqueSorted(diagnostics
    .filter((diagnostic) => diagnostic.severity !== "info")
    .map((diagnostic) => `${diagnostic.code}: ${diagnostic.message}`));
  const excludedSourceIds = uniqueSorted(diagnostics
    .filter((diagnostic) => diagnostic.code === "document_duplicate")
    .map((diagnostic) => persistedDocumentationEvidenceID(diagnostic.documentId ?? "", identities)));
  const overview = [
    `${map.overview.documentCount} normalized documentation document(s)`,
    `${map.overview.sectionCount} section(s)`,
    map.overview.formats.length > 0 ? `formats: ${map.overview.formats.join(", ")}` : "",
    map.overview.languages.length > 0 ? `languages: ${map.overview.languages.join(", ")}` : "",
  ].filter(Boolean).join("; ") + ".";
  return {
    overview,
    documents,
    topics,
    workflows,
    ...(entries("authentication").length > 0 ? { authentication: entries("authentication") } : {}),
    ...(entries("errors").length > 0 ? { errors: entries("errors") } : {}),
    ...(entries("examples").length > 0 ? { examples: entries("examples") } : {}),
    ...(map.overview.languages.length > 0 ? { languages: uniqueSorted(map.overview.languages) } : {}),
    ...(gaps.length > 0 ? { gaps } : {}),
    ...(qualityWarnings.length > 0 ? { quality_warnings: qualityWarnings } : {}),
    ...(excludedSourceIds.length > 0 ? { excluded_source_ids: excludedSourceIds } : {}),
  };
}

function rawManifest(
  result: IngestionResult,
  legacyDocuments: ReadonlyMap<string, LegacyDocumentReference>,
): RawManifestEntry[] {
  return [...result.pages]
    .sort((left, right) => compareText(left.url, right.url))
    .map((page) => {
      const legacy = legacyDocuments.get(page.url);
      return {
        canonical_url: page.url,
        title: page.title,
        media_type: page.contentType,
        byte_size: Buffer.byteLength(page.html, "utf8"),
        response_status: page.status,
        rendered: page.rendered,
        raw_sha256: prefixedHash(sha256(page.html)),
        injection_indicators: [...page.indicators].sort(compareText),
        legacy_knowledge_document_id: legacy?.knowledgeDocumentId ?? null,
        legacy_snapshot_id: legacy?.snapshotId ?? null,
      };
    });
}

async function lockCurrentCrawlLease(client: pg.PoolClient, job: Job): Promise<void> {
  const lease = await client.query<{ id: string }>(`
    SELECT id::text
    FROM crawl_jobs
    WHERE id = $1
      AND state = 'running'
      AND lease_owner = $2
      AND lease_expires_at > now()
    FOR UPDATE`, [job.id, job.lease_owner]);
  if (!lease.rows[0]) {
    throw new CrawlerJobError("crawler_lease_lost", "The crawler no longer owns this job lease; typed candidates were not committed.");
  }
}

async function assertCurrentCrawlLease(client: pg.PoolClient, job: Job): Promise<void> {
  const lease = await client.query<{ id: string }>(`
    SELECT id::text
    FROM crawl_jobs
    WHERE id = $1
      AND state = 'running'
      AND lease_owner = $2
      AND lease_expires_at > now()`, [job.id, job.lease_owner]);
  if (!lease.rows[0]) {
    throw new CrawlerJobError("crawler_lease_lost", "The crawler lease expired before typed candidates could be committed.");
  }
}

function runIdentityMatches(existing: ExistingRun, job: Job, shape: RunShape): boolean {
  return existing.deployment_id === job.product_id
    && existing.organisation_id === job.organisation_id
    && existing.asset_kind === shape.assetKind
    && existing.target_id === shape.targetId
    && existing.target_key === shape.targetKey
    && existing.source_id === job.source_id
    && existing.pipeline_version === shape.versions.pipeline
    && existing.parser_version === shape.versions.parser
    && existing.normalizer_version === shape.versions.normalizer
    && existing.mapper_version === shape.versions.mapper;
}

async function ensureRunningRun(client: pg.PoolClient, job: Job, shape: RunShape): Promise<"running" | "complete"> {
  await client.query(`
    INSERT INTO developer_asset_ingestion_runs(
      id, deployment_id, organisation_id, asset_kind, target_id, target_key, source_id,
      resolved_source_uri, resolved_source_revision, resolved_source_hash,
      state, attempt, pipeline_version, parser_version, normalizer_version, mapper_version,
      raw_manifest, raw_manifest_hash, diagnostics,
      discovered_count, acquired_count, failed_count, skipped_count, quarantined_count,
      lease_owner, lease_expires_at, heartbeat_at, queued_at, started_at
    )
    SELECT $1, $2, $3, $4, $5, $6, $7,
           $8, '', $9, 'running', $10, $11, $12, $13, $14,
           $15::jsonb, $16, $17::jsonb,
           $18, $19, $20, $21, $22,
           job.lease_owner, job.lease_expires_at, job.heartbeat_at, now(), now()
    FROM crawl_jobs job
    WHERE job.id = $1
      AND job.state = 'running'
      AND job.lease_owner = $23
      AND job.lease_expires_at > now()
    ON CONFLICT (id) DO NOTHING`, [
    job.id,
    job.product_id,
    job.organisation_id,
    shape.assetKind,
    shape.targetId,
    shape.targetKey,
    job.source_id,
    job.location,
    shape.rawManifestHash,
    job.attempt,
    shape.versions.pipeline,
    shape.versions.parser,
    shape.versions.normalizer,
    shape.versions.mapper,
    JSON.stringify(shape.rawManifest),
    shape.rawManifestHash,
    JSON.stringify(shape.diagnostics),
    shape.diagnostics.counts && typeof shape.diagnostics.counts === "object"
      ? Number((shape.diagnostics.counts as Record<string, unknown>).discovered ?? 0) : 0,
    shape.rawManifest.length,
    shape.diagnostics.counts && typeof shape.diagnostics.counts === "object"
      ? Number((shape.diagnostics.counts as Record<string, unknown>).failed ?? 0) : 0,
    shape.diagnostics.counts && typeof shape.diagnostics.counts === "object"
      ? Number((shape.diagnostics.counts as Record<string, unknown>).skipped ?? 0) : 0,
    shape.rawManifest.filter((item) => item.injection_indicators.length > 0).length,
    job.lease_owner,
  ]);

  const current = await client.query<ExistingRun>(`
    SELECT id::text, deployment_id::text, organisation_id::text, asset_kind,
           target_id::text, target_key, source_id::text, state, attempt,
           pipeline_version, parser_version, normalizer_version, mapper_version,
           raw_manifest_hash, resolved_source_hash
    FROM developer_asset_ingestion_runs
    WHERE id = $1
    FOR UPDATE`, [job.id]);
  const existing = current.rows[0];
  if (!existing || !runIdentityMatches(existing, job, shape)) {
    throw new CrawlerJobError("developer_asset_run_conflict", "The crawl job UUID is already bound to a different immutable developer-asset ingestion identity.");
  }
  if (existing.state === "review_ready" || existing.state === "published") {
    if (existing.raw_manifest_hash !== shape.rawManifestHash || existing.resolved_source_hash !== shape.rawManifestHash) {
      throw new CrawlerJobError("developer_asset_run_conflict", "An immutable completed ingestion run has a different acquired-source hash.");
    }
    return "complete";
  }
  if (existing.state === "queued") {
    if (existing.attempt !== job.attempt) throw new CrawlerJobError("developer_asset_run_conflict", "The queued ingestion run belongs to a different crawl attempt.");
    const started = await client.query(`
      UPDATE developer_asset_ingestion_runs
      SET state = 'running', raw_manifest = $2::jsonb, raw_manifest_hash = $3,
          resolved_source_uri = $4, resolved_source_hash = $3,
          diagnostics = $5::jsonb, lease_owner = $6,
          lease_expires_at = (SELECT lease_expires_at FROM crawl_jobs WHERE id = $1),
          heartbeat_at = now(), started_at = COALESCE(started_at, now()), finished_at = null
      WHERE id = $1 AND state = 'queued'`, [
      job.id, JSON.stringify(shape.rawManifest), shape.rawManifestHash, job.location,
      JSON.stringify(shape.diagnostics), job.lease_owner,
    ]);
    if (started.rowCount !== 1) throw new CrawlerJobError("developer_asset_run_conflict", "The ingestion run could not be started from its queued state.");
    return "running";
  }
  if (existing.state !== "running" || existing.attempt !== job.attempt) {
    throw new CrawlerJobError("developer_asset_run_conflict", `The immutable ingestion run is already ${existing.state}.`);
  }
  await client.query(`
    UPDATE developer_asset_ingestion_runs
    SET raw_manifest = $2::jsonb, raw_manifest_hash = $3,
        resolved_source_uri = $4, resolved_source_hash = $3,
        diagnostics = $5::jsonb, lease_owner = $6,
        lease_expires_at = (SELECT lease_expires_at FROM crawl_jobs WHERE id = $1),
        heartbeat_at = now()
    WHERE id = $1 AND state = 'running'`, [
    job.id, JSON.stringify(shape.rawManifest), shape.rawManifestHash, job.location,
    JSON.stringify(shape.diagnostics), job.lease_owner,
  ]);
  return "running";
}

async function insertStages(client: pg.PoolClient, job: Job, stages: readonly StageRow[]): Promise<void> {
  for (const stage of stages) {
    await client.query(`
      INSERT INTO developer_asset_ingestion_stages(
        id, ingestion_run_id, stage_name, attempt, state, input_hash, output_hash,
        checkpoint, diagnostics, started_at, finished_at
      )
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,now(),now())
      ON CONFLICT (ingestion_run_id, stage_name, attempt) DO UPDATE
      SET state = EXCLUDED.state, input_hash = EXCLUDED.input_hash,
          output_hash = EXCLUDED.output_hash, checkpoint = EXCLUDED.checkpoint,
          diagnostics = EXCLUDED.diagnostics, error_code = '', error_message = '',
          started_at = COALESCE(developer_asset_ingestion_stages.started_at, EXCLUDED.started_at),
          finished_at = EXCLUDED.finished_at, updated_at = now()`, [
      deterministicIngestionUUID("stage", `${job.id}:${stage.name}:${job.attempt}`),
      job.id,
      stage.name,
      job.attempt,
      stage.state,
      stage.inputHash,
      stage.outputHash,
      JSON.stringify(stage.checkpoint),
      JSON.stringify(stage.diagnostics),
    ]);
  }
}

async function markReviewReady(client: pg.PoolClient, job: Job, diagnostics: Readonly<Record<string, unknown>>): Promise<void> {
  await assertCurrentCrawlLease(client, job);
  const updated = await client.query(`
    UPDATE developer_asset_ingestion_runs
    SET state = 'review_ready', diagnostics = $2::jsonb,
        lease_owner = '', lease_expires_at = null, heartbeat_at = now(),
        error_code = '', error_message = '', finished_at = now()
    WHERE id = $1 AND state = 'running' AND lease_owner = $3`, [job.id, JSON.stringify(diagnostics), job.lease_owner]);
  if (updated.rowCount !== 1) {
    throw new CrawlerJobError("crawler_lease_lost", "The typed ingestion run could not be finalized by the current lease owner.");
  }
}

function documentationKind(document: NormalizedDocument): "guide" | "reference" | "tutorial" | "concept" | "changelog" | "example" | "other" {
  const value = `${document.title} ${document.source.canonicalUrl}`;
  if (/\bchangelog|release notes?\b/i.test(value)) return "changelog";
  if (/\btutorial|walkthrough|quick ?start\b/i.test(value)) return "tutorial";
  if (/\bexample|sample|recipe\b/i.test(value)) return "example";
  if (/\breference|api|endpoint|method|class|function\b/i.test(value)) return "reference";
  if (/\bconcept|architecture|overview\b/i.test(value)) return "concept";
  return document.blocks.length > 0 ? "guide" : "other";
}

function referencedBlocks(document: NormalizedDocument, section: NormalizedSection): NormalizedBlock[] {
  const byId = new Map(document.blocks.map((block) => [block.id, block]));
  return section.blockSlices.map((slice) => byId.get(slice.blockId)).filter((block): block is NormalizedBlock => Boolean(block));
}

function sectionContentKind(blocks: readonly NormalizedBlock[], title: string): "prose" | "code" | "table" | "schema" | "operation" | "example" | "warning" | "mixed" {
  if (/\bwarning|caution|danger|important\b/i.test(title)) return "warning";
  if (/\bexample|sample\b/i.test(title)) return "example";
  if (/\bschema|model\b/i.test(title)) return "schema";
  if (/\b(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE)\s+\//.test(title)) return "operation";
  const types = new Set(blocks.filter((block) => block.type !== "heading").map((block) => block.type));
  if (types.size === 1 && types.has("code")) return "code";
  if (types.size === 1 && types.has("table")) return "table";
  if (types.has("code") || types.has("table")) return "mixed";
  return "prose";
}

function sectionSourceRange(blocks: readonly NormalizedBlock[]): { start: number | null; end: number | null } {
  if (blocks.length === 0) return { start: null, end: null };
  return {
    start: Math.min(...blocks.map((block) => block.sourceRange.startOffset)),
    end: Math.max(...blocks.map((block) => block.sourceRange.endOffset)),
  };
}

function documentationAgentMarkdown(
  documents: readonly NormalizedDocument[],
  sections: readonly NormalizedSection[],
  identities?: DocumentationPersistenceIDs,
): string {
  const lines = [
    "# Documentation Map",
    "",
    `Documents: ${documents.length}`,
    `Sections: ${sections.length}`,
    "",
    "## Table of contents",
    "",
  ];
  for (const document of documents) {
    const documentID = persistedDocumentationEvidenceID(document.id, identities);
    lines.push(`- ${document.title.replace(/\s+/g, " ")} — ${document.source.canonicalUrl} — evidence \`document:${documentID}\``);
    for (const section of sections.filter((candidate) => candidate.documentId === document.id)) {
      const breadcrumb = section.headingPath.map((item) => item.text).join(" > ") || section.title;
      const sectionID = persistedDocumentationEvidenceID(section.id, identities);
      lines.push(`  - ${breadcrumb.replace(/\s+/g, " ")} — evidence \`section:${sectionID}\``);
    }
  }
  return `${lines.join("\n").trim()}\n`;
}

function documentationStages(
  job: Job,
  manifestHash: string,
  corpus: ReturnType<typeof buildDocumentationCorpus>,
  mapHash: string,
): StageRow[] {
  const parsedHash = hashJSON(corpus.documents.map((document) => ({ id: document.id, raw: document.source.rawSha256 })));
  const normalizedHash = hashJSON(corpus.documents.map((document) => ({ id: document.id, normalized: document.normalizedSha256 })));
  const sectionsHash = hashJSON(corpus.sections.map((section) => ({ id: section.id, content: section.contentSha256 })));
  const qualityHash = hashJSON(corpus.diagnostics);
  const sourceHash = hashJSON({ source_id: job.source_id, location: job.location });
  const skip = (name: StageRow["name"], reason: string, inputHash: string): StageRow => ({
    name, state: "skipped", inputHash, outputHash: "", checkpoint: {}, diagnostics: { reason },
  });
  return [
    { name: "acquire", state: "succeeded", inputHash: sourceHash, outputHash: manifestHash, checkpoint: { acquired_count: corpus.documents.length }, diagnostics: {} },
    { name: "validate", state: "succeeded", inputHash: manifestHash, outputHash: manifestHash, checkpoint: {}, diagnostics: {} },
    { name: "parse", state: "succeeded", inputHash: manifestHash, outputHash: parsedHash, checkpoint: { document_logical_ids: corpus.documents.map((item) => item.id) }, diagnostics: {} },
    { name: "normalize", state: "succeeded", inputHash: parsedHash, outputHash: normalizedHash, checkpoint: { schema_version: NORMALIZED_DOCUMENT_SCHEMA_VERSION }, diagnostics: {} },
    { name: "segment", state: "succeeded", inputHash: normalizedHash, outputHash: sectionsHash, checkpoint: { section_logical_ids: corpus.sections.map((item) => item.id) }, diagnostics: {} },
    skip("extract", "Documentation extraction is represented by the normalized document and section records.", sectionsHash),
    { name: "map", state: "succeeded", inputHash: sectionsHash, outputHash: mapHash, checkpoint: { map_logical_id: corpus.map.id }, diagnostics: {} },
    skip("ai_enrich", "Deterministic output is reviewable without optional AI enrichment.", mapHash),
    { name: "quality_check", state: "succeeded", inputHash: sectionsHash, outputHash: qualityHash, checkpoint: { diagnostic_count: corpus.diagnostics.length }, diagnostics: { items: corpus.diagnostics } },
    skip("build_index", "Published retrieval indexes are built from an approved Go-side publication.", mapHash),
    { name: "review", state: "succeeded", inputHash: mapHash, outputHash: mapHash, checkpoint: { state: "review_ready" }, diagnostics: {} },
  ];
}

async function documentationOutputExists(
  client: pg.PoolClient,
  job: Job,
  documentCount: number,
  sectionCount: number,
): Promise<boolean> {
  const result = await client.query<{ document_count: number; section_count: number; map_count: number }>(`
    SELECT
      (SELECT count(*)::integer FROM documentation_documents WHERE ingestion_run_id = $1) AS document_count,
      (SELECT count(*)::integer FROM documentation_sections section
        JOIN documentation_documents document ON document.id = section.documentation_document_id
       WHERE document.ingestion_run_id = $1) AS section_count,
      (SELECT count(*)::integer FROM documentation_maps
       WHERE ingestion_run_id = $1 AND map_version = $2) AS map_count`, [job.id, DOCUMENTATION_MAP_VERSION]);
  const counts = result.rows[0];
  return counts?.document_count === documentCount && counts.section_count === sectionCount && counts.map_count === 1;
}

async function persistDocumentation(
  client: pg.PoolClient,
  job: Job,
  result: IngestionResult,
  legacyDocuments: ReadonlyMap<string, LegacyDocumentReference>,
): Promise<CandidatePersistenceResult> {
  const manifest = rawManifest(result, legacyDocuments);
  const manifestHash = hashJSON(manifest);
  const corpus = buildDocumentationCorpus(result.pages.map((page) => ({
    content: page.html,
    source: {
      sourceId: job.source_id,
      canonicalUrl: page.url,
      contentType: page.contentType,
      sourceKind: job.source_kind,
      snapshotId: legacyDocuments.get(page.url)?.snapshotId,
    },
    documentKind: "documentation" as const,
  })));
  const documentUUIDs = new Map(corpus.documents.map((document) => [
    document.id,
    deterministicIngestionUUID("documentation-document", `${job.id}:${document.id}`),
  ]));
  const sectionUUIDs = new Map(corpus.sections.map((section) => [
    section.id,
    deterministicIngestionUUID("documentation-section", `${job.id}:${section.id}`),
  ]));
  const persistedIDs: DocumentationPersistenceIDs = { documents: documentUUIDs, sections: sectionUUIDs };
  const agentMarkdown = documentationAgentMarkdown(corpus.documents, corpus.sections, persistedIDs);
  const structuredMap = persistedDocumentationMapBody(corpus.map, corpus.diagnostics, persistedIDs);
  const mapHash = hashJSON({ structuredMap, agentMarkdown });
  const partial = result.failedCount > 0 || result.skippedCount > 0;
  const diagnostics = {
    partial,
    counts: {
      discovered: result.discoveredCount,
      acquired: result.pages.length,
      failed: result.failedCount,
      skipped: result.skippedCount,
      redirected: result.redirectedCount,
      quarantined: manifest.filter((item) => item.injection_indicators.length > 0).length,
    },
    acquisition: result.diagnostics,
    quality: corpus.diagnostics,
    processor_versions: DOCUMENTATION_PROCESSOR_VERSIONS,
    map_logical_id: corpus.map.id,
  };
  const shape: RunShape = {
    assetKind: "documentation",
    targetId: job.source_id,
    targetKey: `source:${job.source_id}`,
    versions: DOCUMENTATION_PROCESSOR_VERSIONS,
    rawManifest: manifest,
    rawManifestHash: manifestHash,
    diagnostics,
  };
  const runState = await ensureRunningRun(client, job, shape);
  if (runState === "complete") {
    if (!(await documentationOutputExists(client, job, corpus.documents.length, corpus.sections.length))) {
      throw new CrawlerJobError("developer_asset_run_incomplete", "The immutable review-ready documentation run is missing one or more typed outputs.");
    }
    return { state: "already_persisted", diagnostics: [] };
  }

  const pages = new Map(result.pages.map((page) => [page.url, page]));
  for (let ordinal = 0; ordinal < corpus.documents.length; ordinal++) {
    const document = corpus.documents[ordinal];
    const page = pages.get(document.source.canonicalUrl);
    const normalizedMarkdown = document.blocks.map(renderBlock).join("\n\n");
    const legacy = legacyDocuments.get(document.source.canonicalUrl);
    await client.query(`
      INSERT INTO documentation_documents(
        id, deployment_id, ingestion_run_id, legacy_knowledge_document_id,
        source_path, canonical_url, title, document_kind, language, media_type,
        normalized_markdown, content_hash, visibility, ordinal, metadata
      )
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'',$9,$10,$11,$12,$13,$14::jsonb)`, [
      documentUUIDs.get(document.id),
      job.product_id,
      job.id,
      legacy?.knowledgeDocumentId ?? null,
      document.source.canonicalUrl,
      document.source.canonicalUrl,
      document.title,
      documentationKind(document),
      page?.contentType ?? document.source.contentType,
      normalizedMarkdown,
      prefixedHash(sha256(normalizedMarkdown)),
      job.visibility,
      ordinal,
      JSON.stringify({
        logical_id: document.id,
        logical_evidence_id: `document:${document.id}`,
        evidence_id: `document:${documentUUIDs.get(document.id)}`,
        schema_version: document.schemaVersion,
        format: document.format,
        source_lineage: {
          ...document.source,
          rawSha256: prefixedHash(document.source.rawSha256),
          normalizedSha256: prefixedHash(document.normalizedSha256),
        },
        block_logical_ids: document.blocks.map((block) => block.id),
        diagnostic_codes: corpus.diagnostics.filter((item) => item.documentId === document.id).map((item) => item.code),
      }),
    ]);
  }

  const firstSectionByPath = new Map<string, string>();
  for (const document of corpus.documents) {
    const documentSections = corpus.sections.filter((section) => section.documentId === document.id);
    for (const section of documentSections) {
      const uuid = sectionUUIDs.get(section.id);
      if (!uuid) throw new CrawlerJobError("developer_asset_run_incomplete", `Missing persisted identity for section ${section.id}.`);
      const pathKey = canonicalJSON(section.headingPath.map((item) => ({ level: item.level, anchor: item.anchor })));
      const parentKey = canonicalJSON(section.headingPath.slice(0, -1).map((item) => ({ level: item.level, anchor: item.anchor })));
      const parentUUID = section.headingPath.length > 1 ? firstSectionByPath.get(`${document.id}\0${parentKey}`) ?? null : null;
      const blocks = referencedBlocks(document, section);
      const sourceRange = sectionSourceRange(blocks);
      const languages = [...new Set(blocks.filter((block) => block.type === "code" && block.language).map((block) => block.type === "code" ? block.language ?? "" : ""))];
      await client.query(`
        INSERT INTO documentation_sections(
          id, deployment_id, documentation_document_id, parent_section_id,
          ordinal, heading_level, heading, anchor, breadcrumb, content_kind,
          normalized_text, code_language, token_estimate, source_start, source_end,
          content_hash, metadata
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::text[],$10,$11,$12,$13,$14,$15,$16,$17::jsonb)`, [
        uuid,
        job.product_id,
        documentUUIDs.get(document.id),
        parentUUID,
        section.ordinal,
        section.headingPath.at(-1)?.level ?? 0,
        section.title,
        section.anchor,
        section.headingPath.map((item) => item.text),
        sectionContentKind(blocks, section.title),
        section.content,
        languages.length === 1 ? languages[0] : "",
        section.estimatedTokens,
        sourceRange.start,
        sourceRange.end,
        prefixedHash(section.contentSha256),
        JSON.stringify({
          logical_id: section.id,
          logical_evidence_id: `section:${section.id}`,
          evidence_id: `section:${uuid}`,
          document_logical_id: section.documentId,
          heading_path: section.headingPath,
          block_slices: section.blockSlices,
          source_lineage: {
            ...section.source,
            rawSha256: prefixedHash(section.source.rawSha256),
            documentNormalizedSha256: prefixedHash(section.source.documentNormalizedSha256),
          },
        }),
      ]);
      if (!firstSectionByPath.has(`${document.id}\0${pathKey}`)) firstSectionByPath.set(`${document.id}\0${pathKey}`, uuid);
    }
  }

  await client.query(`
    INSERT INTO documentation_maps(
      id, deployment_id, ingestion_run_id, documentation_collection_revision_id,
      map_version, structured_map, agent_markdown, content_hash, visibility
    )
    VALUES ($1,$2,$3,NULL,$4,$5::jsonb,$6,$7,$8)`, [
    deterministicIngestionUUID("documentation-map", `${job.id}:${corpus.map.id}`),
    job.product_id,
    job.id,
    DOCUMENTATION_MAP_VERSION,
    JSON.stringify(structuredMap),
    agentMarkdown,
    mapHash,
    job.visibility,
  ]);
  await insertStages(client, job, documentationStages(job, manifestHash, corpus, mapHash));
  await markReviewReady(client, job, diagnostics);
  return { state: "persisted", diagnostics: [] };
}

function openAPIFormat(page: PageRecord): "json" | "yaml" {
  return page.html.trimStart().startsWith("{") ? "json" : "yaml";
}

async function resolveContractTarget(
  client: pg.PoolClient,
  job: Job,
): Promise<{ id: string; visibility: "private" | "public" } | null> {
  const target = await client.query<{ api_contract_id: string; visibility: "private" | "public" }>(`
    SELECT binding.api_contract_id::text, contract.visibility::text
    FROM api_contract_sources binding
    JOIN api_contracts contract
      ON contract.id = binding.api_contract_id
     AND contract.deployment_id = binding.deployment_id
    WHERE binding.deployment_id = $1
      AND binding.source_id = $2
      AND binding.lifecycle = 'attached'
      AND contract.lifecycle = 'active'
    ORDER BY binding.api_contract_id
    LIMIT 2
    FOR SHARE OF binding, contract`, [job.product_id, job.source_id]);
  if (target.rows.length > 1) {
    throw new CrawlerJobError("contract_catalog_binding_ambiguous", "The OpenAPI source is bound to more than one active API contract.");
  }
  const resolved = target.rows[0];
  return resolved ? { id: resolved.api_contract_id, visibility: resolved.visibility } : null;
}

function contractStages(job: Job, manifestHash: string, candidate: OpenAPICandidate): StageRow[] {
  const sourceHash = hashJSON({ source_id: job.source_id, location: job.location });
  const normalizedHash = prefixedHash(candidate.contentHash);
  const extractedHash = hashJSON({
    operations: candidate.operations.map((item) => item.contentHash),
    schemas: candidate.schemas.map((item) => item.contentHash),
    examples: candidate.examples.map((item) => item.contentHash),
  });
  const mapHash = prefixedHash(candidate.mapContentHash);
  const skip = (name: StageRow["name"], reason: string, inputHash: string): StageRow => ({
    name, state: "skipped", inputHash, outputHash: "", checkpoint: {}, diagnostics: { reason },
  });
  return [
    { name: "acquire", state: "succeeded", inputHash: sourceHash, outputHash: manifestHash, checkpoint: { acquired_count: 1 }, diagnostics: {} },
    { name: "validate", state: "succeeded", inputHash: manifestHash, outputHash: prefixedHash(candidate.sourceHash), checkpoint: { openapi_version: candidate.openapiVersion }, diagnostics: {} },
    { name: "parse", state: "succeeded", inputHash: manifestHash, outputHash: normalizedHash, checkpoint: { candidate_logical_id: candidate.logicalId }, diagnostics: {} },
    { name: "normalize", state: "succeeded", inputHash: normalizedHash, outputHash: normalizedHash, checkpoint: { format: candidate.sourceFormat }, diagnostics: {} },
    skip("segment", "OpenAPI candidates are segmented into typed operations, schemas, and examples during extraction.", normalizedHash),
    { name: "extract", state: "succeeded", inputHash: normalizedHash, outputHash: extractedHash, checkpoint: { operation_count: candidate.operations.length, schema_count: candidate.schemas.length, example_count: candidate.examples.length }, diagnostics: {} },
    { name: "map", state: "succeeded", inputHash: extractedHash, outputHash: mapHash, checkpoint: { map_version: API_CONTRACT_MAP_VERSION }, diagnostics: {} },
    skip("ai_enrich", "Deterministic contract output is reviewable without optional AI enrichment.", mapHash),
    { name: "quality_check", state: "succeeded", inputHash: extractedHash, outputHash: hashJSON(candidate.diagnostics), checkpoint: { diagnostic_count: candidate.diagnostics.length }, diagnostics: { items: candidate.diagnostics } },
    skip("build_index", "Published retrieval indexes are built from an approved Go-side publication.", mapHash),
    { name: "review", state: "succeeded", inputHash: mapHash, outputHash: mapHash, checkpoint: { state: "review_ready" }, diagnostics: {} },
  ];
}

async function persistOpenAPI(
  client: pg.PoolClient,
  job: Job,
  result: IngestionResult,
  legacyDocuments: ReadonlyMap<string, LegacyDocumentReference>,
  resolvedTarget?: { id: string; visibility: "private" | "public" } | null,
): Promise<CandidatePersistenceResult> {
  const target = resolvedTarget === undefined ? await resolveContractTarget(client, job) : resolvedTarget;
  if (!target) return {
    state: "pending_contract_target",
    diagnostics: [{
      code: "contract_catalog_target_pending",
      severity: "warning",
      message: "The OpenAPI source was acquired and kept in legacy review storage, but it must be bound to an API contract before a typed contract candidate can be created.",
      url: result.pages[0]?.url,
    }],
  };
  const targetId = target.id;
  const page = result.pages[0];
  if (!page) throw new CrawlerJobError("openapi_document_invalid", "The OpenAPI run has no acquired document.");
  let candidate: OpenAPICandidate;
  try {
    candidate = buildOpenAPICandidate(page.html, openAPIFormat(page));
  } catch (error) {
    throw new CrawlerJobError("openapi_document_invalid", "The OpenAPI document could not be normalized into a typed candidate.", { cause: error });
  }
  const existingCandidate = await client.query<{ id: string }>(`
    SELECT id::text
    FROM api_contract_candidates
    WHERE api_contract_id = $1 AND content_hash = $2
    LIMIT 1`, [targetId, prefixedHash(candidate.contentHash)]);
  if (existingCandidate.rows[0]) return {
    state: "unchanged_contract",
    diagnostics: [{
      code: "contract_candidate_unchanged",
      severity: "info",
      message: "The normalized OpenAPI contract is unchanged from an existing immutable candidate.",
      url: page.url,
    }],
  };

  const manifest = rawManifest(result, legacyDocuments);
  const manifestHash = hashJSON(manifest);
  const diagnostics = {
    partial: result.failedCount > 0 || result.skippedCount > 0,
    counts: {
      discovered: result.discoveredCount,
      acquired: result.pages.length,
      failed: result.failedCount,
      skipped: result.skippedCount,
      redirected: result.redirectedCount,
      quarantined: manifest.filter((item) => item.injection_indicators.length > 0).length,
    },
    acquisition: result.diagnostics,
    quality: candidate.diagnostics,
    processor_versions: OPENAPI_PROCESSOR_VERSIONS,
    contract_target_id: targetId,
    candidate_logical_id: candidate.logicalId,
  };
  const shape: RunShape = {
    assetKind: "contract",
    targetId,
    targetKey: `contract:${targetId}`,
    versions: OPENAPI_PROCESSOR_VERSIONS,
    rawManifest: manifest,
    rawManifestHash: manifestHash,
    diagnostics,
  };
  const runState = await ensureRunningRun(client, job, shape);
  if (runState === "complete") {
    const existing = await client.query<{ count: number }>(`
      SELECT count(*)::integer AS count
      FROM api_contract_candidates
      WHERE ingestion_run_id = $1 AND api_contract_id = $2`, [job.id, targetId]);
    if (existing.rows[0]?.count !== 1) throw new CrawlerJobError("developer_asset_run_incomplete", "The immutable review-ready contract run is missing its typed candidate.");
    return { state: "already_persisted", diagnostics: [] };
  }

  const candidateUUID = deterministicIngestionUUID("api-contract-candidate", `${targetId}:${candidate.logicalId}`);
  await client.query(`
    INSERT INTO api_contract_candidates(
      id, deployment_id, api_contract_id, ingestion_run_id, openapi_version,
      source_format, normalized_contract, source_hash, content_hash,
      validation_result, parser_version, visibility, diagnostics
    )
    VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10::jsonb,$11,$12,$13::jsonb)`, [
    candidateUUID,
    job.product_id,
    targetId,
    job.id,
    candidate.openapiVersion,
    candidate.sourceFormat,
    JSON.stringify(candidate.normalizedContract),
    prefixedHash(candidate.sourceHash),
    prefixedHash(candidate.contentHash),
    JSON.stringify({
      valid: !candidate.diagnostics.some((item) => item.severity === "error"),
      validator: "crawler-static-openapi-parser/1",
      diagnostics: candidate.diagnostics,
    }),
    OPENAPI_PROCESSOR_VERSIONS.parser,
    job.visibility === "public" && target.visibility === "public" ? "public" : "private",
    JSON.stringify({ logical_id: candidate.logicalId, source_url: page.url, items: candidate.diagnostics }),
  ]);

  const operationUUIDs = new Map<string, string>();
  for (const operation of candidate.operations) {
    const uuid = deterministicIngestionUUID("api-contract-operation", `${targetId}:${operation.logicalId}`);
    operationUUIDs.set(operation.logicalId, uuid);
    await client.query(`
      INSERT INTO api_contract_operations(
        id, deployment_id, api_contract_candidate_id, operation_key, operation_id,
        method, path_template, tags, summary, description, security,
        request_schema_refs, response_schema_refs, content_hash, ordinal
      )
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8::text[],$9,$10,$11::jsonb,$12::text[],$13::text[],$14,$15)`, [
      uuid,
      job.product_id,
      candidateUUID,
      operation.operationKey,
      operation.operationId,
      operation.method,
      operation.pathTemplate,
      operation.tags,
      operation.summary,
      operation.description,
      JSON.stringify(operation.security),
      operation.requestSchemaRefs,
      operation.responseSchemaRefs,
      prefixedHash(operation.contentHash),
      operation.ordinal,
    ]);
  }
  for (const schema of candidate.schemas) {
    await client.query(`
      INSERT INTO api_contract_schemas(
        id, deployment_id, api_contract_candidate_id, schema_key, schema_document, content_hash
      )
      VALUES ($1,$2,$3,$4,$5::jsonb,$6)`, [
      deterministicIngestionUUID("api-contract-schema", `${targetId}:${schema.logicalId}`),
      job.product_id,
      candidateUUID,
      schema.schemaKey,
      JSON.stringify(schema.schemaDocument),
      prefixedHash(schema.contentHash),
    ]);
  }
  for (const example of candidate.examples) {
    await client.query(`
      INSERT INTO api_contract_examples(
        id, deployment_id, api_contract_candidate_id, api_contract_operation_id,
        name, example_kind, media_type, status_code, value, content_hash
      )
      VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`, [
      deterministicIngestionUUID("api-contract-example", `${targetId}:${example.logicalId}`),
      job.product_id,
      candidateUUID,
      example.operationLogicalId ? operationUUIDs.get(example.operationLogicalId) ?? null : null,
      example.name,
      example.exampleKind,
      example.mediaType,
      example.statusCode,
      JSON.stringify(example.value),
      prefixedHash(example.contentHash),
    ]);
  }
  await client.query(`
    INSERT INTO api_contract_maps(
      id, deployment_id, api_contract_candidate_id, map_version,
      structured_map, agent_markdown, content_hash
    )
    VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7)`, [
    deterministicIngestionUUID("api-contract-map", `${targetId}:${candidate.logicalId}:${API_CONTRACT_MAP_VERSION}`),
    job.product_id,
    candidateUUID,
    API_CONTRACT_MAP_VERSION,
    JSON.stringify(candidate.structuredMap),
    candidate.agentMarkdown,
    prefixedHash(candidate.mapContentHash),
  ]);
  await insertStages(client, job, contractStages(job, manifestHash, candidate));
  await markReviewReady(client, job, diagnostics);
  return { state: "persisted", diagnostics: [] };
}

export async function persistDeveloperAssetCandidates(
  pool: pg.Pool,
  job: Job,
  result: IngestionResult,
  legacyDocuments: ReadonlyMap<string, LegacyDocumentReference>,
): Promise<CandidatePersistenceResult> {
  if (!["website", "upload", "openapi"].includes(job.source_kind)) return { state: "pending_contract_target", diagnostics: [] };
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await lockCurrentCrawlLease(client, job);
    let persisted: CandidatePersistenceResult;
    if (job.source_kind === "openapi") {
      persisted = await persistOpenAPI(client, job, result, legacyDocuments);
    } else if (job.source_kind === "upload") {
      // An explicit active contract binding is authoritative for an uploaded
      // OpenAPI file; ordinary unbound uploads remain documentation assets.
      const contractTarget = await resolveContractTarget(client, job);
      persisted = contractTarget
        ? await persistOpenAPI(client, job, result, legacyDocuments, contractTarget)
        : await persistDocumentation(client, job, result, legacyDocuments);
    } else {
      persisted = await persistDocumentation(client, job, result, legacyDocuments);
    }
    await assertCurrentCrawlLease(client, job);
    await client.query("COMMIT");
    return persisted;
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}
