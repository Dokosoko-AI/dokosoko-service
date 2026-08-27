"use client";

import { Check, LockKeyhole, ShieldCheck } from "lucide-react";

import { Badge, Button, Dialog } from "../../core/control";
import { Confirmation } from "../shared";
import { type SourceKind, type useSourceWorkflow } from "../use-source-workflow";

export function SourceDialogs({ workspace }: {
  workspace: ReturnType<typeof useSourceWorkflow>;
}) {
  const {
    addSourceOpen,
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
    closeSourceReview,
    confirmSourcePublication,
  } = workspace;

  return <>
    <Dialog
      open={addSourceOpen}
      onClose={closeSourceDialog}
      title="Add knowledge source"
      description="Add a URL-backed source or upload one text document. Every source starts private and draft for review before publication."
      actions={<><Button outline disabled={sourceBusy} onClick={() => closeSourceDialog(false)}>Cancel</Button><Button color="indigo" disabled={sourceBusy || !sourceName.trim() || (sourceKind === "upload" ? !sourceFile || Boolean(sourceFileError) : !sourceLocation.trim())} onClick={createSource}>{sourceBusy ? "Adding…" : sourceKind === "upload" ? "Upload source" : "Add source"}</Button></>}
    >
      <div className="auth-form compact-form"><label className="auth-field"><span>Name</span><input value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="Developer documentation" /></label><label className="auth-field"><span>Type</span><select value={sourceKind} onChange={(event) => selectSourceKind(event.target.value as SourceKind)}><option value="website">Website</option><option value="openapi">OpenAPI</option><option value="git">Git repository</option><option value="upload">Upload a file</option></select></label>{sourceKind === "upload" ? <label className="auth-field"><span>File</span><input ref={sourceFileInput} type="file" accept=".md,.mdx,.txt,.html,.htm,.json,.yaml,.yml,text/plain,text/markdown,text/html,application/json,application/yaml,text/yaml" aria-invalid={Boolean(sourceFileError)} aria-describedby={`source-upload-guidance${sourceFileError ? " source-upload-error" : ""}`} onChange={(event) => selectSourceFile(event.target.files?.[0] ?? null)} /><small id="source-upload-guidance">UTF-8 .md, .mdx, .txt, .html, .htm, .json, .yaml, or .yml; up to 5 MB in this setup. Content is treated as untrusted text, and embedded scripts are never executed.</small>{sourceFileError && <small id="source-upload-error" className="source-upload-error" role="alert">{sourceFileError}</small>}</label> : <label className="auth-field"><span>Location</span><input type="url" value={sourceLocation} onChange={(event) => setSourceLocation(event.target.value)} placeholder={sourceKind === "git" ? "https://github.com/vendor/docs" : sourceKind === "openapi" ? "https://api.example.com/openapi.yaml" : "https://docs.example.com"} /></label>}</div>
    </Dialog>

    <Dialog
      open={Boolean(sourceReview)}
      onClose={(open) => { if (!open && !sourceReviewBusy) closeSourceReview(); }}
      title={`Review ${sourceReview?.source.name ?? "documentation"}`}
      description={sourceReviewAttachIntegrationID ? "Approve the exact crawl generation. DokoSoko will publish the selected immutable pages, create or reuse a reviewed documentation set, and pin its exact revision to this API." : "Approve the exact completed crawl generation and only the immutable pages that should be available to APIs."}
      actions={<><Button outline disabled={sourceReviewBusy} onClick={closeSourceReview}>Cancel</Button><Button color="indigo" disabled={sourceReviewBusy || Boolean(sourceReview?.publication) || Boolean(sourceReview?.source.quarantined) || !sourceReviewAcknowledged || sourceReviewSelection.length === 0} onClick={confirmSourcePublication}>{sourceReviewBusy ? "Publishing…" : sourceReviewAttachIntegrationID ? "Publish & attach" : "Publish reviewed generation"}</Button></>}
    >
      {sourceReview && <div className="mcp-import-review">
        {sourceReviewAttachIntegrationID && <div className="private-default-note"><LockKeyhole />Only the reviewed publication is attached, and the API receives a pin to its exact immutable resource-set revision.</div>}
        <div className="import-summary"><Badge color={sourceReview.publication ? "green" : "amber"}>{sourceReview.publication ? `Published r${sourceReview.publication.revision}` : "Needs review"}</Badge><code>{sourceReview.crawl_job.id}</code><span>{sourceReview.documents.length} fetched · {sourceReview.crawl_job.changed_count} changed</span></div>
        <div className="catalog-list">{sourceReview.documents.map((document) => {
          const safe = (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0;
          const selected = sourceReviewSelection.includes(document.id);
          return <label className="catalog-tool" key={document.id}><input type="checkbox" disabled={!safe || Boolean(sourceReview.publication)} checked={selected} onChange={(event) => setSourceReviewSelection((items) => event.target.checked ? [...items, document.id] : items.filter((id) => id !== document.id))} /><span className="check-box">{selected && <Check />}</span><span><strong>{document.title}</strong><code>{document.canonical_url}</code><small>{document.changed ? "Changed in this generation" : "Unchanged snapshot reused"} · trust {document.trust_level}/100</small>{document.injection_indicators.length > 0 && <small>Classifier indicators: {document.injection_indicators.join(", ")}</small>}</span><Badge color={safe ? "zinc" : "red"}>{safe ? document.state : "quarantined"}</Badge></label>;
        })}</div>
        {!sourceReview.publication && <Confirmation checked={sourceReviewAcknowledged} onChange={setSourceReviewAcknowledged}>I reviewed generation {sourceReview.crawl_job.id}, its changed and unchanged pages, and the exact {sourceReviewSelection.length} selected document{sourceReviewSelection.length === 1 ? "" : "s"}.</Confirmation>}
        <div className="private-default-note"><ShieldCheck />Publishing creates an immutable source-publication ID and content hash. Later crawls require a new review; failed or partial generations cannot replace this one.</div>
      </div>}
    </Dialog>
  </>;
}
