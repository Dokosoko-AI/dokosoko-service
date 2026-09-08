import assert from "node:assert/strict";
import test from "node:test";
import type { APICrawlJob, APIIntegration, APISourcePublication, APISourceReview } from "../app/lib/api";
import type { APIDocumentationBinding, APIResourceBindings, DocumentationCollection, DocumentationCollectionInput, DocumentationCollectionRevision } from "../app/lib/developer-assets-api";
import { documentationBindingMatches, documentationReviewFingerprint, documentationSetupPath, documentationSourceSlug, DocumentationSetupChangedError, finishDocumentationSetup, parseDocumentationSetupSelection, pendingDocumentationImport } from "../app/lib/documentation-setup";
import { mergeSourceMetadata, type Source } from "../app/lib/console-domain";

function fixture() {
  const review = { source: { id: "source", name: "Guide", product_id: "deployment", visibility: "private", quarantined: false, revision: 1 }, crawl_job: { id: "run", state: "review", failed_count: 0, skipped_count: 0 }, documents: [{ id: "doc", content_hash: "text", state: "validated", injection_indicators: [] }, { id: "omitted", content_hash: "other", state: "validated", injection_indicators: [] }] } as unknown as APISourceReview;
  const origin = { id: "api", deployment_id: "deployment", visibility: "private" } as APIIntegration;
  const state = { review: structuredClone(review), origin: structuredClone(origin), collections: [] as DocumentationCollection[], revisions: [] as DocumentationCollectionRevision[], resources: { documentation: [], contracts: [], sdks: [] } as APIResourceBindings, calls: { publish: 0, create: 0, revise: 0, attach: 0, change: 0 } };
  function saveRevision(content: DocumentationCollectionInput, root: DocumentationCollection) {
    const publication = state.review.publication!;
    const revision: DocumentationCollectionRevision = { documentation_collection_name: root.name, documentation_collection_slug: root.slug, documentation_collection_description: root.description, content_hash: `collection-hash-${state.revisions.length + 1}`, reviewed_by: "reviewer", reviewed_at: "2026-09-07T00:00:00Z", published_at: "2026-09-07T00:00:00Z", id: `revision-${state.revisions.length + 1}`, deployment_id: root.deployment_id, documentation_collection_id: root.id, revision: root.revision, visibility: content.visibility ?? "private", selection_manifest: [{ kind: "source_publication", evidence_id: `source-publication:${publication.id}`, content_hash: publication.content_hash, include_descendants: true, selector: {} }] };
    state.revisions.unshift(revision);
  }
  const transport = {
    review: async () => state.review,
    publishSource: async (_review: APISourceReview, ids: string[]) => {
      state.calls.publish++; state.review.source.revision++;
      state.review.publication = { id: `publication-${state.calls.publish}`, product_id: "deployment", source_id: "source", crawl_job_id: state.review.crawl_job.id, visibility: state.review.source.visibility, content_hash: `hash-${state.calls.publish}` } as APISourcePublication;
      state.review.published_document_ids = ids;
      state.review.documents.forEach((document) => { if (ids.includes(document.id)) document.state = "published"; });
    },
    collections: async () => state.collections,
    revisions: async () => state.revisions,
    create: async (input: DocumentationCollectionInput) => {
      state.calls.create++;
      const root: DocumentationCollection = { ...input, id: "collection", deployment_id: "deployment", organisation_id: "organisation", revision: 1, visibility: input.visibility ?? "private", description: input.description ?? "", created_at: "2026-09-07T00:00:00Z", updated_at: "2026-09-07T00:00:00Z" };
      state.collections = [root]; saveRevision(input, root); return root;
    },
    revise: async (_id: string, input: DocumentationCollectionInput) => {
      state.calls.revise++;
      const root = { ...state.collections[0], ...input, revision: (input.revision ?? 0) + 1 } as DocumentationCollection;
      state.collections = [root]; saveRevision(input, root); return root;
    },
    integration: async () => state.origin,
    resources: async () => state.resources,
    attach: async (_id: string, collectionID: string, revisionID: string, visibility: APIIntegration["visibility"]) => {
      state.calls.attach++;
      state.resources.documentation = [{ id: "binding", documentation_collection_id: collectionID, pinned_revision_id: revisionID, visibility, lifecycle: "attached", follow_latest: false, selector: {}, revision: 1 } as APIDocumentationBinding];
    },
    change: async (_id: string, binding: APIDocumentationBinding, revisionID: string) => {
      state.calls.change++; state.resources.documentation = [{ ...binding, revision: binding.revision + 1, pinned_revision_id: revisionID }];
    },
  };
  const input = { review, selected: ["doc"], origin };
  return { state, input, transport };
}

test("documentation setup retains exact URL context and rejects ambiguous queue recovery", () => {
  const selection = parseDocumentationSetupSelection("?setup=content&api=api&source=source&queue=pending&after=old");
  assert.deepEqual(parseDocumentationSetupSelection(documentationSetupPath(selection).split("?")[1]), selection);
  const job = { id: "new", state: "review" } as APICrawlJob;
  assert.equal(pendingDocumentationImport([job, { id: "old" } as APICrawlJob], selection), job);
  assert.throws(() => pendingDocumentationImport([job, { id: "other" } as APICrawlJob, { id: "old" } as APICrawlJob], selection), DocumentationSetupChangedError);
  assert.throws(() => pendingDocumentationImport([job], selection), DocumentationSetupChangedError);
});

test("documentation setup publishes only selected content and attaches its exact typed revision", async () => {
  const { state, input, transport } = fixture();
  const result = await finishDocumentationSetup(input, transport);
  assert.deepEqual(state.review.published_document_ids, ["doc"]);
  assert.equal(result.collection.slug, documentationSourceSlug("source"));
  assert.equal(documentationBindingMatches(state.resources.documentation[0], result.revision, "private"), true);
  assert.deepEqual(state.calls, { publish: 1, create: 1, revise: 0, attach: 1, change: 0 });
});

for (const stage of ["publishSource", "create", "attach"] as const) test(`documentation setup recovers a lost ${stage} response without repeating its write`, async () => {
  const { state, input, transport } = fixture();
  const failing = { ...transport, [stage]: async (...args: unknown[]) => {
    await (transport[stage] as (...values: unknown[]) => Promise<unknown>)(...args);
    throw new Error("response lost after commit");
  } };
  await assert.rejects(finishDocumentationSetup(input, failing), /response lost/);
  await finishDocumentationSetup(input, transport);
  assert.deepEqual(state.calls, { publish: 1, create: 1, revise: 0, attach: 1, change: 0 });
});

test("a later import advances the same documentation collection and leaves earlier revisions intact", async () => {
  const { state, input, transport } = fixture();
  const first = await finishDocumentationSetup(input, transport);
  const original = structuredClone(first.revision);
  state.review = structuredClone(input.review);
  state.review.crawl_job.id = "later-run";
  state.review.source.revision = 3;
  state.review.documents[0].content_hash = "updated text";
  const review = structuredClone(state.review);
  const next = await finishDocumentationSetup({ ...input, review, collection: first.collection, expectedBinding: state.resources.documentation[0] }, transport);
  assert.equal(next.collection.id, first.collection.id);
  assert.equal(next.revision.revision, 2);
  assert.deepEqual(state.revisions.find((revision) => revision.id === original.id), original);
  assert.deepEqual(state.calls, { publish: 2, create: 1, revise: 1, attach: 1, change: 1 });
});

test("concurrent attachment replacement and selectors require renewed review", async () => {
  for (const mutation of ["revision", "selector"] as const) {
    const { state, input, transport } = fixture();
    await finishDocumentationSetup(input, transport);
    const expectedBinding = structuredClone(state.resources.documentation[0]);
    state.resources.documentation[0] = { ...expectedBinding, pinned_revision_id: "other-version", ...(mutation === "revision" ? { revision: 2 } : { selector: { document_ids: ["other"] } }) };
    await assert.rejects(finishDocumentationSetup({ ...input, expectedBinding }, transport), DocumentationSetupChangedError);
    assert.equal(state.calls.change, 0);
  }
});

test("changed review, different published decisions and public audience mismatch fail closed", async () => {
  for (const mutation of ["content", "audience", "quarantine", "partial", "decisions"] as const) {
    const { state, input, transport } = fixture();
    if (mutation === "content") state.review.documents[0].content_hash = "changed";
    if (mutation === "audience") state.origin.visibility = "public";
    if (mutation === "quarantine") state.review.source.quarantined = true;
    if (mutation === "partial") state.review.crawl_job.skipped_count = 1;
    if (mutation === "decisions") await transport.publishSource(state.review, ["omitted"]);
    await assert.rejects(finishDocumentationSetup(input, transport), DocumentationSetupChangedError);
    assert.equal(state.calls.create, 0); assert.equal(state.calls.attach, 0);
  }
});

test("source publication transition preserves the review fingerprint and loaded source metadata", async () => {
  const { state, input, transport } = fixture();
  const before = documentationReviewFingerprint(input.review);
  await transport.publishSource(state.review, ["doc"]);
  assert.equal(documentationReviewFingerprint(state.review), before);
  const local = { ...state.review.source, crawlState: "running", pages: 25, lastCrawl: "yesterday" } as Source;
  const values = mergeSourceMetadata([local], { ...local, visibility: "public", revision: 8 });
  assert.equal(values[0].crawlState, "running"); assert.equal(values[0].pages, 25);
  assert.equal(values[0].visibility, "public");
  assert.equal(mergeSourceMetadata(values, local), values, "stale reads do not roll back newer metadata");
});
