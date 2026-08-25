"use client";

import { useRef, useState, type Dispatch, type SetStateAction } from "react";

import {
  APIError,
  api,
  type APIProduct,
  type APISourcePublication,
  type APISourceReview,
} from "../../lib/api";
import {
  integrationIncludesSourcePublication,
  manifestIncludesSourcePublication,
  sourcePublicationManifestEntry,
  type DocumentationAttachmentResult,
  type Source,
} from "./shared";

export type SourceKind = "website" | "openapi" | "git" | "upload";

const sourceUploadMaxBytes = 5_000_000;
const sourceUploadExtensions = new Set([".md", ".mdx", ".txt", ".html", ".htm", ".json", ".yaml", ".yml"]);
function sourceUploadValidationError(file: File) {
  const extension = file.name.slice(file.name.lastIndexOf(".")).toLowerCase();
  if (!sourceUploadExtensions.has(extension)) return "Choose a Markdown, text, HTML, JSON, or YAML file.";
  if (file.size > sourceUploadMaxBytes) return "The selected file is larger than the 5 MB limit for this setup.";
  if (file.size === 0) return "The selected file is empty.";
  return "";
}

export function useSourceWorkflow({ product, apiConnected, sources, setSources, refreshCatalog, showToast }: {
  product: APIProduct;
  apiConnected: boolean;
  sources: Source[];
  setSources: Dispatch<SetStateAction<Source[]>>;
  refreshCatalog: () => Promise<void>;
  showToast: (message: string) => void;
}) {
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [sourceName, setSourceName] = useState("");
  const [sourceKind, setSourceKind] = useState<SourceKind>("website");
  const [sourceLocation, setSourceLocation] = useState("");
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [sourceFileError, setSourceFileError] = useState("");
  const sourceFileInput = useRef<HTMLInputElement>(null);
  const [sourceBusy, setSourceBusy] = useState(false);
  const [sourceReview, setSourceReview] = useState<APISourceReview | null>(null);
  const [sourceReviewSelection, setSourceReviewSelection] = useState<string[]>([]);
  const [sourceReviewAcknowledged, setSourceReviewAcknowledged] = useState(false);
  const [sourceReviewBusy, setSourceReviewBusy] = useState(false);
  const [sourceReviewAttachIntegrationID, setSourceReviewAttachIntegrationID] = useState("");

  function resetSourceForm() {
    setSourceName("");
    setSourceKind("website");
    setSourceLocation("");
    setSourceFile(null);
    setSourceFileError("");
    if (sourceFileInput.current) sourceFileInput.current.value = "";
  }

  function closeSourceDialog(open: boolean) {
    if (!open && sourceBusy) return;
    setAddSourceOpen(open);
    if (!open && !sourceBusy) resetSourceForm();
  }

  function selectSourceKind(kind: SourceKind) {
    setSourceKind(kind);
    setSourceFileError("");
    if (kind !== "upload") {
      setSourceFile(null);
      if (sourceFileInput.current) sourceFileInput.current.value = "";
    }
  }

  function selectSourceFile(file: File | null) {
    setSourceFile(file);
    setSourceFileError(file ? sourceUploadValidationError(file) : "Choose a file to upload.");
    if (file && !sourceName.trim()) setSourceName(file.name.replace(/\.[^.]+$/, ""));
  }

  async function createSource() {
    if (sourceKind === "upload" && !sourceFile) {
      setSourceFileError("Choose a file to upload.");
      return;
    }
    setSourceBusy(true);
    try {
      if (sourceKind === "upload" && sourceFile) {
        const validationError = sourceUploadValidationError(sourceFile);
        if (validationError) {
          setSourceFileError(validationError);
          return;
        }
        try {
          new TextDecoder("utf-8", { fatal: true }).decode(await sourceFile.arrayBuffer());
        } catch {
          setSourceFileError("The selected file is not valid UTF-8 text.");
          return;
        }
      }
      const created = apiConnected
        ? sourceKind === "upload" && sourceFile
          ? await api.uploadSource(product.id, product.organisation_id, sourceName.trim(), sourceFile)
          : await api.createSource(product.id, product.organisation_id, sourceName.trim(), sourceKind, sourceLocation.trim())
        : { id: `src_${Date.now()}`, name: sourceName.trim(), kind: sourceKind, location: sourceKind === "upload" ? sourceFile?.name ?? "uploaded file" : sourceLocation.trim(), visibility: "private" as const, published: false, quarantined: false, revision: 1 };
      setSources((items) => [...items, { id: created.id, name: created.name, kind: created.kind, location: created.location, visibility: created.visibility, published: created.published, quarantined: created.quarantined, crawlState: "draft", pages: 0, lastCrawl: "Not crawled", revision: created.revision }]);
      setAddSourceOpen(false);
      resetSourceForm();
      showToast(`${created.name} was added privately.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not add source.");
    } finally {
      setSourceBusy(false);
    }
  }

  async function crawlSource(id: string) {
    try {
	  if (apiConnected) {
		await api.queueCrawl(product.id, id);
		window.setTimeout(() => pollCrawl(id), 1500);
	  }
      setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState: "queued", lastCrawl: "Queued now" } : item));
      showToast("Crawl queued. The isolated worker will update review state.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not queue crawl.");
    }
  }

  async function pollCrawl(id: string, attempt = 0) {
	try {
	  const jobs = await api.crawlJobs(product.id, id);
	  const latest = jobs[0];
	  if (!latest) return;
	  const crawlState: Source["crawlState"] = latest.state === "failed" ? "failed" : latest.state === "cancelled" ? "cancelled" : latest.state === "review" || latest.state === "succeeded" ? "review" : latest.state === "running" ? "running" : "queued";
	  setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState, pages: latest.fetched_count, lastCrawl: latest.finished_at ? new Date(latest.finished_at).toLocaleString() : latest.state } : item));
	  if ((latest.state === "queued" || latest.state === "running") && attempt < 40) {
		window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
		return;
	  }
	  if (latest.state === "review" || latest.state === "succeeded") {
		const refreshed = (await api.sources(product.id)).find((source) => source.id === id);
		const review = await api.sourceReview(product.id, id, latest.id).catch(() => null);
		if (refreshed) setSources((items) => items.map((item) => item.id === id ? { ...item, revision: refreshed.revision, published: refreshed.published, quarantined: refreshed.quarantined, crawlState: review?.publication ? "synced" : "review", latestPublication: review?.publication ?? item.latestPublication } : item));
	  }
	} catch {
	  if (attempt < 5) window.setTimeout(() => pollCrawl(id, attempt + 1), 3000);
	}
  }

  async function attachReviewedSourcePublication(integrationID: string, source: Source, publication: APISourcePublication): Promise<DocumentationAttachmentResult> {
	const [{ integration }, documentationSets] = await Promise.all([api.integration(integrationID), api.resourceSets("documentation")]);
	if (integrationIncludesSourcePublication(integration, publication.id)) {
	  const current = integration.resources?.find((link) => link.kind === "documentation" && manifestIncludesSourcePublication(link.resolved_revision?.manifest, publication.id));
	  return { attached: false, resourceSetName: current?.name ?? source.name, revision: current?.resolved_revision?.revision ?? publication.revision };
	}

	const attachedSetIDs = new Set(integration.resources?.map((link) => link.resource_set_id) ?? []);
	let resource = documentationSets.find((set) => !attachedSetIDs.has(set.id) && manifestIncludesSourcePublication(set.latest_revision?.manifest, publication.id));
	if (!resource) {
	  resource = await api.createResourceSet({
		kind: "documentation",
		name: `${integration.display_name} · ${source.name}`.slice(0, 120),
		description: `Reviewed ${source.name} documentation for ${integration.display_name}.`,
		manifest: [sourcePublicationManifestEntry(source, publication)],
	  });
	}
	const revisionID = resource.latest_revision?.id;
	if (!revisionID) throw new Error("The reviewed documentation set has no immutable revision to pin.");
	await api.attachResourceSet(integration.id, resource.id, revisionID);
	await refreshCatalog();
	return { attached: true, resourceSetName: resource.name, revision: resource.latest_revision?.revision ?? publication.revision };
  }

  function closeSourceReview() {
	setSourceReview(null);
	setSourceReviewSelection([]);
	setSourceReviewAcknowledged(false);
	setSourceReviewAttachIntegrationID("");
  }

  async function publishSource(source: Source, attachIntegrationID = "") {
	if (!apiConnected) {
	  showToast("Generation review is available in the live console.");
	  return;
	}
	setSourceReviewBusy(true);
	try {
	  const review = await api.sourceReview(product.id, source.id);
	  const safe = review.documents.filter((document) => (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0).map((document) => document.id);
	  setSourceReview(review);
	  setSourceReviewSelection(safe);
	  setSourceReviewAcknowledged(false);
	  setSourceReviewAttachIntegrationID(attachIntegrationID);
	} catch (error) {
	  setSourceReviewAttachIntegrationID("");
	  showToast(error instanceof APIError ? error.message : "Could not load this crawl generation for review.");
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  async function confirmSourcePublication() {
	if (!sourceReview || !sourceReviewAcknowledged || sourceReviewSelection.length === 0) return;
	setSourceReviewBusy(true);
	try {
	  const result = await api.publishSource(product.id, sourceReview.source.id, { revision: sourceReview.source.revision, crawl_job_id: sourceReview.crawl_job.id, document_ids: sourceReviewSelection, acknowledge_reviewed: true });
	  const source = sources.find((item) => item.id === result.source.id) ?? {
		id: result.source.id,
		name: result.source.name,
		kind: result.source.kind,
		location: result.source.location,
		visibility: result.source.visibility,
		published: result.source.published,
		quarantined: result.source.quarantined,
		crawlState: "synced" as const,
		pages: result.publication.document_count,
		lastCrawl: result.publication.published_at,
		revision: result.source.revision,
	  };
	  setSources((items) => items.map((item) => item.id === result.source.id ? { ...item, published: result.source.published, quarantined: result.source.quarantined, revision: result.source.revision, crawlState: "synced", latestPublication: result.publication } : item));
	  let message = `${result.source.name} generation r${result.publication.revision} was atomically published.`;
	  if (sourceReviewAttachIntegrationID) {
		try {
		  const attachment = await attachReviewedSourcePublication(sourceReviewAttachIntegrationID, source, result.publication);
		  message = attachment.attached
			? `${message} Revision ${attachment.revision} was pinned to the API.`
			: `${message} That exact revision was already attached.`;
		} catch (error) {
		  message = `${message} Attachment still needs attention: ${error instanceof APIError || error instanceof Error ? error.message : "the reviewed set could not be attached"}`;
		}
	  }
	  closeSourceReview();
	  showToast(message);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : "Could not publish this reviewed generation.");
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  return {
    addSourceOpen, setAddSourceOpen,
    sourceName, setSourceName,
    sourceKind,
    sourceLocation, setSourceLocation,
    sourceFile,
    sourceFileError,
    sourceFileInput,
    sourceBusy,
    sourceReview,
    sourceReviewSelection, setSourceReviewSelection,
    sourceReviewAcknowledged, setSourceReviewAcknowledged,
    sourceReviewBusy,
    sourceReviewAttachIntegrationID,
    closeSourceDialog,
    selectSourceKind,
    selectSourceFile,
    createSource,
    crawlSource,
    attachReviewedSourcePublication,
    publishSource,
    closeSourceReview,
    confirmSourcePublication,
  };
}
