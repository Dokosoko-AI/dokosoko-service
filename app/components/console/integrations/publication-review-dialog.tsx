"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APIIntegrationDetail } from "../../../lib/api";
import { developerAssetsApi } from "../../../lib/developer-assets-api";
import { integrationPath, integrationValidationPath } from "../../../lib/console-routes";
import { integrationPublicationRows, publicationRecord, publicationRecords, publicationText as text, resolvePublicationReviewRows, reviewedPublicationInput, type PublicationRecord, type PublicationSelection, type ResolvedPublicationReviewRow } from "../../../lib/integration-publication-review";
import { sdkSetupPath } from "../../../lib/sdk-setup";
import { Badge, Button, Dialog } from "../../core/control";
import { ConsoleLink } from "../console-link";
import { developerAssetError, enumLabel, LoadingPanel, PrettyJSON } from "../developer-assets/developer-asset-ui";

const groups = ["documentation", "contracts", "sdks", "global", "tools", "authorization", "connections"] as const;
const metadataFields = ["display_name", "family_key", "version_key", "description", "visibility", "lifecycle", "replacement_integration_id", "sunset_at"] as const;
type Review = { detail: APIIntegrationDetail; rows: ResolvedPublicationReviewRow[] };

export function IntegrationPublicationReviewDialog({ integrationID, onClose, onPublished, onNavigate }: { integrationID: string; onClose: () => void; onPublished: (id: string) => Promise<void>; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const [review, setReview] = useState<Review | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const [published, setPublished] = useState(false);
  const mounted = useRef(false);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    let cancelled = false;
    queueMicrotask(async () => {
      if (cancelled) return;
      setLoading(true); setAcknowledged(false); setReview(null);
      try {
        const detail = await api.integration(integrationID);
        if (detail.integration.id !== integrationID || detail.publish_status.integration_id !== integrationID) throw Error(t("publicationReview.loadFailed"));
        const status = detail.publish_status;
        const rows = await resolvePublicationReviewRows(integrationPublicationRows(publicationRecord(status.current_snapshot), publicationRecord(status.latest_revision?.snapshot)), detail.integration.deployment_id, developerAssetsApi);
        if (!cancelled) setReview({ detail, rows });
      } catch { if (!cancelled) setProblem(t("publicationReview.loadFailed")); }
      finally { if (!cancelled) setLoading(false); }
    });
    return () => { cancelled = true; };
  }, [attempt, integrationID, t]);

  async function publish() {
    if (!review || !acknowledged || busy || published) return;
    setBusy(true); setProblem("");
    try {
      const input = reviewedPublicationInput(review.detail.publish_status, integrationID);
      // Revalidation belongs to the server. Never fetch and publish a newer
      // candidate here: the operator has only reviewed the captured selection.
      await api.publishIntegration(input.integrationID, input.candidateRevision, input.candidateManifestHash);
      if (!mounted.current) return;
      setPublished(true);
      await onPublished(integrationID);
      if (mounted.current) onClose();
    } catch (error) {
      if (mounted.current) { setAcknowledged(false); setProblem(developerAssetError(error, t("publicationReview.publishFailed"))); }
    } finally { if (mounted.current) setBusy(false); }
  }
  const status = review?.detail.publish_status;
  const snapshot = publicationRecord(status?.current_snapshot);
  const previous = publicationRecord(status?.latest_revision?.snapshot);
  const changedMetadata = metadataFields.filter((key) => text(snapshot[key]) !== text(previous[key]));
  function navigate(path: string) { onClose(); onNavigate(path); }
  function metadataValue(record: PublicationRecord, key: typeof metadataFields[number]) {
    if (!text(record[key])) return t("publicationReview.removedValue");
    if (key === "visibility" || key === "lifecycle") return enumLabel(t, text(record[key]));
    return key === "replacement_integration_id" ? <ConsoleLink path={integrationPath(text(record[key]))} onNavigate={navigate}>{t("publicationReview.openReplacement")}</ConsoleLink> : text(record[key]);
  }
  return <Dialog open onClose={() => { if (!busy) onClose(); }} title={t("publicationReview.title", { name: text(snapshot.display_name) || t("publicationReview.api") })} description={t("publicationReview.description")} actions={<>
    <Button outline disabled={busy} onClick={onClose}>{t(published ? "common.close" : "common.cancel")}</Button>
    {!published && <><Button outline disabled={busy || loading} onClick={() => { setProblem(""); setAttempt((value) => value + 1); }}>{t("publicationReview.refresh")}</Button><Button disabled={loading || busy || !status?.ready || !status.has_changes || !acknowledged} onClick={() => void publish()}>{t(busy ? "integrations.publishing" : "integrations.publish")}</Button></>}
  </>}>
    {loading ? <LoadingPanel label={t("publicationReview.loading")} /> : review && <div className="integration-publication-review">
      <div className="sdk-release-facts"><strong>{text(snapshot.display_name)} · {text(snapshot.version_key)}</strong><Badge>{enumLabel(t, text(snapshot.visibility))}</Badge><Badge color={status?.ready ? "green" : "amber"}>{t(status?.ready ? "publicationReview.ready" : "publicationReview.blocked")}</Badge></div>
      <p>{t(snapshot.visibility === "public" ? "publicationReview.publicAudience" : "publicationReview.privateAudience")}</p>
      {status?.validations.map((validation, index) => <div key={`${validation.code}:${index}`} className="publication-review-validation"><p>{validation.message}</p><ConsoleLink path={integrationValidationPath(integrationID, validation.tab)} onNavigate={navigate}>{t("publicationReview.resolve")}</ConsoleLink></div>)}
      {status?.latest_revision ? <p>{t("publicationReview.previous", { revision: status.latest_revision.revision })}</p> : <p>{t("publicationReview.first")}</p>}
      {status?.latest_revision && <p>{status.serving_revision ? t("publicationReview.serving", { revision: status.serving_revision }) : t("publicationReview.notServing")}</p>}
      {status?.latest_revision && !status.latest_delivery_ready && <p>{t("publicationReview.pendingDelivery", { revision: status.latest_revision.revision })}</p>}
      {!status?.has_changes && <p>{t("publicationReview.noChanges")}</p>}
      {changedMetadata.length > 0 && <details><summary>{t("publicationReview.apiDetails")}</summary><dl>{changedMetadata.map((key) => <div key={key}><dt>{t(`publicationReview.fields.${key}`)}</dt><dd>{status?.latest_revision && <>{metadataValue(previous, key)} → </>}{metadataValue(snapshot, key)}</dd></div>)}</dl></details>}
      {groups.map((kind) => { const values = review.rows.filter((row) => row.kind === kind); return values.length ? <section key={kind}><h3>{t(`publicationReview.groups.${kind}`)}</h3>{values.map((row) => <PublicationRow key={row.key} row={row} integrationID={integrationID} onNavigate={navigate} />)}</section> : null; })}
      {!review.rows.some((row) => row.kind === "tools" && row.state !== "removed") && <p>{t("publicationReview.noTools")}</p>}
      <p>{t("publicationReview.reviewEvidenceHelp")}</p>
      {review.rows.some((row) => row.kind === "tools" && row.state !== "removed") && <p>{t("publicationReview.liveChecks")}</p>}
      {status?.has_changes && !published && <label className="compact-check"><input type="checkbox" checked={acknowledged} disabled={busy || !status.ready} onChange={(event) => setAcknowledged(event.target.checked)} /><span>{t("publicationReview.acknowledge")}</span></label>}
      {published && <p>{t("publicationReview.published")}</p>}
      <details><summary>{t("integrations.technicalDetails")}</summary><PrettyJSON value={{ candidate_revision: status?.candidate_revision, manifest_hash: status?.current_manifest_hash, changes: status?.changes, snapshot }} /></details>
    </div>}
    {problem && <div role="alert"><p className="auth-problem">{problem}</p>{review && !published && <p>{t("publicationReview.retryHelp")}</p>}</div>}
  </Dialog>;
}

function PublicationRow({ row, integrationID, onNavigate }: { row: ResolvedPublicationReviewRow; integrationID: string; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  const value = row.value;
  const audienceLabel = (value: unknown) => text(value) ? enumLabel(t, text(value)) : t("publicationReview.removedValue");
  const packageID = text(value.sdk_package_id), releaseID = text(value.sdk_release_id);
  function selectionLabel(selection: PublicationSelection) {
    return [selection.version, selection.revision !== undefined ? t("publicationReview.revision", { revision: selection.revision }) : "", selection.guidanceRevision !== undefined ? t("publicationReview.guidanceRevision", { revision: selection.guidanceRevision }) : ""].filter(Boolean).join(" · ");
  }
  function details(record: PublicationRecord) {
    const effect = text(record.effect) || text(record.action_type);
    return <div className="sdk-release-facts">
      {text(record.visibility) && <Badge>{audienceLabel(record.visibility)}</Badge>}
      {record.primary === true && <span>{t("publicationReview.primaryContract")}</span>}
      {effect && <span>{t("publicationReview.effect", { effect })}</span>}
      {Array.isArray(record.required_grants) && <span>{t("publicationReview.grants", { grants: record.required_grants.map(text).join(", ") || t("publicationReview.noGrants") })}</span>}
      {typeof record.confirmation_required === "boolean" && <span>{t(record.confirmation_required ? "publicationReview.confirmation" : "publicationReview.noConfirmation")}</span>}
      {publicationRecords(record.current_revisions).map((revision, index) => <span key={index}>{text(revision.base_url)} · {text(revision.authentication_type)}</span>)}
    </div>;
  }
  const currentLabel = selectionLabel(row.selection), previousLabel = row.previousSelection ? selectionLabel(row.previousSelection) : "";
  return <article className="publication-review-row">
    <div className="sdk-release-facts"><strong>{row.kind === "global" ? t("publicationReview.global") : row.title || t(`publicationReview.groups.${row.kind}`)}</strong><Badge color={row.state === "removed" ? "amber" : row.state === "unchanged" ? "zinc" : "blue"}>{t(`publicationReview.states.${row.state}`)}</Badge></div>
    <p>{row.state === "changed" && previousLabel && previousLabel !== currentLabel && <>{previousLabel} → </>}{currentLabel}</p>
    {row.state === "changed" && row.previous && text(row.previous.visibility) !== text(value.visibility) && <p>{t("publicationReview.audienceChanged", { before: audienceLabel(row.previous.visibility), after: audienceLabel(value.visibility) })}</p>}
    {details(value)}
    {row.state === "removed" && <p>{t("publicationReview.removalHelp")}</p>}
    {row.state === "changed" && row.previous && <details><summary>{t("publicationReview.previousSelection")}</summary>{details(row.previous)}<p>{t("publicationReview.exactSelectionChanged")}</p></details>}
    {row.kind === "sdks" && packageID && releaseID && <ConsoleLink path={sdkSetupPath({ package: packageID, release: releaseID, publication: text(value.sdk_content_publication_id), api: integrationID, step: "review" })} onNavigate={onNavigate}>{t("publicationReview.inspectGuidance")}</ConsoleLink>}
  </article>;
}
