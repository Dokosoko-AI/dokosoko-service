import type { APICrawlJob, APIIntegration, APISourcePublication, APISourceReview } from "./api";
import { sectionPath } from "./console-routes";
import type { APIDocumentationBinding, APIResourceBindings, DocumentationCollection, DocumentationCollectionInput, DocumentationCollectionRevision } from "./developer-assets-api";

const fields = ["setup", "api", "source", "run", "queue", "after"] as const;
export type DocumentationSetupSelection = Record<typeof fields[number], string>;
export function parseDocumentationSetupSelection(search = ""): DocumentationSetupSelection {
  const params = new URLSearchParams(search);
  return Object.fromEntries(fields.map((key) => [key, (params.get(key) ?? "").trim()])) as DocumentationSetupSelection;
}
export function documentationSetupPath(selection: Partial<DocumentationSetupSelection>) {
  const params = new URLSearchParams({ setup: "content" });
  for (const key of fields) if (selection[key]) params.set(key, selection[key]!);
  return `${sectionPath("documents")}?${params}`;
}
export function validDocumentationProgress(value: unknown): value is DocumentationSetupSelection {
  return !!value && typeof value === "object" && fields.every((key) => typeof (value as Record<string, unknown>)[key] === "string" && String((value as Record<string, unknown>)[key]).length <= 200);
}
export class DocumentationSetupChangedError extends Error {
  constructor() { super("documentation_setup_changed"); }
}
export function pendingDocumentationImport(jobs: APICrawlJob[], selection: Pick<DocumentationSetupSelection, "queue" | "after">) {
  if (selection.queue !== "pending") return undefined;
  const count = selection.after ? jobs.findIndex((job) => job.id === selection.after) : jobs.length;
  // More than one new import is ambiguous. Ask the operator to select one.
  if (count < 0 || count > 1) throw new DocumentationSetupChangedError();
  return count === 1 ? jobs[0] : undefined;
}
export function documentationReviewFingerprint(review: APISourceReview) {
  return JSON.stringify([review.source.id, review.source.visibility, review.source.quarantined, review.crawl_job.id,
    review.crawl_job.failed_count, review.crawl_job.skipped_count,
    review.documents.map((document) => [document.id, document.content_hash, document.state === "published" ? "validated" : document.state, document.injection_indicators]).sort((a, b) => String(a[0]).localeCompare(String(b[0])))]);
}
export function documentationSourceSlug(sourceID: string) { return `source-${sourceID}`; }
const emptySelector = (value: unknown) => !!value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).length === 0;
export function documentationRevisionMatches(revision: DocumentationCollectionRevision, publication: APISourcePublication) {
  const members = revision.selection_manifest;
  return revision.deployment_id === publication.product_id && revision.visibility === publication.visibility && members.length === 1 &&
    members[0].kind === "source_publication" && members[0].evidence_id === `source-publication:${publication.id}` &&
    members[0].content_hash === publication.content_hash && members[0].include_descendants === true && emptySelector(members[0].selector);
}
export function documentationBindingMatches(binding: APIDocumentationBinding | undefined, revision: DocumentationCollectionRevision, visibility: APIIntegration["visibility"]) {
  return binding?.lifecycle === "attached" && binding.documentation_collection_id === revision.documentation_collection_id && binding.pinned_revision_id === revision.id && !binding.follow_latest && binding.visibility === visibility && emptySelector(binding.selector);
}

type DocumentationSetupTransport = {
  review: (deploymentID: string, sourceID: string, runID: string) => Promise<APISourceReview>;
  publishSource: (review: APISourceReview, ids: string[]) => Promise<unknown>;
  collections: () => Promise<DocumentationCollection[]>;
  revisions: (id: string) => Promise<DocumentationCollectionRevision[]>;
  create: (input: DocumentationCollectionInput) => Promise<DocumentationCollection>;
  revise: (id: string, input: DocumentationCollectionInput) => Promise<DocumentationCollection>;
  integration: (id: string) => Promise<APIIntegration>;
  resources: (id: string) => Promise<APIResourceBindings>;
  attach: (id: string, collectionID: string, revisionID: string, visibility: APIIntegration["visibility"]) => Promise<unknown>;
  change: (id: string, binding: APIDocumentationBinding, revisionID: string) => Promise<unknown>;
};

// Each step recovers an exact committed result before considering a write.
// The stable source-derived collection slug avoids a new collection per import.
// Existing API publications remain untouched; attachment changes the draft only.
export async function finishDocumentationSetup(input: {
  review: APISourceReview; selected: string[]; collection?: DocumentationCollection;
  origin?: APIIntegration; expectedBinding?: APIDocumentationBinding;
}, transport: DocumentationSetupTransport) {
  const review = await transport.review(input.review.source.product_id, input.review.source.id, input.review.crawl_job.id);
  if (review.source.product_id !== input.review.source.product_id || documentationReviewFingerprint(review) !== documentationReviewFingerprint(input.review) || review.source.quarantined || !["review", "succeeded"].includes(review.crawl_job.state) || review.crawl_job.failed_count || review.crawl_job.skipped_count) throw new DocumentationSetupChangedError();
  const selected = review.documents.filter((document) => input.selected.includes(document.id));
  if (!selected.length || selected.length !== input.selected.length || selected.some((document) => !["validated", "published"].includes(document.state) || document.injection_indicators.length)) throw new DocumentationSetupChangedError();
  if (input.origin) {
    const origin = await transport.integration(input.origin.id);
    if (origin.id !== input.origin.id || origin.deployment_id !== review.source.product_id || origin.visibility !== input.origin.visibility || origin.visibility === "public" && review.source.visibility !== "public") throw new DocumentationSetupChangedError();
  }
  let current = review;
  if (!current.publication) {
    if (current.source.revision !== input.review.source.revision) throw new DocumentationSetupChangedError();
    await transport.publishSource(current, selected.map((document) => document.id));
    current = await transport.review(current.source.product_id, current.source.id, current.crawl_job.id);
  }
  const publication = current.publication;
  const included = current.published_document_ids;
  if (!publication || publication.product_id !== review.source.product_id || publication.source_id !== review.source.id || publication.crawl_job_id !== review.crawl_job.id || publication.visibility !== review.source.visibility || !included || included.length !== selected.length || selected.some((document) => !included.includes(document.id))) throw new DocumentationSetupChangedError();
  const slug = documentationSourceSlug(review.source.id);
  let collection = (await transport.collections()).find((value) => value.slug === slug);
  if (collection && (collection.deployment_id !== review.source.product_id || collection.lifecycle !== "active")) throw new DocumentationSetupChangedError();
  const findExact = async (value: DocumentationCollection) => (await transport.revisions(value.id)).find((revision) => revision.documentation_collection_id === value.id && documentationRevisionMatches(revision, publication));
  let revision = collection ? await findExact(collection) : undefined;
  if (!revision) {
    if (collection?.id !== input.collection?.id || collection?.revision !== input.collection?.revision) throw new DocumentationSetupChangedError();
    const content: DocumentationCollectionInput = {
      name: collection?.name ?? review.source.name, slug, description: collection?.description ?? "",
      visibility: publication.visibility, lifecycle: "active", revision: collection?.revision,
      members: [{ kind: "source_publication", id: publication.id, include_descendants: true, selector: {} }], acknowledge_reviewed: true,
    };
    collection = collection ? await transport.revise(collection.id, content) : await transport.create(content);
    revision = await findExact(collection);
  }
  if (!collection || !revision) throw new DocumentationSetupChangedError();
  if (input.origin) {
    const origin = input.origin;
    const resources = await transport.resources(origin.id);
    const existing = resources.documentation.find((binding) => binding.lifecycle === "attached" && binding.documentation_collection_id === collection.id);
    const visibility = input.expectedBinding?.visibility ?? origin.visibility;
    if (documentationBindingMatches(existing, revision, visibility)) return { collection, revision };
    if (existing?.id !== input.expectedBinding?.id || existing?.revision !== input.expectedBinding?.revision || existing && (!emptySelector(existing.selector) || existing.follow_latest)) throw new DocumentationSetupChangedError();
    if (existing) await transport.change(origin.id, existing, revision.id);
    else await transport.attach(origin.id, collection.id, revision.id, visibility);
  }
  return { collection, revision };
}
