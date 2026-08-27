"use client";


import { useTranslation } from "react-i18next";
import { Check, FileUp, Globe2, LockKeyhole, ShieldCheck } from "lucide-react";

import { Badge, Button, Dialog } from "../../core/control";
import { Confirmation } from "../shared";
import { type SourceKind, type useSourceWorkflow } from "../use-source-workflow";

export function SourceDialogs({ workspace }: {
  workspace: ReturnType<typeof useSourceWorkflow>;
}) {
  const { t } = useTranslation();
  const {
    addSourceOpen,
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
      <div className="auth-form compact-form source-form">
        <fieldset className="source-kind-selector">
          <legend>{t("sourceDialogs.type")}</legend>
          <div className="source-kind-options documentation-source-kinds">
            {([
              { kind: "website" as const, icon: <Globe2 />, label: t("sourceDialogs.website") },
              { kind: "upload" as const, icon: <FileUp />, label: t("sourceDialogs.uploadAFile") },
            ]).map((option) => <label className={sourceKind === option.kind ? "selected" : ""} key={option.kind}>
              <input type="radio" name="source-kind" value={option.kind} checked={sourceKind === option.kind} onChange={() => selectSourceKind(option.kind as SourceKind)} />
              <span className="source-kind-icon">{option.icon}</span>
              <strong>{option.label}</strong>
              <Check className="source-kind-check" />
            </label>)}
          </div>
        </fieldset>
        {sourceKind === "upload" ? <label className="auth-field"><span>{t("sourceDialogs.file")}</span><input ref={sourceFileInput} type="file" accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml,text/plain,text/markdown,text/html,application/json,application/yaml,text/yaml" aria-invalid={Boolean(sourceFileError)} aria-describedby={`source-upload-guidance${sourceFileError ? " source-upload-error" : ""}`} onChange={(event) => selectSourceFile(event.target.files?.[0] ?? null)} /><small id="source-upload-guidance">{t("sourceDialogs.utfN8MdMdxTxtHtmlHtmJsonYaml")}</small>{sourceFileError && <small id="source-upload-error" className="source-upload-error" role="alert">{sourceFileError}</small>}</label> : <label className="auth-field"><span>{t("sourceDialogs.location")}</span><input type="url" value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder="https://example.com/docs" aria-describedby="source-website-boundary" /><small id="source-website-boundary">{t("sourceDialogs.websitePathBoundary")}</small></label>}
      </div>
    </Dialog>

    <Dialog
      open={Boolean(sourceReview)}
      onClose={(open) => { if (!open && !sourceReviewBusy) closeSourceReview(); }}
      title={t("sourceDialogs.review", { value1: String(sourceReview?.source.name ?? t("navigation.docs")) })}
      description={sourceReviewAttachIntegrationID ? t("sourceDialogs.approveTheExactCrawlGenerationDokoSokoWillPublishThe") : t("sourceDialogs.approveTheExactCompletedCrawlGenerationAndOnlyThe")}
      actions={<><Button outline disabled={sourceReviewBusy} onClick={closeSourceReview}>{t("common.cancel")}</Button><Button color="indigo" disabled={sourceReviewBusy || Boolean(sourceReview?.publication) || Boolean(sourceReview?.source.quarantined) || !sourceReviewAcknowledged || sourceReviewSelection.length === 0} onClick={confirmSourcePublication}>{sourceReviewBusy ? t("sourceDialogs.publishing") : sourceReviewAttachIntegrationID ? t("sourceDialogs.publishAttach") : t("sourceDialogs.publishReviewedGeneration")}</Button></>}
    >
      {sourceReview && <div className="mcp-import-review">
        {sourceReviewAttachIntegrationID && <div className="private-default-note"><LockKeyhole />{t("sourceDialogs.onlyTheReviewedPublicationIsAttachedAndTheAPI")}</div>}
        <div className="import-summary"><Badge color={sourceReview.publication ? "green" : "amber"}>{sourceReview.publication ? t("sourceDialogs.publishedR", { revision: String(sourceReview.publication.revision) }) : t("sourceDialogs.needsReview")}</Badge><code>{sourceReview.crawl_job.id}</code><span>{t("sourceDialogs.fetchedAndChanged", { fetched: sourceReview.documents.length, changed: sourceReview.crawl_job.changed_count })}</span></div>
        <div className="catalog-list">{sourceReview.documents.map((document) => {
          const safe = (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0;
          const selected = sourceReviewSelection.includes(document.id);
          return <label className="catalog-tool" key={document.id}><input type="checkbox" disabled={!safe || Boolean(sourceReview.publication)} checked={selected} onChange={(event) => setSourceReviewSelection((items) => event.target.checked ? [...items, document.id] : items.filter((id) => id !== document.id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{document.title}</strong><code>{document.canonical_url}</code><small>{document.changed ? t("sourceDialogs.changedInThisGeneration") : t("sourceDialogs.unchangedSnapshotReused")} {t("sourceDialogs.trust")} {document.trust_level}/100</small>{document.injection_indicators.length > 0 && <small>{t("sourceDialogs.classifierIndicators")} {document.injection_indicators.join(", ")}</small>}</span><Badge color={safe ? "zinc" : "red"}>{safe ? document.state : t("sourceDialogs.quarantined")}</Badge></label>;
        })}</div>
        {!sourceReview.publication && <Confirmation checked={sourceReviewAcknowledged} onChange={setSourceReviewAcknowledged}>{t("sourceDialogs.iReviewedGeneration")} {sourceReview.crawl_job.id}{t("sourceDialogs.itsChangedAndUnchangedPagesAndTheExact")} {sourceReviewSelection.length} {t("sourceDialogs.selectedDocument")}{sourceReviewSelection.length === 1 ? "" : t("sourceDialogs.s")}.</Confirmation>}
        <div className="private-default-note"><ShieldCheck />{t("sourceDialogs.publishingCreatesAnImmutableSourcePublicationIDAndContent")}</div>
      </div>}
    </Dialog>
  </>;
}
