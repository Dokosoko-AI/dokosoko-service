import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import type pg from "pg";
import {
  deterministicIngestionUUID,
  DOCUMENTATION_PROCESSOR_VERSIONS,
  persistedDocumentationMapBody,
  persistDeveloperAssetCandidates,
  type LegacyDocumentReference,
} from "./candidate-persistence";
import { buildDocumentationCorpus, buildOpenAPICandidate } from "./ingestion";
import { CrawlerJobError } from "./security";
import type { IngestionResult, Job, PageRecord } from "./sources";

type QueryCall = { sql: string; values: readonly unknown[] };

const persistedMapFixtures = JSON.parse(readFileSync(
  new URL("./fixtures/persistence-map-bodies.json", import.meta.url),
  "utf8",
)) as { documentation: unknown; contract: unknown };

function job(overrides: Partial<Job> = {}): Job {
  return {
    id: "00000000-0000-0000-0000-000000000001",
    organisation_id: "00000000-0000-0000-0000-000000000002",
    product_id: "00000000-0000-0000-0000-000000000003",
    source_id: "00000000-0000-0000-0000-000000000004",
    source_name: "Documentation",
    source_kind: "upload",
    location: "guide.md",
    visibility: "private",
    attempt: 1,
    lease_owner: "candidate-test-worker",
    lease_expires_at: new Date("2099-01-01T00:01:00Z"),
    heartbeat_at: new Date("2099-01-01T00:00:00Z"),
    ...overrides,
  };
}

function page(overrides: Partial<PageRecord> = {}): PageRecord {
  return {
    url: "upload://00000000-0000-0000-0000-000000000004/guide.md",
    title: "Guide",
    text: "Guide Hello world.",
    html: "# Guide\n\nHello world.",
    contentType: "text/markdown",
    status: 200,
    indicators: [],
    rendered: false,
    ...overrides,
  };
}

function ingestion(record: PageRecord): IngestionResult {
  return {
    pages: [record],
    discoveredCount: 1,
    failedCount: 0,
    skippedCount: 0,
    redirectedCount: 0,
    diagnostics: [],
  };
}

function legacy(record: PageRecord): ReadonlyMap<string, LegacyDocumentReference> {
  return new Map([[record.url, {
    knowledgeDocumentId: "00000000-0000-0000-0000-000000000010",
    snapshotId: "00000000-0000-0000-0000-000000000011",
  }]]);
}

function mockPool(options: {
  contractTarget?: { id: string; visibility: "private" | "public" } | null;
  leaseOwned?: boolean;
  runState?: "running" | "review_ready" | "published";
  runAttempt?: number;
} = {}): { calls: QueryCall[]; pool: pg.Pool } {
  const calls: QueryCall[] = [];
  let runValues: readonly unknown[] | null = null;
  const client = {
    query: async (sql: string, values: readonly unknown[] = []) => {
      calls.push({ sql, values });
      if (sql.includes("FROM crawl_jobs") && sql.includes("SELECT id::text") && !sql.includes("INSERT INTO")) {
        return { rows: options.leaseOwned === false ? [] : [{ id: job().id }], rowCount: options.leaseOwned === false ? 0 : 1 };
      }
      if (sql.includes("FROM api_contract_sources binding")) {
        const target = options.contractTarget;
        return target
          ? { rows: [{ api_contract_id: target.id, visibility: target.visibility }], rowCount: 1 }
          : { rows: [], rowCount: 0 };
      }
      if (sql.includes("FROM api_contract_candidates") && sql.includes("content_hash")) return { rows: [], rowCount: 0 };
      if (sql.includes("INSERT INTO developer_asset_ingestion_runs")) {
        runValues = values;
        return { rows: [], rowCount: 1 };
      }
      if (sql.includes("FROM developer_asset_ingestion_runs") && sql.includes("FOR UPDATE")) {
        assert.ok(runValues);
        return {
          rows: [{
            id: runValues[0],
            deployment_id: runValues[1],
            organisation_id: runValues[2],
            asset_kind: runValues[3],
            target_id: runValues[4],
            target_key: runValues[5],
            source_id: runValues[6],
            state: options.runState ?? "running",
            attempt: options.runAttempt ?? runValues[9],
            pipeline_version: runValues[10],
            parser_version: runValues[11],
            normalizer_version: runValues[12],
            mapper_version: runValues[13],
            raw_manifest_hash: runValues[15],
            resolved_source_hash: runValues[8],
          }],
          rowCount: 1,
        };
      }
      if (sql.includes("FROM documentation_documents WHERE ingestion_run_id")) {
        return { rows: [{ document_count: 1, section_count: 1, map_count: 1 }], rowCount: 1 };
      }
      return { rows: [], rowCount: sql.includes("UPDATE developer_asset_ingestion_runs") ? 1 : 0 };
    },
    release: () => undefined,
  };
  return { calls, pool: { connect: async () => client } as unknown as pg.Pool };
}

test("deterministic logical IDs map to stable RFC 4122 UUIDv5 values", () => {
  const first = deterministicIngestionUUID("documentation-section", "section_abc");
  assert.equal(first, deterministicIngestionUUID("documentation-section", "section_abc"));
  assert.notEqual(first, deterministicIngestionUUID("documentation-section", "section_def"));
  assert.notEqual(
    deterministicIngestionUUID("documentation-document", "run-a:document_same"),
    deterministicIngestionUUID("documentation-document", "run-b:document_same"),
    "record UUIDs remain deterministic without colliding across immutable ingestion runs",
  );
  assert.match(first, /^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});

test("persists a normalized documentation run, lineage, sections, map, stages, and review state under one lease", async () => {
  const value = job();
  const record = page();
  const { calls, pool } = mockPool();

  const persisted = await persistDeveloperAssetCandidates(pool, value, ingestion(record), legacy(record));
  assert.equal(persisted.state, "persisted");
  assert.equal(calls[0].sql, "BEGIN");
  assert.match(calls[1].sql, /FROM crawl_jobs[\s\S]*lease_owner = \$2[\s\S]*FOR UPDATE/);
  const run = calls.find((call) => call.sql.includes("INSERT INTO developer_asset_ingestion_runs"));
  assert.ok(run);
  assert.equal(run.values[3], "documentation");
  assert.equal(run.values[4], value.source_id);
  assert.equal(run.values[5], `source:${value.source_id}`);
  assert.equal(run.values[10], DOCUMENTATION_PROCESSOR_VERSIONS.pipeline);
  assert.match(String(run.values[15]), /^sha256:[0-9a-f]{64}$/);

  const document = calls.find((call) => call.sql.includes("INSERT INTO documentation_documents"));
  assert.ok(document);
  assert.equal(document.values[3], legacy(record).get(record.url)?.knowledgeDocumentId);
  assert.match(String(document.values[10]), /^sha256:[0-9a-f]{64}$/);
  const metadata = JSON.parse(String(document.values[13])) as Record<string, unknown>;
  assert.match(String(metadata.logical_id), /^document_/);
  assert.equal(metadata.logical_evidence_id, `document:${metadata.logical_id}`);
  assert.equal(metadata.evidence_id, `document:${document.values[0]}`);

  const section = calls.find((call) => call.sql.includes("INSERT INTO documentation_sections"));
  assert.ok(section);
  assert.match(String(section.values[15]), /^sha256:[0-9a-f]{64}$/);
  const sectionMetadata = JSON.parse(String(section.values[16])) as Record<string, unknown>;
  assert.equal(sectionMetadata.logical_evidence_id, `section:${sectionMetadata.logical_id}`);
  assert.equal(sectionMetadata.evidence_id, `section:${section.values[0]}`);

  const mapInsert = calls.find((call) => call.sql.includes("INSERT INTO documentation_maps"));
  assert.ok(mapInsert);
  const structuredMap = JSON.parse(String(mapInsert.values[4])) as {
    documents: Array<{ id: string; children?: Array<{ id: string }> }>;
    topics: Array<{ aliases?: string[] }>;
    workflows: Array<{ id: string }>;
  };
  assert.equal(structuredMap.documents[0]?.id, document.values[0]);
  assert.equal(structuredMap.documents[0]?.children?.[0]?.id, section.values[0]);
  assert.ok(structuredMap.topics.every((topic) => !topic.aliases?.some((alias) => /^document_/.test(alias))));
  assert.ok(structuredMap.workflows.every((entry) => entry.id === section.values[0] || !/^section_/.test(entry.id)));
  const agentMarkdown = String(mapInsert.values[5]);
  assert.ok(agentMarkdown.includes(`evidence \`document:${document.values[0]}\``));
  assert.ok(agentMarkdown.includes(`evidence \`section:${section.values[0]}\``));
  assert.doesNotMatch(JSON.stringify(structuredMap), /"(?:document|section)_[0-9a-f]+"/);
  assert.equal(calls.filter((call) => call.sql.includes("INSERT INTO developer_asset_ingestion_stages")).length, 11);
  assert.ok(calls.some((call) => /UPDATE developer_asset_ingestion_runs[\s\S]*state = 'review_ready'/.test(call.sql)));
  assert.equal(calls.at(-1)?.sql, "COMMIT");
});

test("an unbound OpenAPI source remains in legacy review with an explicit target diagnostic", async () => {
  const value = job({ source_kind: "openapi", location: "https://api.example.com/openapi.json" });
  const record = page({
    url: value.location,
    title: "Example API",
    contentType: "application/json",
    html: JSON.stringify({ openapi: "3.1.0", info: { title: "Example API", version: "1" }, paths: {} }),
  });
  const { calls, pool } = mockPool({ contractTarget: null });
  const persisted = await persistDeveloperAssetCandidates(pool, value, ingestion(record), legacy(record));

  assert.equal(persisted.state, "pending_contract_target");
  assert.equal(persisted.diagnostics[0]?.code, "contract_catalog_target_pending");
  assert.equal(calls.some((call) => call.sql.includes("INSERT INTO developer_asset_ingestion_runs")), false);
  assert.equal(calls.at(-1)?.sql, "COMMIT");
});

test("an exact immutable review-ready documentation run retries as a verification-only no-op", async () => {
  const value = job({ attempt: 2 });
  const record = page();
  const { calls, pool } = mockPool({ runState: "review_ready", runAttempt: 1 });

  const persisted = await persistDeveloperAssetCandidates(pool, value, ingestion(record), legacy(record));
  assert.equal(persisted.state, "already_persisted");
  assert.ok(calls.some((call) => call.sql.includes("FROM documentation_documents WHERE ingestion_run_id")));
  assert.equal(calls.some((call) => call.sql.includes("INSERT INTO documentation_documents")), false);
  assert.equal(calls.some((call) => call.sql.includes("INSERT INTO documentation_sections")), false);
  assert.equal(calls.some((call) => /UPDATE developer_asset_ingestion_runs[\s\S]*state = 'review_ready'/.test(call.sql)), false);
  assert.equal(calls.at(-1)?.sql, "COMMIT");
});

test("a bound OpenAPI source persists its exact contract candidate and typed map without inventing a catalog row", async () => {
  const contractId = "00000000-0000-0000-0000-000000000020";
  const value = job({ source_kind: "openapi", location: "https://api.example.com/openapi.json", visibility: "public" });
  const record = page({
    url: value.location,
    title: "Example API",
    contentType: "application/json",
    html: JSON.stringify({
      openapi: "3.1.0",
      info: { title: "Example API", version: "1" },
      paths: { "/widgets": { get: { operationId: "listWidgets", responses: { "200": { description: "OK" } } } } },
      components: { schemas: { Widget: { type: "object", properties: { id: { type: "string" } } } } },
    }),
  });
  const { calls, pool } = mockPool({ contractTarget: { id: contractId, visibility: "private" } });
  const persisted = await persistDeveloperAssetCandidates(pool, value, ingestion(record), legacy(record));

  assert.equal(persisted.state, "persisted");
  const lookup = calls.find((call) => call.sql.includes("FROM api_contract_sources binding"));
  assert.deepEqual(lookup?.values, [value.product_id, value.source_id]);
  assert.match(lookup?.sql ?? "", /binding\.lifecycle = 'attached'/);
  const run = calls.find((call) => call.sql.includes("INSERT INTO developer_asset_ingestion_runs"));
  assert.equal(run?.values[3], "contract");
  assert.equal(run?.values[4], contractId);
  assert.equal(calls.some((call) => call.sql.includes("INSERT INTO api_contracts")), false);
  const candidate = calls.find((call) => call.sql.includes("INSERT INTO api_contract_candidates"));
  assert.ok(candidate);
  assert.equal(candidate.values[2], contractId);
  assert.equal(candidate.values[11], "private", "candidate visibility cannot widen a private contract target");
  assert.ok(calls.some((call) => call.sql.includes("INSERT INTO api_contract_operations")));
  assert.ok(calls.some((call) => call.sql.includes("INSERT INTO api_contract_schemas")));
  assert.ok(calls.some((call) => call.sql.includes("INSERT INTO api_contract_maps")));
});

test("a stale lease rolls back before any typed candidate mutation", async () => {
  const value = job();
  const record = page();
  const { calls, pool } = mockPool({ leaseOwned: false });
  await assert.rejects(
    persistDeveloperAssetCandidates(pool, value, ingestion(record), legacy(record)),
    (error: unknown) => error instanceof CrawlerJobError && error.code === "crawler_lease_lost",
  );
  assert.equal(calls.some((call) => /INSERT INTO (?:developer_asset|documentation|api_contract)/.test(call.sql)), false);
  assert.equal(calls.at(-1)?.sql, "ROLLBACK");
});

test("OpenAPI candidate extraction is deterministic and preserves operation/schema evidence IDs", () => {
  const source = JSON.stringify({
    openapi: "3.1.0",
    info: { title: "Payments", version: "1" },
    paths: { "/charges": { post: { operationId: "createCharge", tags: ["Charges"], responses: { "201": { description: "Created" } } } } },
    components: { schemas: { Charge: { type: "object" } } },
  });
  const first = buildOpenAPICandidate(source, "json");
  const second = buildOpenAPICandidate(source, "json");
  assert.deepEqual(first, second);
  assert.equal(first.operations[0]?.operationKey, "POST /charges");
  assert.equal(first.schemas[0]?.schemaKey, "Charge");
  assert.match(JSON.stringify(first.structuredMap), /operation:operation_/);
  assert.match(first.agentMarkdown, /evidence `operation:operation_/);
});

test("persisted documentation and contract maps match the shared Go JSON fixtures", () => {
  const corpus = buildDocumentationCorpus([{
    content: "# Guide\n\n## Authentication\n\nUse a bearer token.\n\n## Examples\n\n```ts\nclient.list()\n```",
    source: {
      sourceId: "source-docs",
      canonicalUrl: "https://docs.example.com/guide",
      contentType: "text/markdown",
      sourceKind: "upload",
    },
    documentKind: "documentation",
  }]);
  assert.deepEqual(
    persistedDocumentationMapBody(corpus.map, corpus.diagnostics),
    persistedMapFixtures.documentation,
  );

  const contract = buildOpenAPICandidate(JSON.stringify({
    openapi: "3.1.0",
    info: { title: "Example", version: "1" },
    servers: [{ url: "https://api.example.com" }],
    paths: {
      "/widgets": {
        get: {
          operationId: "listWidgets",
          tags: ["Widgets"],
          security: [{ bearerAuth: [] }],
          responses: {
            "200": { description: "OK" },
            "401": { description: "Unauthorized" },
          },
        },
      },
    },
    components: {
      securitySchemes: { bearerAuth: { type: "http", scheme: "bearer" } },
      schemas: { Widget: { type: "object", description: "A widget." } },
    },
  }), "json");
  assert.deepEqual(contract.structuredMap, persistedMapFixtures.contract);
});
