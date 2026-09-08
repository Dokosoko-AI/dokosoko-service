"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APIIntegrationPublishStatus } from "../../../lib/api";
import { Badge, Button } from "../../core/control";
import { PanelHeader } from "../../core/layout";
import { developerAssetError } from "../developer-assets/developer-asset-ui";

export function IntegrationPublicationDeliveryStatus({ status, onChanged }: { status: APIIntegrationPublishStatus; onChanged: () => void | Promise<void> }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const latest = status.latest_revision;
  if (!latest) return null;
  async function retry() {
    if (!latest || busy) return;
    setBusy(true); setProblem("");
    try {
      // Repair only the saved revision shown beside the action. Draft changes
      // are never inputs to delivery retry.
      await api.activateIntegrationRevision(status.integration_id, latest.id);
      await onChanged();
    } catch (error) { setProblem(developerAssetError(error, t("publicationReview.deliveryRetryFailed"))); }
    finally { setBusy(false); }
  }
  return <section className="panel integration-delivery-status">
    <PanelHeader title={t("publicationReview.deliveryTitle")} action={<Badge color={status.latest_delivery_ready ? "green" : "amber"}>{t(status.latest_delivery_ready ? "publicationReview.deliveryReady" : "publicationReview.deliveryPending")}</Badge>} />
    <div className="integration-delivery-content">
      <p>{status.serving_revision ? t("publicationReview.serving", { revision: status.serving_revision }) : t("publicationReview.notServing")}</p>
      {!status.latest_delivery_ready && <><p>{t("publicationReview.pendingDelivery", { revision: latest.revision })}</p><Button outline disabled={busy} onClick={() => void retry()}>{t(busy ? "publicationReview.retryingDelivery" : "publicationReview.retryDelivery", { revision: latest.revision })}</Button></>}
      {problem && <p className="auth-problem" role="alert">{problem}</p>}
    </div>
  </section>;
}
