"use client";


import { useTranslation } from "react-i18next";
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
  if (!sourceUploadExtensions.has(extension)) return "extension";
  if (file.size > sourceUploadMaxBytes) return "too-large";
  if (file.size === 0) return "empty";
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
  const { t } = useTranslation();
  const [addSourceOpen, setAddSourceOpen] = useState(false);
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
  const uploadValidationMessage = (file: File) => {
    const code = sourceUploadValidationError(file);
    return code === "extension" ? t("sourceWorkflow.invalidUploadExtension")
      : code === "too-large" ? t("sourceWorkflow.uploadTooLarge")
        : code === "empty" ? t("sourceWorkflow.uploadEmpty")
          : "";
  };

  function resetSourceForm() {
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
    setSourceFileError(file ? uploadValidationMessage(file) : t("sourceWorkflow.chooseAFileToUpload"));
  }

  async function createSource() {
    if (sourceKind === "upload" && !sourceFile) {
      setSourceFileError(t("sourceWorkflow.chooseAFileToUpload"));
      return;
    }
    setSourceBusy(true);
    try {
      if (sourceKind === "upload" && sourceFile) {
        const validationError = uploadValidationMessage(sourceFile);
        if (validationError) {
          setSourceFileError(validationError);
          return;
        }
        try {
          new TextDecoder("utf-8", { fatal: true }).decode(await sourceFile.arrayBuffer());
        } catch {
          setSourceFileError(t("sourceWorkflow.theSelectedFileIsNotValidUTFN8Text"));
          return;
        }
      }
      const created = apiConnected
        ? sourceKind === "upload" && sourceFile
          ? await api.uploadSource(product.id, product.organisation_id, sourceFile)
          : await api.createSource(product.id, product.organisation_id, sourceKind, sourceLocation.trim())
        : { id: `src_${Date.now()}`, name: sourceKind === "upload" ? sourceFile?.name ?? t("sourceWorkflow.uploadedFile") : sourceLocation.trim(), kind: sourceKind, location: sourceKind === "upload" ? sourceFile?.name ?? t("sourceWorkflow.uploadedFile") : sourceLocation.trim(), visibility: "private" as const, published: false, quarantined: false, revision: 1 };
      setSources((items) => [...items, { id: created.id, name: created.name, kind: created.kind, location: created.location, visibility: created.visibility, published: created.published, quarantined: created.quarantined, crawlState: "draft", pages: 0, lastCrawl: "not-crawled", revision: created.revision }]);
      setAddSourceOpen(false);
      resetSourceForm();
      showToast(t("sourceWorkflow.wasAddedPrivately", { name: String(created.name) }));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("sourceWorkflow.couldNotAddSource"));
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
      setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState: "queued", lastCrawl: "queued-now" } : item));
      showToast(t("sourceWorkflow.crawlQueuedTheIsolatedWorkerWillUpdateReviewState"));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : t("sourceWorkflow.couldNotQueueCrawl"));
    }
  }

  async function pollCrawl(id: string, attempt = 0) {
	try {
	  const jobs = await api.crawlJobs(product.id, id);
	  const latest = jobs[0];
	  if (!latest) return;
	  const crawlState: Source["crawlState"] = latest.state === "failed" ? "failed" : latest.state === "cancelled" ? "cancelled" : latest.state === "review" || latest.state === "succeeded" ? "review" : latest.state === "running" ? "running" : "queued";
	  setSources((items) => items.map((item) => item.id === id ? { ...item, crawlState, pages: latest.fetched_count, lastCrawl: latest.finished_at ? t("format.dateTime", { value: new Date(latest.finished_at) }) : latest.state } : item));
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
		description: t("sourceWorkflow.reviewedDocumentationDescription", { source: source.name, integration: integration.display_name }),
		manifest: [sourcePublicationManifestEntry(source, publication)],
	  });
	}
	const revisionID = resource.latest_revision?.id;
	if (!revisionID) throw new Error(t("sourceWorkflow.theReviewedDocumentationSetHasNoImmutableRevisionTo"));
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
	  showToast(t("sourceWorkflow.generationReviewIsAvailableInTheLiveConsole"));
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
	  showToast(error instanceof APIError ? error.message : t("sourceWorkflow.couldNotLoadThisCrawlGenerationForReview"));
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
	  let message: string = t("sourceWorkflow.generationPublished", { name: result.source.name, revision: result.publication.revision });
	  if (sourceReviewAttachIntegrationID) {
		try {
		  const attachment = await attachReviewedSourcePublication(sourceReviewAttachIntegrationID, source, result.publication);
		  message = attachment.attached
			? t("sourceWorkflow.generationPublishedAndPinned", { name: result.source.name, publicationRevision: result.publication.revision, attachmentRevision: attachment.revision })
			: t("sourceWorkflow.generationPublishedAlreadyAttached", { name: result.source.name, revision: result.publication.revision });
		} catch (error) {
		  message = t("sourceWorkflow.generationPublishedAttachmentNeedsAttention", { name: result.source.name, revision: result.publication.revision, error: error instanceof APIError || error instanceof Error ? error.message : t("sourceWorkflow.reviewedSetCouldNotBeAttached") });
		}
	  }
	  closeSourceReview();
	  showToast(message);
	} catch (error) {
	  showToast(error instanceof APIError ? error.message : t("sourceWorkflow.couldNotPublishThisReviewedGeneration"));
	} finally {
	  setSourceReviewBusy(false);
	}
  }

  return {
    addSourceOpen, setAddSourceOpen,
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
