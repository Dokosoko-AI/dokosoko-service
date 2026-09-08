"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APICrawlReviewDocument, type APISourceReviewContent } from "../../../lib/api";
import { Badge, Button } from "../../core/control";
import { EvidenceContent, evidenceChange } from "../../core/evidence-content";
import { developerAssetError } from "./developer-asset-ui";

export function SourceReviewDocument({ productID, sourceID, document, selected, onSelected, publishedDecision, disabled }: {
  productID: string; sourceID: string; document: APICrawlReviewDocument;
  selected: boolean; onSelected: (selected: boolean) => void; disabled: boolean;
  publishedDecision?: "included" | "excluded" | "unknown";
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState<APISourceReviewContent | null>(null);
  const [problem, setProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [compare, setCompare] = useState(false);
  const safe = (document.state === "validated" || document.state === "published") && document.injection_indicators.length === 0;

  useEffect(() => {
    if (!open || content) return;
    let cancelled = false;
    api.sourceReviewContent(productID, sourceID, document.crawl_job_id, document.id).then((value) => {
      if (cancelled) return;
      if (value.document.id !== document.id || value.document.crawl_job_id !== document.crawl_job_id || value.document.content_hash !== document.content_hash) {
        setProblem(t("sourceContent.changed")); return;
      }
      setContent(value); setProblem("");
    }).catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("sourceContent.unavailable"))); });
    return () => { cancelled = true; };
  }, [attempt, content, document.content_hash, document.crawl_job_id, document.id, open, productID, sourceID, t]);
  const change = content?.previous ? evidenceChange(content.previous.body, content.body) : null;

  return <article className="source-review-document">
    <header>
      <label className="compact-check">{!publishedDecision && <input type="checkbox" checked={selected} disabled={disabled || !safe} onChange={(event) => onSelected(event.target.checked)} />}<span><strong>{document.title}</strong><small>{/^https?:\/\//i.test(document.canonical_url) ? document.canonical_url : t("sourceContent.importedFile")}</small></span></label>
      <Badge color={!safe ? "red" : publishedDecision === "included" ? "green" : "zinc"}>{!safe ? t("sourceContent.blocked") : publishedDecision ? t(`sourceContent.${publishedDecision}`) : document.changed ? t("sourceContent.changedImport") : t("sourceContent.unchangedImport")}</Badge>
      <Button outline onClick={() => setOpen((value) => !value)}>{t(open ? "sourceContent.close" : "sdkCatalog.readExactContent")}</Button>
    </header>
    {document.injection_indicators.length > 0 && <p className="auth-problem">{t("sourceDialogs.classifierIndicators")} {document.injection_indicators.join(", ")}</p>}
    {open && <div className="source-review-content">
      {problem ? <><p className="auth-problem" role="alert">{problem}</p><Button outline onClick={() => { setProblem(""); setAttempt((value) => value + 1); }}>{t("common.retry")}</Button></> : !content ? <p aria-live="polite">{t("common.loading")}</p> : <>
        <p>{t("sourceContent.storedText")}</p>
        {content.previous ? <><p>{t("sourceContent.previous", { revision: content.previous.publication_revision })}</p><label className="compact-check"><input type="checkbox" checked={compare} onChange={(event) => setCompare(event.target.checked)} /><span>{t("sourceContent.compare")}</span></label></> : <p>{t("sourceContent.noPrevious")}</p>}
        {compare && change ? change.unchanged ? <p>{t("sourceContent.unchangedText")}</p> : <div className="source-review-diff"><section><h4>{t("sourceContent.before")}</h4><pre>{change.removed || t("sourceContent.noText")}</pre></section><section><h4>{t("sourceContent.after")}</h4><pre>{change.added || t("sourceContent.noText")}</pre></section></div> : <EvidenceContent text={content.body} label={document.title} />}
        <details><summary>{t("sourceContent.provenance")}</summary><dl className="entity-detail-grid"><div><dt>{t("sourceContent.location")}</dt><dd><code>{document.canonical_url}</code></dd></div><div><dt>{t("documentationExplorer.contentHash")}</dt><dd><code>{document.content_hash}</code></dd></div><div><dt>{t("sourceContent.generation")}</dt><dd><code>{document.crawl_job_id}</code></dd></div></dl></details>
      </>}
    </div>}
  </article>;
}
