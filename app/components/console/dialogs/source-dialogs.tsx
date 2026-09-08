"use client";


import { AIReadinessPanel } from "../ai-readiness-panel";
import { useState } from "react";
import { SourceReviewDocument } from "../developer-assets/source-review-document";
import { KnowledgeProcessingPanel } from "../developer-assets/knowledge-processing-panel";
import { useTranslation } from "react-i18next";
import { Check, FileUp, Globe2, LockKeyhole, ShieldCheck } from "lucide-react";

import { Badge, Button, Dialog } from "../../core/control";
import { Confirmation } from "../shared";
import { type SourceKind, type useSourceWorkflow } from "../use-source-workflow";

export function SourceDialogs({ workspace }: {
  workspace: ReturnType<typeof useSourceWorkflow>;
}) {
  const { t } = useTranslation();
  const [processingReadyRunID, setProcessingReadyRunID] = useState("");
  const {
    addSourceOpen,
    sourceKind,
    sourceLocation, setSourceLocation,
    sourceFile,
    sourceFileError,
    sourceFileInput,
    sourceBusy,
    sourceCreateProblem, sourceRecoveryUnavailable,
    sourceReview,
    sourceReviewSelection, setSourceReviewSelection,
    sourceReviewAcknowledged, setSourceReviewAcknowledged,
    sourceReviewBusy, sourceReviewProblem,
    sourceReviewAttachIntegrationID,
    closeSourceDialog,
    selectSourceKind,
    selectSourceFile,
    createSource,
    closeSourceReview,
    confirmSourcePublication,
  } = workspace;

  return <>
    <Dialog
      open={addSourceOpen}
      onClose={closeSourceDialog}
      title={t("sourceDialogs.addKnowledgeSource")}
      description={t("sourceDialogs.addAURLBackedSourceOrUploadOneText")}
      actions={<><Button outline disabled={sourceBusy} onClick={() => closeSourceDialog(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={sourceBusy || (sourceKind === "upload" ? !sourceFile || Boolean(sourceFileError) : !sourceLocation.trim())} onClick={createSource}>{sourceBusy ? t("sourceDialogs.adding") : sourceKind === "upload" ? t("sourceDialogs.uploadSource") : t("sourceDialogs.addSource")}</Button></>}
    >
      {addSourceOpen && <AIReadinessPanel preserveForm compact />}
      {sourceCreateProblem && <p className="auth-problem" role="status" aria-live="polite">{sourceCreateProblem}</p>}
      {sourceRecoveryUnavailable && <p role="status">{t("sourceCreation.recoveryUnavailable")}</p>}
      <div className="auth-form compact-form source-form">
        <fieldset className="source-kind-selector">
          <legend>{t("sourceDialogs.type")}</legend>
          <div className="source-kind-options documentation-source-kinds">
            {([
              { kind: "website" as const, icon: <Globe2 />, label: t("sourceDialogs.website") },
              { kind: "upload" as const, icon: <FileUp />, label: t("sourceDialogs.uploadAFile") },
            ]).map((option) => <label className={sourceKind === option.kind ? "selected" : ""} key={option.kind}>
              <input type="radio" name="source-kind" value={option.kind} checked={sourceKind === option.kind} disabled={sourceBusy} onChange={() => selectSourceKind(option.kind as SourceKind)} />
              <span className="source-kind-icon">{option.icon}</span>
              <strong>{option.label}</strong>
              <Check className="source-kind-check" />
            </label>)}
          </div>
        </fieldset>
        {sourceKind === "upload" ? <label className="auth-field"><span>{t("sourceDialogs.file")}</span><input ref={sourceFileInput} type="file" disabled={sourceBusy} accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml,text/plain,text/markdown,text/html,application/json,application/yaml,text/yaml" aria-invalid={Boolean(sourceFileError)} aria-describedby={`source-upload-guidance${sourceFileError ? " source-upload-error" : ""}`} onChange={(event) => selectSourceFile(event.target.files?.[0] ?? null)} /><small id="source-upload-guidance">{t("sourceDialogs.utfN8MdMdxTxtHtmlHtmJsonYaml")}</small>{sourceFileError && <small id="source-upload-error" className="source-upload-error" role="alert">{sourceFileError}</small>}</label> : <label className="auth-field"><span>{t("sourceDialogs.location")}</span><input type="url" disabled={sourceBusy} value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder="https://example.com/docs" aria-describedby="source-website-boundary" /><small id="source-website-boundary">{t("sourceDialogs.websitePathBoundary")}</small></label>}
      </div>
    </Dialog>

    <Dialog
      open={Boolean(sourceReview)}
      onClose={(open) => { if (!open && !sourceReviewBusy) closeSourceReview(); }}
      title={t("sourceDialogs.review", { value1: String(sourceReview?.source.name ?? t("navigation.docs")) })}
      description={sourceReviewAttachIntegrationID ? t("sourceDialogs.approveTheExactCrawlGenerationDokoSokoWillPublishThe") : t("sourceDialogs.approveTheExactCompletedCrawlGenerationAndOnlyThe")}
      actions={<><Button outline disabled={sourceReviewBusy} onClick={closeSourceReview}>{t(sourceReview?.publication ? "common.close" : "common.cancel")}</Button>{(!sourceReview?.publication || sourceReviewAttachIntegrationID) && <Button color="indigo" disabled={sourceReviewBusy || Boolean(sourceReview?.source.quarantined) || (!sourceReview?.publication && (!sourceReviewAcknowledged || sourceReviewSelection.length === 0 || processingReadyRunID !== sourceReview?.crawl_job.id))} onClick={confirmSourcePublication}>{sourceReviewBusy ? t("sourceDialogs.publishing") : sourceReview?.publication && sourceReviewAttachIntegrationID ? t("sourceDialogs.retryAttachment") : sourceReviewAttachIntegrationID ? t("sourceDialogs.publishAttach") : t("sourceDialogs.publishReviewedGeneration")}</Button>}</>}
    >
      {sourceReview && <div className="mcp-import-review">
        {sourceReviewProblem && <p className="auth-problem" role="alert">{sourceReviewProblem}</p>}
        {!sourceReview.publication && <><KnowledgeProcessingPanel key={sourceReview.crawl_job.id} runID={sourceReview.crawl_job.id} onReady={setProcessingReadyRunID} /><p>{t("sdkCatalog.reviewDraftDescription")}</p></>}
        {sourceReviewAttachIntegrationID && <div className="private-default-note"><LockKeyhole />{t("sourceDialogs.onlyTheReviewedPublicationIsAttachedAndTheAPI")}</div>}
        <div className="import-summary"><Badge color={sourceReview.publication ? "green" : "amber"}>{sourceReview.publication ? t("sourceDialogs.publishedR", { revision: String(sourceReview.publication.revision) }) : t("sourceDialogs.needsReview")}</Badge><Badge color="zinc">{sourceReview.publication?.visibility ?? sourceReview.source.visibility}</Badge><span>{t("sourceDialogs.fetchedAndChanged", { fetched: sourceReview.documents.length, changed: sourceReview.crawl_job.changed_count })}</span></div>
        <div className="source-review-documents">{sourceReview.documents.map((document) => <SourceReviewDocument
          key={`${sourceReview.crawl_job.id}:${document.id}:${document.content_hash}`}
          productID={sourceReview.source.product_id} sourceID={sourceReview.source.id} document={document}
          selected={sourceReviewSelection.includes(document.id)} disabled={sourceReviewBusy || Boolean(sourceReview.source.quarantined)}
          publishedDecision={sourceReview.publication ? sourceReview.published_document_ids ? sourceReview.published_document_ids.includes(document.id) ? "included" : "excluded" : "unknown" : undefined}
          onSelected={(selected) => { setSourceReviewAcknowledged(false); setSourceReviewSelection((items) => selected ? [...items, document.id] : items.filter((id) => id !== document.id)); }}
        />)}</div>
        {!sourceReview.publication && <Confirmation checked={sourceReviewAcknowledged} onChange={setSourceReviewAcknowledged}>{t("sourceDialogs.iReviewedGeneration")} {sourceReview.crawl_job.id}{t("sourceDialogs.itsChangedAndUnchangedPagesAndTheExact")} {sourceReviewSelection.length} {t("sourceDialogs.selectedDocument")}{sourceReviewSelection.length === 1 ? "" : t("sourceDialogs.s")}.</Confirmation>}
        <div className="private-default-note"><ShieldCheck />{t("sourceDialogs.publishingCreatesAnImmutableSourcePublicationIDAndContent")}</div>
      </div>}
    </Dialog>
  </>;
}
