"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { DeveloperAssetRecord, SDKContentCandidateRecord } from "../../../lib/developer-assets-api";
import { Badge, Button } from "../../core/control";
import { MarkdownEvidence, PrettyJSON, recordID, recordString, recordTitle } from "./developer-asset-ui";
import { sampleValidated, type SDKDecisionState } from "./sdk-catalog-helpers";

type ReviewKind = "file" | "sample";
export function SDKGuidanceReview({ record, previous, files, samples, readOnly, disabled = false, onChange }: {
  record: SDKContentCandidateRecord; previous?: SDKContentCandidateRecord;
  files: SDKDecisionState; samples: SDKDecisionState; readOnly: boolean; disabled?: boolean;
  onChange: (kind: ReviewKind, id: string, value: SDKDecisionState[string]) => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState(recordID(record.files[0] ?? record.samples[0] ?? {}, ""));
  const [compare, setCompare] = useState(false);
  const all = [...record.files.map((value) => ({ value, kind: "file" as const })), ...record.samples.map((value) => ({ value, kind: "sample" as const }))];
  const visible = all.filter(({ value }) => `${recordTitle(value, "")} ${recordString(value, "source_path")} ${recordString(value, "language")}`.toLowerCase().includes(query.toLowerCase()));
  const selected = all.find(({ value }) => recordID(value, "") === selectedID);
  const current = selected ? (selected.kind === "file" ? files : samples)[selectedID] ?? { decision: "", reason: "", reviewEvidence: "" } : undefined;
  const sourceFile = selected?.kind === "file" ? selected.value : selected ? record.files.find((value) => recordID(value, "") === recordString(selected.value, "sdk_publication_file_id")) : undefined;
  const previousRecord = previous?.candidate.sdk_release_id === record.candidate.sdk_release_id && previous.candidate.deployment_id === record.candidate.deployment_id ? previous : undefined;
  const matchingPreviousFiles = sourceFile ? previousRecord?.files.filter((value) => recordString(value, "source_path") === recordString(sourceFile, "source_path")) ?? [] : [];
  const previousValue = matchingPreviousFiles.length === 1 ? matchingPreviousFiles[0] : undefined;
  const removed = previousRecord?.files.filter((value) => !record.files.some((file) => recordString(file, "source_path") === recordString(value, "source_path"))) ?? [];
  function sampleCaption(value: DeveloperAssetRecord) {
    const start = value.source_start, end = value.source_end;
    // Extracted sample ranges use zero-based lines with an exclusive end.
    const location = value.origin === "extracted" && typeof start === "number" && typeof end === "number" && Number.isInteger(start) && Number.isInteger(end) && start >= 0 && end > start
      ? t("sdkSetup.sourceLines", { start: start + 1, end }) : "";
    return [recordString(value, "language"), recordString(value, "source_path"), location].filter(Boolean).join(" · ");
  }
  function content(value: DeveloperAssetRecord, kind: ReviewKind = "file") {
    const title = recordTitle(value, t("sdkSetup.guidance"));
    return kind === "file" && (/markdown/.test(recordString(value, "media_type")) || recordString(value, "language") === "markdown") ? <MarkdownEvidence label={title}>{recordString(value, "normalized_content")}</MarkdownEvidence> : <pre className="developer-asset-markdown"><code>{recordString(value, kind === "sample" ? "code" : "normalized_content") || t("sdkCatalog.contentWithheld")}</code></pre>;
  }
  const change = (patch: Partial<SDKDecisionState[string]>) => { if (selected && current && !readOnly && !disabled) onChange(selected.kind, selectedID, { ...current, ...patch }); };
  return <div className="sdk-guidance-review">
    <div className="sdk-guidance-directory"><label className="auth-field"><span>{t("sdkSetup.findContent")}</span><input value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      {visible.map(({ value, kind }) => { const id = recordID(value, ""); const decision = (kind === "file" ? files : samples)[id]?.decision; return <button type="button" key={`${kind}:${id}`} aria-current={id === selectedID ? "true" : undefined} onClick={() => { setSelectedID(id); setCompare(false); }}><strong>{recordTitle(value, kind === "file" ? t("sdkCatalog.file") : t("sdkSetup.sample"))}</strong>{kind === "sample" && <small>{sampleCaption(value)}</small>}<small>{t(`sdkSetup.decisions.${decision || "pending"}`)}{kind === "sample" ? ` · ${t("sdkSetup.sample")}` : ""}</small></button>; })}
      {visible.length === 0 && <p>{t("sdkSetup.noMatchingContent")}</p>}
    </div>
    <section className="sdk-guidance-reader" aria-label={t("sdkSetup.exactContent")}>
      {selected && current ? <><header><h3>{recordTitle(selected.value, t("sdkSetup.guidance"))}</h3>{previousRecord && selected.kind === "file" && <Badge>{t(!previousValue ? "sdkSetup.added" : recordString(previousValue, "content_hash") === recordString(selected.value, "content_hash") ? "sdkSetup.unchanged" : "sdkSetup.changed")}</Badge>}</header>
        {selected.kind === "sample" && <><p>{sampleCaption(selected.value)}</p><p>{t(sampleValidated(selected.value) ? "sdkCatalog.machineEvidencePassed" : "sdkCatalog.explicitReviewEvidenceRequired")}</p></>}
        {selected.kind === "sample" && content(selected.value, "sample")}
        {previousValue && <Button outline onClick={() => setCompare((value) => !value)}>{t(compare ? "sdkSetup.hideComparison" : selected.kind === "sample" ? "sdkSetup.compareSourceFile" : "sdkSetup.comparePrevious")}</Button>}
        {compare && previousValue && sourceFile ? <>{selected.kind === "sample" && <p>{t("sdkSetup.sourceFileComparison", { path: recordString(sourceFile, "source_path") })}</p>}<div className="source-review-diff"><section><h4>{t("sdkSetup.previousPublished")}</h4>{content(previousValue)}</section><section><h4>{t("sdkSetup.currentImport")}</h4>{content(sourceFile)}</section></div></> : selected.kind === "file" ? content(selected.value) : null}
        {!readOnly ? <div className="sdk-guidance-decision auth-form compact-form"><label className="auth-field"><span>{t("sdkCatalog.decision")}</span><select value={current.decision} disabled={disabled} onChange={(event) => change({ decision: event.target.value as SDKDecisionState[string]["decision"] })}><option value="">{t("sdkCatalog.choose")}</option>{selected.kind === "file" ? <option value="included">{t("sdkCatalog.include")}</option> : <option value="approved">{t("sdkCatalog.approve")}</option>}<option value="excluded">{t("sdkCatalog.exclude")}</option><option value="quarantined">{t("sdkCatalog.quarantine")}</option></select></label>
          {["excluded", "quarantined"].includes(current.decision) && <label className="auth-field"><span>{t("sdkCatalog.reason")}</span><textarea value={current.reason} disabled={disabled} onChange={(event) => change({ reason: event.target.value })} /></label>}
          {selected.kind === "sample" && current.decision === "approved" && !sampleValidated(selected.value) && <label className="auth-field"><span>{t("sdkCatalog.explicitReviewEvidence")}</span><textarea value={current.reviewEvidence} disabled={disabled} onChange={(event) => change({ reviewEvidence: event.target.value })} /><small>{t("sdkCatalog.summarizeTheManualChecksThatJustifyApprovalOfThis")}</small></label>}
        </div> : <div><p>{t("sdkSetup.publishedDecision", { decision: t(`sdkSetup.decisions.${current.decision || "pending"}`) })}</p>{current.reason && <p>{t("sdkCatalog.reason")}: {current.reason}</p>}{current.reviewEvidence && <p>{t("sdkCatalog.explicitReviewEvidence")}: {current.reviewEvidence}</p>}</div>}
        <details><summary>{t("sdkSetup.evidenceDetails")}</summary><PrettyJSON value={selected.value} /></details>
      </> : <p>{t("sdkSetup.selectContent")}</p>}
    </section>
    {removed.length > 0 && <details className="sdk-guidance-removed"><summary>{t("sdkSetup.removedFiles", { count: removed.length })}</summary>{removed.map((file) => <details key={recordID(file, "")}><summary>{recordTitle(file, t("sdkCatalog.file"))}</summary>{content(file)}</details>)}</details>}
  </div>;
}
